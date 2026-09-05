package app

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/ghl-telnyx-integration/internal/workflow"
)

func TestHighLevelOutboundRejectsUnknownLocation(t *testing.T) {
	a := &App{HLSecret: "test-secret", LocationID: "loc-allowed", FromNumber: "+13125551212"}
	body, _ := json.Marshal(map[string]any{
		"contactId": "c1", "locationId": "loc-other", "messageId": "m1", "type": "SMS",
		"phone": "+13125551213", "message": "hello",
	})
	r := httptest.NewRequest("POST", "/webhooks/highlevel/outbound", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHighLevelOutboundRejectsAttachments(t *testing.T) {
	a := &App{HLSecret: "test-secret", FromNumber: "+131****1212"}
	body, _ := json.Marshal(map[string]any{
		"contactId": "c1", "locationId": "loc", "messageId": "m1", "type": "SMS",
		"phone": "+131****1213", "message": "hello", "attachments": []string{"https://example.test/a.png"},
	})
	r := httptest.NewRequest("POST", "/webhooks/highlevel/outbound", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestWorkflowEnrollRejectsUnknownLocationAndInvalidPhone(t *testing.T) {
	defs, err := workflow.EnabledCatalog([]string{"weekly-follow-up"})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{HLSecret: "test-secret", LocationID: "loc-allowed", Workflows: defs}
	body, _ := json.Marshal(map[string]any{
		"external_id": "e1", "location_id": "loc-other", "workflow_key": "weekly-follow-up",
		"contact_id": "c1", "to": "+131****1212", "from": "+131****1213",
		"consent_at": time.Now().UTC(), "consent_source": "test",
	})
	r := httptest.NewRequest("POST", "/workflows/enroll", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("unknown location status=%d body=%s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{
		"external_id": "e1", "location_id": "loc-allowed", "workflow_key": "weekly-follow-up",
		"contact_id": "c1", "to": "3125551212", "from": "+131****1213",
		"consent_at": time.Now().UTC(), "consent_source": "test",
	})
	r = httptest.NewRequest("POST", "/workflows/enroll", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-secret")
	w = httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid phone status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminStatusRequiresToken(t *testing.T) {
	a := &App{AdminToken: "admin-secret"}
	r := httptest.NewRequest("GET", "/admin/status", nil)
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestTelnyxRejectsDotSeparatedSignatures(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"data":{"id":"evt","event_type":"message.received","payload":{"id":"m","from":{"phone_number":"+13155551212"},"to":[{"phone_number":"+13155551213"}],"text":"hi"}}}`)
	ts := fmt.Sprint(time.Now().Unix())
	sig := ed25519.Sign(priv, []byte(ts+"."+string(body)))
	r := httptest.NewRequest("POST", "/webhooks/telnyx", bytes.NewReader(body))
	r.Header.Set("telnyx-timestamp", ts)
	r.Header.Set("telnyx-signature-ed25519", base64.StdEncoding.EncodeToString(sig))
	w := httptest.NewRecorder()
	(&App{WebhookKey: pub}).Routes().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestTelnyxIgnoresInboundForUnknownNumber(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"data":{"id":"evt","event_type":"message.received","payload":{"id":"m","from":{"phone_number":"+13155551212"},"to":[{"phone_number":"+19995550101"}],"text":"hi"}}}`)
	ts := fmt.Sprint(time.Now().Unix())
	sig := ed25519.Sign(priv, []byte(ts+"|"+string(body)))
	r := httptest.NewRequest("POST", "/webhooks/telnyx", bytes.NewReader(body))
	r.Header.Set("telnyx-timestamp", ts)
	r.Header.Set("telnyx-signature-ed25519", base64.StdEncoding.EncodeToString(sig))
	w := httptest.NewRecorder()
	(&App{WebhookKey: pub, FromNumber: "+13155551213"}).Routes().ServeHTTP(w, r)
	if w.Code != 202 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
