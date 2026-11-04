# Stack research: attention-inbox-consolidation

Paths are relative to the repo root, against worktree HEAD `9c6471773`. Labels: VERIFIED = opened or grepped at the cited line; INFERRED = reasoned, not run. Nothing here was executed at runtime.

## 1. Stack and versions (VERIFIED)
- Go: `connectrpc.com/connect v1.20.0`, `entgo.io/ent v0.14.5` (`go.mod:17,19`).
- Web: `@connectrpc/connect(-web) ^2.1.1`, `react ^19`, `next 15.3.2`, `@vanilla-extract/css 1.21.3` and `recipes ^0.5.7` (`web-app/package.json:72-73,96,111-112,149`). Bun only. Navigation goes through `@/lib/nav`, and every new `page.tsx` needs a `route-meta.ts` row.
- Ent: generated code is gitignored. Regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`. Commit only `session/ent/schema/*.go`.
- Existing event-style ent tables to copy: `error_event.go`, `escape_event.go`, `compaction_event.go`, `backlog_status_event.go`, `stage_transition.go`, `trigger_fire_event.go`. Field shape for `error_event` is at `session/ent/schema/error_event.go:13-24`.
- New RPC checklist: proto in `proto/session/v1/`, `make proto-gen`, handler in `server/services/`, `make registry-generate`, route tables, sinkguard read/write tag.

## 2. Existing stores and logs
- **Notification history** (VERIFIED): `server/notifications/store.go`, `NotificationHistoryStore`. It is a JSON file (`loadFromDisk`/`saveToDisk`, lines 781/809), not ent.
  - `NotificationRecord` is at `store.go:74-96`: `SessionID`, `NotificationType`, `Priority`, `Metadata`, `IsRead`, and dedup via `OccurrenceCount`/`LastOccurredAt`.
  - It already has a retention cap (`enforceRetention`, line 744), orphan pruning, and dedup (`findDuplicate`, `mergeOccurrence`).
  - It records only what was *notified*. Suppressed or never-emitted states (the actual gap) are not in it.
  - MCP `get_notification_history` is in `server/mcp/tools_notifications.go`.
  - Implication: a durable event log of state transitions, including suppressed ones, needs a new ent table (the precedent above). The JSON store is a poor fit for per-session queries and cross-restart range scans.
- **Activity-log precedent**: `project_plans/backlog-item-activity-log/` (requirements, `research/stack.md`, ADR-001 sibling table, ADR-002 single-entry event payload). It is for backlog items, not sessions. Reuse its pattern: append-only sibling ent table, one-event payload, repository `Append`/`List`. Do not reuse `BacklogProgressNote`, which has no session provenance and an item-level focus.
- **Event-driven poller plan**: `project_plans/review-queue-event-driven/` exists. Requirements NFR says to prefer it over new polling.

## 3. Hook and transcript signal for "turn ended / ends with a question"
- **Stop hook is wired but discarded** (VERIFIED). `HookStopNotification` maps to the Claude `Stop` event at `server/services/hook_injector.go:24,53,118`, routed to `/api/hooks/stop`. `HandleStop` (`server/services/hook_receivers.go:65-70`) only logs `[hook/stop]` and the body size, then returns 200. The body is drained and not parsed.
  - Claude's Stop payload carries `transcript_path` and `stop_hook_active`. `HookEventName` and `transcript_path` appear in the payload struct in `cmd/ssq-hooks/main.go` (grep hit; I did not open the struct).
  - Implication: a turn-ended signal is available for Claude sessions at near-zero cost. Parse the body, store the event, and read the last assistant message from the `transcript_path` JSONL tail.
  - `last_assistant_message` is not confirmed in the repo and is UNVERIFIED. Treat the transcript JSONL as the source.
- Other hooks: PermissionRequest, PreToolUse, PostToolUse, UserPromptSubmit, and a Notification matcher in `session/mux/hooks.go:88`. Hooks are injected per session, so coverage depends on injection (`hook_injector.go`). Whether manually created sessions get Stop injected is UNVERIFIED.
- **Transcript JSONL ingestion** exists: `session/history_watcher.go` (fsnotify on `~/.claude/projects/*.jsonl`) and `session/claude_adapter.go`. It indexes history and is not a turn-end detector. Wiring it as one is new work.
- **Non-Claude programs** (aider, agy, pi): Claude hooks do not apply. `cmd/ssq-hooks/main.go` has Gemini/Antigravity/OpenCode decision writers (`:135-141`), so some other programs have hook channels. Others will need the screen-detector fallback. Per-program coverage is a stated feasibility risk.
- Prose-question classification still needs a heuristic or classifier on the last message text (`pkg/classifier/classifier.go` exists; scope not checked). A Stop hook gives "turn ended" reliably, and does not by itself give "is a question".

## 4. `updatedAt`: production, serialization, and the reload bug

Production (VERIFIED):
- `touchUpdatedAt()` (`session/instance_state.go:36-38`) is `i.UpdatedAt = time.Now()`. It exists so the web reducer does not drop an update whose `updatedAt` is unchanged (comment at `instance_state.go:32-35`).
- Callers:
  - `instance_state.go:53` (`transitionTo`, every status change)
  - `instance_state.go:103` (`ForceStatus` path, via the loadStatus/touch block)
  - `instance_state.go:538,571`
  - `instance_actor_setters.go:499` (pin)
  - `instance_actor_setters.go:847,880,964` (creation-failure, retry, GitHub resolution)
  - `instance_hibernate.go:166`
- Direct writes: `instance_actor_setters.go:616` (note), `instance_terminal.go:116` (this is `Rename`, not terminal I/O), `instance.go:2700` (`Restart`), `instance_workspace.go:368` (change dir).
- So `UpdatedAt` is a "dirty stamp" for UI cache invalidation, bumped by pin, note, rename, and every status transition, including operational ones. It is not "last meaningful change" or age.

Serialization (VERIFIED):
- `session/instance_serialization.go:110` builds `InstanceData{... UpdatedAt: time.Now() ...}` inside `ToInstanceData()`. It ignores `snap.UpdatedAt`. Every persist stamps the DB with the save time.
- Persist callers are `storage.go:375` (`SaveInstances`, all started instances), `:624` (`AddInstance`) and `:672` (`UpdateInstance`).
- `ent_repository.go:515` writes it with `SetUpdatedAt(data.UpdatedAt)`. Load restores it at `instance_serialization.go:303`.
- The partial updates `UpdateReviewQueueState`, `UpdateTimestamps`, `UpdateLastAddedToQueue`, `UpdateInitialPromptSentAt`, `UpdateLastAcknowledged`, `UpdateLastViewed` and `UpdateSessionMetadata` (`ent_repository.go:1129,1163,1191,1209,1226,1243,1268`) also set `updated_at = time.Now()` in the DB. The ent schema has `UpdateDefault(time.Now)` on the column (`session/ent/schema/session.go:43-45`).
- `UpdateLastViewed` and `UpdateTimestamps` fire on routine activity. `MarkViewed()` is called when a terminal websocket opens (`server/services/connectrpc_websocket.go:1320,2112`, `connectrpc_websocket_shell.go:420`), which is what a browser reload does.

Wire and web (VERIFIED):
- `server/adapters/instance_adapter.go:83` sends `snap.UpdatedAt` (the in-memory value).
- `ListSessions` reads live in-memory instances from the poller, not the DB (`server/services/session_service_crud.go:71-76`).
- `SessionCard.tsx:1272` renders `formatTimeAgo(session.updatedAt)` (formatter defined at `:485-490`). The formatter itself is stateless: `now - ts`.
- The store keeps the existing entity when `updatedAt` seconds and nanos match (`web-app/src/lib/store/sessionsSlice.ts:60-98`).

Root-cause hypotheses (not reproduced; no runtime check was run):
1. VERIFIED mechanism, INFERRED as the cause of the reported symptom. `UpdatedAt` is not a stable, semantic timestamp. The persisted value is "last time any save ran" (`instance_serialization.go:110`), and the DB column is also bumped by `UpdateLastViewed`/`UpdateTimestamps`. After a server restart, `FromInstanceData` restores that save time (`:303`), so the card shows roughly "time since last save/restart" rather than anything about the session. This matches "resets" after a restart or redeploy (`make install-service`).
2. INFERRED. For a pure browser reload with no server restart, the in-memory `UpdatedAt` is served unchanged, so the reload alone should not change it. A change would need something on reload to hit one of the bumpers (e.g. `MarkViewed` followed by a status transition or a `Restart`/`changeDir`) or a differing source of truth. I did not find a reload-specific path that writes `Instance.UpdatedAt`. The operator's observation may really be restart-driven, or there may be a transition on attach. This needs a repro on an isolated instance.
3. Requirements hypothesis check: `instance_terminal.go:116` is `Rename`, which is semantic but rare. It is not a reload trigger. The requirements doc's pointer to it as a non-semantic touch is wrong on that point.
- Fix direction (INFERRED): add a separate, persisted `LastStateChangeAt` (or reuse `LastMeaningfulOutput`) set only by semantic transitions, and keep `UpdatedAt` as the cache-invalidation stamp. Stop `ToInstanceData` from overwriting it with `time.Now()` (use `snap.UpdatedAt`). Audit sort and stale consumers first: `useFilteredGroupedSessions.ts:147` sorts by `updatedAt`, `useBackgroundSessions.ts:64` and `backgroundActivity.ts:153,257` use it, and `transition_gate.go:462` uses `ExpectedUpdatedAt` as an optimistic-concurrency token, so changing `UpdatedAt` semantics is risky.

## 5. In-repo patterns to follow
- Lock-free reads: add any new field to `InstanceSnapshot` (`session/instance_snapshot.go`) and read via `Snapshot()`. Compare sessions with `Workspace().ActiveDir`.
- No revival: any surfacing path must not call `Start(false)`, `RecoverFromStopped` or similar on archived sessions (`noarchivedrevival`). The new Stop-hook handler lives in `server/services`, which is outside the analyzer's file set, but the rule still applies in spirit.
- Rollout flags: live-settable via the feature-flag RPC (`attention_prose_question_detection`, `session_event_log`). No env vars.
- Delivery: events must go through the hidden-session delivery gate and `smart-notification-dedup` (`server/notifications/question_dedup.go`, `approval_group.go`, `server/deliverygate/`) so the log and tray do not double-count.
- UI: vanilla-extract `*.css.ts` files beside components; new routes need `route-meta.ts` rows and `FeatureGuard`; e2e specs need the `// @feature` header and `data-testid` or ARIA locators; mobile and desktop both.
- Existing pages for consolidation: `web-app/src/app/notifications/`, `web-app/src/app/unfinished/` (Up Next), review-queue page; `ReviewQueuePanel`.

## 6. Open items for later phases
- Reproduce the reload symptom on an isolated instance (record `updatedAt` before and after reload, with and without restart) before changing semantics.
- Confirm Stop-hook injection coverage for all Claude session types, and the Stop payload fields (`transcript_path`, `stop_hook_active`) against the real payload in `cmd/ssq-hooks/main.go`.
- Decide ent table vs. extending the JSON notification store (recommendation above: new ent table).
