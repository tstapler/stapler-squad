# UX Research: mobile terminal touch scrolling

Confidence labels: VERIFIED = read in this repo; PRIOR-ART = from general knowledge of platform behavior and common implementations, not re-fetched this session (UNVERIFIED against live sources; check before citing externally).

## 1. Comparable patterns

| Pattern | Source | Notes |
|---|---|---|
| Direction: content follows finger (drag down reveals earlier output; drag up reveals later output) | PRIOR-ART: iOS/Android native scroll, Termux, Blink Shell, JuiceSSH, Chrome | Universal for touch. Inverted ("natural scrolling off") is a trackpad setting only; never use on touch. |
| Normal screen with scrollback: scroll the local buffer | PRIOR-ART: Termux, xterm.js default | Local `scrollLines()` is correct here. |
| Alt-screen / mouse-tracking apps: translate drag to wheel events (SGR mouse 64/65) or arrow/PgUp keys | PRIOR-ART: Termux (alt-screen drag sends arrow keys), iTerm2 "scroll wheel sends arrow keys in alt screen", Blink | This is the established fix for "drag does nothing in vim/less/TUIs". |
| Momentum | PRIOR-ART: native fling uses exponential decay; iOS decel rate ~0.998/ms, Android Chrome fling friction roughly 0.015-0.02 with velocity cap ~8000 px/s; min fling velocity ~50 px/s | Treat as starting constants to tune on device, not requirements. Suggested: sample last ~100 ms of touchmove for velocity, decay v *= 0.95 per 16 ms frame, stop under ~0.05 line/frame. |
| Line quantization | Repo requirement (rabbit hole) | Carry fractional remainder across frames so slow drags and decaying momentum do not stall at <1 line. |

### Disambiguation thresholds
Current code (VERIFIED, `web-app/src/lib/hooks/useTerminalGestures.ts`): `longPressMs` default 400 (L48), scroll commit when `absDy > 15` px (L228), tap = `totalDy < 8` and `elapsed < longPressMs` (L275), double-tap 300 ms / 20 px (L84-85).

PRIOR-ART reference points: Android `ViewConfiguration` touch slop 8 dp, long-press timeout 400 ms (Android default 500 ms), double-tap timeout 300 ms; iOS long-press 500 ms, ~10 pt movement tolerance. The repo's 15 px scroll threshold is above Android's 8 dp slop (about 8 CSS px at dpr-independent dp); slop larger than 8 delays scroll start and feels laggy, but a larger value protects long-press selection. Recommendation: keep long-press 400 ms; consider lowering scroll-commit slop to ~10 px and subtracting the slop from the first delta so there is no initial jump. Rule: movement beyond slop before the long-press timer fires = scroll (cancel timer); timer fires first with movement under slop = selection mode (scroll disabled for the remainder of that touch).

## 2. Mental models
- Job: "glance at what the agent printed without summoning the keyboard." Keyboard covers roughly half the viewport, so reading and typing are competing modes; users enter reading mode when the keyboard is closed.
- Tap-to-focus opens the keyboard (existing behavior, regression anchor). Therefore a drag must never be interpreted as a tap: the tap path is gated by `totalDy < 8` (VERIFIED L275), and the scroll path must also not call `terminal.focus()` or fire a synthetic click. Verify on device that touchend after a scroll does not focus the hidden textarea (browsers can synthesize click after touchend unless `preventDefault` is called on touchend/touchstart).
- Two scrollback layers confuse users: xterm local scrollback vs the TUI's own (Claude Code). User expectation is one continuous "scroll the output" gesture; PgUp/PgDn toolbar keys already trained users that the TUI scrolls. Drag should reach the same layer PgUp/PgDn does.
- Expectation of 1:1 tracking while the finger is down; momentum only after release; touching during momentum stops it (tap-to-stop) and must not register as a tap-to-focus.

## 3. Accessibility
- WCAG 2.5.1 Pointer Gestures (Level A): path-based gestures need a single-pointer alternative. A one-finger drag is a path-based gesture, so a non-gesture alternative is required; the toolbar PgUp/PgDn keys satisfy it provided they remain visible and operable without the keyboard open (verify they are present in the no-keyboard state). Also consider Home/End or "jump to bottom" affordance.
- WCAG 2.5.8 Target Size (Minimum, AA, 24x24 CSS px): toolbar keys should meet it (check, not verified).
- Reduced motion: honor `window.matchMedia('(prefers-reduced-motion: reduce)')`. WCAG 2.3.3 (AAA) concerns motion animation; momentum is user-initiated, so strictly optional, but disabling or sharply shortening inertia under reduced-motion is low cost and expected. Read it at gesture start (not module load) so it responds to setting changes.
- Screen readers: TalkBack touch-exploration remaps one-finger drag; xterm.js accessibility tree is separate. Do not break two-finger scroll gestures handled by TalkBack; ignore multi-touch (`touches.length > 1`) in the handler.
- `touch-action`: setting `touch-action: none` on the terminal container (or calling preventDefault on non-passive touchmove) is needed to stop Chrome pull-to-refresh/overscroll; this removes native pinch-zoom there, so pinch zoom (if used) must be handled in code or left to the toolbar. No `touch-action: none` currently on the terminal in `web-app/src` (VERIFIED by grep: only resize handles/backlog page use it).

## 4. Error states and edge cases
| Situation | Expected behavior |
|---|---|
| Keyboard opens/closes mid-gesture | Cancel gesture and momentum on viewport resize; do not apply stale deltas against new row height. |
| Keyboard open/close redraw (blank terminal) | Visible failure mode is "black with content off-screen"; user's only recovery is close/reopen keyboard or reload. After viewport settles, fit then `terminal.refresh(0, rows-1)`; ideally scroll to keep cursor line visible. Silent recovery preferred over a user-facing error. |
| Rotation | Same as keyboard resize; cancel momentum, refit, refresh. Row count changes, so the quantization remainder must reset. |
| Alt-screen, no scrollback (xterm `buffer.active.type === 'alternate'`, baseY = 0) | Local `scrollLines()` is a silent no-op, which is exactly the "drag does nothing" symptom. Must forward to the app (wheel/PgUp) instead; if the app ignores it, nothing more can be done, so feedback is limited. |
| Mouse tracking on (DECSET 1000/1002/1006) | Forwarding wheel events as SGR mouse reports is the native contract; PgUp/PgDn keys are the fallback. Open question from requirements: which one Claude Code responds to. |
| At top/bottom boundary | No overscroll bounce; stop momentum at the edge. Consider subtle haptic (`navigator.vibrate`) only if desired; not needed. |
| Selection mode active | Drag extends selection, never scrolls. |
| Fast direction reversal mid-fling | New touch cancels momentum immediately. |

## 5. Job-to-be-done
When I am away from my desk and an agent session needs a check, I want to read recent output and scroll back through it with one hand, so I can decide whether to steer it, without a keyboard covering half the screen or a layout that goes blank.
- Functional: scroll both directions, reaching the same history PgUp/PgDn reaches.
- Emotional: trust that the terminal is live (a blank screen reads as "crashed" and prompts reloads).
- Social/other: none.
- Success signals map to requirements: 1:1 tracking, no keyboard open on scroll, 0 blank redraws in 50 cycles.

## Recommendations (for planning)
1. Direction: positive finger dy (down) = reveal earlier output, mapped identically in all modes; add a unit test for both signs.
2. Route by mode: normal buffer with scrollback -> `scrollLines`; alt-screen or mouse tracking -> forward as wheel/PgUp-equivalent. Treat PgUp/PgDn keys as the proven fallback.
3. Coalesce to one scroll dispatch per rAF with a fractional-line accumulator.
4. Gate momentum on `prefers-reduced-motion`; cancel on touchstart, resize, orientationchange.
5. Keep PgUp/PgDn toolbar keys reachable (2.5.1 alternative); do not rely on the gesture alone.
6. Add `touch-action: none`/non-passive preventDefault on the terminal surface to defeat pull-to-refresh, and confirm drag never focuses the textarea.
7. Device-verify constants (slop, friction, velocity cap) on a physical Android Chrome device.
