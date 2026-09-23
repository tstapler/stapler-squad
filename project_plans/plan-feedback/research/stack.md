# Research: Stack (Agent 1)

## Verdict

No new dependencies needed. This feature is implementable entirely with
libraries/frameworks already in `go.mod` and `web-app/package.json`, following
patterns that already exist in this codebase (`PlanVerdictBox.tsx`'s reject-reason
textarea, `JulesDispatchDialog.tsx`'s feedback-capture dialog).

## Backend (Go)

- **Go**: 1.26.6 (`go.mod`)
- **RPC framework**: `connectrpc.com/connect v1.20.0` (`go.mod`)
- **ORM**: `entgo.io/ent v0.14.5` (`go.mod`) — `session/ent/schema/` is hand-written,
  generated output is gitignored; any new persisted field goes through
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  per this repo's `CLAUDE.md`.
- Relevant handlers already exist and need no new package:
  - `TriggerTriage` — `server/services/backlog_service_trigger_triage.go`
  - `TransitionBacklogItemStatus` / `RejectPlan` — `server/services/backlog_service_lifecycle.go`

### Existing proto message shapes (`proto/session/v1/backlog.proto`)

```protobuf
message TransitionBacklogItemStatusRequest {
  string item_id = 1;
  string target_status = 2;
  string expected_status = 3;
  google.protobuf.Timestamp expected_updated_at = 4;
  string override_reason = 5;
}
message TransitionBacklogItemStatusResponse {
  BacklogItem item = 1;
}

message TriggerTriageRequest {
  string item_id = 1;
  // feedback, if non-empty, requests a refinement of the item's most recent
  // completed triage result instead of a fresh triage run. Requires a prior
  // completed triage result to exist.
  string feedback = 2;
  bool chat_mode = 3;
}
message TriggerTriageResponse {
  ItemSession item_session = 1;
}

message RejectPlanRequest {
  string item_id = 1;
  // reason is required free-text feedback explaining what should change,
  // mirroring TriggerTriageRequest.feedback's refinement-input pattern.
  string reason = 2;
}
message RejectPlanResponse {
  BacklogItem item = 1;
}
```

(Line numbers: `TransitionBacklogItemStatusRequest` at
`proto/session/v1/backlog.proto:651`, `TriggerTriageRequest` at `:693`,
`RejectPlanRequest` at `:719`; RPC declarations at `:1383`, `:1392`, `:1403`.)

**Key implication for Phase 3 planning**: `TriggerTriageRequest.feedback` is
already the exact "what to change" channel `RejectPlan`'s "Regenerate Plan with
This Feedback" button uses today (`useBacklogService.ts` calls
`triggerTriage({ itemId, feedback })` — see `:985`). No new proto field is
needed on `TriggerTriageRequest` itself for the feedback text. What's still
undecided (Phase 3, per requirements.md's Rabbit Holes) is:
1. Whether `TransitionBacklogItemStatusRequest.override_reason` (already a
   plain string, no schema change needed) or a new dedicated field is used to
   carry the "why sending back" text if the flow does a transition-then-
   trigger-triage two-call sequence from one UI action.
2. Whether the one-click submit does `TransitionBacklogItemStatus` then
   `TriggerTriage(feedback=...)` back-to-back (no proto changes at all, reusing
   both requests exactly as shaped today), or needs a combined RPC (would be
   a genuinely new message — out of proportion per the Appetite section).

No proto regeneration is required unless Phase 3 decides a new field/RPC is
needed; if it does, `make proto-gen` is the existing regen command.

## Frontend (web-app/)

- **Framework**: Next.js `15.3.2`, React `^19.0.0` / `react-dom ^19.0.0`
  (`web-app/package.json`)
- **Language**: TypeScript `^5.9.3`
- **RPC client**: `@connectrpc/connect ^2.1.1` + `@connectrpc/connect-web ^2.1.1`,
  proto messages via `@bufbuild/protobuf ^2.11.0` (generated client:
  `@/gen/session/v1/backlog_pb`)
- **State management**: no Redux/Context for this surface — `@reduxjs/toolkit`
  is present in `package.json` but the backlog detail surface uses a plain
  custom hook, `web-app/src/lib/hooks/useBacklogService.ts`, holding local
  `useState`/`useCallback` and exposing action functions
  (`transitionStatus`, `triggerTriage`, `rejectPlan` — all already present,
  at lines `929`, `981`, `1020` respectively) plus derived fields like
  `planRejectionReason`/`planRejectionTime` (`:144-152`). A new "send back
  with feedback" action should be added here as a sibling method, not a new
  state layer.
- **Form/validation library**: none (no `react-hook-form`, no `zod`,
  no `formik` in `package.json`). Every existing feedback-capture surface
  uses a raw controlled `<textarea>` + `useState`, e.g. `PlanVerdictBox.tsx`
  (`reason`/`showReject` state, `:80-97`, `:214-216`) and
  `JulesDispatchDialog.tsx` (`prompt` state + `<textarea className={textarea}>`,
  `:91`, `:208-210`). Follow this pattern — no new library.
- **Modal/dialog primitive**: `@radix-ui/react-dialog ^1.1.15` is a
  dependency but the two most relevant precedents in this exact component
  family do *not* use it:
  - `PlanVerdictBox.tsx` is an **inline expand/collapse box** (a `showReject`
    boolean toggles a plain `<div>` block in place, no portal/modal) — this
    is the closer precedent since it's the same "reject a plan with reason"
    interaction this feature extends.
  - `JulesDispatchDialog.tsx` is a **custom `createPortal`-based dialog**
    (not Radix `Dialog`) with its own focus trap (`useFocusTrap` hook,
    `web-app/src/lib/hooks/useFocusTrap.ts`) and vanilla-extract CSS module.
  Phase 3 should pick between "inline box on the Actions section" (mirrors
  `PlanVerdictBox`, simplest, no new focus-trap code) or "dialog" (mirrors
  `JulesDispatchDialog`, better for a longer form); either is buildable with
  already-used primitives, no new dependency either way.
- **Styling**: vanilla-extract (`@vanilla-extract/css ^1.20.1`,
  `@vanilla-extract/recipes ^0.5.7`, `@vanilla-extract/next-plugin ^2.5.1`) —
  every sibling component in `web-app/src/components/backlog/detail/` has a
  matching `ComponentName.css.ts`; a new component/section should follow
  the same `*.css.ts` colocation.

### Existing UI wiring to extend

- `web-app/src/lib/backlog/itemActions.ts`: defines the `send_back_idea` /
  `send_back_ready` action-name union (`:40-41`) and the status→action
  eligibility sets `CAN_SEND_BACK_IDEA`/`CAN_SEND_BACK_READY` (`:114`, `:122`).
  `send_back_refining` (named in requirements.md as dead code) is **not**
  defined anywhere in this file — confirms requirements.md's finding #2 that
  no `CAN_SEND_BACK_REFINING` set exists, so the button can never appear.
- `web-app/src/components/backlog/detail/ActionsSection.tsx`: renders the
  `send_back_idea` ("↩ Return to Triage", `:428-437`) and `send_back_ready`
  ("↩ Back to Ready", `:440-449`) buttons, gated on `actions.has(...)`.
- `useBacklogService.ts`'s `triggerTriage` signature already accepts optional
  feedback: `triggerTriage: (id: string, feedback?: string) => Promise<{ itemSessionId: string } | null>` (`:691`), calling
  `clientRef.current.triggerTriage({ itemId: id, feedback: feedback ?? "" })` (`:985`).

## Length cap

Requirements.md specifies matching `maxRejectReasonLength = 10000`
(`backlog_service_lifecycle.go:892`) for the new feedback field — reuse that
constant/value rather than inventing a new limit.

## Summary of what's genuinely new vs. reused

| Piece | New? |
|---|---|
| Go RPC framework, ent ORM, proto toolchain | Reused as-is, no version bump |
| `TriggerTriageRequest.feedback` channel | Reused as-is |
| `TransitionBacklogItemStatusRequest.override_reason` | Reused as-is (if Phase 3 routes feedback through it) |
| React/Next/TS/connect-web/vanilla-extract | Reused as-is |
| Textarea + local `useState` feedback-capture pattern | Reused (copy `PlanVerdictBox`/`JulesDispatchDialog` pattern) |
| `useBacklogService.ts` action method for "send back with feedback" | New method, same file, same conventions as `transitionStatus`/`triggerTriage`/`rejectPlan` |
| New proto message/field | Only if Phase 3 decides existing fields (`feedback`, `override_reason`) are insufficient — not expected per Appetite |
