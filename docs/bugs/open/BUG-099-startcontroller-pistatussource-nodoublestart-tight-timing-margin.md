# BUG-099: `TestStartController_PiStatusSourceNoDoubleStart` has a tight timing margin, not a logic bug [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-04, investigating a reported reproducible failure of this test on PR #697
**Impact**: Test-only. One failure reported (`iter-0`, "should have 1 item(s), but has 0"); not reproduced
locally across 40+ runs (plain, `-count=5`, `-race`, and under artificial CPU load via background `yes`
processes).

## Problem Description

PR #697 bundles three commits (`787a61165` "self-heal Instance.Started() in streamViaHub",
`a440ebce0` pi-support stderr/rendering fixes, `292b086e2` session flake fixes). The PR author suspected
`787a61165`'s new `Instance.MarkStartedIfTmuxAlive()` might interact badly with
`TestStartController_PiStatusSourceNoDoubleStart`'s double-start guard.

**Ruled out**: `git show --stat` on all three commits confirms none touch
`session/instance_controller.go`, `session/instance_controller_test.go`, `session/instance_pi_status.go`,
or `session/pi_status_source.go`. `787a61165` only touches `server/services/connectrpc_websocket.go` and
`session/instance_state.go`. The pi-extension `StartController` path
(`session/instance_controller.go`'s `controllerExtensions()` dispatch) never checks `Instance.Started()`
at all -- `piExtension.supported()` only checks `isPi(i.Program)` and the feature flag -- so
`MarkStartedIfTmuxAlive` cannot affect this test's code path even in principle.

## Reproduction Steps

Not reproduced. Tried:
- `go test ./session/... -run TestStartController_PiStatusSourceNoDoubleStart -count=1|5 -v` (multiple times)
- Same, with `-race`
- Same, with ~20 background `yes > /dev/null &` processes for artificial CPU contention on an 18-core
  machine

All passed. One suggestive data point: in a cold test binary (first invocation after a fresh `go test`
process start), the first 1-2 subtests took ~4.1-4.4s end-to-end (fork/exec + shell startup +
`StopController`'s `Kill()`/`wg.Wait()` join), settling to ~0.13s for the remaining 18 once warm. That's
close enough to the test's original 5s deadline that additional load (a real CI runner, EDR/AV on-access
scanning of a freshly-written executable script, concurrent `go build`/`go test` invocations) plausibly
pushes a legitimate (non-buggy) subprocess spawn past the deadline, producing a "zero PIDs" failure
indistinguishable from the double-start bug the test guards against.

## Fix Approach

Applied as an interim mitigation: bumped the poll deadline from 5s to 15s
(`session/instance_controller_test.go`, `TestStartController_PiStatusSourceNoDoubleStart`). This is a
margin widening, not a root-cause fix -- if the failure recurs even at 15s, the next step is to stop
relying on wall-clock polling for an out-of-band OS-level signal (the whole reason this test writes to a
PID file from the child, instead of just checking `inst.piStatusSrc.Load()`, is that it must observe a
would-be-discarded second subprocess that the in-memory `atomic.Pointer` would silently hide) and instead
use a filesystem event (e.g. `fsnotify`) or a `sync.WaitGroup`-backed signal the fake `pi` script trips
before sleeping, removing the timing dependency entirely.

## Related

- `.claude/skills/fix-flaky-tests-dont-defer/SKILL.md`
- `docs/bugs/open/BUG-098-crosstest-testdir-pollution-from-leaked-tmux-pipeline-goroutine.md` -- same
  "observed once under load, not reproducible in isolation" shape.
- PR #697 (`stapler-squad-frontend-memory` branch)
