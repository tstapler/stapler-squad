# BUG-062: `TriggerTriage` Passes an Unvalidated `repo_path` to the Headless Subprocess's WorkDir, Producing a Misleading "claude not found" Error [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-08-05, live in this repo's own deployed instance — four backlog items self-filed via the `create_backlog_item` MCP tool with `repo_path` set to a bare slug (e.g. `"stapler-squad"`) instead of an absolute path (caller error). Every triage attempt for all four failed identically with `fork/exec /home/tstapler/.local/bin/claude: no such file or directory`, which looked exactly like a broken claude-binary install.
**Impact**: Medium. Any backlog item with a non-absolute or non-existent `repo_path` silently and permanently fails triage with an error that misdirects debugging toward the wrong subsystem (binary installation) instead of the actual problem (bad input data). No data loss, but real, reproduced debugging-time cost: this looked like an environment problem until a manual `subprocess.run(cwd=...)` repro in Python was diffed against the Go behavior.

## Root Cause

`TriggerTriage`'s background goroutine (`server/services/backlog_service_triage.go`) passes `item.RepoPath` directly as `headless.CallOptions.WorkDir` with zero validation:

```go
raw, _, callErr := s.headlessPool.CallBlocking(triageCtx,
    headless.FeatureKeyTriage,
    headless.HeadlessTriageSystemPrompt(),
    triagePrompt,
    headless.CallOptions{WorkDir: itemRepoPath},
)
```

`WorkDir` ultimately becomes `exec.Cmd.Dir` for the `claude -p` subprocess. Go's `os/exec` has a well-documented quirk: when `Cmd.Dir` does not exist, the resulting `fork/exec` error names the **executable path**, not the directory — e.g. `fork/exec /home/tstapler/.local/bin/claude: no such file or directory`. This is textually indistinguishable from "the claude binary is genuinely missing," even though the binary is fine and the real problem is the bogus working directory.

`item.RepoPath` is stored as a bare string with no invariant enforced anywhere between where it's set (`CreateBacklogItem`/`UpdateBacklogItem`, RPC or MCP) and where it's used as a subprocess `Dir` (`TriggerTriage`'s goroutine). Nothing checked it was absolute, and nothing checked it existed. `classifyHeadlessCallError` (same file) has no bucket for this failure mode either — it fell into the generic `"other"` category, giving no signal that this was a distinct, systematic class of failure rather than a one-off.

## Fix

Added validation to `TriggerTriage` (`server/services/backlog_service_triage.go`), immediately after the existing `item.RepoPath == ""` check (step 3) and before any ItemSession or artifact-dir creation (step 3a onward):

```go
if !filepath.IsAbs(item.RepoPath) {
    return nil, connect.NewError(connect.CodeFailedPrecondition,
        fmt.Errorf("repo_path %q is not an absolute path", item.RepoPath))
}
if fi, statErr := os.Stat(item.RepoPath); statErr != nil || !fi.IsDir() {
    return nil, connect.NewError(connect.CodeFailedPrecondition,
        fmt.Errorf("repo_path %q does not exist or is not a directory", item.RepoPath))
}
```

This validates synchronously, in `TriggerTriage` itself — the single point every caller goes through:

- The RPC handler calls `TriggerTriage` directly.
- `MaybeTriggerTriage` (BUG-061's shared auto-triage gate for MCP-created items) calls `TriggerTriage` internally, so it inherits this check for free with no duplicated logic.
- Any future creation path that reuses `MaybeTriggerTriage`/`TriggerTriage` gets the same protection automatically.

A bad `repo_path` is now rejected with an accurate `CodeFailedPrecondition` error, synchronously, before any doomed goroutine, ItemSession, or artifact directory is created — the caller (RPC client, MCP tool, or `MaybeTriggerTriage`'s caller) gets an immediate, correctly-attributed rejection instead of silence followed by a misleading error 0-1s later.

**`classifyHeadlessCallError`**: not extended with a new bucket. Fail-fast validation makes this error class unreachable via the `CallBlocking` path it classifies — there is nothing left for a new bucket to catch. Prevention was preferred over classification, per the task's explicit guidance.

**MCP tool schema descriptions** (`server/mcp/tools_backlog.go`, `create_backlog_item` and `import_github_issue`): tightened `repo_path`'s description to explicitly require an absolute path and call out that a bare name or `owner/repo` shorthand will be rejected — the `create_backlog_item` schema's old wording ("Local filesystem path **or owner/repo**") actively suggested the exact malformed input that caused this incident.

**`session/headless/caller.go`**: added a doc-comment note on `CallOptions.WorkDir` documenting the `os/exec.Cmd.Dir` executable-misattribution quirk, pointing at this bug — a genuinely non-obvious Go gotcha worth flagging for any future `WorkDir`-bearing call site, independent of this specific fix.

Not attempted (explicitly out of scope): auto-resolving a bare repo slug into a real path by searching known repos/worktrees. Speculative scope creep — fail clearly instead, and let a human or calling agent supply the correct absolute path.

## Regression Tests

`server/services/backlog_service_triage_test.go`:
- `TestTriggerTriage_should_RejectRelativeRepoPath_Before_CreatingAnyItemSession` — a relative `repo_path` (e.g. `"stapler-squad"`) is rejected with `CodeFailedPrecondition` and an error mentioning "not an absolute path"; asserts zero ItemSessions were created and the headless pool was never called.
- `TestTriggerTriage_should_RejectNonExistentAbsoluteRepoPath_Before_CreatingAnyItemSession` — an absolute but non-existent path is rejected with "does not exist or is not a directory"; same zero-ItemSession/zero-pool-call assertions.
- `TestTriggerTriage_should_Succeed_When_RepoPathIsValidAbsoluteExistingDirectory` — regression guard: a valid absolute existing directory (`t.TempDir()`) still triages successfully and creates exactly one triage ItemSession.

`go test ./server/services ./server/mcp ./session` all pass; `go build ./...` and `make lint` are clean.

## Phase D — Classification (per `quality:reflect-and-fix`)

**Classification**: Type Safety Gap. `repo_path` is stored and threaded through the system as a plain `string` with no type or runtime invariant enforcing "must be an absolute, existing directory" anywhere between where a caller sets it and where it's consumed as a subprocess working directory. Any string value is accepted at every layer (storage, RPC, MCP tool) and the failure only ever surfaces at the one place — `exec.Cmd.Dir` — where the invariant actually matters, and even then with a misattributed error message.

**Earliest enforcement point**: A compile-time check can't express "this string must be an absolute existing directory" in Go without a wrapper type (e.g. a `ValidatedRepoPath` value object) that would need to be threaded through storage, proto, and every RPC/MCP boundary — disproportionate for a single string field with one real consumer of the invariant. The regression tests above (unit-level, at the one function that actually needs the invariant) are the earliest achievable enforcement point given that constraint, and doing the validation eagerly in `TriggerTriage` (rather than only checking right before the `CallBlocking` call inside the goroutine) closes the gap for the synchronous RPC/MCP-tool caller too, not just for the goroutine's own error message.

**Recurring shape**: None identified as a repeat of a previously-named pattern in this codebase's bug history — this is the first instance of "an unvalidated filesystem path reaches a subprocess's working directory with a misattributed OS error" here. Flagging the general shape for future awareness: any code path that takes a caller-supplied string and later uses it as `exec.Cmd.Dir` (or any other API with a similarly surprising failure-mode misattribution) should validate at the boundary, not rely on the subprocess call site's own error to be diagnosable. The `session/headless/caller.go` doc-comment addition above exists specifically so a future `WorkDir`-bearing call site doesn't have to rediscover this gotcha from scratch.

## Related

- BUG-061 (`docs/bugs/fixed/BUG-061-mcp-created-backlog-items-never-trigger-triage.md`) — introduced `MaybeTriggerTriage`, the shared auto-triage gate this fix's validation is inherited by for free.
- `session/headless/caller.go`'s `CallOptions.WorkDir` doc comment — now documents the `os/exec.Cmd.Dir` executable-misattribution quirk for future `WorkDir` call sites.
