# Adversarial Review: insights-session-visibility

**Date**: 2026-09-12
**Verdict**: CONCERNS
**Review round**: 2 (re-review after patch pass)

6 of 7 round-1 items are cleanly resolved. The 7th (role "filterable" vs. "sortable")
was resolved on its own narrow terms but the patch pass's *separate* fix for the
scope-creep concern (deferring the role-filter dropdown entirely) reopens the same
Success Metric contradiction from a different angle — the plan no longer builds any
mechanism to filter by role at all, while requirements.md's amended metric still says
"filtering is the mechanism that serves the stated goal." No BLOCKER remains.

## Blockers

(none)

## Concerns

- [ ] **requirements.md's amended Success Metric still requires role to be filterable, but the plan defers the only role-filtering mechanism, and neither document reconciles this.**
  requirements.md:79-83 was amended to read: "A triage/review session's role is a
  first-class, **filterable** field on `SessionTokenSummary` itself... (Sorting by role
  is not required — **filtering is the mechanism that serves the stated goal**.)" That
  amendment resolves the original "filterable but not sortable" contradiction on its own
  terms — but the patch pass's separate fix for the scope-creep concern (round 1) moved
  the *only* thing that would let a user filter by role — the role-filter dropdown — to
  `plan.md`'s "Deferred / Follow-up ideas" section, out of PR2's scope entirely. Nothing
  in PR2 adds `sessionRole` to the Fuse.js search index either (Task 2.2.1a only adds
  `tags`, not `sessionRole`, to `FuseDoc`/`fuse.keys`). So after this plan ships, no UI
  or search mechanism lets a user filter the session list by role — only the tag chips
  and text search exist. Read charitably, "filterable" in the amended metric could mean
  "a real proto field a *future* filter could act on" (consistent with Scope §3, which
  only ever asked for the role *badge* source to change) rather than "a filter ships in
  this PR" — but nothing in the plan or requirements doc states that reading explicitly,
  so a future reader checking the plan against its own Success Metrics will look for a
  role filter, not find one, and have to reverse-engineer why.
  **Recommendation**: add one sentence — either to requirements.md's Success Metric
  parenthetical or to plan.md's Deferred section — stating explicitly that "filterable"
  is satisfied by the field's existence on the wire type (queryable by any future
  consumer), not by a UI filter shipping in this PR, and that the UI filter is the
  Deferred item. This is a documentation reconciliation, not a code change.

## Minors

(none beyond what round 1 already logged as clean/no-action — see below)

## Round 1 → Round 2 resolution status

- **BLOCKER — `make registry-generate` never run/committed** — RESOLVED. Task 1.4c
  (Epic 1.4, backend validation) and Task 2.5d (Epic 2.5, frontend validation) both add
  an explicit "run `make registry-generate`, review the diff, commit changed files"
  step, each with a one-line rationale tying it to CI's hard-fail-on-diff behavior and
  to the specific new test functions / new components that would otherwise cause a
  diff. Additionally, Tasks 2.1.2a and 2.1.3a now instruct adding a
  `// +feature: insights-dashboard` marker to the two new chart components
  (`TokenBreakdownBar.tsx`, `TokenBreakdownChart.tsx`), matching the existing
  `ModelBreakdownChart.tsx`/`DailySpendChart.tsx` convention the original review flagged
  as a second, independent registry-diff risk. Cross-references between tasks
  (`Task 2.5b`/`Task 2.5d` mentioned inline elsewhere in the doc) all resolve to the
  correct, current task letters — no stale numbering introduced by the insertion.

- **CONCERN — `AttachSessionToItem` multi-item role misattribution undocumented** —
  RESOLVED. ADR-029's Consequences section now has a dedicated bullet, "Known
  limitation — one session UUID legitimately attached to two different backlog items,"
  citing `AttachSessionToItem` (`server/services/backlog_service_sync.go:103-109`) and
  explicitly stating the most-recent-`created_at`-wins lookup will surface the second
  item's role even when inspecting data generated during the first item's attachment,
  framed as a known, accepted, non-regression limitation.

- **CONCERN — doubled full-table-scan-per-page-load + per-event `watchInsights` scan not named as an accepted inefficiency** —
  RESOLVED. ADR-029 gained a new "Known inefficiencies" section stating plainly that the
  scan now runs twice per page load (once via `GetSessionBacklogIndex`, once via the new
  `InsightsService` role lookup) and once per `WatchInsights` streamed event, with
  explicit reasoning (personal single-user tool, small row counts, the project's own
  2-concurrent-session WIP-limit convention bounding realistic event frequency) and a
  named escape hatch (short-TTL cache or request-scoped memoization) if it's ever
  measured to matter. This directly answers round 1's "unquantified" critique of the
  `watchInsights` frequency concern by citing the WIP-limit convention as the
  quantification argument the ADR previously lacked.

- **CONCERN — role filter dropdown is scope creep without explicit sign-off** —
  RESOLVED as stated, but see the new Concern above: deferring it (rather than shipping
  it or explicitly asking the user) is a legitimate way to resolve "this wasn't asked
  for," but it reopens the separate "filterable" Success Metric question that round 1's
  *other* concern (below) was resolved against. The "Deferred / Follow-up ideas" section
  correctly states the dropdown "reuses the existing `modelFilter` dropdown pattern...
  deferred pending explicit user confirmation, per 'do what has been asked, nothing more,
  nothing less.'" No dangling reference to the dropdown remains anywhere else in the
  plan — Epic 2.3's goal, its dependency-diagram entry (`Epic 2.3 role display`), and its
  sole story (2.3.1) all describe only the badge-source change, with no stray "Story
  2.3.2" or filter-dropdown task left behind.

- **CONCERN — "filterable but not sortable" contradicts requirements.md's Success Metric** —
  PARTIALLY RESOLVED / NEW ISSUE INTRODUCED. requirements.md was independently amended
  to drop the "sortable" claim and add "(Sorting by role is not required — filtering is
  the mechanism that serves the stated goal.)" — this cleanly resolves the original
  literal contradiction (sortable was claimed but never built). But combined with the
  *other* fix above (deferring the filter dropdown out of scope), the amended metric's
  own replacement clause — "filtering is the mechanism that serves the stated goal" — is
  now itself unmet by the plan as written. See the Concern entry above for the specific
  recommendation (one clarifying sentence, no code change).

- **CONCERN — Cache ROI/Waste Score tooltip jscpd risk deferred to "check after the fact"** —
  RESOLVED. Task 2.1.0a ("Record the jscpd baseline before adding new files") is now the
  first task under Epic 2.1, run before any new PR2 component/test files are added,
  giving an up-front headroom measurement rather than only checking at Epic 2.5's final
  gate (Task 2.5b, which still exists and remains the actual enforcement point — Task
  2.1.0a is explicitly scoped as "just an early heads-up measurement," consistent with
  the original recommendation).

- **MINOR — misleading Task 1.3.1b grep instruction** — RESOLVED. The task no longer
  tells the reader to locate an existing test via `grep -rl`; it now states plainly "No
  existing test covers `GetAllItemSessionsWithBacklogInfo` — add the new test to
  `session/ent_repository_backlog_test.go`... not by extending an existing one,"
  removing the misleading implication a matching test file already exists.

- **Previously-verified-clean items re-confirmed intact, undisturbed by the patch**:
  - Import-direction claim for `insightsBacklogReader` (Task 1.3.2a: `server/services`
    already imports `session`, no cycle risk) — present, unchanged.
  - `Tooltip`'s `asChild` no-extra-DOM-node design (Task 2.4.1b) — present, unchanged.
  - e2e column-count safety note (Task 2.1.2d: "update... column-count/header
    assertions (if any)") — present, unchanged.
  - PR-split recommendation (2 PRs, stacked, PR2 branches from `main` not PR1) —
    present, unchanged.
  - Recharts-vs-inline-SVG split (per-row bar = plain SVG/CSS for virtualized-table
    perf; detail view = Recharts stacked `BarChart`) — present, unchanged.
  - Tag filter OR semantics, stated inline at the filter-predicate call site (Task
    2.2.2b) — present, unchanged.
  - `.Order(ent.Desc(itemsession.FieldCreatedAt))` collateral fix (Story 1.3.1 /
    Task 1.3.1a) — present, unchanged.
  - Cache ROI header tooltip (Story 2.4.2) — present, unchanged.
