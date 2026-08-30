# ADR-001: Steer From the Backlog View by Widening `UpdateSession.steer_message`, Not by Adding an RPC

**Status**: Accepted
**Date**: 2026-08-12
**Project**: backlog-operator-feedback-loop (Gap 2, AC6/AC7)
**Supersedes / relates to**: nothing — this is the first decision on browser-originated
steering of non-autonomous sessions.

## Context

AC7 requires the new backlog-detail Steer affordance to "use the existing `steer_session`
path — no parallel steering implementation." Research (`research/architecture.md` §3.1,
`research/stack.md` sub-feature 2, `research/ux.md` Gap 2) found that "the existing path"
is not one thing — there are two, and the browser can only reach one of them:

| | MCP tool `steer_session` | RPC `UpdateSession.steer_message` |
|---|---|---|
| Handler | `server/mcp/tools_terminal.go:627-706` | `server/services/session_service.go:2011-2033` |
| Reachable from browser JS? | **No** — MCP transport, agent-only | Yes (`useSessionService.updateSession`) |
| Gate | none | `if !instance.AutonomousMode { return FailedPrecondition }` (`session_service.go:2013-2016`) |
| Send primitive | `inst.RunWithResume` (Stopped OneShot + conversation UUID) else `inst.SendKeys(msg+"\r")` (`tools_terminal.go:663-699`) | `instance.GetController().SendCommandImmediate(msg+"\r")` |

Backlog-linked work and review sessions are never spawned with `AutonomousMode: true`
(`server/services/backlog_service_triage.go` has no `AutonomousMode` wiring at
spawn-from-item time; autonomous mode is the separate opt-in "Fix Autonomously" feature).
So the only browser-reachable steer RPC rejects, with `FailedPrecondition`, exactly the
sessions AC6 targets. Shipping the affordance against the RPC unchanged would produce a
button that always fails — the precise "affordance that has no effect" outcome
`research/ux.md` names as worse than no button at all.

## Decision

**Widen the existing `UpdateSession.steer_message` handler to fall back to
`Instance.SendKeys(msg + "\r")` when `instance.AutonomousMode` is false**, instead of
returning `FailedPrecondition`. No new RPC, no new proto field, no change to the general
session list's UI.

```go
// server/services/session_service.go, replacing the current lines 2012-2033
if req.Msg.SteerMessage != nil && *req.Msg.SteerMessage != "" {
    if instance.AutonomousMode {
        // Unchanged: autonomous sessions keep the ClaudeController command-queue path.
        // (existing controller != nil / SendCommandImmediate / log-on-failure block)
    } else {
        // New: non-autonomous, Instance-backed sessions get the same PTY send primitive
        // the MCP steer_session tool already falls back to (tools_terminal.go:690).
        // Unlike the autonomous branch, a send failure is returned to the caller so the
        // UI can surface it (research/ux.md's Gap 2 error-state table).
        if err := instance.SendKeys(*req.Msg.SteerMessage + "\r"); err != nil {
            return nil, connect.NewError(connect.CodeFailedPrecondition,
                fmt.Errorf("failed to steer session %q: %w", instance.Title, err))
        }
    }
    // Both branches publish the existing "Steering input sent" NotificationEvent.
}
```

Both browser and MCP paths now converge on the same send primitive
(`Instance.SendKeys`, `session/instance_tmux.go:628`) for the ordinary case, which is what
AC7's "no parallel steering implementation" means in practice.

### Explicitly deferred: the `RunWithResume` branch

The MCP tool also steers a **Stopped** `OneShot` session that has a stored Claude
conversation UUID by launching a `claude --resume` subprocess with a 5-minute timeout
(`tools_terminal.go:667-681`). **This branch is deliberately NOT ported into
`UpdateSession`.** `UpdateSession` is a general-purpose unary mutation RPC — rename, tags,
program change, autonomous-mode toggle, and pause/resume all route through the same handler
and are processed sequentially in one call. Blocking it for up to five minutes on a
subprocess would change the latency contract of every other field update a caller batches
into the same request, and there is no partial-success shape for a unary handler that has
already applied three other field mutations before hanging on the fourth.

Consequence: a Stopped session is **not** steerable from the backlog view in v1. The UI
pre-computes this (see ADR-002) and renders the control disabled with an explanatory
`title` rather than letting the operator click into a guaranteed failure. Resurrecting a
finished session with new direction remains available via the MCP tool and via the item's
own re-trigger actions.

## Alternatives Considered

1. **A new dedicated RPC (e.g. `SteerSession(session_id, message)`) mirroring the MCP tool
   1:1.** Rejected: it creates a second browser-reachable steer surface with its own
   proto message, generated bindings, registry entry, and error contract — literally the
   "parallel steering implementation" AC7 forbids — to avoid a four-line change to a
   handler that already has the field, the instance lookup, the notification publish, and
   the frontend hook (`useSessionService.updateSession`, `useSessionService.ts:302-333`)
   in place. `research/stack.md` leaned this way on blast-radius grounds; that reasoning
   assumed touching `UpdateSession` meant touching the *general session list's* behavior,
   which it does not — the list's menu item stays gated on `session.autonomousMode`
   (`SessionActionsOverflow.tsx:723`), so no existing UI reaches the widened branch.

2. **Relax only the frontend gate (`SessionActionsOverflow.tsx:723`) and reuse the
   existing dialog.** Rejected on evidence, not preference: the server-side
   `FailedPrecondition` at `session_service.go:2013` is independent of the UI gate, so
   this produces a visible button that fails on click for every non-autonomous session.
   It also expands the general session list's menu surface, which requirements.md places
   out of scope ("the general session list's Steer dialog is unchanged").

3. **Set `AutonomousMode: true` on backlog-spawned work sessions so the existing path
   applies.** Rejected: `AutonomousMode` is not a steering capability flag — it starts an
   `AutonomousDriver` loop (`session/instance.go:552-554`) that changes how the session
   runs. Flipping it to unlock a UI affordance would silently change agent behavior for
   every backlog work session, a far larger and entirely unrelated behavior change.

## Consequences

- One backend file changes (`server/services/session_service.go`), additively; the
  autonomous branch's behavior — including its existing log-only-on-send-failure
  handling — is untouched.
- The two branches now have **asymmetric error contracts**: autonomous send failures are
  logged and the RPC still returns success; non-autonomous send failures return
  `FailedPrecondition`. This asymmetry is deliberate for this pass (the new branch needs a
  real error for the new UI; changing the old branch's contract would alter behavior the
  existing autonomous UI depends on) and is recorded in plan.md's Unresolved Questions as
  a follow-up to unify, not as an oversight.
- No proto change, no `make proto-gen` needed for Gap 2, no new registry-backend entry
  (`session:update` already exists) — only a `testIds`/`lastModified` touch on the
  existing entry.
- A steer message remains fire-and-forget: "sent" is not "acted upon". The UI copy says
  "Sent" rather than "Applied", carrying forward the same expectation-setting the existing
  autonomous dialog uses (`research/ux.md` Gap 2, JTBD note 3).
