package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ujjwal/gopherq/internal/handler"
	"github.com/ujjwal/gopherq/internal/job"
	"github.com/ujjwal/gopherq/internal/store"
	"github.com/ujjwal/gopherq/internal/store/memory"
)

// testRegistry knows two job types: "test" accepts any payload, and
// "strict" rejects every payload.
func testRegistry() *handler.Registry {
	noop := func(context.Context, json.RawMessage) error { return nil }

	reg := handler.NewRegistry()
	reg.Register("test", noop, nil)
	reg.Register("strict", noop, func(json.RawMessage) error {
		return errors.New(`needs "to"`)
	})
	return reg
}

// do sends one request to a server backed by st and returns the response.
func do(t *testing.T, st store.Store, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()

	NewServer(st, testRegistry()).Handler().ServeHTTP(rec, req)
	return rec
}

// errorMessage decodes {"error": "..."} from a response.
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Error
}

func TestHealth(t *testing.T) {
	rec := do(t, memory.New(), http.MethodGet, "/health", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestCreateJob(t *testing.T) {
	st := memory.New()

	rec := do(t, st, http.MethodPost, "/jobs",
		`{"type": "test", "payload": {"to": "user@example.com"}}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	var created job.Job
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("expected a generated job ID")
	}
	if created.Status != job.StatusPending {
		t.Fatalf("status = %q, want pending", created.Status)
	}
	if created.CreatedAt.IsZero() || !created.UpdatedAt.Equal(created.CreatedAt) {
		t.Fatalf("updated_at = %v, want it equal to created_at %v",
			created.UpdatedAt, created.CreatedAt)
	}

	// The job must really be in the store, not just in the response.
	if _, err := st.Get(context.Background(), created.ID); err != nil {
		t.Fatalf("job not saved: %v", err)
	}
}

func TestCreateJobRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{name: "not JSON", body: `{oops`,
			wantMsg: "invalid JSON body"},
		{name: "missing type", body: `{"payload": {"a": 1}}`,
			wantMsg: "type and valid payload are required"},
		{name: "missing payload", body: `{"type": "test"}`,
			wantMsg: "type and valid payload are required"},
		{name: "unknown type", body: `{"type": "emial", "payload": {}}`,
			wantMsg: `unknown job type "emial"`},
		{name: "invalid payload", body: `{"type": "strict", "payload": {}}`,
			wantMsg: `invalid payload: needs "to"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := memory.New()
			rec := do(t, st, http.MethodPost, "/jobs", tc.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if got := errorMessage(t, rec); got != tc.wantMsg {
				t.Fatalf("error = %q, want %q", got, tc.wantMsg)
			}

			// Nothing may be saved for a rejected request.
			if jobs, _ := st.List(context.Background()); len(jobs) != 0 {
				t.Fatalf("store has %d jobs, want 0", len(jobs))
			}
		})
	}
}

func TestGetJob(t *testing.T) {
	st := memory.New()
	if err := st.Create(context.Background(), job.Job{ID: "job-1", Type: "test"}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, st, http.MethodGet, "/jobs/job-1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got job.Job
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "job-1" {
		t.Fatalf("id = %q, want job-1", got.ID)
	}
}

func TestGetJobNotFound(t *testing.T) {
	rec := do(t, memory.New(), http.MethodGet, "/jobs/missing", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListJobsOldestFirst(t *testing.T) {
	st := memory.New()
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	for _, j := range []job.Job{
		{ID: "newer", CreatedAt: base.Add(time.Minute)},
		{ID: "older", CreatedAt: base},
	} {
		if err := st.Create(context.Background(), j); err != nil {
			t.Fatal(err)
		}
	}

	rec := do(t, st, http.MethodGet, "/jobs", "")

	var jobs []job.Job
	if err := json.NewDecoder(rec.Body).Decode(&jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].ID != "older" || jobs[1].ID != "newer" {
		t.Fatalf("got %+v, want [older newer]", jobs)
	}
}

func TestListJobsEmptyIsArray(t *testing.T) {
	rec := do(t, memory.New(), http.MethodGet, "/jobs", "")

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("body = %s, want []", got)
	}
}

// collidingStore wraps a real memory store but pretends the first
// `collisions` IDs are already taken.
type collidingStore struct {
	*memory.Store
	collisions int
}

func (c *collidingStore) Create(ctx context.Context, j job.Job) error {
	if c.collisions > 0 {
		c.collisions--
		return store.ErrAlreadyExists
	}
	return c.Store.Create(ctx, j)
}

func TestCreateJobRetriesOnIDCollision(t *testing.T) {
	tests := []struct {
		name       string
		collisions int
		wantStatus int
	}{
		{name: "succeeds on third try", collisions: maxIDAttempts - 1, wantStatus: http.StatusCreated},
		{name: "gives up after max attempts", collisions: maxIDAttempts, wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := &collidingStore{Store: memory.New(), collisions: tc.collisions}

			rec := do(t, st, http.MethodPost, "/jobs", `{"type": "test", "payload": {}}`)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

// brokenStore fails every Get and List, like a database that is down.
type brokenStore struct {
	*memory.Store
}

var errDatabaseDown = errors.New("dial tcp 10.0.0.5:5432: connection refused")

func (brokenStore) Get(context.Context, string) (job.Job, error) {
	return job.Job{}, errDatabaseDown
}

func (brokenStore) List(context.Context) ([]job.Job, error) {
	return nil, errDatabaseDown
}

func TestStoreFailureHidesDetails(t *testing.T) {
	for _, path := range []string{"/jobs", "/jobs/job-1"} {
		t.Run(path, func(t *testing.T) {
			rec := do(t, brokenStore{memory.New()}, http.MethodGet, path, "")

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Fatalf("response leaks internal error: %s", rec.Body.String())
			}
		})
	}
}
