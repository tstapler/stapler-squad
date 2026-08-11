# Implementation Plan: insights-cost-pricing-gaps

**Feature**: Close the missing-model-family pricing gap on the Insights dashboard and make unpriced-model cost visibly distinct from genuinely-zero cost, end to end (backend pricing table → proto → UI).
**Date**: 2026-07-27
**Status**: Ready for implementation
**ADRs**: `ADR-001-unpriced-signal-return-shape.md`, `ADR-002-capacity-monitor-pricing-scope-deferred.md`, `ADR-003-backlog-cost-lookup-scope-deferred.md`

---

## Step 0.5 — Creative pass (recorded in Pattern Decisions below)

Three approaches were considered for the G2 "unpriced signal" mechanism (the one architecturally significant decision in this plan): an additive second return value, a sentinel value, and a full wrapper type. Full analysis is in `ADR-001-unpriced-signal-return-shape.md`, summarized in the Pattern Decisions table. This mirrors — and does not redo — the reasoning already worked out in `research/architecture.md` §3.

---

## Domain Glossary

*(Ubiquitous language — exact names must be used consistently in code, tests, comments. All Go/proto names here already exist in the codebase or are new fields added by this plan; nothing here invents a new Go type unless explicitly marked "new.")*

| Term | Definition | Notes |
|---|---|---|
| `ModelFamily` | A normalized model identifier string (e.g. `"claude-sonnet-5"`), produced by `tokens.NormalizeModelFamily(modelID string) string` (`session/tokens/pricing.go:115-136`); the key type of `PricingTable.Prices`. | Remains a plain `string`, not a newtype — see Pattern Decisions row 7 for why introducing one is rejected here. |
| `ModelPricing` | Existing struct (`session/tokens/types.go:66-73`) holding `InputPricePerMTok`, `OutputPricePerMTok`, `CacheWritePerMTok`, `CacheReadPerMTok` (all USD per 1M tokens), and `EffectiveDate` (ISO `2006-01-02`) for one `ModelFamily`. | Unchanged shape; new entries added to `DefaultPricingTable()`. |
| `PricingTable` | Existing struct (`session/tokens/types.go:76-81`): `Prices map[string]ModelPricing`, `LoadedAt time.Time`, `ConfigPath string`. | Unchanged shape. Not made concurrency-safe in this plan — see Pattern Decisions row 2 (load-once, no hot-reload). |
| `PricingUnavailable` | **New** proto `bool` field on `ModelBreakdown` (`proto/session/v1/insights.proto`) — true when this family had token usage but no `PricingTable.Prices` entry. | Field number 7. |
| `UnpricedModels` | **New** proto `repeated string` field on `SessionTokenSummary` (field 17), `DailyTokenBucket` (field 9), and `GetInsightsSummaryResponse` (field 13) — the set of `ModelFamily` values with usage but no pricing entry, scoped to that message's granularity (one session / one day / the whole response). | camelCases to `unpricedModels` in generated TS. Note: `requirements.md` (Problem Statement, AC-2/AC-3) uses "unaccounted" interchangeably with "unpriced" when describing this same gap — the two words name one concept throughout this project's artifacts, not two separate buckets; "unpriced"/`UnpricedModels` is the term that was carried into code/proto/UI, and "unaccounted" only ever appears as the informal, user-facing framing of the identical condition. |
| `PricingOverride` | A JSON file (default path `<configDir>/pricing_overrides.json`, `configDir` from `config.GetConfigDir()`) loaded via the existing `tokens.LoadPricingOverride(configPath string) (*PricingTable, error)` (`session/tokens/pricing.go:82-102`) at server startup, merging entries over `DefaultPricingTable()`. | Load-once at startup; no watcher. |
| `IsStale` | Existing `PricingTable` method (`session/tokens/pricing.go:185-200`) — `true` when any priced entry's `EffectiveDate` is more than 30 days old. | Currently dead code (no caller); wired into a startup log in this plan. |
| `<synthetic>` sentinel | The literal string Claude Code's own JSONL transcript writer sets as `message.model` for internal, always-zero-usage assistant turns (confirmed empirically: 198/198 real `<synthetic>` turns sampled from this machine's `~/.claude/projects/*.jsonl` have `input_tokens=output_tokens=cache_creation_input_tokens=cache_read_input_tokens=0`). | Filtered out of `TurnTimeline`/`modelCounts` in `session/tokens/parser.go` so it never reaches pricing lookup and is never mistaken for a real unpriced model. |
| `EstimateCost` | Existing `PricingTable` method (`session/tokens/pricing.go:140-181`), changed by this plan to `func (pt *PricingTable) EstimateCost(r *ParseResult) (cost float64, unpriced []string)`. | See ADR-001. |
| `ModelFamilyCost` | Existing `PricingTable` method (`session/tokens/pricing.go:203-237`), changed by this plan to `func (pt *PricingTable) ModelFamilyCost(r *ParseResult) (costs map[string]float64, unpriced map[string]bool)`. | See ADR-001. |
| Orphan / `Associator` | Pre-existing, **unrelated** concept (`session/tokens/association.go`) — a token-usage record that couldn't be matched to a stapler-squad `Session`. Investigated and confirmed a separate root cause (matching heuristics, not pricing). | Explicitly out of scope for this project — see Unresolved Questions. |
| `CapacityMonitor.estimateCost` | Pre-existing, **independent**, hardcoded, substring-matched pricing function (`server/services/capacity_monitor.go:351-371`) feeding `config.CostBudgetUSD` / `cost_budget_exceeded`. | Explicitly out of scope for this project — see ADR-002 and Unresolved Questions. |
| `BacklogService.buildCostLookup` | Pre-existing consumer of `EstimateCost` (`server/services/backlog_service.go:399-429`, plus `server/services/backlog_service_query.go:455-477`) feeding `SessionCostEntry`/`resp.TotalCostUsd` in the Backlog UI. | Requires only the minimal Task 1.3.1g compile fix (`cost, _ := ...`) in this project; surfacing the unpriced signal there is explicitly out of scope — see ADR-003 and Unresolved Questions. |
| `knownActiveClaudeFamilies` | **New** test-only `[]string` var in `session/tokens/pricing_test.go`, maintained independently of `DefaultPricingTable()`'s keys, used by the G4 completeness guardrail test. | Deliberately a second, hand-maintained source of truth — see Pattern Decisions row 3. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `EstimateCost`/`ModelFamilyCost` unpriced signal | Additive second return value (`(cost, unpriced)`) | Go idiom (mirrors `LookupByModel`'s `(ModelPricing, bool)`); ADR-001 | (1) Sentinel value (`-1.0` cost); (2) `CostEstimate{USD, Complete}` wrapper struct | (1) A sentinel float is silently summed by any unchecked `total += cost`, reintroducing this exact bug class. (2) A proto message can't embed a Go struct, so the wrapper is destructured back into two flat fields at the one real serialization boundary anyway — ceremony without payoff; violates `interface-pollution-checklist.md` smell #6. |
| `server/dependencies.go` pricing-override loading | Load-once at process startup, no file watcher | Existing codebase idiom — `config.LoadConfig()`/`LoadConfigFromPath` (`config/config.go:748-813`) load JSON once at startup everywhere else; no `fsnotify` anywhere in `config/` | `atomic.Pointer[PricingTable]` copy-on-write hot-reload | `PricingTable.Prices` is a plain unsynchronized `map[string]ModelPricing`; in-place mutation of a table already handed to concurrent request goroutines is a guaranteed Go runtime crash (`fatal error: concurrent map read and map write`), not just a race (`research/pitfalls.md` §4). AC-4 only requires "without a full code deploy" — a config-file edit + `systemctl --user restart stapler-squad` already satisfies that, per `.claude/rules/systemd-user-service.md`. Building hot-reload machinery this codebase has never needed elsewhere is unjustified complexity (interface-pollution-checklist smell #1: speculative capability nothing in requirements.md asks for). |
| G4 regression guardrail | (a) Unit test asserting unknown-model detection (AC-5) + (b) a second completeness test iterating an independently-maintained `knownActiveClaudeFamilies` list + (c) `IsStale()` wired into a startup log | Mirrors `docs/registry/`'s generate-and-diff shape at unit-test scale (`research/build-vs-buy.md` §4) | (i) A test hardcoding today's known model list only; (ii) a CI lint gate failing the whole build on any unrecognized model string | (i) Only proves *today's* gap is closed — the exact "individually-patched instance" failure mode `docs/tasks/backlog-feature-improvement.md` has recorded 10+ times for this shape of bug (silent-continue-on-unknown-key) in this same codebase. (ii) A build-wide lint gate would break unrelated PRs the moment any new model ID shows up in an unrelated test fixture — too blunt; scope the guardrail to the pricing/insights test suite plus a runtime signal instead. |
| `<synthetic>` model handling | Filter at the parser boundary (`session/tokens/parser.go`, `processAssistantEntry`) — never added to `TurnTimeline`/`modelCounts` | Parse-don't-validate / clean separation of concerns; empirically verified via live data (198/198 sampled `<synthetic>` turns on this machine have zero usage) | Add an explicit `"<synthetic>"` entry to `DefaultPricingTable()` with all rates `0.0` | A zero-priced table entry conflates "known free" with "priced," and would need to live forever in a table meant to track real Anthropic pricing. Filtering at the parser boundary is a one-line, well-scoped change (empirically confirmed safe — `<synthetic>` never carries nonzero usage in this codebase's own history) and keeps `PricingTable` free of a non-model bookkeeping entry. |
| AC-6 registry entry for pricing coverage | Fold test-coverage updates into the **existing** RPC-scoped backend entries (`GetInsightsSummary.json`, `ListSessionTokens.json`); add a **new** frontend entry for the UI indicator | Verified against `docs/registry/schema.json` and `tools/scanner/backend/proto_scanner.go`'s proto+marker scan | A new standalone `docs/registry/features/backend/pricing-lookup.json` entry (research's originally-recommended option) | `docs/registry/schema.json`'s backend `Feature` schema *requires* `service`/`method`/`protoFile` — every existing backend entry (confirmed by reading `GetInsightsSummary.json`, `defaults/get.json`, etc.) is 1:1 with a real proto RPC method, generated additively by the scanner. Pricing-lookup is not its own RPC — it's a change to the *shape* of two existing RPCs' responses (`GetInsightsSummary`, `ListSessionTokens`). Inventing a schema-violating entry with no backing RPC would be accepted by no tooling and drift from the very first `make registry-generate`. The new UI badge/legend behavior, by contrast, *is* a legitimate new component-scoped frontend feature (component-based schema, no RPC binding required) — deviation from `research/features.md`'s default recommendation, justified by direct schema/tooling verification research did not perform. |
| `ModelFamily` representation | Plain `string` (unchanged) | Type-driven-design: only introduce a newtype when it prevents a real confusion class | A `ModelFamily` newtype (`type ModelFamily string`) wrapping the normalized key | `NormalizeModelFamily` already centralizes every place a raw model ID becomes a lookup key; a newtype here would ripple through `map[string]ModelPricing`, every proto string field (which can't hold a Go newtype anyway), and every test fixture in `pricing_test.go`/`insights_service_test.go` for a benefit (preventing "wrong string passed where a family was expected") that a single centralizing function already provides. Out-of-scope churn relative to this project's goals. |
| `PricingTable.Prices` unpriced-family aggregation shape (per call) | `EstimateCost` → `[]string` (session-scoped, small, order matters for stable proto/UI rendering, so returned pre-sorted); `ModelFamilyCost` → `map[string]bool` (caller needs O(1) per-family membership checks when building `ModelBreakdown` rows and `DailyTokenBucket.UnpricedModels`, and is already merging across multiple `ParseResult`s) | `research/architecture.md` §3 exact recommendation | A single unified return shape (e.g. both return `[]string`) | Forcing `ModelFamilyCost`'s caller (`insights_service.go:179,192`) to do `O(n)` "already in slice?" checks on every merge across sessions/days is unnecessary — a map is the natural accumulator there, whereas `EstimateCost`'s caller just needs a per-session ordered list to store directly into a proto `repeated string` field. |

---

## Migration Plan

**Omitted.** No schema or database migration is involved — this is an in-memory Go map (`PricingTable.Prices`), a proto message change (regenerated via `make proto-gen`, not a stored-data migration), and a new optional JSON config file that is additive/absent-safe by construction (`LoadPricingOverride` gracefully no-ops when the file doesn't exist, per Story 1.4.1).

---

## Observability Plan

- **Logs**:
  - Startup: after `pricing := tokens.DefaultPricingTable()` (post-override) in `server/dependencies.go`, log `log.Warn("pricing table is stale", "loadedAt", pricing.LoadedAt)` if `pricing.IsStale()` — wires the existing, currently-dead `IsStale()` method into a visible signal for the first time (Story 1.5.2).
  - Startup: `LoadPricingOverride` failure (missing file is not an error to log; malformed JSON or unreadable file is) logs `log.Warn("failed to load pricing override, using defaults", "path", overridePath, "err", err)` (Story 1.4.1) — never silently swallowed, never crashes startup.
  - Runtime: `InsightsService.GetInsightsSummary` logs `log.Warn("insights: unpriced model family observed", "family", family)` **once per family per process lifetime** (deduped via a small mutex-guarded `map[string]bool` field on `InsightsService`, initialized in `NewInsightsService`) the first time a previously-unseen unpriced family appears in a response — catches the *next* new model family in production the moment it's first used, independent of anyone looking at the dashboard (Story 1.5.2; recommended by `research/pitfalls.md` §5 as the strongest, cheapest G4 guardrail).
- **Metrics**: none added. This is a low-traffic, single-operator internal tool (`research/ux.md` §3) — a log line is sufficient signal; a new metrics pipeline would be disproportionate.
- **Alerts**: none — no on-call, single user (Tyler). "No new alerts required."

---

## Risk Control

- **Feature flag**: not gated. This is a bug-fix-shaped correctness change to an internal single-user dashboard, not a risky behavioral change requiring staged rollout. The one operator-facing knob is the pricing override file itself (`<configDir>/pricing_overrides.json`), which is opt-in by presence (absent file = no behavior change from today).
- **Rollback procedure**: standard revert via PR close + revert commit. No data migration to unwind (Migration Plan: omitted).
- **Staged rollout**: full rollout on merge (single-operator tool, no cohort concept applies).

---

## Unresolved Questions

*(Genuinely open items are checkboxes with an owner. Decisions already made during planning — including the two the requirements explicitly asked to be recorded rather than left open — are stated as resolved, not as checkboxes.)*

- [ ] **Exact $/MTok figures for `claude-sonnet-5` (and any other newly-active families) are not yet verified.** `research/build-vs-buy.md` §3 cites an *illustrative, cached-2026-06-24* table from the bundled `claude-api` skill — those numbers must **not** be copied into `pricing.go`. Blocks Story 1.1.1 — owner: whoever implements Tasks 1.1.1a and 1.1.1a2, which are a live `WebFetch` against `https://platform.claude.com/docs/en/pricing.md` plus an independent second confirmation (repeat `WebFetch` or corroborating `WebSearch`), immediately before writing the table entry.
- **Resolved — Sonnet 5 introductory-vs-steady-state rate**: use the **introductory rate**, with `EffectiveDate` set to the implementation date (expected 2026-07-27 or later) and a code comment noting the 2026-08-31 steady-state transition. No separate hardcoded reminder mechanism is added: with `EffectiveDate = 2026-07-27`, `IsStale()`'s 30-day threshold trips around **2026-08-26** — which lands *before*, not after, the 2026-08-31 transition (a correction to the looser "shortly after" framing in requirements.md; the arithmetic is `EffectiveDate + 30d < 2026-08-31`). Either way this is a self-resetting guardrail that surfaces via Story 1.5.2's startup log roughly 5 days ahead of the actual rate change, which is a *better* outcome than "shortly after" and requires no extra code.
- **Resolved — `<synthetic>` sentinel handling**: filter at the parser boundary (`session/tokens/parser.go`), not as a zero-priced `DefaultPricingTable()` entry. See Pattern Decisions table and Epic 1.2. Empirically verified safe (198/198 sampled turns have zero usage).
- **Resolved — `server/services/capacity_monitor.go`'s independent, substring-matched pricing table is out of scope for this project.** Recorded in `ADR-002-capacity-monitor-pricing-scope-deferred.md`. A fast-follow bug must be filed (`pm:log-bug` or a new backlog item) covering its silent mis-pricing of any Claude model not matching `opus`/`haiku`/`flash`/`pro` (falls through to a "sonnet default" that may be wrong, not merely missing) and its live feed into `CostBudgetUSD`/`cost_budget_exceeded`, which can pause/stop autonomous sessions today. This plan does not touch `capacity_monitor.go`. Story 3.1.1 below is the task that files the fast-follow — it is documentation/process, not a code task.
- **Resolved — orphaned sessions ("ORPHANED: 24" in the reported screenshot) are confirmed unrelated.** `session/tokens/association.go`'s `Associator.Associate()` matching heuristics are a separate root cause from pricing-table gaps (`research/features.md` §2). No task in this plan touches `association.go`. Noted explicitly here so a future reader of this plan doesn't wonder if it was missed.
- **Resolved — `server/services/backlog_service.go`'s `buildCostLookup`/`server/services/backlog_service_query.go`'s cost query have the identical unpriced-silently-`$0` bug and are also out of scope for this project (beyond the minimal compile fix Task 1.3.1g requires).** Recorded in `ADR-003-backlog-cost-lookup-scope-deferred.md`, mirroring ADR-002's treatment of `capacity_monitor.go`. A fast-follow bug must be filed covering threading the unpriced signal through `buildCostLookup`'s return value and `SessionCostEntry`, plus a Backlog UI badge mirroring Epic 2.3's `SessionsTable.tsx` treatment. This plan does not touch Backlog's cost display beyond Task 1.3.1g's compile fix.
- **Resolved — historical pre-fix cost re-computation/backfill is out of scope** (per requirements.md Non-Goals). One accepted, documented rough edge: once `claude-sonnet-5` pricing lands (AC-1), already-recorded historical sonnet-5 usage will read as a *plainly-priced* `$0`-or-real-cost (computed live against the current table, since `EstimateCost`/`ModelFamilyCost` recompute from raw tokens on every request rather than persisting cost-at-time-of-use) with **no** `PricingUnavailable`/`UnpricedModels` flag, even for dates before the fix shipped — this is expected, not a bug, and is called out here so it isn't mistaken for one later.
- **Resolved — alternative "additive helper" design for G2 (considered, rejected).** A separate, earlier flat-pipeline planning pass for this same backlog item (`docs/tasks/sonnet-5-costs-blank/plan.md`, predates this SDD run) proposed a non-breaking alternative to ADR-001: a new additive method `func (pt *PricingTable) UnpricedModels(r *ParseResult) []string`, called alongside the existing unchanged `EstimateCost`/`ModelFamilyCost`, avoiding any signature change and therefore Task 1.3.1g's `backlog_service.go`/`backlog_service_query.go` compile fix entirely. This is a real, smaller-diff option — but it was not adopted: it requires the helper to independently re-derive "which families are unpriced" via its own second pass over `r.TurnTimeline`, rather than returning the fact directly from the one loop that already knows it while computing cost. That's the same class of risk this repo's `.claude/rules/go-double-checked-locking.md` warns against in spirit (return what was actually computed, not a value re-derived by a second, potentially-drifting computation) — ADR-001's additive-second-return-value approach was kept because it guarantees the unpriced signal and the cost figure can never disagree, at the cost of two trivial one-line call-site edits (already resolved by Task 1.3.1g/h).
- **Resolved — `MODEL_COLORS` has no `claude-sonnet-5` swatch in `ModelOverTimeChart.tsx:26-33`.** Confirmed cosmetic-only: `colorForModel()` (`:38`) falls back to `FALLBACK_COLORS[index % FALLBACK_COLORS.length]` for any family missing from the explicit map, so an unpriced-or-priced sonnet-5 series still renders with a distinct, stable color — no functional gap. Not worth a task; left as-is per this project's own bias toward minimal diffs.

---

## Dependency Visualization

```
Phase 1 — Backend Pricing Correctness
  Epic 1.1 (G1: pricing entries)         Epic 1.2 (<synthetic> filter)
        |                                        |
        |                                        v
        |                               Epic 1.3 (G2: unpriced signal, backend)
        |                                 Story 1.3.1 (PricingTable signatures)
        |                                        |
        |                                        v
        |                                 Story 1.3.2 (proto fields + proto-gen)
        |                                        |
        |                                        v
        |                                 Story 1.3.3 (insights_service.go call sites)
        |                                        |
        +----------------------+-----------------+
                                |
                                v
                       Epic 1.4 (G3: override wiring)   Epic 1.5 (G4: guardrails)
                                |                                 |
                                +----------------+----------------+
                                                 |
                                                 v
Phase 2 — Frontend Visibility (needs Story 1.3.2's generated TS types)
  Epic 2.1 (ModelBreakdownChart) -> Epic 2.2 (SummaryCards) -> Epic 2.3 (SessionsTable) -> Epic 2.4 (DailySpendChart, ModelOverTimeChart, ProjectedCostCard)
                                                 |
                                                 v
Phase 3 — Scope Decision Documentation (independent, can run any time)
  Epic 3.1 (capacity_monitor.go fast-follow filing)
                                                 |
                                                 v
Phase 4 — Registry & Ship Gate (needs everything above)
  Epic 4.1 (AC-6 registry) -> Epic 4.2 (AC-7 make quick-check — FINAL TASK)
```

Epic 1.1 and Epic 1.2 have no dependency on each other and can be done in either order, but both must land before Epic 1.3 Story 1.3.3 (call-site wiring) so the shipped diff never has a state where `<synthetic>` shows up as "pricing unavailable." Epic 2.4 has no ordering dependency on Epics 2.1–2.3 beyond needing the same Story 1.3.2 generated types — it's listed last only because it was added later during plan repair, not because it must run after the others. Phase 3 is fully independent (pure documentation/process) and can run at any point.

---

## Phase 1: Backend Pricing Correctness

### Epic 1.1: G1 — Close the immediate pricing gap
**Goal**: `DefaultPricingTable()` has a correct, freshly-verified entry for every currently-active Claude model family, closing the reported "Sonnet 5 costs blank" gap at the source.

#### Story 1.1.1: Add verified pricing entries for missing active model families
**As a** user of the Insights dashboard, **I want** Sonnet 5 (and any other missing active family) usage to be priced, **so that** the Total Cost tile reflects my real spend.

**Acceptance Criteria** (AC-1):
- `DefaultPricingTable()` includes a `claude-sonnet-5` entry (and `claude-opus-5`/`claude-haiku-5` if confirmed active) with all four rate fields populated from a live source, not memory/training data.
  - *Given* `ParseResult{PrimaryModel: "claude-sonnet-5-20250929", TurnTimeline: []TurnStats{{Model: "claude-sonnet-5-20250929", Input: 1_000_000, Output: 1_000_000}}}` and `pt := DefaultPricingTable()` (post-fix), *When* `pt.EstimateCost(result)` is called, *Then* `cost` equals `pt.Prices["claude-sonnet-5"].InputPricePerMTok + pt.Prices["claude-sonnet-5"].OutputPricePerMTok` (each rate × 1 for 1,000,000 tokens) and `unpriced` is empty — no longer the `0.0` it would have returned pre-fix.

**Files**: `session/tokens/pricing.go`, `session/tokens/pricing_test.go`

##### Task 1.1.1a: WebFetch current Anthropic pricing (~5 min)
- `WebFetch` `https://platform.claude.com/docs/en/pricing.md` at implementation time. Record the verified Input/Output $/MTok for every `claude-*` family currently listed as active, and the standard cache-write/cache-read multipliers (historically ≈1.25× input for 5-min-TTL cache write, ≈0.1× input for cache read — confirm current values on the page, don't assume unchanged from existing 3.x/4.x entries).
- Cross-check against `session/tokens/pricing_test.go`'s existing `TestEstimateCost_WhenKnownModel_ExpectExactPrice` numbers (`claude-sonnet-4`: $3/$15) to sanity-check the fetched format before trusting new entries.
- **Failure mode**: if the `WebFetch` fails, times out, or the page's format has changed enough that rates can't be reliably extracted: (1) retry the `WebFetch` once; (2) if it still fails, try `WebSearch` for an official Anthropic pricing announcement/blog post covering the missing family; (3) if neither succeeds, stop and escalate to the user (Tyler) rather than falling back to memorized/training-data figures or stalling indefinitely — this task exists specifically to prevent unverified numbers from entering `pricing.go` (per Unresolved Questions), so guessing defeats its purpose. Do not let Task 1.1.1b proceed without a live-verified source.
- Files: none written yet — this task is a research/verification step whose output feeds Task 1.1.1b.

##### Task 1.1.1a2: Independently re-verify the fetched figures before they're written into `pricing.go` (~5 min)
- **Why this task exists** (pre-mortem Failure #1, P1): a single `WebFetch` has no independent check — if the implementing agent mistranscribes a number (transposed digit, per-1K-vs-per-1M mix-up, or picks the promotional rate when steady-state was intended or vice versa), nothing catches it, because Task 1.1.1e's test would otherwise hardcode whatever number this same fetch produced (circular — it can only catch a later code regression, not a transcription error at the source).
- Obtain a **second, independent confirmation** of Task 1.1.1a's figures before Task 1.1.1b writes them into `pricing.go`. Either is acceptable:
  - (a) A second `WebFetch` of the same `https://platform.claude.com/docs/en/pricing.md` page, done as a fresh fetch (not reusing cached output from Task 1.1.1a), with the two sets of Input/Output/cache-write/cache-read figures compared field-by-field; or
  - (b) A `WebSearch` for an official Anthropic pricing announcement/blog post/changelog entry covering `claude-sonnet-5` (and any other newly-active family from Task 1.1.1c), with that source's figures compared against Task 1.1.1a's.
- **If the two sources agree**: proceed to Task 1.1.1b with the confirmed figures.
- **If the two sources disagree on any field**: stop and escalate to the user (Tyler) with both figures shown side by side, rather than silently picking one source over the other or averaging them. Do not let Task 1.1.1b proceed with an unresolved mismatch.
- Files: none written yet — this task is a second research/verification step whose output, together with Task 1.1.1a's, feeds Task 1.1.1b.

##### Task 1.1.1b: Add `claude-sonnet-5` entry to `DefaultPricingTable()` (~3 min)
- In `session/tokens/pricing.go:23-77`, add a `"claude-sonnet-5": {ModelFamily: "claude-sonnet-5", InputPricePerMTok: <verified>, OutputPricePerMTok: <verified>, CacheWritePerMTok: <verified>, CacheReadPerMTok: <verified>, EffectiveDate: "<today's date, e.g. 2026-07-27>"}` entry using Task 1.1.1a's introductory-rate figures, cross-confirmed by Task 1.1.1a2 (per Unresolved Questions' resolved decision). Do not proceed if Task 1.1.1a2 escalated an unresolved mismatch.
- Add a `//` comment directly above the entry noting: `// Introductory rate through 2026-08-31; steady-state rate differs — re-verify via WebFetch after that date.`
- Files: `session/tokens/pricing.go`

##### Task 1.1.1c: Add any other confirmed-active missing families (~3 min)
- Based on Task 1.1.1a's fetch, add entries for `claude-opus-5`/`claude-haiku-5` (or whichever families the live pricing page lists as active and currently missing from `DefaultPricingTable()`) following the same shape as Task 1.1.1b.
- If no other families are missing, skip this task (do not add speculative entries for families not confirmed active).
- Files: `session/tokens/pricing.go`

##### Task 1.1.1d: Update `DefaultPricingTable()`'s doc comment (~2 min)
- `session/tokens/pricing.go:21-22`'s `// DefaultPricingTable returns a PricingTable with hardcoded defaults as of 2026-05-15.` — update the "as of" date to match Task 1.1.1b's `EffectiveDate`.
- Files: `session/tokens/pricing.go`

##### Task 1.1.1e: Update existing pricing tests for the new entries (~3 min)
- `session/tokens/pricing_test.go`'s `TestEstimateCost_WhenKnownModel_ExpectExactPrice` and `TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded` currently use `claude-sonnet-4` fixtures — no change needed to those, but add one new test `TestEstimateCost_WhenSonnet5Model_ExpectExactPrice` mirroring the same shape (`PrimaryModel: "claude-sonnet-5-6"`, 1M input + 1M output, asserting `cost` against the exact verified figures from Task 1.1.1b — hardcode the verified numbers here, this is a test, not the production table).
- **Do not stop at the bare hardcoded-number assertion above** — per pre-mortem Failure #1 (P1), that assertion alone is circular: it can only catch a later code regression away from whatever number Tasks 1.1.1a/1.1.1a2 produced, not a transcription error in the number itself (both the code and the test would independently agree on the same wrong figure). Add a **second, plausibility-based assertion** in the same test, independent of the exact hardcoded value: assert that `pt.Prices["claude-sonnet-5"].InputPricePerMTok` and `.OutputPricePerMTok` each fall within a bounded multiple of `pt.Prices["claude-sonnet-4"]`'s known-correct rate (e.g. between 0.2x and 5x — same-generation Claude models are typically within a bounded ratio of each other; widen/narrow the band if Task 1.1.1a2's confirmed figures warrant it, but the band must be non-trivial enough to fail on a gross unit/order-of-magnitude error such as a per-1K/per-1M mix-up or a transposed digit). This plausibility check fails even if the "verified" figure itself was wrong in a way both sources of Task 1.1.1a2 happened to share, or if a future edit reintroduces a unit-scale bug.
- Files: `session/tokens/pricing_test.go`

---

### Epic 1.2: Exclude the `<synthetic>` sentinel from model-family aggregation
**Goal**: The internal `<synthetic>` turn marker Claude Code's own transcript writer emits never enters `TurnTimeline`/`modelCounts`, so it can never be mistaken for a real unpriced model once Epic 1.3 ships the unpriced-signal UI.

#### Story 1.2.1: Filter `<synthetic>` at the parser boundary
**As a** developer relying on Insights' pricing-unavailable indicator, **I want** the indicator to never fire for the internal `<synthetic>` marker, **so that** the signal stays trustworthy (no false positives).

**Acceptance Criteria** (supports AC-2/AC-3 correctness, not independently numbered in requirements.md):
- *Given* a JSONL assistant entry with `"model":"<synthetic>"` and `"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}` (the exact real shape confirmed on this machine), *When* `Parser.processAssistantEntry` processes it, *Then* `result.TurnTimeline` does not gain an entry for it and `modelCounts["<synthetic>"]` is never incremented (so it can never become `PrimaryModel` and can never reach `PricingTable.Prices["<synthetic>"]` lookup).

**Files**: `session/tokens/parser.go`, `session/tokens/parser_test.go`

##### Task 1.2.1a: Add the sentinel constant and guard `modelCounts`/`TurnTimeline` (~4 min)
- In `session/tokens/parser.go`, add `const syntheticModelSentinel = "<synthetic>"` near the top of the file (alongside other package-level consts, or directly above `processAssistantEntry` at line 117 if no consts block exists).
- At line 178 (`if msg.Model != "" { modelCounts[msg.Model]++ }`), change the guard to `if msg.Model != "" && msg.Model != syntheticModelSentinel { modelCounts[msg.Model]++ }`.
- At line 183 (`result.TurnTimeline = append(result.TurnTimeline, turn)`), wrap it: `if msg.Model != syntheticModelSentinel { result.TurnTimeline = append(result.TurnTimeline, turn) }`.
- Leave `msg.Usage` totals (`result.TotalInput` etc., lines 152-155), tool-use extraction (lines 159-176), and `result.MessageCount++` (line 182) unchanged — `<synthetic>` turns always carry zero usage (empirically confirmed) so this doesn't change any aggregate totals, and narrowing the change to exactly `modelCounts`/`TurnTimeline` avoids collateral risk to `ToolUsage`/`MessageCount` tracking that's out of this project's scope.
- The `syntheticModelSentinel` filter is a literal-string match scoped to exactly this one sentinel today — a future internal zero-usage marker with a different literal (e.g. `<none>`, `<internal>`) would not be caught by this guard and would flow through to the unpriced-signal machinery as a false-positive "pricing unavailable" badge. This is a reasonable, explicitly-scoped blind spot (empirically verified for `<synthetic>` specifically, and the failure mode is a visible false-positive, not a silent one — per `adversarial-review.md`), but leave a comment at the constant declaration making the narrowness visible in the code, not just in planning docs: `// only known internal sentinel today; extend this list if Claude Code adds another`.
- Files: `session/tokens/parser.go`

##### Task 1.2.1b: Add regression test (~3 min)
- In `session/tokens/parser_test.go`, add `TestParseFile_WhenSyntheticModelTurn_ExpectExcludedFromTimelineAndModelCounts`, following the existing fixture pattern (see `TestParseFile_WhenMissingUsageField_ExpectZeroTokensForThatTurn` at line 51 for the JSONL-fixture-string style used in this file). Assert the resulting `ParseResult.TurnTimeline` has no entry with `Model == "<synthetic>"` and `ParseResult.PrimaryModel` is not `"<synthetic>"` when a real model turn is also present in the fixture.
- Files: `session/tokens/parser_test.go`

---

### Epic 1.3: G2 — Surface the unpriced-model signal, backend
**Goal**: `EstimateCost`/`ModelFamilyCost` report which families were skipped; that signal reaches the RPC response.

#### Story 1.3.1: Change `PricingTable` cost methods to report unpriced families
**As a** backend developer, **I want** `EstimateCost`/`ModelFamilyCost` to tell their caller which families had no pricing entry, **so that** the caller can flag it instead of silently reporting `$0`.

**Acceptance Criteria** (AC-2, exact example from requirements):
- *Given* a `ParseResult` with `TurnTimeline: [{Model: "claude-sonnet-5-20250929", Input: 1000, Output: 500}]` and a `PricingTable` missing a `claude-sonnet-5` entry, *When* `EstimateCost` is called, *Then* it returns `(0.0, []string{"claude-sonnet-5"})`.

**Files**: `session/tokens/pricing.go`, `session/tokens/pricing_test.go`, `server/services/backlog_service.go`, `server/services/backlog_service_query.go`

##### Task 1.3.1a: Change `EstimateCost`'s signature and main loop (~5 min)
- `session/tokens/pricing.go:140-181`: change signature to `func (pt *PricingTable) EstimateCost(r *ParseResult) (cost float64, unpriced []string)`.
- Add `unpricedSet := make(map[string]bool)` near the other `map[string]int64` declarations at lines 146-149.
- In the main loop (lines 169-178), change `if !ok { continue }` to `if !ok { unpricedSet[family] = true; continue }`.
- Before `return total`, build a sorted slice: `unpriced = make([]string, 0, len(unpricedSet)); for f := range unpricedSet { unpriced = append(unpriced, f) }; sort.Strings(unpriced)`. Add `"sort"` to imports.
- Files: `session/tokens/pricing.go`

##### Task 1.3.1b: No separate change needed — `EstimateCost`'s `PrimaryModel` fallback shares the main loop's single lookup site (~1 min, verification only)
- Confirmed by direct trace of `session/tokens/pricing.go:140-181`: unlike `ModelFamilyCost` (which has two independent `pt.Prices[family]` lookup sites — see Task 1.3.1c), `EstimateCost` has only **one**. The `if len(r.TurnTimeline) == 0 && r.PrimaryModel != ""` fallback (lines 160-166) populates `modelInputs`/`modelOutputs`/etc. for the synthetic single-family case, and that map is then consumed by the *single* pricing loop at lines 169-178 — the same loop Task 1.3.1a instruments with `unpricedSet` tracking. So the fallback path is automatically covered with no additional code. (This corrects `research/pitfalls.md` §3's framing, which describes "the same duplication risk" applying to both of `EstimateCost`'s branches — that duplication is real for `ModelFamilyCost`, which does have two separate lookup sites, but not for `EstimateCost`, which has only one.)
- Files: none (documentation of a verified fact; no diff)

##### Task 1.3.1c: Change `ModelFamilyCost`'s signature and both code paths (~5 min)
- `session/tokens/pricing.go:203-237`: change signature to `func (pt *PricingTable) ModelFamilyCost(r *ParseResult) (costs map[string]float64, unpriced map[string]bool)`.
- Initialize `unpriced := make(map[string]bool)` alongside `result := make(map[string]float64)` at line 208.
- In the per-turn loop (lines 210-221), change `if !ok { continue }` to `if !ok { unpriced[family] = true; continue }`.
- In the `PrimaryModel` fallback (lines 224-234), change `if ok { ... }` to also handle the `else` case: `if ok { ...cost...; result[family] = cost } else { unpriced[family] = true }`.
- Return `result, unpriced` instead of just `result`.
- Files: `session/tokens/pricing.go`

##### Task 1.3.1d: Fix the 3 existing `EstimateCost` call sites in `pricing_test.go` (~3 min)
- `TestEstimateCost_WhenKnownModel_ExpectExactPrice` (line 46), `TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded` (line 75): change `cost := pt.EstimateCost(result)` to `cost, unpriced := pt.EstimateCost(result)` and add `assert.Empty(t, unpriced)` to each (both use known models, so `unpriced` must be empty — a useful assertion, not boilerplate).
- Files: `session/tokens/pricing_test.go`

##### Task 1.3.1e: Rewrite `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` for AC-5 (~3 min)
- Lines 51-63: change `cost := pt.EstimateCost(result)` / `assert.Equal(t, 0.0, cost)` to `cost, unpriced := pt.EstimateCost(result)` / `assert.Equal(t, 0.0, cost)` / `assert.Equal(t, []string{"gpt-99-turbo"}, unpriced)` — this is the literal AC-5 example from requirements.md, now assertable.
- **Rename the test function** from `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` to `TestEstimateCost_WhenUnknownModel_ExpectZeroCostAndFamilyFlaggedUnpriced` — the old name describes and asserts the pre-fix silent-fallback behavior this exact task disproves; keeping the old name after this rewrite would leave the test's own name lying about what it now verifies. Update any other reference to the old name in this file (e.g. comments) accordingly.
- Files: `session/tokens/pricing_test.go`

##### Task 1.3.1f: Add a mixed-family regression test for `ModelFamilyCost` (~4 min)
- Add `TestModelFamilyCost_WhenMixedKnownAndUnknownFamilies_ExpectKnownPricedAndUnknownFlagged` to `session/tokens/pricing_test.go`: build a `ParseResult` whose `TurnTimeline` contains both a priced family (e.g. `claude-sonnet-4`) and an unpriced family (e.g. `gpt-99-turbo`) in the same result. Assert `costs` contains the correct nonzero cost for the priced family only, and `unpriced["gpt-99-turbo"] == true` while `unpriced["claude-sonnet-4"]` is absent/false.
- This is the one test shape that exercises `ModelFamilyCost`'s `PrimaryModel`-fallback guard (`pricing.go:224`, which triggers on an empty `result` map, not an empty `TurnTimeline` — a subtly different condition than `EstimateCost`'s guard at line 160) interacting with a partially-populated `result` map, per `adversarial-review.md`'s flagged gap ("no test covers a `ParseResult` with both a priced and an unpriced model family in the same `TurnTimeline`").
- Files: `session/tokens/pricing_test.go`

##### Task 1.3.1g: Fix `EstimateCost` call sites outside `session/tokens` (~4 min)
- `EstimateCost`'s signature change in Task 1.3.1a breaks compilation at the two call sites that exist outside `insights_service.go`/`pricing_test.go` — confirmed by direct grep, these are the only two. Both must be fixed in the same commit as Task 1.3.1a; Go does not allow ignoring an added return value at an existing single-value-assignment call site without editing the call site itself (this corrects `ADR-001`'s Consequences section — see the ADR-001 edit accompanying this task).
- `server/services/backlog_service.go:429` — inside `BacklogService.buildCostLookup`'s closure, which returns plain `float64`. Change:
  ```go
  return pt.EstimateCost(r)
  ```
  to:
  ```go
  cost, _ := pt.EstimateCost(r)
  return cost
  ```
- `server/services/backlog_service_query.go:471` — change `cost := s.pricing.EstimateCost(result)` to `cost, _ := s.pricing.EstimateCost(result)`.
- Neither call site needs the unpriced signal itself — see the Unresolved Questions entry / `ADR-003-backlog-cost-lookup-scope-deferred.md` for the explicit scope decision on whether Backlog's cost display should surface unpriced usage in a later change.
- Files: `server/services/backlog_service.go`, `server/services/backlog_service_query.go`

##### Task 1.3.1h: Compile checkpoint — `go build ./...` (~1 min)
- Run `go build ./...` immediately after Tasks 1.3.1a, 1.3.1c, 1.3.1g land, and confirm it exits `0` before starting Story 1.3.2. This catches any other stray `EstimateCost`/`ModelFamilyCost` call site this plan missed at the point the signature change is introduced, rather than only at the final Epic 4.2 gate.
- Files: none (validation only)

---

#### Story 1.3.2: Extend the proto schema with unpriced-signal fields
**As a** frontend developer, **I want** the RPC response to carry pricing-unavailable flags, **so that** the UI can render them without inferring from `cost == 0`.

**Acceptance Criteria** (AC-2, wire-level):
- *Given* the proto changes below, *When* `make proto-gen` is run, *Then* `session/gen/proto/go/session/v1/insights.pb.go` gains `PricingUnavailable bool` on `ModelBreakdown` and `UnpricedModels []string` on `SessionTokenSummary`/`DailyTokenBucket`/`GetInsightsSummaryResponse`, and `web-app/src/gen/session/v1/insights_pb.ts` gains the matching camelCased fields, with no other generated field renamed or removed.

**Files**: `proto/session/v1/insights.proto`

##### Task 1.3.2a: Add `pricing_unavailable` to `ModelBreakdown` (~2 min)
- `proto/session/v1/insights.proto:64-71`: add `bool pricing_unavailable = 7; // true when total_input_tokens/total_output_tokens > 0 but no PricingTable entry exists for model_family` after `session_count = 6;`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.3.2b: Add `unpriced_models` to `SessionTokenSummary` (~2 min)
- `proto/session/v1/insights.proto:23-40`: add `repeated string unpriced_models = 17; // ModelFamily values with usage but no pricing entry, for this session` after `repeated TopToolEntry top_tools = 16;`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.3.2c: Add `unpriced_models` to `DailyTokenBucket` (~2 min)
- `proto/session/v1/insights.proto:50-61`: add `repeated string unpriced_models = 9; // union of unpriced ModelFamily values across sessions rolled into this day` after `map<string, int64> tokens_by_model = 8;`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.3.2d: Add `unpriced_models` to `GetInsightsSummaryResponse` (~2 min)
- `proto/session/v1/insights.proto:91-104`: add `repeated string unpriced_models = 13; // aggregate union across all sessions in this response, for a dashboard-level banner` after `google.protobuf.Timestamp pricing_as_of = 12;`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.3.2e: Regenerate proto bindings (~2 min)
- Run `make proto-gen`. Confirm `session/gen/proto/go/session/v1/insights.pb.go` and `web-app/src/gen/session/v1/insights_pb.ts` are updated and `go build ./...` still compiles (call sites not yet updated will show missing-field-usage only where Story 1.3.3 hasn't run yet — no compile errors expected from the proto regen alone).
- Files: `session/gen/proto/go/session/v1/insights.pb.go`, `web-app/src/gen/session/v1/insights_pb.ts` (generated, commit per this repo's convention — `web-app/src/gen` is tracked despite `.gitignore`)

---

#### Story 1.3.3: Thread the unpriced signal through `InsightsService`'s 4 call sites
**As a** user of the Insights dashboard, **I want** every cost figure the backend returns to carry its pricing-completeness status, **so that** the frontend never has to guess.

**Acceptance Criteria** (AC-2, service-level):
- *Given* two sessions — one using `claude-sonnet-4` only, one using an unpriced `claude-opus-6` — *When* `GetInsightsSummary` is called, *Then* `resp.Models` contains a `ModelBreakdown{ModelFamily: "claude-opus-6", EstimatedCostUsd: 0, PricingUnavailable: true}` entry, `resp.UnpricedModels` contains `"claude-opus-6"`, and the `claude-sonnet-4` entry has `PricingUnavailable: false`.

**Files**: `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.3.3a: Update `GetInsightsSummary`'s per-session call site (line 115) (~4 min)
- Change `costUSD := s.pricing.EstimateCost(r)` to `costUSD, unpriced := s.pricing.EstimateCost(r)`.
- Add `UnpricedModels: unpriced,` to the `summary := &sessionv1.SessionTokenSummary{...}` literal (lines 127-142).
- Add a new accumulator near the other totals at lines 58-70: `allUnpricedFamilies = make(map[string]bool)`. After computing `unpriced`, add `for _, f := range unpriced { allUnpricedFamilies[f] = true }`.
- Files: `server/services/insights_service.go`

##### Task 1.3.3b: Update the daily-bucket call site (line 179) (~4 min)
- Change `modelFamilyCostsForDay := s.pricing.ModelFamilyCost(r)` to `modelFamilyCostsForDay, unpricedForDay := s.pricing.ModelFamilyCost(r)`.
- Add a parallel accumulator `dailyUnpriced := make(map[string]map[string]bool)` near `dailyMap` (line 66). Inside the `if bucketDay != ""` block (around line 162), after creating/looking up `b`, also look up/create `dailyUnpriced[bucketDay]` the same way, then `for family := range unpricedForDay { dailyUnpriced[bucketDay][family] = true }`.
- Files: `server/services/insights_service.go`

##### Task 1.3.3c: Populate `DailyTokenBucket.UnpricedModels` when building the sorted daily slice (~3 min)
- In the "Build sorted daily slice" block (lines 229-238), when appending each `dailyMap[k]` to `daily`, also set `dailyMap[k].UnpricedModels = sortedKeys(dailyUnpriced[k])` (add a small local helper `sortedKeys(m map[string]bool) []string` if one doesn't already exist in this file — check for an existing sort helper first, e.g. near `buildTopEntries`, before adding a new one).
- Files: `server/services/insights_service.go`

##### Task 1.3.3d: Update the model-breakdown call site (line 192) and set `PricingUnavailable` (~4 min)
- Change `modelFamilyCosts := s.pricing.ModelFamilyCost(r)` to `modelFamilyCosts, unpricedFamilies := s.pricing.ModelFamilyCost(r)`.
- After the existing `for family, cost := range modelFamilyCosts { ... }` loop (lines 208-216), add: `for family := range unpricedFamilies { mb := modelMap[family]; if mb == nil { mb = &sessionv1.ModelBreakdown{ModelFamily: family}; modelMap[family] = mb }; mb.PricingUnavailable = true; mb.SessionCount++ }` — mirrors the existing defensive nil-check idiom already used in the loop directly above it, and (per `architecture-review.md`'s Concerns) increments `SessionCount` the same way the priced-family loop above it does, so an unpriced-but-heavily-used family's row doesn't show real nonzero token counts alongside a contradictory `SessionCount: 0`.
- Files: `server/services/insights_service.go`

##### Task 1.3.3e: Populate `GetInsightsSummaryResponse.UnpricedModels` (~2 min)
- In the `resp := &sessionv1.GetInsightsSummaryResponse{...}` literal (lines 261-274), add `UnpricedModels: sortedKeys(allUnpricedFamilies),` (reusing Task 1.3.3c's helper).
- Files: `server/services/insights_service.go`

##### Task 1.3.3f: Update `ListSessionTokens`'s call site (line 314) (~3 min)
- Change `costUSD := s.pricing.EstimateCost(r)` to `costUSD, unpriced := s.pricing.EstimateCost(r)` and add `UnpricedModels: unpriced,` to that function's `summary := &sessionv1.SessionTokenSummary{...}` literal.
- Files: `server/services/insights_service.go`

##### Task 1.3.3g: Add `insights_service_test.go` coverage for the unpriced signal (~5 min)
- Using the existing `newInsightsFixture`/`newResult` helpers (lines 56-90), add `TestGetInsightsSummary_WhenUnpricedModelFamily_ExpectPricingUnavailableFlagged`: build a fixture with `newResult("uuid-1", "gpt-99-turbo", "/proj", 1000, 500, 0, time.Now())`, call `GetInsightsSummary`, assert the returned `ModelBreakdown` for `"gpt-99-turbo"` has `PricingUnavailable == true` and `resp.Msg.UnpricedModels` contains `"gpt-99-turbo"`.
- Add `TestListSessionTokens_WhenUnpricedModelFamily_ExpectUnpricedModelsPopulated` mirroring the above for `ListSessionTokens`'s `SessionTokenSummary.UnpricedModels`.
- Files: `server/services/insights_service_test.go`

##### Task 1.3.3h: Add an end-to-end `<synthetic>` leak regression test (~4 min)
- **Why this task exists** (pre-mortem Failure #2, P2): Epic 1.2's `<synthetic>` filter is a parser-boundary unit test (Task 1.2.1b) proving the filter works for the one fixture it authored — it does not prove `<synthetic>` can never reach `resp.UnpricedModels` end-to-end through the full `GetInsightsSummary` path. Because Epic 1.3 now turns *any* unpriced family into a loud UI badge, a `<synthetic>` leak that used to be invisible (silent `$0.00`) would become a new, visible, confusing `"<synthetic> (pricing unavailable)"` artifact this project itself would introduce.
- In `server/services/insights_service_test.go`, add `TestGetInsightsSummary_WhenSyntheticTurnMixedWithRealTurns_ExpectSyntheticNeverSurfacedAsUnpriced`: build a fixture (using `newInsightsFixture`/`newResult`) whose `ParseResult.TurnTimeline` mixes a `<synthetic>`-model turn (zero usage, matching the real empirical shape from `session/tokens/pricing.go`'s Domain Glossary entry) with real turns from at least one priced model (e.g. `claude-sonnet-4`) and one genuinely unpriced model (e.g. `gpt-99-turbo`), run it through the full `GetInsightsSummary` RPC path, and assert:
  - `resp.Msg.UnpricedModels` contains `"gpt-99-turbo"` but never contains `"<synthetic>"`.
  - No entry in `resp.Msg.Models` (`ModelBreakdown` rows) has `ModelFamily == "<synthetic>"`.
- This closes the gap between "filtered in the parser unit test" and "actually never surfaces on the wire," which is what a user would notice.
- Files: `server/services/insights_service_test.go`

---

#### Epic 1.3 Checkpoint: Regression gate before Epic 1.4

##### Task 1.3.4a: Run `go test ./session/tokens/... -v` (~2 min)
- Run `go test ./session/tokens/... -v` and confirm every test passes, including Story 1.3.1's updated/new tests (Tasks 1.3.1d/e/f). This is the discrete regression gate `research/pitfalls.md` §3 recommends immediately after the `EstimateCost`/`ModelFamilyCost` signature change, rather than deferring the first real test signal to the final `make quick-check` in Task 4.2.1a — a regression caught here is far cheaper to root-cause than one surfacing after Epics 1.4/1.5 and all of Phase 2 have stacked on top.
- Must be green before Epic 1.4 begins.
- Files: none (validation only)

---

### Epic 1.4: G3 — Wire up the pricing override mechanism
**Goal**: A JSON file can patch in new/corrected pricing without a code change + redeploy, loaded once at startup, failing soft on any error.

#### Story 1.4.1: Load `pricing_overrides.json` at startup with fail-soft fallback
**As an** operator (Tyler), **I want** to drop a JSON file with updated pricing and restart the service, **so that** I don't need a full code change + redeploy to fix a pricing gap.

**Acceptance Criteria** (AC-4):
- *Given* `<configDir>/pricing_overrides.json` contains `{"claude-sonnet-5": {"InputPricePerMTok": 2.00, "OutputPricePerMTok": 10.00, "CacheWritePerMTok": 2.50, "CacheReadPerMTok": 0.20, "EffectiveDate": "2026-07-27"}}`, *When* the server starts, *Then* `insightsSvc`'s `pricing.Prices["claude-sonnet-5"].InputPricePerMTok == 2.00` (the override value). *Given* that same file instead contains malformed JSON (e.g. a trailing comma), *When* the server starts, *Then* it still starts successfully, `pricing` retains `DefaultPricingTable()`'s unmodified values, and one `log.Warn` line is emitted.

**Files**: `server/dependencies.go`

##### Task 1.4.1a: Replace the bare `DefaultPricingTable()` call with override-aware loading (~5 min)
- `server/dependencies.go:1070`, replace `pricing := tokens.DefaultPricingTable()` with:
  ```go
  pricing := tokens.DefaultPricingTable()
  if configDir, cfgErr := config.GetConfigDir(); cfgErr == nil {
      overridePath := filepath.Join(configDir, "pricing_overrides.json")
      if overrideTable, loadErr := tokens.LoadPricingOverride(overridePath); loadErr == nil {
          pricing = overrideTable
      } else if !os.IsNotExist(loadErr) {
          log.Warn("failed to load pricing override, using defaults", "path", overridePath, "err", loadErr)
      }
  }
  ```
  This follows the exact idiom already used twice in this same file (`configDir, configErr := config.GetConfigDir()` at lines 846 and 881) and matches `config.LoadConfig()`'s fail-soft shape. Because `pricing` is only reassigned in the success branch, a malformed-JSON error leaves the original `DefaultPricingTable()` instance intact — satisfying the "don't discard valid defaults" requirement without any change to `LoadPricingOverride` itself.
- Files: `server/dependencies.go` (`config`, `filepath`, `os` already imported in this file — confirm no new imports needed)

##### Task 1.4.1b: Add a Go test exercising the fail-soft path (~4 min)
- **Committed test location**: `session/tokens/pricing_test.go`, alongside the existing `TestLoadPricingOverride_*` family — not `server/dependencies_test.go`, since `dependencies.go`'s `BuildRuntimeDeps` is a large, heavily-integration-shaped function not conducive to isolated unit testing, and no `server/dependencies_test.go` currently exists (confirmed; do not create one for this single case).
- Extend `session/tokens/pricing_test.go` with `TestLoadPricingOverride_WhenMalformedJSON_ExpectErrorReturnedDefaultsUntouched` asserting `LoadPricingOverride` returns a non-nil `error` and that a *separately held* `DefaultPricingTable()` reference is unaffected — this documents the caller-side contract Task 1.4.1a's "only reassign `pricing` on success" logic relies on, without needing a `BuildRuntimeDeps`-level integration test.
- Files: `session/tokens/pricing_test.go`

---

### Epic 1.5: G4 — Regression guardrails
**Goal**: The next new Claude model family produces a visible signal (test failure + runtime log), not another silent blank-cost gap.

#### Story 1.5.1: Completeness guardrail test
**As a** future maintainer, **I want** a test that fails the moment a new active Claude family is referenced without a pricing entry, **so that** this bug class can't recur silently.

**Acceptance Criteria** (AC-5, completeness half):
- *Given* a `knownActiveClaudeFamilies` list `["claude-opus-4", "claude-sonnet-4", "claude-haiku-4", "claude-opus-3", "claude-sonnet-3", "claude-haiku-3", "claude-sonnet-5"]` (post-Epic-1.1) maintained independently of `DefaultPricingTable()`'s keys, *When* a completeness test checks `_, ok := pt.Prices[family]` for each entry in that list, *Then* every check is `ok == true` — and if a maintainer later appends `"claude-opus-6"` to the list without adding a matching `DefaultPricingTable()` entry, the test fails with a clear message naming the missing family.

**Files**: `session/tokens/pricing_test.go`

##### Task 1.5.1a: Add the independently-maintained active-family list and completeness test (~4 min)
- In `session/tokens/pricing_test.go`, add:
  ```go
  // knownActiveClaudeFamilies is maintained independently of DefaultPricingTable()'s keys —
  // deliberately a second source of truth, so a maintainer must touch both this list and
  // the pricing table when a new Claude model family becomes active, giving this test a
  // real chance to fail loudly if one is updated without the other.
  var knownActiveClaudeFamilies = []string{
      "claude-opus-4", "claude-sonnet-4", "claude-haiku-4",
      "claude-opus-3", "claude-sonnet-3", "claude-haiku-3",
      "claude-sonnet-5",
  }

  func TestDefaultPricingTable_WhenKnownActiveFamily_ExpectPricingEntryExists(t *testing.T) {
      pt := DefaultPricingTable()
      for _, family := range knownActiveClaudeFamilies {
          _, ok := pt.Prices[family]
          assert.True(t, ok, "known active family %q has no DefaultPricingTable() entry", family)
      }
  }
  ```
- If Task 1.1.1c added `claude-opus-5`/`claude-haiku-5`, include them in the list too.
- Files: `session/tokens/pricing_test.go`

#### Story 1.5.2: Wire `IsStale()` and a runtime unpriced-family warning into logs
**As an** operator, **I want** a log line the moment pricing data goes stale or a new unpriced family is seen in production traffic, **so that** I don't have to notice a blank number on the dashboard myself.

**Acceptance Criteria** (AC-5, runtime-signal half):
- *Given* `pricing.IsStale()` returns `true` at startup (e.g. all `EffectiveDate`s are 31+ days old), *When* `server/dependencies.go` initializes `InsightsService`, *Then* a `log.Warn` line naming the stale table is emitted exactly once at startup.
- *Given* `GetInsightsSummary` is called twice in a row and both responses include `"claude-opus-6"` in `UnpricedModels`, *When* the second call completes, *Then* only **one** `log.Warn` line for `"claude-opus-6"` has been emitted across both calls (deduped, not spammed per-request).

**Files**: `server/dependencies.go`, `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.5.2a: Log `IsStale()` at startup (~2 min)
- In `server/dependencies.go`, immediately after Task 1.4.1a's override-loading block (still inside the `if homeDir, homeDirErr := os.UserHomeDir(); homeDirErr == nil` block, before `insightsSvc = services.NewInsightsService(...)`), add:
  ```go
  if pricing.IsStale() {
      log.Warn("pricing table is stale (an entry's EffectiveDate is 30+ days old)", "loadedAt", pricing.LoadedAt)
  }
  ```
- Files: `server/dependencies.go`

##### Task 1.5.2b: Add a deduped unpriced-family warn log to `InsightsService` (~5 min)
- `server/services/insights_service.go`: add an unexported field to the `InsightsService` struct (lines 21-25): `loggedUnpricedFamilies map[string]bool` and `logMu sync.Mutex`. Initialize the map in `NewInsightsService` (lines 27-38).
- At the end of `GetInsightsSummary`, after building `allUnpricedFamilies` (Task 1.3.3a), add a small unexported method call: `s.warnNewUnpricedFamilies(allUnpricedFamilies)` defined as:
  ```go
  func (s *InsightsService) warnNewUnpricedFamilies(families map[string]bool) {
      s.logMu.Lock()
      defer s.logMu.Unlock()
      for family := range families {
          if !s.loggedUnpricedFamilies[family] {
              s.loggedUnpricedFamilies[family] = true
              log.Warn("insights: unpriced model family observed", "family", family)
          }
      }
  }
  ```
- Add `"sync"` and `"github.com/tstapler/stapler-squad/log"` imports (the latter not currently imported in this file — confirmed via its current import block, `insights_service.go:3-14`; `capacity_monitor.go:16` and `dependencies.go:14` in this same repo already import it under this exact path, so this is a same-package-family addition, not a new dependency).
- Files: `server/services/insights_service.go`

##### Task 1.5.2c: Add a test asserting the dedup behavior (~4 min)
- In `server/services/insights_service_test.go`, add `TestGetInsightsSummary_WhenCalledTwiceWithSameUnpricedFamily_ExpectLoggedOnce` — since asserting on actual log output is awkward, instead assert on the *observable* dedup state: call `GetInsightsSummary` twice with the same unpriced-family fixture and assert `len(insightsSvc.loggedUnpricedFamilies) == 1` after both calls (whitebox test in the same package, consistent with this file already being `package services`).
- Files: `server/services/insights_service_test.go`

---

## Phase 2: Frontend Visibility (G2/AC-3)

*Depends on Story 1.3.2's regenerated `web-app/src/gen/session/v1/insights_pb.ts`.*

### Epic 2.1: `ModelBreakdownChart.tsx` — legend indicator (primary fix for the reported bug)
**Goal**: The screenshot bug (an unpriced model's bar is invisible because it's zero-height) is fixed by annotating the always-rendered legend entry, not the bar.

#### Story 2.1.1: Add a text-labeled legend badge for unpriced families
**As a** user viewing the Cost by Model Family chart, **I want** to see which model's bar is fake-zero because pricing is missing, **so that** I don't mistake it for genuinely free usage.

**Acceptance Criteria** (AC-3):
- *Given* `GetInsightsSummaryResponse.models` contains `ModelBreakdown{ModelFamily: "claude-opus-6", EstimatedCostUsd: 0, PricingUnavailable: true, TotalInputTokens: 500000}`, *When* `ModelBreakdownChart` renders, *Then* the legend entry reads `"claude-opus-6 (pricing unavailable)"` instead of just `"claude-opus-6"`.

**Files**: `web-app/src/app/insights/ModelBreakdownChart.tsx`, `web-app/src/app/insights/ModelBreakdownChart.css.ts`, `web-app/src/app/insights/ModelBreakdownChart.test.tsx` (new)

##### Task 2.1.1a: Thread `pricingUnavailable` through `DataPoint`/`toDataPoints` (~3 min)
- `ModelBreakdownChart.tsx:42-56`: add `pricingUnavailable: boolean;` to the `DataPoint` interface, and `pricingUnavailable: m.pricingUnavailable,` to the object built in `toDataPoints`.
- Files: `web-app/src/app/insights/ModelBreakdownChart.tsx`

##### Task 2.1.1b: Render the legend suffix and a badge dot variant (~4 min)
- Lines 106-113 (`legendRow`/`legendItem` map): change `{d.family}` to:
  ```tsx
  {d.family}
  {d.pricingUnavailable && <span className={unpricedLabel}> (pricing unavailable)</span>}
  ```
- No `role="alert"` — this is informational, not urgent, per `research/ux.md` §4.
- Files: `web-app/src/app/insights/ModelBreakdownChart.tsx`

##### Task 2.1.1c: Add `unpricedLabel` style (~2 min)
- `ModelBreakdownChart.css.ts`: add `export const unpricedLabel = style({ color: vars.color.warningText, fontStyle: "italic" });` — reuses the existing `warningText` token (same token `SessionsTable.css.ts`'s `orphanBadge` uses), per `css-architecture.md`'s "token names only" rule.
- Files: `web-app/src/app/insights/ModelBreakdownChart.css.ts`

##### Task 2.1.1d: Jest/RTL test for the legend badge (~4 min)
- New file `web-app/src/app/insights/ModelBreakdownChart.test.tsx`, following the existing pattern in `web-app/src/components/ui/Badge.test.tsx`. Add `describe("ModelBreakdownChart") > it("ModelBreakdownChart_should_showUnpricedSuffix_When_pricingUnavailable")`: render with a single `ModelBreakdown`-shaped prop where `pricingUnavailable: true`, assert `screen.getByText(/pricing unavailable/i)` is present.
- Files: `web-app/src/app/insights/ModelBreakdownChart.test.tsx`

---

### Epic 2.2: `SummaryCards.tsx` — aggregate footnote
**Goal**: The Total Cost tile visibly discloses that its number excludes unpriced usage, addressing the underlying "how much am I actually spending" concern even though the literal AC only requires an indicator.

#### Story 2.2.1: Add a `cardSub` footnote when any model is unpriced
**As a** user glancing at Total Cost, **I want** a one-line note when the figure is incomplete, **so that** I don't mistake it for my true total spend.

**Acceptance Criteria** (AC-3):
- *Given* `GetInsightsSummaryResponse.unpricedModels == ["claude-opus-6"]`, *When* `SummaryCards` renders, *Then* the Total Cost tile shows a `cardSub`-styled line reading `"excludes 1 unpriced model"` beneath the existing session-count `cardSub` line.

**Files**: `web-app/src/app/insights/SummaryCards.tsx`, `web-app/src/app/insights/SummaryCards.test.tsx` (new)

##### Task 2.2.1a: Add the footnote (~3 min)
- `SummaryCards.tsx:16-22` (the Total Cost `card` block): after the existing session-count `<span className={cardSub}>`, conditionally add:
  ```tsx
  {summary.unpricedModels.length > 0 && (
    <span className={cardSub}>
      excludes {summary.unpricedModels.length} unpriced model{summary.unpricedModels.length !== 1 ? "s" : ""}
    </span>
  )}
  ```
- Files: `web-app/src/app/insights/SummaryCards.tsx`

##### Task 2.2.1b: Jest/RTL test (~3 min)
- New file `web-app/src/app/insights/SummaryCards.test.tsx`. `describe("SummaryCards") > it("SummaryCards_should_showUnpricedFootnote_When_unpricedModelsPresent")`: render with a `GetInsightsSummaryResponse`-shaped prop where `unpricedModels: ["claude-opus-6"]`, assert `screen.getByText(/excludes 1 unpriced model/i)`.
- Files: `web-app/src/app/insights/SummaryCards.test.tsx`

---

### Epic 2.3: `SessionsTable.tsx` — per-session cost cell badge
**Goal**: The per-session cost column (which also flows through `fmtCost`, per `SessionsTable.tsx:159`) gets the same treatment as the chart and summary tile, since a session using only an unpriced model would otherwise show a plain `$0.0000` there too.

**`TopNTables.tsx` scope note**: `research/ux.md:120-125` flagged `SessionsTable.tsx`/`TopNTables.tsx` together and asked planning to confirm whether both need the same per-row cost treatment. Confirmed during this plan repair: `TopNTables.tsx` has no cost column at all (it renders top-N rankings by token count/tool usage, not cost), so it is correctly out of scope for this epic — recorded here explicitly so a future reader isn't left wondering whether it was checked or just dropped.

#### Story 2.3.1: Badge the cost cell when a session has unpriced usage
**As a** user scanning the sessions table, **I want** a session whose cost includes unpriced usage to be visibly flagged, **so that** I don't read its `$0.0000`/partial cost as complete.

**Acceptance Criteria** (AC-3):
- *Given* `SessionTokenSummary{EstimatedCostUsd: 0, UnpricedModels: ["claude-opus-6"]}`, *When* `SessionsTable` renders that row, *Then* the Cost cell shows `fmtCost(0)` followed by a badge reading `"unpriced"`, mirroring the existing `orphanBadge` pattern at `SessionsTable.tsx:137`.

**Files**: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 2.3.1a: Add `unpricedBadge` style (~2 min)
- `SessionsTable.css.ts`: add a sibling to `orphanBadge` (line 87), `export const unpricedBadge = style([orphanBadge, {}]);` (identical visual treatment via vanilla-extract composition — same warning tokens, no new colors) or, if `.css.ts` composition syntax needs an explicit base, duplicate `orphanBadge`'s declaration under a new export name `unpricedBadge`.
- Files: `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 2.3.1b: Render the badge in the Cost cell (~3 min)
- `SessionsTable.tsx:159`: change `<td className={tdRight}>{fmtCost(s.estimatedCostUsd)}</td>` to:
  ```tsx
  <td className={tdRight}>
    {fmtCost(s.estimatedCostUsd)}
    {s.unpricedModels.length > 0 && <span className={unpricedBadge}>unpriced</span>}
  </td>
  ```
- Add `unpricedBadge` to the existing import block at line ~20-21.
- Files: `web-app/src/app/insights/SessionsTable.tsx`

---

### Epic 2.4: Remaining cost-rendering surfaces — `DailySpendChart`, `ModelOverTimeChart`, `ProjectedCostCard`
**Goal**: Close the gap `adversarial-review.md` flagged as a BLOCKER — Epics 2.1–2.3 only wire the unpriced signal into 3 of the cost-rendering frontend surfaces. Three more surfaces render cost figures with no unpriced indicator and were untouched by the original Phase 2 scope: `DailySpendChart.tsx` (daily spend line), `ModelOverTimeChart.tsx` (per-family spend/tokens over time), and `useProjectedCost.ts`/`ProjectedCostCard` (projected monthly spend). The last of these is a real automated-decision consumer — `useProjectedCost` sums `b.estimatedCostUsd` across the current month's `DailyTokenBucket`s to project spend forward, and that projection drives `ProjectedCostCard`'s over-budget warning (`InsightsDashboard.tsx:170,187`) — so silently excluding unpriced usage here doesn't just look wrong, it actively under-warns Tyler about his real spend. `DailyTokenBucket.UnpricedModels` is already added to the proto and populated backend-side by Tasks 1.3.2c/1.3.3b/c; this epic is what makes it a live field instead of a dead one.

##### Task 2.4.0a: Verify current file contents before any Epic 2.4 edit task begins (~5 min, verification only)
- **Why this task exists** (pre-mortem Failure #3, P2): Epic 2.4's tasks below were added during the repair pass, not the original Step 0.5 creative pass, and the adversarial review's own verification language ("closely enough to trust the diffs described") is hedged, not a hard confirmation. In particular, `ModelOverTimeChart.tsx`'s legend may be library-rendered (e.g. a charting library's auto-generated `<Legend>` via a `formatter`/`content` prop) rather than the manually-mapped JSX list Task 2.4.2b assumes when it says to edit "the legend map (lines 154-162)."
- Before starting Tasks 2.4.1a–2.4.3e, read the actual current contents of `web-app/src/app/insights/DailySpendChart.tsx`, `web-app/src/app/insights/ModelOverTimeChart.tsx`, and `web-app/src/app/insights/ProjectedCostCard.tsx` (plus `web-app/src/lib/hooks/useProjectedCost.ts`). Confirm, for each file: (a) the line numbers cited in Tasks 2.4.1a/2.4.1b, 2.4.2a/2.4.2b, and 2.4.3a/2.4.3c still match; (b) `ModelOverTimeChart.tsx`'s legend is in fact manually-mapped JSX (not a charting-library-managed `<Legend>` component) — if it turns out to be library-managed, stop and adapt Task 2.4.2b's approach to that library's legend-customization mechanism (e.g. a custom `content`/`formatter` render prop) instead of applying the manual-JSX diff as literally written.
- Mirrors Task 1.3.1b's "verification only" pattern — this task produces no diff itself, only a confirmation (or a documented correction) that unblocks the edit tasks that follow.
- Files: none (verification only)

#### Story 2.4.1: `DailySpendChart.tsx` — badge days with unpriced usage
**As a** user viewing the daily spend chart, **I want** days that include unpriced-model usage visibly marked, **so that** I don't read a day's line value as complete when it isn't.

**Acceptance Criteria** (AC-3, extending it to this surface):
- *Given* `daily: DailyTokenBucket[]` where one bucket has `unpricedModels: ["claude-opus-6"]` and the rest have `unpricedModels: []`, *When* `DailySpendChart` renders, *Then* a footnote below the chart title reads `"1 day includes unpriced model usage"` (pluralized correctly for N days), and no footnote renders when every bucket's `unpricedModels` is empty.

**Files**: `web-app/src/app/insights/DailySpendChart.tsx`, `web-app/src/app/insights/DailySpendChart.css.ts`, `web-app/src/app/insights/DailySpendChart.test.tsx` (new)

##### Task 2.4.1a: Thread `hasUnpriced` through `DataPoint`/`toDataPoints` (~3 min)
- `DailySpendChart.tsx:21-33`: add `hasUnpriced: boolean;` to the `DataPoint` interface, and `hasUnpriced: (b.unpricedModels?.length ?? 0) > 0,` to the object built in `toDataPoints`.
- Files: `web-app/src/app/insights/DailySpendChart.tsx`

##### Task 2.4.1b: Render an unpriced-days footnote (~3 min)
- In the non-empty render branch (after `data` is computed), compute `const unpricedDayCount = data.filter((d) => d.hasUnpriced).length;` and, directly under `chartTitle`, conditionally render:
  ```tsx
  {unpricedDayCount > 0 && (
    <span className={unpricedFootnote}>
      {unpricedDayCount} day{unpricedDayCount !== 1 ? "s" : ""} include{unpricedDayCount === 1 ? "s" : ""} unpriced model usage
    </span>
  )}
  ```
- No `role="alert"` — informational, consistent with `research/ux.md` §4's "informational, not urgent" call already applied to `ModelBreakdownChart` (Task 2.1.1b).
- Files: `web-app/src/app/insights/DailySpendChart.tsx`

##### Task 2.4.1c: Add `unpricedFootnote` style (~2 min)
- `DailySpendChart.css.ts`: add `export const unpricedFootnote = style({ color: vars.color.warningText, fontStyle: "italic", fontSize: vars.fontSize.sm });` — reuses the existing `warningText` token, per `css-architecture.md`'s "token names only" rule (same token `ModelBreakdownChart.css.ts`'s `unpricedLabel` uses, Task 2.1.1c).
- Files: `web-app/src/app/insights/DailySpendChart.css.ts`

##### Task 2.4.1d: Jest/RTL test (~3 min)
- New file `web-app/src/app/insights/DailySpendChart.test.tsx`, following the pattern in `web-app/src/app/insights/ModelBreakdownChart.test.tsx` (Task 2.1.1d). `describe("DailySpendChart") > it("DailySpendChart_should_showUnpricedFootnote_When_anyDayHasUnpricedModels")`: render with one `DailyTokenBucket`-shaped prop where `unpricedModels: ["claude-opus-6"]`, assert `screen.getByText(/unpriced model usage/i)` is present; add a negative case asserting the footnote is absent when all `unpricedModels` are empty.
- Files: `web-app/src/app/insights/DailySpendChart.test.tsx`

---

#### Story 2.4.2: `ModelOverTimeChart.tsx` — legend annotation per unpriced family
**As a** user viewing the model-over-time chart, **I want** an unpriced family's legend entry marked the same way `ModelBreakdownChart` marks it, **so that** the two charts give a consistent signal for the same underlying gap.

**Acceptance Criteria** (AC-3, extending it to this surface):
- *Given* `daily: DailyTokenBucket[]` where at least one bucket's `unpricedModels` contains `"claude-opus-6"` and `costByModel`/`tokensByModel` also has a `"claude-opus-6"` key (so it appears in `collectModels`'s output), *When* `ModelOverTimeChart` renders, *Then* the legend entry for `claude-opus-6` reads `"claude-opus-6 (pricing unavailable)"`, mirroring `ModelBreakdownChart`'s fix (Task 2.1.1b) exactly.

**Files**: `web-app/src/app/insights/ModelOverTimeChart.tsx`, `web-app/src/app/insights/ModelOverTimeChart.css.ts`, `web-app/src/app/insights/ModelOverTimeChart.test.tsx` (new)

##### Task 2.4.2a: Add a `collectUnpricedModels` helper (~3 min)
- `ModelOverTimeChart.tsx`, near `collectModels` (lines 51-58): add
  ```ts
  function collectUnpricedModels(daily: DailyTokenBucket[]): Set<string> {
    const unpriced = new Set<string>();
    for (const bucket of daily) {
      for (const m of bucket.unpricedModels ?? []) unpriced.add(m);
    }
    return unpriced;
  }
  ```
- Files: `web-app/src/app/insights/ModelOverTimeChart.tsx`

##### Task 2.4.2b: Compute and pass the unpriced set into the legend render (~3 min)
- In `ModelOverTimeChart` (line 87), add `const unpricedModels = useMemo(() => collectUnpricedModels(daily), [daily]);`.
- In the legend map (lines 154-162), change:
  ```tsx
  {m}
  ```
  to:
  ```tsx
  {m}
  {unpricedModels.has(m) && <span className={unpricedLegendLabel}> (pricing unavailable)</span>}
  ```
- Files: `web-app/src/app/insights/ModelOverTimeChart.tsx`

##### Task 2.4.2c: Add `unpricedLegendLabel` style (~2 min)
- `ModelOverTimeChart.css.ts`: add `export const unpricedLegendLabel = style({ color: vars.color.warningText, fontStyle: "italic" });` — same token/shape as Task 2.1.1c's `unpricedLabel`.
- Files: `web-app/src/app/insights/ModelOverTimeChart.css.ts`

##### Task 2.4.2d: Jest/RTL test (~4 min)
- New file `web-app/src/app/insights/ModelOverTimeChart.test.tsx`. `describe("ModelOverTimeChart") > it("ModelOverTimeChart_should_showUnpricedSuffix_When_familyInUnpricedModels")`: render with `daily` data where one bucket's `costByModel`/`tokensByModel` includes `"claude-opus-6"` and `unpricedModels: ["claude-opus-6"]`, assert `screen.getByText(/pricing unavailable/i)` is present in the legend.
- Files: `web-app/src/app/insights/ModelOverTimeChart.test.tsx`

---

#### Story 2.4.3: `useProjectedCost.ts`/`ProjectedCostCard` — projection caveat
**As a** user relying on the projected-monthly-spend card, **I want** a caveat when the projection window included unpriced usage, **so that** I don't treat an under-projected number as my true expected spend — this is the one surface in this epic with real decision consequences (it feeds the over-budget warning).

**Acceptance Criteria** (AC-3, extending it to this surface — this is the strongest instance of `research/pitfalls.md` §7's "unpriced $0 looks cheapest, not unknown, and actively steers automated decisions" risk):
- *Given* `daily: DailyTokenBucket[]` with ≥7 days in the current month and at least one of those days has a non-empty `unpricedModels`, *When* `useProjectedCost(daily)` is called, *Then* the returned `ProjectedCostResult` has `hasUnpricedUsage: true`, and *When* `ProjectedCostCard` renders that result, *Then* it shows a caveat line (e.g. `"Projection excludes unpriced usage"`) alongside the existing `"Based on N of M days"` sub-line. *Given* no day in the current-month window has unpriced usage, *Then* `hasUnpricedUsage: false` and no caveat renders.

**Files**: `web-app/src/lib/hooks/useProjectedCost.ts`, `web-app/src/lib/hooks/__tests__/useProjectedCost.test.ts`, `web-app/src/app/insights/ProjectedCostCard.tsx`, `web-app/src/app/insights/ProjectedCostCard.css.ts`, `web-app/src/app/insights/ProjectedCostCard.test.tsx` (new)

##### Task 2.4.3a: Add `hasUnpricedUsage` to `ProjectedCostResult` (~3 min)
- `useProjectedCost.ts:4-7`: add `hasUnpricedUsage: boolean;` to the `ProjectedCostResult` interface.
- In the `useMemo` body, after `currentMonthBuckets` is computed (line 24), add: `const hasUnpricedUsage = currentMonthBuckets.some((b) => (b.unpricedModels?.length ?? 0) > 0);` and include `hasUnpricedUsage` in the returned object (line 32).
- Files: `web-app/src/lib/hooks/useProjectedCost.ts`

##### Task 2.4.3b: Update `useProjectedCost` unit tests (~3 min)
- In `web-app/src/lib/hooks/__tests__/useProjectedCost.test.ts`, add `useProjectedCost_should_setHasUnpricedUsageTrue_When_anyCurrentMonthDayHasUnpricedModels` and a negative case asserting `hasUnpricedUsage: false` when no bucket has unpriced usage.
- Files: `web-app/src/lib/hooks/__tests__/useProjectedCost.test.ts`

##### Task 2.4.3c: Render the caveat in `ProjectedCostCard` (~3 min)
- `ProjectedCostCard.tsx:40` (directly after the existing `<span className={sub}>Based on {projection.daysData} of {projection.daysInMonth} days</span>` line), conditionally add:
  ```tsx
  {projection.hasUnpricedUsage && (
    <span className={caveat}>Projection excludes unpriced usage</span>
  )}
  ```
- Files: `web-app/src/app/insights/ProjectedCostCard.tsx`

##### Task 2.4.3d: Add `caveat` style (~2 min)
- `ProjectedCostCard.css.ts`: add `export const caveat = style({ color: vars.color.warningText, fontStyle: "italic", fontSize: vars.fontSize.sm });` — same token/shape as the other unpriced indicators added across Epics 2.1/2.4.
- Files: `web-app/src/app/insights/ProjectedCostCard.css.ts`

##### Task 2.4.3e: Jest/RTL test for `ProjectedCostCard` (~3 min)
- New file `web-app/src/app/insights/ProjectedCostCard.test.tsx`. `describe("ProjectedCostCard") > it("ProjectedCostCard_should_showUnpricedCaveat_When_hasUnpricedUsageTrue")`: render with a `projection` prop where `hasUnpricedUsage: true`, assert `screen.getByText(/excludes unpriced usage/i)`; add a negative case for `hasUnpricedUsage: false`.
- Files: `web-app/src/app/insights/ProjectedCostCard.test.tsx`

---

## Phase 3: Scope Decision Documentation

### Epic 3.1: File the `capacity_monitor.go` fast-follow
**Goal**: Record and hand off the out-of-scope `CapacityMonitor.estimateCost` risk identified in `ADR-002-capacity-monitor-pricing-scope-deferred.md`, so it isn't silently forgotten once this project ships.

#### Story 3.1.1: File a fast-follow bug for `CapacityMonitor.estimateCost`
**As a** future maintainer, **I want** a tracked backlog item for the substring-matched `capacity_monitor.go` pricing table, **so that** the risk documented in ADR-002 has an owner and doesn't get lost.

**Acceptance Criteria**: not an AC-numbered requirement — this is process/documentation work implied by `research/pitfalls.md` §6's explicit "needs an explicit in-scope-vs-fast-follow decision... rather than being silently left unaddressed."
- *Given* `ADR-002-capacity-monitor-pricing-scope-deferred.md` exists, *When* this story completes, *Then* a backlog item or logged bug exists referencing `server/services/capacity_monitor.go:351-371`, `config.CostBudgetUSD`, and ADR-002 by path.
- *Given* the fast-follow bug is filed, *When* this project's own PR is opened (Epic 4.2/Phase 7 of the SDD workflow), *Then* the PR description includes the filed bug's link/ID under an explicit "Known related issue (not fixed here)" heading — so ADR-002's documented live risk (a real automated pause/stop trigger mis-pricing Claude models today) isn't missed by a reviewer who doesn't separately read the ADR. Per `adversarial-review.md`'s Concerns: ADR-002 argues this bug is worse than the one being fixed, so its visibility can't depend on a reviewer stumbling onto the ADR file.

**Files**: none (no code touched — this is a `pm:log-bug` / backlog-item action plus a PR-description requirement, not a file edit)

##### Task 3.1.1a: File the fast-follow bug (~3 min)
- Use `pm:log-bug` (or create a new backlog item directly) summarizing: "`CapacityMonitor.estimateCost` (`server/services/capacity_monitor.go:351-371`) silently mis-prices any Claude model not matching `opus`/`haiku`/`flash`/`pro` substrings at a generic 'sonnet default' rate, feeding the live `CostBudgetUSD`/`cost_budget_exceeded` autonomous-session-pause trigger. See `project_plans/insights-cost-pricing-gaps/decisions/ADR-002-capacity-monitor-pricing-scope-deferred.md` for full context and the two remediation options considered." No code change.
- Files: none

##### Task 3.1.1b: Reference the filed bug in this project's PR description (~1 min)
- When opening the PR for this project (Epic 4.2 / `sdd:7-ship`), add a "Known related issue (not fixed here)" heading to the PR description, pasting Task 3.1.1a's filed bug/backlog item **ID verbatim** (not just a hyperlink — the raw ID string, so it survives a future grep even if the link rots) plus one sentence summarizing the risk (substring-matched pricing table can mis-price a Claude model at the wrong nonzero rate and drive a live autonomous-session pause). This makes ADR-002's deferred risk visible to a reviewer without requiring them to separately open the ADR file. See Task 4.2.1a, which gates on this.
- Files: none (PR description only)

### Epic 3.2: File the `BacklogService.buildCostLookup` fast-follow

**Goal**: Record and hand off the out-of-scope Backlog cost-lookup gap identified in `ADR-003-backlog-cost-lookup-scope-deferred.md`, so it isn't silently forgotten once this project ships — mirroring Epic 3.1's treatment of ADR-002, per architecture-review.md's re-review concern that ADR-003 had no analogous filing/visibility task.

#### Story 3.2.1: File a fast-follow bug for `BacklogService.buildCostLookup`
**As a** future maintainer, **I want** a tracked backlog item for the Backlog UI's unpriced-silently-`$0` cost display, **so that** the risk documented in ADR-003 has an owner and doesn't get lost.

**Acceptance Criteria**: not an AC-numbered requirement — process/documentation work, same shape as Story 3.1.1.
- *Given* `ADR-003-backlog-cost-lookup-scope-deferred.md` exists, *When* this story completes, *Then* a backlog item or logged bug exists referencing `server/services/backlog_service.go:399-429`, `server/services/backlog_service_query.go:455-477`, and ADR-003 by path.
- *Given* the fast-follow bug is filed, *When* this project's own PR is opened (Epic 4.2/Phase 7), *Then* the PR description includes the filed bug's link/ID under the same "Known related issue (not fixed here)" heading used for Task 3.1.1b (both entries under one heading is fine), so a reviewer sees both deferred pricing gaps without reading either ADR separately.

**Files**: none (no code touched)

##### Task 3.2.1a: File the fast-follow bug (~3 min)
- Use `pm:log-bug` (or create a new backlog item directly) summarizing: "`BacklogService.buildCostLookup` (`server/services/backlog_service.go:399-429`) and its cost query (`server/services/backlog_service_query.go:455-477`) silently render `$0.00` for any unpriced model family (e.g. a Claude model missing from `DefaultPricingTable()`), identical to the bug this project fixes for the Insights page — but the Backlog UI's per-item cost display was left out of this project's scope. See `project_plans/insights-cost-pricing-gaps/decisions/ADR-003-backlog-cost-lookup-scope-deferred.md` for full context." No code change.
- Files: none

##### Task 3.2.1b: Reference the filed bug in this project's PR description (~1 min)
- Add Task 3.2.1a's bug/backlog item **ID verbatim** (not just a hyperlink) to the same "Known related issue (not fixed here)" heading Task 3.1.1b adds, as a second bullet, with one sentence summarizing the risk (Backlog per-item cost still silently reads `$0.00` for unpriced Claude models after this PR ships). See Task 4.2.1a, which gates on this.
- Files: none (PR description only)

---

## Phase 4: Registry & Ship Gate

### Epic 4.1: AC-6 — Registry updates
**Goal**: The feature registry reflects the new pricing-completeness test coverage and the new UI indicator, per `.claude/rules/feature-registry.md`.

#### Story 4.1.1: Update existing backend registry entries
**As a** repo maintainer, **I want** `GetInsightsSummary`/`ListSessionTokens`'s registry entries to reflect their new test coverage, **so that** `coverage-gaps.json` doesn't grow.

**Acceptance Criteria** (AC-6):
- *Given* Story 1.3.3g added `TestGetInsightsSummary_WhenUnpricedModelFamily_ExpectPricingUnavailableFlagged` and `TestListSessionTokens_WhenUnpricedModelFamily_ExpectUnpricedModelsPopulated`, *When* `docs/registry/features/backend/GetInsightsSummary.json` and `ListSessionTokens.json` are updated, *Then* both have `"tested": true` with the new test names appended to `"testIds"` and `"lastModified"` set to the implementation date.

**Files**: `docs/registry/features/backend/GetInsightsSummary.json`, `docs/registry/features/backend/ListSessionTokens.json`

##### Task 4.1.1a: Update `GetInsightsSummary.json` (~2 min)
- Before editing the JSON, `grep -n "func TestGetInsightsSummary_WhenUnpricedModelFamily_ExpectPricingUnavailableFlagged" server/services/insights_service_test.go` to confirm the exact test function name actually landed as written (catches a rename or typo introduced during Task 1.3.3g's implementation) — paste the grep-confirmed name, not the name as originally planned, if they differ (per pre-mortem Failure #5, P3).
- Set `"tested": true`, append the grep-confirmed test name to `"testIds"`, update `"lastModified"`.
- Files: `docs/registry/features/backend/GetInsightsSummary.json`

##### Task 4.1.1b: Update `ListSessionTokens.json` (~2 min)
- Before editing the JSON, `grep -n "func TestListSessionTokens_WhenUnpricedModelFamily_ExpectUnpricedModelsPopulated" server/services/insights_service_test.go` to confirm the exact test function name actually landed as written — paste the grep-confirmed name, not the name as originally planned, if they differ (per pre-mortem Failure #5, P3).
- Set `"tested": true`, append the grep-confirmed test name to `"testIds"`, update `"lastModified"`.
- Files: `docs/registry/features/backend/ListSessionTokens.json`

#### Story 4.1.2: Add a new frontend registry entry for the pricing-unavailable indicator
**As a** repo maintainer, **I want** the new UI indicator tracked as its own frontend feature, **so that** it's not invisible to the registry.

**Acceptance Criteria** (AC-6):
- *Given* `ModelBreakdownChart.tsx` gains a `// +feature: insights-pricing-unavailable-indicator` marker (Task 4.1.2a), *When* `docs/registry/features/frontend/insights-pricing-unavailable-indicator.json` is added, *Then* it references `web-app/src/app/insights/ModelBreakdownChart.tsx`, has `"tested": true`, and lists Task 2.1.1d's Jest test name in `"testIds"`.

**Files**: `web-app/src/app/insights/ModelBreakdownChart.tsx`, `docs/registry/features/frontend/insights-pricing-unavailable-indicator.json`

##### Task 4.1.2a: Add the `// +feature:` marker (~2 min)
- `ModelBreakdownChart.tsx` already has `// +feature: insights-dashboard` at line 1. Add a second marker comment directly above the legend-rendering block added in Task 2.1.1b: `// +feature: insights-pricing-unavailable-indicator`.
- Files: `web-app/src/app/insights/ModelBreakdownChart.tsx`

##### Task 4.1.2b: Create the frontend registry entry (~3 min)
- Before creating the JSON, `grep -n "ModelBreakdownChart_should_showUnpricedSuffix_When_pricingUnavailable" web-app/src/app/insights/ModelBreakdownChart.test.tsx` to confirm the exact test name actually landed as written (catches a rename or typo introduced during Task 2.1.1d's implementation) — use the grep-confirmed `describe`/`it` name, not the name as originally planned, if they differ (per pre-mortem Failure #5, P3).
- Create `docs/registry/features/frontend/insights-pricing-unavailable-indicator.json`:
  ```json
  {
    "id": "insights-pricing-unavailable-indicator",
    "type": "frontend",
    "component": "ModelBreakdownChart",
    "path": "web-app/src/app/insights/ModelBreakdownChart.tsx",
    "markerLine": <line number from Task 4.1.2a>,
    "tested": true,
    "testIds": ["ModelBreakdownChart > ModelBreakdownChart_should_showUnpricedSuffix_When_pricingUnavailable"],
    "lastModified": "<implementation date>T00:00:00.000Z"
  }
  ```
- Files: `docs/registry/features/frontend/insights-pricing-unavailable-indicator.json`

##### Task 4.1.2c: Regenerate and verify no coverage-gap growth (~3 min)
- Run `make registry-generate`. Confirm `docs/registry/coverage-gaps.json`'s entry count does not increase relative to `main` (it should decrease, since two backend entries move from untested to tested and one new frontend entry is added pre-tested).
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json` (all generated)

---

### Epic 4.2: AC-7 — Final validation gate

#### Story 4.2.1: Run `make quick-check`
**As a** developer shipping this change, **I want** the full quick-check gate green, **so that** the PR meets this repo's bar before review.

**Acceptance Criteria** (AC-7):
- *Given* every task above is complete, *When* `make quick-check` is run, *Then* it exits `0` — `go build .` and `$(BIN_TMUX)` succeed, `go test ./...` (via `test-coverage`) and `test-race` pass including every new test added in Stories 1.1.1, 1.2.1, 1.3.1, 1.3.3, 1.4.1, 1.5.1, 1.5.2, and every new Jest test added in Stories 2.1.1, 2.2.1, 2.3.1, 2.4.1, 2.4.2, 2.4.3, `lint` and `lint-css-tokens` report no new issues, and `registry-diff` shows no drift against the committed registry files updated in Epic 4.1.

**Files**: none (validation only)

##### Task 4.2.1a: Run `make quick-check` and fix any fallout (~5 min, may repeat)
- Run `make quick-check`. If it fails, diagnose against the specific failing sub-target (`build`, `test-coverage`, `test-race`, `lint`, `lint-css-tokens`, `registry-diff`) and fix in the relevant file from the task above that owns that area — do not patch around a failure without identifying which prior task introduced it.
- **Registry truthfulness cross-check** (per pre-mortem Failure #5, P3): in addition to `registry-diff`'s structural check, `grep` every `testIds` entry touched by Tasks 4.1.1a/4.1.1b/4.1.2b against the actual test files (`server/services/insights_service_test.go`, `web-app/src/app/insights/ModelBreakdownChart.test.tsx`) to confirm each named test function/case still exists verbatim on disk — `registry-diff`/`make registry-generate` are structurally self-consistent but don't verify a claimed test name corresponds to a real test, so this is a separate, explicit check.
- **Before this task's PR is opened** (per pre-mortem Failure #4, P2): confirm Tasks 3.1.1a and 3.2.1a's two fast-follow backlog-item IDs exist, and paste both IDs **verbatim** into the PR description under the "Known related issue (not fixed here)" heading (Tasks 3.1.1b/3.2.1b) — not merely "referenced" or described in prose. A reviewer or a future grep of the PR description must be able to find the literal backlog-item ID string for both ADR-002's and ADR-003's deferred bugs; a link alone without the raw ID text is not sufficient, since links rot and PR descriptions are the only durable trace once the PR merges and scrolls out of view.
- This is the final task in the plan. Do not consider the project complete until this task's `make quick-check` run is green with output shown, per this repo's "no completion claim without proof" engineering discipline rule.
- Files: whichever file(s) a failure traces back to
