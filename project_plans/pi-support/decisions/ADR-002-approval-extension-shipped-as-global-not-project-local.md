# ADR-002: Ship the Approval Extension as a Global pi Extension, Not Project-Local

**Status**: Accepted
**Date**: 2026-09-02
**Project**: pi-support

## Context

`research/pitfalls.md` PITFALL-1 and `research/stack.md` confirm: pi's project-local
extensions (`.pi/extensions/*.ts`) "load only after the project is trusted," and only
global (`~/.pi/agent/extensions/*.ts`) or CLI-supplied (`-e`) extensions get a vote on the
`project_trust` event. There is no documented way for a project-local extension to
bootstrap its own trust.

stapler-squad creates a **new git worktree per session** (see top-level `CLAUDE.md`,
`session/` worktree management) — meaning a pi-backed session's working directory is very
often a directory pi has never seen before. If the approval extension is project-local, every
first run of a new worktree risks the extension silently not loading at all until the user
manually answers pi's trust prompt for that specific worktree — a silent, per-worktree
enforcement gap that is exactly PITFALL-1's top-severity concern ("the gate looks installed
but doesn't actually work, and nothing tells you").

`research/pitfalls.md`'s own mitigation list names this directly: "consider shipping the
approval extension as a **global** extension... rather than project-local, so the approval
gate never depends on a per-worktree trust decision the user hasn't made yet."

## Decision

`ssq-hooks install pi` writes the approval extension to
`~/.pi/agent/extensions/ssq-approval.ts` (global scope), not `<worktree>/.pi/extensions/*.ts`.
This is a one-time, per-machine install (mirroring `installOpenCode()`'s global
`~/.config/opencode/plugins/ssq-hooks.js` placement, `cmd/ssq-hooks/main.go:1302`) rather than
a per-session/per-worktree write, and is unaffected by any individual worktree's trust state.

The approval-URL string embedded in the template cannot be worktree-specific as a result (it
is the server's base URL, which is already worktree-independent — `hookEndpoints()` resolves
from the running server's address, not the project directory), so this constraint does not
lose any functionality. Per-session identification for the audit trail comes from the
`X-CS-Session-ID`-equivalent field in the JSON body (a `session_id`/`cwd` field the extension
reads from pi's own event context at call time), not from which physical file was injected.

## Alternatives Considered

| Option | Rejected because |
|---|---|
| Project-local (`.pi/extensions/ssq-approval.ts`), written per-worktree at session creation | Subject to pi's per-directory trust gate; a new worktree (the common case for this tool) would silently ship with zero enforcement until the user manually trusts it — the exact failure PITFALL-1 flags as CRITICAL. |
| Write to both global and project-local paths defensively | Reintroduces the "patch both paths" anti-pattern this project's own precedent (`agy-support`'s `installAgy()`, per `research/build-vs-buy.md` §4) already had to walk back — risks double-fired `tool_call` handlers if pi ever loads both scopes for the same event, and still doesn't remove the trust-gate risk for the project-local copy. |

## Consequences

- Global placement means the extension is installed once per machine (via `ssq-hooks install
  pi`), not per-session — simpler injection lifecycle, no per-worktree write/idempotency
  logic needed for the extension file itself (still need it for the binary copy step, which
  already exists for other agents).
- The extension applies to **every** pi invocation on the machine once installed, including
  ones started outside stapler-squad — acceptable per requirements (approval enforcement
  being "on" for more pi usage than just stapler-squad-launched sessions is a superset of the
  ask, not a scope violation), but worth calling out explicitly in user-facing docs/UI copy so
  it isn't a surprise.
- Global extensions still need Phase 1's live-verification spike to confirm they are in fact
  exempt from the trust gate (the docs say "global or CLI extensions get a vote on trust," but
  the *practical* default-trusted behavior for a freshly-installed global extension with no
  explicit trust prompt at all should be confirmed against the installed pi binary before this
  ADR's premise is treated as verified rather than doc-derived).
