package carddav

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

// hashForTest produces the same encoding the auth service writes.
func hashForTest(t *testing.T, password string) string {
	t.Helper()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		64*1024, 1, 4,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash))
}

type stubAppPwRepo struct {
	records []domain.AppPassword
}

func (r *stubAppPwRepo) Create(context.Context, *domain.AppPassword) error { return nil }
func (r *stubAppPwRepo) ListByUser(context.Context, string) ([]domain.AppPassword, error) {
	return r.records, nil
}
func (r *stubAppPwRepo) ListAllByUser(context.Context, string) ([]domain.AppPassword, error) {
	return r.records, nil
}
func (r *stubAppPwRepo) GetByID(context.Context, string) (*domain.AppPassword, error) {
	return nil, nil
}
func (r *stubAppPwRepo) Delete(context.Context, string) error         { return nil }
func (r *stubAppPwRepo) UpdateLastUsed(context.Context, string) error { return nil }

// An installation upgraded from before the creation cap can hold more passwords than the cap
// allows. Truncating the verification loop at the cap — an obvious way to bound the work —
// would silently revoke access for whichever clients happened to sort last, because
// ListAllByUser returns rows in no defined order. The loop must stay complete; the cost is
// bounded by the creation cap going forward and by the argon2 semaphore right now.
func TestVerifyAppPassword_AccountOverTheCapStillAuthenticates(t *testing.T) {
	const total = 25

	repo := &stubAppPwRepo{}
	for i := 0; i < total; i++ {
		repo.records = append(repo.records, domain.AppPassword{
			ID:           fmt.Sprintf("ap-%02d", i),
			UserID:       "u1",
			PasswordHash: hashForTest(t, fmt.Sprintf("token-%02d", i)),
		})
	}

	s := &Server{
		appPwRepo:   repo,
		authCache:   newAuthCache(),
		throttle:    newAuthThrottle(),
		trusted:     newTrustedProxySet(nil),
		argon2Slots: make(chan struct{}, authArgon2Concurrency),
	}

	// The last one is the interesting case: any truncation would drop it.
	if !s.verifyAppPassword(context.Background(), "u1", fmt.Sprintf("token-%02d", total-1)) {
		t.Fatalf("password %d of %d failed to authenticate — the loop is truncated", total, total)
	}
	if !s.verifyAppPassword(context.Background(), "u1", "token-00") {
		t.Fatal("the first password failed to authenticate")
	}
	if s.verifyAppPassword(context.Background(), "u1", "not-a-real-token") {
		t.Fatal("an unknown token authenticated")
	}
}

// Recently-used passwords are tried first, so the common case hashes once rather than walking
// the whole list.
func TestVerifyAppPassword_MostRecentlyUsedIsTriedFirst(t *testing.T) {
	recent := time.Now()
	older := recent.Add(-24 * time.Hour)

	repo := &stubAppPwRepo{records: []domain.AppPassword{
		{ID: "never", UserID: "u1", PasswordHash: hashForTest(t, "token-never")},
		{ID: "old", UserID: "u1", PasswordHash: hashForTest(t, "token-old"), LastUsedAt: &older},
		{ID: "recent", UserID: "u1", PasswordHash: hashForTest(t, "token-recent"), LastUsedAt: &recent},
	}}

	s := &Server{
		appPwRepo:   repo,
		authCache:   newAuthCache(),
		throttle:    newAuthThrottle(),
		trusted:     newTrustedProxySet(nil),
		argon2Slots: make(chan struct{}, authArgon2Concurrency),
	}

	// All three must still verify; ordering is an optimisation, not a filter.
	for _, token := range []string{"token-recent", "token-old", "token-never"} {
		if !s.verifyAppPassword(context.Background(), "u1", token) {
			t.Fatalf("%s failed to authenticate", token)
		}
	}
}
