package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL, TelnyxBaseURL, TelnyxToken, TelnyxProfileID, FromNumber                   string
	HighLevelToken, HighLevelBaseURL, HighLevelLocationID, HighLevelConversationProviderID string
	HighLevelWebhookSecret, AdminToken                                                     string
	HighLevelClientID, HighLevelClientSecret, HighLevelRedirectURI, HighLevelUserType      string
	EnabledWorkflowKeys                                                                    []string
	WebhookKey                                                                             ed25519.PublicKey
	EnableSending                                                                          bool
	Shutdown                                                                               time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:                     os.Getenv("DATABASE_URL"),
		TelnyxBaseURL:                   valueOrDefault("TELNYX_BASE_URL", "https://api.telnyx.com"),
		TelnyxToken:                     os.Getenv("TELNYX_API_KEY"),
		TelnyxProfileID:                 os.Getenv("TELNYX_MESSAGING_PROFILE_ID"),
		FromNumber:                      os.Getenv("TELNYX_FROM_NUMBER"),
		HighLevelToken:                  os.Getenv("HIGHLEVEL_TOKEN"),
		HighLevelBaseURL:                valueOrDefault("HIGHLEVEL_BASE_URL", "https://services.leadconnectorhq.com"),
		HighLevelLocationID:             os.Getenv("HIGHLEVEL_LOCATION_ID"),
		HighLevelConversationProviderID: os.Getenv("HIGHLEVEL_CONVERSATION_PROVIDER_ID"),
		HighLevelWebhookSecret:          os.Getenv("HIGHLEVEL_WEBHOOK_SECRET"),
		AdminToken:                      os.Getenv("ADMIN_TOKEN"),
		HighLevelClientID:               os.Getenv("HIGHLEVEL_CLIENT_ID"),
		HighLevelClientSecret:           os.Getenv("HIGHLEVEL_CLIENT_SECRET"),
		HighLevelRedirectURI:            os.Getenv("HIGHLEVEL_REDIRECT_URI"),
		HighLevelUserType:               valueOrDefault("HIGHLEVEL_USER_TYPE", "Location"),
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
	for _, key := range strings.Split(os.Getenv("ENABLED_WORKFLOWS"), ",") {
		if key = strings.TrimSpace(key); key != "" {
			c.EnabledWorkflowKeys = append(c.EnabledWorkflowKeys, key)
		}
	}
	return c, nil
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
