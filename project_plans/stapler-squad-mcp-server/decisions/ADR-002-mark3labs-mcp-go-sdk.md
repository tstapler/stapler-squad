# ADR-002: Use mark3labs/mcp-go as the MCP SDK

**Status**: Accepted
**Date**: 2026-04-18

## Context

Two Go MCP SDK options exist:
1. `github.com/modelcontextprotocol/go-sdk` — official SDK, maintained with Google; unstable with breaking changes through mid-2025
2. `github.com/mark3labs/mcp-go` — community SDK; most widely adopted Go MCP library as of 2026

## Decision

Use `github.com/mark3labs/mcp-go`.

## Rationale

- Largest production adoption in the Go MCP ecosystem as of 2026
- API stable enough to build on; the official SDK is newer and less battle-tested
- Both support stdio transport — all we need for v1
- Migration scope is bounded to `server/mcp/` if we switch later

## Consequences

- Non-official dependency. Monitor maintenance cadence.
- If `mark3labs/mcp-go` is deprecated, migration is bounded to `server/mcp/` package.

## Alternatives Considered

- **modelcontextprotocol/go-sdk**: Viable but relatively new as of 2026. Prefer the more battle-tested option for initial implementation.
- **TypeScript @modelcontextprotocol/sdk**: Most mature overall, but requires Node.js runtime. Rejected per ADR-001.
