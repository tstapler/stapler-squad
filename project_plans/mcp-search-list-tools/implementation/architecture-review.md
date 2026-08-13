# Architecture Review: mcp-search-list-tools
**Date**: 2026-08-12
**Verdict**: CONCERNS (1 concern, non-blocking; plan is otherwise clean)

## Constitution Check

No `docs/adr/ADR-000-architecture-constitution.md` found in the repository (`docs/adr/`
contains ADR-001 through ADR-026 plus several un-prefixed numbered ADRs, but no ADR-000) —
skipped.

## Summary

This is a small, mechanical infra task — three thin MCP presentation-adapter handlers
wrapping three already-existing ConnectRPC service methods, with no new domain model and no
persistence layer. Verified against the actual code (not just the plan's own claims):

- `workflowHandlers`/`rulesHandlers` really do have the single-field `{svc
  *services.SessionService}` shape the plan says `notificationHandlers`/`searchHandlers`
  should mirror (`server/mcp/tools_workflow.go:18-20`, `server/mcp/tools_rules.go:16-18`).
- `okResult`/`errResult`/`workflowServiceErrResult` exist exactly as described
  (`server/mcp/tools_discovery.go:73-85`, `server/mcp/tools_workflow.go:375-388`) and are
  legitimately reused, not duplicated.
- `ListBacklogItemsResponse` genuinely has no `total_count`/`has_more`/limit fields
  (`proto/session/v1/backlog.proto:355-357`) — the plan's client-side
  truncate-and-count-before-truncating design (Task 1.1.1b) is the correct adapter response
  to that RPC gap, not an invented workaround.
- `GetNotificationHistoryRequest`/`SearchClaudeHistoryRequest`/proto message field lists
  (`session.proto:972-993,1344-1381`) match the plan's field-by-field mapping exactly,
  including the `BacklogItem.category` field the plan cites (`backlog.proto:147`).
- `build-vs-buy.md`'s verdict ("hand-write, do not adopt codegen") matches the plan's Pattern
  Decisions table's stated rationale verbatim — no drift between research and plan.
- An unrecognized `sort_by` degrades safely to the default ordering
  (`session/ent_repository_backlog.go:424-430`, a `default:` case, not silent
  zero-results) — confirming the plan's asymmetric decision to validate `status` (whose
  failure mode actually is silent-empty-results) but not `sort_by` is correct, not an
  inconsistency.

No SOLID, layering, DDD-boundary, sum-type, or GoF/PoEAA mismatch findings survived
verification. The Pattern Decisions table (lines 39-46) already pre-empts and correctly
resolves the primitive-obsession, interface-pollution, and pattern-selection questions this
review would otherwise raise — treat that table as evidence review already happened once
during planning, not merely as assertions.

## Blockers

None.

## Concerns

- [ ] **Task 1.1.1a** (`server/mcp/tools_backlog.go`) — `validBacklogStatuses` is a
  hand-typed `[]string{"idea", "refining", ..., "archived"}` literal that duplicates, rather
  than derives from, the typed `session/domain.BacklogStatus*` constants
  (`session/domain/backlog.go:16-24`) it's meant to mirror. No exported `AllBacklogStatuses`
  slice exists yet to source from (confirmed — grepped `session/domain/*.go` and
  `session/*.go`), so today's 9-entry list is accurate, but it's a second, independent
  source of truth: if a future change adds a 10th `BacklogStatus*` constant and forgets this
  list, `list_backlog_items` will reject a status value that's actually valid — silently,
  since the handler's own explicit purpose (per pitfalls.md §4, cited in the plan) is to
  catch exactly this class of mismatch for the *inverse* case (unknown value from the
  caller), not to guard itself against drifting out of sync with its own source enum. This
  is the "Parse, Don't Validate" boundary done partially: the handler validates a raw string
  against a copy of the domain enum instead of either (a) building the copy from the enum,
  or (b) parsing into the domain `BacklogStatus` type before use.
  **Remediation**: change Task 1.1.1a to build the var from the constants —
  `var validBacklogStatuses = []string{string(domain.BacklogStatusIdea),
  string(domain.BacklogStatusRefining), string(domain.BacklogStatusReady), ...}` — so the
  two lists cannot silently diverge. (A full parse-to-`BacklogStatus`-then-back-to-string
  round trip isn't warranted: `ListBacklogItemsRequest.Status` is `repeated string` with no
  proto enum counterpart, so the value has to cross the RPC boundary as a string regardless
  — deriving the literal list from the constants captures the whole benefit at near-zero
  cost.) This is a one-line change to Task 1.1.1a's bullet, not a restructure.

## Nitpicks

- The plan's `type_filter` handling (Task 1.2.1c, `notificationTypeByName`) does the
  string→enum parse-at-boundary correctly, including leaving the proto `optional
  NotificationType` field genuinely nil (not a pointer to the zero value) when the caller
  omits it — worth calling out as the pattern to imitate if the `validBacklogStatuses`
  concern above is ever revisited for a case where a real proto enum exists.
- Task 1.3.1f's explicit, PR-flagged fallback (test argument-mapping only if the FTS content
  fixture proves too heavy, deferring content-match coverage to `session/search`'s existing
  engine-level tests) is the right way to handle a testability gap under this rule set — it
  names the gap rather than silently skipping it. No action needed; noted as a positive
  pattern, not a finding.
