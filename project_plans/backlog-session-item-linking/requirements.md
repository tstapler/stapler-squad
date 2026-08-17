# Requirements: backlog-session-item-linking

**Date**: 2026-08-16
**Type**: bugfix + feature addition (extends `server/mcp/tools_backlog.go` MCP surface and
`session/backlog_commands.go` slash-command generation)
**Source item**: `3224bc15-6025-495b-9dff-219d7d0892b5` — "MCP backlog harness: no way to
(re)link a session to an item, and skill templates use a stale placeholder item_id"

## Problem Statement

While working backlog item `0e775762-2aa7-48c5-a3fb-bea11b3a0350` from an agent session, the
reporting session hit two related gaps in the backlog MCP harness that together made it
impossible to formally close the item, even though the underlying work was independently
verified (`git diff`, `git show`, `go build`, `go test`, `gh pr view` all confirmed the fix
was already merged via PR #503).

### Gap 1 — no agent-facing way to (re)link a session to a backlog item

`request_review`, `report_progress`, and `submit_triage_result`
(`server/mcp/tools_backlog.go:304,376,505,665,758`) all reject calls against a real item
with a bare `PERMISSION_DENIED: this session is not linked to the specified backlog item`
whenever no `item_sessions` row exists for the calling session UUID. This happens whenever a
session picks up in-flight work after a prior session already advanced (or fully merged) the
item — the new session's UUID was never written to `item_sessions`.

Investigation confirms the backend already has the primitive needed to fix this:
`BacklogService.AttachSessionToItem` (`server/services/backlog_service_sync.go:29`) links an
existing session to an item and is unit-tested (`backlog_service_test.go:1709`,
`backlog_service_triage_test.go:2022`). It is only ever called from `SpawnSessionFromItem`
during normal item-triage session creation — it is **not exposed as an MCP tool**, so a
session that discovers mid-run (e.g. from its own branch name) that it should be linked to an
item has no callable path to fix its own linkage. The only workaround found was reading the
workspace-scoped SQLite DB directly (`~/.stapler-squad/workspaces/<workspace-id>/sessions.db`,
`item_sessions` table) — fragile (schema not guaranteed stable) and not something a session
should need to do to unblock itself.

The `PERMISSION_DENIED` error itself also carries no actionable detail: no reason ("no
`item_sessions` row found for this session") and no hint ("call X to fix this"), so an agent
hitting it has no way to self-correct without a human or out-of-band DB access.

### Gap 2 — generated skill/command templates can carry a stale `item_id`

`/backlog:status`, `/backlog:done-N`, `/backlog:fail-N`, `/backlog:review`, and
`/backlog:ship` are generated per-session by `buildDefaultSlashCommandSet`
(`session/backlog_commands.go:79-136`), which bakes the `item.ID` available **at generation
time** into each command's static markdown body via `WriteSlashCommands`
(`session/backlog_commands.go:31`). `WriteSlashCommands` has exactly two real callers today
(`server/services/backlog_service_triage.go`'s `SpawnSessionFromItem` and
`server/services/backlog_service_sync.go`'s `AttachSessionToItem`), both of which regenerate
the files from the item passed in at that moment.

Reproduced in this very triage session: its generated `/backlog:*` commands reference
`item_id=b608ab1e-b86e-4130-8879-7328cd363063`, which does not exist
(`get_backlog_item` → `ITEM_NOT_FOUND`) and is not this session's actual item id
(`3224bc15-6025-495b-9dff-219d7d0892b5`). Following the generated instructions verbatim fails
immediately. Because regeneration is coupled to the two existing call sites, any path that
changes which item a session is *effectively* working on without going through one of those
two functions (or that goes through them but the caller reasons about the wrong item) leaves
stale command files on disk with no signal that they're wrong.

### Net effect

A session cannot discover why it's blocked, cannot self-heal its own linkage, and cannot
trust its own generated slash commands to name the right item — the fix from Gap 1
(exposing `AttachSessionToItem` as an MCP tool) should also regenerate the slash commands
via the existing `WriteSlashCommands` path so Gap 2 cannot recur after a relink.

## Goals

1. Give an agent session a callable MCP tool to link/relink itself to a specific backlog
   item, reusing the existing `BacklogService.AttachSessionToItem` logic rather than
   duplicating it.
2. Make the `PERMISSION_DENIED: not linked` error actionable — name the specific cause and
   the tool that resolves it.
3. Provide a way to resolve "which item does my current branch belong to" without hand-
   parsing the branch string or reading SQLite directly.
4. Guarantee that after a (re)link, this session's generated `/backlog:*` slash commands
   reference the correct item id — eliminate the stale-placeholder failure mode structurally,
   not by asking future sessions to double check it by hand.
5. Provide a read-only way to inspect current session↔item linkage without reaching into the
   SQLite file.

## Non-Goals

- Redesigning the overall backlog pipeline, reconciliation, or stuck-item remediation
  machinery (`session/backlog_remediation.go`, `StuckReason`) — out of scope.
- A generalized "conversation handoff" protocol between arbitrary agents (LangGraph/AutoGen-
  style) — this item only needs session→item linkage, not multi-agent handoff.
- Automatic/implicit relinking without an explicit tool call — an agent must call the new
  tool deliberately; the harness should not silently reassign sessions to items based on
  branch-name heuristics alone (a session could be on a stale branch for legitimate reasons).
- Fixing every possible cause of a stale `item_sessions` row (e.g. reconciler races) — this
  item is scoped to giving the *agent* a self-service recovery path, not eliminating every
  root cause that could produce the mismatch.

## Acceptance Criteria (initial — refined further in plan/validate)

1. A new MCP tool (e.g. `link_session_to_backlog_item`) exists, is registered in
   `registerBacklogTools`, and calls into `BacklogService.AttachSessionToItem` (or an
   equivalent path) to create/update the `item_sessions` row for the calling session UUID.
2. Calling the new tool against a valid item id it isn't yet linked to succeeds and a
   subsequent `request_review`/`report_progress`/`submit_triage_result` call against that
   item id no longer returns `PERMISSION_DENIED`.
3. The new tool rejects (with a clear error) linking to an item id that doesn't exist, and
   documents any role/state constraints inherited from `AttachSessionToItem` (e.g. item
   status requirements) in its tool description.
4. The five existing `PERMISSION_DENIED: this session is not linked...` error sites in
   `server/mcp/tools_backlog.go` are updated to include a `hint` (or equivalent structured
   field already used elsewhere in the MCP error envelope) pointing at the new tool.
5. After a successful (re)link via the new tool, this session's `.claude/commands/backlog/*`
   files are regenerated (via the existing `WriteSlashCommands` path) so they reference the
   newly linked item id — verified by reading the file contents post-call in a test.
6. A read-only introspection path exists (MCP tool or an addition to `get_backlog_item`'s
   response) to show which item(s) the calling session is linked to, without requiring direct
   SQLite access.
7. Test coverage: unit test(s) for the new MCP tool (success, not-found item, already-linked
   idempotency) and for the slash-command regeneration side effect.

## Constraints / Context

- Repo convention: ent schema changes require `go run -mod=mod entgo.io/ent/cmd/ent generate
  --feature sql/upsert ./session/ent/schema` — only relevant if the plan needs an `item_sessions`
  schema change (current expectation: no schema change needed, `AttachSessionToItem` already
  covers the write path).
- MCP tool additions follow the existing pattern in `server/mcp/tools_backlog.go`
  (`mcpgo.NewTool(...)`, registered in `registerBacklogTools`).
- `make build && make test` (generates protos) is the standard verification gate;
  `make quick-check` for build+test+lint.
- Branch-name-derived item lookup (from the "suggested harness additions" in the source item)
  is a stretch goal folded into Goal 3 / AC 6 scope during planning — the branch format
  observed is `backlog/triage-<session-uuid>-<item-id>-<slug>` (confirmed from the reporting
  session's own branch, `backlog/triage-dad636fa-e690-4992-87d5-6c9a272cb7eb-18cb9fbad00a6bf8-fix-deletesession-goroutine-leak-timeout`
  — note this example's second segment is not itself a bare item-id UUID, so the plan phase
  must confirm the actual parse rule against `session/backlog_lifecycle.go` /
  `server/services/backlog_service_triage.go` branch-construction code before committing to a
  parser).

## Open Questions — resolved by Phase 2 research

- **Is `AttachSessionToItem`'s state transition safe to trigger outside spawn time?** Yes —
  resolved (`research/architecture.md`). Its status guard only permits attaching to `idea`/
  `ready`/`in_progress` items (`server/services/backlog_service_sync.go:54-60`), and the
  `in_progress` transition it triggers is one-directional and a no-op once already there
  (`session.CanTransitionBacklog`, `session/domain/backlog.go:391`). No new parameter needed
  to call it safely from an arbitrary point in a session's lifecycle.
- **Restrict relinking to branch-matching sessions?** Left manual/explicit per the Non-Goals
  above — the tool description states this explicitly (see plan.md).
- **What is the actual branch-name grammar?** Resolved, and the original assumption in this
  document was **wrong**. `research/build-vs-buy.md` traced the real construction path
  (`session/instance_worktree.go:108` → `server/services/backlog_service_triage.go:706-720`)
  and found branches are named `backlog/<repoName>-<title-slug>` — **no session UUID or item
  ID is ever embedded**, unlike the `backlog/triage-<uuid>-<item-id>-<slug>` shape assumed in
  the Problem Statement (which was this reporting session's own worktree/tmux name, not its
  git branch — the two are different strings in this codebase). **Goal 3 is re-scoped**:
  instead of a branch-name parser, use the already-shipped reverse lookup
  `Storage.GetItemSessionBySessionUUID(ctx, sessionUUID)` (`session/storage.go:932`), which
  answers "which item is *this* session currently linked to" directly from `item_sessions`
  with no string parsing at all. This also directly serves Goal 5 (introspection) — the two
  goals converge on one lookup.

## Additional findings from Phase 2 research (see research/*.md for full detail)

- `server/mcp/tools_backlog.go`'s `errResult(code, message, remediation string)` already has a
  third parameter for actionable hints (`server/mcp/tools_discovery.go:73`) — the 5
  `PERMISSION_DENIED` sites just pass `""` for it today (e.g. `tools_backlog.go:376,505,665,758`).
  AC4 is a matter of filling in that existing parameter, not adding new error-envelope plumbing.
- `AttachSessionToItem` has **no exclusivity or idempotency guard**: it always inserts a new
  `ItemSession` row (no unique constraint on `item_id` in `session/ent/schema/item_session.go`)
  and never checks whether another live session already holds the item — exposing it verbatim
  would let a session hijack/duplicate another session's in-progress item. The plan must add a
  per-item liveness check (there is no existing per-item primitive — `hasActiveWorkSession`/
  `countLiveBacklogWorkSessions`, `server/services/backlog_service_triage.go:896`, is aggregate/
  WIP-cap only) before exposing this as an MCP tool.
- `backlogHandlers` (`server/mcp/tools_backlog.go:91`) has no `*BacklogService` reference today.
  The HTTP-transport MCP path can thread it through cheaply via existing DI
  (`server/dependencies.go:1200`); the stdio-transport fallback (`main.go:1040`,
  `buildMCPDeps`) does not have access to construct one and the new tool should degrade
  gracefully (a clear "not available over this transport" error) rather than force new stdio
  plumbing — this is a plan-phase scoping decision.
- `WriteSlashCommands` (`session/backlog_commands.go:31-68`) writes each command file via a
  direct, non-atomic `os.WriteFile` (unlike `WriteBacklogContextFile`'s temp+rename) — a
  crash mid-loop can leave a mixed stale/fresh file set. Pre-existing, but a new agent-callable
  relink path increases how often that window gets hit; call out in plan.md whether it's in
  scope to fix here or tracked separately.
