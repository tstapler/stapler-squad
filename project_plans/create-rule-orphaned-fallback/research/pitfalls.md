# Pitfalls: create-rule-orphaned-fallback

All findings below are VERIFIED against `origin/main` (PR #315 merged) via
`git show origin/main:<path>` / `git grep origin/main` — the local `main`
worktree is stale (32 ahead / 26 behind) and does not contain this code.

## 1. Frontend-only fix is sufficient — exactly one gating call site

`git grep -n "escalation_reason_category" origin/main` across all non-doc,
non-test source finds only two production reads, both in
[`web-app/src/components/sessions/ReviewQueuePanel.tsx`](https://github.com/tstapler/stapler-squad/blob/origin/main/web-app/src/components/sessions/ReviewQueuePanel.tsx):

- **Line 751** — emoji lookup for the reason text:
  `` ESCALATION_REASON_EMOJI[queueItem.metadata["escalation_reason_category"] ?? ""] ?? "" ``.
  Already null-safe (`?? ""` twice) and, more importantly, this whole block is
  gated on `queueItem.metadata["escalation_reason"]` being truthy — for an
  orphaned approval that field is *also* absent, so this branch already falls
  through to `"Reason not recorded — this request predates escalation-reason
  tracking."` (confirmed by the existing Jest test `"shows the
  orphaned-approval fallback copy when escalation_reason is absent"`,
  `ReviewQueuePanel.test.tsx:987`). **No change needed here.**
- **Line 845** — the Create Rule gate: `queueItem.metadata?.["escalation_reason_category"]
  === "no-match"`. **This is the only site that needs to change.**

No Go backend code gates *rendering* on this value — `session/review_queue_poller.go:854-855`
only decides whether to *set* the metadata key at all, and analytics
(`server/services/analytics_store.go`) computes its own category independently
(see §2). So a frontend-only fix (loosen `===` to a denylist, see requirements)
fully addresses the reported bug without touching Go. This directly answers the
first research question: **yes, frontend-only is sufficient** — there is no
second call site anywhere else in the codebase that needs the same treatment.

## 2. Server-side default would NOT pollute analytics — but would poison disk state if chosen anyway

Checked whether defaulting `PendingApproval.EscalationCategory` to `"no-match"`
server-side (e.g. in `ApprovalStore.loadFromDisk`,
[`approval_store.go:379-401`](https://github.com/tstapler/stapler-squad/blob/origin/main/server/services/approval_store.go#L379-L401))
would double-count in the "Escalation Reasons" analytics table
(`ApprovalAnalyticsPanel.tsx`, backed by `AnalyticsSummary.EscalationReasonCounts`).

**It would not.** `EscalationReasonCounts` is built at query time in
`analytics_store.go:414-455` via `classifier.CategorizeEscalationRuleID(e.RuleID)`
applied fresh to each historical `AnalyticsEntry.RuleID` — a completely
separate write/read path (SQLite-backed `AnalyticsEntry`, populated once at
decision time in `RecordFromResult`) that never reads
`PendingApproval.EscalationCategory` or `ApprovalMetadata.EscalationCategory`
at all. `GetApprovalMetadataBySession` (`approval_store.go:146`) has exactly
one production caller, `session/review_queue_poller.go:828/830` — so a
server-side default's blast radius is provably limited to the review-queue
metadata path, not analytics.

That said, a server-side default is still riskier than it looks, for a
different reason than analytics: `PersistedApproval.EscalationCategory` uses
`json:"escalation_category,omitempty"` (`approval_store.go:60`). If
`loadFromDisk` mutates the in-memory value from `""` to `"no-match"`, the next
`persistToDiskLocked()` write will serialize that defaulted value back to
`pending_approvals.json` as if it were a real classification — permanently
erasing the distinction between "actually classified no-match" and "we don't
know, this predates the field." This is a one-way data-provenance loss with no
compensating benefit (the frontend-only fix achieves the same UX outcome
without ever touching disk state), so it's a reason to prefer the frontend-only
approach even though the analytics-pollution risk itself is ruled out.

## 3. Denylist inverts the current allowlist — permissive-by-default is a real forward-compat gap

Current logic is an **allowlist**: show only if `=== "no-match"`. The proposed
fix is a **denylist**: hide only if the category is explicitly one of
`explicit-rule`, `domain-age`, `secret-scan`, `unclassifiable`, `unexpected`
(the 5 non-no-match constants in
[`pkg/classifier/escalation.go`](https://github.com/tstapler/stapler-squad/blob/origin/main/pkg/classifier/escalation.go#L9-L29)),
otherwise show.

This is a real risk, not just a style nit: `escalation_reason_category` is a
plain `string` at every hop from `classifier.EscalationCategory` (Go newtype)
down to `ReviewItem.Metadata["escalation_reason_category"]`
(`map[string]string`) to the TypeScript `Record<string, string>` metadata prop
— there is **no enum/union type anywhere in the chain**, so adding a 7th
`EscalationCategory` constant to `escalation.go` in the future will not
produce a compiler error, a TypeScript exhaustiveness error, or any other
forced touchpoint in `ReviewQueuePanel.tsx`. Under a denylist, that new
category silently defaults to "show Create Rule" unless someone remembers to
add it to the frontend's hide-list by hand.

Notably, this codebase has already been burned by exactly this class of bug
once in this same feature: `CategorizeEscalationRuleID`'s own doc comment
(`escalation.go:55-58`) says its fallback exists specifically "to guard
against a `ComputeDailyBuckets`-style missing-default bug," and
`EscalationUnexpected` was added as a pre-mortem P3 fix for the same reason.
Recommend the implementation add either (a) a Jest test that asserts the
frontend's hide-list contains exactly the 5 non-no-match category strings (so
a new backend constant makes an existing test fail rather than silently
passing), or (b) a code comment in `ReviewQueuePanel.tsx` pointing back at
`escalation.go`'s const block so an editor of one is prompted to check the
other. This is the same un-enforced two-file sync problem that already exists
for `ApprovalAnalyticsPanel.tsx`'s `ESCALATION_CATEGORY_LABELS` map — not a
new pattern, but worth closing here rather than adding a third instance.

## 4. No race/flakiness risk expected — but scope any server-side change away from `Create()`

`server/services/approval_service_test.go:479-519`
(`TestApprovalStore_Create_ConcurrentEscalations_NoDataRace`) and
`session/review_queue_poller_test.go:968/1019` both exercise `EscalationCategory`
only with **explicit, non-empty** values (`fmt.Sprintf("category-%d", i)`,
`"no-match"`) — no test currently exercises the empty/missing path through
`Create()`, so a purely frontend-only fix touches zero Go code and introduces
no race risk at all.

If a server-side default were chosen anyway (not recommended per §2), it
should be scoped to `loadFromDisk` only (parallel to the existing `Orphaned:
true` force-set at `approval_store.go:398`), never to `Create()` — `Create()`
is the concurrency-tested hot path, and adding an `if EscalationCategory == ""`
branch there risks entangling with
`TestApprovalStore_Create_ConcurrentEscalations_NoDataRace`'s assumption that
each goroutine's distinct value survives untouched. `loadFromDisk` is
single-threaded (runs once at `NewApprovalStore` construction, before any
concurrent access begins), so it carries no such risk.

## 5. Local worktree divergence is an implementation-routing hazard, not a design hazard

`main` in this worktree is 32 commits ahead / 26 behind `origin/main` — PR #315
(escalation-reasoning) exists only on `origin/main`. `ReviewQueuePanel.tsx`
line 845, `review_queue_poller.go` line 854-855, and every other file this fix
touches do not exist in their current form on local `main`. Whoever runs
`sdd:5-implement` (or otherwise branches to write the fix) must branch from
`origin/main` — e.g. a fresh worktree via `git worktree add <path> -b
fix/create-rule-orphaned-fallback origin/main` — not from local `main`.
Branching from local `main` would either fail to find the code being patched,
or reintroduce the 26 origin-side commits local is missing as spurious diff
noise when the branch is eventually pushed/rebased against `origin/main`. This
is purely a routing/branching instruction for the next phase, not a design
concern — flag it in the implementation plan's setup step.

## Summary of recommendation

The evidence in §1-§4 supports the requirements doc's implied preference:
**frontend-only fix**, denylist implemented as a single `const` array/set in
`ReviewQueuePanel.tsx` co-located with (and ideally cross-referencing)
`pkg/classifier/escalation.go`'s constant list, plus one regression test
extending the existing `"shows the orphaned-approval fallback copy…"` fixture
to also assert `create-rule-<id>` **is** present (closing the gap that today's
test at `ReviewQueuePanel.test.tsx:987` exercises the exact orphaned scenario
but never asserts button visibility). No backend change is needed, and none is
recommended given the disk-provenance-poisoning risk in §2.
