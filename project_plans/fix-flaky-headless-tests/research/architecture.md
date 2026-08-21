# Research: Architecture — Agent 3

Scope: whether the fake-claude invocation pattern (clusters 1+2) is centralized
or duplicated and where the right fix seam is; whether fixing it can stay
test-only; and an independent architectural read on cluster 3's
exit-classification logic. This complements, and does not repeat, Agent 1's
`research/stack.md` (idiomatic-fix mechanics, platform causes) and Agent 4's
`research/pitfalls.md` (process pitfalls) — both already exist and were read
before writing this doc; conclusions below are cross-referenced against them,
not duplicated.

## 1. Is the write-script-then-exec pattern centralized or duplicated?

**Both — inconsistently.** Two independent things are conflated in the
requirements doc's framing and need to be separated:

**(a) Script *construction* (`os.WriteFile(path, script, 0o755)`) is NOT
centralized.** Three separate near-duplicate helper functions each inline
their own `os.WriteFile`:
- `writeCapabilityCheckFakeClaudeScript` — `session/headless/capability_check_test.go:20-27`
- `writeSleepForeverFakeClaudeScript` — `session/headless/pool_test.go:166-172`
- An inline write (no helper function) in
  `TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir` —
  `session/headless/pool_test.go:687-691`

`capability_check_test.go`'s helper doc comment (line 19) claims to "mirror
the pattern used in `session/review_gate_test.go`'s
`writeOccupyAwareFakeClaudeScript`" — that function **no longer exists** in
`session/review_gate_test.go` (grepped directly; no `0o755`/fake-claude script
writer remains there). This is stale doc rot, not load-bearing for the fix,
but worth a one-line cleanup while touching this file.

`caller_test.go`'s four `os.WriteFile(..., 0o755)` calls (lines 39, 86, 101,
105) are a **false positive** for this cluster — traced `caller.go`'s
`findClaudeBinary`/discovery path (`session/headless/caller.go:100-101`) and
confirmed it only does `os.Stat(candidate)` + `info.Mode()&0o111 != 0` to
*discover* a binary path; it never forks/execs the file. These tests exercise
binary-discovery logic, not subprocess invocation, so they are unaffected by
any OS exec-refusal and don't need to change.

**(b) `ProcessRunner` *construction* IS centralized, via `NewProcessRunnerForTesting`** —
`session/headless/fake_runner.go:152-154`, whose own doc comment explains why
it exists: `Pool.CallWithOptions`'s WorkDir one-shot path type-asserts on the
concrete `*ProcessRunner` type, rejecting `FakeRunner`, so any test exercising
real WorkDir/tool-access propagation must go through this constructor. It is
used at `capability_check_test.go:55,86,106,142` and `pool_test.go:183` — but
**not** at `pool_test.go:696`, which constructs `&ProcessRunner{claudeBin:
scriptPath}` directly, bypassing the shared seam. `pool_test.go:696` is
exactly `TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir`, cluster
2's named failing test — so the fix must either migrate this call site onto
`NewProcessRunnerForTesting` (or an interpreter-aware sibling) or the fix
seam won't actually cover it.

**Right fix point:** `NewProcessRunnerForTesting` in `fake_runner.go` is the
correct, minimal seam — change it once (or add a sibling constructor next to
it) and update the one outlier call site (`pool_test.go:696`) to use it, plus
have each of the 3 script-writing helpers stop depending on the exec bit
being honored. This is strictly cheaper than touching `ProcessRunner.Run` or
`executor.StartProcess` for every caller.

## 2. Does fixing this require touching production code?

Traced `ProcessRunner.Run` in full (`session/headless/runner.go:115-143`):

```go
func (r *ProcessRunner) Run(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, func() error, error) {
    args = append(args, r.toolAccessArgs()...)
    ...
    proc, err := executor.StartProcess(ctx, r.claudeBin, args, opts...)
    ...
}
```

`r.claudeBin` is passed as `executor.StartProcess`'s `name` argument, which
`managed_process.go:174` hands to `safeexec.CommandContext(derived, name,
args...)` — a thin wrapper (`executor/safeexec/safeexec.go:30`) around
`exec.CommandContext(ctx, name, arg...)`. This is a **direct fork/exec of
`name`**, not shell-mediated, and `args` here are the *claude CLI flags*
(`-p`, `--output-format`, `--allowedTools`, etc.) computed internally by
`Run`/`toolAccessArgs` — not raw shell-invocation arguments a caller controls.

**Consequence:** a caller cannot fix this by simply constructing
`ProcessRunner{claudeBin: "/bin/sh"}` today, because `Run` would then exec
`/bin/sh` with argv `["-p", "--output-format", "json", ...]` — `sh` would
misinterpret `-p` as its own flag, not run the script at all. There is
currently **no seam** in `ProcessRunner` to say "exec via this interpreter,
with the script path as argv[0] before the computed args."

**What this means for scope:**
- `executor.StartProcess`/`safeexec` genuinely need **zero** changes — they
  already do exactly what's asked of them (exec `name` with `args`); the gap
  is entirely in what `ProcessRunner.Run` chooses to pass as `name`.
- Closing the gap requires one small, additive, opt-in change to
  `ProcessRunner` itself (e.g., an unexported `interpreter string` field,
  populated only by a new/updated test constructor, defaulting to `""` for
  every real call site): when set, `Run` execs `r.interpreter` with
  `append([]string{r.claudeBin}, args...)` instead of exec'ing `r.claudeBin`
  directly. For every existing production caller (real `claude` binary,
  never test-constructed with an interpreter) this field stays `""` and
  behavior is provably unchanged — `git grep 'ProcessRunner{' session/headless/*.go`
  outside `_test.go` shows the only non-test constructor is
  `NewClaudeRunner`/equivalent production wiring, which never sets this
  field.
- This is technically a change to a file that also contains production code
  (`runner.go`), but the change is inert for production (default zero value)
  and only exercised by test constructors — matching the requirements doc's
  "test-only in effect" framing and its "no change to
  `executor.StartProcess`/`safeexec` beyond what's needed" scope boundary.
  Agent 1's `stack.md` independently proposes the same shape (§1, "thread an
  interpreter arg through `ProcessRunner`/`NewProcessRunnerForTesting`") —
  this confirms it from the call-graph side rather than the idiom side.

## 3. Cluster 3 — is the exit-classification logic architecturally sound?

Read `session/unfinished/gogitstore/mmap_truncation_test.go` in full (224
lines). Traced the control flow of `runTruncHelper` (lines 187-223), which is
the only thing that can run inside the re-exec'd subprocess for
`TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection`:

- Every path through the anonymous function at lines 204-221 ends by printing
  exactly one of two markers before returning: `NO_FAULT_OCCURRED sum=...`
  (line 220, no panic) or `ORDINARY_RECOVER_CAUGHT_IT: ...` (line 210, panic
  recovered — this branch, since `setPanicOnFault=false` for this test).
  `"SURVIVED"` (line 222) is then printed unconditionally after that function
  returns.
- A genuine hardware fault that kills the process (SIGBUS/SIGSEGV) never
  reaches either print statement — the OS terminates the process mid-fault.
  That case surfaces in the **parent** as `cmd.CombinedOutput()` returning a
  non-nil `err` (an `*exec.ExitError`), handled by the `err != nil` branch at
  line 127 — separate from the "clean exit, check markers" branch at 131-136.
- `cmd.CombinedOutput()`/`cmd.Wait()` guarantee the output-copying goroutines
  drain to EOF (`io.Copy` on the `os.Pipe()` read ends) before `Wait()`
  returns — so there is **no** race where the parent observes `err == nil`
  (clean exit) while stdout capture is incomplete. A signal-killed child by
  definition produces a non-nil `err`, which routes to the *other* branch.

**Conclusion on the specific scenario posed by the research question**
("clean exit, no markers, due to a stdout-capture/process-death race"): this
does not exist as a real gap. Every code path that leads to `err == nil` in
the parent has already, by construction, executed a `fmt.Println` that prints
one of the two markers before the process could exit(0). The `t.Fatalf` at
line 136 (unrecognized clean exit) is therefore dead code given the current
`runTruncHelper` implementation — not a live gap, but also not wrong to keep
as a guard against a *future* edit to `runTruncHelper` that adds a new
completion path without a matching marker (this is exactly the "bug-hiding
vs. legitimate tolerance" distinction Agent 4's `pitfalls.md` §3 makes: the
`t.Fatalf` is the guard-rail and must survive any change here).

**The real, narrower gap — already independently identified by Agent 1
(`stack.md` §2) and Agent 4 (`pitfalls.md` §2) — is different from what was
hypothesized above, and my read agrees it's the more actionable finding:**
line 127's `if err != nil { ...; return }` treats *any* non-nil error as
"the expected crash," without checking that the process actually died from
`SIGBUS`/`SIGSEGV` specifically. A subprocess that fails for an unrelated
reason (helper `-test.run` regex matched nothing, `git` not on `PATH` inside
the subprocess despite the outer `exec.LookPath` check, a `t.Fatal` inside
`buildTruncationFixtureHandle` before truncation even happens) also produces
a non-nil `err` today and gets silently accepted as if the crash had been
proven. `os.ProcessState.Sys().(syscall.WaitStatus)` — `.Signaled()` +
`.Signal() == syscall.SIGBUS || syscall.SIGSEGV` — closes this precisely, is
portable across Linux/macOS (unix build tag), and requires no change to the
tolerant two-marker branch at all. This is the concrete form AC3's "(b) fix a
genuine gap" branch should take; per Agent 4's `pitfalls.md` §4, this test
was already tightened once before for the *no-fault* tolerance
(`dccee742a`), so this signal-check tightening is a distinct, smaller,
first-time change, not a redundant third pass.

**Architectural verdict:** the marker/tolerant-outcome design (lines
132-135, 178) is sound and should not be touched. The one real, worthwhile
tightening is narrowing the `err != nil` branch to confirm the process was
actually killed by the expected fault signal, which the plan should scope as
a small, additive check — not a restructuring of the test's control flow.

## 4. Existing in-repo precedent for this class of problem

**No existing precedent for "invoke a script via `sh <path>` to bypass
exec-bit/OS restrictions."** Searched broadly (`grep -rn '"sh"\|"/bin/sh"'
--include='*_test.go'` across the whole repo, excluding stale
`.claude/worktrees/*` copies): every hit uses `sh`/`/bin/sh` as the program
being launched *itself* (e.g. `session/mcp_integration_test.go:159`,
`testutil/tmux_integration_test.go:32`,
`server/services/session_service_shells_test.go` — all spawn an interactive
shell as a tmux session's command), never as an interpreter wrapping a
separately-written script file. This fix establishes a new pattern for this
repo, not one that mirrors an existing idiom — worth calling out explicitly
in the plan/PR rather than implying it follows precedent.

**There IS a directly relevant precedent for the AC1 fallback
("clear skip/diagnostic instead of a confusing failure")**, missed by
Agent 1/4's searches: `server/services/backlog_triage_harness_test.go:242-259`,
`checkPoolStartAllowed`. It pre-flights a real subprocess start
(`exec.Command(truePath)` with `Setsid: true`), and on failure:

```go
if runErr := cmd.Run(); runErr != nil {
    if strings.Contains(runErr.Error(), "operation not permitted") ||
        strings.Contains(runErr.Error(), "permission denied") {
        t.Skip("subprocess with Setsid blocked by seccomp sandbox — run this test from a real terminal:\n\t...")
    }
    t.Fatalf("pool start pre-check failed unexpectedly: %v", runErr)
}
```

This is the exact idiom AC1's fallback describes — string-match the specific
OS-refusal error text, `t.Skip` with an actionable message pointing at how to
actually run the test, and `t.Fatalf` for anything else so a genuine
regression doesn't get silently swallowed as "probably sandboxed." Its own
comment (lines 239-241) documents that it was *narrowed* over time ("the
executor was fixed to skip Setpgid when Setsid is active, so this pre-check
now only fires in truly sandboxed environments") — i.e., this repo has
already been through one iteration of "diagnose vs. silently skip" for a
structurally similar EPERM-on-subprocess-start problem, and converged on
"skip with a specific, actionable message, guarded by a specific string
match, never a blanket catch-all." If the plan's primary fix (interpreter
invocation, §2 above) turns out infeasible for some call site, this file is
the concrete style reference to fall back to — not a fresh design.

## Synthesis for the plan (sdd:3-plan)

1. **Fix seam:** add an opt-in, unexported interpreter field to
   `ProcessRunner` (`session/headless/runner.go`), wired only through a new
   or updated constructor in `fake_runner.go` (extending
   `NewProcessRunnerForTesting` or adding a sibling e.g.
   `NewShellWrappedProcessRunnerForTesting`). Zero production behavior
   change (field defaults to `""`); `executor.StartProcess`/`safeexec`
   untouched, matching the requirements doc's scope boundary exactly.
2. **Call sites to update:** the 3 script-writing helpers
   (`capability_check_test.go:20-27`, `pool_test.go:166-172`,
   `pool_test.go:687-691`) plus the one outlier construction that bypasses
   the shared constructor (`pool_test.go:696` — must be migrated onto
   whichever constructor carries the interpreter change, not just left as a
   literal `&ProcessRunner{...}`).
3. **Fallback idiom**, if the interpreter approach proves insufficient for
   some call site: mirror `backlog_triage_harness_test.go`'s
   `checkPoolStartAllowed` — specific error-string match, `t.Skip` with an
   actionable message, `t.Fatalf` for anything unrecognized. Do not invent a
   new skip idiom when this one already exists and was itself refined once.
4. **Cluster 3:** no restructuring of the tolerant-outcome branches. Add a
   `syscall.WaitStatus`-based signal check to the `err != nil` branch
   (line 127) to confirm the death was actually `SIGBUS`/`SIGSEGV` before
   treating it as the expected crash, per Agent 1's proposed snippet in
   `stack.md` §2. This is additive and narrow — it does not touch the
   `t.Fatalf` guard-rail at line 136, which must survive unchanged.
5. **Doc-rot cleanup (opportunistic, not blocking):** the stale reference in
   `capability_check_test.go:19` to a `session/review_gate_test.go` helper
   that no longer exists there.

## Sources

All findings above are from direct reads/greps in this session:
`session/headless/{runner.go,fake_runner.go,capability_check_test.go,pool_test.go,caller_test.go,caller.go}`,
`executor/managed_process.go`, `executor/safeexec/safeexec.go`,
`session/unfinished/gogitstore/mmap_truncation_test.go`,
`server/services/backlog_triage_harness_test.go`,
`session/review_gate_test.go` (confirmed absence), plus repo-wide
`grep -rn '"sh"\|"/bin/sh"' --include='*_test.go'` (excluding
`.claude/worktrees/*`). Cross-referenced against
`project_plans/fix-flaky-headless-tests/research/stack.md` (Agent 1) and
`research/pitfalls.md` (Agent 4), both already written when this doc was
started.
