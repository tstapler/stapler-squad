# ADR-001: Keep `RiskLevel` Vocabulary As-Is; Represent "Unknown" as Empty String, Not Zero-Value Enum

## Status

Accepted

## Context

`pkg/classifier.RiskLevel` (`pkg/classifier/classifier.go:16-24`) is a 4-value Go `iota` enum
(`RiskLow=0 < RiskMedium=1 < RiskHigh=2 < RiskCritical=3`) already produced by the classifier
on every escalation path. This feature threads it through `PendingApproval` → persistence →
proto → two independent frontends. Two decisions here are non-obvious enough to record:

1. **What vocabulary the UI shows.** The originating GitHub issue asked for a "P0/P1/P2"
   scheme, borrowed from incident-response tooling (PagerDuty, Gastown's Deacon/Mayor
   pattern), where **P0 = most severe** (lower number = more urgent).
2. **How to represent "severity was never computed"** (an approval loaded from disk that
   predates this field, or any future code path that constructs a `PendingApproval` outside
   the classifier). The Go zero value of `RiskLevel` is `RiskLow` (`iota` starts at 0) — if that
   zero value is ever allowed to reach the UI unfiltered, an unclassified-but-potentially-dangerous
   item silently renders as "Low" and sorts to the bottom of a severity-first queue, which is the
   exact failure mode this feature exists to prevent.

## Decision

**1. Keep the plain `RiskLevel` vocabulary (Low/Medium/High/Critical) end-to-end — do not
introduce P0/P1/P2.**

`RiskLevel`'s `iota` direction (higher number = more severe) is the *opposite* numeric
direction of P0/P1/P2 (lower number = more severe). Naively mapping `RiskCritical→P0` also
forces a lossy 4→3 collapse (`RiskLow` has no P-level). The same `RiskLevel` vocabulary is
already wire-compatible on `ApprovalRuleProto.risk_level` and `ClassificationAnalytics`, one
click away in the rules-management UI. Introducing a second vocabulary for the identical
underlying value in a sibling screen would force users to mentally re-map "Critical" in one
UI to "P0" in another — the exact "two priority notions" confusion `requirements.md` already
flags as a non-goal for `queue.Priority` vs. approval risk, except here it would be the *same*
concept wearing two labels. GitHub Dependabot's own 4-tier Critical/High/Moderate/Low
convention (same tier count, near-identical names) is a shipped, widely recognized precedent
for keeping 4 levels rather than collapsing to 3.

**2. `PendingApproval.RiskLevel` is a `string` (not the typed `classifier.RiskLevel` enum),
set once via the existing `riskLevelString()` helper at creation time. Empty string
(`""`) means "not recorded," and is never confused with `"low"`.**

The alternative — keep `RiskLevel` as the typed enum in memory and convert to string only at
the proto/metadata boundary — cannot represent "never computed" without either (a) reordering
the `iota` so a `RiskUnspecified` value is 0 (which would corrupt every already-persisted
`ApprovalRule.risk_level` `field.Int()` row in the ent-backed rules store, an unrelated but
sibling storage surface that persists the same `iota` today) or (b) adding a companion
`RiskLevelKnown bool`/pointer that every future call site must remember to check. Storing the
already-canonicalized string end-to-end removes the bug class instead of mitigating it: an
absent/omitted JSON field or an unset Go string field decodes to `""`, which is trivially
distinguishable from `"low"`, exactly matching how `EscalationReason`/`EscalationCategory`
already behave for pre-existing approvals (`approval_store.go:32-35`'s "Empty for approvals
created before this field existed" comment). No downstream consumer (proto builder,
`ReviewItem.Metadata` enrichment, `PersistedApproval`) re-derives or re-converts the value —
they all copy the string verbatim, so there is exactly one place (`approval_handler.go`'s
`createApproval` label) where `riskLevelString()` is ever called for this field.

**3. UI treats `risk_level == ""` as "Severity not recorded," rendered as a distinct neutral
badge state, and sorted as if High/Critical (fail-safe, near the top) — never silently
defaulted to Low.**

Consistent with this feature's own purpose (surface the one dangerous item in a crowded
queue), under-communicating risk is worse than over-communicating it.

## Consequences

- No change to `classifier.RiskLevel`'s `iota` ordering or values — no migration risk to the
  existing `ApprovalRule.risk_level` `field.Int()` column.
- Every boundary (`PersistedApproval`, `PendingApprovalProto`, `ReviewItem.Metadata["risk_level"]`,
  `PlainApproval.riskLevel`) carries a plain string that is either one of
  `"low"|"medium"|"high"|"critical"` or `""` — no enum type is introduced on the wire, matching
  the existing `ApprovalRuleProto.risk_level`/`SuggestedRuleProto.risk_level` convention.
- Frontend components (`SeverityBadge`, severity sort comparators, severity filter) must all
  treat `""` as a distinct fourth-plus-one state, not fall through a `switch` default that
  happens to land on "Low."
- `classifier.RiskLevel` gains no new methods (`String()`, `IsHigherThan()`) as part of this
  feature — no Go-side call site introduced by this plan needs to sort or compare `RiskLevel`
  values (aggregation is a map increment keyed by string; sorting happens client-side in
  TypeScript). This closes the "possible gap" `research/build-vs-buy.md` flagged without
  adding unused surface area to the type.
