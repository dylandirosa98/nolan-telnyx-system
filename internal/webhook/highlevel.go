package webhook

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
)

func VerifyHighLevel(r *http.Request, body []byte, key ed25519.PublicKey) error {
	encoded := r.Header.Get("X-GHL-Signature")
	if encoded == "" || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("missing webhook authentication")
	}
	signature, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !ed25519.Verify(key, body, signature) {
		return fmt.Errorf("invalid webhook signature")
	}
	return nil
}

func OfficialHighLevelPublicKey() (ed25519.PublicKey, error) {
	const encodedDER = "MCowBQYDK2VwAyEAi2HR1srL4o18O8BRa7gVJY7G7bupbN3H9AwJrHCDiOg="
	der, err := base64.StdEncoding.DecodeString(encodedDER)
	if err != nil {
		return nil, err
	}
	// Ed25519 SubjectPublicKeyInfo is a 12-byte ASN.1 prefix followed by the 32-byte key.
	if len(der) != 44 {
		return nil, fmt.Errorf("unexpected HighLevel public key length")
	}
	key := append(ed25519.PublicKey(nil), der[len(der)-ed25519.PublicKeySize:]...)
	return key, nil
}
