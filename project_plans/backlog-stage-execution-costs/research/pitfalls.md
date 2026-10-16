# Pitfalls & Risks Research — backlog-stage-execution-costs

Agent 4 (Pitfalls), SDD Phase 2. Companion to `requirements.md`. Grounded in this repo's
actual bug history (`docs/bugs/`), two directly-precedent prior projects
(`insights-cost-pricing-gaps`, `backlog-configurable-pipeline`), and direct reading of
`session/ent/schema/pipeline_mode.go`, `session/pipeline_engine.go`, `session/headless/`,
and `session/instance_program.go`.

---

## 1. Per-stage model/program override: what commonly goes wrong

### 1a. Stale/cached resolution after a config change mid-run
`CachingPipelineEngine` (`session/pipeline_engine.go`) already has this exact shape today for
`PipelineMode` templates: it loads a `pipelineModeCache` at construction and only refreshes via
an explicit `InvalidateCache` call that RPC write handlers must remember to invoke after
Create/Update/Delete/Enable/Disable. If per-stage program+model overrides are added as new
fields on the same `PipelineMode` entity, they inherit this cache-staleness surface for free —
**but only if every new write path also calls `InvalidateCache`**. Concretely: `plan.md`'s notes
for `CachingPipelineEngine` say "Epic 2.2... must invalidate the cache" — confirm the
per-stage program/model UI's PATCH/PUT handler is wired into the same invalidation call, not a
new one that forgets it (same failure shape as `insights-cost-pricing-gaps`'s "the fix must
touch all call sites, not just the visible one").
Per §4 of `backlog-configurable-pipeline`'s pitfalls research, the resolved value should also be
**captured once at triage/session-start time**, not re-read live at each of the three
consultation points (`WriteSlashCommands`, triage prompt builder, review-gate runner) — otherwise
a mid-flight program/model edit could have triage run under model A and review run under model B
for the same item, with no record of which config produced which cost entry.

### 1b. A model-name typo silently falling back with no visible error
`CachingPipelineEngine`'s existing pattern for an unresolved `pipeline_mode` slug is: log a
Warning and fall back to `DefaultPipelineEngine` (`session/pipeline_engine.go:344` area,
`"[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default"`). A per-stage
**model** override needs the equivalent guard, but the failure mode is worse here than for a
mode slug: an unrecognized model string handed to `headless.CallOptions.Model` or
`Instance.Program`/`SwitchProgram` doesn't necessarily "fail to resolve" the way a missing map
key does — it gets passed straight through to the CLI subprocess as a literal
`--model <typo>` argument, and the actual failure surfaces as a subprocess-level error (exit
code, malformed CLI response) rather than a clean "unknown mode" branch. This is the same
misattribution shape as **BUG-062** (`docs/bugs/fixed/BUG-062-...md`): an unvalidated string
threaded through to a subprocess produces a confusing, wrongly-attributed error far from the
actual bad input. Concrete guard: validate the model string against a known-good list (or at
minimum log the resolved model/program pair before every headless call) so a typo produces an
immediate, correctly-attributed rejection at config-save time, not a mysterious CLI failure
hours later during an autonomous run.

### 1c. Cost estimates becoming wrong when a model is deprecated/repriced
Direct, already-diagnosed precedent: `session/tokens/pricing.go`'s `DefaultPricingTable()` has
zero forcing function tying `IsStale()` (>30-day-old `EffectiveDate`) to any visible signal —
nothing calls it outside tests (`insights-cost-pricing-gaps` pitfalls §1). If per-stage cost
aggregation reads from the same pricing table (which it should, to stay consistent with the
Insights global total per this project's scope), it inherits that exact staleness risk, PLUS a
new one specific to this feature: **a stage's configured model can itself become invalid or
repriced** (e.g. a user pins `claude-opus-4` to the review stage, then that model is retired).
The existing `EstimateCost`/`ModelFamilyCost` silent "unknown family → $0" behavior (BUG-049,
BUG-050 below) means a stage pinned to a deprecated model would silently show $0 spend for that
stage forever — not "stale," but **invisible**, and specifically dangerous for a soft-budget
warning (§3) since $0 never crosses any threshold.

### 1d. "Free model" not actually being $0 (hidden costs)
Not literally true of Claude models in this codebase (no free tier tracked in
`DefaultPricingTable()`), but the *shape* applies directly to the headless-pool design (§2): a
model/program combination that hits rate limits produces **retries at the caller level** (the
30-minute triage budget, `MaxRemediationAttempts` backoff schedule) — each retry is a fresh
billable call. A per-stage cost view that counts only "successful" calls, not retried/failed
attempts that still consumed tokens before erroring, would undercount real spend. See §3's
double-counting-vs-undercounting discussion — this is the mirror-image risk (missing cost, not
duplicated cost).

---

## 2. Headless CLI subprocess execution across multiple programs (`claude`, `aider`, `gemini`)

### 2a. `claude -p`'s existing headless machinery is deep, and none of it is generic today
`session/headless/` (`pool.go`, `caller.go`, `runner.go`) is built specifically around
`claude -p --output-format stream-json` semantics: JSON-lines parsing, an `is_error` field
inspected from the CLI's own output (not the OS exit code — see BUG-080 below), and a
scan-loop structure (`firstCallScanState`) tuned to that exact streaming format. Feasibility
research (a separate research dimension) needs to confirm whether `aider`/`gemini` have an
equivalent scriptable/headless mode with **parseable, structured cost data** at all — this
pitfalls doc's finding is that *even if* they do, the parser is almost certainly a different
shape (different JSON schema, or no JSON at all — e.g. `aider --yes --message` prints
human-readable text unless a specific machine-output flag exists), so each adapter needs its own
stream parser, not a shared one. Do not assume `pool.go`/`caller.go`'s scan logic generalizes;
budget for genuinely separate per-program adapters behind a common interface, with `claude`
remaining the reference/fallback implementation per the requirements' explicit scope ("fall
back to Claude headless otherwise").

### 2b. Differing exit codes / error formats require per-program classification
`classifyHeadlessCallError` (`server/services/backlog_service_triage.go`) already required two
separate bug fixes to get right for `claude` alone:
- **BUG-080**: two of its branches (`process_error` for exit codes 1/2/130) were *dead code* —
  no call site ever actually translated `exec.Cmd`'s real OS exit code into those sentinels; the
  only signal ever inspected was the CLI's own `is_error` JSON field, an app-level signal
  distinct from the exit code. A new adapter for `aider`/`gemini` that *doesn't* emit a
  structured `is_error`-equivalent field would have **no error signal at all** unless the
  classifier is extended to actually read the subprocess exit code for that adapter — the
  existing code doesn't do this for any program today, so it must be built new, not reused.
- **BUG-062**: an unvalidated `WorkDir` (subprocess cwd) produces a *misattributed* OS-level
  error (`fork/exec <claude binary path>: no such file or directory` even though the real
  problem is a bad working directory) — `os/exec.Cmd.Dir`'s well-documented quirk. This is
  program-agnostic (same `os/exec` semantics for any subprocess), so a new `aider`/`gemini`
  adapter automatically inherits this failure shape and must apply the same eager
  `filepath.IsAbs` + `os.Stat` validation `TriggerTriage` already does — not rediscover it via a
  confusing "aider not found" report when the binary is actually installed fine.
- **BUG-050**: `capacity_monitor.go`'s *independent, second* pricing table already does
  substring model-name matching (`opus`/`haiku`/`flash`/`pro`) that silently mis-prices unmatched
  models at a wrong nonzero "sonnet default" rather than failing visibly — and it already reaches
  into Gemini's naming space (`flash`, `pro`). Any new program adapter's cost parsing must not
  add a third independent pricing surface; route through the same reconciled pricing source this
  project's cost-aggregation work establishes, or the codebase ends up with three drifting
  tables instead of the two `insights-cost-pricing-gaps` already found and only partially
  reconciled (ADR-002 explicitly deferred `capacity_monitor.go`'s unification — confirm this
  project either fixes BUG-050 as a prerequisite/adjacent fix or explicitly scopes it out with
  the same kind of ADR, rather than silently adding a fourth cost source).

### 2c. Subprocess hangs with no timeout — the `idleTimeout` backstop is `claude`-specific
`session/headless/pool.go`'s `idleTimeout` (10 minutes, `var idleTimeout = time.Minute`)
detects "no new stream-json output line" as a stall signal, independent of the caller's own
hard deadline (`triageCallBudget`, 30 minutes). This backstop exists **because `claude -p`'s
streaming output gives a reliable per-line progress signal** — a new adapter for a program that
doesn't stream JSON lines (e.g. a program that buffers and prints once at the end) has no
equivalent "still alive" signal to reset an idle timer against, so it would silently lose this
protection and rely solely on the caller's hard deadline. That's a real regression: the hard
deadline alone is exactly the shape that caused **BUG-093** (queue-wait counted against the
caller's own budget, misclassified as a genuine LLM timeout) — a program with no idle-progress
signal is more exposed to that failure class, not less, because there's no way to distinguish
"still working, slowly" from "genuinely stuck" short of the ultimate deadline firing. Any new
adapter needs an explicit design decision here: either find an equivalent progress signal for
that program (e.g. periodic heartbeat via `--verbose`) or accept "idle timeout == hard timeout"
for that adapter and document the tradeoff, rather than silently inheriting `idleTimeout`'s
name without its actual guarantee.

### 2d. Concurrent headless pool exhaustion if multiple programs share one pool
`session/headless/pool.go`'s `MaxConcurrentSessions` semaphore (5, `server/dependencies.go`) is
already a **single global limit shared by every current headless call site** (triage, review,
one-shot PR creation, autonomous-driver calls, approval-handler classification — 11 call sites
per BUG-093's own accounting). BUG-093 is the live, already-fixed incident of this pool being
oversubscribed by a *single* program (`claude`) during a bulk-import burst. Adding `aider`/
`gemini` as additional consumers of the **same 5-slot pool** multiplies the contention surface
without multiplying capacity — a burst of triage/review calls across multiple programs would
compete for the same 5 slots, and BUG-093's fix (bounding queue-wait to `maxQueueWait`, 2
minutes, separate from the caller's own budget) mitigates the *misdiagnosis* but not the
*throughput* problem. Concrete design question for the plan phase: does each program get its own
concurrency semaphore (avoids cross-program starvation, but multiplies total concurrent
subprocess load on the host), or do they share one pool sized for the sum (simpler, but a
`gemini`-heavy triage burst can starve a concurrently-running `claude` review)? Either way,
`classifyHeadlessCallError`'s `"pool_saturated"` bucket (BUG-093's fix) needs to stay accurate
per-program, not conflate "claude's pool is saturated" with "gemini's pool is saturated" if pools
are split, or continue working correctly if they're merged.

### 2e. The `SwitchProgram` interactive-work-stage path has its own distinct hazard class
For the work/implementation stage (interactive, not headless), `Instance.Program`/
`SwitchProgram` (`session/instance_program.go`) is the existing mechanism, and it already
encodes a subtlety this project must not break: switching *within* the Claude/Antigravity family
ports conversation history (`PortSessionHistory`); switching *across* families (e.g. `claude` →
`aider` or `claude` → `gemini`) explicitly clears conversation state
(`ClearConversationState`). A pipeline mode that assigns different programs to different stages
means a **work-stage program switch can happen mid-item**, exactly the cross-family case that
discards history — worth an explicit product decision (is losing conversation continuity when a
pipeline mode changes program between stages acceptable, given each stage is a fresh headless
call anyway for triage/review, but the *work* stage's interactive session is the one place
continuity has mattered before). `SwitchProgram` already serializes under `restartTriggerMu`
shared with `SetAutoApprove` specifically to prevent double-restart races if pipeline-mode
selection and another auto-approve/capacity-monitor trigger fire near-simultaneously — a new
pipeline-mode-driven program switch must go through this same method, not a parallel
"just set `Instance.Program` directly" shortcut, or it reintroduces the exact race this locking
was built to close.

---

## 3. Cost-aggregation / budget-warning design risks

### 3a. Double-counting on retry/resume
Every headless call site here already retries on failure (triage's 5-attempt remediation cap
with a 30m/2h/8h/24h/72h backoff schedule, `MaxRemediationAttempts`). If per-stage cost
aggregation sums "every headless call attributed to this stage" without distinguishing a
genuinely new attempt from a resumed/retried one that partially ran before failing, a stage
that needed 3 retries to succeed would show 3x its real productive cost — which may in fact be
*correct* (each retry really did burn tokens) but must be labeled as such (e.g. "3 attempts,
$X total") rather than presented as if the stage cost $X on a single clean run, since the two
readings lead to different conclusions about whether the per-stage config is well-tuned.
Conversely, per `insights-cost-pricing-gaps`' own §3 finding about `EstimateCost`'s "single pass
discards which families were skipped" — the new `SessionRole` breakdown dimension must survive
the *existing* daily aggregation rollup (`insights_service.go`'s `modelFamilyCostsForDay`) which
already sums *before* exposing any completeness signal; adding a role dimension on top of an
aggregation that already has this "signal doesn't survive the rollup" gap compounds rather than
fixes it unless addressed together.

### 3b. Budget warning based on stale/cached data (ADR-029 precedent)
`ADR-029-session-role-batched-snapshot-not-live-join.md` (`insights-session-visibility`) already
established the pattern this project's soft-budget-warning feature will reuse: `session_role` is
fetched via a **batched per-request snapshot**, not a live join, and is explicitly documented as
having an "eventually consistent, refreshed on next JSONL write or full page reload" staleness
profile — accepted because `session_role` is write-once/immutable in production. A **budget
threshold check is a fundamentally different consistency requirement**: unlike a read-only
display field, a soft budget warning's entire value is in firing promptly when a threshold is
crossed. If the same batched-snapshot pattern is reused for budget checks (i.e., the warning is
computed from the same periodically-refreshed Insights snapshot rather than checked at the
moment each headless call's cost is recorded), a stage could cross its configured threshold and
the warning would not appear until the next snapshot refresh — silently under-warning for
however long that refresh interval is. Recommend: budget-threshold evaluation happens
**inline, at the point cost is recorded per call** (mirroring how `CapacityMonitor.checkThresholds`
already does inline threshold checking against `CostBudgetUSD` today, not via a batched
snapshot), not via the Insights read-path's batched/cached shape — those are different
consistency needs served by the same underlying cost data, and conflating them risks a
warning that's silently late.

### 3c. Floating-point summation drift across many small per-call costs
Every current cost path already sums `float64` per-call amounts with no `math/big` or
fixed-point representation (`EstimateCost`, `ModelFamilyCost`, `CapacityMonitor.estimateCost`).
For a single item's lifetime aggregate this is unlikely to matter in absolute dollar terms (the
error is sub-cent-scale even after thousands of calls), but the requirement here — a *drillable*
Insights view where a global total, a per-stage subtotal, and a per-item subtotal must all be
internally consistent (per the requirements' explicit "consistent with existing global total") —
means the same underlying per-call values get summed multiple times along different dimensions
(by day, by model family, now also by role/stage, and by backlog item). If each dimension's
rollup independently re-sums from a different intermediate representation (e.g. one path sums
already-rounded-to-cents display values, another sums raw float64), the displayed subtotals can
fail to add up to the displayed total by a cent or two — a visible, confusing "the parts don't
sum to the whole" bug distinct from FP-precision-loss in the mathematical sense. Concrete guard:
derive every new stage/role breakdown from the *same* underlying per-call cost values the
existing global total already sums from (`EstimateCost`'s per-family loop), not a separately
recomputed figure, so drift can't be introduced between the two views.

### 3d. Feedback-loop risk: a wrong per-stage cost driving an automated decision
`insights-cost-pricing-gaps` pitfalls §6 already found and filed (BUG-050) a live case of this:
`CapacityMonitor.checkThresholds`'s `cost_budget_exceeded` trigger can pause/stop a live
autonomous session based on a *second, independent, more dangerous* pricing table than the one
Insights uses. This project's soft-budget-warning feature is a **new instance of the same
pattern-class** — a computed cost figure driving user-visible or automated behavior — so it
should not be built by adding a *third* cost-computation path. If the soft-budget-warning here
and `CapacityMonitor`'s existing hard-ish budget check end up as two independently-maintained
implementations of "compare accumulated cost to a threshold," that's the same drift risk
`insights-cost-pricing-gaps` flagged for pricing tables, just one layer up (threshold-comparison
logic instead of per-token pricing). Recommend the plan phase make an explicit call: either reuse
`CapacityMonitor`'s threshold-comparison shape for the new per-stage/per-item soft warning, or
document why a separate implementation is warranted (e.g. soft warning is advisory/UI-only,
`CapacityMonitor`'s is enforcement) — but state it, rather than silently duplicating.

---

## 4. Single-user local tool: what to design against, and what to explicitly skip

This project's own requirements already scope out multi-tenant cost tracking — correctly, per
this repo's established "personal, single-user tool" framing (ADR-029's Known Inefficiencies
section makes the identical call: "this is a personal single-user tool... where the absolute row
count is small," and defers a fix to "if this ever becomes a measured problem"). Apply the same
proportionality here:

- **Skip**: per-tenant quota isolation, rate-limiting other users' access to the pool, auth/authz
  around who can configure per-stage overrides, encryption of cost data at rest. None of this
  has a threat model in a single-user local tool.
- **Real risk instead — a config mistake silently costing real money**: the concrete failure
  modes worth designing against are all in §1-§2 above, restated as the actual "harm model" for
  this tool:
  1. A typo'd model string (§1b) silently running headless calls against an unintended
     (possibly much more expensive, e.g. `opus` instead of `haiku`) model for every triage/review
     on an item, with no validation catching it before the first real call.
  2. A deprecated/unpriced model (§1c) making a stage's real spend invisible ($0 display) rather
     than merely stale — the single most dangerous shape here, since it defeats the entire
     purpose of building per-stage cost visibility in the first place, and is a *known,
     already-happened* bug pattern in this exact codebase (BUG-049, BUG-050).
  3. A soft-budget-warning that looks like it's protecting against overspend but is actually
     silently not firing promptly (§3b) — worse than no warning at all, since a user who trusts
     the (broken) warning will not otherwise notice.
  4. **"Silently doing nothing"** — the mirror-image risk explicitly named in the task: an
     unresolved/misconfigured per-stage override falling back to `DefaultPipelineEngine`
     behavior with only a Warn-level log line (the existing pattern for `pipeline_mode` slugs)
     is *appropriate* for keeping the pipeline running, but must not be the *only* signal. Given
     this is a solo-maintainer tool where nobody is watching logs in real time, the existing
     "log a Warning and fall back" pattern (adequate for a slug typo, which just re-runs default
     templates) is **not adequate on its own for a model/program override typo**, because the
     consequence isn't "runs the default templates" — it's "runs against the wrong, possibly
     costly model" or "the headless call fails outright with a misattributed error" (§1b/§2b).
     Recommend the per-stage override surface fail loudly at config-save time (validate against
     a known-model/known-program list before persisting), not just at call time via a log line.

---

## Summary of concrete, actionable risks for the plan phase

1. Per-stage program/model overrides on `PipelineMode` must be wired into the **same
   `InvalidateCache` call** every other `PipelineMode` field mutation already requires — a
   forgotten wiring point silently serves stale overrides (§1a).
2. A typo'd model/program string must be validated at config-save time, not left to surface as a
   confusing subprocess-level error later (§1b) — this repo has two directly on-point precedents
   for "unvalidated string reaches a subprocess and produces a misattributed error"
   (BUG-062) and "silent wrong-nonzero-cost fallback" (BUG-050).
3. Building `aider`/`gemini` headless adapters requires **new** per-program error classification
   and idle-timeout-equivalent stall detection — `claude`'s existing `classifyHeadlessCallError`
   and `idleTimeout` do not generalize; both were built and bug-fixed (BUG-080, BUG-093)
   specifically around `claude -p`'s JSON-stream shape (§2a-§2c).
4. Decide explicitly whether new programs share the existing 5-slot `MaxConcurrentSessions` pool
   or get their own — BUG-093's fix only addressed misdiagnosis of pool saturation, not the
   underlying throughput math, which gets worse as more programs compete for the same slots
   (§2d).
5. Do not let per-stage cost aggregation become a **third** independent pricing/threshold
   computation path alongside `session/tokens/pricing.go` and `capacity_monitor.go`'s existing
   two (already-diverged) tables — reconcile with or explicitly build on the canonical
   `PricingTable`, and treat BUG-050/BUG-049 as adjacent prerequisite risk, not unrelated
   background noise (§2b, §3d).
6. Soft-budget-warning threshold checks must be evaluated **inline at cost-recording time**, not
   sourced from the same batched/cached snapshot pattern `ADR-029` correctly established for
   read-only Insights display — those have different staleness tolerances, and reusing the
   display pattern for a warning risks a silently-late warning (§3b).
7. New stage/role cost breakdowns must be derived from the same underlying per-call values the
   existing global total sums, not independently recomputed, to keep drilldown subtotals summing
   to the displayed total (§3c).
8. Given the single-user context, skip multi-tenant/auth defenses entirely, but treat "config
   mistake silently costs real money" and "config mistake silently does nothing visible" as the
   two real threat models to design against explicitly (§4).
