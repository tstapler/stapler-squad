# Stack Research: unified-vcs-widget

## 1. Frontend data-fetching patterns (three incompatible contracts today)

No React Query/SWR anywhere in this repo — all data fetching is hand-rolled `useState`/`useEffect`/`useCallback` hooks calling ConnectRPC clients directly (`createClient(Service, transport)`). Three distinct patterns exist for VCS-ish data, and they differ in *shape*, not just plumbing:

| Surface | Hook | Fetch trigger | Data source | Notes |
|---|---|---|---|---|
| `VcsPanel` | `useSessionVcs` (`web-app/src/lib/hooks/useSessionVcs.ts`, 147 lines) via `SessionVcsContext` (`web-app/src/lib/contexts/SessionVcsContext.tsx`) | Redux-store-driven: re-fetches when the session's `status`/`updatedAt` changes (via `useAppSelector`/`selectAllSessions`), plus a 60s fallback poll (paused when `document.hidden`) | `SessionService.getVCSStatus({id})` → `VCSStatus`; `SessionService.getSessionDiff({id})` → diff stats | Instantiated **once per SessionDetail** via a Provider so `VcsPanel`, `FilesTab`, `DiffViewer` share one fetch (this is the shared-cache pattern already in the repo — closest existing template for a unifying hook). **Crucially, GitHub PR/CI fields (`githubOwner`, `githubPrUrl`, `githubApprovedCount`, `githubChangesReqCount`, `githubCheckConclusion`) are NOT fetched by this hook at all** — `VcsPanel.tsx` reads them directly off the `session` prop (passed down from the Redux session list, itself populated by `watchSessions` push updates from `PRStatusPoller`). |
| Backlog item detail | `useBacklogItemShipStatus` (`web-app/src/lib/hooks/useBacklogItemShipStatus.ts`, 52 lines) | One-shot fetch on mount + manual `refetch()`; explicitly **no polling** ("shipped status doesn't change on its own once a session has ended") | `BacklogService.getBacklogItemShipStatus({itemId})` → `BacklogItemShipStatus` | Computed live from git history at read time (see §4) — not from a session/session-list at all, so it has no path to the live GitHub review/CI fields. |
| `UnfinishedItemDetail` | `useUnfinishedWork` (`web-app/src/lib/hooks/useUnfinishedWork.ts`, 135 lines) | Server-push streaming RPC, not polling | `UnfinishedWorkService.watchUnfinishedWork()` (bidi/server stream) → `Map<key, UnfinishedWorktree>`, plus `scanUnfinishedWork()` to trigger a rescan | Maintains a live in-memory map keyed by `repoPath|branch`, reconnects with a 3s backoff on stream error. Already carries a *subset* of GitHub PR fields (`githubPrNumber/Url/State/Priority`) natively on `UnfinishedWorktree`, but no CI conclusion or review counts. |

**Compatibility verdict**: these three cannot share a single fetch mechanism as-is (poll-driven-by-Redux vs. one-shot vs. streaming), but they *can* share a single **data-shape contract** if the proto is unified (see §2) — i.e., build one `useVcsWidgetData(props)`-style hook per surface that normalizes into a common `VcsWidgetProps` shape, rather than trying to force one fetch strategy on all three. The `SessionVcsProvider` context-sharing pattern is the right template to imitate for avoiding duplicate fetches within a single widget instance.

## 2. Proto/ent types involved

All in `proto/session/v1/`:

- **`VCSStatus`** (`types.proto:870`) — `type`, `branch`, `head_commit`, `description`, `ahead_by`/`behind_by`, `upstream`, `has_staged/unstaged/untracked/conflicts`, `is_clean`, and 4 `repeated FileChange` lists (`staged_files`, `unstaged_files`, `untracked_files`, `conflict_files`).
- **`FileChange`** (`types.proto:849`) — `path`, `status` (`FileStatus` enum), `is_staged`, `old_path`, `additions` (int32, from `git diff --numstat`), `deletions` (int32). This is the per-file diff-stat granularity `ShipStatusDisplay` currently lacks.
- **`Session`** (`types.proto`, fields 25–39) — the richest live GitHub state: `github_pr_number`, `github_pr_url`, `github_owner`, `github_repo`, `github_source_ref`, `github_pr_state`, `github_pr_is_draft`, `github_pr_priority`, `github_approved_count`, `github_changes_req_count`, `github_check_conclusion`, `last_pr_status_check` (timestamp). Populated by `PRStatusPoller` (see §3) — **lives only on the in-memory `Session`/`Instance`, no durable table**.
- **`BacklogItemShipStatus`** (`backlog.proto:210`) — `shipped`, `shipped_via` ("pr"/"direct"/""), `pr_url`, `branch_name`, `branch_exists`, `ahead_of_main`, `behind_main`, `last_commit_sha`, `last_commit_message`, `last_commit_at`, `error`, `repeated ShippedCommit commits`. **No GitHub CI/review fields, no per-file diff stats** — this is the gap the requirements call out.
- **`ShippedCommit`** (`backlog.proto:238`) — `sha`, `summary`, `author_name`, `authored_at`.
- **`UnfinishedWorktree`** (`types.proto:1319`) — composite key (`repo_path`, `branch`, `worktree_path`), display fields, `has_uncommitted`, `commits_ahead/behind`, `default_branch`, `changed_files`, `lines_added/removed` (aggregate only, not per-file), `ahead_commit_messages` (up to 5, as plain strings not `ShippedCommit`), scan metadata, dismiss/snooze flags, `session_ids`, and a **partial** GitHub enrichment block (`github_pr_number/url/state/priority` — no approved/changes-requested counts, no CI conclusion).

**Field-name reconciliation needed for the merge**: `UnfinishedWorktree.lines_added/lines_removed/changed_files` (aggregate ints) vs. `VCSStatus`'s per-file `FileChange.additions/deletions` (need aggregation for compact mode) vs. `BacklogItemShipStatus` (no diff stats at all today — proposed extension target). Commit list shape also differs: `UnfinishedWorktree.ahead_commit_messages` is `repeated string`, `BacklogItemShipStatus.commits` is `repeated ShippedCommit` (richer, has SHA/author/timestamp) — the widget should standardize on `ShippedCommit`.

## 3. GitHub API client: `gh` CLI, not a Go SDK

No `google/go-github` or GraphQL client in `go.mod`. All GitHub API access shells out to the **`gh` CLI** via `safeexec.CommandContext` (repo's subprocess wrapper — see `.claude/rules/prefer-go-git-over-subshells.md`, which is about *git* ops, not GitHub API ops, so this shelling-out is the existing, accepted pattern for GitHub specifically since `gh` doesn't have a first-class Go SDK equivalent in use here).

- **Package**: `github/` (root-level Go package, imported as `github.com/tstapler/stapler-squad/github`) — `github/client.go` is the core file.
- **Key function**: `GetPRInfoCtx(ctx, owner, repo, prNumber)` (`github/client.go:246`) runs `gh pr view <ref> --repo <repoRef> --json <fields>`, parsing into `ghPRResponse` → `PRInfo` (`github/client.go:40`, includes `reviews` and `statusCheckRollup` sub-objects).
- **Review counts**: `parseReviewCounts(reviews []ghReviewItem) (approved, changesRequested int)` (`github/client.go:307`).
- **CI conclusion**: `getCheckConclusion(checks []ghStatusCheckItem) (conclusion, status string)` (`github/client.go:334`).
- **Poller**: `session/pr_status_poller.go` — `PRStatusPoller` runs a bounded-concurrency (`ConcurrentFetches`) loop calling `GetPRInfoCtx` per live session/instance and pushing results onto the in-memory `Session.github*` fields via `applyPRUpdate` (`pr_status_poller.go:381`). This is the poller to reuse/extend for durable persistence — either by adding a persistence step to its existing update path, or by having `GetBacklogItemShipStatus` (or a new backend method) call `GetPRInfoCtx` directly for done items using `item.PrURL`.
- **Also present**: `GetPRForBranch`, `GetPRComments`, `GetPRDiff`, `MergePR`, `ClosePR`, `PostPRComment`, `CloneRepository` — all `gh` CLI subprocess wrappers in the same file/pattern.

**Recommendation for backend work**: reuse `github.GetPRInfoCtx` (and its review/CI parsing helpers) rather than adding a new GitHub client library — it already returns everything (`ApprovedCount`, `ChangesReqCount`, `CheckConclusion`) the durable-persistence extension needs; the new work is *where/when to call it and persist the result*, not how to fetch it.

## 4. ent ORM: no durable snapshot schema exists yet — ship status is computed on read

`session/ent/schema/backlog_item.go` has a `pr_url` string field and nothing else GitHub-related. There is **no ent schema backing `BacklogItemShipStatus` or `ShippedCommit`** — `server/services/backlog_service_ship_status.go`'s `GetBacklogItemShipStatus` computes everything live at request time:
1. Loads the `BacklogItem` + its `ItemSessionSummary` list from storage (ent-backed).
2. Finds the most recent work-session's `LastCommitSha`/`LastCommitMessage`/`LastCommitAt` (these *are* durably stored ent fields, on `item_session.go` presumably).
3. Calls `git.IsCommitOnMain(item.RepoPath, mainBranch, sha)` (go-git-based, live git operation against the repo on disk) to determine `shipped`.
4. Calls `git.BranchAheadBehind` and `git.ListShippedCommits` (also live git-history reads) for ahead/behind counts and the commit list.

So today's "durable" ship status is really "recomputed from git history every request" — it survives worktree cleanup only because it reads from `item.RepoPath` (the repo, not the worktree) rather than the ephemeral worktree directory. **This is exactly why GitHub CI/PR/review data and per-file diff stats can't currently survive worktree cleanup**: git history has no concept of GitHub review state or diff `--numstat` per file at a point in time — that data must be captured *while it's live* (from `Session.github*` fields, populated by `PRStatusPoller`) and written to a **new ent schema** (e.g., extending `backlog_item.go` or a new `backlog_item_ship_status` / `github_pr_snapshot` entity) at a natural persistence point (session completion/worktree cleanup, or opportunistically whenever `PRStatusPoller` succeeds for a session tied to a backlog item).

Existing ent schemas worth modeling the new persistence after: `diffstats.go` (already models diff-stat persistence — check its field shape for the additions/deletions pattern to reuse for per-file snapshots) and `worktree.go`/`item_session.go` (show how commit SHA/message/timestamp are durably captured today). Schema changes must use the exact command in `session/ent/generate.go` (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`) per `.claude/rules/ent-schema-generation.md`.

## 5. React / vanilla-extract versions and variant pattern

- React `^19.0.0`, `react-dom ^19.0.0`.
- `@vanilla-extract/css ^1.20.1`, `@vanilla-extract/recipes ^0.5.7`, `@vanilla-extract/next-plugin ^2.5.1`.
- The `recipe()` pattern (per `.claude/rules/css-architecture.md`) is already used repo-wide for exactly this kind of density/variant need — see `web-app/src/components/ui/Badge.css.ts`:
  ```ts
  export const badge = recipe({
    base: { display: "inline-flex", ... },
    variants: {
      intent: { default: {...}, success: {...}, warning: {...}, error: {...} },
      size: { sm: {...}, md: {...} },
    },
    defaultVariants: { intent: "default", size: "md" },
  });
  ```
  This is the direct template for the unified VCS widget's "full detail vs. compact list-row" density requirement: add a `density` (or `variant`) axis (e.g. `full` | `compact`) to a `recipe()`, following the same `variants` + `defaultVariants` shape, rather than building two separate components. Other multi-variant `recipe()` usages worth checking as precedent: `web-app/src/components/layout/DrawerNav.css.ts`, `web-app/src/components/logs/LogRow.css.ts`.
- Existing components most directly relevant as starting points/reference for the unified widget: `web-app/src/components/sessions/VcsPanel.tsx` (+ `.css.ts`), `web-app/src/components/shared/VcsStatusDisplay.tsx` (+ `.css.ts`), `web-app/src/components/backlog/ShipStatusDisplay.tsx` (+ `.css.ts`, has a co-located `.test.tsx`).

## Key files reference

- `web-app/src/lib/contexts/SessionVcsContext.tsx`, `web-app/src/lib/hooks/useSessionVcs.ts`
- `web-app/src/lib/hooks/useBacklogItemShipStatus.ts`
- `web-app/src/lib/hooks/useUnfinishedWork.ts`, `web-app/src/lib/hooks/useUnfinishedWorkConfig.ts`
- `web-app/src/components/sessions/VcsPanel.tsx`
- `web-app/src/components/shared/VcsStatusDisplay.tsx`
- `web-app/src/components/backlog/ShipStatusDisplay.tsx`
- `web-app/src/components/unfinished/UnfinishedItemDetail.tsx`, `UnfinishedItem.tsx`, `UnfinishedRepoGroup.tsx`
- `web-app/src/components/ui/Badge.css.ts` (recipe/variant template)
- `proto/session/v1/types.proto` (`VCSStatus` L870, `FileChange` L849, `Session` github fields L80-139, `UnfinishedWorktree` L1319)
- `proto/session/v1/backlog.proto` (`BacklogItemShipStatus` L210, `ShippedCommit` L238)
- `server/services/backlog_service_ship_status.go` (compute-on-read ship status)
- `session/pr_status_poller.go` (`PRStatusPoller`, live GitHub polling for sessions)
- `github/client.go` (`gh` CLI wrapper — `GetPRInfoCtx`, `parseReviewCounts`, `getCheckConclusion`)
- `session/ent/schema/backlog_item.go`, `session/ent/schema/diffstats.go`, `session/ent/schema/worktree.go`, `session/ent/schema/item_session.go`
- `session/ent/generate.go` (correct ent codegen command)
