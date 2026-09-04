package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ DB *pgxpool.Pool }
type Outbound struct{ LocationID, ContactID, MessageID, To, From, Text string }

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
	_, e := s.DB.Exec(ctx, `UPDATE outbound_jobs SET status='sent',provider_message_id=$2,locked_until=NULL,updated_at=now() WHERE id=$1`, id, providerID)
	return e
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
func (s *Store) UpdateDelivery(ctx context.Context, eventID, providerID, status string) error {
	_, e := s.DB.Exec(ctx, `INSERT INTO delivery_events(provider_event_id,job_id,status) SELECT $1,id,$3 FROM outbound_jobs WHERE provider_message_id=$2 ON CONFLICT(provider_event_id) DO NOTHING`, eventID, providerID, status)
	return e
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
	return outbound, nil
}
func (s *Store) RecordInbound(ctx context.Context, eventID, from, to, body string) (bool, error) {
	result, err := s.DB.Exec(ctx, `INSERT INTO inbound_messages(provider_event_id,from_number,to_number,body) VALUES($1,$2,$3,$4) ON CONFLICT(provider_event_id) DO NOTHING`, eventID, from, to, body)
	return result.RowsAffected() == 1, err
}

var _ = fmt.Sprint
var _ = time.Second
