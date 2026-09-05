package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkflowLocalEndToEnd(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL for compose-backed E2E")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &store.Store{DB: db}
	_, _ = db.Exec(ctx, `UPDATE settings SET sending_paused=false WHERE id=1`)
	definitions, err := workflow.EnabledCatalog([]string{"weekly-follow-up"})
	if err != nil {
		t.Fatal(err)
	}
	// This test exercises queue transport, not the wall-clock sending window.
	// Quiet-hour boundaries are exercised with fixed times in workflow unit tests.
	definition := definitions["weekly-follow-up"]
	definition.Window = workflow.SendWindow{StartMinute: 0, EndMinute: 24 * 60}
	definitions["weekly-follow-up"] = definition
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	fake := &provider.FakeTelnyx{}
	application := &App{Store: store, Telnyx: fake, WebhookKey: publicKey, HLSecret: "test-secret", EnableSending: true, Workflows: definitions}

	stamp := time.Now().UTC()
	externalID := "workflow-e2e-" + stamp.Format("150405.000000000")
	body, _ := json.Marshal(map[string]any{
		"external_id":    externalID,
		"location_id":    "loc-e2e",
		"workflow_key":   "weekly-follow-up",
		"contact_id":     externalID,
		"to":             "+13105551212",
		"from":           "+13105551213",
		"timezone":       "Asia/Tokyo",
		"consent_at":     stamp.Add(-time.Hour),
		"consent_source": "postcard_keyword_reply",
		"variables":      map[string]string{"contact.first_name": "Sam", "contact.full_address": "123 Main St", "location.name": "Example Company"},
	})
	request := httptest.NewRequest("POST", "/workflows/enroll", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-secret")
	response := httptest.NewRecorder()
	application.Routes().ServeHTTP(response, request)
	if response.Code != 202 {
		t.Fatalf("enroll status %d: %s", response.Code, response.Body.String())
	}
	if _, err = db.Exec(ctx, `UPDATE workflow_enrollments SET next_run_at=now() WHERE external_id=$1`, externalID); err != nil {
		t.Fatal(err)
	}

	run, cancel := context.WithCancel(ctx)
	defer cancel()
	go application.RunWorkflowWorker(run)
	go application.RunWorker(run)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fake.Mu.Lock()
		sentCount := len(fake.Sent)
		fake.Mu.Unlock()
		if sentCount == 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	fake.Mu.Lock()
	sent := append([]provider.SendRequest(nil), fake.Sent...)
	fake.Mu.Unlock()
	if len(sent) != 1 || sent[0].Text != "Hi, this is Example Company following up about 123 Main St. Has anything changed? Reply STOP to opt out." {
		t.Fatalf("sent=%#v", sent)
	}

	eventID := "reply-" + externalID
	webhookBody, _ := json.Marshal(map[string]any{"data": map[string]any{
		"id":         eventID,
		"event_type": "message.received",
		"payload": map[string]any{
			"id":   "provider-inbound-" + externalID,
			"from": map[string]string{"phone_number": "+13105551212"},
			"to":   []map[string]string{{"phone_number": "+13105551213"}},
			"text": "yes, call me",
		},
	}})
	timestamp := fmt.Sprint(time.Now().Unix())
	signature := ed25519.Sign(privateKey, []byte(timestamp+"|"+string(webhookBody)))
	request = httptest.NewRequest("POST", "/webhooks/telnyx", bytes.NewReader(webhookBody))
	request.Header.Set("telnyx-timestamp", timestamp)
	request.Header.Set("telnyx-signature-ed25519", base64.StdEncoding.EncodeToString(signature))
	response = httptest.NewRecorder()
	application.Routes().ServeHTTP(response, request)
	if response.Code != 202 {
		t.Fatalf("reply status %d: %s", response.Code, response.Body.String())
	}
	var state string
	if err = db.QueryRow(ctx, `SELECT state FROM workflow_enrollments WHERE external_id=$1`, externalID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != string(workflow.StateCompleted) {
		t.Fatalf("state=%q", state)
	}
}
