package webhook

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestVerifyHighLevel(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"messageId":"message-1"}`)
	request := httptest.NewRequest("POST", "/webhooks/highlevel/outbound", nil)
	request.Header.Set("X-GHL-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, body)))
	if err = VerifyHighLevel(request, body, publicKey); err != nil {
		t.Fatal(err)
	}
	if err = VerifyHighLevel(request, append(body, ' '), publicKey); err == nil {
		t.Fatal("modified body should fail verification")
	}
}

func TestOfficialHighLevelPublicKey(t *testing.T) {
	key, err := OfficialHighLevelPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != ed25519.PublicKeySize {
		t.Fatalf("key length=%d", len(key))
	}
}
