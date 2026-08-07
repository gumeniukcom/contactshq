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
	"time"

	"github.com/emersion/go-vcard"

	"github.com/emersion/go-webdav/carddav"
	vcardpkg "github.com/gumeniukcom/contactshq/internal/vcard"
)

// defaultHTTPTimeout caps every request to a remote CardDAV server. Without it a host that
// accepts the connection and then goes silent parks a worker goroutine forever; the pool is
// four goroutines wide, so four such syncs stop backups and dedup as well.
const defaultHTTPTimeout = 30 * time.Second

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

// NewCardDAVClientProviderWithHTTPClient creates a CardDAV provider with a pre-configured HTTP client.
// This is used for OAuth2-authenticated CardDAV servers (e.g., Google CardDAV).
func NewCardDAVClientProviderWithHTTPClient(ctx context.Context, endpoint string, httpClient *http.Client) (*CardDAVClientProvider, error) {
	return newCardDAVClientProvider(ctx, endpoint, withTimeout(httpClient))
}

// withTimeout returns a client that is guaranteed to have a request deadline. The caller may
// own the client (an OAuth2 one, say), so a copy is timed out rather than the original.
func withTimeout(c *http.Client) *http.Client {
	if c.Timeout > 0 && c.CheckRedirect != nil {
		return c
	}
	clone := *c
	if clone.Timeout == 0 {
		clone.Timeout = defaultHTTPTimeout
	}
	if clone.CheckRedirect == nil {
		clone.CheckRedirect = redirectPolicy
	}
	return &clone
}

// newCardDAVClientProvider is the single place a provider is assembled. Both exported
// constructors go through it: when they each built the struct themselves, one of them
// silently omitted httpClient and baseURL and every conditional PUT nil-panicked.
func newCardDAVClientProvider(ctx context.Context, endpoint string, httpClient *http.Client) (*CardDAVClientProvider, error) {
	resolvedEndpoint, abPath, err := discoverAddressBook(ctx, httpClient, endpoint)
	if err != nil {
		return nil, err
	}

	client, err := carddav.NewClient(httpClient, resolvedEndpoint)
	if err != nil {
		return nil, fmt.Errorf("create carddav client: %w", err)
	}

	base, err := url.Parse(resolvedEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	return &CardDAVClientProvider{
		client:     client,
		httpClient: httpClient,
		baseURL:    base,
		abPath:     abPath,
	}, nil
}

type CardDAVClientProvider struct {
	client *carddav.Client
	// httpClient and baseURL back the conditional PUT that go-webdav's client cannot do:
	// its PutAddressObject sends no If-Match.
	httpClient *http.Client
	baseURL    *url.URL
	abPath     string
}

func NewCardDAVClientProvider(ctx context.Context, endpoint, username, password string) (*CardDAVClientProvider, error) {
	return NewCardDAVClientProviderWithOptions(ctx, endpoint, username, password, false)
}

func NewCardDAVClientProviderWithOptions(ctx context.Context, endpoint, username, password string, skipTLSVerify bool) (*CardDAVClientProvider, error) {
	var base http.RoundTripper
	if skipTLSVerify {
		base = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // user-opted in
		}
	}
	httpClient := &http.Client{
		Timeout: defaultHTTPTimeout,
		// Validating the URL a user typed is not enough on its own: a permitted host can
		// answer 302 Location: http://169.254.169.254/ and the client would follow it.
		// Discovery does exactly this — resolveWellKnown issues a GET and reads the final URL.
		CheckRedirect: redirectPolicy,
		Transport: &basicAuthTransport{
			username: username,
			password: password,
			base:     base,
		},
	}

	// The resolved endpoint may differ from the original when .well-known redirected to
	// another host or path, so the provider is built from what discovery returned.
	return newCardDAVClientProvider(ctx, endpoint, httpClient)
}

// discoverAddressBook tries multiple RFC 6764 strategies to find the address book path.
// Returns (resolvedEndpoint, abPath, error).
// resolvedEndpoint may differ from the input when a .well-known redirect leads to another host.
//
//  1. Standard discovery: FindCurrentUserPrincipal → FindAddressBookHomeSet → FindAddressBooks
//  2. .well-known/carddav (RFC 6764): HTTP GET follows the redirect, then run discovery on the final URL
//  3. DNS SRV/TXT (RFC 6764 §11): carddav.DiscoverContextURL → discovery on returned URL
//  4. Treat u.Path as a direct address book path (user provided a full address book URL)
func discoverAddressBook(ctx context.Context, httpClient *http.Client, endpoint string) (resolvedEndpoint, abPath string, err error) {
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
	if finalURL, e := resolveWellKnown(ctx, httpClient, u); e == nil {
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
func resolveWellKnown(ctx context.Context, httpClient *http.Client, u *url.URL) (string, error) {
	wellKnownURL := u.Scheme + "://" + u.Host + "/.well-known/carddav"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
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

var (
	_ IncrementalProvider = (*CardDAVClientProvider)(nil)
	_ ConditionalWriter   = (*CardDAVClientProvider)(nil)
)

// objectURL is the absolute URL of a contact within the address book.
func (p *CardDAVClientProvider) objectURL(uid string) string {
	return p.baseURL.ResolveReference(&url.URL{Path: p.abPath + uid + ".vcf"}).String()
}

// PutIfMatch writes a contact with an If-Match header, so the server rejects the write if
// someone else changed it since ifMatch. go-webdav's client sends no If-Match, so this is
// a raw request.
func (p *CardDAVClientProvider) PutIfMatch(ctx context.Context, item SyncItem, ifMatch string) (PutResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.objectURL(item.RemoteID), strings.NewReader(item.VCardData))
	if err != nil {
		return PutResult{}, err
	}
	req.Header.Set("Content-Type", "text/vcard; charset=utf-8")
	if ifMatch == "" {
		req.Header.Set("If-None-Match", "*")
	} else {
		req.Header.Set("If-Match", quoteETag(ifMatch))
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PutResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusPreconditionFailed {
		return PutResult{}, ErrPreconditionFailed
	}
	if resp.StatusCode >= 400 {
		return PutResult{}, fmt.Errorf("put %s: HTTP %d", item.RemoteID, resp.StatusCode)
	}

	etag := unquoteETag(resp.Header.Get("ETag"))
	if etag == "" {
		// Some servers omit the ETag on PUT; read it back so sync state stays accurate.
		if obj, gerr := p.client.GetAddressObject(ctx, p.abPath+item.RemoteID+".vcf"); gerr == nil {
			etag = obj.ETag
		}
	}
	return PutResult{RemoteID: item.RemoteID, ETag: etag}, nil
}

func quoteETag(etag string) string {
	if strings.HasPrefix(etag, `"`) {
		return etag
	}
	return `"` + etag + `"`
}

func unquoteETag(etag string) string {
	return strings.Trim(etag, `"`)
}

// ListChanges fetches a delta via RFC 6578 sync-collection, then MultiGET for the card
// bodies the sync report does not carry.
//
// Not every CardDAV server implements sync-collection. When one fails to — on the first
// sync, where the cursor is empty — this falls back to a full List and reports no cursor,
// so the engine keeps doing full syncs against that server. When a stored token is
// rejected, it surfaces as ErrCursorExpired and the engine re-lists in full.
func (p *CardDAVClientProvider) ListChanges(ctx context.Context, cursor string) (Delta, error) {
	resp, err := p.client.SyncCollection(ctx, p.abPath, &carddav.SyncQuery{
		DataRequest: carddav.AddressDataRequest{AllProp: false},
		SyncToken:   cursor,
	})
	if err != nil {
		if cursor == "" {
			// The server does not support sync-collection. Fall back to a full listing;
			// with no cursor stored, every run stays a full sync.
			items, listErr := p.List(ctx)
			if listErr != nil {
				return Delta{}, listErr
			}
			return Delta{Updated: items, Full: true}, nil
		}
		// A stored token the server no longer accepts.
		return Delta{}, ErrCursorExpired
	}

	delta := Delta{Cursor: resp.SyncToken, Full: cursor == ""}

	for _, path := range resp.Deleted {
		delta.Deleted = append(delta.Deleted, extractUIDFromPath(path))
	}

	// sync-collection carries etags but not card bodies; fetch the changed ones.
	if len(resp.Updated) > 0 {
		paths := make([]string, 0, len(resp.Updated))
		for _, obj := range resp.Updated {
			paths = append(paths, obj.Path)
		}

		objects, err := p.client.MultiGetAddressBook(ctx, p.abPath, &carddav.AddressBookMultiGet{
			Paths:       paths,
			DataRequest: carddav.AddressDataRequest{AllProp: true},
		})
		if err != nil {
			return Delta{}, fmt.Errorf("multiget changed contacts: %w", err)
		}

		for _, obj := range objects {
			vcardData := cardToString(obj.Card)
			h := sha256.Sum256([]byte(vcardData))
			uid := getUID(obj.Card)
			if uid == "" {
				uid = extractUIDFromPath(obj.Path)
			}
			delta.Updated = append(delta.Updated, SyncItem{
				RemoteID:    uid,
				ETag:        obj.ETag,
				ContentHash: hex.EncodeToString(h[:]),
				VCardData:   vcardData,
			})
		}
	}

	return delta, nil
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

func (p *CardDAVClientProvider) Put(ctx context.Context, item SyncItem) (PutResult, error) {
	card, err := vcard.NewDecoder(strings.NewReader(item.VCardData)).Decode()
	if err != nil {
		return PutResult{}, fmt.Errorf("decode vcard: %w", err)
	}

	path := p.abPath + item.RemoteID + ".vcf"
	obj, err := p.client.PutAddressObject(ctx, path, card)
	if err != nil {
		return PutResult{}, err
	}

	// CardDAV stores the object at the path we chose, so the id never changes.
	return PutResult{RemoteID: item.RemoteID, ETag: obj.ETag}, nil
}

func (p *CardDAVClientProvider) Delete(ctx context.Context, remoteID string) error {
	path := p.abPath + remoteID + ".vcf"
	return p.client.RemoveAll(ctx, path)
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

// cardToString delegates to the shared encoder. It used to be a local copy that called
// go-vcard directly, which is how this package kept serialising photos with an escaped
// comma after the bug was fixed in internal/vcard.
func cardToString(card vcard.Card) string {
	return vcardpkg.CardToString(card)
}

// BaseURLForTest exposes the resolved base URL so a test can assert where discovery ended up.
func (p *CardDAVClientProvider) BaseURLForTest() string {
	if p.baseURL == nil {
		return ""
	}
	return p.baseURL.String()
}
