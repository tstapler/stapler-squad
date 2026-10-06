# Implementation Plan: cold-start-uuid-loss

**Feature**: Attempt on-disk conversation-UUID recovery before the resume/fresh
decision is baked into a launch command — at every call site that builds one
from a possibly-stale in-memory UUID — and make a forced fresh-start after a
failed recovery durably visible to the user.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: None — see "Step 3: ADR judgment call" below for the explicit reasoning.

> **Provenance note.** This plan originated from
> `project_plans/session-revive-uuid-loss/implementation/plan.md` — the same
> underlying bug (`startLocked`/`start` deciding resume-vs-fresh from an
> in-memory UUID without attempting on-disk recovery first), retriaged under
> this project's name, `cold-start-uuid-loss`. That prior plan was never
> implemented. It has been repaired here to resolve
> `session-revive-uuid-loss`'s **adversarial review** (verdict BLOCKED, 2
> blockers) and **architecture review** (verdict CONCERNS, 1 blocker) before
> being adopted as this project's plan. Every Epic below that differs from the
> prior plan is called out explicitly; Epics/tasks the reviews did not flag
> are carried forward with only the anchor-line/field-name updates needed to
> stay internally consistent with the rewritten parts.

---

## Step 0.5: Creative pass — alternatives considered

1. **(a) Move the recovery attempt earlier + reorder `initTmuxSession()`;
   extend the same helper to `Resume()`/`Restart()`.**
   Strength: minimal, localized change at each of the four call sites —
   reuses one shared decision helper (`prepareColdRestore`) unchanged across
   `startLocked`, `start`, `Resume`, and `Restart`, and keeps
   `initTmuxSession()`'s single responsibility ("build a command from
   already-decided state") intact. Weakness: requires touching four
   call sites instead of two, and — per the adversarial review's Blocker
   1 finding against the prior plan — omitting any of the four reproduces
   the exact bug at that site.
2. **(b) Make `initTmuxSession()`/`buildLaunchCommand()` itself resume-aware**
   by re-resolving the UUID right before building the command, instead of
   trusting whatever `i.claudeSession.ConversationUUID` already holds.
   Strength: structurally guarantees the invariant "the launch command always
   reflects the freshest known UUID" at a single choke point. Weakness:
   `buildLaunchCommand()` is a pure string-assembly function called from four
   different contexts with different locking/liveness guarantees
   (`initTmuxSession()` inside the actor, `Resume()`/`Restart()` outside it);
   folding a `DetectByPath` scan into it would run disk I/O unconditionally
   on every call, including the hot-restore and first-time-setup paths where
   Goal 2/AC2 requires zero added work.
3. **(c) Two-phase: always launch with `--resume` if a UUID is *ever* found
   via recovery, reconcile after the fact if Claude actually started a new
   conversation anyway.**
   Weakness: unchanged from the prior plan's analysis — this is exactly the
   shape of bug `recoverFromStaleResume` (`session/instance_claude.go:78-96`)
   already exists to break, and still doesn't solve AC1 for "never captured
   any UUID."

**Chosen: (a), expanded to four call sites.** The prior plan chose (a) for
two call sites (`startLocked`/`start`); the adversarial review's Blocker 1
showed that scope was incomplete — `Resume()` and `Restart()` build launch
commands from the same kind of possibly-stale in-memory UUID and are both
reachable in production (`Resume` RPC handler; `SessionDriver`'s
`handleDriverFailure` non-`Stopped` branch and `SwitchProgram`). This plan
extends (a) to all four.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **ColdRestore** | The code branch taken inside `startLocked`/`start` when `!firstTimeSetup && !i.pm().IsAlive()` — the tmux pane is confirmed dead and a new tmux session must be created to either resume or start fresh. | Existing concept (`session/instance.go:878-921`, `:1068-1127`); unchanged from the prior plan. |
| **startCycleGeneration** | A `uint64` counter on `Instance`, incremented once at the top of every `prepareColdRestore()` call (i.e. once per attempted resume/fresh decision, regardless of which of the four call sites triggered it). In-memory only, not persisted. | New — **replaces** the prior plan's bare `recoverySuppressed bool` (see adversarial Blocker 2 below). |
| **recoverySuppressedGeneration** | A `uint64` field on `Instance`. Zero means "no suppression armed." A non-zero value `g` means "the very next `prepareColdRestore()` call — the one that observes `startCycleGeneration == g` after incrementing — must skip recovery and return `FreshExpected`." Set to `i.startCycleGeneration + 1` inside `ClearConversationState()`. Consumed (reset to 0) by `prepareColdRestore()` whether or not the generation actually matched. In-memory only, not persisted. | New — **replaces** `RecoverySuppressed`. Scoping suppression to an exact generation number (rather than a bool that stays "true" until *some* future call happens to consume it) means a call site that never reaches `prepareColdRestore()` — or a legitimate capture that happens in between — cannot leave suppression armed indefinitely; a mismatched generation is simply not honored, self-correcting without requiring every mutator to remember to reset a flag. |
| **EverHadConversationHistory** | A durable `bool` field on `Instance`, persisted alongside `HistoryFilePath`. Set to `true` the first time a non-empty `ConversationUUID` is captured for this instance (via `SetHistoryInfo` or `recoverConversationUUIDByPath`/the live-PID path in `tryExtractConversationUUID`). Reset to `false` only by `ClearConversationState()`. | Unchanged from the prior plan, except its guarding lock is now accurate (see architecture Blocker fix in Epic 1.2). |
| **recoverConversationUUIDByPath** | New unexported `(*Instance) recoverConversationUUIDByPath() bool` method: scans `~/.claude/projects/<encoded-path>/` via `detector.DetectByPath` for the newest JSONL and, if found, records it under the same locked write pattern as `ClearConversationState`/`SetHistoryInfo`. Never inspects a live pane's open files — safe to call regardless of whether `i.pm().IsAlive()` is true or false. Returns whether a UUID is now known (either because this call found one, or because one was already known). | New. Extracted from `tryExtractConversationUUID()`'s existing path-fallback body specifically so `prepareColdRestore()` — now called from `Restart()` where the pane may still be alive at call time — has no dependency on pane liveness (resolves architecture Concern 2 against the prior plan). `tryExtractConversationUUID()` is refactored to call this for its own fallback branch instead of duplicating the scan. |
| **prepareColdRestore** | `(*Instance) prepareColdRestore() coldRestoreOutcome` method, called from **all four** launch-command-building call sites — `startLocked`, `start`, `Resume`, `Restart` — before each builds its launch command. Encapsulates: advance `startCycleGeneration`, honor a matching `recoverySuppressedGeneration`, else attempt `recoverConversationUUIDByPath`, and compute this cycle's `ReviveOutcome`. | Expanded from the prior plan (which called it from only two sites) per adversarial Blocker 1. No longer depends on the caller having already confirmed `!i.pm().IsAlive()` — see `recoverConversationUUIDByPath` above. |
| **coldRestoreOutcome** | The return struct from `prepareColdRestore`: `{ Outcome ReviveOutcome }`. Callers call `outcome.Outcome.ShouldResume()` to decide whether to pass a UUID into `buildLaunchCommand`/`initTmuxSession`. | Changed from the prior plan: the redundant `Resume bool` field is dropped (architecture Concern 1) in favor of a `ShouldResume()` method on `ReviveOutcome` itself — one source of truth, no illegal-state combination possible. |
| **ReviveOutcome** | A 4-value enum (Go string-const type + mirrored proto enum): `RESUME_LIVE`, `RESUME_RECOVERED`, `FRESH_EXPECTED`, `FRESH_LOST_HISTORY`. Now also carries a `ShouldResume() bool` method (`true` for `RESUME_LIVE`/`RESUME_RECOVERED`). | Unchanged in shape from the prior plan; gains the `ShouldResume()` method. Persisted on `Instance.LastReviveOutcome`, exposed via `proto/session/v1/types.proto`. |
| **ReasonColdRestoreLostHistory** | The `string` reason constant (`"cold-restore-fresh-lost-history"`) passed to `i.fireLifecycleEvent(EventStarted, reason)` when `ReviveOutcome == FRESH_LOST_HISTORY`. | Unchanged from the prior plan. **Only fired from `startLocked`/`start`** — see Epic 1.6/1.7 and the new Unresolved Question about why `Resume()`/`Restart()` deliberately do not fire `EventStarted`. |
| **onColdRestoreLostHistory** | `SessionService` method, same shape as `onRateLimitRecovery`, that publishes an `events.NewNotificationEvent(...)` (WARNING/MEDIUM) when it observes `EventStarted` with `reason == ReasonColdRestoreLostHistory`, gated on `!inst.Hidden`. | Changed from the prior plan: notification ID now includes a per-occurrence timestamp component (adversarial Concern 1), and the linked-item lookup helper it calls is renamed from `rateLimitLinkedItemID` to `linkedItemIDForInstance` (architecture Concern 3). |
| **RevivedContextBadge** | Frontend component: a small aria-labeled badge on `SessionCard`/`SessionRow`, shown when `session.reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY`. | Unchanged from the prior plan. Reads `LastReviveOutcome` off the `Session` proto directly — this is why the badge (unlike the toast) surfaces correctly even for `Resume()`/`Restart()` cycles that don't fire `EventStarted`. |

11 glossary terms (was 9 — `startCycleGeneration`/`recoverySuppressedGeneration` together replace the single `RecoverySuppressed` term, a net +1, and `recoverConversationUUIDByPath` is new, a further +1).

---

## Step 3: Technology / pattern validation and ADR judgment call

Unchanged from the prior plan's reasoning: every pattern reused here is
already present in this codebase (actor-serialized `Instance` methods,
`LifecycleEvent`/reason strings, `pkg/events.NewNotificationEvent`, proto
enum + TS `assertNever` exhaustiveness, JSON-tagged persisted struct
fields). The repair work in this rewrite (generation-counter suppression,
the extracted path-only recovery helper, four call sites instead of two)
does not introduce a new dependency, subsystem, or cross-cutting
architectural direction — it applies the same chosen pattern (a) more
completely and closes two correctness gaps in how it was wired. **This plan
explicitly decides not to write a formal ADR**, for the same reason the
prior plan gave: the one piece of new vocabulary that rises above
implementation detail (the `ReviveOutcome` enum) already follows the
`DetectedStatus`/`AttentionReason` structural precedent.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Recovery call ordering | Move the recovery attempt before the launch-command build, at **all four** call sites that build one from a possibly-stale in-memory UUID | Step 0.5 (a), expanded | (b) Make `buildLaunchCommand`/`initTmuxSession` itself resume-aware | Those functions run unconditionally in contexts (hot-restore, first-time-setup, `Resume()`'s live-pane branch) where Goal 2/AC2 requires zero added disk I/O; folding the scan in there can't distinguish those cases without re-deriving the same guard `prepareColdRestore` already centralizes |
| Recovery call ordering | (same) | | (c) Always speculatively launch with `--resume`, reconcile after the fact | Reintroduces exactly the stale-resume loop `recoverFromStaleResume` already exists to break |
| Scope of call sites | Four: `startLocked`, `start`, `Resume`, `Restart` | Adversarial review Blocker 1 | Two: `startLocked`/`start` only (prior plan's scope) | `Resume()` (the `Resume` RPC's own dead-pane branch, `instance.go:1443-1451`) and `Restart()` (called from `SessionDriver.handleDriverFailure`'s non-`Stopped` branch and from `SwitchProgram`) reproduce the identical bug and are both reachable in production; `research/features.md` in the prior project explicitly named `SessionDriver` as the dominant field-observed trigger, so leaving `Restart()` unfixed would ship a fix that misses its own most-likely trigger |
| Shared logic between all four call sites | One unexported `prepareColdRestore` helper called from all four | AC4; adversarial Blocker 1's own recommendation | Four independently patched copies of the decision logic | AC4 forbids duplicated divergent logic; the prior plan's own research already found comment drift between just the *two* existing copies — four independent copies would only compound that risk |
| Intentional-clear vs. accidental-loss | `recoverySuppressedGeneration uint64`, matched against `startCycleGeneration` at consumption time | Adversarial review Blocker 2's remediation option (b) | (i) Bare one-shot `recoverySuppressed bool` (prior plan); (ii) clear the bool unconditionally at the top of every subsequent start/restart attempt (Blocker 2's option (a)) | (i) is provably unsafe per Blocker 2's trace: `SwitchProgram` → `ClearConversationState()` → `Restart(true)` (which, in the prior plan, never consumed the flag) leaves it armed to wrongly suppress a *later*, unrelated legitimate recovery. (ii) would work now that all four call sites reach `prepareColdRestore`, but only by convention — a future fifth call site that forgets to reset it reintroduces the bug. The generation-number comparison is self-correcting: a stale `recoverySuppressedGeneration` from a past cycle simply fails the equality check on any later cycle, with no dependency on every caller remembering to reset anything |
| Path-only recovery, decoupled from pane liveness | Extract `recoverConversationUUIDByPath()` from `tryExtractConversationUUID()`'s existing fallback body; `prepareColdRestore()` calls only this, never the live-PID path | Architecture review Concern 2's remediation option ("extract a path-only helper") | (i) `prepareColdRestore()` calls `tryExtractConversationUUID()` wholesale, relying on a comment-only "caller already confirmed `!IsAlive()`" precondition (prior plan); (ii) a defensive `if i.pm().IsAlive() { log.Error(...) }` guard at the top of `prepareColdRestore()` | (i) is the flagged defect: an unenforced precondition with no compiler/runtime signal. (ii) is actively wrong once `Restart()` is added as a caller (Blocker 1's fix) — `Restart()` calls `prepareColdRestore()` **before** `KillSession()`, so the pane may legitimately still be alive at that point (e.g. `SwitchProgram`'s `Restart(true)` on an active session); logging that as an error would be a false positive on a normal path. Extracting a path-only helper sidesteps the precondition entirely: it is correct regardless of pane liveness because it never inspects the pane |
| New field write locking | `recoverConversationUUIDByPath()`'s direct-mutation write (including the new `everHadConversationHistory` write) takes `claudeSessionMu.Lock()` + nested `i.mu.Lock()`, matching `ClearConversationState`/`SetHistoryInfo`'s exact pattern; `tryExtractConversationUUID()`'s live-PID-path write is updated to match | Architecture review Blocker's remediation option (a) | Option (b): leave the write unlocked and correct Task 1.1.1b's doc comment to describe the *actual* (unsynchronized) invariant instead | (a) actually closes the race the blocker's evidence names (`HistoryLinker.correlateSession`/`scanAllSessions`'s 5s-tick poller calling `HasClaudeSession()` — itself lock-protected — concurrently with this write) for the fields this fix depends on, rather than merely documenting that the race exists. Since Task 1.2.1c (the task the blocker flagged) is already the one touching this exact block, and no caller of `tryExtractConversationUUID` already holds `claudeSessionMu` (confirmed: all 5 call sites read via `GetClaudeConversationUUID()`/`HasClaudeSession()` first, which take-and-release the lock, before calling), adding the lock inside the function introduces no self-deadlock risk |
| `coldRestoreOutcome` shape | `{ Outcome ReviveOutcome }` only, plus `func (o ReviveOutcome) ShouldResume() bool` | Architecture review Concern 1 | `{ Resume bool; Outcome ReviveOutcome }` (prior plan) | `Resume` was a pure function of `Outcome` (true iff `Outcome ∈ {ResumeLive, ResumeRecovered}`) — a derivable duplicate that could disagree with `Outcome` after a future edit, reintroducing exactly the illegal-state-combination problem the "Durable field shape" row below already argues against at the `ReviveOutcome`-vs-two-booleans level |
| "Had history before" signal | Durable `EverHadConversationHistory` bool set at capture/clear time | pitfalls.md finding 3/4 (prior plan, unchanged) | Infer "had history" from `HistoryFilePath == ""` at decision time | Unchanged reasoning from the prior plan |
| Decision-time UUID recovery mechanism | `recoverConversationUUIDByPath()`, a small concrete method (not an interface) | `.claude/rules/interface-pollution-checklist.md` | New `Resolver`/`Recovery` interface type | One concrete implementation, no second one imminent |
| User-visible signal transport | Reuse `LifecycleEvent`/`fireLifecycleEvent` reason string + `pkg/events.NewNotificationEvent`, mirroring `onRateLimitRecovery`'s shape — **fired only from `startLocked`/`start`**, not `Resume()`/`Restart()` | architecture.md; ux.md (prior plan) — scope narrowed here, see Unresolved Questions | Also fire `EventStarted` from `Resume()`/`Restart()` so the toast covers all four call sites | `EventStarted` has existing subscribers beyond the notification listener this fix adds — `BacklogLifecycleListener.onSessionStarted` and `review_queue_poller.go`'s reconciliation logic — that assume "a real start just happened." `Resume()`/`Restart()` firing it would be a new, untested behavioral change to those listeners, well beyond this bug fix's blast radius. The durable field (`LastReviveOutcome`, surfaced via `RevivedContextBadge`) is set from all four call sites regardless and already meets AC3's "at minimum" bar without this risk |
| Notification ID entropy | `cold-restore-lost-history-<instanceUUID>-<unixTimestamp>` | Adversarial review Concern 1 | `cold-restore-lost-history-<instanceUUID>` (prior plan) | A pure function of the stable instance UUID collides with itself on a second occurrence — `NotificationHistoryStore.Append()`'s exact-ID-match check runs before the read/unread dedup logic and silently no-ops the second, real occurrence regardless of read state. A timestamp component routes every occurrence through the intended `findUnreadDuplicate` collapse-or-create-new path instead |
| Linked-item lookup helper naming | Rename `rateLimitLinkedItemID` → `linkedItemIDForInstance`, update its 2 existing call sites (`onRateLimitDetected`, `onRateLimitRecovery`) plus the new `onColdRestoreLostHistory` call | Architecture review Concern 3 | Reuse `rateLimitLinkedItemID` under its existing name (prior plan) | The function's behavior (best-effort, bounded `ItemSession` lookup) has nothing to do with rate limiting; keeping the name would mislead a future reader grepping for rate-limit code |
| Corrupt/zero-byte JSONL candidates | Filter zero-byte files out of `DetectByPath`'s candidate list | Adversarial review Concern 2 | No filter; accept as a documented deferral (prior plan's fallback option) | Cheap (one `info.Size() == 0` check already available from the `os.DirEntry.Info()` call already being made) and benefits every existing caller of `DetectByPath`, not just the new one — chosen over documenting a deferral because the fix costs less than the deferral note would |
| Same-directory `DetectByPath` ambiguity | Accept as an inherited, documented limitation | pitfalls.md §b; architecture.md Risk 1 (prior plan, unchanged) | Add `HistoryFilePath`-match cross-check or content-level `cwd` verification | Unchanged reasoning from the prior plan — out of scope for this bugfix's ACs |
| `DetectByPath` timeout on the (now more frequently exercised) critical path | Not added in this pass | Adversarial review Concern 3 — explicitly **not** folded into this rewrite | Add a context-aware timeout to `DetectByPath` | `DetectByPath` gained two new call sites in this rewrite (`recoverConversationUUIDByPath`, called from `Restart()` before `KillSession()`) but the underlying I/O cost (a single small directory listing) is unchanged from what the prior plan already accepted as low-risk; flagged again explicitly here rather than silently dropped — candidate follow-up, not blocking |

---

## Migration Plan

Additive-only, updated for the field-shape changes above. Four new
persisted/wire/in-memory fields, each with a safe zero-value default so
existing sessions round-trip correctly with no backfill:

- `Instance.startCycleGeneration uint64` and `Instance.recoverySuppressedGeneration
  uint64` (both in-memory only, not persisted — always start `0`, which is
  correct: `0` means "no suppression armed," matching every existing
  session's implicit state today). **Replaces** the prior plan's
  `Instance.recoverySuppressed bool`.
- `Instance.everHadConversationHistory bool` and `Instance.LastReviveOutcome
  ReviveOutcome` — unchanged from the prior plan: both added to the
  JSON-persisted struct in `session/storage.go` with `,omitempty`-style
  tags, same pattern as `HistoryFilePath string
  \`json:"history_file_path,omitempty"\`` at `session/storage.go:111`. An
  existing session loaded from disk with no `ever_had_conversation_history`
  key deserializes to `false` — conservative, matches the prior plan's
  migration reasoning exactly.
- `revive_outcome` proto field (72) on `Session` — unchanged from the prior
  plan; proto3 fields are optional-by-default, confirmed still free
  (`awk '/^message Session {/,/^}/' proto/session/v1/types.proto | grep -oE
  "= [0-9]+;" | sort -n | tail -1` → `71`, so `72` is still available).

---

## Observability Plan

- **Logs**: `prepareColdRestore` logs one line per invocation (`log.Info`
  when `Outcome == RESUME_RECOVERED`, `log.Warn` when `FRESH_LOST_HISTORY`,
  `log.Debug` otherwise) with `session`, `path`, `outcome`, and — new —
  `caller` fields (one of `startLocked`/`start`/`Resume`/`Restart`, passed
  as a small string argument at each of the four call sites) so a future
  reader can tell from the log alone which of the four launch paths
  triggered a given recovery/loss, without that context the prior plan's
  single-caller design didn't need to distinguish.
- **Metrics**: none added — unchanged from the prior plan's reasoning
  (out of scope per requirements.md's Non-goals).
- **Alerts**: none added — unchanged from the prior plan's reasoning
  (single-user self-hosted deployment, no existing alerting infrastructure).

## Risk Control

- **Feature flag**: none added — unchanged from the prior plan's reasoning.
  `recoverySuppressedGeneration`'s generation-matched design is itself the
  safety valve (see Pattern Decisions), and is strictly more robust than the
  prior plan's bare bool, so no additional kill-switch is warranted for a
  fix this scoped.
- **Rollback procedure**: revert the commit — unchanged from the prior
  plan's reasoning. All new fields are additive with safe zero-value
  defaults.
- **Staged rollout**: not applicable — unchanged from the prior plan's
  reasoning.

## Unresolved Questions

- [x] ~~Should `EverHadConversationHistory` resetting to `false` on
      `ClearConversationState()` still allow a later, unrelated cold-restore
      to correctly re-earn `FRESH_LOST_HISTORY`?~~ — **Resolved by design**,
      unchanged from the prior plan: `EverHadConversationHistory` is re-set
      to `true` the moment any of the three capture paths
      (`SetHistoryInfo`/`recoverConversationUUIDByPath`/`tryExtractConversationUUID`'s
      live-PID path) next captures a UUID, independent of the generation
      counter.
- [x] ~~Can `RecoverySuppressed` sit armed indefinitely and wrongly suppress
      a later, unrelated legitimate recovery?~~ — **Resolved** by replacing
      the bare bool with `recoverySuppressedGeneration`/`startCycleGeneration`
      (adversarial Blocker 2; see Pattern Decisions). Confirmed by
      Task 4.1.4's test, which replays Blocker 2's exact trace.
- [ ] **New**: `Resume()`/`Restart()` set `i.LastReviveOutcome` (so the badge
      is accurate) but deliberately do **not** fire `EventStarted` (so the
      toast notification and `BacklogLifecycleListener`/reconciliation
      side effects are not triggered from these paths) — see the "User-visible
      signal transport" Pattern Decisions row. This means a `FRESH_LOST_HISTORY`
      reached via `Resume()`/`Restart()` gets the durable badge but not the
      toast, while the same outcome reached via `startLocked`/`start` gets
      both. Is this asymmetry acceptable long-term, or should a narrower,
      `EventStarted`-independent notification hook be added for
      `Resume()`/`Restart()` in a fast-follow? — blocks nothing in this plan
      (AC3's "at minimum" bar is met by the badge alone); flagged for
      explicit reviewer sign-off since it's a new asymmetry this rewrite
      introduces, not one the original requirements anticipated — owner: PR
      reviewer.
- [ ] Should the ux.md layer-3 in-session banner be built now or deferred as
      a fast-follow? — unchanged from the prior plan; still deferred, still
      blocks nothing.
- [ ] Should a stronger `DetectByPath` disambiguation be scoped as its own
      follow-up item? — unchanged from the prior plan; still deferred
      (Task 4.1.1f documents the limitation with an explicit test).
- [ ] Is a `DisableColdRestoreRecovery`-style config kill-switch worth
      building? — unchanged from the prior plan; still `skip unless a
      reviewer asks for it`.
- [ ] **New (fast-follow, not required for this PR)**: architecture review
      Concern 4 recommended a `ConversationRecoveryState` value type on
      `Instance` (`MarkCaptured()`/`MarkCleared()`/`ConsumeSuppression()`
      methods) to enforce the "these fields change together" invariant in
      one place instead of at every mutator call site. That recommendation
      is carried forward unchanged and, if anything, applies with slightly
      more force now that `everHadConversationHistory`,
      `recoverySuppressedGeneration`, and `startCycleGeneration` are mutated
      from `ClearConversationState`, `SetHistoryInfo`,
      `recoverConversationUUIDByPath`, `tryExtractConversationUUID`, and
      `prepareColdRestore` — five mutators, not four. Not built here; this
      plan's own tests (Epic 4.1) directly cover the interactions that would
      break if a mutator forgot a field — owner: backlog triage, candidate
      new item.

---

## Dependency Visualization

```
Phase 1 — Recovery ordering (backend, required for AC1/AC2/AC4/AC6)
  Epic 1.1 (types/fields) ──┬─→ Epic 1.2 (wire into ClearConversationState/
                             │            SetHistoryInfo/recoverConversationUUIDByPath/
                             │            tryExtractConversationUUID + zero-byte filter)
                             │                       │
                             └─→ Epic 1.3 (prepareColdRestore + ShouldResume) ←┘
                                          │
                    ┌─────────────┬───────┴───────┬─────────────┐
                    ▼             ▼               ▼             ▼
         Epic 1.4          Epic 1.5         Epic 1.6        Epic 1.7
     (startLocked reorder) (start reorder)  (Resume         (Restart
                                              integration)    integration)
                    │             │               │             │
                    └─────────────┴───────┬───────┴─────────────┘
                                          ▼
Phase 2 — Durable signal (backend, required for AC3)
  Epic 2.1 (proto enum + field) ──→ Epic 2.2 (LifecycleEvent reason,
                                              startLocked/start only) ──→ Epic 2.3
                                                                          (notification
                                                                           listener,
                                                                           notifID entropy +
                                                                           renamed helper)
                                          │
                                          ▼
Phase 3 — Frontend surface (ux.md, unchanged from prior plan)
  Epic 3.1 (RevivedContextBadge + SessionCard/SessionRow wiring, toast verify)
                                          │
                                          ▼
Phase 4 — Tests (required for AC5, covers all of Phase 1-3)
  Epic 4.1 (backend unit + integration, expanded for 4 call sites +
            generation-counter regression + zero-byte filter) ──→ Epic 4.2
            (frontend component test, unchanged)
```

Epics 1.4-1.7 are siblings (all four depend on 1.1-1.3, none depend on each
other) and can be implemented/reviewed in any order or in parallel. Phase 1
must land before Phase 2 (the `ReviveOutcome` value Phase 2 persists and
publishes is computed by Phase 1's `prepareColdRestore`). Phase 3 depends on
Phase 2's proto field existing. Phase 4 tasks are written per-epic alongside
their Phase 1-3 counterparts, grouped here for visibility into total test
coverage.

---

## Phase 1: Recovery-before-decision ordering

### Epic 1.1: New domain types and fields
**Goal**: Introduce `ReviveOutcome` (+ `ShouldResume()`),
`startCycleGeneration`, `recoverySuppressedGeneration`, and
`EverHadConversationHistory` as first-class `Instance` state before any
control-flow change touches them.

#### Story 1.1.1: Add ReviveOutcome type, ShouldResume method, and Instance fields
**As a** developer implementing the cold-restore fix, **I want** the new
domain types to exist before I wire them into control flow, **so that** the
control-flow changes in Epics 1.3-1.7 are small, reviewable diffs on top of
already-compiling types.
**Acceptance Criteria**:
- `ReviveOutcome` is a `string`-backed type with 4 named constants and a
  `ShouldResume() bool` method.
  - *Given* the new `session/instance_claude.go` const block, *When* code
    elsewhere references `session.ReviveOutcomeFreshLostHistory`, *Then* it
    resolves to the string value `"fresh_lost_history"`, and
    `session.ReviveOutcomeResumeLive.ShouldResume()` and
    `session.ReviveOutcomeResumeRecovered.ShouldResume()` both return `true`
    while the other two constants return `false`.
- `Instance` has four new fields: `startCycleGeneration uint64`,
  `recoverySuppressedGeneration uint64`, `everHadConversationHistory bool`,
  `LastReviveOutcome ReviveOutcome`.
**Files**: `session/instance_claude.go`, `session/instance.go`

##### Task 1.1.1a: Add `ReviveOutcome` type + constants + `ShouldResume()` (~4 min)
- In `session/instance_claude.go`, near the existing `staleResumePattern`
  const (line 19), add:
  ```go
  type ReviveOutcome string

  const (
  	ReviveOutcomeUnspecified      ReviveOutcome = ""
  	ReviveOutcomeResumeLive       ReviveOutcome = "resume_live"
  	ReviveOutcomeResumeRecovered  ReviveOutcome = "resume_recovered"
  	ReviveOutcomeFreshExpected    ReviveOutcome = "fresh_expected"
  	ReviveOutcomeFreshLostHistory ReviveOutcome = "fresh_lost_history"
  )

  // ShouldResume reports whether this outcome means the caller should have a
  // conversation UUID available to pass to buildLaunchCommand/initTmuxSession.
  func (o ReviveOutcome) ShouldResume() bool {
  	return o == ReviveOutcomeResumeLive || o == ReviveOutcomeResumeRecovered
  }
  ```
- Files: `session/instance_claude.go`

##### Task 1.1.1b: Add new fields to the `Instance` struct (~3 min)
- In `session/instance.go`, next to the existing `HistoryFilePath string`
  field (line 230) and under the `claudeSessionMu` comment (line 301), add:
  `everHadConversationHistory bool`, `LastReviveOutcome ReviveOutcome`
  (exported — read by `server/adapters/instance_adapter.go` in Phase 2),
  `startCycleGeneration uint64`, `recoverySuppressedGeneration uint64`.
  Doc-comment: "`everHadConversationHistory`/`LastReviveOutcome` are guarded
  by `claudeSessionMu` (same lock order as `HistoryFilePath`) — every writer
  in this file takes `claudeSessionMu.Lock()` (nested `i.mu.Lock()` inside)
  around the write; `startCycleGeneration`/`recoverySuppressedGeneration`
  are guarded the same way but are only ever read/written from inside
  `prepareColdRestore()` and `ClearConversationState()`." Unlike the prior
  plan's version of this doc comment, this one is accurate as written
  because Task 1.2.1c (below) moves the corresponding write under lock —
  see the architecture-review.md blocker this resolves.
- Files: `session/instance.go`

##### Task 1.1.1c: Persist the two durable fields in storage (~3 min)
- Unchanged from the prior plan. In `session/storage.go`, next to the
  existing `HistoryFilePath string \`json:"history_file_path,omitempty"\``
  field (line 111), add `EverHadConversationHistory bool
  \`json:"ever_had_conversation_history,omitempty"\`` and
  `LastReviveOutcome string \`json:"last_revive_outcome,omitempty"\``. Wire
  the load/save mapping functions in this file to also copy these two
  fields. Do **not** persist `startCycleGeneration`/
  `recoverySuppressedGeneration` — both are correctly transient (a process
  restart naturally invalidates any pending suppression; the generation
  counter has no meaning across a restart).
- Files: `session/storage.go`

##### Task 1.1.1d: Expose `LastReviveOutcome` on `InstanceSnapshot` (~3 min)
- Unchanged from the prior plan. In `session/instance_snapshot.go`, add
  `LastReviveOutcome ReviveOutcome` to the `InstanceSnapshot` struct (near
  `HistoryFilePath string` at line 117) and copy it in `buildSnapshot` (near
  line 198's `HistoryFilePath: i.HistoryFilePath,`).
- Files: `session/instance_snapshot.go`

---

### Epic 1.2: Wire the new fields into existing mutators; extract the path-only recovery helper
**Goal**: `ClearConversationState`, `SetHistoryInfo`, and a new
`recoverConversationUUIDByPath` (extracted from `tryExtractConversationUUID`)
become the single source of truth for `recoverySuppressedGeneration`/
`everHadConversationHistory`, under correctly-locked writes, and in a shape
`prepareColdRestore()` can call safely regardless of pane liveness.

#### Story 1.2.1: ClearConversationState arms suppression for the next generation and resets EverHadConversationHistory
**As a** session that is being intentionally reset (stale-resume rejection,
program-family switch, or user "start over"), **I want** the very next
resume/fresh decision — whichever of the four call sites handles it — to
skip path-based recovery and treat me as a clean slate, **so that** I don't
loop back onto the exact UUID/JSONL I was just deliberately disconnected
from (Goal 4 / AC6), without that suppression silently outliving the one
cycle it was meant for (adversarial Blocker 2).
**Acceptance Criteria**:
- Every call to `ClearConversationState()` sets `recoverySuppressedGeneration
  = i.startCycleGeneration + 1` and `everHadConversationHistory = false`
  under the same lock order (`claudeSessionMu` outer, `i.mu` inner) it
  already uses for `ConversationUUID`/`HistoryFilePath`.
  - *Given* an `Instance` with `startCycleGeneration == 5` and
    `everHadConversationHistory == true`, *When*
    `recoverFromStaleResume()` calls `i.ClearConversationState()`, *Then*
    immediately after that call `i.recoverySuppressedGeneration == 6` and
    `i.everHadConversationHistory == false`.
  - *Given* the same `Instance` immediately after that clear, *When*
    `prepareColdRestore()` is called exactly once more (advancing
    `startCycleGeneration` to `6`), *Then* it observes a matching
    generation and returns `FreshExpected` without attempting recovery.
  - *Given* the same `Instance`, but a **second, unrelated**
    `prepareColdRestore()` call happens later (advancing
    `startCycleGeneration` to `7`, with no intervening `ClearConversationState()`
    call), *Then* the stale `recoverySuppressedGeneration == 6` does **not**
    match `startCycleGeneration == 7`, and recovery proceeds normally —
    this is the exact scenario adversarial Blocker 2 traced as broken under
    the prior plan's bare-bool design.
**Files**: `session/instance_claude.go`

##### Task 1.2.1a: Set the generation and EverHadConversationHistory inside ClearConversationState (~4 min)
- In `session/instance_claude.go`'s `ClearConversationState()` (line 278),
  inside the existing `i.mu.Lock()`/`i.mu.Unlock()` section (alongside the
  `i.claudeSession.ConversationUUID = ""` / `i.HistoryFilePath = ""`
  writes), add `i.recoverySuppressedGeneration = i.startCycleGeneration + 1`
  and `i.everHadConversationHistory = false`. Update the function's doc
  comment to describe the generation-matched suppression scheme instead of
  a bare flag.
- Files: `session/instance_claude.go`

##### Task 1.2.1b: Set EverHadConversationHistory=true in SetHistoryInfo (~3 min)
- Unchanged from the prior plan. In `session/instance_claude.go`'s
  `SetHistoryInfo()` (line 464), inside the existing `i.mu.Lock()` section,
  add: when the new `conversationUUID` argument is non-empty, set
  `i.everHadConversationHistory = true`.
- Files: `session/instance_claude.go`

##### Task 1.2.1c: Extract `recoverConversationUUIDByPath()`; move its write under lock; refactor `tryExtractConversationUUID` to use it (~10 min)
- **This is the task the architecture review's Blocker targeted** (the
  prior plan's version of this task added an unlocked write with a doc
  comment claiming it *was* locked). The fix: extract the path-fallback
  scan into its own method with a correctly-locked write, and make both
  `tryExtractConversationUUID`'s fallback branch and the new
  `prepareColdRestore` (Epic 1.3) call it, instead of routing new callers
  through `tryExtractConversationUUID`'s dual-purpose (live-PID +
  path-fallback) body — this also resolves architecture Concern 2 (no more
  "assumes pane is dead" precondition on the new callers).
- In `session/instance_claude.go`, add:
  ```go
  // recoverConversationUUIDByPath scans ~/.claude/projects/<encoded-path>/
  // for the newest JSONL matching this instance's effective root dir and, if
  // found, records it as this instance's conversation UUID/history file
  // path. Unlike tryExtractConversationUUID, it never inspects a live pane's
  // open files — callers that want the live-PID fast path should call
  // tryExtractConversationUUID directly. Safe to call regardless of whether
  // the tmux pane is currently alive (cold-start-uuid-loss: prepareColdRestore
  // calls this from Restart(), which may run before the pane is killed).
  // Returns true if a conversation UUID is now known (either just recovered,
  // or already known on entry).
  func (i *Instance) recoverConversationUUIDByPath() bool {
  	if i.HasClaudeSession() {
  		return true
  	}
  	detector := i.historyDetector
  	if detector == nil {
  		detector = NewHistoryFileDetectorWithRealInspector()
  	}
  	effectivePath := i.GetEffectiveRootDir()
  	if effectivePath == "" {
  		return false
  	}
  	info, err := detector.DetectByPath(effectivePath)
  	if err != nil {
  		log.Warn("recoverConversationUUIDByPath: path-based detect error", "session", i.Title, "err", err)
  	}
  	if info == nil {
  		log.Debug("recoverConversationUUIDByPath: no jsonl file found", "session", i.Title)
  		return false
  	}

  	i.claudeSessionMu.Lock()
  	i.mu.Lock()
  	if i.claudeSession == nil {
  		i.claudeSession = &ClaudeSessionData{}
  	}
  	i.claudeSession.ConversationUUID = info.ConversationUUID
  	i.HistoryFilePath = info.HistoryFilePath
  	i.everHadConversationHistory = true
  	snap := buildSnapshot(i)
  	i.mu.Unlock()
  	i.snapshot.Store(snap)
  	i.claudeSessionMu.Unlock()

  	log.ForSession(i.Title).Info("uuid assigned via recoverConversationUUIDByPath", "uuid", info.ConversationUUID, "path", info.HistoryFilePath)
  	return true
  }
  ```
- Refactor `tryExtractConversationUUID()` (line 308): keep its early-return
  fast-path check and its live-PID `detector.Detect(pid)` branch exactly as
  they are today, but (1) move that live-PID branch's own direct-mutation
  write (today's lines 356-362) under the identical `claudeSessionMu.Lock()`
  + nested `i.mu.Lock()` pattern shown above (including the new
  `i.everHadConversationHistory = true` write), and (2) replace the
  path-fallback block (today's lines 336-349) with a single call:
  `if info == nil { i.recoverConversationUUIDByPath(); return }` — i.e. once
  the live-PID path has been tried and found nothing, delegate the rest of
  the function to the new helper instead of duplicating the scan. Update
  the function's doc comment: remove the stale "assumes stateMutex is
  already held by the caller" claim (verified false — none of the 5
  existing call sites, `session/agy_adapter.go:55`,
  `session/claude_adapter.go:60`, `session/instance.go:921,1127`,
  `session/instance_workspace.go:80`, hold `claudeSessionMu` when calling
  this), replacing it with: "Thread-safe: takes `claudeSessionMu` internally
  around its writes."
- Files: `session/instance_claude.go`

##### Task 1.2.1d: Filter zero-byte JSONL candidates in DetectByPath (~3 min)
- Folds in adversarial review Concern 2 (corrupt/0-byte JSONL sanity
  check), cheaply, at the source shared by every caller of `DetectByPath`
  rather than adding a per-caller check. In
  `session/history_detector.go`'s `DetectByPath()` candidate-collection
  loop (~line 174, where `entry.Info()` is already called to read
  `modTime`), add: `if info.Size() == 0 { continue }` before appending the
  candidate. This is a minimal sanity check, not full JSONL validation
  (parsing every candidate's first line would be a larger, unjustified
  change for this bug fix) — matches the review's "at minimum ... a minimal
  sanity check (non-zero size...)" recommendation exactly.
- Files: `session/history_detector.go`

---

### Epic 1.3: The `prepareColdRestore` shared helper
**Goal**: One function, callable from all four launch-command-building call
sites, that performs the entire recovery-then-decide computation this bug
is about — correct regardless of which caller invokes it or whether the
tmux pane happens to be alive at call time.

#### Story 1.3.1: Implement prepareColdRestore with generation-scoped suppression
**As a** developer wiring this helper into `startLocked`/`start`/`Resume`/
`Restart`, **I want** one function that returns "what's the durable outcome
signal for this cycle," **so that** all four call sites share identical
logic (AC4) and none has to re-derive the
`HasClaudeSession()`/suppression/`DetectByPath`/`EverHadConversationHistory`
interaction itself.
**Acceptance Criteria**:
- `prepareColdRestore()` never calls `DetectByPath` when `HasClaudeSession()`
  is already true.
  - *Given* an `Instance` with `i.claudeSession.ConversationUUID ==
    "550e8400-e29b-41d4-a716-446655440000"`, *When* `prepareColdRestore()`
    runs, *Then* it returns `coldRestoreOutcome{Outcome:
    ReviveOutcomeResumeLive}` without touching the filesystem, and
    `startCycleGeneration` has still been incremented (bookkeeping advances
    on every call, even the fast-path one).
- `prepareColdRestore()` honors and consumes a matching
  `recoverySuppressedGeneration` before attempting any recovery, and
  ignores (but still consumes, to avoid leaving stale state around) a
  non-matching one.
  - *Given* an `Instance` with `i.claudeSession == nil`,
    `startCycleGeneration == 5`, and `recoverySuppressedGeneration == 6`
    (just armed by `ClearConversationState()`), *When*
    `prepareColdRestore()` runs (advancing `startCycleGeneration` to `6`),
    *Then* it returns `coldRestoreOutcome{Outcome:
    ReviveOutcomeFreshExpected}`, performs no `DetectByPath` scan, and
    leaves `i.recoverySuppressedGeneration == 0` afterward.
  - *Given* the same starting state but `recoverySuppressedGeneration == 8`
    (armed for a generation further in the future than this call reaches),
    *When* `prepareColdRestore()` runs (advancing `startCycleGeneration` to
    `6`), *Then* the generations do not match, recovery is attempted
    normally, and `i.recoverySuppressedGeneration` is left at `8`
    unconsumed (it is only ever consumed on the exact generation it names —
    this is intentionally conservative: a suppression armed "too far in the
    future" relative to some skipped cycle should still apply when its
    named generation is actually reached, rather than being silently lost).
- `prepareColdRestore()` is callable regardless of `i.pm().IsAlive()`, with
  no defensive liveness check needed, because it never reaches
  `tryExtractConversationUUID()`'s live-PID branch.
  - *Given* an `Instance` with `i.pm().IsAlive() == true` and
    `i.claudeSession == nil`, *When* `prepareColdRestore()` runs, *Then* it
    still correctly falls through to `recoverConversationUUIDByPath()` (a
    path-only scan) rather than panicking, erroring, or silently
    misbehaving — this is the case `Restart()` (Epic 1.7) exercises when
    called before `KillSession()`.
**Files**: `session/instance_claude.go`

##### Task 1.3.1a: Add the coldRestoreOutcome struct and function signature (~3 min)
- In `session/instance_claude.go`, add:
  ```go
  // coldRestoreOutcome is prepareColdRestore's result: the durable
  // ReviveOutcome signal for this start/restart cycle. Callers use
  // Outcome.ShouldResume() to decide whether to pass a UUID into
  // buildLaunchCommand/initTmuxSession — there is deliberately no separate
  // bool field here (see cold-start-uuid-loss Pattern Decisions: a
  // duplicate of a derivable value is a bug waiting to diverge from it).
  type coldRestoreOutcome struct {
  	Outcome ReviveOutcome
  }

  // prepareColdRestore attempts on-disk UUID recovery — via
  // recoverConversationUUIDByPath, which is path-only and safe regardless
  // of tmux pane liveness — BEFORE the caller builds a launch command from
  // i.claudeSession.ConversationUUID (cold-start-uuid-loss Goal 1). Called
  // from all four places that build a launch command from a possibly-stale
  // in-memory UUID: startLocked, start, Resume, and Restart. Its result
  // must be consumed (via Outcome.ShouldResume()) before any of those
  // callers reads i.claudeSession.ConversationUUID for this cycle.
  func (i *Instance) prepareColdRestore() coldRestoreOutcome {
  	// body added in Task 1.3.1b
  }
  ```
- Files: `session/instance_claude.go`

##### Task 1.3.1b: Implement the decision body with generation-scoped suppression (~8 min)
- Body logic:
  ```go
  func (i *Instance) prepareColdRestore() coldRestoreOutcome {
  	i.claudeSessionMu.Lock()
  	i.startCycleGeneration++
  	gen := i.startCycleGeneration
  	alreadyKnown := i.claudeSession != nil && i.claudeSession.ConversationUUID != ""
  	suppressed := !alreadyKnown && i.recoverySuppressedGeneration == gen
  	if suppressed {
  		i.recoverySuppressedGeneration = 0
  	}
  	i.claudeSessionMu.Unlock()

  	if alreadyKnown {
  		return coldRestoreOutcome{Outcome: ReviveOutcomeResumeLive}
  	}
  	if suppressed {
  		return coldRestoreOutcome{Outcome: ReviveOutcomeFreshExpected}
  	}

  	if i.recoverConversationUUIDByPath() {
  		return coldRestoreOutcome{Outcome: ReviveOutcomeResumeRecovered}
  	}

  	i.claudeSessionMu.RLock()
  	everHad := i.everHadConversationHistory
  	i.claudeSessionMu.RUnlock()
  	if everHad {
  		return coldRestoreOutcome{Outcome: ReviveOutcomeFreshLostHistory}
  	}
  	return coldRestoreOutcome{Outcome: ReviveOutcomeFreshExpected}
  }
  ```
  Note the increment-and-check happens under one short `claudeSessionMu.Lock()`
  section (no disk I/O while holding it); `recoverConversationUUIDByPath()`
  is called with no lock held (it takes its own internally, as built in
  Task 1.2.1c) so a slow directory scan does not block concurrent
  `HasClaudeSession()`/`GetClaudeSession()` readers.
- Files: `session/instance_claude.go`

---

### Epic 1.4: Restructure `startLocked`'s call ordering
**Goal**: `initTmuxSession()` reads a `ConversationUUID` that recovery has
already had a chance to populate, for the `startLocked` (production) path.
Unchanged from the prior plan except for referencing `outcome.Outcome.ShouldResume()`
instead of the dropped `outcome.Resume` field.

#### Story 1.4.1: Move initTmuxSession() to run after prepareColdRestore in the cold-restore branch
**As a** session whose tmux pane died and whose in-memory UUID was never
captured, **I want** `startLocked` to attempt recovery before it builds the
`claude` launch command, **so that** a resumable JSONL that exists on disk
is actually used instead of silently discarded (AC1).
**Acceptance Criteria**:
- `i.initTmuxSession()` is no longer called unconditionally at the top of
  `startLocked`; it is called once per branch, after that branch's decision
  is fully known.
  - *Given* a `ColdRestore` for a session whose effective root dir is
    `/home/user/.stapler-squad/worktrees/proj-42` and a real JSONL exists at
    `~/.claude/projects/-home-user--stapler-squad-worktrees-proj-42/550e8400-e29b-41d4-a716-446655440000.jsonl`,
    with `i.claudeSession == nil`, *When* `startLocked` runs, *Then*
    `i.prepareColdRestore()` populates
    `i.claudeSession.ConversationUUID == "550e8400-e29b-41d4-a716-446655440000"`
    **before** `i.initTmuxSession()` is called, so `i.LaunchCommand`
    contains the substring `--resume 550e8400-e29b-41d4-a716-446655440000`.
**Files**: `session/instance.go`

##### Task 1.4.1a: Remove the unconditional initTmuxSession() call at the top (~2 min)
- Unchanged from the prior plan. Delete `i.initTmuxSession()` at
  `session/instance.go:858` (immediately after the `i.Title == ""` check,
  before `i.pm().ResetExitOnce()`). Leave `i.pm().ResetExitOnce()` /
  `i.pm().SetOnExitCallback(...)` (lines 860-861) in place.
- Files: `session/instance.go`

##### Task 1.4.1b: Call prepareColdRestore + initTmuxSession in the cold-restore branch (~5 min)
- In the `if !i.pm().IsAlive() {` block (line 879), immediately after
  entering it and before the existing `startPath :=
  i.resolveStartPath(...)` line, add:
  ```go
  outcome := i.prepareColdRestore()
  i.initTmuxSession()
  i.LastReviveOutcome = outcome.Outcome
  ```
  Then replace the existing `if i.HasClaudeSession() { ... } else { ... }`
  log branch (lines 881-885) with `if outcome.Outcome.ShouldResume() { ...
  } else { ... }` — same log messages, driven by the already-computed
  outcome instead of re-calling `HasClaudeSession()`.
- Files: `session/instance.go`

##### Task 1.4.1c: Call initTmuxSession() in the hot-restore and first-time-setup branches (~3 min)
- Unchanged from the prior plan. Add `i.initTmuxSession()` as the first
  line of the `else` branch (hot restore, line 922) and as the first line
  of the outer `else` branch (first-time setup, line 936). Verify via `go
  build ./session/...` that this compiles.
- Files: `session/instance.go`

##### Task 1.4.1d: Update the cold-restore comment block to describe the new order (~2 min)
- Unchanged from the prior plan. Update the existing comment above
  `i.tryExtractConversationUUID()` at line 914-920 to note that
  `prepareColdRestore()` (new, above) already attempted path-based recovery
  before the launch command was built — this second call is the
  live-process confirmation pass, not redundant with it.
- Files: `session/instance.go`

---

### Epic 1.5: Mirror the restructuring into `start()`
**Goal**: Identical fix in `Instance.start` (reachable via
`StartWithCleanup`), satisfying AC4's symmetry requirement. Unchanged from
the prior plan except for `ShouldResume()` usage.

#### Story 1.5.1: Apply the identical restructuring to Instance.start
**As a** maintainer relying on AC4 ("no duplicated divergent logic between
call sites"), **I want** `start()`'s cold-restore branch to call the exact
same `prepareColdRestore()` helper in the exact same position relative to
`initTmuxSession()`, **so that** the paths cannot drift apart the way they
already had before this fix.
**Acceptance Criteria**:
- `start()`'s cold-restore branch produces byte-identical `coldRestoreOutcome`
  behavior to `startLocked`'s for the same fixture state.
  - *Given* two otherwise-identical instances — one driven through
    `Instance.Start(false)` → `startLocked`, one through
    `Instance.StartWithCleanup(false)` → `start` — both with `i.claudeSession
    == nil` and the same resumable JSONL on disk, *When* each is invoked,
    *Then* both call `i.prepareColdRestore()` (verified via `grep -c "func
    (i \*Instance) prepareColdRestore" session/instance_claude.go` equals
    `1`), and both end with `i.LastReviveOutcome ==
    ReviveOutcomeResumeRecovered` and `--resume <uuid>` present in
    `i.LaunchCommand`.
**Files**: `session/instance.go`

##### Task 1.5.1a: Remove the unconditional initTmuxSession() call in start() (~2 min)
- Delete `i.initTmuxSession()` at `session/instance.go:1040`. Leave
  `i.pm().ResetExitOnce()` / `SetOnExitCallback(...)` (lines 1046-1047) in
  place.
- Files: `session/instance.go`

##### Task 1.5.1b: Call prepareColdRestore + initTmuxSession in start()'s cold-restore branch (~5 min)
- Mirror of Task 1.4.1b inside the `if !i.pm().IsAlive() {` block at line
  1069: `outcome := i.prepareColdRestore()`, then `i.initTmuxSession()`,
  then `i.LastReviveOutcome = outcome.Outcome`, then replace the
  `if i.HasClaudeSession() { ... } else { ... }` log branch (lines
  1072-1080) with `if outcome.Outcome.ShouldResume() { ... } else { ... }`.
- Files: `session/instance.go`

##### Task 1.5.1c: Call initTmuxSession() in start()'s hot-restore and first-time-setup branches (~3 min)
- Mirror of Task 1.4.1c: add `i.initTmuxSession()` at the top of the `else`
  (hot restore, line ~1128) and the outer first-time-setup branch.
- Files: `session/instance.go`

##### Task 1.5.1d: Update start()'s cold-restore comment block (~2 min)
- Mirror of Task 1.4.1d for the comment block at lines 1122-1126.
- Files: `session/instance.go`

---

### Epic 1.6: Integrate prepareColdRestore into `Resume()` (NEW — resolves adversarial Blocker 1)
**Goal**: `Resume()`'s dead-pane branch — reached via the `Resume` RPC
handler (`server/services/session_service.go:1872`), the "revive a paused
session" user action the project is named after — attempts on-disk UUID
recovery before rebuilding the launch command, exactly like `startLocked`/
`start`'s cold-restore branch. Confirmed by the adversarial review to
reproduce the identical bug with no recovery attempt today
(`session/instance.go:1443-1451`).

#### Story 1.6.1: Resume's dead-pane branch calls prepareColdRestore before building the launch command
**As a** user resuming a paused session whose tmux pane was killed to free
memory, **I want** the resume path to recover a UUID that was never
captured in memory (or was lost to an intervening restart race) from disk,
**so that** resuming a paused session does not silently start a brand-new
Claude with no memory of the paused conversation (same bug as AC1, at a
second call site).
**Acceptance Criteria**:
- `Instance.Resume()`'s `else` branch (tmux dead) calls
  `i.prepareColdRestore()` and uses `outcome.Outcome.ShouldResume()` to
  decide whether to pass a UUID into `buildLaunchCommand`, instead of
  reading `i.claudeSession.ConversationUUID` directly with no recovery
  attempt.
  - *Given* a `Paused` `Instance` whose tmux pane is dead, whose
    `i.claudeSession == nil`, and for which a resumable JSONL exists on
    disk for its effective root dir, *When* `i.Resume()` runs, *Then*
    `i.LaunchCommand` (captured via the existing `program := ...` local)
    contains `--resume <recovered-uuid>`, and `i.LastReviveOutcome ==
    ReviveOutcomeResumeRecovered`.
  - *Given* the same starting state but no JSONL exists on disk and
    `i.everHadConversationHistory == true` (a real prior conversation whose
    file is now missing/unreadable), *When* `i.Resume()` runs, *Then* the
    session starts fresh (no `--resume` in `i.LaunchCommand`) and
    `i.LastReviveOutcome == ReviveOutcomeFreshLostHistory` — visible via
    `RevivedContextBadge` on the next snapshot read, even though (per the
    "User-visible signal transport" Pattern Decisions row) no toast fires
    for this path.
**Files**: `session/instance.go`

##### Task 1.6.1a: Replace the raw UUID capture in Resume()'s dead-pane branch (~5 min)
- In `session/instance.go`'s `Resume()` (line 1386), inside the `else`
  branch starting at line 1443 ("Tmux session is dead (killed on pause to
  free memory)..."), replace:
  ```go
  var claudeSessionID string
  if i.claudeSession != nil {
  	claudeSessionID = i.claudeSession.ConversationUUID
  }
  program := i.buildLaunchCommand(claudeSessionID)
  ```
  with:
  ```go
  // Attempt on-disk UUID recovery before rebuilding the launch command
  // (cold-start-uuid-loss Blocker 1 — same fix as startLocked/start's
  // cold-restore branch, applied here since a paused session's tmux pane
  // being dead is exactly the ColdRestore precondition).
  outcome := i.prepareColdRestore()
  i.LastReviveOutcome = outcome.Outcome
  var claudeSessionID string
  if outcome.Outcome.ShouldResume() {
  	claudeSessionID = i.GetConversationUUID()
  }
  program := i.buildLaunchCommand(claudeSessionID)
  ```
  Update the existing comment two lines above ("Rebuild the TmuxSession
  object with the current Claude UUID so the program is launched with the
  correct --resume flag") to say "with the current (or just-recovered)
  Claude UUID."
- Files: `session/instance.go`

##### Task 1.6.1b: Verify the existing `if claudeSessionID != ""` log line still reads correctly (~2 min)
- The existing `log.Info("resume: reinitializing tmux session with
  --resume", ...)` line at ~1466-1468 already branches on
  `claudeSessionID != ""` — no change needed there, since `claudeSessionID`
  is still populated (now via recovery-aware logic) before that check runs.
  Confirm by reading the surrounding 10 lines after Task 1.6.1a's edit that
  no stale reference to the old unconditional capture remains.
- Files: none (verification only)

---

### Epic 1.7: Integrate prepareColdRestore into `Restart()` (NEW — resolves adversarial Blocker 1)
**Goal**: `Restart()` — called from `SessionDriver.handleDriverFailure`'s
non-`Stopped` branch (`session/session_driver.go:551`) and from
`SwitchProgram` (`session/instance_program.go:76`) — attempts on-disk UUID
recovery before rebuilding the launch command. The adversarial review named
`SessionDriver` as plausibly the *dominant* field-observed trigger for this
whole bug class ("restarts faster than UUID capture can complete"), making
this the highest-value of the two new call sites.

#### Story 1.7.1: Restart calls prepareColdRestore before building the launch command
**As a** session that `SessionDriver` restarts after detecting an
unexpected exit, **I want** the restart to recover a UUID that was never
captured or was lost to the restart race itself, **so that** repeated
driver-triggered restarts do not compound into a fresh Claude with no
memory of the conversation — the scenario `research/features.md` (from the
prior project) identified as the likely dominant real-world trigger for
this bug.
**Acceptance Criteria**:
- `Instance.Restart()` calls `i.prepareColdRestore()` before capturing
  `claudeSessionID`, instead of reading `i.claudeSession.ConversationUUID`
  directly with no recovery attempt. Called **before** `KillSession()` (the
  pane may still be alive at this point — e.g. `SwitchProgram`'s
  `Restart(true)` on an `Active` session — which is fine because
  `prepareColdRestore` never depends on pane liveness, per Epic 1.2's
  `recoverConversationUUIDByPath` extraction).
  - *Given* an `Instance` whose effective status is not `Stopped`, whose
    tmux pane has actually died (the `handleDriverFailure` non-`Stopped`
    branch case — status hasn't caught up to reflect the dead pane yet),
    whose `i.claudeSession == nil`, and for which a resumable JSONL exists
    on disk, *When* `handleDriverFailure` calls `inst.Restart(false)`,
    *Then* the rebuilt `i.LaunchCommand` contains `--resume
    <recovered-uuid>` and `i.LastReviveOutcome ==
    ReviveOutcomeResumeRecovered`.
  - *Given* `SwitchProgram`'s `waspaused` branch (worktree recreation,
    lines ~1546-1561), which unconditionally clears `claudeSessionID` and
    the stored UUID/`HistoryFilePath` for a documented, unrelated reason
    (the encoded project path changes on worktree recreation, so any
    recovered UUID would be for the wrong path anyway), *When* that branch
    runs **after** `prepareColdRestore()` has already populated
    `claudeSessionID`, *Then* the `waspaused` override still wins —
    `claudeSessionID` ends up `""` regardless of what `prepareColdRestore`
    found, preserving today's existing (correct, unrelated) behavior
    unchanged.
**Files**: `session/instance.go`

##### Task 1.7.1a: Replace the raw UUID capture at the top of Restart() (~5 min)
- In `session/instance.go`'s `Restart()` (line 1509), replace the existing
  block (lines 1527-1531):
  ```go
  // Capture Claude session ID if available for resuming
  var claudeSessionID string
  if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" {
  	claudeSessionID = i.claudeSession.ConversationUUID
  }
  ```
  with:
  ```go
  // Capture Claude session ID if available for resuming — attempt on-disk
  // UUID recovery first (cold-start-uuid-loss Blocker 1) so a session whose
  // in-memory UUID was lost to a prior restart race (the SessionDriver
  // non-Stopped branch this method is called from, session_driver.go:551)
  // still resumes instead of silently starting fresh. Called before
  // KillSession() below; safe even if the pane is still alive at this point
  // (e.g. SwitchProgram's Restart(true) on an Active session) because
  // prepareColdRestore's recovery path never depends on pane liveness.
  outcome := i.prepareColdRestore()
  i.LastReviveOutcome = outcome.Outcome
  var claudeSessionID string
  if outcome.Outcome.ShouldResume() {
  	claudeSessionID = i.GetConversationUUID()
  }
  ```
- Files: `session/instance.go`

##### Task 1.7.1b: Verify the waspaused override block still executes after Task 1.7.1a's edit, unchanged (~2 min)
- Read `session/instance.go`'s `waspaused` block (lines ~1546-1561, inside
  `if i.gitManager.HasWorktree() { if waspaused { ... claudeSessionID = ""
  ... } }`) after Task 1.7.1a's edit and confirm it is textually unchanged
  and still runs after the new `prepareColdRestore()` call, so its
  deliberate override of `claudeSessionID`/`i.claudeSession.ConversationUUID`/
  `i.HistoryFilePath` for the worktree-recreation case is preserved exactly.
  Do **not** route this block through `ClearConversationState()` as part of
  this task — it is an unrelated, already-correct, same-cycle override with
  no future-cycle suppression implications (no `prepareColdRestore()` call
  happens again this cycle), so converting it would be scope creep beyond
  what any AC in this plan requires. Note this explicitly as a deliberate
  non-change in the PR description.
- Files: none (verification only)

---

## Phase 2: Durable revive-outcome signal (Goal 3, AC3)

### Epic 2.1: Proto enum and wire field
Unchanged from the prior plan.

#### Story 2.1.1: Add ReviveOutcome enum and revive_outcome field to proto
**As a** frontend developer, **I want** `session.reviveOutcome` available on
the `Session` message the same way `session.historyFilePath` already is,
**so that** `RevivedContextBadge` (Phase 3) can read it with no extra RPC —
and so that it reflects `LastReviveOutcome` set from **any** of the four
Phase 1 call sites, not just `startLocked`/`start`.
**Acceptance Criteria**:
- `proto/session/v1/types.proto` declares a top-level `enum ReviveOutcome`
  (5 values, `_UNSPECIFIED = 0` first) and a `ReviveOutcome revive_outcome =
  72;` field on `Session` (confirmed still free: `awk '/^message Session {/,/^}/'
  proto/session/v1/types.proto | grep -oE "= [0-9]+;" | sort -n | tail -1`
  → `71`).
**Files**: `proto/session/v1/types.proto`

##### Task 2.1.1a: Add the enum and field to types.proto (~4 min)
- Unchanged from the prior plan. Add, near `enum DetectedStatus` (line
  380):
  ```protobuf
  enum ReviveOutcome {
    REVIVE_OUTCOME_UNSPECIFIED = 0;
    REVIVE_OUTCOME_RESUME_LIVE = 1;
    REVIVE_OUTCOME_RESUME_RECOVERED = 2;
    REVIVE_OUTCOME_FRESH_EXPECTED = 3;
    REVIVE_OUTCOME_FRESH_LOST_HISTORY = 4;
  }
  ```
  Add inside `message Session { ... }`, after `string workspace_key = 71;`:
  `ReviveOutcome revive_outcome = 72;`
- Files: `proto/session/v1/types.proto`

##### Task 2.1.1b: Regenerate bindings (~2 min)
- Run `make proto-gen`. Verify `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` picked up the new enum/field.
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*_pb.ts` (generated)

##### Task 2.1.1c: Map Go ReviveOutcome to proto enum in the adapter (~4 min)
- Unchanged from the prior plan. In `server/adapters/instance_adapter.go`,
  near the existing `protoSession.HistoryFilePath = snap.HistoryFilePath`
  line (117), add a small `reviveOutcomeToProto(o session.ReviveOutcome)
  sessionv1.ReviveOutcome` switch function and set
  `protoSession.ReviveOutcome = reviveOutcomeToProto(snap.LastReviveOutcome)`.
- Files: `server/adapters/instance_adapter.go`

---

### Epic 2.2: Lifecycle event reason
**Goal**: `EventStarted` carries a defined reason string when this cycle's
outcome was `FRESH_LOST_HISTORY` — **only for the `startLocked`/`start`
call sites**, per the "User-visible signal transport" Pattern Decisions row
above. `Resume()`/`Restart()` (Epics 1.6/1.7) set `i.LastReviveOutcome`
directly without firing `EventStarted`, so they do not go through this
Epic's listener.

#### Story 2.2.1: Fire EventStarted with ReasonColdRestoreLostHistory from startLocked/start
**As a** listener subscribed to `Instance` lifecycle events, **I want** to
distinguish "started normally" from "forced fresh after a failed recovery"
without inspecting session state myself, **so that** the notification
listener in Epic 2.3 can be a thin, reason-string-driven filter.
**Acceptance Criteria**:
- `i.fireLifecycleEvent(EventStarted, reason)` is called with
  `ReasonColdRestoreLostHistory` exactly when `outcome.Outcome ==
  ReviveOutcomeFreshLostHistory` for this start cycle, and with `""` (as
  today) otherwise — at `startLocked`'s and `start`'s existing
  `fireLifecycleEvent(EventStarted, ...)` call sites only.
  - *Given* a `ColdRestore` where `prepareColdRestore()` returned
    `coldRestoreOutcome{Outcome: ReviveOutcomeFreshLostHistory}`, *When*
    `startLocked` reaches its existing `i.fireLifecycleEvent(EventStarted,
    "")` call (line 999), *Then* the reason argument is
    `"cold-restore-fresh-lost-history"` instead of `""`.
  - *Given* the same `FRESH_LOST_HISTORY` outcome reached via `Resume()` or
    `Restart()` instead, *When* those methods complete, *Then* **no**
    `fireLifecycleEvent(EventStarted, ...)` call is made — `i.LastReviveOutcome`
    is set (visible via the badge) but `EventStarted` (and therefore the
    Epic 2.3 toast, and `BacklogLifecycleListener.onSessionStarted`) is not
    triggered, per the deliberate scope decision recorded in Pattern
    Decisions and the new Unresolved Question.
**Files**: `session/instance_controller.go`, `session/instance.go`

##### Task 2.2.1a: Add the ReasonColdRestoreLostHistory constant (~2 min)
- Unchanged from the prior plan. In `session/instance_controller.go`, near
  the `LifecycleEvent` consts (line 73-88), add:
  ```go
  // ReasonColdRestoreLostHistory is passed to fireLifecycleEvent(EventStarted, ...)
  // when a cold restore was forced fresh after prepareColdRestore's recovery
  // attempt found nothing, despite the session having captured a real
  // conversation UUID at some point before this cycle. Only fired from
  // startLocked/start — see cold-start-uuid-loss's Pattern Decisions for why
  // Resume()/Restart() deliberately do not fire EventStarted.
  const ReasonColdRestoreLostHistory = "cold-restore-fresh-lost-history"
  ```
- Files: `session/instance_controller.go`

##### Task 2.2.1b: Pass the reason at both fireLifecycleEvent(EventStarted, ...) call sites (~3 min)
- Unchanged from the prior plan. In `session/instance.go`, change
  `i.fireLifecycleEvent(EventStarted, "")` at line 999 (`startLocked`) and
  line 1225 (`start`) to compute a `reason` local (`""` unless
  `i.LastReviveOutcome == ReviveOutcomeFreshLostHistory`, in which case
  `ReasonColdRestoreLostHistory`) and pass it instead of the literal `""`.
- Files: `session/instance.go`

---

### Epic 2.3: Notification listener
**Goal**: The reason string from Epic 2.2 reaches a durable, user-visible
notification, mirroring `onRateLimitRecovery`'s shape, with per-occurrence
ID entropy and a feature-neutral helper name.

#### Story 2.3.1: onColdRestoreLostHistory SessionService method and wiring
**As a** user whose session lost its previous conversation on revive, **I
want** a notification (not just a log line) telling me this happened, and
**I want** a second, later occurrence for the same session to also surface
(not silently collide with the first), **so that** I don't act on a false
assumption that the agent remembers our prior conversation, even after
repeated driver-restart churn.
**Acceptance Criteria**:
- `SessionService.onColdRestoreLostHistory` publishes exactly one
  `events.NewNotificationEvent` per firing, type WARNING (8), priority
  MEDIUM (2), gated on `!inst.Hidden`.
  - *Given* a non-`Hidden` `Instance` titled `"my-worktree-session"` whose
    `EventStarted` fires with `reason ==
    "cold-restore-fresh-lost-history"`, *When*
    `coldRestoreOutcomeListener.OnLifecycleEvent` receives it, *Then*
    `s.eventBus.Publish(events.NewNotificationEvent(...))` is called with
    `notificationType = 8` (WARNING), `priority = 2` (MEDIUM), and a title
    containing `"my-worktree-session"`.
  - *Given* the same event but `inst.Hidden == true`, *When* the listener
    receives it, *Then* no notification is published.
- The notification ID includes a per-occurrence entropy component, so a
  **second** `FRESH_LOST_HISTORY` occurrence for the same session is not
  silently dropped by `NotificationHistoryStore.Append()`'s exact-ID-match
  idempotency check.
  - *Given* two `EventStarted`/`ReasonColdRestoreLostHistory` firings for
    the same `inst.UUID` at two different times, *When* both reach
    `onColdRestoreLostHistory`, *Then* the two `notifID` values differ, and
    both reach `NotificationHistoryStore.Append()`'s `findUnreadDuplicate`
    path (collapse-if-unread, new-record-if-already-read) rather than both
    matching on `existing.ID == record.ID` and the second being silently
    no-op'd.
**Files**: `server/services/session_service.go`

##### Task 2.3.1a: Rename rateLimitLinkedItemID to linkedItemIDForInstance (~4 min)
- Folds in architecture review Concern 3. In
  `server/services/session_service.go`, rename `rateLimitLinkedItemID`
  (line 3964) to `linkedItemIDForInstance`, updating its doc comment to
  drop the rate-limit-specific framing ("looks up the backlog item ID
  linked to inst's session, bounded by a lookup timeout — used by any
  best-effort notification that wants to attach a linked backlog item")
  and updating its 2 existing call sites (`onRateLimitDetected` line 3985,
  `onRateLimitRecovery` line 4012) to the new name. Purely mechanical, no
  behavior change — run `go build ./server/...` to confirm.
- Files: `server/services/session_service.go`

##### Task 2.3.1b: Add coldRestoreOutcomeListener type (~3 min)
- Unchanged from the prior plan. In `server/services/session_service.go`,
  near `autoArchiveListener` (line 3865-3875), add:
  ```go
  type coldRestoreOutcomeListener struct {
  	svc  *SessionService
  	inst *session.Instance
  }

  func (l *coldRestoreOutcomeListener) OnLifecycleEvent(event session.LifecycleEvent, reason string) {
  	if event == session.EventStarted && reason == session.ReasonColdRestoreLostHistory {
  		l.svc.onColdRestoreLostHistory(l.inst)
  	}
  }
  ```
- Files: `server/services/session_service.go`

##### Task 2.3.1c: Add onColdRestoreLostHistory method with per-occurrence notifID (~5 min)
- Add, mirroring `onRateLimitRecovery` but using the renamed helper from
  Task 2.3.1a and a timestamp-suffixed `notifID` (adversarial Concern 1):
  ```go
  func (s *SessionService) onColdRestoreLostHistory(inst *session.Instance) {
  	if !inst.Hidden {
  		linkedItemID := s.linkedItemIDForInstance(inst)
  		notifID := fmt.Sprintf("cold-restore-lost-history-%s-%d", inst.UUID, time.Now().Unix())
  		s.eventBus.Publish(events.NewNotificationEvent(
  			inst.UUID, inst.Title, notifID,
  			int32(8), // NotificationType_WARNING
  			int32(2), // NotificationPriority_MEDIUM
  			fmt.Sprintf("Session %q started fresh — previous conversation could not be resumed", inst.Title),
  			"The session's tmux pane restarted and the previous conversation history could not be found on disk. Earlier context is not available.",
  			events.SessionScopedMetadata(nil, linkedItemID),
  		))
  	}
  }
  ```
  Unlike `onRateLimitRecovery`'s `notifID` (parameterized by a per-cycle
  Claude session ID passed in as a function argument), this notification
  has no equivalent per-cycle identifier available at the call site — a
  Unix timestamp is the simplest source of per-occurrence entropy that
  needs no new plumbing through `fireLifecycleEvent`'s string-only reason
  argument.
- Files: `server/services/session_service.go`

##### Task 2.3.1d: Register the listener in wireCallbacks (~2 min)
- Unchanged from the prior plan. In `wireCallbacks` (line 985), add
  `inst.RegisterLifecycleListener(&coldRestoreOutcomeListener{svc: s, inst:
  inst})` alongside the other `wire*`/`RegisterLifecycleListener` calls.
- Files: `server/services/session_service.go`

---

## Phase 3: Frontend surface

Unchanged from the prior plan — the `RevivedContextBadge` reads
`session.reviveOutcome` directly off the `Session` proto, which Epic 2.1's
adapter mapping populates from `snap.LastReviveOutcome` regardless of which
of the four Phase 1 call sites set it. No frontend change is needed to
cover the two new call sites; this is precisely why the badge (not the
toast) is the mechanism AC3's "at minimum" bar is scoped against for
`Resume()`/`Restart()`.

### Epic 3.1: RevivedContextBadge

#### Story 3.1.1: RevivedContextBadge component
**As a** user browsing my session list, **I want** to see, at a glance and
without opening the session, that a session's context was lost on its last
restart, **so that** I know not to trust "continue where we left off"
before I've even opened it.
**Acceptance Criteria**:
- `RevivedContextBadge` renders only when `session.reviveOutcome ===
  ReviveOutcome.FRESH_LOST_HISTORY`; renders nothing (`null`) for the other
  3 enum values.
  - *Given* a `Session` proto value with `reviveOutcome ===
    ReviveOutcome.FRESH_LOST_HISTORY`, *When* `<RevivedContextBadge
    session={session} />` renders, *Then* the DOM contains an element with
    `role="status"`, `aria-label="This session lost its previous
    conversation and started fresh"`, and a decoration icon marked
    `aria-hidden="true"` — following `ConnectionIndicator.tsx`'s
    accessibility shape.
  - *Given* a `Session` with any other `reviveOutcome` value, *When*
    `<RevivedContextBadge session={session} />` renders, *Then* it returns
    `null`.
**Files**: `web-app/src/components/sessions/RevivedContextBadge.tsx`, `web-app/src/components/sessions/RevivedContextBadge.css.ts`

##### Task 3.1.1a: Create RevivedContextBadge.css.ts (~3 min)
- Unchanged from the prior plan. New vanilla-extract style file per
  `.claude/rules/css-architecture.md`, colocated with the component.
  Minimal: a `wrapper` style (inline-flex, small gap) and an `icon` style.
- Files: `web-app/src/components/sessions/RevivedContextBadge.css.ts`

##### Task 3.1.1b: Create RevivedContextBadge.tsx (~4 min)
- Unchanged from the prior plan.
  ```tsx
  "use client";
  import { ReviveOutcome, Session } from "@/gen/session/v1/types_pb";
  import { icon, wrapper } from "./RevivedContextBadge.css";

  interface RevivedContextBadgeProps { session: Session; }

  export function RevivedContextBadge({ session }: RevivedContextBadgeProps) {
    if (session.reviveOutcome !== ReviveOutcome.FRESH_LOST_HISTORY) return null;
    return (
      <span
        className={wrapper}
        role="status"
        aria-live="polite"
        aria-label="This session lost its previous conversation and started fresh"
        data-testid="revived-context-badge"
      >
        <span className={icon} aria-hidden="true">⚠</span>
        Context lost
      </span>
    );
  }
  ```
- Files: `web-app/src/components/sessions/RevivedContextBadge.tsx`

##### Task 3.1.1c: Wire into SessionCard.tsx (~3 min)
- Unchanged from the prior plan. Next to the existing `<StatusBadge
  detectedStatus={detectedStatus} .../>` call (line 538), add
  `<RevivedContextBadge session={session} />`.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 3.1.1d: Wire into SessionRow.tsx (~3 min)
- Unchanged from the prior plan. Same wiring in
  `web-app/src/components/sessions/SessionRow.tsx`, extending the row's
  `aria-label` (line 210) to append `", context: lost"` when applicable.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.1.2a: Verify the existing toast pipeline needs no changes (~2 min)
- Unchanged from the prior plan. Run `cd web-app && npx jest --no-coverage
  --testPathPatterns="NotificationToast"` to confirm nothing needs to
  change.
- Files: none (verification only)

---

## Phase 4: Tests

### Epic 4.1: Backend tests
**Goal**: Cover the recovery-before-decision ordering itself at **all four**
call sites, the generation-scoped suppression guard (replaying adversarial
Blocker 2's exact trace), the zero-byte-JSONL filter, the `ShouldResume()`
method, the durable signal, the renamed helper, and the notifID entropy fix.

#### Story 4.1.1: prepareColdRestore unit tests
**As a** reviewer of this fix, **I want** unit tests at the smallest
testable unit (`prepareColdRestore` itself), **so that** the ordering logic
is verified without the flakiness/heaviness of real tmux sessions.
**Acceptance Criteria**:
- All 6 branches of `prepareColdRestore` (HasClaudeSession-true,
  generation-matched-suppression, recovery-succeeds, recovery-fails +
  EverHadHistory, recovery-fails + never-had-history, same-directory
  collision) are covered by a distinct test, plus the two new
  generation-mismatch cases from Story 1.3.1's ACs.
**Files**: `session/instance_cold_restore_decision_test.go` (new)

##### Task 4.1.1a: TestPrepareColdRestore_RecoversUUID_When_JSONLExistsButUUIDNeverCaptured (~5 min)
- Unchanged from the prior plan (adjusted for the dropped `Resume` field).
  Temp home dir, encoded project dir, a written `.jsonl` file. Assert
  `coldRestoreOutcome{Outcome: ReviveOutcomeResumeRecovered}` and
  `inst.everHadConversationHistory == true` (AC1).
- Files: `session/instance_cold_restore_decision_test.go` (new)

##### Task 4.1.1b: TestPrepareColdRestore_StartsFresh_When_NoJSONLExists (~4 min)
- Unchanged from the prior plan. No JSONL directory created. Assert
  `coldRestoreOutcome{Outcome: ReviveOutcomeFreshExpected}` and
  `inst.everHadConversationHistory == false` (AC2).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1c: TestPrepareColdRestore_SkipsRecovery_When_SuppressedGenerationMatches (~5 min)
- Rewritten for the generation-scoped design. Set `inst.startCycleGeneration
  = 5` and `inst.recoverySuppressedGeneration = 6` (i.e. armed for the very
  next call), with a JSONL present on disk (so a naive implementation
  *would* recover it). Call `prepareColdRestore()` once; assert the result
  is `ReviveOutcomeFreshExpected` (not `ResumeRecovered`),
  `DetectByPath`/`recoverConversationUUIDByPath` is never reached (verify
  via a `historyDetector` test double that fails the test if `DetectByPath`
  is called), `inst.startCycleGeneration == 6` afterward, and
  `inst.recoverySuppressedGeneration == 0` (consumed).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1d: TestPrepareColdRestore_AttemptsRecovery_When_SuppressedGenerationIsStale (~5 min)
- New test, directly covering adversarial Blocker 2's failure trace. Set
  `inst.startCycleGeneration = 5` and `inst.recoverySuppressedGeneration =
  6` (armed for generation 6), then call `prepareColdRestore()` **twice**
  without an intervening `ClearConversationState()` call — the first call
  consumes generation 6 as expected (asserts `FreshExpected`, matching Task
  4.1.1c); the **second** call (now at generation 7, with
  `recoverySuppressedGeneration` already reset to `0` by the first call)
  must attempt recovery normally against a JSONL present on disk, asserting
  `ResumeRecovered` — proving a stale suppression cannot silently apply to
  a second, unrelated cycle the way the prior plan's bare bool could.
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1e: TestPrepareColdRestore_SignalsFreshLostHistory_When_RecoveryFailsButEverHadHistory (~4 min)
- Unchanged from the prior plan (renumbered). `inst.everHadConversationHistory
  = true`, no JSONL on disk. Assert `coldRestoreOutcome{Outcome:
  ReviveOutcomeFreshLostHistory}` (AC3).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1f: TestPrepareColdRestore_NoSignal_When_GenuinelyNeverHadHistory (~3 min)
- Unchanged from the prior plan (renumbered). `inst.everHadConversationHistory
  = false` (default), no JSONL. Assert `ReviveOutcomeFreshExpected`, not
  `FreshLostHistory`.
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1g: TestPrepareColdRestore_SameDirectoryCollision_DocumentsKnownLimitation (~5 min)
- Unchanged from the prior plan (renumbered). Two `Instance`s pointed at
  the same encoded project dir, one JSONL file with a pinned mtime (`os.Chtimes`).
  Assert both instances recover the **same** UUID (documents the accepted
  limitation).
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1h: TestPrepareColdRestore_WorksRegardlessOfPaneLiveness (~4 min)
- New test, covering architecture Concern 2's resolution directly. Build an
  `Instance` with a process-manager test double whose `IsAlive()` returns
  `true` (simulating the `Restart()`-before-`KillSession()` case) and
  `i.claudeSession == nil`, with a resumable JSONL on disk. Call
  `prepareColdRestore()` and assert it still returns `ResumeRecovered` —
  proving `prepareColdRestore` does not depend on (and, unlike the prior
  plan's version, never even checks) `i.pm().IsAlive()`.
- Files: `session/instance_cold_restore_decision_test.go`

##### Task 4.1.1i: TestReviveOutcome_ShouldResume (~2 min)
- New, small table test: `ReviveOutcomeResumeLive.ShouldResume() ==
  true`, `ReviveOutcomeResumeRecovered.ShouldResume() == true`,
  `ReviveOutcomeFreshExpected.ShouldResume() == false`,
  `ReviveOutcomeFreshLostHistory.ShouldResume() == false`.
- Files: `session/instance_claude_test.go`

##### Task 4.1.1j: TestDetectByPath_SkipsZeroByteJSONL (~4 min)
- New, covering Task 1.2.1d. Write two files to the encoded project dir: a
  0-byte `<uuid-a>.jsonl` with a newer mtime, and a real (non-empty)
  `<uuid-b>.jsonl` with an older mtime. Assert `DetectByPath` returns
  `uuid-b`, not `uuid-a` — proving the zero-byte candidate is filtered out
  even though it would otherwise win on recency.
- Files: `session/history_detector_test.go`

#### Story 4.1.2: Integration-level ordering tests proving the launch command is actually fixed, at all four call sites
**As a** reviewer skeptical that this fix does more than change a log line,
**I want** tests asserting `--resume <uuid>` is actually present in the
built launch command at each of `startLocked`, `start`, `Resume`, and
`Restart`, **so that** the fix is proven against the actual bug at every
call site the adversarial review named, not just the two the prior plan
covered.
**Acceptance Criteria**:
- Four tests (one per call site), each driving a real cold-restore-shaped
  scenario with no in-memory UUID but a real on-disk JSONL, and inspecting
  `inst.LaunchCommand` for the `--resume` flag.
**Files**: `session/instance_cold_restore_test.go`

##### Task 4.1.2a: TestColdRestore_RecoversUUIDBeforeLaunch_When_UUIDNeverCapturedButJSONLExists (~5 min)
- Unchanged from the prior plan. Following `TestColdRestore_WithUUID`'s
  tmux-fixture setup, set a real `HistoryFileDetector` pointed at a temp
  home dir containing a written JSONL, do **not** call
  `inst.SetClaudeSession(...)`. After `inst.StartWithCleanup(false)`,
  assert `strings.Contains(inst.LaunchCommand, "--resume")`.
- Files: `session/instance_cold_restore_test.go`

##### Task 4.1.2b: TestColdRestore_Mirror_start_RecoversUUIDBeforeLaunch (~3 min)
- Unchanged from the prior plan. `StartWithCleanup` already calls
  `i.start(...)` directly, so Task 4.1.2a already covers this — note
  explicitly in the PR description rather than adding a duplicate test.
- Files: none (verification, documented in PR description)

##### Task 4.1.2c: TestResume_RecoversUUIDBeforeLaunch_When_UUIDNeverCapturedButJSONLExists (~6 min)
- New, covering Epic 1.6. Build a `Paused` `Instance` whose tmux pane is
  confirmed dead (`i.pm().IsAlive() == false` on the test double), whose
  `i.claudeSession == nil`, and whose `historyDetector` is pointed at a
  temp home dir containing a written JSONL under the instance's encoded
  effective root dir. Call `i.Resume()`. Assert the resulting
  `i.LaunchCommand` contains `--resume <uuid-from-jsonl>` and
  `i.LastReviveOutcome == ReviveOutcomeResumeRecovered` — this specific
  assertion is what would fail against pre-fix `Resume()`, which built the
  command from `i.claudeSession.ConversationUUID` (nil at this point) with
  no recovery attempt.
- Files: `session/instance_cold_restore_test.go`

##### Task 4.1.2d: TestRestart_RecoversUUIDBeforeLaunch_When_UUIDNeverCapturedButJSONLExists (~6 min)
- New, covering Epic 1.7. Build a `started`, non-`Stopped` `Instance` whose
  `i.claudeSession == nil` and whose `historyDetector` is pointed at a temp
  home dir containing a written JSONL. Call `i.Restart(false)`. Assert the
  resulting `i.LaunchCommand` contains `--resume <uuid-from-jsonl>` and
  `i.LastReviveOutcome == ReviveOutcomeResumeRecovered` — mirrors 4.1.2c,
  proving the fix at the `SessionDriver`-triggered call site the
  adversarial review flagged as the plausibly dominant real-world trigger.
- Files: `session/instance_cold_restore_test.go`

##### Task 4.1.2e: TestRestart_WaspausedOverrideStillWinsAfterPrepareColdRestore (~5 min)
- New, covering Story 1.7.1's second AC. Build a `Paused`, worktree-backed
  `Instance` with a recoverable JSONL on disk (so `prepareColdRestore`
  would otherwise return `ResumeRecovered`). Call `i.Restart(false)` with
  `waspaused == true` (i.e. `i.Status == Paused` at call time). Assert the
  final `i.LaunchCommand` does **not** contain `--resume` — proving the
  existing worktree-recreation override (lines ~1546-1561) still wins over
  whatever `prepareColdRestore` found, unchanged by this fix.
- Files: `session/instance_cold_restore_test.go`

#### Story 4.1.3: Regression guards for the intentional-clear flows
**As a** reviewer verifying Goal 4/AC6, **I want** explicit tests proving
`recoverFromStaleResume` does not loop back onto the same stale UUID via
the new recovery path, and that a stale suppression cannot leak into a
later, unrelated cycle, **so that** this fix cannot silently reintroduce
the bug `recoverFromStaleResume` already fixed, nor the new bug adversarial
Blocker 2 found in the prior plan's design.
**Acceptance Criteria**:
- Unchanged core AC from the prior plan, plus the generation-leak
  scenario now covered by Task 4.1.1d above (cross-referenced here rather
  than duplicated).
**Files**: `session/instance_claude_test.go`

##### Task 4.1.3a: TestRecoverFromStaleResume_DoesNotRediscoverSameStaleUUID_OnNextColdRestore (~5 min)
- Unchanged from the prior plan. Write the JSONL for the stale UUID to
  disk, call `inst.recoverFromStaleResume()`, and assert
  `prepareColdRestore()`'s result is `ReviveOutcomeFreshExpected`, not a
  resume of the stale UUID.
- Files: `session/instance_claude_test.go`

##### Task 4.1.3b: Verify TestTryExtractConversationUUID_PathFallbackRepopulatesAfterClear still passes, updated for the extracted helper (~3 min)
- Run `go test ./session/... -run
  TestTryExtractConversationUUID_PathFallbackRepopulatesAfterClear -v`
  after all Phase 1 changes land. Unlike the prior plan's version of this
  task (which expected zero modifications), Task 1.2.1c's refactor changes
  `tryExtractConversationUUID`'s path-fallback branch to delegate to
  `recoverConversationUUIDByPath()` — if this test asserts against
  internal call structure rather than externally-observable
  UUID/HistoryFilePath behavior, update it to match the refactor; if it
  only asserts observable behavior (the expected case, per AC5), it should
  pass unmodified. Note in the PR description which case applied.
- Files: `session/instance_claude_test.go` (updated only if needed)

##### Task 4.1.3c: TestClearConversationState_ArmsSuppressionForNextGenerationOnly (~4 min)
- New. Directly tests Story 1.2.1's ACs: call `ClearConversationState()` on
  an instance with `startCycleGeneration == 5`; assert
  `recoverySuppressedGeneration == 6`. Then simulate two intervening
  `prepareColdRestore()` calls via a stub/direct field manipulation that
  advances `startCycleGeneration` past `6` without consuming the
  suppression (representing a hypothetical future call path that doesn't
  go through `prepareColdRestore`); assert that a subsequent real
  `prepareColdRestore()` call at generation `8` does **not** honor the
  stale suppression — the self-correcting property the generation design
  is chosen for.
- Files: `session/instance_claude_test.go`

### Epic 4.2: Frontend tests

#### Story 4.2.1: RevivedContextBadge tests
Unchanged from the prior plan.
**As a** reviewer of the frontend change, **I want** a Jest/RTL test
confirming the badge only renders for `FRESH_LOST_HISTORY` and carries the
correct accessible name, **so that** the accessibility contract is enforced
by CI.
**Acceptance Criteria**:
- Test file covers both the positive-render and null-render cases from
  Story 3.1.1's acceptance criteria.
**Files**: `web-app/src/components/sessions/__tests__/RevivedContextBadge.test.tsx`

##### Task 4.2.1a: RevivedContextBadge.test.tsx — positive and null render (~4 min)
- Unchanged from the prior plan. New test file, following
  `StatusBadge.test.tsx`'s structure. Two cases: `reviveOutcome ===
  FRESH_LOST_HISTORY` renders the badge with correct `aria-label`/`role`/
  hidden icon; each of the other 3 enum values renders `null`.
- Files: `web-app/src/components/sessions/__tests__/RevivedContextBadge.test.tsx`

##### Task 4.2.1b: Run the full frontend suite for this directory (~2 min)
- Unchanged from the prior plan. `cd web-app && npx jest --no-coverage
  --testPathPatterns="RevivedContextBadge|SessionCard|SessionRow"`.
- Files: none (verification only)
