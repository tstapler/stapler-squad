# Features Research — `duplicate` Backlog Status

## 1. How `archived` is implemented end-to-end (the direct precedent)

**State machine** (`session/backlog.go`):
- `BacklogStatusArchived BacklogStatus = "archived"` (line 18), one of 7 enum values.
- `validTransitions` (lines 121–151): idea/refining/ready/done → archived; **archived → idea** is the only reopen path (line 148–150). No guard function fires for these transitions in `TransitionGuard` — they fall into the `default: return nil` case (line 221–224). Only `idea→ready`, `ready→in_progress`, `refining→ready`, `review→done` have guard bodies today.
- Notably, **`in_progress` and `review` cannot transition to `archived`** in the current table (only `idea`, `refining`, `ready`, `done` can). The new AC set requires duplicate to be reachable from `idea/refining/ready/in_progress/review` — i.e. a **strictly wider fan-in** than archived's. This is the first explicit divergence: don't copy archived's transition edges verbatim, the ACs call for more.

**ent schema** (`session/ent/schema/backlog_item.go`):
- `archived_at` is a plain `field.Time(...).Optional().Nillable()` (lines 60–62) — no additional entity, no edge. Set alongside `status` and `user_modified_status_at` in one `UpdateOneID(...).Save(ctx)` call.
- No proto field per se for `archived_at` beyond timestamp on the general item — check `backlogItemToProto`/`ArchiveBacklogItemResponse` wiring if duplicate needs its own RPC response shape.

**Repository layer** (`session/ent_repository_backlog.go`):
- `ArchiveBacklogItem(ctx, id)` (lines 251–271): fetches nothing first, just does one `UpdateOneID(parsedID).SetArchivedAt(now).SetStatus(archived).SetUserModifiedStatusAt(now).Save(ctx)`. **No optimistic-concurrency precondition check at all** — diverges from `UpdateBacklogItem`/`TransitionBacklogItemStatus`, which both fetch current row first and compare `precondition.ExpectedStatus`/`ExpectedUpdatedAt` against it, returning `ErrPreconditionFailed` on mismatch (lines 183–205, 274–296). **The duplicate-status AC explicitly requires optimistic concurrency for the atomic status+duplicate_of_id write** — so the mark-duplicate repository method should follow the `TransitionBacklogItemStatus` pattern (fetch-then-conditional-update), not the simpler `ArchiveBacklogItem` pattern. This is the second explicit divergence.
- `TransitionBacklogItemStatus` also appends an audit row to `BacklogStatusEvent` (lines 309–318) as a best-effort/non-fatal side effect — `ArchiveBacklogItem` does **not** write an audit event. Decide whether duplicate transitions get an audit trail; recommend yes (via the same non-fatal append pattern) since it flows through a proper state-machine transition, not a one-off archival action.

**List exclusion** (`session/ent_repository_backlog.go` lines 133–144):
```go
if len(filter.Statuses) > 0 {
    q = q.Where(backlogitem.StatusIn(filter.Statuses...))
} else if filter.ExcludeTerminal {
    q = q.Where(backlogitem.StatusNotIn(
        string(BacklogStatusDone),
        string(BacklogStatusArchived),
    ))
}
```
Simple hardcoded 2-element exclusion list gated by `filter.ExcludeTerminal` (set from `!req.Msg.IncludeTerminal` in `server/services/backlog_service.go:466`, and reset to `false` when an explicit status filter is passed, line 470). Adding duplicate exclusion is a one-line change: append `string(BacklogStatusDuplicate)` to `StatusNotIn(...)`. Note the field is named `ExcludeTerminal` even though `archived`/`duplicate` aren't truly terminal (both can reopen to `idea`) — it really means "hidden by default." No rename needed, just extend the list; renaming would be a larger, out-of-scope refactor.

**Web UI** (`web-app/src/components/backlog/`):
- `BacklogItemBadge.tsx`: `STATUS_CLASS` record maps status string → CSS class; `getStatusClass` falls back to `styles.statusArchived` for unknown statuses (line 24) — **this fallback default needs its own explicit `duplicate` entry**, otherwise an item marked duplicate would visually render as if archived until the map is updated.
- `lib/backlog/status.ts`: `STATUS_LABELS` record + `getStatusLabel` fallback (title-cases unknown statuses by replacing `_`→space). Needs a `duplicate: "Duplicate"` entry.
- `BacklogItemBadge.css.ts`: per-status CSS classes reference `vars.statusBadge.<name>Bg/Fg/Border` design tokens for some statuses (ready/in_progress/review/done use dedicated `input*`/`uncommitted*`/`approval*`/`complete*` token triads) and generic tokens for others (idea uses `surfaceMuted`/`textMuted`/`borderMuted`; archived reuses the same `surfaceMuted`/`textDisabled`/`borderMuted`; refining uses `warningBg`/`warningText`/`warning`). **A new `duplicate` status needs its own token triad** (e.g. `vars.statusBadge.duplicateBg/duplicateFg/duplicateBorder`) added to `theme-contract.css.ts` (currently defines `approvalBg/Fg/Border`, `inputBg/Fg/Border`, `completeBg/Fg/Border`, `uncommittedBg/Fg/Border`, `idleBg/Fg/Border`, `staleFg`, `processingBg/Fg/Border` — no `duplicate*` slot yet), then given concrete hex values in **all 6** theme implementation blocks in `theme.css.ts`: `lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme` (found via `createTheme(vars, {...})` at lines 55, 149, 246, 351, 456, 561). Each must independently satisfy WCAG AA contrast for its own background/foreground pairing — this is nontrivial design work, not just plumbing.
- `BacklogItemCard.tsx`: `getActionSpec(item)` switch-cases status → `{ label, action, isDone/disabled }` for card button behavior; `archived` case returns `{ label: "Archived", action: "archived", isDone: true }` (a disabled/terminal-looking button). Needs a `duplicate` case, likely `{ label: "Duplicate", action: "duplicate", isDone: true }` plus (per the ACs) a "Duplicate of: <link>" treatment — the card and `BacklogItemDetail.tsx` will need new conditional rendering blocks (the existing file already has ~10 status-conditional blocks, e.g. `item.status === "review"`, `item.status === "done"`), following that established per-status-conditional-JSX-block pattern.
- `web-app/src/app/backlog/page.tsx`: `ALL_STATUSES` array (lines 29–37) drives both `StatusFilterChips` and the `STATUS_CSS` map; `"archived"` is explicitly excluded from the default-displayed filter chips (line 87–88, comment: "too noisy") even though it's in `ALL_STATUSES`. Recommend the same treatment for `duplicate` — add to `ALL_STATUSES` (needed for table sorting/coloring) but exclude it from the default `displayStatuses` filter-chip list.

## 2. Precedent for "linked entity" / self-referential relations in this ent schema

Checked every schema file in `session/ent/schema/` for self-referential edges (an entity with an edge pointing back to its own `.Type`) — **there is no precedent**. The one apparent match (`Session` → `edge.To("claude_session", ClaudeSession.Type)`) is a different entity type (`ClaudeSession`, not `Session`), a false positive from substring matching.

Existing "linked entity" patterns are all **cross-type** relations, not self-references:
- `BacklogItem` ↔ `ItemSource`: `edge.From("source", ItemSource.Type).Ref("backlog_items").Unique()` on `BacklogItem`, paired with `edge.To("backlog_items", BacklogItem.Type)` on `ItemSource` — a standard many-to-one FK edge (many backlog items per source), resolved via `.WithSource()` eager-loading in `ListBacklogItems` (`ent_repository_backlog.go:170`).
- `BacklogItem` → `ItemSession`, `BacklogItem` → `Session`, `BacklogItem` → `BacklogStatusEvent`: all one-to-many `edge.To(...)`.

**Recommendation**: given no self-referential ent edge exists anywhere in this codebase, and the acceptance criteria already specify `duplicate_of_id` as a plain string field (not an edge), the simplest-safe path that matches both precedent-avoidance and the stated AC is a **plain nullable string/UUID field** (`field.String("duplicate_of_id").Optional()` or `field.UUID(...).Optional().Nillable()`), validated at the application layer (`TransitionGuard`) rather than an ent self-edge with FK constraints. Rationale:
  - A self-edge would require ent to generate a unique/self-referential foreign key on the same table, which is more schema-migration risk for a feature this codebase has never exercised before.
  - `archived_at`'s own precedent — bare nullable field, app-level logic — is the more relevant analog than `ItemSource`'s edge, since `ItemSource` is genuinely a distinct entity type with its own table/lifecycle, whereas duplicate-of references a row of the **same** table.
  - App-level validation (empty / self-reference / nonexistent-target checks, as the ACs specify) is exactly the kind of guard `TransitionGuard` already centralizes for other transitions (see `ErrACRequired`, `ErrPlanRequired` sentinel-error pattern in `session/backlog.go:162–168`) — three new sentinel errors (e.g. `ErrDuplicateTargetEmpty`, `ErrDuplicateTargetSelf`, `ErrDuplicateTargetNotFound`) fit that established idiom directly.
  - Downside to accept: no DB-level referential integrity (a deleted target row silently orphans the `duplicate_of_id` string) — mitigated by the "graceful missing-target degradation" AC, which already anticipates this and asks for app-level handling, not a DB constraint.

## 3. Edge cases: chains, deletion of target, mutual duplication

None of the 13 stated ACs address these; they need an explicit design decision in planning.

**Chains** (A dup-of B, B later marked dup-of C): Recommend **forbidding** marking an item duplicate of a target that is *itself* already `duplicate` status. Simplest safe rule: in `TransitionGuard`'s new `from X → duplicate` guard, reject if `TargetItem.Status == BacklogStatusDuplicate` (surfaced as one of the "nonexistent target" family of errors, or a 4th distinct sentinel e.g. `ErrDuplicateTargetIsDuplicate`). This avoids: (a) needing to resolve/flatten chains for display ("Duplicate of: B" which is itself "Duplicate of: C" — confusing UX and an extra query hop the ACs don't budget for), (b) cycles becoming reachable transitively even if direct mutual dup (A↔B) is blocked. Enforcing "canonical items are never themselves duplicates" keeps the forward link always exactly one hop, matching the ACs' "Duplicate of:" text/link treatment which implies a single resolved target, not a chain-walk.

**Mutual duplication** (A dup-of B, B dup-of A): Directly prevented by the chain rule above — if A is marked dup-of B, B becomes ineligible to be marked dup-of anything (since only non-duplicate statuses transition *into* duplicate per the AC list; B is only reachable as `duplicate → idea` reopen, and only from `duplicate`, not into a new duplicate-of-target while still `duplicate`). Worth an explicit unit test regardless, since the guard is doing double duty (chains + mutual both blocked by the same "target must not already be duplicate" check).

**Target deleted/archived**: There is no hard delete path found for `BacklogItem` in the repository layer (only soft-delete via `archived_at`+status). So "deleted" reduces to "archived." Recommend: **allow** marking duplicate-of an archived item (archived items aren't forbidden as targets under the AC's literal "nonexistent target" check — the row still exists) but this is a product-judgment call worth flagging to the user/planner: an archived canonical item behind a "Duplicate of:" link may look like a dead end in the UI. The "graceful missing-target degradation" AC should extend to: target row exists but is itself archived/duplicate → still render the link, just visually de-emphasized (matching the existing `statusArchived` muted-token treatment already in the badge CSS).

## 4. Reverse lookup (given canonical item X, list all its duplicates) — scope check

The 13 ACs, as read, cover only the **forward** link: `duplicate_of_id` field, `mark_duplicate` tool, badge/detail rendering of "Duplicate of: <canonical>", and list-exclusion of duplicate items themselves. None of the 13 mention:
- A new `ListBacklogItems` filter/param to fetch items where `duplicate_of_id == X`.
- Any UI affordance on the canonical item's card/detail view showing "N items marked as duplicates of this."

**Recommendation: out of scope for this feature.** The literal AC text supports only the forward direction. Flag reverse lookup as a natural, low-risk follow-up (a straightforward `WHERE duplicate_of_id = ?` query plus a small badge/count on the canonical item's detail view) — but do not build it now, since it's not testable against any stated AC and would inflate the task list unreviewed. If a reviewer/user wants it in-scope, it can be added as AC #14 rather than assumed.

## 5. Industry comparison: "mark as duplicate" in other issue trackers

- **Jira**: "Duplicate" is a *link type* ("this issue duplicates / is duplicated by"), not a workflow *status*. The issue keeps its original status (e.g. still "To Do") and gets a separate relationship link plus, by convention, a manual transition to a terminal "Closed" status with resolution = "Duplicate." Two issues can each link to the other bidirectionally ("duplicates" / "is duplicated by" are inverse link labels on the same relationship), and Jira does not prevent chains — A can duplicate B which duplicates C, and Jira does not auto-resolve/collapse the chain for you.
- **GitHub Issues**: No native "duplicate" state at all — convention is a `duplicate` **label** plus a bot/manual comment "Duplicate of #123" and then closing the issue as "not planned." The reference is a free-text/markdown issue-number mention, not a structured field — no validation, no chain prevention, fully manual.
- **Linear**: Has a first-class "Mark as duplicate" action that sets a genuine `duplicateOf` relation (structured, queryable) and visually shows "Duplicate of ENG-123" with a clickable chip; marking auto-moves the item to a `Duplicate`/`Cancelled`-adjacent state. This is the closest industry analog to what's being built here — a structured field + dedicated status + clickable reference, matching the plan's `duplicate_of_id` + `BacklogStatusDuplicate` + UI link approach.

**Takeaway for this project**: the plan's design (dedicated status + plain link field validated at the app layer, single-hop forward reference, reachable from any active state) most closely mirrors **Linear's** model, which is the most structured/validated of the three — a reasonable target since this codebase already has synchronous validation infrastructure (`TransitionGuard`) that Jira/GitHub's looser conventions don't have to work within. Chain-prevention (recommended in §3) is stricter than all three (Jira/GitHub allow chains implicitly; Linear does not document explicit chain-blocking) — but is the simplest safe behavior given no AC guidance, and can be relaxed later if a real chain use case emerges.

## Key files referenced

- `session/backlog.go` — state machine, `validTransitions`, `TransitionGuard`, sentinel errors (lines 11–19, 118–225)
- `session/ent/schema/backlog_item.go` — fields/edges/indexes (no self-referential edge precedent anywhere in `session/ent/schema/*.go`)
- `session/ent/schema/item_source.go` — cross-type edge precedent (`edge.From(...).Ref(...).Unique()`)
- `session/ent_repository_backlog.go` — `ArchiveBacklogItem` (251–271, no precondition), `TransitionBacklogItemStatus` (274–319, precondition + audit event), `ListBacklogItems` exclusion (133–144)
- `session/repository.go` — `ErrPreconditionFailed`, `BacklogItemPrecondition` (lines 11–12, 302–303)
- `server/services/backlog_service.go` — `ArchiveBacklogItem` RPC handler (568–590), `UpdateBacklogItem` precondition wiring (541–550)
- `server/mcp/tools_backlog.go` — MCP tool handler pattern (`requestReview`, lines 235–274): item_id UUID validation, session-link permission check, sentinel error results; no existing MCP tool mutates backlog status directly today (mark_duplicate would be the first)
- `web-app/src/components/backlog/BacklogItemBadge.tsx` + `.css.ts`, `web-app/src/lib/backlog/status.ts` — status→class/label maps needing a `duplicate` entry
- `web-app/src/styles/theme-contract.css.ts` (statusBadge token slots, lines 93–113) + `web-app/src/styles/theme.css.ts` (6 theme implementations: light/dark/matrix/cyberpunk77/wh40k/clean)
- `web-app/src/components/backlog/BacklogItemCard.tsx` — `getActionSpec` per-status switch (archived case at lines 42–43)
- `web-app/src/app/backlog/page.tsx` — `ALL_STATUSES` array + default-filter-chip exclusion pattern for `archived` (lines 29–37, 87–88)
