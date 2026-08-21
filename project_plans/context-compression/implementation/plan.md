# Implementation Plan: context-compression

**Feature**: Retarget "context compression at 85%" into "Restart with a compressed handoff summary" — an async, Hermes-technique-informed summary of a degrading session's real transcript content, delivered as the opening prompt of a fresh restart session, triggered manually today and by a future `ContextHealth` RED verdict once `context-health-monitoring` ships.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: `decisions/ADR-001-handoff-summary-restart-not-live-injection.md`

---

## Step 0.5 — Creative Pass: alternatives already evaluated

Full analysis in `research/build-vs-buy.md`. Summary (see also ADR-001):

| # | Option | Strength | Weakness |
|---|--------|----------|----------|
| 1 | Build custom Go compressor: track tokens in `claude_controller.go`, call a cheap model, splice a synthetic message into the live conversation at 85% | Token tracking and the cheap-model call are both independently buildable/reusable | The splice mechanism cannot compress anything — `SendCommand` only appends PTY keystrokes; it cannot remove turns the CLI subprocess already holds (`research/build-vs-buy.md` §"the architecture constraint") |
| 2 | Rely on Claude Code's own native auto-compaction, surface it in the UI only | Strictly better compression than anything buildable at the controller layer, and `context-compaction-detection` already has this fully planned | Visibility only — no worse-case recovery path (clean restart) if native compaction still isn't enough |
| 3 | **Extend `context-health-monitoring`'s deferred "restart with summary" direction ← CHOSEN** | Reuses `SessionSummaryGenerator`'s proven async-dispatch/dedup/persist pipeline and the `ContextHealth` RED trigger; substantially less new code than Option 1 | Requires its own new SDD cycle (this plan) since neither sibling project designed the summarization/restart mechanic itself |
| 4 | Port Hermes's Python `ContextEngine` verbatim | N/A — different language, different architecture (Hermes owns the messages array; stapler-squad drives an opaque CLI subprocess) | Not attempted as code; its *techniques* (REFERENCE-ONLY prefix, head/tail framing, tool-output pruning, proportional budget with a ceiling) are adopted into Option 3's prompt construction instead |

Rejected alternatives are also recorded in the Pattern Decisions table below, per row.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `HandoffSummary` | The feature as a whole, and the ent entity/Go struct that persists one generated summary for one source session. | Analogous to `SessionSummary` (`session/ent/schema/session_summary.go`) but content-different: an "active task + state to resume" packet, not a "what happened" completion report. |
| `HandoffSummaryStatus` | String-backed status: `pending` / `generating` / `ready` / `error`. | Exact shape of `SessionSummaryStatus` (`session/session_summary_service.go` uses `SessionSummaryStatusGenerating` etc. as typed string consts). |
| `HandoffSummaryGenerator` | Go orchestrator (`session/handoff_summary_service.go`) owning the headless pool, ent client, and an in-process dedup map (`sync.Map`) — mirrors `SessionSummaryGenerator` (`session/session_summary_service.go:54-93`) field-for-field. | Async dispatch only; never called synchronously from an RPC handler. |
| `GenerateHandoffSummary` | New function in `session/headless/features.go` (alongside `GenerateSessionCompletionNarrative`, line 335) that builds the REFERENCE-ONLY-prefixed prompt and dispatches it via `pool.CallBlocking(ctx, FeatureKeyHandoffSummary, ...)`. | Not a method — a package-level function, per the file's existing convention. |
| `FeatureKeyHandoffSummary` | New `FeatureKey` const `"handoff-summary"` in `session/headless/features.go`, alongside `FeatureKeySessionCompletionSummary` (line 26). | Distinct key so per-feature session rotation doesn't mix handoff and completion-narrative calls. |
| `ReferenceOnlyPrefix` | The exact wording quoted verbatim in `requirements.md:26-33` ("[CONTEXT COMPACTION — REFERENCE ONLY]..."), reused unmodified as a Go string constant. | Placed at the top of the rendered summary text so the *new* session's Claude Code process treats it as background, not as an active instruction to re-execute. |
| `ActiveTaskSection` | The `## Active Task` section `GenerateHandoffSummary`'s prompt is instructed to always produce, naming what the new session should do next. | Directly named in `requirements.md`'s Hermes excerpt ("Your current task is identified in the '## Active Task' section"). |
| `TranscriptWindow` | Struct `{Head, Middle, Tail []ClaudeConversationMessage}` — the three-way split of a session's real message content built by `buildTranscriptWindow`. | Head/tail protection reframed per ADR-001: since there is no live history to splice, "protection" means the prompt is given the head and tail *verbatim* and instructed to compress only the middle into prose, not that any programmatic array-splice occurs. |
| `buildTranscriptWindow` | Function in `session/handoff_summary_excerpt.go` that calls `session.ClaudeSessionHistory.GetMessagesFromConversationFile` (`session/history.go:571-581`) and splits the result into `TranscriptWindow{Head: first N, Tail: last M, Middle: everything between}`. | `N`/`M` are small fixed constants (first/last 2 messages) — enough to ground "original task" and "current state" without needing config. |
| `pruneExcerptText` | Function in `session/handoff_summary_excerpt.go` that truncates any single `ClaudeConversationMessage.Content` longer than `maxExcerptMessageBytes` before it's included in the prompt. | Builds on top of `session/history.go`'s `extractMsgContent` (lines 401-432), which **already drops tool_use/tool_result blocks** (it only concatenates `type == "text"` content items) — this function only adds a size cap on oversized text blocks (e.g. a pasted file dump inside an assistant turn), it does not re-implement blob stripping. |
| `SummaryBudget` | Struct `{MiddleExcerptMaxBytes int}` — the "proportional... with an absolute ceiling" sizing rule from `requirements.md`. | Proportional to `len(Middle)` messages, capped at `HandoffSummaryConfig.MaxMiddleExcerptTokens` (default equivalent to ~12k tokens of excerpt text, matching the Hermes-cited ceiling) — see Pattern Decisions for why this budgets the *input* excerpt, not the model's output length. |
| `HandoffSummaryConfig` | New config struct in `config/types.go`, alongside `CapacityConfig` (line 291). | Fields: `Enabled bool` (default true), `MaxMiddleExcerptTokens int` (default ~12000). |
| `HandoffSummaryConfigOrDefault` | `func (c HandoffSummaryConfig) HandoffSummaryConfigOrDefault() HandoffSummaryConfig` — exact shape of `CapacityConfigOrDefault` (`config/types.go:310-320`). | `<= 0` on `MaxMiddleExcerptTokens` means unset, not "no limit." |
| `ContextHealth` | **Borrowed, not redefined here.** The green/amber/red per-session signal designed (not yet shipped) by `project_plans/context-health-monitoring/implementation/plan.md`. | This plan only *consumes* a future RED transition as a trigger — see Phase 4 and Unresolved Questions. Its own `ContextHealthLevel`/`ContextHealthVerdict`/proto fields 72-73 are that project's types, not redefined here. |
| `TriggerHandoffSummary` (RPC) | New ConnectRPC method on `HandoffSummaryService`, mirrors `RegenerateSessionSummary` (`server/services/session_summary_service.go`): dispatches `HandoffSummaryGenerator.GenerateAndPersist` as a goroutine and returns the current (possibly still-generating) row. | Callable from the manual "Restart with summary" button (always available) and, later, from the RED-verdict auto-trigger (Phase 4, blocked). |
| `GetHandoffSummary` (RPC) | New ConnectRPC method, mirrors `GetSessionSummary` — read-only row lookup by `session_id`, with the same lazy stale-`GENERATING`-row reconcile-to-`ERROR` behavior. | |
| `restart_from_session_id` | New field 28 on `CreateSessionRequest` (`proto/session/v1/session.proto`) — the source session's ID, for lineage. | Purely informational; does not change session-creation *behavior* (see Pattern Decisions "session-creation mode"). |
| `restarted_from_session_id` | New field 74 on `Session` (`proto/session/v1/types.proto`, chosen to avoid colliding with `context-health-monitoring`'s already-claimed-but-unshipped fields 72/73 — see Migration Plan's coordination note) — mirrors `restart_from_session_id` back onto the created session's own record, so the UI can render "Restarted from: <link>". | Empty when the session was not created via this flow. |
| `RestartWithSummaryButton` | New React component (`web-app/src/components/sessions/RestartWithSummaryButton.tsx`) — triggers generation, polls via `useHandoffSummary`, and on a READY row calls `createSession({ path, prompt: summary.summaryText, restartFromSessionId: sessionId })`. | |
| `useHandoffSummary` | New hook (`web-app/src/lib/hooks/useHandoffSummary.ts`) — near-verbatim structural copy of `useSessionSummary` (`web-app/src/lib/hooks/useSessionSummary.ts:64-220`): poll-while-generating, stale-request guard via `activeSessionIdRef`, `MAX_NULL_POLL_ATTEMPTS`. | |
| `HandoffSummarySection` | New React component in the session detail **Info tab**, built on `WorkflowHistorySection.tsx`'s structural pattern (capped list, `CollapsibleSection`, explicit empty state, `role="list"`/`role="listitem"`) per `research/ux.md` §2's recommendation. | Not a card-row badge (ux.md §1). Typically renders 0 or 1 row (a session usually triggers this once), but the list shape accommodates retries after an `ERROR` row. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall mechanism | Restart-with-handoff-summary (new session, spawn-time `prompt`) | ADR-001; `research/build-vs-buy.md` Option 3 | Option 1: live mid-session compression via synthetic PTY injection | `SendCommand`/`SendCommandImmediate` (`session/claude_controller.go:452-514`) can only append PTY keystrokes — no mechanism removes turns the CLI subprocess already holds, so injection cannot reduce context even if built (`research/architecture.md` §1-2, `research/build-vs-buy.md` "architecture constraint") |
| Persistence | New `HandoffSummary` ent entity (own table), not a column/kind on `SessionSummary` | `session/ent/schema/session_summary.go` as structural template | Reuse `SessionSummary` with a `kind` discriminator column | Different content shape (active-task packet vs. completion narrative) and a different dedup/status lifecycle owner would complicate `SessionSummaryGenerator`'s already-tested FR-7 guard; a second small entity is cheaper than branching one god-entity |
| Async orchestration | `HandoffSummaryGenerator.GenerateAndPersist`, mirroring `SessionSummaryGenerator.GenerateAndPersist` exactly: `tryAcquire`/dedup via `sync.Map`, panic-safe deferred recover, interim `GENERATING` upsert, final status-transitioning upsert | `session/session_summary_service.go:159-479` | Synchronous generation inline in the RPC handler | LLM round-trip latency (seconds) on the RPC critical path with no retry/dedup guard — exactly the cost/latency gap `research/pitfalls.md` §3 flags as missing from the original AC list |
| Transcript content source | `session/history.go`'s `ClaudeSessionHistory.GetMessagesFromConversationFile` (real message text) | `session/history.go:326-436,571-581` (verified: `extractMsgContent` returns actual `Content` text) | `session/tokens`' `ParseResult`/`TurnTimeline` | `session/tokens` deliberately never retains message content (`session/tokens/doc.go`'s privacy guarantee, confirmed by `research/pitfalls.md` §3) — it has token counts and tool *names* only. `session/history.go` is a separate, already-existing reader (used today by the conversation-fork/resume UI) that does retain real content; no new JSONL parser is needed. |
| Tool-output pruning | Reuse `extractMsgContent`'s existing text-only filter (already excludes `tool_use`/`tool_result` blocks) + a new byte-cap on oversized text blocks (`pruneExcerptText`) | `session/history.go:401-432` (only `type == "text"` content items are concatenated) | Write a new pruning pass that re-parses raw JSONL `tool_use`/`tool_result` blocks | `extractMsgContent` already strips exactly the categories AC-4 names (images, diff/tool-output blobs are non-text content items and are already dropped); duplicating that logic is unjustified generic work per `.claude/rules/interface-pollution-checklist.md` item 5 |
| Summary delivery into the new session | `CreateSessionRequest.prompt` (field 7 — CLI-arg at spawn time) | `proto/session/v1/session.proto:491-494` | `initial_prompt` (field 15 — typed keystrokes after Ready) | `prompt` is delivered once, at process spawn, with no PTY-timing race. `initial_prompt`'s keystroke-typing mechanism is exactly the queue-gated/idle-state-dependent hazard `research/pitfalls.md` §2 warns against for live injection — irrelevant for a genuinely fresh process, but `prompt` remains the more direct of the two for an opening message |
| Session-creation mode | Reuse `SessionType_SESSION_TYPE_DIRECTORY`, no new proto enum value; add `restart_from_session_id` as a plain informational field | `.claude/rules/session-creation-registry.md`'s documented `autonomous` exception ("backend session type is shared but behavior is driven by additional request parameters") | New `SESSION_TYPE_RESTART_WITH_SUMMARY` enum value + full 7-touchpoint mode | The backend lifecycle (worktree/path resolution, no new branch) is identical to an ordinary directory session pointed at the source session's own path (`resolvedPath = sourceInstance.Path`) — only `prompt` content and a lineage field differ. This is the exact shape the registry's own exception clause describes. |
| Trigger surface | RPC (`TriggerHandoffSummary`) callable from (a) an always-available manual button in session detail, (b) later, a RED-`ContextHealth`-gated auto-suggestion (Phase 4, blocked) | `requirements.md` Problem Statement ("has to manually restart... no recovery path" — an *assisted*, not fully automatic, path) | Fully automatic restart with no confirmation once RED fires | Auto-restarting a session out from under a user with no confirmation is a destructive, no-rollback action (echoes the `--resume`-fragility precedent `research/pitfalls.md` §2 names for `session/instance_claude.go`) |
| Frontend polling hook | `useHandoffSummary`, near-verbatim structural copy of `useSessionSummary` | `web-app/src/lib/hooks/useSessionSummary.ts:64-220` | A generic `usePolledEntity<T>` hook parameterized over both summary types | Two call sites (one, after this plan) does not justify a generic abstraction per `.claude/rules/interface-pollution-checklist.md` item 5 — extract only if a third poller appears |
| Frontend UI surface | Small capped list (`HandoffSummarySection`) in the session detail **Info tab**, modeled on `WorkflowHistorySection.tsx` | `research/ux.md` §2's explicit recommendation | A new card-row badge, or a full new "Restart" tab | ux.md §1: card-badge space is reserved for live/ambient state; this is a point-in-time, user-reviewed record. A 9th tab is not justified for a rarely-multiple event type (ux.md §2) |
| `HandoffSummaryStatus` type | String-backed typed const, exact shape of `SessionSummaryStatus` | Existing convention, `session/session_summary_service.go` | Go `int` iota enum (as `context-health-monitoring` chose for `ContextHealthLevel`) | Matches the sibling entity this one is structurally closest to (`SessionSummary`), not the sibling this plan does not touch (`ContextHealth`) — consistency with the pattern actually being mirrored, not a blanket "always use iota" rule |

**No non-standard technology is introduced.** Every mechanism (ent entity + upsert pattern, `headless.PoolClient` dispatch, ConnectRPC read+trigger pair, `sync.Map` dedup, React polling hook, vanilla-extract styling, `WorkflowHistorySection`'s list pattern) already exists in this repo and is being reused, not invented.

---

## Migration Plan

- New ent schema `session/ent/schema/handoff_summary.go` → new `handoff_summaries` SQLite table (ent-managed auto-migration on startup, same as every existing ent entity in this repo — no manual migration script). Regenerate with the **required** flag per `.claude/rules/ent-schema-generation.md`:
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
- New proto fields: `CreateSessionRequest.restart_from_session_id = 28` (`session.proto`), `Session.restarted_from_session_id = 74` (`types.proto`). **Field-number coordination note**: the current highest field actually in the file today is `71` (`workspace_key`), but `context-health-monitoring/implementation/plan.md` (Task 2.1.1a) has *already claimed* `72`/`73` for `context_health`/`context_health_reason` in its own unimplemented plan. Since this plan's Phase 4 explicitly depends on that project landing first (see Unresolved Questions), `74` is chosen deliberately to avoid a collision — **re-verify the actual next-free field number in `types.proto` at implementation time** (`grep -oE "= [0-9]+;" ... | sort -n | tail -1` inside the `Session` message) rather than trusting `74` blindly, in case field numbering has moved further by then. New file `proto/session/v1/handoff_summary.proto` (service + messages, mirrors `session_summary.proto`). Run `make proto-gen` after each proto change, which regenerates both `gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.
- No backfill needed — a session created before this feature ships simply has no `HandoffSummary` row, which `HandoffSummarySection` renders as its explicit empty state.

## Observability Plan

- **Logs**: mirror `SessionSummaryGenerator`'s status-transition log lines exactly, tagged `[HandoffSummary]`: `"[HandoffSummary] PENDING -> GENERATING"`, `"... GENERATING -> READY"`, `"... GENERATING -> ERROR (stage)"` (`session/session_summary_service.go:345,373,478` are the literal templates to copy). Additionally log the restart itself: `log.Info("[HandoffSummary] restart session created", "source_session", sourceID, "new_session", newID)` in the `CreateSession` handler when `restart_from_session_id` is set.
- **Metrics**: none. Single-user local tool, consistent with `context-health-monitoring`'s Observability Plan rationale.
- **Alerts**: none — user-facing advisory action, not an oncall-relevant signal.

## Risk Control

- **Feature flag**: `HandoffSummaryConfig.Enabled` (default `true`). Setting it `false` hides `RestartWithSummaryButton` and makes `TriggerHandoffSummary` return `connect.CodeFailedPrecondition` — a real on/off switch (unlike `ContextHealthConfig`'s threshold-based silencing, there's no natural "very high threshold" analog for a one-shot manual action).
- **Rollback procedure**: standard PR revert. The new `handoff_summaries` table is left empty/unused on revert — no destructive migration, no data loss for any other feature (mirrors `context-health-monitoring`'s "nothing persisted" rollback story, except here the persisted rows themselves are harmless orphans, not recomputed state).
- **Staged rollout**: not applicable (single local binary). Validate with the manual second-instance pattern from `CLAUDE.md` (`PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test`) — never `make install-service` mid-development.

## Unresolved Questions

- [ ] `context-health-monitoring`'s `ContextHealth` proto fields (`context_health`/`context_health_reason`, fields 72-73 on `Session`) and `Instance.SetContextHealth`/`ContextHealthVerdict` **do not exist yet** — that project is itself fully planned but unimplemented. Phase 4's "auto-suggest Restart-with-Summary on RED" story cannot compile or ship until `context-health-monitoring`'s Phase 1 (signal) and Phase 2 (proto) land. This blocks **Story 4.1** only — every other story in this plan (manual trigger, generation pipeline, RPC, restart-session creation, Info-tab list) has no dependency on that project and can ship independently. Owner: whoever implements this plan next; re-check `context-health-monitoring`'s status before starting Phase 4.
- [ ] Is the Hermes-cited "~12k token" ceiling for `HandoffSummaryConfig.MaxMiddleExcerptTokens` a safe default without local calibration against this codebase's own long-running sessions, or does it need tuning? Blocks nothing structurally (it's a config default, changeable without a code change) but affects out-of-the-box summary quality. Owner: implementer, revisit after first real long-session use (mirrors `context-health-monitoring`'s identical "Phase 6 manual review" deferral for its own heuristic thresholds).
- [ ] Does `headless.Pool`'s dispatch path (which shells out to the user's own `claude` CLI via `pool.CallBlocking`, not `server/services/anthropic_client.go`'s HTTP client) have the same OAuth-only/no-API-key fail-closed gap `research/pitfalls.md` §3 flagged for `AnthropicAIClient`? If the pool always has a usable session-scoped credential (since it invokes the user's own already-authenticated `claude` CLI), this gap may not apply here at all — unconfirmed. Blocks nothing (the generator already degrades to an `ERROR` row + fallback text on any failure, matching `SessionSummaryGenerator`'s existing `narrativeFallbackLLMFailure` pattern), but affects whether "Restart with Summary" reliably works for every auth configuration. Owner: implementer, verify empirically during Phase 6 by testing under an OAuth-only (no `ANTHROPIC_API_KEY`) session.

## Dependency Visualization

```
                    Phase 1 — Backend summary generation
  ┌────────────────────────────────────────────────────────────────────┐
  │ 1.1 HandoffSummary ent schema + generate --feature sql/upsert       │
  │        (session/ent/schema/handoff_summary.go)                     │
  │                          │                                         │
  │ 1.2 HandoffSummaryConfig (config/types.go, config/config.go)        │
  │                          │                                         │
  │ 1.3 buildTranscriptWindow + pruneExcerptText + SummaryBudget        │
  │        (session/handoff_summary_excerpt.go)                        │
  │                          │                                         │
  │ 1.4 GenerateHandoffSummary + REFERENCE-ONLY prompt                  │
  │        (session/headless/features.go)          ◄────────┐          │
  │                          │                               │          │
  │ 1.5 HandoffSummaryGenerator.GenerateAndPersist (needs 1.1,1.3,1.4)  │
  │        (session/handoff_summary_service.go)                        │
  └──────────────────────────┬───────────────────────────────────────┘
                             │
                    Phase 2 — ConnectRPC surface
  ┌──────────────────────────▼───────────────────────────────────────┐
  │ 2.1 proto: handoff_summary.proto (service+messages) +              │
  │     CreateSessionRequest.restart_from_session_id=28 +              │
  │     Session.restarted_from_session_id=74  →  make proto-gen        │
  │                          │                                         │
  │ 2.2 HandoffSummaryService (Get/Trigger) + dependencies.go wiring   │
  │        (server/services/handoff_summary_service.go)                │
  │                          │                                         │
  │ 2.3 CreateSession: resolve path from source session + set          │
  │     restarted_from_session_id on the new Session                   │
  │        (server/services/session_service.go)                        │
  └──────────────────────────┬───────────────────────────────────────┘
                             │  (TS bindings emitted by 2.1)
                    Phase 3 — Frontend (manual trigger, ships now)
  ┌──────────────────────────▼───────────────────────────────────────┐
  │ 3.1 useHandoffSummary hook                                         │
  │                          │                                         │
  │ 3.2 RestartWithSummaryButton (trigger → poll → create session)    │
  │                          │                                         │
  │ 3.3 HandoffSummarySection in Info tab                              │
  └──────────────────────────┬───────────────────────────────────────┘
                             │
                    Phase 4 — Auto-trigger on ContextHealth RED (BLOCKED)
  ┌──────────────────────────▼───────────────────────────────────────┐
  │ 4.1 Wire into (future) publishContextHealth level-transition hook  │
  │     — depends on context-health-monitoring Phase 1+2 shipping      │
  │     first (see Unresolved Questions). Not started until then.      │
  └────────────────────────────────────────────────────────────────────┘

Phases 1 and 2.1 (proto) may start in parallel; 2.2/2.3 depend on 2.1's generated
bindings. Phase 3 depends on 2.1's generated TS bindings. Phase 4 is gated on an
external, unimplemented project and is not scheduled by this plan.
```

---

## Phase 1: Backend summary generation

### Epic 1.1: Persistence

**Goal**: A durable `HandoffSummary` row per source session, following `SessionSummary`'s proven schema shape exactly.

#### Story 1.1.1: `HandoffSummary` ent schema

**As a** developer, **I want** a `HandoffSummary` entity independent of the live `Session` row, **so that** a generated summary survives even if the source session is later archived/deleted.

**Acceptance Criteria**:
- The schema has no `Edges()` — `session_id` is a plain unique string field, not a required edge back to `Session`.
  - *Given* the generated ent code, *When* a `Session` row is deleted, *Then* its corresponding `HandoffSummary` row (if any) is untouched (no cascading delete, no FK constraint error).
- `status` defaults to `"pending"` and `session_id` is unique (one row per source session, upserted on retry — matching `SessionSummary`'s `OnConflictColumns(sessionsummary.FieldSessionID)` pattern).
  - *Given* a fresh `HandoffSummary.Create()` with no explicit status, *When* the row is queried back, *Then* `row.Status == "pending"`.

**Files**: `session/ent/schema/handoff_summary.go` (new)

##### Task 1.1.1a: Create the ent schema (~4 min)
- New file `session/ent/schema/handoff_summary.go`, package `schema`, doc comment mirroring `session/ent/schema/session_summary.go`'s doc comment (no `Edges()`, survives `Session` deletion by construction).
- Fields: `id` (String, Unique, NotEmpty, Immutable), `session_id` (String, NotEmpty, Unique), `session_title` (String, Optional), `status` (String, Default `"pending"`), `active_task` (Text, Optional — the short "## Active Task" excerpt, denormalized for list display without parsing `summary_text`), `summary_text` (Text, Optional — the full rendered, REFERENCE-ONLY-prefixed handoff text, exactly what gets passed as `CreateSessionRequest.prompt`), `middle_messages_summarized` (Int, Default 0), `generation_started_at` (Time, Optional, Nillable), `generated_at` (Time, Optional, Nillable), `error_stage` (String, Optional), `error_message` (Text, Optional).
- `session_id` needs no separate `index.Fields("session_id")` entry — its `Unique()` field definition already covers lookups (confirmed: `session_summary.go`'s own `Indexes()` comment at lines 91-93 states this explicitly and only indexes `status`, line 98). Add a matching `func (HandoffSummary) Indexes() []ent.Index { return []ent.Index{index.Fields("status")} }` for the same reason (status is queried/filtered but not unique).
- Files: `session/ent/schema/handoff_summary.go`

##### Task 1.1.1b: Regenerate ent code (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md` — **not** the flag-less form).
- Run `go build ./...` to confirm the generated `session/ent/handoffsummary/`, `session/ent/handoffsummary_create.go`, etc. compile.
- Files: `session/ent/**` (generated, commit all of it together per the rule's stated workflow)

---

### Epic 1.2: Configuration

**Goal**: A feature flag and a summary-sizing ceiling, following `CapacityConfig`'s idiom exactly.

#### Story 1.2.1: `HandoffSummaryConfig`

**As a** stapler-squad user, **I want** the feature toggleable and the summary budget tunable in `config.json`, **so that** I can disable it or raise the excerpt ceiling without rebuilding.

**Acceptance Criteria**:
- `Enabled` defaults to `true`; `MaxMiddleExcerptTokens` defaults to `12000` when unset or `<= 0`.
  - *Given* an empty `config.json`, *When* `LoadConfig()` runs, *Then* `cfg.HandoffSummary.Enabled == true` and `cfg.HandoffSummary.MaxMiddleExcerptTokens == 12000`.
- An explicit `{"handoff_summary": {"enabled": false}}` disables the feature without touching the budget.
  - *Given* `config.json` containing `{"handoff_summary": {"enabled": false}}`, *When* `LoadConfigFromPath` runs, *Then* `cfg.HandoffSummary.Enabled == false` and `cfg.HandoffSummary.MaxMiddleExcerptTokens == 12000` (default, since it was absent).

**Files**: `config/types.go`, `config/config.go`, `config/config_test.go`

##### Task 1.2.1a: Add `HandoffSummaryConfig` struct + `OrDefault` (~4 min)
- In `config/types.go`, immediately after `CapacityConfigOrDefault` (ends line ~320), add:
  ```go
  // HandoffSummaryConfig holds configuration for the restart-with-handoff-summary feature.
  type HandoffSummaryConfig struct {
      // Enabled toggles the feature. Default: true.
      Enabled bool `json:"enabled,omitempty"`
      // MaxMiddleExcerptTokens caps the proportional middle-transcript excerpt fed
      // to the summarizer. Default: 12000 (Hermes-cited ceiling, requirements.md).
      MaxMiddleExcerptTokens int `json:"max_middle_excerpt_tokens,omitempty"`
  }
  ```
- Note: `Enabled` uses `omitempty`, so an explicit `false` still round-trips correctly only because `HandoffSummaryConfigOrDefault` must NOT treat zero-value `false` as "unset" — track "was the key present" via a pointer if `omitempty` proves ambiguous in testing (Task 1.2.1c must assert this explicitly), otherwise default `Enabled` at `DefaultConfig()` construction time only, never inside `OrDefault`.
- Add `func (c HandoffSummaryConfig) HandoffSummaryConfigOrDefault() HandoffSummaryConfig` applying the default only to `MaxMiddleExcerptTokens` (`<= 0` → `12000`); leave `Enabled` untouched (its `DefaultConfig()`-time default is set once at construction, per the note above).
- Files: `config/types.go`

##### Task 1.2.1b: Register on `Config` (~3 min)
- In `config/config.go`, add `HandoffSummary HandoffSummaryConfig \`json:"handoff_summary,omitempty"\`` to the `Config` struct immediately after `Capacity` (line ~339).
- In `DefaultConfig()` (line ~463), add `cfg.HandoffSummary = HandoffSummaryConfig{Enabled: true}.HandoffSummaryConfigOrDefault()`.
- In `LoadConfigFromPath` (line ~916), add `cfg.HandoffSummary = cfg.HandoffSummary.HandoffSummaryConfigOrDefault()`.
- Files: `config/config.go`

##### Task 1.2.1c: Tests (~4 min)
- In `config/config_test.go`, add `TestHandoffSummaryConfigOrDefault_AppliesDefaultToZeroBudget` and `TestLoadConfig_HandoffSummaryExplicitlyDisabled_StaysDisabled` covering the two Given-When-Then cases above — the second test is the one that catches the `omitempty`/zero-value ambiguity flagged in Task 1.2.1a.
- Files: `config/config_test.go`

---

### Epic 1.3: Transcript excerpt assembly

**Goal**: Turn a source session's real transcript content into a head/middle/tail-split, pruned, budget-capped excerpt — the input to summarization.

#### Story 1.3.1: `buildTranscriptWindow`

**As** the handoff-summary generator, **I want** the source session's messages split into head/middle/tail, **so that** the summarization prompt can preserve the original task and current state verbatim and compress only the middle.

**Acceptance Criteria**:
- The first 2 and last 2 messages are carried verbatim into `Head`/`Tail`; everything else goes into `Middle`.
  - *Given* a conversation of 10 `ClaudeConversationMessage`s (chronological), *When* `buildTranscriptWindow(messages)` runs, *Then* `len(window.Head) == 2`, `len(window.Tail) == 2`, `len(window.Middle) == 6`, and `window.Head` equals messages 1-2 and `window.Tail` equals messages 9-10 exactly (no reordering).
- A short conversation (≤4 messages) has an empty `Middle` rather than overlapping Head/Tail.
  - *Given* a conversation of exactly 4 messages, *When* `buildTranscriptWindow` runs, *Then* `window.Head` is messages 1-2, `window.Tail` is messages 3-4, and `window.Middle` is empty (no message appears in more than one of the three slices).

**Files**: `session/handoff_summary_excerpt.go` (new), `session/handoff_summary_excerpt_test.go` (new)

##### Task 1.3.1a: Implement `TranscriptWindow` + `buildTranscriptWindow` (~5 min)
- New file `session/handoff_summary_excerpt.go`, package `session`.
- Define `const headMessageCount = 2`, `const tailMessageCount = 2`.
- Define `type TranscriptWindow struct { Head, Middle, Tail []ClaudeConversationMessage }`.
- Implement `func buildTranscriptWindow(messages []ClaudeConversationMessage) TranscriptWindow`: if `len(messages) <= headMessageCount+tailMessageCount`, split evenly between Head/Tail with no overlap (first half → Head, second half → Tail, Middle empty); otherwise `Head = messages[:headMessageCount]`, `Tail = messages[len(messages)-tailMessageCount:]`, `Middle = messages[headMessageCount:len(messages)-tailMessageCount]`.
- Files: `session/handoff_summary_excerpt.go`

##### Task 1.3.1b: Tests (~4 min)
- New file `session/handoff_summary_excerpt_test.go`.
- `TestBuildTranscriptWindow_SplitsHeadMiddleTailForLongConversation` and `TestBuildTranscriptWindow_ShortConversationHasEmptyMiddle` per the Given-When-Then above.
- Files: `session/handoff_summary_excerpt_test.go`

#### Story 1.3.2: `pruneExcerptText` + `SummaryBudget`

**As** the handoff-summary generator, **I want** oversized text blocks truncated and the middle excerpt capped to a budget, **so that** the prompt stays within `MaxMiddleExcerptTokens` regardless of session length.

**Acceptance Criteria**:
- A single message's content longer than `maxExcerptMessageBytes` is truncated with a visible marker.
  - *Given* a `ClaudeConversationMessage` whose `Content` is 20,000 bytes and `maxExcerptMessageBytes == 4000`, *When* `pruneExcerptText(msg)` runs, *Then* the returned content is exactly 4000 bytes plus a trailing `"... [truncated]"` marker.
- The middle excerpt as a whole is capped at the configured token budget (approximated as bytes/4).
  - *Given* a `TranscriptWindow.Middle` whose concatenated pruned content totals 60,000 bytes and `SummaryBudget{MiddleExcerptMaxBytes: 48000}` (12000 tokens × 4 bytes/token), *When* `applySummaryBudget(middle, budget)` runs, *Then* the returned excerpt string is ≤ 48000 bytes, and truncation happens at a message boundary (never mid-message) so no single message is split across the cutoff — earliest-dropped-first (oldest middle messages are cut first, keeping the messages closest to the tail, which are more recent and more relevant).

**Files**: `session/handoff_summary_excerpt.go`, `session/handoff_summary_excerpt_test.go`

##### Task 1.3.2a: Implement `pruneExcerptText` (~3 min)
- In `session/handoff_summary_excerpt.go`, add `const maxExcerptMessageBytes = 4000` and `func pruneExcerptText(content string) string`: if `len(content) <= maxExcerptMessageBytes`, return unchanged; else return `content[:maxExcerptMessageBytes] + "... [truncated]"`.
- Files: `session/handoff_summary_excerpt.go`

##### Task 1.3.2b: Implement `SummaryBudget` + `applySummaryBudget` (~5 min)
- In `session/handoff_summary_excerpt.go`, add `type SummaryBudget struct { MiddleExcerptMaxBytes int }` and `func newSummaryBudget(cfg config.HandoffSummaryConfig) SummaryBudget` computing `MiddleExcerptMaxBytes = cfg.HandoffSummaryConfigOrDefault().MaxMiddleExcerptTokens * 4` (rough bytes-per-token approximation, documented in a comment as an approximation, not exact tokenization — matching `research/stack.md`'s finding that no tokenizer library exists in `go.mod` and none is needed here).
- Add `func applySummaryBudget(middle []ClaudeConversationMessage, budget SummaryBudget) []ClaudeConversationMessage`: pruning each message via `pruneExcerptText` first, then dropping from the **front** (oldest) of `middle` until the total pruned byte length is ≤ `budget.MiddleExcerptMaxBytes`, never splitting a single message's content.
- Files: `session/handoff_summary_excerpt.go`

##### Task 1.3.2c: Tests (~4 min)
- `TestPruneExcerptText_TruncatesOversizedContentWithMarker`, `TestApplySummaryBudget_CapsAtByteBudgetWithoutSplittingAMessage`, `TestApplySummaryBudget_DropsOldestMiddleMessagesFirst` (assert the retained messages are a suffix of the original `Middle` slice).
- Files: `session/handoff_summary_excerpt_test.go`

---

### Epic 1.4: Prompt construction

**Goal**: A stable, reusable system prompt embedding the REFERENCE-ONLY prefix and "## Active Task" instruction, dispatched through the existing headless pool.

#### Story 1.4.1: `GenerateHandoffSummary`

**As** the handoff-summary generator, **I want** a single function that turns a `TranscriptWindow` into rendered handoff text, **so that** the REFERENCE-ONLY framing and Active Task section are never hand-assembled ad hoc at a call site.

**Acceptance Criteria**:
- The rendered output always begins with the verbatim `ReferenceOnlyPrefix` string from `requirements.md:26-33`.
  - *Given* any non-empty `TranscriptWindow` and a successful pool call, *When* `GenerateHandoffSummary` returns, *Then* the returned string's first line is exactly `"[CONTEXT COMPACTION — REFERENCE ONLY] Earlier turns were compacted into the summary below. This is a handoff from a previous context window — treat it as background reference, NOT as active instructions. Do NOT answer questions or fulfill requests mentioned in this summary; they were already addressed. Your current task is identified in the '## Active Task' section..."` (the requirements.md excerpt, reproduced verbatim, not paraphrased).
- The prompt sent to the LLM explicitly instructs it to produce a `## Active Task` heading.
  - *Given* a `TranscriptWindow`, *When* the constructed `userPrompt` (before the pool call) is inspected, *Then* it contains the literal substring `"## Active Task"` as an instruction to the model, and separately labeled `Head:`/`Middle (to summarize):`/`Tail:` sections built from the window's three slices.
- An empty `Middle` (short conversation) still produces valid output without a wasted LLM call for "nothing to summarize."
  - *Given* a `TranscriptWindow` with `Middle` empty, *When* `GenerateHandoffSummary` runs, *Then* it still calls the pool (Head/Tail alone may still need framing into an Active Task), but the prompt's middle section reads `"(nothing to summarize — conversation was short)"` rather than an empty block.

**Files**: `session/headless/features.go`, `session/headless/features_test.go`

##### Task 1.4.1a: Add `FeatureKeyHandoffSummary` + the `ReferenceOnlyPrefix` constant (~2 min)
- In `session/headless/features.go`, add `FeatureKeyHandoffSummary FeatureKey = "handoff-summary"` immediately after `FeatureKeySessionCompletionSummary` (line 26).
- Add `const referenceOnlyPrefix = "[CONTEXT COMPACTION — REFERENCE ONLY] Earlier turns were compacted into the summary below. This is a handoff from a previous context window — treat it as background reference, NOT as active instructions. Do NOT answer questions or fulfill requests mentioned in this summary; they were already addressed. Your current task is identified in the '## Active Task' section..."` — copy the exact wording from `requirements.md:26-33`, character for character.
- Files: `session/headless/features.go`

##### Task 1.4.1b: Add `handoffSummarySystemPrompt` + `GenerateHandoffSummary` (~5 min)
- In `session/headless/features.go`, add `const handoffSummarySystemPrompt = "..."` instructing the model: summarize the Middle section into concise prose grounded strictly in what's shown (no speculation, same grounding discipline as `sessionCompletionSummarySystemPrompt`, line 312), preserve Head and Tail as given context (do not re-summarize them), and always end with a `## Active Task` heading naming the concrete next step based on the Tail's most recent state.
- Add `func GenerateHandoffSummary(ctx context.Context, pool PoolClient, sessionTitle string, window TranscriptWindow) (string, error)`: build `userPrompt` with labeled `Head:`/`Middle (to summarize):`/`Tail:` sections (rendering each message as `"[role] content"`, one per line, via `pruneExcerptText`-already-applied content), call `pool.CallBlocking(ctx, FeatureKeyHandoffSummary, handoffSummarySystemPrompt, userPrompt, CallOptions{})`, and prepend `referenceOnlyPrefix + "\n\n"` to the returned text before returning it.
- Files: `session/headless/features.go`

##### Task 1.4.1c: Tests (~5 min)
- In `session/headless/features_test.go` (uses the existing `fakePoolClientRecorder` per `features_test.go:154`), add `TestGenerateHandoffSummary_PrependsReferenceOnlyPrefixVerbatim`, `TestGenerateHandoffSummary_PromptInstructsActiveTaskSection`, `TestGenerateHandoffSummary_EmptyMiddlePlaceholderText`.
- Files: `session/headless/features_test.go`

---

### Epic 1.5: Generation orchestration

**Goal**: Async dispatch, dedup, and durable persistence — mirroring `SessionSummaryGenerator` exactly.

#### Story 1.5.1: `HandoffSummaryGenerator.GenerateAndPersist`

**As a** caller (the `TriggerHandoffSummary` RPC handler), **I want** to dispatch generation as a detached goroutine that dedupes concurrent triggers and always lands in a terminal `READY`/`ERROR` state, **so that** the frontend can poll `GetHandoffSummary` and always eventually see a resolved row.

**Acceptance Criteria**:
- A second concurrent call for the same session while one is already in flight is dropped (no duplicate row, no duplicate LLM call).
  - *Given* `GenerateAndPersist(ctx, "sess-1", ...)` already running (blocked mid-pool-call), *When* a second `GenerateAndPersist(ctx, "sess-1", ...)` is called, *Then* the second call returns immediately without writing any row and without calling the pool (verified via a fake `PoolClient` call-count assertion).
- On success, the row transitions `PENDING → GENERATING → READY` with `summary_text` and `active_task` populated.
  - *Given* a source session with a real conversation file and a successful pool call, *When* `GenerateAndPersist` completes, *Then* `FindRowBySessionID` returns a row with `Status == "ready"`, non-empty `SummaryText` starting with the `ReferenceOnlyPrefix`, and non-empty `GeneratedAt`.
- On pool failure, the row lands in `ERROR` with a stage and message, not stuck in `GENERATING`.
  - *Given* a fake `PoolClient` that returns an error, *When* `GenerateAndPersist` completes, *Then* `FindRowBySessionID` returns a row with `Status == "error"`, `ErrorStage == "generation"`, and `ErrorMessage` equal to the pool error's text.
- A panic inside the pipeline is recovered and does not crash the process or leave the dedup guard held.
  - *Given* a fake `PoolClient` whose `CallBlocking` panics, *When* `GenerateAndPersist` runs (as a goroutine, per its calling convention), *Then* the panic is recovered (test asserts `recover()` behavior via a wrapped test harness, mirroring `TestGenerateAndPersist_PanicRecovered` if one exists for `SessionSummaryGenerator`, or a new equivalent test), the process does not crash, and a subsequent call for the same session is not blocked by a permanently-held guard.

**Files**: `session/handoff_summary_service.go` (new), `session/handoff_summary_service_test.go` (new)

##### Task 1.5.1a: Define `HandoffSummaryGenerator` struct + constructor (~4 min)
- New file `session/handoff_summary_service.go`, package `session`.
- Define `HandoffSummaryStatus` string type with consts `HandoffSummaryStatusPending = "pending"`, `...Generating = "generating"`, `...Ready = "ready"`, `...Error = "error"` (exact shape of `SessionSummaryStatus*` consts referenced in `session/session_summary_service.go`).
- Define `type HandoffSummaryGenerator struct { entClient *ent.Client; pool headless.PoolClient; inFlight sync.Map }`.
- Add `func NewHandoffSummaryGenerator(entClient *ent.Client, pool headless.PoolClient) *HandoffSummaryGenerator`.
- Add `tryAcquire`/`release` methods, copied verbatim in shape from `SessionSummaryGenerator.tryAcquire` (`session/session_summary_service.go:159-176`).
- Files: `session/handoff_summary_service.go`

##### Task 1.5.1b: Implement `FindRowBySessionID` (~2 min)
- Mirror `SessionSummaryGenerator.FindRowBySessionID` (`session/session_summary_service.go:101-110`) exactly, querying `g.entClient.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionID)).Only(ctx)`, wrapping not-found as `session.ErrNotFound`.
- Files: `session/handoff_summary_service.go`

##### Task 1.5.1c: Implement `GenerateAndPersist` (~5 min)
- Signature: `func (g *HandoffSummaryGenerator) GenerateAndPersist(ctx context.Context, sourceSessionID, sourceSessionTitle string)`.
- `release, ok := g.tryAcquire(sourceSessionID); if !ok { return }; defer release()`.
- `defer func() { recover() ... }()` panic guard, mirroring `session/session_summary_service.go:280-284` exactly (log-and-swallow, never crash the process).
- Interim upsert to `GENERATING` (mirrors lines 329-344, using `HandoffSummary.Create()...OnConflictColumns(handoffsummary.FieldSessionID).Update(...)`).
- Call `session.NewClaudeSessionHistoryFromClaudeDir()` (`session/history.go:69`), then `.GetMessagesFromConversationFile(sourceSessionID, 0)` (0 = all messages, `session/history.go:571-581`) to get the real transcript content. On error, upsert an `ERROR` row (stage `"transcript"`) and return.
- `window := buildTranscriptWindow(messages)`; `window.Middle = applySummaryBudget(window.Middle, newSummaryBudget(config.LoadConfig().HandoffSummary))`.
- `summaryText, err := headless.GenerateHandoffSummary(ctx, g.pool, sourceSessionTitle, window)`. On error, upsert `ERROR` (stage `"generation"`) and return.
- Extract `activeTask` from `summaryText` (substring after the `"## Active Task"` heading, best-effort — empty string if the heading is missing, never an error).
- Final upsert to `READY` with `SummaryText`, `ActiveTask`, `MiddleMessagesSummarized: len(window.Middle)`, `GeneratedAt: time.Now()`.
- Files: `session/handoff_summary_service.go`

##### Task 1.5.1d: Tests (~5 min)
- New file `session/handoff_summary_service_test.go`, using a fake `headless.PoolClient` (mirroring `fakePoolClientRecorder`) and an in-memory ent client (mirroring `session/session_summary_service_test.go`'s setup, if that pattern exists — otherwise the standard `enttest.Open` in-memory sqlite pattern used elsewhere in `session/ent/`).
- `TestGenerateAndPersist_DedupsConcurrentCallsForSameSession`, `TestGenerateAndPersist_SuccessTransitionsToReadyWithSummaryText`, `TestGenerateAndPersist_PoolFailureTransitionsToError`, `TestGenerateAndPersist_PanicIsRecoveredAndGuardIsReleased`.
- Files: `session/handoff_summary_service_test.go`

---

## Phase 2: ConnectRPC surface

### Epic 2.1: Proto

**Goal**: A new service (mirroring `SessionSummaryService`) plus two small additive fields for restart lineage.

#### Story 2.1.1: `handoff_summary.proto`

**As a** frontend client, **I want** `GetHandoffSummary`/`TriggerHandoffSummary` RPCs and a `HandoffSummaryProto` message, **so that** `useHandoffSummary` has a typed contract to poll.

**Acceptance Criteria**:
- The proto compiles and generates both Go and TS bindings with no field-number conflicts.
  - *Given* the new file `proto/session/v1/handoff_summary.proto`, *When* `make proto-gen` runs, *Then* `gen/proto/go/session/v1/handoff_summary.pb.go`, `gen/proto/go/session/v1/handoff_summary_grpc.pb.go`/`sessionv1connect` bindings, and `web-app/src/gen/session/v1/handoff_summary_pb.ts` are all produced with no errors.
- `CreateSessionRequest.restart_from_session_id` and `Session.restarted_from_session_id` occupy the next free field numbers with no collision.
  - *Given* `CreateSessionRequest`'s highest field in use is `27` (`alias_name`) and `Session`'s highest field actually present in the file is `71` (`workspace_key`, with `72`/`73` already reserved-but-unshipped by `context-health-monitoring`'s plan), *When* the new fields are added at `28` and `74` respectively, *Then* `make proto-gen && make build` succeeds with no field-number collision against either the current file or that sibling plan's claimed numbers.

**Files**: `proto/session/v1/handoff_summary.proto` (new), `proto/session/v1/session.proto`, `proto/session/v1/types.proto`

##### Task 2.1.1a: Create `handoff_summary.proto` (~5 min)
- New file, structurally mirroring `proto/session/v1/session_summary.proto:1-87` exactly: `service HandoffSummaryService { rpc GetHandoffSummary(...); rpc TriggerHandoffSummary(...); }`, `message HandoffSummaryProto { session_id, session_title, HandoffSummaryStatus status, string active_task, string summary_text, int32 middle_messages_summarized, string error_message, string error_stage, google.protobuf.Timestamp generated_at }`, plus a `HandoffSummaryStatus` enum (`UNSPECIFIED/PENDING/GENERATING/READY/ERROR`, mirroring `SessionSummaryStatus`'s shape — check `types.proto` for that enum's exact definition and copy its structure).
- `GetHandoffSummaryRequest{session_id}` / `GetHandoffSummaryResponse{optional HandoffSummaryProto summary}`; `TriggerHandoffSummaryRequest{session_id}` / `TriggerHandoffSummaryResponse{HandoffSummaryProto summary}`.
- Files: `proto/session/v1/handoff_summary.proto`

##### Task 2.1.1b: Add `restart_from_session_id` + `restarted_from_session_id` (~3 min)
- In `proto/session/v1/session.proto`, add `string restart_from_session_id = 28;` to `CreateSessionRequest`, doc-commented: "Optional: source session this restart is seeded from (lineage only — does not change session-creation behavior; see .claude/rules/session-creation-registry.md's autonomous exception)."
- In `proto/session/v1/types.proto`, add `string restarted_from_session_id = 74;` to `Session`, doc-commented "Empty unless this session was created via Restart-with-Summary." **Before adding**, re-run `grep -oE "= [0-9]+;" ...` over the `Session` message (see Migration Plan's coordination note) to confirm `74` is still free — if `context-health-monitoring` has shipped by the time this task runs, fields `72`/`73` will already be in use for `context_health`/`context_health_reason`, which is expected and fine; if it has NOT shipped, use the next number after the actual current highest instead of hardcoding `74`.
- Run `make proto-gen`.
- Files: `proto/session/v1/session.proto`, `proto/session/v1/types.proto`

---

### Epic 2.2: `HandoffSummaryService` + wiring

**Goal**: Expose the generator over ConnectRPC and wire it at server startup, mirroring `SessionSummaryService`/`dependencies.go`'s existing pattern.

#### Story 2.2.1: `GetHandoffSummary` / `TriggerHandoffSummary` handlers

**As a** frontend client, **I want** the same read/trigger contract `SessionSummaryService` already offers, **so that** `useHandoffSummary` can reuse `useSessionSummary`'s exact polling shape.

**Acceptance Criteria**:
- `GetHandoffSummary` returns `summary: nil` (not an error) when no row exists yet.
  - *Given* a `session_id` with no `HandoffSummary` row, *When* `GetHandoffSummary` is called, *Then* the response has `Summary == nil` and no error.
- `TriggerHandoffSummary` dispatches generation asynchronously and returns immediately with the current (possibly `GENERATING`) row.
  - *Given* a session with no existing row, *When* `TriggerHandoffSummary` is called, *Then* the RPC returns within milliseconds with `Summary.Status == GENERATING` (the interim upsert, not blocking on the LLM call), and a subsequent `GetHandoffSummary` poll eventually observes `READY` or `ERROR`.
- `TriggerHandoffSummary` returns `CodeFailedPrecondition` when `HandoffSummaryConfig.Enabled == false`.
  - *Given* `config.HandoffSummary.Enabled == false`, *When* `TriggerHandoffSummary` is called, *Then* the RPC returns `connect.CodeFailedPrecondition` and no row is created or dispatched.

**Files**: `server/services/handoff_summary_service.go` (new), `server/services/handoff_summary_service_test.go` (new), `server/dependencies.go`

##### Task 2.2.1a: Implement `HandoffSummaryService` (~5 min)
- New file `server/services/handoff_summary_service.go`, structurally mirroring `server/services/session_summary_service.go:1-90`'s `GetSessionSummary`/`RegenerateSessionSummary` shape: `type HandoffSummaryService struct { generator *session.HandoffSummaryGenerator }`, `NewHandoffSummaryService`, `GetHandoffSummary` (query + `toHandoffSummaryProto`), `TriggerHandoffSummary` (check `config.LoadConfig().HandoffSummary.Enabled` first; if disabled, return `connect.CodeFailedPrecondition`; else `go s.generator.GenerateAndPersist(context.Background(), req.Msg.SessionId, sessionTitle)` — **confirmed**: `RegenerateSessionSummary` uses exactly this pattern, `context.Background()` not the request `ctx`, with the comment "Critical: detached goroutine with context.Background(), never the [request context]" at `server/services/session_summary_service.go:153,157` — mirror it verbatim, do not pass `ctx`).
- Add `+api: GetHandoffSummary` / `+api: TriggerHandoffSummary` markers per `.claude/rules/feature-registry.md`.
- Files: `server/services/handoff_summary_service.go`

##### Task 2.2.1b: `toHandoffSummaryProto` mapping (~2 min)
- Small pure function converting `*ent.HandoffSummary` → `*sessionv1.HandoffSummaryProto`, mirroring `session_summary_service.go`'s equivalent (name TBD by grep — `toSessionSummaryProto`, confirmed referenced at line ~72 above).
- Files: `server/services/handoff_summary_service.go`

##### Task 2.2.1c: Wire into `server/dependencies.go` (~4 min)
- Immediately after the existing `sessionSummaryGenerator` construction block (`server/dependencies.go:588-600`), add the equivalent for `HandoffSummaryGenerator`: `var handoffSummaryGenerator *session.HandoffSummaryGenerator; if entClient := storage.GetEntClient(); entClient != nil { handoffSummaryGenerator = session.NewHandoffSummaryGenerator(entClient, headlessPool) }`.
- Add a `HandoffSummaryGenerator *session.HandoffSummaryGenerator` field to both `Dependencies` structs that already carry `SessionSummaryGenerator` (`server/dependencies.go:101,438`), and set it at both of `SessionSummaryGenerator`'s existing assignment sites (`:140,1289`), so `server.go` can read `deps.HandoffSummaryGenerator` the same way it reads `deps.SessionSummaryGenerator`.
- Register the new service with the ConnectRPC mux at the same site `SessionSummaryService` is registered: `server/server.go:373`, `sessionSummaryService := services.NewSessionSummaryService(deps.SessionSummaryGenerator, deps.SessionService)` (followed by its mux-handler registration a few lines below) — add the mirrored `handoffSummaryService := services.NewHandoffSummaryService(deps.HandoffSummaryGenerator)` plus its handler registration immediately after.
- Files: `server/dependencies.go`, `server/server.go`

##### Task 2.2.1d: Tests (~5 min)
- New file `server/services/handoff_summary_service_test.go`, mirroring `server/services/session_summary_service_test.go`'s setup.
- `TestGetHandoffSummary_ReturnsNilWhenNoRowExists`, `TestTriggerHandoffSummary_DispatchesAsyncAndReturnsGeneratingRow`, `TestTriggerHandoffSummary_ReturnsFailedPreconditionWhenDisabled`.
- Files: `server/services/handoff_summary_service_test.go`

---

### Epic 2.3: Restart-session creation

**Goal**: `CreateSession` accepts `restart_from_session_id` and derives the new session's path from the source session, without adding a new `SessionType`.

#### Story 2.3.1: Path derivation + lineage field

**As a** user restarting a degrading session, **I want** the new session to start in the same working directory as the source session, **so that** I don't have to re-specify the path by hand.

**Acceptance Criteria**:
- When `restart_from_session_id` is set and `path` is empty, the new session's path is resolved from the source session's live/persisted `Path`.
  - *Given* a source session with `Path == "/home/user/repo"` and a `CreateSessionRequest{RestartFromSessionId: "<source-id>", Path: "", SessionType: DIRECTORY, Prompt: "<handoff text>"}`, *When* `CreateSession` runs, *Then* the created session's `Path == "/home/user/repo"` and its `RestartedFromSessionId == "<source-id>"`.
- An explicit `path` on the request still wins over the derived one (never silently overridden).
  - *Given* the same source session but `CreateSessionRequest.Path == "/home/user/other-repo"` explicitly set, *When* `CreateSession` runs, *Then* the created session's `Path == "/home/user/other-repo"` (explicit request value preserved).
- A `restart_from_session_id` pointing at a session that no longer exists returns a clear error, not a silent fallback.
  - *Given* `RestartFromSessionId: "does-not-exist"`, *When* `CreateSession` runs, *Then* it returns `connect.CodeNotFound` with a message naming the missing source session.

**Files**: `server/services/session_service.go`

##### Task 2.3.1a: Resolve path from `restart_from_session_id` (~4 min)
- In `server/services/session_service.go`'s `CreateSession` (starts line 1260), immediately before the existing "Resolve GitHub URLs to local paths" block (line ~1327), add: if `req.Msg.RestartFromSessionId != ""` and `resolvedPath == ""`, look up the source instance via `s.FindLiveInstance(req.Msg.RestartFromSessionId)` (falling back to `s.storage.ListInstanceData()` lookup by ID if not live, matching the existing-session-lookup pattern used elsewhere in this file for `fork_source_id`), and set `resolvedPath = source.Path`. Return `connect.CodeNotFound` if no source session is found by either path.
- Files: `server/services/session_service.go`

##### Task 2.3.1b: Set `restarted_from_session_id` on the created session (~2 min)
- Immediately after the new `Instance` is constructed (grep the existing `fork_source_id`-adjacent field-setting block for the exact insertion point — likely near where other lineage-ish request fields are copied onto the new `Instance`), set `inst.RestartedFromSessionID = req.Msg.RestartFromSessionId` and thread it through `InstanceToProto` (`server/adapters/instance_adapter.go`) to populate `Session.restarted_from_session_id`.
- Files: `server/services/session_service.go`, `session/instance.go` (new field), `server/adapters/instance_adapter.go`

##### Task 2.3.1c: Tests (~5 min)
- `TestCreateSession_RestartFromSessionId_DerivesPathFromSource`, `TestCreateSession_RestartFromSessionId_ExplicitPathWins`, `TestCreateSession_RestartFromSessionId_MissingSourceReturnsNotFound`.
- Files: `server/services/session_service_test.go`

---

## Phase 3: Frontend (manual trigger — ships independent of Phase 4)

### Epic 3.1: Data hook

**Goal**: Poll `GetHandoffSummary` while generating, mirroring `useSessionSummary` structurally.

#### Story 3.1.1: `useHandoffSummary`

**As a** frontend component, **I want** a hook that fetches and polls a session's handoff summary, **so that** `RestartWithSummaryButton` and `HandoffSummarySection` share one data layer.

**Acceptance Criteria**:
- Polling starts automatically while `status` is `PENDING`/`GENERATING`/`UNSPECIFIED`(no row), and stops on `READY`/`ERROR`.
  - *Given* `sessionId = "s1"` and a mocked `HandoffSummaryService` client returning `{summary: {status: GENERATING}}` then, on the next poll tick, `{summary: {status: READY, summaryText: "..."}}`, *When* the hook is mounted and one poll interval elapses, *Then* `data.status` transitions from `GENERATING` to `READY` and `intervalRef` is cleared (no further polls).
- `trigger()` dispatches `TriggerHandoffSummary` and immediately reflects the returned (likely `GENERATING`) row, then resumes polling.
  - *Given* no existing row, *When* `trigger()` is called, *Then* `data.status` becomes `GENERATING` synchronously after the call resolves, and polling restarts if it had stopped.

**Files**: `web-app/src/lib/hooks/useHandoffSummary.ts` (new), `web-app/src/lib/hooks/useHandoffSummary.test.ts` (new)

##### Task 3.1.1a: Implement the hook (~5 min)
- New file, near-verbatim structural copy of `useSessionSummary.ts:1-220`: same `POLL_INTERVAL_MS`/`MAX_NULL_POLL_ATTEMPTS` constants, same `activeSessionIdRef`/`pollInFlightRef` staleness guards, same `startPolling`/`stopPolling`/`fetchSummary` shape — but calling `HandoffSummaryService`'s `getHandoffSummary`/`triggerHandoffSummary` instead of `SessionSummaryService`'s methods, and exposing `trigger` instead of `regenerate`.
- `isGenerating(status)` equivalent: `HandoffSummaryStatus.UNSPECIFIED | PENDING | GENERATING`.
- Files: `web-app/src/lib/hooks/useHandoffSummary.ts`

##### Task 3.1.1b: Tests (~5 min)
- Mirror `useSessionSummary.test.ts`'s test shapes (mock transport, assert polling start/stop transitions, assert `trigger()`'s immediate state update).
- Files: `web-app/src/lib/hooks/useHandoffSummary.test.ts`

---

### Epic 3.2: Restart action

**Goal**: A button that drives the full trigger → poll → create-session flow.

#### Story 3.2.1: `RestartWithSummaryButton`

**As a** user with a long-running session, **I want** a "Restart with summary" button, **so that** I get an assisted recovery path instead of manually re-deriving context.

**Acceptance Criteria**:
- Clicking the button with no existing summary calls `trigger()` and shows a generating state.
  - *Given* no existing `HandoffSummary` row, *When* the user clicks the button, *Then* `useHandoffSummary.trigger()` is called and the button renders a disabled "Generating summary..." state (matching `SessionSummaryPanel`'s existing generating-state visual convention).
- Once `READY`, the button becomes "Start new session from this summary" and, on click, calls `createSession` with the summary text as `prompt` and the source session's ID as `restartFromSessionId`.
  - *Given* `data.status === READY` and `data.summaryText === "[CONTEXT COMPACTION...]..."`, *When* the user clicks "Start new session from this summary", *Then* `createSession({ path: undefined, sessionType: DIRECTORY, prompt: data.summaryText, restartFromSessionId: sessionId, title: "<derived>" })` is called exactly once, and on success the UI navigates to the newly created session.
- Feature-flagged: the button does not render at all when the backend reports the feature disabled.
  - *Given* a `GetHandoffSummary` (or a dedicated feature-flag surface — confirm during implementation whether `HandoffSummaryConfig.Enabled` needs its own read RPC, or can ride on an existing config-exposure surface) response indicating the feature is disabled, *When* the session detail view renders, *Then* `RestartWithSummaryButton` renders `null`.

**Files**: `web-app/src/components/sessions/RestartWithSummaryButton.tsx` (new), `RestartWithSummaryButton.css.ts` (new), `RestartWithSummaryButton.test.tsx` (new)

##### Task 3.2.1a: Implement the button component (~5 min)
- New file, three-state render (idle → generating → ready-to-restart), using `useHandoffSummary(sessionId)` and `useSessionService().createSession`.
- vanilla-extract styling per `.claude/rules/css-architecture.md` — no `.module.css`, no hardcoded colors, tokens from `theme.css.ts`.
- Files: `web-app/src/components/sessions/RestartWithSummaryButton.tsx`, `RestartWithSummaryButton.css.ts`

##### Task 3.2.1b: Tests (~5 min)
- `RestartWithSummaryButton_should_TriggerGeneration_When_ClickedWithNoSummary`, `..._should_CreateSessionWithSummaryAsPrompt_When_ClickedWhileReady`, `..._should_RenderNothing_When_FeatureDisabled`.
- Files: `web-app/src/components/sessions/RestartWithSummaryButton.test.tsx`

---

### Epic 3.3: Info-tab history section

**Goal**: A capped, collapsible record of this session's handoff-summary attempts, per `research/ux.md` §2's recommendation.

#### Story 3.3.1: `HandoffSummarySection`

**As a** user reviewing a session, **I want** to see whether/when a handoff summary was generated for it, **so that** I can inspect what would be (or was) carried into a restart.

**Acceptance Criteria**:
- Renders an explicit empty state when no `HandoffSummary` row exists, rather than omitting the section.
  - *Given* `useHandoffSummary(sessionId).data === null`, *When* `HandoffSummarySection` renders inside the Info tab, *Then* it shows `"No handoff summary generated for this session."` rather than rendering nothing (matching `WorkflowHistorySection`'s "always render, explicit empty state" convention, `WorkflowHistorySection.tsx`'s doc comment).
- Uses `role="list"`/`role="listitem"`, not `role="status"` — this is a historical record, not a live-announced region.
  - *Given* a `READY` row, *When* the section renders, *Then* the container has `role="list"` and the single row has `role="listitem"`, and the row's icon carries `aria-hidden="true"` with a visible text label alongside it (per `research/ux.md` §3).
- Embeds `RestartWithSummaryButton` on the row itself when `status === READY`.
  - *Given* a `READY` row, *When* the section renders, *Then* the row includes the "Start new session from this summary" action (Epic 3.2), not a separate detached button elsewhere on the page.

**Files**: `web-app/src/components/sessions/HandoffSummarySection.tsx` (new), `.css.ts` (new), `.test.tsx` (new), `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.3.1a: Implement the section (~5 min)
- New component, structurally modeled on `WorkflowHistorySection.tsx:1-60`: `CollapsibleSection` wrapper, `role="list"`/`"listitem"`, explicit empty-state text, single-row rendering of the `HandoffSummary` data (status, relative timestamp via the same `formatRelativeTime` idiom as `CheckpointList.tsx:15-24`, embedded `RestartWithSummaryButton` when ready).
- Files: `web-app/src/components/sessions/HandoffSummarySection.tsx`, `HandoffSummarySection.css.ts`

##### Task 3.3.1b: Mount in the Info tab (~3 min)
- In `SessionDetailView.tsx`, inside the `activeTab === "info"` block (line 836), render `<HandoffSummarySection sessionId={session.id} />` below the existing key/value grid, not as a new tab.
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.3.1c: Tests (~4 min)
- `HandoffSummarySection_should_RenderExplicitEmptyState_When_NoRowExists`, `..._should_UseListRoles_NotStatusRole_When_Rendered`, `..._should_EmbedRestartButton_When_StatusReady`.
- Files: `web-app/src/components/sessions/HandoffSummarySection.test.tsx`

---

## Phase 4: Auto-trigger on `ContextHealth` RED (BLOCKED — do not start until dependency lands)

**Status**: Blocked on `context-health-monitoring` shipping its Phase 1 (signal) + Phase 2 (proto `context_health`/`context_health_reason` fields, `Instance.SetContextHealth`) — see Unresolved Questions. No task in this phase should be started before verifying that project's state.

### Epic 4.1: Auto-surface on RED transition

**Goal**: When a session's `ContextHealth` first transitions to `HealthRed`, auto-dispatch handoff-summary generation so a "Restart with summary" offer is ready the moment the user notices the badge, rather than making them wait through a full generation cycle after clicking.

#### Story 4.1.1: Wire into the (future) `publishContextHealth` transition hook

**As a** user, **I want** the handoff summary already generating by the time I see a RED health badge, **so that** clicking "Restart with summary" doesn't make me wait through the LLM call from a cold start.

**Acceptance Criteria** *(written against `context-health-monitoring/implementation/plan.md`'s Task 2.2.3a, which is itself unimplemented as of this writing — do not treat these file:line references as live until that plan ships)*:
- A `HealthGreen`/`HealthAmber` → `HealthRed` transition dispatches `HandoffSummaryGenerator.GenerateAndPersist` in the background, without blocking the existing level-transition log/publish flow.
  - *Given* `publishContextHealth`'s level-transition branch (`context-health-monitoring/implementation/plan.md` Task 2.2.3a, "On a Level change") fires with `verdict.Level == HealthRed` and `prevLevel != HealthRed`, *When* that branch executes, *Then* it additionally calls `go handoffSummaryGenerator.GenerateAndPersist(context.Background(), inst.ID(), inst.Title())` alongside its existing log/publish calls.
- A transition into RED that isn't a fresh crossing (e.g. already RED, reason string changed) does **not** re-trigger generation.
  - *Given* a session already `HealthRed` whose `Reason` changes but `Level` does not, *When* `publishContextHealth` runs, *Then* no new `GenerateAndPersist` call is dispatched (relies on the same level-only gating `context-health-monitoring`'s Task 2.2.3a already implements for its own log/publish decision — this story adds a call inside that same gated branch, not a second independent gate).

**Files**: `server/dependencies.go` (the future `publishContextHealth` function — exact line TBD once `context-health-monitoring` ships)

##### Task 4.1.1a: Add the dispatch call inside the existing level-transition branch (~3 min, once unblocked)
- Locate `publishContextHealth`'s "On a Level change" branch (post-`context-health-monitoring` implementation). Add `if verdict.Level == tokens.HealthRed { go handoffSummaryGenerator.GenerateAndPersist(context.Background(), inst.ID(), inst.Title()) }` immediately after the existing log line.
- Files: `server/dependencies.go`

---

## Closing out the original acceptance criteria

Per `requirements.md`'s Acceptance Criteria and this research's findings (ADR-001):

| Original AC | Disposition |
|---|---|
| Token usage tracked per turn from Claude API response metadata | **Superseded** — already solved by `session/tokens` (no controller-level API metadata access exists or is needed); no task in this plan re-touches it |
| Compression fires at a configurable threshold (85%), not more than once per N turns | **Superseded and reframed** — Phase 4's RED-verdict trigger (once unblocked) plus Phase 3's always-available manual trigger replace the threshold/cooldown design; dedup is per-session in-flight guarding (Epic 1.5), not a turn-count cooldown |
| Summary uses the verbatim REFERENCE-ONLY prefix | **Carried forward literally** — Epic 1.4, Task 1.4.1a |
| Tool output blobs pruned before summarization | **Carried forward, reframed mechanism** — Epic 1.3's `pruneExcerptText` + `session/history.go`'s existing text-only content filter, not a live-transcript pre-pass |
| Compression event shown in session detail UI | **Carried forward, reframed surface** — Epic 3.3's `HandoffSummarySection`, an Info-tab list with an actionable restart button, not a passive timeline badge |
| Unit tests: threshold detection, head/tail protection boundaries, tool output pruning | **Carried forward, reframed target** — `buildTranscriptWindow` (Story 1.3.1), `applySummaryBudget`/`pruneExcerptText` (Story 1.3.2) tests cover the equivalent boundaries for the new mechanism |
