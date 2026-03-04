package sync

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
)

// basicAuthTransport injects HTTP Basic Auth into every request.
type basicAuthTransport struct {
	username, password string
	base               http.RoundTripper // nil → use http.DefaultTransport
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.username, t.password)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// bearerAuthTransport injects an OAuth2 Bearer token into every request.
// Used for Google CardDAV and other OAuth2-based CardDAV servers.
type bearerAuthTransport struct {
	tokenSource oauth2TokenSource
	base        http.RoundTripper
}

// oauth2TokenSource is a minimal interface matching oauth2.TokenSource.
type oauth2TokenSource interface {
	Token() (*oauth2Token, error)
}

// oauth2Token holds a bearer access token (avoids importing golang.org/x/oauth2 directly).
type oauth2Token struct {
	AccessToken string
}

func (t *bearerAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("get bearer token: %w", err)
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// NewCardDAVClientProviderWithHTTPClient creates a CardDAV provider with a pre-configured HTTP client.
// This is used for OAuth2-authenticated CardDAV servers (e.g., Google CardDAV).
func NewCardDAVClientProviderWithHTTPClient(endpoint string, httpClient *http.Client) (*CardDAVClientProvider, error) {
	resolvedEndpoint, abPath, err := discoverAddressBook(httpClient, endpoint)
	if err != nil {
		return nil, err
	}

	client, err := carddav.NewClient(httpClient, resolvedEndpoint)
	if err != nil {
		return nil, fmt.Errorf("create carddav client: %w", err)
	}

	return &CardDAVClientProvider{
		client: client,
		abPath: abPath,
	}, nil
}

type CardDAVClientProvider struct {
	client *carddav.Client
	abPath string
}

func NewCardDAVClientProvider(endpoint, username, password string) (*CardDAVClientProvider, error) {
	return NewCardDAVClientProviderWithOptions(endpoint, username, password, false)
}

func NewCardDAVClientProviderWithOptions(endpoint, username, password string, skipTLSVerify bool) (*CardDAVClientProvider, error) {
	var base http.RoundTripper
	if skipTLSVerify {
		base = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // user-opted in
		}
	}
	httpClient := &http.Client{
		Transport: &basicAuthTransport{
			username: username,
			password: password,
			base:     base,
		},
	}

	resolvedEndpoint, abPath, err := discoverAddressBook(httpClient, endpoint)
	if err != nil {
		return nil, err
	}

	// Create final client pointing to the resolved base URL (may differ from original
	// if .well-known redirected to a different host/path).
	client, err := carddav.NewClient(httpClient, resolvedEndpoint)
	if err != nil {
		return nil, fmt.Errorf("create carddav client: %w", err)
	}

	return &CardDAVClientProvider{
		client: client,
		abPath: abPath,
	}, nil
}

// discoverAddressBook tries multiple RFC 6764 strategies to find the address book path.
// Returns (resolvedEndpoint, abPath, error).
// resolvedEndpoint may differ from the input when a .well-known redirect leads to another host.
//
//  1. Standard discovery: FindCurrentUserPrincipal → FindAddressBookHomeSet → FindAddressBooks
//  2. .well-known/carddav (RFC 6764): HTTP GET follows the redirect, then run discovery on the final URL
//  3. DNS SRV/TXT (RFC 6764 §11): carddav.DiscoverContextURL → discovery on returned URL
//  4. Treat u.Path as a direct address book path (user provided a full address book URL)
func discoverAddressBook(httpClient *http.Client, endpoint string) (resolvedEndpoint, abPath string, err error) {
	ctx := context.Background()

	u, parseErr := url.Parse(endpoint)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid endpoint URL: %w", parseErr)
	}

	// Strategy 1: full discovery at the provided endpoint.
	if path, e := tryDiscoverFull(ctx, httpClient, endpoint); e == nil {
		return endpoint, path, nil
	}

	// Strategy 2: .well-known/carddav — use an HTTP GET so redirects are followed,
	// then run full discovery at the final (redirected) URL.
	if finalURL, e := resolveWellKnown(httpClient, u); e == nil {
		if path, e2 := tryDiscoverFull(ctx, httpClient, finalURL); e2 == nil {
			return finalURL, path, nil
		}
	}

	// Strategy 3: DNS SRV + TXT records (only when no explicit path was given).
	if u.Path == "" || u.Path == "/" {
		if dnsURL, e := carddav.DiscoverContextURL(ctx, u.Host); e == nil {
			if path, e2 := tryDiscoverFull(ctx, httpClient, dnsURL); e2 == nil {
				return dnsURL, path, nil
			}
		}
	}

	// Strategy 4: treat the URL's path component as a direct address book path.
	// The user likely provided something like https://dav.example.com/addressbooks/user/default/.
	p := u.Path
	if p == "" {
		p = "/"
	}
	return endpoint, p, nil
}

// resolveWellKnown performs a plain HTTP GET to /.well-known/carddav on the given host.
// The http.Client follows redirects automatically; we return the final request URL.
func resolveWellKnown(httpClient *http.Client, u *url.URL) (string, error) {
	wellKnownURL := u.Scheme + "://" + u.Host + "/.well-known/carddav"
	resp, err := httpClient.Get(wellKnownURL) //nolint:noctx // background discovery
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("well-known returned HTTP %d", resp.StatusCode)
	}
	// resp.Request.URL is the final URL after all redirects.
	return resp.Request.URL.String(), nil
}

// tryDiscoverFull implements the three-step CardDAV principal discovery:
//  1. PROPFIND {DAV:}current-user-principal
//  2. PROPFIND {urn:ietf:params:xml:ns:carddav}addressbook-home-set
//  3. PROPFIND (Depth:1) on the home set to enumerate address books
func tryDiscoverFull(ctx context.Context, httpClient *http.Client, endpoint string) (string, error) {
	client, err := carddav.NewClient(httpClient, endpoint)
	if err != nil {
		return "", err
	}

	// Step 1: resolve the current-user-principal URL.
	// If the server does not support this property (old servers), fall back to "".
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		principal = ""
	}

	// Step 2: find addressbook-home-set at the principal URL.
	homeSet, err := client.FindAddressBookHomeSet(ctx, principal)
	if err != nil {
		return "", fmt.Errorf("find home set: %w", err)
	}

	// Step 3: enumerate address books under the home set.
	books, err := client.FindAddressBooks(ctx, homeSet)
	if err != nil {
		return "", fmt.Errorf("find address books: %w", err)
	}
	if len(books) == 0 {
		return "", fmt.Errorf("no address books found at %s", endpoint)
	}
	return books[0].Path, nil
}

func (p *CardDAVClientProvider) Name() string {
	return "carddav"
}

func (p *CardDAVClientProvider) List(ctx context.Context) ([]SyncItem, error) {
	objects, err := p.client.QueryAddressBook(ctx, p.abPath, &carddav.AddressBookQuery{
		DataRequest: carddav.AddressDataRequest{AllProp: true},
	})
	if err != nil {
		return nil, fmt.Errorf("query address book: %w", err)
	}

	items := make([]SyncItem, 0, len(objects))
	for _, obj := range objects {
		vcardData := cardToString(obj.Card)
		h := sha256.Sum256([]byte(vcardData))
		uid := getUID(obj.Card)
		if uid == "" {
			uid = extractUIDFromPath(obj.Path)
		}

		items = append(items, SyncItem{
			RemoteID:    uid,
			ETag:        obj.ETag,
			ContentHash: hex.EncodeToString(h[:]),
			VCardData:   vcardData,
		})
	}

	return items, nil
}

func (p *CardDAVClientProvider) Get(ctx context.Context, remoteID string) (*SyncItem, error) {
	obj, err := p.client.GetAddressObject(ctx, p.abPath+remoteID+".vcf")
	if err != nil {
		return nil, err
	}

	vcardData := cardToString(obj.Card)
	h := sha256.Sum256([]byte(vcardData))

	return &SyncItem{
		RemoteID:    remoteID,
		ETag:        obj.ETag,
		ContentHash: hex.EncodeToString(h[:]),
		VCardData:   vcardData,
	}, nil
}

func (p *CardDAVClientProvider) Put(ctx context.Context, item SyncItem) (string, error) {
	card, err := vcard.NewDecoder(strings.NewReader(item.VCardData)).Decode()
	if err != nil {
		return "", fmt.Errorf("decode vcard: %w", err)
	}

	path := p.abPath + item.RemoteID + ".vcf"
	obj, err := p.client.PutAddressObject(ctx, path, card)
	if err != nil {
		return "", err
	}

	return obj.ETag, nil
}

func (p *CardDAVClientProvider) Delete(ctx context.Context, remoteID string) error {
	path := p.abPath + remoteID + ".vcf"
	return p.client.RemoveAll(ctx, path)
}

func cardToString(card vcard.Card) string {
	var sb strings.Builder
	enc := vcard.NewEncoder(&sb)
	_ = enc.Encode(card)
	return sb.String()
}

func getUID(card vcard.Card) string {
	f := card.Get(vcard.FieldUID)
	if f == nil {
		return ""
	}
	return f.Value
}

func extractUIDFromPath(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSuffix(parts[len(parts)-1], ".vcf")
}
