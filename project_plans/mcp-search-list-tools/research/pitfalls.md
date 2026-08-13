# Research: Pitfalls — MCP search/list tool exposure

Agent 4 (Pitfalls). Scope: wrapping `GetNotificationHistory`, `ListBacklogItems`,
`SearchClaudeHistory` as new MCP tools in `server/mcp/*.go`.

## 1. Unbounded result sizes — context-blowup risk per RPC

Checked each backend RPC's own limit/pagination behavior (not the future MCP wrapper's):

| RPC | Backend default limit | Backend max cap | MCP wrapper must add its own limit? |
|---|---|---|---|
| `SearchClaudeHistory` (`server/services/search_service.go:502-508`) | 20 (`limit <= 0` → 20) | 100 (clamped) | No — safe as-is, mirror it into the tool schema like `search_sessions` does |
| `GetNotificationHistory` (`server/notifications/store.go:258-264`) | 50 (`opts.Limit <= 0` → 50) | `MaxNotifications` = 500 (`server/notifications/store.go:17`) | Recommend clamping the MCP tool's own max lower (e.g. 50) — 500 full `NotificationHistoryRecord`s (title+message+metadata each) is still a lot of LLM context even though the store won't return more than that |
| `ListBacklogItems` (`server/services/backlog_service_query.go:107-142`) | **None** — the handler never sets `filter.Limit`, so it's 0 | **1000**, but only via a DB-layer `defaultSafetyLimit` in `session/ent_repository_backlog.go:479-484`, invisible to the RPC caller | **Yes, required** — `ListBacklogItemsRequest` (`proto/session/v1/backlog.proto:336-352`) has no `limit`/`offset`/`page_size` field at all. A caller cannot request fewer than "up to 1000" items short of `status`/`priority` filtering. Each `BacklogItem` proto includes `description`, `notes`, acceptance criteria, and session links — 1000 of those would be a severe context blowout. |

**Action for planning**: the `list_backlog_items` MCP tool needs an MCP-level `limit`
parameter (default ~10, per the `list_sessions` convention referenced in
`project_plans/stapler-squad-mcp-server/decisions/ADR-004-tool-surface-design.md:33,42`
— "default limit 10 ... prevents context bloat") applied **client-side after** calling
`ListBacklogItems`, since the RPC itself won't do it. Also consider slimming the returned
fields the way `get_backlog_item`'s handler builds a truncated markdown summary
(`session.SanitizeForAgentContext`, `server/mcp/tools_backlog.go`) rather than dumping the
full proto — a list of raw `BacklogItem` protos (with `description`/`notes`) at even 10-20
items could still be large.

## 2. Test coverage convention for new MCP tools

No single "one test per tool" mandate, but a well-established pattern to follow, drawn from
`server/mcp/tools_backlog_test.go` and `feature_flag_test.go`:

- Per handler: one `Test<Handler>_<Scenario>` per behavior — happy path
  (`TestGetBacklogItem_ReturnsItemWithEnvelope`), not-found
  (`TestGetBacklogItem_ReturnsNotFoundError`), invalid-input validation
  (`TestReportProgress_ValidatesStatusValues`), and an "invalid enum/out-of-range value is
  ignored/rejected cleanly" case (`TestSubmitTriageResult_IgnoresInvalidPriorityAndCategory_When_OutOfRange`)
  — directly relevant precedent for pitfall #4 below.
- If the tool is gated by a feature flag, add it to the `TestBacklogHandlers_FeatureDisabled`
  table in `feature_flag_test.go` (only applies if the new tool is registered under the
  `backlogEnabled` gate — see #5).
- **Two count-based tests will interact with new tool registration, but differently:**
  - `server/mcp/server_test.go`'s `TestToolRegistrationCount` hardcodes `16` and only scans
    `server.go`, `tools_discovery.go`, `tools_lifecycle.go`, `tools_terminal.go`,
    `tools_vcs.go`. **If new tools are registered in one of those five files (e.g.
    `tools_discovery.go`, since `search_sessions` lives there), this hardcoded count must be
    bumped or the test fails.** Putting new tools in a new file (`tools_notifications.go`,
    `tools_search.go`, or reusing `tools_backlog.go` for `list_backlog_items`) sidesteps this
    specific test, but not silently — `tools_backlog.go` isn't in that scanned list either,
    so it's excluded either way.
  - `server/mcp/server_integration_test.go`'s `expectedToolCount` (build-tag `integration`)
    is self-updating — it calls `NewCore(...).ListTools()` and compares the live subprocess
    count against that, so it needs no manual update. Prefer relying on this one; the
    hardcoded one in `server_test.go` is the one to watch for staleness.
- Integration test (`TestMCPHandshakeSubprocess`, `integration` build tag) builds the whole
  binary and does a real stdio JSON-RPC handshake — no per-tool integration test is required
  beyond what that single test already covers (tool count + list shape), but see the CI
  timeout note in #5.

## 3. `docs/registry/` — MCP tools are NOT part of that registry

Grepped `server/mcp/*.go` for `// +api:` and `// +feature:` markers: **zero matches**. None
of the existing 55 MCP tool registrations carry a registry marker, and
`docs/registry/features/backend/` (49 files) has no MCP-specific entries. The registry
(`.claude/rules/feature-registry.md`) tracks ConnectRPC handlers and React components, not
MCP tool wrappers around them.

**Conclusion**: no new `docs/registry/features/*.json` file is needed for the new MCP tools
themselves. The underlying RPCs already carry their own markers where present —
`// +api: backlog:list-items` (`server/services/backlog_service_query.go:104`) and
`// +api: history:search` (`server/services/session_service.go:3037`) — those markers stay
on the RPC handler, not the MCP wrapper, and don't need duplicating. (`GetNotificationHistory`
has no `+api:` marker at all currently — pre-existing gap, out of scope for this task per the
registry rule's own scope: it governs *new/modified* features, and the RPC itself isn't being
modified here.)

## 4. Enum/filter value mismatch risk

- **`ListBacklogItems.status`** (`repeated string status = 1` — proto.go type is a plain
  `string`, not a proto enum) flows straight into `session.BacklogItemFilter.Statuses` and
  then `backlogitem.StatusIn(filter.Statuses...)` (`session/ent_repository_backlog.go:462-466`)
  — an ent-generated SQL `IN` predicate. **An invalid/misspelled status string does not
  error** — it silently matches zero rows (empty result, not a tool error). This is a real
  trap for an LLM caller: passing `"in_progress"` vs the actual value (need to confirm exact
  string constants, e.g. `session.BacklogStatusInProgress`) with a typo returns an empty list
  with no indication anything was wrong. **Recommend the MCP tool validate `status`/`priority`
  values against the known `session.BacklogStatus*` constants before calling the RPC** and
  return `ErrInvalidArgument` with the valid set listed, rather than passing through and
  getting a silent empty result — mirrors `TestReportProgress_ValidatesStatusValues`'s
  existing precedent for validating status strings in this same package.
- **`GetNotificationHistory.type_filter`** (`optional NotificationType type_filter = 3`) *is*
  a real proto enum. Converting an MCP string argument to this enum value requires an explicit
  string→int32 mapping in the new handler (no such conversion is needed for `status_filter` in
  `list_sessions`, which stays a plain string compared via `strings.EqualFold` against
  `session.Status.String()` — not a proto enum at all). Get the mapping wrong or leave it
  unvalidated and an invalid string silently resolves to the enum zero value
  (`NOTIFICATION_TYPE_UNSPECIFIED`) rather than erroring, which — depending on what
  `NotificationHistoryStore.List`'s `TypeFilter` comparison does with that zero value — could
  either return everything or nothing, with no error surfaced either way. **Validate the
  enum string against `mcpgo.Enum(...)` in the tool schema (as `list_sessions.status_filter`
  already does) AND explicitly reject/error on an unrecognized value in the handler**, not
  just silently default it.
- **`ListBacklogItems.priority`** is `repeated int32`, not an enum — no valid/invalid string
  mapping risk, but out-of-range ints (negative, huge) aren't validated either; same "silently
  returns empty" risk as status, lower severity since there's no typo-prone string involved.

## 5. Prior related findings

- **`project_plans/stapler-squad-mcp-server/decisions/ADR-004-tool-surface-design.md`** is
  the origin of the "default limit 10" convention this task explicitly extends — cite it
  directly rather than re-deriving the rationale.
- **`docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md`**:
  `.github/workflows/mcp-integration.yml` runs `go test -tags=integration -timeout=150s -race
  ./server/mcp/... ./server/services/...` and this budget is **already borderline/sometimes
  exceeded** independent of any single PR's changes. `TestMCPHandshakeSubprocess`
  (`server_integration_test.go`) builds the full binary per run — adding tools doesn't add a
  new subprocess build, but any *new* `integration`-tagged test in this package adds to an
  already-tight shared budget. Worth flagging if the new tools' tests add their own
  `integration`-tagged subprocess tests rather than reusing the existing handshake test.
- **`server/mcp/rate_limiter.go`** and **`feature_flag_test.go`**: rate limiting is
  **opt-in per tool**, not automatic. Only `write_to_session` (`writeLim`, a `tokenBucket`)
  and `create_session` (`createSessionLimiter`, referenced in the file's own doc comment) are
  rate-limited today; `search_sessions`/`list_sessions`/`get_session` in
  `registerDiscoveryTools` carry **no** rate limiting at all. Since the three new tools are
  read-only (list/search), following the `registerDiscoveryTools` precedent (no rate limiting)
  rather than the mutation-tool precedent is correct — no new limiter needed unless the new
  tools prove expensive enough to warrant one (e.g. `SearchClaudeHistory`'s FTS index sync is
  the most likely candidate for that, since `search_service.go:479-489` does an
  `IncrementalSync` on every call).
- Feature flag gating (`backlogEnabled`) only applies to tools built with
  `registerBacklogTools`/`registerGoalTools` — i.e. it should apply to the new
  `list_backlog_items` tool (same feature domain as `get_backlog_item`), but **not** to
  `get_notification_history` or `search_claude_history`, which have no corresponding flag
  today (`registerDiscoveryTools`, `registerTerminalTools`, `registerVCSTools` are all
  registered unconditionally in `NewCore`). Registering `list_backlog_items` outside the
  `backlogHandlers`/flag gate would be an inconsistency worth flagging in review.

## 6. Context / auth / workspace-scoping concerns

Traced `search_sessions`'s handler (`discoveryHandlers.searchSessions`,
`server/mcp/tools_discovery.go`) — it calls `d.store.LoadInstances()` directly with no
identity/workspace check on `ctx` at all; the only contextual value MCP tools thread through
this package is the injected `STAPLER_SESSION_UUID` (`WithSessionUUID`/`callerSessionUUID` in
`tools_backlog.go:26-45`), used purely for backlog-item *attribution* (which session reported
progress/requested review), not for authorization or workspace scoping.

Confirmed the same is true one layer down: `SessionService.SearchClaudeHistory`,
`SessionService.GetNotificationHistory` (`server/services/session_service.go:3037-3043,
3278-3283`) are pure single-line delegations to `searchSvc`/`notificationSvc` with no
ctx-based auth/scope check, and `BacklogService.ListBacklogItems`
(`server/services/backlog_service_query.go:107`) is the same shape. There is **no HTTP
middleware-only auth/scoping logic being bypassed** by calling these service methods directly
from MCP instead of through the ConnectRPC HTTP path — `ctx` is passed through unmodified in
both paths, and neither path does anything scope-relevant with it. This process is
single-tenant (one `Storage`/`NotificationHistoryStore` instance per server process, not
multi-workspace-scoped per request), so **there is no required scope check for the new tools
to accidentally skip** — the existing pattern of "call the service method directly with
`ctx` passed through" is safe to replicate for all three new tools. Only carry forward
`STAPLER_SESSION_UUID` context injection where a tool needs to attribute the call to a
specific session (as `get_backlog_item`/`report_progress` do) — none of the three new
read-only tools appear to need that, since they're queries, not attributed writes.
