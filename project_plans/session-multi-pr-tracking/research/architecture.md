# Architecture Research: session-multi-pr-tracking

Scope: data model and layering for a session that owns several PRs, and for a backlog item that spans several PRs.
Confidence labels: VERIFIED = file read this session; INFERRED = reasoned, not run.

## 0. Prior work this builds on (not re-derived)

- `project_plans/github-pr-status/research/architecture.md` - the `PRStatusPoller` design: one shared ticker, ETag cache, poller -> storage -> EventBus -> ConnectRPC. Its "one PR per session" shape is what we now widen.
- `project_plans/backlog-session-item-linking/research/architecture.md` - `AttachSessionToItem` inserts an `ItemSession` row with no unique constraint on `(session_uuid, item)`, so session<->item is already many-to-many. It is the precedent for a link table instead of a scalar field.
- `project_plans/pr-fix-steering/research/architecture.md` - keeps per-item dedup state in memory on `BacklogService`; and §5 argues an Event-Command-Policy table is not warranted for a small change (here it is, because the change spans three state machines).
- `project_plans/backlog-pr-pending-session-respawn` - NOT FOUND as `research/architecture.md` under that name (glob matched only the three above). Its behavior is covered below from code (`session/backlog_lifecycle_pr.go`).

## 1. Current state (VERIFIED)

Single PR on the session, in four stores:
- `Instance.GitHubPRNumber/GitHubPRURL` (`session/instance.go:344-347`), copied into `InstanceSnapshot.GitHub` (`session/instance_snapshot.go:42-43, 219-220`).
- Ent `Session` columns `github_pr_url`, `github_pr_number`, `github_pr_status_terminal`, plus state columns (`session/ent/schema/session.go:136-143`).
- Per-session poll state (`GitHubPRState/IsDraft/Priority/StatusTerminal`, `LastPRStatusCheck`) - also single-valued.
- Backlog: `BacklogItem.pr_url`, `pr_number` (`session/ent/schema/backlog_item.go:98-101`). Item-level scalar PR, independent of the session's.

Writers of the session PR (all overwrite; none appends) - this answers requirements Open Question 1:

| Writer | Where | Notes |
|---|---|---|
| Create from PR URL | `server/services/session_service_create.go:896-897` -> `instance.go:1222` | initial PR |
| Deferred GitHub resolution | `session/instance_actor_setters.go:948-949` (`setGitHubResolutionLocked`) | only if `PRNumber > 0` |
| `SetGitHubPR(url, n)` | `instance_actor_setters.go:274-286` | actor-routed |
| `SetGitHubPRNumber` | `instance_actor_setters.go:~295` | poller discovery by branch |
| Terminal push scan | `session/session_driver.go:961-962` (`scanAndLinkPRURL`) | **bypasses the actor**: takes `inst.mu` and calls `buildSnapshot` directly; pre-existing deviation from the setter pattern |
| Load from storage | `session/session.go:338-339`, `instance_serialization.go:344-345`, `ent_repository.go:1559-1560` | hydration |
| PR creation service | `server/services/pr_creation_service.go:166-169, 279-313` | reads, and emits `session_updated` for `github_pr_url/number` |

No flow records a second PR. A second push to the same branch is a no-op; a new branch/PR overwrites. The poller discovers by branch (`pr_status_poller.go:381-420`), so a rework PR on a new branch replaces the number silently.

Poller (VERIFIED, `session/pr_status_poller.go:371-456, 488-531`): `fetchAndUpdatePRStatus(ctx, inst)` reads `snap.GitHub.GitHubPRNumber`; if 0, discovers by branch (`GetPRForBranchConditional`, list-ETag keyed `owner/repo/branch`); else `GetPRInfoConditional` with `ETagCache`. `applyPRUpdate` writes `inst.UpdatePRStatus` then `storage.UpdateInstancePRStatus(title, ...)`. `noPRPollAfter` is keyed by session `Title`. Terminal (`GitHubPRStatusTerminal`) stops polling for the whole session - this is the "stale PR" mechanism: once PR A merges the session is terminal and PR B is never polled.

`session/pr_tracking.go` (VERIFIED): `RefreshPRInfo`, `GetPRComments`, `GetPRDiff`, `PostComment`, `SetCommitStatus`, `MergePR`, `ClosePR`, `GeneratePRContextPrompt` all start with `gh := i.GitHub()` and use `gh.PRNumber`. Every one is an implicit "current PR" action.

Backlog (VERIFIED by grep, `session/backlog_lifecycle_pr.go`, 2450 lines): `ReconcilePRPending` is gated on `FindPRPendingItems`' `PrNumberGT(0)` (line ~167); uses `item.PrNumber/PrURL` for `IsPRMerged`, `GetPRStatus`, head-branch verification (~1632-1702); `reconcilePRPendingWithoutPRItems` flags pr_pending with `pr_number=0` as `StuckReasonPRPendingNoPR`; `pushAndCreatePR` writes `PrNumber/PrURL` onto the item (~596, 734, 1356). Completion gate (`docs/reference/backlog-completion-gate-and-cleanup.md`) is AC-based (`unmetAcCriteriaError`), independent of PRs; PR merge -> `done` comes from `ReconcilePRPending` and `transitionBouncingItemToDone`.

## 2. Decisions

### D1. Session<->PR relation: new ent entity `SessionPR` (not JSON)

Recommend a dedicated ent entity. Rejected: JSON array on the session row.
- Needs cross-session queries (poller "open PRs due for poll", retention sweeper's PR-safety check, backlog gate "all PRs of this item terminal", search "which session owns PR X"). JSON forces load-all-and-filter.
- Per-PR state (state, draft, priority, CI, reviews, terminal flag, last-check, role) is naturally columns; the poller updates one PR without rewriting the session blob.
- Matches precedent: `ItemSession`, `BacklogStatusEvent`, `StageTransition` are all link/event entities in `session/ent/schema/`.
- Rejected `PreviousPRs []string`: leaves per-PR state unmodeled (requirements, Alternatives).

Fields (proposal): `id`; `host` (default `github.com`), `owner`, `repo`, `number`; `url`; `head_branch`; `base_branch`; `role` enum {`primary`,`followup`,`stacked`,`rework`,`external`}; `state` {open,closed,merged}; `is_draft`; `priority`; `check_conclusion`; `approved_count`; `changes_req_count`; `terminal` bool; `opened_at`, `merged_at/closed_at`, `last_checked_at`; `attached_at`; `attach_source` {created_from_url, push_scan, branch_discovery, pr_creation_service, jules, backfill, manual}.
Edges: `session` (M:1, by session id/uuid - confirm which key `ItemSession` uses; it uses `session_uuid`), optional `backlog_item` (M:1).

Bounded read (NFR "no N+1"): load PRs for a list page with a single `WHERE session_id IN (...)` query, grouped in memory; cap at 50 per session in the attach path (requirements say tens at most). INFERRED; confirm in the plan with the list RPC's existing eager-load path in `session/ent_repository.go`.

### D2. Identity key: `(host, owner, repo, number)`

- Unique index on those four columns **plus** `session_id`. The same PR may legitimately be attached to two sessions (e.g., a rework session and the original), so uniqueness is per session, not global. Global lookups use the non-unique four-column index.
- Introduce one value type `PRRef{Host, Owner, Repo string; Number int}` in `session/` (primitive-obsession checklist: the poller and `pr_tracking.go` currently pass `owner, repo string, prNumber int` as a pile, e.g. `InvalidateAndRefresh(ctx, owner, repo, prNumber)` at `pr_status_poller.go:151`). Normalize host (lowercase, empty -> `github.com`) in its constructor, reusing `github.NewRepoRefWithHost` semantics.
- Where owner/repo are not on the PR (today they come from the session's `GitHubOwner/GitHubRepo`), the PR row must carry its own: a stacked or follow-up PR can live in a fork or a different repo than the session's origin.

### D3. "Current / active PR": derived, with an explicit pin override

Recommend a **derived default** plus an optional **pinned** override, not purely explicit.
- Derivation (single pure function `ActivePR(prs []SessionPR, pinned *PRKey) *SessionPR`, unit-testable):
  1. pinned PR if set and still attached;
  2. else the open PR whose `head_branch` equals the session's current branch (`inst.CurrentBranch()`), because the agent is working on that branch now;
  3. else the most recently attached non-terminal PR;
  4. else the most recently attached PR (so merged-only sessions still show something and the legacy fields stay populated);
  5. else nil.
- Stacked case (two open): rule 2 wins; if the branch matches neither, rule 3 picks newest. The UI shows all PRs, and actions accept an explicit `PRRef` that overrides the default. This directly satisfies the metric "PR A merged, PR B open, actions hit B".
- Pin is stored on the session (one nullable field `pinned_pr_id`) and is set by user action only; automation never pins. Rationale: a purely explicit choice means every existing caller (`RefreshPRInfo` etc.) needs a decision from someone who has none; a purely derived one has no escape hatch for the stacked ambiguity. INFERRED; owner decision (Open Question 3) - flag in planning.
- `ActivePR` result also drives the **legacy mirror** (D5), so old clients and `GitHubPRNumber` readers see the active PR, not the oldest.

### D4. Layering

```
GitHub (github/)            PRRef, PRInfo, NewConditionalRequest        unchanged
  ^
session/pr_set (new)        SessionPR value type, ActivePR(), attach/transition rules   pure domain, no I/O
  ^
session/ent_repository_*    SessionPR entity CRUD, bulk load by session ids             persistence
  ^
session/Instance (actor)    holds an immutable []PRView copy inside InstanceSnapshot    lock-free read
  ^
PRStatusPoller              iterates PRs (not sessions); applies results via actor setter
pr_tracking.go              methods take PRRef (or nil = ActivePR)
backlog_lifecycle_pr.go     asks a PRSetReader for the item's PRs; never reads item.PrNumber directly
server/services             RPCs: list PRs, attach/detach, pin; proto carries repeated SessionPR + legacy scalars
```

New interface lives with its consumer (`session/`), per the repo's interface-pollution checklist: `PRSetReader{ PRsForItem(ctx, itemID) ([]PRSnapshot, error) }` used by the backlog lifecycle; `PRGateway` (existing `prGitHubClient`, `pr_status_poller.go:27`) stays the only GitHub seam.

### D5. Migration of legacy fields (dual-write, flag-gated)

- Ent auto-migrates the new table (`session/ent_repository_migrations.go` uses `funcMigration` list; add `"session_pr backfill"` next to `"github pr url backfill"` at line 81). Idempotent: for every Session with `github_pr_number > 0` and no `SessionPR` row for `(host,owner,repo,number)`, insert one with `role=primary`, state from `github_pr_state`, `terminal` from `github_pr_status_terminal`, `attach_source=backfill`. Pattern: `session/github_pr_url_backfill.go` (same shape; must run **after** it so URLs exist). Log count (requirements: observability).
- Backlog items with `pr_number > 0` get a `SessionPR` row linked to the item (and to its `ItemSession` work session if there is exactly one live link, else item-only; the `backlog_item` edge is nullable for that reason).
- Dual write during rollout: every attach/transition writes the relation **and** recomputes the legacy columns from `ActivePR`. With the flag off, readers use legacy columns only (relation written, not read) - exactly the Risk Control in requirements. Rollback needs no restore.
- Do not drop `github_pr_*` columns in this project; deprecate in a later one. Never commit generated ent output (CLAUDE.md).
- Rollout flag: live-settable via the feature-flag RPC, global + per-scope; name in planning (suggest `session_multi_pr`).

### D6. Instance actor / Snapshot interaction

Constraints from `.claude/rules/instance-lock-free-reads.md`: mutable `*Instance` fields are written under `i.mu.Lock()` in `instance_actor_setters.go` and republished via `i.snapshot.Store(...)`; readers use `Snapshot()`.

- Add `PRs []PRView` to `InstanceSnapshot` (and its `GitHub` group) as an **immutable slice copy** (always copy on write; never alias the instance's backing array). Add `ActivePR PRRef`-derived scalars so existing `snap.GitHub.GitHubPRNumber` readers keep working unchanged; they are populated from `ActivePR` (D5 legacy mirror). `buildSnapshot` is the single place that computes it.
- Writes go through one new setter, `Instance.UpsertPR(PRUpdate)` / `AttachPR`, implemented like `setGitHubPRURLLocked` (lock, mutate, `buildSnapshot`, `snapshot.Store`). The DB write (SessionPR row) is a separate step performed by the caller after the actor returns, matching how the poller does `inst.UpdatePRStatus` then `storage.UpdateInstancePRStatus`.
- The poller must stop reading mutable fields and use `Snapshot()` (it already does).
- Fix on the way: `session_driver.go:961-962` mutates `inst.GitHubPRURL/Number` directly instead of via the actor. Route it through `AttachPR` (the new writer must be the only one, else the relation and the legacy mirror diverge). Add to the existing `make actor-field-guard` ratchet that no code outside setters writes the `GitHubPR*` fields (INFERRED that this guard can be extended; confirm the guard's config in planning).
- `noarchivedrevival`: none of the new code starts/revives sessions; the poller path only reads and updates PR state. Respawn code for `pr_pending` (rework) already checks archival and stays untouched.

### D7. Poller and `pr_tracking.go` become PR-aware

Poller (`PRStatusPoller`):
- Unit of work changes from "session" to "(session, PR)". `checkAllSessions` (line 280) builds the work list from each snapshot's non-terminal PRs. Merged/closed PRs set `terminal=true` on the **PR row** and drop out - satisfying "merged and closed PRs are not re-polled".
- `GitHubPRStatusTerminal` on the session becomes derived: terminal iff **no non-terminal PR remains and branch discovery finds nothing new**. Retention sweeper's PR-safety check (`session_retention_sweeper.go`, reads the persisted flag) keeps working because the mirror is recomputed on every transition. Careful: a session whose PR A just merged must NOT go terminal-forever if it may open PR B; branch discovery (below) must keep a slow cadence for sessions whose newest PR is merged and whose branch is still alive.
- Discovery generalizes: `GetPRForBranchConditional` today finds *a* PR for the current branch and sets the number if it is 0. Change to: on each discovery tick (cadence = existing `NoPRBackoff` when the session has no open PR; longer when it has one), list PRs for the current branch; any PR not yet attached becomes `AttachPR(attach_source=branch_discovery)`. Rate-limit: one list call per session per tick, as today, with the same list-ETag (key `owner/repo/branch`), so cost grows only with sessions without an open PR on the current branch, not with PR count. INFERRED; verify call counts with the existing poller tests.
- Per-PR ETag already exists (`ETagCache` keyed by PR via `GetPRInfoConditional`), so open-PR polling cost is O(open PRs) at today's per-PR cost. `noPRPollAfter` stays keyed by session `Title` (it is a discovery backoff, not per-PR).
- `InvalidateAndRefresh(ctx, owner, repo, prNumber)` (line 151) -> take `PRRef`.

`pr_tracking.go`:
- Change each method to take a `PRRef` selector: `RefreshPRInfo(ctx, ref)` etc., plus thin wrappers using `ActivePR` for callers that do not choose. Do the extraction as a mechanical refactor first (see disposition), then add the explicit parameter. Callers: RPC handlers for comments/diff/merge/close, "Ask to fix" (`docs/how-to/use-up-next-pr-tabs-and-ask-to-fix.md`), `GeneratePRContextPrompt`. Each RPC gains an optional PR identifier; absent = active PR.
- `MergePR`/`ClosePR` are destructive: with an explicit `PRRef` they must verify the PR belongs to the session (attached row exists) before acting. This is the stale-PR safety property to test.

Jules (`session/jules_session_poller.go`, 4 refs) and `up-next-tabs-pr-nudge`/`server/pr_annotations.go`, `server/review_queue_manager.go`, `server/prwatch_sessions.go` read the scalar; they stay on the legacy mirror in phase 1 and migrate to the relation behind the flag. Jules multi-output: record each PR through `AttachPR(attach_source=jules)`; no extra design needed (answers the Jules open question, INFERRED from the 4 call sites listed by grep).

### D8. Backlog completion gate and `pr_pending` for multi-PR items

Observed: the gate is AC-based and PR-agnostic; `pr_pending` and its reconcilers are scalar-PR. Recommendation:

- **Item PR set** = union of `SessionPR` rows linked to the item (via `backlog_item` edge or via any `ItemSession` work session). `BacklogItem.pr_url/pr_number` become the **primary/active** PR mirror (same legacy-mirror idea as D5), so existing queries (`FindPRPendingItems` with `PrNumberGT(0)`, `reconcilePRPendingWithoutPRItems`) keep their meaning.
- **Completion rule (proposed, owner decision)**: item may go `pr_pending -> done` only when (a) at least one PR is merged and (b) every other attached PR is merged **or** closed, and (c) no attached PR is open. A closed-unmerged PR is acceptable only if some other PR is merged (otherwise the item's work never landed; fall to the existing superseded/closed handling in `closeIfSupersededByMain`). Rejected alternative "done when the primary PR merges": that is today's behavior and strands follow-ups; rejected "all PRs merged": a closed duplicate would block forever.
- `ReconcilePRPending` loops PRs of the item instead of reading `item.PrNumber`: `IsPRMerged`/`GetPRStatus` per open PR; head-branch verification (~1702) applies per PR using that PR's `head_branch`. An old merged PR no longer decides state (success metric 3). An item with an old PR merged and a new PR open **stays** `pr_pending` and tracks the open one.
- `reconcilePRPendingWithoutPRItems` condition changes from `pr_number == 0` to "no PRs in the set".
- The AC gate (`unmetAcCriteriaError`) is orthogonal; leave unchanged. Optional later: report PR-set status in `request_review` rejection notes.
- Re-entering `in_progress`/rework when a PR-fix respawn happens: new PR attaches with `role=rework`, set unaffected.
- Terminal cleanup (`CleanupTerminalItem`, `SessionRetentionSweeper`'s PR-terminal check) must use "all PRs terminal", otherwise a worktree is deleted while a follow-up PR is open. This is the highest-risk read site (data loss), so it gets a test before the flag flips.

## 3. Event-Command-Policy table

| Event | Command | Policy / invariant | Owner (module) |
|---|---|---|---|
| Session created from PR URL | `AttachPR(ref, role=primary, source=created_from_url)` | Idempotent on `(session, host, owner, repo, number)`; recompute mirror | `server/services/session_service_create.go` -> `Instance.AttachPR` |
| Agent pushes, terminal output contains PR URL | `AttachPR(ref, source=push_scan)` | Must go through actor; replaces `session_driver.go:961` direct write; first PR is `primary`, later ones `followup` | `session/session_driver.go` |
| Poller branch discovery finds unattached PR for session's branch | `AttachPR(ref, source=branch_discovery)` | Only PRs whose `head_branch` is the session's branch (or owned by same author); cap 50/session; log attach | `PRStatusPoller` |
| PR creation service opens PR | `AttachPR(ref, role per request, source=pr_creation_service)` | Records new PR even if the session already has one (no overwrite); emits `session_updated` | `server/services/pr_creation_service.go` |
| Jules produces an output PR | `AttachPR(ref, source=jules)` | Same relation, no special path | `session/jules_session_poller.go` |
| Poll tick | `PollPR(ref)` for each non-terminal PR | O(open PRs); 304 bumps `last_checked_at` only; rate-limit/auth handling unchanged | `PRStatusPoller` |
| PR observed merged/closed | `MarkPRTerminal(ref, state)` | Sets PR `terminal`; stops polling it; recompute `ActivePR` and mirror; update session terminal iff no non-terminal PR and discovery idle | `PRStatusPoller` + `Instance` actor |
| Active PR changes (new attach, terminal, pin) | `RecomputeMirror()` | Writes legacy `github_pr_*` from `ActivePR`; log "active PR changed" | `Instance` actor (`buildSnapshot`) |
| User pins/unpins a PR | `PinPR(ref \| nil)` | User action only; pin must reference an attached PR | `server/services` RPC |
| User/agent does merge/close/comment/diff/ask-to-fix | `Merge/Close/...(ref \| nil)` | `nil` -> `ActivePR`; explicit ref must be attached to this session else `FailedPrecondition` | `session/pr_tracking.go` |
| Item reaches `pr_pending` / reconcile tick | `ReconcileItemPRs(item)` | Per-PR merged check; done only under the D8 rule; open PR keeps item `pr_pending` | `session/backlog_lifecycle_pr.go` |
| Item has `pr_pending` with empty PR set | `MarkStuck(PRPendingNoPR)` | Same stuck reason, condition = empty set not `pr_number==0` | `reconcilePRPendingWithoutPRItems` |
| Item terminal (done/archived) | `CleanupTerminalItem` | Proceeds only if all attached PRs terminal (retention sweeper check too) | `server/services/backlog_service.go`, `session_retention_sweeper.go` |
| Process start | `BackfillSessionPRs` | Idempotent; runs after URL backfill; logs backfilled count | `ent_repository_migrations.go` |

## 4. Disposition for hot areas

| Area | Evidence | Disposition | Why |
|---|---|---|---|
| `session/backlog_lifecycle_pr.go` (2450 lines, ~40 `PrNumber/PrURL` reads) | line count; grep above | **Isolate via seam** | Add `PRSetReader` + an `itemPRs(item)` helper that returns the set with legacy fallback; change reconcilers to call it. Do not restructure the file in this project. Tests pin current single-PR behavior first. |
| `session/pr_status_poller.go` (531 lines) | `fetchAndUpdatePRStatus` interleaves discovery, fetch, persist | **Refactor-first (small)** | Extract `discoverPRsForBranch` and `pollOne(ref)` from `fetchAndUpdatePRStatus`, keeping behavior identical and tests green, then change the loop's unit of work. Churn hotspot with auth/backoff/ETag state; widening it in place would blur those. |
| `session/pr_tracking.go` (234 lines) | all methods repeat `GitHub()`/ref/number prelude | **Refactor-first (trivial)** | Extract `resolvePR(ref \| nil) (PRRef, error)` once, then add the parameter. |
| `session/instance_actor_setters.go`, `instance_snapshot.go` | 13 / 6 refs; rule file | **Extend as-is** | Add one setter and snapshot field following the existing pattern. Route `session_driver.go:961` through it. |
| Ent schema / `ent_repository_migrations.go` | `funcMigration` list | **Extend as-is** | New entity and one backfill step, same pattern as `github_pr_url_backfill.go`. |
| `session_driver.go` `scanAndLinkPRURL` | direct field write | **Extend as-is** (fix one call) | Swap direct write for `AttachPR`. |
| Server consumers (`pr_annotations.go`, `review_queue_manager.go`, `prwatch_sessions.go`, `unfinished_work_service.go`, `approval_service.go`, `instance_adapter.go`) | 1-7 refs each | **Extend as-is** | Keep reading the legacy mirror in phase 1; migrate individually behind the flag. Missing one is the stale-PR risk, so add a grep-based test/lint ratchet listing allowed readers (INFERRED: pattern of `make actor-field-guard`). |

## 5. Risks and open points for planning

1. Session key for the edge: `ItemSession` uses `session_uuid`; the poller keys by `Title`. Pick `session_uuid` (INFERRED; Title is not unique-safe across renames) and confirm in plan.
2. Terminal-forever semantics for sessions after first PR merges (D7) is the most likely regression: a session that merged PR A and later opens PR B must resume polling. Needs an explicit test.
3. Retention sweeper and `CleanupTerminalItem` PR-terminal checks (D8) are the data-loss sites; ship their tests before flipping the flag.
4. Owner decisions: D3 pin semantics, D8 completion rule, flag name.
5. Proto: add `repeated SessionPR prs` and an optional `pr` selector on PR RPCs; keep scalar fields populated for the mobile app. New RPCs need `make proto-gen` and `make registry-generate`.
6. Not verified: runtime behavior (nothing was built or run); counts above come from grep and file reads.
