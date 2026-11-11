# Spike results: Stories 0.1.1 and 0.1.2

Run 2026-11-08 in worktree `ssq-wt-mobile-input-spike`. Labels: VERIFIED = command run or source opened in this worktree; INFERRED = reasoning, not observed.
Installed: ghostty-web 0.4.0, @xterm/xterm 6.0.0, @playwright/test 1.57.0 (Chromium 143, Pixel 7 emulation).

## 0.1.1 ghostty-web citations (VERIFIED, `web-app/node_modules/ghostty-web/dist/ghostty-web.js`)

| Claim | Result |
|---|---|
| keyCode 229 early return | CONFIRMED at line 851, in `InputHandler.handleKeyDown`: `if (this.isDisposed \|\| this.isComposing \|\| A.isComposing \|\| A.keyCode === 229) return;` |
| Blanket `beforeinput` preventDefault | CONFIRMED at line 2330, in `Terminal.open(A)`: `A.setAttribute("contenteditable", "true"), A.addEventListener("beforeinput", (E) => E.preventDefault())`. `A` is the element passed to `open()`. |
| Own hidden textarea | CONFIRMED at line 2332: `document.createElement("textarea")`, autocorrect/autocapitalize/spellcheck off, 1x1px, opacity 0, `clipPath: inset(50%)`, appended to the open() element; `this.focus()` focuses it (line 2373). |

Listener set (cite by condition):
- On the open() element (`InputHandler`, line 819): `keydown` (handler above), `paste`, `compositionstart`, `compositionupdate`, `compositionend`. Plus `beforeinput` (preventDefault) from `open()`. Also `mousedown` (capture), `mousemove`, `mouseleave`, `click`, `wheel` (non-passive, capture).
- `compositionstart` sets `isComposing=true`; `compositionend` sends `event.data` through `onData` if non-empty and removes stray text nodes from the container (line 992-1004). INFERRED from source (not run): a composition whose events bubble to the container emits text on `compositionend`; the 229 early return only drops the `keydown`.
- On the canvas: `mousedown` and `touchend`, both `preventDefault()` then focus the textarea (lines 2334-2337).
- On ghostty's own textarea: only `paste` (preventDefault + stopPropagation, calls `this.paste(text)`), and a one-shot `blur` during context-menu handling (line 1969). There is NO `keydown`, `beforeinput`, `input` or `composition*` listener on the textarea itself; those events bubble to the open() element where the listeners above live. VERIFIED by `grep -n addEventListener`.
- Decision feed, Task 1.4.1c: the layer must stop/capture events at the textarea (capture phase on the textarea or a wrapper) before they bubble to the container, or the blanket `beforeinput` preventDefault and the 229 drop stay in the path. The textarea is not an isolated input surface; its parent is the dead-end.

`navigator.userAgentData.platform`: on the Pixel 7 emulated page over `http://localhost` it returned `"Android"` (VERIFIED, probe output). On an `about:blank`/`setContent` page (insecure context) it was `null`, so the platform gate must tolerate `null` and fall back to the UA string (INFERRED for plain-http LAN origins: UA-CH is secure-context-only; the app is reached over https on :8444 per memory, but `http://localhost:8543` counts as secure).

## 0.1.1 DECSET 2004 access (VERIFIED, ran in headless Chromium: both terminals fed `\x1b[?2004h` then `\x1b[?2004l`)

| Renderer | readable | API | Observed (before / after h / after l) |
|---|---|---|---|
| xterm.js 6.0.0 (`XtermTerminal.tsx`) | yes | `terminal.modes.bracketedPasteMode` (typings `xterm.d.ts:1919`) | false / true / false |
| ghostty-web 0.4.0 (`GhosttyTerminal.tsx`) | yes | `term.getMode(2004, false)` on the public `Terminal` (dist line 2815) or `term.wasmTerm.getMode(2004, false)` / `wasmTerm.hasBracketedPaste()` | false / true / false (all three forms agree) |

- Neither accessor is read anywhere in `web-app/src` today (grep of `bracketedPaste|getMode(|2004` finds only a test-generator escape table). `toITerminal` in `ghosttyAdapter.ts` does not expose the mode, so Story 1.1.3 / 1.2.x must add a narrow `bracketedPasteEnabled(): boolean` to the renderer-facing contract: xterm reads `modes.bracketedPasteMode`, ghostty reads `term.getMode(2004, false)`; false when `wasmTerm` is not yet open (`getMode` calls `assertOpen()` and throws otherwise, VERIFIED at line 2816, so wrap in a closed-state guard).
- ghostty-web's own `Terminal.paste()` (line 2407) is the renderer's built-in bracketed-paste path; the layer's `insertFromPaste` can route through it or wrap itself using the same flag. Not exercised here.
- Feeds: Task 1.1.3 (paste policy, per-renderer `readable: yes`, so the "joined" fallback branch of Story 4.2.1 is only for closed/unready terminals), Task 4.2.1b.
- Probe: bundle of xterm + ghostty-web (bun build) loaded into Playwright Chromium; scripts live in the scratchpad (not committed).

## 0.1.2 CDP IME probe (VERIFIED, Playwright Chromium, `devices['Pixel 7']`, textarea on http://localhost)

Event order (`value` is the textarea value at event time):

| Call | Events |
|---|---|
| `Input.imeSetComposition {text:"hel"}` | `compositionstart`(data "") -> `compositionupdate`("hel") -> `beforeinput insertCompositionText`("hel", isComposing true) -> `input insertCompositionText` (value "hel") |
| `imeSetComposition {text:"hello"}` | `compositionupdate` -> `beforeinput insertCompositionText` -> `input` (value "hello"); no new compositionstart |
| `Input.insertText {text:"hello"}` while composing | `compositionupdate` -> `beforeinput insertCompositionText` -> `input` -> `compositionend`("hello") (a commit, value unchanged "hello") |
| `Input.insertText {text:"x"}` with no composition | `beforeinput insertText`("x", isComposing false) -> `input insertText` |
| `imeSetComposition` with `replacementStart/End` 4..7 over "abc def" | `compositionstart`(data "def") -> `compositionupdate`("DEF") -> `beforeinput insertCompositionText` -> `input` (value "abc DEF"); `insertText` with the same range commits it. |

- No `keydown`/`keyup` events at all around composition or insertText. Real Android Gboard emits `keydown` keyCode 229 per key; CDP does not, so the layer's 229 handling is NOT exercised by these calls.
- `insertReplacementText` cannot be produced by any CDP call tried. The helper provides `syntheticReplacementText` (untrusted, dispatched via `dispatchEvent`, edits nothing); specs using it must state so (matches the validation row SM-2 fallback).
- Decision feed, Task 5.1.1a: `imeCompose`/`imeCommit` give real `compositionstart/update/end` and `insertCompositionText`; keep them. The helper in `tests/e2e/helpers/ime.ts` already implements the real path with synthetic only for replacement text, so the plan's fallback branch (insertText + synthetic beforeinput) is only needed for `insertReplacementText`.

## 0.1.2b Backspace on the textarea after reset to `""` (VERIFIED for CDP; real keyboards UNKNOWN until Task 0.1.3b)

CDP `Input.dispatchKeyEvent` rawKeyDown/keyUp, Backspace:

| Case | Events |
|---|---|
| keyCode 8, textarea `""` | `keydown`(8) -> `beforeinput deleteContentBackward` -> `keyup`. No `input` (nothing to delete). |
| keyCode 8, textarea `"x"` (sentinel), caret at end | `keydown`(8) -> `beforeinput deleteContentBackward` -> `input deleteContentBackward` (value ""). -> `keyup` |
| keyCode 229, textarea `""` or `"x"` | `keydown`(229) -> `keyup`. No `beforeinput`, no `input`, nothing deleted (value stays "x"). |
| Playwright `keyboard.press('Backspace')`, `""` | same as keyCode 8 empty case |

- Finding: desktop-engine Chromium fires `beforeinput deleteContentBackward` even on an EMPTY textarea when the key is keyCode 8. So CDP cannot reproduce the failure the pre-mortem worries about (a real Android keyboard sending only key events, or nothing, on empty). The only shape CDP can model is "keydown 229, no beforeinput", which the `keycode-229` helper variant emits.
- Decision feed, Task 1.1.2a (INFERRED, pending device Task 0.1.3b): do not rely on `beforeinput deleteContentBackward` on an empty textarea. Reducer must accept BOTH shapes and de-dup them: `beforeinput deleteContentBackward` OR (`keydown` Backspace with keyCode 8 or 229 and `key === "Backspace"`) and emit one `\x7f`, never two when both arrive (CDP keyCode 8 delivers both on an empty field, so the de-dup path IS exercisable now). A sentinel character makes the browser produce `input deleteContentBackward` reliably (confirmed above) but then requires restoring the sentinel after each delete and cannot be used if Gboard clears composing regions around it; the keydown fallback is needed either way. Recommended: keydown-Backspace fallback with de-dup, sentinel left as an option if the device result shows no key event at all.

## CDP is not Gboard

Everything above is Chromium's own event generation from DevTools commands. It confirms event names, ordering and that composed text lands in a textarea; it does not model Gboard's keydown-229 stream, autocorrect replacement ranges (`insertReplacementText`), suggestion-strip rewrites, swipe, or Backspace shape on an empty field. Every e2e spec built on `helpers/ime.ts` must say in its header which events are real CDP events and which are synthetic, and device Task 0.1.3b / the pre-merge phone gate (5.1.4c) remains the arbiter.

## Files

- `tests/e2e/helpers/ime.ts` (`imeCompose`, `imeCommit`, `imeReplaceRange`, `imeBackspace`, `syntheticReplacementText`)
- `tests/e2e/ime-helper.spec.ts` (2 tests, run locally with a scratch playwright config: 2 passed in 742 ms; self-contained, no server needed)
