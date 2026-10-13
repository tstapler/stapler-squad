# Requirements: Start backlog work from the latest remote main, not a stale local branch

## Source

Backlog item `ea3c02bd-44be-4253-bfbe-f1f6b87b9b71`: "Start backlog work from the
latest remote main, not a stale local branch"

## Branch base

Verified 2026-10-13 after `git fetch origin`: this branch
(`backlog/stapler-squad-backlog-branch-from-latest-main`) is based on the current
`origin/main` tip, `436ff342bccaf5efd653ac59fc77296fd8e2efa1` (0 commits behind). No
upstream is configured for the branch, so `origin/main` is the base.

## Original description

New backlog items can begin planning and implementation from an out-of-date base, which
leads to costly rework when the changes are rebased or merged later. When work starts on a
backlog item, the item should branch from the latest remote default branch (origin/main),
or from the configured upstream branch if one is set. The base commit should be recorded
so the item shows what it was branched from.

## Pre-triage codebase findings

### What is already implemented

Both the triage phase and work phase already fetch from origin before branching:

**Work phase (`CreateBacklogWorktree`**, `session/instance_worktree.go:370`):
- Calls `git.ResolveWorktreeBaseCommit(resolvedRepo)` → `ResolveDefaultBranchSHA` →
  `ResolveOriginBranchSHA` → `FetchBranch` (30 s timeout, go-git via Backend)
- Falls back to local branch only when origin is unreachable (offline / no remote)
- Hard-fails if neither origin nor local can resolve the branch (never branches from
  ambient HEAD except for unborn repos)
- `BaseBranch` override on `BacklogItemData` supported via `ResolveExplicitBranchSHA`
  (also fetches from origin first)

**Triage phase (`TriggerTriage`**, `server/services/backlog_service_trigger_triage.go`):
- Same `ResolveWorktreeBaseCommit` call with `FetchBranch`
- Soft fallback to ambient HEAD on any resolution failure (best-effort for triage;
  a bad base is caught with a hard error later in the work phase)

**`BaseCommitSha` recording** (`SetItemSessionBaseCommit`,
`server/services/backlog_service_triage.go:1179`):
- After the worktree is created, `GetGitHeadSHA(worktreePath)` captures the worktree
  HEAD and stores it in `ItemSession.base_commit_sha` (the ent schema field)
- Also set for re-review sessions and via `AttachSessionToItem`
- This SHA is used by the review gate for `base..HEAD` diffs and by superseded-rework
  detection

### Remaining gaps

1. **`base_commit_sha` is not surfaced in the `ItemSession` proto message** — the
   `ItemSession` proto (`proto/session/v1/backlog.proto:77`) has `worktree_branch` but
   no `base_commit_sha` field; `itemSessionToProto` never maps `is.BaseCommitSha` to a
   proto field. Clients (UI, MCP) cannot see what commit the work branched from.

2. **No UI visibility** — neither the backlog item detail page nor the MCP
   `get_backlog_item` tool surface the base commit SHA / base branch name that work
   sessions started from. The description says "the item shows what it was branched
   from," which implies this visibility is the primary deliverable.

3. **`BacklogItem.BaseBranch` (the configured override) is surfaced in the proto but
   there is no `effective_base_branch` or `branched_from_sha` field on `BacklogItem`
   itself** — users can see the override they set, but not what origin branch was
   actually resolved at spawn time for items with no override.

4. **Triage soft-fallback to ambient HEAD** — when `ResolveWorktreeBaseCommit` fails
   during triage (origin unreachable), the triage worktree silently branches from
   ambient HEAD. This is deliberate and documented, but is still a gap versus the
   stated requirement that work begins from the latest remote default branch.

### Out of scope

- Rework cycles intentionally reuse the existing `backlog/<item>` branch rather than
  rebasing to main on every iteration — this is by design and is not being changed.
- `new_worktree` session type (distinct feature, fixed by PR #833).

## Acceptance criteria

1. When a first-time backlog work session is spawned for a git-managed repo, the
   worktree branches from the freshly fetched `origin/<default-branch>` tip, not from
   the repo's ambient checked-out HEAD, confirmed by a test that checks the worktree's
   HEAD SHA matches the mock origin tip.
2. When `BaseBranch` is set on the item, the worktree branches from that branch's
   freshly fetched origin tip (not its local SHA), with a hard error if the branch
   doesn't exist on origin or locally.
3. After work is spawned, `ItemSession.base_commit_sha` (the ent DB field) contains the
   worktree's HEAD SHA — a non-empty 40-character hex string.
4. The `ItemSession` proto response returned by `SpawnSessionFromItem`,
   `GetBacklogItem`, and `ListBacklogItems` includes a `base_commit_sha` field
   populated with the recorded SHA for work-role sessions.
5. The backlog item detail UI (or the MCP `get_backlog_item` response) displays the
   base branch / commit that each work session was branched from, so an operator can
   verify work started from an up-to-date base.
6. Existing tests (especially `TestResolveWorktreeBaseCommit_*`,
   `TestCreateBacklogWorktree_*`, `TestSpawnSessionFromItem_*`) continue to pass with no
   regressions.

## Complexity estimate

Medium — ACs 1–3 are already satisfied by existing code (verified above), so the work
is primarily proto/API plumbing (AC 4) and UI/MCP surfacing (AC 5). No ent schema
changes are needed (the DB field already exists). Estimated: 1–2 back-end touches + 1
frontend touch.

## Open questions

- Should `base_commit_sha` be displayed as a truncated short SHA or a full 40-char SHA
  in the UI? (Suggest: 7-char short SHA with a copyable tooltip.)
- Should the triage soft-fallback to ambient HEAD be hardened to a hard failure, or
  left soft? (Suggest: leave soft for triage, the work phase already hard-fails.)
- Should `BacklogItem` itself gain an `effective_base_branch` field (what was resolved
  at spawn time), or is surfacing it per-`ItemSession` sufficient? (Suggest: per-session
  is enough; the branch can differ across rework rounds if main renames.)
