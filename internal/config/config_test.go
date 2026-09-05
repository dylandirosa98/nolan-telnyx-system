package config

import (
	"encoding/base64"
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

func TestLoadRequiresTelnyxSendConfigurationWhenSending(t *testing.T) {
	for _, key := range []string{"TELNYX_API_KEY", "TELNYX_MESSAGING_PROFILE_ID", "TELNYX_FROM_NUMBER"} {
		t.Run(key, func(t *testing.T) {
			setCompleteSendingEnvironment(t)
			t.Setenv(key, "")
			if _, err := Load(); err == nil {
				t.Fatalf("ENABLE_SENDING without %s should fail", key)
			}
		})
	}
}

func TestLoadRequiresHighLevelConnectionWhenSending(t *testing.T) {
	for _, key := range []string{"HIGHLEVEL_CLIENT_ID", "HIGHLEVEL_CLIENT_SECRET", "HIGHLEVEL_REDIRECT_URI", "HIGHLEVEL_LOCATION_ID", "HIGHLEVEL_CONVERSATION_PROVIDER_ID"} {
		t.Run(key, func(t *testing.T) {
			setCompleteSendingEnvironment(t)
			t.Setenv(key, "")
			if _, err := Load(); err == nil {
				t.Fatalf("ENABLE_SENDING without %s should fail", key)
			}
		})
	}
}

func setCompleteSendingEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ENABLE_SENDING", "true")
	t.Setenv("TELNYX_PUBLIC_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("TELNYX_API_KEY", "api-key")
	t.Setenv("TELNYX_MESSAGING_PROFILE_ID", "profile")
	t.Setenv("TELNYX_FROM_NUMBER", "+12025550101")
	t.Setenv("HIGHLEVEL_CLIENT_ID", "client-id")
	t.Setenv("HIGHLEVEL_CLIENT_SECRET", "client-secret")
	t.Setenv("HIGHLEVEL_REDIRECT_URI", "https://example.test/oauth/highlevel/callback")
	t.Setenv("HIGHLEVEL_LOCATION_ID", "location")
	t.Setenv("HIGHLEVEL_CONVERSATION_PROVIDER_ID", "provider")
}
