# Implementation Plan: pr-fix-steering

**Feature**: When `AutoReopenForPRFix` finds an already-active work session for a `pr_pending`
item with a CI failure, blocking review, or merge conflict, steer that live session with the
problem detail instead of silently skipping the respawn.
**Date**: 2026-08-26
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-program-gating-exact-match.md) (program-gating exact-match),
[ADR-002](../decisions/ADR-002-steer-failed-stuck-reason.md) (new `StuckReasonSteerFailed`)

**Event-Command-Policy/EventStorming note**: Not produced. Both the architecture research for
this project and the sibling `pr-review-followup` project (same reconciliation loop, same
Complexity-2 scoring) concluded this table isn't warranted at this scope — the change is a
single new branch inside one existing reconcile method plus a small, already-scoped set of
pure helper functions, not a new event-driven subsystem with multiple commands/policies to
map.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionSteerer` | Consumer-defined interface (`server/services/backlog_service.go`) exposing `SessionProgram` and `SteerActiveSession` to `BacklogService`, satisfied by `*SessionService`. | Mirrors `SessionStopper`'s existing shape/placement exactly. |
| `SessionProgram` | `SessionSteerer` method: `(program string, ok bool)` — the live `Instance.Program` for a session UUID, or `ok=false` if not tracked live. | Read-only lookup; never mutates. |
| `SteerActiveSession` | `SessionSteerer` method: injects `message` into the live session identified by `sessionUUID`, returning an error on any delivery failure. | Delegates to `steerInstance` after resolving the instance via `FindLiveInstance`. |
| `steerInstance` | New private method on `*SessionService` extracted from `UpdateSession`'s inline `SteerMessage` handling — the implementation `UpdateSession`'s RPC handler uses, and which `SteerActiveSession` also delegates to. | Extract-Method refactor; also where the autonomous-branch silent-error bug is fixed. Returns only plain wrapped errors (`fmt.Errorf`) on every branch — never a `connect.NewError`/`connect.Code*` — since `SteerActiveSession` calls it in-process from `BacklogService`, which must not need to import `connectrpc.com/connect` to interpret a delivery error. `UpdateSession` is the sole place that translates a `steerInstance` error into a `connect.Code` (see Task 1.1.1b). Not every steering entry point in the codebase goes through this method: the MCP `steer_session` tool (`server/mcp/tools_terminal.go`) resolves the instance and calls `SendKeys`/`RunWithResume` directly, with its own independent timeout handling, unaffected by anything in this plan. |
| `fixContext` | The pre-existing string parameter to `AutoReopenForPRFix`/`PRFixSpawner`, equal to `PRStatus.FeedbackText` — a rendered, `## <Section>`-headed markdown description of the PR's problem(s). | Reused verbatim as the steer message body; never re-derived. |
| `reasonSignature` | New value type: the ordered list of `## <Section>` markdown headers present in a `fixContext` string (e.g. `["## Merge conflict", "## Failing CI checks"]`) — or, for a header-less `fixContext` (e.g. the "PR closed without merging" call site, `session/backlog_lifecycle_pr.go`, which is a plain sentence with no `"## "` headers), a single-element signature holding the trimmed full string itself, so two different header-less messages don't collapse into the same empty signature. | Built by `buildReasonSignature`; deliberately excludes header body text (CI log snippets, reviewer comment bodies) so dedup isn't defeated by volatile content. |
| `lastSteerReason` | Per-item dedup state: `{signature reasonSignature, at time.Time, sessionUUID string}`, the most recently *delivered* reason signature, when, and which session received it. | Mirrors `session/nudge_dedup.go`'s `lastNudge` shape, plus `sessionUUID` (architecture review concern: if the item's active work session changes between ticks — e.g. a human manually starts a replacement session — dedup state keyed only by item would silently never steer the new session for an already-"delivered" reason). Stored in `BacklogService.steerDedup` (`sync.Map`, key = item ID). |
| `steerCooldown` | Package var, `5 * time.Minute` — how long an exact-repeat `reasonSignature` stays suppressed once delivered. | See Unresolved Questions / rationale below; distinct from `maxReworkBlockStaleness` (15min, different purpose) and `nudgeCooldown` (3min, different message class). |
| `isDuplicateSteerReason` | Pure function: `(candidate reasonSignature, last lastSteerReason, sessionUUID string, now time.Time, cooldown time.Duration) bool` — true if `sessionUUID` equals `last.sessionUUID`, `candidate` equals `last.signature`, and `now` is within `cooldown` of `last.at`. | Mirrors `isDuplicateNudge`'s shape. A `sessionUUID` mismatch against `last.sessionUUID` is treated the same as "never delivered" (bypasses cooldown) regardless of signature equality — see Story 4.2.1's session-changed GWT example. |
| `nextLastSteerReason` | Pure function: `(prev lastSteerReason, candidate reasonSignature, sessionUUID string, delivered bool) lastSteerReason` — advances dedup state (including which session received it) only on a successful delivery. | Mirrors `nextLastNudge`. |
| `conflictDebounceState` | Per-item state: `{pending *pendingConflict}`, where `pendingConflict{signature reasonSignature, since time.Time}` — tracks a `## Merge conflict` header that has appeared but not yet been confirmed on two consecutive ticks. | Stored in `BacklogService.steerConflictDebounce` (`sync.Map`, key = item ID). `pending == nil` unambiguously means "nothing pending" — a type-enforced state, not a two-field convention (a non-nil `pending` is guaranteed fully populated by construction, so "signature set but since zero" is unrepresentable). |
| `confirmConflictChange` | Pure function: `(candidate reasonSignature, last reasonSignature, sessionUUID string, state conflictDebounceState) (confirmed bool, next conflictDebounceState)` — implements the two-consecutive-tick confirmation for a *newly appearing* conflict header. | See Pattern Decisions and Story 2.3.1 for the exact state machine. Like `isDuplicateSteerReason`, a `sessionUUID` mismatch against `state.pending.sessionUUID` is treated as "nothing pending yet for this session" (restarts confirmation), not a match. |
| `steerInFlight` | New `sync.Map` field on `BacklogService`, keyed by item ID, guarding `steerActiveSessionForPRFix`'s **entire body** — every degrade branch as well as delivery — against two overlapping calls racing to steer/notify the same item before dedup state is recorded. | Self-cleaning `LoadOrStore`/`defer Delete`, mirrors `spawnInFlight`. Wraps the whole method, not just the `SteerActiveSession` call, because `reconcilePRPendingItem` is shared by both the 60s-tick loop and the synchronous, webhook-triggered `TriggerPRFixForEvent` (pre-mortem.md P2 #2) — a routine overlap for a CI-churning PR, not a rare race. |
| `steerActiveSessionForPRFix` | New private `BacklogService` method, called from `AutoReopenForPRFix`'s active-session branch in place of the old unconditional skip. | Owns the nil-safe/dedup/debounce/program-gating/notification orchestration. |
| `buildSteerMessage` | Pure function: `(program string, fixContext string) string` — applies program-gating (ADR-001) and truncation, producing the final PTY-bound message. | Truncates to `session.MaxSteerMessageLength` with a trailing pointer if `fixContext` (plus any appended slash-command suffix) would exceed it. |
| `notifyActiveSessionSteered` | New helper alongside `notifyRespawnBlockedByActiveSession` — publishes a `NOTIFICATION_TYPE_INFO`/`LOW` event and resolves any open `StuckReasonRespawnBlockedActive` *and* `StuckReasonSteerFailed` rows on success; publishes `NOTIFICATION_TYPE_WARNING`, marks a new `StuckReasonSteerFailed` row, and resolves any open `StuckReasonRespawnBlockedActive` row on failure. Takes the triggering `reasonSignature` and the session's `program` as parameters (the same values `steerActiveSessionForPRFix` already computed as `candidate`/`program`) so its title/body can name the actual reason category, per `research/ux.md` §2, instead of a generic phrase, and so the failure branch can append a remediation path for Claude Code sessions (Task 4.3.2b, UX triad review). | Both paths carry `metadata: {"item_id": itemID}`, matching the established pattern. The cross-resolution (each path clears the *other* reason) keeps the two `StuckReason`s mutually exclusive per item — see Story 4.3.2's "at most one open at a time" acceptance criterion (adversarial review). `steerActiveSessionForPRFix`'s own degrade paths mirror this via the new `resolveSteerFailedLogged` helper (Task 4.3.2c). No `item.PrNumber` is threaded into this call chain anywhere in this plan (verified: `grep -n PrNumber plan.md` has no hits before this note) — the reason-naming title uses `itemTitle`, not a PR number, per Task 4.3.2a/b. The failure branch's remediation-path suffix reuses `isClaudeCodeProgram` (Task 3.1.1a) rather than re-implementing the same program check a second time (Epic 3.1's `buildSteerMessage` already made this exact decision to gate the `/github:pr-ship` instruction). |
| `humanReadableReasonSet` | New pure function: `(sig reasonSignature) string` — turns a `reasonSignature`'s ordered headers into a short, comma/"and"-joined human phrase for a notification title/body (e.g. `["## Merge conflict"]` → `"a merge conflict"`; `["## Merge conflict", "## Failing CI checks"]` → `"a merge conflict and failing CI"`). Strips the dynamic `@author` half of a `"## Review: changes requested by @author"` header down to `"a blocking review"` — that level of detail stays in the terminal's full `fixContext`, per `research/ux.md` §2's "not the full `FeedbackText` Markdown blob" guidance. Falls back to `"a PR problem"` for a header-less signature (e.g. the "PR closed without merging" call site). | Used by both branches of `notifyActiveSessionSteered` (Tasks 4.3.2a/4.3.2b). Defined alongside `notifyActiveSessionSteered` in `server/services/backlog_service_pr_fix_steer.go`. |
| `StuckReasonSteerFailed` | New `domain.StuckReason` constant (`"steer_failed"`) / proto `STUCK_REASON_STEER_FAILED = 19` — marks a backlog item whose active-session steer attempt failed. | See ADR-002. Distinct from `StuckReasonRespawnBlockedActive`, which still covers the nil-safe/not-live/dedup-suppressed degrade paths. |
| `findActiveWorkSession` | Existing helper (`backlog_service_triage.go`) — returns the open work-role `ItemSession` for an item, if any. | Unchanged; still the trigger condition for the branch this project modifies. |
| `HasActiveWorkSession` | New `session.PRFixSpawner` method (Story 4.2.2, pre-mortem.md P1 fix): `(ctx, itemID) (bool, error)` — a side-effect-free check mirroring `findActiveWorkSession`, implemented on `*BacklogService`. | Lets `remediatePRFixWithBackoffGate` (`session/backlog_lifecycle_pr.go`) decide whether to bypass its 30m→72h `RemediationDue` backoff gate for the steer path *before* calling `AutoReopenForPRFix`, without duplicating `AutoReopenForPRFix`'s own steer-vs-spawn decision — that decision stays solely inside `AutoReopenForPRFix`/`steerActiveSessionForPRFix`. |
| `mockSessionSteerer` | New test fake implementing `SessionSteerer`, mirroring `mockSessionStopper` (`server/services/backlog_service_test.go:173`). | Wired via `svc.SetSessionSteerer(...)` in tests. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Cross-package call from `BacklogService` into session steering | Consumer-defined interface (`SessionSteerer`) + setter injection | PoEAA (Fowler): Service Layer boundary; precedent `SessionStopper` | (a) Call `SessionService.UpdateSession`'s RPC handler directly; (b) publish a `PRProblemDetected` domain event on the event bus for `SessionService` to self-consume | (a) forces constructing a `connect.Request`/proto message just to reuse two lines of branching logic, and couples `BacklogService` to a wire-format type instead of a 2-method interface — the exact Interface Pollution smell (forwarding-only wrapper) this repo's checklist flags. (b) makes the steer attempt's success/failure asynchronous and un-awaitable, but `steerActiveSessionForPRFix` needs a synchronous result to decide whether to advance dedup state and which notification to publish — event-bus fan-out reintroduces the indirection both this project's and the sibling project's research already found unwarranted at this scope (see the omitted Event-Command-Policy table above). |
| `UpdateSession`'s inline steer logic vs. the new `SteerActiveSession` caller | Extract Method → shared private `steerInstance` | GoF: not a creational/structural/behavioral gap — pure refactoring discipline (DRY) | Reimplement the autonomous/interactive branching a second time inside `SteerActiveSession` | A second implementation would drift from `steerInstance` over time (e.g. one branch gets the nil-controller fix, the other doesn't) and duplicates already-tested logic (`TestUpdateSession_SteerMessage_*`) instead of reusing it. |
| Error type crossing the `steerInstance` boundary | `steerInstance` returns only plain wrapped errors (`fmt.Errorf("steer session %q: %w", ...)`); `connect.Code*` translation happens exclusively at `UpdateSession`'s own call site | Clean Architecture's inward-only dependency rule: an infrastructure/transport-layer error type must not leak into a shared primitive a non-transport caller also consumes | Keep `connect.NewError(connect.CodeFailedPrecondition/CodeDeadlineExceeded, ...)` inside `steerInstance` (the original extraction's naive verbatim move) | `BacklogService.SteerActiveSession` calls `steerInstance` in-process from a reconciliation loop, never over RPC. Leaving `connect.NewError` inside `steerInstance` would force `backlog_service_pr_fix_steer.go` to import `connectrpc.com/connect` just to branch on delivery-failure class — the same leaky-abstraction smell `.claude/rules/interface-pollution-checklist.md` flags for interfaces, applied here to an error type (architecture review finding). |
| Autonomous-branch nil-controller/"controller not started" handling | Return the error to the caller (bug fix during extraction) | Type-driven design: don't let a `nil`/error case silently degrade to a log line when the caller can act on it | Keep the existing `log.Warn`-and-continue behavior | Most backlog work sessions run autonomous — silently swallowing this exact failure mode is precisely the gap requirements.md's "every steer attempt (success or failure) must be visible" success metric exists to close; leaving it would make `SteerActiveSession`'s new caller report false successes. |
| Dedup durability | In-memory `sync.Map` (`steerDedup`), not a DB column/table | PoEAA: in-process cache vs. Repository — bounded, disposable state doesn't need a Repository | Persist `lastSteerReason` in the `backlog_stuck_state` table or a new column | Blast radius of a lost dedup entry (e.g. server restart) is one redundant PTY message, not a re-dispatched session or lost work — the same reasoning `spawnInFlight`/`steerInFlight` already apply to in-memory-only guards in this file. A DB column would need a migration this project's research confirmed is unnecessary. |
| Conflict re-steer trigger | Explicit 2-consecutive-tick confirmation (`confirmConflictChange`), separate from the cooldown check | Type-driven design: model "pending confirmation" as its own small state machine rather than folding it into the cooldown's boolean | Treat any `HasConflicts`-containing `reasonSignature` change as immediately steerable, relying on the existing cooldown alone | GitHub's `mergeStateStatus` is known-stale (cli/cli#9583, cited in pitfalls research §6) — a single tick's "still conflicting" read may already be wrong if the agent just pushed a fix. Folding this into the cooldown (a *time*-based suppressor) would not fix a *staleness* problem — two different failure modes need two different guards. |
| Program-gating comparison | Exact literal match after the `program == "" \|\| program == "claude"` idiom | See ADR-001 | `strings.Contains(strings.ToLower(program), "claude")` (mirroring `ClaudeAdapter.CanHandle`) | `CanHandle` solves a different problem (transcript-format adapter selection) and would false-positive on `"proxy-claude"`, a real configured value in this codebase's own test fixtures. |
| Failed-steer visibility | New `StuckReasonSteerFailed` constant | See ADR-002 | Reuse `StuckReasonRespawnBlockedActive` with distinguishing free-text `detail` | `BlockerChip` renders a fixed label ("Auto-respawn skipped…") keyed purely by the enum value — the free-text detail isn't shown on the chip, so reusing the reason would render "skipped" for an outcome that is strictly worse (attempted and failed), contradicting the "must not regress to silence" success metric. |
| Steer message construction | Transaction Script (`buildSteerMessage`, a single pure function: gate → truncate → return) | PoEAA: Transaction Script for a single, non-branching business rule with no persistent state of its own | A `SteerMessageBuilder` type/interface | The logic is one linear pipeline with a single caller today — introducing a type/interface here would be the Speculative Interface smell this repo's checklist explicitly flags (one implementation, no near-term second one). |

---

## Migration Plan
*Omitted — no schema or data changes.* `domain.StuckReason` and the ent `backlog_stuck_state.reason` column are both plain, unconstrained strings (`session/ent/schema/backlog_stuck_state.go:37`); adding `StuckReasonSteerFailed` is a new Go constant + proto enum value + adapter switch case + frontend map entries, not a migration. Confirmed no DB `CHECK` constraint on this column exists in the schema.

## Observability Plan
- **Logs**: one structured log line per steer attempt from `steerActiveSessionForPRFix`, matching the existing `[AutoReopenForPRFix]` prefix convention (e.g. `[AutoReopenForPRFix] steering active session %s for item %s: %s` / `... steer failed: %v`).
- **Metrics**: none new — this reuses the existing ~60s reconcile ticker; no new counter/gauge is introduced per requirements.md's "negligible added work per tick" NFR. (A future metric, e.g. a steer-attempt counter, is a candidate follow-up, not required here.)
- **Alerts**: none new. A failed steer surfaces via the existing notification/`BlockerChip` machinery (`notifyActiveSessionSteered`'s WARNING path + `StuckReasonSteerFailed` row), which is the alerting surface this whole feature is built on top of.

## Risk Control
- **Feature flag**: none, per requirements.md's explicit Risk Control section — this extends code (`AutoReopenForPRFix`) that already runs unconditionally in production; a flag would need its own plumbing for a change whose blast radius is already bounded by dedup + the `SessionSteerer`-nil-safe degrade path.
- **Rollback procedure**: revert the PR. `SetSessionSteerer` wiring is additive (one new line in `server/dependencies.go`); removing it or reverting the whole diff restores the exact prior behavior (`notifyRespawnBlockedByActiveSession`-only), since the nil-safe degrade path is exercised by construction whenever `sessionSteerer` is unset.
- **Staged rollout**: not applicable — single-operator deployment (`~/.stapler-squad/` systemd service), no multi-tenant staging. The nil-safe degrade path (Story 4.2.1) is itself the safety net: if `SetSessionSteerer` is ever un-wired or misbehaves, the system falls back to today's shipped behavior automatically, not manually.
- **Pre-existing test-suite flakiness (no action needed)**: the new `-race` concurrency test (Task 5.3.1h) and this project's DI/wiring tests (`SetSessionSteerer`, Epic 4.1) add load to `server/services`, a package with documented CI flakiness under full-suite parallel load (BUG-067). This is a known pre-existing consideration for this package, not a new problem this project introduces — per `.claude/rules/fix-flaky-tests-dont-defer.md`'s philosophy of fixing observed flakes rather than pre-emptively engineering around a hypothetical one, no action is taken here beyond this note.

## Unresolved Questions
- [ ] None blocking. `steerCooldown = 5 * time.Minute` is a deliberate choice, not a placeholder: it clears the ≥60s floor requirements.md's Open Questions section asked for (the reconcile ticker's own cadence, `server/dependencies.go:1180`'s `time.NewTicker(60 * time.Second)`), giving ~5 reconcile ticks of headroom for a steered agent to actually start responding before an identical-reason re-steer could fire — long enough to avoid spamming a session that's already working the problem, short enough that a dropped/lost steer (e.g. a mid-flight server restart clearing `steerDedup`) retries well within a single work session's lifetime. It sits below `maxReworkBlockStaleness` (15min, `backlog_service_triage.go:1289` — a different purpose: staleness of a *whole session*, not a message) and above `nudgeCooldown` (3min, `session/nudge_dedup.go` — a live in-session nudge, lower latency/consequence than a reconciler-driven PR-fix steer). If this value proves too short/long in practice, it's a one-line `var` change with no other coupling.
- [ ] `AutoRespawnReview`/`AutoRespawnAutonomousWork`'s own active-session-skip branches are explicitly out of scope (requirements.md) — tracked as future follow-up only, not a gap in this plan.
- [ ] requirements.md's Rabbit Holes item on pane-snapshot re-arm detection (mirroring `session/nudge_dedup.go`'s `isDuplicateNudge`, which re-arms a cooldown whenever the terminal pane's own output changes) was investigated and deliberately rejected, not silently dropped: `research/build-vs-buy.md` §3 finds it would be "nearly useless" for this feature specifically, because a working agent produces near-constant PTY output as a matter of course — a pane-output-based re-arm would look "re-armed" on essentially every tick, defeating the cooldown entirely. This plan's `reasonSignature`-based dedup (Epic 2.1/2.2) uses reason-content change as its re-arm trigger instead, which is well-defined for this use case.
- [ ] **(Product triad review, 2nd pass) Does the Medium (1–2 weeks) appetite still hold, now reconciled against the sibling `pr-review-followup` project (also Complexity-2, same reconciliation loop) mapping to Small (1–3 days)?** A fresh Product-lens review found the prior note's "task/story counts grew only modestly... more headroom than the delta alone would demand" framing asserted that Medium had slack without independently justifying why Medium — not Small — was the right starting bucket, and without reconciling the ~80% raw-task-time gap (~4.6h vs. ~2.55h, both floors excluding review/iteration/debugging) against only a +15%/+7% task/story-count difference. Replacing that framing with the actual causal argument, verified against this project's own artifacts:
  1. **Medium was a deliberate Phase-1 choice, not an unexamined default.** requirements.md's Scope section commits, from ideation onward — before any planning-phase task breakdown existed — to both the change-detection/cooldown mechanism (In Scope bullet 3) and a visible backlog-item comment/notification per steer attempt (In Scope bullet 4, "per the standing project preference that self-heal/auto-act paths must never act silently," requirements.md:111-113). Those two capabilities are exactly what separates a "wire it once" Small scope from a "wire it, plus a cooldown policy and an audit trail" Medium scope. **Verification note**: the literal two-option wording reportedly presented to the user during Phase 1 ideation (a concretely-scoped Small vs. Medium choice) is not preserved in any project artifact and could not be independently confirmed for this reconciliation — what is verifiable is that requirements.md's checked-in Scope section already commits to the Medium-tier capabilities, consistent with a deliberate choice rather than scope drift, even though the exact original option phrasing can't be cited.
  2. **The growth beyond that initial Medium estimate came from four rounds of review finding real, verified correctness bugs — not speculative feature creep.** Each of the following, left unfixed, would have shipped broken or silently inert: the autonomous-branch swallowed-controller-error and unbounded-PTY-write bugs in `steerInstance` (Story 1.1.2; adversarial-review.md's "Unbounded `SendCommandImmediate` hang leaking `steerInFlight`" prior blocker) and their web-app consequence, a dialog that would silently swallow the now-real RPC error (Story 1.1.3); the `steerInFlight` guard widening from delivery-only to the method's entire body, closing a routine (not rare) race between the 60s ticker and the webhook-dispatched goroutine (pre-mortem.md P2 #2, Task 4.2.1a); the `buildReasonSignature` header-less false-dedup bug for the "PR closed without merging" call site (Task 2.1.1b, adversarial-review.md); the `remediatePRFixWithBackoffGate` 30m→72h backoff gate that would have starved the entire cooldown/dedup design for up to 72h after the first steer (pre-mortem.md P1, Story 4.2.2); the Connect-RPC error type that would otherwise leak from `steerInstance` into its non-transport caller (Pattern Decisions: "Error type crossing the `steerInstance` boundary"); and the generic-phrase notification that named nothing about the actual PR problem on the one path (a failed steer) where the notification is the operator's only record (`humanReadableReasonSet`, Task 4.3.2a, `research/ux.md` §2/§3).
  3. **Given #2, the honest reconciliation is that trimming scope now to force a Small rating would mean deliberately re-introducing bugs this project's own review process already found and fixed** — a worse outcome than accepting that Medium, chosen deliberately and then modestly exceeded by real bug fixes surfaced through rigor, is the right characterization rather than a discipline failure. The ~80% effort/count gap against the sibling is explained by this project's dual nature: it is simultaneously a reconciliation-loop extension (like the sibling) **and** a bug-fix/hardening pass on adjacent, pre-existing code (`UpdateSession`'s swallowed error, `RemediationDue`'s backoff gate not accounting for a content-aware caller) that the sibling project's scope never touched.
  **Conclusion: Medium still holds**, now for a substantiated causal reason rather than an asserted "had slack" claim. If a future revision finds growth that is *not* explained by a verified review-found bug fix (i.e. genuine scope creep, not hardening), re-run this comparison rather than assuming the same conclusion still applies.

## Dependency Visualization

```
Phase 1: Steer Primitive (SessionService + its web-app caller)
  Epic 1.1 Extract steerInstance (Story 1.1.1)
             └─▶ fix its swallowed-error/PTY-timeout bugs (Story 1.1.2)
                    └─▶ fix "Give Direction" dialog's result-checking (Story 1.1.3) ─┐
                        (1.1.3 depends on 1.1.2 landing first: 1.1.2's behavior
                        change is what turns the dialog's pre-existing silent
                        result-discard from harmless into a real swallowed error)
  Epic 1.2 SessionSteerer interface + DI ◄────────────────────────────────────────────┘ (needs steerInstance to delegate to)
                    │
                    ▼
Phase 2: Reason Signature, Dedup & Debounce (pure functions, no dependency on Phase 1)
  Epic 2.1 reasonSignature type/builder
  Epic 2.2 Cooldown dedup (isDuplicateSteerReason / nextLastSteerReason)
  Epic 2.3 Conflict two-tick debounce (confirmConflictChange)
                    │
                    ▼
Phase 3: Steer Message Construction (needs ADR-001 gating; independent of Phase 1/2 internals)
  Epic 3.1 buildSteerMessage (program-gate + truncate)
                    │
      ┌─────────────┴─────────────┐
      ▼                           ▼
Phase 4: Integration (needs Phase 1 interface, Phase 2 dedup/debounce, Phase 3 message)
  Epic 4.1 BacklogService fields + SetSessionSteerer
  Epic 4.2 steerActiveSessionForPRFix + call-site swap (Story 4.2.1)
            + remediatePRFixWithBackoffGate bypass for the steer path,
              PRFixSpawner.HasActiveWorkSession (Story 4.2.2 — pre-mortem P1)
  Epic 4.3 StuckReasonSteerFailed (domain/proto/adapter/web-app) + notifyActiveSessionSteered
                    │
                    ▼
Phase 5: Tests (validates all of the above)
  Epic 5.1 steerInstance unit tests (Phase 1)
  Epic 5.2 dedup/debounce pure-function tests (Phase 2)
  Epic 5.3 AutoReopenForPRFix integration tests (Phase 4)
```

---

## Phase 1: Steer Primitive Extraction & Fix (SessionService + its web-app caller)

### Epic 1.1: Extract `steerInstance` from `UpdateSession`
**Goal**: Move the existing autonomous/interactive steer branching (`server/services/session_service.go:2929-2972`) into a private, independently callable method with zero behavior change for `UpdateSession`, translating transport-layer errors only at `UpdateSession`'s own call site (not inside the extracted method), then fix the two documented bugs in the autonomous branch (the swallowed controller error, and its unbounded PTY write) — and fix the one existing web-app caller (the "Give Direction" dialog) that the autonomous-branch bug fix would otherwise leave silently swallowing a now-real RPC error (Story 1.1.3).

#### Story 1.1.1: Extract-Method refactor preserving current `UpdateSession` behavior
**As a** maintainer, **I want** the steer branching logic isolated in its own method, **so that** a second caller (`SteerActiveSession`) can reuse it without duplicating the autonomous/interactive split.
**Acceptance Criteria**:
- `UpdateSession`'s `SteerMessage` handling block is replaced by a call to `s.steerInstance(ctx, instance, *req.Msg.SteerMessage)`, with all existing pre-checks (length validation against `session.MaxSteerMessageLength`) still performed before the call.
  - *Given* an `UpdateSessionRequest` with `SteerMessage` set to a string longer than `session.MaxSteerMessageLength`, *When* `UpdateSession` runs, *Then* it returns `connect.CodeInvalidArgument` before `steerInstance` is ever invoked (unchanged from today).
- Every existing `TestUpdateSession_SteerMessage_*` test in `server/services/session_service_test.go` passes unmodified after the refactor, including the ones asserting `connect.CodeFailedPrecondition` on the RPC response (`TestUpdateSession_SteerMessage_NonAutonomousSession_SendKeysFailure_ReturnsFailedPrecondition`, `TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController`) — these assert on `UpdateSession`'s externally observable RPC code, which is unchanged even though the code is now constructed at `UpdateSession`'s own call site instead of inside `steerInstance` (see the "Error type crossing the `steerInstance` boundary" Pattern Decision).
  - *Given* the pre-refactor test suite for `TestUpdateSession_SteerMessage_*`, *When* run against the post-refactor `session_service.go`, *Then* all cases pass with no test-file changes required for this story (test-file changes for the bug fix are Story 1.1.2, not this one).
- `steerInstance` itself never constructs a `connect.NewError`/`connect.Code*` value on any branch — it returns only plain wrapped errors (`fmt.Errorf`). `UpdateSession` is the sole translator from a `steerInstance` error to a `connect.Code`.
  - *Given* `steerInstance`'s non-autonomous branch encountering a `SendKeys` failure or a timeout, *When* the returned error is inspected directly (bypassing `UpdateSession`), *Then* it is a plain `error` for which `errors.As(err, &connectErr)` (with `connectErr *connect.Error`) is `false`.
**Files**: `server/services/session_service.go`

##### Task 1.1.1a: Extract `steerInstance(ctx, instance, message) error` (~5 min)
- Add a new private method `func (s *SessionService) steerInstance(ctx context.Context, instance *session.Instance, message string) error` directly below `UpdateSession`.
- Move the full body of the existing `if instance.AutonomousMode { ... } else { ... }` block (lines 2934-2971) into this method, replacing `*req.Msg.SteerMessage` with the `message` parameter, and — per the "Error type crossing the `steerInstance` boundary" Pattern Decision — replacing the non-autonomous branch's two `return nil, connect.NewError(connect.Code*, fmt.Errorf(...))` returns with plain wrapped errors instead of moving them verbatim:
  ```go
  select {
  case err := <-errCh:
      if err != nil {
          return fmt.Errorf("steer session %q: %w", instance.Title, err)
      }
  case <-timeoutCtx.Done():
      return fmt.Errorf("timed out steering session %q: %w", instance.Title, timeoutCtx.Err())
  }
  ```
  (dropping the `*sessionv1.UpdateSessionResponse` nil return, since the new method returns only `error`). Wrapping the timeout branch's error around `timeoutCtx.Err()` (which is `context.DeadlineExceeded`) — rather than a bare string — is deliberate: it's what lets `UpdateSession`'s call site (Task 1.1.1b) distinguish "timed out" from "delivery failed" via `errors.Is` without `steerInstance` needing to know about `connect.Code` at all.
- Files: `server/services/session_service.go`

##### Task 1.1.1b: Replace `UpdateSession`'s inline block with the call, translating the error into a `connect.Code` (~4 min)
- Replace the moved block in `UpdateSession` with:
  ```go
  if err := s.steerInstance(ctx, instance, *req.Msg.SteerMessage); err != nil {
      if errors.Is(err, context.DeadlineExceeded) {
          return nil, connect.NewError(connect.CodeDeadlineExceeded, err)
      }
      return nil, connect.NewError(connect.CodeFailedPrecondition, err)
  }
  ```
  keeping the pre-existing length-check (`if len(*req.Msg.SteerMessage) > session.MaxSteerMessageLength { ... }`) above it unchanged. This is the one place in the whole plan that constructs a `connect.Code` from a `steerInstance` outcome — `SteerActiveSession` (Story 1.2.1) never does this translation, since it isn't an RPC handler.
- Files: `server/services/session_service.go`

##### Task 1.1.1c: Build and run existing steer tests (~2 min)
- Run `go build ./server/... && go test ./server/services -run TestUpdateSession_SteerMessage -v` and confirm all pass unmodified.
- Files: none (verification only)

#### Story 1.1.2: Fix the autonomous branch's swallowed controller error, and bound its PTY write with a timeout
**As an** operator, **I want** a steer attempt against an autonomous session with no running controller (or a wedged one) to return a real error within a bounded time, **so that** `SteerActiveSession`'s caller (and any future caller) can tell "delivered" from "silently dropped," and a dead controller can never permanently wedge the reconciler's per-item steer guard.
**PR-description callout (architecture review)**: this story changes `UpdateSession`'s externally observable RPC behavior for *every* caller of that RPC (the "Give Direction" UI dialog, the `steer_session` MCP tool's callers that go through `UpdateSession`, any future caller) — not only the new `SteerActiveSession` path this project introduces. It's low-risk (the sibling non-autonomous branch has surfaced steer failures as RPC errors for a while, so existing callers already handle a failing steer), but the eventual PR description must call this out explicitly as a public RPC behavior change, not bury it as an internal refactor detail a reviewer could miss scanning the diff.
**Acceptance Criteria**:
- `steerInstance`'s autonomous branch returns a non-nil error (not just a `log.Warn`) when `instance.GetController()` returns `nil`, or when `SendCommandImmediate` itself errors — both as plain wrapped errors, never a `connect.NewError` (per the "Error type crossing the `steerInstance` boundary" Pattern Decision — `UpdateSession` alone assigns the `connect.Code`).
  - *Given* a `session.Instance` with `AutonomousMode=true` and `GetController()` returning `nil` (no controller started), *When* `steerInstance(ctx, instance, "fix the conflict")` is called, *Then* it returns a non-nil error containing `"controller not started"` (or equivalent) and does **not** call `s.notifySteerSent`.
  - *Given* the same instance but `GetController()` returns a non-nil controller whose `SendCommandImmediate` returns an error, *When* `steerInstance` is called, *Then* it returns that error wrapped with context (e.g. `fmt.Errorf("steer autonomous session %q: %w", instance.Title, sendErr)`) and does not call `notifySteerSent`.
- The autonomous branch's `SendCommandImmediate` call is bounded by a timeout, mirroring the sibling non-autonomous branch's existing `SendKeys` bound — it must not be able to hang the calling goroutine forever.
  - *Given* a fake controller whose `SendCommandImmediate` blocks past the timeout (never returns), *When* `steerInstance(ctx, instance, "fix the conflict")` is called on an autonomous instance backed by that controller, *Then* `steerInstance` returns within (approximately) 5 seconds with a non-nil error wrapping `context.DeadlineExceeded`, rather than blocking indefinitely — verified in isolation, and, per Story 4.2.1's `steerInFlight` guard, this is exactly the condition that would otherwise permanently leak that guard's `defer Delete` for the item.
- `UpdateSession`'s externally observable behavior for this case changes from "200 OK, silent no-op" to "an RPC error is returned" — this is the deliberate, scoped bug fix (requirements.md decision #2), not a regression.
  - *Given* the same nil-controller autonomous instance, *When* a `steer_session` MCP call or UI steer action triggers `UpdateSession` with `SteerMessage` set, *Then* the RPC now returns a non-OK status (`connect.CodeFailedPrecondition`, assigned by `UpdateSession`'s Task 1.1.1b translation) instead of silently succeeding.
**Files**: `server/services/session_service.go`, `server/services/session_service_test.go`

##### Task 1.1.2a: Replace the `log.Warn`-and-continue with a returned error, bounded by a timeout (~6 min)
- In the autonomous branch of `steerInstance`, change:
  ```go
  controller := instance.GetController()
  if controller != nil {
      if _, sendErr := controller.SendCommandImmediate(message + "\r"); sendErr != nil {
          log.Warn("[steerInstance] failed to send steer_message", "session", instance.Title, "err", sendErr)
      } else {
          s.notifySteerSent(instance, message)
      }
  }
  ```
  to:
  ```go
  controller := instance.GetController()
  if controller == nil {
      return fmt.Errorf("steer autonomous session %q: controller not started", instance.Title)
  }

  // Bound the PTY write the same way the sibling non-autonomous branch already
  // bounds SendKeys: SendCommandImmediate's own ~5min internal timeout doesn't
  // start protecting until after the raw PTY write already happened
  // (session/pty_access.go's Write has no deadline), so a wedged/dead
  // controller can hang this call indefinitely — which would leak
  // steerActiveSessionForPRFix's steerInFlight guard forever (its deferred
  // Delete never runs).
  errCh := make(chan error, 1)
  go func() {
      _, sendErr := controller.SendCommandImmediate(message + "\r")
      errCh <- sendErr
  }()

  timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
  defer cancel()

  select {
  case sendErr := <-errCh:
      if sendErr != nil {
          return fmt.Errorf("steer autonomous session %q: %w", instance.Title, sendErr)
      }
  case <-timeoutCtx.Done():
      return fmt.Errorf("timed out steering autonomous session %q: %w", instance.Title, timeoutCtx.Err())
  }
  s.notifySteerSent(instance, message)
  return nil
  ```
  Wrapping the timeout case around `timeoutCtx.Err()` (`context.DeadlineExceeded`) matches the non-autonomous branch's Task 1.1.1a pattern exactly, so `UpdateSession`'s single `errors.Is(err, context.DeadlineExceeded)` check (Task 1.1.1b) correctly routes this branch's timeout to `connect.CodeDeadlineExceeded` too, with no autonomous-specific translation logic needed.
- Files: `server/services/session_service.go`

##### Task 1.1.2b: Add regression tests for the fixed error path and the timeout bound (~7 min)
- Add `TestSteerInstance_AutonomousNilController_ReturnsError` and `TestSteerInstance_AutonomousSendCommandImmediateError_ReturnsError` to `server/services/session_service_test.go`, following the existing `TestUpdateSession_SteerMessage_*` naming/fixture style (construct a minimal `*session.Instance` with `AutonomousMode: true` and no controller / a fake erroring controller).
- Add `TestSteerInstance_AutonomousSendCommandImmediateHangs_TimesOutRatherThanBlockingForever`: use a fake controller whose `SendCommandImmediate` blocks on a channel that the test never closes (past the 5-second timeout), call `steerInstance` in the test goroutine with a normal (non-canceled) `context.Background()`, and assert both that the call returns (use `require.Eventually` or a bounded `select`/timer around the call itself, not `time.Sleep`-then-check) and that the returned error satisfies `errors.Is(err, context.DeadlineExceeded)`.
- Add `TestUpdateSession_SteerMessage_AutonomousNilController_NowReturnsError` confirming `UpdateSession` itself now surfaces this as a non-nil error with `connect.CodeFailedPrecondition` (documents the intentional behavior change from Story 1.1.2's acceptance criteria, and confirms the Task 1.1.1b translation applies to the autonomous branch too).
- Files: `server/services/session_service_test.go`

##### Task 1.1.2c: Run the new and existing steer tests together (~2 min)
- `go test ./server/services -run "TestSteerInstance|TestUpdateSession_SteerMessage" -v` — confirm new tests pass and no existing test regresses.
- Files: none (verification only)

#### Story 1.1.3: Fix the "Give Direction" dialog to check the steer result instead of assuming success
**As an** operator using the "Give Direction" dialog on an autonomous session, **I want** to see an error if my steering message failed to deliver, **so that** Story 1.1.2's behavior change (an autonomous steer with a dead/not-started controller now returns a real RPC error instead of silently no-op'ing) doesn't leave me thinking a failed steer succeeded.
**Context**: Verified directly — `handleSteerAutonomousSession` (`web-app/src/app/page.tsx:292-295`) calls `updateSession(sessionId, { steerMessage: message })` and discards the result; `useSessionService.ts`'s `updateSession` never throws, it catches internally, dispatches `setError(...)` to Redux, and returns `null` on any RPC failure. The caller, `SessionActionsOverflow.tsx`'s "Give Direction" dialog (`:504` Enter-key handler, `:519` Send-button handler), doesn't await `onSteerAutonomousSession` at all — it calls it and unconditionally runs `setIsSteerOpen(false); setSteerMessage("")` on the very next lines regardless of outcome. Today this is harmless because a nil-controller autonomous steer silently no-ops server-side; after Story 1.1.2 ships, the identical click now produces a real failed `updateSession` call that this path still doesn't check, so the dialog closes as if it worked and the operator is told nothing. The app's established pattern for exactly this kind of user-facing RPC-failure surfacing is `useNotifications()`'s `addNotification({ message, notificationType: "error", sessionId, sessionName })` (used the same way in `web-app/src/components/sessions/SessionList.tsx` for a failed bulk delete) — reuse it rather than inventing a new mechanism.
**Acceptance Criteria**:
- `handleSteerAutonomousSession` reports whether the steer succeeded to its caller, and surfaces a toast via `addNotification(...)` on failure.
  - *Given* `updateSession(...)` resolves to `null` (the documented failure signal), *When* `handleSteerAutonomousSession(sessionId, message)` runs, *Then* it calls `addNotification` with `notificationType: "error"` and resolves to `false`.
  - *Given* `updateSession(...)` resolves to a non-null `Session`, *When* `handleSteerAutonomousSession` runs, *Then* it does not call `addNotification` and resolves to `true`.
- The "Give Direction" dialog (`SessionActionsOverflow.tsx`) only closes and clears its input when the steer actually succeeded; on failure it stays open (with the typed message preserved) so the operator can retry or see the toast.
  - *Given* `onSteerAutonomousSession` resolves to `false`, *When* the operator presses Enter or clicks Send in the dialog, *Then* `isSteerOpen` remains `true` and `steerMessage` is unchanged (not cleared).
  - *Given* `onSteerAutonomousSession` resolves to `true`, *When* the operator presses Enter or clicks Send, *Then* the dialog closes and the input clears — unchanged from today's behavior.
- **(UX triad review)** The Send button (and the input's Enter-key path) is disabled from the moment the steer RPC is fired until it resolves (success or failure), so a slow response plus a double-click or double-Enter can't fire a second, duplicate `SteerActiveSession`/`UpdateSession` call. This is not a new pattern for this file: `SessionActionsOverflow.tsx` already disables its Restart-confirm button via an `isRestarting` boolean state var for the exact same reason (`:110,309-310`, `handleRestartConfirm`) — the "Give Direction" dialog is simply the first of this component's action dialogs missing the equivalent guard.
  - *Given* the operator has pressed Send (or Enter) and the steer RPC has not yet resolved, *When* the operator presses Enter or clicks Send again, *Then* no second `onSteerAutonomousSession` call is made — the control is disabled (or the handler no-ops) for the duration of the in-flight call.
  - *Given* the RPC resolves (either outcome), *When* the dialog is still open (i.e. the failure case), *Then* the control re-enables so the operator can retry.
- **(UX triad review, 2nd pass)** Disabling the input and Send button while `isSteering` is `true` must not leave the dialog un-closeable by keyboard for the duration of the RPC: the Cancel button stays enabled throughout, and Escape closes the dialog regardless of `isSteering`.
  - *Given* the operator has pressed Send and the steer RPC has not yet resolved, *When* the operator presses Escape, *Then* the dialog closes (`isSteerOpen` becomes `false`) even though the input and Send button are currently disabled.
  - *Given* the same in-flight state, *When* the Cancel button's `disabled` prop is inspected, *Then* it is `false`/absent — only the input and Send button are disabled, never the whole dialog.
- **(UX triad review, 3rd pass — focus-handling regression)** Disabling the currently-focused input the instant `isSteering` becomes `true` must not rely on the browser's native auto-blur-to-`document.body` behavior: that implicit refocus breaks *both* the Escape handler (attached to the dialog wrapper `<div>`'s own `onKeyDown` per the 2nd-pass fix above — a `div`'s `onKeyDown` only receives events bubbling from its own descendants, so once focus leaves the dialog's DOM subtree entirely, Escape stops reaching it) *and* `useFocusTrap`'s Tab-cycling (its focusable-elements snapshot is taken once when the dialog opens and isn't re-evaluated when the input is disabled, so the trap keeps trying to cycle back to an element that's no longer in the tab order). The fix is to move focus explicitly to the Cancel button — a still-enabled, still-focusable control already present in the trap's initial snapshot — in the same code path that sets `isSteering(true)`, before the RPC fires, so focus never actually leaves the dialog.
  - *Given* the Give Direction dialog is open with the input focused, *When* the operator presses Enter to send, *Then* focus moves to the Cancel button before the RPC completes, and Escape/Tab continue to work while the RPC is in flight.
**Files**: `web-app/src/app/page.tsx`, `web-app/src/components/sessions/SessionActionsOverflow.tsx`, `tests/e2e/session-actions-steer-focus.spec.ts`

##### Task 1.1.3a: Make `handleSteerAutonomousSession` check the result and toast on failure (~5 min)
- In `web-app/src/app/page.tsx`, add `addNotification` from `useNotifications()` (`@/lib/contexts/NotificationContext`) to this component's existing hooks, and change:
  ```tsx
  const handleSteerAutonomousSession = useCallback(async (sessionId: string, message: string): Promise<void> => {
      track({ name: "session_autonomous_steer", category: "user_action" });
      await updateSession(sessionId, { steerMessage: message });
    }, [updateSession, track]);
  ```
  to:
  ```tsx
  const handleSteerAutonomousSession = useCallback(async (sessionId: string, message: string): Promise<boolean> => {
      track({ name: "session_autonomous_steer", category: "user_action" });
      const result = await updateSession(sessionId, { steerMessage: message });
      if (result === null) {
          addNotification({
              message: "Failed to send steering message — the session may not be running.",
              notificationType: "error",
              sessionId,
              sessionName: "",
          });
          return false;
      }
      return true;
    }, [updateSession, track, addNotification]);
  ```
- Files: `web-app/src/app/page.tsx`

##### Task 1.1.3b: Update the prop type and dialog handlers in `SessionActionsOverflow.tsx` (~9 min)
- Change the `onSteerAutonomousSession?: (sessionId: string, message: string) => void;` prop type to `onSteerAutonomousSession?: (sessionId: string, message: string) => Promise<boolean> | void;` (kept optional/loose so any future caller that doesn't need the result isn't forced to return one).
- Add an `isSteering` boolean state var (mirroring this same file's existing `isRestarting`/`isDeleting`/`isCreatingCheckpoint`/`isSavingProgram` in-flight-disable pattern — `:110` for `isRestarting`'s declaration, `handleRestartConfirm`'s `disabled={isRestarting}` wiring at `:309-310`), set `true` immediately before firing the steer call and back to `false` in a `finally` once it resolves (both success and failure), so a slow response plus a double-click/double-Enter can't fire a duplicate call (UX triad review finding).
- **(UX triad review, 2nd pass) Escape-to-close vs. the disabled input.** Disabling the text `input` while `isSteering` is `true` (below) means that element stops receiving keydown events in most browsers, so the existing Escape handler at `:508` (attached to the `input`'s own `onKeyDown`) would become unreachable while a steer is in flight — the dialog couldn't be closed by keyboard until the RPC resolved. **Verified this dialog already has a separate Cancel button** (`:529-531`, `onClick={... setIsSteerOpen(false); setSteerMessage(""); }}`), distinct from the Send button and the input. Fix: only the input and the Send button get `disabled={isSteering}` — the Cancel button is **not** added to that list, so it stays enabled and Tab-reachable for the entire in-flight duration, and it needs no result-checking or `finally` wiring since closing the dialog doesn't depend on the steer outcome. In addition, move the `Escape` case out of the input's own `onKeyDown` and onto the dialog content `<div>`'s `onKeyDown` (`:488-495`, which is never disabled — divs have no disabled state), so pressing Escape closes the dialog regardless of `isSteering`, not just when the input happens to be enabled and focused.
- **(UX triad review, 3rd pass) Move focus to Cancel *before* disabling the input, don't rely on browser auto-blur.** Add a `cancelButtonRef = useRef<HTMLButtonElement>(null)` alongside `steerDialogRef`, and attach it to the Cancel button (`ref={cancelButtonRef}`). Disabling the focused input without first moving focus somewhere still-enabled lets the browser auto-blur to `document.body` — invisibly moving focus *outside* the dialog's DOM subtree, which defeats both the Escape handler (a bubbling listener on the dialog `<div>` never sees an event that originates at `document.body`) and `useFocusTrap`'s Tab-cycling (its focusable-elements snapshot, taken once at dialog-open time, isn't re-evaluated when the input drops out of the tab order). Calling `cancelButtonRef.current?.focus()` in the same statement that sets `isSteering(true)` — before the input's `disabled` prop takes effect and before the RPC fires — keeps focus inside the dialog the whole time, so both mechanisms keep working unmodified.
- In the Enter-key handler (`:503-506`) and the Send-button handler (`:517-521`), guard on `isSteering`, set it around the call, and await the call, only clear/close on a truthy (or `undefined`, for backward compatibility with a `void`-returning caller) result. Add `onKeyDown` to the dialog content `<div>` for Escape, since the input's own handler no longer owns that case:
  ```tsx
  <div
    ref={steerDialogRef}
    role="dialog"
    aria-modal="true"
    aria-labelledby="steerDialogTitle"
    className={dialogContent}
    onClick={(e) => e.stopPropagation()}
    onKeyDown={(e) => {
        if (e.key === "Escape") { setIsSteerOpen(false); setSteerMessage(""); }
    }}
  >
    {/* ... */}
    <input
      /* ... */
      disabled={isSteering}
      onKeyDown={async (e) => {
          if (e.key === "Enter" && steerMessage.trim() && !isSteering) {
              setIsSteering(true);
              // Move focus to Cancel *before* the input's disabled prop takes
              // effect, so focus stays inside the dialog instead of the
              // browser auto-blurring it to document.body (see the 3rd-pass
              // bullet above — that auto-blur is what breaks Escape and the
              // focus trap's Tab-cycling once the input leaves the tab order).
              cancelButtonRef.current?.focus();
              try {
                  const ok = await onSteerAutonomousSession?.(session.id, steerMessage.trim());
                  if (ok !== false) { setIsSteerOpen(false); setSteerMessage(""); }
              } finally {
                  setIsSteering(false);
              }
          }
      }}
    />
    {/* ... */}
    <button ref={cancelButtonRef} onClick={(e) => { e.stopPropagation(); setIsSteerOpen(false); setSteerMessage(""); }} className={cancelButton}>
      Cancel
    </button>
  ```
  and the same `cancelButtonRef.current?.focus()` call analogously in the Send button's `onClick`, immediately after its own `setIsSteering(true)`. Add `disabled={isSteering}` to **only** the Send button and the text `input` (so typing is also blocked mid-flight, consistent with the confirm-dialog precedent disabling both its action buttons) — deliberately **not** to the Cancel button (`:529-531`), which stays enabled and Tab-reachable throughout so the dialog always has a keyboard-operable close affordance even while the RPC is in flight. Swap the Send button's label to "Sending…" while `isSteering` is `true` (matching `isRestarting`'s `"Restarting..."` label-swap convention).
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 1.1.3c: Add/adjust frontend tests and typecheck (~9 min)
- Add a test to this component's existing test file covering: `onSteerAutonomousSession` resolving `false` keeps the dialog open and preserves the typed message; resolving `true` closes it and clears the input (mirrors this repo's existing dialog-behavior test style).
- Add a test covering the in-flight disable: with `onSteerAutonomousSession` returning a pending (not-yet-resolved) promise, assert the Send button is disabled while the promise is pending, and a second click/Enter does not invoke `onSteerAutonomousSession` a second time; resolve the promise and assert the control re-enables.
- **(UX triad review, 2nd pass)** Add a test asserting Escape still closes the dialog while `isSteering` is `true` (RPC pending): with `onSteerAutonomousSession` returning a pending promise, press Send/Enter to start it, then dispatch an `Escape` keydown on the dialog and assert `isSteerOpen` becomes `false` — this is the regression guard for the disabled-input-swallows-keydown gap this task's fix closes. Also assert the Cancel button remains enabled (not `disabled`) while `isSteering` is `true`.
- Run `cd web-app && pnpm exec tsc --noEmit && npx jest --no-coverage --testPathPatterns="SessionActionsOverflow|page"`.
- **Scope note (UX triad review, 3rd pass):** these jsdom/RTL tests exercise the *application logic* (state transitions, which handlers fire) but not real browser focus/blur behavior — jsdom does not reproduce a browser auto-blurring a disabled, focused element to `document.body`. They confirm the Escape handler and `isSteering` wiring are implemented correctly; they cannot, on their own, prove the focus-handling regression described in the new acceptance criterion above is actually fixed in a real browser. That proof is Task 1.1.3d's job.
- Files: `web-app/src/components/sessions/__tests__/SessionActionsOverflow.test.tsx` (confirmed existing test file — also check `__tests__/SessionActionsOverflow.focus.test.tsx` isn't affected by the handler signature change)

##### Task 1.1.3d: Add a real-browser Playwright test for focus handling during an in-flight steer (~10 min)
**(UX triad review, 3rd pass)**: jsdom/RTL (Task 1.1.3c) cannot reproduce a real browser's auto-blur-to-`document.body` behavior when a focused element becomes `disabled`, so it cannot actually catch a regression here — only a real-browser test can. Follow this repo's e2e conventions (`.claude/rules/e2e-test-conventions.md`): a `@feature` annotation header, `data-testid`/ARIA-role locators only (no CSS class selectors), and no `waitForTimeout` (assert on state via `expect(locator)...`/`waitForSelector` instead).
- Add `tests/e2e/session-actions-steer-focus.spec.ts` with a `// @feature session:update` header. Test shape:
  1. Navigate to a session with the "Give Direction" action available (an autonomous work session — reuse this repo's existing session-creation e2e helpers rather than a new fixture) and open the dialog via its trigger control (`getByRole("button", { name: /give direction/i })` or the option's accessible name).
  2. Type a message into the dialog's textbox (`getByRole("dialog", { name: "Give Direction" }).getByRole("textbox")`).
  3. Intercept/delay the `UpdateSession` RPC (`page.route(...)` on the ConnectRPC endpoint, mirroring the interception pattern `backlog-pipeline-mode.spec.ts`/`backlog-session-steer.spec.ts` already use for this app's RPCs) so it doesn't resolve immediately — the delay is what puts `isSteering` in a real, observably-pending state instead of racing a same-tick assertion against it.
  4. Press Enter to send.
  5. While the RPC is still pending, assert (via `expect(...).toBeFocused()` or equivalent) that the Cancel button — not `document.body` — currently holds focus.
  6. Press `Escape` and assert the dialog closes (`expect(page.getByRole("dialog", { name: "Give Direction" })).toBeHidden()`), proving the Escape handler still receives the event with focus still inside the dialog.
  7. Repeat the send-then-pending setup and instead press `Tab`/`Shift+Tab` a few times, asserting focus stays cycling among the dialog's focusable elements (input, Send, Cancel) and never lands on `document.body` or an element outside the dialog — proving `useFocusTrap`'s cycling isn't broken by the disabled input.
  8. Let the intercepted RPC resolve and assert the dialog's post-resolution state (closed on success) as a sanity check that the interception didn't otherwise change observable behavior.
- This is the acceptance test for the new Story 1.1.3 criterion above — it must fail against the pre-fix code (relying on implicit browser auto-blur) and pass once Task 1.1.3b's explicit `cancelButtonRef.current?.focus()` call is in place; confirm this by temporarily reverting that one line locally and observing the new test fail, before landing it passing.
- Files: `tests/e2e/session-actions-steer-focus.spec.ts`

### Epic 1.2: `SessionSteerer` Interface & DI Wiring
**Goal**: Expose `steerInstance` to `BacklogService` via the same consumer-defined-interface pattern as `SessionStopper`.

#### Story 1.2.1: Define `SessionSteerer` and implement it on `SessionService`
**As a** `BacklogService`, **I want** a minimal interface for reading a session's program and steering it, **so that** I can depend on two methods instead of all of `SessionService`.
**Acceptance Criteria**:
- `SessionSteerer` is defined in `server/services/backlog_service.go` (not `session_service.go`) with exactly the two methods from requirements.md decision #1.
  - *Given* the `server/services` package, *When* `go vet ./server/services` runs, *Then* `*SessionService` satisfies `SessionSteerer` (verified by a compile-time assertion `var _ SessionSteerer = (*SessionService)(nil)`).
- `SessionProgram(sessionUUID)` returns `("", false)` for a UUID with no live instance, and `(instance.Program, true)` for a live one.
  - *Given* a `SessionService` with no instance tracked for UUID `"abc-123"`, *When* `SessionProgram("abc-123")` is called, *Then* it returns `("", false)`.
  - *Given* a live instance with UUID `"abc-123"` and `Program: "claude"`, *When* `SessionProgram("abc-123")` is called, *Then* it returns `("claude", true)`.
- `SteerActiveSession(ctx, sessionUUID, message)` resolves the instance via `s.FindLiveInstance(sessionUUID)` and delegates to `steerInstance`, returning an explicit "not found" error if the instance isn't live.
  - *Given* no live instance for UUID `"missing-uuid"`, *When* `SteerActiveSession(ctx, "missing-uuid", "hello")` is called, *Then* it returns a non-nil error and does not panic.
**Files**: `server/services/backlog_service.go`, `server/services/session_service.go`

##### Task 1.2.1a: Add the `SessionSteerer` interface (~3 min)
- In `server/services/backlog_service.go`, directly below the `SessionStopper` interface block, add:
  ```go
  // SessionSteerer allows BacklogService to inject a message into an already-
  // active session (e.g. a PR-fix problem description) instead of skipping a
  // respawn outright. It is nil-safe: BacklogService degrades gracefully when
  // not wired, mirroring SessionStopper.
  type SessionSteerer interface {
      SessionProgram(sessionUUID string) (program string, ok bool)
      SteerActiveSession(ctx context.Context, sessionUUID, message string) error
  }
  ```
- Files: `server/services/backlog_service.go`

##### Task 1.2.1b: Implement `SessionProgram` on `SessionService` (~3 min)
- Add, near `FindLiveInstance` (`server/services/session_service.go:869`):
  ```go
  // SessionProgram implements SessionSteerer. Returns ok=false if sessionUUID
  // has no live instance tracked.
  func (s *SessionService) SessionProgram(sessionUUID string) (string, bool) {
      inst := s.FindLiveInstance(sessionUUID)
      if inst == nil {
          return "", false
      }
      return inst.Program, true
  }
  ```
- Files: `server/services/session_service.go`

##### Task 1.2.1c: Implement `SteerActiveSession` on `SessionService` (~4 min)
- Add directly below `SessionProgram`:
  ```go
  // SteerActiveSession implements SessionSteerer, delegating to the same
  // steerInstance UpdateSession's SteerMessage handling uses.
  func (s *SessionService) SteerActiveSession(ctx context.Context, sessionUUID, message string) error {
      inst := s.FindLiveInstance(sessionUUID)
      if inst == nil {
          return fmt.Errorf("steer session %q: not tracked live", sessionUUID)
      }
      return s.steerInstance(ctx, inst, message)
  }
  ```
- Files: `server/services/session_service.go`

##### Task 1.2.1d: Add compile-time assertion + build (~3 min)
- Add `var _ SessionSteerer = (*SessionService)(nil)` near the top of `server/services/session_service.go` (alongside any existing similar assertions, or immediately after the imports if none exist).
- Run `go build ./server/...`.
- Files: `server/services/session_service.go`

#### Story 1.2.2: Wire `SetSessionSteerer` into `BacklogService` and `dependencies.go`
**As the** server bootstrap, **I want** `BacklogService` wired with the real `SessionSteerer`, **so that** the reconciler's steer path is live in production.
**Acceptance Criteria**:
- `BacklogService` gets a `sessionSteerer SessionSteerer` field and a `SetSessionSteerer(s SessionSteerer)` setter, mirroring `SetSessionStopper`.
  - *Given* a fresh `*BacklogService` with no `SetSessionSteerer` call, *When* any code path reads `s.sessionSteerer`, *Then* it observes `nil` (the documented nil-safe default).
- `server/dependencies.go` calls `backlogSvc.SetSessionSteerer(sessionService)` immediately alongside the existing `backlogSvc.SetSessionStopper(sessionService)` call.
  - *Given* the server bootstrap in `dependencies.go`, *When* `BuildRuntimeDeps` runs (verified: the actual wiring call site, `server/dependencies.go:1191`, lives inside `BuildRuntimeDeps`, not `BuildCoreDepsWithOptions`), *Then* `backlogSvc.sessionSteerer` is non-nil and equal to `sessionService`.
- **(pre-mortem.md P2 #5)** This wiring is asserted by an actual boot-level test, not `go build` alone — see Task 1.2.2d/e: `TestBuildRuntimeDeps_should_WireSessionSteererAndSessionStopper_When_ServerBoots` fails if `SetSessionSteerer` (or `SetSessionStopper`) is ever omitted or misplaced, closing the "compiles, ships silently inert" gap a nil-safe degrade path would otherwise hide forever.
**Files**: `server/services/backlog_service.go`, `server/dependencies.go`, `server/dependencies_test.go`

##### Task 1.2.2a: Add the field and setter (~3 min)
- In `server/services/backlog_service.go`, add `sessionSteerer SessionSteerer` to the `BacklogService` struct near `sessionStopper`, and add:
  ```go
  // SetSessionSteerer wires the optional session steerer used to inject a
  // PR-fix problem description into an already-active session instead of
  // skipping the respawn outright.
  func (s *BacklogService) SetSessionSteerer(steerer SessionSteerer) {
      s.sessionSteerer = steerer
  }
  ```
  directly below `SetSessionStopper` (`backlog_service.go:465-468`).
- Files: `server/services/backlog_service.go`

##### Task 1.2.2b: Wire it in `dependencies.go` (~2 min)
- In `server/dependencies.go`, directly below `backlogSvc.SetSessionStopper(sessionService)` (line 1191), add `backlogSvc.SetSessionSteerer(sessionService)`.
- Files: `server/dependencies.go`

##### Task 1.2.2c: Build and confirm wiring (~2 min)
- `go build ./server/...` — confirms `*SessionService` satisfies `SessionSteerer` at this call site too (redundant with Task 1.2.1d but exercises the actual wiring line).
- Files: none (verification only)

##### Task 1.2.2d: Add getters + a boot-level wiring-assertion test for `SetSessionSteerer` (and `SetSessionStopper`, same gap) (~8 min)
**(pre-mortem.md P2 #5)**: `SetSessionSteerer`'s wiring in `server/dependencies.go` (Task 1.2.2b, one new line) has no test that fails if it's omitted or misplaced. The nil-safe degrade path (`s.sessionSteerer == nil`) is *by design* externally indistinguishable from today's shipped behavior — falls back to `notifyRespawnBlockedByActiveSession`, unchanged — so a forgotten wiring line compiles, passes `make ci`, and ships the entire feature silently inert. **Precedent check (verified, not assumed):** grepped `server/dependencies_test.go` and `server/services/backlog_service.go` — no existing test asserts `sessionStopper`/`sessionSteerer` non-nil post-boot, and neither field has a getter (`sessionStopper`/`sessionSteerer` are unexported on `*BacklogService`, which lives in a different package — `services` — than `dependencies_test.go`, package `server`). The closest analogous pattern this repo already uses is `SessionService.GetBacklogLifecycleListener()` (`server/services/session_service.go:1259`), consumed by `TestBuildRuntimeDeps_should_PopulateBacklogLifecycleListener_When_ServerBoots` (`server/dependencies_test.go:201`) to assert wiring identity post-boot via `BuildDependencies()`. **This is the first such test for `SessionStopper`/`SessionSteerer` — there is no prior instance to extend, state that plainly rather than implying one.**
- In `server/services/backlog_service.go`, directly below `SetSessionSteerer` (Task 1.2.2a), add two getters whose only purpose is making this wiring testable from `package server` (both fields are otherwise unexported and unreadable outside `services`):
  ```go
  // GetSessionSteerer returns the wired SessionSteerer, or nil if
  // SetSessionSteerer was never called. Exists so server-package tests can
  // assert real bootstrap wiring (dependencies.go) without exposing the
  // field directly — mirrors SessionService.GetBacklogLifecycleListener's
  // wiring-test-support role (pre-mortem.md P2 #5: this is the first such
  // getter for this pattern on BacklogService).
  func (s *BacklogService) GetSessionSteerer() SessionSteerer {
      return s.sessionSteerer
  }

  // GetSessionStopper returns the wired SessionStopper, or nil. Added
  // alongside GetSessionSteerer since SetSessionStopper had the identical
  // untested-wiring gap (pre-mortem.md P2 #5) — fixing both costs one extra
  // assertion in the same test, not a second test file.
  func (s *BacklogService) GetSessionStopper() SessionStopper {
      return s.sessionStopper
  }
  ```
- In `server/dependencies_test.go`, add `TestBuildRuntimeDeps_should_WireSessionSteererAndSessionStopper_When_ServerBoots`, following `TestBuildRuntimeDeps_should_PopulateBacklogLifecycleListener_When_ServerBoots`'s exact shape (confirmed at `server/dependencies_test.go:201`: calls `BuildDependencies()`, asserts non-nil, asserts identity against `deps.SessionService`; `ServerDependencies.BacklogService *services.BacklogService` is a direct, already-exported field, confirmed at `server/dependencies.go:77`):
  ```go
  func TestBuildRuntimeDeps_should_WireSessionSteererAndSessionStopper_When_ServerBoots(t *testing.T) {
      deps, err := BuildDependencies()
      if err != nil {
          t.Fatalf("BuildDependencies: %v", err)
      }
      if deps.BacklogService.GetSessionSteerer() == nil {
          t.Fatal("expected SessionSteerer to be wired onto BacklogService")
      }
      if deps.BacklogService.GetSessionSteerer() != deps.SessionService {
          t.Fatal("expected the wired SessionSteerer to be the same *SessionService instance BuildDependencies constructs")
      }
      if deps.BacklogService.GetSessionStopper() == nil {
          t.Fatal("expected SessionStopper to be wired onto BacklogService")
      }
  }
  ```
- Files: `server/services/backlog_service.go`, `server/dependencies_test.go`

##### Task 1.2.2e: Build and run the new wiring test (~2 min)
- `go build ./... && go test ./server -run TestBuildRuntimeDeps_should_WireSessionSteererAndSessionStopper -v`
- Files: none (verification only)

---

## Phase 2: Reason Signature, Dedup & Conflict Debounce (pure functions)

### Epic 2.1: `reasonSignature` type and builder
**Goal**: A stable, comparable representation of "what's currently wrong with this PR" derived only from `fixContext`'s section headers.

#### Story 2.1.1: `reasonSignature` type + `buildReasonSignature`
**As** `steerActiveSessionForPRFix`, **I want** a comparable value that changes only when the *category* of problem changes, **so that** dedup isn't defeated by volatile CI-log/review-comment text.
**Acceptance Criteria**:
- `buildReasonSignature(fixContext string)` extracts every line starting with `"## "` in order, ignoring all other content.
  - *Given* `fixContext = "## Merge conflict\nRebase onto main.\n\n## Failing CI checks\n- lint\n"`, *When* `buildReasonSignature(fixContext)` is called, *Then* it returns a `reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}`.
- Two `reasonSignature` values built from `fixContext` strings that differ only in body text under identical headers compare equal.
  - *Given* `fixContext1 = "## Failing CI checks\n- lint\n"` and `fixContext2 = "## Failing CI checks\n- unit-tests\n"`, *When* both are passed through `buildReasonSignature` and compared with `.equal(...)`, *Then* the result is `true` (same header, different failing-check name — not a new category).
- A `reasonSignature` exposes `hasHeader(header string) bool` for the conflict-debounce logic in Epic 2.3.
  - *Given* `reasonSignature{headers: []string{"## Merge conflict"}}`, *When* `.hasHeader("## Merge conflict")` is called, *Then* it returns `true`, and `.hasHeader("## Failing CI checks")` returns `false`.
- A header-less `fixContext` (zero `"## "`-prefixed lines) does **not** collapse to an empty, universally-equal signature — `buildReasonSignature` falls back to a single-element signature built from the trimmed full string, so two *different* header-less messages compare unequal while two *identical* ones still dedup correctly. This matters because a real, reachable call site produces exactly this shape: the "PR closed without merging" branch (`session/backlog_lifecycle_pr.go`'s `fixCtx := fmt.Sprintf("PR #%d (%s) was closed without merging...")`) feeds `AutoReopenForPRFix` a plain sentence with no markdown headers at all.
  - *Given* `fixContext1 = "PR #42 (url1) was closed without merging. Investigate why, address any concerns, and open a fresh PR."` and `fixContext2 = "PR #99 (url2) was closed without merging. Investigate why, address any concerns, and open a fresh PR."` (two different, unrelated header-less messages), *When* both are passed through `buildReasonSignature` and compared with `.equal(...)`, *Then* the result is `false`.
  - *Given* `fixContext1` and an identical copy `fixContext1Again` (same header-less string, e.g. the same PR re-observed on a later tick), *When* both are passed through `buildReasonSignature` and compared with `.equal(...)`, *Then* the result is `true`.
**Files**: `server/services/backlog_service_pr_fix_steer.go` (new file), `server/services/backlog_service_pr_fix_steer_test.go` (new file), `session/git/worktree_git.go` (doc comment only, Task 2.1.1d)

##### Task 2.1.1a: Create the file and define `reasonSignature` (~4 min)
- Create `server/services/backlog_service_pr_fix_steer.go` with package `services`, imports (`strings`, `time`, `fmt`, `context`), and:
  ```go
  // reasonSignature is the stable subset of a fixContext string used to detect
  // whether a PR's problem category has meaningfully changed between
  // reconcile ticks. Built only from fixContext's "## <Section>" markdown
  // headers (session/git/worktree_git.go's PRStatus.render), never from the
  // body text under each header — CI check names and reviewer comment bodies
  // are expected to shift between polls without representing a new category
  // of problem.
  type reasonSignature struct {
      headers []string
  }

  func (r reasonSignature) equal(other reasonSignature) bool {
      if len(r.headers) != len(other.headers) {
          return false
      }
      for i := range r.headers {
          if r.headers[i] != other.headers[i] {
              return false
          }
      }
      return true
  }

  func (r reasonSignature) hasHeader(header string) bool {
      for _, h := range r.headers {
          if h == header {
              return true
          }
      }
      return false
  }

  const conflictHeader = "## Merge conflict"
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 2.1.1b: Implement `buildReasonSignature`, with a fallback for header-less `fixContext` (~4 min)
- Add below the type definitions:
  ```go
  // buildReasonSignature extracts fixContext's ordered "## " headers. The
  // exact header strings this depends on are emitted by PRStatus.render()
  // (session/git/worktree_git.go) — that is the source of truth; a wording
  // change there silently changes every dedup signature's identity, which is
  // why TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender (Task
  // 2.1.1d) pins the exact strings.
  //
  // Not every fixContext has headers: the "PR closed without merging" call
  // site (session/backlog_lifecycle_pr.go) builds a plain, header-less
  // sentence. Without a fallback, every such message would produce the same
  // empty signature and compare equal, falsely deduping unrelated header-less
  // steers (e.g. two different closed PRs) as identical. The fallback treats
  // the trimmed full string as a single-element signature instead, so
  // different header-less messages differ while identical ones still dedup.
  func buildReasonSignature(fixContext string) reasonSignature {
      var headers []string
      for _, line := range strings.Split(fixContext, "\n") {
          if strings.HasPrefix(line, "## ") {
              headers = append(headers, line)
          }
      }
      if len(headers) == 0 {
          if trimmed := strings.TrimSpace(fixContext); trimmed != "" {
              headers = []string{trimmed}
          }
      }
      return reasonSignature{headers: headers}
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 2.1.1c: Unit tests for signature building/equality, including the header-less fallback (~7 min)
- Create `server/services/backlog_service_pr_fix_steer_test.go` with `TestBuildReasonSignature_ExtractsHeadersOnly`, `TestReasonSignature_Equal_IgnoresBodyText`, `TestReasonSignature_HasHeader`, covering the three original GWT examples above.
- Add `TestBuildReasonSignature_HeaderlessFixContext_DifferentMessagesProduceDifferentSignatures` and `TestBuildReasonSignature_HeaderlessFixContext_IdenticalMessagesProduceEqualSignatures`, covering the two header-less GWT examples in Story 2.1.1's acceptance criteria (model the two "PR closed without merging" fixtures directly on `session/backlog_lifecycle_pr.go`'s actual `fmt.Sprintf` shape, not a placeholder string).
- Files: `server/services/backlog_service_pr_fix_steer_test.go` (new file)

##### Task 2.1.1d: Pin `buildReasonSignature`'s header strings against `PRStatus.render()` (~5 min)
- Add `TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender` to `server/services/backlog_service_pr_fix_steer_test.go` (or, if package-visibility requires it, alongside `PRStatus.render()`'s own tests in `session/git`): construct a `PRStatus` exercising each of `render()`'s four header-emitting branches (`HasConflicts`, `failedChecks`, `blockingReviews`, `commentReviews`/`generalComments`), call `render()`, run the result through `buildReasonSignature`, and assert the exact resulting header strings (`"## Merge conflict"`, `"## Failing CI checks"`, the dynamic `"## Review: changes requested by @<author>"`, `"## Reviewer comments"`, `"## PR comments"`) — so a future wording change to `render()` fails this test loudly instead of silently changing dedup identity.
- Add a one-line doc comment on `PRStatus.render()` itself (`session/git/worktree_git.go:565`) cross-referencing `buildReasonSignature` as a dependent on its exact header text (this is an implementation-time source-code touch, not a plan-only change, but is called out here so the task isn't dropped).
- Files: `server/services/backlog_service_pr_fix_steer_test.go`, `session/git/worktree_git.go` (doc comment only)

### Epic 2.2: Cooldown-Based Dedup
**Goal**: Suppress an exact-repeat `reasonSignature` steer within `steerCooldown` of its last delivery.

#### Story 2.2.1: `isDuplicateSteerReason` / `nextLastSteerReason` / `steerCooldown`
**As** `steerActiveSessionForPRFix`, **I want** to skip re-steering with an identical, recently-delivered reason, **so that** the active session isn't spammed every ~60s reconcile tick.
**Acceptance Criteria**:
- `isDuplicateSteerReason` returns `false` for a zero-value `lastSteerReason` (never delivered before).
  - *Given* `last := lastSteerReason{}` (zero value) and `candidate := reasonSignature{headers: []string{"## Failing CI checks"}}`, *When* `isDuplicateSteerReason(candidate, last, "uuid-1", time.Now(), steerCooldown)` is called, *Then* it returns `false`.
- `isDuplicateSteerReason` returns `true` for an identical signature delivered to the *same session* within `steerCooldown`, and `false` once `steerCooldown` has elapsed.
  - *Given* `last := lastSteerReason{signature: reasonSignature{headers: []string{"## Failing CI checks"}}, at: now.Add(-1*time.Minute), sessionUUID: "uuid-1"}` and `candidate` with the same headers, *When* `isDuplicateSteerReason(candidate, last, "uuid-1", now, 5*time.Minute)` is called, *Then* it returns `true`; *When* called with `now.Add(6*time.Minute)` instead, *Then* it returns `false`.
- `isDuplicateSteerReason` returns `false` regardless of signature/cooldown when the caller's `sessionUUID` doesn't match `last.sessionUUID` — a session change is treated as "never delivered," bypassing cooldown entirely (architecture review concern: the active work session can change between ticks, e.g. a human manually starts a replacement session).
  - *Given* the same `last` as above (`sessionUUID: "uuid-1"`, delivered 1 minute ago, identical signature to `candidate`), *When* `isDuplicateSteerReason(candidate, last, "uuid-2", now, 5*time.Minute)` is called (a *different* session UUID), *Then* it returns `false` even though the signature and timing would otherwise be a duplicate.
- `nextLastSteerReason` only advances state when `delivered=true`, and always records the delivering session's UUID.
  - *Given* `prev := lastSteerReason{}` and `candidate := reasonSignature{headers: []string{"## Failing CI checks"}}`, *When* `nextLastSteerReason(prev, candidate, "uuid-1", false)` is called, *Then* the result equals `prev` unchanged; *When* called with `delivered=true`, *Then* the result's `signature` equals `candidate`, `sessionUUID` equals `"uuid-1"`, and `at` is set to (approximately) now.
**Files**: `server/services/backlog_service_pr_fix_steer.go`, `server/services/backlog_service_pr_fix_steer_test.go`

##### Task 2.2.1a: Define `lastSteerReason`, `steerCooldown`, `isDuplicateSteerReason` (~6 min)
- Add to `server/services/backlog_service_pr_fix_steer.go`:
  ```go
  // lastSteerReason is per-item dedup state, mirroring session/nudge_dedup.go's
  // lastNudge shape, plus sessionUUID — the session that actually received
  // this delivery. Stored in BacklogService.steerDedup (sync.Map, key=itemID).
  // sessionUUID exists so a change of active work session between ticks (a
  // human manually starting a replacement session, say) is never mistaken for
  // an already-delivered reason to a session that never actually received it
  // (architecture review concern).
  type lastSteerReason struct {
      signature   reasonSignature
      at          time.Time
      sessionUUID string
  }

  // steerCooldown bounds how long an exact-repeat PR-fix-steer reason stays
  // suppressed once delivered. 5 minutes: above the reconcile ticker's own
  // 60s cadence (server/dependencies.go) so a same-reason retry isn't fired
  // every single tick, below maxReworkBlockStaleness (15min, a different
  // "whole session is stalled" signal) so a genuinely dropped/lost steer
  // retries within one work session's lifetime.
  var steerCooldown = 5 * time.Minute

  // isDuplicateSteerReason treats a sessionUUID mismatch against last.sessionUUID
  // the same as "never delivered" — bypassing cooldown regardless of signature
  // equality — because a reason "delivered" to a now-dead prior session was
  // never actually seen by the new one.
  func isDuplicateSteerReason(candidate reasonSignature, last lastSteerReason, sessionUUID string, now time.Time, cooldown time.Duration) bool {
      if last.at.IsZero() {
          return false
      }
      if last.sessionUUID != sessionUUID {
          return false
      }
      if now.Sub(last.at) > cooldown {
          return false
      }
      return candidate.equal(last.signature)
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 2.2.1b: Define `nextLastSteerReason` (~2 min)
- Add:
  ```go
  func nextLastSteerReason(prev lastSteerReason, candidate reasonSignature, sessionUUID string, delivered bool) lastSteerReason {
      if !delivered {
          return prev
      }
      return lastSteerReason{signature: candidate, at: time.Now(), sessionUUID: sessionUUID}
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 2.2.1c: Unit tests, including the session-changed bypass (~7 min)
- Add `TestIsDuplicateSteerReason_ZeroValue_NotDuplicate`, `TestIsDuplicateSteerReason_WithinCooldown_IsDuplicate`, `TestIsDuplicateSteerReason_AfterCooldown_NotDuplicate`, `TestIsDuplicateSteerReason_SessionUUIDChanged_NotDuplicate_EvenWithinCooldown`, `TestNextLastSteerReason_NotDelivered_Unchanged`, `TestNextLastSteerReason_Delivered_AdvancesAndRecordsSessionUUID` to the test file, covering the GWT examples exactly (use an injectable `now` rather than real `time.Now()` inside the function bodies under test where the acceptance criteria require exact boundary behavior — `isDuplicateSteerReason` already takes `now` as a parameter for this reason).
- Files: `server/services/backlog_service_pr_fix_steer_test.go`

### Epic 2.3: Conflict Two-Consecutive-Tick Debounce
**Goal**: A newly-appearing `## Merge conflict` header must be observed on two consecutive reconcile ticks before it's allowed to trigger a steer, per pitfalls research §6 (GitHub `mergeStateStatus` staleness, cli/cli#9583). **Known, accepted limitation (pre-mortem.md #4, P3):** this debounce only guards against GitHub *over*-reporting a conflict (a stale-but-clearing status); it has no equivalent guard against *under*-reporting (a real conflict/CI failure GitHub hasn't reflected yet), which could leave a broken PR with an active session un-steered and with no stuck-reason row at all. Closing that asymmetry is out of this project's scope — a future watchdog (an item stuck in `pr_pending` with an active session for N ticks with zero `CIFailing`/`HasBlockingReviews`/`HasConflicts` ever observed true, yet never reaching merged/closed) is a candidate follow-up, not part of this plan.

#### Story 2.3.1: `conflictDebounceState` + `confirmConflictChange`
**As** `steerActiveSessionForPRFix`, **I want** a newly-appearing conflict signal held for one extra tick before acting on it, **so that** I don't steer an agent about a conflict it may have already resolved with its own push between polls.
**Acceptance Criteria**:
- A conflict header appearing for the first time (not present in the last *delivered* signature) is **not** confirmed on its first observed tick; the state records it as pending. "Nothing pending" is type-enforced (`state.pending == nil`), not a two-field convention — a non-nil `pending` is always fully populated by construction, so a partially-set state (e.g. a signature recorded with a zero `since`) is unrepresentable.
  - *Given* `last := reasonSignature{headers: []string{"## Failing CI checks"}}` (no conflict previously delivered), `sessionUUID := "uuid-1"`, and `state := conflictDebounceState{}` (`pending == nil`, nothing pending), and `candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}`, *When* `confirmConflictChange(candidate, last, sessionUUID, state)` is called, *Then* it returns `(false, next)` where `next.pending != nil` and `next.pending.signature.equal(candidate)` is `true`.
- The same conflict signature observed on the next tick, for the *same session*, **is** confirmed, and the state clears.
  - *Given* the `next` state from the previous example, the same `sessionUUID := "uuid-1"`, and the same `candidate` observed again, *When* `confirmConflictChange(candidate, last, sessionUUID, next)` is called, *Then* it returns `(true, conflictDebounceState{})` (confirmed, `pending == nil` again).
- A tick where the conflict disappears clears any pending state (restart-from-scratch on reintroduction).
  - *Given* a pending `state` (`state.pending != nil`) from a prior conflict observation, and `candidate := reasonSignature{headers: []string{"## Failing CI checks"}}` (no conflict header this tick), *When* `confirmConflictChange(candidate, last, sessionUUID, state)` is called, *Then* it returns `(false, conflictDebounceState{})` (`pending == nil`).
- A `reasonSignature` change that does **not** newly introduce a conflict header (e.g. `CIFailing` newly appears while conflict state is unchanged from `last`) bypasses the debounce entirely — `confirmConflictChange` is only consulted by the caller when the conflict header itself is newly appearing relative to `last`.
  - *Given* `last := reasonSignature{headers: []string{"## Merge conflict"}}` (conflict already present last delivery) and `candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}` (conflict unchanged, CI newly failing), *When* the caller (Story 4.2.1) checks whether conflict is *newly* appearing via `candidate.hasHeader(conflictHeader) && !last.hasHeader(conflictHeader)`, *Then* the condition is `false`, so `confirmConflictChange` is never called for this tick and the steer proceeds immediately (only the cooldown/dedup check from Epic 2.2 applies).
- A pending confirmation recorded for one session does **not** confirm on a second consecutive tick observed for a *different* session — the active work session having changed between ticks restarts confirmation from scratch, exactly as if the conflict were newly appearing (architecture review concern, same treatment as `isDuplicateSteerReason`'s session-changed bypass).
  - *Given* `state.pending` was recorded with `sessionUUID: "uuid-1"` on the prior tick, and the same `candidate` is now observed with `sessionUUID: "uuid-2"` (the active work session changed), *When* `confirmConflictChange(candidate, last, "uuid-2", state)` is called, *Then* it returns `(false, next)` where `next.pending.sessionUUID == "uuid-2"` — treated as a fresh first observation, not a confirmation.
**Files**: `server/services/backlog_service_pr_fix_steer.go`, `server/services/backlog_service_pr_fix_steer_test.go`

##### Task 2.3.1a: Define `conflictDebounceState` and `confirmConflictChange` (~6 min)
- Add:
  ```go
  // pendingConflict is always fully populated when it exists — see
  // conflictDebounceState's doc comment for why it's a pointer, not a
  // two-field struct. sessionUUID records which active work session the
  // pending observation was made against, mirroring lastSteerReason's
  // session-changed handling.
  type pendingConflict struct {
      signature   reasonSignature
      since       time.Time
      sessionUUID string
  }

  // conflictDebounceState tracks, per item, whether a newly-observed conflict
  // header is awaiting its second consecutive confirming tick before it may
  // trigger a steer. pending == nil unambiguously means "nothing pending" —
  // a type-enforced state, not a two-field convention: a plain
  // {pendingSignature reasonSignature, pendingSince time.Time} struct would
  // let a signature be recorded with a zero pendingSince (or vice versa), a
  // state no code path is meant to produce but nothing would prevent
  // (architecture review finding). A non-nil *pendingConflict is always
  // fully populated by construction.
  type conflictDebounceState struct {
      pending *pendingConflict
  }

  // confirmConflictChange implements the two-consecutive-tick confirmation
  // for a newly-appearing "## Merge conflict" header (pitfalls research §6:
  // GitHub's mergeStateStatus is known-stale, cli/cli#9583). Callers must only
  // invoke this when candidate.hasHeader(conflictHeader) &&
  // !last.hasHeader(conflictHeader) — i.e. the conflict is new relative to the
  // last *delivered* signature. Any tick without the conflict header, or a
  // tick whose active session differs from the pending observation's
  // sessionUUID (architecture review concern), resets pending state so
  // confirmation restarts from scratch.
  func confirmConflictChange(candidate, last reasonSignature, sessionUUID string, state conflictDebounceState) (confirmed bool, next conflictDebounceState) {
      if !candidate.hasHeader(conflictHeader) {
          return false, conflictDebounceState{}
      }
      if state.pending != nil && state.pending.sessionUUID == sessionUUID && state.pending.signature.equal(candidate) {
          return true, conflictDebounceState{}
      }
      return false, conflictDebounceState{pending: &pendingConflict{signature: candidate, since: time.Now(), sessionUUID: sessionUUID}}
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 2.3.1b: Unit tests for the 5 GWT scenarios (~6 min)
- Add `TestConfirmConflictChange_FirstTick_NotConfirmed`, `TestConfirmConflictChange_SecondConsecutiveTick_Confirmed`, `TestConfirmConflictChange_ConflictResolved_ClearsPending`, `TestConfirmConflictChange_SessionUUIDChanged_RestartsConfirmation`, plus a doc-only note (not a test, since it's a caller-side condition) referencing the fourth GWT in the story's acceptance criteria comment.
- Files: `server/services/backlog_service_pr_fix_steer_test.go`

---

## Phase 3: Steer Message Construction

### Epic 3.1: Program-Gated Message with Truncation
**Goal**: One pure function producing the final PTY-bound message: `fixContext`, optionally suffixed with the slash-command instruction, truncated to fit `session.MaxSteerMessageLength`.

#### Story 3.1.1: `buildSteerMessage(program, fixContext string) string`
**As** `steerActiveSessionForPRFix`, **I want** a single function that applies program-gating and truncation, **so that** neither rule can be applied inconsistently or forgotten at the call site.
**Acceptance Criteria**:
- For `program == "claude"` (or `""`, per ADR-001), the suffix `"\n\nRun /github:pr-ship to address this."` is appended to `fixContext`, provided the result still fits within `session.MaxSteerMessageLength`.
  - *Given* `program = "claude"` and `fixContext = "## Failing CI checks\n- lint\n"` (well under the limit), *When* `buildSteerMessage(program, fixContext)` is called, *Then* the result equals `fixContext + "\n\nRun /github:pr-ship to address this."`.
- For any other `program` value (e.g. `"aider"`, `"proxy-claude"` per ADR-001), no suffix is appended — the plain `fixContext` is returned (subject to truncation).
  - *Given* `program = "aider"` and the same `fixContext`, *When* `buildSteerMessage(program, fixContext)` is called, *Then* the result equals `fixContext` unchanged, with no slash-command text.
- When `fixContext` alone would exceed `session.MaxSteerMessageLength` once the suffix and truncation pointer are accounted for, `fixContext` is truncated with a trailing pointer — but the suffix is appended *after* truncation, never truncated away itself.
  - *Given* `fixContext` is a string of `session.MaxSteerMessageLength + 500` bytes and `program = "claude"`, *When* `buildSteerMessage(program, fixContext)` is called, *Then* the result's length is `<= session.MaxSteerMessageLength`, and the result contains the literal string `"...[truncated — see item notes for full context]"` followed by the `/github:pr-ship` suffix.
- **(pre-mortem.md P2 #3)** A realistic *composed* long `fixContext` — e.g. multiple failing CI checks plus one substantive review comment body, mirroring `PRStatus.render()`'s actual output shape, not a synthetic oversized string in isolation — must still preserve the suffix. This closes a real bug in an earlier draft of this function: appending the suffix *before* truncating and then cutting from the tail silently dropped exactly the one actionable instruction the steered agent needs, while the non-actionable diagnostic dump survived.
  - *Given* `program = "claude"` and a `fixContext` built from several `## Failing CI checks` entries plus a `## Review: changes requested by @author` section whose body alone is ~2000 bytes (combined, pushing the naive `fixContext + suffix` well past `session.MaxSteerMessageLength`), *When* `buildSteerMessage(program, fixContext)` is called, *Then* the result's length is `<= session.MaxSteerMessageLength` **and** the result ends with the literal suffix `"\n\nRun /github:pr-ship to address this."` — never with the truncation pointer alone.
**Files**: `server/services/backlog_service_pr_fix_steer.go`, `server/services/backlog_service_pr_fix_steer_test.go`

##### Task 3.1.1a: Implement the program-gating half (~4 min)
- Add:
  ```go
  const prShipSuffix = "\n\nRun /github:pr-ship to address this."
  const truncationPointer = "...[truncated — see item notes for full context]"

  // isClaudeCodeProgram mirrors server/workflows/scheduler.go:385's exact-match
  // idiom (ADR-001) — not ClaudeAdapter.CanHandle's substring match, which
  // solves a different problem (transcript-format adapter selection) and
  // would false-positive on values like "proxy-claude".
  func isClaudeCodeProgram(program string) bool {
      return program == "" || program == "claude"
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 3.1.1b: Implement `buildSteerMessage`, reserving room for the suffix BEFORE truncating (~6 min)
- Add:
  ```go
  // buildSteerMessage produces the final PTY-bound steer message: fixContext,
  // optionally suffixed with the Claude-Code-specific fix instruction, always
  // fitting within session.MaxSteerMessageLength.
  //
  // The suffix is appended AFTER truncation, not before (pre-mortem.md P2
  // #3): an earlier version built msg := fixContext+suffix and truncated
  // msg from the tail, which silently dropped the suffix — the one
  // actionable instruction — on any fixContext long enough to need
  // truncation, since the suffix sat at the very end of what got cut. A
  // realistic composed fixContext (a couple of failing checks plus one
  // substantive review comment body — PRStatus.render() appends reviewer
  // comment bodies verbatim with no cap) reaches this case routinely, not
  // just in a synthetic oversized-string test.
  func buildSteerMessage(program, fixContext string) string {
      suffix := ""
      if isClaudeCodeProgram(program) {
          suffix = prShipSuffix
      }
      body := fixContext
      // Reserve room for suffix + truncationPointer against fixContext
      // alone, so both always fit even when fixContext itself must be cut.
      budget := session.MaxSteerMessageLength - len(suffix) - len(truncationPointer)
      if budget < 0 {
          budget = 0
      }
      if len(body) > budget {
          body = body[:budget] + truncationPointer
      }
      return body + suffix
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 3.1.1c: Unit tests for the 4 GWT scenarios + boundary + realistic composed message (~8 min)
- Add `TestBuildSteerMessage_ClaudeProgram_AppendsSuffix`, `TestBuildSteerMessage_NonClaudeProgram_NoSuffix`, `TestBuildSteerMessage_OverLength_Truncates`, and `TestBuildSteerMessage_TruncatedResult_NeverExceedsMaxLength` (assert `len(result) <= session.MaxSteerMessageLength` as an explicit invariant check, not just the truncation-marker presence).
- Add `TestBuildSteerMessage_RealisticComposedLongFixContext_SuffixSurvivesTruncation` (pre-mortem.md P2 #3): build `fixContext` from several `## Failing CI checks` entries plus a `## Review: changes requested by @author` section with a multi-hundred-byte body — mirroring `PRStatus.render()`'s actual combined shape, not an isolated synthetic long string — call `buildSteerMessage("claude", fixContext)`, and assert both `len(result) <= session.MaxSteerMessageLength` and `strings.HasSuffix(result, prShipSuffix)`.
- Files: `server/services/backlog_service_pr_fix_steer_test.go`

---

## Phase 4: Integration into `AutoReopenForPRFix`

### Epic 4.1: `BacklogService` Fields & Setter (dedup/debounce/in-flight state)
**Goal**: Declare the new per-item state maps on `BacklogService`, following `spawnInFlight`'s existing placement/style.

#### Story 4.1.1: Add `steerDedup`, `steerConflictDebounce`, `steerInFlight` fields
**As** `BacklogService`, **I want** three `sync.Map` fields for dedup, conflict-debounce, and in-flight guarding, **so that** `steerActiveSessionForPRFix` has somewhere to keep this state across reconcile ticks.
**Acceptance Criteria**:
- A fresh `*BacklogService` (from `NewBacklogService`) has all three maps ready to use with zero explicit initialization (Go's zero-value `sync.Map` is directly usable).
  - *Given* a freshly constructed `*BacklogService`, *When* `s.steerDedup.Load("some-item-id")` is called before any `Store`, *Then* it returns `(nil, false)` without panicking.
**Files**: `server/services/backlog_service.go`

##### Task 4.1.1a: Add the three fields with doc comments (~4 min)
- In `server/services/backlog_service.go`, directly below the existing `spawnInFlight sync.Map` field, add:
  ```go
  // steerDedup maps itemID -> lastSteerReason, the most recently delivered
  // PR-fix steer reason signature, when, and which session received it —
  // suppresses an exact-repeat steer within steerCooldown, but only for the
  // same session (a changed active work session is always treated as never
  // delivered — architecture review concern). In-memory only (see plan.md's
  // Pattern Decisions: bounded blast radius, no DB durability needed).
  steerDedup sync.Map
  // steerConflictDebounce maps itemID -> conflictDebounceState — the
  // two-consecutive-tick confirmation gate for a newly-appearing merge
  // conflict signal (pitfalls research §6, cli/cli#9583), also keyed by
  // session identity via conflictDebounceState's own pendingConflict.sessionUUID.
  steerConflictDebounce sync.Map
  // steerInFlight maps itemID -> struct{}, guarding steerActiveSessionForPRFix
  // against two overlapping reconcile ticks racing to steer the same item
  // before steerDedup is updated. Mirrors spawnInFlight's self-cleaning
  // LoadOrStore/defer-Delete idiom.
  steerInFlight sync.Map
  ```
- Files: `server/services/backlog_service.go`

### Epic 4.2: `steerActiveSessionForPRFix` and the `AutoReopenForPRFix` Call-Site Swap
**Goal**: Replace the unconditional skip in `AutoReopenForPRFix` with the full steer-or-degrade decision, adding zero `TransitionBacklogItemStatus` calls (regression guard for the double-transition-churn incident).

#### Story 4.2.1: Implement `steerActiveSessionForPRFix`
**As** `AutoReopenForPRFix`, **I want** one helper that tries to steer the active session and degrades to the existing notify-only behavior whenever it safely can't, **so that** the reconciler never regresses to silence and never double-transitions the item's status.
**Acceptance Criteria**:
- When `s.sessionSteerer` is `nil`, the helper calls the existing (unchanged) `notifyRespawnBlockedByActiveSession` and returns, with no panic and no new notification type.
  - *Given* a `*BacklogService` with `sessionSteerer == nil`, *When* `steerActiveSessionForPRFix(ctx, itemID, title, status, activeUUID, fixContext)` is called, *Then* `notifyRespawnBlockedByActiveSession` is invoked exactly once with the same arguments `AutoReopenForPRFix`'s old call site used, and no `StuckReasonSteerFailed` row is created.
- When `s.sessionSteerer.SessionProgram(activeUUID)` returns `ok=false` (session not live per the steerer's own bookkeeping), the helper degrades the same way.
  - *Given* a non-nil `sessionSteerer` whose `SessionProgram("uuid-1")` returns `("", false)`, *When* the helper runs for `activeSessionUUID="uuid-1"`, *Then* it falls back to `notifyRespawnBlockedByActiveSession` and does not call `SteerActiveSession`.
- When the computed `reasonSignature` is a duplicate of `steerDedup`'s stored value within `steerCooldown` *for the same session* (and, for a newly-appearing conflict, `confirmConflictChange` has not yet confirmed it for that session), the helper suppresses the steer and falls back to the notify-only path with the same reason ("dedup suppressed"), leaving `steerDedup`'s stored state unchanged so the next tick can retry.
  - *Given* `steerDedup` holding `lastSteerReason{signature: sigA, at: now.Add(-1*time.Minute), sessionUUID: "uuid-1"}` for `itemID`, and a candidate `fixContext` whose `buildReasonSignature` equals `sigA`, observed for `activeSessionUUID="uuid-1"` (unchanged), *When* the helper runs with `now` unchanged, *Then* `SteerActiveSession` is never called, `notifyRespawnBlockedByActiveSession` is called once, and `steerDedup.Load(itemID)` afterward still returns the original `at` timestamp (unchanged).
- When the item's active work session has changed since the last delivery (`steerDedup`'s stored `sessionUUID` differs from the current `activeSessionUUID`), the helper treats the candidate as never-delivered and steers immediately, bypassing cooldown/debounce entirely regardless of `reasonSignature` equality — a reason "delivered" to a now-dead prior session was never actually seen by the new one (architecture review concern).
  - *Given* `steerDedup` holding `lastSteerReason{signature: sigA, at: now, sessionUUID: "uuid-1"}` for `itemID` (delivered moments ago — well within `steerCooldown`), and the helper now runs for the *same item* but `activeSessionUUID="uuid-2"` (a human manually started a replacement work session) with a candidate `fixContext` whose `buildReasonSignature` equals `sigA` (identical reason), *When* the helper runs, *Then* `SteerActiveSession` is called (not suppressed) and, on success, `steerDedup.Load(itemID)` afterward returns a `lastSteerReason` with `sessionUUID: "uuid-2"`.
- Otherwise, the helper builds the message via `buildSteerMessage(program, fixContext)`, calls `s.sessionSteerer.SteerActiveSession(ctx, activeUUID, message)` guarded by `steerInFlight`, and on success updates `steerDedup` (via `nextLastSteerReason`, recording `activeSessionUUID`) and calls `notifyActiveSessionSteered(..., deliverErr=nil)`; on failure it calls `notifyActiveSessionSteered(..., deliverErr=err)` and leaves `steerDedup` unchanged.
  - *Given* a non-nil `sessionSteerer` with `SessionProgram` returning `("claude", true)` and `SteerActiveSession` returning `nil`, and no prior `steerDedup` entry for `itemID`, *When* the helper runs, *Then* `steerDedup.Load(itemID)` afterward returns a `lastSteerReason` whose `signature` equals the candidate, `sessionUUID` equals `activeSessionUUID`, and `notifyActiveSessionSteered` is called with `deliverErr=nil`.
  - *Given* the same setup but `SteerActiveSession` returns a non-nil error, *When* the helper runs, *Then* `steerDedup.Load(itemID)` afterward is unchanged from before the call, and `notifyActiveSessionSteered` is called with the non-nil `deliverErr`.
- `steerInFlight` guards this method's **entire body** — every degrade branch (nil-safe, not-live, dedup/debounce suppression) as well as the delivery path — not just the call to `SteerActiveSession`. **(pre-mortem.md P2 #2, upgraded from a rare synthetic race to a routine one):** `reconcilePRPendingItem` (`session/backlog_lifecycle_pr.go:1240`) is the shared body for both the 60s-tick loop (`ReconcilePRPending`) and `TriggerPRFixForEvent` (`:1599`). `TriggerPRFixForEvent` itself is invoked synchronously (in-line, awaited) from the GitHub webhook handler (`server/services/github_webhook_pr_fix.go:302`) on every `check_run`/`workflow_run`/`pull_request_review`/`issue_comment` event (shipped in PR #628) — but, verified against the current source, it dispatches the actual `reconcilePRPendingItem` call onto a background goroutine (`go func() { l.reconcilePRPendingItem(...) }()`, `session/backlog_lifecycle_pr.go:1609-1611`) rather than running it inline with the HTTP response; the doc comment there explains why (GitHub's ~10s webhook delivery timeout vs. `reconcilePRPendingItem`'s multiple sequential GitHub API calls and possible session spawn). It's that dispatched goroutine — running concurrently with the ticker's own goroutine, not the HTTP handler's request goroutine — that creates the race. A CI-churning PR — this feature's own target population — will routinely have a webhook delivery's dispatched goroutine land mid-tick, not as a rare timing coincidence. Guarding only the delivery step would leave the degrade branches' `resolveSteerFailedLogged`/`notifyRespawnBlockedByActiveSession` calls unguarded, letting two overlapping calls race to open/resolve `StuckReasonSteerFailed`/`StuckReasonRespawnBlockedActive` rows out of order — `BlockerChip`'s single-chip collapse could then flicker between the two non-deterministically.
  - *Given* two goroutines both calling `steerActiveSessionForPRFix` for the same `itemID` at (approximately) the same time — one shaped like a `ReconcilePRPending` tick call, the other shaped like a `TriggerPRFixForEvent` webhook-delivery call (per `session/backlog_lifecycle_pr.go:1599`'s actual call shape into this same reconcile logic), *When* both run, *Then* exactly one reaches any of this method's branches (degrade or delivery) and the other returns immediately without calling `s.sessionSteerer.SteerActiveSession`, `resolveSteerFailedLogged`, or `notifyRespawnBlockedByActiveSession` a second time — mirroring `spawnInFlight`'s `LoadOrStore`/`defer Delete` guard, but wrapping the whole function instead of one branch.
- No branch of this helper calls `s.storage.TransitionBacklogItemStatus` (regression guard for the double-transition-churn incident at `backlog_service_triage.go:2037-2046`).
  - *Given* any of the above scenarios, *When* the helper completes, *Then* a test double for `storage.TransitionBacklogItemStatus` (or an equivalent call-recording fake) records zero invocations attributable to this helper.
**Files**: `server/services/backlog_service_pr_fix_steer.go`, `server/services/backlog_service_triage.go`

##### Task 4.2.1a: Implement the `steerInFlight` guard (wrapping the WHOLE method) + nil-safe / not-live degrade checks (~6 min)
- In `server/services/backlog_service_pr_fix_steer.go`, add:
  ```go
  // steerActiveSessionForPRFix is AutoReopenForPRFix's active-session branch:
  // steer the already-running session with fixContext instead of skipping the
  // respawn outright, falling back to the existing notify-only behavior
  // whenever it can't safely do so. Never calls TransitionBacklogItemStatus —
  // see backlog_service_triage.go:2037-2046's double-transition-churn incident.
  func (s *BacklogService) steerActiveSessionForPRFix(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID, fixContext string) {
      // steerInFlight guards this method's ENTIRE body, not just the
      // delivery call below — including every degrade branch's
      // resolveSteerFailedLogged/notifyRespawnBlockedByActiveSession calls
      // (pre-mortem.md P2 #2). reconcilePRPendingItem is the shared body for
      // both the 60s-tick loop (ReconcilePRPending) AND TriggerPRFixForEvent
      // (session/backlog_lifecycle_pr.go:1599). TriggerPRFixForEvent is
      // itself invoked synchronously from the GitHub webhook handler
      // (server/services/github_webhook_pr_fix.go:302) on every
      // check_run/workflow_run/pull_request_review/issue_comment event (PR
      // #628), but dispatches reconcilePRPendingItem onto a background
      // goroutine rather than running it inline — it's that dispatched
      // goroutine, concurrent with the ticker, that actually races. A
      // CI-churning PR (this feature's target population) will routinely
      // have a webhook-dispatched goroutine land mid-tick, not as a rare
      // synthetic race. Guarding only delivery would leave the
      // degrade branches racing unguarded, letting two overlapping calls
      // open/resolve StuckReasonSteerFailed/StuckReasonRespawnBlockedActive
      // out of order. Mirrors spawnInFlight's LoadOrStore/defer-Delete idiom.
      if _, already := s.steerInFlight.LoadOrStore(itemID, struct{}{}); already {
          return
      }
      defer s.steerInFlight.Delete(itemID)

      if s.sessionSteerer == nil {
          // A degrade path reaffirms "blocked by active session," which is
          // never simultaneously a failed-steer condition — resolve any stale
          // SteerFailed row from an earlier tick (Story 4.3.2's invariant).
          s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
          s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, itemTitle, currentStatus, activeSessionUUID)
          return
      }
      program, ok := s.sessionSteerer.SessionProgram(activeSessionUUID)
      if !ok {
          s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
          s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, itemTitle, currentStatus, activeSessionUUID)
          return
      }
      // dedup/debounce/delivery: Task 4.2.1b
      _ = program
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 4.2.1b: Implement dedup + conflict-debounce gating, keyed by session identity (~6 min)
- Extend the method body (replacing the `_ = program` placeholder). `activeSessionUUID` is threaded into both `confirmConflictChange` and `isDuplicateSteerReason` so a changed active work session is always treated as "never delivered" (architecture review concern), never as a dedup match:
  ```go
  candidate := buildReasonSignature(fixContext)

  lastVal, _ := s.steerDedup.Load(itemID)
  last, _ := lastVal.(lastSteerReason)

  newlyConflict := candidate.hasHeader(conflictHeader) && !last.signature.hasHeader(conflictHeader)
  if newlyConflict {
      debounceVal, _ := s.steerConflictDebounce.Load(itemID)
      debounceState, _ := debounceVal.(conflictDebounceState)
      confirmed, next := confirmConflictChange(candidate, last.signature, activeSessionUUID, debounceState)
      s.steerConflictDebounce.Store(itemID, next)
      if !confirmed {
          s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
          s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, itemTitle, currentStatus, activeSessionUUID)
          return
      }
  } else {
      s.steerConflictDebounce.Delete(itemID)
  }

  if isDuplicateSteerReason(candidate, last, activeSessionUUID, time.Now(), steerCooldown) {
      s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
      s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, itemTitle, currentStatus, activeSessionUUID)
      return
  }
  ```
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 4.2.1c: Implement the delivery + outcome handling (~4 min)
- The `steerInFlight` guard already wraps this call (Task 4.2.1a) — do not add a second guard here. Append:
  ```go
  message := buildSteerMessage(program, fixContext)
  deliverErr := s.sessionSteerer.SteerActiveSession(ctx, activeSessionUUID, message)
  s.steerDedup.Store(itemID, nextLastSteerReason(last, candidate, activeSessionUUID, deliverErr == nil))
  s.notifyActiveSessionSteered(ctx, itemID, itemTitle, currentStatus, activeSessionUUID, message, program, candidate, deliverErr)
  ```
  Note: `notifyActiveSessionSteered`'s signature (Story 4.3.2) takes `currentStatus session.BacklogStatus` as its 4th parameter — needed by the failure branch's `MarkStuck` call — so this call site passes the `currentStatus` already in scope from `steerActiveSessionForPRFix`'s own parameters, not a placeholder. It also takes `program` (already computed at Task 4.2.1a's `s.sessionSteerer.SessionProgram` call, in scope here) as its 7th parameter and the `candidate reasonSignature` already computed at the top of this method (Task 4.2.1b) as its 8th, immediately before `deliverErr` — `program` is what lets the failure branch's remediation-path suffix reuse `isClaudeCodeProgram` (UX triad review, Task 4.3.2b) and `candidate` is what lets the notification title/body name the actual reason category (P1 fix) instead of a generic phrase; neither needs new computation, both are already in scope.
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 4.2.1d: Replace the call site in `AutoReopenForPRFix` (~3 min)
- In `server/services/backlog_service_triage.go`, replace:
  ```go
  if active := findActiveWorkSession(sessions); active != nil {
      s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
      return nil
  }
  ```
  with:
  ```go
  if active := findActiveWorkSession(sessions); active != nil {
      s.steerActiveSessionForPRFix(ctx, itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID, fixContext)
      return nil
  }
  ```
- Files: `server/services/backlog_service_triage.go`

##### Task 4.2.1e: Build (~2 min)
- `go build ./server/...`
- Files: none (verification only)

#### Story 4.2.2: Bypass `remediatePRFixWithBackoffGate`'s backoff gate for the steer path
**As** `AutoReopenForPRFix`'s active-session branch, **I want** to be reached and evaluated on every reconcile tick regardless of `RemediationDue`'s due/not-due state, **so that** Phase 2/3's reason-signature dedup/cooldown/debounce machinery (tuned for the ~60s reconcile-tick cadence) actually gets to run at that cadence in production, instead of only once per 30m→2h→8h→24h→72h backoff window — this closes pre-mortem.md's P1 finding, without which Success Metric #2 ("re-steer on reason change") would be effectively unreachable for up to 72h after the first steer.

**Root cause (verified against current source, not assumed):** `AutoReopenForPRFix`'s entire call — both the spawn branch and, after Story 4.2.1, the new steer branch — is invoked *exclusively* from `remediatePRFixWithBackoffGate` (`session/backlog_lifecycle_pr.go:1182-1229`), which gates the call behind `l.storage.RemediationDue(ctx, itemID, domain.StuckReasonPRNeedsFix)`: `if !due { ...; return false, nil }` (confirmed at the function's `if !due` branch) skips calling `fixSpawner.AutoReopenForPRFix` entirely for the tick. `RemediationDue`'s schedule (`session/backlog_remediation.go`'s `remediationBackoffSchedule`, confirmed: `30m, 2h, 8h, 24h, 72h`) is sized for OOM-restart bursts (per that file's own doc comment) — not a per-tick, content-aware decision. Nothing in Story 4.2.1's `notifyActiveSessionSteered` resets or advances this row on a successful steer, so once the gate fires once for an item, `AutoReopenForPRFix` — and therefore `steerActiveSessionForPRFix` — is not reachable again for up to 72h, starving Epic 2.2/2.3's dedup/debounce design of the ~60s cadence it was built for.

**Fix — prevention (b) from pre-mortem.md, pulling the steer branch out from behind the gate entirely** (chosen over resetting/advancing the shared backoff row, which the spawn path still needs untouched): give `remediatePRFixWithBackoffGate` a cheap, side-effect-free way to know — *before* consulting `RemediationDue` — whether `AutoReopenForPRFix` would take the active-session (steer) branch, and if so, call `AutoReopenForPRFix` unconditionally, skipping `RemediationDue` entirely for that tick. This is a query-then-call split, not a second copy of the steer-vs-spawn decision: which branch `AutoReopenForPRFix` takes stays solely inside `AutoReopenForPRFix`/`steerActiveSessionForPRFix` (Story 4.2.1) exactly as already planned — this story only adds a pre-check so the caller knows when to bypass the gate. The no-active-session (spawn) branch is completely unaffected: it still goes through the exact same `RemediationDue`-gated call it does today, with no weakening of spawn-storm protection.

**Acceptance Criteria**:
- `session.PRFixSpawner` gains a new method, `HasActiveWorkSession(ctx context.Context, itemID string) (bool, error)`, alongside the existing `AutoReopenForPRFix`. It is side-effect-free — no status transition, no `MarkStuck`, no notification — and mirrors the same `findActiveWorkSession` check `AutoReopenForPRFix` already performs internally.
  - *Given* an itemID with a live work-role `ItemSession`, *When* `HasActiveWorkSession(ctx, itemID)` is called, *Then* it returns `(true, nil)` without transitioning the item's status or calling `MarkStuck`/notifying anything.
  - *Given* an itemID with no live work session, *When* called, *Then* it returns `(false, nil)`.
- `remediatePRFixWithBackoffGate` calls `fixSpawner.HasActiveWorkSession(ctx, itemID)` before consulting `l.storage.RemediationDue`. When it returns `(true, nil)`, the function calls `fixSpawner.AutoReopenForPRFix(ctx, itemID, fixCtx)` immediately and returns its result as `(true, err)` — `RemediationDue` is never consulted for this tick. When it returns `(false, nil)` or a non-nil error (fail-open — logged, never returned, matching this function's existing fail-open rationale for its other best-effort `MarkStuck`/`FindOpenStuckStates`/`RemediationDue` error paths), execution falls through to the existing `RemediationDue`-gated path, completely unchanged.
  - *Given* an active session was steered for reason A at tick N, and the failure reason changes to B at tick N+1 — within the `RemediationDue` backoff window that would otherwise block a respawn attempt for up to 72h — *When* `AutoReopenForPRFix` runs at tick N+1 (`HasActiveWorkSession` still reports `true`), *Then* `steerActiveSessionForPRFix` is still invoked and delivers a steer for reason B, not blocked by the backoff gate (Epic 2.2's `isDuplicateSteerReason` sees the changed `reasonSignature` and does not suppress it).
  - *Given* `HasActiveWorkSession` returns `(false, nil)` (the spawn case), *When* `remediatePRFixWithBackoffGate` runs, *Then* the existing `RemediationDue` gate and its 30m→2h→8h→24h→72h backoff behavior is exercised exactly as before this story — fully protecting the spawn path against a spawn storm, unchanged.
- **Positive side-finding restated (pre-mortem.md):** since `RemediationDue` continues to gate the spawn path unchanged, and the steer path runs under only its own tighter, content-aware throttle (Epic 2.2's 5-minute cooldown + reason-signature dedup, Epic 2.3's two-tick conflict debounce) — a throttle a coarse time-based backoff can't improve on here, because steering an already-active session can't create a duplicate session the way an unthrottled spawn could — this bypass introduces no new spawn-storm or steer-storm risk.
**Files**: `session/backlog_lifecycle_pr.go`, `server/services/backlog_service_triage.go`, `session/backlog_lifecycle_test.go`, `session/backlog_lifecycle_pr_trigger_test.go`

##### Task 4.2.2a: Add `HasActiveWorkSession` to the `PRFixSpawner` interface (~2 min)
- In `session/backlog_lifecycle_pr.go`, extend the interface (`:22-24`):
  ```go
  type PRFixSpawner interface {
      AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error
      // HasActiveWorkSession reports whether itemID currently has a live work
      // session, mirroring AutoReopenForPRFix's own findActiveWorkSession
      // check. Side-effect-free — no transition, no MarkStuck, no
      // notification. Lets remediatePRFixWithBackoffGate decide whether to
      // bypass the RemediationDue backoff gate before calling
      // AutoReopenForPRFix, without duplicating the steer-vs-spawn decision
      // itself, which stays solely inside AutoReopenForPRFix (pre-mortem.md
      // P1 fix).
      HasActiveWorkSession(ctx context.Context, itemID string) (bool, error)
  }
  ```
- Files: `session/backlog_lifecycle_pr.go`

##### Task 4.2.2b: Implement `HasActiveWorkSession` on `BacklogService` (~3 min)
- In `server/services/backlog_service_triage.go`, directly above `AutoReopenForPRFix` (`:2018`), add:
  ```go
  // HasActiveWorkSession implements session.PRFixSpawner's query half — a
  // side-effect-free check letting remediatePRFixWithBackoffGate decide
  // whether to bypass its backoff gate before calling AutoReopenForPRFix.
  // Mirrors the same findActiveWorkSession check AutoReopenForPRFix performs
  // internally; does not transition status or mark/notify anything.
  func (s *BacklogService) HasActiveWorkSession(ctx context.Context, itemID string) (bool, error) {
      if s.storage == nil {
          return false, fmt.Errorf("storage not available")
      }
      sessions, err := s.storage.ListItemSessions(ctx, itemID)
      if err != nil {
          return false, fmt.Errorf("list sessions: %w", err)
      }
      return findActiveWorkSession(sessions) != nil, nil
  }
  ```
- Files: `server/services/backlog_service_triage.go`

##### Task 4.2.2c: Bypass the gate in `remediatePRFixWithBackoffGate` (~5 min)
- In `session/backlog_lifecycle_pr.go`, insert directly above the existing `due, justParked, gateErr := l.storage.RemediationDue(...)` line:
  ```go
  // Bypass the backoff gate entirely for the steer path (pre-mortem.md P1):
  // steering an already-active session has its own tighter, content-aware
  // throttle (5-min cooldown + reason-signature dedup, two-tick conflict
  // debounce — Epic 2.2/2.3) and, unlike the no-active-session spawn branch
  // below, can't create a duplicate session — so it doesn't need, and must
  // not be slowed by, the 30m->72h backoff that exists solely to prevent a
  // spawn storm on that other branch.
  hasActive, activeErr := fixSpawner.HasActiveWorkSession(ctx, itemID)
  if activeErr != nil {
      log.WarningLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate HasActiveWorkSession item=%s: %v", itemID, activeErr)
      // fail open — fall through to the due-gated path below, same rationale
      // as every other best-effort check in this function.
  } else if hasActive {
      log.InfoLog().Printf("[BacklogLifecycle] remediatePRFixWithBackoffGate item=%s: active session found, steering unconditionally (bypassing backoff gate)", itemID)
      return true, fixSpawner.AutoReopenForPRFix(ctx, itemID, fixCtx)
  }
  ```
  Also update this function's existing doc comment (currently describing it as unconditionally wrapping `AutoReopenForPRFix` with the backoff gate) to note the steer-path bypass.
- Files: `session/backlog_lifecycle_pr.go`

##### Task 4.2.2d: Extend `fakePRFixSpawner` and add the session-package regression test (~8 min)
- In `session/backlog_lifecycle_test.go`, add a configurable field to `fakePRFixSpawner` (`:754`) — e.g. `hasActiveWorkSession bool` (default `false`, matching every existing test's implicit "no active session" behavior, since none of them set it) — and implement:
  ```go
  func (f *fakePRFixSpawner) HasActiveWorkSession(ctx context.Context, itemID string) (bool, error) {
      return f.hasActiveWorkSession, nil
  }
  ```
- In `session/backlog_lifecycle_pr_trigger_test.go` (the existing natural home for `remediatePRFixWithBackoffGate`-level tests — it already tests `TriggerPRFixForEvent`'s webhook-triggered-tag behavior at `TestTriggerPRFixForEvent_should_TagFixAttemptLogAsWebhookTriggered` and the poller equivalent at `TestReconcilePRPending_should_TagFixAttemptLogAsPollerTriggered`), add `TestReconcilePRPending_should_SteerOnEveryTick_When_ActiveSessionPresentEvenWithinBackoffWindow`: drive the reconcile path across two ticks with `fakePRFixSpawner{hasActiveWorkSession: true}`, changing the item's PR status between ticks so the underlying failure reason genuinely differs (e.g. conflict resolved, CI now failing — the same example requirements.md uses for Success Metric #2), and assert `fakeSpawner.callCount == 2` — `AutoReopenForPRFix` was called on both ticks, proving the second call was never blocked by `RemediationDue`'s 30-minute floor. This is the **session-package-level** integration test pre-mortem.md's P1 explicitly calls for, distinct from the `server/services`-level tests (Epic 5.3) that cover `steerActiveSessionForPRFix`'s own dedup/debounce logic in isolation.
- Add this test's name to Epic 5.3's Story 5.3.1 scenario list below for cross-reference (it lives in a different package/file, so it is not one of that story's fourteen `server/services`-level scenarios, but both together are what closes pre-mortem.md P1).
- Files: `session/backlog_lifecycle_test.go`, `session/backlog_lifecycle_pr_trigger_test.go`

##### Task 4.2.2e: Build and run (~3 min)
- `go build ./... && go test ./session -run "TestReconcilePRPending|TestTriggerPRFixForEvent" -v`
- Files: none (verification only)

### Epic 4.3: `StuckReasonSteerFailed` and `notifyActiveSessionSteered`
**Goal**: Per ADR-002, a failed steer gets a distinct, visible `StuckReason`; a successful one resolves any existing blocked-active row without inventing a new reason for it. **Known pre-existing gap, not a regression:** `remediationActionByReason` (`server/services/backlog_service_stuck.go`) has no case for `StuckReasonSteerFailed`, so `BlockerChip`'s "Retry now" affordance will render for it but `TriggerRemediationNow` will return `connect.CodeUnimplemented` on click — the same gap `StuckReasonRespawnBlockedActive` already has today (also unwired, verified: neither reason has a case in that switch); wiring an automated remediation for either reason is out of scope for this project.

#### Story 4.3.1: Add `StuckReasonSteerFailed` end-to-end
**As a** reviewer looking at `BlockerChip`, **I want** a failed steer to render distinctly from "respawn skipped," **so that** I can tell "actively attempted and failed" from "didn't try."
**Acceptance Criteria**:
- `domain.StuckReasonSteerFailed` exists, is included in `AllStuckReasons`, and `IsValid()` returns `true` for it.
  - *Given* `r := domain.StuckReasonSteerFailed`, *When* `r.IsValid()` is called, *Then* it returns `true`; *When* `domain.AllStuckReasons` is scanned for `r`, *Then* it is present exactly once.
- `proto/session/v1/backlog.proto` defines `STUCK_REASON_STEER_FAILED = 19`, and after `make proto-gen`, `toProtoStuckReason(domain.StuckReasonSteerFailed)` returns `sessionv1.StuckReason_STUCK_REASON_STEER_FAILED`, with `fromProtoStuckReason` as its exact inverse.
  - *Given* `domain.StuckReasonSteerFailed`, *When* round-tripped through `toProtoStuckReason` then `fromProtoStuckReason`, *Then* the result equals the original value.
- `web-app`'s `STUCK_REASON_LABELS`/`STUCK_REASON_ICONS`/`STUCK_REASON_CLASS` all have an entry for `StuckReason.STEER_FAILED` (TypeScript would otherwise fail to compile, per the `Record<StuckReason, T>` typing).
  - *Given* `web-app/src/components/backlog-stuck/stuckReason.ts` after this change, *When* `pnpm exec tsc --noEmit` runs in `web-app/`, *Then* it succeeds with no "missing property" error on any of the three `Record<StuckReason, T>` maps.
**Files**: `session/domain/backlog.go`, `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`, `web-app/src/components/backlog-stuck/stuckReason.ts`, `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 4.3.1a: Add the domain constant (~2 min)
- In `session/domain/backlog.go`, add `StuckReasonSteerFailed StuckReason = "steer_failed"` next to `StuckReasonRespawnBlockedActive`'s definition, add it to `AllStuckReasons`, and add it to the `IsValid()` switch's case list.
- Files: `session/domain/backlog.go`

##### Task 4.3.1b: Add the proto enum value + regenerate (~3 min)
- In `proto/session/v1/backlog.proto`, add `STUCK_REASON_STEER_FAILED = 19;` (with a doc comment mirroring the existing style, referencing `domain.StuckReasonSteerFailed`) after `STUCK_REASON_BLOCKED_BY_DEPENDENCY = 18;`.
- Run `make proto-gen` (regenerates `gen/`, not committed per repo convention).
- Files: `proto/session/v1/backlog.proto`

##### Task 4.3.1c: Add the adapter switch cases (~3 min)
- In `server/services/backlog_service_stuck.go`, add a case to both `toProtoStuckReason` and `fromProtoStuckReason` mapping `domain.StuckReasonSteerFailed` ⟷ `sessionv1.StuckReason_STUCK_REASON_STEER_FAILED`.
- Files: `server/services/backlog_service_stuck.go`

##### Task 4.3.1d: Add the frontend label/icon/class (~4 min)
- In `web-app/src/components/backlog-stuck/stuckReason.css.ts`, add `chipSteerFailed` mirroring `chipPushFailed`'s error styling.
- In `web-app/src/components/backlog-stuck/stuckReason.ts`, add `[StuckReason.STEER_FAILED]: "Steer attempt failed"` to `STUCK_REASON_LABELS`, `[StuckReason.STEER_FAILED]: "⛔"` to `STUCK_REASON_ICONS`, `[StuckReason.STEER_FAILED]: styles.chipSteerFailed` to `STUCK_REASON_CLASS`.
- Files: `web-app/src/components/backlog-stuck/stuckReason.ts`, `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 4.3.1e: Build/typecheck both sides (~3 min)
- `go build ./...` and (`cd web-app && pnpm exec tsc --noEmit`).
- Files: none (verification only)

#### Story 4.3.2: `notifyActiveSessionSteered`
**As an** operator, **I want** a visible notification for both a successful and a failed steer, **so that** "every steer attempt is visible" (requirements.md's success metric) holds without opening the notification bell for the failure case, and **I want** `StuckReasonSteerFailed` and `StuckReasonRespawnBlockedActive` to never both sit open on the same item, **so that** `BlockerChip`'s single-chip-per-item collapse can't non-deterministically show the stale, less-severe reason instead of the fresh one (undermining ADR-002's entire stated purpose).
**Acceptance Criteria**:
- On success (`deliverErr == nil`), the helper publishes `NOTIFICATION_TYPE_INFO`/`NOTIFICATION_PRIORITY_LOW`, resolves any open `StuckReasonRespawnBlockedActive` row via `resolveRespawnBlockedActiveLogged`, and **also** resolves any open `StuckReasonSteerFailed` row (a success supersedes a prior failure recorded on an earlier tick for the same item); it does **not** call `MarkStuck` with any reason.
  - *Given* an item with an open `StuckReasonRespawnBlockedActive` row and a call `notifyActiveSessionSteered(ctx, itemID, title, currentStatus, activeUUID, message, program, reason, nil)`, *When* it runs, *Then* `s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive)` is called and no `MarkStuck` call occurs, and the published event's type is `NOTIFICATION_TYPE_INFO`.
  - *Given* an item with an open `StuckReasonSteerFailed` row (from an earlier failed tick) and a call `notifyActiveSessionSteered(ctx, itemID, title, currentStatus, activeUUID, message, program, reason, nil)` (this tick succeeded), *When* it runs, *Then* `s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonSteerFailed)` is also called.
- On failure (`deliverErr != nil`), the helper publishes `NOTIFICATION_TYPE_WARNING`/`NOTIFICATION_PRIORITY_MEDIUM` (or higher, per UX research §3 wanting it to stand out relative to the routine INFO case), calls `MarkStuck(ctx, itemID, domain.StuckReasonSteerFailed, currentStatus, <detail including deliverErr>)`, and **also** resolves any open `StuckReasonRespawnBlockedActive` row for the item — verified real gap (adversarial review): without this, a tick that hits the dedup/not-live degrade path (opening `RespawnBlockedActive`) followed by a later tick that attempts and fails to steer (opening `SteerFailed`) would leave both rows open simultaneously, and `BlockerChip`'s unordered single-chip collapse could then non-deterministically render the stale "Auto-respawn skipped" chip instead of the fresh, more severe "Steer attempt failed" one.
  - *Given* `notifyActiveSessionSteered(ctx, itemID, title, currentStatus, activeUUID, message, program, reason, someErr)`, *When* it runs, *Then* `s.storage.MarkStuck` is called with `domain.StuckReasonSteerFailed`, the published event's type is `NOTIFICATION_TYPE_WARNING`, and `s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive)` is also called.
- **(P1 fix, UX re-review)** Both the success and failure notification's title and body name the actual reason category from `reason`, via `humanReadableReasonSet(reason)` — not a generic phrase like "with PR-fix context" or "failed" alone — per `research/ux.md` §2's explicit content spec ("Notification title: name the PR and the reason category... If multiple reasons are true simultaneously... name the set, not just the first one").
  - *Given* `reason := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}` and `deliverErr == nil`, *When* `notifyActiveSessionSteered` runs, *Then* the published event's title is `"Steered active session — <itemTitle> has a merge conflict and failing CI"` and its body includes both `activeSessionUUID` and the phrase `"a merge conflict and failing CI"`.
  - *Given* the same `reason` but `deliverErr != nil`, *When* `notifyActiveSessionSteered` runs, *Then* the published event's title is `"Failed to steer active session — <itemTitle> needs attention for a merge conflict and failing CI"` and its body includes `activeSessionUUID`, the phrase `"a merge conflict and failing CI"`, **and** `deliverErr`'s text — so the operator sees both what was wrong with the PR and why the automated fix-attempt itself didn't work, since a failed steer produces zero terminal signal and this notification is the operator's only record (`research/ux.md` §3).
- **(UX triad review)** On failure, when the active session's `program` satisfies `isClaudeCodeProgram(program)` (Task 3.1.1a), the failure notification's body also names a manual remediation path, since the operator's only signal for a failed steer currently tells them what's wrong but not what to do about it — reusing the exact program-gating decision `buildSteerMessage` (Epic 3.1) already makes for the same reason, not a second, divergent check.
  - *Given* `reason`, `deliverErr != nil`, and `program = "claude"`, *When* `notifyActiveSessionSteered` runs, *Then* the published event's body ends with the literal phrase `" — try /github:pr-ship manually"`.
  - *Given* the same inputs but `program = "aider"`, *When* `notifyActiveSessionSteered` runs, *Then* the published event's body does **not** contain `/github:pr-ship` — a non-Claude-Code session gets no slash-command instruction, matching requirements.md's Constraints ("a literal `/github:pr-ship` instruction only means anything to a Claude Code session").
  - *Given* a header-less `reasonSignature` (e.g. from the "PR closed without merging" call site), *When* either branch runs, *Then* the reason phrase falls back to `"a PR problem"` rather than an empty or malformed phrase.
  - *Given* an item with a prior open `StuckReasonRespawnBlockedActive` row (from an earlier degrade-path tick) and a subsequent failed-steer tick, *When* `notifyActiveSessionSteered` runs with a non-nil `deliverErr`, *Then* the prior `RespawnBlockedActive` row is resolved when the new `SteerFailed` row is opened — at most one of the two reasons is ever open for this item afterward.
- The three degrade paths inside `steerActiveSessionForPRFix` that call `notifyRespawnBlockedByActiveSession` (nil `sessionSteerer`, `SessionProgram` reporting not-live, dedup/debounce suppression) also resolve any open `StuckReasonSteerFailed` row for the item — the reverse direction of the invariant above, so a later "the session turned out fine, we're just skipping/suppressing" tick clears a stale failure record instead of leaving it to linger next to the reaffirmed `RespawnBlockedActive` row. (See Task 4.2.1a/4.2.1b, not this story's own files — `notifyActiveSessionSteered` only owns the success/failure branch's half.)
  - *Given* an item with an open `StuckReasonSteerFailed` row, *When* `steerActiveSessionForPRFix` runs and hits any of its three degrade branches (e.g. `s.sessionSteerer == nil`), *Then* `s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonSteerFailed)` is called before/alongside the existing `notifyRespawnBlockedByActiveSession` call.
- Both paths' published event carries `metadata: {"item_id": itemID}`.
  - *Given* either the success or failure path, *When* the published `events.Event` is inspected, *Then* `.NotificationMetadata["item_id"] == itemID`.
**Files**: `server/services/backlog_service_pr_fix_steer.go`, `server/services/backlog_service_triage.go`

##### Task 4.3.2a: Add `humanReadableReasonSet`, then implement the success branch naming the reason (~8 min)
**(P1 fix, UX re-review)**: the notification title/body must name the actual reason category (per `research/ux.md` §2), not a generic "with PR-fix context" phrase — see the Domain Glossary's `humanReadableReasonSet` entry.
- Add to `server/services/backlog_service_pr_fix_steer.go`, above `notifyActiveSessionSteered`:
  ```go
  // humanReadableReasonSet turns a reasonSignature's ordered headers into a
  // short phrase for a notification title/body, e.g. ["## Merge conflict",
  // "## Failing CI checks"] -> "a merge conflict and failing CI". Strips the
  // dynamic "@author" half of a review header down to "a blocking review" —
  // that detail belongs in the terminal's full fixContext, not the compact
  // notification card (research/ux.md §2). Falls back to "a PR problem" for
  // a header-less signature (e.g. the "PR closed without merging" fixContext).
  //
  // Note: a new PRStatus.render() section header added without a matching
  // case here silently falls through to the generic fallback phrase rather
  // than failing loudly — acceptable since the fallback is still
  // informative, but worth knowing (the pinning test below only catches a
  // wording change to an existing header, not an entirely new one).
  func humanReadableReasonSet(sig reasonSignature) string {
      var phrases []string
      for _, h := range sig.headers {
          switch {
          case h == "## Merge conflict":
              phrases = append(phrases, "a merge conflict")
          case h == "## Failing CI checks":
              phrases = append(phrases, "failing CI")
          case strings.HasPrefix(h, "## Review: changes requested by"):
              phrases = append(phrases, "a blocking review")
          case h == "## Reviewer comments":
              phrases = append(phrases, "reviewer comments")
          case h == "## PR comments":
              phrases = append(phrases, "PR comments")
          }
      }
      switch len(phrases) {
      case 0:
          return "a PR problem"
      case 1:
          return phrases[0]
      case 2:
          return phrases[0] + " and " + phrases[1]
      default:
          return strings.Join(phrases[:len(phrases)-1], ", ") + ", and " + phrases[len(phrases)-1]
      }
  }
  ```
  Add `TestHumanReadableReasonSet_HeaderStrings_MatchPRStatusRender` alongside `TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender` (Task 2.1.1d) so the same `PRStatus.render()`-header-string pin covers this function's `switch` cases too — a future wording change to `render()`'s headers should fail this test loudly instead of silently making `humanReadableReasonSet` fall through to the empty-phrases branch for a header it no longer recognizes.
- Then add `notifyActiveSessionSteered` itself, now taking the session's `program` as its 7th parameter and the triggering `reasonSignature` as its 8th (immediately before `deliverErr`) so both branches can call `humanReadableReasonSet` on the reason, and the failure branch can gate a remediation-path suffix on the program, without either check being duplicated a second time:
  ```go
  // notifyActiveSessionSteered records the outcome of a PR-fix steer attempt
  // against an already-active session — success is INFO/LOW and resolves any
  // open respawn_blocked_active or steer_failed row (a successful steer isn't
  // a stuck condition, and supersedes any earlier tick's failure); failure is
  // WARNING and marks StuckReasonSteerFailed (ADR-002) so it's visible via
  // BlockerChip without opening the notification bell, resolving any open
  // respawn_blocked_active row so the two reasons are never both open at once
  // (adversarial review: BlockerChip's single-chip collapse would otherwise
  // non-deterministically show whichever stale reason a query happened to
  // return first). Both branches name the actual reason category via
  // humanReadableReasonSet(reason) rather than a generic phrase, since a
  // failed steer produces zero terminal signal and this notification is the
  // operator's only record of what was wrong (research/ux.md §2/§3). The
  // failure branch also names a remediation path for a Claude Code session
  // (isClaudeCodeProgram(program), Task 3.1.1a) — the operator's only signal
  // otherwise says what's wrong but not what to do about it (UX triad review).
  func (s *BacklogService) notifyActiveSessionSteered(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID, message, program string, reason reasonSignature, deliverErr error) {
      reasonPhrase := humanReadableReasonSet(reason)
      if deliverErr == nil {
          log.InfoLog().Printf("[AutoReopenForPRFix] steered active session %s for item %s", activeSessionUUID, itemID)
          s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)
          s.resolveSteerFailedLogged(ctx, "AutoReopenForPRFix", itemID)
          if s.eventBus == nil {
              return
          }
          s.eventBus.Publish(events.NewNotificationEvent(
              itemID, "", uuid.New().String(),
              int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
              int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW),
              fmt.Sprintf("Steered active session — %s has %s", itemTitle, reasonPhrase),
              fmt.Sprintf("%s — session %s was steered for %s.", itemTitle, activeSessionUUID, reasonPhrase),
              map[string]string{"item_id": itemID},
          ))
          return
      }
      // failure branch: Task 4.3.2b
  }
  ```
  Note: no `item.PrNumber` is threaded into `steerActiveSessionForPRFix`/`notifyActiveSessionSteered` anywhere in this plan, so the title names the item by `itemTitle` (already a parameter), not a PR number — matching `research/ux.md` §2's title example in spirit ("name the PR and the reason category") using the identifying info actually in scope. Threading `PrNumber` through as an additional parameter is a candidate follow-up, not required here.
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 4.3.2b: Implement the failure branch, naming the reason, a remediation path, and resolving the opposite stuck reason (~7 min)
- Replace the `// failure branch` comment with:
  ```go
  log.WarningLog().Printf("[AutoReopenForPRFix] failed to steer active session %s for item %s: %v", activeSessionUUID, itemID, deliverErr)
  if s.storage != nil {
      if _, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonSteerFailed, currentStatus,
          fmt.Sprintf("AutoReopenForPRFix failed to steer active session %s (%s): %v", activeSessionUUID, reasonPhrase, deliverErr)); err != nil {
          log.WarningLog().Printf("[AutoReopenForPRFix] MarkStuck(steer_failed) item=%s: %v", itemID, err)
      }
  }
  // A prior degrade-path tick may have left RespawnBlockedActive open for this
  // item; a failed steer is a strictly more specific/severe finding, so clear
  // the stale one — see this story's "at most one open at a time" acceptance
  // criterion (adversarial review).
  s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)
  if s.eventBus == nil {
      return
  }
  body := fmt.Sprintf("%s — steering session %s failed (%s): %v", itemTitle, activeSessionUUID, reasonPhrase, deliverErr)
  // Name a remediation path for a Claude Code session — the operator's only
  // signal on a failed steer otherwise says what's wrong but not what to do
  // about it (UX triad review). Reuses buildSteerMessage's own program-gating
  // decision (Epic 3.1) rather than a second, divergent check.
  if isClaudeCodeProgram(program) {
      body += " — try /github:pr-ship manually"
  }
  s.eventBus.Publish(events.NewNotificationEvent(
      itemID, "", uuid.New().String(),
      int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
      int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
      fmt.Sprintf("Failed to steer active session — %s needs attention for %s", itemTitle, reasonPhrase),
      body,
      map[string]string{"item_id": itemID},
  ))
  ```
  The failure body includes `reasonPhrase` (what was wrong with the PR), `deliverErr` (why the automated steer attempt itself didn't work), and, for a Claude Code session, a manual fallback instruction — per the gap this fix closes: previously the failure body reported only the delivery error, leaving the operator, whose only signal this is (research/ux.md §3), with no idea what problem triggered the steer in the first place, nor what to do next now that the automated path failed.
- Files: `server/services/backlog_service_pr_fix_steer.go`

##### Task 4.3.2c: Add `resolveSteerFailedLogged`, mirroring `resolveRespawnBlockedActiveLogged` (~3 min)
- In `server/services/backlog_service_triage.go`, directly below `resolveRespawnBlockedActiveLogged` (`:1405`), add:
  ```go
  // resolveSteerFailedLogged clears an open StuckReasonSteerFailed row for
  // itemID, logging (not returning) any storage error. Mirrors
  // resolveRespawnBlockedActiveLogged exactly, for the opposite direction of
  // this project's "at most one of {SteerFailed, RespawnBlockedActive} open
  // at a time" invariant (adversarial review).
  func (s *BacklogService) resolveSteerFailedLogged(ctx context.Context, caller, itemID string) {
      if s.storage == nil {
          return
      }
      if _, err := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonSteerFailed); err != nil {
          log.WarningLog().Printf("[%s] ResolveStuck(steer_failed) item=%s: %v", caller, itemID, err)
      }
  }
  ```
- Files: `server/services/backlog_service_triage.go`

##### Task 4.3.2d: Build (~2 min)
- `go build ./server/...`
- Files: none (verification only)

---

## Phase 5: Tests

### Epic 5.1: `steerInstance` Extraction Tests
Covered by Stories 1.1.1/1.1.2/1.1.3 (Tasks 1.1.1c, 1.1.2b, 1.1.2c, 1.1.3c) — no additional epic-level work; listed here for dependency-visualization completeness.

### Epic 5.2: Dedup/Debounce Pure-Function Tests
Covered by Stories 2.1.1/2.2.1/2.3.1/3.1.1 — no additional epic-level work; listed here for dependency-visualization completeness.

### Epic 5.3: `AutoReopenForPRFix` Integration Tests
**Goal**: Exercise the full `steerActiveSessionForPRFix` branch end-to-end via a `mockSessionSteerer`, following `TestAutoReopenForPRFix_ActiveWorkSession_*` naming.

#### Story 5.3.1: `mockSessionSteerer` fake and the full scenario matrix
**As a** future maintainer, **I want** the entire decision tree covered by name-matching tests, **so that** a regression in any branch (nil-safe degrade, dedup, debounce, program-gating, truncation, in-flight guard, no-double-transition) is caught immediately.
**Acceptance Criteria**:
- `mockSessionSteerer` implements `SessionSteerer`, records every `SteerActiveSession` call (UUID + message), and lets a test configure `SessionProgram`'s return per UUID and `SteerActiveSession`'s return error per call.
  - *Given* `m := &mockSessionSteerer{programs: map[string]string{"uuid-1": "claude"}}`, *When* `m.SessionProgram("uuid-1")` is called, *Then* it returns `("claude", true)`; *When* called for an unconfigured UUID, *Then* it returns `("", false)`.
- Each of the following is its own named test in `server/services/backlog_service_triage_test.go` (or a new `backlog_service_pr_fix_steer_integration_test.go` if the triage test file is already large — check line count and split if `> 3000` lines per this repo's own file-size norms): steer delivered on a new reason; dedup suppresses an identical reason within cooldown; **re-steers on a genuinely different reason even within cooldown** (`TestAutoReopenForPRFix_ActiveWorkSession_ReSteersOnReasonChange_EvenWithinCooldown` — adversarial-review Concern: exercises Success Metric #2, which no prior test covered); **an active-session UUID change bypasses dedup and re-steers even for an identical reason** (architecture-review concern: the item's active work session can change between ticks); a Claude-vs-non-Claude program difference does not itself defeat dedup (dedup is keyed on `reasonSignature`, not program); conflicts require two consecutive ticks before steering; nil `sessionSteerer` degrades to existing notify-only; `SessionProgram` `ok=false` degrades to existing notify-only; a steer failure produces a WARNING notification + `StuckReasonSteerFailed` row, not silence (`TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_ProducesWarningAndStuckRow`); **a successful steer publishes an INFO notification and resolves any open `RespawnBlockedActive` row**, named explicitly rather than left implicit (`TestAutoReopenForPRFix_ActiveWorkSession_SuccessfulSteer_PublishesInfoNotificationAndResolvesRespawnBlockedActive` — adversarial-review Concern: Success Metric #4's success half previously had no named test, only the failure half did); a message is truncated when it would exceed `session.MaxSteerMessageLength`; `steerInFlight` prevents a concurrent duplicate send; no `TransitionBacklogItemStatus` call happens in this branch; **a failed steer resolves a prior open `RespawnBlockedActive` row, and a degrade-path reaffirmation of `RespawnBlockedActive` resolves a prior open `SteerFailed` row** (`TestAutoReopenForPRFix_ActiveWorkSession_SteerFailureResolvesStalePriorRespawnBlockedActiveRow` — adversarial-review Concern: without this, the two `StuckReason`s could both sit open simultaneously, and `BlockerChip`'s single-chip collapse could non-deterministically show the stale one).
  - *Given* the naming convention `TestAutoReopenForPRFix_ActiveWorkSession_SteersInsteadOfSkipping_When_SessionSteererWired`, *When* `go test ./server/services -run TestAutoReopenForPRFix_ActiveWorkSession -v` runs, *Then* all fourteen scenario tests listed above are present and pass.
**Files**: `server/services/backlog_service_test.go` (add `mockSessionSteerer`), `server/services/backlog_service_triage_test.go` (or a new `backlog_service_pr_fix_steer_integration_test.go`)

##### Task 5.3.1a: Add `mockSessionSteerer` (~5 min)
- In `server/services/backlog_service_test.go`, directly below `mockSessionStopper`, add:
  ```go
  // mockSessionSteerer implements SessionSteerer for tests, mirroring
  // mockSessionStopper's shape.
  type mockSessionSteerer struct {
      programs    map[string]string // uuid -> program; absent = not live
      steerErr    map[string]error  // uuid -> error SteerActiveSession returns
      steerCalls  []mockSteerCall
      mu          sync.Mutex
  }

  type mockSteerCall struct {
      uuid    string
      message string
  }

  func (m *mockSessionSteerer) SessionProgram(uuid string) (string, bool) {
      p, ok := m.programs[uuid]
      return p, ok
  }

  func (m *mockSessionSteerer) SteerActiveSession(_ context.Context, uuid, message string) error {
      m.mu.Lock()
      defer m.mu.Unlock()
      m.steerCalls = append(m.steerCalls, mockSteerCall{uuid: uuid, message: message})
      return m.steerErr[uuid]
  }
  ```
- Files: `server/services/backlog_service_test.go`

##### Task 5.3.1b: Check file size and pick the test file (~2 min)
- `wc -l server/services/backlog_service_triage_test.go` — if it already exceeds roughly 3000 lines, create `server/services/backlog_service_pr_fix_steer_integration_test.go` (package `services`) for the new tests instead of growing the existing file further; otherwise append to `backlog_service_triage_test.go` near the existing `TestAutoReopenForPRFix_ActiveWorkSession_*` tests.
- Files: none (decision only, informs the remaining tasks' file target)

##### Task 5.3.1c: Write the nil-safe / not-live degrade tests (~5 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_DegradesToNotifyOnly_When_SessionSteererNotWired` and `..._When_SessionProgramReportsNotLive` — construct a `*BacklogService` (via the existing test harness), with `SetSessionSteerer(nil)` (or simply never called) for the first, and a `mockSessionSteerer{programs: map[string]string{}}` for the second; assert `notifyRespawnBlockedByActiveSession`'s observable effect (existing test helper/assertion pattern already used by sibling `TestAutoReopenForPRFix_ActiveWorkSession_*` tests) fires, and `mockSessionSteerer.steerCalls` is empty.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1d: Write the dedup + cooldown tests, including reason-change and session-change bypasses (~9 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_SteersOnNewReason` and `..._DedupSuppresses_When_IdenticalReasonWithinCooldown` — call `AutoReopenForPRFix` twice in immediate succession with the same `fixContext` for the second test, asserting exactly one `SteerActiveSession` call.
- `TestAutoReopenForPRFix_ActiveWorkSession_ReSteersOnReasonChange_EvenWithinCooldown` — call `AutoReopenForPRFix` once with a `fixContext` producing reason signature A, then immediately again (still within `steerCooldown`) with a `fixContext` producing a genuinely different signature B (a different header set — e.g. conflicts resolved but CI now failing, the literal example from requirements.md's Success Metrics), asserting **two** `SteerActiveSession` calls, not one — pinning `isDuplicateSteerReason`'s changed-candidate branch, which no prior test exercised.
- `TestAutoReopenForPRFix_ActiveWorkSession_SessionUUIDChanged_ReSteersDespiteIdenticalReasonAndCooldown` — steer once for `activeSessionUUID="uuid-1"`, then call `AutoReopenForPRFix` again immediately (within `steerCooldown`) for the *same item* but with `active.SessionUUID="uuid-2"` and an unchanged `fixContext`/reason signature, asserting a **second** `SteerActiveSession` call against `"uuid-2"` — pinning the architecture-review concern that a changed active work session must never be treated as an already-delivered duplicate.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1e: Write the conflict-debounce test (~5 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_ConflictRequiresTwoConsecutiveTicks` — call `AutoReopenForPRFix` with a conflict-containing `fixContext` twice, asserting zero `SteerActiveSession` calls after the first and exactly one after the second.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1f: Write the program-gating/message-content test (~4 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_ProgramGatingDoesNotAffectDedupKey` — steer once with `program="claude"`, then again with an unchanged `fixContext` but a `mockSessionSteerer` reporting a different program for the same UUID; assert the second call is still suppressed by dedup (dedup keys on `reasonSignature`, not `program` — confirms Story 4.2.1's design doesn't accidentally couple the two).
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1g: Write the failure- and success-notification tests, asserting reason-naming and remediation-path content (~13 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_ProducesWarningAndStuckRow` — configure `mockSessionSteerer.steerErr[uuid] = errors.New(...)` and a `fixContext` producing a known `reasonSignature` (e.g. a conflict-only header), assert the storage fake recorded a `MarkStuck(..., domain.StuckReasonSteerFailed, ...)` call and the event bus fake recorded a `NOTIFICATION_TYPE_WARNING` event **whose title and body both contain the reason phrase (`"a merge conflict"`) and whose body also contains the configured error's text** — not just asserting on `NotificationType`/`NotificationPriority` as before this fix (P1, UX re-review): a test that only checks type/priority would still pass against the old generic-phrase wording, so it must assert on the actual title/body strings to catch a regression back to genericness.
- `TestAutoReopenForPRFix_ActiveWorkSession_SuccessfulSteer_PublishesInfoNotificationAndResolvesRespawnBlockedActive` — with the storage fake pre-seeded with an open `StuckReasonRespawnBlockedActive` row for the item (simulating a prior degrade-path tick), configure `mockSessionSteerer.SteerActiveSession` to return `nil` and a `fixContext` producing a known `reasonSignature`; assert the event bus fake recorded a `NOTIFICATION_TYPE_INFO` event **whose title and body contain the reason phrase**, and that the storage fake's `ResolveStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive)` was called — naming this explicitly closes the gap the adversarial review found: only the failure half of Success Metric #4 had a named test before.
- `TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_ClaudeCodeProgram_NamesRemediationPath` — same failure setup as the test above but with `mockSessionSteerer.SessionProgram` returning `("claude", true)`; assert the WARNING event's body ends with the literal string `" — try /github:pr-ship manually"` (UX triad review: the failure notification must name a remediation path, not just what's wrong).
- `TestAutoReopenForPRFix_ActiveWorkSession_SteerFailure_NonClaudeCodeProgram_OmitsRemediationPath` — same setup but `mockSessionSteerer.SessionProgram` returning `("aider", true)`; assert the WARNING event's body does **not** contain `/github:pr-ship` — mirrors `buildSteerMessage`'s own program-gating so the two never disagree about which sessions get the instruction.
- `TestHumanReadableReasonSet_MultipleReasons_NamesTheSet` — `reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}` produces `"a merge conflict and failing CI"` (both named, not just the first — the specific gap `research/ux.md` §2 calls out: "a truncated title that only mentions CI when conflicts are also present would send Tyler down the wrong manual path").
- `TestHumanReadableReasonSet_HeaderlessSignature_FallsBackToGenericPhrase` — a header-less `reasonSignature` (single-element, full-string fallback per Task 2.1.1b) produces `"a PR problem"`.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1h: Write the truncation and in-flight tests (~8 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_MessageTruncated_When_FixContextExceedsMaxLength` — assert the recorded `mockSteerCall.message` is `<= session.MaxSteerMessageLength` and ends with the `/github:pr-ship` suffix (per Story 3.1.1's P2 #3 fix, truncation happens before the suffix is appended, so the suffix — not the truncation pointer — is the last thing in the message).
- `TestAutoReopenForPRFix_ActiveWorkSession_SteerInFlight_PreventsDuplicateConcurrentSend` (pre-mortem.md P2 #2): **deterministic synchronization mechanism, not ad hoc goroutine timing** — this repo already has a concrete idiom for exactly this "N goroutines racing a per-item `sync.Map` guard" shape: `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated`'s `spawnInFlight` regression test (`server/services/backlog_service_test.go:2178`). Follow it rather than inventing a new mechanism: gate every goroutine behind a shared `start := make(chan struct{})`, have each one block on `<-start` immediately before calling `AutoReopenForPRFix`, then `close(start)` to release them together ("release all goroutines together to maximize the race window" — the same comment `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated` uses), and join with `sync.WaitGroup` — no `time.Sleep`-based polling, and no channel-blocking fake inside `mockSessionSteerer.SteerActiveSession` (that would be inventing a second, divergent concurrency-testing idiom rather than reusing this repo's existing one). Shape two of the released goroutines to call `AutoReopenForPRFix` for the same item — one like a `ReconcilePRPending` tick, the other like a `TriggerPRFixForEvent` webhook-delivery call (verify `TriggerPRFixForEvent`'s actual call shape at `session/backlog_lifecycle_pr.go:1599` first so the setup is realistic, not two interchangeable synthetic goroutines) — then assert, exactly as `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated` asserts exact success/conflict counts rather than "at least one," that exactly one goroutine's call reached *any* of `steerActiveSessionForPRFix`'s branches: exactly one `SteerActiveSession` call recorded on `mockSessionSteerer`. Add a second sub-test or table case forcing the degrade branch instead (e.g. `mockSessionSteerer{programs: map[string]string{}}`), using the identical gated-goroutine setup, asserting exactly one `resolveSteerFailedLogged`/`notifyRespawnBlockedByActiveSession` invocation — proving the guard covers that path too, not just delivery. Run this test with `-race` (matching the `spawnInFlight` test's own convention, called out in its doc comment) so a guard that's present but scoped too narrowly is caught by the race detector even on a run where the outcome-count assertion alone would luck into passing.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1i: Write the no-double-transition regression guard (~3 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_NoTransitionBacklogItemStatusCall` — reuse this test file's existing storage fake's call-recording for `TransitionBacklogItemStatus` (already exercised elsewhere in this file per the double-transition-churn incident's own regression tests) and assert zero calls across every scenario in this story.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1j: Write the mutual-exclusion regression test for the two `StuckReason`s (~5 min)
- `TestAutoReopenForPRFix_ActiveWorkSession_SteerFailureResolvesStalePriorRespawnBlockedActiveRow` — pre-seed the storage fake with an open `StuckReasonRespawnBlockedActive` row for the item (simulating an earlier degrade-path tick), then run a tick where `mockSessionSteerer.SteerActiveSession` returns an error; assert the storage fake's `ResolveStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive)` was called (not just `MarkStuck(..., StuckReasonSteerFailed, ...)`), and add the mirror-direction case in the same test or a sibling one — a prior open `StuckReasonSteerFailed` row is resolved when a degrade path (nil `sessionSteerer`, not-live, or dedup/debounce suppression) reaffirms `RespawnBlockedActive`. This pins the adversarial-review finding that the two reasons must never both be open on the same item at once.
- Files: (per Task 5.3.1b's decision)

##### Task 5.3.1k: Run the full new test suite (~3 min)
- `go test ./server/services -run "TestAutoReopenForPRFix_ActiveWorkSession|TestSteerInstance|TestBuildReasonSignature|TestReasonSignature|TestIsDuplicateSteerReason|TestNextLastSteerReason|TestConfirmConflictChange|TestBuildSteerMessage|TestHumanReadableReasonSet" -v` and confirm all pass.
- Files: none (verification only)

##### Task 5.3.1l: Full `make quick-check` pass (~5 min)
- Run `make quick-check` (build + test + lint) to catch any lint issue (e.g. `golangci-lint`'s `dupl`/`gocognit` gates) introduced by the new file, and fix anything it flags before considering this project done.
- Files: as needed to address lint findings
