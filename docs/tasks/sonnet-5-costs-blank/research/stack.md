# Stack Research — Sonnet 5 Costs Blank

## 1. Existing libraries relevant to a fix

**Go** (`go.mod`, module `github.com/tstapler/stapler-squad`, `go 1.26.3`):

- **JSON**: stdlib `encoding/json` only — `session/tokens/pricing.go:4` already uses it for `LoadPricingOverride`. No third-party JSON lib (no `jsoniter`, no `encoding/json/v2`). Any new config format should stay on stdlib `encoding/json` for consistency, or `gopkg.in/yaml.v3` (already a dependency, `go.mod:48`) if YAML is preferred for hand-editing.
- **HTTP client**: no dedicated HTTP client library beyond stdlib `net/http` (transitively via `golang.org/x/net v0.55.0`, `go.mod:42`). If a fetched-pricing-API approach were chosen, `net/http` + context timeouts is the idiomatic path already used elsewhere in the codebase (e.g. `otelhttp` instrumentation, `go.mod:34`).
- **File watching**: `github.com/fsnotify/fsnotify v1.9.0` (`go.mod:16`) is already a dependency and already used in `session/tokens/store.go:28` and documented in `session/tokens/doc.go:14` for hot-reloading JSONL history files. The same mechanism could hot-reload a pricing override file without polling.
- **File locking**: `github.com/gofrs/flock v0.12.1` (`go.mod:19`) is available if a pricing config file needs safe concurrent read/write.
- **Caching / concurrency**: `github.com/puzpuzpuz/xsync/v4 v4.5.0` (`go.mod:25`) for a concurrent-safe in-memory cache if a fetched or hot-reloaded pricing table needs lock-free reads under concurrent `EstimateCost` calls (today `PricingTable.Prices` is a plain `map[string]ModelPricing` with no synchronization — see §3).
- **Cron**: `github.com/robfig/cron/v3 v3.0.1` (`go.mod:26`) is already used elsewhere in the app (workflow scheduling) and could drive a periodic pricing-refresh job if a fetch-based approach were adopted.
- **go-git**: present (`github.com/go-git/go-git/v5`, `go.mod:17`) but irrelevant to pricing — not applicable here per the repo's own `prefer-go-git-over-subshells` rule (no git operation involved).

**Frontend** (`web-app/package.json`): not directly relevant — the blank-cost bug is a backend computation problem (`EstimateCost`/`ModelFamilyCost` return 0.0), not a rendering problem. `zod` (`web-app/package.json`) is available if a pricing-override JSON needed client-side schema validation for an admin UI, but that's speculative scope beyond the confirmed root cause.

## 2. Does Anthropic publish a machine-readable pricing API?

**No.** Confirmed via the bundled `claude-api` Anthropic-SDK reference skill (Anthropic's own current documentation, cached 2026-06-24):

- The Models API (`GET /v1/models`, `GET /v1/models/{id}`, or SDK `client.models.retrieve(id)` / `client.models.list()`) returns `id`, `display_name`, `created_at`, `max_input_tokens`, `max_tokens`, and a `capabilities` object (thinking support, effort levels, vision, structured outputs, etc.) — **it does not return price**. There is no `price`, `input_cost`, or similar field anywhere in the Model object schema.
- There is no dedicated pricing endpoint (no `/v1/pricing`, no `/v1/models/{id}/pricing`). The only documented source of USD/MTok rates is the human-readable pricing page (`https://platform.claude.com/docs/en/pricing.md`), which is prose/table documentation, not a JSON API contract Anthropic guarantees to keep stable for programmatic scraping.
- Anthropic's own SDKs (Python/TypeScript/Go/etc.) hardcode per-model pricing nowhere in the client — cost estimation is left entirely to the caller. This confirms hardcoding + a documented update process (what `session/tokens/pricing.go` already does) is the intended integration pattern, not an oversight in this codebase.

**Current confirmed rates** (per the skill's cached pricing table, dated 2026-06-24 — cite `platform.claude.com/docs/en/pricing.md` as the authoritative source and re-verify at implementation time since prices do change):

| Model | Input $/MTok | Output $/MTok |
|---|---|---|
| Claude Opus 5 (`claude-opus-5`) | $5.00 | $25.00 |
| Claude Opus 4.8 / 4.7 / 4.6 | $5.00 | $25.00 |
| Claude Sonnet 5 (`claude-sonnet-5`) | $3.00 ($2.00 intro through 2026-08-31) | $15.00 ($10.00 intro) |
| Claude Sonnet 4.6 | $3.00 | $15.00 |
| Claude Haiku 4.5 (`claude-haiku-4-5`) | $1.00 | $5.00 |

Claude Sonnet 4.5 (`claude-sonnet-4-5`) is now a "legacy, still active" model in Anthropic's catalog and isn't in the current headline pricing table, but historically shared the same Sonnet-tier rate ($3.00 / $15.00) as Sonnet 4.6 — worth confirming against the live pricing page rather than assuming, since legacy-tier pricing isn't always identical to current-tier pricing.

**Notable finding for the fix itself**: Claude Sonnet 5's rate ($3.00 input / $15.00 output) is numerically **identical** to the existing hardcoded `"claude-sonnet-4"` entry in `session/tokens/pricing.go:35-42`. The bug is purely a *lookup-key* miss (`NormalizeModelFamily` has no pattern for a `-5` generation suffix), not a stale-price problem — the dollar amounts wouldn't even need to change, only the family-name normalization / table key needs a new entry.

**Recommendation given no API exists**: Document prices manually in `DefaultPricingTable()` (or better, a hot-reloadable override file — see §4) with a clear, auditable update process:
1. A code comment linking to the live pricing URL (`https://platform.claude.com/docs/en/pricing.md`) as the source of truth, already the pattern used in `DefaultPricingTable()`'s doc comment (`pricing.go:21`, "as of 2026-05-15").
2. A `EffectiveDate` field per entry (already exists on `ModelPricing`, `types.go:72`) plus the existing `IsStale()` method (`pricing.go:183-200`, 30-day threshold) — this machinery already exists and already flags staleness; it's just never surfaced anywhere actionable (see §4 for wiring it up).

## 3. Compatibility of the existing `PricingTable`/`ModelPricing` shape

`session/tokens/types.go:65-81`:

```go
type ModelPricing struct {
	ModelFamily        string
	InputPricePerMTok  float64
	OutputPricePerMTok float64
	CacheWritePerMTok  float64
	CacheReadPerMTok   float64
	EffectiveDate      string // ISO date
}

type PricingTable struct {
	Prices     map[string]ModelPricing
	LoadedAt   time.Time
	ConfigPath string
}
```

**Adding a new model family is clean and requires no struct changes.** `ModelPricing` has no model-specific fields (no per-generation quirks baked into the type), so a `claude-sonnet-5` (or `claude-sonnet-5-family`, depending on the normalization key chosen) entry is a straight `map` insert — either as a new hardcoded literal in `DefaultPricingTable()` or as a JSON object matching `ModelPricing`'s field names via `LoadPricingOverride()`.

**Migration/versioning concerns for the JSON override format (`LoadPricingOverride`, `pricing.go:82-102`):**

- The override format is `map[string]ModelPricing` keyed by normalized family name — e.g. `{"claude-sonnet-5": {"InputPricePerMTok": 3.00, ...}}`. Go's `encoding/json` field matching is case-insensitive by default, so JSON keys can be `inputPricePerMTok` or `InputPricePerMTok` interchangeably; this is undocumented behavior a hand-written config file author would need to discover by trial. **Recommendation**: add explicit `json:"..."` struct tags to `ModelPricing` (currently absent) to pin a single canonical casing (e.g. `input_price_per_mtok`) and make the contract unambiguous in the file itself.
- `LoadPricingOverride` **merges** into `DefaultPricingTable()`'s hardcoded map (`pricing.go:96-99`, `table.Prices[family] = pricing`) rather than replacing it wholesale — this is good: an override file only needs to contain the *new or changed* families, not a full re-declaration of every existing price. No breaking-change risk here since old entries survive untouched unless explicitly overridden.
- **No schema version field** exists in the override format today. If the `ModelPricing` struct ever grows a field with different required-ness (e.g. a future `CachedInputTierPerMTok`), an old override file with the old key set would silently zero-value the new field (Go JSON unmarshal leaves missing fields as their zero value) rather than erroring. For a config format meant to be hand-edited and long-lived, consider either (a) validating required fields explicitly after unmarshal, or (b) accepting the zero-value-fills-gap behavior since it's forward-compatible (old files still parse, just get 0.0 for new fields — which for a *price* field is arguably the wrong failure mode, since 0.0 silently produces free/blank cost exactly like the current bug, rather than erroring loudly).
- `pt.Prices[family] = pricing` line 98 has no validation that `InputPricePerMTok`/`OutputPricePerMTok` are non-zero/non-negative — a typo'd `0` in the override file (or a missing field defaulting to `0.0`) reproduces the exact blank-cost symptom this bug report is about, just via config error instead of missing-model error. Any fix that wires up `LoadPricingOverride` should add a sanity check (e.g. reject or warn on an entry with `InputPricePerMTok <= 0`).

**`NormalizeModelFamily` is the actual defect surface**, not the pricing table shape (`pricing.go:104-136`). It has three pattern branches: `dateSuffixPattern` (strips `-YYYYMMDD`), `legacyModelPattern` (handles old `claude-3-opus-...` → `claude-opus-3` format), and `variantSuffixPattern` (`^(claude-(?:opus|sonnet|haiku)-\d+)-\d+$`, strips a *second* trailing digit like `-6` off `claude-sonnet-4-6` → `claude-sonnet-4`). None of these branches map a `claude-sonnet-5-...` or `claude-sonnet-5` ID onto an existing table key — a bare `claude-sonnet-5` model ID has no trailing variant digit to strip (it *is* the family digit), so it falls through to the final `return normalized` (`pricing.go:135`) as literally `"claude-sonnet-5"`, which then has `pt.Prices["claude-sonnet-5"]` — `!ok` — and `EstimateCost`/`ModelFamilyCost` silently `continue` past it (`pricing.go:172-173`, `212-213`), producing $0.00.

## 4. Recommended "lookup prices using tokens" mechanism

Ranked recommendation, combining the confirmed root cause with what's already half-built in this codebase:

1. **Wire up the already-existing but dead `LoadPricingOverride()`.** It's fully implemented and tested (per the task's premise) but `server/dependencies.go:1070` calls `tokens.DefaultPricingTable()` directly and never calls `LoadPricingOverride`. This is the lowest-effort, highest-leverage fix: add a config path (e.g. `~/.stapler-squad/pricing-overrides.json`, consistent with the existing `~/.stapler-squad/config.json` / `sessions.json` convention referenced in the project's `CLAUDE.md`), call `LoadPricingOverride` at startup with a fallback to `DefaultPricingTable()` on missing-file, and document the JSON shape. This directly satisfies "a way to look up prices using token counts rather than a static hardcoded table" — it becomes a *configurable* table instead of a compiled-in one, without a redeploy.
2. **Add the `claude-sonnet-5` (and future `-N` generation) pattern to `NormalizeModelFamily` regardless of #1** — this is the direct root-cause fix and should ship even if the override mechanism isn't wired up, since it's what makes *any* Sonnet-5-family session cost-attributable at all, override file or not. Given `variantSuffixPattern` already generalizes on `\d+` for the trailing variant digit, the real gap is that the *family* digit itself (`4` vs `5` vs `6`...) is only ever matched via hardcoded literal keys in `DefaultPricingTable()`, not derived — every new generation requires both a regex/normalization change *and* a new table entry. Consider whether `NormalizeModelFamily` should track "latest known family per line" separately from raw table lookup, so an unrecognized-but-plausible `claude-sonnet-N` for N > any hardcoded key could at least fall back to the nearest known Sonnet-tier price with a logged warning, rather than silently returning 0. This trades "possibly wrong but non-zero" for "guaranteed zero" — a product decision, not a pure engineering one.
3. **Do not build a fetch-from-Anthropic-API pricing table** — confirmed in §2, no such API exists. This path is a dead end regardless of engineering effort spent on it.
4. **Flag unknown models instead of silently returning 0** — regardless of #1/#2, `EstimateCost` (`pricing.go:170-173`) and `ModelFamilyCost` (`pricing.go:211-215`) should distinguish "cost is genuinely $0" from "cost is unknown because the model isn't in the table." Concretely: have `EstimateCost` return `(float64, []string)` (total cost + list of unrecognized family names it skipped) or have `ModelFamilyCost`'s returned map include an explicit sentinel/marker for unpriced families, so the Insights UI can render "unaccounted" instead of a blank/zero value that looks like free usage. This is the direct fix for the user-visible symptom ("Insights page shows blank/unaccounted-for costs") independent of whether the underlying price gets fixed the same day — it turns a silent data-quality bug into a visible, actionable one. The existing `IsStale()` method (`pricing.go:183-200`) is a precedent for this kind of "surface a data-quality signal" API and could be extended or paired with an `UnpricedFamilies()` accessor.

**Summary**: root cause is a missing `NormalizeModelFamily`/table entry for the Sonnet-5 generation (#2, small and urgent); the durable fix the user is asking for ("look up prices using our tokens... rather than a static hardcoded table") is wiring the dormant `LoadPricingOverride` into startup (#1); and the UX gap (blank costs look like $0, not "unknown") should be closed with explicit unknown-model surfacing (#4) so this class of bug is visible immediately next time instead of silently under-reporting spend.
