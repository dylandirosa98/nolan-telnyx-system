package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func (s *Store) CreateOAuthState(ctx context.Context, ttl time.Duration) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	state := hex.EncodeToString(raw[:])
	_, err := s.DB.Exec(ctx, `INSERT INTO oauth_states(state,expires_at) VALUES($1,now()+$2::interval)`, state, fmt.Sprintf("%f seconds", ttl.Seconds()))
	return state, err
}

func (s *Store) ConsumeOAuthState(ctx context.Context, state string) error {
	result, err := s.DB.Exec(ctx, `DELETE FROM oauth_states WHERE state=$1 AND expires_at>now()`, state)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("invalid or expired oauth state")
	}
	return nil
}
