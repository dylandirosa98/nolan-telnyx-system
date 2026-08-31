package app

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"example.com/ghl-telnyx-integration/internal/domain"
	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/internal/webhook"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type App struct {
	Store         *store.Store
	Telnyx        provider.Telnyx
	HighLevel     provider.HighLevel
	WebhookKey    ed25519.PublicKey
	HLSecret      string
	EnableSending bool
	Logger        *slog.Logger
}

func (a *App) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); w.Write([]byte(`{"status":"ok"}`)) })
	m.HandleFunc("/readyz", a.ready)
	m.HandleFunc("/webhooks/highlevel/outbound", a.highlevel)
	m.HandleFunc("/webhooks/telnyx", a.telnyx)
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
	LocationID string `json:"location_id"`
	MessageID  string `json:"message_id"`
	To         string `json:"to"`
	From       string `json:"from"`
	Text       string `json:"text"`
}

func (a *App) highlevel(w http.ResponseWriter, r *http.Request) {
	if a.HLSecret != "" && r.Header.Get("Authorization") != "Bearer "+a.HLSecret {
		http.Error(w, "unauthorized", 401)
		return
	}
	var p ghlRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || json.Unmarshal(body, &p) != nil || p.LocationID == "" || p.MessageID == "" || domain.ValidateE164(p.To) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	ok, err := a.Store.Enqueue(r.Context(), store.Outbound{LocationID: p.LocationID, MessageID: p.MessageID, To: p.To, From: p.From, Text: p.Text})
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	if ok {
		w.WriteHeader(202)
	} else {
		w.WriteHeader(200)
	}
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
		_ = a.Store.UpdateDelivery(r.Context(), e.Data.ID, p.ID, e.Data.EventType)
	}
	if len(p.To) > 0 && (domain.IsOptOut(p.Text) || p.AutoresponseType == "STOP") {
		_ = a.Store.Suppress(r.Context(), p.From.PhoneNumber, "telnyx", e.Data.ID)
		if a.HighLevel != nil {
			_ = a.HighLevel.SetSMSDND(r.Context(), p.From.PhoneNumber)
		}
	}
	if a.HighLevel != nil && e.Data.EventType == "message.received" && len(p.To) > 0 {
		if err := a.Store.RecordInbound(r.Context(), e.Data.ID, p.From.PhoneNumber, p.To[0].PhoneNumber, p.Text); err != nil {
			http.Error(w, "database error", 500)
			return
		}
		_ = a.HighLevel.ForwardInbound(r.Context(), provider.Inbound{From: p.From.PhoneNumber, To: p.To[0].PhoneNumber, Text: p.Text, ProviderEventID: e.Data.ID})
	}
	w.WriteHeader(202)
}
func (a *App) RunWorker(ctx context.Context) {
	if !a.EnableSending {
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
		if pe, ok := err.(*provider.Error); ok && domain.ClassifyProviderError(pe.Status, pe.Code) == domain.RetrySuppression {
			_ = a.Store.Suppress(ctx, j.To, "telnyx", "")
			_ = a.Store.Fail(ctx, j.ID)
			continue
		}
		if j.Attempts < 5 {
			_ = a.Store.Retry(ctx, j.ID, time.Duration(1<<min(j.Attempts, 6))*time.Second)
		} else {
			_ = a.Store.Fail(ctx, j.ID)
		}
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
