# Feature Specification: CardDAV Service & Device Setup

Kind: journey
Status: shipped
Constitution: v1.0.0

This spec is retrospective: it was reconstructed from the implementation at commit `23a167c`
(tag `v0.4.0`), not written before it. Every requirement below was read out of the code at that
commit, and the parentheticals name the file — and the line, where it pins the claim down —
that makes each one true. It covers ContactsHQ acting as a **CardDAV server** for phones and
desktop clients, plus the discovery and onboarding surfaces that get a device connected.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Sync contacts to a phone with the built-in CardDAV account (Priority: P1)

A user adds a CardDAV account on iOS, macOS, or Thunderbird pointing at their ContactsHQ
host. The device discovers the principal, finds the one address book, downloads every
contact, and from then on creates, edits and deletes contacts that appear in the web UI —
and sees web-UI changes appear on the phone.

**Why this priority**: This is the product's reason for existing as a *hub*. Everything else
in this spec — CTag, sync tokens, the profile, the guide — exists to make this one journey
work and keep working.

**Independent Test**: Register an account, PUT a card at the object URL, PROPFIND the
collection, GET the card back, DELETE it. `internal/carddav/carddav_e2e_test.go` does exactly
this against a real in-memory database and the real HTTP surface
(`TestPropfindRootExposesPrincipal:161`, `TestPropfindHomeSetListsAddressBook:171`,
`TestPropfindAddressBookListsContacts:183`, `TestPutGetRoundTrip:198`,
`TestDeleteAddressObject:234`).

**Acceptance Scenarios**:

1. **Given** a client with valid credentials, **When** it PROPFINDs `/dav/` at Depth 0,
   **Then** the response names the principal `/dav/{email}/`
   (`internal/carddav/backend.go:107-113`; test `carddav_e2e_test.go:173`).
2. **Given** the principal, **When** the client PROPFINDs the home set
   `/dav/{email}/addressbooks/` at Depth 1, **Then** the one address book
   `/dav/{email}/addressbooks/contacts/` is listed
   (`backend.go:115-148`; test `carddav_e2e_test.go:183`).
3. **Given** a stored contact, **When** the client PROPFINDs the address book at Depth 1,
   **Then** the response contains `/dav/{email}/addressbooks/contacts/{uid}.vcf`
   (`backend.go:211-242`; test `carddav_e2e_test.go:195`).
4. **Given** a card PUT at the object URL, **When** it is GET back, **Then** the UID, names
   and typed values survive the round trip (test `carddav_e2e_test.go:210`), and the contact
   exists in the `contacts` table with parsed `first_name`/`last_name`, so the REST API and
   CardDAV see the same record (`backend.go:292-322`; test `carddav_e2e_test.go:227`).
5. **Given** a stored contact, **When** the client DELETEs the object URL, **Then** the
   server answers 204 and the contact is gone from the database
   (`backend.go:371-399`; test `carddav_e2e_test.go:246`).
6. **Given** an unauthenticated request, **When** any DAV method is issued, **Then** the
   server answers 401 with `WWW-Authenticate: Basic realm="ContactsHQ CardDAV"`
   (`internal/carddav/server.go:72-77`; test `carddav_e2e_test.go:137`). *(The verification
   itself is spec 001's subject; only the challenge shape is claimed here.)*

---

### User Story 2 - Two devices editing the same contact do not silently overwrite each other (Priority: P1)

A contact is open on a phone and on a laptop. Both save. The second save is refused instead
of quietly discarding the first device's edit.

**Why this priority**: This is data loss, and it is silent. It is the failure a user cannot
detect, cannot report precisely, and cannot recover from without a backup. It ranks with
Story 1 because a sync that loses edits is worse than no sync.

**Independent Test**: PUT a card, read its ETag, PUT a different version (moving the ETag on),
then PUT with the stale ETag in `If-Match` and observe 412 plus the surviving other edit
(`carddav_e2e_test.go:337-362`).

**Acceptance Scenarios**:

1. **Given** a contact whose ETag has moved on, **When** a client PUTs with the old value in
   `If-Match`, **Then** the server answers **412 Precondition Failed** and the stored card is
   the other device's version (`internal/carddav/backend.go:351-368`; test
   `carddav_e2e_test.go:337`).
2. **Given** a contact at its current ETag, **When** a client PUTs with that value in
   `If-Match`, **Then** the write succeeds (test `carddav_e2e_test.go:364`).
3. **Given** a contact that already exists, **When** a client PUTs with `If-None-Match: *`
   (create-only), **Then** the server answers 412 and the existing card is untouched
   (`backend.go:343-349`; test `carddav_e2e_test.go:386`).
4. **Given** no such contact, **When** a client PUTs with `If-None-Match: *`, **Then** the
   contact is created (test `carddav_e2e_test.go:404`).
5. **Given** no such contact, **When** a client PUTs with any `If-Match` value, **Then** the
   server answers 412 — the precondition cannot be satisfied (`backend.go:356-359`; test
   `carddav_e2e_test.go:415`).
6. **Given** any PUT or GET of a contact, **When** the client reads the `ETag` header,
   **Then** it is quoted exactly once, because the backend hands go-webdav the bare value
   (`backend.go:324-331`; test `carddav_e2e_test.go:427`).

---

### User Story 3 - A polling client asks "did anything change?" instead of re-reading everything (Priority: P1)

A phone polls every few minutes. When nothing has changed it transfers one small answer
rather than an ETag listing of the whole address book.

**Why this priority**: go-webdav implements neither extension and offers no hook to add them
(`internal/carddav/sync_collection.go:13-19`). Without them every poll costs a listing of the
entire collection — measured at 48 KiB per poll for 200 contacts, growing linearly
(`sync_collection.go:16-18`). On a phone that is battery and data, forever.

**Independent Test**: Read the CTag twice with no writes in between (stable), then create,
update and delete a contact and observe it advance each time
(`carddav_e2e_test.go:567-584`).

**Acceptance Scenarios**:

1. **Given** an unchanged collection, **When** the CTag is read twice, **Then** the value is
   identical; **When** a contact is created, updated, or deleted, **Then** it changes
   (`internal/repository/change_journal.go:17-26`, `76-84`; test `carddav_e2e_test.go:567`).
2. **Given** a Depth-0 PROPFIND on the address book asking for
   `{http://calendarserver.org/ns/}getctag` or `{DAV:}sync-token`, **When** it is served,
   **Then** the answer is a 207 multistatus carrying those properties
   (`sync_collection.go:112-177`).
3. **Given** a client with no token, **When** it issues a `sync-collection` REPORT with an
   empty `sync-token`, **Then** it receives every contact, no deletions, and a token
   (`sync_collection.go:186-251`, `change_journal.go:112-114`; test
   `carddav_e2e_test.go:609`).
4. **Given** a client holding a current token, **When** nothing has changed, **Then** the
   report names no contact and returns the same token (test `carddav_e2e_test.go:624`).
5. **Given** a client holding an older token, **When** contacts have been added and deleted,
   **Then** the added contact is reported with its ETag and the deleted contact's URL is
   named with `HTTP/1.1 404 Not Found`, while untouched contacts are not repeated
   (`sync_collection.go:232-245`; test `carddav_e2e_test.go:635`).
6. **Given** a REPORT that asks for `{urn:ietf:params:xml:ns:carddav}address-data`, **When**
   it is served, **Then** each changed response carries the full card
   (`sync_collection.go:226`, `236-238`; test `carddav_e2e_test.go:655`).
7. **Given** a token this server never issued — garbage, another server's URL, or a sequence
   ahead of the collection's own — **When** the REPORT is served, **Then** the answer is
   **403** carrying `<D:valid-sync-token/>`, which tells the client to resynchronise from
   scratch (`sync_collection.go:91-97`, `196-220`; test `carddav_e2e_test.go:669`).
8. **Given** a PROPFIND or REPORT that is *not* one of the two extensions, **When** it
   arrives, **Then** it reaches go-webdav untouched — `addressbook-query` and
   `addressbook-multiget` keep working (`server.go:118-140`; tests
   `carddav_e2e_test.go:598`, `:687`, `:264`).

---

### User Story 4 - Set up an iPhone without typing a server path (Priority: P2)

A signed-in user opens **Settings → Connect Devices**, taps **Download Profile**, and installs
a `.mobileconfig` that fills in host, port, TLS and principal URL. Only the password is typed.

**Why this priority**: Onboarding friction, not correctness. Manual setup is documented and
works; the profile removes the step users get wrong (the URL). Below the three correctness
stories.

**Independent Test**: Call `GET /api/v1/setup/ios-profile` with a bearer token and check the
payload type, principal URL and the download headers
(`internal/handler/profile_handler.go:17-98`). No automated test does this — see
Known Divergences.

**Acceptance Scenarios**:

1. **Given** an authenticated user, **When** they request the iOS profile, **Then** the
   response is an Apple configuration profile
   (`Content-Type: application/x-apple-aspen-config`) served as an attachment named
   `ContactsHQ.mobileconfig` (`profile_handler.go:96-98`).
2. **Given** the request arrived over HTTPS (directly or per `X-Forwarded-Proto`), **When**
   the profile is generated, **Then** it sets `CardDAVUseSSL` true and port 443; over plain
   HTTP it sets false and port 80 (`profile_handler.go:26-42`).
3. **Given** a reverse proxy setting `X-Forwarded-Host`, **When** the profile is generated,
   **Then** the hostname in the profile is the external one, not the internal
   (`profile_handler.go:20-23`).
4. **Given** any email or host, **When** it is written into the plist, **Then** `& < > " '`
   are escaped so the profile cannot be broken or injected
   (`profile_handler.go:44-46`, `101-110`).
5. **Given** the Settings page, **When** the user opens it, **Then** the exact CardDAV
   collection URL is shown with a copy button, manual instructions for iOS/macOS/Thunderbird
   are listed, and every password mention links to App Passwords
   (`web/src/views/settings/SetupView.vue:8-115`).

---

### User Story 5 - Discover the server from the domain name alone (Priority: P2)

A client is given `contacts.example.com` and finds the CardDAV service itself.

**Why this priority**: RFC 6764 discovery is what makes "Server: your domain" in the setup
instructions correct. Without it users must paste a full collection path, which is Story 4's
failure mode.

**Independent Test**: `curl -I https://host/.well-known/carddav` → 301 to `/dav/`
(`docs/reverse-proxy.md:100-107`). Manual only — see Known Divergences.

**Acceptance Scenarios**:

1. **Given** any client, **When** it requests `/.well-known/carddav`, **Then** the server
   answers **301** to the configured DAV prefix with a trailing slash, cacheable for a day
   (`cmd/server/main.go:278-283`).
2. **Given** an anonymous visitor, **When** they open `/setup`, **Then** a public guide
   covering prerequisites (HTTPS), iPhone/iPad, macOS, Thunderbird and troubleshooting is
   served without login (`internal/web/handler.go:24-33`,
   `internal/web/templates/setup-guide.html`).

---

### Edge Cases

These are boundary conditions the code handles deliberately. Behaviour that is *wrong*, stale
or unenforced is recorded under Known Divergences instead, not here.

- **What happens when a stored vCard cannot be decoded?** GET does not fail. The server
  synthesises a minimal card from the UID, version and name so the client still sees the
  resource (`backend.go:181-209`, `411-428`).
- **What happens when a restored backup moves the sequence backwards?** Sync tokens are a
  per-address-book counter, so a client can arrive holding a token from a database that no
  longer exists. This is handled rather than prevented: a token ahead of the collection's own
  sequence is rejected with `valid-sync-token` (`sync_collection.go:217-220`) precisely
  because it "belongs to another database", and the client resynchronises.
- **What happens on a client's very first `sync-collection`?** An empty token returns the whole
  collection and **no** deletions — a client that has never seen the collection has nothing to
  delete (`change_journal.go:88-89`, `112-114`; test `carddav_e2e_test.go:609`).
- **What happens when `If-Match` names a contact that does not exist?** 412: the precondition
  cannot be satisfied, and inventing the contact would defeat the point of asking
  (`backend.go:356-359`).
- **What happens when the client omits the `.vcf` suffix?** `extractUIDFromPath` trims a
  trailing `.vcf` only if present (`backend.go:401-409`), so `/…/contacts/{uid}` addresses the
  same object. No client is known to rely on that; it is an accident of the implementation,
  not a promise.
- **What happens to a request that is neither of the two extensions?** It is delegated to
  go-webdav untouched, so `addressbook-query` and `addressbook-multiget` keep working
  (`server.go:118-140`; tests `carddav_e2e_test.go:598`, `:687`, `:264`).

## Requirements *(mandatory)*

### Functional Requirements

**Path layout and resource routing**

- **FR-001**: The server MUST expose the layout `/{prefix}/{email}/` (principal),
  `/{prefix}/{email}/addressbooks/` (home set), `/{prefix}/{email}/addressbooks/contacts/`
  (address book), `/{prefix}/{email}/addressbooks/contacts/{uid}.vcf` (address object).
  (`internal/carddav/backend.go:55-79`)
- **FR-002**: Those four depths (1, 2, 3, 4 segments below the prefix) MUST be preserved,
  because go-webdav classifies a resource purely by segment count: a wrong depth makes a
  collection PROPFIND list nothing and dispatches a contact DELETE to `DeleteAddressBook`.
  (`backend.go:45-54`; test `carddav_e2e_test.go:159-171` asserts the four depths)
- **FR-003**: Paths MUST be built through the exported helpers `PrincipalPath`,
  `HomeSetPath`, `AddressBookPath`, `AddressObjectPath` rather than by string concatenation
  at call sites. (`backend.go:60-79`; used by `sync_collection.go:133`, `233`, `243` and
  `server.go:149`)
- **FR-004**: The address book collection MUST be identified by comparing the request path to
  `AddressBookPath` for the *authenticated* user, tolerating the missing trailing slash.
  (`server.go:145-151`)

**Backend mapping onto contacts**

- **FR-005**: Every backend operation MUST resolve data from the authenticated principal in
  the request context, never from the email segment in the URL; the segment is decorative.
  (`backend.go:107-242`, `249-261`, `371-383` — all read `GetUserID`/`GetUserEmail`)
- **FR-006**: Each user MUST have exactly one address book, named from the stored record;
  creating an additional one and deleting one MUST be refused.
  (`backend.go:123-148`, `173-179`)
- **FR-007**: `GET` on an address object MUST return the stored vCard text, decoded; if it
  cannot be decoded the server MUST synthesise a minimal card from UID, version and name
  rather than fail. (`backend.go:181-209`, `411-428`)
- **FR-008**: `PUT` MUST store the card verbatim, parse it through the shared vCard package,
  and write the derived scalar columns and child records so the REST API sees the same
  contact. (`backend.go:274-322`; `internal/vcard`)
- **FR-009**: `PUT` MUST derive the ETag as the first 8 bytes of the card's SHA-256, hex
  encoded, and MUST return it **unquoted** — go-webdav adds the quotes.
  (`backend.go:287-288`, `324-331`; test `carddav_e2e_test.go:427`)
- **FR-010**: `DELETE` on an address object MUST remove the contact identified by the UID in
  the path within the authenticated user's address book. (`backend.go:371-399`)
- **FR-011**: The address book MUST advertise `CARDDAV:max-resource-size` when a limit is
  configured, so a client learns the limit before uploading; zero omits the property.
  (`backend.go:86-105`, `143-147`; wired at `cmd/server/main.go:286-287`; config validated
  positive at `internal/config/config.go:355-360`)
- **FR-036**: A `PUT` through `/dav` whose **stored** card exceeds
  `carddav.max_resource_bytes` MUST be refused with **413 Request Entity Too Large**, naming
  both the card's size and the limit; the contact MUST NOT be written.
  (`backend.go:275-286`) The number compared is the length of the card **after** go-webdav has
  decoded the body and `internal/vcard` has re-encoded it (`backend.go:274`), not the request's
  `Content-Length`. A `GET` is not bounded: a resource larger than the advertised maximum may
  still be read. Three consequences the code makes true and this spec will not overstate:
  - The limit binds the `/dav` write path **only**. `POST /api/v1/contacts` (2 MiB via
    `apiBodyLimit`, `cmd/server/main.go:397-407`, `:273`), `POST /import/*`
    (`server.max_import_bytes`) and inbound sync all store cards without consulting it. The
    advertised value is therefore honest about what a *device* may upload, and says nothing
    about what the database may hold.
  - It is **policy, not protection** (constitution Principle IV). `/dav` is mounted through
    `adaptor.HTTPHandler` under the global 32 MiB `BodyLimit` (`cmd/server/main.go:211`,
    `:299`), and go-webdav decodes the whole body into a `vcard.Card` before the backend runs
    (`go-webdav@v0.7.0/carddav/server.go:671-676`, where its own max-resource-size check is
    still a `TODO`). The allocation is already spent when the check fires. What the check buys
    is that the card does not reach the database and the client is told why.
  - Refusing is **413**, not RFC 6352's 403 with a `CARDDAV:max-resource-size` precondition
    element: go-webdav v0.7.0 has no renderer for that element, so a 403 would be
    indistinguishable from an authorization failure. A `*webdav.HTTPError` returned from the
    backend keeps its status (`go-webdav@v0.7.0/internal/server.go:14-28`), so 413 arrives
    as 413.

**Preconditions (lost-update protection)**

- **FR-012**: `If-None-Match: *` MUST mean create-only: 412 if the contact already exists.
  (`backend.go:342-349`)
- **FR-013**: `If-Match` MUST mean update-only against the version the client last saw: 412 if
  the contact is absent, 412 if the ETag does not match, 400 if the header cannot be parsed.
  (`backend.go:351-368`)
- **FR-014**: When neither header is present the write MUST proceed unconditionally — most
  clients do not send them. (`backend.go:337-340`, `351-353`)

**CTag (CalendarServer extension)**

- **FR-015**: A Depth-0 PROPFIND on the address book requesting `getctag` MUST answer with the
  collection's current change sequence. (`sync_collection.go:112-141`, `254-260`)
- **FR-016**: The same handler MUST answer `{DAV:}sync-token`, `{DAV:}resourcetype` and
  `{DAV:}supported-report-set` when asked, and MUST advertise `sync-collection`,
  `addressbook-query` and `addressbook-multiget` in the report set — that is how a client
  discovers the extension exists. (`sync_collection.go:142-151`; test
  `carddav_e2e_test.go:586`)
- **FR-017**: Properties the server does not have MUST come back in a `404 Not Found`
  propstat rather than being silently dropped. (`sync_collection.go:153-172`)
- **FR-018**: A PROPFIND that asks for none of the extension properties MUST be delegated to
  go-webdav unchanged. (`sync_collection.go:121-125`; test `carddav_e2e_test.go:598`)

**RFC 6578 collection synchronisation**

- **FR-019**: Sync tokens MUST be minted as `urn:contactshq:sync:<seq>` and parsed back to
  that sequence; an empty token means "I have nothing".
  (`sync_collection.go:24-47`)
- **FR-020**: A `sync-collection` REPORT MUST return, for the requested token: every contact
  changed since it with its ETag, every UID deleted since it as a `404 Not Found` response,
  and the collection's current token to come back with.
  (`sync_collection.go:226-248`; `internal/repository/change_journal.go:86-128`)
- **FR-021**: With an empty token the report MUST return the whole collection and **no**
  deletions — a client that has never seen the collection has nothing to delete.
  (`change_journal.go:88-89`, `112-114`; test `carddav_e2e_test.go:609`)
- **FR-022**: `address-data` MUST be included per changed resource only when the client asked
  for it, XML-escaped. (`sync_collection.go:226`, `236-238`)
- **FR-023**: A token that this server did not mint, or that is ahead of the collection's own
  sequence, MUST be refused with **403** and `<D:valid-sync-token/>`, so the client
  resynchronises instead of silently missing deletions.
  (`sync_collection.go:91-97`, `196-200`, `214-220`; test `carddav_e2e_test.go:669`)
- **FR-024**: A REPORT whose body is not a `sync-collection` MUST be delegated to go-webdav.
  (`sync_collection.go:187-193`; test `carddav_e2e_test.go:687`)
- **FR-025**: The extension sniffing MUST read the body under a bounded limit and restore it
  for the delegated handler. (`server.go:126-131`, `143`)

**The change journal this depends on** *(owned by spec 002; stated here as the contract this
spec reads)*

- **FR-026**: Every contact write MUST bump the address book's `change_seq` inside the same
  transaction, so a client can never read a CTag no contact carries yet.
  (`internal/repository/change_journal.go:12-26`; `internal/repository/bun_contact.go:58-70`,
  `115`, `134`, `272`)
- **FR-027**: Every deletion path — single, bulk, delete-all, and merge — MUST record a
  tombstone at the new sequence, because a deleted resource cannot be named from the contacts
  table. (`change_journal.go:28-48`; `bun_contact.go:100-121`, `123-141`, `255-276`;
  merge at `internal/repository/bun_contact_relations.go:78-93`;
  `internal/domain/contact_tombstone.go:9-12`)
- **FR-028**: Recreating a UID MUST drop its tombstone, or a synchronising client would be
  told to delete a contact that is present. (`change_journal.go:50-63`;
  `bun_contact.go:66`; `internal/repository/bun_contact_relations.go:112-113`)

**Discovery and onboarding**

- **FR-029**: `GET /.well-known/carddav` MUST answer 301 to the configured DAV prefix with a
  trailing slash and a one-day `Cache-Control`. (`cmd/server/main.go:278-283`)
- **FR-030**: `GET /api/v1/setup/ios-profile` MUST require authentication and return a
  complete `com.apple.carddav.account` payload — host, port, TLS flag, principal URL,
  username — with stable payload identifiers per account and fresh UUIDs per download.
  (`internal/handler/handler.go:177-179`; `internal/handler/profile_handler.go:17-98`)
- **FR-031**: The profile MUST prefer `X-Forwarded-Host` / `X-Forwarded-Proto` over the direct
  request's host and scheme, since the server is expected to sit behind a TLS-terminating
  proxy. (`profile_handler.go:20-34`; `docs/reverse-proxy.md:1-62`)
- **FR-032**: Values interpolated into the plist MUST be XML-escaped.
  (`profile_handler.go:44-46`, `101-110`)
- **FR-033**: A public, unauthenticated setup guide MUST be served at `/setup`, reachable
  without a session, covering the HTTPS prerequisite, iPhone/iPad, macOS, Thunderbird and
  troubleshooting. It is registered before the SPA mount and outside every authenticated
  group. (`internal/web/handler.go:24-33`; `internal/web/templates/setup-guide.html`)
- **FR-034**: The authenticated Settings page MUST show the exact CardDAV collection URL with
  a copy action, offer the profile download, and steer users to App Passwords rather than
  their account password. (`web/src/views/settings/SetupView.vue:8-115`, `137-170`)
- **FR-035**: The DAV prefix and the advertised per-card limit MUST be configurable, and a
  non-positive limit MUST refuse startup.
  (`internal/config/config.go:49-50`, `128-135`, `188-189`, `355-360`)

*Not a requirement of this spec, but the mount cannot work without it:* Fiber only routes
methods listed in `RequestMethods`, so the WebDAV verbs are registered at the composition root
(`cmd/server/main.go:41-49`, `223`). That wiring belongs to spec 008.

### Key Entities

- **Address book** — one per user. Carries `name`, `description` and `change_seq`, the
  per-collection counter that *is* the CTag. (`internal/domain` address book model;
  `migrations/021_change_journal.up.sql:11`)
- **Contact** — a stored vCard plus derived columns. CardDAV identity is `uid`
  (`{uid}.vcf`); version identity is `etag`; sync position is `change_seq`.
  (`migrations/021_change_journal.up.sql:12`)
- **Contact tombstone** — `(address_book_id, uid, change_seq, deleted_at)`. Exists solely so
  RFC 6578 can name deleted resources. Dropped when the UID is recreated.
  (`internal/domain/contact_tombstone.go`; `migrations/021_change_journal.up.sql:14`)
- **CTag** — the address book's current `change_seq`, rendered as a bare integer.
  (`sync_collection.go:137`)
- **Sync token** — `urn:contactshq:sync:<seq>`; the `change_seq` a client last saw.
  (`sync_collection.go:24-29`)
- **iOS configuration profile** — a generated, unsigned Apple plist describing one CardDAV
  account. Not persisted; built per request. (`internal/handler/profile_handler.go`)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A poll of an unchanged address book transfers a **constant-size** answer,
  independent of contact count. The baseline it replaces is 48 KiB per poll for 200 contacts,
  growing linearly.
- **SC-002**: After a change, a synchronising client transfers only the changed and deleted
  resources: an untouched contact is never repeated, and a client that already has everything
  receives zero resource entries.
- **SC-003**: Of two devices saving the same contact concurrently with conditional requests,
  **at most one** write lands; the loser is told so with 412 and the winner's content survives
  verbatim.
- **SC-004**: 100% of deletions are reported by URL to every client whose token predates them,
  with **no expiry window** — tombstones are never pruned, so every token the server ever
  issued remains answerable.
- **SC-005**: A client presenting an unusable token (unparseable, foreign, or ahead of the
  collection) is told to resynchronise in 100% of cases rather than receiving an empty delta.
- **SC-006**: A device can be pointed at the bare domain name and reach the service without
  the user knowing any path: `/.well-known/carddav` answers 301 to the collection prefix.
- **SC-007**: One-tap iOS setup requires the user to type **one** value — the password. Host,
  port, TLS and principal URL come from the profile.
- **SC-008**: The CardDAV surface is covered by 30 end-to-end tests exercising the real HTTP
  handler against a migrated database, spanning auth, path depths, CRUD, preconditions, the
  per-card size limit, CTag, sync-collection and delegation.

## Assumptions

- **One address book per user, named `contacts` in the URL regardless of its display name.**
  The path segment is a constant (`backend.go:57`) while the display name comes from the
  record. Multiple address books were never a goal; creation is refused outright
  (`backend.go:173-175`).
- **`GetOrCreateByUserID`'s race handling is best-effort, not a lock.** It retries the read
  after a failed insert (`bun_addressbook.go:59-65`). Adequate for the single-instance
  deployment this project supports; not a distributed guarantee.
- **The change journal is trusted to be transactional.** This spec's CTag and token semantics
  are only correct because `nextChangeSeq` runs inside the caller's transaction
  (`change_journal.go:12-16`). Any write path added outside a transaction would break sync
  silently, not loudly.
- **HTTPS is assumed to be terminated by a reverse proxy.** iOS and macOS clients require it;
  the server itself speaks plain HTTP and the setup guide says so
  (`setup-guide.html:41-47`, `docs/reverse-proxy.md:1-15`).
- **Scope boundaries — what this spec deliberately does not cover**, because another spec owns
  it:
  - **Authentication of the DAV request** — Basic auth parsing, app-password fallback, the
    five-minute verdict cache and the per-IP throttle. Spec 001.
  - **The change journal itself** — how `change_seq` and tombstones are written. Spec 002;
    this spec only reads them, and restates the contract as FR-026..FR-028.
  - **ContactsHQ as a CardDAV *client*** against someone else's server, including the sync
    engine's own RFC 6578 usage and endpoint validation. Spec 006 and `docs/sync.md`.
  - **WebDAV verb registration and the `/dav` mount** — composition-root wiring. Spec 008.
  - **Contact modelling, vCard encoding and multi-value fields** — owned by their own specs;
    this spec treats a card as opaque text plus whatever `internal/vcard` derives from it.

## Status

Shipped. Every requirement above is serving at `23a167c` (tag `v0.4.0`). The CardDAV surface
and the onboarding surfaces predate that tag; the CTag and RFC 6578 `sync-collection`
extensions shipped in v0.2.0 (`CHANGELOG.md:209-212`). Nothing in this spec is aspirational —
where shipped behaviour is wrong or unenforced it is recorded under Known Divergences, not
softened here.

## Code Paths

Owned by this spec:

- `internal/carddav/server.go`
- `internal/carddav/backend.go`
- `internal/carddav/sync_collection.go`
- `internal/carddav/carddav_e2e_test.go`
- `internal/handler/profile_handler.go`
- `internal/web/templates/setup-guide.html`
- `web/src/views/settings/SetupView.vue`
- `docs/reverse-proxy.md`

## References

Paths this spec reads, cites or depends on but does **not** own:

- `internal/carddav/authcache.go` — the verdict cache behind DAV authentication. Spec 001.
- `internal/carddav/throttle.go` — the per-IP failure throttle. Spec 001.
- `internal/repository/change_journal.go` — `change_seq`, tombstones and `ChangesSince`, the
  contract restated as FR-026..FR-028. Spec 002.
- `internal/domain/contact_tombstone.go` — the tombstone record. Spec 002.
- `migrations/021_change_journal.up.sql`, `migrations/021_change_journal.down.sql` — the
  `change_seq` column and `contact_tombstones` table. Spec 002.
- `internal/vcard/parser.go` — decodes a PUT card into the scalar and child columns (FR-008).
  Spec 003.
- `internal/vcard/encoder.go` — the single writer of vCard text; this spec stores a PUT card
  verbatim and never encodes one itself. Spec 003.
- `cmd/server/main.go` — the `/dav` mount, `RequestMethods`, the `.well-known` route
  registration and `WithMaxResourceSize` wiring. Spec 008.
- `internal/handler/handler.go` — registration of `GET /api/v1/setup/ios-profile` under the
  authenticated group. Spec 008.
- `internal/web/handler.go` — registration of the public `/setup` route. Spec 008.
- `internal/config/config.go` — `carddav.path_prefix` and `carddav.max_resource_bytes`,
  including the positive-limit validation behind FR-035. Spec 008.

External dependencies:

- **go-webdav v0.7.0** supplies the DAV/CardDAV request handling this spec wraps. Its resource
  classification by segment depth is the reason for FR-002, and its 500-by-default error
  mapping is the reason for the error-shape divergence below
  (`go-webdav@v0.7.0/internal/server.go:14-28`).
- **go-vcard** decodes cards on read; `internal/vcard` is the only writer (spec 003 owns that
  rule).

## Enforced By

CardDAV protocol surface — all in `internal/carddav/carddav_e2e_test.go`, which runs the real
HTTP handler against a migrated in-memory database:

- `TestPathHierarchyDepths` — FR-001, FR-002
- `TestPropfindRootExposesPrincipal`, `TestPropfindHomeSetListsAddressBook`,
  `TestPropfindAddressBookListsContacts` — FR-005, FR-006
- `TestPutGetRoundTrip`, `TestPutPersistsContact`, `TestPutUpdatesExistingContact` — FR-007,
  FR-008
- `TestETagHeaderIsSingleQuoted` — FR-009
- `TestDeleteAddressObject` — FR-010
- `TestPutRejectsACardOverTheAdvertisedLimit` — FR-036 (413, and the row is not written),
  `TestPutAcceptsACardAtTheAdvertisedLimit` (a card exactly at the limit is not over it, so
  the boundary cannot silently go off by one),
  `TestPutIsUnboundedWhenNoLimitIsSet` (pins `NewBackend`-without-`WithMaxResourceSize`, which
  is a constructor contract and **not** a reachable configuration — `config.go:355-360` refuses
  a non-positive `carddav.max_resource_bytes`)
- `TestPutWithIfNoneMatchRejectsExistingContact`, `TestPutWithIfNoneMatchCreatesNewContact` —
  FR-012
- `TestPutWithStaleIfMatchIsRejected`, `TestPutWithCurrentIfMatchSucceeds`,
  `TestPutWithIfMatchOnMissingContactIsRejected` — FR-013, FR-014, SC-003
- `TestCTagChangesOnWriteAndIsStableOtherwise` — FR-015
- `TestPropfindAdvertisesSyncCollectionSupport` — FR-016
- `TestPropfindWithoutCTagIsDelegated`, `TestReportOtherThanSyncCollectionIsDelegated`,
  `TestReportAddressbookQuery` — FR-018, FR-024
- `TestSyncCollection_EmptyTokenReturnsWholeCollection` — FR-021, SC-002
- `TestSyncCollection_UnchangedCollectionReturnsEmptyDelta` — FR-019 (the token round-trips),
  FR-020, SC-001, SC-002
- `TestSyncCollection_ReportsCreationsAndDeletions` — FR-020, SC-002, SC-004
- `TestSyncCollection_CarriesCardsWhenAddressDataRequested` — FR-022
- `TestSyncCollection_RejectsUnknownTokens` — FR-023, SC-005 (three cases: garbage, foreign
  URL, a sequence ahead of the collection)
- `TestAuthRequired`, `TestWrongPasswordRejected`, `TestAppPasswordAuthenticates` — the 401
  challenge shape claimed in Story 1 scenario 6; the verification itself is spec 001's

The change journal this spec reads (owned by spec 002, listed because FR-026..FR-028 are
meaningless without them) — `internal/repository/change_journal_test.go`,
`internal/repository/merge_into_test.go`, `internal/repository/migrate_postgres_test.go`:

- `TestChangeSeq_AdvancesOnEveryWrite` — FR-026
- `TestChangesSince_ZeroTokenReturnsWholeCollection` — FR-021
- `TestChangesSince_ReportsOnlyWhatHappenedAfterTheToken` — FR-020
- `TestChangesSince_ReportsDeletions`, `TestChangesSince_ReportsBulkDeletions`,
  `TestChangesSince_ReportsDeleteAll` — FR-027
- `TestMergeInto_JournalsBothTheUpdateAndTheDeletion`,
  `TestMergeInto_UsesOneSequenceForBothChanges` — FR-027 for the merge path
- `TestChangesSince_RecreatedUIDHasNoTombstone` — FR-028
- `TestDelete_UnknownContactIsANoop` — a deletion of nothing must not mint a sequence
- `TestPostgres_ChangeJournal` — the same guarantees on PostgreSQL

Configuration:

- `TestCardDAVConfigValidate` (`internal/config/config_test.go`) — FR-035's second half: a
  non-positive `carddav.max_resource_bytes` refuses startup. The `path_prefix` half has no
  test.

Review-only and unenforced requirements are named in Known Divergences below, not padded out
with enforcers that do not exist.

## Known Divergences

**A PUT stores the card under the UID it is addressed by.** The contact is keyed on the UID in
the request path, but the client's bytes were stored unchanged — so a card claiming a different
UID inside was what export and sync pushed onward. `InjectUID` now runs before the size check
and the ETag hash, so the stored bytes, the key and the hash all agree.


**Unenforced limits**

- **`carddav.max_resource_bytes` binds one write path, not the record.** FR-036 refuses an
  oversized `PUT` through `/dav` (`internal/carddav/backend.go:275-286`), and that is the whole
  of it. A card larger than the limit can still be created through `POST /api/v1/contacts`, an
  import, or an inbound sync run, and once stored it is served by `GET` and by
  `sync-collection` without complaint. The named failure mode: **a contact stored above the
  limit by any of those paths syncs out to a device fine and then becomes permanently
  unwritable from that device** — the device's next `PUT` of it gets 413, and most CardDAV
  clients retry indefinitely rather than surface the refusal. The remedy is the operator's:
  raise `CHQ_CARDDAV_MAX_RESOURCE_BYTES`. See the upgrade note in `CHANGELOG.md`, which carries
  the pre-flight query against `contacts.vcard_data`.
- **A 413 from `/dav` is diagnosable only from the access log.** `internal/carddav` has no
  logger, so the refusal surfaces as a single `middleware.RequestLogger` line
  (`internal/handler/middleware/logger.go:47-55`) carrying the status and the request path —
  which does contain the UID — and nothing else. The card's size and the limit reach the
  *client* in the response body and are never recorded server-side.
- **`/dav` is exempt from the fixed-error-text convention, and this is where that shows.**
  Constitution Principle III and `newErrorHandler` govern the Fiber error path; the DAV mount
  does not pass through it. go-webdav's `ServeError` ends in `http.Error(w, err.Error(), code)`
  (`go-webdav@v0.7.0/internal/server.go:14-28`), so FR-036's message — two byte counts — is
  echoed to the client verbatim. Byte counts are not secret, so this is acceptable here; it is
  recorded because a reader would otherwise assume the API's policy applies to this mount. The
  same mechanism is what leaks internal text in the divergence below.
- **PROPFIND/REPORT bodies on the address book path are truncated at 1 MiB.**
  `serveSyncExtensions` buffers at most `maxSyncRequestBody` and hands the *buffered* copy on
  to go-webdav (`server.go:126-131`, `143`). A larger body — a very large
  `addressbook-multiget`, say — is silently cut before delegation. FR-025 has no test.

**Behaviour that contradicts a stated project rule**

- **Backend errors surface as 500 with the internal message.** Every backend failure returns a
  plain `fmt.Errorf` (`backend.go:161`, `:197`, `:206`, `:396`), and go-webdav maps a non-
  `HTTPError` to `500` with `err.Error()` in the body
  (`go-webdav@v0.7.0/internal/server.go:14-28`). So GETting a missing contact yields
  `500 contact not found`, not 404 — and internal text reaches the client, which is the
  opposite of constitution Principle III and of the API's fixed `"internal server error"`
  policy (`newErrorHandler`, `cmd/server/main.go`). Only the precondition checks build real
  `HTTPError`s (`backend.go:345`, `:357`, `:362`, `:365`).
- **`internal/carddav/server.go` is claimed here in whole while it still holds two domains.**
  At `23a167c` the file carries the routing surface (`ServeHTTP:71`, `serveSyncExtensions:118`,
  `isAddressBookPath:145`) *and* the credential-verification surface (`authenticate:160`,
  `verifyCredentials:190`, `verifyAppPassword:221`, `VerifyArgon2id:260`) in 328 lines. Spec
  000 FR-018 requires the split; until it lands, this path is owned here by assignment rather
  than by cleanliness, and constitution Principle VII holds only because this spec claims it
  and spec 001 does not.

**Configuration that only half works**

- **The iOS profile and the Settings page hard-code `/dav`.** `carddav.path_prefix` is
  configurable (`internal/config/config.go:129-130`, default `/dav` at `:188`) and the
  `.well-known` redirect honours it (`cmd/server/main.go:279-282`), but the profile writes
  `/dav/{email}/` literally (`internal/handler/profile_handler.go:62`) and the Vue view builds
  `/dav/{email}/addressbooks/contacts/` literally
  (`web/src/views/settings/SetupView.vue:137-140`). Changing the prefix breaks both onboarding
  surfaces while the server itself keeps working.

**Correctness rough edges**

- **The UID in the path wins over the UID in the card.** `PutAddressObject` takes the UID from
  the URL and only falls back to the card's `UID` property (`backend.go:263-272`), yet stores
  the card verbatim (`backend.go:274`, `:315`). A client that PUTs to `a.vcf` a card saying
  `UID:b` gets a contact keyed `a` whose stored vCard says `b`; nothing reconciles them.
- **`addressbook-query` is not filtered server-side.** `QueryAddressObjects` returns the whole
  collection and lets go-webdav filter it (`backend.go:244-247`), so a narrow query on a large
  address book still loads every contact — the cost SC-001 removes from polling remains here.
- **Two different lookups for the same address book.** The go-webdav backend uses
  `GetByUserID` and reports "no address books" when the row is missing
  (`backend.go:129-135`), while the sync extensions use `GetOrCreateByUserID` and create it
  (`sync_collection.go:202`, `255`; `internal/repository/bun_addressbook.go:45-67`). A user
  whose address book row does not exist sees an empty home set from a plain PROPFIND but a
  working CTag.
- **A PROPFIND asking for the CTag at any Depth other than 0 is delegated.**
  `handlePropfindCTag` bails out unless `Depth` is absent or `0` (`sync_collection.go:113-115`),
  and go-webdav has no CTag, so such a client gets no CTag at all.
- **`sync-level` is parsed and ignored.** The field is decoded (`sync_collection.go:63`) but
  never inspected; the collection is flat, so every level behaves as level 1.
- **Tombstones are never pruned.** That is what makes every token this server ever issued
  answerable (`sync_collection.go:222-224`) and what SC-004 is built on, and it means
  `contact_tombstones` grows without bound over the life of an install. Pruning would need a
  horizon and a rule for rejecting tokens older than it; neither exists.
- **Two identical ETag derivations exist.** The CardDAV backend computes
  `sha256(vcard)[:8]` inline (`backend.go:287-288`); the REST path calls
  `service.ContactETag`, which is the same computation (`internal/service/contact.go:347-350`).
  They agree today; nothing enforces that they keep agreeing, and the exported function exists
  specifically because a second hand-rolled copy would drift.

**Stale strings and third-party dependencies on server-rendered pages**

- **The setup guide links to `/docs/reverse-proxy.md`**
  (`internal/web/templates/setup-guide.html:45`), but no route serves `/docs`
  (`internal/web/handler.go:12-45` registers `/`, `/setup` and `/app` only). The link 404s;
  the file exists only in the repository.
- **The public setup guide loads Tailwind from a CDN** (`setup-guide.html:7`), so it renders
  unstyled on an air-gapped install. It is one of **two** server-rendered pages that do this —
  the landing page carries the same `<script src="https://cdn.tailwindcss.com">`
  (`internal/web/templates/landing.html:7`). Every other asset the server ships is embedded.
- **The profile is unsigned**, so iOS marks it "Unverified" during installation. Nothing in
  `profile_handler.go` signs it.

**Requirements with no test at all**

- **The whole onboarding and discovery half of this spec is untested.** FR-029 (`.well-known`
  redirect), FR-030, FR-031 and FR-032 (the iOS profile — `internal/handler` has only
  `health_test.go` and `registration_policy_test.go`), FR-033 (the public `/setup` route) and
  FR-034 (`SetupView.vue` — `web/src` carries no test for it) have no automated enforcer of
  any kind. Stories 4 and 5 are verified by hand, which is why the hard-coded `/dav` prefix and
  the dead `/docs` link above survived. SC-006 and SC-007 rest on the same manual checks.
- **FR-011 is not tested**: no test asserts that `CARDDAV:max-resource-size` appears in a
  PROPFIND response, or that zero omits it. Only the *enforcement* half of the limit (FR-036)
  has enforcers; the *advertisement* the whole feature is named after still has none.
- **FR-017 is not tested**: the e2e suite asserts `404 Not Found` only as the spelling of a
  *deletion* in a sync report (`carddav_e2e_test.go:619`, `:651`), never as the propstat for a
  property the server does not have.
- **FR-003 and FR-004 are review-only.** The path helpers are exercised indirectly by every
  test that builds a URL through them, but nothing fails if a future call site concatenates a
  path by hand.
- **SC-001's baseline is a recorded measurement, not a CI assertion.** The 48 KiB per poll for
  200 contacts is stated in the comment at `internal/carddav/sync_collection.go:16-18`; no
  benchmark reproduces it and no test bounds the response size.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-11 | unreleased | `PutAddressObject` injects the path UID into the stored card before hashing, ending the mismatch between the key and the card's own UID. | — |
| 2026-08-07 | v0.4.0 | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | v0.4.0 | Restructured to the house template; ownership of `internal/carddav/server.go` recorded here in full; admissions moved from Edge Cases and Assumptions into Known Divergences; corrected the claim that the setup guide was the only server-rendered page loading a CDN asset (`landing.html:7` does too). | — |
| 2026-08-07 | unreleased | D1: `carddav.max_resource_bytes` is now enforced on `PUT` through `/dav` with a 413 (FR-036 added, enforced by `TestPutRejectsACardOverTheAdvertisedLimit`, `TestPutAcceptsACardAtTheAdvertisedLimit`, `TestPutIsUnboundedWhenNoLimitIsSet`). The divergence that it was "advertised, never enforced" is replaced by three narrower ones: the limit binds the `/dav` write path only — so a card stored above it via the API, an import or inbound sync becomes permanently unwritable from a device; a refusal is diagnosable only from one access-log line; and `/dav` is exempt from Principle III's fixed error text, which is why the 413 body carries the byte counts. The paired divergence saying `CHANGELOG.md` was wrong to promise a 413 is removed — the changelog is now right, and carries the upgrade note and pre-flight query. FR-011 is unchanged and still untested: only enforcement gained enforcers, not the advertisement. Backend and test line citations throughout renumbered for the inserted code. | — |
