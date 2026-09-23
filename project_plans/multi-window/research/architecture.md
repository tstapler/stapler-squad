# Research: window-layer architecture over the pane-tiling engine

## Current architecture (as read)

- `web-app/src/lib/pane/paneTypes.ts` — pure types. `PaneState = { root: PaneNode, focusedPaneId, zoomedPaneId }`. `PersistedPaneLayout` is `{ version: 1, root, focusedPaneId, zoomedPaneId }` — a flat, single-tree shape with no wrapper/collection concept. `PaneAction` is a closed union (`SPLIT_PANE`, `CLOSE_PANE`, `RESIZE_PANE`, `FOCUS_PANE`, `ASSIGN_SESSION`, `ASSIGN_TAB`, `ZOOM_PANE`, `NUDGE_RESIZE`, `SET_PANE_VIEW`, `SWAP_PANES`, `RESET_LAYOUT`, `RESTORE_LAYOUT`, `SPLIT_AND_ASSIGN_SESSION`) — no window concept anywhere in it.
- `web-app/src/lib/pane/paneReducer.ts` (266 lines) — pure `(PaneState, PaneAction) => PaneState`, single-tree only, imports helpers from `paneUtils.ts` (212 lines: id generation, tree walking/replace, adjacency, swap, max-depth guard).
- `web-app/src/lib/pane/usePaneReducer.ts` (135 lines) — the actual integration point. Wraps `useReducer(paneReducer, ...)` and layers three effects on top: (1) one-time restore from `loadPaneLayout()` gated on `sessions !== null`, with a `restoredRef` guard and session-ID repair via `validateAndRepair`; (2) re-validate-on-session-change effect that strips stale `sessionId`s; (3) a 300ms-debounced `savePaneLayout(state)` on every state change, gated by the same `restoredRef`. It also special-cases `RESET_LAYOUT` in a `wrappedDispatch` to clear localStorage synchronously. All three effects and the debounce timer are **per-hook-instance, ref/state-based** — there is exactly one `usePaneReducer()` call in the app today.
- `web-app/src/lib/pane/usePaneLayout.ts` (73 lines) — the localStorage I/O + schema-version gate. `loadPaneLayout()` returns `null` unless `parsed.version === 1` — this is a strict equality check, not `>= 1` or `in [1,2]`, so it's the natural seam for a v1→v2 migration branch.
- `web-app/src/components/pane/PaneTilingContainer.tsx` (366 lines) — the single mount point. Calls `usePaneReducer(sessions)` once, calls `usePaneShortcuts(state, dispatch, containerRef)` once, owns a bunch of local UI state (picker overlays, mobile sheet, peek modal) that also reads `state.root`/`state.focusedPaneId` directly, and renders `<PaneSplitRenderer root={state.root} ... />`. Mounted from exactly one place: `web-app/src/app/page.tsx` (confirmed via grep — the other hits are test files).
- `web-app/src/components/pane/PaneSplitRenderer.tsx` (502 lines) — pure recursive renderer over `PaneNode`, receives `dispatch` and leaf/split styling; not persistence- or lifecycle-aware.
- `web-app/src/lib/pane/usePaneShortcuts.ts` (263 lines) — registers ~12 shortcuts via `useShortcut(id, { key, modifiers, context: "cockpit", action })` against a module-level `registry` (`shortcutRegistry.ts`), each shortcut's `action` closing over `state`/`dispatch` from the single `PaneTilingContainer` instance. `useShortcut` itself is generic (id + `Shortcut` object, register-on-mount/deregister-on-unmount) — it doesn't care how many "owners" call it, so a second layer of shortcuts (window switching) can register through the same `registry` with a distinct id namespace (e.g. `"window.switch-next"`) without touching `usePaneShortcuts.ts`. `ShortcutContext` dispatch logic (`shortcutRegistry.ts:102-104`) already carves out `"cockpit"` as privileged (fires even when a terminal pane has focus) — window shortcuts likely want the same context, which is a config choice, not a code change.

## Integration boundary recommendation

**Recommendation: (a) a new hook wrapping an array of `usePaneReducer`-shaped state, not (b) a restructured single reducer.**

Concretely: a `useWindowManager()` hook that owns `{ windows: { id, name, paneState }[], activeWindowId }`, and for each window keeps a `PaneState` produced by the **existing, unmodified** `paneReducer`. It does not need to literally call `usePaneReducer()` N times (that hook's own `useEffect`-per-instance persistence model doesn't compose cleanly across an array — see below); instead it should call the pure `paneReducer` directly per-window and take over persistence itself, but reuse `paneReducer`/`paneUtils`/`paneTypes` unchanged. This satisfies requirements.md's explicit constraint ("reusing the existing pane engine unmodified") and the feasibility risk called out under "Feasibility Risks."

Tradeoffs weighed:

- **(a) Array of independent pane states + active index** (recommended)
  - Keeps `paneReducer`'s action type and pure-function contract completely untouched — every existing `paneReducer.test.ts` case keeps passing unmodified against a single window's state.
  - `PaneSplitRenderer` and `PaneTilingContainer`'s internals (picker overlays, mobile sheet) keep operating on one `PaneState` at a time — the active window's — so the diff to those 366+502 lines is additive (props stay the same shape), not a rewrite.
  - Persistence naturally becomes "serialize the whole `windows` array + `activeWindowId`" as one localStorage blob — matches the requirement that "all windows... survive a page reload" as a single unit, and is the natural place to hang schema versioning (see below).
  - Re-render cost on switch: only the active window's `PaneState` needs to feed `PaneSplitRenderer`; inactive windows' `PaneState` trees sit inert in the `windows` array and don't re-render anything until switched to — cheap, no risk of the < 100ms perceived-switch SLO being blown by re-rendering N trees.
  - Cost: `usePaneReducer`'s own effects (restore-once, re-validate-on-session-change, debounced save) are written assuming exactly one `PaneState`/one set of refs. They cannot be reused as-is for N windows — the new hook needs its own restore/save/re-validate effects operating over the array, replicating (not calling) that logic. This is expected rewrite surface, not a red flag: `usePaneReducer.ts` is only 135 lines and its logic (restore-once-when-sessions-arrive, debounce-save, re-validate-on-session-list-change) is the right shape to lift almost mechanically into a windows-array version.

- **(b) Single reducer over `{ windows: PaneState[], activeWindowId }`**
  - Would require either (i) teaching `paneReducer` itself to be window-aware (directly violates the "must not modify pane engine behavior" constraint), or (ii) writing an outer reducer that dispatches pane actions into the active window's sub-state by manually re-invoking `paneReducer(windows[activeIndex], action)` — which is *architecturally* the same as (a) (still calls the pure reducer unchanged, per-window) but forces window-management actions (`CREATE_WINDOW`, `CLOSE_WINDOW`, `RENAME_WINDOW`, `SWITCH_WINDOW`) and pane actions into one reducer/action-union, coupling two conceptually separate state machines (confirmed separate by requirements.md itself, which describes windows and panes as two distinct layers/registries) and complicating `RESTORE_LAYOUT`'s existing single-tree semantics.
  - No material benefit for this domain: bounded, "low double digits" window count (per requirements.md's Scalability note) means no reducer-composition/perf case for merging them.

- **(c) Other option considered and rejected**: a Context-based `WindowContext` provider wrapping N separately-mounted `<PaneTilingContainer>` instances (one per window, all mounted, visibility toggled via CSS). Rejected because `PaneTilingContainer` currently registers pane shortcuts and localStorage effects **per mount** (`usePaneReducer`'s refs are per-instance) — mounting N of them means N sets of debounce timers and N `usePaneShortcuts` registrations firing/deregistering against the shared `registry` as windows are created/closed, and N sets of "restore on sessions load" races. It also multiplies the mobile-picker/peek-modal local state N-fold for no benefit, since only one window is ever visible/interactive at a time. Unmount-inactive-windows-on-switch would also defeat the persistence/"exact state" requirement transiently. This is strictly worse than (a) for the same reason `usePaneReducer`'s effects don't compose across an array: the hook was written assuming a single owner.

## Persisted schema evolution (`PersistedPaneLayout` v1 → v2)

Current schema (`usePaneLayout.ts:57-73`) already gates strictly on `parsed.version !== 1` returning `null` on any mismatch — this is the exact seam needed. Recommended v2 shape:

```ts
interface PersistedWindow {
  id: string;
  name: string;
  root: PaneNode;
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;
}

interface PersistedWindowLayoutV2 {
  version: 2;
  windows: PersistedWindow[];
  activeWindowId: string;
}
```

Migration path inside `loadPaneLayout()` (or a new sibling `loadWindowLayout()`):
1. Read raw JSON once.
2. If `parsed.version === 2` (and shape-checks pass), return as-is.
3. Else if `parsed.version === 1` (and shape-checks pass — i.e. it's a valid `PersistedPaneLayout`), wrap it: `{ version: 2, windows: [{ id: generateWindowId(), name: "Window 1", root: parsed.root, focusedPaneId: parsed.focusedPaneId, zoomedPaneId: parsed.zoomedPaneId }], activeWindowId: <that id> }`. This is lossless by construction — every field of v1 maps 1:1 into the sole v2 window.
4. Else (unknown version, malformed JSON, missing fields): return `null` and let the caller fall through to `initialPaneState`/a fresh single window — never throw, matching the existing `try { } catch { return null }` pattern and requirements.md's Risk Control ("fallback that preserves the raw old data untouched if migration parsing fails"). Per requirements.md, log a client-side error on this path (existing logging pattern) rather than silently swallowing it, since it's the one case that could look like unexplained data loss to a user.
5. Do **not** delete/overwrite the raw v1 `localStorage` key until a v2 write succeeds — read v1, construct v2 in memory, write v2 key (can reuse `cockpit.paneLayout` with the new shape, or move to a new key like `cockpit.windowLayout` and leave the old key as an untouched fallback/backup; the latter is safer for the "explicit fallback path" rabbit hole and costs one extra localStorage read).

Testing per requirements.md's Rabbit Holes: a fixture-based test loading a real captured v1 `cockpit.paneLayout` value (multi-split tree, non-null `zoomedPaneId`) and asserting the v2 result is `{ windows: [{ name: "Window 1", root: <same root>, ... }], activeWindowId: <window's id> }` byte-for-byte on the pane-tree fields.

## Hotspot disposition: Extend as-is

`git log --oneline -20 -- web-app/src/lib/pane/ web-app/src/components/pane/` shows the pane module touched in ~18 of the repo's commits, but nearly all are additive feature commits (mobile fixes, session-peek integration, artifacts extraction, tab-strip cleanup) spread across the project's whole history, not a tight cluster of recent churn or repeated bug-fix thrashing on the same lines — no signs of the module being unstable or fought-with. File sizes are moderate (`paneReducer.ts` 266 lines, `paneUtils.ts` 212, `PaneTilingContainer.tsx` 366, `PaneSplitRenderer.tsx` 502) and each already has focused test coverage (`paneReducer.test.ts` at 466 lines, `usePaneReducer.persistence.test.ts`, `usePaneLayout.test.ts`). Recommendation: **Extend as-is** for the pane engine itself (`paneTypes.ts`/`paneReducer.ts`/`paneUtils.ts` — touch zero, per the requirement) combined with an **Isolate via seam** for the new window layer: build `useWindowManager` (or similarly named hook) as a new, separate module (`web-app/src/lib/window/`) that composes the pane engine from the outside rather than editing `usePaneReducer.ts`/`PaneTilingContainer.tsx` in place beyond the minimal prop-threading needed (swap `PaneTilingContainer`'s internal `usePaneReducer(sessions)` call for a `useWindowManager`-supplied `[activeWindowState, activeWindowDispatch]` pair with the same `[PaneState, Dispatch<PaneAction>]` signature, so the 366-line component's body barely changes). This keeps the one file with real complexity (`PaneSplitRenderer.tsx`, 502 lines) completely untouched.
