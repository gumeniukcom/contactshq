package sync_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

var strict = chqsync.EndpointPolicy{}
var permissive = chqsync.EndpointPolicy{AllowInsecure: true}

func TestValidateProviderEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		policy   chqsync.EndpointPolicy
		wantErr  bool
	}{
		{name: "https", endpoint: "https://dav.example.com/", policy: strict},
		{name: "https with a path and port", endpoint: "https://dav.example.com:8443/addressbooks/u/", policy: strict},

		// A sync request carries the provider's username and password.
		{name: "http refused by default", endpoint: "http://dav.example.com/", policy: strict, wantErr: true},
		{name: "http allowed when opted in", endpoint: "http://dav.example.com/", policy: permissive},

		// The schemes that make this a request-forgery surface rather than a typo.
		{name: "file", endpoint: "file:///etc/passwd", policy: permissive, wantErr: true},
		{name: "gopher", endpoint: "gopher://example.com/", policy: permissive, wantErr: true},
		{name: "no scheme", endpoint: "dav.example.com", policy: permissive, wantErr: true},

		{name: "empty", endpoint: "", policy: permissive, wantErr: true},
		{name: "whitespace", endpoint: "   ", policy: permissive, wantErr: true},
		{name: "no host", endpoint: "https:///addressbooks/", policy: permissive, wantErr: true},

		// Credentials in the URL would be logged and stored alongside it.
		{name: "userinfo", endpoint: "https://user:pw@dav.example.com/", policy: permissive, wantErr: true},

		// The address itself is NOT filtered: syncing against a CardDAV server on the local
		// network is a supported, ordinary use of this application.
		{name: "private address is allowed", endpoint: "https://192.168.1.10/dav/", policy: strict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := chqsync.ValidateProviderEndpoint(tt.endpoint, tt.policy)
			if tt.wantErr {
				require.ErrorIs(t, err, chqsync.ErrInvalidEndpoint)
				return
			}
			require.NoError(t, err)
		})
	}
}

// The endpoint lives inside the step's JSON config, which is why ValidateStep had to be
// widened to see it.
func TestValidateStepEndpoint_ChecksTheConfig(t *testing.T) {
	err := chqsync.ValidateStep(
		"carddav", `{"endpoint":"file:///etc/passwd"}`,
		"internal", "",
		permissive,
	)
	require.ErrorIs(t, err, chqsync.ErrInvalidEndpoint)

	require.NoError(t, chqsync.ValidateStep(
		"carddav", `{"endpoint":"https://dav.example.com/"}`,
		"internal", "",
		strict,
	))
}

// A config that cannot be read is a different problem, reported elsewhere; it must not be
// mistaken for a bad endpoint.
func TestValidateStepEndpoint_IgnoresUnreadableConfig(t *testing.T) {
	require.NoError(t, chqsync.ValidateStep("carddav", "not json", "internal", "", strict))
	require.NoError(t, chqsync.ValidateStep("carddav", "", "internal", "", strict))
}

func TestValidateStepEndpoint_StillEnforcesShape(t *testing.T) {
	// Provider to provider remains rejected.
	err := chqsync.ValidateStep("carddav", "", "google", "", strict)
	require.ErrorIs(t, err, chqsync.ErrInvalidStep)
}

// Validating the string a user typed is not enough: a permitted host can redirect onward.
// This is the case that makes address filtering unnecessary for the pivot it would prevent.
func TestProvider_RefusesARedirectToAnotherHost(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("metadata"))
	}))
	t.Cleanup(elsewhere.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/latest/meta-data/", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	// Discovery follows .well-known through a plain GET; the redirect must not be honoured.
	provider, err := chqsync.NewCardDAVClientProviderWithOptions(
		t.Context(), origin.URL+"/dav/", "user", "pass", false)

	// Construction falls back to treating the path as the address book, so it may succeed —
	// what matters is that the provider did not end up pointed at the other host.
	if err == nil {
		require.NotContains(t, provider.BaseURLForTest(), elsewhere.URL,
			"a redirect to a different host must not be followed")
	}
}

// Within one host a redirect is ordinary — many CardDAV servers use one for .well-known.
func TestProvider_FollowsARedirectWithinTheSameHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/carddav" {
			http.Redirect(w, r, "/dav/", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	_, err := chqsync.NewCardDAVClientProviderWithOptions(t.Context(), srv.URL+"/", "user", "pass", false)
	require.NoError(t, err, "a same-host redirect is normal and must still work")
}
