package app

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"example.com/ghl-telnyx-integration/internal/domain"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/internal/webhook"
	"example.com/ghl-telnyx-integration/internal/workflow"
	"github.com/jackc/pgx/v5"
)

type App struct {
	Store               *store.Store
	Telnyx              provider.Telnyx
	HighLevel           provider.HighLevel
	OAuth               *provider.OAuthClient
	WebhookKey          ed25519.PublicKey
	HighLevelWebhookKey ed25519.PublicKey
	HLSecret            string
	AdminToken          string
	LocationID          string
	FromNumber          string
	EnableSending       bool
	Workflows           map[string]workflow.Definition
	Logger              *slog.Logger
}

func (a *App) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	m.HandleFunc("/readyz", a.ready)
	m.HandleFunc("/webhooks/highlevel/outbound", a.highlevel)
	m.HandleFunc("/webhooks/telnyx", a.telnyx)
	m.HandleFunc("/workflows/enroll", a.enrollWorkflow)
	m.HandleFunc("/admin", a.adminPage)
	m.HandleFunc("/admin/status", a.adminStatus)
	m.HandleFunc("/admin/sending", a.adminSending)
	m.HandleFunc("/oauth/highlevel/start", a.oauthStart)
	m.HandleFunc("/oauth/highlevel/callback", a.oauthCallback)
	return m
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
	if a.LocationID != "" && p.LocationID != a.LocationID {
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
		if err := a.applyWorkflowReply(ctx, from, text); err != nil {
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
		res, err := a.Telnyx.Send(ctx, provider.SendRequest{To: j.To, From: j.From, Text: j.Text, IdempotencyKey: j.LocationID + ":" + j.MessageID})
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
				_ = a.Store.RetryInbound(ctx, msg.EventID, time.Minute)
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
				_ = a.Store.RetryDelivery(ctx, event.EventID, time.Minute)
			}
		}
		if !worked {
			time.Sleep(250 * time.Millisecond)
		}
	}
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
	if a.LocationID != "" && locationID != a.LocationID {
		http.Error(w, "oauth location does not match this deployment", http.StatusForbidden)
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
