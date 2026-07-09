package carddav

import (
	"context"
	"encoding/base64"
	"net/http"
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
}

func NewServer(backend *Backend, userRepo repository.UserRepository, appPwRepo repository.AppPasswordRepository, prefix string) *Server {
	handler := &carddav.Handler{
		Backend: backend,
		Prefix:  prefix,
	}

	return &Server{
		handler:   handler,
		backend:   backend,
		userRepo:  userRepo,
		appPwRepo: appPwRepo,
		authCache: newAuthCache(),
	}
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

	verdict := s.authenticate(r.Context(), email, password)
	if !verdict.ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="ContactsHQ CardDAV"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := WithUserID(r.Context(), verdict.userID)
	ctx = WithUserEmail(ctx, verdict.userEmail)
	r = r.WithContext(ctx)

	s.handler.ServeHTTP(w, r)
}

// authenticate verifies Basic-auth credentials, consulting the short-lived verdict
// cache first. A cached positive keeps working until its TTL expires, so a password
// change takes up to authCachePositiveTTL to lock out old CardDAV clients.
func (s *Server) authenticate(ctx context.Context, email, password string) authVerdict {
	key := authCacheKey(email, password)
	if v, ok := s.authCache.get(key); ok {
		if v.ok {
			// Refresh last-used bookkeeping is skipped on cache hits by design:
			// it would defeat the point of not touching the DB per request.
			return v
		}
		return authVerdict{}
	}

	verdict := s.verifyCredentials(ctx, email, password)
	s.authCache.put(key, verdict)
	return verdict
}

func (s *Server) verifyCredentials(ctx context.Context, email, password string) authVerdict {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil || user == nil {
		return authVerdict{}
	}

	if !verifyArgon2id(password, user.PasswordHash) {
		// Fallback: try app-specific passwords
		if !s.verifyAppPassword(ctx, user.ID, password) {
			return authVerdict{}
		}
	}

	return authVerdict{ok: true, userID: user.ID, userEmail: user.Email}
}

func (s *Server) verifyAppPassword(ctx context.Context, userID, password string) bool {
	if s.appPwRepo == nil {
		return false
	}
	passwords, err := s.appPwRepo.ListAllByUser(ctx, userID)
	if err != nil || len(passwords) == 0 {
		return false
	}
	for _, ap := range passwords {
		if verifyArgon2id(password, ap.PasswordHash) {
			_ = s.appPwRepo.UpdateLastUsed(ctx, ap.ID)
			return true
		}
	}
	return false
}

func verifyArgon2id(password, encodedHash string) bool {
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
