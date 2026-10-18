# ADR-004: Inline actions are risk-tiered; keystroke-writing actions stay localhost-only until the remote chain is reviewed

**Status**: Proposed (2026-10-17) - requires owner confirmation (see plan Unresolved Questions); triad round 2: this ADR becomes Accepted when the owner-defaults list in plan `Unresolved Questions` is confirmed at the Phase 5 start gate (one explicit accept step for all eight ADRs; until then the stated defaults proceed)
**Project**: notification-actions-and-agent-messaging
**Flag**: `notification_inline_actions` (per-scope `channel:remote`)

## Context

Reply to a pending question is localhost-only today (`EvaluateLocalWrite`, `server/services/hidden_question_reply.go`), so an operator on a phone over the private-CA remote port `:8444` cannot use it. Mobile is a stated requirement and "answer from the phone" is the highest-value remote use. The local listener `:8543` has no authentication (`SetupAuth` has no non-test caller; ADR-010 context); the `:8444` chain has auth and a DNS-rebinding Host allowlist (ADR-010 decision 7). The guard that actually protects a keystroke is `DialogMatch`, not caller authentication. ADR-010 had a review blocker for claiming `:8543` calls are "authenticated".

## Decision

Classify every `ActionKind` into a tier in the descriptor:

- **Tier A, data-only** (names are the `ActionKind` enum values): `ANSWER_QUESTION` for a durable GuidanceRequest in a non-live route, `ANSWER_GUIDANCE` (a data write through `AnswerGuidanceRequest`; no pane write), `BACKLOG_DECISION`, `APPROVE`/`DENY` (already available remotely today through `ResolveApproval` - parity preserved), `CANCEL_WAKE` and `CANCEL_PEER_HELP` (row updates only), `OPEN`. Allowed on both listeners. Story 2.8 asserts each on the remote chain.
- **Tier B, writes bytes into a live pane**: `ANSWER_QUESTION` with route `live_keystroke`, any steer. Allowed on `:8543` only in v1. On the remote listener the descriptor is emitted with `tier=B` and `available=false, reason="desktop_only"`; the UI shows "Answer from desktop" with Open session. The durable answer record route (saved for later / hook reply if ADR-005 selects it) remains Tier A, so a phone can still answer a question whose delivery is not a keystroke.
- Tier B on the remote listener is promoted only after spike S6 passes (review of the `:8444` chain on the exact handlers: auth, Host allowlist, request-origin checks, no CSRF-able GET). Promotion is a per-scope live flag override (`notification_inline_actions` scope `channel:remote:tierB`), not a code change.

## Consequences

- Phone users get approve/deny/backlog decisions and durable answers in v1; live-pane keystrokes remain desktop-only with explicit copy rather than a silent failure.
- No claim of operator authentication on `:8543` anywhere in code, docs or UI copy.
- **Residual risk (added after adversarial review)**: Tier B on `:8543` rests on caller location, not identity; any local process can invoke it, bounded only by `DialogMatch`. Extending Tier B from hidden sessions to **visible** sessions widens the blast radius to panes where the human may be typing. Mitigation: a visible-session `live_keystroke` answer is refused (`NOT_SENT`, reason `human_active`) when `HumanActivity` is within 5 s; this is an AC of Story 3.4 (Task 3.4f). **The signal is defined (pre-mortem #16)**: under tmux control mode an ssq client is always attached, so attachment says nothing. `HumanActivity` is the latest of (a) the last browser-originated pane input recorded per session in `paneWriter.SendInput` (`server/services/terminal_access.go:140`, the sink of both web-terminal stream handlers `server/services/connectrpc_websocket.go` and `server/services/session_service_stream_terminal.go`) and (b) `#{client_activity}` of any attached tmux client whose `#{client_control_mode}` is 0, i.e. a human's own `tmux attach` (both format variables are in the tmux 3.6a manual; live behavior is verified by Spike S7 item (10)). A `list-clients` failure drops signal (b) only. Input through ssq-mux or other writers is not covered and is listed as a residual risk. Resume scheduled prompts (a flag write) is available on the local listener only. The listener-identity for descriptors is a `ListenerClass` derived from the request context and passed into the assembler (Task 2.2d).
- Slightly more descriptor surface (`tier`, `available`, `reason`).

## Alternatives rejected

- **Block all inline actions on remote**: forfeits mobile entirely.
- **Allow Tier B remotely now**: unreviewed write path into panes across the network.
- **Separate remote-only endpoint**: forks the guard chain.
