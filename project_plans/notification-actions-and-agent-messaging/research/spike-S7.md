# Spike S7: verify code state the plan assumes

**Date**: 2026-10-18 | **Worktree**: `nam-spikeB` at `497016cde` | Live probes ran on an isolated instance (`PORT=62873 --remote-port 62874`, `STAPLER_SQUAD_INSTANCE=nam-spikeB`, tmux socket `ssq-manual-2`, Claude Code 2.1.303, model haiku). `:62871/:62872` were occupied by a stale manual instance (pid 2221147, started 15:06, not mine), so the next free block was used; the live `:8543` service was not touched.

| # | Item | Result |
|---|---|---|
| 1 | ADR-010 Reply implemented | **PASS** |
| 2 | Client-event RPC / sink for Story 1.5 | **Sink chosen: `POST /api/analytics`** (REST, not an RPC) |
| 3 | `EventBus` types for `session_state` | **PASS** (`session.updated` + `UpdatedFields`) |
| 4 | History record proto + gate `typeClass` | **PASS** |
| 5 | `GetFeatureFlagScoped` has no production caller | **PASS** (confirmed) |
| 6 | Injected PostToolUse matcher fires for `AskUserQuestion` | **PASS** (live), with two caveats |
| 7 | MCP header echo (Task 0.7c) | **PASS** (Claude Code redacts it) |
| 8 | Background outcome evidence | **0 rows would show `finished` at launch** |
| 9 | Claude Code version source | transcript `version` field and `claude --version`; hook payload UNVERIFIED |
| 10 | Human-input path | **PASS**, with a finding about ssq's own pty client |
| 11 | jscpd + flaky baseline | **0.14% displayed (0.1377% exact), margin 24 duplicated lines** |

## (1) ADR-010 Reply is implemented

```
$ git grep -n 'rpc ReplyToPendingQuestion' -- proto
proto/session/v1/session.proto:173:  rpc ReplyToPendingQuestion(ReplyToPendingQuestionRequest) returns (ReplyToPendingQuestionResponse) {}
$ git grep -n 'func (s \*SessionService) ReplyToPendingQuestion' -- server
server/services/hidden_question_reply.go:285:func (s *SessionService) ReplyToPendingQuestion(
$ sed -n 4p project_plans/notification-tray-and-hidden-session-gate/decisions/ADR-010-audited-reply-to-pending-question.md
**Date**: 2026-10-07 (repaired 2026-10-09; implemented 2026-10-10, see plan.md "PR 1e / PR 5r implementation record")
```
The flag controller `replyFlagController` is at `hidden_question_reply.go:85` (Story 0.8's model). Note the ADR's own `Status:` line still says "Proposed" while the Date line says implemented; the code is real.

## (2) The real client-event handler (Story 1.5 depends on this)

There are two candidates; neither is an RPC that takes arbitrary named events.

- `LogClientEvents` RPC (`proto/session/v1/session.proto:491`, handler `server/services/session_service_delegates.go:965`): request is `repeated ClientLogEntry{level, message, timestamp, url, user_agent, session_id}`; it only writes browser **console log lines** to slog (`logClientEntry`). No counters, no histograms, not named events. **Not usable as a metric sink.**
- **`POST /api/analytics`** (`server/handlers/analytics_handler.go:105`, `AnalyticsHandler.HandlePost`; client `web-app/src/lib/analytics/HttpAnalyticsProvider.ts:61`): body `{"events":[{id?, name, category, duration_ms?, session_id?, page?, component?, labels{string:string}}]}`, max 100 events and 512 KB per request, 1000 requests/minute limiter, `name` any non-empty string (newlines stripped), `category` **must be one of `user_action | performance | navigation | rpc`** (else 400). Recorded **server-side as one `AnalyticsEvent` row per event** by `analytics.SQLiteAnalyticsProvider.Record` (`server/analytics/sqlite_provider.go:27`, labels stored as JSON). **There are no OTel/Prometheus counters or histograms on this path.** `GET /api/analytics/summary` aggregates rows into top events, p50/p95/p99 for `rpc`-category durations and page views.

Live proof on the isolated instance:
```
$ curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:62873/api/analytics -d '{"events":[{"name":"notification_action_invoked","category":"user_action","labels":{"kind":"APPROVE"},"duration_ms":12}]}'
204
$ ... '{"events":[{"name":"x","category":"custom"}]}'
invalid event category: must be one of user_action, performance, navigation, rpc   (400)
$ curl -s http://127.0.0.1:62873/api/analytics/summary   # top_events now lists "notification_action_invoked"
```
The same table also holds backend-recorded events (`session.created`, `session.status_changed`, `session.deleted` appeared in the summary of this fresh instance; the recorder is presumably `server/analytics/subscriber.go`, not read).

**Decision for Story 1.5**: client-originated UI events use `POST /api/analytics` with `category="user_action"` and the event name + labels carrying the dimension (arbitrary names are accepted). Counters and histograms that need alerting (the plan's gate/feed metrics) must **not** assume an OTel instrument here: server-side counters go through `deliverygate.Metrics().Add(...)` (`server/services/hidden_question_reply.go:557-561`, surfaced at `/actuator`), and anything wanting a histogram must query `AnalyticsEvent` rows or add an instrument explicitly. Task 1.5a starts from this corrected sink.

## (3) `EventBus` types for `session_state`

`pkg/events/types.go`: no `session_state` type. Use `EventSessionUpdated` ("session.updated") with `Event.UpdatedFields` containing `events.FieldStatus` ("status"), plus `OldStatus`/`NewStatus`, `DetectedStatusTyped`, `DetectedContext`; also `EventSessionArchived` ("session.archived"), `EventSessionDeleted`, `EventNotification` ("session.notification", carries `NotificationID/Type/Priority/Title/Message/Metadata`), `EventApprovalResponse`. Full list in `spike-S4.md` section 1.

## (4) History record proto and gate `typeClass` table

- History record: **`NotificationHistoryRecord`** (`proto/session/v1/session.proto:1772`): `id, session_id, session_name, notification_type, priority, title, message, metadata(map), created_at, is_read, read_at, occurrence_count, last_occurred_at, is_pending_decision`. RPCs: `GetNotificationHistory`, `ClearNotificationHistory`.
- `typeClass` (`server/deliverygate/policy.go:131`), 15 entries: `ClassNeedsHuman` = APPROVAL_NEEDED, INPUT_REQUIRED, CONFIRMATION_NEEDED; `ClassFailure` = ERROR, FAILURE; `ClassRoutine` = UNSPECIFIED, TASK_COMPLETE, PROCESS_STARTED, PROCESS_FINISHED, WARNING (advisory unless `HintFailure`), INFO, DEBUG, STATUS_CHANGE, AUTO_APPROVED, CUSTOM.

## (5) `GetFeatureFlagScoped` production callers

```
$ git grep -n 'GetFeatureFlagScoped(' -- '*.go' | grep -v _test.go
config/config.go:1925:func (c *Config) GetFeatureFlagScoped(name, scope string, defaultValue bool) bool {
$ git grep -c 'GetFeatureFlagScoped(' -- '*_test.go'
config/feature_flag_scopes_test.go:4
```
Only the definition; the only callers are 4 test lines. `GetFeatureFlagScopedOverride` (used by `feature_flag_mutation.go:64,144,156,218`) is the production-side reader. Story 0.8 adds the first production caller. No note needed.

## (6) Injected PostToolUse matcher vs `AskUserQuestion` (live)

Static: `hookMatcherGroup.Matcher` is `json:"matcher,omitempty"` and `prependHookEntry` never sets it (`server/services/approval_handler.go:992`, `hook_injector.go:530`), so injected PostToolUse groups have **no matcher** (verified in the generated `.claude/settings.local.json`: `PostToolUse matcher= None` for both `post-tool-use` and `post-tool-use-drift-check`).

Live (Claude Code 2.1.303, `--debug-file`): after answering a real `AskUserQuestion` dialog,
```
05:32:43.244Z [DEBUG] Hooks: PostToolUse:AskUserQuestion [F='.../hook-proofs/'"$STAPLER_SESSION_UUID"; ...   (x2: drift-check + post-tool-use)
server log: {"time":"2026-10-18T22:32:43.185…","msg":"[hook/post-tool-use]","session":"spikeB-askq2","tool":"","bytes":1141}
            {"time":"2026-10-18T22:32:44.567…","msg":"[hook/stop]","bytes":660}
```
and `PostToolUse:ScheduleWakeup` fired the same way (`05:33:04.433Z`, Task 5.1e's tool). **PASS: the generated matcher fires for `AskUserQuestion` and `ScheduleWakeup`; Story 3.4e's `acknowledged` can derive from the hook receipt** and Task 3.4e does not need to add a matcher.

Caveats (both change tasks):
1. **Receipt carries no tool name**: `HandlePostToolUse` logs `tool` from the `X-CS-Tool-Name` header, which the generated command does not set, so the log shows `"tool":""`. The tool name is only in the 1141-byte body (`HandlePostToolUse` drains and discards it, `hook_receivers.go:70`). Task 3.4e must parse `tool_name` from the body.
2. **Hooks injected after launch do not take effect for the running Claude process**: the first run used MCP `create_session` with `hooks:[...post_tool_logging, git_drift_check]`, which injects *after* the session is Active (`server/mcp/tools_lifecycle.go:243`); across a full turn plus a Bash control call no PostToolUse hook ran (debug log has no `PostToolUse` lines, server log no `[hook/…]` receipts). After deleting and re-creating the session so Claude started with the file already in place, the hooks fired. Extra hooks therefore apply from the *next* launch; any design that relies on injecting them into a live session is wrong.
3. The generated command hard-codes `X-CS-Session-ID: <title at injection time>` (the receipt printed `spikeB-askq2` for session `spikeB-askq3`): a re-created session with a new title reuses the stale label unless re-injected. Receipts should be matched by the proof/`STAPLER_SESSION_UUID`, not that header.

## (7) Task 0.7c: does Claude echo the MCP `X-Stapler-Session-Proof` header? **PASS**

Method: local echo MCP endpoint (`127.0.0.1:62879`, Python, scratchpad), `--mcp-config` file with `headers:{"X-Stapler-Session-Proof":"SENTINEL-PROOF-7f3a9c21d4e85b60"}`, `command claude --model haiku --debug-file <log>` in tmux, `/mcp` UI opened on the `echo` server, `ping` tool called.

```
echo endpoint received (all 6 requests): x-stapler-session-proof: SENTINEL-PROOF-7f3a9c21d4e85b60   (so the header is sent)
debug log line 294: MCP server "echo": HTTP transport options: {"url":"http://127.0.0.1:62879/mcp","headers":{"User-Agent":"claude-code/2.1.303 (cli)","Accept-Encoding":"identity","X-Stapler-Session-Proof":"[REDACTED]"},...
grep -c SENTINEL-PROOF… claude-debug-mcp.log -> 0
grep -c … pane-full.txt (300-line scrollback) and the /mcp detail pane -> 0   (/mcp shows "URL: http://127.0.0.1:62879", no headers)
grep -rl 7f3a9c21 ~/.claude ~/.claude.json <session cwd> -> only this agent's own transcript (my command text); the probe session's transcript is clean
```
**PASS: the sentinel appears in no debug log, no `/mcp` UI, no pane and no `~/.claude` file** in Claude Code 2.1.303 (the header *name* is logged with a `[REDACTED]` value). Residual: whether the redaction is name-based (the name contains "Proof") or applies to all custom headers was not tested, and redaction could change in another version. Task 4.9e's redaction scope stays as written; the residual-risk paragraph should say "verified for 2.1.303 only" and the layout-drift canary's version source (item 9) can double as a re-check trigger.

## (8) Background outcome evidence (read-only; ADR-001 decision 6)

Method: copied `~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db` and its `-wal` to the scratchpad and queried the copy with `sqlite3 "file:sessions.db?mode=ro"`. The live DB was only read by `cp`.

```
status hidden archived n      -- sessions grouped
3      1      1        238    -- Stopped, hidden, archived
3      0      1        26
3      1      0        10     <- Stopped, hidden, NOT archived (what hidden_only list returns; archived excluded unless include_archived, session_service_crud.go)
...
```
The plan's "11 live stopped hidden sessions" is **10** now. All 10 are `review:<item8>` sessions. For each, the join the ADR specifies (`item_sessions.session_uuid = sessions.uuid`, i.e. `GetItemSessionBySessionUUID`) finds **no ItemSession row**:
```
sessions s JOIN item_sessions i ON i.session_uuid=s.uuid WHERE s.status=3 AND s.hidden=1 AND s.archived_at IS NULL  -> 0 rows
(joining on s.title instead -> 0 rows)
```
So role / `ended_at` / `end_reason` / verdict are all absent for the 10: **their `background_outcome` would be `unknown`, and 0 rows show `finished` at launch.** Contrast: the 238 archived hidden-stopped sessions all have an ItemSession; and for 8 of the 10 the *backlog item* (matched by id prefix) has review verdicts on other review sessions (e.g. `ec5e3618`: 6 review rows, 6 verdicts), which is why the lookup-by-session-UUID is the gap, not absence of review work.

Fallback per the ADR: rows ship as `stopped_unknown`, and Story 1.5's alert must report "`finished` is zero while stopped sessions exist" **from day one** (it is the launch state, not a hypothetical). Optional improvement for Story 1.x (not decided here): map by `(item prefix in title, role)` is unsafe (several review sessions per item); do not guess.

`end_reason` values present in `item_sessions` (2845 rows) and their classification against ADR-001 decision 6:

| end_reason | role | n | verdict rows | class |
|---|---|---|---|---|
| `` (empty) | review / work / triage / diagnose | 999 / 849 / 512 / 13 | 947 / 1 / 0 / 0 (triage_result set on 360 triage) | non-failure |
| `other` | triage / review | 215 / 2 | 0 | failure (allowlist) |
| `timeout` | triage | 190 | 0 | failure |
| `parse_error` | triage | 31 | 0 | failure |
| `pool_saturated` | triage / review | 17 / 1 | 0 | failure |
| `shutdown` | triage | 13 | 0 | non-failure (explicit) |
| `idle` | triage / review | 2 / 1 | 0 | failure |

**No value is outside the ADR's allowlist plus explicit-non-failure list.** Nothing to report as unknown.

## (9) Claude Code version source (Task 3.3h)

- `command claude --version` -> `2.1.303 (Claude Code)` (note: in this user's shell `claude` is a function that appends escape codes, so ssq must exec the binary, not the function).
- The session **transcript carries it**: every JSONL line has a `version` key (`grep -o '"version":"[0-9.]*"'` -> `7 × "2.1.303"` in the probe session's transcript); the TUI banner and API header (`cc_version=2.1.303.917`) also show it.
- Hook payload: **UNVERIFIED** (the receipt handlers drain the body without parsing; not captured). 
- Recommended source for the canary: transcript `version` (no extra process) with `claude --version` at spawn as fallback; `unknown` otherwise.

## (10) Human-input path

Browser-originated pane writes: `paneWriter.SendInput` (`server/services/terminal_access.go:140`) is called from `connectrpc_websocket.go:1439`, `:2199` and `connectrpc_websocket_shell.go:311`; `p.writer.Sender().SendInput` at `connectrpc_websocket.go:3478`. Other writers that type as the operator: the `WriteToSession` RPC / `write_to_session` MCP tool (`server/services/terminal_service.go:141`, via `session.SubmitContentWithEnter` / `SendKeysWithTimeout` under a `LeaseWriterOther` lease), which is what ssq-mux/IDE and automation use. `HumanActivity` (ADR-004) must include both.

`tmux list-clients` on the isolated instance, tmux 3.6a:
```
client-1258883 staplersquad_keepalive    ctl=1 act=1792387717      # ssq control-mode client
/dev/pts/56    staplersquad_spikeB-askq3 ctl=0 act=1792387916      # ssq's OWN pty client (pid 1377862: "tmux -L ssq-manual-2 attach-session -t staplersquad_spikeB-askq3")
# plain human `tmux attach` (helper tmux server running it):
/dev/pts/69    staplersquad_spikeB-askq3 ctl=0 act=1792388041
# after typing "z" in that client:            /dev/pts/69 ctl=0 act=1792388055   (moved; the pts/56 value did not)
```
**PASS** for the plan's claim (a human client shows `client_control_mode=0` and `client_activity` moves on keystrokes). **Finding**: ssq itself keeps a non-control pty client per session (pts/56, `ctl=0`) that is indistinguishable from a human by `client_control_mode`; `client_activity` of ssq's own client also moves when ssq writes through it. A `list-clients` human-activity signal must exclude ssq's own client (match by `client_tty`/pid of the attach process ssq spawned) or it will always read "human active".

## (11) jscpd and flaky-suite baseline (main-equivalent: this worktree is at `497016cde`, docs-only ahead of main's code)

```
$ cd web-app && pnpm run lint:duplicates      # jscpd src; rc=0
Total: 2432 files | 331101 lines | 3008539 tokens | 15 clones | 456 duplicated lines (0.14%) | 5296 tokens (0.18%)
```
Exact figure **456 / 331,101 = 0.1377%**, displayed `0.14%`. jscpd rounds to two decimals (`Math.round(1e4*cloned/total)/100`, `@jscpd/core` `calculatePercentage`) and fails when `threshold (0.14) < rounded percentage` (`@jscpd/finder` threshold reporter), i.e. at 0.15% rounded = 0.145% = **481 duplicated lines**. **Margin = 24 duplicated lines (0.0073 percentage points to the failing 0.145%)**, not the 0.10% in memory (that figure is stale: threshold was raised to 0.14% and the margin is now ~0.007%). Clone breakdown: tsx only (15 clones); json/typescript/javascript/markdown/css have none.

**Per-story duplication budget (Task 0.7d; Story 5.4 table input).** Base: 24 lines of headroom shared by every web story, plus ~1 line per 3,300 new non-duplicated lines of denominator (negligible). Allocation (default 0 for stories with no new heavily-mocked test file; the three that add them get the budget; rebase and re-run `pnpm run lint:duplicates` before each merge because the budget is shared and order-dependent):

| Web story / area | New heavily-mocked files expected | Budget (duplicated lines) |
|---|---|---|
| ActionRow + `useInvokeNotificationAction` (Epic 2 web) | 2 | 6 |
| Interactive-question card / answer UI (Epic 3 web) | 2 | 6 |
| Background activity feed (Epic 1 web) | 2 | 6 |
| Wake/peer-help UI (Epic 4 web) | 1 | 4 |
| Reserve (rebase conflicts, shared fixtures not yet extracted) | - | 2 |
| **Total** | | **24** |

Any story that exceeds its slice first extracts shared mocks into `web-app/src/components/ui/__fixtures__/` or root `__mocks__` (the plan's mitigation) rather than raising the threshold; a threshold raise is an owner decision.

**Known-flaky suites, run once, sequentially, in this worktree:**

| Suite (plan Story 5.4 list) | Command | Result |
|---|---|---|
| BUG-079 Jest: StuckItem, websocket-transport, TerminalOutput.enter-detection, BacklogItemDetail.markdown | `npx jest --no-coverage -i --testPathPatterns="StuckItem.test\|websocket-transport.test\|TerminalOutput.enter-detection.test\|BacklogItemDetail.markdown.test"` | **PASS** 4 suites, 56 tests (isolated; the bug only appears in the 354-file full run, which was **not run**: memory) |
| Jest StuckItem/WebSocket/BacklogDetail broader pattern | `npx jest --no-coverage -i --testPathPatterns="StuckItem\|WebSocket\|BacklogDetail"` | **PASS** 5 suites, 138 tests |
| BUG-090 hub reconnect, BUG-099 retry races (`server/services`) | `go test ./server/services -run 'RestartPump\|HubRegistry\|RetrySession' -count=1` | **PASS** (`ok ... 1.823s`) |
| BUG-079 AutonomousDriver idle timer (`session`) | `go test ./session -run AutonomousDriver -count=1` | **PASS** (`ok ... 3.732s`) |
| BUG-067 / BUG-099 full `server/services` race suite (~500 s) | not run | **not measured** (runtime, not a flake); attribute future timeouts there to the known bug first |

Isolated passes say nothing about full-suite-only flakes, so a future full-suite failure in these files is "known flaky" only if it also passes in isolation.
