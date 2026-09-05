package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"example.com/ghl-telnyx-integration/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isolatedStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("audit_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		root.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); _, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db}
}

func TestConcurrentClaimsAndDuplicateEnqueue(t *testing.T) {
	s := isolatedStore(t)
	ctx := context.Background()
	const n = 100
	for i := 0; i < n; i++ {
		o := Outbound{LocationID: "test", MessageID: fmt.Sprint(i), To: "+12025550101", From: "+12025550102", Text: "test"}
		if created, err := s.Enqueue(ctx, o); err != nil || !created {
			t.Fatalf("enqueue: %v %v", created, err)
		}
		if created, err := s.Enqueue(ctx, o); err != nil || created {
			t.Fatalf("duplicate: %v %v", created, err)
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[int64]bool{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				j, err := s.Claim(ctx)
				if err == pgx.ErrNoRows {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if seen[j.ID] {
					t.Errorf("duplicate claim %d", j.ID)
				}
				seen[j.ID] = true
				mu.Unlock()
				if err = s.Complete(ctx, j.ID, fmt.Sprintf("provider-%d", j.ID)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("claimed %d want %d", len(seen), n)
	}
}

func TestSuppressionAndLeaseRecovery(t *testing.T) {
	s := isolatedStore(t)
	ctx := context.Background()
	o := Outbound{LocationID: "test", MessageID: "one", To: "+12025550101", From: "+12025550102", Text: "test"}
	if _, err := s.Enqueue(ctx, o); err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE outbound_jobs SET locked_until=now()-interval '1 second' WHERE id=$1", j.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Claim(ctx)
	if err != nil || recovered.ID != j.ID || recovered.Attempts != 2 {
		t.Fatalf("reclaim: %+v %v", recovered, err)
	}
	if err = s.Retry(ctx, j.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Suppress(ctx, o.To, "test", "stop"); err != nil {
		t.Fatal(err)
	}
	if blocked, err := s.IsSuppressed(ctx, o.To); err != nil || !blocked {
		t.Fatalf("suppression %v %v", blocked, err)
	}
	if _, err = s.Claim(ctx); err != pgx.ErrNoRows {
		t.Fatalf("suppressed job claim: %v", err)
	}
}

func TestDeliveryBeforeSendCompletionAndInboundDedup(t *testing.T) {
	s := isolatedStore(t)
	ctx := context.Background()
	if created, err := s.UpdateDelivery(ctx, "event", "provider", "delivered"); err != nil || !created {
		t.Fatalf("delivery %v %v", created, err)
	}
	if created, err := s.UpdateDelivery(ctx, "event", "provider", "pending"); err != nil || created {
		t.Fatalf("duplicate delivery %v %v", created, err)
	}
	if _, err := s.Enqueue(ctx, Outbound{LocationID: "test", MessageID: "message", Text: "test"}); err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Complete(ctx, j.ID, "provider"); err != nil {
		t.Fatal(err)
	}
	var id int64
	var status string
	if err = s.DB.QueryRow(ctx, "SELECT job_id,status FROM delivery_events WHERE provider_event_id='event'").Scan(&id, &status); err != nil || id != j.ID || status != "delivered" {
		t.Fatalf("correlation %d %s %v", id, status, err)
	}
	if err = s.RetryDelivery(ctx, "event", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimUnprocessedDelivery(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.MarkDeliverySynced(ctx, "event"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimUnprocessedDelivery(ctx); err != pgx.ErrNoRows {
		t.Fatalf("synced event reclaimed: %v", err)
	}
	if created, err := s.RecordInbound(ctx, "in", "+12025550101", "+12025550102", "hello"); err != nil || !created {
		t.Fatalf("inbound %v %v", created, err)
	}
	if created, err := s.RecordInbound(ctx, "in", "+12025550101", "+12025550102", "hello"); err != nil || created {
		t.Fatalf("duplicate inbound %v %v", created, err)
	}
	if _, err = s.ClaimUnprocessedInbound(ctx); err != pgx.ErrNoRows {
		t.Fatalf("leased inbound reclaimed: %v", err)
	}
	if err = s.RetryInbound(ctx, "in", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimUnprocessedInbound(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.MarkInboundProcessed(ctx, "in"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimUnprocessedInbound(ctx); err != pgx.ErrNoRows {
		t.Fatalf("processed inbound reclaimed: %v", err)
	}
}

func TestOAuthStateSingleUseAndExpiry(t *testing.T) {
	s := isolatedStore(t)
	ctx := context.Background()
	state, err := s.CreateOAuthState(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConsumeOAuthState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if err = s.ConsumeOAuthState(ctx, state); err == nil {
		t.Fatal("state reused")
	}
	state, err = s.CreateOAuthState(ctx, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ConsumeOAuthState(ctx, state); err == nil {
		t.Fatal("expired state accepted")
	}
}

func TestInboundAndDeliveryDeadLetterLifecycle(t *testing.T) {
	s := isolatedStore(t)
	ctx := context.Background()
	if _, err := s.RecordInbound(ctx, "dead-inbound", "+12025550101", "+12025550102", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryInbound(ctx, "dead-inbound", 0); err != nil {
		t.Fatal(err)
	}
	inbound, err := s.ClaimUnprocessedInbound(ctx)
	if err != nil || inbound.Attempts != 1 {
		t.Fatalf("inbound=%+v err=%v", inbound, err)
	}
	if err = s.FailInbound(ctx, inbound.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimUnprocessedInbound(ctx); err != pgx.ErrNoRows {
		t.Fatalf("dead inbound reclaimed: %v", err)
	}

	if _, err = s.UpdateDelivery(ctx, "dead-delivery", "provider-dead", "failed"); err != nil {
		t.Fatal(err)
	}
	if err = s.RetryDelivery(ctx, "dead-delivery", 0); err != nil {
		t.Fatal(err)
	}
	delivery, err := s.ClaimUnprocessedDelivery(ctx)
	if err != nil || delivery.Attempts != 1 {
		t.Fatalf("delivery=%+v err=%v", delivery, err)
	}
	if err = s.FailDelivery(ctx, delivery.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimUnprocessedDelivery(ctx); err != pgx.ErrNoRows {
		t.Fatalf("dead delivery reclaimed: %v", err)
	}
	counts, err := s.QueueCounts(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if counts.FailedInbound != 1 || counts.FailedDelivery != 1 {
		t.Fatalf("dead-letter counts=%+v", counts)
	}
}
