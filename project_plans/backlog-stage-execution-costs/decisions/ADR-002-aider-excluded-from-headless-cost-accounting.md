# ADR-002: Exclude Aider from Headless Cost Accounting; Gemini CLI Gets a Full Adapter

## Status
Accepted

## Context
The requirements doc's Rabbit Hole asked which currently-supported programs
(`claude`, `aider`, `gemini`, others via `config.GetAvailablePrograms()`) can
realistically run headless (non-interactive) with a parseable cost. Three
independent research passes (stack.md, architecture.md, build-vs-buy.md) all
verified the same facts against each CLI's own documentation (2026-09-17):

- **Gemini CLI**: `gemini -p "<prompt>" --output-format json` returns a
  single, well-formed JSON object on stdout with per-model token counts
  (`stats.models[*].tokens`), confirmed against the
  [headless mode docs](https://google-gemini.github.io/gemini-cli/docs/cli/headless.html).
  No dollar-cost field, but token counts are enough to compute cost via a new
  pricing-table entry (mirrors how every other adapter already works).
- **Aider**: `aider --message "<prompt>" --yes` runs non-interactively but
  has **no `--output-format json` equivalent at all** — confirmed against
  [aider's scripting docs](https://aider.chat/docs/scripting.html). The only
  structured cost/token data source is a separate, explicitly-opt-in
  side-channel: `--analytics-log <file> --no-analytics` writes a local JSONL
  file whose `message_send` events carry `cost`/`total_cost`/token fields
  (verified against
  [aider's own sample-analytics.jsonl](https://github.com/aider-ai/aider/blob/main/aider/website/assets/sample-analytics.jsonl)).
  This requires a fundamentally different adapter shape (spawn subprocess →
  wait for exit → tail/parse a sidecar temp file → sum possibly-multiple
  `message_send` events) than every other adapter's "parse one JSON blob
  from stdout" shape, plus temp-file lifecycle management.

## Decision
Ship a `GeminiCaller` headless adapter (`session/headless/gemini_caller.go`)
implementing the existing `headless.PoolClient` interface. **Do not** build
an Aider headless adapter in this project. If a `PipelineMode`'s triage or
review stage is configured with `program: "aider"`, that configuration is
**rejected at settings-save time** (`session.ValidatePipelineModeContent`,
extended per plan.md Story 1.3.1) with an explicit error — not silently
accepted and then falling back at call time. Aider remains fully usable for
the **interactive work stage** (`Instance.Program`/`SwitchProgram` already
supports it; this project changes nothing there), since that path has no
cost-accounting requirement this ADR is about.

## Rationale
1. **Effort asymmetry is real, not marginal.** Gemini's adapter is "shell out,
   parse one JSON blob, look up a pricing table" — the same shape every
   existing headless call already has. Aider's adapter requires new
   subprocess-lifecycle machinery (a per-call temp file, event-type
   filtering, summing potentially-multiple LLM round-trips per invocation)
   that no other adapter in this codebase needs, for the sole benefit of one
   program.
2. **Silent-zero risk is worse than a loud rejection.** Per pitfalls
   research, an adapter that cannot produce a trustworthy cost must never
   report `$0.00` — that would look like a free stage and would silently
   defeat the entire point of this project's soft-budget-warning feature
   (a stage that actually costs money would never cross its threshold). A
   fragile stdout-regex-scrape approach (the only alternative to the
   sidecar-file protocol) risks exactly this failure mode on every Aider CLI
   upgrade. Rejecting the configuration outright at save time is strictly
   safer than shipping a parser that might silently degrade to zero.
3. **Matches the requirements doc's own stated scope boundary.** "Building
   headless support for a program that has no scriptable non-interactive mode
   at all" is explicitly out of scope — Aider's `--message` flag *is*
   scriptable, but its cost/token output is not machine-parseable through
   that path, which is the same practical outcome for this project's
   purposes (visibility + soft budget warnings, both of which require a
   trustworthy cost number).

## Consequences
- `PipelineEngine.ExecutorFor`'s headless-caller resolution (plan.md Story
  2.3.1) validates the resolved `program` against a small allow-list
  (`"claude"`, `"gemini"`, `""`/default) for **headless roles only**
  (triage, review) — an unrecognized or explicitly-excluded program name
  falls back to the Claude headless caller with a Warn log naming the item,
  stage, and rejected program, exactly mirroring
  `CachingPipelineEngine`'s existing unresolved-slug fallback discipline.
- The `work` stage role has no such allow-list — Aider (and any other
  `Instance.Program`-compatible value) remains fully selectable there.
- If Aider's official CLI ever adds a `--output-format json`-equivalent flag,
  or if a future project decides the sidecar-file protocol is worth the
  investment, this decision should be revisited as a new, independently
  scoped project — not silently reopened mid-implementation here.

## Addendum (2026-09-17, plan-repair pass): three consequences of the
## "silent-zero is worse than a loud rejection" principle that the original
## plan didn't fully close

Two independent reviews (architecture, adversarial) of `implementation/plan.md`
found that the Gemini adapter this ADR approves didn't yet honor this ADR's
own Rationale #2 in three concrete ways. Recorded here rather than silently
fixed only in plan.md, since all three are direct extensions of this ADR's
reasoning, not new topics:

1. **The `priced` signal had no channel to reach `ItemSession`.**
   `headless.PoolClient.CallBlocking`'s `CostSink` (`func(usd float64)`) had
   no way to say "this call was unpriced" — when `GeminiCaller` doesn't call
   `sink` (an unpriced model family), the caller's cost variable silently
   stays at Go's zero value, indistinguishable from a genuinely free call.
   This is exactly the failure this ADR's Rationale #2 says must never
   happen, just relocated from Aider (excluded above) to Gemini (shipped).
   Fixed by extending `CostSink` to `func(usd float64, priced bool)` and
   threading `priced` through to a new `ItemSession.cost_priced` field
   (plan.md Epic 2.5), consumed by the Insights role/item breakdown
   (Epic 4.1) and `StageCostChart`'s unpriced indicator (Epic 5.2) — the
   same abstain-rather-than-guess convention `session/tokens/pricing.go`'s
   `EstimateCost`/`ModelFamilyCost` and `ModelBreakdownChart.tsx`'s
   `pricingUnavailable` prop already use elsewhere in this codebase.
2. **No concurrency bound.** `GeminiCaller` is a bare `exec.CommandContext`
   adapter, not built on `headless.Pool`, so it inherited none of `Pool`'s
   `MaxConcurrentSessions` semaphore (BUG-093 precedent: this codebase has
   already been burned once by an unbounded pool). Fixed by giving
   `GeminiCaller` its own semaphore, sized to the same `5` already hardcoded
   for Claude's `Pool` at `server/dependencies.go:741` (plan.md Story 3.1.1,
   Task 3.1.1a/b).
3. **Availability was checked once, at startup, never again.** A `gemini`
   binary present at `PipelineMode`-save time but later removed from `$PATH`
   would silently downgrade every subsequent call to paid Claude, with only
   a Warn log as the sole signal — the same silent-substitution failure
   shape this ADR's Rationale #2 exists to prevent, just moved from "cost
   reporting" to "which program actually ran." Fixed by re-probing
   availability at call time (`GeminiCaller.Available()`, short-TTL cached)
   and persisting a visible, UI-surfaced fallback marker
   (`ItemSession.configured_program`/`executor_fallback_reason`) alongside
   the existing Warn log, rather than relying on the log alone (plan.md
   Story 2.3.1, Story 2.1.2).

No change to this ADR's core Decision (ship `GeminiCaller`, exclude Aider) —
these are hardening fixes to the adapter's implementation, not a revisit of
which programs get adapters.
