package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TelnyxClient struct {
	BaseURL, Token, MessagingProfileID string
	HTTP                               *http.Client
}

func (c *TelnyxClient) Send(ctx context.Context, r SendRequest) (SendResult, error) {
	b, _ := json.Marshal(map[string]any{"from": r.From, "to": r.To, "text": r.Text, "messaging_profile_id": c.MessagingProfileID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v2/messages", bytes.NewReader(b))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", r.IdempotencyKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SendResult{}, &Error{Status: resp.StatusCode, Code: fmt.Sprint(resp.StatusCode), Message: "Telnyx request failed"}
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SendResult{}, err
	}
	return SendResult{ProviderID: out.Data.ID}, nil
}

var _ = time.Second
