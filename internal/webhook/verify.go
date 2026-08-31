package webhook

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func VerifyTelnyx(r *http.Request, body []byte, key ed25519.PublicKey, now time.Time, maxAge time.Duration) error {
	sig := r.Header.Get("telnyx-signature-ed25519")
	ts := r.Header.Get("telnyx-timestamp")
	if sig == "" || ts == "" || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("missing webhook authentication")
	}
	stamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || now.Sub(time.Unix(stamp, 0)) > maxAge || now.Before(time.Unix(stamp, 0).Add(-maxAge)) {
		return fmt.Errorf("stale webhook")
	}
	s, err := base64.StdEncoding.DecodeString(sig)
	if err != nil || !ed25519.Verify(key, []byte(ts+"."+string(body)), s) {
		return fmt.Errorf("invalid webhook signature")
	}
	return nil
}

func HeaderValues(r *http.Request) string {
	return strings.Join([]string{r.Header.Get("telnyx-signature-ed25519"), r.Header.Get("telnyx-timestamp")}, ",")
}
