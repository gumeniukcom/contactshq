package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/worker"
)

// waitFor polls until cond holds, so tests never sleep for a fixed duration.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

// A panicking handler must not take the process down, and the worker must keep serving.
func TestWorker_PanicInHandlerDoesNotKillWorker(t *testing.T) {
	ctx := context.Background()
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	var mu sync.Mutex
	var survived int

	w.Register("boom", func(context.Context, json.RawMessage) error {
		panic("provider returned nonsense")
	})
	w.Register("ok", func(context.Context, json.RawMessage) error {
		mu.Lock()
		survived++
		mu.Unlock()
		return nil
	})

	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { _ = w.Stop(ctx) })

	require.NoError(t, w.Enqueue(ctx, "boom", map[string]string{}))
	require.NoError(t, w.Enqueue(ctx, "ok", map[string]string{}))

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return survived == 1
	}, "the worker to process a job after a panicking one")
}

func TestWorker_HandlerErrorDoesNotStopWorker(t *testing.T) {
	ctx := context.Background()
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	var mu sync.Mutex
	calls := 0

	w.Register("job", func(context.Context, json.RawMessage) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return errors.New("nope")
	})

	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { _ = w.Stop(ctx) })

	require.NoError(t, w.Enqueue(ctx, "job", nil))
	require.NoError(t, w.Enqueue(ctx, "job", nil))

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return calls == 2
	}, "both failing jobs to run")
}

// A backup scheduled moments before shutdown used to vanish with the buffered channel.
func TestWorker_StopDrainsQueuedJobs(t *testing.T) {
	ctx := context.Background()
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	var mu sync.Mutex
	var ran []string
	release := make(chan struct{})

	w.Register("slow", func(context.Context, json.RawMessage) error {
		<-release
		mu.Lock()
		ran = append(ran, "slow")
		mu.Unlock()
		return nil
	})
	w.Register("queued", func(context.Context, json.RawMessage) error {
		mu.Lock()
		ran = append(ran, "queued")
		mu.Unlock()
		return nil
	})

	require.NoError(t, w.Start(ctx))

	// Occupy the single worker, then queue two jobs behind it.
	require.NoError(t, w.Enqueue(ctx, "slow", nil))
	require.NoError(t, w.Enqueue(ctx, "queued", nil))
	require.NoError(t, w.Enqueue(ctx, "queued", nil))

	close(release)
	require.NoError(t, w.Stop(ctx))

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, ran, 3, "queued jobs must run rather than be discarded on shutdown")
}

// Draining is bounded by the context the caller hands to Stop.
func TestWorker_StopRespectsContextDeadline(t *testing.T) {
	w := worker.NewGoroutineWorker(1, zap.NewNop())
	w.Register("never", func(context.Context, json.RawMessage) error { return nil })

	require.NoError(t, w.Start(context.Background()))
	require.NoError(t, w.Enqueue(context.Background(), "never", nil))

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.Stop(cancelled)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWorker_UnknownJobTypeIsIgnored(t *testing.T) {
	ctx := context.Background()
	w := worker.NewGoroutineWorker(1, zap.NewNop())

	var mu sync.Mutex
	ran := false
	w.Register("known", func(context.Context, json.RawMessage) error {
		mu.Lock()
		ran = true
		mu.Unlock()
		return nil
	})

	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { _ = w.Stop(ctx) })

	require.NoError(t, w.Enqueue(ctx, "nonexistent", nil))
	require.NoError(t, w.Enqueue(ctx, "known", nil))

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return ran
	}, "the known job to run after an unknown one")
}
