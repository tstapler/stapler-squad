# Research: Program-switching persistence and discoverability

Backlog item: `c35902a2-8027-4910-a8bd-2c6d0fd564fc` — "Switching session programs does not
save correctly and so it's unclear how to change running sessions."

This is pre-implementation research only. No source was modified.

> Note: an earlier version of this file (superseded by this pass) described the codebase as
> it was *before* a "Change Program" overflow-menu feature was added to
> `SessionActionsOverflow.tsx` / `SessionCard.tsx` / `SessionRow.tsx`. That UI now exists.
> This revision reflects the current `main` tree and narrows the remaining gaps
> accordingly — most of the "discoverability" problem described previously has already been
> partially addressed, but not everywhere, and the persistence problem has a more precise
> root cause than "the proto field gets dropped."

## Existing patterns to reuse

**Program switching for an existing session is not a "session creation mode."** It does not
need the 7-touchpoint registry in `.claude/rules/session-creation-registry.md` (proto enum,
`SessionType` constant, etc.) — that pattern governs *how a new session's worktree/directory
is provisioned* at creation time. Program is a mutable attribute of an already-created
session, so it belongs to the generic "update an existing session" pattern instead, which
already exists and already handles `program`:

- Proto: `UpdateSessionRequest.program` (`optional string program = 5`) —
  `proto/session/v1/session.proto:576-614`. This request also carries `status`, `category`,
  `title`, `tags`, `working_dir`, `rate_limit_enabled`, `autonomous_mode`, `steer_message` —
  program-switching should keep following this single generic-PATCH-style RPC rather than
  growing a dedicated `ChangeSessionProgram` RPC.
- Backend handler: `server/services/session_service.go:1400` `SessionService.UpdateSession`.
  The program branch is `session_service.go:1472-1505`. Structurally it mirrors the tags/
  category/status branches immediately around it: guarded by `req.Msg.Program != nil`,
  diffs against `instance.Program`, calls a dedicated `instance.Set*` mutator, appends to
  `updatedFields`, and (like the rate-limit/autonomous branches) triggers an additional live
  side effect.
- Go instance mutator: `session.Instance.SetProgram` —
  `session/instance_actor_setters.go:193-204`. Same actor pattern as `SetTags`,
  `SetCategory`, `SetTitleDirect`, `SetWorkingDir`, `SetRateLimitEnabled`: routes through
  `sendSyncErr`/`instanceState` to mutate under the instance's own lock and rebuild its
  snapshot (`buildSnapshot`).
- A second, separate program-switch code path exists for *automatic* switching (rate-limit
  fallback, not user-initiated): `SessionService.UpdateSessionProgram` at
  `server/services/session_service.go:3958-3988`, invoked only from
  `server/services/capacity_monitor.go:308` when a provider's rate limit is hit. It
  duplicates the port-history-and-restart logic found in the `UpdateSession` handler (see
  "Gaps" below) rather than calling into it.

## How similar "update session" flows persist correctly

`UpdateSession` (`server/services/session_service.go:1400-1620`) is the canonical mutation
handler for an in-flight session and is the pattern to mirror for any fix:

1. Resolve the live instance via `s.reviewQueuePoller.GetInstances()` (falls back to
   `loadInstancesWithWiring()` only if no poller is wired). `GetInstances()`
   (`session/review_queue_poller.go:891-898`) returns a **shallow copy of the slice**, but
   the same underlying `*Instance` pointers the poller uses for background polling — so
   mutating `instance` in the handler mutates the live/polled object too (no dual-copy
   divergence).
2. Each field is applied via a dedicated `Set*` actor method on `Instance`, not by mutating
   struct fields directly from the RPC layer (see `SetTags` — `session_service.go:1466`,
   `SetCategory` — `:1454`, `SetWorkingDir` — `:1509`, `SetRateLimitEnabled` — `:1517`).
3. All per-field mutations are applied in memory first; a single
   `s.storage.SaveInstances(instances)` call persists everything at the end
   (`session_service.go:1595-1598`). `Storage.SaveInstances` (`session/storage.go:238-263`)
   is an **upsert per instance into the ent/DB-backed repository**, not a monolithic
   sessions.json rewrite, so calling it with a single-instance slice (as
   `UpdateSessionProgram` does) is safe and idempotent — it does not clobber other sessions'
   state.
4. Status changes (pause/resume) are deliberately handled **last**, specifically so that if
   a mutation with a live side effect fails, already-applied metadata is not silently lost —
   see the comment at `session_service.go:1566-1568`. The status change is followed
   immediately by the single `SaveInstances` call, so the whole set of updates either
   persists together or the handler returns an error before reaching that line.
5. A `SessionUpdatedEvent` (or `...WithDetection` variant) is published after a successful
   save so the frontend's live store gets an authoritative update via `WatchSessions`
   (`session_service.go:1600-1616`), not just the direct RPC response.

**The program branch does not fully follow rule 4, and this is the likely root cause of
"does not save correctly."** It performs its own out-of-band
`s.storage.SaveInstances(instances)` at `session_service.go:1494-1498` *before* calling
`instance.Restart(true)`, specifically so the new program is durable "even if Restart
fails" (comment at `:1493`). But `Restart()` (`session/instance.go:1368+`) stops the
controller and kills the tmux session *before* it can fail — `i.StopController()` then
`i.KillSession()` (`session/instance.go:1391-1396`) — and if `KillSession()` errors,
`UpdateSession` returns a `CodeInternal` error (`session_service.go:1499-1502`) **without**
publishing a `SessionUpdatedEvent` and without the frontend ever receiving
`response.session`.

On the frontend, `useSessionService.updateSession`
(`web-app/src/lib/hooks/useSessionService.ts:280-316`) only calls
`dispatch(upsertSession(response.session))` on the success path (`:304-306`); the catch
block (`:309-312`) only sets an error string and returns `null` — it never re-syncs the
Redux store. Net effect: the DB has the new `program` (pre-saved), but the UI still shows the
old program plus a generic "failed to restart" error — the value **did** save, but the user
has no way to tell, which matches the bug report's "does not save correctly" symptom exactly.
This divergence-on-partial-failure is the one place `UpdateSession`'s program branch deviates
from the "save once, after everything succeeds" pattern used by every other field in the same
handler.

Tags — the cleanest "does persist correctly" example to copy — is fully atomic: `SetTags`
happens in memory (`session_service.go:1461-1470`), and there's a single terminal
`SaveInstances` call for the whole request, with no early/partial save.

## Current UI for program switching (what exists today)

A "Change Program" feature is already built and wired in **two** of the app's three session
surfaces:

- `web-app/src/components/sessions/SessionActionsOverflow.tsx` implements the full picker:
  an `onChangeProgram` prop (`:64-65`), a "⚙️ Change Program" menu item gated on that prop
  being provided (`:613-620`), and a modal `<select>` dialog with a "System default" option,
  the known-programs list from `useAvailablePrograms()` (`:125`), and a custom-value
  fallback option (`:745-803`). Saving calls `onChangeProgram(session.id, value)`
  (`:790`) from the dialog's Save button, with a local `isSavingProgram` spinner but no
  optimistic UI beyond that.
- `web-app/src/components/sessions/SessionCard.tsx:841` and
  `web-app/src/components/sessions/SessionRow.tsx:387` both wire
  `onChangeProgram={(_id, program) => { void sessionActions.update({ program }); }}` —
  i.e. they pass the callback through `useSessionActions` → `useSessionService.updateSession`
  → the `UpdateSession` RPC described above.
- `web-app/src/components/sessions/SessionDetailView.tsx` has a **second, independent**
  inline editor for the same field (not reusing `SessionActionsOverflow`'s picker): a
  `<select>` shown when `isEditingProgram` is true (`:951-968`), driven by its own
  `useAvailablePrograms()` call (`:175`) and a `handleSaveProgram` built via the generic
  `makeStringFieldEditor` helper (`:355-359`, factory shared with the `workingDir` and
  `category` inline editors), which calls `actions.update({ program: v })` — same RPC, same
  code path as the overflow-menu picker.

### Gap: `PaneHeader.tsx` does not expose "Change Program"

`web-app/src/components/pane/PaneHeader.tsx:106-122` renders `SessionActionsOverflow` for
the session-detail pane header but does **not** pass `onChangeProgram` (compare its prop
list — `onPause`, `onResume`, `onDelete`, `onRestart`, `onClone`, `onNewWorkspace`,
`onCreateCheckpoint`, `onRunOneShot`, `onSetRateLimitEnabled`, `onToggleAutonomousMode`,
`onSteerAutonomousSession`, `onClearConversationState`, `onUpdateTags` — no
`onChangeProgram`). Since the menu item is gated on `onChangeProgram && (...)`
(`SessionActionsOverflow.tsx:613`), "Change Program" is silently absent from the pane
header's overflow menu — the one place a user looking at a running session's terminal pane
would most naturally look for it. It only appears in the session list card/row overflow
menu, or by scrolling to the "Program" field in the detail info panel and clicking its
pencil icon. This is a concrete, still-open instance of the "unclear how to change running
sessions" half of the bug report: the feature exists, but is missing from a surface users
would reasonably check first.

## Gaps identified

1. **Partial-failure inconsistency (program branch only).** `UpdateSession`'s program branch
   pre-saves to the DB before calling `Restart()`, then returns an error without publishing
   an update event if `Restart()` fails. Every other field in the same handler saves once,
   atomically, after all mutations succeed. This is the most likely source of "does not save
   correctly" — the value **does** save, but the client never learns that, and shows a stale
   value plus an error toast.
2. **Duplicated program-switch logic.** `UpdateSessionProgram`
   (`server/services/session_service.go:3958`, used only by `capacity_monitor.go:308` for
   automatic rate-limit fallback switching) re-implements the same "port history if
   claude⇄antigravity, save, restart-if-active" sequence found in `UpdateSession`'s program
   branch (`:1484-1503`), with slightly different error handling (it logs-and-continues on
   save failure instead of returning an error). A fix should consider extracting one shared
   helper both call, so a persistence fix isn't applied in only one of the two places.
3. **Missing UI entry point.** `PaneHeader.tsx` never passes `onChangeProgram` to
   `SessionActionsOverflow`, so the pane/detail header overflow menu has no "Change Program"
   item, unlike the session-list card/row overflow menus.
4. **No feature-registry entry.** `docs/registry/features/` has no per-feature JSON for
   program-switching (`grep -ri program docs/registry/features/` only turns up
   `analytics/get-program.json` and `ui/analytics-drill-down.json`, both about program-usage
   *analytics*, not switching). Per `.claude/rules/feature-registry.md`, a fix should add
   `docs/registry/features/frontend/change-session-program.json` (and/or a backend entry if
   `UpdateSession`'s program field is treated as its own feature) and run
   `make registry-generate`.
5. **No test coverage for the program-update path.** `server/services/session_service_test.go`
   has dedicated tests for tags (`TestUpdateSession_TagsUpdate`,
   `TestUpdateSession_TagsUpdate_Replaces`) and generic ordering
   (`TestUpdateSession_HandlerOrdering_MetadataBeforeStatus`), but there is no
   `TestUpdateSession_ProgramUpdate` and no test exercising the restart-on-active-session or
   claude⇄antigravity history-porting branches (every other `Program:` occurrence in that
   test file is just `"claude"` used as an unrelated fixture default). On the frontend, no
   test file references `onChangeProgram`/"Change Program" — neither
   `web-app/src/components/sessions/__tests__/` nor `tests/e2e/` has any hits. This aligns
   with `.claude/rules/feature-testing-registry.md`'s expectation that user-triggerable
   actions have dedicated test coverage; "Change Program" isn't an Omnibar action so it
   isn't covered by that specific registry either — it needs its own component/e2e test.

### Suggested fix shape (for the planning phase, not applied here)

- Make the program branch's persistence atomic with the rest of `UpdateSession` (only
  pre-save if `Restart()` genuinely needs the persisted program to succeed; otherwise fold
  it into the single terminal `SaveInstances` call), and on `Restart()` failure still publish
  a `SessionUpdatedEvent` (or otherwise ensure the frontend store is corrected) so the UI
  reflects the actually-persisted `program` value even when the live restart failed.
- Extract the shared "port history + save + restart-if-active" sequence into one helper used
  by both `UpdateSession`'s program branch and `UpdateSessionProgram`.
- Wire `onChangeProgram` into `PaneHeader.tsx`'s `SessionActionsOverflow` usage, reusing the
  same `sessionActions.update({ program })` pattern already used by `SessionCard.tsx` and
  `SessionRow.tsx`.
- Add `docs/registry/features/frontend/change-session-program.json`, run
  `make registry-generate`, and add Go tests (`TestUpdateSession_ProgramUpdate`, plus a
  restart-failure case) and a frontend/e2e test for the "Change Program" flow per
  `.claude/rules/feature-registry.md` and `.claude/rules/e2e-test-conventions.md`.
