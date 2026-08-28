# Requirements: pr-fix-steering

**Date**: 2026-08-26
**Type**: feature addition (extension of existing autonomous reconciliation loop)
**Complexity**: 2 — focused feature; targeted extension of an existing, well-tested mechanism (not new infrastructure), consistent with the sibling `pr-review-followup` project's scoring for the same reconciliation loop.

## Problem Statement

`ReconcilePRPending` already detects when an open backlog PR needs work — `GetPRStatus`
(`session/git/worktree_git.go:500`) reports `CIFailing`, `HasBlockingReviews`, and
`HasConflicts` — and `AutoReopenForPRFix` (`server/services/backlog_service_triage.go:2018`)
builds a `fixContext` string describing the problem and spawns a **new** fix session for it.
But when `findActiveWorkSession` (`backlog_service_triage.go:1253`) finds a work session
already running for that item, `AutoReopenForPRFix` takes a different branch
(`backlog_service_triage.go:2048-2051`): it calls `notifyRespawnBlockedByActiveSession` (a
log line + a stuck-reason notification) and returns — the running session itself is never
told what's broken or what to do about it. The human has to notice the notification and
manually paste instructions (e.g. "run `/github:pr-ship`") into the live session.

The steering primitive to fix this already exists and is already used elsewhere:
`UpdateSession`'s `SteerMessage` handling (`server/services/session_service.go:2929-2972`)
injects a message into a running session, branching on autonomous (`ClaudeController.
SendCommandImmediate`) vs. interactive (`Instance.SendKeys`) sessions. It is simply not
wired into the reconciler's blocked-respawn path.

## Baseline

Today, when a backlog PR develops a problem (CI failure, blocking review, or merge
conflict) while its work session is still active:
- `AutoReopenForPRFix` logs `[AutoReopenForPRFix] item %s already has an active session
  %s; skipping respawn` and records a `StuckReasonRespawnBlockedActive` notification
  (`backlog_service_triage.go:1363`, `notifyRespawnBlockedByActiveSession`).
- The running session receives no signal. It keeps working on whatever it was already
  doing, oblivious to the new CI failure/review/conflict, until either it happens to
  re-check the PR itself, or a human reads the notification and manually steers it (via
  the UI's steer box or `steer_session`/`send_control`).
- This is the exact gap the user named: the harness *detects* the problem but does not
  *act* on it when a session is already there to receive the information.

## Users / Consumers

- `BacklogService`'s reconciliation loop (`ReconcilePRPending` → `AutoReopenForPRFix`) is
  the direct caller of the new behavior.
- Indirect beneficiary: Tyler (solo operator) — no longer has to notice a
  `respawn_blocked_active` notification and manually retype what's broken into a live
  session.

## Success Metrics

- When `AutoReopenForPRFix` finds an active work session for a `pr_pending` item with
  `CIFailing`/`HasBlockingReviews`/`HasConflicts` true, that session receives a steer
  message describing the specific problem(s) and pointing at the fix path (e.g.
  `/github:pr-ship` for Claude Code sessions) within one reconcile tick — no human
  paste-in required.
- If the failure *reason* changes while the session is still active (e.g. conflicts
  resolved but CI now failing), the session receives an updated steer reflecting the new
  reason — this is not a fire-once-forever guard.
- An identical, already-delivered reason does **not** re-steer on every ~60s poll tick
  (no spam).
- Every steer attempt (success or failure) is visible in the backlog item's activity/
  notification trail, not just a log line — consistent with the standing project
  preference that self-heal/auto-act paths must never act silently.

## Appetite

Medium (1–2 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

This bucket was chosen deliberately at ideation, not defaulted from the sibling `pr-review-followup` project's Small (1–3 days) rating for the same reconciliation loop — see `implementation/plan.md`'s Unresolved Questions for the full reconciliation (task/story-count growth vs. raw-effort growth, and why the review-driven scope growth beyond this baseline doesn't change the bucket).

## Constraints

- Must follow the existing consumer-defined-interface DI pattern already established for
  this exact kind of cross-package call: `SessionStopper` (`server/services/
  backlog_service.go:47`, wired via `SetSessionStopper`) is the precedent — a new
  `SessionSteerer`-shaped interface, implemented by `SessionService` and injected the same
  way, avoids a circular import from `server/services` back into itself and avoids
  reinventing DI wiring `server/dependencies.go` already does for `SessionStopper`.
- Must reuse the existing autonomous-vs-interactive branch logic
  (`session_service.go:2934-2971`) rather than reimplement `SendKeys`/`ClaudeController`
  handling a second time.
- Steering content must degrade gracefully for non-Claude-Code sessions: a literal
  `/github:pr-ship` instruction only means anything to a Claude Code session. Must check
  `instance.Program` before including a slash-command instruction; other programs (e.g.
  Aider) get plain-English problem description only.
- No new user-facing configuration surface — this is an internal reconciliation-loop
  behavior change, not a new backlog/session setting.

## Non-functional Requirements

- **Performance SLO**: not specified — this runs inside the existing ~60s reconcile tick;
  added work is one steer call per affected item per tick, negligible.
- **Scalability**: not applicable — bounded by the number of concurrently active,
  problem-flagged `pr_pending` items (currently single-digit).
- **Security classification**: internal — no new external surface; reuses existing
  session-injection code paths already reachable via the UI and MCP tools.
- **Data residency**: no special requirements.
- **Accessibility**: partially covered by static scanning, with a targeted dynamic test for the one risk class static scanning can't reach. This project's two web-app touchpoints (the new `StuckReasonSteerFailed` `BlockerChip` label/icon and the "Give Direction" dialog's new error-toast/in-flight-disable path) both reuse existing components (`BlockerChip`, `useNotifications()`'s toast) that are already subject to this repo's Axe Core CI gate (see root `CLAUDE.md`'s "UX analysis CI" section, which blocks PRs touching `web-app/src/` on WCAG AA violations) — that gate covers markup/ARIA correctness for both touchpoints. Axe Core is a static analyzer, though: it cannot catch a runtime keyboard-focus/auto-blur interaction bug, such as the one found and fixed in Story 1.1.3 (disabling the focused steer-message input on `isSteering` would otherwise let the browser auto-blur focus to `document.body`, breaking both the Escape handler and `useFocusTrap`'s Tab-cycling). Coverage for that specific risk class comes from the real-browser Playwright test added in Task 1.1.3d (`tests/e2e/session-actions-steer-focus.spec.ts`), not from the CI gate — dynamic keyboard-focus interactions generally are not covered by Axe Core and would need a similar per-case test if a future touchpoint introduces one.

## Scope

### In Scope
- A `SessionSteerer` interface on `BacklogService` (mirroring `SessionStopper`), backed by
  `SessionService`'s existing autonomous/interactive steer branching, injected via a new
  `SetSessionSteerer` call from `server/dependencies.go`.
- Replacing (or extending) the `findActiveWorkSession`-blocked branch inside
  `AutoReopenForPRFix` so it steers the active session with the same problem detail
  `fixContext` already computes, instead of only logging + notifying.
- A change-detection/cooldown mechanism keyed on the PR's failure-reason signature
  (`CIFailing`/`HasBlockingReviews`/`HasConflicts` tuple, plus a coarse conflict/CI detail
  string) so repeated identical reasons don't re-steer every tick, but a changed reason
  does.
- A visible backlog-item comment/notification recording each steer attempt and outcome,
  extending the existing `notifyRespawnBlockedByActiveSession` audit trail rather than
  replacing it.
- Unit tests for the new branch, following the existing
  `TestAutoReopenForPRFix_ActiveWorkSession_*` naming convention, plus tests for the
  dedup/change-detection logic in isolation (pure-function style, matching
  `nudge_dedup.go`'s `isDuplicateNudge`/`nextLastNudge` test pattern).

### Out of Scope
- Extending the same treatment to `AutoRespawnReview`'s or `AutoRespawnAutonomousWork`'s
  own active-session-skip branches (review rework, generic autonomous respawn) — not
  requested; candidate future follow-up, noted below.
- New UI for steer history — existing notification/activity-log surfaces already show
  backlog item events.
- Redesigning `steer_session` (MCP) or `UpdateSession`'s `SteerMessage` handling — this
  project is a new *caller* of that existing machinery, not a change to it.
- Cross-agent slash-command support (teaching Aider or other non-Claude-Code programs to
  run an equivalent of `/github:pr-ship`).

## Rabbit Holes

- **Re-steer cadence.** Too naive ("steer every tick while the condition holds") spams the
  session every ~60s; too naive the other way ("steer once ever per item") silently stops
  helping after the first shot even when the reason changes. Needs an explicit
  reason-signature comparison, not a boolean "already steered" flag.
- **Program-awareness.** Sending a Claude-Code-specific slash command into an Aider
  session's PTY would just look like garbage input typed at its prompt.
- **Interrupting in-progress work.** A steer message arriving mid-turn could land while
  the agent is mid-generation or mid-tool-call. The existing `SendKeys`/
  `SendCommandImmediate` paths are already used for browser- and MCP-originated steering
  today, so this is likely already a solved problem — but should be explicitly confirmed
  in research rather than assumed, since this project adds a *new, automated* caller of
  that path (previously only human-triggered).
- **Redundant nudge detection.** No existing signal distinguishes "the active session
  already knows about this exact CI failure and is working on it" from "it has no idea."
  Without this, every reason-signature change re-steers even if the agent's own recent PTY
  output already shows it noticed. Research should check whether a pane-snapshot check
  similar to `nudge_dedup.go`'s re-arm logic is warranted, or whether that's overkill given
  the coarser reason-signature dedup already covers the common case.

## Alternatives Considered

- **Status quo (skip + notify only).** Rejected — this is the exact gap being closed; a
  human must currently notice and manually paste instructions.
- **Kill and respawn a fresh session even when one is active.** Rejected — defeats the
  purpose of the existing `findActiveWorkSession` guard, which was specifically built to
  stop a churning transition/respawn cycle that clobbered a legitimately in-progress
  multi-hour session (see the guard's own comment at `backlog_service_triage.go:2037-2046`
  citing a live incident in `docs/tasks/backlog-feature-improvement.md`).
- **Notify only, richer content, still no PTY injection.** Rejected — defeats the
  "fully autonomous" goal `AutoReopenForPRFix` already establishes for the no-active-
  session case; would reintroduce a manual step this project exists to remove.

## Feasibility Risks

- `SendKeys`/`SendCommandImmediate` reliability against a currently-busy session needs
  confirming from existing tests/behavior (`session_service_test.go`'s
  `TestUpdateSession_SteerMessage_*` cases) rather than assumed to always succeed cleanly.
- Circular-import risk if the new interface isn't scoped the same way `SessionStopper` is
  — mitigated by following that exact precedent (interface lives in the consumer package,
  `server/services`, not in `session`).

## Observability Requirements

- One structured log line per steer attempt, success or failure, matching the existing
  `[AutoReopenForPRFix]` prefix convention already used throughout this file.
- A visible backlog-item comment/notification per delivered steer (what was detected, what
  was sent), per the standing project preference that self-heal/auto-act paths must post a
  visible record rather than act silently.

## Risk Control

- No feature flag — this extends code (`AutoReopenForPRFix`) that already runs
  unconditionally in production; rollback is a plain revert.
- Blast radius is bounded by the reason-signature dedup (Rabbit Holes above): a bug in the
  new path can, at worst, send one redundant steer message per reason change per item —
  not an unbounded spam loop — because the dedup check gates every call.

## Open Questions

- Should the reason-signature cooldown reuse `nudge_dedup.go`'s existing
  `nudgeCooldown = 3*time.Minute` constant, the reconciler's own ~60s tick, or a distinct
  constant tuned for this path? Defer to planning phase with a citation-backed rationale,
  matching how `maxReworkBlockStaleness` (`backlog_service_triage.go:1289`) documents why
  its 15-minute threshold is distinct from neighboring thresholds.
- Is extending the same active-session-steer treatment to `AutoRespawnReview` worth a
  follow-up project once this one ships? Not blocking — noted as likely future work only.
