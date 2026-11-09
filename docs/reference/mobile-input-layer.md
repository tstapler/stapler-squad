# Mobile input layer

Reference for the phone keyboard input layer: a hidden `<textarea>` that turns Android soft-keyboard events (`beforeinput`, composition) into terminal bytes, under either renderer. Decisions: `project_plans/mobile-input-layer/decisions/ADR-001..008`. Troubleshooting: `docs/how-to/debug-phone-typing.md`. Browser-side metric tables: `docs/reference/client-network-telemetry.md`.

Status markers: sections marked **(planned)** describe later delivery slices (PR-B queue, PR-C diagnostics sheet) and are not in the code yet.

## Where it attaches

`shouldAttachMobileInput()` (`web-app/src/lib/terminal/mobileInput/platform.ts`) is true only for an Android, touch-primary (`pointer: coarse`), no-hover device. Desktop, iOS, iPadOS and touch laptops with a hardware keyboard never attach, whatever the flag says. Chrome's "Request desktop site" drops `Android` from the user agent and opts out. `attachMobileInput` returns `null` (zero listeners, zero DOM nodes) off that class or when the browser lacks `beforeinput`.

## Seam contract

`attachMobileInput(container, opts): MobileInputController | null` in `mobileInput/attachMobileInput.ts`. The `container` must be a sibling of, never inside, the renderer's element (a textarea inside it would have its `beforeinput` swallowed by the renderer's blanket `preventDefault`; the attach logs an error and returns `null`).

`AttachOptions` (if this table and `AttachOptions` disagree, the code wins):

| Option | Required | Meaning |
|---|---|---|
| `onData(bytes, seq)` | yes | Receives each atomic unit (text, DEL runs, escape sequences) and returns `"sent" \| "queued" \| "dropped"` (or nothing, read as `sent`). Never log `bytes`. |
| `getModifiers()` / `consumeModifiers()` | yes | Armed Ctrl/Alt state from the host's modifier bar; consumed once per character. |
| `isReadOnly()` | yes | True suppresses emission and sets `inputmode="none"`. |
| `onAnomaly(kind)` | yes | Anomaly class only (`ImeAnomaly`), never data. Feed `createMobileInputTelemetry().onAnomaly`. |
| `onDiagnostic(entry)` | no | Content-free trace (`unit`, `invalidate`, `anomaly`, `watchdog`) for the ring buffer. Feed `telemetry.onDiagnostic` too. |
| `onQueueDiscard(listener)` | no | Subscribes the layer to queue discards (PR-B). |
| `onDeferredWaiting(n)` | no | Grapheme count held back by a deferred commit after 300 ms; `null` when cleared. |
| `bracketedPasteEnabled()` | no | DECSET 2004 state of the active renderer; defaults to off. |
| `policy` | no | `MobileInputPolicy` tunables (`policy.ts`). |
| `clock` | no | Injected time source; the layer never reads wall-clock time or `requestAnimationFrame` directly. |
| `rendererElement` | no | The element handed to the renderer's `open()`, for the container-outside check. |
| `describedBy` | no | Id of the polite status region the textarea is described by. |

The controller exposes `element`, `active`, `focused`, `focus()` (call inside the user gesture), `blur()`, `bindTapTarget(surface)`, `invalidate(reason)` (`session`, `pool_reuse`, `visibility`, `queue_discard`, `transport`), `reportQueueDiscard(report)`, `syncReadOnly()` and `dispose()`.

Typed text never reaches the console, `onAnomaly`, diagnostics or telemetry: only anomaly classes, result classes and sequence ids do.

### Reporting telemetry

```ts
const telemetry = createMobileInputTelemetry({ renderer: "xterm", clock });
attachMobileInput(container, { ...opts, onAnomaly: telemetry.onAnomaly, onDiagnostic: (d) => { ring.push(d); telemetry.onDiagnostic(d); } });
// on dispose: telemetry.dispose()
```

`mobileInput/telemetry.ts` aggregates and rate-limits (below). It sends through the shared analytics sink (`trackSharedEvent`) unless a `track` function is injected.

## Flags

| Flag | Default | Notes |
|---|---|---|
| `mobile_input_layer` | on (server default in `server/services/feature_flag_service.go`; client constant `MOBILE_INPUT_LAYER_DEFAULT` in `flagStore.ts` must equal it, a test asserts it) | Rollback-only kill switch, live-settable in the feature-flag panel with per-scope override. Off falls back to the plain terminal, and ghostty is then not used on Android phones. Affects only the Android class. |
| `mobile_input_queue` | off | Queue keystrokes while disconnected and send on reconnect **(planned, PR-B)**. Needs the layer flag. |
| `mobile_input_diagnostics` | off | The full diagnostics sheet **(planned, PR-C)**. The minimal `?inputdebug=1` panel is not behind it. |
| `mobile_input_compose` | off | Compose box for longer input **(planned)**. |
| `mobile_input_edit_context` | off | EditContext instead of the textarea where supported (ADR-006). |

Flag off on the Android class is never silent: a one-line dismissible status hint appears in plain language (it names no query parameter or flag id).

### `?mobile_input=0|1` override

Open any page with `?mobile_input=0` (force off) or `?mobile_input=1` (force on). The flag store reads it before first mount, applies it in the same load and stores it under `localStorage` key `ssq:flag-override:mobile_input_layer`. Semantics, from `flagStore.ts`:

- It is per origin (`localStorage` is per origin): repeat it on `:8543`, `:8444`, the Tailscale host and the LAN IP.
- It beats the server value, because `persistFlags` rewrites `ssq:flag:mobile_input_layer` from the server on every refresh and would otherwise erase it within about 30 s.
- It lasts until the user flips the flag in the feature-flag panel (`recordFlagWrite` clears the override so the server value governs again).
- Any other value is ignored. Without the param, flipping the flag off needs one reload to persist and a second to apply.

## Input sites and the `received` / `forwarded` stages

The server counts every input frame at its entry path (`server/services/terminal_input_metrics.go`, sites in `terminal_input_sites.go`). `received` is recorded at the entry, before the read-only and permission gates. `forwarded` is recorded when the gates passed and the write was issued, so `received - forwarded` means "denied before the write", never "this path does not record forwarded".

| `path` label | Entry function | `forwarded` recorded | Gates before it |
|---|---|---|---|
| `cm` | `handleReadLoopInput` (shared read loop; covers the control-mode and hub `onInput` closures) | when the `onInput` closure returns true, after the closure's own gates | writer present, `CanSendCommand` |
| `shell` | `handleShellInput` after `forwardShellInput` | when `forwardShellInput` returns true | writer present, `CanSendCommand` |
| `capture` | `handleCapturePaneInput` after `forwardCapturePaneInput` | when `forwardCapturePaneInput` returns true | writer present, `CanSendCommand` |
| `stream` | `StreamTerminal` (`session_service_stream_terminal.go`) | after the PTY write succeeds | writer present |

For `cm`, `shell` and `capture`, a `SendInput` error is swallowed (logged at most) and the frame still counts as `forwarded`: the stream is kept alive and keystrokes may be lost under load, so `forwarded` is "issued", not "delivered". `stream` is the exception: a write error ends the stream before `forwarded` is recorded. The terminal mux bridge reaches one of these sites, so mux input is counted under `cm`.

## Metrics

Server (ADR-005; class-only, bounded labels, never a session id or content):

| Series | Labels |
|---|---|
| `terminal_input_frames_total` | `kind` (`text`, `escape`, `other`), `listener` (`local`, `remote`, `unknown`), `path` (`cm`, `shell`, `capture`, `stream`), `stage` (`received`, `forwarded`) |
| `terminal_input_bytes_total` | same labels |

`kind` comes from the first byte plus a scan of at most the first 64 bytes: ESC first is `escape`; printable UTF-8 plus `\r \t \n \x7f` is `text`; anything else is `other`.

Browser (via `POST /api/analytics`, allow-listed in `server/analytics/client_metrics.go`):

| Event | Series | Labels |
|---|---|---|
| `cm.input` | `client_input_total` | `event` (`emitted`, `queued`, `dropped_disconnected`), `renderer` (`xterm`, `ghostty`) |
| `cm.ime_anomaly` | `client_ime_anomaly_total` | `kind` (the `ImeAnomaly` union), `renderer` |

Requirement names map as follows: `input_dropped_disconnected_total` is `client_input_total{event="dropped_disconnected"}`; `ime_event_sequence_unrecognized_total` is `client_ime_anomaly_total{kind="unrecognized_sequence"}`.

Sender behavior (`mobileInput/telemetry.ts`): per-unit counts are summed and sent as one beacon per event class per 10 s with a `count` label (the server caps it at 1000); anomalies are limited to 1 per kind per 10 s with a burst of 5 per tab (token bucket, injected clock); a throwing `track` never disturbs typing. Dashboard: `ssq-client-network`, row "Phone terminal input", panels "Terminal input" and "Phone input anomalies and drops". There is no alert rule in this repo; see `project_plans/mobile-input-layer/requirements.md` (OBS-4).

### Actuator

`GET /actuator/terminal-input` (JSON; `?format=text` for a terminal) returns the bounded per-session tally (LRU of 256 sessions): `sessionId`, `frames`, `bytes`, `byKind`, `byPath` (`{received, forwarded}` per path) and `lastAt`. It carries no content field. Registered beside `/actuator/ws-unary` in `server/actuator.go`.

### Logs

Info on the first input per stream and a summary at most once per minute per stream; frame counts and classes only, never content.

## Fixtures and privacy

Replay fixtures (`mobileInput/__fixtures__/`, schema in `fixtureSchema.ts`) have a `source`. A `device` fixture must carry `scrubbed: true` and a `signoff`; both are self-attested, so `fixtureLint.test.ts` is a soft guard, not proof that a trace is clean (ADR-007, ADR-008). Review the diff of any new device fixture by hand.

## Secret entry limitation

The layer cannot detect an echo-off prompt, so characters typed at a password prompt travel through the same path as any other text. The soft keyboard's suggestion strip, autocorrect and clipboard history can retain what is typed, and TalkBack reads typed characters aloud through the polite live region (the same limitation as ADR-008). Do not type secrets on the phone through this layer; use a hardware keyboard or paste from a password manager.

## Related

- Debug procedure: `docs/how-to/debug-phone-typing.md`
- Browser telemetry: `docs/reference/client-network-telemetry.md`
- Plan and validation: `project_plans/mobile-input-layer/implementation/plan.md`, `validation.md`
