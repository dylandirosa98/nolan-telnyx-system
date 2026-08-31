package app

import (
	"bytes"
	"context"
	"encoding/json"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestFakeLocalEndToEnd(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL for compose-backed E2E")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := &store.Store{DB: db}
	_, _ = db.Exec(ctx, `UPDATE settings SET sending_paused=false WHERE id=1`)
	fake := &provider.FakeTelnyx{}
	a := &App{Store: s, Telnyx: fake, EnableSending: true}
	id := "e2e-" + time.Now().Format("150405.000000000")
	b, _ := json.Marshal(map[string]string{"location_id": "loc-e2e", "message_id": id, "to": "+13125551212", "from": "+13125551213", "text": "hello"})
	r := httptest.NewRequest("POST", "/webhooks/highlevel/outbound", bytes.NewReader(b))
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 202 {
		t.Fatalf("enqueue status %d", w.Code)
	}
	run, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go a.RunWorker(run)
	<-run.Done()
	fake.Mu.Lock()
	defer fake.Mu.Unlock()
	if len(fake.Sent) != 1 {
		t.Fatalf("sent %d messages", len(fake.Sent))
	}
}
