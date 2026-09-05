package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOAuthCallbackAcceptsSingleUseState(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	st := &store.Store{DB: db}
	state, err := st.CreateOAuthState(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		if values.Get("client_id") != "client-id" || values.Get("client_secret") != "client-secret" || values.Get("code") != "install-code" {
			t.Fatalf("unexpected oauth form keys")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"token_type":    "Bearer",
			"userType":      "Location",
			"locationId":    "oauth-test-location",
			"scope":         "contacts.readonly",
			"expires_in":    3600,
		})
	}))
	defer oauthServer.Close()

	application := &App{
		Store: st,
		OAuth: &provider.OAuthClient{
			BaseURL:      oauthServer.URL,
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RedirectURI:  "https://example.test/oauth/highlevel/callback",
			UserType:     "Location",
			HTTP:         oauthServer.Client(),
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/oauth/highlevel/callback?code=install-code&state="+state, nil)
	response := httptest.NewRecorder()
	application.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"connected"`) {
		t.Fatalf("body=%s", response.Body.String())
	}
	stored, err := st.LoadOAuthToken(ctx, "highlevel", "oauth-test-location")
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != "access-token" || stored.RefreshToken != "refresh-token" {
		t.Fatal("oauth tokens were not stored")
	}

	reused := httptest.NewRecorder()
	application.Routes().ServeHTTP(reused, httptest.NewRequest(http.MethodGet, "/oauth/highlevel/callback?code=install-code&state="+state, nil))
	if reused.Code != http.StatusUnauthorized {
		t.Fatalf("reused state status=%d body=%s", reused.Code, reused.Body.String())
	}
}

func TestOAuthCallbackRejectsMissingOrInvalidState(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	application := &App{
		Store: &store.Store{DB: db},
		OAuth: &provider.OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
	}
	for name, target := range map[string]string{
		"missing": "/oauth/highlevel/callback?code=code",
		"invalid": "/oauth/highlevel/callback?code=code&state=" + strings.Repeat("0", 64),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			application.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			if response.Code != http.StatusUnauthorized && response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
