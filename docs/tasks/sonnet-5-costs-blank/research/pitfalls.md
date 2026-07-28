# Pitfalls — "Sonnet 5 costs blank"

Research for the fix to `session/tokens/pricing.go`'s hardcoded pricing table missing
generation-5 Claude models. All line numbers reference the current tree.

## 1. Regex trace — does the variant-suffix regex swallow gen-5 into gen-4 pricing?

Relevant patterns, `session/tokens/pricing.go:11-19`:

```go
var dateSuffixPattern = regexp.MustCompile(`-\d{8}$`)
var variantSuffixPattern = regexp.MustCompile(`^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$`)
var legacyModelPattern = regexp.MustCompile(`^claude-(\d+)-(\w+)(?:-\d{8})?$`)
```

`NormalizeModelFamily` (`pricing.go:115-136`) applies them in order: strip date → try
legacy pattern → try variant-suffix pattern → else return as-is.

Traced by hand against three plausible raw IDs:

| Input | After date-strip | legacyModelPattern | variantSuffixPattern | Final family | Verdict |
|---|---|---|---|---|---|
| `claude-sonnet-4-5-20250929` (real, already-shipped Sonnet 4.5) | `claude-sonnet-4-5` | no match (`sonnet` isn't `\d+` right after `claude-`) | matches: group1=`claude-sonnet-4`, trailing `-5` treated as variant | `claude-sonnet-4` | Collapses into gen-4 pricing — **intentional**, matches the docstring's own worked example (`claude-sonnet-4-6 → claude-sonnet-4`) |
| `claude-sonnet-4-5` (no date) | `claude-sonnet-4-5` | no match | matches, same as above | `claude-sonnet-4` | Same collapse |
| `claude-sonnet-5-20260201` (hypothetical gen-5, no point release) | `claude-sonnet-5` | no match | **no match** — pattern requires two trailing digit groups (`-\d+-\d+$`); only one digit segment (`5`) exists, nothing left after `\d+` consumes it | `claude-sonnet-5` (unchanged) | Falls through unmapped → `EstimateCost`/`ModelFamilyCost` skip it → **silent $0**, matches the reported bug |
| `claude-sonnet-5-1-20260301` (hypothetical Sonnet 5.1 point release) | `claude-sonnet-5-1` | no match | matches: group1=`claude-sonnet-5` | `claude-sonnet-5` | Also unmapped today → silent $0, but *correctly* funnels to the same key a fix would add |

**Key finding — landmine for the implementer, not a live bug today:** the variant
regex fires on *any* `claude-<role>-N-M` shape, regardless of whether `N-M` is a true
minor/point release of generation `N` or is semantically a new generation encoded with
a hyphen (e.g. if Anthropic ever ships a real "generation 4.5" that should be priced
differently from generation 4 proper — `claude-sonnet-4-5` already exists in the wild
per `web-app/src/lib/constants/programs.ts:32` as "Claude Sonnet 4.5", and the pricing
table today has no entry for it — it silently gets priced as `claude-sonnet-4` at
$3/$15 per MTok). If the fix adds a differently-priced `"claude-sonnet-4-5"` key
directly to `pt.Prices`, **that key is unreachable** — normalization always collapses
`claude-sonnet-4-5*` down to `claude-sonnet-4` before the map lookup happens, so the
new entry would silently never match. Any gen-5-specific pricing key added must be
one of the forms that survive normalization unchanged or predictably (`claude-sonnet-5`,
`claude-opus-5`, `claude-haiku-5` — bare, no trailing `-N` — since that's what a raw ID
like `claude-sonnet-5-20260315` normalizes to). Verify the *actual* production model ID
Anthropic ships before hardcoding the key; don't assume the `-N-M` convention observed
in prior generations holds.

No case was found where the regex accidentally produces a **wrong-but-plausible**
price for a genuine gen-5 ID today — the two failure modes are "collapses into gen-4
(existing, arguably-intended behavior for 4.x point releases)" and "falls through
unmapped → $0 (the reported bug)". The risk is prospective: get the new map key wrong
and you reintroduce a silent-$0 or silent-wrong-price bug for gen 5 too.

## 2. `IsStale()` — dead code, second silent-failure mode

`PricingTable.IsStale()` (`pricing.go:185-200`) checks whether any `EffectiveDate` is
>30 days old. Repo-wide grep for `IsStale` turns up exactly two hits outside its own
file: both are its own unit tests
(`session/tokens/pricing_test.go:80-106` — `TestPricingTable_WhenIsStale_Expect31DaysReturnTrue`
/ `...Expect29DaysReturnFalse`). It is **never called** from `server/dependencies.go`,
`server/services/insights_service.go`, `server/services/backlog_service.go`, or any
web-app component (grepped `.go`/`.ts`/`.tsx` repo-wide, zero non-test call sites).

All hardcoded `EffectiveDate` values in `DefaultPricingTable()` are `"2026-05-15"`
(`pricing.go:33,41,49,57,65,73`). As of the current date (2026-07-25) that's already
~71 days old — `IsStale()` would return `true` right now if anything called it. Since
nothing does, there is no UI badge, log warning, or metric that would have caught
*either* the missing gen-5 entries *or* the fact that the existing gen-3/4 numbers are
themselves past their own self-declared staleness threshold. This is a second,
independent silent-failure mode from the one in the bug report — worth flagging even
if out of scope for the minimal fix, since fixing gen-5 pricing without wiring
`IsStale()` (or equivalent) anywhere just resets the clock on the same class of bug.

## 3. Blast radius — every caller of pricing, not just Insights

Repo-wide grep for `EstimateCost|ModelFamilyCost|LookupByModel|NormalizeModelFamily`
(non-test):

- `server/services/insights_service.go:92,115,179,184,192,194,314` — Insights dashboard
  (the reported bug surface): per-day cost buckets, model filter, cost breakdown.
- `server/services/backlog_service.go:400-403,408-431` — `SetTokenStore` wires
  `tokenStore`/`pricing` onto `BacklogService`; `buildCostLookup()` (`:408-431`) calls
  `pt.EstimateCost(r)` (`:429`) to resolve a **per-session USD cost** used somewhere in
  backlog session listings/cost display. Same blank-for-gen-5 exposure as Insights,
  independently reachable.
- `server/services/backlog_service_query.go:471` — `GetBacklogItemCost` RPC handler
  (`:445-483`) calls `s.pricing.EstimateCost(result)` per linked session and sums into
  `resp.TotalCostUsd` / per-session `EstimatedCostUsd` — this is the backlog item
  detail page's cost rollup, a **third independent UI surface** that would show wrong
  (understated/zero) totals for gen-5 sessions, not just Insights.
- `server/dependencies.go:1070,1076` — the only production wiring site:
  `pricing := tokens.DefaultPricingTable()` then passed to both
  `services.NewInsightsService(tokenStore, pricing, associator)` (`:1074`) and
  `backlogSvc.SetTokenStore(tokenStore, pricing)` (`:1076`). **One shared
  `*PricingTable` instance feeds both Insights and Backlog** — fixing
  `DefaultPricingTable()` fixes all three call sites at once, but any test/verification
  pass must check backlog cost displays (item detail rollup + per-session lookup), not
  just the Insights dashboard, or the fix could look complete while two of three
  surfaces are unverified.
- `LoadPricingOverride` (`pricing.go:82-102`) is dead code confirmed: only reference
  outside its own definition and `pricing_test.go:108-137`'s
  `TestLoadPricingOverride_WhenValidConfigJSON_ExpectOverridesApplied` is nothing —
  `server/dependencies.go` calls `DefaultPricingTable()` directly, never
  `LoadPricingOverride()`. There's no config file, flag, or env var that would let an
  operator patch in gen-5 pricing without a code change even though the mechanism
  exists and is tested in isolation.
- Frontend note, not a pricing-table bug but same bug class: `MODEL_COLORS` in
  `web-app/src/app/insights/ModelOverTimeChart.tsx:26-33` hardcodes swatch colors for
  the same six gen-3/gen-4 families and has no gen-5 entries. Unlike the Go pricing
  gap this one degrades gracefully — `colorForModel()` (`:37-39`) falls back to a
  rotating palette (`FALLBACK_COLORS`, `:35`) for unknown families — so it won't be
  blank, just inconsistent from a design/branding standpoint. Worth a one-line fix in
  the same PR if gen-5 chart series get new keys added, but not a functional blocker.

## 4. Test coverage gaps in `pricing_test.go`

Full file read (`session/tokens/pricing_test.go`, 138 lines). Covered:

- `NormalizeModelFamily` table test (`:13-32`) — 6 cases, all gen-3/gen-4 shapes
  (`claude-sonnet-4-6[-date]`, `claude-opus-4-7`, `claude-3-opus-date` legacy,
  `claude-haiku-4` bare, `unknown-model-xyz` passthrough). **No case for a bare
  `claude-<role>-5` (no point-release suffix) or a `claude-<role>-5-N` shape** — i.e.
  no test asserts what generation-5 IDs normalize to today, which is exactly the
  ambiguity in §1.
- `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` (`:51-63`) — proves the
  silent-$0 behavior is by-design and already asserted for a made-up model
  (`gpt-99-turbo`), but the test model is obviously non-Claude and not date/variant
  shaped, so it doesn't exercise the normalization path a real unmapped Claude
  generation would take (i.e. it never proves that an *unmapped-but-Claude-shaped*
  family, post-normalization, still silently returns 0 — a subtly different code path
  since `gpt-99-turbo` never matches any of the three regexes and returns unchanged).
- No test exercises `EstimateCost`/`ModelFamilyCost`/`LookupByModel` returning `false`/
  0 **and asserts that this is surfaced anywhere** (log, error, metric) — because
  today it isn't; the functions are documented to silently return zero
  (`pricing.go:139`, `:239-240`), and tests just confirm that contract rather than
  challenge it.
- `IsStale()` tests (`:80-106`) cover the 30-day boundary correctly (31 days true, 29
  days false) but nothing tests that `IsStale()` is *called* by anything in production
  — consistent with §2's dead-code finding.
- `LoadPricingOverride` test (`:108-137`) proves the merge-over-defaults mechanism
  works in isolation but doesn't touch wiring, consistent with §3's dead-code finding.
- No boundary test for the variant-suffix regex's double-digit-group requirement
  (e.g. nothing asserts `claude-sonnet-5` alone is left unmapped while
  `claude-sonnet-5-1` normalizes to `claude-sonnet-5`) — the exact distinction that
  matters for picking the right map key in the fix (§1).

## 5. Deployment/ops risk

Pricing is compiled into the binary (`DefaultPricingTable()`, `pricing.go:23-77`) with
no live config path wired (`LoadPricingOverride` is dead per §3). Every future Claude
model release that needs new pricing — including whatever "Sonnet 5" actually is —
requires a source change to `pricing.go`, a rebuild, and a redeploy
(`make install-service`) before Insights/Backlog cost numbers become accurate again.
There's no fallback, warning, or override surface an operator can use in the interim;
the gap persists silently (per the bug report) until someone notices the dashboard
looks wrong, files a bug, and ships a code fix. `LoadPricingOverride` already exists
and is tested — wiring it up (e.g. an optional JSON file path via config/env) would
close this gap for future generations without requiring the redeploy-per-release
cycle, though that's a scope decision beyond the minimal "add gen-5 entries" fix.

One-line cross-reference: if the fix's verification/rollout path includes running
`make install-service` against the live dev instance, note per
`.claude/rules/tmux-keep-server-on-restart.md` that this restarts the tmux server and
kills every live tmux session unless the deployed unit passes `--tmux-keep-server` —
unrelated to the pricing bug itself, but a real risk if this fix is deployed from a
terminal running inside one of those sessions.
