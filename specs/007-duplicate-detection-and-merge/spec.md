# Feature Specification: Duplicate Detection & Contact Merge

Kind: journey
Status: shipped
Constitution: v1.0.0

Reconstructed from the implementation at commit `23a167c` (`v0.4.0` plus three follow-up commits,
`v0.4.0-3-g23a167c`). Nothing below was designed and then built: every requirement was read out of
the shipped code at the cited path before it was written down. Where the implementation has a
deliberate limitation, a stale string or a genuine gap, it is recorded under **Known Divergences**
rather than dressed up as a requirement that is met.

The subject is contact data quality inside one address book: finding pairs that are probably the
same person, and combining two records into one.

## User Scenarios & Testing *(mandatory)*

The user is the owner of one address book. Everything here is scoped to that owner: detection
runs against the caller's address book, and every stored pair carries the caller's user id.

### User Story 1 - Find the records that are probably the same person (Priority: P1)

An address book that has absorbed contacts from a phone, a Google account and a CSV export holds
the same person three times. The owner opens **Contacts → Duplicates**, presses **Scan now**, and
gets back a list of pairs, each one saying in words why it was paired: "Same email:
ada@example.com".

**Why this priority**: Nothing else in this domain is reachable without it. A merge screen with
no pairs to merge is dead weight, and a detector that cannot finish on a real address book is the
same as no detector at all — before the current implementation, a scan of 10 000 contacts took
39 seconds and allocated 13.6 GB (`internal/service/duplicate_detect_test.go:153-165`).

**Independent Test**: Seed two contacts sharing an email address and two sharing a normalised
phone number, run detection, and read the stored pairs and their reasons without opening the
merge screen.

**Acceptance Scenarios**:

1. **Given** two contacts with the same email address in different letter case, **When**
   detection runs, **Then** one pair is recorded with score 1.0 and the single reason
   `email_match` carrying the matched address
   (`internal/service/duplicate_detect_test.go:86-110`).
2. **Given** two contacts whose phone numbers differ only in punctuation, **When** detection
   runs, **Then** one pair is recorded with score 0.8, reasons `phone_match` plus `name_exact`
   or `name_similar` (`internal/service/duplicate_detect_test.go:90-108`).
3. **Given** two contacts with identical names and nothing else in common, **When** detection
   runs, **Then** **no** pair is recorded — a name alone cannot reach the threshold
   (`internal/service/duplicate_detect_test.go:97-117`).
4. **Given** a pair reachable through both a shared email and a shared phone, **When** detection
   runs, **Then** exactly one row is stored, scored by the stronger key
   (`internal/service/duplicate_detect_test.go:235-248`).
5. **Given** a scan has already been run and nothing has changed, **When** it is run again,
   **Then** zero new pairs are reported (`internal/service/duplicate_detect_test.go:121-135`).
6. **Given** two contacts with different primary addresses that share only a **second** email —
   or a **second** phone number — **When** detection runs, **Then** they are paired exactly as if
   the shared value had been the primary one
   (`internal/service/duplicate_detect_test.go:329-372`). Before this behaviour existed, the
   pair list could offer a "lossless" one-click merge for two records the detector had never
   found, because the subset check reads the same tables the detector did not.

---

### User Story 2 - Combine two records without losing anything I wanted (Priority: P1)

Two records for the same person each hold something the other does not: one has the work address,
the other the home address and a second phone number. The owner opens the merge screen, picks
which record survives, ticks the values to keep from either side, sees exactly what will be
discarded, and confirms.

**Why this priority**: This is the payoff. It is P1 alongside Story 1 because before v0.4.0 the
per-field choice on this screen **did nothing** — the UI sent keys like `first_name` while the
server resolved by vCard property name, so every key missed and the winner's value always won
(`CHANGELOG.md:107-112`). A merge screen that silently ignores the user's choices is worse than
no screen, because it destroys data while looking careful.

**Independent Test**: Open a pair with a work address on one side and a home address on the
other, keep both, merge, and read the surviving card.

**Acceptance Scenarios**:

1. **Given** a pair whose two cards hold different addresses, **When** both are ticked and the
   merge is confirmed, **Then** the surviving card carries both — the winner's values first,
   then the loser's additions (`internal/vcard/merge_cards.go:192-207`).
2. **Given** a multi-valued property, **When** the screen loads, **Then** every value from both
   sides is selected by default (`web/src/utils/merge.ts:91-102`).
3. **Given** a single-valued property such as the display name, **When** the screen loads,
   **Then** the surviving record's value is preselected, falling back to the other side when the
   winner has none (`web/src/utils/merge.ts:103-109`).
4. **Given** a value is unticked, **When** the preview updates, **Then** that value appears by
   name in a "Will be discarded" list, struck through and labelled with its field — not merely
   counted (`web/src/components/contacts/MergePreviewCard.vue:41-56`).
5. **Given** the user changes which record survives, **When** the choice flips, **Then** the
   single-valued defaults are rebuilt for the new winner while multi-valued groups still keep
   everything (`web/src/views/contacts/ContactMergeView.vue:172-174`).
6. **Given** a merge is confirmed, **When** it completes, **Then** exactly one contact remains
   and the removed one is gone (`internal/service/merge_service_test.go:149-160`).

---

### User Story 3 - The merge does not confuse the devices already syncing (Priority: P1)

The owner merges two contacts. The phone syncing over CardDAV learns that one card changed and
one card was removed — it does not re-create the deleted record on the next sync, and it does not
keep showing a contact that no longer exists.

**Why this priority**: A merge that leaves the sync layer inconsistent undoes itself. Before
v0.4.0 the merge wrote through a path that skipped seven child tables and did not advance the
collection's change counter, so CardDAV clients never learned the surviving contact had changed
(`CHANGELOG.md:111-113`). Leaving the loser's sync state behind makes the next export re-create
it from the remote or raise a conflict for a contact that no longer exists
(`internal/service/merge_service.go:166-169`).

**Independent Test**: Merge two contacts and inspect the change journal and the sync-state rows
directly; no UI involved.

**Acceptance Scenarios**:

1. **Given** a merge, **When** it completes, **Then** the winner's update and the loser's
   deletion are written in one transaction
   (`internal/repository/bun_contact_relations.go:66-95`).
2. **Given** a merge, **When** a sync-collection report spans it, **Then** it returns exactly two
   entries — the updated winner and the removed loser — sharing one sequence number
   (`internal/repository/bun_contact_relations.go:61-65`, `internal/repository/merge_into_test.go:69`).
3. **Given** the removed contact had sync state for several providers, **When** the merge
   completes, **Then** every one of those rows is deleted and the survivor's are untouched
   (`internal/service/merge_log_service_test.go:102-119`).
4. **Given** the loser's card cannot be parsed, **When** a merge is attempted, **Then** it fails
   and neither contact is modified
   (`internal/service/merge_service_test.go:241-253`).

---

### User Story 4 - Undo a merge I regret (Priority: P2)

Two days later the owner realises the wrong record was discarded. The merge is recorded, with a
snapshot of the discarded card, and the record survives the deletion of both contacts involved.

**Why this priority**: Merging is destructive and irreversible in the data model — the loser row
is deleted, not tombstoned as a contact. Below P1 because the recovery path is manual and, at
`23a167c`, has no user interface at all (see **Known Divergences**).

**Independent Test**: Merge two contacts, delete the survivor as well, then read
`GET /contacts/merge-log` and confirm the record and the snapshot are still there.

**Acceptance Scenarios**:

1. **Given** a merge, **When** it completes, **Then** a record exists carrying both display
   names, the discarded record's UID, a copy of its card and the choices that were made
   (`internal/service/merge_service.go:232-242`).
2. **Given** both contacts are later deleted, **When** the history is read, **Then** the record
   is still present — it holds no foreign key to contacts
   (`migrations/023_merge_log.up.sql:1-10`, `internal/repository/merge_log_test.go:17`).
3. **Given** the discarded card carried a photo, **When** the snapshot is stored, **Then** the
   PHOTO, LOGO and SOUND values are stripped
   (`internal/service/merge_service.go:239`, `internal/vcard/encoder.go:165-180`).
4. **Given** the retention window has passed, **When** pruning runs, **Then** the record is
   removed (`internal/worker/jobs/dedup_job.go:44-59`).

---

### User Story 5 - Merge without reading a screen when there is nothing to read (Priority: P2)

Two records where one is strictly a subset of the other. The list offers a single **Keep A**
button, because the server has confirmed that keeping A loses nothing.

**Why this priority**: Most duplicates in practice are one full record and one stub. Forcing the
full merge screen for those makes the feature tedious enough to abandon. It sits below the
correctness stories because the guard — offering the shortcut only when it is provably lossless —
is what makes it acceptable at all.

**Independent Test**: Create a pair where B's emails and phones are all present on A, list the
pairs, and confirm only **Keep A** is offered.

**Acceptance Scenarios**:

1. **Given** B's every email and phone is also on A, **When** the list is rendered, **Then**
   **Keep A** is offered (`internal/repository/bun_potential_duplicate.go:131-181`,
   `web/src/utils/duplicates.ts:71-84`).
2. **Given** each record holds something the other does not, **When** the list is rendered,
   **Then** neither quick button appears and the row says so in words
   (`web/src/views/contacts/DuplicatesView.vue:145-147`).
3. **Given** the subset flags are absent from the response, **When** the list is rendered,
   **Then** the shortcut is withheld — unknown reads as unsafe
   (`web/src/utils/duplicates.ts:71-84`).
4. **Given** a quick merge is clicked, **When** it is about to run, **Then** a confirmation names
   both records and states that one will be deleted
   (`web/src/views/contacts/DuplicatesView.vue:283-289`).

---

### User Story 6 - Let it run on its own (Priority: P3)

The owner turns on scheduled detection at 02:00 and stops thinking about it. A badge in the
sidebar shows how many pairs are waiting.

**Why this priority**: Convenience over a manual scan that already works. Real, but nothing
breaks without it.

**Independent Test**: Save a schedule, confirm the job is registered in the running scheduler
without a restart, then remove it.

**Acceptance Scenarios**:

1. **Given** a cron expression that does not parse, **When** the setting is saved with detection
   enabled, **Then** it is rejected (`internal/handler/duplicate_handler.go:248-252`).
2. **Given** a valid enabled schedule is saved, **When** the response returns, **Then** the job
   is already registered in the running scheduler — no restart
   (`internal/handler/duplicate_handler.go:264-270`, `internal/worker/scheduler.go:161-202`).
3. **Given** detection is switched off, **When** the setting is saved, **Then** the scheduled job
   is removed (`internal/handler/duplicate_handler.go:267-269`).
4. **Given** the server restarts, **When** it boots, **Then** every enabled schedule is
   re-registered from the database (`cmd/server/main.go:177-186`).
5. **Given** pending pairs exist, **When** any page loads, **Then** the sidebar shows the count
   (`internal/handler/duplicate_handler.go:143-150`,
   `web/src/components/layout/Sidebar.vue:88-97`).

---

### User Story 7 - Tell the system a pair is not a duplicate (Priority: P3)

Two colleagues share an office phone number. The owner dismisses the pair, and it does not come
back on the next scan.

**Why this priority**: Without it, a false positive is permanent noise. P3 because the list can
be filtered and the pair ignored; dismissal makes that durable rather than possible.

**Independent Test**: Dismiss a pair, re-run detection, confirm it is not reported as new and
still reads `dismissed`.

**Acceptance Scenarios**:

1. **Given** a pending pair, **When** it is dismissed, **Then** its status becomes `dismissed`
   (`internal/handler/duplicate_handler.go:169-190`).
2. **Given** a dismissed pair, **When** detection runs again, **Then** no new row is created —
   the unique index refuses it (`migrations/024_potential_duplicates_unique.up.sql:11-12`,
   `internal/repository/bun_potential_duplicate.go:30-43`).
3. **Given** a dismissed pair, **When** the filter is set to `dismissed` or `all`, **Then** it is
   listed (`internal/handler/duplicate_handler.go:74-80`,
   `internal/repository/bun_potential_duplicate.go:141-145`).
4. **Given** a pair belonging to another account, **When** dismissal is attempted, **Then** it is
   refused with 403 (`internal/handler/duplicate_handler.go:181-183`).

---

### Edge Cases

Boundary conditions the shipped code answers. Where the answer is narrower or rougher than a
reader would expect, the entry says so and **Known Divergences** carries the admission.

- **What happens when one key is shared by hundreds of contacts?** The bucket is skipped
  entirely above 500 members, on the ground that a value shared by that many people does not
  identify a person (`internal/service/duplicate_detector.go:26,136-142`). The skip is a log
  warning only — see Known Divergences.
- **What happens when two scans for the same user overlap?** The second is refused before any
  work is done and the API answers 409
  (`internal/service/duplicate_detector.go:94-97,204-218`,
  `internal/handler/duplicate_handler.go:157-163`).
- **What happens when a scan is cancelled mid-flight?** Cancellation is checked between buckets
  and between inserts, so an aborted scan leaves the pairs it had already written and adds no
  more (`internal/service/duplicate_detector.go:130-132,185-187`).
- **What happens when the same pair is discovered through two different keys?** One row, scored
  by the stronger key, with contact ids normalised smallest-first
  (`internal/service/duplicate_detector.go:125-127,155-167`).
- **What happens when either card cannot be decoded?** The merge fails before anything is
  written; neither contact is touched (`internal/service/merge_service.go:127-146`,
  `internal/service/merge_service_test.go:241-253`).
- **What happens when the discarded contact has already been deleted by someone else?** The
  transaction still saves the winner rather than failing
  (`internal/repository/bun_contact_relations.go:87-89`,
  `internal/repository/merge_into_test.go:97`).
- **What happens when the merge history cannot be written?** The merge proceeds and the failure
  is logged as a warning (`internal/service/merge_service.go:244-247`). The service also runs
  with no history repository configured at all (`internal/service/merge_service.go:34-36`).
- **What happens when a selection mentions no properties at all?** The winner's card survives
  unchanged — an empty selection means "keep the winner"
  (`internal/vcard/merge_cards.go:176-185`).
- **What happens on an upgrade where `(a,b)` and `(b,a)` both exist?** Migration 024 refuses to
  create its unique index, the transaction aborts and the server does not start until an
  operator resolves it by hand — stated as intended inside the migration
  (`migrations/024_potential_duplicates_unique.up.sql:6-9`).

## Requirements *(mandatory)*

### Functional Requirements

**Finding candidate pairs**

- **FR-001**: Detection MUST group a user's contacts by the keys that can produce a match — the
  lower-cased trimmed email address and the digits-only phone number — and compare only within a
  group. Email keys and phone keys MUST NOT collide.
  (`internal/service/duplicate_detector.go:122-172`)
- **FR-001a**: Those keys MUST be drawn from **every** email and phone number a contact holds,
  not only from the `contacts.email` and `contacts.phone` columns: a person's second address
  identifies them exactly as well as their first, and the subset check that decides whether a
  one-click merge is lossless has always read `contact_emails` and `contact_phones` (FR-018), so
  a detector that could not see them let the list prove a pair safe to collapse on data the
  detector was never shown. The child values MUST be **added** to the keys from the flat columns,
  never substituted for them — migration 014 created the child tables and backfilled nothing
  (constitution Principle I), so a contact stored before it has a populated `contacts.email` and
  no `contact_emails` row until something re-saves it.
  (`internal/service/duplicate_detector.go:109-117,151-172`,
  `internal/repository/bun_contact.go:320-359`)
- **FR-001b**: One contact MUST appear in one bucket at most once. Nearly every contact holds its
  primary address in both the flat column and the child table, and a repeated entry would not
  create a false pair — it would inflate `len(bucket)` against the cap of FR-010, pushing a
  scannable bucket over it and getting the whole bucket skipped.
  (`internal/service/duplicate_detector.go:126-135`)
- **FR-002**: Contacts sharing no key MUST never be compared.
  (`internal/service/duplicate_detector.go:129-182`,
  `internal/service/duplicate_detect_test.go:199-215`)
- **FR-003**: A shared email MUST score 1.0; a shared normalised phone MUST score 0.8. A pair
  MUST be recorded only at or above 0.8, which makes a name match alone insufficient by
  construction. (`internal/service/duplicate_detector.go:19,237-265`)
- **FR-004**: An email-matched pair MUST carry exactly one reason. A phone-matched pair MUST
  additionally carry `name_exact` when the two full names are equal ignoring case, or
  `name_similar` when their Levenshtein distance is at most 2.
  (`internal/service/duplicate_detector.go:241-264`)
- **FR-005**: Each reason MUST record the value that matched, not only the kind of match, so the
  explanation is not recomputed on the client — a contact with several phone numbers makes that
  guess wrong about as often as right.
  (`internal/service/duplicate_detector.go:64-73`, `web/src/utils/duplicates.ts:40-57`)
- **FR-006**: A reader of stored reasons MUST accept both the current object form and the older
  bare-string form, and MUST treat unreadable JSON as "no explanation" rather than an error.
  (`web/src/utils/duplicates.ts:12-37`)
- **FR-007**: A pair MUST be stored with the smaller contact id first, whichever order it was
  discovered in. (`internal/service/duplicate_detector.go:158-162`,
  `internal/service/duplicate_detect_test.go:219-232`;
  `internal/repository/bun_potential_duplicate.go:188-191`)
- **FR-008**: At most one row per `(user_id, contact_a_id, contact_b_id)` MUST exist, enforced by
  a unique index, and inserts MUST be insert-or-ignore rather than select-then-insert so two
  overlapping scans cannot both decide the pair is new.
  (`migrations/024_potential_duplicates_unique.up.sql:11-12`,
  `internal/repository/bun_potential_duplicate.go:30-43`)
- **FR-009**: A pair reachable through more than one key MUST be recorded once, at the stronger
  score. (`internal/service/duplicate_detector.go:125-127,164-167`)
- **FR-010**: A key shared by more than 500 contacts MUST be skipped, with a warning whose key
  value is truncated so an address is not written into the log.
  (`internal/service/duplicate_detector.go:26,136-142,220-230`)
- **FR-011**: A second detection run for the same user MUST be refused while one is in progress,
  and the API MUST answer 409 with a message saying so.
  (`internal/service/duplicate_detector.go:28-29,94-97,204-218`,
  `internal/handler/duplicate_handler.go:157-163`)
- **FR-012**: Detection MUST honour cancellation of its context, checked between buckets and
  between inserts. (`internal/service/duplicate_detector.go:183-185,238-240`)
- **FR-013**: Detection MUST read only the columns it compares, not whole contact rows. The
  child values of FR-001a MUST come from two narrow `(contact_id, value)` projections, not from
  a relation load and not from a join onto the contact rows: `Relation("Emails")` pulls
  `vcard_data` and `photo_uri` — tens of megabytes read and discarded on ten thousand contacts —
  and a join repeats every selected contact column once per child row.
  (`internal/repository/bun_contact.go:305-359`, `internal/domain/contact.go:61-70`)
- **FR-014**: Detection MUST report how many contacts were examined and how many **new** pairs
  were recorded. (`internal/service/duplicate_detector.go:59-62,109,195-197`)

**Reading and dismissing pairs**

- **FR-015**: The pair list MUST default to `pending`, MUST treat an explicitly empty status as
  "every status", and MUST support `all` as the way to ask for everything.
  (`internal/handler/duplicate_handler.go:74-80`,
  `internal/repository/bun_potential_duplicate.go:121-122,141-145`)
- **FR-016**: The page size MUST default to 20 and MUST be **clamped** to 100 when a larger value
  is asked for — not reset to the default, which made a pair past the twentieth unreachable.
  (`internal/handler/duplicate_handler.go:67-91`)
- **FR-017**: The list MUST be ordered by score then recency, and MUST return the total so the UI
  can page. (`internal/repository/bun_potential_duplicate.go:147-150`)
- **FR-018**: The list MUST report, per pair and computed in SQL, whether one side's emails and
  phones are wholly contained in the other's — comparing emails case-insensitively and phones by
  digits only, matching how the detector decides identity.
  (`internal/repository/bun_potential_duplicate.go:124-181`)
- **FR-019**: The list MUST NOT load the two contacts' child collections; fourteen joins per page
  for data nothing displays is why the single-pair endpoint exists.
  (`internal/repository/bun_potential_duplicate.go:124-139`)
- **FR-020**: Reading one pair MUST return both contacts with all seven child collections, plus
  the list of selectable value candidates.
  (`internal/repository/bun_potential_duplicate.go:67-119`,
  `internal/handler/duplicate_handler.go:106-140`)
- **FR-021**: Value identifiers MUST be minted on the server and returned alongside the pair in
  the duplicate detail response; a client MUST NEVER recompute them. They are content hashes over
  the property name, the value and its parameters, and a second implementation in TypeScript
  would have to agree with this one byte for byte forever — a disagreement would silently drop
  values from a merge. (`internal/handler/duplicate_handler.go:125-138`,
  `internal/vcard/merge_cards.go:54-81`)
- **FR-022**: Reading one pair MUST filter on ownership inside the query, not in a check the
  caller is trusted to remember.
  (`internal/repository/bun_potential_duplicate.go:62-74`,
  `internal/repository/duplicate_pair_test.go:103`)
- **FR-023**: Dismissal MUST set the pair's status to `dismissed`, MUST be refused for another
  user's pair, and MUST survive later scans by virtue of FR-008.
  (`internal/handler/duplicate_handler.go:169-190`)
- **FR-024**: The count of pending pairs MUST be available as its own endpoint so a navigation
  badge does not have to fetch a page.
  (`internal/handler/duplicate_handler.go:142-150`,
  `internal/repository/bun_potential_duplicate.go:220-225`)

**Scheduling**

- **FR-025**: Each user MUST have their own detection schedule and on/off switch, defaulting to
  disabled at `0 2 * * *`. (`migrations/016_user_dedup_settings.up.sql`,
  `internal/handler/duplicate_handler.go:226-232`)
- **FR-026**: A cron expression MUST be validated before being saved when detection is enabled.
  (`internal/handler/duplicate_handler.go:248-252`)
- **FR-027**: Saving a schedule MUST take effect in the running scheduler immediately — register
  or re-register when enabled, remove when disabled — with no restart.
  (`internal/handler/duplicate_handler.go:264-270`, `internal/worker/scheduler.go:161-202`)
- **FR-028**: Enabled schedules MUST be re-registered from the database at startup.
  (`cmd/server/main.go:177-186`)
- **FR-029**: A scheduled firing MUST enqueue a job rather than run detection on the scheduler's
  goroutine. (`internal/worker/scheduler.go:169-176`,
  `internal/worker/jobs/dedup_job.go:61-82`)

**Merging: which value wins**

- **FR-030**: A merge MUST accept a per-value selection — vCard property name to the value ids to
  keep — and that form MUST take precedence over the older whole-property form. It is the only
  way to express "the work address from one record and the home address from the other".
  (`internal/service/merge_service.go:81-94,186-202`)
- **FR-031**: The older whole-property form (`winner`/`loser` per property) MUST keep working, so
  the one-click "keep this one" buttons need no per-value knowledge.
  (`internal/service/merge_service.go:191-201`)
- **FR-032**: UID and VERSION MUST always come from the winner of a merge, never from the
  discarded record and never regenerated: every synced device knows the contact by that UID.
  (`internal/vcard/merge_cards.go:146-169`)
- **FR-033**: A property the selection does not mention MUST keep the winner's values. An empty
  selection therefore means "keep the winner", which is the safe default for a caller that says
  nothing. (`internal/vcard/merge_cards.go:176-185`)
- **FR-034**: The merged card MUST be re-parsed and applied to every modelled field of the
  surviving contact; a parse failure MUST abort the merge rather than substitute empty values.
  (`internal/service/merge_service.go:127-146`,
  `internal/service/merge_service_test.go:241-253`)

**Merging: what it does to the database**

- **FR-035**: The surviving contact, its child rows and the deletion of the discarded contact
  MUST be written in one transaction.
  (`internal/repository/bun_contact_relations.go:54-95`,
  `internal/service/merge_service.go:159-164`)
- **FR-036**: That transaction MUST record the deletion in the change journal, at the same
  sequence number as the winner's update, so a CardDAV sync-collection report spanning the merge
  returns exactly two entries.
  (`internal/repository/bun_contact_relations.go:61-65,78-93`,
  `internal/repository/merge_into_test.go:15,69`)
- **FR-037**: A merge whose discarded contact no longer exists MUST still save the winner.
  (`internal/repository/bun_contact_relations.go:87-89`,
  `internal/repository/merge_into_test.go:97`)
- **FR-038**: Every provider's sync state for the discarded contact MUST be deleted; the
  survivor's MUST be left alone.
  (`internal/service/merge_service.go:250-274`,
  `internal/service/merge_log_service_test.go:102-119`)
- **FR-039**: Pair rows referencing either contact MUST be removed, and a failure to remove them
  MUST be logged rather than silently swallowed.
  (`internal/service/merge_service.go:152-157,171-175`)
- **FR-040**: Winner and loser MUST both belong to the caller's address book, and MUST differ.
  (`internal/service/merge_service.go:98-120`, `internal/handler/duplicate_handler.go:200-214`)
- **FR-041**: When the request names the pair it resolves, that pair's ownership MUST be verified
  before the merge, because the id is recorded against the merge.
  (`internal/service/merge_service.go:107-111,204-217`)

**Merge history**

- **FR-042**: Every merge MUST leave a record carrying the surviving contact's id and display
  name, the discarded contact's UID and display name, a copy of the discarded card, the choices
  made, and a timestamp. (`internal/service/merge_service.go:221-247`,
  `migrations/023_merge_log.up.sql:11-21`)
- **FR-043**: That record MUST hold no foreign key to contacts, so deleting either contact — or
  both — does not destroy the history. (`migrations/023_merge_log.up.sql:1-10`,
  `internal/repository/merge_log_test.go:17`)
- **FR-044**: The stored snapshot MUST have its PHOTO, LOGO and SOUND values stripped, to keep a
  row from carrying hundreds of kilobytes. (`internal/service/merge_service.go:239`,
  `internal/vcard/encoder.go:165-180`)
- **FR-045**: A failure to write the record MUST NOT abort the merge, and MUST be logged as a
  warning. (`internal/service/merge_service.go:244-247`,
  `internal/service/merge_log_service_test.go:122-132`)
- **FR-046**: The service MUST work with no history repository configured at all.
  (`internal/service/merge_service.go:34-36,222-224`,
  `internal/service/merge_log_service_test.go:134-140`)
- **FR-047**: History MUST be readable per user, newest first, with a bounded page size.
  (`internal/handler/duplicate_handler.go:48-65`, `internal/repository/bun_merge_log.go:29-41`)
- **FR-048**: Records older than a configured retention window MUST be pruned, and the window
  MUST be configurable with a default of 30 days.
  (`internal/worker/jobs/dedup_job.go:41-59`, `internal/config/config.go:146-151,193`,
  `configs/config.example.yaml:66-70`)
- **FR-049**: Pruning MUST run on the detection job **and** once at startup, so an instance with
  no detection schedule still prunes. (`internal/worker/jobs/dedup_job.go:80`,
  `cmd/server/main.go:142-144`)

**The screens**

- **FR-050**: The merge screen MUST present the choice of which record survives separately from
  the choice of which values are kept, and MUST say why: the survivor keeps the identifier that
  synced devices know it by (FR-032). (`web/src/views/contacts/ContactMergeView.vue:42-57`,
  `web/src/components/contacts/MergeContactColumn.vue`)
- **FR-051**: Values MUST be grouped by property, with mutually exclusive choices for
  single-valued properties and independent choices for multi-valued ones.
  (`web/src/components/contacts/MergeFieldGroup.vue:20-70`)
- **FR-052**: A group where the two sides differ MUST be marked in words, not by colour alone.
  (`web/src/components/contacts/MergeFieldGroup.vue:7-18`)
- **FR-053**: The screen MUST show what the merged record will contain and MUST name every value
  that will be discarded, derived from the current selection with no request to the server.
  (`web/src/components/contacts/MergePreviewCard.vue`, `web/src/utils/merge.ts:115-136`)
- **FR-054**: Changing which record survives MUST rebuild the single-valued defaults.
  (`web/src/views/contacts/ContactMergeView.vue:163-174`)
- **FR-055**: The request MUST take the winner from the user's explicit choice, never inferred
  from which side more values were taken from — inference could silently change which UID
  survives. (`web/src/utils/merge.ts:148-172`)
- **FR-056**: The merge screen MUST load its pair by id in one request, not by searching a page
  of results. (`web/src/views/contacts/ContactMergeView.vue:176-188`,
  `web/src/api/contacts.ts:87-97`)
- **FR-057**: A pair that is gone MUST be distinguished from a request that failed: the first
  offers a way back, the second a retry.
  (`web/src/views/contacts/ContactMergeView.vue:12-28,189-198`)
- **FR-058**: A failed merge MUST leave the screen mounted with the selection intact.
  (`web/src/views/contacts/ContactMergeView.vue:218-223`)
- **FR-059**: The one-click merge MUST be offered only when the server has confirmed the other
  record holds nothing extra; absent flags MUST read as unsafe.
  (`web/src/views/contacts/DuplicatesView.vue:118-147`, `web/src/utils/duplicates.ts:71-84`)
- **FR-060**: Any merge MUST be confirmed first, naming both records and stating that one will be
  deleted. (`web/src/views/contacts/DuplicatesView.vue:169-177,283-289`,
  `web/src/views/contacts/ContactMergeView.vue:88-97,149-156`)
- **FR-061**: The list MUST render the match as an explanation and a two-valued confidence label,
  not as a percentage. (`web/src/utils/duplicates.ts:40-69`,
  `web/src/views/contacts/DuplicatesView.vue:84-92`)

### Key Entities

- **Potential duplicate pair** — `potential_duplicates`. One row per unordered pair of contacts
  belonging to one user, holding a score, the reasons as JSON, and a status of `pending` or
  `dismissed`. Contact ids are stored smallest-first and are unique per user. Cascades away when
  either contact is deleted. (`internal/domain/potential_duplicate.go`,
  `migrations/006_potential_duplicates.up.sql`,
  `migrations/024_potential_duplicates_unique.up.sql`)
- **Match reason** — a code (`email_match`, `phone_match`, `name_exact`, `name_similar`) plus the
  value that matched. Stored as a JSON array in the pair row; older rows hold bare codes.
  (`internal/service/duplicate_detector.go:64-73`)
- **Subset flags** — `b_subset_of_a` / `a_subset_of_b`, computed per row by the list query and
  never stored. Present on list reads, absent on single-pair reads.
  (`internal/domain/potential_duplicate.go:26-30`,
  `internal/repository/bun_potential_duplicate.go:153-181`)
- **Value candidate** — one selectable value from either card: a content-hash id (FR-021), the
  property, the value, its parameters, and which side it came from (`winner`, `loser`, or `both`
  when identical). Minted per request, never stored. (`internal/vcard/merge_cards.go:44-52`)
- **Selection** — vCard property name to the set of value ids to keep. The per-value form of a
  merge request; a property it does not name keeps the winner's values (FR-033).
  (`internal/vcard/merge_cards.go:42`, `internal/service/merge_service.go:85-89`)
- **Merge record** — `merge_log`. One row per merge: both display names, the discarded record's
  UID, a photo-stripped copy of its card, the choices as JSON, and a timestamp. No foreign key to
  contacts. Pruned by age. (`internal/domain/merge_log.go`,
  `migrations/023_merge_log.up.sql`)
- **Detection settings** — `user_dedup_settings`. One row per user: cron schedule and an enabled
  flag. (`internal/domain/user_dedup_settings.go`,
  `migrations/016_user_dedup_settings.up.sql`)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A scan of **10 000 contacts completes in tens of milliseconds**, down from
  **38.9 seconds** and 13.6 GB — roughly a 1500× improvement. Measured 2026-08-06 on the
  maintainer's machine at 8 ms and 4.9 MB, before detection read the child tables; re-measured
  2026-08-07 on an Apple M3 at about 25 ms and 16.8 MB for a book where every contact holds two
  emails and two phone numbers, and about 10 ms and 7.3 MB for the same fixture as 2026-08-06.
  The two dates are different machines and only the same-day pairs are comparable.
- **SC-002**: Cost grows close to linearly with the address book: ten times the contacts costs
  about fourteen times the work (1.8 ms → 25 ms, measured 2026-08-07 with secondary values),
  where it previously cost about a hundred times (0.39 s → 38.9 s). Near-linearity is the
  property that matters; a super-linear figure would mean all-pairs comparison had come back.
- **SC-003**: Re-running detection over an unchanged address book records **zero** new pairs, and
  a dismissal survives every later scan.
- **SC-004**: At most one detection run per account is ever in flight; a second attempt is
  refused rather than executed.
- **SC-005**: 100% of pairs written by the current detector carry at least one reason naming the
  value that matched, so the list never has to say "83% similar".
- **SC-006**: A confirmed merge leaves exactly one contact where there were two, and a CardDAV
  client synchronising across the merge receives exactly two changes: one update and one removal.
- **SC-007**: After a merge, zero sync-state rows reference the removed contact — so no later
  export re-creates it or raises a conflict for it.
- **SC-008**: Every merge is reconstructible by hand for at least the retention window (30 days
  by default), including after both contacts involved have been deleted. Everything is
  recoverable except the discarded record's photo.
- **SC-009**: A user can produce a single merged record holding the work address from one source
  record and the home address from the other, in one operation.
- **SC-010**: Before confirming, a user can read every value the merge will discard, named
  individually rather than counted.
- **SC-011**: Changing the detection schedule takes effect **without a server restart**.
- **SC-012**: Every pair the detector found is reachable from the list — page size is clamped to
  100 rather than reset, and the status filter can be cleared.
- **SC-013**: The one-click merge is offered only where it is provably lossless; where it is not,
  the user is told so and routed to the screen that shows the differences.

## Assumptions

Conditions the implementation takes as given. Several are limitations rather than choices, and
are stated as such; where a limitation costs the user something, **Known Divergences** says what.

- **Identity is an email address or a phone number.** Names contribute explanation, never a
  match: the 0.8 threshold is unreachable by name alone, and that is pinned by a characterisation
  test so nobody changes it by accident
  (`internal/service/duplicate_detect_test.go:71-81`). Two records for one person with no shared
  email and no shared phone are, by design, not duplicates as far as this system is concerned.
- **Every email and phone a contact holds represents it for detection purposes, but nothing
  else does.** The flat columns and the two child tables are read; the other five child
  collections are not, because the scorer has no kind of match for them
  (`internal/repository/bun_contact.go:305-359`). Reading them still has to stay cheap: two
  two-column projections rather than whole contact rows, for the same reason the column-limited
  query exists at all. What that costs is recorded under Known Divergences.
- **One address book per user.** Detection resolves the address book through
  `GetOrCreateByUserID` (`internal/service/duplicate_detector.go:99-102`), and merge does the
  same. Cross-book duplicates are not a concept here.
- **Single instance.** The single-flight guard is in-process memory and the job queue is a
  channel in one process. Two servers on one database would each scan and each schedule. The
  unique index keeps the *data* correct under that; the *work* is not deduplicated. This matches
  the project's stated single-instance stance.
- **The job queue is in-memory and lossy.** A restart drops a queued detection run. Unlike
  backups, there is no boot-time catch-up: the consequence is a scan that happens later, which is
  acceptable, and that acceptance is why no catch-up was written.
- **A merge is irreversible in the data model, and the history is the mitigation.** The discarded
  contact row is deleted, not tombstoned as a contact, because the pair table cascades on contact
  deletion. The compensating design — a table with no foreign keys and its own snapshot — is what
  makes recovery possible at all, and it is recovery by hand, not a feature.
- **The vCard text is the source of truth for a merge.** Which values survive is decided over
  decoded cards and the result is re-parsed back into the relational fields
  (`internal/service/merge_service.go:122-146`). The serialisation contract, including the
  encoder's documented limitation where a `;` inside a single-valued TEXT property is not
  escaped, belongs to domain 003 and is inherited whole.
- **Migrations are forward-only** (constitution Principle I). `006`, `016`, `023` and `024`
  cannot be rolled back by the application; undoing any of them means restoring a database dump.

**Depends on, and inherits from, sibling domains:**

- **Contact records and their child tables (002)** — the merge writes through the same child-row
  replacement path as an ordinary update, and the detector reads the contact table's
  denormalised columns.
- **vCard encode/decode and the card-level merge helpers (003)** — `Candidates`, `MergeCards`,
  `ParseVCard`, `ApplyToContact`, `ChildRecordsFor`, `StripPhoto`. This spec decides which value
  wins; 003 owns how a card is read and written.
- **The change journal and CardDAV sync semantics (004)** — a merge is only correct because it
  journals the deletion at the winner's sequence number.
- **Sync state (006)** — the merge deletes the discarded contact's rows; the meaning of those
  rows belongs to the sync domain.
- **The scheduler and the in-process job queue (008)** — registration, re-registration and
  removal of a per-user cron job, and the queue the firing enqueues onto.
- **The shared expensive-operation rate limiter (008)** — gates
  `POST /contacts/duplicates/detect` alongside imports, backups and pipeline triggers
  (`internal/handler/handler.go:102-105,134`).
- **Configuration (008)** — `merge.log_retention_days` / `CHQ_MERGE_LOG_RETENTION_DAYS`, bound
  explicitly in `envBoundKeys` (`internal/config/config.go:54`).

**Deliberately outside this spec:**

- Ordinary contact create, read, update and delete, and the child-table write path a merge reuses
  (002).
- The vCard serialisation contract — decoding, encoding, escaping, value identity — which is 003.
  This spec states which value wins; it does not state how a value becomes text.
- Conflicts between a local and a remote version of the *same* contact. That is a different
  problem with a different table, a different resolution model and a different UI (006).
- The shared rate limiter on expensive operations (008).
- Deduplication across users or across address books. Neither exists.
- Any automatic merge. Every merge in this system is initiated by a person, and every one is
  confirmed first.

## Status

Shipped. Reconstructed from the tree at `23a167c`, which is tag `v0.4.0` plus three follow-up
commits (two documentation, one `config.example.yaml` fix — none of them touch this domain).
Every requirement above describes behaviour present in that tree, with the exceptions collected
under **Known Divergences**.

## Code Paths

Owned by this spec. Nothing here is claimed by another spec, and no bare directory is claimed in
the five dense trees.

- `internal/service/duplicate_detector.go`
- `internal/service/duplicate_detector_test.go`
- `internal/service/duplicate_detect_test.go`
- `internal/service/merge_service.go`
- `internal/service/merge_service_test.go`
- `internal/service/merge_log_service_test.go`
- `internal/handler/duplicate_handler.go`
- `internal/domain/potential_duplicate.go`
- `internal/domain/merge_log.go`
- `internal/domain/user_dedup_settings.go`
- `internal/repository/bun_potential_duplicate.go`
- `internal/repository/bun_merge_log.go`
- `internal/repository/bun_user_dedup_settings.go`
- `internal/repository/bun_user_dedup_settings_test.go`
- `internal/repository/duplicate_pair_test.go`
- `internal/repository/merge_log_test.go`
- `internal/repository/merge_into_test.go`
- `internal/worker/jobs/dedup_job.go`
- `internal/worker/jobs/dedup_job_test.go`
- `migrations/006_potential_duplicates.up.sql`
- `migrations/006_potential_duplicates.down.sql`
- `migrations/016_user_dedup_settings.up.sql`
- `migrations/016_user_dedup_settings.down.sql`
- `migrations/023_merge_log.up.sql`
- `migrations/023_merge_log.down.sql`
- `migrations/024_potential_duplicates_unique.up.sql`
- `migrations/024_potential_duplicates_unique.down.sql`
- `web/src/views/contacts/DuplicatesView.vue`
- `web/src/views/contacts/ContactMergeView.vue`
- `web/src/views/contacts/ContactMergeView.spec.ts`
- `web/src/components/contacts/MergeContactColumn.vue`
- `web/src/components/contacts/MergeFieldGroup.vue`
- `web/src/components/contacts/MergeFieldGroup.spec.ts`
- `web/src/components/contacts/MergePreviewCard.vue`
- `web/src/components/contacts/MergePreviewCard.spec.ts`
- `web/src/components/contacts/DuplicateSummary.vue`
- `web/src/components/contacts/DuplicateSummary.spec.ts`
- `web/src/utils/duplicates.ts`
- `web/src/utils/duplicates.spec.ts`
- `web/src/utils/merge.ts`
- `web/src/utils/merge.spec.ts`

## References

Paths this spec touches but does **not** own.

- `internal/vcard/merge_cards.go` — owned by 003. `Candidates`, `MergeCards` and `valueID` are
  what make FR-021, FR-032 and FR-033 true; the encode/decode contract underneath them is 003's.
- `internal/vcard/encoder.go` — owned by 003. `StripPhoto` is what FR-044 calls.
- `internal/repository/bun_contact_relations.go` — owned by 002. `MergeInto` is the transaction
  FR-035 to FR-037 depend on, but the file is the contact write path.
- `internal/repository/change_journal.go` — owned by 002. The journal `MergeInto` writes into
  (FR-036).
- `internal/repository/interfaces.go` — owned by 008. Declares `MergeLogRepository`,
  `PotentialDuplicateRepository` and `UserDedupSettingsRepository` (`:138-160`).
- `cmd/server/main.go` — owned by 008. The startup `jobs.PruneMergeLog` call at `:142-144`
  (FR-049) and the schedule loading at `:177-186` (FR-028).
- `internal/worker/scheduler.go` — owned by 008. The dedup registration helpers at `:161-202`
  (FR-027, FR-029).
- `internal/handler/handler.go` — owned by 008. Registers this domain's routes at `:122-137`,
  including the ordering that keeps `/duplicates/count` from being swallowed by
  `/duplicates/:id`, and the `expensive` rate limiter on `/duplicates/detect`.

Boundaries with sibling specs, stated so ownership is unambiguous: `ListForDedup` and
`ListDedupValues` (`internal/repository/bun_contact.go:305-359`), which FR-013 and FR-001a cite,
live in the contact repository and belong to 002, as does `domain.ContactValueRef`
(`internal/domain/contact.go:61-70`); `internal/repository/migrate_postgres_test.go`, which
holds `TestPostgresListDedupValues`, belongs to 008 — this domain's PostgreSQL enforcers have
always lived there; `web/src/components/layout/Sidebar.vue`, cited by User Story 6,
belongs to the layout owner; `CHANGELOG.md`, cited for the pre-v0.4.0 history, is release
documentation and belongs to nobody's `## Code Paths`.

## Enforced By

**Go tests** — package `service_test`, `repository_test`, `worker_test` and `jobs_test`, all run
by `go test ./... -count=1 -race` in the `test` job of `.github/workflows/ci.yml`.

Detection:

- `TestDetect_CharacterisesCurrentBehaviour` (`internal/service/duplicate_detect_test.go`) —
  FR-003, FR-004, and the "a name alone never matches" rule behind Story 1 scenario 3.
- `TestDetect_BucketsByKey` (`internal/service/duplicate_detect_test.go`) — FR-001, FR-002.
- `TestDetect_PairsOnASecondaryEmail`, `TestDetect_PairsOnASecondPhone`
  (`internal/service/duplicate_detect_test.go`) — FR-001a. Both reported zero pairs before the
  child tables were read.
- `TestDetect_StillPairsAContactWithNoChildRows`
  (`internal/service/duplicate_detect_test.go`) — FR-001a's union half: the pre-014 contact that
  has flat columns and no child rows is still paired.
- `TestDetect_DoesNotInflateABucketWhenThePrimaryAlsoAppearsAsAChildRow`
  (`internal/service/duplicate_detect_test.go`) — FR-001b. It asserts the bucket is still
  *scanned* rather than the pair count, because a repeated entry costs a bucket its scan, not a
  duplicate row: 300 contacts whose primary phone is also a child row make one bucket of 300,
  under the cap; without the guard the bucket is 600 and skipped.
- `TestDetect_NormalisesPairOrder` (`internal/service/duplicate_detect_test.go`) — FR-007.
- `TestDetect_PairFoundByTwoKeysIsRecordedOnce` (`internal/service/duplicate_detect_test.go`) —
  FR-009.
- `TestDetect_DoesNotDuplicateAnExistingPair` (`internal/service/duplicate_detect_test.go`) —
  FR-008, SC-003.
- `TestDetect_SkipsImplausiblyLargeBuckets`,
  `TestDetect_SkipsImplausiblyLargeBucketsBuiltFromChildValues`
  (`internal/service/duplicate_detect_test.go`) — FR-010, including that the cap counts a
  switchboard number held in `contact_phones` exactly as it counts one held on the row.
- `TestDetect_RefusesAConcurrentRunForTheSameUser` (`internal/service/duplicate_detect_test.go`)
  — FR-011 (the service half), SC-004.
- `TestDetect_HonoursContextCancellation` (`internal/service/duplicate_detect_test.go`) — FR-012.
- `TestDetect_EmptyAddressBook` (`internal/service/duplicate_detect_test.go`) — FR-014.
- `BenchmarkDetect`, `BenchmarkDetectWithSecondaryValues`
  (`internal/service/duplicate_detect_test.go`) — SC-001, SC-002. Neither is run by CI; see
  Known Divergences. `BenchmarkDetectWithSecondaryValues` bounds the Go-side bucketing cost of
  FR-001a only — it runs against an in-memory repository and issues no query, so it says nothing
  about the cost of reading `contact_emails` and `contact_phones`, which is argued on
  `ListDedupValues` and pinned by `TestPostgresListDedupValues` instead.
- `TestScoreContacts_ExactEmailMatch`, `TestScoreContacts_DifferentEmail_PhoneMatch`,
  `TestScoreContacts_NameExactMatch`, `TestScoreContacts_NameSimilar`, `TestScoreContacts_NoMatch`
  (`internal/service/duplicate_detector_test.go`) — characterisation of the pre-bucket scorer that
  the current one must reproduce; FR-003, FR-004.
- `TestLevenshtein`, `TestNormalizePhone` (`internal/service/duplicate_detector_test.go`) —
  FR-001 (phone normalisation), FR-004 (the distance-2 rule).

Merging:

- `TestMerge_DeletesLoserAndKeepsWinner` (`internal/service/merge_service_test.go`) — SC-006.
- `TestMerge_ResolutionSelectsLoserField` (`internal/service/merge_service_test.go`) — FR-030,
  FR-031.
- `TestMerge_RewritesWinnerChildRows`,
  `TestMerge_WritesThroughSaveSoChildRowsAndChangeSeqFollow`
  (`internal/service/merge_service_test.go`) — FR-034, FR-035, FR-036.
- `TestMerge_UnparseableLoserCardFailsWithoutTouchingTheWinner`
  (`internal/service/merge_service_test.go`) — FR-034.
- `TestMerge_RejectsSameContact`, `TestMerge_RejectsContactFromAnotherAddressBook`
  (`internal/service/merge_service_test.go`) — FR-040.
- `TestMerge_RemovesDuplicateRecordsForBothSides` (`internal/service/merge_service_test.go`) —
  FR-039.
- `TestMergeInto_JournalsBothTheUpdateAndTheDeletion`,
  `TestMergeInto_UsesOneSequenceForBothChanges` (`internal/repository/merge_into_test.go`) —
  FR-036, SC-006.
- `TestMergeInto_UnknownLoserStillSavesTheWinner` (`internal/repository/merge_into_test.go`) —
  FR-037.

Merge history:

- `TestMerge_RecordsTheMergeWithASnapshotOfTheLoser` (`internal/service/merge_log_service_test.go`)
  — FR-042.
- `TestMerge_SnapshotStripsThePhoto` (`internal/service/merge_log_service_test.go`) — FR-044.
- `TestMerge_ClearsTheLosersSyncState` (`internal/service/merge_log_service_test.go`) — FR-038,
  SC-007.
- `TestMerge_LogFailureIsReportedAndDoesNotAbortTheMerge`
  (`internal/service/merge_log_service_test.go`) — FR-045.
- `TestMerge_WorksWithoutAMergeLog` (`internal/service/merge_log_service_test.go`) — FR-046.
- `TestMergeLog_SurvivesDeletionOfBothContacts` (`internal/repository/merge_log_test.go`) —
  FR-043, SC-008.
- `TestMergeLog_ColumnNamesRoundTrip` (`internal/repository/merge_log_test.go`) — FR-042.
- `TestMergeLog_ListIsScopedToTheUserAndNewestFirst` (`internal/repository/merge_log_test.go`) —
  FR-047.
- `TestMergeLog_DeleteOlderThanPrunesOnlyPastTheCutoff` (`internal/repository/merge_log_test.go`)
  — FR-048 (the repository half only; see Known Divergences).

Reading pairs and settings:

- `TestPotentialDuplicate_GetByIDWithContactsLoadsChildCollections`
  (`internal/repository/duplicate_pair_test.go`) — FR-020.
- `TestPotentialDuplicate_GetByIDWithContactsRejectsAnotherUser`,
  `TestPotentialDuplicate_GetByIDWithContactsUnknownID`,
  `TestPotentialDuplicate_GetByIDReturnsNilForUnknownID`
  (`internal/repository/duplicate_pair_test.go`) — FR-022.
- `TestPotentialDuplicate_ListByUserComputesSubsetFlags`
  (`internal/repository/duplicate_pair_test.go`) — FR-018, FR-019, SC-013.
- `TestPotentialDuplicate_ListByUserStatusAll` (`internal/repository/duplicate_pair_test.go`) —
  FR-015.
- `TestDedupSettings_GetReturnsNilWhenNotSet`, `TestDedupSettings_UpsertInsertAndGet`,
  `TestDedupSettings_UpsertUpdatesExisting`, `TestDedupSettings_ListAllEmpty`,
  `TestDedupSettings_ListAllReturnsAll` (`internal/repository/bun_user_dedup_settings_test.go`) —
  FR-025, and the persistence half of FR-028.

Scheduling and the job:

- `TestRegisterDedupForUser`, `TestRemoveDedupForUser`, `TestReregisterDedupForUser`
  (`internal/worker/scheduler_test.go`) — FR-027, SC-011.
- `TestDedupPayload_Serializable` (`internal/worker/scheduler_test.go`),
  `TestDedupJobPayload_Roundtrip` (`internal/worker/jobs/dedup_job_test.go`) — FR-029.
- `TestDedupJob_RunsTheScan` (`internal/worker/jobs/dedup_job_test.go`) — FR-029.
- `TestDedupJob_WrapsAFailureWithTheUserID`, `TestDedupJob_RejectsAnUnreadablePayload`,
  `TestDedupJobHandler_InvalidPayload` (`internal/worker/jobs/dedup_job_test.go`) — a bad payload
  never silently scans nobody.

**PostgreSQL** — package `repository`, run by `go test ./internal/repository/ -run TestPostgres`
in the `postgres` job of `.github/workflows/ci.yml`:

- `TestPostgresListDedupValues` (`internal/repository/migrate_postgres_test.go`) — FR-013's
  narrow-projection half and FR-001a's scoping, on the other driver. It is the only PostgreSQL
  coverage `ListDedupValues` gets: the `postgres` job runs `-run TestPostgres` against package
  `repository` alone, so a test elsewhere, or one without that name prefix, never executes
  against PostgreSQL at all.
- `TestPostgresPotentialDuplicateUniqueIndex` (`internal/repository/migrate_postgres_test.go`) —
  FR-008 on the other driver.
- `TestPostgres_MergeLogRoundTrip` (`internal/repository/migrate_postgres_test.go`) — FR-042 on
  the other driver.
- `TestPostgres_MigrateAppliesEverySchemaObject` (`internal/repository/migrate_postgres_test.go`)
  — `merge_log`, `potential_duplicates` and `user_dedup_settings` are in `expectedTables`
  (`:70,73,81`), so a migration this domain adds cannot go unlisted.

**Frontend** — Vitest, run by `npm run test` in the `frontend` job of
`.github/workflows/ci.yml`:

- `web/src/utils/duplicates.spec.ts` — `describe('parseMatchReasons')` covers FR-006 (legacy
  `string[]`, current object form, a mixture, entries with no code, an empty value);
  `describe('reasonLabel')` covers FR-005 and FR-061; `describe('confidenceLabel')` covers
  FR-061; `describe('canKeepA / canKeepB')` covers FR-059 and SC-013, including "refuses when the
  server said nothing".
- `web/src/utils/merge.spec.ts` — `describe('buildMergeModel')` covers FR-051 and FR-052;
  `describe('defaultSelection')` covers Story 2 scenarios 2 and 3;
  `describe('previewFromSelection')` and `describe('discardedBySelection')` cover FR-053 and
  SC-010; `describe('buildMergePayload')` covers FR-030 ("keys the selection by vCard property
  names, not contact fields") and FR-055 ("takes the winner from the explicit choice");
  `describe('mergeDiffers')` guards the untouched default.
- `web/src/views/contacts/ContactMergeView.spec.ts` — FR-056 ("loads the pair by id and never
  lists"), FR-057 (not-found and forbidden versus a retryable transport failure), FR-054
  ("rebuilds the defaults when the surviving record changes"), FR-058 ("stays mounted and shows
  the error when the merge fails"), and SC-009/SC-010 end to end.
- `web/src/components/contacts/MergeFieldGroup.spec.ts` — FR-051 (checkboxes for multi-valued,
  radios for single-valued) and FR-052 ("marks a differing group in words").
- `web/src/components/contacts/MergePreviewCard.spec.ts` — FR-053, SC-010 ("names each discarded
  value rather than only counting them").
- `web/src/components/contacts/DuplicateSummary.spec.ts` — the list summary rendering behind
  FR-059 and FR-061.

**Boot-time and review-only:**

- FR-011's 409 status code, FR-015's status defaulting, FR-016's clamp, FR-023's 403, FR-024,
  FR-026, FR-047's page bound and FR-050's copy are **review-only**: there is no handler test for
  this domain. See Known Divergences.
- FR-021 is review-only. `Candidates` is exercised through the merge screen's fixtures, but no
  test asserts that a client never mints an id — it is enforced by there being no hashing code
  under `web/src`.

## Known Divergences

Where shipped behaviour is narrower, rougher or more surprising than the requirements above
suggest. None of these is presented as a solved requirement.

**Reading the child tables costs half again as much memory, on every scan.** Detection no
longer ignores secondary emails and phone numbers (FR-001a), but the `seen` set that keeps one
contact out of one bucket twice (`internal/service/duplicate_detector.go:126-135`) is allocated
whether or not there is anything to de-duplicate. Measured on one machine in one session, the
child-row-free fixture went from 4.9 MB to 7.3 MB per scan at ten thousand contacts with the
time unchanged inside the noise (`internal/service/duplicate_detect_test.go:181-192`). SC-001's
4.9 MB was measured before this change and is no longer what the code allocates.

**The bucket cap now counts secondary values, so a shared address can hide a pair that used to
be reported.** `maxBucketSize = 500` is applied to the union of flat and child values
(`internal/service/duplicate_detector.go:26,189-195`). A key that stayed under the cap while only
the `phone` column fed it can now cross it — an office number held by 300 people on their contact
row and by another 250 as a second number is a single bucket of 550 and is dropped. The pairs
inside it were reported before this change and are not reported after it. The drop is a log
warning; nothing in the API or the UI says so, which is the divergence recorded below about
oversized buckets, now with a wider reach.

**Detection reads only the emails and phones from the child tables.** The other five child
collections — addresses, URLs, IMs, categories, dates — are not read, because none of them is a
key the scorer can match on (`internal/service/duplicate_detector.go:290-318`: the only kinds are
`email` and `phone`). This is narrower than "detection sees the child tables" and is deliberate:
reading a table nobody buckets on would be cost with no pairs behind it.

**The subset check and the detector agree on which values matter, but not on how they compare
them.** Both now read `contact_emails` and `contact_phones`, which is what closed the old
asymmetry. They still normalise differently: the detector lower-cases and trims an email and
keeps digits only from a phone in Go (`internal/service/duplicate_detector.go:143-171,360-371`),
while the subset flags do it in SQL with `lower()` and nested `REPLACE`
(`internal/repository/bun_potential_duplicate.go:153-181`), whose replacement list is
` `, `-`, `(`, `)`, `+`, `.` — so a phone written with any other punctuation normalises one way
for the detector and another for the subset check. Two implementations of one rule.

**The subset check compares emails and phones and nothing else.** A record whose only unique
contribution is a note, a birthday or a URL is still reported as a subset, and the one-click
merge is offered for it (`internal/repository/bun_potential_duplicate.go:156-170`). SC-013's
"provably lossless" is provable over emails and phones only.

**An oversized bucket is skipped silently from the user's point of view.** More than 500 contacts
sharing one key and the whole bucket is dropped with a log warning
(`internal/service/duplicate_detector.go:26,136-142`). Two genuine duplicates that happen to
share the office switchboard number with 600 colleagues will never be reported, and nothing in
the API or the UI says so.

**The threshold and the bucket cap are compile-time constants.** `duplicateScoreThreshold = 0.8`
and `maxBucketSize = 500` (`internal/service/duplicate_detector.go:19,26`). The only user-facing
settings are the schedule and the on/off switch (`migrations/016_user_dedup_settings.up.sql`).
There is no "match on name too" option, and `user_dedup_settings` has no column for one.

**The score has only ever taken two values.** 1.0 for an exact email, 0.8 for a phone
(`internal/service/duplicate_detector.go:242-250`). The UI stopped rendering it as a percentage
for exactly that reason and shows "Certain match" / "Likely match"
(`web/src/utils/duplicates.ts:63-69`). The `score REAL` column is therefore far more precise than
the thing it stores.

**The single-flight guard is per process.** It is a `sync.Mutex` over an in-memory map
(`internal/service/duplicate_detector.go:38-41`). Two server processes sharing one database would
each admit a scan, so SC-004 holds per process, not per database. The unique index makes the
outcome correct rather than duplicated, but the work is done twice. Multi-instance is not a
supported configuration for this project.

**A refused concurrent scan is invisible in the web UI.** `Detect` answers 409 with a clear
message (`internal/handler/duplicate_handler.go:159-163`), but `runDetect` in
`web/src/views/contacts/DuplicatesView.vue:257-267` has a `finally` and no `catch`: the rejection
escapes, the success message stays empty, and the user sees the button stop spinning with no
explanation. `dismiss` (`:252-255`) and `fetchDuplicates` (`:218-233`) are the same shape.

**A merged pair leaves no `merged` status behind.** The only two statuses ever written are
`pending` (`internal/service/duplicate_detector.go:177`) and `dismissed`
(`internal/handler/duplicate_handler.go:185`). A merge deletes the pair row outright
(`internal/service/merge_service.go:154,172`) and the database would cascade it anyway when the
loser goes (`migrations/006_potential_duplicates.up.sql:4-5`). That is precisely why `merge_log`
exists as a separate table with no foreign key (`migrations/023_merge_log.up.sql:1-10`) — but it
means the pair list can never show "these two were merged". Open: should it be able to? The pair
id *is* stored inside the resolution JSON (`internal/service/merge_service.go:226`), so the link
exists and nothing reads it.

**The merge record is written *before* the merge is applied.** `recordMerge` runs at
`internal/service/merge_service.go:150`; `MergeInto` at `:162`. The ordering is deliberate — the
loser is gone afterwards and there would be nothing left to describe — but a database failure
inside `MergeInto` leaves a history row for a merge that did not happen. Nothing detects or
removes it.

**There is no undo, only the material to do one by hand.** No endpoint restores from the log, and
the snapshot has PHOTO, LOGO and SOUND stripped (`internal/service/merge_service.go:239`), so a
photo that existed only on the discarded record is unrecoverable. SC-008's "reconstructible by
hand" means exactly: read the JSON, re-create the contact from `loser_vcard`, accept the lost
photo.

**The merge history has no user interface at all.** `GET /contacts/merge-log` is registered
(`internal/handler/handler.go:137`) and implemented
(`internal/handler/duplicate_handler.go:48-65`), but nothing under `web/src` calls it — there is
no `mergeLog` function in `web/src/api/contacts.ts` and no view. The recovery path in Story 4
requires an HTTP client. Open: deferred, or intended for operators only? The endpoint, the
repository, the retention setting and the pruning job are all shipped and tested; the code does
not say which.

**Retention pruning is global, not per user.** `DeleteOlderThan` filters on `merged_at` only
(`internal/repository/bun_merge_log.go:47-57`), and the single knob `merge.log_retention_days`
(default 30, `internal/config/config.go:193`) applies to the whole instance. A user cannot keep
their own history longer.

**FR-049 has no enforcer.** `TestMergeLog_DeleteOlderThanPrunesOnlyPastTheCutoff` proves the
repository query; nothing asserts that `PruneMergeLog` is actually invoked from the dedup job
handler (`internal/worker/jobs/dedup_job.go:80`) or once at startup
(`cmd/server/main.go:142-144`). `internal/worker/jobs/dedup_job_test.go` stubs the scanner only
and never constructs the handler with a merge-log repository. Deleting either call site would
turn no test red.

**This domain has no handler tests.** `internal/handler/` holds only `health_test.go`,
`registration_policy_test.go` and `backup_download_test.go`; there is no
`duplicate_handler_test.go`. Everything the handler alone decides is unenforced: the 409 for a refused scan (FR-011), the status defaulting and the
`all` keyword (FR-015), the page-size clamp that SC-012 rests on (FR-016), the 403 on another
user's pair (FR-023), the count endpoint (FR-024), cron validation (FR-026), and the immediate
re-registration on save (FR-027 — the scheduler helpers are tested, the handler's call into them
is not). FR-016 in particular is a regression fix with no regression test.

**FR-028 is unenforced.** The startup loop in `cmd/server/main.go:177-186` has no test; only the
repository read it depends on (`TestDedupSettings_ListAllReturnsAll`) does.

**SC-001 and SC-002 are a benchmark, not a gate.** Neither `BenchmarkDetect` nor
`BenchmarkDetectWithSecondaryValues` is run by CI (`.github/workflows/ci.yml` runs
`go test ./... -count=1 -race`, which does not execute benchmarks). The figures are hand
measurements from two machines on two days, recorded in the comments above each benchmark and
restated in `CHANGELOG.md`. A future change that reintroduced the quadratic scan would not redden
the build.

**No benchmark measures the query.** Both benchmarks run against `mockContactRepo`, an in-memory
map, so the I/O cost of `ListDedupValues` — the only genuinely new cost in FR-001a — is argued
from the query shape and the indexes it uses (`internal/repository/bun_contact.go:320-359`) and
pinned only by `TestPostgresListDedupValues` asserting the projection is two columns wide. An
address book whose child tables are far larger than its contact table would cost more than the Go
figures suggest, and nothing here would show it.

**Ownership is enforced two different ways.** `GetByIDWithContacts` filters on `user_id` inside
the query (`internal/repository/bun_potential_duplicate.go:67-74`), which is what `Get` and the
merge path use. `Dismiss` uses `GetByID`, which does not, and compares the ids in Go afterwards
(`internal/handler/duplicate_handler.go:174-183`). Both are correct today; only one is correct by
construction, and FR-022 only describes the first.

**Value identifiers go stale without saying so.** They are content hashes minted per request
(`internal/handler/duplicate_handler.go:125-134`, `internal/vcard/merge_cards.go:56-81`). If
either card is edited between opening the merge screen and confirming, a selected id no longer
matches any field and that value is simply omitted from the merged card
(`internal/vcard/merge_cards.go:195-204`) — no error, no warning, and the user is told nothing.

**The merge screen offers 14 properties; everything else keeps the winner's values silently.**
`MERGE_FIELD_SPECS` lists FN, N, NICKNAME, ORG, TITLE, ROLE, BDAY, NOTE, EMAIL, TEL, ADR, URL,
IMPP, CATEGORIES (`web/src/utils/merge.ts:27-42`). Candidates for anything else are collected
into `model.unlisted` (`web/src/utils/merge.ts:85-88`) — and then never rendered and never sent.
Combined with FR-033, the loser's value for an unlisted property is discarded without ever being
shown. The field list is a curation, not a contract: adding a property to it is a UI change with
no server counterpart.

**Migration 024 will fail an upgrade rather than discard rows.** If an installation somehow holds
both `(a,b)` and `(b,a)`, the unique index cannot be created and the transaction aborts. This is
stated as intended in the migration itself
(`migrations/024_potential_duplicates_unique.up.sql:6-9`), and it means the server will not start
until the operator resolves it by hand.

**The four migrations ship `.down.sql` files that nothing applies.** `006`, `016`, `023` and `024`
each have one, and `MigrateFS` globs `*.up.sql` only. Their presence suggests a rollback that does
not exist.

**The scheduled scan lives in an in-memory queue.** The cron job enqueues onto a buffered channel
in one process (`internal/worker/scheduler.go:169-176`); a restart drops whatever was queued.
Unlike backups, there is no boot-time catch-up for a missed detection run — the pairs are simply
found on the next firing.

**`scoreContacts` is dead in production.** It is the previous all-pairs scorer, kept only so a
characterisation test can pin what the bucketed scan must reproduce
(`internal/service/duplicate_detector.go:267-303`), and it keeps `fmt` alive through a
`var _ = fmt.Sprintf` at `:305`.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-07 | — | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | — | Rewritten to the house template: header replaced (Kind/Status/Constitution; `Feature Branch`, `Created`, `Status: Implemented` and the scope-note comment removed), `Dependencies` and `Out of Scope` folded into Assumptions, `Open Questions` folded into Known Divergences, and `Status`/`Code Paths`/`References`/`Enforced By`/`Known Divergences`/`Amendments` added in template order. Ownership narrowed to an explicit per-file list; `References` restated as an eight-entry list with owning spec named. Every admission moved out of Edge Cases and Assumptions into Known Divergences, leaving Edge Cases as answered boundary conditions. `Enforced By` written from grep-verified test names, and five unenforced areas (handler layer, FR-028, FR-049, benchmark-only SC-001/SC-002, FR-021) recorded as gaps. FR-021, FR-032 and FR-033 restated so each is stated once and unambiguously, since sibling specs defer here. SC-001/SC-002 stripped of their test-file citations, which the template's success-criteria lint rejects. The open question about missing Spec Kit templates is closed: `.specify/templates/overrides/spec-template.md` and `.specify/memory/constitution.md` now exist and are what this rewrite followed. | — |
| 2026-08-07 | unreleased | **D5** — detection now buckets on secondary emails and second phone numbers, not only on `contacts.email` / `contacts.phone`. Added FR-001a (union of flat and child values, never substitution — migration 014 backfilled nothing) and FR-001b (one contact per bucket at most once, because a repeated entry inflates the bucket against the cap of FR-010 rather than creating a false pair); widened FR-013 to require two narrow `(contact_id, value)` projections rather than a relation load or a join; corrected the Assumptions bullet that said the primary columns represent the contact. Removed the "Detection is blind to the child tables" divergence, which is no longer true, and replaced it with four narrower admissions: the `seen` set costs about 50% more memory on every scan (4.9 MB → 7.3 MB at 10 000 contacts, same machine and session), the bucket cap now counts secondary values and can therefore *stop* reporting a pair it used to report, only the two child tables the scorer can match on are read, and the detector and the subset check still normalise phone numbers two different ways. Restated SC-001/SC-002 with the new measurements and named the machines, since the 2026-08-06 and 2026-08-07 figures are not comparable. Added a divergence recording that no benchmark measures the new query. | — |
| 2026-08-07 | unreleased | D4: corrected the statement that `internal/handler/` holds exactly two test files (it holds three; `backup_download_test.go` is owned by spec 005) and rewrapped the paragraph. Wording only; no requirement, enforcer or code path changed here. | D4 |
