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
	a := &App{Store: s, Telnyx: fake, HLSecret: "test-secret", FromNumber: "+13105551213", EnableSending: true}
	id := "e2e-" + time.Now().Format("150405.000000000")
	b, _ := json.Marshal(map[string]string{"contactId": "contact-e2e", "locationId": "loc-e2e", "messageId": id, "type": "SMS", "phone": "+13105551212", "message": "hello"})
	r := httptest.NewRequest("POST", "/webhooks/highlevel/outbound", bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 200 {
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

func TestDisabledWorkerWaitsForShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		(&App{}).RunWorker(ctx)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("disabled worker exited before shutdown")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disabled worker did not stop after cancellation")
	}
}

func TestHighLevelDeliveryStatus(t *testing.T) {
	recipients := []struct {
		PhoneNumber string `json:"phone_number"`
		Status      string `json:"status"`
	}{{PhoneNumber: "+131****1212", Status: "delivered"}}
	if got := highLevelDeliveryStatus("message.sent", recipients); got != "pending" {
		t.Fatalf("sent status=%s", got)
	}
	if got := highLevelDeliveryStatus("message.finalized", recipients); got != "delivered" {
		t.Fatalf("finalized status=%s", got)
	}
	recipients[0].Status = "delivery_failed"
	if got := highLevelDeliveryStatus("message.finalized", recipients); got != "failed" {
		t.Fatalf("failed status=%s", got)
	}
}
