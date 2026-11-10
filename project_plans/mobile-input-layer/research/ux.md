# UX Research: typing into a terminal from a phone

Date: 2026-11-06. Scope: UX dimension of `mobile-input-layer`.

Evidence labels: VERIFIED = opened in this repo this session (path:line) or a normative spec I can cite by URL; INFERRED = from my prior knowledge of the product/spec, not re-fetched in this session (no web fetch was run). Treat INFERRED comparable-app claims as hypotheses for Phase 3 to spot-check.

## 1. Comparable patterns

| Product | Autocorrect / suggestions | Compose or preview | Keyboard show/hide + resize | Hardware kbd | Label |
|---|---|---|---|---|---|
| Termux (Android) | Terminal view sets `TYPE_NULL`-style input so Gboard suggestions are off; users who want them use a "text input" mode. Extra-keys row above IME (Esc, Tab, Ctrl, Alt, arrows) with sticky modifiers. | None by default; direct-to-pty. | Resizes terminal on IME open (`adjustResize`); extra keys stick to IME. | Works as normal keys; extra row can hide. | INFERRED (https://wiki.termux.com/wiki/Touch_Keyboard) |
| Blink Shell (iOS) | Software kbd works but is raw; strong hardware-kbd focus; configurable key bar (Ctrl/Esc/arrows, swipe-on-key variants). | No compose bar. | Terminal reflows above kbd; kbd accessory bar toggles. | First-class (Caps-as-Esc/Ctrl remap). | INFERRED (https://docs.blink.sh/) |
| Termius | Direct input; top-of-keyboard key row with Ctrl/Alt/Fn, snippets. Optional "text input" toggle for IME. | Snippets act as pre-composed commands. | Reflows above kbd. | Supported. | INFERRED (https://termius.com/) |
| JuiceSSH | Direct input with extra-key row; known weakness with Gboard suggestions producing duplicate text. | None. | `adjustResize`. | Supported. | INFERRED |
| ConnectBot | Raw input; keyboard-visible toggle; long-press on screen for Ctrl/Esc popup. | None. | Manual toggle. | Supported. | INFERRED |
| Claude Code on web/mobile (claude.ai/code) | Chat-style native text box, not a pty: full IME (suggestions, swipe, autocorrect) because it is a normal `<textarea>` and the text is submitted as a message. | Native text box IS the compose bar; send on tap. | Normal web form behavior. | Enter submits. | INFERRED |
| GitHub Codespaces mobile web | VS Code web terminal (xterm.js); soft-keyboard input is known flaky; GitHub recommends the mobile app / hardware keyboard for the terminal. | None. | Viewport shrink only. | Works best. | INFERRED (xterm.js #3600 cited in requirements.md:16) |
| Warp | Desktop-only; block-based input editor is a native-text editor separate from the pty (autocomplete/suggestions rendered in the editor, committed on Enter). Relevant as the "compose before commit" model. | Yes: input editor, then submit as one unit. | n/a | n/a | INFERRED (https://docs.warp.dev/) |
| Working Copy (iOS) | Not a terminal; its editor and command palette use native text fields with full autocorrect control. Relevant only as precedent for native-text fields where the content is text, not keystrokes. | n/a | n/a | n/a | INFERRED |

Takeaway (INFERRED): terminal apps split into two camps. Raw-keystroke terminals (Termux, Blink, JuiceSSH, ConnectBot) turn off or tolerate-badly IME suggestions and compensate with an extra-keys row. Compose-first products (Claude web, Warp) give the user a native text field and send a unit. Nobody in the raw camp does rewrite-aware suggestions well; that is the gap this project can beat.

Existing in-repo decisions (VERIFIED):
- Extra-keys toolbar already Termux-style with sticky Shift/Ctrl/Alt: `web-app/src/components/sessions/TerminalOutput.tsx:2392-2420`. Hidden when `readOnly` (`:2397`). Keys fire on `onPointerDown` with `preventDefault()` so the IME is not dismissed (`:2400+`).
- `mobile-ux-improvements` decisions: toolbar toggle persisted in localStorage (ADR-004), `visualViewport`/`dvh` keyboard avoidance (ADR-003), 44px key min-height (`project_plans/mobile-ux-improvements/requirements.md:35`, `--min-touch-target: 44px` at `:57`). That project's primary target was iOS Safari and explicitly excluded Android (`:40`); mobile-input-layer is the Android complement.
- Compact toolbar defaults for phones with fine pointer: `TerminalOutput.tsx:318-356`. Mouse mode defaults to `none` on mobile (`:397`).
- Read-only hint already rate-limited at 10s: `TerminalOutput.tsx:90,1068-1080`.
- Manual Reconnect button after 5s disconnected: `TerminalOutput.tsx:1360-1370`.

## 2. Mental models and the suggestion-replaces-word problem

Phone user expectation (INFERRED): tapping a suggestion replaces the word being typed; autocorrect fires on space/punctuation and may rewrite the previous word; swipe typing inserts a whole word plus trailing space; long-press backspace deletes words.

Terminal reality (VERIFIED for ghostty-web: requirements.md:14-16): bytes are write-only; the app (Claude Code's prompt box, readline, stelekit) owns the buffer. The client cannot read what the app thinks the line is. A "replacement" must be synthesized as N backspaces (`\x7f`) plus new text, which is only correct if the app's cursor is at the end of the composed region and each backspace removes exactly one grapheme.

Options for a replacement:

| Model | Behavior | Pros | Cons |
|---|---|---|---|
| A. Live rewrite (diff while composing) | Send backspaces+text on every `insertCompositionText` change | Echo feels immediate; matches what Termux does with composition off | Many flicker/backspace storms; wrong if user moves cursor or app reflows; high orphan risk; each intermediate state hits the TUI (Claude Code may re-render per key) |
| B. Preview then commit | Show composing text in a local overlay at the cursor (underlined); send bytes only when composition ends (`compositionend`, space, Enter, or blur) | Exactly one rewrite per word; no partial garbage in pane; undo is local until commit | Slight echo delay (until commit); needs overlay positioned at terminal cursor (renderer must expose cursor cell) |
| C. Compose bar (native textarea above keyboard) | Full native editing; "Send" writes the entire string (as bracketed paste or typed) | Perfect IME fidelity, autocorrect, swipe, long-press, selection; trivially accessible; works on every renderer | Not a terminal: no per-key response (tab completion, `?`, vim, y/n prompts); extra tap to send; context switch; Enter semantics ambiguous |

Recommendation (INFERRED, to validate in Phase 3 design): ship B as the default for plain-text insertion, with rewrites bounded to the current composition region (requirements.md:84-85), plus C as an opt-in secondary mode ("Compose" toggle in the toolbar) for long prompts to agents, which is the owner's dominant use (typing English instructions to Claude Code). Keystroke-critical contexts (y/n, menus, vim, Esc, arrows, Ctrl combos) always go direct via the toolbar or hardware keys; they never pass through compose.

Rules for B:
1. Composition text is shown locally and not sent until commit; commit = diff against last committed text of the region.
2. If the region's committed text contains characters other than printable text (cursor keys were pressed mid-composition), abandon rewrite: send only the append (no backspaces) and flag `ime_event_sequence_unrecognized_total`.
3. Backspace outside composition = single `\x7f` per `deleteContentBackward`; long-press repeat arrives as repeated events; word delete (`deleteWordBackward`) sends `\x17` (Ctrl-W) rather than N backspaces (INFERRED: readline/Claude Code both honor it; verify).
4. Enter = `\r`, never `\n`, and flushes any pending composition first (ordering matters; the Enter/Backspace wrapping bug in xterm.js #3600 cited in requirements.md:16).
5. Emoji/multi-codepoint: send as one UTF-8 write, no chunk split inside a grapheme.

## 3. Accessibility

- WCAG 2.2 SC 2.5.8 Target Size (Minimum): 24x24 CSS px (AA). https://www.w3.org/TR/WCAG22/#target-size-minimum — VERIFIED (normative). Project already uses a stricter 44px (matches 2.5.5 AAA, https://www.w3.org/TR/WCAG22/#target-size-enhanced). Requirement: any new control (Compose toggle, indicator retry button, diagnostics toggle) is at least 44x44 CSS px, spacing included.
- SC 2.4.7 Focus Visible and 2.4.11 Focus Not Obscured (Minimum): the soft keyboard and sticky toolbar must not cover the focused control. https://www.w3.org/TR/WCAG22/#focus-not-obscured-minimum — VERIFIED (normative). The visualViewport work in ADR-003 serves this; add test that the hidden input's caret/terminal cursor stays above toolbar.
- SC 4.1.2 Name, Role, Value and 4.1.3 Status Messages: connection-lost and read-only indicators must be `role="status"` (polite) or `role="alert"` for dropped input; toolbar buttons already carry `aria-label` and `aria-pressed` (`TerminalOutput.tsx:2405-2416`, VERIFIED).
- ARIA for the input element: xterm.js uses a hidden `<textarea>` with `aria-label="Terminal input"` (INFERRED, https://github.com/xtermjs/xterm.js/blob/master/src/browser/CoreBrowserTerminal.ts); ghostty-web uses a contenteditable with `role="textbox"` (per task brief; INFERRED). Our shared input layer should expose ONE element: `<textarea role="textbox" aria-multiline="true" aria-label="Terminal input" aria-describedby=<status id>>`, so assistive tech sees the same control regardless of renderer. Do not set `aria-hidden`; do not move it off-screen with `display:none` (breaks focus and keyboard); keep it 1px, opacity ~0, positioned at the cursor so TalkBack's focus rectangle and the IME candidate window anchor near the text.
- TalkBack (INFERRED): in a focused textbox TalkBack announces typed characters via accessibility text-change events, which depend on the element's value changing. A write-only terminal that clears the textarea after each commit will announce nothing or wrong text. Mitigation: keep the textarea value equal to the in-flight composition only, announce committed text through a polite live region, and treat terminal output separately (do not re-announce the whole screen). Gboard + TalkBack "explore by touch" double-tap activation focuses the textarea, so the focus path must not rely on touch events with `preventDefault` (requirements.md:85).
- Prefers-reduced-motion and contrast: indicator styling must meet 3:1 non-text contrast (SC 1.4.11, https://www.w3.org/TR/WCAG22/#non-text-contrast, VERIFIED) and not rely on color alone (icon + text).

## 4. Error and edge states

Current behavior (VERIFIED): `sendInput` returns silently when read-only (`useTerminalFlowControl.ts:242`) and, when not connected, `ensureConnected("send input")` warns to console and returns (`:243`, per requirements.md:21). The user sees nothing; a phone user with the keyboard up cannot see the console. The Reconnect button appears only after 5s (`TerminalOutput.tsx:1360-1370`), and the toolbar and typing remain visually enabled.

Proposed behavior:

1. Stream disconnected while typing.
   - Visual: within 250 ms of the first dropped/queued input, show an inline status chip pinned just above the extra-keys row: "Disconnected - typing is not being sent. Reconnect" (44px target, `role="status"`, `aria-live="polite"`). Toolbar keys get a dim "will queue" state, not hidden.
   - Queue policy: queue plain-text input (not escape sequences, not control keys) in a bounded buffer (<= 4 KiB, <= 30 s). On reconnect, flush if the session id and pane are unchanged, otherwise discard with a visible "Discarded N chars typed while offline" chip. Escape sequences and Ctrl combos are NOT queued (replaying `Esc`/`Ctrl-C` seconds later in a different TUI state is dangerous). Overflow drops oldest-first with the same chip. Rationale: replayed text is idempotent-ish in a prompt; control keys are not. (INFERRED design; needs owner approval since it changes "silently drop" semantics.)
   - Count: `input_dropped_disconnected_total` (requirements.md:105) incremented for every dropped/discarded byte class; queued-then-flushed counted separately (`input_queued_flushed_total`).
   - The composition preview (model B) keeps showing locally while queued, tinted with a "pending" style so the user can tell it hasn't reached the pane.
2. Read-only session (`readOnly` / `terminal_read_only`). Keep the soft keyboard from opening: set `readonly`/`inputmode="none"` on the input element and hide the extra-keys row (already hidden at `:2397`). A tap on the terminal shows the existing rate-limited hint (`:1068-1080`) as `role="status"`: "This terminal is view-only". Do not queue input.
3. Keyboard layout differences. Never assume QWERTY: rely on `beforeinput.data`, not `keyCode`/`key`, for text (`keyCode 229` is the IME-busy sentinel, requirements.md:15). Toolbar glyph keys (`/`, `-`) are layout-independent bytes. For non-Latin layouts (CJK, Arabic RTL) composition must pass through unmodified UTF-8; no ASCII-only filters (INFERRED risk: `isMouseReport` filter and paste chunking must not split surrogate pairs).
4. IME unavailable / odd. If no `beforeinput` arrives after N taps within a focused session (soft keyboard open, `keydown` 229 seen, zero data), fall back automatically: show the Compose bar (model C) with a one-time notice "Keyboard input fell back to compose mode". Also covers voice-only IMEs and password-manager keyboards. Increment `ime_event_sequence_unrecognized_total`.
5. Flag fallback. `mobile_input_layer` off: current behavior. On touch, if `ghostty_renderer` is on and the layer is not active, fall back to xterm.js and show nothing user-facing, but log the reason once (requirements.md:70, 110). Diagnostics overlay (below) displays "renderer: xterm (fallback: mobile_input_layer off)" so a confused owner can see why the renderer changed.
6. Hardware keyboard attached. Detect via `(pointer: fine)` plus first `keydown` with `keyCode !== 229` and physical `code`. When active: soft-keyboard policy and Compose bar default off, extra-keys row follows the existing `compactToolbar` rule (`TerminalOutput.tsx:318-325`, VERIFIED), IME layer yields to renderer's native key handling so desktop behavior is byte-identical (requirements.md:36). Android with a hardware kbd still uses IME for non-Latin text; keep `beforeinput` path armed but let `keydown` of printable keys win to avoid double-sends (dedupe by event time window).

## 5. Jobs to be done

Owner persona: runs several AI agent sessions, often from a phone (requirements.md:9).

- Functional: answer an agent's question or approve a step (short text, y/n, Enter) within seconds; give a one-paragraph instruction to an agent; interrupt (Esc, Ctrl-C); recover a misspelled command without retyping.
- Emotional: confidence that "what I typed is what the agent got" (no phantom or missing characters); relief from not needing a laptop; no anxiety when the connection flickers (knowing whether input was sent).
- Social: not blocking a teammate or the agent's pipeline while away from the desk; appearing responsive; (for shared remote listener use) showing a view-only state clearly so no one assumes they can intervene.

Implication: optimize the 80% path (English sentences into the agent prompt) for IME fidelity via preview-then-commit and Compose, and the 20% (control/TUI navigation) for the toolbar. Error states exist to preserve trust in "sent vs not sent".

## 6. Concrete UX requirements (proposed, numbered for Phase 3)

- UX-1: Typing on a phone via autocomplete pick, autocorrect, swipe, and Backspace produces the same final text in the pane as a hardware keyboard (maps to success metric 1).
- UX-2: Default text path is preview-then-commit; no more than one backspace-rewrite burst per composed word; rewrite bounded to the active composition region.
- UX-3: Enter, Backspace, Esc, Tab, arrows, Ctrl combos always bypass composition and are sent immediately and in order relative to pending text.
- UX-4: Optional Compose bar (native textarea + Send, 44px targets), auto-offered on IME fallback; sends as a bracketed paste when the app has enabled it, else typed text.
- UX-5: Disconnected: visible status chip within 250 ms, queue plain text (<= 4 KiB / 30 s), never queue control sequences, discard with visible notice, counters incremented. No silent drop.
- UX-6: Read-only: soft keyboard does not open; hint is `role="status"`; extra-keys hidden.
- UX-7: All new controls >= 44x44 CSS px, labelled, focus-visible, not obscured by the soft keyboard (SC 2.5.8, 2.4.7, 2.4.11).
- UX-8: One input element across renderers with `role="textbox"`, `aria-label="Terminal input"`, `aria-multiline="true"`; committed text announced via polite live region; verified with TalkBack manually.
- UX-9: Hardware keyboard detected; desktop path unchanged byte-for-byte and in latency.
- UX-10: Diagnostics overlay (below) reachable in <= 2 taps, off by default, no input content stored by default.

## 7. Diagnostics overlay design

Purpose: let the owner (or a bug report) see "last N input events and bytes sent" on a new keyboard without dev tools on a phone.

Entry: long-press (800 ms) on the connection chip, or toolbar overflow "Input debug", or `?inputdebug=1`. Hidden by default; state persisted in localStorage per device (try/catch, like the toolbar toggle at `TerminalOutput.tsx:328-389`).

Layout (bottom sheet, max 40vh so the terminal stays visible, scrollable, 44px tap rows, `role="dialog"` with title "Input diagnostics"):

1. Header row: renderer (xterm | ghostty | fallback reason), `mobile_input_layer` flag state, stream state (connected / reconnecting / read-only), IME state (composing / idle), `beforeinput` supported, userAgent short, keyboard hint (Gboard / Samsung / unknown by event pattern).
2. Counters (since open): events seen, bytes sent (text / escape), dropped-disconnected, queued, anomalies.
3. Ring buffer table, last N=50 events, newest first: `t+ms | source (beforeinput/keydown/input/composition*/toolbar) | inputType or key | data class | composing? | action (sent/held/dropped/rewrite) | bytes out (count + class)`.
   - Content policy (aligned with "never log input content", requirements.md:59,107): default view shows data class and length only, e.g. `text(5)`, `bs(2)+text(4)`, `esc(3)`. A "Reveal text" switch, session-only and off at open, shows the data locally; revealed content is never beaten to server or beacon.
4. Actions: Copy report (JSON of the ring buffer in the redacted form plus header), Clear, Pause capture. The Copy report is what the owner attaches to a bug to feed the replay corpus (successful path to recording real Gboard/Samsung streams, requirements.md:43,98).
5. Optional beacon: opt-in "Send anonymized event shapes" posts only class/length sequences through the client telemetry beacon (pattern in `docs/reference/client-network-telemetry.md`).

Implementation notes: the ring buffer lives in the shared input layer (not in the renderers), fixed 200 entries in memory, zero work when overlay closed except a push to a bounded array (keeps desktop path cost near zero; gate push behind a flag checked once per event). Existing `mobileDebug` helper (`TerminalOutput.tsx:1213`, `mobileDebug.toolbarKey(...)`, VERIFIED to exist) is the likely precedent to extend rather than duplicate.

## 8. Open items for Phase 3
- Confirm with owner: queue-on-disconnect semantics (UX-5) and Compose bar as opt-in (UX-4).
- Spot-check INFERRED comparables (Termux wiki, Blink docs, Warp input editor) before citing in the ADR.
- Verify Ctrl-W word-delete behavior inside Claude Code's prompt box on a real session.
- Needs a real TalkBack pass (cannot be verified in CI).
