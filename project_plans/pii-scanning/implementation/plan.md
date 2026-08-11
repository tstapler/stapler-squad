# Implementation Plan: pii-scanning

**Feature**: Built-in PII pattern scanner (email/SSN/credit-card) wired into the approval hook, escalating matching Bash commands and Write/Edit content for manual review instead of auto-denying them, with a new `pii-scan` escalation category surfaced in the review queue badge and analytics table.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: PII scanning defaults to enabled + escalate](../decisions/ADR-001-pii-scan-defaults-enabled-escalate.md)

---

## Step 0.5 — Alternatives Considered

Three high-level shapes were considered for where PII detection lives in the system:

1. **Extend `secret_scanner.go` in place** — add PII patterns to the existing `secretPatterns` list and let secret-scan's auto-deny branch handle them too.
   *Strength*: zero new call sites, smallest diff.
   *Weakness*: forces PII onto secret-scan's auto-deny control flow, directly contradicting AC4/the issue's own recommendation that PII should escalate (a human decision), not auto-deny (an unconditional block) — test-fixture PII would hard-block agent work.
2. **New `pii_scanner.go` + a shared `ContentScanner` interface unifying secret-scan and PII-scan behind one type** — the "proper OOP" instinct.
   *Strength*: one call site, one mental model for "scan this text."
   *Weakness*: the two callers need genuinely different control flow (secret-scan denies-and-returns; PII-scan escalates-and-falls-through, or optionally denies per config) — an interface would immediately need a decision/branch parameter, which is `.claude/rules/interface-pollution-checklist.md` smell #4 (forwarding-only wrapper) and smell #1 (speculative interface) in one move.
3. **New `pii_scanner.go` sharing only the low-level `scanPatterns` mechanism (function, not interface), with PII-scan structured as a sibling of the domain-age branch** (escalate + `goto createApproval`), separate pattern lists, separate config. *(chosen)*
   *Strength*: matches the one genuine shared need (byte-capped, ordered-list regex scanning) without forcing shared control flow; PII's escalate-by-default behavior falls out naturally from following the domain-age branch's existing shape.
   *Weakness*: two pattern lists and two call sites to keep straight in `approval_handler.go`, and the shared `scanPatterns`/`extractWriteContent` helpers need their own file to avoid implying either scanner owns the other.

Option 3 is the strongest and is what this plan implements — it is `research/architecture.md`'s own conclusion (§1–§2), restated here per Step 0.5's requirement to record alternatives explicitly rather than only cite the research doc. See the Pattern Decisions table below for the alternatives rejected on each individual component within option 3.

---

## Domain Glossary

| Term | Definition |
|---|---|
| PII | Email, SSN, and credit-card-number patterns that identify an individual — the three built-in categories this feature detects. |
| `namedPattern` | The shared `{Name string; Pattern *regexp.Regexp}` struct extracted from `secret_scanner.go`'s `secretPattern`, used by both the secret and PII pattern lists. |
| `scanPatterns` | The shared low-level function that runs a byte-capped text against an ordered `[]namedPattern` and returns the first match (name + found bool). |
| `piiPatterns` | The package-level `[]namedPattern` slice of built-in email/SSN/credit-card patterns in `pii_scanner.go`. |
| `ScanForPII` | The public function (mirrors `ScanForSecrets`) that scans text against `piiPatterns` plus any compiled custom patterns and returns a `PIIScanResult`. |
| `PIIScanResult` | `{Found bool; PatternName string}` — the outcome of a `ScanForPII` call. |
| `isValidLuhn` | Unexported arithmetic helper validating a digit string against the Luhn checksum, gating credit-card-shaped regex matches. |
| `hasValidLuhnMatch` | Helper that extracts every credit-card-shaped digit run from text and returns true if at least one passes `isValidLuhn`. |
| `extractWriteContent` | Shared helper returning the `(field, content)` pair a Write (`"content"`) or Edit (`"new_string"`) tool call is about to write, or `("", "")` for other tools. |
| `looksBinary` | Cheap NUL-byte heuristic that skips PII/secret scanning of non-text Write/Edit content. |
| `redactedPII` | The `"[REDACTED: PII detected]"` sentinel written in place of a matched field before analytics persistence — distinct from the existing `redactedSecret` sentinel so a reviewer can tell which scanner redacted a field. |
| `skipPIIScanForPath` | Helper checking a request's `Cwd`/`file_path` against configured (or default) skip-path substrings; returns true when PII scanning should be bypassed. |
| `PIIScanningConfig` | New nested config struct (`config/types.go`) holding `Enabled`, `CustomPatterns`, `OnDetection`, `SkipPathPatterns`. |
| `EnabledOrDefault` / `OnDetectionOrDefault` / `SkipPathPatternsOrDefault` | Tri-state accessor methods on `PIIScanningConfig`, mirroring the existing `TmuxExecGateConfig.SlotsOrDefault`/`SessionRetentionConfig.EnabledOrDefault` pattern. |
| `EscalationPIIScan` | New `EscalationCategory` constant (`"pii-scan"`) added to `pkg/classifier/escalation.go`. |
| `RuleIDPIIScan` | New sentinel `RuleID` constant (`"pii-scan"`) emitted by the PII-scan escalate/deny branch and categorized by `CategorizeEscalationRuleID`. |
| `FormatPIIEscalationReason` / `FormatPIIDenyMessage` | User-facing message builders for the escalate and deny paths, mirroring `FormatSecretDenyMessage`. |
| `piiContentScanMaxBytes` | 65536-byte (64 KB) cap applied when scanning Write/Edit content — distinct from the existing 4096-byte command-text cap, sized for file bodies rather than shell arguments. |
| `PendingApproval` / `PersistedApproval` | Existing structs (`approval_store.go`) that carry `EscalationCategory = "pii-scan"` through the review-queue chain with no structural change. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Content-scanning mechanism | Shared plain function (`scanPatterns`), not a shared interface | Go idiom ("discover interfaces, don't design with them") | GoF Strategy — a `ContentScanner` interface unifying secret-scan and PII-scan | The two callers need different control flow (deny-and-return vs. escalate-and-fall-through, or config-driven deny); an interface would immediately need a decision/branch parameter, defeating the abstraction (`architecture.md` §1; `.claude/rules/interface-pollution-checklist.md` smell #4). |
| Pattern list representation | Table-driven data (`[]namedPattern` slice + range loop) | Existing `secretPatterns` precedent | GoF Chain of Responsibility — each pattern as a handler object | Patterns have no per-step behavior beyond "does this regex match" — a slice+loop is simpler and exactly matches the file this feature is modeled on; CoR adds indirection with zero behavioral benefit. |
| Config schema | Nested value struct + `OrDefault()` accessor methods | Existing `TmuxExecGateConfig`/`SessionRetentionConfig`/`BrowserPassthroughConfig` precedent in `config/types.go` | A dedicated constructor + `Validate()` interface layer | 5+ sibling configs already use the plain-nested-struct pattern; a heavier validation-object layer for one more feature is checklist smell #1 (speculative interface) and inconsistent with siblings. |
| Credit-card validation | Inline validator function (`isValidLuhn`) gating a regex candidate match | Presidio's "recognizer + validator" two-stage design (`features.md` §2) | A `Validator` interface implemented per pattern type | Single call site, single implementation today (only credit-card needs arithmetic validation) — textbook checklist smell #1; a plain function suffices until a second pattern needs one. |
| Escalation taxonomy extension | Sentinel string constants + switch-case categorization (`CategorizeEscalationRuleID`) | Existing `pkg/classifier/escalation.go` pattern | A shared proto `enum` between Go and TS | Already explicitly rejected for this exact taxonomy in `project_plans/escalation-reasoning/implementation/plan.md` — 5 fixed, backend-only string keys don't justify `SessionType`-style 7-touchpoint proto-enum machinery; PII is the 6th key, doesn't change the calculus (`architecture.md` §4). |
| False-positive path exclusion | Substring match of `Cwd`/`file_path` against a configurable list, with a built-in default | Simplest-thing-that-works; avoids a new dependency | Full glob/doublestar matching, or adopting gitleaks'/trufflehog's `allowlist` config format | `build-vs-buy.md` §4 explicitly rejected adopting a foreign config *format*; a glob library is a new dependency for a feature whose entire value proposition is staying zero-dependency (`stack.md`). Substring matching against a short user-configurable list covers the stated fixture-directory use case without the added surface. |
| PII scan vs. secret scan decision-on-match | Two independent branches in `approval_handler.go`, PII's structured as a sibling of the domain-age branch (`goto createApproval`), not of the secret-scan branch (`return`) | `architecture.md` §2 | One merged branch with a shared "decision" flag | Secret-scan must return early (never reach the classifier); PII-scan (by default) must fall through to the manual-review queue. Merging the branches would need the same decision-parameter smell rejected above. |

---

## Resolutions to requirements.md's Open Questions

1. **Escalate vs. deny** → **ESCALATE by default**, with `pii_scanning.on_detection: "deny"` as an explicit opt-in for stricter environments. Any value other than `"deny"` is treated as escalate (fail-safe: escalate is never worse than the pre-feature status quo, since it never blocks work outright). Matches requirements.md's own recommendation and `architecture.md`'s structural analysis (PII-scan is a sibling of the domain-age branch, not the secret-scan branch).
2. **False-positive rate / testdata path exclusion** → **YES**, include an early-return path-exclusion check. Implemented as substring matching of `payload.Cwd` and `ToolInput["file_path"]` against `PIIScanningConfig.SkipPathPatternsOrDefault()`, which falls back to a built-in default list (`testdata/`, `/fixtures/`, `/mocks/`, `_test.go`, `.test.ts`, `.test.tsx`, `.spec.ts`) when unset. This directly targets `pitfalls.md`'s largest named false-positive source and `architecture.md` §7's "cheap addition at the insertion point" recommendation.
3. **Unify secret-scan and PII-scan?** → **NO.** Shared low-level `scanPatterns`/`extractWriteContent` helpers; separate pattern lists (`secretPatterns` vs. `piiPatterns`); separate call sites with separate control flow (secret-scan denies+returns; PII-scan escalates+falls-through by default, or denies+returns when `on_detection: "deny"`). Per `architecture.md` §1.
4. **Luhn validation** → **YES**, implemented inline as the unexported `isValidLuhn(digits string) bool` in `pii_scanner.go`. No new dependency — matches `stack.md`'s and `build-vs-buy.md`'s independent recommendations.
5. **Where does `pii_scanning` config live?** → `config.Config.PIIScanning PIIScanningConfig` field in `config/config.go` (new field, placed alongside `SessionRetention`/`TmuxExecGate`/`Hibernation`), with `PIIScanningConfig` itself defined in `config/types.go` — confirmed by reading both files directly; this is the exact structural slot 5 sibling nested-config features already occupy.

**UX scope-alignment resolutions** (from `research/ux.md`):
- "PII detections tile in analytics" (requirements.md line 31) = **a new row in the existing `ESCALATION_CATEGORY_LABELS`-driven Escalation Reasons table**, not a new dedicated stat card. No new component.
- **Redaction vs. reviewability**: redact matched text **only at persistence time** (the analytics store, via the existing `sanitizedInput`/sentinel copy pattern) — **never at live-queue display time**. The `PendingApproval`/`ApprovalStore.persistToDiskLocked()` JSON file and the review-queue UI continue to show raw `command`/`file_path`/content-derived metadata verbatim, exactly as they do today for every other escalation category, because the reviewer's job (judging fixture-vs-real) requires seeing the actual text. AC8's "redacted before persisted to session history / analytics logs" is scoped to the analytics store specifically — the pending-approval JSON store is an accepted, intentional exception, not a gap to close in this feature (`ux.md` §4, option 1).
- **Compliance drill-down** (per-decision audit view, approver attribution) is **out of scope** for this plan. AC6 is satisfied by the existing count-only `escalationReasonCounts["pii-scan"]` surface. A filterable review-history view is a materially larger, separate feature (`ux.md` §5) — named here as a deliberate non-goal, not an oversight.
- **Per-pattern severity** (email vs. SSN vs. credit-card each defaulting to `RiskCritical`) is a deliberate MVP simplification per AC7 and the issue's stated intent, not an oversight. A future per-pattern `RiskLevel` is a plausible fast-follow, not scoped here.
- **PII assembled across multiple tool calls** (e.g. name in one Edit, SSN in a later one) is an accepted, out-of-scope gap — would require session-level buffering across hook invocations, a materially larger change than a per-call regex scan (`features.md` §3.6).
- **Already-masked data** (`***-**-6789`, `4111********1111`) needs no special-case code: the SSN/credit-card patterns require digit characters, so masked text containing `*`/`X` simply does not match by construction — noted here so it isn't rediscovered as a missing feature.

---

## Observability Plan

- **Logs**: the PII-scan branch logs via `log.ForSession(sessionID).Info(...)` with `tool`, `pattern` (name only, never matched text), `field` (`command`/`content`/`new_string`), and `escalation_category: "pii-scan"` — mirroring the existing secret-scan log line's pattern-name-only discipline (`pitfalls.md` §3's explicit rule: never log matched text, only pattern metadata).
- **Metrics**: no new metrics infrastructure. `AnalyticsStore.RecordFromResult` + the existing `escalationReasonCounts["pii-scan"]` map entry (already generic, `proto/session/v1/types.proto`'s `map<string,int32>`) is the metric surface — visible today via the 7/14/30/90-day `ApprovalAnalyticsPanel` window selector with zero new plumbing.
- **Alerts**: none proposed. A spike in `pii-scan` escalations is visible via the existing analytics window; dedicated alerting is out of scope, consistent with the "count-only" UX resolution above.

## Risk Control

- **Feature flag / kill switch**: `pii_scanning.enabled` (default `true`) is the on/off control — setting it to `false` in `config.json` fully disables the new scan block with zero code rollback needed. This mirrors `SessionRetentionConfig`'s pointer-bool pattern rather than the generic `Config.FeatureFlags` map, since this is a fixed, single-purpose toggle, not a dynamically-registered flag.
- **Rollback procedure**: if PII scanning floods the review queue with false positives in practice, the first line of defense is `pii_scanning.skip_path_patterns` (widen the exclusion list) or `pii_scanning.enabled: false` — no deploy/rollback of the binary is required, only a config edit, since the check is config-gated at every call. `on_detection` never needs a rollback path of its own: escalate (the default) is never a regression from the pre-feature baseline, since nothing was previously auto-denied for PII.
- **Staged rollout**: before merging, run the full scanner against this repo's own fixture-heavy directories (`server/services/testdata/`, `web-app/**/*.test.ts(x)`, seed data under `session/ent/` migrations) as a manual verification pass (Task 6.1.6a) to confirm the default skip-path list actually absorbs the repo's own realistic fixture noise — not automated, but required before treating `Enabled: true` as a safe default.

## Unresolved Questions

- [ ] Confirm the exact `ToolInput` key names Claude Code's Write/Edit `PreToolUse`/`PermissionRequest` hook actually sends (`"content"` for Write, `"new_string"`/`"old_string"` for Edit) against Claude Code's hook schema docs, since `architecture.md` §3 marks this **INFERRED, not verified** — no repo-local test fixture exists today constructing a real Write/Edit `tool_input` payload. Blocks Task 1.1.1c (`extractWriteContent`) and Task 6.1.2a (Write-content integration test) — first sub-step of Task 1.1.1c is this verification; if key names differ, adjust the lookups before merging (a verification step, not a redesign). Owner: implementer.

None else outstanding — escalate-vs-deny, unify-vs-separate, Luhn, config location, and the "tile" UX ambiguity are all resolved above with citations back to the research docs.

## Dependency Visualization

```
Phase 1: Shared scanning mechanism + PII pattern library
   │  (content_scanner.go, pii_scanner.go — no config/handler wiring yet)
   ▼
Phase 2: Config schema (PIIScanningConfig)
   │
   ▼
Phase 3: ApprovalHandler wiring (SetPIIScanningConfig, PII-scan block, secret-scan
   │      content-scan extension, server.go wiring)
   │
   ├──────────────────────────────────────────────┐
   ▼                                               ▼
Phase 6: Backend integration tests           (needs Phase 4a's RuleID/category
   (needs Phase 3 + Phase 4a)                 constants to assert against)

Phase 4a: Go escalation taxonomy  ─────────────────┘
   (pkg/classifier/escalation.go — independent of Phase 1–3, can run in parallel)

Phase 4b: TS escalation taxonomy
   │  (escalationCategory.ts — independent of Phase 1–3/4a, can run in parallel)
   ▼
Phase 5: Review Queue badge + Analytics label UI
   (needs Phase 4b's "pii-scan" union member to exist)
```

Phases 1–3 and 4a/4b are independent work streams that can be parallelized; Phase 5 depends only on 4b, Phase 6 depends on Phase 3 and 4a.

---

## Phase 1: Shared Scanning Mechanism & PII Pattern Library

### Epic 1.1: Shared content-scanning helpers

**Goal**: Extract the generic "scan a byte-capped string against an ordered pattern list" mechanism out of `secret_scanner.go` into a shared file, and add the Write/Edit content-extraction helper both scanners will use, without changing `ScanForSecrets`'s observable behavior.

#### Story 1.1.1: Extract `namedPattern`/`scanPatterns`, add `extractWriteContent`/`looksBinary`

**As a** developer extending the approval hook's scanning coverage, **I want** a shared low-level scanning function and a shared write-content extractor, **so that** secret-scan and PII-scan can both use them without duplicating the byte-cap/first-match-wins logic or inventing two different ways to read Write/Edit payloads.

**Acceptance Criteria**:
- `scanPatterns` reproduces `ScanForSecrets`'s exact current behavior (byte cap, first-match-wins) when called with `secretPatterns` and 4096.
  - *Given* the command text `` `curl -H "Authorization: Bearer ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234"` ``, *When* `scanPatterns(text, secretPatterns, 4096)` is called, *Then* it returns `("GitHub personal access token", true)` — identical to today's `ScanForSecrets(text)` result.
- `extractWriteContent` returns the correct `(field, content)` pair per tool name.
  - *Given* `toolName = "Write"`, `toolInput = map[string]interface{}{"file_path": "/tmp/seed.sql", "content": "INSERT INTO users VALUES ('a@b.com')"}`, *When* `extractWriteContent(toolName, toolInput)` is called, *Then* it returns `("content", "INSERT INTO users VALUES ('a@b.com')")`.
  - *Given* `toolName = "Edit"`, `toolInput = map[string]interface{}{"old_string": "foo", "new_string": "bar a@b.com"}`, *When* called, *Then* it returns `("new_string", "bar a@b.com")` — `old_string` (the pre-image) is never scanned.
  - *Given* `toolName = "Bash"`, *When* called, *Then* it returns `("", "")`.
- `looksBinary` skips non-text content cheaply.
  - *Given* content containing a NUL byte (`"abc\x00def"`), *When* `looksBinary(content)` is called, *Then* it returns `true`.
  - *Given* content `"plain text with an email a@b.com"`, *When* called, *Then* it returns `false`.

**Files**: `server/services/content_scanner.go` (new), `server/services/secret_scanner.go`

##### Task 1.1.1a: Create `content_scanner.go` with `namedPattern` + `scanPatterns` (~4 min)
- New file `server/services/content_scanner.go`. Define `type namedPattern struct { Name string; Pattern *regexp.Regexp }` and `func scanPatterns(text string, patterns []namedPattern, maxBytes int) (patternName string, found bool)` — byte-cap `text` to `maxBytes` (reuse the existing `if len(text) > maxBytes { text = text[:maxBytes] }` shape from `secret_scanner.go:54-56`), range over `patterns`, return on first `Pattern.MatchString(text)` hit.
- Files: `server/services/content_scanner.go`

##### Task 1.1.1b: Refactor `secret_scanner.go` to use `namedPattern`/`scanPatterns` (~4 min)
- Change `type secretPattern struct{...}` to `type secretPattern = namedPattern` (type alias, zero call-site changes needed elsewhere in the package) OR change `secretPatterns` to `[]namedPattern` directly and delete the now-redundant `secretPattern` type — prefer the latter (delete `secretPattern`, grep confirms it's unexported and only used within this file). Rewrite `ScanForSecrets` to call `scanPatterns(text, secretPatterns, 4096)` and wrap the result in `SecretScanResult{Found: found, PatternName: name}`.
- Run `go test ./server/services/... -run TestScanForSecrets` to confirm zero behavior change.
- Files: `server/services/secret_scanner.go`

##### Task 1.1.1c: Add `extractWriteContent` + `looksBinary` to `content_scanner.go` (~5 min)
- **First**: verify the `ToolInput` key names against Claude Code's documented `PreToolUse`/`PermissionRequest` hook payload schema (per the Unresolved Question above) — confirm `"content"` (Write) and `"new_string"`/`"old_string"` (Edit) are correct before wiring the lookups.
- Add `func extractWriteContent(toolName string, toolInput map[string]interface{}) (field, content string)`: switch on `toolName` (`"Write"` → read `toolInput["content"]`; `"Edit"` → read `toolInput["new_string"]`; default → `("", "")`), each via a `.(string)` type assertion with `ok` check (matching the existing `payload.ToolInput["command"].(string)` idiom at `approval_handler.go:225`).
- Add `func looksBinary(content string) bool`: return true if `strings.IndexByte(content, 0) >= 0` within the first 8000 bytes (mirrors git's own binary-detection heuristic size), false otherwise.
- Files: `server/services/content_scanner.go`

##### Task 1.1.1d: Unit tests for `scanPatterns`/`extractWriteContent`/`looksBinary` (~5 min)
- New file `server/services/content_scanner_test.go`. Table-driven tests: `TestScanPatterns_FirstMatchWins`, `TestExtractWriteContent` (Write/Edit/Bash/unknown-tool cases from the Story's GWT examples above), `TestLooksBinary` (NUL-byte present/absent cases).
- Files: `server/services/content_scanner_test.go`

---

### Epic 1.2: PII pattern library

**Goal**: A `pii_scanner.go` mirroring `secret_scanner.go`'s shape exactly, covering email, SSN (with invalid-range exclusion), and Luhn-validated credit-card numbers, plus the redaction/message-formatting helpers.

#### Story 1.2.1: Email + SSN patterns, `ScanForPII` skeleton

**As a** developer implementing PII detection, **I want** `ScanForPII` covering email and SSN shapes, **so that** the approval hook has a working detector for the two simplest PII categories before credit-card/Luhn complexity is added.

**Acceptance Criteria** (AC1 from requirements.md):
- Email pattern matches a realistic address and does not match code/URLs without an `@`.
  - *Given* text `"contact john.doe+test@example.co.uk for details"`, *When* `ScanForPII(text, nil)` is called, *Then* it returns `PIIScanResult{Found: true, PatternName: "Email address"}`.
  - *Given* text `"https://example.com/api/v1"`, *When* called, *Then* it returns `PIIScanResult{}` (no match).
- SSN pattern matches a plausible SSN and excludes known-invalid ranges.
  - *Given* text `"ssn: 219-09-9999"`, *When* called, *Then* it returns `Found: true, PatternName: "Social Security Number"`.
  - *Given* text `"order-id: 000-12-3456"` (area number `000` is invalid), *When* called, *Then* it returns `PIIScanResult{}` (no match) — the invalid-range exclusion mirrors `stack.md`'s recommended pattern `\b(?!000|666|9\d{2})\d{3}-(?!00)\d{2}-(?!0000)\d{4}\b`.
- v1 scope is dashed-format SSN only (no bare 9-digit or space-separated variants) — a stated limitation, not an oversight, mirroring `secret_scanner.go`'s "intentionally conservative" precedent.

**Files**: `server/services/pii_scanner.go` (new)

##### Task 1.2.1a: Create `pii_scanner.go` with `piiPatterns` (email, SSN), `PIIScanResult`, `ScanForPII` (~5 min)
- New file `server/services/pii_scanner.go`. Define `const piiContentScanMaxBytes = 65536`. Define `var piiPatterns = []namedPattern{{Name: "Email address", Pattern: regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)}, {Name: "Social Security Number", Pattern: regexp.MustCompile(`\b(?!000|666|9\d{2})\d{3}-(?!00)\d{2}-(?!0000)\d{4}\b`)}}` (credit card added in Story 1.2.2). Define `type PIIScanResult struct { Found bool; PatternName string }`. Define `func ScanForPII(text string, customPatterns []namedPattern) PIIScanResult` calling `scanPatterns(text, piiPatterns, piiContentScanMaxBytes)` first, falling back to `scanPatterns(text, customPatterns, piiContentScanMaxBytes)` if no built-in pattern hit.
- Files: `server/services/pii_scanner.go`

##### Task 1.2.1b: Tests for email/SSN true/false positives (~5 min)
- New file `server/services/pii_scanner_test.go`, following `secret_scanner_test.go`'s two-bucket shape: `TestScanForPII_NoFalsePositives` (URLs, code without `@`, invalid-range SSNs) and `TestScanForPII_TruePositives` (realistic email/SSN examples from the Story's GWT above).
- Files: `server/services/pii_scanner_test.go`

#### Story 1.2.2: Credit-card pattern + Luhn validation

**As a** developer implementing PII detection, **I want** credit-card matches gated by Luhn validation, **so that** the single highest-volume false-positive source (any 16-digit number) is not flagged as PII.

**Acceptance Criteria** (AC1, resolution of Open Question 4):
- A Luhn-valid test credit card number matches; a same-length but Luhn-invalid digit run does not.
  - *Given* text `"card: 4111111111111111"` (a well-known Luhn-valid Visa test number), *When* `ScanForPII(text, nil)` is called, *Then* it returns `Found: true, PatternName: "Credit card number"`.
  - *Given* text `"order id 4111111111111199"` (16 digits, fails Luhn), *When* called, *Then* it does **not** match on the credit-card pattern (and does not match email/SSN either) — result is `PIIScanResult{}`.

**Files**: `server/services/pii_scanner.go`, `server/services/pii_scanner_test.go`

##### Task 1.2.2a: `isValidLuhn` + `hasValidLuhnMatch`, wire into `ScanForPII` (~5 min)
- Add `func isValidLuhn(digits string) bool` (sum right-to-left, double every second digit, subtract 9 if >9, check `sum % 10 == 0`; return false for non-digit input or length outside 13–19).
- Add a credit-card candidate pattern `creditCardCandidatePattern = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)` (not added to `piiPatterns` directly, since it needs Luhn gating, not a bare `MatchString`).
- Add `func hasValidLuhnMatch(text string) bool`: `creditCardCandidatePattern.FindAllString(text, -1)`, strip `[ -]` separators from each candidate via `strings.NewReplacer(" ", "", "-", "")`, call `isValidLuhn` on the stripped digits, return true on first pass.
- Update `ScanForPII` to check `hasValidLuhnMatch(text)` before/alongside the `piiPatterns` loop and return `PIIScanResult{Found: true, PatternName: "Credit card number"}` on a hit.
- Files: `server/services/pii_scanner.go`

##### Task 1.2.2b: Luhn test table (~4 min)
- Add `TestIsValidLuhn` (table: known-valid test card numbers for Visa/Mastercard/Amex/Discover — all pass; the same numbers with one digit incremented — all fail; non-digit input — false) and a `ScanForPII` case asserting the Luhn-invalid 16-digit example from the Story's GWT does not match.
- Files: `server/services/pii_scanner_test.go`

#### Story 1.2.3: Redaction sentinel + message formatting

**As a** developer wiring PII-scan into the approval handler, **I want** a PII-specific redaction sentinel and message builders, **so that** a reviewer/auditor can distinguish a PII redaction from a secret redaction, and the escalate/deny paths have ready-made user-facing text.

**Acceptance Criteria** (AC8 groundwork):
- `redactedPII` is a distinct string from `redactedSecret`.
  - *Given* both constants, *When* compared, *Then* `redactedPII != redactedSecret` and `redactedPII == "[REDACTED: PII detected]"`.
- `FormatPIIEscalationReason` names the pattern and field.
  - *Given* `patternName = "Social Security Number"`, `field = "command"`, *When* `FormatPIIEscalationReason(patternName, field)` is called, *Then* it returns `"Detected Social Security Number in command — escalated for manual review."`.

**Files**: `server/services/ai_interfaces.go`, `server/services/pii_scanner.go`

##### Task 1.2.3a: Add `redactedPII` + message formatters (~3 min)
- In `ai_interfaces.go`, add `redactedPII = "[REDACTED: PII detected]"` to the existing redaction-sentinel `const` block (alongside `redactedSecret`/`redactedPrompt`), with a doc comment distinguishing it from `redactedSecret`.
- In `pii_scanner.go`, add `func FormatPIIEscalationReason(patternName, field string) string` and `func FormatPIIDenyMessage(patternName string) string` (mirroring `FormatSecretDenyMessage`'s shape), per the Story's GWT text.
- Files: `server/services/ai_interfaces.go`, `server/services/pii_scanner.go`

---

## Phase 2: Config Schema

### Epic 2.1: `PIIScanningConfig`

**Goal**: A JSON-configurable `PIIScanningConfig` living in `config/types.go`/`config/config.go`, following the existing `TmuxExecGateConfig`/`SessionRetentionConfig` nested-struct-with-`OrDefault()` convention exactly.

#### Story 2.1.1: Config struct, defaults, and Config field

**As an** operator, **I want** `pii_scanning.enabled`/`custom_patterns`/`on_detection`/`skip_path_patterns` in the existing JSON config, **so that** I can tune or disable PII scanning without a code change (AC9).

**Acceptance Criteria** (AC9):
- Config round-trips through JSON with defaults applied when fields are absent.
  - *Given* a fresh `config.json` with no `pii_scanning` key at all, *When* loaded via `LoadConfigFromPath`, *Then* `cfg.PIIScanning.EnabledOrDefault() == true`, `cfg.PIIScanning.OnDetectionOrDefault() == "escalate"`, and `cfg.PIIScanning.SkipPathPatternsOrDefault()` returns the built-in default list (non-empty).
  - *Given* `config.json` containing `{"pii_scanning": {"enabled": false}}`, *When* loaded, *Then* `cfg.PIIScanning.EnabledOrDefault() == false`.
  - *Given* `config.json` containing `{"pii_scanning": {"skip_path_patterns": []}}` (explicit empty array, not absent), *When* loaded, *Then* `cfg.PIIScanning.SkipPathPatternsOrDefault()` returns an empty slice, **not** the built-in defaults — an explicit empty list means "no skip patterns," distinct from an absent key meaning "use the built-in list."

**Files**: `config/types.go`, `config/config.go`

##### Task 2.1.1a: Add `PIIScanningConfig` struct + `OrDefault()` methods to `config/types.go` (~5 min)
- Add `const PIIOnDetectionEscalate = "escalate"` and `const PIIOnDetectionDeny = "deny"`.
- Add `var defaultPIISkipPathPatterns = []string{"testdata/", "/fixtures/", "/mocks/", "_test.go", ".test.ts", ".test.tsx", ".spec.ts"}`.
- Add `type PIIScanningConfig struct { Enabled *bool; CustomPatterns []string; OnDetection string; SkipPathPatterns []string }` with `json:"enabled,omitempty"` / `json:"custom_patterns,omitempty"` / `json:"on_detection,omitempty"` / `json:"skip_path_patterns,omitempty"` tags and doc comments per the Domain Glossary entries above.
- Add `func (c PIIScanningConfig) EnabledOrDefault() bool` (nil → true, matching `SessionRetentionConfig.EnabledOrDefault`'s pattern at `config/types.go:49-54`).
- Add `func (c PIIScanningConfig) OnDetectionOrDefault() string` (returns `PIIOnDetectionDeny` only when `OnDetection == PIIOnDetectionDeny`, else `PIIOnDetectionEscalate`).
- Add `func (c PIIScanningConfig) SkipPathPatternsOrDefault() []string` (returns `defaultPIISkipPathPatterns` only when `c.SkipPathPatterns == nil`, i.e. the key was absent from JSON — an explicit `[]` stays `[]`).
- Files: `config/types.go`

##### Task 2.1.1b: Add `PIIScanning` field to `Config` struct (~2 min)
- In `config/config.go`, add `PIIScanning PIIScanningConfig `json:"pii_scanning,omitempty"`` to the `Config` struct, placed after the `SessionRetention` field (~line 343) alongside the other nested feature-config fields, with a one-line doc comment referencing `server/services/pii_scanner.go`.
- Files: `config/config.go`

##### Task 2.1.1c: Config round-trip + defaults tests (~5 min)
- In `config/config_test.go`, add `TestPIIScanningConfigRoundTrip` (mirrors `TestNotificationPrefsRoundTrip` at `config_test.go:529-541`) and `TestPIIScanningConfigDefaults` covering the three GWT cases in Story 2.1.1 above (absent key, explicit `false`, explicit empty `skip_path_patterns`).
- Files: `config/config_test.go`

---

## Phase 3: ApprovalHandler Wiring

### Epic 3.1: PII scan call site

**Goal**: Wire `ScanForPII`/`extractWriteContent`/config into `HandlePermissionRequest`, inserted between the secret-scan block and the domain-age block, following the domain-age branch's `goto createApproval` shape for the escalate path and the secret-scan branch's `return` shape for the deny path — plus extend secret-scan itself to also scan Write/Edit content (AC3's explicit scope decision).

#### Story 3.1.1: `ApprovalHandler` config field + setter

**As a** developer wiring config into the handler, **I want** `SetPIIScanningConfig`, **so that** `server.go` can inject the live config the same way it injects the domain checker/analytics store/poll interval.

**Acceptance Criteria**:
- Calling `SetPIIScanningConfig` with `CustomPatterns: []string{"[invalid("}` does not panic and compiles zero patterns from that entry.
  - *Given* `cfg := config.PIIScanningConfig{CustomPatterns: []string{"employee-id-\\d{6}", "[invalid("}}`, *When* `h.SetPIIScanningConfig(cfg)` is called, *Then* `h.piiCustomPatterns` contains exactly one `namedPattern` (for `employee-id-\d{6}`), and a warning is logged for the invalid entry — no panic, no startup failure.

**Files**: `server/services/approval_handler.go`

##### Task 3.1.1a: Add `piiConfig`/`piiCustomPatterns` fields + `SetPIIScanningConfig` setter (~5 min)
- Add `piiConfig config.PIIScanningConfig` and `piiCustomPatterns []namedPattern` fields to the `ApprovalHandler` struct (`approval_handler.go:68-83`), with doc comments matching the `domainChecker`/`classifier` field style ("optional: ...").
- Add `import "github.com/tstapler/stapler-squad/config"` (already used elsewhere in this package, e.g. `approval_service.go:9`, `session_retention_sweeper.go:9` — no import-cycle risk).
- Add `func (h *ApprovalHandler) SetPIIScanningConfig(cfg config.PIIScanningConfig)`: store `h.piiConfig = cfg`; for each string in `cfg.CustomPatterns`, `regexp.Compile` it (reject/skip + `log.Warn` on error, per the Story's GWT and `pitfalls.md`'s "compile once at config-load time, fail fast" recommendation), append successes to `h.piiCustomPatterns` as `namedPattern{Name: "Custom PII pattern", Pattern: compiled}`.
- Files: `server/services/approval_handler.go`

#### Story 3.1.2: Path-exclusion helper

**As a** reviewer in a fixture-heavy repo, **I want** PII scanning to skip known test/fixture paths, **so that** the review queue isn't flooded with expected fixture data (resolution of Open Question 2).

**Acceptance Criteria**:
- *Given* `cwd = "/repo/testdata/fixtures"`, `filePath = ""`, `patterns = defaultPIISkipPathPatterns`, *When* `skipPIIScanForPath(cwd, filePath, patterns)` is called, *Then* it returns `true`.
- *Given* `cwd = "/repo/src"`, `filePath = "internal/handler.go"`, `patterns = defaultPIISkipPathPatterns`, *When* called, *Then* it returns `false`.

**Files**: `server/services/content_scanner.go`, `server/services/content_scanner_test.go`

##### Task 3.1.2a: Add `skipPIIScanForPath` (~4 min)
- Add `func skipPIIScanForPath(cwd, filePath string, patterns []string) bool` to `content_scanner.go`: for each `p` in `patterns`, return `true` if `strings.Contains(cwd, p) || strings.Contains(filePath, p)`.
- Files: `server/services/content_scanner.go`

##### Task 3.1.2b: Tests for `skipPIIScanForPath` (~4 min)
- Table-driven test covering the Story's two GWT cases plus an empty-`patterns` case (always false).
- Files: `server/services/content_scanner_test.go`

#### Story 3.1.3: Secret-scan content-scan extension + PII-scan block

**As an** approval-hook maintainer, **I want** the secret scanner to also cover Write/Edit content (not just Bash commands) and a new PII-scan block inserted right after it, **so that** both AC2 (PII on Bash) and AC3 (PII on file writes) are satisfied, and secret-scan's own historical Bash-only gap (confirmed by `architecture.md` §3's exhaustive grep) is closed using the same shared mechanism.

**Acceptance Criteria** (AC2, AC3, AC4, AC7, AC8, AC10):
- Bash command PII escalates (AC2, AC4, AC7).
  - *Given* `payload.ToolName = "Bash"`, `payload.ToolInput["command"] = "curl -d ssn=219-09-9999 http://internal"`, `pii_scanning.enabled` default (true), *When* `HandlePermissionRequest` runs, *Then* it does **not** call the classifier (falls through via `goto createApproval` before reaching it), and a `PendingApproval` is created with `EscalationCategory == "pii-scan"`, `RiskLevel == classifier.RiskCritical`.
- Write content PII escalates (AC3).
  - *Given* `payload.ToolName = "Write"`, `payload.ToolInput = {"file_path": "/repo/src/seed.sql", "content": "INSERT INTO users VALUES ('john@company.com')"}`, *When* `HandlePermissionRequest` runs, *Then* the same escalation occurs, citing field `"content"` in the reason text.
- Analytics redaction is field-scoped, not blanket (AC8).
  - *Given* the Bash example above, *When* `analyticsStore.RecordFromResult` is called, *Then* the recorded `ToolInput["command"] == redactedPII`, and no other `ToolInput` key is altered.
- Secret-scan's existing Bash-only auto-deny behavior is unchanged for the command field (AC10).
  - *Given* the existing secret command `` `curl -H "Authorization: Bearer ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234" https://api.example.com"` ``, *When* run through the modified handler, *Then* `TestApprovalHandler_SecretNotPersistedToAnalytics` (unmodified) still passes byte-for-byte.
- Secret-scan is additionally extended to Write/Edit content (the AC3 scope decision made explicit in this plan).
  - *Given* `payload.ToolName = "Write"`, `payload.ToolInput = {"file_path": "/repo/.env", "content": "AWS_ACCESS_KEY_ID=AKIAABCDEFGHIJKLMNOP"}`, *When* `HandlePermissionRequest` runs, *Then* it auto-denies (secret-scan's existing behavior extended to the new `content` field), with `sanitizedInput["content"] == redactedSecret`.

**Files**: `server/services/approval_handler.go`

##### Task 3.1.3a: Add a shared per-field redact-and-record helper, then extend secret-scan to Write/Edit content (~5 min)
- Once this Story is done there are **four** redact-then-record call sites (secret-scan on `command`, secret-scan on the new write-content field, PII-scan escalate, PII-scan deny) — enough real callers to justify a shared helper, unlike the single-caller cases the Pattern Decisions table rejects abstractions for. Add `func (h *ApprovalHandler) recordRedactedFinding(payload classifier.PermissionRequestPayload, field, sentinel string, result classifier.ClassificationResult, sessionID string)` to `approval_handler.go`: no-ops if `h.analyticsStore == nil`; otherwise shallow-copies `payload.ToolInput`, replaces `[field]` with `sentinel`, builds `sanitizedPayload`, and calls `h.analyticsStore.RecordFromResult(sanitizedPayload, result, sessionID, "", 0)` — this is exactly the existing `:233-239` block, extracted and parameterized by `field`/`sentinel` instead of hardcoding `"command"`/`redactedSecret`.
- Rewrite the existing secret-scan block's redaction call to use `h.recordRedactedFinding(payload, "command", redactedSecret, result, sessionID)`.
- In the existing secret-scan block (`approval_handler.go:223-251`), after the existing `command`-only check, add a second check using `extractWriteContent(payload.ToolName, payload.ToolInput)`: if `content != "" && !looksBinary(content)`, run `ScanForSecrets(content)`; on a hit, call `h.recordRedactedFinding(payload, field, redactedSecret, result, sessionID)` (where `field` is whatever `extractWriteContent` returned), then `h.writeDecision(w, "deny", msg); return` — identical auto-deny shape to the command-field check, now reusable via the helper.
- Files: `server/services/approval_handler.go`

##### Task 3.1.3b: Insert the PII-scan escalate-path block (~5 min)
- Insert a new block between the (now-extended) secret-scan block and the domain-age block (`approval_handler.go:251`–`:260`), gated on `h.piiConfig.EnabledOrDefault()`.
- Build the scan target list: `command` (if present) and the `extractWriteContent` result (if present and `!looksBinary`).
- Before scanning, check `skipPIIScanForPath(payload.Cwd, filePath, h.piiConfig.SkipPathPatternsOrDefault())` — `filePath, _ := payload.ToolInput["file_path"].(string)` — and skip the whole block if it returns true.
- For each target, call `ScanForPII(target.text, h.piiCustomPatterns)`. On a hit where `h.piiConfig.OnDetectionOrDefault() == config.PIIOnDetectionEscalate` (the default): build `piiEscalation := classifier.ClassificationResult{Decision: classifier.Escalate, RiskLevel: classifier.RiskCritical, RuleID: classifier.RuleIDPIIScan, RuleName: "PII Detection", Reason: FormatPIIEscalationReason(hit.PatternName, target.field)}`; call `h.recordRedactedFinding(payload, target.field, redactedPII, piiEscalation, sessionID)` (the Task 3.1.3a helper); log via `log.ForSession(sessionID).Info(...)` per the Observability Plan; set `escalation = piiEscalation`; `goto createApproval`.
- Files: `server/services/approval_handler.go`

##### Task 3.1.3c: Add the PII-scan deny-path branch (~4 min)
- Within the same loop from Task 3.1.3b, when `h.piiConfig.OnDetectionOrDefault() == config.PIIOnDetectionDeny`: build `msg := FormatPIIDenyMessage(hit.PatternName)` and `denyResult := classifier.ClassificationResult{Decision: classifier.AutoDeny, RiskLevel: classifier.RiskCritical, RuleID: classifier.RuleIDPIIScan, RuleName: "PII Detection", Reason: msg}`; call `h.recordRedactedFinding(payload, target.field, redactedPII, denyResult, sessionID)`; log; then `h.writeDecision(w, "deny", msg); return` — mirroring the secret-scan block's return shape exactly, per the Pattern Decisions table's "PII scan vs secret scan decision-on-match" row.
- Files: `server/services/approval_handler.go`

#### Story 3.1.4: Server wiring

**As an** operator, **I want** the live config's `PIIScanning` settings threaded into the running `ApprovalHandler`, **so that** editing `config.json` and restarting actually changes scanning behavior (closing the loop on AC9).

**Acceptance Criteria**:
- *Given* `deps.Config.PIIScanning.Enabled` set to `false` in the live config, *When* the server starts and wires `approvalHandler`, *Then* `approvalHandler.piiConfig.EnabledOrDefault() == false`.

**Files**: `server/server.go`

##### Task 3.1.4a: Call `SetPIIScanningConfig` in the wiring block (~2 min)
- In `server/server.go`, in the `approvalHandler` wiring block (~line 488, alongside `approvalHandler.SetDomainChecker(...)`), add `approvalHandler.SetPIIScanningConfig(deps.Config.PIIScanning)`.
- Files: `server/server.go`

---

## Phase 4: Escalation Taxonomy

### Epic 4.1: Go taxonomy

**Goal**: Add the `pii-scan` category to `pkg/classifier/escalation.go` so `CategorizeEscalationRuleID` doesn't silently misfile PII escalations as `explicit-rule` (the single highest-likelihood "ships broken and nobody notices" risk per `pitfalls.md`).

#### Story 4.1.1: `EscalationPIIScan` + `RuleIDPIIScan`

**As a** developer, **I want** the PII category recognized by the Go-side categorization switch, **so that** `PendingApproval.EscalationCategory` is `"pii-scan"`, not a silently-wrong `"explicit-rule"` (AC4).

**Acceptance Criteria** (AC4):
- *Given* `RuleID = classifier.RuleIDPIIScan`, *When* `CategorizeEscalationRuleID(RuleID)` is called, *Then* it returns `EscalationPIIScan` (`"pii-scan"`), not the `default:` fallback `EscalationExplicitRule`.

**Files**: `pkg/classifier/escalation.go`, `pkg/classifier/escalation_test.go`

##### Task 4.1.1a: Add constants + switch case (~3 min)
- Add `EscalationPIIScan EscalationCategory = "pii-scan"` to the `EscalationCategory` const block (`escalation.go:10-30`), with a doc comment mirroring `EscalationSecretScan`'s style.
- Add `RuleIDPIIScan = "pii-scan"` to the sentinel `RuleID` const block (`escalation.go:36-52`).
- Add `case RuleIDPIIScan: return EscalationPIIScan` to `CategorizeEscalationRuleID`'s switch (`escalation.go:60-75`), placed after the `RuleIDSecretScan` case.
- Files: `pkg/classifier/escalation.go`

##### Task 4.1.1b: Add table row to `TestCategorizeEscalationRuleID` (~2 min)
- Add `{"pii-scan sentinel", RuleIDPIIScan, EscalationPIIScan}` to the existing table in `escalation_test.go:9-21` — the exact regression test `pitfalls.md` recommended by name.
- Files: `pkg/classifier/escalation_test.go`

### Epic 4.2: TS taxonomy

**Goal**: Mirror the Go union member in TypeScript, plus a same-file sync-guard test per `pitfalls.md`'s explicit recommendation ("even a hardcoded parallel array... is strictly better than only a shared code comment").

#### Story 4.2.1: `EscalationCategory` union + sync-guard test

**As a** frontend developer, **I want** `"pii-scan"` in the TS union and a test that fails loudly if the two enumerations ever drift, **so that** the badge/label maps below can reference it with compile-time safety.

**Acceptance Criteria** (AC4):
- *Given* the TS `EscalationCategory` union, *When* type-checked against a literal `"pii-scan"`, *Then* it compiles without error.
- *Given* a hardcoded array of the 7 expected category literals, *When* the sync-guard test runs, *Then* it asserts the array's length and contents match — a future addition to one side without the other fails this test.

**Files**: `web-app/src/lib/sessions/escalationCategory.ts`, `web-app/src/lib/sessions/escalationCategory.test.ts` (new)

##### Task 4.2.1a: Add `"pii-scan"` to the union (~2 min)
- Add `| "pii-scan"` to the `EscalationCategory` union in `escalationCategory.ts:5-11`, keeping the existing comment about hand-syncing with `pkg/classifier/escalation.go`.
- Files: `web-app/src/lib/sessions/escalationCategory.ts`

##### Task 4.2.1b: Create `escalationCategory.test.ts` sync-guard (~5 min)
- New file. Define `const EXPECTED_CATEGORIES: EscalationCategory[] = ["no-match", "explicit-rule", "domain-age", "secret-scan", "pii-scan", "unclassifiable", "unexpected"]` (hardcoded, matching `pkg/classifier/escalation.go`'s const block by hand). Add a test asserting `ESCALATION_CATEGORY_LABELS` (imported from `ApprovalAnalyticsPanel.tsx`, or re-declared minimally if not exported — check exportability first) has exactly these keys, via `Object.keys(...).sort()` compared to `EXPECTED_CATEGORIES.slice().sort()`. This is the "same-file test asserting the TS union's literal set" `pitfalls.md` recommended.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="escalationCategory.test"` to verify.
- Files: `web-app/src/lib/sessions/escalationCategory.test.ts`

---

## Phase 5: Review Queue & Analytics UI

### Epic 5.1: Review queue badge

**Goal**: The `🔒 PII` badge (AC5) — a single map entry, no new component, per `ux.md`'s conclusion.

#### Story 5.1.1: `ESCALATION_REASON_EMOJI` entry

**As a** developer reviewing a PII-flagged item, **I want** a `🔒` prefix on the escalation reason line, **so that** I can visually distinguish it from `no-match`/`explicit-rule`/`domain-age`/`unclassifiable`/`unexpected` items in the queue (AC5).

**Acceptance Criteria** (AC5):
- *Given* `queueItem.metadata["escalation_reason_category"] = "pii-scan"` and `queueItem.metadata["escalation_reason"] = "Detected Social Security Number in command — escalated for manual review."`, *When* `ReviewQueuePanel` renders that item, *Then* the reason paragraph reads `"🔒 Detected Social Security Number in command — escalated for manual review."`.

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 5.1.1a: Add the map entry (~2 min)
- Add `"pii-scan": "🔒",` to `ESCALATION_REASON_EMOJI` (`ReviewQueuePanel.tsx:141-147`), and update the comment above it (`:138-140`) to note `pii-scan` is present (unlike `secret-scan`, since PII escalates and does reach the queue).
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

### Epic 5.2: Analytics label

**Goal**: The "PII detections" row in the existing Escalation Reasons table (AC6) — again a single map entry, no new stat card.

#### Story 5.2.1: `ESCALATION_CATEGORY_LABELS` entry

**As a** team lead reviewing analytics, **I want** a labeled "PII detected in request" row with a count, **so that** I can see PII-escalation volume without a new UI surface (AC6).

**Acceptance Criteria** (AC6):
- *Given* `summary.escalationReasonCounts = {"pii-scan": 3}`, *When* `ApprovalAnalyticsPanel` renders, *Then* the Escalation Reasons table shows a row with label `"PII detected in request"` and count `3`.

**Files**: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`, `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx`

##### Task 5.2.1a: Add the map entry (~2 min)
- Add `"pii-scan": "PII detected in request",` to `ESCALATION_CATEGORY_LABELS` (`ApprovalAnalyticsPanel.tsx:98-105`) — this is a full `Record<EscalationCategory, string>`, so the TS compiler enforces this entry exists once the union grows (Task 4.2.1a); skipping it is a compile error, not a silent gap.
- Files: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

##### Task 5.2.1b: Extend the Escalation Reasons test (~3 min)
- In `ApprovalAnalyticsPanel.test.tsx`'s `"Escalation Reasons"` describe block (`:329+`), add `"pii-scan": 4` to the existing `mockSummary.escalationReasonCounts` object in the `"renders Escalation Reasons table with mapped labels and counts"` test, and add `expect(screen.getByText("PII detected in request")).toBeInTheDocument()` + `expect(screen.getByText("4")).toBeInTheDocument()`.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="ApprovalAnalyticsPanel.test"` to verify.
- Files: `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx`

---

## Phase 6: Backend Integration Tests

### Epic 6.1: End-to-end `HandlePermissionRequest` coverage

**Goal**: Prove the full request → escalate/deny → analytics-redaction → queue chain works, mirroring the existing `approval_handler_secret_test.go`/`TestHandlePermissionRequest_EscalationReason_DomainAge` precedents exactly, in a new `approval_handler_pii_test.go`.

#### Story 6.1.1: Escalate-path integration tests

**As a** maintainer, **I want** end-to-end tests proving a PII-bearing Bash command escalates with the right category and redacted analytics, **so that** AC2/AC4/AC7/AC8 are verified together, not just unit-by-unit.

**Acceptance Criteria**: covered by the GWT examples in Story 3.1.3 above (Bash SSN example).

**Files**: `server/services/approval_handler_pii_test.go` (new)

##### Task 6.1.1a: `TestHandlePermissionRequest_EscalationReason_PIIScan` (~5 min)
- Mirror `TestHandlePermissionRequest_EscalationReason_DomainAge` (`approval_handler_test.go:238-265`) exactly: `h, store := newTestHandler(5 * time.Second)`; no domain checker/classifier needed (PII-scan short-circuits before both, same as domain-age); `go waitForFirstApprovalThenResolve(t, store, &captured)`; `postPermissionRequestWithCommand(t, h, "test-session", "Bash", "curl -d ssn=219-09-9999 http://internal")`; assert `captured.EscalationCategory == "pii-scan"` and `captured.EscalationReason == "Detected Social Security Number in command — escalated for manual review."`.
- Files: `server/services/approval_handler_pii_test.go`

##### Task 6.1.1b: `TestApprovalHandler_PIINotPersistedToAnalytics` (~5 min)
- Mirror `TestApprovalHandler_SecretNotPersistedToAnalytics` (`approval_handler_secret_test.go:57-101`) exactly, but for PII: `h, analyticsStore := newTestHandlerWithAnalytics(t)`; post a command containing the SSN example; assert (via `analyticsStore.LoadWindow`) that the persisted entry's `CommandPreview == redactedPII` and `assert.NotContains(t, e.CommandPreview, "219-09-9999")`.
- Files: `server/services/approval_handler_pii_test.go`

#### Story 6.1.2: Write-content integration test

**As a** maintainer, **I want** a test proving PII in Write tool content escalates, **so that** AC3 (the genuinely new coverage surface) is exercised end-to-end, not just at the `extractWriteContent`/`ScanForPII` unit level.

**Files**: `server/services/approval_handler_pii_test.go`

##### Task 6.1.2a: `TestHandlePermissionRequest_PIIScan_WriteContent` (~5 min)
- New helper `postPermissionRequestWithToolInput(t, h, sessionID, toolName string, toolInput map[string]interface{}) *httptest.ResponseRecorder` (generalizes `postPermissionRequestWithCommand`, which becomes a one-line wrapper calling it with `map[string]interface{}{"command": command}`). Post `toolName: "Write"`, `toolInput: {"file_path": "/repo/src/seed.sql", "content": "INSERT INTO users VALUES ('john@company.com')"}`, `cwd: "/repo/src"` (not a skip-path). Assert escalation with `EscalationCategory == "pii-scan"` and reason text citing field `"content"`.
- Files: `server/services/approval_handler_pii_test.go`, `server/services/approval_handler_secret_test.go` (refactor `postPermissionRequestWithCommand` into the new generalized helper — keep the old name as a thin wrapper so existing call sites are untouched)

#### Story 6.1.3: `on_detection: "deny"` mode test

**As an** operator running a stricter environment, **I want** a test proving `on_detection: "deny"` actually auto-denies instead of escalating, **so that** the config knob (AC9) is verified, not just assumed from the code.

**Files**: `server/services/approval_handler_pii_test.go`

##### Task 6.1.3a: `TestHandlePermissionRequest_PIIScan_DenyMode` (~4 min)
- `h.SetPIIScanningConfig(config.PIIScanningConfig{OnDetection: config.PIIOnDetectionDeny})`. Post the SSN command. Assert the HTTP response decodes to `resp.HookSpecificOutput.Decision.Behavior == "deny"` (not an escalation reaching the store).
- Files: `server/services/approval_handler_pii_test.go`

#### Story 6.1.4: Path-exclusion + disabled-config tests

**As a** maintainer, **I want** tests proving the skip-path list and the `enabled: false` kill switch both actually suppress escalation, **so that** the Risk Control section's rollback levers are proven to work, not just documented.

**Files**: `server/services/approval_handler_pii_test.go`

##### Task 6.1.4a: `TestHandlePermissionRequest_PIIScan_SkipsTestdataPath` (~4 min)
- Post `toolName: "Bash"`, `command: "echo ssn=219-09-9999"`, `cwd: "/repo/testdata/fixtures"` (default config, no explicit `SkipPathPatterns` override — relies on the built-in default). Assert **no** `PendingApproval` is created within a short timeout (use `testutil.WaitForCondition` with a "still empty" assertion, or assert the response is a plain `allow`/whatever the no-classifier default resolves to — confirm exact expected behavior against `newTestHandler`'s zero-classifier default before writing the assertion).
- Files: `server/services/approval_handler_pii_test.go`

##### Task 6.1.4b: `TestHandlePermissionRequest_PIIScan_DisabledConfig` (~3 min)
- `h.SetPIIScanningConfig(config.PIIScanningConfig{Enabled: boolPtr(false)})` (add a local `boolPtr` helper if one doesn't already exist in this test package — grep first). Post the SSN command. Assert no `pii-scan` escalation occurs.
- Files: `server/services/approval_handler_pii_test.go`

#### Story 6.1.5: Secret-scan regression + extension coverage

**As a** maintainer, **I want** proof that secret-scan's existing behavior is unchanged and its new content-scan coverage works, **so that** AC10 is verified, not assumed from "the diff looks additive."

**Files**: `server/services/approval_handler_pii_test.go`, `server/services/approval_handler_secret_test.go`

##### Task 6.1.5a: Run existing secret-scan tests unmodified + add content-scan coverage (~4 min)
- Run `go test ./server/services/... -run TestApprovalHandler_Secret` and confirm both existing tests (`TestApprovalHandler_SecretNotPersistedToAnalytics`, `TestApprovalHandler_LoadWindow_ContainsNoSecret`) still pass unmodified (AC10).
- Add `TestApprovalHandler_SecretScan_ContentAlsoScanned`: post `toolName: "Write"`, `toolInput: {"file_path": "/repo/.env", "content": "AWS_ACCESS_KEY_ID=AKIAABCDEFGHIJKLMNOP"}` via the Task 6.1.2a helper; assert the response is `"deny"` and analytics shows `redactedSecret` in place of `content`.
- Files: `server/services/approval_handler_pii_test.go`

##### Task 6.1.6a: Manual fixture-noise verification pass (Risk Control staged rollout) (~5 min)
- Manually run `ScanForPII` against a sample of this repo's own fixture-heavy content (a few files under `server/services/testdata/` if present, a `web-app/**/*.test.ts` snippet with mock user data) via a throwaway `go run` scratch script or an ad-hoc test — confirm the default `skip_path_patterns` list actually suppresses expected fixture paths, and note any additional default patterns needed before treating `Enabled: true` as safe. Not a permanent test — a one-time manual verification gate per the Risk Control section.
- Files: none persisted (manual verification step; document findings in the PR description, not a new file)

---

## Post-Implementation Checklist

- [ ] `make build && make test` green (Go).
- [ ] `cd web-app && npx jest --no-coverage --testPathPatterns="escalationCategory.test|ApprovalAnalyticsPanel.test"` green.
- [ ] `make lint` green.
- [ ] `make registry-generate` run and any changed `docs/registry/features/*.json` committed, since this adds no new RPC/UI feature marker but does touch existing backend/frontend files — confirm no registry drift, per `.claude/rules/feature-registry.md`.
- [ ] Manual fixture-noise verification (Task 6.1.6a) findings noted in the PR description.
