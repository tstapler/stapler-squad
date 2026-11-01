# Implementation Plan: ghostty-terminal-migration

**Date**: 2026-10-29

## Phase 0 — Spike (gate on research findings)

**Goal**: Confirm ghostty-web is production-usable before committing to full migration.

### Task 0.1 — API audit
- Install `@coder/ghostty-web` (or the correct npm package name from the repo) into
  `web-app/` via `bun add`.
- Confirm the package's TypeScript types. Specifically verify:
  - `write(data: Uint8Array | string)` method exists.
  - Resize / cell-dimension API.
  - Whether buffer introspection (`rows`, `cols`, `buffer.active.length`) is exposed.
  - Canvas/WASM init lifecycle.
- **Gate**: If the API is fundamentally incompatible with `ITerminal`, file a
  compatibility issue and reassess scope.

### Task 0.2 — WASM bundle size measurement
- Build a minimal HTML page that imports ghostty-web, measure total JS + WASM size.
- If > 2 MB, confirm lazy-load path works (`import()` + `<Suspense>`).
- **Gate**: If no lazy-load path, evaluate impact on LCP before proceeding.

### Task 0.3 — DOM docking proof-of-concept
- Verify that `anchor.appendChild(ghosttyHostEl)` moves the rendering canvas without
  destroying the WASM state or causing re-initialization.
- **Gate**: If AppendChild destroys state, the TerminalPool keep-alive strategy needs
  a redesign (not in scope for this plan version).

## Phase 1 — Conformance Runbook (can run in parallel with Phase 0)

**Goal**: Produce the runbook before the migration, so it can validate both renderers.

### Task 1.1 — Create test vector directory
- Create `tests/terminal-conformance/` with 12+ `.ans` test vector files per research doc.
- Each file: raw ANSI/VT bytes that exercise one feature.

### Task 1.2 — Write runbook document
- Create `docs/how-to/vt-conformance-runbook.md`.
- Sections: prerequisites, automated test vectors, manual test steps (vim, htop, Claude
  Code TUI), known xterm.js gaps, expected ghostty-web improvements.
- Include a "before migration" baseline table: which test vectors pass on xterm.js.

### Task 1.3 — CI conformance harness
- Add a Node.js/Bun test in `web-app/src/lib/terminal/__tests__/conformance.test.ts`
  that:
  1. Initializes each renderer (xterm.js baseline, later ghostty-web) in jsdom or node.
  2. Feeds each `.ans` vector.
  3. Reads back the rendered text via the ITerminal buffer or serialize API.
  4. Compares to `.expected.txt` snapshot.
- Wire into `make test` (not a separate gate yet; full gate in Phase 3).

## Phase 2 — GhosttyTerminal Component

**Goal**: Replace XtermTerminal.tsx with a ghostty-web-backed component.

### Task 2.1 — Feature flag
- Add `ghostty_renderer` to the config feature-flags system.
- Default: `false` (uses xterm.js).

### Task 2.2 — GhosttyTerminal.tsx
- Create `web-app/src/components/sessions/GhosttyTerminal.tsx`.
- Implements `XtermTerminalHandle` (same ref interface, same props as `XtermTerminalProps`).
- Uses ghostty-web's WASM init + canvas lifecycle.
- Adapter maps `ITerminal` methods: `write`, `clear`, `refresh`, `scrollToBottom`,
  `buffer.active.{cursorY, viewportY, length}`.

### Task 2.3 — Fit adapter
- Port `cellDimensions.ts` pixel-measurement logic to ghostty-web's cell-size API.
- `GhosttyTerminal`'s `fit()` and `resize()` methods call this adapter.

### Task 2.4 — Serialize shim
- If ghostty-web has no `serialize()` equivalent, implement an in-memory shadow buffer
  in `TerminalStreamManager` that is populated on every `write()` call and truncated to
  `scrollback` lines.
- Keep this opt-in behind a `useSerialize` prop; only `TerminalOutput`'s
  `prependScrollbackBatch` path needs it.

### Task 2.5 — Search shim
- Implement client-side text search over the rendered rows using ghostty-web's buffer
  introspection API (or a shadow text buffer if buffer access is not exposed).
- Match the `search(term): boolean` / `searchNext(term): boolean` contract.

### Task 2.6 — URL / link handling
- Map ghostty-web's OSC 8 link callback to the same `TerminalLinkMenu` component used
  by the xterm.js WebLinksAddon path.

### Task 2.7 — Routing in TerminalOutput.tsx
- In `TerminalOutput.tsx`, conditionally render `<GhosttyTerminal ref={termRef}>` vs
  `<XtermTerminal ref={termRef}>` based on the `ghostty_renderer` flag.
- No other changes to TerminalOutput.tsx logic.

### Task 2.8 — TerminalPool compatibility
- Verify the pool's host-element docking (`anchor.appendChild(entry.hostEl)`) works
  with ghostty-web's canvas. If an extra wrapper div is needed, add it.

## Phase 3 — Testing & Validation

### Task 3.1 — Unit tests for GhosttyTerminal
- Replace / add tests in `web-app/src/components/sessions/__tests__/` for:
  - `GhosttyTerminal.init.test.tsx`: WASM loads, canvas mounts.
  - `GhosttyTerminal.write.test.tsx`: data written → ITerminal contract satisfied.
  - `GhosttyTerminal.fit.test.tsx`: resize adapter rounds correctly.
  - `GhosttyTerminal.search.test.tsx`: search / searchNext return correct booleans.

### Task 3.2 — Update TerminalPool tests
- Update `TerminalPool.keepalive.test.tsx` and `TerminalPool.test.tsx` to exercise both
  renderer paths if possible in jsdom, or flag-guard the ghostty path with a skip.

### Task 3.3 — Conformance gate
- Promote the conformance harness from `make test` to `make ready` (a blocking gate).
- Produce a "before vs after" report: which vectors newly pass with ghostty-web.

### Task 3.4 — E2E conformance test
- Add `tests/e2e/terminal-conformance.spec.ts`:
  - Open a session running a short `printf` emitting a conformance vector sequence.
  - Take a screenshot, compare against a golden.
  - Annotate: `// @feature terminal:ghostty-renderer`.

## Phase 4 — Cleanup & Ship

### Task 4.1 — Remove xterm.js addons
- Once `ghostty_renderer` flag is flipped to `true` by default in production and soak
  completes, remove the 7 xterm addon dependencies from `package.json`.
- Remove `XtermTerminal.tsx`, `webglBudget.ts` (superseded), update `TerminalPool.tsx`
  to remove xterm-specific branching.

### Task 4.2 — Remove feature flag
- Remove `ghostty_renderer` from config.

### Task 4.3 — Update documentation
- Update `web-app/src/lib/terminal/README.md` to describe the ghostty-web architecture.
- Update `docs/reference/benchmarks.md` with new render-performance measurements.

## Task Ordering & Dependencies

```
Phase 0 (spike) ─────┬──→ Phase 2 (component)
Phase 1 (runbook) ───┘         │
                               ↓
                         Phase 3 (tests)
                               │
                               ↓
                         Phase 4 (cleanup) — after soak
```

Phases 0 and 1 can run concurrently. Phase 2 is blocked until Phase 0 spike gates
pass. Phase 4 is deferred until soak completes; it is out of scope for the initial PR.

## Adversarial Review Findings

**Issue 1 — ITerminal `buffer` coupling is deep**  
`TerminalStreamManager` reads `terminal.buffer.active.cursorY` and `.viewportY` to decide
whether to scroll to bottom. ghostty-web may not expose a structured `buffer` object
matching this shape. Plan addresses this in Task 2.2 by defining an adapter struct, but
the implementation must not assume ghostty-web's internals are a drop-in.  
*Mitigation*: In the spike (Task 0.1), explicitly test that `buffer.active.cursorY`
analogues are available. If not, refactor TerminalStreamManager to accept a
`getScrollState(): ScrollState` callback instead.

**Issue 2 — `prependScrollbackBatch` is the highest-risk path**  
`TerminalStreamManager.prependScrollbackBatch()` calls `serializeAddon.serialize()` to
capture the current visible buffer, then re-writes it. If the serialize shim (Task 2.4)
is incomplete, paged-scrollback sessions will corrupt their display on reconnect.  
*Mitigation*: Task 2.4 must be exercised by `TerminalStreamManager.resync.test.ts`
before shipping. Do not flag-flip in production until this test passes end-to-end.

**Issue 3 — TerminalPool host-element move under WASM**  
xterm.js keeps its WebGL context alive across `appendChild` moves because the canvas
element itself is not recreated. A WASM-backed renderer may re-initialize if the WASM
memory model is tied to a DOM tree position. The Phase 0 spike (Task 0.3) is mandatory
before any TerminalPool work.  
*Mitigation*: If move destroys state, an alternative is to keep the ghostty-web canvas in
a fixed off-screen div and use `CSS transform` to position it over the visible anchor —
more complex but avoids reinit.

**Issue 4 — Conformance runbook is not automatically regressed**  
A "runbook document" without a CI gate is easily ignored. Task 1.3 + Task 3.3 promote
it to a blocking `make ready` gate, but only after the ghostty-web path is proven. Until
then, the vectors still run in `make test` (non-blocking) to establish the baseline.

**Issue 5 — TerminalOutput.tsx is 2476 lines and tightly coupled to xterm.js internals**  
The conditional-render approach in Task 2.7 is the lowest-risk strategy: it changes only
one `import` path and one `<XtermTerminal>` → `<GhosttyTerminal>` render call. Any
attempt to refactor TerminalOutput.tsx itself in the same PR is out of scope.
