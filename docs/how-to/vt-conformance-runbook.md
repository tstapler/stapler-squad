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

Automated results, measured 2026-10-30 with `xterm.js 6.0.0` and `ghostty-web 0.4.0` (19 vectors):

| Vector | xterm.js | ghostty-web |
|---|---|---|
| 18 vectors (all except `kitty-keyboard-query`) | pass | pass |
| sync-output-decrqm | pass | pass (reply synthesized by `ghosttyQueryResponder.ts`) |
| kitty-keyboard-query | FAIL | pass (reply `CSI ? 0 u` synthesized by `ghosttyQueryResponder.ts`) |

Reading: ghostty-web's WASM parses mode 2026 and the Kitty query but emits no reply, so the adapter answers both itself (`ghosttyQueryResponder.ts`; Kitty flags are reported as 0 because the renderer never enables them). With that shim ghostty-web passes every vector; xterm.js 6 still fails the Kitty query. On the other vectors (REP, DECALN, mode 2026 rendering) xterm.js 6 already passed, so the automated suite shows no further improvement; any remaining benefit has to come from rendering fidelity and grapheme handling, which the manual steps below are meant to measure.

Manual / browser results, measured 2026-10-30 against a separate instance (`PORT=62871`, `--tmux-keep-server`) built with `WEB_MINIFY=1`, Playwright Chromium, cache disabled:

| Check | xterm.js | ghostty-web |
|---|---|---|
| Cold-load LCP on `/`, minified build, 5 runs (ms), 2026-10-30 | 340, 320, 252, 276, 268 (median 276) | 260, 240, 248, 236, 244 (median 244) |
| Cold-load LCP on `/`, unminified build, 5 runs (ms), 2026-11-01 (`measure-lcp.mjs`) | 492, 472, 476, 472, 468 (median 472) | 472, 476, 476, 472, 464 (median 472) |
| Keep-alive pane switch A→B→A | not run | PASS: same canvas element survives, 0 console errors, 0 `ghostty`/`.wasm` requests during the switch, canvas non-blank (3247 lit px before, 2584 after; the drop is the cursor/typed line, not a blank pane) |
| vim (2026-11-01, headless Chromium, `vim -u NONE`: open, `:set number`, `G`, insert text, Esc, viewport resize, `:q!`) | line numbers, inserted text and `~` filler rows render; after the resize the top row is the line start (`45` gutter visible) | line numbers, inserted text and `~` filler rows render. After the resize the top row is a wrapped continuation row (cursor mid-line, no gutter visible), where xterm.js shows the line start. Not diagnosed: could be vim choosing a different row count or a ghostty-web resize difference. `:q!` was sent but the vim-exit screen was not inspected |
| htop (2026-11-01: open, resize to 1300x800, F6 sort menu, Esc, `q`) | meters, colours, process table and the F6 menu with its `Enter Sort / Esc Cancel` bar render (F6 screenshot) | same screenshot content: meters, colours, process table, F6 menu and bar render; the resize redraws without garbage; after `q` the shell prompt returns with no leftover htop rows |
| Claude Code TUI, startup only (2026-11-01: `claude` in a fresh dir, trust dialog, resize, Esc, Ctrl-C twice) | dialog renders; after Ctrl-C the dialog rows stay above the prompt; "Security guide" is underlined (OSC 8 link) | dialog renders; after Ctrl-C the same rows stay above the prompt (identical behaviour); "Security guide" is not underlined, so OSC 8 links are not rendered (plan task 2.6, deferred) |
| Claude Code TUI, long streamed response, scroll-up, in-session dialog | not run | not run (needs an authenticated model call; still needs a person) |

Re-measured 2026-11-01 with `web-app/scripts/measure-lcp.mjs` (unminified `make build` output, isolated instance on `PORT=62873`, cache disabled, flag seeded via localStorage with `GetFeatureFlags` blocked): xterm.js 492, 472, 476, 472, 468 (median 472 ms); ghostty-web 472, 476, 476, 472, 464 (median 472 ms); delta 0 ms against the 200 ms budget. Reproduce with `cd web-app && CHROMIUM_PATH=<chrome-headless-shell> node scripts/measure-lcp.mjs http://localhost:62873 5`.

LCP is measured on `/`, where the pool preloads the chosen renderer; the ghostty-web chunk is only requested when the flag is on. The two LCP rows use different builds, methods and days, so compare within a row only: the delta (ghostty on minus off) is -32 ms in the first and 0 ms in the second. Both are inside the 200 ms budget and indicate no regression; the difference between them is within run-to-run noise. The vim/htop/Claude Code rows were run on a separate instance (`PORT=62877`, private tmux socket) by driving the session with `tmux send-keys` and capturing Playwright screenshots; they are visual observations of screenshots, not pixel diffs. Only the screenshots named above were inspected; a streamed Claude Code response still needs a person at a terminal. Console errors were identical in both renderers (blocked `GetFeatureFlags`, 404s, backlog feature disabled).

## Known ghostty-web limitations in this integration

- No touch gestures, WebGL slot budget or resize sampler (see `GhosttyTerminal.tsx`).
- DECSTR (`ESC [ ! p`, the first bytes of every server snapshot) is logged as unimplemented by the WASM; the following `2J`/`H` still clear the screen.
- Search and serialize are shims (`ghosttySearch.ts`, `ghosttyAdapter.ts`); serialize restores text and SGR only, not cursor or modes.
