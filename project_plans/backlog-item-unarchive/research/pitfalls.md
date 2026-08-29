# Research: Pitfalls of Adding a Reverse (Unarchive) Transition

Agent 4 — Pitfalls. All line refs verified against the current worktree.

## 1. The two existing "archive" code paths already disagree — design against both

There are **two** independent ways an item reaches `status == "archived"` today, and
they behave differently:

- **Manual archive** — `EntRepository.ArchiveBacklogItem`
  (`session/ent_repository_backlog.go:741-780`): unconditional `UpdateOneID(...).SetArchivedAt(now).SetStatus(archived)`,
  no CAS/precondition, no `CanTransitionBacklog` guard check anywhere in its call
  chain (`server/services/backlog_service_lifecycle.go:340-368` doesn't check current
  status either). Emits `ChangeItemArchived` with `ArchivedAt` populated.
- **Auto-archive sweep** — `BacklogLifecycleListener.archiveStaleDoneItems`
  (`session/backlog_lifecycle.go:1334-1355`) calls `storage.TransitionBacklogItemStatus`
  (not `ArchiveBacklogItem`) with `precondition = {ExpectedStatus: "done"}`. That repo
  method (`session/ent_repository_backlog.go:869-951`) **never sets or clears
  `archived_at`** — confirmed by reading its full body; no `SetArchivedAt` /
  `SetNillableArchivedAt` / `ClearArchivedAt` call exists in it. Emits
  `ChangeStatusTransition`, not `ChangeItemArchived`.

**Consequence already live in the DB today**: items auto-archived by the stale-done
sweep have `status == "archived"` but `archived_at == nil`. Any unarchive logic that
assumes `archived_at` is always non-nil when `status == "archived"` (e.g. to render
"Archived on <date>", or to gate the unarchive button, or as an unarchive
precondition) will silently misbehave for sweep-archived items. This is a pre-existing
asymmetry, not something this feature introduces — but the plan must explicitly decide
whether unarchive-clearing (AC1) is the only fix, or whether the sweep's failure to
*set* `archived_at` on the way in is also worth closing in the same change (the
requirements doc's "out of scope" list doesn't mention this gap either way — flag it
for an explicit decision rather than silently building on the assumption).

## 2. Race window: sweep vs. manual unarchive is actually *precondition-shaped*, not timing-shaped

The two operations don't literally overlap on the same row at the same status —
sweep only ever transitions `done → archived` (precondition `ExpectedStatus: done`),
and unarchive only applies to `archived` items — so there's no window where both fire
on the same state simultaneously *if* both paths route through the CAS-protected
`TransitionBacklogItemStatus`. The real risk is if Unarchive is implemented as a
**bespoke repo method mirroring `ArchiveBacklogItem`** (unconditional `UpdateOneID`,
no CAS): then a double-click / double-tab unarchive race, or an unarchive racing a
different concurrent transition on the same item, has no protection at all. The
docstring on `TransitionBacklogItemStatus` (`session/ent_repository_backlog.go:850-868`)
documents exactly this failure mode — a prior read-then-write race
(`docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md`) reopened an item
that had already legitimately moved on, and a second incident let two dequeue sweeps
double-claim one item. **Any new unarchive write path needs the same genuine
SQL-level CAS** (`Update().Where(id, status=archived)` folded into the UPDATE, not a
separately-fetched-then-written check) — reusing `TransitionBacklogItemStatus` gets
this for free; a hand-rolled dedicated method does not, unless it's deliberately built
to match.

## 3. `CanTransitionBacklog` / gate validation is enforced at the *service* layer, not the repo layer — a dedicated RPC must not skip it

`server/services/backlog_service_lifecycle.go:506` (`s.engine.CanTransition(from, to)`)
and `:542` (`s.engine.ValidateGates(...)`) run inside the `TransitionBacklogItemStatus`
**handler**, before ever calling into storage. `EntRepository.TransitionBacklogItemStatus`
itself performs no legality check — it will CAS-update to *any* `toStatus` the caller
passes. `ArchiveBacklogItem`'s handler/repo pair has no guard check at all (by design —
archiving is allowed from any status). If the plan adds a dedicated
`UnarchiveBacklogItem` RPC (mirroring `ArchiveBacklogItem`'s shape) rather than routing
through the existing `TransitionBacklogItemStatus` RPC, it must **independently**
re-implement the `archived → idea` legality check (trivial today since that's the only
edge out of `archived` — `session/domain/backlog.go:385-387` — but a second, drifting
copy of that rule is exactly the kind of duplication the interface-pollution/DRY
concerns in this repo call out). Routing unarchive through the existing
`TransitionBacklogItemStatus` RPC avoids this duplication entirely and is the
lower-risk option.

## 4. `archived → idea` always lands on `idea`, never the pre-archive status

`validTransitions[BacklogStatusArchived] = {BacklogStatusIdea: true}` is the *only*
transition out of `archived` (`session/domain/backlog.go:385-387`). Since
`ArchiveBacklogItem` can archive an item from any status (`in_progress`, `review`,
`done`, etc. — no guard restricts *which* statuses can be archived) and its handler
best-effort cleans up the item's git worktrees on archive
(`server/services/backlog_service_lifecycle.go` `cleanupItemWorktrees` call in
`ArchiveBacklogItem`), unarchiving a mid-flight item does **not** restore it to
`in_progress` with a live worktree — it resets to `idea` with no worktree, matching a
fresh item. This is presumably intended (requirements explicitly put "any change to
the guard rules" out of scope), but the plan/UI copy should not imply "undo archive"
restores in-flight work state — only the requirements' literal "restore to active
status" claim.

## 5. `WatchBacklogItems` event-stream wiring: the existing `item_archived` event is a trap for symmetry-by-imitation

`web-app/src/lib/hooks/useWatchBacklogItems.ts:13-19` documents, by design, that
`BacklogItemArchivedEvent` (the `item_archived` oneof / `ChangeItemArchived` kind)
carries **only `itemId`/`archivedAt`, no full `BacklogItem` payload** — and is
*deliberately excluded* from the shared list-level `backlogItemsSlice` upsert path.
It's consumed only at the component layer via a separate item-scoped subscription
(`BacklogItemDetail.tsx:254`), not the shared/normalized list store.

This matters directly for AC1 ("the item reappears in default (non-archived) list
views"): if the plan copies `ArchiveBacklogItem`'s pattern for symmetry — a dedicated
`UnarchiveBacklogItem` RPC emitting a new sparse `item_unarchived` event with just
`itemId` — it will reproduce the same "not wired into the list-level slice" gap, and
the item will **not** automatically reappear in list views via the live watch stream
(only after a manual refetch/poll fallback, if one exists). Reusing the existing
`ChangeStatusTransition` / `status_transition` path (fix `TransitionBacklogItemStatus`
to clear `archived_at`, call it for `archived → idea`) sidesteps this entirely:
`status_transition` events already carry the full item and are already wired into
`backlogItemsSlice`, so AC1's list-reappearance requirement is satisfied with zero
additional frontend plumbing. If the plan nonetheless chooses the dedicated-RPC path,
it must explicitly design the new event's frontend handling to *not* repeat the
sparse-payload / no-upsert precedent, or the "reappears in default list views" AC will
silently fail in the live-stream fast path (still eventually correct after any REST
polling backstop this hook has, but not "live").

## 6. Audit trail (AC3) is automatic on one path, manual on the other

`TransitionBacklogItemStatus` calls `recordStatusEvent(...)` unconditionally at
`session/ent_repository_backlog.go:938` — reusing it satisfies AC3 ("status-event audit
history") for free. `ArchiveBacklogItem` also calls `recordStatusEvent` but does so as
an explicit, separate call in its own body (`:768`) — so a bespoke `UnarchiveBacklogItem`
repo method must remember to add this call itself; it is not emitted by any shared
helper both paths funnel through.

## 7. Feature-registry rule applies regardless of which implementation path is chosen

Per `.claude/rules/feature-registry.md`, "adding or modifying any feature — backend
RPC, frontend UI, or both" requires a `docs/registry/features/{backend,frontend}/*.json`
entry, an e2e test under `tests/e2e/`, and `make registry-generate`. This applies even
under the "fix `TransitionBacklogItemStatus` + new UI button" option — it's easy to
mentally categorize that as "just a bug fix" and skip the registry step, but the rule's
trigger is modifying backend RPC behavior or adding frontend UI, both of which apply
here. The new UI action also needs an e2e spec conforming to
`.claude/rules/e2e-test-conventions.md` (feature-annotation header, no
`waitForTimeout`, `data-testid`/ARIA locators only, page helpers under
`tests/e2e/pages/`) — matching AC5's frontend test-coverage requirement but at the e2e
layer specifically, which AC5 (Jest-level UI test) does not by itself satisfy.

## 8. ent-schema-generation rule: not triggered, but verify before assuming so

`.claude/rules/ent-schema-generation.md` requires `--feature sql/upsert` whenever
`session/ent/schema` changes. This feature needs **no schema change**: `archived_at`
and `status` already exist on `BacklogItem`, and the generated builder already exposes
`ClearArchivedAt()` (`session/ent/backlogitem_update.go:413-415` and the
`*UpdateOne` variant at `:1726-1728`) plus `SetNillableArchivedAt` — everything needed
to implement AC1's "clears `archived_at`" requirement is already generated. The only
pitfall here is process, not code: if the plan's implementer reflexively runs `ent
generate` because "we touched an ent-backed field," it's wasted/no-op work (or worse,
run with the wrong flags per the rule's "Wrong" example) for a feature that doesn't
need it. Only if the plan introduces a genuinely new column (e.g. a distinct
`unarchived_at` audit column, if `BacklogStatusEvent` rows are judged insufficient for
AC3) would `--feature sql/upsert` become relevant.

## Summary of explicit design decisions this feature should make up front

1. Route unarchive through `TransitionBacklogItemStatus` (fixed to `ClearArchivedAt()`
   on any outbound transition, or specifically on `archived → idea`) rather than a
   bespoke `UnarchiveBacklogItem` repo method — this reuses the existing CAS/precondition
   protection (#2, #3), audit trail (#6), and list-level event wiring (#5) instead of
   re-deriving all three from scratch.
2. If a dedicated RPC is chosen anyway for API-symmetry-with-Archive reasons, it must:
   independently enforce `CanTransitionBacklog(archived, idea)`, use a genuine
   CAS/precondition (not `UpdateOneID`), call `recordStatusEvent` itself, and carry a
   full `BacklogItem` payload in its event (not the sparse itemId-only shape
   `item_archived` uses) so list views update live.
3. Do not assume `archived_at != nil` whenever `status == "archived"` — sweep-archived
   items already violate that today.
4. Unarchive resets to `idea`, not the pre-archive status; don't imply otherwise in UI
   copy, and don't try to resurrect any worktree/session state torn down at archive time.
5. Registry entries (`docs/registry/features/`) and an e2e spec are required regardless
   of implementation path; ent codegen is not required for this feature as scoped.
