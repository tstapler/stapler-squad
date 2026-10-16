# Pitfalls / Residual-Gap Check — backlog 4d856751 vs. shipped fix (#833/#837)

Scope: verify whether the already-merged fix (PR #833, commit `1cc66c53d`; PR #837,
commit `ccbc1dba6`) fully satisfies the 5 acceptance criteria in
`project_plans/worktree-base-branch-and-capture-dup-check/requirements.md`, and
surface any edge case the fix leaves uncovered. No code was changed for this
research task.

## Summary verdict

The fix is **sound for its stated design** (auto-resolve origin default branch,
hard-fail loudly rather than silently branch off the wrong ref, warn on ambient
divergence) and AC1-AC4 hold for the common case. It is **not airtight** against
three edge cases in AC1/AC2's scope, and there is **one concrete, verifiable test
coverage gap** and **one confirmed-unverified manual-QA gap** worth recording before
closing this item as a duplicate.

## 1. Base-branch resolution edge cases (`session/instance_worktree.go`,
`session/git/ops.go`)

Traced `newWorktreeFromResolvedBase` (`session/instance_worktree.go:219`) →
`git.ResolveWorktreeBaseCommit` (`session/git/ops.go:141`) →
`ResolveDefaultBranchSHA` → `ResolveOriginBranchSHA`/`FetchBranch`, with fallback to
`ResolveDefaultLocalBranchSHA`, then `IsUnbornRepo`.

- **No `origin` remote at all**: `git fetch origin -- <candidate>` fails for each of
  `CandidateDefaultBranches` (`main`, `master`, `develop`, `trunk`,
  `session/git/ops.go:52`) exactly the same way an *unreachable* origin does (git
  can't distinguish "no such remote" from "remote unreachable" at this call site),
  so this collapses into the same code path as the next bullet. Not a distinct gap.
- **`origin` exists but is unreachable**: falls back to `ResolveDefaultLocalBranchSHA`
  (local refs only, no fetch). If the local repo happens to have a branch named one
  of the four candidates, resolution still succeeds (just possibly stale). If it
  doesn't, `ResolveWorktreeBaseCommit` returns a hard error
  (`session/git/ops.go:153`), which `newWorktreeFromResolvedBase` propagates as
  `"failed to create git worktree: resolve default branch: ..."`
  (`session/instance_worktree.go:230`) and **session creation fails outright** — no
  worktree, no fallback to ambient HEAD, no partial session left around. This is a
  documented, deliberate tradeoff ("wrong-but-loud failure beats a spawn silently
  branching from an unrelated branch," `session/instance_worktree.go:293`), and it
  **is** unit-tested at the `session/git` level —
  `TestResolveWorktreeBaseCommit_ReturnsError_When_NoCandidateAndNotUnborn`
  (`session/git/ops_test.go:458`) covers "unreachable origin + non-candidate default
  branch name" together. What's *not* tested is the higher-level consequence: no
  test asserts what `CreateSession`/`setupFirstTimeWorktree` actually returns to an
  MCP/API caller in this case (error shape, whether a partial `Instance` record gets
  left behind, whether the caller gets an actionable message). Latency: `FetchBranch`
  has its own hardcoded 30s timeout per candidate
  (`session/git/ops.go:28`, not caller-context-derived), so worst case this
  hard-fail path takes up to 4×30s=120s to surface. Whether that blocks the
  `CreateSession` RPC response or runs in an already-backgrounded "async start"
  path (`session/instance.go`'s `finishFirstTimeSetup`/`Start()`) was **not
  conclusively determined** in this pass — flag as inferred, not verified, if it
  matters to follow-up work.
- **Custom default branch name** (e.g. a repo whose real default branch is
  `development` or `release`, not in `CandidateDefaultBranches`): same hard-error
  path as above, **even when origin is fully reachable** — `ResolveDefaultBranchSHA`
  only ever tries the 4 hardcoded candidate names against origin
  (`session/git/ops.go:65`), never asks git for the repo's actual configured
  default/HEAD branch (e.g. `git remote show origin` or
  `symbolic-ref refs/remotes/origin/HEAD`). A `new_worktree` session against such a
  repo cannot succeed at all unless the caller has a local branch already checked
  out/fetched under one of the 4 candidate names. **There is no explicit
  base-branch override parameter for `new_worktree` session creation** — `i.Branch`
  in this path names the *new* branch to create (`git.ResolveBranchName`,
  `session/git/worktree.go:252`), not a base to fork from (unlike
  `CreateBacklogWorktree`'s `baseBranch` param, or `ResolveExplicitBranchSHA`). So
  AC1's "or at minimum ... require an explicit base-branch parameter" fallback arm
  was never implemented — the fix took the first arm (auto-resolve + warn) only,
  and for a custom-default-branch repo neither arm actually applies: there's no
  auto-resolution possible and no override escape hatch, just a hard failure with
  no workaround short of manually creating a local `main`/`master`/`develop`/`trunk`
  branch first. This is a real, narrow but concrete residual gap against AC1 as
  originally written — worth a one-line note if this item is merged into
  `c7466f05`, not necessarily a new bug (the repo's own dogfood case, `main`, works
  fine, and this tradeoff is intentional/documented — just not fully general).

## 2. Remote-target parity (`server/services/session_service.go`)

`CreateSession`'s remote block (`server/services/session_service.go:2719`) calls
`git.ResolveRemoteWorktreeBaseCommit`, which mirrors
`ResolveWorktreeBaseCommit`'s fallback order and unborn-repo convention exactly
(doc comment says so explicitly, `session/git/ops.go:184`), and
`RemoteAmbientHEADDivergesFromBase` mirrors the divergence-warning behavior via the
same shared `git.FormatAmbientDivergenceWarning`. Confirmed by reading both
implementations side by side — no asymmetry found. `baseSHA == ""` (unborn repo)
correctly omits the SHA arg from `git branch <name>`, which lets git default to
ambient HEAD, mirroring the local path's `git.NewGitWorktreeWithBranch` fallback.
Same "no explicit base-branch override, no non-candidate default-branch name
support" limitation applies identically on this path (same
`CandidateDefaultBranches` list, same hard error). Parity is good; the gap in §1 is
shared, not remote-specific.

## 3. Test run results

`go test ./session/git/... ./session/... -run "Worktree|Ambient|ScrollbackManager|NotReady"`
initially reported `FAIL ... [setup failed]` for several `session/...` subpackages
(`session`, `session/detection`, `session/queue`, etc.) — **not test failures**,
but missing generated code (`gen/proto/go/session/v1`, `session/ent/*`) because
`make build` hadn't been run yet in this worktree, per this repo's own
`CLAUDE.md` ("Build (generates protos)"). After running `make build`, a full rerun
of the same `-run` filter across `./session/git/...`, `./session/...`,
`./server/mcp/...`, and the specific remote-branch test in
`./server/services/...` **all passed**, including:

- `TestSetupFirstTimeWorktree_NewWorktree_BranchesFromOriginDefault_NotAmbientHEAD`
  and `TestSetupFirstTimeWorktree_NewWorktree_NoWarning_When_AmbientHEADMatchesBase`
  (`session/instance_worktree_test.go`, added by #833)
- `TestResolveWorktreeBaseCommit_*` (4 variants) and
  `TestAmbientHEADDivergesFromBase_*` (2 variants), `TestResolveRemoteWorktreeBaseCommit_*`
  (3 variants) in `session/git/ops_test.go`
- `TestReadOutputSessionNotReady`, `TestReadOutputSucceeds_When_ReadyWithNoNewBytes`,
  `TestSessionNotReadyResult` in `server/mcp/tools_terminal_test.go`
- `TestCreateSession_RemoteTarget_NewWorktree_BranchesFromDefaultBranch_NotAmbientHEAD`
  in `server/services/`

**What is NOT covered by any automated test found:**

- **`runCommand` (the `run_command` MCP tool) has zero direct test coverage.**
  Confirmed by grep: `server/mcp/tools_terminal_test.go` only *mentions*
  `runCommand` in two comments (lines 18, 239); no test ever calls
  `th.runCommand(...)`. The only tool actually exercised for the
  SESSION_NOT_READY/empty-output distinction is its sibling `readSessionOutput`.
  `runCommand` does call the same `sessionNotReadyResult` helper
  (`server/mcp/tools_terminal.go:617`), so the *shared* logic is indirectly
  covered via `TestSessionNotReadyResult`, but `runCommand`'s own poll loop
  (submit → poll checksum stability → read final output,
  `server/mcp/tools_terminal.go:543-646`) — the literal code path named in the
  original Issue 2 repro (`run_command echo alive-check`) — is untested.
- **No test exercises the actual production wiring** added in
  `server/services/connectrpc_websocket.go` (`forwardOneControlModeFrame` calling
  `scrollbackManager.AppendOutput` at line ~1666, `forwardCapturePaneOutput` at
  line ~3244) — confirmed via `git show 1cc66c53d --stat`: no
  `connectrpc_websocket_test.go` changes shipped with either PR. Coverage of "does
  live PTY/tmux output actually reach `ScrollbackManager`" is entirely indirect —
  `TestReadOutputSucceeds_When_ReadyWithNoNewBytes` simulates it by calling
  `mgr.AppendOutput(...)` directly from the test, bypassing the two real call
  sites the fix added. If either call site were ever removed/broken by a future
  refactor, no unit test would catch it — only e2e/manual testing would.
- **No Playwright e2e test** references `run_command`, `read_session_output`, or
  `echo alive-check` (`grep -rl` across `tests/e2e/` and `server/mcp/*_test.go`
  returned only the unit-test file above).

## 4. `instance_worktree_test.go` / `tools_terminal_test.go` scenario coverage
(what PR #833 actually added, per `git show 1cc66c53d --stat`)

`session/instance_worktree_test.go` (+87 lines) adds exactly two scenarios, both
with a plain `main` origin branch: (a) ambient HEAD on an unrelated diverged branch
→ asserts the worktree's `BaseCommitSHA` matches origin `main` and
`CreationWarning` is non-empty; (b) ambient HEAD already on `main` → asserts no
warning. Neither exercises: no-origin, unreachable-origin, or non-candidate
default-branch-name repos at the `Instance`/`setupFirstTimeWorktree` level (only
at the lower `session/git` unit level, per §1).

`server/mcp/tools_terminal_test.go` (+64 lines) adds
`TestReadOutputSessionNotReady` (scrollback sequence still 0 → expects
`SESSION_NOT_READY`) and `TestReadOutputSucceeds_When_ReadyWithNoNewBytes`
(scrollback already advanced, next read empty → expects normal success, not
`SESSION_NOT_READY`). Both operate against a `stubStore`/in-memory
`ScrollbackManager` with `AppendOutput` called manually by the test, not through
the real streaming path (see §3).

## 5. PR #837's test-plan checklist — confirmed unchecked, and unconfirmed elsewhere

Pulled via `gh pr view 837 --repo tstapler/stapler-squad --json body`. **All 8
items in the "Test plan" section are unchecked `[ ]`**, not just the two manual
ones as requirements.md flagged — including the automated `go test` invocations:

```
- [ ] go test ./server/mcp/... -run TestReadOutput
- [ ] go test ./server/mcp/... -run TestSessionNotReadyResult
- [ ] go test ./session/git/... -run 'TestAmbientHEADDivergesFromBase|TestResolveRemoteWorktreeBaseCommit'
- [ ] go test ./server/services/... -run TestCreateSession_RemoteTarget_NewWorktree_BranchesFromDefaultBranch_NotAmbientHEAD
- [ ] go test ./server/services/... -run TestCreateSession_RemoteTarget_ConnectionDropDuringCleanup_SurfacesOrphanWarning
- [ ] Manual repro of Issue 1: ... confirm the new worktree's git log matches origin/<default> and CreationWarning is populated
- [ ] Manual repro of Issue 2: ... run echo alive-check via run_command, confirm the echoed output is returned (not empty)
```

Cross-checked against `gh pr checks 837`: all CI jobs passed, including "Test
(affected packages only, fast signal)" — so the automated items almost certainly
did run and pass as part of normal CI, just never got their box ticked. But `gh pr
view 837 --json comments` returns only two bot comments (e2e-video and
feature-coverage bots) — **no comment from the author or anyone else confirms the
two manual-repro checkboxes were ever actually performed**, and nothing in CI could
have exercised them (they require a human clicking through the actual MCP tool
against a live session). By contrast, PR #833's own test-plan checklist is fully
checked `[x]` (`gh pr view 833`). **Net: the fix's automated coverage is real and
passes, but there is no evidence — checkbox, comment, or otherwise — that anyone
manually ran the two exact reported repros (Issue 1's ambient-divergence
create_session, Issue 2's echo alive-check via run_command) against a live tmux
session end-to-end**, which is exactly the gap §3 identifies at the test-suite
level too (runCommand and the real AppendOutput call sites are both only
indirectly covered).

## Recommendation

Nothing found here rises to "must re-open and re-fix before closing as
duplicate" — the design is intentionally conservative (loud failure over silent
wrong branch) and the core repro scenarios (AC1-AC4) are genuinely fixed and
automated-tested. Two follow-ups worth noting in the plan/validation phase rather
than blocking closure:

1. (Low-priority, separate from this duplicate) Add one `runCommand`-level test and
   one true end-to-end test (real tmux session, real `AppendOutput` call site, not
   a stub) so a future refactor of `connectrpc_websocket.go`'s two call sites would
   be caught automatically instead of only by manual QA.
2. (Optional, only if a custom-default-branch repo is a realistic case for this
   project) `ResolveDefaultBranchSHA`/`ResolveRemoteWorktreeBaseCommit` could query
   the repo's actual configured default branch (`git symbolic-ref
   refs/remotes/origin/HEAD` or `git remote show origin`) instead of only trying 4
   hardcoded names — or `new_worktree` session creation could gain an explicit
   base-branch override, mirroring `CreateBacklogWorktree`'s `baseBranch` param.
