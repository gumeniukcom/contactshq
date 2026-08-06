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
	mu       sync.RWMutex // guards handlers: Register may race with a running worker
	handlers map[string]JobHandler

	jobs    chan job
	wg      sync.WaitGroup
	workers int
	logger  *zap.Logger

	// quit tells the workers to stop taking new jobs. It is deliberately separate from
	// cancelling runCtx: shutdown should let the job in flight finish, not interrupt it.
	quit     chan struct{}
	stopOnce sync.Once

	// runCtx is the context handed to handlers; cancel is only pulled once the caller's
	// shutdown deadline has run out.
	runCtx context.Context
	cancel context.CancelFunc
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
		quit:     make(chan struct{}),
	}
}

func (w *GoroutineWorker) Register(jobType string, handler JobHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[jobType] = handler
}

func (w *GoroutineWorker) handlerFor(jobType string) (JobHandler, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	h, ok := w.handlers[jobType]
	return h, ok
}

// QueueDepth reports how many jobs are waiting to be picked up.
func (w *GoroutineWorker) QueueDepth() int {
	return len(w.jobs)
}

// Enqueue adds a job without ever blocking on a full queue.
//
// The scheduler calls this from its cron goroutine with context.Background(); blocking there
// on a full channel parks that goroutine forever and every later scheduled job with it. A
// full queue is a real overload condition, so it is reported rather than waited out.
func (w *GoroutineWorker) Enqueue(ctx context.Context, jobType string, payload any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case <-w.quit:
		return ErrWorkerStopped
	default:
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	select {
	case w.jobs <- job{jobType: jobType, payload: data}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (w *GoroutineWorker) Start(ctx context.Context) error {
	w.runCtx, w.cancel = context.WithCancel(ctx)

	for i := range w.workers {
		w.wg.Add(1)
		go func(id int) {
			defer w.wg.Done()
			for {
				// Prefer draining a ready job over noticing shutdown, so a job already in
				// the buffer is not skipped just because Stop happened to fire first.
				select {
				case j := <-w.jobs:
					w.run(w.runCtx, id, j)
					continue
				default:
				}

				select {
				case <-w.runCtx.Done():
					return
				case <-w.quit:
					return
				case j := <-w.jobs:
					w.run(w.runCtx, id, j)
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
	handler, ok := w.handlerFor(j.jobType)
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

// Stop stops accepting work, lets the job in flight finish, then runs whatever is still
// buffered. A scheduled backup enqueued moments before shutdown used to be dropped without
// a trace.
//
// The cancellation is deliberately staged. Cancelling the handlers' context up front — as
// this used to — aborted the running job with context.Canceled, so a backup interrupted
// mid-write left a partial file behind; meanwhile jobs drained from the buffer afterwards
// still got a live context, which was the same operation treated two different ways. Now the
// caller's context bounds how long shutdown may take, and only when that runs out are the
// handlers actually interrupted.
func (w *GoroutineWorker) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() { close(w.quit) })

	if w.cancel == nil {
		return nil // never started
	}
	// Whatever happens, the run context must not outlive Stop.
	defer w.cancel()

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		// Out of time: interrupt the handlers and wait for them to unwind.
		w.cancel()
		<-done
		return ctx.Err()
	}

	// Drain what is left, still bounded by the caller's deadline.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case j := <-w.jobs:
			w.run(w.runCtx, -1, j)
		default:
			return nil
		}
	}
}
