package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HighLevelClient struct {
	BaseURL                string
	Token                  string
	Tokens                 Tokens
	LocationID             string
	ConversationProviderID string
	HTTP                   *http.Client
}

func (c *HighLevelClient) ForwardInbound(ctx context.Context, inbound Inbound) error {
	contactID := inbound.ContactID
	if contactID == "" {
		var err error
		contactID, err = c.findContactID(ctx, inbound.From)
		if err != nil {
			return err
		}
	}
	conversationID := inbound.ConversationID
	if conversationID == "" {
		var err error
		conversationID, err = c.findOrCreateConversation(ctx, contactID)
		if err != nil {
			return err
		}
	}
	body := map[string]any{
		"type":           "SMS",
		"message":        inbound.Text,
		"conversationId": conversationID,
		"contactId":      contactID,
		"direction":      "inbound",
		"altId":          inbound.ProviderEventID,
		"date":           time.Now().UTC().Format(time.RFC3339Nano),
	}
	if c.ConversationProviderID != "" {
		body["conversationProviderId"] = c.ConversationProviderID
	}
	return c.doJSON(ctx, http.MethodPost, "/conversations/messages/inbound", body, nil)
}

func (c *HighLevelClient) SetSMSDND(ctx context.Context, phone string) error {
	contactID, err := c.findContactID(ctx, phone)
	if err != nil {
		return err
	}
	body := map[string]any{"dndSettings": map[string]any{
		"sms": map[string]any{"status": "active", "message": "Opted out by SMS", "code": "OPTED_OUT"},
	}}
	return c.doJSON(ctx, http.MethodPut, "/contacts/"+url.PathEscape(contactID), body, nil)
}

func (c *HighLevelClient) UpdateMessageStatus(ctx context.Context, messageID, status string) error {
	switch status {
	case "pending", "delivered", "failed", "read":
	default:
		return fmt.Errorf("unsupported HighLevel message status %q", status)
	}
	return c.doJSON(ctx, http.MethodPut, "/conversations/messages/"+url.PathEscape(messageID)+"/status", map[string]string{"status": status}, nil)
}

func (c *HighLevelClient) ExecuteCRM(ctx context.Context, job CRMJob) error {
	contactID := job.ContactID
	if contactID == "" && job.Phone != "" {
		var err error
		contactID, err = c.findContactID(ctx, job.Phone)
		if err != nil {
			return err
		}
	}
	if contactID == "" {
		return fmt.Errorf("HighLevel contact id is required for CRM action %q", job.Action)
	}
	switch job.Action {
	case "create_task":
		title := strings.TrimSpace(job.Body)
		if title == "" {
			title = "Follow up"
		}
		body := map[string]any{
			"title":     title,
			"body":      job.Body,
			"dueDate":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
			"completed": false,
		}
		return c.doJSON(ctx, http.MethodPost, "/contacts/"+url.PathEscape(contactID)+"/tasks", body, nil)
	case "archive_conversation":
		conversationID, err := c.findOrCreateConversation(ctx, contactID)
		if err != nil {
			return err
		}
		if err = c.doJSON(ctx, http.MethodPut, "/conversations/"+url.PathEscape(conversationID), map[string]any{
			"locationId":  firstNonEmpty(job.LocationID, c.LocationID),
			"unreadCount": 0,
		}, nil); err != nil {
			return err
		}
		return c.createNote(ctx, contactID, "Conversation archived after negative reply.")
	default:
		note := "Workflow CRM action: " + job.Action
		if strings.TrimSpace(job.Reply) != "" {
			note += "\nReply: " + job.Reply
		}
		if strings.TrimSpace(job.Body) != "" {
			note += "\n" + job.Body
		}
		return c.createNote(ctx, contactID, note)
	}
}

func (c *HighLevelClient) createNote(ctx context.Context, contactID, body string) error {
	return c.doJSON(ctx, http.MethodPost, "/contacts/"+url.PathEscape(contactID)+"/notes", map[string]string{"body": body}, nil)
}

func (c *HighLevelClient) SearchContacts(ctx context.Context, query string) ([]ContactHit, error) {
	query = strings.TrimSpace(query)
	if query == "" || c.LocationID == "" {
		return nil, nil
	}
	values := url.Values{"locationId": {c.LocationID}, "query": {query}, "limit": {"8"}}
	var result struct {
		Contacts []struct {
			ID, Phone, FirstName, LastName, Name string
		} `json:"contacts"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/contacts/?"+values.Encode(), nil, &result); err != nil {
		return nil, err
	}
	hits := make([]ContactHit, 0, len(result.Contacts))
	for _, contact := range result.Contacts {
		name := strings.TrimSpace(contact.Name)
		if name == "" {
			name = strings.TrimSpace(contact.FirstName + " " + contact.LastName)
		}
		hits = append(hits, ContactHit{ID: contact.ID, Name: name, Phone: contact.Phone})
	}
	return hits, nil
}

func (c *HighLevelClient) SendingNumber(ctx context.Context, contactID string) (string, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" || c.LocationID == "" {
		return "", nil
	}
	var fields struct {
		CustomFields []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			FieldKey string `json:"fieldKey"`
		} `json:"customFields"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/locations/"+url.PathEscape(c.LocationID)+"/customFields", nil, &fields); err != nil {
		return "", err
	}
	fieldID := ""
	for _, field := range fields.CustomFields {
		if isSendingNumberField(field.Name, field.FieldKey) {
			fieldID = field.ID
			break
		}
	}
	if fieldID == "" {
		return "", nil
	}
	var contact struct {
		Contact struct {
			CustomFields []struct {
				ID    string          `json:"id"`
				Value json.RawMessage `json:"value"`
			} `json:"customFields"`
		} `json:"contact"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/contacts/"+url.PathEscape(contactID), nil, &contact); err != nil {
		return "", err
	}
	for _, field := range contact.Contact.CustomFields {
		if field.ID != fieldID {
			continue
		}
		var value string
		if json.Unmarshal(field.Value, &value) == nil {
			return strings.TrimSpace(value), nil
		}
	}
	return "", nil
}

func isSendingNumberField(name, key string) bool {
	name = strings.ToLower(strings.Join(strings.Fields(name), " "))
	key = strings.ToLower(strings.TrimSpace(key))
	return name == "sms from number" || key == "contact.sms_from_number" || strings.HasSuffix(key, ".sms_from_number")
}

func (c *HighLevelClient) findContactID(ctx context.Context, phone string) (string, error) {
	if c.LocationID == "" {
		return "", fmt.Errorf("HighLevel location id is required")
	}
	query := url.Values{"locationId": {c.LocationID}, "number": {phone}}
	var result struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/contacts/search/duplicate?"+query.Encode(), nil, &result); err != nil {
		return "", err
	}
	if result.Contact.ID == "" {
		return "", fmt.Errorf("HighLevel contact not found for phone")
	}
	return result.Contact.ID, nil
}

func (c *HighLevelClient) findOrCreateConversation(ctx context.Context, contactID string) (string, error) {
	query := url.Values{"locationId": {c.LocationID}, "contactId": {contactID}, "limit": {"1"}}
	var found struct {
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/conversations/search?"+query.Encode(), nil, &found); err != nil {
		return "", err
	}
	if len(found.Conversations) > 0 && found.Conversations[0].ID != "" {
		return found.Conversations[0].ID, nil
	}
	var created struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
		ID string `json:"id"`
	}
	body := map[string]string{"locationId": c.LocationID, "contactId": contactID}
	if err := c.doJSON(ctx, http.MethodPost, "/conversations/", body, &created); err != nil {
		return "", err
	}
	if created.Conversation.ID != "" {
		return created.Conversation.ID, nil
	}
	if created.ID != "" {
		return created.ID, nil
	}
	return "", fmt.Errorf("HighLevel create conversation response is missing id")
}

func (c *HighLevelClient) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	if err := c.doJSONOnce(ctx, method, path, requestBody, responseBody, false); err == nil {
		return nil
	} else if pe, ok := err.(*Error); !ok || pe.Status != http.StatusUnauthorized || c.Tokens == nil {
		return err
	}
	return c.doJSONOnce(ctx, method, path, requestBody, responseBody, true)
}

func (c *HighLevelClient) doJSONOnce(ctx context.Context, method, path string, requestBody any, responseBody any, forceRefresh bool) error {
	token, err := c.accessToken(ctx, forceRefresh)
	if err != nil {
		return err
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://services.leadconnectorhq.com"
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Version", "2021-07-28")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return &Error{Status: response.StatusCode, Code: fmt.Sprint(response.StatusCode), Message: strings.TrimSpace(string(payload))}
	}
	if responseBody == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(responseBody); err != nil {
		return fmt.Errorf("decode HighLevel response: %w", err)
	}
	return nil
}

func (c *HighLevelClient) accessToken(ctx context.Context, forceRefresh bool) (string, error) {
	if forceRefresh {
		if refresher, ok := c.Tokens.(TokenRefresher); ok {
			return refresher.ForceRefresh(ctx)
		}
	}
	if c.Tokens != nil {
		return c.Tokens.Token(ctx)
	}
	if c.Token == "" {
		return "", fmt.Errorf("HighLevel access token is required")
	}
	return c.Token, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
