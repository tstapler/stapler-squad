# Architecture Research — insights-cost-pricing-gaps

Agent 3 (Architecture), SDD Phase 2.

## 1. Current shape (confirmed from source)

`session/tokens/types.go:66-81`:
```go
type ModelPricing struct {
	ModelFamily        string
	InputPricePerMTok  float64
	OutputPricePerMTok float64
	CacheWritePerMTok  float64
	CacheReadPerMTok   float64
	EffectiveDate      string
}

type PricingTable struct {
	Prices     map[string]ModelPricing
	LoadedAt   time.Time
	ConfigPath string // empty = hardcoded only
}
```

`session/tokens/pricing.go`:
- `DefaultPricingTable()` (23-77) — 6 hardcoded entries: `claude-opus-4`, `claude-sonnet-4`, `claude-haiku-4`, `claude-opus-3`, `claude-sonnet-3`, `claude-haiku-3`. No `-5` family.
- `LoadPricingOverride(configPath string)` (82-102) — calls `DefaultPricingTable()`, sets `table.ConfigPath`, reads the file with `os.ReadFile`, unmarshals a `map[string]ModelPricing`, and merges each key into `table.Prices`. One-shot, load-once — no watcher, no reload loop. Fully built and unit-tested (`pricing_test.go:108-137`) but **never called** outside tests — confirmed via `grep -rn LoadPricingOverride` returning only `pricing.go` and `pricing_test.go`.
- `NormalizeModelFamily()` (115-136) — regex-based; correctly reduces `claude-sonnet-5-*` → `claude-sonnet-5`. Not the bug.
- `EstimateCost()` (140-181) and `ModelFamilyCost()` (203-237) — both do `pricing, ok := pt.Prices[family]; if !ok { continue }`, silently dropping unpriced usage from the total. No return value or side channel reports which families were skipped.
- `LookupByModel()` (241-245) — thin wrapper, `(ModelPricing, bool)`, already exposes the exact "found/not found" signal `EstimateCost`/`ModelFamilyCost` throw away internally.

`pricing_test.go:51-63` — `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` explicitly pins the silent-zero behavior as "working as intended." This test needs to be extended/replaced per AC-5, not just left in place.

## 2. Full data flow, hop by hop

1. **Raw model ID** — comes from Claude Code JSONL transcript records, parsed into `tokens.ParseResult.TurnTimeline[].Model` / `ParseResult.PrimaryModel` (parsing itself is out of scope here; not re-verified this pass, per requirements.md's existing research).
2. **Normalization** — `tokens.NormalizeModelFamily(turn.Model)`, called at 4 sites: `pricing.go:152` (EstimateCost, per-turn), `pricing.go:161` (EstimateCost, PrimaryModel fallback), `pricing.go:211`/`225` (ModelFamilyCost), and again independently in `insights_service.go:184` and `:194` (model-breakdown loop) — i.e. normalization happens twice for the same turn (once inside `ModelFamilyCost`, once in the caller's own loop over `r.TurnTimeline`) because `ModelFamilyCost` returns cost-by-family but the caller still needs token counts per turn.
3. **Pricing lookup** — `pt.Prices[family]` map lookup inside `EstimateCost`/`ModelFamilyCost`. This is the point where "known" vs "unpriced" is decided and currently discarded.
4. **Backend aggregation** — `server/services/insights_service.go`:
   - `GetInsightsSummary` (41-277): `s.pricing.EstimateCost(r)` at **line 115** for `SessionTokenSummary.EstimatedCostUsd`; `s.pricing.ModelFamilyCost(r)` at **line 179** for the daily bucket's `CostByModel` map; `s.pricing.ModelFamilyCost(r)` again at **line 192** for the `ModelBreakdown` per-family cost. Note: `modelMap` entries are created unconditionally in the token-count loop (193-206) regardless of whether pricing exists, so an unpriced family **does** appear in `resp.Models` — just with `EstimatedCostUsd == 0`, indistinguishable from a genuinely free/zero-usage entry.
   - `ListSessionTokens` (280-410): `s.pricing.EstimateCost(r)` at **line 314** for the same per-session field, no per-model breakdown here.
5. **Proto response** — `proto/session/v1/insights.proto`: `GetInsightsSummaryResponse` (90-104) carries `total_cost_usd`, `models: repeated ModelBreakdown` (each with `estimated_cost_usd`, no pricing-status field), `pricing_as_of` (a single global timestamp from `pt.LoadedAt`, already a "how fresh is my pricing" signal but not a "did I actually price everything" signal). `SessionTokenSummary` (23-40) similarly has only `estimated_cost_usd`, no per-session pricing-completeness flag.
6. **Frontend TS types** — generated from the above proto (`web-app/src/gen/session/v1/insights_pb.ts`, not hand-read but is `make proto-gen` output, mirrors the proto 1:1).
7. **React components** — `web-app/src/app/insights/ModelBreakdownChart.tsx`: builds one bar per `ModelBreakdown` entry unconditionally (`toDataPoints`, lines 48-56), so an unpriced family **is present as a bar**, just height-0 (visually indistinguishable from "$0 because truly free"), confirming requirements.md's characterization. `insightsFormatters.ts:fmtCost` (5-9) has no "N/A"/"unpriced" branch — any float, including a deliberately-flagged unpriced value, would just render as `$0.000`.

## 3. Where to add the "unpriced" signal

### Options considered

**(a) New field(s) on the existing proto response** — e.g. `repeated string unpriced_models = N` on `GetInsightsSummaryResponse`, and/or a `bool has_unpriced_usage` (or `int64 unpriced_tokens`) per `ModelBreakdown`/`SessionTokenSummary` entry.

**(b) Sentinel value convention** — e.g. `EstimatedCostUsd == -1` means "unpriced," or a magic string in an existing field.

**(c) Wrapper type separating "cost" from "cost is estimated/complete"** — e.g. a Go `type CostEstimate struct { USD float64; Complete bool }` threading through `EstimateCost`/`ModelFamilyCost`/the proto/the frontend.

### Recommendation: (a), scoped minimally — extend the existing per-entry messages, don't invent a new wrapper type

Concretely:
- **Go side**: change `EstimateCost` and `ModelFamilyCost` to also report which families were skipped. The minimal-surface way to do this without inventing a `CostEstimate` wrapper (which would ripple through every call site including the two above that don't care about pricing-completeness, e.g. `session/backlog` if it calls these — confirmed `backlogSvc.SetTokenStore(tokenStore, pricing)` in `dependencies.go:1076` is a separate consumer) is to add a **second, additive return value** carrying just the missing-family set, e.g.:
  ```go
  func (pt *PricingTable) EstimateCost(r *ParseResult) (cost float64, unpriced []string)
  func (pt *PricingTable) ModelFamilyCost(r *ParseResult) (costs map[string]float64, unpriced map[string]bool)
  ```
  This is a signature change, not a new type — consistent with `.claude/rules/interface-pollution-checklist.md` smell #5 (unjustified generic) and #6 (wrapper-wraps-wrapper): a `(float64, []string)` tuple return is idiomatic Go (cf. `LookupByModel`'s existing `(ModelPricing, bool)` pattern one function down), whereas a `CostEstimate` struct would be option (c) and is unjustified here — there's exactly one thing "complete-ness" is compositely qualifying (a dollar amount), and Go idiom for "value + a fact about how the value was derived" is a second return value, not a wrapper struct, unless the value needs to be *passed around* later divorced from its origin (it doesn't — every call site consumes both immediately).
- **Proto side**: add `repeated string unpriced_models = 16` to `SessionTokenSummary` (per-session: which families in *this* session had no pricing) and `bool is_unpriced = 6` (or similar) to `ModelBreakdown` (per-family: this whole breakdown row has zero pricing coverage) plus optionally `repeated string unpriced_models = 13` on `GetInsightsSummaryResponse` for a dashboard-level aggregate banner. Use the next free field numbers per message (`SessionTokenSummary` next free is 16; `ModelBreakdown` next free is 7; `GetInsightsSummaryResponse` next free is 13).
- **Reject (b) sentinel values** — magic numbers in a `double` field (e.g. `-1`) are exactly the kind of implicit-contract fragility this repo's rules push against (parallels the "no hardcoded zIndex numbers" CSS rule and the general Go-idiom preference for explicit signals over sentinels — see `go-development` skill's primitive-obsession guidance). A `-1.0` cost is also trivially serialized-and-forgotten by any code that does `sum += cost` without checking, silently reintroducing the exact bug this project fixes.
- **Reject (c) full wrapper type as a Go-level abstraction** — per `.claude/rules/interface-pollution-checklist.md` smell #6 ("struct-wraps-struct-wraps-struct... each layer re-exposes the layer below without adding new exported behavior") and smell #1 (speculative interface/type with no near-term second use): a `CostEstimate{USD, Complete}` struct would need to flow through `EstimateCost` → `insights_service.go` → proto marshaling → frontend, and at every one of those hops it would just be destructured back into two flat fields anyway (a proto message can't embed a Go struct, so the proto boundary forces the split regardless). Building the Go-side wrapper only to immediately unpack it at the one real serialization boundary is ceremony without payoff.

This mirrors the `go-double-checked-locking.md` rule's spirit even though it's not a locking scenario: **return what was actually computed, not a derived/reconstructed signal** — i.e. don't infer "unpriced" downstream from `cost == 0 && tokens > 0`, return the missing-family list directly from the point where the lookup failed (`pt.Prices[family]` miss inside `EstimateCost`/`ModelFamilyCost`), because that's the one place with certain knowledge of *why* the cost is what it is.

### Frontend

- `ModelBreakdownChart.tsx`: use the new `ModelBreakdown.is_unpriced` (or equivalent) field to render those bars in a visually distinct style (hatched/striped fill or a fixed placeholder height + "?" label) instead of a real zero-height bar, and skip them from `sort by cost desc` collapsing them to the bottom silently.
- `SummaryCards.tsx` / a new small banner: surface `unpriced_models` from `GetInsightsSummaryResponse` as a "Pricing unavailable for: claude-sonnet-5" notice — this is the most visible fix for the reported symptom ("Sonnet 5 costs blank").
- `insightsFormatters.ts:fmtCost`: no sentinel-value special-casing needed if the wire format carries an explicit boolean/list (per the (a) vs (b) decision above) — components branch on the boolean field before calling `fmtCost`, so `fmtCost` itself stays a pure formatter with no semantic knowledge of "unpriced."

## 4. G3 — override mechanism

`server/dependencies.go:1070` currently calls only `tokens.DefaultPricingTable()`, never `LoadPricingOverride()`. Searched `config/` package (`config.go`, `state.go`, `defaults.go`, `discovery.go`, etc.) for an existing "watch file, reload on change" pattern: **none exists**. The closest precedent is `config.GetConfigDirForDir()` (`config/config.go:107-168`), which resolves a config directory once via env vars (`STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_TEST_DIR`, `STAPLER_SQUAD_WORKSPACE_MODE`) with a documented priority hierarchy, and `LoadConfigFromPath`/`SaveConfigToPath` (`config.go:749-799`) which load JSON **once at startup**, not via a filesystem watcher — no `fsnotify` import anywhere in `config/` (confirmed via grep). Live-reload is simply not a pattern this codebase uses anywhere for its own config.

**Recommendation: minimal — wire `LoadPricingOverride()` into `dependencies.go` at startup, gated by an optional env var, no watcher.** Concretely:
```go
pricing := tokens.DefaultPricingTable()
if overridePath := os.Getenv("STAPLER_SQUAD_PRICING_OVERRIDE"); overridePath != "" {
    if overrideTable, err := tokens.LoadPricingOverride(overridePath); err != nil {
        log.Warn("failed to load pricing override, using defaults", "path", overridePath, "err", err)
    } else {
        pricing = overrideTable
    }
}
```
placed at `server/dependencies.go:1070` in place of the bare `tokens.DefaultPricingTable()` call. This satisfies AC-4 ("wired into the running server ... without a full code deploy") — dropping/editing a JSON file at the configured path and restarting the service (`systemctl --user restart stapler-squad`, already a routine, non-deploy operation per `.claude/rules/systemd-user-service.md`) is suffient given the Non-Goal explicitly rules out live dynamic pricing-API integration. A file-watcher (fsnotify + hot-swap of `insightsSvc.pricing` under a mutex) would be new infrastructure this codebase has never needed elsewhere, is unjustified complexity for a value that changes on the order of "Anthropic ships a new model family" (weeks/months, not minutes), and fails the same interface-pollution-checklist smell #1 (building for a speculative "need instant reload" requirement nothing in the requirements doc actually asks for — AC-4 explicitly allows "no full code deploy," not "no restart at all"). This is the same "don't over-build" precedent as `.claude/rules/prefer-go-git-over-subshells.md`'s closing note: reach for the minimal tool that does the job, not the general-purpose one that might.

A natural companion location for the override file path: `filepath.Join(configDir, "pricing-override.json")` via `config.GetConfigDir()` would be more discoverable/consistent with existing state-dir conventions than a bare env var with no default path — worth deciding in Phase 3 planning, but either is a small, low-risk choice; recommend defaulting to a well-known path under the config dir *and* allowing the env var to override that path, mirroring the `STAPLER_SQUAD_TEST_DIR` / `STAPLER_SQUAD_INSTANCE` precedent of "sane default, env var escape hatch."

## 5. Proto change required?

**Yes**, if the (a) recommendation above is adopted (it is the recommendation). Touch `proto/session/v1/insights.proto`:
- `ModelBreakdown` message (63-71): add `bool is_unpriced = 7;` (next free field number).
- `SessionTokenSummary` message (23-40): add `repeated string unpriced_models = 16;` (next free field number).
- `GetInsightsSummaryResponse` message (90-104): add `repeated string unpriced_models = 13;` (next free field number) for the dashboard-level aggregate.

Then run `make proto-gen` per CLAUDE.md's documented workflow ("Update protobuf definitions in `proto/session/v1/` if needed → `make proto-gen`"), which regenerates both `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` — confirmed this is the correct, only documented path for proto changes in this repo (no separate manual step needed for the TS side).

## 6. EventStorming / multi-actor domain logic?

**No.** This is a data-pipeline/aggregation feature (JSONL parse → pricing lookup → aggregate → serialize → render), not a multi-actor business-process domain. There's no command/event/policy interplay, no state machine, no cross-actor workflow — a single request handler reads pre-parsed data and computes a response. Skipping the event-command-policy table as instructed.

## Summary of architectural decisions for Phase 3 (plan) to consume

1. **G1**: add `claude-sonnet-5` (and confirm/add `claude-opus-5`/`claude-haiku-5` if in active use — check `session/tokens` fixtures or recent transcript samples in Phase 3) entries to `DefaultPricingTable()` in `session/tokens/pricing.go:23-77`.
2. **G2**: change `EstimateCost`/`ModelFamilyCost` signatures to return `(value, unpriced-families)` as a second return value — not a new wrapper type, not a sentinel. Add `is_unpriced` (ModelBreakdown), `unpriced_models` (SessionTokenSummary, GetInsightsSummaryResponse) fields to `proto/session/v1/insights.proto`; run `make proto-gen`. Update `insights_service.go`'s 4 call sites (lines 115, 179, 192, 314) to thread the new signal through. Update `ModelBreakdownChart.tsx` and add a summary-level unpriced banner (likely in `SummaryCards.tsx` or `InsightsDashboard.tsx`) in the frontend.
3. **G3**: wire `tokens.LoadPricingOverride()` into `server/dependencies.go:1070` behind an env var (e.g. `STAPLER_SQUAD_PRICING_OVERRIDE`) with a sane default path under `config.GetConfigDir()`, load-once-at-startup — no file-watcher, no new dependency, consistent with this codebase's existing config-loading idiom.
4. **G4**: the natural guardrail is AC-5's test (assert a synthetic unknown model family is flagged, not silently zeroed) plus `PricingTable.IsStale()` (already exists, `pricing.go:185-200`, currently unused by any caller — wiring `IsStale()` into a startup log warning or the `pricing_as_of`-adjacent UI badge would be a cheap, already-built second guardrail worth flagging to Phase 3).
