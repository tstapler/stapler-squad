# Architecture Research: Renderer-agnostic soft-keyboard input layer

Scope: where the layer lives, how both renderers consume it, server counters, flag/gate wiring, hotspot dispositions. Paths are relative to the repo root (worktree `ssq-wt-mobile-input`). Confidence: VERIFIED = line opened in this session; INFERRED = deduced; UNVERIFIED = not checked.

## 0. Prior art reused (not re-derived)

- `project_plans/terminal-input-batching/research/architecture.md`: batching wraps chunking inside `useTerminalFlowControl`; the layer must sit upstream of `sendInput` and add nothing between keystroke and `sendInput` beyond one frame. Its Event-Command-Policy (ECP) table format is reused in section 6.
- `project_plans/fix-mouse-input-readiness/research/architecture.md`: input filtering is a ref-backed wrapper on the terminal's `onData` inside the renderer (threaded via refs to keep closures stable, no listener re-registration). The layer follows the same ref pattern. Its "both PTY-state and user-preference" gate is the template for the renderer gate.
- `project_plans/mobile-ux-improvements`: `isKeyboardVisible` is only the extra-keys toolbar (ADR-004, localStorage), not the OS keyboard. It stays untouched.

## 1. Today's input path (VERIFIED)

```
xterm.js:  term.onData -> isMouseReport filter -> onDataRef -> TerminalOutput.handleTerminalData -> deliverTerminalData -> sendInput -> WS
ghostty:   term.onData -> onDataRef (no filter)            -> (same)
toolbar:   sendKey (sticky ctrl/alt/shift maps) -> handleTerminalData / deliverTerminalData
```

- XtermTerminal: `terminal.onData` at `web-app/src/components/sessions/XtermTerminal.tsx:949-952`, mouse filter `MOUSE_REPORT_RE` at `:154-156`, custom key handler `:838` (copy/select-all only for the part read), `preventDefault` in touch drag/scroll-thumb handlers (`:1102`, `:1117`, `:1230-1245`).
- GhosttyTerminal: `term.onData((data) => onDataRef.current?.(data))` at `GhosttyTerminal.tsx:99`; `focus: () => terminalRef.current?.focus()` at `:164`. File header (`:23`) says touch gestures are not ported.
- Renderer choice: fixed per page load at `web-app/src/lib/terminal/TerminalPool.tsx:359` (`preloadFlagValue(GHOSTTY_RENDERER_FLAG) ? GhosttyTerminal : XtermTerminal`).
- TerminalOutput: `deliverTerminalData` `:1049` (`sendInput`, Enter clears approvals), `handleTerminalData` `:1069` (readOnly guard, page-key estimate invalidation), `sendKey` `:1193` (sticky modifiers only applied to toolbar keys; I found no modifier handling on the `onData` path, UNVERIFIED beyond grep of `handleTerminalData`).
- `sendInput` `useTerminalFlowControl.ts:241`; `PASTE_CHUNK_SIZE = 512` `:238`; disconnected input is dropped inside `ensureConnected` with only `console.warn` (`:144-146`).
- Tap-to-focus calls `t.focus()` in `useTerminalGestures.ts:640,660`; a comment at `:683-687` already documents that a synthetic focus pops the Android soft keyboard and the code blurs it, so focus routing is already delicate on Android.

Root-cause statement for the regression (from requirements, ghostty-web source cited there at `dist/ghostty-web.js:851,2330`): ghostty-web swallows keyCode-229 keydowns and `preventDefault`s every `beforeinput`, so Android IME text never reaches `term.onData`. The `ghostty-web` package is not installed in this worktree (`web-app/node_modules/ghostty-web` absent), so those two line numbers are carried over, not re-verified here.

## 2. Where the layer lives

**New module, outside both renderer components:** `web-app/src/lib/terminal/mobileInput/`

```
mobileInput/
  attachMobileInput.ts     // the seam: attach(container, opts) -> MobileInputController
  composition.ts           // pure state machine: (state, event) -> {state, commands}   (no DOM)
  rewrite.ts               // pure diff: region before/after -> backspace count + text
  keymap.ts                // physical-key -> bytes; reuses lib/terminal/modifierKeys.ts maps
  policy.ts                // isTouchDevice(), attribute policy, thresholds
  telemetry.ts             // counters + anomaly beacon (cm.* events)
  mobileInput.css.ts       // vanilla-extract: visually-hidden but focusable textarea
  __fixtures__/*.json      // recorded/synthesized event streams (Gboard, Samsung)
  *.test.ts
```

Why `lib/terminal/` and not `components/sessions/`: it has no React dependency, so both renderers (and any future one) import the same function; the pure state machine is unit-testable and replayable in Jest without a DOM renderer. Matches the existing `lib/terminal/ghosttyAdapter.ts` / `modifierKeys.ts` neighbours.

**Why not inside each renderer:** the regression exists because input handling lived inside the renderer library; duplicating it in two components would repeat the failure mode on the next swap.

**Why not in `TerminalOutput.tsx`:** it is a 2,456-line hotspot and owns the terminal container only indirectly (`terminalContainerRef`); the renderers own the element where focus lands.

### The seam

```ts
// attachMobileInput.ts
export interface MobileInputOptions {
  onData: (bytes: string) => void;          // same sink the renderer's onDataRef uses; bypasses the mouse-report filter (see 5.3)
  getModifiers?: () => { ctrl: boolean; alt: boolean; shift: boolean };  // sticky toolbar state
  consumeModifiers?: () => void;
  isReadOnly?: () => boolean;
  onAnomaly?: (kind: ImeAnomaly) => void;   // class only, never content
}
export interface MobileInputController {
  focus(): void;        // must be called synchronously inside a user gesture
  blur(): void;
  dispose(): void;
  readonly active: boolean;
}
export function attachMobileInput(container: HTMLElement, opts: MobileInputOptions): MobileInputController | null; // null => unsupported, caller keeps native path
```

Each renderer adds one effect (guarded by `useMobileInputLayerActive()`), stores the controller in a ref, and changes its imperative handle to `focus: () => controller?.focus() ?? term.focus()`. That is about 10 lines per renderer. The layer's bytes go straight to the renderer's `onDataRef` (not through `terminal.onData`).

### Hidden element

A `<textarea>` appended to the terminal container (sibling of the renderer's canvas/helper textarea, not a descendant of ghostty's input target, so ghostty's `beforeinput`/`keydown` handlers never see its events). Styled via `mobileInput.css.ts` (vanilla-extract per `docs/reference/css-architecture.md`; no inline CSS): 1px, opacity 0, `pointer-events: none`, `caret-color: transparent`, positioned at the cursor row when known (keeps the browser from scrolling the page when focused). `data-testid="mobile-input-textarea"` for e2e. Attribute policy lives in `policy.ts` (open decision, see section 9): `autocapitalize="none"`, `spellcheck=false`, `autocomplete="off"`, `enterkeyhint="send"`; `autocorrect` left on by default because the owner wants autocorrect/autocomplete to keep working.

On the xterm path the existing `.xterm-helper-textarea` must not also take focus on touch with the layer active (double-send risk): set `inputmode="none"` and `tabindex="-1"` on it while the layer is attached. INFERRED to be sufficient; verify in the Playwright mobile spec by asserting a single set of bytes per keystroke.

## 3. Event-state machine

Pure function in `composition.ts`; the DOM adapter in `attachMobileInput.ts` only normalizes events into the machine's input type and executes returned commands.

States: `Idle`, `Composing{region, sent}`, `Rewriting` (transient inside one event turn), `Degraded` (orphaned-`input` fallback in use).

- `region` = text of the current composition as the browser reports it. `sent` = exactly what this layer has already emitted for that region (string). Rewrites are computed `sent -> region`: `common prefix` kept; for the tail of `sent` after the prefix emit one `\x7f` per grapheme, then emit the new tail. **Bound:** never emit more backspaces than `sent.length` in graphemes, and never when `sent` is empty. This is what stops a rewrite from eating text the user typed before the composition (the rabbit hole in requirements).
- Live-send default (characters echo immediately, matching hardware feel and the "one animation frame" SLO); `commitMode: "deferred"` (hold until `compositionend`) is the fallback if a region exceeds `MAX_REGION` (default 64 graphemes) or an anomaly fires. Decision recorded as an ADR candidate (section 9).
- `keydown` with `keyCode === 229` or `isComposing`: ignored by the machine and NOT `preventDefault`ed (preventing it can suppress `beforeinput` on some Gboard versions; INFERRED from xterm.js #3600, UNVERIFIED on device).
- Physical keyboard on a touch device: non-229 keydowns for Enter/Backspace/Tab/Esc/arrows/Ctrl-combos are mapped by `keymap.ts` and `preventDefault`ed (so no duplicate `beforeinput`); printable keys fall through to `beforeinput`.
- Orphaned `input` (an `input` event with no preceding handled `beforeinput`, xterm.js #3600): `Degraded` mode diffs `textarea.value` against the shadow value and emits the delta, then resets value to `""`; emits `ime_event_sequence_unrecognized`.
- After every commit (`compositionend`, or a non-composing `insertText`), the textarea value is reset to the empty string so the browser's own buffer never grows or drifts from `sent`.

## 4. Data flow

```
Gboard/Samsung -> textarea events -> attachMobileInput (normalize) -> composition.ts (pure) -> commands
   -> keymap/modifier application (sticky ctrl/alt) -> opts.onData(bytes)
   -> renderer onDataRef -> TerminalOutput.handleTerminalData (readOnly, estimate invalidate)
   -> deliverTerminalData -> sendInput (chunk/batch) -> WS -> server input handler (counters) -> tmux
```

Interactions:
- Mouse filter: not in the path. Routing layer bytes through `terminal.onData` would subject a user typing text that resembles a mouse report to `isMouseReport` (`XtermTerminal.tsx:156`); calling `onDataRef` directly avoids it.
- Sticky modifiers: toolbar modifiers today only apply in `sendKey`. The layer takes `getModifiers/consumeModifiers` so a single printable char typed while Ctrl is armed maps to a control code (`c & 0x1f`), and Alt prefixes ESC (reusing `ALT_KEY_MAP` fallback semantics at `TerminalOutput.tsx:1198`). Whether the owner wants this on soft-keyboard text is open (section 9).
- Paste: `insertFromPaste` flows as text; `sendInput` already chunks at 512 bytes (`useTerminalFlowControl.ts:238-290`).
- readOnly: `isReadOnly()` short-circuits before emit; `handleTerminalData` still shows the hint.
- Focus: the soft keyboard opens only when focus is requested inside a user gesture. `useTerminalGestures` tap paths call `t.focus()` (`:640,660`) and have a known blur workaround for Android (`:683-687`); both go through the renderer handle, so routing `handle.focus` to `controller.focus()` keeps the gesture-synchronous call. Touch handlers that `preventDefault` on `touchmove`/`touchend` (`XtermTerminal.tsx:1102,1117`; `useTerminalGestures.ts:409-411,783`) must not wrap the focus call; covered by the e2e spec.
- Attach only to the focused terminal (requirement: many terminals per page): attach on focus/dock, dispose on blur/undock, driven from the pool's dock/focus logic (`TerminalPool.tsx:537` area) via the renderer effect that already tracks `isVisible`.

## 5. Failure and fallback behavior

1. `attachMobileInput` returns `null` (no `beforeinput`, not touch, flag off): renderer keeps its native path, behavior identical to today.
2. Unrecognized sequence: `Degraded` orphaned-input diff; anomaly counter; never throws into the event loop (wrap handlers, `onAnomaly` + `console.error` once).
3. Rewrite bound exceeded: switch to deferred commit for that region, count `ime_rewrite_bound_exceeded`.
4. Disconnected stream: `ensureConnected` (`useTerminalFlowControl.ts:144`) gets an `onInputDropped(bytes)` callback; `TerminalOutput` shows a visible, `aria-live` indicator (`data-testid="terminal-input-dropped"`) and increments `input_dropped_disconnected_total`. Input is dropped, not buffered (replaying stale keystrokes into a different pane state is worse than telling the user).
5. Layer disabled mid-session by the flag: takes effect on next page load, like the other client-read flags.

## 6. Event-Command-Policy table (IME event -> terminal bytes)

| Event | Command | Policy |
|---|---|---|
| `keydown` keyCode 229 / isComposing | none (do not preventDefault) | defer to `beforeinput`; count only |
| `keydown` non-229 mapped control key (Enter, Backspace, Tab, Esc, arrows, Ctrl-x) | emit mapped bytes, preventDefault | applies sticky modifiers; physical keyboard parity with desktop |
| `compositionstart` | enter `Composing{region:"", sent:""}` | clear shadow value |
| `beforeinput` `insertText` (not composing) | emit text | apply Ctrl/Alt if armed (single char only); reset textarea value |
| `beforeinput` `insertCompositionText` | rewrite `sent -> region` (backspaces + tail) | bounded by `sent`; `MAX_REGION` fallback to deferred |
| `beforeinput` `insertReplacementText` (autocorrect/suggestion pick) | rewrite `sent -> replacement`, where `sent` is the word range the event reports via `getTargetRanges()` or the current region | if range unavailable, treat as region rewrite; else anomaly |
| `beforeinput` `deleteContentBackward` | emit `\x7f` x1 (or shrink `sent` if inside composition) | never delete below region start unless region empty (then plain DEL) |
| `beforeinput` `deleteWordBackward` | emit `\x17` | matches Ctrl+W readline behavior |
| `beforeinput` `insertLineBreak` / `insertParagraph` | emit `\r` | finalize region first (flush deferred text) |
| `beforeinput` `insertFromPaste` | emit text | goes through chunking in `sendInput` |
| `compositionend` | finalize: reconcile `sent` with final `data`, then reset value | one last rewrite if `data !== sent` |
| `input` with no handled `beforeinput` | diff value vs shadow, emit delta | `Degraded`; anomaly counter |
| `blur` / `visibilitychange` | finalize composition, dispose draft | never leave a half-sent region |
| `selectionchange` moving the caret inside region | treat as cancel: finalize, reset | prevents backspace counts against a stale region |

Table is the single source for `composition.test.ts` cases; the fixture corpus (`__fixtures__/*.json`) records `{events[], expectedBytes}` and the replay harness runs it against `composition.ts` alone (renderer-independent), plus a thin jsdom adapter test.

## 7. Server input path and counters

VERIFIED call sites (all in `server/services/`):
- Shared read loop: `dispatchInputReadLoopFrame` `connectrpc_websocket.go:3129`; `p.onInput(input.Data)` at `:3131`, read-only attach drops with a Debug log at `:3134`. Two `onInput` closures feed it: control-mode path `:1653` and hub path `:2489` (both do `UpdateTerminalTimestamps`, permission check, `writer.SendInput`).
- Capture-pane path: `handleCapturePaneInput` called from `:3849`. Shell path: `handleShellInput` from `connectrpc_websocket_shell.go:265-267`. Streaming RPC path: `session_service_stream_terminal.go:439` (`case *sessionv1.TerminalData_Input`).
- `writerFor`/`writerDecision` at `connectrpc_websocket.go:404-415`.
- `ws_terminal_mux_bridge.go`: grep for `Input|onInput|SendInput` found no input handling in the file. INFERRED: mux frames reach one of the handlers above; confirm in Phase 3 by test through `/api/ws/terminal-mux`.

**Placement:** one helper `recordTerminalInput(path, listener, data []byte, accepted bool)` in a new file `server/services/terminal_input_metrics.go`, called from the five sites above before the writer/permission checks (so read-only drops and permission denials are counted with `accepted=false`, which is exactly the "phone typed but nothing arrived" diagnostic). The shared read loop (`:3131`) covers both the `:1653` and `:2489` closures with one call; do not edit the closures. Classifier (bytes only, no content stored): `escape` = first byte `0x1b`; `text` = all bytes printable or `\r`/`\t`/`\n`/`\x7f` and valid UTF-8; `other` = everything else (control chords, invalid UTF-8). Mouse reports count as `escape`.

**Metric conventions (VERIFIED pattern):** OTel instruments created with `counterNamed/histNamed/upDownNamed` in `server/services/ws_unary_metrics.go:30-50`; `listener` is a server constant (`local`/`remote`, doc `docs/reference/client-network-telemetry.md` "Server WebSocket bridge metrics"); `server/listener_metrics.go` exists for the listener label. VictoriaMetrics appends `_total`.
- `terminal_input_frames_total{kind=text|escape|other, listener, path=cm|hub|shell|capture|stream, accepted}`
- `terminal_input_bytes_total{...same labels}` (counter, add `len(data)`)
- No session id/title labels (requirement; label bound policy in the telemetry doc). Per-session visibility is in-process: a bounded LRU tally (256 sessions) `map[sessionID]{frames,bytes,lastAt,byKind}`, served at a new `GET /actuator/terminal-input` registered next to `/actuator/ws-unary` (`server/actuator.go:45`, handler pattern `handleActuatorWsUnary` `:124`), JSON with `?format=text` like the other actuators. Plus an Info log on first input per stream and a throttled (1/min/stream) summary line, never content.
- Alert/diagnostic: compare client `client_ime_bytes_emitted_total{renderer}` (below) with `terminal_input_frames_total{kind="text",listener="remote"}`; sustained client >0 with server 0 = the symptom in this project.

**Client telemetry (VERIFIED pattern):** `cm.*` events are allow-listed in `server/analytics/client_metrics.go:28-40` (e.g. `cm.coalesce`, `cm.ws`) and documented in `docs/reference/client-network-telemetry.md`. Add `cm.input` (`event`: `emitted|dropped_disconnected`, `renderer`: `xterm|ghostty`, fixed enums) and `cm.ime_anomaly` (`kind` from a fixed enum), update the allowlist, the doc's metric table and `docs/observability/grafana/dashboards/client-network.json`. Client-side senders live next to `web-app/src/lib/telemetry/`.

## 8. Feature-flag plumbing and renderer gate

- Server: add a `mobile_input_layer` entry to `knownFeatureFlags` beside `ghostty_renderer` (`server/services/feature_flag_service.go:369`, VERIFIED shape: `name`, `description`, optional `defaultValue`). Kill-switch semantics: `defaultValue: true` ("Default: on for touch devices; turn off to restore the previous behavior"). `server/services/config_surface_proof_test.go` and `feature_flag_service_test.go` reference `ghostty_renderer`; they will need the new flag (VERIFIED they mention it; exact assertions not read).
- Client store: `web-app/src/lib/features/flagStore.ts` pre-load values are special-cased by name (`:21`, `preloadFlagValue` `:88-96` returns persisted `"1"` for ghostty). Add `MOBILE_INPUT_LAYER_FLAG` and a pre-load that returns `true` unless persisted `"0"` (kill switch default-on), persisted via the same `ssq:flag:` prefix so the value exists synchronously at terminal mount. Persistence write path is `recordFlagWrite` `:158`.
- **Gate (pure, testable):** `lib/terminal/rendererPolicy.ts`

```ts
export function resolveRenderer({ ghosttyFlag, mobileLayerFlag, isTouch }): "ghostty" | "xterm" {
  if (!ghosttyFlag) return "xterm";
  if (isTouch && !mobileLayerFlag) return "xterm"; // no layer on touch => never ship the dead keyboard
  return "ghostty";
}
```
  Used at `TerminalPool.tsx:359` in place of the inline ternary. `isTouch` = `matchMedia("(pointer: coarse)")` or `navigator.maxTouchPoints > 0`, in `policy.ts`, shared with `useMobileInputLayerActive()` (active = flag && isTouch && controller supported). A hybrid laptop with touchscreen counts as touch; the layer is safe there because physical keys go through `keymap.ts`. Unit tests cover the 8 combinations.
- Rollback is the live flag (applies next load, consistent with the pool comment at `TerminalPool.tsx:355-357`).

## 9. Hotspot dispositions

| File | Disposition | Reason |
|---|---|---|
| `XtermTerminal.tsx` (1,857 lines) | Isolate via seam | Add one effect + handle `focus` rerouting + helper-textarea suppression; no logic moves in. |
| `GhosttyTerminal.tsx` (188) | Extend as-is | Small; same effect as xterm, no refactor needed. |
| `TerminalOutput.tsx` (2,456) | Extend as-is (minimal) | Add only the dropped-input indicator and modifier getters; its input callbacks (`handleTerminalData`/`deliverTerminalData`) are already the right sink. |
| `useTerminalFlowControl.ts` | Extend as-is | Add `onInputDropped` in `ensureConnected`; leave chunking/batching untouched. |
| `useTerminalGestures.ts` (810) | Isolate via seam | Only verify focus calls (`:640,660`) go through the handle; the `:683-687` Android workaround may need to skip when the layer is active (decide in Phase 3 with a device/emulator test). |
| `TerminalPool.tsx` | Refactor-first (tiny) | Extract the renderer ternary into `resolveRenderer` before adding the gate, so the gate is unit-testable. |
| `flagStore.ts` | Extend as-is | Add one constant and one pre-load branch. |
| `connectrpc_websocket.go` (large) | Isolate via seam | One `recordTerminalInput` call at `:3131`; all logic in a new file. Do not restructure the `onInput` closures. |
| `connectrpc_websocket_shell.go`, `session_service_stream_terminal.go` | Extend as-is | One helper call each. |
| `analytics/client_metrics.go` | Extend as-is | Two allow-listed events + instruments, following existing pattern. |

## 10. Repo rules applied

- `.claude/rules/nav-facade.md`: the layer imports no router APIs. The pointer/focus code uses no `window.history` calls. No new page, so no `route-meta.ts` row. Any indicator link uses `@/lib/nav`, not `next/*`.
- CSS: `mobileInput.css.ts` (vanilla-extract), no inline `style` except the dynamic cursor-row offset set via CSS variable.
- Registries: the omnibar registries (`docs/reference/feature-testing-registry.md`) do not apply. Analogue to maintain: (a) the ECP table in section 6 doubles as the fixture index (each row has at least one fixture), (b) `docs/reference/client-network-telemetry.md` tables, (c) `docs/registry/features/` via `make registry-generate` if a `// +feature:` marker is added to the new component files, (d) `tests/e2e` specs start with `// @feature ...`, use `data-testid`/ARIA locators, no `waitForTimeout`.
- Tests: Jest replay (pure machine), jsdom adapter tests, Playwright mobile emulation under both renderers (the Playwright server pool in `tests/e2e/global-setup.ts` runs ghostty only if the flag is set for that spec; spec sets it via the feature-flag RPC). New Go tests call `t.Parallel()`. Bun only; `unset npm_config_user_agent`.
- Duplication gates: keep fixtures as JSON (jscpd counts `.ts`), and share one attach hook between renderers to avoid a copy-pasted effect.

## 11. Open decisions for Phase 3 / ADRs

1. Live-send vs deferred commit default (recommend live with bounded rewrite; record as ADR, validate on device).
2. `autocorrect` on or off (recommend on; owner requirement is that autocorrect works).
3. Apply sticky Ctrl/Alt to soft-keyboard text (recommend yes, single-char only).
4. EditContext as progressive enhancement: add as an `InputSource` adapter behind the same `attachMobileInput` seam only if the replay corpus shows `beforeinput` gaps; not in the first PR.
5. Real device streams: add a dev-only recorder (`?imeRecord=1`) that downloads the normalized event log so the owner can capture Gboard/Samsung sequences; fixtures from it replace synthesized ones.
6. Confirm root-cause split (flag off alone restores letters?) on device before finalizing the gate's wording.

## Files referenced

- `web-app/src/components/sessions/XtermTerminal.tsx`, `GhosttyTerminal.tsx`, `TerminalOutput.tsx`
- `web-app/src/lib/terminal/TerminalPool.tsx`, `ghosttyAdapter.ts`, `modifierKeys.ts`
- `web-app/src/lib/hooks/useTerminalFlowControl.ts`, `useTerminalGestures.ts`
- `web-app/src/lib/features/flagStore.ts`
- `server/services/connectrpc_websocket.go`, `connectrpc_websocket_shell.go`, `session_service_stream_terminal.go`, `ws_unary_metrics.go`, `feature_flag_service.go`
- `server/actuator.go`, `server/listener_metrics.go`, `server/analytics/client_metrics.go`
- `docs/reference/client-network-telemetry.md`, `docs/reference/css-architecture.md`
- `project_plans/terminal-input-batching/research/architecture.md`, `project_plans/fix-mouse-input-readiness/research/architecture.md`
