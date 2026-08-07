# Feature Specification: [FEATURE NAME]

Kind: component
Status: draft
Constitution: v1.0.0

<!--
  This is the ContactsHQ override of the upstream spec template. `resolve_template()`
  in .specify/scripts/bash/common.sh checks templates/overrides/ FIRST, so this file
  is what /speckit-specify and create-new-feature.sh copy.

  Differences from upstream, and why:
  - The `**Feature Branch**` and `**Input**: "$ARGUMENTS"` fields are gone. This repo
    creates no branch per spec (state lives in .specify/feature.json), and both fields
    are exactly the placeholder shapes the structure lint bans.
  - `**Status**: Draft` is gone; the `Status:` header line above and the `## Status`
    section below are the two places status lives, and two more would drift.
  - `Kind:` selects which assertions apply. `journey` MUST carry at least one
    `(Priority: P1)` user story. `component` is exempt from that assertion — nobody
    ships internal/vcard alone — but still owes FR-NNN, SC-NNN and consumer scenarios.
    `meta` is exempt and carries a forward-looking tasks.md.
  - Six repo sections are appended after Assumptions: Status, Code Paths, References,
    Enforced By, Known Divergences, Amendments.

  While `Status: draft`, the content assertions are waived — a half-written spec never
  reddens the build. Promote the status when the spec is finished.

  Read .specify/memory/constitution.md and CLAUDE.md before filling this in.
-->

## User Scenarios & Testing *(mandatory)*

<!--
  journey specs: PRIORITIZED user journeys (P1 first). Each must be INDEPENDENTLY
  TESTABLE — implementing just one still delivers something usable.

  component specs: write these as CONSUMER scenarios instead — who calls this, with
  what, and what they are entitled to assume. Priorities are optional here.
-->

### User Story 1 - [Brief Title] (Priority: P1)

[The journey in plain language.]

**Why this priority**: [What value it delivers and why it ranks here.]

**Independent Test**: [How this is verified on its own.]

**Acceptance Scenarios**:

1. **Given** [initial state], **When** [action], **Then** [expected outcome]

---

### Edge Cases

- What happens when [boundary condition]?
- How does the system handle [error scenario]?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST [specific, observable capability]
- **FR-002**: System MUST NOT [specific prohibition]

### Key Entities *(include if the feature involves persisted data)*

- **[Entity]**: [What it represents, key attributes, relationships — no implementation]

## Success Criteria *(mandatory)*

<!--
  Observable and technology-agnostic. A success criterion names BEHAVIOUR, not the
  test that checks it — enforcers belong in `## Enforced By`. The lint rejects a
  success criterion containing a `TestXxx` name or a `.go` path.
-->

### Measurable Outcomes

- **SC-001**: [Measurable outcome]
- **SC-002**: [Measurable outcome]

## Assumptions

- [Assumption about scope, environment or an existing system this relies on]

## Status

[The tag this shipped at, and — if `partial` — exactly which part is not yet true.]

## Code Paths

<!--
  The AUTHORITATIVE OWNERSHIP list. Exactly one spec may OWN a path, and
  the coverage gate reads this section only.

  For the five dense trees — internal/handler, internal/service, internal/repository,
  internal/domain and web/src — a bare directory path is FORBIDDEN. Claim files or
  subpackages: a blanket claim auto-adopts everything added inside it later and
  permanently disarms the coverage gate.

  migrations/ are claimed individually by the spec that owns the table.
-->

- `path/to/file.go`

## References

<!-- Paths this spec touches but does NOT own; provenance docs (PLAN/PREREG/MEASURE). -->

- `docs/PLAN-example.md`

## Enforced By

<!--
  The named tests, CI steps and boot-time validators that make the claims above true.
  Every TestXxx token here must resolve to a real test.
  A requirement with no enforcer is either review-only — say so — or a gap; say that louder.
-->

- `TestExample` (`path/to/file_test.go`)

## Known Divergences

<!--
  Where shipped behaviour differs from stated intent, ON PURPOSE. This is the most
  important section in the template: a retroactive spec's one serious hazard is
  laundering a shipped compromise into a stated requirement. An old capability whose
  Known Divergences section is empty should be read with suspicion.
-->

- None known.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| YYYY-MM-DD | vX.Y.Z | Initial spec. | — |
