package carddav

import (
	"testing"
	"time"
)

func newTestCache(now *time.Time) *authCache {
	c := newAuthCache()
	c.now = func() time.Time { return *now }
	return c
}

func TestAuthCacheHitAndMiss(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := newTestCache(&now)
	key := authCacheKey("a@example.com", "hunter2")

	if _, ok := c.get(key); ok {
		t.Fatal("empty cache returned a verdict")
	}

	c.put(key, authVerdict{ok: true, userID: "u1", userEmail: "a@example.com"})

	v, ok := c.get(key)
	if !ok || !v.ok || v.userID != "u1" {
		t.Fatalf("get() = %+v, %v; want cached positive for u1", v, ok)
	}
}

func TestAuthCachePositiveExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := newTestCache(&now)
	key := authCacheKey("a@example.com", "hunter2")

	c.put(key, authVerdict{ok: true, userID: "u1"})

	now = now.Add(authCachePositiveTTL - time.Second)
	if _, ok := c.get(key); !ok {
		t.Fatal("positive verdict expired early")
	}

	now = now.Add(2 * time.Second)
	if _, ok := c.get(key); ok {
		t.Fatal("positive verdict outlived its TTL")
	}
}

// Negative verdicts must expire much sooner than positive ones, so a password fix
// is not shadowed by a stale rejection.
func TestAuthCacheNegativeExpiresSooner(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := newTestCache(&now)
	key := authCacheKey("a@example.com", "wrong")

	c.put(key, authVerdict{ok: false})

	now = now.Add(authCacheNegativeTTL + time.Second)
	if _, ok := c.get(key); ok {
		t.Fatal("negative verdict outlived its TTL")
	}

	if authCacheNegativeTTL >= authCachePositiveTTL {
		t.Fatal("negative TTL must be shorter than positive TTL")
	}
}

func TestAuthCacheKeyIsolatesPasswordAndUser(t *testing.T) {
	if authCacheKey("a@example.com", "p1") == authCacheKey("a@example.com", "p2") {
		t.Error("different passwords collide")
	}
	if authCacheKey("a@example.com", "p") == authCacheKey("b@example.com", "p") {
		t.Error("different users collide")
	}
	if got := authCacheKey("a@example.com", "p"); len(got) < 64 {
		t.Errorf("key %q looks unhashed", got)
	}
}

// A credential-stuffing run churns unique keys; the cache must stay bounded.
func TestAuthCacheBounded(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := newTestCache(&now)

	for i := 0; i < authCacheMaxEntries*3; i++ {
		c.put(authCacheKey("a@example.com", string(rune(i))), authVerdict{ok: false})
	}

	c.mu.Lock()
	size := len(c.entries)
	c.mu.Unlock()

	if size > authCacheMaxEntries {
		t.Fatalf("cache grew to %d entries, want <= %d", size, authCacheMaxEntries)
	}
}
