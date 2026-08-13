# Plan — "Sonnet 5 costs blank"

## Executive summary

Insights (and Backlog) cost displays show `$0.0000` for Claude Sonnet-5-generation
sessions because `session/tokens/pricing.go`'s `DefaultPricingTable()` only hardcodes
opus/sonnet/haiku for generations 3 and 4 — there is no gen-5 entry, and
`EstimateCost`/`ModelFamilyCost` silently `continue` past any model-family miss instead
of surfacing it. The fix is a small, precedented table addition (no regex change needed
— `variantSuffixPattern` already generalizes over the generation digit), plus a new
narrow "unpriced model" signal threaded from the pricing package through Insights (and
verified against Backlog's independent cost surfaces) so the next new model generation
fails visibly instead of silently under-reporting spend.

## Implementation approach

1. **Ground the fix in reality first.** Before touching any code, grep a real
   `~/.claude/projects/*.jsonl` transcript for the literal `"model"` string on the
   session that triggered this bug report. The two research docs (architecture.md §3,
   pitfalls.md §1) disagree on whether "Sonnet 5" means a true new top-level generation
   (`claude-sonnet-5-...`, unmapped today → silent $0, matches the bug) or shorthand for
   the already-shipped "Sonnet 4.5" (`claude-sonnet-4-5-...`, which today *already*
   collapses correctly into the existing `claude-sonnet-4` price via
   `variantSuffixPattern` — so if that's the real case, no map entry is missing and the
   bug must be something else, e.g. a genuinely new `claude-opus-5`/`claude-haiku-5` ID,
   or a session whose model string doesn't match any pattern at all). Do not guess.

2. **Root-cause fix: add the missing table entries.** Add `claude-opus-5`,
   `claude-sonnet-5`, `claude-haiku-5` to `DefaultPricingTable()`
   (`session/tokens/pricing.go:23-77`), following the exact shape/style of the existing
   gen-4 entries immediately above them. Per stack.md §2, Sonnet 5's public rate
   ($3.00/$15.00 per MTok) is currently identical to the existing `claude-sonnet-4`
   entry — confirm current rates against `https://platform.claude.com/docs/en/pricing.md`
   at implementation time (prices/intro-pricing windows can change) rather than trusting
   this doc's cached numbers verbatim. No `NormalizeModelFamily` regex change is
   required — `variantSuffixPattern`'s `\d+` group already generalizes over the
   generation digit (confirmed by tracing `claude-sonnet-5-N[-date]` → `claude-sonnet-5`
   in pitfalls.md §1).

3. **Lock in the ambiguous collapse behavior with a regression test.** Add
   `{"claude-sonnet-4-5-20250929", "claude-sonnet-4"}` (and a bare-gen-5 case,
   `{"claude-sonnet-5-20260201", "claude-sonnet-5"}`) to
   `TestNormalizeModelFamily_WhenDateSuffixedID_ExpectStripped`
   (`pricing_test.go:13-32`) so the intentional 4.5→4 collapse and the correct gen-5
   passthrough are both explicit, tested contracts — not implicit regex side effects
   nobody would notice breaking.

4. **Close the "silent miss" gap generally**, so the *next* new model generation doesn't
   reproduce this exact bug. Add a narrow, read-only helper —
   `func (pt *PricingTable) UnpricedModels(r *ParseResult) []string` — that reports which
   normalized families in a session's `TurnTimeline` have no pricing entry. This is
   additive (does not change `EstimateCost`/`ModelFamilyCost` signatures, so no ripple
   into the three independent call sites documented in pitfalls.md §3:
   `insights_service.go`, `backlog_service.go`'s `buildCostLookup`, and
   `backlog_service_query.go`'s `GetBacklogItemCost`).

5. **Surface the signal end-to-end.** `insights_service.go`'s existing per-session loop
   (next to `costUSD := s.pricing.EstimateCost(r)` at line 115) calls
   `UnpricedModels` and rolls up a count. Add `sessions_with_unpriced_models` (or
   similar) to `GetInsightsSummaryResponse` in `proto/session/v1/insights.proto`, run
   `make proto-gen`, and add an "Unpriced Usage" card to
   `web-app/src/app/insights/SummaryCards.tsx` mirroring the existing `orphanCount > 0`
   conditional card (lines 42-48) — same visual treatment, new label. Do **not** infer
   "unpriced" from `estimated_cost_usd === 0` client-side — a session can legitimately
   cost $0 (e.g. an all-cache-read turn), so that's an unsafe proxy (architecture.md §4).

6. **Verify the other two cost surfaces, not just Insights.** Per pitfalls.md §3, both
   `backlog_service.go`'s `buildCostLookup()` and `backlog_service_query.go`'s
   `GetBacklogItemCost` RPC share the *same* `*PricingTable` instance (wired once in
   `server/dependencies.go:1070-1076`), so the table fix repairs all three
   automatically — but verification must explicitly check Backlog's item-detail cost
   rollup and per-session cost lookup, not just the Insights dashboard, or the fix could
   ship looking complete while two of three surfaces are unverified.

7. **Explicitly deferred, not part of this fix** (documented so it isn't silently
   dropped, per this repo's `feedback_document_ai_decisions_in_edge_cases` convention):
   - **Wiring the already-implemented `LoadPricingOverride()` into
     `server/dependencies.go`.** stack.md ranks this as the top recommendation for "a
     way to look up prices using our tokens... rather than a static table"; architecture.md
     argues against it for *this* bug specifically — it doesn't fix a missing-model
     problem by itself (someone still has to populate the JSON on day one), and it adds
     real operational surface (a file to maintain, a load-failure mode, a staleness
     question) that a ~15-line map addition already resolves for the reported symptom.
     Recommendation: defer, but call it out explicitly to the user as an option — see
     suggestions below — since it does directly address the literal ask about avoiding a
     "static hardcoded table," and the mechanism is already written and tested
     (`pricing_test.go:108-137`), just unwired.
   - **Wiring `IsStale()`** (dead code — pitfalls.md §2 confirms zero non-test callers,
     and all current `EffectiveDate` values are already ~71 days stale as of this
     writing) into a startup log warning. Cheap, but a separate concern from the gen-5
     lookup-key bug; worth a follow-up ticket.
   - **`MODEL_COLORS` gen-5 swatch keys** in `ModelOverTimeChart.tsx:26-33` — degrades
     gracefully via an existing fallback palette (`colorForModel()`), so it's cosmetic
     only. Cheap one-line addition if done in the same PR, not a functional blocker.

## Task breakdown

See the `tasks` array in the triage JSON output for the itemized, estimated breakdown
(max 12 tasks, covering: transcript verification, pricing table entries, regression
tests, the `UnpricedModels` helper, proto + service wiring, the SummaryCards UI card,
Backlog surface verification, and the two explicitly-deferred stretch items).

## Dependencies and blockers

- **Blocking, must happen first**: confirm the real raw `model` string from a live
  transcript (step 1). Everything else in this plan branches on that answer.
- **Proto change requires `make proto-gen`** (regenerates `session/gen/session/v1/*.go`
  and `web-app/src/gen/session/v1/*_pb.ts`) before the new field is usable in either
  Go or TS — standard repo workflow, no new tooling needed.
- **No feature-flag work needed** — `GetInsightsSummary` is already
  `featureregistry.StatusStable` and ungated (features.md §4).
- **No external dependency** — confirmed no Anthropic pricing API exists (stack.md §2),
  so there is no "wait on a third-party integration" blocker; this is a self-contained
  code change.
- **Feature-registry housekeeping**: if the proto/RPC response shape changes, run
  `make registry-generate` per this repo's feature-registry rule and update the
  matching per-feature JSON file under `docs/registry/features/`.
