# Validation: Inspect and Nudge Before Restarting

## Adversarial Review Findings

### Finding 1: Snapshot capture before `UpdateItemSessionEnded` — ordering risk
**Claim:** We capture the snapshot before ending the session (step 1 in the plan).
**Risk:** If snapshot capture is slow (e.g., tmux pane capture hangs), it delays the BUG-064-ordered `UpdateItemSessionEnded` call. A concurrent `onSessionExited` goroutine that fires between capture start and `UpdateItemSessionEnded` could race the in_progress→review transition.
**Mitigation:** Snapshot capture should have a short deadline (e.g., 3s context timeout). On timeout, set `snap.CaptureErr` and continue — the BUG-064 ordering invariant is more important than a perfect snapshot tail.

### Finding 2: `DecideStallAction` NudgeAllowance field source
**Claim:** Plan says "fetch from stuck row or BacklogItemData."
**Risk:** If the nudge allowance counter is not readily accessible from within `RemediateStaleWorkSession`, `DecideStallAction` might always see `NudgeAllowance=0` and fall through to `respawn` — silently defeating the nudge path.
**Mitigation:** Before implementation, verify exactly where the nudge-budget counter lives (confirm field name in the `StallDecisionInput` and how PR #1020 populates it in `onAutonomousDriverComplete`'s flow). This is a must-resolve before T2.

### Finding 3: Dead-pane stuck-row gap
**Claim:** For dead-pane events, the stuck-row context is updated by `AutoRespawnAutonomousWork` after `Start(false)`.
**Risk:** `AutoRespawnAutonomousWork` writes a fresh stuck row for the new session, not the dead-pane context. The `stop_reason=dead_pane` annotation may never appear in the stuck row at all — operators see a respawn with no explanation.
**Mitigation:** Option A (plan's MVP): emit `stop_reason=dead_pane` via slog only; accept the stuck-row gap for now. Option B: have `respawnDeadPane` receive a callback that writes a pre-respawn stuck-row entry. Recommend Option B as AC3 says "logged before every restart" — confirm whether "logged" means slog only or stuck row too.

### Finding 4: Recovery guard scope
**Claim:** `RemediateStaleWorkSession` is a method on `AutonomousOrchestrationService`, so it can read `a.recoveries`.
**Risk:** The research found the function on `BacklogService` (`backlog_service_triage.go`). If it is NOT on `AutonomousOrchestrationService`, `a.recoveries` is not accessible.
**Mitigation:** Confirm the receiver type before T6. If it's on `BacklogService`, the guard needs to be injected as a `func(title string) bool` parameter or via an interface method on `AutonomousOrchestrationService`.

### Finding 5: `nudgeStalledWorkSession` vs direct `StallNudger` call
**Claim:** Plan says "call `nudgeStalledWorkSession` path or `StallNudger.NudgeStalledSession` directly."
**Risk:** `nudgeStalledWorkSession` contains `canRestart` checks (session Active, unarchived, AutonomousMode on, no retry pending, ctx not cancelled) and driver re-attach logic (`reattachDriver`). Bypassing it and calling `NudgeStalledSession` directly skips these guards.
**Mitigation:** Prefer calling `nudgeStalledWorkSession` (or the relevant check + nudge sub-call) rather than `NudgeStalledSession` directly. If `nudgeStalledWorkSession`'s re-attach logic is not needed for the stale-work path (the driver is already stopped), factor out the nudge sub-call into a shared helper.

---

## Pre-mortem

**"The feature ships but nudges are silently skipped":** If `NudgeAllowance` is always 0 (Finding 2), or if the nudge path has an error that falls through to `leave=true`, sessions will be respawned without a nudge and no error is surfaced. Add a counter metric `stale_work_nudge_sent` / `stale_work_respawn_no_nudge` to make the ratio observable.

**"The BUG-064 invariant is broken":** If snapshot capture is moved after `UpdateItemSessionEnded`, the snapshot may capture a dead pane instead of the live one. Enforce the ordering with a comment matching the existing BUG-064 comment style.

**"Tests pass but the live path never hits the nudge branch":** `DecideStallAction` row 7 requires `item_in_work + nudge_enabled + nudge_allowed`. In integration, all three must be true. Write at least one integration-level test (not just unit mocks) that exercises the nudge path end-to-end in `server/services/`.

---

## Definition of Done

- [ ] `go build ./...` passes
- [ ] `make test` passes (including new table tests for T5)
- [ ] `make lint` passes (no new golangci findings)
- [ ] `make quick-check` green
- [ ] AC1–AC5 manually verified against a local manual build (see CLAUDE.md's manual instance instructions)
- [ ] Finding 2 (NudgeAllowance source) resolved by code inspection before T2 begins
- [ ] Finding 4 (receiver type) resolved by code inspection before T6 begins
