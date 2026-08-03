# Implementation Plan: escalation-reasoning

**Feature**: Surface *why* a tool-use request was escalated to human review (no-match / explicit-rule / domain-age / unclassifiable) on review-queue cards, persist that reason across restarts, and break it down in approval analytics.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: None — see "Why no ADR" below.

### Why no ADR

Every decision in this plan (struct threading vs. side-table vs. lazy recompute; plain-string vs.
proto-enum taxonomy; where the `EscalationCategory` newtype boundary sits) is either (a) a direct
application of a pattern already used elsewhere in this codebase (the `decision_counts` map-string
pattern, the `ApprovalMetadata` enrichment pattern, the `getAttentionReasonInfo` lookup-table
pattern) or (b) resolved with a one-line "reuse the existing X" rationale that doesn't carry
forward risk once implemented. None of it is a technology bet, a licensing/security concern, or a
decision a future reader would need re-litigated with the alternatives spelled out — the Pattern
Decisions table below already carries that record inline. Per `.claude/rules/sdd-planning-artifacts-commit.md`, this plan itself (and the research it's built on) is the durable artifact; a
separate ADR would just duplicate it.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `classifier.ClassificationResult` | Existing struct (`pkg/classifier/classifier.go:39-47`) carrying `Decision`, `RiskLevel`, `Reason`, `Alternative`, `RuleID`, `RuleName`, `Source` for one classify call. | Reused as-is — no new type. This *is* the "escalation result" object; there is no separate `EscalationEvent` type. |
| `classifier.EscalationCategory` | New `type EscalationCategory string` in `pkg/classifier/escalation.go`. One of 6 constants (5 from the original taxonomy + `EscalationUnexpected`, pre-mortem P3 fix). | Newtype, not raw string, *inside* `pkg/classifier` only — see Pattern Decisions for why it becomes a plain `string` at the `PendingApproval`/`ApprovalMetadata`/`ReviewItem.Metadata` boundary. |
| `EscalationNoMatch` | Constant `"no-match"` — `RuleID == ""`, `Decision == Escalate` (classifier fallback, `classifier.go:523-527`). | |
| `EscalationExplicitRule` | Constant `"explicit-rule"` — a named rule's `Decision == Escalate` fired (e.g. `seed-escalate-git-branch-safe-delete`). | Default/fallback bucket of `CategorizeEscalationRuleID` — any non-empty `RuleID` not matching the other 3 sentinels lands here. |
| `EscalationDomainAge` | Constant `"domain-age"` — `RuleID == "new-domain-check"`, synthetic result built in the domain-age branch (`approval_handler.go:237-266`). | |
| `EscalationSecretScan` | Constant `"secret-scan"` — `RuleID == "secret-scan"`, terminal `AutoDeny` (`approval_handler.go:205-233`). Never appears on a `ReviewItem` (no queue item is created); analytics-only bucket. | |
| `EscalationUnclassifiable` | Constant `"unclassifiable"` — `RuleID == "shell-expansion-program"`, the shell-expansion sentinel (`classifier.go:485-493,536-544`). | |
| `CategorizeEscalationRuleID(ruleID string) EscalationCategory` | New pure function in `pkg/classifier/escalation.go` mapping a `RuleID` string to one of the 5 categories above. | Total function — every input string maps to a category; the "else" branch is `EscalationExplicitRule`, not a silent drop (guards against the `ComputeDailyBuckets`-style missing-default bug in pitfalls.md #2). |
| `EscalationReasonText(result ClassificationResult) string` | New pure function in `pkg/classifier/escalation.go`. Returns `result.Reason` verbatim if non-empty; if empty, the fallback is **category-aware** (pre-mortem P1 fix): the static "no rule matched" sentence only for `EscalationNoMatch`, a rule-naming fallback ("Rule '<RuleID>' flagged this for review — no reason text was provided.") for `EscalationExplicitRule`/`EscalationDomainAge`/`EscalationUnclassifiable`, and a distinct internal-error sentence for `EscalationUnexpected`. | The single source of the human-readable string rendered on the review-queue card and stored in `PendingApproval.EscalationReason`. **Pre-mortem P1**: branching only on `Reason == ""` (ignoring category) would make an explicit-rule escalation whose author left `Reason` blank (a real, unvalidated gap — rules can be created via `UpsertApprovalRule`, YAML import, or an accepted `SuggestedRuleCard` suggestion with no required-field check on `Reason`) render "No approval rule matched this request," flatly contradicting the card's own `RuleID`/category and AC1's "not a generic string" promise. |
| `EscalationUnexpected` | 6th category, constant `"unexpected"` — added by the pre-mortem P3 fix for the classifier-switch `default:` arm (Task 2.1.2a). Distinguishes "classifier returned an unrecognized `ClassificationDecision`" (an internal bug) from a genuine `no-match` coverage gap, so it does not silently inherit `no-match`'s copy or Create-Rule-button eligibility. | `RuleIDUnexpectedDecision = "internal-unexpected-decision"` is the synthetic `RuleID` the `default:` arm sets to route here. |
| `escalation` (local var) | New `var escalation classifier.ClassificationResult` hoisted to the top of `HandlePermissionRequest` (`approval_handler.go`), before the domain-age check. Zero-valued unless the domain-age branch or the classifier's `Escalate` case sets it. | This is the "hoist to function scope" fix described in requirements.md's grounded design constraints. Its zero value (`RuleID: "", Reason: ""`) is itself meaningful: `CategorizeEscalationRuleID("")` → `no-match`, and `EscalationReasonText` on an empty `Reason` returns the static fallback — so the case where neither the domain checker nor the classifier is configured degrades to a sane "no rule matched" default rather than an empty string. |
| `PendingApproval.EscalationReason` / `.EscalationCategory` | Two new `string` fields on `session/services`' `PendingApproval` (`approval_store.go:21-39`). Set once at construction (`approval_handler.go:358-369`), never mutated afterward. | Plain `string`, not the classifier newtype — see Pattern Decisions. |
| `PersistedApproval.EscalationReason` / `.EscalationCategory` | Disk-serializable twin of the above (`approval_store.go:42-53`), `json:"escalation_reason,omitempty"` / `json:"escalation_category,omitempty"`. | `omitempty` gives free backward-compat: approvals persisted before this feature deserialize with empty strings, not an error. |
| `session.ApprovalMetadata.EscalationReason` / `.EscalationCategory` | Two new `string` fields on the poller-facing DTO (`review_queue_poller.go:54-61`). | Copied from `PendingApproval` inside `ApprovalStore.GetApprovalMetadataBySession` (`approval_store.go:137-154`). |
| `ReviewItem.Metadata["escalation_reason"]` / `["escalation_reason_category"]` | Two new keys in the existing generic `map<string,string>` `ReviewItem.Metadata` (proto `types.proto:575`, no schema change). Set in the poller enrichment block (`review_queue_poller.go:807-829`), gated like the existing `if a.Cwd != ""` pattern. | This is what the frontend actually reads. **Cross-artifact consistency note**: `requirements.md`'s grounded-design-constraints section illustratively named this key `escalation_category` (matching `PersistedApproval`'s JSON tag one hop earlier); this plan deliberately uses `escalation_reason_category` at this specific hop instead, to disambiguate from `PersistedApproval.EscalationCategory`'s JSON tag when both are in scope during implementation/debugging — not an accidental drift. |
| `AnalyticsEntry` | Existing per-decision analytics record (`analytics_store.go:17-43`) — already carries `Decision` and `RuleID`, which is all `CategorizeEscalationRuleID` needs; no new field added here. | |
| `AnalyticsSummary.EscalationReasonCounts` | New `map[string]int` field on `AnalyticsSummary` (`analytics_store.go:88-114`), keyed by the 5 category strings. Populated by `ComputeSummary` (`analytics_store.go:317-440`). | |
| `AnalyticsSummaryProto.escalation_reason_counts` | New `map<string, int32> escalation_reason_counts = 17;` on `AnalyticsSummaryProto` (`proto/session/v1/types.proto:1107-1134`) — next free field number after 16. | Same `map<string,int32>` idiom as the existing `decision_counts = 2`. |
| Escalation Reasons table | New section in `ApprovalAnalyticsPanel.tsx`, modeled on the existing "Top Triggered Rules" table (`ApprovalAnalyticsPanel.tsx:276-301`). | |
| `ESCALATION_CATEGORY_LABELS` | New `Record<string, string>` (frontend, `ApprovalAnalyticsPanel.tsx`) mapping the 5 category keys to display labels for the analytics table. Distinct from the 4-entry emoji map used in `ReviewQueuePanel.tsx` (which never needs `secret-scan`, since that category never reaches a `ReviewItem`). | |
| itemContext reason paragraph | The `<p className={escalationReasonText} id={`escalation-reason-${queueItem.sessionId}`}>` rendered as the *first* child inside the existing `pending_approval_id` block (`ReviewQueuePanel.tsx:726-743`), before `commandPreview`. `escalationReasonText` is a new vanilla-extract style (`ReviewQueuePanel.css.ts`) that composes `itemContext` (`style([itemContext, {maxHeight, overflowY, wordBreak}])`) rather than reusing the raw `itemContext` class directly. | Satisfies AC6 ("existing `itemContext` class pattern") via composition, not identity — chosen over mutating the shared `itemContext` class because that class is also used at line 719 for the unrelated `queueItem.context` field on non-approval cards; a direct mutation would silently change that unrelated render path too (pre-mortem/consistency-check finding). Does **not** touch the unrelated suppression guard at line 718 (that guard governs `queueItem.context`, a different field). |
| Create Rule gating | The conditional controlling whether the "✦ Create Rule" button (`ReviewQueuePanel.tsx:818-838`) renders at all. Changed from "`tool_input_command` present" to "`tool_input_command` present **and** `escalation_reason_category === "no-match"`". | Resolved decision (was an open question in architecture.md; ux.md settled it) — recorded once here, not deferred further. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Escalation-reason storage & propagation (top-level architecture) | Plumb through existing structs: `ClassificationResult` → `PendingApproval` → `PersistedApproval` → `ApprovalMetadata` → `ReviewItem.Metadata` | This codebase's own `ApprovalMetadata` enrichment precedent (`review_queue_poller.go:807-829`) | (b) Separate `EscalationLog` side-table keyed by approval ID | features.md #3 confirms the reason's lifetime is 1:1 with `PendingApproval`'s own lifetime (deleted on `Resolve`, no post-resolution audit trail required by AC2) — a second persisted table would just re-implement `PendingApproval`'s own create/resolve/orphan-reload lifecycle for no benefit, and doubles the disk-persistence surface this PR has to get right. |
| Escalation-reason storage & propagation (top-level architecture) | (as above) | (as above) | (c) Recompute the reason lazily by re-running the classifier/domain-checker at render or poll time | Not idempotent for domain-age (`IsNewlyRegistered` is a live network call — re-running it every ~2s poll cycle is expensive and can flip between polls as the domain ages past the threshold), and violates the EventStorming finding that the reason belongs on the `ApprovalCreated` event, fixed once at creation, never re-derived from mutable external state. |
| `EscalationCategory` representation | Newtype (`type EscalationCategory string`) *inside* `pkg/classifier` only, converted to plain `string` at the `PendingApproval`/`PersistedApproval`/`ApprovalMetadata` boundary via `string(...)` | type-driven-design | Propagate the newtype across all 4 downstream structs | Two of those 4 structs already use plain `string` for every sibling field (`RuleID`, `ToolName`, `Cwd`, …), and the last one in the chain, `ReviewItem.Metadata`, is a `map[string]string` — it structurally cannot hold a non-string type. Consistency with the existing convention (not just the hard `map[string]string` constraint) makes plain `string` correct for all 3 downstream structs, not only the last one. |
| Taxonomy encoding (5 categories) | Plain Go string constants + a categorization function, no proto `enum` | stack.md | Proto `enum SessionType`-style enum for `EscalationCategory` | This codebase's one existing proto enum (`SessionType`) is reserved for a heavier UI-driven dropdown pattern (`.claude/rules/session-creation-registry.md`, 7 touchpoints) — overkill and inconsistent for 5 fixed, backend-only string keys that already match the existing `decision_counts`/`RuleStatProto.rule_id` string-keyed convention. |
| Analytics aggregation (AC4) | Extend the existing `ComputeSummary` pure aggregation function (`analytics_store.go:317-440`) with one more accumulator map, following the exact shape of the existing coverage-gap branch (`analytics_store.go:395-406`) | PoEAA — Transaction Script (single-pass aggregation over a flat entry list, no cross-aggregate rules) | A separate `EscalationAnalyticsService`/`Computer` type | The complexity here is "accumulate counts into a map, keyed by a 5-value category" — no persistence, no cross-cutting business rule, no reuse outside this one summary. A Service Layer object would fragment "where analytics summary computation lives" into two places for one map. |
| `EscalationCategory` → emoji/label (frontend) | Static lookup table/map (`ESCALATION_CATEGORY_LABELS` for analytics; a 4-entry emoji map for `ReviewQueuePanel.tsx`), matching the existing `getAttentionReasonInfo` idiom in `StatusBadge.tsx:15-26` | GoF — none needed | GoF Strategy pattern (one renderer object per category) | 4-5 fixed categories with no per-category *behavior* beyond a label/emoji string is not a "recurring problem a pattern would solve" — a Strategy hierarchy here is the interface-pollution-checklist smell #1 (speculative abstraction) applied to components instead of Go interfaces. |
| Create Rule button gating (AC3/AC7) | Inline conditional expression extending the existing `queueItem.metadata?.["…"]` idiom already used at `ReviewQueuePanel.tsx:718,728,733,739` | Existing codebase idiom | Extract a `<CreateRuleButton>` sub-component | Single call site, no reuse elsewhere in the tree — extracting now would be a speculative component split with no second consumer, the same anti-pattern `.claude/rules/interface-pollution-checklist.md` calls out for Go types. |
| `PendingApproval` field mutation discipline | Set-once at construction (`createApproval:` convergence point, `approval_handler.go:358-369`), never mutated after `store.Create` | EventStorming (research/architecture.md) — reason belongs on the `ApprovalCreated` event | Allow later code paths (e.g. a future retry) to overwrite `EscalationReason` | `PendingApproval` has no existing precedent for post-construction field mutation outside `Resolve` (which deletes the record); adding one here for a single new field would be the first exception, with no requirement (AC1-AC8) asking for it. |

---

## Migration Plan

Omit — no schema or data migration. Two independent forms of "additive-only" change, both already
verified free of migration steps:
- **Disk JSON** (`pending_approvals.json`): new `PersistedApproval` fields use `omitempty` tags;
  `encoding/json` unmarshals records written before this feature with the two new Go fields at
  their zero value (`""`), which `CategorizeEscalationRuleID`/rendering code must treat as "reason
  not recorded" (see AC1/AC6 fallback copy in Story 3.1.1) — not an error state.
- **Proto**: `escalation_reason_counts = 17` is a new field on `AnalyticsSummaryProto`, appended
  after the highest existing field number (16). Old clients ignore an unknown field; new clients
  reading a summary from before this feature see an empty map. Run `make proto-gen` (never
  hand-invoke `buf`/`protoc` — see `.claude/docs/*` conventions) then `go build ./...` and a
  frontend typecheck to catch any generation staleness before committing (pitfalls.md #4).

## Observability Plan

- **Logs**: no new log call sites. Two existing structured log lines already fire on the code
  paths this feature touches and gain the new fields as extra structured args where cheap:
  - `log.ForSession(sessionID).Info("[ApprovalHandler] escalating — newly-registered domain", …)`
    (`approval_handler.go:249`) — add `"escalation_category", "domain-age"` as a literal extra arg
    (the category is a compile-time constant on this exact code path, no computation needed).
  - `log.Debug("enriched approval item with hook metadata", …)` (`review_queue_poller.go:828`) —
    add `"escalation_category", item.Metadata["escalation_reason_category"]`.
  No new log lines beyond these two additions — this is a pure metadata-enrichment feature, not a
  new failure-prone code path.
- **Metrics**: not required. Every change in this plan is an in-memory struct-field set, a map
  accumulator, or a string lookup — none crosses the "new operation >100ms" bar the template asks
  about. The domain-age network call (`IsNewlyRegistered`) already exists and is unchanged.
- **Alerts**: no new alerts required. No new external dependency, no new failure mode beyond what
  `classifier.Classify` and `DomainAgeChecker.IsNewlyRegistered` already have.

## Risk Control

- **Feature flag**: not gated. This is additive metadata enrichment plus a UI text/analytics
  addition — it does not change any allow/deny/escalate decision (AC5's explicit "no regressions"
  requirement rules out gating the decision path, and there is no decision-path change to gate).
- **Rollback procedure**: standard revert via PR close + revert commit. Both persistence forms
  (disk JSON, proto) are additive-only (see Migration Plan), so a revert needs no forward-only data
  cleanup in either direction.
- **Staged rollout**: full rollout on merge — this is an internal engineering tool with a single
  deployed instance per user (see `CLAUDE.md`'s state-isolation docs), no user cohort to stage
  across.

## Unresolved Questions

No open questions block the *start* of implementation — all listed below have a plan-level answer.
One decision needs explicit human sign-off before merge, flagged per the adversarial review:

- [x] Create Rule button gating (`research/architecture.md`'s open question: no-match-only vs. keep
  current `tool_input_command`-only gating) — resolved in this plan: gated to
  `escalation_reason_category === "no-match"` only (see Pattern Decisions and Story 3.2.1), per the
  UX research rationale (explicit-rule and domain-age escalations have no "prevent this next time"
  story a pattern rule can express). **Adversarial review flag**: this backlog item went through
  the no-interactive-ideation SDD pipeline (no user available to confirm intent), and the practical
  effect is real — the button currently shows for *any* escalation with `tool_input_command`
  (explicit-rule/domain-age/unclassifiable included); after this change it's hidden for 3 of 4
  categories. The reasoning is sound and research-backed, not a guess, but the PR description
  (Phase 6/7 completion) must call this out explicitly as a behavior change so a human reviewer can
  veto before merge if the original filer's intent differs — do not let this land silently as an
  implementation detail. **Pre-mortem P2 addendum**: specifically for `domain-age`, this also
  removes the button's only current use as a one-click path to allowlist a trusted-but-newly-
  registered domain (`GenerateSuggestedRule` already supports `auto_allow`-decision suggestions,
  not just deny/escalate patterns, per `rules_service.go`'s `validateSuggestion`) — today the button
  is gated on `tool_input_command` presence only, so this is a real, if narrow, capability loss for
  domain-age false positives, not purely a UX-emphasis question. The PR description must name this
  specific trade-off (not just the general gating change) so a reviewer can judge whether losing
  that one-click allow-listing path for domain-age is acceptable, rather than discovering it later
  as a support request ("how do I stop this domain from re-escalating every time?").

## Dependency Visualization

```
Phase 1: Classifier Taxonomy (foundation, no deps)
  Epic 1.1 (escalation.go: type + consts + Categorize + ReasonText)
        │
        ▼
Phase 2: Backend Plumbing — Capture at Source
  Epic 2.1 (hoist + populate `escalation` var in HandlePermissionRequest)
        │
        ▼
  Epic 2.2 (struct threading: PendingApproval → PersistedApproval →
            persistToDiskLocked → loadFromDisk → ApprovalMetadata →
            GetApprovalMetadataBySession → ReviewItem.Metadata)
        │
        ├─────────────────────────────┬───────────────────────────┐
        ▼                              ▼                           ▼
Phase 3: Frontend Display      Phase 4: Analytics (AC4)     Phase 5: Backend Tests (AC2/AC5)
  Epic 3.1 (reason <p>,          Epic 4.1 (proto field +      Epic 5.1 (approval_handler_test.go
    AC1/AC6)                       ComputeSummary +             + approval_service_test.go new
        │                          summaryToProto)               cases, orphaned-reload regression)
        ▼                              │
  Epic 3.2 (Create Rule gating          ▼
    + intent, AC3/AC7) ◄── needs   Epic 4.2 (ApprovalAnalyticsPanel
    escalation_reason_category      table + jest test, AC4)
    from Epic 3.1's plumbing
    (same metadata read)
        │                              │
        └──────────────┬───────────────┘
                        ▼
              Phase 6: E2E (AC8)
                Epic 6.1 (escalation-reasoning.spec.ts —
                  needs full backend chain + Epic 3.1/3.2 live)
                        │
                        ▼
              Phase 7: Registry & Docs
                Epic 7.1 (bump lastModified on 2 existing
                  backend entries, add 1 new frontend entry,
                  make registry-generate)
```

Epic 1.1 and Epic 4.1's proto step can start in parallel with each other (independent files), but
Epic 4.1's `ComputeSummary` step (4.1.2) depends on Epic 1.1's `CategorizeEscalationRuleID` being
importable from `server/services`. Phase 5 (Go tests) and Phase 3/4 (frontend) can proceed in
parallel once Epic 2.2 lands — they touch disjoint files.

---

## Implementation Note (pre-mortem P2)

This plan's 41 tasks cite specific line numbers in `approval_handler.go`, `approval_store.go`,
`review_queue_poller.go`, `ReviewQueuePanel.tsx`, and `ApprovalAnalyticsPanel.tsx` — files with
demonstrated high churn (a prior PR rewrote 927 lines of `ReviewQueuePanel.tsx` in one pass) and,
as of planning time, 10+ sibling worktrees in this same workspace hold in-flight diffs against
these exact files. **Before editing, re-locate each cited construct by name/signature (not blindly
by line number) against the current state of this branch** — the line numbers are planning-time
anchors for review clarity, not a substitute for reading the surrounding code at implementation
time. If a cited line has drifted, prefer the nearest matching construct over line-number literalism.

## Phase 1: Classifier Taxonomy

### Epic 1.1: Escalation Category Taxonomy
**Goal**: Give the codebase a single, tested source of truth for "which of the 5 categories does
this `RuleID` belong to" and "what's the human-readable sentence for this result" — everything
downstream (handler, persistence, analytics, frontend) consumes these two functions instead of
reimplementing the mapping.

#### Story 1.1.1: `EscalationCategory` type, constants, and categorization function
**As a** backend developer wiring escalation reasons into the handler and analytics code, **I
want** a single typed categorization function, **so that** the no-match/explicit-rule/domain-age/
secret-scan/unclassifiable taxonomy is defined exactly once and every consumer (handler,
`ComputeSummary`, tests) agrees on it.

**Acceptance Criteria** (supports AC1, AC4):
- `CategorizeEscalationRuleID` returns the correct category for all 5 known `RuleID` shapes plus
  the "unknown non-empty RuleID" fallback.
  - *Given* `ruleID = ""`, *When* `CategorizeEscalationRuleID(ruleID)` is called, *Then* it returns
    `EscalationNoMatch` (`"no-match"`).
  - *Given* `ruleID = "seed-escalate-git-branch-safe-delete"`, *When* called, *Then* it returns
    `EscalationExplicitRule` (`"explicit-rule"`).
  - *Given* `ruleID = "new-domain-check"`, *When* called, *Then* it returns `EscalationDomainAge`
    (`"domain-age"`).
  - *Given* `ruleID = "secret-scan"`, *When* called, *Then* it returns `EscalationSecretScan`
    (`"secret-scan"`).
  - *Given* `ruleID = "shell-expansion-program"`, *When* called, *Then* it returns
    `EscalationUnclassifiable` (`"unclassifiable"`).
  - *Given* `ruleID = "internal-unexpected-decision"` (`RuleIDUnexpectedDecision`), *When* called,
    *Then* it returns `EscalationUnexpected` (`"unexpected"`) — pre-mortem P3 fix, distinguishes an
    internal classifier bug from a genuine no-match coverage gap.
  - *Given* `ruleID = "some-future-rule-id-nobody-has-seen"`, *When* called, *Then* it returns
    `EscalationExplicitRule` (the default/fallback case — never a silent no-op).
- `EscalationReasonText` returns the classifier's own `Reason` when present, and a **category-aware**
  fallback when `Reason == ""` (pre-mortem P1 fix — not a single static fallback for every category).
  - *Given* `result = ClassificationResult{Decision: Escalate, RuleID: "", Reason: "No matching
    rule; escalated for manual review."}` (the actual `classifySingle` fallback value,
    `classifier.go:526`), *When* `EscalationReasonText(result)` is called, *Then* it returns
    `"No matching rule; escalated for manual review."` verbatim (no re-wrapping).
  - *Given* `result = ClassificationResult{}` (zero value — the case where neither the domain
    checker nor the classifier is configured; `RuleID == ""` categorizes as no-match), *When*
    called, *Then* it returns the static fallback
    `"No approval rule matched this request — escalated to manual review by default."`.
  - *Given* `result = ClassificationResult{Decision: Escalate, RuleID: "custom-rule", Reason: ""}`
    (an explicit-rule match whose author left `Reason` blank — a real, unvalidated gap: no rule
    creation path in this codebase requires `Reason` to be non-empty), *When* called, *Then* it
    returns `Rule "custom-rule" flagged this for review — no reason text was provided.` and
    **does not** contain the substring `"No approval rule matched"` — this is the exact case
    pre-mortem P1 flagged: a blank-`Reason` explicit-rule escalation must never render text that
    contradicts its own category.
  - *Given* `result = ClassificationResult{RuleID: "internal-unexpected-decision", Reason: ""}`,
    *When* called, *Then* it returns `"An internal classification error occurred — review
    manually."`.

**Files**: `pkg/classifier/escalation.go` (new), `pkg/classifier/escalation_test.go` (new)

##### Task 1.1.1a: Create `EscalationCategory` type + 6 constants + shared sentinel RuleID constants (~4 min)
- New file `pkg/classifier/escalation.go`. Package `classifier`.
- `type EscalationCategory string`
- `const (EscalationNoMatch EscalationCategory = "no-match"; EscalationExplicitRule EscalationCategory = "explicit-rule"; EscalationDomainAge EscalationCategory = "domain-age"; EscalationSecretScan EscalationCategory = "secret-scan"; EscalationUnclassifiable EscalationCategory = "unclassifiable"; EscalationUnexpected EscalationCategory = "unexpected")`
  — the 6th constant, `EscalationUnexpected`, is the pre-mortem P3 fix (see Task 2.1.2a).
- **Architecture review concern (Task 1.1.1b)**: also declare shared sentinel `RuleID` constants here —
  `const (RuleIDNewDomainCheck = "new-domain-check"; RuleIDSecretScan = "secret-scan"; RuleIDShellExpansionProgram = "shell-expansion-program"; RuleIDUnexpectedDecision = "internal-unexpected-decision")`
  (the 4th, `RuleIDUnexpectedDecision`, is synthetic — never emitted by the classifier itself, only set by
  `HandlePermissionRequest`'s new `default:` arm) —
  and update the 3 existing emitting sites to reference the first 3 instead of inline string literals:
  `approval_handler.go:225` (secret-scan `RuleID:` literal), `approval_handler.go:254` (domain-age
  `RuleID:` literal), `pkg/classifier/classifier.go:491,542` (shell-expansion `RuleID:` literals).
  This closes the "4th independent copy of the same literal" gap the review flagged — a future
  rename becomes a single edit instead of a silent categorization drift.
- Files: `pkg/classifier/escalation.go`, `server/services/approval_handler.go`, `pkg/classifier/classifier.go`

##### Task 1.1.1b: Implement `CategorizeEscalationRuleID` (~3 min)
- In `pkg/classifier/escalation.go`: `func CategorizeEscalationRuleID(ruleID string) EscalationCategory`
- Body: `switch ruleID { case "": return EscalationNoMatch; case RuleIDNewDomainCheck: return EscalationDomainAge; case RuleIDSecretScan: return EscalationSecretScan; case RuleIDShellExpansionProgram: return EscalationUnclassifiable; case RuleIDUnexpectedDecision: return EscalationUnexpected; default: return EscalationExplicitRule }`
- Files: `pkg/classifier/escalation.go`

##### Task 1.1.1c: Implement `EscalationReasonText`, category-aware (~4 min)
- **Pre-mortem P1 fix**: branch on category, not solely on `Reason == ""` — an empty `Reason` on a
  real rule (`explicit-rule`/`domain-age`/`unclassifiable`) must never render the no-match sentence.
- In `pkg/classifier/escalation.go`:
  ```go
  func EscalationReasonText(result ClassificationResult) string {
      if result.Reason != "" {
          return result.Reason
      }
      switch CategorizeEscalationRuleID(result.RuleID) {
      case EscalationNoMatch:
          return "No approval rule matched this request — escalated to manual review by default."
      case EscalationUnexpected:
          return "An internal classification error occurred — review manually."
      default:
          // explicit-rule / domain-age / unclassifiable with a blank Reason: name the rule
          // rather than falsely claiming no rule matched.
          return fmt.Sprintf("Rule %q flagged this for review — no reason text was provided.", result.RuleID)
      }
  }
  ```
- Files: `pkg/classifier/escalation.go`

##### Task 1.1.1d: Unit tests for both functions (~7 min)
- New file `pkg/classifier/escalation_test.go`. Table-driven test `TestCategorizeEscalationRuleID` covering all 7 cases from the AC above (6 known including `EscalationUnexpected` + 1 fallback).
- Second test `TestEscalationReasonText` covering: non-empty `Reason` passthrough; zero-value/no-match fallback; **pre-mortem P1 case** — `ClassificationResult{RuleID: "custom-rule", Reason: ""}` → assert the result is `Rule "custom-rule" flagged this for review — no reason text was provided.` and explicitly assert it does **not** contain the substring `"No approval rule matched"`; and an `EscalationUnexpected`-category case (`RuleID: RuleIDUnexpectedDecision, Reason: ""`) → assert the internal-error sentence.
- Files: `pkg/classifier/escalation_test.go`

---

## Phase 2: Backend Plumbing — Capture at Source (AC1, AC2)

### Epic 2.1: Hoist and populate the escalation result in `HandlePermissionRequest`
**Goal**: Make the classifier's escalation result (or the domain-age synthetic equivalent) survive
past its current block scope so it reaches the `createApproval:` label regardless of which of the
two escalation paths (domain-age `goto`, or classifier `Escalate` fallthrough) triggered it.

#### Story 2.1.1: Hoist `escalation` var and populate the domain-age branch
**As a** developer fixing the `_ = reason` discard bug, **I want** the domain-age escalation reason
captured in a function-scoped variable instead of thrown away, **so that** it reaches
`PendingApproval` construction.

**Acceptance Criteria** (AC1 domain-age case):
- *Given* a Bash command referencing domain `"sketchy-newdomain.xyz"`, `DomainAgeChecker.IsNewlyRegistered` returning `true`, and `NewDomainThreshold()` of 30 days, *When* `HandlePermissionRequest` evaluates the domain-age branch and jumps to `createApproval`, *Then* the function-scoped `escalation` variable equals `classifier.ClassificationResult{Decision: classifier.Escalate, RiskLevel: classifier.RiskHigh, RuleID: "new-domain-check", RuleName: "New Domain Check", Reason: "Domain \"sketchy-newdomain.xyz\" was registered within the last 30 days — possible phishing or supply-chain risk."}`.

**Files**: `server/services/approval_handler.go`

##### Task 2.1.1a: Hoist `var escalation classifier.ClassificationResult` (~2 min)
- In `approval_handler.go`, immediately before the domain age check block (before line 237's `if h.domainChecker != nil {`), add: `// escalation captures the classification result (or its domain-age synthetic equivalent) that led to this request being queued for manual review. Zero-valued (no-match) unless set below.` followed by `var escalation classifier.ClassificationResult`.
- Files: `server/services/approval_handler.go`

##### Task 2.1.1b: Populate `escalation` in the domain-age branch, remove the discard (~3 min)
- Replace the existing sequence at `approval_handler.go:250-262`:
  ```go
  if h.analyticsStore != nil {
      h.analyticsStore.RecordFromResult(payload, classifier.ClassificationResult{
          Decision:  classifier.Escalate,
          RiskLevel: classifier.RiskHigh,
          RuleID:    "new-domain-check",
          RuleName:  "New Domain Check",
          Reason:    reason,
      }, sessionID, "", 0)
  }
  // Fall through to manual review queue (do NOT return here).
  // The domain reason will appear in the pending approval context.
  _ = reason // will be surfaced when the approval is shown in review queue
  goto createApproval
  ```
  with a version that builds the `ClassificationResult` once and reuses it for both `RecordFromResult` and `escalation`:
  ```go
  domainEscalation := classifier.ClassificationResult{
      Decision:  classifier.Escalate,
      RiskLevel: classifier.RiskHigh,
      RuleID:    "new-domain-check",
      RuleName:  "New Domain Check",
      Reason:    reason,
  }
  if h.analyticsStore != nil {
      h.analyticsStore.RecordFromResult(payload, domainEscalation, sessionID, "", 0)
  }
  // Fall through to manual review queue (do NOT return here).
  escalation = domainEscalation
  goto createApproval
  ```
- Adversarial review concern: the pre-existing `continue`-past-error behavior 3 lines above this
  block (`approval_handler.go:241-245`, when `IsNewlyRegistered` errors on a domain) is left
  unchanged by this task, but its consequence is new once the reason becomes screen-visible — if
  that was the only domain in the command, the reviewer sees a plain no-match/explicit-rule reason
  with no indication a domain check was attempted and came back inconclusive. Add a one-line comment
  at that `continue` noting the interaction, so a future reader doesn't have to re-derive it. No new
  test required (low blast radius, pre-existing behavior).
- Files: `server/services/approval_handler.go`

#### Story 2.1.2: Add explicit `Escalate` case to the classifier switch
**As a** developer, **I want** the classifier's `switch result.Decision` to explicitly capture the
`Escalate` case, **so that** explicit-rule and no-match escalations also populate `escalation`
(currently the switch has no `case classifier.Escalate:` at all — it silently falls through).

**Acceptance Criteria** (AC1 no-match and explicit-rule cases):
- *Given* `classifySingle` returns `ClassificationResult{Decision: Escalate, RiskLevel: RiskMedium, Reason: "No matching rule; escalated for manual review."}` (RuleID `""`), *When* `HandlePermissionRequest`'s classifier switch runs, *Then* `escalation` equals that exact result.
- *Given* a command `"git branch -d feature/foo"` matching seed rule `seed-escalate-git-branch-safe-delete` (`Decision: Escalate, RuleID: "seed-escalate-git-branch-safe-delete", Reason: "Branch deletion modifies repository structure and should be reviewed."`), *When* the switch runs, *Then* `escalation` equals that result.

**Files**: `server/services/approval_handler.go`

##### Task 2.1.2a: Add `case classifier.Escalate: escalation = result` + a `default` arm (~3 min)
- In `approval_handler.go`, inside the `switch result.Decision {` block (currently `case classifier.AutoAllow:` / `case classifier.AutoDeny:` at lines 291/299, then a comment `// Escalate: fall through to manual review queue` at line 311), replace the trailing comment-only fallthrough with an explicit case, plus a `default` arm:
  ```go
  case classifier.Escalate:
      escalation = result
      // Fall through to manual review queue (createApproval label below).
  default:
      // Unrecognized classifier.ClassificationDecision (e.g. a future 4th value). Fail safe
      // toward manual review rather than silently falling through with escalation unset —
      // this switch's missing-case behavior is exactly the bug this feature fixes; guard
      // against it recurring for any future decision value.
      log.Warn("[ApprovalHandler] unrecognized classifier decision, escalating for manual review", "decision", result.Decision)
      escalation = result
      // Pre-mortem P3: route through the synthetic RuleIDUnexpectedDecision sentinel so
      // CategorizeEscalationRuleID buckets this as EscalationUnexpected, not EscalationNoMatch
      // (result.RuleID is almost certainly "" here, since no rule lookup occurred) — an internal
      // classifier bug must not silently render normal "no rule matched" copy or offer the
      // Create Rule CTA as if this were a real coverage gap.
      escalation.RuleID = classifier.RuleIDUnexpectedDecision
  }
  ```
- Architecture review concern: `ClassificationDecision` is a plain `int`-backed const block, not a
  sealed sum type — this `default` arm is the guard against a future 4th value silently bypassing
  `escalation` the same way `Escalate` did before this fix.
- Files: `server/services/approval_handler.go`

#### Story 2.1.3: Set `EscalationReason`/`EscalationCategory` at `PendingApproval` construction
**As a** developer, **I want** the two new fields populated exactly once at the `createApproval:`
convergence point, **so that** every code path that reaches `createApproval` (domain-age `goto`,
classifier `Escalate` fallthrough, or neither) gets a consistent, non-empty reason.

**Acceptance Criteria** (AC1, all 3 paths converge here):
- *Given* `escalation` is the domain-age result from Story 2.1.1's example, *When* the `&PendingApproval{...}` literal at `approval_handler.go:358-369` is constructed, *Then* `approval.EscalationReason == "Domain \"sketchy-newdomain.xyz\" was registered within the last 30 days — possible phishing or supply-chain risk."` and `approval.EscalationCategory == "domain-age"`.

**Files**: `server/services/approval_handler.go`

##### Task 2.1.3a: Add the two fields to the `PendingApproval{}` literal, with a length cap (~3 min)
- In `approval_handler.go`, in the `approval := &PendingApproval{...}` literal (lines 358-369), add two fields:
  ```go
  EscalationReason:   truncateEscalationReason(classifier.EscalationReasonText(escalation)),
  EscalationCategory: string(classifier.CategorizeEscalationRuleID(escalation.RuleID)),
  ```
- Pre-mortem P2 concern: an explicit-rule's `Reason` is free text a rule author can set to any
  length, and `persistToDiskLocked` re-marshals and writes **all** pending approvals to disk on
  every single `Create`/`Resolve` while holding the write lock — an unbounded string here scales
  that cost with rule-author verbosity, not just entry count. Add a small helper in
  `approval_handler.go` (or `escalation.go`):
  ```go
  const maxEscalationReasonLen = 500

  func truncateEscalationReason(s string) string {
      if len(s) <= maxEscalationReasonLen {
          return s
      }
      return s[:maxEscalationReasonLen] + "…"
  }
  ```
- Files: `server/services/approval_handler.go`

### Epic 2.2: Struct threading for persistence (AC2)
**Goal**: Close the 4-struct gap identified in features.md #4 — missing any one of
`PendingApproval` → `PersistedApproval` (persist) → `PersistedApproval` (load) →
`ApprovalMetadata`/`GetApprovalMetadataBySession` silently drops the reason for orphaned
(post-restart) approvals specifically, which is exactly the scenario AC2 tests. Each struct gets
its own task so none is "assumed."

#### Story 2.2.1: Add fields to `PendingApproval` and `PersistedApproval`
**As a** developer, **I want** the two new fields declared on both the live and disk-serializable
approval structs, **so that** there is somewhere for Story 2.1.3's values to live and be persisted.

**Acceptance Criteria** (AC2, structural precondition):
- *Given* the updated `PendingApproval` struct, *When* a value is constructed with `EscalationReason: "x", EscalationCategory: "explicit-rule"`, *Then* it compiles and those fields are readable.
- *Given* the updated `PersistedApproval` struct, *When* `json.Marshal` is called on a value with `EscalationReason: "x"`, *Then* the output JSON contains `"escalation_reason":"x"`; *Given* `EscalationReason: ""`, *When* marshaled, *Then* the key is omitted entirely (`omitempty`).

**Files**: `server/services/approval_store.go`

##### Task 2.2.1a: Add fields to `PendingApproval` struct (~2 min)
- In `approval_store.go`, in the `PendingApproval` struct (lines 21-39), add after `PermissionMode string`:
  ```go
  // EscalationReason and EscalationCategory capture why this request was escalated
  // for manual review, set once at creation from classifier.EscalationReasonText /
  // classifier.CategorizeEscalationRuleID. Empty for approvals created before this
  // field existed (loaded from disk) — never re-derived after creation.
  EscalationReason   string
  EscalationCategory string
  ```
- Files: `server/services/approval_store.go`

##### Task 2.2.1b: Add fields + `omitempty` json tags to `PersistedApproval` struct (~2 min)
- In `approval_store.go`, in the `PersistedApproval` struct (lines 42-53), add after `PermissionMode string \`json:"permission_mode"\``:
  ```go
  EscalationReason   string `json:"escalation_reason,omitempty"`
  EscalationCategory string `json:"escalation_category,omitempty"`
  ```
- Files: `server/services/approval_store.go`

#### Story 2.2.2: `persistToDiskLocked` copy loop (pitfalls.md #1 explicit guard)
**As a** developer, **I want** the two new fields explicitly copied in `persistToDiskLocked`'s
field-by-field construction, **so that** they don't silently vanish on every disk write (this is
the exact bug shape pitfalls.md #1 warns about — the copy is manual, not reflective, so a missed
field compiles fine and just drops data).

**Acceptance Criteria** (AC2):
- *Given* an in-memory `PendingApproval{EscalationReason: "Branch deletion modifies repository structure and should be reviewed.", EscalationCategory: "explicit-rule", ...}` in `ApprovalStore.pending`, *When* `persistToDiskLocked()` runs, *Then* the JSON written to `pending_approvals.json` contains `"escalation_reason":"Branch deletion modifies repository structure and should be reviewed.","escalation_category":"explicit-rule"` for that entry.

**Files**: `server/services/approval_store.go`

##### Task 2.2.2a: Add both fields to the `PersistedApproval{}` literal in `persistToDiskLocked` (~2 min)
- In `approval_store.go`, in `persistToDiskLocked` (lines 296-310), add to the `PersistedApproval{...}` literal after `PermissionMode: a.PermissionMode,`:
  ```go
  EscalationReason:   a.EscalationReason,
  EscalationCategory: a.EscalationCategory,
  ```
- Files: `server/services/approval_store.go`

#### Story 2.2.3: `loadFromDisk` reconstruction
**As a** developer, **I want** the two fields copied back out of `PersistedApproval` when
reconstructing `PendingApproval` on load, **so that** orphaned (post-restart) approvals keep their
escalation reason — this is the exact scenario AC2 requires.

**Acceptance Criteria** (AC2):
- *Given* `pending_approvals.json` contains an entry with `"escalation_reason":"No matching rule; escalated for manual review.","escalation_category":"no-match"`, *When* `NewApprovalStore(filePath)` runs `loadFromDisk()`, *Then* the resulting `PendingApproval` in `s.pending` has `EscalationReason == "No matching rule; escalated for manual review."`, `EscalationCategory == "no-match"`, and `Orphaned == true`.

**Files**: `server/services/approval_store.go`

##### Task 2.2.3a: Add both fields to the `&PendingApproval{}` literal in `loadFromDisk` (~2 min)
- In `approval_store.go`, in `loadFromDisk` (lines 372-384), add to the `a := &PendingApproval{...}` literal after `PermissionMode: p.PermissionMode,`:
  ```go
  EscalationReason:   p.EscalationReason,
  EscalationCategory: p.EscalationCategory,
  ```
- Files: `server/services/approval_store.go`

#### Story 2.2.4: `ApprovalMetadata` + `GetApprovalMetadataBySession`
**As a** developer, **I want** the two fields threaded through the poller-facing DTO, **so that**
the review-queue poller (which only sees `ApprovalMetadata`, never `PendingApproval` directly) has
access to them.

**Acceptance Criteria** (AC1, AC2 — the last hop before the poller):
- *Given* a `PendingApproval` in the store with `EscalationReason: "x", EscalationCategory: "domain-age"` for session `"my-session"`, *When* `GetApprovalMetadataBySession("my-session")` is called, *Then* the returned `[]session.ApprovalMetadata` has an entry with `EscalationReason == "x"` and `EscalationCategory == "domain-age"`.

**Files**: `session/review_queue_poller.go`, `server/services/approval_store.go`

##### Task 2.2.4a: Add two fields to `session.ApprovalMetadata` struct (~2 min)
- In `review_queue_poller.go`, in the `ApprovalMetadata` struct (lines 54-61), add after `Orphaned bool`:
  ```go
  EscalationReason   string
  EscalationCategory string
  ```
- Files: `session/review_queue_poller.go`

##### Task 2.2.4b: Copy fields in `GetApprovalMetadataBySession` (~2 min)
- In `approval_store.go`, in `GetApprovalMetadataBySession` (lines 137-154), add to the `session.ApprovalMetadata{...}` literal after `Orphaned: a.Orphaned,`:
  ```go
  EscalationReason:   a.EscalationReason,
  EscalationCategory: a.EscalationCategory,
  ```
- Files: `server/services/approval_store.go`

#### Story 2.2.5: `ReviewItem.Metadata` enrichment
**As a** frontend developer, **I want** the escalation reason exposed as two new generic metadata
keys on `ReviewItem`, **so that** `ReviewQueuePanel.tsx` can read them with the same
`queueItem.metadata?.["…"]` idiom already used for `tool_input_command`/`cwd`/etc. — no proto change
needed since `Metadata` is already `map<string,string>`.

**Acceptance Criteria** (AC1, this is what AC1's "queue item shows an explanation" ultimately reads from):
- *Given* `ApprovalMetadata{ApprovalID: "abc-123", EscalationReason: "No matching rule; escalated for manual review.", EscalationCategory: "no-match", ...}` returned by the provider for session `"my-session"`, *When* the poller's enrichment block (`review_queue_poller.go:807-829`) runs for that session's `ReviewItem`, *Then* `item.Metadata["escalation_reason"] == "No matching rule; escalated for manual review."` and `item.Metadata["escalation_reason_category"] == "no-match"`.

**Files**: `session/review_queue_poller.go`

##### Task 2.2.5a: Add two metadata keys in the enrichment block (~3 min)
- In `review_queue_poller.go`, inside `if reason == ReasonApprovalPending && rqp.approvalProvider != nil { ... }` (lines 807-830), add after the existing `if a.Cwd != "" { item.Metadata["cwd"] = a.Cwd }` block:
  ```go
  if a.EscalationReason != "" {
      item.Metadata["escalation_reason"] = a.EscalationReason
  }
  if a.EscalationCategory != "" {
      item.Metadata["escalation_reason_category"] = a.EscalationCategory
  }
  ```
- Files: `session/review_queue_poller.go`

---

## Phase 3: Frontend Display (AC1, AC3, AC6, AC7)

### Epic 3.1: Reason line rendering
**Goal**: Render the escalation reason as plain text inside the existing `pending_approval_id`
block, with an emoji prefix keyed by category (WCAG 1.4.1 — not color-only), and wire it into the
card's accessible description.

#### Story 3.1.1: `itemContext` reason paragraph
**As a** reviewer looking at the review queue, **I want** to see *why* a request was escalated
before I see the raw command, **so that** I can decide how carefully to read the command that
follows.

**Acceptance Criteria** (AC1 rendering, AC6, and orphaned-fallback UX):
- *Given* a `ReviewItem` with `metadata["pending_approval_id"] = "abc-123"`, `metadata["escalation_reason"] = "No matching rule; escalated for manual review."`, `metadata["escalation_reason_category"] = "no-match"`, *When* `ReviewQueuePanel` renders that card, *Then* a `<p className={escalationReasonText} id="escalation-reason-<sessionId>">❓ No matching rule; escalated for manual review.</p>` renders as the *first* child inside the `pending_approval_id` block, before the `commandPreview` `<pre>`.
- *Given* `metadata["escalation_reason_category"] = "explicit-rule"` and `metadata["escalation_reason"] = "Branch deletion modifies repository structure and should be reviewed."`, *When* rendered, *Then* the paragraph reads `🛑 Branch deletion modifies repository structure and should be reviewed.` (backend text rendered verbatim, only the emoji prefix is category-driven per ux.md — the frontend does not reconstruct or re-wrap the sentence).
- *Given* `metadata["pending_approval_id"]` is present but `metadata["escalation_reason"]` is absent (an orphaned approval persisted before this feature shipped — the JSON `omitempty` case from the Migration Plan), *When* rendered, *Then* the paragraph shows the fallback copy `"Reason not recorded — this request predates escalation-reason tracking."` instead of being omitted (an empty reason line next to cards that do show one would look like broken UI, per ux.md #4) — using a plain string, no emoji, since no category is known.
- *Given* the poller enrichment in Epic 2.2 is synchronous with `store.Create` (confirmed via `h.queueChecker.CheckSession(inst)` being called immediately after `store.Create` in `HandlePermissionRequest`, `approval_handler.go:383-388` — see pitfalls.md #3), *When* a brand-new escalation is created, *Then* there is no "metadata present but reason still loading" intermediate state to design for — the existing `queueItem.metadata?.["key"] && (...)` guard pattern is sufficient without a new loading-state branch.
- *Given* `metadata["escalation_reason"]` is a 600-character string (exceeding the 500-char cap
  Task 2.1.3a applies at write time, so this specifically exercises the truncated/ellipsized case
  a rule author's verbose `Reason` could still produce), *When* rendered, *Then* the paragraph
  renders within `escalationReasonText`'s bounded box (`maxHeight: 6em`, `overflowY: auto`,
  `wordBreak: break-word`) rather than growing the card unboundedly — this closes the UX design gap
  flagged for Surface A (design/ux.md gap #1) with an explicit test, not just task prose.

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.1.1a: Add a 5-entry category → emoji lookup map (~3 min)
- Near the top of `ReviewQueuePanel.tsx` (module scope, alongside other constants), add:
  ```ts
  const ESCALATION_REASON_EMOJI: Record<string, string> = {
    "no-match": "❓",
    "explicit-rule": "🛑",
    "domain-age": "🌐",
    "unclassifiable": "⚙️",
    "unexpected": "⚠️",
  };
  ```
  (No `secret-scan` entry — that category never reaches a `ReviewItem`, per requirements.md's
  out-of-scope note; an unrecognized/missing category falls through to no emoji via `?? ""`. The
  `unexpected` entry is the pre-mortem P3 fix's category — its text already reads "An internal
  classification error occurred" from `EscalationReasonText`, so no button-gating change is needed
  here: it's naturally excluded from the no-match-only Create Rule gate in Epic 3.2.)
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.1.1b: Insert the reason `<p>` as the first child of the `pending_approval_id` block (~4 min)
- In `ReviewQueuePanel.tsx`, inside the `{queueItem.metadata?.["pending_approval_id"] && ( <> ... )}` block (lines 726-743), insert as the *first* child inside the `<>` fragment, before the `commandPreview` conditional:
  ```tsx
  <p
    className={escalationReasonText}
    id={`escalation-reason-${queueItem.sessionId}`}
  >
    {queueItem.metadata["escalation_reason"]
      ? `${ESCALATION_REASON_EMOJI[queueItem.metadata["escalation_reason_category"] ?? ""] ?? ""} ${queueItem.metadata["escalation_reason"]}`.trim()
      : "Reason not recorded — this request predates escalation-reason tracking."}
  </p>
  ```
- Do not modify the unrelated suppression guard at line 718 (`queueItem.context && !queueItem.metadata?.["pending_approval_id"] && (...)`) — that governs a different field (`queueItem.context`).
- UX design gap (`design/ux.md`): unlike its sibling `commandPreview` (which bounds long content via
  `maxHeight`/`overflowY`/`wordBreak` in `ReviewQueuePanel.css.ts:202-223`), `itemContext` has no
  such bound — an explicit-rule's free-text `Reason` (rule-author-authored, unbounded length) could
  grow the card unboundedly.
- **Consistency-check concern, resolved explicitly**: `itemContext` is also used at
  `ReviewQueuePanel.tsx:719` for the unrelated `queueItem.context` field on non-approval cards —
  do **not** add the bound to the shared `itemContext` class itself, since that would silently
  change rendering for that unrelated field too. Instead add a new vanilla-extract style in
  `ReviewQueuePanel.css.ts` that composes `itemContext`'s existing rules and adds the bound on top:
  ```ts
  export const escalationReasonText = style([
    itemContext,
    { maxHeight: "6em", overflowY: "auto", wordBreak: "break-word" },
  ]);
  ```
  (mirrors `commandPreview`'s own bound values) and use `className={escalationReasonText}` on the
  new `<p>` in Task 3.1.1b instead of `className={itemContext}` — this closes the ambiguity the
  review flagged ("or a scoped variant") in favor of the scoped-variant option, leaving the shared
  class and the `queueItem.context` render path untouched.
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/components/sessions/ReviewQueuePanel.css.ts`

##### Task 3.1.1c: Wire `aria-describedby` on the card's `role="button"` wrapper (~2 min)
- In `ReviewQueuePanel.tsx`, on the `<div className={\`${itemClickable} ...\`} role="button" ...>` wrapper (line 690), add:
  ```tsx
  aria-describedby={
    queueItem.metadata?.["pending_approval_id"]
      ? `escalation-reason-${queueItem.sessionId}`
      : undefined
  }
  ```
- The reason `<p>` itself must not be focusable — do not add `tabIndex` to it (ux.md #3: it only extends the card's accessible description, it isn't independently interactive).
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

### Epic 3.2: Create Rule button gating and intent (AC3, AC7)

#### Story 3.2.1: Gate to no-match only, change intent to `secondary`
**As a** reviewer, **I want** the "Create Rule" button to appear only when it makes sense (no
existing rule covers this command) and to look visually distinct from the primary "✓ Approve"
action, **so that** I don't misclick between two actions of unequal consequence.

**Acceptance Criteria** (AC3, AC7):
- *Given* a `ReviewItem` with `metadata["escalation_reason_category"] = "no-match"` and `metadata["tool_input_command"] = "rm -rf /tmp/foo"`, *When* the card renders its action row, *Then* the "✦ Create Rule" button (`data-testid="create-rule-<sessionId>"`) is rendered with `intent="secondary"`.
- *Given* a `ReviewItem` with `metadata["escalation_reason_category"] = "domain-age"` and `metadata["tool_input_command"]` present, *When* the card renders, *Then* the "Create Rule" button is **not** rendered at all (domain-age has no "prevent this next time" story — the domain's newly-registered status isn't a command pattern a rule can express).
- *Given* the same no-match `ReviewItem`, *When* the action row renders, *Then* the "✓ Approve" button retains `intent="primary"` unchanged (AC7's non-negotiable: `primary` stays reserved for Approve).
- *Given* clicking the visible "Create Rule" button, *When* `generateRule({source: SuggestionSource.COMMAND_SAMPLE, commandSample: "rm -rf /tmp/foo", toolNameFilter: "Bash"})` fires, *Then* the existing `SuggestedRuleCard`/`createPortal` modal flow (lines 1344-1424) opens unchanged — no new RPC plumbing (AC3's explicit scope boundary).

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.2.1a: Change the button's render condition (~3 min)
- In `ReviewQueuePanel.tsx` at line 818, change:
  ```tsx
  {queueItem.metadata?.["tool_input_command"] && (
  ```
  to:
  ```tsx
  {queueItem.metadata?.["tool_input_command"] &&
    queueItem.metadata?.["escalation_reason_category"] === "no-match" && (
  ```
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.2.1b: Change `intent="ghost"` to `intent="secondary"` (~1 min)
- In `ReviewQueuePanel.tsx` at line 820, change `intent="ghost"` to `intent="secondary"` on the Create Rule `<Button>`.
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

#### Story 3.2.2: Jest coverage for reason rendering and button gating
**As a** developer, **I want** the new render branches covered by component tests, **so that**
regressions in the gating logic or intent are caught before merge.

**Acceptance Criteria** (AC5, and codifies the AC3/AC6/AC7 examples above as tests):
- *Given* a mocked `ReviewItem` with `escalation_reason_category: "no-match"`, *When* `ReviewQueuePanel` renders, *Then* the test asserts the reason `<p>` text content and that `create-rule-<id>` is present with `intent="secondary"` (via the rendered class/attribute, not a raw CSS selector — this repo's e2e locator rules don't apply to Jest/RTL, but keep to `data-testid`/role queries for consistency).
- *Given* a mocked `ReviewItem` with `escalation_reason_category: "domain-age"`, *When* rendered, *Then* the test asserts `create-rule-<id>` is **absent** (`queryByTestId` returns `null`).
- *Given* a mocked `ReviewItem` with `pending_approval_id` set but no `escalation_reason` key, *When* rendered, *Then* the test asserts the fallback copy string is shown.
- *Given* a mocked `ReviewItem` with a 600-character `escalation_reason` string, *When* rendered, *Then* the test asserts the reason `<p>` carries the `escalationReasonText` class (not bare `itemContext`) — a proxy assertion for the `maxHeight`/`overflowY` bound applying, since jsdom doesn't compute layout (closes Story 3.1.1's overflow-bound AC and consistency-check nitpick #5).

**Files**: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

##### Task 3.2.2a: Add the 4 test cases above (~6 min)
- In the existing `ReviewQueuePanel.test.tsx`, add a `describe("escalation reason", ...)` block with the 4 cases from the AC.
- Files: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

---

## Phase 4: Analytics Breakdown (AC4)

### Epic 4.1: Proto field + Go aggregation

#### Story 4.1.1: Proto field
**As a** backend developer, **I want** a new typed field on `AnalyticsSummaryProto` for the
escalation-reason breakdown, **so that** the RPC contract carries fixed-key counts instead of
requiring the frontend to derive them from raw entries.

**Acceptance Criteria**:
- *Given* the current `AnalyticsSummaryProto` message with fields numbered 1-16, *When* `escalation_reason_counts` is added, *Then* it uses field number 17 (next available) and type `map<string, int32>`, matching the existing `decision_counts = 2` idiom.
- *Given* the proto change, *When* `make proto-gen` runs, *Then* `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` regenerate with `EscalationReasonCounts map[string]int32` (Go) and `escalationReasonCounts: { [key: string]: number }` (TS), and `go build ./...` plus a frontend typecheck both pass with no other diffs.

**Files**: `proto/session/v1/types.proto`

##### Task 4.1.1a: Add the field to `AnalyticsSummaryProto` (~2 min)
- In `proto/session/v1/types.proto`, inside `message AnalyticsSummaryProto { ... }` (lines 1107-1134), add after `repeated SubcommandStatProto command_subcommand_stats = 16;`:
  ```protobuf
  // Escalation-reason breakdown: counts per category ("no-match", "explicit-rule",
  // "domain-age", "secret-scan", "unclassifiable") — see classifier.EscalationCategory.
  map<string, int32> escalation_reason_counts = 17;
  ```
- Files: `proto/session/v1/types.proto`

##### Task 4.1.1b: Regenerate and verify build (~3 min)
- Run `make proto-gen` (never hand-invoke `buf`/`protoc`), then `go build ./...`, then a frontend typecheck (`cd web-app && npx tsc --noEmit` or the project's existing typecheck script).
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*_pb.ts` (generated)

#### Story 4.1.2: `ComputeSummary` aggregation
**As a** backend developer, **I want** `ComputeSummary` to bucket every escalate/secret-scan-deny
entry into one of the 5 categories, **so that** `ApprovalAnalyticsPanel` has real counts to render.

**Acceptance Criteria** (AC4):
- *Given* `entries = []AnalyticsEntry{ {Decision: "escalate", RuleID: ""}, {Decision: "escalate", RuleID: ""}, {Decision: "escalate", RuleID: "new-domain-check"}, {Decision: "auto_deny", RuleID: "secret-scan"}, {Decision: "escalate", RuleID: "shell-expansion-program"}, {Decision: "escalate", RuleID: "seed-escalate-git-branch-safe-delete"} }`, *When* `ComputeSummary(entries)` runs, *Then* `summary.EscalationReasonCounts == map[string]int{"no-match": 2, "domain-age": 1, "secret-scan": 1, "unclassifiable": 1, "explicit-rule": 1}`.
- *Given* an entry `{Decision: "auto_allow", RuleID: "some-rule"}` (not an escalation and not the secret-scan auto-deny special case), *When* `ComputeSummary` runs, *Then* it contributes to no key in `EscalationReasonCounts` (only `escalate` entries and the specific `auto_deny`+`secret-scan` combination count — this is intentionally broader than the existing coverage-gap branch at line 396, which only catches `RuleID == ""`, per architecture.md's explicit note that this new branch must catch all 5 categories, not just no-match).

**Files**: `server/services/analytics_store.go`

##### Task 4.1.2a: Add `EscalationReasonCounts` field to `AnalyticsSummary` struct (~2 min)
- In `analytics_store.go`, in the `AnalyticsSummary` struct (lines 88-114), add after `CommandSubcommandStats []SubcommandStat \`json:"command_subcommand_stats"\``:
  ```go
  // EscalationReasonCounts breaks down escalations by category (classifier.EscalationCategory
  // string values) — no-match, explicit-rule, domain-age, secret-scan, unclassifiable.
  EscalationReasonCounts map[string]int `json:"escalation_reason_counts"`
  ```
- Files: `server/services/analytics_store.go`

##### Task 4.1.2b: Initialize the accumulator and add the categorization branch (~4 min)
- In `analytics_store.go`, in `ComputeSummary` (lines 317-440):
  - Declare `escalationReasonCounts := make(map[string]int)` alongside the other per-loop
    accumulator maps (`toolCounts`, `deniedCmds`, `ruleCounts`, …, lines 328-339) — i.e. **after**
    the `if len(entries) == 0 { return summary }` early return, not before it. This matches the
    precedent of every other `Top*`-backing accumulator (all nil-for-zero-entries, not
    always-initialized like `DecisionCounts`): `summary.EscalationReasonCounts` is nil when
    `entries` is empty, same as `summary.TopTools`/`summary.TopUncoveredTools` today. A proto
    `map<string,int32>` marshals a nil Go map as an empty map on the wire either way, so this
    has no observable effect on the frontend.
  - Inside the `for _, e := range entries {` loop, after the existing coverage-gap branch (lines 395-406), add:
    ```go
    // Escalation-reason breakdown: every escalate decision, plus the terminal
    // secret-scan auto-deny (which never reaches this loop via `escalate` since it's
    // an AutoDeny — see requirements.md's AC4 scope note). Broader than the
    // coverage-gap branch above (which only catches RuleID == ""): this must catch
    // all 5 categories.
    if e.Decision == "escalate" || (e.Decision == "auto_deny" && e.RuleID == "secret-scan") {
        cat := classifier.CategorizeEscalationRuleID(e.RuleID)
        escalationReasonCounts[string(cat)]++
    }
    ```
- Files: `server/services/analytics_store.go`

##### Task 4.1.2c: Assign the accumulator to the summary before return (~1 min)
- In `analytics_store.go`, in `ComputeSummary`, before `return summary` (line 439), add `summary.EscalationReasonCounts = escalationReasonCounts`.
- Files: `server/services/analytics_store.go`

##### Task 4.1.2d: `summaryToProto` conversion loop (~2 min)
- In `server/services/rules_service.go`, in `summaryToProto` (lines 515+), add after the `CommandSubcommandStats` loop:
  ```go
  p.EscalationReasonCounts = make(map[string]int32, len(s.EscalationReasonCounts))
  for k, v := range s.EscalationReasonCounts {
      p.EscalationReasonCounts[k] = int32(v)
  }
  ```
- Files: `server/services/rules_service.go`

##### Task 4.1.2e: Unit test for the aggregation (~5 min)
- In `server/services/analytics_store_test.go`, add `TestComputeSummary_EscalationReasonCounts` using the exact fixture and expected map from the Story 4.1.2 AC above.
- Files: `server/services/analytics_store_test.go`

### Epic 4.2: Frontend analytics table + test (AC4)

#### Story 4.2.1: "Escalation Reasons" table in `ApprovalAnalyticsPanel`
**As a** reviewer of approval analytics, **I want** a table breaking down escalation counts by
category, **so that** I can see which category dominates over the selected time window.

**Acceptance Criteria** (AC4 — the "frontend rendering test" requirement, not satisfied by the
backend unit test alone):
- *Given* `summary.escalationReasonCounts = {"no-match": 12, "explicit-rule": 5, "domain-age": 2, "secret-scan": 1, "unclassifiable": 0}` returned by a mocked `useApprovalAnalytics`, *When* `ApprovalAnalyticsPanel` renders, *Then* a "Escalation Reasons" table section renders with one row per non-zero-count category, showing the mapped label (via `ESCALATION_CATEGORY_LABELS`) and the count — e.g. a row reading `No auto-approval rule matched` / `12`.
- *Given* `summary.escalationReasonCounts = {"no-match": 0, "explicit-rule": 0, "domain-age": 0, "secret-scan": 0, "unclassifiable": 0}` (or an empty/nil map — e.g. a 7-day window where every
  decision was auto-allow/auto-deny), *When* rendered, *Then* the section shows the panel's existing
  `empty`-styled message ("No escalations in this window.") instead of a header with zero table
  rows — this closes UX design gap #2 (design/ux.md, UX-AC17) with an explicit test, not just task
  prose (consistency-check nitpick #4).

**Files**: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`, `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx`

##### Task 4.2.1a: Add `ESCALATION_CATEGORY_LABELS` map (~2 min)
- In `ApprovalAnalyticsPanel.tsx`, near existing constants, add:
  ```ts
  const ESCALATION_CATEGORY_LABELS: Record<string, string> = {
    "no-match": "No auto-approval rule matched",
    "explicit-rule": "Rule explicitly flagged for review",
    "domain-age": "Newly-registered domain",
    "secret-scan": "Plaintext secret detected",
    "unclassifiable": "Shell expansion — couldn't classify",
    "unexpected": "Internal classification error",
  };
  ```
- Files: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

##### Task 4.2.1b: Add the table section (~6 min)
- In `ApprovalAnalyticsPanel.tsx`, modeled directly on the "Top Triggered Rules" table (lines 276-301), add a new section reading from `summary.escalationReasonCounts`, rendering one row per key with `count > 0`, sorted descending by count, using the existing `table`/`th`/`td`/`row`/`tableSection`/`sectionTitle`/`Bar` styling primitives already imported in this file.
- Label lookup: use `ESCALATION_CATEGORY_LABELS[category] ?? category` (architecture review
  concern: without a fallback, an unmapped category key renders the literal string `undefined`,
  unlike `ESCALATION_REASON_EMOJI`'s already-specified `?? ""` fallback in Task 3.1.1a).
- Empty state (UX design gap flagged in `design/ux.md`): when every category count is 0 (or
  `summary.escalationReasonCounts` is empty/nil), render this section's existing panel-wide
  `empty`/`emptyHint` pattern (see the Daily Breakdown section, `ApprovalAnalyticsPanel.tsx:189-194`)
  instead of an empty table — e.g. `empty` text "No escalations in this window" — rather than
  omitting the section silently.
- Files: `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

##### Task 4.2.1c: Jest tests for the new section, including the empty-state case (~6 min)
- In `ApprovalAnalyticsPanel.test.tsx`, add a test mocking `useApprovalAnalytics` to return the non-zero fixture from the Story 4.2.1 AC, asserting the section renders with the expected labels and counts.
- Add a second test with the all-zero fixture, asserting the `empty`-styled message renders instead of an empty table (closes nitpick #4 above).
- Files: `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx`

---

## Phase 5: Regression + New Backend Tests (AC2, AC5)

### Epic 5.1: `approval_handler_test.go` / `approval_service_test.go`

#### Story 5.1.1: Confirm no regressions, add new escalation-reason cases
**As a** developer, **I want** the existing auto-allow/auto-deny/timeout/orphaned-reload test
suites to pass completely unmodified, plus new cases asserting the 3 AC1 paths and the AC2
orphaned-reload path, **so that** this feature is provably additive.

**Acceptance Criteria** (AC5, AC1, AC2):
- *Given* the full pre-existing `approval_handler_test.go` and `review_queue_determiner_test.go` suites, *When* `make test` runs after all Phase 1-4 changes, *Then* every pre-existing test case passes with zero code changes to its assertions (only new test functions are added to these files, no existing function bodies edited).
- *Given* a fresh `HandlePermissionRequest` call with a command matching no rule, *When* the response is inspected via the store (`store.Get(approvalID)`), *Then* `EscalationReason == "No matching rule; escalated for manual review."` and `EscalationCategory == "no-match"`.
- *Given* the same for an explicit-rule match (`git branch -d feature/foo`) and a domain-age match, *Then* the corresponding category/reason pairs from Stories 2.1.1/2.1.2's examples hold.
- *Given* a `pending_approvals.json` fixture written with `escalation_reason`/`escalation_category` populated for one entry, *When* a new `ApprovalStore` is constructed against that file (simulating restart) and `GetApprovalMetadataBySession` is called, *Then* the returned metadata has both fields populated and `Orphaned == true` (this is the AC2 "four-struct chain" regression guard from features.md #4 — if any single struct/copy-loop from Epic 2.2 was missed, this test fails with an empty string, not a compile error).

**Files**: `server/services/approval_handler_test.go` (new test functions only), `server/services/approval_service_test.go` (new test function)

##### Task 5.1.1a: Baseline check (~2 min)
- Run `go test ./server/services -run TestHandlePermissionRequest` and `go test ./session -run TestReviewQueueDeterminer` before any Phase 1-4 code lands (or immediately after, to confirm zero diffs needed) to establish the "unmodified" baseline claim in the AC.
- Files: none (verification only)

##### Task 5.1.1b: Add 3 new test functions for the AC1 paths (~5 min)
- In `approval_handler_test.go`, add `TestHandlePermissionRequest_EscalationReason_NoMatch`, `TestHandlePermissionRequest_EscalationReason_ExplicitRule`, `TestHandlePermissionRequest_EscalationReason_DomainAge`, each asserting the `PendingApproval.EscalationReason`/`.EscalationCategory` values from Story 5.1.1's AC, using this file's existing test harness/fixtures for classifier and domain-checker setup.
- Files: `server/services/approval_handler_test.go`

##### Task 5.1.1c: Add orphaned-reload regression test (~5 min)
- In `approval_service_test.go`, add `TestApprovalStore_LoadFromDisk_PreservesEscalationReason`, writing a `PersistedApproval` JSON fixture with both new fields populated, constructing a fresh `ApprovalStore` against that file, and asserting `GetApprovalMetadataBySession` returns both fields intact.
- Files: `server/services/approval_service_test.go`

##### Task 5.1.1d: Concurrency regression test (~5 min) — pre-mortem P2
- In `approval_service_test.go`, add `TestApprovalStore_Create_ConcurrentEscalations_NoDataRace`: spin up N (e.g. 20) goroutines each calling `store.Create` with a distinct `PendingApproval{EscalationReason: ..., EscalationCategory: ...}`, run with `go test -race`, and assert all N entries are present and intact afterward (no lost writes, no corrupted `EscalationReason` across entries). This is a regression guard, not a new capability — `ApprovalStore`'s existing single-mutex design already serializes `Create`, so the test's job is confirming this feature's two new string fields don't introduce a copy/aliasing bug under concurrent load, not benchmarking throughput.
- Files: `server/services/approval_service_test.go`

### Epic 5.2: `review_queue_determiner_test.go` regression confirmation
No code changes expected in `review_queue_determiner.go` itself (per requirements.md's grounded
design constraint: it decides *whether* to escalate, not *why* — orthogonal to this feature). Task
is verification-only.

##### Task 5.2.1a: Confirm suite passes unmodified (~2 min)
- Run `go test ./session -run TestReviewQueueDeterminer` after Phase 1-4 lands; expect zero diffs.
- Files: none (verification only)

---

## Phase 6: End-to-End Coverage (AC8)

### Epic 6.1: `escalation-reasoning.spec.ts`

#### Story 6.1.1: Real end-to-end no-match escalation
**As a** developer, **I want** a Playwright spec that drives an actual escalation through the real
HTTP hook endpoint (not a mocked `queueItem.metadata`), **so that** the full chain — hook →
classifier → `ApprovalStore` → poller → `ReviewItem.Metadata` → `ReviewQueuePanel` render — is
proven end to end.

**Acceptance Criteria** (AC8):
- *Given* a session created via `SessionClient` helpers pointed at a command no rule matches (e.g. `tool_input.command = "totally-unmatched-cmd-xyz123 --flag"`), *When* the spec POSTs that payload to `/api/hooks/permission-request` (`server/server.go:484-485`) **without awaiting the response** (the hook blocks server-side until decision/timeout, per pitfalls.md #5) and then calls `waitForReviewQueue(1)`, *Then* the returned queue contains one item for that session.
- *Given* that queue item, *When* the spec navigates to `/review-queue` and locates the card via `page.getByTestId(\`review-item-${sessionId}\`)`, *Then* the escalation reason text ("No matching rule; escalated for manual review." or the plain-text fallback, whichever the fixture triggers) is visible in the rendered DOM.
- *Given* the test has created a pending approval, *When* the test ends (pass or fail), *Then* a cleanup step (`finally`/`afterEach`) resolves it via `approve-${sessionId}` or `deny-${sessionId}` — the spec must not rely on the ~4-minute timeout sweep, since the e2e suite shares one server instance across the whole run and an abandoned approval would pollute other specs' queue-state assertions (pitfalls.md #5).

**Files**: `tests/e2e/escalation-reasoning.spec.ts` (new)

##### Task 6.1.1a: Spec scaffold + unawaited POST (~5 min)
- New file `tests/e2e/escalation-reasoning.spec.ts`, starting with `// @feature session:create, review-queue:list` (adjust to the actual RPC feature IDs used by `waitForReviewQueue`/`getReviewQueue` — check `docs/registry/features/backend/review-queue/get.json`'s `id` field for the exact string).
- Create a session via the existing `SessionClient` session-creation helper, then fire `page.request.post("/api/hooks/permission-request", {data: {...}})` as a backgrounded promise (do not `await` it in the main test flow) with a command guaranteed to match no seed rule.
- Files: `tests/e2e/escalation-reasoning.spec.ts`

##### Task 6.1.1b: Poll and assert reason visible (~4 min)
- Call `client.waitForReviewQueue(1)`, then `page.goto("/review-queue")`, then assert via `page.getByTestId(\`review-item-${sessionId}\`)` that the reason text is visible (e.g. `.toContainText(...)`).
- Files: `tests/e2e/escalation-reasoning.spec.ts`

##### Task 6.1.1c: Cleanup — resolve the approval (~3 min)
- In a `test.afterEach` (or `try/finally` within the test body), click `deny-${sessionId}` (or `approve-${sessionId}`) to resolve the pending approval regardless of test outcome, then `await` the backgrounded POST promise from Task 6.1.1a to confirm the hook itself returns (not just that the queue item disappeared).
- Files: `tests/e2e/escalation-reasoning.spec.ts`

---

## Phase 7: Registry & Docs

### Epic 7.1: Feature registry updates
Per `.claude/rules/feature-registry.md`. No new RPC signature was added (proto field on an existing
message + existing generic metadata map, not a new endpoint), so no new backend entry is strictly
required for AC1-3/6-8 — but the two existing entries whose behavior changed get their
`lastModified` bumped, and the frontend gets its first registry entries for these components (a
pre-existing gap per features.md #5, closed here rather than perpetuated).

##### Task 7.1.1a: Bump `lastModified` on 2 existing backend entries (~2 min)
- Edit `docs/registry/features/backend/approval/get-analytics.json` and `docs/registry/features/backend/review-queue/get.json` — update `lastModified` to the implementation date, and (for `get-analytics.json`) add the new test IDs from Task 4.1.2e/4.2.1c to `testIds`, setting `tested: true`.
- Files: `docs/registry/features/backend/approval/get-analytics.json`, `docs/registry/features/backend/review-queue/get.json`

##### Task 7.1.1b: Add new frontend registry entries — both new UI surfaces (~4 min)
- Create `docs/registry/features/frontend/escalation-reasoning-display.json` per the schema in `docs/registry/schema.json`, `filePath: "web-app/src/components/sessions/ReviewQueuePanel.tsx"`, `testIds` populated from Task 3.2.2a.
- Adversarial review concern: Epic 4.2's "Escalation Reasons" table (`ApprovalAnalyticsPanel.tsx`,
  Story 4.2.1) is a second, independent new UI feature with its own Jest test (Task 4.2.1c) — also
  create `docs/registry/features/frontend/approval-analytics-reason-breakdown.json`,
  `filePath: "web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx"`, `testIds` populated from
  Task 4.2.1c. Omitting this second entry would violate `.claude/rules/feature-registry.md`'s "New
  UI feature → create a registry entry" rule, and `ApprovalAnalyticsPanel.tsx` has zero existing
  registry entries today so the coverage-gaps check in Task 7.1.1c won't catch the omission on its
  own.
- Files: `docs/registry/features/frontend/escalation-reasoning-display.json`, `docs/registry/features/frontend/approval-analytics-reason-breakdown.json`

##### Task 7.1.1c: Regenerate aggregated registry (~1 min)
- Run `make registry-generate`; confirm `docs/registry/coverage-gaps.json` count does not grow.
- Files: generated (`docs/registry/*.json` aggregates)
