# Stack Research: insights-cost-pricing-gaps

Agent 1 (Stack) — SDD Phase 2

## 1. `session/tokens/pricing.go` — exact shapes and signatures

### Types (`session/tokens/types.go:66-81`)

```go
// ModelPricing holds per-model token prices in USD per million tokens.
type ModelPricing struct {
	ModelFamily        string  // normalized key, e.g. "claude-sonnet-4"
	InputPricePerMTok  float64
	OutputPricePerMTok float64
	CacheWritePerMTok  float64
	CacheReadPerMTok   float64
	EffectiveDate      string  // ISO date "2006-01-02"
}

// PricingTable maps normalized model family names to pricing.
// Hardcoded defaults; overridable via config JSON.
type PricingTable struct {
	Prices     map[string]ModelPricing
	LoadedAt   time.Time
	ConfigPath string // empty = hardcoded only
}
```

### Functions (`session/tokens/pricing.go`)

- `func DefaultPricingTable() *PricingTable` (lines 23-77) — hardcoded map, `EffectiveDate: "2026-05-15"` on every entry. Current keys: `claude-opus-4`, `claude-sonnet-4`, `claude-haiku-4`, `claude-opus-3`, `claude-sonnet-3`, `claude-haiku-3`. **No `-5` family entries at all.**
- `func LoadPricingOverride(configPath string) (*PricingTable, error)` (lines 82-102) — starts from `DefaultPricingTable()`, sets `table.ConfigPath = configPath`, reads the file, `json.Unmarshal`s into `map[string]ModelPricing`, merges each entry into `table.Prices` (forcing `pricing.ModelFamily = family` for consistency). Returns the *default* error from `os.ReadFile`/`json.Unmarshal` unwrapped (not wrapped with `fmt.Errorf`) — caller must handle `os.IsNotExist` itself if graceful fallback is wanted.
- `func NormalizeModelFamily(modelID string) string` (lines 115-136) — strips `-\d{8}$` date suffix, handles legacy `claude-3-opus-...` → `claude-opus-3`, and a variant-suffix regex `^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$` (e.g. `claude-sonnet-4-6` → `claude-sonnet-4`). **Confirmed via manual trace: `claude-sonnet-5-20250929` → date-suffix stripped → `claude-sonnet-5` directly; a hypothetical `claude-sonnet-5-1-20250929` would also normalize correctly via the variant-suffix branch.** Normalization is not the bug — it already handles a `-5` family correctly. Only the table itself is missing the `claude-sonnet-5` key.
- `func (pt *PricingTable) EstimateCost(r *ParseResult) float64` (lines 140-181) — builds per-family token maps from `r.TurnTimeline` (or falls back to `r.PrimaryModel`/`r.TotalInput`/etc. when timeline is empty), then `pricing, ok := pt.Prices[family]; if !ok { continue }` — **silently skips, no signal returned**. Returns a single `float64` — there is no secondary return value or field today for "some usage was unpriced." Any G2 fix here requires either (a) a new return type/second return value, or (b) a companion method (e.g. `pt.UnpricedFamilies(r) []string`) called alongside it.
- `func (pt *PricingTable) ModelFamilyCost(r *ParseResult) map[string]float64` (lines 203-237) — same `if !ok { continue }` skip pattern, per-family cost breakdown. Also has zero signal for "family present but unpriced" — an unpriced family simply never appears as a key in the returned map, which is indistinguishable from "family had zero usage."
- `func (pt *PricingTable) IsStale() bool` (lines 183-200) — already exists; checks if any priced entry's `EffectiveDate` is >30 days old. Not currently surfaced to callers/UI (see `PricingAsOf` note below) beyond being computed — confirmed no call site references `IsStale()` outside `pricing_test.go`.
- `func (pt *PricingTable) LookupByModel(modelID string) (ModelPricing, bool)` (lines 241-245) — normalizes then does `pt.Prices[family]`; the `ok bool` here is the one place in this file that already exposes "found vs not found" as a return value. **This is the natural shape to mirror for G2** — e.g. add a method returning per-family `(cost float64, priced bool)` or a `map[string]bool` of unpriced families alongside the existing cost maps, rather than reworking `EstimateCost`'s signature and breaking every call site.

### Existing test pattern (`session/tokens/pricing_test.go:51-63`)

```go
func TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero(t *testing.T) {
	pt := DefaultPricingTable()
	result := &ParseResult{
		PrimaryModel: "gpt-99-turbo",
		TurnTimeline: []TurnStats{{Model: "gpt-99-turbo", Input: 500_000, Output: 500_000}},
	}
	cost := pt.EstimateCost(result)
	assert.Equal(t, 0.0, cost)
}
```
This test currently *asserts* the silent-zero behavior as correct — AC-5 requires extending or replacing it once G2's detection mechanism exists.

## 2. `server/dependencies.go` wiring (lines 1065-1077)

```go
// Initialize TokenStore and InsightsService for token usage analytics.
var insightsSvc *services.InsightsService
if homeDir, homeDirErr := os.UserHomeDir(); homeDirErr == nil {
	historyDir := filepath.Join(homeDir, ".claude", "projects")
	tokenStore := tokens.NewTokenStore(historyDir)
	pricing := tokens.DefaultPricingTable()          // ← G3: never calls LoadPricingOverride
	associator := tokens.NewAssociator(storage)
	historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)
	tokenStore.Start(context.Background())
	insightsSvc = services.NewInsightsService(tokenStore, pricing, associator)
	sessionService.SetTokenStoreReader(tokenStore)
	backlogSvc.SetTokenStore(tokenStore, pricing)
	log.Info("InsightsService initialized", "historyDir", historyDir)
	...
}
```

Confirmed via `grep -rn "LoadPricingOverride"` (whole repo, Go files only): the function is referenced **only** in `session/tokens/pricing.go` (definition) and `session/tokens/pricing_test.go` (test) — zero production call sites. G3's job is exactly to add a call here.

### Reusable idioms already in this codebase for "config file with env-var/instance-aware path + graceful fallback"

- `config.GetConfigDir() (string, error)` / `config.GetConfigDirForDir(dir string) (string, error)` (`config/config.go:117-155`) — the canonical way to resolve a per-instance config directory. Priority order: `STAPLER_SQUAD_TEST_DIR` env var → `STAPLER_SQUAD_INSTANCE` env var → test-mode auto-detect → workspace preference file → `STAPLER_SQUAD_WORKSPACE_MODE=true` per-directory → global default (`~/.stapler-squad`). Any new override-file mechanism should resolve its path via this function so it inherits multi-instance/test isolation for free, rather than hardcoding `~/.stapler-squad/...`.
- `config.LoadConfig()` / `config.LoadConfigFromPath(path string) (*Config, error)` (`config/config.go:748-770, 813+`) — the pattern to mirror for graceful-fallback JSON loading:
  ```go
  func LoadConfig() *Config {
  	configDir, err := GetConfigDir()
  	if err != nil { log.Error(...); return DefaultConfig() }
  	configPath := filepath.Join(configDir, ConfigFileName)
  	cfg, err := LoadConfigFromPath(configPath)
  	if err != nil {
  		if os.IsNotExist(err) {
  			defaultCfg := DefaultConfig()
  			if saveErr := saveConfig(defaultCfg); saveErr != nil { log.Warn(...) }
  			return defaultCfg
  		}
  		log.Warn("failed to load config file", "err", err)
  		return DefaultConfig()
  	}
  	return cfg
  }
  ```
  `LoadConfigFromPath` uses plain `encoding/json` (`json.Unmarshal`) — same library `LoadPricingOverride` already uses. **No new JSON library needed; `encoding/json` is the established choice throughout `config/` and `session/tokens/`.**
- Existing env-var override precedent: `config/config.go:458` reads `ANTHROPIC_API_KEY` directly via `os.Getenv` with a comment `// Apply environment variable overrides (never log the value)`. A `STAPLER_SQUAD_PRICING_OVERRIDE_PATH`-style env var (or simply defaulting to `filepath.Join(configDir, "pricing_overrides.json")` and only loading if the file exists) would match established convention — no fsnotify/live-reload mechanism exists anywhere in `config/` today, so a load-once-at-startup override (matching `LoadPricingOverride`'s existing shape) is consistent with the rest of the codebase; a hot-reload watcher would be a new pattern, not a reused one.
- **Recommended G3 approach**: in `server/dependencies.go`, replace `pricing := tokens.DefaultPricingTable()` with something like:
  ```go
  pricing := tokens.DefaultPricingTable()
  if configDir, cfgErr := config.GetConfigDirForDir(...); cfgErr == nil {
  	overridePath := filepath.Join(configDir, "pricing_overrides.json")
  	if loaded, loadErr := tokens.LoadPricingOverride(overridePath); loadErr == nil {
  		pricing = loaded
  	} else if !os.IsNotExist(loadErr) {
  		log.Warn("failed to load pricing override", "path", overridePath, "err", loadErr)
  	}
  }
  ```
  This reuses the exact `GetConfigDir`/graceful-fallback idiom already established, requires no new dependency, and gives ops a documented, low-friction file to drop in without a redeploy (satisfies AC-4 without inventing a new mechanism).

## 3. `server/services/insights_service.go` call sites (confirmed exact lines in current file)

- Line 115: `costUSD := s.pricing.EstimateCost(r)` — per-session cost, feeds `SessionTokenSummary.EstimatedCostUsd` (line 136) and running totals (`totalCostUSD`, etc.).
- Line 179: `modelFamilyCostsForDay := s.pricing.ModelFamilyCost(r)` — inside the daily-bucket block; feeds `DailyTokenBucket.CostByModel[family]` (map, proto field 7).
- Line 192: `modelFamilyCosts := s.pricing.ModelFamilyCost(r)` — second call (recomputed, not reused from line 179) feeding `ModelBreakdown.EstimatedCostUsd` per family (proto `ModelBreakdown`, field 5) and `mb.SessionCount++`.
- Line 273: `PricingAsOf: timestamppb.New(s.pricing.LoadedAt)` — already threads `PricingTable.LoadedAt` into the RPC response (`GetInsightsSummaryResponse.pricing_as_of`, proto field 12). **This existing field is a ready-made anchor**: the frontend could already show "pricing as of `<date>`" — worth checking in Phase 3 whether it's rendered anywhere in the UI today (not verified in this pass) — and `PricingTable.IsStale()` (unused elsewhere) could feed a `pricing_is_stale` bool alongside it for near-zero additional plumbing.

### Proto message shapes relevant to G2 (`proto/session/v1/insights.proto`)

```protobuf
message SessionTokenSummary {
  ...
  double estimated_cost_usd = 9;
  ...
}
message DailyTokenBucket {
  ...
  double estimated_cost_usd  = 5;
  map<string, double> cost_by_model   = 7;
  map<string, int64>  tokens_by_model = 8;
}
message ModelBreakdown {
  string model_family        = 1;
  int64  total_input_tokens  = 2;
  int64  total_output_tokens = 3;
  int64  cache_read_tokens   = 4;
  double estimated_cost_usd  = 5;
  int32  session_count       = 6;
}
message GetInsightsSummaryResponse {
  repeated SessionTokenSummary sessions  = 1;
  double   total_cost_usd                = 2;
  ...
  repeated DailyTokenBucket daily        = 7;
  repeated ModelBreakdown models         = 8;
  ...
  bool     is_loading                    = 11;
  google.protobuf.Timestamp pricing_as_of = 12;
}
```

None of these messages currently has any "unpriced"/"pricing unavailable" field. G2 will need at least one new proto field — cheapest options ranked by plumbing cost:
1. Add `bool has_unpriced_usage = N;` (or `repeated string unpriced_model_families = N;`) to `ModelBreakdown` — a family either is or isn't priced, so this is a single new field per breakdown row, directly answers "which model's bar is fake-zero."
2. Add `repeated string unpriced_model_families = N;` to `GetInsightsSummaryResponse` (aggregate list) for a page-level banner, cheaper than per-row plumbing but coarser.
3. Both (per-row flag for the chart, aggregate list for a banner) — likely what Phase 3 planning will land on given AC-3's "visibly indicates... rather than an invisible chart bar" wording, which implies per-row detection in `ModelBreakdownChart.tsx`.

Any new field requires `make proto-gen` per this repo's `session-creation-registry.md`-style convention (same regen command, not a new touchpoint list — insights isn't a session-creation-mode feature, just standard proto workflow) and touches both `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.

## 4. Frontend: `insightsFormatters.ts` and cost data flow

`web-app/src/app/insights/insightsFormatters.ts` (full file, 38 lines):

```ts
// +feature: insights-dashboard
export function fmtCost(usd: number): string {
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(2)}`;
}
export function fmtTokens(n: bigint): string { ... }
export function fmtPct(rate: number): string { ... }
export function fmtDate(ts: { seconds: bigint } | undefined): string { ... }
export function shortId(id: string): string { ... }
```

`fmtCost(0)` currently renders `"$0.0000"` — visually identical whether the true cost is zero usage or unpriced usage. This is the exact spot AC-3 needs a companion (e.g. a `fmtCostOrUnpriced(usd, isUnpriced)` helper, or callers branching on a new `hasUnpricedUsage` boolean before calling `fmtCost` at all) — confirmed no existing "unavailable"/"unknown" formatting branch exists in this file today.

`SummaryCards.tsx` imports `fmtCost, fmtTokens, fmtPct` from this file and calls `fmtCost(summary.totalCostUsd)` (line 20) directly on the RPC response field — confirming the response's `total_cost_usd` (camelCased to `totalCostUsd` in the generated TS client) flows straight into formatting with no intermediate "is this real" check anywhere in the pipeline.

The proto-generated TS types live under `web-app/src/gen/session/v1/*_pb.ts` (not read in full this pass — large generated files) and are produced by `make proto-gen` per `proto/session/v1/insights.proto` above; any new proto field appears there automatically after regen with the same camelCase convention (`has_unpriced_usage` → `hasUnpricedUsage`, `unpriced_model_families` → `unpricedModelFamilies`).

## 5. Current/accurate Anthropic pricing for missing model families — GAP, do not fabricate

**I could not verify authoritative, current Anthropic pricing for `claude-sonnet-5` (or confirm/deny `claude-opus-5`'s existence) via file/code research alone** — this requires a live web search against Anthropic's pricing page, which is out of scope for this stack-research pass (this agent was scoped to codebase file reads, not external verification, and no web search was run in this pass). Flagging as an explicit **open gap for Phase 3 (plan)**:

- Do **not** carry forward the existing table's `claude-sonnet-4` numbers ($3/$15/$3.75/$0.30 per MTok) as a guess for `claude-sonnet-5` — historically Anthropic has both raised and held pricing flat across generations; guessing risks silently under- or over-billing in the Insights UI, which is the exact class of bug this project exists to fix.
- Recommended Phase 3 action: either (a) task a research agent with live web search against Anthropic's official pricing page/docs before writing the new `DefaultPricingTable()` entries, or (b) ship the code changes (G2/G3/G4) with a clearly-marked placeholder entry (e.g. `EffectiveDate: "TODO-VERIFY"` or a sentinel that trips `IsStale()`/a lint check) so AC-1 code lands without AC-1's numbers being fabricated with false confidence — the requirements doc explicitly permits this ("a placeholder/TODO with a source-check requirement is acceptable, do not fabricate numbers with false confidence").
- Whatever numbers are used, follow the existing table's exact shape: `InputPricePerMTok`, `OutputPricePerMTok`, `CacheWritePerMTok` (historically ≈1.25× input), `CacheReadPerMTok` (historically ≈0.1× input) — see the ratio pattern already present across every existing entry in `DefaultPricingTable()`, useful for sanity-checking whatever number is eventually sourced.

## 6. JSON/config library survey — no new dependency needed

- `session/tokens/pricing.go` already uses stdlib `encoding/json` for `LoadPricingOverride`.
- `config/config.go`'s `LoadConfigFromPath` also uses stdlib `encoding/json` (`json.Unmarshal`).
- No third-party config/JSON library (viper, koanf, etc.) appears anywhere in `config/` or `session/tokens/` — confirmed via the greps above; only `encoding/json` and `os.Getenv`. G3's wiring should stay on stdlib `encoding/json`, matching both existing call sites, rather than introducing anything new.

## Summary of concrete file/line touchpoints for Phase 3 planning

| Goal | Primary files |
|---|---|
| G1 (pricing table) | `session/tokens/pricing.go` `DefaultPricingTable()` lines 23-77 |
| G2 (unpriced signal) | `session/tokens/pricing.go` `EstimateCost`/`ModelFamilyCost` (140-237); `proto/session/v1/insights.proto` `ModelBreakdown`/`GetInsightsSummaryResponse`; `server/services/insights_service.go` lines 115,179,192; `web-app/src/app/insights/insightsFormatters.ts` `fmtCost`; `ModelBreakdownChart.tsx`, `SummaryCards.tsx` |
| G3 (override wiring) | `server/dependencies.go` lines 1069-1070; reuse `config.GetConfigDir()`/`GetConfigDirForDir()` (`config/config.go:117-155`) + existing `tokens.LoadPricingOverride` (already written, just uncalled) |
| G4 (regression guardrail) | `session/tokens/pricing_test.go` (extend `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero`); possibly a lint/test asserting every family seen in recent JSONL history has a table entry |
