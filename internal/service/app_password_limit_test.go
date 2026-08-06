package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type memAppPasswordRepo struct {
	mu      sync.Mutex
	records []domain.AppPassword
	creates int
}

func (r *memAppPasswordRepo) Create(_ context.Context, ap *domain.AppPassword) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creates++
	r.records = append(r.records, *ap)
	return nil
}

func (r *memAppPasswordRepo) ListByUser(ctx context.Context, userID string) ([]domain.AppPassword, error) {
	return r.ListAllByUser(ctx, userID)
}

func (r *memAppPasswordRepo) ListAllByUser(_ context.Context, userID string) ([]domain.AppPassword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.AppPassword
	for _, ap := range r.records {
		if ap.UserID == userID {
			out = append(out, ap)
		}
	}
	return out, nil
}

func (r *memAppPasswordRepo) GetByID(_ context.Context, id string) (*domain.AppPassword, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.records {
		if r.records[i].ID == id {
			return &r.records[i], nil
		}
	}
	return nil, nil
}

func (r *memAppPasswordRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.records {
		if r.records[i].ID == id {
			r.records = append(r.records[:i], r.records[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *memAppPasswordRepo) UpdateLastUsed(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for i := range r.records {
		if r.records[i].ID == id {
			r.records[i].LastUsedAt = &now
		}
	}
	return nil
}

// Every failed CardDAV auth hashes once per stored app password, so the list has to be
// bounded somewhere. Creation is the right place: it is the only path a user controls.
func TestAppPassword_CreateIsCappedPerUser(t *testing.T) {
	repo := &memAppPasswordRepo{}
	svc := service.NewAppPasswordService(repo)
	ctx := context.Background()

	for i := 0; i < service.MaxAppPasswordsPerUser; i++ {
		_, _, err := svc.Create(ctx, "u1", "device")
		require.NoError(t, err, "creation %d should succeed", i+1)
	}

	_, _, err := svc.Create(ctx, "u1", "one too many")
	require.ErrorIs(t, err, service.ErrTooManyAppPasswords)
	require.Equal(t, service.MaxAppPasswordsPerUser, repo.creates,
		"a rejected creation must not reach the repository")
}

// Deleting one frees a slot — the cap must not be a one-way door.
func TestAppPassword_DeletingFreesASlot(t *testing.T) {
	repo := &memAppPasswordRepo{}
	svc := service.NewAppPasswordService(repo)
	ctx := context.Background()

	for i := 0; i < service.MaxAppPasswordsPerUser; i++ {
		_, _, err := svc.Create(ctx, "u1", "device")
		require.NoError(t, err)
	}
	_, _, err := svc.Create(ctx, "u1", "blocked")
	require.ErrorIs(t, err, service.ErrTooManyAppPasswords)

	list, err := svc.List(ctx, "u1")
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, "u1", list[0].ID))

	_, _, err = svc.Create(ctx, "u1", "now fits")
	require.NoError(t, err)
}

// A revoked app password must stop working immediately. Without the callback the CardDAV
// verdict cache keeps a positive for (email, sha256(password)) for up to five minutes, so the
// user is told access is revoked while the device keeps syncing.
func TestAppPassword_DeleteInvalidatesTheCredentialCache(t *testing.T) {
	repo := &memAppPasswordRepo{}
	userRepo := &stubUserRepo{user: &domain.User{ID: "u1", Email: "owner@example.com"}}

	var invalidated []string
	svc := service.NewAppPasswordService(repo).
		WithCredentialInvalidator(userRepo, func(email string) { invalidated = append(invalidated, email) })

	ctx := context.Background()
	_, ap, err := svc.Create(ctx, "u1", "phone")
	require.NoError(t, err)

	require.NoError(t, svc.Delete(ctx, "u1", ap.ID))
	require.Equal(t, []string{"owner@example.com"}, invalidated)
}

// A delete that does not happen must not invalidate anything either.
func TestAppPassword_FailedDeleteDoesNotInvalidate(t *testing.T) {
	repo := &memAppPasswordRepo{}
	userRepo := &stubUserRepo{user: &domain.User{ID: "u1", Email: "owner@example.com"}}

	var invalidated []string
	svc := service.NewAppPasswordService(repo).
		WithCredentialInvalidator(userRepo, func(email string) { invalidated = append(invalidated, email) })

	ctx := context.Background()
	_, ap, err := svc.Create(ctx, "u1", "phone")
	require.NoError(t, err)

	// Another user's id: the record must not be deleted and nothing invalidated.
	err = svc.Delete(ctx, "someone-else", ap.ID)
	require.ErrorIs(t, err, service.ErrAppPasswordNotFound)
	require.Empty(t, invalidated)
}

type stubUserRepo struct {
	user *domain.User
}

func (r *stubUserRepo) Create(context.Context, *domain.User) error { return nil }
func (r *stubUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	if r.user != nil && r.user.ID == id {
		return r.user, nil
	}
	return nil, nil
}
func (r *stubUserRepo) GetByEmail(context.Context, string) (*domain.User, error) { return nil, nil }
func (r *stubUserRepo) Update(context.Context, *domain.User) error               { return nil }
func (r *stubUserRepo) Delete(context.Context, string) error                     { return nil }
func (r *stubUserRepo) List(context.Context, int, int) ([]*domain.User, int, error) {
	return nil, 0, nil
}
func (r *stubUserRepo) ListAllIDs(context.Context) ([]string, error) { return nil, nil }

// One account filling its quota must not stop anyone else creating passwords.
func TestAppPassword_CapIsPerUser(t *testing.T) {
	repo := &memAppPasswordRepo{}
	svc := service.NewAppPasswordService(repo)
	ctx := context.Background()

	for i := 0; i < service.MaxAppPasswordsPerUser; i++ {
		_, _, err := svc.Create(ctx, "u1", "device")
		require.NoError(t, err)
	}
	_, _, err := svc.Create(ctx, "u1", "blocked")
	require.ErrorIs(t, err, service.ErrTooManyAppPasswords)

	_, _, err = svc.Create(ctx, "u2", "first for this user")
	require.NoError(t, err, "another account has its own quota")
}
