package job

import (
	"errors"
	"testing"
	"time"
)

func TestTransitions(t *testing.T) {
	run := func(j *Job) error { return j.MarkRunning() }
	complete := func(j *Job) error { return j.MarkCompleted() }
	fail := func(j *Job) error { return j.MarkFailed(errors.New("boom")) }
	kill := func(j *Job) error { return j.MarkDead() }

	tests := []struct {
		name       string
		start      JobStatus
		retries    int
		maxRetries int
		action     func(j *Job) error
		wantStatus JobStatus
		wantErr    error
	}{
		// Allowed moves.
		{name: "pending to running", start: StatusPending, maxRetries: 3,
			action: run, wantStatus: StatusRunning},
		{name: "running to completed", start: StatusRunning, maxRetries: 3,
			action: complete, wantStatus: StatusCompleted},
		{name: "running to failed", start: StatusRunning, maxRetries: 3,
			action: fail, wantStatus: StatusFailed},
		{name: "failed to running (retry)", start: StatusFailed, maxRetries: 3,
			action: run, wantStatus: StatusRunning},
		{name: "failed to dead", start: StatusFailed, retries: 3, maxRetries: 3,
			action: kill, wantStatus: StatusDead},

		// Rejected moves: the status must stay the same.
		{name: "retry with no retries left", start: StatusFailed, retries: 3, maxRetries: 3,
			action: run, wantStatus: StatusFailed, wantErr: ErrNoRetriesLeft},
		{name: "pending to completed", start: StatusPending, maxRetries: 3,
			action: complete, wantStatus: StatusPending, wantErr: ErrInvalidTransition},
		{name: "completed to running", start: StatusCompleted, maxRetries: 3,
			action: run, wantStatus: StatusCompleted, wantErr: ErrInvalidTransition},
		{name: "dead to running", start: StatusDead, retries: 3, maxRetries: 3,
			action: run, wantStatus: StatusDead, wantErr: ErrInvalidTransition},
		{name: "empty status to failed", start: "", maxRetries: 3,
			action: fail, wantStatus: "", wantErr: ErrInvalidTransition},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := Job{Status: tc.start, Retries: tc.retries, MaxRetries: tc.maxRetries}

			err := tc.action(&j)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if j.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", j.Status, tc.wantStatus)
			}
			if tc.wantErr != nil && !j.UpdatedAt.IsZero() {
				t.Fatal("UpdatedAt changed even though the move was rejected")
			}
			if tc.wantErr == nil && j.UpdatedAt.IsZero() {
				t.Fatal("UpdatedAt was not set")
			}
		})
	}
}

// TestRetryUntilDead walks one job through every retry until it dies.
func TestRetryUntilDead(t *testing.T) {
	j := Job{Status: StatusPending, MaxRetries: 3}
	mustSucceed(t, j.MarkRunning())

	for want := 1; want <= 3; want++ {
		mustSucceed(t, j.MarkFailed(errors.New("smtp timeout")))
		mustSucceed(t, j.MarkRunning())

		if j.Retries != want {
			t.Fatalf("retries = %d, want %d", j.Retries, want)
		}
		if j.Error != "" {
			t.Fatalf("error should be cleared on retry, got %q", j.Error)
		}
	}

	mustSucceed(t, j.MarkFailed(errors.New("smtp timeout")))

	if j.CanRetry() {
		t.Fatal("expected no retries left")
	}
	if err := j.MarkRunning(); !errors.Is(err, ErrNoRetriesLeft) {
		t.Fatalf("error = %v, want ErrNoRetriesLeft", err)
	}

	mustSucceed(t, j.MarkDead())

	if j.Status != StatusDead {
		t.Fatalf("status = %q, want %q", j.Status, StatusDead)
	}
}

func TestMarkFailedRecordsError(t *testing.T) {
	j := Job{Status: StatusRunning}

	mustSucceed(t, j.MarkFailed(errors.New("email service unavailable")))

	if j.Error != "email service unavailable" {
		t.Fatalf("error = %q, want %q", j.Error, "email service unavailable")
	}
}

func TestMarkFailedWithNilError(t *testing.T) {
	j := Job{Status: StatusRunning}

	mustSucceed(t, j.MarkFailed(nil))

	if j.Error != "unknown error" {
		t.Fatalf("error = %q, want %q", j.Error, "unknown error")
	}
}

func TestUpdatedAtIsCurrentUTC(t *testing.T) {
	before := time.Now()
	j := Job{Status: StatusPending}

	mustSucceed(t, j.MarkRunning())

	if j.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt %v is earlier than %v", j.UpdatedAt, before)
	}
	if j.UpdatedAt.Location() != time.UTC {
		t.Fatalf("UpdatedAt location = %v, want UTC", j.UpdatedAt.Location())
	}
}

func TestCanRetry(t *testing.T) {
	tests := []struct {
		retries    int
		maxRetries int
		want       bool
	}{
		{retries: 0, maxRetries: 3, want: true},
		{retries: 2, maxRetries: 3, want: true},
		{retries: 3, maxRetries: 3, want: false},
		{retries: 0, maxRetries: 0, want: false},
	}

	for _, tc := range tests {
		j := Job{Retries: tc.retries, MaxRetries: tc.maxRetries}

		if got := j.CanRetry(); got != tc.want {
			t.Errorf("CanRetry() with %d/%d = %v, want %v",
				tc.retries, tc.maxRetries, got, tc.want)
		}
	}
}

// mustSucceed stops the test if err is not nil.
func mustSucceed(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
