package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type QueueCounts struct {
	QueuedOutbound    int
	SendingOutbound   int
	FailedOutbound    int
	QueuedCRM         int
	FailedCRM         int
	Suppressions      int
	ActiveEnrollments int
	OAuthTokenPresent bool
	OAuthExpiresAt    time.Time
	OAuthHasExpiry    bool
}

func (s *Store) SetSendingPaused(ctx context.Context, paused bool) error {
	_, err := s.DB.Exec(ctx, `UPDATE settings SET sending_paused=$1,updated_at=now() WHERE id=1`, paused)
	return err
}

func (s *Store) RecordAdminAction(ctx context.Context, action string, detail map[string]any) error {
	payload, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO admin_audit(action,detail) VALUES($1,$2)`, action, payload)
	return err
}

func (s *Store) QueueCounts(ctx context.Context, locationID string) (QueueCounts, error) {
	var counts QueueCounts
	err := s.DB.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM outbound_jobs WHERE status='queued'),
			(SELECT count(*) FROM outbound_jobs WHERE status='sending'),
			(SELECT count(*) FROM outbound_jobs WHERE status='failed'),
			(SELECT count(*) FROM crm_jobs WHERE status IN ('queued','running')),
			(SELECT count(*) FROM crm_jobs WHERE status='failed'),
			(SELECT count(*) FROM suppressions),
			(SELECT count(*) FROM workflow_enrollments WHERE state IN ('pending','awaiting_reply'))`).Scan(
		&counts.QueuedOutbound, &counts.SendingOutbound, &counts.FailedOutbound,
		&counts.QueuedCRM, &counts.FailedCRM, &counts.Suppressions, &counts.ActiveEnrollments)
	if err != nil {
		return QueueCounts{}, err
	}
	if locationID == "" {
		return counts, nil
	}
	var expiresAt *time.Time
	err = s.DB.QueryRow(ctx, `SELECT expires_at FROM oauth_tokens WHERE provider='highlevel' AND location_id=$1`, locationID).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return counts, nil
	}
	if err != nil {
		return QueueCounts{}, err
	}
	counts.OAuthTokenPresent = true
	if expiresAt != nil {
		counts.OAuthExpiresAt = *expiresAt
		counts.OAuthHasExpiry = true
	}
	return counts, nil
}
