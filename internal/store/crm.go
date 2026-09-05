package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type CRMJob struct {
	ID           int64
	EnrollmentID int64
	Action       string
	Body         string
	ContactID    string
	LocationID   string
	Phone        string
	Reply        string
	Attempts     int
}

func (s *Store) ClaimCRM(ctx context.Context) (CRMJob, error) {
	var job CRMJob
	var payload []byte
	err := s.DB.QueryRow(ctx, `
		WITH next AS (
			SELECT id FROM crm_jobs
			WHERE (status='queued' AND available_at<=now())
			   OR (status='running' AND locked_until<now())
			ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE crm_jobs c SET status='running',attempts=attempts+1,locked_until=now()+interval '2 minutes',updated_at=now()
		FROM next WHERE c.id=next.id
		RETURNING c.id,c.workflow_enrollment_id,c.action,c.body,c.payload,c.attempts`).Scan(
		&job.ID, &job.EnrollmentID, &job.Action, &job.Body, &payload, &job.Attempts)
	if err != nil {
		return CRMJob{}, err
	}
	var fields struct {
		LocationID string `json:"location_id"`
		ContactID  string `json:"contact_id"`
		Phone      string `json:"phone"`
		Reply      string `json:"reply"`
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &fields)
	}
	job.LocationID = fields.LocationID
	job.ContactID = fields.ContactID
	job.Phone = fields.Phone
	job.Reply = fields.Reply
	return job, nil
}

func (s *Store) CompleteCRM(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE crm_jobs SET status='done',locked_until=NULL,updated_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) RetryCRM(ctx context.Context, id int64, delay time.Duration) error {
	_, err := s.DB.Exec(ctx, `UPDATE crm_jobs SET status='queued',available_at=now()+$2::interval,locked_until=NULL,updated_at=now() WHERE id=$1`, id, fmt.Sprintf("%f seconds", delay.Seconds()))
	return err
}

func (s *Store) FailCRM(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE crm_jobs SET status='failed',locked_until=NULL,updated_at=now() WHERE id=$1`, id)
	return err
}
