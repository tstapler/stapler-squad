# Architecture: Fix "Sonnet 5 costs blank" in Insights

## 0. Confirmed facts from reading the code

- `session/tokens/pricing.go:16` — `variantSuffixPattern` = `^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$`.
  This strips **any** trailing `-N` after `claude-<family>-<digit>`, with no upper bound on
  N. So `claude-sonnet-4-5-20250929` → (date stripped) `claude-sonnet-4-5` → variant regex
  matches → collapses to `claude-sonnet-4`. Confirmed by the existing test case
  `pricing_test.go:18`: `"claude-sonnet-4-6" → "claude-sonnet-4"`. The regex was written
  entirely for the "point-release" reading of the suffix.
- `DefaultPricingTable()` (`pricing.go:23-77`) hardcodes exactly 6 entries: opus/sonnet/haiku
  × generation 4 and 3. No generation-5 entries exist under any key.
- `EstimateCost`/`ModelFamilyCost` (`pricing.go:140-237`) both do `pricing, ok :=
  pt.Prices[family]; if !ok { continue }` — a table miss is silently dropped, contributing
  `0.0` to the total with no signal anywhere in the return value.
- `LoadPricingOverride(configPath string)` (`pricing.go:82-102`) exists, is tested
  (`pricing_test.go:108-137`), but has **zero callers** outside its own test —
  `server/dependencies.go:1070` calls `tokens.DefaultPricingTable()` directly, and nothing
  in the codebase ever calls `LoadPricingOverride`.
- `PricingTable.ConfigPath` (`types.go:80`) is a struct field that only `LoadPricingOverride`
  ever sets — it's otherwise dead.
- `PricingTable.IsStale()` (`pricing.go:185-200`) is also currently uncalled outside its own
  tests — `PricingAsOf` is sent to the frontend (`insights_service.go:273`,
  `insights.proto:103`) but nothing renders staleness or reads `IsStale()`.
- `SessionTokenSummary` (`insights.proto:23-40`) and `DailyTokenBucket`/`ModelBreakdown`
  (`insights.proto:50-71`) have no "pricing missing" boolean or count anywhere in the proto.
- Frontend: `SummaryCards.tsx:12-51` already has a precedent for a conditionally-rendered
  "problem" card — the `orphanCount > 0` block (lines 42-48) that only renders when
  `summary.sessions.filter(s => s.isOrphan).length > 0`. `insightsFormatters.ts` has no cost
  formatting for "unknown"/`null` — `fmtCost(usd: number)` (line 5) assumes a real number.

## 1. Component boundaries

| Component | Change |
|---|---|
| `session/tokens/pricing.go` | Add generation-5 entries to `DefaultPricingTable()`; fix `NormalizeModelFamily` regex ambiguity (§3); add an `UnpricedFamilies(r *ParseResult) []string`-style helper OR change `EstimateCost`/`ModelFamilyCost` call sites to also report misses (see §1a). |
| `server/dependencies.go:1070` | **No change recommended** (see §2) — keep calling `DefaultPricingTable()` directly. Do not wire `LoadPricingOverride` in this fix; see justification below. |
| `server/services/insights_service.go` | Track which sessions/days hit a pricing-table miss while building `sessions`/`daily`/`models` (the loops at lines 100-230 already normalize per-turn); surface a count via a new proto field. |
| `proto/session/v1/insights.proto` | Add `int32 sessions_with_unpriced_models = 13;` (or similar) to `GetInsightsSummaryResponse`, and/or `bool has_unpriced_usage = N;` to `SessionTokenSummary`, so the frontend can render a real signal instead of inferring from `estimated_cost_usd == 0`. |
| `web-app/src/app/insights/SummaryCards.tsx` | Add a card mirroring the existing `orphanCount > 0` pattern (lines 42-48): render only when the new field is > 0. |
| `web-app/src/app/insights/insightsFormatters.ts` | No change strictly required; optionally a `fmtCost` variant is unnecessary — the new signal is a count/flag, not a cost format. |

### 1a. Why "surface a miss" belongs in the service layer, not just the pricing package

`pricing.go`'s `EstimateCost`/`ModelFamilyCost` are pure functions over `*ParseResult` →
`float64`/`map[string]float64`. Changing their signatures to return an error or a "missing
models" list would ripple into every call site (`insights_service.go:115,179,192`,
`server/services/backlog_service*.go` via `SetTokenStore`, per `dependencies.go:1076`) for a
problem that's fundamentally about *display*, not computation. The minimal, YAGNI-consistent
change is a **new, narrow read-only helper** on `*PricingTable`:

```go
// UnpricedModels returns the set of normalized model families in r.TurnTimeline
// that have no entry in the pricing table (i.e. would silently cost $0).
func (pt *PricingTable) UnpricedModels(r *ParseResult) []string
```

`insights_service.go` calls this once per session inside the existing loop (next to the
`costUSD := s.pricing.EstimateCost(r)` call at line 115) and sets a bool/count on the
response — no change to the hot-path cost functions themselves.

## 2. Hardcoded table vs. external config — recommendation: **keep hardcoded, do not wire `LoadPricingOverride`**

Arguments considered:

- **For wiring the JSON override now:** it's already written and tested
  (`pricing_test.go:108-137`), so "just call it" looks cheap, and it matches the user's
  "look up prices using tokens" framing (config-driven).
- **Against, and decisive:** wiring it means inventing a config path convention (env var,
  `config/` field, default file location), documenting it, and — critically — it does
  **not** fix the reported bug on its own. Anthropic doesn't publish a machine-readable
  pricing feed; an external JSON file still has to be hand-maintained by *someone*, and if
  nobody populates it with `claude-sonnet-5` on day one, costs are still blank. The
  `DefaultPricingTable()` hardcoded-map pattern is what the previous four model families
  (opus/sonnet/haiku × gen 4, gen 3) already use, and it is trivially cheap to add three more
  map entries (~15 lines) when a new generation ships — no new abstraction needed for a
  problem that occurs a few times a year.
- Wiring `LoadPricingOverride` is real scope creep relative to "Sonnet 5 costs blank": it adds
  an operational surface (a file someone must edit and keep in sync, a load-failure mode to
  handle, a place for it to go stale) to fix a problem that a 15-line map addition already
  solves. Per repo's ponytail/YAGNI discipline, this is exactly the kind of "flexible for a
  need that hasn't materialized" work to defer.
- Recommendation: **add generation-5 entries to `DefaultPricingTable()` now** (the fix for
  *this* bug), and **leave `LoadPricingOverride`/`ConfigPath` as unwired, already-tested dead
  code** — it's a reasonable escape hatch to wire up later *if* a real need for
  runtime-editable pricing shows up (e.g. self-hosted models, custom negotiated rates), but
  that's a separate feature with its own design questions (who edits the file, validation,
  hot-reload vs. restart) that don't need answering to fix this bug. Do not touch
  `server/dependencies.go:1070`.
- The *actual* durable fix for "next new model generation shows blank costs again" is §1a's
  `UnpricedModels` surfacing — that turns a silent $0 into a visible signal (an operator
  notices instead of not), which is more valuable than a config file nobody remembers to
  update.

## 3. `NormalizeModelFamily` regex fix

The ambiguity is real: Anthropic's actual public model ID for "Sonnet 4.5" is
`claude-sonnet-4-5-20250929` — a **point release of generation 4**, not a new generation 5.
(There is no confirmed public `claude-sonnet-5-*` ID as of this writing; the bug report's
framing of "Sonnet 5" is examined below.) Given the repo's own `variantSuffixPattern` doc
comment (`pricing.go:13-15`: "minor variant numbers like `-6` or `-7`... claude-sonnet-4-6 →
claude-sonnet-4") the codebase's existing convention already treats `-4-5`, `-4-6`, `-4-7` as
variants of generation 4, not new generations — and the test suite locks that in
(`pricing_test.go:18-21`).

**Two independent gaps, both minimal, both regex/table-only (no new abstraction):**

1. **If "Sonnet 5" really means a new top-level generation** (`claude-sonnet-5[-N][-date]`,
   analogous to how `claude-sonnet-4` and `claude-sonnet-3` are distinct top-level entries
   today), then `variantSuffixPattern` already handles it correctly as-is —
   `claude-sonnet-5-6-20250101` → strips date → `claude-sonnet-5-6` → variant regex captures
   group `claude-sonnet-5` → looks up `pt.Prices["claude-sonnet-5"]`. The **only** missing
   piece is the table entry itself: add `"claude-opus-5"`, `"claude-sonnet-5"`,
   `"claude-haiku-5"` to `DefaultPricingTable()` (§2). No regex change needed for this case —
   `^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$` already generalizes over the generation digit.

2. **If "Sonnet 5" in the bug title is actually shorthand for "Sonnet 4.5"**
   (`claude-sonnet-4-5-*`), the existing regex already collapses it to `claude-sonnet-4`
   (confirmed by the `claude-sonnet-4-6` test case using the same shape), so it would price
   correctly **today** against the existing `claude-sonnet-4` entry — meaning this specific
   sub-case may not need a code change at all, only verification via a new test case
   `{"claude-sonnet-4-5-20250929", "claude-sonnet-4"}` added to
   `TestNormalizeModelFamily_WhenDateSuffixedID_ExpectStripped` (`pricing_test.go:13-32`) to
   lock in the behavior and catch regressions.

**Recommendation:** implement both — add the gen-5 table entries (covers case 1, the literal
bug title) AND add the `claude-sonnet-4-5` regression test case (covers case 2, guards the
existing collapse-to-4 behavior). This is a 2-line table addition set + 1 test case, no regex
edit required, because `variantSuffixPattern`'s `\d+` group already generalizes over the
generation number. **Flag for the implementer:** before writing the fix, check what raw
`model` string is actually appearing in a real `~/.claude/projects/*.jsonl` transcript for
the reported session (grep the JSONL for `"model"`) — do not guess between case 1 and case 2;
the true fix depends on which literal string Anthropic's API is emitting today.

## 4. Where the "no pricing data" indicator surfaces in the UI

`web-app/src/app/insights/SummaryCards.tsx:42-48` is the existing pattern to mirror — a
conditionally-rendered card, same visual treatment as "Orphaned":

```tsx
{unpricedCount > 0 && (
  <div className={card}>
    <span className={cardLabel}>Unpriced Usage</span>
    <span className={cardValue}>{unpricedCount}</span>
    <span className={cardSub}>sessions with unknown model pricing</span>
  </div>
)}
```

driven by a new field on `GetInsightsSummaryResponse` (§1, proto change) rather than by
inferring "cost == 0" client-side (a session can legitimately have $0 cost, e.g. an
all-cache-read no-op turn, so `estimated_cost_usd === 0` is not a safe proxy for "unpriced").
No new component file needed — this is a 6-line addition to the existing `grid` in
`SummaryCards.tsx`, consistent with the file's existing structure. `insightsFormatters.ts`
needs no new function; the value is a plain count, already covered by existing JSX.

Secondary (optional, skip for first pass per YAGNI): per-session indicator in
`SessionsTable.tsx` next to `primary_model` — only worth it if the aggregate card in
`SummaryCards.tsx` proves insufficient for triage; not needed to close this bug.

## 5. End-to-end data flow (fixed state)

```
JSONL transcript: "model": "claude-sonnet-4-5-20250929"  (or "claude-sonnet-5-...")
  → tokens.TokenStore parses into ParseResult.TurnTimeline[i].Model
  → tokens.NormalizeModelFamily(turn.Model)              [pricing.go:115]
      strips date suffix → applies variantSuffixPattern → "claude-sonnet-4" or "claude-sonnet-5"
  → pt.Prices[family] lookup in EstimateCost/ModelFamilyCost [pricing.go:170,212]
      HIT (after §2/§3 fix): real USD cost accumulated
      MISS (any future unmapped model): pt.UnpricedModels(r) records the family [new, §1a]
  → InsightsService.GetInsightsSummary                    [insights_service.go:41]
      costUSD, plus unpriced flag/count, attached to SessionTokenSummary / response totals
  → sessionv1.GetInsightsSummaryResponse (proto)           [insights.proto:91-104]
  → web-app fetch → SummaryCards.tsx renders totalCostUsd (now non-zero)
      + new "Unpriced Usage" card when the count is > 0    [SummaryCards.tsx, §4]
```

## Open question for implementer (not resolved by this research doc)

Confirm the literal raw `model` string Anthropic emits for the session that triggered this
bug report before writing the `NormalizeModelFamily`/table fix — §3 covers both plausible
readings ("Sonnet 4.5" vs. a true "Sonnet 5" generation), but only one is real, and the fix
differs (table-entries-only vs. table-entries-plus-regression-test).
