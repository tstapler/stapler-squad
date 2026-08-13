# Build vs. Buy: context-health-monitoring

Phase 2 research — Research Agent 6. Scope: per-session health signal from (1) tool-call
loop detection and (2) apology/confusion language detection, surfaced on the session card
over the existing ConnectRPC surface. Evaluated against `requirements.md`'s explicit
constraint: **no new external network calls** and reuse of `session/detection/*`.

## Option 1 — Existing OSS library/framework (agent-observability tooling)

Searched for a reusable Go primitive for "detect repeated/looping tool invocation
sequences" or "detect degraded/confused LLM output," and for whether agent-observability
OSS projects (Langfuse, AgentOps, Langtrace/OpenLLMetry) expose that as a library rather
than a hosted dashboard.

Findings:
- **Langfuse** (self-hostable, OSS) added a graph view in 2026 that visually renders agent
  loops as cycles, and ships a "Hallucination evaluator" — but both are features of its
  ingested-trace product, requiring the app to instrument LLM/tool calls via its SDK and
  ship traces to a Langfuse service (self-hosted Postgres/ClickHouse stack), which then
  runs an *LLM-as-judge* evaluation. There is no standalone loop/confusion-detection
  library extractable from it — the detection logic lives inside the ingestion+eval
  pipeline, coupled to its trace schema.
- **AgentOps** and the wider "agent observability" category (per aimultiple.com/Galileo/
  Latitude 2026 roundups) are positioned the same way: SDK-instrumented session replay
  and anomaly detection surfaced on a hosted or self-hosted dashboard, not an importable
  detection primitive.
- The closest real precedent for the *algorithm* is **TokenCircuit** (a LangGraph
  `pre_model_hook`), which combines structural tool-name+argument-type matching with
  Jaccard similarity over tool output to catch both exact and paraphrased loops. This
  validates the *design* (structural match + optional fuzzy layer) but it's Python,
  LangGraph-specific middleware — not a Go library, and not adoptable as a dependency.
- No Go package (searched `pkg.go.dev`, GitHub topics `agent-loops`, `tool-calling`)
  exposes a generic "N-similar-tool-calls-in-a-row" or "confused output" detector as an
  importable function; the few Go agent frameworks with loop detection (e.g. an
  `agent-protocol/adk-golang` `LoopDetector`) bury it as private framework-internal logic
  in their own agent-loop runner, not a standalone library.

**Pros**: would offload algorithm design and testing to a maintained project.
**Cons**: every candidate is shaped as "instrument my LLM call graph, ship traces to a
service" (even self-hosted Langfuse means standing up a new backend + datastore), not
"call a function against a byte slice already in my process." None operate on raw tmux
PTY output or Claude-Code-CLI-specific transcript text — they all assume you're the one
making the LLM API calls, which stapler-squad is not (it observes a black-box CLI's
terminal output). Adopting any of them would mean building an *adapter* from tmux output
to their trace schema, which is strictly more code than the two heuristics themselves.

**Verdict: Not recommended.**

## Option 2 — SaaS/managed "agent health" API

Is there a hosted API that ingests streamed terminal output and returns a health score?

Findings: the products above (Langfuse Cloud, AgentOps, Galileo, Latitude, Helicone) are
exactly this shape for LLM-call traces, and some (Galileo, Confident AI) market
hallucination/quality scoring APIs. But `requirements.md`'s Non-functional Requirements
section is explicit: *"no new external network calls introduced by this feature"* — this
alone rules out any SaaS call per output chunk, independent of cost or latency, since it
would violate a stated NFR, not just be suboptimal. Restating rather than re-litigating:
this option is out of scope for v1 by requirement, not by this analysis's judgment call.
It would also fail the <5ms-per-chunk performance SLO by construction — a network round
trip per PTY chunk is 2–3 orders of magnitude slower than that budget regardless of
which vendor.

**Verdict: Not recommended (ruled out by explicit NFR, not re-evaluated further).**

## Option 3 — LLM-generated bespoke code vs. a small tested library, per algorithm

### 3a. Bounded-window "are the last N tool calls similar" comparator

The comparison needed is narrow: given the last N tool-call records (name + normalized
args) already flowing through the pipeline, decide if ≥3 in a row are "the same call."
Two implementation choices:

- **Hand-rolled ring buffer + exact/normalized-argument match.** A fixed-size ring
  (mirroring the existing `eventRing` pattern in `session/detection/events.go`) storing
  the last N `(toolName, normalizedArgsHash)` pairs, with equality compared via exact
  match after simple normalization (trim whitespace, canonicalize path separators, drop
  timestamps/nonces from args). This is a bounded, allocation-free O(N) comparison —
  well within the <5ms/chunk SLO.
- **Pull in a similarity library** (e.g. `github.com/agext/levenshtein` or
  `github.com/sergi/go-diff`, both of which are *already present in `go.mod` as indirect
  transitive dependencies* — not used by any `.go` file in this repo directly, confirmed
  via `grep -rn "agext/levenshtein\|sergi/go-diff" --include="*.go" .` returning no
  matches — so adopting one would promote it from indirect to direct, not add a wholly
  new dependency to the module graph) for fuzzy matching of argument strings that differ
  by a token or two (e.g. `Edit(file.go, old="foo")` vs. `Edit(file.go, old="foo ")`).

**Recommendation**: start with exact/normalized match (no library). The correctness risk
of hand-rolling *exact* equality after normalization is low — it's string comparison, not
a novel algorithm — and it directly reuses the ring-buffer idiom already established in
this package. Reserve `agext/levenshtein` (already in the dependency graph, MIT-licensed,
small, stable, single-purpose) as a v2 escalation *only if* manual review during Phase 6
shows false negatives from near-identical-but-not-identical argument strings (e.g. an
edit whose old-string differs by trailing whitespace). Don't pull in `go-diff` — it's a
line-diff library (Myers diff for text documents), a heavier and less-fitting tool for
comparing short tool-argument strings than a Levenshtein distance/ratio.

### 3b. Keyword/regex list matching for apology detection

This is unambiguously bespoke code, no library needed: it's a fixed list of
phrases/regexes ("I apologize", "I made a mistake", "let me try again", "sorry for the
confusion", etc.) matched against normalized text, following the *exact* existing
`StatusPattern`/`PatternSet` idiom in `session/detection/detector.go` and
`session/detection/pattern_set.go` (see `getDefaultPatterns()`'s `Error`/`NeedsApproval`
pattern groups for the established style: named regex + description + priority,
compiled once at construction). A generic NLP/sentiment library would be strictly worse
here — heavier, slower, harder to tune per-thresholds, and solving a more general problem
(sentiment/intent classification) than what's needed (does this known Claude-Code-CLI
phrasing appear ≥K times). `requirements.md`'s own Rabbit Holes section already flags this
as "a single well-scoped heuristic module, not a general NLP problem" — this option
confirms that framing rather than revisiting it.

**Verdict (3a): Viable to build hand-rolled now; treat a similarity library as a
follow-on escalation if manual review demands it, not a day-one dependency.**
**Verdict (3b): Build bespoke — Recommended, not even close.**

## Option 4 — Fork/extend `session/detection`'s existing `PatternSet`/`StatusDetector` machinery

This is the strongest option and the one the requirements doc already leans toward
("Alternatives Considered": prefer extending the existing pipeline over a second regex
pass over raw scrollback).

What already exists and is directly reusable:
- **`PatternSet`** (`session/detection/pattern_set.go`) is exactly the right shape for
  apology-language detection: an immutable, pre-compiled `[]*regexp.Regexp` set matched
  against normalized text in priority order, returning a matched pattern name + human
  description. Adding a `ContextConfusion` category to `StatusPatterns` (in
  `session/detection/dtypes`) and a corresponding regex group in `getDefaultPatterns()`
  is additive, not a new subsystem — same compile-once-no-lock design
  (`atomic.Pointer[PatternSet]` in `StatusDetector`), same config-file override path
  (`NewStatusDetectorFromFile`/`LoadPatterns`, matching the `config/` JSON-override
  convention this project's requirements call for).
- **`eventRing`/`DetectionEventSink`** (`session/detection/events.go`,
  `session/detection/event_sink.go`) is a fixed-capacity, mutex-guarded ring buffer
  already recording `DetectionEvent{SessionID, Timestamp, MatchedPattern,
  MatchedCategory, ResultStatus, TailSnippet}` per detection call, at up to 50 calls/
  detection cycle. The apology-count-over-window heuristic is a straightforward
  aggregation over this existing ring (count events matching the new confusion category
  within the last K events/duration) — no new goroutine, no new lock, satisfying the
  NFR against new per-session goroutine/lock contention.
- **Tool-call granularity for loop detection**: confirmed via
  `server/services/hook_receivers.go` (`HandlePreToolUse`/`HandlePostToolUse`,
  registered at `/api/hooks/pre-tool-use`, `/api/hooks/post-tool-use`) and
  `session/artifacts/jsonl.go`/`session/tokens/jsonl_types.go` (which already parse
  `tool_use_id`-keyed content blocks, including typed `bashInput` argument structs) —
  this answers one of `requirements.md`'s Open Questions directly: **yes**, per-tool-call
  data with tool name + structured arguments already flows through the existing hook/
  JSONL ingestion path; loop detection does not need new PTY instrumentation, it needs a
  small ring buffer consuming this already-parsed stream (a separate, smaller data path
  than the PTY-text `PatternSet`, but already present in this codebase, not net-new
  collection).

**Pros**: zero new subsystems — extends two already-tested, already-concurrency-safe
components (`PatternSet` for text patterns, `eventRing`-style ring buffer for windowed
counting) that this project's own maintainers already trust enough to run at 1 Hz across
dozens of sessions. Matches the project's own stated preference. Directly answers two of
the three Open Questions in `requirements.md` (tool-call granularity: yes, via hook JSONL;
no new PTY instrumentation needed).
**Cons**: none identified that aren't shared by any bespoke-code option — the two
heuristics still need new regex content (confusion phrases) and new comparison logic
(tool-call similarity), but that's true no matter which structural home they live in.

**Verdict: Recommended.** Fork/extend `session/detection`'s `PatternSet` (apology
detection) and add a small new ring-buffer-style comparator consuming the existing
hook/JSONL tool-call stream (loop detection), rather than introducing a new detection
subsystem.

## Summary Table

| Option | Verdict |
|---|---|
| 1. OSS agent-observability library/framework | Not recommended — all are trace-ingestion + hosted/self-hosted-dashboard shaped, none expose a Go-importable detection primitive, none operate on raw tmux PTY output |
| 2. SaaS/managed agent-health API | Not recommended — ruled out by requirements.md's explicit "no new external network calls" NFR and the <5ms/chunk SLO, not by further judgment |
| 3a. Loop comparator: bespoke ring buffer vs. similarity library | Viable to build hand-rolled (exact/normalized match) now; escalate to `agext/levenshtein` (already an indirect dep) only if Phase 6 manual review shows false negatives |
| 3b. Apology/keyword matching | Recommended — bespoke, no library, matches existing `PatternSet` idiom |
| 4. Fork/extend `session/detection`'s `PatternSet`/`StatusDetector`/ring-buffer machinery | **Recommended — strongest option**, zero new subsystems, reuses proven concurrency-safe components |

**Overall recommendation**: Build, entirely bespoke, by extending
`session/detection/pattern_set.go` + `session/detection/events.go`'s existing machinery
and the hook/JSONL tool-call stream already parsed in `session/artifacts/jsonl.go`. No
OSS library or SaaS product fits this narrow, PTY/transcript-specific, no-network-calls
problem shape — confirming `requirements.md`'s own "Alternatives Considered" framing
rather than overturning it.
