package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	assigned, err := c.messagingNumbers(ctx)
	if err != nil {
		return nil, err
	}
	added, err := c.attachUnassignedNumbers(ctx)
	if err != nil {
		return assigned, nil
	}
	return append(assigned, added...), nil
}

func (c *TelnyxClient) messagingNumbers(ctx context.Context) ([]string, error) {
	var payload struct {
		Data []struct {
			PhoneNumber        string `json:"phone_number"`
			MessagingProfileID string `json:"messaging_profile_id"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/v2/messaging_phone_numbers?page[size]=250", &payload); err != nil {
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

func (c *TelnyxClient) attachUnassignedNumbers(ctx context.Context) ([]string, error) {
	if c.MessagingProfileID == "" {
		return nil, nil
	}
	var payload struct {
		Data []struct {
			ID                 string `json:"id"`
			PhoneNumber        string `json:"phone_number"`
			MessagingProfileID string `json:"messaging_profile_id"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/v2/phone_numbers?page[size]=250", &payload); err != nil {
		return nil, err
	}
	added := []string{}
	for _, number := range payload.Data {
		if number.MessagingProfileID != "" || number.ID == "" || number.PhoneNumber == "" {
			continue
		}
		body, _ := json.Marshal(map[string]string{"messaging_profile_id": c.MessagingProfileID})
		if err := c.sendJSON(ctx, http.MethodPatch, "/v2/phone_numbers/"+url.PathEscape(number.ID)+"/messaging", body); err != nil {
			continue
		}
		_ = c.assignToActiveCampaign(ctx, number.PhoneNumber)
		added = append(added, number.PhoneNumber)
	}
	return added, nil
}

func (c *TelnyxClient) assignToActiveCampaign(ctx context.Context, phone string) error {
	var payload struct {
		Records []struct {
			CampaignID string `json:"campaignId"`
			Status     string `json:"status"`
		} `json:"records"`
	}
	if err := c.getJSON(ctx, "/v2/10dlc/campaign?recordsPerPage=20", &payload); err != nil {
		return err
	}
	campaignID := ""
	for _, record := range payload.Records {
		if record.Status == "ACTIVE" && record.CampaignID != "" {
			campaignID = record.CampaignID
			break
		}
	}
	if campaignID == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"phoneNumber": phone, "campaignId": campaignID})
	return c.sendJSON(ctx, http.MethodPost, "/v2/10dlc/phoneNumberCampaign", body)
}

func (c *TelnyxClient) getJSON(ctx context.Context, path string, out any) error {
	return c.sendJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *TelnyxClient) sendJSON(ctx context.Context, method, path string, body []byte, out ...any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{Status: resp.StatusCode, Code: fmt.Sprint(resp.StatusCode), Message: "Telnyx request failed"}
	}
	if len(out) == 0 || out[0] == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out[0])
}

var _ = time.Second
