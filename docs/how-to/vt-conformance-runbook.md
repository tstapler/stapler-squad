# VT conformance runbook (xterm.js vs ghostty-web)

Validates the terminal renderer against 19 escape-sequence vectors, and walks the manual TUI checks the vectors cannot cover. The renderer is chosen by the `ghostty_renderer` feature flag (default off = xterm.js).

## Prerequisites

- `cd web-app && bun install` (installs `@xterm/xterm` and `ghostty-web@0.4.0`).
- `make build` once, so `web-app/src/gen` exists (Jest imports the generated protos).
- For manual steps: a running instance (`make install-service`, or the isolated manual instance in `CLAUDE.md`) and vim, htop and Claude Code installed.

## Automated vectors

Vectors live in `tests/terminal-conformance/`: `NAME.ans` (raw bytes fed to a 20x6 terminal), `NAME.expected.txt` (the correct screen per the VT spec, right-trimmed), optional `NAME.response.txt` (bytes the terminal must send back to the host), and `manifest.json`. Expected output is written from the spec, never copied from a renderer.

```bash
make ready-vt-conformance-gate   # blocking; also part of `make ready`
# or: cd web-app && bunx jest --no-coverage src/lib/terminal/__tests__/conformance
```

Both renderers run headless: xterm.js through `Terminal.write` (no DOM needed), ghostty-web through its WASM terminal (`Ghostty.load()`), the same parser the browser canvas drives.

`tests/terminal-conformance/baseline.json` lists vectors each renderer is known to fail. The suite fails if a listed vector starts passing (remove it) or an unlisted one fails (a regression), so the table below cannot rot.

| Vector | What it checks |
|---|---|
| sgr-colon-truecolor | `38:2::r:g:b` does not leak as text |
| cursor-movement | CUP/CUU/CUD/CUF/CUB |
| erase-line | EL 0 and 1 |
| rep-repeat-char | REP (`CSI b`) |
| decaln-alignment | DECALN (`ESC # 8`) |
| sync-output-2026 | Mode 2026 bracketed output still renders |
| sync-output-decrqm | DECRQM `?2026$p` replies `?2026;2$y` |
| kitty-keyboard-query | `CSI ? u` replies `CSI ? 0 u` |
| wide-chars | CJK and emoji take two cells |
| combining-mark | Combining accent shares a cell |
| scroll-region | DECSTBM with LF and RI |
| alt-screen | Mode 1049 swap and restore |
| tab-stops | Default 8-column tabs |
| insert-delete | ICH, DCH, IL |
| osc-title-hyperlink | OSC 0/2/8 print nothing |
| autowrap | DECAWM on and off |
| cursor-position-report | DSR 6 reply |
| backspace-cr | BS and CR |
| full-reset | RIS |

### Adding a vector

Add `NAME.ans` + `NAME.expected.txt` (+ `NAME.response.txt`), append to `manifest.json`, run the gate, and add it to `baseline.json` for any renderer that fails it.

## Manual steps

Run each with the flag off, then on (Settings > Feature flags > `ghostty_renderer`, then reload the page), and record the result in the table below.

1. **vim**: `vim /etc/hosts`; `:set mouse=a`; click to move the cursor; `ggVG` selection; `:q`. Check no stale cells after leaving, the cursor shape changes between normal and insert mode, and the mouse works.
2. **htop**: run `htop`; resize the browser window; sort with F6; quit. Check bars, colours and the function-key bar redraw cleanly after a resize.
3. **Claude Code TUI**: start a session, run a long response, scroll up, open a dialog, press Esc. Check no flicker during streaming and no leftover rows after the prompt redraws.
4. **Keep-alive**: with two panes, switch A to B and back. Content must be intact with no blank flash.
5. **Rollback**: turn the flag off, reload; the page must render with xterm.js (`data-renderer="ghostty"` absent from the terminal container).

## Before/after comparison

Automated results, measured 2026-10-29 with `xterm.js 6.0.0` and `ghostty-web 0.4.0`:

| Vector | xterm.js | ghostty-web |
|---|---|---|
| 17 vectors (all except the two below) | pass | pass |
| kitty-keyboard-query | FAIL | FAIL |
| sync-output-decrqm | pass | FAIL |

Reading: on these vectors ghostty-web does **not** improve on xterm.js 6. xterm.js 6 already passes REP, DECALN and mode 2026, so those are not gaps today. ghostty-web does not answer DECRQM for mode 2026 or the Kitty keyboard query (its WASM build emitted no reply), which makes it worse for TUIs that probe for 2026 support. Any benefit has to come from the manual steps (rendering fidelity, grapheme handling) and is **not yet measured**.

Manual results (not run: this was an unattended agent session with no interactive terminal or browser, so no row below was measured; run the steps above and replace "not run"):

| Check | xterm.js | ghostty-web |
|---|---|---|
| vim | not run | not run |
| htop | not run | not run |
| Claude Code TUI | not run | not run |
| Keep-alive pane switch | not run | not run |

## Known ghostty-web limitations in this integration

- No touch gestures, WebGL slot budget or resize sampler (see `GhosttyTerminal.tsx`).
- DECSTR (`ESC [ ! p`, the first bytes of every server snapshot) is logged as unimplemented by the WASM; the following `2J`/`H` still clear the screen.
- Search and serialize are shims (`ghosttySearch.ts`, `ghosttyAdapter.ts`); serialize restores text and SGR only, not cursor or modes.
