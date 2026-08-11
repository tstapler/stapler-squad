# Requirements: Stapler Squad MCP Server

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-18

## Problem Statement

LLM agents (Claude, etc.) have no programmatic interface to Stapler Squad — there is no way to issue one-shot tasks like "create a workspace", "delegate a task to a specific agent", or "read terminal output from a session" without manual user intervention. This blocks full LLM-driven automation of multi-agent workflows. The primary user is the developer/operator running Stapler Squad who wants to orchestrate sessions via AI tooling.

## Success Criteria

- An LLM agent can create a workspace, delegate a task, and read terminal output in a single prompt with no manual steps
- External tools can control the full session lifecycle: create, pause, resume, destroy
- Sessions can be queried for state, metadata, tags, and branch info
- Terminal I/O (read scrollback, write input) is accessible via MCP tools
- The design is a solid foundation for additional MCP tools beyond the initial set

## Scope

### Must Have (MoSCoW)
- **Create/destroy sessions** — spin up new worktree sessions or tear them down via MCP tool calls
- **Read terminal output** — fetch scrollback/current output from any running session
- **Write input to sessions** — send keystrokes or commands to a running session
- **Query session state** — list sessions, get status, metadata, tags, branch info
- Robust, extensible design to build further features on

### Out of Scope
- Nothing is permanently out of scope; the user indicated no hard exclusions — ship incrementally

## Constraints

- **Tech stack**: No hard constraint — choose the best tool for the job
- **Timeline**: Not specified; ship incrementally with a solid foundation first
- **Dependencies**: Stapler Squad already has a ConnectRPC API for session management; MCP server should wrap or reuse it rather than bypassing internals
- **Deployment**: Should integrate naturally with the existing stapler-squad binary/process model

## Context

### Existing Work
- Stapler Squad has a ConnectRPC API (`server/services/`) with session management endpoints
- Protobuf schemas for sessions are defined in `proto/session/v1/`
- Sessions run in isolated tmux sessions with git worktrees
- Terminal streaming is already implemented via ConnectRPC
- External PTY multiplexing (`claude-mux`) already exists for external session monitoring

### Stakeholders
- Tyler Stapler (developer/operator) — primary user and stakeholder
- LLM agents (Claude, etc.) — consumers of the MCP tools

## Research Dimensions Needed

- [ ] Stack — evaluate MCP server implementation options (Go library, TypeScript SDK, embedded vs sidecar)
- [ ] Features — survey MCP tool design patterns, comparable MCP servers, best practices for tool schema design
- [ ] Architecture — design patterns for wrapping ConnectRPC in MCP, session lifecycle management, streaming tool output
- [ ] Pitfalls — known failure modes in MCP servers, security considerations for local tool exposure, terminal I/O edge cases
