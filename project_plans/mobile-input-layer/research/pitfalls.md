# Pitfalls: mobile-input-layer (SDD Phase 2)

Labels: VERIFIED = opened file/ran command or read the cited page's search snippet; INFERRED = reasoning from evidence. Web claims come from search-result summaries, not full page reads, so they are VERIFIED-as-reported only.
Rank = likelihood (L) x impact (I), each 1-3. Score 9 = design against explicitly; 6 = mitigate in plan; <=4 = note.

## Ranked summary

| # | Pitfall | L | I | Score |
|---|---|---|---|---|
| P1 | Gboard event contract is unstable (no keydown/composition, 229 keydowns, mismatched inputType) | 3 | 3 | 9 |
| P2 | Diff-based text reconciliation resends whole line / eats chars (xterm 6 `_handleAnyTextareaChanges`) | 3 | 3 | 9 |
| P3 | Touch handlers call `preventDefault` and may block focus/keyboard; stray `focus()` pops keyboard | 2 | 3 | 6 |
| P4 | Playwright/CDP cannot reproduce real Gboard/Samsung; green e2e gives false confidence | 3 | 3 | 9 |
| P5 | Sticky Ctrl/Alt/Shift only wired on the toolbar path, not on typed IME text | 3 | 2 | 6 |
| P6 | Rewrite/backspace burst interacts with 512B chunking + 10ms delay + mouse-report filter | 2 | 3 | 6 |
| P7 | Hidden-textarea residue after IME commit corrupts later voice/IME replace input | 2 | 2 | 4 |
| P8 | Logging/metrics leak input content or blow label cardinality | 2 | 3 | 6 |
| P9 | CI gates (jscpd ratchet, zero-warning lint, registry, route tables, BUG-087 filename, bun) | 3 | 2 | 6 |
| P10 | Two renderers: seam leaks; ghostty gate misses a path | 2 | 3 | 6 |
| P11 | Hot-path counter overhead on desktop typing | 1 | 2 | 2 |
| P12 | Silent drop when disconnected (`sendInput`) hides the symptom again | 2 | 2 | 4 |

## P1. Gboard/Samsung event contract is unstable (9)

- Gboard plain taps can arrive as `beforeinput`/`input` only: no keydown, no composition, no onData. https://github.com/NousResearch/hermes-agent/issues/137986 (VERIFIED as reported).
- With suggestions off, Gboard sends keyCode-229 keydowns then plain `input` events without composition; xterm's `_keyDownSeen` guard swallowed them. https://github.com/NousResearch/hermes-agent/issues/136179 (reported).
- Samsung Keyboard emits `insertCompositionText` with `isComposing:true` even for ordinary typing. https://contenteditable.realerror.com/scenarios/scenario-samsung-keyboard-text-prediction/ (reported).
- `beforeinput` may say `insertCompositionText` while `input` says `deleteContentBackward`; store `targetRanges` at beforeinput. https://contenteditable.realerror.com/docs/ime-composition/ (reported). Also `beforeinput` may not fire at all for suggestion insertion mid-composition, so `preventDefault()` there may never run (same source).
- Android Chrome composition events are inconsistent: https://blog.open-xchange.com/resources/ox-techblog/article/android-chrome-and-composition-events/ ; ProseMirror calls it "the absolute worst": https://discuss.prosemirror.net/t/contenteditable-on-android-is-the-absolute-worst/3810
- Enter handlers miss Enter when keyboard fires beforeinput-only or keyCode 229: https://github.com/agent-of-empires/agent-of-empires/issues/1174
- Repo already knows 229 matters elsewhere: `web-app/src/components/command-bar/CommandBar.tsx:150,156`, `web-app/src/lib/hooks/useKeyboard.ts:71` skip `keyCode===229`. If the new layer's textarea lives inside a region those global handlers see, they will ignore its keydowns (INFERRED, fine) but a global handler that does NOT guard 229 could `preventDefault` real keys (check during implementation).

Design against: never depend on any single event class. Treat `beforeinput`, `input`, `compositionupdate/end`, and 229 keydown as redundant signals feeding one state machine keyed off the textarea's value (read the DOM value, not the event payload); dedupe by value snapshot; do not rely on `preventDefault()` working on `beforeinput`. Test against recorded sequences, with an "unrecognized sequence" counter (already in requirements).

## P2. Diff-based reconciliation bugs (9)

- xterm 6 `CompositionHelper._handleAnyTextareaChanges` computes `newValue.replace(oldValue,'')`, so autocorrect and double-space-period resend the whole line. https://github.com/NousResearch/hermes-agent/issues/138081 ; https://github.com/Ark0N/Codeman/pull/541 ; https://github.com/devswha/ovis/pull/2789 ; upstream https://github.com/xtermjs/xterm.js/issues/6078 (reported).
- Stray DEL when Enter follows a committed char in the same batch; Enter clears the textarea and the diff reads it as deletion. https://github.com/Ark0N/Codeman/issues/448 (reported).
- Workaround used by several projects: one DEL per removed code point, then inserted text. Pitfall: code points vs UTF-16 units vs grapheme clusters (emoji, ZWJ). INFERRED: count by code point is still wrong for TUI apps that delete by grapheme; bound rewrites to the composition region and prefer not rewriting at all when the region crosses a newline.
- Terminal apps (Claude Code input box) have their own cursor model; shell readline and TUI differ on DEL semantics. Requirements rabbit hole confirms (`requirements.md:83`).

Design against: (a) reset the textarea to a sentinel (empty or fixed cursor-in-middle string) after each commit so the diff base is always known; (b) bound rewrite to composition region, cap backspace count, fall back to "append only" on anomaly and count it; (c) never clear the textarea in the same tick as Enter is emitted without snapshotting first; (d) property/fuzz test on diff function.

## P3. Touch handlers vs. soft keyboard (6)

Repo facts (VERIFIED):
- Gesture touchstart is registered `{passive:false}` on the container, touchmove/touchend `{passive:false}` on document: `web-app/src/lib/hooks/useTerminalGestures.ts:785-788`.
- State machine returns `preventDefault` on scroll start/continue/end and coast-stop: `web-app/src/lib/terminal/gestureMachine.ts:89,91,93,104,113,114`. Comment at `:110`: "preventDefault stops Chrome synthesizing a click (and focus) after a scroll". Horizontal-first deliberately does not preventDefault (`:85`).
- Plain taps call `t.focus()` (when not mouse-forwarding) at `useTerminalGestures.ts:640` and `:660`. In mouse-tracking mode (TUIs like Claude Code that enable mouse), the tap is converted to an SGR click and `t.focus()` is called only after `onSendData` (`:660`). This focus happens inside touchend handling, which is a user gesture, so should open the keyboard; but note `preventDefault` on touchend suppresses the synthetic click which is how Chrome normally focuses, so the code relies on the explicit `focus()`.
- `endSelecting` blurs the textarea on purpose to stop xterm's "Linux" platform sniff (matches Android) from popping the keyboard mid-selection: `useTerminalGestures.ts:683-689`. A new input layer that focuses its own hidden element on selection/copy would reintroduce that popup.
- Selection-handle touchstart is `{passive:false}`: `XtermTerminal.tsx:1136`; scrollbar thumb touchstart `:1258`.
Web evidence: preventDefault on touchstart can block the Android keyboard, and keyboard activation is bundled with native scroll; call preventDefault only on touchmove (Rick Byers) https://lists.w3.org/Archives/Public/public-touchevents/2014Apr/0008.html ; focus() needs a user gesture (same thread). Old (2014), treat as background; no current-Chrome source found (GAP).

Design against: the input layer's `focus()` must run synchronously in the touchend/click gesture, never in a timer, rAF or promise; keep the `touchstart` path free of preventDefault; one explicit "focus owner" function used by tap path and by toolbar; blur-after-selection behavior ported to the new element; on-device test checklist for "tap -> keyboard opens" after scroll, after selection, in mouse-tracking mode.

## P4. Emulation limits: Playwright cannot be the proof (9)

- CDP offers `Input.imeSetComposition` (sets candidate text, empty cancels) and `Input.insertText` (text not from a key press): https://pkg.go.dev/github.com/chromedp/cdproto/input . One project reports Chromium + Pixel 7 emulation passing 15/15 with imeSetComposition + insertText + keydown 229, but it explicitly did not exercise real Gboard/Samsung: https://github.com/okou-ai/cdp-viewer/issues/3 (reported).
- Cannot emulate: Gboard's suggestion-strip replacement (`insertReplacementText`), swipe-typing commit shape, Samsung's composition-for-everything, the Android composing-region (`InputConnection.setComposingRegion`) semantics, keyboard-open viewport resize/visualViewport behavior, IME-driven "orphan" `input` events (INFERRED; I found no doc that CDP synthesizes these). Whether CDP-driven composition fires `beforeinput insertCompositionText` identically to Android is an unconfirmed GAP.
- Repo: no existing CDP/IME e2e usage (grep for `newCDPSession` across `tests/e2e` returned no files; `hasTouch|isMobile|devices[` appears in `accessibility.spec.ts`, `files-tabs.spec.ts`, `backlog-settings-a11y.spec.ts`, `files-viewer.spec.ts`, `backlog-features-flags.spec.ts`). So CDP IME helper is new infrastructure in `tests/e2e/helpers/`.
- Android-flavored e2e precedent exists: `tests/e2e/android-url-bar.spec.ts` (listed in `tests/e2e/`).

Design against: the Jest replay corpus of recorded real sequences is the correctness proof; Playwright validates wiring (textarea focus, flag, bytes reach pane, both renderers) only, labeled so in the spec header. Owner-device recording is a gating deliverable; synthesized sequences must be tagged `synthetic` in the corpus and counted separately so the 100% pass number cannot be read as device-faithful. Add a debug "record IME events" mode (types only, no content: see P8) so the owner can capture the corpus. CI cannot catch regressions from Gboard updates: keep the unrecognized-sequence counter as the production signal.

## P5. Sticky modifiers (6)

- Sticky Ctrl/Alt/Shift live in `TerminalOutput.tsx`: state `:275`, applied via `CTRL_KEY_MAP[keyData] ?? keyData` at `:1196-1197` inside the toolbar key handler (VERIFIED, lines 1196-1208). `CTRL_KEY_MAP` (`web-app/src/lib/terminal/modifierKeys.ts:5-16`) only maps arrows/Home/End/PgUp/PgDn plus `/` and `-`; letters have no entry, so Ctrl then typed `c` from the soft keyboard falls through unchanged (`?? keyData`) if routed through this map: sends `c` not `\x03`. INFERRED: for typed text the layer needs its own letter mapping (`char & 0x1f`) and Alt prefix (ESC), and must clear the sticky flag after one use, which the toolbar path presumably does but the IME path is a different producer.
- Risk: a multi-char commit (autocomplete pick "hello ") under sticky Ctrl; define semantics: apply to first char only or reject modifier and clear. Also IME composition with Ctrl active: Gboard shows word candidates; modifier + composition is undefined.

Design against: route IME output through the same `deliverTerminalData`/modifier function as the toolbar (single choke point) and specify sticky semantics for single-char vs multi-char commits; unit-test table.

## P6. Chunking, bracketed paste, mouse filter (6)

- `sendInput` splits at `PASTE_CHUNK_SIZE = 512` bytes with `CHUNK_DELAY_MS = 10` (`web-app/src/lib/hooks/useTerminalFlowControl.ts:238-239,248,284-285`). Chunking is byte-based on UTF-8 (`TextEncoder`): a chunk boundary can split a multibyte char or an escape sequence (INFERRED; check the loop at `:284` slices the Uint8Array, so a split inside a multi-byte code point is possible; server/tmux reassembles bytes, but the 10ms inter-chunk gap lets other input interleave: a Backspace burst for a rewrite plus new text sent as separate `sendInput` calls could reorder around a chunked paste).
- Rewrite = N DELs + text. N is bounded by composition length so normally < 512; but a rewrite of a long autocompleted line plus chunking means DELs and text must go in ONE `sendInput` call to preserve order (INFERRED).
- Bracketed paste: grep for `200~`/`2004` in `web-app/src/components` and `web-app/src/lib/hooks` found nothing in the sending path, so xterm.js (not this repo) wraps pastes; with the new layer a Gboard "paste" from the suggestion strip arrives as `insertFromPaste`/`insertText` and would NOT be bracketed, so multi-line text becomes executed lines (security-relevant, INFERRED). Design: treat `insertFromPaste`/multi-line `insertText` as paste and wrap with `\x1b[200~...\x1b[201~` only if the app enabled mode 2004 (the renderer knows); strip embedded `\x1b[201~` from pasted text.
- `MOUSE_REPORT_RE = /^(?:\x1b\[(?:<\d+;\d+;\d+[Mm]|M[\s\S]{3}))+$/` (`XtermTerminal.tsx:155-156`) and the filter in `terminal.onData` at `:950` drop whole strings that are mouse reports when override is `none`. VERIFIED. The layer's output bypasses `terminal.onData`, so (a) it won't be filtered (fine) but (b) the ghostty path has its own onData; ensure the layer does not need the filter and a user typing literal text like `\x1b[M...` can't be mistaken (anchored regex, low risk). The real hazard: if the layer calls xterm's `triggerDataEvent` (Codeman approach) the filter applies.
- Ordering vs touch-click mouse reports: taps emit `onSendData(press+release)` (`useTerminalGestures.ts:660` area); interleaving with IME flush is fine only if both go through one queue.

Design against: single ordered output queue; one `sendInput` per logical edit; atomic DEL+text; grapheme/codepoint-safe chunking test; explicit paste classification.

## P7. Hidden textarea residue (4)

xterm issue: IME-committed text stays in the hidden textarea until blur; later voice-IME field replacement corrupts input. https://github.com/xtermjs/xterm.js/issues/6012 (reported). Design against: clear/reset the buffer at commit boundaries (compatible with P2) and set `autocomplete=off`, `autocapitalize=off`, `autocorrect=off`, `spellcheck=false` per requirements; note turning autocorrect off may hide the very suggestion strip the owner wants (product decision: owner wants autocomplete/autocorrect working, so attributes are a policy knob, not constant).

## P8. Observability pitfalls (6)

- Never log input: requirements `requirements.md:58,115` say counters carry sizes and classes only. Pitfall spots: the first-input Info log line and "event shape beacon" must carry only `inputType`, length bucket, composing bool. Passwords typed into sudo prompts go through the same path (INFERRED). Also don't put `data` in debug dumps: `web-app/src/lib/terminal/TerminalDebugMonitor.ts` and `mobileDebug.ts` exist (not read) and may already capture strings; check them (GAP).
- Cardinality: requirements specify per-session counters at `terminal_input_frames_total{kind}` ; per-session ID as a label is unbounded. Use class labels only (kind, listener=local|remote, renderer, client_class); per-session detail goes to the actuator JSON view, not Prometheus. The repo has a documented label-bounds convention in `docs/reference/client-network-telemetry.md` (listed in CLAUDE.md index; not re-read) - follow it.
- Server input handlers to instrument: `connectrpc_websocket.go:3129`, `connectrpc_websocket_shell.go:265,:3849`, `session_service_stream_terminal.go:439` (from requirements). Missing one leaves a blind transport (INFERRED); add a test that enumerates input entry points or a single `recordInput` choke point.
- Hot path: `terminal-input-batching` already batches; a counter increment is an atomic add and negligible; do NOT classify via regexing the payload per frame beyond first-byte check (kind=escape iff first byte 0x1b) to keep O(1). Per-session map lookups under a mutex on every frame would be the real cost: use `atomic` fields on the stream struct (see `golang-concurrency` skill, `docs/explanation/concurrency-patterns.md`).
- Client beacon: `ime_event_sequence_unrecognized_total` must be rate limited and bounded, or a buggy keyboard floods the beacon path.

## P9. CI / repo gate gotchas (6)

- jscpd: `web-app/.jscpd.json` has `minLines 20, minTokens 200, threshold 0.14` (VERIFIED). CLAUDE.md says the baseline is mostly irreducible `jest.mock(...)` blocks and each new heavily mocked test file nudges the ratchet (0.1 -> 0.12 -> 0.14). A replay harness with many near-identical corpus test files or two renderer integration tests duplicating setup would trip it. Mitigate: table-driven single replay test reading corpus JSON, shared fixture helper, one integration test parametrized over renderers; run `bun run lint:duplicates` in `web-app/` before pushing.
- Zero-warning web lint; `@/lib/nav` facade is irrelevant unless adding a page (no new page expected; if a settings panel page is added, it needs a `route-meta.ts` row and the 3-file route tables - memory `instinct_pr_ci_gates_lint_routes_config_surface`).
- Feature registry: `make registry-generate` after adding RPC/page/`// +feature:` markers; `tested` flag is hand-set (memory `instinct_new_rpc_ci_gates`). If the flag toggle uses the existing feature-flag RPC, no new RPC; a new beacon RPC would trigger sinkguard write classification.
- BUG-087: avoid "gate" in Go test filenames (memory note; underlying flake doc `docs/bugs/open/BUG-087-captureLogs-global-slog-swap-races-under-t-parallel.md` - problem is `captureLogs` swapping global `slog` under `t.Parallel`). So server counter tests must not swap the global logger; inject a logger or read counters. Also name the `ghostty_renderer` fallback test file without "gate".
- Flag: live-settable, no env var (`requirements.md:50`). Existing flag plumbing: `web-app/src/lib/features/flagStore.ts`, usages in `GhosttyTerminal.tsx`, `TerminalPool.tsx`. A flag read at module load / terminal construction means flipping it live won't affect already-mounted terminals (INFERRED); make the layer subscribe or document "applies to new terminals".
- Bun not pnpm/npm: `unset npm_config_user_agent` before `make build`; worktree needs `make build` for generated protos/ent (gen dirs gitignored; memory `instinct_alias_session`, `instinct_web_app_bun_user_agent_guard`). Do not commit generated code.
- E2E conventions (CI-checked, per `CLAUDE.md` E2E section): `// @feature ...` first line; no `waitForTimeout`; `data-testid`/ARIA locators only; helpers under `tests/e2e/pages/`. The hidden textarea needs a `data-testid` and `aria-label` (also good for a11y/TalkBack); Playwright cannot target an `aria-hidden`/`display:none` input, and a hidden-but-focusable element must stay in the a11y tree or the Axe CI check may flag it (INFERRED).
- Flaky rule: `t.Parallel()` and no real sleeps (`deterministic-fast-tests`). For Jest, inject a fake clock for any "settle after N ms" logic (batching) and pin `Math.random` (memory `instinct_flake_root_causes_wakes_and_backoff`). Composition debounce timers in the layer must be injectable.
- Manual testing: use the isolated instance recipe in CLAUDE.md (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET`), never `make install-service` pre-merge; the owner phone reaches `:8444` over remote listener so the manual instance needs `--remote-port 62872`, and the private CA cert issue (memory `project_mobile_app_server`) may block the phone from a different port/host (GAP).

## P10. Two renderers and the ghostty gate (6)

- ghostty-web 0.4.0 ignores 229 keydowns (`dist/ghostty-web.js:851`) and `preventDefault`s every `beforeinput` (`:2330`) (from requirements, `requirements.md:15`; not independently re-opened). If the new layer's textarea is a child of the ghostty container, ghostty's own listeners on the container or document can still see bubbled events and `preventDefault` them, cancelling IME insertion (INFERRED). Design: place the layer's element outside the renderer's DOM subtree or stop propagation at the layer root for input events.
- Both renderers may attach their own hidden textarea (xterm creates one: `terminal.textarea` is used at `useTerminalGestures.ts:687`). Two focusable text inputs = keyboard flicker, double input if both react to the same IME text. Design: disable xterm's textarea path on touch when the layer is active (e.g. `readonly`/`tabindex=-1`), or make the layer the only sink.
- Gate: requirement says `ghostty_renderer` falls back to xterm on touch unless layer active. Pitfall: `TerminalPool.tsx` and `GhosttyTerminal.tsx` both read the flag; gate in one function (`flagStore.ts` consumer), not in each; touch detection must match `isTouchPrimary` used in `XtermTerminal.tsx` (referenced near the selection code) rather than a second definition.

## P11. Hot-path overhead (2)

Desktop path must stay zero-cost (`requirements.md:56`): attach the layer only when touch-primary and flag on; no `beforeinput` listener on desktop. Client counters = integer increments only.

## P12. Silent drop (4)

`sendInput` returns early with only a console.warn when not connected (`useTerminalFlowControl.ts:241` via `ensureConnected`) and early-returns on `readOnlyRef` (`:241`). IME text typed during a reconnect is lost and the composition state machine may believe it was sent (rewrite then deletes characters that were never sent). Design: `sendInput` should return a boolean; layer must reset its diff base on a failed send (or queue) and surface the indicator.

## Explicit design-against list (top)

1. Single state machine fed by redundant signals, value-snapshot based, no reliance on `beforeinput.preventDefault` (P1, P2).
2. Atomic edit emission: DEL*n + text in one `sendInput`, bounded to composition region, anomaly fallback, counter (P2, P6).
3. Focus only inside user gesture; never in touchstart; port blur-after-selection (P3).
4. Corpus of recorded real sequences is the proof; Playwright only validates wiring; synthetic tagged (P4).
5. One output choke point shared with toolbar modifiers; defined sticky semantics; paste classification and bracketed paste (P5, P6).
6. Metrics: class labels only, atomic counters, no content, rate-limited beacon (P8).
7. Test layout that stays under jscpd 0.14, no "gate" filenames, injectable clocks, data-testid on the textarea (P9).
8. Layer element isolated from renderer listeners; one text sink per terminal (P10).
9. sendInput reports success so the layer resets on drop (P12).

## Gaps (could not verify)

- Current Chrome Android rule for `preventDefault` on touchend and keyboard open (only 2014 sources found).
- Whether `beforeinput` is cancelable for Gboard composition events on Chrome Android.
- Whether CDP `Input.imeSetComposition` produces the same `beforeinput`/`input` sequence as Android's InputConnection.
- Contents of `TerminalDebugMonitor.ts` / `mobileDebug.ts` regarding input capture; ghostty-web dist lines (taken from requirements).
- EditContext support status on Chrome Android was not researched in this pass (search budget); belongs to the state-of-the-art dimension.
