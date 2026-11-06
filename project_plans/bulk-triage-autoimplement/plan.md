# Implementation Plan: Bulk Triage and Auto-Implementation

Item: 2ec26b1c-710b-4838-8658-e9f8968e8599
Complexity: 2

## Summary

Extend the existing backlog bulk-action toolbar (Archive, Delete) with two new actions: **Bulk Triage** and **Bulk Auto-Implement**. Both are frontend fan-outs to existing single-item RPCs; no proto/server changes. Key decisions from research and pre-mortem:
- `spawnSessionFromItem` returns `{queued: true}` at the WIP cap — requires a third `queued` result bucket in feedback (distinct from success/failure).
- Auto-implement confirmation dialog uses `useFocusTrap` (full focus management); `useFocusTrap` hook confirmed present in `web-app/src/lib/hooks/useFocusTrap.ts`.
- **Both** triage AND spawn fan-outs are chunked (max 5 concurrent) — spawn has no server-side WIP cap for the fan-out itself (only a per-item session cap), so unbounded parallel RPC fires risk a resource storm.
- Items pre-filtered for eligibility in page.tsx handlers before fan-out, to avoid confusing "failed" counts from status-race `CodeFailedPrecondition` errors.
- **`handleAutoImplementSelected` must check `busyRef.current` at entry** (pre-mortem P1-2) to prevent double-tap/double-fire on mobile before React commits the busy state.
- **Eligible IDs are snapshot-frozen at dialog-open time** (pre-mortem P1-3) into a `useRef` to prevent TOCTOU divergence between dialog count and handler execution count.

---

## Tech Debt Disposition

The existing `BacklogBulkActions.tsx` delete confirmation dialog skips `useFocusTrap`, a gap already noted but out of scope here. The auto-implement dialog will set the correct pattern; the delete dialog gap is tracked as a follow-up (not a blocker).

**Disposition: Extend-as-is** — the `BacklogBulkActions` component is clean, not a hotspot. New actions integrate via new callback props, maintaining the same separation of concerns as Archive/Delete.

---

## Epic 1 — Bulk Triage Action

### Story 1.1 — Eligibility helpers

**Task 1.1a** — Add `isBulkTriageEligible(item)` pure function to `web-app/src/lib/backlog/bulkActions.ts`:
- Eligible when: `status === "idea" || status === "ready"` AND `triageStatus !== "running"`.
- Export type alias `type EligibilityFilter<T> = (item: T) => boolean` for reuse.
- No changes to `runBulk` or `formatBulkFeedback`.

**Task 1.1b** — Add `runBulkChunked(ids, action, chunkSize)` helper in `web-app/src/lib/backlog/bulkActions.ts`:
- Splits `ids` into chunks of `chunkSize` (default 5), runs each chunk as `Promise.allSettled`, merges results into `BulkResult`.
- Used only by bulk triage to avoid spawning many headless LLM processes simultaneously.
- Export alongside `runBulk`.

**Task 1.1c** — Unit tests in `web-app/src/lib/backlog/bulkActions.test.ts` for both helpers:
- `isBulkTriageEligible`: cover `idea`, `ready`, `ready + triageStatus=running`, `in_progress`, `archived`.
- `runBulkChunked`: cover chunk-boundary splitting, partial failure, full failure.

### Story 1.2 — BacklogBulkActions component update

**Task 1.2a** — Add `onTriageSelected: () => void` prop to `BacklogBulkActionsProps` interface.

**Task 1.2b** — Render "Triage selected" button in the `actions` div:
```tsx
<button
  type="button"
  className={actionButton}
  disabled={busy}
  onClick={() => { track({...}); onTriageSelected(); }}
  data-testid="backlog-bulk-triage-selected"
>
  Triage selected
</button>
```
- Position: before "Archive selected" (natural action-progression order: Triage → Archive → Delete).

**Task 1.2c** — Tests in `BacklogBulkActions.test.tsx`:
- Button visible when `selectedCount > 0`.
- Click fires `onTriageSelected` handler.
- Button disabled when `busy=true`.

### Story 1.3 — Page wiring

**Task 1.3a** — In `web-app/src/app/backlog/page.tsx`, add `handleTriageSelected` callback:
```typescript
const handleTriageSelected = useCallback(async () => {
  const byId = new Map(items.map((i) => [i.id, i]));
  const eligible = visibleSelectedIds.filter((id) => {
    const it = byId.get(id);
    return it !== undefined && isBulkTriageEligible(it);
  });
  const skipped = visibleSelectedIds.length - eligible.length;
  if (eligible.length === 0) {
    showBulkFeedback(skipped > 0 ? `Nothing to triage (${skipped} ineligible)` : "Nothing to triage");
    dispatch(clearSelection());
    return;
  }
  const result = await runBulkChunkedAction("Triage started for", eligible, triggerTriage, 5);
  if (!result || !mountedRef.current) return;
  const feedbackSuffix = skipped > 0 ? ` (${skipped} skipped — ineligible)` : "";
  showBulkFeedback(formatBulkFeedback("Triage started for", result) + feedbackSuffix);
  if (result.failed.length === 0) dispatch(clearSelection());
  else dispatch(selectAll(result.failed));
}, [items, visibleSelectedIds, triggerTriage, runBulkChunkedAction, showBulkFeedback, dispatch]);
```
- Wire `onTriageSelected={handleTriageSelected}` to `<BacklogBulkActions>`.

---

## Epic 2 — Bulk Auto-Implementation Action

### Story 2.1 — Spawn result type with queued bucket

**Task 2.1a** — Add `SpawnBulkResult` interface to `web-app/src/lib/backlog/bulkActions.ts`:
```typescript
export interface SpawnBulkResult {
  started: string[];  // spawned live sessions
  queued: string[];   // hit WIP cap, transitioned to queued status
  failed: string[];
}
```

**Task 2.1b** — Add `runBulkSpawnChunked(ids, spawnFn, chunkSize = 5)` helper:
- Splits `ids` into chunks of `chunkSize`, runs each chunk as `Promise.allSettled`, merges results.
- Categorises each settled result: `{ queued: true }` → `queued`, `{ sessionUuid }` → `started`, rejected → `failed`.
- Returns `SpawnBulkResult`.
- **Must chunk** (same as `runBulkChunked` for triage) — unbounded concurrent spawn RPCs risk the 2026-07-29 OOM pattern.

**Task 2.1c** — Add `formatSpawnBulkFeedback(result: SpawnBulkResult): string`:
- "Started N session(s)" for pure-success (queued.length === 0, failed.length === 0).
- "Started N, queued M" when queued > 0 and failed === 0.
- "Started N, queued M (K failed)" for partial failures.

**Task 2.1d** — Unit tests for `SpawnBulkResult`, `runBulkSpawn`, `formatSpawnBulkFeedback`.

### Story 2.2 — Eligibility helper

**Task 2.2a** — Add `isBulkAutoImplementEligible(item)` to `web-app/src/lib/backlog/bulkActions.ts`:
- Eligible when: `status === "ready"` AND (`item.skipPlanning || item.planApproved`).
- Unit tests: cover `ready+skipPlanning=true`, `ready+planApproved=true`, `ready+neither`, `in_progress`, `idea`.

### Story 2.3 — BacklogBulkActions component update

**Task 2.3a** — Add `onAutoImplementSelected: () => void` prop to `BacklogBulkActionsProps`.

**Task 2.3b** — Add `confirmingAutoImplement: boolean` local state (alongside `confirmingDelete`).

**Task 2.3c** — Render "Auto-implement selected" button:
```tsx
<button
  type="button"
  className={actionButton}
  disabled={busy}
  onClick={() => { track({...}); setConfirmingAutoImplement(true); }}
  data-testid="backlog-bulk-auto-implement-selected"
>
  Auto-implement selected
</button>
```

**Task 2.3c.5** — Add `autoImplementSnapshotRef = useRef<string[]>([])` in `page.tsx`. When the "Auto-implement selected" button is clicked, snapshot the current `autoImplementEligibleIds` into `autoImplementSnapshotRef.current` BEFORE calling `setConfirmingAutoImplement(true)`. Pass both `eligibleAutoImplementCount` (derived from `autoImplementSnapshotRef.current.length`) and the snapshot via `onAutoImplementSelected` so handler uses frozen list, not live list.

**Task 2.3d** — Render confirmation dialog (outside the toolbar div, parallel to confirmingDelete dialog):
```tsx
{confirmingAutoImplement && (
  <div className={styles.overlay} onClick={() => setConfirmingAutoImplement(false)}>
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="backlog-bulk-auto-implement-title"
      className={styles.dialog}
      data-testid="backlog-bulk-auto-implement-dialog"
      ref={autoImplementDialogRef}
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => { if (e.key === "Escape") setConfirmingAutoImplement(false); }}
    >
      <h2 id="backlog-bulk-auto-implement-title">
        Start autonomous implementation for {eligibleAutoImplementCount} {eligibleAutoImplementCount === 1 ? "item" : "items"}?
      </h2>
      {ineligibleAutoImplementCount > 0 && (
        <p style={{ color: "var(--text-secondary)", fontSize: "13px" }}>
          {ineligibleAutoImplementCount} selected {ineligibleAutoImplementCount === 1 ? "item" : "items"} ineligible (not ready or plan not approved) — will be skipped.
        </p>
      )}
      <p className={styles.warningNotice}>
        ⚠ Sessions will run autonomously without manual steps.
      </p>
      <div className={styles.dialogActions}>
        <button type="button" autoFocus className={clearButton} onClick={() => { track({...}); setConfirmingAutoImplement(false); }}>
          Cancel
        </button>
        <button
          type="button"
          className={actionButton}
          data-testid="backlog-bulk-auto-implement-confirm"
          onClick={() => {
            track({...});
            setConfirmingAutoImplement(false); // close BEFORE async work
            onAutoImplementSelected();
          }}
        >
          Start {eligibleAutoImplementCount} {eligibleAutoImplementCount === 1 ? "session" : "sessions"}
        </button>
      </div>
    </div>
  </div>
)}
```

Notes:
- Dialog closes immediately on confirm click (before the async fan-out) to prevent double-fire.
- `eligibleAutoImplementCount` / `ineligibleAutoImplementCount` are new props passed from page.tsx.
- `useFocusTrap(autoImplementDialogRef, confirmingAutoImplement)` added for proper keyboard focus trapping.
- `styles.warningNotice` added to `BacklogBulkActions.css.ts`.

**Task 2.3e** — Import and wire `useFocusTrap` (already exists in `web-app/src/lib/hooks/useFocusTrap.ts` based on JulesDispatchDialog usage).

**Task 2.3f** — Tests in `BacklogBulkActions.test.tsx`:
- Button visible when `selectedCount > 0`.
- Click opens dialog with correct count text.
- Cancel closes dialog; `onAutoImplementSelected` not called.
- Confirm fires `onAutoImplementSelected`; dialog closes.
- Ineligible count visible in dialog when `ineligibleAutoImplementCount > 0`.
- Escape key closes dialog.
- Dialog uses `role="dialog"` and `aria-modal="true"`.
- Buttons disabled when `busy=true`.

### Story 2.4 — Page wiring

**Task 2.4a** — In `web-app/src/app/backlog/page.tsx`, compute eligible/ineligible counts:
```typescript
const autoImplementEligibleIds = useMemo(() => {
  const byId = new Map(items.map((i) => [i.id, i]));
  return visibleSelectedIds.filter((id) => {
    const it = byId.get(id);
    return it !== undefined && isBulkAutoImplementEligible(it);
  });
}, [items, visibleSelectedIds]);
const autoImplementIneligibleCount = visibleSelectedIds.length - autoImplementEligibleIds.length;
```

**Task 2.4b** — Add `handleAutoImplementSelected` callback:
```typescript
// Takes frozenIds snapshot (from autoImplementSnapshotRef) — NOT live autoImplementEligibleIds.
// This prevents TOCTOU divergence between the count shown in the dialog and the actual fan-out.
const handleAutoImplementSelected = useCallback(async (frozenIds: string[]) => {
  // P1-2 guard: double-tap protection — check busyRef before entering (React's
  // setBulkBusy(true) hasn't committed yet at this point on mobile).
  if (busyRef.current || frozenIds.length === 0) {
    if (frozenIds.length === 0) showBulkFeedback("Nothing eligible for auto-implement");
    return;
  }
  busyRef.current = true;
  setBulkBusy(true);
  try {
    const result = await runBulkSpawnChunked(frozenIds, (id) => spawnSessionFromItem(id, { autonomous: true }));
    if (!mountedRef.current) return;
    showBulkFeedback(formatSpawnBulkFeedback(result));
    if (result.failed.length === 0) dispatch(clearSelection());
    else dispatch(selectAll(result.failed));
  } finally {
    busyRef.current = false;
    if (mountedRef.current) setBulkBusy(false);
  }
}, [spawnSessionFromItem, showBulkFeedback, dispatch]);
```

**Task 2.4c** — Wire new props to `<BacklogBulkActions>`:
```tsx
<BacklogBulkActions
  ...
  onTriageSelected={handleTriageSelected}
  onAutoImplementSelected={handleAutoImplementSelected}
  eligibleAutoImplementCount={autoImplementEligibleIds.length}
  ineligibleAutoImplementCount={autoImplementIneligibleCount}
/>
```

---

## Epic 3 — CSS updates

**Task 3.1** — Add `warningNotice` class to `web-app/src/components/backlog/BacklogBulkActions.css.ts`:
- Background: `vars.color.warningBg` (amber), text: `vars.color.warningText`, padding 8px 12px, border-radius 6px.
- If `warningBg`/`warningText` tokens don't exist in vars, use `background: "rgba(234, 179, 8, 0.12)"` and `color: "var(--text-primary)"` as fallbacks.

---

## Epic 4 — Feature registry

**Task 4.1** — Update `docs/registry/features/backlog-bulk-actions.json` to include the new RPC calls (`TriggerTriage`, `SpawnSessionFromItem` — already registered under other features; add bulk-action marker).
- Or run `make registry-generate` and commit changed files.

---

## Testing checklist

| Scenario | Where tested |
|---|---|
| `isBulkTriageEligible` eligibility matrix | `bulkActions.test.ts` |
| `isBulkAutoImplementEligible` eligibility matrix | `bulkActions.test.ts` |
| `runBulkChunked` chunking + partial failure | `bulkActions.test.ts` |
| `runBulkSpawn` queued/started/failed buckets | `bulkActions.test.ts` |
| `formatSpawnBulkFeedback` string variants | `bulkActions.test.ts` |
| Triage button visible, fires handler | `BacklogBulkActions.test.tsx` |
| Auto-implement button visible, opens dialog | `BacklogBulkActions.test.tsx` |
| Dialog cancel/confirm/Escape | `BacklogBulkActions.test.tsx` |
| Dialog shows eligible/ineligible counts | `BacklogBulkActions.test.tsx` |
| Partial-failure leaves failed IDs selected | `BacklogBulkActions.test.tsx` |
| Both buttons disabled when `busy=true` | `BacklogBulkActions.test.tsx` |

---

## Unresolved Questions

None — all research questions resolved.

---

## Out of Scope

- Adding `BulkTriage` / `BulkAutoImplement` RPCs to the proto (contradicts AC 10, no server changes).
- Fixing the existing delete dialog's focus-trap gap (pre-existing, separate follow-up).
- Board view bulk actions (board page unaffected per AC).
- Adding a global server-side triage concurrency cap (would require proto change; client-side chunking mitigates the immediate risk).
