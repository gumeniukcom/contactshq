# Feature Specification: Contact Record & Catalog

Kind: journey
Status: shipped
Constitution: v1.0.0

This specification is retrospective: it was reconstructed from the implementation at commit
`23a167c` (`v0.4.0` plus three follow-up commits), after the behaviour shipped. Every requirement
carries the file it was read from. Where the code does something a reader would not expect, it is
written down as found — under **Edge Cases** or **Known Divergences** — rather than smoothed into
a requirement that was met.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Keep a contact and find it again (Priority: P1)

A signed-in person records a person or company — names, several email addresses and phone
numbers, an employer, a job title, postal addresses, websites, messaging handles, tags, a
birthday, a free-text note — and later finds that record by typing any fragment of any of those
values into one search box.

**Why this priority**: without this the product has nothing to sync, back up, de-duplicate or
export. Every other domain in the system reads the rows this story writes.

**Independent Test**: create a contact through `POST /api/v1/contacts` with a `fields` object,
then `GET /api/v1/contacts?q=<a value that only appears in a child row>` and confirm the contact
comes back with its child collections attached.

**Acceptance Scenarios**:

1. **Given** a signed-in user with no address book row yet, **When** they create their first
   contact, **Then** an address book named "Contacts" is created for them on the spot and the
   contact lands in it (`internal/repository/bun_addressbook.go:45-67`,
   `internal/service/contact.go:70`).
2. **Given** a contact whose only match for "secret@work.org" is a row in `contact_emails` and
   not the denormalised `contacts.email` column, **When** the user searches for `secret`,
   **Then** that contact is returned (`internal/repository/bun_contact.go:166-199`;
   `internal/repository/bun_contact_search_test.go:16-63`).
3. **Given** a contact with two email addresses, **When** the user opens the list, **Then** the
   first address is shown with a `+1` badge rather than a truncated single value
   (`web/src/components/contacts/ContactTable.vue` — `primaryEmail`, `extraEmailCount`).
4. **Given** a save whose child rows cannot be written, **When** the write fails, **Then** the
   contact row is rolled back with it and the previous children remain intact
   (`internal/repository/bun_contact_relations.go:40-52`;
   `internal/repository/bun_contact_save_test.go:103-153`).

---

### User Story 2 - Edit a contact without destroying what the form cannot see (Priority: P1)

A contact that arrived from Google or from a phone carries properties this application does not
model — an embedded photo, `X-ABLabel`, a `KEY`. Renaming that contact in the web form must not
delete them.

**Why this priority**: the failure mode is silent and irreversible from the user's side, and it
propagates: the stripped card is pushed back to every synced device on the next run. This is the
reason the web form posts structured `fields` instead of a whole rebuilt card.

**Independent Test**: store a card containing `PHOTO:` and `X-ABLabel`, `PUT` a `fields` payload
that changes only the first name, and assert both properties are still in the stored card.

**Acceptance Scenarios**:

1. **Given** a stored card with `PHOTO:` and `X-ABLabel`, **When** the user changes the first
   name through the form, **Then** the edit is applied and both unmanaged properties survive
   (`internal/service/contact.go:158-168`;
   `internal/service/contact_fields_test.go:41-60`).
2. **Given** the same contact, **When** the user clears every email row, **Then** the emails are
   removed from the card and the photo is untouched
   (`internal/service/contact_fields_test.go:100-110`).
3. **Given** an edit through the form, **When** it is saved, **Then** the contact's UID does not
   change (`internal/vcard/merge.go:62-65`; `internal/service/contact_fields_test.go:87-97`).
4. **Given** a client that owns the whole card, **When** it sends `vcard_data` instead of
   `fields`, **Then** the stored card is replaced wholesale — including the loss of a photo the
   replacement omits (`internal/service/contact.go:169-175`;
   `internal/service/contact_fields_test.go:113-124`).
5. **Given** a save through any path, **When** it completes, **Then** the flat columns are
   re-derived from the card that was actually stored, not from the request
   (`internal/service/contact.go:164-168, 221`; `internal/service/contact_fields_test.go:63-85`).

*A managed property the `fields` payload does not model is the exception to this story's promise;
see Known Divergences, "`geo` is carried by the form but not editable in it".*

---

### User Story 3 - Work through a list: sort, filter, page, select, delete (Priority: P2)

Someone with thousands of contacts narrows the list by tag, organisation, "has email" or "has
phone", sorts it, pages through it, selects contacts across the page and deletes or exports the
selection in one action.

**Why this priority**: the record is useless at scale without this, but a single contact is still
viewable, editable and syncable if none of it existed.

**Independent Test**: seed contacts whose insertion order differs from every sort order under
test, then page through `GET /api/v1/contacts?sort_by=…&sort_dir=…` and assert the sequence and
the page boundaries.

**Acceptance Scenarios**:

1. **Given** contacts inserted in an order unrelated to any sort, **When** the list is requested
   sorted by name, email or creation time, **Then** the returned order follows the requested
   sort and pages slice that same sequence
   (`internal/repository/bun_contact_order_test.go:73-144`).
2. **Given** the filter bar, **When** a tag and an organisation are selected, **Then** only
   contacts carrying both are counted and returned
   (`internal/repository/bun_contact.go:34-48`;
   `internal/repository/bun_contact_filter_test.go:107-123`).
3. **Given** a selection of contacts, **When** the user confirms a bulk delete, **Then** one
   request deletes them and the response states how many of the requested ids actually existed
   (`internal/handler/contact_handler.go:162-186`;
   `web/src/views/contacts/ContactListView.vue:347-368`).
4. **Given** an id belonging to another user's address book in the same bulk request, **When**
   the delete runs, **Then** that contact is not touched and is not counted
   (`internal/repository/bun_contact_bulk_test.go:78-90`).
5. **Given** a user who types the confirmation word, **When** they confirm "Delete All", **Then**
   every contact in their address book is removed
   (`internal/handler/contact_handler.go:188-199`;
   `web/src/views/contacts/ContactListView.vue:163-172`).

---

### User Story 4 - Synchronising clients learn what changed, including deletions (Priority: P2)

A CardDAV client or a sync provider asks "what happened since token N" and receives exactly the
contacts written since then plus the UIDs of the ones removed.

**Why this priority**: this domain owns the write side of that answer. A deletion cannot be
inferred from the contacts table, so if this half is wrong a deleted contact lives on every
phone forever. The read side belongs to 004 and 006; this story is only about the counter and
the tombstones being written correctly.

**Independent Test**: exercise create / update / delete / bulk delete / delete-all against a
repository and assert the collection counter advances on each and that the tombstones name the
removed UIDs — `internal/repository/change_journal_test.go`.

**Acceptance Scenarios**:

1. **Given** any write — create, update, or delete — **When** it commits, **Then** the address
   book's change counter has advanced (`internal/repository/change_journal_test.go:51-72`).
2. **Given** a client with no token, **When** it asks for changes, **Then** it receives the whole
   collection and no deletions (`internal/repository/change_journal.go:112-114`;
   `internal/repository/change_journal_test.go:75-87`).
3. **Given** a contact deleted after the client's token, **When** the client asks, **Then** its
   UID appears as a deletion and not as an update
   (`internal/repository/change_journal_test.go:106-121`).
4. **Given** a contact recreated under a UID it once had, **When** the client asks, **Then** the
   UID is reported as present and not as deleted — the tombstone was dropped on the re-create
   (`internal/repository/bun_contact.go:66`;
   `internal/repository/bun_contact_relations.go:111-119`;
   `internal/repository/change_journal_test.go:159-174`).
5. **Given** a delete for an id that does not exist, **When** it runs, **Then** nothing changes
   and no tombstone is invented (`internal/repository/change_journal_test.go:177-187`).

---

### User Story 5 - Hand a contact to someone standing next to you (Priority: P3)

From a contact's page a user downloads its `.vcf` file or shows a QR code another phone can
scan.

**Why this priority**: a convenience on top of a record that is already complete. Nothing else
depends on it.

**Acceptance Scenarios**:

1. **Given** an open contact, **When** the user clicks "Download .vcf", **Then** the stored card
   is returned as `text/vcard` with a filename derived from the contact's UID
   (`internal/handler/contact_handler.go:201-216`).
2. **Given** an open contact, **When** the user opens the QR dialog, **Then** a PNG is fetched on
   demand and shown (`internal/handler/qrcode_handler.go:23-43`;
   `web/src/views/contacts/ContactViewView.vue:198-207`).

---

### Edge Cases

Observed in the code, not hypothetical. Behaviour that is deliberate and bounded lives here;
behaviour that contradicts an intent stated elsewhere lives in **Known Divergences**.

- **There is no top-level `photo_uri` create or update field at all.** `photo_uri` is populated
  only by parsing a `PHOTO` property out of a card that arrived as `vcard_data`, from an import,
  or from a sync provider (`internal/vcard/domain_helper.go:27`). The API returns it; nothing in
  the API sets it directly.
- **`vcard_data` on update is a loaded gun by design.** It replaces the whole card, so a client
  that sends a partial card silently destroys the rest (`internal/service/contact.go:169-175`).
  The web UI never uses this path (`web/src/utils/contact-form.ts:27-61`).
- **The bulk-delete cap is 500 ids per request**, above anything the paged UI can select, and an
  oversized request deletes nothing (`internal/service/contact.go:280-302`;
  `internal/service/contact_bulk_test.go:64-72`).
- **An empty id list deletes nothing**, and is never read as "everything"
  (`internal/handler/contact_handler.go:169-171`; `internal/repository/bun_contact.go:249-251`).
- **Deleting a contact that does not exist is not an error**; `DELETE /contacts/:id` answers 404
  only because the service checks ownership first (`internal/service/contact.go:240-247`), while
  the repository's own `Delete` is a no-op for an unknown id
  (`internal/repository/bun_contact.go:101-109`).
- **A UID can be resurrected.** Creating a contact under a UID that was deleted drops the
  tombstone, so the UID is reported to clients as present rather than deleted
  (`internal/repository/bun_contact.go:66`;
  `internal/repository/bun_contact_relations.go:111-119`).
- **Deleting one contact removes its child rows through `ON DELETE CASCADE`, which on SQLite
  depends on a connection pragma.** Foreign keys are enabled in the DSN
  (`internal/repository/db.go:33`); a deployment that opened the database differently would
  orphan child rows.

## Requirements *(mandatory)*

### Functional Requirements

#### Address book

- **FR-001**: Every contact operation MUST resolve the caller's address book, creating one named
  "Contacts" if the user does not have one yet, so that accounts predating the address-book table
  are repaired on first use (`internal/repository/bun_addressbook.go:45-67`, called from every
  method of `internal/service/contact.go`).
- **FR-002**: A create that loses a race MUST re-read and use the address book the other request
  created rather than failing (`internal/repository/bun_addressbook.go:59-65`).
- **FR-003**: The address book MUST carry a monotonic change counter used as the collection's
  CTag (`internal/domain/addressbook.go:17-19`; `migrations/021_change_journal.up.sql:11`).

#### The record

- **FR-004**: A contact MUST be stored as the authoritative vCard text plus denormalised scalar
  columns for display and filtering: names (first, middle, last, prefix, suffix, nickname),
  primary email, primary phone, organisation, department, title, role, note, birthday,
  anniversary, gender, time zone, geo, photo URI and rev
  (`internal/domain/contact.go:9-59`; `migrations/013_contacts_extra_fields.up.sql` adds the 13
  columns beyond the original set).
- **FR-005**: Repeated values MUST live in seven child tables — emails, phones, addresses, URLs,
  IMs, categories, dates — each keyed to its contact with `ON DELETE CASCADE`
  (`migrations/014_contact_child_tables.up.sql`; `internal/domain/contact_field.go`).
- **FR-006**: A contact and all of its child rows MUST be written in one transaction; a failure
  in any part MUST leave the previous state intact
  (`internal/repository/bun_contact_relations.go:40-52, 99-141`;
  `internal/repository/bun_contact_save_test.go:103-153`).
- **FR-007**: Saving with an empty child collection MUST delete the rows that were there, so
  removing the last email from a card removes its row
  (`internal/repository/bun_contact_relations.go:16-25`;
  `internal/repository/bun_contact_save_test.go:155-168`).
- **FR-008**: Child-row primary keys MUST be generated by the application, not the database
  (`internal/repository/bun_contact_relations.go:145-174`).
- **FR-009**: A contact's ETag MUST be derived from its vCard text as the first 8 bytes of its
  SHA-256, hex-encoded, through a single exported function so a repair command computes exactly
  what the write paths store (`internal/service/contact.go:341-351`).
- **FR-010**: A contact MUST be unique by `(address_book_id, uid)`
  (`migrations/001_init.up.sql:21-34`).

#### Create

- **FR-011**: The system MUST accept three create shapes, in precedence order: a structured
  `fields` object, a whole `vcard_data` card, or the flat single-value form
  (`internal/service/contact.go:81-105`).
- **FR-012**: When a submitted card carries no UID, the system MUST mint one and inject it into
  the card; when it carries one, that UID MUST be kept
  (`internal/service/contact.go:99-104`).
- **FR-013**: Empty values in a `fields` payload MUST be dropped rather than stored as blank
  rows — empty emails, phones, categories and wholly-empty addresses
  (`internal/service/contact_fields.go:61-70, 102-120`;
  `internal/service/contact_fields_test.go:150-166`).
- **FR-014**: The first email and first phone in a `fields` payload MUST become the contact's
  primary email and phone (`internal/service/contact_fields.go:128-133`).

#### Read

- **FR-015**: Reading one contact MUST return it with all seven child collections attached
  (`internal/repository/bun_contact_relations.go:246-262`;
  `internal/service/contact.go:131`).
- **FR-016**: A contact belonging to another user's address book MUST be indistinguishable from
  one that does not exist (`internal/service/contact.go:135-137, 152, 244`).
- **FR-017**: A request for an id that does not exist MUST answer 404, not 500 — the repository
  reports absence rather than surfacing "no rows"
  (`internal/repository/bun_contact_relations.go:253-257`;
  `internal/repository/bun_contact_save_test.go:172-182`).

#### Update

- **FR-018**: An update sent as `fields` MUST be merged into the stored card so that properties
  the form does not model survive, and MUST NOT change the contact's UID
  (`internal/service/contact.go:158-168`, delegating to `internal/vcard/merge.go:43`).
  The merge replaces the *managed* properties wholesale (`internal/vcard/merge.go:14-31`), so
  `ContactFields` MUST be able to express every one of them — including ones the browser has no
  control for, which it carries through unchanged (`internal/service/contact_fields.go:13-43`).
  A managed property missing from `ContactFields` is erased by every edit.
- **FR-019**: After a merge the flat columns MUST be re-derived by re-parsing the stored card,
  not from the request payload (`internal/service/contact.go:164-168, 221`).
- **FR-020**: An update sent as `vcard_data` MUST replace the stored card in full
  (`internal/service/contact.go:169-175`).
- **FR-021**: An update sent as flat fields MUST apply only the fields present, preserving the
  rest of the parsed card (`internal/service/contact.go:176-219`).
- **FR-022**: Every update MUST recompute the ETag and set the modification time
  (`internal/service/contact.go:222-223`).

#### Delete

- **FR-023**: Deleting a contact MUST remove the row and record a tombstone naming its UID, in
  one transaction (`internal/repository/bun_contact.go:100-121`).
- **FR-024**: Bulk delete MUST take a list of ids, de-duplicate it, reject more than 500 ids,
  delete only contacts in the caller's address book, and report how many rows were actually
  removed against how many were requested
  (`internal/service/contact.go:280-319`; `internal/repository/bun_contact.go:248-279`;
  `internal/handler/contact_handler.go:162-186`).
- **FR-025**: An empty id list MUST delete nothing — it must never be read as "everything"
  (`internal/handler/contact_handler.go:169-171`; `internal/repository/bun_contact.go:249-251`;
  `internal/repository/bun_contact_bulk_test.go:101-112`).
- **FR-026**: Delete-all MUST remove every contact in the caller's address book and tombstone all
  of their UIDs (`internal/repository/bun_contact.go:123-140`).

#### Change journal (written here, read elsewhere)

- **FR-027**: Every write MUST claim its collection sequence number inside the same transaction
  as the write itself, so a client can never read a CTag no contact carries yet
  (`internal/repository/change_journal.go:12-26`;
  `internal/repository/bun_contact_relations.go:44-50`).
- **FR-028**: Deletions MUST leave a tombstone row per removed UID carrying the sequence number
  of the deletion (`internal/repository/change_journal.go:28-48`).
- **FR-029**: Creating or resurrecting a UID MUST delete its tombstone
  (`internal/repository/bun_contact.go:66`;
  `internal/repository/bun_contact_relations.go:111-119`).
- **FR-030**: A "changes since" query MUST return contacts written after the given sequence and
  the UIDs deleted after it; with a zero token it MUST return the whole collection and no
  deletions (`internal/repository/change_journal.go:86-128`).

#### List, filter, search, facets

- **FR-031**: The list MUST be paginated, defaulting to 50 per page and clamping any limit
  outside 1..200 back to 50 (`internal/handler/contact_handler.go:24-30`).
- **FR-032**: The list MUST be sortable by name (last then first), email, organisation, creation
  time or update time, ascending or descending, with the sort column resolved against a
  whitelist (`internal/repository/bun_contact.go:14-32`).
- **FR-033**: The sort order requested MUST survive the loading of child collections; children
  are attached to the already-ordered page instead of re-selecting the parents
  (`internal/repository/bun_contact_relations.go:299-364`;
  `internal/repository/bun_contact_order_test.go:73-144`).
- **FR-034**: The list MUST be filterable by one or more categories, by organisation, by "has an
  email" and by "has a phone", where the last two consider both the flat column and the child
  rows (`internal/repository/bun_contact.go:34-48`).
- **FR-035**: Search MUST match a substring against the flat name, nickname, email, phone,
  organisation, department, title and note columns and, through a UNION over the child tables,
  against every email, phone, address part, URL, IM handle and category
  (`internal/repository/bun_contact.go:166-199`).
- **FR-036**: Search results MUST honour the same filters, sort and pagination as an unfiltered
  list (`internal/repository/bun_contact.go:194-198`;
  `internal/repository/bun_contact_order_test.go:146-157`).
- **FR-037**: A facets endpoint MUST return the total contact count and the distinct categories
  and organisations of the address book, with empty results expressed as empty lists
  (`internal/repository/bun_contact.go:201-243`; `internal/handler/contact_handler.go:74-86`).
- **FR-038**: List and search MUST return the total matching count alongside the page, so the UI
  can paginate (`internal/repository/bun_contact.go:160-163`;
  `internal/handler/contact_handler.go:66-71`).

#### Per-contact artefacts

- **FR-039**: A contact's stored vCard MUST be downloadable as `text/vcard` with a filename
  derived from its UID (`internal/handler/contact_handler.go:201-216`).
- **FR-040**: A contact MUST be renderable as a scannable PNG code, default 256 px, generated on
  request (`internal/handler/qrcode_handler.go:23-43`; `internal/service/qrcode.go:17-26`).

#### Routing and transport

- **FR-041**: Static contact sub-paths (`/facets`, `/bulk-delete`, and the duplicate routes owned
  by 007) MUST be registered before `/:id`, because matching is by registration order
  (`internal/handler/handler.go:114-143`).
- **FR-042**: Contact endpoints MUST require authentication and MUST be scoped to the
  authenticated user's address book (`internal/handler/handler.go:100`, plus FR-016).
- **FR-043**: Ordinary JSON endpoints, including contact writes, MUST be capped at 2 MiB per
  request body — generous for a contact with an embedded photo, far below the import allowance
  (`cmd/server/main.go:397-407`; `internal/handler/handler.go:79`).

#### Web UI

- **FR-044**: The contacts screen MUST offer a table view and a card view, remembering the
  choice, and defaulting to cards on a narrow viewport
  (`web/src/views/contacts/ContactListView.vue:221-226`).
- **FR-045**: The table MUST show an avatar (photo if present, otherwise coloured initials), the
  primary email and phone with a `+N` badge for the rest, organisation with title, and up to
  three tags with a `+N` overflow
  (`web/src/components/contacts/ContactTable.vue`;
  `web/src/components/contacts/ContactAvatar.vue`).
- **FR-046**: A photo stored as raw base64 MUST be rendered by detecting the JPEG or PNG magic
  prefix and adding the data-URI header; an unrecognised value MUST fall back to initials rather
  than issuing a broken request (`web/src/components/contacts/ContactAvatar.vue:35-46`).
- **FR-047**: The contact form MUST present the record in grouped sections — Name, Contact,
  Organization, Personal, Addresses, Web & Messaging, Tags — with add/remove rows for every
  repeated value (`web/src/components/contacts/ContactForm.vue`;
  `web/src/components/contacts/MultiFieldRow.vue`).
- **FR-048**: The form MUST submit a structured `fields` payload and never assemble a vCard in
  the browser (`web/src/utils/contact-form.ts:19-61`;
  `web/src/views/contacts/ContactCreateView.vue:28-38`;
  `web/src/views/contacts/ContactDetailView.vue`).
- **FR-049**: Opening a contact MUST show a read-only card — the common intent is to look
  something up — with edit behind a second click and every value one click to copy
  (`web/src/router/index.ts:43-52`; `web/src/views/contacts/ContactViewView.vue`;
  `web/src/components/contacts/CopyableRow.vue`).
- **FR-050**: The read-only card MUST omit sections that hold nothing
  (`web/src/views/contacts/ContactViewView.vue:36-38`).
- **FR-051**: Selecting contacts MUST raise a bulk action bar offering select-all, export and
  delete, and the count of the current selection
  (`web/src/components/contacts/BulkActionBar.vue`;
  `web/src/views/contacts/ContactListView.vue:142-149`).
- **FR-052**: Bulk delete from the UI MUST be one request, and MUST tell the user when fewer
  contacts were deleted than were selected
  (`web/src/views/contacts/ContactListView.vue:345-368`).
- **FR-053**: "Delete All" MUST require the user to type a confirmation word
  (`web/src/views/contacts/ContactListView.vue:163-172`).
- **FR-054**: The filter bar MUST be driven by the facets endpoint, showing the user's own tags
  and organisations and a count of active filters
  (`web/src/components/contacts/FilterBar.vue`;
  `web/src/views/contacts/ContactListView.vue:26-39, 271-274`).
- **FR-055**: The dashboard MUST show the user's total contact count, obtained from the contacts
  list rather than a dedicated endpoint (`web/src/views/DashboardView.vue:109-116`).

### Key Entities

- **AddressBook** — one per user in practice, created on demand, named "Contacts". Holds the
  collection's change counter (`internal/domain/addressbook.go`).
- **Contact** — the record. Owns a UID unique within its address book, the authoritative vCard
  text, a derived ETag, the sequence number of its last write, and denormalised scalar columns
  for display, sorting and filtering (`internal/domain/contact.go`).
- **ContactEmail / ContactPhone / ContactURL / ContactIM** — a value with a type, a preference
  rank and (for emails and phones) a label (`internal/domain/contact_field.go:5-67`).
- **ContactAddress** — PO box, extended, street, city, region, postal code, country, plus type
  and label (`internal/domain/contact_field.go:29-45`).
- **ContactCategory** — one tag (`internal/domain/contact_field.go:69-76`).
- **ContactDate** — a date of a kind: `bday`, `anniversary` or `other`, stored as its raw vCard
  string (`internal/domain/contact_field.go:78-87`).
- **ChildRecords** — the seven collections bundled as one value, because a contact and its
  children have to be written together (`internal/domain/contact_field.go:89-100`).
- **ContactTombstone** — the memory that a UID was deleted, at a given sequence
  (`internal/domain/contact_tombstone.go`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A contact found by any of its stored values — including a value that exists only in
  a repeated field — is returned by a single search query, with no field-specific syntax required
  from the user.
- **SC-002**: Editing a contact through the application's own form never removes an *unmanaged*
  property the form does not display. Measured as: 0 of the unmanaged properties present before
  an edit are absent after it. (Managed-but-unmodelled properties are the stated exception; see
  Known Divergences.)
- **SC-003**: A failed write leaves no partially-updated contact: after a rejected save, 100% of
  the previous scalar values and child rows are still readable.
- **SC-004**: Every state-changing operation on a contact is observable by a synchronising client
  in one query — 5 of 5 write kinds (create, update, single delete, bulk delete, delete-all)
  advance the collection counter and, where applicable, name the removed UIDs.
- **SC-005**: A user can remove an arbitrary selection of up to 500 contacts in one request and is
  told the exact number removed; a request above that bound removes nothing.
- **SC-006**: A page of contacts is bounded at 200 records regardless of what the client asks
  for, so no single request can be made to return an entire large address book.
- **SC-007**: The order a user asks for is the order they get, on every page: 4 of 4 sort
  configurations and both pages of a paginated read return the exact expected sequence.
- **SC-008**: A contact id from another account is never readable, updatable or deletable —
  cross-address-book access is rejected at the service boundary and again in the delete
  statement.
- **SC-009**: A contact write request is refused above 2 MiB, which is small enough that N
  concurrent writes cannot exhaust memory before a handler runs.

## Assumptions

Written as found in the code, including where the implementation deliberately stops short.

- **Migrations are forward-only.** `.down.sql` files exist but no code applies them
  (constitution, Principle I; `internal/repository/db.go`). The 13 columns added by `013` and the
  seven tables added by `014` can never be dropped by an upgrade; a rollback means restoring a
  dump.
- **SQLite runs on a single connection** (`internal/repository/db.go:37`). Every transaction in
  this domain — and there is one per write — serialises against every other database operation in
  the process. This is what makes the concurrent-address-book race unreachable on SQLite, and it
  is also why long-running operations elsewhere in the system are documented as a hazard.
- **The vCard is the authority; the columns are a cache.** Any disagreement is resolved by
  re-parsing the card (`internal/service/contact.go:221`). A change to the encoder therefore
  moves every affected contact's ETag, which is the reason the `reencode-vcards` command exists
  (constitution, Principles II and V).
- **The API exposes no way to create a second address book**, and the CardDAV path layout assumes
  exactly one (`/dav/{email}/addressbooks/contacts/`). That the database does not enforce this is
  recorded in Known Divergences.
- **Birthday and anniversary are stored as raw vCard date strings** and displayed as stored on the
  read-only card (`internal/domain/contact_field.go:85`;
  `web/src/views/contacts/ContactViewView.vue:102-103`). Only the form converts between
  `YYYYMMDD` and the HTML date input (`web/src/utils/contact-form.ts:6-17`).
- **The bulk "Export" button in this domain's UI calls the export endpoint owned by 005**
  (`web/src/views/contacts/ContactListView.vue:372-385` → `GET /export/vcard?ids=`). Only the
  button and its selection belong here.
- **Deleting a contact is unconditional.** There is no soft delete, no undo and no retention
  window; the tombstone remembers the UID, not the contact
  (`internal/domain/contact_tombstone.go`). "Delete All" is guarded only by a typed confirmation
  in the UI (`web/src/views/contacts/ContactListView.vue:168`).
- **The dashboard is a landing page, not an analytics surface.** It shows the contact total beside
  a static tile and the user's role (`web/src/views/DashboardView.vue:27-77`); what that means in
  practice is recorded in Known Divergences.

## Status

Shipped. Reconstructed from the tree at `23a167c`, which is tag `v0.4.0` plus three follow-up
commits (two documentation, one `config.example.yaml` fix — none of them touch this domain).
Every requirement below describes behaviour present in that tree.

## Code Paths

Owned by this spec. Nothing here is claimed by another spec, and no bare directory is claimed in
the five dense trees.

- `internal/domain/addressbook.go`
- `internal/domain/contact.go`
- `internal/domain/contact_field.go`
- `internal/domain/contact_tombstone.go`
- `internal/handler/contact_handler.go`
- `internal/handler/qrcode_handler.go`
- `internal/repository/bun_addressbook.go`
- `internal/repository/bun_contact.go`
- `internal/repository/bun_contact_relations.go`
- `internal/repository/bun_contact_bulk_test.go`
- `internal/repository/bun_contact_filter_test.go`
- `internal/repository/bun_contact_order_test.go`
- `internal/repository/bun_contact_save_test.go`
- `internal/repository/bun_contact_search_test.go`
- `internal/repository/change_journal.go`
- `internal/repository/change_journal_test.go`
- `internal/repository/types.go`
- `internal/service/addressbook.go`
- `internal/service/contact.go`
- `internal/service/contact_test.go`
- `internal/service/contact_flat_update_test.go`
- `internal/service/contact_bulk_test.go`
- `internal/service/contact_fields.go`
- `internal/service/contact_fields_test.go`
- `internal/service/qrcode.go`
- `migrations/013_contacts_extra_fields.up.sql`
- `migrations/013_contacts_extra_fields.down.sql`
- `migrations/014_contact_child_tables.up.sql`
- `migrations/014_contact_child_tables.down.sql`
- `migrations/021_change_journal.up.sql`
- `migrations/021_change_journal.down.sql`
- `web/src/api/contacts.ts`
- `web/src/stores/contacts.ts`
- `web/src/utils/contact-form.ts`
- `web/src/utils/contact-form.spec.ts`
- `web/src/views/DashboardView.vue`
- `web/src/views/contacts/ContactCreateView.vue`
- `web/src/views/contacts/ContactDetailView.vue`
- `web/src/views/contacts/ContactListView.vue`
- `web/src/views/contacts/ContactViewView.vue`
- `web/src/components/contacts/BulkActionBar.vue`
- `web/src/components/contacts/ContactAvatar.vue`
- `web/src/components/contacts/ContactAvatar.spec.ts`
- `web/src/components/contacts/ContactCard.vue`
- `web/src/components/contacts/ContactForm.vue`
- `web/src/components/contacts/ContactTable.vue`
- `web/src/components/contacts/CopyableRow.vue`
- `web/src/components/contacts/FilterBar.vue`
- `web/src/components/contacts/MultiFieldRow.vue`

## References

Paths this spec touches but does **not** own.

- `internal/vcard/merge.go` — owned by 003. `MergeIntoVCard` is what makes FR-018 true; its own
  contract, including which properties it treats as managed, belongs to 003.
- `internal/vcard/domain_helper.go` — owned by 003. Re-derives the flat columns from the stored
  card (FR-019).
- `internal/vcard/parser.go` — owned by 003.
- `internal/handler/handler.go` — owned by 008. Registers this domain's routes; FR-041 to FR-043
  state what that registration must preserve.
- `cmd/server/main.go` — owned by 008. Composition root and the body-limit policy behind FR-043.
- `internal/repository/db.go` — owned by 008. Driver setup, the SQLite single-connection pool and
  the foreign-key pragma this domain's `ON DELETE CASCADE` relies on.
- `internal/repository/interfaces.go` — owned by 008. Declares `ContactRepository` and
  `AddressBookRepository`.
- `internal/service/exporter.go` — owned by 005. Reached from this domain's bulk-selection
  "Export" button only.
- `migrations/001_init.{up,down}.sql` — owned by 008. Creates `address_books` and `contacts`;
  FR-010 and the missing `UNIQUE(user_id)` both live in that file.

Boundaries with sibling specs, stated so ownership is unambiguous: the vCard text itself is 003;
the CardDAV protocol surface that *reads* the change journal is 004; file import and export are
005; how a contact reaches a remote provider is 006; duplicate detection, merging two contacts
and every `Merge*`/`Duplicate*` file is 007.

Three read paths live in files this spec owns but exist only to serve 007, which states what
they must return: `ListForDedup` and `ListDedupValues`
(`internal/repository/bun_contact.go:305-359`) and the two-column projection they share,
`domain.ContactValueRef` (`internal/domain/contact.go:61-70`). They are deliberately narrower
than this domain's own reads — neither touches `vcard_data` or `photo_uri`, and
`ListDedupValues` returns bare `(contact_id, value)` rows from `contact_emails` and
`contact_phones` rather than loading the child collections the way `GetByIDWithRelations` does
(007 FR-013). Changing what a contact's emails or phones look like is this spec's business;
changing what detection does with them is not.

## Enforced By

**Repository layer** (package `repository_test`, run by `go test ./... -count=1 -race` in the
`test` job of `.github/workflows/ci.yml`):

- `TestSave_InsertsContactAndChildren`, `TestSave_UpdatesExistingContactAndReplacesChildren`
  (`internal/repository/bun_contact_save_test.go`) — FR-005, FR-006.
- `TestSave_RollsBackContactWhenChildWriteFails`, `TestSave_RollsBackUpdateWhenChildWriteFails`
  (`internal/repository/bun_contact_save_test.go`) — FR-006, SC-003.
- `TestSave_EmptyChildrenClearsExistingRows` (`internal/repository/bun_contact_save_test.go`) —
  FR-007.
- `TestGetWithRelations_UnknownIDReturnsNilWithoutError`
  (`internal/repository/bun_contact_save_test.go`) — FR-017.
- `TestContactList_SortByNameAsc`, `TestContactList_SortByNameDesc`,
  `TestContactList_SortByCreatedAt` (`internal/repository/bun_contact_filter_test.go`) — FR-032.
- `TestContactList_FilterByCategory`, `TestContactList_FilterByOrg`,
  `TestContactList_FilterHasEmail`, `TestContactList_FilterHasPhone`
  (`internal/repository/bun_contact_filter_test.go`) — FR-034.
- `TestContactFacets` (`internal/repository/bun_contact_filter_test.go`) — FR-037.
- `TestListWithRelations_PreservesSortOrder`, `TestListWithRelations_StillLoadsChildren`,
  `TestListWithRelations_PaginationIsStable`, `TestSearchWithRelations_PreservesSortOrder`
  (`internal/repository/bun_contact_order_test.go`) — FR-033, FR-036, SC-007.
- `TestContactSearch_ByChildEmail`, `TestContactSearch_ByChildPhone`,
  `TestContactSearch_ByChildCategory`, `TestContactSearchWithRelations_LoadsChildRows`
  (`internal/repository/bun_contact_search_test.go`) — FR-035, FR-015, SC-001.
- `TestDeleteMany_RemovesOnlyTheNamedContacts`,
  `TestDeleteMany_IgnoresContactsFromAnotherAddressBook`,
  `TestDeleteMany_UnknownIDsAreNotAnError`, `TestDeleteMany_EmptyIDsIsANoop`
  (`internal/repository/bun_contact_bulk_test.go`) — FR-024, FR-025, SC-008.
- `TestListByIDs_ReturnsOnlyOwnContactsInSortOrder`, `TestListByIDs_EmptyIDsReturnsNothing`
  (`internal/repository/bun_contact_bulk_test.go`) — the id-scoped read behind the UI's bulk
  selection.
- `TestChangeSeq_AdvancesOnEveryWrite` (`internal/repository/change_journal_test.go`) — FR-027,
  SC-004.
- `TestChangesSince_ZeroTokenReturnsWholeCollection`,
  `TestChangesSince_ReportsOnlyWhatHappenedAfterTheToken`, `TestChangesSince_ReportsDeletions`,
  `TestChangesSince_ReportsBulkDeletions`, `TestChangesSince_ReportsDeleteAll`
  (`internal/repository/change_journal_test.go`) — FR-023, FR-026, FR-028, FR-030.
- `TestChangesSince_RecreatedUIDHasNoTombstone` (`internal/repository/change_journal_test.go`) —
  FR-029.
- `TestDelete_UnknownContactIsANoop` (`internal/repository/change_journal_test.go`) — the
  unknown-id edge case.

**Service layer** (package `service_test`):

- `TestCreate_SetsAllFields`, `TestCreate_EmptyTitleNote_NotInVCard`, `TestUpdate_ModifiesTitleNote`,
  `TestGenerateVCard_IncludesTitleNote`, `TestGenerateVCard_OmitsEmptyFields`
  (`internal/service/contact_test.go`) — FR-011 (flat form), FR-021.
- `TestUpdate_WithFieldsPreservesUnmodelledProperties` (`internal/service/contact_fields_test.go`)
  — FR-018, SC-002.
- `TestUpdate_WithFieldsRefreshesFlatColumns` (`internal/service/contact_fields_test.go`) — FR-019.
- `TestUpdate_WithFieldsKeepsUID` (`internal/service/contact_fields_test.go`) — FR-018 (UID half).
- `TestUpdate_WithFieldsClearsRemovedValues` (`internal/service/contact_fields_test.go`) — FR-007
  through the service.
- `TestUpdate_WithVCardDataStillReplacesWholeCard` (`internal/service/contact_fields_test.go`) —
  FR-020.
- `TestCreate_WithFieldsBuildsFullCard` (`internal/service/contact_fields_test.go`) — FR-011,
  FR-014.
- `TestContactFields_ToParsedSkipsEmptyValues` (`internal/service/contact_fields_test.go`) —
  FR-013.
- `TestContactFields_ToParsedCarriesGeo` (`internal/service/contact_fields_test.go`),
  `TestUpdate_FieldsEditKeepsGeo` and `TestUpdate_FieldsEditWithEmptyGeoClearsIt`
  (`internal/service/contact_test.go`) — FR-018's managed-set half, and the pin on
  "absent means clear" so nobody turns it into "absent means preserve".
- `TestDeleteMany_DeletesInOneCallAndReportsCount`, `TestDeleteMany_DeduplicatesIDs`,
  `TestDeleteMany_RejectsAnOversizedRequest`, `TestDeleteMany_EmptyListDeletesNothing`
  (`internal/service/contact_bulk_test.go`) — FR-024, FR-025, SC-005.
- `TestExportVCardByIDs_ExportsOnlyTheSelectedContacts`,
  `TestExportVCardByIDs_EmptyIDsExportsEverything`,
  `TestExportVCard_MatchesExportVCardByIDsWithNoIDs`,
  `TestExportVCardByIDs_RejectsAnOversizedRequest` (`internal/service/contact_bulk_test.go`) —
  these live in a file this spec owns but enforce the export contract owned by 005; they are named
  here so nobody deletes them as unclaimed.

**Frontend** (vitest, `npm run test` in the `frontend` job of `.github/workflows/ci.yml`):

- `web/src/utils/contact-form.spec.ts` — `toFieldsPayload` drops blank rows, trims, converts
  dates, and *never sends a `vcard_data` field*; `formFromContact` prefers child rows over the
  denormalised primaries and round-trips a birthday. Enforces FR-048 and the browser half of
  FR-013. The three `geo round-trip (no visible input)` cases are the **only** guard on `geo`
  surviving a browser edit — there is no control whose disappearance a reviewer would notice.
- `web/src/components/contacts/ContactAvatar.spec.ts` — initials, the `?` fallback, the JPEG and
  PNG magic-prefix repair, pass-through of an already-usable URL, and refusing to request an
  unrecognised photo. Enforces FR-046.

**End to end** (the `docker` job of `.github/workflows/ci.yml`): the step *"vCard list separators
survive a round trip"* creates a contact through `POST /api/v1/contacts` with a `fields` body
against a running image and asserts the result in the export. It is the only automated exercise
of this domain's HTTP layer.

**Review-only, with no automated enforcer.** Stated here rather than left to be discovered:

- FR-001, FR-002, FR-003 — address-book creation, the lost-race re-read and the CTag counter have
  no direct test. The counter is covered indirectly by `TestChangeSeq_AdvancesOnEveryWrite`; the
  race is not covered at all.
- FR-008, FR-009, FR-010, FR-012, FR-016, FR-022 — id generation, the ETag derivation, the
  `(address_book_id, uid)` uniqueness, UID minting and injection, cross-user invisibility and the
  ETag/`updated_at` refresh on update are all read from the code only.
- FR-031, FR-037 (handler half), FR-038 (handler half), FR-039, FR-040, FR-041, FR-042, FR-043,
  SC-006, SC-009 — `internal/handler/contact_handler.go` and `internal/handler/qrcode_handler.go`
  have **no test file at all**; the only tests in `internal/handler` are `health_test.go` and
  `registration_policy_test.go`.
- FR-044, FR-045, FR-047, FR-049 to FR-055 — the views and the remaining components have no
  component tests; the only two `.spec.ts` files this spec owns are the two named above.

## Known Divergences

**A flat update merges into the stored card instead of rebuilding it.** `PUT {"first_name":"X"}`
used to run `BuildVCard`, which renders only the properties this application models — so an
edit through the flat path deleted PHOTO, KEY, X-ABLabel and every other property the card
arrived with. It now merges. Separately, and predating that: `BuildVCard` keeps `p.FN` when it is
already set, and `parsed` is seeded from the stored card, so renaming through the flat path
updated `N` and left `FN` reading the old name. `FN` is now cleared when a name component
changes, which re-derives it while leaving a deliberately-set display name alone on other edits.


**Search folds case explicitly, because the two supported engines disagree.** `LIKE` is
case-sensitive on PostgreSQL and folds ASCII on SQLite, and the whole suite runs on SQLite — so a
bare `LIKE` passed everywhere while `john` failed to find `John Smith` on the engine
docker-compose provisions. Both sides of every comparison are now lowered
(`internal/repository/bun_contact.go`), and `TestPostgres_SearchIsCaseInsensitive` runs on the
engine that actually differs. Note the folding is ASCII-only in SQLite: a query in a non-Latin
script still matches only exactly on that engine.


- **An empty address book serialises `contacts` as JSON `null`, not `[]`, while
  `GET /contacts/facets` normalises its empty lists to `[]`.** The repository returns a nil slice
  when nothing matches and the handler passes it straight to the encoder
  (`internal/repository/bun_contact.go:155-164`, `internal/handler/contact_handler.go:66-71`);
  the facets path deliberately does the opposite (`internal/repository/bun_contact.go:224-226,
  238-240`). Reproduced against Bun v1.2.18 + SQLite. The list view dereferences `.length` on that
  value (`web/src/views/contacts/ContactListView.vue:88`) —
  [NEEDS CLARIFICATION: whether the SPA actually throws on a first-run empty book was not verified
  by running the app]. Two endpoints in one domain disagree about how to express "nothing".
- **`limit` is clamped, `offset` is not.** FR-031 bounds the page size; nothing bounds the offset.
  It is whatever `strconv.Atoi` produced, including a negative number, and a parse error silently
  becomes 0 (`internal/handler/contact_handler.go:24-30`). No test covers either.
- **An unknown `sort_by` silently sorts by name** rather than being rejected. The column list is a
  whitelist precisely because the value reaches an `ORDER BY` expression, so this is safe, but a
  client with a typo gets a wrong order and no signal
  (`internal/repository/bun_contact.go:14-32`).
- **Search is `LIKE '%q%'`, and `LIKE` is not case-insensitive on both engines.** SQLite folds
  ASCII case by default; PostgreSQL's `LIKE` does not, so FR-035 and SC-001 hold on SQLite and are
  unverified on PostgreSQL. **No test in this domain runs against PostgreSQL at all**: the
  PostgreSQL CI job runs `go test ./internal/repository/ -run TestPostgres`, and not one test in
  the six repository test files this spec owns carries a name matching that filter
  (`.github/workflows/ci.yml`, job `postgres`; CLAUDE.md, "PostgreSQL coverage naming").
- **`geo` is carried by the form but not editable in it, and an API client that omits it still
  clears `GEO`.** `ContactFields` now has a `geo` key (`internal/service/contact_fields.go:38-42`)
  and `ToParsed` copies it (`:94`), so the web form round-trips the stored value instead of
  deleting it. But no control renders it: the form reads `contact.geo` and posts it back unchanged
  (`web/src/utils/contact-form.ts:47-49, 84`), so a user cannot set or correct a location through
  the browser, and the value is displayed nowhere. Because `fields` is a **full replacement of the
  managed set**, a direct API caller who posts `fields` without a `geo` key still clears `GEO` —
  exactly as omitting `note` clears the note. That is the deliberate contract, pinned by
  `TestUpdate_FieldsEditWithEmptyGeoClearsIt`; preserving absent keys would make Note, Gender and
  TZ unclearable. The surviving-properties guarantee of User Story 2 therefore still covers
  *unmanaged* properties only: any property that is managed (`internal/vcard/merge.go:14-31`) but
  unmodelled by the form payload is silently erased by an edit.
- **The QR code is a MECARD, not a vCard**, despite the method being called `GenerateVCardQR`. It
  carries exactly last name, first name, one phone, one email and the organisation — the
  denormalised columns only, so a contact's second phone number is not in the code
  (`internal/service/qrcode.go:17-26`). FR-040 is written to say "a scannable PNG code" for that
  reason. Untested.
- **`POST /api/v1/contacts` accepts unknown top-level keys and still answers 201.** The body is
  decoded into `CreateContactInput`, which has exactly `first_name`, `last_name`, `email`,
  `phone`, `org`, `title`, `note`, `vcard_data` and `fields`
  (`internal/service/contact.go:35-48`); anything else — `categories`, `addresses`, `urls`,
  `ims`, a second email — is discarded in silence and the response is 201 with a contact that does
  not contain it (`internal/handler/contact_handler.go:88-105`). Multi-value data must go inside
  `fields`, and a caller who does not know that gets no error.
- **Nothing enforces one address book per user.** `address_books` has no `UNIQUE(user_id)`
  (`migrations/001_init.up.sql:11-19`), and `GetOrCreateByUserID` only re-reads after its own
  insert *fails* (`internal/repository/bun_addressbook.go:59-65`). Two simultaneous first-writes
  can therefore both succeed and leave two address books; subsequent reads would see whichever row
  the engine returns first. On SQLite the single-connection pool makes this unreachable in
  practice (`internal/repository/db.go:37`); on PostgreSQL it is not structurally prevented.
  FR-002 describes the mitigation, not a guarantee.
- **Tombstones are never pruned.** No code deletes a tombstone except the re-create path
  (`internal/repository/change_journal.go:50-63`), so the table grows with the number of deletions
  the installation has ever performed. There is no retention setting and no command to compact it.
- **`pref` is never set by the web form, and labels are stored but never sent.** The payload
  builder emits only `value` and `type` (`web/src/utils/contact-form.ts:28-29`), so "primary"
  means "first row" for anything created or edited in the browser, even though the model and the
  service accept a real preference rank and a per-value `label`
  (`internal/service/contact_fields.go:48-59`; `web/src/utils/contact-form.ts:51-58`). Values
  arriving from a sync provider or an import can carry both, which the table then honours
  (`web/src/components/contacts/ContactTable.vue` — `primaryEmail`).
- **The contact form has seven field sections, not the eight this project's earlier notes claim**:
  Name, Contact, Organization, Personal, Addresses, Web & Messaging, Tags
  (`web/src/components/contacts/ContactForm.vue`). The eighth block is the Cancel/Save action row.
  FR-047 is written to the seven that exist.
- **The dashboard renders one real number, not "cross-cutting counts".** The contact total is
  fetched as a one-record list request; the other two tiles are a hard-coded "CardDAV: Active"
  string and the user's role (`web/src/views/DashboardView.vue:27-77, 109-116`). A failure to load
  the count is swallowed and leaves `0` on screen — indistinguishable from an empty address book.
- **`internal/service/addressbook.go` is dead code.** `AddressBookService` is never constructed:
  the only references to it in the repository are its own declaration and constructor. Handlers
  reach the address book through `ContactService` and match on `ErrAddressBookNotFound`, which is
  declared in `internal/service/contact.go:19`. The file is owned here so that its removal is a
  decision rather than an accident.
- **The change-journal delete helpers have one caller outside this domain.**
  `internal/repository/bun_contact_relations.go:66-95` is also used by the merge path in 007, which
  shares a single sequence number between the surviving contact's update and the merged-away
  contact's tombstone. The helpers live here; that caller does not, and a change to their
  signature or their sequencing has an owner in another spec.
- **No performance target exists for this domain.** Search is an unindexed `LIKE '%…%'` scan plus
  a UNION over six child tables and there is no benchmark for list, search or facets, so no
  latency or throughput claim is made in Success Criteria.
  [NEEDS CLARIFICATION: a target would have to be established by measurement before it could be
  claimed.] The measured `×4870` figure in the project's notes belongs to duplicate detection
  (007) and is not restated here.
- **`ON DELETE CASCADE` on the seven child tables is enforced by a DSN pragma, not by a test.**
  `internal/repository/db.go:33` opens SQLite with `_pragma=foreign_keys(1)`; no test asserts that
  a deployment which opened the database differently would be caught.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-11 | unreleased | Flat updates merge rather than rebuild, so unmodelled vCard properties survive; `FN` re-derives on a rename. Covered by `TestUpdate_FlatEditKeepsUnmodelledProperties`. | — |
| 2026-08-11 | unreleased | Empty list and search results are built as `[]` rather than nil, so a no-match search no longer serialises as `null` and blanks the list view; the same normalisation applied to `ListByIDs`, `ListAll` and `ListForDedup`, at the repository rather than at each caller. Search now folds case on both sides. Both covered by `TestPostgres_EmptyListAndSearchAreNotNil` and `TestPostgres_SearchIsCaseInsensitive`. | — |
| 2026-08-07 | v0.4.0 | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | v0.4.0 | Conformed to the house template: house header, six repo sections added, `Scope Note` folded into the header prose and Code Paths/References, `Surprises` folded into Known Divergences, Success Criteria stripped of test and `.go` citations (moved to Enforced By). | — |
| 2026-08-07 | unreleased | D6: `ContactFields` gained `geo`, so a form edit no longer erases `GEO`. FR-018 now states that `ContactFields` must be able to express the whole managed set. The divergence is narrowed, not removed: `geo` has no input in the browser (round-trip only), and a direct API caller who omits `geo` from `fields` still clears it, because `fields` remains a full replacement of the managed set. | — |
| 2026-08-07 | unreleased | **D5** — `ContactRepository` gained `ListDedupValues`, and `internal/domain/contact.go` the `ContactValueRef` two-column projection it returns, so duplicate detection can bucket on secondary emails and phone numbers. Both live in files this spec owns; what they must return is stated by 007 FR-001a and FR-013, and the boundary paragraph under References now says so. No behaviour of this domain changed: no existing query, entity or field was touched. | — |
