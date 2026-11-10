# Research: Features / state of the art for mobile soft-keyboard input in web terminals

Date: 2026-11-06. Method: web search + one page fetch (xterm.js #3600). Labels: VERIFIED = a fetched page or search-result snippet states it (URL given); INFERRED = my synthesis. Search snippets were summarized by the search tool, not read in full, so "VERIFIED" here means "reported by the cited source", not "I ran it".

## 1. What others do

### xterm.js (closest relative of our problem)
- Issue #3600 "Erratic text output ... Chrome on Android": open since 2022-01, labels area/mobile, help wanted, bug; no fix linked; maintainer called it very low priority. Symptoms: duplicated/jumbled chars, deletes ignored, also Firefox Android, MeshCentral, Cockpit. VERIFIED: https://github.com/xtermjs/xterm.js/issues/3600
- Root cause per an April 2026 comment: `CompositionHelper.ts` finalizes a composition synchronously when a non-composition keydown arrives mid-composition, then lets the keydown through; Gboard's finalized composition often contains the same character, so it is sent twice. The `_dataAlreadySent` dedupe depends on keydowns Gboard often does not supply (synthetic keyCode 229, no key/code). VERIFIED (same URL).
- Suggested (unimplemented) design: treat composition events as hints and reconcile against the textarea's final state; only pass through keys that cannot be part of a composition (arrows, Tab). Warned it affects desktop IME and needs Gboard, SwiftKey and CJK testing. VERIFIED (same URL). This is essentially the design we are planning.
- PR #4007 (merged) partly fixed single-char Backspace by emitting DEL when the textarea shrinks; composition duplication remains. VERIFIED (same URL).
- Workarounds seen: custom JS virtual keyboard dispatching key events to the helper textarea; running in an iframe; PR #3578 proposed a programmatic sendKey API. Downstream projects since late 2025 add "mobile composer bars, IME bypass layers, Enter debouncing" (not upstream). VERIFIED (same URL).
- Related: onData fired only after Enter pressed twice on Android in a React Native WebView (xterm 5.5.0, #5108); cause unconfirmed. https://www.goodfirstissue.org/pt-BR/xtermjs/xterm.js/issues/5108
- Codeman PR #65 "prevent double character input on Android Shift+key" (merged 2026-04-18): record keydown timestamps on the xterm textarea, skip `input` events within 50 ms of a handled keydown; still catches keyCode 229 characters xterm misses; listener gated by `isTouchDevice()`. VERIFIED: https://github.com/Ark0N/Codeman/pull/65 . Weakness (INFERRED): a time window heuristic, depends on xterm internals, no handling of rewrites.

### ghostty-web
- Search found no upstream issue about Android IME (gap). Our own finding (requirements.md): v0.4.0 ignores keyCode 229 and preventDefaults every beforeinput. Treat as UNVERIFIED upstream; file a follow-up issue.

### ttyd / wetty / GoTTY / hterm / Chrome Secure Shell / Blink Shell
- GoTTY had no mobile input; the fix was moving to xterm.js (ttyd uses xterm.js, so it inherits #3600). https://github.com/yudai/gotty/issues/134
- wetty 0.2.0 (hterm) had poor mobile support; xterm.js-based wetty was the alternative. https://support.glitch.com/t/solution-to-mobile-console-issues/4206
- hterm: IME input historically did not work (crbug 218382, 2016 thread, possibly stale). https://groups.google.com/a/chromium.org/g/chromium-hterm/c/YUQNQOvCvFk
- Blink Shell (iOS) uses hterm plus Mosh; no source found describing its IME path. Gap. https://github.com/blinksh/blink
- Net (INFERRED): no web terminal found has a robust Android IME story; users switch keyboards or use external-keyboard workarounds.

### Termux (native Android, a different layer but instructive)
- Uses a char-based mode (`enforce-char-based-input`, visible-password input type) to avoid autocorrect/composition; default off assuming IMEs honor TYPE_NULL like Gboard. Gboard also quirks on focus loss (mode not restored) and language switches (termux-x11 uses visible-password except for CJK IMEs). VERIFIED: https://github.com/termux/termux-app/issues/1539 , https://github.com/termux/termux-app/issues/2503 , https://github.com/termux/termux-x11/commit/da4fb8436ac0c871e31c7bfb772f43be9b485aaf
- Lesson (INFERRED): the web analogue is `autocomplete=off autocorrect=off autocapitalize=none spellcheck=false` plus `inputmode`/password-like hints; this suppresses the suggestion strip, which kills autocomplete the owner wants, so policy must be switchable (compose mode).
- xterm.dart: Gboard autocorrect left typed text in the composition buffer until Enter; turning autocorrect off fixed it. https://www.github.com/TerminalStudio/xterm.dart/issues/176

### code-server / VS Code
- Android on-screen Backspace is a known upstream problem; workaround `"keyboard.dispatch": "keyCode"`; Bluetooth keyboard recommended. https://github.com/coder/code-server/issues/7024 . No direct VS Code web terminal Android source found (gap).

### Monaco / EditContext
- Monaco uses a hidden textarea to reach OS text services; EditContext lets the view update and report composition location without cancelling composition. https://w3c.github.io/editing/docs/EditContext/explainer.html
- Support: web-features explorer lists Chrome Android 121; caniuse (via search snippet) lists Chrome Android only from 156; Safari/Firefox unsupported. CONFLICT, UNVERIFIED for Android. https://web-platform-dx.github.io/web-features-explorer/features/edit-context , https://caniuse.com/mdn-api_editcontext . No evidence found that Monaco/VS Code ships it on Android. Action for Phase 3: test on the owner's device before relying on it; treat as progressive enhancement behind feature detection.

### CodeMirror 6
- Discards what the keyboard did on Backspace and runs its own command; inspects DOM change after `deleteContentBackward` to detect word deletion (maintainer comment). https://github.com/codemirror/codemirror.next/issues/682
- 6.25.0: Gboard Enter that strips a trailing space at line end is recognized as Enter. https://github.com/codemirror/view/releases/tag/6.25.0
- Forum: quick backspacing of a word's first letter can leave a composition considered active, blocking Enter (non-Gboard keyboards, Chrome bug filed). https://discuss.codemirror.net/t/autocomplete-gets-stuck-and-permanently-breaks-enter-key-when-quickly-backspacing-on-android/9514
- Approach (INFERRED from the above): do not trust event semantics; observe the DOM mutation, then map to a command, with special-case recognizers for known keyboard patterns (Enter-as-deletion). This "DOM diff + recognizers" is the proven model.

### ProseMirror
- Dedicated Android input plugin (~450 lines) plus composition-state plugin; Gboard is the main target; avoids DOM changes near the cursor during composition because the browser resets composition; "cursor wrapper" placeholder. Recent fixes: 1.39.3 Enter in code blocks with spell-correction selected on Gboard; 1.42.5 deletions on Chrome Android misread as Enter. https://discuss.prosemirror.net/t/contenteditable-on-android-is-the-absolute-worst/3810 , https://app.unpkg.com/prosemirror-view@1.42.8/files/CHANGELOG.md
- Lesson: never mutate the hidden input's content/selection while a composition is live; the hidden element is the IME's state and resetting it breaks the keyboard.

### Lexical, Slate, JupyterLab
- Searches returned nothing usable (Lexical, JupyterLab) or only an unrelated Slate issue (slate-angular #194, duplicate text with composition). Gap; not blocking. Prior art above is sufficient.

### Gboard event-contract evidence
- Gboard historically sent no composition events and non-IME beforeinput types (2017 W3C testing); Enter mid-text gave `insertParagraph`, Backspace `deleteContentBackward`; keyboards wrap Backspace in composition so editors must wait for composition end and may need to undo, hence DOM diffing. 2017 data, may be stale. https://lists.w3.org/Archives/Public/public-editing-tf/2017Jun/0007.html , https://lists.w3.org/Archives/Public/public-editing-tf/2017Nov/0001.html , https://lists.w3.org/Archives/Public/public-editing-tf/2017Jun/0004.html
- On virtual keyboards all keydowns are keyCode 229 / key "Unidentified"; Enter is the exception and may be preventDefault-able. https://lists.w3.org/Archives/Public/public-editing-tf/2017Jun/0007.html , https://forum.inductiveautomation.com/t/virtual-keyboard-event-key-unidentified/42463

### Mosh (for local echo)
- Client predicts screen effect of keystrokes, shows only when confident; predictions grouped in epochs, speculative echo starts after the first few predictions in an epoch are confirmed; unconfirmed predictions underlined on high-latency links; recovers from wrong predictions within one RTT; avoids echoing passwords. https://mailman.mit.edu/pipermail/mosh-devel/2012-June/000206.html , https://github.com/mobile-shell/mosh

## 2. Edge cases and failure modes the design must handle

Event-level (each needs a recorded-corpus scenario):
1. Enter/Backspace wrapped in composition: keydown 229 then compositionupdate/end; emit exactly one CR / DEL, never also the finalized composition text (xterm #3600 duplication cause).
2. Duplicated text: composition end repeats the last char already sent from a keydown/insertText; Codeman Shift+key "AA". Make the hidden input's value the single source of truth and emit only the diff versus last-sent text; do not use timing windows.
3. Autocorrect/autocomplete replacing a word after space: arrives as `insertReplacementText` or a composition/value rewrite of earlier text including prior space; must translate to N backspaces + new text, bounded to the region we sent since last commit (INFERRED rule).
4. Swipe typing: inserts a whole word, often with a leading/trailing space and sometimes in a composition; may later be replaced by a suggestion pick.
5. Spacebar-swipe cursor movement: Gboard moves the hidden input's selection with no insertion; must map to nothing (or optional Left/Right arrows) and never cause text re-emit. Selection changes must not be interpreted as edits.
6. Emoji/CJK: surrogate pairs and multi-char compositions; do not split on UTF-16 units when counting backspaces; CJK keeps composition open across many updates and must emit only on commit (or show preview). Termux-x11 special-cases CJK IMEs.
7. Voice dictation: large insertText bursts, then replacement rewrites of recent words; same diff path, chunk like paste.
8. Clipboard suggestion chip: insertText or insertFromPaste of arbitrary length; decide bracketed-paste vs raw (see 11).
9. Enter that deletes a trailing space (CodeMirror 6.25.0): a value shrink right before newline.
10. Stuck composition (CodeMirror forum): add a watchdog that force-ends composition on blur, timeout, or a next unambiguous event.
11. Bracketed paste: only wrap when app enabled DECSET 2004 and event is a real paste; autocomplete/dictation inserts must not be wrapped (they would appear as pastes in Claude Code); chunking via existing `PASTE_CHUNK_SIZE`.
12. Long-press accent popups: produce replacement of a char just typed (insertReplacementText or delete+insert); must replace, not append.
13. Keyboard open/close/resize: visualViewport resize, focus loss when touch handlers preventDefault, keyboard hides on blur from toolbar tap (toolbar buttons must not steal focus: use pointerdown preventDefault); scroll-to-cursor and pane refit; Termux #2503 shows focus-loss mode bugs.
14. Hardware keyboard attached to phone: real keydowns with proper key/code; must be handled via the normal key path and not double-sent via input events; detect per event (key !== "Unidentified" and keyCode != 229), not per device.
15. Sticky Ctrl/Alt/Shift toolbar modifiers: apply to the next committed character from beforeinput/input (e.g., Ctrl+C from soft keyboard "c"), clear after one use, with a lock state; must work when the letter arrives as a composition.
16. Autocorrect suppression conflicts: disabling suggestions kills autocomplete the owner wants; keep a switch and per-keyboard defaults.
17. Selection/caret drift: the hidden input accumulates text; periodically reset (when no composition live) with a sentinel scheme, never mid-composition (ProseMirror lesson).
18. Disconnected stream: input must be queued or visibly rejected, not console.warn-dropped (already in requirements).
19. Terminal app with own cursor/wrap model: a rewrite sends backspaces that the TUI may interpret differently (Claude Code input box, vim normal mode); safe path is only rewriting text we sent in this composition region and only when the app is at a prompt; otherwise fall back to commit-on-compose-end.
20. Mouse-report filter (`isMouseReport`) and touch gesture handlers must not intercept IME-originated data.
21. Multiple terminals: attach only to the focused one.
22. Password/secret entry: never log content; Mosh-style predictions must not echo when the app disabled echo.

## 3. Unstated user needs (INFERRED)
- Typing must feel instant over a flaky mobile link (Tailscale/cellular): latency matters more than on LAN.
- Ability to compose a long prompt (Claude Code prompts) with autocorrect and dictation, review, then send, without every keystroke hitting the pane. Phone users drive agents with multi-sentence prompts.
- Undo of a bad autocorrect before it reaches the agent.
- Visible state: connected / dropped / queued; which input mode is active.
- Sticky modifiers and common chords (Ctrl+C, Ctrl+R, Esc, Tab, arrows) one tap away without the keyboard closing.
- Works the same with Gboard, Samsung Keyboard, SwiftKey, Chrome iOS (not regress).
- A fast way to report "my keyboard misbehaves" with a replayable capture and no content leakage.

## 4. Ranked "beyond state of the art" capabilities

Ranking: value to owner x confidence x differentiation.

1. DOM-diff reconciler core with composition-region bounding and known-quirk recognizers (CodeMirror model + xterm #3600 proposal), renderer-agnostic. Table stakes but nobody ships it for terminals. Must be first.
2. Recorded-event corpus and replay harness: capture real beforeinput/composition/keydown/input/selection sequences from the owner's devices, strip content to a shape-preserving form, replay in Jest and Playwright against both renderers. No terminal project has this; it is the regression insurance against renderer swaps.
3. Compose bar / input preview (a visible, ordinary text field mode with send-on-Enter or Send button, autocorrect/dictation/swipe fully native, supports multi-line, undo, history): sidesteps the terminal-cursor mismatch (edge 19) entirely and matches the owner's real use (prompts to agents). Downstream xterm projects already add "mobile composer bars" (VERIFIED in #3600), so this is validated demand.
4. Diagnostics overlay and server/client counters: live panel showing the last N event shapes (no content), composition state, anomalies, bytes sent/acked; plus the beacon for unrecognized sequences. Turns "typing is dead" into a one-minute diagnosis (matches requirements).
5. Per-keyboard quirk profiles: detect Gboard / Samsung / SwiftKey / unknown via observed event signatures (not UA), select a recognizer set and attribute policy; auto-fallback to compose mode when an unrecognized anomaly rate exceeds a threshold. Remote-updatable profile data via the feature-flag path.
6. Mosh-style local echo / predictive display for the typed-but-unacked text: show pending text (underlined) in an overlay so the phone feels instant over high latency and the user can see input arrived before the pane repaints; confirm/rollback on server output. Higher risk with TUIs; limit to a prompt-like overlay at the cursor and only when echo is on. Phase 3 should decide scope (INFERRED risk: wrong predictions in Claude Code's box).
7. Optimistic send queue with offline buffering and visible indicator; replay on reconnect with ordering guarantees (replaces silent drop).
8. EditContext progressive enhancement on Chrome Android, feature-detected, behind a sub-flag; only after device verification (support data conflicts, see section 1). Low priority until verified.
9. Sticky/lock modifier integration with composition (chords from soft-keyboard letters), plus gesture-based cursor movement mapped to arrows (spacebar swipe -> Left/Right) as an opt-in.
10. Self-test mode: an in-app typing test page that checks the keyboard against the recognizer set and uploads a content-free shape report, so new devices get profiled without the owner's phone.

## 5. Gaps
- No sources found for ghostty-web upstream IME issues, Lexical, JupyterLab, Slate (only unrelated), VS Code terminal on Android, Blink Shell input path, or current (2026) Gboard/Samsung event sequences. The 2017 W3C data may be stale; the real device corpus (owner action) supersedes it.
- EditContext Android support is contradictory between sources; verify on device.
