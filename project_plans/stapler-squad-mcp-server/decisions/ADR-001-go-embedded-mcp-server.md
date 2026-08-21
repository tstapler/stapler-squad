# ADR-001: Embed MCP Server in Stapler Squad Binary

**Status**: Accepted
**Date**: 2026-04-18

## Context

The MCP server needs access to session management capabilities. The deployment model determines how it gets that access: by sharing process memory with the existing service layer (embedded), or by calling over a network boundary (sidecar).

The server is local-only, single-user. Claude spawns the MCP process itself via stdio.

## Decision

Embed the MCP server inside the `stapler-squad` binary, activated by a `--mcp` flag. When `--mcp` is set, the binary runs in MCP stdio mode instead of starting the HTTP web server. The two modes are mutually exclusive.

## Rationale

- **Direct service access**: The MCP handler calls session service methods directly — no RPC round-trip per tool call, no network error surface
- **Single binary**: No second binary to build, distribute, or keep in sync
- **Zero port management**: stdio mode needs no port; no conflict with localhost:8543
- **Consistent with existing patterns**: The binary already supports multiple run modes

## Consequences

- `--mcp` and web server mode cannot run simultaneously in the same process. Acceptable — Claude spawns a fresh MCP process per session.
- All logging reachable from the MCP code path must be directed to stderr, not stdout (stdout is the MCP protocol channel).
- Tight coupling to service layer internals: MCP tools must be updated when service interfaces change.

## Alternatives Considered

- **Sidecar Go process**: Separate binary calling ConnectRPC. Rejected — adds a second binary and unnecessary RPC latency for a local tool with no isolation benefit.
- **TypeScript sidecar**: Rejected — adds Node.js runtime dependency. No benefit over Go.
- **Embedded with HTTP transport**: Rejected for v1 — local-only use case does not need HTTP.
