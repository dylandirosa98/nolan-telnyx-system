package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL, TelnyxBaseURL, TelnyxToken, TelnyxProfileID, FromNumber, HighLevelToken string
	WebhookKey                                                                           ed25519.PublicKey
	EnableSending                                                                        bool
	Shutdown                                                                             time.Duration
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), TelnyxBaseURL: os.Getenv("TELNYX_BASE_URL"), TelnyxToken: os.Getenv("TELNYX_API_KEY"), TelnyxProfileID: os.Getenv("TELNYX_MESSAGING_PROFILE_ID"), FromNumber: os.Getenv("TELNYX_FROM_NUMBER"), HighLevelToken: os.Getenv("HIGHLEVEL_TOKEN"), Shutdown: 10 * time.Second}
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
	return c, nil
}
