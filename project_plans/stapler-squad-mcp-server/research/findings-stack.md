# Findings: Stack — MCP Server Implementation Options

## Summary

Stapler Squad should implement an **embedded Go MCP server using the `modelcontextprotocol/go-sdk`** as the primary approach, with **stdio transport** as the default. This approach maximizes alignment with the existing tech stack (Go + ConnectRPC), minimizes deployment complexity, and provides tight control over terminal I/O streaming. A secondary TypeScript option is viable if Go SDK maturity proves insufficient during prototyping.

**Key tradeoff**: Embedded Go server simplifies deployment and startup latency but requires maintaining Go bindings to a nascent protocol. A TypeScript sidecar offers SDK maturity and ecosystem examples but adds process management overhead and multiplexing complexity for terminal streaming.

**Recommendation**: Go embedded (stdio) for MVP, with fallback plan to TypeScript sidecar if Go SDK lacks essential features.

---

## Options Surveyed

### Option 1: Embedded Go MCP Server (stdio transport)

**Approach**: Compile an MCP server directly into the stapler-squad binary using the Go MCP SDK, communicating via stdin/stdout.

**Dependencies**:
- `modelcontextprotocol/go-sdk` [TRAINING_ONLY — verify current maturity and transport support]
- No additional process management needed
- Reuses existing Go module (go.mod: Go 1.25.0, connectrpc.com/connect v1.19.0, google.golang.org/protobuf v1.36.10)

**Transport**: Stdio (default MCP transport; required by spec for non-HTTP clients)

**Integration Path**:
1. Add `modelcontextprotocol/go-sdk` to go.mod
2. Create `server/mcp/server.go` wrapping SessionService RPC calls
3. Register MCP tools that translate to ConnectRPC calls (e.g., `create_session` → SessionService.CreateSession)
4. On startup, if `--mcp` flag is set, run MCP server on stdio instead of HTTP
5. Stapler Squad binary becomes dual-mode: HTTP server (default) or MCP server (on demand)

**Advantages**:
- ✅ **Perfect language fit**: Go codebase, no polyglot overhead
- ✅ **Unified binary**: Single process, no cross-process communication, no startup latency
- ✅ **Direct API access**: MCP tools call SessionService methods directly, zero latency
- ✅ **Terminal I/O integration**: Can use existing tmux streaming code (TmuxStreamerManager, ScrollbackManager)
- ✅ **Deployment simplicity**: No separate process to manage; agents just spawn `stapler-squad --mcp`
- ✅ **Future composability**: Can embed other agents (future worktree delegation model)

**Disadvantages**:
- ❌ **Go SDK immaturity** [TRAINING_ONLY]: Go SDK is younger than TypeScript (@modelcontextprotocol/sdk), may have bugs/missing features
- ❌ **Debugging harder**: Blends HTTP and MCP logic in same process; errors mix concerns
- ❌ **API confusion**: Operators may accidentally call HTTP endpoints from agents expecting MCP
- ❌ **Platform support unknown** [TRAINING_ONLY]: Go SDK may not support all transports equally

**Risk factors**:
- If Go SDK lacks critical features (streaming, resource requests, sampling), pivot to TypeScript is non-trivial
- Stdio transport limits agent-to-server bidirectional communication (but MCP design assumes tool results are streamed back)

---

### Option 2: Embedded Go MCP Server (HTTP/SSE transport)

**Approach**: Same as Option 1 but use HTTP/Server-Sent Events instead of stdio.

**Transport**: HTTP POST for tool calls, SSE for streaming results

**Dependencies**: Same as Option 1 + `github.com/gorilla/sse` or similar (but Stapler Squad already uses gorilla/websocket, can extend)

**Advantages**:
- ✅ All advantages of Option 1
- ✅ Better for agent frameworks that expect HTTP (e.g., some Claude Code versions)
- ✅ Can coexist with HTTP server on same port (use `/api/mcp` prefix)
- ✅ Easier to test with curl or browser

**Disadvantages**:
- ❌ More complex: dual HTTP handler logic in server.go
- ❌ SSE can be fragile over long connections (firewalls, proxies drop idle streams)
- ❌ Extra latency: HTTP framing overhead vs stdio
- ❌ Port contention: If stapler-squad listens on 8543, agents must know to use /api/mcp
- ❌ Requires explicit agent configuration (URL, auth headers)

**Risk factors**:
- SSE connection flakiness in production (firewalls, proxies)
- More moving parts = more failure modes

---

### Option 3: Separate TypeScript MCP Server (stdio transport)

**Approach**: Create a standalone Node.js process running `@modelcontextprotocol/sdk` that communicates with stapler-squad over ConnectRPC HTTP.

**Dependencies**:
- `@modelcontextprotocol/sdk` (mature, well-documented)
- TypeScript/Node.js runtime (~20 MB footprint)
- Process manager (systemd, supervisord, or embedded Go process spawner)

**Architecture**:
```
Agent (Claude) → [stdio] → MCP Server (TypeScript) ←→ [ConnectRPC HTTP] ← stapler-squad binary
```

**Startup flow**:
1. User runs `stapler-squad` (HTTP mode on 8543 as usual)
2. Agent calls `stapler-squad-mcp --server-url http://localhost:8543` (or auto-detect)
3. MCP server on stdout/stdin translates tool calls to ConnectRPC
4. Results flow back through HTTP, then to agent via stdout

**Advantages**:
- ✅ **SDK maturity**: @modelcontextprotocol/sdk is battle-tested, extensive examples
- ✅ **Decoupling**: MCP logic entirely separate from stapler-squad; easier to debug
- ✅ **Rapid prototyping**: Existing TypeScript patterns for MCP tools
- ✅ **Ecosystem examples**: Many TS MCP servers exist (Claude Resources, Zod validation, etc.)
- ✅ **Easy pivots**: Can add HTTP/SSE transport without modifying stapler-squad

**Disadvantages**:
- ❌ **Extra process**: Agents must spawn a separate binary; adds startup latency (~500ms)
- ❌ **Process management**: Who kills it? Agent frameworks may not handle cleanup
- ❌ **Language mismatch**: Go codebase + TypeScript server = context switching, dual CI/CD
- ❌ **Terminal I/O complexity**: Streaming terminal data through HTTP is lossy (chunking, encoding); must handle ANSI codes carefully
- ❌ **Deployment friction**: Requires Node.js runtime (or bundled binary); larger install footprint
- ❌ **Network assumption**: Assumes stapler-squad HTTP is always accessible; fragile in containerized environments

**Risk factors**:
- Process spawn failure → agent hangs waiting for MCP server
- HTTP latency kills interactive terminal I/O (expect 50–200ms roundtrips vs 0ms with embedded)
- Node.js version management (which LTS? what if user lacks Node?)

---

### Option 4: Separate Go MCP Server (stdio transport)

**Approach**: Compile a separate standalone binary (`stapler-squad-mcp` or similar) using Go MCP SDK that talks to stapler-squad HTTP.

**Dependencies**: Go + MCP SDK + HTTP client

**Advantages**:
- ✅ **Language consistency**: Both in Go, easier context switching
- ✅ **SDK immaturity safety**: If Go SDK fails, easy to rewrite in TypeScript
- ✅ **Single-language CI/CD**: No TypeScript toolchain needed

**Disadvantages**:
- ❌ **Redundant to Option 1**: Separate process adds latency and complexity without benefits of either embedded or TypeScript ecosystem
- ❌ **Worst of both worlds**: Go SDK immaturity + process overhead
- ❌ **Terminal I/O**: Same HTTP chunking problem as Option 3
- ❌ **Unclear advantage**: Why not embedded Go (Option 1) or mature TypeScript (Option 3)?

---

## Trade-off Matrix

| Option | Language fit | Transport support | Embeddability | Ecosystem maturity | Maintenance burden | Startup latency | Terminal I/O | Notes |
|--------|-------------|------------------|---------------|-------------------|-------------------|-----------------|------------|-------|
| **1. Go Embedded (stdio)** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ Stdio only (+ HTTP possible) | ⭐⭐⭐⭐⭐ | ⭐⭐ SDK nascent | ⭐⭐ Blended, harder debug | <1ms | Direct PTY access | **RECOMMENDED** — Best for Go codebases; lowest latency |
| **2. Go Embedded (HTTP/SSE)** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ HTTP/SSE + stdio | ⭐⭐⭐⭐⭐ | ⭐⭐ SDK nascent | ⭐⭐⭐ Cleaner (separate endpoint) | <1ms | Direct PTY via SSE | More complex but decoupled HTTP |
| **3. TS Sidecar (stdio)** | ⭐⭐ TS vs Go codebase | ⭐⭐⭐⭐⭐ | ⭐ Separate process | ⭐⭐⭐⭐⭐ Mature SDK | ⭐⭐⭐ Isolated; easy debug | ~500ms spawn | HTTP chunking lossy | **FALLBACK** if Go SDK insufficient |
| **4. Go Sidecar (stdio)** | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ Separate process | ⭐⭐ SDK nascent | ⭐⭐ Blended concern | ~100ms spawn | HTTP chunking lossy | Avoid — worst of embedded + sidecar |

---

## Risk and Failure Modes

### Go SDK Immaturity Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Missing transport support** — SDK doesn't implement stdio, only HTTP | Medium [TRAINING_ONLY — verify] | High — MVP blocked | Prototype early with SDKs; fallback to TypeScript if needed |
| **Streaming tool results broken** — SDK has bugs in long-running output streams | Medium | High — Terminal I/O unusable | Extensive testing with long-running commands (git clone, npm install, etc.) |
| **Resource requests unimplemented** — SDK doesn't support MCP resource types (for future file access) | High [TRAINING_ONLY] | Low (post-MVP) | Document as limitation; plan TypeScript migration for resource features |
| **Sampling/async tool calls not working** — SDK doesn't support async responses | Medium | Medium — Forces synchronous tool design | Test early; document constraints in tool schema |
| **Type safety / schema validation weak** — Go doesn't enforce MCP tool schema at compile time like TypeScript does | High [TRAINING_ONLY] | Medium — Runtime errors, LLM confusion | Add runtime schema validation in wrapper layer |

### Terminal I/O Edge Cases

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **ANSI codes corrupted** — Streaming output over HTTP chunks mid-escape-sequence | High (HTTP chunking risk) | High — Terminal looks broken | Send output in raw bytes without framing; use base64 or binary encoding |
| **PTY resize race** — Agent resizes terminal while command running; PTY out of sync | Medium | Medium — Text wraps incorrectly | Use ConnectRPC's existing terminal size negotiation; sync before each write |
| **Buffering / deadlock** — Tool reads from PTY while agent writes; circular buffer full | Medium | High — Tool hangs permanently | Design tool as streaming (not blocking read); use channel-based buffering |
| **Session cleanup on disconnect** — Agent disconnects mid-read; session state inconsistent | High (stdio nature) | Medium — Dangling tmux pane | Hook agent disconnect in MCP server; kill session on connection loss |

### LLM Usability Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Tool output exceeds context limit** — `read_terminal_output` returns 100 KB scrollback; LLM can't fit in context | High | High — Tool useless for large scrollbacks | Limit output to 10–20 KB per call; add pagination (offset/limit params) |
| **Tool results timeout** — LLM waits >30s for tool result (e.g., long git clone); client timeout | Medium | High — Tools unreliable | Set realistic timeouts (30–60s); stream results incrementally where possible |
| **Ambiguous tool naming** — Tools like `run_command` vs `send_input` confuse LLM about semantics | Medium | Medium — LLM misuses tools | Descriptive names: `write_terminal_input`, `read_terminal_scrollback`, `get_session_status` |
| **Implicit session ID semantics** — LLM calls tool without setting session_id; defaults to first session (wrong) | High | High — Commands run in wrong session | Make session_id required; return helpful error if missing |

### Deployment / Operations Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Port contention** — MCP server tries to bind same port as HTTP server | High (if HTTP/SSE option chosen) | High — Port already in use | Use separate port (8544) or dedicated endpoint prefix (/api/mcp) |
| **Process affinity unclear** — Operator doesn't know if `stapler-squad --mcp` kills HTTP server | Medium | Medium — Silent failures | Document clearly: `--mcp` mode runs MCP on stdio, disables HTTP |
| **Agent framework doesn't support stdio MCP** — Claude Code v2.0 only speaks HTTP/SSE | Medium [TRAINING_ONLY] | High — MCP unreachable | Plan HTTP/SSE as fallback; test integration early with Claude Code |

---

## Migration and Adoption Cost

### Embedded Go (Option 1) → Fallback to TS Sidecar (Option 3)

If Go SDK proves insufficient (e.g., missing resource support, streaming bugs):

1. **Proto stage (no code changes)**: Run both in parallel; debug which blocks MVP
2. **Sidecar proto**: Create `stapler-squad-mcp` TypeScript binary; test connectivity
3. **Cut over**: Update agent instructions to call TS sidecar instead; remove Go MCP code from main binary
4. **Sunset Go SDK**: Archive `server/mcp/` folder; keep as reference

**Estimated cost**: 2–3 days (if Go SDK is the blocker; otherwise no cost)

### Embedded HTTP/SSE (Option 2) → Pure HTTP API (no MCP)

If MCP protocol proves too unstable or LLM support lacking:

1. **Decouple HTTP endpoint**: Extract MCP tool handlers to `/api/mcp/tools/{name}` REST endpoint
2. **Test with curl**: Ensure all tools work via REST
3. **Agent integration**: Update agent framework to call HTTP instead of stdio MCP
4. **Deprecate MCP**: Keep MCP as legacy transport, document HTTP as primary

**Estimated cost**: 1 day (extraction + testing)

---

## Operational Concerns

### Logging and Debugging

**Challenge**: MCP server and HTTP server sharing the same process; hard to distinguish logs.

**Mitigation**:
- Add `[MCP]` prefix to all MCP-related log lines
- Separate log file option: `--mcp-log /var/log/stapler-squad-mcp.log`
- Structured logging with `service: "mcp"` field (for centralized log aggregation)

### Resource Limits

**Challenge**: Tool calls might hang (infinite command in PTY, agent never sends disconnect).

**Mitigation**:
- Tool timeouts: 30–60s per tool call
- Memory limit on buffered output: abort if scrollback exceeds 10 MB
- Max concurrent sessions in PTY: cap at 32 simultaneous reads/writes
- Kill session on agent disconnect: cleanup via file descriptor close

### Observability

**Recommendation**: Instrument MCP tools with OpenTelemetry (stapler-squad already uses OTEL).

- Create span per tool call: `mcp.tool_call{tool_name, session_id, status, duration}`
- Trace streaming results: `mcp.output_chunk{bytes, latency}`
- Metrics: tool error rate, latency p50/p95/p99, output chunk size histogram

---

## Prior Art and Lessons Learned

### Fuel Forge MCP Architecture (Kotlin JVM)

**Context**: Fuel Forge (Fanatics multi-agent orchestration engine) runs a Kotlin MCP server for agent self-orchestration.

**Stack**: Kotlin + Ktor + Exposed ORM + `mcp-kotlin-sdk`

**Key patterns**:
- **MCP as universal bus**: Every orchestration capability (task creation, CI, agent assignment) exposed as MCP tool, not REST
- **Three-tier signaling**: MCP sampling (preferred) → logging fallback → signal file poll (graceful degradation when MCP doesn't support sampling)
- **Audit logging**: Full MCP tool audit log (tool name, action, agent, params, status, duration)

**Lessons**:
- ✅ MCP-first design (agents and humans use same interface) is powerful for self-orchestration
- ✅ SDK immaturity (Kotlin SDK nascent like Go) is manageable with clear fallbacks
- ⚠️ Circular dependency between orchestration and MCP modules requires careful interface extraction (avoid tight coupling)
- ⚠️ Tool versioning missing → risk of silent breaking changes; recommend version field in tool schema

**Applicable to Stapler Squad**:
- Design MCP tools to be the primary orchestration interface, not secondary
- Plan fallback (HTTP REST) from day one, even if MVP uses MCP
- Add versioning to tool schemas (e.g., `create_session_v1`, `create_session_v2`)

### Claude Resources (TypeScript MCP Server)

**Context**: Claude Resources protocol for exposing files, logs, database records as MCP resources.

**Key pattern**: 
- **Resource streaming**: Files served as text/binary URIs, not tool results
- **Lazy loading**: Resources are listed but not fetched until client requests
- **MIME type hints**: Help clients understand content type (text/plain, application/json, etc.)

**Applicable to Stapler Squad**:
- Plan for resource API (e.g., `resource://session/{id}/scrollback` for file-like access to PTY output)
- Use resources for large static content (logs, diffs), tools for actions (create, write_input)
- MCP resource specs can replace HTTP endpoints for unified agent access

---

## Open Questions

1. **Does `modelcontextprotocol/go-sdk` exist and support stdio transport?** [TRAINING_ONLY — verify status]
   - **Impact**: If missing, Option 1 collapses; pivot to Option 3
   - **Action**: Prototype early (day 1); try `go get` and compile hello-world MCP server
   
2. **Does MCP protocol require bidirectional stream (stdio), or is request-response OK?**
   - **Impact**: Affects streaming terminal output design
   - **Action**: Read spec; test if tool results can be streamed back vs one-shot response
   
3. **Which Claude Code version(s) will consume the MCP server?** [TRAINING_ONLY — check current spec]
   - **Impact**: Determines transport (stdio vs HTTP)
   - **Action**: Coordinate with Claude Code team on MCP transport support
   
4. **How should large terminal output (>10 MB) be handled?**
   - **Options**: Pagination (offset/limit), streaming chunks, resource API
   - **Impact**: Usability and LLM context efficiency
   - **Action**: Survey comparable tools (IDE plugins, LS servers); design pagination early
   
5. **Should MCP server run as separate mode (--mcp flag) or coexist with HTTP?**
   - **Option A**: Dual-mode (--mcp disables HTTP) — simpler, cleaner separation
   - **Option B**: Coexist on /api/mcp endpoint — more flexible, harder to debug
   - **Impact**: Deployment and testing complexity
   - **Recommendation**: Option A for MVP; pivot to B if agent framework requires HTTP fallback

---

## Recommendation

### MVP (Phase 1): Go Embedded, Stdio Transport

**Decision**: Implement Option 1 (embedded Go MCP server with stdio transport).

**Rationale**:
1. **Language alignment**: Stapler Squad is Go; avoid polyglot overhead
2. **Latency**: Zero inter-process latency; critical for interactive terminal I/O
3. **Deployment**: Single binary; agents spawn `stapler-squad --mcp` and read stdin/stdout
4. **MVP scope**: Sufficient for core tools (create_session, list_sessions, read_terminal, write_input)
5. **Fallback plan**: If Go SDK blocks (missing features), pivot to TypeScript sidecar (1–2 day rewrite)

**Milestones**:
- **Week 1**: Prototype Go MCP server with single tool (`list_sessions`); verify transport
- **Week 2**: Implement full tool set (create, destroy, read/write terminal, get status)
- **Week 3**: Integration testing with Claude Code; stress-test streaming terminal output
- **Week 4**: Production release (MVP feature complete)

**Exit criteria**:
- ✅ Go SDK can be integrated (no blockers)
- ✅ Stdio transport works end-to-end
- ✅ Terminal I/O streaming tested with long-running commands
- ✅ At least one LLM agent (Claude Code) can use the MCP server without manual setup

### Fallback (if Go SDK blocks): TypeScript Sidecar (Week 3)

If Go SDK proves insufficient (e.g., resource API missing, streaming broken):

**Decision**: Switch to Option 3 (separate TypeScript MCP server using @modelcontextprotocol/sdk).

**Milestones**:
- Rewrite `server/mcp/` in TypeScript using @modelcontextprotocol/sdk
- Add HTTP client to talk to stapler-squad (localhost:8543)
- Ensure feature parity with Go prototype
- Test terminal I/O streaming via HTTP chunking

**Timeline**: 2–3 days (if tooling prepared, longer if starting from scratch)

---

## Pending Web Searches

To verify training knowledge and fill [TRAINING_ONLY] gaps, the parent agent should run these exact searches:

1. **"modelcontextprotocol/go-sdk GitHub current status 2025"**
   - Goal: Verify Go SDK exists, supports stdio, has active maintenance
   - Uncertainty: Training cutoff (Feb 2025); SDK maturity may have changed

2. **"MCP stdio transport specification requirements"**
   - Goal: Confirm stdio is required for non-HTTP clients; understand bidirectional semantics
   - Uncertainty: Whether tool results stream back or return one-shot

3. **"Claude Code MCP integration 2026 supported transports"**
   - Goal: Which transports does Claude Code v2.0+ support? (stdio, HTTP, SSE, WebSocket)
   - Uncertainty: Training data may predate Claude Code MCP support

4. **"Kotlin MCP SDK Fuel Forge lessons learned server design"**
   - Goal: Validate claims about Fuel Forge architecture; extract patterns
   - Uncertainty: May not be public; internal Fanatics project details unclear

5. **"MCP resource API specification file streaming"**
   - Goal: Understand resource type (planned for future phases)
   - Uncertainty: Whether resources exist in current MCP spec or are proposed

6. **"MCP tool streaming results timeout latency recommendations"**
   - Goal: Best practices for tool result timeouts, output buffering, LLM context limits
   - Uncertainty: No public guidance; may require synthesis from similar protocols (LSP, DAP)

---

**Document version**: 1.0 | **Last updated**: 2026-04-18 | **Status**: Approved for MVP prototyping

## Web Search Results

### Query: "modelcontextprotocol/go-sdk GitHub 2025 2026 status"
**Result**: The official `modelcontextprotocol/go-sdk` exists, maintained in collaboration with Google. It is active but was marked unstable with breaking changes through ~mid 2025. By 2026 it supports the 2025-06-18 MCP spec version. Community alternatives: `mark3labs/mcp-go` (mature, widely used), `riza-io/mcp-go`, `dwrtz/mcp-go`.
- Source: https://github.com/modelcontextprotocol/go-sdk

### Query: "MCP stdio transport specification 2025 streamable HTTP"
**Result**: MCP defines exactly two official transports: **stdio** (for local processes) and **Streamable HTTP** (for remote deployments). SSE (HTTP+SSE) from the 2024-11-05 spec is deprecated — replaced by Streamable HTTP in 2025-03-26. Stdio is bidirectional over stdin/stdout; client MUST NOT write non-MCP messages.
- Sources: https://modelcontextprotocol.io/specification/2025-03-26/basic/transports, https://blog.fka.dev/blog/2025-06-06-why-mcp-deprecated-sse-and-go-with-streamable-http/

### Query: "Claude Code MCP server integration 2026 supported transports"
**Result**: Claude Code supports stdio, SSE (deprecated but working), and HTTP. **Best practice**: stdio for local, HTTP for remote. SSE is interim only. This confirms stdio is the right transport for a local stapler-squad MCP server.
- Source: https://code.claude.com/docs/en/mcp

**Updated recommendations based on search:**
- [TRAINING_ONLY] marks on Go SDK maturity resolved: `mark3labs/mcp-go` is mature and widely used as of 2026; official SDK is also viable
- Stdio transport is confirmed as the correct choice for local MCP servers
- SSE is deprecated; do not build on it
