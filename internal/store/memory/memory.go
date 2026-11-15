// Package memory is an in-memory implementation of store.Store.
// Jobs are lost when the process exits.
package memory

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ujjwal/gopherq/internal/job"
	"github.com/ujjwal/gopherq/internal/store"
)

// Compile-time check that *Store satisfies the store.Store interface.
var _ store.Store = (*Store)(nil)

// Store keeps jobs in a map guarded by a read-write mutex.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]job.Job
}

// New returns an empty, ready-to-use Store.
func New() *Store {
	return &Store{
		jobs: make(map[string]job.Job),
	}
}

// Create saves j, or returns store.ErrAlreadyExists if its ID is taken.
func (m *Store) Create(ctx context.Context, j job.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.jobs[j.ID]; exists {
		return store.ErrAlreadyExists
	}

	m.jobs[j.ID] = clone(j)
	return nil
}

// Get returns the job with the given ID, or store.ErrNotFound.
func (m *Store) Get(ctx context.Context, id string) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	j, exists := m.jobs[id]
	if !exists {
		return job.Job{}, store.ErrNotFound
	}

	return clone(j), nil
}

// List returns every job, oldest first, with ties broken by ID.
func (m *Store) List(ctx context.Context) ([]job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	jobs := make([]job.Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, clone(j))
	}
	m.mu.RUnlock()

	slices.SortFunc(jobs, func(a, b job.Job) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})

	return jobs, nil
}

// ClaimNext picks the ready job with the earliest RunAt, marks it running
// and returns it, or returns store.ErrNoJobs. The whole check-and-change
// happens under one lock, so two callers can never claim the same job.
func (m *Store) ClaimNext(ctx context.Context) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}

	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	var next job.Job
	found := false
	for _, j := range m.jobs {
		if !j.IsReady(now) {
			continue
		}
		if !found || claimOrder(j, next) < 0 {
			next = j
			found = true
		}
	}
	if !found {
		return job.Job{}, store.ErrNoJobs
	}

	if err := next.MarkRunning(); err != nil {
		return job.Job{}, fmt.Errorf("claim job %s: %w", next.ID, err)
	}
	m.jobs[next.ID] = next

	return clone(next), nil
}

// claimOrder is negative when a should be claimed before b: earliest
// RunAt first, then earliest CreatedAt, then smallest ID.
func claimOrder(a, b job.Job) int {
	if c := a.RunAt.Compare(b.RunAt); c != 0 {
		return c
	}
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

// Update replaces the stored job with j, or returns store.ErrNotFound.
func (m *Store) Update(ctx context.Context, j job.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.jobs[j.ID]; !exists {
		return store.ErrNotFound
	}

	m.jobs[j.ID] = clone(j)
	return nil
}

// clone returns a copy of j that shares no memory with the original.
// Job is copied because it is passed by value; Payload is a slice, so its
// bytes must be copied separately.
func clone(j job.Job) job.Job {
	j.Payload = bytes.Clone(j.Payload)
	return j
}
