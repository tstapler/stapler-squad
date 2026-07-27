# Pitfalls & Risks Research — insights-cost-pricing-gaps

Agent 4 (Pitfalls), SDD Phase 2. Companion to `requirements.md`.

## 1. Hardcoded third-party pricing tables — general failure modes

`session/tokens/pricing.go`'s `DefaultPricingTable()` already exhibits several of the classic
risks, which the fix should not just patch for `claude-sonnet-5` but harden against recurring:

- **Staleness with no forcing function.** Every entry carries `EffectiveDate: "2026-05-15"` and
  `IsStale()` (lines 185-200) already detects entries >30 days old — but nothing *calls*
  `IsStale()` anywhere outside `pricing_test.go`. A correctly-dated-but-wrong price is
  indistinguishable from a correct one until someone manually notices. G4's guardrail should
  wire `IsStale()` into something visible (log warning at startup, or surfaced in the Insights UI
  itself via the existing `PricingAsOf` field already returned at `insights_service.go:273`)
  rather than leaving it an inert helper.
- **Wrong unit / decimal-shift errors.** Prices are per-million-tokens (`InputPricePerMTok` etc.)
  and every cost computation divides by `1_000_000.0` consistently across `EstimateCost` and
  `ModelFamilyCost` — good. But this is exactly the kind of invariant that's easy to violate when
  adding a new entry by hand (e.g. pasting an official per-1K-token price into a per-1M field is a
  1000x error, or vice versa). A unit test that sanity-checks new entries fall within a plausible
  band (e.g. `0 < InputPricePerMTok < 200`) would catch a fat-fingered decimal shift that a plain
  "entry exists" test would not.
- **Missing cache-read/cache-write tiers.** The struct already has `CacheWritePerMTok` /
  `CacheReadPerMTok` and both are used in every cost computation — but nothing enforces that a
  *newly added* entry populates them. A zero-value `ModelPricing{InputPricePerMTok: X,
  OutputPricePerMTok: Y}` with cache fields left at their Go zero-value (`0.0`) compiles fine and
  silently undercounts cost for any model with heavy cache usage (Claude models normally have
  substantial cache-read volume) — this is a *quieter* version of the exact bug this project is
  fixing, just partial instead of total.
- **Batch API / prompt-caching discount tiers not modeled at all.** The current `ModelPricing`
  struct has no field for batch-API discounted rates. If usage data from `TurnTimeline` doesn't
  distinguish batch vs. interactive calls, this is a known-and-accepted gap, not a regression —
  but worth an explicit non-goal note if it comes up in scoping, since "match Anthropic's public
  pricing exactly" is a deeper rabbit hole than table maintenance.
- **Currency/rounding is not a live risk here** — everything is USD, and `fmtCost` in
  `insightsFormatters.ts` is presentational only; no evidence of currency conversion anywhere in
  the pipeline.
- **A second, independent, drifted pricing table already exists in this codebase** —
  see §5, this is the most concrete "staleness/duplication" risk found, not a hypothetical.

## 2. The "silently swallow unknown key → zero" pattern — direct precedent in this repo

This bug class (a lookup that returns a zero-value/no-op instead of a visible error, contributing
silently wrong aggregate state) is not new to this codebase. `docs/tasks/backlog-feature-improvement.md`
(most recently updated 2026-07-27, commit `5839ffd30`) documents a **10th-plus recorded instance**
of a structurally identical shape in the backlog/session-lifecycle domain, termed
"swallowed status-transition": *a status-mutating call fails, the error is only logged, the
caller/RPC still reports success, and no sweep exists to catch the resulting mismatch between
recorded state and reality.* That doc's explicit recommendation after the 10th recurrence is
directly applicable to G4 here:

> "don't just patch these four call sites — use `quality:reflect-and-fix`'s taxonomy to find the
> earliest enforceable rung (a lint rule flagging '...with no caller-visible signal', a shared
> helper that makes silent failure structurally impossible, or a test asserting every call site
> either propagates or explicitly justifies swallowing) rather than adding an Nth
> individually-patched instance."

The pricing bug is the same shape one layer over: `pricing, ok := pt.Prices[family]; if !ok {
continue }` in both `EstimateCost` (line 172) and `ModelFamilyCost` (line 213/226) is a silent
"skip and contribute nothing" — functionally identical to "log-and-continue" in the backlog code,
just without even the log line. **Risk for this project specifically**: if G1/G2/G4 only add the
missing `claude-sonnet-5` entry and one test asserting *that specific* model is priced, this
repeats the exact "individually-patched instance, shape recurs at the next model release" failure
mode the backlog doc warns about. G4 should target the *shape* (any unknown family silently
producing $0 with no signal) not just the *instance* (sonnet-5 missing).

## 3. Retrofitting "distinguish unpriced from free" onto already-aggregated code

Both `EstimateCost` and `ModelFamilyCost` build per-family token maps first, then sum/lookup
pricing per family in a **single pass that discards which families were skipped**. Concretely:

- In `EstimateCost` (lines 168-180), the `for family, inputTok := range modelInputs` loop's `if
  !ok { continue }` branch has no side channel — by the time the function returns a single
  `float64`, there is no way for the caller to know whether `total` reflects "all usage was
  priced" or "some families were silently excluded." **Retrofitting requires changing the return
  type/signature**, not just adding a field somewhere — e.g. returning `(total float64, unpriced
  []string)` or a small result struct. This is a breaking API change to an exported method with
  at least 3 call sites (`insights_service.go:115, 314`, and `ModelFamilyCost` at 179/192).
- **Regression risk on the already-working priced-model path**: the natural refactor is to
  restructure the loop to always record which families hit `!ok`, which means touching the exact
  loop body that currently produces correct, tested totals for known models
  (`TestEstimateCost_WhenKnownModel_ExpectExactPrice`,
  `TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded`). A careless refactor (e.g.
  changing the accumulation order, or accidentally double-counting a family that appears in both
  the `TurnTimeline` loop and the `PrimaryModel` fallback branch at lines 160-166) would silently
  break a currently-passing, currently-correct cost number for known models — a strictly worse
  regression than the bug being fixed, since it would corrupt *priced* data rather than just
  leaving *unpriced* data at zero. **Mitigation**: keep the existing per-family loop structure
  intact and additively collect the skipped-family set alongside it, rather than restructuring
  control flow; run the existing `pricing_test.go` suite (already has both known-model and
  unknown-model cases) as a regression gate before/after.
- `ModelFamilyCost`'s two code paths (per-turn loop at 210-221, and the `PrimaryModel` fallback at
  224-234) independently repeat the same `pricing, ok := ...; if !ok { continue/skip }` logic —
  any unpriced-signal change has to be made in both places or the fallback path (used when
  `TurnTimeline` is empty) will silently regress back to the old behavior while the primary path
  is fixed. Same duplication risk applies to `EstimateCost`'s fallback branch at lines 160-166 vs.
  its main loop.
- The daily-cost aggregation path (`insights_service.go` line ~179, "modelFamilyCostsForDay") sums
  `ModelFamilyCost()` results across sessions *per day* before the caller sees them — if the
  unpriced signal is only surfaced per-session and this daily rollup doesn't propagate it, a day
  with one unpriced-model session would still render as a clean (if lower-than-real) total with no
  indication anything was excluded. The unpriced signal needs to survive the daily aggregation
  step, not just the per-session one — worth confirming with Agent covering the backend/API
  design dimension.

## 4. Override mechanism (G3) risks

- **Concurrency / hot-reload race.** `PricingTable.Prices` is a plain `map[string]ModelPricing`
  with **no mutex or synchronization anywhere** in `session/tokens/types.go` or `pricing.go`.
  `InsightsService` holds a single `*tokens.PricingTable` pointer set once at construction
  (`server/dependencies.go:1070`, `NewInsightsService` at `insights_service.go:28-34`) and never
  reassigns it. If G3's file-watch approach mutates `table.Prices[family] = pricing` in place on
  a live table (as `LoadPricingOverride` currently does at line 98) while request goroutines are
  concurrently ranging over `pt.Prices` in `EstimateCost`/`ModelFamilyCost`, that is an
  **unsynchronized concurrent map read/write — a Go runtime fatal error (`fatal error: concurrent
  map read and map write`), not just a data race**, guaranteed to crash the process under any
  real concurrent insights traffic once a reload fires.
    - This repo's `.claude/rules/go-double-checked-locking.md` documents the adjacent pitfall
      ("return the locally-computed value, not the cache slot" after a lock-protected
      read-compute-write) — directly relevant in spirit but not a literal fit, since that rule is
      about *returning stale results after a race*, not *crashing on concurrent map access*. The
      correct pattern here is closer to the `go-concurrency` skill's **copy-on-write /
      `atomic.Pointer` swap** idiom: on reload, build a brand-new `*PricingTable` (or a new
      `Prices` map) off to the side, then atomically swap the pointer (`atomic.Pointer[PricingTable]`
      or equivalent) that readers dereference — never mutate a table that's already been handed to
      readers. `InsightsService.pricing` would need to become an `atomic.Pointer[tokens.PricingTable]`
      (or a getter method backed by one) instead of a plain struct field for this to be safe.
    - If G3 instead choses "reload only at process start / no live file-watch" (simplest, safest),
      this entire class of risk is avoided — worth flagging as the lower-risk option unless a
      live-reload requirement is explicit in the requirements (it is not; G3's own wording says
      "without a full code change + redeploy," which a restart-to-reload config file already
      satisfies without introducing hot-reload concurrency at all).
- **Path/permission issues with an env-var-configured path.** `LoadPricingOverride(configPath
  string)` already does a plain `os.ReadFile` with a `//nolint:gosec` — if wired via an env var
  (e.g. `STAPLER_SQUAD_PRICING_OVERRIDE_PATH`), typical failure modes: (a) path doesn't exist yet
  on a fresh install — must fail soft (fall back to defaults + log) not hard (crash startup), (b)
  relative vs. absolute path ambiguity if the working directory differs between `go run` / systemd
  unit / test harness (see `.claude/docs/state-isolation.md` conventions for per-instance state
  dirs — worth following the same `STAPLER_SQUAD_INSTANCE`-scoped pattern so a manual test
  instance doesn't clobber the live deployed instance's override file), (c) malformed JSON in the
  override file should not take down cost reporting for *all* models, only fail to apply the
  override — `LoadPricingOverride`'s current `json.Unmarshal` failure returns `nil, err` and
  discards the entire table including the valid defaults; a partial-failure-tolerant version
  should keep defaults intact and only skip/report the malformed override.

## 5. G4 guardrail — too weak vs. too strong

- **Too weak, concretely possible**: a test that hardcodes today's known model list (e.g.
  `assert pricing table contains ["claude-opus-4", "claude-sonnet-4", ..., "claude-sonnet-5"]`)
  only proves *today's* gap is closed — it does nothing to catch the *next* new family, which is
  the actual bug class per §2. This is the "individually-patched instance" failure mode already
  called out in the backlog doc's own reflect-and-fix recommendation.
- **Stronger, still cheap options** (favor these per AC-5's phrasing "extending... as needed"):
  - A test that asserts every family *actually observed* in some representative/live corpus of
    recent session transcripts (or a fixture list maintained separately from the pricing table)
    has a pricing entry — fails when a genuinely new model shows up in real usage data, not on a
    fixed guess-list.
  - Have `EstimateCost`/`ModelFamilyCost` (post-G2 change) expose the unpriced-family set, and add
    a *runtime* warning/metric (not just a test) whenever it's non-empty — this catches the next
    new model in production the moment it's first used, which is strictly earlier and more
    reliable than any CI-time fixture list.
- **Too strong / noisy failure mode to avoid**: a CI check that fails the *build* (not just a
  specific pricing test) whenever `NormalizeModelFamily` encounters any string it doesn't
  recognize as one of a fixed enum — that would break CI on unrelated PRs the moment Anthropic (or
  any other tracked provider) ships a model, since model IDs show up in test fixtures/session logs
  unrelated to pricing work. Keep the guardrail scoped to the pricing/insights test suite and/or a
  runtime signal, not a repo-wide lint gate.
- **Process guardrail as a complement, not instead of**: AC-4/AC-6 already call for a
  docs/registry update — pairing that with a short "how to add a new model's pricing" doc note
  (where, what fields, how to verify) lowers the friction/memory burden the *next* time a human
  or agent has to update this table, similar in spirit to why this rule doc itself exists.

## 6. Feedback-loop risk — wrong/stale pricing feeding other automated decisions

Grepped for `budget`/`Budget` usage across the backend outside insights: **confirmed a real,
currently-live feedback loop**, and it does **not** go through `session/tokens/pricing.go` at all:

- `config/types.go:257-258` — `CostBudgetUSD float64` ("accumulated USD cost limit... Default: 0"),
  consumed by `server/services/capacity_monitor.go:243` (`checkThresholds`): when
  `EstimatedCostUSD >= CostBudgetUSD`, it triggers `"cost_budget_exceeded"` and calls
  `handleTransitionTrigger`, which can pause/stop an autonomous session.
- `EstimatedCostUSD` is computed by `CapacityMonitor.estimateCost()` at
  `server/services/capacity_monitor.go:351-371` — **a second, entirely separate, hardcoded pricing
  table**, independent of `session/tokens/pricing.go`'s `DefaultPricingTable()`:
  ```go
  func (m *CapacityMonitor) estimateCost(model string, input, output int64) float64 {
      inputPrice := 3.0   // sonnet default
      outputPrice := 15.0 // sonnet default
      model = strings.ToLower(model)
      if strings.Contains(model, "opus") { inputPrice = 15.0; outputPrice = 75.0 }
      else if strings.Contains(model, "haiku") { inputPrice = 0.25; outputPrice = 1.25 }
      else if strings.Contains(model, "flash") { inputPrice = 0.075; outputPrice = 0.3 }
      else if strings.Contains(model, "pro") { inputPrice = 1.25; outputPrice = 5.0 }
      return (float64(input)*inputPrice + float64(output)*outputPrice) / 1_000_000.0
  }
  ```
  This is **worse than the Insights bug**, not better: instead of a visible-if-you-look $0, an
  unrecognized model (e.g. `claude-sonnet-5`, since the substring checks only match
  `opus`/`haiku`/`flash`/`pro` — a "sonnet" model with no other qualifying substring falls through
  to the "sonnet default" `3.0`/`15.0`, which may be *wrong* for the real model rather than merely
  missing) **silently mis-prices at whatever the nearest substring match happens to be**, with no
  cache-tier accounting at all, and directly drives an automated pause/stop decision on live
  autonomous sessions via `CostBudgetUSD`.
- **This is squarely the "feedback loop into other automated decisions" risk the task asked about,
  and it's real today, not hypothetical**: stale/wrong pricing here doesn't just mis-render a
  chart, it can cause `CapacityMonitor` to either (a) never trip `cost_budget_exceeded` for a model
  whose real per-token cost is much higher than any of the four substring buckets (cost overrun
  risk — the OOM-incident-style failure this repo has WIP-cap precedent for guarding against per
  `feedback_backlog_wip_limit.md`), or (b) trip it too early/wrong for a model that happens to
  match the wrong substring.
- **Scoping call for the plan phase**: the requirements/AC's don't explicitly mention
  `capacity_monitor.go`, and unifying the two pricing tables is a larger change than "add missing
  entries to `DefaultPricingTable()`." Recommend the plan phase make an explicit, documented
  decision: either (a) fold `CapacityMonitor.estimateCost` into `tokens.PricingTable` as part of
  this project (closes the feedback-loop risk directly, but expands scope beyond the stated
  non-goals which only exclude "live pricing API integration," not table unification), or (b)
  explicitly flag it as an adjacent, out-of-scope bug for a fast-follow (`sdd:fix-bug` or
  `pm:log-bug`), same as this project's own non-goal note on historical cost backfill. Silently
  leaving it unaddressed without a decision recorded risks it being mistaken as "already covered"
  once G1-G4 ship, since both tables live under the umbrella of "session/model cost estimation."

## 7. LLM-agent-driven / autonomous-codebase-specific risk

Beyond the `CostBudgetUSD` feedback loop (§6), this repo is itself the tool that manages Claude
Code sessions for its own development — the "insights" data this project fixes is partly about
*this repo's own agent-driven work* (see `.claude/rules/*` referencing backlog automation, WIP
caps, remediation backoff). Two second-order risks worth flagging:

- If a future SDD/autonomous pass reads Insights cost data to make a decision (e.g. "which model
  family is most expensive, dial back its usage") before G2/G4 land, an unpriced model reading
  $0 would look *cheapest*, not *unknown* — actively steering automated decisions in the wrong
  direction. No such consumer exists in the code today (grep found none), but it's a plausible
  next feature given this repo's trajectory (see `docs/registry/` and `backlog-feature-improvement`
  skill's emphasis on more automation), so G2's "unpriced" signal should be a first-class,
  machine-readable field (not just a UI string) for exactly this reason — cheap to do now,
  expensive to retrofit once a consumer depends on the old ambiguous-zero shape.
- The regression-guardrail discussion in §5 is doubly important in an agent-driven repo: a future
  Claude Code session adding a new model integration (e.g. a new provider) has no natural prompt
  to touch `session/tokens/pricing.go` unless something *forces* the omission to be visible
  (test failure, runtime warning) — an agent won't "remember" to update a pricing table the way a
  long-tenured human engineer eventually would from repeated pain. This is the same argument the
  backlog-feature-improvement doc makes for preferring structural fixes over judgment-dependent
  process fixes.

## Summary of concrete, actionable risks for the plan phase

1. Both `EstimateCost` and `ModelFamilyCost` have **duplicated skip-on-unknown logic across a
   primary loop and a fallback branch each** — any unpriced-signal change must touch all four
   spots or one path will silently regress.
2. Retrofitting the unpriced signal is a **breaking signature change** on two exported methods
   with 3+ call sites; keep the existing per-family loop structure intact and additively track
   skipped families rather than restructuring, to avoid corrupting the already-correct priced-model
   totals.
3. If G3 does live hot-reload, the shared `map[string]ModelPricing` **must** move to an
   atomic-pointer-swap / copy-on-write pattern — in-place mutation of a `*PricingTable` already
   in use by concurrent request goroutines is a guaranteed crash, not just a race.
4. `LoadPricingOverride`'s malformed-JSON path currently discards the entire default table on
   error — should fail soft and keep defaults if wiring this up live.
5. G4's guardrail should target the *shape* (any unrecognized family silently produces zero
   cost) via a data-driven or runtime-signal check, not a hardcoded "today's known models" list —
   per direct, repeated precedent in `docs/tasks/backlog-feature-improvement.md`'s
   "swallowed status-transition" findings (10+ recorded recurrences of the identical
   silent-failure shape in this same codebase).
6. **`server/services/capacity_monitor.go:351-371` has an independent, more dangerous, hardcoded
   pricing table** (substring-matched, silently mis-prices unrecognized models via a "sonnet
   default" fallback rather than $0) that drives a real automated action (`CostBudgetUSD` /
   `cost_budget_exceeded` pausing live autonomous sessions) — this is the concrete feedback-loop
   risk the task asked about, already live in production, and needs an explicit
   in-scope-vs-fast-follow decision during planning rather than being silently left unaddressed.
