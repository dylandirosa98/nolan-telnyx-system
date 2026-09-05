package app

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"example.com/ghl-telnyx-integration/internal/domain"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/internal/webhook"
	"example.com/ghl-telnyx-integration/internal/workflow"
	"github.com/jackc/pgx/v5"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type App struct {
	Store               *store.Store
	Telnyx              provider.Telnyx
	HighLevel           provider.HighLevel
	WebhookKey          ed25519.PublicKey
	HighLevelWebhookKey ed25519.PublicKey
	HLSecret            string
	FromNumber          string
	EnableSending       bool
	Workflows           map[string]workflow.Definition
	Logger              *slog.Logger
}

func (a *App) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); w.Write([]byte(`{"status":"ok"}`)) })
	m.HandleFunc("/readyz", a.ready)
	m.HandleFunc("/webhooks/highlevel/outbound", a.highlevel)
	m.HandleFunc("/webhooks/telnyx", a.telnyx)
	m.HandleFunc("/workflows/enroll", a.enrollWorkflow)
	return m
}
func (a *App) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.DB.Ping(r.Context()); err != nil {
		http.Error(w, "not ready", 503)
		return
	}
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ready"}`))
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
	} else if a.HLSecret != "" {
		authenticated = r.Header.Get("Authorization") == "Bearer "+a.HLSecret
	}
	if !authenticated {
		http.Error(w, "unauthorized", 401)
		return
	}
	var p ghlRequest
	if json.Unmarshal(body, &p) != nil || p.Type != "SMS" || len(p.Attachments) > 0 || p.ContactID == "" || p.LocationID == "" || p.MessageID == "" || p.Message == "" || domain.ValidateE164(p.Phone) != nil || domain.ValidateE164(a.FromNumber) != nil {
		http.Error(w, "invalid request", 400)
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
	if e.Data.EventType == "message.sent" || e.Data.EventType == "message.finalized" {
		outbound, err := a.Store.FindOutboundByProviderID(r.Context(), p.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "database error", 500)
			return
		}
		status := highLevelDeliveryStatus(e.Data.EventType, p.To)
		if err == nil && outbound.ContactID != "" && a.HighLevel != nil {
			if err = a.HighLevel.UpdateMessageStatus(r.Context(), outbound.MessageID, status); err != nil {
				http.Error(w, "HighLevel error", 502)
				return
			}
		}
		if err = a.Store.UpdateDelivery(r.Context(), e.Data.ID, p.ID, status); err != nil {
			http.Error(w, "database error", 500)
			return
		}
	}
	if len(p.To) > 0 && (domain.IsOptOut(p.Text) || p.AutoresponseType == "STOP") {
		if err := a.Store.Suppress(r.Context(), p.From.PhoneNumber, "telnyx", e.Data.ID); err != nil {
			http.Error(w, "database error", 500)
			return
		}
		if a.HighLevel != nil {
			if err := a.HighLevel.SetSMSDND(r.Context(), p.From.PhoneNumber); err != nil {
				http.Error(w, "HighLevel error", 502)
				return
			}
		}
	}
	if e.Data.EventType == "message.received" && len(p.To) > 0 {
		inserted, err := a.Store.RecordInbound(r.Context(), e.Data.ID, p.From.PhoneNumber, p.To[0].PhoneNumber, p.Text)
		if err != nil {
			http.Error(w, "database error", 500)
			return
		}
		if inserted && len(a.Workflows) > 0 {
			if err := a.applyWorkflowReply(r.Context(), p.From.PhoneNumber, p.Text); err != nil {
				logger := a.Logger
				if logger == nil {
					logger = slog.Default()
				}
				logger.Error("apply workflow reply", "error", err)
				http.Error(w, "database error", 500)
				return
			}
		}
		if a.HighLevel != nil {
			if err := a.HighLevel.ForwardInbound(r.Context(), provider.Inbound{From: p.From.PhoneNumber, To: p.To[0].PhoneNumber, Text: p.Text, ProviderEventID: e.Data.ID}); err != nil {
				http.Error(w, "HighLevel error", 502)
				return
			}
		}
	}
	w.WriteHeader(202)
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
		if err != nil || supp {
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
				_ = a.Store.Suppress(ctx, j.To, "telnyx", "")
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

func highLevelDeliveryStatus(eventType string, recipients []struct {
	PhoneNumber string `json:"phone_number"`
	Status      string `json:"status"`
}) string {
	if eventType == "message.sent" {
		return "pending"
	}
	if len(recipients) > 0 && recipients[0].Status == "delivered" {
		return "delivered"
	}
	return "failed"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
