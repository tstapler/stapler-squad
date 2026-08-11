# Research: Pitfalls (Agent 4)

Research question: what commonly goes wrong when teams "fix" this exact class
of problem (platform-only-reproducible flake; subprocess-crash-signal test;
loosening an assertion), so the plan avoids repeating known mistakes.

## 1. Fixing a failure you cannot reproduce on your own machine

**The core risk:** any fix for clusters 1+2 written and "verified" in this
Linux session is verified only by inspection/reasoning, not by observing the
failure go away. That is a real category of false-confidence bug — the fix
can look right, pass on Linux, and still not touch the actual macOS failure
mode, because the Linux run never exercised the code path being fixed
(fork/exec of a freshly-written temp script never gets refused on this
machine in the first place).

**Confirmed and load-bearing for this plan:** this repo's CI has **zero
macOS runners**. `grep -rn "runs-on" .github/workflows/*.yml` shows every
workflow (`build.yml`, `lint.yml`, `benchmark.yml`, `e2e-video.yml`, etc.) is
pinned to `ubuntu-latest` or `ubuntu-22.04`. There is no CI matrix leg that
would ever catch a regression in, or validate a fix for, a macOS-only exec
failure — which is the well-run-project mitigation (a macOS runner in the
matrix) this repo does not have. That means:
- A merged "fix" for clusters 1+2 can silently be wrong indefinitely; CI will
  stay green on Linux either way.
- The only verification loop available is a human running the specific
  cluster 1/2 tests on a real macOS machine, by hand, after merge — this
  MUST be an explicit follow-up task (already anticipated in requirements.md
  acceptance criterion 6), not an implicit assumption that "tests pass in CI"
  means the platform-specific bug is fixed.

**Well-run-project patterns to borrow, even without adding a macOS CI leg
(out of scope per requirements.md's "no change to executor beyond what's
needed" boundary — but worth naming as options for the plan to consider or
explicitly defer):**
- Prefer a fix that is *correct independent of whether Gatekeeper/EDR is
  actually the cause* — e.g., invoking the fake-claude script through an
  already-trusted interpreter (`sh <path>`) is safe/no-op on Linux (where the
  hypothesis doesn't reproduce) and only matters if the macOS hypothesis is
  right. This is a strictly-safer fix shape than one that changes behavior
  based on `runtime.GOOS`, because a GOOS-conditional fix path is *itself*
  something that can only be verified on the platform it targets.
- If the exec truly can be refused by something outside the program's
  control, make the failure mode a **clear, named skip/diagnostic**
  (`t.Skipf("fake-claude exec refused: %v — likely OS-level exec restriction, not a code bug", err)`)
  rather than a confusing downstream symptom (cluster 1's "cached result
  stuck at false" — a caching-logic-shaped symptom for an exec-permission
  problem). A future person hitting this on their own Mac should see "exec
  was refused" immediately, not have to re-derive this session's whole
  root-cause chain from a generic assertion failure.
- Don't claim cluster 1/2 are "fixed" in the PR description — claim they are
  "hardened against the identified failure mode, unverified against a live
  recurrence" and say explicitly where/how a maintainer confirms it (this
  matches requirements.md acceptance criterion 6, which already anticipates
  this and should be treated as non-negotiable, not aspirational).

**One nuance worth flagging for the plan:** the search results below
(`golang-nuts` threads, `evilmartians/lefthook#157`) attribute most
"operation not permitted" fork/exec failures on macOS to the
`com.apple.quarantine` extended attribute — which is normally only set by
apps that go through the macOS quarantine-aware download path (Safari, Mail,
curl with `--xattr`, etc.), **not** by a plain Go `os.WriteFile`. A freshly
`os.WriteFile`'d temp script in `t.TempDir()` typically does **not** carry
`com.apple.quarantine`. That doesn't invalidate the Gatekeeper/TCC hypothesis
in requirements.md, but it does mean "Gatekeeper" may be too narrow/specific
a label — the actual blocker on the reporting user's Mac could equally be
endpoint security/MDM software (CrowdStrike, Jamf Protect, similar) that
intercepts exec() calls for newly-created executables in `$TMPDIR`,
independent of quarantine xattrs. **Pitfall: don't let the fix's diagnostic
message or doc comment overclaim a specific mechanism ("Gatekeeper blocked
this") when the evidence only supports a broader claim ("the OS or an
OS-level security agent refused to exec a freshly-written script").** Say
"an OS-level exec restriction (Gatekeeper, TCC, or third-party endpoint
security software)" rather than asserting the specific culprit as fact.

## 2. Pitfalls in the subprocess-crash-signal test itself (cluster 3)

- **Go's re-exec-self pattern is a known, standard technique** (see
  `rednafi.com`'s "Re-exec testing Go subprocesses" writeup and the pattern's
  origin in stdlib's own `os/exec` test suite) — `os.Args[0]` +
  `-test.run=^TestName$` + an env var sentinel to select the helper-process
  branch is exactly what `mmap_truncation_test.go` already does
  (`GOGITSTORE_TRUNC_HELPER=1`, lines 114-137 and 160-182). This is not a
  fragile pattern by itself, but it has known sharp edges:
  - `cmd.CombinedOutput()` interleaves stdout/stderr by write order, which is
    fine for this test's `bytes.Contains` marker checks (order-independent),
    but would be a real hazard if a future edit tried to assert on exact
    output *ordering* between the two streams — signal-crash output (the
    runtime's own fatal-signal traceback to stderr) can race arbitrarily
    against the test's own `fmt.Println` calls to stdout.
  - The exit-code/error path (`err != nil` after `CombinedOutput()`) does not
    by itself distinguish "process was killed by SIGBUS/SIGSEGV" from "the
    subprocess's own `-test.run` regex matched nothing" or "the git binary
    genuinely wasn't found inside the subprocess's environment" — today the
    test treats *any* non-nil `err` as "expected — this IS the point of the
    test" (`mmap_truncation_test.go:127-129`). That's a real (if narrow)
    over-broad accept-set: a subprocess that fails to even start the intended
    helper branch would also produce a non-nil `err` and get logged as if the
    crash had been proven, when nothing was actually demonstrated. Look at
    whether tightening this (e.g., checking `cmd.ProcessState.Sys()` for an
    actual signal, on platforms where that's available) is in scope, or
    explicitly document why it isn't (subprocess helper-launch failures would
    almost certainly also produce a distinguishing stderr message from `go
    test` itself, so this may be low-value to tighten — but that's a
    judgment call the plan should make explicitly, not silently skip).
- **`runtime/debug.SetPanicOnFault` is not airtight across all fault types
  either** — [golang/go#41155](https://github.com/golang/go/issues/41155)
  ("runtime: process crash instead of panic on SIGBUS with
  SetPanicOnDefault(true)") documents a real case where `SetPanicOnFault`
  itself failed to convert a SIGBUS into a recoverable panic and the process
  crashed anyway. This directly reinforces why the *paired* WITH-protection
  test (`TestMmapIndexHandle_TruncateWhileMapped_RecoverableWithProtection`)
  already tolerates its own crash outcome (lines 173-176: "this can happen if
  ... rather than the protection failing") — that tolerance is itself
  correct, not a shortcut, and should not be "tightened" without addressing
  this upstream Go issue's finding first.
- **This is genuinely the class of test the Go community has flagged as
  inherently flake-prone**: [golang/go#62244](https://github.com/golang/go/issues/62244)
  ("cmd/go: add support for dealing with flaky tests") notes that the tests
  most prone to flakiness "involve timeouts and the network and subprocesses"
  — this test is a subprocess test whose entire premise is triggering
  platform/kernel-dependent undefined behavior, which is close to the
  worst-case combination for determinism. That is exactly why requirements.md's
  framing (cluster 3 is "self-documented platform-dependent UB the test
  already tolerates," not a bug) is the right frame, and a pitfall for this
  plan is chasing perfect determinism here at all — that goal is unreachable
  by the nature of what's being tested, and the existing tolerant-outcome
  design is the correct response to that fact, not a compromise to fix.

## 3. Pitfalls in "loosening" an assertion — how to tell legitimate tolerance from bug-hiding

`.claude/rules/fix-flaky-tests-dont-defer.md` is explicit that re-excusing
flakes as "known, unrelated" without fixing them is the anti-pattern this
repo has fallen into before, and names this exact test
(`TestRemoveHooksConfig_...` is the other repeat offender cited, but the
rule's spirit applies here too). The line between legitimate widening and
bug-hiding, applied to this test:

- **Legitimate** (already present, lines 132-135, 178): the widened
  accept-set is *documented with a doc comment stating exactly which
  platform variance it tolerates and why* ("this run's specific
  truncation/read pattern happened not to fault at the OS level
  (platform/kernel-dependent)"), it's scoped to the *specific* two outcomes
  that are physically possible given the operation being performed (a fault
  either occurs or it doesn't — there is no third silent-corruption outcome
  a memory read of already-mapped, already-validated-header pages can
  produce), and — critically — it does not change what the test would do if
  a genuinely wrong outcome occurred (e.g. the process exiting 0 with *no*
  recognized marker at all still fails the test today, line 136:
  `t.Fatalf(...)`).
- **Bug-hiding pattern to avoid**: widening the accept-set to include an
  outcome that is *possible* but *would indicate the actual mitigation
  doesn't work* — e.g., if a future edit added "subprocess exited 0 with no
  marker" to the tolerated set instead of keeping it as a hard failure, that
  would silently absorb a real regression (a fixed mitigation that no longer
  either faults correctly or panic-recovers correctly) into "expected
  platform variance." The existing code does NOT do this — the `t.Fatalf`
  on an unrecognized/no-marker clean exit (line 136) is the actual bug-guard
  and must be preserved by any change from this plan, not loosened alongside
  the legitimate two-outcome tolerance.
- Applied to acceptance criterion 3's two branches: if the plan finds "no
  fault occurred, no marker present" is a real gap (distinct from "no fault
  occurred, `NO_FAULT_OCCURRED` marker present," which is already handled),
  the fix must be to make the helper always emit *some* recognized marker for
  every code path it can take (closing the classification gap), not to widen
  the parent test's accept-set to swallow an unmarked clean exit. That is the
  precise distinction between remedy (b) and remedy (a) in acceptance
  criterion 3, and the plan should pick (b)'s framing by default given this
  repo's explicit rule against loosening assertions to make flakes go away.

## 4. Repo-specific history — this exact test has already been touched once for flakiness

`git log --all --oneline -- session/unfinished/gogitstore/mmap_truncation_test.go`
shows a prior, directly relevant commit:

- **`dccee742a` — "fix(tests): fix flaky/failing tests across executor, tmux,
  gogitstore, and git packages"** — its own commit body states for gogitstore
  specifically: *"broadened the truncate-while-mapped test's accepted
  outcomes to also treat a no-fault read as non-failing, since whether the OS
  actually faults on a truncated mmap read is platform/kernel-dependent."*
  This is almost certainly the commit that produced the exact tolerant
  branch (`NO_FAULT_OCCURRED`/`ORDINARY_RECOVER_CAUGHT_IT`, lines 132-135)
  requirements.md's root-cause hypothesis for cluster 3 already points to.
  **Pitfall this surfaces directly: cluster 3 has already been "fixed" once
  before under near-identical framing.** If this plan reintroduces a similar
  widening (rather than recognizing the work is already done, per
  requirements.md acceptance criterion 7's explicit YAGNI guidance), it would
  be the second redundant pass at the same fix — the plan should treat
  `dccee742a` as strong evidence for acceptance criterion 7's "no code change
  needed, already covered" branch and confirm-not-repeat rather than
  re-widen.
- **`d88078d90`/`d404bb557`/`527c225de`/`42808a249` — "fix(gogitstore):
  replace direct exec.Command with safeexec.CommandContext"`** (multiple
  commits, evidently landed more than once across rebases/merges) — this is
  the change that gives `mmap_truncation_test.go` its current
  `safeexec.CommandContext(...)` call (line 123/169) instead of raw
  `exec.Command`. Relevant because `safeexec` is this repo's established
  wrapper with `WaitDelay` pre-set specifically to avoid zombie-process
  accumulation — any change to the fake-claude invocation path in clusters
  1/2 should use the same `safeexec.CommandContext` wrapper for consistency,
  per `.claude/rules/prefer-go-git-over-subshells.md`'s spirit (prefer the
  established, already-hardened primitive over a fresh raw `exec.Command`
  call).
- No commit message anywhere in `--all` history contains "gatekeeper" or
  "operation not permitted" (`git log --all -i --grep`, both empty) — so the
  Gatekeeper/exec-refusal hypothesis for clusters 1/2 is genuinely new to
  this investigation, not a rediscovery of a previously-diagnosed and
  previously-fixed issue the way cluster 3 is. That asymmetry matters for
  the plan: cluster 3 should lean toward "confirm already fixed," clusters
  1/2 should lean toward "this is the first real attempt," consistent with
  requirements.md's own framing.
- House rules already covered by requirements.md and not repeated in depth
  here: `.claude/rules/fix-flaky-tests-dont-defer.md`'s core instruction
  (root-cause and fix or file a tracked follow-up, never silently re-excuse)
  and `.claude/CLAUDE.md`'s "no fix without root cause" / "run it, don't read
  it" — both already explicitly threaded through requirements.md's
  acceptance criteria (0, 4, 6). One addition not yet stated elsewhere: the
  fix-flaky rule's "only exception" clause (skip-for-now is acceptable "when
  fixing it now would meaningfully expand the current change's blast
  radius... say so explicitly and file the follow-up immediately") is the
  right escape hatch *if* the plan concludes a true fix for clusters 1/2
  requires touching `executor.StartProcess`/`safeexec` more broadly than the
  fake-claude test-script path — which requirements.md's "Out of scope"
  section already pre-emptively rules out. If Agent 1/3's research finds the
  minimal fix can't stay scoped that narrowly, that's a conflict between two
  requirements.md constraints that should be flagged back, not silently
  resolved either way.

## Sources

- [os/exec: "operation not permitted" in TestCredentialNoSetGroups · golang/go#24736](https://github.com/golang/go/issues/24736)
- [Running any command on MacOS fails with "operation not permitted" · evilmartians/lefthook#157](https://github.com/Arkweid/lefthook/issues/157)
- [runtime: process crash instead of panic on SIGBUS with SetPanicOnDefault(true) · golang/go#41155](https://github.com/golang/go/issues/41155)
- [cmd/go: add support for dealing with flaky tests · golang/go#62244](https://github.com/golang/go/issues/62244)
- [Re-exec testing Go subprocesses — rednafi.com](https://rednafi.com/go/test-subprocesses/)
- Repo history: `git log --all --oneline -- session/unfinished/gogitstore/mmap_truncation_test.go` (commits `dccee742a`, `d88078d90`, `cfa01210e` and their duplicates)
- Repo config: `.github/workflows/*.yml` (`grep -rn "runs-on"` — confirms no macOS runner anywhere in CI)
