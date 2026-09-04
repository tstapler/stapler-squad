# Research: Feature Landscape for Review-Queue Severity

Companion to `project_plans/review-queue-severity/requirements.md`. Covers: existing
severity/priority concepts, edge cases in the escalation paths, unstated user needs, and
industry-practice conventions for triage-queue severity.

## 1. Correction to requirements.md: the target UI is not `ReviewQueuePanel.tsx`

Requirements.md states the approval queue is "surfaced in `ReviewQueuePanel.tsx`, backed by
`PendingApproval` / `ListPendingApprovals`." This is **not accurate** and should be corrected
before planning assigns AC3/AC4 to a file:

- `web-app/src/components/sessions/ReviewQueuePanel.tsx` renders `ReviewItem[]` (from
  `ListReviewQueue`), imports `Priority`/`AttentionReason` from `session/queue/queue.go`'s
  proto mirror, and already has a full `priority`/`age`/`diffSize`/`name` sort system
  (`SortField` type, line 121) and a rich multi-dimensional filter system
  (`FILTER_URL_KEYS`, line 124) — this is the **session-attention** queue (requirements.md's
  own item #8, the "distinct concept" queue), not the approval queue. It only touches
  `PendingApproval` indirectly: it calls `approve`/`deny` from `useApprovalsContext` (line 8,
  459) when a `ReviewItem` happens to carry `AttentionReason.APPROVAL_PENDING`.
- The actual `PendingApproval` list UI lives in **`ApprovalDrawer.tsx`** (list container,
  currently sorted by `secondsRemaining` ascending — "most urgent first" per an "MP3
  requirement" comment at line 17/63) and **`ApprovalCard.tsx`** (per-item renderer — grep
  confirms no `escalationReason`/`escalationCategory`/badge rendering there today at all).
  `ApprovalPanel.tsx` is a third consumer (session-scoped list, e.g. embedded in a session
  detail view).
- **Implication for planning**: AC3 (severity badge + default sort) and AC4 (severity filter)
  belong on `ApprovalCard.tsx` (badge) and `ApprovalDrawer.tsx`/`ApprovalPanel.tsx` (sort/filter
  container), not `ReviewQueuePanel.tsx`. `ReviewQueuePanel.tsx` may still want a *secondary*
  severity indicator on its own `APPROVAL_PENDING` items for consistency, but that's additive,
  not the primary surface.
- Existing precedent worth reusing: `ApprovalDrawer.tsx`'s current sort ("most urgent first" by
  expiry) already establishes that a *single* primary sort key is the norm here — see open
  question in requirements.md about severity as primary vs. tiebreaker under
  age/expiry-proximity. The existing code's own choice (urgency-of-expiry as the *only* sort
  key today) suggests severity should probably be primary with expiry as tiebreaker, or vice
  versa, but not fight for the same slot — needs an explicit decision in plan.md.

## 2. Other severity/priority concepts in the codebase

Beyond the two requirements.md already found (`pkg/classifier.RiskLevel`,
`session/queue.Priority`), a third, more directly relevant precedent exists:

- **`web-app/src/lib/hooks/useReviewQueueNotifications.ts`** — a 3-tier notification-urgency
  system keyed on `AttentionReason` (not risk), independent of `session/queue.Priority`:
  - Tier 1 (`APPROVAL_PENDING`, `INPUT_REQUIRED`, `WAITING_FOR_USER`) → persistent toast + OS
    notification + sound.
  - Tier 2 (`ERROR_STATE`, `TESTS_FAILING`, `STALE`) → brief auto-minimizing toast + OS
    notification only if tab hidden.
  - Tier 3 (everything else) → history panel only, no interrupt.
  - **Relevant gap**: every `APPROVAL_PENDING` item is Tier 1 today regardless of the
    underlying risk — a `RiskLow` test-file edit and a `RiskCritical` `rm -rf` currently fire
    the identical sound/toast/OS-notification treatment. This is the concrete, already-built
    hook point for "severity-based notification urgency" (see §3).
- **`web-app/src/lib/utils/notificationMapping.ts`**'s `priorityColor()` (line 94) — maps
  `urgent/high/medium/low` → `var(--color-error, #f44336)` / `--color-warning` / `--color-info`
  / `--color-success`. **Caveat**: `grep` of `web-app/src/app/globals.css` shows none of
  `--color-error`/`--color-warning`/`--color-info`/`--color-success` are actually defined
  there — this function relies entirely on its hardcoded hex fallbacks, which violates
  `.claude/rules/css-architecture.md`'s "token names only" rule (the defined equivalents are
  `--error`/`--error-bg`, `--warning`/`--warning-bg`, `--success`/`--success-bg`, plus no
  defined "info" token at all). **Do not copy this pattern** for the new severity badge; use
  the actually-defined tokens, and note this as pre-existing tech debt worth flagging
  separately (`.claude/rules/fix-collateral-debt` per user memory) rather than propagating it.
- **`session/queue/queue.go`**'s `Priority.Emoji()` (line 93-107) — 🔴/🟡/🔵/⚪ for
  Urgent/High/Medium/Low. Another existing visual-vocabulary precedent, emoji-based rather
  than CSS-color-based; not obviously reusable for a badge component but shows the codebase
  already has two independent severity→visual mappings (color-token-based and emoji-based) for
  the *other* priority concept — a third, inconsistent one for `RiskLevel` should be avoided.
- **`ApprovalRuleProto.risk_level`** (`proto/session/v1/types.proto:1084`) is threaded to the
  frontend (`web-app/src/gen/session/v1/types_pb.ts:2037`) and round-tripped through
  `ApprovalRulesPanel.tsx`'s `upsertRule` call (line 253: `riskLevel: rule.riskLevel`) — but
  **contrary to requirements.md's claim #2** ("the rules-management UI can already show/edit
  risk per rule"), there is no visible risk-level badge, select, or input anywhere in
  `ApprovalRulesPanel.tsx` — `riskLevel` is silently passed through on toggle/edit, never
  rendered or exposed as an editable field. So risk level is *wire-compatible* on the rule
  entity but not yet a real UI feature even there. Worth correcting that claim, and it means
  this project may be the first to actually build a `RiskLevel` badge component — there's no
  existing one to copy, only the color-token precedent above to follow.
- No severity/priority concept found in backlog items (`session/domain/backlog.go`,
  `session/backlog*.go`) — backlog items have no equivalent field; not a precedent to borrow
  from.
- No severity/priority concept found in the workflow system (`session/backlog_plugin*.go`,
  `mcp__stapler-squad__create_workflow` surface) — out of scope, confirmed nothing to reconcile
  there.

## 3. Edge cases in `server/services/approval_handler.go`

Traced every path that reaches the `createApproval:` label (line 384) in
`ServeHTTP`/`handlePermissionRequest`, to answer "what `RiskLevel` does each get":

| Path | Reaches `createApproval`? | `escalation.RiskLevel` |
|---|---|---|
| Secret scan auto-deny (line 223-251) | **No** — returns early with `writeDecision("deny", ...)`. Only recorded to analytics with `RiskCritical` for the analytics stream; never becomes a `PendingApproval`. | N/A — no severity threading needed |
| Domain-age check, newly-registered domain (line 258-293) | Yes, via `goto createApproval` (line 289) | `RiskHigh`, hardcoded (line 279) |
| `AskUserQuestion` (line 295-304) | **No** — returns early via `writeDeferDecision`, only fires an informational notification (`broadcastQuestionNotification`). Never creates a `PendingApproval`. | N/A |
| Classifier `AutoAllow` / `AutoDeny` (line 344-363) | **No** — both return early. | N/A |
| Classifier `Escalate` (line 364-366) | Yes | `result.RiskLevel`, whatever the matching rule/pattern set (rule-derived or pattern-derived, per requirements.md items 1-2) |
| Classifier returns an unrecognized/future decision value (line 367-381) | Yes — explicit fail-safe branch added specifically to avoid silently falling through with `escalation` unset | `result.RiskLevel` as returned by the classifier for that (anomalous) result — likely zero-value `RiskLow` in practice since no rule matched, but not guaranteed |
| `h.classifier == nil` (guard at line 307) | Yes — skips the whole classification `if` block and falls straight through to `createApproval` with `escalation` still at its `var escalation classifier.ClassificationResult` zero value (line 256) | **`RiskLow`** (since `RiskLow RiskLevel = iota` is the zero value) — this is the literal "zero-valued escalation" case requirements.md's research question asks about |
| Headless-pool autonomous LLM approval (line 386-410) | Only on fallthrough (LLM response unparseable or call failed) — does **not** independently compute a `RiskLevel`; it inherits whatever `escalation` already held from the paths above | Inherits upstream value — no new edge case, but confirms headless-pool fallthrough must not silently reset `RiskLevel` to zero when threading is added |

**Answer to requirements.md's explicit edge-case question**: `h.classifier == nil` is the only
path where `escalation` is genuinely zero-valued, and it silently maps to `RiskLow` (iota 0)
rather than "unknown/unset." In production this can't happen — `server/server.go:485` always
calls `approvalHandler.SetClassifier(deps.SessionService.GetClassifier())` during wiring — but
it's live in any test harness that constructs `ApprovalHandler` without calling
`SetClassifier`, and Go's zero-value semantics mean a future refactor that lets a genuinely-nil
classifier reach production would **silently mislabel unclassified escalations as low-risk**
rather than failing loudly. Recommend explicitly guarding this in plan.md: either give
`RiskLevel` an explicit `RiskUnknown` zero value (breaking change to the existing iota ordering
used throughout `pkg/classifier/classifier.go` — costly) or document/assert that this path is
unreachable in production and add a regression test pinning `h.classifier != nil` after
`server.go` wiring.

**Manual rule edits after a pending approval already exists**: `EscalationReason` /
`EscalationCategory` (and thus, once added, `RiskLevel`) are set once at `PendingApproval`
creation from the `escalation` classification result captured at that moment
(`approval_store.go` comment: "never re-derived after creation"). If a user edits or deletes
the matching `ApprovalRule` while the approval is still pending, the already-queued approval's
severity does **not** update — it's a point-in-time snapshot, consistent with how
`EscalationReason`/`EscalationCategory` already behave. This is very likely the desired
behavior (an approval card shouldn't change its risk label out from under a reviewer who's
mid-review), but plan.md should state it explicitly as a design decision rather than an
implicit consequence of the "capture once" pattern, since it's the kind of thing a reviewer
will ask about.

## 4. Unstated user needs beyond the explicit ACs

- **Severity-based notification urgency** (sound/toast intensity): concretely actionable today
  because `useReviewQueueNotifications.ts`'s tier system already exists — a `RiskCritical`
  `APPROVAL_PENDING` item could escalate beyond Tier 1's current single sound/toast treatment
  (e.g. a distinct/louder `NotificationSound`, or never auto-minimizing), while a `RiskLow`
  approval could arguably *not* need the same interrupt-level urgency it gets today. This
  wasn't in the ACs and would touch a different file (`useReviewQueueNotifications.ts`) than
  the ones requirements.md scoped — flag as a candidate follow-up rather than silently
  in-scope, per requirements.md's own "flag as follow-up if research disagrees" instruction.
- **Keyboard shortcut to jump to highest-severity item**: `useReviewQueueNavigation.ts` already
  implements arrow-key/keyboard navigation through `ReviewQueuePanel.tsx`'s `ReviewItem` list
  (line 111-137) — but that's the session-attention queue, not the approval queue.
  `ApprovalDrawer.tsx`/`ApprovalCard.tsx` have no keyboard navigation at all currently (no
  `keydown` listeners found in either file). A "jump to highest severity" shortcut would be new
  ground in the approval queue specifically, not an extension of an existing approval-queue
  keyboard system.
- **Bulk-approve-by-severity-tier**: no bulk-approve/bulk-deny capability exists for
  `PendingApproval` at all today (`ApprovalCard.tsx` / `useApprovals.ts` only expose
  single-item `approve`/`deny`) — this would be new capability, not a severity-aware extension
  of an existing bulk action. Worth flagging as a heavier ask than the AC list implies; likely
  out of scope for this pass given requirements.md's "additive metadata, not a behavior change"
  framing (AC7).
- **Severity as an escalation-category disambiguator**: `ReviewQueuePanel.tsx` already has an
  `EscalationCategory` type (`@/lib/sessions/escalationCategory`) used for grouping/filtering
  session-level items — once `RiskLevel` is threaded to `PendingApprovalProto`, there may be
  demand to cross-filter "show me all `RiskCritical` items regardless of category" vs. "show me
  all `secret-scan` category items regardless of risk" — the two dimensions are orthogonal and
  both already exist as separate fields on `PendingApproval` (`EscalationCategory` +, once
  added, `RiskLevel`), so this is naturally supported by AC4's filter control as long as it's
  designed as an independent facet, not folded into the existing category filter.

## 5. Industry practice for severity tiers in a triage queue

- **PagerDuty** (via web search, Aug 2026): uses P1–P5 (or Sev1–Sev5), lower number = higher
  impact. P1 = Critical/immediate (e.g. full outage), P2 = High, P3 = Medium, P4 = Low, P5 =
  Informational. Their own best-practice guidance explicitly warns "more than five
  classification levels makes triage more complex and time-consuming," and notes many orgs
  stop at 3 (P1-P3) rather than using all 5.
  [Severity Levels — PagerDuty Incident Response Documentation](https://response.pagerduty.com/before/severity_levels/),
  [Incident Severity Classification — PagerDuty](https://www.pagerduty.com/resources/incident-management-response/learn/incident-severity-classification/)
- **GitHub Dependabot alerts** (via web search, Aug 2026): exactly 4 tiers — **Critical / High
  / Moderate / Low**, derived from the advisory's CVSS score, sorted Critical→Low with
  color-coded badges. This is the closest external analog in tier *count* to this codebase's
  existing `RiskLevel` (`Low/Medium/High/Critical`) — same 4 levels, same names modulo
  Moderate-vs-Medium. Directly supports requirements.md's recommendation to keep the existing
  4-level `RiskLevel` rather than collapsing to 3-level P0/P1/P2: an already-shipped, widely
  recognized triage UI (Dependabot) uses the same 4-tier shape this codebase's classifier
  already outputs.
  [5 tips for prioritizing Dependabot alerts](https://github.blog/security/supply-chain-security/5-tips-for-prioritizing-dependabot-alerts/)
- **Sentry** (general knowledge, not independently re-verified this session): issue-level
  severity is typically `fatal / error / warning / info / debug` — a 5-tier scheme oriented
  around log-level semantics rather than triage urgency; less directly analogous to an
  approval-risk queue than Dependabot's CVSS-derived tiers.
- **Default sort behavior**: both PagerDuty (severity-first triage) and Dependabot
  (Critical→Low sort) default to highest-severity-first, matching AC3's "sorts the queue by
  severity by default (highest risk first)" — no external precedent found for defaulting to
  anything else in a security/risk triage context, which weakens the case for making severity
  a secondary tiebreaker under age/expiry (open question in requirements.md) rather than the
  primary key. If anything, the age/expiry tiebreaker is the outlier vs. industry norms, though
  it exists here for a real reason (approvals expire and block the calling session) that
  neither PagerDuty incidents nor Dependabot alerts share.
- **Gastown/Bernstein** (named in the original GitHub issue per requirements.md context):
  requirements.md's own scope section already concludes their Deacon/Mayor
  automatic-escalation-routing pattern has no equivalent concept in this codebase (no
  roles/assignees) and is correctly out of scope — no further research needed there; nothing
  found in this session contradicts that conclusion.

## Summary of research-driven adjustments to carry into plan.md

1. **File-target correction**: AC3/AC4 UI work belongs on `ApprovalCard.tsx` +
   `ApprovalDrawer.tsx`/`ApprovalPanel.tsx`, not `ReviewQueuePanel.tsx`.
2. **Zero-value edge case**: `h.classifier == nil` silently yields `RiskLow`, not "unknown" —
   decide whether to guard/test this or accept it as an unreachable-in-production, iota-zero
   default.
3. **Snapshot semantics**: confirm (don't just inherit implicitly) that `RiskLevel` is
   captured once at approval creation and does not track subsequent rule edits, matching
   `EscalationReason`/`EscalationCategory`'s existing behavior.
4. **Color tokens**: use `.claude/rules/css-architecture.md`'s actually-defined tokens
   (`--error`, `--warning`, `--success`, etc.) for the severity badge — do not copy
   `notificationMapping.ts`'s `priorityColor()` pattern, which references undefined
   `--color-*` tokens.
5. **4-tier over 3-tier**: industry precedent (Dependabot) plus requirements.md's own
   recommendation both favor keeping `RiskLevel`'s existing 4 levels rather than collapsing to
   P0/P1/P2 naming.
6. **Notification-tier follow-up**: flag severity-aware notification urgency
   (`useReviewQueueNotifications.ts`) as an explicit out-of-scope-for-now follow-up rather than
   silently omitting it, since it's genuinely low-effort given the existing tier hook.
