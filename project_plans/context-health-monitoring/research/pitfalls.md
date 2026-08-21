# Pitfalls Research: context-health-monitoring

## 1. Loop detection over streaming tool-call/text events

**No structured tool-call+args event stream exists today.** `session/detection/approval.go`'s
`ApprovalRequest` is a regex match over terminal text (`ApprovedPattern.Pattern` + `ExtractedData
map[string]string` capture groups) keyed to approval *prompts*, not a general tool-invocation log
with `(toolName, args)` per call. The Claude binary's own patterns
(`session/detection/binaries/claude.go`, `Processing.tool_use`) only match a *line* like
`Reading|Writing|Editing|Executing|Running <path>` — there is no parsed "tool + normalized
argument" struct anywhere in `session/detection`. Confirms the Open Question in
`requirements.md` ("Does the existing event data include enough structure to do loop detection
cheaply?") is **no** — loop detection must parse tool invocation lines itself and define its own
normalized-argument comparison key. Get the normalization wrong (e.g. comparing raw strings
including timestamps, line numbers, or generated tmp paths that differ per call) and every retry
loop that legitimately varies one argument (e.g. retrying a failed `Read` at a slightly different
offset, or `Bash` polling `git status` in a loop) becomes indistinguishable from a real stuck loop.

- **False positives**: legitimate repeated polling (e.g. an agent polling `tmux capture-pane` or
  `git status` while waiting on a long-running background command, or re-running the same test
  command after each small fix) looks identical to a stuck loop under exact-match comparison.
  Any "3+ similar calls in a row" heuristic needs a definition of "similar" precise enough to
  exclude deliberate, useful repetition — the requirements.md Rabbit Holes section already flags
  this ("get this wrong and it's all false positives or all false negatives").
- **False negatives**: a loop that varies one argument each iteration (e.g. `Edit` on the same
  file with a slightly different `old_string` each attempt, or incrementing a retry counter
  embedded in a shell command) evades naive exact-match or even shape-based comparison. A
  similarity threshold that's too loose reintroduces false positives; too strict and it never
  fires.
- **Performance**: comparing every new event against the *entire* session history is an O(n)
  (or worse, O(n²) over a session's lifetime) scan that grows unboundedly as a session runs for
  hours — this directly conflicts with the stated NFR (`requirements.md`: "<5ms per output chunk
  processed, must not block rendering"). The codebase's own precedent for this exact tension is
  `session/detection/events.go`'s `eventRing` — a fixed-capacity ring buffer (2000 slots,
  sized explicitly "because ClaudeController and IdleDetector share one ring... 2000 slots = ~33
  seconds of headroom at 1 Hz") guarded by a single mutex (`event_sink.go`). Loop detection should
  reuse this bounded-window idiom (e.g. last N tool-call events, N small — the loop threshold is
  only 3+) rather than scanning growing per-session history; there is no reason to compare against
  more than the last few events since the heuristic only cares about *consecutive* repeats.

## 2. Confusion/apology keyword detection — brittleness and noise risk

**Per-backend pattern asymmetry is already severe and observable today**, not hypothetical.
`session/detection/binaries/claude.go` has ~30 detailed status patterns (Processing, Active,
Success, Error, Idle, InputRequired, NeedsApproval) tuned to Claude Code's exact phrasing and
spinner glyphs (e.g. `[·✢✳✶✻✽●*✦]` thinking-verb regex, `Synthesizing`, cost-summary-line
`\$\d+\.\d+\s+•`). By contrast `session/detection/binaries/aider.go` has **empty pattern slices
for every category except `NeedsApproval`** (a single `(Y)es/(N)o/(D)on't ask again` pattern).
An apology/confusion-language heuristic keyed off Claude-specific phrasing ("I apologize", "I
made a mistake") will silently produce **zero signal for Aider sessions** — not a false negative
on individual messages, but total blindness for an entire backend, exactly the risk called out in
requirements.md's Rabbit Holes ("'I apologize' is Claude-specific phrasing; Aider or other agent
backends may never emit it"). Any implementation needs either a per-`BinaryDetector`
confusion-pattern set (mirroring how `Patterns()` is already implemented per binary) or an explicit
decision to scope the feature to Claude-only sessions for this iteration, stated rather than left
implicit.

- **Brittleness across phrasing/locale**: keyword matching on "I apologize" / "sorry" style
  strings breaks for non-English output, and any agent backend that phrases self-correction
  differently ("Oops", "Let me try again", "Correcting course") evades detection entirely without
  an enumerated-and-maintained keyword list per backend — an ongoing maintenance burden similar to
  the existing per-binary pattern sets, which already required multiple documented bug-regression
  fixes (`session/detection/bug_regression_test.go` has 30+ `TestBug*` cases guarding against
  false-positive/false-negative regressions in the existing *idle/active* pattern matching alone).
  Confusion detection is a new pattern category and should expect a comparable regression-test
  investment, not a one-shot regex.
- **Alert-fatigue risk undermines the whole feature**: requirements.md's own Success Metrics
  acknowledge "no ground-truth labeled dataset exists yet" for false-positive rate — meaning a
  noisy signal ships before it's known to be noisy. If health badges routinely amber/red on
  benign self-correction language, users will learn to ignore or (per the Risk Control section's
  own suggested mitigation) raise thresholds until the feature is effectively off, which defeats
  the competitive differentiation the requirements are chasing ("explicit context-health
  monitoring... is rare but high-value"). See `.claude/rules/*` — there is no repo-wide rule
  file for alert-fatigue yet, but there is a very close precedent below.

## 3. Concurrency/locking — double-checked-locking risk in a health cache

**Confirmed pattern risk.** `.claude/rules/go-double-checked-locking.md` documents a real bug
class already fixed once in this codebase (`session/git/worktree_git.go` `IsDirty`): a
read-lock → cache-miss → compute → write-lock → conditional-store sequence that returned
`g.cache` (the shared slot) instead of the locally-computed value, so a goroutine that lost the
write race silently returned a foreign result. A per-session `ContextHealth` cache is the exact
same shape: multiple goroutines (PTY output ingestion, a poller, an RPC handler serving the
frontend) may all compute health state concurrently for the same session ID. If the
implementation follows "check cache → miss → compute → lock → conditionally store → return
shared slot," it reproduces the identical bug. The fix is identical too: **always return the
locally-computed value**, never re-read the slot after the conditional store. `IdleDetector`
(`session/detection/idle.go`) is a good structural template to copy instead — it separates
"expensive computation outside the lock" (regex/pattern matching) from "state mutation under a
single `sync.RWMutex`" (`DetectStateFromContent`'s two-phase comment: "Phase 1: expensive
detection outside the lock... Phase 2: state update under write lock"), and always returns
`id.currentState` read fresh under the same lock right after the write — not a value computed by
some other, possibly-stale invocation. `ContextHealth` should follow this same two-phase
lock-minimal shape per session, not introduce a new caching pattern.

- Also worth checking: the NFR explicitly says "without new per-session goroutine/lock
  contention" — meaning `ContextHealth` computation should likely piggyback on the *existing*
  per-session detection goroutine/PTY-read path (as `IdleDetector.RecordActivity` and
  `DetectStateFromContent` already do) rather than spinning up a second polling loop per session,
  which would double lock acquisitions across dozens of concurrent sessions.

## 4. Config pitfalls — threshold validation and malformed/missing config

**Established safe-default idiom exists and should be copied exactly.** `config/config.go` shows
two governing patterns:

1. **`LoadConfig()` never panics on malformed or missing config** — `LoadConfigFromPath` returns
   an error on bad JSON, and `LoadConfig()` catches it: `os.IsNotExist` → write+return
   `DefaultConfig()`; any other parse error → `log.Warn` and return `DefaultConfig()` (config.go
   lines ~760–782). A malformed `context_health` JSON block must not crash the server or block
   startup — it should log and fall back to defaults, consistent with every other config field.
2. **`*OrDefault()` accessor methods clamp and default numeric thresholds**, e.g.
   `MaxConcurrentBacklogWorkItemsOrDefault()` (config.go:604-615): falls back to a named default
   constant when the configured value is `<= 0` (i.e. unset or invalid), and clamps to a documented
   hard ceiling ("to guard against reintroducing the 2026-07-12 OOM") even if a bad value slips
   through. `AnalyticsMaxAgeDaysOrDefault()` follows the identical shape. The `context_health`
   loop-count / apology-count thresholds should get the same treatment: a
   `LoopThresholdOrDefault()` / `ApologyThresholdOrDefault()` (or similar) that treats `<= 0` as
   "unset, use default" rather than a literal 0-count threshold — a naive zero-value threshold
   would either disable the heuristic entirely (0 loops "before" triggering, if interpreted as "no
   threshold") or fire constantly (if interpreted as ">= 0 matches trigger"), depending on
   comparison direction. Pick the failure-safe interpretation explicitly and test it, matching the
   defensive style already established for `ResourcePressureThreshold` and friends in
   `config/types.go`.

## 5. UI pitfalls — badge/alert fatigue prior art

**Direct, closely-analogous prior art exists**: `project_plans/smart-notification-dedup/` is an
already-researched (and per its `implementation/` dir, already-planned) fix for a near-identical
alert-fatigue bug — "Fork Pressure: Critical" notifications re-firing every ~2 minutes for the
lifetime of an elevated-but-*unchanging* zombie-process condition, because the cooldown timer
reset on every window rather than tracking whether the condition had actually *worsened*. Its
`research/pitfalls.md` documents 8 specific pitfalls worth mapping onto `ContextHealth`:

- **Condition-change gating, not just a cooldown timer** (Pitfall 2): a health badge that
  re-flags amber/red on every polling cycle while the loop/apology count is *unchanged* (not
  worsening) will retrain users to ignore it, exactly the failure mode Problem 1 in that project
  describes. `ContextHealth` should fire a state-*transition* signal (green→amber, amber→red —
  already stated in requirements.md's Observability Requirements) and suppress re-emission when
  the underlying counts haven't increased, mirroring the "worsened = higher count OR higher level"
  rule from that project's Pitfall 8.
- **Per-metric baselines** (Pitfall 8): if `ContextHealth` is red because of *either* the loop
  heuristic *or* the apology heuristic (an OR condition, as scoped in requirements.md), track each
  metric's baseline independently — a case where the loop count drops but apology count rises
  should still count as "worsened," analogous to the zombies-vs-failures mixed case documented
  there.
- **Re-arm on clear** (FR-5 / Pitfall 6 in that project): once a session's health returns to
  green, the next amber/red transition should be treated as fresh (not suppressed by a stale
  baseline) — same reasoning applies directly to `ContextHealth`'s session-card badge.
- No repo-wide rule file yet formalizes "alert/badge fatigue" as a named anti-pattern outside
  that one project's docs — worth flagging in Phase 3 planning as a candidate for a shared
  `.claude/rules/` entry once both features exist, per the `stapler-squad-rules` skill, but that's
  a follow-on, not a blocker for this feature.
