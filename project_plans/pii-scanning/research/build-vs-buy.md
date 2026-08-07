# Research: Build vs. Buy — PII Scanning

Companion to `research/stack.md` (which surveys OSS Go PII libraries and the Luhn
dependency question in detail — not re-litigated here except where it changes the
verdict). This doc adds the two dimensions stack.md doesn't cover (SaaS/managed APIs,
fork/adapt of gitleaks/trufflehog pattern formats) and reframes the whole space as an
explicit build/viable/no-go decision per option, per the requirements doc's four
questions.

Repo license: **AGPLv3** (`LICENSE.md:1`). This matters for option 4 (forking/vendoring
another project's code) more than for options 1–2, since consuming a permissively
licensed *dependency* (MIT/Apache-2.0) from an AGPL project is unrestricted — the
AGPL's copyleft obligations attach to code you distribute as part of *this* project,
not to what a dependency's own upstream license permits.

## 1. Existing OSS Go libraries for PII detection

Full survey with per-library links in `research/stack.md:35-48`. Summary of the
maturity signal:

| Library | Stars | License | Status |
|---|---|---|---|
| [`gen0cide/pii`](https://github.com/gen0cide/pii) | low (~tens) | permissive (MIT-style) | last substantive activity years old; effectively unmaintained |
| [`philterd/go-phileas`](https://github.com/philterd/go-phileas) | ~51 | Apache-2.0 | actively developed (2026 copyright, sibling Java/Python projects under active development) — most credible of the surveyed set |
| [`aavaz-ai/pii-scrubber`](https://github.com/aavaz-ai/pii-scrubber) | ~9 | unclear (not confirmed) | young, single-org project |
| `intMeric/pii-extractor`, `stackgenhq/genie/pkg/pii`, `ullauri/piidetect`, `vsemashko/go-pii-sanitizer` | low | mixed/unconfirmed | niche, single-maintainer, no adoption signal |

None of these have anything close to gitleaks' or trufflehog's adoption (tens of
thousands of stars, CI-integration ecosystem, dedicated maintainer teams). The single
most mature entry, `go-phileas`, is Apache-2.0 (compatible) and closest in spirit to
Presidio (rule + validator + redaction pipeline) — but it's a port of a Java project,
brings its own `Filter`/`Policy` config model, and would require adapting its output
back into the `secretPattern`/`SecretScanResult` shape this feature must match
(acceptance criteria 1 in `requirements.md:37`). None of the surveyed libraries save
meaningful implementation effort over the ~15-20 lines (3 regexes + a Luhn function)
the requirements doc estimates the hand-rolled version needs.

**gitleaks itself** (MIT, `github.com/gitleaks/gitleaks`) is a secret scanner, not a
PII scanner — it has no SSN/credit-card/email rule set and its detection is
credential-shaped (entropy + regex on TOML-configured rules), which is what
`secret_scanner.go` already reimplements a minimal version of. It's cited by the
requirements doc as "similar to what secret_scanner.go already reimplements," not as a
source of PII patterns.

- **Pros of adopting a library** (best case, `go-phileas`): saves writing ~3 regexes;
  gets you a pre-built `custom_patterns`-equivalent config surface; Apache-2.0 is
  license-compatible.
- **Cons**: adapter/integration burden to fit the `secretPattern` shape likely exceeds
  the code it would save; new dependency to vet, pin, and keep patched (supply-chain
  surface `secret_scanner.go` currently has none of); maturity is thin relative to the
  bar this repo holds contributions to (a 51-star Apache port is not "battle-tested" in
  the way gitleaks or a stdlib algorithm is); no library in the survey is used widely
  enough that a future maintainer would recognize it on sight, unlike `regexp`.
- **Verdict: Not recommended.** None of the surveyed libraries clear the bar of
  "meaningfully less work or meaningfully more correctness than ~20 lines of stdlib
  regex + Luhn," and all of them cost a dependency-vetting burden the zero-dependency
  precedent doesn't pay today.

## 2. SaaS / managed API (AWS Comprehend PII, Google Cloud DLP, Presidio-as-a-service)

This scanner runs **synchronously in the approval hook path** — it must return before
the tool call it's gating is allowed to proceed (mirroring `ScanForSecrets`'s
in-process call at `approval_handler.go:223-251` per the requirements doc). That
constraint alone is close to disqualifying for any network-hop option:

- **AWS Comprehend `DetectPiiEntities`**: typical single-document synchronous latency
  is in the ~100ms-1s range depending on payload size and region, before accounting
  for the added network hop from wherever stapler-squad's backend runs. Cost is
  per-100-character-unit billing — nontrivial at Bash-command/file-write scan volume
  if every tool call is scanned. Requires AWS credentials/IAM wiring the project
  doesn't currently have for this codepath.
- **Google Cloud DLP `inspectContent`**: similar shape — a real API round-trip,
  per-request cost, a new credential/config surface, and the exact irony the
  requirements doc's own Problem section flags: sending text that *might contain
  PII* to a third party in order to find out whether it contains PII. For content
  originating in Bash commands or file writes during an agent session — which may
  include customer data, credentials-adjacent config, or proprietary source —
  this is a materially different trust boundary than the current fully-local
  `ScanForSecrets`.
- **Presidio as a self-hosted service** (Microsoft, MIT-licensed, Python, deployed as
  gRPC/REST microservices — see `microsoft/presidio-genproto` for the Go gRPC
  bindings): avoids the third-party data-residency problem since you'd run it
  in-cluster, but introduces a **new deployed service dependency** (Python runtime,
  spaCy models, a gRPC/REST call from the Go backend) for a feature whose entire
  precedent (`secret_scanner.go`) is a 72-line, zero-dependency, in-process function.
  Latency is still a network hop (loopback/in-cluster, so much better than a public
  API, but still nonzero and now a new failure mode: what does the approval hook do
  if the Presidio sidecar is down or slow?).

- **Pros**: highest theoretical detection quality (Presidio and DLP use ML/NLP
  entity recognition, not just regex — better recall on names, addresses, less
  format-rigid PII than email/SSN/credit-card).
  - **Data residency**: local self-hosted Presidio avoids the third-party-send
    problem; cloud DLP/Comprehend do not.
- **Cons**: adds a network dependency (latency + availability risk) into a synchronous
  approval-blocking path; cloud options send potentially-sensitive text to a third
  party specifically to detect whether it's sensitive; new credential/IAM/deployment
  surface disproportionate to the acceptance criteria's actual scope (email, SSN,
  credit-card — all regex-shaped, not entity-recognition-shaped); per-call cost at
  scan-every-tool-call volume; self-hosted Presidio requires operating a Python
  service alongside a Go monolith the project otherwise has no Python runtime
  dependency for.
- **Verdict: Not recommended** for the scope in the acceptance criteria (regex-shaped
  PII types only). Cloud DLP/Comprehend are explicitly worse than the status quo on
  the exact risk this feature exists to reduce (data leaving the trust boundary).
  Self-hosted Presidio would be **Viable** only if/when the product later needs
  ML-grade entity recognition (names, addresses, free-text PII) that regexes
  structurally can't catch — worth a forward-looking note in the plan doc, not an
  MVP dependency.

## 3. LLM-generated regex vs. battle-tested library — is hand-rolling reckless?

The GitHub issue's literal proposal (Visa-only credit-card regex, no Luhn check) would
be reckless if shipped as-is: a bare `4\d{15}` (or similar) matches roughly 1 in 10,000
random 16-digit sequences and will false-positive constantly on non-card data (order
IDs, timestamps concatenated, test fixture data) in exactly the kind of content this
feature scans (Bash args, file writes in test/fixture-heavy agent sessions — see the
requirements doc's Open Question 2 on false-positive rate).

But that's an argument for **validating the match**, not for buying a library. The
existing `secret_scanner.go` precedent already hand-rolls comparably fragile-looking
patterns in production (JWT three-segment regex, PEM header regex, inline
`password=value` heuristics) and the codebase's own convention is clearly "hand-rolled
regex is fine when the pattern is well-understood and testable in isolation." Email
and SSN are exactly that: well-documented formats with an essentially fixed shape.
Credit card numbers are the one case in scope where format alone is *not* enough —
which is precisely why Luhn matters.

**Luhn validation**: this is arithmetic, not a regex-expressible property (sum digits
right-to-left, double every second digit, subtract 9 if the result exceeds 9, check
`sum % 10 == 0`). It is genuinely ~10-15 lines of stdlib-only Go with no external
state or edge cases beyond "is this a valid digit string" — see `research/stack.md:23-33`
for the full comparison against community Luhn packages (`phedde/luhn-algorithm`,
`durango/go-credit-card`, etc.), all of which wrap the same ~15 lines with more
surface area (brand tables, expiry/CVV fields) than this feature needs. There is no
"stdlib-adjacent tiny algorithm as a dependency" option worth taking — the algorithm
*is* the tiny thing; a dependency for it would be strictly worse than inlining it
(one more `go.sum` entry, one more thing to vet, for code smaller than its own test
file).

- **Pros of hand-rolling (regex + inline Luhn)**: matches existing codebase
  convention exactly; zero new dependencies; each pattern is independently unit-testable;
  full control over false-positive tuning (SSN invalid-range exclusion, credit-card
  Luhn gate) that a generic library wouldn't necessarily expose per-pattern.
- **Cons**: the team owns correctness and false-positive tuning long-term (same cost
  `secret_scanner.go` already carries); no free multi-country/multi-format coverage
  (e.g. non-US SSN-equivalents, IBAN) if that's ever needed — out of scope per the
  requirements doc's acceptance criteria (email/SSN/credit-card only), so not a
  present cost.
- **Verdict: Recommended**, but *with* Luhn validation on the credit-card pattern —
  not the issue's literal Visa-only, unvalidated proposal. Luhn is inline arithmetic,
  not a dependency decision.

## 4. Fork or adapt gitleaks' / trufflehog's pattern config format

Both tools' rule format is essentially: `{id, description, regex, keywords, entropy,
allowlist}` expressed in TOML (gitleaks, `.gitleaks.toml`) or YAML (trufflehog
detectors, though trufflehog's Go detectors are mostly compiled-in structs, not a
loadable config format — its config-driven surface is much thinner than gitleaks').
Both are designed for a fundamentally different problem than this feature: **secret**
detection (credential-shaped, often keyed to a specific vendor's token format,
frequently paired with entropy scoring), not **PII** detection (fixed structural
formats like SSN/email/credit-card with no entropy signal at all — a valid SSN is not
"high entropy," it's a fixed-width digit pattern).

Adapting *just the config format* (not the binary) would mean: defining a TOML/YAML
schema mirroring gitleaks' `[[rules]]` shape, writing a loader, and mapping loaded
rules into `secretPattern`-equivalents at startup. That's strictly more code than the
requirements doc's proposed `pii_scanning.custom_patterns` field (acceptance criterion
9, `requirements.md:45`) — which is already just "additional user regexes in the
existing JSON config" — needs. Gitleaks' TOML schema also carries fields (`entropy`,
`keywords`, `allowlist.regexTarget`) that don't map cleanly onto pure-shape PII
patterns, so most of an adapted schema would be dead weight for this use case.

- **Pros**: gitleaks' config format is a familiar shape to anyone who's used it
  (`[[rules]]` blocks), and its `allowlist` concept (regex/path/commit exclusions) is
  conceptually similar to the "skip `testdata/`/`fixtures/` paths" idea raised in
  the requirements doc's Open Question 2 — worth borrowing as an *idea*, not a format.
- **Cons**: introduces a new config-file format (TOML) alongside the project's
  existing JSON config convention (`config/` package) for no clear benefit; the
  schema is optimized for a different detection problem (entropy-scored secrets, not
  fixed-shape PII); still requires writing a loader/mapper by hand, so it doesn't
  actually save implementation work over just adding fields to the existing JSON
  config the requirements doc already points at.
- **Verdict: Not recommended** as a format to adopt wholesale. The *allowlist/path-exclusion
  concept* is worth carrying into the plan doc (addresses Open Question 2), expressed
  as a plain field in the existing JSON config schema, not a ported TOML format.

## Summary table

| Option | Verdict | One-line why |
|---|---|---|
| 1. OSS Go PII library (go-phileas, gen0cide/pii, etc.) | Not recommended | Adapter cost exceeds the ~20 lines it would save; none are adoption-proven |
| 2a. Cloud DLP/Comprehend (managed API) | Not recommended | Sends PII to a third party to detect PII; adds latency + cost to a sync approval path |
| 2b. Self-hosted Presidio | Viable (future) | Solves data-residency but adds a Python service dependency disproportionate to regex-shaped MVP scope |
| 3. Hand-rolled regex + inline Luhn | **Recommended** | Matches existing `secret_scanner.go` precedent exactly; Luhn is ~15 lines of stdlib arithmetic, not a dependency decision |
| 4. Fork/adapt gitleaks or trufflehog config format | Not recommended | Wrong-shaped schema (entropy-scored secrets vs. fixed-shape PII); existing JSON config is simpler and already proposed |

## Overall recommendation

**Build**, following the `secret_scanner.go` precedent exactly: a new
`server/services/pii_scanner.go` with a `piiPattern`-shaped list (email, SSN with
invalid-range exclusion, credit-card digit-sequence regex) plus an unexported
`isValidLuhn(digits string) bool` helper gating the credit-card match. Zero new Go
module dependencies. This is consistent with `research/stack.md`'s independent
"Dependency verdict" (`stack.md:56-58`) reached via the narrower Luhn/library-survey
angle — both research paths converge on the same answer from different starting
questions.

Revisit self-hosted Presidio only if a future requirement needs entity-recognition-grade
detection (free-text names, addresses) that no regex can express — that's a distinct,
larger feature, not a variant of this one.

## Sources

- [gitleaks/gitleaks — LICENSE (MIT)](https://github.com/gitleaks/gitleaks/blob/master/LICENSE)
- [philterd/go-phileas](https://github.com/philterd/go-phileas)
- [gen0cide/pii](https://github.com/gen0cide/pii)
- [aavaz-ai/pii-scrubber](https://github.com/aavaz-ai/pii-scrubber)
- [Microsoft/presidio-genproto (Go gRPC bindings)](https://github.com/microsoft/presidio-genproto/blob/master/golang/ocr.pb.go)
- [dmnyu/go-presidio (unofficial Go client)](https://pkg.go.dev/github.com/dmnyu/go-presidio)
- `research/stack.md` (this project's companion research doc — OSS library survey detail, regex pattern reference, Luhn dependency comparison)
- `server/services/secret_scanner.go` (in-repo precedent, read in full for this research)
- `LICENSE.md` (repo license: AGPLv3)
