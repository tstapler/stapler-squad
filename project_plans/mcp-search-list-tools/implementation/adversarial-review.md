# Adversarial Review: mcp-search-list-tools

**Date**: 2026-08-12
**Verdict**: CONCERNS

## Citation Spot-Check Summary

I verified every exact file:line, function-signature, and struct-field citation the plan
makes that is load-bearing for an implementer, against the live source in this worktree.
**All of them checked out** — none are wrong or dangling:

- `server/mcp/tools_backlog.go:118` (`backlogHandlers` struct), `:84` (`allowedSelfResolveSourceStatuses`
  ends there), `:194`/`:318` (`getBacklogItem` start/end), `:204` (dangling-reference string,
  exact text match), `:1716` (`registerBacklogTools`), `:217` (`SanitizeForAgentContext` call site)
  — all exact.
- `server/mcp/tools_workflow.go:338-371` (`stringArg`/`stringPtrArg`/`boolArg`/`boolPtrArg`/`int32PtrArg`)
  and `:375` (`workflowServiceErrResult`, body confirmed to map NotFound/InvalidArgument/Unavailable
  and fall through to Internal exactly as described) — exact.
- `server/mcp/tools_discovery.go:73-85` (`errResult`/`okResult`) — exact.
- `server/mcp/server.go:59-62` and `:63-66` (the two `if` blocks the new `register*Tools` calls
  are wired into) — exact, verbatim.
- `server/mcp/server_test.go` `TestToolRegistrationCount` — scans exactly the 5 files the plan
  names (`server.go`, `tools_discovery.go`, `tools_lifecycle.go`, `tools_terminal.go`,
  `tools_vcs.go`), hardcoded count is exactly 16, and none of the new tools land in those 5
  files — the "no edit needed" claim is correct.
- `session/domain/backlog.go:16-24` (9 `BacklogStatus*` constants) — exact, and `session.BacklogStatus*`
  aliasing into that package (`session/backlog.go:14-25`) confirmed.
- `proto/session/v1/types.proto` `NotificationType` enum (14 non-UNSPECIFIED values) — the plan's
  `notificationTypeByName` list matches all 14 names exactly.
- `services.NewBacklogService(storage, nil, nil, nil, nil, nil)` — signature is
  `(storage *session.Storage, creator SessionCreator, cfg *config.Config, engine session.WorkflowEngine,
  pipelineEngine session.PipelineEngine, pipelineModeRepo session.PipelineModeRepository)`, 6 params,
  matches; confirmed used 3x already in `tools_backlog_test.go` (incl. line 754).
- `notifications.NewNotificationHistoryStore(filePath string) (*NotificationHistoryStore, error)` — exact.
- `session.SanitizeForAgentContext(s string, maxLen int) string` — exact.
- `services.NewSessionServiceWithSearchEngine(storage session.InstanceStore, eventBus *events.EventBus,
  searchEngine *search.SearchEngine) *SessionService` — exact; `*session.Storage` (what
  `newTestBacklogStorage` returns) does satisfy `session.InstanceStore` (compile-time assertion at
  `session/storage.go:221`).
- `mcpgo.Items(schema any) PropertyOption` — verified to exist in the pinned
  `github.com/mark3labs/mcp-go@v0.48.0` module cache (`mcp/tools.go:1304`) with exactly the
  `map[string]any{...}` usage the plan's code blocks use.
- `ListBacklogItemsResponse` proto message genuinely has no pagination fields (`Items` only) —
  confirms the plan's "the RPC has none" claim about a native limit.
- `SearchService.SearchClaudeHistory` genuinely calls `ss.searchEngine.IncrementalSync(hist)`
  unconditionally on every call (`server/services/search_service.go:~479-480`) — confirms the
  pitfalls-doc claim that a hand-seeded `IndexMessage` fixture would be wiped by the next call,
  justifying the heavier on-disk-fixture test approach in Task 1.3.1f.
- `NotificationService.GetNotificationHistory`'s `UnreadCount` genuinely comes from
  `ns.notificationStore.GetUnreadCount()` (store-global), not from the filtered result set —
  confirms the plan's AC wording ("UnreadCount reflects the store's own unread count, not the
  filtered count") is accurate, not just asserted.

No BLOCKER-level citation errors found. This plan's citations are unusually well-verified.

## Blockers

None.

## Concerns

- **`priority` filter has no runtime validation, unlike `status`** — Task 1.1.1a/1.1.1b build
  `validBacklogStatuses`/`isValidBacklogStatus` specifically so an unrecognized `status` value is
  rejected with a remediation hint instead of silently matching zero rows (this is Story 1.1.1's
  second AC, explicitly testing for it). The same class of bug applies to `priority`: nothing in
  Task 1.1.1b's description validates that each parsed `priority` value is in `1..5` before it
  reaches `BacklogService.ListBacklogItems` — the JSON-schema `minimum`/`maximum` hint registered
  in Task 1.1.1c is a client-side hint only, not a server-side guarantee, and `BacklogItemFilter`'s
  underlying storage query will simply match zero rows on an out-of-range value, exactly the
  failure mode the plan calls out and fixes for `status` but not `priority`. Recommendation: add
  the same explicit range check (`1 <= p <= 5`) with an `ErrInvalidArgument` + remediation message,
  mirroring the status-validation code path, or explicitly note in the plan why priority is exempt.

- **`search_claude_history`'s per-call `IncrementalSync` cost is flagged by research but not
  addressed in the plan's Risk Control section.** `pitfalls.md` §5 explicitly names this as "the
  most likely candidate" among the three new tools to need rate limiting, since every single call
  triggers a full FTS `IncrementalSync` over Claude history. The plan's Risk Control section
  discusses feature-flag asymmetry as a named, accepted risk but is silent on this one — no rate
  limit is added, and no explicit "accepted, here's why" note appears (unlike the flag-asymmetry
  callout, which does get one). An LLM client that calls this tool repeatedly (e.g. in a retry
  loop, or multiple sessions concurrently) can trigger repeated full index syncs with no
  backpressure. Recommendation: either add a lightweight rate limit (the `writeLim`/`tokenBucket`
  pattern already exists in this package) or add an explicit sentence to the Risk Control section
  naming this as an accepted risk for the same reason `registerDiscoveryTools`'s existing
  unthrottled tools are accepted (matches precedent, but say so).

- **`list_backlog_items` inherits the backing RPC's fully unbounded fetch.** `BacklogService.ListBacklogItems`
  has no server-side pagination — it calls `storage.ListBacklogItemSummaries(ctx, filter)` and
  returns everything that matches, with the MCP handler doing the only truncation, client-side,
  after the full result set is already materialized in memory (confirmed: `ListBacklogItemsResponse`
  proto has no limit/offset fields). This is pre-existing RPC behavior, not introduced by this plan,
  but the plan is what turns it into an LLM-triggerable, no-feedback-loop query path for the first
  time via MCP (the web UI presumably has its own pagination UX friction; an LLM does not). As the
  backlog table grows, this becomes an increasingly expensive call with no signal to the plan's own
  design that this exists. Not necessarily blocking, but worth a one-line acknowledgment in Risk
  Control alongside the other two.

- **Task 1.3.1f's fallback explicitly permits skipping true end-to-end coverage of the story's
  primary acceptance criterion.** Story 1.3.1's first AC ("A query returns matching sessions with
  snippets, defaulting to 10 results") is the central behavior of the tool. Task 1.3.1f's own text
  says if the on-disk FTS fixture "proves too heavy," an acceptable fallback is testing only
  argument-parsing/mapping against an empty index, deferring content-matching coverage to the
  already-existing `session/search` package tests. That existing coverage never exercises the new
  `searchClaudeHistory` MCP handler's own mapping/truncation logic (snippet sanitization via
  `SanitizeForAgentContext`, `SearchResultBrief` field mapping) against real matched content — so
  taking the fallback leaves the handler's actual data-shaping code, the one part of Epic 1.3 this
  plan is adding, uncovered by any test that exercises a real match. The plan does say to flag this
  explicitly rather than silently skip it, which is good practice, but as written the fallback is a
  live option, not a last resort — recommend making the on-disk fixture the required path and the
  argument-only test purely additive, given how load-bearing this AC is.

- **`workflowServiceErrResult`'s hardcoded error text ("workflow operation failed: %v") is reused
  a third time for domains it doesn't name.** The helper is already reused by `tools_rules.go` for
  approval rules; this plan reuses it again for backlog listing, notification history, and Claude
  history search. Its Internal-error fallback message literally says `"workflow operation failed: %v"`
  regardless of which of these five now-unrelated domains triggered it — an LLM client (or a human
  debugging via the MCP transcript) reading `INTERNAL_ERROR: workflow operation failed: failed to
  list backlog items: ...` will be misled about which subsystem actually failed. This is a
  pre-existing wart the plan explicitly chooses to extend rather than fix (reasonable, given the
  interface-pollution-checklist's stance against speculative abstraction for a single new caller),
  but the plan should at minimum note this cosmetic inconsistency exists post-change, since it's
  now shared across 5 unrelated domains instead of 2.

## Minors

- **`BacklogItemBrief.Category` is typed `string`, but the source proto field is
  `optional string category = 29` (a `*string` in generated Go, accessed via `item.GetCategory()`).
  The plan's task text doesn't call out the pointer-to-value conversion; an implementer following
  the struct literally could write `Category: item.Category` and fail to compile. Trivial to fix,
  but worth naming so the implementing subagent doesn't stall on it.
- **Glossary/task inconsistency**: the Domain Glossary describes `SearchResultBrief` as including a
  `metadata` field, but Task 1.3.1b's actual struct definition (`SearchResultBrief struct { SessionID,
  SessionName, Project string; MessageIndex int32; Score float32; Snippets []SearchSnippetBrief }`)
  omits it. Not fatal — the Task-level code block should win over the glossary prose — but worth a
  one-line fix so the two don't disagree.
- **The plan's own "55-tool convention" citation (Pattern Decisions row 1, sourced to `architecture.md §5`)
  repeats a stale count.** `architecture.md §5` itself already discloses that a live `grep -c
  'mcpgo.NewTool('` across `server/mcp/*.go` returns 39, not 55, and that the gap's source was
  "not re-derived" — i.e. the research phase flagged this as an open discrepancy rather than
  resolving it, and the plan then cites the unresolved "55" figure without carrying the caveat
  forward. Independently confirmed via `grep -rc "mcpgo.NewTool(" server/mcp/*.go` (excluding
  tests): total is 39. Doesn't change the architectural conclusion (resource-scoped tools are
  still clearly the dominant, correct convention at 39 tools too), so this is cosmetic — but a
  plan that spot-checks its own line numbers this carefully elsewhere shouldn't leave a
  known-wrong number sitting in its own rationale table.
