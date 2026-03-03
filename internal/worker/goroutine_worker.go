package worker

import (
	"context"
	"encoding/json"
	"sync"

	"go.uber.org/zap"
)

type JobHandler func(ctx context.Context, payload json.RawMessage) error

type GoroutineWorker struct {
	handlers map[string]JobHandler
	jobs     chan job
	wg       sync.WaitGroup
	workers  int
	cancel   context.CancelFunc
	logger   *zap.Logger
}

type job struct {
	jobType string
	payload json.RawMessage
}

func NewGoroutineWorker(workers int, logger *zap.Logger) *GoroutineWorker {
	if workers <= 0 {
		workers = 4
	}
	return &GoroutineWorker{
		handlers: make(map[string]JobHandler),
		jobs:     make(chan job, 100),
		workers:  workers,
		logger:   logger,
	}
}

func (w *GoroutineWorker) Register(jobType string, handler JobHandler) {
	w.handlers[jobType] = handler
}

func (w *GoroutineWorker) Enqueue(ctx context.Context, jobType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	select {
	case w.jobs <- job{jobType: jobType, payload: data}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *GoroutineWorker) Start(ctx context.Context) error {
	ctx, w.cancel = context.WithCancel(ctx)

	for i := range w.workers {
		w.wg.Add(1)
		go func(id int) {
			defer w.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-w.jobs:
					handler, ok := w.handlers[j.jobType]
					if !ok {
						w.logger.Error("unknown job type", zap.Int("worker_id", id), zap.String("job_type", j.jobType))
						continue
					}
					if err := handler(ctx, j.payload); err != nil {
						w.logger.Error("job failed", zap.Int("worker_id", id), zap.String("job_type", j.jobType), zap.Error(err))
					}
				}
			}
		}(i)
	}

	return nil
}

func (w *GoroutineWorker) Stop(ctx context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	return nil
}
