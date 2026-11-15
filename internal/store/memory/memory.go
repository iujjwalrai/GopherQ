// Package memory is an in-memory implementation of store.Store.
// Jobs are lost when the process exits.
package memory

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"

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

// clone returns a copy of j that shares no memory with the original.
// Job is copied because it is passed by value; Payload is a slice, so its
// bytes must be copied separately.
func clone(j job.Job) job.Job {
	j.Payload = bytes.Clone(j.Payload)
	return j
}
