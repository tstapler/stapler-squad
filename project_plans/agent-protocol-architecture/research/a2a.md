# A2A (Agent2Agent Protocol) — Research

## What A2A Solves

A2A standardizes how one AI agent invokes another **opaque, independently-hosted** agent across
a trust/network boundary — it does not expose the callee's internal reasoning, prompts, or tool
calls, only a task/message interface. The model:

- **Discovery**: an agent publishes an **AgentCard** — a JSON manifest at a well-known URL
  (`/.well-known/agent-card.json`) describing its skills, supported input/output modes, and
  required auth. A caller fetches this card to learn what the remote agent can do and how to
  authenticate to it.
- **Invocation**: the caller sends a **Task** to the remote agent. Three transport bindings are
  defined as functionally equivalent: JSON-RPC 2.0, HTTP+JSON/REST, and gRPC. JSON-RPC 2.0 remains
  the reference implementation in most SDKs; REST and gRPC are official, not afterthoughts.
- **Streaming / long-running work**: Server-Sent Events (SSE) for incremental updates, plus
  webhook-style push notifications for tasks that outlive a single HTTP connection.
- **Task lifecycle**: 8 states — `SUBMITTED`, `WORKING`, `INPUT_REQUIRED`, `AUTH_REQUIRED`,
  `COMPLETED`, `CANCELED`, `FAILED`, `REJECTED`. `input-required`/`auth-required` are non-terminal
  and resumable: the caller sends a follow-up `Message` against the same task ID, which is how
  multi-turn clarification works without a new task.
- **Message/artifact model**: `Message` objects carry `parts[]` of three kinds — `TextPart`,
  `FilePart` (base64 bytes or URL reference), `DataPart` (structured JSON). Task outputs live in a
  separate `artifacts[]` array on the Task using the same Part structure, distinct from message
  history (so "the conversation" and "the deliverable" don't get conflated).
- **Auth**: the AgentCard declares OpenAPI-style `securitySchemes` (API key, HTTP Basic/Bearer,
  OAuth2 flows, OIDC, mTLS). There is no A2A-native trust-establishment layer beyond that — trust
  between agents, and transitively in whatever MCP tools a remote agent uses internally, is left to
  the implementer. This is the most commonly cited gap in critiques of the spec.

## Governance and Maturity

Created by Google, announced April 2025 with 50+ initial partners (Atlassian, Salesforce, SAP,
ServiceNow, Box, Cohere, Intuit, LangChain, MongoDB, PayPal, Workday, plus SIs). Donated to the
**Linux Foundation**; the project formally launched under LF in June 2025. Governance is now a
**Technical Steering Committee** including Google, Microsoft, AWS, Cisco, Salesforce, ServiceNow,
SAP, and IBM (IBM joined after merging its own competing "Agent Communication Protocol" into A2A
in August 2025 — note the acronym collision with Zed's unrelated "Agent Client Protocol," also
abbreviated ACP, which is a separate candidate in this comparative research set).

Current spec version is v1.0.1. Adoption claims as of the one-year retrospective (~150+
organizations, presence in major cloud platforms, some enterprise production use). Still actively
governed (TSC + recent IBM merger), not yet frozen, but past the "single-vendor experiment" stage.

Notable public criticism (e.g. Hacker News threads "why A2A sucks," "Is A2A the Future?") argues
the direct agent-to-agent RPC/chat model is the wrong distributed-systems primitive — critics
prefer event/pub-sub architectures — and that backpressure, burst management, and cross-agent trust
are pushed onto implementers unsolved by the spec itself.

## Relationship to MCP

Confirmed directly from Google's own launch post: "A2A is an open protocol that complements
Anthropic's Model Context Protocol (MCP), which provides helpful tools and context to agents." The
standard framing — MCP is vertical (model ↔ tools/data), A2A is horizontal (agent ↔ agent) — holds
up and is echoed by Microsoft's dual support for both in Azure AI Foundry/Copilot Studio. No
evidence MCP has grown agent-delegation features that overlap A2A; if anything the noted friction
runs the other way — translating an inbound A2A task into a sequence of local MCP tool calls at the
boundary is called out as the actual integration work, not something either spec automates for you.

## Fit Against stapler-squad's Actual Architecture

The audit's framing is right, and the code confirms it: **stapler-squad has no cross-process or
cross-network agent delegation anywhere today.** Every mechanism that "drives an agent" is one Go
process controlling a subprocess or tmux pane it spawned itself, on the same machine, with no
opaque boundary and no remote party:

- `session/session_driver.go` — a goroutine that watches `tmux` pane text via regex, answers
  startup dialogs, and types keystrokes into a Claude Code CLI process the same Go binary started.
  There is no "callee agent" here in the A2A sense — it's terminal automation of a local process.
- `session/autonomous_driver.go` (`AutonomousDriver`) — polls the same local session for idle
  status, calls `d.headlessPool.CallBlockingWithOptions(...)` to get an orchestrator decision
  (`NEXT_MESSAGE:` / `DONE:`), then injects that text into the same tmux pane via
  `d.controller.SendCommandImmediate`. The "headless pool" call is itself another local `claude -p`
  subprocess (see `session/headless/runner.go`, `session/headless/pool.go`) — not a network call to
  a peer agent, just a second local LLM invocation used as a planner for the first.
- `session/headless/` — `ClaudeRunner.Run` execs the local `claude` binary
  (`executor.StartProcess`), captures stdout, and applies a stripped-down, allow-listed environment
  (`claudeAllowedEnvPrefixes`). Session pooling exists only to reuse prompt-cache prefixes and cap
  concurrency — it is a local resource manager, not a discovery/auth layer for external agents.
- `server/services/backlog_service.go` (`SpawnSessionFromItem`, `TriggerTriage`,
  `TriggerReReview`) — spawns a local session/worktree and optionally attaches an
  `AutonomousDriver` to it, or calls the headless pool directly for triage classification
  (`s.headlessPool.CallBlockingWithOptions` at line ~1222). Same process, same host.
- `session/backlog_plugin_github.go` and `session/backlog_plugin_github_prs.go` — polling REST
  clients against the GitHub Issues/Pulls APIs. These fetch **data** (issue/PR metadata) to
  populate the backlog; they never invoke another AI agent, never send a Task, and have no concept
  of an AgentCard or capability negotiation. This is the closest thing in the codebase to "talking
  to an external service," and it's a plain data-ingestion polling plugin, not agent delegation.
- `server/push/notifier.go` — one-way outbound notification delivery (Web Push today; comment notes
  future email/Slack). Strictly a notification sink, not a task-delegation channel; nothing calls
  back into stapler-squad as a peer agent.

In short: today's problem is **intra-process control of one locally-owned CLI tool** (readiness
detection, keystroke injection, turn-taking, stateless one-shot calls) — exactly the problem A2A
explicitly does not address. A2A's entire value proposition (AgentCard discovery, cross-boundary
auth, opaque black-box task delegation to an agent you don't control) has no counterpart in
stapler-squad's current architecture, because there is no second, independently-owned agent
anywhere in the flow. Adopting A2A now would mean building a discovery/auth/task-envelope layer to
solve a delegation problem stapler-squad does not have, while leaving the actual audited pain
points (regex-based readiness detection, two inconsistent driving mechanisms, no single
"drive a session" abstraction) completely untouched — those are session/process-control problems,
which is squarely ACP's (Agent Client Protocol) territory, not A2A's. See `research/acp.md` for
that comparison.

## Verdict: Not Relevant (Now) — Adopt-Later-If-X

**Not relevant today.** A2A solves inter-agent delegation across a trust boundary; stapler-squad's
current architecture has exactly one agent (Claude Code, or whichever CLI is configured) fully
owned and locally controlled by the stapler-squad process itself. There is no second agent to
delegate to, discover, or authenticate against. Introducing A2A now would add a protocol layer with
no corresponding problem, and would not fix the terminal-scraping driver, the driver/AutonomousDriver
duplication, or the missing single session-control abstraction — the actual items in the audit.

**Condition that would make it relevant:** if stapler-squad ever needs to farm work out to agents it
does *not* start and does not fully control — e.g. (a) a fleet of remote/cloud-hosted agent workers
that stapler-squad dispatches backlog items to instead of spawning a local tmux+worktree session,
(b) interoperating with another team's/vendor's agent platform that exposes its own AgentCard
(e.g. a hosted review agent, a hosted triage service run by someone else), or (c) stapler-squad
itself wanting to expose "spawn and manage a coding session" as a capability other tools/agents can
call into via a published AgentCard (turning stapler-squad into an A2A *server*, not just a
client). None of these exist in the current codebase; if backlog auto-triage or session execution
ever moves off-box to agents outside stapler-squad's own process boundary, A2A (rather than a
bespoke webhook/REST scheme) becomes worth revisiting at that point.

## Sources

- https://github.com/a2aproject/A2A
- https://a2a-protocol.org/latest/specification/
- https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability/
- https://www.linuxfoundation.org/press/linux-foundation-launches-the-agent2agent-protocol-project-to-enable-secure-intelligent-communication-between-ai-agents
- https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year
- https://lfaidata.foundation/communityblog/2025/08/29/acp-joins-forces-with-a2a-under-the-linux-foundations-lf-ai-data/
- https://news.ycombinator.com/item?id=48413946
