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
| 1 | vCard write paths lose data | S | Three defects, one shape — a write path discards bytes it did not model. A flat `PUT` rebuilds the card and deletes PHOTO, KEY and every X- property (`contact.go:215`); a truncated file yields one card fewer with no error (`split.go:46-51`), including during a restore that has already deleted the originals; a CardDAV PUT stores a card whose inner UID differs from the path it is keyed on (`backend.go:263-274`) |
| 2 | A browser edit destroys the preferred email or phone | S | `toFieldsPayload` emits only `{value, type}` (`contact-form.ts:29-33`) while `builder.go:107` stamps PREF on the first row. A Google contact whose preferred address is its second one loses that on the first save — and pushes the loss back out on the next sync. Same shape as the GEO bug fixed on 2026-08-08 |
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

## Decided (2026-08-11)

All sixteen open questions were answered by the maintainer. The work each one creates is listed
here rather than in the table above, because these are decisions first and tasks second — a
future reader needs the choice and its reason, not just the ticket.

| Question | Decision | Work it creates |
|---|---|---|
| Recovery when the only admin is lost | **Guard *and* CLI.** Each alone leaves a hole: the guard protects new instances, the command repairs bricked ones | Count remaining admins in `UpdateRole`/`DeleteUser`; add `contactshq set-role <email> <role>` |
| First account on a fresh install | **Build the bootstrap sign-up form**, driven by `/auth/config` | A screen that is correct exactly once in an instance's life; makes the README true |
| Cron timezone | **UTC.** The label already says so | `WithLocation(time.UTC)`; breaking note for bare binaries in another zone |
| Quick merge "provably lossless" | **Widen the proof** to every field a merge can discard | Extend `subsetExpr` beyond emails and phones; fewer pairs will offer the shortcut |
| `carddav.max_resource_bytes` | **Upload policy, not a database invariant.** A PUT no larger than what is already stored is accepted | Compare against the stored size before refusing; ends the unwritable-contact trap |
| OAuth `state` | **Bind it to the initiating user** | Store the user id with the connection, compare in the callback |
| Restore that failed to reconcile | **Success with a loud warning**, not 500 | `RestoreResult.warning`, surfaced prominently by the SPA |
| Wholesale vCard on ingest | **Store verbatim.** A hub accepts; it does not nitpick | `reencode-vcards` is permanent — say so in its help and in spec 003 |
| Unknown JSON keys | **Reject with 400.** A failure shaped like success is the worst kind | `DisallowUnknownFields` on the contact create/update path |
| CORS | **Drop `cors.New()`** — nothing in the product needs it | Delete the middleware, reword FR-039 |
| Changing an email | **Require the current password** — the only check that also catches a valid-but-wrong address | Add the confirmation to `PUT /users/me` |
| Merge history | **Operator tool.** Reword Story 4 rather than build a screen for a rare recovery | Spec 007 text only |
| `carddav.path_prefix` | **Freeze at `/dav`, delete the key** — a knob that today only breaks onboarding | Remove the config key and its uses |
| Tailwind CDN | **Embed a hand-written stylesheet** for the two server-rendered pages | A product that does not phone home must not phone home for its own UI |
| Shared registries | **One owner of record, read-only citations elsewhere.** Principle VII keeps no holes | Write the rule into `specs/README.md` |
| Toolkit pin `0.15.1.dev0` | **The committed tree is authoritative.** It is vendored, offline-capable and matches five sibling repositories | Record it; stop reading the version number as a reproducibility promise |

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
