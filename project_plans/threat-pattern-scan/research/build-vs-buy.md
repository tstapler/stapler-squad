# Research: Build vs. Buy for `pkg/threatscan/`

Complements `research/stack.md` (which already concludes stdlib `regexp` is technically
sufficient and mirrors `secretPatterns`). This doc answers the narrower question: should the
*pattern content and detection approach* be built from scratch, sourced from an existing Go
library, sourced from a hosted API, or forked from the Hermes Agent reference — rather than
just "what Go stdlib/deps are available."

## 1. Existing Go OSS libraries for prompt-injection detection

Searched for Go-native equivalents to Rebuff, LLM Guard, Vigil, NeMo Guardrails (all
Python-only — confirmed no Go port of any of them exists). Found one small Go-native project:

- **`github.com/mdombrov-33/go-promptguard`** ([pkg.go.dev](https://pkg.go.dev/github.com/mdombrov-33/go-promptguard/detector), [github](https://github.com/mdombrov-33/go-promptguard)) —
  a `detector` package with a `RoleInjectionDetector` (special tokens, XML/HTML role-switch
  tags, role-switching phrases) and a `TokenAnomalyDetector` (unusual character distributions,
  Unicode/zero-width anomalies). Structurally the closest match to this item's scope.
  - **Pros**: Go-native, no subprocess/FFI boundary, MIT-style OSS, conceptually close to what
    `pkg/threatscan/` needs (pattern-based, not ML).
  - **Cons**: single/small-maintainer project, no evidence of production adoption or security
    audit, would become a new `go.mod` dependency for a security-relevant code path (higher bar
    than a typical library add — a vulnerability or abandonment risk directly undermines the
    injection defense itself), and its pattern set is not verified against this project's
    specific scope semantics (`all`/`context`/`strict`) or its "pattern ID for logging, never
    log the matched substring" requirement.
  - Other hits (an "Extensible Go microkernel for LLM guardrails," an "AI API Relay Security
    Audit Tool," Praetorian's "Augustus") are adjacent tooling — proxy/audit binaries or
    guardrail *frameworks*, not embeddable pattern-scan libraries — not a fit for "one function
    called inline before building a prompt."
- **Verdict: Not recommended.** The requirements doc explicitly asks for "a small,
  dependency-free package (stdlib `regexp` only ... over any new abstraction layer)." Taking on
  an unaudited, low-adoption external dependency for a security control is the wrong trade even
  before weighing that its pattern set and scope model don't already match this item's spec —
  it would need to be read, vetted, and probably partially rewritten anyway, at which point
  building from scratch is less total risk, not more.

## 2. SaaS / managed prompt-injection-detection APIs

Evaluated the two most cited hosted options — Lakera Guard and Azure AI Content Safety /
Prompt Shields — against this item's actual call pattern: synchronous, runs before *every*
work-session/review/triage prompt build, on backlog content that may be private.

| Dimension | Lakera Guard | Azure Prompt Shields |
|---|---|---|
| Latency | ~30-50ms cloud API; ~10-15ms self-hosted enterprise tier | ~100-300ms |
| Pricing | Request-based; "a few thousand dollars/month" at 1-5M req/month | Per-1,000-text-record billing; free tier 5,000 records/mo, then usage-billed |
| Data residency | Content leaves the process boundary to a third-party API (or a paid self-hosted deployment) | Content leaves to Azure by default |

- **Pros**: Both catch semantic/paraphrase variants a regex scanner structurally cannot (this
  is their whole value proposition vs. pattern matching); no in-repo pattern-maintenance burden.
- **Cons**: 
  - **Latency**: this scan runs synchronously on the hot path of *every* prompt build
    (work-session start, every review, every triage/retriage) — even Lakera's best-case ~30ms
    cloud latency is pure overhead added to each of those calls, for a check that a
    zero-network regex scan does in microseconds. Azure's 100-300ms is a much harder sell on a
    per-prompt-build gate.
  - **Data residency**: backlog titles/descriptions/AC text are user-authored and the
    requirements doc treats them as sensitive enough to warrant a dedicated in-repo scanner —
    routing that same content to a third-party SaaS API for every prompt build is a materially
    larger exposure surface than keeping it in-process, and stapler-squad has no existing
    external-API-call pattern for backlog content today (checked: no HTTP client wired into
    `session/backlog_review.go`, `backlog_triage.go`, or `backlog_context.go`).
  - **Cost**: ongoing per-request billing for what the requirements doc calls a "quick win, no
    new infrastructure required" item is a scope mismatch — this would add a recurring
    operational cost and a new external-service dependency (auth, availability, rate limits) to
    a feature explicitly scoped as "pure logic."
  - **New failure mode**: a synchronous gate that depends on a third-party API's uptime means
    an outage or rate-limit either blocks all prompt builds or requires a fail-open/fail-closed
    policy decision that doesn't exist today.
- **Verdict: Not recommended.** Wrong shape for this call site on all three axes (latency, data
  residency, cost/scope). Worth reconsidering only if a future item explicitly wants
  semantic/paraphrase-level detection as a distinct, separately-scoped defense layer — not as a
  drop-in replacement for this item's regex scanner.

## 3. LLM-generated/hand-rolled regex list vs. a maintained pattern corpus

- **No single canonical "injection regex corpus" exists to mirror wholesale** the way, say, a
  secrets-detection tool can mirror gitleaks' rule file. The closest analogues are:
  - **Vigil-LLM's YAML signature files** (`vigil-llm`, Python project) — human-curated
    regex/keyword signatures for jailbreak/injection phrasing, openly licensed. Useful as a
    *reference to read* for phrasing coverage (the "ignore previous instructions" family, DAN-
    style role-play triggers, system-prompt-override phrasing) even though the project itself
    is Python and out of scope to depend on.
  - **OWASP LLM01:2025 (Prompt Injection)** writeups and the GenAI Incident Database — catalog
    known injection phrasing patterns and bypass classes (filler-word insertion, encoding,
    translation) rather than shipping a regex file, but are a good correctness cross-check for
    "did we cover the well-known families."
  - The Hermes Agent `tools/threat_patterns.py` reference this item is explicitly modeled on
    (see §4) — the requirements doc already extracts its structural properties (three scopes,
    fuzzy-gap technique, pattern-ID-only logging) but the actual pattern strings are not present
    in this environment to copy directly.
- **Correctness risk framing**: a hand-rolled/LLM-drafted pattern list's risk is almost entirely
  **false negatives from missed phrasing variants**, not false positives from bad regex syntax —
  the mechanics (RE2 syntax, fuzzy-gap `\s+(?:\w+\s+)*`) are simple and easy to get syntactically
  right; the risk is failing to anticiate a bypass phrasing a maintained corpus would have
  already catalogued. This is inherent to *any* static pattern list, maintained-corpus or not —
  regex-based detection is explicitly a "known-bad phrasing" net, not a semantic one (see
  `research/stack.md` §4's OWASP/NeMo Guardrails sourcing on this exact limitation).
- **Verdict: Viable, with a mitigation.** Hand-rolling is the only option consistent with the
  requirements doc's explicit scope (no new dependency, mirror the Hermes reference's
  structure), but the plan phase should treat OWASP LLM01 and Vigil's signature *phrasing*
  (not code) as a correctness checklist when drafting the initial pattern set — cross-reference
  each of the five required families (classic injection, system-prompt override, role-play/
  identity hijack, deception, hidden-HTML injection) against those sources' documented phrasing
  variants before considering the pattern set complete, rather than drafting patterns from the
  requirements doc's one-line descriptions alone.

## 4. Is the Hermes Agent `tools/threat_patterns.py` reference available locally to fork?

Searched broadly for a local checkout to adapt directly rather than reimplementing from the
issue's prose description:

```
find ~/Programming ~/code ~/dotfiles -iname "*threat_patterns*"   → no results
find ~/Programming ~/code ~/dotfiles -maxdepth 4 -iname "*hermes*" → no results
grep -ril "hermes agent" ~/Programming ~/code ~/dotfiles           → no results
find ~/code/github.com -iname "*hermes*"                          → no results
```

`~/Programming` and `~/code/github.com` were both listed in full (`ls`) as a cross-check — no
directory name resembling "hermes" or a "threat_patterns" file exists anywhere under either
tree, or under `~/dotfiles`.

- **Verdict: Not available.** There is no local Hermes Agent checkout in this environment. The
  requirements doc's "Reference design" section is the only source of truth for this item —
  it already extracts and states the reference's structural properties explicitly (one shared
  registry, three scopes with documented semantics, the fuzzy-gap regex technique, pattern-ID-
  only logging). The plan/implementation phase should treat that section as the full spec
  rather than expecting to diff against or port actual source — there is nothing to fork. If
  the Hermes Agent repo is later made available (e.g. a teammate has a private checkout), it
  would be worth a follow-up comparison pass against the shipped pattern set, but this item
  should not block on locating it.

## Summary

| Option | Verdict |
|---|---|
| Stdlib `regexp`, hand-rolled, mirroring `secretPatterns` | **Recommended** |
| `go-promptguard` or similar small Go OSS lib | Not recommended |
| Hosted SaaS API (Lakera Guard, Azure Prompt Shields) | Not recommended |
| Hand-rolled patterns, cross-checked against OWASP LLM01 / Vigil phrasing (not code) | Viable (recommended mitigation for #1) |
| Fork Hermes Agent `tools/threat_patterns.py` directly | Not available — no local checkout found |

Net: nothing changes `research/stack.md`'s conclusion. Build `pkg/threatscan/` from scratch in
stdlib `regexp`, structured like `secretPatterns`/`RunPreGateSecurityCheck`, using the
requirements doc's reference-design section (not a forkable source file) as the spec, and treat
OWASP LLM01 / Vigil-style phrasing catalogs as a correctness checklist rather than a dependency.
