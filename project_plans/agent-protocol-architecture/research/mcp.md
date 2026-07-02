# MCP (Model Context Protocol) — Research

## What MCP is

MCP is Anthropic's open protocol (donated to broader multi-vendor governance; OpenAI and Google
have since backed it) for connecting an LLM client to external tools/data. It is JSON-RPC 2.0 on
the wire. A server exposes three primitive types — **tools** (executable actions), **resources**
(read-only data), **prompts** (reusable templates) — over one of two transports: **stdio** (local
subprocess, one client per process) or **Streamable HTTP** (remote, supports SSE for streaming,
multi-client). Client and server negotiate capabilities on connect.

### What MCP explicitly does NOT standardize

- Multi-agent orchestration or delegation between agents (that's the A2A/ACP space).
- Session/process lifecycle of *the agent itself* — MCP's "lifecycle" (Initialization → Operation
  → Shutdown) is the client↔server *connection* lifecycle, not anything about how an agent's own
  execution turns, tmux panes, or worktrees are driven.
- Cross-session state persistence — MCP is stateless about anything beyond a single connection;
  servers own their own state stores.
- Authentication/authorization at production scale — OAuth 2.1 was added, but adoption in the
  wild is inconsistent; most deployed servers still have weak or no auth.

### Maturity (as of mid-2026)

- Explosive first-18-months adoption: 3,000+ published servers, official SDKs (TS, Python, C#,
  Java, Swift), every major IDE/agent client integrated support. A 2026-07-28 spec release
  candidate is in flight; the protocol is still evolving with breaking changes between versions.
- Known production pain points reported industry-wide: JSON-RPC overhead vs. in-process calls
  (irrelevant for stapler-squad's stdio/local-HTTP use), stateful-session-vs-load-balancer
  friction (irrelevant — single-user local server), inconsistent auth, tool-schema drift/lying
  schemas, and "tool overexposure" (too many tools blows context — directly relevant, see below).
- Emerging pattern: MCP servers auto-generated from an OpenAPI spec (source of truth = OpenAPI,
  MCP is a downstream projection). Consistent industry advice: **do not** do this 1:1 — REST is
  resource/CRUD-shaped, MCP tools should be task/workflow-shaped for an LLM caller. Generated
  1:1 tool sets get pruned and rewritten by hand afterward. No credible pattern found for the
  reverse (MCP schema as source of truth, generating REST/UI from it) — every real-world example
  goes API-spec → MCP, not MCP → API/UI, because MCP tool descriptions are written for LLM
  cognition (verbose, role-aware, envelope-wrapped) not for machine-to-machine or human-UI
  consumption.

## Current stapler-squad MCP usage (grounded in code)

- `server/mcp/server.go` — `NewCore()` wires up an `mcpserver.MCPServer` and registers tool groups
  conditionally (backlog/goal tools only if `storage != nil`). Two transports are actually live:
  stdio (`RunServer`, spawned per Claude Code session) and Streamable HTTP (`NewHTTPHandler`,
  mounted at `/mcp` on the existing server) — this is deliberate: sessions can call back into
  stapler-squad either as a subprocess-spawned server or over HTTP without spawning anything.
- Session identity is threaded via `STAPLER_SESSION_UUID` env var → `WithSessionUUID(ctx, uuid)` →
  `sessionUUIDFromContext()` (`server/mcp/tools_backlog.go` lines 20-42). This is how a tool call
  knows *which* stapler-squad session is calling it — it is bespoke plumbing, not anything MCP
  gives you; MCP has no concept of "caller identity tied to a specific external session."
- Tool groups: `tools_discovery.go` (list/get/search sessions), `tools_lifecycle.go`
  (create/pause/resume/stop/update session — this literally drives tmux + git worktree lifecycle
  *through* MCP tool calls, e.g. `createSession` calls `inst.Start(true)`,
  `session.StartSessionDriver(inst, path)`), `tools_terminal.go`, `tools_vcs.go` (git diff/branch
  read tools), `tools_backlog.go` (role-gated: `get_backlog_item`, `report_progress`,
  `request_review`, `submit_review_verdict`, `submit_triage_result` — each checks
  `ItemSession.SessionRole` before allowing the call), `tools_goal.go` (`set_session_goal`,
  `get_session_goal`, `update_session_task`).
- Notably: `create_session` **is itself a session-lifecycle-control tool exposed over MCP** — so
  stapler-squad already uses MCP as a control-plane API for tmux/worktree lifecycle, not just for
  passive data lookups. This matters for the verdict below.

## The RPC/MCP/UI mismatch (grounded in code)

`proto/session/v1/backlog.proto` defines 15 RPCs on `BacklogService`:
`CreateBacklogItem`, `GetBacklogItem`, `ListBacklogItems`, `UpdateBacklogItem`,
`ArchiveBacklogItem`, `TransitionBacklogItemStatus`, `SpawnSessionFromItem`,
**`AttachSessionToItem`**, `TriggerTriage`, `CancelTriage`, `ApprovePlan`, `SuggestNextItem`,
`OverrideVerdict`, `TriggerReReview`, **`TriggerSync`**, `CreateItemSource`, `ListItemSources`,
`UpdateItemSource`, `DeleteItemSource`, `GetSyncHistory`.

`server/mcp/tools_backlog.go` exposes only 5 MCP tools: `get_backlog_item`, `report_progress`,
`request_review`, `submit_review_verdict`, `submit_triage_result`. These map to a *subset* of
the RPC surface and are shaped completely differently (role-gated, envelope-wrapped, workflow-
guidance text embedded in the response) — they are not a projection of the RPC schema, they are
a hand-written, purpose-built "what should an agent in role X be allowed to do next" surface.

Confirmed by grep: `AttachSessionToItem` and `TriggerSync` exist **only** as RPC method names —
they appear in `server/features/backlog.go`, `server/services/backlog_service.go`, and the
generated `web-app/src/gen/session/v1/backlog_pb.ts`, but nowhere in `server/mcp/*.go` and nowhere
in a hand-written (non-generated) frontend file. There is no UI affordance and no MCP tool for
either. This is exactly the drift the audit flagged — three independent surfaces (RPC, MCP, UI),
grown by whoever needed which one, with `.claude/rules/feature-registry.md` policing backend/
frontend feature *tracking* but nothing in the repo enforcing RPC↔MCP↔UI surface parity itself.

## Does "MCP tool schema as single source of truth" fix this?

**No — and the research is unambiguous on this, not just for stapler-squad.** The only real-world
"single source of truth" pattern found is OpenAPI/RPC-spec → generate MCP tools, never the
reverse, and even that forward direction is discouraged as a 1:1 mechanical mapping. The reasons
generalize directly to this codebase:

- MCP tool descriptions in `tools_backlog.go` are written *for an LLM's cognition* — long prose
  descriptions, role-conditional response text (see `getBacklogItem`'s triage/work/review
  branches), deliberately restricted parameter shapes (e.g. `submit_review_verdict` takes a
  curated verdict array, not the full `UpdateBacklogItem` field set). A UI component needs
  different things: form validation, optimistic updates, pagination cursors shaped for a table,
  loading states. A REST/RPC caller needs yet a third shape (full CRUD, no role gating baked into
  the transport). Collapsing these into one generated artifact would either dumb down the MCP
  tool (bad for the LLM) or bloat the UI/RPC surface with agent-role-gating logic that doesn't
  belong there.
- The actual gap (`AttachSessionToItem`, `TriggerSync` missing from both MCP and UI) is not a
  schema-generation problem — it's a **registry/enforcement** problem: nobody keeps a checklist
  that says "every new RPC needs a decision: does this need a UI affordance, an MCP tool, both, or
  neither (internal-only)." That's solvable with process (extend `docs/registry/` /
  `.claude/rules/feature-registry.md` to explicitly cross-reference RPC method → MCP tool name →
  UI component, flagging RPCs with zero downstream consumers) — not with a codegen pipeline off
  MCP schemas.

## Concrete gap MCP *can* plausibly close here

The genuinely fitting move is narrower than "MCP as primary interface pattern": **audit which
existing backlog RPCs are agent-actionable but have no MCP tool, and add hand-written MCP tools
for the ones an autonomous session should reasonably call itself** — mirroring the role-gated
pattern already used for `report_progress`/`request_review`/`submit_review_verdict`. Candidates
worth a tool, based on what an agent session plausibly needs to self-serve:
- `AttachSessionToItem` — plausible: an agent spawned outside the normal `SpawnSessionFromItem`
  flow could self-attach. Needs the same role/permission gating already used elsewhere.
- `SuggestNextItem` — plausible: a MCP tool `suggest_next_backlog_item` fits the existing
  discovery-tool pattern (`tools_discovery.go`) directly.
- `TriggerSync`, `CreateItemSource`/`ListItemSources`/`UpdateItemSource`/`DeleteItemSource`,
  `GetSyncHistory` — these are operator/admin actions (configuring external sync sources), not
  something an autonomous coding session should trigger on itself. These belong in the UI only;
  exposing them over MCP would be scope creep, not gap-closing.

**First step if adopting further:** treat this as a per-RPC triage exercise (UI-only vs.
MCP-only vs. both vs. neither), not a mechanical "generate tool per RPC." Encode the outcome as
an explicit column in the feature registry (`docs/registry/backend-features.json`) —
`"mcpTool": "attach_session_to_item" | null` — so gaps show up the same way `coverage-gaps.json`
already surfaces missing tests.

## What MCP cannot help with at all

The audit's *other* named problem — the bespoke tmux-scraping `session/session_driver.go`
readiness detector, and the two parallel session-driving mechanisms (`AutonomousDriver` vs.
headless-pool/ADR-022) — is completely outside MCP's scope. MCP standardizes how a *model* calls
*tools*; it has nothing to say about how stapler-squad drives a *process* (tmux pane content,
keystroke injection, spinner-regex detection) or which of two internal execution engines runs a
session. Ironically, stapler-squad already *uses* MCP tools (`create_session`, `pause_session`,
etc. in `tools_lifecycle.go`) as a control-plane for that lifecycle — but the tool call itself is
just a thin wrapper around the same bespoke `Instance.Start()`/`Destroy()`/driver code; MCP is the
transport for *invoking* the lifecycle action, not a replacement for the lifecycle logic itself.
That gap needs ACP or a custom internal abstraction, not more MCP.

## Verdict

**Adopt-partially.** MCP is already correctly used in this repo as the agent-facing control/data
surface, and closing the specific `AttachSessionToItem`/`SuggestNextItem`-shaped gaps by adding a
few more hand-written, role-gated MCP tools (following the existing `tools_backlog.go` pattern) is
worth doing; but "MCP tool schemas as the single source of truth that generates RPC and UI" is not
a real pattern anywhere in the ecosystem and would actively fight MCP's design (tool descriptions
are written for LLM cognition, not for codegen) — the RPC/MCP/UI drift is a registry/process gap,
not a schema-unification opportunity, and MCP has zero jurisdiction over the session-lifecycle
driving problem that's the other half of the original audit.

Sources:
- [The 2026-07-28 MCP Specification Release Candidate](https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/)
- [The 2026 MCP Roadmap](https://blog.modelcontextprotocol.io/posts/2026-mcp-roadmap/)
- [Shortcomings of Model Context Protocol (MCP) Explained](https://www.cdata.com/blog/navigating-the-hurdles-mcp-limitations)
- [MCP Adoption Statistics 2026](https://www.digitalapplied.com/blog/mcp-adoption-statistics-2026-model-context-protocol)
- [Generating MCP tools from OpenAPI: benefits, limits and best practices (Speakeasy)](https://www.speakeasy.com/mcp/tool-design/generate-mcp-tools-from-openapi)
- [Auto-generating MCP Servers from OpenAPI Schemas: Yay or Nay? (Neon)](https://neon.com/blog/autogenerating-mcp-servers-openai-schemas)
- [Best Practices for Mapping REST APIs to MCP Tools (Zuplo)](https://zuplo.com/learning-center/mapping-rest-apis-to-mcp-tools)
- [A survey of agent interoperability protocols: MCP, ACP, A2A, ANP](https://arxiv.org/pdf/2505.02279)
