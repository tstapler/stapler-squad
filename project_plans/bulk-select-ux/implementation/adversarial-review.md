# Adversarial Review: bulk-select-ux

**Date**: 2026-06-23
**Verdict**: BLOCKED

---

# Re-Review: bulk-select-ux (Patch Pass)

**Date**: 2026-06-23
**Verdict**: APPROVED WITH CONCERNS — 0 blockers, 8 concerns, 6 minors

## Blocker Re-Verification

### Blocker 1 — `beforeunload` flush ✅ RESOLVED
Task 3.3.1-B is struck through and marked "REMOVED (Blocker 1 fix)." The section explicitly documents that `useEffect` cleanup (unmount only) is the sole lifecycle safety mechanism and that tab-close data loss during the 5-second undo window is an **accepted known limitation**, documented verbatim in the plan. The Pattern Decisions table is updated to reflect the removal. `flushPendingDeletes` in Task 3.2.1-B correctly notes that `setPendingDeleteIds` on unmount is a no-op. No `beforeunload` or `sendBeacon` references remain.

### Blocker 2 — Undo restore source of truth ✅ RESOLVED
`pendingDeleteRef` now includes `sessions: Session[]` in the type definition (Task 3.2.1-A), with an inline comment explaining why IDs alone are insufficient. Task 3.2.1-D specifies undo as: `clearTimeout` + `setPendingDeleteIds(new Set())` + `pendingDeleteRef.current = null` — fully synchronous, no RPC, no server round-trip. The `filteredSessions` memo (Task 4.1.1-A) correctly excludes `pendingDeleteIds`, making the optimistic removal and restoration mechanically coherent.

### Blocker 3 — Shift+click anchor filtered out ✅ RESOLVED
`computeRangeIds` now explicitly handles `anchorIdx === -1` with a single-select fallback (Task 2.1.1-B). Task 2.1.1-C adds a full Given-When-Then for the filtered-anchor edge case, specifying that `lastAnchorRef` is NOT updated on fallback (stale anchor preserved). Silent graceful degradation is an acceptable design decision for this developer tool.

### Blocker 4 — Mobile touch hover-reveal ✅ RESOLVED
Task 1.4.1-A adds `"@media": { "(hover: none)": { visibility: "visible", pointerEvents: "auto" } }` to `checkboxCell`. Acceptance criteria explicitly cover the touch-device path. The design decision (always-on checkboxes on touch devices, even when `selectMode=false`) is intentional and documented.

---

## New Issues Introduced by Patches

### Regression Check

**Pattern Decisions table — `pendingDeleteRef` storage description is stale** (minor): Line 63 in the Pattern Decisions table still shows `ids: string[]` (not `Set<string>`) while Task 3.2.1-A correctly uses `ids: Set<string>`. No functional impact — the task spec is authoritative — but the table is inconsistent. Low-impact documentation error; will not cause an implementation bug.

**Pattern Decisions table — anchor state row self-contradicts** (minor): The "Anchor state" row (Pattern Chosen column) still says `useState<string | null>` but the Reason column corrects itself mid-sentence: "actually a ref is better here; use `useRef<string | null>`." Task 2.1.1-A correctly specifies `useRef`. The task spec wins, but this table entry will confuse implementers. Pre-existing issue; not introduced by the patch, but should be corrected.

**Blocker 4 patch introduces always-on checkboxes on touch in idle mode** (concern, not regression): On `@media (hover: none)` devices, checkboxes are permanently visible even when `selectMode=false`. This is a deliberate design decision per the patch notes. However, it slightly undermines the hover-reveal UX philosophy (checkboxes should not add visual noise at rest). The acceptance criterion explicitly accepts this — it is a known trade-off, not a regression. No action required unless product objects.

---

## Carried-Forward Concerns (from first review — not resolved by patches)

- [ ] **`activeSelection` at zero — no auto-exit from select mode specified**: Filter change can cause `activeSelection.size === 0` while `selectMode=true`, showing "0 selected" in BulkActions. Plan does not specify whether to auto-exit. UX is ambiguous.

- [ ] **Replace-not-stack silently forfeits prior undo window**: Task 3.2.1-A: new bulk delete flushes the previous pending delete immediately, forfeiting remaining undo time for batch 1 with no user feedback. Should be documented as an accepted design decision or surfaced to the user.

- [ ] **`useEffect` empty-array cleanup stale closure risk**: Task 3.3.1-A uses `useEffect(() => () => flushPendingDeletes(), [])`. `flushPendingDeletes` is a `useCallback`; if it is recreated, the unmount cleanup holds the stale reference. Safe only because `flushPendingDeletes` reads directly from `pendingDeleteRef.current` (a ref, not closed-over state) — but the plan does not explicitly confirm this safety guarantee. An implementer could introduce a bug here.

- [ ] **`data-select-mode` — which element is "the container"**: Task 1.4.1-C specifies the virtualizer scroll container div (`containerRef` element) but does not identify the precise DOM element. The plan should name the JSX variable or describe the DOM path to prevent ambiguity during implementation.

- [ ] **`handlePauseSelected` / `handleResumeSelected` not described**: The E2E test `bulk-pause in row mode` asserts a passing result, but the plan has no implementation task for these handlers. If they don't exist or aren't wired to `activeSelection`, the test will fail. This is a missing task.

- [ ] **`NotificationToast` `onUndo` ordering — `pendingDeleteRef.current = null` must precede state setters**: Task 3.2.1-D specifies step 3 as `pendingDeleteRef.current = null` before state setters. The plan now explicitly states this ordering requirement. This is addressed sufficiently but warrants an explicit test verifying the ref is null before re-render.

- [ ] **`Escape` uses `stopImmediatePropagation` — silences ALL downstream `document` listeners**: Task 2.2.1-A. `stopImmediatePropagation` is a broader hammer than `stopPropagation`; it silences any listener registered after `SessionList` on `document`. Not introduced by the patches but remains an architectural concern.

- [ ] **`BulkDeleteConfirmModal` removal may break card mode**: Task 3.2.1-C removes the confirm modal without specifying whether card mode uses it. "Must not break existing card mode selection behavior" is a constraint; removing `BulkDeleteConfirmModal` needs an explicit verification step.

---

## Carried-Forward Minors (from first review)

- **`onChange={() => {}}` on checkbox is a11y anti-pattern** (Task 1.2.1-B): Space key fires `onChange`, not `onClick`. Keyboard users cannot toggle checkboxes. Fix: move toggle logic to `onChange`.

- **`navigator.platform` deprecated** (Task 2.2.1-C): Use `navigator.userAgentData?.platform` with `navigator.userAgent` fallback.

- **`role="row"` without `role="grid"` parent is invalid ARIA** (Task 4.2.1-B): Orphaned `role="row"` fails WCAG 1.3.1. Consider `role="option"` within `role="listbox"` instead.

- **No E2E test for `activeSelection` filtered intersection behavior**: The trickiest behavior (select 5, filter to 3, confirm count, delete only 3) has no test story.

- **`computeRangeIds` location is now committed** (Task 2.1.1-B): `rangeSelect.ts` is specified. Minor previously resolved by patch.

- **`showBulkDeleteConfirm` / `BulkDeleteConfirmModal` removal needs card mode verification step**: Same as concern above; also a minor implementation risk.

---

## Summary

All 4 prior blockers are resolved. The patches are mechanically correct and sufficiently specified. The 8 carried-forward concerns (none newly introduced by the patches) are worth tracking but do not block implementation. The 2 most actionable concerns for implementation: (1) the `onChange`/a11y issue on checkboxes is a **minor that will cause a real accessibility bug** — promote to blocker if WCAG AA compliance is a CI gate; (2) the stale `flushPendingDeletes` closure in the unmount effect should be called out explicitly in implementation notes.

## Blockers

- [ ] **`beforeunload` flush is a stub, not a solution** — Task 3.3.1-B explicitly leaves the body empty with a comment "if available; otherwise accept the data loss risk." This is not an implementation plan — it is a documented intention to skip the requirement. The requirement states "Undo toast for bulk delete: show a snackbar with 'Undo' button for 5 seconds" with implied correctness on close. ConnectRPC uses `fetch`-based transports that cannot be sent synchronously in `beforeunload`. `navigator.sendBeacon` requires a URL endpoint configured to accept beacon payloads (POST body as `Blob`/`FormData`) — ConnectRPC endpoints don't support this by default. The plan must either: (a) confirm that the ConnectRPC delete endpoint can be wrapped in a `sendBeacon`-compatible handler, (b) accept and document the data loss explicitly in the requirements doc (not just the plan), or (c) reduce the undo window to a shorter duration that makes the race window less likely. As written, tab close silently skips deletes — sessions appear deleted to the user but persist on the server.

- [ ] **Undo restore has no source of truth for session order** — Task 3.2.1-D says "Restores the session IDs back into the displayed list (reverse the optimistic removal)" but the plan stores only `ids: Set<string>` in `pendingDeleteRef` — not the sessions' original positions in `filteredSessions` or their sort keys. After optimistic removal, the displayed list is re-sorted/re-filtered by the virtualizer. When undo fires, inserting sessions back into a `Set<string>` of IDs and re-fetching from the server (or local cache) will produce correct data, but the visual re-insertion order is undefined unless the plan specifies storing full session objects AND their original indices. The glossary says `pendingDeleteRef` stores `sessions: SessionData[]` but Task 3.2.1-A's type definition only has `ids: Set<string>` — a direct contradiction. One of these is wrong. If `SessionData[]` is not stored, undo cannot restore sessions without a server round-trip (which requires a `RestoreSession` RPC that the requirements document says may not exist).

- [ ] **Shift+click with anchor filtered out between clicks is unhandled** — `computeRangeIds` returns `[targetId]` if `anchorIdx === -1` (anchor not found in `flatItems`). This means: user clicks session A (anchor set), applies a filter that removes session A from `filteredSessions`, then Shift+clicks session C — result is only `{C}` is selected, not a range. This is silently incorrect behavior with no user feedback. The anchor ID must be validated against the current `flatItems` at Shift+click time. The plan should specify: if the anchor is not in the current flat list, either (a) reset the anchor to the target session and perform a plain toggle, or (b) show a brief status message "Anchor session is filtered out — starting new selection." Without this, the Shift+click behavior is unpredictably broken when filters change.

- [ ] **Mobile touch devices: hover-reveal checkboxes are permanently invisible** — The requirements scope explicitly marks "Mobile-specific touch-and-hold to enter select mode" as out of scope, but the memory note for this project states "always consider both form factors." More critically, the requirements say "Checkboxes are hover-revealed on both row and card items (no need to click 'Select' first to see them)" as a success metric. On touch devices, `:hover` is not reliably triggered on tap — touch events fire `touchstart`/`touchend`, not `mouseover`. The pure CSS hover-reveal (`${row}:hover &`) will never show the checkbox on mobile. The plan has no fallback: not a `[data-select-mode="true"]` path (which only works after select mode is already entered), not a touch event listener, not a `"Select" button` as entry point for mobile. Touch users cannot initiate bulk selection at all. If mobile is truly out of scope, the requirements success metric #2 ("Checkboxes are hover-revealed") must be qualified with "desktop only" and the requirements doc updated — not left as a stated success metric that the implementation silently fails on mobile.

## Concerns

- [ ] **`activeSelection` at zero: no plan for auto-exit from select mode** — The plan computes `activeSelection = selectedSessions ∩ filteredSessions`. If a filter change causes ALL selected sessions to be filtered out, `activeSelection.size` drops to 0 while `selectMode` remains `true`. The `BulkActions` toolbar will show "0 selected" with Delete/Pause buttons. This is a confusing state: select mode is on but nothing is selected. The plan does not specify whether `selectMode` should auto-exit when `activeSelection.size === 0`. The UX is ambiguous — auto-exit could be surprising if the user intends to adjust filters and re-select; staying in select mode with 0 is confusing. Either behavior must be specified and implemented.

- [ ] **Replace-not-stack semantics for pending deletes double-fires RPCs on rapid successive deletes** — Task 3.2.1-A: "if a new bulk delete fires while `pendingDeleteRef.current` is non-null, immediately flush the previous pending delete." This means: user deletes batch 1 (3 sessions), waits 3 seconds, deletes batch 2 (2 sessions) — batch 1's delete RPCs fire immediately (before their 5-second undo window expires). The user had 2 seconds of undo window remaining for batch 1 but it was forfeited silently. The plan should either: (a) warn the user in the toast ("Undo for previous deletion no longer available"), (b) stack multiple undo windows (adds complexity), or (c) document this as an accepted design decision with rationale.

- [ ] **`useEffect` empty-array unmount flush: stale closure risk on `flushPendingDeletes`** — Task 3.3.1-A adds `useEffect(() => () => flushPendingDeletes(), [])` with "intentionally empty." But `flushPendingDeletes` is a `useCallback` with its own dependencies. If `flushPendingDeletes` is recreated (its dependencies change), the closure captured in the empty-array effect will reference the stale version. The cleanup closure captures the function reference at mount time, not the latest. Solution: use `useRef` to hold a `flushRef.current = flushPendingDeletes` pattern, and call `flushRef.current()` in the cleanup — or use a `pendingDeleteRef` that the flush reads directly (which it does), making the stale closure moot only if `flushPendingDeletes` reads directly from `pendingDeleteRef.current` without closed-over state. The plan must explicitly confirm this is safe or fix it.

- [ ] **`data-select-mode` attribute on scroll container breaks CSS cascade for off-screen virtualized rows** — Task 1.4.1-C puts `data-select-mode` on the virtualizer scroll container. The CSS selector `[data-select-mode="true"] &` in `checkboxCell` uses a descendant combinator. This is correct for rows that are in the DOM. However, the virtualizer unmounts off-screen rows. When a row is remounted after scrolling, the CSS will immediately apply `data-select-mode` correctly — so this is fine functionally. The concern is that the acceptance criterion says "Given `selectMode=true` in SessionList, When the list container renders, Then `data-select-mode="true"` is on the container element" — but the plan does not specify which element is the "container." The `containerRef` could be the scroll container, the list wrapper, or an intermediate div. The plan should specify a precise DOM path or add a note about which element gets the attribute.

- [ ] **Bulk pause/resume operations in `handlePauseSelected` and `handleResumeSelected` are not described** — The plan adds `activeSelection`-aware bulk delete (Task 4.1.1-B) but only mentions `handlePauseSelected` and `handleResumeSelected` by name without specifying where they are, whether they exist today, whether they need undo treatment, or whether they use `activeSelection` vs. `selectedSessions`. The E2E test `bulk-pause in row mode` asserts that sessions "show paused status" after clicking "Pause Selected" — but if `handlePauseSelected` doesn't exist yet or isn't wired to `activeSelection`, this test will fail. This is a missing task in the plan.

- [ ] **`NotificationToast` `onUndo` callback timing: toast dismissed before undo fires** — Task 3.1.1-B: the "Undo" button calls `notification.onUndo!()` then `removeNotification(notification.id)`. If `onUndo` is synchronous (it should be — it just clears the timer and restores state), this order is fine. However, the plan does not specify whether `removeNotification` triggers a re-render that could cause `onUndo`'s state updates to be batched with the notification removal. In React 18 automatic batching, both state updates will batch together — this is fine. But if `onUndo` calls `clearTimeout` and also fires a setter (e.g., `setFilteredSessions`), and these batch with `removeNotification`'s state update, the combined re-render must not accidentally re-run the pending delete timer. The plan should add a note confirming that `pendingDeleteRef.current = null` is set synchronously in `undoFn` before any state setters, so the flush path sees `null` during the same render cycle.

- [ ] **Keyboard shortcut: `Escape` uses `stopImmediatePropagation` which affects ALL other listeners** — Task 2.2.1-A: the Escape handler calls `e.stopImmediatePropagation()` to prevent `page.tsx`'s handler. `stopImmediatePropagation` silences ALL subsequent `keydown` listeners registered on `document` — not just `page.tsx`. If any MCP tool, notification system, or modal registers an Escape listener on `document` after `SessionList` mounts, it will be silenced when `selectMode` is true. `e.stopPropagation()` would be sufficient if `page.tsx`'s handler is on the same element as `SessionList`'s; `stopImmediatePropagation` is a sledgehammer. The plan should justify the choice or switch to a less aggressive approach (check `e.defaultPrevented`, or use a priority system).

## Minors

- **Task 1.2.1-B: `onChange={() => {/* controlled via onClick */}}` is an a11y anti-pattern** — Native checkboxes must have an `onChange` that actually does something; suppressing it prevents keyboard-driven checkbox toggling (Space key on focused checkbox fires `onChange`, not `onClick`). The handler on `onClick` won't fire for keyboard Space. Fix: move the toggle logic to `onChange` and pass the synthetic event up, or ensure `onToggleSelect` fires on `onChange` as well.

- **`navigator.platform` for macOS detection (Task 2.2.1-C) is deprecated** — `navigator.platform` has been deprecated in favor of `navigator.userAgentData.platform` (behind a feature flag) or parsing `navigator.userAgent`. Use `navigator.userAgentData?.platform?.toLowerCase().includes("mac")` with a fallback to `navigator.userAgent.includes("Mac")`.

- **`role="row"` on `SessionRow` without `role="rowgroup"` / `role="grid"` parent is invalid ARIA** — Task 4.2.1-B adds `role="row"` to the row div but the virtualizer scroll container would need `role="grid"` or `role="listbox"` as parent to make the row role valid. Orphaned `role="row"` elements fail WCAG 1.3.1 (Info and Relationships). If the plan chooses `aria-multiselectable`, the full ARIA grid pattern must be established on the container. Otherwise, use `role="option"` within `role="listbox"` — simpler and sufficient for a selection list.

- **No test for "select all → filter → deselect filtered → what is selected count"** — The E2E test plan covers range-select and undo but does not include a test for the `activeSelection` derived intersection behavior (select 5, filter to 3, confirm count shows 3, delete, confirm only 3 deleted). This is one of the trickiest behaviors in the feature and has no corresponding test story.

- **`computeRangeIds` utility location** — The plan says "or a small utility at `web-app/src/lib/utils/rangeSelect.ts`" without committing to one. This non-decision creates ambiguity for the implementer. Recommend committing: put it in `rangeSelect.ts` so it's unit-testable in isolation.

- **`showBulkDeleteConfirm` modal removal in Task 3.2.1-C may break card mode** — The plan says "Remove the `showBulkDeleteConfirm` modal and `BulkDeleteConfirmModal` component (or repurpose it for a non-bulk confirmation flow)." If card mode currently uses `BulkDeleteConfirmModal` as its confirmation step, removing it without verifying card mode's flow would break existing behavior. The plan says "Must not break existing card mode selection behavior" (constraint). This task needs an explicit "verify card mode is unaffected" step before removal.
