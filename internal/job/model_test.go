package job

import (
	"errors"
	"testing"
)


func TestJobLifecycle(t *testing.T){
	j:= Job{
		ID: "job-001",
		Status: StatusPending,
		MaxRetries: 3,
	}

	j.MarkRunning()

	if j.Status!=StatusRunning{
		t.Fatal("expected Running Status")
	}
	j.MarkCompleted()
	if j.Status!=StatusCompleted{
		t.Fatal("expected Completed Status")
	}
}

func TestJobFailure(t *testing.T){
	j:=Job{}

	j.MarkFailed(errors.New("email service unavailable"))

	if(j.Status!=StatusFailed){
		t.Fatal("expected failed status")
	}
	if j.Error!="email service unavailable"{
		t.Fatal("unaccepted error message")
	}


}

func TestCanRetry(t *testing.T) {
    j := Job{
        Retries:    1,
        MaxRetries: 3,
    }

    if !j.CanRetry() {
        t.Fatal("expected job to be retryable")
    }
}