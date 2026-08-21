# Research: Feature Landscape — Context Health Monitoring

Agent: Research Agent 2 (Features), SDD Phase 2

## 1. What already exists in `session/detection/` — plug in, don't duplicate

`session/detection/` is a mature, multi-layered system. ContextHealth should be a
new consumer/aggregator over this data, not a parallel regex pipeline.

- **`terminal_detector.go`** defines the `TerminalDetector` interface (implemented by
  `StatusDetector`, defined in `detector.go`, not read in full this pass): `Detect`,
  `DetectWithContext`, `DetectFromLines`, `RecentEvents(n)`, `SetSessionID`.
  `DetectedStatus` (`detector.go:18`) enumerates categories already computed per
  session every ~1s: `StatusExecuting`, `StatusProcessing`, `StatusIdle`, `StatusReady`,
  `StatusWaitingForAgent`, `StatusNeedsApproval`, `StatusInputRequired`, `StatusSuccess`,
  `StatusError`, `StatusTestsFailing`, `StatusUnknown`. Regexes for these categories are
  compiled per-agent-binary in `pattern_set.go` from `session/status_patterns.yaml`
  (agent-specific, e.g. `claude.go` under `binaries/`) — there is already an `"error"`
  category regex list (`pattern_set.go:17,46`), which is the closest existing thing to
  "apology/error language" but it's tuned for *status classification* (is Claude in an
  error state right now), not *frequency-over-time* (has it apologized/errored N times in
  a row). New confusion-language matching is genuinely new heuristic work, but the error
  regex list is a reasonable seed vocabulary to start from rather than inventing patterns
  from scratch.
- **`events.go` / `event_sink.go`**: `DetectionEventSink` wraps a fixed-capacity ring
  buffer (`EventRingCap = 2000`) of `DetectionEvent{SessionID, Timestamp, MatchedPattern,
  MatchedCategory, ResultStatus, TailSnippet(512B)}`, one entry per `Detect()` call.
  `StatusDetector.RecentEvents(n)` (via the `TerminalDetector` interface) already gives
  any consumer a rolling window of *what category fired, when, and a text snippet* —
  this is the natural data source for a loop/confusion heuristic that needs to look at
  "the last N detection events for this session," rather than re-scanning raw PTY output.
  At ~1Hz this buffer holds roughly 30+ minutes of history; exact wall-clock coverage
  depends on how often `Detect`/`DetectFromLines` fire (up to 50 calls per status check
  per the comment at `events.go:20-22`), so ContextHealth's design should not assume a
  fixed time window from event count alone — read timestamps.
- **`idle.go`**: `IdleDetector` computes `IdleStateActive/Waiting/Timeout/Unknown` from
  the same `TerminalDetector`, with debouncing (`DebounceDelay`), a `minActivityInterval`
  no-op guard on `RecordActivity`, and **explicit session-restart continuity handling**
  via `InitializeFromTimestamp` (rejects future timestamps, rejects timestamps >24h old,
  restores `lastActivity` otherwise) — this is the precedent to copy for "session was
  paused/restarted, don't misfire health on stale state." `IdleDetectorConfig` (threshold,
  debounce, buffer size) with `DefaultIdleDetectorConfig()` is the precedent for a
  `ContextHealthConfig` struct with sane defaults, injectable and overridable.
- **`approval.go`**: `ApprovalDetector` is a separate regex-pattern system (bash/file/tool
  confirmation prompts) with its own history ring (`maxHistory`), pub/sub via
  `Subscribe`/channels, and a `GetStatistics()` rollup. Important distinction: **"waiting
  for approval" is already a first-class, separately-tracked state** (also visible as the
  now-deprecated `SESSION_STATUS_NEEDS_APPROVAL` in
  `proto/session/v1/types.proto:341`, folded into `SESSION_STATUS_ACTIVE` + a substatus).
  ContextHealth must NOT count "waiting on approval" time or tool-call-adjacent text as
  loop/confusion signal — see edge cases below.
- **`session/tokens/`** (`types.go`, `jsonl_types.go`, `parser.go`) is a second, richer
  data source not mentioned in the prompt's file list but directly relevant: it parses
  Claude Code's own JSONL transcripts (not PTY text) and already extracts, per assistant
  turn, `TurnStats{Timestamp, Model, Input/Output/Cache tokens, ToolNames []string}` and
  session-level `ToolUsage map[string]ToolTokenStats{ToolName, CallCount, MCPServer}`.
  This is structured (tool name + count), not text-pattern-matched — a much better
  substrate for loop detection than PTY regex if it can be extended. **Important
  constraint**: `jsonlContent.Input` (`jsonl_types.go:39`) — the actual tool-call
  arguments — is parsed transiently by the token parser but explicitly **not
  captured/stored**, per a documented privacy stance ("Message content is never stored",
  `types.go:7`) and a perf fix (`PerfFix-4`, `jsonl_types.go:48-51`, avoiding a ~237KB
  per-call allocation for `Input`). Loop detection needs to compare tool-call *arguments*
  for similarity, which today's token parser deliberately discards. This is a real design
  fork for Phase 3: either (a) hook loop detection into the terminal/PTY detection layer
  instead (regex/text similarity on rendered tool-call output, consistent with the
  existing `session/detection` package this feature is scoped to), or (b) extend the
  JSONL path to compute a lightweight fingerprint (e.g., a hash of normalized args) without
  retaining the raw payload — a deliberate scope decision, not a default, given the
  existing "never store message content" line in the codebase.

## 2. Edge cases for the two heuristics

**Loop detection (repeated tool call w/ similar args):**
- Legitimate polling loops that *are* the task — e.g., an agent doing `git status` in a
  wait loop for a background job, or repeatedly checking `docker ps` — must not be flagged.
  Precedent for this exact false-positive shape is in
  `project_plans/backlog-stuck-item-visibility/requirements.md`'s "Rabbit Holes" section
  (line 58-59): "Distinguishing 'genuinely stuck' from 'actively cycling but making
  progress' ... a naive time-since-last-transition threshold risks false positives on
  legitimately slow-but-working items." That doc's proposed mitigation was "N consecutive
  FAIL verdicts with no diff change, not just elapsed time" — the analogous heuristic here
  is likely "N repeats of the same tool+args with no *change in outcome/output*", not
  count alone.
- Tool calls that are repeated by design with *varying* args (e.g., iterating a batch —
  reading file 1, file 2, file 3) must not match "similar args" naively; similarity
  needs to be args-content-aware, not just same-tool-name.
- A session actively waiting on approval or user input (`StatusNeedsApproval`,
  `StatusInputRequired`) is not "looping" even if the same tool call is queued/retried —
  must gate on idle/active state from `IdleDetector`/`ApprovalDetector` first.
- Sessions using `--tmux-keep-server`-style long idle periods, hibernated
  (`SESSION_STATUS_HIBERNATED`) or paused (`SESSION_STATUS_PAUSED`) sessions must freeze
  health rather than accumulate loop signal while genuinely inactive — same continuity
  problem `IdleDetector.InitializeFromTimestamp` already solves for idle state; a fresh
  ContextHealth detector must not start counting from zero after every server restart
  reconciliation cycle either (this repo restarts frequently in dev — see
  `.claude/rules/tmux-keep-server-on-restart.md` and the stuck-item-visibility doc's
  observation of "15+ restarts observed on a single day").
- Very short sessions (few tool calls so far) have no meaningful signal — needs an
  explicit minimum-sample-size floor before emitting anything other than "unknown"/green,
  mirroring `IdleStateUnknown`'s "don't maintain Unknown, default to Waiting" pattern in
  `idle.go:233-236` (i.e., decide the default state for insufficient data explicitly,
  don't let an empty window degrade silently into a false amber/red).

**Confusion/apology-language detection:**
- Apology phrases occur in ordinary non-confused conversation too — "Sorry, that command
  needs sudo", "my apologies, wrong file path, fixing now" followed by an actual fix is
  normal self-correction, not degradation. Frequency/streak-based ("N such phrases within
  M turns with no successful action in between"), not single-match, is required — same
  shape as the `errorRegexes` category which already exists for one-shot status
  classification but would need a *rate* wrapper for this feature.
- Multi-language agent output: the requirements scope this to Claude Code/Aider sessions,
  but if any configured agent binary runs non-English models/prompts, an English-only
  apology-phrase regex list will silently under-detect. Given the existing pattern-set
  system is already per-agent-binary and YAML-configurable (`status_patterns.yaml`), the
  same per-binary configurability precedent should extend to confusion phrases if this is
  a real risk — but confirm with requirements whether non-English agents are in scope
  before building it (this may be a non-issue if all target agents are English-default).
- Tool output that happens to contain the word "error" (e.g., a test runner printing
  "0 errors") must not be conflated with the agent expressing its *own* confusion —
  the existing `error` category in `pattern_set.go` presumably already has to solve this
  problem for status detection; a shared or borrowed pattern set should inherit whatever
  false-positive guards it already has rather than re-deriving them.

## 3. Likely unstated needs beyond the explicit requirements

- **Mute/dismiss per session**: directly precedented by
  `project_plans/smart-notification-dedup/requirements.md`, which already establishes
  condition-change gating ("don't re-emit unless the condition worsened"), a
  count/severity baseline reset on improvement, and per-`(sessionId, notificationType)`
  dedup for native OS notifications. A red/amber context-health indicator that re-alerts
  every poll cycle without this gating will reproduce exactly the "Fork Pressure: Critical
  spamming" bug that doc was written to fix. ContextHealth's tooltip/indicator should
  follow the same worsened-since-last-seen model, and any toast/native notification for a
  health transition (if this feature adds one — not explicitly in scope per the prompt's
  "green/amber/red indicator" framing) must reuse that dedup path rather than invent a new
  one.
- **Health history/trend, not just current state**: the existing `DetectionEventSink`
  ring buffer and `TurnStats` timeline (used for token burn-rate charts per
  `project_plans/token-monitoring/requirements.md`'s "Session efficiency score" trend
  chart) are both precedent for surfacing a trend, not just a point-in-time value — likely
  worth exposing "how long has this session been amber" or a short sparkline in the
  tooltip, consistent with how idle duration (`GetIdleDuration`) and detection history
  (`RecentEvents`) are already both exposed as durations/rolling windows elsewhere in this
  package, not single booleans.
- **Distinguishing "looping" from "stuck waiting for approval"**: this is not just a
  heuristic edge case (see above) but a UI/API design need — `ContextHealth`'s reason
  string must be distinguishable from the existing `NeedsApproval`/`InputRequired`
  substatus so a user glancing at a session card doesn't see two different-looking
  indicators claiming the same thing, or a red context-health badge stacked confusingly
  on top of the (correct, unrelated) approval-pending badge. The proto's own comment at
  `types.proto:340-341` shows the codebase already went through one iteration of folding
  `NEEDS_APPROVAL` from a top-level status into a substatus for exactly this kind of
  clarity reason — ContextHealth should be additive to that substatus model, not a
  competing top-level enum value.
- **Threshold configurability implies a debug/inspection surface**: given `idle.go`'s
  `IdleDetectorConfig` and the approval detector's pattern list are both runtime-tunable,
  users will likely want to see *why* a session went amber/red (which heuristic fired,
  what evidence) — `RecentEvents`/`GetStatistics`-style introspection already exists for
  the two sibling detectors, so ContextHealth's reason string (already in scope per the
  requirements: "ContextHealth status+reason") should probably carry enough structure
  (which heuristic, count/streak, timestamp of first occurrence) to answer that, not just
  a static string.

## 4. Related prior project_plans — cross-reference notes

- **`project_plans/token-monitoring/requirements.md`**: establishes the JSONL-parsing
  data source (`session/tokens/`) this feature could optionally extend for loop
  detection's tool-call data, and the "session efficiency score" trend-chart concept as a
  precedent for a health-trend view. Also the precedent for a session-card badge +
  dedicated dashboard page pairing (mirrors the light-badge-on-card / deeper-view-elsewhere
  split this feature's green/amber/red indicator + tooltip likely wants).
- **`project_plans/detector-plugins/requirements.md`**: not directly about health, but
  establishes that `session/detection/binaries/*.go` + `status_patterns.yaml` is already
  moving toward user-configurable, per-agent pattern sets (TOML-driven plugin detectors).
  If confusion-language patterns need to be per-agent or user-extensible later, this is
  the existing extension point/precedent to align with rather than a new bespoke config
  path.
- **`project_plans/backlog-stuck-item-visibility/requirements.md`**: the single most
  relevant prior-thinking doc for "is this session actually doing useful work," despite
  being scoped to backlog automation items rather than live interactive tmux sessions. Its
  "Rabbit Holes" section names the exact false-positive risk (naive threshold vs.
  genuinely-progressing-but-slow work) that loop detection will hit, and its chosen
  mitigation pattern (require *no change* across N repeats, not just elapsed
  time/count) is a directly reusable heuristic shape. Its `Success Metrics` section is
  also a model for how to state a falsifiable metric for a single-user internal tool
  (no multi-tenant telemetry) if this feature needs one.
- **`project_plans/smart-notification-dedup/requirements.md`**: directly relevant to the
  unstated "mute/dismiss" need above — reuse its condition-change-gating and baseline-reset
  model rather than re-deriving alert-fatigue handling from scratch.
- **`project_plans/terminal-analytics/requirements.md`**: adjacent (also instruments the
  PTY/terminal pipeline) but not substantively relevant — it's about escape-sequence
  mangling/rendering fidelity, not agent behavior classification. No reuse expected beyond
  general awareness that PTY-pipeline instrumentation is a recurring theme in this repo.
- No `project_plans/stale-session*` or similarly-named directory exists; the closest
  matches by content are `review-gate-stale-session-rework` (not read in depth this pass —
  worth a follow-up glance in Phase 3 if "session considered stale" logic there overlaps)
  and `backlog-stuck-item-visibility` (covered above).

## Key file references

- `session/detection/idle.go` — `IdleDetector`, `IdleDetectorConfig`, restart-continuity pattern
- `session/detection/approval.go` — `ApprovalDetector`, separate approval-request tracking
- `session/detection/terminal_detector.go` — `TerminalDetector` interface
- `session/detection/event_sink.go`, `events.go` — `DetectionEventSink`, `DetectionEvent` ring buffer
- `session/detection/pattern_set.go`, `session/status_patterns.yaml` — per-agent regex categories including existing `error` category
- `session/tokens/types.go`, `jsonl_types.go` — JSONL-derived `TurnStats.ToolNames`, `ToolUsage`; tool-args (`Input`) intentionally not persisted
- `config/config.go:229+` — `Config` struct pattern for adding a new `ContextHealthConfig` sub-struct
- `proto/session/v1/types.proto:321-351` — `SessionStatus` enum, precedent for substatus vs. top-level status modeling
- `project_plans/backlog-stuck-item-visibility/requirements.md` — stuck-vs-progressing heuristic precedent
- `project_plans/smart-notification-dedup/requirements.md` — alert dedup/condition-change-gating precedent
- `project_plans/token-monitoring/requirements.md` — JSONL data source + trend/badge UI precedent
