package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/ghl-telnyx-integration/internal/store"
	"github.com/jackc/pgx/v5"
)

const highLevelProvider = "highlevel"

type HighLevelTokenSource struct {
	Store      *store.Store
	OAuth      *OAuthClient
	LocationID string
	Fallback   string
	Now        func() time.Time
}

func (s *HighLevelTokenSource) Token(ctx context.Context) (string, error) {
	if s == nil {
		return "", fmt.Errorf("HighLevel token source is required")
	}
	now := s.now()
	token, err := s.Store.LoadOAuthToken(ctx, highLevelProvider, s.LocationID)
	if errors.Is(err, pgx.ErrNoRows) {
		if s.Fallback != "" {
			return s.Fallback, nil
		}
		return "", fmt.Errorf("HighLevel oauth token is missing")
	}
	if err != nil {
		return "", err
	}
	if token.ExpiresAt.After(now.Add(2 * time.Minute)) {
		return token.AccessToken, nil
	}
	return s.ForceRefresh(ctx)
}

func (s *HighLevelTokenSource) ForceRefresh(ctx context.Context) (string, error) {
	if s.OAuth == nil {
		if s.Fallback != "" {
			return s.Fallback, nil
		}
		return "", fmt.Errorf("HighLevel oauth client is not configured")
	}
	next, err := s.Store.ReplaceOAuthToken(ctx, highLevelProvider, s.LocationID, func(current store.OAuthToken) (store.OAuthToken, error) {
		refreshed, refreshErr := s.OAuth.Refresh(ctx, current.RefreshToken, current.UserType)
		if refreshErr != nil {
			return store.OAuthToken{}, refreshErr
		}
		locationID := refreshed.LocationID
		if locationID == "" {
			locationID = current.LocationID
		}
		return store.OAuthToken{
			Provider:     highLevelProvider,
			LocationID:   locationID,
			AccessToken:  refreshed.AccessToken,
			RefreshToken: refreshed.RefreshToken,
			TokenType:    refreshed.TokenType,
			UserType:     refreshed.UserType,
			Scope:        refreshed.Scope,
			ExpiresAt:    refreshed.ExpiresAt(s.now()),
		}, nil
	})
	if err != nil {
		if s.Fallback != "" && errors.Is(err, pgx.ErrNoRows) {
			return s.Fallback, nil
		}
		return "", err
	}
	return next.AccessToken, nil
}

func (s *HighLevelTokenSource) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
