package carddav

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Credential verification for the CardDAV mount. It sits apart from server.go because the
// transport surface there belongs to the CardDAV domain (spec 004) while everything here —
// passwords, app passwords, argon2id — belongs to identity (spec 001). One file cannot be
// claimed by both; see constitution Principle VII.
//
// The argon2id verifier is duplicated from internal/service on purpose: importing that package
// here would be a cycle. Both copies must move together.

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
