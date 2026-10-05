package app

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/ghl-telnyx-integration/internal/domain"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/internal/webhook"
	"example.com/ghl-telnyx-integration/internal/workflow"
	"github.com/jackc/pgx/v5"
)

const defaultSignalDeskWebhookURL = "https://signal-desk-inbox.chillindylan.chatgpt.site/api/telnyx"

type App struct {
	Store                                   *store.Store
	Telnyx                                  provider.Telnyx
	HighLevel                               provider.HighLevel
	OAuth                                   *provider.OAuthClient
	WebhookKey                              ed25519.PublicKey
	HighLevelWebhookKey                     ed25519.PublicKey
	HLSecret                                string
	AdminToken                              string
	SignalDeskToken                         string
	SignalDeskWebhookURL                    string
	LocationID                              string
	AllowedLocationIDs                      []string
	FromNumber                              string
	TelnyxVoiceCredentialID                 string
	EnableSending                           bool
	VAPIDPublic, VAPIDPrivate, VAPIDSubject string
	Workflows                               map[string]workflow.Definition
	Logger                                  *slog.Logger
	HTTP                                    *http.Client
}

func (a *App) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	m.HandleFunc("/readyz", a.ready)
	m.HandleFunc("/webhooks/highlevel/outbound", a.highlevel)
	m.HandleFunc("/webhooks/outbound", a.highlevel)
	m.HandleFunc("/webhooks/telnyx", a.telnyx)
	m.HandleFunc("/workflows/enroll", a.enrollWorkflow)
	m.HandleFunc("/admin", a.adminPage)
	m.HandleFunc("/admin/status", a.adminStatus)
	m.HandleFunc("/admin/sending", a.adminSending)
	m.HandleFunc("/oauth/start", a.oauthStart)
	m.HandleFunc("/oauth/callback", a.oauthCallback)
	m.HandleFunc("/oauth/highlevel/start", a.oauthStart)
	m.HandleFunc("/oauth/highlevel/callback", a.oauthCallback)
	m.HandleFunc("/signal-desk/conversations", a.signalDeskConversations)
	m.HandleFunc("/signal-desk/unknown", a.signalDeskUnknown)
	m.HandleFunc("/signal-desk/voice/token", a.signalDeskVoiceToken)
	m.HandleFunc("/signal-desk/voice/status", a.signalDeskVoiceStatus)
	m.HandleFunc("/signal-desk/voice/provision", a.signalDeskVoiceProvision)
	return m
}

func (a *App) signalDeskVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := a.signalDeskSecret()
	if secret == "" || !bearerSecretOK(r.Header.Get("Authorization"), secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	telnyx, ok := a.Telnyx.(*provider.TelnyxClient)
	if !ok || telnyx.Token == "" {
		http.Error(w, "Telnyx access is unavailable", http.StatusServiceUnavailable)
		return
	}
	baseURL := strings.TrimRight(telnyx.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.telnyx.com"
	}
	credentialID, _ := a.voiceCredentialID(r.Context())
	result := map[string]any{"configuredCredentialId": credentialID}
	for key, path := range map[string]string{
		"connections":           "/v2/credential_connections?page[size]=100",
		"credentials":           "/v2/telephony_credentials?page[size]=100",
		"outboundVoiceProfiles": "/v2/outbound_voice_profiles?page[size]=100",
	} {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, baseURL+path, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+telnyx.Token)
		client := a.HTTP
		if client == nil {
			client = http.DefaultClient
		}
		response, err := client.Do(req)
		if err != nil {
			result[key+"Status"] = http.StatusBadGateway
			continue
		}
		var payload struct {
			Data []map[string]any `json:"data"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
		response.Body.Close()
		result[key+"Status"] = response.StatusCode
		if decodeErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			items := make([]map[string]any, 0, len(payload.Data))
			for _, item := range payload.Data {
				items = append(items, map[string]any{
					"id": item["id"], "name": firstNonNil(item["connection_name"], item["name"], item["username"]),
					"connectionId": item["connection_id"], "active": item["active"],
				})
			}
			result[key] = items
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func firstNonNil(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func (a *App) signalDeskVoiceToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := a.signalDeskSecret()
	if secret == "" || !bearerSecretOK(r.Header.Get("Authorization"), secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	credentialID, err := a.voiceCredentialID(r.Context())
	if err != nil || credentialID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Voice calling has not been connected yet."})
		return
	}
	baseURL := "https://api.telnyx.com"
	if telnyx, ok := a.Telnyx.(*provider.TelnyxClient); ok && telnyx.BaseURL != "" {
		baseURL = strings.TrimRight(telnyx.BaseURL, "/")
	}
	target := baseURL + "/v2/telephony_credentials/" + url.PathEscape(credentialID) + "/token"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, nil)
	if err != nil {
		http.Error(w, "voice token unavailable", http.StatusBadGateway)
		return
	}
	if telnyx, ok := a.Telnyx.(*provider.TelnyxClient); ok {
		req.Header.Set("Authorization", "Bearer "+telnyx.Token)
	}
	req.Header.Set("Accept", "text/plain")
	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		http.Error(w, "voice token unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Telnyx could not create a call token."})
		return
	}
	token := strings.TrimSpace(string(payload))
	if strings.HasPrefix(token, "{") {
		var decoded struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(payload, &decoded) == nil && decoded.Data != "" {
			token = decoded.Data
		}
	}
	if token == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Telnyx returned an empty call token."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"data": token})
}

func (a *App) signalDeskUnknown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := a.signalDeskSecret()
	if secret == "" || !bearerSecretOK(r.Header.Get("Authorization"), secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	messages, err := a.Store.RecentUnknownInbound(r.Context(), 20)
	if err != nil {
		a.logger().Error("load unknown inbound messages", "error", err)
		http.Error(w, "messages unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"messages": messages})
}

func (a *App) signalDeskConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := a.signalDeskSecret()
	if secret == "" || !bearerSecretOK(r.Header.Get("Authorization"), secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodPost {
		var body struct {
			EventID    string `json:"event_id"`
			LocationID string `json:"location_id"`
			From       string `json:"from"`
			To         string `json:"to"`
			Text       string `json:"text"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body) != nil || body.EventID == "" || body.LocationID == "" || domain.ValidateE164(body.From) != nil || domain.ValidateE164(body.To) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !a.allowsLocation(r.Context(), body.LocationID) {
			http.Error(w, "unknown location", http.StatusForbidden)
			return
		}
		if err := a.HighLevel.PromoteInbound(r.Context(), provider.Inbound{LocationID: body.LocationID, From: body.From, To: body.To, Text: body.Text, ProviderEventID: body.EventID}); err != nil {
			a.logger().Error("move Signal Desk message to HighLevel", "location_id", body.LocationID, "error", err)
			http.Error(w, "conversation could not be created", http.StatusBadGateway)
			return
		}
		if a.Store != nil {
			if err := a.Store.ResolveSignalDeskInbound(r.Context(), body.EventID); err != nil {
				a.logger().Error("resolve Signal Desk message", "error", err)
				http.Error(w, "message was moved but could not be removed from the unknown inbox", http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	locationID := strings.TrimSpace(r.URL.Query().Get("location_id"))
	if !a.allowsLocation(r.Context(), locationID) {
		http.Error(w, "unknown location", http.StatusForbidden)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit != 50 && limit != 100 {
		limit = 20
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days != 1 && days != 7 {
		days = 2
	}
	conversations, err := a.HighLevel.RecentConversations(r.Context(), locationID, limit)
	if err != nil {
		a.logger().Error("load HighLevel conversations", "location_id", locationID, "error", err)
		http.Error(w, "conversations unavailable", http.StatusBadGateway)
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	filtered := conversations[:0]
	for _, conversation := range conversations {
		if conversation.LastAt >= cutoff {
			filtered = append(filtered, conversation)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"conversations": filtered})
}

func (a *App) ready(w http.ResponseWriter, r *http.Request) {
	if a.Store == nil || a.Store.DB == nil {
		http.Error(w, "not ready", 503)
		return
	}
	if err := a.Store.DB.Ping(r.Context()); err != nil {
		http.Error(w, "not ready", 503)
		return
	}
	w.WriteHeader(200)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

type ghlRequest struct {
	ContactID      string   `json:"contactId"`
	ConversationID string   `json:"conversationId"`
	LocationID     string   `json:"locationId"`
	MessageID      string   `json:"messageId"`
	Type           string   `json:"type"`
	Attachments    []string `json:"attachments"`
	Message        string   `json:"message"`
	Phone          string   `json:"phone"`
	UserID         string   `json:"userId"`
}

func (a *App) highlevel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	authenticated := false
	if len(a.HighLevelWebhookKey) == ed25519.PublicKeySize {
		authenticated = webhook.VerifyHighLevel(r, body, a.HighLevelWebhookKey) == nil
	}
	if !authenticated && a.HLSecret != "" {
		authenticated = bearerSecretOK(r.Header.Get("Authorization"), a.HLSecret)
	}
	if !authenticated {
		http.Error(w, "unauthorized", 401)
		return
	}
	var p ghlRequest
	if json.Unmarshal(body, &p) != nil || p.Type != "SMS" || len(p.Attachments) > 0 || p.ContactID == "" || p.LocationID == "" || p.MessageID == "" || p.Message == "" || len(p.Message) > 1600 || domain.ValidateE164(p.Phone) != nil || domain.ValidateE164(a.FromNumber) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if !a.allowsLocation(r.Context(), p.LocationID) {
		http.Error(w, "unknown location", http.StatusForbidden)
		return
	}
	ok, err := a.Store.Enqueue(r.Context(), store.Outbound{LocationID: p.LocationID, ContactID: p.ContactID, MessageID: p.MessageID, To: p.Phone, From: a.FromNumber, Text: p.Message})
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"status":         "success",
		"type":           "SMS",
		"messageId":      p.MessageID,
		"conversationId": p.ConversationID,
		"dateAdded":      time.Now().UTC().Format(time.RFC3339Nano),
		"duplicate":      !ok,
	})
}

func (a *App) resolveSendingNumber(ctx context.Context, locationID, contactID, fallback string) (string, error) {
	raw := ""
	if a.HighLevel != nil && contactID != "" {
		var value string
		var err error
		if lookup, ok := a.HighLevel.(interface {
			SendingNumberFor(context.Context, string, string) (string, error)
		}); ok {
			value, err = lookup.SendingNumberFor(ctx, locationID, contactID)
		} else {
			value, err = a.HighLevel.SendingNumber(ctx, contactID)
		}
		if err != nil {
			return "", err
		}
		raw = value
	}
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	owned := []string{}
	if a.Telnyx != nil {
		numbers, err := a.Telnyx.OwnedNumbers(ctx)
		if err != nil {
			return "", err
		}
		owned = numbers
	}
	return domain.SelectSendingNumber(raw, fallback, owned)
}

func (a *App) acceptsInbound(ctx context.Context, to string) bool {
	if a.FromNumber == "" || to == a.FromNumber {
		return true
	}
	if a.Telnyx == nil {
		return false
	}
	owned, err := a.Telnyx.OwnedNumbers(ctx)
	if err != nil {
		return false
	}
	for _, number := range owned {
		if number == to {
			return true
		}
	}
	return false
}

func (a *App) telnyx(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := webhook.VerifyTelnyx(r, body, a.WebhookKey, time.Now(), 5*time.Minute); err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var e struct {
		Data struct {
			ID        string `json:"id"`
			EventType string `json:"event_type"`
			Payload   struct {
				ID   string `json:"id"`
				From struct {
					PhoneNumber string `json:"phone_number"`
				} `json:"from"`
				To []struct {
					PhoneNumber string `json:"phone_number"`
					Status      string `json:"status"`
				} `json:"to"`
				Text             string `json:"text"`
				AutoresponseType string `json:"autoresponse_type"`
			} `json:"payload"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &e) != nil {
		http.Error(w, "bad json", 400)
		return
	}
	p := e.Data.Payload
	insertedInbound := false
	insertedDelivery := false
	if e.Data.EventType == "message.sent" || e.Data.EventType == "message.finalized" {
		status := highLevelDeliveryStatus(e.Data.EventType, p.To)
		inserted, err := a.Store.UpdateDelivery(r.Context(), e.Data.ID, p.ID, status)
		if err != nil {
			http.Error(w, "database error", 500)
			return
		}
		insertedDelivery = inserted
	}
	if e.Data.EventType == "message.received" && len(p.To) > 0 {
		if !a.acceptsInbound(r.Context(), p.To[0].PhoneNumber) {
			w.WriteHeader(202)
			return
		}
		inserted, err := a.Store.RecordInbound(r.Context(), e.Data.ID, p.From.PhoneNumber, p.To[0].PhoneNumber, p.Text)
		if err != nil {
			http.Error(w, "database error", 500)
			return
		}
		insertedInbound = inserted
		if domain.IsOptOut(p.Text) || p.AutoresponseType == "STOP" {
			if err := a.Store.Suppress(r.Context(), p.From.PhoneNumber, "telnyx", e.Data.ID); err != nil {
				http.Error(w, "database error", 500)
				return
			}
		}
	}
	w.WriteHeader(202)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	if insertedDelivery {
		if err := a.syncDeliveryEvent(r.Context(), e.Data.ID, p.ID, highLevelDeliveryStatus(e.Data.EventType, p.To)); err != nil {
			a.logger().Error("sync delivery", "error", err)
		}
	}
	if insertedInbound {
		if err := a.processInbound(r.Context(), e.Data.ID, p.From.PhoneNumber, p.To[0].PhoneNumber, p.Text); err != nil {
			a.logger().Error("process inbound", "error", err)
		}
	}
	if webhookURL := a.signalDeskWebhookURL(); webhookURL != "" && r.Header.Get("x-signal-desk-forwarded") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.forwardSignalDesk(ctx, r, body, webhookURL); err != nil {
			a.logger().Error("forward Signal Desk webhook", "error", err)
		}
	}
}

func (a *App) forwardSignalDesk(ctx context.Context, source *http.Request, body []byte, webhookURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-signal-desk-forwarded", "1")
	for _, name := range []string{"telnyx-signature-ed25519", "telnyx-timestamp"} {
		if value := source.Header.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Signal Desk webhook returned %d", resp.StatusCode)
	}
	return nil
}

func (a *App) signalDeskSecret() string {
	if a.AdminToken != "" {
		mac := hmac.New(sha256.New, []byte(a.AdminToken))
		_, _ = mac.Write([]byte("signal-desk-v1"))
		return fmt.Sprintf("%x", mac.Sum(nil))
	}
	return a.SignalDeskToken
}

func (a *App) signalDeskWebhookURL() string {
	if a.SignalDeskWebhookURL != "" {
		return a.SignalDeskWebhookURL
	}
	secret := a.signalDeskSecret()
	if secret == "" {
		return ""
	}
	target, err := url.Parse(defaultSignalDeskWebhookURL)
	if err != nil {
		return ""
	}
	query := target.Query()
	query.Set("token", secret)
	target.RawQuery = query.Encode()
	return target.String()
}

func (a *App) processInbound(ctx context.Context, eventID, from, to, text string) error {
	suppressed, err := a.Store.IsSuppressed(ctx, from)
	if err != nil {
		return err
	}
	if (domain.IsOptOut(text) || suppressed) && a.HighLevel != nil {
		if err := a.HighLevel.SetSMSDND(ctx, from); err != nil {
			return err
		}
	}
	if len(a.Workflows) > 0 {
		if err := a.applyWorkflowReply(ctx, from, to, text); err != nil {
			return err
		}
	}
	if a.HighLevel != nil {
		if err := a.HighLevel.ForwardInbound(ctx, provider.Inbound{From: from, To: to, Text: text, ProviderEventID: eventID}); err != nil {
			return err
		}
	}
	return a.Store.MarkInboundProcessed(ctx, eventID)
}

func (a *App) syncDelivery(ctx context.Context, providerID, status string) error {
	outbound, err := a.Store.FindOutboundByProviderID(ctx, providerID)
	if err != nil {
		return err
	}
	if outbound.MessageID != "" && a.HighLevel != nil {
		return a.HighLevel.UpdateMessageStatus(ctx, outbound.MessageID, status)
	}
	return nil
}

func (a *App) syncDeliveryEvent(ctx context.Context, eventID, providerID, status string) error {
	if err := a.syncDelivery(ctx, providerID, status); err != nil {
		return err
	}
	return a.Store.MarkDeliverySynced(ctx, eventID)
}

func (a *App) RunWorker(ctx context.Context) {
	if !a.EnableSending {
		<-ctx.Done()
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		j, err := a.Store.Claim(ctx)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		paused, err := a.Store.SendingPaused(ctx)
		if err != nil || paused {
			_ = a.Store.Retry(ctx, j.ID, time.Second)
			continue
		}
		supp, err := a.Store.IsSuppressed(ctx, j.To)
		if err != nil {
			_ = a.Store.Retry(ctx, j.ID, time.Second)
			continue
		}
		if supp {
			_ = a.Store.Fail(ctx, j.ID)
			continue
		}
		from, err := a.resolveSendingNumber(ctx, j.LocationID, j.ContactID, j.From)
		if err != nil {
			if errors.Is(err, domain.ErrUnknownSendingNumber) {
				_ = a.Store.Fail(ctx, j.ID)
			} else {
				_ = a.Store.Retry(ctx, j.ID, time.Second)
			}
			continue
		}
		res, err := a.Telnyx.Send(ctx, provider.SendRequest{To: j.To, From: from, Text: j.Text, IdempotencyKey: j.LocationID + ":" + j.MessageID})
		if err == nil {
			_ = a.Store.Complete(ctx, j.ID, res.ProviderID)
			continue
		}
		if pe, ok := err.(*provider.Error); ok {
			switch domain.ClassifyProviderError(pe.Status, pe.Code) {
			case domain.RetrySuppression:
				_ = a.Store.Suppress(ctx, j.To, "telnyx", pe.Code)
				_ = a.Store.Fail(ctx, j.ID)
				continue
			case domain.RetryPermanent:
				_ = a.Store.Fail(ctx, j.ID)
				continue
			}
		}
		if j.Attempts < 5 {
			_ = a.Store.Retry(ctx, j.ID, time.Duration(1<<min(j.Attempts, 6))*time.Second)
		} else {
			_ = a.Store.Fail(ctx, j.ID)
		}
	}
}

func (a *App) RunCRMWorker(ctx context.Context) {
	if a.HighLevel == nil {
		<-ctx.Done()
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		job, err := a.Store.ClaimCRM(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if err != nil {
			a.logger().Error("claim crm", "error", err)
			time.Sleep(time.Second)
			continue
		}
		if err = a.HighLevel.ExecuteCRM(ctx, provider.CRMJob{
			Action: job.Action, Body: job.Body, ContactID: job.ContactID,
			LocationID: job.LocationID, Phone: job.Phone, Reply: job.Reply,
		}); err != nil {
			a.logger().Error("execute crm", "action", job.Action, "error", err)
			if job.Attempts < 5 {
				_ = a.Store.RetryCRM(ctx, job.ID, time.Duration(1<<min(job.Attempts, 6))*time.Second)
			} else {
				_ = a.Store.FailCRM(ctx, job.ID)
			}
			continue
		}
		_ = a.Store.CompleteCRM(ctx, job.ID)
	}
}

func (a *App) RunInboundWorker(ctx context.Context) {
	const maxAttempts = 5
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		worked := false
		msg, err := a.Store.ClaimUnprocessedInbound(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			a.logger().Error("claim inbound", "error", err)
			time.Sleep(time.Second)
			continue
		}
		if err == nil {
			worked = true
			if err = a.processInbound(ctx, msg.EventID, msg.From, msg.To, msg.Body); err != nil {
				a.logger().Error("process inbound", "error", err)
				if msg.Attempts >= maxAttempts {
					_ = a.Store.FailInbound(ctx, msg.EventID)
				} else {
					_ = a.Store.RetryInbound(ctx, msg.EventID, highLevelRetryDelay(msg.Attempts))
				}
			}
		}
		event, err := a.Store.ClaimUnprocessedDelivery(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			a.logger().Error("claim delivery", "error", err)
			time.Sleep(time.Second)
			continue
		}
		if err == nil {
			worked = true
			if err = a.syncDeliveryEvent(ctx, event.EventID, event.ProviderMessageID, event.Status); err != nil {
				a.logger().Error("sync delivery", "error", err)
				if event.Attempts >= maxAttempts {
					_ = a.Store.FailDelivery(ctx, event.EventID)
				} else {
					_ = a.Store.RetryDelivery(ctx, event.EventID, highLevelRetryDelay(event.Attempts))
				}
			}
		}
		if !worked {
			time.Sleep(250 * time.Millisecond)
		}
	}
}

func highLevelRetryDelay(attempt int) time.Duration {
	return time.Duration(1<<min(attempt, 6)) * time.Minute
}

func highLevelDeliveryStatus(eventType string, recipients []struct {
	PhoneNumber string `json:"phone_number"`
	Status      string `json:"status"`
}) string {
	if eventType == "message.sent" {
		return "pending"
	}
	if len(recipients) == 0 {
		return "failed"
	}
	switch recipients[0].Status {
	case "delivered":
		return "delivered"
	case "delivery_unconfirmed":
		return "pending"
	default:
		return "failed"
	}
}

func (a *App) logger() *slog.Logger {
	if a.Logger != nil {
		return a.Logger
	}
	return slog.Default()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if a.AdminToken == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if bearerSecretOK(r.Header.Get("Authorization"), a.AdminToken) {
		return true
	}
	if bearerSecretOK("Bearer "+r.Header.Get("X-Admin-Token"), a.AdminToken) {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (a *App) adminStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	paused := true
	if a.Store != nil {
		if value, err := a.Store.SendingPaused(r.Context()); err == nil {
			paused = value
		}
	}
	counts, err := a.Store.QueueCounts(r.Context(), a.LocationID)
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	writeJSON(w, 200, map[string]any{
		"enable_sending":      a.EnableSending,
		"sending_paused":      paused,
		"from_number":         a.FromNumber != "",
		"location_configured": a.LocationID != "",
		"telnyx_webhook_key":  len(a.WebhookKey) == ed25519.PublicKeySize,
		"highlevel_token":     counts.OAuthTokenPresent,
		"oauth_expires_at":    counts.OAuthExpiresAt,
		"queued_outbound":     counts.QueuedOutbound,
		"sending_outbound":    counts.SendingOutbound,
		"failed_outbound":     counts.FailedOutbound,
		"queued_crm":          counts.QueuedCRM,
		"failed_crm":          counts.FailedCRM,
		"failed_inbound":      counts.FailedInbound,
		"failed_delivery":     counts.FailedDelivery,
		"suppressions":        counts.Suppressions,
		"active_enrollments":  counts.ActiveEnrollments,
		"ready_to_send":       a.EnableSending && !paused && a.FromNumber != "" && len(a.WebhookKey) == ed25519.PublicKeySize,
	})
}

func (a *App) adminSending(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if err := a.Store.SetSendingPaused(r.Context(), body.Paused); err != nil {
		http.Error(w, "database error", 500)
		return
	}
	_ = a.Store.RecordAdminAction(r.Context(), "set_sending_paused", map[string]any{"paused": body.Paused})
	writeJSON(w, 200, map[string]any{"sending_paused": body.Paused, "enable_sending": a.EnableSending})
}

func (a *App) adminPage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Launch readiness</title>
<p>Use <code>GET /admin/status</code> with <code>Authorization: Bearer $ADMIN_TOKEN</code>.</p>
<p>Pause sending with <code>POST /admin/sending {"paused":true}</code>.</p>
<p>Process-level ENABLE_SENDING remains the hard safety gate.</p>`))
}

func (a *App) allowsLocation(ctx context.Context, id string) bool {
	if id == "" {
		return false
	}
	if a.LocationID == "" && len(a.AllowedLocationIDs) == 0 {
		return true
	}
	if id == a.LocationID {
		return true
	}
	for _, allowed := range a.AllowedLocationIDs {
		if id == allowed {
			return true
		}
	}
	if a.Store == nil {
		return false
	}
	_, err := a.Store.LoadOAuthToken(ctx, "highlevel", id)
	return err == nil
}

func (a *App) oauthStart(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	if a.OAuth == nil || a.OAuth.ClientID == "" || a.OAuth.RedirectURI == "" {
		http.Error(w, "oauth is not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := a.Store.CreateOAuthState(r.Context(), 10*time.Minute)
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	values := url.Values{
		"response_type": {"code"},
		"client_id":     {a.OAuth.ClientID},
		"redirect_uri":  {a.OAuth.RedirectURI},
		"state":         {state},
	}
	http.Redirect(w, r, "https://marketplace.gohighlevel.com/oauth/chooselocation?"+values.Encode(), http.StatusFound)
}

func (a *App) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if a.OAuth == nil {
		http.Error(w, "oauth is not configured", http.StatusServiceUnavailable)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		http.Error(w, "invalid oauth callback", 400)
		return
	}
	if err := a.Store.ConsumeOAuthState(r.Context(), state); err != nil {
		http.Error(w, "invalid oauth state", http.StatusUnauthorized)
		return
	}
	token, err := a.OAuth.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "oauth exchange failed", http.StatusBadGateway)
		return
	}
	locationID := token.LocationID
	if locationID == "" {
		locationID = a.LocationID
	}
	if locationID == "" {
		http.Error(w, "oauth response is missing location", 400)
		return
	}
	if err = a.Store.SaveOAuthToken(r.Context(), store.OAuthToken{
		Provider:     "highlevel",
		LocationID:   locationID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		UserType:     token.UserType,
		Scope:        token.Scope,
		ExpiresAt:    token.ExpiresAt(time.Now().UTC()),
	}); err != nil {
		http.Error(w, "database error", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "connected", "location_id": locationID, "expires_at": token.ExpiresAt(time.Now().UTC())})
}
