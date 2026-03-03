package worker

import "context"

type TaskWorker interface {
	Enqueue(ctx context.Context, jobType string, payload any) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
