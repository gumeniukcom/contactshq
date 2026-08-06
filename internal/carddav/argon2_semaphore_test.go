package carddav

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

// blockingUserRepo parks inside GetByEmail so a test can observe how many verifications are
// in flight at once.
type blockingUserRepo struct {
	inFlight atomic.Int32
	peak     atomic.Int32
	release  chan struct{}
	entered  chan struct{}
}

func (r *blockingUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	cur := r.inFlight.Add(1)
	for {
		peak := r.peak.Load()
		if cur <= peak || r.peak.CompareAndSwap(peak, cur) {
			break
		}
	}
	r.entered <- struct{}{}
	<-r.release
	r.inFlight.Add(-1)
	return nil, nil // no such user: the caller returns a negative verdict without hashing
}

func (r *blockingUserRepo) Create(context.Context, *domain.User) error { return nil }
func (r *blockingUserRepo) GetByID(context.Context, string) (*domain.User, error) {
	return nil, nil
}
func (r *blockingUserRepo) Update(context.Context, *domain.User) error { return nil }
func (r *blockingUserRepo) Delete(context.Context, string) error       { return nil }
func (r *blockingUserRepo) List(context.Context, int, int) ([]*domain.User, int, error) {
	return nil, 0, nil
}
func (r *blockingUserRepo) ListAllIDs(context.Context) ([]string, error) { return nil, nil }

// The semaphore is what actually bounds memory: argon2id here is configured at 64 MiB, so
// without a cap the peak is 64 MiB × concurrent requests, and nothing else in the /dav path
// limits that.
func TestVerifyCredentials_ConcurrencyIsCapped(t *testing.T) {
	repo := &blockingUserRepo{
		release: make(chan struct{}),
		entered: make(chan struct{}, 64),
	}

	s := &Server{
		userRepo:    repo,
		authCache:   newAuthCache(),
		throttle:    newAuthThrottle(),
		trusted:     newTrustedProxySet(nil),
		argon2Slots: make(chan struct{}, authArgon2Concurrency),
	}

	const attempts = 32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.verifyCredentials(context.Background(), "nobody@example.com", "pw")
		}(i)
	}

	// Wait until the slots are saturated, then confirm nothing more got in.
	for i := 0; i < authArgon2Concurrency; i++ {
		select {
		case <-repo.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("verifications never started")
		}
	}
	time.Sleep(100 * time.Millisecond) // give any excess goroutine a chance to slip through

	if got := repo.inFlight.Load(); got > authArgon2Concurrency {
		t.Fatalf("%d verifications in flight, cap is %d", got, authArgon2Concurrency)
	}

	close(repo.release)
	wg.Wait()

	if peak := repo.peak.Load(); peak > authArgon2Concurrency {
		t.Fatalf("peak concurrency was %d, cap is %d", peak, authArgon2Concurrency)
	}
}

// A caller that goes away must not keep holding a slot.
func TestVerifyCredentials_CancelledContextDoesNotTakeASlot(t *testing.T) {
	s := &Server{
		userRepo:    &blockingUserRepo{release: make(chan struct{}), entered: make(chan struct{}, 1)},
		authCache:   newAuthCache(),
		throttle:    newAuthThrottle(),
		trusted:     newTrustedProxySet(nil),
		argon2Slots: make(chan struct{}, authArgon2Concurrency),
	}

	// Fill every slot.
	for i := 0; i < authArgon2Concurrency; i++ {
		s.argon2Slots <- struct{}{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan authVerdict, 1)
	go func() { done <- s.verifyCredentials(ctx, "nobody@example.com", "pw") }()

	select {
	case v := <-done:
		if v.ok {
			t.Fatal("a cancelled request must not authenticate")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a cancelled request waited for a slot instead of giving up")
	}
}
