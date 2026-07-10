package carddav

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// go-webdav's CardDAV handler implements neither the CalendarServer CTag extension nor
// RFC 6578 collection synchronisation, and offers no way to add them. Without them a
// client has no way to ask "did anything change" — it re-reads every ETag in the address
// book on every poll. Measured on 200 contacts that is 48 KiB per poll, and it grows
// linearly.
//
// These two requests are answered here, before the request reaches go-webdav.
const (
	nsDAV      = "DAV:"
	nsCardDAV  = "urn:ietf:params:xml:ns:carddav"
	nsCalServ  = "http://calendarserver.org/ns/"
	syncPrefix = "urn:contactshq:sync:"
)

func syncToken(seq int64) string {
	return syncPrefix + strconv.FormatInt(seq, 10)
}

// parseSyncToken reads back a token this server issued. An empty token means the client
// has nothing and wants the whole collection.
func parseSyncToken(token string) (int64, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, true
	}
	rest, ok := strings.CutPrefix(token, syncPrefix)
	if !ok {
		return 0, false
	}
	seq, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || seq < 0 {
		return 0, false
	}
	return seq, true
}

// --- request parsing ---

type propfindRequest struct {
	XMLName xml.Name   `xml:"DAV: propfind"`
	Prop    *propNames `xml:"DAV: prop"`
}

type propNames struct {
	Raw []xml.Name `xml:",any"`
}

type syncCollectionRequest struct {
	XMLName   xml.Name   `xml:"DAV: sync-collection"`
	SyncToken string     `xml:"DAV: sync-token"`
	SyncLevel string     `xml:"DAV: sync-level"`
	Prop      *propNames `xml:"DAV: prop"`
}

func (p *propNames) has(space, local string) bool {
	if p == nil {
		return false
	}
	for _, n := range p.Raw {
		if n.Space == space && n.Local == local {
			return true
		}
	}
	return false
}

// --- response writing ---

func writeMultiStatus(w http.ResponseWriter, body string) error {
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, err := io.WriteString(w, xml.Header+body)
	return err
}

// writePreconditionFailed tells the client its sync token is too old to honour, which
// RFC 6578 spells as a 403 carrying the valid-sync-token precondition. Clients respond by
// resynchronising from scratch instead of silently missing deletions.
func writeInvalidSyncToken(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusForbidden)
	_, err := io.WriteString(w, xml.Header+
		`<D:error xmlns:D="DAV:"><D:valid-sync-token/></D:error>`)
	return err
}

func escape(s string) string {
	var sb strings.Builder
	_ = xml.EscapeText(&sb, []byte(s))
	return sb.String()
}

// --- PROPFIND with CTag ---

// handlePropfindCTag answers a Depth:0 PROPFIND on the address book that asks for the
// CTag or a sync token. Anything else it is asked for comes back as 404, which is what a
// DAV server is supposed to say about properties it does not have.
//
// Returns false when the request is not one of ours, so the caller delegates.
func (s *Server) handlePropfindCTag(w http.ResponseWriter, r *http.Request, body []byte) bool {
	if depth := r.Header.Get("Depth"); depth != "" && depth != "0" {
		return false
	}

	var req propfindRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		return false
	}
	wantsCTag := req.Prop.has(nsCalServ, "getctag")
	wantsSyncToken := req.Prop.has(nsDAV, "sync-token")
	if !wantsCTag && !wantsSyncToken {
		return false
	}

	seq, err := s.collectionSeq(r.Context())
	if err != nil {
		http.Error(w, "failed to read collection state", http.StatusInternalServerError)
		return true
	}

	href := AddressBookPath(s.backend.prefix, GetUserEmail(r.Context()))

	var found, missing strings.Builder
	if wantsCTag {
		fmt.Fprintf(&found, `<CS:getctag>%d</CS:getctag>`, seq)
	}
	if wantsSyncToken {
		fmt.Fprintf(&found, `<D:sync-token>%s</D:sync-token>`, escape(syncToken(seq)))
	}
	if req.Prop.has(nsDAV, "resourcetype") {
		found.WriteString(`<D:resourcetype><D:collection/><C:addressbook/></D:resourcetype>`)
	}
	if req.Prop.has(nsDAV, "supported-report-set") {
		found.WriteString(`<D:supported-report-set>` +
			`<D:supported-report><D:report><D:sync-collection/></D:report></D:supported-report>` +
			`<D:supported-report><D:report><C:addressbook-query/></D:report></D:supported-report>` +
			`<D:supported-report><D:report><C:addressbook-multiget/></D:report></D:supported-report>` +
			`</D:supported-report-set>`)
	}

	for _, name := range req.Prop.Raw {
		switch {
		case name.Space == nsCalServ && name.Local == "getctag",
			name.Space == nsDAV && name.Local == "sync-token",
			name.Space == nsDAV && name.Local == "resourcetype",
			name.Space == nsDAV && name.Local == "supported-report-set":
			continue
		}
		fmt.Fprintf(&missing, `<%s xmlns="%s"/>`, escape(name.Local), escape(name.Space))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `<D:multistatus xmlns:D="%s" xmlns:C="%s" xmlns:CS="%s">`, nsDAV, nsCardDAV, nsCalServ)
	fmt.Fprintf(&sb, `<D:response><D:href>%s</D:href>`, escape(href))
	if found.Len() > 0 {
		fmt.Fprintf(&sb, `<D:propstat><D:prop>%s</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>`, found.String())
	}
	if missing.Len() > 0 {
		fmt.Fprintf(&sb, `<D:propstat><D:prop>%s</D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat>`, missing.String())
	}
	sb.WriteString(`</D:response></D:multistatus>`)

	_ = writeMultiStatus(w, sb.String())
	return true
}

// --- REPORT sync-collection ---

// handleSyncCollection answers RFC 6578 collection synchronisation: everything that
// changed since the client's token, deletions named explicitly.
//
// Returns false when the REPORT body is not a sync-collection, so the caller delegates to
// go-webdav's addressbook-query and addressbook-multiget.
func (s *Server) handleSyncCollection(w http.ResponseWriter, r *http.Request, body []byte) bool {
	var req syncCollectionRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		return false
	}
	if req.XMLName.Space != nsDAV || req.XMLName.Local != "sync-collection" {
		return false
	}

	ctx := r.Context()
	sinceSeq, ok := parseSyncToken(req.SyncToken)
	if !ok {
		_ = writeInvalidSyncToken(w)
		return true
	}

	ab, err := s.backend.abRepo.GetOrCreateByUserID(ctx, GetUserID(ctx))
	if err != nil {
		http.Error(w, "failed to load address book", http.StatusInternalServerError)
		return true
	}

	changes, err := s.backend.contactRepo.ChangesSince(ctx, ab.ID, sinceSeq)
	if err != nil {
		http.Error(w, "failed to read changes", http.StatusInternalServerError)
		return true
	}

	// A token from the future belongs to another database — a restored backup, a
	// different server. Honouring it would hand the client an empty delta and leave it
	// permanently out of date.
	if sinceSeq > changes.Seq {
		_ = writeInvalidSyncToken(w)
		return true
	}

	// Tombstones are never pruned, so every token this server issued can still be
	// answered. Pruning them would need a horizon to reject tokens older than the
	// deletions that were forgotten.

	wantsData := req.Prop.has(nsCardDAV, "address-data")
	email := GetUserEmail(ctx)

	var sb strings.Builder
	fmt.Fprintf(&sb, `<D:multistatus xmlns:D="%s" xmlns:C="%s">`, nsDAV, nsCardDAV)

	for _, c := range changes.Updated {
		href := AddressObjectPath(s.backend.prefix, email, c.UID)
		fmt.Fprintf(&sb, `<D:response><D:href>%s</D:href><D:propstat><D:prop>`, escape(href))
		fmt.Fprintf(&sb, `<D:getetag>"%s"</D:getetag>`, escape(c.ETag))
		if wantsData {
			fmt.Fprintf(&sb, `<C:address-data>%s</C:address-data>`, escape(c.VCardData))
		}
		sb.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`)
	}

	for _, uid := range changes.DeletedUIDs {
		href := AddressObjectPath(s.backend.prefix, email, uid)
		fmt.Fprintf(&sb, `<D:response><D:href>%s</D:href><D:status>HTTP/1.1 404 Not Found</D:status></D:response>`, escape(href))
	}

	fmt.Fprintf(&sb, `<D:sync-token>%s</D:sync-token>`, escape(syncToken(changes.Seq)))
	sb.WriteString(`</D:multistatus>`)

	_ = writeMultiStatus(w, sb.String())
	return true
}

func (s *Server) collectionSeq(ctx context.Context) (int64, error) {
	ab, err := s.backend.abRepo.GetOrCreateByUserID(ctx, GetUserID(ctx))
	if err != nil {
		return 0, err
	}
	return s.backend.abRepo.ChangeSeq(ctx, ab.ID)
}
