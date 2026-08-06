# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
uses [Semantic Versioning](https://semver.org/). While the major version is `0`, breaking
changes may appear in a minor release.

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

[0.3.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.3.0
[0.2.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.2.0
[0.1.0]: https://github.com/gumeniukcom/contactshq/releases/tag/v0.1.0
