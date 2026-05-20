# Token & Spend Monitoring — Requirements

## Problem Statement

Stapler-squad manages Claude Code sessions but provides no visibility into how many tokens each session consumes, what it costs, or which skills/commands/plugins are driving spend. Users cannot tell if they are being efficient or identify where token budget is being wasted.

## Goals

1. Track API spend and token usage per session
2. Identify expensive skills, commands, and plugins
3. Compare efficiency across sessions and workflows
4. Provide budget alerts and spend limits
5. Surface aggregate reports similar to [claude-insights](https://github.com/yahav10/claude-insights)

## Data Sources

- **Claude Code CLI sessions** — main claude CLI sessions managed by stapler-squad
- **Subagent spawns** — token usage from Task tool / subagent calls within sessions
- **MCP server calls** — tokens used by MCP tool calls (Datadog, Slack, etc.)
- **Skills & commands loaded** — track which /skills and /commands were activated

## Data Collection

Parse Claude Code JSONL transcripts stored in `~/.claude/projects/<project-hash>/`. Each session produces a `.jsonl` file containing message-level token usage metadata. This is the same source used by claude-insights. Collection happens in a Go parser — no Node.js dependency.

## Cost Model

Hybrid approach:
1. **Hardcoded pricing table** — Anthropic published prices per model (input/output/cache tokens), updated manually
2. **Live pricing fallback** — optionally fetch from Anthropic usage API when API key is configured
3. **Raw token mode** — always show raw token counts regardless of pricing availability

## UI Surface

### Session List & Detail (lightweight)
- Token badge on session cards showing total session tokens
- Cost badge (e.g. "$0.03") when pricing is available
- Model indicator (Opus / Sonnet / Haiku)

### Dedicated Monitoring Dashboard (`/insights` route)
Full analytics page with:

**Time Range Controls**
- Per-session breakdown table
- Daily / weekly / monthly rollup charts
- Configurable date range picker

**Top-N Tables**
- Most expensive sessions (by cost and by tokens)
- Most-used skills and commands (by activation count and by token consumption)
- MCP server call costs

**Model Breakdown**
- Opus vs Sonnet vs Haiku usage split (tokens + cost)
- Cache hit rate (input_tokens_cache_read vs input_tokens)

**Trend Charts**
- Daily spend over time (line chart)
- Session efficiency score (output tokens / total tokens ratio)

## Functional Requirements

### FR-1: JSONL Parser
- Parse `~/.claude/projects/*/` session JSONL files
- Extract per-message: model, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens
- Extract tool use metadata: tool name, MCP server name if present
- Extract `/` command and skill activations from message content patterns
- Handle malformed or partial JSONL without crashing

### FR-2: Token Aggregation
- Aggregate tokens at session level: total, by turn, by tool
- Identify subagent sessions (spawned via Task tool) and link to parent
- Track message-level timeline for "token burn rate" calculations

### FR-3: Cost Calculation
- Apply pricing table: map model → (input_price, output_price, cache_write_price, cache_read_price) per 1M tokens
- Support manual price table override via config
- Show estimated cost with configurable currency (USD default)
- Flag when pricing data is stale (>30 days since last update)

### FR-4: Skills & Commands Detection
- Detect `/skill-name` and `/command-name` patterns in human turn messages
- Detect `SKILL.md` or skill file loads in tool results (Read tool on `~/.claude/skills/`)
- Build per-session skill activation list with token cost attribution

### FR-5: Session Association
- Match JSONL sessions to stapler-squad session records by directory path and timestamp
- Sessions without a matching stapler-squad record show in "orphan" section of dashboard
- Subagent sessions (nested `~/project/` hashes) linked to parent session

### FR-6: Dashboard UI
- `/insights` route in the React SPA
- Real-time data via existing ConnectRPC streaming pattern
- Charts using a lightweight library (recharts, already used in similar projects)
- Persistent filter state (time range, model, session tag) in URL params

### FR-7: Alerts & Limits
- Configurable per-session token budget (warn at X%, hard stop at Y%)
- Daily/monthly spend limit with alert notification in the session list
- Badge turns red when a session exceeds its budget

### FR-8: Export
- Export dashboard data as CSV (sessions, token counts, costs)
- Optional JSON export for programmatic consumption

## Non-Functional Requirements

- NFR-1: Parser must handle 10k+ line JSONL files without blocking the UI (stream or paginate)
- NFR-2: Dashboard must load in < 2s for 90 days of data
- NFR-3: Token data must not be sent to any external service without explicit user opt-in
- NFR-4: Pricing table updates must not require a binary rebuild (loaded from config or embedded JSON)

## Out of Scope

- Real-time token streaming during an active session (post-session analysis only)
- Multi-user / team spend aggregation
- Anthropic API billing reconciliation (JSONL counts may differ from billed amounts)

## Success Criteria

- Given a completed session, dashboard shows total tokens, estimated cost, model used, and top skills/commands within 5 seconds of session end
- Given 90 days of sessions, the insights dashboard loads under 2s
- Token counts match claude-insights output for the same session files (within 1% tolerance)
- Budget alerts fire before a session exceeds the configured limit

## Reference

- claude-insights: https://github.com/yahav10/claude-insights (TypeScript, same JSONL source)
- Anthropic token pricing: https://www.anthropic.com/pricing
- Stapler-squad JSONL location: `~/.claude/projects/<hash>/`
