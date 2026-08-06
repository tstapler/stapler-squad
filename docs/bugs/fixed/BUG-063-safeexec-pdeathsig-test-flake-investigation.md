# BUG-063: `TestEnsurePdeathsig_GrandchildDiesWhenMiddleIsSigkilled` Flake Investigation [SEVERITY: Low]

**Status**: ✅ Hardened (test-only change), root cause of the original flake unconfirmed
**Discovered**: 2026-08-05, twice during `make test` full-suite runs while investigating the `session/tmux` flake tracked at backlog item `6be30c82-8255-4fc9-b3f9-ed783b0625a9`. Filed separately per `.claude/rules/fix-flaky-tests-dont-defer.md` since that item's scope is limited to `session/tmux`.
**Impact**: Intermittent `make test` failures in `executor/safeexec`, unrelated to `EnsurePdeathsig`'s actual runtime correctness (see Production-Defect Analysis below). Blocks a clean "all green" full-suite run.

## Problem Description

`executor/safeexec/safeexec_pdeathsig_linux_test.go:51` re-execs itself as a "middle" process, has it spawn a "grandchild" (`sleep 30`) with `Pdeathsig=SIGKILL` set via `EnsurePdeathsig`, `SIGKILL`s the middle process, then polls for up to 3s for the grandchild to disappear via `syscall.Kill(pid, 0)`. It was observed failing twice during unrelated `make test` runs but never reproduced in 3 isolated reruns or 5 full-suite reruns noted in the original triage.

## Reproduction Attempt (AC0)

Prior non-repro attempts (3x isolated, 5x full `make test`) were statistically weak evidence for a load-dependent race — see `project_plans`/triage research math: a race with even a 10-20% per-run trigger probability under load has a 33-59% chance of not appearing in 5 trials by chance alone.

Ran a substantially heavier, multi-strategy campaign instead: **3,000 total iterations of the target test, 0 failures**, across three independently-motivated load-generation strategies (all on a 24-core dev box, peak load average ~10.7, memory/swap monitored and stable throughout):

1. **Real full-suite noise** — background `go test -short ./...` (3, later 8, concurrent loops) + sequential `-count=300` runs of the target test, x2 = 600 iterations.
2. **Self-parallel contention** — many concurrent invocations of the target test itself, directly stressing the same fork/exec/wait/reap kernel paths the test depends on (more representative of the failure's actual mechanism than generic CPU load): 12×50 + 16×50 = 1,400 iterations, layered on top of continuing full-suite noise.
3. **`GOMAXPROCS`-constrained oversubscription** — simulating a core-starved CI runner: `GOMAXPROCS=2`, 20×30=600, and `GOMAXPROCS=1`, 40×10=400 = 1,000 iterations.

This is 10x the original AC's 300-iteration minimum and covers both "same machine, heavier noise" and "fewer-cores" contention profiles. **The flake did not reproduce under any of these strategies.**

## Production-Defect Analysis (AC1)

**Per-thread `Pdeathsig` hypothesis: refuted.** Read Go 1.26.4's `syscall/exec_linux.go` directly:

- `forkAndExecInChild1` brackets the entire fork→child-setup→exec sequence between `runtime_BeforeFork()` (`exec_linux.go:331`) and `runtime_AfterForkInChild()` (`exec_linux.go:418`). Per `forkAndExecInChild`'s own doc comment (`exec_linux.go:130-133`): *"In the child, this function must not acquire any locks... no rescheduling, no malloc calls, and no new stack segments."* The `PR_SET_PDEATHSIG` `prctl` call (`exec_linux.go:549-550`) runs inside that bracket via `RawSyscall6` — there is no Go-scheduler preemption point between fork and `prctl`/`execve`, so the classic "goroutine migrated to a different OS thread" scenario (`go.dev/issue/27505`, cited at `exec_linux.go:92-95`) cannot occur inside `os/exec`'s own machinery.
- Zero `runtime.LockOSThread()` call sites anywhere in this repo (`grep -rn LockOSThread` — no matches), so there's no application-level gap either, though it isn't needed for this code path regardless.
- Per `man 2 prctl` (`PR_SET_PDEATHSIG`), the parent-death signal is tied to the specific OS thread that issued the `prctl` call. The risk this creates (per `go.dev/issue/27505`) is **premature/spurious delivery** — not **missed delivery**. This test's failure mode is a whole-process `SIGKILL` (`middle.Process.Kill()`), which terminates every OS thread in the target process atomically, including whichever thread originally called `prctl`. Whole-process death always subsumes the registering thread's death, so delivery is guaranteed by kernel semantics regardless of Go's internal thread scheduling.
- `EnsurePdeathsig` (`executor/safeexec/safeexec_pdeathsig_linux.go:24-29`) is a single unconditional field assignment — no live-goroutine `prctl` call, no conditional/racy path.

**Conclusion: not a production defect.**

**Zombie-reap-latency hypothesis: answered analytically only.** No repro was captured despite the 3,000-iteration campaign, so there is no live `/proc/<pid>/stat` or wall-clock-delta evidence to instrument — this half of AC1 could not be empirically resolved. What is independently verifiable (real Linux `kill(2)`/`wait(2)` behavior, not speculative) is that the test's only liveness check, `syscall.Kill(pid, 0)`, cannot distinguish a zombie awaiting reap from a genuinely running process — the kernel keeps a zombie's PID table entry until something calls `wait()` on it, and signal-0 delivery succeeds for both. This is a real gap in the test's assertion correctness regardless of whether it caused the historical flake.

## Resolution

Hardened `processAlive` (`executor/safeexec/safeexec_pdeathsig_linux_test.go`) to read `/proc/<pid>/stat` and treat zombie state (`Z`) as dead, rather than relying solely on `kill(pid, 0)`. `Pdeathsig`'s contract is "the kernel terminates the child" — a zombie already satisfies that; how promptly some other process (init/subreaper) gets around to reaping the corpse is a separate, unbounded-latency concern this test shouldn't be sensitive to. This closes a real (if not empirically confirmed as the historical trigger) ambiguity per the investigation's own research recommendation against closing bare with no concrete change.

Verified: 500 further iterations of the hardened test under renewed background `go test ./...` contention, 0 failures; full `go test -short ./executor/...` and `golangci-lint run ./executor/safeexec/...` clean.

## Related

- Sibling investigation: `session/tmux` flake at backlog `6be30c82-8255-4fc9-b3f9-ed783b0625a9` (see `BUG-051`), which this item was split out from per `.claude/rules/fix-flaky-tests-dont-defer.md`.
