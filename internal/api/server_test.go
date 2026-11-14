package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ujjwal/gopherq/internal/job"
)

func TestCreateJob(t *testing.T) {
	server := NewServer()
	handler := server.Handler()

	body := []byte(`{
		"type": "email",
		"payload": {
			"to": "user@example.com"
		}
	}`)

	req := httptest.NewRequest(
		http.MethodPost,
		"/jobs",
		bytes.NewReader(body),
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var created job.Job

	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	if created.ID == "" {
		t.Fatal("expected generated job ID")
	}

	if created.Status != job.StatusPending {
		t.Fatalf("expected pending status, got %s", created.Status)
	}
}

func TestHealth(t *testing.T) {
	server := NewServer()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}