# Architecture Research: PII Scanning Integration

## Judgment: no EventStorming table

This is a straight extension of an existing, working pattern (secret-scan → escalation →
review-queue → analytics), not a new business domain with multiple actors/policies. The
`review-queue-severity/research/architecture.md` precedent (threading `RiskLevel` through the
same chain) already established that this class of feature is "thread a new field through five
existing structs," not new domain modeling — same judgment applies here, doubly so.

## 1. Share a common scanner abstraction, or stay parallel? → Share the pattern, not necessarily one call site

`server/services/secret_scanner.go` is a 73-line file: a `[]secretPattern{Name, *regexp.Regexp}`
slice, `ScanForSecrets(text string) SecretScanResult` (4096-byte cap, first-match-wins), and a
`FormatSecretDenyMessage` formatter. Nothing about it is secret-specific at the mechanism level —
it's generic "run a list of named regexes over a byte-capped string and report the first hit."

Recommendation for planning: **extract the generic mechanism, keep the pattern lists and
call-site decisions separate.**

- A shared low-level helper — e.g. `scanPatterns(text string, patterns []namedPattern, maxBytes int) (name string, found bool)` — used by both `ScanForSecrets` and a new `ScanForPII`. This satisfies AC3's "must be added generically enough that secret-scan can share it if scoped that way" without forcing the two domains into one type.
- Keep `secretPatterns` and `piiPatterns` as **separate lists** (different false-positive tolerance, different lifecycle — PII patterns need `custom_patterns` config per AC9, secrets currently don't). Merging the lists would force one config/behavior model onto two domains that already diverge on the single most important axis: **decision on match** (secret-scan → `AutoDeny`, PII-scan → `Escalate`, per AC4/AC10).
- Keep them as **two call sites** in `approval_handler.go`, not one. They run at different points today for a load-bearing reason: secret-scan runs *first*, before rule evaluation, and returns immediately (`return` at approval_handler.go:249) — it never reaches the classifier or the escalation/queue path at all. PII-scan's target behavior (escalate, not deny) means it must fall through to the `createApproval:` label instead of returning, i.e. it behaves like the **domain-age check** (approval_handler.go:260-293), not like the secret-scan block. Structurally, PII-scan is a sibling of the domain-age branch, not a sibling of the secret-scan branch, even though its pattern-matching internals resemble secret-scan's.

This is why `.claude/rules/interface-pollution-checklist.md`'s smell #4 (forwarding-only
wrapper) and smell #1 (speculative interface) both argue against a single `ContentScanner`
interface/type unifying secret+PII: the two callers need different control flow (return-early
deny vs. fall-through escalate), so a shared interface would immediately need a
decision/branch parameter, defeating the abstraction's value. A shared **function**, not a
shared **interface**, is the right level — concrete types until a second real implementation
forces a genuine seam (checklist item #1's own worked example).

## 2. Where PII-scan sits in the request flow (approval_handler.go)

Confirmed by reading `HandlePermissionRequest` (`server/services/approval_handler.go:199-410`):

1. Parse payload (`:207`)
2. Resolve `sessionID` (`:218`)
3. **Secret scan** (`:223-251`) — Bash `command` only, auto-deny, `return`s immediately (never reaches classifier).
4. `var escalation classifier.ClassificationResult` hoisted here (`:256`) — the escalation-reasoning feature's fix for making escalation-setting branches composable. **PII-scan's escalation branch must set this same `escalation` var**, not return early, exactly like domain-age does.
5. **Domain age check** (`:260-293`) — builds a synthetic `ClassificationResult{Decision: Escalate, RuleID: RuleIDNewDomainCheck, ...}`, records it, sets `escalation`, then `goto createApproval` — **this is the structural template for PII-scan**, not the secret-scan block.
6. AskUserQuestion special-case (`:299-304`)
7. Classifier `Classify()` call and its `switch result.Decision` (`:307-382`)
8. `createApproval:` label (`:384`) — autonomous-LLM branch, then falls through to building the actual `PendingApproval`/queue item using `escalation`.

**Recommended insertion point**: a new block between the secret-scan block (`:251`) and the
domain-age block (`:260`), following the exact shape of the domain-age branch — build a
synthetic `classifier.ClassificationResult{Decision: Escalate, RiskLevel: classifier.RiskCritical, RuleID: classifier.RuleIDPIIScan, RuleName: "PII Detection", Reason: ...}`, call
`h.analyticsStore.RecordFromResult(...)` with a redacted payload (mirroring the secret-scan
block's `sanitizedInput`/`sanitizedPayload` pattern at `:233-239`, not the domain-age block,
which doesn't redact because it has nothing sensitive to redact), set `escalation`, `goto
createApproval`.

Should PII-scan run before or after domain-age? Order doesn't matter functionally (both `goto`
past the classifier once either fires), but running it right after secret-scan keeps the two
"static content pattern scanners" visually adjacent in the function, vs. domain-age which is a
live network-dependent check — a readability grouping, not a correctness requirement.

## 3. File-write content scanning — verified, not assumed

Grepped every `ToolInput[...]` read site in the Go backend
(`server/dependencies.go:212`, `approval_handler.go:225,261,347,359,514,517,520,549,603,606`,
`analytics_store.go:164-165`, `classifier.go:439,710,728`): **only `"command"`, `"file_path"`,
`"description"`, and `"prompt"` keys are ever read.** No code path anywhere in `server/` or
`pkg/classifier/` reads `ToolInput["content"]`, `ToolInput["new_string"]`, or
`ToolInput["old_string"]`. This directly confirms requirements.md's claim (§2, "confirmed NOT
currently scanned for secrets either") — **VERIFIED** by exhaustive grep, not inferred.

Caveat for planning: `PermissionRequestPayload.ToolInput` (`pkg/classifier/classifier.go:57`) is
declared as `map[string]interface{}`, decoded from whatever JSON Claude Code's PreToolUse hook
actually sends — this repo has no test fixture constructing a Write/Edit `tool_input` payload
(grepped `server/services/*_test.go` and `pkg/classifier/*_test.go` for `"content"`/
`"new_string"`/`"old_string"`: zero hits), so the *presence* of `content` (Write) and
`old_string`/`new_string` (Edit) keys in the real hook payload is **INFERRED from Claude Code's
documented hook contract, not verified against a repo-local fixture**. Planning/implementation
should add a test fixture with a real captured Write/Edit `tool_input` payload (or confirm
against Claude Code's hook schema docs) before relying on exact key names.

Given that, the file-content scan block should be:

```go
if content, ok := payload.ToolInput["content"].(string); ok && content != "" {
    // Write tool
}
if newStr, ok := payload.ToolInput["new_string"].(string); ok && newStr != "" {
    // Edit tool — old_string is the pre-image, not attacker/PII-controlled *new* content,
    // so only new_string needs scanning for content being introduced.
}
```

Since AC3 requires this generically enough for secret-scan to reuse, the cleanest shape is a
helper that extracts "the content this tool call is about to write" once
(`extractWriteContent(toolName string, toolInput map[string]interface{}) string`), called by
both the secret-scan block and the new PII-scan block — this is the one place a shared
extraction helper clearly pays for itself (single well-defined transformation, two real
callers today), unlike the "shared interface" idea rejected in §1.

## 4. Escalation taxonomy: consistency requirements between Go and hand-mirrored TS

`pkg/classifier/escalation.go` and `web-app/src/lib/sessions/escalationCategory.ts` are two
independent, hand-synced enumerations — confirmed no codegen bridge exists (the TS file's own
comment says so, and `escalation-reasoning/implementation/plan.md`'s Pattern Decisions table
explicitly rejected a proto `enum` for this taxonomy: "5 fixed, backend-only string keys" don't
justify the `SessionType`-style 7-touchpoint proto-enum machinery documented in
`.claude/rules/session-creation-registry.md`). Adding `pii-scan` requires touching, by hand, in
lockstep:

| File | Change |
|---|---|
| `pkg/classifier/escalation.go` | New `EscalationPII EscalationCategory = "pii-scan"` constant; new `RuleIDPIIScan = "pii-scan"` sentinel; add a `case RuleIDPIIScan: return EscalationPII` arm to `CategorizeEscalationRuleID` (`:60-75`). |
| `web-app/src/lib/sessions/escalationCategory.ts` | Add `"pii-scan"` to the `EscalationCategory` union. |
| `web-app/src/components/sessions/ReviewQueuePanel.tsx` | Add `"pii-scan": "🔒"` to `ESCALATION_REASON_EMOJI` (`:141-147`, currently a `Partial<Record<...>>` — 4 of 6 categories present; adding a 5th entry does not force the other unmapped categories to be filled, since it's `Partial`). |
| `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | Add `"pii-scan": "PII detected in content"` (or similar) to `ESCALATION_CATEGORY_LABELS` (`:98-105`) — **this one is a full `Record<EscalationCategory, string>`, not `Partial`**, so the TypeScript compiler will hard-fail the build if this entry is missed once `"pii-scan"` is added to the union type. This is the one place the hand-sync gap gets *compile-time* enforcement; the emoji map and the Go-side switch statement do not get that safety net — `CategorizeEscalationRuleID`'s `default: return EscalationExplicitRule` (escalation.go:73) means a forgotten `case RuleIDPIIScan` doesn't fail loudly, it silently miscategorizes PII escalations as generic `explicit-rule`. Planning should call this out as a specific regression risk requiring a unit test (`TestCategorizeEscalationRuleID_should_ReturnPII_When_RuleIDIsPIIScan` or similar), not just visual review.

No proto/schema change is required for the taxonomy itself (confirmed by requirements.md §5 and
independently by `AnalyticsSummaryProto.escalation_reason_counts` already being a generic
`map<string, int32>` at `proto/session/v1/types.proto:1107-1134` — a new string key is additive,
no field number consumed).

## 5. Full downstream propagation chain (confirmed identical to the `RiskLevel`/reason precedent)

Both prior features in this area (`escalation-reasoning`, `review-queue-severity`) independently
converged on the same five-hop chain, and PII-scan's `ClassificationResult` (built synthetically
in `approval_handler.go`, same as domain-age) rides the same rails with zero new hops:

```
ClassificationResult (RuleID: RuleIDPIIScan, RiskLevel: RiskCritical, Decision: Escalate)
  → PendingApproval.EscalationReason / .EscalationCategory   (approval_store.go, set once at construction)
  → PersistedApproval (same fields, json:"...,omitempty")
  → session.ApprovalMetadata (review_queue_poller.go DTO)
  → ReviewItem.Metadata["escalation_reason"] / ["escalation_reason_category"]  (proto map<string,string>, no schema change)
  → ReviewQueuePanel.tsx reads queueItem.metadata["escalation_reason_category"] for the emoji lookup
```

`CategorizeEscalationRuleID(classifier.RuleIDPIIScan)` → `EscalationPII` → `"pii-scan"` string
flows through every hop above as a plain string (per the escalation-reasoning plan's explicit
"newtype only inside `pkg/classifier`, plain `string` at every struct boundary" decision,
justified because `ReviewItem.Metadata` is structurally `map[string]string` and cannot hold a
richer type) — no new struct fields needed anywhere in this chain; PII-scan is purely a new
*value* flowing through fields that `RiskLevel`/`EscalationCategory` already added.

## 6. Redaction consistency (AC8)

Secret-scan's redaction pattern (`approval_handler.go:233-239`): shallow-copy `ToolInput`,
replace the sensitive key with the `redactedSecret` sentinel, build a `sanitizedPayload` copy of
the whole struct (not mutating the original, since it's reused for the response), and pass
*that* to `RecordFromResult`. PII-scan must do the same for **both** `command` and (per AC3)
`content`/`new_string` — i.e. redact whichever key(s) actually matched, not blanket-redact every
key, so a PII hit in `content` doesn't also scrub an unrelated, clean `command` field from the
analytics record. This means the redaction step needs to know *which* field(s) the match came
from — a small but real difference from secret-scan's current single-field (`command`-only)
redaction, worth flagging explicitly in the plan phase's task breakdown since it's easy to
under-scope as "just copy the existing redaction block."

## 7. What this research deliberately does not decide

- **Luhn validation / multi-brand credit card patterns** (open question 4) — a patterns-content
  decision, not an architecture decision; the `scanPatterns` helper from §1 is agnostic to how
  sophisticated any individual pattern's validation logic is.
- **`pii_scanning` config schema/location** (open question 5) — the `config/` package's existing
  structure needs a dedicated look in a stack/config research pass, not this architecture pass.
- **`testdata/`/`fixtures/` path exclusion** (open question 2) — a product/false-positive-rate
  decision; architecturally, path-based exclusion would live as a cheap early-return in the new
  PII-scan block (checking `payload.Cwd`/`ToolInput["file_path"]` against a path pattern before
  calling the scanner), which fits the recommended insertion point in §2 without complicating it.
