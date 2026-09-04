package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTelnyxClientParsesProviderErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "loc:message" {
			t.Errorf("idempotency key=%q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":[{"code":"40300","detail":"Number is blocked"}]}`))
	}))
	defer server.Close()
	client := &TelnyxClient{BaseURL: server.URL, Token: "test", MessagingProfileID: "profile", HTTP: server.Client()}
	_, err := client.Send(context.Background(), SendRequest{To: "+13105551212", From: "+13105551213", Text: "hello", IdempotencyKey: "loc:message"})
	providerError, ok := err.(*Error)
	if !ok {
		t.Fatalf("error=%T %v", err, err)
	}
	if providerError.Status != http.StatusUnprocessableEntity || providerError.Code != "40300" || providerError.Message != "Number is blocked" {
		t.Fatalf("provider error=%#v", providerError)
	}
}

func TestTelnyxClientRejectsSuccessWithoutMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()
	client := &TelnyxClient{BaseURL: server.URL, HTTP: server.Client()}
	if _, err := client.Send(context.Background(), SendRequest{}); err == nil {
		t.Fatal("missing provider message ID should fail")
	}
}
