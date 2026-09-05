package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOAuthClientExchangesAndRefreshes(t *testing.T) {
	var lastPath, lastBody, lastAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		lastAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		lastBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","userType":"Location","locationId":"loc","expires_in":3600}`))
	}))
	defer server.Close()

	client := &OAuthClient{BaseURL: server.URL, ClientID: "id", ClientSecret: "secret", RedirectURI: "https://example.test/callback", UserType: "Location", HTTP: server.Client()}
	token, err := client.Exchange(context.Background(), "code-1")
	if err != nil {
		t.Fatal(err)
	}
	if lastPath != "/oauth/token" || token.AccessToken != "access" || token.LocationID != "loc" {
		t.Fatalf("exchange path=%s token=%#v", lastPath, token)
	}
	if lastBody == "" || lastAuth != "" {
		t.Fatalf("exchange should be form-encoded without bearer auth: auth=%q body=%q", lastAuth, lastBody)
	}
	if !token.ExpiresAt(time.Unix(0, 0)).Equal(time.Unix(3600, 0)) {
		t.Fatalf("expires_at=%s", token.ExpiresAt(time.Unix(0, 0)))
	}

	token, err = client.Refresh(context.Background(), "refresh", "Location")
	if err != nil {
		t.Fatal(err)
	}
	if token.RefreshToken != "refresh" || lastPath != "/oauth/token" {
		t.Fatalf("refresh token=%#v path=%s", token, lastPath)
	}
}

func TestOAuthClientRejectsMissingAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"refresh_token":"only"}`))
	}))
	defer server.Close()
	client := &OAuthClient{BaseURL: server.URL, HTTP: server.Client()}
	if _, err := client.Exchange(context.Background(), "code"); err == nil {
		t.Fatal("missing access_token should fail")
	}
}
