# Architecture Research: cold-start-uuid-loss

## 0. This bug has already been researched (and planned) twice — build on it, don't re-derive

Two prior SDD projects cover this exact bug in `session/instance.go`'s cold-restore logic:

- `project_plans/session-cold-start-uuid-loss/research/architecture.md` — full data-flow trace of
  every `ConversationUUID` writer, confirms `tryExtractConversationUUID` is the one setter that
  bypasses the `SetHistoryInfo`/`SetClaudeConversationUUID` persistence callback, and identifies a
  **second, more severe bug**: `initTmuxSession()` early-returns on `i.pm().HasSession()` (a
  pointer-non-nil check, not a liveness check) so an **in-process** restart reuses the exact
  `enrichedProgram` string (including `--resume`/no-`--resume`) baked in at the *first* start in
  that process's lifetime — the recovered UUID has no effect on the actual launch command unless
  recovery runs and updates `i.claudeSession` **before** `initTmuxSession()`, not just before the
  log line.
- `project_plans/session-revive-uuid-loss/` — went further: has `research/architecture.md`,
  `design/ux.md`, and a full `implementation/plan.md` (status: "Ready for implementation"),
  including a proposed `prepareColdRestore()` shared helper, a `RecoverySuppressed` field (to
  avoid re-discovering a UUID a deliberate `ClearConversationState()` just walked away from — the
  `recoverFromStaleResume` regression risk), an `EverHadConversationHistory` durable flag, and a
  `ReviveOutcome` enum threaded into a new notification.

**Verified: neither plan was implemented.** `grep -n "prepareColdRestore\|coldRestoreDecision\|RecoverySuppressed\|EverHadConversationHistory\|ReviveOutcome" session/*.go` returns zero
hits. The live code at HEAD still calls `i.tryExtractConversationUUID()` *after* `pm().Start()`,
exactly as both prior research docs describe it. These three `project_plans/*cold-start*`/`*revive-uuid*` directories are almost certainly duplicate SDD runs against the same backlog item/bug report on the same day (2026-08-06) — this project's job is to consolidate, verify, and correct
the prior work against current `HEAD`, not restart from zero. The rest of this document:
(a) answers this project's specific new question (which code path is *actually live* — the prior
docs left this as an open rabbit hole), and (b) re-verifies the prior docs' locking/actor claims
directly against current `session/actor.go` (which itself has visibly advanced since those docs
were written — the "additive snapshot infrastructure" the `instance-actor-concurrency` design doc
proposed as future work is now fully live code, not a proposal).

---

## 1. Which call path is live: `startLocked`, definitively

This is the one question the prior two research passes left as an open "Rabbit Hole" /
"Open Question." It's now resolved with a direct call-site trace, not inference from doc comments:

```
Instance.Start(firstTimeSetup bool) error           [session/instance.go:828]
    → i.sendSyncErr(func(s) error { return startLocked(s, firstTimeSetup) })

Instance.StartWithCleanup(firstTimeSetup bool)       [session/instance.go:1011]
    → i.start(firstTimeSetup, true, &cleanup)         [session/instance.go:1023, i.startMu.Lock()]
```

`grep -rn "StartWithCleanup" --include="*.go" .` across the whole repo returns **zero** call
sites outside `_test.go` files (`session/mcp_integration_test.go`,
`session/session_creation_test.go`, `session/comprehensive_session_creation_test.go`,
`session/instance_cold_restore_test.go`, `session/session_restart_test.go`,
`session/integration_test.go`). Every production call site — `session/session_driver.go:541`,
`server/mcp/tools_github.go:225`, `server/mcp/tools_lifecycle.go:174,347,450,466`,
`session/health.go:179,229`, `session/instance_claude.go:88` (`recoverFromStaleResume`),
`session/instance_serialization.go:405,456` (storage load path),
`session/instance_hibernate.go:110,154`, `server/dependencies.go:681,698,1107`,
`server/services/session_service.go:891,943,1591,3266` — calls `.Start(...)`, which routes
through `startLocked` via the actor mailbox.

**Conclusion: `start()`/`StartWithCleanup` (the `startMu`-locked block at `instance.go:1023-1150+`,
including its cold-restore duplicate at `:1068-1127`) is dead in production and reachable only
from tests.** `startLocked` (`instance.go:845-935`, cold-restore block at `:878-921`) is the sole
live path. `startLocked`'s own doc comment already says as much implicitly ("Differences from
`start()`... `startMu` and `restartMu` are retained; Epic 7 makes the final decision" —
`instance.go:841`, referring to the in-flight `instance-actor-concurrency` IAC migration's Epic 7,
"Final `stateMutex` Deletion," `project_plans/instance-actor-concurrency/implementation/plan.md:2214`)
but the doc comment alone doesn't prove *nothing calls it*; the grep across every non-test `.go`
file does.

### Implication for the fix

- The fix's correctness-critical work belongs in `startLocked`'s cold-restore block
  (`instance.go:878-921`) — that's what real restarts (inactivity timeout, service restart,
  reboot-then-revive) actually execute.
- `start()`'s duplicate block (`:1068-1127`) is not reachable from any production trigger today.
  It's still worth updating for consistency (tests like `instance_cold_restore_test.go` exercise
  it directly and would otherwise assert stale behavior, and IAC Epic 7 hasn't scheduled its
  deletion — see `project_plans/instance-actor-concurrency/implementation/plan.md`, no
  `StartWithCleanup` removal task found in Epics 1-7), but it is **not** the "which one is
  authoritative" ambiguity the requirements' Rabbit Holes section worried about — there's no risk
  of "fixing one, leaving the other reachable with the bug intact" in production, because only
  one is reachable at all. The session-revive-uuid-loss plan already reached the same conclusion
  independently (a single shared `prepareColdRestore`-style helper, called from both) — that
  remains the right shape; this section just confirms the risk framing (both being live prod
  paths) was not actually the case.
- If plan phase chooses to extract a shared helper (recommended, per both prior docs' precedent of
  `resolveStartPath`/`setupFirstTimeWorktree`), call it identically from both `startLocked` and
  `start()` bodies — cheap to keep both in sync mechanically even though only one matters for the
  live bug, and it avoids leaving `start()`'s copy in a state that silently diverges further next
  time someone edits only the live path.

---

## 2. Actor/mailbox constraints on `tryExtractConversationUUID`/`DetectByPath`

`session/actor.go` (current HEAD, not the `instance-actor-concurrency` design doc's proposal —
that proposal is now substantially implemented) defines:

- `command func(i *Instance)` — a closure run by the single per-instance actor goroutine
  (`runActor`, `actor.go:121`).
- `instanceState{ inst *Instance }` — a capability token, only constructed inside
  `sendSyncErr`/`send`/`sendCtx` closures, proving a function executes on the actor goroutine.
  "Locked" twins (`startLocked`, etc.) take `*instanceState` and access `Instance` fields directly,
  "without stateMutex" (`actor.go:27-28`).
- `Instance.sendSyncErr` (`actor.go:33-52`) — blocks the calling goroutine until the actor
  processes the command; if `i.liveInstance.Load()` is `nil` (pre-construction / some test setups)
  it runs `fn` synchronously on the caller's own goroutine instead of enqueueing.

**Is the cold-restore decision already running on the actor thread in both branches?** Yes for
`startLocked` (it's invoked from inside `sendSyncErr`'s closure by construction — there is no way
to call `startLocked` except from within an actor command, since it takes `*instanceState` which
only `sendSyncErr`/`send`/`sendCtx` construct). `start()` is **not** actor-routed at all — it's the
pre-actor-migration `i.startMu.Lock()`-based implementation, confirmed dead in production per §1,
so it's serialized by a plain mutex instead of mailbox confinement. This matters only for the
test-only path; the live path (`startLocked`) is unconditionally actor-confined already, so a fix
placed inside `startLocked`'s cold-restore block does not need any new locking to be safe against
*other commands on the same instance* — single-goroutine confinement already guarantees that.

**What a fix must still be careful about is calls that reach outside the actor.**
`tryExtractConversationUUID()` (`instance_claude.go:308-363`) and `DetectByPath` themselves do no
locking of their own and don't reach outside the instance, so calling them earlier in
`startLocked` (before `initTmuxSession()`, per prior research's §1.3/§2.1 finding) is safe from
the actor-confinement angle. The actual constraint is about the **write** `tryExtractConversationUUID`
performs when it finds a UUID — see §3.

---

## 3. `claudeSessionMu` vs. `i.mu` — what a fix must not violate

Two separate locks protect two separate concerns, and the fix touches both indirectly:

### 3.1 `claudeSessionMu` — protects `i.claudeSession` against **non-actor** readers

`claudeSessionMu sync.RWMutex` (`instance.go:303`, comment: "protects `claudeSession` and
`claudeSessionIDSavedCallback`"). Its readers/writers (`GetClaudeSession`, `SetClaudeSession`,
`HasClaudeSession`, `GetConversationUUID`, `SetClaudeConversationUUID`, `SetHistoryInfo`,
`ClearConversationState` — all in `instance_claude.go`) take `claudeSessionMu.RLock()`/`.Lock()`
**directly**, not via `sendSync`/the actor mailbox. This is deliberate — these are meant to be
cheap, non-blocking reads/writes callable from any goroutine (RPC handlers, pollers) without
paying for a mailbox round-trip. That means **`claudeSessionMu` is the only thing serializing
`i.claudeSession` field access between the actor goroutine and every other goroutine in the
process** — actor-goroutine confinement (§2) does not cover this field, because these accessors
don't go through the actor.

**Pre-existing gap, confirmed still present at HEAD, that the fix must not perpetuate or worsen**:
`tryExtractConversationUUID` mutates `i.claudeSession.ConversationUUID` and `i.HistoryFilePath`
**directly at `instance_claude.go:357-361`, without taking `claudeSessionMu.Lock()`**. Its doc
comment (`instance_claude.go:302-304`) says "assumes stateMutex is already held by the caller" —
stale terminology (there is no separate `stateMutex`; this predates the `claudeSessionMu` rename,
confirmed by the session-revive-uuid-loss research pass reaching the same conclusion). Today this
is a live, if narrow, data race: a concurrent `GetClaudeSession()`/`HasClaudeSession()` call from
an RPC handler (`claudeSessionMu.RLock()`) can race the actor's unlocked write. It has presumably
gone unnoticed because the write happens late (after `pm().Start()`, near the end of the
cold-restore branch) and the window is short. **Moving this call earlier — the core of this fix —
does not shrink that window; if anything, making the recovered UUID load-bearing for the
resume-vs-fresh decision makes correctness here matter more, not less.**

Recommendation for the plan phase: whatever new/relocated call performs the recovery write should
take `claudeSessionMu.Lock()` around the mutation (matching `SetHistoryInfo`'s pattern, not
`tryExtractConversationUUID`'s), or route through `SetHistoryInfo` itself (which already does this
correctly **and** already fires the persistence callback — see prior research §2.2, "stop
bypassing the callback" is a two-birds fix: taking the lock and triggering the save are the same
code path). Do not add a third, differently-ordered locking pattern.

### 3.2 `i.mu` — protects `buildSnapshot()` against **legacy direct-lock writers**, not against actor commands

`i.mu sync.RWMutex` (`instance.go:369`) is **not** what makes actor commands like `startLocked`
safe — single-goroutine mailbox confinement does that on its own (`actor.go:107-109`: "actor
commands (the 'Locked' family) mutate Instance fields relying purely on actor-goroutine
confinement, without taking `i.mu` at all"). `i.mu` exists because a handful of *legacy* setters
(`MarkViewed`, `MarkUserResponded`, `MarkAcknowledged`, `SetLastMeaningfulOutput`,
`RecoverFromStopped`) still mutate fields directly from arbitrary caller goroutines under
`i.mu.Lock()`, bypassing the actor entirely (`actor.go:110-112`). `runActor`'s post-command
snapshot rebuild (`actor.go:129-132`) takes `i.mu.RLock()` around `buildSnapshot()` purely to
serialize against *those* writers, not against the actor's own just-completed command.

**Implication: a fix placed inside `startLocked` does not need to take `i.mu` at all** — it isn't
one of the five legacy direct-lock writers, and the snapshot publish that happens automatically
after the command returns (`actor.go:127-132`) already takes `i.mu.RLock()` for the reasons above.
`ClearConversationState`/`SetHistoryInfo` (`instance_claude.go:278-296`, `:464-492`) *do* take
`i.mu.Lock()` nested inside `claudeSessionMu.Lock()` — but that's because they're called from
**outside** the actor too (they're plain public methods, not actor-only "Locked" twins; grep
confirms `ClearConversationState` is called from `SwitchProgram`/`recoverFromStaleResume`, neither
of which is itself always inside an active actor command) and they must publish their own snapshot
synchronously rather than waiting for the actor's next post-command rebuild, since nothing
guarantees an actor command runs soon after. **If the plan phase reuses `SetHistoryInfo` as the
persistence write (recommended in §3.1), this `i.mu` nesting comes along "for free"** —
`SetHistoryInfo`'s existing body already does `claudeSessionMu.Lock()` → `i.mu.Lock()` →
mutate → `buildSnapshot()` → `i.mu.Unlock()` → `snapshot.Store()` → `claudeSessionMu.Unlock()`,
exactly the one documented lock order (`instance_claude.go:285-287`: "the only lock order used
anywhere for these two locks"). A fix should call it, not reimplement its locking inline inside
`startLocked`.

### 3.3 Net guidance for the plan phase

- Inside `startLocked`'s cold-restore block: reading `HasClaudeSession()` and calling a
  path-fallback recovery is fine with no new locking (actor confinement covers ordering against
  other commands on this instance; `HasClaudeSession()` already takes `claudeSessionMu.RLock()`
  internally).
- The **write** of a recovered UUID must go through `claudeSessionMu.Lock()` — reuse
  `SetHistoryInfo` rather than `tryExtractConversationUUID`'s direct-field-write pattern, which
  both closes the pre-existing data race (§3.1) and gets persistence (the prior research's other
  major finding — recovery today is memory-only until `HistoryLinker`'s async sweep or an
  incidental full save happens to run first) as a side effect of the same change.
- Do not add any new lock or reorder `claudeSessionMu`/`i.mu` relative to each other — the
  existing nesting (`claudeSessionMu` outer, `i.mu` inner, both released before
  `snapshot.Store()`... actually released in the order shown in `SetHistoryInfo`) is the only
  order used anywhere in the file and both prior research passes flag deviating from it as a named
  risk.

---

## 4. Event-Command-Policy table — skipped

Per instructions: this is a single-`Instance` revive-sequence bug fix inside an existing actor
command, not a multi-actor business domain. No saga, no cross-aggregate policy. `startLocked`/
`start()` are already mutually exclusive per-instance (actor mailbox / `startMu` respectively);
§1-§3 above are the complete concurrency picture this fix needs.
