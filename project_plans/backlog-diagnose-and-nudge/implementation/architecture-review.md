# Architecture Review: backlog-diagnose-and-nudge
**Date**: 2026-09-27 (re-review after fix pass)
**Verdict**: CONCERNS

## Blockers

None. The original blocker is resolved — see "Blocker 1: resolved" below.

### Blocker 1 (previous review): resolved

Previous finding: `DiagnoseOutcomeKind`'s closed 5-value sum type could not represent
a durable "Diagnosing (in-flight)" state, and no task persisted an in-flight row, so
`ListDiagnoseDispatches` had nothing to return mid-dispatch.

The fix pass adds a genuinely separate lifecycle enum, `DiagnoseDispatchStatus{Pending,
Completed}` (Domain Glossary, plan.md:81), explicitly distinct from and orthogonal to
`DiagnoseOutcomeKind` (plan.md:80 now says so explicitly: *"Does **not** represent the
pre-completion 'Diagnosing…' state... a `Pending` row has no `OutcomeKind` yet"*), and
wires it through every layer with concrete tasks, not just prose:

- **Schema** (Story 5.2.1, plan.md:1090-1120): `DiagnoseDispatch` ent schema gets a
  `Status` field (`Pending`/`Completed`) alongside the nullable outcome fields.
  `DiagnoseDispatchStore` exposes `Record` (insert `Pending`, no outcome — Task
  5.2.1a/c) and `MarkCompleted` (update the *same row* to `Completed` + outcome —
  Task 5.2.1c), plus `ListByItem` returning chronological rows each carrying its own
  `Status`.
- **Write-at-dispatch-start** (Story 5.1.1, Task 5.1.1d, plan.md:1018-1027): explicitly
  persists the `Pending` row via `DiagnoseDispatchStore.Record` "immediately after the
  in-flight guard passes, before calling `dispatch`" — i.e. before the diagnostic
  session is even created, not after the fact. Task 5.1.1e adds a unit test that
  queries `ListByItem` for that row before the diagnostic session completes,
  specifically simulating a page-refresh race. This is the piece that was missing
  before: a concrete task writing a durable row at dispatch start, not just a plan
  that claims one exists.
- **Update-not-reinsert on completion** (Story 5.1.3, plan.md:1063-1078; Story 5.2.2,
  plan.md:1122-1150): both dispatch failure and normal outcome completion update the
  *same* `Pending` row via `MarkCompleted`, threading the `dispatchID` returned by
  Task 5.1.1d's `Record` call through so the correct row is targeted — the plan is
  explicit ("never a second, separate row") at both call sites.
- **RPC surface** (Story 7.1.1, plan.md:1248-1268): `ListDiagnoseDispatches` response
  carries a `status` field distinct from `outcome_kind`, with a GWT
  (plan.md:1264-1267) explicitly covering "dispatch still in flight... simulating a
  page refresh... returns the same `Pending` row unchanged."
- **Frontend** (Story 8.2.1, plan.md:1369-1380): the "Diagnosing…" state is explicitly
  specified to be sourced from `ListDiagnoseDispatches`' `status: Pending` field, not
  from the transient button-pending state Stories 8.1.1/8.1.2 own — stated as
  replacing that client-only state once a dispatch ID exists server-side, with a GWT
  covering a fresh page load with no button-click state in memory.

This is a complete, concretely-specified chain from schema through UI, each link
naming the exact task/file/test that closes it — not just a claim that the gap is
closed. Verdict: **genuinely resolved**, contingent on implementation following the
plan as written (which is all a plan review can certify).

## Concerns

- [ ] **New: `DiagnoseDispatchStatus` and `OutcomeKind` are independent, unconstrained
      fields — nothing in the plan prevents them from disagreeing (e.g. a `Completed`
      row with a nil `OutcomeKind`, or a `Pending` row with one set).** This is the
      question the fix pass's own type-driven-design rigor was applied to for
      `DiagnoseOutcomeKind` (Story 1.2.1's `Validate()`, Task 1.2.1c, rejects e.g.
      `Nudged` with a non-nil `GateReason`) but was **not** extended to the new
      `Status`/`OutcomeKind` pairing on the `DiagnoseDispatch` row itself. The ent
      schema (Task 5.2.1a, plan.md:1110) describes `OutcomeKind` as merely "nullable
      until completion" — a comment, not an enforced invariant — and the domain-level
      description (plan.md:93, plan.md:149-155) is prose ("created `Pending`... updated
      to `Completed`... never created only after the fact"), not a type or constraint
      that makes the illegal combination unrepresentable. In practice this is
      moderately well mitigated: `DiagnoseDispatchStore` exposes only two write
      methods, `Record` (always inserts `Pending` with no outcome — Task 5.2.1c) and
      `MarkCompleted` (always sets `Status: Completed` and the outcome fields
      atomically together — Task 5.2.1c, called from Story 5.1.3 and Story 5.2.2 only)
      — so as long as those two methods remain the only writers, the invariant holds
      by convention. But nothing stops a future third write path, a raw ent mutation,
      or a test fixture from constructing the split-brain state, and Task 5.2.1d's
      test list ("record-as-pending, mark-completed updates the same row, chronological
      list reflecting mixed pending/completed rows") does not include a case asserting
      the pairing is rejected when violated. Since `DiagnoseOutcomeDisplay` (Story
      8.2.1) branches on `status` to decide whether to read `outcome_kind` at all, a
      violation here would silently mis-render (e.g. a `Completed` row with no
      `OutcomeKind` would fall through Task 8.2.1a's switch with no matching case).
      **Recommendation**: extend Story 1.2.1's `Validate()` (or add a sibling on the
      `DiagnoseDispatch` domain type used by `MarkCompleted`/`ListByItem`) to reject
      `(Completed, OutcomeKind == nil)` and `(Pending, OutcomeKind != nil)`
      combinations — cheap now, before Phase 5/7 code exists to work around it.

- [x] **Resolved (previous Concern 1)**: the residual "future 4th write-capable tool
      could bypass the gate" caveat now appears directly in the Tech Debt Disposition
      table's own Justification column (plan.md:131 — *"This is the one item in this
      table that is genuinely fixed, not just isolated, for the three known call sites
      — but not eliminated structurally: per the Creative Pass (Approach A's stated
      weakness, above), a future fourth write-capable tool could still skip the gate;
      nothing but code review and the narrow lint ratchet/rules-file guardrail...
      backstops that residual case."*), not only in the Creative Pass section. A
      reader of the table alone now gets the accurate, non-overclaimed picture.

- [x] **Resolved (previous Concern 2)**: Task 3.2.1a (plan.md:652-656) now cites
      `server/services/session_service.go:1035-1057` for `isSafeSteerStatus`/
      `safeIdleStatusContexts`. Verified directly:
      `grep -n "isSafeSteerStatus\|safeIdleStatusContexts" server/services/session_service.go`
      shows `safeIdleStatusContexts` defined at line 1044 and `isSafeSteerStatus` at
      lines 1050-1057 (func body at 1056-1057) — matches the plan's citation exactly.
      The plan also now correctly explains the origin of the old wrong citation:
      `backlog_service_pr_fix_steer.go:180` is in fact `isClaudeCodeProgram` (verified:
      `sed -n '180p' server/services/backlog_service_pr_fix_steer.go` →
      `func isClaudeCodeProgram(program string) bool {`), a different helper the plan
      correctly cites elsewhere (Task 3.2.1c) — so the correction is accurate, not just
      reasserted.

## Nitpicks

- [x] **Resolved**: `DiagnoseNudgeConfig`'s four `OrDefault()` accessors now all have
      explicit hard-ceiling constants (Task 1.1.1a, plan.md:293-301): `MaxNudgesPerItem`
      →10, `CooldownSeconds`→86400, `IdleSettleWindowSeconds`→3600,
      `BundleTokenBudget`→500000 — previously only the first had one specified.
- `DiagnoseDispatcher` (Story 5.1.1) still ends up owning bundle assembly, session
  dispatch, `NudgeCapStore`, `DiagnoseDispatchStore`, and `notifyDiagnoseEvent` — a wide
  dependency surface for one orchestrator, modeled on an existing precedent
  (`reconcileOrphanedTriageItems`) of similar shape. Not a new violation; still worth
  watching as a God-Object candidate if later stories keep adding to it. Unchanged
  from the previous review — not addressed by the fix pass (out of scope for this
  targeted fix, reasonably).
- `NudgeGateCheck`'s `(bool, SafetyGateReason)` return shape (Domain Glossary,
  plan.md:78) is still a mild echo of the "boolean + reason" pattern the plan
  correctly rejects at the outcome level. Acceptable for an internal Chain-of-
  Responsibility link; unchanged from the previous review, not addressed by the fix
  pass, still low-priority.
