package job

import "time"


type JobStatus string

const (
	StatusPending JobStatus = "pending"
	StatusRunning JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed JobStatus = "failed"
	StatusDead JobStatus = "dead"
)

type Job struct {
	ID string `json:"id"`
	Type string `json:"type"`
	Payload []byte `json:"payload"`
	Status JobStatus `json:"status"`
	Retries int `json:"retries"`
	MaxRetries int `json:"max_retries"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Error string `json:"error"`
}


func (j *Job) MarkRunning() {
	j.Status=StatusRunning
	j.Error=""
}


func (j *Job) MarkCompleted() {
	j.Status=StatusCompleted
}

func (j *Job) MarkFailed(err error){
	j.Status=StatusFailed
	j.Error=err.Error()
}

func (j *Job) CanRetry() bool {
	return j.Retries < j.MaxRetries
}
