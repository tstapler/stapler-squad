# Build vs. Buy — `insights-cost-pricing-gaps`

Research agent: Agent 6 (Build vs. Buy), SDD Phase 2.

Scope: evaluate whether any existing library, official Anthropic feed, or
in-repo pattern should replace/supplement the hand-maintained
`DefaultPricingTable()` in `session/tokens/pricing.go`, consistent with the
requirements doc's Non-Goal ("static table + override is sufficient" unless
research finds a live integration is trivially cheap).

Current state (confirmed by reading `session/tokens/pricing.go`):

- `DefaultPricingTable()` is a hardcoded `map[string]ModelPricing` keyed by
  normalized model family (`claude-opus-4`, `claude-sonnet-4`, ...), dated
  "as of 2026-05-15". `ModelPricing` carries `InputPricePerMTok`,
  `OutputPricePerMTok`, `CacheWritePerMTok`, `CacheReadPerMTok`,
  `EffectiveDate`.
- `LoadPricingOverride(configPath string)` already exists: reads a JSON file
  of `map[string]ModelPricing`, merges over the defaults. It is **not called
  anywhere** — `server/dependencies.go:1070` only calls
  `tokens.DefaultPricingTable()`.
- `IsStale()` already flags entries whose `EffectiveDate` is >30 days old —
  an existing, unused staleness signal worth wiring into G4's guardrail.

---

## 1. Existing OSS library for Claude/LLM pricing lookups

### Option: `litellm`'s `model_prices_and_context_window.json` (BerriAI)

- **What it is**: a single static JSON file
  (`github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json`)
  that is the pricing/context-window source of truth for litellm, covering
  100+ providers including every current Anthropic model family. Each entry
  carries `input_cost_per_token`, `output_cost_per_token`,
  `cache_creation_input_token_cost`, `cache_read_input_token_cost` (all
  **per-token**, not per-MTok — needs a ×1,000,000 conversion to match this
  repo's `PerMTok` fields), plus `max_input_tokens`, `mode`, capability
  flags.
- **Maturity / freshness**: actively maintained, high commit velocity,
  litellm explicitly documents day-0 model sync
  (`docs/proxy/sync_models_github.md`) — new model launches (e.g. a new
  Claude family) typically land in this file within hours to a couple of
  days of an Anthropic announcement, driven by GitHub issues/community PRs
  rather than an Anthropic-side feed.
- **License**: MIT (litellm project). The JSON file itself is plain data,
  trivially fetchable/parseable without importing any Python code — no
  license friction for a Go consumer.
- **Go port**: no maintained official Go port of litellm's cost engine
  found. The JSON file is language-agnostic, so "porting" only means
  writing a small Go struct + fetch/parse step — there's nothing to import,
  only a URL to fetch and a schema to map.
- **Integration cost if adopted as the `LoadPricingOverride` source**:
  - Fetch `https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json` (~1-2MB, thousands of entries for all providers) — would need to be filtered down to `claude-*` keys.
  - Map litellm's key scheme (`claude-3-5-sonnet-20241022`,
    `claude-opus-4-1`, etc. — exact model IDs, not normalized families) to
    this repo's normalized-family keys (`NormalizeModelFamily()` output).
    Requires either normalizing litellm's keys the same way, or keeping a
    small alias table.
  - Convert per-token → per-MTok (`× 1_000_000`).
  - Decide a sync cadence (cron/CI job that re-fetches and regenerates the
    override JSON, or a `make` target a human runs periodically) and commit
    the generated override file, since `LoadPricingOverride` reads from a
    local path, not a URL.
  - Trust: this becomes a **third-party, community-sourced** number feeding
    a real cost dashboard — errors in litellm's file (which does happen,
    e.g. lag on introductory/promotional pricing windows) propagate
    silently unless something diffs it against the authoritative source.

- **Pros**: broad coverage (all providers, not just Anthropic, useful if
  this repo ever prices non-Claude models per the requirements doc's
  explicit non-goal caveat "unless the codebase already tracks them in the
  same table"); free; actively maintained; captures new models fast;
  removes the "someone has to remember to hand-type new prices" toil for
  G1/G4 going forward.
- **Cons**: adds a network dependency (or a periodic-sync job) and a
  third-party trust boundary to a correctness-critical dashboard number;
  schema mapping/conversion code to write and maintain; per-token → per-MTok
  conversion and key-normalization are both places a silent bug could
  reintroduce exactly the class of bug this project is fixing; still needs
  a human or CI job to actually run the sync — it does not eliminate the
  "someone must act when a new model ships" problem, it just makes acting
  on it cheaper.
- **Verdict: Viable, not recommended for this pass.** It's a legitimate
  option for a **future** iteration of G3 (an automated override-refresh
  job), but adopting it now is more integration surface than this fix
  needs. AC-1 only requires closing the *current* gap (Sonnet 5 + any other
  missing active families) and AC-4 only requires *a* working override
  mechanism, not an auto-synced one. Recommend wiring
  `LoadPricingOverride()` into `server/dependencies.go` (already-written
  code, zero new dependency) as the G3 answer now, and file adopting
  litellm's feed as a **follow-up** enhancement to G4's guardrail — the
  Non-Goal explicitly scopes a live/dynamic pricing integration out unless
  "trivially cheap," and a third-party JSON sync with schema mapping and a
  trust boundary is not trivial.

### Other options considered

- No maintained **Go-native** package specifically for Anthropic/Claude
  pricing was found (searched for "Go Anthropic pricing library",
  "claude-pricing-go", etc. — nothing came up beyond generic SDK repos,
  which don't expose pricing data at all, only model IDs). Anthropic's own
  Go SDK (`anthropic-sdk-go`, already a candidate dependency per this
  repo's `prefer-go-git`-style conventions) does not expose a pricing
  table — `client.models.retrieve()` returns `max_input_tokens`,
  `max_tokens`, and `capabilities`, but **no price fields** (confirmed
  against the `claude-api` skill's Models API reference: the model object
  has `id`, `display_name`, `created_at`, `max_input_tokens`, `max_tokens`,
  `capabilities` — pricing is not part of the schema).
- No other broadly-adopted OSS "LLM pricing table" project was found beyond
  litellm's file (it is the de facto community standard other tools, e.g.
  the "LiteLLM Model Prices Viewer" and several pricing-comparison SaaS
  sites cited in search results, build on top of).

---

## 2. Anthropic's own pricing API / published pricing feed

**Finding: Anthropic does not publish a machine-readable pricing API or
JSON feed.** Confirmed two ways:

1. The bundled `claude-api` skill — which is this repo's own up-to-date
   reference for Claude API/model/pricing facts — lists the pricing source
   as `https://platform.claude.com/docs/en/pricing.md`, a **Markdown/HTML
   documentation page** meant for human or WebFetch-and-extract
   consumption, not a stable JSON contract. Its own "Current Models" table
   is explicitly a cache ("cached: 2026-06-24") maintained by manually
   re-scraping that docs page — i.e. even Anthropic's own developer-facing
   tooling treats pricing as scrape-and-cache, not fetch-a-JSON-endpoint.
2. The Models API (`GET /v1/models`, `GET /v1/models/{id}`) — the one
   genuinely machine-readable, versioned Anthropic endpoint relevant here —
   explicitly does **not** carry price fields (see above). It's the right
   tool for context-window/capability discovery, not pricing.
3. A web search surfaced only third-party aggregator sites (finout.io,
   calcis.dev, tldl.io, pricepertoken.com, aipricing.guru) republishing
   Anthropic's list prices in their own scraped/derived JSON — none of
   these are Anthropic-operated, none carry any freshness/accuracy SLA, and
   using one as a pricing source for this project would mean trusting a
   random third party's scrape of Anthropic's docs page over doing the same
   scrape ourselves. One search snippet claimed a `/api/pricing.json`
   endpoint, but tracing it back shows it belongs to one of those
   third-party aggregator sites, not `anthropic.com` or
   `platform.claude.com` — **not an official Anthropic endpoint**, and not
   trustworthy enough to hardcode into a Go codebase's pricing table.

**Implication for G3**: since there is no official, stable, machine-readable
Anthropic pricing feed to point an auto-populating override at, "wire up
`LoadPricingOverride()` to a config file that a human (or a scheduled
scrape-the-docs-page job) edits" remains the correct design — exactly what
the Non-Goal anticipates ("static table + override is sufficient"). There is
no trivially-cheap live integration available; a live integration would mean
either scraping/parsing Anthropic's HTML pricing page (fragile, breaks
silently on any redesign) or depending on a third-party unofficial mirror
(a trust and freshness problem, covered in §1). Recommend not pursuing
either for this project.

**Verdict: Confirms the static-table-plus-manual-override design. Not
recommended to attempt an official-feed integration — none exists.**

---

## 3. Pricing numbers must come from the authoritative source, not LLM memory

**Flag, explicitly, for the implementation phase:** this task requires
literal dollar figures per model family (AC-1: "includes pricing entries for
`claude-sonnet-5` [...] identified during research"). LLM-recalled pricing
is not reliable enough for this — training-data cutoffs, promotional
pricing windows (e.g. this repo's own bundled `claude-api` skill notes
Claude Sonnet 5 has an *introductory* rate through 2026-08-31 that differs
from its steady-state rate — a naive one-time price entry would silently go
stale on that exact boundary), and simple recall drift are all real risks.

**Requirement for whoever implements G1**: pull final $/MTok figures from
Anthropic's live pricing page
(`https://platform.claude.com/docs/en/pricing.md`, via WebFetch, at
implementation time — not from this research doc, not from training
memory) or from `ant messages count_tokens` / `client.models.retrieve()`
capability checks plus the pricing page, immediately before writing the
`DefaultPricingTable()` entries. For reference, the `claude-api` skill's
cached table (cached 2026-06-24, itself sourced from the same pricing page)
lists as of that date:

| Model | Input $/MTok | Output $/MTok |
|---|---|---|
| `claude-sonnet-5` | $3.00 ($2.00 intro through 2026-08-31) | $15.00 ($10.00 intro) |
| `claude-opus-5` | $5.00 | $25.00 |
| `claude-opus-4-8` | $5.00 | $25.00 |
| `claude-opus-4-7` | $5.00 | $25.00 |
| `claude-opus-4-6` | $5.00 | $25.00 |
| `claude-sonnet-4-6` | $3.00 | $15.00 |
| `claude-haiku-4-5` | $1.00 | $5.00 |
| `claude-fable-5` | $10.00 | $50.00 |

**This table is illustrative only, cached as of 2026-06-24, and must be
re-verified against the live pricing page before being written into
`pricing.go`** — in particular the Sonnet 5 introductory-vs-steady-state
split needs an explicit decision (use intro rate now and set a reminder to
update after 2026-08-31? use steady-state now to avoid a silent
under-estimate later? — this is a product decision for the plan phase, not
a research one). Cache-write/cache-read per-MTok multipliers (this repo's
`CacheWritePerMTok`/`CacheReadPerMTok` fields) are not in the table above
and must be sourced separately (Anthropic's standard cache-write premium is
1.25× base input for the default 5-minute TTL, 2× for 1-hour TTL; cache-read
is ~0.1× base input — confirm current multipliers on the pricing page,
don't assume they're unchanged from the existing hardcoded 3.x-model
entries in `pricing.go`).

**Correctness stakes**: this feeds Tyler's real cost dashboard
(`insights_service.go`). An incorrect price here doesn't fail loudly — it
just reports a wrong dollar figure that looks plausible, which is worse
than the current blank/$0 bug because it's not visibly broken. Treat this
as a data-correctness requirement with the same rigor as a financial
calculation, not a copy-paste convenience.

---

## 4. In-repo precedent for "keep a data table current with a guardrail" (G4)

**Yes — a strong, directly-applicable precedent exists: the feature
registry tooling in `docs/registry/` / `tools/scanner/`.**

Read `Makefile` (`registry-generate-backend`, `registry-generate-frontend`,
`registry-aggregate`, `registry-diff` targets) and
`docs/registry/README.md`. The pattern:

1. **Generator** (`tools/scanner/backend/cmd/scanner`, a Go binary) scans
   source (proto files + `// +api:` marker comments in handler code) and
   writes/updates per-feature JSON files under `docs/registry/features/`.
   Generation is additive, then `tools/scanner/prune-stale-backend.sh`
   removes entries whose backing RPC no longer exists, keeping the
   committed set in sync with the actual code.
2. **Dry-run diff gate**: `registry-diff` (→
   `tools/scanner/validate-registry.sh`) re-runs the same scan and compares
   against the **committed** registry files without writing, so it can run
   in CI/`make quick-check` and fail (or at least flag) when the code has
   drifted from the committed source of truth.
3. **Wired into both fast and slow paths**: `quick-check` includes
   `registry-diff` (fast, advisory); `ci` includes the full
   `registry-generate` (regenerate-and-commit expectation).

**This is exactly the "generate-and-diff" shape G4 needs**, and it is a
much better fit than inventing a bespoke guardrail:

- **Generator equivalent for pricing**: a small check (test or `make`
  target) that walks `NormalizeModelFamily()`'s known-pattern set — or,
  more directly, walks the set of model families actually observed in
  recorded session usage (`TurnTimeline` data already flows through
  `EstimateCost`/`ModelFamilyCost`) — and asserts every family present in
  real usage data has a `DefaultPricingTable()` (or override) entry.
- **Diff-gate equivalent**: rather than scanning proto files, the
  "source of truth" to diff against is Anthropic's model catalog. The
  cheapest version of this **without** the live-integration cost flagged in
  §2 is a **test** that fails when `session/tokens/pricing.go`'s table is
  missing a family for any model ID appearing in the test fixtures /
  historical parse corpus already used by `pricing_test.go` — i.e. extend
  `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` (per AC-5) into
  a two-part guardrail:
  1. A unit test asserting unknown-model detection is surfaced (not
     silently zeroed) — satisfies AC-5 directly.
  2. A **second**, cheap regression test that enumerates every
     `claude-*` family currently documented as "Active" in
     `shared/models.md` from the bundled `claude-api` skill (or a small
     hardcoded "known active families" list maintained alongside
     `DefaultPricingTable()`) and asserts each has a pricing entry — this
     is the direct Go-native analog of `registry-diff`: a assertion that
     fails loudly in CI the day a maintainer adds a new active model
     family reference elsewhere in the code (e.g. in
     `session-creation-registry.md`-style model pickers, if any exist) but
     forgets to price it.
- **Optional doc'd-process fallback** (AC-4's "OR a documented decision"
  escape hatch) is not needed given the registry precedent exists and is
  cheap to imitate — no reason to fall back to "just a doc" when a
  test-based guardrail following an established repo pattern is available.

**Verdict: Recommended.** Do not invent a new guardrail mechanism. Follow
the `docs/registry/` generate-and-diff shape at unit-test scale: extend
`pricing_test.go` with (a) the AC-5 unknown-model-is-flagged test and (b) a
"every currently-active Claude family has a price" completeness test, kept
next to `DefaultPricingTable()` so the two are visually coupled for the next
person who edits either.

---

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| litellm `model_prices_and_context_window.json` as override source | Broad coverage, actively maintained, MIT, saves future hand-typing | New network/sync dependency, schema mapping (per-token→per-MTok, key normalization), third-party trust boundary on a correctness-critical number, doesn't remove the "someone must act" step | **Viable, not recommended now** — good follow-up for a later G4 iteration, not this pass |
| Anthropic official pricing API/JSON | Would be authoritative and low-risk if it existed | Doesn't exist — only an HTML/Markdown docs page; no versioned/stable JSON contract | **Not available** — confirms static-table + manual-override is correct, per the Non-Goal |
| Third-party pricing-aggregator JSON (finout, tldl.io, etc.) | Already-structured JSON | Unofficial, no freshness/accuracy guarantee, adds an untrusted intermediary | **Not recommended** |
| Wire up existing `LoadPricingOverride()` into `server/dependencies.go` | Already written, zero new dependencies, directly satisfies AC-4 | Still requires a human to edit the override file when prices change (same limitation as the litellm option minus the auto-sync) | **Recommended** — do this for G3 |
| Copy `docs/registry/` generate-and-diff pattern into a `pricing_test.go` completeness test | Reuses proven in-repo pattern, Go-native, no new tooling, cheap | Requires maintaining a "known active families" list somewhere (low cost) | **Recommended** — do this for G4 |
| LLM-recalled pricing numbers | Fast | Can be stale, wrong, or miss promotional-rate windows (e.g. Sonnet 5's intro pricing) — directly corrupts a real cost dashboard | **Not recommended — flag for implementation phase**: always re-verify against `platform.claude.com/docs/en/pricing.md` at implementation time |
