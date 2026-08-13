# ADR-012: Context Injection Mechanism

**Status**: Superseded — see `implementation/plan.md` Architectural Decision: "Context delivered via MCP + session initial prompt + slash commands + DB-synced context file"
**Date**: 2026-05-10
**Superseded**: 2026-05-10

> **Why superseded**: The original decision chose `.backlog-context.md` as the *sole* injection mechanism. The final plan promotes four complementary mechanisms: (1) `--append-system-prompt` at session launch carries the full item context into the system prompt without any file modification; (2) pre-filled slash commands in `.claude/commands/backlog/` give the agent ergonomic per-criterion commands with the item ID baked in; (3) `get_backlog_item` MCP tool is the authoritative live re-orientation call; (4) `.backlog-context.md` is retained as a **DB-synced convenience file** the agent can `Read` directly — it is rewritten from the DB whenever AC changes while the session is active, not a one-time snapshot. CLAUDE.md is never modified. The concerns about `context_snapshot_at` staleness (see Consequences below) are resolved by rewriting the file on AC change and recording `ac_snapshot` on `ItemSession` for review gate comparison.

## Context

When a session is spawned from a backlog item, the agent running inside that session needs access to the item's title, description, acceptance criteria, priority, and notes so it can orient itself and execute against the correct criteria. Without this context, the agent starts cold and must either ask the user for context (friction) or proceed without it (drift, exactly the Beads failure mode).

The context must be available:
- At session start, before the agent issues any tool calls
- After server restarts (sessions may resume without a live server connection)
- Across all session types (directory, new worktree, existing worktree, one-off)
- Without requiring the agent to know to ask for it

Three candidate mechanisms were evaluated. The choice must not mutate project-owned files or require a live MCP connection to be useful.

## Decision

Write a `.backlog-context.md` file to the session's worktree root at spawn time.

The file is written by the `BacklogService.SpawnSessionFromItem` handler immediately after the worktree is ready and before the session process starts. Its path is `<worktree_root>/.backlog-context.md`. The file is scoped to the item ID to avoid stale-context bugs when a worktree is reused.

File format:

```markdown
# Backlog Item Context
<!-- managed by Stapler Squad — do not edit manually -->

**Item ID**: <uuid>
**Title**: <title>
**Priority**: <1–5>
**Status**: in_progress

## Description
<description>

## Acceptance Criteria
- [ ] <criterion 0>
- [ ] <criterion 1>

## Notes
<notes>

## Source
<optional GitHub issue URL>

## MCP Tools
The following MCP tools are available for this session:
- `report_progress(item_id, criteria_index, status, note)` — update AC checkpoint
- `request_review(item_id, message)` — pause and notify the human
- `get_backlog_item(item_id)` — retrieve current item context on demand
```

The `item_id` is embedded in the file so agents that re-read it mid-session have the correct identifier for subsequent MCP calls.

Cleanup: the file is deleted on session close (via the existing `EventExited` lifecycle hook). If the session crashes before the hook fires, the file persists until the next session startup reconciliation pass, which detects orphaned context files and removes them.

The worktree's `.gitignore` is updated at spawn time to include `.backlog-context.md` so the file is never committed accidentally.

## Alternatives Considered

**Option B: Prepend to `CLAUDE.md`**

Writing backlog context to the top of the project's `CLAUDE.md` file ensures Claude Code picks it up automatically on startup, but it mutates a project-owned file. This is a destructive operation: if the session is abandoned mid-work the prepended block remains in the file, corrupting project instructions for all other sessions on the same repo. Conflicts arise when two sessions share the same working directory. There is no clean rollback. Rejected.

**Option C: MCP resource only — agent pulls via `get_backlog_item`**

Exposing the item as an MCP resource that the agent fetches on demand avoids any filesystem writes, and the context is always current. However, this requires a live MCP connection to the Stapler Squad server. If the server restarts while a long-running session is active, the agent loses the ability to re-orient. Nothing forces the agent to call `get_backlog_item` at startup — it must already know the item ID, which it can only get from prior context. The MCP resource is valuable as a supplement (for mid-session re-orientation and AC updates) but is insufficient as the sole injection vector. Rejected as the primary mechanism; retained as a complement.

**Option D: Session `initial_prompt` field**

Using the existing `initial_prompt` field on the session record to carry the context block is session-scoped and requires no file writes. However, the initial prompt is consumed once at session startup and is not re-readable by the agent after compaction. As the session context grows and CLAUDE.md instructions lose relative weight (the Beads problem), the agent cannot re-read the initial prompt to re-orient. A file is re-readable at any time. Rejected as the primary mechanism; the `initial_prompt` may be used to tell the agent where to find the file.

## Consequences

**Positive**

- Survives server restarts: the file is on disk in the worktree, readable without any server involvement.
- Works across all session types: directory, new worktree, existing worktree, and one-off sessions all have a resolved `worktree_root` path.
- Re-readable at any time: unlike `initial_prompt`, the agent can re-read the file mid-session to re-orient after compaction.
- No mutation of project files: `.backlog-context.md` is `.gitignore`d and owned entirely by Stapler Squad.
- File is visible in the filesystem: agents already understand file-based context (they read `CLAUDE.md`) so this follows established conventions.
- Complements the MCP resource: both mechanisms can coexist without conflict.

**Negative**

- Writes a file to the user's working directory (potentially surprising if they list files).
- Context becomes stale if the backlog item's AC is edited while the session is running. Mitigation: record `context_snapshot_at` on the `ItemSession` record at spawn time; the review gate flags AC divergence explicitly.
- Requires cleanup logic on session exit. Mitigation: `EventExited` lifecycle hook handles deletion; reconciliation pass handles crash survivors.
- Token cost: injecting the full item context at session start consumes tokens before the agent reads any project files. Mitigation: strip verbose notes and source links from the injected file; keep only title and AC items. Full context remains available via `get_backlog_item`.
