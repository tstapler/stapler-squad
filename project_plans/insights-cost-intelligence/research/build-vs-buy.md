# Build vs. Buy — insights-cost-intelligence

Research question: for the waste-pattern findings engine, per-tool cost breakdown,
richer sort/search, and session drill-down route described in
`project_plans/insights-cost-intelligence/requirements.md`, should any part be
bought/vendored/adapted from an existing library, SaaS, or reference project rather
than hand-written?

## 1. Existing OSS library or framework

### Generic Go rules-engine libraries (Grule, RuleGo, GoRules/ZEN, go-rules-engine)

Searched current (2026) options: `hyperjumptech/grule-rule-engine` (Drools-inspired
GRL DSL), `rulego/rulego` (Apache-2.0, embeddable component-orchestration engine),
`gorules/zen` (Rust-core, Go bindings, JDM decision-table format), `Icheka/go-rules-engine`
(JSON-AST rules).

- **Pros**: mature DSL/JSON rule authoring, hot-reloadable rules without recompiling,
  useful if rules needed to be user-editable at runtime.
- **Cons**: every one of these is built for *externalized, user-authored* business
  rules (loan approval, pricing tiers, IoT orchestration) — an interpreter layer,
  a rule-authoring format, and (for ZEN) a Rust FFI dependency. This project's
  Non-Goals explicitly rule out a "general-purpose rules/config UI for tuning
  waste-detector thresholds" (requirements.md Out-of-Scope) — thresholds are
  hardcoded constants for v1. Pulling in a rules-engine framework buys exactly the
  capability (dynamic rule loading/DSL) that's been explicitly deferred, while adding
  a new dependency, a new mini-language to document, and CGO/FFI risk (ZEN) in a
  single-operator Go binary that otherwise has none.
- **Verdict: Not recommended.** The six waste-pattern heuristics in scope are
  independent `func(SessionTokenSummary) []Finding`-shaped threshold/ratio checks
  with no need for cross-rule dependency graphs, priority/conflict resolution, or
  runtime rule editing — the actual value proposition of a rules engine. A plain Go
  slice of heuristic functions run in sequence is strictly simpler and has zero new
  dependencies.

### "LLM usage anomaly/waste detection" libraries

Searched broadly (LLM cost/anomaly detection, "token waste detector"). Nothing
dedicated exists as an importable library — the capability shows up only as a
bundled feature inside full LLM gateways/observability platforms (Bifrost,
Helicone, LiteLLM Proxy), which are proxy servers sitting in the request path, not
libraries over already-parsed JSONL logs.

- **Pros**: n/a — no fit.
- **Cons**: these are all live-traffic proxies (swap your API base URL) designed for
  multi-provider, multi-team gateway deployments, not a library that ingests
  already-parsed `SessionTokenSummary` data and returns findings. Adopting one would
  mean routing all Claude Code traffic through a new proxy — a much larger
  architectural change than this project's scope, and moot anyway since stapler-squad
  already has its own JSONL parser and pricing table (explicitly out of scope to
  replace).
- **Verdict: Not recommended.**

### The four reference projects — reusable code vs. ideas only

Per the prompt, these were already researched in an earlier pass; noting them here
as prior art without re-fetching:

- **Netflix's internal `claude-code-cost-optimizer-plugin` skill** — internal-only,
  not importable into an open-source local tool regardless of fit; treated as an
  ideas source (the heuristic categories: cache-hit floor, CLAUDE.md bloat, redundant
  reads, session ceiling, model-switch cache-bust, tool failure rate) already listed
  verbatim in requirements.md Scope item 1.
  Attempted a live re-check via `mcp__NECP__get-netflix-engineering-context` and
  Netflix-internal search is unavailable in this session (no Netflix MCP context
  reachable from this environment/worktree) — deferred to whatever the earlier
  research pass already captured; nothing new confirmed here.
- **`Tanisha-Katara/cacheeconomics`, `happy-token/TokenUsage`,
  `lucemia/claude-session-analyzer`** — re-searched GitHub/web for these exact
  names and found no matching public repositories (searches for
  "Tanisha-Katara cacheeconomics claude_code.py" and "happy-token TokenUsage
  weighted scoring" returned unrelated results only). This may mean the repos are
  private, deleted/renamed since the earlier research pass, or the names/authors
  were mis-transcribed there. **This should be flagged back to whoever ran the
  earlier research pass** — the specific claims in requirements.md (cacheeconomics'
  Python heuristics module, TTL-crossover/invoice-reconciliation logic; TokenUsage's
  weighted-scoring/activity-classification approach) could not be independently
  re-verified in this pass. Treat the "port formulas 1:1" question (see §4) as
  answerable only from the earlier pass's saved notes/fetched content, not from a
  fresh lookup — if those notes don't exist as durable artifacts (they aren't in
  `project_plans/insights-cost-intelligence/` today — the directory had only
  `requirements.md` before this research pass), that earlier research is
  effectively lost and would need to be redone before Phase 3 planning can cite it
  with confidence.
- **Verdict on "ship reusable code, not just ideas"**: based on what's
  independently verifiable in this pass, **none** of the four are importable/vendorable
  Go or TS code — at best they're a spec of formulas and category lists to
  reimplement, and even that spec's provenance needs re-confirming per the note
  above before Phase 3 leans on it.

## 2. SaaS / managed API

Searched current Claude Code cost-tooling landscape. Confirmed three tiers exist:

1. **Anthropic-native**: Usage & Cost API, Claude Code Analytics API — org-level
   aggregated usage/billing, not per-session waste findings, and requires org/API
   billing account credentials this tool doesn't have (stapler-squad reads local
   JSONL directly).
2. **Local/self-hosted dashboards reading the same JSONL logs**: `ccusage`,
   `phuryn/claude-usage`, "Token Dashboard" — closest analogues to
   stapler-squad's own `/insights` route, but none expose an installable
   findings/heuristics *library* — they're standalone CLIs/dashboards, and their
   waste-detection logic (where present) isn't factored out for reuse either.
3. **Gateway/proxy SaaS** (Cloudflare AI Gateway, Bifrost, Helicone, LiteLLM Proxy):
   multi-team chargeback and live-traffic anomaly alerting, requires proxying all
   API calls through a third party.

- **Pros**: zero maintenance for whichever slice of functionality is bought.
- **Cons**: every option either requires routing live traffic through a third-party
  proxy (a new egress path — directly conflicts with this project's Security
  classification: "no new external data egress") or requires Anthropic org-billing
  API access this single-operator local tool doesn't have and isn't trying to get.
  None do "waste-pattern findings over already-parsed local JSONL" as a hosted API.
- **Verdict: Not recommended.** Confirmed no fit, as expected going in — a
  single-operator, local-only, JSONL-log tool has no SaaS integration point that
  doesn't either add network egress or require infrastructure (a billing API key)
  this tool deliberately doesn't depend on.

## 3. LLM-generated implementation vs. battle-tested library

For the heuristic threshold/ratio logic itself (cache-hit floor, CLAUDE.md size
check, redundant-read count, session-cost ceiling, mid-session model-switch
detection, tool failure-rate ratio):

- Every one of these is a pure function over already-computed aggregate fields
  (`SessionTokenSummary`, `TopToolEntry`, cache read/write counts) — comparisons
  against constants, a ratio, or a simple pattern match over an ordered event list
  (model-switch detection). None involve statistical modeling, machine learning,
  concurrency, cryptography, or anything with a well-known "don't roll your own"
  failure mode.
- The Alternatives Considered section of requirements.md already rejected
  LLM-at-runtime judgment for this exact reason (cost/latency/non-determinism), and
  §1 above rejected a rules-engine framework as solving a problem (dynamic rule
  authoring) this project doesn't have.
- **Verdict: Recommended — hand-written Go, confirmed.** This is a genuine
  "just write it" case. There is no meaningful battle-tested-library risk being
  skipped: the risk surface here is getting the *thresholds and formulas* right
  (a product/data question, resolved via fixture-based tests per the Feasibility
  Risks section), not an algorithmic-correctness risk a library would mitigate.
  ~6 small functions plus a `[]Finding` fixture-test harness is proportionate; no
  dependency pulls its own weight here.

## 4. Fork or adapt cacheeconomics' Python heuristics module

Per §1's finding, the specific `cacheeconomics` repo/module named in
requirements.md could not be independently located in this research pass to
inspect its actual code — this answer is therefore reasoned from the *description*
in requirements.md itself (TTL-crossover curves, reconciliation-against-invoice
gating, "saving_vs_uncached" counterfactual) rather than from re-read source, and
should be treated as provisional pending someone re-locating/re-confirming the
original repo.

- **The counterfactual formula ("saving vs. uncached") is likely portable as a
  spec**: computing `cost_if_all_input_had_been_fresh - actual_cost_with_cache` is
  a formula, not an algorithm — it can be copied conceptually and reimplemented in
  Go directly against `SessionTokenSummary`'s existing cache read/write/hit fields
  without needing the source code at all. This is the piece requirements.md's Scope
  item 3 (cache ROI) most directly wants.
- **TTL-crossover curve modeling does not apply.** stapler-squad has one fixed
  cache TTL behavior (Claude Code's own 5m/1h ephemeral cache), not a
  configurable-TTL cost-curve optimization problem across multiple candidate TTLs —
  there's nothing to crossover-compare. This is cacheeconomics-specific scope this
  project has no analog for.
- **Invoice-reconciliation gating does not apply**, per the Out-of-Scope list
  ("Live Anthropic billing reconciliation against JSONL-derived costs" is
  explicitly excluded) — stapler-squad has no real invoice to reconcile against,
  only the JSONL-derived estimate, so any "abstain rather than guess" logic that
  depends on comparing estimate-vs-invoice has no second data source to gate on
  here. The *spirit* of "abstain rather than guess" (documented in requirements.md's
  Rabbit Holes as the precedent to follow for the per-tool cost-attribution caveat)
  is worth keeping, but it applies to a different uncertainty (per-tool cost
  apportionment) than the one cacheeconomics uses it for (invoice mismatch).
- **Verdict: Viable, but as a formula-to-reimplement, not a module to port 1:1.**
  Only one piece (the counterfactual cache-savings formula) transfers directly, and
  even that transfers as a five-line arithmetic expression against existing fields,
  not as ported code — there's no Python module worth vendoring or transliterating
  line-by-line since roughly half its described feature surface (TTL curves,
  invoice gating) doesn't apply to this project's simpler estimated-cost-only
  context. Recommend Phase 3 planning state the cache-ROI formula explicitly as a
  one-line spec in the plan doc rather than referencing "port cacheeconomics" as an
  implementation instruction.

## Summary table

| Option | Verdict |
|---|---|
| Go rules-engine library (Grule/RuleGo/ZEN/etc.) for the heuristics | Not recommended |
| Generic LLM-usage anomaly-detection library | Not recommended (none exists as an importable library) |
| Vendor/port reference-project code wholesale | Not recommended — none independently confirmed as vendorable; treat as ideas/spec only, and re-confirm the earlier pass's source claims before relying on them further |
| SaaS/managed cost-analysis API | Not recommended — no fit, confirmed (egress + billing-API mismatch) |
| Hand-written Go for the ~6 heuristics | Recommended |
| Port cacheeconomics' cache-ROI counterfactual formula (not the module) | Viable, as a one-line spec, not a code port |
| Port cacheeconomics' TTL-crossover / invoice-reconciliation logic | Not recommended — no analog in this project's scope |
