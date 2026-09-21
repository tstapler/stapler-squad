# Build vs. Buy: backlog-stage-execution-costs

Agent 6, SDD Phase 2. Repo: `/home/tstapler/Programming/stapler-squad`.

## 1. Multi-agent-CLI orchestration abstraction

**Question:** is there a reusable OSS library for "run any of several coding-agent CLIs behind a uniform headless interface with cost/token accounting," vs. hand-extending `session/headless`?

What exists today in-repo: `session/headless/` (`client.go`, `runner.go`, `pool.go`, `caller.go`) wraps `claude -p`, with `runner.go`'s `StreamChunk.CostUSD` populated "only on the final chunk from a first-call JSON response" — i.e. it already parses Claude's own `--output-format json` cost field. `PoolClient.CallBlocking` is the one call site `BacklogService` depends on.

What's out there:
- **gate4agent** (Rust) — a transport-layer abstraction (Pipe/NDJSON, PTY, ACP/JSON-RPC) across Claude Code/Codex/Gemini/OpenCode. Solves process I/O framing, not cost accounting, and is Rust — a foreign runtime dependency for a Go backend.
- **agent-orc/runner** (.NET) — "hardened spawning, streamed events, lifecycle & quota tracking" across Claude Code/Codex/Copilot/Gemini. Closest conceptual match, but .NET — same cross-runtime problem.
- **NEEDLE**, **ralph-harness**, **Kiro Crew** — orchestration/loop scaffolds (bead queues, checkpoint/resume, cron triggers) sitting *above* per-CLI invocation, not a cost-accounting abstraction layer themselves; several are thin/early-stage repos (no evidence of wide adoption or Go bindings).
- No project surfaced that is (a) Go-native, (b) maintained, and (c) provides normalized cost/token accounting across heterogeneous agent CLIs. Every "universal runner" found in this pass treats cost as CLI-specific detail left to the caller, exactly like `session/headless` already does for Claude.

**Verdict: Not recommended (buy).** Nothing found is Go-native, mature, or actually does the hard part (normalized cost accounting across incompatible output formats — see item 3). Adopting a foreign-language library to wrap subprocess CLIs stapler-squad already knows how to spawn (`Instance.Program` already switches between `claude`/`aider`) would add a runtime dependency for no capability gain. **Extend `session/headless` by hand**, one adapter per program (see item 3 for why this is unavoidable regardless of library choice — the CLIs themselves disagree on whether structured cost output exists at all).

## 2. Cost/pricing table maintenance

**Question:** should `session/tokens/pricing.go`'s hardcoded `DefaultPricingTable()` (431 lines, per-model `InputPricePerMTok`/`OutputPricePerMTok`/cache rates, each entry dated and source-commented, e.g. `claude-sonnet-5` verified 2026-07-27 against `platform.claude.com/docs/en/about-claude/pricing`) be replaced by a maintained OSS pricing DB?

- **LiteLLM's `model_prices_and_context_window.json`** (BerriAI/litellm) is real: root-of-repo JSON, CI-updated, backed up to a second file, covers 100+ providers including Anthropic, Google/Gemini, and (via community model-metadata files) OpenAI-compatible providers. License is MIT (LiteLLM's repo license) — permissive, vendorable.
- Cons for this project: (a) it's Python-ecosystem-first — consuming it in Go means either vendoring the raw JSON and writing a Go loader (fine) or pulling in no live Go client; (b) it's fetched *remotely at runtime* by LiteLLM's own clients — stapler-squad would need to decide whether to vendor a snapshot (drifts, needs a refresh job) or fetch live (network dependency in a "local tool," a new failure mode `pricing.go`'s current hardcoded-with-dated-comments design deliberately avoids); (c) Aider's own `.aider.model.metadata.json` format is yet a *third* schema — LiteLLM's JSON doesn't cover Aider-specific cost-metadata conventions, so adopting it doesn't eliminate hand-maintenance for the very programs this feature is adding.
- The existing table's discipline (per-entry `EffectiveDate` + source URL comment, `pricing_test.go` presumably pinning known values) is exactly the audit trail a vendored blob would erase — cross-checking LiteLLM's JSON against Anthropic's own pricing page is still required work either way, since LiteLLM aggregates community-submitted data.

**Verdict: Viable but not recommended as a wholesale replacement.** Vendor LiteLLM's JSON *only if/when* the pricing table needs to cover many third-party model families at once (e.g. supporting arbitrary Gemini/OpenAI models per user config) — for the two concretely-scoped additions here (Gemini CLI's own models, if a headless adapter ships; Aider's `.aider.model.metadata.json` convention), hand-adding entries to `pricing.go` following the current pattern is cheaper and keeps the audit trail. Revisit if model-family count grows past what hand-maintenance can track.

## 3. Non-interactive/scriptable mode for Aider and Gemini CLI — the requirements.md Rabbit Hole

This is the load-bearing finding for the whole feature's scope.

### Gemini CLI: feasible
[Headless Mode docs](https://google-gemini.github.io/gemini-cli/docs/cli/headless.html) confirm `gemini -p "<prompt>" --output-format json` returns structured JSON:
```json
{
  "response": "...",
  "stats": {
    "models": { "gemini-2.5-pro": { "api": {...}, "tokens": {"prompt":24939,"candidates":20,"total":25113,"cached":21263,"thoughts":154,"tool":0} } },
    "tools": {...}, "files": {...}
  }
}
```
No dollar-cost field — only token counts per model. Cost requires applying stapler-squad's own pricing table to `stats.models[*].tokens` (reinforces item 2: Gemini model families need pricing entries regardless of vendoring decision). **A Gemini headless adapter is buildable**: shell out `gemini -p "<prompt>" --output-format json`, parse `stats.models`, compute cost locally.

### Aider: not feasible as structured output
[`aider.chat/docs/scripting.html`](https://aider.chat/docs/scripting.html) confirms the only non-interactive mode is `--message`/`--msg`/`-m` ("process reply then exit") plus `--yes` for auto-confirm — there is **no `--output-format json` equivalent**. Cost/token totals are printed as human-readable text interleaved with the model's reply on stdout (confirmed via aider community docs — no machine-parseable schema documented anywhere in the official docs). The only alternative is aider's Python scripting API (`Coder.create()`/`coder.run()`), which the docs explicitly flag: *"not officially supported or documented, and could change in future releases without providing backwards compatibility."* Shelling out to a regex-scraped stdout format, or embedding an unsupported Python API from a Go process, are both fragile against every aider upgrade.

**Verdict:**
- **Gemini CLI adapter: Recommended / feasible** — build it per requirements.md's "adapters for feasible ones."
- **Aider adapter: Not recommended for cost accounting** — build the headless *execution* path (already partially exists via `Instance.Program`/`SwitchProgram`) but do not promise parseable cost for Aider-run stages; either exclude Aider stages from the cost dashboard (tag them "cost unavailable," mirroring the existing `hasUnpricedUsage`/"Projection excludes unpriced usage" pattern already in `ProjectedCostCard.tsx`) or gate an Aider adapter behind first confirming the actual stdout cost-line format is stable enough to regex (spike, not part of this feature's committed scope). This directly resolves the Rabbit Hole: cross-program headless execution is feasible for Claude + Gemini, not for Aider without accepting scrape-fragility.

## 4. Budget/threshold-alerting UI pattern

Checked `web-app/package.json` dependencies (full list read) and existing UI: no toast/notification library (no `react-hot-toast`, `sonner`, `react-toastify`, etc.) is present. Existing "warning banner" precedent in-repo is **always bespoke small components**, not a shared library:
- `web-app/src/app/insights/ProjectedCostCard.tsx` — exactly the pattern requirements.md wants for the soft budget warning: local `threshold` state, `isWarning` boolean, conditional `warningText` span styled via vanilla-extract (`ProjectedCostCard.css.ts`), a `caveat` span for the unpriced-usage case. No animation/toast library involved.
- `web-app/src/components/DeepLinkErrorBanner.tsx` — same bespoke-banner pattern for a different domain.

**Verdict: Recommended — extend `ProjectedCostCard.tsx`'s existing pattern** (or a sibling component styled identically) for the new per-stage budget warning, using vanilla-extract + plain React state exactly as already done. **Not recommended: adding a toast/alert library.** There is no gap this codebase's existing pattern doesn't already fill, and requirements.md itself flags not to add a new library without strong reason — there isn't one here.

## 5. LLM-generated vs. battle-tested for cost aggregation math

`server/services/insights_service.go` (920 lines) **already contains** the exact aggregation shape this feature needs, just scoped to activity-type/model instead of stage/role/item:
- `GetInsightsSummary` (line 198) builds `activityMap map[sessionv1.ActivityType]*sessionv1.ActivityCostBreakdown`, accumulating `ab.EstimatedCostUsd += costUSD` per session (line 320), plus a parallel `CostByModel map[string]float64` accumulator (line 373-374) and a `totalCostUSD` running sum (line 347).
- `sessionRolesForSessions` (line 86) already builds a `sessionUUID → SessionRole` map from persisted `session_role` (ADR-029), and `SessionRole` is already attached to each summary (line 180) — i.e., **role-keyed grouping infrastructure already exists**, it's just not yet reduced into a role-keyed cost breakdown the way activity-type is.
- `tokens.AttributeToolCosts` (referenced line 840, "Epic 1.2," ADR-001) is the precedent for a *third* cost-attribution dimension (per-tool) layered onto the same `ParseResult`/pricing-table inputs, with explicit double-counting guards (`CostMayDoubleCount`, `CostUnpriced`).

**Verdict: Not recommended to write new aggregation code from scratch.** The per-stage/per-item summation and threshold comparison is exactly the "just write it" case requirements.md anticipates — no external library is warranted — but the right move is to **extend `insights_service.go`'s existing accumulator pattern** (add a stage/backlog-item-keyed map alongside `activityMap`/model-cost accumulators, reusing `sessionRolesForSessions`-style lookups for stage/item association) rather than duplicate the summation logic in a new file or service. Doing otherwise would fork the exact "which sessions count, how do we avoid double-counting tool costs" logic ADR-001 already solved once.

## Summary table

| # | Area | Verdict |
|---|---|---|
| 1 | Multi-CLI orchestration abstraction | Not recommended — no mature Go-native option; extend `session/headless` by hand |
| 2 | OSS pricing DB (LiteLLM) | Viable, not recommended now — hand-extend `pricing.go`; revisit if model-family count balloons |
| 3a | Gemini CLI headless cost | Recommended — `--output-format json` gives per-model token stats, feasible adapter |
| 3b | Aider headless cost | Not recommended — no structured output exists; execution-only or scrape-and-accept-fragility, not in scope |
| 4 | Budget warning UI | Recommended — extend `ProjectedCostCard.tsx`'s bespoke pattern; no new library |
| 5 | Cost aggregation math | Recommended — extend `insights_service.go`'s existing `activityMap`/`CostByModel`/role-map accumulators; no library, no new service |
