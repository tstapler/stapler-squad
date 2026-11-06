# Build vs. Buy: session-classifier-pipeline

Research for Phase 2 (SDD). Evaluates whether the sync-rule engine, LLM fallback,
cycle detection, content-hashing, and poller/headless plumbing should be built from
scratch, sourced from an OSS library, or forked/adapted from existing in-repo code.

## 1. External Go rule-engine libraries vs. generalizing `pkg/classifier`

Checked `go.mod` (repo root): no rule-engine, expression-evaluator, or CEL dependency
is present today (`grule-rule-engine`, `expr-lang/expr`, `google/cel-go`, etc. are all
absent). `pkg/classifier/classifier.go` (2857 lines) already implements a
priority-ordered `Rule` / `RuleBasedClassifier` with `RuleSource` (seed/user/generated)
provenance, `SeedRules()`, ent-backed persistence (`session/ent/schema/approvalrule.go`),
and full test coverage for the approval-rule domain.

**hyperjumptech/grule-rule-engine** — GRL DSL, its own rule language and knowledge-base
concept. Actively maintained but introduces a second rule syntax users would need to
learn alongside the existing `Rule` struct's regex/field-match model; embedding it would
mean either (a) running two independent rule systems side by side, or (b) writing a
translation layer from `Rule` into GRL and back for editing, which is more integration
work than the generalization this project already needs.

**expr-lang/expr / google/cel-go** — general expression evaluators (not rule-priority
engines); would still require hand-building the priority/fixpoint/tag-dependency
evaluation loop and the seed/user/generated provenance model around them. They would
replace `CommandCriteria.Matches`-style predicate logic, not `RuleBasedClassifier`
itself.

**Verdict: Not recommended.** Requirements.md is explicit and unambiguous: "generalize
... not build parallel infrastructure for session tagging," and lists "build a
standalone tagging-rule engine independent of `pkg/classifier`" under Alternatives
Considered as **rejected** "per explicit user direction." Adopting any external
rule-engine library — even scoped to only the tagging domain — would be exactly the
parallel infrastructure the requirements rule out, since regex/field matching plus
priority ordering is already fully solved by the existing `Rule` type; the only real
work is factoring a shared matching core out of the tool-use-specific `Classify`
orchestration (the Feasibility Risk already flagged in requirements.md). No external
library removes that refactor — it only adds a second rule model to reconcile against.

## 2. SaaS/managed classification APIs vs. self-hosted cheap-LLM fallback

`README.md`: stapler-squad is "a web-based mission control ... Run it with `ssq`, then
open `http://localhost:8543`" — a single local self-hosted binary/service, not a
multi-tenant SaaS product. `session_summary_service.go` and `session/headless/features.go`
already route every LLM call through the local `headless.PoolClient` (which shells out to
locally-installed CLI agents), and `anthropic_client.go`/`server/workflows/model_families.go`
hardcode `claude-haiku-4-5-20251001` as the existing cheap-model choice. Requirements.md's
Non-functional Requirements section states data residency is "Not applicable — local
self-hosted service," but also requires the LLM prompt be scoped to metadata "unless
research determines richer content is needed and safe" — i.e., session names, branches,
paths, and program are already being sent off-device to Anthropic's API today for other
features (narrative generation), so this isn't a residency regression, but it does mean
whatever data goes to a *hosted classification/labeling* SaaS is an additional third
party beyond Anthropic.

A hosted content-classification API (e.g. AWS Comprehend custom classification, GCP
Natural Language, or a labeling SaaS) would add: a second vendor credential/config
surface, per-call cost on top of (not instead of) the already-integrated Anthropic
billing, network dependency for a background poller in a tool whose explicit selling
point is local-first operation, and no clear quality advantage over a general-purpose
Haiku-class call for a task this open-ended ("is this a bugfix vs. refactor vs. test
work") — classification-as-a-service products are tuned for fixed label taxonomies with
training data, not zero-shot judgment calls on arbitrary session metadata.

**Verdict: Not recommended.** Self-hosted cheap-LLM fallback via the existing
`headless.PoolClient` pattern fits the repo's local-first model, reuses infrastructure
that's already integrated and tested, and avoids a second vendor relationship for a
task the existing model choice already handles. A SaaS classifier would be viable only
if Haiku-class zero-shot accuracy proved insufficient in practice — not a Day-1 concern
per requirements.md's Appetite/Scope.

## 3. LLM-generated vs. battle-tested library: cycle detection and content-hashing

**(a) Fixpoint / cycle detection for tag-dependency rule chains.** No graph library
(`gonum`, `dominikbraun/graph`, etc.) is present in `go.mod`, and no existing
cycle-detection code exists anywhere in `pkg/classifier` or `session/` (grepped for
`cycle|Cycle|topological|DAG|graph` across both — the only hits are unrelated comments
about container lifecycle and backlog rework cycles). Requirements.md's Rabbit Holes
section is explicit that this "needs an explicit iteration cap and/or cycle detection,
not just 'loop until no change'" — but the actual mechanism needed is simple: rules are
evaluated to a fixpoint over a small, bounded tag set (tens of tags, not an
arbitrary-sized graph), so a **hard iteration cap** (e.g. `len(rules)` or a fixed
constant) combined with a **fired-rule-signature dedup** (skip re-firing a rule that
already added its tag) is sufficient to guarantee termination without needing a real
directed-graph cycle-detection algorithm (e.g. DFS with a visited/gray-set, as used for
topological sort). Pulling in a graph library for this would be over-engineering: the
"graph" here is implicit in rule-tag dependencies, not an explicit adjacency structure
worth modeling as one.

**Verdict: Not recommended (library); hand-roll the iteration cap.** This is a case
where the LLM-generated implementation is trivial and well-understood (bounded loop +
cap + logged warning on cap-hit, exactly as requirements.md's Observability Requirements
calls for) — reaching for a graph library adds a dependency and an abstraction mismatch
for a problem that doesn't need general graph traversal.

**(b) Content-hashing for the LLM cache key.** `go.mod` already depends on
`github.com/spaolacci/murmur3` (non-cryptographic) and stdlib `crypto/sha256` is already
the repo's established convention for exactly this kind of cache/dedup key — see
`config/workspacepath/workspacepath.go:117`, `internal/history/models.go:29`
(`Event.GenerateID`, hashing command+source+timestamp into a dedup ID), `server/tls.go:229`,
and `server/services/push_service.go:159/173`. This is a cache key, not a security
boundary (no adversarial-collision concern), so `fnv` (stdlib `hash/fnv`, faster,
smaller) would also work correctness-wise, but it has no precedent in this repo — every
existing "hash this content for a stable ID/cache key" call site uses `sha256`.

**Verdict: Recommended — `crypto/sha256`, matching existing repo convention.**
Consistency with `internal/history/models.go`'s established pattern outweighs the minor
performance edge of `fnv`/`murmur3` for a low-frequency (per session-mutation) hash.

## 4. Fork/adapt existing in-repo code vs. design fresh

**Debounced background poller with content-hash caching.** `session/pr_status_poller.go`
(467 lines) and `session/worktree_pr_poller.go` (374 lines) both implement the same
shape: a `Poller` struct with `Start(ctx)`/`Stop()`, an internal `pollLoop()` on a
ticker, a per-item fetch-and-update method, auth/error backoff handling
(`isAuthOK`, `handleFetchError`), and an `OnUpdated` callback hook. Neither currently
does content-hash caching specifically (they use HTTP ETags via `github.ETagCache` for
GitHub API conditional requests — a different but analogous "don't redo work if nothing
changed" mechanism). Requirements.md explicitly directs mirroring these two pollers'
*structure* ("mirroring `pr_status_poller.go`/`worktree_pr_poller.go`'s structure") and
flags "reuse vs. duplicate" as a real decision in Rabbit Holes — i.e., whether a common
poller base/registration point exists to hook into rather than copy-pasting the
Start/Stop/pollLoop skeleton a third time.

**Verdict: Recommended — adapt the pattern, don't fork the files.** Copy the
Start/Stop/pollLoop/backoff *shape* into a new `session/session_classifier_poller.go`
(or similar), replacing the ETag-based change-detection with a `sha256`-based content
hash of the classification-relevant session fields, and replacing the GitHub
fetch-and-update with a `headless.PoolClient` call. Do not literally fork
`pr_status_poller.go`'s file — its domain logic (PR fetch, ETag cache, GitHub auth
checks) is irrelevant to tag classification and would only add dead weight to strip out.
If Phase 3 planning finds a shared base worth extracting (e.g. a common
`Poller{Start,Stop,pollLoop}` skeleton), that extraction is a reasonable follow-on
refactor but isn't required to ship this feature — requirements.md scopes it as "modeled
on," not "share code with."

**Headless/LLM invocation code for structured tag output.** `session/headless/features.go`
has two invocation shapes: prose-returning (`GenerateSessionCompletionNarrative`,
`GenerateHandoffSummary` — return a string) and JSON-returning (`GenerateSummarize`-style
calls at lines 236/245 and 256/262, which call `pool.CallBlocking(...)` then
`json.Unmarshal([]byte(raw), &resp)` into a typed struct). The JSON-returning shape is
the correct precedent for tag classification, not the narrative one — requirements.md's
Feasibility Risks section already flags that `GenerateSessionCompletionNarrative`
"generates prose, not a tag decision" and anticipates needing "a new headless helper."

**Verdict: Recommended — add a new `GenerateSessionTags`-style function in
`session/headless/features.go`** following the existing JSON-unmarshal-response pattern
(new `FeatureKey`, system prompt instructing the model to return a JSON array of tags or
an `Unclassified` sentinel, `pool.CallBlocking` + `json.Unmarshal`, cost callback wired
through exactly like `GenerateSessionCompletionNarrative`'s `func(usd float64) { cost = usd }`
for the cost-tracking Observability Requirement). This is new code, but it's a same-shaped
sibling of existing code, not a fresh design.

**Ent schema for user-editable tagging rules.** `session/ent/schema/approvalrule.go` is a
flat field list (no edges) with `rule_id`/`priority`/`enabled`/`source`/timestamps common
fields, plus tool-use-specific fields (`tool_pattern`, `command_pattern`,
`required_flags`, etc.) and JSON-array fields for structured criteria. A tagging rule
needs the same common fields (id/name/priority/enabled/source/timestamps) but a
different payload: match fields (name/branch/path/program regex patterns, dependent-tag
condition) and a tag-output field, not a decision/risk-level output.

**Verdict: Viable — new ent schema, same pattern, not a drop-in shared table.** Matches
requirements.md's own Rabbit Holes conclusion ("may not be a drop-in reuse of the
existing ent types"). Create a sibling schema (e.g. `session/ent/schema/tagrule.go`)
copying `approvalrule.go`'s common-field shape and CRUD/provenance conventions, rather
than trying to force tag-output rules into `ApprovalRule`'s decision/risk-level columns.

## Summary Table

| # | Question | Verdict |
|---|---|---|
| 1 | External rule-engine library (grule/expr/cel) | **Not recommended** — violates explicit "generalize, don't parallel-build" direction |
| 2 | SaaS/managed classification API for LLM fallback | **Not recommended** — self-hosted `headless.PoolClient` + Haiku already fits and is integrated |
| 3a | Graph-cycle-detection library for fixpoint termination | **Not recommended** — hand-roll iteration cap + fired-rule dedup |
| 3b | Content-hash function (sha256 vs fnv vs murmur3) | **Recommended: `crypto/sha256`** — matches repo's existing cache-key convention |
| 4a | Fork `pr_status_poller.go`/`worktree_pr_poller.go` | **Adapt the Start/Stop/pollLoop shape**, don't fork the file (domain logic is irrelevant) |
| 4b | New structured-tag-output headless helper | **Recommended** — new function mirroring the existing JSON-unmarshal `CallBlocking` pattern, not the prose-narrative one |
| 4c | Ent schema for tagging rules | **Viable** — sibling schema to `approvalrule.go`, not a shared table |
