# Research: Stack — backlog-agent-communication

**Date**: 2026-07-23

## Existing MCP Server Stack (what any new tool must fit into)

- **SDK**: `mark3labs/mcp-go` (see ADR-002 in `project_plans/stapler-squad-mcp-server/decisions/`),
  embedded directly in the Go binary (ADR-001: "go embedded MCP server" — no separate
  process). Tools registered via `s.AddTool(mcpgo.NewTool(...), handler)` on an
  `*mcpserver.MCPServer`.
- **Injection**: MCP config is injected into each spawned session's
  `.claude/settings.local.json` by `server/services/mcp_injector.go`, per ADR-005
  ("mcp injection strategy") — a new tool becomes available to pipeline sessions
  automatically once registered server-side; no per-session opt-in plumbing needed.
- **Tool file layout** (`server/mcp/`): one file per capability area —
  `tools_backlog.go` (backlog lifecycle: `get_backlog_item`, `report_progress`,
  `request_review`, `submit_review_verdict`, `submit_triage_result`),
  `tools_goal.go` (`set_session_goal`, task tree updates),
  `tools_lifecycle.go` (session-level lifecycle control), `tools_terminal.go`
  (terminal I/O), `tools_vcs.go` (git status/diff), `tools_github.go` (PR listing).
  New tools for this project's four dimensions most naturally extend
  `tools_backlog.go` (new backlog-scoped tools) or warrant a new
  `tools_backlog_escalation.go` / `tools_backlog_handoff.go` file if the tool count
  grows large enough to justify a split (existing precedent: `tools_backlog.go` at
  884 lines is already the largest tool file, `tools_terminal.go` at 755 lines is
  next).
- **Auth/caller identity**: every backlog tool call resolves `callerSessionUUID(ctx)`
  from `STAPLER_SESSION_UUID` (`server/mcp/tools_backlog.go:36`) and then verifies the
  calling session is linked to the target item via
  `storage.GetItemSessionBySessionAndItem` — role checks (e.g.
  `submitReviewVerdict` requires `itemSession.Role == "review"`) follow the same
  pattern. Any new tool needs the identical guard: resolve caller → verify item link
  → verify role if role-scoped.
- **Feature flag**: every backlog tool handler starts with
  `if r := featureDisabledResult(h.enabledCheck); r != nil { return r, nil }` — a
  single kill switch for the whole backlog MCP surface. New tools must follow this.
- **Error/result helpers**: `errResult(ErrInvalidArgument|ErrPermissionDenied|
  ErrInternalError, msg, hint)` and `mcpgo.NewToolResultText(...)` are the two
  canonical response shapes — structured error codes already exist and are the
  natural extension point for structured error/finding *codes* proposed in this
  project (e.g. a `report_infra_issue` tool's `category` field).

## Persistence Stack (ent ORM)

- **ORM**: entgo.io/ent, generated via `go run -mod=mod entgo.io/ent/cmd/ent generate
  --feature sql/upsert ./session/ent/schema` (the `--feature sql/upsert` flag is
  mandatory — see `.claude/rules/ent-schema-generation.md`; omitting it silently
  breaks `UpsertRule`-style methods).
- **Relevant existing schemas** (`session/ent/schema/`):
  - `item_session.go` — one row per pipeline-stage session (work/triage/review),
    already carries `ac_snapshot`, `verification_notes`, `last_commit_sha`,
    `pipeline_mode_snapshot(_hash)`, and a `ReviewVerdict` edge. This is the natural
    home for new structured "handoff context" fields (dimension 1) if the design
    lands on "attach richer structured JSON to the ItemSession row" rather than a
    new entity.
  - `backlogprogressnote.go` — append-only history table for `report_progress` calls,
    a `edge.From("item", BacklogItem.Type)`-linked child entity. Directly reusable as
    the structural pattern for a new append-only "review findings" or "dispute" log
    entity (dimension 1 / pain point B) — same shape: `item_id`, freeform text,
    a small status enum, `created_at` immutable, indexed on `(item_id, created_at)`.
  - `BacklogItem` (not read in full this pass, but referenced throughout) already
    carries `pr_url`, `pr_number` fields (pain point A) and the `ReviewVerdict` edge
    used by `latestReviewVerdict`.
- **ReviewVerdictData** (`session` package, not ent-generated): `ItemSessionID`,
  `OverallOutcome`, `PerCriterion` (JSON `[]CriterionVerdict`), `Summary` (freeform
  string) — this is the existing "structured-ish" review output. `PerCriterion`
  already has an `Evidence` string per criterion (see `session/domain/backlog.go`
  `CriterionVerdict`), so some structure already exists; what's missing is anything
  beyond per-criterion PASS/FAIL/PARTIAL/UNVERIFIABLE + evidence text — e.g. no
  category/severity taxonomy, no "type of finding" (bug vs scope-gap vs
  infra-broken), no forward link agents downstream can query cheaply beyond parsing
  the JSON string.

## Notification / Human-Visibility Stack

- **`session.Notifier` interface** (`session/backlog_lifecycle.go`): a single
  `Notify(itemID, title, message string, notificationType, priority int32)` method,
  implemented outside `package session` (adapter over `pkg/events`, avoiding an
  import cycle). Every existing "tell the human something happened" call in the
  backlog pipeline (stuck-parked notices, review-session-exited-without-verdict,
  auto-rework-paused) goes through this one call. This is the natural transport for
  any new "ask for help" / "infra broken" / "verdict disputed" human-facing signal —
  no new notification plumbing needed, just new call sites with well-chosen
  `notificationType`/`priority` values (types/priorities are `sessionv1.
  NotificationType`/`NotificationPriority` proto enums — check
  `proto/session/v1/session.proto` for the full value set before adding a new one).
- **`/unfinished` UI + `StuckReason`**: the durable, queryable human-visible surface
  for anything that needs a *persistent, actionable* row (not just an ephemeral
  push notification) — see `research/architecture.md` for the full mechanics. A
  `web-push-notifications` feature already exists per `project_plans/
  web-push-notifications/` (not re-read this pass, but relevant prior art for the
  transport layer if push-to-phone is considered for "ask for help" urgency).

## Proto / Codegen Conventions

- Any new backend RPC surface (e.g. a UI-facing "resolve escalation" or "adjudicate
  dispute" action, distinct from the MCP tool an agent calls) goes in
  `proto/session/v1/*.proto` → `make proto-gen`, following the same pattern BUG-040's
  fix used for `StuckReasonPRPendingNoPR` (new proto enum value +
  `toProtoStuckReason`/`fromProtoStuckReason` mapping in
  `server/services/backlog_service_stuck.go` + TypeScript-exhaustive
  `Record<StuckReason, T>` maps in `web-app/src/components/backlog-stuck/
  stuckReason.ts`). Any new `StuckReason`-shaped enum this plan introduces should
  follow that exact checklist.

## Frontend Stack (if UI surfaces are proposed)

- vanilla-extract for new CSS (`.claude/rules/css-architecture.md`) — no CSS Modules
  for new components.
- Existing `/unfinished` page and backlog item detail page
  (`web-app/src/components/backlog*`, `web-app/src/components/backlog-stuck/`) are
  the two most likely integration points for new human-visible states (escalations,
  disputes, infra reports) rather than a wholly new page — consistent with the
  "reuse existing surfaces" constraint in requirements.md.

## No New External Dependencies Anticipated

Nothing researched so far suggests a new third-party library, service, or
infrastructure component is needed — the four dimensions are additive surface on
top of the existing MCP tool / ent schema / Notifier / StuckReason stack, which
matches the "low operational overhead, no new infrastructure" constraint in
requirements.md. This should be revisited in the plan phase only if research turns
up a case (e.g. a genuinely new "Master agent" always-on service) where the
lightest-weight option is insufficient.
