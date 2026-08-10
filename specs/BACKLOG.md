# Backlog

Every entry here comes from a `## Known Divergences` section in one of the nine specs. 184 of
them were triaged on 2026-08-10: **70 deliberate** (a decision whose reason still holds — no
action), **75 fix**, **25 decide**, **14 stale**.

This file is an **index with priorities, not a second source of truth**. The authoritative wording
of each defect stays in the owning spec's `## Known Divergences`; when the two disagree, the spec
wins and this file is what gets corrected. Delete an entry when it ships — do not leave it here
marked done, or this becomes a changelog with worse provenance.

Ranked by what a user of a single-instance self-hosted deployment actually suffers, divided by
effort. Not by how interesting the bug is.

## Closed on 2026-08-10, while writing this

Three were too severe to file:

- **Export and backup fused stored vCards into one card.** An address book populated by import
  exported to a file that re-imported as **0 of 3** contacts, and a `replace` restore deleted all
  three and put one back. Fixed in `935cd0f`. Found by the critic reviewing this triage — no spec
  admitted it.
- **A clean checkout did not compile.** `internal/web/static/spa/.gitkeep` was deleted in
  `a0ab02f`; CI stayed green because both Go jobs recreate it. Fixed in `88e70af`.
- **The ownership gate could be disarmed by prose.** `unclaimed()` read every backtick in
  `UNCLAIMED.md`, and the word `web/src` inside one row's *reason* exempted all 106 files under
  it. The gate reported green over the largest tree in the repository. Tightened to read the
  table's first column only; the first run then found two genuinely unclaimed files.

## Fix

Effort is S (an afternoon), M (a day or two), L (more).

| # | What | Effort | Why it ranks here |
|---|---|---|---|
| 1 | Empty contact list serialises as `null` and blanks the list view | S | The only entry a user hits on an ordinary day with no unusual configuration: **any search that matches nothing**. `bun_contact.go:160-164` returns a nil slice; `ContactListView.vue:88` reads `.length` off it |
| 2 | Search is case-sensitive on PostgreSQL — the engine `docker compose` ships | S | Bare `LIKE` (`bun_contact.go:172-182`). SQLite folds ASCII case; PostgreSQL does not. `john` does not find `John Smith` on the deployment the project itself provisions |
| 3 | vCard write paths lose data | S | Three defects, one shape — a write path discards bytes it did not model. A flat `PUT` rebuilds the card and deletes PHOTO, KEY and every X- property (`contact.go:215`); a truncated file yields one card fewer with no error (`split.go:46-51`), including during a restore that has already deleted the originals; a CardDAV PUT stores a card whose inner UID differs from the path it is keyed on (`backend.go:263-274`) |
| 4 | A browser edit destroys the preferred email or phone | S | `toFieldsPayload` emits only `{value, type}` (`contact-form.ts:29-33`) while `builder.go:107` stamps PREF on the first row. A Google contact whose preferred address is its second one loses that on the first save — and pushes the loss back out on the next sync. Same shape as the GEO bug fixed on 2026-08-08 |
| 5 | `reencode-vcards` repairs the database and no CardDAV client ever sees it | S | Writes through `db.NewUpdate`, bypassing `nextChangeSeq` (`reencode.go:161-168`). The CTag **is** the address book's `change_seq`, so a CTag-polling client (iOS) never asks — while the command prints "Every CardDAV client will re-download the whole address book". The one maintenance command the constitution holds up as its worked example does not reach the clients it exists to repair |
| 6 | Two paths delete files they did not create | S | `make clean` runs `rm -f contactshq.db`, which is the default production DSN. Backup retention deletes any `.vcf` in the backup directory, and spec 005's own Independent Test tells the operator to put one there |
| 7 | Refuse a second pipeline against the same provider type | S | Sync state is keyed on provider name, so two CardDAV pipelines share one cursor and one conflict queue, and the engine reads the crossover as mass local change. A 400 at save is safe under either answer to Q4 below, so it should not wait on it |
| 8 | Merge is advertised as lossless and is not, in three ways | M | The subset check ignores the flat `contacts.email`/`phone` columns the detector unions in, so a pre-014 contact is a vacuous subset and quick merge discards its only email; unlisted properties (PHOTO) are destroyed without appearing under "Will be discarded"; a winner edited between load and confirm can produce a card with no FN |
| 9 | The import screen reports a clean import that partly failed | S | Server sends `errors` as an int, the SPA declares `string[]` and guards on `.length`, so the block never renders. `Skipped` is rendered from a field nothing increments |
| 10 | Error bodies leak internals or carry the wrong status where the central handler is bypassed | S | Three paths never reach `newErrorHandler`. `/dav` returns 500 with the message for a card deleted between polls — which clients retry forever instead of settling on the 404 |
| 11 | Warn when `skip_tls_verify` is used | S | After 0.5.0 refuses stored `http://` credentials, ticking this box is the operator's path of least resistance — same exposure, now invisible, and `ValidateProviderEndpoint` never sees it. *(Added by the triage critic; the triage itself missed it.)* |
| 12 | Unbounded argon2id concurrency on `/auth/login` and `/auth/register` | M | The 4-slot semaphore exists only in `carddav.Server`. Ten concurrent requests from one IP is a ~640 MiB spike, and the shared rate limiter permits it |
| 13 | The duplicates screen swallows every failure | S | `runDetect` and `fetchDuplicates` have `finally` and no `catch`; `dismiss` has neither. The button stops spinning and nothing happens. `quickMerge` already does it correctly in the same file |
| 14 | A backup has no test that it can be restored | M | The only `.vcf.gz` any test reads is one it wrote itself. A regression in the compressed writer produces backups discovered as unrestorable on the day one is needed. Retention has no ordering assertion, so a sort that inverted would delete the newest and keep the oldest, silently |
| 15 | Invariants held by convention that should be held by construction | M | The ETag/ContentHash formula is copied to **thirteen** sites; two argon2id verifiers exist for a stated reason that does not hold; the password minimum is a literal beside the constant meant to hold it; `gvcard.NewEncoder` has no lint rule despite Principle V |
| 16 | The handler layer has three test files for seventy-eight routes | M | Every user-visible defect on this list lives at a boundary these tests do not cross. The service and repository layers are well covered, which is why the bugs are not there |
| 17 | Papercuts (13 items) | S | One afternoon, one commit. Includes: `?ids=` silently ignored by CSV and JSON export; `CHQ_SERVER_WRITE_TIMEOUT` missing from `envBoundKeys` so it is silently ignored; `bodylimit.go` matching with `strings.Contains`; `recordMerge` writing history before the merge that may fail; a dead `/docs/reverse-proxy.md` link in the setup guide; CI's `-run TestPostgres` filter meaning any PostgreSQL test named otherwise never runs |

## Decide

Sixteen questions where the trade-off is real and the answer is the maintainer's. Each is stated
with what goes wrong under either choice; the full text is in the triage transcript.

The three worth answering first, because other work waits on them:

1. **How does an instance recover if its only administrator is demoted or deleted?** Nothing
   validates that one remains, and no CLI path grants the role back — recovery today is editing
   the database by hand.
2. **How is the first account created on a fresh install?** Verified: the SPA has no sign-up form
   and never calls `/auth/config`, while `README.md` says "The first account you register becomes
   the administrator". The endpoint, the policy code and the README describe a flow no shipped
   client performs.
3. **Does a saved cron expression mean UTC or the process's local timezone?** The scheduler uses
   `time.Local`; the UI labels the field "(UTC)". Containers happen to be UTC, so this only bites
   a bare binary in another zone.

The rest, in brief: is quick merge allowed to say "provably lossless" when the proof covers only
emails and phones; is `carddav.max_resource_bytes` a database-wide invariant or an upload policy;
does `carddav.path_prefix` earn its keep when two onboarding surfaces hard-code `/dav`; is a
wholesale-supplied vCard stored verbatim forever or normalised on ingest; should the landing page
work offline; is cross-origin API access supported at all; should a restore that wrote everything
but failed to reconcile report success-with-warning or failure; should the JSON API reject unknown
keys; is a syntactic email guard worth adding; is merge history user-facing or an operator tool;
should OAuth `state` be bound to the initiating session; and does a shared registry like the route
table get one owner of record or an explicit exception to Principle VII.

## Stale

Twenty spec entries describe behaviour the code no longer has. These are corrections to the
**specs**, not to the code, and they are the cheapest work here.

Six of them were written by the agents that made the 2026-08-08 fixes, and claim more than the
code delivers — including one asserting `.gitkeep` is committed when the file did not exist. The
constitution's answer to laundering an accident into a requirement is `## Known Divergences`; it
has no answer for a fixer grading their own fix.

## Themes

Patterns worth more than the sum of their instances:

- **Nil is not empty at the JSON boundary.** A Go nil slice serialises as `null` and every
  consumer that reads `.length` breaks. It blanked both conflict screens in August and is doing
  it to the contact list now. Normalise at the repository, not at each caller.
- **The SPA's TypeScript types are hand-written fiction and nothing compares them to the wire.**
  Every user-visible defect here that is not data loss is an instance of this.
- **One formula, many copies.** Principle V says one writer for vCard encoding and generalises
  nowhere — the ETag derivation lives at thirteen sites.
- **Principle III holds only where the central error handler runs.** It is written as a property
  of the process and implemented as a property of one middleware.
- **Destructive operations that do not close their own loop.** The write half is careful; the
  bookkeeping half was an afterthought.
- **The handler layer is where users meet the software and where the tests stop.**
- **Ownership enforced by a remembered line rather than by construction.** None is a live
  vulnerability on a single-tenant instance — which is why they will survive until that changes.
- **Self-reported amendments overstate what was fixed.** A second reader, or the gate, has to
  check the amendment and not just the code.
