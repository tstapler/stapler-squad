# Research: "Sonnet 5 costs blank" — reusable patterns

## 1. Existing "config file overrides hardcoded defaults" pattern

The repo already has this exact pattern, just not wired up for pricing:

- **`config/discovery.go:52-105`** (`LoadDiscoveryConfig`) is the canonical example:
  `DefaultDiscoveryConfig()` (line 54) returns hardcoded safe defaults; `LoadDiscoveryConfig()`
  (line 67) resolves `configDir` via `config.GetConfigDir()`, reads
  `filepath.Join(configDir, "discovery.json")` (line 74), and on `os.IsNotExist` **writes the
  default config to disk** (line 78-83) so the file becomes self-documenting; on any other
  read/parse error it logs and falls back to the in-memory default (never crashes). `SaveDiscoveryConfig`
  (line 108) is the symmetric writer.
- **`config/config.go:107-169`** (`GetConfigDir`/`GetConfigDirForDir`) is the standard path-resolution
  helper already used by `discovery.go` — priority chain: `STAPLER_SQUAD_TEST_DIR` env var → `STAPLER_SQUAD_INSTANCE`
  env var → test-mode auto-detect → preferred-workspace file → `STAPLER_SQUAD_WORKSPACE_MODE` → shared default
  (`~/.stapler-squad`). Any pricing-override file should live under this same directory
  (e.g. `filepath.Join(configDir, "pricing.json")`), not a bespoke env var, so it inherits multi-instance/test
  isolation for free.
- `session/tokens/pricing.go:82-102` (`LoadPricingOverride`) already implements the *load+merge* half of this
  pattern (reads JSON, merges over `DefaultPricingTable()` per-family) but never resolves a path via
  `config.GetConfigDir()` — it just takes a `configPath string` argument — and nothing calls it.
  `server/dependencies.go:1070` calls `tokens.DefaultPricingTable()` directly, bypassing the override path
  entirely.
- Conclusion: wiring `LoadPricingOverride` in is a small, precedented change — resolve
  `configPath := filepath.Join(configDir, "pricing.json")` via `config.GetConfigDir()`, call
  `LoadPricingOverride`, and fall back to `DefaultPricingTable()` on any error, mirroring
  `discovery.go`'s exact fallback shape. This fixes the general "new model family shows up before code ships"
  problem, but is a *separate, larger* fix than adding a `claude-sonnet-5`/`claude-opus-5`/`claude-haiku-5`
  entry to `DefaultPricingTable()` — the two are complementary, not substitutes.

## 2. How Insights renders cost — does it degrade gracefully or show truly blank?

It does **not** show a literal blank string — it shows `$0.0000`, which reads as "this session cost nothing,"
i.e. silently wrong rather than visibly missing:

- `web-app/src/app/insights/insightsFormatters.ts:5-9` (`fmtCost`) always returns a formatted dollar string;
  `usd < 0.01` branches to 4-decimal precision, so cost `0` renders as `"$0.0000"`, never `""`.
- `web-app/src/app/insights/SummaryCards.tsx:20` renders `fmtCost(summary.totalCostUsd)` directly — no
  zero/missing-price special case.
- `web-app/src/app/insights/SessionsTable.tsx:159` renders `fmtCost(s.estimatedCostUsd)` per-row, same
  pattern.
- `web-app/src/app/insights/ModelBreakdownChart.tsx:58-60,98-102` (`fmtDollar`) renders a real bar for the
  model family (it *does* still appear — see §5 below on why) but with height/value `$0.000`.
- So the actual user-visible symptom is "Sonnet 5 sessions show `$0.0000`" (or a chart bar pinned at 0),
  not empty/missing UI — "blank" in the bug title is describing the *value*, not empty markup.

## 3. Existing pattern to surface "unrecognized/stale" data — already built, never wired to the UI

`PricingTable` already has a staleness signal that is plumbed all the way to the API response but is a
**dead signal in the frontend** — this is the reusable mechanism for an "unpriced model" flag too:

- `session/tokens/pricing.go:183-200` (`IsStale`) returns true if any priced entry's `EffectiveDate` is
  >30 days old.
- `server/services/insights_service.go:273` sets `PricingAsOf: timestamppb.New(s.pricing.LoadedAt)` on
  every `GetInsightsSummaryResponse`.
- `proto/session/v1/insights.proto:103` declares `google.protobuf.Timestamp pricing_as_of = 12;` on the
  response message.
- `grep` across `web-app/src` found **zero** consumers of `pricingAsOf` or an `isStale`/staleness field —
  the backend computes and ships this data but no component reads or renders it. There is no existing
  warning-badge/toast call site for pricing data specifically.
- General badge infrastructure does exist and could be reused for a new "unpriced model" indicator, e.g.
  `web-app/src/components/sessions/StatusBadge.tsx:15-37` (`getAttentionReasonInfo`) is a label/icon/variant
  lookup keyed off an enum, rendered via CSS variants (`StatusBadge.css.ts`) — the same shape (icon + label +
  css variant) could carry a "no pricing data" badge on `ModelBreakdownChart` or `SessionsTable` rows.
- Nothing currently returns a per-model "unpriced" boolean from the backend (`ModelBreakdown` proto/struct
  has no such field) — that would be new plumbing, not a reuse of an existing exposed signal, unlike
  `pricing_as_of` which already exists end-to-end and only needs a frontend consumer.

## 4. Feature-flag gating — does it matter for this fix?

No. `server/features/analytics.go:35-43` (`AnalyticsGetInsightsSummary`) marks the `GetInsightsSummary` RPC
`Status: featureregistry.StatusStable` (not experimental/gated) — Insights cost display is fully live for
all users, not hidden behind a flag. `server/features/insights.go:6-13` only covers the separate
`WatchInsights` streaming RPC (`StatusExperimental`), which is unrelated to cost computation/display. No
feature-flag check gates whether `EstimateCost`/`ModelFamilyCost` results reach the UI, so the fix doesn't
need to thread through any flag machinery.

## 5. Existing tests documenting the "add a new model family" pattern

`session/tokens/pricing_test.go` is the reuse template for adding `claude-sonnet-5`/`claude-opus-5`/`claude-haiku-5`:

- `TestNormalizeModelFamily_WhenDateSuffixedID_ExpectStripped` (line 13-32) is a table test over
  `NormalizeModelFamily` input→output pairs (e.g. `"claude-sonnet-4-6-20250514"` → `"claude-sonnet-4"`).
  Adding `"claude-sonnet-5-1-20260601"` → `"claude-sonnet-5"` as a new case is the direct pattern to follow;
  the existing `variantSuffixPattern` regex (`pricing.go:16`, `^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$`)
  already generalizes over the version digit, so `claude-sonnet-5-N` normalizes correctly today — the gap is
  purely the missing `DefaultPricingTable()` entry, not the normalization regex.
- `TestEstimateCost_WhenKnownModel_ExpectExactPrice` (line 34-49) and
  `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` (line 51-63) together document the exact
  before/after: today a `"claude-sonnet-5"`-family session behaves like the `"gpt-99-turbo"` case in the
  second test (cost silently resolves to `0.0`) rather than the first (`$18.0` for known pricing). Adding a
  `claude-sonnet-5` entry to `DefaultPricingTable()` moves Sonnet-5 sessions from the second test's behavior
  to the first's.
- `TestLoadPricingOverride_WhenValidConfigJSON_ExpectOverridesApplied` (line 108-137) is the test template
  for §1's wiring fix if `LoadPricingOverride` gets called from `server/dependencies.go` — it already proves
  override-merge-over-defaults works; only the caller-side wiring (config path resolution +
  `server/dependencies.go:1070`) is untested/unimplemented.
- No existing test asserts on `ModelFamilyCost`'s "silently drop unknown family from the result map"
  behavior (`pricing.go:212-213`) or on `InsightsService`'s resulting `ModelBreakdown.EstimatedCostUsd`
  staying at its zero value while `TotalInputTokens`/`TotalOutputTokens` still populate correctly
  (`server/services/insights_service.go:192-216`) — this is the deeper mechanism worth a regression test:
  the model family *does* still appear in `ModelBreakdown` (populated unconditionally from `TurnTimeline` at
  lines 193-206) but its cost silently stays `0` because the second loop (lines 208-216) only touches
  families present in `pt.Prices`.

## Summary for implementation

- Minimum fix: add `claude-opus-5`, `claude-sonnet-5`, `claude-haiku-5` entries to
  `DefaultPricingTable()` in `session/tokens/pricing.go:23-77`, following the exact shape of the `-4` entries
  immediately above them, plus new table-test cases in `pricing_test.go` per §5.
- `NormalizeModelFamily` needs **no changes** — its existing `variantSuffixPattern` already produces
  `claude-sonnet-5` from any `claude-sonnet-5-N[-date]` input.
- Optional but precedented follow-up: wire `LoadPricingOverride` into `server/dependencies.go:1070` via
  `config.GetConfigDir()`, following `config/discovery.go`'s load/fallback/auto-write shape (§1), so future
  model releases don't require a binary rebuild.
- Optional but precedented follow-up: surface `PricingAsOf`/`IsStale` (or a new per-model "unpriced" signal)
  in the Insights UI using the existing badge pattern (`StatusBadge.tsx`, §3) so a future unpriced model fails
  visibly instead of silently reading as "$0.00 — free."
- No feature flag work needed (§4) — `GetInsightsSummary` is already `StatusStable` and ungated.
