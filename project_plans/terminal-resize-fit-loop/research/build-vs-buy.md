# Build vs. Buy — Terminal Resize/Fit Loop Fix

Agent 6, Phase 2 research. Scope: `web-app/src/components/sessions/XtermTerminal.tsx` (ResizeObserver → FitAddon loop, WebGL fallback) and `web-app/src/lib/hooks/useTerminalFlowControl.ts` (resize RPC dedup).

## Current versions in use (web-app/package.json)

| Package | Installed (locked) | Latest stable | Notes |
|---|---|---|---|
| `@xterm/xterm` | 5.5.0 | **6.0.0** | Major bump available; not evaluated further (out of scope — would be a separate upgrade project, not a resize-loop fix) |
| `@xterm/addon-fit` | 0.10.0 | 0.11.0 (+ active `0.12.0-beta.*` prereleases, most recent beta 2026-07-19) | One stable release behind |
| `@xterm/addon-webgl` | 0.18.0 | 0.19.0 | One stable release behind |
| `@xterm/addon-canvas` | not installed | 0.7.0 | Not a current dependency at all |

## 1. Existing OSS

**Is there a newer/different FitAddon or community "stable fit" wrapper that already solves the ResizeObserver↔fit() feedback loop?**

No. Searched `xtermjs/xterm.js` issues for "ResizeObserver loop", "fit() infinite loop", and related terms. Found several long-standing, still-open reports of FitAddon resize instability, none with an upstream structural fix:

- [#4841 "FitAddon resizes incorrectly"](https://github.com/xtermjs/xterm.js/issues/4841) — multiple users (2023–2024) report `fit()` causing dimension oscillation/growth on refresh or with the DOM renderer. Maintainer (`jerch`) could not reproduce reliably; root-caused informally to reference-char measurement rounding (`#4366`, float→integer switch) and DOM-renderer row-height quantization, not to a code fix. Still open.
- [#4113 "revamp resize logic in demo"](https://github.com/xtermjs/xterm.js/issues/4113) — maintainer (`Tyriar`) confirms resize inconsistencies stem from a broader unresolved design gap (`#702`, "no dimensions being exposed") that both the demo app and VS Code's terminal work around with private/bespoke resize logic, not a library-level fix. One linked PR (`#5541`) addresses a sub-piece but the general "revamp" is still open.
- [#5320 "wtf why it goes width=1"](https://github.com/xtermjs/xterm.js/issues/5320) — maintainer could not reproduce in the official demo despite calling `fit()` repeatedly and at edge cases; closed as unreproducible, no fix shipped.
- Diffed `FitAddon.ts` between 0.10.0 (installed) and 0.11.0 (latest stable) via unpkg/jsDelivr: `proposeDimensions()` core algorithm (measure `_terminal._core._renderService.dimensions`, divide by cell size, floor) is unchanged in substance between these versions. No stability-relevant fix landed there.

Conclusion: **the addon itself has no known upstream cure for this class of bug.** The maintainers' own position (per #4113/#702) is that embedders (VS Code included) must write their own resize-stabilization/debounce layer on top of `proposeDimensions()`/`fit()`. This is exactly the shape of the acceptance criteria (2-tick confirmation, dead-band, explicit force flag) — it is the expected, sanctioned pattern, not a workaround for a solved problem.

No community "stable fit" npm wrapper package was found (searched npm for xterm resize/fit/debounce wrapper packages — `xterm-for-react`, `react-xtermjs`, `@pablo-lion/xterm-react` etc. exist as React binding libraries, but none implement dead-band/tick-confirmation resize stabilization; they wire `ResizeObserver` → `fit()` directly, same naive pattern as the code we're fixing).

**Verdict: Not recommended (no OSS fix exists) — confirms hand-building the stabilization layer is necessary, not a reinvention of an already-solved problem.**

**Should `@xterm/addon-fit`/`@xterm/addon-webgl` be bumped to latest stable as part of this fix?**
Optional/low-value. Diff review shows no behavior change relevant to the loop bug between 0.10.0→0.11.0 or 0.18.0→0.19.0. Bumping is safe but shouldn't be treated as part of the fix — the fix belongs in application code regardless of addon patch version. **Viable but out of scope**; can be a follow-up patch-version bump.

**Does `@xterm/addon-canvas` need to be added as an explicit dependency for the canvas fallback, or does xterm.js fall back to its default DOM renderer automatically?**

Confirmed via xterm.js addon-webgl README and issue #3271 ("Make the DOM renderer the default and move the canvas renderer into an addon"): xterm.js core ships **only the DOM renderer** built-in. Both WebGL and Canvas renderers were extracted into separate addon packages. When `WebglAddon` is disposed (e.g., in an `onContextLoss` handler), xterm.js's `RenderService` reverts to the DOM renderer automatically — **no canvas addon required for that path**.

However, the acceptance criteria specifically call for a **canvas-renderer fallback** (not DOM) on WebGL px/col discrepancy — canvas renderer is a deliberate middle ground (faster than DOM, more measurement-stable than WebGL in the reported discrepancy scenario). Since canvas is not bundled by xterm.js core, **`@xterm/addon-canvas` must be added as an explicit new dependency** (`^0.7.0`, matching the `@xterm/xterm@5.5.0` peer range — canvas 0.7.0's peer dep on `@xterm/xterm` is `^5.0.0`, confirmed compatible; do not go to a `0.8.x`+ if one targets `@xterm/xterm@6` without also bumping xterm itself).

**Verdict: Recommended — add `@xterm/addon-canvas` as a new pinned dependency; this is "buy" for the fallback renderer itself (don't hand-roll a canvas renderer), "build" for the discrepancy-detection/switchover logic around it.**

## 2. SaaS / managed

Not applicable. This is a client-side rendering/measurement bug inside a browser terminal emulator component (DOM/WebGL/Canvas cell-size math and React effect scheduling) — there is no managed service, API, or SaaS product that addresses ResizeObserver/xterm.js resize semantics. Skipped per instructions.

## 3. LLM-generated vs. battle-tested library, per sub-problem

### (a) Debounce / 2-tick dead-band confirmation logic

Reviewed: does this warrant pulling in `lodash.debounce` or a similar tested utility, or is hand-written ref-based state safe?

- Searched the codebase for existing debounce utilities: `web-app/src/lib/hooks/useTerminalFlowControl.ts` already implements ad hoc time-throttling (`RESYNC_THROTTLE_MS = 2000`, `THROTTLE_MS = 200`) via plain `Date.now()` comparisons and refs — no shared debounce hook or lodash dependency exists anywhere in `web-app/`. `lodash` itself is not currently a dependency (checked `package.json`).
- The required logic is **not a generic debounce** — `lodash.debounce` collapses rapid calls to one trailing call after a quiet period; it has no concept of "value must repeat unchanged across N ticks" (dead-band + hysteresis over specific proposed values, not just call-rate limiting). Using `lodash.debounce` would still let a single ResizeObserver callback with a transient, incorrect `proposeDimensions()` reading through un-checked — it solves call-rate, not oscillation/flapping, which is the actual bug class documented in xterm.js #4841. Adding `lodash` as a new dependency purely for `.debounce()` (when the codebase has zero other lodash usage) is also disproportionate.
- The 2-consecutive-tick confirmation described in the acceptance criteria is a small, fully-testable state machine: `{ lastProposed: {cols,rows} | null, pendingConfirm: {cols,rows} | null }` updated in the `ResizeObserver` callback, ~15–25 lines. It has clear unit-testable boundaries (jitter case, boundary-flapping case) called out explicitly in the acceptance criteria (#6), which is itself the safety net a library would otherwise provide.

**Verdict: Recommended — hand-write it.** The existing codebase pattern (refs + manual comparisons, no debounce lib) is consistent with this; pulling in `lodash.debounce` adds a dependency without covering the actual bug (value-repetition, not call-rate) and this logic is small enough that the acceptance criteria's own regression-test requirement (#6) provides the correctness guarantee a library would otherwise buy.

### (b) WebGL failure detection / fallback trigger

Reviewed: is a bespoke actual-vs-expected pixel-width diff reasonable, or does `WebglAddon` expose an official error/loss event that should be used instead?

- Confirmed `WebglAddon` (via GitHub README and `WebglAddon.ts`/`addon-webgl.d.ts` typings) exposes exactly one relevant public event: **`onContextLoss: IEvent<void>`**, which fires on the canvas's native `webglcontextlost` event (GPU context dropped — OOM, driver reset, tab suspend/resume). This is a real, well-documented, officially-supported hook and should absolutely be wired up (cheap, "buy" — call `addon.dispose()` in the handler, which auto-reverts to DOM per the addon-canvas/webgl research above).
- **`onContextLoss` does not cover the specific bug described in this project**: a WebGL px/col *measurement discrepancy* (character-cell-width math disagreeing with actual rendered pixel width) is not a context-loss event — the GPU context is alive and rendering, just measuring cells inconsistently vs. the DOM-based `proposeDimensions()` math. Nothing in xterm.js's public API (`WebglAddon`, `Terminal`, `FitAddon`) exposes a "measurement mismatch" event; this is not a documented/known bug with an official signal, so it cannot be "bought."
- Therefore the acceptance criteria's `Number.isFinite`-guarded actual-vs-expected width diff is the only viable detection mechanism for *that* specific failure mode. It's a bespoke heuristic, but a narrowly scoped one (one comparison, one tolerance constant, one-directional escalation) that composes with, rather than replaces, the officially-supported `onContextLoss` handler.

**Verdict: Split** — **Recommended (buy)**: wire `WebglAddon.onContextLoss` as the primary/cheap GPU-context-loss path. **Recommended (build)**: bespoke `Number.isFinite`-guarded px/col discrepancy check as a second, independent trigger for the measurement-mismatch failure mode the official API doesn't cover — both should exist side by side, they are not alternatives to each other.

## 4. Fork or adapt — prior art in this repo's history

Ran `git log --oneline -- web-app/src/components/sessions/XtermTerminal.tsx` and `git log --oneline -- web-app/src/lib/hooks/useTerminalFlowControl.ts`. Full history (7 and 2 commits respectively):

```
XtermTerminal.tsx:
8638ec2a feat: Add mouse support to web terminal
9cb8422c feat(approvals): persist approval decisions and improve ApprovalCard UX
5b492216 feat(frontend): quick wins - typed hooks, terminal themes, modal state, routes
dd52e9f4 feat(terminal): improve terminal rendering and scrollback handling
31c6768a test: add comprehensive test coverage and performance benchmarks
5a13debc feat: implement Phase 1 of Claude config editor feature
14ee8c95 docs: update TODO.md with accurate Web UI progress (40% complete)

useTerminalFlowControl.ts:
53327064 feat(frontend): adopt Redux Toolkit + protobuf-es v2 for shared state management
6027479f refactor(terminal): decompose TerminalOutput and useTerminalStream into focused modules
```

No commit message or diff references resize loops, ResizeObserver debouncing, fit-loop guards, or reverted resize fixes. The current `resizeCount <= 3 ? 10ms : 250ms` adaptive-debounce logic (lines 254–297 of `XtermTerminal.tsx`) and the `RESYNC_THROTTLE_MS`/`THROTTLE_MS` time-throttles in `useTerminalFlowControl.ts` (lines 108, 372) appear to have been introduced organically as part of `dd52e9f4` ("improve terminal rendering and scrollback handling") or later feature work, not as a dedicated bug-fix commit — i.e., today's dedup logic is itself the naive first attempt, not a previously-tried-and-abandoned stabilization approach. There is nothing to fork or avoid re-treading; this is genuinely greenfield for the dead-band/2-tick pattern.

**Verdict: Not applicable / N/A** — no prior fit-loop-guard pattern exists in history to reuse or avoid. Build fresh, informed by the specific past-tense gap (time-only throttling with no value-dedup, confirmed still present at `useTerminalFlowControl.ts:364-410`'s `resize()` — it checks `THROTTLE_MS = 200` elapsed time but does not compare incoming `(cols,rows)` against the last-sent pair, exactly the gap acceptance criterion #3 calls out).

## Summary table

| Sub-problem | Option | Verdict |
|---|---|---|
| ResizeObserver↔fit() loop fix, generally | Upstream xterm.js/FitAddon fix | Not recommended — doesn't exist, maintainers confirm it's an embedder responsibility |
| Bump `@xterm/addon-fit`/`addon-webgl` to latest stable | Dependency bump | Viable, out of scope for this fix |
| Canvas renderer for WebGL fallback | Add `@xterm/addon-canvas` dependency | Recommended — required, not bundled in core |
| 2-tick dead-band debounce logic | Hand-write (build) | Recommended |
| 2-tick dead-band debounce logic | `lodash.debounce` or similar (buy) | Not recommended — wrong problem shape, new dependency for no coverage gain |
| WebGL GPU context loss | `WebglAddon.onContextLoss` (buy) | Recommended — official, cheap, should be wired regardless |
| WebGL px/col measurement discrepancy | Bespoke pixel-diff heuristic (build) | Recommended — no official signal exists for this failure mode |
| Reuse a prior in-repo fit-loop-guard | Fork/adapt from git history | N/A — no prior attempt exists |
