# Requirements: PII Scanning as a Built-In Auto-Approval Rule Category

Source: migrated GitHub issue [TylerStaplerAtFanatics/stapler-squad#54](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/54), backlog item `f931a7d9-0296-4bf3-be16-6e06dbfa2cfc`. No interactive stakeholder present for this session — requirements below are derived directly from the issue body plus codebase research; open questions are flagged rather than assumed.

## Problem

Agent sessions frequently touch real PII (test fixtures with user records, seed data with emails, occasionally credentials/API keys). Today the approval pipeline has a secret scanner (`server/services/secret_scanner.go`) that auto-denies a narrow set of credential-shaped regexes on Bash command text only. There is no equivalent for PII patterns (email, SSN, credit card), and file **write content** is never scanned at all — only `command` in the Bash tool input. Undetected PII can be:
- logged permanently into session history (search-indexed, long retention)
- committed into git history via a file write
- exposed in Bash command args shown in the approval UI

## Goal

Add PII detection as a first-class, built-in check in the approval pipeline, modeled directly on the existing secret-scanner precedent, so PII-shaped content escalates (not necessarily auto-denies — see open question) before an agent action is approved.

## Existing System (from codebase research — see `research/codebase.md`)

- **Secret scanner precedent**: `server/services/secret_scanner.go` — a `[]secretPattern{Name, *regexp.Regexp}` list, `ScanForSecrets(text string) SecretScanResult`, capped at first 4096 bytes of `payload.ToolInput["command"]`. Wired into `server/services/approval_handler.go:223-251`, runs *before* rule evaluation, auto-denies, redacts before analytics logging.
- **Escalation taxonomy**: `pkg/classifier/escalation.go` — `EscalationCategory` enum (`no-match`, `explicit-rule`, `domain-age`, `secret-scan`, `unclassifiable`, `unexpected`) and sentinel `RuleID`s (`RuleIDSecretScan` etc.), mirrored by hand in `web-app/src/lib/sessions/escalationCategory.ts` (no codegen bridge — must be updated in both places).
- **Review queue UI**: `web-app/src/components/sessions/ReviewQueuePanel.tsx` has `ESCALATION_REASON_EMOJI` map (`:141-147`). Note: `secret-scan` deliberately has **no** emoji entry because it auto-denies and never reaches the review queue — a PII category that *escalates* (queues for review) needs a real badge entry here, unlike secret-scan.
- **Analytics**: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` already renders `escalation_reason_counts` from `AnalyticsSummaryProto` (`proto/session/v1/types.proto:1137-1139`) — a "PII detections" count is additive to this existing map, no new proto field required for the base metric.
- **Severity levels** (issue #40) already exist: `pkg/classifier.RiskLevel` (`low/medium/high/critical`) flows through `ApprovalRuleProto.risk_level` and the rule builder UI.
- **Escalation reasoning** (issue #45) already exists: `EscalationReasonText` in `pkg/classifier/escalation.go` produces human-readable "why" strings shown in the queue.
- There is **no generic "rule category" enum** in the ent-backed `ApprovalRule` system (`tool_category` is tool-classification — builtin/mcp/etc — not threat-classification). The realistic integration point is the same hardcoded call-site pattern secret-scan uses, not a new user-configurable `ApprovalRule.category` field.

## Proposed Behavior (from issue, reconciled with existing architecture)

1. **New built-in PII pattern set**, same shape as `secretPattern`: email, SSN, credit card (Visa given as example; consider Luhn-validated multi-brand), plus reuse/dedupe with existing API-key patterns in `secret_scanner.go` rather than duplicating them.
2. **Scan scope**: Bash command args (extend the existing call site) **and** file write content where available via the hook (`ToolInput["content"]`/`["new_string"]` for Write/Edit — confirmed NOT currently scanned for secrets either, so this is a scope expansion for both scanners, not PII-only).
3. **Decision**: issue proposes `ESCALATE` (not the secret-scanner's auto-`DENY`) — PII in test fixtures is often legitimate/expected, unlike a live credential. This is the key behavioral divergence from the secret-scan precedent and needs explicit product confirmation (see Open Questions).
4. **UI**: `🔒 PII` badge in `ESCALATION_REASON_EMOJI`, "PII detections" tile in analytics, reuse existing `risk_level`/severity plumbing (default P0/critical per issue) rather than inventing a parallel severity system.
5. **Config**: `pii_scanning.enabled`, `custom_patterns` (user-supplied regex), `on_detection` (escalate/deny) — issue's example JSON. Needs a concrete home: likely `config/` (existing JSON config, see `config/` package) rather than a new file.
6. **Compliance logging**: PII rule violations logged separately for audit — needs to reuse existing analytics/audit logging rather than a new subsystem; scope to be confirmed in planning.

## Acceptance Criteria

1. A new PII pattern scanner exists (new file, e.g. `server/services/pii_scanner.go`), covering at minimum email, SSN, and credit-card-number patterns, following the `secret_scanner.go` structure (name + compiled regex list, size-capped scan).
2. The scanner runs in the approval hook path (`approval_handler.go`) against Bash command text, on the same call-site pattern as the existing secret scan.
3. The scanner also runs against file write/edit content where the hook payload provides it (`ToolInput["content"]`/`["new_string"]` or equivalent) — this is new coverage the secret scanner also currently lacks, so it must be added generically enough that secret-scan can share it if scoped that way in planning.
4. A detected PII match escalates the request (queues for human review with a reason) rather than silently auto-denying, distinct from the secret-scanner's current auto-deny behavior — with an `EscalationCategory` addition (e.g. `pii-scan`) in `pkg/classifier/escalation.go`, mirrored in `web-app/src/lib/sessions/escalationCategory.ts`.
5. The review queue UI (`ReviewQueuePanel.tsx`) shows a distinct `🔒 PII` badge/emoji for PII-flagged items, added to `ESCALATION_REASON_EMOJI`.
6. Approval analytics (`ApprovalAnalyticsPanel.tsx`) surfaces a "PII detections" count, using the existing `escalation_reason_counts` map — no new proto field required for the base count.
7. PII detections default to the highest existing severity level (matching `RiskLevel.Critical` / P0, per issue's stated intent), using the existing severity system rather than a new one.
8. Matched PII text is redacted before it is persisted to session history / analytics logs (mirroring `secret_scanner.go`'s `redactedSecret` sentinel pattern) — raw PII must never be written to durable logs as a side effect of detecting it.
9. `pii_scanning` is configurable: enable/disable, `custom_patterns` (additional user regexes), `on_detection` mode — landing in the existing JSON config mechanism (exact file TBD in planning).
10. Regression: existing secret-scan behavior (auto-deny on credential patterns) is unchanged by this work unless planning explicitly decides to unify the two scanners.

## Explicit Non-Goals (unless planning finds otherwise)

- Building a new generic "rule category" system on the ent-backed `ApprovalRule` — the issue's YAML rule example is illustrative; the actual integration point is the hardcoded scanner call-site pattern, per existing secret-scan precedent.
- Building severity levels or escalation-reasoning infra from scratch — both already exist (issues #40, #45); this feature only needs to *use* them.

## Open Questions (for planning phase / stakeholder)

1. **Escalate vs. deny**: the issue's YAML example says `decision: ESCALATE`, but the "Proposed Behaviour" prose in the issue also says "should be blocked or escalated" — ambiguous. Recommend ESCALATE by default (test fixtures are common false positives) with `on_detection` config allowing `deny` for stricter environments.
2. **False-positive rate**: naive email/SSN/credit-card regexes will flag a large fraction of realistic test fixtures. Needs a scoped decision — e.g. skip files under `testdata/`/`fixtures/` paths, or accept the noise and rely on ESCALATE (human review) rather than DENY to absorb it.
3. **Should secret-scan and PII-scan be unified into one "content scanner" abstraction**, or remain two independent call sites/pattern lists? Issue frames PII as new; codebase already has secret-scan as an almost-identical mechanism — duplicating vs. refactoring is a real planning decision, not a detail.
4. **Credit card validation**: issue's regex is Visa-only and un-Luhn-checked (high false-positive rate on any 16-digit number). Planning should decide whether to add Luhn validation and multi-brand support or ship the simple regex first.
5. **Where does `pii_scanning` config actually live?** — needs confirmation against the existing `config/` package's structure before planning locks the schema.
