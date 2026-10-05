package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"example.com/ghl-telnyx-integration/internal/provider"
)

const (
	signalDeskVoiceProfileName    = "Signal Desk Browser Calls"
	signalDeskVoiceConnectionName = "Signal Desk Browser"
	signalDeskVoiceCredentialName = "Signal Desk Browser"
)

type telnyxResource struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ConnectionName string `json:"connection_name"`
	ResourceID     string `json:"resource_id"`
}

func (a *App) signalDeskVoiceProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := a.signalDeskSecret()
	if secret == "" || !bearerSecretOK(r.Header.Get("Authorization"), secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	credentialID, err := a.ensureSignalDeskVoice(r.Context())
	if err != nil {
		a.logger().Error("provision Signal Desk voice", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "credential_id": credentialID})
}

func (a *App) ensureSignalDeskVoice(ctx context.Context) (string, error) {
	if credentialID, err := a.voiceCredentialID(ctx); err != nil || credentialID != "" {
		return credentialID, err
	}
	profileID, err := a.findTelnyxResource(ctx, "/v2/outbound_voice_profiles?page[size]=100", signalDeskVoiceProfileName)
	if err != nil {
		return "", fmt.Errorf("list Telnyx outbound voice profiles: %w", err)
	}
	if profileID == "" {
		profileID, err = a.createTelnyxResource(ctx, "/v2/outbound_voice_profiles", map[string]any{
			"name": signalDeskVoiceProfileName, "enabled": true, "whitelisted_destinations": []string{"US"},
		})
		if err != nil {
			return "", fmt.Errorf("create Telnyx outbound voice profile: %w", err)
		}
	}
	connectionID, err := a.findTelnyxResource(ctx, "/v2/credential_connections?page[size]=100", signalDeskVoiceConnectionName)
	if err != nil {
		return "", fmt.Errorf("list Telnyx SIP connections: %w", err)
	}
	if connectionID == "" {
		username, password, err := newSIPCredentials()
		if err != nil {
			return "", err
		}
		connectionID, err = a.createTelnyxResource(ctx, "/v2/credential_connections", map[string]any{
			"active": true, "connection_name": signalDeskVoiceConnectionName,
			"user_name": username, "password": password, "anchorsite_override": "Latency",
			"outbound": map[string]string{"outbound_voice_profile_id": profileID},
		})
		if err != nil {
			return "", fmt.Errorf("create Telnyx SIP connection: %w", err)
		}
	}
	credentialID, err := a.createTelnyxResource(ctx, "/v2/telephony_credentials", map[string]string{
		"connection_id": connectionID, "name": signalDeskVoiceCredentialName,
	})
	if err != nil {
		return "", fmt.Errorf("create Telnyx browser credential: %w", err)
	}
	return credentialID, nil
}

func (a *App) voiceCredentialID(ctx context.Context) (string, error) {
	if a.TelnyxVoiceCredentialID != "" {
		return a.TelnyxVoiceCredentialID, nil
	}
	return a.findTelnyxResource(ctx, "/v2/telephony_credentials?page[size]=100", signalDeskVoiceCredentialName)
}

func (a *App) findTelnyxResource(ctx context.Context, path, name string) (string, error) {
	var payload struct {
		Data []telnyxResource `json:"data"`
	}
	if err := a.telnyxJSON(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return "", err
	}
	for _, item := range payload.Data {
		if item.Name == name || item.ConnectionName == name {
			return item.ID, nil
		}
	}
	return "", nil
}

func (a *App) createTelnyxResource(ctx context.Context, path string, body any) (string, error) {
	var payload struct {
		Data telnyxResource `json:"data"`
	}
	if err := a.telnyxJSON(ctx, http.MethodPost, path, body, &payload); err != nil {
		return "", err
	}
	if payload.Data.ID == "" {
		return "", fmt.Errorf("Telnyx response did not include a resource id")
	}
	return payload.Data.ID, nil
}

func (a *App) telnyxJSON(ctx context.Context, method, path string, body, out any) error {
	telnyx, ok := a.Telnyx.(*provider.TelnyxClient)
	if !ok || telnyx.Token == "" {
		return fmt.Errorf("Telnyx access is unavailable")
	}
	baseURL := strings.TrimRight(telnyx.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.telnyx.com"
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+telnyx.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Errors []struct {
				Detail string `json:"detail"`
			} `json:"errors"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&failure)
		if len(failure.Errors) > 0 && failure.Errors[0].Detail != "" {
			return fmt.Errorf("%s", failure.Errors[0].Detail)
		}
		return fmt.Errorf("Telnyx returned status %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
}

func newSIPCredentials() (string, string, error) {
	usernameBytes := make([]byte, 6)
	passwordBytes := make([]byte, 24)
	if _, err := rand.Read(usernameBytes); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(passwordBytes); err != nil {
		return "", "", err
	}
	return "signaldesk" + hex.EncodeToString(usernameBytes), base64.RawURLEncoding.EncodeToString(passwordBytes), nil
}
