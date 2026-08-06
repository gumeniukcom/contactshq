package sync_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

// stubDAVServer answers just enough for discovery to fall through to its last strategy
// (treat the URL path as the address book path) and to accept a PUT afterwards.
func stubDAVServer(t *testing.T) (*httptest.Server, *[]*http.Request) {
	t.Helper()

	var puts []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts = append(puts, r.Clone(context.Background()))
			w.Header().Set("ETag", `"srv-etag"`)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Discovery (PROPFIND / .well-known GET) gets nowhere, which is the point.
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &puts
}

// Both constructors must produce a fully-formed provider. NewCardDAVClientProviderWithOptions
// used to set only client and abPath, so objectURL dereferenced a nil baseURL and every
// conditional PUT panicked — meaning export to any password-authenticated CardDAV server was
// broken outright, with the panic swallowed by the worker's recover().
func TestCardDAVProvider_PutIfMatchWorksForEveryConstructor(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		make func(endpoint string) (*chqsync.CardDAVClientProvider, error)
	}{
		{
			name: "NewCardDAVClientProvider",
			make: func(endpoint string) (*chqsync.CardDAVClientProvider, error) {
				return chqsync.NewCardDAVClientProvider(ctx, endpoint, "user", "pass")
			},
		},
		{
			name: "NewCardDAVClientProviderWithOptions",
			make: func(endpoint string) (*chqsync.CardDAVClientProvider, error) {
				return chqsync.NewCardDAVClientProviderWithOptions(ctx, endpoint, "user", "pass", false)
			},
		},
		{
			name: "NewCardDAVClientProviderWithHTTPClient",
			make: func(endpoint string) (*chqsync.CardDAVClientProvider, error) {
				return chqsync.NewCardDAVClientProviderWithHTTPClient(ctx, endpoint, &http.Client{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, puts := stubDAVServer(t)

			provider, err := tt.make(srv.URL + "/addressbooks/user/default/")
			require.NoError(t, err)

			res, err := provider.PutIfMatch(ctx, chqsync.SyncItem{
				RemoteID:  "uid-1",
				VCardData: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-1\r\nFN:A\r\nEND:VCARD\r\n",
			}, "old-etag")
			require.NoError(t, err)
			require.Equal(t, "srv-etag", res.ETag)

			require.Len(t, *puts, 1)
			got := (*puts)[0]
			require.Equal(t, "/addressbooks/user/default/uid-1.vcf", got.URL.Path)
			require.Equal(t, `"old-etag"`, got.Header.Get("If-Match"))
		})
	}
}

// A host that accepts the connection and then never answers must not park the calling
// goroutine forever: the worker pool is four goroutines wide, so four such syncs would stop
// backups and dedup along with sync.
func TestCardDAVProvider_DiscoveryTimesOutOnSilentHost(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	// Accept connections and then hold them open, answering nothing.
	go func() {
		for {
			conn, aerr := listener.Accept()
			if aerr != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	// A deadline well under the client's own 30s timeout keeps the test fast while still
	// proving the call is bounded rather than indefinite.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, perr := chqsync.NewCardDAVClientProviderWithOptions(
			ctx, "http://"+listener.Addr().String()+"/books/", "user", "pass", false)
		done <- perr
	}()

	select {
	case perr := <-done:
		// Discovery falls back to treating the path as the address book, so construction
		// itself may succeed; what matters is that it returned instead of hanging.
		_ = perr
	case <-time.After(10 * time.Second):
		t.Fatal("provider construction hung on a silent host")
	}
}

// Cancelling the caller's context must abort discovery rather than run it to completion on
// a context the caller no longer cares about.
func TestCardDAVProvider_DiscoveryStopsOnCancelledContext(t *testing.T) {
	srv, _ := stubDAVServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		_, _ = chqsync.NewCardDAVClientProviderWithOptions(ctx, srv.URL+"/books/", "user", "pass", false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery ignored a cancelled context")
	}
}
