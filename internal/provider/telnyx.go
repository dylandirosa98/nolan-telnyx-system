package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			Errors []struct {
				Code   string `json:"code"`
				Detail string `json:"detail"`
			} `json:"errors"`
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		providerError := &Error{Status: resp.StatusCode, Code: fmt.Sprint(resp.StatusCode), Message: "Telnyx request failed"}
		if json.Unmarshal(body, &payload) == nil && len(payload.Errors) > 0 {
			if payload.Errors[0].Code != "" {
				providerError.Code = payload.Errors[0].Code
			}
			if payload.Errors[0].Detail != "" {
				providerError.Message = payload.Errors[0].Detail
			}
		}
		return SendResult{}, providerError
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SendResult{}, err
	}
	if out.Data.ID == "" {
		return SendResult{}, fmt.Errorf("Telnyx response is missing message id")
	}
	return SendResult{ProviderID: out.Data.ID}, nil
}

func (c *TelnyxClient) OwnedNumbers(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v2/messaging_phone_numbers?page[size]=250", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &Error{Status: resp.StatusCode, Code: fmt.Sprint(resp.StatusCode), Message: "Telnyx number lookup failed"}
	}
	var payload struct {
		Data []struct {
			PhoneNumber        string `json:"phone_number"`
			MessagingProfileID string `json:"messaging_profile_id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	numbers := make([]string, 0, len(payload.Data))
	for _, number := range payload.Data {
		if c.MessagingProfileID != "" && number.MessagingProfileID != c.MessagingProfileID {
			continue
		}
		if number.PhoneNumber != "" {
			numbers = append(numbers, number.PhoneNumber)
		}
	}
	return numbers, nil
}

var _ = time.Second
