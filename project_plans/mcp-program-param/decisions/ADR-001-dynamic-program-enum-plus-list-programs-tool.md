# ADR-001: Dynamic `program` enum via a shared helper, plus an additive `list_programs` tool

**Status**: Accepted
**Date**: 2026-09-24

## Context

`create_session` (`server/mcp/tools_lifecycle.go:59`) and `create_session_for_pr`
(`server/mcp/tools_github.go:101-103`) both declare `program` with a hardcoded
`mcpgo.Enum("claude", "aider")`. The backend accepts any registered program ID
(built-in or custom, via `UpsertProgramConfig`); the hardcoded enum rejects
everything else at the MCP schema layer before the request ever reaches the
backend. `research/features.md` confirms 5 of 7 built-ins (`pi`, `opencode`,
`gemini`, `agy`, `bash`) are already unreachable through this enum, independent
of any custom program.

Two options were raised in the backlog item:
1. Drop the enum, rely on free-form text + description.
2. Populate the enum dynamically from `ListProgramsConfig` at tool-registration
   time.

`research/stack.md` confirms option 2 needs no new dependency (`mcpgo.Enum`
takes a variadic `...string`, and `svc.ListProgramsConfig` is a cheap,
synchronous, side-effect-free call safe to make once at `NewCore` time).
`research/architecture.md` confirms `mcpgo.Enum` is never server-enforced
(schema-only) and flags that duplicating the "call ListProgramsConfig, build
Enum" logic independently in both files would recreate the exact drift bug
this item exists to fix — the same shape as `tools_backlog.go:598-608`'s
hand-synced `eventTypeFilter` counter-example.

## Decision

**Core fix (in AC6's scope — `tools_lifecycle.go`, `tools_github.go`, one new
shared helper file):**

Introduce `server/mcp/tools_program_common.go` with `programSchemaOptions(svc
*services.SessionService) []mcpgo.PropertyOption`, which calls
`svc.ListProgramsConfig` once (nil-guarded) and returns a `Description(...)` +
`Enum(...)` pair built from the live built-in + custom program list. Both
`registerLifecycleTools` and `registerGitHubTools` call this one helper instead
of hand-rolling their own `mcpgo.Enum(...)` literal.

This is a **hybrid of options 1 and 2, not option 2 alone**: the enum is
populated dynamically (option 2's mechanism), but the description text is
written as if the enum weren't authoritative (option 1's honesty about
staleness) — it explicitly tells the agent the enum is a startup-time
convenience list, not an exhaustive/live one, and names `list_programs` (see
below) as the live-authoritative source. This directly targets the one
documented weakness of a pure dynamic enum (`research/pitfalls.md`:
"Dynamic enum ... stale until process restart") without giving up option 1's
honesty about that staleness, and without giving up option 2's advantage of a
concrete, click-to-select schema hint for the common case (`research/stack.md`
confirms nothing in mcp-go forces choosing one over the other).

**Additive fix (beyond AC6's named scope — a new, clearly-labeled story):**

Add a new read-only `list_programs` MCP tool, backed directly by
`svc.ListProgramsConfig`, so an agent has a live/authoritative discovery path
that isn't frozen at server-registration time. `research/features.md` names
this as "the clearest concrete gap for AC4"; `research/architecture.md`
confirms no such tool exists today across all of `server/mcp/*.go`.

### Why add `list_programs` given it exceeds AC6's literal scope

AC4 as literally written ("an enum populated from `ListProgramsConfig`, or
equivalent" is sufficient) is already satisfied by the dynamic enum alone —
`list_programs` is not required to pass the acceptance criteria. It is added
anyway because:
- It is the only mechanism that stays correct in real time (a program added
  via `UpsertProgramConfig` after server start is invisible to the enum until
  restart — a real, if bounded, gap per `research/pitfalls.md`).
- It is cheap: a direct passthrough to an existing, already-tested service
  call (`DefaultsService.ListProgramsConfig`), no new proto, no new backend
  logic, following the exact `list_sessions`/`list_approval_rules`/
  `list_workflows` naming and registration precedent already in this package.
- It is additive and reversible — it does not touch `CreateSessionRequest`,
  `create_session`, or `create_session_for_pr` behavior, so it carries none of
  AC6's "no proto or backend validation behavior changes" risk.

It is scoped as **Phase 2 / Epic 2.1** in `plan.md`, separate from Phase 1's
core fix, so it can be reviewed, deferred, or dropped independently without
touching the ACs it isn't required for.

### Addendum: a soft, non-fatal warning on an unrecognized `program` (added during plan repair)

The architecture review pointed out that `programSchemaOptions`/`programIDs` only
build the schema hint — nothing checked an incoming `program` value against that
same list before calling `CreateSession`, unlike this feature's own precedent
(`tools_notifications.go`'s `parseNotificationTypeFilter`, which validates as well
as builds the enum). A typo'd or hallucinated `program` would still succeed at the
tool-call layer (`success: true`) and fail only later, silently, as shell output in
the session's tmux pane — worse for AC4 than doing nothing, per `research/pitfalls.md`.
Phase 1 (Epic 1.2/1.3) now adds a same-call, non-fatal `ProgramWarning` field to
`CreateSessionResult`/`CreateSessionForPRResult` when `program` isn't in
`programIDs(svc)`'s registration-time snapshot. This is deliberately a warning, not
a rejection: a custom program registered via `UpsertProgramConfig` after this
process started must still be allowed to launch — the same registration-time-
snapshot caveat this ADR already documents for the enum itself.

## Alternatives Rejected

| Alternative | Reason rejected |
|---|---|
| Option 1 alone (drop enum, free-form only) | Weaker AC4 support — relies solely on description text with no schema-level hint; `research/pitfalls.md` flags this as a real UX regression risk (typos succeed at the API layer and fail silently later in tmux output, not as a tool-call error). |
| Option 2 alone (dynamic enum, no discovery tool, no staleness caveat in description) | Leaves the staleness gap undocumented — an agent has no way to learn the enum can be incomplete, and no live fallback when it is. |
| Independent duplication of the `ListProgramsConfig`-calling logic in both files (status quo shape, just with a live call instead of a literal) | Recreates the exact two-file-drift bug this backlog item exists to fix, per `research/architecture.md`'s explicit recommendation against it. |
| Caching/memoizing `list_programs`' response instead of calling `ListProgramsConfig` per invocation | Unnecessary — `research/stack.md` confirms the call is cheap, synchronous, and side-effect-free; caching would reintroduce the exact staleness problem `list_programs` exists to solve. |

## Consequences

- Two files change in Phase 1 (`tools_lifecycle.go`, `tools_github.go`) plus
  one new shared helper file — matches AC6's scope.
- Phase 2 adds one new tool registration + handler in a new
  `server/mcp/tools_programs.go` file, plus its registration call in
  `NewCore` (`server/mcp/server.go`) — outside AC6's named files, called out
  here and in `plan.md` as additive.
- The enum can still go stale between server restarts; this is now
  documented in the tool description rather than silently true.
