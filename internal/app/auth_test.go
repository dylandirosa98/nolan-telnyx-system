package app

import "testing"

func TestBearerSecretOK(t *testing.T) {
	if bearerSecretOK("Bearer secret", "secret") != true {
		t.Fatal("matching bearer token should pass")
	}
	if bearerSecretOK("Bearer secretx", "secret") {
		t.Fatal("different length should fail")
	}
	if bearerSecretOK("Bearer wrong", "secret") {
		t.Fatal("wrong token should fail")
	}
	if bearerSecretOK("secret", "secret") {
		t.Fatal("missing Bearer prefix should fail")
	}
	if bearerSecretOK("Bearer secret", "") {
		t.Fatal("empty secret should fail closed")
	}
}
