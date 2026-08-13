# Adversarial Review: Immortal Migration Plan

Date: 2026-05-22
Reviewer: Adversarial pass on plan.md
Input: plan.md (updated 2026-05-22), live codebase audit

---

## Verdict: CONCERNS

The plan is structurally sound and covers the core migration correctly. However, two concerns
require resolution before implementation starts. Neither is a blocker to starting Phase 1 in a
fresh session, but both require additional stories or scope adjustments.

---

## Requirement Coverage Check

### R1: Session lifecycle abstraction (create, stop, pause, resume through interface)
- Covered: Stories 1.1–1.5 wire `Instance` to `ProcessManager`
- Gap: `Start()` in ProcessManager matches `TmuxManager.Start(dir string)` but instance.go also
  calls `SetSession()` to reinitialize the TmuxSession mid-lifecycle (see critical issue below)
- **Status: PARTIALLY COVERED — see Critical Issue #1**

### R2: Terminal streaming abstraction (PTY/output to web UI)
- Covered: `StartControlMode()`, `SubscribeToControlModeUpdates()`, `UnsubscribeFromControlModeUpdates()`
  are in the ProcessManager interface; NativeProcessManager fanOut() replaces the control mode protocol
- Status: COVERED

### R3: Process supervision and restart (native backend must actually restart)
- Covered: Story 2.3 (supervise loop), Story 2.6 (test)
- Non-negotiable requirements NM-1 through NM-7 all mapped
- Status: COVERED

### R4: Config-driven backend selection
- Covered: Story 1.3 (OpenFeature), Story 1.4 (config.json + main.go)
- Status: COVERED

### R5: Tmux backend continues to work, no regressions
- Covered: Story 1.2 (TmuxBackend adapter), Story 1.7 (test fix)
- Status: COVERED (pending Critical Issue #1 resolution)

---

## Critical Issue #1: Session()/SetSession() Cannot Be Simply Removed

**Finding:** Story 1.6 says "remove Session()/SetSession() from TmuxManager interface" and that
"callers use TmuxBackend.TmuxManager() type assertion." The live codebase shows this is more
invasive than described.

`session/instance.go` has these direct callers of `tmuxManager.Session()` / `tmuxManager.SetSession()`:

| Line | Call | Context |
|------|------|---------|
| 563, 565 | `SetSession(...)` | `FromInstanceData()` — initial session wire-up for Running instances |
| 574, 576 | `SetSession(...)` | `FromInstanceData()` — wire-up for Stopped instances |
| 1182 | `SetSession(session)` | Session reassignment (likely `Start()` path) |
| 1819, 1821 | `SetSession(...)` | Another reassignment path |
| 2081 | `Session()` | `GetTmuxSession()` public method — returns `*tmux.TmuxSession` |
| 2111 | `SetSession(session)` | Yet another path |
| 2459 | `Session()` | `CaptureCurrentState()` — calls `session.GetPaneCurrentPath()` |

Additionally, `HasSession()` (line 357 in TmuxManager interface) has 6 callers in instance.go
(lines 1159, 1318, 1540, 1750, 2118, 2941). `HasSession()` is NOT in the proposed ProcessManager
interface.

**What this means:**
1. `Session()`/`SetSession()` cannot be removed from TmuxManager interface without moving the
   initialization logic out of instance.go into the factory/TmuxBackend — a larger refactor than
   Story 1.6 implies.
2. `HasSession()` must be added to ProcessManager interface OR the 6 callers must be migrated to
   a different check (possibly `IsAlive()`).
3. `GetTmuxSession()` (line 2078) is a public method on Instance that returns `*tmux.TmuxSession`.
   Callers outside this package who use this method need a plan.

**Recommended resolution for Story 1.6:**

Option A (minimal scope): Keep `Session()`, `SetSession()`, and `HasSession()` in the TmuxManager
interface. They are tmux-specific but they are called from instance.go which will now hold a
ProcessManager field. The plan must either:
  (a) add these to ProcessManager (leaking tmux types into the interface — bad), or
  (b) type-assert to `*TmuxBackend` at each call site to reach TmuxManager() — workable but verbose

Option B (cleaner): Move all `SetSession()` calls into `TmuxBackend.Initialize()` and out of
instance.go. Instance no longer calls SetSession directly; instead `NewProcessManager()` handles
initialization. This is the right long-term shape but adds scope.

Option C (pragmatic for Phase 1): Add `HasSession() bool` to ProcessManager. Keep `Session()`
and `SetSession()` in TmuxManager but out of ProcessManager. The 3 call sites that call
`i.tmuxManager.Session()` in instance.go type-assert via `i.processManager.(*TmuxBackend).TmuxManager().Session()`.
Update plan to document this explicitly.

**Recommendation: Option C.** It keeps Phase 1 focused on the field rename without requiring
initialization logic restructuring. Story 1.6 scope should be reduced to: "add HasSession() to
ProcessManager interface; document that Session()/SetSession() callers will type-assert through
TmuxBackend in Phase 1; full cleanup deferred to Phase 3."

---

## Critical Issue #2: GetPaneCurrentPath Not in ProcessManager Interface

**Finding:** `instance.go:2463` calls `tmuxSession.GetPaneCurrentPath()` directly on the
`*tmux.TmuxSession` object retrieved via `i.tmuxManager.Session()`. This is NOT going through
the ProcessManager interface at all — it reaches all the way through to the TmuxSession.

This pattern exists at line 2459–2463:
```go
tmuxSession := i.tmuxManager.Session()
path, err := tmuxSession.GetPaneCurrentPath()
```

If Story 1.6 removes `Session()` from TmuxManager, this breaks. But even with `Session()`
remaining via type-assertion, `GetPaneCurrentPath()` is not in ProcessManager.

**Recommended resolution:**
- Add `GetCurrentWorkingDirectory() (string, error)` to ProcessManager interface (it already
  appears in the architecture research's proposed interface)
- `TmuxBackend` implements it by calling `b.mgr.Session().GetPaneCurrentPath()`
- `NativeProcessManager` implements it via `executor.ShortLivedCmd` querying the process CWD
- Update Story 1.1 to include this method
- Remove the direct `tmuxSession.GetPaneCurrentPath()` call from instance.go in Story 1.5

---

## Other Issues

### Issue #3: HasSession() Missing from ProcessManager Interface

`HasSession()` has 6 callers in instance.go (lines 1159, 1318, 1540, 1750, 2118, 2941).
It is NOT in the proposed ProcessManager interface. After the field rename in Story 1.5,
these will fail to compile if HasSession() is not added.

**Fix:** Add `HasSession() bool` to ProcessManager interface in Story 1.1.
For NativeProcessManager, `HasSession()` can return the same value as `IsAlive()`.

### Issue #4: Number of Callers — 89 vs. "~85"

Live grep count shows 89 occurrences of `tmuxManager` in session/. The plan says "~89 call sites"
in Story 1.5 (updated from original "~85"). The implementation instruction in the plan is correct;
this is just a note that the real count should be verified by grep before claiming Story 1.5 done.

```bash
grep -c "tmuxManager" session/instance.go
```
Run this after Story 1.5 to confirm zero remaining occurrences.

### Issue #5: review_queue_poller.go Callers Are on Instance, Not processManager

The plan correctly identifies that `review_queue_poller.go` calls `inst.GetTmuxSessionName()`
on `*Instance`, not on the interface. The plan's fix (Instance.GetTmuxSessionName() becomes a
wrapper) is correct and no type-assertion is needed for these callers. Confirmed by code reading.

### Issue #6: pty_discovery.go — Research Inaccuracy Corrected

The original requirements and plan.md (pre-update) referenced `pty_discovery.go:275,291` as
callers of `GetTmuxSessionName()`. Live codebase audit shows zero occurrences of
`GetTmuxSessionName` in pty_discovery.go. The updated plan.md correctly removes this reference.
No action needed.

### Issue #7: DoesSessionExist Callers Are Not All Equivalent to IsAlive

Story 1.5 says "map DoesSessionExist() callers to IsAlive()". The 7 callers are:

| Line | Context | IsAlive() substitution safe? |
|------|---------|------------------------------|
| 580 | `FromInstanceData()` recovery check | **Needs scrutiny** — checks if session existed before recovery Start() |
| 1022 | Stop flow | Probably safe — checking if alive before kill |
| 1509 | `DoesSessionExist()` wrapper method | Yes — this IS the public wrapper; it now calls `IsAlive()` |
| 1655 | Pre-capture check | Probably safe |
| 2373 | Pre-resume check | Probably safe |
| 2421 | Start/restart path | Probably safe |
| 2456 | `CaptureCurrentState()` | Yes — already guarded by HasSession() above it |

The primary concern is line 580: `FromInstanceData()` checks `DoesSessionExist()` specifically
because it might be called before the session is "started" in the stapler-squad sense.
`IsAlive()` should behave identically (both query tmux for session existence), but this must be
verified by reading the TmuxProcessManager implementations of both methods.

**Action:** Story 1.5 should note that line 580 requires explicit verification that
`IsAlive()` semantics match `DoesSessionExist()` for pre-start instances.

### Issue #8: NativeProcessManager Missing `GetPanePID()` Strategy

Story 2.4 says `GetPanePID()` should use `executor.ShortLivedCmd` to query the PID of the top
process in the PTY. This is correct but the plan does not specify HOW — `ShortLivedCmd` to run
what? The equivalent of `tmux display-message #{pane_pid}` would be to read the PID of the
child process from `n.cmd.Process.Pid`, which is directly available as a field.

**Fix:** `GetPanePID()` on NativeProcessManager should return `int32(n.cmd.Process.Pid)` when
the process is alive. No ShortLivedCmd needed. Update Story 2.4 to clarify this.

---

## Summary of Required Plan Updates

| # | Issue | Severity | Action |
|---|-------|----------|--------|
| 1 | Session()/SetSession() cannot be simply removed | BLOCKING | Adopt Option C: type-assert for 3 Session() callers; add HasSession() to interface; defer SetSession() cleanup to Phase 3 |
| 2 | GetPaneCurrentPath not in ProcessManager | BLOCKING | Add `GetCurrentWorkingDirectory()` to ProcessManager interface; implement in TmuxBackend via Session().GetPaneCurrentPath() |
| 3 | HasSession() missing from ProcessManager | BLOCKING | Add `HasSession() bool` to ProcessManager interface |
| 4 | 89 call site count | INFORMATIONAL | Already corrected in updated plan.md |
| 5 | review_queue_poller callers on Instance | VERIFIED CORRECT | No change needed |
| 6 | pty_discovery.go inaccuracy | CORRECTED | Already corrected in updated plan.md |
| 7 | DoesSessionExist line 580 semantics | MODERATE | Story 1.5 must explicitly verify line 580 semantics |
| 8 | GetPanePID() strategy | MINOR | Simplify to `n.cmd.Process.Pid` cast to int32 |

---

## Story/Task Counts

**Phase 1:** 7 stories (1.1–1.7), ~12 tasks
**Phase 2:** 6 stories (2.1–2.6), ~8 tasks
**Total:** 13 stories, ~20 tasks

---

## What the Plan Gets Right

- Interface method list is grounded in the actual `TmuxManager` definition (lines 353–391)
- call site count (89) is correct (grep-verified)
- review_queue_poller.go GetTmuxSessionName() pattern correctly identified
- `//nolint:norawexec` justification requirement correctly called out
- `creack/pty` PTY master fd lifecycle (open for process lifetime) documented
- Pitfall mitigations NM-1 through NM-7 all mapped to specific stories
- `GetPaneDimensions` hot path identified and addressed (track last-set size)
- `GetCursorPosition` confirmed as non-hot-path (zero server/ callers)
- OpenFeature decision is correct: InMemoryProvider at startup, no flagd needed
- Backwards-compatibility: empty `process_manager_backend` defaults to "tmux"
- comprehensive_session_creation_test.go fix is specifically identified

---

## Final Verdict

**CONCERNS** — not BLOCKED. The three BLOCKING issues (HasSession, GetCurrentWorkingDirectory,
Session()/SetSession() strategy) are all resolvable by additive changes to Story 1.1 and a
scope clarification to Story 1.6. No stories need to be removed; three need to be expanded.

The plan should be updated to:
1. Add `HasSession() bool` and `GetCurrentWorkingDirectory() (string, error)` to the
   ProcessManager interface in Story 1.1
2. Revise Story 1.6 scope: keep Session()/SetSession() in TmuxManager; type-assert at the
   3 instance.go call sites that need Session(); defer SetSession() migration to Phase 3
3. Story 2.4: note that GetPanePID() can use `n.cmd.Process.Pid` directly, not ShortLivedCmd

With these adjustments, the plan is implementable in a fresh session without requiring
re-research or significant scope changes.
