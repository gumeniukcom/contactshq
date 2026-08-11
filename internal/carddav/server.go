package carddav

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/emersion/go-webdav/carddav"
	"github.com/gumeniukcom/contactshq/internal/repository"
)

type Server struct {
	handler   *carddav.Handler
	backend   *Backend
	userRepo  repository.UserRepository
	appPwRepo repository.AppPasswordRepository
	authCache *authCache
	throttle  *authThrottle
	trusted   *trustedProxySet

	// argon2Slots bounds how many password verifications may run at once, capping peak
	// memory at 64 MiB per slot no matter how many requests arrive.
	argon2Slots chan struct{}
}

func NewServer(backend *Backend, userRepo repository.UserRepository, appPwRepo repository.AppPasswordRepository, prefix string) *Server {
	return NewServerWithTrustedProxies(backend, userRepo, appPwRepo, prefix, nil)
}

// NewServerWithTrustedProxies is NewServer plus the proxy list used to attribute a request to
// a client address. Without it, every client behind a reverse proxy shares one failure bucket.
func NewServerWithTrustedProxies(
	backend *Backend,
	userRepo repository.UserRepository,
	appPwRepo repository.AppPasswordRepository,
	prefix string,
	trustedProxies []string,
) *Server {
	handler := &carddav.Handler{
		Backend: backend,
		Prefix:  prefix,
	}

	return &Server{
		handler:     handler,
		backend:     backend,
		userRepo:    userRepo,
		appPwRepo:   appPwRepo,
		authCache:   newAuthCache(),
		throttle:    newAuthThrottle(),
		trusted:     newTrustedProxySet(trustedProxies),
		argon2Slots: make(chan struct{}, authArgon2Concurrency),
	}
}

// InvalidateUser drops every cached authentication verdict for an account.
//
// Call it whenever a credential stops being valid — a changed password, a deleted app
// password. Without it the old secret keeps opening CardDAV for up to authCachePositiveTTL,
// which is five minutes of the user believing they revoked access when they had not.
func (s *Server) InvalidateUser(email string) {
	s.authCache.invalidateEmail(email)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Basic ") {
		w.Header().Set("WWW-Authenticate", `Basic realm="ContactsHQ CardDAV"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(authHeader[6:])
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	email, password := parts[0], parts[1]

	verdict, throttled := s.authenticate(r.Context(), email, password, clientIP(r, s.trusted))
	if throttled {
		w.Header().Set("Retry-After", strconv.Itoa(int(authBlockDuration.Seconds())))
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}
	if !verdict.ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="ContactsHQ CardDAV"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := WithUserID(r.Context(), verdict.userID)
	ctx = WithUserEmail(ctx, verdict.userEmail)
	r = r.WithContext(ctx)

	if s.serveSyncExtensions(w, r) {
		return
	}

	s.handler.ServeHTTP(w, r)
}

// serveSyncExtensions answers the two requests go-webdav cannot: the CalendarServer CTag
// and RFC 6578 sync-collection. Everything else falls through untouched.
func (s *Server) serveSyncExtensions(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "PROPFIND" && r.Method != "REPORT" {
		return false
	}
	if !isAddressBookPath(r.URL.Path, s.backend.prefix, GetUserEmail(r.Context())) {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxSyncRequestBody))
	if err != nil {
		return false
	}
	// The delegated handler still needs to read the body.
	r.Body = io.NopCloser(bytes.NewReader(body))

	switch r.Method {
	case "PROPFIND":
		return s.handlePropfindCTag(w, r, body)
	case "REPORT":
		return s.handleSyncCollection(w, r, body)
	}
	return false
}

// maxSyncRequestBody bounds the XML we buffer before deciding who handles the request.
const maxSyncRequestBody = 1 << 20

func isAddressBookPath(path, prefix, email string) bool {
	if email == "" {
		return false
	}
	want := AddressBookPath(prefix, email)
	return path == want || path+"/" == want
}
