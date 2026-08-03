# Requirements: context-health-monitoring

**Date**: 2026-08-02
**Type**: feature addition
**Complexity**: 3 — system design (touches backend detection pipeline, config/rules engine, approval analytics, and frontend session UI; no external integration but multiple subsystems)

## Problem Statement

stapler-squad manages long-running Claude Code / Aider agent sessions in tmux, but has no way to detect when an agent's context window is degrading (looping on the same tool calls, repeatedly apologizing/self-correcting, slowing down, or approaching the context limit). Today the only signal a user gets is the raw terminal output — they must notice degradation themselves, often after the agent has already made several bad decisions. Competitive analysis (Tutti, evaluated across 122 agent orchestration tools) shows explicit context-health monitoring with a configurable handoff-threshold trigger is rare but high-value: it warns before quality drops and offers a "restart with a compressed handoff summary" recovery path instead of silently losing the session's progress.

## Baseline

Today, a user notices context degradation only by actively watching a session's terminal output or reading back through scrollback after the fact. There is no badge, no alert, no automatic detection of tool-call loops or repeated error/apology language, and no assisted way to hand off to a fresh session — a user who wants to "restart clean" must manually re-derive context and paste it into a new session themselves.

## Users / Consumers

- **Interactive users** watching the session list / board (`web-app/src/components/sessions/`) who need an at-a-glance signal that a running session needs attention.
- **Backlog automation** (`session/detection`, `server/services/approval_*`) which already tracks tool-call events, approval requests, and detector state per session and would emit/consume health signals.
- **Rules engine** consumers (existing "Rules can reference X" pattern seen in approval policy) that may want to gate risky auto-approvals when health is red.

## Success Metrics

- A running session that loops on the same tool call ≥3 times in a row, or emits ≥5 apology/error-style messages, is flagged amber/red in the UI within one polling/event cycle — compared to the baseline of no signal at all.
- A user can click "Restart with summary" on a red-flagged session and get a new session pre-seeded with a generated handoff summary, without hand-copying scrollback.
- False-positive rate on the loop/apology heuristics is low enough that health badges are trusted (validated via manual review during Phase 6, not a hard numeric target — no ground-truth labeled dataset exists yet).

## Appetite

Medium (1–2 weeks) for a first shippable slice: health scoring (loop + apology-language signals only, since those are derivable from existing terminal/detection infrastructure without new data collection) + session-card badge + tooltip. Token-percentage signal and the handoff/restart flow are separate, larger stories that may extend into Large if pursued in the same iteration — see Rabbit Holes and Out of Scope.

## Constraints

- No dedicated backend/frontend deadline; driven by backlog priority only.
- Must not require restarting the live systemd-managed instance (`:8543`) to develop/test — use the manual second-instance pattern documented in project `CLAUDE.md`.
- Must reuse an existing per-session data pipeline rather than standing up a parallel one. **Superseded by Phase 2/3 findings (see ADR-001):** `session/detection`'s `PatternSet`/`event_sink.go` pipeline was the pipeline originally envisioned here, but Phase 2 architecture research found it carries no tool-call identity or arguments (only a status category + text snippet), which loop detection requires. The chosen pipeline is instead `session/tokens` (the existing Claude JSONL transcript parser/cache/subscriber pipeline) — still an existing, reused pipeline, just not the one this line originally named.

## Non-functional Requirements

- **Performance SLO**: health-signal computation must not add noticeable latency to the terminal output pipeline; target <5ms per output chunk processed, and it must not block rendering.
- **Scalability**: must handle the existing session concurrency ceiling (dozens of concurrent tmux sessions) without new per-session goroutine/lock contention (see `.claude/docs/concurrency-patterns.md`).
- **Security classification**: internal — health signals and generated handoff summaries may contain session content; no new external network calls introduced by this feature.
- **Data residency**: not applicable (local-only feature).

## Scope

### In Scope
- Per-session health signal computation from two initial heuristics: (1) repeated tool call with similar arguments (loop detection), (2) repeated "I apologize" / "I made a mistake" style language in agent output (confusion detection).
- An amber/red health indicator surfaced on the session card, with a tooltip explaining which signal(s) triggered it. **Green and "not enough data yet" are both intentionally unrendered (no badge)** — surfacing a badge only when it needs attention, per Phase 3 UX design (`design/ux.md`); this is a deliberate refinement of "green/amber/red," not a rendered three-state traffic light.
- Configurable thresholds for the two initial heuristics (loop count, apology-message count), following the existing config JSON pattern (`config/`).
- A single new backend data point (`ContextHealth` status + triggering reason) exposed via the existing session ConnectRPC surface so the frontend can render it without polling raw scrollback itself.

### Out of Scope (this iteration — see Rabbit Holes)
- Token-usage-percentage signal — depends on `project_plans/token-monitoring` (or `token-cost-tracking`) landing token/context-window accounting first; do not duplicate that work here. Wire it in as a third signal once that data exists.
- Approval-request-rate spike signal — depends on `server/services/approval_service.go` analytics already having a per-session time-windowed rate; needs its own investigation of whether that data is queryable cheaply.
- "Restart with summary" handoff-packet generation and the new-session creation flow it triggers — this touches the 7-touchpoint session-creation registry (`.claude/rules/session-creation-registry.md`) and deserves its own follow-on requirements/plan rather than being bolted onto health *detection*.
- Rules-engine integration (deny risky operations when health is red) — depends on this iteration shipping a stable `ContextHealth` value the rules engine can reference; sequenced as a follow-on.
- Approval-analytics dimension (health as a facet in existing analytics dashboards) — follow-on once the health signal itself is proven useful.

## Rabbit Holes

- **Apology/looping heuristics are language- and tool-shape-dependent.** "I apologize" is Claude-specific phrasing; Aider or other agent backends may never emit it. The loop-detection "similar arguments" comparison needs a concrete similarity definition (exact match vs. fuzzy) — get this wrong and it's all false positives or all false negatives. Treat as a single well-scoped heuristic module, not a general NLP problem.
- **"Restart with summary"** sounds like one button but is actually: summarization (calls back into an LLM), new session creation (7 touchpoints), and state transfer (what "counts" as context to carry over) — each a separate research/plan cycle. Explicitly deferred out of scope above so this iteration doesn't balloon into a second `session-creation-registry` project.
- **Health as a rules-engine input** implies the rules engine already has a stable extension point for new predicates; confirm rather than assume — if it doesn't, that's its own small yak-shave.

## Alternatives Considered

- **Build vs. adopt**: no known off-the-shelf library does tmux/Claude-Code-transcript-specific loop/confusion detection; this is inherently bespoke to stapler-squad's detection pipeline. Deferred to Phase 2 research (`build-vs-buy.md`) to confirm no adjacent OSS project already solves this narrow problem (unlikely, but check).
- **Polling scrollback text vs. hooking the event stream**: the existing `session/detection` package already normalizes and classifies terminal events (`normalizer.go`, `event_sink.go`); prefer extending that pipeline over adding a second regex pass over raw scrollback.

## Feasibility Risks

- Unclear whether `session/detection`'s existing event classification already captures enough granularity (tool name + args per call) to do loop detection cheaply, or whether new instrumentation is needed — Phase 2 research must confirm.
- Apology/confusion language detection is a heuristic keyword/pattern match, not true "is the agent confused" detection — risk of both false positives (legitimate uses of "I apologize") and false negatives (confusion without that phrasing). Acceptable for a first amber/red signal, not a hard gate.

## Observability Requirements

Health-state transitions (green→amber, amber→red) should be logged at info level per session (session ID, triggering signal, threshold crossed) so false-positive/false-negative rates can be reviewed manually post-launch. No new oncall alert — this is a user-facing advisory signal, not a production health check.

## Risk Control

Ship behind a config default of "enabled" but fully configurable per the `context_health` JSON block in the original proposal (threshold values); a user who finds it noisy can raise thresholds to effectively disable without a code change. No feature flag needed beyond the existing config-driven threshold pattern — standard revert via PR close/revert if the heuristics prove unworkable.

## Open Questions

- Does `session/detection`'s existing per-tool-call event data include enough structure (tool name, argument summary) today to compute "3+ similar calls in a row" without new instrumentation? *(for Phase 2 research)*
- Is there an existing per-session approval-rate time series usable for a future "approval rate spike" signal, or does that require new aggregation? *(for Phase 2 research, informs Out of Scope sequencing only)*
- What normalized data shape should `ContextHealth` take on the proto/ConnectRPC surface (enum + reason string vs. richer struct) — deferred to Phase 3 planning's Domain Glossary step.
