# Plan
1. Locate dispatch site for ship/work sessions; add pure `isNoopLoop(count, threshold, hasPass, headUnchanged)` in session/stuck_decisions.go + tests.
2. Add ent field `noop_dispatch_count` + `last_dispatch_head` (edit schema only, run generate with --feature sql/upsert); record head SHA (go-git, not subprocess) at session exit; increment if unchanged, reset otherwise.
3. Add `StuckReasonRepeatedNoopDispatch`; reconciler modeled on reconcileBouncingItems; selfHealStuck case; notification; WARN at 3rd retry.
4. Dispatcher gate: skip items with open row; log reason.
5. Derive `duplicate_pending` from `duplicate_ref=` marker; add to proto + `make proto-gen`; registry-generate.
6. Improve report_duplicate idempotent message (pending confirmation + how to resolve).
7. Frontend: badge on backlog item + /unfinished row for new reason and duplicate_pending (pnpm).
8. Tests (Go table tests, jest badge, e2e header conventions) and docs update to stuck-item doc.
## Adversarial review
- Risk: gate could block legitimate rework → count resets on new HEAD/verdict change; operator "clear" action needed.
- Risk: counting failures of the identity bug as no-op → count only sessions that ended with unchanged HEAD, regardless of cause; that's desired here.
- Risk: new StuckReason missing from selfHeal switch → explicit test enumerating all reasons.
- Risk: dupl/gocyclo gate on reconciler copy of bouncing code → extract shared helper.
