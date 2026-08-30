# Research: Technology Stack — PII Scanning

## Repo baseline

- **Go version**: `go 1.26.3` (`go.mod:3`). Module: `github.com/tstapler/stapler-squad`.
- Repo already carries ~136 direct `require` entries (connectrpc, ent, otel, etc.) — dependency-heavy overall, but the specific precedent this feature extends (`secret_scanner.go`) is **zero-dependency**: only `fmt` and `regexp` from stdlib (`server/services/secret_scanner.go:1-6`). No third-party regex, validation, or scrubbing library is pulled in for that file.

## Existing precedent: `secret_scanner.go`

72 lines total. Shape to mirror:
- `type secretPattern struct { Name string; Pattern *regexp.Regexp }`
- Package-level `var secretPatterns = []secretPattern{...}` with `regexp.MustCompile` literals, comments explaining each pattern's false-positive tradeoffs.
- `ScanForSecrets(text string) SecretScanResult` — caps input at first 4096 bytes, returns first match only (`Found bool`, `PatternName string`).
- `FormatSecretDenyMessage(patternName string) string` — user-facing message builder.

All matching logic is plain `regexp.MustCompile(...).MatchString(...)` — Go's stdlib `regexp` (RE2 engine, linear-time, no catastrophic backtracking risk). This is directly sufficient for email and SSN pattern matching; no external regex engine is needed or justified.

## What stdlib `regexp` can and can't do

- **Sufficient for**: email shape, SSN shape (`\d{3}-\d{2}-\d{4}` plus invalid-range exclusions), credit-card-number shape (13–19 digit sequences, optionally with brand-prefix regexes for Visa/Mastercard/Amex/Discover).
- **Not sufficient for**: Luhn checksum validation — that's arithmetic over the matched digit string, not a regex-expressible property. Needs a small Go function post-match, not a regex library.

### Luhn validation — implement in stdlib, do not add a dependency

Luhn's algorithm is ~10-15 lines of pure arithmetic over a digit string (sum digits right-to-left, double every second digit, subtract 9 if >9, check sum mod 10 == 0). No external package is needed — this matches the repo's zero-dep precedent in `secret_scanner.go` and avoids adding a dependency for trivial logic that would otherwise need vetting/pinning.

If a build-vs-buy decision favors a library anyway, community options exist but are all thin/unmaintained-risk wrappers around the same ~15 lines:
- [`github.com/phedde/luhn-algorithm`](https://pkg.go.dev/github.com/phedde/luhn-algorithm) — general-purpose Luhn generate/validate, not credit-card-specific.
- [`github.com/durango/go-credit-card`](https://github.com/durango/go-credit-card) / [`github.com/josue/go-credit-card`](https://github.com/josue/go-credit-card) — Luhn + brand detection + expiry/CVV validation (expiry/CVV fields are irrelevant to this feature's scope, which only ever sees a bare number in text).
- [`github.com/retgits/creditcard`](https://pkg.go.dev/github.com/retgits/creditcard) — similar scope to durango's.
- [`github.com/sgumirov/go-cards-validation`](https://github.com/sgumirov/go-cards-validation) — same category.

**Recommendation**: implement Luhn inline in `pii_scanner.go` as an unexported helper (`func isValidLuhn(digits string) bool`). None of the above libraries are maintained by orgs the project already trusts, add a dependency for something trivially testable in isolation, and all carry more surface area (card-brand tables, expiry logic) than this feature needs.

## OSS PII-detection libraries surveyed (broader scope than just Luhn)

For context on whether a full PII-scanning library should be adopted instead of extending the hand-rolled pattern list (build-vs-buy is a planning-phase call, not decided here):

| Library | Notes |
|---|---|
| [`aavaz-ai/pii-scrubber`](https://github.com/aavaz-ai/pii-scrubber) | Extensible Go lib to identify/mask PII (credit card, email, SSN) via customizable regex scrubbers — closest in shape to what this feature needs, but pulls in its own pattern/config abstraction that would compete with the existing `secretPattern` shape rather than extend it. |
| [`vsemashko/go-pii-sanitizer`](https://pkg.go.dev/github.com/vsemashko/go-pii-sanitizer/sanitizer) | `ContentPattern` type pairs a regex with an optional validation func (e.g. Luhn) — same conceptual shape this feature will hand-roll (regex + validator callback). |
| [`intMeric/pii-extractor`](https://github.com/intMeric/pii-extractor) | Multi-country PII extraction (phone, SSN, ZIP) with dedup and optional LLM validation — heavier scope (LLM call) than a synchronous approval-hook check should take on. |
| [`stackgenhq/genie/pkg/pii`](https://pkg.go.dev/github.com/stackgenhq/genie/pkg/pii) | Entropy + bigram + Luhn + context-aware key detection — closer to the *secret*-scanning problem (detecting high-entropy strings) than PII shape-matching; published 2026-03, unproven track record. |
| [`gen0cide/pii`](https://pkg.go.dev/github.com/gen0cide/pii) | Built-in rule set: phone, SSN, email, IP, credit card, address, banking, UUID, VIN — broadest coverage but also broadest scope-creep risk relative to acceptance criteria (email/SSN/credit-card only). |
| [`ullauri/piidetect`](https://pkg.go.dev/github.com/ullauri/piidetect) | Static-analysis tool (scans *source code files* for PII), not a runtime text-scanning library — wrong tool for this use case (scanning live Bash/file-write payloads, not a codebase). |

**None of these are a clear "buy" over "build"** given the requirements doc's constraints: the feature must produce a `secretPattern`-shaped list callable from the same `approval_handler.go` call site, support `custom_patterns` from user JSON config, and stay dependency-light like its precedent. Every surveyed library brings its own competing pattern/config abstraction that would need to be adapted to fit rather than adopted wholesale. Recommend extending `secret_scanner.go`'s pattern (or a shared abstraction per Open Question 3) with hand-rolled regexes + an inline Luhn validator, consistent with the zero-dependency precedent.

## Regex patterns (reference, for planning phase)

- **Email**: a pragmatic (not full RFC 5322) pattern is standard practice — something like `\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`. Full RFC 5322 compliance is explicitly not worth the regex complexity for a detection/escalation use case (false negatives on obscure valid addresses are an acceptable tradeoff vs. an unreadable pattern).
- **SSN**: `\b(?!000|666|9\d{2})\d{3}-(?!00)\d{2}-(?!0000)\d{4}\b` — excludes the known-invalid SSN ranges (000/666/9xx area numbers, 00 group, 0000 serial) to cut obvious false positives, mirroring how `secret_scanner.go` already tunes patterns to reduce noise (e.g. its `$VAR` exclusion on the "Inline secret env var" pattern).
- **Credit card**: match candidate digit sequences first (`\b(?:\d[ -]?){13,19}\b`, stripped of separators), then validate with Luhn — same two-stage design already anticipated in the requirements doc ("consider Luhn-validated multi-brand" vs. the issue's naive Visa-only regex). Brand-prefix regexes (Visa `4`, Mastercard `5[1-5]`/`2[2-7]`, Amex `3[47]`, Discover `6(?:011|5)`) are optional metadata for the match reason string, not required for detection.

## Dependency verdict

No new Go module dependency is needed for this feature. `regexp` (stdlib) covers all three pattern types' shape-matching; Luhn is inline arithmetic. This keeps the PII scanner consistent with `secret_scanner.go`'s zero-dependency precedent and avoids adding an unvetted/low-adoption third-party package (none of the surveyed libraries have significant adoption signals — no stars/download data surfaced high-confidence "community standard" status) for logic under ~20 lines.

## Sources

- [durango/go-credit-card](https://github.com/durango/go-credit-card)
- [josue/go-credit-card](https://github.com/josue/go-credit-card)
- [phedde/luhn-algorithm](https://pkg.go.dev/github.com/phedde/luhn-algorithm)
- [retgits/creditcard](https://pkg.go.dev/github.com/retgits/creditcard)
- [sgumirov/go-cards-validation](https://github.com/sgumirov/go-cards-validation)
- [aavaz-ai/pii-scrubber](https://github.com/aavaz-ai/pii-scrubber)
- [vsemashko/go-pii-sanitizer](https://pkg.go.dev/github.com/vsemashko/go-pii-sanitizer/sanitizer)
- [intMeric/pii-extractor](https://github.com/intMeric/pii-extractor)
- [stackgenhq/genie/pkg/pii](https://pkg.go.dev/github.com/stackgenhq/genie/pkg/pii)
- [gen0cide/pii](https://pkg.go.dev/github.com/gen0cide/pii)
- [ullauri/piidetect](https://pkg.go.dev/github.com/ullauri/piidetect)
