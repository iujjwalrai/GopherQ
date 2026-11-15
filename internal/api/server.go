package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ujjwal/gopherq/internal/job"
	"github.com/ujjwal/gopherq/internal/store"
)

// maxIDAttempts is how many fresh IDs createJob tries if they collide.
const maxIDAttempts = 3

type Server struct {
	store store.Store
}

// NewServer returns a Server that keeps jobs in s.
func NewServer(s store.Store) *Server {
	return &Server{store: s}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /jobs", s.createJob)
	mux.HandleFunc("GET /jobs", s.listJobs)
	mux.HandleFunc("GET /jobs/{id}", s.getJob)

	return loggingMiddleware(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

type createJobRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req createJobRequest

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if req.Type == "" || len(req.Payload) == 0 ||
		!json.Valid(req.Payload) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "type and valid payload are required",
		})
		return
	}

	now := time.Now().UTC()
	j := job.Job{
		Type:       req.Type,
		Payload:    req.Payload,
		Status:     job.StatusPending,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.createWithUniqueID(r.Context(), &j); err != nil {
		log.Printf("create job: %v", err)
		writeInternalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, j)
}

// createWithUniqueID gives j a random ID and saves it. If the ID is
// already taken, it tries again with a new one, up to maxIDAttempts times.
func (s *Server) createWithUniqueID(ctx context.Context, j *job.Job) error {
	var err error

	for range maxIDAttempts {
		j.ID, err = generateID()
		if err != nil {
			return fmt.Errorf("generate ID: %w", err)
		}

		err = s.store.Create(ctx, *j)
		if !errors.Is(err, store.ErrAlreadyExists) {
			return err // saved (nil), or failed for another reason
		}
	}

	return fmt.Errorf("no unique ID after %d attempts: %w", maxIDAttempts, err)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.List(r.Context())
	if err != nil {
		log.Printf("list jobs: %v", err)
		writeInternalError(w)
		return
	}

	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	j, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "job not found",
		})
		return
	}
	if err != nil {
		log.Printf("get job %s: %v", id, err)
		writeInternalError(w)
		return
	}

	writeJSON(w, http.StatusOK, j)
}

func generateID() (string, error) {
	b := make([]byte, 16)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// writeInternalError sends a generic 500. The real error is logged by the
// caller and never shown to the client.
func writeInternalError(w http.ResponseWriter) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "internal server error",
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		log.Printf(
			"%s %s %s",
			r.Method,
			r.URL.Path,
			time.Since(start),
		)
	})
}
