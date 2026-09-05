package store

import (
	"context"
	"fmt"
	"time"
)

type OAuthToken struct {
	Provider     string
	LocationID   string
	AccessToken  string
	RefreshToken string
	TokenType    string
	UserType     string
	Scope        string
	ExpiresAt    time.Time
}

func (s *Store) SaveOAuthToken(ctx context.Context, token OAuthToken) error {
	if token.Provider == "" || token.LocationID == "" || token.AccessToken == "" {
		return fmt.Errorf("oauth token provider, location, and access token are required")
	}
	_, err := s.DB.Exec(ctx, `
		INSERT INTO oauth_tokens(provider,location_id,access_token,refresh_token,token_type,user_type,scope,expires_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())
		ON CONFLICT(provider,location_id) DO UPDATE SET
			access_token=EXCLUDED.access_token,
			refresh_token=CASE WHEN EXCLUDED.refresh_token='' THEN oauth_tokens.refresh_token ELSE EXCLUDED.refresh_token END,
			token_type=EXCLUDED.token_type,
			user_type=EXCLUDED.user_type,
			scope=EXCLUDED.scope,
			expires_at=EXCLUDED.expires_at,
			updated_at=now()`,
		token.Provider, token.LocationID, token.AccessToken, token.RefreshToken, valueOrBearer(token.TokenType),
		token.UserType, token.Scope, token.ExpiresAt)
	return err
}

func (s *Store) LoadOAuthToken(ctx context.Context, providerName, locationID string) (OAuthToken, error) {
	var token OAuthToken
	err := s.DB.QueryRow(ctx, `
		SELECT provider,location_id,access_token,refresh_token,token_type,user_type,scope,expires_at
		FROM oauth_tokens WHERE provider=$1 AND location_id=$2`, providerName, locationID).Scan(
		&token.Provider, &token.LocationID, &token.AccessToken, &token.RefreshToken, &token.TokenType,
		&token.UserType, &token.Scope, &token.ExpiresAt)
	return token, err
}

func (s *Store) ReplaceOAuthToken(ctx context.Context, providerName, locationID string, refresh func(OAuthToken) (OAuthToken, error)) (OAuthToken, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return OAuthToken{}, err
	}
	defer tx.Rollback(ctx)

	var current OAuthToken
	err = tx.QueryRow(ctx, `
		SELECT provider,location_id,access_token,refresh_token,token_type,user_type,scope,expires_at
		FROM oauth_tokens WHERE provider=$1 AND location_id=$2 FOR UPDATE`, providerName, locationID).Scan(
		&current.Provider, &current.LocationID, &current.AccessToken, &current.RefreshToken, &current.TokenType,
		&current.UserType, &current.Scope, &current.ExpiresAt)
	if err != nil {
		return OAuthToken{}, err
	}
	next, err := refresh(current)
	if err != nil {
		return OAuthToken{}, err
	}
	if next.AccessToken == "" {
		return OAuthToken{}, fmt.Errorf("oauth refresh returned an empty access token")
	}
	if next.RefreshToken == "" {
		next.RefreshToken = current.RefreshToken
	}
	if next.LocationID == "" {
		next.LocationID = current.LocationID
	}
	if next.Provider == "" {
		next.Provider = current.Provider
	}
	_, err = tx.Exec(ctx, `
		UPDATE oauth_tokens SET access_token=$3,refresh_token=$4,token_type=$5,user_type=$6,scope=$7,expires_at=$8,updated_at=now()
		WHERE provider=$1 AND location_id=$2`,
		next.Provider, next.LocationID, next.AccessToken, next.RefreshToken, valueOrBearer(next.TokenType),
		next.UserType, next.Scope, next.ExpiresAt)
	if err != nil {
		return OAuthToken{}, err
	}
	return next, tx.Commit(ctx)
}

func valueOrBearer(tokenType string) string {
	if tokenType == "" {
		return "Bearer"
	}
	return tokenType
}
