# Pitfalls: mobile drag-scroll and blank terminal after keyboard open

Paths are relative to `web-app/src/`. Confidence: VERIFIED = opened code / ran grep; EXTERNAL = from web search results (not independently reproduced); INFERRED = reasoning only.

## 1. Why drag-DOWN can fail while drag-up works

Code under review: `lib/hooks/useTerminalGestures.ts` (onTouchMove ~L212-262) and `lib/terminal/touchDrag.ts` (`rafThrottlePoint`).

### 1a. Sub-cell remainder is discarded every frame (VERIFIED, most likely cause of "partial" scrolling)
The rAF callback does `moveDy = clientY - lastY; lastY = clientY; lines = Math.round(-moveDy / cachedCellH)`. `lastY` is advanced even when `lines === 0`, so the fractional remainder is thrown away. At 60 fps a slow or medium drag moves roughly 3-10 px/frame while a cell is roughly 15-20 px, so `|moveDy/cellH| < 0.5` on most frames and nothing scrolls in either direction; only fast flicks register. Fix: accumulate (`acc += -moveDy / cellH; lines = trunc(acc); acc -= lines`) and do not discard `lastY` progress.

### 1b. Math.round is asymmetric around zero (VERIFIED by JS semantics; minor)
`Math.round(0.5) === 1` but `Math.round(-0.5) === -0`, and `Math.round(-1.5) === -1` vs `Math.round(1.5) === 2`. Drag-up gives positive `lines` (rounds up at .5); drag-down gives negative `lines` (rounds toward zero at .5). This biases toward up but is tiny next to 1a. Use `Math.trunc` on an accumulator.

### 1c. `scrollLines()` only moves xterm's own buffer; alt-screen has no scrollback (INFERRED from xterm behavior + requirements)
In alt-screen (Claude Code TUI) xterm's normal-buffer scrollback is not shown, so `scrollLines(n)` is a no-op or scrolls stale normal-buffer history. "Scrolls up mostly" is plausibly the normal-buffer scrollback behind the alt screen being at/near the bottom (down = nothing to scroll, up = some history). Drag-down looks dead because the viewport is already at the bottom (`ydisp == ybase`). Confirm by logging `terminal.buffer.active.type`, `baseY`, `viewportY` during a drag. The correct mechanism is forwarding input to the TUI (PgUp/PgDn, or SGR wheel events `\x1b[<64;col;rowM` / `<65` when mouse tracking is on).

### 1d. preventDefault timing / Chrome already claimed the gesture (partly VERIFIED, partly EXTERNAL)
- Listeners: `touchstart` on container (passive:false), `touchmove` on `document` with `{passive:false}` (L360-363). Explicit `passive:false` avoids Chrome's default-passive treatment of document-level touchmove, so this is OK.
- `preventDefault()` is only called once state is `SCROLLING` (after a 15 px PENDING threshold). During PENDING, `touchmove` events are not prevented. Chrome can start a scroll/overscroll during that window; once a scroll has begun, later `touchmove` events are non-cancelable and `preventDefault()` is ignored ("Ignored attempt to cancel a touchmove event with cancelable=false"). The code does not check `e.cancelable`. Sources: https://www.uriports.com/blog/easy-fix-for-intervention-ignored-attempt-to-cancel-a-touchmove-event-with-cancelable-false/ , https://github.com/mapbox/mapbox-gl-js/issues/11961
- Mitigated by CSS: `.terminal` in `components/sessions/XtermTerminal.css.ts:60` has `touchAction: "none"`, which tells Chrome up front not to pan, so the above should be rare INSIDE the terminal element. Gaps: (i) `touch-action` is not inherited and only applies to the element where the touch starts (and its ancestors' constraints are intersected), so touches landing on overlay children with `manipulation` (e.g. toolbar/button at XtermTerminal.css.ts:31, :131) behave differently; (ii) the document-level touchmove handler fires for touches that started outside the container, though gated by `state`.

### 1e. Pull-to-refresh / overscroll (VERIFIED absence of CSS)
`grep overscroll` over `web-app/src` and `web-app/public` finds only `components/shared/VirtualLogList.css.ts:6` (`overscrollBehaviorY: contain`). There is NO `overscroll-behavior` on `html`/`body`/`.terminal`. With `touch-action:none` on the terminal, pull-to-refresh should not trigger from inside it, but a drag-down that ends up as a page-level gesture (touch starting on a `manipulation` area, or the page itself being taller than the viewport during keyboard transitions) can trigger Chrome pull-to-refresh or page overscroll, which would also steal the gesture (`touchcancel` -> `transitionToIdle`). Cheap hardening: `overscroll-behavior: none` on `html, body` (and `contain` on the terminal container). Chrome only enables pull-to-refresh at page scroll offset 0, which is exactly the "drag down" direction, so this asymmetry matches the symptom. EXTERNAL/INFERRED: https://developer.mozilla.org/en-US/docs/Web/CSS/overscroll-behavior

### 1f. rAF throttle + lastY (VERIFIED)
`rafThrottlePoint` keeps only the latest point per frame (fine). `lastY` is set at PENDING->SCROLLING (`lastY = touch.clientY`) so the 15 px threshold distance is dropped (initial 15 px of finger travel never scrolls; up and down alike). Combined with 1a this makes slow drags feel dead. `cachedCellH` is cached at drag start; if 0 the callback silently returns.

### 1g. Interaction with mouse-tracking (INFERRED)
`isMouseTracking()` (`lib/terminal/mouseTracking.ts`) is consulted for tap/select but NOT in the SCROLLING branch, so a mouse-tracking TUI still gets local `scrollLines()`. Needs a branch that forwards wheel/PgUp-PgDn input.

## 2. Black/blank xterm after soft-keyboard open

Current code: `components/sessions/TerminalOutput.tsx` ~L1160-1185.

### 2a. Dropped (not debounced) resize events with a fixed 400 ms timer (VERIFIED)
`onVpResize` sets `isFittingRef.current = true` and schedules `fit()` after 400 ms (mobile). Any `visualViewport` resize events arriving during that window are ignored (`if (isFittingRef.current) return;`), and the flag is only cleared one rAF after the fit. This is a leading-edge throttle, not a trailing debounce: if the keyboard animation runs longer than 400 ms, or Android emits a late final resize, the last size is never re-fitted. Result: terminal sized for an intermediate viewport height, content off-screen/black. Prefer trailing debounce keyed on "viewport height unchanged for N frames", and always re-run after the last event.

### 2b. No refresh after fit (VERIFIED; EXTERNAL guidance)
`fit()` is called with no `terminal.refresh(0, rows-1)` afterwards. Search guidance recommends a refresh after re-fit. Source: https://github.com/xerktech/Turma/pull/862 and the linked checklist.

### 2c. fit() on a zero-size container (EXTERNAL; plausible here)
A `fit()` while the container measures 0x0 (hidden pane, `display:none`, keyboard transition collapsing the flex height) yields a 0-row terminal that paints black until a later resize. Guard: skip fit when `clientWidth/clientHeight` is 0 and retry. Sources: https://github.com/xerktech/Turma/pull/860 (hidden pane -> black terminal), https://github.com/xerktech/Turma/pull/862 (re-fit until sized; no-op on 0-height). The `isVisible` effect (L1160, `setTimeout 50` then fit) is also a fixed-delay fit with no size guard.

### 2d. Stale `--viewport-height` / ordering (VERIFIED partially)
`--viewport-height` defaults to `100dvh` (`app/globals.css:102`) and is updated by `ViewportProvider` via rAF-batched `visualViewport` updates (per requirements). Layouts use `calc(var(--viewport-height) - header)` (`app/page.css.ts:53-54`). The fit listener and the CSS variable updater are independent: if `fit()` measures before the CSS var is applied (or before layout settles), it fits to the old height; there is no ordering guarantee between ResizeObserver, visualViewport listeners and the rAF batch. Best fix: fit from a `ResizeObserver` on the terminal container (fires after layout with the real size) rather than from a visualViewport timer. INFERRED; ordering not tested on device.

### 2e. Renderer / context loss (VERIFIED absence; EXTERNAL risk)
`grep -i webgl|CanvasAddon` in `components` and `lib` found no WebGL/canvas addon use by name search in the files grepped (the grep returned nothing), so context loss is probably not the cause unless xterm's default renderer is canvas/DOM in the installed version. Some Android GPU drivers paint WebGL canvases black even at correct size (EXTERNAL, Turma PR #862). Confirm the renderer in `package.json` / xterm version before ruling out. Also xterm 4.0.0 fixed missing canvas refresh on DPR change: https://newreleases.io/project/github/xtermjs/xterm.js/release/4.0.0 ; related: https://github.com/xtermjs/xterm.js/issues/1087 (display refresh after resize), https://github.com/xtermjs/xterm.js/issues/4344 (fit addon 0.6.0 / xterm 5.1.0 not resizing).

### 2f. Instrumentation suggestion (INFERRED)
On each fit, log (debug level) `visualViewport.height`, container `clientHeight`, `terminal.rows`, and whether fit was skipped; on repro, a 0 or stale value will identify 2a/2c/2d without a device debugger.

## Sources
- https://github.com/xerktech/Turma/pull/862
- https://github.com/xerktech/Turma/pull/860
- https://github.com/xtermjs/xterm.js/issues/1087
- https://github.com/xtermjs/xterm.js/issues/4344
- https://newreleases.io/project/github/xtermjs/xterm.js/release/4.0.0
- https://www.uriports.com/blog/easy-fix-for-intervention-ignored-attempt-to-cancel-a-touchmove-event-with-cancelable-false/
- https://github.com/mapbox/mapbox-gl-js/issues/11961
- https://developer.mozilla.org/en-US/docs/Web/CSS/overscroll-behavior
