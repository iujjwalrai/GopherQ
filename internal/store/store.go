// Package store defines how GopherQ saves and loads jobs.
// The API, and later the workers, depend only on the Store interface,
// so the in-memory store can be swapped for Postgres without changing them.
package store

import (
	"context"
	"errors"

	"github.com/ujjwal/gopherq/internal/job"
)

// Errors returned by Store implementations. Check them with errors.Is.
var (
	ErrNotFound      = errors.New("job not found")
	ErrAlreadyExists = errors.New("job already exists")
	ErrNoJobs        = errors.New("no job ready to run")
)

// Store saves and loads jobs. Implementations must be safe to use from
// many goroutines at once.
type Store interface {
	// Create saves a new job. It returns ErrAlreadyExists if a job with
	// the same ID is already stored.
	Create(ctx context.Context, j job.Job) error

	// Get returns the job with the given ID, or ErrNotFound.
	Get(ctx context.Context, id string) (job.Job, error)

	// List returns every job, oldest first (by CreatedAt, then by ID).
	List(ctx context.Context) ([]job.Job, error)

	// ClaimNext atomically picks the ready job (see job.IsReady) with the
	// earliest RunAt (ties: CreatedAt, then ID), marks it running, saves
	// it and returns it. Two callers never get the same job. It returns
	// ErrNoJobs if no job is ready.
	ClaimNext(ctx context.Context) (job.Job, error)

	// Update replaces the stored job that has j's ID with j. It returns
	// ErrNotFound if there is no such job.
	Update(ctx context.Context, j job.Job) error
}
