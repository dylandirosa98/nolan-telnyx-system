package app

import (
	"crypto/subtle"
	"strings"
)

func bearerSecretOK(header, secret string) bool {
	if secret == "" {
		return false
	}
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	got := []byte(token)
	want := []byte(secret)
	if len(got) != len(want) {
		subtle.ConstantTimeCompare(want, want)
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
