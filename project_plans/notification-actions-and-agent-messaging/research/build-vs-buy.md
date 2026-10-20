# Build vs. Buy and Competitive Survey

**Date**: 2026-10-17. **Scope**: agent-to-human questions, agent-to-agent help, self-scheduling and wake triggers, notification actions, background feeds.
**Labels**: VERIFIED = primary page opened this session (URL given). INFERRED = from search snippets or secondary sources, or my reasoning. GAP = could not find a source.

## 1. What others implement

### 1.1 Primitives we can reuse (VERIFIED unless noted)

**Claude Code AskUserQuestion has three programmatic answer channels.** This settles the "rabbit hole" in requirements.md.
- **`PreToolUse` hook, allow + `updatedInput`.** The hook matches `AskUserQuestion`, reads `questions` from stdin, and returns `permissionDecision: "allow"` with `updatedInput` that echoes `questions` plus `answers` (a map of question text to option label). "Returning allow alone is not sufficient." The question is 1-4 questions with 2-4 options each; multi-select answers are comma-joined. Source: https://code.claude.com/docs/en/hooks (sections "AskUserQuestion", "PreToolUse decision control").
- **`PreToolUse` `defer`.** It works only with `claude -p` (non-interactive). The process exits with `stop_reason: "tool_deferred"` and `deferred_tool_use`. The caller later runs `claude -p --resume <id>`, and the hook returns `allow` + answers. There is no timeout. The session file is kept 30 days by default (`cleanupPeriodDays`). It is ignored when Claude makes several tool calls in one turn, and in interactive sessions it logs a warning and is ignored. Same page, "Defer a tool call for later". This is the closest thing to a durable human-in-the-loop primitive, but it only fits headless sessions.
- **Agent SDK `canUseTool`.** The callback receives `toolName == "AskUserQuestion"` and returns `{behavior:"allow", updatedInput:{questions, answers}}`. `AskUserQuestion` is not available in subagents spawned via the Agent tool. Source: https://code.claude.com/docs/en/agent-sdk/user-input.
- **Consequence.** Our tmux-hosted interactive sessions can use channel 1 (a hook that blocks while it waits on our server). That avoids keystroke injection for Claude Code. Whether the hook may block for hours is not stated (GAP). Verify hook timeout limits before ADR.

**Claude Code `Notification` hook** (VERIFIED, https://code.claude.com/docs/en/hooks#notification). Matchers: `permission_prompt`, `idle_prompt`, `auth_success`, `elicitation_dialog`, `elicitation_complete`, `elicitation_response`, `agent_needs_input`, `agent_completed`. The last two fire only while agent view is open in a terminal (v2.1.198+). Notification hooks "can't block or modify". They are side-effect only. This is a detection signal, not an answer channel.

**Claude Code scheduling** (VERIFIED, https://code.claude.com/docs/en/scheduled-tasks).
- Three tiers: cloud Routines (1 hour minimum), Desktop tasks (1 minute), and `/loop` (1 minute, session must be open). `CronCreate`, `CronList` and related tools exist.
- Recurring tasks expire after 7 days. There is no catch-up for missed fires: a late task fires once when idle.
- Restore on `--resume` has exceptions. The docs point to "Channels" for event-driven wakes instead of polling (page not opened, so INFERRED what Channels are).
- Consequence: Claude Code's native scheduling is per-conversation and lossy across restarts. It cannot satisfy "wakes survive a service restart" or "wake on another session's state". We need our own durable layer that re-prompts via `send`/resume.

**MCP elicitation** (VERIFIED, https://modelcontextprotocol.io/specification/draft/client/elicitation).
- Servers send `elicitation/create` with `mode: form` (flat JSON-schema: string, number, boolean, enum or multi-enum) or `mode: url`. In the draft spec this is delivered inside an `InputRequiredResult` and answered via `inputResponses` on a retried request.
- It is server-to-client, scoped to an in-flight request. It has no durability, no queue, and no addressing of a human other than "the client's user". Claude Code surfaces it as the `elicitation_dialog` notification.
- It is useful as a schema vocabulary (flat form schema, enum options) for our structured question entity. It is not a transport for async or durable asks.

**A2A protocol** (VERIFIED, https://a2a-protocol.org/latest/topics/life-of-a-task/).
- Tasks have a lifecycle with `input-required` and `auth-required` interrupted states and terminal states `completed|canceled|rejected|failed`. `contextId` groups tasks. Terminal tasks are immutable, so follow-ups create a new task in the same context, with `referenceTaskIds`.
- It is v1.0 and is now in the Agentic AI Foundation (https://a2a-protocol.org/latest/blog/2026/08/27/a-new-chapter-for-a2a-joining-the-agentic-ai-foundation/). Go SDK: https://github.com/a2aproject/a2a-go (listed at https://a2a-protocol.org/latest/sdk/).
- It is designed for opaque cross-vendor agents over HTTP with agent cards. Our peers are co-located tmux sessions in one process. We should borrow the state vocabulary (`input-required`, context/task split, immutability) and not adopt the wire protocol.

**LangGraph interrupts** (VERIFIED, https://docs.langchain.com/oss/python/langgraph/interrupts). `interrupt(value)` checkpoints state and waits indefinitely. Resume is `Command(resume=...)` on the same `thread_id`. An optional `response_schema` lets clients render typed forms. The node re-executes from its start on resume. The pattern matches our durable-guidance entity: persisted question, a cursor id, a typed response schema, resume by id. Python/JS only, so it is not directly usable.

**Temporal** (VERIFIED, https://docs.temporal.io/encyclopedia/application-message-passing). Signals are async fire-and-forget writes. Updates are synchronous and tracked, with validation. Queries are reads. Timers are durable. This gives the right vocabulary: a wake is a signal, a help request that needs a reply is an update.

**Inngest `step.waitForEvent`** (VERIFIED, https://www.inngest.com/docs/reference/functions/step-wait-for-event). It pauses until a matching event arrives or a timeout fires, with a CEL `if` match expression. It is explicitly cited for human-in-the-loop and for coordinating functions. The "event match expression plus timeout" shape is the exact model for our wake triggers (session X reaches state Y, or PR check completes, or human answer arrives, or timeout T).

### 1.2 Competing agent managers

| Tool | What I could verify | Source |
|---|---|---|
| **Conductor** | Runs Claude Code, Codex, Cursor and OpenCode in parallel isolated workspaces. Workflow, checks, diff viewer and an API exist. Docs index shows no notification-action or scheduling pages (not exhaustively crawled). | https://docs.conductor.build/ (VERIFIED, intro only) |
| **Claude Squad** | Go TUI, tmux session plus worktree per agent. Nothing found on structured questions. | secondary directory pages only (INFERRED) |
| **Crystal** | Reported deprecated in Feb 2026, succeeded by closed-source Nimbalyst. Notification behavior: GAP. | https://crystl.dev/blog/best-ai-agent-orchestration-tools/ (INFERRED) |
| **Vibe Kanban** | Kanban for parallel agents. Local MCP server exposing project/task tools (`list_projects`, `list_tasks`, `create_task`). Maker announced shutdown April 2026, now community-maintained. No ask-human or wake primitives found. | https://vibekanban.com/docs/integrations/vibe-kanban-mcp-server (search snippet, INFERRED) |
| **Sculptor** | One Docker container per agent. Notification/question features: GAP. | https://agentsroom.dev/blog/best-multi-agent-coding-tools (INFERRED) |
| **Cursor background agents** | Slack integration: follow-up instructions in a thread (owner only), per-agent target via context menu, `@Cursor list my agents`, completion notification with PR link. No agent-initiated question channel found. | https://docs.cursor.com/en/slack (search snippet, INFERRED) |
| **Devin** | v3 session API exposes `status_detail`: `working`, `waiting_for_user`, `waiting_for_approval`, `finished`. Send-message endpoint auto-resumes a suspended session. | https://docs.devin.ai/api-reference/v3/sessions/post-enterprise-sessions-messages.md (search snippet, INFERRED) |
| **Codex cloud** | Slack `@Codex` creates cloud tasks. Results come back in thread, follow-up by continuing. Mid-run ask-human channel: GAP. | https://developers.openai.com/codex/integrations/slack (snippet, INFERRED) |
| **Copilot coding agent** | Mission control with session states including "awaiting feedback". Steering mid-session, applied at the next tool-call boundary, via chat or a comment in Files changed. Needs write access. | https://github.blog/changelog/2025-10-28-a-mission-control-to-assign-steer-and-track-copilot-coding-agent-tasks/ (snippet, INFERRED) |
| **Amp, Factory** | Not researched: GAP. Dropped for effort, no source found. | n/a |

**Pattern across competitors (INFERRED).** The strongest shared idea is an explicit "waiting for user / awaiting feedback" session status, surfaced in a list and in chat integrations, with a free-text follow-up reply. None of the sources I could verify shows structured option answers in a notification surface, peer-to-peer agent help, or self-scheduled event triggers. Those look like differentiators if the claim holds, but it rests on absence of evidence from shallow searches (low confidence).

## 2. Build vs. buy evaluation

### 2.1 Agent-to-human questions (structured, durable, answerable inline)

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| (1) OSS framework (LangGraph interrupts) | Mature durable-interrupt model; typed response schema | Python/JS runtime; our sessions are CLI harnesses, not graphs; would add a second store | Not recommended (borrow design only) |
| (2) SaaS (Devin/Copilot/Slack-style) | Zero build | Cannot host our tmux sessions; local-only data constraint | Not recommended |
| (3a) Bespoke on existing `GuidanceRequest` entity + Claude Code `PreToolUse` hook answer channel | No parallel store (constraint); hook returns `allow` + `updatedInput.answers`, so no keystroke injection for Claude Code; reuses delivery gate | Per-harness adapters; hook-blocking duration unverified | **Recommended** |
| (3b) MCP elicitation as the transport | Standard, schema-typed | In-flight and non-durable; client-scoped; not addressable to our UI | Not recommended as transport; Viable as schema vocabulary (flat form, enums) |
| (4) Fork/adapt a competitor UI | None found with structured answers | Vibe Kanban in maintenance mode; Crystal deprecated; Conductor closed | Not recommended |

Non-Claude harnesses: fall back to terminal-text detection plus keystroke reply (already in requirements). Not researched per harness here; the capability matrix is another agent's job.

### 2.2 Agent-to-agent help requests

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| (1) A2A protocol + `a2a-go` | Standard vocabulary, Go SDK exists | Built for opaque remote agents (HTTP, agent cards); heavy for in-process peers; harnesses cannot speak it natively | Not recommended as wire; **adopt the state model** (`input-required`, context/task split, immutable terminal) |
| (2) SaaS | None relevant | Locality | Not recommended |
| (3) Bespoke MCP tool (`request_help(peer, question)`) backed by the same guidance entity with asker and answerer fields | One store; server-side caller-identity gate (per prior instinct on MCP surfaces); works for every harness that speaks MCP | We own the semantics | **Recommended** |
| (4) Fork | n/a | n/a | Not recommended |

### 2.3 Self-scheduling and wake triggers

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| (1a) Claude Code native `CronCreate` / `/loop` / Routines | Zero build for Claude Code | Per-conversation; 7-day expiry; no catch-up; lost on restart without `--resume`; Claude-only; cannot express session-state or human-answer triggers | Not recommended as the substrate; Viable as an optional accelerator |
| (1b) Go cron lib (robfig/cron, go-co-op/gocron) | Cron parsing and next-time math solved | In-memory only; robfig reported unmaintained since 2020 (secondary sources, INFERRED); no event triggers, no persistence | Viable for cron-expression parsing only, with persistence in our table |
| (1c) Persistent job queue (River on Postgres, asynq on Redis) | Durable, retries | We use SQLite/ent, so River or asynq adds Postgres or Redis; overkill for tens of sessions | Not recommended |
| (1d) Workflow engine (Temporal, Inngest) | Exactly the right model (durable timers, signals, `waitForEvent` with match expression and timeout) | New server/infra to run; violates single-binary local deployment | Not recommended; **copy the model** |
| (2) SaaS (Inngest cloud, Trigger.dev) | Hosted durability | Local-only data; sessions are on the operator's machine | Not recommended |
| (3) Bespoke ent `wake_trigger` table + scheduler goroutine + evaluation on the existing event bus | Matches constraints: survives restart (load on boot, fire overdue once, no catch-up storm); `noarchivedrevival` check lives at the single fire site; supports time, event (backlog/PR/CI), session-state, and human-answer triggers via one match-expression-plus-timeout shape; live-settable flag | We own correctness (missed fires, double fires, clock jumps); needs idempotent fire with a CAS on state | **Recommended**, using a small cron-parse lib for the schedule math only |
| (4) Fork a scheduler | n/a | n/a | Not recommended |

Design cues to copy: Inngest's event-match plus timeout as one primitive; Claude Code's "no catch-up, fire once when idle" for missed fires (but surface a "missed" counter, since requirements want alerting on misses after restart); Temporal's Signal-vs-Update split (wake = signal, help-reply = tracked update).

### 2.4 Notification actions and background feed

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| (1) OSS notification center (Novu, Knock) | Inbox UI, action buttons | Cross-channel SaaS-ish framework; our inbox already exists, scope excludes channel redesign; constraint to reuse the delivery gate | Not recommended |
| (2) Slack/chat as the action surface | Competitors use it (Cursor, Codex) | Out of scope (Slack redesign excluded) | Not recommended here |
| (3) Bespoke: action descriptors on the existing notification record (type, payload schema, handler id), extended through the `OmnibarAction` registry conventions | Reuses gate, RPCs, registries; one interaction to resolve | Registry sync discipline | **Recommended** |
| (4) Fork a competitor UI | None viable | n/a | Not recommended |

Background activity feed: no competitor pattern found to copy. Bespoke only, driven by fixing the producer gap documented in requirements.md.

## 3. Competitor feature matrix

Legend: Y = verified in a primary source; y = reported in a secondary source or snippet; - = not found (GAP, not proof of absence); n/a.

| Feature | Claude Code | Agent SDK | Conductor | Claude Squad | Vibe Kanban | Cursor BG | Devin | Codex cloud | Copilot agent | LangGraph | Temporal / Inngest | MCP elicit. | A2A |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Structured agent-to-human question | Y (AskUserQuestion) | Y (canUseTool) | - | - | - | - | y (waiting_for_user) | - | y (awaiting feedback) | Y (interrupt) | Y (Inngest waitForEvent) | Y (form) | Y (input-required) |
| Programmatic answer of that question | Y (hook updatedInput) | Y | - | - | - | - | y (message API) | - | y (steer) | Y (Command resume) | Y (signal/event) | Y | Y |
| Durable across restart | Y (defer + resume, 30d) | partial | - | - | - | y | y | y | y | Y | Y | N (in-flight) | Y (task state) |
| Agent-to-agent help | partial (subagents; AskUserQuestion unavailable there) | partial | - | - | - | - | - | - | - | partial (multi-agent graphs) | Y (signals) | N | Y |
| Self-scheduled wake by time | Y (CronCreate; 7-day expiry, session-bound) | - | - | - | - | - | - | - | - | - | Y (timers) | N | N |
| Wake by event or state | partial (Channels, unverified) | - | - | - | - | - | - | - | - | Y (via resume) | Y | N | N |
| Wake survives restart | partial (`--resume`) | - | - | - | - | - | - | - | - | Y | Y | N | n/a |
| Notification hook / signal | Y (Notification matchers) | Y (hooks) | - | - | - | y (Slack done) | y | y (Slack) | y | n/a | n/a | y (via notification) | n/a |
| Inline action in notification | - | n/a | - | - | - | y (Slack follow-up) | - | y (Slack follow-up) | y (Files-changed comment) | n/a | n/a | n/a | n/a |
| Background/hidden activity feed | partial (agent view) | n/a | - | - | - | y (agent list) | y (session list) | y (task list) | y (mission control) | n/a | y (run list) | n/a | n/a |

## 4. Gap list vs stapler-squad

Baseline taken from requirements.md only; I did not re-audit the code (INFERRED).

1. **No structured AskUserQuestion answer path for live sessions.** Claude Code's hook `allow + updatedInput.answers` is the supported channel and replaces tmux keystroke injection for Claude Code. Highest-leverage finding. Open item: hook blocking limits and behavior inside tmux interactive mode.
2. **No `defer`-style durable resume for headless Claude sessions.** `PreToolUse defer` plus `--resume` is a built-in durable ask for `-p` sessions (triage/review/diagnose one-shots). Fits the hidden-session backlog automation; subject to the single-tool-call caveat.
3. **No typed response schema on guidance requests.** LangGraph (`response_schema`), MCP elicitation (flat form schema) and Claude's `questions/options` all carry options. Add schema and options to the entity.
4. **No explicit "waiting for user / awaiting approval" session status surfaced in lists.** Devin and Copilot both expose it. The Background tab and session list should key off it.
5. **Peer help has no entity or gate.** Reuse `GuidanceRequest` with asker and answerer; add a server-side caller-identity check. Borrow A2A states.
6. **No durable wake-trigger table.** `ScheduleWakeup` polling remains (ssq#428). Claude Code's native scheduling cannot cover restart, event or session-state triggers. Build the table; trigger = match expression plus timeout plus action; missed-fire counter.
7. **Notification actions are read-only.** Competitors offer follow-up from Slack/mission control. Ours needs action descriptors per notification type.
8. **Background feed has no producer for healthy hidden work.** No competitor pattern to copy; own design.
9. **Unresearched.** Amp, Factory, Sculptor and Crystal internals; per-harness (Aider, Codex CLI, OpenCode, Gemini, Antigravity, pi) answer channels; Claude Code "Channels"; hook timeout ceiling. Each needs a primary-source pass before the ADR.

## 5. Recommendation summary

Build on our own entities: extend `GuidanceRequest` (schema, options, asker/answerer) and add a durable `wake_trigger` table with a scheduler goroutine. Do not adopt a framework, queue, or SaaS. Borrow vocabulary from A2A (states), Inngest (event match plus timeout), LangGraph (response schema), and MCP elicitation (flat form schema). Use Claude Code's hook `updatedInput` as the answer channel. Use a small cron-parse library only for schedule math.
