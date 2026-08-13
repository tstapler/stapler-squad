# ADR-028: Closure-Based Mailbox Commands, Not a Discriminated Union

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/implementation/plan.md` (Open Decisions §1)

---

## Context

`research/architecture.md` §1 recommended a closure-based command type
(`command{name string; fn func(s *instanceState)}`) over a discriminated struct-per-command-type
union. The plan synthesis flagged this as needing an explicit ADR because it appears to cut
against this codebase's own established convention: both the `OmnibarAction` union
(`web-app/src/lib/omnibar/actions/types.ts` + `dispatch.ts`) and the session-creation mode
registry (`.claude/rules/session-creation-registry.md`) use discriminated unions plus an
exhaustive switch specifically so that adding a new variant without wiring its handler is a
compile-time (TypeScript) or lint-time (`exhaustive` style) failure, not a silent gap.

`session/instance_tmux.go`'s `programKind` sealed interface (`claudeProgram`/`plainProgram`) is
this codebase's existing Go-side example of the same discriminated-union discipline.

## Decision

Use closures (`func(s *instanceState)`) as the command payload, not a discriminated union with
an exhaustive switch in the actor's run loop.

The reason the discriminated-union pattern earns its keep elsewhere in this codebase is that it
separates a command's *type* from its *behavior*, which must then be looked up and dispatched —
often in a different file than the type is defined (`OmnibarAction`'s type lives in `types.ts`,
its behavior is wired in `dispatch.ts`). That separation is exactly what creates the "forgot to
wire a case" risk the exhaustive switch protects against.

The actor's mailbox has no such separation: each of the ~75 existing `stateMutex`-guarded method
bodies converts almost mechanically into a closure that already *is* its own behavior — there is
no second file where that behavior must be independently re-registered, and therefore no
"missing case" failure mode for an exhaustive switch to guard against. The run loop's `for cmd :=
range mailbox { cmd.fn(s) }` handles every closure by construction; there is nothing to forget.

Imposing a discriminated union here would mean defining ~75 near-identical command structs plus
an exhaustive switch that does nothing but call each struct's own logic — bureaucratic ceremony
that doesn't exist to catch a real bug class for this specific use, unlike `OmnibarAction` where
the ceremony is load-bearing.

## Consequences

### Positive
- Each of the ~75 existing locked method bodies converts with minimal restructuring — close to a
  mechanical `i.send(func(s *instanceState) { <existing body> })` wrap per call site, keeping the
  Epic 4/5 (state-machine core, `session_service.go` cutover) diffs reviewable.
- No second registry (a command-type enum) to keep in sync with the actor's switch — one less
  place for drift.

### Negative / Accepted tradeoffs
- Debuggability is slightly worse than a named discriminated union: a stack trace or log line
  shows an anonymous closure rather than a named command type. Mitigate by setting `command.name`
  (already in the proposed struct) from the calling method for logging/tracing, even though `fn`
  carries the actual behavior.
- This decision is scoped to the `Instance` actor's mailbox specifically. It does not change or
  weaken the discriminated-union requirement for `OmnibarAction` or the session-creation registry
  — those solve a genuinely different problem (cross-file type-to-behavior dispatch) and keep
  their exhaustive-switch discipline unchanged.
