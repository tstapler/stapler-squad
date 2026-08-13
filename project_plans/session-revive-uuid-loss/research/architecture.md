# Architecture Research: session-revive-uuid-loss

## Current code shape (verified against HEAD, `git -C . rev-parse HEAD` not pinned here — line numbers below are live-read, not from the stale requirements.md estimate)

`session/instance.go`'s `startLocked` has the cold-restore block at **L878-921**
(inside the `!firstTimeSetup` branch) and its restart-path mirror at **L1068-1127**
(second `!firstTimeSetup` implementation, same function shape, ~90% textually
identical). Both do, in order:

```
if !i.pm().IsAlive() {
    startPath := i.resolveStartPath(...)
    if i.HasClaudeSession() {          // <-- decision made here
        // log "cold restoring with --resume"
    } else {
        // log.Warn "starting fresh"    // <-- AC3's "only a log line"
    }
    ...VNC/CDP setup (unrelated)...
    i.pm().Start(startPath)
    i.pm().RestoreWithWorkDir(startPath)
    if i.claudeSession != nil { i.claudeSession.ConversationUUID = ""; i.HistoryFilePath = "" }
    i.tryExtractConversationUUID()      // <-- recovery attempt happens AFTER
}
```

`i.tryExtractConversationUUID()` (`session/instance_claude.go:308-363`) already
has exactly the recovery logic Goal 1 asks for: it early-returns if a UUID is
already set (line 310-312), otherwise tries live-pane inspection, then falls
back to `detector.DetectByPath(i.GetEffectiveRootDir())`. Because the pane is
provably dead in this branch (we're inside `!i.pm().IsAlive()`), the live-pane
fast path is always a no-op here and the call degrades to pure path-based
detection — so the fix is not "write new recovery logic," it's "invoke recovery
before the branch instead of after."

## Where to put "attempt recovery before deciding"

**Recommendation: extract a shared unexported helper, called from both blocks,
not inline duplication.** AC4 explicitly requires no divergent duplicated
logic, and the two blocks already show duplication drift (compare their
comments at L906-909 vs L1107-1111 — same intent, reworded independently),
which is exactly the failure mode AC4 is guarding against for the new code.

Proposed shape (illustrative, not final — plan phase should refine):

```go
// coldRestoreDecision resolves whether a cold restore (tmux pane dead) should
// resume via --resume or start fresh, attempting on-disk UUID recovery first
// so a JSONL transcript that exists but was never linked in memory is not
// discarded. Must be called from inside an actor command (startLocked or its
// mirror) — it mutates i.claudeSession fields directly, same as
// tryExtractConversationUUID which it wraps.
func (i *Instance) coldRestoreDecision() (resume bool, hadPriorHistorySignal bool) {
    if i.HasClaudeSession() {
        return true, false
    }
    hadHistoryFilePathBefore := i.HistoryFilePath != "" // read before recovery mutates it
    i.tryExtractConversationUUID()  // path-fallback only; pane is dead here
    if i.HasClaudeSession() {
        return true, false
    }
    return false, hadHistoryFilePathBefore // or a persisted "ever had a UUID" signal — see risk below
}
```

Both cold-restore blocks would call this once, before the `if i.HasClaudeSession()`
branch, and use the returned `resume` bool instead of re-calling `HasClaudeSession()`.
This also naturally satisfies AC2 (recovery finding nothing → falls straight
through to the existing fresh-start code, no behavior change, one extra
`DetectByPath` stat-directory call — cheap, same call the code already makes
seconds later).

Do **not** try to unify the two blocks further than this (e.g. collapsing all
~90 lines into one call site) — that's a larger refactor than this bug fix
warrants and each block has its own surrounding VNC/CDP/worktree wiring that
isn't part of this bug. Keep the extraction scoped to the decision logic only.

## Integration points

1. **`HistoryFileDetector.DetectByPath`** (`session/history_detector.go:137-182`)
   — already the mechanism in play; no new detector needed. See risk #1 below
   for a correctness gap in how it's invoked today.
2. **Persistence** — `SetHistoryInfo` (`session/instance_claude.go:464-499`)
   already fires `claudeSessionIDSavedCallback` synchronously on UUID change
   (commit `d9816ec77`, 2026-08-02), which the service layer wires to an
   immediate `SaveInstances` write. `tryExtractConversationUUID`, however,
   mutates `i.claudeSession` **directly** (no `SetHistoryInfo`, no callback —
   see "Locking" section) — so a recovery that happens inside the cold-restore
   block, same as today's post-decision call, does **not** by itself trigger a
   durable save. That's pre-existing behavior (unchanged by moving the call
   earlier), but worth flagging: if the plan phase wants the recovered UUID
   persisted immediately rather than at the next incidental `SaveInstances`
   sweep, route through `SetHistoryInfo` instead of setting `i.claudeSession`
   fields inline, or have the caller persist explicitly after `startLocked`
   returns.
3. **Notification/event system for AC3** — reuse, don't invent. Two existing
   layers, and the fix needs both:
   - `session.LifecycleEvent` / `LifecycleListener` (`session/instance.go:69-95`,
     registered per-instance in `server/services/session_service.go`'s
     `wireCallbacks`) is the low-level signal from `session` → `server/services`.
     `startLocked` already calls `i.fireLifecycleEvent(EventStarted, "")` at
     L999/L1225 after every successful start. The `reason` string parameter
     is unused for `EventStarted` today but exists precisely for this: pass a
     defined reason constant (e.g. `session.ReasonColdRestoreLostHistory`) so
     a listener can distinguish "resumed cleanly" from "forced fresh after a
     failed recovery attempt" without inventing a new `LifecycleEvent` value.
     Existing precedent for a `reason`-string-only distinction:
     `review_queue_poller.go:458/473/497` fires `EventExited`/`EventStarted`
     with reasons like `"reconcile-session-missing"` / `"reconcile-session-revived"`.
   - `pkg/events.NewNotificationEvent` + `eventBus.Publish` (used from
     `server/services/session_service.go`, e.g. `onRateLimitRecovery` at
     L4001-4021) is the **durable, user-visible** half AC3 asks for —
     notifications published this way are persisted by `notificationStore`
     (see `server/services/notification_service.go`'s doc comment,
     `GetNotificationHistory`), not just an ephemeral pub/sub event, and they
     already reach the frontend's notification surface. `onRateLimitRecovery`
     is architecturally the closest existing analog to this bug's "auto-heal
     action the user must be told about" (matches the
     `feedback_document_ai_decisions_in_edge_cases` memory convention already
     cited in requirements.md Goal 3) — a new `onColdRestoreLostHistory(inst,
     sessionID)` following that exact shape (title/message/priority, gated on
     `!inst.Hidden`) is the natural landing spot, wired as a new
     `LifecycleListener` (same registration pattern as `autoArchiveListener`
     / `sessionExitedPublisher`) rather than calling `eventBus` from
     `session/instance.go` directly — `session` package does not import
     `server/services`/`pkg/events` (see `session/backlog_item_change.go:65`'s
     comment on the reverse-import-cycle constraint and its `Notifier`
     adapter pattern — follow that same shape here).
   - Frontend: `web-app/src/components/sessions/StatusBadge.tsx` /
     `SessionCard.tsx` already render `DetectedStatus`/`AttentionReason`-driven
     badges, and `history_file_path` / `claude_conversation_uuid` are already
     proto fields (`proto/session/v1/types.proto:146,150`) surfaced to the
     client. A dedicated `AttentionReason` value is *not* recommended for this
     — that enum drives the review-queue determiner
     (`session/review_queue_determiner.go`), a heavier, differently-scoped
     mechanism than "tell the user this one session lost its thread." The
     notification-history entry is sufficient for AC3's "at minimum" bar;
     don't couple this fix to the review-queue subsystem.

## Locking / actor-safety

`startLocked` runs as an actor command (`instanceState` capability token,
`session/actor.go`) — the calling goroutine is serialized per-instance, so no
new lock is needed to protect the *decision* itself. The constraint is entirely
about the calls the decision helper makes:

- `HasClaudeSession()` and `tryExtractConversationUUID()` already run today
  from inside this same actor context (existing code at L881, L921, L1072,
  L1127) — the proposed helper does not change this exposure.
- **Pre-existing inconsistency worth flagging, not introducing**:
  `tryExtractConversationUUID` mutates `i.claudeSession` fields *directly*,
  without taking `claudeSessionMu` (its doc comment at L302-304 says it
  "assumes stateMutex is already held by the caller" — stale terminology,
  there is no separate stateMutex, this predates the `claudeSessionMu` rename).
  Contrast with `SetHistoryInfo`/`ClearConversationState`, which take
  `claudeSessionMu.Lock()` then nest `i.mu.Lock()` around the write *and* the
  `buildSnapshot`/`i.snapshot.Store` call (documented lock order:
  `claudeSessionMu` outer, `i.mu` inner — see the comment block at
  `instance_claude.go:281-287`, "the only lock order used anywhere for these
  two locks"). Readers like `GetClaudeSession()`/`HasClaudeSession()`
  (`instance_claude.go:253-273`) *do* take `claudeSessionMu.RLock()`. Moving
  the `tryExtractConversationUUID()` call earlier in `startLocked` does not by
  itself introduce a new race (it's the same unlocked-write pattern the code
  already has), but if the plan phase adds any *new* field write here (e.g. a
  "recovery attempted and failed" flag for Goal 2, a
  `hadPriorHistorySignal` marker, or switches the recovery write to go through
  `SetHistoryInfo` per the persistence recommendation above), that write must
  follow the documented `claudeSessionMu` → `i.mu` order and go through
  `buildSnapshot`/`i.snapshot.Store`, exactly like `SetHistoryInfo` does — do
  not add a third, differently-ordered locking path.
- No cross-instance locking is implicated; this is entirely single-instance
  state.

## Two correctness risks the plan phase must address (not just "where to call it")

### Risk 1 — `DetectByPath` picks "newest JSONL in the directory," with no
correlation to *this* session's prior identity

`project_plans/session-resume-uuid-fix/implementation/plan.md` documents a
real, previously-shipped bug: `HistoryLinker.correlateSession` (a *different*
caller of the same `DetectByPath`) can attribute another session's newer JSONL
file to a paused/hibernated session sharing the same project directory. The
fix there was a `pathFallbackAllowed` gate keyed on `alreadyLinked` +
`inst.Status` (`session/history_linker.go:274-276`).

`tryExtractConversationUUID` — the function this new fix leans on — calls the
exact same `DetectByPath` (`session/history_detector.go:137`, confirmed: picks
the candidate with the largest `modTime` among all valid-UUID `.jsonl` files in
`~/.claude/projects/<encoded-path>/`, no other disambiguation) with **no
equivalent guard**. It only avoids clobbering an already-set UUID (early return
at L310-312) — it does not check `inst.Status`, and by definition **cannot**
check "already linked," because this bug's whole premise (Goal 1) is that
`ConversationUUID` is empty when we call it. So: two other sessions sharing an
effective root dir (common for `SessionTypeDirectory` sessions, or any two
worktree sessions that happen to resolve to the same encoded path) + a dead
tmux pane + no in-memory UUID → recovery will confidently attach this session
to *the other session's* newer JSONL, not "no recovery." That's arguably worse
than AC2's "start fresh" outcome (silent conversation swap vs. a visible fresh
start).

**This needs a plan-phase decision**, not just an implementation detail:
possible mitigations —
  - Track `HistoryFilePath` even when `ConversationUUID` is cleared (currently
    both are wiped together at L910-913/L1118-1121 and in
    `ClearConversationState`), and only trust `DetectByPath`'s result when it
    matches the last-known `HistoryFilePath` for *this* session — else treat
    it as "ambiguous," not "recovered" (falls into Goal 2/AC3's "recovery
    attempted and still failed" bucket rather than confidently resuming).
  - Or thread `inst.Status`-style caution through to `tryExtractConversationUUID`
    the same way `correlateSession` does.
  - At minimum, this should be called out explicitly in `plan.md` and covered
    by a test analogous to `TestHistoryLinker_CorrelateSession_PausedSession_PreservesUUID`
    but for this new call site.

### Risk 2 — Intentional clears vs. accidental loss are indistinguishable today

`ClearConversationState()` is called from two **deliberate** "start over" call
sites, both of which then flow back through this same cold-restore code path:
- `session/instance_program.go:66` (`SwitchProgram`, leaving the
  Claude/Antigravity family — a stale `--resume` UUID would target the wrong
  binary).
- `session/instance_claude.go:83` (`recoverFromStaleResume` — Claude already
  rejected `--resume <uuid>` as invalid/stale, so the whole point is to stop
  retrying that UUID and start clean; it then calls `i.RecoverFromStopped()`
  and `i.Start(false)`).

Both end up in the exact `!i.pm().IsAlive()` branch this fix modifies (the
pane is dead/being restarted in both cases). Naively calling
`tryExtractConversationUUID()`'s `DetectByPath` fallback before the decision
would, in the `recoverFromStaleResume` case, very likely **re-discover the
same stale UUID from the same on-disk JSONL file** (the file doesn't move or
get deleted just because Claude rejected resuming it) and resume with it
again — silently reintroducing the stale-resume loop `recoverFromStaleResume`
exists to break. This is precisely the regression AC6 ("No change to
legitimate first-time-setup or explicit 'start fresh' flows") prohibits, and
neither `ClearConversationState()` nor the two call sites currently record
*why* the clear happened — there's no flag on `Instance` distinguishing "I
cleared this on purpose, don't try to recover it" from "this session lost
track of its UUID due to a race."

**This needs a plan-phase decision.** Candidate approaches:
  - Add an explicit "do not attempt recovery" signal set by
    `recoverFromStaleResume`/`SwitchProgram` alongside their
    `ClearConversationState()` call (e.g. a short-lived `skipNextRecovery
    bool` or an explicit `clearReason` on the instance, consumed and reset by
    the recovery helper) — mirrors the existing pattern of `reason` strings on
    lifecycle events.
  - Or scope recovery to only fire when the UUID was never intentionally
    cleared in this Start cycle — e.g. gate on whether `ClearConversationState`
    was the most recent mutator before this `Start()` call.
  - Whichever shape, this is the highest-risk gap found in this research pass
    and should be a named open question in `plan.md`, not discovered during
    implementation.

## Test coverage already in place to protect

`session/instance_workspace_test.go` already exercises
`tryExtractConversationUUID()` directly (multiple call sites) — any refactor
extracting a shared helper must keep these passing unmodified or update them
deliberately, not incidentally. `session/history_linker_test.go`'s
`TestHistoryLinker_CorrelateSession_PausedSession_PreservesUUID` /
`_HibernatedSession_PreservesUUID` are the reference shape for the new
same-directory-collision test Risk 1 calls for.
