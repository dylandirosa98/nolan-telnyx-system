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
		EnableSending: c.EnableSending, Workflows: workflows, Logger: slog.Default(),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go a.RunWorkflowWorker(ctx)
	go a.RunCRMWorker(ctx)
	go a.RunInboundWorker(ctx)
	a.RunWorker(ctx)
}
