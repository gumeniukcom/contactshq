package carddav

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// CardDAV clients (iOS, Thunderbird) issue many requests per sync session, and every
// Basic-auth check costs a 64 MiB argon2id hash — one per app password on a mismatch.
// authCache memoizes the verdict for a short window so a sync session pays that cost
// once instead of hundreds of times.
//
// Negative verdicts are cached far more briefly: they exist to blunt credential-stuffing
// against a single wrong password, not to persist a decision.
const (
	authCachePositiveTTL = 5 * time.Minute
	authCacheNegativeTTL = 30 * time.Second
	authCacheMaxEntries  = 1024
)

type authVerdict struct {
	ok        bool
	userID    string
	userEmail string
	expiresAt time.Time
}

type authCache struct {
	mu      sync.Mutex
	entries map[string]authVerdict
	now     func() time.Time // injectable for tests
}

func newAuthCache() *authCache {
	return &authCache{
		entries: make(map[string]authVerdict),
		now:     time.Now,
	}
}

// key derives a cache key from the presented credentials. The password never enters
// the map: only its SHA-256, which is enough to detect a repeat of the same secret.
func authCacheKey(email, password string) string {
	sum := sha256.Sum256([]byte(password))
	return email + ":" + hex.EncodeToString(sum[:])
}

func (c *authCache) get(key string) (authVerdict, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.entries[key]
	if !ok {
		return authVerdict{}, false
	}
	if c.now().After(v.expiresAt) {
		delete(c.entries, key)
		return authVerdict{}, false
	}
	return v, true
}

func (c *authCache) put(key string, v authVerdict) {
	ttl := authCacheNegativeTTL
	if v.ok {
		ttl = authCachePositiveTTL
	}
	v.expiresAt = c.now().Add(ttl)

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= authCacheMaxEntries {
		c.evictExpiredLocked()
		// Still full: an attacker is churning keys. Drop everything rather than grow
		// unbounded — the worst case is that legitimate clients re-hash once.
		if len(c.entries) >= authCacheMaxEntries {
			c.entries = make(map[string]authVerdict, authCacheMaxEntries)
		}
	}
	c.entries[key] = v
}

// invalidateEmail drops every cached verdict for one account.
//
// Cache keys are email + SHA-256 of the presented password, so the entries for a given
// account share the "email:" prefix and can be dropped without knowing any password.
func (c *authCache) invalidateEmail(email string) {
	if email == "" {
		return
	}
	prefix := email + ":"

	c.mu.Lock()
	defer c.mu.Unlock()

	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
}

func (c *authCache) evictExpiredLocked() {
	now := c.now()
	for k, v := range c.entries {
		if now.After(v.expiresAt) {
			delete(c.entries, k)
		}
	}
}
