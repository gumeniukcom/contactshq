# Feature Specification: vCard Representation & Encoding

Kind: component
Status: shipped
Constitution: v1.0.0

Reconstructed from the implementation at `23a167c`, released as v0.4.0 (`CHANGELOG.md:9`). Nothing
below was designed and then built; every requirement was read out of the shipped code and carries
the citation that proves it. Where the code does something the requirements do not endorse, it is
recorded under **Known Divergences** rather than promoted into a requirement.

This is a `component` domain — the very example the house template names when it says "nobody ships
`internal/vcard` alone". The user stories below are therefore consumer scenarios written as the
things an end user or an operator actually does through the one representation this component owns.
The card text stored in `contacts.vcard_data` is the system of record; the flat columns and child
tables are derived from it (`internal/vcard/domain_helper.go:10`).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A contact keeps its data through storage and export (Priority: P1)

A user imports or creates a contact whose values contain characters that mean something in the
vCard grammar — a `data:` photo URI containing a comma, two categories separated by a comma, a note
containing a comma, an address with structured components. The contact is exported, fetched over
CardDAV, or synced outward, and every value still means what it meant on the way in.

**Why this priority**: this is the defect the domain exists to prevent. Before the value-type-aware
encoder, every stored photo came out as `PHOTO:data:image/jpeg;base64\,/9j/…` and did not render on
iOS, and `CATEGORIES:work\,friends` decoded as one category named `work,friends`
(`internal/vcard/encoder.go:15-42`, `CHANGELOG.md:101`).

**Independent Test**: create a contact with `{"fields":{"categories":["work","friends"]}}`, export
via `GET /api/v1/export/vcard`, and confirm the line reads `CATEGORIES:work,friends`. This is
exactly what CI asserts against a live container (`.github/workflows/ci.yml:231-251`).

**Acceptance Scenarios**:

1. **Given** a card whose PHOTO is `data:image/jpeg;base64,/9j/4AAQ`, **When** the system writes it,
   **Then** the emitted line is `PHOTO:data:image/jpeg;base64,/9j/4AAQ` with no backslash
   (`internal/vcard/encoder_test.go:48-54`).
2. **Given** a NOTE containing `hello, world`, **When** the system writes it, **Then** the comma is
   escaped as `NOTE:hello\, world`, because in a TEXT value a comma is data
   (`internal/vcard/encoder_test.go:86-92`).
3. **Given** `CATEGORIES` holding `work,friends`, **When** the card is written and read back,
   **Then** the decoder reports two categories (`internal/vcard/encoder_test.go:200-202`).
4. **Given** a value containing CRLF or a bare CR, **When** the card is written, **Then** the only
   carriage returns in the output are line terminators and the value carries `\n`
   (`internal/vcard/encoder_test.go:117-130`).
5. **Given** a card with no value the old writer mishandled, **When** it is written, **Then** the
   bytes are identical to what `go-vcard`'s own encoder produces, so no stored ETag moves and no
   CardDAV client re-downloads (`internal/vcard/encoder_test.go:141-167`).

---

### User Story 2 - Editing a contact does not delete what the app does not model (Priority: P1)

A contact arrives from Google or an iPhone carrying an embedded PHOTO, `X-ABLabel`,
`X-SOCIALPROFILE`, `KEY`, `PRODID` and `REV`. The user renames it in the web form and saves. The
properties the form never showed are still on the card afterwards.

**Why this priority**: the alternative — rebuilding the card from the modelled fields — silently
destroys the photo of every synced contact on the first edit (`internal/vcard/merge.go:34-42`).

**Independent Test**: `PUT /api/v1/contacts/:id` with a `fields` body on a contact whose card has
`X-` properties, then re-read the card and confirm they are present.

**Acceptance Scenarios**:

1. **Given** a stored card with unmodelled properties, **When** the user edits the name through
   `fields`, **Then** PHOTO, X-ABLabel, X-SOCIALPROFILE, KEY, PRODID and REV all survive
   (`internal/vcard/merge_test.go:36-65`; the call site is `internal/service/contact.go:158-164`).
2. **Given** the user clears every email in the form, **When** the edit is saved, **Then** the EMAIL
   lines are gone and the unmodelled properties are untouched
   (`internal/vcard/merge_test.go:89-104`).
3. **Given** the caller sends a different UID, **When** the edit is saved, **Then** the stored UID
   wins (`internal/vcard/merge_test.go:107-122`, `internal/vcard/merge.go:63-66`).
4. **Given** nothing was actually changed, **When** the edit is saved twice, **Then** both results
   are byte-identical (`internal/vcard/merge_test.go:150-165`).

This guarantee holds for the `fields` path only. The flat-field path does not have it — see
**Known Divergences**.

---

### User Story 3 - An operator repairs cards written by the old encoder (Priority: P2)

An instance that ran an earlier version has cards on disk with the broken escaping. The operator
runs `contactshq reencode-vcards`, sees how many cards would change, then re-runs it with
`--apply --reconcile-sync-state` during a maintenance window.

**Why this priority**: fixing the writer does not fix data already written. It is P2 rather than P1
because it is a one-off migration path, not a daily operation.

**Independent Test**: run the command with no flags against a database holding a legacy card and
confirm it prints a count and `Dry run — nothing was written`
(`.github/workflows/ci.yml:277-278`).

**Acceptance Scenarios**:

1. **Given** no flags, **When** the command runs, **Then** nothing is written and the output ends
   with a dry-run notice (`cmd/server/reencode.go:89-92`, `cmd/server/reencode_test.go:72-88`).
2. **Given** `--apply` without `--reconcile-sync-state`, **When** the command runs, **Then** it
   exits 2 and explains that the next export would rewrite the whole remote address book
   (`cmd/server/reencode.go:54-61`, `cmd/server/reencode_test.go:156-163`,
   `.github/workflows/ci.yml:280-284`).
3. **Given** `--apply --reconcile-sync-state`, **When** the command finishes, **Then** each rewritten
   contact carries the ETag the ordinary write paths would have produced
   (`cmd/server/reencode.go:164`, `cmd/server/reencode_test.go:90-106`).
4. **Given** the command has already been applied, **When** it is run again, **Then** it reports
   `0 of N` — the rewrite is a fixed point (`.github/workflows/ci.yml:286-288`,
   `cmd/server/reencode_test.go:44-47`).
5. **Given** a stored card the decoder cannot read, **When** the command applies, **Then** that row
   is counted and left byte-for-byte alone (`cmd/server/reencode.go:144-148`,
   `cmd/server/reencode_test.go:52-70`).
6. **Given** the rewrite succeeded but reconciliation failed, **When** the command exits, **Then** it
   exits 1 and tells the operator not to run a pipeline (`cmd/server/reencode.go:95-99`).

---

### User Story 4 - Importing a multi-card file loses nothing (Priority: P2)

A user uploads a `.vcf` export containing many contacts, one of which has an embedded photo on a
single 200 KB line. Every card in the file is imported.

**Why this priority**: an import that silently drops everything after the first large card is
indistinguishable from a successful import until the user goes looking for a contact.

**Independent Test**: `POST /api/v1/import/vcard` with a file whose second card follows a very long
PHOTO line, then count the contacts.

**Acceptance Scenarios**:

1. **Given** a file with three cards where the first has a 200,000-character PHOTO line, **When** it
   is split, **Then** all three cards come back intact (`internal/vcard/split_long_test.go:12-34`).
2. **Given** a file using CRLF, LF, or no trailing newline, **When** it is split, **Then** one card
   is returned with unmangled line endings (`internal/vcard/split_long_test.go:36-60`).
3. **Given** a card with no UID, **When** it is imported, **Then** a UID is inserted before
   `END:VCARD` (`internal/vcard/split.go:59-73`, `internal/service/importer.go:62-67`).
4. **Given** a card that already has a UID after a very long line, **When** it is imported,
   **Then** no second UID is added (`internal/vcard/split_long_test.go:69-81`).

---

### User Story 5 - Merging duplicates picks individual values, not whole properties (Priority: P3)

Two duplicate contacts are shown side by side. The user keeps the work email from one and the home
email from the other, and the merged contact has both.

**Why this priority**: it is a refinement of an existing merge feature, not a data-integrity
guarantee. It sits here because the identity of a value — the thing the UI selects by — is defined
by this component.

**Independent Test**: `GET /api/v1/contacts/duplicates/:id` returns a `candidates` array of value
references; posting a selection of two EMAIL ids to the merge endpoint yields a card with both.

**Acceptance Scenarios**:

1. **Given** a duplicate pair, **When** the detail is fetched, **Then** every non-empty value of both
   cards is listed once, with `side` set to `winner`, `loser` or `both`
   (`internal/vcard/merge_cards.go:83-132`, `internal/handler/duplicate_handler.go:126-139`).
2. **Given** a selection naming both emails, **When** the merge runs, **Then** both survive with
   their TYPE parameters (`internal/vcard/merge_cards_test.go:52-79`).

Which value wins when a selection is silent, and which record's UID and VERSION survive, are
**owned by spec 007** (007 FR-032, FR-033). They are implemented in `internal/vcard/merge_cards.go`
— a file this spec owns — and asserted by tests listed under **Enforced By**, but this spec states
no requirement about them.

### Edge Cases

Boundary and malformed-input behaviours of the shipped build. Deliberate compromises and admissions
that used to sit here have been moved to **Known Divergences**, where the constitution requires
them.

- **A card missing `END:VCARD` is discarded by the splitter.** The buffer is only emitted when the
  terminator is seen, so a truncated upload silently yields one contact fewer
  (`internal/vcard/split.go:46-51`). No test covers this shape.
- **`StripPhoto` returns its input unchanged when the card cannot be decoded** — a merge-log
  snapshot with a photo still in it is better than no snapshot (`internal/vcard/encoder.go:170-173`).
- **A card without `VERSION` is refused, not repaired.** Encoding returns `ErrMissingVersion` rather
  than inventing a version (`internal/vcard/encoder.go:127-130`).
- **There is no undo for `reencode-vcards`.** The command says so before writing and tells the
  operator to take a database dump first (`cmd/server/reencode.go:72-78`).

## Requirements *(mandatory)*

### Functional Requirements

FR numbers are **stable identifiers**, not an ordering. Four requirements were withdrawn when
ownership was settled under constitution Principle VII; their numbers are retired rather than
reused, and each is replaced below by a labelled cross-reference. The surviving numbers did not move.

**Parsing**

- **FR-001**: The system MUST parse vCard 3.0 and 4.0 text into a single in-memory shape,
  `ParsedContact` (`internal/vcard/parser.go:14`, `internal/vcard/types.go:8`).
- **FR-002**: The system MUST extract the five `N` components into separate name fields, and when
  `N` yields neither a first nor a last name it MUST derive them from `FN` by splitting at the last
  space (`internal/vcard/parser.go:33-49`).
- **FR-003**: The system MUST keep every `EMAIL`, `TEL`, `ADR`, `URL`, `IMPP` and `CATEGORIES` value,
  not only the first, and MUST record each value's TYPE, PREF and LABEL
  (`internal/vcard/parser.go:57-101,152-159,174-181`).
- **FR-004**: The system MUST expose a primary email and phone for display, taken from the preferred
  value where one is marked and otherwise from the first (`internal/vcard/parser.go:58-75`).
- **FR-005**: The system MUST read preference from `PREF=n` (vCard 4.0) and from the `TYPE=pref`
  pseudo-type (vCard 3.0), and MUST NOT report `pref` as a type
  (`internal/vcard/parser.go:186-211`).
- **FR-006**: The system MUST split `ORG` at the first semicolon into organisation and department,
  and MUST map `BDAY` and `ANNIVERSARY` into a typed date list
  (`internal/vcard/parser.go:104-111,161-168`).

**Building**

- **FR-007**: The system MUST build vCard 4.0 output from `ParsedContact`
  (`internal/vcard/builder.go:43`).
- **FR-008**: The system MUST always emit an `FN`, deriving it from the name components, then the
  organisation, then the UID, because RFC 6350 requires it (`internal/vcard/builder.go:63-90`).
- **FR-009**: The system MUST mark the first email and the first phone as preferred when the caller
  marked none (`internal/vcard/builder.go:107,123`).

**Encoding — the single writer**

- **FR-010**: All vCard serialisation MUST go through `EncodeCard`; `gvcard.NewEncoder` MUST NOT be
  called outside a parity test (`internal/vcard/encoder.go:122`, `internal/vcard/builder.go:15-34`;
  the only remaining call is the byte-for-byte comparison at `internal/vcard/encoder_test.go:163`).
- **FR-011**: The system MUST choose escaping by the property's value type: URI-valued properties
  escaped not at all, list-valued properties escaped per item with the comma left as a separator,
  structured properties escaped per component with both separators left intact, and everything else
  escaped as TEXT (`internal/vcard/encoder.go:44-114`).
- **FR-012**: The system MUST normalise CRLF and bare CR to LF before escaping so no carriage return
  reaches the output except as a line terminator, and MUST drop newlines from URI values rather than
  emit an unparseable line (`internal/vcard/encoder.go:73-91`).
- **FR-013**: The emitted layout MUST match `go-vcard`'s byte for byte for values that need no
  correction — BEGIN first, VERSION second, remaining properties in sorted order, parameters sorted,
  CRLF endings — so that fixing the escaping does not move every stored ETag
  (`internal/vcard/encoder.go:116-154`, `internal/vcard/encoder_test.go:141-167`).
- **FR-014**: Encoding a card without `VERSION` MUST fail with a distinct error rather than emit an
  invalid card (`internal/vcard/encoder.go:127-130`, `internal/vcard/encoder_test.go:224`).
- **FR-015**: Encoding MUST be a fixed point across a decode/encode round trip, so escaping cannot
  accumulate over sync cycles (`internal/vcard/encoder_test.go:207-222`).
- **FR-016**: Parameter values MUST keep the previous escaping treatment, which nothing was
  mis-serialising (`internal/vcard/encoder.go:215-218`, `internal/vcard/encoder_test.go:234`).
- **FR-017**: The system MUST be able to strip embedded `PHOTO`, `LOGO` and `SOUND` payloads from a
  card, replacing them with a marker, for snapshots kept in the merge log
  (`internal/vcard/encoder.go:159-193`, used at `internal/service/merge_service.go:239`).

**Splitting and identity**

- **FR-018**: The system MUST split a multi-card file on `BEGIN:VCARD`/`END:VCARD` accepting CRLF,
  LF or CR line endings, without a line-length limit — an embedded photo exceeds
  `bufio.Scanner`'s 64 KiB default and everything after it used to vanish
  (`internal/vcard/split.go:7-55`).
- **FR-019**: The system MUST insert a `UID` before `END:VCARD` when a card has none, and MUST leave
  an existing UID alone (`internal/vcard/split.go:59-73`).

**Mapping to the contact record**

- **FR-020**: The system MUST copy the scalar fields of a parsed card onto the contact row without
  touching child relations, the raw card, or the ETag (`internal/vcard/domain_helper.go:10-41`).
- **FR-021**: The system MUST convert the multi-value fields of a parsed card into rows for the
  seven child tables — emails, phones, addresses, URLs, IMs, categories, dates — each with a fresh
  identifier (`internal/vcard/domain_helper.go:44-169`).

**Merging an edit into an existing card**

- **FR-022**: When applying edited fields to an existing card, the system MUST replace the
  properties it models wholesale and carry every other property over untouched
  (`internal/vcard/merge.go:13-61`).
- **FR-023**: The existing card's `UID` MUST win over the caller's (`internal/vcard/merge.go:63-66`).
- **FR-024**: When the existing card is empty or cannot be decoded, the system MUST build a fresh
  card rather than fail the edit (`internal/vcard/merge.go:44-52`).
- **FR-025**: The system MUST be able to report which properties of a card are unmodelled, for tests
  and diagnostics (`internal/vcard/merge.go:73-88`).

**Per-value merge of two cards**

- **FR-026**: The system MUST identify a single value of a single property by a content-derived
  identifier covering the property name, the value and its parameters — never by position, because a
  position means something different the moment either card changes
  (`internal/vcard/merge_cards.go:56-77`).
- **FR-027**: The system MUST list the candidate values of both cards with identical values collapsed
  to one entry marked as present on both sides, in a stable order
  (`internal/vcard/merge_cards.go:83-132`).
- **FR-028** *(withdrawn — see 007 FR-033)*: that a property absent from a selection keeps the
  winner's values is a which-value-wins rule, owned by
  `specs/007-duplicate-detection-and-merge/spec.md`. This spec owns how a value becomes text and
  how a value is identified (FR-026, FR-027); it does not state which value survives.
- **FR-029** *(withdrawn — see 007 FR-032)*: that `UID` and `VERSION` always come from the winner is
  a which-record-survives rule, owned by `specs/007-duplicate-detection-and-merge/spec.md`, which
  also states why it exists (every synced device knows the contact by that UID). The encoder's own
  invariant — a card without VERSION fails to encode — is unaffected and remains FR-014.
- **FR-030** *(withdrawn — see 007 FR-021)*: that value identifiers are minted on the server and
  returned with the duplicate detail is a property of the duplicate-detail endpoint's contract with
  its client, implemented in `internal/handler/duplicate_handler.go` and owned by
  `specs/007-duplicate-detection-and-merge/spec.md`. What the identifier *is* remains FR-026 here,
  because that definition is what makes a second, client-side implementation impossible to keep in
  step.

**The `reencode-vcards` command**

- **FR-031**: The command MUST default to a dry run that writes nothing and reports how many cards
  would change (`cmd/server/reencode.go:32,89-92`).
- **FR-032**: `--apply` MUST be refused without `--reconcile-sync-state`, with an explanation and
  exit code 2 (`cmd/server/reencode.go:54-61`).
- **FR-033**: Before writing, the command MUST state that there is no undo and that every CardDAV
  client will re-download the address book (`cmd/server/reencode.go:71-79`).
- **FR-034**: Rows MUST be read and written in bounded batches (500) rather than in one statement,
  because a bulk write inside a transaction would hold SQLite's single connection long enough for
  the container health check to restart the process (`cmd/server/reencode.go:19-28,126-172`).
- **FR-035**: A card the decoder cannot read MUST be counted and left untouched
  (`cmd/server/reencode.go:144-148`).
- **FR-036**: A rewritten contact's ETag MUST be recomputed with the same function the ordinary write
  paths use (`cmd/server/reencode.go:164` calling `service.ContactETag`,
  `internal/service/contact.go:344-351`).
- **FR-037**: Reconciliation MUST re-encode each stored merge anchor, recompute its content hash from
  the re-encoded anchor with the sync engine's own function, and point `local_etag` at what the
  contact now hashes to (`cmd/server/reencode.go:185-220`, `internal/sync/engine.go:660-666`).
- **FR-038**: Reconciliation MUST leave `remote_etag` alone — it is the remote server's opaque value
  and nothing local touched the remote side (`cmd/server/reencode.go:180-184`,
  `cmd/server/reencode_test.go:150-151`).
- **FR-039**: A second run over already-repaired data MUST report nothing to do
  (`cmd/server/reencode.go:107-119`, `.github/workflows/ci.yml:286-288`).
- **FR-040**: If the rewrite succeeds but reconciliation fails, the command MUST exit non-zero and
  warn against running a pipeline (`cmd/server/reencode.go:94-99`).
- **FR-041** *(withdrawn — see 008 FR-062)*: that a subcommand must not run migrations and must
  refuse to operate on a schemaless database, exiting 5, is the CLI **dispatch contract**, owned by
  `specs/008-runtime-configuration-and-delivery/spec.md` and implemented in `cmd/server/cli.go:109-138`
  — a file this spec does not own. `reencode-vcards` is a caller of that contract and inherits it
  unchanged: FR-031..FR-040 state only what the command does to stored cards.

### Key Entities

- **Raw card (`contacts.vcard_data`)**: the system of record. Everything else about a contact is
  derived from it (`internal/service/contact.go:100-118`).
- **`ParsedContact`**: the one in-memory shape every layer trades in — name parts, primary email and
  phone, multi-value lists, organisation, and a handful of scalars. It models a fixed property set;
  anything outside it has no home here (`internal/vcard/types.go:8-47`).
- **`Field` / `Address` / `Date`**: one value of a multi-value property, with type, preference and
  label; a seven-component postal address; a typed date (`internal/vcard/types.go:51-77`).
- **`gvcard.Card`**: the property map handed to `EncodeCard`; the only thing the writer accepts
  (`internal/vcard/encoder.go:122`).
- **`ValueID` / `ValueRef` / `Selection`**: the identity of one value, the candidate offered to the
  merge UI, and the caller's choice per property (`internal/vcard/merge_cards.go:36-52`).
- **Derived values**: the contact ETag (`sha256` of the card text, first 8 bytes, hex —
  `internal/service/contact.go:348`) and the sync content hash (full `sha256`, hex —
  `internal/sync/engine.go:663`). Both are functions of the encoder's output, which is why the
  encoder's byte-level stability is load-bearing.
- **Merge anchor (`sync_states.base_vcard`, `local_etag`, `content_hash`)**: the three columns the
  repair command must move in step with a card rewrite (`cmd/server/reencode.go:185-220`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A contact created with two categories exports with both of them, and a card imported
  with the old escaping exports with both after repair — asserted end to end against a running
  container on every CI run.
- **SC-002**: A photo URI survives storage and export unescaped, so photos render on iOS clients:
  an exported body contains `PHOTO:data:image/jpeg;base64,/9j/4AAQ` with no backslash.
- **SC-003**: Zero ETag churn from the encoder change for cards that contained no mishandled value:
  output is byte-identical to the previous writer's.
- **SC-004**: Encoding is a fixed point — a second pass over decoded output produces identical bytes,
  so no card gains a backslash per sync cycle.
- **SC-005**: 100% of unmodelled properties on a Google- or iOS-originated card survive a form edit
  — six of six in the fixture: PHOTO, X-ABLabel, X-SOCIALPROFILE, KEY, PRODID, REV.
- **SC-006**: A multi-card file whose first card carries a 200,000-character photo line yields all
  three of its cards, not one.
- **SC-007**: `--apply` without `--reconcile-sync-state` exits with code 2 in 100% of invocations,
  in a real container and not only in a unit test.
- **SC-008**: After a repair run, a second run reports `0 of N` cards needing a rewrite.
- **SC-009**: After a repair run, every sync state's `local_etag` equals its contact's current ETag
  and its `content_hash` agrees with its re-encoded anchor, so the number of contacts the next
  export sees as locally modified is zero.
- **SC-010**: A card the decoder cannot read is byte-identical before and after an applied repair
  run — the repair command never loses data.

## Out of Scope

- The CardDAV wire protocol — reports, CTag, sync-collection, path layout (spec 004).
- CSV column mapping on import and export (spec 005).
- Which contact wins when two are merged, which value wins when a selection is silent, and how
  duplicates are detected (spec 007). This spec owns only the encode/decode contract that merge
  relies on, plus the per-value identity that makes a per-field choice expressible.
- The CLI dispatch contract — argument whitelisting, exit codes, the refusal to run migrations
  (spec 008). This spec owns only what `reencode-vcards` does to stored cards.
- Photo storage, resizing, or fetching: an embedded photo is carried as an opaque value.

## Assumptions

- The application reads what it writes with `go-vcard`'s decoder. Every escaping decision here is
  constrained by that decoder's behaviour, not by RFC 6350 alone — which is the whole reason the
  semicolon gap exists (`internal/vcard/encoder.go:32-42`).
- `github.com/emersion/go-vcard` supplies the decoder, the card data structure, and the layout this
  encoder reproduces byte for byte. The encoder exists only because that library's *writer* escapes
  every value identically.
- Only vCard 3.0 and 4.0 are claimed (`internal/vcard/types.go:1-3`). Behaviour on a vCard 2.1 file
  is whatever the underlying decoder does; no code in this repository handles the 2.1
  quoted-printable or `ENCODING=b` forms. [NEEDS CLARIFICATION: is vCard 2.1 input intended to be
  supported, rejected, or simply undefined?]
- Card size is bounded elsewhere, not here: `carddav.max_resource_bytes` bounds a single card on the
  CardDAV path (`internal/config/config.go:131-134`) and `server.max_body_bytes` bounds an upload.
  Nothing in this package limits the size of a card it will encode or split.
- A single instance owns the database while `reencode-vcards` runs. The command takes no lease and
  the surrounding documentation tells the operator to stop pipelines first
  (`cmd/server/reencode.go:71-78`).

## Status

Shipped at **v0.4.0** (`23a167c`, `CHANGELOG.md:9`). Every requirement stated above is true of that
build. The gaps are in *enforcement*, not in behaviour — see Known Divergences for the three
requirement groups with no automated test and the one invariant nothing checks mechanically.

## Code Paths

Owned by this spec:

- `internal/vcard/types.go`
- `internal/vcard/parser.go`
- `internal/vcard/parser_test.go`
- `internal/vcard/builder.go`
- `internal/vcard/encoder.go`
- `internal/vcard/encoder_test.go`
- `internal/vcard/terminate.go`
- `internal/vcard/terminate_test.go`
- `internal/vcard/split.go`
- `internal/vcard/split_truncated_test.go`
- `internal/vcard/split_long_test.go`
- `internal/vcard/domain_helper.go`
- `internal/vcard/merge.go`
- `internal/vcard/merge_test.go`
- `internal/vcard/merge_cards.go`
- `internal/vcard/merge_cards_test.go`
- `cmd/server/reencode.go`
- `cmd/server/reencode_test.go`

## References

Touched but **not** owned. Each is cited above as evidence, never claimed:

- `internal/service/contact.go` — the create/update write paths that call `MergeIntoVCard`,
  `BuildVCard` and `ContactETag` (spec 002).
- `internal/sync/engine.go` — `ContentHash`, the second value derived from this encoder's output
  (spec 006).
- `internal/handler/duplicate_handler.go` — mints and returns value identifiers (spec 007).
- `internal/service/merge_service.go` — the only caller of `StripPhoto` (spec 007).
- `internal/carddav/backend.go` — re-encodes cards arriving over CardDAV (spec 004).
- `internal/sync/carddav_client.go` — re-encodes cards fetched from a remote CardDAV server
  (spec 006).
- `internal/sync/internal_provider.go` — hashes stored card text (spec 006).
- `internal/service/importer.go` — splits an uploaded file and injects UIDs (spec 005).
- `cmd/server/cli.go` — the dispatch contract `reencode-vcards` is reached through (spec 008).

Provenance: `CHANGELOG.md:98-104` (the defect this domain was created to fix) and
`.github/workflows/ci.yml:228-300` (the end-to-end assertions).

## Enforced By

**Parsing (FR-001..FR-006)** — `internal/vcard/parser_test.go`:
`TestParseVCard_V3BasicFields`, `TestParseVCard_MultipleEmails`, `TestParseVCard_MultiplePhones`,
`TestParseVCard_Dates`, `TestParseVCard_V4Categories`, `TestParseVCard_V4Address`,
`TestParseVCard_FNFallback`, `TestNewFromSimple`.

**Building (FR-007, FR-008)** — `internal/vcard/parser_test.go`: `TestBuildVCard_RoundTrip`,
`TestBuildVCard_DeriveFN`.

**Encoding (FR-011..FR-016)** — `internal/vcard/encoder_test.go`:
`TestEncodeCard_EscapingIsValueTypeAware` (FR-011),
`TestEncodeCard_NewlinesAreNormalisedAndEscaped` and `TestEncodeCard_NewlineInAURIIsDropped` (FR-012),
`TestEncodeCard_LayoutMatchesGoVCardForUnaffectedValues` (FR-013),
`TestEncodeCard_MissingVersionIsAnError` (FR-014),
`TestEncodeCard_RoundTripsThroughTheDecoder` and `TestEncodeCard_IsIdempotentAcrossRoundTrips`
(FR-015), `TestEncodeCard_ParametersAreUnchanged` (FR-016).

**Splitting and identity (FR-018, FR-019)** — `internal/vcard/split_long_test.go`:
`TestSplitVCards_LongPhotoLineDoesNotTruncate`, `TestSplitVCards_LineEndings`,
`TestSplitVCards_Empty`, `TestInjectUID_FindsUIDAfterLongLine`, `TestInjectUID_AddsWhenMissing`;
and `internal/vcard/parser_test.go`: `TestSplitVCards`, `TestSplitVCards_Empty`,
`TestInjectUID_AddsMissing`, `TestInjectUID_PreservesExisting`. (The two `TestSplitVCards_Empty`
functions are distinct: one is in package `vcard`, the other in package `vcard_test`.)

**Merging an edit (FR-022..FR-025)** — `internal/vcard/merge_test.go`:
`TestMergeIntoVCard_PreservesUnmodelledProperties`, `TestMergeIntoVCard_ReplacesManagedProperties`,
`TestMergeIntoVCard_ClearedFieldsAreRemoved`, `TestMergeIntoVCard_KeepsExistingUID` (FR-023),
`TestMergeIntoVCard_EmptyExistingBuildsFresh` and `TestMergeIntoVCard_UnparseableExistingBuildsFresh`
(FR-024), `TestMergeIntoVCard_RoundTripIsStable`, `TestUnmanagedProperties` (FR-025).

**Per-value identity (FR-026, FR-027)** — `internal/vcard/merge_cards_test.go`:
`TestMergeCards_ValueIDsAreContentBasedNotPositional` (FR-026),
`TestCandidates_ExcludesIdentityProperties`, `TestCandidates_MarksWhichSideEachValueCameFrom` and
`TestMergeCards_IdenticalValueOnBothSidesAppearsOnce` (FR-027),
`TestMergeCards_KeepsValuesFromBothSides`, `TestMergeCards_PreservesParametersOfKeptValues`,
`TestMergeCards_RejectsUnreadableInput`, `TestMergeCards_OutputIsItselfAValidInput`. Two further
tests in this file — `TestMergeCards_IdentityAlwaysComesFromTheWinner` and
`TestMergeCards_EmptySelectionYieldsTheWinner`, plus
`TestMergeCards_ExplicitEmptyListDropsTheProperty` — assert rules **owned by spec 007**
(FR-032, FR-033); they live here because the code does, not because this spec claims them.

**The repair command (FR-031..FR-040)** — `cmd/server/reencode_test.go`:
`TestReencodeContacts_DryRunWritesNothing` (FR-031),
`TestRunReencodeVCards_ApplyRequiresReconcile` (FR-032),
`TestReencodeContacts_LeavesUndecodableCardsAlone` (FR-035),
`TestReencodeContacts_ApplyRewritesCardAndETag` (FR-036),
`TestReconcileSyncStates_PointsLocalETagAtTheRewrittenCard` (FR-037, FR-038),
`TestReencodeVCard_FixesLegacyEscapingAndIsIdempotent` (FR-039).

**End to end, against a live container** — `.github/workflows/ci.yml`:
the step *"vCard list separators survive a round trip"* (`:231-251`) covers SC-001 and the created
contact's categories; the step *"reencode-vcards is a dry run by default and refuses a half job"*
(`:275-300`) covers SC-001's repaired half, SC-002 (`:299-300`), SC-007 (`:280-284`) and
SC-008 (`:286-288`).

**Success criteria to unit tests**: SC-003 →
`TestEncodeCard_LayoutMatchesGoVCardForUnaffectedValues`; SC-004 →
`TestEncodeCard_IsIdempotentAcrossRoundTrips`; SC-005 →
`TestMergeIntoVCard_PreservesUnmodelledProperties`; SC-006 →
`TestSplitVCards_LongPhotoLineDoesNotTruncate`; SC-009 →
`TestReconcileSyncStates_PointsLocalETagAtTheRewrittenCard`; SC-010 →
`TestReencodeContacts_LeavesUndecodableCardsAlone`.

## Known Divergences

**A card left open at EOF is returned, not dropped.** `SplitVCards` emitted a card only on
`END:VCARD`, so a truncated file yielded one contact fewer with no error — including on a backup
restore, which reaches the same splitter *after* replace mode has deleted the originals. The
pending buffer is now returned so the caller parses it, fails, and counts the failure. Covered by
`TestSplitVCards_KeepsATruncatedFinalCard`.


**Import and restore used to strip the card terminator, and export glued the cards together.**
`strings.TrimSpace` on each card before storing removed the trailing CRLF, so concatenating
stored cards produced `END:VCARDBEGIN:VCARD` on one physical line — and `SplitVCards`, which
tests for a line *starting* with `BEGIN:VCARD`, kept only the first card. An address book
populated by import therefore exported to a file that re-imported as **0 of 3** contacts, and a
`replace` restore of such a backup deleted everything and inserted one card. Cards written by
`EncodeCard` were always terminated correctly, which is why every existing fixture passed and no
test caught it. `vcard.Terminated` is now applied on both write paths (export, backup) and both
store paths (import, restore); applying it on write repairs databases that already hold trimmed
cards, with no data migration.


1. **A `;` inside a single-valued TEXT property is not escaped**, although RFC 6350 §3.4 requires
   it. `go-vcard`'s decoder unescapes only `\\`, `\n` and `\,`; emitting `NOTE:a\; b` would make
   this application read its own note back as the literal `a\; b`, and undoing it after decoding is
   ambiguous. Fixing it properly means owning the decode path
   (`internal/vcard/encoder.go:32-42`, `internal/vcard/encoder_test.go:169-171`).

2. **"One writer per representation" is true of the writer, not of everything derived from it.**
   Constitution Principle V says all encoding goes through `internal/vcard`; it does. But two
   values computed *from* the encoder's output live outside this package by design —
   `ContentHash` (`internal/sync/engine.go:663`) and `ContactETag`
   (`internal/service/contact.go:348`) — and beyond those two exported functions at least six
   further private copies of the same two formulas exist:
   `internal/carddav/backend.go:287-288`, `internal/sync/internal_provider.go:47,73,93`,
   `internal/sync/carddav_client.go:242,391`, `internal/service/sync_conflict.go:163,168`.
   Nothing keeps them in step; an encoder change that moved hashes would have to be traced through
   all eight sites by hand.

3. **FR-010 has no mechanical enforcer.** That `gvcard.NewEncoder` appears nowhere but the parity
   test is true today by inspection (`grep -rn NewEncoder --include='*.go'` returns exactly
   `internal/vcard/encoder_test.go:163`), and no lint rule, CI step or test asserts it. A second
   call site would be caught only in review.

4. **Updating a contact with flat fields instead of `fields` rebuilds the card from the modelled
   properties and therefore drops unmodelled ones.** Only the `fields` path goes through
   `MergeIntoVCard`; the flat-field path ends in `BuildVCard`
   (`internal/service/contact.go:172-219`). User Story 2's guarantee holds for the web form, which
   sends `fields` — not for a caller that PUTs `{"first_name": "..."}`.

5. **A card supplied wholesale is stored verbatim, not re-encoded.** `POST /api/v1/import/vcard`
   stores the file's own card text (`internal/service/importer.go:76,91`), and
   `PUT /contacts/:id` with `vcard_data` stores the caller's string
   (`internal/service/contact.go:170-171`). Cards arriving over CardDAV or from a remote CardDAV
   server *are* re-encoded (`internal/carddav/backend.go:274`,
   `internal/sync/carddav_client.go:390`). So legacy escaping can still enter the database through
   import, and `reencode-vcards` remains a live remedy rather than a one-off migration.

6. **The encoder does not fold long lines.** `EncodeCard` writes each property on one physical line
   regardless of length (`internal/vcard/encoder.go:141-150`); a card with an embedded photo has one
   very long line, which RFC 6350 §3.2 does not require but every other producer does. Folded
   *input* is accepted, because the decoder unfolds it.

7. **`InjectUID` matches only a line beginning `UID:`** (`internal/vcard/split.go:61`). A card whose
   UID carries a parameter (`UID;VALUE=uri:…`) would not be recognised and a second UID line would
   be added. No test covers this shape.

8. **The parsed shape is lossy, on purpose, and three losses are worth naming.** A `data:` PHOTO is
   not copied into `ParsedContact.PhotoURI`, though it stays on the card
   (`internal/vcard/parser.go:139-146`); `NICKNAME` keeps only its first comma-separated value
   (`internal/vcard/parser.go:52-55`) even though the encoder treats NICKNAME as a list on write
   (`internal/vcard/encoder.go:53`); and unknown and `X-` properties are not represented in
   `ParsedContact` at all (`internal/vcard/parser.go:10-13`). The third is precisely why
   preservation depends on `MergeIntoVCard` rather than on the parsed shape.

9. **FR-020 and FR-021 have no tests at all.** There is no `internal/vcard/domain_helper_test.go`,
   and no test anywhere calls `ApplyToContact` or `ChildRecordsFor`. The mapping from a parsed card
   to the contact row and its seven child tables is exercised only indirectly, through service-level
   tests owned by other specs. This is a gap, not a review-only decision.

10. **FR-005 and FR-009 are enforced only indirectly.** No test distinguishes a preferred value from
    the first value: `TestParseVCard_MultipleEmails` asserts `PrimaryEmail == Emails[0].Value`
    (`internal/vcard/parser_test.go:97-103`), which passes whether or not the `PREF` handling works.
    Nothing asserts that `BuildVCard` writes `PREF=1` on the first email and phone
    (`internal/vcard/builder.go:107,123`).

11. **`reencode-vcards` deliberately does not bump `change_seq`**, so CardDAV clients learn about the
    rewrite by ETag alone and not also through the sync-collection report
    (`cmd/server/reencode.go:158-160`). A client that syncs exclusively by `sync-collection` will not
    see the repaired cards until something else touches them.

12. **The splitter's discard of an unterminated card is silent and untested.** It is listed under
    Edge Cases as behaviour; it is repeated here because "one contact fewer, no error" is the shape
    of a data loss nobody notices (`internal/vcard/split.go:46-51`).

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-11 | unreleased | A truncated final card is returned rather than silently discarded, so an import or restore reports the failure instead of losing a contact. | — |
| 2026-08-10 | unreleased | **D-term** — recorded the terminator loss on import/restore and the unseparated concatenation on export/backup, which silently reduced a multi-card address book to one contact on any round trip. Fixed at all four sites via `vcard.Terminated`; regression covered by `TestTerminated_ConcatenatedCardsStillSplit`. Found by the divergence triage, admitted by no spec. | — |
| 2026-08-07 | v0.4.0 | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | v0.4.0 | Conformed to the house template. Withdrew FR-028, FR-029 and FR-030 to spec 007 and FR-041 to spec 008 under constitution Principle VII; numbers retired, not reused. Moved eight admissions out of Edge Cases into Known Divergences and added four more (unenforced FR-010, untested FR-020/FR-021, indirectly enforced FR-005/FR-009, untested splitter discard). | — |
| 2026-08-07 | unreleased | Citations only; no requirement changed. Three line references into files this spec does not own were re-anchored after D1 and D3 moved them: the `sha256`/`hex` copy of the ETag formula in `internal/carddav/backend.go` (`:274` → `:287-288`, moved by the 413 check), the `cardToString` re-encode on the same PUT path (`:273` → `:274`), and `openCLIDatabase` in `cmd/server/cli.go` (`:108-136` → `:109-138`, which now also returns the loaded config). Specs 001, 004 and 008 were re-anchored by those changes; this one was missed. | D1, D3 |
