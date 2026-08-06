package worker

import (
	"context"
	"errors"
)

var (
	// ErrQueueFull reports that the queue had no room. Enqueue never blocks waiting for
	// space: the cron scheduler calls it from its own goroutine with a background context,
	// so blocking there stalls every future scheduled job, not just this one.
	ErrQueueFull = errors.New("job queue is full")

	// ErrWorkerStopped reports an enqueue after shutdown began. Accepting the job would
	// promise execution that is no longer going to happen.
	ErrWorkerStopped = errors.New("worker is stopping")
)

type TaskWorker interface {
	Enqueue(ctx context.Context, jobType string, payload any) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// QueueDepth reports how many jobs are waiting. Queue state was entirely unobservable
	// before; any durable implementation must be able to answer this too.
	QueueDepth() int
}
