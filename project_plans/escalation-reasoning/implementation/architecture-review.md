# Architecture Review: escalation-reasoning
**Date**: 2026-08-02
**Verdict**: CONCERNS

Reviewed `implementation/plan.md` (772 lines) and `requirements.md` against the current code
(`server/services/approval_handler.go`, `approval_store.go`, `analytics_store.go`,
`rules_service.go`, `pkg/classifier/classifier.go`, `session/review_queue_poller.go`) and the
Phase 2 `research/build-vs-buy.md` verdict. No code exists yet; all findings are against the plan
text and the real line numbers/signatures it cites, which were spot-checked and found accurate.

## Constitution Violations

N/A — no `docs/adr/ADR-000-architecture-constitution.md` found in this repository (confirmed via
`find docs/adr -iname "*constitution*"`, no match).

## Blockers

None identified. The plan's four-struct threading design (`PendingApproval` →
`PersistedApproval` → `ApprovalMetadata` → `ReviewItem.Metadata`), its rejection of a proto enum,
and its capture-at-decision-time (not recompute-at-render) discipline are all sound and verified
against the actual construction sites (`approval_handler.go:358`, `approval_store.go:372` are the
*only* two `PendingApproval{}` literals in the codebase — Epic 2.2's per-struct stories cover both
completely, no third site is missed).

## Concerns

- [ ] **Task 1.1.1b (`CategorizeEscalationRuleID`)** — the categorization switch hardcodes
  `"new-domain-check"`, `"secret-scan"`, `"shell-expansion-program"` as fresh string literals.
  These sentinel RuleIDs already exist as independent literals at their production sites
  (`approval_handler.go:225,254`, `pkg/classifier/classifier.go:491,542` — confirmed via grep, no
  shared constant exists today). Adding a fourth independent copy in `escalation.go` means a
  future rename of any one emitting site silently degrades that path to the
  `EscalationExplicitRule` default fallback — no compile error, no test failure unless a test
  happens to assert that exact string. **Remediation**: export shared constants once (e.g. in
  `pkg/classifier`: `RuleIDNewDomainCheck`, `RuleIDSecretScan`, `RuleIDShellExpansionProgram`) and
  reference them from both the emitting sites and `CategorizeEscalationRuleID`'s switch, so a
  rename is a single edit. Cheap to add to Task 1.1.1a/b since `escalation.go` is a new file.

- [ ] **Task 2.1.2a (`switch result.Decision` in `HandlePermissionRequest`)** — this is literally
  the switch whose missing `Escalate` case caused the discard bug this feature fixes. After the
  fix it handles all 3 currently-defined `ClassificationDecision` values but still has no
  `default:` arm. `ClassificationDecision` is a plain `int`-backed const block (not a sealed sum
  type), so nothing stops a future 4th value from silently falling through with no `escalation`
  set — the exact failure class this PR is fixing, recurring. **Remediation**: add
  `default: log.Warn("[ApprovalHandler] unrecognized classifier decision", "decision", result.Decision); escalation = result`
  (fail safe toward manual review, not silent auto-allow) while this exact switch is already open
  for edit in this task.

- [ ] **Task 4.2.1b (`ESCALATION_CATEGORY_LABELS` lookup)** — unlike Task 3.1.1a's
  `ESCALATION_REASON_EMOJI`, which explicitly documents a `?? ""` fallback for an unrecognized
  category, the analytics-table task doesn't specify a fallback for a category key not present in
  the 5-entry label map. `Record<string,string>` access on a missing key is `undefined` at
  runtime despite the TS type claiming `string`, so an unmapped category (stale data, or a future
  6th category landing in the Go taxonomy before the frontend map catches up) would render the
  literal text `undefined` in the table. **Remediation**: specify
  `ESCALATION_CATEGORY_LABELS[key] ?? key` in Task 4.2.1b, matching the emoji map's precedent.

- [ ] **`EscalationCategory` boundary (requested evaluation)** — the specific decision to keep
  `EscalationCategory` as a typed newtype only inside `pkg/classifier` and cast to plain `string`
  at the `PendingApproval`/`PersistedApproval`/`ApprovalMetadata` boundary **is sound**: it's
  consistent with the existing sibling-field convention (`RuleID` is already plain `string` on the
  same structs) and the last hop (`ReviewItem.Metadata`) structurally requires `string` anyway.
  The residual gap is cross-language, not cross-struct: the frontend (`ESCALATION_REASON_EMOJI`,
  `ESCALATION_CATEGORY_LABELS`, and the `=== "no-match"` gate in Task 3.2.1a) hardcodes the same 5
  literal strings the Go constants define, with nothing enforcing the two sets stay identical.
  Story 1.1.1d's Go test and Story 3.2.2/4.2.1c's Jest tests each hardcode the same literals as
  their own production code, so a coordinated typo on one side, or a category added to Go without
  a frontend counterpart, wouldn't be caught by either suite. Given the deliberate (and
  reasonable, per Pattern Decisions) choice to avoid a generated enum for 5 fixed values, a full
  codegen fix is disproportionate — but a comment cross-reference in the frontend maps pointing at
  `classifier.EscalationCategory`'s definition site would at least make a future editor of one
  side aware of the other.

## Nitpicks

- `PendingApproval`/`PersistedApproval`/`ApprovalMetadata` carry `EscalationReason` and
  `EscalationCategory` as two independent fields rather than one small paired value type, so
  nothing enforces they're always set together across the 3 structs' construction literals. This
  mirrors the codebase's existing unaddressed `RuleID`/`RuleName` sibling-field convention on
  `ClassificationResult` itself, so it's consistent with precedent rather than a new smell — not
  worth deviating from convention for.
- The `classifier.EscalationCategory` newtype is cast to `string` immediately at Task 2.1.3a's
  call site rather than held as the typed value on `PendingApproval` (an in-memory-only struct,
  never serialized directly) until the `PersistedApproval`/`ApprovalMetadata` boundary where
  `string` is structurally required. The plan's stated rationale (consistency with `RuleID`
  already being plain `string` on the same struct) is a reasonable trade-off — flagged only for
  awareness, not as something to change.
- Task 3.2.2a's Jest coverage exercises only 2 of the 4 reachable `escalation_reason_category`
  values for the Create Rule button gate (`"no-match"` present, `"domain-age"` absent); doesn't
  explicitly assert `"explicit-rule"`/`"unclassifiable"` also hide the button. Logically redundant
  given the `=== "no-match"` gate, but a table-driven test over all 4 non-secret-scan categories
  would be marginally more robust to a future gate-condition rewrite.
