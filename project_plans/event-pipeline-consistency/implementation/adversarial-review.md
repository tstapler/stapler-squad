# Adversarial Review — Event Pipeline Consistency Implementation Plan

**Reviewer role:** Adversarial — assume the plan is wrong until proven right.  
**Date:** 2026-06-20  
**Verdict:** CONCERNS (no hard blockers, but 6 significant issues require correction before implementation)

---

## Summary

The plan is structurally sound and most of its factual claims check out. However, it contains
**two naming errors that will cause compile failures** (issue 2 and 4), **one method that does not
exist** that the plan treats as a simple "call it" task (issue 3), and **several incorrect
assumptions** about the shape of existing code (issues 5, 8, 9). These need to be corrected in
the plan before implementation begins; a developer following the plan as written will hit
compilation failures on Epic 1 Story 1.3 and Epic 2 Story 2.1 within minutes.

---

## Issue 1 — VERIFIED: Story 1.1 proto and generated Go structs are correct

**Claim checked:** `SessionAcknowledgedEvent` is field 7 in the `SessionEvent` oneof; generated
wrapper is `SessionEvent_SessionAcknowledged`.

**Finding:** Confirmed correct.

- `proto/session/v1/events.proto` line 21: `SessionAcknowledgedEvent session_acknowledged = 7;`
- `gen/proto/go/session/v1/events.pb.go` line 291: `type SessionEvent_SessionAcknowledged struct { SessionAcknowledged *SessionAcknowledgedEvent }`
- Fields `SessionId`, `AcknowledgedAt`, `Reason` all exist with correct names.
- `event_converter.go` switch currently has NO `EventSessionAcknowledged` case — the gap is real.

**The plan's code snippet is correct.** The field mapping `event.Context → Reason` is also correct:
`NewSessionAcknowledgedEvent(sessionID, reason string)` stores `reason` in `event.Context`
(`pkg/events/types.go` line 136).

**Classification:** VERIFIED

---

## Issue 2 — BLOCKER: Story 1.3 uses wrong method name `rqm.signalActivityCh()`

**Claim checked:** Plan says to call `rqm.signalActivityCh()` as the safety-net signal.

**Finding:** The method does not exist. The actual method is `rqm.signalActivity()` (no "Ch" suffix).

From `server/review_queue_manager.go` line 164:
```go
func (rqm *ReactiveQueueManager) signalActivity() { ... }
```

The plan's code snippet references `rqm.signalActivityCh()` — this is a nonexistent method. If an
implementer copies the snippet verbatim, the file will not compile.

**Locking concern (also checked):** `queue.Remove()` is safe to call from the event handler goroutine
concurrently with the reactive poll loop. `session.ReviewQueue` uses its own mutex (`rq.mu`) and
`Remove` acquires a write lock — no additional concern from Story 1.3's event handler.

**Fix required:** Replace `rqm.signalActivityCh()` with `rqm.signalActivity()` in the code snippet.

**Also note (VERIFIED):** `handleEvent` currently handles exactly `EventUserInteraction`,
`EventSessionAcknowledged`, and `EventApprovalResponse` — `EventSessionDeleted` is absent. The
gap G3 is real. Story 1.3 correctly identifies both the file and the switch.

**Classification:** BLOCKER (compile error on copy-paste; correctable with a one-word fix)

---

## Issue 3 — BLOCKER: Story 2.1 calls `approvalStore.RemoveBySession()` — method does not exist

**Claim checked:** Plan says "Check whether `approvalStore` has a `RemoveBySession(sessionID string)`
method. If not, add one."

**Finding:** `RemoveBySession` does not exist. The correct existing method is `CancelSession`.

From `server/services/approval_store.go` line 202:
```go
func (s *ApprovalStore) CancelSession(sessionID string) []string { ... }
```

`CancelSession` does exactly what the plan wants: denies all pending approvals for a session,
closes `decisionCh` channels (sends `ApprovalDecision{Behavior: "deny", Message: "Session
restarted"}`), and removes entries from the store. It returns the list of cancelled approval IDs.

The plan instructs implementers to add a new method, which would be duplicate work. If the plan is
followed without reading this note, the implementation will either correctly discover `CancelSession`
or incorrectly implement a redundant `RemoveBySession` alongside it.

**Scope impact:** Story 2.1 scope is smaller than the plan states. No new method needs to be added
to `ApprovalStore` or its interface. The implementer only needs to call
`s.approvalStore.CancelSession(sessionTitle)` in `DeleteSession`. Note: `approvalStore` is a field
on `SessionService` (line 85), so it is available in `DeleteSession`'s receiver scope — that part
of the plan is correct.

**Additional finding:** `CancelSession` sends a `"deny"` decision with the reason
`"Session restarted"` (not "Session deleted"). The message wording may need adjustment for the
deletion case; this is a cosmetic concern, not a functional one.

**Classification:** BLOCKER (plan describes adding a method that already exists under a different
name; story must be rewritten)

---

## Issue 4 — CONCERN: Story 2.2 `removeItem` description of existing code is accurate, but
"simpler alternative" conflicts with the delta-approach

**Claim checked:** Plan says `removeItem` uses `state.reviewQueue.totalItems - 1` (decrement
counter approach), and proposes fixing it.

**Finding:** Confirmed accurate. `web-app/src/lib/store/reviewQueueSlice.ts` lines 61–63:
```ts
const newTotal = Math.max(0, state.reviewQueue.totalItems - 1);
state.reviewQueue.totalItems = newTotal;
state.stats.totalItems = Math.max(0, state.stats.totalItems - 1);
```

The double-decrement bug is real. Both plan fix variants are mechanically correct.

**Subtle issue with the preferred "simpler" approach:** The plan's preferred alternative
(`state.reviewQueue.totalItems = state.reviewQueue.items.length`) diverges from how `totalItems` is
set elsewhere. `setReviewQueue` (line 44) sets `state.stats.totalItems = action.payload.totalItems`
(from the server), not `items.length`. If the server's `totalItems` ever differs from
`items.length` (e.g., the queue is paginated or filtered), syncing from `items.length` in
`removeItem` would diverge. The delta approach (`removed = before - after`) is safer and should
be the recommended fix, not the "simpler alternative."

**Classification:** CONCERN (plan is accurate about the bug; fix direction is correct but the
"prefer simpler" recommendation may introduce a new divergence if pagination is added later)

---

## Issue 5 — BLOCKER: Story 3.1 `removeDetectedStatus` adds a `subStatus` field that does not exist
on session entities in `sessionsSlice`

**Claim checked:** Plan says to clear `session.subStatus` in the new `removeDetectedStatus` reducer.

**Finding:** The `Session` entity in `sessionsSlice` has NO `subStatus` field. The entity shape
from `web-app/src/lib/store/sessionsSlice.ts` uses the proto-generated `Session` type directly
(`import { Session } from "@/gen/session/v1/types_pb"`). The slice manages `detectedStatusMap`
as separate extra state — a `Record<string, { detectedStatus; detectedContext }>` keyed by session
ID. There is no `subStatus` field on `Session` entities in the adapter.

The plan's code snippet:
```ts
const session = state.entities[action.payload];
if (session) {
  session.subStatus = undefined;  // or: session.needsApproval = false;
}
```

This would assign to a field that does not exist on the `Session` proto type, causing a TypeScript
compilation error. The correct approach is to only delete from `detectedStatusMap`:
```ts
removeDetectedStatus(state, action: PayloadAction<string>) {
  delete state.detectedStatusMap[action.payload];
}
```

The plan acknowledges "Adjust the field name to match the actual sessionsSlice entity shape" but
then provides a broken example. This will confuse implementers.

**Classification:** BLOCKER (the example code will not compile; correctable but the plan must
remove the `session.subStatus = undefined` example entirely and replace it with `delete
state.detectedStatusMap[action.payload]`)

---

## Issue 6 — CONCERN: Story 3.2 — `dispatch` IS available in `approvalResponse` case but routes
through a callback, not directly

**Claim checked:** Plan says to add `dispatch(removeDetectedStatus(sessionId))` in the
`approvalResponse` case of `handleSessionEvent`.

**Finding:** `dispatch` is available in `handleSessionEvent` — it is captured from `useAppDispatch()`
at the top of `useSessionService`. Lines 701–747 of `useSessionService.ts` show the switch with
the `approvalResponse` case at line 736. `dispatch` is in scope.

**However:** The `approvalResponse` case in `handleSessionEvent` currently routes via
`onApprovalResponseRef.current?.(approvalId, sessionId)`. The actual toast removal and
`refreshHistory()` calls happen in `SessionServiceContext.tsx` line 83–89. The plan's proposed
`dispatch(removeDetectedStatus(sessionId))` would go in `useSessionService.ts` directly — but
the `removeDetectedStatus` action has not been added to `sessionsSlice` yet (that's Story 3.1).

**Dependency check:** Story 3.2 correctly depends on Story 3.1. The plan's dependency graph shows
3.1 before 3.2 implicitly. This is correct.

**Classification:** CONCERN (no error, but the plan needs to be explicit that 3.1 must complete
before 3.2 can be implemented — they must land in the same PR or in order)

---

## Issue 7 — VERIFIED: Story 4.3 — `useReviewQueue` exists and has no reconnect logic

**Claim checked:** Does `useReviewQueue` exist? Does it have any reconnect logic?

**Finding:** `useReviewQueue` is at `web-app/src/lib/hooks/useReviewQueue.ts`. The stream setup
(lines 256–308) catches errors in a `try/catch` but on `AbortError` returns with no reconnect.
Non-`AbortError` failures print to console and exit silently — no retry loop. The plan's claim
that there is no reconnect logic is confirmed.

The plan's pseudo-code for a reconnect loop is directionally correct. One concern: the plan
references `subscribeToWatchReviewQueue({ initialSnapshot: true, signal: ... })` which is a
fictional function. The actual pattern would wrap the existing `clientRef.current.watchReviewQueue(...)`
call in a `while` loop. Implementers should be directed to mirror the `startStream` loop in
`useSessionService.ts` lines 763–819, not invent a new abstraction.

**Classification:** VERIFIED (gap is real; reconnect approach is correct but pseudo-code is
abstract)

---

## Issue 8 — CONCERN: Story 4.4 — `reviewQueueSlice` has NO `addItem` or `updateItem` reducers

**Claim checked:** Plan says "add `addItem` and `updateItem` actions if they do not already exist."

**Finding:** Current `reviewQueueSlice.ts` exports exactly: `setReviewQueue`, `setReviewQueueStats`,
`setLoading`, `setError`, `removeItem`. Neither `addItem` nor `updateItem` exists.

The plan correctly notes they may not exist, but the phrase "if they do not already exist" implies
they might — they definitely don't. Story 4.4 scope should be clear that adding both reducers is
mandatory, not conditional.

The plan's proposed implementations for `addItem` and `updateItem` are mechanically correct and
consistent with the existing slice shape. The guard `totalItems = items.length` in `addItem` is
the right approach (avoids the separate-counter drift bug fixed in 2.2).

**Classification:** CONCERN (no error; plan's hedging language may cause an implementer to skip
creating the reducers assuming they exist)

---

## Issue 9 — VERIFIED: Story 5.1 SSR guard — correctly identifies `output: "export"`, guard is
appropriate

**Claim checked:** Is the app SSR or static export? Is the SSR guard necessary?

**Finding:** `web-app/next.config.ts` line 9: `output: "export"`. This is a fully static Next.js
export — pages run in the browser only. There is NO server-side rendering at request time.

**However**, `output: "export"` still runs Next.js build-time code in Node.js (to generate the
static HTML). vanilla-extract `.css.ts` files run in Node during build. The plan's `typeof window
=== "undefined"` guard is appropriate: if any module that imports `broadcastChannel.ts` is
evaluated at build time (e.g., via a page component that's server-rendered during the export
pass), the guard prevents `new BroadcastChannel(...)` from crashing in the Node.js context where
`BroadcastChannel` does not exist.

**The plan's note** — "The `"use client"` directive in Next.js App Router is not applicable here
(this is a plain React + Vite/CRA app based on the project structure)" — is **incorrect**. This is
a Next.js app (`next.config.ts` exists, `"use client"` directives are visible in
`useReviewQueue.ts` line 1). The guard is still correct behavior, but the rationale in the plan
misstates the app's framework identity.

**Classification:** CONCERN (the guard code is correct; the explanatory comment in the plan
incorrectly identifies the framework, which could mislead future maintainers)

---

## Issue 10 — VERIFIED: Dependency order Epic 4 → Epic 3 is correctly stated

**Claim checked:** Does Epic 4's `selectReviewQueueItemsWithLiveStatus` selector depend on
`removeDetectedStatus` (Story 3.1) being implemented first?

**Finding:** The selector in Story 4.1 reads `detectedStatusMap` from `sessionsSlice` state.
`detectedStatusMap` already exists in `sessionsSlice` — it is populated by the existing
`upsertSession` reducer. The selector does NOT depend on `removeDetectedStatus` existing. It joins
queue items with `sessionsSlice.entities` using the existing `detectedStatusMap`.

However, the plan's stated dependency ("Epic 4 depends on Epic 3") is still correct for a different
reason: without Story 3.1's `removeDetectedStatus` action, `detectedStatusMap` will never be
cleared on approval/acknowledgement, so the joined selector would always return stale "Needs
Approval" state even after the event. Epics 3 and 4 are jointly required for the feature to work
end-to-end.

The dependency graph in the plan (`Epic 3 → Epic 4`) is correct.

**Classification:** VERIFIED

---

## Issue 11 — NEW FINDING: Story 1.2 `InteractionType` field name mismatch in event struct

**Gap found independently:** Story 1.2 says to "map `event.InteractionType` string to the
`sessionv1.UserInteractionEvent_InteractionType` enum." This is correct — `event.InteractionType`
(field name confirmed in `pkg/events/types.go` line 57) does carry the interaction type string.

**However:** `UserInteractionEvent` in proto uses `type` as the field name (not `interaction_type`).
Generated Go accessor: `GetType()` returns a `UserInteractionEvent_InteractionType` enum. The plan's
code snippet maps `event.InteractionType` (a string) to the enum — but the snippet leaves the
mapping as a comment (`// map event.InteractionType string to ... enum value here`). This is the
right approach (emit `INTERACTION_TYPE_UNSPECIFIED` if mapping is unknown) but the implementer
needs to look up the enum values, which are fully defined in the proto:

```
INTERACTION_TYPE_UNSPECIFIED = 0
INTERACTION_TYPE_TERMINAL_INPUT = 1
INTERACTION_TYPE_APPROVAL_GIVEN = 2
...
```

The string values used in `NewUserInteractionEvent` calls across the codebase need to be mapped.
Without this mapping, all `UserInteractionEvent` proto emissions will have `type = UNSPECIFIED`.
This is acceptable per the plan ("Use INTERACTION_TYPE_UNSPECIFIED if mapping is unknown") but the
plan should flag this as a follow-up task, not leave it implicit.

**Classification:** CONCERN (plan correctly flags it as a known gap; should be explicitly tracked
as a task not an inline comment)

---

## Consolidated Issue List by Severity

### BLOCKERS (plan is wrong; must fix before implementation)

| # | Issue | Fix Required |
|---|---|---|
| 2 | Story 1.3 code snippet calls `rqm.signalActivityCh()` — method does not exist | Replace with `rqm.signalActivity()` |
| 3 | Story 2.1 says to add `RemoveBySession` — method already exists as `CancelSession` | Rewrite story to call `s.approvalStore.CancelSession(sessionTitle)` |
| 5 | Story 3.1 code snippet mutates `session.subStatus` — field does not exist on `Session` proto type | Remove `session.subStatus = undefined` example; reducer should only `delete state.detectedStatusMap[id]` |

### CONCERNS (risky or misleading; address before implementation)

| # | Issue | Risk |
|---|---|---|
| 4 | Story 2.2 "simpler" alternative (`totalItems = items.length`) may diverge if pagination added | Use delta approach as primary recommendation |
| 6 | Story 3.1 and 3.2 must land in order (3.2 imports action from 3.1) | Make ordering explicit; suggest single PR for Epic 3 |
| 7 | Story 4.3 reconnect pseudo-code is abstract | Direct implementer to mirror `startStream` pattern from `useSessionService.ts` |
| 8 | Story 4.4 "if they do not exist" language implies `addItem`/`updateItem` may exist — they don't | Replace with "add the following new reducers" |
| 9 | Story 5.1 says this is "not a Next.js App Router app" — it is Next.js with `output: export` | Correct the explanatory comment; the guard code itself is correct |
| 11 | Story 1.2 `InteractionType` string→enum mapping left as inline comment | Create explicit follow-up task |

### VERIFIED (checked and correct)

| # | Item |
|---|---|
| 1 | Story 1.1: proto field 7 exists, generated wrapper name correct, event.Context→Reason mapping correct |
| 1b | Story 1.1: `EventSessionAcknowledged` case is genuinely absent from `event_converter.go` |
| 1.3 | Story 1.3: `EventSessionDeleted` is genuinely absent from `handleEvent` switch |
| 6 | Story 3.2: `dispatch` is in scope in `handleSessionEvent`'s `approvalResponse` case |
| 7 | Story 4.3: `useReviewQueue` exists at correct path; no reconnect logic present |
| 10 | Dependency graph Epic 3 → Epic 4 is correct |

---

## Overall Verdict: CONCERNS

The plan is BLOCKED from copy-paste implementation by three specific errors (issues 2, 3, 5) but
the overall architecture and dependency ordering are sound. Fixing the three blockers and
addressing the six concerns will make the plan safe to hand to an implementer.

The highest-risk fix is **Issue 3 (CancelSession)**: an implementer who blindly adds
`RemoveBySession` would create dead code alongside an existing method that already does the job.
The second-highest risk is **Issue 5 (subStatus)**: the TypeScript compile error would be
immediate and confusing.

No changes to the epic structure, dependency graph, or architectural approach are required.
