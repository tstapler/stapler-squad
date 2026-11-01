# Research: ghostty-web Migration

**Date**: 2026-10-29

## Source Library: coder/ghostty-web

**Repo**: https://github.com/coder/ghostty-web  
**Authored by**: Coder (the cloud development environment company)  
**Core**: WebAssembly compilation of Ghostty's VT engine (written in Zig)

Ghostty is a high-conformance terminal emulator that passes vttest, supports the Kitty
keyboard protocol, SGR 1006 mouse, OSC 8 hyperlinks, Synchronized Output (DEC PM 2026),
and sixel graphics. `ghostty-web` exposes this engine as a Web Component / WASM module
suitable for embedding in browser-based terminals.

## Current Renderer Architecture

### Dependency graph

```
TerminalOutput.tsx (2476 lines)
  └─ XtermTerminal.tsx          ← React wrapper for xterm.js Terminal
       ├─ Terminal               (@xterm/xterm ^6.0.0)
       ├─ FitAddon               (@xterm/addon-fit ^0.11.0)
       ├─ WebLinksAddon          (@xterm/addon-web-links ^0.12.0)
       ├─ WebglAddon             (@xterm/addon-webgl ^0.19.0)  ← WebGL budget-gated
       ├─ CanvasAddon            (@xterm/addon-canvas ^0.7.0)  ← fallback renderer
       ├─ SearchAddon            (@xterm/addon-search ^0.16.0)
       ├─ SerializeAddon         (@xterm/addon-serialize ^0.14.0)
       └─ Unicode11Addon         (@xterm/addon-unicode11 ^0.9.0)

TerminalStreamManager.ts
  └─ ITerminal (interface over Terminal)
       ├─ write(data: string | Uint8Array, cb?): void
       ├─ clear(): void
       ├─ refresh(start, end): void
       ├─ scrollToBottom(): void
       └─ buffer.active / buffer.normal (length, cursorY, viewportY)

TerminalPool.tsx
  └─ host <div> per entry, moved via appendChild (no WebGL context loss)

lib/terminal/ support modules:
  cellDimensions.ts, cursorRect.ts, postFitRepaint.ts, webglBudget.ts,
  scrollRouting.ts, scrollOverride.ts, scrollKinematics.ts, gestureMachine.ts,
  touchDrag.ts, mouseTracking.ts, modifierKeys.ts, EscapeSequenceParser.ts,
  TerminalStreamManager.ts, RedrawThrottler.ts, TerminalDimensionCache.ts,
  handshakeDims.ts, bufferFilter.ts, preSizing.ts, viewportSettle.ts
```

### XtermTerminalHandle (public API consumed by TerminalOutput.tsx)

```typescript
interface XtermTerminalHandle {
  terminal: Terminal | null;           // raw xterm.js instance (for resize events)
  serializeAddon: SerializeAddon | null; // scrollback serialization
  write(data: string): void;
  writeln(data: string): void;
  clear(): void;
  focus(): void;
  fit(): void;                         // trigger FitAddon.fit()
  resize(cols: number, rows: number): void;  // bypass-fit direct grid resize
  refit(opts?: RefitOptions): void;    // start sampler + force repaint
  search(term: string): boolean;
  searchNext(term: string): boolean;
  // + scroll/selection/URL methods
}
```

## Known xterm.js Conformance Gaps

| Sequence | xterm.js status | Ghostty status |
|---|---|---|
| Kitty keyboard protocol | Partial (legacy fallback) | Full |
| SGR 1006 mouse | Supported | Supported |
| OSC 8 hyperlinks | Supported (via addon) | Native |
| Synchronized Output (DEC PM 2026) | No | Yes |
| Sixel graphics | No | Yes |
| `DECSET 2048` — Unicode grapheme clustering | No | Yes |
| `DECSTR` soft terminal reset | Partial | Full |
| REP (CSI b) — repeat previous character | No | Yes |
| DECALN — screen alignment test | No | Yes |

The most impact for AI agent sessions: Synchronized Output prevents partial-render
flicker during Claude Code's complex TUI updates; better Kitty protocol support ensures
modifier keys (Ctrl+C, Ctrl+D, arrow keys) are not ambiguous on modern keyboards; OSC 8
hyperlinks allow file references to be clickable.

## WASM Integration Considerations

ghostty-web ships a WASM binary (compiled from Zig). Key questions for the build:

1. **Bun WASM import**: Bun supports WASM via `import wasmUrl from './foo.wasm?url'` or
   `new URL('./foo.wasm', import.meta.url)`. The ghostty-web package will need to either
   ship with an inlined WASM (base64) or as a separate `.wasm` asset URL.
2. **Next.js**: `next.config.js` may need `config.experiments.asyncWebAssembly = true`
   or the package may handle this via a lazy `fetch()`.
3. **Bundle size concern**: Ghostty's VT engine + renderer in WASM may be 1–5 MB. This
   must be lazy-loaded (not part of the initial bundle) to avoid LCP regression.

## Adapter Mapping

| xterm.js / addon | ghostty-web equivalent | Notes |
|---|---|---|
| `terminal.write(data)` | `terminal.input(data)` (TBD — verify API) | Core data path |
| `FitAddon.fit()` | `terminal.resize(cols, rows)` + pixel measurement | Must port `cellDimensions.ts` logic |
| `WebglAddon` / `CanvasAddon` | Built-in (Ghostty renderer) | `webglBudget.ts` becomes irrelevant |
| `SearchAddon.findNext()` | TBD | May need in-JS shim over ghost's buffer |
| `SerializeAddon.serialize()` | TBD | Scrollback serialization for `prependScrollbackBatch` |
| `Unicode11Addon` | Built-in (Ghostty has full Unicode support) | No addon needed |
| `WebLinksAddon` | OSC 8 native + click handler | Potentially simpler |
| `terminal.buffer.active.length` | TBD | Used for scroll depth calcs |

## Support Modules Impact

Most `lib/terminal/` support modules are either:
- **Renderer-agnostic** (TerminalStreamManager, RedrawThrottler, EscapeSequenceParser,
  CircularBuffer, scrollKinematics, messageQueue) → keep unchanged.
- **xterm.js-specific** (webglBudget, postFitRepaint's WebGL-slot logic, CanvasAddon
  fallback) → need replacement or removal.
- **Geometry helpers** (cellDimensions, cursorRect, handshakeDims, TerminalDimensionCache)
  → must port to ghostty-web's cell metric API.
- **Input / gesture** (gestureMachine, touchDrag, modifierKeys, mouseTracking) → likely
  portable if ghostty-web exposes an `onData` callback for input injection.

## Conformance Runbook Design

The runbook (`docs/how-to/vt-conformance-runbook.md`) must cover:

### Automated test vectors (`tests/terminal-conformance/`)

Each vector is a `.ans` file containing a sequence to feed to the renderer, plus a
`.expected.txt` or `.expected.png` snapshot. CI runs them against both renderers and
diffs output.

| Test vector | File | What it validates |
|---|---|---|
| 256-colour SGR | `256color.ans` | CSI 38;5;N m |
| True-colour SGR | `truecolor.ans` | CSI 38;2;R;G;B m |
| OSC 8 hyperlink | `osc8.ans` | OSC 8 ;; URL ST |
| Synchronized Output | `sync-output.ans` | DEC PM 2026h / 2026l |
| Kitty keyboard | `kitty-kbd.ans` | CSI ? u |
| Alternate screen | `altscreen.ans` | DECSET 1049h / 1049l |
| Unicode graphemes | `unicode.ans` | Emoji, CJK, combining |
| vttest cursor | `vttest-1.ans` | vttest §1 cursor movement |
| vttest screen | `vttest-2.ans` | vttest §2 screen features |
| vttest attributes | `vttest-3.ans` | vttest §3 character attributes |
| DECSTR reset | `decstr.ans` | CSI ! p |
| REP repeat | `rep.ans` | CSI N b |

### Manual test steps (documented in runbook)

1. Open a session running `vim` — verify cursor movement, status line, normal/insert mode.
2. Open a session running `htop` — verify colour bar charts, mouse click selects process.
3. Open Claude Code session — verify TUI panels, progress indicators, flicker-free
   scrolling.
4. Test Ctrl+C / Ctrl+D / arrow keys — verify they are correctly received by the agent.

## Risk Analysis

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| ghostty-web API incompatible with ITerminal | Medium | High | Spike first; create thin adapter |
| SerializeAddon has no ghostty equivalent | Medium | High | Shadow buffer shim in TerminalStreamManager |
| WASM too large for initial load | Medium | Medium | Lazy-load + skeleton placeholder |
| TerminalPool appendChild breaks WASM context | Low | High | Spike with proof-of-concept before full implementation |
| Search regression (no native search) | Medium | Medium | In-JS search shim over rendered text |
| TerminalOutput.tsx 2476-line coupling | High | Medium | Feature flag gates; xterm fallback stays in build |
| cellDimensions.ts hard-codes xterm metrics | Medium | Medium | Port to ghostty-web's cell-size API |

## Estimated LOC Impact

| Component | Change |
|---|---|
| XtermTerminal.tsx (~1500 lines) | Replace with GhosttyTerminal.tsx |
| XtermTerminal.css.ts | Replace |
| webglBudget.ts | Delete (ghost has no WebGL slot budget) |
| postFitRepaint.ts | Significant rework (no WebGL addon) |
| cellDimensions.ts | Port to ghostty API |
| TerminalStreamManager.ts | Minimal (ITerminal may need 1-2 new methods) |
| TerminalPool.tsx | Moderate (host element docking verification) |
| TerminalOutput.tsx | Minimal (ref type change at XtermTerminalHandle callsites) |
| lib/terminal/__tests__ | Rewrite ~34 test files |

Total estimated new/changed code: ~3000–4000 lines plus ~1500 lines removed (xterm wrapper).
