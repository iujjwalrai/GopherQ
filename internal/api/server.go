package api

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "log"
    "net/http"
    "sync"
    "time"

    "github.com/ujjwal/gopherq/internal/job"
)

type Server struct {
    jobs map[string]job.Job
    mu   sync.RWMutex
}

func NewServer() *Server {
    return &Server{
        jobs: make(map[string]job.Job),
    }
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

    id, err := generateID()
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{
            "error": "could not generate job ID",
        })
        return
    }

    j := job.Job{
        ID:         id,
        Type:       req.Type,
        Payload:    req.Payload,
        Status:     job.StatusPending,
        MaxRetries: 3,
        CreatedAt:  time.Now().UTC(),
    }

    s.mu.Lock()
    s.jobs[j.ID] = j
    s.mu.Unlock()

    writeJSON(w, http.StatusCreated, j)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
    s.mu.RLock()

    jobs := make([]job.Job, 0, len(s.jobs))

    for _, j := range s.jobs {
        jobs = append(jobs, j)
    }

    s.mu.RUnlock()

    writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")

    s.mu.RLock()
    j, exists := s.jobs[id]
    s.mu.RUnlock()

    if !exists {
        writeJSON(w, http.StatusNotFound, map[string]string{
            "error": "job not found",
        })
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