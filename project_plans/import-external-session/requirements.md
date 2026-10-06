# Requirements: import-external-session

**Date**: 2026-07-16
**Type**: feature addition
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
Users who start a Claude Code session outside stapler-squad (a plain terminal, a raw tmux pane, or an IDE terminal wrapped with `ssq-mux`) have no way to bring that session under stapler-squad's management. Today the external session and any stapler-squad-managed session are two disconnected things: conversation history, worktree state, and lifecycle tracking exist only on the external side, and stapler-squad has no record of the session at all (unless it happens to be `ssq-mux`-wrapped and merely streamed/mirrored, which still isn't a first-class managed `Instance`).

## Baseline
Today, to get an externally-started Claude session under stapler-squad's control, a user must manually note the conversation UUID (or dig it out of `~/.claude/projects/*.jsonl`), kill the external process themselves, and create a brand-new stapler-squad session that happens to `claude --resume <uuid>` into the same conversation — with no guarantee the paths, worktree, or in-flight state line up, and no built-in confirmation that the resume actually picked up the right conversation before the original is gone. `ssq-mux`-discovered sessions are visible to stapler-squad live, but there is no action to promote a `DiscoveredSession` into a fully managed `Instance`.

## Users / Consumers
- Developers who habitually start Claude Code from IDE terminals (IntelliJ, VS Code) via the existing `ssq-mux` wrapper.
- Developers who start Claude Code from a plain shell/tmux pane without any stapler-squad wrapper, and later decide they want stapler-squad's session management (worktrees, backlog integration, review flows) for that conversation.

## Success Metrics
- A user can turn any running external Claude session (`ssq-mux`-discovered, or a plain tmux/terminal session pointed at manually) into a fully-managed stapler-squad `Instance` with complete conversation history, in one user-initiated action — no manual UUID lookup, no manual `claude --resume` invocation.
- Zero conversation data loss: the imported session's history in stapler-squad matches the external session's `~/.claude/projects/*.jsonl` at the moment of import.
- The external process is only terminated after explicit user confirmation, and only after import has been verified to succeed — replacing today's "kill first, hope resume works" workflow with "verify first, kill on confirmation."

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- Must reuse existing plumbing where possible: `ExternalSessionDiscovery` / `mux.Discovery` (ssq-mux path), `HistoryLinker` / `HistoryFileDetector` (JSONL correlation path), `HistoryAdapter` (`ClaudeAdapter`, `AgyAdapter`) for conversation import/export.
- Killing an external process is a destructive, hard-to-reverse action on the user's own terminal/IDE — must always require explicit confirmation before kill, per this repo's general safety posture on destructive actions.
- Whether this becomes a new `SessionType` / 7-touchpoint creation mode (`.claude/rules/session-creation-registry.md`) vs. a lighter action bolted onto existing discovery flows is unresolved — deferred to Phase 2 research.

## Non-functional Requirements
- **Performance SLO**: not specified — import is a user-initiated, one-shot action, not a hot path.
- **Scalability**: not applicable — one user, a handful of concurrent external sessions at most.
- **Security classification**: internal — operates only on local Unix sockets (`ssq-mux`) and local `~/.claude/projects/` files; no network exposure.
- **Data residency**: no special requirements — all data stays local.

## Scope
### In Scope
- Import path via `ssq-mux`-discovered sessions (promote a `DiscoveredSession` to a managed `Instance`).
- Import path via manual pointer to a plain tmux/terminal session with no `ssq-mux` wrapper (e.g. by project directory or conversation JSONL/UUID), using `HistoryLinker`/`HistoryFileDetector`-style correlation.
- Non-Claude external programs (anything `HistoryAdapter` already supports, e.g. Antigravity via `AgyAdapter`) — explicitly requested as in-scope, not deferred.
- Batch import of multiple external sessions at once — explicitly requested as in-scope, not deferred.
- Confirm-before-kill flow for shutting down the original external session after import succeeds, with no undo once confirmed.

### Out of Scope
- None explicitly excluded by the user for v1 — see Rabbit Holes below for why "everything in scope" is a risk the research/planning phases must actively manage, not a free pass.

## Rabbit Holes
- **"Everything in scope" for a Large-but-bounded appetite**: batch import + multi-program support + two independent discovery paths + a destructive kill flow is a lot of surface for 3–6 weeks. Phase 3 planning must sequence this into shippable slices (e.g. single ssq-mux import first, manual/JSONL import second, batch/multi-program last) rather than attempting all paths simultaneously.
- **Conversation UUID / JSONL correlation ambiguity**: a plain tmux session with no `ssq-mux` wrapper has no reliable metadata (PID, cwd, command) unless `HistoryFileDetector`'s proc-inspection approach is extended; multiple simultaneous Claude processes in the same project directory could correlate to the wrong JSONL file.
- **Worktree/path mismatch**: the external session's cwd may not correspond to any git worktree stapler-squad currently knows about — import may need to create a new worktree/session entry rather than attach to an existing one, and it's unclear how that interacts with `session-creation-registry.md`'s 7 touchpoints.
- **Kill-after-confirm race**: between "import succeeded" and "user confirms kill," the external process could exit on its own, change directory, or spawn a resumed `--resume` conflict if the user doesn't wait — needs explicit state handling.
- **Batch import failure modes**: if importing session 3 of 5 fails, what happens to 1, 2, and the not-yet-attempted 4, 5? Needs a defined all-or-nothing vs. partial-success policy.

## Alternatives Considered
- Keep today's manual workflow (note UUID, kill by hand, create new session with `--resume`) — rejected as the explicit problem being solved.
- Restrict to `ssq-mux`-only import (skip plain-tmux/manual path) — rejected by user; manual path explicitly requested as in-scope.

## Feasibility Risks
- `HistoryFileDetector`'s process-to-JSONL correlation (used today for already-managed sessions) has not been proven for *unmanaged* external processes at import time — needs validation in research.
- No existing UI surface for "browse discovered-but-unmanaged sessions and import" — likely net-new frontend work regardless of which integration point (new creation mode vs. bolt-on action) is chosen.
- Multi-program support (`AgyAdapter` etc.) depends on `PortSessionHistory`'s existing translate/sync logic, which was built for switching programs on an already-managed session, not for importing an unmanaged one — reuse may require refactoring, not just calling.

## Observability Requirements
Emit structured logs for each import attempt (discovery path used, source PID/socket or JSONL path, target session ID, success/failure) and for each kill-after-confirm action (target PID, confirmation timestamp, exit status). No new oncall alert — failures surface synchronously to the initiating user via the UI/RPC error, consistent with other session-creation flows.

## Risk Control
Ship behind a feature flag / env var given the tmux/PTY lifecycle and process-killing surface across external tools (IntelliJ, VS Code, plain terminals). Kill action always requires explicit user confirmation with no undo — import must be verified successful before kill is even offered.

## Open Questions
- Should this be a new `SessionType` (full 7-touchpoint creation mode) or a lighter action on existing `ExternalSessionDiscovery` flows? → Phase 2 research.
- How does batch import compose with the confirm-before-kill flow — one confirmation per session, or one batch confirmation? → Phase 3 planning.
- Does importing a plain tmux/terminal session (no `ssq-mux`) require a live PTY attach at all, or is it purely a metadata + history operation (resume via `claude --resume` in a new stapler-squad-managed tmux session, then kill the old pane)? → Phase 2 research.
- What identifies "the same session" across a batch import when multiple unmanaged processes share a project directory? → Phase 2 research.
