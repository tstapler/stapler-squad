# UX Design: mobile-input-layer

Inputs: `requirements.md`, `research/ux.md`, plan.md Phase 3 (Stories 3.1.1-3.1.4) and Phase 4 (4.1.1-4.2.1). Evidence labels as in research: items marked (R) trace to `research/ux.md`, (P) to `plan.md`.

## 0. Conventions that apply to every surface

- **Touch targets**: every tappable control is >= 44x44 CSS px including padding (`--min-touch-target: 44px`, mobile-ux-improvements ADR). Adjacent targets are separated by >= 8px.
- **Theme**: vanilla-extract only (`vars` from `web-app/src/styles/theme-contract.css.ts`): `vars.color.textPrimary/textSecondary`, `cardBackground`, `modalBackground`, `borderColor`, `warning`, `error`, `success`, `primary`, `onPrimaryFill`. No hex literals, no per-surface dark-mode media queries; the theme contract already switches light/dark. Text contrast >= 4.5:1, non-text (icons, borders, focus ring) >= 3:1. Status is icon + text, never color alone.
- **Soft keyboard**: new surfaces are `flex-shrink: 0` children of the existing sticky-flex shell in `TerminalOutput.tsx` (height from `--viewport-height`, written by `ViewportProvider`; `project_plans/mobile-ux-improvements/decisions/ADR-003-sticky-flex-layout-for-toolbar-keyboard-avoidance.md`), never `position: fixed`, so when the IME opens the shell shrinks and the chip, compose bar and toolbar stay above the keyboard. That ADR was written for iOS Safari's fixed-position-vs-layout-viewport bug and makes no Android claim: Android Chrome's behavior (`resizes-content` vs `resizes-visual` vs `overlays-content`, and `interactive-widget`) and Samsung Keyboard split/floating/one-handed modes are UNVERIFIED and are measured by plan Story 0.1.5 before the layout rule is considered proven. The bottom sheet is capped at `min(40vh, 40 * visualViewport.height / 100)` so it never exceeds the visible area when the keyboard is up. No surface may obscure the terminal cursor row (WCAG 2.4.11); cursor-row-visible has an emulation check (UX-33) as well as the real-keyboard check (UX-1).
- **Chrome stack budget and minimum terminal rows**: the stack is toolbar, compose bar (when on), chip. Guarantee (plan Story 1.4.3): the terminal keeps at least 5 full rows in `comfortable` density (for example 360x340 with all three surfaces) and never fewer than 3 rows (landscape 740x170, short phone 360x240). When it would fall below 5 rows the layout switches to `compact`, in priority order: the chip collapses into a 44x44 badge in the toolbar row that expands as an overlay (not in flow); the compose bar becomes one 44px line with a scrollable draft; if 3 rows are still not available the compose bar auto-collapses (draft kept) and hints queue. The toolbar is never hidden. At 360px width the chip text wraps to at most two lines and Reconnect and Dismiss (44px each, 8px apart) never overlap the compose bar.
- **Stacking order, bottom to top, above the keyboard**: extra-keys toolbar, compose bar (when on), status chip. At most one chip at a time; a newer message replaces the older one.
- **Screen reader (TalkBack)**: single hidden input has `role="textbox"`, `aria-multiline="true"`, `aria-label="Terminal input"`, `aria-describedby` = status-region id. Two persistent live regions per terminal mount (plan Story 1.4.4): a POLITE region (`role="status"`) shared by the chip states, read-only hint, flag-off hint, paste-flattened hint, IME-fallback notice, compose offer, DeferredCommit indicator, "Report copied" and the committed-text echo; and an ASSERTIVE region (`role="alert"`) used only when typed content was lost or may be incomplete (discard notices, the "Sending is stuck" state). Components write through one `announce(kind, text, dedupeKey)` and carry no `role` of their own, so a message is announced once per state change, a re-render with the same state is silent, and an assertive message is never also written to the polite region. TalkBack typed-character announcement: see S1. Never `aria-hidden`, never `display:none`.
- **Flag off** (`mobile_input_layer` false): none of the new layer surfaces (chip, compose bar, fallback notice) render and the old behavior (console.warn on drop, existing read-only hint, existing Reconnect button after 5s) is unchanged (P Story 3.1.1). Two carve-outs, so "none render" means none of the layer's own interactive surfaces: (1) on the Android class a one-line dismissible informational `role="status"` hint in plain language ("Phone keyboard support is off, so typing may not work well. You can turn it on in the feature flags panel.") shows so flag off is never silent (plan Story 1.3.2); the copy contains no query parameter and no flag identifier (`?mobile_input=0|1` is documented only in `docs/how-to/debug-phone-typing.md`); (2) the diagnostics surfaces are separate (`?inputdebug=1` panel in PR-A1, full sheet behind `mobile_input_diagnostics`) and still open with the header `layer: off`.
- **PR-A has no drop notice**: the chip is PR-B. In PR-A, text typed while disconnected does not appear and the user sees only the existing disconnected state (stream status and the Reconnect button after 5 s, unchanged); the layer never deletes already-sent prompt text on reconnect or session switch (plan Story 1.2.4).
- **Desktop / hardware keyboard**: none of the new surfaces appear unless an error state occurs (chip) or the user asks (diagnostics, compose). Hidden input path is not armed on `pointer: fine` without touch (R 4.6). Desktop layout of chip/sheet is a left-aligned max-width 480px variant, same components.
- **Copy rules**: messages state what happened and what to do; never include typed content.

## Surface inventory

| # | Surface | Type | Phase |
|---|---|---|---|
| S1 | Hidden terminal input element + keyboard open/close behavior | Interactive (invisible) | 1 |
| S2 | Composition feedback (in-pane rewrite) | Interactive, no new chrome | 1 |
| S3 | Input status chip (disconnected / queued / discarded) | Interactive | 3 |
| S4 | Read-only hint | Interactive | 1/3 |
| S5 | Compose bar + toolbar toggle | Interactive | 4 |
| S6 | IME-fallback notice | Interactive | 4 |
| S7 | Diagnostics bottom sheet | Interactive | 4 |
| S8 | Renderer-fallback and flag-off state | Non-interactive (condensed) | 1 |
| S9 | Diagnostics report JSON + log/metric output | Non-interactive (condensed) | 4/2 |

---

## S1. Hidden terminal input element

Not visible; defines how the keyboard opens, closes, and is exposed to assistive tech.

```
 +--------------------------------+
 | terminal pane                  |
 |   $ ls_                        |   <- hidden 1px textarea sits at cursor row
 |                                |      (opacity 0, pointer-events none)
 +--------------------------------+
 | [Esc][Tab][Ctrl][Alt][^][v]... |   <- extra-keys toolbar, 44px keys
 +--------------------------------+
 |        soft keyboard           |
 +--------------------------------+
```

Flow:
1. User taps the terminal -> `controller.focus()` runs synchronously inside the tap (user gesture) -> keyboard opens; terminal resizes via `visualViewport`; cursor row remains visible.
2. User types -> reducer emits bytes (S2). Toolbar keys use `onPointerDown` + `preventDefault` so the keyboard does not close (existing).
3. Exit: system Back / keyboard-dismiss chevron closes the IME; tapping outside the terminal or switching terminals calls `blur`. Focus is not trapped; Tab/Shift-Tab with a hardware keyboard moves focus normally.
4. Hardware keyboard: first `keydown` with `keyCode != 229` and a physical `code` -> keydown handled by the renderer; the matching `beforeinput` for that physical key is suppressed structurally (a physical `keydown` with `keyCode != 229` is handled once and its `beforeinput` is ignored); there is no timing-window dedupe (ADR-002 rejects it as non-deterministic).

Error / edge states:

| Condition | User sees | Exit |
|---|---|---|
| Keyboard fails to open (focus blocked by touch `preventDefault`) | Nothing opens; after the 2nd tap with no `focus` event the S6 notice offers Compose mode | Compose toggle (S5) or tap again |
| Session read-only | Keyboard does not open (`inputmode="none"`), S4 hint | Switch to a writable session |
| Stream disconnected | Keyboard opens; S3 chip appears on first typed char | Reconnect action in chip |
| Flag off | Old renderer-native input | n/a |

TalkBack typed-character announcement (research/ux.md TalkBack note, INFERRED until the device check): the textarea value is only the in-flight composition and is reset at commit boundaries, so TalkBack's own text-change echo is unreliable for a write-only terminal. At each word commit the committed text is written once to the polite live region and cleared after 1 s (never logged, never in the diagnostics ring beyond class and length); Backspace runs and read-only sessions announce nothing; terminal output is not announced. Because the spoken echo includes text typed at a password prompt, this shares the ADR-008 secret-entry limitation. Owner proof: plan Task 5.1.4d (UX-31).

Acceptance: UX-1, UX-3, UX-5, UX-27, UX-31, UX-33 (consolidated list below).

## S2. Composition feedback (in-pane rewrite)

The plan's reducer rewrites live inside the composition region as one atomic edit (backspaces + text, a single send), bounded to 64 graphemes, with a deferred-commit mode above that (P Glossary: `MAX_REGION`, `DeferredCommit`). The research preview overlay (model B) is not in the plan; this design keeps **no extra preview chrome**: the user sees the pane update with each atomic edit. Decision recorded (ADR-002): live atomic rewrite stays; model B preview-then-commit is rejected, so S2 needs no change.

```
type "helo" -> pane: helo_
tap suggestion "hello" -> one atomic edit -> pane: hello_   (no intermediate blank)
long text (>64 graphemes) -> pane updates when the word commits (DeferredCommit)
```

Flow: type, autocomplete pick, autocorrect on space, swipe word, Backspace, Enter. Enter flushes any pending composition first and sends `\r` last. Emoji go as one write.

Measurable flicker criterion (plan Story 1.2.5, UX-32): a rewrite is one `onData` string sent as one `sendInput`, emitted within one animation frame of the `beforeinput`; sampled once per animation frame across an autocorrect, the pane shows at most ONE intermediate state, and never a blank frame or prompt text shorter than before the rewrite. If the owner's device shows flicker the harness does not catch, it is recorded and a preview/echo follow-up is filed; the harness bound stays as the regression guard.

DeferredCommit feedback (UX-32): while a commit longer than 64 graphemes or containing a newline (dictation, long swipe) is held, the pane shows nothing, so after 300 ms a one-line polite status "Typing... (N characters waiting)" shows above the toolbar and is removed when the commit lands or the stuck-composition watchdog fires; no content is shown and it respects reduced motion.

Error / edge states:

| Condition | User sees | Exit |
|---|---|---|
| Text is queued/dropped (disconnected) | S3 chip; pane shows nothing new for that text | Reconnect; retype if discarded |
| Layer loses sync (cursor moved by arrow key mid-composition) | Append only, no backspaces; no visible message | Anomaly counted in diagnostics |
| Terminal app changes cursor model (multi-line prompt) | Possible misplaced rewrite; no message | Backspace / Ctrl-U in toolbar; diagnostics shows anomaly |

## S3. Input status chip

Appears within 250 ms of the first dropped or queued input (P 3.1.2). Pinned directly above the toolbar/compose bar. `data-testid="terminal-input-dropped"`.

```
Disconnected, text queued:
+------------------------------------------------+
| (!) Disconnected - typing is not being sent.   |
|     Text is held for 30 s.        [ Reconnect ]|   Reconnect = 44px
+------------------------------------------------+

Queue flushed (auto-dismiss 4 s):
+------------------------------------------------+
| (ok) Reconnected - sent 12 typed characters.   |
+------------------------------------------------+

Discarded (stays until dismissed or next input):
+------------------------------------------------+
| (!) Discarded 10 chars typed while offline. [x]|   x = 44px
+------------------------------------------------+
```

Interaction flow:
1. First dropped/queued input -> chip "Disconnected - typing is not being sent." (contains "not being sent", P AC). Live region announces politely.
2. User taps **Reconnect** -> triggers the existing reconnect; label changes to "Reconnecting..." (disabled, `aria-busy`).
3. Reconnected, same session id, within 30 s -> queue flushes in order -> chip shows success for 4 s, then is removed.
4. Reconnected but different session id, TTL (30 s) expired, or overflow (> 4 KiB) -> chip becomes the Discarded message with dismiss (x). Overflow discards the whole queue: "Discarded N chars typed while offline".
5. Control keys (Esc, arrows, Ctrl combos) are never queued: they show the same Disconnected chip, text "Keys are not sent while disconnected" if only control input was attempted; nothing replays later.
6. Auto-reconnect succeeding without user action also runs steps 3/4.
7. Stuck and timeout states (plan Task 3.1.5b, UX-35): Reconnecting for more than 10 s shows "Still trying to reconnect. This is taking longer than usual." with Retry and Dismiss; a replay in flight shows "Sending N typed characters...", after 5 s "Still sending N typed characters. Keep this page open.", and after 15 s without completion "Sending is stuck. Some typed text may be missing; check the terminal." (assertive) with Reconnect and Dismiss. A completed flush always replaces these with the success state.

Error and edge states:

| State | Message | Action / exit |
|---|---|---|
| Disconnected | "Disconnected - typing is not being sent." | Reconnect; or ignore (chip clears when reconnected) |
| Reconnect fails | "Still disconnected. Check your connection." | Retry (same button); dismiss (x) hides until next input |
| Reconnecting longer than 10 s | "Still trying to reconnect. This is taking longer than usual." | Retry; dismiss (x) |
| Replay in flight | "Sending N typed characters..." (after 5 s: "Still sending N typed characters. Keep this page open.") | Wait; dismiss (x) |
| Replay stuck after 15 s | "Sending is stuck. Some typed text may be missing; check the terminal." | Reconnect; dismiss (x) |
| Queue overflow | "Discarded N chars typed while offline." | Dismiss (x); retype |
| Queue TTL expired | "Discarded N chars - offline for more than 30 s." | Dismiss (x) |
| Session changed on reconnect | "Discarded N chars - the session changed." | Dismiss (x) |
| Read-only | No chip; see S4 | n/a |
| Flag off | No chip; old console.warn | n/a |

Theme: `vars.color.warning` border/icon on `cardBackground`; success state uses `vars.color.success`; text `textPrimary`. Dark mode comes from the theme contract. Chip max-width 100% minus 16px gutters on phone; 480px on desktop. Reduced-motion: no slide animation, instant appearance.

TalkBack: chip states go to the shared POLITE region and discard/stuck messages to the ASSERTIVE region (section 0, plan Story 1.4.4), written once through `announce()`; the Reconnect button has `aria-label="Reconnect terminal"`; dismiss `aria-label="Dismiss message"`. In compact density (section 0) the chip is a 44x44 badge in the toolbar row that expands as an overlay; its message and actions are identical.

## S4. Read-only hint

Read-only sessions (`terminal_read_only`) must not open the keyboard and must say why (R UX-6).

```
tap on terminal (read-only):
+------------------------------------------------+
| (eye) This terminal is view-only.        [ x ] |
+------------------------------------------------+
```

Flow: tap -> hint (rate-limited to once per 10 s, existing `TerminalOutput.tsx:1068-1080`) -> auto-dismisses in 4 s or on (x). Extra-keys row and compose toggle are hidden. Nothing is queued; typing is impossible so no drop chip.

Errors/edges: session becomes writable while hint is up -> hint removed, keyboard opens on next tap. Session becomes read-only while keyboard is open -> keyboard closes (`inputmode="none"`, blur) and the hint appears once. Exit: always dismissible; navigate to another session.

Resolved: Task 1.4.1d adds `role="status"` and `aria-live="polite"` to the existing hint (plan Story 1.4.1 AC).

## S5. Compose bar and toggle

Opt-in native textarea for long prompts (P 4.2.1). Off by default; hidden when read-only.

```
Toolbar (toggle is the 44px "Compose" key):
[Esc][Tab][Ctrl][Alt][^][v][..][ Compose ]

Compose on:
+------------------------------------------------+
| +--------------------------------------------+ |
| | Message to terminal...                     | |   textarea, autocorrect ON,
| |                                            | |   2-5 rows, scrolls
| +--------------------------------------------+ |
| [ Close ]                 [ Send ][ Send+Enter]|   all 44px
+------------------------------------------------+
| [Esc][Tab][Ctrl]...                             |
| soft keyboard                                   |
```

Flow:
1. Tap **Compose** (aria-pressed toggles) -> bar opens, textarea focused (inside gesture), keyboard opens, terminal shrinks.
2. User types with full IME (autocorrect, swipe, dictation).
3. **Send** -> writes the whole string as one unit: bracketed paste (`\x1b[200~...\x1b[201~`) when mode 2004 is on, else typed text; embedded `\x1b[201~` is stripped. **Send+Enter** does the same then `\r`. Textarea clears, stays open and focused for the next message.
4. **Close** (or Compose toggle again, or hardware Escape) -> bar closes; draft kept in memory for the session and restored on reopen; focus returns to the terminal input.
5. Empty text: Send/Send+Enter are `aria-disabled`.

Error / edge states:

| State | User sees | Exit |
|---|---|---|
| Disconnected on Send | Text is queued through the same path; S3 chip shows; textarea keeps the text until queue accepts, then clears | Reconnect; Close |
| Queue overflow on Send (text > 4 KiB) | Text is NOT cleared; inline error "Too long to hold while offline (4096 chars max)." | Edit shorter, reconnect, or Close |
| Read-only session | Toggle hidden; if bar was open, it closes with the S4 hint | Switch session |
| Mode 2004 unknown (renderer cannot report) | Falls back to typed text; no message (spike 0.1.1 outcome) | n/a |
| IME unavailable | Compose is itself the fallback (S6) | n/a |
| Flag off | Toggle not rendered | n/a |

Theme: `vars.color.modalBackground`, `inputBorder`, `inputFocusBorder` (focus ring 3:1), `primary` for Send with `onPrimaryFill` text. Pointer fine/desktop: toggle available in toolbar overflow; Ctrl/Cmd+Enter sends; plain Enter inserts newline.

TalkBack: textarea `aria-label="Compose message"`; Send `aria-label="Send"`; Send+Enter `aria-label="Send and press Enter"`; toggle `aria-label="Compose mode"` with `aria-pressed`. After send, live region says "Sent".

## S6. IME-fallback notice

Trigger (either, once per session): (a) 5 `keydown 229` events with zero `beforeinput` in a focused session (P 4.2.1; "no text at all"); (b) 3 layer anomalies (`unrecognized_sequence`, `stuck_composition`, `orphaned_input_fallback`) within 60 s while `beforeinput` traffic is otherwise normal ("wrong or garbled text"), with the variant copy "Typing looks unreliable with this keyboard. Try compose mode?" (plan Task 4.2.2c, UX-34). The diagnostics sheet also carries a "Try compose mode" action and the Compose toggle stays in the toolbar overflow, so the offer is never the only path.

```
+------------------------------------------------+
| (i) Keyboard input fell back to compose mode.  |
|                         [ Got it ] [ Turn off ]|
+------------------------------------------------+
```

Flow: notice appears and the compose bar (S5) opens focused. **Got it** dismisses the notice (compose bar stays). **Turn off** closes compose and stops auto-offering for this device (localStorage, try/catch). Notice auto-dismisses after 8 s but a live-region announcement was already made.

Errors/edges: localStorage blocked -> works for the session only. If `beforeinput` later arrives normally, no change is forced; the user closes compose manually. Flag off or read-only: never shown.

## S7. Diagnostics bottom sheet

Opened by (1) an "Input diagnostics" entry in the app settings surface (primary, discoverable), (2) toolbar overflow "Input debug", (3) `?inputdebug=1`, and (4) as a secondary shortcut only, long-press (800 ms) on the connection chip. <= 2 taps from the terminal via (1) or (2) (UX-36). Closed by default; open state persisted in localStorage (try/catch). Android collision: a long press on the chip can raise Chrome's context menu / selection callout, so the chip sets `user-select: none` and `-webkit-touch-callout: none` and suppresses `contextmenu` on that element only; the long-press is never the sole path, and the how-to lists settings and overflow first.

```
+------------------------------------------------+  max 40vh, scrolls inside
| Input diagnostics                       [ Close]|  title, role="dialog"
| renderer: xterm (fallback: mobile_input_layer off)
| layer: on | stream: connected | IME: idle      |
| beforeinput: yes | keyboard: Gboard?           |
| events 42  bytes text 31 / esc 3  dropped 0    |
|  queued 0  anomalies 1                         |
| [Copy report] [Clear] [Pause] [ ] Reveal text  |  44px rows
|------------------------------------------------|
| t+ms  src         type            act   bytes  |
| +12   beforeinput insertText      sent  text(1)|
| +40   beforeinput insertReplace.. sent  bs(2)+text(4)|
| ...                                            |
+------------------------------------------------+
```

Flow:
1. Open -> capture starts (ring buffer 200, 50 shown, newest first). Closed -> push is a no-op.
2. **Copy report** copies redacted JSON (header + data class + length only) to clipboard; toast "Report copied". **Clear** empties the table and resets counters. **Pause** freezes capture (button becomes **Resume**). **Reveal text** switch (off at open, session-only) shows content locally; it never changes the copied JSON. The revealed text is held only in an in-memory buffer inside the sheet, cleared when reveal is switched off or the sheet closes, and is never part of the report data.
3. Exit: **Close** button, Back gesture, tapping the scrim, or hardware Escape. Focus returns to the element that opened it. Focus is trapped inside while open; terminal input still works behind the sheet only if the sheet is non-modal on desktop (desktop: non-modal side panel; phone: modal sheet).

Error / edge states:

| State | User sees | Exit |
|---|---|---|
| Clipboard API denied | Report shown in a selectable textarea with "Copy blocked - select and copy manually." | Close |
| No events yet | "No input events yet. Type in the terminal to see them." | Close |
| `beforeinput` unsupported | Header "beforeinput: no (compose mode recommended)" with Open Compose action | Compose / Close |
| Flag off | Header "layer: off"; ring buffer remains empty; header still shows renderer and reason | Close |
| localStorage blocked | Sheet works; open state not remembered | n/a |
| Keyboard open | Sheet height clamps to visual viewport; table scrolls | Close |

TalkBack: `role="dialog"`, `aria-modal="true"` on phone, `aria-labelledby` the title; table rows are `role="row"`; Copy/Clear/Pause have text labels; Reveal is `role="switch"`. Theme: `modalBackground`, `modalBorder`, `textPrimary`; monospace rows at >= 14px.

## S8. Renderer fallback and flag-off state (condensed, non-interactive)

Sample (diagnostics header only; no other UI is shown):
```
renderer: xterm (fallback: mobile_input_layer off)
```
Acceptance:
- On a touch device with `ghostty_renderer` on and `mobile_input_layer` off, the pane uses xterm.js with one console/log line; the only user-facing message is the Android-class hint below.
- Diagnostics header always states renderer and reason.
- Toggling the flag in the feature-flag panel takes effect on the second page load without rebuild (the first load persists the value); `?mobile_input=0|1` takes effect in the same load and persists per origin (plan Task 1.3.2e).
- On the Android class with the flag off, the one-line plain-language hint of section 0 is visible (no query parameter or flag identifier in its text; UX-28 carve-out).
- Desktop (no touch) rendering is unchanged by the gate.

## S9. Diagnostics report and operator output (condensed, non-interactive)

Sample:
```json
{"renderer":"xterm","layer":true,"stream":"connected","keyboard":"gboard?",
 "events":[{"t":12,"source":"beforeinput","inputType":"insertText","dataClass":"text","len":1,"composing":true,"action":"sent","bytesOut":"text(1)"}]}
```
Acceptance:
- Report contains no typed characters, regardless of the Reveal switch.
- The report is content-free and cannot become a replayable trace; `reportToSkeleton(report)` yields a shape summary for triage only. Replayable fixtures come from the separate capture-for-fixture artifact: `captureToTrace(capture)` produces a trace the corpus accepts except `expectedBytes`, `scrubbed` and `signoff` (plan Stories 4.1.1, 4.1.3, ADR-007, ADR-008).
- Server counters/log lines carry sizes and classes only.
- Report header includes renderer, flag state, stream state.

---

## Flow exit-path matrix

| Flow | Exit path | Error states covered |
|---|---|---|
| Type with keyboard | IME dismiss / blur | disconnected (S3), read-only (S4), flag off, IME unavailable (S6) |
| Disconnected typing | Reconnect, dismiss, auto-clear | overflow, TTL, session change, reconnect failure |
| Read-only tap | Dismiss / switch session | n/a |
| Compose | Close, Send | disconnected, overflow, read-only, 2004 unknown |
| IME fallback | Got it / Turn off | storage blocked |
| Diagnostics | Close / Back / scrim / Esc | clipboard denied, empty, unsupported, flag off |

## UX acceptance criteria

Typing and keyboard
1. UX-1: On a phone, tapping the terminal opens the soft keyboard in one tap and the cursor row stays visible above the toolbar (verify with Gboard and Samsung Keyboard).
2. UX-2: Autocomplete pick, autocorrect, swipe word and Backspace produce the same final text in the pane as a hardware keyboard (human check against the same sentence).
3. UX-3: Enter, Esc, Tab, arrows and Ctrl combos from the toolbar take effect immediately and never close the keyboard.
4. UX-4: Pressing Enter after typing a word never reorders text relative to Enter.
5. UX-5: On desktop with a hardware keyboard, no new UI appears and typing feels unchanged.

Disconnected input
6. UX-6: Within 250 ms of the first typed char while disconnected, a chip reading "Disconnected - typing is not being sent." appears above the toolbar.
7. UX-7: The chip has a Reconnect button of at least 44x44 px that starts a reconnect in one tap.
8. UX-8: After reconnecting to the same session within 30 s, queued text arrives once, in order, and the chip confirms the count then disappears within 4 s.
9. UX-9: Esc, arrows and Ctrl combos typed while disconnected are never replayed after reconnect.
10. UX-10: Overflow, TTL expiry and session change each show "Discarded N chars ..." with a dismiss button; none is silent.
11. UX-11: A failed reconnect shows "Still disconnected. Check your connection." and the Reconnect button remains usable.

Read-only
12. UX-12: In a read-only session, tapping the terminal does not open the keyboard and shows "This terminal is view-only." at most once per 10 s.
13. UX-13: In a read-only session the extra-keys row and Compose toggle are hidden.

Compose bar
14. UX-14: The Compose toggle (44px) opens a visible textarea with Send and Send+Enter in one tap; it is off by default.
15. UX-15: Send inserts the text as one unit (bracketed paste when the app enabled it) and does not press Enter; Send+Enter presses Enter once.
16. UX-16: Text of more than 4096 chars sent while offline is not cleared and shows "Too long to hold while offline (4096 chars max)."
17. UX-17: Closing compose keeps the draft; reopening restores it.
18. UX-18: After 5 keydown-229 events with no text, a one-time notice "Keyboard input fell back to compose mode" shows with "Got it" and "Turn off".

Diagnostics
19. UX-19: The diagnostics sheet opens in at most 2 taps or via `?inputdebug=1` and is titled "Input diagnostics".
20. UX-20: Header shows renderer and fallback reason, e.g. "renderer: xterm (fallback: mobile_input_layer off)".
21. UX-21: Copy report, Clear and Pause work; copied JSON never contains typed text even with Reveal on.
22. UX-22: The sheet closes via Close, Back, scrim tap and Escape, and focus returns to the opener.
23. UX-23: When the clipboard is blocked, the report is shown selectable with manual-copy guidance.

Accessibility and theme
24. UX-24: Every new interactive control is at least 44x44 CSS px with 8px spacing, has an accessible name, and shows a visible focus ring at 3:1 contrast.
25. UX-25: With TalkBack on, the chip, discard notice and read-only hint are announced once, and the hidden input is announced as "Terminal input, edit box".
26. UX-26: All new surfaces use theme tokens; in both light and dark themes text contrast is >= 4.5:1 and icons/borders >= 3:1; status never relies on color alone.
27. UX-27: With the soft keyboard open, chip, compose bar and toolbar remain fully visible above it and the sheet never exceeds the visible viewport.
28. UX-28: With the flag off, none of the layer's own interactive surfaces (chip, compose bar, fallback notice) render and behavior matches today's build, with exactly two carve-outs: the informational plain-language flag-off hint on the Android class (UX-39) and the diagnostics surfaces (header `layer: off`).
29. UX-29: No error state is a dead end: each has a dismiss, retry, reconnect or close action (see the exit matrix).
30. UX-30: With reduced motion on, chip and sheet appear without animation.

Added by the triad-review repair (2026-11-08)
31. UX-31: With TalkBack on, typing a word announces the committed word once (not per character twice, not silent), the element is announced "Terminal input, edit box", and Backspace runs announce nothing; owner-checked in the pre-merge phone session (plan Task 5.1.4d).
32. UX-32: An autocorrect rewrite shows at most one intermediate state per animation-frame sampling, never a blank frame or shortened prompt; a DeferredCommit (> 64 graphemes or newline) shows "Typing... (N characters waiting)" after 300 ms and removes it on commit.
33. UX-33: Cursor-row-visible holds under keyboard-height emulation on the Pixel 7 profile (automated) and on the real Chrome and Samsung keyboard modes recorded by Story 0.1.5 (owner device).
34. UX-34: Compose mode is offered after 5 keydown-229 events with no text AND after 3 layer anomalies within 60 s (wrong or garbled text); it is always reachable manually from the toolbar overflow and the diagnostics sheet.
35. UX-35: Reconnecting over 10 s, a replay in flight, a replay over 5 s and a replay stuck over 15 s each show their own copy with a Retry, Reconnect or Dismiss action; none is silent or a dead end.
36. UX-36: The diagnostics sheet is reachable in at most 2 taps from the settings entry or the toolbar overflow without any long-press; the chip long-press is a secondary shortcut that does not trigger the Android context menu.
37. UX-37: With toolbar, compose bar, chip and keyboard all present, the terminal keeps at least 5 full rows at 360x340 and 320x300 and at least 3 at 740x170 and 360x240; at 360px width the chip, Reconnect and Dismiss do not overlap and need no horizontal scroll.
38. UX-38: Announcements use two regions: polite for status and hints, assertive only for lost or possibly incomplete typed content; a message announces once per state change and an assertive message is never duplicated in the polite region.
39. UX-39: The flag-off hint on the Android class uses plain language with no query parameter or flag identifier.


## Plan alignment (Phase 3 plan-repair, 2026-11-08)

Each UX gap now has a plan owner:

| UX item | Plan story/task |
|---|---|
| Read-only hint `role="status"` (S4) | Task 1.4.1d |
| IME-fallback notice controls "Got it" / "Turn off" / localStorage opt-out (S6, UX-18) | Story 4.2.2, Task 4.2.2b |
| Compose bar extras: Send+Enter, draft retention across Close, too-long-offline error, close on read-only (S5, UX-14..17, UX-13) | Story 4.2.1, Task 4.2.1d |
| Chip extra states: "Still disconnected", "keys not sent" for control-only input, 4 s success confirmation, dismissible discard notices (S3, UX-8, 10, 11) | Story 3.1.5, Task 3.1.5a |
| Enter typed while disconnected is not queued and shows "Enter not sent" (S3, UX-9) | Story 3.1.3 (ADR-004) |
| Model B preview vs live (S2) | Decided: live atomic rewrite (ADR-002); S2 unchanged |
| Diagnostics sheet: modal on phone (focus trap, scrim, `aria-modal`), non-modal docked panel on desktop (S7, UX-22) | Story 4.1.2 |
| Flag off is a rollback state, not a user-facing setting; no UI offers it (S8, UX-28) | Plan Risk Control |
| Tap opens the soft keyboard on both renderers (UX-1) | Story 1.2.3 (`bindTapTarget` on the visible surface, synchronous focus, physical-phone check Task 0.1.3b) |
| S9 report vs trace (`reportToSkeleton`, `captureToTrace`) | Stories 4.1.1 (A1) and 4.1.3 (PR-C) |
| Flag-off hint, `?mobile_input=0|1`, `?inputdebug=1` minimal panel (S8, S7) | Tasks 1.3.2e, 1.4.1e, 2.3.1a (PR-A1) |
| TalkBack typed-character announcement, two live regions (S1, section 0, UX-31, UX-38) | Story 1.4.4, Task 5.1.4d |
| Rewrite flicker criterion and DeferredCommit feedback (S2, UX-32) | Story 1.2.5, Task 5.1.1e |
| Chrome stack budget, minimum terminal rows, 360px chip layout (section 0, UX-37) | Story 1.4.3 |
| Android keyboard resize modes, ADR-003 iOS-origin assumption, cursor-row emulation (section 0, UX-33) | Story 0.1.5 |
| Compose offer on repeated anomalies (S6, UX-34) | Task 4.2.2c |
| Plain-language flag-off hint (section 0, S8, UX-39) | Story 1.3.2 |
| Reconnecting and replay stuck copy (S3, UX-35) | Task 3.1.5b |
| Diagnostics entry in settings/overflow, long-press collision (S7, UX-36) | Task 4.1.2c |
| Chip texts for TTL and session change (S3) | Stories 3.1.3 and 3.1.5 (PR-B, behind `mobile_input_queue`, default off) |

Platform scope: S1 applies only on the Android touch-primary class (`shouldAttachMobileInput()`); iOS Safari and touch laptops with a hardware keyboard see no new surface (UX-5, UX-28). Compose bar, diagnostics sheet and fallback notice are behind their own sub-flags and ship after the first PR.
