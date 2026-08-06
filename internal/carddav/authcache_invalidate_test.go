package carddav

import "testing"

func TestAuthCache_InvalidateEmailDropsOnlyThatAccount(t *testing.T) {
	c := newAuthCache()

	keep := authCacheKey("other@example.com", "their-password")
	c.put(authCacheKey("owner@example.com", "password-one"), authVerdict{ok: true, userID: "u1"})
	c.put(authCacheKey("owner@example.com", "password-two"), authVerdict{ok: true, userID: "u1"})
	c.put(keep, authVerdict{ok: true, userID: "u2"})

	c.invalidateEmail("owner@example.com")

	if _, ok := c.get(authCacheKey("owner@example.com", "password-one")); ok {
		t.Fatal("a verdict for the invalidated account survived")
	}
	if _, ok := c.get(authCacheKey("owner@example.com", "password-two")); ok {
		t.Fatal("a second verdict for the invalidated account survived")
	}
	if _, ok := c.get(keep); !ok {
		t.Fatal("another account's verdict was dropped")
	}
}

func TestAuthCache_InvalidateEmailIgnoresBlankAndUnknown(t *testing.T) {
	c := newAuthCache()
	key := authCacheKey("owner@example.com", "password")
	c.put(key, authVerdict{ok: true, userID: "u1"})

	c.invalidateEmail("")
	c.invalidateEmail("nobody@example.com")

	if _, ok := c.get(key); !ok {
		t.Fatal("an unrelated invalidation dropped a live verdict")
	}
}

// An email that is a prefix of another must not take the longer one down with it: keys are
// "email:hash", so the separator has to be part of the prefix match.
func TestAuthCache_InvalidateEmailDoesNotMatchOnPrefixAlone(t *testing.T) {
	c := newAuthCache()

	short := authCacheKey("bob@example.com", "pw")
	long := authCacheKey("bob@example.com.attacker.net", "pw")
	c.put(short, authVerdict{ok: true})
	c.put(long, authVerdict{ok: true})

	c.invalidateEmail("bob@example.com")

	if _, ok := c.get(short); ok {
		t.Fatal("the target account was not invalidated")
	}
	if _, ok := c.get(long); !ok {
		t.Fatal("a different account whose email starts with the target's was invalidated too")
	}
}

// The server-level entry point is what production calls.
func TestServer_InvalidateUser(t *testing.T) {
	s := &Server{authCache: newAuthCache()}

	key := authCacheKey("owner@example.com", "password")
	s.authCache.put(key, authVerdict{ok: true, userID: "u1"})

	s.InvalidateUser("owner@example.com")

	if _, ok := s.authCache.get(key); ok {
		t.Fatal("InvalidateUser left the verdict in place — the old password would keep working")
	}
}
