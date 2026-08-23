# UX Design: InputDropBadge (drop-and-signal for dropped keystrokes)

Backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e` | Requirements Goal 3 / AC3 |
Prior research: `../research/ux.md` | Task breakdown: `../implementation/plan.md`
Story 2.3

Scope: the visible+audible signal shown when input typed while disconnected is
**dropped** (never replayed) because its `MessageQueue` was closed by a
superseded connection generation (Story 2.2). This document does not cover the
drop-detection mechanism itself (Stories 2.1/2.2) — only the user-facing
surface.

---

## Step 1 — Surfaces identified

This one feature has **four** distinct user-facing surfaces, not one:

1. **Badge appearing** — the moment `onInputDropped(count)` fires and the
   visual badge mounts/becomes visible.
2. **Badge holding / auto-dismissing** — the badge's visible duration and its
   exit transition.
3. **Single-keystroke vs. coalesced-episode content** — the badge/announcement
   text differs (singular vs. plural, with count) depending on how many drops
   landed inside the coalescing window.
4. **Screen-reader-only announcement** — a *distinct* surface from the visual
   badge: the assertive live region text, which has its own timing and its own
   failure modes (backgrounded tab, mid-announcement interruption) independent
   of whether the visual badge is even on-screen.

Each is designed below, followed by interaction flow and edge cases, then
testable UX acceptance criteria.

---

## Step 2 — Wireframes and flows

### 2.1 Layout: where the badge sits relative to the terminal

```
┌─────────────────────────────────────────────────────────┐
│ Toolbar (session controls, connection indicator, ...)     │
├─────────────────────────────────────────────────────────┤
│                                                           │
│   $ git status                                           │
│   On branch main                                         │
│   ...                                                    │
│   $ git comm∎                            <- caret, live  │
│                                                           │
│                                                           │
│              ┌───────────────────────────┐               │
│              │ ⚠ 3 keystrokes dropped —  │  <- badge,    │
│              │   reconnecting            │     bottom-   │
│              └───────────────────────────┘     center,   │
│                                                pointer-   │
│                                                events:none│
├─────────────────────────────────────────────────────────┤
│ Mobile keyboard toolbar (if present)                     │
└─────────────────────────────────────────────────────────┘
```

- Portal-rendered to `document.body` (matches `XtermTerminal.tsx`'s
  `copiedToast`, lines 638-657), positioned `fixed`, `bottom: 80px`,
  `left: 50%`, `transform: translateX(-50%)` — reuse the exact same
  coordinates as `copiedToast` (`XtermTerminal.css.ts` `copiedToast` style,
  lines 115-129) so it sits in the same "floating terminal chrome" band users
  already associate with transient, non-blocking terminal feedback (the copy
  toast lives here today).
- `zIndex: zIndex.floatingTerminalUI` (1085, `theme-contract.css.ts:197`) —
  same slot as `copiedToast`, so the two never fight over stacking (they are
  mutually exclusive in practice — copy toast fires on a text-selection copy,
  drop badge fires on a connection-supersede event — but if both were
  hypothetically visible at once, same z-index keeps their layering
  predictable rather than undefined).
- `pointer-events: none` on the badge root (same convention as
  `TerminalOutput.tsx`'s `resizingOverlay`, line ~1531-1534 comment) — the
  badge is never a click target, so clicks/drags pass straight through to the
  terminal underneath it.
- Rendered as a sibling to `unavailableOverlay`/`resizingOverlay` inside
  `styles.terminal` in `TerminalOutput.tsx` (~line 1516-1542) per plan Task
  2.3.3 — but note: those two overlays are absolutely positioned *inside* the
  terminal container, while `copiedToast`'s pattern (which this design
  follows) is a `fixed`-position portal to `document.body`. Recommend the
  `fixed`/portal approach (matching `copiedToast`, not the in-container
  overlays) so the badge remains visible even if the terminal container is
  short, scrolled, or the badge's content is wider than the container —
  **flagged as a plan correction**, see Step 4.

### 2.2 Phase timeline: appear → hold → auto-dismiss

```
t=0ms                                          t=+400ms   t=+8400ms   t=+8700ms
  │                                                │           │           │
  │  keystroke(s) dropped, MessageQueue.close()    │           │           │
  │  returns droppedCount > 0                      │           │           │
  │                                                │           │           │
  ├─ onInputDropped(n) fires ──────────────────────┤           │           │
  │  (coalescing window starts/extends)            │           │           │
  │                                                │           │           │
  │                                     [coalescing window closes,          │
  │                                      episode count finalized]           │
  │                                                │           │           │
  │                                                ├─ BADGE MOUNTS ────────┤
  │                                                │  fade-in (~150ms)     │
  │                                                │  "⚠ N keystroke(s)    │
  │                                                │   dropped —           │
  │                                                │   reconnecting"       │
  │                                                │  + <LiveRegion        │
  │                                                │    role="alert"       │
  │                                                │    politeness=        │
  │                                                │    "assertive" />     │
  │                                                │    fires ONCE here    │
  │                                                │                       │
  │                                                │   [visible, holding]  │
  │                                                │                       │
  │                                                │                       ├─ fade-out (~150ms)
  │                                                │                       │  auto-dismiss,
  │                                                │                       │  no ack needed
```

- **0–400ms (coalescing window)**: matches plan Task 2.3.3's 400ms debounce.
  No visual change yet — the badge does not flicker in per-keystroke; it
  waits for the episode to close.
- **400ms mark**: episode finalized (count frozen), badge mounts and the
  `LiveRegion` text is set exactly once with the final count.
- **400ms → 8400ms (hold, 8000ms total per `toastAutoCloseMs`'s
  non-actionable default in `lib/notification-policy.ts:38-42`)**: badge
  stays visible, `LiveRegion` text is *not* re-announced (the live region's
  content doesn't change during hold — see Step 3 caveat on
  `useLiveRegion()`'s auto-clear-after-1s behavior, which this component must
  not blindly reuse as-is).
- **8400ms → 8700ms**: fade-out (mirrors `copiedToast`'s existing
  `fadeInOut` keyframe shape, `XtermTerminal.css.ts:106-111`, but this badge's
  hold portion is ~8s vs. `copiedToast`'s ~1.5s total — do not reuse the
  keyframe's baked-in duration, only its easing/transform shape).

### 2.3 Interaction flow: 1 drop vs. N coalesced vs. overlapping episodes

**Case A — single dropped keystroke:**
```
keystroke dropped (n=1)
  → 400ms coalescing window closes with count=1
  → badge text: "1 keystroke dropped — reconnecting"
  → LiveRegion announces: "1 keystroke dropped while reconnecting" (once)
  → auto-dismiss after 8s
```

**Case B — several keystrokes dropped within the coalescing window:**
```
keystroke dropped (n=1) ─┐
keystroke dropped (n=2)  ├─ all within 400ms of the first
keystroke dropped (n=3) ─┘
  → coalescing window closes with count=3
  → badge text: "3 keystrokes dropped — reconnecting"
  → LiveRegion announces once: "3 keystrokes dropped while reconnecting"
  → auto-dismiss after 8s from episode close (not from the first drop)
```

**Case C — a second drop episode starts while the first badge is still
visible** (e.g. flap → recover → flap again within the 8s hold window):
```
Episode 1: badge showing "2 keystrokes dropped", t=1200ms..9200ms
                                              │
New drop arrives at t=5000ms (Episode 2 starts, count=1 so far)
                                              │
                              400ms later, at t=5400ms, Episode 2 closes
                              with its own count (say 1)
                                              │
  → Episode 2 REPLACES Episode 1's displayed count/text immediately
    (badge text updates to "1 keystroke dropped — reconnecting",
    NOT "3" — Episode 1's already-announced count is not carried forward)
  → LiveRegion re-announces (role="alert" re-fires) for Episode 2,
    because it is a semantically distinct event ("something new just
    happened"), even though the badge never fully disappeared between
    episodes
  → Episode 2's own 8s auto-dismiss timer starts fresh at t=5400ms
    (does NOT inherit/extend Episode 1's remaining hold time)
```
Rationale for "replace + re-announce, don't merge counts": merging counts
across episodes silently invents a number the user never saw a matching
badge for (e.g. if Episode 1 already displayed and was read aloud as "2",
then silently becoming "5" without a new announcement misinforms both sighted
and screen-reader users about what just happened vs. what happened earlier).
Each drop episode is a discrete, independently-true event; the badge always
reflects only the most recent one.

### 2.4 Screen-reader-only announcement — a distinct surface

The `LiveRegion` text update is decoupled from the visual badge's mount
lifecycle in one important way: **the DOM node must be always-present and
stable** (never mount/unmount the `<div role="alert">` itself — only its text
content changes), per `research/ux.md` §3's explicit recommendation, because
AT (NVDA/JAWS/VoiceOver) reliably detect content mutations on a stable node
far better than newly-mounted elements. This means:
- The `LiveRegion` (with `role="alert"`, Task 2.3.1's new prop) should be
  rendered **unconditionally** by `InputDropBadge` (always in the DOM,
  content empty by default), not gated behind the same `visible` boolean that
  controls the *visual* badge's opacity/display. The visual fade-out at 8s
  need not clear the live-region text (nothing re-announces old text on an
  unrelated re-render, since `aria-live` only fires on a content *change*).
- This is why Step 1 calls the screen-reader announcement a separate surface
  from the visual badge — they share a trigger (`onInputDropped`) but have
  independent DOM lifecycles.

---

## Step 3 — Edge cases

### 3.1 Does the badge steal focus while the user is actively typing?

**No, by design, and this must be verified, not assumed.** The badge/portal
must never call `.focus()`, must not be a `<button>`/interactive element, and
must not be inside a tab order (no `tabIndex`). `role="alert"` on the live
region is sufficient for AT to interrupt-and-announce **without** moving DOM
focus — this is the documented behavior distinction between `role="alert"`
and things like a native `alert()` dialog or a focus-trapping modal.
Concretely:
- `InputDropBadge`'s root element must not have `tabIndex="0"` or an
  `autoFocus` prop.
- The xterm.js instance's own DOM focus (wherever the user's cursor/caret
  currently is) is untouched — verify with a Jest/RTL test that asserts
  `document.activeElement` is unchanged before/after `onInputDropped` fires
  (Task 2.3.4's test file is the natural home for this assertion; it is not
  currently listed in the plan's task description — flagged in Step 4).

### 3.2 What if the drop happens while the tab is backgrounded?

Two sub-questions, since "backgrounded" has two failure surfaces:

- **Visual badge**: if the tab is backgrounded when `onInputDropped` fires,
  the badge still mounts/updates in the DOM (React state updates are not
  paused by backgrounding — only rAF/timers *may* be throttled by the
  browser, which affects the fade-in/out *animation smoothness*, not whether
  the badge is present). When the user returns to the tab, if the badge's
  8-second window hasn't elapsed yet, they see it normally. If backgrounding
  caused the browser to throttle the `setTimeout` driving auto-dismiss (Chrome
  throttles background timers to ~1/min), the badge may still show slightly
  past its nominal 8s on return — this is an acceptable, non-harmful skew (a
  stale-but-still-relevant notice lingering a bit longer than intended is
  strictly safer than it disappearing before the user ever saw it).
- **Live region announcement**: this is the one place where "lost, not just
  delayed" is a real risk. Screen readers only announce `aria-live` regions
  while assistive technology is actively attached to a **foregrounded,
  rendered** page in most implementations — a backgrounded browser tab
  generally does not vocalize at all (this matches how VoiceOver/NVDA behave
  with any background tab, not something specific to this component). If the
  content changes again before the user returns (e.g., a second drop episode
  per §2.3 Case C overwrites the first while backgrounded), **the first
  episode's announcement is permanently lost** — there is no queue/replay for
  screen-reader announcements. This is analogous to, and no worse than, how
  this project already treats other assertive announcements (e.g.
  `NotificationToast`'s `approval_needed` type also only vocalizes to an
  attached AT on a foregrounded tab) — **acceptable, not a regression to
  fix**, but worth stating explicitly so it isn't mistaken for a bug later:
  the visual badge (which persists across the backgrounding, per above) is
  the durable record of "something happened," and the live region is
  best-effort/real-time only.

### 3.3 What if `onInputDropped` fires with `count === 0`?

Not a real scenario given `MessageQueue.close()`'s contract (Task 2.1.1 only
returns a positive `droppedCount`), but `InputDropBadge`/its caller in
`TerminalOutput.tsx` (Task 2.3.3) should defensively no-op on `count <= 0`
rather than mounting an empty/misleading badge — cheap guard, worth stating
explicitly since `onInputDropped` is threaded through `disconnect()` too
(Task 2.2.4), and a normal user-initiated disconnect with an empty queue
should never surface a badge.

---

## Step 4 — Flagged changes to plan.md's Story 2.3

1. **Rendering location: `fixed`-position portal, not an absolutely-positioned
   in-container overlay.** Task 2.3.3 currently describes rendering
   `InputDropBadge` "alongside the existing `unavailableOverlay`/
   `resizingOverlay` elements (~line 1516-1542)" — those two are
   `position: absolute` *inside* `styles.terminal`. Task 2.3.2 (correctly)
   says to model the badge on `copiedToast`, which is `position: fixed` and
   portal-rendered to `document.body`. These are two different positioning
   strategies and the plan's wording could be read as "put it in the same
   container," which would clip/misposition a `fixed`+portal element. **Fix**:
   Task 2.3.3 should say the badge is rendered as a `createPortal` sibling
   near (not inside) the other overlays — same DOM insertion point as
   `copiedToast`'s portal in `XtermTerminal.tsx`, not the `styles.terminal`
   overlay stack.
2. **`toastAutoCloseMs` signature mismatch.** Task 2.3.2 says "Auto-dismiss
   after `toastAutoCloseMs` (import from
   `web-app/src/lib/notification-policy.ts`...)" but
   `toastAutoCloseMs(type: NotificationData["notificationType"])` takes a
   notification-type argument and a dropped-keystroke event has no
   `NotificationData` type to pass in. **Fix**: either (a) call
   `toastAutoCloseMs(undefined)` / a type that resolves to the function's
   fallback branch (returns `8_000` for anything that isn't actionable or
   `error`/`task_failed`), with a comment explaining the reuse, or (b) skip
   the function and import/reference the literal `8_000` with a comment
   pointing at `notification-policy.ts`'s default branch as the source of
   truth, so the two values can't silently drift. Prefer (a) if the type
   system allows passing a literal string not in the real union without
   fighting TypeScript; otherwise (b). Either way, do not hardcode a bare
   `8000` with no reference to the policy file, and do not attempt to add a
   new `NotificationType` variant just for this — that would pull the badge
   into the `NotificationToast` system this design (and `research/ux.md`)
   explicitly avoids.
3. **`LiveRegion` mount lifecycle must be unconditional, not gated on
   `visible`.** Task 2.3.2's prop shape (`{ count: number; visible: boolean }`)
   doesn't say whether the nested `<LiveRegion>` is conditionally rendered
   alongside the visual badge. Per Step 2.4 above, the `LiveRegion` element
   must always be present in the DOM (empty string when idle) — only its
   `message` prop changes — independent of the visual badge's `visible`
   fade state. **Fix**: `InputDropBadge` should render `<LiveRegion
   role="alert" politeness="assertive" message={announcementText} />`
   unconditionally, and drive `announcementText` from a value that's cleared
   independently of the visual `visible` prop's timer (do not reuse
   `useLiveRegion()`'s hook as-is without checking its 1-second
   auto-clear — see item 4).
4. **`useLiveRegion()`'s built-in 1-second auto-clear is wrong for this use
   case and must not be reused verbatim.** `LiveRegion.tsx`'s existing
   `useLiveRegion()` hook (lines 33-43) clears its message via
   `setTimeout(() => setMessage(""), 1000)`. That's fine for one-shot
   announcements that don't need to persist, but if `InputDropBadge` reuses
   this hook unmodified, a second drop episode arriving 1-5 seconds after the
   first (well within the first badge's 8s visible hold, per §2.3 Case C)
   would only correctly re-announce if the message *changes* — which it
   does, since the count differs — so this specific hook behavior happens to
   still work for Case C. However, if two consecutive episodes ever produced
   the **exact same count** (e.g. "1 keystroke dropped" twice in a row), the
   live-region text would be identical both times, and because React only
   fires `aria-live` on a DOM text *mutation*, the second identical-text
   announcement could silently fail to be read aloud by some AT. **Fix**: add
   a defeat-dedup mechanism (e.g. append a zero-width space or an
   incrementing hidden counter to the announced string) so consecutive
   same-count episodes still produce a DOM mutation and are each announced —
   this is a known, documented ARIA live-region gotcha, not a new invention.
   Add a Jest test for "two consecutive one-keystroke-dropped episodes both
   announce" to Task 2.3.4's test list.
5. **Task 2.3.4's test list is missing a focus-safety assertion.** Add "badge
   appearing does not move `document.activeElement`" as an explicit test
   case (Step 3.1 above) — the plan's current test list (role/aria-live,
   singular/plural text, auto-dismiss timing) doesn't cover this, and it's
   the concrete way AC3's "never requires acknowledgment to dismiss" /
   non-focus-trapping guarantee gets verified rather than assumed.

No change recommended to the **400ms coalescing window** or the **8-second
auto-dismiss duration** themselves — both are well-justified by
`research/ux.md` (300-500ms debounce range; reuse of the existing
non-actionable toast default) and this design's Step 2 timeline confirms
they compose correctly (coalescing closes, *then* the 8s hold starts, so the
effective on-screen minimum is ~8.4s and the numbers don't need retuning).

---

## Step 5 — UX Acceptance Criteria

### Visual

- **UX-AC-1**: The badge becomes visible within **500ms** of the triggering
  keystroke drop (400ms coalescing window + ≤100ms render/mount), for both
  the single-keystroke and coalesced-episode cases.
- **UX-AC-2**: The badge is positioned (`fixed`, `bottom: 80px`,
  horizontally centered, `zIndex: zIndex.floatingTerminalUI`) so that it never
  overlaps the terminal's current cursor row in the default viewport size
  (≥600px tall) — verified by a Playwright screenshot test asserting the
  badge's bounding rect does not intersect the xterm cursor's bounding rect
  at time of appearance.
- **UX-AC-3**: The badge auto-dismisses within **8–8.5 seconds** of becoming
  visible (matching `notification-policy.ts`'s non-actionable
  `toastAutoCloseMs` default of 8000ms, ± the ~150-300ms fade transition),
  with no user action required.
- **UX-AC-4**: Badge text is singular for `count === 1` ("1 keystroke
  dropped — reconnecting") and plural with the exact count for `count > 1`
  ("N keystrokes dropped — reconnecting").
- **UX-AC-5**: A second drop episode starting while the first badge is still
  visible replaces the displayed count/text with the new episode's own count
  (not a running total) and restarts the 8-second hold from the new episode's
  close time.

### Screen reader

- **UX-AC-6**: The drop event is announced via a `role="alert"` element with
  `aria-live="assertive"` (via `LiveRegion`'s new `role` prop, Task 2.3.1)
  **exactly once per coalesced episode** — not once per dropped keystroke,
  and not zero times for a 3-keystroke episode (single-episode-single-
  announcement, verified with a Jest test asserting the live-region's text
  content mutates exactly once per `onInputDropped` call cluster within the
  400ms window).
- **UX-AC-7**: Two consecutive episodes with an identical count (e.g. "1"
  then "1" again) each still produce an independent, detectable DOM text
  mutation on the live-region node (per Step 4 item 4's dedup fix) — verified
  by asserting the underlying text node's content differs between the two
  announcements even though the human-readable count is the same.
- **UX-AC-8**: The `LiveRegion` DOM node itself is never unmounted/remounted
  across the badge's appear→hold→dismiss cycle — only its text content
  changes (verified via a stable `data-testid`/ref identity check across the
  full lifecycle in a Jest test).

### No dead ends

- **UX-AC-9**: The badge is never a focus target — `document.activeElement`
  is unchanged immediately before and after `onInputDropped` fires, whether
  or not the terminal currently has focus.
- **UX-AC-10**: The badge has no interactive elements (no button, no
  dismiss "X", no link) and `pointer-events: none` is set on its root, so
  clicks/drags in the badge's screen region pass through to the terminal
  underneath at all times during the badge's visible window.
- **UX-AC-11**: The badge never requires any user action (click, keypress,
  focus) to disappear — it is removed purely by the auto-dismiss timer.

### Contrast / keyboard

- **UX-AC-12**: Badge text (`vars.color.warningText` on
  `vars.color.warningBg`, matching the existing warning-toast token pairing)
  meets **4.5:1** contrast in both the light theme (`#92400e` on `#fef3c7` —
  computed ≈6.8:1) and the dark theme (`#fbbf24` on `#78350f` — computed
  ≈5.4:1) per WCAG 2.1 formula — verified with an automated axe-core /
  Lighthouse contrast check in addition to the manual computation recorded
  here.
- **UX-AC-13**: The badge is not part of the tab order (`tabIndex` absent/
  not `0`) and receives no `:focus` styling, confirming keyboard navigation
  is genuinely not applicable to this non-interactive element (this is a
  confirmation, not a keyboard-operability requirement — there is nothing to
  operate).
- **UX-AC-14**: No focus trap: a Tab-key press while the badge is visible
  moves focus exactly where it would have moved with the badge absent
  (verified by comparing `document.activeElement` after Tab with the badge
  visible vs. a control run with it absent).

---

## Summary of plan.md deviations (see Step 4 for full detail)

| # | Story 2.3 item | Recommended change |
|---|---|---|
| 1 | Task 2.3.3 wording | Portal to `document.body` near `copiedToast`, not inside the `styles.terminal` absolute-overlay stack |
| 2 | Task 2.3.2 `toastAutoCloseMs` call | Needs a concrete argument/fallback strategy since the function requires a `NotificationData["notificationType"]` |
| 3 | Task 2.3.2 `LiveRegion` usage | Must render unconditionally (not gated on `visible`) |
| 4 | `useLiveRegion()` reuse | Add a same-count-episode dedup mechanism so consecutive identical-count announcements both fire |
| 5 | Task 2.3.4 test list | Add a focus-safety ("no focus steal") assertion |

Coalescing window (400ms) and auto-dismiss timing (8s) are confirmed correct
as currently specified — no change recommended there.
