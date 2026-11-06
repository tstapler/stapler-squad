# Implementation Plan: Start backlog work from the latest remote main

## Summary

AC 1–3 are already satisfied by the existing codebase: worktrees are branched from a
freshly-fetched `origin/<default-branch>` tip (or `BaseBranch` override), and
`ItemSession.base_commit_sha` is stored in the DB. The remaining work is:

- **Story 1**: Expose `base_commit_sha` in the `ItemSession` proto (AC 4)
- **Story 2**: Map it in `itemSessionToProto` and add a proto-round-trip test (AC 4)
- **Story 3**: Surface it in the MCP `get_backlog_item` response and/or the UI (AC 5)

---

## Story 1 — Add `base_commit_sha` to `ItemSession` proto

**File**: `proto/session/v1/backlog.proto`

Add field 28 to the `ItemSession` message, immediately after `error_detail` (field 27):

```protobuf
  // base_commit_sha is the worktree's HEAD at spawn time — the base of the
  // base..HEAD diff the review gate uses, and confirmation that work started
  // from the freshly-fetched origin/<default-branch> tip. Empty for rows
  // created before this field existed or for non-work-role sessions.
  string base_commit_sha = 28;
```

Then run: `make proto-gen` (generates `gen/session/v1/backlog.pb.go`).

Do NOT commit the generated output; the Makefile's `build` target regenerates it.

**Acceptance check**: `make build` succeeds.

---

## Story 2 — Map the field in `itemSessionToProto` + add a test

**File**: `server/services/backlog_service.go`

In `itemSessionToProto` (line ~888), add to the struct literal:

```go
BaseCommitSha: is.BaseCommitSha,
```

**File**: `server/services/backlog_service_triage_test.go` (or
`backlog_service_test.go`)

Add a test `TestSpawnSessionFromItem_should_RecordBaseCommitSha_In_ProtoResponse`:

1. Create a git repo with a commit on `main`, push a tracking ref for origin/main.
2. Call `SpawnSessionFromItem`.
3. Assert that `response.ItemSession.BaseCommitSha` is a non-empty 40-char hex string.
4. Assert it matches the SHA `git.GetGitHeadSHA` would return from the worktree.

Follow the patterns in `TestSpawnSessionFromItem_should_StampRealGitBranchOnItemSession`
(`backlog_service_test.go:1935`), which already verifies `worktree_branch` is set.

**Acceptance check**: `make test` passes for `./server/services/...`.

---

## Story 3 — Surface base_commit_sha in MCP response and/or UI

### 3a. MCP (operator/agent visibility — primary AC 5 target)

**File**: `server/mcp/tools_backlog.go`

The `get_backlog_item` MCP tool already returns `ItemSession` objects. Since Story 2
adds `BaseCommitSha` to the proto, it will automatically appear in the JSON-marshalled
MCP response with no additional code change, as long as the MCP layer serializes the
proto to JSON directly.

Verify by checking `buildGetBacklogItemResult` (or equivalent) in `tools_backlog.go`
to confirm it serializes the full `ItemSession` proto — if so, no code change needed
for MCP.

If the MCP layer hand-builds the response struct (not auto-marshalled from proto),
add `BaseCommitSha: is.BaseCommitSha` in the same pattern used for `BranchName`.

### 3b. Frontend (user visibility)

**File**: `web-app/src/components/BacklogItemDetail.tsx` (or the session history
sub-component)

In the work-session row that displays `worktree_branch`, add a short-SHA display:

```tsx
{session.baseCommitSha && (
  <span title={session.baseCommitSha} className={styles.baseCommitChip}>
    from {session.baseCommitSha.slice(0, 7)}
  </span>
)}
```

The `baseCommitSha` field name will be auto-generated from the proto snake_case →
camelCase conversion the frontend already uses for other `ItemSession` fields.

**Acceptance check**: The session history row in the Backlog Item Detail drawer shows
"from a1b2c3d" (7-char short SHA) for work sessions that have a `base_commit_sha`.

---

## Story 4 — Hardening test: first spawn uses freshly-fetched SHA (regression guard)

**File**: `session/git/ops_test.go` (or a new `instance_worktree_test.go`)

Add `TestCreateBacklogWorktree_BranchesFromFetchedOriginTip_NotAmbientHead`:

1. Create a local bare "origin" repo with a commit on `main`.
2. Clone it locally, leaving the clone's ambient HEAD on a diverged feature branch.
3. Add a new commit to the origin's `main` (ahead of the clone's cached remote ref).
4. Call `CreateBacklogWorktree` against the clone.
5. Assert the returned worktree's HEAD SHA matches the **latest** origin/main tip (the
   new commit), not the clone's stale local `main` or the ambient HEAD.

This closes the regression gap noted in the research: no existing test proves freshness
against a *moved* origin. (`TestResolveWorktreeBaseCommit_*` tests that the function
fetches, but not that `CreateBacklogWorktree` as a whole delivers a fresh worktree.)

**Acceptance check**: `make test` passes for `./session/...`.

---

## Sequence

Stories 1 and 2 can be done in a single commit (proto field + mapping + test).
Story 3a typically requires no code (auto-serialized); verify first.
Story 3b (frontend) is a separate commit.
Story 4 is an optional hardening step; can be a follow-up if Stories 1–3 are time-boxed.

## Files changed

| File | Change |
|---|---|
| `proto/session/v1/backlog.proto` | Add `base_commit_sha` field 28 to `ItemSession` |
| `server/services/backlog_service.go` | Map `BaseCommitSha` in `itemSessionToProto` |
| `server/services/backlog_service_test.go` | Add proto round-trip test |
| `server/mcp/tools_backlog.go` | Verify/add `BaseCommitSha` in MCP result (Story 3a) |
| `web-app/src/components/BacklogItemDetail.tsx` | Show short SHA in session row (Story 3b) |
| `session/git/ops_test.go` | Add freshness regression test (Story 4, optional) |
