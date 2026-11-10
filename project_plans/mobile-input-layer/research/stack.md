# Stack research: renderer-agnostic Android soft-keyboard input layer

Date: 2026-11-06. Labels: VERIFIED = source opened or command run this session; INFERRED = reasoning or recollection, not checked against a primary source this session.

## 1. Repo versions (VERIFIED, `web-app/package.json`, `web-app/bun.lock`, `npm view`)

| Item | Repo | Latest on npm |
|---|---|---|
| @xterm/xterm | ^6.0.0, locked 6.0.0 (bun.lock:1153) | latest 6.0.0; `beta` 6.1.0-beta.336 (2026-10-29) |
| ghostty-web | 0.4.0 pinned (bun.lock:1703) | latest 0.4.0; `next` 0.4.0-next.21.g640db46 (2026-10-28) |
| bun | packageManager bun@1.4.3 | |
| jest / jsdom env | ^30.2.0 / jest-environment-jsdom ^30.2.0 | |
| @playwright/test | ^1.57.0 (web-app), ^1.40.0 (tests/e2e) | |
| react / typescript | ^19.0.0 / ^5.9.3 | |

Implication: jsdom does not implement real IME, `getTargetRanges`, or `EditContext`. The Jest replay suite must synthesize `InputEvent`/`CompositionEvent`/`KeyboardEvent` sequences by hand (INFERRED; jsdom's InputEvent exists but has no IME engine).

## 2. Browser APIs

### beforeinput / Input Events (VERIFIED: https://w3c.github.io/input-events/)
- Cancelable: `insertText`, `insertReplacementText` (spellcheck/autocorrect/writing suggestions), `deleteContentBackward`, `insertLineBreak`, `insertFromPaste` (must be preceded by a `paste` event).
- NOT cancelable: `insertCompositionText` (the beforeinput/input pair following each `compositionupdate`). Only events inside an IME composition are non-cancelable; every other beforeinput is cancelable.
- `getTargetRanges()`: for `<textarea>`/`<input>` it returns an empty array; ranges are only meaningful on contenteditable hosts. So a textarea-based layer cannot learn the replaced range from the event; it must diff against its own mirrored value/selection (`selectionStart/End`) instead. (spec text above; the design consequence is INFERRED.)
- Consequence: `preventDefault()` on beforeinput (what ghostty-web 0.4.0 does) is harmless for plain `insertText` on desktop but, during composition, cannot stop the DOM mutation; the textarea value still changes. A terminal layer should treat the hidden element's value as scratch state and reset it after commit.

### keyCode 229 and composition on Android (partly VERIFIED, old sources)
- Android Chrome keydown during IME input carries keyCode 229 (Windows "IME process key"); 2017 W3C thread: Android had no composition events at the time and keydown reported 229; Chromium tracked crbug.com/737349. https://lists.w3.org/Archives/Public/public-editing-tf/2017Jun/0006.html and https://lists.w3.org/Archives/Public/public-editing-tf/2017Jun/0007.html (VERIFIED as thread content; current state UNVERIFIED).
- Chromium 2018 (Chrome 65): `setComposingRegion()` fires compositionstart if no composition range is open and compositionupdate every time. https://lists.w3.org/Archives/Public/public-editing-tf/2018Feb/0000.html (VERIFIED as a post; INFERRED still current).
- 2026 downstream report (Codeman PR 65): symbols from Gboard's symbol keyboard (e.g. "/") produce keyCode 229 + `input` event WITHOUT compositionstart/compositionend wrapping. https://github.com/Ark0N/Codeman/pull/65.diff (VERIFIED as search snippet; not independently reproduced). This is the "orphaned input" case; the layer cannot rely on composition events being present, and must not drop 229 keydowns without a beforeinput/input path behind them.
- Design rule (INFERRED from the above): never derive bytes from keydown when `keyCode===229` or `isComposing`; derive from `beforeinput`/`input` diffs, and handle hardware keys (keyCode != 229) from keydown. Dedupe the same text arriving via keydown and beforeinput (ghostty next.21 does this, see 4).

### Hidden textarea vs contenteditable vs EditContext
- Hidden `<textarea>` (xterm.js and ghostty-web's own approach): beforeinput targets empty ranges, simple value diffing, well-trodden. Risk: Gboard suggestion strip behavior depends on the surrounding text in the field; an always-empty field gives the IME no context, which is mostly beneficial (fewer rewrites) but disables swipe-continuation spacing heuristics (INFERRED).
- contenteditable: gives `getTargetRanges`, but more IME quirks and ghostty-web already makes its container contenteditable (`dist/ghostty-web.js:2330` in 0.4.0).
- EditContext: https://web-platform-dx.github.io/web-features-explorer/features/edit-context says Chrome and Chrome Android 121 (2024-01-23), Firefox and Safari not supported, status "Limited availability" (VERIFIED, via search summary; caniuse https://caniuse.com/mdn-api_editcontext lists Chrome 121+, Edge 121+, Samsung Internet 25+ per search summary, and its Chrome-for-Android row is inconsistent, listing only version 156). MDN marks it Experimental and not Baseline: https://developer.mozilla.org/en-US/docs/Web/API/EditContext_API (VERIFIED). Events: `textupdate`, `textformatupdate`, `characterboundsupdate`, `compositionstart/end` (MDN page names TextUpdateEvent, TextFormatUpdateEvent, CharacterBoundsUpdateEvent). Recommendation: progressive enhancement only, behind feature detection (`'EditContext' in window`); iOS and Firefox Android still need the textarea path, so the textarea path is the primary one. Whether Gboard on Chrome Android actually behaves better with EditContext is UNVERIFIED and must be tested on-device.

### Virtual keyboard sizing (VERIFIED: https://developer.mozilla.org/en-US/docs/Web/API/VirtualKeyboard_API)
- `navigator.virtualKeyboard.overlaysContent = true` stops the viewport resize; `geometrychange` + `boundingRect`; CSS `env(keyboard-inset-height)`; `virtualkeyboardpolicy="manual"` on contenteditable lets you call `show()`/`hide()`. Experimental, secure context only, feature-detect. Chromium-only (INFERRED; MDN table not captured).
- `visualViewport` (resize/scroll events) is the cross-browser fallback for keeping the terminal and toolbar above the keyboard (INFERRED, standard API; not fetched this session).
- Important for the focus requirement: the soft keyboard only opens if `focus()` happens inside a user activation on the hidden input; `preventDefault()` on touchstart/mousedown can suppress it (matches the requirements' rabbit hole; INFERRED).

### Attributes (INFERRED from HTML spec/MDN knowledge; not fetched this session, confirm on-device)
- `autocapitalize="off"`, `autocorrect="off"`, `spellcheck="false"`, `autocomplete="off"`, `enterkeyhint="send"` (or `enter`), `inputmode="text"` (or `none` when using a custom toolbar). `autocorrect` is WebKit-origin and Chrome Android honors `spellcheck`/`autocapitalize`; Gboard may still show suggestions regardless (hence handle `insertReplacementText`). Observed in ghostty-web 0.4.0 source: it sets autocorrect/autocapitalize/spellcheck on its textarea (`dist/ghostty-web.js:2332`, VERIFIED local), and the report still says typing fails, so attributes alone are not the fix. `type=password`-style tricks to disable suggestions are a known hack (INFERRED); avoid.

## 3. Local code facts (VERIFIED, installed `node_modules`)

ghostty-web 0.4.0 (`web-app/node_modules/ghostty-web/dist/ghostty-web.js`):
- :851 `handleKeyDown` returns early on `isComposing || A.isComposing || keyCode === 229`.
- :2330 container is made `contenteditable` and `beforeinput` is blanket `preventDefault()`ed; textarea created at :2332 with autocorrect/autocapitalize/spellcheck off.
- :807-997 composition listeners only toggle `isComposing`; composition text is not emitted as data (grep shows no `insertCompositionText` handling).
- Net effect confirms the requirements' diagnosis: with the 0.4.0 input path, Android IME text yields no `onData`.

@xterm/xterm 6.0.0 (`src/browser/input/CompositionHelper.ts:96,110`): keyCode 229 handling is a composition-position heuristic (`setTimeout` read of textarea value diffs), not beforeinput-driven.

## 4. Did newer releases fix IME? 

- xterm.js #3600 ("Erratic text output ... Chrome on Android"): still OPEN, labels help wanted / type/bug / area/mobile; maintainer Tyriar (Jan 2022) "very low on my priority list"; no linked fix PR. PR #4007 (merged) fixed only single-char backspace via DEL when textarea shrinks. Many downstream projects (11 listed, Dec 2025 to Oct 2026) report working around it. https://github.com/xtermjs/xterm.js/issues/3600 (VERIFIED via fetch summary).
- xterm.js PR #5024 (merged 2024-04-08, milestone 6.0.0): fixes duplicate input for some (Chinese) IMEs by using the new composition start as the previous composition's end. Included in 6.0.0 (per PR page milestone; INFERRED that it ships in the installed 6.0.0). Not Gboard-targeted. https://github.com/xtermjs/xterm.js/pull/5024 (VERIFIED summary).
- xterm 6.1.0-beta.336 (2026-10-29, VERIFIED by `npm pack` + read): `CoreBrowserTerminal._keyDown` has a new `_keyDownSeen` gate fix for macOS IME shift-commit (#5887, #6144), and CompositionHelper gained ~18 lines. Still keyCode-229/composition-position based; no Android/Gboard-specific change found. Not a fix for #3600; it is a beta, so not recommended to adopt.
- ghostty-web 0.4.0 (GitHub release, "09 Dec"): IME for Chinese/Japanese (PR #90), iOS support (PR #76), DECCKM (PR #81). https://github.com/coder/ghostty-web/releases (VERIFIED summary).
- ghostty-web `0.4.0-next.21.g640db46` (2026-10-28, VERIFIED by `npm pack` + read): adds `handleBeforeInput` (mobile/IME input) that handles `insertText`, `insertReplacementText`, `insertLineBreak`/`insertParagraph` (as CR), `deleteContentBackward`, `deleteContentForward`, `insertFromPaste`, with dedupe against recent keydown/composition-end/paste data. It returns early when `isComposing`/`A.isComposing`, so text delivered as `insertCompositionText` during an active composition (typical Gboard word composing) is still not emitted until compositionend, and `deleteContentBackward` maps to an empty string at this point (final mapping not traced). So the next build likely fixes Gboard-without-composition cases but is a prerelease with unknown rewrite correctness. Recommendation: do not depend on it; our own layer should own correctness (consistent with requirements) and can be validated against it as a second opinion.

## 5. Test tooling notes
- Jest 30 + jsdom: synthetic event replay harness; hand-built corpus (recorded JSON from real devices when available). INFERRED.
- Playwright: Android emulation (`devices['Pixel 7']`, hasTouch/isMobile) emulates viewport/touch but NOT Gboard. Real IME-like sequences can be injected via Chromium CDP `Input.imeSetComposition` / `Input.insertText` (INFERRED from CDP docs recollection; verify at https://chromedevtools.github.io/devtools-protocol/tot/Input/). Therefore e2e proves integration/wiring, while the recorded-event corpus proves IME semantics; real-device acceptance remains the owner's step.
- Conventions: e2e specs need `// @feature` header, `data-testid`/ARIA locators, no `waitForTimeout` (repo CLAUDE.md).

## 6. Open items for later phases
1. Confirm on a real phone whether `ghostty_renderer` off restores plain typing (unverified).
2. Record Gboard (suggestion strip on/off, swipe, symbol keyboard) and Samsung Keyboard event streams to build the corpus.
3. Decide whether a `beforeinput`-primary hidden textarea mirrors a short rolling value (to give the IME context) or stays empty.
4. Evaluate EditContext only as an opt-in enhancement after the textarea path is stable.
