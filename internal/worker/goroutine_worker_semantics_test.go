package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/worker"
)

// Stop must let the job in flight finish. It used to cancel the handlers' context before
// waiting, so a backup interrupted mid-write saw context.Canceled and left a partial file.
func TestWorker_StopWaitsForInFlightJob(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	started := make(chan struct{})
	var finished atomic.Bool
	var sawCancellation atomic.Bool

	w.Register("slow", func(ctx context.Context, _ json.RawMessage) error {
		close(started)
		select {
		case <-time.After(300 * time.Millisecond):
			finished.Store(true)
		case <-ctx.Done():
			sawCancellation.Store(true)
		}
		return nil
	})

	require.NoError(t, w.Start(context.Background()))
	require.NoError(t, w.Enqueue(context.Background(), "slow", nil))

	<-started

	// A shutdown budget comfortably longer than the job.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Stop(ctx))

	require.True(t, finished.Load(), "the job in flight must be allowed to finish")
	require.False(t, sawCancellation.Load(), "the handler must not see a cancelled context during a graceful stop")
}

// When the caller's shutdown budget runs out, the handler does get interrupted — otherwise a
// wedged job would hold shutdown open indefinitely.
func TestWorker_StopInterruptsAfterDeadline(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	started := make(chan struct{})
	var sawCancellation atomic.Bool

	w.Register("stuck", func(ctx context.Context, _ json.RawMessage) error {
		close(started)
		<-ctx.Done()
		sawCancellation.Store(true)
		return nil
	})

	require.NoError(t, w.Start(context.Background()))
	require.NoError(t, w.Enqueue(context.Background(), "stuck", nil))
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := w.Stop(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, sawCancellation.Load(), "an over-budget job must be interrupted")
}

// Enqueue must never block on a full queue: the scheduler calls it from its cron goroutine
// with context.Background(), so a blocked call parks that goroutine and every later job.
func TestWorker_EnqueueDoesNotBlockForeverWhenFull(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	release := make(chan struct{})
	w.Register("blocker", func(_ context.Context, _ json.RawMessage) error {
		<-release
		return nil
	})
	require.NoError(t, w.Start(context.Background()))
	t.Cleanup(func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = w.Stop(ctx)
	})

	// Fill the buffer (capacity 100) plus the one the worker picks up.
	var lastErr error
	for i := 0; i < 200; i++ {
		if lastErr = w.Enqueue(context.Background(), "blocker", nil); lastErr != nil {
			break
		}
	}

	require.ErrorIs(t, lastErr, worker.ErrQueueFull,
		"a full queue must be reported, not waited on")
}

// Enqueue with a background context on a full queue must return promptly. Deliberately
// asserted against the test deadline: the old code had no way to return at all.
func TestWorker_EnqueueReturnsPromptlyWithBackgroundContext(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	release := make(chan struct{})
	w.Register("blocker", func(_ context.Context, _ json.RawMessage) error {
		<-release
		return nil
	})
	require.NoError(t, w.Start(context.Background()))
	t.Cleanup(func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = w.Stop(ctx)
	})

	// Keep enqueueing until the queue reports itself full. Asserting on a single call after a
	// counted fill would be racy: the worker picks one job off the channel at a moment of its
	// own choosing, freeing exactly one slot.
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 10_000; i++ {
			if err := w.Enqueue(context.Background(), "blocker", nil); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, worker.ErrQueueFull,
			"a full queue must be reported rather than waited on")
	case <-time.After(5 * time.Second):
		t.Fatal("Enqueue blocked on a full queue with a background context")
	}
}

// Accepting a job after shutdown began would promise execution that is not going to happen.
func TestWorker_EnqueueAfterStopIsRefused(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())
	w.Register("noop", func(context.Context, json.RawMessage) error { return nil })

	require.NoError(t, w.Start(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Stop(ctx))

	err := w.Enqueue(context.Background(), "noop", nil)
	require.ErrorIs(t, err, worker.ErrWorkerStopped)
}

// Buffered jobs are drained on shutdown rather than dropped.
func TestWorker_StopDrainsBufferedJobs(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	var ran atomic.Int32
	gate := make(chan struct{})
	w.Register("counted", func(_ context.Context, _ json.RawMessage) error {
		ran.Add(1)
		return nil
	})
	w.Register("gate", func(_ context.Context, _ json.RawMessage) error {
		<-gate
		return nil
	})

	require.NoError(t, w.Start(context.Background()))

	// Occupy the single worker, then queue jobs behind it.
	require.NoError(t, w.Enqueue(context.Background(), "gate", nil))
	for i := 0; i < 3; i++ {
		require.NoError(t, w.Enqueue(context.Background(), "counted", nil))
	}

	close(gate)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Stop(ctx))

	require.Equal(t, int32(3), ran.Load(), "buffered jobs must run during shutdown, not be dropped")
}

func TestWorker_QueueDepthReportsBacklog(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	release := make(chan struct{})
	w.Register("blocker", func(_ context.Context, _ json.RawMessage) error {
		<-release
		return nil
	})
	require.NoError(t, w.Start(context.Background()))

	require.NoError(t, w.Enqueue(context.Background(), "blocker", nil))
	// Wait for the worker to actually pick that one up before measuring.
	require.Eventually(t, func() bool { return w.QueueDepth() == 0 }, time.Second, 5*time.Millisecond)

	for i := 0; i < 5; i++ {
		require.NoError(t, w.Enqueue(context.Background(), "blocker", nil))
	}
	require.Equal(t, 5, w.QueueDepth())

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Stop(ctx))
	require.Zero(t, w.QueueDepth())
}

// Register is called from composition-root code and could race a running worker; the map
// write must be guarded. Under -race this fails loudly if it is not.
func TestWorker_RegisterDuringRunIsRaceFree(t *testing.T) {
	w := worker.NewGoroutineWorker(2, zap.NewNop())
	w.Register("noop", func(context.Context, json.RawMessage) error { return nil })
	require.NoError(t, w.Start(context.Background()))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			w.Register("late", func(context.Context, json.RawMessage) error { return nil })
		}
	}()

	for i := 0; i < 50; i++ {
		if err := w.Enqueue(context.Background(), "noop", nil); err != nil && !errors.Is(err, worker.ErrQueueFull) {
			t.Errorf("Enqueue: %v", err)
		}
	}
	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Stop(ctx))
}
