# Architecture Review: vscode-extension
**Date**: 2026-08-06
**Verdict**: CONCERNS

## Constitution Violations

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (checked directly: `test -f docs/adr/ADR-000-architecture-constitution.md` → missing). No constitution constraints to apply; this section is empty by inspection, not by omission.

## Blockers

None open. Re-review of the 3 prior blockers:

- **Epic 3.3 / Story 3.3.1 — tree view never registered**: RESOLVED. Task 3.3.1e ("Register the tree view in extension.ts and wire disposal") now calls `vscode.window.createTreeView("staplerSquadSessions", { treeDataProvider: sessionTreeDataProvider })` in `activate()` and pushes the returned `treeView` into `context.subscriptions`, alongside `sessionTreeDataProvider`.
- **Epic 3.3 / `SessionTreeDataProvider`'s `EventEmitter` never disposed**: RESOLVED. The same Task 3.3.1e has `SessionTreeDataProvider` implement `vscode.Disposable` with a `dispose()` that disposes its internal `EventEmitter` (created in Task 3.3.1b), and pushes the provider itself into `context.subscriptions`.
- **AC-2 disconnected-status-bar branch / AC-5 approval-promotion ordering — zero unit-test coverage path**: RESOLVED. Task 2.1.2d extracts `formatStatusBarState(counts, connectionState)` into `viewModels/statusBarFormat.ts` — pure, no `vscode` import — consumed by `StatusBarController.refresh()`; Task 2.1.2e adds its Jest test. Task 4.2.1d extracts `buildTreeRows(sessions, approvals)` into `viewModels/buildTreeRows.ts` — pure, ordering-only — consumed by `SessionTreeDataProvider.getChildren()`; Task 4.2.1e adds its Jest test. Both modules also appear in Phase 6's AC-10 test inventory (Story 6.1.1).

## Concerns

- [ ] **`extension.ts` as an accreting God-function/central coordinator** — Epics 2.1, 3.3, 4.2, and 4.3 each wire more work into the same shared `tickFn`/`activate()`: by Phase 4 the single tick fetches three RPCs (`ListSessions`, `GetReviewQueue`, `ListPendingApprovals`), computes status counts, refreshes the status bar, refreshes the tree, and runs notification de-dup — all inline in one file with no closed extension point. Every new epic requires editing the same function (an Open/Closed violation), and Phase 8's `autoOpenWorktree`/streaming fast-follows would extend it further. The review brief flagged this exact risk explicitly and it is real, not just a hypothetical. — **Recommendation**: introduce an explicit `PollOrchestrator` (or `refreshAll(client, listeners)`) module that owns the tick's RPC fan-out and dispatches results to registered listeners (status bar, tree provider, notifier). `extension.ts`'s `activate()` then stays wiring-only — constructing collaborators and registering them with the orchestrator — and each feature epic touches only its own listener, not a shared inline function.

- [ ] **Race window between optimistic approve/deny and in-flight poll reconciliation (Task 4.2.2b)** — If a poll tick's `ListPendingApprovals` call was already in flight (issued before the mutation) when the user clicks Approve/Deny, that tick's stale response can land *after* the optimistic removal and overwrite it, making the just-approved/denied item flicker back into the tree until the *next* tick self-heals it. The plan's reconciliation model ("next refresh always replaces state wholesale") handles the failure-revert case correctly but not this ordering race on the success path. — **Recommendation**: tag poll responses with a monotonic tick counter; when an optimistic mutation is applied, record its generation and ignore any in-flight poll response whose request predates it, or simply await/cancel any in-flight tick before applying reconciliation after a mutation.

- [ ] **`ConnectionState`'s `"connecting"` literal is declared but never produced (Domain Glossary)** — The type is `"connected" | "connecting" | "disconnected"`, but Task 2.1.2c's `tickFn` only ever transitions to `"connected"` (success) or `"disconnected"` (failure); no task sets `"connecting"`. This leaves an unreachable state in the union (any exhaustive consumer still has to account for it) and leaves unspecified what the status bar renders in the real gap between `activate()` and the first tick's resolution — the legitimate use case `"connecting"` seems to exist for. — **Recommendation**: either drop `"connecting"` from the type so it matches actual behavior (2-state union), or have `StatusBarController`'s constructor set `"connecting"` synchronously before `PollScheduler.start()`'s first tick resolves, and document that transition in Task 2.1.2a.

- [ ] **Undocumented signature evolution of `SessionTreeDataProvider.refresh()`** — Task 3.3.1b defines `refresh(sessions)` (one argument). Task 4.2.2b's reconciliation description calls `refresh(sessions, approvals)` (two arguments) without any task explicitly extending the method signature in between. An implementer following tasks in order would hit an inconsistency between Phase 3 and Phase 4. — **Recommendation**: either define `refresh(sessions, approvals)` from Task 3.3.1b onward (with `approvals` defaulting to `[]` until Epic 4.2 populates it), or add an explicit sub-task under Epic 4.2 that extends the signature.

## Nitpicks

- `config.ts` mixes pure config-reading (`getConfig`) with a stateful one-time side effect (Story 5.2.2's non-localhost warning, which needs `vscodeAdapter.showInformationMessage` plus its own "already warned" flag). A separate `configWarnings.ts` would keep `config.ts` a pure reader.
- `VsCodeAdapter`'s pass-through functions are legitimately one-line forwards here (a testability seam, not a GoF pattern) — worth a one-line code comment explaining why, so a future reviewer doesn't mistake it for the forwarding-wrapper smell in `.claude/rules/interface-pollution-checklist.md` without that context.
- The plan never states whether the tick's three RPC calls (`ListSessions`, `GetReviewQueue`, `ListPendingApprovals`) run concurrently (`Promise.all`) or sequentially; sequential awaits would needlessly stretch tick latency and widen the race window noted in Concerns above.
