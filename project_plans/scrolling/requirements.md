# Requirements: scrolling

**Date**: 2026-10-01
**Type**: feature addition (two user-reported defects plus QoS hardening, momentum, an override toggle and a device spike)
**Complexity**: 3
**Priority**: Top of the personal backlog for mobile use: mobile steering of agent sessions is the product's differentiator and both defects block reading output without the soft keyboard. No competing backlog item is ranked above it (no `docs/tasks/scrolling.md` exists; this file is the feature doc). **Evidence and its limit**: the target user is a single operator (the author); there is no usage data, no second user, and frequency and severity are self-reported. The priority rests on that operator's judgment of the product's differentiator, not on measured demand, and no other backlog item was formally compared (a one-person backlog); re-ranking is the owner's call. The appetite grew from two bug reports to Large because momentum, QoS hardening and the override were explicitly kept (decision 2026-10-01).

## Problem Statement
On mobile (Android Chrome), two terminal problems hurt reading output without the soft keyboard:
1. **Touch-drag scrolling is one-directional.** Touch-hold-drag scrolls up "mostly" but does not scroll down. The on-screen toolbar's PgUp/PgDn keys do drive Claude Code's internal scrollback correctly, whereas the drag gesture calls xterm's `scrollLines()` (`web-app/src/lib/hooks/useTerminalGestures.ts:246`), which only moves xterm's own scrollback, not the TUI's. **Status of the cause: hypothesized, not confirmed.** The code path is read (VERIFIED), and it explains why a drag cannot reach a TUI's scrollback; it does not by itself explain why one *direction* is dead. Spike Q2(b) (plan.md Task 0.1.2c) confirms or replaces this hypothesis; the fix ships either way (routing plus an accumulator plus CSS hardening cover every candidate), and Open Question 1 stays open until Q2(b) is recorded.
2. **Intermittent blank terminal after the keyboard opens.** The terminal sometimes stays black with content pushed off-screen instead of redrawing immediately. Root cause unknown (candidates: debounced refit in `TerminalOutput.tsx:~1169` waiting 400ms on mobile, `ViewportProvider` rAF-batched `visualViewport` updates, no forced xterm refresh after resize).

## Baseline
- **Device record (every device pass; the bug entries and PR cite it)**: phone model, Android version, Chrome version (`chrome://version`), screen size and device pixel ratio, system font scale, active xterm renderer (canvas or WebGL), TalkBack version when used. Without these fields a result is not reproducible.
- Drag up scrolls partially; drag down does nothing visible; user falls back to the on-screen PgUp/PgDn keys.
- Keyboard-open redraw failures are worked around by (presumably) closing/reopening the keyboard or reloading.
- **Baseline frequency of the blank terminal: unknown.** No repro rate has been measured. The device spike (plan.md Task 0.1.2d) must count blanks per N open/close cycles before any fix lands, so improvement is measured against a number. Note: 50 clean cycles only bound the failure rate below about 6% at 95% confidence (rule of three: 3/50), so "0 blanks in 50" is evidence of improvement only if the measured baseline rate is well above that. **Rule for N**: let `b` be the baseline blank rate measured on the unmodified build (blanks / cycles). Run the post-fix check with `N = ceil(3 / b)` cycles, which at 95% confidence (rule of three) shows the fixed rate is below `b` if 0 blanks occur (b = 6% gives N = 50; b = 2% gives N = 150; b = 1% gives N = 300). If the baseline run finds no blank in its own N cycles, the baseline is "not reproduced" and the post-fix claim stays unproven. If no device is available, the baseline is "not measured" and D7 is stated as unproven in the PR.

## Users / Consumers
Stapler-squad operators using the web UI from a phone/tablet to monitor and steer agent sessions (Claude Code in tmux).

## Success Metrics
- **Drag-down defect pass/fail (the headline defect, checked by D1/D2)**: in a local-buffer session **and** in a TUI session, **10 consecutive down-drags each scroll by at least 1 line (local) or send at least one page key and visibly move the app (TUI), and 10 consecutive up-drags likewise**. One miss in 10 is a fail. This is the pass/fail line the bug entry closes on.
- Touch-drag scrolls in both directions without opening the keyboard: **1:1 in the local xterm buffer; page-stepped, capped and rate-limited in TUI modes (Claude Code etc.)**, reaching the same scrollback the PgUp/PgDn keys use. (Decision 2026-10-01: the TUI exception is accepted; a TUI cannot be driven 1:1 because it only accepts discrete page keys or wheel reports.) **Default routing (decision 2026-10-01, reverses the earlier safe default): until the device spike verifies the routing rows, sessions whose terminal is in the alternate screen or has mouse tracking on auto-route drag to the TUI (`tui-pgkeys`, PgUp/PgDn bytes); everything else scrolls locally. Accepted cost: a plain tmux shell (which reports the alternate screen) may be misrouted until the spike corrects the rows; the user can force `local` with the scroll-mode override.** A user can override auto-routing with a persisted `auto | local | tui` scroll-mode setting.
- Jank (tune on device): no frame longer than 32 ms during an active drag or fling on the target Android phone (DevTools performance trace), and scroll dispatches stay at one per animation frame.
- Momentum (outcome): a fast fling in the local buffer coasts and then stops with no visible overshoot or bounce; a slow release does not coast; momentum is bounded and cancelled by any touch. The numeric constants (velocity thresholds, decay, frame limits) live in plan.md `MOMENTUM_CONSTANTS` and are tuned on device against Chrome's native list-fling feel; this file does not repeat them.
- After keyboard open/close, the terminal repaints with content visible promptly once the keyboard stops moving, no later than the 400 ms fit path it replaces (the exact budget is plan.md's "repaint timing budget"); 0 blank-terminal occurrences across a repeated open/close test on Android Chrome, with N set by the baseline rule above (N = ceil(3 / baseline rate)).
- Regression anchor: PgUp/PgDn toolbar keys, long-press selection, tap-to-focus, and double-tap word-select still work. **How it is checked**: characterization tests (plan.md Story 1.2.4) are written and run green against the unmodified hook before any change and re-run after every hook story in CI, plus device checklist D5. Two intended changes to the anchor: (1) tap tolerance is widened from 8 px to `SLOP_PX` (15 px) so a drifting thumb tap still taps and there is no dead zone (a tap while a selection is active now clears the selection without opening the keyboard); (2) the toolbar PgUp/PgDn become route-aware (scroll xterm history when the effective route is local, same bytes as today in a TUI route) so they are a WCAG 2.5.1 equivalent of the drag (plan.md Story 1.2.9, design/ux.md S4); byte parity in TUI routes is pinned by test.
- Accessibility (outcome): a persisted Touch gestures (scroll, select) Off setting leaves native TalkBack gestures untouched over the terminal and the toolbar keys still scroll; page JavaScript cannot detect TalkBack, so this is a manual switch verified in D8. Screen-reader access to terminal *content* is out of scope (see Out of Scope).
- Misroute visibility (local only, no telemetry): `MobileDebugLog` records route decisions, override changes and misroute-proxy events (an override change or toolbar PgUp/PgDn within seconds of an auto-routed drag). Outcome checked in the device passes: **trial = two sessions per scenario (Claude Code in tmux; a plain tmux shell), each one continuous 10-minute run with at least 20 drag gestures, fresh page load, debug flag on**; Claude Code in tmux needs 0 override flips and a plain tmux shell needs at most 1 per session, recorded in the PR. The counts are read from `window.__termDebug.stats()` (and the entries from `dump()`).
- **Post-ship outcome check (the feature worked in daily use, not only in the passes)**: after the deploy, 7 days of normal mobile use with `debug-terminal-mobile` on. Pass = **at most 3 `toolbar-key-after-drag` misroute-proxy events per day averaged, 0 blank terminals that needed Redraw or a reload, and 0 override flips after the first day**. `stats()` resets on page load, so the operator copies `stats()` into the bug entries at the end of each day. This is a manual tally by one operator, not telemetry; if the flag was off it is reported as "not measured".

**Which metrics need what to verify**

| Metric | CI-verifiable (jest, fake timers) | Device-only (physical Android Chrome) |
|---|---|---|
| Drag both directions, accumulator, direction signs | Yes | Confirmation (D1, D2) |
| Default routing table (alternate to `tui-pgkeys`, normal to `xterm-local`) | Yes (table test) | Whether the rows are right for tmux and Claude Code (spike Q2) |
| One dispatch per frame, per-frame cap | Yes | Frame trace for the 32 ms jank bound |
| Momentum constants and cancel rules | Yes | Momentum feel and tuning |
| Repaint on every refit (including at-rest) | Yes (automated anchor, 200 cycles with mocks) | The 50-cycle zero-blank run and baseline count (proves pixels, not just `refresh()` calls) |
| Toolbar PgUp/PgDn route-aware (local scrolls history; TUI bytes unchanged) | Yes | Confirmation (D5) |
| TUI wheel vs PgUp, renderer identity, pull-to-refresh, touchend click suppression, TalkBack (Touch gestures (scroll, select) Off), pinch-zoom | No | Yes |

**Explicit dependency**: a physical Android phone running Chrome, on the same network as a laptop hosting the manual instance (plan.md recipe). The single operator must have it available; without it the spike cannot run, rows stay unverified, and the 50-cycle metric stays unproven. **Calendar reservation**: the phone is needed in three booked slots (end of week 1, end of week 4 into week 5, start of week 6; plan.md "Device calendar"); the operator needs at least 1 week of notice per slot.

## Appetite
**Large (3–6 weeks)** for one developer (revised 2026-10-01 from Medium, 1–2 weeks; decision: keep all scope). Includes momentum/inertia, QoS hardening, the override toggle and its accessibility work, automated tests (about 170 named in validation.md), and device verification with iteration time.

**Schedule, stated honestly**: the recomputed total is about 236 h (5.9 weeks: about 184 h build, about 40 h device-bound, a 12 h fix-pass reserve) against a 6-week ceiling, so real slack is about 4 h (half a working day). Tier A build ends in **week 5**; Tier A is device-verified early in week 6 (three booked device slots, plan.md "Device calendar", each needing about 1 week of lead time with the operator). **Cut line if the appetite is overrun**: scope is not cut by default. Tier B (paste-interleave guard 1.2.6, hidden-tab drop 2.1.6, Q4/Q5 records 0.1.2f, wheel/X10 encoders, e2e extension 3.1.2) is scheduled in what is left of weeks 4 and 6. If week 5 ends with Tier A build unfinished, no Tier B item starts and the rest move to the backlog under the adjacent-fixes rule below. Tier A always ships: drag both directions, routing and override, momentum, toolbar-key equivalence, the Touch gestures (scroll, select) setting, and the **entire redraw fix, including the bounce-hold bypass (Story 2.1.5), which is Tier A because Q3(d) may show it is the primary fix**.

## Scope-to-defect traceability

Every kept item either fixes a reported defect or is labelled hardening so a reviewer can tell which is which.

| Item | Ties to | Label |
|---|---|---|
| Routing, accumulator, CSS, override, toolbar-key equivalence, jump-to-latest | Problem 1 (drag defect) or its escape hatches | Defect fix / mitigation |
| Repaint seam, `refit()`, settle signal, delete 400 ms pipeline, bounce-hold bypass (2.1.5), Redraw button | Problem 2 (blank terminal) | Defect fix |
| Momentum | Reading long output after the drag works (user decision 2026-10-01) | Kept feature, not a defect fix |
| Paste-interleave guard (1.2.6) | None of the two defects; prevents corrupting a paste when a drag fires mid-paste | **Hardening, not a defect fix** (Tier B) |
| Hidden-tab drop and repaint (2.1.6) | Problem 2 only if Q5b observes a blank or stale canvas on return; otherwise dropped | Conditional defect fix, else not built |
| Q4/Q5 investigation (0.1.2f), wheel/X10 encoders, e2e extension (3.1.2) | Evidence for Deferred QoS and the wheel path | **Hardening and investigation, not defect fixes** (Tier B) |

**Riskiest assumption**: that the shipped default routing rows (alternate screen or mouse tracking to `tui-pgkeys`, otherwise local) match what the spike finds for a plain tmux shell and for Claude Code. **Pivot criterion**: if the spike shows the two scenarios report the same `(bufferType, mouseTrackingMode)`, the default switches to the scenario the operator uses most and the picker handles the other; if the misroute trial still needs more than 1 override flip per session, per-session override memory moves from follow-up into scope with a new appetite decision (plan.md "Plan risks").

## Constraints
- Must work with xterm.js alt-screen and mouse-tracking apps (Claude Code TUI), as well as plain scrollback.
- Touch handling stays TouchEvent-based (ADR-012).
- Don't restart the live service to test; use a manual instance (see CLAUDE.md port block).

## Non-functional Requirements
- **Performance SLO**: scroll updates coalesced to ≤1 per animation frame; no jank on Android Chrome.
- **Scalability**: not applicable
- **Security classification**: internal
- **Data residency**: no special requirements

## Scope
### In Scope
- Drag-scroll in both directions and correct target (xterm scrollback vs. TUI scrollback via PgUp/PgDn-equivalent input or wheel/mouse events).
- A persisted `auto | local | tui` scroll-mode override toggle as the escape hatch for misrouted sessions, a visible effective-mode chip, and a persisted Touch gestures (scroll, select) On/Off setting as the TalkBack fallback.
- Route-aware toolbar PgUp/PgDn (WCAG 2.5.1 equivalence with the drag).
- Default routing ships **unverified** (decision 2026-10-01): alternate-screen or mouse-tracking sessions route to `tui-pgkeys`, others to `xterm-local`. The device spike (plan.md Phase 0) then confirms or adjusts the rows after the fact; the PR must state routing is unverified and list the misroute risk (plan.md Routing Verification Statement).
- Tracking bug entries in `docs/bugs/open/` for the two defects. **Closure criteria**: the drag bug moves to `docs/bugs/fixed/` when D1 and D2 meet the pass/fail threshold above. The blank-terminal bug moves to `fixed/` only when the D7 run meets its N-cycle threshold with a measured baseline; if D7 is unproven (no device, or baseline not reproduced or not measured, as the PR then states), the entry **stays open with status "fix shipped, unverified"** and is not moved. This keeps closure consistent with the "D7 unproven" allowance.
- Momentum/inertia on release.
- QoS items from `research/qos.md` (Large appetite, see plan.md "QoS additions"): bypass the server resize bounce-hold for keyboard-settle refits; `visualViewport` `offsetTop` + `height` in the settle signal; scroll output capped per animation frame (no fixed wheel throttle on the 1:1 path) and never interleaved into an in-flight paste; drop stale output and repaint on return from a hidden tab (conditional on a spike verdict); investigation-only spike of the production streaming path, client `FlowControl` handling and `NEXT_PUBLIC_RECONNECT_V2`.
- Root-cause and fix for the blank-terminal-after-keyboard-open redraw.
- Adjacent fixes discovered in the same code paths, **capped**: only inside the files this plan already names, only if the fix is under about 30 lines and needs no new story; anything larger goes to the backlog (`docs/bugs/open/` for defects) instead of this project.
- Unit/jest tests; e2e where feasible.
- Relabel the existing always-visible "Resize" toolbar button as "Redraw" so the manual recovery is discoverable.

### Out of Scope
- **Out of scope (accessibility)**: screen-reader access to terminal *content* (xterm `screenReaderMode`). Reason: the content is a continuously rewritten TUI screen, the feature has its own performance and verbosity design, and this project is about touch scrolling and redraw. Recorded as a follow-up.
- **Excluded (real exclusions; the user excluded nothing else, but these are not built here):** QoS follow-ups, recorded in plan.md "Deferred / follow-up QoS" and not implemented here: honoring `FlowControl` on the control-mode WebSocket, tmux `pause-after`, sequence-numbered resume, DEC 2026 synchronized-output buffering, RTT indicator. Rejected: mosh-style predictive echo (brittle with the agent TUI, off-problem). Not needed: `permessage-deflate` (gzip envelope compression exists).

## Rabbit Holes
- Deciding when to forward scroll to the TUI vs. scroll xterm locally (alt-screen? mouse-tracking? Claude Code's scrollback mode) — detection heuristics can be brittle.
- Android Chrome intermittent repro of the redraw bug; may need instrumentation before a fix is provable.
- Interaction between `touch-action`, `preventDefault`, and Chrome's own pull-to-refresh/overscroll handling.
- Momentum physics interacting with line-quantized scrolling.

## Alternatives Considered
- Sending PgUp/PgDn escape sequences from drag vs. SGR wheel mouse events vs. xterm `scrollLines()` fallback.
- Fixing redraw via explicit `terminal.refresh()`/`fit()` after viewport settles vs. replacing the fixed 400ms debounce with `visualViewport` resize-settled detection.

## Feasibility Risks
- Redraw bug is intermittent and device-specific; may only be verifiable on a physical Android device.

## Observability Requirements
`MobileDebugLog` (plan.md Observability Plan): a localStorage-flag-gated (`debug-terminal-mobile`), 500-entry ring buffer recording per-frame scroll samples (buffer type, tracking mode, target), per-fit and per-resize events, renderer, context loss and forced-refresh reasons, dumped via `window.__termDebug.dump()`. It also records `route-decision`, `override-change` and `misroute-proxy` entries (local only, see plan.md Observability). Off by default, zero overhead when off. The dump is attached to the verification record for any blank occurrence. No server metrics or alerts.

## Risk Control
Frontend-only and revertible by PR revert, but not low risk: default TUI routing ships unverified and a plain tmux shell may be misrouted (decision 2026-10-01). Controls: the **Routing Verification Statement** in plan.md (a PR-body statement checked by the reviewer, not a mechanical merge block: PR states routing is unverified and lists the misroute risk; the spike then confirms or adjusts rows), the persisted `auto | local | tui` override and toolbar PgUp/PgDn as always-available escape hatches, `MobileDebugLog` for diagnosing misroutes and blanks, and deploy via `make install-service` only after the manual-instance device pass. No feature flag.

## Open Questions
- Why does drag-down fail specifically (touchmove coalescing, `lastY` handling, mouse-tracking mode, or `preventDefault`/overscroll)? Problem 1 states a hypothesized cause (a drag cannot reach a TUI's scrollback); this question is what remains open about the one-directional symptom, to be answered by spike Q2(b).
- Is the black screen a missed `fit()`/`refresh()`, a zero-size canvas during the transition, or a WebGL context issue?
- Which scroll mechanism does Claude Code's internal scrollback respond to (PgUp/PgDn keys only, or wheel events too)?
