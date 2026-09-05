package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/ghl-telnyx-integration/internal/workflow"
	"github.com/jackc/pgx/v5"
)

type WorkflowRecord struct {
	ID         int64
	ExternalID string
	LocationID string
	From       string
	Enrollment workflow.Enrollment
}

func (s *Store) CreateWorkflowEnrollment(ctx context.Context, externalID, locationID, from string, enrollment workflow.Enrollment) (int64, bool, error) {
	variables, err := json.Marshal(enrollment.Contact.Variables)
	if err != nil {
		return 0, false, err
	}
	var id int64
	err = s.DB.QueryRow(ctx, `
		INSERT INTO workflow_enrollments(
			external_id,location_id,workflow_key,contact_id,to_number,from_number,contact_timezone,variables,
			consent_at,consent_source,state,next_run_at,variant,sent_count
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT(location_id,external_id) DO NOTHING
		RETURNING id`,
		externalID, locationID, enrollment.WorkflowKey, enrollment.Contact.ID, enrollment.Contact.Phone,
		from, enrollment.Contact.Timezone, variables, nullableTime(enrollment.Contact.ConsentAt), enrollment.Contact.ConsentSource,
		enrollment.State, nullableTime(enrollment.NextRun), enrollment.Variant, enrollment.SentCount,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

func (s *Store) ClaimDueWorkflow(ctx context.Context) (WorkflowRecord, error) {
	row := s.DB.QueryRow(ctx, `
		WITH next AS (
			SELECT id FROM workflow_enrollments
			WHERE state IN ('pending','awaiting_reply')
			  AND next_run_at IS NOT NULL AND next_run_at<=now()
			  AND (locked_until IS NULL OR locked_until<now())
			ORDER BY next_run_at,id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE workflow_enrollments w SET locked_until=now()+interval '5 minutes',updated_at=now()
		FROM next WHERE w.id=next.id
		RETURNING w.id,w.external_id,w.location_id,w.from_number,w.workflow_key,w.contact_id,
			w.to_number,w.contact_timezone,w.variables,w.consent_at,w.consent_source,w.state,w.next_run_at,w.variant,w.sent_count`)
	return scanWorkflowRecord(row)
}

func (s *Store) ClaimWorkflowsForReply(ctx context.Context, phone, fromNumber string) ([]WorkflowRecord, error) {
	rows, err := s.DB.Query(ctx, `
		WITH active AS (
			SELECT id FROM workflow_enrollments
			WHERE to_number=$1
			  AND ($2='' OR from_number=$2)
			  AND state IN ('pending','awaiting_reply')
			  AND (locked_until IS NULL OR locked_until<now())
			FOR UPDATE SKIP LOCKED
		)
		UPDATE workflow_enrollments w SET locked_until=now()+interval '5 minutes',updated_at=now()
		FROM active WHERE w.id=active.id
		RETURNING w.id,w.external_id,w.location_id,w.from_number,w.workflow_key,w.contact_id,
			w.to_number,w.contact_timezone,w.variables,w.consent_at,w.consent_source,w.state,w.next_run_at,w.variant,w.sent_count`, phone, fromNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []WorkflowRecord
	for rows.Next() {
		record, scanErr := scanWorkflowRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) ApplyWorkflowCommands(ctx context.Context, record WorkflowRecord, commands []workflow.Command, reply string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE workflow_enrollments SET state=$2,next_run_at=$3,variant=$4,sent_count=$5,
			last_reply=CASE WHEN $6='' THEN last_reply ELSE $6 END,locked_until=NULL,updated_at=now()
		WHERE id=$1 AND locked_until IS NOT NULL`,
		record.ID, record.Enrollment.State, nullableTime(record.Enrollment.NextRun), record.Enrollment.Variant,
		record.Enrollment.SentCount, reply)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("workflow enrollment %d is not claimed", record.ID)
	}

	for _, command := range commands {
		switch command.Type {
		case workflow.CommandSendSMS:
			messageID := fmt.Sprintf("workflow:%d:%d", record.ID, record.Enrollment.SentCount)
			payload, _ := json.Marshal(map[string]any{
				"workflow_key":  record.Enrollment.WorkflowKey,
				"enrollment_id": record.ID,
				"location_id":   record.LocationID,
				"contact_id":    record.Enrollment.Contact.ID,
				"message_id":    messageID,
				"to":            record.Enrollment.Contact.Phone,
				"from":          record.From,
				"text":          command.Body,
			})
			availableAt, rateErr := reserveWorkflowRate(ctx, tx, record.Enrollment.WorkflowKey, command.Rate)
			if rateErr != nil {
				return rateErr
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO outbound_jobs(location_id,message_id,to_number,from_number,body,payload,workflow_enrollment_id,available_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT(location_id,message_id) DO NOTHING`,
				record.LocationID, messageID, record.Enrollment.Contact.Phone, record.From, command.Body, payload, record.ID, availableAt)
		case workflow.CommandSuppress:
			_, err = tx.Exec(ctx, `
				INSERT INTO suppressions(phone_number,source,provider_event_id) VALUES($1,'workflow_reply','')
				ON CONFLICT(phone_number) DO UPDATE SET source=EXCLUDED.source,updated_at=now()`, record.Enrollment.Contact.Phone)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE outbound_jobs SET status='cancelled',updated_at=now() WHERE to_number=$1 AND status='queued'`, record.Enrollment.Contact.Phone)
			}
		case workflow.CommandCreateTask, workflow.CommandCRMAction, workflow.CommandArchiveConversation:
			payload, _ := json.Marshal(map[string]any{
				"location_id": record.LocationID,
				"contact_id":  record.Enrollment.Contact.ID,
				"phone":       record.Enrollment.Contact.Phone,
				"reply":       reply,
			})
			action := command.Action
			if action == "" {
				action = string(command.Type)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO crm_jobs(workflow_enrollment_id,action,body,payload)
				VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
				record.ID, action, command.Body, payload)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) FailWorkflow(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE workflow_enrollments SET state='failed',next_run_at=NULL,locked_until=NULL,updated_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) RetryWorkflow(ctx context.Context, id int64, delay time.Duration) error {
	_, err := s.DB.Exec(ctx, `UPDATE workflow_enrollments SET next_run_at=now()+$2::interval,locked_until=NULL,updated_at=now() WHERE id=$1`, id, fmt.Sprintf("%f seconds", delay.Seconds()))
	return err
}

func scanWorkflowRecord(row pgx.Row) (WorkflowRecord, error) {
	var record WorkflowRecord
	var variables []byte
	var consentAt *time.Time
	var nextRun *time.Time
	err := row.Scan(
		&record.ID, &record.ExternalID, &record.LocationID, &record.From,
		&record.Enrollment.WorkflowKey, &record.Enrollment.Contact.ID, &record.Enrollment.Contact.Phone,
		&record.Enrollment.Contact.Timezone, &variables, &consentAt, &record.Enrollment.Contact.ConsentSource, &record.Enrollment.State,
		&nextRun, &record.Enrollment.Variant, &record.Enrollment.SentCount,
	)
	if err != nil {
		return WorkflowRecord{}, err
	}
	if err = json.Unmarshal(variables, &record.Enrollment.Contact.Variables); err != nil {
		return WorkflowRecord{}, err
	}
	if consentAt != nil {
		record.Enrollment.Contact.ConsentAt = *consentAt
	}
	if nextRun != nil {
		record.Enrollment.NextRun = *nextRun
	}
	return record, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func reserveWorkflowRate(ctx context.Context, tx pgx.Tx, key string, rate workflow.Rate) (time.Time, error) {
	if rate.BatchSize <= 0 || rate.Per <= 0 {
		rate = workflow.Rate{BatchSize: 1, Per: time.Second}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO workflow_rate_limits(workflow_key,batch_remaining)
		VALUES($1,$2) ON CONFLICT(workflow_key) DO NOTHING`, key, rate.BatchSize)
	if err != nil {
		return time.Time{}, err
	}
	var next time.Time
	var remaining int
	if err = tx.QueryRow(ctx, `SELECT next_allowed_at,batch_remaining FROM workflow_rate_limits WHERE workflow_key=$1 FOR UPDATE`, key).Scan(&next, &remaining); err != nil {
		return time.Time{}, err
	}
	now := time.Now().UTC()
	if next.Before(now) {
		next = now
		remaining = rate.BatchSize
	}
	availableAt := next
	remaining--
	if remaining <= 0 {
		next = next.Add(rate.Per)
		remaining = rate.BatchSize
	}
	_, err = tx.Exec(ctx, `UPDATE workflow_rate_limits SET next_allowed_at=$2,batch_remaining=$3,updated_at=now() WHERE workflow_key=$1`, key, next, remaining)
	return availableAt, err
}
