package config

import (
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing DATABASE_URL should fail")
	}
}

func TestLoadRequiresTelnyxPublicKeyWhenSending(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ENABLE_SENDING", "true")
	t.Setenv("TELNYX_PUBLIC_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("ENABLE_SENDING without TELNYX_PUBLIC_KEY should fail")
	}
}
