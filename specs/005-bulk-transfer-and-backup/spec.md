# Feature Specification: Bulk Transfer, Backup & Restore

Kind: journey
Status: shipped
Constitution: v1.0.0

Reconstructed from the implementation at `23a167c` (three commits after `v0.4.0`). Every
requirement below was read out of the code at the cited path before it was written down. Where
the implementation has a deliberate limitation or a genuine gap it is recorded under
**Known Divergences** rather than dressed up as a requirement that is met.

Scope: every path by which a bulk of contacts enters or leaves as a file — vCard and CSV import,
vCard/CSV/JSON export, scheduled and manual backup, restore, and the history that answers whether
backups are working. What lies outside this spec, and who owns it, is listed under **References**.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Backups happen without being asked for (Priority: P1)

A user turns on scheduled backups, picks a cadence and how many copies to keep, and optionally
asks for them to be compressed. From then on the server writes a dated file containing every
contact in their address book, prunes the oldest copies past the retention count, and records
what it did. The same thing can be triggered by hand from the Backup screen.

The settings row is per user (`internal/domain/user_backup_settings.go`,
`migrations/008_user_backup_settings.up.sql`) and is edited at
`web/src/views/backup/BackupView.vue:43-68` through `web/src/api/backup.ts:24-31`. Saving it
re-registers the job in the running scheduler (`internal/handler/backup_handler.go:166-194`,
`internal/worker/scheduler.go:143-158`). The scheduled firing runs
`internal/worker/jobs/backup_job.go:29-45`; the manual button posts to
`internal/handler/backup_handler.go:98-109`. Both land in `BackupService.CreateWithTrigger`
(`internal/service/backup.go:117-179`), which writes the file
(`internal/service/backup.go:181-303`) and then applies retention
(`internal/service/backup.go:575-591`).

**Why this priority**: Nothing else in this domain has any value without it. Restore has nothing
to restore from, and the health display has nothing to report on.

**Independent Test**: Enable a schedule, press "Create backup", and confirm a file appears in the
list with a plausible size; set retention to 1 and create a second backup, and confirm only the
newest file survives. `internal/service/backup_integrity_test.go:56` drives the create-and-list
half against a real temporary directory.

**Acceptance Scenarios**:

1. **Given** a user with contacts and `compress` off, **When** they `POST /api/v1/backup/create`,
   **Then** a `backup-<timestamp>.vcf` file is written under that user's backup directory and the
   response carries its filename, size and creation time.
2. **Given** `compress` is on, **When** a backup runs, **Then** the file is named `.vcf.gz` and
   its contents are gzip-encoded.
3. **Given** retention is 3 and four backups exist, **When** a fifth is created, **Then** the
   oldest files are deleted so three remain, newest first.
4. **Given** the backup file cannot be finished, **When** the write fails, **Then** no partial
   file is visible to the list or download endpoints.
5. **Given** a user saves a new schedule, **When** the settings are stored, **Then** the running
   scheduler picks up the change immediately — no restart is needed.

---

### User Story 2 - Put an address book back the way it was (Priority: P2)

Something went wrong — a bad import, a sync that deleted the wrong things, a person who cleared
their phone. The user opens the Backup screen, picks a file, and chooses whether to *merge* it
into what is there now or to *replace* everything with it. Replace is guarded: they have to type
the word "replace" before the button becomes usable
(`web/src/views/backup/BackupView.vue:161-177`, `:300-303`).

The request goes to `internal/handler/backup_handler.go:143-155` via
`web/src/api/backup.ts:20-22`, and the whole operation is
`BackupService.Restore` (`internal/service/backup.go:396-502`): read and parse everything first,
only then delete, then insert, then reconcile sync state
(`internal/service/backup.go:504-532`).

**Why this priority**: This is the payoff for User Story 1, and the operation with the worst
failure mode in the whole product. It is second only because it cannot be exercised until
backups exist.

**Independent Test**: Place a known backup file in a user's backup directory, restore it in each
mode, and check the resulting address book against the file. This is exactly how the service
tests drive it (`internal/service/backup_restore_test.go:20`).

**Acceptance Scenarios**:

1. **Given** a backup with two cards and an address book with one unrelated contact, **When**
   restore runs in `merge` mode, **Then** both cards are added, the existing contact is untouched,
   and the response reports 2 imported / 0 skipped.
2. **Given** a backup containing a card whose UID already exists locally, **When** restore runs in
   `merge` mode, **Then** that card is counted as skipped and the local contact is left alone.
3. **Given** an address book with contacts, **When** restore runs in `replace` mode with a valid
   backup, **Then** the previous contacts are gone and only the backup's contacts remain.
4. **Given** a backup file that is empty or contains no parseable card, **When** restore runs in
   `replace` mode, **Then** the request fails and **not one existing contact is deleted**.
5. **Given** a gzip backup that decompresses past the configured cap, **When** restore runs in
   `replace` mode, **Then** the request fails before any deletion and the address book survives
   intact.
6. **Given** a restore that has already deleted the address book, **When** the caller's request is
   cancelled, **Then** the inserts still run to completion.
7. **Given** contacts that were synced to a remote provider and are not present in the backup,
   **When** a `replace` restore drops them, **Then** their sync state rows are removed so the next
   export does not read them as local deletions and delete them on the remote too.

---

### User Story 3 - Know whether backups are actually working (Priority: P3)

The Backup screen opens with one line: healthy, failing, overdue, never run, or off. Below it is
a history of attempts — when, what triggered it, how many contacts, how big, and the error if it
failed. If the server was down when a scheduled backup was due, it notices at boot and queues one.

The verdict is computed client-side in `web/src/utils/backup-health.ts:33-100` from
`GET /api/v1/backup/status` and `GET /api/v1/backup/runs`
(`internal/handler/backup_handler.go:40-96`, `web/src/api/backup.ts:33-40`), which read the
`backup_runs` table (`internal/domain/backup_run.go`, `migrations/025_backup_runs.up.sql`,
`internal/repository/bun_backup_run.go:36-88`). The boot-time catch-up is
`cmd/server/startup.go:78-161`.

**Why this priority**: The failure this addresses is silent. A scheduled backup starts failing,
nothing surfaces it, and it is discovered on the day the backup is needed. It is P3 rather than
P1 because backups still happen without it — the user just cannot tell.

**Independent Test**: Kill the server mid-backup, restart it, and confirm the run shows as
`interrupted` rather than sitting at `running` forever; leave the server down past a scheduled
backup, restart, and confirm a run appears with trigger `catchup`. The catch-up half is driven
directly by `cmd/server/startup_test.go:210-277`.

**Acceptance Scenarios**:

1. **Given** a manual backup that succeeds, **When** it finishes, **Then** the history shows one
   `completed` row with trigger `manual`, the filename, byte size and contact count.
2. **Given** a backup that fails because the database is unavailable, **When** it returns,
   **Then** the history shows a `failed` row carrying the cause.
3. **Given** a backup running when the process is shut down gracefully, **When** the caller's
   context is already cancelled, **Then** the history row is still closed rather than left at
   `running`.
4. **Given** scheduled backups are on and the last success is older than one schedule period,
   **When** the server starts, **Then** exactly one backup is queued for that user, recorded with
   trigger `catchup`.
5. **Given** scheduled backups are off for a user, **When** the server starts, **Then** no
   catch-up is queued — a backup the user switched off was not "missed".
6. **Given** the last successful backup is older than two schedule periods, **When** the Backup
   screen loads, **Then** it reads `overdue` and is styled as alarming.

---

### User Story 4 - Bring a file of contacts in (Priority: P4)

A user has contacts somewhere else — an old phone export, a spreadsheet, another address book
service. They pick vCard or CSV on the Import screen
(`web/src/views/import-export/ImportView.vue:44-65`, `web/src/api/import-export.ts:4-18`),
choose the file, and it lands in their address book.

The routes are `internal/handler/import_handler.go:18-33` (vCard) and `:54-69` (CSV), registered
with a wider body limit at `internal/handler/handler.go:150-152`. The work is
`internal/service/importer.go:35-108` (vCard, matched on UID) and `:110-180` (CSV, one new
contact per row).

**Why this priority**: It is the ordinary way an address book gets populated in the first place,
but a failure here is recoverable — the user still has the source file.

**Independent Test**: Upload a `.vcf` with several cards and a `.csv` with a header row, and
confirm the contacts appear with names, emails, phones, organisation, title and note.
`internal/service/importer_test.go:21-75` does exactly this at the service boundary.

**Acceptance Scenarios**:

1. **Given** a multi-card `.vcf` file, **When** it is uploaded to `POST /api/v1/import/vcard`,
   **Then** each parseable card becomes a contact and the response reports how many were imported.
2. **Given** a vCard whose UID already exists in the address book, **When** it is imported,
   **Then** the existing contact is updated in place with the new card's content.
3. **Given** a vCard with no `UID` property, **When** it is imported, **Then** a UID is generated
   and written into the stored card.
4. **Given** a file where some cards are malformed, **When** it is imported, **Then** the good
   cards are still imported and the bad ones are counted as errors rather than aborting the run.
5. **Given** a CSV whose header uses `firstname`, `E-Mail`, `Company` and `notes`, **When** it is
   imported, **Then** those columns are recognised — matching is case-insensitive and each field
   has a set of accepted aliases.
6. **Given** an upload larger than the configured import limit, **When** it is posted, **Then** it
   is refused with `413` before any handler reads it.

---

### User Story 5 - Take a file of contacts out (Priority: P5)

A user wants their data as a file — the whole address book as vCard, CSV or JSON from the Export
screen (`web/src/views/import-export/ExportView.vue:24-45`,
`web/src/api/import-export.ts:20-30`), or just the contacts they ticked in the list view as a
single vCard download (`web/src/api/contacts.ts:58-64`).

Server side that is `internal/handler/export_handler.go:21-75` over
`internal/service/exporter.go:26-135`.

**Why this priority**: Valuable and frequently used, but purely read-only: nothing here can lose
or corrupt anything.

**Independent Test**: Export each format and confirm the downloaded file parses and contains the
expected number of contacts; select a handful in the list and confirm the selective vCard
download contains exactly those that belong to the caller
(`internal/service/contact_bulk_test.go:84-126`, `internal/service/exporter_test.go:21-70`).

**Acceptance Scenarios**:

1. **Given** an address book with contacts, **When** `GET /api/v1/export/vcard` is called with no
   parameters, **Then** every contact's stored vCard is returned as one downloadable `.vcf`.
2. **Given** a selection in the list view, **When** the user exports the selection, **Then** the
   request names those ids and the file contains exactly those contacts — repeated ids do not
   duplicate a contact, and ids belonging to another user's address book return nothing.
3. **Given** more than 500 ids in one request, **When** it is sent, **Then** it is refused with a
   `400` naming the limit.
4. **Given** any of the three export endpoints, **When** it responds, **Then** it carries a
   content type and an attachment filename so a browser downloads rather than renders it.

---

### Edge Cases

Boundary conditions the code handles deliberately. Gaps and unmet intentions are **not** here —
they are in **Known Divergences**.

- *Cancellation is deliberately not honoured after the destructive step of a restore.* Restore
  checks `ctx.Err()` while parsing and never again. Between `DeleteAll` and the inserts a
  cancellation would leave an empty address book, so "more cancellable" is refused on purpose
  (`internal/service/backup.go:446-452`, `:466-486`;
  `internal/service/cancellation_test.go:63`). Stated as FR-049.
- *The empty-backup guard applies to `replace` only.* A merge restore from a file whose every card
  is unparseable returns `200` with `imported: 0, errors: N` — nothing is deleted, so there is
  nothing to guard (`internal/service/backup.go:457`).
- *An oversized backup is an error, never a truncation.* The reader takes one byte past the cap so
  a file that decompresses too far is rejected rather than silently cut short — a truncated read
  in `replace` mode would destroy contacts the file could no longer supply
  (`internal/service/backup.go:564`).
- *A backup filename carries millisecond precision* so two backups taken in the same second cannot
  collide (`internal/service/backup.go:206-213`).
- *A backup is written under a temporary name and renamed into place* so a failed write leaves no
  partial file and no orphan behind (`internal/service/backup.go:251-302`).
- *A retention failure does not invalidate the backup just written.* It is logged; the new file
  stands (`internal/service/backup.go:234-241`).
- *Unloadable user settings do not fail a backup.* It proceeds on instance defaults
  (`internal/service/backup.go:187-194`).
- *History recording is best-effort.* If the run row cannot be opened or closed, the failure is
  logged and the backup still proceeds and still returns its result
  (`internal/service/backup.go:139`, `:175`).
- *`backup_runs` is never pruned.* This is deliberate — roughly one row per user per day — and is
  contrasted in config with `sync_runs`, which is pruned because it grows per pipeline execution
  (`internal/config/config.go:156`).

## Requirements *(mandatory)*

### Functional Requirements

**Import**

- **FR-001**: The system MUST accept a vCard import either as a multipart `file` field or as the
  raw request body, and MUST refuse a request that supplies neither with `400`
  (`internal/handler/import_handler.go:18-33`).
- **FR-002**: vCard import MUST split the payload into cards and, for each, look up the caller's
  address book by UID: an existing contact is updated in place with the new card, a new UID
  becomes a new contact. Both outcomes count as `imported`
  (`internal/service/importer.go:69-104`).
- **FR-003**: A card with no UID MUST have one generated and injected into the stored card text so
  the contact is addressable afterwards (`internal/service/importer.go:62-67`).
- **FR-004**: A card that fails to parse MUST increment an error count and MUST NOT abort the
  import (`internal/service/importer.go:56-60`).
- **FR-005**: Import MUST stop on a cancelled context, and MUST do so only at a point where
  nothing has been written (`internal/service/importer.go:47-49`,
  `internal/service/cancellation_test.go:29-58`).
- **FR-006**: CSV import MUST read the first row as a header, lower-case and trim each column
  name, and resolve seven fields through alias sets — first name (`first_name`, `firstname`,
  `first name`), last name (`last_name`, `lastname`, `last name`), email (`email`, `e-mail`),
  phone (`phone`, `telephone`, `tel`), organisation (`org`, `organization`, `company`), title
  (`title`, `job_title`) and note (`note`, `notes`, `description`)
  (`internal/service/importer.go:125-150`, `internal/service/importer.go:182-189`).
- **FR-007**: CSV import MUST build a vCard for each row through the shared vCard builder and
  store it as a new contact with a newly minted UID (`internal/service/importer.go:152-176`).
- **FR-008**: A row the CSV reader cannot decode MUST be counted as an error and skipped, not
  fatal to the import (`internal/service/importer.go:139-142`).
- **FR-009**: The import routes MUST be allowed a larger request body than the rest of the API,
  resolved by the single per-path body-limit middleware rather than by nesting limits
  (`cmd/server/main.go:272-275`, `internal/handler/handler.go:75-80`,
  `internal/handler/handler.go:148-152`).

**Export**

- **FR-010**: `GET /export/vcard` MUST return the stored vCard text of every contact in the
  caller's address book concatenated into one document
  (`internal/service/exporter.go:35-60`, `internal/handler/export_handler.go:21-34`).
- **FR-011**: When an `ids` query parameter is present it MUST be parsed as a comma-separated
  list, with blank entries dropped and duplicates removed, and only those contacts exported
  (`internal/handler/export_handler.go:37-49`, `internal/service/exporter.go:49`,
  `internal/service/contact.go:305`).
- **FR-012**: A request naming more than 500 ids MUST be refused with `400` and a message stating
  the limit (`internal/service/exporter.go:36-38`, `internal/service/contact.go:280`,
  `internal/handler/export_handler.go:26-28`).
- **FR-013**: Every export MUST be scoped to the caller's own address book; ids that do not belong
  to it MUST simply not appear in the output (`internal/service/exporter.go:40-50`).
- **FR-014**: `GET /export/csv` MUST emit a fixed header row —
  `first_name,last_name,email,phone,org,title,note` — followed by one row per contact
  (`internal/service/exporter.go:77-81`).
- **FR-015**: `GET /export/json` MUST emit an indented array of objects carrying the flat contact
  fields plus the full `vcard_data` (`internal/service/exporter.go:87-132`).
- **FR-016**: All three export endpoints MUST set a matching content type and an
  `attachment` content disposition naming `contacts.vcf`, `contacts.csv` or `contacts.json`
  (`internal/handler/export_handler.go:32-33`, `:59-60`, `:72-73`).

**Backup creation**

- **FR-017**: A backup MUST write every contact's stored vCard into a single file under
  `<backup.dir>/<userID>/`, named `backup-<YYYYMMDD-HHMMSS-mmm>.vcf`, with millisecond precision
  so two backups in the same second cannot collide
  (`internal/service/backup.go:201-213`, `:275-283`).
- **FR-018**: A backup MUST be written under a temporary name and renamed into place only once
  complete, and a failed write MUST leave nothing behind — no partial file visible to listing,
  download or retention, and no orphaned temporary file
  (`internal/service/backup.go:251-302`, `internal/service/backup_integrity_test.go:56-78`).
- **FR-019**: When the user's `compress` setting is on, the backup MUST be gzip-encoded and named
  with a `.vcf.gz` suffix (`internal/service/backup.go:207-212`, `:268-273`).
- **FR-020**: After a backup is published, retention MUST delete the oldest files beyond the
  configured count. A retention failure MUST NOT invalidate the backup just written, and MUST be
  logged rather than swallowed (`internal/service/backup.go:234-241`, `:575-591`,
  `internal/service/backup_integrity_test.go:117-153`).
- **FR-021**: If the user's settings cannot be loaded, the backup MUST still proceed on defaults
  rather than fail (`internal/service/backup.go:187-194`).
- **FR-022**: Backup settings MUST be per user, exposed at `GET/PUT /api/v1/backup/settings`, and
  MUST fall back to instance defaults — the configured schedule, retention 7, uncompressed — for a
  user who has never saved any (`internal/service/backup.go:595-610`,
  `internal/handler/backup_handler.go:157-194`, `internal/domain/user_backup_settings.go`,
  `migrations/008_user_backup_settings.up.sql`).
- **FR-023**: A schedule saved with backups enabled MUST be validated as a cron expression and
  rejected with `400` if it is not (`internal/handler/backup_handler.go:174-178`).
- **FR-024**: Saving settings MUST re-register or remove that user's scheduled job in the running
  scheduler immediately, without a restart (`internal/handler/backup_handler.go:184-191`,
  `internal/worker/scheduler.go:143-158`).
- **FR-025**: At startup the server MUST register a scheduled backup job for every user whose
  settings have backups enabled with a non-empty schedule
  (`cmd/server/main.go:162-176`, `internal/service/backup.go:621-630`).

**Backup history**

- **FR-026**: Every backup attempt MUST be recorded, whatever its outcome, and the record MUST be
  written inside the backup service rather than in the scheduled job — the manual
  `POST /api/v1/backup/create` runs synchronously in the handler and would otherwise never reach
  the history (`internal/service/backup.go:111-124`, `internal/handler/handler.go:234`).
- **FR-027**: A successful run MUST record the filename, byte size, contact count and whether it
  was compressed; a failed run MUST record the cause
  (`internal/service/backup.go:152-170`, `internal/service/backup_run_test.go:100-149`).
- **FR-028**: A run MUST record what started it — `manual`, `scheduled` or `catchup`
  (`internal/domain/backup_run.go:19-26`, `internal/worker/jobs/backup_job.go:36-40`).
- **FR-029**: Finalisation of a run MUST use a context detached from the caller's, with its own
  short timeout, so a backup interrupted by a graceful shutdown is recorded rather than left at
  `running` forever (`internal/service/backup.go:147-178`,
  `internal/service/backup_run_test.go:153-166`).
- **FR-030**: Recording MUST be best-effort: a failure to open or close a history row is logged
  and MUST NOT fail the backup (`internal/service/backup.go:139-144`, `:175-178`).
- **FR-031**: The service MUST behave exactly as it did before the history existed when no run
  repository is wired in (`internal/service/backup.go:49-54`, `:129-131`,
  `internal/service/backup_run_test.go:169-176`).
- **FR-032**: `GET /api/v1/backup/runs` MUST return the caller's runs newest first, defaulting to
  50 and capped at 200 per page (`internal/handler/backup_handler.go:37-64`,
  `internal/repository/bun_backup_run.go:36-48`).
- **FR-033**: `GET /api/v1/backup/status` MUST answer "is my backup working" in one request with
  the last success, the last attempt whatever its outcome, and the next scheduled firing taken
  from the registered job (`internal/handler/backup_handler.go:71-96`,
  `internal/repository/bun_backup_run.go:54-88`, `internal/worker/scheduler.go:215-235`).
- **FR-034**: Only a `completed` run MUST count as the last success — a failed or still-running row
  says nothing about whether the data is safe
  (`internal/repository/bun_backup_run.go:50-70`).
- **FR-035**: Backup status MUST be behind authentication and scoped to the calling user; it MUST
  NOT be exposed on the unauthenticated health endpoint
  (`internal/handler/backup_handler.go:66-73`, `internal/handler/handler.go:237`).

**Missed-backup catch-up at boot**

- **FR-036**: At startup, for each user whose backup schedule is enabled, the server MUST compare
  the age of the last successful backup against one schedule period and queue a backup when it is
  older — a user who enabled backups and has never had one counts as overdue
  (`cmd/server/startup.go:78-135`, `cmd/server/startup_test.go:210-256`).
- **FR-037**: A queued catch-up MUST be recorded with trigger `catchup` so it is distinguishable
  in the history from the scheduled run (`cmd/server/startup.go:156-161`,
  `internal/domain/backup_run.go:23-25`).
- **FR-038**: The schedule period MUST be derived by asking the cron expression for its next two
  firings and measuring the gap, rather than by special-casing schedule shapes
  (`cmd/server/startup.go:137-154`, `cmd/server/startup_test.go:172-185`).
- **FR-039**: A user with backups switched off MUST NOT get a catch-up — a backup nobody asked for
  was not missed (`cmd/server/startup.go:98-100`, `cmd/server/startup_test.go:238-246`).
- **FR-040**: An unparseable schedule MUST yield no catch-up rather than a wrong one, and the
  catch-up pass MUST tolerate any of its dependencies being absent without panicking
  (`cmd/server/startup.go:86-88`, `:102-105`, `cmd/server/startup_test.go:273-277`).

**Backup file management**

- **FR-041**: `GET /api/v1/backup/list` MUST build its answer from the user's backup directory,
  including only `.vcf` and `.vcf.gz` files, sorted newest first
  (`internal/service/backup.go:305-340`).
- **FR-042**: Resolving a backup by id MUST reject any name that is not a backup filename and any
  path that resolves outside the user's own backup directory
  (`internal/service/backup.go:343-369`, `internal/service/backup_integrity_test.go:32-52`).
- **FR-043**: Download and delete MUST be scoped to the calling user and MUST answer `404` for
  anything that does not resolve (`internal/handler/backup_handler.go:119-141`).
- **FR-057**: `GET /api/v1/backup/download/:id` MUST answer with an `attachment` content
  disposition naming the backup file being sent, in the same form as the export endpoints of
  FR-016 (`internal/handler/backup_handler.go:130`). The name is the file's own — the SPA does not
  read it (it sets the name itself from the listing), so this exists for API clients: `curl -OJ`
  and anything else that saves a response under the name the server gave it.

**Restore**

- **FR-044**: `POST /api/v1/backup/restore/:id` MUST accept `?mode=merge` or `?mode=replace`,
  defaulting to `merge`, and MUST reject any other value with `400`
  (`internal/handler/backup_handler.go:143-149`).
- **FR-045**: Restore MUST read and parse the entire backup, preparing every contact, **before**
  anything destructive runs. An unreadable or empty file must not become the permanent loss of an
  address book (`internal/service/backup.go:389-464`).
- **FR-046**: A `replace` restore that prepared no contacts MUST fail with a distinct
  "empty backup" error and MUST NOT call the delete step
  (`internal/service/backup.go:380-381`, `:457-460`,
  `internal/service/backup_restore_test.go:53-80`).
- **FR-047**: Restore MUST cap the decompressed size it reads at `backup.max_restore_bytes`
  (default 128 MiB) and MUST detect the overrun by reading one byte past the limit, so an
  oversized backup is an error rather than a silent truncation
  (`internal/service/backup.go:23-28`, `:538-572`, `internal/config/config.go:137-144`,
  `internal/config/config.go:192`, `cmd/server/main.go:115`).
- **FR-048**: An oversized backup MUST be rejected before any deletion, leaving existing contacts
  intact (`internal/service/backup_integrity_test.go:83-113`).
- **FR-049**: Restore MUST honour cancellation during parsing only. There MUST be no `ctx.Err()`
  check between the delete step and the inserts: cancelling there would leave an empty address
  book and turn "more cancellable" into data loss
  (`internal/service/backup.go:446-452`, `:466-486`,
  `internal/service/cancellation_test.go:63-112`).
- **FR-050**: In `merge` mode a card whose UID already exists MUST be skipped and counted; in
  `replace` mode all existing contacts MUST be deleted first and every prepared card inserted
  (`internal/service/backup.go:457-486`, `internal/service/backup_restore_test.go:82-120`).
- **FR-051**: Restore MUST return counts of imported, skipped and errored cards
  (`internal/service/backup.go:99-104`, `:412`).
- **FR-052**: After a restore the system MUST drop the sync state of every tracked contact that is
  no longer present, because such a row maps a remote contact to a local one that is gone and the
  next export or two-way run would read it as a local deletion and delete it on the remote
  (`internal/service/backup.go:495-531`, `internal/service/backup_restore_test.go:167-181`).
- **FR-053**: The sync state of a contact that *did* come back MUST be left in place — its local
  ETag no longer matches, so the restored content travels outward as an ordinary edit rather than
  as a deletion (`internal/service/backup.go:501-503`, `:520-523`,
  `internal/service/backup_restore_test.go:198-208`).
- **FR-054**: Restore MUST work when no sync-state repository is wired in; the reconciliation is
  optional wiring (`internal/service/backup.go:504-507`,
  `internal/service/backup_restore_test.go:211-218`).
- **FR-055**: The web UI MUST require the user to type the word `replace` before a replace restore
  can be submitted (`web/src/views/backup/BackupView.vue:161-171`, `:301-303`).
- **FR-056**: The Backup screen MUST open with a single-line verdict — `healthy`, `failing`,
  `overdue`, `never` or `disabled` — treating a failed or interrupted latest attempt as more
  significant than an older success, and marking a success older than two schedule periods as
  overdue (`web/src/utils/backup-health.ts:12-100`).

### Key Entities

- **Backup file**: a plain vCard document containing every contact in one user's address book at
  one moment, optionally gzip-encoded. Its *identity is its filename* — there is no database row
  for the file. Lives at `<backup.dir>/<userID>/backup-<timestamp>.vcf[.gz]`.
- **BackupRun**: one recorded attempt to create a backup — who for, what triggered it (manual,
  scheduled, catchup), its status (running, completed, failed, interrupted), the resulting
  filename, size and contact count, the error if any, and when it started and finished. It exists
  because the files cannot answer "when did this last succeed": retention deletes them, and at
  retention 1 the only surviving file is always the newest one
  (`internal/domain/backup_run.go`, `migrations/025_backup_runs.up.sql`).
- **UserBackupSettings**: one row per user — cron schedule, retention count, enabled flag, compress
  flag. Absent means instance defaults (`internal/domain/user_backup_settings.go`,
  `migrations/008_user_backup_settings.up.sql`).
- **BackupInfo**: the view of a backup file the API returns — id (the filename), filename, size and
  creation time, all derived from the directory listing (`internal/service/backup.go:91-97`).
- **RestoreResult / ImportResult**: the counts a bulk operation reports — imported, skipped,
  errored. Structurally identical; `skipped` is meaningful for restore and dead for import
  (`internal/service/backup.go:99-104`, `internal/service/importer.go:29-33`).
- **SyncState** (referenced, owned by 006): the mapping between a remote contact and a local one.
  Restore is a *writer* of this table only in the negative sense — it deletes rows whose local
  contact did not come back.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A restore that is rejected — empty backup, unparseable backup, oversized backup, or
  cancelled during parsing — leaves **100% of existing contacts** in place. Every rejection path
  returns before the delete step is reached.
- **SC-002**: A `replace` restore that has begun deleting completes its inserts regardless of the
  caller going away. This is a property of one stretch of code that contains no cancellation
  check between the delete and the last insert — as FR-049 states it. It is established by
  reading that stretch and pinned by a test that cancels immediately after the delete; it is not
  a proof over all paths.
- **SC-003**: A restore removes **zero** contacts on any connected remote provider: every sync
  state whose local contact did not come back is dropped before the next sync can read it as a
  deletion.
- **SC-004**: Every backup attempt appears in the history — manual, scheduled and catch-up alike —
  so "when did my backup last succeed" is answerable **from one request**
  (`GET /api/v1/backup/status`), not by counting files.
- **SC-005**: **No** backup run remains in the `running` state after the process that started it
  has exited: an interrupted run is closed at the next boot, and a run interrupted by a graceful
  shutdown is finalised in-process on a detached context.
- **SC-006**: A scheduled backup missed because the server was down is taken **at the next start**
  rather than at the next cron firing — reducing the worst-case gap from one full schedule period
  to the length of the outage.
- **SC-007**: A user can tell whether backups are working **without reading a log file or listing
  a directory** — one word on the Backup screen, with the failing attempt's error text beside it.
- **SC-008**: Restore memory is bounded by an explicit, configurable number (default **128 MiB**
  decompressed, `backup.max_restore_bytes`), not by the compressed file size — a gzip bomb cannot
  turn a restore into an out-of-memory kill.
- **SC-009**: A single selective export request is bounded at **500 contact ids** and is refused
  outright above that, so a runaway or hostile selection cannot become an unbounded query. Within
  the limit the export is scoped to the caller's address book: an id that does not belong to it is
  absent from the file rather than an error, exactly as FR-013 requires. A selective export is
  therefore **not** guaranteed to contain every id requested.
- **SC-010**: A partially malformed import file still imports every good card: one bad card costs
  one contact, not the whole file.
- **SC-011**: Changing a backup schedule takes effect **without a server restart** — the job is
  re-registered in the running scheduler as part of saving the setting.

## Assumptions

Conditions the implementation takes as given.

- **Single instance.** The backup directory is a local path and the job queue is in-process. Two
  processes sharing one database would each run their own schedules and each write into the same
  directory. The reconciliation that closes interrupted runs is bounded by process start time
  specifically so that a second instance is harmless rather than destructive, but multi-instance
  is not a supported configuration (mechanism owned by 008).
- **The job queue is in-memory and lossy.** A buffered channel of 100 jobs in one process; a
  restart drops whatever was queued. The boot-time catch-up exists precisely because that loss is
  unacceptable for backups and acceptable for everything else
  (`internal/worker/goroutine_worker.go:45`, `cmd/server/startup.go:68-77`).
- **The backup directory is durable and is somebody else's problem.** Backups are files on the
  server's own filesystem (`internal/service/backup.go:201`). Nothing here replicates, encrypts or
  verifies them offsite, and the backup directory is not itself included in any backup. If the
  volume holding `backup.dir` is lost, the backups go with it. Restore is the recovery path for
  user error and bad syncs, not for host loss.
- **Restore holds the whole decompressed backup in memory.** This is why the cap exists. A larger
  address book than the cap allows requires raising `backup.max_restore_bytes` and having the
  memory for it; there is no streaming restore.
- **Import and restore run synchronously inside the HTTP request.** Neither is a background job,
  which is why the configuration refuses a non-zero `server.write_timeout` (invariant owned by
  008). A very large import occupies a request for its whole duration.
- **A backup is its stored vCard text.** The relational contact fields are treated as derived from
  the card, so restoring a card is treated as restoring the contact
  (`internal/service/backup.go:275`). The consequence when they have drifted apart is recorded
  under Known Divergences.
- **Migrations are forward-only.** `008_user_backup_settings` and `025_backup_runs` cannot be
  rolled back by the application; the `.down.sql` files exist but nothing applies them. Undoing a
  schema change means restoring a database dump (constitution, Principle I).
- **The vCard and CSV text formats themselves are specified in domain 003.** This spec assumes a
  parser that splits a stream into cards, a builder that renders one, and the encoder's documented
  limitations — including the deliberate gap where a `;` inside a single-valued TEXT property is
  not escaped. Import, export, backup and restore inherit all of it.
- **`?ids=` selection is bounded by the UI's own paging.** The 500-id cap is described in the code
  as well above anything the list view can produce; it is there to stop a hostile payload becoming
  an unbounded query, not to constrain normal use.

## Status

**Shipped.** Reconstructed at `23a167c` (`v0.4.0-3-g23a167c`); every requirement above was read
out of that tree. The migrations that back it — `008_user_backup_settings` and `025_backup_runs` —
are applied at startup and are present in `expectedTables`
(`internal/repository/migrate_postgres_test.go:60`, `:80`).

## Code Paths

Paths this spec **owns**. Exactly one spec may own a path (constitution, Principle VII).

- `internal/domain/backup_run.go`
- `internal/domain/user_backup_settings.go`
- `internal/handler/import_handler.go`
- `internal/handler/export_handler.go`
- `internal/handler/backup_handler.go`
- `internal/handler/backup_download_test.go`
- `internal/repository/bun_backup_run.go`
- `internal/repository/bun_user_backup_settings.go`
- `internal/service/importer.go`
- `internal/service/importer_test.go`
- `internal/service/exporter.go`
- `internal/service/exporter_test.go`
- `internal/service/backup.go`
- `internal/service/backup_integrity_test.go`
- `internal/service/backup_restore_test.go`
- `internal/service/backup_run_test.go`
- `internal/service/cancellation_test.go`
- `internal/worker/jobs/backup_job.go`
- `internal/worker/jobs/backup_job_test.go`
- `migrations/008_user_backup_settings.up.sql`
- `migrations/008_user_backup_settings.down.sql`
- `migrations/025_backup_runs.up.sql`
- `migrations/025_backup_runs.down.sql`
- `web/src/api/backup.ts`
- `web/src/api/import-export.ts`
- `web/src/utils/backup-health.ts`
- `web/src/utils/backup-health.spec.ts`
- `web/src/views/backup/`
- `web/src/views/import-export/`

## References

Paths this spec touches but does **not** own.

- `cmd/server/startup.go` — the boot-time catch-up and reconciliation pass (FR-036…FR-040 name it;
  the reconciliation mechanism and its process-start-time boundary belong to 008).
- `cmd/server/startup_test.go`
- `cmd/server/main.go` — composition root: wiring, body limits, backup job registration.
- `internal/worker/scheduler.go` — cron registration and `NextRun` (FR-023…FR-025, FR-033).
- `internal/worker/goroutine_worker.go` — the in-process job queue the catch-up enqueues into.
- `internal/vcard/split.go`, `internal/vcard/builder.go`, `internal/vcard/parser.go` — the single
  vCard writer and its parser, owned by 003.
- `internal/domain/sync_state.go`, `internal/repository/bun_sync.go`,
  `internal/repository/interfaces.go` — the sync state restore reconciles, owned by 006.
- `internal/config/config.go` — `backup.*` keys, including `max_restore_bytes`; the configuration
  invariants themselves are owned by 008.
- `internal/handler/handler.go` — route registration and the per-path body-limit resolution.
- `web/src/types/index.ts` — the `ImportResult` / `RestoreResult` shapes the SPA consumes.

Out of scope and owned elsewhere: the vCard and CSV text formats (003); the boot-time
reconciliation mechanism, the `server.write_timeout` invariant and the shared expensive-operation
rate limiter (008); replication to a remote provider, which is not a backup (006).

## Enforced By

**Import**

- `TestImportVCard_ExtractsTitleNote`, `TestImportVCard_ExtractsTitleNote_MultipleCards`
  (`internal/service/importer_test.go:21`, `:32`) — FR-002.
- `TestImportCSV_ExtractsTitleNote` (`internal/service/importer_test.go:58`) — FR-006 and FR-007,
  for the canonical header names only.
- `TestImportVCard_StopsOnACancelledContext`, `TestImportCSV_StopsOnACancelledContext`
  (`internal/service/cancellation_test.go:29`, `:43`) — FR-005.
- `TestBodyLimit_AnOverrideRaisesTheLimitForItsPath`
  (`internal/handler/middleware/bodylimit_test.go:92`) — the mechanism FR-009 relies on.

**Export**

- `TestExportVCardByIDs_ExportsOnlyTheSelectedContacts` (`internal/service/contact_bulk_test.go:84`)
  — FR-011 and FR-013.
- `TestExportVCardByIDs_EmptyIDsExportsEverything` (`internal/service/contact_bulk_test.go:97`) and
  `TestExportVCard_MatchesExportVCardByIDsWithNoIDs` (`:106`) — FR-010.
- `TestExportVCardByIDs_RejectsAnOversizedRequest` (`internal/service/contact_bulk_test.go:118`) —
  FR-012, SC-009.
- `TestExportCSV_IncludesTitleNoteColumns` (`internal/service/exporter_test.go:21`) — FR-014.
- `TestExportJSON_IncludesTitleNote` (`internal/service/exporter_test.go:46`) — FR-015.

**Backup creation and files**

- `TestCreate_LeavesNoPartialFileVisible` (`internal/service/backup_integrity_test.go:56`) —
  FR-018, and the listing half of FR-041.
- `TestCreate_RetentionFailureIsLoggedNotSwallowed` (`internal/service/backup_integrity_test.go:117`)
  — FR-020.
- `TestGetPath_RejectsNonBackupFilenames` (`internal/service/backup_integrity_test.go:32`) — FR-042.
- `TestBackupDownload_SetsAnAttachmentFilename` (`internal/handler/backup_download_test.go:20`) —
  FR-057.

**Settings and scheduling**

- `TestValidateCron_Valid`, `TestValidateCron_Invalid` (`internal/worker/scheduler_test.go:179`,
  `:194`) — FR-023.
- `TestRegisterBackupForUser` (`internal/worker/scheduler_test.go:78`) — FR-025.
- `TestReregisterBackupForUser`, `TestReregisterBackupForUser_EmptyRemoves`
  (`internal/worker/scheduler_test.go:208`, `:217`) — FR-024, SC-011.
- `TestBackupPayload_Serializable` (`internal/worker/scheduler_test.go:105`) — the payload the
  scheduled job carries.

**History**

- `TestBackupRun_SuccessIsRecordedWithTheFileDetails` (`internal/service/backup_run_test.go:100`) —
  FR-026, FR-027.
- `TestBackupRun_ScheduledTriggerIsRecordedSeparately` (`internal/service/backup_run_test.go:118`) —
  FR-028.
- `TestBackupRun_FailureIsRecordedWithTheCause` (`internal/service/backup_run_test.go:130`) — FR-027.
- `TestBackupRun_IsFinalisedEvenWhenTheCallersContextIsCancelled`
  (`internal/service/backup_run_test.go:153`) — FR-029, SC-005.
- `TestBackupRun_WithoutARepositoryNothingChanges` (`internal/service/backup_run_test.go:169`) —
  FR-031.
- `TestBackupJob_RunsTheBackupAsScheduledByDefault`, `TestBackupJob_PassesAnExplicitTriggerThrough`,
  `TestBackupJob_WrapsAFailureWithTheUserID`, `TestBackupJob_RejectsAnUnreadablePayload`
  (`internal/worker/jobs/backup_job_test.go:39`, `:52`, `:66`, `:77`) — FR-028, FR-037.

**Catch-up at boot**

- `TestSchedulePeriod` (`cmd/server/startup_test.go:172`) — FR-038.
- `TestCatchUpMissedBackups_QueuesWhenTheLastSuccessIsStale` (`cmd/server/startup_test.go:210`),
  `TestCatchUpMissedBackups_SkipsAFreshSuccess` (`:225`),
  `TestCatchUpMissedBackups_QueuesWhenThereIsNoSuccessAtAll` (`:249`) — FR-036, SC-006.
- `TestCatchUpMissedBackups_SkipsADisabledSchedule` (`cmd/server/startup_test.go:238`) — FR-039.
- `TestCatchUpMissedBackups_QueuesAtMostOnePerUser` (`cmd/server/startup_test.go:258`) — bounded
  count only; see Known Divergences.
- `TestCatchUpMissedBackups_ToleratesMissingDependencies` (`cmd/server/startup_test.go:273`) —
  FR-040.
- `TestReconcileInterruptedRuns_ClosesBothHistories` (`cmd/server/startup_test.go:88`) and
  `TestReconcileInterruptedRuns_LeavesRunsStartedAfterThisProcess` (`:105`) — SC-005 at boot;
  the mechanism is owned by 008.

**Restore**

- `TestRestore_ReplaceWithEmptyBackupKeepsExistingContacts`,
  `TestRestore_ReplaceWithUnparseableBackupKeepsExistingContacts`
  (`internal/service/backup_restore_test.go:53`, `:68`) — FR-045, FR-046, SC-001.
- `TestRestore_ReplaceSwapsContents`, `TestRestore_MergeKeepsExistingAndSkipsDuplicates`
  (`internal/service/backup_restore_test.go:82`, `:102`) — FR-050, FR-051.
- `TestRestore_LongPhotoLineDoesNotTruncateBackup`
  (`internal/service/backup_restore_test.go:123`) — a long folded line survives the round trip.
- `TestRestore_DropsSyncStateOfContactsThatDidNotComeBack`
  (`internal/service/backup_restore_test.go:167`) — FR-052, SC-003.
- `TestRestore_MergeKeepsSyncStateOfUntouchedContacts`,
  `TestRestore_KeepsSyncStateOfRestoredContacts`
  (`internal/service/backup_restore_test.go:184`, `:198`) — FR-053.
- `TestRestore_WithoutSyncStateRepoStillRestores`
  (`internal/service/backup_restore_test.go:211`) — FR-054.
- `TestRestore_OversizedBackupIsRejectedBeforeDeleting`
  (`internal/service/backup_integrity_test.go:83`) — FR-047, FR-048, SC-001, SC-008.
- `TestRestore_IsNotAbortedAfterDeleteAll` (`internal/service/cancellation_test.go:63`) — FR-049,
  SC-002.
- `TestRestore_CancelledDuringParsingDeletesNothing`
  (`internal/service/cancellation_test.go:90`) — FR-049, SC-001.

**Web UI**

- `web/src/utils/backup-health.spec.ts` — the `backupHealth` suite (`:24-91`, eight cases covering
  healthy / failing / interrupted / overdue / one missed period / never / disabled / missing
  settings) and the `schedulePeriodMs` suite (`:92-106`) — FR-056, SC-007.

**Schema**

- `TestPostgres_MigrateAppliesEverySchemaObject` (`internal/repository/migrate_postgres_test.go:85`)
  — `backup_runs` and `user_backup_settings` exist on PostgreSQL, not only on SQLite.

**CI**

- `.github/workflows/ci.yml` — `go test ./... -count=1 -race` (job `test`), `npm run test` in
  `web/` (job `frontend`), and `go test ./internal/repository/ -run TestPostgres` (job
  `postgres`).

## Known Divergences

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


Where shipped behaviour differs from stated intent, or where a requirement above has no test
behind it. Nothing here is presented as a requirement that is met.

**Contradicts a stated project rule**

- **`POST /backup/restore/:id` returns the raw error text** (`err.Error()`), unlike the rest of the
  API, which answers a fixed `"internal server error"` (constitution, Principle III). This is what
  makes "backup contains no readable contacts" and "backup exceeds the maximum restore size"
  legible to the user, but it also means a database error's text reaches the client
  (`internal/handler/backup_handler.go:152`). Deliberate, and not defended — the legibility could
  have been bought with typed `*fiber.Error` values instead.
**Backup download**

- **`GET /backup/download/:id` states a filename but not a content type.** FR-057 is met; the
  narrower gap that replaced it is that the response's `Content-Type` is still whatever fasthttp's
  `SendFile` infers from the extension, not a type the handler chose
  (`internal/handler/backup_handler.go:130-131`). The three export endpoints set both
  (FR-016). Nothing depends on the inferred value today: the SPA asks for a blob and API clients
  are saving the bytes to disk.

**Restore**

- **A restore that succeeds can still return a 500.** If sync-state reconciliation fails after the
  contacts have been written, the `RestoreResult` is discarded and an error is returned. The
  contacts are in place; the caller cannot tell (`internal/service/backup.go:488`).
- **Restore mints a new internal contact id** even for a contact whose UID is unchanged
  (`internal/service/backup.go:436`). Only the UID survives a `replace` restore; anything holding
  an internal id across a restore is holding a dangling reference.
- **Sync-state reconciliation is one lookup per state row.** It walks every sync state the user has
  and calls `GetByUID` for each (`internal/service/backup.go:514`). Correct, but linear in the
  number of tracked contacts and not batched.
- **A backup captures stored vCard text only.** `writeBackupFile` writes `contact.VCardData` and
  nothing else (`internal/service/backup.go:275`), so anything in the relational child tables that
  has drifted from the stored card is not in the backup and does not come back.

**Import**

- **CSV import never deduplicates.** Every row becomes a new contact with a freshly minted UID, so
  importing the same CSV twice produces two copies of everything
  (`internal/service/importer.go:152`). Only vCard import matches on UID.
- **`skipped` is always 0 for imports.** Neither import path ever increments it; the field exists
  in the response shape and in the UI, and is dead (`internal/service/importer.go:41`).
- **The import screen never shows per-card errors.** The API returns `errors` as a count
  (`ImportResult.Errors int`, `internal/service/importer.go:32`) while the SPA types it as
  `string[]` and renders `v-if="result.errors?.length"` — a number has no `.length`, so the block
  never renders and a partly-failed import looks clean
  (`web/src/types/index.ts:404-408`, `web/src/views/import-export/ImportView.vue:34-36`).
  FR-004 and SC-010 are true of the server and invisible in the UI.
- **CSV import reads only seven fields.** Multi-value emails, phones, addresses, URLs, IMs,
  categories and dates cannot be expressed in the CSV shape it accepts
  (`internal/service/importer.go:144`).
- **Cancellation during import is largely theoretical in production.** The check is real and
  tested, but with fasthttp the request context is only cancelled at server shutdown, so in
  practice FR-005 fires on shutdown rather than when a browser tab is closed.

**Export**

- **CSV export is lossy.** It writes a fixed seven-column row per contact from the flat columns
  only; every multi-value field is dropped (`internal/service/exporter.go:77`). vCard export is
  lossless because it emits the stored card verbatim, and JSON export includes `vcard_data`
  alongside the flat fields, so it is lossless too.
- **`?ids=` applies to vCard export only.** CSV and JSON always export the whole address book
  (`internal/service/exporter.go:63`, `:100`).

**Backup files and catch-up**

- **Retention counts whatever is in the directory.** Any file ending `.vcf` or `.vcf.gz` that lands
  in a user's backup directory is listed as a backup and is eligible for deletion by retention
  (`internal/service/backup.go:321`, `:575`).
- **The catch-up "at most one per user" claim is a property of the boot sequence, not of the
  loop.** `catchUpMissedBackups` runs once per start and does not deduplicate the user list it is
  given; the id list happens to come from `ListAllIDs`, which is unique. The test asserts only
  that the count stays bounded (`cmd/server/startup.go:91`, `cmd/server/startup_test.go:258`).
- **Catch-up covers backups and nothing else.** The job queue is a buffered channel of 100 in one
  process; anything queued and not yet run is lost on restart. Catch-up is described in the code
  as "the cheap half of a durable queue" — the one job whose loss actually costs something
  (`internal/worker/goroutine_worker.go:45`, `cmd/server/main.go:195`).

**Requirements with no enforcer**

These are review-only. Naming them is the point of this section; each is a gap, not a decision.

- **FR-001** — no test exercises the vCard import handler's multipart-or-raw-body branch or its
  `400`. There is no test file for `internal/handler/import_handler.go`.
- **FR-006, partially** — `TestImportCSV_ExtractsTitleNote` uses the canonical header names only.
  The alias sets the requirement enumerates (`firstname`, `e-mail`, `company`, `notes`, …) and the
  case-insensitive matching are untested.
- **FR-016** — no test asserts the content type or `Content-Disposition` of any export endpoint;
  `internal/service/exporter_test.go` stops at the service boundary.
- **FR-017 and FR-019** — the backup filename format and the gzip-and-`.vcf.gz` path are never
  asserted on creation. `TestRestore_OversizedBackupIsRejectedBeforeDeleting` reads a `.vcf.gz`
  file it wrote itself, which proves the reader, not the writer.
- **FR-022** — the fallback to instance defaults for a user with no saved settings is untested.
- **FR-032, FR-033, FR-034** — `internal/repository/bun_backup_run.go` has no test at all: neither
  the newest-first ordering, the 50/200 paging, nor the "only `completed` counts as success" rule
  is verified anywhere. SC-004 rests on this code.
- **FR-035** — nothing asserts that `/backup/status` is behind authentication; it is visible in the
  route table (`internal/handler/handler.go:237`) and nowhere else.
- **FR-041, sorting** — `TestCreate_LeavesNoPartialFileVisible` checks the listing filter, not the
  newest-first order.
- **FR-043** — no test covers download/delete scoping or the `404`.
- **FR-044** — no test covers the `?mode=` parsing or its `400` on an unknown value.
- **FR-055** — the typed-`replace` confirmation is Vue component state with no component test;
  `web/` has unit tests for `utils/` only.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-10 | unreleased | **D-term** — recorded the terminator loss on import/restore and the unseparated concatenation on export/backup, which silently reduced a multi-card address book to one contact on any round trip. Fixed at all four sites via `vcard.Terminated`; regression covered by `TestTerminated_ConcatenatedCardsStillSplit`. Found by the divergence triage, admitted by no spec. | — |
| 2026-08-07 | v0.4.0 | Initial retrospective spec, reconstructed at `23a167c`. | — |
| 2026-08-07 | v0.4.0 | Conformed to the house template: house header replacing `Feature Branch`/`Input`; ownership recorded in `Code Paths` and `References`; `Enforced By` added with verified test names; admissions moved out of Edge Cases and Assumptions into `Known Divergences`, including the requirements that have no enforcer; SC-002 restated as FR-049 states it rather than as an absolute; SC-009 corrected to agree with FR-013; test and file citations removed from Success Criteria and added to the user stories. | — |
| 2026-08-07 | unreleased | D4: `GET /backup/download/:id` now sets an `attachment` content disposition naming the file (FR-057 added, enforced by `TestBackupDownload_SetsAnAttachmentFilename`); the matching Known Divergence is replaced by the narrower one that the response's content type is still inferred by `SendFile`. The Backup screen's download button now reports a failed request instead of doing nothing. | — |
