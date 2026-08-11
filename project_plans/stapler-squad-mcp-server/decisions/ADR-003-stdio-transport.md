# ADR-003: Use stdio Transport for MCP Server

**Status**: Accepted
**Date**: 2026-04-18

## Context

MCP defines two official transports as of the 2025-03-26 spec:
1. **stdio** — bidirectional over stdin/stdout; for local process invocation
2. **Streamable HTTP** — HTTP POST/GET with optional SSE; for remote deployments

HTTP+SSE from the 2024-11-05 spec is deprecated. Claude Code documentation (verified 2026): "A good default is to pick stdio for everything on your machine and HTTP for everything else."

## Decision

Use stdio transport. The MCP server is invoked as `./stapler-squad --mcp` and communicates over stdin/stdout.

## Rationale

- Local-only, single-user — stdio is the spec-recommended transport for this use case
- No port to manage, no auth surface, no network configuration
- Claude Code supports stdio natively for all local MCP servers

## Consequences

- Cannot be accessed remotely without a wrapper. Acceptable — remote access is out of scope for v1.
- Logging must be directed to stderr; stdout is the MCP protocol channel. All logging reachable from `--mcp` path must be audited.
- One client at a time (stdio is point-to-point). Acceptable for single-user local use.

## Alternatives Considered

- **Streamable HTTP**: For remote deployments. Rejected for v1 — no remote use case, adds unnecessary complexity.
- **HTTP+SSE**: Deprecated in MCP spec 2025-03-26. Rejected.
