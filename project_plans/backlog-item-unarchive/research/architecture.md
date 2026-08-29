# Architecture Research: Backlog Item Unarchive (Agent 3)

## Verified facts

All facts cited in the assignment were re-verified against source, plus a few new ones needed to decide between options.

1. **State machine already guards `archived -> idea`, and only that target.**
   [`session/domain/backlog.go:384-386`](session/domain/backlog.go#L384-L386):
   ```go
   BacklogStatusArchived: {
       BacklogStatusIdea: true,
   },
   ```
   Confirmed by `TestCanTransition_ArchivedToIdeaIsExplicit` (`session/backlog_test.go:80-93`), which also asserts `archived->ready`, `archived->in_progress`, `archived->done` are all `false`.

2. **`ActionsSection.tsx` has zero render branches for `status === "archived"`.**
   `grep 'item.status ===' web-app/src/components/backlog/detail/ActionsSection.tsx` returns exactly 8 matches: `ready` (x2 helper vars), `idea`, `ready`, `queued`, `in_progress`, `review` (x2), `done`. No `archived` branch exists. The only archived-related UI in this file is a *different* mechanism — a `terminalState` prop (`"archived" | "removed" | null`) that renders a read-only "This item was archived elsewhere" notice, but that's driven by a **live watch event arriving while the detail pane is already open** (`BacklogItemDetail.tsx:216-282`, watching for `BacklogItemArchivedEvent`/`BacklogItemRemovedEvent`), not by the item's initial loaded status. An item that is *already* archived when the detail page loads gets no notice and no action — just silence.

3. **`send_back_idea` exists but is a red herring for this feature.** `BacklogItemDetail.tsx:549-551` maps it to `transitionStatus(item.id, "idea")`, and `ActionsSection.tsx:348` renders its button inside the `review`-status block (line 213), not an archived-status block. It's the "return to triage from review" action, unrelated to unarchive.

4. **`TransitionBacklogItemStatus` never touches `archived_at`, on any transition.** `session/ent_repository_backlog.go:869-950`. The `Update()` builder (line ~885) only calls `.SetStatus(...)` and `.SetUserModifiedStatusAt(now)`. Confirmed no `ArchivedAt`/`ClearArchivedAt` call exists anywhere in the function. So today, a raw API call transitioning `archived -> idea` would flip `status` back to `idea` but leave `archived_at` non-null — a real latent bug, not just a UI gap.

5. **`ClearArchivedAt()` already exists on the generated ent builder** — no `ent generate` needed. `session/ent/backlogitem_update.go:414` (`BacklogItemUpdate.ClearArchivedAt()`) and line 1727 (`BacklogItemUpdateOne` variant). The fix is a single additional builder call gated on `from == BacklogStatusArchived`, using the same `update` variable already built in `TransitionBacklogItemStatus`.

6. **The RPC-level guard path for `TransitionBacklogItemStatus` already fully supports `archived -> idea` today**, including gate checks. `server/services/backlog_service_lifecycle.go:486-560`: loads the item, checks `s.engine.CanTransition(from, to)` (line 504 — already `true` per fact #1), builds a `BacklogItemTransitionInput` and calls `s.engine.ValidateGates(guardInput, to)`. The gates that block transitions (AC-required, plan-required, verdict-required, code-not-on-main) are all keyed off the **target** status (`ready`, `done`, etc.) — none fire for `to == idea`, which is the least-restricted status in the machine. So no new business-rule wiring is needed for the RPC layer; it already accepts this exact transition.

7. **The repository method already writes the audit trail unconditionally.** `recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, fromStatus, string(toStatus), triggeredBy, note)` is called on every successful `TransitionBacklogItemStatus`, `session/ent_repository_backlog.go:929`. An `archived -> idea` transition through this path automatically satisfies AC3 (audit trail) with zero new code — it's a `BacklogStatusEvent` row with `from_status="archived", to_status="idea"` exactly like every other transition.

8. **Compare `ArchiveBacklogItem`** (`session/ent_repository_backlog.go:741-778`): it does *not* go through `TransitionBacklogItemStatus` or the guarded engine at all — it's a separate hand-rolled `UpdateOneID().SetArchivedAt(now).SetStatus(...).SetUserModifiedStatusAt(now)` call, with its own `recordStatusEvent` call and its own event kind (`ChangeItemArchived` → proto `BacklogItemArchivedEvent`, a distinct oneof branch in `BacklogItemEvent`, `proto/session/v1/backlog.proto:608-693`). This asymmetry exists because *archive* bypasses the state-machine engine's guard/gate checks entirely (it's allowed from any active status) — not because "leaving archived" needs special event semantics. Unarchive is the mirror-image case: it *is* a normal guarded engine transition (fact #6), so it fits the generic path, unlike archive.

9. **`BacklogItemStatusChangedEvent` already carries both `old_status` and `new_status`** (`proto/session/v1/backlog.proto:641-654`). Any `WatchBacklogItems` consumer that cares about "this item was just reopened from archive" specifically (vs. any other transition) can already test `old_status == "archived" && new_status == "idea"` on the existing generic event — no new proto message or oneof branch is required to make that distinction observable.

10. **`UnarchiveSession` (session lifecycle) is a poor structural analog, not a template to copy.** `server/services/session_service.go:4234-4248`: it's a two-line `SetArchivedAt(nil)` + save, because sessions have **no status state machine** — `archived_at` is the *only* state being toggled. Backlog items are different: they have both a status state machine *and* an `archived_at` timestamp, and the ask's own state-machine already models "unarchive" as a guarded status transition (`archived -> idea`), not a bare timestamp flip. Mirroring `UnarchiveSession`'s literal shape (a bespoke RPC that only clears a timestamp) would either (a) skip the state machine and just flip `status` + `archived_at` directly, duplicating and bypassing `CanTransition`/`ValidateGates`/`recordStatusEvent` that `TransitionBacklogItemStatus` already provides, or (b) internally call `TransitionBacklogItemStatus` anyway, making the new RPC a pure pass-through wrapper.

## Options

### Option A — Dedicated `UnarchiveBacklogItem` RPC + new `BacklogItemUnarchivedEvent`

- New `rpc UnarchiveBacklogItem` in `proto/session/v1/backlog.proto`, new request/response messages, `make proto-gen`.
- New `BacklogService.UnarchiveBacklogItem` handler — either (a) reimplements `CanTransition`/`ValidateGates`/`recordStatusEvent`/`ClearArchivedAt` from scratch (duplicating fact #6/#7's logic verbatim), or (b) calls `s.storage.TransitionBacklogItemStatus(ctx, id, BacklogStatusIdea, ...)` internally and is a thin wrapper.
- New repository method or reuse of `TransitionBacklogItemStatus` with the `archived_at`-clear fix from fact #4/#5 either way — **the leak fix is mandatory in either option**, it doesn't disappear by adding a new RPC.
- New `BacklogItemUnarchivedEvent` message + oneof branch, new `BacklogChangeKind` constant, new case in `backlog_item_event_publisher.go` and `backlog_service_events.go`, new frontend event-type handling in `useWatchBacklogItems.ts`.
- New frontend RPC call (`useBacklogService.ts` / equivalent hook), new `unarchiveBacklogItem` action wired into `ActionsSection.tsx`.
- Matches the literal wording of the backlog item's ask ("RPC + repository method + UI action... matching the existing session pattern") and gives archive/unarchive full symmetry as an RPC pair, mirroring `ArchiveBacklogItem`/`DeleteBacklogItem`.
- Cost: new proto surface (2+ messages, 1 RPC, 1 oneof branch), new event plumbing across 4+ files, and — per fact #9 — the new event type adds no information a consumer couldn't already derive from `old_status`/`new_status` on the existing generic event. It also re-introduces exactly the "wrapper-only" smell called out in `.claude/rules/interface-pollution-checklist.md` (smell #4, forwarding-only wrapper) if implemented as (b), or duplicates guard logic if implemented as (a).

### Option B — Fix `TransitionBacklogItemStatus` to clear `archived_at`, add UI action that calls it directly (no new RPC)

- One repository-level fix: in `TransitionBacklogItemStatus` (`session/ent_repository_backlog.go`, right where `update := r.client.BacklogItem.Update().Where(...)` is built), add:
  ```go
  if from == BacklogStatusArchived {
      update = update.ClearArchivedAt()
  }
  ```
  `from` is already computed a few lines below as `current.Status` — needs to be hoisted a few lines earlier, or the field name compared directly off `current.Status` (a string) before the builder. Either is a small, local diff. `ClearArchivedAt()` already exists (fact #5) — no `ent generate` required.
- No proto changes, no `make proto-gen`, no new RPC, no new event type — the existing `BacklogItemStatusChangedEvent` (fact #9) already carries everything a watcher needs.
- Audit trail (AC3) is satisfied automatically by the existing `recordStatusEvent` call (fact #7) — zero new code.
- UI: add an `item.status === "archived"` branch to `ActionsSection.tsx` with a button calling `onAction("unarchive")`; wire `case "unarchive": await transitionStatus(item.id, "idea"); break;` in `BacklogItemDetail.tsx`'s existing `handleAction` switch (same pattern as every other status-transition action already in that switch, e.g. `mark_ready`, `mark_done`).
- Cost: ~5-15 lines across 3 files (`ent_repository_backlog.go`, `ActionsSection.tsx`, `BacklogItemDetail.tsx`), plus tests. No proto/codegen, no new watcher plumbing.
- Deviates from the ask's literal phrasing ("Add `UnarchiveBacklogItem` (RPC + repository method + UI action)") but the ask's own AC1 explicitly leaves this open: *"either a dedicated `UnarchiveBacklogItem` RPC, or a fix to `TransitionBacklogItemStatus` plus a UI-level call into it — implementation decided in plan.md"* — i.e., the requirements doc already anticipated and authorized Option B as an equally-valid resolution.

## Recommendation: Option B

Reasoning, in order of weight:

1. **The `archived_at` leak fix (fact #4) is required under both options and is the actual bug.** Option A does not avoid writing this fix — it just relocates where it's called from. Since the fix has to exist in `TransitionBacklogItemStatus` (or a copy of its logic) either way, the only real question Option A adds is "is a new RPC/event worth the extra surface," and fact #9 answers that: no, the information is already on the wire via `old_status`/`new_status`.
2. **The RPC layer (fact #6) and audit trail (fact #7) already handle this transition correctly today** — every gate check, precondition, and status-event record that a hand-written `UnarchiveBacklogItem` handler would need to reimplement already runs, unmodified, for `target_status="idea"`. Building a parallel RPC either duplicates that logic (bug-prone: two places that must agree on gates forever) or wraps it (interface-pollution smell #4 in this repo's own checklist).
3. **`ArchiveBacklogItem` is a poor precedent for "RPC pair symmetry" because archive itself is structurally special** (fact #8): it bypasses the guard engine because it's valid from *any* active status, so it needs its own code path and event type. Unarchive has exactly one valid source status and is already inside the guarded engine — it doesn't have the same structural reason to be special-cased.
4. **Blast radius**: Option B touches 3 files with no proto regen; Option A touches proto definitions, 2+ generated-code consumers (`backlog_item_event_publisher.go`, `backlog_service_events.go`), the TS event-union in `useWatchBacklogItems.ts`, plus everything Option B also needs. Given `make proto-gen` regenerates both Go and TS bindings repo-wide, Option A's diff is materially larger for zero behavioral gain per fact #9.
5. **The requirements doc already blesses this choice** (AC1's explicit "or" clause) — this isn't overriding the ask, it's exercising the discretion the ask granted.

The one thing Option B must get right that a naive read of AC1 might skip: the `archived_at` clear must be scoped to `from == BacklogStatusArchived` (or equivalently, whenever `archived_at` is currently set), not unconditionally on every transition — a generic unconditional clear would be a silent behavior change for other transitions that happen to run through the same method. Since `archived` only ever transitions to `idea` in the table (fact #1), gating on `from == BacklogStatusArchived` is both correct today and future-proof if the table ever grows a second target from `archived`.

## Event-Command-Policy table

Skipped. This is a single-actor, single-transition feature (a user clicking one button triggers one guarded state transition that already exists in the domain model) with no new cross-system policy, no automatic/scheduled trigger, and no new consumer-facing event contract (fact #9). It fits the existing `TransitionBacklogItemStatus` command path used by every other manual status change in this UI (`mark_ready`, `mark_done`, `send_back_idea`, etc.) — a guarded-CRUD update, not a multi-actor workflow.
