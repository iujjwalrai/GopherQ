// Package handler maps job types (like "email") to the code that runs them.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// HandlerFunc does the work for one job type. It receives the job's
// payload; returning an error marks that attempt as failed.
type HandlerFunc func(ctx context.Context, payload json.RawMessage) error

// ValidateFunc checks a payload before a job is accepted. A nil
// ValidateFunc means the job type accepts any JSON payload.
type ValidateFunc func(payload json.RawMessage) error

// ErrUnknownType is returned by Validate for a job type with no handler.
var ErrUnknownType = errors.New("unknown job type")

type entry struct {
	run      HandlerFunc
	validate ValidateFunc
}

// Registry is the "phone book" from job type to handler.
//
// Register every handler at startup, before the server and workers use the
// registry. After that it is only read, which is safe from many goroutines.
type Registry struct {
	entries map[string]entry
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]entry)}
}

// Register adds the handler for jobType. validate may be nil.
//
// It panics if jobType is empty, run is nil, or jobType is already
// registered: those are programming mistakes, best caught at startup.
func (r *Registry) Register(jobType string, run HandlerFunc, validate ValidateFunc) {
	if jobType == "" {
		panic("handler: job type must not be empty")
	}
	if run == nil {
		panic(fmt.Sprintf("handler %q: handler func is nil", jobType))
	}
	if _, exists := r.entries[jobType]; exists {
		panic(fmt.Sprintf("handler %q already registered", jobType))
	}

	r.entries[jobType] = entry{run: run, validate: validate}
}

// Has reports whether jobType has a registered handler.
func (r *Registry) Has(jobType string) bool {
	_, exists := r.entries[jobType]
	return exists
}

// Validate checks payload with jobType's validate function. It returns an
// error wrapping ErrUnknownType if jobType is not registered.
func (r *Registry) Validate(jobType string, payload json.RawMessage) error {
	e, exists := r.entries[jobType]
	if !exists {
		return fmt.Errorf("%w %q", ErrUnknownType, jobType)
	}
	if e.validate == nil {
		return nil
	}
	return e.validate(payload)
}
