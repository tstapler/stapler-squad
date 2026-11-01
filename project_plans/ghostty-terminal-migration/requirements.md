# Requirements: ghostty-terminal-migration

**Date**: 2026-10-29
**Type**: feature (renderer replacement)

## Problem Statement

stapler-squad currently renders terminal output using xterm.js (`@xterm/xterm ^6.0.0`)
with seven addons (canvas, fit, search, serialize, unicode11, web-links, webgl). xterm.js
has known gaps in VT/escape-sequence conformance — particularly around modern sequences
(Kitty keyboard protocol, SGR mouse, OSC 8 hyperlinks, sixel graphics, Synchronized Output
DEC private mode 2026) — that cause AI agent output to render incorrectly or be silently
dropped. The `new-renderer` plan documented one instance of this class of bug.

`ghostty-web` (https://github.com/coder/ghostty-web) is a WebAssembly port of the
Ghostty terminal emulator's VT engine, authored by Coder. Ghostty is one of the most
conformant terminal emulators available (passes vttest, supports Kitty protocol, SGR
mouse, sixel, etc.). Migrating to ghostty-web is expected to eliminate the category of
rendering bugs caused by xterm.js's conformance gaps.

The second deliverable is a **VT conformance runbook**: a reproducible, documented test
procedure (escape-sequence test vectors + visual screenshots) that can be run before and
after the migration to prove the new renderer improves compatibility and can be re-run
on any future renderer upgrade.

## Users / Consumers

- **End users** watching AI agents' terminal output in the browser — correctness and
  visual fidelity of rendered text, cursor, and alternate-screen programs (vim, less,
  htop, Claude Code's own TUI) directly affect their ability to monitor agent progress.
- **AI agents** (Claude Code, Aider) whose output includes modern escape sequences that
  xterm.js partially mishandles; better conformance means fewer display artifacts during
  autonomous sessions.
- **`XtermTerminal.tsx`** — the React component being replaced.
- **`TerminalStreamManager.ts`** — already uses an `ITerminal` interface; it is the
  primary consumer of the renderer's `write()` / `clear()` / `refresh()` API.
- **`TerminalPool.tsx`** — manages up to N live renderer instances for pane-switch
  keep-alive; the docking mechanism (`appendChild` of host element) must work with the
  replacement.

## Success Metrics

1. VT conformance runbook passes with ≥ the same coverage on ghostty-web as on xterm.js,
   with the previously-failing sequences now passing.
2. `XtermTerminal.tsx` is replaced (or wrapped behind a feature flag) by a
   `GhosttyTerminal` component that satisfies the same `XtermTerminalHandle` public
   interface.
3. `TerminalStreamManager`'s `ITerminal` contract is met by the ghostty-web adapter with
   no changes to the stream manager's internal logic.
4. All existing `XtermTerminal` unit tests are replaced with equivalent `GhosttyTerminal`
   tests that pass.
5. `make ci` passes (lint, unit tests, build).
6. No regression in the `TerminalPool` keep-alive / pane-switch behaviour.

## Constraints

- **No changes to `TerminalStreamManager.ts` internals**: The stream manager's
  `ITerminal` interface may be extended if ghostty-web needs additional methods, but
  existing flow-control / watermark / chunk logic must be unchanged.
- **Feature-flag rollout**: The migration must be guarded by a feature flag
  (`ghostty_renderer` or similar) so the live service can run xterm.js while ghostty-web
  is validated. Both renderers must coexist in one build.
- **Bun-only build**: All JS dependency changes go through `bun add`; no npm/yarn/pnpm.
- **ghostty-web ships WASM**: The WASM binary must be bundled correctly in both the
  Next.js and Bun build paths (`WEB_BUILD=bun`). WASM loading strategy must be confirmed
  before implementation begins.
- **`XtermTerminalHandle` surface must be preserved**: TerminalOutput.tsx calls into
  `XtermTerminalHandle`'s `terminal`, `serializeAddon`, `write`, `writeln`, `clear`,
  `focus`, `fit`, `resize`, `refit`, `search`, `searchNext`, and scroll methods.
  ghostty-web must expose equivalents for each, or they must be shimmed.
- **Conformance runbook must be reproducible**: Not a one-off manual check. It must
  produce artifacts (screenshots or text diffs) that can be committed and diffed in CI
  or re-run by any developer.

## Scope

### In Scope

1. **Spike ghostty-web**: Prototype a minimal `GhosttyTerminal` React component that
   initialises a ghostty-web instance, passes ANSI data to it, and mounts into the DOM.
2. **Adapter layer**: Implement `GhosttyTerminal.tsx` satisfying `XtermTerminalHandle`.
   Shim any missing capabilities (fit, serialize, search).
3. **ITerminal extension**: Extend `TerminalStreamManager`'s `ITerminal` interface only
   if ghostty-web's API requires it; add no stream-manager logic.
4. **TerminalPool compatibility**: Verify and if necessary adjust the pool's host-element
   docking mechanism to work with ghostty-web's DOM shape.
5. **Feature flag**: Add `ghostty_renderer` flag to the feature-flags system; route
   `XtermTerminal` vs `GhosttyTerminal` through it.
6. **VT conformance runbook**: Write `docs/how-to/vt-conformance-runbook.md` covering:
   - OSC 8 hyperlinks
   - SGR mouse (1006)
   - Kitty keyboard protocol
   - Synchronized Output (DEC PM 2026)
   - Alternate-screen enter/exit (DECSC/DECRC, smcup/rmcup)
   - 256-colour and true-colour (24-bit) SGR
   - Unicode (emoji, CJK, combining characters)
   - vttest sections 1–4 (cursor movement, screen features, attributes)
7. **Conformance test fixture**: A set of ANSI escape-sequence test vectors (stored in
   `tests/terminal-conformance/`) that can be fed to either renderer and compared.
8. **Migration gate**: CI step or `make ready` gate that runs the conformance vectors
   against the active renderer.

### Out of Scope

- Replacing the PTY or Go-side transport; this is purely a client-side renderer swap.
- Sixel graphics support (nice-to-have; log an issue if ghostty-web supports it).
- Removing xterm.js from the build until the feature flag is flipped globally.
- Any changes to `TerminalOutput.tsx`'s stream connection or flow-control logic.

## Open Questions

1. **ghostty-web API surface**: Does `@coder/ghostty-web` expose a `write(data:
   Uint8Array)` method and a resize API compatible with `ITerminal`? What does its
   React component model look like?
2. **WASM loading**: Does ghostty-web ship its WASM inline (base64) or as a separate
   asset? How does the Bun build handle WASM imports today?
3. **Serialize equivalent**: `SerializeAddon.serialize()` is used by
   `TerminalStreamManager.prependScrollbackBatch`. Does ghostty-web expose a buffer
   serialization API, or must we shim it with an in-memory shadow buffer?
4. **FitAddon equivalent**: xterm.js uses `FitAddon.fit()` to measure container pixels
   → cols/rows. Does ghostty-web have an equivalent, or must `cellDimensions.ts` be
   ported?
5. **Search**: Does ghostty-web support in-buffer text search natively, or is a shim
   needed?
6. **TerminalPool docking**: The pool does `anchor.appendChild(entry.hostEl)` to move
   the rendered terminal between DOM locations without destroying the WebGL context.
   Does ghostty-web's canvas/WASM instance survive that same move?
7. **Bundle size**: What is the WASM binary size, and what impact does it have on initial
   page load (LCP)?
