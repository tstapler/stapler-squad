# Agent Protocol Architecture — Recommendation

Synthesized from `research/mcp.md`, `research/a2a.md`, `research/acp.md`. See
`../backlog-cross-platform-audit/` for the underlying pain points this research responds to.

## TL;DR

| Protocol | Verdict | Why |
|---|---|---|
| **MCP** | Adopt-partially | Already correctly used as the agent-facing control plane. Close two specific gaps (`AttachSessionToItem`, `SuggestNextItem`). Not a fix for the session-driver problem or for RPC/UI drift in general. |
| **A2A** | Not relevant now | Solves delegation between independently-owned agents across a trust boundary. stapler-squad has no second agent anywhere in its flow — everything is one process driving a local `claude` subprocess it fully owns. |
| **ACP** | Prototype behind a flag | Directly targets the real pain point (terminal-scraping session driver), but no first-party Claude Code support exists, and a structural limit means it can never fully replace the driver — only run alongside it for Claude sessions specifically. |

**None of the three is a drop-in replacement for the bespoke architecture.** The honest framing:
MCP is already right-sized for what it's used for, A2A doesn't apply to a problem stapler-squad
doesn't have, and ACP is the only one aimed at the actual fragility — but adopting it is a
multi-quarter bet on a third-party ecosystem, not a rewrite you can do this sprint.

## Why not A2A

A2A (Google, now Linux Foundation-governed, v1.0.1, TSC includes Microsoft/AWS/Cisco/
Salesforce/SAP/IBM) standardizes agent discovery (`AgentCard`) and task delegation over
JSON-RPC/REST/gRPC with SSE streaming and an 8-state task lifecycle. That's real infrastructure
for a real problem — but the problem is "how does Agent A hand work to Agent B, which it
doesn't control, across an organizational boundary." Every current stapler-squad flow is
`AutonomousDriver` or the headless pool calling a **local `claude -p` subprocess it started and
fully owns**. The GitHub plugins are polling data-fetchers, not agent delegation. There's no
second agent to delegate to today.

**Revisit if**: stapler-squad ever needs to (a) farm backlog work out to a fleet of remote agent
workers it doesn't directly manage, or (b) interoperate with another team's hosted agent
platform, or (c) expose "spawn/manage a session" as a capability other systems call via a
published AgentCard. None of these are on the table today.

## Why MCP is "keep doing what you're doing, plus two fixes"

MCP is already correctly scoped in this repo: `server/mcp/*.go` exposes session
lifecycle/discovery, VCS reads, and role-gated backlog workflow tools to the agent running
inside a session, with `STAPLER_SESSION_UUID` threading identity through. That's exactly what
MCP is for (model↔tool access), and it's done reasonably well.

The audit's "RPC/MCP/UI drift" finding is real but is **not** an MCP problem — no project
anywhere generates RPC/UI from an MCP tool schema (or the reverse); MCP tool descriptions are
written for LLM cognition (verbose, role-conditional, curated params), while RPC/UI need
form-validation/pagination/CRUD shapes. Trying to unify them through MCP would be forcing the
wrong abstraction to do a registry/process job.

**Concrete action**: add MCP tools for `AttachSessionToItem` and `SuggestNextItem` (both
currently RPC/UI-orphaned per the audit) following the existing hand-written, role-gated
pattern in `tools_backlog.go`. This is a half-day task, not a redesign.

## Why ACP is the right long-term direction but not a now-action

ACP (Zed-originated, now Zed+JetBrains co-governed, protocolVersion stable at 1, 20+ native
agent implementations including Gemini CLI, Codex, Copilot CLI) standardizes exactly the thing
`session/session_driver.go` currently fakes with regex: structured `session/update`,
`session/request_permission`, and `session/prompt` messages instead of scraping terminal output
for a readline prompt, answering dialogs by pattern-matching pane text, and injecting keystrokes
with read-back verification.

Two hard facts constrain how far this can go:

1. **No first-party Claude Code support.** The GitHub feature request
   (`anthropics/claude-code#6686`) was stale-bot-closed with no Anthropic response; Zed publicly
   invited Anthropic to build native support and got silence. The path today is a third-party
   bridge. The originally-linked one (`Xuanwo/acp-claude-code`) is dead/self-archived; its
   successor `agentclientprotocol/claude-agent-acp` is materially healthier (2,172 stars,
   actively maintained, wraps the official Claude Agent SDK rather than scraping a pty) — but
   it's still not Anthropic, and untested here for CLI/SDK feature parity (MCP passthrough,
   `--dangerously-skip-permissions`, session resume, etc.).
2. **Structural ceiling.** `session/instance_tmux.go`'s `classifyProgram` supports non-Claude
   programs (aider, plain shells) via a catch-all `plainProgram` path with no ACP equivalent.
   `session_driver.go` and `session/detection/` can **never be fully deleted** regardless of ACP
   adoption — they'll always be needed for non-Claude sessions. The only honest move is a
   Claude-only, feature-flagged driver running *alongside* the existing one, not a replacement
   for it.

Note also: ACP would replace the *input/detection plumbing* of `autonomous_driver.go`, not its
turn-continuation policy (max-turns, stuck detection, rate-limit backoff) — that orchestration
logic stays regardless of transport.

**Concrete first step, if pursued**: a throwaway spike — wire `claude-agent-acp` behind a new
`STAPLER_SESSION_USE_ACP` flag for Claude-only sessions, and see whether it actually eliminates
a category of the historical fragility bugs (indented spinners, CR-overwritten spinners,
asterism prefixes) in a side-by-side comparison, before committing to anything broader. This is
a multi-week spike, not a sprint task — don't schedule it opportunistically alongside the
backlog audit fixes.

## Suggested sequencing

1. Do the backlog audit fixes first (`../backlog-cross-platform-audit/gaps-and-risks.md`) — they're
   scoped, understood, and don't depend on any protocol decision.
2. Add the two missing MCP tools (`AttachSessionToItem`, `SuggestNextItem`) — cheap, no
   architectural risk, closes a real gap.
3. Treat ACP as a deliberate, separately-scoped spike (not a "while we're in there" add-on) —
   time-box it, and judge success by whether it actually reduces the historical fragility bug
   count for Claude sessions, not by protocol purity.
4. Leave A2A alone until there's an actual second agent in the picture.
