# Research: Similar Features, Edge Cases, Unstated Needs

Scope: Phase 2 (Features) research for `pii-scanning`. Covers the direct in-repo
precedent, industry approaches, edge cases the design must handle, and
unstated user needs beyond the explicit acceptance criteria.

## 1. In-repo precedent: `server/services/secret_scanner.go`

Read in full (72 lines). Structure:

```go
type secretPattern struct {
    Name    string
    Pattern *regexp.Regexp
}
var secretPatterns = []secretPattern{ /* ~10 entries */ }

type SecretScanResult struct {
    Found       bool
    PatternName string
}

func ScanForSecrets(text string) SecretScanResult {
    if len(text) > 4096 {
        text = text[:4096]
    }
    for _, p := range secretPatterns {
        if p.Pattern.MatchString(text) {
            return SecretScanResult{Found: true, PatternName: p.Name}
        }
    }
    return SecretScanResult{}
}

func FormatSecretDenyMessage(patternName string) string { ... }
```

Key properties worth mirroring or deliberately diverging from:

- **First-match-wins, not all-matches.** Returns on the first pattern hit —
  no aggregation of every match in the text. Fine for auto-deny (any hit
  blocks), but a PII scanner that reports *what kind* of PII for the review
  UI may want to consider whether "first match" hides other categories
  present in the same text (e.g. an email + a credit card in one payload —
  reviewer only sees "email").
- **Byte-length cap, not rune-aware.** `text[:4096]` slices by byte index,
  not rune boundary — on multi-byte UTF-8 input this can split a rune mid-
  sequence. Doesn't corrupt `regexp` matching (Go's `regexp` operates on
  bytes and tolerates invalid UTF-8 in the trailing fragment), but truncation
  location becomes unicode-unaware, so this is worth explicitly deciding on
  rather than copy-pasting silently.
- **Conservative-by-comment.** The comment on `secretPatterns` states
  "intentionally conservative to minimize false positives" — e.g. the
  generic `password=` pattern explicitly excludes shell variable references
  (`$VAR`, `"$VAR"`) to avoid flagging `-Pflyway.password="$RDS_TOKEN"`. This
  is the single most load-bearing design precedent for PII: the existing
  scanner already made an explicit, documented trade-off to suppress a class
  of false positive rather than accept the noise. PII regexes (email, SSN,
  credit card) are inherently far noisier than credential-shaped regexes, so
  this precedent argues for *more* aggressive false-positive suppression
  design work, not less.
- **No context awareness** — pure regex over an opaque string, no awareness
  of surrounding file path, tool name, or "is this a fixture" signal.
- **Call site**: `server/services/approval_handler.go:223-251`, runs first
  (before rule evaluation / domain-age check), reads
  `payload.ToolInput["command"]` only — confirmed via grep that no code path
  anywhere currently reads `ToolInput["content"]`, `["new_string"]`, or
  `["old_string"]` (file write/edit payload fields) for either secret or PII
  scanning. This validates requirements.md's claim that file-content scanning
  is a genuine scope expansion, not already partially covered.
- **Redaction-before-persist pattern**: on a hit, `approval_handler.go`
  shallow-copies `payload.ToolInput` into `sanitizedInput`, replaces the
  scanned field with the `redactedSecret` sentinel (`ai_interfaces.go:11`,
  `"[REDACTED: secret detected]"`), and only *then* calls
  `analyticsStore.RecordFromResult(sanitizedPayload, ...)`. The original
  `payload` (with real secret) is never passed to the analytics store. This
  exact pattern is the direct template for AC #8 ("matched PII text is
  redacted before it is persisted") — a PII scanner should produce an
  equivalent `redactedPII` sentinel and follow the identical copy-then-
  replace-then-record sequence, not reuse `redactedSecret` (a reviewer
  reading `secret-scan` in `CommandPreview` vs. seeing `[REDACTED: PII
  detected]` needs the distinction to know *why* something was scrubbed).
- **Escalation taxonomy integration**: `pkg/classifier/escalation.go`
  defines `RuleIDSecretScan = "secret-scan"` as a package-level sentinel
  constant and `CategorizeEscalationRuleID` switches on it. A `RuleIDPIIScan`
  constant + `EscalationPIIScan` category follows the identical pattern —
  confirmed this is a `switch` with an explicit `default: EscalationExplicitRule`
  fallback, so forgetting to add the new case doesn't silently misfile PII
  hits as no-match; it misfiles them as `explicit-rule`, which is still wrong
  but a smaller class of bug worth naming here so planning treats the
  `escalationCategory.ts` mirror as a hard requirement, not optional polish.
- **Test structure precedent**: `secret_scanner_test.go` has exactly two top-
  level tests — `TestScanForSecrets_NoFalsePositives` and
  `TestScanForSecrets_TruePositives` — i.e. the existing scanner is already
  tested primarily as a false-positive/true-positive table, not per-pattern
  unit tests. A PII scanner test file should follow the same two-bucket
  shape, and the false-positive bucket is where "realistic test fixture"
  regressions (see §3) should be pinned down.

## 2. Industry approaches

### Microsoft Presidio (most directly comparable — open-source PII de-identification)

- Combines regex/pattern recognizers with NER (spaCy/transformer models) and
  **contextual scoring** — a bare 16-digit number near the word "card" or
  "account" scores higher confidence than the same digits in isolation.
  stapler-squad's regex-only, no-NLP scanner can't replicate contextual
  scoring cheaply, but the *design lesson* transfers: a raw regex hit on
  digits-only patterns (credit card, SSN) is inherently higher-noise than a
  structurally distinctive pattern (email's `@`, GitHub token's `ghp_`
  prefix), and the design should treat those two pattern classes differently
  (e.g. credit card gets Luhn validation as a cheap non-ML confidence booster
  — directly answers requirements.md Open Question #4).
- Presidio's recognizers explicitly validate credit cards via Luhn and set
  confidence to 1.0/0.0 accordingly, rather than shipping the un-validated
  regex — this is the field's normalized answer to Open Question #4: add
  Luhn as a cheap validator layered on top of the digit-shape regex, not a
  full replacement scoring system.
- Presidio ships an **allow-list / deny-list mechanism** and per-recognizer
  confidence threshold as first-class config — reinforces requirements.md's
  Open Question #2 (fixture-path allowlisting) as an industry-standard
  knob, not a stapler-squad-specific edge case.

### TruffleHog / Gitleaks / GitGuardian (secret scanning — closer analog)

- TruffleHog's headline differentiator is **live verification**: a detected
  AWS key is only reported if it actually authenticates against AWS,
  reportedly cutting triage volume by up to 90% on noisy repos. This has no
  direct PII equivalent (there's no "verify this is a real SSN" API), which
  is itself an important asymmetry to name in the design doc: PII scanning
  cannot borrow secret-scanning's most effective false-positive-reduction
  technique, so it must lean harder on the *other* techniques below.
  UNVERIFIED/INFERRED from vendor marketing copy — treat as directional, not
  a hard number, since no independent benchmark was opened.
- **Path-based ignore files are the standard mitigation for fixture noise**:
  `.trufflehogignore`, Gitleaks' allowlist config, all support path-glob
  exclusions like `test/fixtures/`, `**/mocks/`, `*.test.js`. This is the
  industry-standard answer to requirements.md Open Question #2 — directly
  supports adding a path-glob allowlist to the `pii_scanning` config schema
  (e.g. `skip_paths: ["testdata/**", "fixtures/**"]`), not just relying on
  ESCALATE-not-DENY to absorb the noise.
- Common practice is running a **fast/cheap scanner pre-commit and a
  deeper/slower scanner in CI** — not directly applicable to stapler-squad's
  single approval-hook call site, but reinforces that even mature tools
  don't try to make one pass both fast and maximally accurate; the 4096-byte
  cap in `secret_scanner.go` is stapler-squad's version of that same
  trade-off and should be revisited (not silently copy-pasted) for PII given
  file-content payloads can be far larger than command strings.

### AWS Comprehend PII

- ML-based, no confirmed Luhn validation per the AWS re:Post community
  question found in search — corroborates that Presidio's explicit
  Luhn-validation choice is the "better" reference implementation to follow
  for a regex-based scanner, not Comprehend's.
- Recognizes partially-masked card numbers (last-4-only) as PII-adjacent
  context but doesn't flag the masked remainder as a hit — relevant to the
  "already-redacted data" edge case below (§3).

## 3. Edge cases and failure modes the design must handle

1. **False positives in test fixtures** — the single largest expected noise
   source per requirements.md Open Question #2. Realistic seed/fixture data
   (`test@example.com`, `4111-1111-1111-1111` Visa test number, sequential
   fake SSNs like `123-45-6789`) will match naively. Mitigations, in order
   of design cost: (a) ESCALATE not DENY absorbs some cost by putting a
   human in the loop rather than hard-blocking; (b) Luhn validation kills a
   large fraction of random-digit false credit-card matches (real Luhn test
   numbers like `4111111111111111` still pass Luhn, so this only reduces,
   doesn't eliminate, fixture noise); (c) path-glob allowlist
   (`testdata/**`, `fixtures/**`, `*_test.go`) per industry precedent above;
   (d) a curated deny-list of well-known placeholder values (`test@example.com`,
   RFC 2606 reserved domains, common SSN test values like `000-00-0000`).
2. **Multi-byte / Unicode text.** The existing scanner's byte-based `[:4096]`
   truncation can split a UTF-8 rune. For PII this matters more than for
   secrets: names/addresses in non-Latin scripts, and some locales' national
   ID formats use non-ASCII digits. Decide explicitly whether v1 scope is
   ASCII-only patterns (email/SSN/credit-card as specified are all ASCII by
   definition) and document that as a stated limitation rather than an
   accidental gap — but the truncation boundary should still be made
   rune-safe (`utf8.RuneCountInString`/`strings` boundary-aware trim) so a
   split multi-byte sequence at the cut point doesn't corrupt an otherwise-
   matchable pattern near the boundary.
3. **Huge file content.** Command strings are bounded by shell/OS argument
   limits; file write/edit `content`/`new_string` payloads are not — a
   large seed-data JSON or CSV fixture could be megabytes. The existing
   4096-byte cap was sized for command-line arguments; scanning file content
   needs its own explicit size policy (same cap reused? a larger cap? scan
   only a sample?) — a silent reuse of 4096 bytes would make PII detection
   in large fixture files essentially non-functional (most PII would live
   past the truncation point), which contradicts AC #3's intent.
4. **Binary files.** Write/Edit tool content is normally text, but nothing
   guarantees it — a base64-encoded blob or genuinely binary payload run
   through PII regexes wastes CPU and can produce nonsense matches (e.g.
   16-digit-shaped byte sequences in binary data hitting the credit-card
   regex). Needs either a binary-content sniff-and-skip (e.g. check for a
   NUL byte or invalid-UTF-8 ratio, same heuristic `git diff` uses) or an
   explicit decision to only scan when `ToolInput` indicates a known-text
   file extension.
5. **Already-redacted/masked data.** Fixture or log data that already shows
   `***-**-6789` or `4111********1111` is *not* raw PII and re-flagging it
   is a pure false positive with no security value — worth an explicit
   pattern-exclusion (e.g. skip runs containing `*`/`X` mask characters
   inside the digit sequence) since this is a common and easily-detectable
   case, unlike general fixture-noise which requires an allowlist.
6. **PII split across multiple tool calls.** A single Bash command or single
   Edit hunk scanned in isolation misses PII assembled incrementally — e.g.
   an agent writes a first name in one Edit call and appends the matching
   SSN in a second. This is a real coverage gap but is almost certainly
   out of scope for this feature (would require session-level state /
   buffering across hook invocations, a materially larger change than a
   per-call regex scan) — the design doc should name this explicitly as a
   known, accepted gap rather than let it surface later as a "why didn't
   this catch X" bug report.
7. **Regex catastrophic backtracking / ReDoS.** Not called out in
   requirements.md. `secret_scanner.go`'s existing patterns use bounded
   quantifiers (`{6,}`, `{32,48}`) rather than nested unbounded groups, which
   avoids this class of bug — a Luhn-adjacent or SSN-adjacent PII regex
   should hold to the same discipline, especially since file content (unlike
   command strings) can be attacker-influenced-length in principle (an agent
   writing to a file based on fetched web content, for instance).
8. **Case/format variants for the same PII type.** SSNs written as
   `123456789` (no dashes) vs `123-45-6789` vs `123 45 6789`; credit cards
   with spaces, dashes, or no separator. The single Visa-only regex named in
   the issue only covers one format — planning's Open Question #4 (Luhn +
   multi-brand) should also resolve separator-format coverage, not just
   brand coverage.

## 4. Unstated user needs beyond the explicit acceptance criteria

These are needs a reviewer/compliance stakeholder would likely raise even
though they're not in the 10 acceptance criteria as written:

1. **Allowlisting known-safe fixture directories/patterns.** Directly named
   as Open Question #2 but not yet an acceptance criterion — the "false
   positive rate" question is really *"how do I tell the scanner 'I know,
   this directory is fake data, stop flagging it'"*. Without this, teams
   with heavy fixture usage will either disable PII scanning entirely (worst
   outcome — defeats the feature) or drown the review queue in noise until
   trust in the badge erodes (the same "cry wolf" failure mode `secret-scan`
   already sidesteps by being narrow/conservative). Recommend surfacing this
   as an explicit AC in planning, not leaving it as a config nice-to-have.
2. **Audit trail / "what was redacted."** AC #8 requires redaction before
   persistence, which is correct for preventing raw PII leaking into logs —
   but it also means a reviewer or compliance auditor investigating *why* an
   item was escalated has no way to see what actually matched, only the
   pattern *name* (mirroring `SecretScanResult.PatternName`, itself never
   the raw value). This is intentional and correct for secrets (you never
   want the secret value anywhere), but for PII a compliance stakeholder may
   legitimately want to know "was this a real SSN or a fixture SSN" without
   re-exposing the raw text — e.g. count + pattern-type + redacted-length
   metadata, or a one-way hash for dedup/audit purposes without storing the
   plaintext. Worth an explicit design decision in planning rather than
   silently inheriting secret-scan's "we log only the pattern name" posture,
   since the ESCALATE (not DENY) behavior implies a human *will* look at the
   original content in the review UI anyway — so the "never persist raw PII"
   goal already has a narrower blast radius than for auto-denied secrets.
3. **Ability to review/undo a false-flag efficiently.** Since PII escalates
   rather than denies, reviewers will hit this repeatedly on fixture-heavy
   repos. An unstated need is a low-friction "approve and don't ask again for
   this pattern in this path" action from the review queue UI itself, which
   would double as a mechanism for building the allowlist in (1) rather than
   requiring a separate config-file edit — this is a plausible fast-follow to
   flag for planning even if out of v1 scope.
4. **Distinguishing PII severity by field type.** AC #7 defaults everything
   to `RiskLevel.Critical`/P0, but industry PII taxonomies (GDPR "special
   category" data vs. ordinary personal data) generally don't treat every
   PII type as equally severe — an email address in a test fixture is a much
   lower-stakes finding than an unmasked SSN or full credit card number.
   Flattening all three to Critical/P0 by default (per issue's stated
   intent, and correctly kept as v1 scope per requirements.md) is a
   reasonable MVP simplification, but planning should note it as a
   deliberate simplification with a likely follow-up (per-pattern
   `risk_level`), not an oversight, so it isn't rediscovered as a "bug" later.
5. **Compliance logging retention/access semantics.** AC #10 in
   requirements.md ("compliance logging... scope to be confirmed in
   planning") implies a stakeholder (security/compliance) wants to know PII
   was *detected* even if redacted — i.e. a durable count/audit record
   distinct from the ephemeral review-queue item, since queue items likely
   get cleared/archived after review. This is effectively the same need as
   (2) but from a retention-policy angle rather than a single-incident
   investigation angle — worth naming both framings in planning since they
   may lead to different storage decisions (e.g. `analytics_store` running
   counters vs. an explicit append-only compliance log).

## Sources

- [Microsoft Presidio: PII Detection Guide 2026](https://explainx.ai/blog/microsoft-presidio-pii-detection-anonymization-guide-2026)
- [PII detection evaluation - Microsoft Presidio](https://microsoft.github.io/presidio/evaluation/)
- [FAQ - Microsoft Presidio](https://microsoft.github.io/presidio/faq/)
- [Gitleaks vs TruffleHog 2026: Secret Scanner Benchmarks](https://appsecsanta.com/secret-scanning-tools/gitleaks-vs-trufflehog)
- [TruffleHog vs Gitleaks vs GitHub Secret Scanning](https://secrails.com/blog/trufflehog-vs-gitleaks-github-secret-scanning-guide)
- [Rafter - Pre-Commit Hooks for Secret Detection](https://rafter.so/blog/secrets/pre-commit-hooks-secret-detection)
- [AWS Comprehend unable to detect card number as PII unless masked - AWS re:Post](https://repost.aws/questions/QUzbSh0KaGTueiy31cjUwHZg/aws-comprehend-unable-to-detect-card-number-as-pii-unless-it-called-out-as-card-or-masking)
- [Detecting PII entities - Amazon Comprehend docs](https://docs.aws.amazon.com/comprehend/latest/dg/how-pii.html)
- [checksum-validated PII guardrail plugins (Luhn/MOD-97) - Drupal.org](https://www.drupal.org/project/ai/issues/3580692)
- In-repo: `server/services/secret_scanner.go`, `server/services/secret_scanner_test.go`, `server/services/approval_handler.go:223-251`, `pkg/classifier/escalation.go`, `pkg/classifier/classifier.go:57`, `server/services/ai_interfaces.go:9-11`
