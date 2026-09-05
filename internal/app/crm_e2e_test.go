package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"example.com/ghl-telnyx-integration/internal/provider"
	"example.com/ghl-telnyx-integration/internal/store"
	"example.com/ghl-telnyx-integration/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCRMWorkerCompletesQueuedTask(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL for compose-backed E2E")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	st := &store.Store{DB: db}
	fake := &provider.FakeHighLevel{}
	application := &App{Store: st, HighLevel: fake, Logger: nil}

	var enrollmentID int64
	if err = db.QueryRow(ctx, `
		INSERT INTO workflow_enrollments(external_id,location_id,workflow_key,contact_id,to_number,from_number,state)
		VALUES($1,'loc','manual-follow-up-1-week','contact','+13125551212','+13125551213','completed')
		RETURNING id`, "crm-e2e-"+time.Now().Format("150405.000000000")).Scan(&enrollmentID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"location_id": "loc", "contact_id": "contact", "phone": "+13125551212"})
	if _, err = db.Exec(ctx, `INSERT INTO crm_jobs(workflow_enrollment_id,action,body,payload) VALUES($1,'create_task','14 day follow up',$2)`, enrollmentID, payload); err != nil {
		t.Fatal(err)
	}

	run, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go application.RunCRMWorker(run)
	deadline := time.Now().Add(2 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		fake.Mu.Lock()
		for _, job := range fake.CRM {
			if job.Action == "create_task" && job.Body == "14 day follow up" && job.ContactID == "contact" {
				found = true
				break
			}
		}
		fake.Mu.Unlock()
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		fake.Mu.Lock()
		defer fake.Mu.Unlock()
		t.Fatalf("crm=%#v", fake.CRM)
	}
}

func TestAdminStatusAndPause(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL for compose-backed E2E")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	application := &App{Store: &store.Store{DB: db}, AdminToken: "admin-secret", EnableSending: false, FromNumber: "+13125551213"}
	statusReq := httptest.NewRequest("GET", "/admin/status", nil)
	statusReq.Header.Set("Authorization", "Bearer admin-secret")
	statusResp := httptest.NewRecorder()
	application.Routes().ServeHTTP(statusResp, statusReq)
	if statusResp.Code != 200 {
		t.Fatalf("status=%d body=%s", statusResp.Code, statusResp.Body.String())
	}
	pauseBody, _ := json.Marshal(map[string]bool{"paused": true})
	pauseReq := httptest.NewRequest("POST", "/admin/sending", bytes.NewReader(pauseBody))
	pauseReq.Header.Set("Authorization", "Bearer admin-secret")
	pauseResp := httptest.NewRecorder()
	application.Routes().ServeHTTP(pauseResp, pauseReq)
	if pauseResp.Code != 200 {
		t.Fatalf("pause=%d body=%s", pauseResp.Code, pauseResp.Body.String())
	}
	_, _ = db.Exec(ctx, `UPDATE settings SET sending_paused=false WHERE id=1`)
}

func TestDisabledWorkflowWorkerWaits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		(&App{}).RunWorkflowWorker(ctx)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("disabled workflow worker exited before shutdown")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disabled workflow worker did not stop")
	}
}
