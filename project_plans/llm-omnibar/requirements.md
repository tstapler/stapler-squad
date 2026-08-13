# Requirements: LLM Omnibar

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-19

## Problem Statement

Creating a new stapler-squad session currently requires the user to know the right
repository path, branch name, and how to phrase the initial prompt. This friction
slows down the "I have a task in mind → agent is working on it" loop. The target
user is a developer who wants to describe work in plain English and have a session
wired up and running with minimal manual configuration.

## Success Criteria

- A developer can type `> fix the login bug in auth service` in the omnibar and
  have the New Session form pre-filled (title, path, branch, prompt) within 3 seconds
- The same intent can be triggered headlessly (hotkey / API / MCP tool) without
  opening the UI, returning a session ID
- All three LLM backends (Claude CLI, Claude API, AWS Bedrock) work end-to-end
- The feature is usable with zero API keys configured (Claude CLI subscription path)

## Scope

### Must Have (MoSCoW)
- `>` prefix in existing omnibar triggers LLM intent-parsing mode
- LLM returns: session title, path, branch, program, session_type, initial prompt,
  tags/category, and optionally a suggested existing session to reuse
- LLM output pre-fills the existing New Session form; user can edit before submitting
- Starter context sent to LLM: recent/known repo paths, existing session titles +
  statuses, known git branches — just enough to anchor suggestions
- LLM uses the MCP server (`list_sessions`, `search_sessions`, `get_session`) to
  fetch any additional detail it needs during intent resolution
- Three pluggable LLM backends behind a common interface:
  1. **Claude CLI** — invokes `claude -p <prompt>` as a subprocess using the user's
     subscription; default when no API key is configured
  2. **Claude API** — direct Anthropic SDK call; requires `ANTHROPIC_API_KEY`
  3. **AWS Bedrock** — Claude via Bedrock; requires AWS credentials + model ARN
- Active backend selectable in stapler-squad settings; Claude CLI is the default
- Headless path: dedicated API endpoint (`POST /api/sessions/intent`) accepts a
  plain-text description and returns structured session parameters + creates the
  session if `execute: true`
- MCP tool: `create_session_from_intent` wraps the same endpoint so external agents
  (including claude itself via the stapler-squad MCP server) can trigger session
  creation from a description

### Out of Scope
- Multi-turn conversation / clarification dialog (v1 is one-shot parse)
- Voice input
- Session templates or saved intents
- LLM-powered editing of existing sessions (only creation)
- Any LLM backends beyond the three listed above

## Constraints

- **Tech stack**: Go backend (existing stapler-squad server), React + TypeScript
  frontend, mark3labs/mcp-go for MCP tool registration
- **Claude CLI dependency**: subprocess invocation; must handle PATH, auth errors,
  and slow startup gracefully (3s timeout for intent parse)
- **No mandatory API key**: Claude CLI path must work out of the box
- **Existing form reuse**: LLM output pre-fills the existing `NewSessionModal`
  component; no separate session-creation UI is built
- **MCP server already running**: The HTTP MCP endpoint at `/mcp` is available
  during intent resolution — LLM backends can use it as a tool

## Context

### Existing Work
- stapler-squad MCP server is implemented (PR #24) with 15 tools including
  `list_sessions`, `search_sessions`, `create_session`, `get_session`
- HTTP MCP transport (`/mcp`) is mounted on the existing server; new sessions
  automatically receive `--mcp-server` CLI flag
- Existing `NewSessionModal` React component handles session creation form
- `POST /api/sessions` (via ConnectRPC `CreateSession`) is the session creation endpoint

### Stakeholders
- Tyler Stapler (sole user/developer)

## LLM Backend Interface Contract

All three backends must implement a single Go interface:

```go
type IntentParser interface {
    // ParseIntent takes a natural-language description and optional starter context,
    // returns structured session parameters. The LLM may use the MCP server URL
    // to fetch additional context during parsing.
    ParseIntent(ctx context.Context, description string, ctx StarterContext) (*SessionIntent, error)
}

type StarterContext struct {
    RecentPaths    []string          // known repo paths
    Sessions       []SessionSummary  // title + status of existing sessions
    MCPServerURL   string            // so the LLM can call back for more data
}

type SessionIntent struct {
    Title              string
    Path               string
    Branch             string
    Program            string  // "claude" | "aider"
    SessionType        string
    InitialPrompt      string
    Tags               []string
    SuggestedSessionID string  // non-empty if an existing session should be reused
    Confidence         float64 // 0–1; low confidence triggers form-only mode
}
```

## Research Dimensions Needed

- [ ] Stack — evaluate subprocess CLI invocation, Anthropic Go SDK, AWS Bedrock Go SDK
- [ ] Features — survey comparable "natural language → structured output" UX patterns
- [ ] Architecture — design patterns for pluggable LLM backends, prompt engineering for structured JSON output, MCP tool use during intent resolution
- [ ] Pitfalls — CLI subprocess latency, structured output reliability, Bedrock auth complexity, context window limits for starter data
