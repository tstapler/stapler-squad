# Requirements: backlog-item-activity-log

**Date**: 2026-08-18
**Type**: Feature addition (existing project: stapler-squad backlog automation)
**Mode**: Auto Mode — ideate interview skipped; requirements synthesized directly from a
detailed incident report supplied by the user. No open interview questions remain.

## Problem Statement

stapler-squad's backlog automation spawns dedicated Claude Code sessions with
`role="work"/"review"/"triage"` for a specific backlog item, with `STAPLER_SESSION_UUID`
set in their environment. The MCP tools that record official progress/verdicts
(`report_progress`, `request_review`, `report_blocked`, `report_duplicate`,
`report_pr_created`, `submit_review_verdict` — see `server/mcp/tools_backlog.go`) are
gated on that env var matching the item's assigned session.

In practice, a session that is *not* one of these spawned role sessions — a developer's
long-running general dev/CLI session, or another session that picks up an item ad hoc —
has no `STAPLER_SESSION_UUID` (or one that doesn't match the item). Every one of the
tools above then returns `PERMISSION_DENIED`, so that session has no way to record
status, leave a note, or hand off context to whoever looks at the item next. The only
trace of that work is the session's own chat transcript, invisible to the backlog system,
reviewers, and other sessions.

**Concrete incident** (this session, 2026-08-18): picked up ready backlog item
`772ba14e-e80a-448b-84c6-fc82089687e3` ("Cross-test flakiness among newly-parallelized
session/server-services tests") from a session with no matching `STAPLER_SESSION_UUID`,
fixed a confirmed data race, and called `report_progress` — rejected with
`PERMISSION_DENIED: STAPLER_SESSION_UUID not set — this tool must be called from a
session spawned by Stapler Squad`.

## Users / Consumers

- **Primary**: any Claude Code session (work/review/triage-spawned, ad hoc dev session,
  or another agent) that wants to leave a note on a backlog item it is not strictly
  assigned to.
- **Secondary**: humans and other sessions reading `get_backlog_item` output or watching
  the live event stream (`WatchBacklogItems`) to see what's happened on an item.
- **Secondary**: the web UI's `BacklogItemDetail` page, which should render the update
  history live.

## Success Metrics

- A session with no `STAPLER_SESSION_UUID` (or a mismatched one) can successfully post a
  free-form status update to a backlog item via a new MCP tool, without needing the
  strict role/session gating that `report_progress`/`submit_review_verdict` use.
- Each posted update records enough identity/provenance (calling session UUID if present,
  session title, timestamp) to answer "who said this and when" — no fully anonymous
  entries.
- Updates are visible in `get_backlog_item` output and pushed live through the existing
  event bus (`server/services/backlog_service_events.go`) so `BacklogItemDetail` updates
  without a page refresh, matching the existing pattern for status/verdict changes.
- Existing role-gated tools (`report_progress`, `request_review`, `report_blocked`,
  `report_duplicate`, `report_pr_created`, `submit_review_verdict`) are provably
  unchanged: same gating, same semantics, same tests still pass.
- Regression test proves an ungated/mismatched-UUID session CAN post and read updates,
  and that the gated tools remain rejected under the same conditions.

## Constraints

- No hard deadline. No explicit performance/compliance requirement.
- Must follow this repo's existing conventions: Conventional Commits, `make quick-check`
  / `make ci` as the pre-push gate, ent schema regen requires
  `--feature sql/upsert` (`session/ent/generate.go`), proto changes go through
  `make proto-gen`, PRs are opened as drafts (`gh pr create --draft`) and never
  self-merged.
- Must reuse an existing "notes"/"activity" concept in the backlog item model if one
  exists (checked during research — see below) rather than invent a parallel one.
- Must not weaken or change the behavior of `report_progress`, `request_review`,
  `report_blocked`, `report_duplicate`, `report_pr_created`, `submit_review_verdict`.
- Must follow this repo's Go idiom rules during implementation: no speculative
  interfaces, no same-typed-primitive parameter piles
  (`.claude/rules/primitive-obsession-checklist.md`), prefer go-git over subshells where
  relevant (`.claude/rules/prefer-go-git-over-subshells.md`), and invoke the
  `/go-development`-family skills for any `.go` work per CLAUDE.md.
- Untrusted-input handling: the free-form update text is LLM/user-controlled and must get
  the same sanitization care as other similar fields — precedent: PR #534 (`7b9aee4cd`,
  "sanitize LLM-controlled triage title against path traversal").

## Scope

### In Scope

- One new MCP tool (name decided during planning, e.g. `post_backlog_update`) that:
  - Is callable from any session — with or without `STAPLER_SESSION_UUID`, and
    regardless of whether that UUID matches the item's assigned session.
  - Accepts a free-form message plus the target `item_id`.
  - Records identity/provenance per update (session UUID if present, session title,
    timestamp), following the `session_id` parameter + env-fallback precedent in
    `server/mcp/tools_goal.go`.
  - Sanitizes/validates the free-form text before persisting.
- Persistence for an ordered list of `{author session/title, timestamp, message}` entries
  per backlog item — reusing an existing model/table if one is found during research,
  otherwise the simplest new structure that satisfies this shape (ent schema change
  likely, using `--feature sql/upsert` on regen).
- Exposing the update log through:
  - `get_backlog_item` MCP tool output.
  - `GetBacklogItem`/equivalent RPC response (proto change likely, `make proto-gen`).
  - The live event stream (`WatchBacklogItems` / `server/events`) so new updates push to
    connected clients without a refresh.
- Web UI: `BacklogItemDetail` renders the update/activity log, and reflects new entries
  pushed via the live stream.
- Feature registry updates if a new RPC/component/marker is introduced
  (`make registry-generate`, `// +api:` / `// +feature:` markers,
  `docs/registry/features/`).
- Tests: new MCP tool, new/changed service method(s), storage/ent layer, and a web UI
  test for rendering the log.

### Out of Scope

- Any change to the gating, behavior, or meaning of `report_progress`, `request_review`,
  `report_blocked`, `report_duplicate`, `report_pr_created`, `submit_review_verdict`.
- A full threaded-comments/reply system (replies-to-replies, reactions, edit/delete
  history). Start from "append a timestamped, attributed note; read the log back."
  Broaden only if research finds a strong, concrete reason to.
- Access control beyond what already exists for backlog items (no new
  per-update permissions/roles).
- Editing or deleting existing update entries (append-only log for v1).

## Open Questions

None outstanding — the user's brief is exhaustive enough to proceed directly to
research. Design specifics intentionally deferred to Phase 2/3 per the user's own
instructions:
- Exact new MCP tool name (candidates: `post_backlog_update`, `add_backlog_note`).
- Whether an existing notes/activity/comment concept already exists in
  `session/backlog*.go` / `server/services/backlog_service*.go` to reuse instead of a new
  ent schema (research must check this thoroughly before designing new persistence).
- Exact shape of the ent schema / proto message for the update log entry.
- Whether the update log is stored as a dedicated ent type with an edge to the backlog
  item, or as a JSON-serialized field on the existing item (decided in Phase 3 based on
  what research finds about existing patterns, e.g. how status history or verdicts are
  currently persisted).
