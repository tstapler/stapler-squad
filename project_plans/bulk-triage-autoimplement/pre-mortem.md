# Pre-Mortem: Bulk Triage and Auto-Implement

*Feature: Bulk Triage + Bulk Auto-Implement on the backlog toolbar.*
*Written from the perspective: it is 6 months after ship and something broke.*

---

## P1 — Data loss, OOM/DoS, security, completely broken feature

### P1-1: `runBulkSpawn` fires 50 parallel HTTP requests, overwhelming the server

**What happened:** A user selected all 50 items in a large backlog and clicked "Auto-implement selected". The client fired 50 simultaneous `spawnSessionFromItem` calls. The Go HTTP server handled each in its own goroutine; each call hit the Git backend, the worktree allocator, and the tmux session creator concurrently. The server OOM-killed under the combined peak memory of 50 goroutines × session bootstrap allocation.

**Root cause:** `runBulkSpawn` uses `Promise.allSettled` with no concurrency cap — the same pattern as the original `runBulk`. The WIP cap transitions excess items to `queued` status, but that only affects tmux session creation server-side; the HTTP flood of 50 simultaneous RPC calls still lands all at once.

**Probability:** M — backlogs of 50+ items are plausible; the OOM threshold depends on host RAM, but CI logs from 2026-07-29 showed a session-leak OOM at a much smaller concurrent count.  
**Impact:** H — kills the server process and every live tmux session with it.  
**In the plan?** No. The plan adds chunking only for triage (`runBulkChunked`, max 5). Auto-implement fan-out is fully parallel.  
**Recommendation:** Reuse `runBulkChunked` (or a new `runBulkSpawnChunked`) with a conservative cap of 3–5 for auto-implement as well, matching the triage precedent.

---

### P1-2: `handleAutoImplementSelected` does not check `busyRef.current` before entering

**What happened:** A user double-tapped the "Start N sessions" button in the confirm dialog on a mobile device within a single React event cycle. Because the dialog closes synchronously before `bulkBusy` state propagates (close-before-async pattern), the `busy` prop passed to the toolbar buttons was not yet `true` when the second tap registered. `handleAutoImplementSelected` was invoked twice. Both invocations passed the `ids.length === 0` guard, both set `busyRef.current = true` in sequence, and both ran `runBulkSpawn` concurrently. The same 20 items were spawned twice, creating 20 duplicate sessions.

**Root cause:** Unlike `runBulkAction` (which opens with `if (busyRef.current) return`), `handleAutoImplementSelected` sets `busyRef.current = true` after the eligibility guard — it never *reads* `busyRef.current`. Closing the dialog synchronously before the async work means the dialog confirm button is only disabled for the duration of one synchronous call stack, not through the async lifetime of the operation. The second tap fires before React commits the `setBulkBusy(true)` re-render.

**Probability:** L on desktop (one click), M on mobile (double-tap).  
**Impact:** H — duplicate autonomous sessions consuming worktrees, compute, and credits.  
**In the plan?** No. The `busyRef` guard pattern is established in `runBulkAction` but not extended to `handleAutoImplementSelected`.  
**Recommendation:** Add `if (busyRef.current) return;` as the first line of `handleAutoImplementSelected`, identical to `runBulkAction`.

---

### P1-3: Dialog title count diverges from handler eligibility at execute time (TOCTOU)

**What happened:** A user selected 5 items. The live WatchBacklogItems stream delivered status updates while the confirm dialog was open: 3 of the 5 items transitioned to `in_progress` (another host started them). The dialog still showed "Start 5 sessions". The user clicked Confirm. `handleAutoImplementSelected` re-evaluated `autoImplementEligibleIds` from the current `items` store (which now reflected the live updates) — only 2 items were eligible. The user got feedback "Started 2 sessions" after having confirmed a dialog that said 5.

**Root cause:** The dialog renders `eligibleAutoImplementCount` and `ineligibleAutoImplementCount` as props computed in a `useMemo` that recalculates on every `items` or `visibleSelectedIds` change. While the dialog is open, the memo keeps updating. The count shown in the title reflects the moment the dialog last rendered, not the moment it was opened. The handler also uses the same live memo. Both are "correct" at their respective moments but can diverge between open and confirm.

**Probability:** M — live stream delivers status changes continuously under normal load.  
**Impact:** M–H — in the "0 eligible at execute time" case, the user gets silent no-op after confidently confirming a non-zero count. Not data loss, but a severely broken experience that erodes trust.  
**In the plan?** Partially acknowledged via the pre-filtering approach, but the dialog count drift scenario is not addressed.  
**Recommendation:** Snapshot the eligible IDs into a `useRef` at the moment the dialog opens (when `setConfirmingAutoImplement(true)` fires) and pass the frozen snapshot to both the dialog display and the handler. This decouples the "what you confirmed" count from live stream churn.

---

### P1-4: `triggerTriage` on an item already in `refining` triageStatus fires duplicate triage

**What happened:** An item had `status === "ready"` and `triageStatus === "refining"` (the server had started a triage but the client-side `triageStatus` field hadn't been updated via the live stream yet — a sub-second window). `isBulkTriageEligible` checks `triageStatus !== "running"` but the field from the live stream was still `null` or stale. The eligibility filter passed. `triggerTriage` was called. The server accepted it (returned 200 — the RPC doesn't deduplicate if a prior triage session exists but is not yet in a `running` status). A second triage session was created for the same item.

**Root cause:** `isBulkTriageEligible` relies on client-side cached `triageStatus`. The live stream delivers status updates with non-zero latency. The guard `triageStatus !== "running"` only catches items already confirmed as running by the stream; it does not catch items in the triage pipeline but not yet acknowledged.

**Probability:** M — the stale-window is narrow per item, but with a 50-item bulk selection the aggregate probability of at least one hit is substantial.  
**Impact:** M — duplicate triage sessions consume LLM compute and can produce conflicting AC criteria on the item.  
**In the plan?** Not fully. The plan acknowledges status-race for pre-filtering but focuses on `FailedPrecondition` errors, not the scenario where the server silently accepts a duplicate.  
**Recommendation:** Server-side: make `TriggerTriage` idempotent — if a triage session for the item already exists and is not in a terminal state, return success without creating another. Client-side: expand the eligibility guard to also exclude `triageStatus === "refining"`.

---

## P2 — Significant degradation, confusing UX, hard-to-debug issues

### P2-1: "Auto-implement selected" button enabled even when zero eligible items are selected

**What happened:** A user selected 8 items, all of which were in `idea` status (not `ready`, not `planApproved`). The "Auto-implement selected" button was enabled (not gated on `eligibleAutoImplementCount > 0`). The user clicked it, saw a dialog saying "Start 0 sessions?" with an ineligibility note, got confused, clicked Confirm anyway, and received feedback "Nothing eligible for auto-implement." The user filed a support ticket thinking the feature was broken.

**Root cause:** The plan does not disable the "Auto-implement selected" button when `eligibleAutoImplementCount === 0`. The `busy` prop only prevents double-fire during an in-flight operation; it says nothing about eligibility.

**Probability:** H — the most common misuse pattern.  
**Impact:** M — no data loss, but generates confusion and support noise.  
**In the plan?** No — the plan omits this guard.  
**Recommendation:** Pass `eligibleAutoImplementCount` as a prop to `BacklogBulkActions` (it already flows there for the dialog display) and add `disabled={busy || eligibleAutoImplementCount === 0}` to the "Auto-implement selected" button.

---

### P2-2: `runBulkChunked` chunk 1 failures do not stop later chunks, leaving partial state difficult to recover

**What happened:** A user triaged 30 items in 6 chunks of 5. Chunks 1–2 succeeded (10 items). Chunks 3–6 failed due to a server restart mid-operation. The failed IDs (20 items) were left in the selection for retry. The user re-triaged, but the first 10 items had already been successfully triaged and were now in `refining` status — they were now ineligible (correctly filtered out), but the user had to manually inspect which items to retry. With 30 items across a live-updating table, this was hard to do at a glance.

**Root cause:** `runBulkChunked` processes chunks sequentially but merges all results at the end into a single `BulkResult`. Failed IDs are left selected (correct for atomic retry). But the feedback message "Triage started for 10 of 30 (20 failed)" doesn't distinguish between "failed transiently" and "failed because already triaging" — the user has no way to know whether retrying the 20 is safe.

**Probability:** M — server restarts and transient failures are normal in a managed service.  
**Impact:** M — partial triage is recoverable, but the UX path to recovery is unclear.  
**In the plan?** Partially — the plan specifies leaving failed IDs selected. It does not address recovery discoverability.  
**Recommendation:** Include the ineligible-skip count in the feedback for re-runs ("Triage started for 10 of 20 eligible; 10 ineligible skipped") so the user understands what retrying will actually attempt.

---

### P2-3: Focus is not returned to the trigger button after the auto-implement dialog closes via Confirm

**What happened:** A keyboard-only user opened the dialog with Enter on "Auto-implement selected", confirmed with Enter, and found focus lost to the document body. Screen reader users reported the same: focus vanished after dialog closure.

**Root cause:** `useFocusTrap(autoImplementDialogRef, confirmingAutoImplement)` is called without a `triggerRef`. Per `useFocusTrap.ts` line 98–99, `triggerEl?.focus()` in the cleanup is a no-op when `triggerRef` is `undefined`. The dialog closes via `setConfirmingAutoImplement(false)` before the async work, unmounting the dialog and triggering the cleanup — focus is not restored to the button that opened it.

**Probability:** H — any keyboard or screen reader user who confirms the dialog hits this.  
**Impact:** M — not a security or data issue, but a WCAG 2.1 failure (SC 4.1.3 / Focus management) that blocks keyboard users.  
**In the plan?** No — the plan wires `useFocusTrap` but omits the `triggerRef` parameter.  
**Recommendation:** Hold a `useRef` for the "Auto-implement selected" button and pass it as `triggerRef` to `useFocusTrap`.

---

### P2-4: Stale eligibility feedback when `visibleSelectedIds` includes items that scrolled out of view

**What happened:** A user applied a search filter that hid most items, then selected all visible items (20), cleared the search filter, and clicked "Triage selected". `visibleSelectedIds` filtered against `sortedItems` (the current visible set), which now included items that were hidden during selection. The triage fan-out triggered items the user had not intended to include.

**Root cause:** `visibleSelectedIds` is re-derived on every render from `selectedIds ∩ sortedItems`. When the user cleared the filter, `sortedItems` expanded to include all items, and `visibleSelectedIds` grew accordingly — the intersection grew without the user explicitly selecting additional items.

**Probability:** L — requires a specific filter-then-clear workflow.  
**Impact:** M — unintended triage/auto-implement on items the user didn't explicitly select.  
**In the plan?** No — the plan focuses on items that drop out of the visible set (deselection), not items that re-enter it (unintended inclusion).  
**Recommendation:** Document this behavior in a code comment. Consider whether `visibleSelectedIds` should be an *intersection* (current behavior: items selected AND currently visible) or whether selection should be frozen at click time. Current behavior is defensible but should be clearly named.

---

### P2-5: WIP cap "queued" bucket silently inflates success metrics in analytics

**What happened:** Analytics showed "bulk auto-implement success rate = 92%". In reality, 30% of "started" events were actually `queued` — items that the WIP cap had parked without creating a session. The feedback message said "Started 15, queued 5" but the `track()` call in the confirm handler fired `{ name: "backlog_bulk_auto_implement_confirm", labels: { count: eligibleAutoImplementCount } }` — before the handler ran, so it always reflected the pre-execute count, not the actual started/queued/failed split.

**Root cause:** Analytics are fired on the "Confirm" click (before async work, by design to capture intent). There is no follow-up analytics event fired after the operation completes with actual outcome buckets.

**Probability:** H — any deployment with a WIP cap in use.  
**Impact:** M — misleading metrics don't break the feature but impede debugging and product decisions.  
**In the plan?** No. The plan specifies a `track()` on confirm click; it does not specify a post-operation outcome event.  
**Recommendation:** Fire a second `track()` event after `runBulkSpawn` resolves with `{ started, queued, failed }` counts as labels.

---

## P3 — Minor annoyances, edge cases

### P3-1: Feedback "Triage started for 0 of 0" shown when all selected items are ineligible and skipped

**What happened:** A user selected 3 in-progress items. `handleTriageSelected` computed `eligible = []` and `skipped = 3`. The plan's early-return path shows "Nothing to triage (3 ineligible)" and calls `dispatch(clearSelection())`. This is correct behavior, but the "Nothing to triage" message is easy to miss — it appears in the same low-contrast feedback strip as success messages, uses no distinct styling, and times out in 4 seconds. Users re-submitted the same selection multiple times before noticing it.

**Probability:** M — ineligible-only selections are a common first-time-user pattern.  
**Impact:** L — no harm done; just confusion and repeated clicks.  
**In the plan?** The message text is specified correctly; styling is not.  
**Recommendation:** Consider a distinct `aria-live="assertive"` or a brief toast variant for "nothing to do" outcomes, separate from the success feedback strip.

---

### P3-2: `formatSpawnBulkFeedback` returns a different shape than `formatBulkFeedback`; passing the wrong one to a shared component would silently truncate queued info

**What happened:** A developer refactoring the feedback strip passed `formatBulkFeedback(result)` instead of `formatSpawnBulkFeedback(result)` to the auto-implement handler's `showBulkFeedback` call. TypeScript did not catch this because both functions return `string`. The "queued" count was silently dropped: "Started 8 of 10" instead of "Started 8, queued 2".

**Root cause:** `BulkResult` and `SpawnBulkResult` are structurally different types (`succeeded/failed` vs. `started/queued/failed`), but `formatBulkFeedback` and `formatSpawnBulkFeedback` both accept their respective types and return `string`. There is no compile-time guard preventing passing a `SpawnBulkResult` to `formatBulkFeedback` if the developer destructures it manually.

**Probability:** L — only hits on future refactors, not at initial implementation.  
**Impact:** L — queued count is lost from the feedback message; no data loss or incorrect behavior otherwise.  
**In the plan?** No.  
**Recommendation:** Consider co-locating `runBulkSpawn` and `formatSpawnBulkFeedback` so they're always used as a pair, or add a branded type tag (`_kind: "spawn"`) to `SpawnBulkResult` to make it structurally distinct from `BulkResult`.

---

### P3-3: Selection cleared on full success even when live stream has not yet reflected the new status

**What happened:** A user triaged 5 items. All `triggerTriage` calls returned 200. `dispatch(clearSelection())` fired. 200ms later, the live stream delivered status updates showing 2 of the 5 items had actually returned to `idea` status (the triage session bounced immediately due to a missing repo config). The user had no visual indication which items bounced because the selection was already gone.

**Probability:** L — requires a fast triage bounce race.  
**Impact:** L — items are still visible in the table; the user can re-select manually. Slightly confusing.  
**In the plan?** No — this is an inherent limitation of fire-and-forget fan-out; the plan acknowledges it implicitly.  
**Recommendation:** Document as a known limitation in a code comment. The alternative (poll for completion before clearing) would require a substantial streaming solution that is out of scope.

---

## Summary Table

| ID | Risk | Prob | Impact | In Plan? |
|---|---|---|---|---|
| P1-1 | `runBulkSpawn` uncapped parallel flood (OOM) | M | H | No |
| P1-2 | `handleAutoImplementSelected` missing `busyRef` guard (duplicate sessions) | L | H | No |
| P1-3 | Dialog count / handler eligibility TOCTOU divergence | M | M–H | Partial |
| P1-4 | `triggerTriage` duplicate on stale `triageStatus` field | M | M | Partial |
| P2-1 | "Auto-implement" button enabled with 0 eligible items | H | M | No |
| P2-2 | Partial triage failure recovery UX is unclear | M | M | Partial |
| P2-3 | Focus not returned after dialog confirm (keyboard/a11y) | H | M | No |
| P2-4 | `visibleSelectedIds` expands unexpectedly when filter cleared | L | M | No |
| P2-5 | Analytics events fire pre-execute; queued/failed not captured | H | M | No |
| P3-1 | "Nothing to triage" feedback easy to miss | M | L | Partial |
| P3-2 | `formatBulkFeedback` / `formatSpawnBulkFeedback` mix-up risk | L | L | No |
| P3-3 | Selection cleared before live stream reflects bounce | L | L | No |
