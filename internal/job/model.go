package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// JobStatus is the lifecycle state of a job.
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusDead      JobStatus = "dead"
)

// Errors returned by the Mark… methods. Callers check them with errors.Is.
var (
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrNoRetriesLeft     = errors.New("no retries left")
)

// allowedTransitions lists, for each status, the statuses it may move to.
// A status that is not a key here (completed, dead) cannot move anywhere.
var allowedTransitions = map[JobStatus][]JobStatus{
	StatusPending: {StatusRunning},
	StatusRunning: {StatusCompleted, StatusFailed},
	StatusFailed:  {StatusRunning, StatusDead},
}

func (s JobStatus) canTransitionTo(next JobStatus) bool {
	return slices.Contains(allowedTransitions[s], next)
}

// Job is a unit of work submitted to the queue.
type Job struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	Status     JobStatus       `json:"status"`
	Retries    int             `json:"retries"`
	MaxRetries int             `json:"max_retries"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	RunAt      time.Time       `json:"run_at"` // not run before this time
	Error      string          `json:"error"`
}

// IsReady reports whether a worker may claim the job at time now: it is
// pending, or failed with retries left, and its RunAt has arrived.
func (j *Job) IsReady(now time.Time) bool {
	waiting := j.Status == StatusPending ||
		(j.Status == StatusFailed && j.CanRetry())

	return waiting && !j.RunAt.After(now)
}

// MarkRunning moves a pending job, or a failed job being retried, to running.
// Retrying a failed job uses up one retry.
func (j *Job) MarkRunning() error {
	if err := j.checkTransition(StatusRunning); err != nil {
		return err
	}

	if j.Status == StatusFailed {
		if !j.CanRetry() {
			return ErrNoRetriesLeft
		}
		j.Retries++
	}

	j.Error = ""
	j.setStatus(StatusRunning)
	return nil
}

// MarkCompleted moves a running job to completed.
func (j *Job) MarkCompleted() error {
	if err := j.checkTransition(StatusCompleted); err != nil {
		return err
	}

	j.setStatus(StatusCompleted)
	return nil
}

// MarkFailed moves a running job to failed and records why it failed.
func (j *Job) MarkFailed(cause error) error {
	if err := j.checkTransition(StatusFailed); err != nil {
		return err
	}

	j.Error = "unknown error"
	if cause != nil {
		j.Error = cause.Error()
	}

	j.setStatus(StatusFailed)
	return nil
}

// MarkDead moves a failed job to dead (the dead-letter state).
func (j *Job) MarkDead() error {
	if err := j.checkTransition(StatusDead); err != nil {
		return err
	}

	j.setStatus(StatusDead)
	return nil
}

// CanRetry reports whether the job still has retries left.
func (j *Job) CanRetry() bool {
	return j.Retries < j.MaxRetries
}

func (j *Job) checkTransition(to JobStatus) error {
	if !j.Status.canTransitionTo(to) {
		return fmt.Errorf("%w: %q -> %q", ErrInvalidTransition, j.Status, to)
	}
	return nil
}

func (j *Job) setStatus(s JobStatus) {
	j.Status = s
	j.UpdatedAt = time.Now().UTC()
}
