# Bulk Triage / Bulk Auto-Implement: Known Pitfalls and Risks

## 1. WIP-Cap Queuing for Bulk Auto-Implement

**Finding:** `SpawnSessionFromItem` enforces a server-side WIP cap (default 2, configurable via
Settings → Defaults as `MaxConcurrentBacklogWorkItemsOrDefault`). When `liveCount >= wipCap`, the
item is transitioned to `queued` status and `SpawnSessionFromItemResponse{Queued: true}` is
returned — not an error. `runBulk` treats a successful response (no thrown exception) as
`succeeded`, so the UI will report "Spawned N items" even though most were only *queued*, not
actually running.

**Risk:** Misleading feedback — the user believes N sessions are active when N-2 are parked in
`queued`, waiting for `BacklogLifecycleListener.onSessionExited` or `ReconcileStuck` to dequeue
them.

**Mitigation needed:** `spawnSessionFromItem` should surface the `queued` boolean so
`formatBulkFeedback` can report "Started 2, queued 8" rather than "Started 10 items". The
`BulkResult` type or the action wrapper must distinguish `Queued: true` responses from fully-live
spawns.

---

## 2. Triage Spam: No Rate Limit, but Per-Item In-Flight Guard Prevents Duplication

**Finding:** `TriggerTriage` uses a `sync.Map` called `triageInFlight` as an atomic check-and-set
per item ID (`LoadOrStore`). A second concurrent `TriggerTriage` call for the same item returns
`CodeAlreadyExists`. However, the guard is **per-item**, not a global fan-out ceiling. Triggering
triage for 20 distinct items in parallel spawns 20 independent headless LLM subprocesses.

There is no global triage concurrency cap equivalent to `maxConcurrentBacklogWorkItems`.
Triage sessions are deliberately **not** counted by `maxConcurrentBacklogWorkItems` (see comment
at line 2501 in `backlog_service_triage.go`: "Deliberately NOT gated by
maxConcurrentBacklogWorkItems"). The only in-flight protection is per-item deduplication.

**Risk:** Bulk-triggering triage for many items simultaneously can saturate Claude API quotas or
machine resources. At 20 items there is no server-side throttle — each call creates its own
headless subprocess (worktree + claude invocation) immediately.

**Mitigation needed:** Either (a) add a global triage concurrency cap on the server side, or
(b) fan the bulk action out in small batches client-side (e.g. process in groups of 3–5) rather
than all at once via `Promise.allSettled`. Document the current absence of a server-side guard so
implementers make an explicit call.

---

## 3. Partial-Failure and Status-Race Edge Cases

**Finding:** The existing `runBulk` pattern fans out all `action(id)` calls in parallel via
`Promise.allSettled`. For both new actions there are race windows:

### Bulk Triage
- `TriggerTriage` accepts only `idea` or `ready` status items. Items at `ready` are moved back to
  `idea` before starting — but the server aborts with `CodeFailedPrecondition` if the item has
  moved past `ready` since the user selected it (a concurrent work-session spawn would do this).
  That RPC failure is caught as a `failed` entry by `runBulk`, which is correct behavior, but the
  feedback message "Triggered 7 of 10 (3 failed)" gives no hint that the 3 failures were already
  in-progress items rather than errors.
- Items not in `idea`/`ready` status (e.g. `in_progress`, `done`, `archived`) return
  `CodeFailedPrecondition` immediately, which maps to `failed` in `runBulk`. The selection logic
  should pre-filter to eligible statuses before fan-out to avoid noisy failures.

### Bulk Auto-Implement
- `SpawnSessionFromItem` rejects items not in `ready` or `in_progress` status with
  `CodeFailedPrecondition`. Between the user selecting items and fan-out firing, an item could
  transition (e.g. from `ready` to `queued` by the auto-dequeue sweep, or to `in_progress` by a
  concurrent manual spawn). That transition causes a failed entry.
- The planning gate (`!planApproved && !skipPlanning`) also rejects with `CodeFailedPrecondition`
  for non-autonomous spawns. The bulk confirm dialog should note that only items with approved
  plans (or `skipPlanning: true`) will succeed — pre-filtering the selection to `ready` items with
  `planApproved || skipPlanning` avoids wasted calls.

---

## 4. Confirm-Dialog Double-Click / Re-Fire Guard

**Finding:** The existing delete confirmation dialog calls `setConfirmingDelete(false)` then
`onDeleteSelected()` in the same click handler — the dialog closes synchronously before the async
bulk action completes, so a second click on the (now-gone) confirm button is impossible. This is
the correct pattern.

**Risk for auto-implement:** If the new confirm dialog is implemented the same way (close on
confirm click, then fire), no duplicate-fire problem exists. However, if the dialog stays open
during the async fan-out (to show progress or a spinner), the confirm button must be disabled
immediately on first click (set a local `confirming` flag or rely on the page-level `busyRef`
guard in `runBulkAction`). The page-level `busyRef.current` check at line 709 already prevents
double-fire if the confirm button calls `onAutoImplementSelected`, but only after the first call
reaches `runBulkAction` — there is a brief window between click and `busyRef.current = true`
if the dialog fires the handler asynchronously. Close the dialog immediately on confirm, mirroring
delete.

---

## 5. OOM Risk from Spawning Many Sessions

**Finding:** The 2026-07-29 OOM incident (MEMORY.md) was caused by a session-leak bug
(archive-on-terminal never killed the tmux pane), not bulk spawning per se. The WIP cap (default
2) means bulk auto-implement can at most add 2 live sessions immediately — the remainder are
`queued` and dequeued one at a time as slots free. This is a meaningful protection against the
original OOM class.

**Residual risk:** If the WIP cap is set to a high value (or if `wipErr != nil` causes the cap
check to be skipped — see line 613: "allowing spawn"), a bulk auto-implement of 20 items could
still bypass queuing. The `wipErr` bypass is particularly dangerous here: a transient DB read
failure during bulk fan-out lets every call through simultaneously.

**Mitigation needed:** Add a client-side hard cap (e.g. refuse to bulk-implement more than
`maxConcurrentBacklogWorkItems * 2` items at once, surfaced in the confirm dialog) so even a
server-side failure mode can't cascade into an OOM.

---

## 6. Test Isolation Pattern for New Actions

**Finding:** Existing bulk action tests in `BacklogBulkActions.test.tsx` mock only UI-layer
handlers (no service calls at all — the component just calls `onArchiveSelected`, etc.). Tests
for the page-level handlers in `BacklogPage.*.test.tsx` mock `useBacklogService()`:

```tsx
// from BacklogPage.archivedFilter.test.tsx / BoardPage.stuckItems.test.tsx pattern
jest.mock("@/lib/backlog/useBacklogService", () => ({
  useBacklogService: () => ({
    archiveBacklogItem: jest.fn().mockResolvedValue(undefined),
    deleteBacklogItem: jest.fn().mockResolvedValue(undefined),
    spawnSessionFromItem: jest.fn(),
    triggerTriage: jest.fn(),
    // ...other methods
  }),
}));
```

**Required for new actions:** `triggerTriage` and `spawnSessionFromItem` both already appear in
the mock objects in `BacklogItemDetail.*.test.tsx` (seen at lines 64, 95 respectively). New
page-level bulk tests need:
- `triggerTriage: jest.fn().mockResolvedValue({})` for triage — no `queued` field to worry about
  since `TriggerTriageResponse` has no such field.
- `spawnSessionFromItem: jest.fn().mockResolvedValue({ queued: false })` for the standard path,
  and a second mock variant returning `{ queued: true }` to test the queued-feedback branch.

The `runBulk` utility itself is already unit-tested in `bulkActions.test.ts` — no changes needed
there unless a `BulkResult` type extension is required to carry the `queued` count.
