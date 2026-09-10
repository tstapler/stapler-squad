# Stack Research: vcs-tab-redesign

This is a full-stack wiring task, not a new-dependency task: every data source
the requirements ask for already exists in the codebase. No new Go module or
npm package is needed. Findings below with file:line citations.

## 1. Current dependency versions

**Go (`go.mod`)**
- `go 1.26.4`
- `github.com/go-git/go-git/v5 v5.14.0` (`go-git/gcfg`, `go-git/go-billy/v5 v5.6.2` as supporting deps)
- `connectrpc.com/connect v1.20.0`, `connectrpc.com/otelconnect v0.8.0`
- `google.golang.org/protobuf v1.36.12`
- `buf.build/gen/go/...` toolchain deps (bufplugin, protovalidate, registry) — all `// indirect`, pulled in by the `buf` CLI toolchain, not hand-maintained

**web-app (`web-app/package.json`)**
- `react: ^19.0.0`
- `@bufbuild/protobuf: ^2.11.0`, `@bufbuild/protoc-gen-es: ^2.11.0`
- `@connectrpc/connect: ^2.1.1`, `@connectrpc/connect-web: ^2.1.1`

**Date/time formatting** — no `date-fns`/`dayjs`/`luxon` dependency exists or is needed. A relative-time formatter already exists and is already wired into `VcsWidget`: `formatRelativeTime` in `web-app/src/lib/utils/datetime.ts`, used at `web-app/src/components/shared/VcsWidget.tsx:62` for the historical-snapshot "As of …" line (`snapshotAt`). The live-session "as of" staleness timestamp (Scope item 6) should reuse this exact function — do not add a date library.

**Collapsible/disclosure UI primitive** — already exists, no new component needed. `web-app/src/components/ui/Collapsible.tsx` exports `CollapsibleGroup` (wraps a shared Radix `Accordion.Root`, `type="multiple"`, for cross-header roving-tabindex keyboard nav — ADR-027) and `CollapsibleSection` (single progressive-disclosure section; renders as `Accordion.Item`/`Trigger`/`Content` inside a group, or mounts its own implicit `Accordion.Root` standalone). Built on `@radix-ui/react-accordion`. This is the exact primitive for itemized CI checks (Scope 3) and the "why blocked" rollup (Scope 4) — both are natural `CollapsibleSection`/`CollapsibleGroup` candidates, following the pattern already used throughout `web-app/src/components/backlog/detail/*.tsx` (e.g. `ActivityLogSection.tsx`, `ProgressHistorySection.tsx`).

## 2. Go data sources — signatures and repo-state requirements

All three read functions live in packages that already follow `.claude/rules/prefer-go-git-over-subshells.md` (go-git, no subshells) and `code-go-git` thread-safety conventions.

### `session/git/ops.go` — `ListShippedCommits` (stateless, opens repo per call)

```go
// session/git/ops.go:369
func ListShippedCommits(repoPath, baseSHA, headSHA string) ([]ShippedCommit, error)
```
- session/git/ops.go:330 — `ShippedCommit{ SHA, Summary, AuthorAt time.Time, AuthorName string }`
- Both `baseSHA`/`headSHA` **must already be resolved commit hashes**, not branch names (doc comment: caller typically has these from `GitWorktreeData.BaseCommitSHA` and the work session's `LastCommitSha` — these stay valid even after the branch is deleted post-merge).
- Implementation: `git.PlainOpenWithOptions(repoPath, ...)` opens fresh each call (no shared/cached `*git.Repository`, no mutex) — BFS from `head`, testing `IsAncestor(base)`, bounded by `listShippedCommitsCap = 100` (session/git/ops.go:355). Newest-first, matches a PR "Commits" tab.
- Thread-safety: safe to call from a live-session code path as-is — it does not touch any shared cache/mutex, unlike the `GoGitVCSReader` methods below. No adaptation needed beyond passing correct SHAs.
- Sibling `CommitInfo(repoPath, sha string) (ShippedCommit, error)` (session/git/ops.go:340) and `FileStatsBetween(repoPath, baseSHA, headSHA string) ([]FileStat, error)` (session/git/ops.go:434) exist alongside it for single-commit lookup and per-file diff stats respectively — `FileStatsBetween` is a second candidate for the aggregate diff-stat line (Scope 2) if per-file breakdown is wanted, though `DiffShortstat` below already gives the aggregate.

### `session/unfinished/gogit_vcs_reader.go` — `AheadBehind`, `CommitMessages`, `DiffShortstat` (cached, mutex-guarded, on `*GoGitVCSReader`)

```go
// session/unfinished/gogit_vcs_reader.go:1140
func (g *GoGitVCSReader) AheadBehind(worktreePath, base string) (int, int, error)

// session/unfinished/gogit_vcs_reader.go:1204
func (g *GoGitVCSReader) CommitMessages(worktreePath, base string, max int) ([]string, error)

// session/unfinished/gogit_vcs_reader.go:1305
func (g *GoGitVCSReader) DiffShortstat(worktreePath string) (DiffStat, error)
```
- All three are receiver methods on `*GoGitVCSReader`, not free functions — a live-session caller needs a `*GoGitVCSReader` instance (check how it's constructed/injected elsewhere, e.g. the backlog scanner, before reusing).
- All three are already safe for concurrent/live-session use: they use `entry.mu.Lock()`/`defer entry.mu.Unlock()` per-repo mutexes (never explicit unlock — panic-safe), `singleflight` (`sfDo`) to coalesce concurrent identical calls, and a 30s TTL cache (`diffStatCacheTTL`) keyed per-worktree/base — explicitly built to survive "concurrent scanner workers" (doc comment at gogit_vcs_reader.go:1301-1304 cites this as the top mutex hotspot fix, 537M cycles / 13,941 events).
- `AheadBehind`: BFS merge-base + two bounded `countCommitsTo` walks (no full-history walk); cache key `worktreePath + "\x00" + base`.
- `CommitMessages`: returns `[]string` of `"<7-char sha> <first line>"`, capped at `max`; three-phase locking (snapshot HEAD/base hash → compute reachable set → walk log) to minimize lock hold time.
- `DiffShortstat` returns a `DiffStat` (changed-file/line counts) — this is the direct source for Scope 2's "aggregate diff-stat line in full mode" (currently only wired to `compact`/backlog-scanner contexts per the requirements doc).
- `base` param in all three is a ref string resolved via `resolveRef(repo, base)` — confirm what ref shapes are accepted (branch name, SHA, `origin/main`, etc.) before wiring; not fully inspected in this pass, but the doc comments imply a resolvable ref, not necessarily a bare SHA.

### Adaptation needed
- `ListShippedCommits` is ready to call directly from a live-session path — no adaptation.
- The `GoGitVCSReader` methods require getting/injecting a `*GoGitVCSReader` instance into whatever service handles live-session VCS status (check `server/services/workspace_service.go` for how session status assembly is structured, and whether a `GoGitVCSReader` is already available there or needs to be threaded in).

## 3. GitHub client — current shapes (`github/client.go`)

### `PRInfo` struct (github/client.go:50-75)
```go
type PRInfo struct {
	Number, Title, Body, HeadRef, HeadSHA, BaseRef, State, Author string/int
	Labels       []string
	HTMLURL      string
	CreatedAt, UpdatedAt time.Time
	IsDraft      bool
	Mergeable    string
	Additions, Deletions, ChangedFiles int

	// Review and CI status fields (populated by GetPRInfo with extended fields)
	ReviewDecision        string // "approved"/"changes_requested"/"review_required"/""
	ApprovedCount         int
	ChangesRequestedCount int
	CheckConclusion       string // "success"/"failure"/"pending"/"action_required"/"neutral"/""
	CheckStatus           string // "completed"/"in_progress"/""
}
```
Note: `PRInfo` already carries `ReviewDecision`/counts/`CheckConclusion`/`CheckStatus` as single collapsed values — it does NOT carry the itemized `[]ghStatusCheckItem` or per-review `Body` text. Those are consumed and discarded inside `GetPRInfo`/whatever assembles `PRInfo` from the raw `gh` JSON response.

### `ghStatusCheckItem` (github/client.go:126-132) — itemized CI checks, currently discarded
```go
type ghStatusCheckItem struct {
	Name, Context, State, Status, Conclusion string
	// State: SUCCESS/FAILURE/PENDING/ERROR/NEUTRAL
	// Status: completed/in_progress/queued
	// Conclusion: success/failure/cancelled/action_required/neutral/skipped/timed_out
}
```
Populated from `gh pr view --json statusCheckRollup` (`ghPRResponse.StatusCheckRollup`, github/client.go:113). Only consumer today is `getCheckConclusion(checks []ghStatusCheckItem) (conclusion, status string)` (github/client.go:357-395), which collapses the whole slice down to one `(conclusion, status)` pair via failure/in-progress/success precedence and **discards `Name`/`Context` and per-check state**. Scope item 3 requires exposing `[]ghStatusCheckItem` (or a new typed slice on `PRInfo`) itemized, not just the collapsed pair.

### `ghReviewItem` (github/client.go:117-123) — review body text, currently discarded
```go
type ghReviewItem struct {
	Author struct{ Login string }
	State  string // APPROVED, CHANGES_REQUESTED, DISMISSED, COMMENTED, PENDING
	Body   string
}
```
Populated from `ghPRResponse.Reviews` (github/client.go:112). Only consumer today is `parseReviewCounts(reviews []ghReviewItem) (approved, changesRequested int)` (github/client.go:330-354) — dedupes to each author's latest non-COMMENTED/non-DISMISSED state and counts APPROVED vs CHANGES_REQUESTED, **discarding `Body` entirely**. Scope item 5 ("reviewer's changes-requested reason text") needs the `Body` from CHANGES_REQUESTED reviews surfaced, not just counted.

### `GetPRComments` (github/client.go:556) — separate call, PR-level comments not review bodies
```go
func GetPRComments(owner, repo string, prNumber int) ([]PRComment, error)
```
Shells out via `safeexec.CommandContext(ctx, "gh", "pr", "view", prRef, "--repo", repoRef, "--json", "comments")` — a **separate** `gh` invocation from the one that fetches `reviews`/`statusCheckRollup` (those come from whatever builds `ghPRResponse`). `PRComment` (github/client.go:78-86) has `ID, Author, Body, CreatedAt, Path, Line, IsReview`. This is issue/review *comments*, distinct from review *body* text on `ghReviewItem` — Scope item 5 wants the latter (the review's own `Body`), not `GetPRComments`.

## 4. Proto shape and adapter data flow

### Current `VCSStatus` proto message (proto/session/v1/types.proto:949-997)
Carries only local git-state fields — branch, head_commit, description, ahead_by/behind_by, upstream, has_staged/unstaged/untracked/conflicts, is_clean, and 4 `repeated FileChange` lists (staged/unstaged/untracked/conflict). **No PR/GitHub fields, no commit list, no aggregate diff-stat field.**

### PR/GitHub fields actually live on `Session`, not `VCSStatus`
`proto/session/v1/types.proto:80-141` — `Session` message's "GitHub integration fields" block: `github_pr_number` (25), `github_pr_url` (26), `github_owner`/`github_repo` (27/28), `github_pr_state` (33), `github_pr_is_draft` (34), `github_pr_priority` (35), `github_approved_count` (36), `github_changes_req_count` (37), `github_check_conclusion` (38 — single collapsed string), `last_pr_status_check` (39, `google.protobuf.Timestamp`). These are populated by `PRStatusPoller` per the doc comments, separately from `VCSStatus`.

There's also a fuller `PRInfo` proto message (proto/session/v1/types.proto:735-783) — used for PR-URL session-creation context, NOT wired to live-session status; it has number/title/body/refs/state/author/labels/html_url/is_draft/mergeable/additions/deletions/changed_files/created_at/updated_at, but **no review-decision, no check-conclusion, no itemized checks** — a different, older shape than the `Session`-level GitHub fields.

### `vcsStatusToProto` (server/services/workspace_service.go:414-448)
Straight 1:1 field mapper from the internal `vc.VCSStatus` Go struct to `sessionv1.VCSStatus` proto, plus per-file-list mapping via `fileChangeToProto`. No commits, no aggregate stats, no GitHub data — confirms `VCSStatus` proto is local-git-only today, matching the requirements doc's read of `fromSessionVcs`.

### `fromSessionVcs` adapter (web-app/src/lib/vcs/adapters.ts:83-96)
```ts
export function fromSessionVcs(status: VCSStatus, session?: Session): VcsWidgetData {
  return {
    kind: "live",
    branch: status.branch,
    isClean: status.isClean,
    fileChanges: flattenFileChanges(status),
    aheadOfMain: status.aheadBy,
    behindMain: status.behindBy,
    branchExists: true,
    commits: [],                          // <-- hardcoded empty (Scope 1)
    github: fromSessionGithub(session),   // reads the Session-level github_* fields
    shipped: false,
    // no aggregateStats set (Scope 2)
  };
}
```
`fromSessionGithub` (adapters.ts:68-81) maps `Session.github*` fields into `GithubSummary` (web-app/src/lib/vcs/types.ts:33-43) — `owner, repo, prUrl, prNumber, prState, isDraft, checkConclusion, approvedCount, changesReqCount`. `GithubSummary` has no room for itemized checks or review body text; extending it is required for Scope 3/5.

`VcsWidgetData` (types.ts:47-83) already has an `aggregateStats?: { filesChanged, additions, deletions }` field (used today only by `fromUnfinishedWorktree`, adapters.ts:214) and a `commits: CommitSummary[]` field (`CommitSummary = { sha, summary, authorName?, authoredAt? }`, types.ts:23-28) — both are ready-made slots `fromSessionVcs` needs to actually populate; no new TS types needed for Scope 1/2, only new proto fields to source the data from and adapter wiring.

`deriveMergeabilityState` (web-app/src/lib/vcs/mergeability.ts:20-35) picks a single `MergeabilityState` via a fixed precedence chain (shipped → snapshot_unavailable → no_pr → draft → conflicted → changes_requested → ci_failing → closed_unshipped → ci_pending → ready_to_merge) — this is the "single top-precedence pill" Scope item 4 wants replaced/supplemented with an ALL-blocking-reasons rollup. The existing function can likely stay as the pill (still useful as a headline state) while a new sibling function collects every true condition instead of returning on first match.

### Required proto additions (inferred, not yet designed — for the planning phase)
1. `VCSStatus` (or a new nested message): `repeated ShippedCommitInfo commits` and an aggregate diff-stat sub-message (filesChanged/additions/deletions) to satisfy Scope 1/2 — note `session/git/ops.go`'s `ShippedCommit` Go struct already has the right shape to mirror.
2. Extend the `Session`-level GitHub fields (or introduce a new message referenced from `Session`) with: `repeated CheckItem checks` (name, conclusion, status — mirroring `ghStatusCheckItem`) for Scope 3, and a way to carry the changes-requested review body text(s) for Scope 5 (e.g. `repeated ReviewFeedback` with author + body + state).
3. A staleness timestamp is likely already coverable by the existing `last_pr_status_check` (types.proto:130) for the GitHub-derived half of the tab — but the git-state half (`VCSStatus`) has no equivalent "as of" field today; check whether one is needed there too, or whether VCSStatus is always computed synchronously (no staleness) unlike the polled GitHub fields.

## 5. `make proto-gen` workflow

```
make proto-gen   # target: Makefile:503-518
```
- Depends on `ensure-tools` and `web-app/node_modules/.modules.yaml`.
- Stamp-file gated (`$(PROTO_STAMP)`): regenerates only if the stamp is missing, any `proto/**/*.proto` is newer than the stamp, `protoc-gen-es` binary is newer than the stamp, or the expected Go/TS output files (`gen/proto/go/session/v1/session.pb.go`, `web-app/src/gen/session/v1/session_pb.ts`) are missing.
- Runs `buf generate proto` — outputs to `gen/proto/go/` (Go) and `web-app/src/gen/` (TypeScript), both gitignored per generated-code policy (mirrors the `session/ent/*.go` policy in root CLAUDE.md — do not commit generated output, only the `.proto` source changes).
- Invoked automatically by `make build`/`make test`/`make lint` (all depend on proto-gen equivalents per root CLAUDE.md's ent-gen note pattern) — so editing `.proto` files and running `make build` regenerates both sides before compiling.
- Workflow for this feature: edit `proto/session/v1/types.proto` (and/or `session.proto`) → `make proto-gen` (or just `make build`, which triggers it) → implement Go-side population (`vcsStatusToProto` etc.) → implement TS-side adapter changes (`fromSessionVcs`) → `go build ./...` and `cd web-app && npx tsc --noEmit` (or equivalent) to confirm both sides compile against the new generated types.

## Version/stability concerns

- None of the involved dependencies are unusually old or flagged for upgrade: go-git v5.14.0, ConnectRPC v1.20.0/v2.1.1, protobuf v1.36.x, React 19, `@bufbuild` v2.11.0 are all current-generation majors already in active use elsewhere in the codebase.
- The only real risk is **scope creep in the Go GitHub client**: `getCheckConclusion`/`parseReviewCounts` are pure collapsing functions with no test coverage gaps noted here, but exposing the underlying itemized slices means either (a) adding new fields to `PRInfo` alongside the existing collapsed ones (safer, additive), or (b) restructuring how `ghPRResponse` data flows to callers — (a) is strongly preferred to avoid touching working collapse logic other callers may depend on.
- `GoGitVCSReader`'s three methods are receiver methods requiring an instance — confirm at planning time where/how one is already constructed for live sessions (likely already present given `session/unfinished` package naming suggests it's for a different scanning context; may need a second reader instance or a shared one — worth checking `server/server.go`'s dependency wiring in Phase 3 planning).
