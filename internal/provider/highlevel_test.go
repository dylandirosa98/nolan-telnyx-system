package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHighLevelClientForwardsInboundIntoConversation(t *testing.T) {
	var inbound map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("Version") != "v3" {
			t.Fatalf("missing HighLevel authentication headers")
		}
		switch r.URL.Path {
		case "/contacts/search/duplicate":
			if r.URL.Query().Get("locationId") != "loc" || r.URL.Query().Get("number") != "+13125551212" {
				t.Fatalf("contact query=%s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"contact": map[string]string{"id": "contact"}})
		case "/conversations/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []map[string]string{{"id": "conversation"}}, "total": 1})
		case "/conversations/messages/inbound":
			if r.Method != http.MethodPost {
				t.Fatalf("method=%s", r.Method)
			}
			_ = json.NewDecoder(r.Body).Decode(&inbound)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "loc", ConversationProviderID: "provider", HTTP: server.Client()}
	err := client.ForwardInbound(context.Background(), Inbound{From: "+13125551212", To: "+13125551213", Text: "hello", ProviderEventID: "event"})
	if err != nil {
		t.Fatal(err)
	}
	if inbound["type"] != "SMS" || inbound["message"] != "hello" || inbound["contactId"] != "contact" || inbound["conversationId"] != "conversation" || inbound["conversationProviderId"] != "provider" || inbound["altId"] != "event" {
		t.Fatalf("inbound=%#v", inbound)
	}
}

func TestHighLevelClientSetsSMSDND(t *testing.T) {
	var update map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/contacts/search/duplicate":
			_ = json.NewEncoder(w).Encode(map[string]any{"contact": map[string]string{"id": "contact"}})
		case "/contacts/contact":
			if r.Method != http.MethodPut {
				t.Fatalf("method=%s", r.Method)
			}
			_ = json.NewDecoder(r.Body).Decode(&update)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "loc", HTTP: server.Client()}
	if err := client.SetSMSDND(context.Background(), "+13125551212"); err != nil {
		t.Fatal(err)
	}
	settings := update["dndSettings"].(map[string]any)
	sms := settings["sms"].(map[string]any)
	if sms["status"] != "active" || sms["code"] != "OPTED_OUT" {
		t.Fatalf("update=%#v", update)
	}
}

func TestHighLevelClientUpdatesMessageStatus(t *testing.T) {
	var update map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/conversations/messages/message/status" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&update)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := &HighLevelClient{BaseURL: server.URL, Token: "token", HTTP: server.Client()}
	if err := client.UpdateMessageStatus(context.Background(), "message", "delivered"); err != nil {
		t.Fatal(err)
	}
	if update["status"] != "delivered" {
		t.Fatalf("update=%#v", update)
	}
	if err := client.UpdateMessageStatus(context.Background(), "message", "unknown"); err == nil {
		t.Fatal("unknown status should fail")
	}
}
