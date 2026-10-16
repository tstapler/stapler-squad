# Requirements: mcp-program-param

## Source

Backlog item `306b8eeb-8766-4de4-a587-7c7887a33ac1`: "MCP create_session/create_session_for_pr's `program` param rejects custom programs the backend supports."

## Problem

`CreateSessionRequest.program` (`proto/session/v1/session.proto:693`) is a free-form
`string`. The backend already accepts arbitrary registered program IDs — confirmed by
the reporter: register a custom program via `UpsertProgramConfig`, then create a
session with that program ID directly through `POST /api/session.v1.SessionService/CreateSession`
— no validation error.

The two MCP tool definitions that wrap session creation hardcode the parameter to a
2-value enum, rejecting anything outside `claude`/`aider` before the request ever
reaches the backend:

- `server/mcp/tools_lifecycle.go:59` (`create_session`) —
  `mcpgo.WithString("program", ..., mcpgo.Enum("claude", "aider"))`
- `server/mcp/tools_github.go:101-103` (`create_session_for_pr`) — same pattern

An agent using the MCP tools has no way to launch a session with a custom program
(e.g. one added via the "Program Configurations" UI / `ProgramsManager.tsx`) without
bypassing the MCP tool and calling the ConnectRPC HTTP API directly — defeating the
purpose of exposing an MCP tool for session creation at all.

## Expected Behavior

The MCP tool's `program` parameter must accept any registered program ID, not just
`claude`/`aider`, while still giving an agent enough information to pick a sane value
without a separate discovery call in the common case.

Two options raised in the backlog item description:
1. Drop `mcpgo.Enum(...)` entirely; document `program` as free-form text, matching the
   proto, and let the backend's existing `CreateSession`/`ResolveProgramConfig`
   validation be the sole source of truth.
2. Populate the enum dynamically from `ListProgramsConfig` (built-ins +
   `cfg.SessionDefaults.Programs`, see `server/services/defaults_service.go:684`) at
   tool-definition time.

Research/plan phases below decide between these (and any hybrid) based on how
`mcpgo.WithString`/`mcpgo.Enum` are implemented (static vs. dynamic tool schemas) and
existing conventions elsewhere in `server/mcp/`.

## Acceptance Criteria

1. Creating a session via the `create_session` MCP tool with a `program` value that
   matches a registered custom `ProgramConfig.ID` (built-in or user-added via
   `UpsertProgramConfig`) succeeds — no client-side schema rejection before the
   request reaches the backend.
2. Creating a session via the `create_session_for_pr` MCP tool with the same kind of
   custom `program` value succeeds, for parity with `create_session`.
3. `claude` and `aider` continue to work as `program` values for both tools (no
   regression for the default/common case), and the default (`claude` when omitted)
   is unchanged.
4. An agent calling either tool still has some way to discover valid program values
   (via tool description text, an `enum` populated from `ListProgramsConfig`, or
   equivalent) — the fix must not silently trade "rejects valid input" for "no
   guidance on valid input" with zero documentation.
5. Existing MCP lifecycle/github tool tests continue to pass; new/updated tests cover
   creating a session with a non-`claude`/`aider` program value through the MCP tool
   path.
6. No proto or backend validation behavior changes — this is scoped to the MCP tool
   schema layer only (`server/mcp/tools_lifecycle.go`, `server/mcp/tools_github.go`,
   and any shared helper introduced to keep the two in sync).

## Out of Scope

- Changing `CreateSessionRequest.program`'s proto type or backend validation.
- Building new UI for program discovery.
- Any other MCP tool parameter's enum constraints (e.g. `session_type`).
