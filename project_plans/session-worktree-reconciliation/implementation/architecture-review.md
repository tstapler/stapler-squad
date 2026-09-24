# Architecture Review: session-worktree-reconciliation
**Date**: 2026-09-23 (iteration 2)
**Verdict**: CONCERNS

## Scope of this pass

Targeted re-review of the two iteration-1 BLOCKERS and their two adjacent CONCERNS, against the current `project_plans/session-worktree-reconciliation/implementation/plan.md` and the real code. The full 13-point lens checklist was not re-run.

## Blockers

None. Both iteration-1 blockers are resolved by the current plan.

- **Blocker 1 (`Notifier.Notify`'s `itemID` misuse for session-only flags) — RESOLVED.** The plan now adds `Notifier.NotifySession(sessionID, title, message string, notificationType int32, urgent, important bool)` as a sibling method (Domain Glossary row, Task 1.2.3a-prep, Files list for Story 1.2.3) and routes the no-linked-item case through it instead of `Notify`. Verified this is coherent and buildable against the real code:
  - `session/backlog_lifecycle.go:39-41`'s `Notifier` interface currently has only `Notify` — adding a sibling method is a 1-line, additive interface change, not a breaking one.
  - `server/services/backlog_notifier.go`'s real `EventBusNotifier.Notify` confirms the bug's mechanism: `events.NewNotificationEvent(itemID, "", ..., map[string]string{"item_id": itemID})` — unconditionally stuffs `itemID` into both the event's `SessionID` slot and `metadata["item_id"]`. The plan's proposed `NotifySession` body (`events.NewNotificationEvent(sessionID, "", ..., map[string]string{})`) is a straightforward parallel construction — same event constructor (`pkg/events/types.go:339`, re-exported via `server/events/forward.go`), just the real session ID in the `SessionID` slot and an empty metadata map. No structural obstacle.
  - Verified the UI consumer still behaves correctly under this design: `web-app/src/components/ui/NotificationItem.tsx:311-327` renders the backlog-item link when `metadata["item_id"]` is present, and falls back to a `sessionId`-based "View Session" link when it's absent — exactly the branch `NotifySession`'s empty-metadata event will hit.
  - `session/backlog_lifecycle_test.go`'s `fakeNotifier` (~line 2426-2431) currently implements only `Notify`; the plan's Task 1.2.3a-prep correctly identifies it needs `NotifySession` added to keep compiling.
  - Verdict: sound and buildable.

- **Blocker 2 (no `StuckReason` value defined for `MarkStuck`) — RESOLVED.** The plan adds `StuckReasonWorktreeInconsistent` (Task 1.2.3b-prep) to `session/domain/backlog.go`, appends it to `AllStuckReasons`, and bumps the exhaustive-count test from 20 to 21. Verified against real code: `session/domain/backlog.go`'s `AllStuckReasons` currently has exactly 20 entries, and `session/domain/backlog_test.go:81-84`'s `TestAllStuckReasons_should_contain20Entries_When_Enumerated` currently asserts `len(AllStuckReasons) != 20`. The plan's described diff (add the const, add it to the slice, bump the literal to 21) is the complete, correct fix — nothing else references the count. The Tech Debt Disposition table now also explicitly discloses this second touched file, closing the "silent about touching a second file" gap iteration 1 flagged.

## Concerns

- [ ] **ADR-001 precedent-layering (carried forward from iteration 1, not addressed by this fix — same root cause as former Blocker 1, now downgraded).** `notifyIfActiveWorkSessionStale` (`server/services/backlog_service_triage.go`) is a `server/services`-package method always called with a real `itemID`; the new sweep is a `session`-package function per ADR-001 that must handle the item-optional case that precedent never did. The plan still describes `NotifySession`/`resolveFinding` as "mirroring" that dual-write pattern (Domain Glossary row for `FlagSessionForOperator`, Story 1.2.3's AC). That's a reasonable design choice on its own, but it's re-deriving the shape, not reusing code — worth the plan saying so explicitly rather than implying more reuse than exists. Low severity; doesn't block implementation.

- [ ] **`ListInstanceDataWithWorktree(ctx)` compile-error — RESOLVED, verified.** Grepped the current plan.md for every occurrence of `ListInstanceDataWithWorktree`: all three sites (Domain Glossary, dependency-visualization diagram, Task 1.1.2b) now call it with zero arguments, and Task 1.1.2b explicitly documents the real no-`ctx` signature (`session/storage.go:438`, confirmed by direct read) and its `context.Background()`-internal caveat. No `(ctx)` call remains anywhere.

- [ ] **Story 1.2.3 AC ambiguity for the no-linked-item case — RESOLVED, verified.** The current AC (plan.md line ~248) now explicitly states: "*given* no linked item, *then* `MarkStuck` is not called at all (the notify call above still fires via `NotifySession`)" — the exact ambiguity iteration 1 flagged is now spelled out as an AC, not left to inference.

## Nitpicks

- **`LiveWorktreeEntry` alias still unused (carried forward, unaddressed).** The Domain Glossary still defines `LiveWorktreeEntry` as an alias for `git.NativeWorktreeEntry`, but Task 1.2.1a's `matchLiveWorktree` signature still uses `git.NativeWorktreeEntry` directly — the alias is never referenced by any task. Cosmetic; either use the alias in task text or drop the glossary row.
- **Filename placeholders — RESOLVED.** Task 1.3.2b now cites `server/dependencies_test.go` (confirmed real, 21828 bytes, covers the periodic-ticker wiring block) and Story 2.1.1 cites `server/services/backlog_service_triage_test.go` (confirmed real, existing `cleanupItemWorktreesExcept` call sites at lines 2793/2829/2873/2915 — grep matches the plan's cited line numbers exactly). Both citations are accurate; the prior hedge is gone.
- **Bonus finding verified: Epic B's real signature and `is.BacklogItemID` usage.** Read `server/services/backlog_service.go:1233` directly: `func (s *BacklogService) cleanupItemWorktreesExcept(ctx context.Context, sessions []session.ItemSessionSummary, exceptPath string)` — matches the plan's Story 2.1.1 text exactly (not `(ctx, item, keepSessionUUID)` as an earlier draft apparently had it). `session.ItemSessionSummary` (`session/repository.go:193-196`) does carry both `BacklogItemID` and `SessionUUID` fields, so the plan's corrected use of `is.BacklogItemID` for the `Notify` call's `itemID` slot (never `is.SessionUUID`) is valid and available on the struct already in scope at that call site.
