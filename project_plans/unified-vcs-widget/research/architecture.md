# Architecture Research: unified-vcs-widget

Research Agent 3 (Architecture) — SDD Phase 2, project `unified-vcs-widget`.

Prior audits checked for overlap: `project_plans/backlog-cross-platform-audit/research/implementation-inventory.md`
and `project_plans/frontend-architecture-audit/research/{component-coupling,state-management}.md` were skimmed —
neither documents the three VCS surfaces' data-flow or the backend persistence gap identified below. This is new
ground; no prior analysis to reconcile with.

## 1. Data Flow Today

| Surface | Component | Hook/Context | RPC | Go Handler | Data Source | Live only? |
|---|---|---|---|---|---|---|
| Session detail VCS tab | `VcsPanel.tsx` | `useSessionVcsContext()` → `useSessionVcs.ts` (`web-app/src/lib/hooks/useSessionVcs.ts`) | `SessionService.GetVCSStatus`, `SessionService.GetSessionDiff` | `server/services/session_service.go` (`GetVCSStatus`) | Live git read against the session's worktree (`findInstanceFast` → in-memory `Instance`); GitHub PR/CI fields (`session.github*`) come from the `Session` object itself, populated by `PRStatusPoller` | **Yes** — requires a live in-memory `Instance`; returns nothing once worktree is torn down |
| Backlog item detail → "Version Control" | `BacklogItemDetail.tsx` (~1244–1279) | `useVcsStatus(sessionId)` (live) falling back to `useBacklogItemShipStatus(itemId)` (`web-app/src/lib/hooks/useBacklogItemShipStatus.ts`) | `SessionService.GetVCSStatus` (live path) / `BacklogService.GetBacklogItemShipStatus` (historical path) | `session_service.go` `GetVCSStatus` / `server/services/backlog_service_ship_status.go` `GetBacklogItemShipStatus` | Live path: same as VcsPanel. Historical path: **computed on-demand from git history at RPC-call time** — `git.IsCommitOnMain`, `git.BranchAheadBehind`, `git.ListShippedCommits` run against `item.RepoPath` using the last work session's `LastCommitSha`/`BaseCommitSHA` (durable ent fields on `ItemSession`), no DB-cached VCS data at all | Live path yes; historical path works after worktree cleanup but **has no GitHub PR/CI/review data** — those live only on the (possibly gone) `Instance`, not in this computation |
| Unfinished item detail | `UnfinishedItemDetail.tsx` | `useUnfinishedWork.ts` (`web-app/src/lib/hooks/useUnfinishedWork.ts`) | `UnfinishedWorkService.WatchUnfinishedWork` (server-streaming) + `ScanUnfinishedWork` (trigger) | `server/services/unfinished_work_service.go` | Periodic filesystem/git scan of watched directories, enriched with GitHub PR fields (`github_pr_number/url/state/priority`) "populated from session PR state when sessions cover this worktree" (types.proto:1354) — i.e. joined from live `Instance` PR state when a matching session exists, not durably stored per-worktree | Live scan; GitHub enrichment is a live join, same PRStatusPoller-dependent gap as the other two |

Key structural fact: **all three surfaces ultimately either read a live worktree/`Instance`, or (in the one durable
path, `GetBacklogItemShipStatus`) recompute git-log facts on every call** — there is no cached/durable table of VCS
status. `useBacklogItemShipStatus` is explicitly documented as "No polling: shipped status doesn't change on its own
once a session has ended" (comment in the hook), which is true for the git-history part but is exactly the
assumption that breaks for GitHub PR/CI/review state, which *can* change after done (see §5).

## 2. Backend Persistence Design

### How `BacklogItemShipStatus` / `ShippedCommit` are populated today

Not stored anywhere — `GetBacklogItemShipStatus` (`server/services/backlog_service_ship_status.go`) is a pure
read-only, compute-on-call RPC:

1. `s.storage.ListItemSessions(ctx, itemId)` → find the most recent `SessionRoleWork` session (durable, ent-backed).
2. `git.IsCommitOnMain(item.RepoPath, mainBranch, lastWork.LastCommitSha)` — walks git history live.
3. `git.BranchAheadBehind(...)` and `git.ListShippedCommits(item.RepoPath, wt.BaseCommitSHA, lastCommitSha)` — also
   live git operations against `item.RepoPath` (the *original* repo clone, not the deleted worktree — this is why it
   survives worktree cleanup: it only needs `repo_path` + two SHAs, both durable).

This is why CI/PR/review-count/per-file-diff-stats are absent: those require either a GitHub API call (PR/CI/review
state) or a worktree-relative diff computation (per-file stats) — neither is derivable from `repo_path` + a commit
SHA alone the way the git-log-based fields are.

### Where GitHub PR/CI state actually lives today — and the gap

`session/ent/schema/session.go` (`Session` ent schema) persists only `github_pr_url`, `github_pr_number`,
`github_owner`, `github_repo` (lines ~114–127). It does **not** have columns for `github_check_conclusion`,
`github_approved_count`, `github_changes_req_count`, `github_pr_is_draft`, or `github_pr_state` — all of which exist
on the `Session` **proto** message (`types.proto` lines 109–141) and are populated in-memory only.

The persistence hook makes this explicit — `session/storage.go:524-529`:

```go
// UpdateInstancePRStatus updates the PR status fields for a specific instance.
// PR fields are not stored in the ent schema — they live in memory and are re-populated by
// PRStatusPoller on each poll cycle. No DB write is needed.
func (s *Storage) UpdateInstancePRStatus(_, _, _, _ string, _, _ int, _, _ bool) error {
	return nil
}
```

All six parameters are discarded (`_`). This is the root cause named in the requirements' Feasibility Risks section:
CI conclusion, approved count, and changes-requested count are **never durably persisted**, anywhere, for any
session — they exist only in the in-memory `Instance` struct and are reconstructed by `PRStatusPoller` polling the
GitHub API on a timer. Once the process restarts or the `Instance` is gone (worktree cleaned up, session archived),
this data is unrecoverable without a fresh GitHub API call — which itself requires knowing the PR number, which *is*
persisted (`github_pr_number`).

Per-file diff stats: no durable equivalent exists at all. `git.ListShippedCommits` returns commit metadata only
(SHA, summary, author, date) — no file list, no additions/deletions. `FileChange` (types.proto:849, additions/
deletions/status/path) is only ever populated by the **live** `GetVCSStatus` path against a real worktree
(`git.GitWorktree.Diff()` — `session/git/diff.go`). There is no `git` helper that computes a per-file diff between
two arbitrary SHAs in a repo without a worktree (the closest analog, `ListShippedCommits(repoPath, baseSHA,
headSHA)`, only walks `git log`, not `git diff --numstat`).

### Minimal backend change

Two additions, both additive (no breaking changes to existing RPCs):

**A. New ent fields on `BacklogItem` (not `Session`)** — snapshot fields belong on the backlog item because that's
the durable entity that outlives sessions/worktrees, and `BacklogItemShipStatus` is already keyed off
`item.RepoPath` + `item.PrURL`. Add to `session/ent/schema/backlog_item.go`:

```go
field.String("shipped_check_conclusion").Optional(),
field.Int("shipped_approved_count").Optional().Default(0),
field.Int("shipped_changes_req_count").Optional().Default(0),
field.Time("shipped_snapshot_at").Optional().Nillable(),
field.String("shipped_file_stats").Optional().
    Comment("JSON []FileChangeStat{Path,Status,Additions,Deletions} — per-file diff stats captured at ship time, same encoding pattern as session_artifacts"),
```

Follow `.claude/rules/ent-schema-generation.md`: regenerate with
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.

Storing `shipped_file_stats` as a JSON blob column (mirroring the existing `session_artifacts` JSON-blob pattern on
`Session`, `session/ent/schema/session.go` line ~131) avoids a new child table/edge for what is a bounded,
write-once-then-rarely-updated list — consistent with `.claude/rules/interface-pollution-checklist.md`'s smell #6
(don't add a layer/table that doesn't earn its place). A `ShippedFileStat` repeated message can still be exposed on
the proto (`BacklogItemShipStatus.file_stats`) even though the Go/DB side is a JSON string, same as
`AcceptanceCriteria` already does (`acceptance_criteria` column is `JSON []AcCriterion`).

**B. New git helper for per-file diff stats between two SHAs**, alongside `ListShippedCommits` in
`session/git/ops.go`:

```go
// FileStatsBetween returns per-file addition/deletion counts for base..head,
// via `git diff --numstat base head` — no worktree required (works purely off
// repoPath's object store), mirroring ListShippedCommits' base..head contract.
func FileStatsBetween(repoPath, baseSHA, headSHA string) ([]FileStat, error)
```

**C. Snapshot capture point** — extend `ReconcilePRPending` (`session/backlog_lifecycle.go:1666-1702`, the existing
reconciler — see §3) at the exact moment it detects `merged == true` and transitions the item to `done`. This is the
natural, already-scheduled tick where the last live `prStatus` (`*git.PRStatus`, from `g.GetPRStatus(item.PrNumber)`
— already fetched at line ~1706, has `ApprovedCount`, `ChangesRequestedCount`, and can be extended to surface a CI
conclusion string) is available, and it's the same "capture a snapshot at ship time" pattern the requirements
doc points to (ShipStatusDisplay's commit list). Also call the new `git.FileStatsBetween(item.RepoPath,
wt.BaseCommitSHA, lastWork.LastCommitSha)` here and persist both into the new `BacklogItem` fields via a
`BacklogItemUpdate` (the same update-struct pattern already used at `backlog_lifecycle.go:1722-1725` for clearing PR
fields on close).

This means **no new poller/webhook is needed for the capture itself** — `ReconcilePRPending` already runs on a
schedule against every `pr_pending` item and already has the richest PR status available (`git.PRStatus`) at exactly
the transition-to-done boundary. Webhook-driven capture (mentioned as an open question) would only be worth adding
later to reduce latency between "PR merged on GitHub" and "reconciler tick notices" — not required for correctness,
since the poll interval already governs today's `done` transition itself.

## 3. Integration Points

- **`server/services/session_service.go`** — `github_check_conclusion` etc. are *read* (rendered into the `Session`
  proto) but not *set* here; they're set on the in-memory `Instance` via `inst.UpdatePRStatus(...)`
  (`session/pr_status_poller.go:396`) and read back out when the `Session` proto is serialized
  (`session/instance_serialization.go` / snapshot). `session_service.go` itself has no PR-polling logic — it's a
  thin proto-mapping layer over `Instance`/`Storage`.
- **`session/pr_status_poller.go`** (`PRStatusPoller`) — the *session-level* GitHub reconciler. Polls every live
  `Instance` with a PR, calls `github.GetPRInfoConditional`/`GetPRForBranchConditional` (ETag-cached), and applies
  results via `applyPRUpdate` → `inst.UpdatePRStatus(...)` (in-memory) + `storage.UpdateInstancePRStatus(...)`
  (**no-op**, see §2). This is the poller responsible for `githubCheckConclusion`/`githubApprovedCount`/
  `githubChangesReqCount` on `Session` — and it is *not* the backlog-item-level reconciler.
- **`session/backlog_lifecycle.go` `ReconcilePRPending`** (line 1666) — the *backlog-item-level* reconciler ("polls
  items in pr_pending status... transitions to done when merged"). This is the mechanism named generically in
  `.claude/rules/sdd-planning-artifacts-commit.md`'s "reconciler patterns" reference and is the correct extension
  point for durable snapshot capture (§2C) — it already owns the done-transition and already fetches
  `git.PRStatus` fresh from GitHub at that moment, independent of whether any `Instance`/worktree still exists.
- **`server/services/backlog_service_ship_status.go`** — the RPC to extend so `BacklogItemShipStatus` returns the
  new snapshot fields when the durable item fields are populated, falling back to the current live-computed fields
  when not (e.g. items shipped before this feature ships, or items that went `done` via the `pushAndCreatePR`
  direct-fallback path at line 1437, which has no PR at all — `shipped_via = "direct"`, no snapshot needed since
  there's no PR/CI to capture).

## 4. Frontend Component Architecture Proposal

Single shared component, `VcsWidget` (proposed location: `web-app/src/components/shared/VcsWidget.tsx`, alongside
existing `VcsStatusDisplay.tsx` which it can compose/replace), taking one normalized prop shape and a `mode` flag:

```tsx
// web-app/src/lib/vcs/types.ts — the ONE normalized shape all three sources adapt into.
// Deliberately flat, no optional-field explosion beyond what's genuinely conditional
// per data source (interface-pollution-checklist smell #1/#6: no speculative nesting).
export interface VcsWidgetData {
  branch: string;
  isClean: boolean;
  fileChanges: FileChangeSummary[];   // unified shape for staged/unstaged/untracked/conflict — see below
  aheadOfMain: number;
  behindMain: number;
  branchExists: boolean;              // false = branch deleted (merged/closed)
  commits: CommitSummary[];           // newest first; empty when only a single HEAD commit is known
  github: GithubSummary | null;       // null when no PR ever existed for this item/session
  isLive: boolean;                    // true = data can change without a page refresh (live worktree);
                                       // false = historical snapshot (drives "as of <time>" copy, no refresh button)
  snapshotAt: Date | null;            // set only when isLive is false and a captured-at timestamp exists
}

export interface FileChangeSummary {
  path: string;
  oldPath?: string;
  status: "modified" | "added" | "deleted" | "renamed" | "copied" | "untracked" | "ignored" | "conflict";
  additions: number;
  deletions: number;
  section: "conflict" | "staged" | "unstaged" | "untracked"; // VcsPanel's 4 buckets; compact mode ignores this
}

export interface CommitSummary {
  sha: string;
  summary: string;
  authorName?: string;
  authoredAt?: Date;
}

export interface GithubSummary {
  owner: string;
  repo: string;
  prUrl: string;
  prNumber: number;
  prState: string;       // "open" | "closed" | "merged" | ""
  isDraft: boolean;
  checkConclusion: string;
  approvedCount: number;
  changesReqCount: number;
}
```

**One normalization function per source** (per interface-pollution-checklist — concrete functions, no adapter
class/interface layer):

```ts
// web-app/src/lib/vcs/adapters.ts
export function fromSessionVcs(status: VCSStatus, session?: Session): VcsWidgetData { ... }
export function fromShipStatus(status: BacklogItemShipStatus): VcsWidgetData { ... }
export function fromUnfinishedWorktree(wt: UnfinishedWorktree): VcsWidgetData { ... }
```

Each is a plain function, not a class, not behind an interface (no second implementation is ever needed per source —
smell #1). They live in the *consumer's* directory (`web-app/src/lib/vcs/`), not next to the proto types, per the
"interface near the consumer" idiom this repo already applies to Go.

**Component API**:

```tsx
interface VcsWidgetProps {
  data: VcsWidgetData;
  mode: "full" | "compact";           // full = VcsPanel-equivalent (all file lists, GitHub row, commit list);
                                       // compact = UnfinishedItemDetail-equivalent (stats line + commit list only)
  onNavigateToFile?: (path: string) => void; // full mode only; when absent, file paths render as plain text
  onViewDiff?: () => void;            // renders a "View Diff" button (ReviewChangesModal / WorktreeDiffModal caller's choice)
  onRefresh?: () => void;             // omitted entirely when data.isLive is false — no refresh button on a snapshot
}
```

`mode` answers open question 2 (UnfinishedItemDetail becomes `mode="compact"` of the shared component rather than
staying fully distinct — its stats-row + commit-list + action-buttons shape maps directly onto
`VcsWidgetData.fileChanges`/`commits`, with the action buttons (Commit & Push, Summarize) staying in
`UnfinishedItemDetail` as surrounding chrome, not inside `VcsWidget` itself — `VcsWidget` only owns the
read-only display).

`onNavigateToFile` being optional (rather than a `hasFilesTab: boolean` mode variant) answers open question 3 as a
pragmatic default: pass the callback where a Files tab exists (session context), omit it where it doesn't (Backlog
detail) — `VcsWidget` degrades to plain (non-clickable) filenames automatically, exactly like `VcsPanel.tsx`'s
existing conditional (`onNavigateToFile ? styles.filePathClickable : ""` at line 85) already does today. No inline
expandable diff is needed inside the widget itself for v1 — `onViewDiff` already covers "open a diff somewhere else"
for both Backlog (`ReviewChangesModal`) and Unfinished (`WorktreeDiffModal`); building a *third*, inline diff
renderer duplicates two already-existing modal diff viewers for no clear win, so it's out of scope unless a later
UX pass specifically asks for it.

## 5. Consistency Requirements

**`done` is not fully terminal for review/CI activity** — the ent/lifecycle code shows two escape hatches:

- `AutoReopenForPRFix` (`session/backlog_lifecycle.go`) can move an item **out of** `pr_pending`/back to
  `in_progress` when a PR is closed without merging or CI fails/reviewers block — this happens *before* `done`, so
  it doesn't threaten a captured snapshot's validity, it just means the snapshot capture point (merge detection) is
  correctly gated behind those checks already.
- However, nothing in this codebase re-opens an item **after** it reaches `done`. `ReconcilePRPending` only ever
  polls items still in `pr_pending` (`FindPRPendingItems`) — once transitioned to `done`, the item drops out of that
  query entirely, so no further reconciliation happens automatically. Practically: a GitHub PR can still receive a
  late review comment or a re-run CI check *after* merge (rare but real — e.g. a reviewer leaves a post-merge
  comment, or someone re-runs a required check retroactively for audit purposes), and the durable snapshot would go
  stale with no code path to refresh it.

**Recommendation**: treat the snapshot as "as of merge time" by design (surfaced via `snapshotAt` in the UI, per
§4) rather than trying to keep it live-updated post-done — this matches how `ShipStatusDisplay` and
`useBacklogItemShipStatus` already frame historical data ("No polling: shipped status doesn't change on its own once
a session has ended" — true for the git-log parts, and by this same reasoning acceptable to extend to the PR
snapshot: once merged, the *code* is fixed, and stale review/CI metadata is a much lower-stakes staleness than stale
shipped-code data would be). If real-time accuracy of post-merge CI/review state is later required, it is
explicitly out of scope per requirements ("no regression... real-time push... out of scope") — a manual `refetch()`
already exists on `useBacklogItemShipStatus` and could be pointed at a new snapshot-refresh RPC if ever needed,
without any schema change.

## Event-Command-Policy Table

| Domain Event | Policy Trigger | Command | Actor/System |
|---|---|---|---|
| `WorkSessionCommitted` | Work session pushes a commit | `RecordLastCommit(itemId, sessionUUID, sha)` | Work session (via `Storage.UpdateItemSessionCommit`-style call) |
| `ReviewPassed` | Review gate verdict = PASS | `PushAndCreatePR(itemId)` or `TransitionBacklogItemStatus(item, done)` if nothing to ship | `BacklogLifecycleListener.pushAndCreatePR` |
| `PRCreated` | `pushAndCreatePR` succeeds in creating a GitHub PR | `TransitionBacklogItemStatus(item, pr_pending)` + persist `pr_url`/`pr_number` | `BacklogLifecycleListener` |
| `PRPendingTickDue` | Scheduler interval elapses for an item in `pr_pending` | `ReconcilePRPending(item)` — fetch `IsPRMerged` + `GetPRStatus` | `BacklogLifecycleListener.ReconcilePRPending` (existing reconciler) |
| `PRMergedDetected` | `ReconcilePRPending` sees `IsPRMerged == true` | **`CaptureShipSnapshot(itemId, prStatus, fileStats)`** *(new)* then `TransitionBacklogItemStatus(item, done)` | `BacklogLifecycleListener.ReconcilePRPending` (extended — §2C) |
| `PRClosedWithoutMerge` | `ReconcilePRPending` sees `IsClosed == true` | Clear `pr_url`/`pr_number`, `AutoReopenForPRFix(itemId)` | `BacklogLifecycleListener.ReconcilePRPending` |
| `PRCIFailingOrBlocked` | `ReconcilePRPending` sees `CIFailing`/`HasBlockingReviews`/`HasConflicts` | `AutoReopenForPRFix(itemId, fixContext)` | `BacklogLifecycleListener.ReconcilePRPending` |
| `SessionPRPollTickDue` | Scheduler interval elapses for a live `Instance` with a PR | `fetchAndUpdatePRStatus(inst)` | `PRStatusPoller` (session-level, distinct from the item-level reconciler above) |
| `SessionPRStatusChanged` | `PRStatusPoller` gets a non-304 response | `inst.UpdatePRStatus(...)` (in-memory only — `storage.UpdateInstancePRStatus` is a no-op, §2) | `PRStatusPoller.applyPRUpdate` |
| `BacklogDetailOpened` | User opens a backlog item's detail view | `GetVCSStatus(sessionId)` (live, may fail) then `GetBacklogItemShipStatus(itemId)` (historical fallback) | `BacklogItemDetail.tsx` |
| `ShipSnapshotRequested` *(new)* | `GetBacklogItemShipStatus` RPC call, item has durable snapshot fields set | Read `shipped_check_conclusion`/`shipped_approved_count`/`shipped_changes_req_count`/`shipped_file_stats` off `BacklogItem`, no live git/GitHub call | `BacklogService.GetBacklogItemShipStatus` (extended — §2) |

## Summary of Concrete Changes Implied

1. `session/ent/schema/backlog_item.go` — 5 new optional fields (§2A) + `go generate` with `--feature sql/upsert`.
2. `session/git/ops.go` — new `FileStatsBetween(repoPath, baseSHA, headSHA)` helper (§2B).
3. `session/backlog_lifecycle.go` `ReconcilePRPending` — call snapshot capture immediately before the existing
   `done` transition at line 1693 (§2C).
4. `proto/session/v1/backlog.proto` — extend `BacklogItemShipStatus` with `shipped_check_conclusion`,
   `shipped_approved_count`, `shipped_changes_req_count`, `repeated ShippedFileStat file_stats`, `snapshot_at`; run
   `make proto-gen`.
5. `server/services/backlog_service_ship_status.go` — populate the new response fields from the durable `BacklogItem`
   columns.
6. `web-app/src/lib/vcs/types.ts` + `adapters.ts` (new) — `VcsWidgetData` shape + 3 adapter functions (§4).
7. `web-app/src/components/shared/VcsWidget.tsx` (new) — the shared component, `mode: "full" | "compact"` (§4).
8. Wire into `VcsPanel.tsx` (full mode, `onNavigateToFile` wired to Files tab), `BacklogItemDetail.tsx` (full mode,
   no `onNavigateToFile`, `onViewDiff` → `ReviewChangesModal`), and evaluate `UnfinishedItemDetail.tsx` (compact
   mode) per the requirements' "if it fits" framing.
