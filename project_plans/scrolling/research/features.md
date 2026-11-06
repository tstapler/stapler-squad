# Research: Features (touch-drag scroll landscape and edge cases)

Confidence labels: VERIFIED = code opened in this repo. INFERRED = from general knowledge of the apps/specs; not re-checked online in this pass (no web lookups were run).

## 1. What the existing code does (VERIFIED)

### PgUp/PgDn toolbar keys
- `web-app/src/components/sessions/TerminalOutput.tsx:1984` sends `'\x1b[5~'` (PgUp) and `:2018` sends `'\x1b[6~'` (PgDn) via `sendKey()` (`:792`).
- `sendKey` applies sticky modifiers (Ctrl/Alt/Shift map to `\x1b[5;5~`, `\x1b[5;3~`, `\x1b[5;2~` etc., `:15-42`), then `handleTerminalData(data)`, i.e. the same path as real keystrokes: raw bytes to the PTY/tmux pane.
- So the keys are plain CSI PageUp/PageDown input delivered to the foreground program. They work with Claude Code because the TUI itself interprets them and scrolls its own transcript. They do not touch xterm's scrollback at all.

### Drag gesture
- `useTerminalGestures.ts:217-257`: PENDING -> SCROLLING after `|dy| > 15px`; each touchmove goes through `rafThrottlePoint`; handler computes `lines = Math.round(-moveDy / cachedCellH)` and calls `terminal.scrollLines(lines)` (`:245-246`). Nothing is ever sent to the PTY.
- `XtermTerminal.tsx:908,922,957` also use `scrollLines`, but only for the custom scrollbar track/thumb (not gestures).
- `isMouseTracking()` (`lib/terminal/mouseTracking.ts`) is consulted only for tap and selection, never for scroll. There is no alt-screen check anywhere in the gesture path (only a `TerminalStreamManager.ts:380` detector for alt-screen exit).
- No `touch-action` or `overscroll-behavior` is set on the terminal container (grep of `web-app/src` finds only unrelated components: ResizeHandle, backlog, log viewers).

## 2. Why drag-down fails (root-cause hypotheses, ranked)

1. **Wrong target (primary, explains "works for the keys, not the drag").** Claude Code runs a full-screen TUI. In the alternate screen buffer xterm has no scrollback (`buffer.active.length == rows`), so `scrollLines()` has nothing to scroll in either direction. The tmux/Claude history lives in the app/tmux, reachable only by input (PgUp/PgDn, wheel/mouse events, or tmux copy-mode). Any partial movement the user sees comes from xterm's normal-buffer scrollback that exists before/after the TUI (or from the browser scrolling something else). Finger drag in one direction can appear to work only when the normal buffer has history above the viewport; in the other direction it clamps at viewportY == bottom, so "does nothing". Which direction is dead depends on where the viewport is parked. (Hypothesis; confirm by logging `buffer.active.type`, `viewportY`, `length` on a failing drag.)
2. **Sub-cell deltas are discarded.** `lastY` is advanced every frame (`:244`) even when `lines === 0`, and `Math.round` needs >= 0.5 cell (~8px at ~17px cells) of movement within a single frame. Slow drags (< ~480 px/s) therefore scroll zero lines in both directions; the fractional remainder is thrown away rather than accumulated. Also `Math.round(-0.5) == -0` but `Math.round(0.5) == 1`, giving a slight directional bias. Fix: accumulate a pixel remainder, quantize with `Math.trunc`.
3. **Browser claims the gesture before the 15px threshold.** In PENDING the handler never calls `preventDefault()` (`:249-250`), and no `touch-action: none` is set. Chrome Android decides on pan at slop (~10px) and then makes later touchmoves non-cancelable and may fire `touchcancel` (the hook maps touchcancel to IDLE, `:353`). A drag in one direction can also trigger pull-to-refresh/overscroll when the page is at scrollTop 0 (drag-down is the pull-to-refresh direction, which matches the symptom asymmetry). Fix: `touch-action: none` (or `pan-x`) plus `overscroll-behavior: contain` on the terminal container; keep `{passive:false}` listeners.
4. Less likely: the 15px threshold phase drops the first 15px (no scroll until then), and `lastY` reset at transition, which adds perceived lag but not a direction bias.

## 3. How comparable terminals behave (INFERRED, from general knowledge; verify before citing)

| App | Normal buffer | Alt-screen / mouse-tracking TUI | Momentum |
|---|---|---|---|
| Termux (Android) | Drag scrolls local scrollback, 1:1-ish, with fling | In alt-screen, drag is converted to Up/Down arrow keys (DECCKM-aware) or, when the app enabled mouse tracking, to wheel button events (btn 64/65); this is the "alternate scroll mode" (DECSET 1007) behavior | Fling implemented via Android scroller |
| Blink Shell (iOS) | Native scroll view of hterm/xterm | Wheel events sent when mouse mode on; otherwise arrow keys/alternate scroll | iOS native inertia |
| Termius / JuiceSSH | Local scrollback drag | Termius sends wheel/arrow sequences in TUIs; JuiceSSH historically sent arrows/PgUp-style input in alt-screen | OS-native fling |
| ttyd / wetty (xterm.js) | xterm's own viewport scroll (native overflow scroll, so browser momentum) | Touch scroll does nothing useful in alt-screen unless the app enabled mouse tracking; xterm.js converts wheel (not touch) to SGR wheel events | Browser-native only for the normal buffer |
| VS Code terminal (desktop/web) | Wheel/touch scrolls xterm viewport | xterm.js sends wheel as mouse reports when tracking is on; when off and alt-screen, "alternate scroll" turns wheel into arrow keys (`terminal.integrated.alternateBuffer...` behavior) | Smooth-scroll setting, not touch-first |

Common pattern across the good ones: **decide target by terminal mode, not by gesture**:
- Normal buffer, no mouse tracking: scroll local scrollback.
- Mouse tracking on: emit wheel button events (SGR `\x1b[<64;col;rowM` up, `65` down, or X10 `\x1b[M`+char(96/97)) - the encoding must match the app's requested encoding (`terminal.modes` exposes the tracking mode; SGR vs UTF-8 vs X10 encoding is chosen via DECSET 1006/1005, which xterm.js does not expose as a public flag, so default to SGR with a fallback decision or reuse xterm's internal mouse service if reachable).
- Alt-screen without mouse tracking: DECSET 1007 alternate-scroll: send Up/Down arrows (honoring application-cursor-keys, `\x1bOA/\x1bOB` vs `\x1b[A/\x1b[B`) or PgUp/PgDn for page-sized gestures.
- tmux: if the pane is in tmux and the user wants tmux scrollback, wheel events enter copy-mode only if tmux `mouse on`; otherwise need prefix+`[`. PgUp/PgDn in a plain tmux pane do nothing (tmux copy-mode needs `C-b [` first; Shift-PgUp passes through).

Open point (must test, not assume): whether Claude Code responds to wheel/mouse events or only to PgUp/PgDn. Requirements open question 3 is still unanswered by this research; the toolbar proves PgUp/PgDn work (user-reported).

## 4. Momentum / inertia (INFERRED design guidance)
- Track velocity over the last ~100ms of samples; on touchend, if |v| > ~0.3 px/ms start a rAF loop with exponential decay (`v *= 0.95` per 16ms, stop below ~0.02 px/ms).
- Keep a fractional pixel accumulator and emit integer line steps (or wheel events) per frame; cap emitted wheel events per frame (e.g. <= 3) so a TUI is not flooded.
- Cancel momentum on any new touchstart, on mode change (alt-screen toggle), on session switch, and on component unmount.
- Respect `prefers-reduced-motion` (skip fling).
- In TUI-forwarding mode, momentum means sending synthesized input after the finger lifts; bound total events to avoid runaway scrolling after the TUI scrolls to an end.

## 5. Edge cases / failure modes the design must handle

| Case | Needed behavior |
|---|---|
| Alt-screen, no mouse tracking | Forward as arrows/PgUp-PgDn (alternate scroll); never call `scrollLines` (no-op) |
| Mouse tracking on | Forward SGR wheel events at the touch cell; do not start xterm selection on drag |
| Mode flips mid-gesture (TUI exits to shell) | Re-evaluate mode per frame, not once at gesture start; cancel momentum |
| xterm scrollback exists but TUI is on alt-screen | Mode, not buffer length, decides target |
| Selection vs scroll | Long-press timer (`longPressMsRef`) vs 15px slop is existing arbitration; keep slop > finger jitter and keep long-press cancel on movement |
| Pull-to-refresh / overscroll | `touch-action: none` + `overscroll-behavior: contain`; call `preventDefault` from first move, not just after threshold |
| `touchcancel` mid-drag | Currently drops to IDLE; acceptable, but momentum should not start from a cancel |
| Multi-touch | Existing: cancel gesture on `touches.length !== 1`; keep (pinch zoom out of scope) |
| tmux copy-mode active | PgUp/PgDn/wheel behave differently; q exits; do not auto-enter copy-mode silently |
| Rotation / keyboard resize | Re-read `cellH` per gesture (it is cached per drag already); abort gesture and momentum on resize event; refit and `terminal.refresh(0, rows-1)` after viewport settles (ties into the blank-terminal bug) |
| Direction convention | Natural scrolling: finger down = view older content. Test both directions explicitly; current code's sign (`-moveDy`) is correct for xterm but the dead direction is the clamped one |
| Tests | jsdom has no real touch physics: unit-test pure functions (accumulator, velocity/decay, target selection by mode) and keep e2e for the integration; Playwright touch emulation can dispatch TouchEvents but not Chrome's pan/pull-to-refresh arbitration, so that part needs a real device |

## 6. Recommendations for the design phase
1. Extract a pure `ScrollTargetSelector` (normal buffer / mouse-tracking wheel / alt-screen arrows or PgUp) keyed on `terminal.buffer.active.type` and `terminal.modes`.
2. Replace per-frame `Math.round` with a pixel accumulator; make drag 1:1.
3. Add `touch-action: none`/`overscroll-behavior: contain` on the terminal container and call `preventDefault` in PENDING once movement clearly exceeds slop.
4. Add velocity-based momentum on top of the same accumulator.
5. Before choosing PgUp-vs-wheel for the TUI path, empirically test Claude Code's response to SGR wheel events in a manual instance (ports 62871+); fall back to PgUp/PgDn (known working) if wheel is ignored.
