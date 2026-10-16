# Stack research: confirm shipped fix's approach and dependencies (duplicate-verification mode)

No new stack is proposed — this backlog item's fix already shipped via PR #833
(`1cc66c53d`) and PR #837 (`ccbc1dba6`), both merged to `main` two days before this
triage. This document confirms the shipped approach and dependencies are sound, per
`sdd:2-research`'s Complexity-1 "verify, don't redesign" calibration.

## 1. `session/git/ops.go` base-branch resolution

Read in full (lines 1–260+): `ResolveDefaultBranchSHA`, `ResolveOriginBranchSHA`,
`ResolveWorktreeBaseCommit`, `AmbientHEADDivergesFromBase`,
`ResolveRemoteWorktreeBaseCommit`, plus their siblings `ResolveLocalBranchSHA`,
`ResolveDefaultLocalBranchSHA`, `FormatAmbientDivergenceWarning`,
`RemoteAmbientHEADDivergesFromBase`, `ResolveExplicitBranchSHA`.

- `ResolveWorktreeBaseCommit` (`session/git/ops.go:141`) tries origin's fetched
  default-branch tip first (`ResolveDefaultBranchSHA` → `ResolveOriginBranchSHA`,
  candidates `main`/`master`/`develop`/`trunk`), falls back to a local candidate
  branch tip if every origin fetch fails (offline/no-origin), and only falls back to
  ambient HEAD (`baseSHA == ""`) for a genuinely unborn repo (`IsUnbornRepo`) — the
  one case where no other branch can exist to misattribute work to. Any other
  resolution failure is a hard error, never a silent ambient-HEAD fallback.
- `AmbientHEADDivergesFromBase` (`session/git/ops.go:160`) is a pure comparison
  (`GetHeadCommitSHA` vs. `baseSHA`) used only to decide whether to surface a
  warning — it never gates the branch resolution itself.
- `ResolveRemoteWorktreeBaseCommit` (`session/git/ops.go:187`) mirrors the same
  fallback order and unborn-repo convention but through `tmux.CommandRunner.Run`
  over SSH, since go-git can't reach a remote host's filesystem directly.
- This exactly matches Issue 1's acceptance criteria: branch from
  `origin/<default-branch>` by default, fetching first, warn on ambient-HEAD
  divergence rather than silently substituting it.

## 2. `session/instance_worktree.go`'s `newWorktreeFromResolvedBase`

`newWorktreeFromResolvedBase` (`session/instance_worktree.go:219`) is the
resolve-then-construct entry point for `SessionTypeNewWorktree`:

1. Resolves the branch name (`git.ResolveBranchName`).
2. Resolves the true repo root (`ResolveMainRepoRoot`, falling back to `i.Path`).
3. Calls `git.ResolveWorktreeBaseCommit(resolvedRepo)`.
4. If `baseSHA == ""` (unborn repo), branches from ambient HEAD via
   `git.NewGitWorktreeWithBranch` — safe, no other branch exists.
5. Otherwise, checks `AmbientHEADDivergesFromBase`; if diverged, sets
   `i.CreationWarning` via `git.FormatAmbientDivergenceWarning` and logs a warning.
6. Constructs the worktree from the resolved `baseSHA` via
   `git.NewGitWorktreeFromCommitSHA` — never from ambient HEAD when a real base
   exists.

`i.CreationWarning` is set before the snapshot republish in
`setupFirstTimeWorktree` (`session/instance_worktree.go:81-89`), which re-locks
`i.mu`, rebuilds the snapshot, and stores it — consistent with the
`instance-lock-free-reads.md` project rule (write under lock, republish
`Snapshot()`, no separate unguarded read path introduced).

## 3. Terminal-output capture wiring

- `forwardOneControlModeFrame` (`server/services/connectrpc_websocket.go:1639`):
  after coalescing a frame batch and before sending it over the WebSocket, calls
  `h.scrollbackManager.AppendOutput(p.sessionID, buf)` (line 1666), best-effort
  (logs a warning on failure, never breaks live streaming). The comment at
  line 1661-1663 states directly that this used to be the gap: "AppendOutput was
  called only from tests, so run_command/read_session_output always read back
  empty regardless of what the session actually printed."
- `forwardCapturePaneOutput` (`server/services/connectrpc_websocket.go:3219`), the
  legacy `STAPLER_SQUAD_USE_CONTROL_MODE=false` capture-pane polling path, has the
  identical `p.scrollbackManager.AppendOutput(p.cpt.sessionID, []byte(fullContent))`
  call (line 3244) with an explicit comment cross-referencing
  `forwardOneControlModeFrame`'s — both live output paths are covered, not just
  the default one.
- `sessionNotReadyResult` (`server/mcp/tools_terminal.go:187`) checks
  `th.scrollback.CurrentSequence(sessionID) != 0`; returns a `SESSION_NOT_READY`
  error (`server/mcp/types.go:154`) when the scrollback sequence hasn't advanced
  since creation (no bytes ever arrived from the PTY/tmux stream yet), distinct
  from a command that legitimately printed nothing (which still advances the
  sequence via the shell's own prompt redraw). Shared by `readSessionOutput` and
  `runCommand` per its doc comment, so the two checks can't silently diverge.

This exactly matches Issue 2's acceptance criteria: real output is captured
(scrollback is actually written on both delivery paths), and "still initializing"
is now distinguishable from "ran and produced nothing" via `SESSION_NOT_READY`.

## 4. Git plumbing / library conventions

The shipped fix follows the repo's own conventions, verified against two written
project rules:

- **`prefer-go-git-over-subshells` skill**: local repo inspection in `ops.go`
  goes through `github.com/go-git/go-git/v5` (`OpenRepo`, `plumbing.NewRemoteReferenceName`,
  `plumbing.NewBranchReferenceName`) rather than shelling out — `ResolveOriginBranchSHA`,
  `ResolveLocalBranchSHA` both call `OpenRepo` + `repo.Reference(...)`, not
  `safeexec.CommandContext("git", "rev-parse", ...)`. The only subshell in this code
  path is `FetchBranch`'s `git fetch` (line 36), which the skill explicitly carves
  out as still-appropriate ("any operation needing a credential helper for
  push/fetch against a real remote") — go-git's own fetch support doesn't cover
  this case as cleanly as the CLI's credential-helper integration.
- `ResolveRemoteWorktreeBaseCommit` and `RemoteAmbientHEADDivergesFromBase` shell
  out via `tmux.CommandRunner.Run` (SSH), which is correct: go-git operates on a
  local filesystem `*git.Repository`, and these two functions are the SSH-remote
  counterparts by design (doc comments say so explicitly), not a case where go-git
  could have been used instead.
- **`.claude/rules/norawghrequest.md`** documents a parallel, but GitHub-REST-API-
  specific, "prefer the wrapper" convention (`NewConditionalRequest` etc. over raw
  `http.NewRequest`) — not applicable to git plumbing. It does, however, reference
  a sibling analyzer precedent: `tools/lint/norawgitopen` (confirmed present at
  `tools/lint/norawgitopen/`), which enforces "always open local repos via
  `session/git.OpenRepo`, never raw `go-git` `PlainOpen`/`PlainOpenWithOptions`."
  `grep -n "PlainOpen" session/git/ops.go` returns no matches — every local open in
  the resolution functions goes through `OpenRepo`, so the fix complies with this
  analyzer's rule too.

Conclusion: the shipped implementation is idiomatic for this codebase — go-git for
local reads, CLI subshell only for fetch/SSH-remote operations, consistent with
both the documented skill and the enforced `norawgitopen` lint rule. No stack
change is needed or suggested.

## 5. Post-fix history check — no residual/half-finished integration

```
git log --oneline -5 -- session/git/ops.go session/instance_worktree.go \
  server/services/connectrpc_websocket.go server/mcp/tools_terminal.go
```

```
65edb5c73 feat(git)!: remove native_git_worktree/native_git_merge feature flags (#849)
ccbc1dba6 [claudesquad] work complete for "new_worktree sessions branch from dirty local HEAD instead of origin/<default>, and terminal output capture returns empty for trivial commands" (pre-PR) (#837)
e2085dec4 fix(mcp): route steer_session/write_to_session/run_command through two-write submit (#832)
1cc66c53d fix(session): resolve new_worktree base branch from origin, wire scrollback capture (#833)
10d836911 feat(scroll): forward client scroll-up to Claude's alt-screen PageUp (#817)
```

One commit touched these files after the fix landed: `65edb5c73` (#849, "remove
native_git_worktree/native_git_merge feature flags"). Inspected via
`git show 65edb5c73 -- session/git/ops.go ...`: it only touches `ops.go`
(101 lines changed: 12 insertions, 89 deletions), and only two kinds of edits —

1. A doc-comment fix (`ResolveOriginBranchSHA`'s comment dropped a reference to
   the now-deleted `legacySetupNewWorktree`).
2. Deletion of the unrelated `legacyMergeMainIntoWorktree` subprocess-based
   implementation and its feature-flag dispatch in `MergeMainIntoWorktree`
   (a completely different function, used by branch-sync/PR-drift reconciliation,
   not worktree base-branch resolution).

It does not touch `ResolveWorktreeBaseCommit`, `AmbientHEADDivergesFromBase`,
`ResolveRemoteWorktreeBaseCommit`, `newWorktreeFromResolvedBase`, the
`AppendOutput` call sites, or `sessionNotReadyResult` at all. No revert, no
TODO, no half-finished integration point — #849 is unrelated cleanup (promoting
an already-fully-rolled-out native-git feature flag to unconditional) that
happened to touch the same file.

## Test evidence (re-run during this research, not just cited)

```
go test ./session/git/... -run "TestAmbientHEAD|TestResolveRemoteWorktreeBaseCommit|TestResolveWorktreeBaseCommit" -v
```

All 9 tests pass:
`TestResolveWorktreeBaseCommit_UsesOriginDefaultBranch`,
`TestResolveWorktreeBaseCommit_FallsBackToLocal_When_OriginFetchFails`,
`TestResolveWorktreeBaseCommit_ReturnsEmptySHA_When_RepoIsUnborn`,
`TestResolveWorktreeBaseCommit_ReturnsError_When_NoCandidateAndNotUnborn`,
`TestAmbientHEADDivergesFromBase_True_When_CheckedOutBranchDiffersFromBase`,
`TestAmbientHEADDivergesFromBase_False_When_AmbientHEADIsBase`,
`TestResolveRemoteWorktreeBaseCommit_MatchesLocalResolution`,
`TestResolveRemoteWorktreeBaseCommit_ReturnsEmptySHA_When_RepoIsUnborn`,
`TestResolveRemoteWorktreeBaseCommit_ReturnsError_When_RunnerFailsEntirely`.

## Bottom line

The shipped fix's technical approach is sound and matches project convention: no
stack changes, no leftover TODOs, no revert, no half-finished integration. This
supports the requirements.md conclusion that backlog item `4d856751-...` is a
duplicate of `c7466f05-...` and should be closed/merged into that item rather than
re-triggering implementation.
