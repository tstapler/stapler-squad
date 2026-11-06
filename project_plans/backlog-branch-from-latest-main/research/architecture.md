# Research: Start backlog work from the latest remote main

## 1. Current branching flow (verified against source)

### Work-session path (primary)

```
SpawnSessionFromItem (backlog_service_triage.go:518)
  └─ spawnSessionAfterGates (backlog_service_triage.go:887)
       └─ resolveSessionPath (backlog_service_triage.go:1779)
            └─ session.CreateBacklogWorktree (session/instance_worktree.go:370)
                 └─ git.WithRepoWorktreeLock
                      ├─ RepairCorruptedGitRepo
                      ├─ if BaseBranch != ""
                      │    └─ git.ResolveExplicitBranchSHA  ← fetches from origin first
                      └─ else
                           └─ git.ResolveWorktreeBaseCommit ← fetches from origin first
                                ├─ git.ResolveDefaultBranchSHA
                                │    └─ git.ResolveOriginBranchSHA
                                │         └─ git.FetchBranch (30s timeout)
                                └─ fallback: git.ResolveDefaultLocalBranchSHA (offline)
```

Key: `FetchBranch` always runs a real `git fetch origin <branch>` before resolving the
SHA, so the returned SHA reflects origin's true current tip, not the cached local
remote-tracking ref.

### Triage path

```
TriggerTriage (backlog_service_trigger_triage.go)
  ├─ if BaseBranch != "": git.ResolveExplicitBranchSHA (fetches first)
  └─ else: git.ResolveWorktreeBaseCommit (fetches first)
       └─ on any error: soft-fallback to ambient HEAD (best-effort for triage)
```

### Base commit recording

After the worktree is created, `GetGitHeadSHA(worktreePath)` reads the worktree's HEAD
and stores it as `ItemSession.base_commit_sha` in the ent DB via `SetItemSessionBaseCommit`
(`session/storage_backlog.go:646`). This is set at three call sites:

| Call site | Role |
|---|---|
| `spawnSessionAfterGates` L1179 | Work session spawn |
| `AttachSessionToItem` L141 | Manual attach |
| `TriggerReReview` L3357 | Re-review spawn |

Missing: triage sessions do not record a base commit (the triage worktree is
ephemeral and gets renamed / cleaned up anyway).

## 2. Proto and API layer (gap analysis)

### ItemSession proto (`proto/session/v1/backlog.proto:77`)

Present fields relevant to branching:
- `worktree_branch` (field 14) — branch name
- `worktree_path` (field 15) — worktree directory

**Missing**: no `base_commit_sha` field. `itemSessionToProto` (`backlog_service.go:888`)
does not map `is.BaseCommitSha` to any proto field. The value lives in the DB but is
never sent over the wire.

### BacklogItem proto (`proto/session/v1/backlog.proto:212`)

- `base_branch` (field 37) — the user-configured override (surfaced already)

**Missing**: no `effective_base_sha` or `effective_base_branch` field showing what was
actually resolved at spawn time for items with no override.

## 3. Impacted files for the proto + mapping change

| File | Change |
|---|---|
| `proto/session/v1/backlog.proto` | Add `base_commit_sha` field to `ItemSession` message |
| `gen/session/v1/backlog.pb.go` | Auto-generated (do not edit; `make proto-gen`) |
| `server/services/backlog_service.go` | Map `is.BaseCommitSha` in `itemSessionToProto` |

## 4. UI surfacing (gap analysis)

The `ItemSession` proto is consumed by:
- `BacklogItemDetail.tsx` — session history list in the item detail drawer
- MCP `get_backlog_item` tool (`server/mcp/tools_backlog.go`) — returns
  `GetBacklogItemResponse` which includes the item and its sessions

Neither currently displays `base_commit_sha`. Adding it to the proto (AC 4) is
necessary but not sufficient for AC 5 — a UI or MCP change is also required.

### Minimal viable display

Option A (backend-only): Surface it in the MCP `get_backlog_item` response body as
"branched from commit `<shortSHA>`". Satisfies AC 5 for operator/agent visibility
without a frontend change.

Option B (frontend + MCP): Show a short SHA badge next to the branch name in the
session history row of `BacklogItemDetail.tsx`. More visible but requires a frontend
PR.

## 5. Existing test coverage for the branching behavior

Tests already covering AC 1–3 (confirmed passing, no code changes needed):

| Test | Location | What it covers |
|---|---|---|
| `TestResolveWorktreeBaseCommit_*` | `session/git/ops_test.go:353+` | Fetches from origin, falls back to local |
| `TestResolveExplicitBranchSHA_UsesOrigin` | `session/git/ops_test.go:575` | Explicit branch fetches origin first |
| `TestCreateBacklogWorktree_AnchorsAtMainRepo_When_RepoPathIsAWorktree` | `session/repo_path_test.go:599` | Worktree anchored at main repo root |
| `TestCreateBacklogItem_BaseBranch_RoundTripsThroughGetBacklogItem` | `session/backlog_lifecycle_test.go:4867` | BaseBranch persists through GetBacklogItem |

Gap: no test asserts that `ItemSession.BaseCommitSha` is populated and non-empty in the
proto response after `SpawnSessionFromItem`. This should be added as part of AC 4.

## 6. Risk and alternatives

### Risk: proto field number collision
Adding `base_commit_sha` to `ItemSession` requires picking the next unused field number
(currently highest is 27 for `error_detail`). Field 28 is safe.

### Risk: backward compatibility
Existing `ItemSession` rows in the DB have `base_commit_sha` populated (the ent field
exists and is already written). Adding field 28 to the proto is a backward-compatible
addition — old clients ignore the new field, new clients see it populated for all rows
that had it set.

### Alternative: defer UI to a follow-up
Since the DB and domain layer already store the value correctly (AC 1–3 verified), the
highest-value deliverable is the proto exposure (AC 4). The UI/MCP display (AC 5) could
be a follow-up, though the item description's "the item shows" phrasing makes it a
primary AC.

## 7. Related items and history

- PR #833: `fix(session): resolve new_worktree base branch from origin, wire scrollback
  capture` — fixed the `new_worktree` session type; does not affect backlog work paths.
- PR #837: also related to worktree base-branch resolution (different item
  `c7466f05-3d19-4d25-a822-9ea1ac7a6faa`).
- Item `4d856751-bfe9-4566-99ca-2c483b23bd06` (`worktree-base-branch-and-capture-dup-check`):
  was a duplicate of the above, confirmed already-fixed during triage.
- This item (`ea3c02bd`) is **distinct** from those: the branching behavior is already
  correct; the remaining work is proto/API/UI exposure of `base_commit_sha`.
