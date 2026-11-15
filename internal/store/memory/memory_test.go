package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ujjwal/gopherq/internal/job"
	"github.com/ujjwal/gopherq/internal/store"
	"github.com/ujjwal/gopherq/internal/store/memory"
)

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	s := memory.New()

	want := job.Job{ID: "job-1", Type: "email", Payload: json.RawMessage(`{"to":"a@b.com"}`)}
	if err := s.Create(ctx, want); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Type != want.Type || string(got.Payload) != string(want.Payload) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGetMissingJob(t *testing.T) {
	_, err := memory.New().Get(context.Background(), "nope")

	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestCreateDuplicateIDKeepsOriginal(t *testing.T) {
	ctx := context.Background()
	s := memory.New()

	if err := s.Create(ctx, job.Job{ID: "job-1", Type: "email"}); err != nil {
		t.Fatal(err)
	}

	err := s.Create(ctx, job.Job{ID: "job-1", Type: "webhook"})
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("error = %v, want ErrAlreadyExists", err)
	}

	got, _ := s.Get(ctx, "job-1")
	if got.Type != "email" {
		t.Fatalf("type = %q, original job was overwritten", got.Type)
	}
}

func TestListOldestFirstThenByID(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	// Inserted out of order on purpose. "b" and "a" share a timestamp.
	for _, j := range []job.Job{
		{ID: "c", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "b", CreatedAt: base},
		{ID: "a", CreatedAt: base},
	} {
		if err := s.Create(ctx, j); err != nil {
			t.Fatal(err)
		}
	}

	jobs, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, j := range jobs {
		gotIDs = append(gotIDs, j.ID)
	}
	if fmt.Sprint(gotIDs) != "[a b c]" {
		t.Fatalf("order = %v, want [a b c]", gotIDs)
	}
}

func TestListEmptyIsNotNil(t *testing.T) {
	jobs, err := memory.New().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// A nil slice would be encoded as JSON null instead of [].
	if jobs == nil {
		t.Fatal("List returned nil, want an empty slice")
	}
}

func TestPayloadIsCopied(t *testing.T) {
	ctx := context.Background()
	s := memory.New()

	payload := json.RawMessage(`{"n":1}`)
	if err := s.Create(ctx, job.Job{ID: "job-1", Payload: payload}); err != nil {
		t.Fatal(err)
	}

	// Change the caller's bytes after saving.
	payload[5] = '9'

	// Change the bytes of a job we read back.
	got, _ := s.Get(ctx, "job-1")
	got.Payload[5] = '7'

	stored, _ := s.Get(ctx, "job-1")
	if string(stored.Payload) != `{"n":1}` {
		t.Fatalf("stored payload changed to %s", stored.Payload)
	}
}

func TestCancelledContext(t *testing.T) {
	s := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before we even start

	if err := s.Create(ctx, job.Job{ID: "job-1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}
	if _, err := s.Get(ctx, "job-1"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get error = %v, want context.Canceled", err)
	}
	if _, err := s.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List error = %v, want context.Canceled", err)
	}
	if _, err := s.ClaimNext(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("ClaimNext error = %v, want context.Canceled", err)
	}
	if err := s.Update(ctx, job.Job{ID: "job-1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Update error = %v, want context.Canceled", err)
	}
}

// mustCreate saves every job or stops the test.
func mustCreate(t *testing.T, s *memory.Store, jobs ...job.Job) {
	t.Helper()
	for _, j := range jobs {
		if err := s.Create(context.Background(), j); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClaimNextOrder(t *testing.T) {
	s := memory.New()
	base := time.Now().UTC().Add(-time.Hour) // all in the past, so ready

	mustCreate(t, s,
		job.Job{ID: "d", Status: job.StatusPending, RunAt: base.Add(2 * time.Minute)},
		job.Job{ID: "b", Status: job.StatusPending, RunAt: base, CreatedAt: base.Add(time.Second)},
		job.Job{ID: "a", Status: job.StatusPending, RunAt: base, CreatedAt: base},
		job.Job{ID: "c", Status: job.StatusPending, RunAt: base, CreatedAt: base.Add(time.Second)},
		// never claimable:
		job.Job{ID: "future", Status: job.StatusPending, RunAt: time.Now().Add(time.Hour)},
		job.Job{ID: "done", Status: job.StatusCompleted, RunAt: base},
		job.Job{ID: "out-of-retries", Status: job.StatusFailed, Retries: 3, MaxRetries: 3, RunAt: base},
	)

	// a: earliest RunAt and CreatedAt. b, c: same RunAt and CreatedAt, so by ID.
	// d: latest RunAt among the ready jobs.
	for _, want := range []string{"a", "b", "c", "d"} {
		got, err := s.ClaimNext(context.Background())
		if err != nil {
			t.Fatalf("claiming %s: %v", want, err)
		}
		if got.ID != want {
			t.Fatalf("claimed %s, want %s", got.ID, want)
		}
		if got.Status != job.StatusRunning {
			t.Fatalf("claimed job status = %q, want running", got.Status)
		}

		stored, _ := s.Get(context.Background(), want)
		if stored.Status != job.StatusRunning {
			t.Fatalf("stored status = %q, want running", stored.Status)
		}
	}

	if _, err := s.ClaimNext(context.Background()); !errors.Is(err, store.ErrNoJobs) {
		t.Fatalf("error = %v, want ErrNoJobs", err)
	}
}

func TestClaimNextRetriesFailedJob(t *testing.T) {
	s := memory.New()
	mustCreate(t, s, job.Job{
		ID: "job-1", Status: job.StatusFailed, Retries: 0, MaxRetries: 3,
		Error: "smtp timeout", RunAt: time.Now().Add(-time.Second),
	})

	got, err := s.ClaimNext(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// MarkRunning from Step 1a did its work: one retry used, error cleared.
	if got.Status != job.StatusRunning || got.Retries != 1 || got.Error != "" {
		t.Fatalf("got status=%q retries=%d error=%q, want running/1/\"\"",
			got.Status, got.Retries, got.Error)
	}
}

func TestClaimNextEmptyStore(t *testing.T) {
	_, err := memory.New().ClaimNext(context.Background())

	if !errors.Is(err, store.ErrNoJobs) {
		t.Fatalf("error = %v, want ErrNoJobs", err)
	}
}

// TestClaimNextNeverDoubleClaims has 8 workers race to claim 200 jobs.
// Every job must be claimed exactly once. Run it with -race.
func TestClaimNextNeverDoubleClaims(t *testing.T) {
	s := memory.New()
	const total = 200
	for i := range total {
		mustCreate(t, s, job.Job{ID: fmt.Sprintf("job-%03d", i), Status: job.StatusPending})
	}

	var (
		mu     sync.Mutex
		counts = make(map[string]int) // job ID -> times claimed
		wg     sync.WaitGroup
	)

	for range 8 {
		wg.Go(func() {
			for {
				j, err := s.ClaimNext(context.Background())
				if errors.Is(err, store.ErrNoJobs) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}

				mu.Lock()
				counts[j.ID]++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(counts) != total {
		t.Fatalf("claimed %d different jobs, want %d", len(counts), total)
	}
	for id, n := range counts {
		if n != 1 {
			t.Errorf("job %s claimed %d times, want 1", id, n)
		}
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	mustCreate(t, s, job.Job{ID: "job-1", Status: job.StatusPending})

	j, err := s.ClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.MarkCompleted(); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, j); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(ctx, "job-1")
	if got.Status != job.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
}

func TestUpdateMissingJob(t *testing.T) {
	err := memory.New().Update(context.Background(), job.Job{ID: "nope"})

	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateCopiesPayload(t *testing.T) {
	ctx := context.Background()
	s := memory.New()
	mustCreate(t, s, job.Job{ID: "job-1"})

	payload := json.RawMessage(`{"n":1}`)
	if err := s.Update(ctx, job.Job{ID: "job-1", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	payload[5] = '9' // change the caller's bytes after saving

	got, _ := s.Get(ctx, "job-1")
	if string(got.Payload) != `{"n":1}` {
		t.Fatalf("stored payload changed to %s", got.Payload)
	}
}

// TestConcurrentAccess is meant to be run with -race. Without the mutex,
// the race detector reports these goroutines touching the map at once.
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := memory.New()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			id := fmt.Sprintf("job-%d", i)
			if err := s.Create(ctx, job.Job{ID: id}); err != nil {
				t.Error(err)
			}
			if _, err := s.Get(ctx, id); err != nil {
				t.Error(err)
			}
			if _, err := s.List(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	jobs, _ := s.List(ctx)
	if len(jobs) != 50 {
		t.Fatalf("len = %d, want 50", len(jobs))
	}
}
