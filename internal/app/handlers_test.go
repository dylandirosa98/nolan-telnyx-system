package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/workflow"
)

func TestSignalDeskConversationsRequireTokenAndAllowedLocation(t *testing.T) {
	conversation := provider.Conversation{ContactNumber: "+13125551212", LastBody: "hello", LastDirection: "inbound", LastAt: 1}
	a := &App{
		SignalDeskToken:    "desk-secret",
		AllowedLocationIDs: []string{"loc-allowed"},
		HighLevel:          &provider.FakeHighLevel{Conversations: []provider.Conversation{conversation}},
	}

	unauthorized := httptest.NewRecorder()
	a.Routes().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/signal-desk/conversations?location_id=loc-allowed", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/signal-desk/conversations?location_id=loc-allowed", nil)
	request.Header.Set("Authorization", "Bearer desk-secret")
	response := httptest.NewRecorder()
	a.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "+13125551212") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	blocked := httptest.NewRequest(http.MethodGet, "/signal-desk/conversations?location_id=other", nil)
	blocked.Header.Set("Authorization", "Bearer desk-secret")
	blockedResponse := httptest.NewRecorder()
	a.Routes().ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("blocked status=%d", blockedResponse.Code)
	}
}

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

func TestAllowedLocationIncludesNolanSMS3(t *testing.T) {
	a := &App{LocationID: "sJrqUGJbC5EwZx12tvwG", AllowedLocationIDs: []string{"sJrqUGJbC5EwZx12tvwG", "93SBBPzXyG32eVt5BD7D"}}
	if !a.allowsLocation(context.Background(), "93SBBPzXyG32eVt5BD7D") || !a.allowsLocation(context.Background(), "sJrqUGJbC5EwZx12tvwG") {
		t.Fatal("approved locations were rejected")
	}
	if a.allowsLocation(context.Background(), "other") {
		t.Fatal("unknown location was accepted")
	}
}

func TestResolveSendingNumberUsesCSVColumn(t *testing.T) {
	a := &App{
		HighLevel:  &provider.FakeHighLevel{FromNumbers: map[string]string{"c1": "8563057016"}},
		Telnyx:     &provider.FakeTelnyx{Owned: []string{"+18563057016", "+17405552852"}},
		FromNumber: "+17405552852",
	}
	got, err := a.resolveSendingNumber(context.Background(), "loc", "c1", a.FromNumber)
	if err != nil || got != "+18563057016" {
		t.Fatalf("got %s err %v", got, err)
	}
	got, err = a.resolveSendingNumber(context.Background(), "loc", "missing", a.FromNumber)
	if err != nil || got != "+17405552852" {
		t.Fatalf("blank column got %s err %v", got, err)
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
