# Adversarial Review: isolated-dev-stacks

Date: 2026-07-04
Verdict: CLEAN
Iteration: 3 (final scoped re-review of remaining blocker)

## Previously-Blocked Items Re-Reviewed

- [x] Blocker 1 (orphan detection via backendPid/frontendPid) — RESOLVED — Verified against ADR-001 §1 (lines 54-85) and plan.md Tasks 3.2.1e (lines 297-299), 3.2.1g (lines 305-308), 3.2.1h (lines 310-314), and the Story 3.2.1 AC (line 276). Point-by-point:
  1. **Manifest write timing (3.2.1e):** the write is explicitly gated on "once both children are ready (i.e. after Task 3.2.1b's backend health-poll AND Task 3.2.1c's frontend readiness-poll have both resolved)," and the task text states outright "the manifest never exists in a partially-populated state where one child's PID is known and the other isn't." `backendPid: backendChild.pid` / `frontendPid: frontendChild.pid` are captured at that single point, not earlier. Confirmed correct.
  2. **Negative-PID kill on 3.2.1g:** the sweep issues `process.kill(-pid, 'SIGTERM')` (escalating to `-pid`/SIGKILL) explicitly annotated "(its own process group, since `detached: true` made it its own group leader)." ADR-001 §1 independently spells out why this matters for `next dev`'s `next-server` grandchild ("the child itself plus anything it spawned"). Confirmed the plan uses the process-group form, not a bare positive-PID kill — this is the exact defect iteration 2 found (the old sweep read only the launcher's `pid` and killed `-pid` on the *launcher's* group, which is unrelated to either child).
  3. **Independent per-child liveness check:** 3.2.1g probes `backendPid` and `frontendPid` separately via `process.kill(pid, 0)` and reaps each independently ("For each PID found alive, reap it BEFORE proceeding... After both backendPid and frontendPid have been checked (and reaped if alive)..."). The Story 3.2.1 AC's worked example is deliberately mixed-liveness (`backendPid: 40213` alive, `frontendPid: 40217` dead) and the plan text walks through both branches distinctly (dead → "does nothing further"; alive → full reap sequence). Confirmed not all-or-nothing.
  4. **Acceptance criterion asserts the reap, not a side effect:** the rewritten AC (plan.md:276) requires "confirms `40213` is no longer running via a follow-up `process.kill(40213, 0)` throwing `ESRCH` — only *after* that confirmation does it delete the stale manifest and proceed." This is a genuine liveness recheck proving death, replacing iteration 2's "no `EADDRINUSE`" AC that passed trivially regardless of whether reaping occurred. Task 3.2.1h additionally requires the unit test to assert `process.kill` is called with `-backendPid`/`-frontendPid` specifically, "never `process.kill(-pid, ...)` on the launcher's own PID field" — this closes the exact loophole iteration 2's Blocker 4 note flagged (a mocked test that could pass while encoding the flawed launcher-pid assumption). Confirmed.

  All four sub-checks pass against the plan text as written. Blocker 1 is resolved.

## Blockers

(empty — no blockers remain from this pass)

## Concerns

Note: 9 concerns and 4 minors from iteration 1 remain unaddressed except where noted below; not re-verified in this pass.

Carried forward from iteration 2:
- The "gate `STAPLER_SQUAD_EXTRA_ORIGINS` to `STAPLER_SQUAD_INSTANCE`" idea from iteration 1's Blocker 3 recommendation was not adopted — the env var is still read unconditionally in `main.go` for every instance, including a systemd-managed one, if the var happens to be set in its environment. Judged acceptable given strict localhost-only validation and startup logging, but worth reconsidering if this mechanism is ever generalized beyond "2-5 concurrent local dev stacks" (ADR-001's own Consequences section makes the same caveat).
- The iteration-1 concern "no CI wiring specified for the new TypeScript tests" appears substantially addressed as a side effect of iteration 2's Blocker 4 fix (Task 3.2.1h's CI-wiring instructions cover both `launch.test.ts` and `ports.test.ts`), though this was not the focus of that pass or this one and should be confirmed directly at the next full review.

New this pass (from item 5 of the verification scope — not the original blocker, downgraded to Concern):
- **Manifest read/write has no stated atomicity or corrupt-JSON handling.** Task 4.1.1b explicitly calls out reusing an "atomic temp-file+rename pattern" for `workspace_meta.json`; Task 3.2.1e (the `dev-stack.json` writer) and Task 3.2.1g (the sweep's reader) make no equivalent statement. If the launcher is killed at the exact moment it is mid-write to `dev-stack.json` (a narrow window, but the same class of hard-kill event Blocker 1 was about), a future sweep's JSON parse of a partially-written file could throw, and neither task text mentions a try/catch or a "treat unparseable manifest as absent" fallback. Task 3.2.1g does correctly handle the *missing-file* case (see item 5 detail below) — this concern is specifically about a *corrupt-but-present* file, a narrower and lower-probability variant. Recommend Task 3.2.1g's implementation wrap the read+parse in a try/catch that logs a warning and proceeds as if no manifest existed (same as the missing-file path), and that Task 3.2.1e use write-to-temp-then-rename for `dev-stack.json` to shrink the corruption window to effectively zero, matching the precedent already used for `workspace_meta.json`. Not severe enough to block: the window is narrow, the failure mode is "sweep behaves as if orphan-detection didn't run for one launch" rather than a crash loop, and it does not regress anything iteration 2 flagged.

## Minors

(carried forward from iteration 1, not re-verified in this pass — see prior version of this file / architecture-review.md for full text)

New this pass:
- Item 5 of the verification scope (first-ever launch, no manifest yet) is handled correctly and does not need a Concern of its own: Task 3.2.1g frames the whole sweep as conditional — "check whether `~/.stapler-squad/instances/<name>/dev-stack.json` already exists. If it does: read its `backendPid`/`frontendPid`..." — with no read attempted when the file is absent, so a first-ever launch for an instance name degrades gracefully (no sweep, straight to port allocation) rather than crashing on a missing file.
