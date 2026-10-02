# Implementation Plan: scrolling

**Feature**: Fix Android Chrome touch-drag scrolling (both directions, correct target, momentum) and the blank terminal after soft-keyboard open.
**Date**: 2026-10-01
**Status**: Ready for implementation (default routing ships unverified; Phase 0 spike confirms rows post-hoc; see Routing Verification Statement)
**Type**: feature addition, Complexity 3 (matches requirements.md)

## Routing Verification Statement (pre-mortem P1 #1 and #2; revised 2026-10-01)

This is a **PR-body statement, not a mechanical merge block**: nothing in CI fails if it is missing. The reviewer reads the PR body and the default-table test (validation.md "Routing Verification Statement checks"). It was previously named "Hard Merge Gate"; that name overstated the control.

**Decision 2026-10-01 (reverses the earlier safe default):** routing ships **unverified by default** rather than defaulting to local. Until the Phase 0 device spike verifies the rows, a session in the alternate screen or with mouse tracking on auto-routes drag to the TUI; the spike then confirms or adjusts the rows post-hoc.

**Default rows (shipped with `ROUTING_VERIFIED = false` in `lib/terminal/scrollRouting.ts`):**

| Priority | When | Target |
|---|---|---|
| 1 | `bufferType === 'alternate'` (any `mouseTrackingMode`) | `tui-pgkeys` |
| 2 | `mouseTrackingMode !== 'none'` (any `bufferType`) | `tui-pgkeys` |
| default | `bufferType === 'normal'` and tracking `none` | `xterm-local` |

`tui-wheel` is never selected by default rows (only when `TuiScrollPolicy === 'wheel'`, set by a verified Q1). `'local'` / `'tui'` overrides short-circuit the table. The tmux copy-mode note stays in force (Story 0.1.3, row "plain shell inside tmux"): PgUp in tmux enters copy-mode and scrolls tmux history, PgDn at the bottom exits it, and a stale copy-mode can swallow keystrokes meant for the app (INFERRED from tmux defaults; the spike verifies).

**What the PR must say (reviewers check this):**
- "TUI auto-routing is unverified (`ROUTING_VERIFIED=false`); default rows are alternate screen or mouse tracking -> `tui-pgkeys`, otherwise `xterm-local`."
- The misroute risk, listed: (1) a plain shell inside tmux reports the alternate screen, so a drag sends PgUp/PgDn to tmux (scrolls tmux history; bash ignores the key; a stale copy-mode may swallow the next keystrokes); (2) a TUI that renders in the normal buffer with tracking `none` routes local and the drag does nothing; (3) PgUp/PgDn in non-Claude alt-screen apps (less, vim) behave page-wise, not 1:1. Escape hatches: the S6 override (`local` forces local; `tui` forces pages) and the toolbar PgUp/PgDn.
- "The 50-cycle blank metric (D7) is unproven" unless the device run is attached; baseline blank rate unknown unless Task 0.1.2d recorded it.

**After the spike (Task 0.1.3a):** the Q2 mode matrix confirms or edits the default rows, Q1 sets `TuiScrollPolicy`, `ROUTING_VERIFIED` flips to `true` in the same PR or a follow-up, and Spike Findings (with log excerpts) are committed here. The routing guard test is: **the default table routes `alternate -> tui-pgkeys` and `normal` + tracking `none -> xterm-local`**, `tui-wheel` is unreachable unless policy is `'wheel'`, and while `ROUTING_VERIFIED=false` each routing decision is logged with `unverified:true` (so a device dump shows which rows were unverified).

**Story 1.2.1b and Story 2.1.2 are not gated on the spike.** Both are built to handle either Q2/Q3 outcome (the table is data; `refit()` has no renderer or container-size assumption) and ship unconditionally. The only gate is the PR statement above; the Q3 verdicts steer which regression test is the primary proof, not whether the code merges.

**ADRs**: None new. Note: "ADR-002" (resize sampler) and "ADR-012" (TouchEvent transport) have **no files in `docs/adr/`** (verified; `docs/adr/012-*` is an unrelated react-virtuoso ADR). They exist only as code comments (`XtermTerminal.tsx` sampler block ~L1020-1139, `useTerminalGestures.ts`) and in `docs/tasks/*` (e.g. `docs/tasks/terminal-jank.md`). Cite those, not `docs/adr/`. The single-owner-of-fit decision narrows the ADR-002 sampler's role; it is recorded in Pattern Decisions, and a real ADR may be written at ship time.

All paths are under `web-app/src/` unless stated. Inputs: `project_plans/scrolling/requirements.md`, `research/*.md`. Appetite: **Large (3-6 weeks)**, one developer, all scope kept (revised 2026-10-01 from Medium; see "Schedule and sequencing"). Target: Android Chrome. Transport stays TouchEvent-based (ADR-012).

## QoS additions (from `research/qos.md`, Large appetite)

Six items from [`research/qos.md`](../research/qos.md) section 3 are folded in; the rest are in "Deferred / follow-up QoS" (before Plan risks). Every file:line below was re-read in this worktree on 2026-10-01 (VERIFIED) unless marked.

| # | QoS item (qos.md rank) | Verdict after reading code | Where |
|---|---|---|---|
| 1 | Bypass resize bounce-hold for keyboard-driven refits (rank 3) | **Partly confirmed, scoped down.** The hold (`useTerminalFlowControl.ts:391-403`, 3 s doubling to 15 s) does **not** delay the *local* fit/refresh (XtermTerminal sampler, no dependency on `resize()`). It delays only the server resize RPC and the follow-up `currentPaneRequest`. It still matters: `TerminalOutput.tsx:909-910` calls `clearBufferBeforeResize()` (`clearBufferBeforeResize` defined L393-396, calls `xtermRef.current?.clear()`) **then** a non-forced `resize(cols, rows)`, and a keyboard close returns to a size already in `sentHistoryRef`, which the code classifies as a bounce. Result: canvas cleared now, server snapshot held 3-15 s (INFERRED as the observed "stays black"; spike Q3(d) proves or refutes it). qos.md's wording ("delay the post-keyboard refit") is corrected: it delays the *server repaint*, not the local refit | Story 2.1.5; spike Q3(d) |
| 2 | `visualViewport` `offsetTop` + `height` in the refit path; zero-size guard (rank 3) | Settle currently keys on `height` only and `TerminalOutput.tsx:1176-1186` listens to vv `resize` only (not `scroll`); `ViewportProvider.tsx:34-55` already uses both events and `offsetTop`. Zero-size guard **already planned** (Story 2.1.1 `FitGuard`, 2.1.2 retry bound), not duplicated | Story 2.1.4 (amended) |
| 3 | Scroll input must not queue behind paste (rank 5) | **No latency gap, one correctness gap.** Input path is `onSendData` -> `handleTerminalData` -> `sendInput` (`TerminalOutput.tsx:770-771`, `useTerminalFlowControl.ts:237-290`) -> `MessageQueue` FIFO (`lib/terminal/MessageQueue.ts:28-37`). Pastes > 512 B are chunked, one `setTimeout(10 ms)` per chunk, so a scroll key waits behind at most one in-flight chunk. The real gap is **interleaving**: a PgUp/wheel push lands mid-paste and corrupts the pasted text | Story 1.2.6 |
| 4 | Cap output per animation frame, not a fixed wheel throttle (rank 1) | Reconciled with existing caps (see Story 1.1.4 table) | Story 1.1.4 |
| 5 | Verify visibility-resync path and `NEXT_PUBLIC_RECONNECT_V2` (rank 6) | Verify-only spike task; one small conditional task after it | Task 0.1.2f, Story 2.1.6 |
| 6 | Which streaming path production uses; is client `FlowControl` honored (gap #1) | Investigation only | Task 0.1.2f (Q4) |

---

## Creative pass (alternatives)

| Approach | Strength | Weakness |
|---|---|---|
| A. Mode-routed pure modules + hook integration + single-owner fit (chosen) | Fixes all leading candidates behind testable seams; spike findings change one switch, not the design | More files than a patch |
| B. Minimal patch: accumulator + always send PgUp/PgDn + `refresh()` after the 400ms fit | Smallest diff | Page-sized, not 1:1; no momentum; leaves two competing fit pipelines (the suspected race) |
| C. Adopt xterm 6 built-in touch `Gesture`/`@use-gesture` | Free inertia | Cannot target the TUI's scrollback; fights the long-press/tap state machine (build-vs-buy.md) |

Chosen: A. Phase 0 instruments first so A's branches are picked on evidence.

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `ScrollTarget` | `'xterm-local' \| 'tui-wheel' \| 'tui-pgkeys'` — where a scroll step is delivered | Sum type; exhaustive switch |
| `ScrollMode` | Snapshot `{ bufferType: 'normal'\|'alternate'; mouseTrackingMode: 'none'\|'x10'\|'vt200'\|'drag'\|'any' }` read from the terminal | Re-read every frame, not once per gesture. A *signal*, not the verdict: bufferType alone cannot tell a TUI from tmux (see `ScrollRoutingPolicy`) |
| `ScrollRoutingPolicy` | Decision table `rules: Array<{ when: Partial<ScrollMode>; target: ScrollTarget }>` plus `default: ScrollTarget`, first match wins, with an optional user override `'auto' \| 'local' \| 'tui'` (persisted in localStorage per device, surfaced as a toggle) | Data, not code: filled from the Spike Findings table (Q2) so the spike edits rows, not logic. Never hardcoded on `bufferType` alone. **Default until the spike verifies rows** (decision 2026-10-01): rows `alternate -> tui-pgkeys`, `tracking != none -> tui-pgkeys`, `default: 'xterm-local'` (`ROUTING_VERIFIED=false`); see Routing Verification Statement |
| `decideScrollTarget(mode, policy, tuiPolicy, override)` | Pure function `ScrollMode -> ScrollTarget`; `override` of `'local'`/`'tui'` short-circuits the table | Lives in `lib/terminal/scrollRouting.ts` |
| `TuiScrollPolicy` | `'wheel' \| 'pgkeys'`: which TUI mechanism to use once the target is a TUI | Passed as a hook option (default constant switched by spike result Q1); wheel is selected **only** when explicitly `'wheel'` |
| `ScrollAccumulator` | Converts pixel deltas into integer line steps, carrying the fractional remainder | `Math.trunc`, symmetric up/down |
| `LineDelta` | Signed integer line count; positive = toward newer output (finger moved up) | Newtype alias; direction tested both signs |
| `MomentumTracker` | Rolling velocity window plus exponential decay producing per-frame pixel deltas after release | Takes injected clock/rAF. Constants (canonical, shared with design/ux.md; exported as `MOMENTUM_CONSTANTS`, tune on device): window 100 ms; min fling 0.3 px/ms; decay `v *= 0.95` per 16 ms frame; stop below 0.02 px/ms; velocity cap 8 px/ms; max 120 frames |
| Slop rule | One rule: slop `SLOP_PX` (exported constant, **default 15 px**; 10 px is a fallback tried in D9 only if 15 fails ux.md tuning criterion (c)); **tap tolerance equals `SLOP_PX`** (a release under the slop before 400 ms is a tap; no dead zone; the old 8 px tap threshold is widened, one intended regression-anchor change); on crossing, `ScrollAccumulator` is seeded with the **overshoot only** (travel minus `SLOP_PX`) | Gives no initial jump (UX AC2); the fixed `SLOP_PX` finger-to-content offset is accepted. Tests import `SLOP_PX` rather than hardcoding 15 (or 8 for the tap tolerance). Identical wording in design/ux.md S1 |
| `TUI_PAGE_STEP_LINES` | `max(1, floor((rows - 1) / 2))`: post-slop finger travel (in lines) that emits one page key in TUI routes (half a page) | Replaces the full-page threshold (a half-screen drag did nothing); tuned on device (D9, ux.md criterion e) |
| `EffectiveScroll` | `{ target: ScrollTarget; source: 'auto' \| 'override'; gesturesOn: boolean }`, computed by one hook `useEffectiveScrollMode(scrollMode, override, gestureOn)` (created in Task 1.2.5e) from `decideScrollTarget`, the override and the Gesture scrolling setting | Single source for the toolbar PgUp/PgDn (Story 1.2.9), the chip and the jump button. **Data flow (upward)**: the live `ScrollMode` (`bufferType`, `mouseTrackingMode`) lives on the xterm `Terminal` inside `XtermTerminal`; `XtermTerminal` reports it to `TerminalOutput` through an `onScrollModeChange(mode)` prop, fired only when the value changes (see Task 1.2.5e). The drag hook does **not** use this hook: it re-reads `readScrollMode(terminal)` every frame (never stale) and receives only `override` and `gestureScrollEnabled` as options |
| `connectionEpoch` | Counter owned by `TerminalOutput` (`useState`), incremented on a reconnect (the flag-off handlers at `TerminalOutput.tsx:~969/~1067`; `useTerminalStream.ts` reconnect when `NEXT_PUBLIC_RECONNECT_V2` is on) and on every full-snapshot write (the `TerminalStreamManager` `onFullSnapshot` callback, `:268-270`) | Gesture-cancel and `netPagesUp`-reset trigger (ux.md S7/S9). Exposed as a number prop `connectionEpoch` threaded `TerminalOutput` -> `XtermTerminal` -> hook option and read by `JumpToLatestButton` (Task 1.2.5f; threading in Task 1.2.5g) |
| Repaint timing budget | After `ViewportSettled` fires, content is visible within one sampler tick (50 ms) plus one animation frame; the settle itself must fire no later than 400 ms after the first `visualViewport` event for a keyboard animation that stabilizes within 300 ms | The only place these numbers live; requirements.md and ux.md state the outcome and point here |
| `ReducedMotionPref` | Result of `matchMedia('(prefers-reduced-motion: reduce)')`, read at gesture start | Momentum disabled when true |
| `WheelReport` | Encoded mouse wheel bytes: SGR `\x1b[<64;c;rM` (up) / `<65;c;rM` (down); X10 fallback | Pure encoder |
| `PageKeyBytes` | `\x1b[5~` (PgUp) / `\x1b[6~` (PgDn) | Same bytes the toolbar sends in a TUI route (`TerminalOutput.tsx:1984/2018`); in a local route the toolbar calls `scrollPages` instead (Story 1.2.9) |
| `postFitRepaint` | Step after a confirmed `fit()` or a forced `refit()`: always `refresh(0, rows-1)`; `clearTextureAtlas()` **only when the active renderer is WebGL** | The "repaint seam". The code logs "WebGL2 unavailable (Android?), using canvas renderer" (~`XtermTerminal.tsx:606`), so on the target phone the WebGL branch may never run; the `refresh` is the renderer-independent part and must be sufficient on its own. Takes `renderer: 'webgl' \| 'canvas' \| 'dom'` from a ref set at renderer init (L606 area) and by `triggerCanvasFallback` |
| `refit()` | Imperative handle on XtermTerminal that calls a `requestFitRef` (set inside the mount effect) which starts the sampler **directly**, bypassing the 150 ms RO debounce (`XtermTerminal.tsx:1126`), with `forceRepaint:true` and a `reason` | Replaces bare `fit()` at `XtermTerminal.tsx:1269-1271`; `reason` is `viewport-settle \| visibility \| manual-resize \| font-change \| context-loss` (logged, see design/ux.md S5) |
| `requestFitRef` | `MutableRefObject<(opts:{forceRepaint:boolean; reason:RefitReason}) => void>` assigned in the mount `useEffect` to a closure over `startSamplerIfNeeded` | Bridges the effect-local sampler (L1038-1101) to `useImperativeHandle` (L1250) |
| `ViewportSettled` | Signal that **both** `visualViewport.height` and `visualViewport.offsetTop` are unchanged for 3 consecutive animation frames, or a max-wait timeout elapses (600 ms), whichever first; armed by vv `resize` **and** `scroll` events (QoS item 2) | Replaces fixed 400 ms timer; timeout covers an animating URL bar that never stabilizes. Repaint-timing metric everywhere: content visible within one sampler tick (50 ms) + one animation frame after `ViewportSettled` fires |
| TUI | Terminal application that owns its own scrollback (Claude Code, less, vim, a tmux pane in copy-mode) | Used consistently for the non-`xterm-local` targets (`tui-wheel`, `tui-pgkeys`) |
| `COASTING` | `GestureState` entered on `touchend` from SCROLLING when momentum starts | A touchstart in COASTING cancels momentum and sets a `consumedByCoast` flag so the following touchend skips the tap/focus path |
| `PerFrameCap` | Upper bound on output emitted in one animation frame, per target (local lines, wheel reports, page keys); replaces any fixed-interval wheel throttle on the 1:1 path | See Story 1.1.4 |
| `FitGuard` | Zero-size check: skip fit and retry when container `clientWidth`/`clientHeight` is 0 | Pure predicate |
| `MobileDebugLog` | localStorage-flag-gated (`debug-terminal-mobile`) structured logger | Follows existing `debug-terminal` flag convention (`lib/terminal/TerminalStreamManager.ts:208`) |

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Scroll target selection | Strategy: pure function over a data-driven `ScrollRoutingPolicy` decision table + user override | GoF | if/else inside hook; hardcoded `bufferType==='alternate'` | `bufferType` mis-routes TUIs in the normal buffer and plain shells inside tmux (alt screen); table rows are filled from spike evidence and an override toggle is the escape hatch |
| `ScrollTarget`, `TuiScrollPolicy` | Sum types with exhaustive switch | type-driven-design | string flags | Compiler-checked routing |
| `LineDelta` / pixel vs line units | Branded number types | type-driven-design | raw `number` | Prevents px/line mix-ups (the existing bug class) |
| `ScrollAccumulator`, `MomentumTracker` | Small stateful classes with injected clock, no DOM | PoEAA-lite / Transaction Script rejected | Logic inline in rAF callback | Fake-clock deterministic tests (deterministic-fast-tests) |
| Hook <-> terminal | Adapter: hook calls `route(lines)`; encoders return bytes passed to existing `onSendData` | GoF Adapter | Synthetic `WheelEvent` on `.xterm-screen` | Only as fallback if spike shows manual SGR is ignored; explicit bytes are testable |
| Fit ownership | Single owner of *resize-driven* fits (XtermTerminal sampler, ADR-002 per code comments/`docs/tasks`) + `refit()` facade via `requestFitRef`; initial-mount (L710) and renderer-swap (L502) fits stay as documented exceptions (Story 2.1.3 table) | Facade | Keep two pipelines | Pipeline 2's `fit()` bypasses stability bookkeeping (architecture.md s3) |
| Settled detection | rAF stable-height poll (pure, injected rAF) | - | `navigator.virtualKeyboard` | Chromium-only extra surface; rAF-stable works without opt-in |
| Momentum / gestures | Build ~60 lines, no new dependency | build-vs-buy.md | `@use-gesture`, hammer.js, xterm built-in `Gesture` | None reach TUI scrollback; conflict with state machine |

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `components/sessions/XtermTerminal.tsx` (1378 lines) | God component; carefully tuned ADR-002 resize sampler; bare `fit()` handle | **Isolate via seam** | New logic goes in `lib/terminal/*` pure modules; localized edits only: `requestFitRef` + sampler branches (L1038-1101), imperative handle (L1269-1271), and the bare-`fit()` dispositions in Story 2.1.3. A refactor-first pass would risk ADR-002 regressions for no benefit to either bug |
| `lib/hooks/useTerminalGestures.ts` (374 lines) | Scroll branch mixes quantization, routing, DOM | **Isolate via seam** | Extract quantization/routing/momentum to pure modules; hook keeps state machine only (plus COASTING) |
| `components/sessions/TerminalOutput.tsx` (2033 lines) | God component; duplicate fit pipeline at L1160-1188 (50 ms visibility `setTimeout(fit)`, 400 ms `onVpResize` + `isFittingRef` L124); manual-resize `fit()` at L1511 | **Isolate via seam**, with the duplicate pipeline **deleted** (Story 2.1.3, sequenced after the repaint seam) | Only the fit pipeline is touched, via `refit()`; nothing else in the file is refactored. Deleting (not refactoring) the second pipeline is the minimal change, and leaving it makes the fix unprovable |

## Observability Plan
- **Logs**: `MobileDebugLog` (debug level, off by default) records: per-frame scroll sample `{bufferType, mouseTrackingMode, viewportY, baseY, rows, cellH, moveDy, acc, lines, target}`; per-fit `{vv.height, vv.offsetTop, container clientW/H, rows/cols, fit skipped(reason), renderer(webgl|canvas|dom), repaint forced}`; per-resize `{cols, rows, bounce:boolean, holdMs, bypassed:boolean}`; WebGL `onContextLoss`; momentum start/stop. Dump via `window.__termDebug.dump()` for pasting from a phone.
- **Routing decisions** are logged with `unverified:true` while `ROUTING_VERIFIED=false`, so a device dump shows which rows drove a misroute.
- **Misroute and override metrics (local only, nothing sent anywhere; not `track(...)` analytics)**: `MobileDebugLog` entries `route-decision {target, source, unverified}`, `override-change {from, to}` and `misroute-proxy {kind}`, where `kind` is `override-after-drag` (override changed within 10 s of an auto-routed drag) or `toolbar-key-after-drag` (toolbar PgUp/PgDn pressed within 5 s of an auto-routed drag). `window.__termDebug.stats()` returns the counts since page load. Read in the device passes and recorded in the PR (outcome in requirements.md: Claude Code in tmux 0 flips, plain tmux shell at most 1). Entries only exist when the debug flag is on; with it off the cost is zero and the metric is simply unavailable.
- **Metrics**: none new server-side. Client targets: one scroll dispatch per rAF (asserted in tests) and no frame over 32 ms during drag/fling on the target phone (device trace, tune-on-device). **Owner and pass criterion**: device checklist item D9, run by the developer; pass = a DevTools performance trace of three drags and three flings of about 5 s each shows no frame longer than 32 ms; the trace summary is attached to the PR; a fail blocks closing Tier A (file a perf follow-up only if the cause is outside this project's code).
- **Alerts**: no new alerts required.

## Risk Control
- **Risk level**: not "low": default TUI routing ships unverified (Routing Verification Statement) and a plain tmux shell may be misrouted; the redraw change touches ADR-002-tuned fit code that desktop also uses.
- **Feature flag**: none for the code. Mitigations: the persisted `auto|local|tui` override (per device) and toolbar PgUp/PgDn as always-available escape hatches; debug logging gated by `localStorage['debug-terminal-mobile']`.
- **Rollback**: revert the PR; a misrouted session is fixed immediately with the S6 override without a rollback.
- **Staged rollout**: full rollout on merge; deploy via `make install-service` only after the manual-instance device pass (see CLAUDE.md warning: restart kills tmux unless `--tmux-keep-server`).

## Unresolved Questions
- [ ] Q1: Does Claude Code's scrollback respond to wheel/SGR events, or only PgUp/PgDn? Does it enable 1006 (SGR) encoding? — confirms or sets `TuiScrollPolicy` (default `'pgkeys'`; does not block Story 1.2.1b) — owner: Phase 0 spike (Task 0.1.2b), physical Android device
- [ ] Q2: (a) What are `bufferType` + `mouseTrackingMode` for a plain shell inside tmux, and for Claude Code (inside tmux and, if possible, outside)? (b) Real cause of drag-down failing (alt-buffer no-op vs sub-cell loss vs gesture stolen by browser)? — (a) confirms or edits the shipped default `ScrollRoutingPolicy` rows (Story 1.1.2, post-hoc, not a merge blocker); (b) decides Story 1.2.3 scope (CSS hardening is kept regardless) — owner: Task 0.1.2c
- [ ] Q3: (a) Does the container resize when the keyboard opens, or only `visualViewport`? (b) Which renderer is live on the target phone (WebGL or canvas)? (c) Real cause of black screen (stale size / zero-size / missed refresh / WebGL context loss)? — (a) and (b) should be recorded early (they steer test priority, pre-mortem P1 #2) but do not block Story 2.1.2; (c) decides which regression test is the primary proof — owner: Task 0.1.2d
- [ ] Q3(d) (QoS): In a real keyboard open/close, does `resize()` classify the close as a bounce (`useTerminalFlowControl.ts:391`), and how long is the canvas blank between `clearBufferBeforeResize()` and the next snapshot write? — decides whether Story 2.1.5 is the primary black-screen fix or only a secondary one — owner: Task 0.1.2d
- [ ] Q4 (QoS, investigation only): Which server path serves production sessions' `StreamTerminal` (WebSocket `connectrpc_websocket.go` -> control-mode/hub, vs `session_service.go:4069`), and is client `FlowControl{paused}` honored on that path? — informs the Deferred section only; blocks nothing — owner: Task 0.1.2f
- [ ] Q5 (QoS): Is `NEXT_PUBLIC_RECONNECT_V2` baked into the deployed bundle, and on return from a hidden tab does the terminal drop stale queued output and repaint? — gates Story 2.1.6 — owner: Task 0.1.2f
- [ ] Does xterm 6 built-in touch `Gesture` compete with our hook on the normal buffer? — blocks Task 1.2.1b1 (not 1.2.1a: the tests are written against either outcome) — owner: Task 0.1.2c in the **week-1 device pass** (check `.xterm-viewport` behavior); if no device is available by the end of week 2, apply fallback (1) preemptively because it is harmless when xterm does not compete. Requires `pnpm install` first (Task 0.0.1). **Fallback if it competes (double-scroll or the viewport steals the drag)**, in order: (1) capture-phase `touchstart`/`touchmove` listener on the container that calls `stopPropagation` once the hook's slop is crossed so xterm's viewport handler never sees the move; (2) if xterm still scrolls, set `touch-action: none` and `pointer-events: none` on `.xterm-viewport` for touch devices via `XtermTerminal.css.ts` (the hook is the sole scroll owner); (3) if neither works, drop the local-buffer 1:1 claim for the affected case and record it in Spike Findings. Two tests pin it (Task 1.2.1a3): `scrollLines_should_HaveSingleProductionCaller_InUseTerminalGestures` (a source-scan test over `web-app/src` excluding tests and generated code) and `viewportTouch_should_NotDoubleScroll_When_HookOwnsDrag`. Toolbar keys use `scrollPages` and the jump button uses `scrollToBottom`; both are different APIs and are not covered by that scan.

## Dependency Visualization

```
0.0.1 pnpm install + baseline jest ──> everything (first task)
0.0.2 bug docs (no deps)
0.1.1 debug log + instrumentation (a,b,c1-c4,d) ──> 0.1.2 device spike (Device slot 1) ──┐
1.1.2b routing rows (week 1) ───────────────────────────────────────────────────────────┴──> 0.1.3a apply gate (ONCE, week 2: needs 1.1.2b AND 0.1.2c/e)
                                                                                              0.1.3b re-confirm rows at device slot 2 (verification only, no code unless it contradicts)

Hook file chain (useTerminalGestures.ts; serial, one developer):
1.2.4a regression anchors (on the UNMODIFIED hook) ──> 0.1.1b scroll instrumentation (anchors re-run) ──> 1.2.4b extract gesture state machine (pure module) ──> 1.2.1a1-a3 tests ──> 1.2.1b1 ──> 1.2.1b2 ──> 1.2.1b3 ──> 1.2.1c ──> 1.2.1d1 ──> 1.2.1d2
        ──> 1.2.2a/b1/b2/b3 momentum ──> 1.2.10 S9 behaviors ──> 1.2.5c Gesture-Off option ──> 1.2.5d hint callback ──> 1.2.7b jump-button callback ──> (Tier B) 1.2.6c
   inputs: 1.1.1 Accumulator, 1.1.2 Routing, 1.1.3 Momentum, 1.1.4 per-frame cap (pure, week 1-2); 1.2.5a persistence (pure, week 2; the hook takes `override` and
   `gestureScrollEnabled` as plain options, so 1.2.1b2 needs 1.2.5a only for the type and storage, NOT 1.2.5b/g)
1.2.3 CSS (independent)

Settings / chip / panel chain (components):
1.2.5a persistence ──> 1.2.5e useEffectiveScrollMode + onScrollModeChange ──┐
                       1.2.5f connectionEpoch ──────────────────────────────┼──> 1.2.5g option/prop threading (TerminalOutput -> XtermTerminal -> hook) ──> 1.2.5b1/b2/b3 panel, picker, chip, cue ──> 1.2.7a1/a2 scrollPosition.ts + JumpToLatestButton (pure, no TerminalOutput edit; can land earlier) ──> 1.2.9 toolbar keys (1.2.9b imports scrollPosition.ts `netPagesUp`) ──> 1.2.7b hook callback + mount
1.2.8 Redraw relabel needs 2.1.3b only

Phase 2 (XtermTerminal / TerminalOutput):
2.1.1 repaint seam + FitGuard ──> 2.1.2 wire sampler (refit) ──┐
2.1.4a/b settle module (pure) ─────────────────────────────────┴──> 2.1.4c settle subscription in TerminalOutput (FIRST TerminalOutput edit; old pipeline still present)
                                                                     ──> 2.1.3a delete old onVpResize pipeline, settle is sole path ──> 2.1.3b ──> 2.1.3c ──> 2.1.3d desktop check ──> 2.1.5 bounce bypass (Tier A)
        no merge between 2.1.4c and 2.1.3a (both pipelines would run); land them as one PR step
2.1.5 needs 2.1.3a (the settle callback) and flow-control files
2.1.6 hidden-tab drop needs Q5b (0.1.2f) AND 2.1.1 AND 2.1.3 (Tier B)

3.1.* verification needs all of the above that shipped; 3.1.3 device checklist needs 0.1.2a (manual instance)
```

The earlier circularity (2.1.3a "wires settle from 2.1.4c" while 2.1.4c "replaces what 2.1.3a deletes") is resolved by splitting responsibility: **2.1.4c adds** the settle subscription and its `refit()` call alongside the old pipeline; **2.1.3a then deletes** the old `onVpResize`/`isFittingRef` pipeline. Dependency note: 1.2.6 (paste guard) needs only 1.2.1 for its hook option; it is not downstream of 1.2.7.

**Shared-file edit order (one developer; no two tasks touch a file concurrently)**

| File | Order |
|---|---|
| `TerminalOutput.tsx` | 0.1.1c3 -> 2.1.4c -> 2.1.3a -> 2.1.3b -> 2.1.5c -> 1.2.8 -> 1.2.5f (epoch state) -> 1.2.5e (scroll-mode state, `useEffectiveScrollMode`) -> 1.2.5g (prop threading) -> 1.2.5b3 (mount panel, picker, chip) -> 1.2.5d (first-use hint host) -> 1.2.9 -> 1.2.7b -> (Tier B) 1.2.6c, 2.1.6 |
| `XtermTerminal.tsx` | 0.1.1c2 -> 2.1.2b1 -> 2.1.2b2 -> 2.1.2c1 -> 2.1.2c2 -> 2.1.2c3 -> 2.1.3c -> 1.2.1c (only if a handler needs `stopPropagation`) -> 1.2.5e (`onScrollModeChange`) -> 1.2.5g (props to the hook, `data-gesture-scroll`) -> (Tier B) 1.2.6c |
| `useTerminalGestures.ts` | 1.2.4a (tests only) -> 0.1.1b -> 1.2.4b -> 1.2.1b1 -> 1.2.1b2 -> 1.2.1b3 -> 1.2.1c -> 1.2.1d1 -> 1.2.1d2 -> 1.2.2b1 -> 1.2.2b2 -> 1.2.2b3a -> 1.2.2b3b -> 1.2.10 -> 1.2.5b2 (`onScrollGesture`, for the misroute cue) -> 1.2.5g (options type: `override`, `gestureScrollEnabled`, `onScrollGesture`) -> 1.2.5c -> 1.2.5d -> 1.2.7b -> (Tier B) 1.2.6c |
| `useTerminalFlowControl.ts` / `useTerminalStream.ts` | 0.1.1c4 -> 2.1.5b -> (Tier B) 1.2.6b |

**Test-name coverage map (validation.md is the source of the names; each task writes the rows for its module first)**

| Task(s) | validation.md rows implemented (name prefixes) |
|---|---|
| 0.1.1a, 0.1.1d | `log_*`, `dump_*`, `stats_*` |
| 1.1.1a | `push_*` |
| 1.1.2a | `decideScrollTarget_*`, `encodeWheel_*`, `pageAccumulator_*`, `encodePageKeys_*` |
| 1.1.3a | `momentum_*` and `momentumConstants_*` in `scrollKinematics.test.ts` |
| 1.1.4a | `clampLinesPerFrame_*`, `scrollDrag_should_CallScrollLinesEveryFrame_*`, `scrollDrag_should_DispatchOnceAndClamp_*` |
| 1.2.4a, 1.2.10a | `touchend_should_RunTapPath*`, `touchstart_should_EnterSelecting_*`, `touchstart_should_CancelGesture_*`, `doubleTap_*`, `selecting_*`, `touchcancel_*`, `touchmove_should_NotScrollOrPreventDefault_*`, `touchend_should_Tap_*`, `touchend_should_ClearSelection*` |
| 1.2.1a1-a3, 1.2.1c, 1.2.1d1-d2 | `scrollDrag_*`, `touchmove_*`, `touchend_should_PreventDefault_*`, `touchstart_should_IgnoreGesture_*`, `hook_should_RegisterTouchEventListeners_*`, `hook_should_RemoveAllListenersAndRafs_*`, `scrollLines_should_HaveSingleProductionCaller_*`, `viewportTouch_*` |
| 1.2.2a, 1.2.2b2, 1.2.2b3a-b | `momentum_*` hook tests |
| 1.2.3a | `globalCss_*`, `terminalCss_should_SetOverscrollBehaviorContainAndTouchActionNone` |
| 1.2.5a | `scrollOverride_*`, `gestureScroll_*` |
| 1.2.5b1-b2, 1.2.5d, 1.2.5e-g | `panel_*`, `picker_*`, `toggle_*`, `chip_*`, `hint_*`, `useEffectiveScrollMode_*`, `XtermTerminal_should_*`, `connectionEpoch_*` |
| 1.2.5c | `hook_should_RegisterNoTouchListeners_When_GestureScrollOff`, `hook_should_NeverPreventDefault_*`, `hook_should_ReRegisterListeners_*`, `terminalCss_should_RestoreDefaultTouchAction_*` |
| 1.2.7a1, 1.2.9a | `isAwayFromLive_*`, `netPagesUp_*`, `jumpToLatest_*`, `toolbarPageAction_*`, `toolbarKeys_*` |
| 2.1.1a | `postFitRepaint_*`, `canFit_*` |
| 2.1.2a1, 2.1.2a2 | `refit_*`, `sampler_*`, `onContextLoss_*` |
| 2.1.3a-b, 2.1.4c | `viewportResize_*`, `visibilityTrue_*`, `handleManualResize_*` |
| 2.1.4a | `viewportSettle_*` |
| 2.1.5a | `resize_*`, `handleTerminalResize_*` |
| 3.1.2a | `terminalResize_should_MatchProposeDimensionsAndPaintNonBlank_*` |

Parallelism is moot for one developer; if an agent or second contributor is used, the only safe overlaps are the pure modules (`lib/terminal/*` new files), the test-only tasks, and 1.2.3 CSS.

## Schedule and sequencing (Large, 3-6 weeks, one developer)

**Sizing**: the `Task x.y.zN` entries are checklist steps, each at most half a day (about 4 hours) including its test. Sizing, scheduling and tracking use the story-level estimates below.

| Story | Estimate | Story | Estimate |
|---|---|---|---|
| 0.0.1 install + baseline | 2 h | 2.1.1 repaint seam | 3 h |
| 0.0.2 bug docs | 2 h | 2.1.2 wire sampler (a1, b1, b2, a2, c1-c3) | 13 h |
| 0.1.1 debug log + instrumentation (a, b, c1-c4, **d stats**) | 13 h | 2.1.3 remove 400 ms path (a-d) | 10 h |
| 0.1.2 device spike (a-e) | device slot 1 | 2.1.4 settle signal (a-c) | 5 h |
| 0.1.2f Q4/Q5 | 4 h (Tier B) | 2.1.5 bounce bypass (a-c) | 6 h |
| 0.1.3 apply gate (a once, b at slot 2) | 2 h | 2.1.6 hidden-tab drop (Tier B) | 4 h |
| 1.1.1 accumulator | 3 h | 1.2.1 hook integration (a1-a3, b1-b3, c, d) | 19 h |
| 1.1.2 routing + encoders | 6 h | 1.2.2 momentum in hook (a, b1-b3) | 8 h |
| 1.1.3 momentum tracker | 4 h | 1.2.3 CSS | 1 h |
| 1.1.4 per-frame cap | 3 h | 1.2.4 regression anchors + **1.2.4b state-machine extraction** | 5 h + 6 h |
| **1.2.10 S9 behaviors (new)** | 6 h | 1.2.5 settings (a, c, d, **e, f, g new**, **b1-b3 split**) | 3 + 3 + 2 + 8 + 11 h |
| 1.2.7 jump to latest | 8 h | 1.2.8 Redraw relabel | 2 h |
| 1.2.9 route-aware toolbar keys | 5 h | 1.2.6 paste guard (Tier B) | 8 h |
| 3.1.1, 3.1.4 hygiene (non-device) | 6 h | 3.1.2 e2e (Tier B) | 3 h |

**Honest total** (recomputed from the story table above, which sums to **184 h** of build, device-free): the earlier tally was 143 h + about 34 h = 177 h and missed about 7 h of table rows. Build is about 184 h (about 23 working days). Device-bound work is **about 5 working days (40 h)** in three booked slots (below). Task total about **224 h = 5.6 weeks at 40 productive hours per week**. A fix-pass reserve after device findings of **12 h** brings it to **about 236 h = 5.9 weeks**.

**Slack, stated plainly**: against the 6-week ceiling (240 h) the real slack is **about 4 h (half a working day, under 2%)**. That is far below the 20-30% usually carried for UI work with unverified device behaviour. Tier A alone (about 165 h build + 40 h device + 12 h reserve = about 217 h) is **about 5.4 weeks**; Tier B (15 h after 0.1.2f's 4 h in week 4) only fits if nothing slips. The plan is therefore credible only as "**Tier A build done end of week 5, Tier A device-verified early week 6, Tier B only if the remaining slack survives**". Any slipped device slot or a failed D9/D8 consumes the Tier B window first. This replaces the earlier "Tier A done, including the second device pass, by the end of week 4", which the arithmetic did not support (week 4 held 50+ hours of work before the revision).

**Device calendar (the single external dependency; reserve now)**: the operator's Android phone and a shared network are needed in three slots. Give the operator **at least 1 week of lead time before each slot** and book them at planning time. Slot 1: last day of week 1 (1 day, Tasks 0.1.2a-e, baseline blank count). Slot 2: last day of week 4 plus first day of week 5 (2 days, core-fix pass: D1, D1b, D2, D2c, D2e, D3, D4, D6, D9 jank and slop, D10, plus 0.1.3b, plus a 5-10 cycle D7 smoke with `debug-terminal-mobile` on: check `bypassed:true` appears in the resize log and no blank; the full N-cycle run stays in slot 3). Slot 2 predates the panel, picker and chip (built in week 5), so D2 override cases at slot 2 use default routing or a manual `localStorage` edit of the override key; the picker-driven cases move to D2b at slot 3. Slot 3: first 1.5 days of week 6 (D2b, D2f, D5, D7 N-cycle run (N up to 300 cycles, about 100 minutes), D8, D11). If a slot slips, work moves to the next unblocked item in the chain and the Tier A "verified" date slips by the same amount; if no device exists by the end of week 2, the no-device path in Task 0.1.2d applies and D7 is reported unproven.

Tiers are **sequencing, not cutting**: all scope is kept (decision 2026-10-01). The only cut is the overrun rule in requirements.md.

| Tier | Stories |
|---|---|
| A | 0.0.1, 0.0.2, 0.1.1, 1.2.4, 1.1.1-1.1.4, 1.2.1, 1.2.2, 1.2.3, 1.2.5, 1.2.7, 1.2.8, 1.2.9, 1.2.10, 2.1.1-2.1.4, **2.1.5 (Q3(d) may make it the primary blank-terminal fix; requirements.md says the redraw fix always ships)**, 3.1.1, 3.1.4 |
| B (hardening, not defect fixes; see requirements.md traceability table) | 0.1.2f (Q4/Q5), 1.2.6, 2.1.6 (tied to a defect only if Q5b observes a blank or stale canvas on return), wheel/X10 encoders, 3.1.2 (e2e) |
| Device-gated (interleaved) | 0.1.2a-e (slot 1), 0.1.3a (week 2, after 1.1.2b), 0.1.3b and 3.1.3 (slots 2 and 3) |

| Week | Plan (about 40 productive hours per week; device days counted) |
|---|---|
| 1 | 0.0.1; 0.0.2; 0.1.1 (a, b, c1-c4, d); 1.2.4a regression anchors (green on the unmodified hook); 1.1.1 accumulator; **1.1.2 routing rows (needs to exist before 0.1.3a)**. **Device slot 1, last day: 0.1.2a-e** (mode matrix, renderer, container-vs-viewport, baseline blank count, xterm Gesture competition). About 31 h build + 8 h device |
| 2 | **0.1.3a apply the gate (once; needs 1.1.2b and the slot 1 findings)**; 1.1.3, 1.1.4; 1.2.5a persistence; 2.1.1; 2.1.4a-b; 1.2.3 CSS; **1.2.4b extract the gesture state machine** (under the 1.2.4 anchors); 2.1.2 (a1, b1, b2, a2, c1-c3). About 38 h |
| 3 | Hook chain 1.2.1 (a1-d) and 1.2.2 momentum; then **2.1.4c -> 2.1.3a-d** (settle subscription, then delete the old pipeline; desktop check 2.1.3d). About 38 h |
| 4 | 2.1.5 bounce bypass; 1.2.8 Redraw relabel; 1.2.10 S9 behaviors; 1.2.5c Gesture-Off option; 1.2.5e/f/g (effective mode, epoch, threading); **0.1.2f desk part (Q4a/b, Q5a, 4 h, Tier B, uses the buffer)**; remaining hours are overflow buffer. **Core-fix pass = Device slot 2 starts on the last day** (Q5b is observed there). About 25 h build + 4 h Q4/Q5 + 8 h device + 3 h buffer |
| 5 | Slot 2 day 2 (8 h) and 4 h of the fix-pass reserve; **1.2.5b1-b3 panel, picker, chip, misroute cue; 1.2.5d hint; 1.2.9 toolbar keys; 1.2.7 jump to latest** (the panel and toolbar-equivalence work, moved here from week 4; 26 h). **Tier A build done at the end of this week.** About 38 h |
| 6 | **Device slot 3** (12 h: D2b, D2f, D5, D7, D8, D11); the remaining 8 h of the fix-pass reserve; 3.1.1/3.1.4 hygiene (6 h); PR. That leaves about 4 h of real slack against the remaining Tier B (1.2.6 8 h, 2.1.6 4 h, 3.1.2 3 h = 15 h): only part fits; whatever does not fit goes to the backlog under the overrun rule |

## Phase 0: Spike and instrumentation

### Epic 0.0: Environment and tracking (first, before any TDD)

#### Story 0.0.1: Install dependencies and run the baseline
**Acceptance Criteria**:
- *Given* a clean worktree (`web-app/node_modules` is absent), *When* `cd web-app && pnpm install && npx jest --no-coverage --testPathPatterns="terminal|XtermTerminal|useTerminalGestures|useVisibilityResync"`, *Then* the baseline result (pass/fail counts, any pre-existing failures named) is recorded in this file under "Baseline test run" before any code change, so later failures are attributable. Tests have never been run in this worktree.
**Files**: `plan.md` (Baseline test run note)

##### Task 0.0.1a: `pnpm install` and baseline jest run (first task of the project)
- Use pnpm only (`docs/how-to/use-pnpm-in-web-app.md`). Also run `pnpm run lint:duplicates` once to record the pre-change jscpd baseline (about 0.09%). Re-run `lint:duplicates` weekly (end of each week from week 2) and `make ready` once at the end of week 3 and again in week 5, so duplication and lint findings surface before week 6.
- Files: none

#### Story 0.0.2: Tracking bug entries for the two defects
**Acceptance Criteria**:
- *Given* the repo convention `docs/bugs/open/BUG-NNN-<slug>.md` (verified: 15 files there today), *When* this story completes, *Then* two entries exist: one for dead drag-down scrolling and one for the intermittent blank terminal after keyboard open, each linking requirements.md and this plan and stating how it closes (verification record: D1/D2 and the D7 blank count). Use the next free BUG number at creation time.
**Files**: `docs/bugs/open/BUG-<next>-mobile-touch-drag-scroll-one-directional.md`, `docs/bugs/open/BUG-<next+1>-mobile-terminal-blank-after-keyboard-open.md`

##### Task 0.0.2a: Create the two bug entries
- Follow the format of an existing entry (read one first). Move to `docs/bugs/fixed/` when verified.
- Files: as above

### Epic 0.1: Find the real causes on a device
**Goal**: Turn the INFERRED items into VERIFIED and record the blank-terminal baseline. The spike confirms or adjusts shipped default rows post-hoc; it does not block merge.

#### Story 0.1.1: Debug logging behind a flag
**As a** developer, **I want** structured scroll/fit logs I can read from a phone, **so that** I can prove the causes.
**Acceptance Criteria**:
- Logging is off unless `localStorage['debug-terminal-mobile']==='true'`; zero overhead path when off.
  - *Given* `MobileDebugLog` with the flag unset, *When* `log('scroll', {...})` is called 1000 times, *Then* the ring buffer length is 0 and `console.debug` is not called.
- Ring buffer keeps the last 500 entries and `window.__termDebug.dump()` returns JSON.
  - *Given* flag set and 600 entries logged, *When* `dump()` runs, *Then* it returns 500 entries, oldest dropped.
**Files**: `lib/terminal/mobileDebug.ts`, `lib/terminal/__tests__/mobileDebug.test.ts`

##### Task 0.1.1a: Create `MobileDebugLog` + test
- Pure module; flag read lazily; ring buffer; `window.__termDebug` registration. Test with `localStorage` stubbed.
- Files: `lib/terminal/mobileDebug.ts`, `lib/terminal/__tests__/mobileDebug.test.ts`

##### Task 0.1.1b: Scroll instrumentation in the gesture hook
- In the SCROLLING rAF callback (`lib/hooks/useTerminalGestures.ts` ~L239-247) log `{bufferType: terminal.buffer.active.type, mouseTrackingMode: terminal.modes.mouseTrackingMode, viewportY, baseY, rows, cellH, moveDy, lines}`; also log PENDING->SCROLLING transition and `touchcancel`/non-cancelable `touchmove` (`e.cancelable`).
- Files: `lib/hooks/useTerminalGestures.ts`

##### Task 0.1.1c1: Viewport instrumentation (`ViewportProvider.tsx`)
- Log vv `height` and `offsetTop` on every vv `resize` **and** `scroll` event (`components/providers/ViewportProvider.tsx:34-55`).
- Files: `components/providers/ViewportProvider.tsx`

##### Task 0.1.1c2: Sampler, renderer and context-loss instrumentation (`XtermTerminal.tsx`)
- Log sampler start, each tick, confirmed fit, give-up (~L1014-1149), zero-size skip (~L1144), `onContextLoss`, and the active renderer.
- Files: `components/sessions/XtermTerminal.tsx`

##### Task 0.1.1c3: `onVpResize` and clear-to-snapshot timing (`TerminalOutput.tsx`)
- Log `onVpResize` (~L1172: skipped by `isFittingRef`? fit scheduled?) and the time from `clearBufferBeforeResize()` (`TerminalOutput.tsx:909`) to the next full-snapshot write (`TerminalStreamManager` `onFullSnapshot`, `:268-270`).
- Files: `components/sessions/TerminalOutput.tsx`

##### Task 0.1.1c4: Resize outcome logging (`useTerminalFlowControl.ts`)
- In `resize()` (L292-418) log each call's outcome (`deduped | deferred | bounce(holdMs, streak) | sent`, plus whether `bypassBounceHold` was set); the existing `console.log` at L397 covers only bounce.
- Files: `lib/hooks/useTerminalFlowControl.ts`

##### Task 0.1.1d: `stats()`, `route-decision`, `override-change` and `misroute-proxy` logging (Observability Plan; requirements.md misroute outcome)
- Extend `mobileDebug.ts` with `stats()` (counts since page load; `window.__termDebug.stats()`), the `route-decision {target, source, unverified}`, `override-change {from, to}` and `misroute-proxy {kind}` entry types, and the proxy rules (`override-after-drag` within 10 s of an auto-routed drag; `toolbar-key-after-drag` within 5 s). Pure module with injected clock; the call sites (routing decision in the hook, override setter, toolbar key handlers) are added in Tasks 1.2.1b2, 1.2.5a and 1.2.9b. Tests (named in validation.md REQ-18, written first): `stats_should_CountOverrideChangeAndMisroute_When_OverrideChangedWithin10sOfAutoDrag`; `stats_should_CountToolbarKeyAfterDrag_When_Within5s`; `stats_should_BeUnavailableAndZeroOverhead_When_FlagUnset`.
- Files: `lib/terminal/mobileDebug.ts`, `lib/terminal/__tests__/mobileDebug.test.ts`

#### Story 0.1.2: Manual-instance device spike
**As a** developer, **I want** a reproducible device run on an isolated instance, **so that** the live service on :8543 is never restarted.
**Acceptance Criteria**:
- Each of Q1-Q3 has a recorded verdict with log excerpts.
  - *Given* a Claude Code session in the manual instance and `debug-terminal-mobile` set on the phone, *When* SGR wheel bytes `\x1b[<65;10;10M` x5 are sent via the console, *Then* the verdict "Claude Code scrolls on wheel: yes/no" is written to Spike Findings.
**Files**: `project_plans/scrolling/implementation/plan.md` (Spike Findings section, appended)

**Manual-instance recipe** (CLAUDE.md "Manual/interactive testing"; ports from the manual block):
```bash
cd web-app && pnpm install && pnpm build      # node_modules is absent in this worktree
cd .. && mkdir -p ~/.stapler-squad/manual-builds/manual-1
go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test \
  ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server &
# open http://<laptop-lan-ip>:62871 on the Android phone (same network); stop with: kill %1
```
Never use `make install-service` for this. Phone side: set `localStorage['debug-terminal-mobile']='true'`; attach `chrome://inspect` for console if a cable is available, else use `__termDebug.dump()` and paste.

##### Task 0.1.2a: Build and expose the manual instance (device-gated)
- Run the recipe; verify phone loads the app and can open a Claude Code session.
- Files: none

##### Task 0.1.2b: Q1 protocol: wheel vs PgUp/PgDn (device-gated)
- In a Claude Code session with long output: (1) toolbar PgUp/PgDn baseline works; (2) send SGR wheel up/down from the console via the terminal `onData` path; (3) log `modes.mouseTrackingMode` while Claude Code is foreground and whether SGR is accepted; (4) confirm the app enabled 1006 encoding (xterm does not expose it; check that `\x1b[<65;10;10M` scrolls rather than echoing garbage; also try the X10 form). Record: wheel works yes/no, mouseTrackingMode value, encoding confirmed yes/no. Until encoding is confirmed, `TuiScrollPolicy` stays `'pgkeys'`.
- Files: Spike Findings in `plan.md`

##### Task 0.1.2c: Q2 protocol: drag-down failure and mode matrix (device-gated)
- With logging on, drag up then down in (i) a plain shell with scrollback **inside tmux** (the real deployment: stapler-squad sessions are tmux), (ii) Claude Code in the same tmux session. For each, record `bufferType`, `mouseTrackingMode`, `viewportY/baseY`, `lines`, any `touchcancel`/`cancelable=false`, page pull-to-refresh, and which layer actually moved (xterm scrollback, tmux copy-mode, the TUI). Fill the **mode matrix** in Spike Findings: `{scenario, bufferType, mouseTrackingMode, what-works (local/wheel/pgkeys)}` for (a) plain shell in tmux, (b) Claude Code, (c) optionally a bare shell with no tmux if reachable. Also record tmux copy-mode behavior (does toolbar PgUp enter copy-mode, does PgDn at bottom exit it, is tmux `mouse on`). Also check whether xterm's built-in viewport touch scrolling moves the normal buffer concurrently (double-scroll).
- Files: Spike Findings in `plan.md`

##### Task 0.1.2d: Q3 protocol: black screen (device-gated)
- **Baseline blank rate (measure first when a device exists; it is needed to *claim* improvement, not to merge the fix)**: on the unmodified build with logging only (the week-1 build), run open/close keyboard cycles and record **blanks per N cycles** (target N=50, include one rotation); this is the baseline `b` the fix is judged against. The post-fix run uses `N = ceil(3 / b)` (rule of three, 95% confidence; requirements.md "Rule for N"). If no blank occurs in the baseline run, record "baseline not reproduced in N" and keep the D7 claim unproven. Reconciliation with the Routing Verification Statement: Story 2.1.2 and 2.1.5 merge without a baseline; the PR then says "baseline not measured" and D7 unproven. Nothing in this plan requires the baseline *before the code lands*, only before a claim is made.
- **No-device path**: if no Android phone is available by the end of week 2, record "baseline not measured (no device)" and "Q3 verdicts not run" in Spike Findings, keep all rows `ROUTING_VERIFIED=false`, ship the Q3 branches' code unchanged (it is built for every outcome), and say so in the PR. A desktop Chrome DevTools device-mode resize check can exercise `refit()` and the jest 200-cycle anchor, but it cannot produce a blank rate and must not be reported as one.
- First, before any cycling, record: (1) the **active renderer** (WebGL vs canvas; the log line "WebGL2 unavailable (Android?), using canvas renderer" at ~`XtermTerminal.tsx:606`), and (2) whether the **container's** `clientHeight` changes when the keyboard opens or only `visualViewport.height` does. These two verdicts decide whether the WebGL fixes (`clearTextureAtlas`, `onContextLoss` repaint) are even reachable on the phone and whether the ResizeObserver ever fires.
- Then open/close the keyboard repeatedly until blank; at failure `dump()` and read: last fit size vs `visualViewport.height`, whether fit was skipped (zero-size / `isFittingRef` / sampler give-up), renderer, any `onContextLoss`. Try `terminal.refresh(0, rows-1)` from the console to see if that alone recovers (distinguishes missed-repaint from wrong size).
- **Q3(d), bounce hold (QoS)**: during the same cycles, read the resize log: does the keyboard-close `resize()` report `bounce` with `holdMs >= 3000`, and is the canvas empty (xterm cleared) for that whole window? If yes, Story 2.1.5 is a primary black-screen fix and its regression test is a primary proof; if the canvas is blank with **no** bounce hold, 2.1.5 is a secondary fix (still shipped, it is cheap). Also record `visualViewport.offsetTop` at the failing moment (nonzero supports the Story 2.1.4 offsetTop change).
- Files: Spike Findings in `plan.md`

##### Task 0.1.2e: Record verdicts
- Fill the Spike Findings table (Q1-Q5: verdict, evidence, date).
- Files: `plan.md`

##### Task 0.1.2f: QoS transport and lifecycle investigation (Q4, Q5; investigation only, no code change)
Source: `research/qos.md` gaps 1 and 3 and open items. Record findings only; nothing here blocks merge, except that Story 2.1.6 needs the Q5 verdict.
- **Streaming path (Q4a)**: the client uses `createWebsocketBasedTransport` (`useTerminalStream.ts:7,176`); the server registers `wsHandler.HandleWebSocket` at `"/api" + SessionServiceStreamTerminalProcedure` "before the unary handler" (`server/server.go:403-408`), so the WebSocket path (`connectrpc_websocket.go:765` `HandleWebSocket` -> `:881` `streamTerminal` -> `:1208` `streamViaControlMode` or `:1937` `streamViaHub`) is the expected production path (INFERRED; confirm). Confirm on the manual instance from `~/.stapler-squad/logs/staplersquad.log`: `[streamViaControlMode]`/hub lines versus `resized terminal` / `[FlowControl]` lines from `session_service.go`, for a normal session and (if available) an external one.
- **FlowControl honored? (Q4b)**: `grep -c FlowControl server/services/connectrpc_websocket.go` returns 0 (VERIFIED 2026-10-01); `session_service.go:4503` (`case *sessionv1.TerminalData_FlowControl`) logs `[FlowControl] client requested PAUSE/RESUME` and signals `pauseCh` (created L4208). Provoke it: run `yes | head -n 2000000` in a session on the phone; if the client logs `Sending flow control: paused=true` (`useTerminalFlowControl.ts:446`) but the server log never prints `[FlowControl] client requested PAUSE`, the pause is a no-op on the production path. Record yes/no.
- **Input under output flood (Q4c)**: during that flood press toolbar PgUp and Ctrl-C; record observed round-trip. No instrumentation exists for `MessageQueue` depth; this is qualitative and informs Story 1.2.6 only.
- **RECONNECT_V2 (Q5a)**: `NEXT_PUBLIC_*` is inlined at `pnpm build` time. The repo has `web-app/.env.local.example:1` with the flag commented out, and no reference in `Makefile` or `.github` (grepped the whole repo excluding `node_modules`; VERIFIED). A default build therefore has it **off** (INFERRED for the deployed build). Check the actual bundle (grep the built output for the gated code path) or `web-app/.env.local` on the build host. Flag off: `TerminalOutput.tsx:969` and `:1067` own reconnect; flag on: `useTerminalStream.ts:211,332,553,691` do.
- **Visibility resume (Q5b)**: facts already read: `useVisibilityResync.ts` debounces `visibilitychange` 300 ms (`:7`, `:349`) and, if connected, calls `requestFullResync(true, true)` (`:297`), which only sends a `currentPaneRequest`; it does not drop the client write queue or force a repaint. `TerminalStreamManager.writeStateBatched` accumulates into `writeBuffer` and flushes in `requestAnimationFrame` (`:558-563`), which does not run while the tab is hidden (qos.md #15), so a backlog builds, flushes on return, and is then cleared by the snapshot (`ANSI_SNAPSHOT_PREFIX` handling, `:268`). On a device: background the tab 30 s during output, return, record (i) flash of stale content, (ii) blank canvas, (iii) time to correct content. Note the plan's `refit()` `reason:'visibility'` covers only the `isVisible` **prop** effect (`TerminalOutput.tsx:1160-1165`), not `document.visibilitychange`.
- Files: Spike Findings in `plan.md`

#### Story 0.1.3: Decision gate
**As a** planner, **I want** findings mapped to branches, **so that** the shipped default rows are confirmed or corrected and all candidate fixes still ship.
**Acceptance Criteria**:
- Branch table filled.
  - *Given* Q1 = "wheel ignored, PgUp works", *When* the gate is applied, *Then* `TuiScrollPolicy` is `'pgkeys'` and the wheel encoder is kept but unused.
**Branch table**:

| Finding | Branch |
|---|---|
| Q1 wheel works | `TuiScrollPolicy='wheel'` for tracking mode; PgUp/PgDn only for alt-screen without tracking |
| Q1 wheel ignored, or 1006 encoding not confirmed | `TuiScrollPolicy='pgkeys'` everywhere in TUI modes; page-sized steps via a page accumulator (see Story 1.1.2): one `\x1b[5~/\x1b[6~` per emitted page, TUI mode is explicitly **non-1:1** |
| Q2 alt-buffer no-op | Routing (Epic 1.1) is the primary fix |
| Q2 Claude Code renders in the **normal** buffer (`bufferType:'normal'`, tracking `none`) | `bufferType` cannot identify the TUI. Add a table row keyed on the distinguishing signal the spike finds (e.g. tracking mode, or tmux pane state); if none distinguishes it from a plain shell, default to `tui-pgkeys` for tmux-hosted sessions and expose the `'auto'\|'local'\|'tui'` toggle so the user picks. Do not ship the `'alternate'`-only rule |
| Q2 plain shell inside tmux reports `bufferType:'alternate'` (tmux uses alt screen) | `bufferType==='alternate'` alone must not route to PgUp/PgDn (bash ignores it). Rule: alternate + tracking `none` + tmux session -> `tui-pgkeys` only if spike shows tmux honors it; otherwise `xterm-local` is a no-op and the table routes to wheel/pgkeys per Q1. **tmux copy-mode behavior (INFERRED from tmux defaults, verify in spike)**: with tmux's default root key table, `PageUp` in a pane enters copy-mode (`copy-mode -eu`) and scrolls one page; further PgUp/PgDn move through tmux's history; PgDn back to the bottom exits copy-mode (the `-e` flag); with tmux `mouse on`, wheel events do the same. So for a plain shell in tmux, `tui-pgkeys` scrolls the tmux history (not bash), but a Claude Code pane receives PgUp itself (it is foreground and tmux forwards keys to it, unless tmux copy-mode is already active). The spike records for each scenario: is the pane in copy-mode after PgUp, does PgDn exit it, does `mouse on` apply, and whether a stale copy-mode swallows subsequent keystrokes to the app. Record per-scenario rows in the mode matrix; the override toggle covers any misclassified case |
| Q2 mode matrix shows shell and Claude Code are distinguishable by `mouseTrackingMode` | Key rows on `mouseTrackingMode` (wheel when tracking on and Q1 says wheel works; else pgkeys), bufferType as secondary |
| Q2 sub-cell loss / stolen gesture | Accumulator and CSS hardening are primary; still ship routing |
| Q3 container size does NOT change when keyboard opens (only `visualViewport` shrinks) | RO never fires, so `refit()` (direct sampler start, no RO dependency) on the settle signal is the only trigger; `refit()` additionally forces a container reflow check (reads `offsetHeight`, compares with `visualViewport.height`, logs a mismatch); `--viewport-height` consumers (ViewportProvider) must drive container height or the plan's premise needs revisiting |
| Q3 active renderer is canvas (WebGL2 unavailable on the phone) | WebGL-specific fixes (`clearTextureAtlas`, `onContextLoss` repaint) are moot on the phone; `refresh(0, rows-1)` after `refit()` is the primary proof and must work with the canvas renderer; `postFitRepaint` stays conditional on renderer |
| Q3(d) keyboard close reported as `bounce` with hold >= 3 s and canvas blank meanwhile | Story 2.1.5 (bypass hold for viewport-settle refits) is a primary black-screen fix; its test is a primary proof alongside the repaint tests |
| Q3(d) no bounce hold observed | Story 2.1.5 ships as secondary hardening; primary proof stays with the other Q3 rows |
| Q4 FlowControl not honored on production path | Record only; feeds Deferred item D1. No change in this project |
| Q5 `RECONNECT_V2` off in deployed build | Record only (`TerminalOutput.tsx:969/1067` owns reconnect). Do not flip the flag in this project |
| Q5 stale flash or blank canvas on return from hidden tab | Ship Story 2.1.6; if neither is observed, drop 2.1.6 and record why |
| Q3 stale size | `ViewportSettled` + single-owner fit are primary proof |
| Q3 missed repaint | `postFitRepaint` is primary proof |
| Q3 zero-size or WebGL loss | `FitGuard` retry / `onContextLoss` repaint are primary proof |

##### Task 0.1.3a: Apply the gate (scheduled ONCE, week 2; depends on Task 1.1.2b and on Tasks 0.1.2c/e)
- **Scheduling fix**: this task was listed in both week 2 and week 6 and depends on the rows file that Task 1.1.2b creates. It is now one task: it runs in week 2 after 1.1.2b has landed (1.1.2 is moved to week 1) and after device slot 1 has produced the mode matrix. If slot 1 slipped, it runs on the first working day after the matrix exists; until then the shipped default rows stand and `ROUTING_VERIFIED=false`. The week-6 item is **Task 0.1.3b**, below.
- Compare the Q2 mode matrix with the shipped default rows (Routing Verification Statement: `alternate -> tui-pgkeys`, `tracking != none -> tui-pgkeys`, default `xterm-local`). Confirm them or edit the rows in `lib/terminal/scrollRouting.ts` (Task 1.1.2b), set `TUI_SCROLL_POLICY` per Q1, then set `ROUTING_VERIFIED = true`; note primary regression test per Q3 in Spike Findings. If the spike is not run, the default rows ship as unverified (PR states routing is unverified and lists the misroute risk); the former safe default (local for everything) is withdrawn.
- Files: `plan.md`, `lib/terminal/scrollRouting.ts`

##### Task 0.1.3b: Re-confirm the rows at device slot 2 (verification only)
- During the slot 2 core-fix pass (D1b, D2) compare the observed routing for a tmux shell and Claude Code with the rows set by 0.1.3a. No code change unless the pass contradicts them; if it does, edit the rows (data) and note it in Spike Findings. Flip `ROUTING_VERIFIED` only here, if still `false`.
- Files: `plan.md`, `lib/terminal/scrollRouting.ts` (only on contradiction)

**Spike Findings** (to fill in Phase 0):

| Q | Verdict | Evidence | Date |
|---|---|---|---|
| Q1 | _pending device_ | | |
| Q2 | _pending device_ | | |
| Q3 (incl. Q3(d) bounce hold) | _pending device_ | | |
| Q3 baseline blank rate (blanks / N cycles, unmodified build) | _pending device_ | | |
| Q4 (streaming path; FlowControl honored) | _pending_ (investigation only) | | |
| Q5 (RECONNECT_V2 value; visibility resume) | _pending device_ | | |

**Mode matrix** (Task 0.1.2c; drives the `ScrollRoutingPolicy` rows):

| Scenario | bufferType | mouseTrackingMode | What works (local / wheel / pgkeys) | tmux copy-mode behavior (enters on PgUp? PgDn exits? `mouse on`?) |
|---|---|---|---|---|
| Plain shell in tmux | _pending device_ | _pending device_ | | |
| Claude Code (in tmux) | _pending device_ | _pending device_ | | |
| Bare shell, no tmux (optional) | _pending device_ | _pending device_ | | n/a |

---

## Phase 1: Touch scrolling

### Epic 1.1: Pure scroll modules (no DOM, fake-clock tested)
**Goal**: Correctness lives in small pure modules; jest with injected clock/rAF, no real sleeps.

#### Story 1.1.1: Fractional-line accumulator
**As a** mobile user, **I want** slow drags to scroll 1:1, **so that** content follows my finger in both directions.
**Acceptance Criteria**:
- Remainder is carried; up and down are symmetric.
  - *Given* `ScrollAccumulator` with cellH 18, *When* 10 frames each push 3 px up, *Then* total emitted `LineDelta` is 1 (30/18 truncated) and the carried remainder is 12 px; the same with -3 px yields -1.
- First-slop distance is not dropped.
  - *Given* a touch that crossed the slop (`SLOP_PX`=15) at 22 px total travel, *When* scrolling begins, *Then* the accumulator is seeded with the overshoot only (22 - 15 = 7 px), so the first frame emits no line jump (UX AC2: no initial jump at slop crossover); later movement is 1:1. The fixed `SLOP_PX` finger-to-content offset is accepted. `SLOP_PX` is **15 px** (the long-press timer rationale in the existing comment); 10 px is a fallback candidate tried in D9 only if 15 fails ux.md tuning criterion (c), and is adopted only if tap and long-press still pass (tap tolerance moves with the slop). Tests import `SLOP_PX` instead of the literal.
**Files**: `lib/terminal/scrollKinematics.ts`, `lib/terminal/__tests__/scrollKinematics.test.ts`

##### Task 1.1.1a: Write failing accumulator tests
- Cases: symmetry, remainder carry, 1000-frame zero drift (sum of lines within +-1 of travel/cellH), `reset()` on cellH change.
- Files: `lib/terminal/__tests__/scrollKinematics.test.ts`

##### Task 1.1.1b: Implement `ScrollAccumulator`
- `push(dyPx, cellH): LineDelta` using `Math.trunc` on `acc`; `reset()`.
- Files: `lib/terminal/scrollKinematics.ts`

#### Story 1.1.2: Mode-based scroll routing
**As a** mobile user, **I want** the drag to reach the same scrollback the PgUp/PgDn keys reach, **so that** Claude Code scrolls.
**Acceptance Criteria**:
- Routing is a decision table, not a hardcoded `bufferType` check.
  - *Given* the shipped default table (`ROUTING_VERIFIED=false`; Routing Verification Statement rows) and override `'auto'`, *When* `decideScrollTarget`, *Then* `{alternate, any tracking} -> 'tui-pgkeys'`, `{normal, tracking != none} -> 'tui-pgkeys'`, `{normal, none} -> 'xterm-local'`; with override `'local'` always `'xterm-local'`, with `'tui'` always `'tui-pgkeys'`. Named test: `decideScrollTarget_should_RouteAlternateToTuiPgkeys_When_DefaultTableAndAuto` (replaces the former `..._NeverReturnTui_When_RoutingUnverified...` guard).
  - *Given* `ROUTING_VERIFIED=false`, *When* a routing decision is made, *Then* a `MobileDebugLog` entry carries `unverified:true`.
  - *Given* an injected table that replaces a default row (e.g. `{alternate, none} -> 'xterm-local'` after the spike shows tmux shells misroute), *Then* that mode resolves per the injected row; changing only the table data changes the result, with no code change.
  - *Given* a table row for "TUI in normal buffer" (spike-derived, e.g. keyed on tracking mode) and a matching mode, *Then* the target is `'tui-*'` even though `bufferType==='normal'` (pins the blocker: normal-buffer TUI is not forced to `xterm-local`).
  - *Given* a table row for "plain shell in tmux (alternate screen)", *Then* it resolves per the spike row, not to PgUp/PgDn by default.
  - *Given* override `'local'` or `'tui'`, *Then* the table is ignored and that target is returned for every mode.
  - *Given* `{alternate, tracking:'any'}` with `TuiScrollPolicy` `'pgkeys'`, *Then* `'tui-pgkeys'`; with `'wheel'`, *Then* `'tui-wheel'`. **Invariant test**: for every table row and every mode, `'tui-wheel'` is returned only when `TuiScrollPolicy==='wheel'` (default `'pgkeys'`).
- Encoders emit exact bytes.
  - *Given* `LineDelta=-2` (toward older), col 10, row 5, *When* `encodeWheel`, *Then* two `\x1b[<64;10;5M` reports; for +3, three `\x1b[<65;10;5M`; for pgkeys older, `\x1b[5~`.
  - *Given* wheel coordinates, *Then* col/row are the touch-start cell (`startCol`/`startRow`, `useTerminalGestures.ts:74-75`), 1-based and clamped to `[1, cols]`/`[1, rows]`; SGR is the only emitted encoding unless the spike (Q1) confirms another (X10 encoder kept but unused; a test asserts it is unreachable from `decideScrollTarget`).
- Page-key mode is page-accumulated, rate-limited, and explicitly non-1:1.
  - *Given* policy `pgkeys`, rows 24 (`TUI_PAGE_STEP_LINES` = floor(23/2) = 11 lines, half a page), *When* post-slop travel is 1 line, *Then* zero page keys are emitted; *When* cumulative post-slop travel reaches 11 lines, *Then* exactly one `\x1b[5~`/`\x1b[6~` is emitted and the remainder is carried (no `ceil`, no minimum-one-page); at 22 lines, two keys (subject to the per-frame and rate limits).
  - *Given* a momentum fling in pgkeys mode, *Then* total emitted pages per fling are capped (default 5) and at most one page is emitted per 100 ms (page rate-limit), after which momentum is cancelled.
**Files**: `lib/terminal/scrollRouting.ts`, `lib/terminal/__tests__/scrollRouting.test.ts`, reuse `lib/terminal/mouseTracking.ts`

##### Task 1.1.2a: Write failing routing/encoder tests
- Table-driven over mode x policy; wheel SGR and X10 bytes; per-call cap (<=3 reports/frame) assertion; direction sign tests.
- Files: `lib/terminal/__tests__/scrollRouting.test.ts`

##### Task 1.1.2b: Implement `decideScrollTarget`, `ScrollRoutingPolicy` table + `TuiScrollPolicy`
- Pure function over the data table; override param (`'auto'|'local'|'tui'`, persisted in localStorage, small toggle in the terminal toolbar/settings). `TUI_SCROLL_POLICY` default `'pgkeys'` (proven working per requirements) until Task 0.1.3a changes it; policy is also a hook option for test injection.
- Default rows (decision 2026-10-01, see Routing Verification Statement): `alternate -> tui-pgkeys`, `tracking != none -> tui-pgkeys`, `default: 'xterm-local'`, `ROUTING_VERIFIED=false`, with an `unverified:true` log field while false. The Q2 mode matrix confirms or edits these rows and flips `ROUTING_VERIFIED`. Known cost (stated in the PR): the `alternate` row misroutes a plain tmux shell and a normal-buffer Claude Code with tracking `none` routes local; the S6 override fixes either.
- Files: `lib/terminal/scrollRouting.ts`

##### Task 1.1.2c: Implement `encodeWheel` / `encodePageKeys` + `PageAccumulator`
- SGR with 1-based, clamped col/row from touch-start cell; X10 encoder retained but not selectable. Page mode: a second accumulator in step units (`TUI_PAGE_STEP_LINES`, half a page) that emits one page key only when post-slop travel >= one step, carries the remainder, with per-fling cap and 100 ms rate-limit (injected clock).
- Files: `lib/terminal/scrollRouting.ts`

#### Story 1.1.3: Momentum tracker
**As a** mobile user, **I want** a fling to coast and stop, **so that** long scrollback is reachable, with reduced-motion respected.
**Acceptance Criteria**:
- Decay with injected clock.
  - *Given* samples ending at 1.2 px/ms upward and fake rAF at 16 ms, *When* released, *Then* per-frame velocity decays by 0.95 and ends below 0.02 px/ms in a finite frame count (no more than 120 frames). Constants are the canonical `MOMENTUM_CONSTANTS` set (window 100 ms; min fling 0.3 px/ms; decay 0.95/16 ms frame; stop 0.02 px/ms; velocity cap 8 px/ms; max 120 frames), asserted by a test so ux.md and code cannot drift.
- Reduced motion disables it.
  - *Given* `ReducedMotionPref` true, *When* released at 1.2 px/ms, *Then* `start()` returns no frames.
- Cancel is immediate.
  - *Given* active momentum, *When* `cancel()` (touchstart/resize/mode change), *Then* no further frames are emitted.
**Files**: `lib/terminal/scrollKinematics.ts`, `lib/terminal/__tests__/scrollKinematics.test.ts`

##### Task 1.1.3a: Write failing momentum tests with fake clock/rAF
- Velocity window (100 ms), min-fling threshold (0.3 px/ms), decay, termination, reduced-motion, cancel, per-frame event cap.
- Files: `lib/terminal/__tests__/scrollKinematics.test.ts`

##### Task 1.1.3b: Implement `MomentumTracker`
- `addSample(t, y)`, `release(reducedMotion)`, `step(dt) -> dyPx`, `cancel()`; constants exported and marked "tune on device".
- Files: `lib/terminal/scrollKinematics.ts`

#### Story 1.1.4: Per-frame output cap (QoS item 4)
**As a** mobile user on a slow or flooded link, **I want** scroll output bounded per animation frame instead of throttled on a timer, **so that** the local buffer still tracks my finger 1:1 while a TUI is never flooded.
Source: `research/qos.md` #18 (a reference recipe throttles wheel events to about 8/s; that conflicts with 1:1 feel, so cap per frame) and #9 (rAF coalescing). Reconciled with the caps already in this plan:

| Target | Per-frame cap (this story, `PerFrameCap`) | Existing limit, kept unchanged |
|---|---|---|
| `xterm-local` | one `scrollLines` per rAF carrying all travel since the last frame (REQ-6), lines clamped to +-`rows` | none; **no fixed-interval throttle** on this path |
| `tui-wheel` | <= 3 reports per frame (Task 1.1.2c / `encodeWheel` cap) | none |
| `tui-pgkeys` | <= 1 page key per frame | 100 ms page rate-limit and 5-page per-fling cap (Story 1.1.2). Kept because the quantum is a whole page, so 1 per frame would be about 60 full TUI repaints per second; this is a per-quantum limit, not the rejected 80/120 ms wheel throttle |

**Acceptance Criteria**:
- Local clamp is per frame, symmetric, and does not carry the excess.
  - *Given* `rows` 30 and a single frame whose travel is 90 lines (e.g. after a long main-thread stall), *When* `clampLinesPerFrame(90, 30)` and `clampLinesPerFrame(-90, 30)`, *Then* `30` and `-30`; the discarded excess is not added back to the accumulator remainder (1:1 beyond one screen per frame has no visible meaning).
- No timer throttle on the 1:1 path.
  - *Given* 60 consecutive 16 ms frames each carrying at least one line of travel in `xterm-local`, *When* the hook runs them with fake rAF, *Then* `scrollLines` is called on all 60 frames (a fixed 80/120 ms throttle would give about 8 to 12).
- Stalled frames do not replay.
  - *Given* a 200 ms gap between two rAF callbacks with 40 `touchmove`s in it, *Then* exactly one dispatch occurs on the next frame (all travel since the last dispatch, clamped), not one per missed frame.
**Files**: `lib/terminal/scrollKinematics.ts` (`clampLinesPerFrame`, pure), `lib/hooks/useTerminalGestures.ts` (apply in the rAF callback and in momentum frames), `lib/terminal/__tests__/scrollKinematics.test.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.1.4a: Failing tests (red)
- Tests: `clampLinesPerFrame_should_ClampToRowsBothSigns_When_LargeDelta`; `scrollDrag_should_CallScrollLinesEveryFrame_When_60FramesAtOneLineEach_InLocalBuffer`; `scrollDrag_should_DispatchOnceAndClamp_When_200msFrameGapWith40Touchmoves`. The two hook tests depend on Task 1.2.1b1 and are written failing here, kept skipped (`it.todo`) until 1.2.1b1 lands, then enabled.
- Files: `lib/terminal/__tests__/scrollKinematics.test.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.1.4b: Implement `clampLinesPerFrame` (green)
- Pure function in `scrollKinematics.ts`. **Reuse `rafThrottlePoint` (`lib/terminal/touchDrag.ts:42`, already imported by the hook at L22 and used for the scroll path at ~L240) for per-frame coalescing of drag points; do not add a second coalescer.** `clampLinesPerFrame` is applied inside that callback; momentum frames run in the single rAF loop that consumes `MomentumTracker.step()`, which is not a second touch-point coalescer.
- Files: `lib/terminal/scrollKinematics.ts`

### Epic 1.2: Hook integration and hardening
**Goal**: The gesture hook delegates to the pure modules; browser gestures cannot steal the drag.

#### Story 1.2.4: Regression anchors (sequenced FIRST in Epic 1.2: characterization tests before any hook rewrite)
**As a** maintainer, **I want** existing touch behavior pinned, **so that** the rewrite does not break it.
**Acceptance Criteria**:
- Tap, long-press selection, double-tap word-select, multi-touch cancel unchanged; PgUp/PgDn toolbar bytes unchanged.
  - *Given* a touch with `totalDy 5` and elapsed 100 ms, *When* `touchend`, *Then* tap path runs (focus called once); *Given* a stationary touch for 400 ms, *Then* SELECTING state; *Given* two touches, *Then* gesture cancels; PgUp key sends `\x1b[5~`.
**Files**: `lib/hooks/__tests__/useTerminalGestures.test.ts`, `components/sessions/__tests__` (toolbar test if one exists; otherwise assert bytes in `scrollRouting.test.ts`)

##### Task 1.2.4a: Add/confirm regression tests
- Write and run these against the **current, unmodified** hook (green baseline) before Task 1.2.1a; re-run after each of Tasks 1.2.1b1-b3, 1.2.1d1-d2 and 1.2.10b to prove the rewrite did not change them. A test that fails on the unmodified hook is a finding to record (BUG entry), not to "fix" in this story.
- Files: `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.4b: Extract the gesture state machine into a pure module (6 h, budgeted; runs under the 1.2.4a anchors)
- **Why**: `useTerminalGestures.ts` (374 lines) will absorb about ten options (`override`, `gestureScrollEnabled`, `tuiScrollPolicy`, `connectionEpoch`, `isInputBusy`, `onPageKeysSent`, `onScrollGesture`, momentum, cell geometry, `onSendData`) plus COASTING/CANCELLED handling and the S9 table. Left inline it becomes the next hotspot. Extract the **S9 transition logic** (`GestureState`, events, `reduce(state, event) -> {state, effects}`) into `lib/terminal/gestureMachine.ts` as a pure function with no DOM or React; the hook keeps listener wiring, timers, rAF and effect execution only. Group the options into one typed `GestureOptions` object.
- Pure behavior-preserving refactor first (no new states): the 1.2.4a anchors must stay green before and after. New states (COASTING, CANCELLED) and the S9 additions are added to the module by Tasks 1.2.2b1 and 1.2.10, with table-driven tests of the transition table in `lib/terminal/__tests__/gestureMachine.test.ts`.
- Files: `lib/terminal/gestureMachine.ts`, `lib/terminal/__tests__/gestureMachine.test.ts`, `lib/hooks/useTerminalGestures.ts`

#### Story 1.2.1: Route drag through accumulator and target
**As a** mobile user, **I want** drag up and down to scroll the right layer without opening the keyboard, **so that** I can read output one-handed.
**Acceptance Criteria**:
- Both directions work in normal buffer.
  - *Given* a mocked `Terminal` with normal buffer and cellH 18, *When* touchmove drags +90 px down then -90 px up in single frames, *Then* `scrollLines` is called with -5 then +5 (finger down = older).
- TUI target forwards bytes, never `scrollLines`.
  - *Given* a table row (injected verified table, or override `'tui'`) routing to `'tui-pgkeys'`, policy `'pgkeys'`, cellH 18, 24 rows, *When* the finger travels 213 px down in total (15 px slop plus **198 px post-slop travel = 11 lines = one `TUI_PAGE_STEP_LINES`**), *Then* `onSendData` receives exactly one `\x1b[5~` and `scrollLines` is not called; *When* total travel is 115 px (100 px post-slop, 5.5 lines), *Then* nothing is sent. (The earlier AC "414 px (23 lines) drag gives exactly one PgUp" conflicted with the overshoot seeding and the half-page step and is withdrawn.)
- First move past slop prevents default.
  - *Given* a PENDING touch whose first move crosses `SLOP_PX` with `e.cancelable===true`, *When* the move is handled, *Then* `preventDefault` is called; with `cancelable===false` it is not called and a debug log entry is written.
- Mode re-evaluated per frame.
  - *Given* mode flips alternate -> normal mid-drag, *When* the next frame runs, *Then* the target switches to `'xterm-local'` and the accumulator is reset.
- Drag never focuses the terminal.
  - *Given* a completed scroll, *When* `touchend` fires, *Then* `terminal.focus` is not called.
- `touchend` after a scroll does not synthesize a click.
  - *Given* a SCROLLING gesture, *When* `touchend` fires with `e.cancelable===true`, *Then* `preventDefault` is called (suppresses the synthesized click/focus); with `cancelable===false` it is not called and a debug log entry is written.
- Keyboard toggle or rotation mid-drag cancels the gesture.
  - *Given* a SCROLLING drag with a non-zero accumulator remainder, *When* a `visualViewport` resize or `orientationchange` fires, *Then* state returns to IDLE, the line and page remainders are reset to 0, no further `scrollLines`/`onSendData` calls occur, and further `touchmove`s of that same touch are ignored until a new `touchstart` (UX AC17). The same applies when the S6 override changes mid-gesture or `connectionEpoch` changes (reconnect or full-snapshot write; also resets `netPagesUp`).
**Files**: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`
**AC groups (one per task, so no task carries all eight behaviors)**: (A) both directions + slop/overshoot seed + local dispatch -> Tasks 1.2.1a1, 1.2.1b1; (B) routing per frame + mode flip + TUI forwarding -> Tasks 1.2.1a2, 1.2.1b2-b3; (C) first-move `preventDefault`, drag never focuses, `touchend` click suppression + mid-gesture cancel -> Tasks 1.2.1a3, 1.2.1c, 1.2.1d1, 1.2.1d2.

##### Task 1.2.1a1: Failing hook tests, group A (both directions, seed, local dispatch)
- Extend `useTerminalGestures.test.ts` with mocked `terminal.buffer.active.type`, `modes`, `scrollLines`, `onSendData`; rAF faked. Named tests: `scrollDrag_should_CallScrollLinesMinus5ThenPlus5_When_Drag90PxDownThenUp_InNormalBuffer`; **`scrollDrag_should_ScrollBothDirections_InNormalAndAlternate`** (UX-1; both signs in the normal buffer and, for the alternate buffer, `\x1b[5~`/`\x1b[6~` in the right order); `scrollDrag_should_DispatchAtMostOncePerRaf_When_ManyTouchmovesInOneFrame`.
- Files: `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.1a2: Failing hook tests, group B (routing per frame, mode flip, TUI forwarding, direction)
- Files: `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.1a3: Failing hook tests, group C/D plus the single-`scrollLines`-caller pins
- `preventDefault` rules, no focus after scroll, `touchend` click suppression, mid-gesture cancel (viewport, orientation, override, `connectionEpoch`); `scrollLines_should_HaveSingleProductionCaller_InUseTerminalGestures` (source scan) and `viewportTouch_should_NotDoubleScroll_When_HookOwnsDrag`.
- Files: `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.1b1: Accumulator and local dispatch in the hook (AC group A)
- In the SCROLLING rAF callback replace `Math.round`/`scrollLines` with `ScrollAccumulator.push` and `clampLinesPerFrame`, dispatching via `scrollLines` for the local path only. Seed the accumulator with the overshoot (travel minus `SLOP_PX`; no initial jump); `SLOP_PX` is an exported constant (default 15; 10 only per the D9 fallback rule). Preserve ADR-012 (code-comment/`docs/tasks`) TouchEvent listeners. Keep the MobileDebugLog calls from 0.1.1b. Guard against xterm's built-in touch `Gesture` per the open-question fallback.
- Files: `lib/hooks/useTerminalGestures.ts` (about 1 file, under 150 changed lines)

##### Task 1.2.1b2: Routing wiring and `readScrollMode` (AC group B, routing half)
- `decideScrollTarget(readScrollMode(terminal), routingTable, tuiPolicy, override)` per frame; mode flip resets the accumulator; the override arrives as a plain `override` hook option (typed by Story 1.2.5a; the prop path from `TerminalOutput` is Task 1.2.5g, so this task is tested with an injected option and does not wait for the panel). Log `route-decision` entries (Task 0.1.1d). Type `readScrollMode`'s `terminal.modes` access once in `mouseTracking.ts` instead of a new `as any`. Log `unverified:true` while `ROUTING_VERIFIED=false`.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/terminal/scrollRouting.ts` (add `readScrollMode`), `lib/terminal/mouseTracking.ts`

##### Task 1.2.1b3: TUI dispatch (AC group B, TUI half)
- TUI targets dispatch via `onSendData`: pgkeys uses the page accumulator with the per-fling cap and rate limit; wheel (only when `TuiScrollPolicy==='wheel'`) uses touch-start `startCol`/`startRow` clamped to cols/rows. Direction mapping tested both signs: drag down -> `\x1b[5~` (PgUp, earlier output), drag up -> `\x1b[6~`.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/terminal/scrollRouting.ts`

##### Task 1.2.1c: `preventDefault` hardening and scrollbar/selection-handle ignore (tests first)
- Tests first: `touchmove_should_PreventDefault_When_FirstMovePastSlopAndCancelable`; `touchmove_should_NotPreventDefaultAndLog_When_NotCancelable`; **`touchstart_should_IgnoreGesture_When_TouchBeganOnScrollbarOrSelectionHandle`** (test fixture: a `touchstart` whose target is the scrollbar thumb/track element or a selection-handle element; assert state stays IDLE and no listener side effect).
- Call `preventDefault` from first move beyond slop in PENDING only when `e.cancelable`; ignore touches that did not start in the container; do not start a gesture when the touch begins on selection handles / scrollbar thumb / track (check their handlers `stopPropagation`).
- Files: `lib/hooks/useTerminalGestures.ts`, `components/sessions/XtermTerminal.tsx` (only if a handler needs `stopPropagation`)

##### Task 1.2.1d1: `touchend` suppresses the synthesized click (tests first, then implement)
- In `touchend`, when the gesture was SCROLLING (or consumed by COASTING) and `e.cancelable`, call `e.preventDefault()` so Chrome does not synthesize a click/focus; log when not cancelable. Tests: `touchend_should_PreventDefault_When_ScrollCompletedAndCancelable`; `touchend_should_NotPreventDefaultAndLog_When_NotCancelable`; `touchend_should_NotFocusTerminal_When_ScrollCompleted`.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.1d2: Mid-gesture cancel listeners (tests first, then implement)
- Add the hook-owned `visualViewport` resize / `orientationchange` / override-change / `connectionEpoch`-change listener that cancels PENDING/SCROLLING (not only COASTING): state -> CANCELLED -> IDLE at touchend, `ScrollAccumulator` and page accumulator `reset()`, mark the touch ignored until the next `touchstart`. Tests: `scrollDrag_should_ResetAccumulatorAndCancel_When_ViewportResizeMidGesture`; `scrollDrag_should_IgnoreRemainingMoves_When_OrientationChangeMidGesture`; `scrollDrag_should_CancelAndResetNetPagesUp_When_ConnectionEpochChanges`. Task 1.2.2b3a reuses these listeners for COASTING; it does not add a second set.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

#### Story 1.2.2: Momentum in the hook
**As a** mobile user, **I want** fling inertia that I can stop with a touch, **so that** scrolling feels native.
**Acceptance Criteria**:
- Fling continues then stops.
  - *Given* a drag released at 1.2 px/ms in normal buffer, *When* fake rAF advances 60 frames, *Then* `scrollLines` is called with decreasing magnitudes, at most one dispatch per frame, and none after velocity < 0.02 px/ms.
- Cancellation sources.
  - *Given* momentum active, *When* `touchstart`, `visualViewport` resize, `orientationchange`, buffer-type change, or unmount occurs, *Then* no further `scrollLines`/`onSendData` calls. The hook subscribes to `visualViewport` resize/`orientationchange` itself and cleans up in its own effect; the viewport-settle helper knows nothing about the hook.
- Touch-to-stop is not a tap (explicit COASTING state).
  - *Given* `GestureState` COASTING (momentum active after a SCROLLING `touchend`), *When* `touchstart` occurs, *Then* momentum is cancelled and `consumedByCoast` is set; *When* the matching `touchend` fires with no movement, *Then* the tap/focus path is skipped, `terminal.focus` is not called, the keyboard does not open, and state returns to IDLE. A `touchstart` that begins a new drag past slop clears the flag and scrolls normally.
  - *Given* momentum ended on its own, *Then* state is IDLE and the next tap focuses as before (flag not stale).
**Files**: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.2a: Failing momentum-in-hook tests
- Files: `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.2b1: COASTING state and `MomentumTracker` wiring
- Add `COASTING` to `GestureState`; feed samples on touchmove; start momentum on `touchend` from SCROLLING only (not `touchcancel`); read `ReducedMotionPref` at gesture start; dispatch decaying frames through the same per-frame cap path.
- Files: `lib/hooks/useTerminalGestures.ts`

##### Task 1.2.2b2: `consumedByCoast` flag and stale-flag tests
- Set the flag on a `touchstart` during COASTING, consume it at the matching `touchend`; clear on any new touchstart that begins a drag past slop. Tests: `touchcancel` during COASTING resets state and the flag (pre-mortem P3 #5); a non-coast-stop `touchstart` clears a stale flag; `touchstart` on the exact frame momentum ends.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.2b3a: Momentum cancel sources, including buffer-type change
- Reuse the Task 1.2.1d2 listeners for COASTING (`touchstart`, `visualViewport` resize, `orientationchange`, `connectionEpoch`, unmount) and add the one new source: **buffer-type change, subscribed with `terminal.buffer.onBufferChange`** (disposed in the hook's cleanup; no polling). Tests: `momentum_should_Cancel_When_BufferTypeChanges` (fire the mocked `onBufferChange`); `momentum_should_Cancel_When_VisualViewportResizeOrOrientationChangeOrUnmount`; `hook_should_DisposeBufferChangeSubscription_When_Unmounted`.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.2b3b: Local edge-stop and TUI page cap
- Stop at an edge when `viewportY` is at 0 or `baseY` for `'xterm-local'` only (a TUI owns its scrollback; no boundary is detectable); in TUI modes apply the per-fling page cap and rate limit from Story 1.1.2, then cancel. Tests: `momentum_should_StopAtEdge_When_ViewportYAtBoundary_InLocalBuffer`; `momentum_should_StopAtPageCap_When_TuiTarget`.
- Files: `lib/hooks/useTerminalGestures.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

#### Story 1.2.10: S9 state-table behaviors (design/ux.md S9, AC11, AC23)
**As a** mobile user, **I want** the edge cases of the gesture state table to behave as documented, **so that** a cancelled touch, a sideways drag or a drifting tap never scrolls, focuses wrongly or strands me.
**Acceptance Criteria**:
- `touchcancel` while SCROLLING resets and does not coast.
  - *Given* a SCROLLING gesture with velocity above the fling threshold, *When* `touchcancel` fires, *Then* state is IDLE, the accumulator and page remainder are 0, no momentum frames are scheduled, and the next `touchstart` begins a fresh PENDING.
- Horizontal-first drag is ignored.
  - *Given* a PENDING touch whose first move past `SLOP_PX` has |dx| > |dy|, *Then* state is CANCELLED, `scrollLines` and `onSendData` are never called, `preventDefault` is not called (Android back-swipe preserved), and the release is not a tap.
- Tap tolerance equals the slop (no dead zone).
  - *Given* a touch that drifts 12 px and releases at 150 ms, *Then* the tap path runs (`terminal.focus` called once); *Given* 14 px, *Then* also a tap; *Given* a release after crossing `SLOP_PX`, *Then* it is a scroll, never a tap. The tap threshold in the hook is `SLOP_PX` (previously 8 px; the 1.2.4a anchor with 5 px stays green).
- Tap while a selection is active clears it only.
  - *Given* an active selection, *When* a tap (under the slop, under 400 ms) ends, *Then* `terminal.clearSelection()` is called and `terminal.focus` is not; *When* the next tap ends, *Then* focus runs as usual.
**Files**: `lib/terminal/gestureMachine.ts`, `lib/hooks/useTerminalGestures.ts`, tests in `lib/terminal/__tests__/gestureMachine.test.ts` and `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.10a: Failing tests (red)
- `touchcancel_should_ResetStateAndNotCoast_When_ScrollingTouchCancelled`; `touchmove_should_NotScrollOrPreventDefault_When_HorizontalFirstPastSlop`; `touchend_should_Tap_When_MovedUnderSlopAndReleasedBefore400ms` (replaces the withdrawn dead-zone test `touchend_should_DoNothing_When_MovedBetween8PxAndSlop`); `touchend_should_ClearSelectionAndNotFocus_When_TapWhileSelectionActive`; table-driven transition tests in `gestureMachine.test.ts` for each S9 row added here.
- Files: the two test files above

##### Task 1.2.10b: Implement in the state machine and the hook (green)
- Add the CANCELLED transitions for `touchcancel` and horizontal-first, set the tap threshold to `SLOP_PX`, and the selection-clear branch. Re-run the 1.2.4a anchors (the one intended anchor change is the widened tap tolerance, recorded in the PR).
- Files: `lib/terminal/gestureMachine.ts`, `lib/hooks/useTerminalGestures.ts`

#### Story 1.2.3: Overscroll / pull-to-refresh hardening
**As a** mobile user, **I want** downward drags not to trigger page refresh, **so that** drag-down is reliable.
**Acceptance Criteria**:
- No page-level overscroll.
  - *Given* the app on Android Chrome at page scrollTop 0, *When* dragging down inside the terminal, *Then* the page does not reload or bounce (device checklist item D4; static assertion that computed `overscroll-behavior` on `html` is `none`).
**Files**: `app/globals.css` (add `html, body { overscroll-behavior: none }`), `components/sessions/XtermTerminal.css.ts` (add `overscrollBehavior: "contain"` next to existing `touchAction: "none"` at L60)

##### Task 1.2.3a: Add overscroll CSS
- Note: `touch-action:none` already present (`XtermTerminal.css.ts:60`); do not re-add. Overlays using `manipulation` (L31, L131) are left as is unless the spike shows a stolen gesture there.
- Files: `app/globals.css`, `components/sessions/XtermTerminal.css.ts`

#### Story 1.2.5: Scrolling settings: override, Gesture scrolling, mode chip, hints (S6)
**As a** mobile user, **I want** to force local or TUI scrolling and to turn drag capture off for a screen reader, **so that** a misrouted session is fixable and TalkBack still works.
**Mount point (decided; verified by reading `TerminalOutput.tsx`)**: there is no existing terminal settings menu (the only menus are `TerminalContextMenu`/`TerminalLinkMenu`, unrelated). The host is the toolbar in `TerminalOutput.tsx`: the `secondaryActions` array (L1544, rendered inline on desktop at L1698 and in the mobile "More" overflow row at L1878-1890) gets a "Scrolling" entry whose handler toggles an inline `ScrollingPanel` rendered beneath the toolbar in the same way as `mobileOverflowRow`; the **mode chip** (a button that opens the compact picker, which links to the full panel) is placed in the always-visible `styles.actions` row next to the Resize/Redraw button, outside the `toolbarExpanded` conditional, so it is reachable with the toolbar collapsed and the keyboard closed.
**Acceptance Criteria**:
- Control and persistence per design/ux.md S6 and AC21.
  - *Given* the "How dragging scrolls" fieldset of native radios (Auto (recommended), Terminal history, Page keys; each with its one-line description; per device, not per session), *When* the user selects "Page keys", *Then* `localStorage['terminal-scroll-override']==='tui'`, the next drag frame routes to `tui-pgkeys`, and the polite live region announces "Scroll mode: Page keys"; *When* the page reloads, *Then* the selection is restored; *When* "Auto" is selected, *Then* the key is removed and `ScrollRoutingPolicy` decides.
- Active mode visible: the Auto row shows "now: Page keys|Terminal history" and the chip shows the effective mode (`useEffectiveScrollMode`, same `decideScrollTarget` result). *Given* a normal-buffer mode and Auto, *Then* the chip reads "Terminal history"; *Given* alternate and Auto, *Then* "Page keys"; *Given* Gesture scrolling Off, *Then* the route stays visible with a suffix ("Terminal history, gestures off"). The chip's accessible name states the route, the gesture state and the action (`aria-label="Scroll mode: Terminal history. Gesture scrolling on. Opens scroll settings"`); radios carry their descriptions via `aria-describedby`.
- Chip opens a **compact picker** (three route radios only) that **closes on select** and returns focus to the chip, with a "More scrolling settings" button opening the full panel; the "Scrolling" toolbar entry opens the full panel. *Given* the picker is open and the user selects Page keys, *Then* the override is stored, the picker closes, and exactly one announcement is made (the chip does not announce while the panel or picker is open).
- Panel never starves the terminal: *Given* opening the panel inline would leave fewer than 5 terminal rows (keyboard open), *Then* it renders as a scrollable overlay (no terminal resize, no refit); reduced motion removes open/close animation.
- Misroute cue: *Given* a drag past the slop on the `xterm-local` route with at least one line of post-slop travel and `viewportY` unchanged for the whole gesture, *Then* the chip is highlighted (heavier border and a leading "!") for 5 s with one polite announcement, at most once per 30 s.
- Chip visibility rule: *Given* `matchMedia('(any-pointer: coarse)')` false and no `touchstart` seen, *Then* the chip is not rendered; *Given* a `touchstart` on the terminal, *Then* it renders. Hidden when fewer than `MIN_ROWS_FOR_OVERLAYS` (5) rows are visible.
- Gesture scrolling: *Given* the switch set Off, *Then* `localStorage['terminal-gesture-scroll']==='off'`, the hook registers no touch listeners on the surface and never calls `preventDefault`, the surface has the default `touch-action`, and toolbar PgUp/PgDn still scroll (Story 1.2.9); *Given* On (default), *Then* behavior is as designed. The panel carries "Turn off if you use a screen reader (TalkBack)" and the wide-output note.
- Storage failure: *Given* `localStorage.getItem`/`setItem` throws (private mode, quota), *Then* both settings fall back to defaults (`auto`, On) with no exception and the in-memory choice still applies for the session.
- Targets and focus: every control, the chip and the hint's dismiss button at least 44x44 CSS px (24x24 floor), visible focus; closing the panel or picker returns focus to its opener. Verified at 200% font size (device D8): toolbar, chip, picker and panel remain usable.
- First-use hint: *Given* `terminal-scroll-hint-seen` unset and the first drag-scroll, *Then* a short `role="status"` hint for the effective route (one sentence plus "Tap the chip to change") is shown and **stays until the user taps its "Got it" button or opens the picker/panel (no auto-timeout)**; the key is set **on dismissal**, not on display; *Given* it is set, *Then* no hint. The full tmux exit text (PgDn to the bottom) is permanent help in the full panel, not in the hint; no Esc or `q` button exists anywhere.
**Files**: `lib/terminal/scrollOverride.ts` (read/write/validate both persisted values; invalid -> defaults; try/catch around storage), `lib/terminal/__tests__/scrollOverride.test.ts`, `components/sessions/ScrollingPanel.tsx` (+ test; contains the radios, switch, picker variant and `ScrollModeChip`), `lib/hooks/useEffectiveScrollMode.ts` (+ test), `components/sessions/TerminalOutput.tsx` (secondaryActions entry, chip, panel mount, epoch and scroll-mode state), `components/sessions/XtermTerminal.tsx` (props and `onScrollModeChange`), `lib/hooks/useTerminalGestures.ts` (`gestureScrollEnabled` option), `components/sessions/XtermTerminal.css.ts` (touch-action keyed on a data attribute)

##### Task 1.2.5a: Persistence module + test (pure; scheduled in week 2 with the pure modules, a prerequisite of Tasks 1.2.1b2 (type and storage only) and 1.2.5c; also logs `override-change` via Task 0.1.1d)
- Pure get/set with a `localStorage` stub for `terminal-scroll-override` and `terminal-gesture-scroll`; a subscription so a change takes effect on the next frame. Tests: persist and restore, invalid stored value -> default, key removed on `auto`, **storage throwing on get and on set -> defaults, no exception** (`scrollOverride_should_FallBackToDefaults_When_LocalStorageThrows`).
- Files: `lib/terminal/scrollOverride.ts`, `lib/terminal/__tests__/scrollOverride.test.ts`

##### Task 1.2.5b1: `ScrollingPanel` with `picker` and `full` variants (tests first)
- One component, `variant: 'picker' | 'full'`: native radios in a fieldset with `aria-describedby` descriptions; the picker closes on select, returns focus to the chip, and shows "More scrolling settings"; the full variant adds the Gesture scrolling switch, the permanent tmux and wide-output help, and the polite live region; overlay rendering when inline would leave fewer than 5 rows (`panel_should_RenderAsOverlay_When_InlineWouldLeaveFewerThanFiveRows`); no animation under reduced motion. Tests: `panel_should_UseNativeRadiosInLabelledFieldsetWithDescriptions_And_AnnounceChange`; `toggle_should_ChangeSelectionOnArrowKeys`; `picker_should_CloseAndReturnFocusToChip_When_RouteSelected`; `picker_should_OpenFullPanel_When_MoreSettingsPressed`; `chip_should_NotAnnounce_When_PanelOrPickerOpen`; `toggle_should_ShowEffectiveModeUnderAuto`; `toggle_should_RouteNextDragToPgKeys_When_TuiSelected` (hook integration).
- Files: `components/sessions/ScrollingPanel.tsx`, `components/sessions/__tests__/ScrollingPanel.test.tsx`

##### Task 1.2.5b2: `ScrollModeChip` and the misroute cue (tests first)
- Chip text per ux.md S6 (route always visible; ", gestures off" suffix), the accessible name that mentions route, gesture state and action, the touch-detection rule, the 44x44 target, hidden under 5 rows. Misroute cue: highlight (border plus a leading "!", not colour alone) for 5 s with one polite announcement, at most once per 30 s, driven by an `onScrollGesture({route, postSlopLines, viewportYChanged})` callback from the hook. Tests: `chip_should_BeHidden_When_NoCoarsePointerAndNoTouchSeen`; `chip_should_Render_When_TouchstartSeen`; `chip_should_OpenPicker_When_Tapped`; `chip_should_HaveAtLeast44pxTarget`; `chip_should_BeHidden_When_FewerThanFiveRows`; `chip_should_StillShowRoute_When_GesturesOff`; `chip_should_NameRouteAndGestureState_InAccessibleName`; `chip_should_HighlightOnce_When_LocalDragMovedNothing`; `chip_should_NotHighlight_When_TuiRoute`.
- Files: `components/sessions/ScrollingPanel.tsx` (exports `ScrollModeChip`), its test, `lib/hooks/useTerminalGestures.ts` (the `onScrollGesture` callback, one site)

##### Task 1.2.5b3: Mount the panel, picker and chip in `TerminalOutput.tsx`
- Add the `secondaryActions` "Scrolling" entry (full panel) and the chip in `styles.actions` (opens the picker). Order per the shared-file edit table (after 1.2.5g). Component test in `TerminalOutput.refit.test.tsx` or a sibling: chip rendered with the toolbar collapsed; the entry opens the full variant.
- Files: `components/sessions/TerminalOutput.tsx`

##### Task 1.2.5c: Gesture scrolling Off in the hook and CSS
- `gestureScrollEnabled` hook option: when false, register no touch listeners on the surface (and remove them live when toggled); `XtermTerminal.css.ts` applies `touchAction: none` only under `[data-gesture-scroll="on"]` and the default otherwise. Tests: `hook_should_RegisterNoTouchListeners_When_GestureScrollOff`; `hook_should_NeverPreventDefault_When_GestureScrollOff`; live toggle re-registers; `terminalCss_should_RestoreDefaultTouchAction_When_GestureScrollOff` (CSS restores the default `touch-action` when `data-gesture-scroll` is not `on`).
- Files: `lib/hooks/useTerminalGestures.ts`, `components/sessions/XtermTerminal.css.ts`, `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.5d: First-use hint (dismissible, persists until dismissed)
- Small `useScrollHint` hook plus a `role="status"` element in the panel host; fires on the first SCROLLING transition; one short sentence per route plus "Tap the chip to change" and a 44x44 "Got it" button; no timeout; `terminal-scroll-hint-seen` is written on dismissal or when the picker/panel opens. Tests: `hint_should_ShowOnceOnFirstDragWithRouteText_And_NotOfferEscOrQ`; `hint_should_StayUntilDismissed_And_NotAutoTimeout`; `hint_should_SetSeenFlagOnDismissOnly`; `hint_should_NotShow_When_SeenFlagSet`.
- Files: `components/sessions/ScrollingPanel.tsx` (or a sibling `ScrollHint.tsx`), `lib/hooks/useTerminalGestures.ts` (one callback), tests

##### Task 1.2.5e: `useEffectiveScrollMode` and the upward scroll-mode data flow
- The terminal modes live on the xterm `Terminal` inside `XtermTerminal`; the chip, toolbar keys and jump button live in `TerminalOutput`. `XtermTerminal` gets an `onScrollModeChange(mode: ScrollMode)` prop, called only when `readScrollMode(terminal)` differs from the last reported value, re-read on `terminal.buffer.onBufferChange` and `terminal.onWriteParsed` (already used at `XtermTerminal.tsx:~796`; if `mouseTrackingMode` changes arrive only through writes, the `onWriteParsed` re-read covers them, cost one comparison per chunk). `TerminalOutput` keeps the value in state (set only on change) and `useEffectiveScrollMode(scrollMode, override, gestureOn)` returns `EffectiveScroll`. Tests: `useEffectiveScrollMode_should_ReturnOverrideTarget_When_OverrideLocalOrTui`; `useEffectiveScrollMode_should_FollowTable_When_Auto`; `XtermTerminal_should_ReportScrollModeOnlyOnChange`; `useEffectiveScrollMode_should_KeepRoute_When_GesturesOff`.
- Files: `lib/hooks/useEffectiveScrollMode.ts`, `lib/hooks/__tests__/useEffectiveScrollMode.test.ts`, `components/sessions/XtermTerminal.tsx`, `components/sessions/TerminalOutput.tsx`

##### Task 1.2.5f: `connectionEpoch` (where incremented and exposed)
- `TerminalOutput` owns `const [connectionEpoch, setConnectionEpoch] = useState(0)` and increments it in the reconnect handlers (flag off: `TerminalOutput.tsx:~969/~1067`; flag on: via the `useTerminalStream.ts` reconnect callback) and in the `onFullSnapshot` callback. Exposed as the numeric prop `connectionEpoch` (threaded by Task 1.2.5g). Tests: `connectionEpoch_should_Increment_When_ReconnectOrFullSnapshot`; the consumer tests are `scrollDrag_should_CancelAndResetNetPagesUp_When_ConnectionEpochChanges` (1.2.1d2) and the `JumpToLatestButton` reconnect test (1.2.7a1).
- Files: `components/sessions/TerminalOutput.tsx`, `components/sessions/__tests__/TerminalOutput.refit.test.tsx` (or sibling)

##### Task 1.2.5g: Thread options `TerminalOutput` -> `XtermTerminal` -> `useTerminalGestures`
- The hook is called inside `XtermTerminal`, so every setting must reach it as a prop. Add typed props and pass them into the hook's `GestureOptions` object: `scrollOverride`, `gestureScrollEnabled` (also sets `data-gesture-scroll` on the surface), `tuiScrollPolicy`, `connectionEpoch`, `onPageKeysSent`, `onScrollGesture`, and later (Tier B, 1.2.6c) `isInputBusy`. One typed `ScrollGestureProps` bundle avoids ten loose props. Tests: `XtermTerminal_should_PassOverrideAndGestureFlagToHook`; `XtermTerminal_should_SetDataGestureScrollAttribute`. Changing a prop takes effect on the next frame without remounting.
- Files: `components/sessions/XtermTerminal.tsx`, `components/sessions/TerminalOutput.tsx`, `lib/hooks/useTerminalGestures.ts` (options type only), tests

#### Story 1.2.7: Jump to latest and output-while-scrolled (design/ux.md S7)
**As a** mobile user scrolled up, **I want** a visible way back to live output and no yanking when new output arrives, **so that** I can read history and return without hunting for keys.
**Acceptance Criteria**:
- Button visible iff away from live output and enough rows are visible.
  - *Given* a local buffer with `viewportY < baseY` and at least 5 visible rows, *Then* the button renders (44x44 minimum, real `<button>`, accessible name, fixed-width label "Jump to latest"); *Given* `viewportY === baseY` or fewer than 5 rows, *Then* it does not.
  - *Given* a TUI target, *Then* it renders (label "Page down to latest") only while the estimate is valid and `netPagesUp > 0`. The estimate is invalidated by any user keystroke or paste, resize, mode/override change, `connectionEpoch` change, or `netPagesUp > 5`; while invalid the button is hidden, not clamped.
- Placement: *Given* the cursor row's rectangle intersects the button's, *Then* the button renders top-right instead; it never overlaps the cursor line or last line. The label is fixed per route and never changes while visible ("Jump to latest" local, "Page down to latest" TUI; the two are the only variants). To avoid a moving control, placement is evaluated only when the button becomes visible and when a gesture or fling ends, never mid-drag, and the corner is kept until the button hides.
- Tap scrolls without focusing.
  - *Given* the button, *When* tapped in the local buffer, *Then* `scrollToBottom()` runs, `terminal.focus` is not called, the keyboard does not open; in TUI mode it sends `min(netPagesUp, 5)` PgDn at the 100 ms rate limit and resets the estimate. Toolbar PgUp/PgDn presses count toward `netPagesUp` (Story 1.2.9).
- Output while scrolled holds position.
  - *Given* the user is scrolled up in the local buffer, *When* output arrives, *Then* `viewportY` is unchanged, the label does not change, a dot and visually hidden "new output" text appear, and one polite "New output below" announcement is made per scrolled-away episode (debounced 2 s, re-armed only after returning to bottom); *Given* a finger is down or momentum is running, *Then* no programmatic scroll occurs.
- Reduced motion: *Given* `prefers-reduced-motion: reduce`, *Then* the button has no transition or animation.
- Reconnect and empty buffer: *Given* `connectionEpoch` changes while scrolled, *Then* `netPagesUp` resets to 0 and the button is recomputed from the buffer; *Given* an empty buffer, *Then* the button is hidden and a drag is a no-op.
**Files**: `components/sessions/JumpToLatestButton.tsx` (+ test), `lib/terminal/scrollPosition.ts` (pure `isAwayFromLive`, `netPagesUp` tracker with validity rules) (+ test), `lib/hooks/useTerminalGestures.ts` (report page keys sent; one callback), mount in `TerminalOutput.tsx`
##### Task 1.2.7a1: Failing tests (red): `scrollPosition.test.ts` and `JumpToLatestButton.test.tsx`
- `lib/terminal/__tests__/scrollPosition.test.ts`: `isAwayFromLive_should_BeTrueOnlyWhenViewportAboveBase`; `netPagesUp_should_FloorAtZero_When_MorePgDnThanPgUp`; `netPagesUp_should_BeInvalid_When_KeystrokeResizeModeChangeReconnectOrOverFive`.
- `components/sessions/__tests__/JumpToLatestButton.test.tsx`: `jumpToLatest_should_BeHiddenInTui_When_EstimateInvalid`; `jumpToLatest_should_SendAtMostFivePgDn_When_Tapped`; `jumpToLatest_should_NotFocusTerminal_When_Tapped`; `jumpToLatest_should_HoldPosition_When_OutputArrivesWhileScrolledUp`; `jumpToLatest_should_MoveTopRight_When_CursorRowIntersects` plus `jumpToLatest_should_KeepCorner_When_GestureInProgress` (relocation only on show and gesture end); `jumpToLatest_should_AnnounceOncePerEpisode`; `jumpToLatest_should_HaveNoTransition_When_ReducedMotion`; `jumpToLatest_should_BeHidden_When_FewerThanFiveRows`; `jumpToLatest_should_ResetEstimate_When_ConnectionEpochChanges`; `jumpToLatest_should_BeHiddenAndDragNoop_When_BufferEmpty`.
- Files: the two test files

##### Task 1.2.7a2: Implement the pure module and the component (green)
- `lib/terminal/scrollPosition.ts` (`isAwayFromLive`, `netPagesUp` tracker with validity rules) then `components/sessions/JumpToLatestButton.tsx`; label fixed per route ("Jump to latest" / "Page down to latest"), corner evaluated only when the button becomes visible or a gesture ends.
- Files: `lib/terminal/scrollPosition.ts`, `components/sessions/JumpToLatestButton.tsx`

##### Task 1.2.7b: Hook callback and mount
- Files: `lib/hooks/useTerminalGestures.ts`, `components/sessions/TerminalOutput.tsx` (after 1.2.9 in the TerminalOutput.tsx edit order)

#### Story 1.2.8: Redraw button and device checks for pinch-zoom/TalkBack (design/ux.md S8, AC24-25)
**Decision**: no new menu item. The existing always-visible "Resize" button (`TerminalOutput.tsx:~1684-1692`, outside the `toolbarExpanded` conditional; calls `handleManualResize`, L1508) becomes the Redraw action, so it is discoverable without any settings menu. Needs only Story 2.1.3b (`handleManualResize` routed through `refit()`); it does not depend on Story 1.2.5.
**Acceptance Criteria**:
- *Given* the toolbar, *Then* the always-visible button reads "Redraw" with `aria-label="Redraw terminal (fixes a blank screen)"` and a matching title; *When* tapped, *Then* `xtermRef.current.refit({reason:'manual-resize'})` is called once and `refresh(0, rows-1)` runs even at unchanged dims, and the existing server resize message after the fit still goes out.
- *Given* a 360 px wide phone viewport, *Then* the button is visible and at least 44x44 CSS px (24x24 floor) with the toolbar collapsed (device D10).
- Device checks D8 (pinch-zoom via font-size reaching 200%; TalkBack with Gesture scrolling Off and On) and D10 (Redraw recovers a forced blank) are in Story 3.1.3.
**Files**: `components/sessions/TerminalOutput.tsx` (button label, aria, `handleManualResize` call), its test
##### Task 1.2.8a: Failing tests (red)
- `redrawButton_should_CallRefitWithManualResize_When_Tapped`; `redrawButton_should_BeRenderedWhenToolbarCollapsed`.
- Files: `components/sessions/__tests__/TerminalOutput.refit.test.tsx`

##### Task 1.2.8b: Relabel to "Redraw" and route through `refit()` (green)
- Label, `aria-label`, title; `handleManualResize` calls `refit({reason:'manual-resize'})`. Release-note it: the button is relabelled for all users including desktop (same action).
- Files: `components/sessions/TerminalOutput.tsx`

#### Story 1.2.9: Route-aware toolbar PgUp/PgDn (WCAG 2.5.1 equivalence; design/ux.md S4, AC27)
**As a** user who cannot or does not want to drag, **I want** the toolbar PgUp/PgDn to scroll the same history the drag scrolls, **so that** the single-pointer alternative is equivalent in every session type.
**Acceptance Criteria**:
- *Given* the effective route `xterm-local` (from `useEffectiveScrollMode`, override included), no modifier armed, and xterm history that can move, *When* PgUp is tapped, *Then* `terminal.scrollPages(-1)` is called and no bytes are sent; PgDn calls `scrollPages(1)`.
- *Given* a TUI route, *Then* `\x1b[5~` / `\x1b[6~` are sent exactly as today (byte-parity test, `TerminalOutput.tsx:1984/2018`), and the press counts toward `netPagesUp`.
- *Given* any route with Ctrl, Alt or Shift armed, *Then* the modified bytes from the existing maps (L15-41) are sent and `scrollPages` is not called.
- *Given* `auto` with a local route and xterm already at that edge (nothing to move), *Then* the key falls through and sends the bytes (preserves today's behavior for a normal-buffer TUI misclassified as local); *Given* an explicit `local` override, *Then* no fall-through.
- Residual risk recorded: in `auto`, a normal-buffer TUI with xterm scrollback is treated as local and toolbar PgUp scrolls xterm history rather than the app until the Q2 rows are corrected or the user selects Page keys (UNVERIFIED; D5 records it).
**Files**: `components/sessions/TerminalOutput.tsx` (the two key handlers at L1984 and L2018; route read from `useEffectiveScrollMode`), `lib/terminal/scrollRouting.ts` (a pure `toolbarPageAction(route, modifiers, canScroll, override)`), tests in `lib/terminal/__tests__/scrollRouting.test.ts` and `components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx` (share fixtures from `terminalOutputTestMocks.ts`)
##### Task 1.2.9a: Failing tests, then pure `toolbarPageAction`
- `toolbarPageAction_should_ReturnScrollPages_When_LocalRouteAndNoModifier`; `toolbarPageAction_should_ReturnBytes_When_TuiRoute`; `toolbarPageAction_should_ReturnModifiedBytes_When_ModifierArmed`; `toolbarPageAction_should_FallThroughToBytes_When_AutoLocalAtEdge`; `toolbarPageAction_should_NotFallThrough_When_ExplicitLocalAtEdge`.
- Files: `lib/terminal/scrollRouting.ts`, `lib/terminal/__tests__/scrollRouting.test.ts`
##### Task 1.2.9b: Wire the toolbar keys
- Replace the two handlers with `toolbarPageAction`; feed `netPagesUp`; component test `toolbarKeys_should_ScrollPages_When_LocalRoute` and `toolbarKeys_should_SendExactBytes_When_TuiRoute` (the latter is also the regression anchor).
- Files: `components/sessions/TerminalOutput.tsx`, `components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx`

#### Story 1.2.6: Scroll bytes never interleave into an in-flight paste (QoS item 3)
**As a** user who pastes a long block, **I want** a drag or fling not to inject page keys into the middle of the paste, **so that** pasted text is not corrupted.
**Audit result (VERIFIED 2026-10-01)**: all outgoing messages (input, resize, flow control, scrollback requests) share one FIFO `MessageQueue` (`lib/terminal/MessageQueue.ts:28-37`) with no priority lane; this is acceptable for latency because a paste over 512 B is split into chunks, one `setTimeout(10 ms)` each (`useTerminalFlowControl.ts:234-290`), so a scroll key waits behind at most one pushed chunk. The gap is ordering, not delay: a `\x1b[5~` pushed between two chunks lands inside the pasted text. Local-buffer scrolling (`scrollLines`) never touches this path.
**Acceptance Criteria**:
- Chunked paste exposes an in-flight signal.
  - *Given* `sendInput` with a 2000-byte string (4 chunks), *When* the first chunk is sent, *Then* `isInputChunking()` is true; *When* the last chunk is sent, or the session changes, or the connection drops (the early `return`s at L266-268 and the `catch` at L281-284), *Then* it is false. A <= 512 B input never sets it.
- TUI scroll steps are dropped, not queued, while a paste is in flight.
  - *Given* `isInputChunking()` true and a `tui-pgkeys` step due, *When* the frame runs, *Then* `onSendData` is not called, the step does not count toward the per-fling page cap, and a debug log line is written; *When* the paste finishes, *Then* the next due step sends normally.
  - *Given* `xterm-local`, *Then* scrolling is unaffected by the signal.
- Toolbar PgUp/PgDn (`sendKey`) are explicit user actions and are not gated.
**Files**: `lib/hooks/useTerminalFlowControl.ts` (flag set/cleared in `sendInput`), `lib/hooks/useTerminalStream.ts` (expose at the return near L731), `components/sessions/TerminalOutput.tsx` and `components/sessions/XtermTerminal.tsx` (thread an `isInputBusy` prop), `lib/hooks/useTerminalGestures.ts` (option `isInputBusy`), tests in `lib/hooks/__tests__/useTerminalFlowControl.test.ts` and `lib/hooks/__tests__/useTerminalGestures.test.ts`

##### Task 1.2.6a: Failing tests
- `sendInput_should_SetChunkingFlagUntilLastChunk_When_2000BytePaste`; `sendInput_should_ClearChunkingFlag_When_SessionChangesMidPaste`; `sendInput_should_NotSetChunkingFlag_When_512BytesOrFewer`; `scrollDrag_should_DropTuiPageKeyAndLog_When_InputChunking`; `scrollDrag_should_StillScrollLocally_When_InputChunking`.
- Files: the two test files above

##### Task 1.2.6b: Flag in flow control and stream hook (2 source files)
- Use a ref (not state) so the gate reads the live value inside the rAF callback; expose `isInputChunking()` from the stream hook.
- Files: `lib/hooks/useTerminalFlowControl.ts`, `lib/hooks/useTerminalStream.ts`

##### Task 1.2.6c: Prop threading and gesture gate (3 source files; sequence after the Phase 2 chain merges, shares files with it)
- Thread `isInputBusy` through `TerminalOutput` -> `XtermTerminal` -> hook option; gate only TUI page keys.
- Files: `components/sessions/TerminalOutput.tsx`, `components/sessions/XtermTerminal.tsx`, `lib/hooks/useTerminalGestures.ts`

---

## Phase 2: Keyboard-open redraw

### Epic 2.1: Single-owner fit with post-fit repaint
**Goal**: One *runtime* pipeline (the XtermTerminal sampler, reached via `refit()`) owns resize-driven fits; every fit it performs, and every `refit()` request even at unchanged dimensions, repaints; zero-size never leaves a stale canvas. Not literally the only `fit()` call: initial-mount and font-change fits remain, with dispositions in Story 2.1.3.

#### Story 2.1.1: Repaint seam and zero-size guard
**As a** mobile user, **I want** the terminal repainted after every fit, **so that** it is never black.
**Acceptance Criteria**:
- Repaint step (renderer-aware).
  - *Given* a mocked `Terminal` (rows 30) with `renderer==='webgl'`, *When* `postFitRepaint` runs, *Then* `refresh(0, 29)` and `clearTextureAtlas()` are each called once.
  - *Given* `renderer==='canvas'` (the likely state on the target Android phone) or `'dom'`, *Then* only `refresh(0, 29)` is called and `clearTextureAtlas` is never called; the canvas path gets the same `refresh` on every `refit()`.
- Zero-size guard.
  - *Given* container `clientHeight 0`, *When* `canFit(el)`, *Then* false and no `fit()` is called; *Given* 320x640, *Then* true.
**Files**: `lib/terminal/postFitRepaint.ts`, `lib/terminal/__tests__/postFitRepaint.test.ts`

##### Task 2.1.1a: Failing tests for `postFitRepaint` and `canFit`
- Files: `lib/terminal/__tests__/postFitRepaint.test.ts`

##### Task 2.1.1b: Implement `postFitRepaint`, `canFit`
- Logs via MobileDebugLog when a repaint is forced.
- `postFitRepaint(terminal, renderer, reason)` takes the renderer and reason (logged per design/ux.md S5).
- Files: `lib/terminal/postFitRepaint.ts`

#### Story 2.1.2: Wire seam into XtermTerminal (`requestFitRef` + repaint on every branch)
**As a** mobile user, **I want** `refit()` to reach the sampler and always repaint, **so that** the canvas is never stale after a keyboard resize, even when rows/cols did not change.
**Not gated on the spike**: this story ships unconditionally (Routing Verification Statement). It is built for either Q3 outcome (no renderer or container-size assumption; `refit()` does not depend on the ResizeObserver). The Q3 verdicts only steer which regression test is the primary proof (Story 0.1.3 branch table). Never claim the 50-cycle metric without a device run (the PR carries the "D7 unproven" note).
**Root cause being fixed** (VERIFIED by reading `XtermTerminal.tsx`): `sampleTick`/`startSamplerIfNeeded` are local to the mount `useEffect` (L1038-1101) while `useImperativeHandle` (L1250) is outside it, so `refit()` cannot reach them; and `postFitRepaint` would only fire in the schedule branch, whose `fit()` is at L1052. When proposed dims equal applied dims the sampler takes the at-rest branch (L1074-1078) and stops with no fit and no repaint, which is exactly the "keyboard open/close, rows/cols unchanged, canvas stale" case. The give-up branch (L1083-1090) likewise stops silently.
**Design**:
- **Non-RO trigger**: `refit()` must not depend on the ResizeObserver (the container may not resize when only `visualViewport` shrinks). Before starting the sampler it reads the container's `offsetHeight` (forces a reflow) and logs `visualViewport.height` and `visualViewport.offsetTop` vs container height and `getBoundingClientRect().top`; a mismatch is logged as a warning (it is the Q3 "container does not track the viewport" signal).
- Inside the mount effect, assign `requestFitRef.current = ({forceRepaint, reason}) => { if (forceRepaint) repaintRequested = true; startSamplerIfNeeded(); }`. If the sampler is already active, the call only sets `repaintRequested` (it is not dropped). `refit()` calls `requestFitRef.current({forceRepaint:true})` and starts the sampler **directly**, bypassing the 150 ms RO debounce at L1126 (the debounce exists to protect tmux/SIGWINCH from RO bursts; a settled `refit()` is a single deliberate request, and the sampler's own 50 ms tick still guards against a stale first read).
- `repaintRequested` is consumed (reset) in `stopSampler`'s callers: schedule branch -> `fit()` then `postFitRepaint` (always, as before); at-rest branch -> `postFitRepaint` iff `repaintRequested`; give-up branch (`sampleCount >= MAX_SAMPLES`) -> `postFitRepaint` iff `repaintRequested` plus a warn log.
- Timing target (same wording as requirements.md and design/ux.md): content visible within one sampler tick (50 ms) + one animation frame after `ViewportSettled` fires; the at-rest repaint lands on the first `sampleTick` (called synchronously from `startSamplerIfNeeded`, L1100), well inside that bound. The settle wait itself (3 stable frames, 600 ms max) is measured in the Phase 0 log and tuned against today's 400 ms path.
- Loop guard (pre-mortem P2 #3): `refit()` ignores a request when dims are unchanged and a repaint already ran this frame. Test: N rapid `visualViewport` resize events yield exactly one `refit()` per settle.
- Zero-size retry exhaustion: `refit()` while `!canFit(container)` retries via rAF for up to 20 attempts, **bounded also by a 1000 ms timeout** (whichever first). On exhaustion: log, call `postFitRepaint` (forced `refresh` so a canvas that already has valid dims is not left stale), and set `pendingRefit=true`, which the ResizeObserver callback consumes on its next non-zero delivery (calls `requestFitRef` again). Never silent.
**Acceptance Criteria**:
- Confirmed fit repaints.
  - *Given* the sampler reaches its schedule branch, *When* `fit()` returns, *Then* `postFitRepaint` is invoked once.
- `refit()` with unchanged dims still repaints (the architecture blocker).
  - *Given* a mocked terminal with proposed dims == applied dims (80x24), *When* `ref.refit()` is called, *Then* `fit()` is not called, `refresh(0, 23)` is called exactly once within the first sampler tick, and the sampler stops.
  - *Given* the sampler is already active when `refit()` is called, *When* it reaches at-rest, *Then* the repaint still happens (request not dropped).
  - *Given* a non-`refit()` ResizeObserver-driven sampler run reaching at-rest, *Then* no extra repaint (no regression in ADR-002 behavior).
- `refit()` bypasses the RO debounce.
  - *Given* fake timers, *When* `refit()` is called, *Then* the sampler's first tick runs with 0 ms elapsed (not 150 ms).
- Retry exhaustion is bounded and non-silent.
  - *Given* container 0x0 throughout, *When* `refit()` is called and 20 rAF attempts or 1000 ms elapse, *Then* a warn is logged, `postFitRepaint` is called once, `pendingRefit` is set, and no further rAF is scheduled; *When* the observer later delivers non-zero size, *Then* a fit/repaint request runs and `pendingRefit` clears.
- Zero-then-restore with identical size still repaints.
  - *Given* container sized 320x400, collapsed to 0, restored to 320x400, *When* the observer delivers the restore, *Then* `postFitRepaint` runs even though `lastContainerSize` matches.
- Context loss recovers (WebGL only; may be moot on the target phone).
  - *Given* WebGL `onContextLoss` fires, *When* handled, *Then* the existing fallback runs and `postFitRepaint` is called with `renderer==='canvas'` afterwards.
- Canvas renderer is covered.
  - *Given* `renderer==='canvas'` from init (no WebGL), *When* `refit()` is called at unchanged dims, *Then* `refresh(0, rows-1)` is called once and `clearTextureAtlas` is not (same assertions as the WebGL case minus the atlas call).
**Files**: `components/sessions/XtermTerminal.tsx` (sampler L1038-1101; RO callback L1103-1147; imperative handle L1250-1271; `onContextLoss` handler registered at L598-601; `triggerCanvasFallback` defined L477-535; renderer-init log "WebGL2 unavailable (Android?), using canvas renderer" ~L606), `components/sessions/__tests__/XtermTerminal*.test.tsx` (extend the existing resize tests; ADR-002 behavior is documented in the code comments at ~L1020-1139 and `docs/tasks/*`, not `docs/adr/`)

##### Task 2.1.2a1: Failing tests, sampler branches (extend the existing resize tests)
- ACs "Confirmed fit repaints", "`refit()` with unchanged dims still repaints" (3 cases), "bypasses the RO debounce". **Automated regression anchor** for Q3: pins "every `refit()` repaints, including at-rest and give-up".
- Files: existing `XtermTerminal` resize test file under `components/sessions/__tests__/`

##### Task 2.1.2b1: `requestFitRef`; repaint in schedule, at-rest and give-up branches
- Implement the Design above for the sampler only (`repaintRequested` consumed per branch; request not dropped while active). Keep the change inside the mount effect.
- Files: `components/sessions/XtermTerminal.tsx`

##### Task 2.1.2b2: `refit()` on the imperative handle, non-RO trigger, loop guard
- Add `refit()` to the handle (direct sampler start, reflow read, viewport-vs-container mismatch log, loop guard); keep `fit()` as a thin alias; preserve scroll-to-bottom anchoring if the viewport was at bottom. Test: N rapid viewport events -> one `refit()` per settle; mismatch log.
- Files: `components/sessions/XtermTerminal.tsx`, test file

##### Task 2.1.2a2: Failing tests, zero-size retry, restore, context loss, canvas, stress
- ACs "Retry exhaustion", "Zero-then-restore", "Context loss", "Canvas renderer", plus the bounded stress case: 200 alternating resize/`refit()` cycles with mixed unchanged/changed dims, asserting `refresh(0, rows-1)` after every `refit()`-initiated run (no real sleeps); and `refit_should_NotRenderErrorUi_When_RetryExhausted` (UX-18: no toast, banner or role=alert; logs only).
- Files: the same test file (or a sibling `XtermTerminal.refit.test.tsx` that **imports shared fixtures from `terminalOutputTestMocks.ts` rather than redefining them**, to protect the jscpd 0.1% gate)

##### Task 2.1.2c1: Zero-size retry bound and exhaustion fallback
- `canFit` retry (20 rAF / 1000 ms); on exhaustion log, `postFitRepaint`, set `pendingRefit`, no further rAF.
- Files: `components/sessions/XtermTerminal.tsx`

##### Task 2.1.2c2: Zero-then-restore forced request
- Track `wasZero` and force a `requestFitRef` on restore regardless of the 1 px `lastContainerSize` dedupe; the RO callback consumes `pendingRefit` on its next non-zero delivery.
- Files: `components/sessions/XtermTerminal.tsx`

##### Task 2.1.2c3: Context-loss repaint and renderer ref at init
- `postFitRepaint` after `triggerCanvasFallback`; set the renderer ref at the "WebGL2 unavailable" path so `postFitRepaint` is conditional from first paint.
- Files: `components/sessions/XtermTerminal.tsx`

#### Story 2.1.3: Remove the competing 400 ms fit path
**As a** developer, **I want** one resize-driven fit pipeline, **so that** the suspected race is gone.
**Acceptance Criteria**:
- No `setTimeout(fit)` / `isFittingRef` pipeline in TerminalOutput.
  - *Given* `TerminalOutput.tsx`, *When* a `visualViewport` resize fires, *Then* the only fit-related call is `xtermRef.current.refit()` on the settled signal (grep for `isFittingRef` and timer-wrapped `.fit()` returns none).
- Visibility effect uses the same path.
  - *Given* `isVisible` goes true, *When* the effect runs, *Then* `refit()` is called (no raw 50 ms `setTimeout(fit)` at L1160-1163).
- Remaining bare `fit()` calls are enumerated with a disposition (grep-verified list; no other call sites may be added):

| Call site | Context | Disposition |
|---|---|---|
| `XtermTerminal.tsx:502` | Post-WebGL-fallback rAF fit (guarded by finite `proposeDimensions`) | **Keep** (exception): one-shot reaction to renderer swap; follow with `postFitRepaint` (Story 2.1.2) |
| `XtermTerminal.tsx:710` | Initial-mount fit after cell dims are available | **Keep** (exception): runs before the sampler/RO exist; owns first size |
| `XtermTerminal.tsx:1052` | The sampler's own confirmed fit | **Keep**: this is the owner |
| `XtermTerminal.tsx:1227`, `:1236` | Font size / family change `setTimeout(..., 0)` | **Route through `refit()`** (font change alters cell metrics, needs the repaint too) |
| `XtermTerminal.tsx:1269-1271` | Imperative handle `fit` | **Alias to `refit()`** |
| `TerminalOutput.tsx:1163`, `:1181` | Visibility fit / `onVpResize` fit | **Delete**, replaced by `refit()` |
| `TerminalOutput.tsx:1511` | `handleManualResize` (user-initiated resize button) | **Route through `refit()`** |
**Files**: `components/sessions/TerminalOutput.tsx` (L124 `isFittingRef`, L1160-1188, L1511), `components/sessions/XtermTerminal.tsx` (L1227, L1236, L1269)

##### Task 2.1.3a: Delete the old `onVpResize`/`isFittingRef` pipeline (runs AFTER Task 2.1.4c; do not merge in between)
- Task 2.1.4c has already added the settle subscription that calls `refit()`. This task deletes the old 400 ms `onVpResize` body, `isFittingRef` and the vv `resize` listener at ~L1186, so the settle path is the only one. The wiring owner is **TerminalOutput**; it does not touch the gesture hook. Between 2.1.4c and 2.1.3a both pipelines would run, so the two land together.
- Files: `components/sessions/TerminalOutput.tsx`

##### Task 2.1.3b: Visibility fit and manual-resize `fit()` to `refit()`; delete dead `isFittingRef`
- Files: `components/sessions/TerminalOutput.tsx`

##### Task 2.1.3c: Route XtermTerminal font-size/family fits through `refit()`
- Files: `components/sessions/XtermTerminal.tsx` (L1227, L1236)

##### Task 2.1.3d: Desktop regression check for deleting the 400 ms fit pipeline (required, blocks 2.1.3 merge)
- The 400 ms pipeline is ADR-002-tuned and also runs on desktop. Before merge: (1) the existing resize tests pass unchanged (record the run); (2) a desktop-Chromium manual check on the manual instance: resize the browser window repeatedly, toggle a side panel, and switch tabs away and back; the terminal fits and shows content each time with no blank and no resize storm (count `resized terminal` lines in `staplersquad.log` stays at today's order of magnitude); (3) run `tests/e2e/terminal-resize.spec.ts` as-is (Story 3.1.2's extension stays optional, but running the existing spec is required here). Record the results in the PR.
- Files: none (record in PR)

#### Story 2.1.4: Viewport settled signal
**As a** mobile user, **I want** the refit to run after the keyboard animation ends, **so that** the final size is always fitted.
**Acceptance Criteria**:
- Trailing, not leading-edge.
  - *Given* fake rAF and a height sequence 800, 640, 520, 480, 480, 480, 480 (N=3 stable frames), *When* the stream runs, *Then* `onSettled` fires once at the third 480 frame, and a later resize 480 -> 800 re-arms and fires again.
- Offset-aware and armed by both vv events (QoS item 2; `research/qos.md` #16: Chrome 108+ resizes only the visual viewport, and `offsetTop` can be nonzero while `height` is stable).
  - *Given* `height` constant at 480 and `offsetTop` sequence 0, 40, 40, 40, 40 (vv `scroll` events), *When* the stream runs, *Then* `onSettled` fires once at the third frame with `offsetTop` 40, never earlier.
  - *Given* a vv `scroll` event while idle that changes `offsetTop`, *Then* the helper arms (today `TerminalOutput.tsx:1186` listens to vv `resize` only; `ViewportProvider.tsx:34-55` already listens to both).
  - The zero-size / WebGL-loss guard is not duplicated here: it is Story 2.1.1 (`FitGuard`) and Story 2.1.2 (retry bound, `pendingRefit`).
- Timeout fallback (never-stable height, e.g. animating URL bar).
  - *Given* heights that change every frame for longer than `maxWaitMs` (600), *When* the timeout elapses, *Then* `onSettled` fires once with the latest height, and re-arms on the next change.
- Fallback when no `visualViewport`.
  - *Given* `window.visualViewport` is undefined, *When* the helper is created, *Then* it uses `window.innerHeight` (offsetTop 0), arms on `window` `resize`, and does not throw.
- Not slower than the path it replaces.
  - *Given* fake rAF at 16 ms and a height sequence that reaches its final value 300 ms after the first `visualViewport` event, *When* the stream runs, *Then* `onSettled` fires no later than 400 ms after that first event (today's fixed 400 ms path); named test `viewportSettle_should_FireWithin400msOfFirstEvent_When_HeightStabilizesWithin300ms`. The 600 ms `maxWaitMs` applies only to a never-stable height. On device (D7) the logged first-vv-event-to-`refit()` latency has a median of at most 400 ms; if it is not, tune `stableFrames`/`maxWaitMs` before claiming the redraw fix.
- Decoupled from the gesture hook.
  - *Given* the module, *Then* it imports nothing from `lib/hooks/*` and has no momentum/cancel callback; the gesture hook cancels momentum from its own `visualViewport`/`orientationchange` listeners (Story 1.2.2).
**Files**: `lib/terminal/viewportSettle.ts`, `lib/terminal/__tests__/viewportSettle.test.ts`

##### Task 2.1.4a: Failing fake-rAF tests
- Files: `lib/terminal/__tests__/viewportSettle.test.ts`

##### Task 2.1.4b: Implement `createViewportSettle(vv, raf, {stableFrames, maxWaitMs})`
- Stability key is the pair `(height, offsetTop)`; the factory subscribes to vv `resize` and `scroll` (and `window` `resize` in the fallback) and unsubscribes on dispose. Single-purpose: cancels pending rAF/timeout on dispose; no knowledge of momentum.
- Files: `lib/terminal/viewportSettle.ts`

##### Task 2.1.4c: Wire it in TerminalOutput (the FIRST `TerminalOutput.tsx` edit of Phase 2; adds, does not delete)
- `createViewportSettle` owns its event subscriptions (vv `resize` + `scroll`) and calls `() => xtermRef.current?.refit()` as `onSettled`; it is added next to the old `vp.addEventListener('resize', onVpResize)` at `TerminalOutput.tsx:1186`, which Task 2.1.3a then deletes. Subscription lives in `TerminalOutput` (not `ViewportProvider`, whose rAF batching at `ViewportProvider.tsx:34-55` stays untouched; settle reads `visualViewport` directly). Disposed on unmount; verified by a test asserting `refit()` is called once per settle and not after unmount.
- Files: `components/sessions/TerminalOutput.tsx`

#### Story 2.1.5: Keyboard-driven resize bypasses the bounce hold (QoS item 1; Tier A)
*Tier A because Q3(d) may show this is the primary blank-terminal fix; the cut line never removes it.*
**As a** mobile user, **I want** the server to learn the post-keyboard size immediately, **so that** the cleared terminal is repainted within one round trip, not 3 to 15 s later.
**Root cause hypothesis (code VERIFIED; link to the observed black screen INFERRED until Q3(d))**: `handleTerminalResize` (`TerminalOutput.tsx:909-910`) clears the xterm buffer and then calls `resize(cols, rows)` without `force`. `resize()` (`useTerminalFlowControl.ts:391-403`) treats any size present in the last 4 sent sizes (`BOUNCE_HISTORY_SIZE=4`, L92) as a bounce and holds the RPC for `min(3000 * 2^streak, 15000)` ms (L393-401). A keyboard open/close is an A -> B -> A oscillation, so the close, and later cycles, are held. The canvas is empty from the `clear()` until the server's post-resize snapshot arrives. The hold does **not** delay the local `fit()`/`refresh()`; do not describe it as delaying the refit.
**Design**: the bounce hold protects tmux/SIGWINCH from *bursts*. A resize that follows `ViewportSettled` is one deliberate, already-debounced request (the settle signal plus Story 2.1.2's loop guard now provide that protection), so only that source bypasses the hold. Value-dedup (L310-317) and the 200 ms throttle (L405-415) stay in force; the existing `force` argument is **not** reused because it also disables dedup. Bypass window: 1000 ms after a settle-driven `refit()`, equal to the sampler's own bound (`MAX_SAMPLES=20` x `SAMPLE_INTERVAL_MS=50`, `XtermTerminal.tsx:41,50`).
**Acceptance Criteria**:
- Bypass sends immediately on a bounce.
  - *Given* `sentHistoryRef` containing 80x24 and a current size of 80x20, *When* `resize(80, 24, false, { bypassBounceHold: true })`, *Then* the RPC is pushed with no timer (fake timers: 0 ms) and `currentPaneRequest` follows at +100 ms (L349-381); `bounceStreakRef` is 0 after the send (L346).
- Non-bypass behavior unchanged (regression).
  - *Given* the same state without the option, *Then* the send is held 3000 ms (streak 1), then 6000 ms (streak 2).
  - *Given* unchanged dims with the option set, *Then* the call is deduped (L310-317).
- Only settle-driven resizes bypass.
  - *Given* the settle callback has just called `refit()` and xterm's `onResize` fires 300 ms later, *When* `handleTerminalResize` runs, *Then* `resize` receives `{ bypassBounceHold: true }`; *Given* an `onResize` 1500 ms after the last settle (e.g. a ResizeObserver burst or font change), *Then* it does not.
- Debug log: each call logs `bypassed: true|false` (Task 0.1.1c fields).
**Files**: `lib/hooks/useTerminalFlowControl.ts` (signature at L292; skip only the `isBounce` branch L391-403), `lib/hooks/useTerminalStream.ts` (type at L82, passthrough at L731), `components/sessions/TerminalOutput.tsx` (`lastSettleRefitAtRef` stamped in the settle `onSettled` from Task 2.1.3a; read at L909-910), tests in `lib/hooks/__tests__/useTerminalFlowControl.test.ts` (extend) and `components/sessions/__tests__/TerminalOutput.refit.test.tsx`

##### Task 2.1.5a: Failing tests
- `resize_should_SendImmediatelyAndResetStreak_When_BypassBounceHoldAndDimsInHistory`; `resize_should_HoldThreeThenSixSeconds_When_NoBypassAndDimsInHistory` (pins today's behavior); `resize_should_StillDedupe_When_BypassAndDimsUnchanged`; `handleTerminalResize_should_PassBypass_When_Within1000msOfSettleRefit`; `handleTerminalResize_should_NotPassBypass_When_1500msAfterSettleRefit`. Fake timers only.
- Files: `lib/hooks/__tests__/useTerminalFlowControl.test.ts`, `components/sessions/__tests__/TerminalOutput.refit.test.tsx` (share fixtures from `terminalOutputTestMocks.ts`)

##### Task 2.1.5b: `bypassBounceHold` option in flow control and stream passthrough (2 source files)
- Skip only the `isBounce` branch (L391-403); type at `useTerminalStream.ts:82`, passthrough at L731.
- Files: `lib/hooks/useTerminalFlowControl.ts`, `lib/hooks/useTerminalStream.ts`

##### Task 2.1.5c: Settle stamp and wiring in TerminalOutput (1 source file; after 2.1.3a)
- `lastSettleRefitAtRef` stamped in the settle `onSettled` from Task 2.1.3a; `handleTerminalResize` passes `{bypassBounceHold:true}` within 1000 ms. Not gated on the spike (ships unconditionally; it must not be called the root-cause fix unless Q3(d) shows `bounce` with hold >= 3 s and a blank canvas).
- Optional follow-up **only if Q3(d) shows a held resize after `clear()`** from a non-settle source: have `resize()` return `'sent' | 'deferred' | 'held' | 'deduped'` and call `clearBufferBeforeResize()` only for `'sent' | 'deferred'` (separate follow-up task, both files above plus `TerminalOutput.tsx`).
- Files: `components/sessions/TerminalOutput.tsx`

#### Story 2.1.6: Return from hidden tab drops stale output and repaints (QoS item 5; conditional on Q5)
**As a** mobile user, **I want** switching back to the browser tab to show current content immediately, **so that** I do not see stale or blank output.
**Prerequisite**: Task 0.1.2f Q5b recorded. If neither a stale flash nor a blank canvas is observed, drop this story and record why in Spike Findings.
**Acceptance Criteria**:
- Stale queue dropped when a snapshot is guaranteed to follow.
  - *Given* `TerminalStreamManager.writeBuffer` holds 50 KB accumulated while hidden and `isConnected` is true, *When* `document.visibilitychange` to visible triggers the resync (`useVisibilityResync.ts:297`), *Then* the pending buffer and `pendingOutputDuringResizeRef` are discarded **before** the `currentPaneRequest` is sent; when not connected, nothing is dropped (the reconnect path replays a snapshot anyway).
- Repaint after resume.
  - *Given* the tab becomes visible, *When* the first snapshot is written, *Then* `refit({reason:'visibility'})` (or `postFitRepaint` with `reason:'visibility'`) has run once so a discarded canvas/GL context is redrawn on both canvas and WebGL renderers.
- Stall watchdog still recovers a failed resync (existing behavior, `useVisibilityResync.ts` stall timer ~L259).
**Files**: `lib/terminal/TerminalStreamManager.ts` (new `dropPendingWrites()`; today only `cleanup()` at ~L568 clears `writeBuffer`), `components/sessions/useVisibilityResync.ts` (call before `requestFullResync`), `components/sessions/TerminalOutput.tsx` (wire `refit` on document visible), tests alongside each
##### Task 2.1.6a: Failing tests (red)
- `dropPendingWrites_should_EmptyBufferAndCancelScheduledFlush`; `visibilityVisible_should_DropPendingWritesBeforeResync_When_Connected`; `visibilityVisible_should_NotDrop_When_Disconnected`; `visibilityVisible_should_RefitWithVisibilityReason`.
- Files: `lib/terminal/__tests__/TerminalStreamManager.test.ts`, `components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.6b: Implement `dropPendingWrites()`, the resync hook call and the visibility refit (green)
- Files: `lib/terminal/TerminalStreamManager.ts`, `components/sessions/useVisibilityResync.ts`, `components/sessions/TerminalOutput.tsx`

---

## Phase 3: Verification

### Epic 3.1: Automated and device verification
**Goal**: Prove criteria; be honest about what only a device can prove.

#### Story 3.1.1: Install and run jest
**Acceptance Criteria**:
- *Given* the finished tree (dependencies were installed in Task 0.0.1a), *When* `cd web-app && npx jest --no-coverage --testPathPatterns="terminal|XtermTerminal|useTerminalGestures|useVisibilityResync"`, *Then* all pass and the count is compared with the Task 0.0.1a baseline; new tests use fake timers/rAF only (no real sleeps).
**Files**: none

##### Task 3.1.1a: Run and fix (baseline already done in 0.0.1a)
- Baseline already recorded in 0.0.1a; here only re-run `pnpm run lint:duplicates` (jscpd gate, absolute 0.1% threshold, no new-code-only scoping) and `make lint`. Risk: about 9 new test files (`XtermTerminal.refit`, `TerminalOutput.refit`, `ScrollingPanel`, `TerminalOutput.toolbarKeys`, `JumpToLatestButton`, hook and flow-control additions) copy mock setup; **new tests import shared fixtures** (`terminalOutputTestMocks.ts` or a new `__tests__/fixtures/terminalMocks.ts`) instead of redefining `Terminal`/`FitAddon`/`visualViewport` mocks, and `jest.mock(...)` registration lines are the only accepted residue (they are the existing irreducible baseline).
- **Coverage targets (jest `--coverage`, run once at the end of this story and recorded in the PR)**: at least 80% line coverage on every new `lib/terminal/*` module (`scrollKinematics`, `scrollRouting`, `scrollOverride`, `scrollPosition`, `gestureMachine`, `postFitRepaint`, `viewportSettle`, `mobileDebug`); **at least 75% line and 70% branch on `lib/hooks/useTerminalGestures.ts` and `lib/hooks/useEffectiveScrollMode.ts`**; **at least 70% line on the new components (`ScrollingPanel`, `ScrollModeChip`, `JumpToLatestButton`)**; the changed sampler/`refit` branches in `XtermTerminal.tsx` each have a named test (coverage of the 1378-line file as a whole is not a target). Below target is a finding to fix or justify in the PR, not a silent pass.
- Files: as needed

#### Story 3.1.2: Desktop-Chromium resize e2e (optional, non-blocking, partial proof only)
**Acceptance Criteria**:
- *Given* the Playwright server and viewport 412x915, *When* `setViewportSize` shrinks to 412x500 then back, *Then* xterm rows equal `proposeDimensions()` after each step and the terminal canvas is non-empty (pixel sample not all black). Cannot emulate real keyboard or touch (no touch project exists); the 50-cycle metric is device-only.
**Files**: `tests/e2e/terminal-resize.spec.ts` (extend; keep `// @feature` header, no `waitForTimeout`)

##### Task 3.1.2a: Extend e2e resize spec
- Files: `tests/e2e/terminal-resize.spec.ts`

#### Story 3.1.3: Device checklist (manual, Android Chrome)
**Acceptance Criteria**: Run on the manual instance (recipe above), record pass/fail in the PR. **Record first, once per pass**: device model, Android version, Chrome version (`chrome://version`), screen size and device pixel ratio, system font scale, active renderer (canvas or WebGL), TalkBack version if used. The bug entries and PR cite these fields.
- D1 (pass/fail threshold for the drag-down defect): in a plain shell with scrollback, **10 consecutive down-drags each scroll the terminal by at least 1 line, and 10 consecutive up-drags likewise**; slow drags track the finger 1:1, no dead direction. One miss in 10 is a fail (the original defect was a direction that did nothing). Repeat the 10-and-10 in a **local session and a TUI session** (D2).
- D1b: Plain shell inside tmux (the real deployment): default routing is unverified and may send PgUp to tmux (copy-mode) rather than scroll the xterm buffer; record what happens and whether S6 `Terminal history` fixes it (feeds Task 0.1.3a).
- D2: Drag down and up in Claude Code (same threshold as D1): **10 consecutive down-drags each send at least one `\x1b[5~` (visible in `dump()`) and the TUI visibly moves by at least 1 page for a half-screen drag; likewise 10 up-drags with `\x1b[6~`**; reaches the same history as toolbar PgUp/PgDn (page-sized, non-1:1 in TUI mode); no keyboard opens; the picker fixes any misrouted case in two taps.
- D2b: Scrolling panel and picker (Auto / Terminal history / Page keys, each with its description) and the mode chip: reachable with the keyboard closed and the toolbar collapsed, chip tap opens the picker which closes on select, labels and targets >= 44x44 readable, effective mode shown under Auto and in the chip (also with Gesture scrolling Off), setting persists across reload, TalkBack announces the radios and changes once; panel open with the keyboard up never leaves fewer than 5 terminal rows; chip hidden on a mouse-only desktop and shown on the phone; first-use hint appears on the first drag, stays until dismissed, and does not reappear after "Got it"; the misroute cue highlights the chip after a zero-movement local drag. **Misroute trial (requirements.md)**: two sessions per scenario (Claude Code in tmux; plain tmux shell), each a continuous 10-minute run with at least 20 drag gestures, fresh page load and debug flag on; read `__termDebug.stats()` at the end (outcome: Claude Code in tmux 0 override flips, plain tmux shell at most 1) and record the counts.
- D2c: Rotate or toggle the keyboard mid-drag: gesture cancels, no jump, no further scroll from that touch.
- D2d: Paste about 2 KB into a Claude Code session and drag during the paste (TUI target forced via S6): pasted text is intact; page keys are dropped while the paste is in flight (Story 1.2.6).
- D2e: Direction: drag down sends PgUp and reveals earlier output; drag up reveals later output (the original bug was direction); verify with `dump()` bytes `\x1b[5~` on drag down in a TUI session.
- D2f: Jump to latest: scroll up in local buffer and in Claude Code; button appears, tapping returns to live output without opening the keyboard; output arriving while scrolled up holds position and shows the new-output label; no scroll under a finger during output.
- D3: Slow drag (about 3 px/frame) scrolls; fling coasts and stops at edge; touch stops it without focusing the terminal.
- D4: Drag down at page top does not pull-to-refresh or bounce.
- D5: Regression and equivalence: in a TUI route toolbar PgUp/PgDn behave exactly as before; in a local route they scroll the same xterm history the drag scrolls (AC27); with a modifier armed they send modified bytes; record which history PgUp reaches in a plain tmux shell and in Claude Code (including the normal-buffer-TUI residual risk); long-press selection, tap-to-focus, double-tap word-select unchanged.
- D8: Accessibility: font-size setting reaches 200% (alternative to blocked pinch-zoom) **and at 200% the toolbar, chip, picker, panel and Redraw button remain usable (no clipped or overlapping controls, targets >= 24x24, chip and Redraw visible with the toolbar collapsed)**; **TalkBack with Gesture scrolling Off**: explore-by-touch and double-tap activation work on the terminal region and toolbar, toolbar PgUp/PgDn scroll, pinch-zoom is restored over the terminal, and **MUST PASS: the user can focus the terminal and type (keyboard opens, text is entered) with Gesture scrolling Off**; a failure here blocks closing Story 1.2.5 (build a focus affordance) rather than being recorded as a note; **TalkBack with the default On**: record whether explore-by-touch and activation still work (if yes, the setting stays as an escape hatch; if no, the panel text and an app-level hint become required follow-ups, recorded); PgUp/PgDn, chip and panel target sizes measured (44x44 target, 24x24 floor) and recorded.
- D9: Jank and tuning (owner: the developer; **pass = DevTools performance trace of three drags and three flings of about 5 s each shows no frame longer than 32 ms; summary attached to the PR; a fail blocks closing Tier A**): run the tuning criteria in design/ux.md S1 at the default `SLOP_PX` 15 (try 10 only if criterion (c) fails; include a drifting-tap check of 8-14 px) and record the result; tune the TUI half-page step and momentum constants; record final values in this plan.
- D10: Redraw: the always-visible "Redraw" button is visible and at least 44x44 CSS px on a 360 px viewport with the toolbar collapsed; with a forced blank it recovers in one tap.
- D11: Landscape with the keyboard open: chip and jump button hidden below 5 visible rows, toolbar keys still work; reconnect while scrolled leaves no stale button or gesture; selection of more than one screen behaves as the documented known WCAG 2.5.1 limitation (drag-selection beyond one screen; see "Plan risks").
- D6: Reduced-motion on (Android "Remove animations"): no fling.
- D7: Keyboard open/close N cycles (N = 50 or the baseline-derived value, below), including rotation once, with `debug-terminal-mobile` on: 0 blank terminals; if any, `dump()` attached to the PR. Compare with the baseline blank rate `b` from Task 0.1.2d and run `N = ceil(3 / b)` cycles instead of 50 when `b` is below about 6%; if `b` was "not reproduced" or "not measured", report D7 as unproven whatever the result.
  - *Given* a Claude Code session and the keyboard closed, *When* opening and closing it 50 times, *Then* content is visible within one sampler tick (50 ms) plus one animation frame after `ViewportSettled` every time (count of blank = 0). Record the active renderer (canvas vs WebGL) with the result. With the resize log on, confirm each keyboard-close `resize()` shows `bypassed:true` and no `holdMs` (Story 2.1.5).
**Files**: none

##### Task 3.1.3a: Execute checklist and record results (device-gated)
- Files: PR description

#### Story 3.1.4: Ship hygiene
**Acceptance Criteria**:
- *Given* the finished diff, *When* `make quick-check` and `make ready` run, *Then* both pass; `make registry-generate` produces no uncommitted changes unless markers moved.
**Files**: `docs/registry/features/*` (only if changed)

##### Task 3.1.4a: Run gates
- Files: as needed

---

## Deferred / follow-up QoS (not implemented in this project)

Source: `research/qos.md` section 3 ranks 7-13 and survey table. These are recorded so they are not re-researched; none blocks the two user problems. Citations are to qos.md rows unless noted.

| ID | Item | Why deferred | Starting point when picked up |
|---|---|---|---|
| D1 | Honor client `FlowControl` on the control-mode WebSocket path (rank 7; survey #5, #7) | Medium cost, protocol-adjacent; gap exists only if Task 0.1.2f Q4b confirms it. `connectrpc_websocket.go` has zero `FlowControl` references (VERIFIED); only `session_service.go:4503` handles it | Skip `ws.send` while paused, then resume with a snapshot; the xterm guide notes client ACK and server accounting must agree or the stream stalls |
| D2 | tmux `pause-after` / `%pause` / `%continue` backpressure (rank 8; survey #8) | Medium-high: parser and tmux version gating. Today a slow subscriber is closed after a 250 ms grace (`session/tmux/control_mode.go:59-68`), i.e. disconnect then full snapshot | `refresh-client -f pause-after=<secs>`, then `capture-pane` and `refresh-client -A '%id:continue'` ([tmux wiki](https://github.com/tmux/tmux/wiki/Control-Mode)) |
| D3 | Sequence-numbered live stream, server ring buffer, resume by offset (rank 9; survey #4, #11) | High: protocol change. No `sequence` on the live `TerminalOutput` stream today; only history has `requestScrollback(fromSequence, limit)` (`useTerminalFlowControl.ts:420`) | Eternal Terminal's BackedWriter model ([ET](https://eternalterminal.dev/howitworks/)) |
| D4 | DEC 2026 synchronized-output buffering in the client writer (rank 10; survey #9) | Partial support exists: `TerminalStreamManager.ts` already forces a `refresh` after `\x1b[?2026l` (L369-373), but it does not buffer between `?2026h` and `?2026l`, so TUI frames can still tear. Fold into the repaint work only if trivial; not needed for either problem | Hold writes between the markers and flush as one `terminal.write` in rAF |
| D5 | RTT indicator and RTT-adaptive server flush interval (rank 11; survey #3, #13) | Needs measurement first; qos.md's mosh constants are INFERRED, not fetched. No RTT signal exists client-side | Reuse the resync round trip; mosh heartbeat model |
| D6 | Mosh-style predictive local echo (rank 12; survey #2) | **Rejected, not just deferred.** Our users read output and steer an agent TUI (Claude Code) whose input box redraws asynchronously, so a client-side prediction overlay is brittle there; neither requirement is about typing latency; cost is high (cell-level overlay plus validation). Revisit only if typing latency complaints appear at RTT above about 150 ms | n/a |
| D7 | `permessage-deflate` (rank 13; survey #14) | **Out.** App-level gzip envelope compression already exists: `server/protocol/compression.go:35` `CompressEnvelopeIfLarge`. Client-side decompression and the threshold were not verified (qos.md section 2); verify those instead of adding a second layer | Check threshold and that the client decompresses |

---

## Plan risks
- Q1-Q3 verdicts require a physical Android device (an explicit dependency) and a reachable manual instance. They no longer block merge (decision 2026-10-01): default routing ships unverified, the PR states so and lists the misroute risk (Routing Verification Statement), and the spike confirms or adjusts rows afterwards. If no device is available, the 50-cycle metric stays unproven and rows stay `ROUTING_VERIFIED=false`.
- The target phone likely uses the canvas renderer (WebGL2 unavailable log at ~`XtermTerminal.tsx:606`), so WebGL-only fixes may be moot; the renderer-independent `refresh` after `refit()` is the load-bearing repaint.
- TUI scroll is deliberately non-1:1 (page-accumulated, capped, rate-limited); only `xterm-local` is 1:1. requirements.md Success Metrics now states this carve-out (user decision 2026-10-01), so it is no longer an unsigned exception.
- Routing misclassification is now the **accepted default risk**: a plain tmux shell (alternate screen) is misrouted to PgUp/PgDn and a normal-buffer TUI routes local; mitigated by the override toggle, the always-visible effective-mode chip, the first-use hint and the route-aware toolbar keys, corrected by the Q2 mode matrix. tmux copy-mode cannot be detected from the client, so only the hint covers a stuck copy-mode.
- Route-aware toolbar keys (Story 1.2.9) change a proven path: a normal-buffer Claude Code that auto-routes local now scrolls xterm history from the toolbar instead of reaching the app, softened by the edge fall-through and cured by Page keys; D5 records which case occurs.
- Gesture scrolling Off (TalkBack fallback) is a manual switch because page JavaScript cannot detect TalkBack; whether the default On already works with TalkBack is unknown until D8.
- SGR/X10 encoding: xterm does not expose the app's chosen mouse encoding; wheel path may be ignored by an app that did not enable 1006.
- PgUp/PgDn in non-TUI alt-screen apps (less/vim) may behave differently than a wheel; `pgkeys` is page-sized, not 1:1.
- Removing the TerminalOutput fit path touches ADR-002-tuned code that desktop also uses; the existing resize tests plus the required desktop regression check (Task 2.1.3d) are the safety net.
- `web-app/node_modules` absent: Task 0.0.1a (`pnpm install` and baseline jest run) is the first task of the project.
- **Riskiest assumption**: that the shipped default rows (alternate screen or mouse tracking -> `tui-pgkeys`, else local) match what the spike observes for both a plain tmux shell and Claude Code. **Pivot criterion** (applied in Task 0.1.3a): if the Q2 matrix shows the two scenarios report the same `(bufferType, mouseTrackingMode)` (so no row can tell them apart), set the table default to the scenario the operator uses most (Claude Code -> `tui-pgkeys`) and rely on the picker for the other; if misroute trials in D2b still need more than 1 override flip per session, per-session memory of the override (currently a follow-up) moves into scope as a new story with an explicit appetite decision.
- **Schedule credibility**: about 236 h (5.9 weeks) against a 6-week ceiling leaves about 4 h of slack; Tier A build ends in week 5 and Tier B competes for week 6. See "Schedule and sequencing".
- **Known WCAG 2.5.1 limitation**: drag-selection beyond one screen has no single-pointer alternative other than extending screen by screen with the route-aware toolbar keys. Recorded, not fixed here (follow-up: selection edge auto-scroll).
- The hook absorbs about ten options; Task 1.2.4b extracts the S9 state machine into `lib/terminal/gestureMachine.ts` so the hook does not become the next hotspot.
- Appetite is Large (3-6 weeks); the overrun cut line is in requirements.md and the Schedule and sequencing section.
- xterm 6 built-in touch `Gesture` may compete with the hook; fallback order in Unresolved Questions.
- QoS Story 2.1.5 relies on the bounce hold being the cause of long blank periods; Q3(d) may show otherwise. It is still safe to ship (the hold only protects against bursts and the bypass is limited to settle-driven resizes within 1000 ms), but it must not be claimed as the root-cause fix without the Q3(d) log.
- QoS Q4/Q5 are investigation-only; their results change the Deferred list, not this project's scope.
