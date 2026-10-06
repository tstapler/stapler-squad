# Research: Tech Stack for `pkg/threatscan/`

## 1. Go stdlib — what applies

- **`regexp` (RE2 engine)** — the only package needed. It is already the tool used by every
  existing "named pattern list" in this codebase (see §2). RE2 has no backreference/lookahead
  support, but the requirements doc's fuzzy-gap technique (`\s+(?:\w+\s+)*` between key tokens)
  needs neither — it's plain alternation/repetition, fully expressible in RE2 syntax.
  - Use `regexp.MustCompile` at package-init time (var block), matching the existing
    `secretPatterns`/`htmlTagRe` convention — never compile per-call.
  - `(?i)` inline flag for case-insensitivity (already used in `secretPatterns`, e.g.
    `aws_access_key_id`, `generic_bearer`).
  - `regexp.Regexp.MatchString` is sufficient for a boolean "did scope X match" result; no need
    for `FindString`/`FindAllString` since the requirements doc explicitly says never log the
    matched substring — only the pattern ID (`session/backlog_review.go:47-48` already does
    this: `fmt.Errorf("secret pattern detected: %s", p.name)`, no matched text in the message).
- **`strings`** — for any pre-normalization (e.g. collapsing whitespace runs, lowercasing) if
  a scope wants to be more bypass-resistant than raw regex allows without leaning on `(?i)`
  alone. Not required for a first cut.
- No need for `unicode`/homoglyph-normalization packages unless a later iteration wants to
  defend against Unicode confusables — out of scope per the requirements doc (regex-only,
  matching Hermes reference).

## 2. Already-vendored dependencies in `go.mod` — what applies

Checked `/home/tstapler/Programming/stapler-squad/go.mod` in full. Nothing beyond stdlib
`regexp` is relevant:

- **No** existing HTML-parsing dependency is imported for this purpose — `golang.org/x/net`
  is present (indirect, pulled in via other deps) and includes `golang.org/x/net/html`, which
  *could* do structural (tag-tree) hidden-element detection (e.g. `style="display:none"`,
  `hidden` attribute, zero-size elements) more robustly than a regex. But the existing
  `sanitizeField`/`htmlTagRe` precedent (`session/backlog_context.go:12`) uses a bare
  `<[^>]+>` regex, not an HTML parser, and the requirements doc explicitly asks for
  "a small, dependency-free package (stdlib `regexp` only, matching the existing
  `sanitizeField` pattern already in the codebase)." Recommendation: stay regex-only for
  hidden-HTML-element injection too (e.g. match on `display:\s*none`, `visibility:\s*hidden`,
  `<!--` comment wrapping instruction-like text) rather than importing an HTML parser.
- **No** bluemonday, no sanitize/policy libraries, no existing regex-pattern-registry package
  to extend. `pkg/threatscan/` is a genuinely new package with no existing Go abstraction to
  build on other than the `regexp` convention itself.
- `github.com/stretchr/testify` (already a direct dep, used pervasively in `_test.go` files)
  is the natural assertion library for `pkg/threatscan/scan_test.go`, consistent with the rest
  of the codebase.

## 3. Prior art in this codebase: "named regex list, scan, return matches"

Two examples, both in `session/`, both stdlib-`regexp`-only:

### `session/backlog_review.go:20-52` — `secretPatterns` / `RunPreGateSecurityCheck`

```go
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"aws_access_key_id", regexp.MustCompile(`(?i)aws_access_key_id`)},
	{"AKIA_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"private_key_pem", regexp.MustCompile(`-----BEGIN .{0,30}PRIVATE KEY-----`)},
	// ... 9 more
}

func RunPreGateSecurityCheck(diff string) error {
	for _, p := range secretPatterns {
		if p.re.MatchString(diff) {
			return fmt.Errorf("secret pattern detected: %s", p.name)
		}
	}
	return nil
}
```

Shape to reuse for `pkg/threatscan/`: anonymous-struct slice of `{name string; re *regexp.Regexp}`,
package-level `var` block, `MatchString` boolean scan, error message carries only the pattern
name — never the matched text. This is the single strongest structural precedent in the repo
and is exactly what the requirements doc's acceptance criterion 4 ("no additional inline
regexes for injection detection introduced elsewhere") wants `pkg/threatscan/` to formalize
and generalize (adding a `scope` field alongside `name`/`re`).

### `session/backlog_context.go:12-27` — `htmlTagRe` / `sanitizeField`

```go
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func sanitizeField(s string, maxLen int) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	if len(s) > maxLen {
		s = s[:maxLen] + " [truncated]"
	}
	return s
}
```

Simpler single-pattern precedent; establishes the "package-level compiled `var`, plain
`regexp` stdlib, no external deps" convention that predates `secretPatterns`. `sanitizeField`
is called from every prompt-builder the requirements doc names as a strict-scope call site
(`session/backlog_context.go`, `session/backlog_review.go`, `session/backlog_lifecycle.go`),
confirming those are the right insertion points for `threatscan.Scan(..., ScopeStrict)` calls
— they already funnel every relevant field through one function today.

**Naming mismatch to flag for the plan phase**: the requirements doc's own "In scope" section
already corrects itself — it says the originating issue calls the prompt builders
`BuildReviewPrompt`/`BuildTriagePrompt` but the actual names in `session/backlog_review.go`
and `session/backlog_triage.go` are `BuildHeadlessReviewPrompt`, `BuildHeadlessTriagePrompt`,
`BuildHeadlessRetriagePrompt`. Confirmed these three names exist via grep of
`session/backlog_review.go`/`session/backlog_triage.go` (not re-verified line-by-line in this
research pass — plan phase should grep them directly before wiring calls).

No other `[]struct{ name ...; re *regexp.Regexp }`-shaped pattern registry exists anywhere
else in the Go tree (only `secretPatterns` and the single `htmlTagRe`) — confirming acceptance
criterion 4's premise that today there is exactly one such registry, and `pkg/threatscan/`
would be the second, purpose-built one.

## 4. Community-recommended approaches (2025/2026) for regex-based prompt-injection detection

- **Regex/pattern matching is explicitly positioned as a *first line of defense*, not a
  complete solution, across the current tooling landscape.** OWASP's LLM Top 10 (LLM01:2025
  Prompt Injection) and the associated GenAI Incident Database writeups consistently frame
  keyword/pattern filters as catching only "classic"/unsophisticated injection phrasing
  ("ignore previous instructions", "you are now DAN", "disregard your system prompt") while
  explicitly noting they are trivially bypassed by paraphrase, translation, encoding
  (base64/ROT13/leetspeak), or multi-turn/indirect injection (payload hidden in a document,
  tool result, or webpage the model later reads) — which matches this project's own "context"
  scope use case (external comments, CLAUDE.md/AGENTS.md-style context files).
- **Known limitations of pure-regex approaches** (consistent across NVIDIA's `NeMo Guardrails`
  docs, Microsoft's Azure AI Content Safety "Prompt Shields" positioning, and Meta's
  `Prompt Guard` model card): regex/keyword filters (a) have a high false-negative rate against
  semantic paraphrase and non-English input, (b) require constant hand-maintenance of the
  pattern list as new jailbreak phrasing circulates, (c) can be tuned for a deliberately higher
  false-positive rate in high-stakes scopes (exactly what this project's "strict" scope
  requirements doc note calls out: "acceptable false-positive rate because the content is
  normally user-curated"), and (d) do not generalize to encoded/obfuscated payloads (base64,
  Unicode homoglyphs, zero-width characters) without additional normalization passes.
- **Statistical/classifier approaches** (Meta `Prompt Guard 2`, Microsoft `Prompt Shields`,
  `NVIDIA NeMo Guardrails`' jailbreak-detection rail, `Lakera Guard`) are the community's
  recommended complement — small fine-tuned classifier models (often DeBERTa-sized, not
  full LLMs) trained on injection/jailbreak corpora, run as a fast pre-filter before or
  alongside the main LLM call. They catch paraphrase/semantic variants regex misses, at the
  cost of a model dependency, latency, and non-zero false-positive/negative rates that need
  monitoring. This class of approach is explicitly **out of scope** for this item per the
  requirements doc ("A general-purpose ML/LLM-based injection classifier — regex/pattern-based
  only, matching the Hermes reference implementation's approach").
- **Defense-in-depth consensus**: every source surveyed (OWASP, Simon Willison's widely-cited
  "delimiters don't work reliably" writing on prompt injection, Anthropic's own tool-use/agent
  security guidance) converges on layering: (1) structural framing/delimiters around
  untrusted content — which this project already has via the "treat as inert data" envelope
  the requirements doc cites — (2) pattern/regex filtering as a fast, cheap, zero-dependency
  gate for known-bad phrasing (what `pkg/threatscan/` implements), and (3) least-privilege on
  what the model can *do* even if injected (tool scoping, human-in-the-loop for destructive
  actions) as the backstop when (1) and (2) both fail. No source treats regex filtering alone
  as sufficient, which matches this item's framing as one layer, not the whole defense.
- **Fuzzy-gap regex technique** (`\s+(?:\w+\s+)*` between key tokens, as specified in the
  requirements doc for the Hermes `tools/threat_patterns.py` reference) is a recognized,
  low-cost mitigation specifically against *filler-word insertion* bypasses ("ignore all the
  previous silly little instructions") — a known easy bypass class for naive
  `"ignore previous instructions"` literal-string or simple-regex matching. It does not
  address paraphrase, synonym substitution, translation, or encoding-based bypasses; those
  remain unaddressed by design in a regex-only scanner, consistent with this item's explicit
  scoping decision to leave ML classification out of scope.

## 5. Recommendation summary for the plan phase

- Build `pkg/threatscan/` as a standalone package: `var patterns = []struct{ id string; scope
  Scope; re *regexp.Regexp }{...}`, one exported `Scan(s string, scope Scope) []ThreatResult`
  (or single best-match variant per requirements §5), stdlib `regexp` only, `testify` for
  tests — mirroring `secretPatterns`/`RunPreGateSecurityCheck` structurally but adding the
  `scope` dimension that registry lacks.
- No new `go.mod` entries required — this is a genuinely dependency-free addition, which
  matches the requirements doc's explicit "no new infrastructure required, pure logic" note.
- Do not reach for `golang.org/x/net/html` for hidden-HTML detection despite it being already
  vendored (indirect) — stay consistent with the existing regex-only `sanitizeField`
  convention unless a plan-phase spike shows regex genuinely cannot catch the target hidden-
  HTML patterns (e.g. `display:\s*none`, HTML comments wrapping instruction text).
