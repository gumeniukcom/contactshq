package carddav_test

import (
	"context"
	"database/sql"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	chqcarddav "github.com/gumeniukcom/contactshq/internal/carddav"
	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const (
	testEmail    = "user@example.com"
	testPassword = "correct-horse-battery-staple"
	davPrefix    = "/dav"
)

// Migrate() globs migrations/ relative to the working directory.
func TestMain(m *testing.M) {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			_ = os.Chdir(dir)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	os.Exit(m.Run())
}

func setupServer(t *testing.T) (*chqcarddav.Server, *bun.DB, string) {
	t.Helper()
	ctx := context.Background()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, repository.Migrate(ctx, db))

	userRepo := repository.NewBunUserRepository(db)
	abRepo := repository.NewBunAddressBookRepository(db)
	contactRepo := repository.NewBunContactRepository(db)
	appPwRepo := repository.NewBunAppPasswordRepository(db)

	// Register through the auth service so the password hash matches what CardDAV verifies.
	authSvc := service.NewAuthService(userRepo, abRepo, config.AuthConfig{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		TokenTTL:  time.Hour,
	})
	user, err := authSvc.Register(ctx, testEmail, testPassword, "Test User")
	require.NoError(t, err)

	backend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, davPrefix)
	srv := chqcarddav.NewServer(backend, userRepo, appPwRepo, davPrefix)

	return srv, db, user.ID
}

func do(t *testing.T, srv *chqcarddav.Server, method, path, body string, headers map[string]string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.SetBasicAuth(testEmail, testPassword)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Result()
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

// hrefs pulls every <href> out of a DAV multistatus body.
func hrefs(t *testing.T, body string) []string {
	t.Helper()

	var ms struct {
		Responses []struct {
			Href string `xml:"href"`
		} `xml:"response"`
	}
	require.NoError(t, xml.Unmarshal([]byte(body), &ms))

	out := make([]string, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		out = append(out, r.Href)
	}
	return out
}

const sampleVCard = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:contact-1\r\nFN:Jane Doe\r\nN:Doe;Jane;;;\r\n" +
	"EMAIL;TYPE=work:jane@example.com\r\nTEL;TYPE=cell:+15551234567\r\nEND:VCARD\r\n"

func objectPath() string { return chqcarddav.AddressObjectPath(davPrefix, testEmail, "contact-1") }

func TestAuthRequired(t *testing.T) {
	srv, _, _ := setupServer(t)

	req := httptest.NewRequest("PROPFIND", davPrefix+"/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Header().Get("WWW-Authenticate"), "Basic")
}

func TestWrongPasswordRejected(t *testing.T) {
	srv, _, _ := setupServer(t)

	req := httptest.NewRequest("PROPFIND", davPrefix+"/", nil)
	req.SetBasicAuth(testEmail, "wrong")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// The path depths below are what go-webdav uses to classify a resource. Getting one
// wrong makes the address book list zero contacts and DELETE hit DeleteAddressBook.
func TestPathHierarchyDepths(t *testing.T) {
	depth := func(p string) int {
		trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSuffix(p, "/"), davPrefix), "/")
		return len(strings.Split(trimmed, "/"))
	}

	require.Equal(t, 1, depth(chqcarddav.PrincipalPath(davPrefix, testEmail)), "principal")
	require.Equal(t, 2, depth(chqcarddav.HomeSetPath(davPrefix, testEmail)), "home set")
	require.Equal(t, 3, depth(chqcarddav.AddressBookPath(davPrefix, testEmail)), "address book")
	require.Equal(t, 4, depth(chqcarddav.AddressObjectPath(davPrefix, testEmail, "u1")), "address object")
}

func TestPropfindRootExposesPrincipal(t *testing.T) {
	srv, _, _ := setupServer(t)

	resp := do(t, srv, "PROPFIND", davPrefix+"/", "", map[string]string{"Depth": "0"})
	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	body := readBody(t, resp)
	require.Contains(t, body, chqcarddav.PrincipalPath(davPrefix, testEmail))
}

func TestPropfindHomeSetListsAddressBook(t *testing.T) {
	srv, _, _ := setupServer(t)

	resp := do(t, srv, "PROPFIND", chqcarddav.HomeSetPath(davPrefix, testEmail), "",
		map[string]string{"Depth": "1"})
	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	require.Contains(t, hrefs(t, readBody(t, resp)), chqcarddav.AddressBookPath(davPrefix, testEmail))
}

// The regression this whole path rework exists for: a client syncing the address book
// must actually receive its contacts.
func TestPropfindAddressBookListsContacts(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	resp := do(t, srv, "PROPFIND", chqcarddav.AddressBookPath(davPrefix, testEmail), "",
		map[string]string{"Depth": "1"})
	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	got := hrefs(t, readBody(t, resp))
	require.Contains(t, got, objectPath(), "address book PROPFIND must list the stored contact")
}

func TestPutGetRoundTrip(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	get := do(t, srv, http.MethodGet, objectPath(), "", nil)
	require.Equal(t, http.StatusOK, get.StatusCode)

	body := readBody(t, get)
	require.Contains(t, body, "UID:contact-1")
	require.Contains(t, body, "Jane")
	require.Contains(t, body, "jane@example.com")
}

// PUT must land in the contacts table so the REST API and CardDAV agree.
func TestPutPersistsContact(t *testing.T) {
	srv, db, userID := setupServer(t)
	ctx := context.Background()

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	ab, err := repository.NewBunAddressBookRepository(db).GetOrCreateByUserID(ctx, userID)
	require.NoError(t, err)

	contact, err := repository.NewBunContactRepository(db).GetByUID(ctx, ab.ID, "contact-1")
	require.NoError(t, err)
	require.NotNil(t, contact)
	require.Equal(t, "Jane", contact.FirstName)
	require.Equal(t, "Doe", contact.LastName)
}

// DELETE on an object path used to be dispatched to DeleteAddressBook and 500.
func TestDeleteAddressObject(t *testing.T) {
	srv, db, userID := setupServer(t)
	ctx := context.Background()

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	del := do(t, srv, http.MethodDelete, objectPath(), "", nil)
	require.Equal(t, http.StatusNoContent, del.StatusCode, "DELETE of a contact must succeed")

	ab, err := repository.NewBunAddressBookRepository(db).GetOrCreateByUserID(ctx, userID)
	require.NoError(t, err)
	contact, err := repository.NewBunContactRepository(db).GetByUID(ctx, ab.ID, "contact-1")
	require.NoError(t, err)
	require.Nil(t, contact, "contact must be gone from the database")
}

func TestReportAddressbookQuery(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	const query = `<?xml version="1.0" encoding="utf-8"?>
<C:addressbook-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:prop><D:getetag/></D:prop>
</C:addressbook-query>`

	resp := do(t, srv, "REPORT", chqcarddav.AddressBookPath(davPrefix, testEmail), query,
		map[string]string{"Content-Type": "application/xml", "Depth": "1"})
	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	require.Contains(t, hrefs(t, readBody(t, resp)), objectPath())
}

// An app password must authenticate CardDAV just like the account password.
func TestAppPasswordAuthenticates(t *testing.T) {
	srv, db, userID := setupServer(t)
	ctx := context.Background()

	appPwSvc := service.NewAppPasswordService(repository.NewBunAppPasswordRepository(db))
	token, _, err := appPwSvc.Create(ctx, userID, "iPhone")
	require.NoError(t, err)

	req := httptest.NewRequest("PROPFIND", davPrefix+"/", nil)
	req.SetBasicAuth(testEmail, token)
	req.Header.Set("Depth", "0")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusMultiStatus, rec.Code)
}

// Guard against UID collisions being silently accepted from a foreign address book.
func TestPutUpdatesExistingContact(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	updated := strings.Replace(sampleVCard, "FN:Jane Doe", "FN:Jane Smith", 1)
	updated = strings.Replace(updated, "N:Doe;Jane;;;", "N:Smith;Jane;;;", 1)

	put2 := do(t, srv, http.MethodPut, objectPath(), updated,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put2.StatusCode)

	get := do(t, srv, http.MethodGet, objectPath(), "", nil)
	body := readBody(t, get)
	require.Contains(t, body, "Smith")
	require.NotContains(t, body, "Doe")
}

// etagOf reads the ETag a PUT or GET reported for a contact.
func etagOf(t *testing.T, srv *chqcarddav.Server, path string) string {
	t.Helper()

	resp := do(t, srv, http.MethodGet, path, "", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	etag := resp.Header.Get("ETag")
	require.NotEmpty(t, etag, "server must expose an ETag for conditional requests")
	return etag
}

// Two devices editing the same contact used to overwrite each other in silence: the
// If-Match header a client sends to guard against that was ignored.
func TestPutWithStaleIfMatchIsRejected(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	staleETag := etagOf(t, srv, objectPath())

	// Someone else saves a new version, moving the ETag on.
	updated := strings.Replace(sampleVCard, "FN:Jane Doe", "FN:Jane Elsewhere", 1)
	put2 := do(t, srv, http.MethodPut, objectPath(), updated,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put2.StatusCode)

	// Our client still holds the old ETag and must be refused.
	mine := strings.Replace(sampleVCard, "FN:Jane Doe", "FN:Jane Mine", 1)
	resp := do(t, srv, http.MethodPut, objectPath(), mine, map[string]string{
		"Content-Type": "text/vcard",
		"If-Match":     staleETag,
	})
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)

	get := do(t, srv, http.MethodGet, objectPath(), "", nil)
	require.Contains(t, readBody(t, get), "Jane Elsewhere", "the other device's edit must survive")
}

func TestPutWithCurrentIfMatchSucceeds(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	current := etagOf(t, srv, objectPath())

	updated := strings.Replace(sampleVCard, "FN:Jane Doe", "FN:Jane Updated", 1)
	resp := do(t, srv, http.MethodPut, objectPath(), updated, map[string]string{
		"Content-Type": "text/vcard",
		"If-Match":     current,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	get := do(t, srv, http.MethodGet, objectPath(), "", nil)
	require.Contains(t, readBody(t, get), "Jane Updated")
}

// "If-None-Match: *" means create-only. A client using it must not clobber a contact
// that already exists.
func TestPutWithIfNoneMatchRejectsExistingContact(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	updated := strings.Replace(sampleVCard, "FN:Jane Doe", "FN:Should Not Land", 1)
	resp := do(t, srv, http.MethodPut, objectPath(), updated, map[string]string{
		"Content-Type":  "text/vcard",
		"If-None-Match": "*",
	})
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)

	get := do(t, srv, http.MethodGet, objectPath(), "", nil)
	require.Contains(t, readBody(t, get), "Jane Doe")
}

func TestPutWithIfNoneMatchCreatesNewContact(t *testing.T) {
	srv, _, _ := setupServer(t)

	resp := do(t, srv, http.MethodPut, objectPath(), sampleVCard, map[string]string{
		"Content-Type":  "text/vcard",
		"If-None-Match": "*",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

// If-Match on a contact that does not exist cannot be satisfied.
func TestPutWithIfMatchOnMissingContactIsRejected(t *testing.T) {
	srv, _, _ := setupServer(t)

	resp := do(t, srv, http.MethodPut, objectPath(), sampleVCard, map[string]string{
		"Content-Type": "text/vcard",
		"If-Match":     `"whatever"`,
	})
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
}

// go-webdav quotes the ETag when it writes the header, so the backend must hand it the
// bare value. Quoting it twice yields `""abc""`, which no client can match against.
func TestETagHeaderIsSingleQuoted(t *testing.T) {
	srv, _, _ := setupServer(t)

	put := do(t, srv, http.MethodPut, objectPath(), sampleVCard,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, put.StatusCode)

	for name, etag := range map[string]string{
		"PUT": put.Header.Get("ETag"),
		"GET": etagOf(t, srv, objectPath()),
	} {
		require.NotEmpty(t, etag, "%s must return an ETag", name)
		require.True(t, strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`),
			"%s ETag %q must be quoted", name, etag)
		inner := strings.Trim(etag, `"`)
		require.NotContains(t, inner, `"`, "%s ETag %q is doubly quoted", name, etag)
		require.NotEmpty(t, inner)
	}
}

// --- CTag and RFC 6578 collection synchronisation ---

const xmlContentType = "application/xml"

func bookPath() string { return chqcarddav.AddressBookPath(davPrefix, testEmail) }

func propfind(t *testing.T, srv *chqcarddav.Server, depth, body string) (*http.Response, string) {
	t.Helper()

	resp := do(t, srv, "PROPFIND", bookPath(), body,
		map[string]string{"Depth": depth, "Content-Type": xmlContentType})
	return resp, readBody(t, resp)
}

func syncCollection(t *testing.T, srv *chqcarddav.Server, token string, withData bool) (*http.Response, string) {
	t.Helper()

	data := ""
	if withData {
		data = `<c:address-data/>`
	}
	body := `<?xml version="1.0"?><d:sync-collection xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">` +
		`<d:sync-token>` + token + `</d:sync-token><d:sync-level>1</d:sync-level>` +
		`<d:prop><d:getetag/>` + data + `</d:prop></d:sync-collection>`

	resp := do(t, srv, "REPORT", bookPath(), body,
		map[string]string{"Depth": "1", "Content-Type": xmlContentType})
	return resp, readBody(t, resp)
}

func ctagOf(t *testing.T, srv *chqcarddav.Server) string {
	t.Helper()

	_, body := propfind(t, srv, "0",
		`<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/">`+
			`<d:prop><cs:getctag/></d:prop></d:propfind>`)

	m := regexp.MustCompile(`<CS:getctag>([^<]*)</CS:getctag>`).FindStringSubmatch(body)
	require.Len(t, m, 2, "no CTag in response: %s", body)
	return m[1]
}

func tokenOf(t *testing.T, body string) string {
	t.Helper()

	m := regexp.MustCompile(`<D:sync-token>([^<]*)</D:sync-token>`).FindStringSubmatch(body)
	require.Len(t, m, 2, "no sync token in response: %s", body)
	return m[1]
}

func putContact(t *testing.T, srv *chqcarddav.Server, uid string) {
	t.Helper()

	card := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + uid + "\r\nFN:" + uid + "\r\nEND:VCARD\r\n"
	resp := do(t, srv, http.MethodPut, chqcarddav.AddressObjectPath(davPrefix, testEmail, uid), card,
		map[string]string{"Content-Type": "text/vcard"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

// Without a CTag a client cannot ask "did anything change" and re-reads every ETag on
// every poll.
func TestCTagChangesOnWriteAndIsStableOtherwise(t *testing.T) {
	srv, _, _ := setupServer(t)

	before := ctagOf(t, srv)
	require.Equal(t, before, ctagOf(t, srv), "reading the collection must not change its CTag")

	putContact(t, srv, "u1")
	afterCreate := ctagOf(t, srv)
	require.NotEqual(t, before, afterCreate, "a new contact must advance the CTag")

	putContact(t, srv, "u1") // update
	afterUpdate := ctagOf(t, srv)
	require.NotEqual(t, afterCreate, afterUpdate, "an updated contact must advance the CTag")

	del := do(t, srv, http.MethodDelete, chqcarddav.AddressObjectPath(davPrefix, testEmail, "u1"), "", nil)
	require.Equal(t, http.StatusNoContent, del.StatusCode)
	require.NotEqual(t, afterUpdate, ctagOf(t, srv), "a deleted contact must advance the CTag")
}

func TestPropfindAdvertisesSyncCollectionSupport(t *testing.T) {
	srv, _, _ := setupServer(t)

	_, body := propfind(t, srv, "0",
		`<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/">`+
			`<d:prop><cs:getctag/><d:supported-report-set/></d:prop></d:propfind>`)

	require.Contains(t, body, "sync-collection", "clients discover support through supported-report-set")
	require.Contains(t, body, "addressbook-multiget")
}

// A PROPFIND that does not ask for our extensions must reach go-webdav untouched.
func TestPropfindWithoutCTagIsDelegated(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")

	resp, body := propfind(t, srv, "1",
		`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getetag/></d:prop></d:propfind>`)

	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)
	require.Contains(t, body, "u1.vcf")
}

func TestSyncCollection_EmptyTokenReturnsWholeCollection(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")
	putContact(t, srv, "u2")

	resp, body := syncCollection(t, srv, "", false)

	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)
	require.Contains(t, body, "u1.vcf")
	require.Contains(t, body, "u2.vcf")
	require.NotContains(t, body, "404 Not Found", "a first sync has nothing to delete")
	require.NotEmpty(t, tokenOf(t, body))
}

// The point of the whole exercise: a poll that finds nothing new says nothing.
func TestSyncCollection_UnchangedCollectionReturnsEmptyDelta(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")

	_, first := syncCollection(t, srv, "", false)
	_, second := syncCollection(t, srv, tokenOf(t, first), false)

	require.NotContains(t, second, "u1.vcf", "a contact the client already has must not repeat")
	require.Equal(t, tokenOf(t, first), tokenOf(t, second), "an unchanged collection keeps its token")
}

func TestSyncCollection_ReportsCreationsAndDeletions(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")
	putContact(t, srv, "u2")

	_, first := syncCollection(t, srv, "", false)
	token := tokenOf(t, first)

	putContact(t, srv, "u3")
	del := do(t, srv, http.MethodDelete, chqcarddav.AddressObjectPath(davPrefix, testEmail, "u1"), "", nil)
	require.Equal(t, http.StatusNoContent, del.StatusCode)

	_, body := syncCollection(t, srv, token, false)

	require.Contains(t, body, "u3.vcf", "the new contact must be reported")
	require.Contains(t, body, "u1.vcf", "the deleted contact must be named")
	require.Contains(t, body, "404 Not Found", "deletion is spelled as a 404 response")
	require.NotContains(t, body, "u2.vcf", "an untouched contact must not repeat")
}

func TestSyncCollection_CarriesCardsWhenAddressDataRequested(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")

	_, body := syncCollection(t, srv, "", true)

	require.Contains(t, body, "address-data")
	// The card is XML-escaped; a client's parser restores it.
	require.Contains(t, body, "BEGIN:VCARD")
	require.Contains(t, body, "UID:u1")
}

// A token this server never issued must not be honoured: answering an empty delta would
// leave the client permanently out of date.
func TestSyncCollection_RejectsUnknownTokens(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")

	for name, token := range map[string]string{
		"garbage":       "garbage",
		"foreign":       "http://other.example/sync/7",
		"from the futu": "urn:contactshq:sync:9999",
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := syncCollection(t, srv, token, false)
			require.Equal(t, http.StatusForbidden, resp.StatusCode)
			require.Contains(t, body, "valid-sync-token")
		})
	}
}

// addressbook-query and addressbook-multiget must still reach go-webdav.
func TestReportOtherThanSyncCollectionIsDelegated(t *testing.T) {
	srv, _, _ := setupServer(t)
	putContact(t, srv, "u1")

	const query = `<?xml version="1.0"?><C:addressbook-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">` +
		`<D:prop><D:getetag/></D:prop></C:addressbook-query>`

	resp := do(t, srv, "REPORT", bookPath(), query,
		map[string]string{"Depth": "1", "Content-Type": xmlContentType})
	require.Equal(t, http.StatusMultiStatus, resp.StatusCode)
	require.Contains(t, readBody(t, resp), "u1.vcf")
}
