# Architecture Research: Threading `classifier.RiskLevel` to the Review Queue

## Judgment: no EventStorming table

This is field-threading through an existing pipeline, not new multi-actor business logic.
There is exactly one producer (the classifier, at 4 call sites that already compute a
`RiskLevel`) and two consumers (an RPC response and two independent frontend read paths).
No new commands, no new state machine, no new actor. An Event-Command-Policy table would
manufacture ceremony for what is a struct-field-plus-DTO-plus-proto-field change. Skipping it.

## 1. Confirmed data flow: where `RiskLevel` is produced and where it is dropped

`classifier.ClassificationResult` (`pkg/classifier/classifier.go:39-47`) carries `RiskLevel
RiskLevel` (4-value enum: `RiskLow`/`RiskMedium`/`RiskHigh`/`RiskCritical`,
`pkg/classifier/classifier.go:16-24`). It is populated at every call site in
`server/services/approval_handler.go` that can lead to a pending approval:

| Call site | File:line | RiskLevel set to |
|---|---|---|
| Secret-scan auto-deny | `approval_handler.go:240-246` | `classifier.RiskCritical` (literal, inline `ClassificationResult`) |
| Domain-age escalation | `approval_handler.go:277-283` | `classifier.RiskHigh` (literal, inline `ClassificationResult`) |
| Rule-based classification via `h.classifier.Classify()` | `approval_handler.go:328` | whatever `classifier.Rule.RiskLevel` (`classifier.go:369`) the matched rule carries, or a seed-rule literal (`classifier.go:452-1339`, e.g. `RiskCritical` for `rm`/force-push at `classifier.go:765,778,825,863`) |
| Unrecognized-decision fail-safe | `approval_handler.go:379-380` | whatever `result.RiskLevel` was already on the (unexpected) decision |

All four converge on the single `escalation classifier.ClassificationResult` local
(`approval_handler.go:256`, set via `goto createApproval` for secret-scan/domain-age or
direct assignment for the `Escalate` case at `approval_handler.go:365`).

At the `createApproval:` label (`approval_handler.go:384-440`), only two fields of
`escalation` survive onto the `PendingApproval` struct:

```go
EscalationReason:   truncateEscalationReason(classifier.EscalationReasonText(escalation)),
EscalationCategory: string(classifier.CategorizeEscalationRuleID(escalation.RuleID)),
```
(`approval_handler.go:436-437`)

`escalation.RiskLevel` is never read here. **requirements.md's claim is precise and confirmed
verbatim** — the classifier already computes a `RiskLevel` for every escalation path,
including the pattern-derived seed rules matching the issue's own examples, and it is dropped
at this exact assignment.

Where it *is* recorded today: only into analytics, via `AnalyticsStore.RecordFromResult`
(`analytics_store.go:163-203`, called from `approval_handler.go:240`, `285`, `340`), which
converts it with the existing `riskLevelString()` helper (`analytics_store.go:574-586`,
returns `"low"/"medium"/"high"/"critical"`) into `AnalyticsEntry.RiskLevel string`
(`analytics_store.go:26`). This is a fire-and-forget write to the analytics DB — it never
flows back into the live `PendingApproval` the reviewer sees.

## 2. Full chain of drop points (every place `RiskLevel` needs to be threaded)

The gap is not a single struct — it is a **5-hop relay**, and every hop after hop 1 currently
loses the value. Confirmed by reading each struct/function in the chain:

```
classifier.ClassificationResult.RiskLevel   (already exists, pkg/classifier/classifier.go:41)
    │  DROPPED HERE — approval_handler.go:436-437 only copies EscalationReason/Category
    ▼
PendingApproval                              (server/services/approval_store.go:21-46) — no RiskLevel field
    │  DROPPED HERE — persistToDiskLocked (approval_store.go:302-323) doesn't serialize it
    ▼
PersistedApproval                            (approval_store.go:49-62) — no RiskLevel field
    │  (loadFromDisk, approval_store.go:381-399, would need the same field on read-back)
    ▼
session.ApprovalMetadata                     (session/review_queue_poller.go:55-68) — no RiskLevel field
    │  DROPPED HERE — GetApprovalMetadataBySession (approval_store.go:146-165) doesn't populate it
    ▼
   ┌─────────────────────────────┴─────────────────────────────┐
   │ PATH A: ApprovalCard/ApprovalDrawer                        │ PATH B: ReviewQueuePanel
   ▼                                                             ▼
PendingApprovalProto                          ReviewItem.Metadata["risk_level"]
(proto/session/v1/types.proto:1034-1061,       (session/review_queue_poller.go:820-862 —
 no risk_level field)                           the enrichment block that today copies
    │  DROPPED HERE — approval_service.go:174-185               escalation_reason/escalation_reason_category
    │  builder doesn't set it                                   into the metadata string map)
    ▼                                                             │  DROPPED HERE — no risk_level key set
PlainApproval (TS)                                                ▼
(web-app/src/lib/api/approvalsApi.ts:13-22,                ReviewQueuePanel.tsx reads
 no riskLevel field)                                        queueItem.metadata["risk_level"]
    ▼
ApprovalCard.tsx / ApprovalDrawer.tsx (no severity UI today)
```

## 3. Critical correction to the research brief: there are TWO independent frontend paths

The brief asked me to verify whether the frontend consumer is `ApprovalCard.tsx`/
`ApprovalDrawer.tsx` rather than `ReviewQueuePanel.tsx`. **Both are real, and they are
architecturally distinct, non-overlapping data paths** — this matters a lot for the plan:

- **`ApprovalDrawer.tsx` / `ApprovalCard.tsx`** (`web-app/src/components/sessions/`) consume
  `PlainApproval` (`web-app/src/lib/api/approvalsApi.ts:13-22`), fetched via
  `ListPendingApprovals` → `PendingApprovalProto` directly, through the `useApprovals` hook
  (`ApprovalDrawer.tsx:21`). This is a **direct RPC-to-component** path.
- **`ReviewQueuePanel.tsx`** (`web-app/src/components/sessions/ReviewQueuePanel.tsx`) does
  **not** consume `PendingApprovalProto` at all. It renders `ReviewItem`
  (`web-app/src/gen/session/v1/types_pb`, backing `session/queue/queue.go`'s session-attention
  queue), and approval-specific fields are flattened into `ReviewItem.metadata` (a
  `map<string,string>`, proto `types.proto:575`) by the enrichment block at
  `session/review_queue_poller.go:820-862`. Confirmed at
  `ReviewQueuePanel.tsx:744-770`: the component reads
  `queueItem.metadata["escalation_reason"]` / `queueItem.metadata["escalation_reason_category"]`
  / `queueItem.metadata["pending_approval_id"]` etc. — string keys, not a nested proto message.

So requirements.md's AC1–AC2 (thread `RiskLevel` into `PendingApproval` and
`ListPendingApprovals`) satisfy **Path A only**. AC3–AC4 (badge + sort + filter on
`ReviewQueuePanel`) require the **separate** `ReviewItem.Metadata["risk_level"]` enrichment
in `review_queue_poller.go`, following the exact precedent already there for
`escalation_reason`/`escalation_reason_category` (`review_queue_poller.go:854-859`). Both
paths read from the same `ApprovalStore` (`GetApprovalMetadataBySession` for Path B,
`ListAll`/`GetBySession` for Path A), so both bottom out at the same
`PendingApproval.RiskLevel` field once it exists — but each needs its own plumbing above
that point. This is not extra design work, just an explicit checklist item the plan must not
miss (a `.claude/rules/session-creation-registry.md`-style "N touchpoints" list, scoped to
this feature).

## 4. `ReviewItem.Priority` vs. `classifier.RiskLevel`: confirmed distinct, confirmed same shape trap

`proto/session/v1/types.proto:625-634` defines `Priority` (`PRIORITY_URGENT`=1 "errors,
failures", `PRIORITY_HIGH`=2 "approvals, prompts" — literally all approval-pending items,
regardless of risk — `PRIORITY_MEDIUM`=3, `PRIORITY_LOW`=4). `ReviewQueuePanel.tsx`'s existing
`"priority"` sort option (`ReviewQueuePanel.tsx:376-377`, `(a.priority - b.priority) * dir`)
sorts on *this* enum. Today every approval-pending `ReviewItem` gets the same
`PRIORITY_HIGH`, which is exactly the flatness problem in the Problem statement — but the fix
is not to reinterpret `Priority`, it's additive metadata alongside it. requirements.md's own
audit (point 8) is correct that these must not be merged; the code confirms it structurally
(different enums, different producers — `Priority` comes from `review_queue_poller.go`'s
reason/session-state logic, `RiskLevel` comes from the classifier and is per-tool-call, not
per-session).

**Recommendation for the plan phase** (not decided here, flagging the shape of the decision):
add severity as a *filter and a distinct/optional sort dimension* over
`metadata["risk_level"]`, analogous to how `programFilter`/`categoryFilter` already work in
`ReviewQueuePanel.tsx` (`ReviewQueuePanel.tsx:234-236, 347-354`) — don't fold it into the
existing `"priority"` sort field or the `Priority` proto enum. Only items with
`metadata["pending_approval_id"]` set will have a `risk_level` at all; other `ReviewItem`
reasons (idle, error, task-complete, etc.) have no analog and should sort after/alongside
using existing tiebreakers, per requirements.md's own open question.

## 5. Consistency requirement: point-in-time snapshot, matching existing precedent exactly

`EscalationReason`/`EscalationCategory` are explicitly documented as set-once-at-creation,
never re-derived:

> "EscalationReason and EscalationCategory capture why this request was escalated for manual
> review, set once at creation from classifier.EscalationReasonText /
> classifier.CategorizeEscalationRuleID. Empty for approvals created before this field
> existed (loaded from disk) — never re-derived after creation." (`approval_store.go:32-35`)

`RiskLevel` should follow the identical contract, for two reasons beyond "be consistent":

1. It is *already* a snapshot by construction upstream — `classifier.Classify()` copies
   `rule.RiskLevel` onto the `ClassificationResult` at match time (`classifier.go:521` for the
   generic rule-match path). If the rule is edited or deleted after that, the already-created
   `PendingApproval` has no live reference to the rule to re-derive from anyway (rules are
   looked up by matching, not by stored rule ID reference on the approval).
2. Re-deriving would require re-running classification against current rules for every
   already-pending approval on every read — extra work with no product requirement asking for
   live-updating severity on an in-flight approval. Nothing in requirements.md's acceptance
   criteria asks for this, and `RiskLevel` mutating out from under a reviewer mid-review would
   be confusing (the reviewer approved/denied based on what they saw).

No new mutability, no re-classification hook, no cache invalidation concern. `RiskLevel` is
computed once in `approval_handler.go`'s existing classify call, captured onto
`PendingApproval` at the same line as `EscalationReason`/`EscalationCategory`, done.

## 6. Analytics: extend `AnalyticsSummaryProto`, following the `escalation_reason_counts` precedent exactly

The data is already there — `ClassificationAnalytics`/`AnalyticsEntry.RiskLevel` is written
on every decision via `RecordFromResult` (`analytics_store.go:180`, already using
`riskLevelString()`). No new storage, no schema migration. The aggregation and wire-through
pattern to copy is the **existing** `EscalationReasonCounts` field, which is the closest
possible precedent (also a category → count breakdown, added without a new message type):

- **Go aggregation** (`analytics_store.go:321-458`, `ComputeSummary`): add a
  `riskLevelCounts := make(map[string]int)` local next to `escalationReasonCounts` (line 345),
  increment it unconditionally per entry (`riskLevelCounts[e.RiskLevel]++`, since every entry
  already carries a risk level string, unlike escalation reason which is conditional) inside
  the same `for _, e := range entries` loop (~line 347-423), and assign
  `summary.RiskLevelCounts = riskLevelCounts` next to `summary.EscalationReasonCounts =
  escalationReasonCounts` (line 455).
- **`AnalyticsSummary` struct** (`analytics_store.go:88-118`): add
  `RiskLevelCounts map[string]int \`json:"risk_level_counts"\`` next to
  `EscalationReasonCounts` (line 117).
- **Proto** (`proto/session/v1/types.proto:1111-1140`, `AnalyticsSummaryProto`): add
  `map<string, int32> risk_level_counts = 18;` (next available field number after
  `escalation_reason_counts = 17`).
- **`summaryToProto`** (`server/services/rules_service.go:518-584`): add the same 3-line
  copy loop used for `EscalationReasonCounts` (`rules_service.go:573-576`) for
  `RiskLevelCounts`.

`GetApprovalAnalyticsResponse` itself (`proto/session/v1/session.proto:1429`,
`GetApprovalAnalytics` handler at `rules_service.go:160-202`) needs **no changes** — it
already embeds `AnalyticsSummaryProto` as `Summary`, so the new map field arrives for free.
No new response field, no new RPC, no new message type.

## 7. Proto field additions — concrete, minimal, follows existing string-not-enum convention

`ApprovalRuleProto.risk_level` (`types.proto:1084`) and `SuggestedRuleProto.risk_level`
(`types.proto:1455`) are both `string`, populated via the same `riskLevelString()` helper
(`rules_service.go:487`, and equivalent in `rule_prompt_builder.go:219`). Follow that
convention rather than introducing a new proto enum:

```protobuf
// PendingApprovalProto — add after seconds_remaining (field 9):
// Classifier-assigned risk level ("low"/"medium"/"high"/"critical"), captured once at
// creation time from classifier.ClassificationResult.RiskLevel. Empty for approvals that
// predate this field (loaded from disk before the upgrade).
string risk_level = 10;
```

Go side: `PendingApproval.RiskLevel classifier.RiskLevel` (typed, matching how the struct
already uses typed classifier fields elsewhere in the package) or a plain `string` set via
`riskLevelString()` at creation time — either works; using the typed enum internally and
converting to string only at the two proto/metadata boundaries (`approval_service.go:174`,
`review_queue_poller.go`'s enrichment block) keeps one canonical string-conversion function
(`riskLevelString`, already in `analytics_store.go:574`) instead of duplicating the
low/medium/high/critical mapping a third time.

## 8. Persistence (`PersistedApproval`) — same field, same helper functions already present

`PersistedApproval` (`approval_store.go:49-62`) needs one new
`RiskLevel string \`json:"risk_level,omitempty"\`` field (string on disk, matching how
`EscalationReason`/`EscalationCategory` are stored as strings there — no reason to persist
the typed int enum). Three call sites need the added field, all pure mechanical copies of
the existing `EscalationReason`/`EscalationCategory` handling:

- `persistToDiskLocked` (`approval_store.go:307-323`): add `RiskLevel: a.RiskLevel` (or
  `riskLevelString(a.RiskLevel)` if the in-memory field stays typed) to the `PersistedApproval{}` literal.
- `loadFromDisk` (`approval_store.go:385-399`): add `RiskLevel: p.RiskLevel` to the
  reconstructed `PendingApproval{}`.
- `GetApprovalMetadataBySession` (`approval_store.go:146-165`): add `RiskLevel: a.RiskLevel`
  to the `session.ApprovalMetadata{}` literal (needed for Path B, see §3).

## 9. What this recommendation deliberately avoids

Per `.claude/rules/interface-pollution-checklist.md` and the requirement to keep this
proportional to a small additive feature:

- **No new interface.** `session.ApprovalMetadataProvider` (`review_queue_poller.go:70`-ish)
  already exists and is satisfied by `*ApprovalStore`; it just gains one more field on the
  struct it already returns. No new provider abstraction.
- **No new RPC, no new proto message.** `risk_level` is a field addition to three existing
  messages (`PendingApprovalProto`, and the analytics map on `AnalyticsSummaryProto`); the
  existing `GetApprovalAnalyticsResponse`/`ListPendingApprovalsResponse` wrap them unchanged.
- **No re-classification/mutation service.** RiskLevel is a snapshot, matching
  `EscalationReason`/`EscalationCategory` exactly (§5) — no cache, no invalidation, no new
  background job.
- **No enum in the wire format.** Follows the existing `risk_level string` convention already
  established by `ApprovalRuleProto`/`SuggestedRuleProto` rather than introducing a new proto
  enum type that would need its own Go↔proto conversion functions.
- **Session-attention `Priority` is untouched**, per requirements.md's explicit scope
  boundary and confirmed structurally distinct in §4.

## 10. Answers to requirements.md's open questions (research-level recommendation, not final)

- **4-level vs. 3-level (P0/P1/P2):** confirmed keep the existing 4-level `RiskLevel`
  end-to-end (Go, proto, storage) — the wire format, persistence, and analytics should all use
  the same `low/medium/high/critical` vocabulary already used by `ApprovalRuleProto` and
  `ClassificationAnalytics`. Any P0/P1/P2 relabeling, if wanted, is a **display-only**
  mapping applied in the frontend badge component, not a backend/proto decision — keeps
  analytics and the rules-management UI (which already shows `RiskLevel` by name) consistent
  with the review queue.
- **Sort as primary key or tiebreaker:** given `ReviewQueuePanel` already has an
  age-proximity concern (`oldestAgeSeconds` callout, expiry countdown in `ApprovalCard`) and
  §4's finding that `risk_level` only exists for a subset of `ReviewItem`s, recommend a
  **dedicated, opt-in sort field** (alongside `"priority"`/`"age"`/`"diffSize"`/`"name"` in
  the existing `SortField` union, `ReviewQueuePanel.tsx:121`) rather than silently overriding
  the default queue order — avoids the exact "must not be confused or merged" trap called out
  in requirements.md point 8, and avoids surprising existing users of the default view. Final
  UX call (default-on vs. opt-in) belongs in planning/UX review, not architecture research.
- **Agent-self-reported severity:** confirmed no existing hook/tool contract carries this;
  out of scope for this pass per requirements.md, and this research found nothing that
  changes that assessment.

## Summary of every file that needs a change (for the plan phase's task breakdown)

| File | Change |
|---|---|
| `server/services/approval_handler.go:427-440` | Add `RiskLevel: escalation.RiskLevel` (or `riskLevelString(...)`) to the `PendingApproval{}` literal |
| `server/services/approval_store.go:21-46` (`PendingApproval`) | Add `RiskLevel` field |
| `server/services/approval_store.go:49-62` (`PersistedApproval`) | Add `RiskLevel` field |
| `server/services/approval_store.go:146-165` (`GetApprovalMetadataBySession`) | Copy `RiskLevel` into `session.ApprovalMetadata` |
| `server/services/approval_store.go:307-323` (`persistToDiskLocked`) | Copy `RiskLevel` into `PersistedApproval{}` |
| `server/services/approval_store.go:385-399` (`loadFromDisk`) | Copy `RiskLevel` back into `PendingApproval{}` |
| `session/review_queue_poller.go:55-68` (`ApprovalMetadata`) | Add `RiskLevel` field |
| `session/review_queue_poller.go:840-860` (enrichment block) | Set `item.Metadata["risk_level"]` from `a.RiskLevel` |
| `proto/session/v1/types.proto:1034-1061` (`PendingApprovalProto`) | Add `string risk_level = 10;` |
| `proto/session/v1/types.proto:1110-1140` (`AnalyticsSummaryProto`) | Add `map<string, int32> risk_level_counts = 18;` |
| `make proto-gen` | Regenerate Go + TS bindings |
| `server/services/approval_service.go:174-185` (`ListPendingApprovals` builder) | Set `RiskLevel: riskLevelString(a.RiskLevel)` on the proto |
| `server/services/analytics_store.go:88-118` (`AnalyticsSummary`) | Add `RiskLevelCounts map[string]int` |
| `server/services/analytics_store.go:321-458` (`ComputeSummary`) | Aggregate risk-level counts in the existing loop |
| `server/services/rules_service.go:518-584` (`summaryToProto`) | Copy `RiskLevelCounts` into the proto map |
| `web-app/src/lib/api/approvalsApi.ts:13-22` (`PlainApproval`) | Add `riskLevel: string` |
| `web-app/src/components/sessions/ApprovalCard.tsx` | Render severity badge |
| `web-app/src/components/sessions/ApprovalDrawer.tsx` | Optional: sort/group by severity (mirrors its existing sort at line 64) |
| `web-app/src/components/sessions/ReviewQueuePanel.tsx` | Badge from `metadata["risk_level"]`, new sort option, new filter dimension (analogous to `programFilter`) |
| Analytics UI (wherever `AnalyticsSummaryProto.escalation_reason_counts` is currently rendered) | Add a parallel severity breakdown chart/list |

This list is the plan phase's task breakdown; nothing here requires new architectural
concepts beyond "one more field, threaded through five structs and two proto messages,
following patterns that already exist three times over in this codebase."
