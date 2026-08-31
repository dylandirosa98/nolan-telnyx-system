package main

import (
	"context"
	"example.com/ghl-telnyx-integration/internal/app"
	"example.com/ghl-telnyx-integration/internal/config"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
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
	a := &app.App{Store: &store.Store{DB: db}, Telnyx: &provider.TelnyxClient{BaseURL: c.TelnyxBaseURL, Token: c.TelnyxToken, MessagingProfileID: c.TelnyxProfileID, HTTP: &http.Client{Timeout: 15 * time.Second}}, EnableSending: c.EnableSending}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	a.RunWorker(ctx)
}
