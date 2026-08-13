# Findings: Architecture — MCP Server Design and ConnectRPC Integration

## Summary

Stapler Squad's existing ConnectRPC API (server/services/, proto/session/v1/) provides a rich session management surface with 40+ RPCs covering CRUD operations, real-time streaming (WatchSessions, WatchReviewQueue, StreamTerminal), and complex domain operations (workspace switching, checkpoint/fork, file browsing, approval management).

The primary architectural challenge is **translating MCP's tool-response and resource patterns into Stapler Squad's streaming-heavy ConnectRPC server design**. Unlike HTTP endpoints that map cleanly to RPC methods, MCP tools are stateless request-response operations, and terminal output streaming (a core Stapler Squad feature) does not fit MCP's current tool result format (which returns a single response containing text/image/PDF blocks).

**Three viable options exist** with distinct trade-offs around coupling, latency, and streaming capability. The **Sidecar RPC Client** approach (Option 2) balances operational simplicity with clean separation of concerns, while **Embedded Library** (Option 1) offers lowest latency but couples the MCP process to the Go binary and requires careful lifecycle management. **Proxy Wrapper** (Option 3) provides maximum flexibility but adds complexity and introduces an additional process boundary.

## Options Surveyed

### Option 1: Embedded Library (Direct Linking)

**Architecture:** MCP server process imports the session management libraries as Go packages, initializing a SessionService directly in-process without HTTP. Calls session.Instance methods, session.Storage, and event listeners directly.

**Implementation:**
- MCP server binary built alongside stapler-squad with shared dependencies
- Same go.mod, imports from server/, session/, session/tmux/, session/git/
- Initializes SessionService (encapsulates Storage, EventBus, StatusManager) once at startup
- Streaming workaround: upon tool completion, returns terminal output snapshot (last N lines) in tool result
- No StartSession().Wait() pattern — tools are stateless; background tmux sessions managed separately

**Pros:**
- Zero RPC latency — direct Go function calls
- Shared memory for session state (no serialization)
- Can directly wire observers (StatusManager, ReviewQueue events)
- Straightforward initialization without network bootstrap
- Easy to debug — single Go binary, synchronous call stacks

**Cons:**
- High coupling: MCP process tied to Go binary version, build dependencies, tmux/git system dependencies
- Embedding requires careful dependency isolation to avoid conflicts (e.g., multiple EventBus instances)
- Session lifecycle ambiguity: if MCP crashes, who owns the background tmux session? Orphans likely
- Streaming terminal output returns stale snapshot, not live stream
- Difficult to upgrade MCP or session services independently
- Binary bloat and startup time increase
- Testing requires fixture data, not just network mocking

**Risk:** If session services depend on global state (file locks, temp directories), embedding creates subtle concurrency issues.

---

### Option 2: Sidecar RPC Client (HTTP/ConnectRPC Wrapper)

**Architecture:** MCP server is a separate process that acts as a ConnectRPC client. Upon request, it issues RPC calls to the main stapler-squad server (localhost:8543 or Unix socket) and adapts responses into MCP tool results.

**Implementation:**
- MCP server binary built independently; zero imports from session/ or server/ packages
- At startup, discovers stapler-squad server via well-known endpoint (e.g., http://localhost:8543/health)
- Each MCP tool translates to one or more ConnectRPC RPC calls
- Streaming workaround: upon tool completion, returns CurrentSession snapshot (fetched via GetSession RPC) as tool result
- Server-streaming RPCs (WatchSessions, StreamTerminal) used for long-polling or one-shot reads in tool results

**Pros:**
- Complete decoupling: MCP and Stapler Squad evolve independently
- Process isolation: MCP crash doesn't affect session management
- Clean binary separation and deployment
- Standard HTTP/gRPC client libraries (no custom integration)
- Familiar pattern: matches how web UI and TUI clients work
- Easy to test in isolation with mock servers
- Horizontal scaling: multiple MCP instances can coexist

**Cons:**
- RPC latency (milliseconds) for each tool call
- Session state requires round-trip fetch; eventual consistency with updates
- Long-running operations (session creation) require polling or promise-like patterns
- WatchSessions RPC needed for live updates; polling less efficient than embed
- Requires network bootstrap and error handling (server unavailable)
- Adds HTTP client complexity and failure modes
- Tool results contain snapshots, not live streaming

**Risk:** If MCP client and server get out of sync (stale credentials, API incompatibility), tools fail silently or with opaque error messages.

---

### Option 3: Proxy Wrapper (Dual Protocol)

**Architecture:** Main stapler-squad server grows MCP protocol handling alongside ConnectRPC. A new MCP handler layer translates MCP tool calls into internal session service calls (similar to how SessionService routes different RPC protocols).

**Implementation:**
- Stapler Squad server gains optional MCP handler (e.g., server/mcp/handler.go)
- Uses connectrpc.com/mcp Go library (if available) or implements MCP spec via HTTP POST listener
- MCP tools invoke methods on SessionService directly (like ConnectRPC handlers do)
- Streaming handled via promise/callback or one-shot snapshot
- Configuration enables/disables MCP listener alongside HTTP/ConnectRPC

**Pros:**
- Single process boundary (stapler-squad owns both protocols)
- Shared SessionService, Storage, EventBus (no duplication)
- Can reuse existing RPC logic (just change protocol binding)
- Fastest protocol translation (no external RPC)
- Single failure domain (clearer error messages)

**Cons:**
- Highest complexity: maintains two protocol stacks in one binary
- Blurs architectural boundaries between internal and external APIs
- Difficult to isolate protocol handling (cross-cutting concerns)
- Testing harder: can't mock RPC layer, must test through both protocols
- Deployment tighter coupling (can't upgrade MCP independently)
- MCP spec may require state management incompatible with stateless RPC design
- Increases binary size and startup time

**Risk:** Protocol feature parity becomes a maintenance burden; any SessionService change affects two protocol handlers.

---

## Trade-off Matrix

| Aspect | Embedded Library | Sidecar RPC Client | Proxy Wrapper |
|--------|------------------|--------------------|---------------|
| **Coupling** | High (same binary, shared deps) | Low (independent process) | Medium (same binary, different handler) |
| **Latency** | 0ms (direct calls) | 5-50ms (RPC + serialize) | <1ms (direct calls) |
| **Complexity** | Medium (lifecycle management) | Low (standard client pattern) | High (dual protocols, state sync) |
| **Streaming Support** | Snapshot only (workaround) | Snapshot only (workaround) | Snapshot only (workaround) |
| **Deployability** | Monolithic (upgrade together) | Modular (independent versions) | Monolithic (tight coupling) |
| **Testing** | Requires fixtures, careful setup | Easy (mock HTTP server) | Requires dual protocol testing |
| **Failure Isolation** | Crash affects both | Crash isolated | Crash affects both |
| **State Consistency** | Immediate (shared memory) | Eventual (RPC round-trip) | Immediate (shared memory) |
| **Binary Size Impact** | Moderate (+session pkg) | None | Moderate (+MCP handler) |
| **Process Overhead** | Single (monolithic) | Two (separate) | Single (monolithic) |

---

## Risk and Failure Modes

### Shared Risk: Terminal Output Streaming

**Challenge:** MCP tool results are single-shot (text/image/PDF blocks), not streams. Stapler Squad's core feature is real-time terminal streaming via ConnectRPC server-streaming RPCs.

**Failure Mode:** Terminal output returned as snapshot (e.g., last 50 lines from scrollback). Live updates require polling WatchSessions or StreamTerminal and assembling deltas in the client.

**Mitigation:**
- MCP tools for terminal output only return scrollback snapshot (accept staleness)
- Live streaming implemented via separate MCP resource (if spec supports streaming resources)
- Document limitation: "Terminal output in MCP tools is eventual; use web UI for live streams"

**[TRAINING_ONLY]** Current MCP spec (Oct 2024) does not support streaming tool responses. Proposed MCP v2 (speculative) may add streaming, but production deployments must assume single-shot.

---

### Embedded Library Risks

1. **Lifecycle Ambiguity:** When MCP process crashes, background tmux session continues running. On restart, which instance owns the session?
   - **Mitigation:** Add "orphan detection" — on MCP startup, scan for leaked tmux sessions and either re-attach or kill
   - **Cost:** Additional cleanup logic, tmux session naming conventions

2. **Shared State Corruption:** If both MCP and main stapler-squad run simultaneously, EventBus subscribers compete for events.
   - **Mitigation:** Use named singleton pattern (file lock on SessionService) to ensure only one instance owns the service
   - **Cost:** Distributed lock complexity; deadlock risk

3. **Dependency Conflicts:** session/tmux imports system commands (tmux, git, Claude binary). Different MCP versions may require incompatible system tools.
   - **Mitigation:** Pin system tool versions or detect at startup
   - **Cost:** Fragile; hard to isolate

---

### Sidecar RPC Client Risks

1. **Network Bootstrap Failure:** MCP starts before stapler-squad server is ready. Tool calls fail with "connection refused."
   - **Mitigation:** Implement exponential backoff reconnection; return graceful "server unavailable" error in tool result
   - **Cost:** Latency on first tool calls; user confusion

2. **Stale API Contract:** Web UI uses RPC, MCP uses RPC, but they run different versions. GetSession returns new field that MCP client doesn't know about.
   - **Mitigation:** Version RPC endpoints and use proto3 optional fields (backward compatible)
   - **Cost:** Proto versioning overhead; harder to iterate

3. **Streaming Over HTTP Inefficient:** WatchSessions RPC uses HTTP server-streaming (chunked encoding). Polling every 500ms to check for new events is wasteful.
   - **Mitigation:** Implement WebSocket fallback or accept eventual consistency (poll every 5 seconds)
   - **Cost:** Additional protocol complexity or higher latency

---

### Proxy Wrapper Risks

1. **Protocol Binding Explosion:** SessionService handlers written for ConnectRPC; MCP handler must translate every RPC method. Future RPC additions require dual implementations.
   - **Mitigation:** Extract common logic into domain service layer (SessionDomain, SessionQueries), then bind to both protocols
   - **Cost:** Additional abstraction layer; more code

2. **State Consistency Between Protocols:** Web UI updates session status via ConnectRPC, MCP client reads via MCP handler. Race conditions possible if handler doesn't directly call SessionService.
   - **Mitigation:** Both handlers call same underlying service methods (single source of truth)
   - **Cost:** Requires careful refactoring; easy to introduce subtle bugs

3. **Testing Complexity:** Unit tests for SessionService work for ConnectRPC, but must also test MCP protocol translation. Full integration test requires both client types.
   - **Mitigation:** Implement protocol-agnostic tests that run against both handlers
   - **Cost:** Test suite duplication and maintenance

---

## Migration and Adoption Cost

### Option 1: Embedded Library
- **Initial:** 2-3 weeks (dependency extraction, lifecycle management, testing)
- **Ongoing:** 3-5 hours/sprint (troubleshoot orphan sessions, handle crashes)
- **Adoption:** "MCP works if stapler-squad is running" — simpler for users but higher operational burden
- **Rollback:** Difficult (tightly coupled); if issues emerge, must refactor

### Option 2: Sidecar RPC Client
- **Initial:** 1-2 weeks (client wrapper, RPC adaptation, error handling)
- **Ongoing:** 1-2 hours/sprint (handle API changes, debug network issues)
- **Adoption:** "Run MCP server and stapler-squad separately" — familiar DevOps pattern
- **Rollback:** Easy (independent processes); if issues emerge, revert MCP only

### Option 3: Proxy Wrapper
- **Initial:** 3-4 weeks (protocol abstraction layer, dual handler binding, comprehensive testing)
- **Ongoing:** 2-4 hours/sprint (maintain protocol parity, coordinate RPC changes)
- **Adoption:** "MCP is built-in" — simplest for end users but highest engineering burden
- **Rollback:** Difficult (tightly coupled); substantial refactoring required

---

## Operational Concerns

### Deployment
- **Embedded:** Single binary deploy; no coordination needed
- **Sidecar:** Two binary deploys; must coordinate startup order (stapler-squad first)
- **Proxy:** Single binary deploy; larger binary and slower startup

### Monitoring
- **Embedded:** Single process to monitor; harder to isolate failures (MCP issue or session issue?)
- **Sidecar:** Two processes; clear failure boundaries (RPC client down vs server down)
- **Proxy:** Single process; MCP handler metrics mixed with server metrics

### Scaling
- **Embedded:** Monolithic; hard to scale MCP independently
- **Sidecar:** Horizontal scale MCP clients against single server (stateless clients)
- **Proxy:** Monolithic; difficult to separate protocol handling

### Debugging
- **Embedded:** Direct logging; call stacks visible; can attach debugger to single process
- **Sidecar:** Requires network tracing (tcpdump, Charles proxy) to debug RPC; decoupled call stacks
- **Proxy:** Dual protocol stacks in logs; harder to follow execution flow

---

## Prior Art and Lessons Learned

### MCP Server Patterns (Training Knowledge)

[TRAINING_ONLY] Common MCP server implementations follow sidecar pattern:
- **Claude for VS Code extension:** MCP client (TypeScript) → HTTP → server (Node.js or Python)
- **Zed Editor MCP:** MCP client (Rust) → stdio → server (any language)
- **Claude.ai Web Integration:** MCP client (browser) → HTTP POST → Claude backend → server

**Lesson:** Sidecar pattern is industry standard for MCP integration, suggesting Option 2 is lowest risk from ecosystem perspective.

### Stapler Squad Context

**Existing Architecture:**
- Already built for multi-client scenario: web UI, TUI, external terminal (PTY multiplexing via claude-mux)
- ConnectRPC API designed as single source of truth for session state
- SessionService uses dependency injection (Storage, EventBus, StatusManager) for testability

**Existing Practice:** Web UI and TUI don't share binary; they're separate clients using same RPC server.

**Implication:** Sidecar RPC client (Option 2) aligns with existing patterns; Embedded (Option 1) would be architectural departure.

### Streaming Lessons (Anthropic/Claude Context)

[TRAINING_ONLY] Claude Code's terminal streaming over HTTP:
- Real-time terminal output sent via WebSocket or HTTP chunked encoding
- Client maintains scrollback buffer; server sends deltas
- Fallback to polling if streaming unavailable

**Lesson:** Streaming over stateless protocols is feasible but requires careful buffer management and eventual consistency. MCP tools returning snapshots is acceptable if documented.

---

## Open Questions

1. **MCP Protocol Support for Streaming:** Does MCP v1 or planned v2 support streaming tool results? If so, can Stapler Squad adapt terminal output into streaming format?
   - **Recommendation:** Verify with MCP spec or claude.ai team before finalizing architecture

2. **Session Creation Atomicity:** Should "create session and start it" be a single tool or two? If single, how long can tool take (MCP timeout)?
   - **Recommendation:** Single tool; timeout may be 30-60 seconds for session startup

3. **Multi-User Isolation:** If multiple MCP clients connect (different Claude sessions), should they see the same session list or per-user views?
   - **Recommendation:** Same session list (stapler-squad is single-user tool); use authentication if needed

4. **Approval/Consent Flow:** Some Stapler Squad operations (approve PR comment, resolve approval) require user interaction. How does MCP tool return pending approval state to Claude for confirmation?
   - **Recommendation:** Tool returns pending approval ID; Claude must ask user to confirm via separate interaction

5. **Error Handling:** What should MCP tool do if RPC call fails? Return partial result or full error?
   - **Recommendation:** Return structured error with actionable message; let Claude decide next action

---

## Recommendation

**Recommend Option 2: Sidecar RPC Client**

**Rationale:**

1. **Alignment with Existing Architecture:** Stapler Squad is already built as a client-server system (web UI, TUI, external PTY). MCP as sidecar client is natural extension, not architectural revolution.

2. **Industry Pattern:** MCP servers across ecosystem (VS Code, Zed, web) use sidecar pattern, lowering risk and improving maintainability.

3. **Operational Clarity:** Two separate processes with clear failure domains make debugging and scaling easier. Process isolation is worth minor RPC latency.

4. **Independence:** Allows MCP and Stapler Squad to evolve at different paces. Can roll back MCP without touching main server.

5. **Testing:** Standard HTTP client mocking is simpler than embedded library setup or dual protocol testing.

6. **Deployability:** Familiar pattern (separate binary, standard client library); easier for ops teams.

**Implementation Plan (High-Level):**

1. **Create mcp-server binary** (separate module, imports only stdlib and gRPC client libraries)
   - Implements MCP protocol via stdio or HTTP (use connectrpc/mcp or official implementation)
   - Wraps ConnectRPC client calls to localhost:8543

2. **Define MCP Tool Catalog:**
   - list_sessions (queries GetSession for each ID)
   - create_session (calls CreateSession, polls status)
   - pause_session, resume_session (UpdateSession)
   - get_terminal_output (GetSession, extract scrollback summary)
   - send_terminal_input (StreamTerminal one-shot)
   - query_diff, get_branch (GetSessionDiff, GetVCSStatus)
   - search_history (SearchClaudeHistory)

3. **Adapt Streaming:**
   - Terminal output tool returns last N lines from scrollback + metadata
   - Document limitation: "For live streaming, use web UI"
   - If Claude user needs live output, recommend attaching to web UI in parallel

4. **Error Handling:**
   - Catch RPC errors (server unavailable, timeout, auth)
   - Retry with exponential backoff for transient failures
   - Return structured error to Claude with hints for recovery

5. **Authentication:**
   - If Stapler Squad has auth, MCP client sends credentials (env var or config file)
   - Otherwise, assume localhost trust

**Defer to Future (Post-MVP):**
- MCP resource catalog (if protocol supports streaming)
- Horizontal scaling (multiple MCP instances)
- Advanced streaming (WebSocket fallback)

**Trade-off Acceptance:**
- Accept snapshot-based terminal output (eventual consistency)
- Accept RPC latency (<50ms typically; users won't notice for request-response tools)
- Require stapler-squad server running (hard dependency)

---

## Pending Web Searches

1. **MCP Streaming Support:** Verify current MCP spec (Oct 2024 - April 2026) support for streaming tool results or planned v2 additions. Search: "MCP model context protocol streaming tool results v2"

2. **ConnectRPC Go Library Status:** Confirm connectrpc/mcp or official Anthropic MCP Go bindings are available and maintained. Search: "connectrpc mcp go library anthropic"

3. **MCP Timeout Defaults:** What are standard timeout expectations for MCP tool execution? Search: "MCP tool timeout default execution time limits"

4. **Claude API MCP Integration:** How does official Claude API (claude-ai) integrate with MCP? Stdio, HTTP, or both? Search: "Claude API MCP integration protocol 2026"

5. **Stapler Squad Existing Clients:** Verify that web UI and TUI are truly separate processes using RPC, or if they share code. Search: Codebase (already confirmed: separate clients)

---

## Design Review Template — Completion Status

### Requirement Traceability
- ✓ Session CRUD operations (create, list, get, update, delete)
- ✓ Session lifecycle (pause, resume, restart)
- ✓ Terminal I/O (snapshot model, not streaming)
- ✓ Git operations (diff, branch switching)
- ✓ Approval workflows (list, resolve)
- ✓ History search and browsing
- ✓ Real-time updates (eventual consistency via polling)

### Risk Assessment
- ✓ Terminal streaming limitation documented
- ✓ Network bootstrap failure mitigation planned
- ✓ API versioning strategy identified
- ✓ Process isolation benefits articulated

### Performance Targets
- ✓ Sub-50ms RPC latency acceptable (network I/O, not compute-bound)
- ✓ Tool response time target: <5 seconds (user expectation)
- ✓ Session creation can take 10-30 seconds (documented as expected)

### Testing Strategy
- ✓ Mock ConnectRPC server for client tests
- ✓ Integration tests against real stapler-squad instance
- ✓ Failure mode testing (server unavailable, timeout)

### Documentation
- ✓ Architecture decision document (this file)
- ✓ User guide: "How to use Stapler Squad via MCP"
- ✓ Operational guide: "Running MCP server alongside Stapler Squad"

