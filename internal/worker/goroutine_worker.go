package worker

import (
	"context"
	"encoding/json"
	"runtime/debug"
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
					w.run(ctx, id, j)
				}
			}
		}(i)
	}

	return nil
}

// run executes one job. Handlers parse vCards and provider responses from outside the
// system, and Fiber's recover middleware only covers HTTP handlers, so a panic in here
// would otherwise take the whole server down.
func (w *GoroutineWorker) run(ctx context.Context, workerID int, j job) {
	handler, ok := w.handlers[j.jobType]
	if !ok {
		w.logger.Error("unknown job type", zap.Int("worker_id", workerID), zap.String("job_type", j.jobType))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("job panicked",
				zap.Int("worker_id", workerID),
				zap.String("job_type", j.jobType),
				zap.Any("panic", r),
				zap.ByteString("stack", debug.Stack()),
			)
		}
	}()

	if err := handler(ctx, j.payload); err != nil {
		w.logger.Error("job failed", zap.Int("worker_id", workerID), zap.String("job_type", j.jobType), zap.Error(err))
	}
}

// Stop cancels the workers and waits for the job in flight to finish, then runs whatever
// is still buffered. A scheduled backup enqueued moments before shutdown used to be
// dropped without a trace.
func (w *GoroutineWorker) Stop(ctx context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()

	for {
		// The caller bounds how long draining may take by the context it passes.
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case j := <-w.jobs:
			w.run(ctx, -1, j)
		default:
			return nil
		}
	}
}
