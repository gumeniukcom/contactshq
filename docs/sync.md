# How synchronisation works

ContactsHQ keeps one address book — the one it stores — in step with external
providers through **pipelines**. This document explains what a pipeline does, how
conflicts are resolved, how incremental sync avoids re-downloading everything, and
which safeguards protect your data.

## Providers

| Provider  | Role                                   | Incremental |
|-----------|----------------------------------------|-------------|
| `internal`| Your ContactsHQ address book           | —           |
| `carddav` | Any CardDAV server (Fastmail, iCloud, Nextcloud, another ContactsHQ) | Yes, when the server supports RFC 6578 |
| `google`  | Google Contacts via the People API     | Yes         |

Every pipeline step pairs one external provider with your internal address book. The
internal book is always the destination; the direction decides which way contacts move.

## Where a pipeline may connect

A sync request carries the provider's username and password, so the endpoint is checked
before anything is fetched. Accepted: an absolute `https` URL with a host and no credentials
in its userinfo. Refused: `http` (unless you opt in), `file://`, `gopher://`, a bare hostname,
and a URL of the form `https://user:pw@host/` — those credentials would be logged and stored
alongside the endpoint.

The check runs at every point an endpoint can enter the system, because there are four and
validating only the obvious one leaves the rest open:

1. CardDAV connect,
2. a stored credential,
3. the endpoint inside a pipeline step's provider config,
4. an endpoint posted to a manual trigger.

Private and link-local addresses are **not** filtered. A CardDAV server on the local network
is an ordinary, supported setup, and filtering addresses would not close the hole it appears
to: a permitted public host can answer `302 Location: http://169.254.169.254/` and the client
would follow it. That is handled where it actually happens — the sync client follows at most
**three** redirects and refuses to cross to a different host. Same-host redirects still work,
which matters because that is how many servers implement `.well-known/carddav`.

To sync against a server reachable only over plain http:

```bash
CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true
```

Prefer fixing the transport where you can. The setting is all-or-nothing, and the password
travels in every request the pipeline makes.

## Directions

A step has one of three directions:

- **import** — the external provider's changes flow into ContactsHQ.
- **export** — your ContactsHQ changes flow out to the provider.
- **two_way** — an import followed by an export.

The pipeline form shows this as `Import (CardDAV → ContactsHQ)`, and so on. (The engine
still accepts the older `pull` / `push` / `bidirectional` names for compatibility.)

## Conflict resolution

When a contact changed on **both** sides since the last sync, ContactsHQ resolves it
according to the step's conflict mode:

- **auto** — a three-way merge against the last-synced version. Non-overlapping edits
  (you added a phone, they added an email) merge cleanly. Overlapping edits that cannot
  be merged are queued for review.
- **source_wins** — the provider's version wins.
- **dest_wins** — the ContactsHQ version wins.
- **skip** — the contact is left as-is and skipped.
- **manual** — always queued for review; ContactsHQ never merges on its own.

Queued conflicts appear under **Sync → Conflicts**, where you resolve each field by
choosing the local or remote value. The resolution is written to the contact and the
sync state is advanced, so the next run does not re-detect it.

## Incremental sync

Re-listing an entire address book on every run is wasteful and, for Google, burns API
quota. When a provider supports it, ContactsHQ fetches only what changed since the last
run, using an opaque **cursor** stored per pipeline.

- **Google** uses a People API sync token. Contacts Google marks as deleted are applied
  as deletions. If Google reports the token as expired (HTTP 410), ContactsHQ discards it
  and resynchronises in full.
- **CardDAV** uses RFC 6578 `sync-collection`, then a `MultiGET` for the changed cards.
  A server that does not implement `sync-collection` falls back to a full listing
  automatically. A token the server rejects triggers a full resync.

On the way **out** (export / two_way), ContactsHQ writes conditionally — `If-Match` for
CardDAV, an ETag for Google — so the server itself rejects a write that would overwrite a
change someone else made in the meantime. That contact becomes a conflict instead of
being clobbered, and the whole remote collection no longer has to be downloaded first
just to compare.

### As a CardDAV server

ContactsHQ's own CardDAV endpoint also speaks the CTag extension and `sync-collection`,
so phones and desktop clients syncing **from** ContactsHQ fetch only what changed. No
configuration is needed; clients negotiate it automatically.

## Safeguards

- **Mass-deletion guard.** A run that would delete more than half of at least five tracked
  contacts is aborted with an error rather than carried out. A truncated or expired
  provider response, or a bug in delta parsing, looks exactly like "the user deleted
  everything" — the guard caps the blast radius of all of them. It stays active in
  incremental mode for the same reason. If you really did delete most of an address book,
  re-run the pipeline after the guard trips, or clear the pipeline's sync state.

- **Restore reconciliation.** Restoring a backup drops the sync state of contacts the
  backup did not bring back, so the next export does not read them as "deleted locally"
  and remove them from the remote provider.

- **Identity preservation.** A contact created locally and pushed out is tracked by the id
  the provider assigns it (Google returns its own `resourceName`), not the local id, so
  the next run does not mistake it for a new contact and duplicate or delete it.

- **A silent provider cannot park a worker.** Every sync HTTP request has a 30-second
  timeout. A host that accepted the connection and then said nothing used to hold its worker
  goroutine forever; four of those and scheduled backups and duplicate detection stopped too,
  because they share the queue.

- **Run history is pruned.** `sync_runs` gains a row per pipeline execution and would grow
  without bound. Rows older than `CHQ_SYNC_RUNS_RETENTION_DAYS` (default 90) are removed at
  startup.

## Limitations

- The Google sync-token and conditional-write paths are covered by unit tests but have not
  been exercised against live Google.
- CardDAV incremental sync assumes a server names each contact's file after its UID, which
  is the common convention. A server that does not is still synced correctly on a full
  listing; only the delta's deletion matching depends on it, and an expired-token resync
  reconciles any drift.
- The job queue is in-memory: a sync scheduled moments before a restart is drained on
  shutdown, but a hard kill (SIGKILL) drops it. It runs on the next scheduled tick.

## Things that moved in v0.4.0

**Provider endpoints are validated, and plain http is refused by default.** See
[Where a pipeline may connect](#where-a-pipeline-may-connect) for what the check accepts.

What this means for a pipeline that already exists depends on where its endpoint is written:

- **Inline in the step's config** — the endpoint is re-checked on every run, so a step
  pointing at `http://` fails until `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true` is set. The
  failure is recorded against that step alone; other steps in the same pipeline still run.
- **In a stored credential** (`credential_id`) — the row is used as it stands. Nothing
  rewrites your database on upgrade, and the run-time path resolves the credential *after*
  the step check, so a credential saved before 0.4.0 keeps working over http. Saving or
  editing it puts it through the check, and it cannot be stored again without the opt-in.

So the validation bounds what can be *entered*, and grandfathers what was already there.
If you want the older rows held to the same rule, open each stored credential and save it:
one that no longer passes will tell you so.

## Things that moved in v0.3.0

Two changes in this release alter bytes the engine compares, so they are worth knowing about
before debugging a surprising resync.

**The vCard encoder changed.** Escaping is now chosen per value type, which changes the exact
bytes of any card holding a comma in a URI, a category separator, a semicolon in a structured
value, or an embedded newline. `cardToString` is applied to data read from the *remote* side
too, so `ContentHash` (engine.go) and `LocalETag` both move for those cards. The first sync
after upgrading can therefore look like "the remote changed" for cards with such values. The
mass-delete circuit breaker does not cover this — it counts deletions only.

Cards already stored are not rewritten automatically; `contactshq reencode-vcards` does that,
and its `--reconcile-sync-state` step exists precisely to stop the next export from pushing the
whole address book outward.

**Merging writes differently.** `ContactRepository.MergeInto` saves the surviving contact and
deletes the merged-away one in a single transaction, under one change sequence, and writes the
loser's tombstone itself. A client synchronising across a merge sees exactly two entries — one
update and one deletion — rather than a window in which the contact exists twice or not at all.
