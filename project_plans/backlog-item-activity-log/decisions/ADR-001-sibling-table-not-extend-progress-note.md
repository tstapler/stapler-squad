# ADR-001: New Sibling `BacklogActivityNote` Table, Not an Extension of `BacklogProgressNote`

**Status**: Accepted (Auto Mode — no open questions)
**Date**: 2026-08-18

## Context

The backlog-item-activity-log feature needs an append-only, per-item, timestamped log of
free-form notes that ANY session can post — with or without `STAPLER_SESSION_UUID`, and
regardless of whether that session is linked to the item. Research
(`project_plans/backlog-item-activity-log/research/stack.md`,
`research/features.md`) found an almost-exact structural precedent already in the
codebase: `BacklogProgressNote` (`session/ent/schema/backlogprogressnote.go`), an
append-only ent table with fields `id`, `item_id`, `criterion_index` (`Min(-1)`, where
`-1` is an established "this note isn't about one AC criterion" sentinel used today by
`SetBacklogItemPRAndTransition`'s status-transition note and an observed-drift summary
note), `note`, `status` (`pending`/`in_progress`/`done`/`fail`), and `created_at`.

Two designs were on the table:

1. **Extend `BacklogProgressNote`** with two new optional fields
   (`author_session_uuid`, `author_session_title`) and write ad hoc notes into it with
   `criterion_index = -1`.
2. **New sibling ent type** (`BacklogActivityNote`), modeled field-for-field on
   `BacklogProgressNote`'s shape, swapping `criterion_index` + `status` for
   `author_session_uuid` + `author_session_title`.

## Decision

Build a **new sibling ent type**, `BacklogActivityNote`
(`session/ent/schema/backlog_activity_note.go`), rather than extending
`BacklogProgressNote`.

Fields: `id` (UUID, default), `item_id` (UUID FK field), `message` (string, required —
this tool's only content, unlike `BacklogProgressNote.note` which is secondary to the
status change), `author_session_uuid` (string, optional — empty means "manual"/no
session), `author_session_title` (string, optional — best-effort, may be empty),
`created_at` (time, `Default(time.Now)`, `Immutable()`). Edge:
`edge.From("item", BacklogItem.Type).Ref("activity_notes").Field("item_id").Unique().Required()`.
Index: `index.Fields("item_id", "created_at")`. On `BacklogItem`:
`edge.To("activity_notes", BacklogActivityNote.Type).Annotations(entsql.OnDelete(entsql.Cascade))`.

## Consequences

**Positive**:

- Keeps `report_progress`'s official AC-status audit trail semantically and
  structurally separate from an anyone-can-post note. `report_progress`,
  `AppendProgressNote`, `ProgressNoteData`, the `progress_notes` proto field, and
  `ProgressHistorySection.tsx` are all untouched — zero risk of regressing the six
  role-gated tools' behavior, which requirements.md explicitly forbids ("Must not
  weaken or change the behavior of ... report_progress ... submit_review_verdict").
- Avoids overloading `BacklogProgressNote.status`, which has no natural value for a
  pure comment (`pending`/`in_progress`/`done`/`fail` all describe AC-criterion
  progress, not "someone left a note").
- Matches this repo's own precedent for the opposite fork of this exact question:
  `project_plans/session-notes/decisions/ADR-001-note-field-on-session-not-sibling-table.md`
  drew the line as "sibling table when multi-writer + append-only + structured; scalar
  field when single-owner + last-write-wins." This feature is unambiguously
  multi-writer (any session) and append-only — squarely on the sibling-table side.
- Directly addresses pitfalls.md's confusability risk: a spoofed-looking free-text
  entry sharing a table/column with real AC-status history would make "is this an
  official progress mark or an informal note" a data-model question instead of a
  clearly-separated one. A dedicated table with its own proto message, event kind, and
  UI section keeps the trust boundary structural, not just presentational.
- The `message` field can be `required` (not `Optional()` like
  `BacklogProgressNote.note`) since it's this tool's only payload — there's no "status
  change with an optional note" shape to accommodate.

**Negative**:

- A second near-identical table/proto message/repository method pair/UI section exists
  side by side with `BacklogProgressNote`'s. This is deliberate duplication, not an
  oversight — see Alternatives Considered.
- Two tables now answer "what happened on this item, in order" for different audiences
  (official AC-progress vs. free-form notes), so a reader wanting a fully unified
  timeline must consult two sources. Per `research/features.md` Finding 1d/4a, this
  matches the UI's existing convention of separate per-kind sections
  (`ProgressHistorySection`, `WorkflowHistorySection`) rather than a unified feed, so it
  is consistent with, not a departure from, today's UX.
- **[Named explicitly post-review, 2026-08-18 — Concern 4 in
  `implementation/adversarial-review.md`]** ADR-002's whole rationale is that
  `BacklogItemData`'s child-edge eager-loads are "correctness-by-convention... no
  compiler enforcement," and that this already regressed once for `progressNotes`
  (`AppendProgressNote` never calling `publishItemChanged`) and, per Blocker 1's fix
  above, regressed a second time for `activityNotes` across every *other*
  `BacklogChangeKind`. This ADR's decision to add a second, independently-maintained
  append-only edge (`activity_notes`) is an accepted tradeoff against that same risk
  class, not a reduction of it: every future `publishItemChanged` call site, and every
  future edge added to `BacklogItem`, now has two child collections
  (`progress_notes`, `activity_notes`) it must remember to eager-load or explicitly guard
  against clobbering, not one. This ADR accepts that doubled surface in exchange for the
  structural trust-boundary separation described above (Positive, bullet 1) — the
  tradeoff is real and is called out here explicitly rather than left implicit.

## Alternatives Considered

**Extend `BacklogProgressNote` with author fields, write with `criterion_index = -1`.**
Rejected. While `-1` is already used for item-level notes, those notes are always
written by the same trust boundary as the criterion-scoped ones (a
`report_progress`-linked session). Routing an ungated write path through the same table
and column set as the gated one creates exactly the confusability pitfalls.md flags: a
`get_backlog_item` renderer or a future maintainer could accidentally treat an ungated
`criterion_index = -1` row as equivalent in trust to a gated one, or a future change to
`report_progress`'s handling of `criterion_index = -1` rows could inadvertently touch
data written by the new ungated tool. Also, `status` has no meaningful value to write for
a pure comment, forcing an awkward sentinel (e.g. `""` or `"note"`) into a column whose
existing values are a closed, meaningful set.
