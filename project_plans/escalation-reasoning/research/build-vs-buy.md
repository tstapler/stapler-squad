# Build vs. Buy: Escalation Reasoning on Review Queue Items

Phase 2 research (SDD). Scope per `project_plans/escalation-reasoning/requirements.md`:
thread an already-computed `RuleID`/`Reason` (or synthetic domain-age reason) through
existing Go structs into an existing `map[string]string` metadata field, render as plain
text in an existing panel, add a 5-category count breakdown via one new proto field, wire
an existing "Create Rule" flow to a new visibility condition, and write one new e2e spec.

## 1. Existing OSS library / framework (rule-engine explainability)

**What exists in the ecosystem:** Policy/rule engines with first-class "why did this
decision happen" support are a real category — OPA/Rego's `decision_id` + explain-mode
tracing, Drools' `KieRuntimeLogger`/agenda-group audit trail, and general Rule Engine
"explanation facilities" literature. These solve *decision provenance* — e.g. "which of N
rules fired, in what order, with what bindings" for graph-shaped rule sets.

**Why it doesn't apply here:** The problem this repo has is not "the rule set is too
complex to explain" — `pkg/classifier/classifier.go` already computes a single flat
`RuleID` + `Reason` string per decision (confirmed: `classifySingle` returns one
`ClassificationResult`, no rule-conflict/precedence graph to trace). The entire
"explainability" requirement is: don't discard a string that's already sitting in a local
variable. Adopting a policy-engine explainability framework here would fail this repo's own
`.claude/rules/interface-pollution-checklist.md`, specifically:
- **Smell #1 (speculative interface)** — a `classifier.Rule` taxonomy of 5 fixed categories
  (no-match / explicit-rule / domain-age / secret-scan / unclassifiable) has exactly one
  "implementation" (the switch in `analytics_store.go`'s `ComputeSummary`) and no second
  consumer on the horizon.
- **Smell #5 (unjustified generic)** — a plugin-style rule-explanation engine used at a
  single call site (`ComputeSummary`'s category bucketing) is a generic solving a problem
  a five-armed `switch`/`if` already solves more legibly.

**Pros of knowing about it (for future reference):** if the classifier ever grows genuine
rule *precedence* (multiple rules matching, conflict resolution, rule composition), OPA's
decision-explain pattern (structured trace objects, not just a string) is the right prior
art to reach for then — not now.

**Cons of adopting now:** new runtime dependency (OPA sidecar or Rego interpreter, or
Drools' JVM requirement — immediately cross-language-incompatible with this Go codebase
anyway), new deployment/ops surface, a rule DSL to learn and maintain, for a fixed 5-value
enum that already exists as `RuleID` string comparisons.

**Verdict: Not recommended.** Worth a one-line note in the ADR for future-self if the
classifier ever grows multi-rule conflict resolution; inappropriate scope now.

## 2. SaaS / managed API (domain-age lookup)

**What exists:** `DomainAgeChecker.IsNewlyRegistered` (`server/services/domain_checker.go:108`)
does call out to a live RDAP registry lookup (`lookupRegistrationDate` →
`fetchRDAPRegistrationDate`, `domain_checker.go:135` on), with an in-memory TTL cache
(`d.cache`, `d.cacheTTL`) to avoid repeat network calls within the cache window.

**Confirmed call site and timing:** `IsNewlyRegistered` is invoked exactly once, at
decision time, inside `HandlePermissionRequest`
(`server/services/approval_handler.go:241`), inside the domain-loop over
`ExtractDomainsFromCommand(cmd)`. The synthetic reason string is built immediately after
(`approval_handler.go:248`: `reason := fmt.Sprintf("Domain %q was registered within the
last %d days...", domain, threshDays)`) and then explicitly discarded
(`approval_handler.go:261`: `_ = reason // will be surfaced when the approval is shown in
review queue`). There is **no code path that re-invokes `IsNewlyRegistered` — or any RDAP
lookup — when a queue item is later rendered.** The review queue reads
`ApprovalMetadata`/`ReviewItem.Metadata`, a static map populated at
poller-run/escalation time, not on each render.

**What this means for design:** the reason string MUST be captured and persisted at
decision time (in the `goto createApproval` branch, before the label), not recomputed at
render time. This is exactly what AC1 and AC2 already specify — this axis doesn't change
the plan, it *confirms* a constraint the requirements already assumed correctly. If the
design mistakenly tried to "look up the reason lazily" when the review queue is displayed,
that would introduce a live RDAP call on every panel render/poll — unnecessary latency,
rate-limit risk against the RDAP registry, and a correctness bug (the domain's age at
render time is irrelevant; what matters is why it was flagged *at decision time*, and the
registration date is immutable historical fact anyway).

**Pros of a managed API here:** RDAP itself is already effectively "buy" (a free, federated,
IANA-specified lookup protocol, not something to reimplement) and the codebase already made
that call correctly — no root-cause work needed on this axis.

**Cons / risk:** none introduced by this feature; the existing cache/TTL design already
protects against redundant network calls. The only failure mode to avoid is *adding* a new
render-time call, which nothing in the requirements proposes.

**Verdict: Not applicable / already correctly buy where it matters** (RDAP lookup itself);
**the reasoning string must be capture-at-decision-time, never recompute-at-render-time** —
confirmed, not just assumed.

## 3. LLM-generated implementation vs. battle-tested library (nearest-match / fuzzy similarity)

**What's tempting:** the original GitHub issue's mockup includes "3 similar commands were
auto-approved in this session" — a nearest-match / similar-command detail that invites
either hand-rolled fuzzy string matching (Levenshtein, Jaccard on tokens) or an LLM call to
say "this looks like commands X, Y, Z you already approved."

**Confirmed out of scope:** requirements.md's "Out of scope" section states explicitly:
"The '3 similar commands were auto-approved in this session' nearest-match detail and
one-click rule-add from the original GitHub issue's mockup are **stretch** in the issue but
not present in any numbered AC — not required." None of AC1–AC8 reference nearest-match or
similarity scoring. This should stay excluded — it is the textbook definition of scope creep
for this ticket (an entirely new computed feature — similarity scoring over a session's
command history — layered onto a task whose only computation is "don't discard a string").

**If a future stretch does add it:** the appropriate off-the-shelf approach is a small,
well-tested Go string-similarity library (e.g. `github.com/agext/levenshtein`,
`github.com/xrash/smetrics`, or a simple n-gram/Jaccard implementation using stdlib
`strings`) run over the session's own already-recorded `AnalyticsEntry.CommandPreview`
history — not an LLM call. Rationale: this is a deterministic, low-latency, cheap
computation (comparing short command strings) with no need for semantic understanding;
an LLM call would add latency, cost, and non-determinism to a panel render for a task a
20-year-old string-distance algorithm handles correctly and reproducibly. Hand-rolling a
novel similarity metric from scratch would itself be a `interface-pollution-checklist.md`-
adjacent smell (unjustified reinvention) — reach for the library, not bespoke code, if this
stretch is ever picked up.

**Verdict: Not recommended now (correctly out of scope); if revisited, Viable via an
off-the-shelf Go string-similarity library — not an LLM call, not hand-rolled from
scratch.**

## 4. Fork or adapt (in-repo precedent)

Two precedents in this same codebase should be forked/copied almost verbatim rather than
designed fresh:

### 4a. Coverage-gap badge/computation pattern (`analytics_store.go` / `ApprovalAnalyticsPanel.tsx`)

Confirmed via `grep -n "CoverageGap"`:
- **Backend aggregation** — `ComputeSummary` (`server/services/analytics_store.go:317-440`)
  loops once over `[]AnalyticsEntry`, and for the coverage-gap bucket specifically
  (`analytics_store.go:395-406`) does exactly the shape of check the new taxonomy needs:
  `if e.Decision == "escalate" && e.RuleID == "" { summary.CoverageGapCount++ ... }`. The
  new 5-category breakdown is the same pattern generalized to a `switch`/`if`-chain over
  `(e.Decision, e.RuleID)` per the taxonomy already specified in requirements.md ("Reason
  category taxonomy" section), accumulating into a `map[string]int` (mirroring
  `uncoveredToolCounts`) and a sorted stat slice (mirroring `topNTools`/`RuleStat`
  construction at `analytics_store.go:410-416`).
- **Struct fields** — `AnalyticsSummary.CoverageGapCount`/`CoverageGapRate`
  (`analytics_store.go:105-106`) are the field-naming/shape precedent for whatever new
  fields carry the 5-category breakdown (e.g. `EscalationReasonStats []CategoryStat`).
- **Proto mirror** — `AnalyticsSummaryProto.coverage_gap_count`/`coverage_gap_rate`
  (`proto/session/v1/types.proto:1122-1126`) plus the existing `RuleStatProto`/
  `ToolStatProto` messages (`types.proto:1136-1154`) are the direct template for a new
  `repeated CategoryStatProto escalation_reason_stats = 17;` (next available field number
  after `command_subcommand_stats = 16`) — a new message shaped like `RuleStatProto`
  (`category`, `count`) rather than reusing `RuleStatProto` itself (semantically distinct:
  rule IDs vs. reason categories).
- **Frontend badge rendering** — `ApprovalAnalyticsPanel.tsx:340-349` (`summary.coverageGapCount > 0 && (() => { ... })()`,
  threshold-based badge class selection `gapBadgeHigh`/`gapBadgeMed`/`gapBadgeLow`) is the
  precedent for rendering the 5-category breakdown — likely as a small stat table or badge
  row rather than reinventing a chart component.

### 4b. `SuggestedRuleCard` / `GenerateSuggestedRule` flow (`ReviewQueuePanel.tsx`)

Confirmed via `grep`: the "Create Rule" button already exists at
`ReviewQueuePanel.tsx:817-834`, already calls `generateRule({ source:
SuggestionSource.COMMAND_SAMPLE, commandSample: queueItem.metadata!["tool_input_command"],
toolNameFilter: ... })`, already opens via `createPortal` at `ReviewQueuePanel.tsx:1345` into
a modal rendering `<SuggestedRuleCard ... />` at `ReviewQueuePanel.tsx:1412`. Per
requirements.md AC3, this flow is **reused as-is** — the only change is conditioning
visibility/emphasis (AC7: `intent="secondary"` instead of the current `intent="ghost"` at
line ~820) on the new escalation-reason-category metadata, not new RPC plumbing, not a new
modal, not a new card component. `commandSample` is already sourced from existing metadata
(`tool_input_command`), confirming zero new backend surface is needed for AC3/AC7 beyond
reading the new `escalation_category` metadata key to decide when to emphasize the button.

**Pros of forking these patterns:** both are already tested (existing Jest tests:
`SuggestedRuleCard.test.tsx`, `useGenerateRule.test.ts`; existing proto/analytics tests
presumably cover `ComputeSummary`), already match this repo's conventions (naming, CSS
class patterns, test IDs), and directly satisfy the interface-pollution checklist's
preference against speculative new abstractions — copying a working `if`-chain and a working
button-wiring is strictly lower risk than inventing a new aggregation strategy or a new
button/modal flow.

**Cons:** none identified — this is the correct default for an SDD "reuse existing
plumbing" task, and no aspect of the new taxonomy or the button-emphasis change requires
diverging from either precedent's shape.

**Verdict: Recommended — fork both almost verbatim.** This is the dominant path for the
entire feature.

## Summary verdict

| Axis | Verdict |
|---|---|
| 1. OSS rule-engine explainability framework (OPA/Drools) | Not recommended — would trip `interface-pollution-checklist.md` smells #1/#5 for a 5-value enum with one consumer |
| 2. SaaS/managed API (RDAP domain-age) | Not applicable to new work — already correctly "buy" where it matters; confirms reason must be captured at decision time, not recomputed at render time |
| 3. LLM vs. hand-rolled (nearest-match similarity) | Not recommended now — confirmed out of scope in requirements.md; if ever revisited, an off-the-shelf Go string-similarity library beats both hand-rolling and an LLM call |
| 4. Fork in-repo pattern (coverage-gap + SuggestedRuleCard) | Recommended — copy both almost verbatim; this is the dominant implementation path |

**Overall: build, reusing existing in-repo patterns almost entirely.** This was not a
foregone conclusion going in — axis 1 required actually confirming the classifier has no
rule-conflict graph (only a flat single-result `classifySingle`) before ruling out a
policy-engine framework, and axis 2 required tracing the actual call site to confirm
`IsNewlyRegistered` truly fires only once at decision time rather than assuming it. Nothing
in the four axes surfaced a case for external adoption; the one genuine design constraint
worth carrying into the plan phase is the capture-at-decision-time requirement from axis 2,
which validates (rather than changes) what requirements.md's AC1/AC2 already specify. The
only new "build" surface is the ~5-branch category classification function itself
(deriving `EscalationCategory` from `(Decision, RuleID)`) — everything else is wiring
through structs/maps/proto fields that already exist, or button/modal code that already
exists.
