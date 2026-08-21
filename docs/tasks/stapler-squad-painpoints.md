# Implementation Plan: Stapler Squad Pain Points

**Status**: Ready for implementation
**Date**: 2026-04-16
**Branch**: `stapler-squad-painpoints`
**Each pain point ships as a separate PR.**

---

## Epic Overview

Seven discrete UX friction points that accumulate daily developer workflow cost. All are independent — none blocks another. The ordering below is by impact/effort ratio.

| # | Pain Point | Effort | Impact |
|---|-----------|--------|--------|
| 1 | Branch autocomplete (real git refs) | 1 week | High — daily friction on every new session |
| 2 | Lazy scrollback Phase 1 (tail-only) | 3 days | High — large sessions currently stall on attach |
| 3 | Mobile touch scroll | 1 week | High — mobile is unusable today |
| 4 | Mobile layout / keyboard | 3 days | High — complements touch fix |
| 5 | Frontend observability | 2 days | Medium — enables future diagnosis |
| 6 | Quick rename / retag | 3 days | Medium — daily friction |
| 7 | Bulk actions | 1 week | Lower — nice to have |

---

## Architecture Decision Records

| File | One-line summary |
|------|-----------------|
| `project_plans/stapler-squad-painpoints/decisions/ADR-001-lazy-scrollback-delivery-strategy.md` | Phase 1 sends last 500 lines via `GetLastN`; Phase 2 adds `GetScrollback` unary RPC for scroll-up loading |
| `project_plans/stapler-squad-painpoints/decisions/ADR-002-branch-autocomplete-rpc-design.md` | New `ListBranches` unary RPC shells to `git for-each-ref refs/heads` with 5-minute in-memory cache; replaces session-derived fallbacks in `useBranchSuggestions` |
| `project_plans/stapler-squad-painpoints/decisions/ADR-003-frontend-observability-approach.md` | Phase 1: custom `POST /api/telemetry` JSON endpoint; Phase 2: OTel JS SDK via dynamic import |
| `project_plans/stapler-squad-painpoints/decisions/ADR-004-mobile-touch-scroll-approach.md` | JS `touchstart`/`touchmove` interception + `terminal.scrollLines(delta)`; CSS `overscroll-behavior: contain` as belt-and-suspenders for iOS 16+ |

---

## Dependency Visualization

```
[1 Branch autocomplete]
  proto/session.proto  -->  server/services/session_service.go
                        -->  web-app/src/lib/hooks/useBranchSuggestions.ts  (replace)
                        -->  web-app/src/gen/  (make generate-proto)

[2 Lazy scrollback Phase 1]
  session/scrollback/buffer.go (GetLastN - already exists)
    --> server/services/terminal_stream_handler.go  (change GetAll → GetLastN(500))
    (no frontend change required)

[3 Mobile touch scroll]
  web-app/src/components/sessions/XtermTerminal.tsx
    --> web-app/src/lib/hooks/useTouchScroll.ts  (new)
    --> web-app/src/components/sessions/XtermTerminal.module.css

[4 Mobile layout / keyboard]
  web-app/src/components/sessions/TerminalOutput.tsx
    (visualViewport resize listener already scaffolded via metricsRef)

[5 Frontend observability]
  server/server.go  (register /api/telemetry route)
    --> server/handlers/telemetry_handler.go  (new)
    --> web-app/src/lib/telemetry.ts  (new)
    --> web-app/src/components/sessions/TerminalOutput.tsx  (POST metricsRef data)

[6 Quick rename/retag]
  web-app/src/components/sessions/SessionCard.tsx  (inline input toggle)
    --> web-app/src/components/sessions/SessionCard.css.ts  (vanilla-extract)
    (uses existing RenameSession RPC - already in proto)

[7 Bulk actions]
  web-app/src/components/sessions/BulkActions.tsx  (already exists)
    --> web-app/src/components/sessions/SessionCard.tsx  (add checkbox)
    --> web-app/src/components/sessions/SessionCard.css.ts
    --> web-app/src/store/bulkSelectionStore.ts  (new or zustand slice)
```

No inter-story dependencies. All seven can be developed in parallel.

---

## Story Breakdown

### Story 1: Branch Autocomplete

**Goal**: When a repo is selected in `SessionWizard`, the branch field shows actual branches from that repo's git refs, not session-derived fallbacks.

**Acceptance criteria**:
- Branch dropdown populated from `git for-each-ref refs/heads` for the selected repo path.
- Results cached 5 minutes; cache key is the absolute repo path.
- Timeout after 2 seconds; returns partial results with `truncated: true` flag.
- `AbortController` cancels in-flight request when repo path changes.
- Arrow + Enter keyboard navigation works (existing `AutocompleteInput` behavior preserved).
- Loading spinner shown while fetching (existing `isLoading` prop on `AutocompleteInput`).

#### Task 1.1 — Proto + backend: Add `ListBranches` RPC (2–3 hrs)

Files: `proto/session/v1/session.proto`, `server/services/session_service.go`

- Add `ListBranches(ListBranchesRequest) returns (ListBranchesResponse)` to `SessionService`.
- Add request/response messages: `repo_path`, `filter` (optional), `max_results` (default 200), `include_remote` (default false).
- Response: `repeated string branches`, `int32 total_count`, `bool truncated`.
- Run `make generate-proto`.

#### Task 1.2 — Backend: Implement handler with cache (2–3 hrs)

Files: `server/services/session_service.go` (or new `server/services/branch_service.go`)

- Handler shells to `git -C <repoPath> for-each-ref refs/heads --format='%(refname:short)'` inside `context.WithTimeout(ctx, 2*time.Second)`.
- Validate `repoPath`: must be absolute, must not contain `..`, must pass `os.Stat` check.
- Filter branches in Go (not via shell grep): `strings.Contains(strings.ToLower(branch), strings.ToLower(filter))`.
- Cache: `sync.Map` of `branchCacheEntry{branches []string, cachedAt time.Time}`, TTL 5 minutes. Key: `repoPath`.
- Return `truncated: true` if timeout fires before command completes; return whatever was collected so far.
- Log branch_list_latency_ms as structured log field.

#### Task 1.3 — Frontend: Replace `useBranchSuggestions` hook (1–2 hrs)

Files: `web-app/src/lib/hooks/useBranchSuggestions.ts`

- Replace `ListSessions`-based branch extraction with `ListBranches` RPC call.
- Call when `repositoryPath` changes and is non-empty.
- Use `AbortController`; cancel on `repositoryPath` change or unmount.
- Return `{ suggestions: string[], isLoading: boolean }` — same interface as today, no `SessionWizard` changes needed.
- Show empty suggestions (not fallback hardcoded list) while loading.

#### Task 1.4 — Testing + integration checkpoint (1–2 hrs)

- Manual test: open SessionWizard on a repo with 10+ real branches; verify all appear.
- Manual test: open SessionWizard on a path that is not a git repo; verify empty list (not an error crash).
- Manual test: open SessionWizard, type partial branch name; verify filter works.
- Manual test: change repo path rapidly; verify no stale results from previous path.

---

### Story 2: Lazy Scrollback Phase 1 (Tail-Only)

**Goal**: `StreamTerminal` sends only the last 500 entries on initial attach. Large sessions attach in under 1 second.

**Acceptance criteria**:
- Sessions with >500 scrollback entries attach without sending the full buffer.
- Sessions with <=500 entries are unaffected.
- No frontend change required.
- Existing `StreamTerminal` reconnect logic is unaffected.

#### Task 2.1 — Backend: Change attach path to `GetLastN(500)` (1–2 hrs)

Files: `server/services/terminal_stream_handler.go` (or wherever `StreamTerminal` sends initial scrollback)

- Locate the code that calls `buffer.GetAll()` or iterates all entries on client connect.
- Replace with `buffer.GetLastN(500)`.
- Add a comment explaining the Phase 1/Phase 2 split and referencing ADR-001.
- Log `scrollback_lines_sent` as a structured field on each attach event.

#### Task 2.2 — Verify (1 hr)

- Create a test session that generates >500 lines (e.g., `yes | head -1000`).
- Attach in browser; verify only the last ~500 lines appear.
- Confirm no regression for sessions with <500 lines.

---

### Story 3: Mobile Touch Scroll

**Goal**: Touch-scrolling inside the terminal viewport works smoothly on iOS and Android. Scrolling does not accidentally scroll the page or trigger browser chrome hide/show.

**Acceptance criteria**:
- Vertical swipe inside the terminal scrolls the xterm viewport (up/down).
- Horizontal swipe is not intercepted (preserved for navigation gestures).
- Long-press text selection still works (not broken by touch interception).
- No regression on desktop (mouse/keyboard unaffected).
- `overscroll-behavior: contain` applied as CSS belt-and-suspenders.

#### Task 3.1 — New hook: `useTouchScroll` (2–3 hrs)

Files: `web-app/src/lib/hooks/useTouchScroll.ts` (new)

```typescript
export function useTouchScroll(
  containerRef: RefObject<HTMLElement>,
  getTerminal: () => Terminal | null
): void
```

- On mount: add `touchstart`, `touchmove`, `touchend` listeners with `{ passive: false }`.
- On `touchstart`: record `touchStartY`, `touchStartX`, set `isScrollingRef = true`.
- On `touchmove`: compute `deltaY = touchStartY - event.touches[0].clientY`. If `|deltaY| > |deltaX| + 10px` (primarily vertical): call `terminal.scrollLines(Math.round(deltaY / lineHeightPx))`, reset `touchStartY`, call `event.preventDefault()`.
- `lineHeightPx`: use `terminal.options.fontSize * 1.2` as safe approximation.
- On `touchend`: reset state.
- On unmount: remove all listeners.

#### Task 3.2 — Wire into `XtermTerminal` (1 hr)

Files: `web-app/src/components/sessions/XtermTerminal.tsx`

- Call `useTouchScroll(containerRef, () => terminalRef.current)` inside the component.
- Guard with `typeof window !== 'undefined' && 'ontouchstart' in window` to skip on desktop.

#### Task 3.3 — CSS additions (30 min)

Files: `web-app/src/components/sessions/XtermTerminal.module.css`

- Add `overscroll-behavior: contain` and `touch-action: pan-x pan-y` to `.terminal` class.
- Do NOT use `touch-action: none` (breaks text selection).

#### Task 3.4 — Mobile testing (1–2 hrs)

- Test on iOS Safari (physical device or BrowserStack): swipe scrolls terminal, long-press selects text, horizontal swipe not intercepted.
- Test on Android Chrome: same validation.
- Test on desktop Chrome: confirm mouse wheel and keyboard unaffected.

---

### Story 4: Mobile Layout / Keyboard

**Goal**: The terminal does not shrink/jump when the iOS virtual keyboard appears. After the keyboard dismisses, the terminal returns to full height.

**Acceptance criteria**:
- Terminal height uses `dvh` units instead of `100vh` so it tracks the dynamic viewport.
- `visualViewport.resize` event triggers `terminal.fit()` with a 300ms debounce (iOS needs time to settle after keyboard dismiss).
- No regression on desktop.

#### Task 4.1 — `visualViewport` resize listener in `TerminalOutput` (1–2 hrs)

Files: `web-app/src/components/sessions/TerminalOutput.tsx`

- In the `useEffect` that sets up terminal connections, also wire:
  ```typescript
  const vp = window.visualViewport;
  if (vp) {
    const onVpResize = () => {
      setTimeout(() => xtermRef.current?.fit(), 300);
    };
    vp.addEventListener('resize', onVpResize);
    return () => vp.removeEventListener('resize', onVpResize);
  }
  ```
- This is additive to the existing `ResizeObserver` in `XtermTerminal.tsx` — the two work in parallel.

#### Task 4.2 — CSS: `dvh` units for terminal height (30 min)

Files: `web-app/src/components/sessions/TerminalOutput.module.css` (or whichever CSS controls the terminal panel height)

- Replace any `height: 100vh` or `height: calc(100vh - Npx)` with `height: 100dvh` / `height: calc(100dvh - Npx)`.
- Add fallback: `height: 100vh; height: 100dvh;` (browsers that don't support `dvh` fall back to `vh`).

#### Task 4.3 — Verify (1 hr)

- iOS Safari: tap any input field on the sessions list page, observe keyboard appear; terminal should not shrink noticeably. Dismiss keyboard; terminal returns to full height.
- iOS Safari: rotate device; terminal refits correctly.

---

### Story 5: Frontend Observability

**Goal**: Four key interaction latencies are captured to backend logs via `POST /api/telemetry`. Slow attaches and slow RPCs are diagnosable via `grep`/`jq` on the log file.

**Acceptance criteria**:
- `POST /api/telemetry` endpoint accepts `{event, duration_ms, session_id?, timestamp, labels?}`.
- Backend logs the event as a structured JSON log line at INFO level.
- Session attach latency (mount-to-first-output) is POSTed after every successful attach.
- RPC round-trip for `StreamTerminal` first byte is captured.
- Client call is fire-and-forget; a failed POST does not bubble an error to the user.

#### Task 5.1 — Backend: register `/api/telemetry` route (1–2 hrs)

Files: `server/server.go`, `server/handlers/telemetry_handler.go` (new)

- Register `POST /api/telemetry` on the HTTP mux (plain JSON, not ConnectRPC).
- Handler: decode JSON body, validate fields (event name required, duration_ms required, max 100 label keys).
- Log as: `slog.Info("frontend_telemetry", "event", req.Event, "duration_ms", req.DurationMs, "session_id", req.SessionId, "labels", req.Labels)`.
- Return 204 No Content on success.
- Rate-limit to 100 requests/minute (single user) to prevent runaway logging.

#### Task 5.2 — Frontend: `telemetry.ts` module (1 hr)

Files: `web-app/src/lib/telemetry.ts` (new)

```typescript
export function track(
  event: string,
  durationMs: number,
  labels?: Record<string, string>,
  sessionId?: string
): void {
  const body = JSON.stringify({ event, duration_ms: Math.round(durationMs), session_id: sessionId, timestamp: new Date().toISOString(), labels });
  fetch('/api/telemetry', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body })
    .catch(() => {}); // fire-and-forget; never throw
}
```

#### Task 5.3 — Wire into `TerminalOutput` (1–2 hrs)

Files: `web-app/src/components/sessions/TerminalOutput.tsx`

- After `logTerminalMetrics()` logs to console, call `track('session_attach', metrics.totalLoadTime, { session_id: sessionId })`.
- After `StreamTerminal` first byte received, call `track('stream_terminal_first_byte', connectionDuration)`.
- `metricsRef` already captures all these values — this is a 5-line addition.

#### Task 5.4 — Verify (30 min)

- Attach to a session; `tail -f ~/.stapler-squad/logs/stapler-squad.log | grep frontend_telemetry` shows the event.
- Close browser tab and reopen; confirm no error surfaced in UI.

---

### Story 6: Quick Rename / Retag

**Goal**: Single click on a session title enters inline edit mode. Enter/blur saves. Esc cancels. Optimistic UI update.

**Acceptance criteria**:
- Clicking the session title text in the session card replaces it with a focused `<input>`.
- Enter or blur: calls `RenameSession` RPC, optimistically updates title in local state.
- Esc: cancels edit, reverts to original title.
- If `RenameSession` fails: shows brief error indicator, reverts title.
- Tab from the title input saves and moves focus to next field (if any) or closes edit.

#### Task 6.1 — Inline edit state in `SessionCard` (1–2 hrs)

Files: `web-app/src/components/sessions/SessionCard.tsx`

- Add `isEditing: boolean`, `editValue: string` state.
- On title click: `setIsEditing(true)`, `setEditValue(session.title)`.
- Render: when `isEditing`, render `<input>` instead of `<span>`. Autofocus the input (`autoFocus` prop or `.focus()` in `useEffect`).
- On `onBlur` or Enter keydown: call save handler. On Esc keydown: cancel.
- Guard accidental blur-to-save: use a `mousedown` ref flag to detect if blur was caused by clicking the cancel button (if one exists).

#### Task 6.2 — Save handler with optimistic update (1 hr)

Files: `web-app/src/components/sessions/SessionCard.tsx`

- Save handler: call `renameSession({ id: session.id, newTitle: editValue })` via ConnectRPC client.
- Optimistic update: call `onTitleChange(editValue)` before RPC resolves (parent must support this callback).
- On RPC error: revert `session.title` in parent state, show brief "rename failed" toast or inline red border on the input.
- `RenameSession` RPC is already defined in `proto/session/v1/session.proto` (line 100–102).

#### Task 6.3 — CSS (vanilla-extract) (30 min)

Files: `web-app/src/components/sessions/SessionCard.css.ts` (new or existing)

- Style the inline input to match the title text visually (same font, same line-height, no border by default, subtle focus ring).
- Use `vars` from the theme contract — no hardcoded colors.
- Example: `border: none; background: transparent; outline: 2px solid ${vars.color.actionPrimary}` on focus.

#### Task 6.4 — Verify (30 min)

- Click title: input appears, focused.
- Type new name, press Enter: title updates in card header immediately; RPC fires in background.
- Press Esc: title reverts.
- Disconnect network, try to rename: error indicator appears, title reverts.

---

### Story 7: Bulk Actions

**Goal**: Checkbox multi-select on session cards. Selecting any card reveals a floating bulk action bar (Pause, Stop, Delete, Add Tag).

**Acceptance criteria**:
- Checkbox appears on session card hover (desktop) or always-visible on mobile.
- Selecting one or more cards shows a floating action bar at the bottom of the viewport.
- Action bar shows count of selected sessions and buttons: Pause, Stop, Delete, Add Tag.
- Each action fires the corresponding RPC for all selected sessions (in parallel, with `Promise.allSettled`).
- Deselect all / close action bar clears selection.
- `BulkActions.tsx` is already present — connect it to real selection state.

#### Task 7.1 — Selection state (1–2 hrs)

Files: `web-app/src/store/bulkSelectionStore.ts` (new, or add to existing store)

- Simple set: `selectedIds: Set<string>`, `toggle(id)`, `selectAll(ids)`, `clear()`.
- Use `useState` or a lightweight zustand slice — whichever the codebase already uses for global state.

#### Task 7.2 — Checkbox on `SessionCard` (1–2 hrs)

Files: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionCard.css.ts`

- Add a `<input type="checkbox">` to the card.
- Desktop: visible only on `hover` (CSS `:hover` selector on the card container).
- Mobile: always visible (use `@media (hover: none)` to override).
- Checkbox `onChange`: calls `bulkSelectionStore.toggle(session.id)`.
- When any session is selected, checkbox is visible without hover (so user can uncheck).

#### Task 7.3 — Wire `BulkActions.tsx` (1–2 hrs)

Files: `web-app/src/components/sessions/BulkActions.tsx` (already exists)

- Inspect current `BulkActions.tsx` to understand its existing interface.
- Connect `selectedIds` from store to the component's props.
- Implement action handlers: for each action (Pause, Stop, Delete), call the corresponding RPC for all `selectedIds` in parallel using `Promise.allSettled`.
- On completion: show a brief count summary ("3 sessions paused"), then clear selection.
- Add Tag action: open the existing tag editor modal pre-populated to add a tag to all selected sessions.

#### Task 7.4 — CSS: floating action bar position (30 min)

Files: `web-app/src/components/sessions/BulkActions.css.ts` (new or existing)

- Position: `position: fixed; bottom: 1.5rem; left: 50%; transform: translateX(-50%); z-index: 50`.
- Use vanilla-extract. Use theme tokens for background, border, shadow.
- Hidden when `selectedIds.size === 0`.

#### Task 7.5 — Verify (30 min)

- Hover session cards; checkboxes appear.
- Select 3 sessions; action bar appears at bottom.
- Click "Stop": all 3 sessions stop; action bar shows "3 sessions stopped".
- Press Esc or click X on action bar: selection clears.

---

## Known Issues

### [HIGH] Cursor corruption risk during lazy scrollback Phase 2 prepend

**Description**: When `GetScrollback` is called in Phase 2 and historical ANSI data is prepended to the xterm viewport via `terminal.write()`, any cursor-positioning sequences in the historical data (`ESC[H`, `ESC[10;5H`) will move the internal cursor. Subsequent live output containing relative cursor moves will render at the wrong coordinates.

**Current exposure**: None. Phase 1 (tail-only) does not prepend. This risk activates only when Phase 2 is implemented.

**Mitigation for Phase 2**:
- Send `\u001b7` (DECSC, save cursor) before the historical data write.
- Send `\u001b8` (DECRC, restore cursor) after.
- Alternatively, only prepend while the stream is paused (flow control `paused: true`) to guarantee no cursor moves during the write window.
- Add a unit test that writes cursor-positioning ANSI, scrolls up, prepends, and verifies cursor position is unchanged.

**Files to watch**: `server/services/terminal_stream_handler.go`, `web-app/src/lib/terminal/LazyScrollbackAddon.ts` (Phase 2)

---

### [HIGH] iOS touch scroll: xterm private API usage risk

**Description**: ADR-004 uses `terminal.scrollLines(n)` which is a PUBLIC xterm.js API. However, if a future need arises to access the viewport directly (e.g., for Phase 2 lazy scrollback prepend triggering), the `_core.viewport` private API would be used, which breaks on xterm.js major version bumps.

**Current exposure**: Low — `terminal.scrollLines()` is public and stable.

**Mitigation**:
- Pin the `@xterm/xterm` version in `package.json` with `=` (exact) rather than `^` or `~` when any private API is introduced.
- Check the xterm.js CHANGELOG before upgrading for breaking changes to `_core`.

**Files to watch**: `web-app/src/lib/hooks/useTouchScroll.ts`, `package.json`

---

### [HIGH] Lazy scrollback ANSI prepend may produce line-count drift

**Description**: xterm.js counts scrollback in rendered lines, not bytes. ANSI sequences that use soft-wrapping or carriage returns can cause a single `ScrollbackEntry` to produce multiple rendered lines. Prepending N `ScrollbackEntry` items does not guarantee N visual lines are added, which means the scroll position arithmetic in Phase 2 will drift.

**Mitigation for Phase 2**:
- Do not rely on entry-count for scroll position. Use the `terminal.buffer.active.baseY` value before and after prepend to compute actual line delta.
- Test with output from `git diff` (contains ANSI color codes and long lines that soft-wrap).

---

### [MEDIUM] `git for-each-ref` path traversal injection

**Description**: The `ListBranches` handler receives `repoPath` from the client. If path validation is insufficient, a malicious value like `../../etc` could cause the `git -C` flag to operate on an unintended directory.

**Mitigation** (required in Task 1.2):
- Call `filepath.Abs(repoPath)` to normalize the path.
- Check that the result is under one of the allowed workspace roots (the same set used by session creation).
- Return `codes.InvalidArgument` if the path fails validation; never pass unvalidated input to the shell command.

**Files**: `server/services/session_service.go` (or `branch_service.go`)

---

### [MEDIUM] Branch autocomplete race: stale results for previous repo

**Description**: User opens SessionWizard, types a repo path, then immediately changes to a different path. If the first `ListBranches` request resolves after the second, the stale results from the first repo overwrite the current results.

**Mitigation** (required in Task 1.3):
- Use `AbortController` in `useBranchSuggestions.ts`. On each new `repositoryPath` value, call `controller.abort()` on the previous controller before creating a new one.
- The existing `useEffect` cleanup function is the correct place for this.

**Files**: `web-app/src/lib/hooks/useBranchSuggestions.ts`

---

### [MEDIUM] iOS 15: `visualViewport.resize` not reliable for keyboard events

**Description**: Safari 15 does not reliably fire `visualViewport.resize` when the virtual keyboard appears. The `dvh` CSS unit approach (Task 4.2) handles this at the CSS layer, but the `terminal.fit()` call in Task 4.1 may not trigger on iOS 15.

**Mitigation**:
- `dvh` CSS units are the primary fix (supported iOS 15.4+). The `visualViewport` listener is a secondary enhancement.
- For iOS 15.0–15.3: layout jank may still occur. Acceptable given the audience (no users confirmed on iOS 15.0–15.3).
- Document in the PR: "iOS 16+ is fully supported; iOS 15.4+ gets `dvh` layout fix but may not get the fit() re-render on keyboard events."

---

### [MEDIUM] Bulk actions: partial failure handling

**Description**: When the bulk "Stop" or "Delete" action fires RPCs for multiple sessions in parallel via `Promise.allSettled`, some RPCs may fail while others succeed. The UI must not silently swallow failures or show misleading success counts.

**Mitigation** (Task 7.3):
- Use `Promise.allSettled` (not `Promise.all`) so that one failure does not prevent others.
- Count results: `{succeeded: N, failed: M}`.
- If any failed: show "N stopped, M failed" with an error color. Do not clear selection for the failed sessions so the user can retry.

---

### [LOW] Safari 15: `overscroll-behavior: contain` not supported

**Description**: `overscroll-behavior: contain` on the xterm container prevents parent scroll propagation on iOS 16+. iOS 15 users will still experience parent-scroll propagation when swiping near the terminal edges.

**Mitigation**: The JS `touchmove` + `event.preventDefault()` handler in `useTouchScroll.ts` handles this as the primary mechanism. The CSS property is belt-and-suspenders. iOS 15 scroll chaining is partially mitigated by `preventDefault()`.

---

## Integration Checkpoints

After each story is complete and before opening a PR:

1. `make lint` passes (lint is part of the build gate).
2. `make test` passes for all packages touched.
3. `make restart-web` succeeds; manually smoke-test the feature.
4. Any new CSS in `.css.ts` files uses `vars` from the theme contract (no hardcoded hex values).
5. Any new or edited `.module.css` files use only tokens defined in `globals.css`.

---

## Context Preparation Guide

Before starting implementation of each story, open a fresh session with this context:

**Story 1 (Branch autocomplete)**:
- Read: `proto/session/v1/session.proto` (RPC definitions)
- Read: `server/services/session_service.go` (existing handler pattern)
- Read: `web-app/src/lib/hooks/useBranchSuggestions.ts` (replace this)
- Read: `web-app/src/components/ui/AutocompleteInput.tsx` (no changes needed, understand interface)
- Read: ADR-002

**Story 2 (Lazy scrollback)**:
- Read: `session/scrollback/buffer.go` (understand `GetLastN`)
- Find and read: the `StreamTerminal` handler (search for `GetAll()` or `scrollback` in `server/services/`)
- Read: ADR-001

**Story 3 (Mobile touch scroll)**:
- Read: `web-app/src/components/sessions/XtermTerminal.tsx` (understand `containerRef`, `terminalRef`)
- Read: ADR-004

**Story 4 (Mobile layout)**:
- Read: `web-app/src/components/sessions/TerminalOutput.tsx` (understand `metricsRef`, existing resize logic)
- Read: `web-app/src/components/sessions/TerminalOutput.module.css` (or equivalent CSS file for terminal height)

**Story 5 (Observability)**:
- Read: `server/server.go` (understand how routes are registered)
- Read: `web-app/src/components/sessions/TerminalOutput.tsx` lines 40–80 (`metricsRef` already has all measurements)
- Read: ADR-003

**Story 6 (Quick rename)**:
- Read: `web-app/src/components/sessions/SessionCard.tsx` (understand existing card structure)
- Read: `proto/session/v1/session.proto` lines 100–102 (`RenameSession` RPC)

**Story 7 (Bulk actions)**:
- Read: `web-app/src/components/sessions/BulkActions.tsx` (understand existing interface)
- Read: `web-app/src/components/sessions/SessionCard.tsx` (add checkbox here)

---

## Success Criteria

- [ ] **Story 1**: Branch dropdown in SessionWizard shows actual git branches for the selected repo path within 200ms (cached) or 2s (uncached).
- [ ] **Story 2**: Sessions with >500 scrollback lines attach and display first output within 1 second.
- [ ] **Story 3**: Touch-scrolling inside the terminal viewport works on iOS Safari and Android Chrome; text selection via long-press is preserved.
- [ ] **Story 4**: Terminal height does not visibly shrink when the iOS virtual keyboard appears. Terminal refits correctly after keyboard dismisses.
- [ ] **Story 5**: Session attach latency appears as structured JSON in `~/.stapler-squad/logs/stapler-squad.log` after every terminal attach.
- [ ] **Story 6**: Single click on session title enters inline edit mode; Enter saves; Esc cancels; optimistic update is immediate.
- [ ] **Story 7**: Checkboxes appear on hover; selecting 2+ sessions reveals bulk action bar; Pause/Stop/Delete/Add Tag work on all selected sessions.
- [ ] All stories: `make lint` and `make test` pass. No new hardcoded CSS values.
