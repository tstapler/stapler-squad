# Validation — "Sonnet 5 costs blank"

## Test plan mapped to acceptance criteria

The backlog item's acceptance criteria (paraphrased from the description): (a) fix all
blank/unaccounted-for costs on the Insights page, (b) ensure prices can be looked up
using token counts for the models actually in use.

| # | Acceptance criterion | Test | Type |
|---|---|---|---|
| 1 | Sonnet-5-generation sessions show a real, non-zero USD cost on Insights, not `$0.0000` | `TestEstimateCost_WhenClaudeSonnet5_ExpectNonZeroPrice` (new, `pricing_test.go`) — table-drive over `claude-sonnet-5-<date>` with known token counts, assert `EstimateCost` returns the documented Sonnet-5 rate × tokens, not `0.0` | Go unit |
| 2 | Opus-5 / Haiku-5 sessions are priced too (not just Sonnet, since the bug title names one model but the root cause is table-wide) | Extend the same test (or a sibling) for `claude-opus-5-<date>` and `claude-haiku-4-5`/`claude-haiku-5-<date>` | Go unit |
| 3 | The existing "Sonnet 4.5 collapses into Sonnet 4 pricing" behavior is intentional and doesn't regress | `TestNormalizeModelFamily_WhenDateSuffixedID_ExpectStripped` gains a case: `{"claude-sonnet-4-5-20250929", "claude-sonnet-4"}` | Go unit |
| 4 | A genuinely new, unmapped model still normalizes correctly (doesn't collapse into the wrong bucket) even though its price is still 0 until an entry is added | New case: `{"claude-sonnet-5-20260201", "claude-sonnet-5"}` (bare gen-5, no point-release suffix) — asserts the variant regex's two-digit-group requirement doesn't accidentally swallow it | Go unit |
| 5 | Insights dashboard totals (`totalCostUsd`, per-day `CostByModel`, `ModelBreakdown.EstimatedCostUsd`) reflect the corrected price for a session mixing gen-4 and gen-5 turns | `TestGetInsightsSummary_WhenMixedModelGenerations_ExpectAllPriced` (new or extended, `insights_service_test.go` if it exists, else add) — synthetic `ParseResult` with both `claude-sonnet-4` and `claude-sonnet-5` turns, assert both contribute non-zero cost to the response | Go unit/integration |
| 6 | Backlog's independent cost surfaces are fixed too, not just Insights (pitfalls.md §3: shared `*PricingTable` instance) | Extend/add a test on `backlog_service.go`'s `buildCostLookup()` and `backlog_service_query.go`'s `GetBacklogItemCost` with a gen-5 session fixture, assert non-zero cost | Go unit |
| 7 | An operator can tell, going forward, when a session's cost is "unknown" vs. "genuinely free" | `TestUnpricedModels_WhenModelNotInTable_ExpectReported` (new, `pricing_test.go`) for the new `UnpricedModels` helper; plus a service-level test that `GetInsightsSummaryResponse`'s new field is populated when a session hits an unmapped family | Go unit |
| 8 | The Insights UI shows an "Unpriced Usage" card only when relevant, matching the existing orphan-card pattern | Manual verification in the running app (`make install-service` + visit Insights) with a synthetic unmapped-model session; optionally a React Testing Library / Jest snapshot on `SummaryCards.tsx` for the conditional-render branch, mirroring however `orphanCount` is (or isn't) already tested | Frontend unit / manual |
| 9 | Regenerated proto compiles cleanly on both sides | `make proto-gen && make build` (Go) and `cd web-app && npx jest --no-coverage` (TS) | Build/CI |
| 10 | Full CI gate passes | `make ci` | CI |

## Edge cases and error scenarios

- **Ambiguous model string** (architecture.md §3, pitfalls.md §1): if the real
  production `model` field turns out to be neither `claude-sonnet-4-5-*` nor a bare
  `claude-sonnet-5-*` but some other shape entirely, the transcript-verification step
  (plan.md step 1) must catch this *before* writing the fix — a test suite built around
  a wrong assumption would pass while the real bug ships unfixed.
- **Regex double-collapse regression**: verify that adding a `"claude-sonnet-4-5"` key
  directly to `pt.Prices` would be a bug in itself, since `variantSuffixPattern` collapses
  that shape to `claude-sonnet-4` before any map lookup — the key would be silently
  unreachable. Add a test asserting `NormalizeModelFamily("claude-sonnet-4-5-<date>")` is
  never returned as a literal `pt.Prices` key search target (i.e. guard against a future
  contributor "fixing" this by adding the wrong map key).
- **Legitimate $0 cost session** (architecture.md §4): a session that is 100%
  cache-read (or otherwise legitimately near-zero cost) must NOT trigger the new
  "Unpriced Usage" card — the new signal must come from `UnpricedModels` (table-miss
  detection), never from `estimated_cost_usd === 0` inference. Add a test with a
  known-priced, all-cache-read `ParseResult` asserting the unpriced count stays 0.
- **`EffectiveDate` staleness**: all six current hardcoded entries are dated
  `2026-05-15`, already >30 days stale per `IsStale()`'s own threshold as of this
  writing (pitfalls.md §2). Not a functional bug (feature is unwired), but the new gen-5
  entries should get a current, accurate `EffectiveDate` rather than perpetuating stale
  dates — and if `IsStale()` gets wired up as a stretch item, verify it fires correctly
  against the *new* dates too, not just the pre-existing gen-3/4 ones.
- **Mixed-generation session in one transcript**: a session that switches models
  mid-conversation (e.g. starts on Sonnet 4, gets escalated to Sonnet 5) must have
  per-turn costs correctly attributed to each family in `TurnTimeline`, not lumped
  under a single model — `EstimateCost`/`ModelFamilyCost` already iterate
  `r.TurnTimeline` per-turn (pricing.go:151,210), so this should already work once the
  table entry exists; add a regression test to confirm since it's the exact repro shape
  most likely to have surfaced the original bug report.
- **Config override still absent**: since `LoadPricingOverride`/wiring is explicitly
  deferred (plan.md step 7), verify no test or code path assumes a config file exists at
  runtime — the fix must work correctly with `DefaultPricingTable()` alone, no override
  file present, matching current production behavior.
- **proto/registry drift**: if the new response field is added, run
  `make registry-diff` to confirm no unexpected coverage-gap growth, per this repo's
  feature-registry rule, before considering the change complete.
