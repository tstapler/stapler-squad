# Research: Pitfalls — exposing ListBacklogItems / GetNotificationHistory / SearchClaudeHistory as MCP tools

Agent 4 (Pitfalls) for `mcp-search-list-tools`. All line references are against this worktree's
current tree (branch `tstapler/triage-66cdf9f5-8011-48bf-8dc8-8019ea207e8c_18cb91114d9312bf`); no
commit SHA was resolved since these files are uncommitted-context reads, not links into a shared
history.

## 1. Context-window blowup

**`list_sessions` precedent (`server/mcp/server.go:120-137`, handler `server/mcp/tools_discovery.go:87-142`):**
Default limit 10, max 100, enforced entirely in the *handler*, not by the schema:
```go
limit := 10
if limitF > 0 { limit = int(limitF) }
if limit > 100 { limit = 100 }
```
`search_sessions` mirrors this at default 10 / max 50 (`tools_discovery.go:189-195`).

**`ListBacklogItemsRequest` has no limit/offset/pagination field at all** —
`proto/session/v1/backlog.proto:336-353`:
```protobuf
message ListBacklogItemsRequest {
  repeated string status = 1;
  repeated int32 priority = 2;
  string sort_by = 3;
  bool include_terminal = 4;
  bool include_archived = 5;
}
```
`BacklogService.ListBacklogItems` (`server/services/backlog_service_query.go:107-147`) returns
every matching item, unbounded, straight from `storage.ListBacklogItemSummaries`. This is the
sharpest risk of the three: unlike `SearchClaudeHistory` and `GetNotificationHistory` (both capped
server-side, see below), there is **no RPC-level ceiling to lean on**. A `list_backlog_items` MCP
tool that just forwards args and returns `Items` verbatim will dump the entire backlog (title +
description + acceptance criteria + notes, per `BacklogItem` proto) into LLM context on a
default/no-filter call. The tool itself must invent a client-side `limit`/pagination or
truncation scheme from scratch — there's no proto field to surface. Given `list_sessions`' own
comment ("Default limit is 10 to avoid filling LLM context", `server.go:121`) is the explicit
precedent this feature is supposed to follow, `list_backlog_items` needs the same discipline
invented at the MCP layer, and probably field-trimming too (e.g. omit `description`/`plan` in the
default list view, matching the `buildSummaryOnly` pattern already used for
`ListBacklogItemSummaries` per the comment at `server/services/backlog_service.go:589`).

**`SearchClaudeHistory`** — capped server-side in `SearchService.SearchClaudeHistory`
(`server/services/search_service.go:501-507`): default 20, max 100, both enforced in Go, not just
documented in the proto comment (`limit = 6; // default: 20, max: 100`,
`proto/session/v1/session.proto:983-984`). Good precedent to reuse, but the MCP tool's own
`Max()`/`DefaultNumber()` schema values should match this ceiling (100), not invent a different
one — and each `SearchResult` carries `repeated SearchSnippet snippets`
(`proto/session/v1/session.proto:1000-1015`) with unbounded-length `text` per snippet; even 20
results with a handful of multi-paragraph snippets each can be a lot of tokens. Worth checking at
plan time whether `IncrementalSync`'s snippet extraction already truncates snippet length, or
whether the MCP tool needs to truncate `text` itself (open question 3 in requirements.md already
flags this).

**`GetNotificationHistory`** — capped server-side too:
`NotificationHistoryStore.List` (`server/notifications/store.go:236-280`) defaults to 50, clamps
to `MaxNotifications = 500` (`store.go:17`). 500 is still large relative to LLM context if the
handler blindly forwards a client-requested `limit=500`; the MCP tool's own schema max should be
tighter (e.g. mirror `list_sessions`' 100, not the store's 500) rather than exposing the full
store ceiling as the tool's ceiling.

**Bottom line:** `ListBacklogItems` is the one RPC with zero built-in ceiling — design a
client-side cap explicitly for it. The other two already have server-side caps; the risk there is
just picking a *tighter* MCP-level default/max than the RPC's own ceiling, consistent with how
`list_sessions`/`search_sessions` already do (their max is 100/50, well under what the backing
store could return).

## 2. Feature-flag / enablement gaps

`registerBacklogTools` is the only gated registration in `NewCore` — gated on
`storage != nil && (backlogEnabled == nil || backlogEnabled())`
(`server/mcp/server.go:63-66`), and every backlog handler calls
`featureDisabledResult(h.enabledCheck)` as its first line (e.g.
`tools_backlog.go:195`, `:341`, `:419`, `:598`, `:862`). `registerGoalTools` reuses the same
`backlogEnabled` flag. **`registerDiscoveryTools`, `registerLifecycleTools`, `registerVCSTools`
have no gate at all** — they're always registered, no `enabledCheck`. There is no
notifications-specific or search-specific feature flag anywhere in the codebase (confirmed:
`grep` across `config/` and `server/services/` for a notifications/search enablement flag turned
up nothing beyond the backlog one).

Implication for the three new tools:
- **`list_backlog_items`** belongs under `backlogHandlers`/`registerBacklogTools` and **must**
  call `featureDisabledResult(h.enabledCheck)` like every sibling backlog tool — this is exactly
  the risk the parent task flagged, and it's real: a copy-paste that skips the first line would
  silently expose backlog listing even when the backlog feature is flagged off, contradicting the
  intent of `backlogEnabled`.
- **`get_notification_history`** and **`search_claude_history`** have no analogous backend flag
  to gate against today. Acceptance criterion 5 in requirements.md states new tools should follow
  "feature-flag gate (`featureDisabledResult`)" as a blanket convention — that's true only for the
  backlog/goal family; forcing a flag check with no underlying flag onto these two would be
  inventing scope not backed by any existing config. Flag this at plan time: either (a) treat
  "feature-flag gate" as backlog-only and let these two tools follow the ungated
  `list_sessions`/`search_sessions` pattern instead, or (b) if a flag is wanted for consistency,
  it needs a real backing config value, not a dummy always-true check — don't fabricate one just
  to satisfy the letter of AC5.

## 3. Argument validation — repeated/enum fields

**`Enum()`/JSON-schema `enum` declarations are not enforced by the mcp-go framework** — confirmed
by reading `list_sessions`' own handler: the schema declares
`mcpgo.Enum("running", "paused", "ready", "loading", "needs_approval")` for `status_filter`
(`server.go:122-124`), but the handler does a bare `strings.EqualFold` comparison
(`tools_discovery.go:107-114`) with **no validation** — an invalid/misspelled `status_filter`
value just silently matches nothing and returns an empty page, not an error. Grepping the
`mark3labs/mcp-go@v0.48.0` server package for schema/argument validation confirms there's no
framework-level enforcement; `Enum()` is documentation for the LLM client only. Handlers that do
validate (e.g. `getBacklogItem`'s `validateUUID` call at `tools_backlog.go:203-205`, or
`submitReviewVerdict`'s explicit per-verdict shape check at `tools_backlog.go:631-635`) do so by
hand.

This matters more for `list_backlog_items` than for a single-string filter, because `status` and
`priority` are **repeated** fields
(`proto/session/v1/backlog.proto:337-338`,
`filter.Statuses = req.Msg.Status` at `backlog_service_query.go:120-121`). If the new tool follows
the `list_sessions` non-validating precedent, one typo'd status in a list of three silently drops
that one filter term rather than erroring — much easier to miss than a single bad string, and the
caller (an LLM) has no signal anything went wrong; it'll just look like fewer/no results without
explanation. Precedent exists in this same file for the *correct* pattern instead: `mcpgo.Items()`
with an inline `"enum"` in the item schema, used for `verdicts[].outcome`
(`tools_backlog.go:1791-1799`, `"enum": []string{"PASS", "FAIL", "PARTIAL", "UNVERIFIABLE"}`), and
the handler backs that schema hint with real validation
(`tools_backlog.go:631-635`, `errResult(ErrInvalidArgument, fmt.Sprintf("verdict[%d]: invalid
shape: %v", i, err), "")`). The new tool should follow the *verdicts* precedent (schema declares
`Items` with `enum`, handler validates and errors clearly), not the *status_filter* precedent
(schema declares `Enum`, handler silently no-ops on mismatch) — otherwise a real, hard-to-debug
argument-validation gap ships baked into the new tool.

Also note: proto `status` is `repeated string` (free-form values validated against Go constants
elsewhere, e.g. `session.BacklogStatusReady`), while `priority` is `repeated int32` with a
documented range (1–5, per `create_backlog_item`'s `mcpgo.Min(1)/Max(5)` at
`tools_backlog.go:1854-1858`) — the array-of-numbers case needs the same `Items()` range check
(`"minimum"/"maximum"` in the item schema, or a manual per-element bounds check), not just a
enum-of-strings check.

## 4. Stale dangling reference — `tools_backlog.go:204`

Current exact wording (verified by direct read):
```go
return errResult(ErrInvalidArgument, err.Error(), "Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."), nil
```
This is the only reference to `list_backlog_items` anywhere in `server/mcp/*.go` today (grepped
the whole package for `list_backlog_items`, `search_claude_history`, `get_notification_history` —
zero other hits). Resolving it is purely a matter of making sure the new tool is actually named
`list_backlog_items` (matching this hint text exactly) — if planning picks a different name (e.g.
`search_backlog_items`, `find_backlog_items`), this hint string needs a matching edit or it
becomes a *second* dangling reference, just relocated rather than fixed. Cheap to get right, easy
to silently miss if the tool-naming decision happens in a later phase than whoever edits this
line.

## 5. Error-handling consistency

Two separate error-code const blocks exist, not one:
- `server/mcp/types.go:61-72` — general codes: `ErrInvalidArgument`, `ErrInternalError`,
  `ErrSessionNotFound`, `ErrConfirmationRequired`, `ErrInvalidStatusTrans`, `ErrSessionNotRunning`,
  `ErrRateLimitExceeded`, `ErrSessionStartupTimeout`, `ErrInvalidPath`, `ErrPTYWriteTimeout`.
- `server/mcp/tools_backlog.go:60-63` — backlog-specific: `ErrPermissionDenied`,
  `ErrItemNotFound`, `ErrFeatureDisabled`.

`errResult(code, message, hint string)` (referenced throughout both files) is the single shared
helper, so the consistency risk isn't the helper itself — it's *which* code const each new tool
reaches for. `list_backlog_items` has an obvious model to copy (`get_backlog_item`'s pattern:
`ErrInvalidArgument` for bad input, `ErrInternalError` for storage failures, plus
`featureDisabledResult`'s `ErrFeatureDisabled`). `get_notification_history` and
`search_claude_history` have no existing sibling in this package to copy from — a new author might
reach for a novel code name (e.g. `ErrSearchFailed`) instead of the existing generic
`ErrInternalError`/`ErrInvalidArgument`, fragmenting the error taxonomy further. Model both on the
`ErrInvalidArgument` (bad query/filter args) + `ErrInternalError` (backend call failed) pair
already used everywhere else, rather than inventing resource-specific codes the way backlog did —
backlog's three extra codes are justified by genuinely distinct semantics (permission, not-found,
feature-off) that don't obviously apply to a read-only search/list call.

## 6. Testing gaps

Every existing tool file has a matching `_test.go` sibling:
`tools_backlog.go`↔`tools_backlog_test.go`, `tools_discovery.go`↔`tools_discovery_test.go`,
`tools_github.go`↔`tools_github_test.go`, `tools_goal.go`↔`tools_goal_test.go`,
`tools_rules.go`↔`tools_rules_test.go`, `tools_terminal.go`↔`tools_terminal_test.go`,
`tools_vcs.go`↔`tools_vcs_test.go`, `tools_workflow.go`↔`tools_workflow_test.go`. There is no
exception in the current tree — a new `tools_*.go` (or new functions added to an existing file)
without a matching test is a deviation from a 100%-consistent convention, not a judgment call, and
per this repo's CLAUDE.md/`fix-flaky-tests-dont-defer.md` discipline ("no completion claim without
proof", tests are the proof) that gap should block calling AC5 satisfied.

Concrete test-fixture risk worth flagging for planning: `discoveryHandlers` (the natural home for
`get_notification_history`/`search_claude_history` by naming convention) currently holds only
`store session.InstanceStore` (`tools_discovery.go:15-17`) — no `*services.SessionService`. Both
target RPCs are only reachable through `SessionService` (`SearchClaudeHistory` at
`server/services/session_service.go:3039`, delegating to `s.searchSvc`; `GetNotificationHistory`
at `session_service.go:3280`, delegating to `s.notificationSvc`). Adding a `svc` field to
`discoveryHandlers` is a **nil-pointer trap already documented once in this exact file**: `NewCore`
calls `registerDiscoveryTools(s, &discoveryHandlers{store: store})`
unconditionally at `server.go:41` — unlike `registerWorkflowTools`/`registerRulesTools`, which are
explicitly gated `if svc != nil` (`server.go:59-62`) specifically because those handlers need
`svc`. `server.go:43-51`'s comment on the `liveFinder` variable spells out the exact failure mode
this would reproduce if skipped: wrapping a nil `*services.SessionService` in a non-nil interface
value makes `h.svc != nil` checks lie, and a method call on it panics on the nil receiver. If the
new tools go into `discoveryHandlers`, either (a) `registerDiscoveryTools`'s call site needs the
same `if svc != nil` gating `registerWorkflowTools` uses (meaning `list_sessions`/`search_sessions`
would need to either move to their own always-on registration or accept being gated behind `svc`
too — a real design fork to resolve at plan time), or (b) the two new handlers need an explicit
nil-`svc` guard mirroring `liveFinder`'s, not a bare field access. This is a concrete,
precedent-documented landmine, not a hypothetical.

Separately: `SearchClaudeHistory`'s handler calls `getOrRefreshHistoryCache` +
`searchEngine.IncrementalSync` (`search_service.go:474-495`) — a real filesystem/index read, not a
pure in-memory lookup like `list_sessions`. A naive MCP-layer test that spins up a real
`SessionService` without seeding fixture Claude-history files risks either an empty/trivial index
(test passes but proves little) or a slow/flaky test if it scans a real `~/.claude` directory
depending on how `SessionService`/`SearchService` resolve the history path in test mode. Check
`server/services/search_service_test.go` for whatever fixture/mock pattern it already uses before
inventing a new one for the MCP-layer test — reuse it rather than re-deriving history-directory
mocking from scratch.

## 7. Existing pagination/filtering fragility signals in current tests

`tools_discovery_test.go` has `TestListSessionsCursorPagination` (`:104`) and
`TestCursorPaginationComplete` (`:289`) — both exist specifically because cursor pagination for
`list_sessions` was non-trivial to get right (cursor encodes `LastTitle`+`CreatedAt`, is looked up
by linear scan for `inst.Title == cursor.LastTitle` at `tools_discovery.go:126-130` — fragile if
two sessions ever share a title, though session titles are presumably unique by construction).
`list_backlog_items` has no obvious unique, stable sort key documented in the same way (no `title`
uniqueness guarantee stated in `backlog.proto`); if the new tool invents its own cursor/limit
scheme (per the context-blowup section above), it should pick a genuinely unique, stable field
(e.g. item ID) as the cursor anchor rather than copying `list_sessions`' title-based approach
verbatim, or the same fragility (silently wrong pagination on a title/anchor collision) transfers
over. No existing backlog-list pagination test exists to model against — this would be new test
surface, not an existing pattern to imitate, since `ListBacklogItemsRequest` has no pagination at
the proto level (see §1) — the MCP tool would be the first place backlog pagination exists at all
in this codebase.

## Summary of concrete, actionable risks for the plan phase

1. `ListBacklogItems` has **no proto-level limit/offset** — `list_backlog_items` must invent a
   client-side cap + (if paginated) a cursor scheme from scratch; can't copy-paste
   `list_sessions`' "just clamp `limit`" because the RPC itself doesn't do pagination.
2. `list_backlog_items` must call `featureDisabledResult(h.enabledCheck)` (real risk: silently
   skippable). The other two tools have **no existing flag to gate on** — don't invent one just to
   satisfy AC5's letter.
3. Repeated `status`/`priority` args need real validation + clear `errResult(ErrInvalidArgument,
   ...)` errors, modeled on `verdicts[].outcome`'s `Items()`+enum pattern — not on
   `status_filter`'s silently-permissive precedent.
4. `tools_backlog.go:204`'s hint string only self-resolves if the tool is literally named
   `list_backlog_items` — confirm the name before treating AC4 as done.
5. Reuse `ErrInvalidArgument`/`ErrInternalError` for the two non-backlog tools rather than minting
   new resource-specific error codes.
6. `discoveryHandlers` lacks a `svc` field; adding one reproduces the exact nil-interface trap this
   file already has a comment warning about (`server.go:43-51`) unless `registerDiscoveryTools`'s
   registration is gated or the handler nil-checks `svc` explicitly.
7. Every sibling tool file has a matching `_test.go` — no exceptions in the current tree; a
   missing test for any of the three new tools is a broken convention, not a gap to defer.
