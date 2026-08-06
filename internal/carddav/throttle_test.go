package carddav

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthThrottle_BlocksAfterLimitAndReleasesLater(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	th := newAuthThrottle()
	th.now = func() time.Time { return now }

	for i := 0; i < authFailureLimit-1; i++ {
		th.recordFailure("10.0.0.1")
		if th.blocked("10.0.0.1") {
			t.Fatalf("blocked after %d failures, limit is %d", i+1, authFailureLimit)
		}
	}

	th.recordFailure("10.0.0.1")
	if !th.blocked("10.0.0.1") {
		t.Fatalf("not blocked after %d failures", authFailureLimit)
	}

	// A different address is unaffected.
	if th.blocked("10.0.0.2") {
		t.Fatal("blocking one address must not affect another")
	}

	now = now.Add(authBlockDuration + time.Second)
	if th.blocked("10.0.0.1") {
		t.Fatal("the block must expire")
	}
	// And the counter starts over rather than leaving the address one failure from a block.
	th.recordFailure("10.0.0.1")
	if th.blocked("10.0.0.1") {
		t.Fatal("counter was not reset after the block expired")
	}
}

// A success clears the counter, so an occasional typo never accumulates into a lockout.
func TestAuthThrottle_SuccessResetsCounter(t *testing.T) {
	th := newAuthThrottle()

	for i := 0; i < authFailureLimit-1; i++ {
		th.recordFailure("10.0.0.1")
	}
	th.recordSuccess("10.0.0.1")

	for i := 0; i < authFailureLimit-1; i++ {
		th.recordFailure("10.0.0.1")
	}
	if th.blocked("10.0.0.1") {
		t.Fatal("a success must reset the failure count")
	}
}

// Failures spread thinner than the window never add up to a block.
func TestAuthThrottle_WindowSlides(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	th := newAuthThrottle()
	th.now = func() time.Time { return now }

	for i := 0; i < authFailureLimit*3; i++ {
		th.recordFailure("10.0.0.1")
		now = now.Add(authFailureWindow + time.Second)
		if th.blocked("10.0.0.1") {
			t.Fatal("isolated failures outside the window must not accumulate")
		}
	}
}

// The defence must not become the memory-growth vector it defends against.
func TestAuthThrottle_MapIsBounded(t *testing.T) {
	th := newAuthThrottle()

	for i := 0; i < authThrottleMaxIPs*3; i++ {
		th.recordFailure(net4(i))
	}
	if len(th.records) > authThrottleMaxIPs {
		t.Fatalf("throttle map grew to %d entries, cap is %d", len(th.records), authThrottleMaxIPs)
	}
}

func net4(i int) string {
	return "10." + itoa(i>>16&0xff) + "." + itoa(i>>8&0xff) + "." + itoa(i&0xff)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		trusted    []string
		want       string
	}{
		{
			name:       "no proxies configured: the peer is the client",
			remoteAddr: "203.0.113.9:5000",
			forwarded:  "198.51.100.7",
			want:       "203.0.113.9",
		},
		{
			name:       "untrusted peer: a forged header is ignored",
			remoteAddr: "203.0.113.9:5000",
			forwarded:  "198.51.100.7",
			trusted:    []string{"10.0.0.1"},
			want:       "203.0.113.9",
		},
		{
			name:       "trusted peer: the leftmost forwarded entry wins",
			remoteAddr: "10.0.0.1:5000",
			forwarded:  "198.51.100.7, 10.0.0.1",
			trusted:    []string{"10.0.0.1"},
			want:       "198.51.100.7",
		},
		{
			name:       "trusted by CIDR",
			remoteAddr: "10.0.5.4:5000",
			forwarded:  "198.51.100.7",
			trusted:    []string{"10.0.0.0/8"},
			want:       "198.51.100.7",
		},
		{
			name:       "trusted peer with no header falls back to the peer",
			remoteAddr: "10.0.0.1:5000",
			trusted:    []string{"10.0.0.1"},
			want:       "10.0.0.1",
		},
		{
			name:       "trusted peer with a garbage header falls back to the peer",
			remoteAddr: "10.0.0.1:5000",
			forwarded:  "not-an-ip",
			trusted:    []string{"10.0.0.1"},
			want:       "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/dav/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			got := clientIP(r, newTrustedProxySet(tt.trusted))
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
