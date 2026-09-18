# Research: Technology Stack — pr-fix-steering

## Summary

No new dependencies. This is a same-language, same-repo extension: one new
consumer-defined interface in `server/services`, one new wiring call in
`server/dependencies.go`, and a new branch inside an existing method
(`AutoReopenForPRFix`). Everything it needs — the steer-injection primitive,
the DI pattern, the notification helper, and a dedup/cooldown style — already
exists in the codebase and should be reused verbatim or by direct analogy.

Go version: `go 1.26.4` (go.mod:3). No new imports required beyond what
`server/services/backlog_service.go` and `server/services/session_service.go`
already import (`context`, `time`, `fmt`, `strings` for the dedup helper).

## 1. `SessionStopper` DI pattern — confirmed precedent for `SessionSteerer`

- **Interface definition**: `server/services/backlog_service.go:45-71` (`SessionStopper`).
  Doc comment at line 46: "allows BacklogService to kill live sessions. It is
  nil-safe: BacklogService degrades gracefully when not wired." Same shape
  should apply to `SessionSteerer` — optional, nil-checked before use.
- **Setter**: `SetSessionStopper` at `server/services/backlog_service.go:465-468`:
  ```go
  func (s *BacklogService) SetSessionStopper(stopper SessionStopper) {
      s.sessionStopper = stopper
  }
  ```
- **Wiring call site**: `server/dependencies.go:1191`:
  ```go
  backlogSvc.SetSessionStopper(sessionService)
  ```
  `sessionService` (a `*services.SessionService`) satisfies `SessionStopper`
  structurally — no explicit `implements` declaration, standard Go idiom. A
  `SetSessionSteerer(sessionService)` call belongs immediately alongside this
  line.
- **Implementation side**: each `SessionStopper` method on `SessionService`
  (`server/services/session_service.go:932-979`, e.g. `StopSessionByUUID`,
  `KillTmuxPaneOnly`, `IsSessionLive`) follows the same shape: resolve the live
  `*session.Instance` via `s.FindLiveInstance(sessionUUID)`
  (`server/services/session_service.go:869`), no-op (return nil, not error) if
  not found ("already gone"), then act on the `Instance`. A new
  `SteerSessionByUUID(ctx, sessionUUID, message string) error`-shaped method
  should follow this exact resolve-or-no-op pattern, then delegate to the same
  autonomous/interactive branch logic below instead of reimplementing it.
- **Why this avoids a circular import**: `SessionStopper`/`SessionSteerer`
  are declared in `server/services` (the consumer/caller package,
  `BacklogService`), not in `session` or as a method directly needing
  `SessionService` type — `session/git`. `services` package already imports
  `session`; `session` does not import `services`, so a
  `server/services`-local interface satisfied structurally by
  `*SessionService` is the only way to call from `BacklogService` into
  session-steering logic without a cycle. This mirrors the interface
  pollution checklist's guidance (`.claude/rules/interface-pollution-checklist.md`):
  the interface is scoped to exactly what `BacklogService` needs and lives in
  the consumer package, not next to `SessionService`'s implementation.

## 2. Existing steer machinery — confirmed exact branch to reuse

`UpdateSession`'s `SteerMessage` handling,
`server/services/session_service.go:2929-2972`:

- Guard: `len(*req.Msg.SteerMessage) > session.MaxSteerMessageLength` —
  constant defined at `session/instance.go:139` (`const MaxSteerMessageLength = 10000`).
- **Autonomous branch** (`instance.AutonomousMode == true`, lines 2934-2943):
  ```go
  controller := instance.GetController()
  if controller != nil {
      if _, sendErr := controller.SendCommandImmediate(*req.Msg.SteerMessage + "\r"); sendErr != nil {
          log.Warn(...)
      } else {
          s.notifySteerSent(instance, *req.Msg.SteerMessage)
      }
  }
  ```
  Doc comment above (line 2924): "Autonomous sessions keep the existing
  ClaudeController command-queue path (ADR-001)." A send failure here is
  logged, not returned — matches this feature's requirement to log success/
  failure without blocking the reconcile tick.
- **Interactive branch** (lines 2944-2971): builds submittable input via
  `session.BuildSubmittableInput(*req.Msg.SteerMessage, true)`
  (`session/instance_tmux.go:1013`), sends via `instance.SendKeys(text)` on a
  goroutine with a 5-second timeout (`context.WithTimeout(ctx, 5*time.Second)`),
  select on `errCh`/`timeoutCtx.Done()`. This branch *returns* an error to the
  caller on failure/timeout — the new `SessionSteerer` method should decide
  whether to surface or just log, per this feature's requirement to record a
  backlog notification either way rather than propagating a hard RPC error.
- `notifySteerSent(instance, message)` — existing hook already called from
  both branches on success; worth checking if it already writes a
  visible record that could double as (or be extended for) this feature's
  "visible backlog-item comment/notification recording each steer attempt"
  requirement, or whether a separate backlog-specific notification (mirroring
  `notifyRespawnBlockedByActiveSession`, see §4) is more appropriate since
  `notifySteerSent` is scoped to the UI/MCP-originated steer path, not backlog
  activity.

`server/mcp/tools_terminal.go:638-706` (`steerSession` MCP tool) — same two
branches in a third caller, confirming this is a stable, exercised pattern:
1. OneShot + Stopped + has conversation UUID → `inst.RunWithResume(ctx, message)`
   subprocess resume (lines 664-681) — a third path not present in
   `UpdateSession`, for sessions that have already exited.
2. Otherwise, PTY `SendKeys` fallback (lines 683-706) — same 5-second-timeout
   goroutine/select shape as the interactive branch above.

**Reliability against a busy session** (Feasibility Risk in requirements):
both `SendCommandImmediate` (ClaudeController command queue) and `SendKeys`
(raw PTY write) are the same primitives already used for browser- and
MCP-triggered human steering today — this project does not need to prove
new reliability characteristics, only add a third caller. `SendKeys` is a
raw terminal write, so it lands whenever tmux delivers it regardless of
Claude's generation state — this is inherent to PTY injection, not a bug to
fix here.

## 3. Program-awareness for slash-command gating

No `Program` enum exists — `Instance.Program` is a plain `string` field (e.g.
`"claude"`, `"claude --model sonnet"`, `"aider"`; see
`session/hibernation_sweeper_test.go:103`, `session/create_managed_instance_test.go:33`).
The established idiom for "is this a Claude Code session" is
`ClaudeAdapter.CanHandle` (`session/claude_adapter.go:27-29`):
```go
func (a *ClaudeAdapter) CanHandle(program string) bool {
    return strings.Contains(strings.ToLower(program), "claude")
}
```
The requirements' constraint ("must check `instance.Program` before including
a slash-command instruction") should reuse this exact
`strings.Contains(strings.ToLower(instance.Program), "claude")` check (or call
`ClaudeAdapter.CanHandle` / `NewClaudeAdapter().CanHandle` directly if that
type is already constructed somewhere reachable) rather than an exact-match
comparison — `Program` can carry flags/args appended to the base command.

## 4. Notification/audit-trail pattern to extend

`notifyRespawnBlockedByActiveSession`
(`server/services/backlog_service_triage.go:1363-1395`) is the existing
audit-trail helper for this exact blocked-respawn branch:
- Logs `[AutoReopenForPRFix] item %s already has an active session %s;
  skipping respawn — %s` (matches this feature's required `[AutoReopenForPRFix]`
  log prefix convention).
- Calls `s.storage.MarkStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive, ...)`
  then `MarkStuckNotified`.
- Publishes unconditionally via `s.eventBus.Publish(events.NewNotificationEvent(...))`
  with `NOTIFICATION_TYPE_INFO` / `NOTIFICATION_PRIORITY_LOW`.
- `resolveRespawnBlockedActiveLogged` (line ~1401) clears the stuck-reason row
  once the guard clears — called from all three `notifyRespawnBlockedByActiveSession`
  callers today.

Per the requirements ("extending the existing
`notifyRespawnBlockedByActiveSession` audit trail rather than replacing it"),
the new steer branch should call `notifyRespawnBlockedByActiveSession` (or a
close variant/extension of it) for the log+stuck-row+notification side effects
that already exist, then *additionally* fire the steer call and record its own
outcome (delivered / failed) — likely as a second `events.NewNotificationEvent`
publish or an extra field/message on the existing one. The exact shape (new
helper vs. parameterizing the existing one) is a planning-phase decision, not
a stack question — but the underlying storage/event API surface is entirely
existing (`s.storage.MarkStuck/MarkStuckNotified/ResolveStuck`,
`s.eventBus.Publish`, `events.NewNotificationEvent`) with no new dependency.

The call site to modify is `AutoReopenForPRFix`'s active-session branch,
`server/services/backlog_service_triage.go:2048-2051`:
```go
if active := findActiveWorkSession(sessions); active != nil {
    s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
    return nil
}
```
`fixContext` (the problem description string, built earlier in
`AutoReopenForPRFix` and used again at line 2100 for the respawn-path prompt:
`fmt.Sprintf("[PR Fix context - PR #%d (%s)]\n%s", item.PrNumber, item.PrURL, fixContext)`)
is already computed before this branch runs and is exactly the content the
steer message should carry — no new problem-description logic needed, just a
new sink for the same string (trimmed/reformatted for a steer message vs. a
spawn prompt, and gated on `instance.Program` per §3 for the `/github:pr-ship`
suffix).

## 5. Dedup/cooldown pattern — `session/nudge_dedup.go`

`session/nudge_dedup.go` (76 lines, package `session`) is a **pure-function**
pair plus a small value type, no I/O:
- `lastNudge{text, at time.Time, pane string}` — bundles related state instead
  of separate primitives (explicitly justified in its own doc comment as
  avoiding "easily-swappable primitive parameters" — same rationale as this
  repo's `primitive-obsession-checklist.md`).
- `isDuplicateNudge(candidate string, n lastNudge, now time.Time, currentPane string) bool` —
  true only if: same normalized text, within `nudgeCooldown` (3 min, a `var`
  not `const` so tests can shrink it), AND the pane tail hasn't produced new
  output since the last nudge (pane-snapshot re-arm — new activity bypasses
  cooldown even for identical text).
- `nextLastNudge(prev lastNudge, nextMsg string, delivered bool, pane string) lastNudge` —
  computes the next state; only advances on confirmed delivery.
- `normalizeNudgeText`/`normalizePaneSnapshot` — whitespace/case normalization
  helpers.

**Assessment for this feature's reason-signature dedup**: the *shape* (pure
functions, a small bundled state struct, a `var` cooldown constant for
testability, "advance only on confirmed delivery") is directly reusable as a
pattern, but the *content* being compared differs enough to warrant a
sibling helper rather than literally reusing `isDuplicateNudge`:
- `nudge_dedup.go` compares free-text nudge strings with fuzzy
  normalization + a live pane-snapshot re-arm (it's deduping *idle-driver*
  nudges against an autonomous session's own recent PTY output).
- This feature's dedup key is structured, not free text: the
  `CIFailing`/`HasBlockingReviews`/`HasConflicts` boolean tuple plus a coarse
  conflict/CI detail string (per requirements' Scope section) — a reason
  *signature*, not a message string. There's no PTY pane-snapshot signal
  naturally available at the `BacklogService` reconciler layer (that lives in
  `session.Instance`, a different package/abstraction level), and the
  requirements' own Rabbit Holes section flags "redundant nudge detection"
  (checking whether the agent's own recent output already shows it noticed)
  as a *research question*, not a committed requirement — the committed
  Success Metric only requires reason-signature-changed detection, not
  pane-aware re-arming.
- Recommendation: write a small new pure-function helper (same style:
  a `prSteerSignature{ciFailing, hasBlockingReviews, hasConflicts bool;
  detail string}`-shaped value, an `isDuplicateSteer(candidate, last
  prSteerSignature, now, cooldown) bool`-style function, advance-only-on-
  delivery) living either in `server/services` (co-located with
  `AutoReopenForPRFix`) or as a new small file in `session/` if it should be
  testable independent of `BacklogService` — but it is a **variant informed
  by** `nudge_dedup.go`'s pattern, not a call into its existing functions,
  since the comparison key and there being no pane-snapshot signal at this
  layer differ. This should be confirmed/decided in the planning phase, but
  nothing here indicates a new dependency or a different testing approach
  than `nudge_dedup_test.go`'s existing table-driven, pure-function style
  already sets as precedent in this repo.
- Cooldown constant: the Open Questions section defers this to planning with
  a citation-backed rationale (mirroring how `maxReworkBlockStaleness` at
  `backlog_service_triage.go:1289` documents its own distinct threshold) —
  this is a value/tuning decision, not a stack/pattern question, so left to
  the plan phase rather than answered here.

## 6. No new dependencies

Everything referenced above resolves to symbols already imported in
`server/services/backlog_service.go`, `server/services/backlog_service_triage.go`,
`server/services/session_service.go`, `server/mcp/tools_terminal.go`,
`session/instance.go`, `session/instance_tmux.go`, `session/nudge_dedup.go`,
and `session/claude_adapter.go`. No new third-party Go module, no new proto
RPC (`UpdateSession`'s `SteerMessage` field and `steer_session`'s MCP schema
are unchanged — this project is a new *caller*, not an API change, per the
requirements' Out of Scope section), and no new generated code (`make
proto-gen`/`make ent-gen` not required).
