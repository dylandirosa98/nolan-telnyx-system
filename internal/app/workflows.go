package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"example.com/ghl-telnyx-integration/internal/workflow"
	"github.com/jackc/pgx/v5"
)

type workflowEnrollRequest struct {
	ExternalID    string            `json:"external_id"`
	LocationID    string            `json:"location_id"`
	WorkflowKey   string            `json:"workflow_key"`
	ContactID     string            `json:"contact_id"`
	To            string            `json:"to"`
	From          string            `json:"from"`
	Timezone      string            `json:"timezone"`
	ConsentAt     time.Time         `json:"consent_at"`
	ConsentSource string            `json:"consent_source"`
	Variables     map[string]string `json:"variables"`
}

func (a *App) enrollWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.HLSecret == "" || r.Header.Get("Authorization") != "Bearer "+a.HLSecret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var request workflowEnrollRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	definition, ok := a.Workflows[request.WorkflowKey]
	if !ok {
		http.Error(w, "unknown workflow", http.StatusNotFound)
		return
	}
	if request.ExternalID == "" || request.LocationID == "" || request.From == "" {
		http.Error(w, "external_id, location_id, and from are required", http.StatusBadRequest)
		return
	}
	enrollment, err := workflow.Start(definition, workflow.Contact{
		ID:            request.ContactID,
		Phone:         request.To,
		Timezone:      request.Timezone,
		ConsentAt:     request.ConsentAt,
		ConsentSource: request.ConsentSource,
		Variables:     request.Variables,
	}, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	id, created, err := a.Store.CreateWorkflowEnrollment(r.Context(), request.ExternalID, request.LocationID, request.From, enrollment)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusAccepted)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"created": created, "enrollment_id": id})
}

func (a *App) RunWorkflowWorker(ctx context.Context) {
	if !a.EnableSending {
		return
	}
	logger := a.Logger
	if logger == nil {
		logger = slog.Default()
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		record, err := a.Store.ClaimDueWorkflow(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if err != nil {
			logger.Error("claim workflow", "error", err)
			time.Sleep(time.Second)
			continue
		}
		definition, ok := a.Workflows[record.Enrollment.WorkflowKey]
		if !ok || !definition.Enabled {
			_ = a.Store.RetryWorkflow(ctx, record.ID, time.Hour)
			continue
		}
		commands, err := workflow.Advance(definition, &record.Enrollment, time.Now().UTC())
		if err != nil {
			logger.Error("advance workflow", "workflow", record.Enrollment.WorkflowKey, "error", err)
			_ = a.Store.FailWorkflow(ctx, record.ID)
			continue
		}
		if err = a.Store.ApplyWorkflowCommands(ctx, record, commands, ""); err != nil {
			logger.Error("apply workflow", "workflow", record.Enrollment.WorkflowKey, "error", err)
			_ = a.Store.RetryWorkflow(ctx, record.ID, time.Minute)
		}
	}
}

func (a *App) applyWorkflowReply(ctx context.Context, phone, text string) error {
	records, err := a.Store.ClaimWorkflowsForReply(ctx, phone)
	if err != nil {
		return err
	}
	for _, record := range records {
		definition, ok := a.Workflows[record.Enrollment.WorkflowKey]
		if !ok {
			if err = a.Store.FailWorkflow(ctx, record.ID); err != nil {
				return err
			}
			continue
		}
		commands := workflow.Reply(definition, &record.Enrollment, text)
		if err = a.Store.ApplyWorkflowCommands(ctx, record, commands, text); err != nil {
			return err
		}
	}
	return nil
}
