# UX Design: frontend-responsiveness

Labels: VERIFIED = file opened this session (path:line); INFERRED = reasoning, not checked. Plan references are to `implementation/plan.md` stories. Requirements: `requirements.md`; research: `research/ux.md`.

## Verified current behavior the design builds on

| Fact | Source |
|---|---|
| A window switch is `router.replace("/?window=<id>")`: a search-param change, pathname stays `/` | `web-app/src/lib/window/useWindowUrlSync.ts:71-77` |
| Focus-move and announcer both fire only on a pathname change, never on search or hash changes | `lib/nav/usePathnameChange.ts:5-6`, `lib/nav/useFocusOnNavigate.ts:9`, `lib/nav/RouteAnnouncer.tsx:22` |
| Today a window switch therefore produces **no announcement and no focus move** (gap confirmed, was INFERRED in research) | same files |
| Tab strip is `role="tablist"` / `role="tab"` with `aria-selected` and roving `tabIndex` (APG tabs, automatic activation). Arrow/Home/End call `onSwitch` **and** focus the tab node | `components/window/WindowTabStrip.tsx:262-265, 430-445` |
| Enter/Space on a tab is an explicit `onKeyDown` branch calling `onSwitch` on a `<div role="tab">` (not a `<button>`) | `components/window/WindowTabStrip.tsx:251-255` |
| No `role="tabpanel"` / `aria-controls` exists for the window content | grep of `components/window`, `app/page.tsx`: no matches |
| Active tab highlight derives from `currentWindowId`, which derives from the URL param, so it cannot update before `router.replace` commits | `useWindowUrlSync.ts:55` (`resolveCurrentWindowId`) |
| Leader: Alt+W, then 1-9/n/p/,/o/Escape, 3 s timeout, capture-phase catch-all disarm | `lib/window/useWindowShortcuts.ts:39-44, 118-150` |
| Route skeleton shows after `PENDING_DELAY_MS = 150`; announcer speaks after 100 ms, clears after 1000 ms | `lib/nav/policy.ts:5-12` |
| `RoutePending` has three skeleton blocks and no stall, timeout or error state. Its comment claims `aria-busy` on the layout `main`, but `RootShell.tsx:25` sets none | `app/RoutePending.tsx:5`, `app/RootShell.tsx:25` |

## Surfaces

| # | Surface | Type |
|---|---|---|
| S1 | Window tab strip: optimistic switch | Interactive |
| S2 | Window content region: retained, hidden and evicted windows | Interactive |
| S3 | Window-switch announcement and focus | Interactive (a11y) |
| S4 | Backlog list: cached, refreshing, out-of-date, cold | Interactive |
| S5 | `RoutePending` stall escalation | Interactive |
| S6 | Nav-link prefetch | Non-interactive |
| S7 | Perf marks and `window.__ssqPerf` | Non-interactive |

---

## S1. Window tab strip: optimistic switch

```
 Window switcher (role=tablist)
 [ api ][*dev*][ logs ] [+] [overview]        *x* = active (aria-selected=true)
   1      2      3
 click/keypress -> same frame: highlight moves to target tab
                -> then content reveals (S2)
```

Flow
1. User clicks a tab, presses Alt+W then `1-9/n/p`, uses Arrow/Home/End on the tablist, or swipes (mobile). All route through one `onSwitch(id)`.
2. In the same frame, a local `pendingWindowId` state flips the tab's `aria-selected` and highlight style. Source of truth stays the URL; `pendingWindowId` clears when `currentWindowId` equals it.
3. `performance.mark("ssq:window-switch:start")` fires in the handler (plan 1.1.2b).
4. Retained target: content reveals (S2). Evicted target: pane chrome renders at once, terminals mount lazily (S2).
5. Mark `:end` after double rAF following commit.

Design rules
- Use `aria-selected`, not `aria-current`. Plan Story 4.1.4 AC says `aria-current="true"` on the tab; on `role="tab"` the selected state is `aria-selected`, which the component already uses. Using both would double-announce.
- If a second switch arrives before the first commits, last one wins; `pendingWindowId` is overwritten, never queued.
- The optimistic state must roll back if the target id disappears (window closed in another tab): revert to `currentWindowId`, announce nothing.
- No CSS transition on the highlight or content (instant is the default; `prefers-reduced-motion` needs no special case if there is none).

Errors and edge cases
| Case | Behavior | Exit |
|---|---|---|
| Target window closed concurrently | Highlight reverts to the resolved window in the same frame | Tab strip stays usable |
| `router.replace` throws or is superseded | `pendingWindowId` cleared by an effect on `currentWindowId` settling plus a 2 s safety timer | Next click works |
| Tab is being renamed (inline editor focused) | Digit/n/p leader follow-ups are already ignored in inputs (`useWindowShortcuts.ts:129-136`); unchanged | Escape ends edit |

## S2. Window content region: retained, hidden, evicted

```
 +--------------------------------------------------------+
 | tablist ...                                            |
 +--------------------------------------------------------+
 | <div role=tabpanel aria-labelledby=tab-w2>  ACTIVE     |   visible, focusable
 |    PaneTilingContainer (w2)                            |
 | <div role=tabpanel ... hidden inert>        w1 (kept)  |   display:none + inert
 | <div role=tabpanel ... hidden inert>        w3 (kept)  |
 | (w4 evicted: not in DOM)                               |
 +--------------------------------------------------------+
```

Flow
1. Switching to a retained window: toggle `hidden`/`inert` on wrappers; the target shows its prior scroll offset and layout.
2. Switching to an evicted window (beyond K = 3, plan 4.1.2): render pane chrome and a per-pane placeholder ("Connecting <session title>...") and mount xterm after first paint. No full-page skeleton. The placeholder is **silent**: it is not a live region, so the only announcement of the switch is S3's `Window <n> of <total>: <name>`. If a pane is still not connected after 1 s, `announce()` speaks once per switch ("Still connecting <title>", or "Still connecting <k> sessions"); a failure speaks once the same way. The xterm slot (not the `tabpanel`) carries `aria-busy` while connecting.
3. Hidden windows are `display:none` plus `inert` (not `aria-hidden` alone), so they leave tab order, the a11y tree and pointer events. They contain no live regions that speak.
4. Flag `window_keep_alive` OFF: single container, today's behavior; S1 and S3 still apply (they fix a present bug and do not depend on keep-alive).

Design rules
- Add `role="tabpanel"` with `aria-labelledby` pointing at the tab id and `aria-controls` on each tab. Today none exists (see table); without it the tablist controls nothing for AT users. Needs tab `id`s in `WindowTabStrip.tsx`.
- Per-window focus memory: store the last focused element (or pane id) on deactivation and restore it on reactivation (plan 4.1.2 says "focus restored" but not where it is stored). A deactivating window that holds focus must hand focus off before `inert` is applied, because setting `inert` on a focused subtree drops focus to `body` (INFERRED browser behavior; verify in the Axe/Playwright test).
- A hidden terminal must not receive keys. Verified requirement in `requirements.md` constraints.
- Evicted placeholder must be skeleton-sized to the pane (no layout shift) and cannot persist: if mount fails, show the S5 error pattern in the pane with Retry.

Errors and edge cases
| Case | Behavior | Exit |
|---|---|---|
| Terminal stream fails to open in a retained or restored window | Pane-level message "Could not connect to <title>" + Retry button; other panes unaffected | Retry, or close pane |
| Archived session in a retained window | No auto-start (`noarchivedrevival`); pane shows its existing archived state | Existing Resume action |
| WebGL contexts exhausted (plan 4.1.3, budget 8) | Hidden terminals yield first; a visible terminal never falls back silently to blank | Canvas/DOM renderer fallback |

## S3. Window-switch announcement and focus

Gap (VERIFIED, see table): neither exists for window switches today.

Flow
1. After a switch commits, announce through the existing polite `RouteAnnouncer` region (no second live region): `Window 2 of 3: api`.
2. Move focus per the table below.

| Trigger | Focus after switch | Why |
|---|---|---|
| Tablist Arrow/Home/End | Stays on the target **tab** | APG roving tabindex; the existing handler focuses the tab node (`WindowTabStrip.tsx:444`). Moving to the terminal would break arrowing through tabs |
| Tab Enter/Space (or a synthesized `click` with `detail === 0`) | Stays on the target **tab**; Tab then enters the panel | The strip uses automatic activation (arrows already select), so Enter/Space has no selection job left under APG; one outcome per control keeps keyboard and AT users oriented. Rejected: "move to the pane" (two outcomes for one control) and "manual activation" (changes arrow behavior, out of scope) |
| Mouse click | Target window's active pane/terminal | Matches tmux; the user chose to enter that window |
| Leader Alt+W + 1-9/n/p | Target window's active pane/terminal | Keyboard users are heading into the terminal |
| Touch or pen tap, swipe | Stays on the target tab; terminal is **not** focused | Focusing the terminal opens the on-screen keyboard (see `docs/bugs/open/BUG-110-mobile-terminal-blank-after-keyboard-open.md`); the operator taps the terminal when they want to type |
| Overview picker | Target window's active pane/terminal | Overview closes |
| Leader `,` / `o` | Unchanged (rename editor / overview) | Not a switch |

Design rules
- Plan Story 4.1.4 said "focus moves to the target window's active pane" unconditionally; that conflicts with the Arrow-key row above. Fixed in the plan (trigger-dependent, including the Enter/Space and touch rows).
- Touch targets: under `(pointer: coarse)` every window tab and the S4/S5 Retry and "Go to Sessions" controls are at least 44x44 CSS px.
- The announcer currently only listens to `usePathnameChange`. Add an imperative `announce(text)` (or a small store) that reuses the clear-then-set generation trick (`RouteAnnouncer.tsx:22-36`), so an identical repeated message ("Window 2 of 3: api") still re-announces. Backlog stall text (S5) uses the same channel.
- Message format: `Window <n> of <total>: <name>`. Use the same string when the window name is empty ("Window 2 of 3").
- If focus restore finds no focusable element, focus the window's `tabpanel` wrapper (`tabIndex=-1`).
- Switching to the already-active window: no announcement, no focus move.

Errors and edge cases
| Case | Behavior | Exit |
|---|---|---|
| Target window has zero panes | Focus the tabpanel wrapper; announce as usual | Empty-window affordance already exists |
| Rapid switches (Alt+W 2, Alt+W 3 within 100 ms) | Only the last announcement speaks (generation reset) | None needed |
| Pathname change and window switch in the same tick | Route announcement wins; window announcement dropped | None needed |

## S4. Backlog list: cached, refreshing, out-of-date, cold

```
 Warm (cache hit)                    Refreshing > 1 s            Revalidation failed
 +---------------------------+       +------------------------+  +---------------------------+
 | Backlog        [filters]  |       | Backlog   (spinner)    |  | Backlog  [Out of date]    |
 | card card card ...        |       |   Refreshing...        |  |   Could not refresh.      |
 | (cached list, interactive)|       | card card card ...     |  |   [Retry]                 |
 +---------------------------+       +------------------------+  | card card card ... (stale)|
                                                                 +---------------------------+
 Cold (no cache): skeleton cards sized like real cards, aria-busy=true on the list container
```

Flow
1. Enter `/backlog`: if a persisted snapshot exists (`BacklogStoreHydration`, loaded into the existing `backlogItemsSlice` with `isSnapshot=true`; plan 6.1.2), render rows immediately and start revalidation (the mount-time REST refresh, which also prunes items deleted server-side). Rows are interactive during revalidation.
2. Revalidation < 1 s: no indicator. Result diffs in place; do not reorder rows under the pointer while a card is hovered or focused (apply the diff on next idle frame or keep the focused row's position).
3. Revalidation > 1 s: show a small "Refreshing..." text indicator next to the title (`role="status"`, polite).
4. Revalidation fails: keep stale rows, show an "Out of date" badge with an **age cue** ("Out of date, saved 3 min ago") and a Retry button; message in the polite region ("Backlog could not refresh. Showing list saved 3 minutes ago."; "Showing saved list." when no age is known). Age buckets: "just now" under 60 s, then "N min ago", "N h ago", "N d ago", from the snapshot's `savedAt` or the warm store's `lastSyncedAt`; rendered in a `<time dateTime>` refreshed every 60 s only while the badge is shown, never in a live region.
5. No cache: skeleton cards (reuse the `SessionListSkeleton` pattern, `components/sessions/SessionListSkeleton.tsx`) with `aria-busy="true"` on the list container. After 1 s the S5 stall text appears inside the list area.
6. First screenful renders first; off-screen cards deferred (`content-visibility: auto` or virtualizer). Provide `contain-intrinsic-size` so scroll length is stable.

Design rules
- Cached data is labeled only when it is known stale (failure) or being refreshed (> 1 s), never merely "cached".
- Cache key is versioned and must not render across instances/workspaces (`STAPLER_SQUAD_INSTANCE`): include instance identity or origin in the key. Plan Story 6.1.2 scopes the key by instance/origin **and status filter** (`statusFilterKey`), so a filtered snapshot is never shown for a different filter.
- Mutations (edit, status change) made while showing a stale list must still apply to the server result, then update the cache.
- Badge and indicator text must meet 4.5:1 contrast in all themes (themes load from `~/.stapler-squad/themes/*.json`, so contrast cannot be fixed in one palette; use existing semantic tokens).

Errors and edge cases
| Case | Behavior | Exit |
|---|---|---|
| Cold load fails | Error panel in list area: "Could not load the backlog." + Retry | Retry, or navigate away |
| Cached snapshot corrupt or wrong version | Discard silently, treat as cold | n/a |
| `sessionStorage` unavailable/full | Memory cache only; no user-visible change | n/a |
| Cached list shows an item deleted on server | Row disappears on revalidation. If it held focus, **focus moves to the next row** (the previous row when it was last; the list heading, `tabIndex=-1`, when the list is empty) and the polite region says "<title> was removed" once. No inline "Removed" placeholder (one behavior only) | Focus never lost to `body` |

## S5. `RoutePending` stall escalation

```
 t < 100 ms     nothing (previous page remains)
 t >= 150 ms    skeleton (existing PENDING_DELAY_MS)
 t >= 1 s       skeleton + "Still loading..."  [Retry] [Go to Sessions]
 t >= timeout   "Could not load this page."    [Retry] [Go to Sessions]   (timeout = 15 s, provisional)
```

Flow
1. Navigation starts: `main` gets `aria-busy="true"` (see gap below).
2. 150 ms: skeleton inside `<main>`. 1 s: the stall notice ("Still loading...", Retry, "Go to Sessions") appears in a **sibling element rendered directly after `<main>`** (`data-testid="route-stall"`, laid over the content area), announced once via the polite channel from S3. It is outside the `aria-busy` region on purpose: assistive tech may ignore updates inside a busy region.
3. Retry re-requests the route chunk (and snapshot warm-up) and resets the timers.
4. Transport/chunk timeout (15 s, **provisional**: a chosen default, not measured, owner-revisable from the plan Task 6.1.3c timing runs): the error state replaces the skeleton in the same sibling element and `aria-busy` is removed from `main`. **No `role="alert"`**: focus moves to the error heading (`tabIndex=-1`), whose text is read once on focus; adding an alert as well would speak twice, and an alert without a focus move would leave keyboard users on the nav link with no path to Retry. `announce()` is not used for the error.

Design rules
- Plan Story 6.1.3 says "nothing < 100 ms, skeleton after ~300 ms"; the code is `PENDING_DELAY_MS = 150` (`lib/nav/policy.ts:5`). Pick one: keep 150 (the plan's 300 is a regression of feedback speed). Update the plan. *(Resolved: plan Story 6.1.3 keeps 150 ms.)*
- `aria-busy` is documented in `RoutePending.tsx:5` but not set in `RootShell.tsx:25`. Add it, driven by the pending state; the escalation text and the error state render in the sibling element after `<main>` (flow step 2), not inside `aria-busy` content that AT ignores.
- Every error state has Retry and a second exit ("Go to Sessions") so a persistently failing route is not a dead end.

Errors and edge cases
| Case | Behavior | Exit |
|---|---|---|
| Chunk load fails (offline, deploy mismatch) | Error state at timeout; Retry does a hard reload on second failure | Go to Sessions |
| Navigation superseded by another | Timers reset, escalation text removed | n/a |
| Keyboard focus was on the nav link | Stays there until content commits, then `useFocusOnNavigate` moves it to `main` | n/a |

## S6. Nav-link prefetch (condensed)

Sample: focusing or hovering the "Backlog" nav link requests the route chunk once and runs the hydration-only warm-up once (reads the persisted snapshot into `backlogItemsSlice`), starting no stream and issuing no RPC (`lib/backlog/warmBacklog.ts` via the `@/lib/nav` facade `prefetch`, plan Story 6.1.3a).

- Triggers on hover (with ~100 ms intent delay) and on keyboard focus; both once per link per page life.
- No visible UI; no announcement; failure is silent (the real navigation retries via S5).
- Must not prefetch on touch scroll or on every focus pass during Tab traversal beyond one request per link.
- Warm-up issues no unary RPC and starts no `WatchBacklogItems` subscription (the hook owns the stream lifecycle; a hover-started subscription would be an owner-less second subscriber, plan Story 6.1.3), so it cannot count against the "zero unary requests blocked > 1 s" metric. Items that went stale while away are corrected by the mount-time REST prune (plan Story 6.1.2).

## S7. Perf marks and `window.__ssqPerf` (condensed)

Sample (dev console): `performance.getEntriesByName("ssq:window-switch")` -> `[{duration: 84.3, ...}]`; `window.__ssqPerf` -> `{inFlight: 2, peakInFlight: 6, queuedOver1s: 0, dupCount: {...}}`.

- `ssq:window-switch` and `ssq:route-change` exist in all builds; `__ssqPerf` is a runtime opt-in (`?ssqperf=1` or `localStorage["ssq:perf"]`) present in any build and absent otherwise (`typeof window.__ssqPerf === "undefined"` without the opt-in; e2e runs the embedded production binary, so the opt-in cannot be dev-build-only).
- Marks never alter focus, DOM or announcements.
- Measure ends at the post-commit paint of real content, not the skeleton.
- No user-facing text.

---

## UX acceptance criteria

Window switching (S1-S3)
1. Pressing Alt+W then `2` moves the active-tab highlight to window 2 within the same frame as the keydown (verified by a frame-stepped Playwright test), before its content is visible.
2. Clicking a tab reaches the visible target window in one click; keyboard switching takes two keys (leader + digit), unchanged.
3. The active tab exposes `aria-selected="true"`; no `aria-current` is added alongside it.
4. Each tab has `aria-controls` pointing to a `role="tabpanel"` that has `aria-labelledby` back to the tab.
5. Window switch (no pathname change) produces the polite announcement `Window 2 of 3: <name>` exactly once; no second live region exists in the DOM.
6. Switching twice to the same window name still announces twice (generation reset).
7. After Arrow/Home/End on the tablist, or Enter/Space on a tab, `document.activeElement` is the target tab, not a terminal; the next Tab enters the panel.
8. After a mouse click, leader digit/n/p, or overview pick, focus is inside the target window's last-focused pane; if none, on its tabpanel. (Touch and swipe are exempt: criterion 33.)
9. Switching back to a window restores its prior scroll offset and last-focused pane.
10. With keep-alive on, tabbing through the page never lands inside a hidden window; hidden wrappers have `inert` and `display:none`.
11. With two windows mounted, one keypress in the active terminal is handled exactly once; the hidden window's handlers do nothing.
12. A switch to an evicted window shows pane chrome and per-pane placeholders (no full-page skeleton) and the tab highlight does not wait on terminal mount.
13. Median `ssq:window-switch` <= 150 ms and no long task >= 50 ms on the seeded fixture for a retained window, in the run that gates per `OPEN-DECISION-throttle-gate` (`implementation/plan.md`, Open Decisions; the other CPU-throttle condition is reported).
14. No CSS transition or animation runs on switching, with or without `prefers-reduced-motion`.
15. Alt+W then 4 s idle then `2` does not switch (3 s leader timeout intact).
16. If the target window is closed mid-switch, the highlight reverts to an existing window and nothing is announced.
17. With `window_keep_alive` OFF, S1 and S3 behavior (highlight, announce, focus) is identical; only S2 differs.

Backlog and route loading (S4-S6)
18. With a cached snapshot, Sessions -> Backlog renders the cached rows before any RPC resolves; the user can click a card during revalidation.
19. No "Refreshing" indicator appears when revalidation finishes in < 1 s; it appears after 1 s and disappears on completion.
20. When revalidation fails, stale rows remain, an "Out of date" badge with an age cue ("saved 3 min ago") and a Retry button show, and the polite region says "Backlog could not refresh. Showing list saved 3 minutes ago." (or "Showing saved list." with no known age).
21. With no cache, skeleton cards match real card height (no layout shift on load; CLS contribution 0 from the list) and the list has `aria-busy="true"`.
22. A refresh diff never moves the focused or hovered card out from under the user; when the focused item is removed, focus moves to the next row (previous when last, the list heading when empty) and "<title> was removed" is announced once; focus is never dropped to `body`.
23. Cold-load failure shows "Could not load the backlog." with Retry; Retry succeeds without a page reload.
24. `RoutePending` renders nothing before 150 ms, skeleton at 150 ms, "Still loading..." plus Retry and "Go to Sessions" at about 1 s (announced once, polite), and an error with Retry and "Go to Sessions" at the provisional 15 s timeout, with focus on the error heading and **no `role="alert"`** (the heading is announced once, not twice).
25. `main` carries `aria-busy="true"` during a pending route and not afterward; the stall and error text sit in a sibling element after `main`, outside the `aria-busy` region.
26. Every stall or error state offers Retry or an exit, with no dead ends and no indefinite skeleton.
27. Focusing the Backlog nav link by keyboard triggers one chunk prefetch and one hydration-only warm-up (no stream, no RPC); repeated focus does not repeat them.
28. Hard-loading `/backlog` on the seeded fixture totals < 1 s of long tasks.

Accessibility (all surfaces)
29. Every control added (Retry, Go to Sessions, badge) is keyboard-reachable and has an accessible name; Retry/Go to Sessions show a visible focus ring.
30. Text for "Refreshing", "Out of date", "Still loading" meets 4.5:1 contrast in the default and one custom theme.
31. The Axe route sweep (`tests/e2e/a11y-route-sweep.spec.ts`) reports no WCAG AA violations with 3 retained windows and with the Backlog in each of S4's states.
32. Screen-reader users can complete "switch to window 3 and start typing in its terminal" using only the keyboard, with each step announced or focus-evident. Manual AT check (VoiceOver): VO-Space on a tab leaves focus on the tab, and Tab then enters the panel (confirms the `detail === 0` activation assumption of criterion 7).
33. A touch or pen tap on a window tab (or a swipe) switches windows without focusing the terminal (no on-screen keyboard opens); under `(pointer: coarse)` each window tab, Retry and "Go to Sessions" is at least 44x44 CSS px.
34. Switching to an evicted window announces only `Window <n> of <total>: <name>`; the "Connecting <title>..." placeholder is silent, and "Still connecting ..." is announced once per switch only if a pane is still not connected after 1 s.

## Gaps the plan must fix

1. **Focus rule conflict (Story 4.1.4).** Unconditional "focus moves to the active pane" breaks APG arrow navigation on the tablist. Make focus target trigger-dependent (S3 table). Also: `aria-current` in the AC must be `aria-selected`.
2. **Missing a11y plumbing (Stories 4.1.2, 4.1.4, 6.1.3).** No `tabpanel`/`aria-controls` exists today; `main` has no `aria-busy` despite `RoutePending.tsx:5` claiming it; `RouteAnnouncer` is pathname-only so window and Backlog-stall announcements need a new imperative `announce()`; per-window focus memory and focus handoff before `inert` have no owner task.
3. **Timing and state inconsistencies.** Plan says skeleton at ~300 ms but `PENDING_DELAY_MS` is 150; the optimistic highlight needs local `pendingWindowId` state (the URL-derived `currentWindowId` cannot lead the URL); the S1/S3 fixes are independent of `window_keep_alive` and should ship unflagged; Backlog cache key needs instance/origin scoping; stale-list refresh diffs must not displace the focused row.

### Resolution (Phase 3 repair, 2026-10-26)
All three gaps are encoded in `implementation/plan.md`: gap 1 in Story 4.1.4 (trigger-dependent focus, `aria-selected`); gap 2 in Stories 4.1.2 (tabpanel wrappers, per-window focus memory, handoff before `inert`), 4.1.4 (imperative `announce()`), and 6.1.3 (`aria-busy` on `main`); gap 3 in Stories 4.1.4 (`pendingWindowId`, shipped unflagged), 6.1.3 (`PENDING_DELAY_MS` stays 150 ms) and 6.1.2 (instance/origin-scoped key, focus-stable refresh diff).

### Triad review fixes (2026-10-26)
Decisions encoded here and in the plan: stall and error text render in a sibling of `<main>` outside `aria-busy` (S5, plan Story 6.1.3); the route error moves focus to its heading with no `role="alert"` (one announcement); touch-originated switches never auto-focus the terminal and tabs, Retry and "Go to Sessions" are >= 44 px under `(pointer: coarse)` (S3, criterion 33); Enter/Space on a tab keeps focus on the tab (S3, criteria 7-8); a focused Backlog row removed by the diff hands focus to the next row (S4, criterion 22); "Go to Sessions" is offered from 1 s and the 15 s timeout is provisional (S5); the Out of date badge carries an age cue (S4, criterion 20); the evicted-window placeholder is silent unless it exceeds 1 s (S2, criterion 34).
