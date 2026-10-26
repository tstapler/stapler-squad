# Verification Evidence: CDP screencast page-target fix

Unit tests: `go test -race -count=3 ./session/cdp/` passes (`session/cdp/screencast_test.go`).

## Linux live run (real Chrome 146)

Isolated instance: `PORT=62873 STAPLER_SQUAD_INSTANCE=cdp-screencast-proof STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-2`, `browser-passthrough` flag on. Session `cdp-proof` created via `POST /api/session.v1.SessionService/CreateSession`; Chrome launched in its pane with `--remote-debugging-port=$CDP_PORT`.

`GetSession` before Chrome started, then after:

```
"cdpState":{"status":"CDP_STATUS_NO_BROWSER"}
"cdpState":{"status":"CDP_STATUS_STREAMING"}   (6 consecutive polls)
```

Server log (`instances/cdp-screencast-proof/logs/staplersquad.log`):

```
cdp: Chrome detected ws_url=ws://127.0.0.1:39041/devtools/browser/8cc62ac9-...
cdp: attaching to page target ws_url=ws://127.0.0.1:39041/devtools/page/39190D5B...
cdp: screencast started port=39041
cdp: first screencast frame stored
```

`/api/sessions/cdp-proof/cdp-stream` WebSocket delivered one binary message: 5515 bytes, JPEG magic `ff d8 ff`. Viewed: orange page reading "CDP LIVE PROOF" (non-blank).

### Re-run on lint-fix head (a134ebc9c), 2026-10-26

Same isolated setup (`PORT=62873`, session `4eb86825-...`, headless Chrome with `--remote-debugging-port=$CDP_PORT`). `GetSession` returned `"cdpState":{"status":"CDP_STATUS_STREAMING"}` on 4 consecutive polls. Log: `cdp: Chrome detected` → `cdp: attaching to page target ws_url=ws://127.0.0.1:42839/devtools/page/9A28...` → `cdp: screencast started` → `cdp: first screencast frame stored` (12 ms after attach). `cdp-stream` WebSocket: 1 binary message, 5789 bytes, magic `ff d8 ff`; decoded image is an orange page reading "CDP LIVE PROOF 2" (non-blank).

## Gaps

1. macOS was not run, since this host is Linux.
2. The UI Browser tab canvas was not opened: the tab is gated on `vncState`, which is unavailable without Xvfb/x11vnc, and the frontend fix (`backlog/stapler-squad-fix-browser-passthrough`) is not an ancestor of this branch. The `cdp-stream` WebSocket frame stands in for the canvas.
3. The cdp-bins PATH wrapper is still bypassed by shell rc files: `which google-chrome-stable` resolves to `/usr/bin`.
