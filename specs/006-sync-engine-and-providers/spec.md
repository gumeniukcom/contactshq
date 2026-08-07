# Feature Specification: Sync Engine, Pipelines & Provider Connections

Kind: journey
Status: shipped
Constitution: v1.0.0

Reconstructed from the implementation at commit `23a167c` (`v0.4.0-3-g23a167c`; `v0.4.0` is the
last tag). Every requirement below was checked against the code at that commit and the
parenthetical names the file that implements it. Where the code does something the narrative
documentation claims it does not, or does not do something the documentation claims it does, the
difference is recorded under `## Known Divergences` rather than smoothed into a requirement.
`docs/sync.md` is listed under `## References` and was used only as a map; every one of its
claims that became a requirement here was re-read in the source first, and two of them did not
survive that check (see `## Known Divergences`).

## User Scenarios & Testing *(mandatory)*

The actor throughout is a signed-in account holder synchronising their own address book.
There is no cross-user or administrative surface in this domain: every handler scopes by
`c.Locals("userID")` and every repository query carries a `user_id`.

### User Story 1 - Import someone else's address book into mine (Priority: P1)

A user runs a CardDAV or Google address book elsewhere and wants its contacts to appear in
ContactsHQ, kept up to date on a schedule without their attention.

**Why this priority**: It is the reason the domain exists. Everything else — conflicts,
cursors, endpoint checks — is machinery in service of this one outcome, and none of it is
reachable without it.

**Independent Test**: Save a credential pointing at a CardDAV server, create a pipeline with
one `carddav → internal`, `direction: import` step, trigger it, and confirm the contacts
appear in the contact list with a `sync_runs` row recording the counts.

**Acceptance Scenarios**:

1. **Given** a pipeline step whose source is `carddav` and whose destination is `internal`,
   **When** it runs for the first time, **Then** every remote contact is created locally and
   a `sync_states` row records the pairing of remote id to local id
   (`internal/sync/engine.go` `pullPhase`).
2. **Given** a subsequent run where nothing changed remotely, **When** it executes, **Then**
   no local contact is written, because the run compares both the remote ETag and a SHA-256
   hash of the card body before acting (`internal/sync/engine.go`, `remoteModified`).
3. **Given** a contact deleted on the remote, **When** the next run executes, **Then** the
   local copy is deleted and the `sync_states` row is removed — provided the deletion passes
   the safety check in Story 6 (`internal/sync/engine.go`).
4. **Given** the run completes, **When** the user opens the pipeline, **Then** a run row
   shows created/updated/deleted/error counts and a start and finish time
   (`internal/sync/engine.go` `Sync`, `GET /pipelines/:id/runs` in
   `internal/handler/pipeline_handler.go`).

---

### User Story 2 - Push my contacts outward, without losing their identity (Priority: P1)

A user edits contacts in ContactsHQ and wants those edits to reach the provider — including
contacts that were created here and have never existed on the remote.

**Why this priority**: Equal to Story 1 because a half-duplex sync is a trap: a user who
believes both sides are converging and finds only one is loses work. The identity rule below
is what makes repeated export runs idempotent rather than destructive.

**Independent Test**: Create a contact locally, run an `export` step twice, confirm the
provider holds exactly one copy and that the second run reports zero changes.

**Acceptance Scenarios**:

1. **Given** a local contact with no `sync_states` row, **When** an export runs, **Then** it
   is written to the provider and tracked under **the id the provider returned**, not the id
   sent (`internal/sync/provider.go` `PutResult`, `internal/sync/engine.go` `pushPhase`).
2. **Given** that contact and a subsequent `two_way` run, **When** the import phase lists the
   remote, **Then** the contact is recognised as already tracked and is neither duplicated nor
   deleted (`internal/sync/engine.go` `pullPhase`).
3. **Given** a contact deleted locally, **When** an export runs, **Then** it is deleted on the
   provider addressed by its *remote* id (`internal/sync/engine.go` `pushPhase`).
4. **Given** a provider that supports conditional writes, **When** a changed contact is
   pushed, **Then** the whole remote collection is not downloaded first — the write itself
   carries the precondition (`internal/sync/engine.go` `pushPhase`).

---

### User Story 3 - Decide what happens when both sides changed (Priority: P1)

Two people edited the same contact — one in ContactsHQ, one in the provider. The user wants
the non-overlapping edits kept and to be asked about the rest, rather than silently losing
one side.

**Why this priority**: This is the failure that costs data rather than time. A sync that
clobbers is worse than no sync, and the three-way base is the only thing that lets the system
tell "changed" from "different".

**Independent Test**: Change the name locally and the phone number remotely, run an `auto`
step, confirm both survive in one card and no conflict is queued. Then change the *same*
field on both sides and confirm a pending conflict appears at `/app/sync/conflicts`.

**Acceptance Scenarios**:

1. **Given** disjoint field changes on the two sides and `conflict_mode: auto`, **When** the
   run executes, **Then** a three-way merge against `sync_states.base_vcard` produces one card
   holding both changes and no conflict is recorded (`internal/sync/merger.go` `MergeVCards`).
2. **Given** the same field changed differently on both sides, **When** `auto` runs, **Then**
   the contact is skipped and a pending conflict row is written carrying base, local and
   remote cards plus a per-field diff list (`internal/sync/engine.go` `recordConflict`,
   `internal/domain/sync_conflict.go`).
3. **Given** `conflict_mode: manual`, **When** any divergence is found, **Then** it is queued
   without ever attempting a merge (`internal/sync/engine.go` `pullPhase`).
4. **Given** an unresolved conflict and several scheduled runs, **When** each run re-detects
   it, **Then** the existing row is updated in place rather than a new row appended
   (`internal/sync/engine.go` `pendingConflictsByRemoteID`).
5. **Given** a pending conflict, **When** the user picks local or remote per field and saves,
   **Then** the merged card is written to the contact, the sync state adopts it as the new
   base, and the conflict is marked resolved (`internal/service/sync_conflict.go` `Resolve`,
   `web/src/views/sync/SyncConflictDetailView.vue`).
6. **Given** a resolution, **When** the next run executes, **Then** it does not re-raise the
   same conflict — `RemoteETag` is advanced to the value seen at detection and `LocalETag` is
   cleared so the resolution is pushed outward
   (`internal/service/sync_conflict.go` `advanceSyncState`).

---

### User Story 4 - Connect a provider without handing this server a weapon (Priority: P1)

A user pastes a provider URL. The deployment must fetch only URLs it is willing to fetch,
because this server will make that request on the user's behalf with credentials attached.

**Why this priority**: P1 because it is a security boundary, not a convenience. Before v0.4.0
the URL was not checked at all, so `file:///etc/passwd` and `http://169.254.169.254/` were
both acceptable (`CHANGELOG.md`, 0.4.0 Breaking).

**Independent Test**: Attempt to save a credential with each of `file://`, a bare hostname,
`https://user:pw@host/` and `http://host/`; confirm all four are refused with a 400 and that
`http://` becomes acceptable only after `CHQ_SYNC_ALLOW_INSECURE_ENDPOINTS=true`.

**Acceptance Scenarios**:

1. **Given** any of the four ways an endpoint can enter the system, **When** a URL is
   submitted, **Then** the same check runs: CardDAV connect
   (`internal/handler/sync_handler.go` `CardDAVConnect`), a stored credential
   (`internal/handler/credential_handler.go` `Create` and `Update`), the endpoint inside a
   pipeline step's JSON (`internal/sync/pipeline.go` `ValidateStep` →
   `ValidateStepEndpoints`), and the endpoint posted to a manual trigger
   (`internal/handler/sync_handler.go` `CardDAVTrigger`).
2. **Given** a scheme other than http or https, **When** it is submitted, **Then** it is
   refused (`internal/sync/endpoint.go`).
3. **Given** a permitted host that answers `302` pointing at a different host, **When** the
   client follows it, **Then** the redirect is refused — validating the string alone would
   not have stopped it (`internal/sync/endpoint.go` `redirectPolicy`).
4. **Given** a redirect within the same host, **When** discovery follows it, **Then** it
   works, because `.well-known/carddav` is normally exactly that
   (`internal/sync/endpoint.go` `redirectPolicy`).
5. **Given** a private-network address such as `https://192.168.1.10/dav/`, **When** it is
   submitted, **Then** it is accepted: LAN CardDAV is a supported setup and is deliberately
   not filtered (`internal/sync/endpoint.go` header comment).

---

### User Story 5 - Build and schedule a pipeline (Priority: P2)

A user assembles one or more steps, chooses a direction and a conflict policy, gives it a
cron schedule, and can also run it on demand.

**Why this priority**: The scheduling wrapper around Stories 1–3. Valuable, but a user with
no pipeline can still sync CardDAV through the manual trigger, so it ranks below the engine
itself.

**Independent Test**: Create a pipeline through the UI with a schedule, confirm it appears in
the list with a humanised schedule, trigger it manually, and confirm the run appears in its
history without restarting the server.

**Acceptance Scenarios**:

1. **Given** a step whose destination is not `internal`, or whose source is `internal`, or
   which names two external providers, **When** the pipeline is saved, **Then** it is refused
   with a 400 naming the step number (`internal/sync/pipeline.go` `ValidateStep`,
   `internal/handler/pipeline_handler.go` `validateSteps`).
2. **Given** a direction of `import`, `export` or `two_way` — or the legacy `pull`, `push`,
   `bidirectional` — **When** the step runs, **Then** it parses to the same three modes; any
   other value is an error rather than a silently empty run (`internal/sync/engine.go`
   `ParseSyncMode`).
3. **Given** an invalid cron expression, **When** the pipeline is saved, **Then** it is
   refused (`internal/handler/pipeline_handler.go`, `worker.ValidateCron`).
4. **Given** a pipeline created, edited or deleted, **When** the request returns, **Then** the
   scheduler's job set already reflects it — no restart is required
   (`internal/handler/pipeline_handler.go` calling `RegisterPipelineJob` /
   `ReregisterPipelineJob` / `RemovePipelineJob`, `internal/worker/scheduler.go`).
5. **Given** a step that fails to build a provider or fails validation, **When** the pipeline
   runs, **Then** the remaining steps still execute and the failure is reported per step
   (`internal/sync/pipeline.go` `Execute`, which appends a `StepResult` with `Error` and
   continues).
6. **Given** run history accumulating over months, **When** the server starts, **Then** rows
   older than `sync.runs_retention_days` are deleted. The setting is this spec's
   (`internal/config/config.go` `SyncConfig`, `internal/repository/bun_sync_run.go`
   `DeleteOlderThan`); the boot-time step that applies it belongs to
   `008-runtime-configuration-and-delivery`.

---

### User Story 6 - Not lose the address book to a provider having a bad day (Priority: P1)

A provider returns an empty or truncated listing. The user's address book must survive it.

**Why this priority**: P1 despite being invisible in normal use. It is the difference between
a failed run and an erased address book, and the failure mode it guards is indistinguishable
from legitimate input.

**Independent Test**: Sync six contacts, make the provider answer with nothing, run again, and
confirm the run fails with a mass-deletion error and all six local contacts remain.

**Acceptance Scenarios**:

1. **Given** at least five tracked contacts and a run that would delete more than half of
   them, **When** the run reaches the deletion step, **Then** it aborts before deleting
   anything (`internal/sync/engine.go` `checkDeletionSafety`).
2. **Given** fewer than five tracked contacts, **When** all of them disappear, **Then** the
   deletions proceed — otherwise a user with two contacts could never delete one
   (`internal/sync/engine.go` `checkDeletionSafety`, `deletionAbortMinimum = 5`).
3. **Given** an incremental run where the provider names deletions explicitly, **When** those
   deletions cross the threshold, **Then** the guard still fires: a bug in delta parsing looks
   exactly like a bulk delete (`internal/sync/engine.go` `pullPhase`).
4. **Given** an export run, **When** more than half the tracked local contacts are missing,
   **Then** the same guard aborts before deleting them from the provider
   (`internal/sync/engine.go` `pushPhase`, second call site at `engine.go:544`).

---

### User Story 7 - Sync only what changed (Priority: P2)

A user with a large address book wants a five-minute schedule not to re-download everything
every time.

**Why this priority**: Efficiency, not correctness — the full-listing path stays correct. It
ranks P2 because the system degrades gracefully to it, automatically, on any provider that
cannot do better.

**Independent Test**: Run a Google or CardDAV import twice; confirm the second run touches
only changed contacts and that a cursor row exists in `sync_cursors`.

**Acceptance Scenarios**:

1. **Given** a provider implementing the incremental interface and a configured cursor store,
   **When** the first run executes, **Then** it fetches the whole collection and stores the
   cursor the provider returned (`internal/sync/engine.go` `remoteChanges`).
2. **Given** a stored cursor, **When** the next run executes, **Then** only the reported
   changes are applied and unchanged contacts are not written
   (`internal/sync/engine.go` `pullPhase`).
3. **Given** a cursor the provider no longer honours, **When** the run executes, **Then** the
   cursor is discarded and the collection is re-listed in full rather than the run failing
   (`internal/sync/engine.go` `remoteChanges`, `ErrCursorExpired`).
4. **Given** a provider that implements neither incremental listing nor conditional writes,
   **When** a run executes, **Then** it falls back to a full listing with no configuration
   (`internal/sync/engine.go` `remoteChanges`).
5. **Given** changes applied successfully, **When** the run ends, **Then** the cursor is
   advanced only then — storing it earlier would skip anything that failed
   (`internal/sync/engine.go`, end of `pullPhase`).

---

### User Story 8 - Connect Google Contacts (Priority: P2)

A user supplies their own Google OAuth client, authorises ContactsHQ, and uses Google as a
pipeline source.

**Why this priority**: A major provider, but it requires the user to register their own OAuth
client in Google Cloud, which puts it behind CardDAV in practical reach.

**Independent Test**: Post a client id and secret to `/auth/google/init`, complete the consent
screen, confirm `/auth/google/status` reports connected, then build a pipeline whose source is
`google` and run it.

**Acceptance Scenarios**:

1. **Given** a client id and secret, **When** the user initialises, **Then** a pending
   connection row is created and an authorisation URL is returned carrying an S256 PKCE
   challenge (`internal/service/google_oauth.go` `GetAuthURL`).
2. **Given** the callback, **When** Google redirects back with a code and state, **Then** the
   code is exchanged using the stored verifier, tokens are saved, the verifier is cleared, and
   the browser lands on the settings page (`internal/service/google_oauth.go`
   `HandleCallback`, `internal/handler/google_handler.go` `Callback`).
3. **Given** an expired access token during a sync, **When** the client refreshes it, **Then**
   the new token is written back to the connection row
   (`internal/service/google_oauth.go` `persistingTokenSource`).
4. **Given** a connected account, **When** the user disconnects, **Then** the refresh token is
   revoked at Google on a best-effort basis and the row is deleted
   (`internal/service/google_oauth.go` `RevokeToken`).
5. **Given** a Google contact created by ContactsHQ, **When** it is written, **Then** the
   `resourceName` Google minted is what gets tracked, and a create that returns no
   `resourceName` is an error rather than a silently mistracked contact
   (`internal/sync/google_provider.go` `Put`).

---

### User Story 9 - Keep provider credentials in one place (Priority: P3)

A user stores a CardDAV endpoint, username and password once and references it from several
pipeline steps.

**Why this priority**: Convenience. Credentials can also be written inline in a step's config,
so nothing is unreachable without the vault.

**Independent Test**: Create a credential, reference it from a step by `credential_id`, and
confirm the step runs without the endpoint or password appearing in the pipeline row.

**Acceptance Scenarios**:

1. **Given** a saved credential, **When** it is listed or fetched, **Then** the password,
   access token, refresh token and client secret are never serialised
   (`internal/domain/provider_connection.go`, `json:"-"` on those fields).
2. **Given** a credential belonging to another user, **When** it is fetched, updated or
   deleted, **Then** the request is refused with 403
   (`internal/handler/credential_handler.go`).
3. **Given** a step referencing `credential_id`, **When** the step runs, **Then** the
   endpoint, username, password and TLS setting come from the stored row
   (`internal/sync/pipeline.go` `createProvider`).
4. **Given** an update that leaves the password field empty, **When** it is saved, **Then**
   the stored password is kept rather than blanked
   (`internal/handler/credential_handler.go` `Update`).

---

### Edge Cases

Boundary conditions the code answers deliberately. Behaviour that contradicts a stated
intention is not here — it is in `## Known Divergences`.

- What happens when the provider answers with an empty listing? The run aborts before deleting
  anything, provided at least five contacts are tracked (FR-014).
- What happens when the address book is tiny — two contacts, both gone? The deletions proceed;
  the guard is disarmed below five tracked contacts so a small address book stays usable
  (FR-014).
- What happens when the stored cursor is too old for the provider to honour? It is discarded
  and the collection is re-listed in full; the run does not fail (FR-005).
- What happens when a provider supports neither delta listing nor conditional writes? The
  engine falls back to a full listing, with no configuration to set (FR-003).
- What happens when a conditional write's precondition fails? It becomes a conflict rather than
  an overwrite — except under `dest_wins`, where the write is retried unconditionally (FR-006).
- What happens when the CardDAV server accepts a write but returns no ETag? The object is
  re-read so the sync state is not left holding a stale version (FR-065).
- What happens when one step of a multi-step pipeline cannot build its provider? Its error is
  recorded against that step and the remaining steps still run (FR-031).
- What happens when a step's stored JSON config cannot be parsed? That is reported as a step
  execution failure, not as an endpoint problem (FR-026).
- What happens when the Google callback arrives for a connection that is already connected? It
  is idempotent rather than an error (FR-055).
- What happens when a conflict's sync state has disappeared before it is resolved? The contact
  is still written; only the state advance is skipped (FR-046).
- What happens when the engine is told to delete a contact that is already absent locally? It
  is a no-op (FR-075).
- What happens when a credential is updated with an empty password field? The stored password
  is kept rather than blanked (User Story 9, scenario 4).

## Requirements *(mandatory)*

### Functional Requirements

FR numbers are stable identifiers. FR-019 and FR-020 are retained as cross-references rather
than renumbered away, so that every other number in this file — and every reference to one
from `## Success Criteria`, `## Assumptions` and sibling specs — keeps its meaning.

**The provider contract**

- **FR-001**: A sync provider MUST expose a name, a full listing, a single-item fetch, a write
  and a delete; the engine MUST drive every provider through that interface alone
  (`internal/sync/provider.go` `SyncProvider`).
- **FR-002**: A write MUST report the identifier the provider actually stored the item under,
  and the engine MUST record that identifier rather than the one it sent. Google mints its own
  `resourceName`; recording the sent id makes the next run treat the remote contact as new and
  the tracked one as deleted, deleting the original and re-importing a copy
  (`internal/sync/provider.go` `PutResult`, `internal/sync/engine.go` `pushPhase`).
- **FR-003**: A provider MAY additionally support delta listing with an opaque cursor. The
  engine MUST detect this at run time and MUST fall back to a full listing when the provider
  does not support it or no cursor store is configured
  (`internal/sync/provider.go` `IncrementalProvider`, `internal/sync/engine.go`
  `remoteChanges`).
- **FR-004**: A provider MAY additionally support conditional writes carrying the version the
  caller last saw. When available, the export phase MUST use it and MUST NOT download the
  remote collection to compare versions first
  (`internal/sync/provider.go` `ConditionalWriter`, `internal/sync/engine.go` `pushPhase`).
- **FR-005**: A cursor the provider rejects as too old MUST be discarded and the collection
  re-listed in full, rather than failing the run
  (`internal/sync/provider.go` `ErrCursorExpired`, `internal/sync/engine.go`;
  recognised from Google's HTTP 410 in `internal/sync/google_provider.go`
  `isExpiredSyncToken` and from a CardDAV report failure with a non-empty cursor in
  `internal/sync/carddav_client.go`).
- **FR-006**: A conditional write whose precondition fails MUST become a conflict rather than
  an overwrite, except under `dest_wins` where the local copy is authoritative and the write
  is retried unconditionally (`internal/sync/engine.go` `pushPhase`).

**The engine**

- **FR-007**: A run MUST execute the import phase for `import` and `two_way`, and the export
  phase for `export` and `two_way`, in that order (`internal/sync/engine.go` `doSync`).
- **FR-008**: The direction vocabulary MUST accept `import`, `export`, `two_way`, the legacy
  `pull`, `push`, `bidirectional`, and an empty string (meaning import); any other value MUST
  be an error so a typo cannot produce a run that does nothing and reports success
  (`internal/sync/engine.go` `ParseSyncMode`).
- **FR-009**: A tracked contact MUST be looked up by the id belonging to the side being
  addressed — remote id against the provider, local id against the address book — because the
  two are equal only by accident (`internal/sync/engine.go` `pullPhase`, `pushPhase`).
- **FR-010**: A remote contact MUST be treated as changed when either its ETag or the SHA-256
  hash of its card body differs from the recorded values
  (`internal/sync/engine.go` `remoteModified`, `ContentHash`).
- **FR-011**: `ContentHash` MUST have exactly one implementation and MUST be exported, so a
  repair command can recompute `sync_states` after the encoder changes; a second
  implementation would disagree and force a full resync
  (`internal/sync/engine.go` `ContentHash`).
- **FR-012**: A run MUST be recorded with its status, per-category counts, start and finish
  time, and — on failure — the error message, whenever a run repository is configured
  (`internal/sync/engine.go` `Sync`, `internal/domain/sync_run.go`).
- **FR-013**: The incremental cursor MUST be stored only after the changes it covers have been
  applied (`internal/sync/engine.go`, end of `pullPhase`).

**Safeguards**

- **FR-014**: A run MUST abort before propagating deletions when more than half of at least
  five tracked contacts would be deleted, on both the import and the export phase. A
  truncated, empty or expired provider response is indistinguishable from a genuine bulk
  delete, and the guard caps the blast radius of all of them
  (`internal/sync/engine.go` `checkDeletionSafety`, called from `pullPhase` at `engine.go:248`
  and `pushPhase` at `engine.go:544`; thresholds `deletionAbortRatio = 0.5`,
  `deletionAbortMinimum = 5`).
- **FR-015**: The deletion guard MUST remain active in incremental mode, where the provider
  names deletions explicitly, because a bug in delta parsing produces the same input as a
  genuine bulk delete (`internal/sync/engine.go`, comment at `deletions :=`).
- **FR-016**: Every HTTP request a sync makes MUST carry an explicit deadline. A host that
  accepts a connection and then goes silent otherwise holds a worker goroutine forever, and
  the pool is four goroutines wide (`internal/worker/goroutine_worker.go:39-41`, default
  `workers = 4`), so four such syncs stop scheduled backups and duplicate detection too —
  they share one queue (`internal/sync/carddav_client.go` `defaultHTTPTimeout = 30 *
  time.Second`).
- **FR-017**: A client supplied by a caller — an OAuth2 client, for instance — MUST be copied
  before a timeout and redirect policy are imposed on it, so the caller's client is not
  mutated (`internal/sync/carddav_client.go` `withTimeout`, `carddav_client.go:47-49`).
- **FR-018**: Provider discovery MUST honour the caller's context cancellation rather than run
  to completion (`internal/sync/carddav_client.go` `discoverAddressBook`).
- **FR-019**: *(Moved.)* Closing history rows left `running` by a dead process, bounded to runs
  that began before this process started, is specified by
  `008-runtime-configuration-and-delivery` FR-041 — `reconcileInterruptedRuns`
  (`cmd/server/startup.go:27`) closes backup runs and sync runs in one pass, so a requirement
  scoped to one of the two tables would describe half a function. This spec still owns the
  repository method that call site uses: `MarkStaleInterrupted`
  (`internal/repository/bun_sync_run.go:65-79`) sets `status = 'interrupted'` with
  `error_message = 'server restarted'` for `running` rows whose `started_at` precedes the
  instant it is given, and never for rows started after it.
- **FR-020**: *(Moved.)* Pruning pipeline-run history at boot is specified by
  `008-runtime-configuration-and-delivery` FR-042 — `pruneSyncRuns`
  (`cmd/server/startup.go:53`) is part of the boot sequence, which 008 owns. The retention
  setting itself is this spec's configuration surface: `sync.runs_retention_days`
  (`internal/config/config.go:153-157`, default 90 at `config.go:194`; 0 or less disables), and
  so is the repository method it drives, `DeleteOlderThan`
  (`internal/repository/bun_sync_run.go`). The table gains a row per pipeline execution, which
  is why the window exists at all.

**Endpoint admission control**

- **FR-021**: A provider URL MUST be validated at all four points at which one can enter the
  system: CardDAV connect (`internal/handler/sync_handler.go` `CardDAVConnect`), credential
  create and update (`internal/handler/credential_handler.go`), the endpoint inside a pipeline
  step's JSON config (`internal/sync/pipeline.go` `ValidateStep`, called from both
  `internal/handler/pipeline_handler.go` `validateSteps` at save time and
  `internal/sync/pipeline.go` `Execute` at run time), and the endpoint posted to a manual
  trigger (`internal/handler/sync_handler.go` `CardDAVTrigger`).
- **FR-022**: Only `http` and `https` MUST be fetchable; `http` MUST be refused unless
  `sync.allow_insecure_endpoints` is set, because a sync request carries the provider's
  username and password (`internal/sync/endpoint.go`, `internal/config/config.go`
  default `false`).
- **FR-023**: A URL with no host, or with credentials in the userinfo component, MUST be
  refused — URL credentials would be logged and stored alongside the URL, and the provider
  config carries its own username and password fields (`internal/sync/endpoint.go`).
- **FR-024**: The HTTP client MUST independently refuse a redirect that leaves the original
  host, and MUST bound a redirect chain at 3. Validating the submitted string alone would not
  stop a permitted host answering `302 Location: http://169.254.169.254/`, and discovery
  follows redirects by design (`internal/sync/endpoint.go` `redirectPolicy`,
  `maxEndpointRedirects = 3`; applied in `internal/sync/carddav_client.go` via
  `CheckRedirect`).
- **FR-025**: Private and loopback addresses MUST NOT be filtered. Syncing against a CardDAV
  server on the local network is a supported use, and a dial-time filter would break it for
  every deployment to defend a single-user instance against its own operator
  (`internal/sync/endpoint.go` header comment).
- **FR-026**: A step configuration that cannot be parsed as JSON MUST NOT be reported as an
  endpoint problem — step execution reports that separately
  (`internal/sync/endpoint.go` `endpointFromConfig`).

**Pipelines**

- **FR-027**: A step's destination MUST be `internal` and its source MUST be a non-empty
  external provider. Provider-to-provider steps MUST be rejected: conflict resolution loads
  the local contact, and with neither side local it cannot work. Chaining two steps through
  the internal book is the supported alternative (`internal/sync/pipeline.go` `ValidateStep`).
- **FR-028**: Step shape, endpoint and direction MUST be validated when the pipeline is saved,
  so a bad configuration is refused at the keyboard rather than at 02:00
  (`internal/handler/pipeline_handler.go` `validateSteps`, called from `Create` and `Update`).
- **FR-029**: A pipeline MUST have at least one step (`internal/handler/pipeline_handler.go`
  `validateSteps`).
- **FR-030**: A step's configuration MUST be re-validated at execution time as well as at save
  time, because a stored row can predate the rule (`internal/sync/pipeline.go` `Execute`).
- **FR-031**: A failing step MUST NOT abort the pipeline; its error MUST be reported against
  that step alone and the remaining steps MUST run (`internal/sync/pipeline.go` `Execute`).
- **FR-032**: A step MAY name a provider inline or reference a stored credential by id; a
  referenced credential MUST belong to the requesting user (`internal/sync/pipeline.go`
  `createProvider`, `cred.UserID != userID` check).
- **FR-033**: Creating, updating or deleting a pipeline MUST update the scheduler immediately,
  without a restart (`internal/handler/pipeline_handler.go`, `internal/worker/scheduler.go`
  `RegisterPipelineJob` / `ReregisterPipelineJob` / `RemovePipelineJob`).
- **FR-034**: A schedule MUST be a valid cron expression or empty
  (`internal/handler/pipeline_handler.go`, `worker.ValidateCron`).
- **FR-035**: Enabled pipelines with a schedule MUST be registered at startup
  (`cmd/server/main.go`, `pipelineRepo.ListAllEnabled` → `sched.RegisterPipelines`).
- **FR-036**: A pipeline MUST be reachable only by its owner; another user's id MUST produce
  "not found" (`internal/service/pipeline.go` `GetByID`, used by every handler path).
- **FR-037**: An omitted conflict mode MUST default to `source_wins` and an omitted direction
  to `import`, rather than silently taking a column default that contradicts the form
  (`internal/service/pipeline.go` `buildStep`).

**Merge and conflicts**

- **FR-038**: A three-way merge MUST compare each vCard property across base, local and
  remote; a property changed on one side only MUST take that side, and a property changed on
  both MUST become a conflict (`internal/sync/merger.go` `MergeVCards`).
- **FR-039**: With no base recorded, agreement between the two sides MUST be taken as-is and
  disagreement MUST become a conflict (`internal/sync/merger.go` `MergeVCards`).
- **FR-040**: `UID` and `VERSION` MUST be carried from the local card and never merged or
  conflicted (`internal/sync/merger.go` `skipFields`).
- **FR-041**: `manual` MUST never auto-merge; only `auto` may attempt one, and only
  `source_wins` may proceed past an unresolved conflict (`internal/sync/engine.go`
  `pullPhase`).
- **FR-042**: A conflict record MUST carry the base, local and remote cards, the remote ETag
  observed at detection, and a per-field diff list (`internal/sync/engine.go`
  `recordConflict`, `internal/domain/sync_conflict.go`,
  `migrations/018_sync_conflict_remote_etag.up.sql`).
- **FR-043**: Repeated runs over an unresolved conflict MUST update the existing row, not
  append one per scheduler tick (`internal/sync/engine.go` `pendingConflictsByRemoteID`).
- **FR-044**: Resolving a conflict MUST write the merged card to the local contact and its
  child records, not merely store it on the conflict row — nothing reads the stored copy back,
  so a resolution that stops there evaporates on the next run
  (`internal/service/sync_conflict.go` `Resolve`, and its type comment).
- **FR-045**: Resolving MUST advance the sync state: the resolved card becomes the new base
  and content hash, the remote ETag becomes the one recorded at detection, and the local ETag
  is cleared so the next export carries the resolution outward
  (`internal/service/sync_conflict.go` `advanceSyncState`).
- **FR-046**: A conflict whose sync state has since disappeared MUST still resolve the contact
  rather than failing (`internal/service/sync_conflict.go` `advanceSyncState`, nil state
  returns without error).
- **FR-047**: A conflict MUST be resolvable only once and only by its owner; an already
  resolved or dismissed conflict MUST be refused with 400, another user's with 403, a missing
  one with 404 (`internal/service/sync_conflict.go` `load`,
  `internal/handler/sync_handler.go` `conflictError`).
- **FR-048**: A conflict MAY be dismissed, marking it reviewed without changing any contact
  (`internal/service/sync_conflict.go` `Dismiss`).
- **FR-049**: The conflict queue MUST be listable with a status filter and pagination, and a
  pending count MUST be available for a badge (`internal/handler/sync_handler.go`
  `ListConflicts`, `CountConflicts`; `web/src/views/sync/SyncConflictsView.vue`).

**Provider connections and Google OAuth**

- **FR-050**: A user MUST be able to store, list, fetch, update and delete provider
  credentials scoped to themselves (`internal/handler/credential_handler.go`;
  routes in `internal/handler/handler.go`).
- **FR-051**: Secrets MUST never be serialised in an API response — password, access token,
  refresh token and client secret are all excluded
  (`internal/domain/provider_connection.go`).
- **FR-052**: A CardDAV connect MUST upsert a single connection of that type for the user
  (`internal/handler/sync_handler.go` `CardDAVConnect`). Note that the database no longer
  enforces uniqueness: `migrations/011_drop_provider_unique.up.sql` dropped the index so
  several credentials of one type can exist; the upsert behaviour here is handler-level.
- **FR-053**: The Google authorisation URL MUST carry a PKCE challenge derived as
  BASE64URL(SHA256(verifier)) with method S256, offline access, and forced consent
  (`internal/service/google_oauth.go` `GetAuthURL`).
- **FR-054**: The callback MUST exchange the code using the verifier stored against that
  pending connection and MUST use the exact redirect URL used to start the flow
  (`internal/service/google_oauth.go` `HandleCallback`, redirect URL stored in `Endpoint`).
- **FR-055**: A callback for an already-connected row MUST be idempotent rather than an error
  (`internal/service/google_oauth.go` `HandleCallback`).
- **FR-056**: A refreshed access token MUST be persisted back to the connection
  (`internal/service/google_oauth.go` `persistingTokenSource`).
- **FR-057**: Disconnecting MUST attempt token revocation at Google on a best-effort basis and
  MUST delete the connection regardless of whether Google was reachable
  (`internal/service/google_oauth.go` `RevokeToken`).
- **FR-058**: Connection status MUST report disconnected when the row is absent, not marked
  connected, or holds no tokens (`internal/handler/google_handler.go` `Status`).
- **FR-059**: A pending Google connection MUST be created as not-connected, forced explicitly
  because the ORM omits a zero-value boolean on insert against a column defaulting to true
  (`internal/service/google_oauth.go`, `SetConnected(ctx, connID, false)`).
- **FR-060**: A new authorisation flow MUST delete any prior *pending* connection for that
  user rather than accumulate OAuth state rows
  (`internal/service/google_oauth.go` `GetAuthURL`).

**CardDAV client provider**

- **FR-061**: Address-book discovery MUST try, in order: full principal discovery at the given
  endpoint; `.well-known/carddav` followed by discovery at the resolved URL; DNS SRV/TXT when
  no path was given; and finally treating the URL's path as the address book itself
  (`internal/sync/carddav_client.go` `discoverAddressBook`).
- **FR-062**: The provider MUST be built from the endpoint discovery actually resolved to, not
  the one supplied, because `.well-known` may redirect
  (`internal/sync/carddav_client.go`).
- **FR-063**: Both public constructors MUST assemble the provider through one internal
  function, so no construction path can omit the fields the conditional write needs
  (`internal/sync/carddav_client.go` `newCardDAVClientProvider` and its comment).
- **FR-064**: A conditional write MUST be issued as a raw request carrying `If-Match`, or
  `If-None-Match: *` when creating, because the underlying WebDAV client sends no precondition
  (`internal/sync/carddav_client.go` `PutIfMatch`). A `412` MUST become the
  precondition-failed error.
- **FR-065**: When a server omits the ETag on a write, the object MUST be re-read so sync
  state stays accurate (`internal/sync/carddav_client.go` `PutIfMatch`).
- **FR-066**: A delta MUST be fetched with `sync-collection` and the changed card bodies
  retrieved with a follow-up multi-get, since the report carries ETags only
  (`internal/sync/carddav_client.go` `ListChanges`).
- **FR-067**: vCard serialisation MUST go through the shared encoder, never a local copy —
  a local copy is how this package kept emitting escaped commas in photo values after the bug
  was fixed centrally (`internal/sync/carddav_client.go` `cardToString` →
  `internal/vcard`).

**Google provider**

- **FR-068**: Contacts MUST be listed with a sync token requested, paging until exhausted, and
  the token MUST be taken from the final page
  (`internal/sync/google_provider.go` `listConnections`).
- **FR-069**: Persons flagged deleted in a delta MUST be reported as deletions rather than
  skipped — they are exactly what incremental sync needs
  (`internal/sync/google_provider.go`).
- **FR-070**: An update MUST carry the ETag; a mismatch (HTTP 412, or 400 whose message names
  `FAILED_PRECONDITION`) MUST become the precondition-failed error
  (`internal/sync/google_provider.go` `PutIfMatch`, `isPreconditionFailure`).
- **FR-071**: A create that returns no `resourceName` MUST be an error, because the contact
  could not be matched on the next run (`internal/sync/google_provider.go` `Put`).
- **FR-072**: The People API field set requested and the field set written MUST be declared in
  one place each (`internal/sync/google_mapper.go` `allPersonFields`,
  `allUpdatePersonFields`).

**Internal provider**

- **FR-073**: The internal provider MUST address contacts by UID and MUST create the user's
  address book on demand (`internal/sync/internal_provider.go`, `GetOrCreateByUserID`).
- **FR-074**: A write MUST update both the stored card and the parsed scalar and child records
  in one save, so a synced contact is searchable and renderable like any other
  (`internal/sync/internal_provider.go` `Put`, `ApplyToContact` + `ChildRecordsFor`).
- **FR-075**: Deleting a contact that is already absent MUST be a no-op, not an error
  (`internal/sync/internal_provider.go` `Delete`).

**Manual triggers and observability**

- **FR-076**: A manual CardDAV sync MUST be enqueued as a background job, using the stored
  connection when the request supplies no URL, and MUST fail with 400 when neither is available
  (`internal/handler/sync_handler.go` `CardDAVTrigger`).
- **FR-077**: Active runs and recent history MUST be listable per user, history bounded to at
  most 100 rows per request (`internal/handler/sync_handler.go` `Status`, `History`;
  `internal/repository/bun_sync_run.go`).
- **FR-078**: Per-pipeline run history MUST be listable, bounded to at most 200 rows
  (`internal/handler/pipeline_handler.go` `ListRuns`).
- **FR-079**: Connections MUST be listable with their type, endpoint, connected flag, last
  sync time and last error, without secrets
  (`internal/handler/sync_handler.go` `ListProviders`).
- **FR-080**: A connection MUST be removable only by its owner
  (`internal/handler/sync_handler.go` `DisconnectProvider`).

**Web surface**

- **FR-081**: The application MUST expose routes for the sync overview, the conflict queue, a
  conflict detail with per-field choices, the credential vault, the pipeline list, create,
  view and edit screens, and Google settings (`web/src/router/index.ts`, which this spec cites
  but does not own — see `## Known Divergences`).
- **FR-082**: The sync overview route MUST exist and point at its view — it once did not,
  which made every `/sync/providers` endpoint unreachable from the UI; a test now asserts it
  (`web/src/router/index.ts` comment; the assertion lives in `web/src/router/routes.spec.ts`).
- **FR-083**: The pipeline form MUST NOT offer a destination picker: the destination is always
  the internal address book (`web/src/views/pipelines/PipelineCreateView.vue`, comment at
  the step interface).
- **FR-084**: Per-field conflict choices MUST default to keeping the local value
  (`web/src/views/sync/SyncConflictDetailView.vue`).

### Key Entities

- **Provider connection** (`provider_connections`) — a stored credential or OAuth grant for one
  external provider: type, name, endpoint, username, password, TLS-verification flag, connected
  flag, last sync time, last error, and the OAuth quintet (access token, refresh token, expiry,
  client id, client secret, scopes). One row also serves as transient OAuth state during a
  Google flow. `internal/domain/provider_connection.go`,
  `migrations/009`, `010`, `011`, `015`.
- **Pipeline** (`pipelines`) — a named, optionally scheduled, optionally enabled unit of work
  belonging to one user. `internal/domain/pipeline.go`.
- **Pipeline step** (`pipeline_steps`) — an ordered element of a pipeline: source type and
  JSON config, destination type and JSON config (destination always `internal`), conflict
  mode, direction. `internal/domain/pipeline.go`, `migrations/007`, `019`.
- **Sync state** (`sync_states`) — the pairing that makes a run idempotent: remote id, local
  id, remote ETag, local ETag, content hash, and `base_vcard`, the three-way merge anchor.
  Keyed by `(user_id, provider_type)` where provider type is the string `source->dest`.
  `internal/domain/sync_state.go`, `migrations/004`.
- **Sync cursor** (`sync_cursors`) — one opaque incremental token per `(user, provider_type)`.
  `internal/domain/sync_cursor.go`, `migrations/022`.
- **Sync run** (`sync_runs`) — the history row for one engine invocation: status, four counts,
  error message, start and finish, and the pipeline that caused it.
  `internal/domain/sync_run.go`, `migrations/003`, `012`.
- **Sync conflict** (`sync_conflicts`) — a divergence awaiting a human: base, local and remote
  cards, the remote ETag at detection, a per-field diff list, status, and the resolved card.
  `internal/domain/sync_conflict.go`, `migrations/005`, `018`.
- **Delta** — what a provider changed since a cursor: updated items, deleted remote ids, the
  next cursor, and a flag saying whether this is the whole collection.
  `internal/sync/provider.go`.
- **Endpoint policy** — what a deployment is willing to fetch. Currently one field: whether
  plain http is permitted. `internal/sync/endpoint.go`, `internal/config/config.go`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A provider answering with an empty listing deletes zero local contacts, for any
  address book of five or more tracked contacts.
- **SC-002**: Deleting one contact out of six still propagates — the guard costs no legitimate
  deletion below the 50% threshold.
- **SC-003**: A silent provider host releases its worker within the client's request deadline
  of 30 seconds; the bounded-return behaviour is observed at a 2-second context deadline
  against a 10-second failure ceiling.
- **SC-004**: A second run against an unchanged remote writes zero contacts and zero sync
  states.
- **SC-005**: Zero of the four endpoint entry points accepts a non-http(s) scheme, a hostless
  URL, or URL-embedded credentials; all four call the same function (the call sites are listed
  in FR-021).
- **SC-006**: A permitted host cannot redirect a sync request to a different host, at any
  point in a chain bounded to 3 hops.
- **SC-007**: A contact created locally and exported twice exists exactly once on the
  provider, and a following two-way run neither deletes nor duplicates it.
- **SC-008**: An unresolved conflict produces exactly one queue row no matter how many times
  the schedule fires.
- **SC-009**: A resolved conflict is not re-raised by the next run. Observed at the level of
  the sync-state transition, not end-to-end.
- **SC-010**: A conditional-writer provider performs zero remote list operations during an
  export.
- **SC-011**: An expired cursor costs one extra full listing and no failed run.
- **SC-012**: Run history stays bounded rather than growing without limit. The window is
  `sync.runs_retention_days` (default 90; 0 or less disables), which is this spec's setting;
  the boot-time step that applies it is specified by
  `008-runtime-configuration-and-delivery` (FR-042).
- **SC-013**: A pipeline the engine cannot run cannot be saved — every step shape the engine
  rejects is refused with 400 by the create and update handlers.
- **SC-014**: No API response in this domain contains a provider password, access token,
  refresh token or client secret.

No throughput or latency target is committed to. The engine's cost is dominated by the remote
provider's response time, and no benchmark exists in this package — `grep` finds no
`Benchmark` function under `internal/sync/`.

## Assumptions

**About the deployment**

- **One connection per provider type per user.** The engine's state key contains only the
  provider type, so the system assumes a user syncs at most one CardDAV server and one Google
  account. The database stopped enforcing this at `migrations/011_drop_provider_unique.up.sql`
  and nothing replaced the constraint. Recorded under `## Known Divergences` because it is an
  assumption the code does not defend.
- **Single instance.** Interrupted-run reconciliation is bounded by process start time
  (`internal/repository/bun_sync_run.go` `MarkStaleInterrupted`), which is safe for one process
  and wrong for two. Consistent with the project's stated position (`CLAUDE.md`).
- **Self-hosted trust boundary.** Provider passwords and OAuth tokens are stored in plaintext,
  documented as a deliberate choice pending multi-tenant use
  (`internal/domain/provider_connection.go`).
- **The operator is not an attacker.** This is the premise behind not filtering private
  addresses (FR-025). It holds for a single-user self-hosted instance and does not hold for a
  shared one.
- **Forward-only migrations.** `sync_states.provider_uri` and `sync_states.sync_token` are
  dead but permanent; a `.down.sql` exists for every migration and none is ever applied
  (`CLAUDE.md`, `internal/repository/db.go`).
- **Contacts are identified by UID across the internal boundary.** Conflict resolution and the
  internal provider both rely on it.
- **A user who reaches the Google flow can register an OAuth client.** There is no
  deployment-wide client: `client_id` and `client_secret` are per-user inputs
  (`internal/handler/google_handler.go` `InitAuth`). A deployment-level
  `google.redirect_url` exists as a fallback only (`internal/service/google_oauth.go`).

**What this relies on, owned elsewhere**

- **A vCard encoder with one writer.** `ContentHash` is computed over encoder output, so
  changing the encoder moves every hash and every local ETag. That seam is owned by
  `003-vcard-representation` (`internal/vcard/encoder.go`, `internal/vcard/parser.go`); this
  spec owns `ContentHash` itself (`internal/sync/engine.go`).
- **The internal address book and contact repository**, reached through `InternalProvider`
  (`internal/sync/internal_provider.go`); the repositories themselves belong to
  `002-contact-record-and-catalog`.
- **The worker queue and scheduler** for scheduled pipelines and enqueued manual syncs
  (`internal/worker/goroutine_worker.go`, `internal/worker/scheduler.go`). The four-goroutine
  width of that pool is what makes FR-016 load-bearing.
- **`server.write_timeout` remaining 0**, because `POST /pipelines/:id/trigger` runs the whole
  pipeline inside the request (`internal/handler/pipeline_handler.go`,
  `internal/config/config.go` validation, owned by 008).
- **The boot sequence** — interrupted-run reconciliation and run-history pruning
  (`cmd/server/startup.go`), owned by `008-runtime-configuration-and-delivery`; see FR-019 and
  FR-020.
- **Third-party libraries**: `github.com/emersion/go-webdav/carddav` (CardDAV client),
  `github.com/emersion/go-vcard` (merge-time parsing), `golang.org/x/oauth2` and
  `google.golang.org/api/people/v1`.

**Out of scope**

- **ContactsHQ acting as a CardDAV server** — the `/dav` mount, CTag, `sync-collection`, the
  change journal and tombstones, including `migrations/021_change_journal`. Owned by
  `004-carddav-service`. This spec's `carddav_incremental_test.go` drives that server as a
  fixture, which is why `internal/carddav/sync_collection.go` and
  `internal/repository/change_journal.go` appear under `## References`.
- **Restore-side sync-state reconciliation** — dropping sync states for contacts a backup did
  not bring back. Owned by the operation that causes it, `005-bulk-transfer-and-backup`.
- **The vCard encoder and parser** whose output feeds `ContentHash`. `003-vcard-representation`.
- **The duplicate detector and merge**, which share `sync_states` only through `MergeService`'s
  use of the repository. `007-duplicate-detection-and-merge`.
- **Authentication and the JWT middleware** that guards every route named here.
  `001-identity-and-credentials`.
- **The worker queue and scheduler implementations** themselves; this spec depends on them and
  states one requirement about the consequence of their width (FR-016).

## Status

`shipped`, at `v0.4.0` — read from the working tree at `23a167c` (`v0.4.0-3-g23a167c`), whose
three commits past the tag are documentation and one config-example fix.

Every requirement above cites a file that can be read at that commit. Two requirement slots,
FR-019 and FR-020, describe behaviour that ships but is specified by
`008-runtime-configuration-and-delivery`; they are retained as cross-references so the
numbering stays stable.

## Code Paths

Owned by this spec:

- `internal/sync/engine.go`
- `internal/sync/engine_test.go`
- `internal/sync/engine_push_test.go`
- `internal/sync/engine_incremental_test.go`
- `internal/sync/provider.go`
- `internal/sync/merger.go`
- `internal/sync/merger_test.go`
- `internal/sync/pipeline.go`
- `internal/sync/mode_test.go`
- `internal/sync/endpoint.go`
- `internal/sync/endpoint_test.go`
- `internal/sync/internal_provider.go`
- `internal/sync/carddav_client.go`
- `internal/sync/carddav_client_construct_test.go`
- `internal/sync/carddav_incremental_test.go`
- `internal/sync/google_provider.go`
- `internal/sync/google_provider_test.go`
- `internal/sync/google_mapper.go`
- `internal/sync/google_mapper_test.go`
- `internal/service/google_oauth.go`
- `internal/service/google_oauth_test.go`
- `internal/service/sync_conflict.go`
- `internal/service/sync_conflict_test.go`
- `internal/service/pipeline.go`
- `internal/handler/sync_handler.go`
- `internal/handler/pipeline_handler.go`
- `internal/handler/credential_handler.go`
- `internal/handler/google_handler.go`
- `internal/domain/pipeline.go`
- `internal/domain/sync_state.go`
- `internal/domain/sync_run.go`
- `internal/domain/sync_conflict.go`
- `internal/domain/sync_cursor.go`
- `internal/domain/provider_connection.go`
- `internal/repository/bun_sync.go`
- `internal/repository/bun_sync_run.go`
- `internal/repository/bun_sync_run_test.go`
- `internal/repository/bun_sync_conflict.go`
- `internal/repository/bun_sync_cursor.go`
- `internal/repository/bun_sync_cursor_test.go`
- `internal/repository/bun_pipeline.go`
- `internal/repository/bun_provider_connection.go`
- `internal/repository/migrate_019_test.go`
- `internal/worker/jobs/pipeline_job.go`
- `internal/worker/jobs/sync_job.go`
- `migrations/003_sync_runs.up.sql`, `migrations/003_sync_runs.down.sql`
- `migrations/003_sync_runs.down.sql`
- `migrations/004_sync_state_base_vcard.up.sql`, `migrations/004_sync_state_base_vcard.down.sql`
- `migrations/004_sync_state_base_vcard.down.sql`
- `migrations/005_sync_conflicts.up.sql`, `migrations/005_sync_conflicts.down.sql`
- `migrations/005_sync_conflicts.down.sql`
- `migrations/007_pipeline_step_direction.up.sql`, `migrations/007_pipeline_step_direction.down.sql`
- `migrations/007_pipeline_step_direction.down.sql`
- `migrations/009_provider_connections.up.sql`, `migrations/009_provider_connections.down.sql`
- `migrations/009_provider_connections.down.sql`
- `migrations/010_provider_connections_tls.up.sql`, `migrations/010_provider_connections_tls.down.sql`
- `migrations/010_provider_connections_tls.down.sql`
- `migrations/011_drop_provider_unique.up.sql`, `migrations/011_drop_provider_unique.down.sql`
- `migrations/011_drop_provider_unique.down.sql`
- `migrations/012_sync_runs_pipeline_id.up.sql`, `migrations/012_sync_runs_pipeline_id.down.sql`
- `migrations/012_sync_runs_pipeline_id.down.sql`
- `migrations/015_provider_connections_oauth.up.sql`, `migrations/015_provider_connections_oauth.down.sql`
- `migrations/015_provider_connections_oauth.down.sql`
- `migrations/018_sync_conflict_remote_etag.up.sql`, `migrations/018_sync_conflict_remote_etag.down.sql`
- `migrations/018_sync_conflict_remote_etag.down.sql`
- `migrations/019_normalize_pipeline_direction.up.sql`, `migrations/019_normalize_pipeline_direction.down.sql`
- `migrations/019_normalize_pipeline_direction.down.sql`
- `migrations/022_sync_cursors.up.sql`, `migrations/022_sync_cursors.down.sql`
- `migrations/022_sync_cursors.down.sql`
- `web/src/api/sync.ts`
- `web/src/api/pipelines.ts`
- `web/src/api/credentials.ts`
- `web/src/api/google.ts`
- `web/src/utils/pipeline.ts`
- `web/src/views/pipelines/`
- `web/src/views/sync/`
- `web/src/views/credentials/`
- `web/src/views/settings/GoogleSettingsView.vue`

## References

Touched or cited by this spec, owned elsewhere:

- `cmd/server/startup.go` — `reconcileInterruptedRuns` (`:27`) and `pruneSyncRuns` (`:53`) act
  on `sync_runs`; both are specified by `008-runtime-configuration-and-delivery` (FR-041,
  FR-042). See FR-019 and FR-020.
- `cmd/server/startup_test.go` — the tests for those two functions, likewise 008's.
- `cmd/server/main.go` — the composition-root wiring of the engine, orchestrator, endpoint
  policy, job handlers and pipeline registration (FR-035).
- `internal/handler/handler.go` — route registration for every endpoint named here.
- `internal/config/config.go` — `SyncConfig` (`:153-162`), its defaults (`:194`) and the
  `sync.*` env bindings (`:55`).
- `internal/repository/interfaces.go` — the repository interfaces this domain's Bun types
  implement.
- `internal/worker/goroutine_worker.go` — the four-goroutine pool FR-016 depends on.
- `internal/worker/scheduler.go` — pipeline job registration (FR-033).
- `internal/vcard/encoder.go`, `internal/vcard/parser.go` — the single writer whose output
  `ContentHash` is taken over (`003-vcard-representation`).
- `internal/carddav/sync_collection.go`, `internal/repository/change_journal.go`,
  `migrations/021_change_journal.up.sql`, `migrations/021_change_journal.down.sql` — the
  CardDAV *server* side, owned by `004-carddav-service`. They appear here only because
  `internal/sync/carddav_incremental_test.go` points this domain's CardDAV *client* at
  ContactsHQ's own server as a fixture.
- `docs/sync.md` — narrative documentation of this domain. **Unenforced**: nothing tests it and
  nothing fails when it drifts. It was used here as a map only, and every claim that became a
  requirement was re-verified in the source. Two of its claims did not survive that check and
  appear under `## Known Divergences` instead: the grandfathered-credential path (which it
  states correctly, but as a design note rather than a limitation) and the assertion that
  resolution is per-field for all conflicts (the wildcard branch in the UI has no server-side
  counterpart).
- `CHANGELOG.md`, v0.4.0 — the breaking-change note for endpoint validation.
- `CLAUDE.md` — project-wide rules on migrations, body limits and `write_timeout`.

## Enforced By

CI runs `go test ./... -count=1 -race` (`.github/workflows/ci.yml:26`); the PostgreSQL job runs
only `go test ./internal/repository/ -count=1 -run TestPostgres` (`:74`), and no test in this
domain matches that prefix, so none of the repository behaviour below is exercised against
PostgreSQL.

**Engine, safeguards and modes**

- `TestSync_NewItems_CreatedInDest`, `TestSync_DeletedFromSource_DeletedFromDest`,
  `TestSync_ConflictSourceWins`, `TestSync_ConflictDestWins_Skips`
  (`internal/sync/engine_test.go`) — FR-007, FR-009, FR-041.
- `TestSync_RecordsSyncRun` (`internal/sync/engine_test.go`) — FR-012.
- `TestPush_CapturesRemoteAssignedID`, `TestPush_DeleteUsesRemoteID`,
  `TestBidirectional_DoesNotDestroyPushedContact`,
  `TestPush_RemoteChangedSinceLastSync_QueuesConflict`, `TestPush_DestWinsOverwritesRemote`
  (`internal/sync/engine_push_test.go`) — FR-002, FR-009, SC-007.
- `TestPull_EmptySourceListingAbortsInsteadOfDeleting`, `TestPull_SmallDeletionPropagates`,
  `TestPull_DeletionGuardIgnoresTinyAddressBooks` (`internal/sync/engine_push_test.go`) —
  FR-014 on the import phase, SC-001, SC-002.
- `TestPull_ConflictIsDedupedAcrossRuns`, `TestPull_ManualModeDoesNotAutoMerge`,
  `TestPull_SourceWinsLeavesNoPendingConflict` (`internal/sync/engine_push_test.go`) — FR-041,
  FR-043, SC-008.
- `TestParseSyncMode`, `TestValidateStep` (`internal/sync/mode_test.go`) — FR-008, FR-027,
  SC-013 at the validator level.

**Incremental and conditional paths**

- `TestIncremental_FirstSyncIsFullAndStoresCursor`,
  `TestIncremental_DeltaAppliesOnlyReportedChanges`,
  `TestIncremental_UnchangedContactsAreNotTouched`,
  `TestIncremental_NonIncrementalProviderStillFullSyncs`
  (`internal/sync/engine_incremental_test.go`) — FR-003, FR-010, SC-004.
- `TestIncremental_ExpiredCursorResyncsFully` (`internal/sync/engine_incremental_test.go`),
  `TestCardDAVIncremental_BadCursorIsExpired` (`internal/sync/carddav_incremental_test.go`),
  `TestIsExpiredSyncToken` (`internal/sync/google_provider_test.go`) — FR-005, SC-011.
- `TestIncremental_MassDeletionInDeltaIsRefused`
  (`internal/sync/engine_incremental_test.go`) — FR-015.
- `TestConditionalPush_DoesNotListRemote`, `TestConditionalPush_RemoteChangedIsAConflict`,
  `TestConditionalPush_DestWinsOverwrites` (`internal/sync/engine_incremental_test.go`) —
  FR-004, FR-006, SC-010.
- `TestCardDAVIncremental_AgainstOwnServer` (`internal/sync/carddav_incremental_test.go`) —
  FR-066, end-to-end against ContactsHQ's own CardDAV server.

**Endpoint admission control**

- `TestValidateProviderEndpoint` (`internal/sync/endpoint_test.go`) — FR-022, FR-023, FR-025,
  including the "private address is allowed" case.
- `TestValidateStepEndpoint_ChecksTheConfig`, `TestValidateStepEndpoint_StillEnforcesShape`,
  `TestValidateStepEndpoint_IgnoresUnreadableConfig` (`internal/sync/endpoint_test.go`) —
  FR-021 at the step-config entry point, FR-026.
- `TestProvider_RefusesARedirectToAnotherHost`, `TestProvider_FollowsARedirectWithinTheSameHost`
  (`internal/sync/endpoint_test.go`) — FR-024, SC-006.

**CardDAV client**

- `TestCardDAVProvider_PutIfMatchWorksForEveryConstructor`
  (`internal/sync/carddav_client_construct_test.go`) — FR-063, FR-064.
- `TestCardDAVProvider_DiscoveryTimesOutOnSilentHost`,
  `TestCardDAVProvider_DiscoveryStopsOnCancelledContext`
  (`internal/sync/carddav_client_construct_test.go`) — FR-016, FR-018, SC-003.

**Merge and conflict resolution**

- `TestMergeVCards_NoChange`, `TestMergeVCards_OnlyRemoteChanged`,
  `TestMergeVCards_OnlyLocalChanged`, `TestMergeVCards_BothChanged`,
  `TestMergeVCards_ExtraFieldRemoteAdded` (`internal/sync/merger_test.go`) — FR-038.
- `TestMergeVCards_NoBase_BothDiffer`, `TestMergeVCards_NoBase_BothSame`
  (`internal/sync/merger_test.go`) — FR-039.
- `TestMergeVCards_UIDPreserved`, `TestApplyResolution_UIDAlwaysLocal`
  (`internal/sync/merger_test.go`) — FR-040.
- `TestApplyResolution_LocalChoice`, `TestApplyResolution_RemoteChoice`,
  `TestApplyResolution_DefaultsToLocal` (`internal/sync/merger_test.go`) — the per-field
  resolution contract behind FR-044 and FR-084.
- `TestResolve_WritesResolvedVCardToContact` (`internal/service/sync_conflict_test.go`) —
  FR-044.
- `TestResolve_AdvancesSyncState`, `TestResolve_MarksConflictResolved`,
  `TestResolve_MissingSyncStateStillUpdatesContact`
  (`internal/service/sync_conflict_test.go`) — FR-045, FR-046, SC-009.
- `TestResolve_RejectsAnotherUsersConflict`, `TestResolve_RejectsAlreadyResolved`,
  `TestResolve_UnknownConflict`, `TestResolve_ContactGone`,
  `TestDismiss_RejectsAnotherUsersConflict` (`internal/service/sync_conflict_test.go`) —
  FR-047.
- `TestDismiss_MarksDismissedWithoutTouchingContact`
  (`internal/service/sync_conflict_test.go`) — FR-048.

**Google**

- `TestGetAuthURL_Success`, `TestGetAuthURL_MissingCredentials`,
  `TestGetAuthURL_MissingRedirectURL` (`internal/service/google_oauth_test.go`) — FR-053,
  FR-059, FR-060.
- `TestHandleCallback_InvalidState` (`internal/service/google_oauth_test.go`) — FR-054.
- `TestGetHTTPClient_NoTokens` (`internal/service/google_oauth_test.go`) — FR-058.
- `TestRevokeToken_DeletesConnection` (`internal/service/google_oauth_test.go`) — FR-057.
- `TestPersonToVCard_BasicName`, `TestPersonToVCard_EmailsAndPhones`,
  `TestPersonToVCard_Address`, `TestPersonToVCard_Organization`, `TestPersonToVCard_Birthday`,
  `TestPersonToVCard_Nickname`, `TestPersonToVCard_Notes`, `TestPersonToVCard_Gender`,
  `TestPersonToVCard_PhotoAndURL`, `TestParsedContactToPerson_BasicName`,
  `TestParsedContactToPerson_EmailsAndPhones`, `TestParsedContactToPerson_Organization`,
  `TestVCardToPerson`, `TestRoundTrip_PersonToVCardAndBack`, `TestNormalizeGoogleType`,
  `TestToGoogleType`, `TestGoogleDateToString`, `TestParseGoogleDate`
  (`internal/sync/google_mapper_test.go`) — FR-072 and the mapping it governs.

**Persistence**

- `TestSyncRun_CreateAndListByUser`, `TestSyncRun_ListActiveByUser_FiltersRunning`,
  `TestSyncRun_Update_CompletesRun` (`internal/repository/bun_sync_run_test.go`) — FR-012,
  FR-077.
- `TestSyncCursor_AbsentIsEmptyNotError`, `TestSyncCursor_SetThenGet`,
  `TestSyncCursor_SetUpserts`, `TestSyncCursor_SeparatePerProvider`, `TestSyncCursor_Delete`
  (`internal/repository/bun_sync_cursor_test.go`) — FR-003, FR-013 at the storage level.
- `TestMigrate019_InvertedStepIsSwapped`, `TestMigrate019_InvertedPushBecomesImport`,
  `TestMigrate019_AlreadyNormalStepOnlyRenamesDirection`,
  `TestMigrate019_SwapsSyncStateSides`, `TestMigrate019_DropsInvertedConflictsKeepsOthers`
  (`internal/repository/migrate_019_test.go`) — the direction normalisation behind FR-008 and
  FR-027.

**Web**

- `routes the sync providers screen` (`web/src/router/routes.spec.ts:49`) — FR-082.
- `has a route for every screen under views/` (`web/src/router/routes.spec.ts:39`) — FR-081, in
  the weak form that no view file under `web/src/views/` is unrouted.

**Requirements with no enforcer.** Stated as gaps, per constitution Principle VI:

- **No handler test exists anywhere in this domain.** `internal/handler/` contains exactly two
  test files, `health_test.go` and `registration_policy_test.go`, and neither touches sync.
  FR-028, FR-029, FR-033, FR-034, FR-036, FR-037, FR-049, FR-050, FR-051, FR-052 and FR-076
  through FR-080 are therefore unenforced, as is SC-013 and SC-014 at the HTTP level. The
  behaviour is verified by reading the handlers only.
- **FR-021 is enforced only at the step-config entry point.** Three of the four call sites it
  names — `CardDAVConnect`, credential create/update, `CardDAVTrigger` — are handler code with
  no test. Removing the call in any of them would not redden CI. SC-005 inherits this.
- **FR-014's export half has no test.** `checkDeletionSafety` is called twice
  (`internal/sync/engine.go:248` and `:544`); only the `pullPhase` call site at `:248` is
  covered. Nothing would catch the `pushPhase` guard being removed.
- **FR-011 is review-only.** Nothing asserts that `ContentHash` has a single implementation;
  the claim rests on `grep`.
- **FR-013 is review-only in its ordering.** The cursor is asserted to be stored
  (`TestIncremental_FirstSyncIsFullAndStoresCursor`) but nothing asserts that it is stored
  *after* the changes are applied. Reordering those two statements would pass CI.
- **FR-017 has no test.** Nothing asserts that `withTimeout` copies the caller's client rather
  than mutating it; passing the same `*http.Client` twice and observing the second call is all
  it would take.
- **FR-030, FR-031 and FR-032 have no test.** Re-validation at execution time, per-step error
  isolation and the `cred.UserID != userID` ownership check in `createProvider` are all
  read-only claims.
- **FR-035 has no test.** Nothing asserts that enabled, scheduled pipelines are registered at
  boot.
- **FR-061, FR-062, FR-065, FR-067, FR-068, FR-069, FR-070, FR-071, FR-073, FR-074, FR-075
  have no direct test.** Discovery ordering, the endpoint-resolution rule, the ETag re-read,
  the shared-encoder rule, Google paging and deletion flags, precondition classification, the
  internal provider's UID addressing and its idempotent delete are each verified by reading.
- **FR-083 and FR-084 have no test.** No component test exists for any view in
  `web/src/views/pipelines/` or `web/src/views/sync/`, and `web/src/utils/pipeline.ts` has no
  `.spec.ts` beside it, unlike six of its sibling utilities.

## Known Divergences

- **Sync state is keyed by provider *type*, not by connection or pipeline.** The engine builds
  its key as `remote.Name() + "->" + local.Name()` — e.g. `carddav->internal`
  (`internal/sync/engine.go`, `Sync`) — and `sync_states`, `sync_cursors` and `sync_conflicts`
  are all keyed on `(user_id, provider_type)`
  (`migrations/022_sync_cursors.up.sql` unique index; `internal/repository/bun_sync_cursor.go`).
  Two CardDAV pipelines for the same user therefore **share one set of sync states and one
  cursor**, and the manual trigger at `POST /sync/carddav/trigger` shares them too — it passes
  an empty pipeline id but the same provider key
  (`internal/worker/jobs/sync_job.go`). Nothing in the code prevents this configuration or
  warns about it. Two CardDAV servers synced by one account will corrupt each other's state.
  `migrations/011_drop_provider_unique.up.sql` removed the constraint that used to make the
  configuration impossible, and nothing replaced it.
- **`POST /sync/google/connect` and `POST /sync/google/trigger` are registered routes that
  return 501.** They are unconditional stubs
  (`internal/handler/sync_handler.go` `GoogleConnect`, `GoogleTrigger`; registered at
  `internal/handler/handler.go`). Google works only through `/auth/google/*` plus a pipeline.
- **Stored credentials are grandfathered past endpoint validation at run time.** A step
  referencing `credential_id` normally carries no `endpoint` key in its config, so
  `ValidateStepEndpoints` finds nothing to check (`internal/sync/endpoint.go`
  `endpointFromConfig` returns `false` on a missing or blank endpoint), and
  `createProvider` copies `cred.Endpoint` in *after* validation has already run
  (`internal/sync/pipeline.go`). A credential saved before v0.4.0 over plain http keeps
  working until someone re-saves it. `docs/sync.md` states this deliberately; it is an accepted
  gap, not a met requirement, and it weakens FR-021 and SC-005.
- **`internal/worker/jobs/sync_job.go` performs no endpoint validation of its own.** The
  handler validates before enqueueing (`internal/handler/sync_handler.go` `CardDAVTrigger`),
  and a `credential_id` resolved inside the job is not checked at all. The job also does not
  verify that the resolved credential belongs to the payload's user — unlike
  `internal/sync/pipeline.go` `createProvider`, which does check `cred.UserID != userID`. No
  current caller enqueues a `credential_id`, so this is unreachable today, but the asymmetry
  is real and FR-032 does not hold on that path.
- **Provider passwords and OAuth tokens are stored in plaintext.** Stated as a deliberate
  choice for a self-hosted deployment in `internal/domain/provider_connection.go`. Inline
  step configs are worse: a username and password written directly into a step live in
  `pipeline_steps.source_config` as JSON (`internal/sync/pipeline.go` `providerConfig`), which
  FR-051 does not cover because that field is not a secret field on a connection row.
- **The PKCE verifier is stored in the connection's `password` column** while the OAuth flow
  is in flight and cleared on success (`internal/service/google_oauth.go`). A flow that is
  never completed leaves the verifier behind until the next `GetAuthURL` deletes the pending
  row.
- **The OAuth `state` parameter is the connection's UUID** (`AuthCodeURL(connID, …)`), and
  `HandleCallback` accepts any state that resolves to a not-yet-connected row without
  binding it to a session. A leaked authorisation URL is a leaked pending connection.
  Still open: is binding `state` to the initiating session intended, or is the unguessable
  UUID considered sufficient for a self-hosted single-user deployment?
- **`ApplyResolution` has no wildcard.** The conflict UI, when a conflict carries no stored
  field diffs, sets `resolution['*'] = 'remote'` to mean "take the remote card wholesale"
  (`web/src/views/sync/SyncConflictDetailView.vue` `resolveAllRemote`). The server reads only
  per-field keys and defaults anything unlisted to local (`internal/sync/merger.go`
  `ApplyResolution`). The "Apply remote (source wins)" button in that branch therefore keeps
  the local card. Confirmed by reading both sides; there is no test covering it, and
  `docs/sync.md`'s claim that resolution is per-field for all conflicts does not survive it.
- **`MergeVCards` compares whole property sets as sorted, joined strings**
  (`internal/sync/merger.go` `serializeField`). Two edits to *different* email addresses on
  the same card are one `EMAIL` conflict, not two, and resolution is all-or-nothing per
  property type. Parameters (`TYPE=work`) are not part of the comparison at all — only
  `Field.Value` is — so a change to a phone's type alone is invisible to the merge. FR-038 is
  true at property-set granularity, not at value granularity.
- **Conflict resolution addresses the contact by UID, not by database id.**
  `SyncConflictService.Resolve` looks up `contactRepo.GetByUID(ctx, ab.ID,
  conflict.LocalContactID)` (`internal/service/sync_conflict.go`), which is correct only
  because `sync_states.local_id` holds the internal provider's `RemoteID`, which is the
  contact's UID (`internal/sync/internal_provider.go`). Renaming either concept breaks
  resolution silently, and no test spans both sides of that coupling.
- **`sync_states.provider_uri` and `sync_states.sync_token` are dead columns.** Neither is
  read or written anywhere outside the struct definition (`internal/domain/sync_state.go`;
  `grep` for `ProviderURI` and `.SyncToken` finds no engine use — the incremental cursor lives
  in `sync_cursors`). They cannot be dropped: migrations are forward-only.
- **Migration 019 is a bulk `UPDATE` inside a migration**, which constitution Principle I and
  `CLAUDE.md` both now forbid. It rewrites `pipeline_steps`, swaps `sync_states` sides and
  deletes inverted `sync_conflicts`
  (`migrations/019_normalize_pipeline_direction.up.sql`). It predates the rule; it is recorded
  here so it is not read as precedent. It is the one migration in this domain with test
  coverage (`internal/repository/migrate_019_test.go`), which is why it is survivable.
- **`POST /pipelines/:id/trigger` runs the whole pipeline synchronously inside the HTTP
  request** (`internal/handler/pipeline_handler.go` `Trigger` calls `orchestrator.Execute`
  directly). It is behind the shared expensive-operation rate limiter
  (`internal/handler/handler.go`) and depends on `server.write_timeout` staying 0. The
  scheduled path, by contrast, goes through the worker queue — so the same pipeline has two
  materially different execution environments depending on how it was started.
- **The job queue is in-memory.** A scheduled sync is drained on graceful shutdown but lost on
  SIGKILL; it runs at the next scheduled tick. Verified only as a documentation claim
  (`docs/sync.md`, Limitations); the queue implementation is owned by
  `008-runtime-configuration-and-delivery`.
- **A CardDAV server without `sync-collection` support is detected only on the first run.**
  `ListChanges` falls back to a full `List` when the report fails with an empty cursor and
  reports no cursor, so every subsequent run is a full sync too
  (`internal/sync/carddav_client.go` `ListChanges`). Nothing records the capability, so the
  failed report is paid on every run.
- **CardDAV delta deletions assume the file is named after the UID.** `extractUIDFromPath`
  takes the last path segment minus `.vcf` (`internal/sync/carddav_client.go`). A server that
  names files otherwise still syncs correctly on a full listing, so the failure is a silently
  degraded incremental path rather than an error.
- **`skip_tls_verify` disables certificate verification entirely** when set on a credential or
  step (`internal/sync/carddav_client.go` `NewCardDAVClientProviderWithOptions`). It is a
  per-connection opt-in with no deployment-level override, so an operator cannot forbid it.
- **Google's sync-token and conditional-write paths have unit coverage but no live-Google
  coverage.** The tests in `internal/sync/engine_incremental_test.go` and
  `internal/sync/google_provider_test.go` use fakes (`docs/sync.md`, Limitations). FR-005,
  FR-068, FR-069 and FR-070 are asserted against this project's model of the People API, not
  against the API.
- **`ConflictSkip` is offered in the UI but is not distinguished in the engine.** The pull
  path skips on any mode that is not `source_wins` and records a conflict only for `auto` and
  `manual` (`internal/sync/engine.go`); the push path overwrites for `dest_wins` and otherwise
  skips. So `skip` behaves as "skip, record nothing" and `dest_wins` behaves as "skip" on
  import. The five options in `web/src/views/pipelines/PipelineCreateView.vue` map onto three
  engine behaviours per phase, and nothing tells the user that.
- **This spec cites `web/src/router/index.ts` (FR-081, FR-082) and
  `web/src/router/routes.spec.ts` without claiming either.** The routes named are this
  domain's; the file is not, and both `001-identity-and-credentials` and
  `008-runtime-configuration-and-delivery` list it. Ownership granularity is the path, so the
  route table cannot be split the way this domain would need. Still open: whether a shared
  route table needs an owner of record.
- **The `sync_runs` boundary is split across two specs.** This spec owns the table, its
  repository and its retention setting; `008-runtime-configuration-and-delivery` owns both
  boot-time operations on it (FR-019, FR-020). That is the correct resolution under Principle
  VII — `cmd/server/startup.go` cannot be owned by halves — but it means a reader looking for
  "what happens to sync history at boot" must follow a cross-reference.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-07 | v0.4.0 | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | v0.4.0 | Rewritten to the house template: header replaced with `Kind`/`Status`/`Constitution` (`Feature Branch`, `Created`, `Source` and the `How to read this` blockquote removed or moved to prose); `Dependencies` and `Out of Scope` folded into Assumptions; `Status`, `Code Paths`, `References`, `Enforced By`, `Known Divergences` and `Amendments` placed in template order. Ownership narrowed from `internal/sync/` as a package to an explicit file list, and the partial `cmd/server/startup.go` claim dropped. Every admission moved out of Edge Cases into Known Divergences; Edge Cases restated as genuine boundary conditions. Test names and `.go` paths removed from Success Criteria and reconciled into Enforced By, with twenty-plus unenforced requirements named as gaps. FR-019 and FR-020 replaced by cross-references to `008-runtime-configuration-and-delivery` FR-041 and FR-042. The `migrations/021_change_journal` open question closed: not claimed here, referenced as `004-carddav-service`'s. Status `Implemented (retrospective)` → `shipped`. | — |
