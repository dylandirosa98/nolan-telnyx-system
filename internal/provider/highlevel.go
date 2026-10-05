package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HighLevelClient struct {
	BaseURL                string
	Token                  string
	Tokens                 Tokens
	LocationID             string
	ConversationProviderID string
	DefaultFromNumber      string
	HTTP                   *http.Client
}

func (c *HighLevelClient) PromoteInbound(ctx context.Context, inbound Inbound) error {
	locationID := strings.TrimSpace(inbound.LocationID)
	if locationID == "" {
		return fmt.Errorf("HighLevel location id is required")
	}
	client := c.forLocation(locationID)
	contactID, err := client.findOrCreateContact(ctx, inbound.From)
	if err != nil {
		return err
	}
	inbound.ContactID = contactID
	inbound.LocationID = locationID
	return client.ForwardInbound(ctx, inbound)
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

func (c *HighLevelClient) RecentConversations(ctx context.Context, locationID string, limit int) ([]Conversation, error) {
	locationID = strings.TrimSpace(locationID)
	if locationID == "" {
		return nil, fmt.Errorf("HighLevel location id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	client := c.forLocation(locationID)
	values := url.Values{
		"locationId":      {locationID},
		"limit":           {strconv.Itoa(limit)},
		"sort":            {"desc"},
		"sortBy":          {"last_message_date"},
		"status":          {"recents"},
		"lastMessageType": {"TYPE_SMS"},
	}
	var search struct {
		Conversations []struct {
			ID, ContactID, Phone, LastMessageBody, LastMessageDirection string
			LastMessageDate                                             json.RawMessage `json:"lastMessageDate"`
		} `json:"conversations"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/conversations/search?"+values.Encode(), nil, &search); err != nil {
		return nil, err
	}

	result := make([]Conversation, 0, len(search.Conversations))
	for _, summary := range search.Conversations {
		if summary.ID == "" {
			continue
		}
		messageValues := url.Values{"limit": {"30"}, "type": {"TYPE_SMS"}}
		var history struct {
			Messages struct {
				Messages []struct {
					ID, AltID, Body, Direction, From, To string
					DateAdded                            json.RawMessage `json:"dateAdded"`
					Attachments                          []string        `json:"attachments"`
				} `json:"messages"`
			} `json:"messages"`
		}
		if err := client.doJSON(ctx, http.MethodGet, "/conversations/"+url.PathEscape(summary.ID)+"/messages?"+messageValues.Encode(), nil, &history); err != nil {
			return nil, err
		}

		conversation := Conversation{
			ContactID:     summary.ContactID,
			ContactNumber: summary.Phone,
			LastBody:      summary.LastMessageBody,
			LastDirection: strings.ToLower(summary.LastMessageDirection),
			LastAt:        providerTime(summary.LastMessageDate),
			Messages:      make([]ConversationMessage, 0, len(history.Messages.Messages)),
		}
		for _, message := range history.Messages.Messages {
			contactNumber, telnyxNumber := message.To, message.From
			if strings.EqualFold(message.Direction, "inbound") {
				contactNumber, telnyxNumber = message.From, message.To
			}
			if conversation.ContactNumber == "" {
				conversation.ContactNumber = contactNumber
			}
			if conversation.TelnyxNumber == "" && telnyxNumber != "" {
				conversation.TelnyxNumber = telnyxNumber
			}
			media, _ := json.Marshal(message.Attachments)
			occurredAt := providerTime(message.DateAdded)
			conversation.Messages = append(conversation.Messages, ConversationMessage{
				ID: message.ID, ProviderMessageID: firstNonEmpty(message.AltID, message.ID),
				Direction: strings.ToLower(message.Direction), ContactNumber: contactNumber,
				TelnyxNumber: telnyxNumber, Body: message.Body, MediaJSON: string(media), OccurredAt: occurredAt,
			})
		}
		conversation.MessageCount = len(conversation.Messages)
		if len(conversation.Messages) > 0 {
			latest := conversation.Messages[0]
			conversation.LastBody = latest.Body
			conversation.LastDirection = latest.Direction
			conversation.LastAt = latest.OccurredAt
		}
		if conversation.LastAt == 0 {
			conversation.LastAt = time.Now().UTC().UnixMilli()
		}
		if conversation.LastDirection != "outbound" {
			conversation.LastDirection = "inbound"
		}
		if !isE164(conversation.TelnyxNumber) {
			conversation.TelnyxNumber = ""
			if summary.ContactID != "" {
				if from, lookupErr := client.SendingNumberFor(ctx, locationID, summary.ContactID); lookupErr == nil && isE164(from) {
					conversation.TelnyxNumber = from
				}
			}
			if conversation.TelnyxNumber == "" && isE164(client.DefaultFromNumber) {
				conversation.TelnyxNumber = client.DefaultFromNumber
			}
		}
		result = append(result, conversation)
	}
	return result, nil
}

func isE164(value string) bool {
	if len(value) < 8 || len(value) > 16 || value[0] != '+' {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func providerTime(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		if number < 1e12 {
			number *= 1000
		}
		return int64(number)
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return 0
	}
	if number, err := strconv.ParseInt(value, 10, 64); err == nil {
		if number < 1e12 {
			number *= 1000
		}
		return number
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func (c *HighLevelClient) SendingNumber(ctx context.Context, contactID string) (string, error) {
	return c.SendingNumberFor(ctx, c.LocationID, contactID)
}

func (c *HighLevelClient) SendingNumberFor(ctx context.Context, locationID, contactID string) (string, error) {
	contactID = strings.TrimSpace(contactID)
	locationID = strings.TrimSpace(locationID)
	if contactID == "" || locationID == "" {
		return "", nil
	}
	client := c.forLocation(locationID)
	var fields struct {
		CustomFields []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			FieldKey string `json:"fieldKey"`
		} `json:"customFields"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/locations/"+url.PathEscape(locationID)+"/customFields", nil, &fields); err != nil {
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
	if err := client.doJSON(ctx, http.MethodGet, "/contacts/"+url.PathEscape(contactID), nil, &contact); err != nil {
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

func (c *HighLevelClient) forLocation(locationID string) *HighLevelClient {
	if c == nil || locationID == "" || locationID == c.LocationID {
		return c
	}
	clone := *c
	clone.LocationID = locationID
	if source, ok := c.Tokens.(*HighLevelTokenSource); ok && source != nil {
		tokenSource := *source
		tokenSource.LocationID = locationID
		clone.Tokens = &tokenSource
	}
	return &clone
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

func (c *HighLevelClient) findOrCreateContact(ctx context.Context, phone string) (string, error) {
	contactID, err := c.findContactID(ctx, phone)
	if err == nil {
		return contactID, nil
	}
	if providerErr, ok := err.(*Error); ok {
		return "", providerErr
	}
	var created struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
		ID string `json:"id"`
	}
	body := map[string]string{"locationId": c.LocationID, "phone": phone}
	if err = c.doJSON(ctx, http.MethodPost, "/contacts/", body, &created); err != nil {
		return "", err
	}
	if created.Contact.ID != "" {
		return created.Contact.ID, nil
	}
	if created.ID != "" {
		return created.ID, nil
	}
	return "", fmt.Errorf("HighLevel create contact response is missing id")
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
