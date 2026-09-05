package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/ghl-telnyx-integration/internal/app"
	"example.com/ghl-telnyx-integration/internal/config"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/internal/webhook"
	"example.com/ghl-telnyx-integration/internal/workflow"
	"example.com/ghl-telnyx-integration/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	c, e := config.Load()
	if e != nil {
		slog.Error("configuration", "error", e)
		os.Exit(1)
	}
	db, e := pgxpool.New(context.Background(), c.DatabaseURL)
	if e != nil {
		os.Exit(1)
	}
	defer db.Close()
	if e = migrations.Apply(context.Background(), db); e != nil {
		slog.Error("database migrations", "error", e)
		os.Exit(1)
	}
	workflows, e := workflow.EnabledCatalog(c.EnabledWorkflowKeys)
	if e != nil {
		slog.Error("workflow configuration", "error", e)
		os.Exit(1)
	}
	highLevelWebhookKey, e := webhook.OfficialHighLevelPublicKey()
	if e != nil {
		slog.Error("HighLevel webhook key", "error", e)
		os.Exit(1)
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	st := &store.Store{DB: db}
	oauth := &provider.OAuthClient{
		BaseURL: c.HighLevelBaseURL, ClientID: c.HighLevelClientID, ClientSecret: c.HighLevelClientSecret,
		RedirectURI: c.HighLevelRedirectURI, UserType: c.HighLevelUserType, HTTP: httpClient,
	}
	a := &app.App{
		Store:  st,
		Telnyx: &provider.TelnyxClient{BaseURL: c.TelnyxBaseURL, Token: c.TelnyxToken, MessagingProfileID: c.TelnyxProfileID, HTTP: httpClient},
		HighLevel: &provider.HighLevelClient{
			BaseURL: c.HighLevelBaseURL, Token: c.HighLevelToken,
			Tokens:     &provider.HighLevelTokenSource{Store: st, OAuth: oauth, LocationID: c.HighLevelLocationID, Fallback: c.HighLevelToken},
			LocationID: c.HighLevelLocationID, ConversationProviderID: c.HighLevelConversationProviderID, HTTP: httpClient,
		},
		OAuth: oauth, WebhookKey: c.WebhookKey, HighLevelWebhookKey: highLevelWebhookKey,
		HLSecret: c.HighLevelWebhookSecret, AdminToken: c.AdminToken, LocationID: c.HighLevelLocationID,
		FromNumber: c.FromNumber, EnableSending: c.EnableSending, Workflows: workflows, Logger: slog.Default(),
	}
	srv := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: a.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		x, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(x)
	}()
	slog.Info("api listening", "addr", srv.Addr)
	if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		os.Exit(1)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
