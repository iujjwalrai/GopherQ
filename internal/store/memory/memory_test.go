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
