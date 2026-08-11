# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
uses [Semantic Versioning](https://semver.org/). While the major version is `0`, breaking
changes may appear in a minor release.

## [Unreleased]

### Fixed

- **Exporting or backing up an imported address book lost every contact but the first.**
  `POST /import/vcard` and a backup restore stored each card with its trailing CRLF stripped, and
  export and backup then wrote the stored cards one after another with nothing between them —
  producing `END:VCARDBEGIN:VCARD` on a single line. Every vCard splitter, this project's
  included, looks for a line *beginning* with `BEGIN:VCARD`, so the rest of the file was
  discarded. Measured on a three-contact import: the exported file re-imported as **0 contacts**,
  and a `replace` restore of a backup deleted all three and inserted one.

  Contacts created through the web form, CardDAV or a merge were always written correctly, which
  is why every existing test passed. If your address book came from an import, **an export or
  backup taken before this release is not a complete copy — take a fresh one.** No data
  migration is needed: the fix is applied when the file is written, so cards already stored
  without a terminator now export correctly.

- **A clean checkout did not compile.** `internal/web/static/spa/.gitkeep` was deleted in
  `a0ab02f`; `go:embed all:static/spa` requires the directory to exist. CI stayed green because
  both Go jobs recreate the file before building.

## [0.5.0] — 2026-08-08

### ⚠️ Breaking

- **A CardDAV card larger than `carddav.max_resource_bytes` (1 MiB by default) is now actually
  refused with `413`.** v0.4.0 announced the limit to clients as `CARDDAV:max-resource-size`
  and never compared anything against it; the v0.4.0 entry below claimed the `413` and was
  wrong. It is real now, on `PUT` through `/dav` only.

  **Read this before upgrading if you have ever stored a large contact — one with an embedded
  photo is the usual case.** The limit is checked on the CardDAV write path and nowhere else:
  `POST /api/v1/contacts`, `POST /import/*` and inbound sync can all still store a card above
  it, and `GET` still serves one. So an oversized contact syncs down to a phone perfectly and
  then can never be edited *from* that phone — the phone's save gets a `413`, and most CardDAV
  clients retry it forever rather than tell the user. Find out first:

  ```sql
  -- PostgreSQL: cards above the 1 MiB default
  SELECT id, uid, octet_length(vcard_data) AS bytes
  FROM contacts WHERE octet_length(vcard_data) > 1048576 ORDER BY bytes DESC;
  -- SQLite: use length(CAST(vcard_data AS BLOB)) instead of octet_length(...)
  ```

  If that returns rows, raise `CHQ_CARDDAV_MAX_RESOURCE_BYTES` above the largest of them before
  upgrading, or shrink those contacts' photos. Note the limit is applied to the card **as
  stored** (re-encoded), not to the bytes on the wire, and that it is policy rather than
  protection: `server.max_body_bytes` is still the only limit that bounds memory, because
  fasthttp reads the whole body before any handler runs.

- **A sync credential stored before v0.4.0 with an `http://` endpoint is now refused at run
  time.** v0.4.0 validated endpoints at the four places one can be *entered*, and a pipeline
  step that names a `credential_id` carries no endpoint of its own — so the check found nothing
  to look at and the stored row was used as it stood. Every run of such a step has been sending
  the provider's password, or an OAuth bearer token, in clear text. The endpoint is now
  validated after the credential is resolved, on the value the connection is actually made
  with, which also covers the OAuth path and does so before the token exchange rather than
  after it.

  **If you sync against a CardDAV server over plain http, set
  `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true` (`sync.allow_insecure_endpoints`) before
  upgrading** — the same variable v0.4.0 introduced, which now covers stored credentials too.
  Nothing rewrites your database; the credential is untouched and the variable brings it back.
  The failure is per step: it is recorded against that step in the pipeline's run history and
  logged, and the pipeline's other steps still run. Google credentials are unaffected, as is
  the internal provider.

  Do **not** reach for the credential's `skip_tls_verify` box instead. It is not an answer to
  this: it leaves `https://` in the URL while removing the verification that makes it mean
  anything, so the password reaches an on-path attacker exactly as `http://` did. That box is
  still honoured and is still not validated here.

### Fixed

- **Searching for something that matches nothing no longer blanks the contact list.** An empty
  result was serialised as JSON `null` rather than `[]`, and the list view reads `.length` off
  it. Any filter combination with no matches produced this, not just a first run.
- **Search is no longer case-sensitive on PostgreSQL.** `LIKE` folds ASCII case on SQLite and
  does not on PostgreSQL, so on the engine `docker compose` provisions, searching `john` did not
  find `John Smith`. Both sides of every comparison are now lowered. The folding SQLite performs
  is ASCII-only, so a query in a non-Latin script still matches exactly there.
- **The sync conflict screens no longer render blank.** When a conflict carried no field-level
  diffs — the ordinary manual-mode case, where the two sides edited *different* properties — the
  server stored the diff list as `null` rather than `[]`. `JSON.parse("null")` returns null
  without raising, so the guard the pages already had never fired and both the detail page and,
  through a single such row, the whole conflicts list came up empty. Found while reviewing the
  fix below, which puts a button on the page this bug prevented from rendering. Rows already
  written as `null` are handled by the browser, so no data has to be rewritten.
- **"Apply remote (source wins)" on a sync conflict now actually applies the remote card.**
  When a conflict carried no per-field diffs, the button reported success and changed nothing:
  the browser sent a wildcard instruction meaning "take the remote card wholesale" and the
  server had never implemented its half, so it kept the local card and marked the conflict
  resolved. Every use of that button since it shipped silently kept the local version.
  **Read this before you use it again.** Now that it works, it does the destructive thing it
  always claimed to: the remote card replaces the contact, and any property that exists only
  locally — a note, a title, a phone the remote does not have — is removed, not merged. On the
  ordinary manual-mode conflict, where the two sides edited different fields, that means your
  local edits go away. The button now asks for confirmation and says so.
  There is no undo in the app. The pre-resolution card is still stored on the conflict row
  (`local_vcard`) and is returned by `GET /api/v1/sync/conflicts/:id`, but no screen shows it
  and the row is marked `resolved`, so recovering a mistake means reading that field over the
  API or the database and restoring the contact by hand. Per-field resolution is unaffected.
- **The contact form no longer discards `GEO`.** `GEO` is one of the vCard properties an edit
  replaces wholesale, and the form payload had no field for it, so saving a contact from the
  web UI wrote an empty `GEO` over whatever the card arrived with — and pushed the loss out to
  every synced client on the next run. The form now carries the stored value through untouched.
  There is still no input for it: you cannot set or change a location from the browser, only
  keep the one that is there.
  Note for API callers: `fields` is a full replacement of the properties it covers, so a `PUT`
  whose `fields` object omits `geo` still clears `GEO`, exactly as omitting `note` clears the
  note. Send the value back if you want to keep it.
- **`set-password` was telling operators that a compromised session survives 24 hours when it
  survives one.** The success message quoted "default 24h" for access tokens and "default 720h"
  for refresh tokens; 0.4.0 changed those defaults to `1h` and `168h` and the text was not moved
  with them. Telling an operator how long the old credential outlives the reset is the only job
  that message has, so being wrong by 24× is the whole message failing. It now reads
  `auth.token_ttl` and `auth.refresh_ttl` out of the configuration the command itself loaded,
  and says plainly that those are the values *this* process read: a subcommand cannot see the
  environment of a server it is not running in, so run it where the server's configuration is
  (`docker compose exec`, not a separate `docker run` with a different `-e` set). This is the
  second place those two numbers went stale after 0.4.0; `config.example.yaml` was the first.
- **`set-password` no longer asks for a password before checking that the database has a
  schema.** On an unmigrated database it prompted twice and only then exited `5`. The check
  comes first now. Against a migrated database nothing changes; against an unmigrated one, a run
  that previously reported a usage error (`2` — a missing `--stdin`, say) now reports `5`,
  because the database is reached first.
- **Duplicate detection now looks at every email address and phone number a contact has, not
  just the first one.** It bucketed on the `contacts.email` and `contacts.phone` columns alone,
  so two records for one person who share only a second address or a second number were never
  compared — while the pair list, which reads `contact_emails` and `contact_phones` to decide
  whether a one-click "Keep A" is lossless, was already using exactly that data. The list could
  therefore call a merge provably safe for a pair the detector had never been able to find.

  **Expect the first scan after upgrading to report more pairs than the last one before it.**
  That is the fix working. The scan summary will look odd while it does: "checked" is still the
  number of contacts and is unchanged, while "found" jumps. Nothing is wrong; the pairs were
  always there.

  **A smaller number of duplicates may stop being reported, and this one is worth reading.**
  A value shared by more than 500 contacts is treated as saying nothing about identity and its
  whole group is skipped — that has always been true, but the count now includes secondary
  values. An office number held by 300 people on their contact row and by another 250 as a
  second number is now one group of 550 and is dropped, where the 300 were compared before.
  The skip is a log line (`skipping an implausibly large duplicate bucket`) and appears nowhere
  in the API or the UI, so if pairs you used to see have gone, that warning is where to look.

  Only emails and phone numbers are read from the child tables; addresses, URLs, messaging
  handles, tags and dates are not, because nothing scores a match on them. A scan costs more
  memory than it did — roughly 7 MB instead of 5 MB per 10 000 contacts before any secondary
  values, and about 17 MB for a book where every contact has two of each — and about twice the
  time. It remains milliseconds, not the 39 seconds it cost before v0.3.0.
- **Downloading a backup names the file and reports its own failures.**
  `GET /api/v1/backup/download/:id` sent the bytes with no `Content-Disposition`, leaving every
  client to infer a name from the URL; the three export endpoints have always set one. It now
  answers `attachment; filename="backup-YYYYMMDD-HHMMSS-mmm.vcf.gz"`. The response's
  `Content-Type` is still inferred from the extension rather than chosen, which the exports do
  set — that half is not fixed.
  On the Backup screen the download button had no error path at all: a backup deleted in another
  tab answered `404` and the button did nothing — no message, no console line, nothing to
  distinguish it from a slow download. It now says so.

### Upgrading

No migrations. Before starting the new version:

1. If any sync pipeline reaches a provider over plain `http://` — including through a credential
   saved before v0.4.0, which used to be exempt — set `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true`,
   or those steps will start failing. Turning on a credential's `skip_tls_verify` is not a
   substitute and does not make the transport confidential.
2. If any stored contact is larger than `carddav.max_resource_bytes` (1 MiB by default), raise
   `CHQ_CARDDAV_MAX_RESOURCE_BYTES` above the largest of them or shrink them. The query to find
   them is above. Otherwise those contacts become read-only from every CardDAV device.
3. Expect the first duplicate scan after upgrading to report more pairs. If pairs you used to see
   have instead disappeared, look for `skipping an implausibly large duplicate bucket` in the log.
4. Take a database dump anyway before resolving any sync conflict with **Apply remote (source
   wins)**: that button now does what it always said it did, and the app has no undo for it.

## [0.4.0] — 2026-08-07

Limits, timeouts, and a request-forgery surface closed. Small in code, but three of these
changes will refuse something your deployment currently does, so read **Breaking** before
upgrading.

### ⚠️ Breaking

- **A provider endpoint over plain `http://` is now refused.** A sync request carries the
  provider's username and password, and until now the URL was never checked at all: `file://`,
  `gopher://` and a bare hostname were equally acceptable. Validation runs at all four places
  an endpoint can enter — CardDAV connect, a stored credential, the endpoint inside a pipeline
  step's config, and the one posted to a manual trigger.
  **If you sync against a CardDAV server on your LAN over http, set
  `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true` before upgrading**, or every run will fail.
  Private addresses themselves are *not* filtered: a CardDAV server on the local network is a
  supported setup, and the redirect rule below is what actually prevents the pivot that
  filtering them would target.
- **`server.write_timeout` must be `0`, and the server refuses to start otherwise.** Restore
  and import run synchronously inside the request; a write deadline truncates an operation that
  is still changing contacts. If you set this key, remove it.
- **Access tokens now live 1 hour instead of 24, and refresh tokens 7 days instead of 30.**
  The web UI refreshes transparently. A script that grabs a token and reuses it for a day will
  need to call `POST /auth/refresh`; a session idle for more than a week now requires a login.
- **Request bodies are capped at 32 MiB** (`CHQ_SERVER_MAX_BODY_BYTES`) and imports likewise; a
  larger upload gets a `413` instead of being read into memory. A per-card CardDAV limit of
  1 MiB was *advertised* to clients here, but not enforced until Unreleased above — this entry
  originally claimed the `413` and was wrong.
  Raise the limits if you import bigger files — but note the cap is what bounds memory:
  fasthttp reads a whole body before any handler runs, so N concurrent uploads cost N × that.

### Added

- **Every request gets an id**, reused from `X-Request-Id` when a proxy supplied one and minted
  otherwise, echoed in the response header and attached to the log line. Without it there was
  no way to connect "it failed at 14:32" to anything in the log.
- **`GET /health` reports the schema version** (`025_backup_runs`), which is the first question
  after an upgrade.
- `read_timeout` (30s) and `idle_timeout` (120s), so a connection that opens and says nothing
  no longer occupies the server indefinitely.

### Changed

- **A successful `/health` check is no longer logged.** Docker polls it every 30 seconds, so an
  idle container's log consisted of nothing else and a real line was buried among thousands. A
  *failing* check is always logged — that is exactly when it matters.
- The sync HTTP client follows at most three redirects and refuses to cross hosts. Validating
  the URL a user typed does not stop a permitted host answering `302 → 169.254.169.254`.
  Same-host redirects still work, which is how many servers implement `.well-known`.

### Configuration

New keys, all optional:

| Env variable | Default | Purpose |
|---|---|---|
| `CHQ_SERVER_MAX_BODY_BYTES` | `33554432` | Largest accepted request body |
| `CHQ_SERVER_MAX_IMPORT_BYTES` | `33554432` | Largest accepted import, must not exceed the above |
| `CHQ_SERVER_READ_TIMEOUT` | `30s` | Deadline for reading a request |
| `CHQ_SERVER_IDLE_TIMEOUT` | `120s` | How long an idle keep-alive connection is held |
| `CHQ_CARDDAV_MAX_RESOURCE_BYTES` | `1048576` | Largest accepted single vCard |
| `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS` | `false` | Permit `http://` provider endpoints |

Changed defaults: `CHQ_AUTH_TOKEN_TTL` `24h` → `1h`, `CHQ_AUTH_REFRESH_TTL` `720h` → `168h`.

### Upgrading

No migrations. Before starting the new version:

1. Set `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true` if any provider endpoint is plain `http`.
2. Remove `server.write_timeout` if you set it — the server will not start with it.
3. Raise `CHQ_SERVER_MAX_IMPORT_BYTES` if you import files larger than 32 MiB.

## [0.3.0] — 2026-08-06

A hardening and observability release. Two defects in here broke shipped features outright,
and one changes a default in a way that requires action on some deployments.

### ⚠️ Breaking

- **Public registration is closed by default.** `POST /auth/register` now returns `403` once
  the instance has an account. Creating the *first* account is still always allowed, so a new
  deployment bootstraps exactly as before, and administrators can add users from
  **Admin → Users** at any time.
  **If your instance relies on open sign-up, set `CHQ_AUTH_ALLOW_REGISTRATION=true` before
  upgrading.** Previously any account was available to anyone who could reach the port.
  `GET /auth/config` reports `{"registration_open": bool}` without authentication.

### Fixed

- **Export to a password-authenticated CardDAV server never worked.** One of the two provider
  constructors left `baseURL` and `httpClient` unset, so the first conditional PUT
  nil-panicked — and the panic was swallowed by the worker's `recover()`, leaving no visible
  error. Both constructors now share one assembly path.
- **Contact photos were corrupted on write.** vCard values were escaped uniformly regardless
  of type, producing `PHOTO:data:image/jpeg;base64\,…` — a payload iOS cannot render. Also
  fixed: `CATEGORIES:work\,friends` collapsing two categories into one, structured values
  (`N`, `ADR`, `ORG`) having their separators escaped, and a bare carriage return travelling
  into the output and making a card unparseable. See `reencode-vcards` below for already-stored
  cards.
- **Per-field choice in the merge screen did nothing.** The UI sent keys like `first_name` and
  `email`; the server resolved by vCard property name, so no key ever matched and the winner's
  value always won. Merging now works per *value*, so "the work address from one record and the
  home address from the other" is expressible.
- **Merging wrote through a path that skipped seven child tables** and did not advance the
  collection's change counter, so CardDAV clients never learned the surviving contact had
  changed. A merge is now one transaction that also tombstones the removed card.
- **The duplicate list could not be paged past its first twenty pairs.** A request for more was
  reset to the minimum instead of clamped to the maximum, making a pair at position 21
  unreachable from the merge screen. Clearing the status filter was equally impossible: an
  empty value was silently replaced by `pending`.
- A developer's local `configs/config.yaml` was baked into the Docker image, leaking its
  secret and silently overriding every value passed through the environment.
- Internal error text (driver messages, file paths) was returned to API clients verbatim.
- A backup could be listed and restored while it was still being written; a partially written
  file is no longer visible until it is complete.
- Shutting down interrupted the job in flight instead of letting it finish, so a backup caught
  by a restart was aborted mid-write.
- `Enqueue` blocked forever on a full queue from the scheduler's goroutine, stalling every
  later scheduled job.
- The avatar on the contact detail page always rendered "?".

### Added

- **`contactshq set-password <email>`** — recover a forgotten password from the machine running
  the server. The password is never taken as a command-line argument. There is still no email
  reset; the server sends no mail.
- **`contactshq reencode-vcards`** — rewrites stored cards with the corrected encoder. Runs as
  a dry run by default and refuses to do half the job: `--apply` requires
  `--reconcile-sync-state`, without which the next export would push the entire address book
  to Google or CardDAV. Take a database dump first; there is no undo.
- **Backup history.** Every attempt is recorded, including manual ones, and the Backup screen
  opens with one line: healthy, failing, overdue, never run, or off — with the error from the
  last failure. `GET /backup/runs` and `GET /backup/status` expose the same data.
  Files on disk could never answer "when did this last succeed": retention deletes them.
- **Interrupted runs are closed at startup**, so a killed container stops leaving history that
  reads "still running" forever.
- **A missed backup is caught up at startup.** If the machine was off overnight, cron alone
  would not help — the next firing is tomorrow and the day's backup is simply gone.
- **Merge history** (`GET /contacts/merge-log`), with a snapshot of the discarded card so a
  merge can be undone by hand. Kept for 30 days by default.
- Per-value merge screen with a result preview and an explicit list of what will be discarded.
- `GET /contacts/duplicates/:id` returns one pair with every value of both contacts.
- The **Sync providers** screen is now reachable. It existed with no route at all, which made
  every `/sync/providers` endpoint invisible from the UI.

### Changed

- **Duplicate detection is roughly 4800× faster.** The scan compared every pair and computed a
  Levenshtein distance for each, then discarded the result: the score threshold was reachable
  only through an exact email or a normalised phone match. Grouping by those keys produces the
  same pairs. On 10 000 contacts a scan went from 39 seconds and 13.6 GB of allocations to
  8 milliseconds and 4.9 MB.
- CardDAV Basic-auth is now bounded: at most four concurrent argon2id verifications, and a
  per-address failure limit. Every miss previously cost a 64 MiB hash, multiplied by the number
  of app passwords on the account, with no rate limiting on `/dav` at all. There is deliberately
  no per-email limit — the email is the CardDAV login and is usually public, so a counter on it
  would be a ready-made way to lock the owner's phone out of sync.
- Changing a password or deleting an app password now takes effect for CardDAV immediately
  rather than after up to five minutes.
- At most 20 app passwords per account.
- The duplicate list explains itself: "Same email: a@b.c" rather than a percentage that only
  ever had two values, and the one-click "keep this one" buttons appear only when the server
  has confirmed the other record holds nothing extra.
- Restore reads at most 128 MiB decompressed (`CHQ_BACKUP_MAX_RESTORE_BYTES`), rejecting an
  oversized archive before deleting anything.
- Sync HTTP requests have a timeout. A host that accepted the connection and then went silent
  used to park a worker goroutine forever — four of those stopped backups and deduplication too.

### Database

Three migrations, applied automatically at startup: `023_merge_log`,
`024_potential_duplicates_unique`, `025_backup_runs`.

Migrations are forward-only — `.down.sql` files exist but nothing applies them. Take a dump
before upgrading.

### Configuration

New keys, all optional:

| Env variable | Default | Purpose |
|---|---|---|
| `CHQ_AUTH_ALLOW_REGISTRATION` | `false` | Open public sign-up |
| `CHQ_BACKUP_MAX_RESTORE_BYTES` | `134217728` | Cap on decompressed restore size |
| `CHQ_MERGE_LOG_RETENTION_DAYS` | `30` | How long merge history is kept |
| `CHQ_SYNC_RUNS_RETENTION_DAYS` | `90` | How long pipeline run history is kept |

### Upgrading

1. Take a database dump.
2. Set `CHQ_AUTH_ALLOW_REGISTRATION=true` if your instance relies on open sign-up.
3. Start the new version; migrations run automatically.
4. Optionally repair previously stored vCards — during a maintenance window, with pipelines
   stopped:
   ```bash
   ./contactshq reencode-vcards                                 # dry run
   ./contactshq reencode-vcards --apply --reconcile-sync-state
   ```
   Every CardDAV client will re-download the address book afterwards, because rewritten cards
   get new ETags.

## [0.2.0]

Incremental sync (RFC 6578 CTag and `sync-collection`, Google sync tokens), conditional
writes, and trusted-proxy support for per-client rate limiting.

## [0.1.0]

First tagged release.

[Unreleased]: https://github.com/gumeniukcom/contactshq/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.5.0
[0.4.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.4.0
[0.3.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.3.0
[0.2.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.2.0
[0.1.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.1.0
