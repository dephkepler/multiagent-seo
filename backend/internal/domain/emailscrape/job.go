package emailscrape

import (
	"context"
	"errors"
)

var ErrJobNotFound = errors.New("scrape job not found")

const (
	JobRunning  = "running"
	JobDone     = "done"
	JobCanceled = "canceled"
	JobFailed   = "failed"
)

type Job struct {
	ID        string
	Sheet     string
	State     string
	Total     int
	Processed int
}

type JobStore interface {
	Create(ctx context.Context, sheet string, total int) (string, error)
	UpdateProgress(ctx context.Context, id string, processed int) error
	Finish(ctx context.Context, id, state, errMsg string) error
	Get(ctx context.Context, id string) (Job, error)
}
