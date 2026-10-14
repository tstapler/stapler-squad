# Validation: Start backlog work from the latest remote main

## Adversarial review

### Challenge 1: Is AC 1 actually already satisfied?

**Claim**: "Work sessions branch from freshly-fetched origin tip."

**Counter-check**: `FetchBranch` has a 30-second timeout. On a slow network or large
repo, the fetch could time out, and the code falls back to `ResolveDefaultLocalBranchSHA`
(the cached local remote-tracking ref, `origin/main` in `.git/refs/remotes/`). If the
local machine hasn't connected to the network in days, this cached ref could be stale
by many commits.

**Verdict**: This is a real (if rare) gap. The fallback is documented and intentional
(offline scenarios), but it means "start from latest remote main" is a best-effort
guarantee, not an absolute one. The item's description implies absolute correctness,
but the practical risk is low for connected machines. Recommend: document this in the
`base_commit_sha` proto field comment so operators know offline fetches may produce a
stale base; hardening to a hard-fail on offline is out of scope.

### Challenge 2: Does `base_commit_sha` actually reflect the freshly-fetched tip?

**Claim**: `SetItemSessionBaseCommit` records `GetGitHeadSHA(worktreePath)` immediately
after worktree creation.

**Counter-check**: `GetGitHeadSHA` reads the worktree's HEAD. If `CreateBacklogWorktree`
reuses an existing branch (`setupFromExistingBranch` path), the worktree HEAD could be
ahead of origin/main (commits already made in a prior work session). In that case, the
"base" is not origin/main — it's the branch tip after prior work.

**Verdict**: This is correct behavior for rework cycles (the base is the start of *this
session's* new commits, not always origin/main). AC 3 says "contains the worktree's HEAD
SHA at spawn time" — that's what is recorded. The description's "what it was branched
from" is accurate per-session, not a repo-wide "original main tip." No change needed;
the plan's AC wording is accurate.

### Challenge 3: Does adding `base_commit_sha = 28` to `ItemSession` break anything?

**Counter-check**: Proto field 28 is unused in `ItemSession` (highest existing field is
27). Adding field 28 is forward-compatible. Old clients deserializing a response with
field 28 will silently ignore it (proto3 unknown-field behavior). No migration needed.

**Verdict**: Safe. No backward-compatibility risk.

### Challenge 4: Will `base_commit_sha` be auto-populated in ALL `ItemSession` proto
responses, or only new spawns?

**Counter-check**: `SetItemSessionBaseCommit` is called at spawn time and at attach time.
Existing `ItemSession` rows in the DB already have `base_commit_sha` populated (the ent
field has existed since before this triage, written at spawn). The `itemSessionToProto`
change will surface it for all existing rows that have it set, not just new ones.

Rows from before the ent field was added (very old items, pre-2026) will have empty
`base_commit_sha`. The proto field is a plain `string` (not `optional`), so it will
serialize as `""` for those rows — correct behavior.

**Verdict**: No issue. The change is safe for existing data.

### Challenge 5: Does the MCP auto-serialize `ItemSession.BaseCommitSha`?

**Claim**: "Story 3a requires no code change if the MCP layer auto-serializes proto."

**Counter-check**: Need to verify `server/mcp/tools_backlog.go`'s `get_backlog_item`
result builder. If it uses `protojson.Marshal` on the full proto, field 28 is automatic.
If it hand-builds a Go struct with explicitly-named fields, `BaseCommitSha` must be
added manually.

**Action**: Implementer must inspect `tools_backlog.go` before claiming Story 3a is
code-free. The plan already flags this as "verify first."

### Challenge 6: Is the frontend `baseCommitSha` field name correct?

**Counter-check**: Proto snake_case `base_commit_sha` → JS/TS camelCase `baseCommitSha`
is standard proto-to-JSON conversion. The frontend connects via ConnectRPC which applies
this transformation. Verify by checking how `worktree_branch` (proto field 14) is
accessed in the frontend — it should be `worktreeBranch` in TS.

**Verdict**: Consistent with existing patterns; no issue anticipated.

## Pre-implementation checklist

- [ ] Confirm `ItemSession` proto field 28 is unused (checked: yes, confirmed above)
- [ ] Confirm `make proto-gen` is available in this worktree (requires `make build` first)
- [ ] Confirm `base_commit_sha` is already set in the DB for recent items
      (`SELECT base_commit_sha FROM item_sessions ORDER BY created_at DESC LIMIT 5`)
- [ ] Verify Story 3a: inspect `server/mcp/tools_backlog.go` for auto- vs. hand-build
- [ ] For Story 3b: confirm `baseCommitSha` field name by checking how `worktreeBranch`
      is used in `BacklogItemDetail.tsx` or its sub-components

## Rollback plan

The proto change (field 28 addition) is backward-compatible and rollback is achieved
by reverting the proto file and regenerating. The `itemSessionToProto` change simply
stops populating field 28 in the response — existing DB rows are unaffected.
No DB migration is required or performed.
