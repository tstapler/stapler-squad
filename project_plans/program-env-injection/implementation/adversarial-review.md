# Adversarial Review: program-env-injection

**Date**: 2026-10-10
**Verdict**: BLOCKED
**Reviewed**: `implementation/plan.md` against `requirements.md` and HEAD `9ef8fbc68` (worktree opened, symbols grepped, three experiments run; see "What I executed").

## Blockers

- [ ] **B1. The plan's own final gate (Task 1.4.2b `make lint`) cannot pass, and no task fixes the cause.** The already-landed `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` violates two blocking custom analyzers. VERIFIED: I built `tools/lint/cmd/linter` and ran it on `./server/services`; it reports 5 findings, all in `server/services/session_service_create_test.go`:
  - `norawexec` at lines 729, 738, 741 (`exec.CommandContext`; must be `safeexec.CommandContext`).
  - `notimesleeptest` at lines 718 and 740 (`time.Sleep`; ADR-003).
  `make lint` depends on `lint-custom` (Makefile:798), and `.github/workflows/lint.yml:217` runs `make lint-custom` over `./...` (whole repo, not new-code-only). The same run over `./session` and `./session/tmux` is clean, so the branch's own test is the only offender. Plan impact:
  - Task 1.1.1c replaces only the line-718 sleep; the 300 ms `time.Sleep` at line 740 is explicitly kept ("Keep the send-keys retry loop as is"), and the three raw `exec.CommandContext` calls are never mentioned.
  - Task 1.1.3c says "reuse ... " and copies the same tmux-driving pattern, which will add more `norawexec` hits unless it uses `safeexec`.
  - Task 1.4.2b's `Verify: every command exits 0` is therefore unsatisfiable as written, and AC3 asks for a *committed* regression test that must survive CI.
  Fix: add a task (before 1.1.1c) that converts the three raw execs to `safeexec.CommandContext` and replaces the 740 send-keys retry with `wait.RequireEventually` (side effect per tick is fine inside the condition), and state in 1.1.3c that every new subprocess call uses `safeexec` and no `time.Sleep`. Make `bin/linter ./server/services ./session ./session/tmux` (or `make lint-custom`) an explicit verify line for each test-writing task, not only at the end.

## Concerns

- [ ] **C1. The `cdfd4e5cf2^` rationale in Pattern Decisions is contradicted by evidence, and AC3's literal wording ("fails on the pre-fix commit") is only met synthetically.** The baseline commit itself is right: `cdfd4e5cf2^` = `4dbbe7b40` (a mainline CI-trend chore commit); `git show cdfd4e5cf2^:session/instance_tmux.go` has `if i.UUID != "" { session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID}) }` at lines 578-580 and no `buildExtraEnv`/`resolveExtraEnvVars`/`wireTmuxSession`. That matches your own run (fails on `show-environment` with only `STAPLER_SESSION_UUID`). But the plan rejects "checking out `cdfd4e5cf2^` and copying the test in" because the parent "cannot host today's test helpers"; you did exactly that and got an assertion failure. Fix: keep the overlay as the primary, cheap, repeatable proof, and add your parent-commit run as corroborating evidence E4b (command, `git rev-parse cdfd4e5cf2^`, verbatim failure), and delete the "cannot host" claim.
- [ ] **C2. Attribution is imprecise: the fix was not all in `cdfd4e5cf2`.** At `cdfd4e5cf2` only `buildExtraEnv` (with an inline program-env merge) and `wireTmuxSession` exist. `resolveExtraEnvVars` and `claudeSettingsEnvOverrideArgs` arrive in `5da2af7bf` (#852, 2026-09-23), which is NOT an ancestor of `cdfd4e5cf2`. Header, Glossary and AC5 text lump the three together; AC5's `git show cdfd4e5cf2^` check is still correct, but name `5da2af7bf` so a reviewer searching the PR #825 squash (a tagging-classifier PR, 53 files) knows where the AC4 half came from.
- [ ] **C3. AC4 is only partly provable and the traceability table does not say so.** Task 1.1.3c/d prove `--settings <env JSON>` reaches the process argv through real tmux and a real shell. "Program env still wins over a global `~/.claude/settings.json` `env` block" is upstream Claude behaviour: nothing in 1.1.3a-d executes it, and E7 may end as `UNVERIFIED`. Mark AC4 in the Traceability table as "flag delivery VERIFIED; precedence = quoted upstream doc / UNVERIFIED" instead of a flat tick, so closing the AC does not overclaim.
- [ ] **C4. Real-tmux test verification omits how CI actually runs them.** `.github/workflows/build.yml` runs `-race -short` with `TMUX_BIN=bin/tmux` under a shared package run. Verify lines for 1.1.1c/1.1.1d/1.1.3c use neither `-race` nor consider `-short`. Add `-race` to the E2 `-count=5` run and to 1.1.3c's verify, and have 1.1.3c follow the existing LookPath-skip convention. In Task 1.2.2a, the `MockCmdExec` closures that record argv are invoked from helper goroutines; guard the captured slice with a mutex (the `ownerMismatchFixture` bools it is modelled on are unsynchronised and only get away with it).
- [ ] **C5. Parallelism claims are wrong at package granularity.** Wave 1 says "disjoint files; run in parallel", but 1.2.1a and 1.2.2a share package `session/tmux`; 1.3.1a and 1.3.2a/b share package `session`; 1.1.3c shares `server/services` with the Wave-2 edits. One agent's half-written `_test.go` fails `go test` for every other agent in that package in the same worktree. Conversely Wave 3 is "serial because the overlay runs share the tree", but `-overlay` never touches the tree (verified: `git status` stayed clean). Fix: one agent per package, or per-agent worktrees; drop the wrong Wave 3 justification.
- [ ] **C6. Scope: AC1-AC5 are fully served by Epic 1.1 plus the lint fix; the other epics have no AC.** Epic 1.2 (two new files in `session/tmux`), Story 1.3.1, 1.3.2 (3 tests), 1.3.3 (web), 1.4.1 and the F1-F5 write-up are "supporting (no AC, from research)" per the plan's own table, ~half of the ~1.3M CU. Given the owner's stated "pursue full scope" preference this is not a blocker, but the work is low marginal value for a bug whose fix already shipped:
  - 1.3.2a's first test (instance `EnvVars` beats program env) restates `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` (CLASH case, `instance_tmux_test.go:1202`) with extra setup.
  - 1.3.2b pins "`EnvVars` is not persisted", which also means request-level `env_vars` are silently lost across a server restart; label it as characterization of a gap in the test name/comment, not a guarantee.
  - 1.4.1a's experiment is already answered: I ran `tmux -L <private> -f /dev/null new-session -d -s dup -e K=first -e K=second 'sleep 30'` on tmux 3.6a and `show-environment -t dup K` printed `K=second` (later wins). Record that and drop the task or keep it as a 1-minute evidence paste.
  Recommend landing Epic 1.1 (AC evidence + lint fix) as its own commit so supporting tests can be rejected without touching the AC proof.
- [ ] **C7. PIT-G "rc-file masking" justification for Task 1.1.1b is speculative.** The pane in the overlay-red run printed `ENVPROBE__END` for `ANTHROPIC_BASE_URL` on this machine (no rc masking). The ProbeKey is cheap and harmless, but the plan presents it as fixing a demonstrated flake; say "defensive" or cite an actual occurrence.
- [ ] **C8. `make ready-complexity-gate`/`dupl` is a non-issue for the new Go tests, and the plan says otherwise.** `.golangci.yml:244-246` excludes `_test.go` from `gocyclo,gocognit,funlen,revive,dupl`. Task 1.4.2b's "fix any table-test duplication by extracting a helper" is moot for Go tests. The gate that can bite is `jscpd` for the added Jest `it` (threshold ratchet noted in CLAUDE.md); say that instead.

## Minors

- Line-number drift in cited "VERIFIED" references: the inline `-e` loop in `start()` is `tmux_session_start.go:222-227` (plan: 225-230); `newSessionArgs` is declared at `:637` (plan: `:638`); `instance.go` also appends to `ExtraEnv` at `1952,1959,2053,2060` (plan lists only 1641/1648/1785/1792); `instance_adapter.go:124` is actually `server/adapters/instance_adapter.go:124`. Correct ones, checked: `instance_tmux.go` 239/433/710/729/756/771/806, `tmux.go:1238`, `defaults_service.go:709`, `config/defaults.go:211`, `session_service_create_test.go:673`, `session_service_create.go:282`, `ssh_runner.go:601`.
- Task 1.1.3c's Unresolved Question (does `CreateSession` accept a fake `claude` path?) is now answered; see experiment 3 below. Close it, and note the expected `ERROR claude launch: MCP server URL unresolved` log is harmless in this test setup.
- Task 1.3.2a: `seedCustomProgram` calls `t.Setenv(STAPLER_SQUAD_TEST_DIR, t.TempDir())` each time; calling it twice silently swaps in a fresh empty config rather than editing the first. Say "load/modify/`SaveConfig`" explicitly.
- Task 1.1.2a edits the copy's `wireTmuxSession`; the pre-fix shape reads `i.UUID` raw. Fine in a scratch overlay, but add "scratch only; production must keep `Snapshot()` per `.claude/rules/instance-lock-free-reads.md`" so nobody pastes it back.
- Unresolved Question 4 (FakeClaude) and Question 5 are the only ones marked blocking an implementation task; Q1-Q3 correctly do not.

## Answers to the specific probes

1. **AC1-AC5 with verifiable commands**: AC1/AC2 yes (E1/E2, 1.1.1b-d). AC3 yes via overlay (E3-E5), mechanism verified, see C1 for wording. AC4 partial (C3). AC5 yes (`git show cdfd4e5cf2^` check + `requirements.md`).
2. **Scope drift / deferrals**: nothing deferred is required by an AC. F1-F5 are all out of AC scope. Drift is on the planned side (C6-C7). One unjustified omission: the lint violations in the landed test (B1).
3. **Unverifiable as written**: Task 1.4.2b (B1); Task 1.1.2b's "FAIL naming `tmux show-environment must carry the program's env`" is verifiable and was reproduced.
4. **Flake risk**: C4, C5, and `RestoreWithWorkDir` recreate-path test costs about 1.5 s of `probeSessionExistsWithRetries` backoff (100+200+400+800 ms) with a mock executor; acceptable but note it.
5. **Repo-rule violations by planned production changes**: none, because there are no production changes. Test code reads via `Snapshot()` where it reads instance fields. The violations are of the *test* rules (`norawexec`, `notimesleeptest`; B1).
6. **`cdfd4e5cf2^` baseline**: correct commit and confirmed shape (C1, C2).

## What I executed

1. Overlay AC3 mechanism: copied `session/instance_tmux.go` to scratch, replaced the `buildExtraEnv()`/`SetExtraEnv` block in `wireTmuxSession` with a UUID-only `SetExtraEnv`, `go build -overlay` OK, then `go test -overlay <json> ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1` FAILED with `show-environment` output `CLAUDECODE=, DISPLAY, ..., STAPLER_SESSION_UUID=...` and message `tmux show-environment must carry the program's env`, plus the in-pane `ENVPROBE__END` failure. `git status --short` showed only the untracked plan directory.
2. `go -C tools/lint build ./cmd/linter` then `linter ./server/services ./session ./session/tmux`: 5 findings, all in `session_service_create_test.go` (B1).
3. Fake `claude` through `CreateSession`, via an overlay-added scratch test: `UpsertProgramConfig` with `Command=<tmp>/claude` and env `ANTHROPIC_BASE_URL` plus a hostile value (`'`, `$(touch ...)`, backticks, `"`, `=`), `SESSION_TYPE_NEW_WORKTREE`. Result: status `Active`; recorded argv = `--settings` then `{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:47000","SSQ_HOSTILE":"it's $(touch .../pwned) `touch .../pwned` \"q\" a=b"}}`; `pwned` absent; `show-environment` contains both keys. Task 1.1.3c is feasible as designed.
4. Duplicate `-e` precedence on tmux 3.6a (private socket, killed afterwards): `K=second` (later wins).

## Re-review (iteration 1)

**Scope**: B1 only, plus the plan.md "Repair log (iteration 1)" for newly introduced blockers.
**Verdict**: CLEAN (B1 resolved on paper; no new BLOCKERs). Two non-blocking notes below.

### B1 verification (VERIFIED by running/reading, not by trusting the log)

- Baseline count is real: built `tools/lint/cmd/linter` into the scratchpad (`go -C tools/lint build -o <scratch>/linter ./cmd/linter`, the Makefile's `$(LINTER_BIN)` recipe, `Makefile:819-821`) and ran it on `./server/services ./session ./session/tmux`: exit 3, exactly 5 findings, all in `server/services/session_service_create_test.go`: `norawexec` at 729:14, 738:7, 741:17; `notimesleeptest` at 718:8, 740:8. Matches Story 1.1.0's AC verbatim.
- Conversion is expressible:
  - `safeexec.CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd` (`executor/safeexec/safeexec.go:30`) is signature-identical to `exec.CommandContext`, so `.CombinedOutput()/.Run()/.Output()` call sites change only the qualifier. `os/exec` must stay imported for `exec.LookPath` (line 6); the plan says so. `safeexec` is not yet imported in this file (sibling `connectrpc_websocket_test.go:24` imports it), so the task must add the import; the plan's wording ("already imported by sibling tests") is accurate but the edit list should include it. `norawexec` flags only `Command`/`CommandContext`, not `LookPath`.
  - `wait.RequireEventually(t testing.TB, condition func() bool, baseTimeout, tick time.Duration, msgAndArgs ...any)` (`testutil/wait/eventually.go:20`) runs `condition()` synchronously on the caller's goroutine (`pollEventually`, same file), so the original comment's objection (testify `Eventually` runs the condition in a goroutine, breaking `t.Skip`/`t.Fatalf`) does not apply; `t.Fatalf` on `Stopped` inside the condition is legal. `testutil/wait` is already imported (line 23). Timeout failure goes through `t.Fatalf` rather than `require.Equal`, so the custom message changes; harmless.
  - Side effects per tick (send-keys then capture-pane) are fine; it replicates the original re-send-until-visible loop, minus the fixed 300 ms sleep. `notimesleeptest` accepts the helper; `time.Sleep` is the only trigger (`tools/lint/notimesleeptest/analyzer.go:63-65`).
- The verify command is the real one: `go -C tools/lint build -o "$(pwd)/bin/linter" ./cmd/linter && bin/linter ./server/services ./session ./session/tmux` mirrors `Makefile:814-821`; `bin/` is gitignored (`.gitignore:107`), so no stray artifact. `make lint-custom` runs `./...` and CI uses the same target, as the plan states.
- Downstream tasks now carry the rule (1.1.3c, 1.2.x, 1.3.x) and 1.4.2b no longer claims an unsatisfiable green; the critical path (1.1.0a -> 1.1.1b -> 1.1.1c -> 1.1.2a-c -> 1.4.2b) is consistent with the repair.

### Repair-log check for new BLOCKERs

None. C1 (E4b: `cdfd4e5cf2^` = `4dbbe7b40`, re-confirmed with `git rev-parse`), C2, C4, C5 (per-package waves), C8 are reflected in the plan body as the log says.

### Non-blocking notes

- N1. Task 1.1.0a and Task 1.1.1c both claim the line-~718 Active-poll replacement (1.1.0a bullet 3 says "see Task 1.1.1c"; 1.1.1c says "same agent as 1.1.0a"). Same agent, same file, so harmless, but state in 1.1.0a that 1.1.1c does the Active poll and 1.1.0a does only the execs plus the send-keys loop, so the linter-green verify in 1.1.0a is not run with line 718 still a `time.Sleep` (it would fail its own `bin/linter` verify as written until 1.1.1c lands). Cheapest fix: have 1.1.0a also do the Active poll and make 1.1.1c reference it.
- N2. Task 1.1.0a's edit list should name the `safeexec` import addition explicitly.
