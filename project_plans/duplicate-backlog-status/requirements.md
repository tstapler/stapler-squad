# Requirements: Duplicate Backlog Status

**Project**: duplicate-backlog-status
**Date**: 2026-07-29
**Status**: Draft
**Source backlog item**: `4f03de7b-3fca-4f3a-84cb-8c6c5abede50`

---

## Problem Statement

The backlog status state machine (`session/backlog.go`) has `idea`, `refining`, `ready`,
`in_progress`, `review`, `done`, `archived` — but no `duplicate` status. When triage
discovers an item describes the same problem as another item, the only option today is
`archived` plus a free-text note pointing at the canonical item. That loses structure:
nothing queryable links the two items, `ListBacklogItems` can't distinguish "archived
because duplicate" from "archived because no longer relevant," and the UI has no way to
jump from a duplicate to its canonical item.

Concrete motivating case: three backlog items (`10128af0-e1eb-47bc-9016-3af8fde83b4d`,
`1dc7ff10-326c-4276-a70f-eb8869713593`, and short-id `67de6c7b`) all independently
describe the same `install-service.sh` `.zshrc`-sourcing bug, discovered via three
separate GitHub-issue-sync imports. Resolving this required manual cross-referencing in
free-text notes rather than a structured relationship.

Related-but-narrower item `5f6975a8-79df-481b-86bb-da9a3340117f` covers exposing
archive/notes RPCs via MCP; this item is about the data model itself lacking a duplicate
concept, plus a purpose-built `mark_duplicate` MCP tool.

## Goals

1. Add `duplicate` as a first-class `BacklogStatus` with correct state-machine transitions.
2. Add a `duplicate_of_id` field on `BacklogItem` (ent schema + proto) that links a
   duplicate item to its canonical counterpart.
3. Enforce data integrity on the transition: `duplicate_of_id` must be set, non-self,
   and reference an existing item before a transition to `duplicate` is allowed.
4. Persist status and `duplicate_of_id` atomically, with optimistic-concurrency
   protection against lost updates.
5. Give agents (triage/work sessions) a self-service MCP tool, `mark_duplicate`, so they
   don't have to fall back to archive + free-text note.
6. Exclude `duplicate` items from default/active backlog views, same as `archived`/`done`.
7. Give the web UI a distinct visual treatment for `duplicate` (not reusing `archived`'s
   badge) with a working link to the canonical item.
8. Update triage-facing guidance to point at `mark_duplicate`, and backfill the three
   motivating example items using the shipped tool.

## Non-Goals

- Bulk/automated duplicate detection (e.g. embedding similarity search) — this item only
  adds the structural primitive; detection remains a human/agent judgment call.
- Merging content between duplicate and canonical items (notes, AC, sessions) — only the
  link is structural, not a data merge.
- Allowing `done → duplicate` — deliberately excluded per AC2; a shipped item cannot
  retroactively become "duplicate."
- Exposing `mark_duplicate` as a ConnectRPC-callable action from the web UI directly
  (it's an MCP tool for agent sessions per the source item; the UI only needs to *render*
  the existing `duplicate_of_id`/status, not set it).
- Reverse lookup (given canonical item X, list all its duplicates) — not covered by any
  of the 13 numbered acceptance criteria; flagged as a follow-up, not built now.
- Chain/cycle prevention beyond a single hop: **in scope** is forbidding marking an item
  duplicate-of a target that is *itself* already `duplicate` status — this single guard
  rule blocks both multi-hop chains (A dup-of B, B dup-of C) and mutual duplication
  (A dup-of B, B dup-of A) at the point an item is marked duplicate, since only
  non-duplicate-status items can be legal targets (see research/features.md §3 for the
  full rationale: it avoids chain-walk display complexity and keeps the forward link
  always exactly one hop). This narrows AC6's "existing target" wording: a target must
  be existing, non-self, AND not itself already `duplicate`-status to be valid. Anything
  beyond this single-hop rule (e.g. resolving/flattening multi-hop chains for display) is
  out of scope.

## Functional Requirements

### FR1 — State machine (`session/backlog.go`)
- Add `BacklogStatusDuplicate BacklogStatus = "duplicate"`.
- `validTransitions` gains:
  - `idea → duplicate`, `refining → duplicate`, `ready → duplicate`,
    `in_progress → duplicate`, `review → duplicate` (from any active status).
  - `duplicate → idea` (reopen a wrongly-marked duplicate).
  - `done → duplicate` is explicitly NOT added.
- `CanTransitionBacklog` must return correct bools for every new edge and every
  previously-invalid edge into/out of `duplicate` (e.g. `done → duplicate` is false,
  `archived → duplicate` is false since it's not in the proposed set).

### FR2 — Data model (ent schema + proto)
- `session/ent/schema/backlog_item.go`: add `field.String("duplicate_of_id").Optional()`.
- Regenerate ent client via
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  (the `--feature sql/upsert` flag is mandatory per this repo's conventions). Generated
  output under `session/ent/*.go` (excluding `schema/`) stays gitignored/uncommitted;
  only `schema/backlog_item.go` is committed.
- `proto/session/v1/backlog.proto`: add `string duplicate_of_id` to both `BacklogItem`
  and `TransitionBacklogItemStatusRequest` (next available field numbers). Regenerate via
  `make proto-gen` (this repo's actual target name — the item description's
  `make generate-proto` does not exist as a Makefile target). Generated bindings under
  `gen/proto/go/...` and `web-app/src/gen/...` stay gitignored/uncommitted.

### FR3 — Transition guard (`TransitionGuard` in `session/backlog.go`)
- New sentinel errors: one each for empty `duplicate_of_id`, self-reference, and
  reference to a nonexistent item, all raised only for transitions targeting
  `duplicate`.
- `BacklogItemTransitionInput` gains a `DuplicateOfID` field (and something to check
  existence — see FR4 for how the service layer supplies this, since `TransitionGuard`
  itself is pure/DB-free per its existing signature).
- Guard rejects when `duplicate_of_id` is empty, equals the item's own id, or does not
  resolve to an existing backlog item; accepts a valid non-self, existing target.

### FR4 — Request plumbing + atomic write
- `TransitionBacklogItemStatusRequest` (proto) gains `string duplicate_of_id` so callers
  can supply it as part of the transition call (per the item's proposed fix — extend the
  transition request rather than requiring a prior `UpdateBacklogItem` call).
- `BacklogService.TransitionBacklogItemStatus` (server/services/backlog_service.go):
  - Looks up the target item (when `duplicate_of_id` is set) to feed `TransitionGuard`
    existence-checking before calling the repository.
  - Passes `duplicate_of_id` through to the repository update.
- `EntRepository.TransitionBacklogItemStatus` (session/ent_repository_backlog.go):
  - Writes `status` and `duplicate_of_id` in the same `UpdateOneID(...).Save(ctx)` call
    (one atomic update, not two round trips).
  - Adds an optimistic-concurrency guard: the update predicate requires
    `StatusEQ(current.Status)` (ent `.Where(...)` on the update builder); if the
    conditional update affects zero rows (i.e. status changed between read and write),
    return `ErrPreconditionFailed` — this closes the existing read-then-blind-write race
    in this method, not just for the duplicate path.

### FR5 — `mark_duplicate` MCP tool (`server/mcp/tools_backlog.go`)
- New tool `mark_duplicate(item_id, duplicate_of_id, note?)` following the existing
  handler pattern (`callerSessionUUID`, `validateUUID`, session-item link check where
  applicable).
- Performs, end-to-end: `CanTransitionBacklog` check, `TransitionGuard` check (via the
  same guard path the RPC uses, so behavior can't drift from the RPC), then the atomic
  transition.
- Disambiguates not-found (`item_id` or `duplicate_of_id` don't exist → `ErrNotFound`-style
  MCP error) from infra errors (DB/connection failures → internal error).
- Best-effort note append: if `note` is supplied, appends it to the item's `notes` field;
  a failure to append must not fail the overall tool call (the transition already
  succeeded) — log and report the transition result with a caveat.

### FR6 — List exclusion (`ListBacklogItems`)
- Default/active-item queries (`ExcludeTerminal` / `StatusNotIn`-style filtering) treat
  `duplicate` the same as `archived`/`done`: excluded unless the caller explicitly
  requests it via `include_terminal` or an explicit `status` filter.

### FR7 — Web UI
- `BacklogItemBadge`, `BacklogItemDetail`, the backlog table + filter chips, and
  `BacklogItemCard`'s action spec all render `duplicate` as visually distinct from
  `archived` — own CSS class / vanilla-extract token, not a reused `archived` style.
- New color tokens added to the theme contract with WCAG-AA contrast in all 6 themes
  (per `.claude/rules/css-architecture.md` conventions — tokens in `theme.css.ts`/
  `globals.css`, no hardcoded hex, no `var()` string literals in `.css.ts` files).
- `BacklogItemDetail` shows a client-side-resolved "Duplicate of: <title>" link to the
  canonical item when `duplicate_of_id` is set. If the canonical item can't be resolved
  (deleted, bad id), degrade to plain "item not found" text — no broken link, no crash.

### FR8 — Tests
- Go: new unit tests in `session/backlog_test.go` for the new transitions/guard edges;
  `backlog_service_test.go` for the atomic write + optimistic concurrency +
  `TransitionBacklogItemStatus` request plumbing; `tools_backlog_test.go` for
  `mark_duplicate` (happy path, not-found disambiguation, guard rejection, best-effort
  note failure).
- Frontend: Jest tests for badge rendering (all statuses incl. `duplicate`), list
  exclusion behavior, and the canonical-item-link resolution (success + missing-target
  degradation).
- All existing Go/Jest suites continue to pass.

### FR9 — Documentation / adoption
- Update triage-facing guidance (CLAUDE.md/rules or `mark_duplicate`'s own MCP tool
  description) to direct agents to use `mark_duplicate` instead of archive + note.
- Backfill the three motivating example items (`10128af0-e1eb-47bc-9016-3af8fde83b4d`,
  `1dc7ff10-326c-4276-a70f-eb8869713593`, and `67de6c7b*`) using the shipped tool as
  adoption proof — pick one as canonical (the item description doesn't specify which;
  default to the earliest-created/most complete one) and mark the other two as its
  duplicates.

## Acceptance Criteria (verbatim from backlog item, numbered for `report_progress`)

1. `session/backlog.go` defines `BacklogStatusDuplicate BacklogStatus = "duplicate"`.
2. `validTransitions` allows `idea|refining|ready|in_progress|review → duplicate` and
   `duplicate → idea` (reopen); `done → duplicate` is deliberately not added.
3. `CanTransitionBacklog` returns the correct bool for all new/rejected edges, covered by
   unit tests.
4. `BacklogItem` ent schema gets an `Optional()` `duplicate_of_id` string field,
   regenerated via
   `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`;
   generated output stays gitignored/uncommitted, only `schema/backlog_item.go` is
   committed.
5. `proto/session/v1/backlog.proto`'s `BacklogItem` and
   `TransitionBacklogItemStatusRequest` carry a `duplicate_of_id` field, regenerated via
   `make proto-gen` (not `make generate-proto`); generated bindings stay
   gitignored/uncommitted.
6. `TransitionGuard` rejects a transition to `duplicate` when `duplicate_of_id` is empty,
   self-referencing, or references a nonexistent item (three new sentinel errors), and
   accepts a valid non-self, existing target.
7. `TransitionBacklogItemStatus` writes `status` and `duplicate_of_id` atomically in one
   update, guarded by an optimistic-concurrency `StatusEQ(current.Status)` precondition
   that returns `ErrPreconditionFailed` on a stale write.
8. A `mark_duplicate` MCP tool (`item_id`, `duplicate_of_id`, optional `note`) in
   `server/mcp/tools_backlog.go` performs `CanTransitionBacklog` + `TransitionGuard` +
   the transition end-to-end, with not-found vs. infra-error disambiguation and a
   best-effort note append.
9. `ListBacklogItems` excludes `duplicate` items from default/active-item results
   (`ExcludeTerminal`/`StatusNotIn`), matching existing `archived`/`done` exclusion
   behavior.
10. Web UI renders a distinct `duplicate` status badge (own CSS class/color token, not
    reusing `archived`'s) across every status-aware surface: `BacklogItemBadge`,
    `BacklogItemDetail`, the backlog table + filter chips, and `BacklogItemCard`'s action
    spec — with WCAG-AA-contrast tokens in all 6 themes.
11. `BacklogItemDetail` shows a client-side-resolved "Duplicate of: " link to the
    canonical item when `duplicate_of_id` is set, degrading to plain "item not found"
    text (no broken link/crash) when the target is missing.
12. Existing and new Go/Jest test suites pass (`session/backlog_test.go`,
    `backlog_service_test.go`, `tools_backlog_test.go`, frontend Jest tests), including
    new tests for the duplicate transitions, guard, `mark_duplicate` tool, list
    exclusion, and badge/link rendering.
13. Triage-facing guidance (CLAUDE.md/rules or the MCP tool's own description) is updated
    to direct agents to `mark_duplicate` instead of archive+note, and the three
    motivating example items are backfilled using the shipped tool as adoption proof.

## Constraints / Conventions Discovered During Scoping

- `session/ent/*.go` (excluding `schema/`) and generated proto output are gitignored;
  only source schema/proto files are committed. Confirmed via `.gitignore` and empty
  `session/ent/` (no generated files present until `go generate`/`proto-gen` runs).
- This repo's actual Makefile target is `proto-gen`, not `generate-proto` (the latter
  doesn't exist) — AC5 already corrects for this; requirements follow AC5.
- `TransitionBacklogItemStatus`'s current repository implementation
  (`session/ent_repository_backlog.go`) already does a precondition check via a
  read-then-compare pattern for `ExpectedStatus`/`ExpectedUpdatedAt`, but the actual
  `Save()` call has no `.Where()` tying the write back to the read — i.e. there's a
  window between the read and the write where a concurrent transition could land and be
  silently overwritten. FR4 closes this generally as part of adding the atomic
  status+duplicate_of_id write, not just for the new field.
- Existing MCP handlers (`reportProgress`, `requestReview`, `submitReviewVerdict`) follow
  a consistent pattern: `callerSessionUUID` → arg validation via `validateUUID` →
  business logic → `errResult(code, msg, hint)` on failure. `mark_duplicate` should match
  this shape.
- CSS work must follow `.claude/rules/css-architecture.md` (vanilla-extract for new
  styles, theme tokens only, no hardcoded hex/`var()` strings).
