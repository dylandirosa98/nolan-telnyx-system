package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/ghl-telnyx-integration/internal/domain"
)

type Config struct {
	DatabaseURL, TelnyxBaseURL, TelnyxToken, TelnyxProfileID, TelnyxVoiceCredentialID, FromNumber string
	HighLevelToken, HighLevelBaseURL, HighLevelLocationID, HighLevelConversationProviderID        string
	HighLevelWebhookSecret, AdminToken                                                            string
	SignalDeskToken, SignalDeskWebhookURL                                                         string
	HighLevelClientID, HighLevelClientSecret, HighLevelRedirectURI, HighLevelUserType             string
	EnabledWorkflowKeys, AllowedLocationIDs                                                       []string
	WebhookKey                                                                                    ed25519.PublicKey
	EnableSending                                                                                 bool
	VAPIDPublic, VAPIDPrivate, VAPIDSubject                                                       string
	Shutdown                                                                                      time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:                     os.Getenv("DATABASE_URL"),
		TelnyxBaseURL:                   valueOrDefault("TELNYX_BASE_URL", "https://api.telnyx.com"),
		TelnyxToken:                     os.Getenv("TELNYX_API_KEY"),
		TelnyxProfileID:                 os.Getenv("TELNYX_MESSAGING_PROFILE_ID"),
		TelnyxVoiceCredentialID:         os.Getenv("TELNYX_VOICE_CREDENTIAL_ID"),
		FromNumber:                      os.Getenv("TELNYX_FROM_NUMBER"),
		HighLevelToken:                  os.Getenv("HIGHLEVEL_TOKEN"),
		HighLevelBaseURL:                valueOrDefault("HIGHLEVEL_BASE_URL", "https://services.leadconnectorhq.com"),
		HighLevelLocationID:             os.Getenv("HIGHLEVEL_LOCATION_ID"),
		HighLevelConversationProviderID: os.Getenv("HIGHLEVEL_CONVERSATION_PROVIDER_ID"),
		HighLevelWebhookSecret:          os.Getenv("HIGHLEVEL_WEBHOOK_SECRET"),
		AdminToken:                      os.Getenv("ADMIN_TOKEN"),
		SignalDeskToken:                 os.Getenv("SIGNAL_DESK_TOKEN"),
		SignalDeskWebhookURL:            os.Getenv("SIGNAL_DESK_WEBHOOK_URL"),
		HighLevelClientID:               os.Getenv("HIGHLEVEL_CLIENT_ID"),
		HighLevelClientSecret:           os.Getenv("HIGHLEVEL_CLIENT_SECRET"),
		HighLevelRedirectURI:            os.Getenv("HIGHLEVEL_REDIRECT_URI"),
		HighLevelUserType:               valueOrDefault("HIGHLEVEL_USER_TYPE", "Location"),
		VAPIDPublic:                     os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivate:                    os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:                    valueOrDefault("VAPID_SUBJECT", "mailto:inbox@localhost"),
		Shutdown:                        10 * time.Second,
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if s := os.Getenv("TELNYX_PUBLIC_KEY"); s != "" {
		b, e := base64.StdEncoding.DecodeString(s)
		if e != nil || len(b) != ed25519.PublicKeySize {
			return c, fmt.Errorf("TELNYX_PUBLIC_KEY must be base64 Ed25519 key")
		}
		c.WebhookKey = ed25519.PublicKey(b)
	}
	c.EnableSending, _ = strconv.ParseBool(os.Getenv("ENABLE_SENDING"))
	if c.EnableSending && len(c.WebhookKey) != ed25519.PublicKeySize {
		return c, fmt.Errorf("TELNYX_PUBLIC_KEY is required when ENABLE_SENDING is true")
	}
	if c.EnableSending {
		if c.TelnyxToken == "" {
			return c, fmt.Errorf("TELNYX_API_KEY is required when ENABLE_SENDING is true")
		}
		if c.TelnyxProfileID == "" {
			return c, fmt.Errorf("TELNYX_MESSAGING_PROFILE_ID is required when ENABLE_SENDING is true")
		}
		if err := domain.ValidateE164(c.FromNumber); err != nil {
			return c, fmt.Errorf("valid TELNYX_FROM_NUMBER is required when ENABLE_SENDING is true")
		}
		if c.HighLevelClientID == "" {
			return c, fmt.Errorf("HIGHLEVEL_CLIENT_ID is required when ENABLE_SENDING is true")
		}
		if c.HighLevelClientSecret == "" {
			return c, fmt.Errorf("HIGHLEVEL_CLIENT_SECRET is required when ENABLE_SENDING is true")
		}
		if c.HighLevelRedirectURI == "" {
			return c, fmt.Errorf("HIGHLEVEL_REDIRECT_URI is required when ENABLE_SENDING is true")
		}
		if c.HighLevelLocationID == "" {
			return c, fmt.Errorf("HIGHLEVEL_LOCATION_ID is required when ENABLE_SENDING is true")
		}
		if c.HighLevelConversationProviderID == "" {
			return c, fmt.Errorf("HIGHLEVEL_CONVERSATION_PROVIDER_ID is required when ENABLE_SENDING is true")
		}
	}
	for _, key := range strings.Split(os.Getenv("ENABLED_WORKFLOWS"), ",") {
		if key = strings.TrimSpace(key); key != "" {
			c.EnabledWorkflowKeys = append(c.EnabledWorkflowKeys, key)
		}
	}
	c.AllowedLocationIDs = uniqueIDs(append([]string{c.HighLevelLocationID}, strings.Split(os.Getenv("HIGHLEVEL_ALLOWED_LOCATION_IDS"), ",")...))
	return c, nil
}

func uniqueIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
