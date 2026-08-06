package carddav_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	chqcarddav "github.com/gumeniukcom/contactshq/internal/carddav"
	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// countingUserRepo counts GetByEmail calls, which is the observable proxy for "did this
// request reach the argon2id path".
type countingUserRepo struct {
	repository.UserRepository
	lookups atomic.Int32
}

func (r *countingUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	r.lookups.Add(1)
	return r.UserRepository.GetByEmail(ctx, email)
}

func setupThrottledServer(t *testing.T, trustedProxies []string) (*chqcarddav.Server, *countingUserRepo) {
	t.Helper()
	ctx := context.Background()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, repository.Migrate(ctx, db))

	baseUserRepo := repository.NewBunUserRepository(db)
	userRepo := &countingUserRepo{UserRepository: baseUserRepo}
	abRepo := repository.NewBunAddressBookRepository(db)
	contactRepo := repository.NewBunContactRepository(db)
	appPwRepo := repository.NewBunAppPasswordRepository(db)

	authSvc := service.NewAuthService(baseUserRepo, abRepo, config.AuthConfig{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		TokenTTL:  time.Hour,
	})
	_, err = authSvc.Register(ctx, testEmail, testPassword, "Test User")
	require.NoError(t, err)

	backend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, davPrefix)
	srv := chqcarddav.NewServerWithTrustedProxies(backend, userRepo, appPwRepo, davPrefix, trustedProxies)

	// Registration itself performed a lookup; start counting from the requests under test.
	userRepo.lookups.Store(0)
	return srv, userRepo
}

func davRequest(t *testing.T, srv *chqcarddav.Server, remoteAddr, forwarded, password string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, davPrefix+"/"+testEmail+"/addressbooks/contacts/", nil)
	req.RemoteAddr = remoteAddr
	req.SetBasicAuth(testEmail, password)
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Code
}

// The headline property: a source that keeps guessing stops costing argon2id work.
func TestDavAuth_ThrottleStopsHashingAfterTheLimit(t *testing.T) {
	srv, userRepo := setupThrottledServer(t, nil)

	// Ten distinct wrong passwords, each a cache miss and therefore a real hash.
	for i := 0; i < 10; i++ {
		code := davRequest(t, srv, "203.0.113.9:5000", "", "wrong-password-"+string(rune('a'+i)))
		require.Equal(t, http.StatusUnauthorized, code, "attempt %d", i+1)
	}
	require.Equal(t, int32(10), userRepo.lookups.Load(),
		"each distinct wrong password should have reached the verification path exactly once")

	// The eleventh is refused without touching the database or the hash.
	code := davRequest(t, srv, "203.0.113.9:5000", "", "wrong-password-k")
	require.Equal(t, http.StatusTooManyRequests, code)
	require.Equal(t, int32(10), userRepo.lookups.Load(),
		"a throttled request must not reach the verification path")
}

// A blocked source must not take anyone else down with it.
func TestDavAuth_ThrottleIsPerAddress(t *testing.T) {
	srv, _ := setupThrottledServer(t, nil)

	for i := 0; i < 10; i++ {
		davRequest(t, srv, "203.0.113.9:5000", "", "wrong-"+string(rune('a'+i)))
	}
	require.Equal(t, http.StatusTooManyRequests,
		davRequest(t, srv, "203.0.113.9:5000", "", "wrong-x"))

	// Another address, correct credentials: unaffected.
	require.NotEqual(t, http.StatusTooManyRequests,
		davRequest(t, srv, "198.51.100.4:5000", "", testPassword),
		"a legitimate client must not be blocked because another address was")
}

// A client with working credentials must keep working indefinitely — the positive cache is
// consulted before the throttle, so it never accumulates failures.
func TestDavAuth_LegitimateClientIsNeverThrottled(t *testing.T) {
	srv, userRepo := setupThrottledServer(t, nil)

	for i := 0; i < 100; i++ {
		code := davRequest(t, srv, "198.51.100.4:5000", "", testPassword)
		require.NotEqual(t, http.StatusTooManyRequests, code, "request %d", i+1)
		require.NotEqual(t, http.StatusUnauthorized, code, "request %d", i+1)
	}

	require.Equal(t, int32(1), userRepo.lookups.Load(),
		"a repeat sync session should hash once, then ride the verdict cache")
}

// Behind a reverse proxy every client shares one TCP peer, so without X-Forwarded-For they
// would all share one bucket — one misconfigured phone would lock out the household.
func TestDavAuth_TrustedProxySeparatesClients(t *testing.T) {
	srv, _ := setupThrottledServer(t, []string{"10.0.0.1"})

	for i := 0; i < 10; i++ {
		davRequest(t, srv, "10.0.0.1:5000", "198.51.100.7", "wrong-"+string(rune('a'+i)))
	}
	require.Equal(t, http.StatusTooManyRequests,
		davRequest(t, srv, "10.0.0.1:5000", "198.51.100.7", "wrong-x"))

	// A different forwarded client through the same proxy has its own budget.
	require.NotEqual(t, http.StatusTooManyRequests,
		davRequest(t, srv, "10.0.0.1:5000", "203.0.113.44", testPassword))
}

// An untrusted peer's X-Forwarded-For is attacker-controlled: honouring it would hand out a
// fresh bucket per request and defeat the throttle entirely.
func TestDavAuth_UntrustedForwardedHeaderCannotMintBuckets(t *testing.T) {
	srv, _ := setupThrottledServer(t, nil) // no proxies trusted

	for i := 0; i < 10; i++ {
		// A different claimed client every time.
		davRequest(t, srv, "203.0.113.9:5000", "198.51.100."+string(rune('1'+i)), "wrong-"+string(rune('a'+i)))
	}

	require.Equal(t, http.StatusTooManyRequests,
		davRequest(t, srv, "203.0.113.9:5000", "198.51.100.99", "wrong-x"),
		"a spoofed X-Forwarded-For must not reset the budget")
}
