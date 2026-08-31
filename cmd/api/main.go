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
	a := &app.App{Store: &store.Store{DB: db}, Telnyx: &provider.TelnyxClient{BaseURL: c.TelnyxBaseURL, Token: c.TelnyxToken, MessagingProfileID: c.TelnyxProfileID, HTTP: &http.Client{Timeout: 15 * time.Second}}, WebhookKey: c.WebhookKey, HLSecret: c.HighLevelToken, EnableSending: c.EnableSending, Logger: slog.Default()}
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
