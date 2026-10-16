# ADR-001: Nil-Tolerant `SessionAttacher` Across MCP Transports

**Status**: Accepted
**Date**: 2026-08-16
**Project**: backlog-session-item-linking

## Context

`link_session_to_item` (the new MCP tool that exposes `BacklogService.AttachSessionToItem` to
agent sessions) needs a `*services.BacklogService`-backed dependency inside `backlogHandlers`
(`server/mcp/tools_backlog.go:91`). `backlogHandlers` is constructed in exactly one place,
`NewCore` (`server/mcp/server.go:28`), which is shared by two distinct MCP transports:

1. **HTTP transport** (`NewHTTPHandler`, mounted at `/mcp` on the main server process) — the
   common case. `server/server.go:502` already has `deps.BacklogService`
   (`*services.BacklogService`, non-nil) available via the existing `CoreDeps`/`ServiceDeps`
   dependency-injection struct (`server/dependencies.go:65,398,1200`).
2. **stdio transport** (`RunServer`, the `--mcp` flag's subprocess mode). In the normal case
   this is a thin client (`RunProxyServer`) that forwards to the HTTP daemon and never
   constructs `backlogHandlers` locally at all. It only falls back to constructing its own
   local deps (`main.go:89`, `buildMCPDeps`) when the HTTP daemon isn't reachable yet (e.g.
   before the main service has finished starting). `buildMCPDeps` calls
   `server.BuildCoreDeps()` (`server/dependencies.go:321`), which does **not** construct a
   `*services.BacklogService` — that requires `cfg`, `workflowEngine`, `pipelineEngine`, and
   `pipelineModeRepo` (`server/dependencies.go:940`), none of which `BuildCoreDeps` builds
   today.

So one of the two transports structurally cannot supply the dependency `link_session_to_item`
needs, at least not without a materially larger change than this feature warrants.

## Decision

Add `backlogSvc *services.BacklogService` as a new parameter to `NewCore`, `NewHTTPHandler`,
and `RunServer`. Inside `NewCore`, convert it to the narrow `SessionAttacher` interface using
the same nil-tolerant guard already used for `liveFinder` (`server/mcp/server.go:42-45`):

```go
var attacher SessionAttacher
if backlogSvc != nil {
    attacher = backlogSvc
}
```

The HTTP call site (`server/server.go:502`) passes the real `deps.BacklogService`. The stdio
fallback call site (`main.go:97`) passes a literal `nil`. `link_session_to_item`'s handler
checks `h.attacher == nil` and returns a `UNAVAILABLE` MCP error with a clear remediation
message ("retry once the HTTP daemon is reachable") rather than panicking.

## Alternatives Considered

**A. Extend `BuildCoreDeps`/`buildMCPDeps` to construct a full `*services.BacklogService` for
the stdio fallback too.** Rejected: would require pulling `cfg` (config loading),
`workflowEngine`, `pipelineEngine`, and `pipelineModeRepo` into a code path whose entire
purpose (per its own doc comment, `main.go:1036-1039`) is to stay minimal — "Phase 1+2 only (no
tmux startup, no HTTP listener, no background pollers)." The fallback path is also, by
construction, only hit in a narrow, already-degraded window (daemon not up yet); building out
its dependency graph to support one new tool's one degraded-mode error message is
disproportionate.

**B. Make `link_session_to_item` fail fast at the `--mcp` stdio entry point instead of
returning a structured MCP error.** Rejected: every other backlog tool already degrades
gracefully in this fallback mode (nil `reviewStopper`/`reviewTrigger` are tolerated silently
today) — a hard process failure for one specific tool would be an inconsistent, surprising
exception to that convention, and would be harder for an agent to recover from than a
structured, actionable `UNAVAILABLE` response.

**C. Require callers to always route through the HTTP daemon (remove the local stdio fallback
path entirely).** Out of scope — the fallback path exists for a reason (daemon startup
ordering) unrelated to this feature, and removing it is a much larger, separate change with
its own risk profile.

## Consequences

- `link_session_to_item` works in the common case (HTTP transport, which is what
  `RunProxyServer` forwards to whenever the daemon is up) and degrades to a clear, structured
  error in the rare fallback case, rather than a nil-pointer panic or a silent no-op.
- `get_linked_item` (the read-only introspection tool) does **not** need this guard at all — it
  only depends on `storage`, which both transports already have. Only the write path
  (`link_session_to_item`) is affected by this ADR.
- Establishes a reusable precedent: any future MCP tool that needs to call into
  `*services.BacklogService` (or another service-layer type not currently threaded through
  `NewCore`) should follow the same nil-tolerant-narrow-interface pattern rather than growing
  `buildMCPDeps`'s dependency graph.
- A session that starts during the narrow "daemon not up yet, using stdio fallback" window and
  needs `link_session_to_item` before the daemon comes up will see `UNAVAILABLE` and must retry
  — accepted as a rare, transient, and self-resolving condition rather than a hard blocker.
