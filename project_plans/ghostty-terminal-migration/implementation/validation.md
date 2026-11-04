# Validation Plan: ghostty-terminal-migration

**Date**: 2026-10-29

## Validation Dimensions

### 1. API Compatibility (Phase 0 gate)

**What**: ghostty-web's JS/TS API satisfies `ITerminal` and `XtermTerminalHandle`.  
**How**: Spike script (`scripts/ghostty-spike.ts`) that imports ghostty-web, calls each
required method, and logs the result.  
**Pass**: All 12 `XtermTerminalHandle` methods callable without TypeScript errors;
`ITerminal.write()`, `clear()`, `refresh()`, `scrollToBottom()`, `buffer.active` all
resolve without runtime error.  
**Fail**: Missing method → define shim in `GhosttyTerminal.tsx` before proceeding.

### 2. WASM Bundle Size

**What**: ghostty-web WASM + JS does not add > 500 KB to the initial bundle.  
**How**: `bun build` with `--analyze` flag; compare `build/stats.json` before and after
adding the dependency.  
**Pass**: WASM is loaded lazily (not in the initial chunk); LCP on a cold load does not
regress > 200 ms.  
**Fail**: Block on lazy-load implementation before shipping.

### 3. DOM Docking (Phase 0 gate)

**What**: Moving the ghostty-web canvas element via `appendChild` does not destroy
renderer state.  
**How**: Manual test: create two host divs, render a recognizable string, move the host
div, confirm string is still visible and can receive new input.  
**Pass**: Visual output survives move; subsequent `write()` calls render correctly.  
**Fail**: Redesign TerminalPool docking strategy (off-screen div + CSS transform).

### 4. VT Conformance (Phase 1 baseline, Phase 3 gate)

**What**: Each of the 12+ conformance test vectors produces the expected output.  
**How**: `web-app/src/lib/terminal/__tests__/conformance.test.ts` feeds vectors to the
active renderer and diffs against `.expected.txt` snapshots.  
**Baseline**: Run against xterm.js; record which tests fail (expected failures).  
**Pass (post-migration)**: ghostty-web passes ≥ baseline + at least the sequences in the
"known xterm.js gaps" table (Synchronized Output, Kitty keyboard, REP, DECALN).  
**CI gate**: `make ready` blocks on this after Phase 3.

### 5. TerminalStreamManager Contract

**What**: `TerminalStreamManager` with a ghostty-web-backed `ITerminal` correctly:
- Buffers and writes 16 KB chunks without data loss.
- Clears and repaints on `ANSI_SNAPSHOT_PREFIX`.
- Pauses at high watermark, resumes at low watermark.
- `prependScrollbackBatch` serializes and re-writes existing content correctly.  
**How**: `TerminalStreamManager.test.ts` and `TerminalStreamManager.resync.test.ts`
must pass with the ghostty adapter substituted for the mock terminal.  
**Pass**: All existing test assertions pass; no new failures introduced.

### 6. Unit Test Coverage

**What**: GhosttyTerminal component has unit tests covering init, write, fit, search.  
**How**: `make test` in `web-app/`.  
**Pass**: All new tests in `GhosttyTerminal.*.test.tsx` pass; no test regression in
existing `XtermTerminal.*.test.tsx` (xterm.js path still tested, still passes).

### 7. Build & Lint

**What**: `make ci` passes.  
**How**: Run locally; confirm lint (ESLint, golangci-lint), TypeScript compilation, and
all unit tests pass.  
**Pass**: `make ci` exits 0.

### 8. TerminalPool Keep-Alive

**What**: Switching which session a pane shows does not re-initialize the ghostty-web
WASM, and old content is visible instantly.  
**How**: `TerminalPool.keepalive.test.tsx` (updated) + manual test: assign session A to
pane 1, type some output, switch to session B, switch back to session A — content must
be intact.  
**Pass**: No blank-terminal flash, no WASM re-init log messages.

### 9. E2E Screenshot Conformance

**What**: A real session whose output includes SGR colour and cursor-movement sequences
renders identically (within pixel tolerance) to the xterm.js baseline screenshot.  
**How**: `tests/e2e/terminal-conformance.spec.ts` Playwright screenshot diff.  
**Pass**: Pixel diff < 5% (expected; font metrics differ slightly between renderers);
no structural corruption (missing rows, wrong colours).

### 10. Feature Flag Rollback

**What**: Disabling the `ghostty_renderer` flag instantly reverts to xterm.js with no
rebuild required.  
**How**: Toggle flag in config JSON; reload browser; confirm XtermTerminal renders.  
**Pass**: Fallback works in < 2 s (next page load); no errors in browser console.

## Rollout Criteria for Flag Flip

Only flip `ghostty_renderer` to `true` by default after ALL of the following:

- [ ] VT conformance gate passes (ghostty-web ≥ xterm.js baseline + gap closures)
- [ ] `TerminalStreamManager.resync.test.ts` passes with ghostty adapter
- [ ] TerminalPool keep-alive manual test passed on a real instance
- [ ] E2E screenshot conformance test passes
- [ ] `make ci` passes
- [ ] Bundle size measurement confirms lazy-load (no LCP regression)
- [ ] 24 h soak on a developer's personal instance with no rendering regressions reported

## Rollback Plan

If ghostty-web introduces regressions after the flag flip:
1. Set `ghostty_renderer: false` in default config. (`make install-service` deploys the
   change to the live instance in < 30 s.)
2. File a bug in the conformance runbook with the failing sequence.
3. Leave xterm.js in the build until the regression is fixed.
