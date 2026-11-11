# Debug phone typing

Use this when typing on the Android phone loses, duplicates or garbles characters, or nothing appears. Background and the full metric list: `docs/reference/mobile-input-layer.md`.

The layer attaches only on an Android touch phone, so reproduce on the phone (or Pixel 7 emulation in Chrome DevTools). Each step below names a real URL or command.

## 1. Confirm what the phone is running

1. Open the app with `?inputdebug=1`, for example `https://onyx.staplerhome.internal:8444/?inputdebug=1`.
2. The "Input diagnostics" dialog lists a **Build** id, **Renderer**, **Layer flag**, **Stream** state and **Keyboard** hint.
3. If the build id is not the one you deployed, hard reload (a cached bundle is the usual cause) and check again before testing anything else. A service restart (`make install-service`) is a transport boundary, so do not redeploy in the middle of a composition.

`Layer flag: off` explains the whole symptom: the layer is not attached. Go to "Rollback" below.

## 2. Read the flag-off hint

When the layer is off on an Android phone, the terminal shows a one-line status: "Phone keyboard support is off, so typing may not work well. You can turn it on in the feature flags panel." Turn it on there, or use the query override in the next section.

## 3. Rollback and the `?mobile_input=0|1` override

`mobile_input_layer` is a rollback switch, not a user setting. Two ways to flip it:

- Feature-flag panel: turn `mobile_input_layer` off. The page stores the new value on one reload and applies it on the next, so reload twice.
- Query override, applied in the same load: open `?mobile_input=0` (off) or `?mobile_input=1` (on). It is stored per origin in `localStorage`, so repeat it on every origin you use: `:8543`, `:8444`, the Tailscale host and the LAN IP. Flipping the flag in the panel clears the override.

The known-good fallback state is the layer off, which restores the plain terminal behavior. With the layer off, ghostty is not used on Android phones either, so the fallback is xterm.js. Turning `ghostty_renderer` off alone does not fix typing: the layer owns input under both renderers (requirements Open Questions).

Hardware and Bluetooth keyboards: the attach test requires a touch-primary device with no hover capability, so a device that reports a mouse or hover pointer (a DeX or desktop-mode session, for example) does not attach the layer. That is expected, not a bug. Physical-key handling while attached is in `mobileInput/physicalKeys.ts`; its on-device behavior is not yet verified.

## 4. Diagnostics panel and Copy report

Entry paths, in order: `?inputdebug=1` (available today), then the settings and toolbar entries and the long-press shortcut on the connection chip **(planned, PR-C)**.

In the panel, type a few characters and press **Copy report**. The report is content-free: it lists event source, class (`text`, `control`, `escape`, `space`, `empty`) and length, never the characters themselves. If the clipboard is blocked, the report text is shown selectable instead. Paste it into the issue or chat. Capture-for-fixture and Reveal are **(planned, PR-C)**.

## 5. Did the keystrokes reach the server?

Compare what the browser says it sent with what the server received. On the same host as the service:

```bash
curl -s "http://localhost:8543/actuator/terminal-input?format=text"
```

Each line is one session: `frames`, `bytes`, `text`/`escape`/`other` counts and `last=`. For per-path detail use the JSON form (`curl -s http://localhost:8543/actuator/terminal-input`) and read `byPath`: each path has `received` and `forwarded`.

Interpret it:

| You see | Meaning |
|---|---|
| Panel shows `text(1)` events, `received` stays flat | Frames never arrived: transport problem (WebSocket blocked or reconnecting). Check the **Stream** field and `client_input_total{event="dropped_disconnected"}`. |
| `received` rises, `forwarded` stays flat | The server saw the frames and refused them: the attach is read-only or the session lacks send permission. |
| `received` and `forwarded` both rise but the text is wrong | Delivery is fine; the bug is in the client rewrite. Copy the report and check the anomaly kinds below. |
| Panel shows no events at all | The layer is off or not attached (step 1) or the keyboard bypassed `beforeinput`. |

The same comparison over time is the **Terminal input** panel on the `ssq-client-network` Grafana dashboard: client `client_input_total{event="emitted"}` against `terminal_input_frames_total{kind="text",stage="received"}` (use `listener="remote"` for the `:8444` listener) and `stage="forwarded"`. The companion panel **Phone input anomalies and drops** graphs `client_ime_anomaly_total` by `kind` (`orphaned_input`, `rewrite_bound_exceeded`, `unrecognized_sequence`, `stuck_composition`, `selection_in_region`, `duplicate_suppressed`) and units queued or dropped while disconnected. Browser numbers are aggregated units and server numbers are frames, so compare whether each line moves, not the ratio. There is no alert rule yet; see OBS-4 in `project_plans/mobile-input-layer/requirements.md`.

## 6. Test on a manual instance without touching the live one

```bash
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1 \
  ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server
# phone or emulation: http://<host>:62871/?inputdebug=1
curl -s "http://localhost:62871/actuator/terminal-input?format=text"
```

Build and cleanup steps are in the repo CLAUDE.md under "Manual/interactive testing".

## Rollback summary

Layer misbehaves on one phone: `?mobile_input=0` on each origin. Layer misbehaves for everyone: turn `mobile_input_layer` off in the feature-flag panel (two reloads to apply). Neither needs a rebuild.
