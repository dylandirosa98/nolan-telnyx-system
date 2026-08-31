package webhook

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerifyTelnyxSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	body := []byte(`{"data":{"id":"evt-1"}}`)
	now := time.Unix(1700000000, 0)
	ts := "1700000000"
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("telnyx-timestamp", ts)
	req.Header.Set("telnyx-signature-ed25519", base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(ts+"."), body...))))
	if err := VerifyTelnyx(req, body, pub, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	req.Header.Set("telnyx-signature-ed25519", "bad")
	if VerifyTelnyx(req, body, pub, now, time.Minute) == nil {
		t.Fatal("bad signature accepted")
	}
}
