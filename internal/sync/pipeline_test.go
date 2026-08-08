package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

// stubCredRepo is the smallest thing that satisfies repository.ProviderConnectionRepository.
// Only GetByID is ever called on the path under test; the rest exist to satisfy the interface
// and panic loudly if that ever stops being true.
type stubCredRepo struct {
	cred *domain.ProviderConnection
}

func (s *stubCredRepo) GetByID(_ context.Context, id string) (*domain.ProviderConnection, error) {
	if s.cred == nil || s.cred.ID != id {
		return nil, nil
	}
	return s.cred, nil
}

func (s *stubCredRepo) Create(context.Context, *domain.ProviderConnection) error {
	panic("not used by createProvider")
}

func (s *stubCredRepo) ListByUser(context.Context, string) ([]*domain.ProviderConnection, error) {
	panic("not used by createProvider")
}

func (s *stubCredRepo) GetByUserAndType(context.Context, string, string) (*domain.ProviderConnection, error) {
	panic("not used by createProvider")
}

func (s *stubCredRepo) Update(context.Context, *domain.ProviderConnection) error {
	panic("not used by createProvider")
}

func (s *stubCredRepo) Delete(context.Context, string) error {
	panic("not used by createProvider")
}

func (s *stubCredRepo) UpdateToken(context.Context, string, string, string, *time.Time) error {
	panic("not used by createProvider")
}

func (s *stubCredRepo) SetConnected(context.Context, string, bool) error {
	panic("not used by createProvider")
}

// countingOAuth records whether the token exchange was attempted. GetHTTPClient talks to
// Google, so "was it called" is the observable that distinguishes validation-before-exchange
// from validation-before-dial.
type countingOAuth struct {
	calls atomic.Int32
}

func (c *countingOAuth) GetHTTPClient(context.Context, *domain.ProviderConnection) (*http.Client, error) {
	c.calls.Add(1)
	return &http.Client{}, nil
}

// countingServer is a plain-http endpoint that records every request it receives, so a test
// can assert that nothing was dialled at all.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func orchestratorWith(credRepo *stubCredRepo, oauth OAuthHTTPClientProvider, policy EndpointPolicy) *PipelineOrchestrator {
	return NewPipelineOrchestrator(nil, nil, nil, nil, credRepo, oauth, zap.NewNop()).
		WithEndpointPolicy(policy)
}

// A credential stored before v0.4.0 may hold an http:// endpoint, because nothing validated it
// when it was written. Resolving it at run time and syncing anyway posts the provider password
// in clear text.
func TestCreateProvider_RefusesAGrandfatheredHTTPCredential(t *testing.T) {
	srv, hits := countingServer(t)

	repo := &stubCredRepo{cred: &domain.ProviderConnection{
		ID:       "cred-1",
		UserID:   "user-1",
		Endpoint: srv.URL + "/dav/", // httptest serves plain http
		Username: "user",
		Password: "provider-password",
	}}

	_, err := orchestratorWith(repo, nil, EndpointPolicy{}).
		createProvider(t.Context(), "user-1", "carddav", `{"credential_id":"cred-1"}`)

	require.ErrorIs(t, err, ErrInvalidEndpoint)
	require.Zero(t, hits.Load(), "a refused endpoint must not be contacted")
}

// The OAuth branch returns from inside the credential block, so validation placed after that
// block would never see it. The bearer token would travel in clear text just as a password does.
func TestCreateProvider_RefusesAnOAuthCredentialOverHTTP(t *testing.T) {
	srv, hits := countingServer(t)

	repo := &stubCredRepo{cred: &domain.ProviderConnection{
		ID:          "cred-1",
		UserID:      "user-1",
		Endpoint:    srv.URL + "/dav/",
		AccessToken: "ya29.token",
	}}
	oauth := &countingOAuth{}

	_, err := orchestratorWith(repo, oauth, EndpointPolicy{}).
		createProvider(t.Context(), "user-1", "carddav", `{"credential_id":"cred-1"}`)

	require.ErrorIs(t, err, ErrInvalidEndpoint)
	require.Zero(t, hits.Load(), "a refused endpoint must not be contacted")
	// GetHTTPClient performs a token exchange. Validating after it would leak the refresh
	// token to Google's token endpoint before deciding the sync may not proceed.
	require.Zero(t, oauth.calls.Load(), "validation must precede the OAuth token exchange")
}

// The ownership check moves when the credential block is restructured; nothing else asserts it.
func TestCreateProvider_RefusesACredentialBelongingToAnotherUser(t *testing.T) {
	repo := &stubCredRepo{cred: &domain.ProviderConnection{
		ID:       "cred-1",
		UserID:   "someone-else",
		Endpoint: "https://dav.example.com/",
	}}

	_, err := orchestratorWith(repo, nil, EndpointPolicy{}).
		createProvider(t.Context(), "user-1", "carddav", `{"credential_id":"cred-1"}`)

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInvalidEndpoint, "ownership is a different refusal")
	require.Contains(t, err.Error(), "not found")
}

// The escape hatch has to keep working: a LAN CardDAV server reachable only over http is a
// supported setup once the operator has opted in.
func TestCreateProvider_AllowsAnHTTPCredentialWithTheOptIn(t *testing.T) {
	srv, hits := countingServer(t)

	repo := &stubCredRepo{cred: &domain.ProviderConnection{
		ID:       "cred-1",
		UserID:   "user-1",
		Endpoint: srv.URL + "/dav/",
		Username: "user",
		Password: "provider-password",
	}}

	provider, err := orchestratorWith(repo, nil, EndpointPolicy{AllowInsecure: true}).
		createProvider(t.Context(), "user-1", "carddav", `{"credential_id":"cred-1"}`)

	require.NoError(t, err)
	require.NotNil(t, provider)
	require.NotZero(t, hits.Load(), "construction should have reached discovery")
}

// Execute already refuses this before createProvider is reached: it calls ValidateStep for
// every step on every run (Execute), and ValidateStep ends in ValidateStepEndpoints. This test
// pins that createProvider no longer depends on its caller having done so.
func TestCreateProvider_RefusesAnInlineHTTPEndpoint(t *testing.T) {
	srv, hits := countingServer(t)

	_, err := orchestratorWith(nil, nil, EndpointPolicy{}).
		createProvider(t.Context(), "user-1", "carddav",
			`{"endpoint":"`+srv.URL+`/dav/","username":"user","password":"pw"}`)

	require.ErrorIs(t, err, ErrInvalidEndpoint)
	require.Zero(t, hits.Load(), "a refused endpoint must not be contacted")
}

// A nil credential repository must not turn a missing credential into an unvalidated sync.
func TestCreateProvider_RefusesAnEmptyEndpoint(t *testing.T) {
	_, err := orchestratorWith(nil, nil, EndpointPolicy{}).
		createProvider(t.Context(), "user-1", "carddav", `{}`)

	require.ErrorIs(t, err, ErrInvalidEndpoint)
}
