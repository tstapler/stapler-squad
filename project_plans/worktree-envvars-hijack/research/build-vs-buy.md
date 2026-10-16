# Regression Test & Tooling Strategy — worktree + envVars hijack bug

Research Agent 6 (repurposed from Build-vs-Buy to Regression-Test/Tooling Strategy),
SDD Phase 2. Root cause of the bug is not yet confirmed by parallel research agents;
this doc answers "once root cause is found, what's the best way to build a
deterministic regression test for this failure class using what's already in the
repo" — not "what caused it."

## 1. Existing `CreateSession` test coverage — what exists, what's missing

Two test files carry `CreateSession` tests, not one:
`server/services/session_service_test.go` (4898 lines, general lifecycle/pipeline
tests) and `server/services/session_service_create_test.go` (711 lines,
session-type-resolution and creation-validation focused). `SESSION_TYPE_NEW_WORKTREE`
only appears in the latter, and only in `resolveSessionType` unit tests
(`session_service_create_test.go:34` `TestResolveSessionType_ExplicitNewWorktree`) —
those test the *classification* of session type from the request, not what
`CreateSession` actually does with a worktree-typed request end to end.

**Confirmed gap:** `grep -n "GetEffectiveRootDir\|ActiveDir\|ExistingDir\|Workspace()"` across
both files returns zero matches. No existing Go test asserts on the resolved
`workingDir`/`activeDir`/`existingDir` after a `SESSION_TYPE_NEW_WORKTREE` creation.
This is exactly the gap the bug fell into — worktree path resolution is
tested for classification, not for the values consumers (`GetEffectiveRootDir`,
tmux session naming) actually see.

**Closest existing template — the pipeline-completion suite:**
`TestCreateSession_should_ReachActiveViaPipeline`
(`server/services/session_service_test.go:4277`) is the best extension point. It has
three subtests (`ModeIsDirectory`, `ModeIsOneOff`, `ModeIsRestart`) — **no
`ModeIsNewWorktree` subtest exists**, another confirmation of the gap, since this is
the one place in the suite that drives a *real* `runBackgroundResolutionPipeline` run
to completion and asserts the final `session.Active` outcome via
`svc.awaitCreationTerminal` (see `assertReachesActiveViaPipeline`,
`session_service_test.go:4716`). A `ModeIsNewWorktree` subtest, extended to also pass
`EnvVars` and assert on the resolved path fields, is the natural home for this
regression test — same fixture (`setupForkTestFixture`,
`server/services/session_service_fork_test.go:30`), same completion primitive, same
file.

**Closest existing envVars test — confirms the exact untested combination:**
`TestCreateSession_ThreadsEnvVars_WhenSetInRequest`
(`server/services/session_service_envvars_test.go:16`) is the only test that sets
`EnvVars` on a `CreateSessionRequest`. It uses `Program: "claude"` and
`Path: t.TempDir()` with `SessionType` left unspecified — which
`TestResolveSessionType_UnspecifiedDefaultsToDirectory`
(`session_service_create_test.go:51`) confirms resolves to `SESSION_TYPE_DIRECTORY`,
not `NEW_WORKTREE`. `t.TempDir()` also isn't a git repo, so worktree-resolution code
never runs. It asserts only `inst.EnvVars["FOO"]` — never `GetEffectiveRootDir()`,
`Workspace().ActiveDir`, or session status. **This test's shape is the closest
existing precedent, but it exercises neither `SESSION_TYPE_NEW_WORKTREE` nor a real
git repo, nor the resolved-path assertion — the bug's exact reproduction requires
combining this test's `EnvVars` setup with `assertReachesActiveViaPipeline`'s
worktree-mode gap above.**

## 2. Simulating "an already-running live session in the same directory" deterministically

The repo already has the primitive needed, and it's process-isolated by design:
`SessionService.testTmuxServerSocket` (`server/services/session_service.go:314-321`)
gives every `SessionService` created in a test process its own uniquely-named tmux
server socket (`test_server_services_<pid>_<counter>`,
`session_service.go:862`), applied to every `tmux.NewTmuxSessionWithServerSocket` call
that service makes (`session_service.go:2796`, `:1688`, `:1756`, `:2861`). This means:

- Tests use **real tmux**, not a mock — sessions are created/read through the actual
  tmux CLI — but on a private socket namespace, so two `SessionService` instances in
  the same test binary (or two parallel tests) never collide with each other or with
  a developer's real `:8543` tmux server. This is exactly the pattern needed: to
  simulate "an already-running live session in the same directory," a test can spin
  up **one `SessionService` fixture** (one `testTmuxServerSocket`) and create **two
  instances against the same repo path** within it — a pre-existing "live" instance
  (e.g. seeded directly via `fix.storage.AddInstance` + `addInstanceToPoller`, the
  pattern `TestCreateSession_should_ReachActiveViaPipeline`'s `ModeIsRestart` subtest
  already uses at `session_service_test.go:4328` for a *paused* source session) plus
  the `CreateSession` call under test — both landing on the same real (socket-isolated)
  tmux server, so any pane-content bleed-through is reproduced for real, not asserted
  away by a mock.
- This satisfies `deterministic-fast-tests`: no raw `sleep`/`time.Sleep`, no real
  network. Completion is observed via `svc.awaitCreationTerminal`
  (poll-based, `session_service_test.go:4723`) or the repo's shared
  `testutil/wait.RequireEventually` (`testutil/wait/eventually.go:20`), which scales
  its timeout to machine load and logs a halfway "still waiting" line instead of
  either a fixed sleep or a silent hang — this is the sanctioned polling primitive
  repo-wide, not a bespoke loop.
- Git repo fixtures are already standardized on `go-git` (per the
  `prefer-go-git-over-subshells` skill), not subprocess `git init`:
  `initGitRepoWithCommit(t, dir)` (`server/services/git_fixture_test.go:78`, wrapping
  `initGitRepoForTest`/`commitFileForTest`) is the existing one-call helper for "a
  real repo with one commit" — exactly what `SESSION_TYPE_NEW_WORKTREE` needs to
  actually exercise worktree creation instead of hitting an early validation error.

No new test-only tmux abstraction is needed — `testTmuxServerSocket` already *is* the
"simulate a live session safely" mechanism; it just isn't currently used by any test
that also passes `EnvVars` + `SESSION_TYPE_NEW_WORKTREE`.

## 3. E2E coverage — extend it, or is this a Go-level bug?

`tests/e2e/session-create-new-worktree.spec.ts` (81 lines) exists and is referenced
by `docs/reference/session-creation-registry.md`'s touchpoint convention, but it
currently tests only **UI form defaults** (`new worktree is the default selection`,
verified via `grep -n "^test("` — the file has no envVars-related assertions and
`grep -n "EnvVars|envVars"` returns nothing). It drives the Omnibar/creation-mode
picker, not backend pipeline state. Similarly,
`server/services/session_service_workspace_peers_test.go` (92 lines, 4 tests) is
about the *workspace-peers UI nudge* feature (warning a user a peer session exists in
the same repo) — adjacent in subject matter but not a test of `ActiveDir` resolution
or tmux pane content, and it also has no `EnvVars` coverage.

**Recommendation: this is fundamentally a Go/backend-level bug, not a UI bug**, and
should be caught there:
- The failure is entirely inside `runBackgroundResolutionPipeline`
  (`server/services/session_creation_pipeline.go:79`) and whatever downstream code
  resolves `workingDir`/`activeDir`/`existingDir` — async server-side state, with no
  UI decision point. `envVars` propagation itself is not even threaded through
  `session_creation_pipeline.go` (confirmed: `grep -n "EnvVars"` on that file returns
  nothing; it's set in `session_service.go` instead per the file list from
  `grep -rln "EnvVars" server/services/`) — so the interaction, whatever it turns out
  to be, crosses a code path boundary a Playwright test can't isolate or debug
  efficiently.
- Per this repo's own `docs/reference/session-creation-registry.md` "7 touchpoints"
  convention (cited in `CLAUDE.md`), a *new session-creation mode* needs E2E-level
  registry updates, but this bug is not a new mode — it's a regression in resolving
  an existing mode's fields, which is squarely inside the Go/integration test layer's
  job per this repo's existing test taxonomy (unit → `server/services` integration
  → E2E for UI-observable behavior only).
- An E2E test *could* eventually assert the symptom (wrong terminal content
  rendered), but it would be slow (real browser + real tmux + real async pipeline),
  flaky (must wait on the same async pipeline race an E2E harness has no direct hook
  into), and diagnostically weak (a failure would show "wrong text in pane," not
  "which field resolved wrong and why"). Not recommended as the primary regression
  test; at most a smoke-level addition to the existing
  `session-create-new-worktree.spec.ts` after the Go-level fix, low priority.

## 4. Recommendation: integration test in `server/services`, extending `TestCreateSession_should_ReachActiveViaPipeline`

**Level: integration test**, in `server/services/session_service_test.go`
(package `services`, run via `go test ./server/services -timeout=20m` per this
repo's Makefile) — not a `session`-package unit test, and not E2E.

Rationale:
- The bug requires a real `CreateSession` RPC call through the full
  `runBackgroundResolutionPipeline`, real tmux (socket-isolated), and a real git repo
  with worktree creation — none of which a `session`-package unit test in isolation
  exercises together; that's precisely the level this suite already operates at
  (`setupForkTestFixture`, `testTmuxServerSocket`, `initGitRepoWithCommit`).
- It is fast and parallel-safe at this level: `TestCreateSession_should_ReachActiveViaPipeline`'s
  existing subtests run with isolated ent-repository storage
  (`session.NewTestEntRepository(t)`) and isolated tmux sockets, so a
  `ModeIsNewWorktree` subtest added alongside `ModeIsDirectory`/`ModeIsOneOff`/`ModeIsRestart`
  costs about as much CI time as any other subtest here (bounded by
  `awaitTimeout = 30*time.Second`, typically resolves in tens of ms).
- Reserving E2E for this bug would make the regression test slow and would test the
  UI layer that isn't where the bug lives (see §3).

**Sketch (Arrange/Act/Assert), once root cause is confirmed — not full code:**

```
Arrange:
  fix := setupForkTestFixture(t)
  wireRegistryForActorSerialization(fix)
  repoDir := t.TempDir(); initGitRepoWithCommit(t, repoDir)   // git_fixture_test.go:78
  // Seed a pre-existing "live" instance in the SAME repoDir, registered with
  // fix.poller (addInstanceToPoller) and given a real tmux session on
  // fix.svc.testTmuxServerSocket, to reproduce "unrelated live session's pane
  // content" bleed-through per §2 above.

Act:
  resp, err := fix.svc.CreateSession(ctx, &CreateSessionRequest{
      Title: "...", Path: repoDir, Program: "claude",
      SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
      EnvVars: map[string]string{"FOO": "bar"},
  })
  assertReachesActiveViaPipeline(t, fix.svc, resp.Msg.Session, awaitTimeout, awaitPollInterval)

Assert:
  inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
  ws := inst.Workspace()
  assert.NotEqual(t, repoDir, ws.ActiveDir, "new-worktree session must not resolve to the bare repo path")
  assert.True(t, strings.HasPrefix(ws.WorktreeDir, ...), "must have an actual worktree dir")
  assert.Equal(t, "bar", inst.EnvVars["FOO"])
  // Plus whatever field root cause implicates in the tmux-session-naming/pane-target
  // path, asserted against the pre-existing live instance's session name to prove
  // no collision.
```

This mirrors `ModeIsRestart`'s existing pattern of seeding a second instance
(`session_service_test.go:4328`) and reuses every fixture/assertion primitive already
in the suite — no new test infrastructure required, only a new subtest.
