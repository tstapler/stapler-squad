# Triad Readiness: triage-autonomous-migration

**Date**: 2026-06-15
**Round**: 1

Full analysis files:
- `/tmp/triad-product-triage-autonomous.txt`
- `/tmp/triad-ux-triage-autonomous.txt`
- `/tmp/triad-engineering-triage-autonomous.txt`

---

## Summary Table

| Lens        | Status             | Blockers | Gaps |
|-------------|---------------------|----------|------|
| Product     | 🟡 Needs Work       | 2        | 6    |
| UX Design   | 🟡 Needs Work       | 1        | 6    |
| Engineering | 🟡 Needs Work       | 2        | 7    |

## Overall: **NEEDS WORK — not ready to implement**

---

## Blockers (must resolve before building)

### Product Blockers

**P-B1: No success metrics**
Zero definition of what "triage is working" looks like after ship. No measurable signal to confirm the fix closed the bugs. Minimum viable: (a) hidden sessions no longer appear in WatchSessions stream, (b) `submit_triage_result` is successfully called on the next real triage run, (c) item advances to `ready` after triage.

**P-B2: Goal/status tracking gap is scoped out but undocumented**
The third problem — AutonomousDriver sessions having no structured goal/status reporting — is named in the problem statement but has no AC and no Epic. The feature ships with a known open gap and nothing in the requirements says "follow-on". Either add an AC or explicitly add a "deferred" section to requirements.md with an owner and ticket.

### UX Blocker

**UX-B1: Stuck triage produces zero user signal**
When `onAutonomousDriverComplete` fires with `Stuck=true` for a triage session, Epic 4 correctly skips the status transition (item stays at `idea`). But no notification is emitted, no badge changes, and no re-trigger affordance exists. The operator has no idea triage failed. This is a complete dead end with no recovery path visible in the UI.

### Engineering Blockers

**E-B1: AC-3 requirements drift — MCP stop signal removed without updating requirements**
`requirements.md` AC-3 explicitly mandates `submit_triage_result` as a belt-and-suspenders completion signal ("whichever fires first stops the driver"). Adversarial review Blocker 3 removed it from the plan because `Stop()` always produces `Stuck=true`. The plan was updated but `requirements.md` was not. Either requirements must drop AC-3 or the plan must re-add a safe stop path (e.g. a `suppressCompletion` flag on `Stop()`). Misaligned requirements/plan = undefined contract for implementers.

**E-B2: AutonomousDriverStarter interface widening has no task for mocks or compile-time assertion**
Adding `StartAutonomousDriverWithTimeout` to the `AutonomousDriverStarter` interface (Task 2.2b) breaks any future test mock that implements the old interface. Pitfall #8 in the plan mentions the compile-time assertion `var _ AutonomousDriverStarter = (*SessionService)(nil)` but no Task is assigned to implement it. And when test mocks appear, they'll silently fail to compile. Add a task.

---

## Gaps (should address before or shortly after ship)

### Product Gaps

| # | Gap |
|---|-----|
| P-G1 | No user persona — "operator" used once without definition; no segment clarity |
| P-G2 | No operator notification when triage completes and item advances to `ready` |
| P-G3 | Triage artifacts undefined in product terms — what does `submit_triage_result` produce and where is it surfaced to the operator? |
| P-G4 | No affordance for re-triggering stuck triage from the backlog UI |
| P-G5 | Uncommitted changes in `web-app/src/app/review-queue/page.tsx` not reconciled with "UI out of scope" declaration |
| P-G6 | In-flight oneShot triage sessions during migration not mentioned (assumed none exist; should be stated) |

### UX Gaps

| # | Gap |
|---|-----|
| UX-G1 | No user flow mapped for the full triage lifecycle (idea → triage running → triage done → ready → spawn work) |
| UX-G2 | Zero progress visibility while triage runs — the hidden session is invisible, no backlog item state changes |
| UX-G3 | Turn notifications from AutonomousDriver use the hidden session title, not the backlog item name — confusing to operators |
| UX-G4 | No "triage in progress" visual state on backlog items; all states (queued/running/stuck/done) look identical |
| UX-G5 | oneShot fallback (headlessPool nil) still cannot call `submit_triage_result` — silent failure path unchanged; operator sees triage session exist but never complete |
| UX-G6 | No UX entry point to view triage output/artifacts once item reaches ready |

### Engineering Gaps

| # | Gap |
|---|-----|
| E-G1 | No integration test for full `TriggerTriage → CreateDirectorySession → AutonomousDriver.Start()` success path |
| E-G2 | No test for graceful degradation branch (`autonomousStarter == nil` → `oneShot=true`) |
| E-G3 | `buildTriagePrompt` was written for oneShot and may include implicit exit framing that conflicts with AutonomousDriver multi-turn orchestration — not verified against the orchestration prompt |
| E-G4 | `StartAutonomousDriverWithTimeout` duplicates the turn-callback body verbatim from `StartAutonomousDriverForInstance` — will diverge; no refactor task exists |
| E-G5 | Task 0.1a code snippet omits surrounding wire-callback lines; developer following it literally could accidentally drop `wireRateLimitCallbacks`/`wireStatusChangeCallback` |
| E-G6 | `findSessionTitleByUUID` helper (Epic 5) has no unit test |
| E-G7 | `buildTriagePrompt` instructs the agent to "call submit_triage_result and notify operator that triage is complete" — with AutonomousDriver, the orchestrator LLM also injects turns after the session becomes idle. Risk: orchestrator injects a post-triage turn before detecting DONE, confusing the agent |

---

## Cross-Lens Themes

Three gaps appear independently across multiple lenses — these are the highest-signal items:

| Theme | Raised by |
|---|---|
| Stuck triage = silent black hole (no notification, no UI state, no recovery affordance) | Product P-G4, UX UX-B1, UX UX-G4 |
| AC-3 / MCP completion signal: requirements say one thing, plan does another | Product P-B2 (goal tracking gap), Engineering E-B1 |
| Goal/status tracking for autonomous sessions is unscoped | Product P-B2, UX UX-G2 |

---

## Recommended Next Steps (by priority)

1. **Fix E-B1** — Update `requirements.md` AC-3: either remove the MCP stop requirement for triage (reflect adversarial review Blocker 3 decision) or define a safe stop mechanism. One-liner change; unblocks the engineering/requirements contract.

2. **Fix UX-B1 + P-G4 together** — Add a `NotificationEvent` emission in `onAutonomousDriverComplete` when `Stuck=true` and role=triage. Add a "Re-trigger triage" button on the backlog item detail panel for items in `idea` status that have a prior stuck triage `ItemSession`. This is the minimum to make stuck triage recoverable.

3. **Fix P-B1** — Add 3 measurable success metrics to `requirements.md` (hidden filter verifiable, submit_triage_result call observable in logs, item→ready transition confirmed in staging).

4. **Fix E-B2** — Add `var _ AutonomousDriverStarter = (*SessionService)(nil)` as a task in plan Epic 2; update the note to remind implementers to update any test mocks.

5. **Address P-B2 / UX-G2** — Add a "Goal & Status Tracking" section to requirements.md that either: (a) defers goal/status reporting to a follow-on ticket with an explicit name, OR (b) adds a new Epic 8 that wires `set_session_goal` into `TriggerTriage`/`TriggerReReview` sessions and surfaces triage progress in the backlog item panel.

6. **Address E-G3** — Read `buildTriagePrompt` and the `autonomousSystemPrompt` side by side. Confirm no contradictory guidance. If needed, add a sentence to the triage prompt: "After calling submit_triage_result, wait for further instructions." This prevents the orchestrator from re-injecting after completion.
