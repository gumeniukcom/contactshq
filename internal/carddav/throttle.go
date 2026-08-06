package carddav

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CardDAV Basic-auth is the cheapest unauthenticated way to make this server work hard: every
// miss costs a 64 MiB argon2id hash, multiplied by the number of app passwords the account
// has, and the /dav mount has no rate limiting of its own.
//
// Two independent measures apply, because they address different things:
//
//   - authArgon2Concurrency caps how much of that work can happen at once. It bounds peak
//     memory at 64 MiB × N regardless of who is asking or from how many addresses, and it
//     cannot lock anybody out — a legitimate client is at worst delayed.
//   - the per-IP failure counter stops one source from grinding away indefinitely.
//
// There is deliberately no bucket keyed on email. The email is the CardDAV login and is
// usually public, so a failure counter on it is a ready-made way for anyone to lock the
// owner's phone out of sync. A defence must not be a better denial of service than the thing
// it defends against.
const (
	authArgon2Concurrency = 4
	authFailureLimit      = 10
	authFailureWindow     = 5 * time.Minute
	authBlockDuration     = 5 * time.Minute
	authThrottleMaxIPs    = 1024
)

type failureRecord struct {
	count        int
	firstFailure time.Time
	blockedUntil time.Time
}

type authThrottle struct {
	mu      sync.Mutex
	records map[string]failureRecord
	now     func() time.Time // injectable for tests
}

func newAuthThrottle() *authThrottle {
	return &authThrottle{
		records: make(map[string]failureRecord),
		now:     time.Now,
	}
}

// blocked reports whether this address is currently shut out.
func (t *authThrottle) blocked(ip string) bool {
	if ip == "" {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	rec, ok := t.records[ip]
	if !ok {
		return false
	}
	if t.now().Before(rec.blockedUntil) {
		return true
	}
	if !rec.blockedUntil.IsZero() {
		// The block has expired; start this address over rather than leaving it one
		// failure away from being blocked again.
		delete(t.records, ip)
	}
	return false
}

// recordFailure counts a rejected attempt and blocks the address once it crosses the limit.
func (t *authThrottle) recordFailure(ip string) {
	if ip == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	rec, ok := t.records[ip]

	// The window is a sliding start: failures spread thinner than the window never add up.
	if !ok || now.Sub(rec.firstFailure) > authFailureWindow {
		rec = failureRecord{firstFailure: now}
	}
	rec.count++
	if rec.count >= authFailureLimit {
		rec.blockedUntil = now.Add(authBlockDuration)
	}

	if len(t.records) >= authThrottleMaxIPs {
		t.evictStaleLocked(now)
		// Still full: someone is churning source addresses. Drop everything rather than let
		// the defence itself become the memory-growth vector.
		if len(t.records) >= authThrottleMaxIPs {
			t.records = make(map[string]failureRecord, authThrottleMaxIPs)
		}
	}
	t.records[ip] = rec
}

// recordSuccess clears the counter, so an occasional typo never accumulates towards a block.
func (t *authThrottle) recordSuccess(ip string) {
	if ip == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.records, ip)
}

func (t *authThrottle) evictStaleLocked(now time.Time) {
	for ip, rec := range t.records {
		if now.After(rec.blockedUntil) && now.Sub(rec.firstFailure) > authFailureWindow {
			delete(t.records, ip)
		}
	}
}

// trustedProxySet answers whether a direct peer is a proxy whose X-Forwarded-For may be
// believed.
type trustedProxySet struct {
	ips  []net.IP
	nets []*net.IPNet
}

func newTrustedProxySet(entries []string) *trustedProxySet {
	set := &trustedProxySet{}
	for _, entry := range entries {
		if ip := net.ParseIP(entry); ip != nil {
			set.ips = append(set.ips, ip)
			continue
		}
		if _, cidr, err := net.ParseCIDR(entry); err == nil {
			set.nets = append(set.nets, cidr)
		}
		// Malformed entries are rejected at config load; ignore them here.
	}
	return set
}

func (s *trustedProxySet) contains(ip net.IP) bool {
	if ip == nil || s == nil {
		return false
	}
	for _, trusted := range s.ips {
		if trusted.Equal(ip) {
			return true
		}
	}
	for _, cidr := range s.nets {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP determines who to hold responsible for a failed attempt.
//
// The /dav mount is served through adaptor.HTTPHandler, so this sees a net/http request whose
// RemoteAddr is the TCP peer. Fiber's own trusted-proxy handling applies to fiber.Ctx and
// never reaches here — which means that behind the reverse proxy the documentation recommends,
// every CardDAV client in the world would otherwise share one address and therefore one
// bucket. X-Forwarded-For is honoured, but only when the peer is a configured proxy;
// otherwise the header is attacker-controlled and would hand out a fresh bucket per request.
func clientIP(r *http.Request, trusted *trustedProxySet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)

	peer := net.ParseIP(host)
	if !trusted.contains(peer) {
		return host
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return host
	}
	// Leftmost entry is the original client.
	first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
	if first == "" || net.ParseIP(first) == nil {
		return host
	}
	return first
}
