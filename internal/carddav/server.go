package carddav

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/emersion/go-webdav/carddav"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"golang.org/x/crypto/argon2"
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

// authenticate verifies Basic-auth credentials, consulting the short-lived verdict
// cache first. A cached positive keeps working until its TTL expires, so a password
// change takes up to authCachePositiveTTL to lock out old CardDAV clients.
//
// Order matters. A cached positive is answered before the throttle is even consulted, so a
// client whose credentials work keeps working while some other address is blocked. Only then
// is the failure counter checked, and only then is an argon2id slot taken.
func (s *Server) authenticate(ctx context.Context, email, password, ip string) (verdict authVerdict, throttled bool) {
	key := authCacheKey(email, password)
	cached, isCached := s.authCache.get(key)
	if isCached && cached.ok {
		// Refreshing last-used bookkeeping is skipped on cache hits by design:
		// it would defeat the point of not touching the DB per request.
		return cached, false
	}

	if s.throttle.blocked(ip) {
		return authVerdict{}, true
	}

	if isCached {
		// A known-bad credential: cheap to answer, but it still counts as an attempt.
		s.throttle.recordFailure(ip)
		return authVerdict{}, false
	}

	v := s.verifyCredentials(ctx, email, password)
	s.authCache.put(key, v)

	if v.ok {
		s.throttle.recordSuccess(ip)
	} else {
		s.throttle.recordFailure(ip)
	}
	return v, false
}

func (s *Server) verifyCredentials(ctx context.Context, email, password string) authVerdict {
	// Everything past this point hashes; hold a slot for all of it, including the app
	// password loop, which hashes once per stored password.
	select {
	case s.argon2Slots <- struct{}{}:
		defer func() { <-s.argon2Slots }()
	case <-ctx.Done():
		return authVerdict{}
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil || user == nil {
		return authVerdict{}
	}

	if !VerifyArgon2id(password, user.PasswordHash) {
		// Fallback: try app-specific passwords
		if !s.verifyAppPassword(ctx, user.ID, password) {
			return authVerdict{}
		}
	}

	return authVerdict{ok: true, userID: user.ID, userEmail: user.Email}
}

// verifyAppPassword tries every app password the account has.
//
// The loop is deliberately not capped: ListAllByUser returns rows in no particular order, so
// truncating it at the creation limit would silently revoke access for whichever clients
// happened to sort last on an account that already exceeded it. The cost is bounded by the
// creation limit for new passwords and by the argon2 slot for everything else.
func (s *Server) verifyAppPassword(ctx context.Context, userID, password string) bool {
	if s.appPwRepo == nil {
		return false
	}
	passwords, err := s.appPwRepo.ListAllByUser(ctx, userID)
	if err != nil || len(passwords) == 0 {
		return false
	}

	// Most-recently-used first, so the common case hashes once.
	sort.SliceStable(passwords, func(i, j int) bool {
		li, lj := passwords[i].LastUsedAt, passwords[j].LastUsedAt
		if li == nil {
			return false
		}
		if lj == nil {
			return true
		}
		return li.After(*lj)
	})

	for _, ap := range passwords {
		if VerifyArgon2id(password, ap.PasswordHash) {
			_ = s.appPwRepo.UpdateLastUsed(ctx, ap.ID)
			return true
		}
	}
	return false
}

// VerifyArgon2id checks a password against an encoded argon2id hash the way CardDAV
// authentication does.
//
// It is exported so other packages can assert that a hash they write is one CardDAV will
// accept. That matters because this is a second, independent implementation — service has its
// own verifyPassword, and the two read the encoded parameters differently (this one derives
// the key length from the stored hash, the other takes it from a constant). service cannot
// import carddav, which is why the duplication exists at all; a hash accepted by only one of
// them would let a user log in over HTTP but not sync, or the reverse.
func VerifyArgon2id(password, encodedHash string) bool {
	const prefix = "$argon2id$v=19$"
	if !strings.HasPrefix(encodedHash, prefix) {
		return false
	}

	rest := encodedHash[len(prefix):]

	// Parse m=65536,t=1,p=4$salt$hash
	paramEnd := strings.Index(rest, "$")
	if paramEnd < 0 {
		return false
	}
	params := rest[:paramEnd]
	rest = rest[paramEnd+1:]

	var memory, time uint32
	var threads uint8
	for _, part := range strings.Split(params, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		val := parseUint(kv[1])
		switch kv[0] {
		case "m":
			memory = uint32(val)
		case "t":
			time = uint32(val)
		case "p":
			threads = uint8(val)
		}
	}

	saltEnd := strings.Index(rest, "$")
	if saltEnd < 0 {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(rest[:saltEnd])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(rest[saltEnd+1:])
	if err != nil {
		return false
	}

	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	if len(expectedHash) != len(computedHash) {
		return false
	}
	result := byte(0)
	for i := range expectedHash {
		result |= expectedHash[i] ^ computedHash[i]
	}
	return result == 0
}

func parseUint(s string) uint64 {
	var n uint64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + uint64(c-'0')
		}
	}
	return n
}
