package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ DB *pgxpool.Pool }
type Outbound struct {
	LocationID, ContactID, MessageID, To, From, Text string
}

func (s *Store) Enqueue(ctx context.Context, o Outbound) (bool, error) {
	b, _ := json.Marshal(o)
	r, e := s.DB.Exec(ctx, `INSERT INTO outbound_jobs(location_id,message_id,to_number,from_number,body,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(location_id,message_id) DO NOTHING`, o.LocationID, o.MessageID, o.To, o.From, o.Text, b)
	return r.RowsAffected() == 1, e
}
func (s *Store) Suppress(ctx context.Context, number, source, event string) error {
	_, e := s.DB.Exec(ctx, `INSERT INTO suppressions(phone_number,source,provider_event_id) VALUES($1,$2,$3) ON CONFLICT(phone_number) DO UPDATE SET source=EXCLUDED.source,provider_event_id=EXCLUDED.provider_event_id,updated_at=now()`, number, source, event)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `UPDATE outbound_jobs SET status='cancelled',updated_at=now() WHERE to_number=$1 AND status='queued'`, number)
	return e
}
func (s *Store) IsSuppressed(ctx context.Context, n string) (bool, error) {
	var b bool
	e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM suppressions WHERE phone_number=$1)`, n).Scan(&b)
	return b, e
}

type Job struct {
	ID int64
	Outbound
	Attempts int
}

func (s *Store) Claim(ctx context.Context) (Job, error) {
	var j Job
	err := s.DB.QueryRow(ctx, `WITH next AS (
		SELECT id FROM outbound_jobs
		WHERE (status='queued' AND available_at<=now()) OR (status='sending' AND locked_until<now())
		ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE outbound_jobs o SET status='sending',attempts=attempts+1,locked_until=now()+interval '2 minutes',updated_at=now()
	FROM next WHERE o.id=next.id
	RETURNING o.id,o.location_id,o.message_id,o.to_number,o.from_number,o.body,o.attempts`).Scan(&j.ID, &j.LocationID, &j.MessageID, &j.To, &j.From, &j.Text, &j.Attempts)
	return j, err
}
func (s *Store) Complete(ctx context.Context, id int64, providerID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE outbound_jobs SET status='sent',provider_message_id=$2,locked_until=NULL,updated_at=now() WHERE id=$1`, id, providerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE delivery_events SET job_id=$1 WHERE provider_message_id=$2 AND job_id IS NULL`, id, providerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Retry(ctx context.Context, id int64, delay time.Duration) error {
	_, e := s.DB.Exec(ctx, `UPDATE outbound_jobs SET status='queued',available_at=now()+$2::interval,locked_until=NULL,updated_at=now() WHERE id=$1`, id, fmt.Sprintf("%f seconds", delay.Seconds()))
	return e
}
func (s *Store) Fail(ctx context.Context, id int64) error {
	_, e := s.DB.Exec(ctx, `UPDATE outbound_jobs SET status='failed',locked_until=NULL,updated_at=now() WHERE id=$1`, id)
	return e
}
func (s *Store) SendingPaused(ctx context.Context) (bool, error) {
	var b bool
	e := s.DB.QueryRow(ctx, `SELECT sending_paused FROM settings WHERE id=1`).Scan(&b)
	return b, e
}
func (s *Store) RecordDelivery(ctx context.Context, eventID, jobID, status string) error {
	_, e := s.DB.Exec(ctx, `INSERT INTO delivery_events(provider_event_id,job_id,status) VALUES($1,NULLIF($2,'')::bigint,$3) ON CONFLICT(provider_event_id) DO NOTHING`, eventID, jobID, status)
	return e
}
func (s *Store) UpdateDelivery(ctx context.Context, eventID, providerID, status string) (bool, error) {
	r, e := s.DB.Exec(ctx, `INSERT INTO delivery_events(provider_event_id,job_id,status,provider_message_id,locked_until)
		VALUES($1,(SELECT id FROM outbound_jobs WHERE provider_message_id=$2),$3,$2,now()+interval '2 minutes')
		ON CONFLICT(provider_event_id) DO NOTHING`, eventID, providerID, status)
	return r.RowsAffected() == 1, e
}

func (s *Store) FindOutboundByProviderID(ctx context.Context, providerID string) (Outbound, error) {
	var outbound Outbound
	var payload []byte
	err := s.DB.QueryRow(ctx, `SELECT payload FROM outbound_jobs WHERE provider_message_id=$1`, providerID).Scan(&payload)
	if err != nil {
		return Outbound{}, err
	}
	if err = json.Unmarshal(payload, &outbound); err != nil {
		return Outbound{}, err
	}
	var alt struct {
		LocationID string `json:"location_id"`
		ContactID  string `json:"contact_id"`
		MessageID  string `json:"message_id"`
		To         string `json:"to"`
		From       string `json:"from"`
		Text       string `json:"text"`
	}
	_ = json.Unmarshal(payload, &alt)
	if outbound.ContactID == "" {
		outbound.ContactID = alt.ContactID
	}
	if outbound.LocationID == "" {
		outbound.LocationID = alt.LocationID
	}
	if outbound.MessageID == "" {
		outbound.MessageID = alt.MessageID
	}
	if outbound.To == "" {
		outbound.To = alt.To
	}
	if outbound.From == "" {
		outbound.From = alt.From
	}
	if outbound.Text == "" {
		outbound.Text = alt.Text
	}
	return outbound, nil
}
func (s *Store) RecordInbound(ctx context.Context, eventID, from, to, body string) (bool, error) {
	result, err := s.DB.Exec(ctx, `INSERT INTO inbound_messages(provider_event_id,from_number,to_number,body,locked_until) VALUES($1,$2,$3,$4,now()+interval '2 minutes') ON CONFLICT(provider_event_id) DO NOTHING`, eventID, from, to, body)
	return result.RowsAffected() == 1, err
}

type InboundMessage struct {
	EventID, From, To, Body string
	Attempts                int
}

func (s *Store) ClaimUnprocessedInbound(ctx context.Context) (InboundMessage, error) {
	var msg InboundMessage
	err := s.DB.QueryRow(ctx, `
		WITH next AS (
			SELECT provider_event_id FROM inbound_messages
			WHERE processed_at IS NULL AND failed_at IS NULL AND (locked_until IS NULL OR locked_until<now())
			ORDER BY created_at,provider_event_id FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE inbound_messages i SET locked_until=now()+interval '2 minutes',attempts=attempts+1
		FROM next WHERE i.provider_event_id=next.provider_event_id
		RETURNING i.provider_event_id,i.from_number,i.to_number,i.body,i.attempts`).Scan(&msg.EventID, &msg.From, &msg.To, &msg.Body, &msg.Attempts)
	return msg, err
}

func (s *Store) MarkInboundProcessed(ctx context.Context, eventID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE inbound_messages SET processed_at=now(),locked_until=NULL WHERE provider_event_id=$1`, eventID)
	return err
}

func (s *Store) RetryInbound(ctx context.Context, eventID string, delay time.Duration) error {
	_, err := s.DB.Exec(ctx, `UPDATE inbound_messages SET locked_until=now()+$2::interval WHERE provider_event_id=$1`, eventID, fmt.Sprintf("%f seconds", delay.Seconds()))
	return err
}

func (s *Store) FailInbound(ctx context.Context, eventID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE inbound_messages SET failed_at=now(),locked_until=NULL WHERE provider_event_id=$1`, eventID)
	return err
}

type DeliveryEvent struct {
	EventID, ProviderMessageID, Status string
	Attempts                           int
}

func (s *Store) ClaimUnprocessedDelivery(ctx context.Context) (DeliveryEvent, error) {
	var event DeliveryEvent
	err := s.DB.QueryRow(ctx, `
		WITH next AS (
			SELECT provider_event_id FROM delivery_events
			WHERE highlevel_synced_at IS NULL AND failed_at IS NULL AND (locked_until IS NULL OR locked_until<now())
			ORDER BY created_at,provider_event_id FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE delivery_events d SET locked_until=now()+interval '2 minutes',attempts=attempts+1
		FROM next WHERE d.provider_event_id=next.provider_event_id
		RETURNING d.provider_event_id,COALESCE(d.provider_message_id,''),d.status,d.attempts`).Scan(&event.EventID, &event.ProviderMessageID, &event.Status, &event.Attempts)
	return event, err
}

func (s *Store) MarkDeliverySynced(ctx context.Context, eventID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE delivery_events SET highlevel_synced_at=now(),locked_until=NULL WHERE provider_event_id=$1`, eventID)
	return err
}

func (s *Store) RetryDelivery(ctx context.Context, eventID string, delay time.Duration) error {
	_, err := s.DB.Exec(ctx, `UPDATE delivery_events SET locked_until=now()+$2::interval WHERE provider_event_id=$1`, eventID, fmt.Sprintf("%f seconds", delay.Seconds()))
	return err
}

func (s *Store) FailDelivery(ctx context.Context, eventID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE delivery_events SET failed_at=now(),locked_until=NULL WHERE provider_event_id=$1`, eventID)
	return err
}

var _ = fmt.Sprint
var _ = time.Second
