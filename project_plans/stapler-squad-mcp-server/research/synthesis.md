# Research Synthesis: Stapler Squad MCP Server

**Date**: 2026-04-18
**Sources**: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

---

## Decision Required

Choose the implementation stack, deployment model, and core tool surface for an MCP server that exposes Stapler Squad session management to LLM agents.

---

## Context

Stapler Squad is a Go application with an existing ConnectRPC API managing AI agent sessions in tmux + git worktrees. The goal is to add an MCP server so Claude (and other LLMs) can programmatically create workspaces, delegate tasks to sessions, and read/write terminal output — all as one-shot tool calls. The server will be local-only, single-user, with stdio transport.

---

## Options Considered

| Option | Summary | Key Trade-off |
|--------|---------|---------------|
| **Embedded Go (stdio)** | MCP server runs inside stapler-squad binary, launched with `--mcp` flag, communicates over stdin/stdout | Zero deployment overhead; tightly coupled; direct service access |
| **Sidecar Go process (stdio)** | Separate binary calls ConnectRPC at localhost:8543; spawned by MCP clients | Clean separation; independent deployability; adds RPC hop |
| **TypeScript sidecar (stdio)** | Node.js process using `@modelcontextprotocol/sdk`; calls ConnectRPC HTTP | Most ecosystem maturity; language mismatch; operational complexity |
| **Embedded Go (HTTP)** | MCP server on a separate port inside stapler-squad binary | Enables remote clients; more complex auth surface; not needed for local use |

---

## Dominant Trade-off

**Simplicity vs. separation of concerns.** The embedded approach gives direct access to session service internals (zero RPC overhead, direct data types, single binary to deploy) but couples the MCP surface tightly to the main application. The sidecar approach provides clean boundaries but introduces a process dependency and network hop. For a local, single-user tool where Claude spawns the MCP server itself via stdio, the embedded approach wins: there is no "independent deployability" benefit when both processes live on the same machine and are orchestrated by the same user.

The secondary tension: **terminal streaming vs. MCP's request/response model.** MCP tools return single results. Terminal output is continuous. This is resolved by designing the `read_session_output` tool as a snapshot tool (last N lines of scrollback) rather than a stream, with the LLM polling if it needs to monitor progress.

---

## Recommendation

**Choose: Embedded Go MCP server with stdio transport, using `mark3labs/mcp-go`**

**Because**: 
- Go is the existing language — no language boundary, no subprocess dependency on Node.js
- `mark3labs/mcp-go` is the most mature community Go MCP library as of 2026; the official `modelcontextprotocol/go-sdk` is viable but had breaking changes through mid-2025
- Embedding in the binary gives the MCP handler direct access to the session service layer — no ConnectRPC round-trip for every tool call
- Stdio transport is confirmed as the correct choice for local MCP clients (Claude Code docs: "stdio for everything on your machine")
- SSE is deprecated; Streamable HTTP is for remote deployments only — neither applies here

**Implementation model**: `./stapler-squad --mcp` runs in stdio mode. Claude adds it to MCP config as a command-based server. The `--mcp` flag puts the binary into MCP mode instead of web server mode.

**Accept these costs**:
- MCP and web server cannot share the same process instance (different invocation modes) — this is acceptable since Claude spawns a fresh MCP process per session
- Tight coupling to session service internals means MCP must be updated when service layer changes — acceptable for a single-maintainer project

**Reject these alternatives**:
- **TypeScript sidecar**: Rejected — adds Node.js as a runtime dependency, language context switch, and npm supply chain surface. No benefit over Go for this use case.
- **Sidecar Go process (separate binary)**: Rejected — adds deployment complexity (two binaries) with no gain for a local-only server. The RPC hop is unnecessary overhead.
- **HTTP transport**: Rejected for v1 — local-only use case does not need it. Can be added later for remote delegation use cases.

---

## Tool Surface Decision

**Choose: 12–15 tools in 4 semantic families**

Keep total tools ≤18 (LLMs degrade with >30–40 tools). Design around workflows, not API endpoints.

### Recommended Tool List

**Session Discovery & Query** (read-only, safe)
- `list_sessions` — list all sessions with status, tags, branch, path; supports filter params
- `get_session` — full details for a single session including recent output preview
- `search_sessions` — full-text + tag search across sessions

**Session Lifecycle** (state-changing)
- `create_session` — create + start a session (worktree + tmux); returns session ID + initial status
- `pause_session` — pause a running session
- `resume_session` — resume a paused session
- `stop_session` — stop and clean up a session (with `confirm: true` guard)
- `update_session` — update metadata: tags, title, category

**Terminal I/O** (high-risk, requires careful design)
- `read_session_output` — get scrollback snapshot; params: `session_id`, `lines` (default 50, max 200), `strip_ansi` (default true); returns `output`, `truncated: bool`, `total_lines: int`
- `write_to_session` — send input to a running session; params: `session_id`, `input`, `press_enter` (default true); returns acknowledgment only (async)
- `wait_for_output` — poll until output matches a pattern or timeout; params: `session_id`, `pattern`, `timeout_seconds` (max 60)

**VCS & Workspace** (git integration)
- `get_session_diff` — current git diff for the session's worktree
- `list_session_branches` — available branches for a repo

**Schema requirements** (all tools):
- Every tool returns a structured result with `success: bool` and `error: { code, message, remediation }` on failure
- State-changing tools return the new state so LLM can verify
- Error codes are machine-readable (e.g., `SESSION_NOT_FOUND`, `SESSION_NOT_RUNNING`)

---

## Critical Design Decisions (from Pitfalls Research)

1. **ANSI stripping ON by default** in `read_session_output` — raw ANSI corrupts LLM context and JSON; strip unless `raw: true` explicitly requested
2. **Output truncation is explicit** — always return `truncated: bool` and `total_lines: int` so LLM knows it has partial data
3. **`write_to_session` is fire-and-forget** — PTY writes are async; tool returns immediately after write; LLM must use `read_session_output` or `wait_for_output` to observe effects
4. **`stop_session` requires `confirm: true`** — prevents accidental destruction; LLM must pass `confirm: true` explicitly
5. **Rate limiting on `write_to_session`** — max 1 call/second per session to prevent runaway input flooding
6. **Command injection is a CRITICAL risk** — `write_to_session` input goes directly to PTY; document clearly that arbitrary input reaches the shell/agent; consider an allowlist mode
7. **Tool descriptions are prompt engineering** — invest in clear, precise descriptions that prevent ambiguous tool selection; 1–2 sentences max per tool

---

## Open Questions Before Committing

- [ ] Can `mark3labs/mcp-go` run in stdio mode cleanly in a Go binary that also has an HTTP server? (verify the transport doesn't conflict with existing listener) — blocks binary design
- [ ] What is the maximum scrollback buffer size in the existing `session/scrollback/` implementation? — blocks `read_session_output` line limit decisions
- [ ] Should `create_session` be synchronous (wait until tmux session ready) or async (return immediately with polling)? — blocks tool schema design; depends on typical session startup latency

---

## Sources

- [findings-stack.md](./findings-stack.md) — implementation options, SDK maturity, transport decisions
- [findings-features.md](./findings-features.md) — tool design patterns, proposed tool list, schema conventions
- [findings-architecture.md](./findings-architecture.md) — embedding vs sidecar, streaming model, ConnectRPC integration
- [findings-pitfalls.md](./findings-pitfalls.md) — security risks (command injection, tool poisoning), terminal I/O edge cases, truncation behavior
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) — official Go SDK
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) — community Go SDK (mature)
- [MCP Transports spec](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports) — stdio and Streamable HTTP
- [Claude Code MCP docs](https://code.claude.com/docs/en/mcp) — supported transports confirmed
- [Block MCP Playbook](https://engineering.block.xyz/blog/blocks-playbook-for-designing-mcp-servers) — tool design best practices
- [Claude Code truncation issue](https://github.com/anthropics/claude-code/issues/2638) — 10KB/256-line limit confirmed
- [MCP security risks](https://www.practical-devsecops.com/mcp-security-vulnerabilities/) — command injection CVEs
