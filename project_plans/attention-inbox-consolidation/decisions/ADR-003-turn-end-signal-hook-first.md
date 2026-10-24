# ADR-003: Hook-first turn-end signal with per-program screen fallback

**Status**: Proposed. Payload shape settled by Spike 0.1.1 (see Spike findings); Phase 0 exit gate evaluated, pending owner confirmation (see Phase 0 exit gate)
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Context

Nothing in the stack produces a "turn ended" fact. `HandleStop` (`server/services/hook_receivers.go:65-70`) drains and logs; `scripts/ssq-hook-handler` `stop` (lines ~448-463) always emits `Task Complete` LOW using `.stop_reason // .reason`, never reading the transcript. Screen patterns (`session/detection/binaries/claude.go`) never classify prose. Regexing a `?` on screen text flags most finished turns (agents end almost every turn with a question-like sentence).

## Decision

1. **Trigger** = Claude `Stop` hook. `HandleStop` parses a bounded body (existing 64 KB `readBody` cap), resolves the session, and records a `TurnEnded{Episode, Kind, Excerpt, Source}` fact on the Instance via an actor setter published to `InstanceSnapshot` (per `.claude/rules/instance-lock-free-reads.md`).
2. **Text source** = last assistant message. Spike 0.1.1 decides between a payload field and reading the tail of `transcript_path` JSONL. Official docs show `transcript_path` and `stop_hook_active`; `last_assistant_message` appears only in third-party guides (UNVERIFIED), so the design assumes the transcript tail and treats a payload field as an optimization. **Superseded by Spike 0.1.1 below: the payload field exists in Claude Code 2.1.310.**
3. **Classifier** = pure function `ClassifyTurnEnd(text) (TurnEndKind, Confidence)`; heuristic first (trailing question, interrogative lead-ins, enumerated options, no pending tool_use). Applied to the last assistant message. The only screen-derived input is the screen-fallback stage (point 5, Story 3.1.3), which extracts the bottom-most prose paragraph above an idle prompt first (never a raw screen tail) and ships `Finished`-only until a labelled per-program corpus gates `Question`. When unsure it returns `Finished` (low priority), not `Question`.
4. **LLM tiebreak** = optional second stage behind `attention_prose_question_llm_tiebreak` (default off), via the existing `session/headless` pool (`server/llm_selector.go`), one flight per episode, cache by message hash, timeout fails open to a visible low-priority item, output constrained to an enum, never on the poll path, raw text redacted before send. Adversarial text ("ignore previous instructions") is in the fixture corpus.
5. **Non-Claude programs** (aider, agy, pi, gemini, opencode) have no Stop hook; a per-program signal table maps each to {hook | screen heuristic | idle-for-N | none}, and "none" is shown as "cannot detect", not silently omitted. Coverage is measured per program on a fixture corpus.
6. `ssq-hook-handler stop` and the HTTP endpoint must not both emit: `HookDeduper` (`server/services/hook_deduper.go`) keys the episode so one stop yields one fact and at most one notification.

7. **Phase 0 go/no-go (added in the Phase 4 patch)**: this design rests on the Stop hook firing for the sessions the operator actually runs. Spikes 0.1.1/0.1.4 must record per-creation-path coverage. If coverage is below the exit criterion in plan.md Story 0.1.5, Phase 3 switches to the named fallback recorded there (F1: throttled transcript-tail polling for Claude; F2: `Finished`/`awaiting_direction` only with "cannot detect" shown, no prose-question kind), decided before Story 3.1.1 starts. Per-program hook coverage is also a precondition for flipping `attention_prose_question_detection` on.

## Alternatives rejected

Screen-regex only (is the bug); LLM as primary detector (cost, nondeterminism, outage hides approvals); external inbox/router products (build-vs-buy: none own the "session needs me" domain logic).

## Consequences

Claude coverage is near-exact; other programs are weaker and visibly labeled. Excerpts persisted are capped and scrubbed (ADR-004).

## Spike findings (Phase 0, 2026-10-24)

All live runs: isolated instance (`STAPLER_SQUAD_INSTANCE=claude-manual-test`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, port 62871), Claude Code 2.1.310 (Sonnet 5.5), `--dangerously-skip-permissions`-equivalent bypass mode, throwaway repos under `~/.cache/aic-tmp/spikes1/`. Labels: VERIFIED = ran it; INFERRED = code reading (file:line).

### 0.1.1 Stop payload and transcript shape (VERIFIED)

- **Last message in payload: yes.** Raw body received by `HandleStop` (temporary debug dump, reverted, not committed) for the turn "Which approach do you want, A or B?" has keys: `session_id`, `transcript_path`, `cwd`, `scratchpad_dir`, `prompt_id`, `permission_mode`, `effort{level}`, `hook_event_name` ("Stop"), `stop_hook_active` (false), `last_assistant_message` ("Which approach do you want, A or B?"), `background_tasks` ([]), `session_crons` ([]). A plain finish turn ("Done.") and a `claude -p` one-shot turn carry the same keys, except the `claude -p` payload had no `scratchpad_dir` (treat it as optional). There is **no `stop_reason` or `reason` key**, so `scripts/ssq-hook-handler stop` (`.stop_reason // .reason // "Task completed"`, lines ~448-463) always takes the default branch and its `max_tokens` / `error*` classification never triggers on this version.
- The same stdin body reached both an HTTP curl hook and a `tee` command hook (compared with a shell string comparison: identical; the stdin copy has one extra trailing newline).
- Fixture: `session/testdata/hooks/stop_payload_claude.json` (ids, paths redacted).
- **Transcript JSONL shape** (`transcript_path`, one JSON object per line; fixture `session/testdata/hooks/transcript_tail_question.jsonl`, trimmed and redacted): the last `"type":"assistant"` line has `message.role="assistant"`, `message.stop_reason="end_turn"`, and `message.content` = array of blocks; text is the `{"type":"text","text":...}` block(s) (tool use would add `{"type":"tool_use"}` blocks). Extracted text equals "Which approach do you want, A or B?" and equals `last_assistant_message`. Lines after it: `{"type":"system","subtype":"stop_hook_summary","hookInfos":[{"command","durationMs"}...]}` then `{"type":"system","subtype":"turn_duration"}`. Many non-conversation line types interleave (`attachment`, `last-prompt`, `mode`, `permission-mode`, `ai-title`, `api-request*`, `file-history-snapshot`); a tail reader must filter on `type`. In the captured file the assistant line precedes the `stop_hook_summary` line (file order), but the race "is the assistant line flushed before the hook runs" was not isolated; the payload field avoids it.
- Decision effect: point 2 of the Decision becomes "payload `last_assistant_message` first, transcript tail as fallback/verification". Not verified: payload behaviour for tool-use-terminated turns, very long messages (truncation), or other Claude versions; the 64 KB `readBody` cap must be re-checked against a long final message.

### 0.1.4 Per-creation-path table

| Creation path | Stop event fires | Last assistant text obtainable | Active Stop hook today | Evidence |
|---|---|---|---|---|
| Web UI / `CreateSession` RPC (`session_creation_pipeline.go`) | yes (VERIFIED) | yes at the hook (payload field); server-side only if an HTTP Stop hook is wired (VERIFIED field; wiring absent) | **global `ssq-hook-handler stop` only**; injected HTTP hook NOT present | Created `aic-spike-web` via `POST /api/session.v1.SessionService/CreateSession`; its `.claude/settings.local.json` held only `PermissionRequest` (matches `approval_handler.go:1044` `InjectHookConfig`, called at `session_creation_pipeline.go:314`); Claude's transcript `stop_hook_summary.hookInfos` listed `ssq-hook-handler stop` + the other global hooks and no stapler HTTP hook; `[hook/stop]` never logged |
| Manual tmux create (claude started by hand in a tmux pane, no ssq session) | yes (VERIFIED) | yes at the hook; **no server-side reader** (server cannot resolve the session) | global `ssq-hook-handler stop` only | `tmux -L ssq-manual-1 new-session ... claude` in a fresh repo: `stop_hook_summary` lists the 5 global hooks only; server log `delivery_unresolved_fail_open` for `aic-manual-tmux`, `NOTIFICATION_TYPE_TASK_COMPLETE`; tray store holds "Task Complete / Task completed" with only cwd/git metadata |
| MCP `create_session` (`server/mcp/tools_lifecycle.go:188`) | yes (INFERRED: same Claude, global hook) | yes at the hook | HTTP Stop hook only if the caller passes `hooks:["stop_notification"]`; otherwise global script only | Tool help and `docs/tasks/stapler-squad-mcp-server.md:172` say the default is `[permission_approval, stop_notification]`, but `tools_lifecycle.go:234-243` passes only the requested names and `wantedHookSet` (`hook_injector.go:342-343`) adds only `permission_approval`. Doc and code disagree; not run live |
| Backlog-dispatched (`SpawnSessionFromItem` -> `CreateDirectorySession`/`CreateWorktreeSession`, `session_service_diagnose.go:122,193`; `backlog_service_triage.go:1205-1222`) | yes (INFERRED for interactive work sessions; `claude -p` one-shot VERIFIED to fire Stop with `last_assistant_message`) | yes at the hook | global script only; injected HTTP hook never (only `HookGitDriftCheck` for autonomous items, `backlog_service_triage.go:1217`) | code reading; those Create* functions call neither `InjectHookConfig` nor `InjectHooksConfig` for Stop |

Which Stop hook is active, summary: the stapler-injected HTTP hook (`hook_injector.go:118`) is active on **no default path**; the only Stop consumer that fires everywhere is the operator's global `~/.claude/settings.json` entry `ssq-hook-handler stop` (installed by `InstallHooks`, `internal/claudehooks/claudehooks.go`). It sees the full payload on stdin but forwards only a notification with title/message (no text). A fresh machine without that global install would have no Stop consumer at all. Consequence for Story 3.1.1: `HandleStop` currently receives nothing in production, so the plan must add `stop_notification` to default injection for the `CreateSession` pipeline and the backlog `Create*Session` paths, or have the script forward the payload; dedup key then needs `session_id` + `prompt_id` (present in the payload) since both consumers fire (matches Decision point 6).

### 0.1.4b Sinks for TASK_COMPLETE and INPUT_REQUIRED, visible session

Policy: a visible session always delivers (`server/deliverygate/policy.go:~160`, `OutcomeDeliver/ReasonVisibleSession`). The class table (`policy.go:134,136`) makes `INPUT_REQUIRED` needs-human and `TASK_COMPLETE` routine, which only matters for hidden sessions (routine suppressed, needs-human delivered).

| Sink | TASK_COMPLETE (visible) | INPUT_REQUIRED (visible) | Source |
|---|---|---|---|
| Tray (history store) | yes: every `EventNotification` is recorded regardless of type (VERIFIED: "Task Complete" rows for `aic-spike-web` and `aic-manual-tmux` in `notifications.json`) | yes (INFERRED, same path) | `server/notifications/subscriber.go:73-80,178-208` |
| Toast (web client) | yes unless capped deck or quiet mode; pending decisions always toast (INFERRED) | yes, always (INFERRED) | `web-app/src/lib/contexts/NotificationContext.tsx:204-206` |
| Web push | no: only priority URGENT within TTL, or approval type | no unless URGENT (hook/script priority is low/medium; INFERRED) | `server/push/subscriber.go:162-181` |
| Slack | only through the review-queue path, for any reason, if `cfg.Slack.NotifyOnQueueItem` (or queue-depth threshold); not triggered by `ssq-notify` bus events | same | `server/review_queue_manager.go:487-500` (gated by `AllowQueueItem`, `gate.go:248`) |

Note: the review queue (screen-detected `ReasonTaskComplete`) emitted the TASK_COMPLETE row for the web session; the global script's own "Task Complete" notification for a web-created session did not appear as a separate row in the isolated run (unexplained; not investigated). For the manual tmux session the script's rows did appear.

**Who reads notification metadata:** `ReactiveQueueManager.OnItemAdded` builds it (`review_queue_manager.go:460`, `events.SessionScopedMetadata(item.Metadata, linkedItemID)`) and publishes it on the event (`:462-480`); consumers are the tray store (`notifications/subscriber.go:204,207`, persists `Metadata`, reads `session_scoped`), the delivery gate (`deliverygate/policy.go:193-210` `ParseClassHint` reads `events.MetadataKeyDeliveryClass` / `MetadataKeyUntrustedType`; `gate.go:248,308` pass `metadata`), and Slack via the same `metadata` argument to `allowQueueItem` (`:491`). Push reads only priority/type (`shouldNotify`). So for Task 3.2.3d, an `attention_reason` key can steer the gate (a new hint key in `ParseClassHint`) and the tray/client rows; push and the Slack formatting would need explicit new reads.

## Phase 0 exit gate (Story 0.1.5): evaluated, pending owner confirmation

Weights: operator's session store, read-only (`sqlite3 -readonly "file:~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db?mode=ro"`, the live store per `~/.stapler-squad/preferred_workspace`; opened read-only in place rather than copied because it is 1.4 GB and being written), `sessions.created_at >= '2026-09-24'`: **552 sessions**, all claude. Classification by `item_sessions.session_uuid` membership (no creation-source column exists):

| Path | Sessions (30 d) | Share | Notes |
|---|---|---|---|
| Backlog-dispatched | 528 | 95.7% | 257 interactive `work`, 271 one-shot hidden (`review`/`diagnose`, `claude -p`) |
| Web UI + MCP (indistinguishable) | 24 | 4.3% | 18 interactive, 6 one-shot hidden; web vs MCP split not recoverable from the store |
| Manual tmux | not in store | unknown | unmanaged sessions are not persisted; operator count unobtainable here |

Scores per the criterion "Stop fires AND last assistant text obtainable (payload field or transcript tail)":

- **Capability weighting (Stop event and payload text exist on the path)**: web yes, manual yes, MCP yes (inferred), backlog yes (inferred; one-shot verified) = 100% of the 552 stored sessions, and of any manual share. **GO.**
- **As-shipped weighting (server receives the payload today)**: 0% of sessions, because no default path injects the HTTP Stop hook; `HandleStop` is dead in production. This is NO-GO on the literal reading, with fallback **F1** (the transcript is readable and its path is known to the server for managed sessions: the instance logs `conversation uuid set ... history=<transcript path>`, and `item_sessions.conversation_uuid` stores it).
- Manual-tmux sensitivity: those sessions fire Stop but are unresolvable server-side; even if they were counted as failures, GO holds while they are at most 20% of sessions; the share is unknown.

**Verdict: GO, conditional (evaluated, pending owner confirmation).** The premise "Claude's Stop event carries the last assistant text" is verified; the plan's defect is wiring, not signal. GO requires Story 3.1.1 to inject the Stop HTTP hook on the `CreateSession` pipeline and the backlog `Create*Session` paths (or forward the payload from `ssq-hook-handler`). If the owner declines that wiring, the chosen fallback is **F1** (transcript-tail polling), not F2. The 80% threshold and the fallback definitions remain owner-to-confirm and OPEN.

Not verified: backlog and MCP paths were not run live; a Claude version other than 2.1.310; payload for tool-use-terminated or very long turns.
