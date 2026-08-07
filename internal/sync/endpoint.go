package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// A provider endpoint is a URL this server will fetch on a user's behalf, which makes every
// place one is accepted a request-forgery surface: `http://169.254.169.254/` reaches a cloud
// metadata service, `file://` reaches the disk.
//
// There are four ways one gets in — a connected CardDAV provider, a stored credential, the
// endpoint inside a pipeline step's JSON config, and the trigger endpoint posted directly in
// a request body. The last is the worst: it is used immediately and leaves no row behind, so
// an attempt is invisible afterwards.
//
// What this file does NOT do is resolve the host and reject private address ranges. That was
// considered and deliberately dropped: syncing against a CardDAV server on the local network
// is a normal, supported use of this application, and a dial-time filter would break it for
// everyone to defend a single-user instance against its own operator. See the plan's
// "explicitly not doing" section.

var (
	// ErrInvalidEndpoint reports a provider URL this server refuses to fetch.
	ErrInvalidEndpoint = errors.New("invalid provider endpoint")
	// ErrTooManyRedirects reports a redirect chain that went on too long.
	ErrTooManyRedirects = errors.New("too many redirects")
	// ErrRedirectToAnotherHost reports a redirect that tried to leave the original host.
	ErrRedirectToAnotherHost = errors.New("redirect to a different host")
)

// maxEndpointRedirects bounds a redirect chain.
const maxEndpointRedirects = 3

// EndpointPolicy is what a deployment is willing to fetch.
type EndpointPolicy struct {
	// AllowInsecure permits plain http. Off by default: credentials travel on these requests.
	AllowInsecure bool
}

// ValidateProviderEndpoint checks a URL before this server will fetch it.
//
// Kept as a standalone function rather than folded into step validation because two of the
// four entry points — the trigger endpoint and stored credentials — never pass through a
// pipeline step at all.
func ValidateProviderEndpoint(endpoint string, policy EndpointPolicy) error {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return fmt.Errorf("%w: endpoint is required", ErrInvalidEndpoint)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEndpoint, err)
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !policy.AllowInsecure {
			return fmt.Errorf("%w: http is refused — credentials would travel in clear text; "+
				"set sync.allow_insecure_endpoints to permit it", ErrInvalidEndpoint)
		}
	default:
		return fmt.Errorf("%w: scheme %q is not fetchable, only http and https are",
			ErrInvalidEndpoint, u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("%w: no host in %q", ErrInvalidEndpoint, trimmed)
	}

	// Credentials in the URL would be logged and stored with it; the provider config carries
	// a username and password of its own.
	if u.User != nil {
		return fmt.Errorf("%w: credentials in the URL are not accepted — use the username and "+
			"password fields", ErrInvalidEndpoint)
	}

	return nil
}

// ValidateStepEndpoints checks the endpoints inside a step's provider configuration.
//
// The configs arrive as JSON blobs, and an unreadable one is not an endpoint problem — step
// execution reports that separately — so it is passed over here rather than rejected.
func ValidateStepEndpoints(sourceConfig, destConfig string, policy EndpointPolicy) error {
	for _, cfg := range []string{sourceConfig, destConfig} {
		endpoint, ok := endpointFromConfig(cfg)
		if !ok {
			continue
		}
		if err := ValidateProviderEndpoint(endpoint, policy); err != nil {
			return err
		}
	}
	return nil
}

func endpointFromConfig(cfg string) (string, bool) {
	if strings.TrimSpace(cfg) == "" {
		return "", false
	}
	var parsed struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		return "", false
	}
	if strings.TrimSpace(parsed.Endpoint) == "" {
		return "", false
	}
	return parsed.Endpoint, true
}

// redirectPolicy bounds where a followed redirect may lead.
//
// Validating the string the user supplied is not enough on its own: a permitted host can
// answer `302 Location: http://169.254.169.254/`, and the client would follow it. Discovery
// does exactly this — resolveWellKnown issues a GET and reads the final URL.
func redirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= maxEndpointRedirects {
		return fmt.Errorf("%w: stopped after %d", ErrTooManyRedirects, maxEndpointRedirects)
	}
	if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("%w: %s tried to send us to %s",
			ErrRedirectToAnotherHost, via[0].URL.Host, req.URL.Host)
	}
	return nil
}
