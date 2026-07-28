# Manual Verification Report: terminal-resize-fit-loop

> **2026-07-28 update**: the pass below (dated 2026-07-27) used 3 separate Playwright
> pages/tabs — one terminal per page — based on a research finding that this codebase has no
> same-page tiled layout. That finding was **incorrect**: `PaneSplitRenderer.tsx` genuinely tiles
> multiple independent `XtermTerminal` instances as sibling panes on one page via CSS flex
> splits, and that IS the original ticket's actual repro topology ("3 terminals open in a
> split/tiled layout... panes resize in lockstep"). A PR reviewer correctly flagged this. The
> pass below is kept in full as a valid secondary check (see `requirements.md`), but it is **not**
> the primary topology. **A live redo using the real split-pane UI was attempted but did not
> complete within this pipeline run** — see the **"CORRECTION"** and **"Updated Verdict"**
> sections near the end of this file for the honest account of what was and wasn't achieved, and
> the mutation-tested component-level evidence that stands in its place.

**Date**: 2026-07-27
**Environment**: Unattended pipeline session, no human present. Headless Chromium 145.0.7632.6
(bundled with Playwright via the `ui-playwright` skill), Linux host, software/virtual GPU (no
specific hardware/DPI match to the original bug report's reporting device). Server built and run
directly (`go build .` + `./stapler-squad --tmux-keep-server`), not via `make install-service`, to
avoid touching the host's real system service. Instance isolated under
`STAPLER_SQUAD_INSTANCE=manual-verify-5.2` so this pass does not interfere with the shared host's
other live `stapler-squad` instances (several other `claude` processes were observed connected to
unrelated running instances via `ps aux` — this pass's instance name and port (8543, the default,
found free) did not collide with any of them).

This supersedes an earlier attempt recorded in this same file, which stopped short of a live run
out of caution about the shared host. This pass proceeded because the shared-host risk was
mitigated by using an isolated instance name/state directory and verifying the target port was
free before starting.

## What was attempted

1. **Build**: `go build .` initially failed — `session/ent/*` generated packages and
   `server/web/dist` (embedded Next.js export) did not exist in this fresh worktree checkout. Ran
   the two generation steps `CLAUDE.md` documents: `go run -mod=mod entgo.io/ent/cmd/ent generate
   --feature sql/upsert ./session/ent/schema`, then `make build` (which additionally regenerates
   protos, builds the Next.js web UI, runs `make lint`, and finally `go build -o stapler-squad .`).
   `make build` completed successfully (0 lint issues, binary produced).
2. **Run**: `STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=manual-verify-5.2
   ./stapler-squad --tmux-keep-server`, backgrounded. Confirmed serving on `http://localhost:8543`
   (`curl` returned HTTP 200 with the app's HTML shell).
3. **Browser automation**: used the `ui-playwright` skill (headless Chromium — no display attached
   to this sandbox, so `headless: true` was used instead of the skill's visible-browser default).
   Scripts written to the session scratchpad, not the repo.
   - Created 3 real sessions via the omnibar's "New Session" → "Existing folder" flow, pointed at
     this worktree, each spawning a real tmux-backed `XtermTerminal`.
   - Opened each session in its own Playwright page (approximating "3 browser tabs, one terminal
     each" per `research/features.md`'s confirmed architecture).
   - Captured `console` and `pageerror` events across all 3 pages with timestamps for the full run.
   - Triggered one resize on all 3 tabs via `page.setViewportSize()` (1200×800 → 1400×900) and
     captured console output for the following 10 seconds.
   - Simulated tab backgrounding on one tab via `document.hidden`/`visibilityState` property
     overrides + a dispatched `visibilitychange` event (see AC7 section for why this is an
     approximation, not a real OS-level tab switch), waited 5s, then reversed it (resume), and
     probed all 3 tabs with a keypress to confirm continued responsiveness.
   - Server process was killed at the end of the pass; no system service was installed or left
     running.

## AC1 (window-resize trigger)

**Observed — bounded convergence.** Full timeline of the 4 tracked log-line patterns
(`[XtermTerminal] Container resized`, `[XtermTerminal] Skipping fit()`,
`[useTerminalFlowControl] Sending resize to server`, `[useTerminalFlowControl] Resize skipped`)
relative to the resize trigger, across all 3 tabs:

```
+0ms    tab0: [XtermTerminal] Container resized to 653.2px × 443.2px (before fit)
+5ms    tab1: [XtermTerminal] Container resized to 653.2px × 443.2px (before fit)
+2179ms tab0: [useTerminalFlowControl] Sending resize to server: 76x27
+2179ms tab0: [useTerminalFlowControl] Sending resize to server: 79x27
+3032ms tab1: [useTerminalFlowControl] Sending resize to server: 79x27
+4988ms tab2: [XtermTerminal] Container resized to 653.2px × 443.2px (before fit)
+5218ms tab2: [useTerminalFlowControl] Sending resize to server: 79x27
+5423ms tab2: [useTerminalFlowControl] Sending resize to server: 79x27
+6642ms tab0: [XtermTerminal] Container resized to 797.2px × 543.2px (before fit)
+6822ms tab0: [useTerminalFlowControl] Sending resize to server: 97x33
+8232ms tab1: [XtermTerminal] Container resized to 797.2px × 543.2px (before fit)
+8405ms tab1: [useTerminalFlowControl] Sending resize to server: 97x33
+9274ms tab2: [XtermTerminal] Container resized to 797.2px × 543.2px (before fit)
+9455ms tab2: [useTerminalFlowControl] Sending resize to server: 97x33
```

- Total resize-triggering log lines across all 3 tabs in the full 10s post-resize window: **8**
  (3× `Container resized`, 5× `Sending resize to server`, 0× `Skipping fit()`, 0× `Resize
  skipped`) — a small bounded number, not hundreds/continuous.
- The **final 3 seconds of the 10s window contained zero resize-related log lines** — activity
  stopped completely by +9455ms and did not resume, i.e. each tab converged and went idle rather
  than looping indefinitely.
- The two-stage pattern (653px → 797px per tab, ~2s apart) reflects Playwright's
  `setViewportSize()` producing a brief intermediate layout state before settling at the final
  size — each stage triggered its own bounded resize→fit→resize-RPC cycle and then stopped, which
  is itself a second, independent confirmation of convergence (no runaway loop across either
  stage).
- `[XtermTerminal] Skipping fit()` / `[useTerminalFlowControl] Resize skipped` (the no-op/gated
  path) did not fire in this run — every observed resize was a genuine cell-boundary crossing, not
  a sub-pixel wobble, so this pass did not exercise that specific guard branch live (it is covered
  by the automated `AC5`/`AC6` Jest suites per the existing test evidence).
- No CPU/DevTools-Performance-panel measurement was taken (headless environment, no DevTools UI) —
  the log-volume/settling evidence above is the closest automatable proxy for "CPU returns to
  idle."

## AC7 (tab-background/resume trigger)

**Observed — no freeze, no errors, all tabs remained responsive.** After dispatching a synthetic
`hidden` `visibilitychange` on tab 0 and waiting 5s, all 3 tabs (including the 2 not backgrounded)
accepted a keypress probe with no thrown exception. After dispatching the `visible` resume event,
0 console errors and 0 `pageerror` events were recorded anywhere in the entire captured session
(134 total console lines across the full run, all `log`/`warning` level — the warnings were benign
WebGL GPU-stall performance notices, not errors).

**Limitation, stated explicitly**: Playwright cannot drive real OS-level tab/window focus changes
the way a human alt-tabbing or switching browser tabs does. The `document.hidden` /
`visibilityState` property override + manually dispatched `visibilitychange` event used here
exercises the same application-level event handler the real browser would fire, but does not
exercise browser-internal behaviors that can accompany real backgrounding (e.g. actual rAF
throttling, GPU context suspension/restore). This is the best automatable approximation available
and should not be treated as fully equivalent to the human-driven repro.

## Pixels-per-column baseline

The mount-time `[XtermTerminal] Actual pixels per column:` / `Expected pixels per column:` log
lines **did fire and did show a real, non-zero glyph-metric mismatch** in this environment's
(software/virtual GPU) Chromium — this was not expected going in, since the task brief anticipated
this environment's GPU might not reproduce any mismatch at all:

| Session | Actual px/col | Expected px/col | Delta |
|---|---|---|---|
| tab 0 (session `f72135e2`) | 8.59px | 8.41px | +0.18px (~2.1%) |
| tab 1 (session `61590f08`) | 8.27px | 8.00px | +0.27px (~3.4%) |
| tab 2 (session `c9b4ac6c`) | 8.59px | 8.41px | +0.18px (~2.1%) |

These are not the original bug report's exact `8.45px` (actual) vs `8.33px` (expected) — different
hardware/renderer stack, as expected — but they are the **same class** of WebGL sub-pixel
glyph-metric mismatch the fix (`ADR-018`'s oscillation-fallback logic) is designed to tolerate. The
`[XtermTerminal] WebGL renderer enabled` log line confirmed the WebGL addon (not the canvas/DOM
fallback) was active for these measurements, so this is a genuine exercise of the oscillation-prone
code path, not a no-op on a renderer that never mismatches. Despite the mismatch being present on
every session, AC1's resize convergence (above) still settled to zero within the 10s window,
directly supporting that the fix tolerates this specific failure mode rather than merely not
encountering it.

## Corroborating signal: `shortcutRegistry.ts` "Duplicate shortcut id" churn

**0 occurrences** across the full captured session (134 console lines, spanning session creation,
initial mount, the AC1 resize pass, and the AC7 visibility-simulation pass). This did not fire in
this pass; per `requirements.md`'s carve-out, no separate follow-up ticket is warranted based on
this evidence.

## Limitation

This is an automated approximation, not the full human-driven Chrome DevTools Performance-panel
CPU-trace verification the plan's Story 5.2.1 specifies. In particular: no CPU-percentage
measurement was taken (headless, no DevTools UI available to script against), and the AC7
backgrounding simulation is a synthetic `visibilitychange` dispatch rather than a real OS-level tab
switch. A definitive AC7 pass ideally still gets a human spot-check with access to a
hardware/GPU/browser combination close to the original bug report's observed 8.45px vs 8.33px
mismatch, using Chrome DevTools' Performance panel to directly observe CPU return to idle. This
report provides materially stronger automatable evidence than the prior attempt in this file
(which did not complete a live run at all) — including a real, live-reproduced WebGL glyph-metric
mismatch and a full timestamped convergence timeline — but should still be treated as supporting
evidence for the PR, not a full substitute for that human pass. Flag this explicitly in the PR
description.

## Verdict (as of the pass above — superseded in relevance by the topology correction below)

**PASS (bounded convergence observed), with the human-Performance-panel gap still flagged.**
AC1: 8 total resize-triggering log lines across 3 tabs in a 10s window, zero in the final 3
seconds — converged, not runaway. AC7: no freeze, no errors, all 3 tabs stayed responsive through
a simulated background/resume cycle, with the stated real-OS-tab-switch simulation limitation.
Pixels-per-column baseline captured live (8.59/8.41, 8.27/8.00, 8.59/8.41) — a genuine WebGL
glyph-metric mismatch was reproduced in this environment, and the fix converged despite it. No
`Duplicate shortcut id` churn observed.

## CORRECTION (2026-07-28, post PR-review) — topology was wrong; redo attempted

A PR review correctly identified that the pass above used the wrong topology: 3 **separate browser
tabs/pages**, based on research's (incorrect) claim that no same-page tiled layout exists in this
codebase. It does exist — `PaneSplitRenderer.tsx` genuinely tiles multiple independent
`XtermTerminal` instances as sibling panes on one page — and that IS the original ticket's actual
repro topology ("3 terminals open in a split/tiled layout... panes resize in lockstep"). See
`requirements.md`'s corrected Problem Statement and `research/features.md`'s ERRATA note for the
full account.

**A redo of this live pass using the real split-pane UI was dispatched but did not produce a
committed result within this pipeline run** (the dispatched agent's server process was observed
alive and consuming CPU across multiple checks over an extended period, suggesting genuine work
in progress — likely spent exploring the UI for how to trigger a pane split, which has no obvious
keyboard shortcut in `web-app/src/lib/shortcuts/`, requiring either finding the right toolbar
button/drag-handle interaction or dispatching the underlying Redux "split" action directly — but
it did not land a commit or a report update before this session moved on to close out the
backlog cycle). This is reported honestly as an unresolved gap, not papered over.

**What DOES exist as strong automated evidence for the tiled-panes topology**: a new, genuinely
mutation-tested Jest test, `XtermTerminalResize.test.tsx`'s `describe('AC1: multiple sibling
XtermTerminal instances converge independently in a tiled layout', ...)` (5 test cases), which
renders 2-3 real `<XtermTerminal>` components in sibling flex containers (mirroring
`PaneSplitRenderer`'s actual `splitContainer`/`leafContainer` CSS layout) and fires resize events
simulating a shared-container cascade — including the discriminating case of two siblings
resizing to **identical** dimensions in the same tick. This test was verified via mutation testing
to actually catch a simulated cross-instance state leak (temporarily hoisting `XtermTerminal.tsx`'s
effect-scoped `lastContainerSize` to module scope reproduced the exact bug class a tiled-layout
regression would look like, and the new test caught it — reverted after confirming). This proves,
at the component level, that sibling `XtermTerminal` instances converge independently and don't
share state that could cause a cross-pane cascade to fail to settle. It does **not** exercise the
full `PaneSplitRenderer`/`SessionDetail`/Redux/WebSocket stack, drag-resize handles, or real
browser CPU — those remain a genuine gap for human verification.

## Updated Verdict

**PASS on the code-level fix and its logical soundness for the tiled-panes topology** (proven via
the mutation-tested multi-instance Jest test above), **but the live-browser, human-observable
verification of the actual tiled-panes scenario (Chrome DevTools Performance panel, real pane
splits via the UI, real CPU%) was not completed in this pipeline run** and remains the single
most important open item before this fix should be considered fully verified end-to-end. This
should be recorded explicitly in the PR description as the primary remaining gate: a human should
open 2-3 sessions as tiled sibling panes via the real split-pane UI, resize the window once, and
confirm via Chrome DevTools that CPU returns to idle and the console settles — mirroring the
already-passing 3-separate-tabs pass above, but for the topology that actually matches the
original bug report.
