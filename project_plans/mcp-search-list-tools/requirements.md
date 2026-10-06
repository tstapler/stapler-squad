# Requirements: Expose existing search/list/filter RPCs as MCP tools

Source: backlog item `66cdf9f5-8011-48bf-8dc8-8019ea207e8c` (non-interactive triage, requirements
transcribed from item description — no ideation interview run).

## Problem

`server/mcp/*.go` registers 55 `mcpgo.NewTool(...)` calls (39 at item authoring time, since grown),
but LLM clients driving stapler-squad via MCP can only search **session metadata**
(`search_sessions`, `server/mcp/server.go:151`) — a plain field match against title/path/branch/tags,
no full-text, no filters, no date range.

Backend RPCs with richer search/filter semantics already exist and are used by the web UI, but have
no MCP exposure:

- `GetNotificationHistory` (`proto/session/v1/session.proto:135`, request at line 1367) — `limit`,
  `offset`, `type_filter`, `session_id`, `unread_only`.
- `ListBacklogItems` (`proto/session/v1/backlog.proto:780`, request at line 336) — `status`
  (repeated), `priority` (repeated), `sort_by`, `include_terminal`, `include_archived`. Only
  single-item backlog MCP tools exist today (`get_backlog_item`, `create_backlog_item`, etc.) — no
  list/filter tool. `server/mcp/tools_backlog.go:204`'s error hint already references a
  `list_backlog_items` tool that does not exist.
- `SearchClaudeHistory` (`proto/session/v1/session.proto:80`, request at line 972) — full-text
  search across Claude conversation history backed by `session/search/` (inverted index), with
  `project`, `model`, `start_time`/`end_time` filters and pagination.
- `SearchFiles` (`proto/session/v1/session.proto:237`) and `SearchGitHubRepos`
  (`proto/session/v1/backlog.proto:868`) — also unexposed, explicitly lower priority / out of
  scope for a first pass per the item.

This is a wiring gap: the backend logic exists and is exercised by the web UI. The LLM operator
can't reach it without a human doing the lookup first or falling back to raw RPC/curl calls.

## Desired outcome

MCP tool coverage extends to the search/list/filter RPCs that already exist server-side, so an LLM
client can discover and query notifications, backlog items, and Claude/session history the same way
a human can in the web UI — without new backend search capability being built.

## Priority signal

*(Carried forward from the source backlog item — added post-triad-review after a Product-lens gap
check found this framing existed in the original item but hadn't survived into this document.)*

- **Kano classification**: Basic expectation (must-be), not a delighter. An LLM operating this app
  is expected to be able to look things up the way a human user already can in the web UI — the
  absence isn't "nice to have more search," it's a capability-parity gap for a tool whose whole
  premise is LLM operation. Reinforced by the fact that `tools_backlog.go:204` already references
  `list_backlog_items` by name in production error text — the codebase itself already assumes this
  gap is closed.
- **RICE signal (qualitative)**: Reach — High (every MCP-driven session/workflow that checks
  notifications, backlog state, or history hits this gap). Impact — Medium-High (removes a
  recurring manual-lookup bottleneck, doesn't add new capability). Confidence — High (RPCs and
  filter semantics already exist and are proven in the web UI; mechanical wrapping, not new
  design). Effort — Low-Medium (plan.md's own 31-task breakdown, ~2 hours estimated task time,
  additive-only, no migration).
- **Risky assumption**: the default result limit (10, matching the `list_sessions` convention) may
  hide the specific item an LLM agent needs often enough to matter in practice — this is a design
  bet, not a measured value, and the mitigation is `offset`-based pagination on all three tools
  (see plan.md), not a higher default. If agents frequently need more than one page, that's a
  signal to revisit the default, not evidence the pagination mechanism is broken.

## In scope (per item's "Out of scope" section — prioritization is a planning-phase call)

- `list_backlog_items` MCP tool wrapping `ListBacklogItems` (resolves the dangling reference at
  `server/mcp/tools_backlog.go:204`).
- `get_notification_history` (or similarly named) MCP tool wrapping `GetNotificationHistory`.
- `search_claude_history` MCP tool wrapping `SearchClaudeHistory`, with an LLM-context-safe default
  limit, following the `list_sessions` convention ("Default limit is 10 to avoid filling LLM
  context", `server/mcp/server.go:121`).
- Follow the established resource-scoped-tool pattern (`search_sessions` precedent) rather than a
  unified cross-resource `search` tool, unless research surfaces a concrete reason to prefer the
  unified design — this is a lean in the item, not a decision, and research phase should settle it.

## Out of scope

- New backend search capability — FTS engine, notification filters, backlog filters all already
  exist server-side.
- `SearchFiles` / `SearchGitHubRepos` MCP exposure in this pass (lower priority per item; may be
  addressed in a follow-up).
- Web UI search UX changes.

## Acceptance criteria

1. An LLM client can list/filter backlog items via MCP (by `status`, `priority`, etc.) without a
   raw RPC/curl call.
2. An LLM client can query notification history via MCP with the same filters
   `GetNotificationHistory` already supports (`type_filter`, `session_id`, `unread_only`,
   pagination).
3. An LLM client can run a full-text Claude/session-history search via MCP (wrapping
   `SearchClaudeHistory`), with results respecting an LLM-context-safe default limit.
4. `server/mcp/tools_backlog.go:204`'s dangling reference to `list_backlog_items` is resolved (the
   tool now exists).
5. Each new tool follows the existing MCP tool conventions in `server/mcp/*.go`: argument
   validation, error results via `errResult`, and a matching `*_test.go` file. The
   `featureDisabledResult` feature-flag gate applies only where an existing flag already governs
   that domain — today that's `backlogEnabled` for `list_backlog_items` only. Notifications and
   Claude-history search have no corresponding flag anywhere in the codebase (confirmed by
   research), and no equivalent flag is invented for them in this pass — `get_notification_history`/
   `search_claude_history` register unconditionally, matching the existing precedent set by
   `create_workflow`/`list_approval_rules`. *(Amended post-adversarial-review: the original
   unscoped wording of this criterion was unsatisfiable, since 2 of 3 target domains have no
   flag to gate against.)*
6. New tools are documented/registered the same way existing tools are (registration in the
   relevant `server/mcp/tools_*.go` + `server.go` wiring, per existing pattern).

## Open questions (carried from item, not resolved here — for research/plan phases)

- Priority order among `SearchClaudeHistory` / `GetNotificationHistory` / `ListBacklogItems` (item
  scopes in all three; if effort forces a cut, which ships first). *(unresolved after Phase 2
  research — no user present to adjudicate; research found no cost/complexity asymmetry among the
  three that would force a natural ordering, so plan.md should either sequence all three as one
  task set or default to `list_backlog_items` first since it resolves acceptance criterion 4's
  dangling reference.)*
- ~~Resource-scoped tools (lean, precedent-backed) vs. one unified `search` tool spanning resource
  types~~ — **Resolved by research**: `research/architecture.md` §5 recommends resource-scoped
  tools (`list_backlog_items`, `get_notification_history`, `search_claude_history`), matching the
  `search_sessions`/`list_sessions` precedent and the non-overlapping filter vocabularies across
  the three RPCs.
- ~~Context-size / truncation strategy for `search_claude_history` results~~ — **Resolved by
  research**: `research/features.md` §1 recommends `SanitizeForAgentContext`-style per-snippet
  truncation inside the structured JSON envelope, plus the repo's existing "default limit 10"
  convention overriding the RPC's native default of 20.

## Suggested entry point

`/sdd:quick` per the item — each tool is a mechanical RPC→MCP-tool wrap following the
`search_sessions` template. A quick planning pass is warranted only if research finds a real case
for the unified-search design.
