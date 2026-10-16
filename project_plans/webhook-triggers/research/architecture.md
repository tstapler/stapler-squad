# Architecture Research: webhook-triggers

## Prior-art incorporated (cited, not re-derived)

- **`project_plans/slack-review-notifications/research/architecture.md`** — established the
  outbound-callback shape this feature reuses wholesale for FR7-FR9: derive a short, independent
  `context.WithTimeout` (~5s) so a slow/dead endpoint never blocks the caller's lifecycle path;
  never log the URL; call directly from the known-good producer call sites
  (`ReactiveQueueManager.OnItemAdded`, `server/review_queue_manager.go:327-419`) rather than a
  generic `EventBus` subscriber, because `NotificationType` doesn't reliably discriminate the
  events this feature cares about from routine churn; fire via `go rqm.slackNotifier.Notify...(...)`
  under the same guard, never propagate the error back to the caller. Every recommendation below
  for FR7/FR8/FR9 is "do what that doc already worked out, generalized from one Slack sink to N
  configurable callback URLs."
- **`project_plans/stale-session-detection/research/architecture.md`** — this project has a full,
  shipped-plan-stage analysis of "what does *stale* mean here." Four independent staleness
  thresholds already exist (`ReviewQueuePollerConfig.StalenessThreshold` 5min, `maxReworkBlockStaleness`
  15min, `maxWorkSessionStaleness` 2hr, plus a new frontend-only one that project adds), and the
  canonical Go computation to reuse for anything new is
  `Instance.GetTimeSinceLastMeaningfulOutput()` (`session/instance_approval.go:112`) — the one
  clean, exported, already-instance-scoped implementation among three duplicated ones. `on_session_stale`
  should fire off of this signal, not invent a fifth threshold/detector.

## Correction to requirements.md — a working cron scheduler already exists

requirements.md states: *"Confirmed by repo search: no `robfig/cron`/`gocron` dependency, no
webhook receiver route, no outbound callback dispatch anywhere in `server/`."* The first half of
that is **wrong** — `github.com/robfig/cron/v3 v3.0.1` is a direct dependency (`go.mod:26`) and is
already fully wired into a working, DB-backed, hot-reloadable cron scheduler:

- **`server/workflows/scheduler.go`** — `Scheduler` wraps `*cron.Cron` (5-field parser:
  `cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow`), keyed `entryMap map[string]cron.EntryID`
  per workflow ID. `Start(ctx)` loads all `CronEnabled` workflows from `session.WorkflowRepository`
  and registers them; `Reload(ctx, wf)` add/removes a single entry (called from
  `WorkflowService.CreateWorkflow`/`UpdateWorkflow` — `server/services/workflow_service.go:165,241`
  — so **enabling/disabling a workflow's cron via RPC already hot-swaps the running scheduler,
  zero restart, zero fsnotify machinery needed**); `Remove(id)` on delete.
- **`session/ent/schema/workflow.go`** — `Workflow` ent entity already has `cron_expression`,
  `cron_enabled`, `command`, `input_template`, `target_directory`, `session_type`, `model`,
  `agent_type` fields, backed by a DB table (not JSON-in-git).
- **`Scheduler.FireNow(ctx, wf, arg string)`** (`scheduler.go:127-188`) builds a prompt from
  `wf.Command`/`wf.InputTemplate` with a single `{{input}}` placeholder replaced via
  `strings.ReplaceAll`, then calls `s.sessionSvc.CreateSession(ctx, req)` through
  `SessionServiceInterface` (a narrow, consumer-defined interface — `scheduler.go:20-24`, already
  following `.claude/rules/interface-pollution-checklist.md`) with `WorkflowId: wf.ID.String()` set
  on the request — **this is FR3's "fire via the same path as manual create_session" already done**,
  and `WorkflowId` on the created session is already a working precedent for FR9's source
  attribution (a trigger-created session is visibly distinguishable from a manually-created one via
  this field).
- **Startup wiring**: `server/dependencies.go:1236-1246` constructs `workflowScheduler` and
  `WorkflowService`; `server/server.go:419-423` starts it (`deps.WorkflowScheduler.Start(serverCtx)`)
  and registers `Stop` as a shutdown hook — the exact "background loop owned by the server,
  cancelled on shutdown" idiom this repo uses everywhere (`ReactiveQueueMgr`, `HistoryLinker`,
  `UnfinishedScanner`, `WorktreePRPoller` — `server.go:140-186`).

**Implication for FR3/FR6/AC2/AC7**: the `cron` trigger type in requirements.md is not new
infrastructure to build — it is (nearly) the existing `Workflow`/`Scheduler`/`create_workflow`/
`run_workflow` feature, which the requirements' own Non-Goals section already namechecks ("existing
`create_workflow`/`run_workflow` MCP tools already give manual chaining"). Planning should treat
`cron` triggers as **"expose/extend the existing Workflow cron capability,"** not a parallel
scheduler. The only genuine gaps versus FR1-FR6 as written: (a) `Workflow.InputTemplate`'s
`{{input}}` replacement is a single-placeholder `strings.ReplaceAll`, not Go `text/template` with
structured field access (`{{issue.key}}`) — that richer templating is needed for the `webhook`
trigger type's payload interpolation (FR4) and should be added as a second render path, not
retrofitted into `FireNow`'s existing simple case unless planning decides to unify them; (b) no
match-criteria fields (repo/branch, event/label filter) exist on `Workflow` yet — needed for
`github_push`/`webhook` types, see below.

## 1. Where an inbound webhook HTTP route belongs

`server/server.go`'s `http.NewServeMux()` (`:67`) already hosts several raw (non-ConnectRPC)
`HandleFunc` routes alongside the ConnectRPC services, grouped by trust boundary:

| Existing route | Line | Trust model |
|---|---|---|
| `/api/hooks/permission-request` | `server.go:511` | Localhost-trusted (Claude Code hook process) |
| `/api/external/approvals`, `/api/external/approvals/respond` | `server.go:461-462` | External CLI/socket clients |
| `POST /api/telemetry` | `server.go:653` | Frontend-originated |

None of these verify a cryptographic signature today — confirmed via `grep -rn "hmac\." server/`:
zero hits. The `sha256.Sum256` calls that do exist (`server/auth/handlers.go:294`,
`server/services/push_service.go:159,173`, `server/tls.go:196`) are content-hashing (dedup keys,
web-push endpoint hashing, cert fingerprinting), **not** HMAC signature verification — so FR2/FR4's
"verify `X-Hub-Signature-256`" / "verify shared-secret HMAC" is genuinely new code, not a reuse of
an existing verifier. Use stdlib `crypto/hmac` + `crypto/sha256`, comparing with `hmac.Equal`
(constant-time) — this is the one piece of this feature with no in-repo precedent to build on.

**Recommended route registration**: a new section in `server.go`, next to the
`/api/hooks/permission-request` registration (both are "external system POSTs into stapler-squad
and the *first* thing the handler does is verify a signature before touching any store" —
structurally the same trust-boundary shape):

```go
srv.mux.HandleFunc("POST /webhooks/github", githubWebhookHandler.Handle)       // FR2: github_push
srv.mux.HandleFunc("POST /webhooks/{slug}", genericWebhookHandler.Handle)      // FR4: generic webhook
```

Each new handler type (`GitHubWebhookHandler`, `GenericWebhookHandler` — concrete types, not an
interface with one implementation, per `.claude/rules/interface-pollution-checklist.md`) follows
the `ExternalWebSocketHandler`/`ApprovalHandler` shape already used for raw-HTTP handlers in this
codebase: verify signature first and reject anything that fails before any store/session-creation
code runs (same "hard requirement, not optional hardening" the Slack research doc already
established for its own Phase 2 interactive-button endpoint).

## 2. Where does session/backlog-item creation get invoked from, and does the trigger-fired path reuse it?

Yes — there is exactly one live precedent for "background process fires session creation without a
human in the loop," and it already goes through the same `CreateSession` RPC handler a human
click does: `Scheduler.FireNow` → `s.sessionSvc.CreateSession(ctx, req)` where `sessionSvc` is the
real `*services.SessionService` (wired at `server/dependencies.go:1240`), i.e. **the exact same
`server/services/session_service.go:1251` `CreateSession` entrypoint** the web UI's Omnibar and the
`create_session` MCP tool call. There is no shortcut/bypass path — `FireNow` builds a
`*sessionv1.CreateSessionRequest` and calls the full RPC handler in-process.

The inbound `github_push`/`webhook` trigger paths should do the same: construct a
`*sessionv1.CreateSessionRequest` (or, if the trigger targets a backlog item rather than a live
session — FR1 doesn't specify which — call `storage.CreateBacklogItem` the same way
`server/mcp/tools_backlog.go:1047` `createBacklogItem` does) and call through the existing service
method, not a parallel creation code path. This automatically satisfies Goal 4 ("triggers create
sessions/backlog items through the existing approval/review path") since `CreateSession`'s existing
validation/classifier/approval-rule logic runs unconditionally regardless of caller.

**One difference from `FireNow`**: `createBacklogItem` requires `callerSessionUUID(ctx)` (an
`STAPLER_SESSION_UUID` env-derived caller identity, `tools_backlog.go:1051-1054`) for its own
attribution/logging — a trigger-fired call has no such session context. This mirrors `FireNow`,
which doesn't set caller UUID either and only attributes via `WorkflowId`. Any inbound-trigger code
path that wants to hit `CreateBacklogItem` directly (bypassing the MCP tool wrapper, since there's
no calling session) needs its own attribution field — see §4 below — rather than trying to
synthesize a fake caller session UUID.

## 3. Where should cron trigger evaluation run?

Already answered by §"Correction to requirements.md" above: `server/workflows/scheduler.go`'s
`Scheduler`, started at `server.go:419-423`. No new background goroutine/ticker loop is needed for
the cron case — extend the existing one (new match-criteria fields on `Workflow`, or a sibling
`TriggerType` enum field distinguishing "workflow" cron entries from new "trigger" cron entries if
planning wants to keep the two conceptually separate — see §4's storage decision).

**Distinct precedent worth citing for contrast**: `session.SyncLoop` (`session/backlog_sync.go:34-98`)
is a *second*, independently-built periodic-ticker background loop (`time.NewTicker(15 * time.Minute)`,
`Start(ctx)`/`Stop()`, `runAllSources` iterates all `Enabled` `ItemSource` rows every tick) driving
the GitHub-issues pull-sync feature. It is **not** the right model to copy for `cron` triggers
specifically (the `Scheduler`/`robfig/cron` path already handles arbitrary per-entry schedules more
precisely than a fixed-interval poll-all-and-check loop would), but its `ItemSource`/`PluginRegistry`
pair (`session/backlog_plugin.go`) is the single strongest existing precedent for **inbound
external-source configuration with dynamic enable/disable**, directly relevant to §4's storage
question:

- `ItemSource` ent entity: `plugin_id` (e.g. `"github_issues"`), `config` (JSON, "encrypted PAT"),
  `enabled bool`, `sync_cursor`, `last_synced_at` — DB-backed, toggled live via RPC, no restart.
- `SourceSyncEvent` ent entity: an audit trail row per sync attempt (`items_created/updated/skipped/errored`,
  `error_message`, timestamps) — direct precedent for what a trigger *firing* audit log should look
  like (FR9's "visibly attributable" + FR8/AC8's "failed/malformed requests... not silently dropped").
- `BacklogItem.SourceID` / `edge.From("source", ItemSource.Type)` (`session/ent/schema/backlog_item.go:162`,
  `session/backlog_sync.go` call sites, `session/ent_repository_backlog.go:214,307-310`) — an
  already-wired, already-generated foreign key from a created item back to the external
  configuration that created it. This is the closest existing analog to FR9's "trigger-created
  sessions/backlog items are tagged with their originating trigger."

## 4. Trigger config storage: ent schema (DB), not JSON-config-in-git

requirements.md guesses "JSON config, likely under existing `config/` JSON persistence patterns,
like approval rules." **This guess is based on a false premise.** `ApprovalRule` — the concrete
example requirements.md points to — is **not** a JSON-file config; it's a full ent schema
(`session/ent/schema/approvalrule.go`), DB-backed, with `enabled bool` toggled live via the
`UpsertApprovalRule`/`DeleteApprovalRule` RPCs and zero file-watching/reload machinery needed,
because the DB row *is* the live state. There **is** a separate, optional
`ConfigFileRulesRepository` interface (`server/services/rules_service.go:22-26`) for a
secondary/alternate config-file-backed rule source, but it's explicitly the seam for a *different*,
nil-by-default capability ("nil = feature unavailable"), not the primary storage for approval
rules — the primary storage is the ent DB table.

Given that correction, the real choice is between the two DB-backed patterns already in this
codebase — `ItemSource` (pull/sync-oriented) and `Workflow` (fire/cron-oriented) — and a
config-file (JSON-in-git) alternative modeled on the one place JSON-file config genuinely is the
primary store in this repo: `config/types.go`'s nested structs on `config.Config`
(`HibernationConfig`, `CapacityConfig`, `SessionRetentionConfig`, etc., embedded at
`config/config.go:331-343`).

**Recommendation: ent schema, not `config/` JSON, for trigger *definitions* — for the same reason
`ApprovalRule`/`Workflow`/`ItemSource` all made that choice and none of the actual dynamic,
per-item-toggleable, audit-logged features in this codebase use file-based config.**

Concretely:
- FR6/AC7 ("enable/disable without redeploy") is a **solved problem for free** with an ent-backed
  `enabled bool` column exposed via RPC — this is exactly what `Workflow.CronEnabled` +
  `Scheduler.Reload` already do, and what `ItemSource.Enabled` + `SyncLoop.runAllSources`'s
  `if !src.Enabled { continue }` check already do. A JSON-config-in-git approach would have to
  *build* the dynamic-reload machinery the `dynamic-rule-reload` project needed to add for
  `claude-settings`-sourced rules (fsnotify watcher, debounce, race-safe read-modify-write merge —
  `project_plans/dynamic-rule-reload/research/architecture.md` §2-3) — pure added complexity with
  no benefit, since none of that machinery is needed when the config already lives in a live DB
  table reachable by RPC.
- FR9 (source attribution) is a **solved problem for free** via the same `SourceID`/edge pattern
  `ItemSource`/`BacklogItem` already use, or the `WorkflowId` field `Workflow`/`CreateSessionRequest`
  already use — either gives a real foreign key from the created session/item back to the trigger
  row, queryable and displayable, rather than a free-text label that could drift from the JSON file
  that produced it.
- A `SourceSyncEvent`-shaped audit table (or extending `Workflow`'s implicit "last cron fire" — not
  currently tracked, `Scheduler` has no fire-history table today) gives FR8/AC8's "failed/malformed
  requests are visible, not silently dropped" a durable home, matching `SourceSyncEvent`'s
  `error_message`/timestamps shape.
- JSON-in-git (`config/`) is the *right* fit for genuinely global, singleton, rarely-changed
  settings (the Slack webhook URL precedent, `SlackConfig` on `config.Config`) — but trigger specs
  are a **collection** of independently enable/disable-able, independently attributable rows, which
  is exactly the shape ent + RPC already serves elsewhere in this codebase and JSON-in-git does not
  (no per-row RPC mutation, no per-row FK for attribution, no per-row audit trail without hand-rolling
  one).

**Concrete schema recommendation**: extend `Workflow` (add `trigger_type` string enum —
`"cron"`/`"github_push"`/`"webhook"`/`"manual"` — defaulting existing rows to `"manual"`/`"cron"`
based on current `CronEnabled`; add `github_repo`, `github_branch`, `webhook_slug` (unique,
indexed, for `/webhooks/{slug}` routing), `webhook_secret` (reuse `ItemSource.Config`'s
"encrypted":true JSON-blob-with-decrypt-key convention rather than inventing a second secret
storage shape), `event_filter`, `label_filter`) rather than inventing a parallel `Trigger` entity
that duplicates `Command`/`InputTemplate`/`TargetDirectory`/`SessionType`/`AgentType`/`Model` and
needs its own `FireNow`-equivalent. A `Trigger` row *is* a `Workflow` row with a different
activation mechanism — same "render a prompt, call `CreateSession`, attribute the result" shape in
every case. The one thing this adds to `Scheduler`/`WorkflowService`: a second, richer template
renderer (Go `text/template`, for FR4's payload interpolation) alongside `FireNow`'s existing
`{{input}}`-only `strings.ReplaceAll` path, selected by `trigger_type`.

Outbound callback config (FR7) is architecturally different — see §5 — and does not need to live on
`Workflow` at all.

## 5. Outbound callbacks (FR7-FR9) — building directly on the Slack research

Per the Slack precedent, the three lifecycle events map to these call sites:

| Event | Where it's determined today | Call site to add the (async, non-blocking) dispatch |
|---|---|---|
| `on_queue_item_created` | `ReactiveQueueManager.OnItemAdded` | `server/review_queue_manager.go:327-419`, next to the existing `rqm.eventBus.Publish(...)` (~line 411) — **identical call site the Slack doc already found and documented**, reuse directly. |
| `on_session_complete` | `ReasonTaskComplete` determination (`session/review_queue_determiner.go:150,225`, surfaced via `AttentionReason_ATTENTION_REASON_TASK_COMPLETE`, `server/services/review_queue_service.go:360-361`) — this is the *session-level* "agent is done, needs review" signal, not a durable status. The *durable*, crash-safe signal is a `BacklogItem` status transition to `BacklogStatusDone`/`BacklogStatusPRPending` (`session/domain/backlog.go:23`) via `TransitionBacklogItemStatus`. | Two candidate points; recommend firing off the **durable status transition**, not the ephemeral `ReasonTaskComplete` flag — see crash-consistency note below. |
| `on_session_stale` | Per stale-session-detection's research: canonical Go signal is `Instance.GetTimeSinceLastMeaningfulOutput()` (`session/instance_approval.go:112`), already consumed by the Review Queue "Stale" badge (`session/review_queue_determiner.go:259`) and the durable `reconcileStaleWorkSessions` (`session/backlog_lifecycle.go:2294`, ticker-driven periodic reconciler — already a background loop, already the right place to add an outbound fire since it already durably marks `StuckReasonStaleWork`). | `session/backlog_lifecycle.go`'s `reconcileStaleWorkSessions`, at the point it durably marks a session stuck — reuse the *durable* detector, not the Review Queue badge's 5-min ephemeral one, to avoid a callback firing on every poll tick while a session sits stale (needs the same "only fire once per crossing" dedup latch the Slack doc flagged for its queue-depth-threshold case, §5 of that doc). |

**Config placement**: follow the `SlackConfig` precedent exactly (`config/types.go`, embedded on
`Config` at `config/config.go:331-343`'s sibling block) — a new `CallbackConfig` struct:

```go
type CallbackConfig struct {
    OnSessionCompleteURL   string `json:"on_session_complete_url,omitempty"`
    OnSessionStaleURL      string `json:"on_session_stale_url,omitempty"`
    OnQueueItemCreatedURL  string `json:"on_queue_item_created_url,omitempty"`
}
```

This treats callbacks as **global, singleton integration targets** ("notify this URL whenever any
session anywhere completes"), matching FR7's literal wording ("each accept a URL," singular) and
the Slack precedent's own config shape. If planning later wants *per-workflow* distinct callback
URLs (e.g. only pipeline-chained sessions notify a URL, not every session), that's an additive
`Workflow`-level field, not a reason to restructure this global config now — avoid building that
generality until a concrete second consumer needs it, per the interface-pollution checklist's
"don't design for a speculative second case."

**Delivery mechanism**: identical to the Slack doc's `SlackNotifier` — a concrete
`CallbackDispatcher` (`server/services/callback_dispatcher.go`, new file), `go`-launched from each
of the three call sites above under a `cfg.Callbacks.OnXURL != ""` guard, deriving its own
`context.WithTimeout(parentCtx, 5*time.Second)`, POSTing a JSON payload, logging (not propagating)
any error, bounded retry per FR8 (a small fixed retry count with backoff — e.g. 3 attempts,
matching "best-effort with bounded retry" — not a durable retry queue, which would be
disproportionate scope for this pass), never logging the URL itself.

### Crash-consistency concern for FR10 pipeline chaining (explicitly asked about)

FR10 wires `on_session_complete` (FR7) back into trigger-fired session creation (FR3) — **this is
architecturally sound as one mechanism, not two**: both are "an event happened; render a
next-session prompt (optionally interpolating prior output); call `CreateSession`." The risk is
specifically the **gap between "prior session marked complete" and "next session actually created"**
if the process crashes mid-way:

- If completion-detection and next-session-creation are both purely in-memory/synchronous within
  one request handler, a crash between them **loses the chain silently** — the prior session shows
  complete, no next session ever appears, and nothing indicates why.
- The fix is **not** a new durable job queue (disproportionate for this pass) — it's to make the
  chain-fire step **idempotent and re-derivable from already-durable state**, the same way
  `reconcileStaleWorkSessions` and `SyncLoop.runAllSources` are periodic reconcilers rather than
  one-shot fire-and-forget: persist which `Workflow`/trigger a `BacklogItem`/`Session` should chain
  to *before* the triggering session starts (e.g. a `next_workflow_id` field set at the point the
  chain is configured, not computed reactively at completion time), and persist a
  `chained_at`/`chain_fired bool` marker *at the same time* the status transitions to done (i.e. in
  the same `TransitionBacklogItemStatus` call, not a follow-up step) so a restarted process can scan
  for `status=done AND next_workflow_id IS NOT NULL AND chain_fired=false` and complete any chain
  that got interrupted, exactly like `reconcileStaleWorkSessions` already scans for missed stale
  transitions on a ticker. This reuses the existing periodic-reconciler idiom instead of inventing
  transactional outbox machinery.
- The prior session's **output** (needed for FR10's "pass the completed session's output... into
  the next session's prompt") must itself be durably captured before or atomically with the status
  transition — check whatever already captures session output for the Review Queue diff/summary
  view (likely already durable, since PR/diff data survives restarts today) rather than assuming a
  fresh read of live tmux scrollback, which would not survive the crash window this analysis is
  about.

## Is an Event-Command-Policy table warranted?

**Yes — unlike `slack-review-notifications` (two POSTs off two known call sites) and
`stale-session-detection` (single-actor, local wiring decisions), this domain has genuinely
multiple external actors (GitHub, a generic webhook caller, the in-process cron clock, the session
lifecycle state machine) and multi-step business rules with real branching (match → verify auth →
render template → create session → attribute source → possibly chain again → possibly notify
externally) as requirements.md's own domain reasoning anticipated.**

| Domain Event (what happened) | Policy trigger (whenever X, then…) | Command (intent to change state) | Actor / System |
|---|---|---|---|
| GitHub push webhook received | whenever signature valid AND repo/branch matches an enabled `github_push` trigger | `VerifyGitHubSignature` → `RenderPromptTemplate` → `CreateSession` | GitHub (external) → new HTTP handler |
| GitHub push webhook received | whenever signature invalid/missing | `RejectRequest(401)` | New HTTP handler |
| Generic webhook POST received at `/webhooks/{slug}` | whenever HMAC valid AND `event`/`label_filter` match an enabled trigger for that slug | `RenderPromptTemplate(payload)` → `CreateSession` | External caller → new HTTP handler |
| Generic webhook POST received | whenever HMAC invalid, slug unknown, or event/label doesn't match | `RejectRequest` / no-op (ignored, not an error, per AC3) | New HTTP handler |
| Cron tick reaches a scheduled time | whenever an enabled `Workflow`/trigger's cron expression is due | `FireNow` → `CreateSession` | `robfig/cron` (in-process) → `Scheduler` |
| `CreateSession` completes for a trigger-fired request | always | `AttributeSource(sessionID, triggerID)` | `SessionService` |
| Session/`BacklogItem` reaches a terminal "done" status | whenever `next_workflow_id` is set and `chain_fired=false` | `RenderPromptTemplate(priorOutput)` → `CreateSession` → `MarkChainFired` | `TransitionBacklogItemStatus` caller / reconciler |
| Session/`BacklogItem` reaches a terminal "done" status | whenever `cfg.Callbacks.OnSessionCompleteURL != ""` | `DispatchCallback(session_complete, payload)` | `CallbackDispatcher` (async, best-effort) |
| Durable stale detector marks a session `StuckReasonStaleWork` (first crossing only) | whenever `cfg.Callbacks.OnSessionStaleURL != ""` | `DispatchCallback(session_stale, payload)` | `CallbackDispatcher` (async, best-effort) |
| `ReactiveQueueManager.OnItemAdded` fires for a new review-queue item | whenever `cfg.Callbacks.OnQueueItemCreatedURL != ""` | `DispatchCallback(queue_item_created, payload)` | `CallbackDispatcher` (async, best-effort) |
| Callback delivery fails after bounded retry | always | `LogDeliveryFailure` (never silently dropped, per FR9) | `CallbackDispatcher` |
| Reconciler restarts (process boot) | whenever any `status=done AND next_workflow_id != nil AND chain_fired=false` rows exist | replay `RenderPromptTemplate` → `CreateSession` → `MarkChainFired` | Periodic reconciler (new, modeled on `reconcileStaleWorkSessions`) |

## Integration points summary

| Area | File(s) |
|---|---|
| Existing cron infra to extend (FR3/FR6) | `server/workflows/scheduler.go`, `session/ent/schema/workflow.go`, `server/services/workflow_service.go` |
| New inbound HTTP routes (FR2, FR4) | `server/server.go` (new `HandleFunc` block near `:511`), new `server/services/github_webhook_handler.go`, `server/services/generic_webhook_handler.go` |
| Signature verification (net-new, no precedent) | `crypto/hmac`/`crypto/sha256` stdlib, `hmac.Equal` |
| Session/backlog creation reuse (Goal 4) | `server/services/session_service.go:1251` `CreateSession`, `server/mcp/tools_backlog.go:1047` `createBacklogItem` pattern |
| Trigger config storage (recommend: extend `Workflow`, not new JSON config) | `session/ent/schema/workflow.go` (+ full ent codegen per `.claude/rules/ent-schema-generation.md`), contrast precedent `session/ent/schema/item_source.go` |
| Rich template rendering (FR4, needed alongside `FireNow`'s simple `{{input}}` path) | New render path in `server/workflows/` or `server/services/`, Go `text/template` |
| Source attribution (FR9) | `Workflow.ID`/`WorkflowId` field on `CreateSessionRequest` (existing), `BacklogItem.SourceID`/`ItemSource` edge (existing alternate pattern) |
| Outbound callback config (FR7) | `config/types.go` new `CallbackConfig`, `config/config.go` new `Callbacks` field (mirrors `SlackConfig` from `project_plans/slack-review-notifications`) |
| Outbound callback dispatch (FR7-FR9) | New `server/services/callback_dispatcher.go`; call sites: `server/review_queue_manager.go:327-419` (`OnItemAdded`), wherever `TransitionBacklogItemStatus` lands on `BacklogStatusDone`, `session/backlog_lifecycle.go:2294` (`reconcileStaleWorkSessions`) |
| Pipeline chaining durability (FR10) | New `next_workflow_id`/`chain_fired` fields (on `BacklogItem` or `Session`, TBD in planning), new periodic reconciler modeled on `session/backlog_lifecycle.go`'s `reconcileStaleWorkSessions` |
| Audit trail for trigger fires (FR8/AC8) | Model on `session/ent/schema/source_sync_event.go`'s shape (`error_message`, timestamps, outcome counts) — new `TriggerFireEvent`-equivalent, or extend `Workflow` with a lightweight last-fire-status field if full history isn't needed |
