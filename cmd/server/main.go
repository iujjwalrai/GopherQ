package main

import (
	"fmt"
	"github.com/ujjwal/gopherq/internal/job"
)


func main(){
	j:= job.Job{
		ID: "job-101",
		Type: "email",
		Status: job.StatusPending,
		MaxRetries: 3,
	}

	fmt.Println(j)
	fmt.Println("Job ID:", j.ID)
    fmt.Println("Job Status:", j.Status)
}