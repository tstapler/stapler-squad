# Research: Known Pitfalls & Risks — backlog-session-lifecycle-ux

Date: 2026-08-01

## 1. Repo-standing rules that apply

All three flagged rules apply directly to this feature:

- **`.claude/rules/ent-schema-generation.md`** — applies. The new respawn-event table is
  a net-new ent schema (`session/ent/schema/*.go`). Must regenerate with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`,
  never the bare `ent generate ./session/ent/schema`. Omitting `--feature sql/upsert`
  compiles fine but silently breaks `UpsertRule`-style methods — a regression that
  wouldn't show up until an actual upsert call site needed it.
- **`.claude/rules/feature-registry.md`** — applies twice: once for the new RPC(s)
  reading the respawn-event table (`docs/registry/features/backend/<feature>.json`,
  `// +api:` marker in the handler), and once for each new/changed frontend surface
  (badges on `BacklogItemCard.tsx`/`SessionCard.tsx`, the new timeline component) —
  `docs/registry/features/frontend/<feature>.json` with a `// +feature:` marker. Run
  `make registry-generate` and confirm `coverage-gaps.json` doesn't grow net-new.
- **`.claude/rules/css-architecture.md`** — applies to every new/touched component
  style. In particular: the new respawn-event timeline needs a **named `zIndex` slot**
  if it renders any overlay/popover — no magic numbers. Any new badge colors (end-reason
  badge, pause-reason badge, stuck-remediation badge) must reference `vars.color.*`
  tokens from the theme contract, not hardcoded hex. New files must be `.css.ts`, not
  `.module.css`. If existing `.module.css` files are edited (e.g. `SessionCard.module.css`
  if the pause-reason badge is added there rather than as new vanilla-extract), only
  tokens already defined in `globals.css` may be referenced — check that file's current
  token list before introducing e.g. a "backing off"/"stuck" status color; it may not
  exist yet and would need to be added to `globals.css` first.

## 2. `end_reason` is *not* fully "already persisted" — it's mid-flight, uncommitted, backend-only

The requirements doc's baseline states `ItemSession.end_reason` "is persisted but
rendered nowhere in web-app." That's now true at the DB layer, but only because of an
**uncommitted, in-progress fix already sitting in this working tree** (BUG-053,
`docs/bugs/fixed/BUG-053-graceful-shutdown-kills-inflight-triage-treated-as-real-failure.md`,
marked FIXED 2026-08-01 but not yet committed to git — `git status` at the start of this
session shows `session/ent/schema/item_session.go`, `session/ent/*`,
`server/services/backlog_service_triage.go`, `session/storage.go`,
`session/storage_backlog.go` etc. all modified-but-unstaged).

Confirmed by reading the diff and the proto:
- `field.String("end_reason")` was just added to `session/ent/schema/item_session.go`,
  set only via the new `Storage.UpdateItemSessionEndedWithReason` (values:
  `"shutdown"`, `"timeout"`, `"process_error"`, `"claude_not_found"`, `"other"`, or `""`).
- **`end_reason` does not exist anywhere in `proto/session/v1/backlog.proto`'s
  `ItemSession` message** (lines 66–84 — 17 fields, no `end_reason`). It also isn't
  populated in any ent→proto conversion path yet.

So "surface `end_reason` in the UI" is not just a frontend wiring task — it requires,
as new work this project must add: (1) a new proto field on `ItemSession`, (2)
`make proto-gen`, (3) populating that field wherever `ItemSession` ent rows are
converted to proto (grep for the conversion helper before assuming it's a single call
site), *then* (4) the frontend badge. Two consequences for planning:

- Don't scope this as "read-only wiring of existing plumbing" for `end_reason`
  specifically — budget it like a small new-field addition, matching Feasibility Risk #1
  in requirements.md (which was right to flag this as unverified, not assumed).
- **Coordinate before touching `session/ent/schema/item_session.go` or
  `server/services/backlog_service_triage.go`.** Both are already mid-edit in this
  working tree for BUG-053. If this project's implementation phase runs in a fresh
  worktree/session, it will not see these uncommitted changes at all (a fresh git
  worktree checks out committed HEAD) — meaning it would either (a) reintroduce
  `end_reason` from scratch, likely producing a conflicting/duplicate schema field once
  BUG-053 is committed and merged, or (b) if run in *this* same working tree, silently
  build on top of someone else's uncommitted, not-yet-reviewed schema change. Confirm
  BUG-053 is committed (and ideally merged to main) before Phase 5 implementation
  branches off, so the schema field is a settled dependency, not a moving target.

`pause_reason` and `remediation_attempts`/`next_remediation_at` do not have this
problem — both are fully committed, stable, already on their respective proto messages.

## 3. `remediation_attempts`/`next_remediation_at` reach only `StuckBacklogItem`, not `BacklogItem` — the board card needs a new field + a new join

- `remediation_attempts` (field 12) and `next_remediation_at` (field 13) live on
  `StuckBacklogItem` (`proto/session/v1/backlog.proto` line 1022+), populated by
  `server/services/backlog_service_stuck.go` (`ListStuckBacklogItems`, the RPC backing
  `/unfinished`).
- The board's own `BacklogItem` message (line 113+) has **no such fields**, and
  `ListBacklogItems` (`server/services/backlog_service_query.go:107`) has no reference
  to "Stuck" or "remediation" anywhere in its body — it does not currently join
  `backlog_stuck_state` at all.
- To surface remediation attempts/context on `BacklogItemCard.tsx` (the board), this
  project must add fields to `BacklogItem` (or a nested sub-message) and add a join/
  lookup in `ListBacklogItems`. That handler runs on every board poll (the board's
  existing polling cadence — check its interval before assuming this is free); a naive
  per-item stuck-state lookup added there is an N+1 query risk if not batched. Given
  backlog size is "tens of items" (per requirements' Scalability note) this is unlikely
  to matter in practice, but the join should still be a single batched query
  (`WHERE item_id IN (...)`), not a per-row lookup in a loop, both for correctness under
  future growth and because it's the obviously-correct shape regardless.

## 4. Unbounded growth of the new respawn-event table

No existing precedent in this codebase caps rows per parent entity for an append-only
event log at the ent layer — `BacklogStatusEvent` (proto, `backlog.proto` ~line 87) is
the closest analog and is also unbounded, but status transitions are inherently rare
(a handful per item lifecycle). Respawn events are not: this instance sees 15+
restarts/day per the sibling `backlog-stuck-item-visibility` research, and every
orphan-recovery sweep after a restart can dispatch a fresh respawn per affected item
(BUG-053's own incident log shows 6 items respawned in a single sweep). Over a
long-lived, frequently-stuck item this can accumulate into the hundreds without a cap.

Recommendations for planning:
- Add a `LIMIT` (e.g. most-recent 50) to the read RPC's ent query by default, with the
  UI's "collapsed timeline" only ever rendering the capped set — never `SELECT *`
  unbounded per item.
- A hard per-item row cap (delete-oldest-beyond-N on insert, or a periodic prune) is
  optional for v1 given "tens of respawn events per item" is the requirements doc's own
  scale estimate, but the *read* path must be bounded regardless of whether the *write*
  path prunes, since an unbounded read is the actual failure mode (large RPC payload,
  slow render) even if row count stays modest in practice today.
- This does not need to block the fallback increment (badges) — it only affects the
  net-new respawn-event RPC/table, consistent with the Rabbit Holes section's warning
  against scope creep into analytics; a `LIMIT`+order-by-timestamp-desc is not
  aggregation, just a bounded read.

## 5. Concurrency at the 4 respawn call sites — pattern already exists, reuse it

`AutoRespawnReview`, `AutoRespawnAutonomousWork`, `AutoRespawnTriage`, and
`RemediateStaleWorkSession` (`server/services/backlog_service_triage.go`) don't use a
per-item mutex for backlog-item mutation — the codebase's actual pattern for TOCTOU
races at these call sites is a `sync.Map`-based in-flight guard
(`s.triageInFlight`/`s.spawnInFlight`, see the `TriggerTriage` diff already in this
working tree, section 3a-i) combined with ent's own row-level atomicity for the actual
writes (single `UpdateOneID`/`Create` calls, no explicit `ent.Tx` wrapping observed in
`backlog_service.go`/`backlog_service_triage.go`).

For the new respawn-event insert: each of the 4 call sites already runs its
existing-session-check → spawn-new-session sequence guarded (where applicable) by these
in-flight maps or by the reconcile loop's own cadence (`session/backlog_sync.go`,
`defaultSyncInterval`, ~60s per the task brief). Inserting one new `Create()` call for
the respawn-event row at each site, positioned *after* the respawn's new session is
successfully created (so the row records "this really happened," not "we attempted
this"), does not introduce new lock contention: it's a single independent INSERT with
no read-modify-write on shared state, so it can't race with the reconcile ticker or
with itself the way a check-then-write on `BacklogStuckState.remediation_attempts`
could. No new mutex is needed. The one thing to get right is *transaction boundary*: if
session creation and the respawn-event insert are meant to be atomically consistent
(never record an event for a session that failed to create, or vice versa), sequence
them as "create session, then on success write the event" rather than wrapping both in
an `ent.Tx` — this codebase has no existing precedent for wrapping cross-entity writes
in a shared transaction in this file, and introducing one here would be new pattern,
not a reuse of an existing one.

## 6. Frontend: progressive disclosure risk on `BacklogItemCard.tsx`

The sibling `backlog-item-detail-ux` project explicitly called out *consolidating*
duplicative status info as a goal. Adding new always-visible badges to the compact
board card for `end_reason`, `pause_reason`, and stuck/remediation state risks
regressing that goal in two concrete ways this project should guard against:

- **Redundancy with existing chips.** `BacklogItemCard.tsx` likely already renders a
  status chip (idea/in_progress/review/stuck/etc.) — confirm before adding a new
  "stuck: N attempts" badge whether the existing status chip already communicates
  "stuck" and this would just be adding attempt-count detail to it, vs. introducing a
  second, separate badge that says something overlapping ("stuck" chip + separate
  "backing off, 3 attempts" badge both visible at once reads as duplicated status, not
  new information).
- **Card layout budget.** The card is deliberately compact (board grid, many cards per
  screen, mobile width per the standing mobile/desktop UX requirement). Requirements.md
  already anticipates this with "default view shows a compact status/reason summary;
  full history/detail expands on demand" — the safe pattern is one compact indicator
  (e.g. a small icon + count, tooltip/expand for the reason text) rather than a new
  full-text badge per data point. Three new pieces of always-visible text
  (`end_reason` + `pause_reason` + remediation count) on an already-dense card is the
  concrete over-disclosure risk to watch for in the planning/UX phase — collapse them
  into a single compact "why is this stuck/paused" indicator on the card, reserving
  full text for the detail panel's `SessionsSection.tsx` (which already has room and
  the `Collapsible` primitive available).

`Collapsible`/`CollapsibleGroup` (`web-app/src/components/ui/Collapsible.tsx`) are
confirmed generic — already reused across `ProgressHistorySection.tsx`,
`LastReviewResultSection.tsx`, `NotesSection.tsx`, `ReviewingSection.tsx`, etc. in
`web-app/src/components/backlog/detail/`. No evidence they're list/timeline-specific;
they take arbitrary children, so a respawn-event timeline can reuse
`Collapsible` directly instead of building a parallel component — confirms the
Rabbit Holes section's open question can be resolved as "reuse, don't extend."

## 7. go-git / subshell rule

N/A — this feature is pure ent/ConnectRPC/React work; none of the 4 respawn call sites
or the new schema/RPC touch git operations (worktrees, branches, commits) at all. No
action needed against `.claude/rules/prefer-go-git-over-subshells.md`.
