# Research: Pitfalls in PII/Secret Detection for an Approval Gate

Phase 2 research for `project_plans/pii-scanning/requirements.md`. Scope: what commonly goes
wrong building this class of scanner, grounded against this repo's actual code
(`server/services/secret_scanner.go`, `server/services/approval_handler.go`,
`server/services/approval_store.go`, `server/services/analytics_store.go`,
`pkg/classifier/escalation.go`, `web-app/src/lib/sessions/escalationCategory.ts`).

## 1. Regex false-positive rate on realistic data

- **Credit card without Luhn check**: any bare `\b\d{16}\b`-style regex (the issue's example is
  Visa-prefix-only, per requirements.md open question 4) matches every 16-digit number —
  UUIDs-as-digits, git commit-adjacent hashes, database IDs, `git log` output, Docker image
  digests, timestamps concatenated in test fixtures. **VERIFIED**: no Luhn implementation
  exists anywhere in this repo today (`grep -rin luhn` across all non-worktree `.go` files
  returns zero hits) — this is a from-scratch addition, not a reuse of existing logic.
  Un-Luhn-checked, this pattern will fire constantly on seed/fixture data, which is exactly the
  content most likely to pass through Write/Edit tool calls in an agent session.
- **SSN vs. phone/zip collision**: `\d{3}-\d{2}-\d{4}` collides with any hyphenated numeric
  fixture of the same shape — order IDs, invoice numbers, zip+4 codes written with a stray
  hyphen, test phone numbers formatted non-standard. There is no positional or contextual
  signal (e.g. a `ssn:` key nearby) proposed in the issue to disambiguate.
- **Email regex over-matching**: a permissive email regex matches `noreply@example.com`,
  `test@test.test`, and every placeholder address in test fixtures and `.env.example` files —
  none of which are real PII. `secret_scanner.go`'s own patterns show the project's established
  practice of keeping patterns "intentionally conservative to minimize false positives"
  (comment at `server/services/secret_scanner.go:15`) — a bare email/SSN pattern set breaks that
  precedent immediately.
- **Design implication**: requirements.md's open question 2 already flags this and proposes
  either path-based skip (`testdata/`/`fixtures/`) or absorbing the noise via ESCALATE instead
  of DENY. Given the ESCALATE decision (open question 1, recommended default), the fallout of a
  high false-positive rate is reviewer fatigue in the queue, not blocked work — survivable, but
  should be scoped explicitly in `plan.md` rather than left as "ship the naive regex and see."
  Luhn validation for credit cards is close to free (a few lines, no new dependency) and should
  be treated as in-scope for the first cut, not deferred — the alternative is a scanner whose
  single most common trigger (any 16-digit number) is wrong most of the time.

## 2. Performance pitfalls

- **Scan scope expansion to file content is a step change in payload size.** Today
  `ScanForSecrets` caps input at 4096 bytes (`server/services/secret_scanner.go:54-56`) and only
  ever sees `payload.ToolInput["command"]`, which is bounded by shell command-line length in
  practice. Requirements.md's AC3 extends scanning to `ToolInput["content"]`/`["new_string"]` for
  Write/Edit — these can be multi-KB to multi-MB file bodies (a generated lockfile, a large seed
  SQL dump, a vendored file). **This must carry the same explicit size cap** (or a
  content-type-aware one) — scanning full multi-MB content against ~10+ patterns on every single
  Write/Edit tool call, synchronously in the approval hook's request path (this is a blocking
  HTTP handler per `approval_handler.go:198-251`, not a background job), adds latency to every
  file write in every session. The existing 4096-byte cap on commands was presumably sized for
  command-line content, not file content, and reusing it verbatim would silently truncate large
  legitimate files before ever reaching the PII patterns.
- **RE2 does not have catastrophic backtracking, but "no ReDoS" is not the same as "no cost."**
  Go's `regexp` package compiles to RE2 automata, guaranteeing linear-time matching in input
  length regardless of pattern — so the classic ReDoS failure mode (exponential backtracking on
  pathological input) is structurally not reachable here. However:
  - Linear-time-per-pattern still means **O(patterns × input length)** total cost per scan call,
    run synchronously per tool call. Today's list is ~10 patterns over ≤4096 bytes — cheap. A
    combined secret+PII list scanning full file content multiplies both factors.
  - RE2's automaton construction has a **bounded DFA cache**; when a pattern's DFA state count
    exceeds the cache budget, Go's `regexp` silently falls back to the slower NFA simulation path
    per-match rather than erroring — this is a quiet performance cliff, not a crash, so it would
    show up as intermittent approval-latency regressions rather than a test failure. None of the
    current `secretPatterns` are complex enough to trigger this, but a naive SSN/CC pattern
    combined with **unvalidated user-supplied `custom_patterns`** (AC9 / open question 5) is the
    concrete risk: a user-authored regex with wide character classes and many alternations could
    hit this path. Custom patterns should be compiled once at config-load time (fail fast, not
    per-request) and ideally sanity-checked for pattern complexity/length before being accepted
    into the active pattern list.
  - No streaming/early-exit: `ScanForSecrets` runs every pattern against the full text even
    after the first match candidate for a *later* pattern would already have decided the
    outcome, because patterns are checked in list order with early return only on the *first*
    match (`server/services/secret_scanner.go:57-61`) — fine at current list size, worth
    re-checking cost if the merged PII+secret list grows substantially per open question 3.

## 3. Redaction correctness before logs/analytics

This is the pitfall most specific to this repo's actual architecture, and where a naive port of
the secret-scan precedent will under-protect PII, because **PII is designed to escalate, not
auto-deny** — and escalation persists the request for human review by design.

- **What's already safe today**: `AnalyticsStore.RecordFromResult`
  (`server/services/analytics_store.go:163-187`) only ever derives `CommandPreview` from
  `payload.ToolInput["command"]` or `["file_path"]` — never from `["content"]`/`["new_string"]`.
  So even once content scanning is added, the ent/SQLite-backed `AnalyticsEntry`
  (`session/storage.go:637` → ent-backed `AnalyticsData`) will **not** pick up raw file content
  as a side effect of today's code shape, *provided the PII scanner's redaction hook only touches
  the same `command`/`file_path` sanitization pattern the secret scanner uses*
  (`approval_handler.go:230-239`, shallow-copies `ToolInput` and replaces `["command"]` with the
  `redactedSecret` sentinel before calling `RecordFromResult`). This must be verified, not
  assumed, once content scanning is implemented — the redaction call site needs to also
  sanitize `content`/`new_string` in the copy, or a future change to `RecordFromResult` that
  starts including content in the preview would silently reintroduce the leak.
- **What is NOT safe today, and is the real gap**: `ApprovalStore` persists the **entire raw
  `ToolInput` map** — unredacted — to a JSON file on disk for any request that reaches the
  pending-approval queue (`PendingApproval.ToolInput` / `PersistedApproval.ToolInput`,
  `server/services/approval_store.go:26,54`, written via `persistToDiskLocked()` →
  `os.WriteFile(tmpPath, data, 0600)` at line 340). This is **exactly the ESCALATE path** that
  requirements.md proposes for PII (AC4, open question 1) — unlike secret-scan's auto-DENY,
  which never creates a `PendingApproval` at all. This means: **the moment PII detection escalates
  a Write/Edit call with a detected SSN/email/credit-card in its content, that raw content is
  written verbatim to `~/.stapler-squad/.../approvals.json` (0600, but plaintext) and rendered
  in the review queue UI so a human can actually review it.** This is not a bug to "fix" by
  redacting — a reviewer approving/denying a PII-flagged write needs to see enough context to
  make the call — but it is a real design tension that AC8 ("matched PII text is redacted before
  persisted to session history/analytics logs") does not currently resolve, because AC8 as
  written maps cleanly onto secret-scan's *auto-deny* redaction pattern but not onto PII's
  *escalate* pattern, where the whole point is that a human needs to see the match. Planning
  needs to explicitly decide: (a) redact only the analytics/audit trail (which is already mostly
  safe per above) while leaving the pending-approval record intact for reviewer context, or
  (b) partially mask matched spans in the *displayed* content (e.g. show `***-**-6789` instead of
  the full SSN) even in the review queue. Option (b) is more protective but harder to implement
  correctly — see next point.
- **Partial redaction is its own failure mode.** If planning chooses masking (e.g. keep last 4
  digits of a credit card, mask an SSN's first 5), the masking logic itself needs to consistently
  redact *all* matches in the text, not just the first (mirroring `ScanForSecrets`'s current
  behavior of returning only the *first* pattern match's name, `secret_scanner.go:57-61` — good
  enough for a boolean auto-deny gate, insufficient for redaction, which needs every match
  location). A single-shot regex `.MatchString()` doesn't give you match spans;
  `FindAllStringIndex`/`FindAllString` is needed to redact all occurrences, and multiple
  overlapping pattern types (an SSN pattern and a phone pattern both partially matching the same
  digit run) can leave a "redacted" string that still leaks digits if redaction order/overlap
  isn't handled carefully.
- **Redaction leaking via secondary channels**: error messages, panics, and log lines that
  interpolate `payload` or `err` directly (rather than the sanitized copy) are a classic escape
  hatch — e.g. a future `fmt.Errorf("failed to scan content: %v", payload)` or a debug/trace log
  that dumps the full hook payload (`server/services/debug_snapshot.go` touches `ToolInput` —
  worth auditing during implementation that debug snapshots don't bypass the same sanitization).
  Any new PII-scanner code path must route logging exclusively through the pattern-name/hit
  metadata (as the existing secret-scan log line already does at
  `approval_handler.go:228`: logs `"pattern", hit.PatternName`, never the matched text itself) —
  this convention should be stated as an explicit rule in the plan, not left implicit.

## 4. Repo-stack-specific risks

- **Go `regexp` / RE2**: confirmed no catastrophic-backtracking risk (see §2) — this is a genuine
  advantage over a PCRE/backtracking-engine implementation and should be stated as a non-issue in
  the plan rather than defended against. The residual risk is DFA-cache fallback cost under
  large/complex custom patterns, not correctness.
  ent/SQLite persistence: `AnalyticsData` is the only ent-backed persistence surface a PII scan
  result would touch (`session/storage.go:637-667`, backing `AnalyticsEntry` in
  `server/services/analytics_store.go`), and per §3 it is bounded to `command`/`file_path`
  previews today — not a raw-content leak vector as currently coded, but this constraint is
  implicit in `RecordFromResult`'s current field selection, not an enforced contract; a future
  change to include `content` in the preview (e.g. to make Write/Edit analytics rows more useful)
  would need to also thread through the same sanitized-copy pattern, and there is nothing that
  would catch a regression here except manual review — worth a test that asserts
  `AnalyticsEntry.CommandPreview` never contains a raw PII match for a synthetic PII-content
  Write call, once content scanning ships. The actual persistence risk for PII specifically is
  the **non-ent JSON file** written by `ApprovalStore.persistToDiskLocked()` (see §3) — this is a
  gap in the requirements/AC list as written, since AC8 only names "session history / analytics
  logs," not the pending-approval JSON store.
- **Hand-mirrored Go/TS enum drift**: `pkg/classifier/escalation.go`'s `EscalationCategory` enum
  and `web-app/src/lib/sessions/escalationCategory.ts`'s TS union
  (`"no-match" | "explicit-rule" | "domain-age" | "secret-scan" | "unclassifiable" |
  "unexpected"`) are maintained by hand with **no codegen bridge** (stated explicitly in the TS
  file's own comment, `escalationCategory.ts:1-4`, which further points to
  `project_plans/escalation-reasoning/implementation/plan.md`'s rationale for rejecting a proto
  enum as overkill for a backend-only string key). Adding `EscalationPIIScan` (AC4) means editing
  **both** files by hand with **exactly matching string literals** — a typo or ordering mismatch
  (e.g. `"pii-scan"` vs `"pii_scan"`) silently breaks the `🔒 PII` badge lookup in
  `ReviewQueuePanel.tsx`'s `ESCALATION_REASON_EMOJI` map (requirements.md §"Review queue UI")
  with no compiler or type error on either side — the TS union is structurally typed against
  string literals, so a drifted value just fails to match any map key at runtime and silently
  renders no badge (not a crash, not a build failure — a silent visual regression). This is the
  single highest-likelihood "ships broken and nobody notices" pitfall in this feature, precisely
  because it has already happened once as an accepted trade-off (the TS comment documents the
  decision was deliberate) rather than an oversight — the plan should treat this touchpoint with
  the same explicit checklist discipline as `.claude/rules/session-creation-registry.md`'s
  7-touchpoint list, and ideally add a same-file test asserting the TS union's literal set
  matches the Go enum's — even a hardcoded parallel array in a Jest test that's manually kept in
  sync is strictly better than only a shared code comment.
- **Redaction sentinel reuse**: `redactedSecret` (`server/services/ai_interfaces.go:9-11`) is
  named/scoped for secrets specifically (`"[REDACTED: secret detected]"`). A PII redaction
  sentinel should be its own constant (e.g. `redactedPII = "[REDACTED: PII detected]"`) rather
  than reusing `redactedSecret` verbatim — conflating the two in logs/UI would make it impossible
  for a reviewer or auditor to tell after the fact whether a given redaction was a credential or
  a PII match, which matters for the "compliance logging" acceptance criterion (AC9/§6 of
  requirements.md).

## Summary of what should be explicitly designed against

1. **Luhn-validate credit card matches** in the first cut — it's cheap and directly targets the
   single highest-volume false-positive source named in the issue itself.
2. **Cap and scope file-content scanning explicitly** (size cap, possibly path-exclusion for
   `testdata`/`fixtures` per open question 2) — do not silently reuse the 4096-byte command cap
   without re-justifying it for file bodies.
3. **Validate/bound `custom_patterns` at config-load time**, not per-request, and reject or warn
   on patterns likely to hit RE2's DFA-cache fallback (long, highly-alternated patterns).
4. **Resolve the AC8 vs. ESCALATE tension explicitly**: decide whether the pending-approval JSON
   store (`ApprovalStore`, not ent/SQLite) is in-scope for redaction/masking, since AC8 as
   written only names "session history / analytics logs" and the real raw-content persistence
   path is the approvals JSON file, not analytics.
5. **Route all new scanner logging through pattern-name-only metadata**, mirroring the existing
   secret-scan log line, and explicitly audit `debug_snapshot.go`'s payload handling for the same
   discipline.
6. **Add a same-PR test pinning the Go/TS `EscalationCategory` literal sets together** — this
   drift risk is proven, not hypothetical, and it fails silently (no badge) rather than loudly.
