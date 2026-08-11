# Implementation Plan: cold-restart-uuid-recovery

**Feature**: Move `HistoryFileDetector.DetectByPath()` recovery ahead of `initTmuxSession()` in both cold-restore code paths, guarded so it cannot resurrect a conversation the user explicitly cleared.
**Date**: 2026-08-10
**Status**: Ready for implementation of Epic 1.0 (verification spike) first; Epics 1.1/1.2 ready to implement once Epic 1.0 confirms or corrects their premise (see Epic 1.0 header, added after engineering triad review round 2).
**ADRs**: ADR-001 (guard against resurrecting an explicitly-cleared conversation)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| Cold restore | The `!firstTimeSetup && !i.pm().IsAlive()` branch of `startLocked()`/`start()` — tmux pane is dead, session must be relaunched | Existing term, used in code comments/log lines already |
| Pre-launch recovery | Calling `i.tryExtractConversationUUID()` *before* `i.initTmuxSession()` instead of only after `i.pm().Start()` | This fix's core change; not a new type, just a call-site move |
| `DetectByPath` fallback | `HistoryFileDetector.DetectByPath()` — scans `~/.claude/projects/<encoded-path>/` for the newest `*.jsonl` by mtime, no live process required | Existing, unmodified except for exposing `ModTime` |
| `conversationClearedAt` | New unexported `time.Time` field on `Instance`, set by `ClearConversationState()` | Guards the `DetectByPath` fallback: a candidate JSONL older than this is not trusted |
| `HistoryFileInfo.ModTime` | New field carrying the winning candidate's on-disk mtime out of `DetectByPath` | Additive struct field; `Detect()` (PID-based fast path) leaves it zero — not needed there |
| Best-effort shared-directory carve-out | The pre-existing, explicitly-accepted limitation from `session-resume-uuid-fix`: an unlinked session sharing a project directory with another session may pick up that session's newer JSONL | Not solved by this fix; explicitly re-affirmed as still accepted (see ADR-001 and Pattern Decisions) |

No new exported types, interfaces, or Strategy/Factory patterns are introduced. This is a call-site reorder plus one small guard timestamp — consistent with `.claude/rules/interface-pollution-checklist.md`.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Recovery call ordering | Plain reorder — call the existing `i.tryExtractConversationUUID()` earlier, no new abstraction | architecture.md §4 | New `ConversationRecoveryStrategy` interface | Single call site (two mirrored copies), no second implementation exists or is planned — speculative interface per `interface-pollution-checklist.md` smell #1 |
| Shared-directory / concurrent-cold-start-same-dir risk (pitfalls.md §1, bullets 1 & 3) | **Accept** the existing `session-resume-uuid-fix` best-effort carve-out; no new guard | pitfalls.md §1, requirements.md "Existing related work" | Add a `lastKnownConversationTimestamp` / per-session JSONL-ownership tag to disambiguate shared directories | YAGNI — no evidence this edge case has caused an actual incident, unlike the bug this fix targets; `session-resume-uuid-fix`'s own requirements already scoped it out by name as "pre-existing limitation, best effort only" |
| `ClearConversationState()` interaction (pitfalls.md §1, bullet 2) | **Add** a `conversationClearedAt` timestamp guard on the `DetectByPath` fallback inside `tryExtractConversationUUID()` | Own code-read finding, see ADR-001 | (a) Do nothing / fold into the same accepted carve-out as the shared-directory case | Rejected — this is a different risk class: concrete, deterministic, self-inflicted by this exact fix's new call, with an already-existing trigger path (`recoverFromStaleResume` → `session/health.go`'s dead-pane recovery), not a rare cross-session coincidence. See ADR-001. |
| ” | ” | ” | (b) Persist the guard timestamp via the `session/ent` schema | Rejected — scope creep (schema migration, `--feature sql/upsert` regenerate) to fix a same-process regression that an in-memory field already closes; the residual cross-process-restart gap is accepted (see ADR-001, Consequences) |
| ” | ” | ” | (c) Scope pre-launch recovery to worktree-mode sessions only | Rejected — does not address the `ClearConversationState` risk at all (directory-mode sessions hit `recoverFromStaleResume` too), and would silently narrow AC1 for the majority-case directory-mode sessions |
| Log level for "recovery attempted, found nothing" vs "found but discarded by guard" vs "found and used" | Keep existing outer WARN/INFO split in `instance.go` (already correct after the reorder); add one new DEBUG line inside `tryExtractConversationUUID`'s fallback branch for the guard-discarded case | pitfalls.md §5 | Always log WARN whenever the in-memory UUID is empty, regardless of cause | WARN-spam on every ordinary first-ever cold start; reserves WARN for genuine anomalies per pitfalls.md §5 |
| AC3 stretch goal (user-visible signal) | Defer to a named follow-up; no proto/UI change in this plan | ux.md §3-4 | Implement the `events.NewNotificationEvent` toast now (the `onRateLimitDetected` precedent) | Explicitly a stretch goal in requirements.md AC3, not a hard AC; disproportionate net-new surface for a reliability bug fix — noted under Risk Control below as the named follow-up |
| `correlateSession()`'s own independent `DetectByPath` call (`session/history_linker.go`) | Leave untouched — no `conversationClearedAt` guard added there | Own code-read finding | Apply the same guard to `correlateSession` for consistency | Out of scope: `correlateSession` doesn't influence the `--resume` launch decision (the bug this plan fixes), the exposure is pre-existing and unrelated to this reorder, and touching `history_linker.go` would expand the diff into a different subsystem for a risk with no reported incident — worth a named follow-up, not a blocker here |

---

## Migration Plan

N/A — no schema or persisted-data changes. `conversationClearedAt` is an unexported, in-process-only field (see ADR-001).

## Observability Plan

- **Logs**:
  - Unchanged: `instance.go`'s existing `log.Info("cold restoring with --resume", ...)` / `log.Warn("cold start: tmux dead, no conversation UUID, starting fresh", ...)` split (lines ~870-874, mirrored ~1061-1069) — now finally accurate, since `i.HasClaudeSession()` reflects the outcome of the *pre-launch* recovery attempt instead of a stale pre-recovery value.
  - New: one `log.Debug("tryextractconversationuuid: found jsonl predates last explicit clear, skipping recovery", "session", i.Title, "path", info.HistoryFilePath, "clearedAt", i.conversationClearedAt)` inside `tryExtractConversationUUID`'s fallback branch — this is the AC3 "enough context to diagnose after the fact" signal, distinguishing "no JSONL ever existed" (existing `"no jsonl file found"` DEBUG line) from "a JSONL existed but predates an explicit clear" (new DEBUG line) from "found and used" (existing `"found conversation via path fallback"` INFO line).
  - Doc-comment fix: `tryExtractConversationUUID`'s stale "The tmux session must be alive for this to work" comment is corrected to describe the `DetectByPath` fallback (architecture.md §2).
- **Metrics**: no new metrics required.
- **Alerts**: no new alerts required.

## Risk Control

- **Feature flag**: not gated — this is a bug-fix reorder with a safety guard, not a new capability; no rollout switch needed.
- **Rollback procedure**: standard revert via PR close + revert commit.
- **Staged rollout**: full rollout on merge.
- **Named follow-ups (deliberately deferred, not part of this plan)**:
  1. User-visible "conversation could not be resumed" toast via `events.NewNotificationEvent` (ux.md §3-4) — stretch goal per requirements.md AC3.
  2. Apply the same `conversationClearedAt` guard to `session/history_linker.go`'s `correlateSession()` for full consistency — pre-existing exposure, no reported incident.
  3. `session/session_driver.go`'s hardcoded 10-minute inactivity-restart window that opens the UUID-loss race in the first place — explicitly out of scope per requirements.md.
  4. Persisting `conversationClearedAt` across a `stapler-squad` process restart (would require `session/ent` schema work) — the in-memory guard added here only protects the same-process regression (ADR-001).
  5. *(added post-pre-mortem, 2026-08-10)* Factor the duplicated guard block (Tasks 1.1.1a/1.1.1b) into one private helper (e.g. `i.recoverConversationBeforeLaunch()`) instead of literal duplication — pre-mortem failure mode #4 (P3): a future one-sided edit to only one call site would silently reintroduce this exact bug class. Low cost; do during implementation of Story 1.1.1, not deferred indefinitely.
  6. *(added post-pre-mortem, 2026-08-10)* Post-ship confirmation step (pre-mortem #5, P2): ~1 week after deploy, grep production logs for the new `tryextractconversationuuid: found conversation via path fallback` / `...skipping recovery` / `...no jsonl file found` lines and record counts, so "shipped" isn't conflated with "confirmed the recovery path actually fires in production." Not a new metric/alert — a one-time manual check.
  7. *(added post-pre-mortem, 2026-08-10)* mtime-granularity race (pre-mortem #3, P2): a JSONL written by `KillSession()`'s triggering event and read by `DetectByPath` within the same mtime-granularity window could be partially written. No debounce added in this plan; worth a small "candidate mtime must be >1-2s old, or file is well-formed JSON" check if this proves to matter in practice — not blocking, no evidence yet.
- **Validation gate addition** *(added after engineering triad review round 2, 2026-08-10)*: Task 1.2.3a's targeted test run should include `-race` (`go test -race ./session -run "..."`) at least once, given this plan adds new `claudeSessionMu` lock discipline around a field read from a call site (`SwitchWorkspace`) that is deliberately lock-free by design elsewhere.

## Unresolved Questions

- *(added after engineering triad review round 2, 2026-08-10)* Does `Instance.KillSession()` + `Instance.Start(false)` — the exact sequence `session/health.go`'s dead-pane recovery uses, and ADR-001's named "concrete trigger path" — actually re-derive the launch command from the current `i.claudeSession.ConversationUUID`, or does `initTmuxSession()`'s `HasSession()` early-return skip `buildLaunchCommand()` whenever a `TmuxSession` object already exists in-process (as traced in Epic 1.0's header)? Resolved by Epic 1.0/Task 1.0.1a before Story 1.1.1/1.1.2 are implemented against it.

## Dependency Visualization

```
Epic 1.1: Hoist pre-launch recovery
  Story 1.1.1 (reorder call sites)
    Task 1.1.1a (startLocked)  --\
    Task 1.1.1b (start)         >-- both required before any test can exercise the fix
    Task 1.1.1c (doc comment)  --/
        |
        v
  Story 1.1.2 (ClearConversationState guard)
    Task 1.1.2a (add field) --> Task 1.1.2b (set in ClearConversationState)
    Task 1.1.2a (add field) --> Task 1.1.2c (ModTime plumbing) --> Task 1.1.2d (guard check)
        |
        v  (Epic 1.1 complete: fix + guard both in place)
Epic 1.2: Regression tests
  Story 1.2.1
    Task 1.2.1a (AC4 integration test) -- depends on Epic 1.1 complete
  Story 1.2.2
    Task 1.2.2a (negative unit test)  -- depends on Story 1.1.2
    Task 1.2.2b (positive unit test)  -- depends on Story 1.1.2
    Task 1.2.2c (ModTime unit test)   -- depends on Task 1.1.2c
  Story 1.2.3
    Task 1.2.3a (targeted go test run)   -- depends on all of 1.2.1/1.2.2
    Task 1.2.3b (make quick-check)       -- depends on Task 1.2.3a
```

---

## Phase 1: Cold-Restart UUID Recovery

### Epic 1.0: Verify the load-bearing premise before implementing (spike)

> **Added after engineering triad review round 2, 2026-08-10 — BLOCKER, must run first.**
> A fresh engineering-lens review traced the actual call path and raised a concrete,
> code-cited concern that undermines this plan's central claim: `TmuxProcessManager.Close()`
> (called by `Instance.KillSession()`) never nils `tm.session` (an `atomic.Pointer`), and
> `initTmuxSession()` (`session/instance_tmux.go:249-253`) early-returns via `HasSession()`
> — true as soon as any `TmuxSession` object has ever existed for this Instance in-process —
> *without* calling `buildLaunchCommand()` again. `TmuxSession.program` is set only at
> construction (`newTmuxSessionWithSocket`) and is never reassigned elsewhere in the package.
> If this reading is correct, `Instance.KillSession()` + `Instance.Start(false)` — the exact
> sequence `session/health.go:219-228`'s dead-pane recovery uses, and the sequence ADR-001
> names as its "concrete trigger path" — relaunches tmux using whatever `t.program` was baked
> in at the *last* `SetSession()` call, NOT a value recomputed from the current
> `i.claudeSession.ConversationUUID`. That would mean neither the pre-existing code nor this
> plan's reorder/guard actually changes behavior for that in-process reuse case — only for a
> genuinely fresh `Instance` (true process-boot cold start, which is what `TestColdRestore_*`
> already exercises via `NewInstanceWithCleanup`). By contrast, `Instance.Restart()`
> (`session/instance.go:1487+`) *does* explicitly rebuild the launch command every call. Which
> of the two paths produced requirements.md's captured log timeline is not established —
> `session_driver.go`'s `handleDriverFailure` calls `Start(false)` directly only when status is
> `Stopped`, otherwise it calls `Restart(false)`.
>
> This must be resolved before Story 1.1.1's tasks are trusted to deliver AC1 for the
> in-process restart-churn scenario (as opposed to the true-cold-boot scenario, which the
> existing plan and AC4 test already cover correctly regardless of this open question).

#### Story 1.0.1: Confirm whether `KillSession()` + `Start(false)` rebuilds the launch command
**As a** plan author, **I want** proof (not inference) of which code path produced the
observed bug, **so that** Story 1.1.1/1.1.2 target the actual failure mode instead of a
plausible-sounding but unverified one.

**Acceptance Criteria**:
- A test or direct trace confirms, for an `Instance` whose `TmuxSession` object already
  exists (constructed once, then `KillSession()` called, then `Start(false)`), whether the
  resulting tmux `new-session`/pane command embeds `--resume <current-uuid>` or reuses the
  stale command from the original construction.

##### Task 1.0.1a: Write a focused trace test contrasting `KillSession()+Start(false)` vs `Restart()` (~15 min)
- In a new or existing `session` package test, construct an `Instance` (real tmux, per
  `instance_cold_restore_test.go`'s helpers), call `Start(true)` once (`firstTimeSetup`) to
  force `TmuxSession` construction, set `i.claudeSession.ConversationUUID` to a known value,
  call `i.KillSession()`, then `i.Start(false)`. Inspect the actual launched command (via the
  `TmuxSession`'s stored `program`/`LaunchCommand` field, whichever this codebase exposes for
  assertion — see how `TestColdRestore_WithUUID` asserts this today) to confirm whether it
  contains `--resume <the-set-uuid>`.
- Run the equivalent through `i.Restart(false)` for contrast, per the round-2 review's finding
  that `Restart()` explicitly captures and rebuilds via `buildLaunchCommand()`.
- **If the trace confirms `KillSession()+Start(false)` does NOT rebuild the command**: this
  plan's Epic 1.1 reorder is still correct and beneficial for true-cold-boot revival (ship it
  as scoped), but does not by itself close the in-process restart-churn scenario in
  requirements.md's captured timeline. Add a follow-up story (not blocking this plan, but
  tracked in Risk Control) to fix `initTmuxSession()`'s `HasSession()` reuse check so a dead
  pane with a stale `program` also gets `buildLaunchCommand()` re-run — likely the same class
  of fix as this plan, one layer up.
- **If the trace confirms it DOES rebuild the command** (i.e. this review's tracing missed
  something, e.g. another code path resets `tm.session`): update ADR-001 to cite the
  confirming test in place of the current inference, and proceed with Epic 1.1/1.2 as written.
- Files: `session/instance_cold_restore_test.go` (or a new `session/instance_restart_launch_command_test.go`)

---

### Epic 1.1: Hoist pre-launch recovery ahead of the launch-command decision
**Goal**: `--resume <uuid>` is decided using a disk-recovered UUID when the in-memory one is empty, at both mirrored cold-restore call sites, without resurrecting an explicitly-cleared conversation.

#### Story 1.1.1: Reorder the recovery call in both `startLocked()` and legacy `start()`
**As a** stapler-squad operator, **I want** a revived session to resume its Claude conversation whenever a JSONL for it still exists on disk, **so that** an inactivity-timeout or crash-triggered restart doesn't silently discard in-flight context.

**Acceptance Criteria**:
- AC1 (requirements.md #1): the recovery attempt runs before `initTmuxSession()`/`buildLaunchCommand()`, not after.
  - *Given* an `Instance` with `SessionType: SessionTypeDirectory`, `Path` pointing at a temp dir, tmux dead (`i.pm().IsAlive() == false`), `i.claudeSession == nil`, and a valid `<uuid>.jsonl` file present under `~/.claude/projects/<ClaudeProjectDirName(Path)>/` (fake home dir injected via `historyDetector`), *When* `inst.Start(false)` runs, *Then* `inst.LaunchCommand` contains `--resume '<uuid>'` and `inst.GetConversationUUID() == "<uuid>"`.
- AC2 (requirements.md #2): if recovery finds nothing, the session still starts fresh — no hard failure.
  - *Given* the same `Instance` but with no matching JSONL file present in the fake home dir, *When* `inst.Start(false)` runs, *Then* `inst.LaunchCommand` does not contain `--resume`, `Start` returns `nil`, and `inst.Status == Running`.

**Files**: `session/instance.go`

##### Task 1.1.1a: Insert guarded pre-launch recovery call in `startLocked()` (~3 min)
- In `session/instance.go`, `startLocked()` (currently line 834), insert before the existing `i.initTmuxSession()` call (currently line 847):
  ```go
  if !firstTimeSetup && !i.pm().IsAlive() && !i.HasClaudeSession() {
      // Recover a persisted conversation UUID from disk BEFORE initTmuxSession()
      // reads i.claudeSession.ConversationUUID to decide whether to embed --resume.
      // i.pm().IsAlive() is guaranteed false here (tm.session is nil until
      // initTmuxSession() registers it below via SetSession()), so
      // tryExtractConversationUUID's internal PID fast-path is a no-op and it
      // falls straight to the DetectByPath fallback — guarded by
      // conversationClearedAt against resurrecting an explicitly-cleared
      // conversation (see ClearConversationState).
      i.tryExtractConversationUUID()
  }

  i.initTmuxSession()
  ```
- Files: `session/instance.go`

##### Task 1.1.1b: Insert the identical guarded call in legacy `start()` (~3 min)
- In `session/instance.go`, `(i *Instance) start(...)` (currently line 1012), insert the same block (identical comment) before the existing `i.initTmuxSession()` call (currently line 1029).
- This is required, not optional: `session/instance_cold_restore_test.go`'s existing tests use `StartWithCleanup()` → `start()` exclusively (features.md §2), so the AC4 regression test in Story 1.2.1 will not exercise the fix unless both call sites are patched.
- Files: `session/instance.go`

##### Task 1.1.1c: Fix the stale doc comment on `tryExtractConversationUUID` (~2 min)
- In `session/instance_claude.go`, replace the doc comment line "The tmux session must be alive for this to work, because it inspects the foreground process's open file descriptors via proc_pidinfo." with a corrected version noting the `DetectByPath` fallback also runs (and is now the expected path) when the tmux session is dead, per architecture.md §2's finding that this comment predates the fallback and could mislead a future reader into thinking the pre-launch call site added in Task 1.1.1a/b is unsafe.
- Also correct the adjacent "IMPORTANT: This method assumes stateMutex is already held by the caller" claim (currently lines 302-304): it is inaccurate today (`SwitchWorkspace` already calls it lock-free by design, `session/instance_workspace.go:76-81`) and would become more misleading once Task 1.1.2d adds an explicit `claudeSessionMu.RLock()` inside the function itself. Replace with a note that the function does not require or assume any caller-held lock; it manages its own locking internally for `conversationClearedAt` (Task 1.1.2d) and, like today, directly mutates `i.claudeSession`/`i.HistoryFilePath` without a lock — a pre-existing, out-of-scope exposure this plan does not change beyond the new field. Flagged in engineering triad review, 2026-08-10.
- Files: `session/instance_claude.go`

---

#### Story 1.1.2: Guard the pre-launch recovery against resurrecting an explicitly-cleared conversation
**As a** user who called "start new conversation" (`ClearConversationState`), **I want** the next cold restart to honor that choice, **so that** an unrelated crash/restart doesn't silently re-resume the conversation I just discarded.

See ADR-001 for the full rationale — this closes a concrete regression this plan's reorder would otherwise introduce: `session/instance_claude.go:78-96`'s `recoverFromStaleResume()` calls `i.ClearConversationState()` then `i.Start(false)`; because `remain-on-exit` keeps the tmux session object alive after the wrapped `claude` process exits, that particular `Start(false)` call takes the hot-restore branch (not cold-restore) and does not relaunch the program. `session/health.go:213-228`'s dead-pane recovery loop is the piece that actually re-launches it: it explicitly calls `instance.KillSession()` then `instance.Start(false)` specifically because "`Start(false)` treats an existing (even dead-paned) tmux session as already running." *That* `Start(false)` call *does* take the cold-restore branch, with the in-memory UUID freshly emptied by `ClearConversationState()` moments earlier and the old (stale/rejected) JSONL still sitting on disk as the newest file in the directory — exactly the input this fix's new pre-launch `DetectByPath` call would otherwise resurrect, recreating the same stale-resume failure and looping.

**Acceptance Criteria**:
- AC3 (requirements.md #3): when recovery is suppressed because the only candidate predates an explicit clear, a diagnosable signal remains in the logs.
  - *Given* an `Instance` whose `ClearConversationState()` was called (setting `conversationClearedAt = T0`), and a `<uuid>.jsonl` fixture on disk with mtime `T0 - 1h` (predates the clear), tmux dead, *When* `inst.tryExtractConversationUUID()` runs (directly, or transitively via `Start(false)`), *Then* `inst.claudeSession.ConversationUUID` remains `""` (no `--resume` embedded) and a log line `"tryextractconversationuuid: found jsonl predates last explicit clear, skipping recovery"` is emitted, distinguishable from the pre-existing `"no jsonl file found"` line used when there was truly never any conversation.
- A JSONL written *after* the clear (e.g. a genuinely new conversation started post-clear, then interrupted again) is still recovered normally — the guard does not disable recovery permanently.
  - *Given* the same `Instance` but the fixture JSONL's mtime is `T0 + 1h` (postdates the clear), *When* `inst.tryExtractConversationUUID()` runs, *Then* `inst.claudeSession.ConversationUUID` is set to the fixture's UUID.

**Files**: `session/instance.go`, `session/instance_claude.go`, `session/history_detector.go`

##### Task 1.1.2a: Add `conversationClearedAt` field to `Instance` (~2 min)
- In `session/instance.go`, add a new unexported field next to `claudeSession`/`claudeSessionMu` (currently lines 298-303):
  ```go
  // conversationClearedAt records when ClearConversationState() last ran.
  // Read by tryExtractConversationUUID's DetectByPath fallback so a cold-restore
  // recovery attempt does not resurrect a JSONL that predates an explicit
  // "start fresh" request. In-memory only (not persisted): does not survive a
  // stapler-squad process restart — see ADR-001, Consequences.
  //
  // Guarded by claudeSessionMu (same lock as claudeSession/HistoryFilePath),
  // NOT by i.mu — see Task 1.1.2d for why tryExtractConversationUUID must take
  // claudeSessionMu itself around the read/compare instead of relying on a
  // caller-held lock.
  conversationClearedAt time.Time
  ```
- `time` is already imported in `instance.go` (used by `CreatedAt`/`UpdatedAt`).
- Files: `session/instance.go`

##### Task 1.1.2b: Set `conversationClearedAt` in `ClearConversationState()` (~2 min)
- In `session/instance_claude.go`, inside `ClearConversationState()` (currently lines 278-296), inside the existing `claudeSessionMu.Lock()` critical section, add `i.conversationClearedAt = time.Now()` alongside the existing `i.claudeSession.ConversationUUID = ""` / `i.HistoryFilePath = ""` writes.
- Files: `session/instance_claude.go`

> **Concurrency note (added after engineering triad review, 2026-08-10):** `tryExtractConversationUUID` is called from at least one site that deliberately does *not* hold the actor lock — `SwitchWorkspace` (`session/instance_workspace.go:76-81`) calls it outside `sendSyncErr`'s actor command specifically to avoid holding the mailbox slot during syscalls, accepting "false negatives are safe" for the pre-existing unguarded `i.claudeSession` field access. `conversationClearedAt` is a *new* field this story adds under the same exposure: `ClearConversationState()` (writer) and `tryExtractConversationUUID()` (reader, added in Task 1.1.2d) can run on different goroutines with no lock ordering between them today. Unlike the pre-existing `i.claudeSession` race (out of scope — not introduced by this plan), this is a new field this plan is adding, so it must not ship unguarded. Task 1.1.2d below takes `claudeSessionMu.RLock()` around the read; no change needed here in 1.1.2a/1.1.2b beyond keeping the write inside the existing `claudeSessionMu.Lock()` section as already specified.

##### Task 1.1.2c: Add `ModTime` to `HistoryFileInfo` and populate it in `DetectByPath` (~3 min)
- In `session/history_detector.go`:
  - Add `"time"` to the import block.
  - Add `ModTime time.Time` to the `HistoryFileInfo` struct (currently lines 14-18), with a one-line doc comment: "ModTime is the on-disk mtime of the winning candidate. Populated by DetectByPath; left zero by Detect() since the PID-based fast path is process ground truth and doesn't need mtime gating."
  - In `DetectByPath` (currently lines 137-199), where the final `&HistoryFileInfo{...}` is constructed (currently lines 194-198), add `ModTime: time.Unix(0, best.modTime)`.
- Files: `session/history_detector.go`

##### Task 1.1.2d: Add the clearedAt-vs-ModTime guard inside `tryExtractConversationUUID` (~4 min)
- In `session/instance_claude.go`, `tryExtractConversationUUID()` (currently lines 308-363), in the `DetectByPath` fallback block (currently lines 336-349), after `info, err = detector.DetectByPath(effectivePath)` and its existing error-log check, insert:
  ```go
  if info != nil {
      i.claudeSessionMu.RLock()
      clearedAt := i.conversationClearedAt
      i.claudeSessionMu.RUnlock()
      if !clearedAt.IsZero() && !info.ModTime.After(clearedAt) {
          log.Debug("tryextractconversationuuid: found jsonl predates last explicit clear, skipping recovery",
              "session", i.Title, "path", info.HistoryFilePath, "clearedAt", clearedAt)
          info = nil
      }
  }
  ```
  placed before the existing `if info != nil { log.Info("tryextractconversationuuid: found conversation via path fallback", ...) }` line, so a discarded candidate falls through to the existing `if info == nil { log.Debug("...no jsonl file found...") }` branch below unchanged.
  **Take `claudeSessionMu.RLock()` around the read of `conversationClearedAt` explicitly** (do not rely on a caller-held lock) — `tryExtractConversationUUID`'s own doc comment claims "stateMutex is already held by the caller," but the real call site `SwitchWorkspace` (`session/instance_workspace.go:76-81`) calls it *outside* the actor lock by design, and the two cold-restore call sites (`session/instance.go:910`, `:1116`, both being moved earlier by Task 1.1.1a/b) don't hold `claudeSessionMu` either. Without this explicit lock, `ClearConversationState()` writing `conversationClearedAt` on one goroutine races with this read on another (flagged in engineering triad review, 2026-08-10) — a new race this plan would otherwise introduce, distinct from the pre-existing (out-of-scope, unguarded) `i.claudeSession` field race on the same call paths.
- Files: `session/instance_claude.go`

---

### Epic 1.2: Regression test coverage

#### Story 1.2.1: AC4 integration regression test
**As a** future contributor, **I want** an automated test proving the cold-start-with-disk-history path launches with `--resume`, **so that** this exact bug (fixed one call site too late) cannot silently regress.

**Acceptance Criteria**:
- AC4 (requirements.md #4): tmux dead, in-memory UUID empty, same-path JSONL present on disk → revived session launches with `--resume <uuid-from-jsonl>`.
  - *Given* `TestColdRestore_WithoutUUID_RecoversFromJSONL` in `session/instance_cold_restore_test.go`, an `Instance` built via `NewInstanceWithCleanup` with `SessionType: SessionTypeDirectory` and `Path: t.TempDir()`, `inst.historyDetector` set to `NewHistoryFileDetectorWithHomeDir(nil, fakeHome)` where `fakeHome` contains `<fakeHome>/.claude/projects/<ClaudeProjectDirName(inst.Path)>/550e8400-e29b-41d4-a716-446655440000.jsonl`, *When* `inst.StartWithCleanup(false)` is called, *Then* `assert.Contains(t, inst.LaunchCommand, "--resume")` and `assert.Contains(t, inst.LaunchCommand, "550e8400-e29b-41d4-a716-446655440000")`, and `inst.GetConversationUUID() == "550e8400-e29b-41d4-a716-446655440000"`.

**Files**: `session/instance_cold_restore_test.go`

##### Task 1.2.1a: Add `TestColdRestore_WithoutUUID_RecoversFromJSONL` (~5 min)
- Follow `TestColdRestore_WithUUID`'s structure (lines 44-97) but invert the setup: do not call `inst.SetClaudeSession(...)`; instead, after `NewInstanceWithCleanup`, set `inst.historyDetector = NewHistoryFileDetectorWithHomeDir(nil, fakeHome)` (`fakeHome := t.TempDir()`), pre-write the fixture JSONL at `filepath.Join(fakeHome, ".claude", "projects", ClaudeProjectDirName(inst.Path), "550e8400-e29b-41d4-a716-446655440000.jsonl")` with `os.MkdirAll` + `os.WriteFile(..., []byte("{}"), 0o644)` before calling `inst.StartWithCleanup(false)`.
- Assert per the Given-When-Then above; also keep the existing `Started()`/`Status == Running` assertions from the sibling tests for parity.
- Files: `session/instance_cold_restore_test.go`

---

#### Story 1.2.2: Unit tests for the `conversationClearedAt` guard
**As a** reviewer, **I want** fast, non-tmux unit tests pinning down the guard's exact boundary, **so that** the decision logic (not just the end-to-end launch command) is independently verifiable.

**Acceptance Criteria**:
- AC5 (requirements.md #5): no change to `session-resume-uuid-fix` behavior — a session that already has a stored UUID must not have it overwritten.
  - *Given* `TestTryExtractConversationUUID_SkipsWhenAlreadyHasID` (existing, `session/instance_workspace_test.go:23-40`) — an `Instance{claudeSession: &ClaudeSessionData{ConversationUUID: existingID}}`, *When* `inst.tryExtractConversationUUID()` runs, *Then* `inst.claudeSession.ConversationUUID == existingID`, unchanged. This test is not modified by this plan; Task 1.2.3a confirms it still passes after Story 1.1.2's changes.
- New: a JSONL older than `conversationClearedAt` is not used.
  - *Given* `Instance{Path: tmpDir, SessionType: SessionTypeDirectory, conversationClearedAt: T0, historyDetector: NewHistoryFileDetectorWithHomeDir(nil, fakeHome)}` with a fixture JSONL at mtime `T0 - 1h` under `fakeHome`, *When* `inst.tryExtractConversationUUID()` runs, *Then* `inst.claudeSession` is either `nil` or has `ConversationUUID == ""`.
- New: a JSONL newer than `conversationClearedAt` is still used.
  - *Given* the same setup but the fixture JSONL's mtime is `T0 + 1h`, *When* `inst.tryExtractConversationUUID()` runs, *Then* `inst.claudeSession.ConversationUUID` equals the fixture's UUID.

**Files**: `session/instance_cold_restore_test.go` (or a new small test alongside it — see Task 1.2.2a), `session/history_detector_test.go`

##### Task 1.2.2a: Add `TestTryExtractConversationUUID_SkipsStaleJSONLAfterClear` (negative case) (~4 min)
- In `session/instance_cold_restore_test.go`, add a fast unit test (no tmux, no `checkTmuxAvailable`/`coldRestoreSocket`): construct a bare `&Instance{Title: ..., Path: tmpDir, SessionType: SessionTypeDirectory, conversationClearedAt: time.Now()}`, set `historyDetector`, write a fixture JSONL via `os.Chtimes` to an explicit past time (`conversationClearedAt.Add(-1 * time.Hour)`), matching `history_detector_test.go`'s existing `os.Chtimes`-with-explicit-times convention (not wall-clock/sleep-based ordering).
- Assert `inst.claudeSession == nil || inst.claudeSession.ConversationUUID == ""` after calling `inst.tryExtractConversationUUID()`.
- Files: `session/instance_cold_restore_test.go`

##### Task 1.2.2b: Add `TestTryExtractConversationUUID_RecoversJSONLNewerThanClear` (positive case) (~4 min)
- Same shape as 1.2.2a but `os.Chtimes` sets the fixture's mtime to `conversationClearedAt.Add(1 * time.Hour)`.
- Assert `inst.claudeSession.ConversationUUID` equals the fixture UUID after `inst.tryExtractConversationUUID()`.
- Files: `session/instance_cold_restore_test.go`

##### Task 1.2.2c: Extend `history_detector_test.go` to assert `ModTime` is populated (~3 min)
- In `session/history_detector_test.go`, extend `TestHistoryFileDetector_DetectByPath_PicksMostRecentWhenMultiple` (currently lines 202-228) with one additional assertion after the existing `assert.Equal(t, uuid2, info.ConversationUUID, ...)`: `assert.WithinDuration(t, future, info.ModTime, time.Second, "ModTime should reflect the winning candidate's mtime")`.
- Files: `session/history_detector_test.go`

---

#### Story 1.2.3: Full validation gate
**As a** maintainer, **I want** the targeted and full test/lint suite green before this ships, **so that** AC6 is met with evidence, not assertion.

**Acceptance Criteria**:
- AC6 (requirements.md #6): `make quick-check` (build + test + lint) stays green.
  - *Given* all of Epic 1.1 and Epic 1.2's changes applied, *When* `make quick-check` is run from the repo root, *Then* it exits `0` with no build errors, no failing tests, and no new lint findings.

**Files**: none (validation only)

##### Task 1.2.3a: Run the targeted test subset (~3 min)
- Run `go test ./session -run "TestTryExtractConversationUUID|TestColdRestore|TestHotRestore|TestHistoryFileDetector" -v` (requires `make build` to have generated protos first, per this repo's `go test ./server/services` convention). Confirm all pass, including the pre-existing `TestTryExtractConversationUUID_SkipsWhenAlreadyHasID` and `TestIsStaleResumeExit`.
- Files: none (test execution only)

##### Task 1.2.3b: Run `make quick-check` (~5 min)
- Run `make quick-check` from the repo root. Fix any build/lint findings surfaced by the new field/log line (e.g. `gofmt -w .` if formatting drifted) before considering the plan complete.
- Files: none (validation only)
