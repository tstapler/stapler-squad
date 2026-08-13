# Findings: Features — MCP Tool Design for Stapler Squad

## Summary

Stapler Squad is a mission control dashboard for orchestrating multiple AI agent sessions (Claude, Aider, etc.) in isolated tmux sessions with git worktrees. The application exposes session management via ConnectRPC endpoints. The goal is to expose these capabilities as MCP tools so an LLM can autonomously create workspaces, delegate tasks, and manage agents.

This research surveyed three tool granularity options (coarse "do everything," fine-grained individual tools, and composite workflows), analyzed MCP design patterns from the Anthropic GitHub MCP server and Fuel Forge (a similar multi-agent orchestrator), and proposed a concrete tool list balancing LLM usability, composability, and schema clarity.

**Key Finding**: MCP tools should mirror user intent and natural task boundaries, not implementation details. Tools like `create_session` (coarse) and `update_session_status` (fine-grained) can coexist; the LLM chooses the right level of abstraction for each task.

---

## Options Surveyed

### Option 1: Coarse-Grained "Do Everything" Tools (5–8 tools)

**Example tools:**
- `create_session_and_start` — single tool handles all creation logic (branch/worktree, start, auto-approve)
- `manage_session_lifecycle` — start/pause/resume/stop with strategy parameter
- `query_sessions` — list + filter + search in one tool

**Pros:**
- Minimal schema complexity for LLM to learn
- Forces clear, simple semantics per tool
- Fewer context tokens spent on tool descriptions

**Cons:**
- Requires LLM to decompose multi-step tasks internally (e.g., "create a session AND check status" requires two tool calls)
- Harder to compose workflows (no granular checkpoint/state machine semantics)
- Tight coupling in protobuf schema (must bundle unrelated params)
- Less useful for scripting or programmatic access

**Applicability to Stapler Squad:**
- Works for initial MVP, but underutilizes the rich session model (tags, checkpoints, approvals)
- Example: An LLM trying to fork a session from a checkpoint cannot express "create new session from checkpoint" + "preserve scrollback" without a custom tool

---

### Option 2: Fine-Grained Individual Tools (20–30 tools)

**Example tools:**
- `start_session`
- `pause_session`
- `resume_session`
- `stop_session`
- `update_session_tags`
- `create_checkpoint`
- `fork_session_from_checkpoint`
- `get_session_vcs_status`
- `switch_session_workspace`
- `list_files_in_session`
- ... and many more

**Pros:**
- Directly maps to LLM intent (one tool per conceptual action)
- Excellent composability for workflows
- Simple, flat tool naming (no need for complex params)
- Easy to add new tools without schema churn

**Cons:**
- Token overhead: many tool descriptions
- LLM must chain calls (e.g., "create session" + "start session" separately)
- Higher latency for multi-step workflows
- Explosion of similar tools (pause/resume/stop all change status differently)

**Applicability to Stapler Squad:**
- Well-suited to Stapler Squad's rich feature set
- Natural mapping: CheckpointProto → `create_checkpoint`, `list_checkpoints`, `fork_session_from_checkpoint`
- Workspaces → `switch_session_workspace`, `list_workspace_targets`
- But risk of "tool salad" if not carefully scoped

---

### Option 3: Composite Workflow Tools (12–18 tools, strategic grouping)

**Example tools:**
- `create_session_with_defaults` — coarse, handles standard creation patterns (directory vs worktree, auto-apply profile defaults)
- `create_session_from_checkpoint` — composite, wraps fork logic
- `update_session_status` — fine, handles start/pause/resume/stop with enum parameter
- `get_session_details` — returns Session protobuf with all fields (status, tags, diff, VCS, etc.)
- `search_sessions` — coarse, handles list + filter + search
- `manage_tags` — coarse, add/remove/replace tags
- `checkpoint_workflow` — composite, create + list + fork in one tool with strategy
- ... approximately 14 more

**Pros:**
- Balanced API surface: complex operations get tools, simple operations grouped
- Better than coarse for composability; better than fine for token cost
- Mirrors Stapler Squad's internal service boundaries (SessionService, CheckpointService, etc.)
- Scales: can add new composite workflows without exploding tool count
- Natural for LLMs: matches task-oriented prompts ("fork a session," "create a checkpoint")

**Cons:**
- Requires careful judgment on where to draw grouping boundaries
- Some composite tools may feel "heavy" (multiple parameters, complex response)
- Tool descriptions must be more detailed to explain when to use composite vs separate tools

**Applicability to Stapler Squad:**
- **Recommended approach**. Stapler Squad already has 70+ RPC endpoints; exposing all as MCP tools would overwhelm. Grouping by user workflow (session lifecycle, checkpoint management, workspace switching) is more natural.
- Example grouping:
  - Session creation & lifecycle: `create_session`, `update_session_status`, `delete_session`
  - Checkpoints: `create_checkpoint`, `list_checkpoints`, `fork_session_from_checkpoint`
  - Workspaces: `switch_session_workspace`, `list_workspace_targets`
  - Tags & organization: `manage_session_tags`
  - Query: `get_session_details`, `search_sessions`, `list_sessions`
  - Files & VCS: `list_files`, `get_file_content`, `search_files`, `get_vcs_status`
  - Configuration: `resolve_session_defaults`

---

## Trade-off Matrix

| Approach | LLM Usability | Composability | Schema Complexity | Discoverability | Token Cost | Best For |
|----------|--------------|---------------|------------------|-----------------|------------|----------|
| **Coarse (5–8)** | High (simple patterns) | Low (monolithic) | Low | Excellent | ~500 tokens for all docs | Simple apps, RPCs only |
| **Fine (20–30)** | Medium (many choices) | Very High (full expressiveness) | High (many similar tools) | Medium (need docs) | ~2k tokens for docs | Complex SDKs, rich workflows |
| **Composite (12–18)** | **High (task-oriented)** | **High (hybrid)** | **Medium** | **Good (clear grouping)** | **~1.2k tokens** | **Rich applications, LLM-first** |

---

## Proposed Tool List

**Rationale**: 14 tools grouped by user workflow and semantic boundary. Maps to Stapler Squad's session model (Instance, Checkpoint, etc.) and user mental models ("create a session," "fork from checkpoint," "switch workspace").

### Session Lifecycle (5 tools)

1. **`create_session`**
   - **Purpose**: Create and optionally start a new AI agent session
   - **Input**: 
     - `title` (required, string): Session name
     - `path` (required, string): Repository root path
     - `branch` (optional, string): Git branch name (creates if missing)
     - `program` (optional, string): Program to run (default: "claude")
     - `working_dir` (optional, string): Directory within repo
     - `auto_yes` (optional, bool): Auto-approve prompts
     - `profile` (optional, string): Apply named profile defaults
     - `session_type` (optional, enum: "directory" | "new_worktree" | "existing_worktree")
     - `existing_worktree` (optional, string): Path to reuse worktree
     - `resume_id` (optional, string): Resume a Claude history entry
   - **Output**: Session object with id, status, created_at, diff_stats, vcs_status
   - **Errors**: DuplicateSessionTitle, InvalidPath, GitError, WorktreeError
   - **Use Case**: "Create a new workspace for feature X and start Claude"

2. **`update_session_status`**
   - **Purpose**: Transition session status (start, pause, resume, stop)
   - **Input**:
     - `session_id` (required, string)
     - `new_status` (required, enum: "RUNNING" | "PAUSED" | "STOPPED")
     - `preserve_terminal_output` (optional, bool): For restart operations
   - **Output**: Updated Session object
   - **Errors**: SessionNotFound, InvalidStatusTransition, TerminalError
   - **Use Case**: "Pause this session and run another"

3. **`delete_session`**
   - **Purpose**: Stop and clean up a session (remove tmux, worktree, branch)
   - **Input**:
     - `session_id` (required, string)
     - `force` (optional, bool): Delete even if running
   - **Output**: `{ success: bool, message: string }`
   - **Errors**: SessionNotFound, CleanupFailed
   - **Use Case**: "Clean up this finished session"

4. **`rename_session`**
   - **Purpose**: Change session title
   - **Input**:
     - `session_id` (required, string)
     - `new_title` (required, string): Must be unique
   - **Output**: Updated Session object
   - **Errors**: SessionNotFound, DuplicateTitle
   - **Use Case**: "Rename this session to reflect its new purpose"

5. **`restart_session`**
   - **Purpose**: Kill and recreate session (preserves branch, optionally preserves scrollback)
   - **Input**:
     - `session_id` (required, string)
     - `preserve_output` (optional, bool): Keep terminal history
   - **Output**: Updated Session object
   - **Errors**: SessionNotFound, TerminalError
   - **Use Case**: "Restart this stuck session"

### Session Query & Organization (4 tools)

6. **`get_session_details`**
   - **Purpose**: Fetch full session state (status, tags, diff, VCS, checkpoints)
   - **Input**:
     - `session_id` (required, string)
   - **Output**: Full Session protobuf with all fields
   - **Errors**: SessionNotFound
   - **Use Case**: "Show me the state of this session"

7. **`search_sessions`**
   - **Purpose**: List and filter sessions with optional full-text search
   - **Input**:
     - `search_query` (optional, string): Fuzzy match title/path/branch/tags
     - `status_filter` (optional, enum): RUNNING | PAUSED | READY | etc.
     - `tag_filter` (optional, string array): Exact tag matches
     - `hide_paused` (optional, bool): Exclude paused sessions
   - **Output**: `{ sessions: Session[], total_count: int, has_more: bool }`
   - **Errors**: InvalidQuery
   - **Use Case**: "Find all running sessions tagged 'urgent'"

8. **`manage_session_tags`**
   - **Purpose**: Add/remove/replace tags for organization
   - **Input**:
     - `session_id` (required, string)
     - `action` (required, enum: "add" | "remove" | "replace")
     - `tags` (required, string array): Tags to apply
   - **Output**: Updated Session object
   - **Errors**: SessionNotFound, InvalidTag
   - **Use Case**: "Tag this session with 'frontend' and 'urgent'"

9. **`list_sessions`**
   - **Purpose**: Get all sessions (lightweight, basic filtering)
   - **Input**:
     - `status_filter` (optional, enum)
     - `hide_paused` (optional, bool)
     - `limit` (optional, int): Default 100, max 1000
   - **Output**: `{ sessions: Session[], total_count: int }`
   - **Errors**: None (returns empty list if no matches)
   - **Use Case**: "Show me all running sessions"

### Checkpoints & Session Forking (3 tools)

10. **`create_checkpoint`**
    - **Purpose**: Capture session state (scrollback, git HEAD, conversation UUID) for later restoration
    - **Input**:
      - `session_id` (required, string)
      - `label` (required, string): Human-readable checkpoint name
    - **Output**: `{ checkpoint_id: string, session_id: string, label: string, timestamp: timestamp }`
    - **Errors**: SessionNotFound, StorageError
    - **Use Case**: "Save a checkpoint before trying a risky refactor"

11. **`list_checkpoints`**
    - **Purpose**: Get all checkpoints for a session
    - **Input**:
      - `session_id` (required, string)
    - **Output**: `{ checkpoints: CheckpointProto[], total_count: int }`
    - **Errors**: SessionNotFound
    - **Use Case**: "Show me all saved checkpoints for this session"

12. **`fork_session_from_checkpoint`**
    - **Purpose**: Create independent session branched from a checkpoint (copies scrollback, git state, conversation history)
    - **Input**:
      - `source_session_id` (required, string)
      - `checkpoint_id` (required, string)
      - `new_title` (required, string): Title for forked session
    - **Output**: New Session object
    - **Errors**: SessionNotFound, CheckpointNotFound, TitleConflict, GitError
    - **Use Case**: "Fork from this checkpoint to try an alternative approach"

### Workspace & VCS Management (2 tools)

13. **`switch_session_workspace`**
    - **Purpose**: Change session's active branch/worktree/revision (restart session with new context)
    - **Input**:
      - `session_id` (required, string)
      - `switch_type` (required, enum: "REVISION" | "WORKTREE" | "DIRECTORY")
      - `target` (required, string): Branch name, worktree path, or revision ID
      - `change_strategy` (optional, enum: "KEEP_AS_WIP" | "BRING_ALONG" | "ABANDON")
      - `create_if_missing` (optional, bool): Auto-create branch/worktree
      - `base_revision` (optional, string): For creating new branches
    - **Output**: `{ success: bool, previous_revision: string, current_revision: string, session: Session }`
    - **Errors**: SessionNotFound, InvalidTarget, ConflictingChanges, GitError
    - **Use Case**: "Switch this session to the 'main' branch"

14. **`get_vcs_status`**
    - **Purpose**: Query session's VCS state (branch, staged/unstaged changes, conflicts, upstream sync)
    - **Input**:
      - `session_id` (required, string)
    - **Output**: VCSStatus protobuf (type, branch, head_commit, ahead_by, behind_by, has_staged, has_unstaged, has_conflicts, files_list)
    - **Errors**: SessionNotFound, NotAGitRepo
    - **Use Case**: "Check if this session has uncommitted changes"

---

## Alternative Tools (Lower Priority)

These tools address secondary use cases and can be added in a second phase:

- **`list_workspace_targets`** — Get available branches/revisions/worktrees for switching (pre-populates UI dropdown)
- **`get_session_diff`** — Fetch unified git diff for a session (for code review)
- **`list_files`** — Browse session's file tree (for IDE integration)
- **`get_file_content`** — Read file from session's worktree
- **`resolve_session_defaults`** — Pre-compute session creation defaults for a given working directory + profile
- **`get_review_queue`** — Get sessions needing user attention (prioritized by urgency)
- **`acknowledge_session`** — Mark a review queue item as addressed
- **`get_notification_history`** — Retrieve recent notifications sent by sessions
- **`list_databases`** — List available workspace databases (for workspace switcher)
- **`switch_database`** — Change active workspace context

---

## Risk and Failure Modes

### 1. Tool Proliferation Fatigue
**Risk**: Adding too many tools (30+) causes LLM decision paralysis and token bloat.

**Mitigation**: Keep primary tool list ≤18 tools. Implement secondary tools only if user demand justifies the complexity. Use clear naming conventions to group related tools (e.g., all "checkpoint_*" tools in one semantic family).

### 2. Ambiguous Tool Boundaries
**Risk**: Overlap between tools (e.g., `update_session_status` vs `restart_session` for "recover from stuck state").

**Mitigation**: Enforce clear ownership semantics:
- `update_session_status` = explicit state transition (Running → Paused → Stopped)
- `restart_session` = kill + recreate, preserving branch/worktree
- Document the distinction in tool descriptions with examples

### 3. Synchronous vs Asynchronous Operations
**Risk**: Session creation can take 10+ seconds (git clone, tmux setup); LLM may time out or lose context.

**Mitigation**: 
- All tools return immediately (fire-and-forget style for long operations)
- Provide polling mechanism: `get_session_details` returns status (LOADING, READY, RUNNING)
- Document expected latency for each tool in the schema
- Consider adding a `wait_for_status` tool for LLM workflows that need blocking

### 4. Orphaned Sessions on Crash
**Risk**: LLM creates a session but crashes before confirming creation; session lingers in tmux.

**Mitigation**: 
- Sessions timeout automatically if no activity for configurable period
- `list_sessions` includes `created_at` and `last_activity` timestamps for cleanup
- Consider adding a `cleanup_orphaned_sessions` admin tool

### 5. Conflicting Tag Semantics
**Risk**: "Tag" overloaded: Stapler Squad uses tags for organization, but MCP tools often use "tags" for resource metadata.

**Mitigation**: 
- Rename in MCP schema to `organization_tags` or `labels` to disambiguate
- Or keep `tags` but document clearly in tool descriptions
- Use `tag_filter` parameter name instead of generic `tags` for clarity

### 6. Workspace Switching Complexity
**Risk**: `switch_session_workspace` is complex (3 switch types, multiple strategies for handling changes). LLM may make invalid transitions.

**Mitigation**:
- Provide `list_workspace_targets` as a discovery tool (LLM calls first to validate choices)
- Use enum parameters (strongly typed) instead of free-text
- Return detailed error messages with valid alternatives
- Document examples: "Switch to branch 'main'," "Switch to worktree '/path/to/worktree'," etc.

---

## Migration and Adoption Cost

### Phase 1: MVP (1–2 weeks)
**Tools**: Tools 1–9 (session lifecycle + query + tags)
- Implement ConnectRPC → MCP bridge in Go
- Export tool schema (JSON Schema spec)
- Test with manual MCP client (test-mcp CLI or Anthropic SDK)

**Effort**: 
- MCP server skeleton: 2–3 days
- Tool wrappers: 3–5 days
- Schema validation: 2 days
- Documentation: 1 day

### Phase 2: Checkpoints & Workflows (1–2 weeks)
**Tools**: Tools 10–12 (checkpoints, fork)
- Wire checkpoint RPC endpoints to MCP tools
- Add scrollback/history fork logic
- Test complex checkpoint workflows

**Effort**: 
- Implementation: 3–5 days
- Testing (edge cases): 2–3 days
- Docs: 1 day

### Phase 3: Advanced (2–3 weeks)
**Tools**: Tools 13–14 + alternatives
- Workspace switching (complex, needs thorough testing)
- VCS status queries
- Optional tools (file browser, defaults resolver, etc.)

**Effort**: 
- Workspace switching: 5–7 days (tricky state machine)
- VCS tools: 2–3 days
- Polish: 2 days

**Total estimated effort**: 4–6 weeks for all 14 tools + alternatives.

### Backward Compatibility
- No breaking changes to ConnectRPC API (existing web UI, TUI continue working)
- MCP tools are additive; can deprecate slowly
- Consider version negotiation in MCP handshake

---

## Operational Concerns

### 1. Authentication & Isolation
**Issue**: Stapler Squad currently has no multi-user access control. MCP tools expose all sessions.

**Recommendation**: 
- For now, assume single-user (localhost-only, no auth)
- If multi-user required later, add RBAC layer: (user_id, resource_id) → allowed_actions
- Document that MCP tools should only be exposed over localhost or TLS + API key

### 2. Error Handling & User Feedback
**Issue**: MCP errors are opaque; LLM doesn't get diagnostic info.

**Recommendation**:
- Include detailed error metadata in every response (error_code, message, suggestion)
- Example: `{ success: false, error_code: "SESSION_NOT_FOUND", message: "Session 'my-session' not found", suggestion: "Run list_sessions to see available sessions" }`
- Log all tool invocations for audit trail

### 3. Rate Limiting
**Issue**: LLM could spam tools, overloading tmux/git.

**Recommendation**:
- Implement per-session rate limits (e.g., max 10 status updates/sec)
- Queue long-running ops (git clone, checkout)
- Return `{ queued_at: timestamp, estimated_delay_ms: 5000 }` for batched operations

### 4. Monitoring & Observability
**Issue**: Need visibility into MCP tool usage and failures.

**Recommendation**:
- Log all tool calls with duration, result, parameters (redacted)
- Expose Prometheus metrics: tool_calls_total, tool_errors_total, tool_duration_seconds
- Integrate with existing Stapler Squad logging (OpenTelemetry)

### 5. Graceful Degradation
**Issue**: If git/tmux fail, LLM needs clear feedback.

**Recommendation**:
- Categorize errors: USER_ERROR (bad input), SYSTEM_ERROR (git failed), TRANSIENT_ERROR (timeout)
- Retry transient errors automatically (up to 3x with exponential backoff)
- For system errors, return actionable message: "Git failed: check repo permissions, run 'git fsck' to diagnose"

---

## Prior Art and Lessons Learned

### 1. Anthropic GitHub MCP Server
**Patterns used**:
- **Tool granularity**: ~15 tools (create/update/list for issues, PRs, commits; search)
  - `create_issue`, `update_issue`, `list_issues`, `search_issues`
  - `create_pull_request`, `update_pull_request`, `list_pull_requests`
  - Balanced: fine-grained enough for compose, coarse enough to avoid explosion
- **Naming convention**: `{verb}_{noun}` (e.g., `create_pull_request`, not `create_pr` or `github_create_pr`)
- **Error handling**: Detailed errors with suggestions
- **Streaming**: Not used (GitHub API is mostly request/response)

**Lesson**: Composite tools work well for API adapters. Group by noun (resource type), not by user intent.

### 2. Fuel Forge MCP Server (Kotlin)
**Context**: Multi-agent orchestration (similar to Stapler Squad).
**Patterns used**:
- ~20 MCP tools exposing task queue, agent status, CI pipeline, merge queue
  - `claim_task`, `update_task_status`, `list_tasks`
  - `trigger_pipeline`, `query_pipeline_status`
  - `enqueue_merge`, `dequeue_merge`
- **Tool design philosophy**: "MCP as the universal bus" — every capability exposed as a tool for both LLMs and humans
- **Composite workflows**: Higher-level tools like `claim_and_execute_task` bundle common patterns
- **State machine clarity**: Tools enforce valid transitions; invalid operations return clear errors
- **Sampling support**: Signal queue for waking agents from idle state (MCP sampling not always available)

**Lesson**: For orchestration systems, composite workflows (claim + execute) are more useful than atomic fine-grained tools. Humans and LLMs should use the same interface.

### 3. Common MCP Server Patterns
- **Pagination**: Tools return `{ results: T[], total_count: int, next_page_token?: string }`
- **Filtering**: Optional parameters for filtering (status, type, labels)
- **Bulk operations**: Some tools support batch input (e.g., `close_issues([issue_ids])`)
- **Streaming**: Rarely used in practice (most MCP implementations use request/response)
- **Sampling**: Some servers use MCP sampling for notifications/polling (check if client supports it first)

---

## Open Questions

1. **Should we support streaming for large file reads or logs?**
   - Current proposal: No (use request/response with pagination)
   - Alt: Add `stream_file_content` tool for large worktrees
   - Decision needed: Verify client support, measure use case frequency

2. **How aggressive should LLM-driven session cleanup be?**
   - Proposal: Sessions persist until explicitly deleted (safe, but accumulates clutter)
   - Alt: Auto-delete stopped sessions after 24h
   - Decision needed: Balance safety vs UX

3. **Should `create_session` accept a template/profile as a shorthand?**
   - Example: `profile: "react-frontend"` auto-applies defaults (auto_yes=true, tags=["frontend"], cli_flags="--verbose")
   - Proposal: Yes, add `profile` parameter
   - Decision needed: Define 3–5 common profiles (or let user define them)

4. **How to handle concurrent terminal input?**
   - If LLM and human both sending input to same session, conflict?
   - Proposal: Queue inputs in order; document that MCP is "master" (human can pause session if needed)
   - Decision needed: Implement input queueing or explicit "acquire lock" tool?

5. **Should we expose internal approval/hook mechanics?**
   - Current: `update_session_status` does not touch pending approvals
   - Alt: Add `resolve_pending_approval` tool so LLM can auto-approve (risky!)
   - Recommendation: Do NOT expose approval mechanics to LLM (security risk, keep human-in-loop)

6. **Multi-workspace support in MCP tools?**
   - Stapler Squad now supports workspace switching (multiple project databases)
   - Should MCP tools support multi-workspace? (e.g., `workspace_id` parameter)
   - Proposal: Keep Phase 1 single-workspace; add in Phase 2 if needed
   - Decision needed: Validate use case

---

## Recommendation

**Adopt Option 3 (Composite Workflow Tools) with the 14-tool proposal.**

**Rationale**:
1. **Balanced for LLMs**: 14 tools is a manageable surface area (~1.2k tokens for documentation), but rich enough to express complex workflows
2. **Mirrors user intent**: Tools align with how humans think about the system ("create a session," "fork from checkpoint," "switch workspace")
3. **Composable**: LLM can chain tools naturally without artificial decomposition
4. **Extensible**: Secondary tools can be added incrementally without major refactoring
5. **Prior art**: Mirrors GitHub MCP (15 tools) and Fuel Forge (20 tools)
6. **Low adoption risk**: ConnectRPC API is untouched; MCP server is purely additive

**Implementation Plan**:
- Week 1–2: Phase 1 (tools 1–9)
- Week 3–4: Phase 2 (tools 10–12)
- Week 5–6: Phase 3 (tools 13–14 + optional tools)

**Next Steps**:
1. Implement MCP server skeleton (Go stdlib + ConnectRPC bridge)
2. Define JSON Schema for all 14 tools (validation, OpenAPI spec generation)
3. Wire first 5 tools (create/update/list/search/tags) end-to-end
4. Test with manual MCP client and Anthropic Claude API
5. Iterate on tool descriptions and error handling based on real usage

---

## Pending Web Searches

Given training cutoff (February 2025), the following web searches would verify/refine recommendations:

1. **"MCP server design patterns 2025"** — Verify if new MCP best practices emerged post-Feb 2025
2. **"Anthropic MCP GitHub server source code"** — Examine latest GitHub MCP implementation for naming/error patterns
3. **"Go MCP SDK examples 2025"** — Confirm available Go libraries for MCP server implementation (vs Python-only)
4. **"ConnectRPC + MCP bridge implementation"** — Check if others have bridged gRPC/ConnectRPC to MCP
5. **"LLM tool design survey 2025"** — Verify whether composite tools (14 tools) or fine-grained (25+ tools) is the current consensus
6. **"MCP streaming semantics"** — Check if streaming support for file I/O is common or niche
7. **"Fuel Forge GitHub repository"** — Examine if the Kotlin MCP server mentioned in docs is publicly available and still active

---

## Appendix: Example Tool Schemas (JSON Schema)

### `create_session`
```json
{
  "name": "create_session",
  "description": "Create and optionally start a new AI agent session with git worktree support",
  "inputSchema": {
    "type": "object",
    "properties": {
      "title": {
        "type": "string",
        "description": "Session name (must be unique)"
      },
      "path": {
        "type": "string",
        "description": "Repository root path (supports ~ expansion)"
      },
      "branch": {
        "type": "string",
        "description": "Git branch name; creates new branch if missing"
      },
      "program": {
        "type": "string",
        "enum": ["claude", "aider", "cline"],
        "description": "Program to run (default: claude)"
      },
      "working_dir": {
        "type": "string",
        "description": "Directory within repo to start in"
      },
      "auto_yes": {
        "type": "boolean",
        "description": "Auto-approve prompts without user interaction"
      },
      "profile": {
        "type": "string",
        "description": "Apply named profile defaults (react-frontend, python-backend, etc.)"
      },
      "session_type": {
        "type": "string",
        "enum": ["directory", "new_worktree", "existing_worktree"],
        "description": "Session workflow type"
      },
      "existing_worktree": {
        "type": "string",
        "description": "Path to reuse existing worktree (if session_type=existing_worktree)"
      },
      "resume_id": {
        "type": "string",
        "description": "Resume a Claude history entry (pass --resume to claude)"
      }
    },
    "required": ["title", "path"]
  }
}
```

### `search_sessions`
```json
{
  "name": "search_sessions",
  "description": "Search and filter sessions by status, tags, and full-text query",
  "inputSchema": {
    "type": "object",
    "properties": {
      "search_query": {
        "type": "string",
        "description": "Fuzzy match against title, path, branch, tags (e.g., 'frontend urgent')"
      },
      "status_filter": {
        "type": "string",
        "enum": ["RUNNING", "PAUSED", "READY", "LOADING", "NEEDS_APPROVAL"],
        "description": "Filter by session status"
      },
      "tag_filter": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Filter by exact tags (AND logic: session must have all)"
      },
      "hide_paused": {
        "type": "boolean",
        "description": "Exclude paused sessions from results"
      }
    }
  }
}
```

---

**Document prepared**: 2026-04-18  
**Research scope**: MCP tool design for Stapler Squad v1 (14-tool proposal)  
**Reviewer**: [Pending feedback from team]


## Web Search Results

### Query: "MCP server tool design best practices 2025 tool naming schema LLM usability"
**Key findings**:
- LLMs become unreliable with >30–40 tools (hallucination, wrong selections) — keep total tool count low
- Tool names, descriptions, and parameter names are treated as prompts — invest in clear language
- Best practice: simple (one action per tool), composable (tools work together), predictable (consistent behavior and errors)
- Use kebab-case or snake_case for tool names; include clear descriptions addressing operational details (pagination, auth)
- Design top-down from workflows, not bottom-up from API endpoints
- Sources: https://modelcontextprotocol.io/specification/2025-06-18/server/tools, https://www.speakeasy.com/mcp/tool-design, https://engineering.block.xyz/blog/blocks-playbook-for-designing-mcp-servers

### Query: "MCP tool result size limits streaming output truncation"
**Key findings**:
- Tool responses silently truncated to ~10KB in some clients (GitHub Copilot CLI at 10KB, Claude Code at 256 lines/10KB)
- Token-based limits (not line limits) are emerging best practice
- Pagination with cursors is the recommended pattern for large result sets
- Terminal output (scrollback) MUST be truncated/paginated before returning as MCP tool result
- Sources: https://github.com/anthropics/claude-code/issues/2638, https://axiom.co/blog/designing-mcp-servers-for-wide-events

**Updated recommendations based on search:**
- [TRAINING_ONLY] on tool count confirmed: keep ≤15–18 tools for Stapler Squad MCP server
- Terminal output tool must implement truncation + line_limit parameter
- Tool descriptions are critical prompt engineering — invest time in them
