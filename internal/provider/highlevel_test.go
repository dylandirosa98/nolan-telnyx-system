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
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("Version") != "2021-07-28" {
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

func TestHighLevelClientPromotesUnknownSenderByCreatingContact(t *testing.T) {
	var contactBody, inbound map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/contacts/search/duplicate":
			_ = json.NewEncoder(w).Encode(map[string]any{"contact": map[string]string{}})
		case "/contacts/":
			_ = json.NewDecoder(r.Body).Decode(&contactBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"contact": map[string]string{"id": "new-contact"}})
		case "/conversations/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []any{}})
		case "/conversations/":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"conversation": map[string]string{"id": "new-conversation"}})
		case "/conversations/messages/inbound":
			_ = json.NewDecoder(r.Body).Decode(&inbound)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "primary", HTTP: server.Client()}
	err := client.PromoteInbound(context.Background(), Inbound{LocationID: "loc-2", From: "+13125551212", To: "+17405552852", Text: "hello", ProviderEventID: "event"})
	if err != nil {
		t.Fatal(err)
	}
	if contactBody["locationId"] != "loc-2" || contactBody["phone"] != "+13125551212" {
		t.Fatalf("contact=%#v", contactBody)
	}
	if inbound["contactId"] != "new-contact" || inbound["conversationId"] != "new-conversation" || inbound["message"] != "hello" {
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

func TestHighLevelClientExecutesCRMActions(t *testing.T) {
	var paths []string
	var task, note map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/contacts/contact/tasks":
			_ = json.NewDecoder(r.Body).Decode(&task)
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/contacts/contact/notes":
			_ = json.NewDecoder(r.Body).Decode(&note)
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/conversations/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []map[string]string{{"id": "conversation"}}})
		case r.URL.Path == "/conversations/conversation":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "loc", HTTP: server.Client()}
	if err := client.ExecuteCRM(context.Background(), CRMJob{Action: "create_task", Body: "14 day follow up", ContactID: "contact"}); err != nil {
		t.Fatal(err)
	}
	if task["title"] != "14 day follow up" || task["completed"] != false {
		t.Fatalf("task=%#v", task)
	}
	if err := client.ExecuteCRM(context.Background(), CRMJob{Action: "weekly_reply", ContactID: "contact", Reply: "yes"}); err != nil {
		t.Fatal(err)
	}
	if note["body"] == nil {
		t.Fatalf("note=%#v", note)
	}
	if err := client.ExecuteCRM(context.Background(), CRMJob{Action: "archive_conversation", ContactID: "contact", LocationID: "loc"}); err != nil {
		t.Fatal(err)
	}
	if len(paths) < 4 {
		t.Fatalf("paths=%v", paths)
	}
}

func TestHighLevelClientRefreshesUnauthorizedToken(t *testing.T) {
	var tokens []string
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens = append(tokens, r.Header.Get("Authorization"))
		if r.Method != http.MethodPut || r.URL.Path != "/conversations/messages/message/status" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := &HighLevelClient{BaseURL: server.URL, Tokens: refreshingToken{current: "old", next: "new"}, HTTP: server.Client()}
	if err := client.UpdateMessageStatus(context.Background(), "message", "delivered"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || tokens[0] != "Bearer old" || tokens[1] != "Bearer new" {
		t.Fatalf("attempts=%d tokens=%v", attempts, tokens)
	}
}

func TestHighLevelClientSearchContacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contacts/" || r.URL.Query().Get("query") != "pat" || r.URL.Query().Get("locationId") != "loc" {
			t.Fatalf("path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []map[string]string{{"id": "c1", "firstName": "Pat", "lastName": "Lee", "phone": "+13125551212"}}})
	}))
	defer server.Close()
	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "loc", HTTP: server.Client()}
	hits, err := client.SearchContacts(context.Background(), "pat")
	if err != nil || len(hits) != 1 || hits[0].ID != "c1" || hits[0].Phone != "+13125551212" || hits[0].Name != "Pat Lee" {
		t.Fatalf("hits=%#v err=%v", hits, err)
	}
}

func TestHighLevelClientReadsSMSFromNumberColumn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/locations/loc/customFields":
			_ = json.NewEncoder(w).Encode(map[string]any{"customFields": []map[string]string{{"id": "field-1", "name": "SMS From Number", "fieldKey": "contact.sms_from_number"}}})
		case "/contacts/contact-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"contact": map[string]any{"id": "contact-1", "customFields": []map[string]any{{"id": "field-1", "value": "8563057016"}}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "loc", HTTP: server.Client()}
	got, err := client.SendingNumber(context.Background(), "contact-1")
	if err != nil || got != "8563057016" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestHighLevelClientLoadsRecentSMSConversations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/conversations/search":
			if r.URL.Query().Get("locationId") != "loc-2" || r.URL.Query().Get("lastMessageType") != "TYPE_SMS" || r.URL.Query().Get("limit") != "20" {
				t.Fatalf("query=%s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"conversations": []map[string]any{{
					"id": "conversation-1", "contactId": "contact-1", "phone": "+13125551212",
					"lastMessageBody": "latest", "lastMessageDirection": "outbound", "lastMessageDate": "2026-10-04T12:00:00Z",
				}},
			})
		case "/conversations/conversation-1/messages":
			if r.URL.Query().Get("type") != "TYPE_SMS" || r.URL.Query().Get("limit") != "30" {
				t.Fatalf("message query=%s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": map[string]any{
					"messages": []map[string]any{{
						"id": "message-1", "altId": "provider-1", "body": "latest", "direction": "outbound",
						"from": "+17405552852", "to": "+13125551212", "dateAdded": "2026-10-04T12:00:00Z",
					}},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &HighLevelClient{BaseURL: server.URL, Token: "token", LocationID: "loc-1", HTTP: server.Client()}
	conversations, err := client.RecentConversations(context.Background(), "loc-2", 20)
	if err != nil || len(conversations) != 1 {
		t.Fatalf("conversations=%#v err=%v", conversations, err)
	}
	got := conversations[0]
	if got.ContactNumber != "+13125551212" || got.TelnyxNumber != "+17405552852" || got.LastBody != "latest" || got.LastDirection != "outbound" || got.MessageCount != 1 || got.LastAt == 0 {
		t.Fatalf("conversation=%#v", got)
	}
}

type refreshingToken struct{ current, next string }

func (r refreshingToken) Token(context.Context) (string, error) { return r.current, nil }
func (r refreshingToken) ForceRefresh(context.Context) (string, error) {
	return r.next, nil
}
