# Research: Build vs. Buy for `pkg/threatscan`

Question: should the extracted package hand-roll regex patterns (as the issue
proposes) or adopt/wrap an existing library for (1) secret detection, (2)
prompt-injection detection, (3) fuzzy/filler-word-tolerant matching?

**Verdict: build from scratch, as the issue proposes. No new dependency for
any of the three sub-problems.**

## 1. Secret detection — existing OSS Go libraries

Checked: [gitleaks/gitleaks](https://github.com/gitleaks/gitleaks) (MIT
license, Go-native — its `detect` package is importable, e.g.
`github.com/zricethezav/gitleaks/v8/detect`, confirmed on
[pkg.go.dev](https://pkg.go.dev/github.com/zricethezav/gitleaks/v8/detect)).
detect-secrets and trufflehog were not further evaluated because they are
Python/CLI-first with no maintained Go-native library surface.

Does not fit this repo's constraints:

- **License**: MIT — fine, not a blocker.
- **API leaks the matched value by contract.** gitleaks' `detect.Finding`
  struct returns the actual matched secret text (`Secret` field) as its
  primary output — that is the tool's whole point (showing you what leaked).
  Requirement AC 2 (`session/backlog_review.go` and its migrated form) is the
  inverse: **never include the matched value** in any returned error, log
  line, or `ThreatMatch` field, verified by
  `TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`.
  Adopting gitleaks would mean importing an API whose entire contract is
  "return the secret" and then discarding/redacting that field at every call
  site — more integration surface than the 12 regexes it would replace.
- **Config model mismatch.** gitleaks rules are TOML-defined
  (`gitleaks.toml`), loaded via a config/allowlist system built for scanning
  git history and file trees. This package's shape — a `Scope`-tagged,
  named-ID Go struct list scanning an in-memory diff/text string — doesn't
  map cleanly onto that model without building a translation layer.
- **Dependency weight.** gitleaks' `detect` package alone pulls ~34 imports
  (git-history walking via go-git, TOML parsing, syntax highlighting via
  chroma, etc.) for capability this package doesn't need (it scans a diff
  string / backlog field text, not a git repository).
- **Maintenance surface trade direction is backwards.** The issue frames this
  as trading "12 hand-maintained regexes" for "a maintained pattern set" —
  but gitleaks' own rule set (hundreds of vendor-specific TOML patterns) is
  significant overkill for a scope that's explicitly bounded to "AWS key,
  GitHub PAT, OpenAI key, Stripe, Slack, npm, SendGrid, Twilio, bearer token,
  DB URL" (AC 4) — i.e., 12 already-known, already-correct regexes. There is
  no coverage gap gitleaks would close here; it would only add surface area.

## 2. Prompt-injection detection — existing OSS Go libraries

Searched current ecosystem (Aug 2026). Findings:

- [mdombrov-33/go-promptguard](https://github.com/mdombrov-33/go-promptguard)
  — the only Go-native prompt-injection library found. Single-maintainer,
  unestablished project; combines pattern matching with statistical
  (entropy/perplexity) analysis and optional LLM-based judging. Not a
  reasonable dependency to take on for a synchronous pre-review-gate check —
  no track record, and the "optional LLM judge" mode would add latency/cost
  this call site (`session/review_gate.go:277`, called before every review
  gate spawn) doesn't have budget for.
- Other results were non-Go: StackOne Defender, Bastion-RAG (Go binary/REST
  service, not an importable library), Praetorian's Augustus (probe-based
  red-team scanner, not a detection library), and two arXiv papers (PIShield,
  SCOUT) describing LLM-feature-based classifiers that require model
  inference access, not a call over local text.
- **Industry-wide, this is not a solved, off-the-shelf problem** — commercial
  guardrail products (as of the "7 Best Prompt Injection Detection Tools"
  roundup and getmaxim.ai's 2026 guide surveyed during this search) still
  lean on pattern/heuristic layers as a first line of defense, with ML/LLM
  classifiers as a secondary layer, not a replacement. A regex heuristic
  layer is the industry-standard *first* line, which is exactly AC 6's scope
  (classic injection phrasing, role-play/identity-hijack, HTML injection,
  exfiltration signals) — this item isn't reinventing something that has a
  mature off-the-shelf answer.

## 3. Fuzzy/approximate matching for AC 7 (filler-word bypass resistance)

Checked `go.mod`/`go.sum` first: `github.com/agext/levenshtein` is present,
but only as a **transitive, indirect** dependency —
`go mod why github.com/agext/levenshtein` resolves it through
`session/ent/enttest → entgo.io/ent/dialect/sql/schema → ariga.io/atlas →
hashicorp/hcl/v2` (HCL's "did-you-mean" typo suggester for schema/config
parsing). It is not something this repo has chosen for text-matching use, and
it is not already available as a direct import without adding an explicit
`require`.

More importantly, **Levenshtein-distance libraries solve the wrong shape of
problem for AC 7.** Edit-distance matching (agext/levenshtein,
lithammer/fuzzysearch, goagrep, fuzzymatch-go — all surveyed) is built for
*character-level* typo tolerance within a single token or short string
("recieve" ~ "receive"). AC 7's requirement is *phrase-level filler-word
insertion* — "ignore all the previous silly instructions" vs. "ignore
previous instructions" — where 1-3 whole words are inserted between two key
tokens. Feeding that pair into a Levenshtein comparison produces a large edit
distance (each inserted word costs roughly its full character length) that
doesn't cleanly threshold against genuinely unrelated text without heavy
tuning, and per-word tokenization workarounds to make edit-distance libraries
handle word-level gaps would end up reimplementing the regex approach anyway
with more moving parts.

The precise, idiomatic fit — and what the issue already proposes — is a
**bounded filler-gap regex**: a pattern of the shape
`key1\s+(?:\w+\s+){0,3}key2` between each pair of key tokens (e.g.
`(?i)ignore\s+(?:\w+\s+){0,3}previous\s+(?:\w+\s+){0,3}instructions`), which
directly encodes "up to N filler words between these two anchors" — the exact
requirement — with zero new dependencies, using only `regexp` (stdlib,
already imported in `session/backlog_review.go` and throughout the repo).

## 4. Net tradeoff given "low-effort, high-maintainability" framing

The issue explicitly scopes this as a bounded structural refactor (AC 4: 12
existing patterns with equivalent coverage; AC 6: "at minimum" 4 new pattern
categories) — not an open-ended threat-detection feature (explicitly listed
as out of scope). Against that framing:

- Every library option surveyed above would **increase**, not decrease,
  long-term maintenance burden: gitleaks trades regex upkeep for TOML-config
  + dependency-upgrade upkeep and a value-leaking API that must be wrapped;
  go-promptguard trades it for a dependency on an unestablished single-author
  project; a Levenshtein library solves a different problem than AC 7 asks
  for and would need workarounds to fit.
- The repo already has a working precedent for this exact shape:
  `pkg/classifier/classifier.go` uses plain stdlib `regexp` with zero
  external pattern/matching libraries for its own rule-based classification,
  confirming hand-rolled regex is this codebase's established convention for
  this class of problem (see `.claude/rules` review conventions and AC 3's
  request to validate structure against `pkg/classifier`).
- The "never log the matched value" contract (AC 2) is idiosyncratic to this
  codebase's frontend/verdict-rendering path and is not a contract any
  surveyed library was designed around — adopting one would require wrapping
  it to enforce a guarantee it doesn't natively provide, which is strictly
  more code than the regexes it would replace.

**Recommendation for the plan phase:** implement `pkg/threatscan` with
hand-rolled `regexp`-based patterns exactly as the issue proposes (mirroring
`pkg/classifier`'s existing conventions), including a bounded filler-gap
regex idiom (`key1\s+(?:\w+\s+){0,N}key2`) for AC 7's fuzzy-bypass
requirement. No new `go.mod` dependency is warranted for any of the three
sub-problems evaluated.
