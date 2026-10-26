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

### Re-run on rebased head (f7e65cabe, main + #1192), 2026-10-26

Branch rebased onto main after #1192 landed the same page-target fix; `manager.go` is now main's, this branch adds tests and this doc. Isolated instance `PORT=62873 STAPLER_SQUAD_INSTANCE=cdp-screencast-proof STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-2`, session `ad25fe4e-...`, headless Chrome (`google-chrome-stable`) with `--remote-debugging-port=$CDP_PORT`, page "CDP LIVE PROOF 3" on orange.

- `GetSession` returned `"cdpState":{"status":"CDP_STATUS_STREAMING"}` on 6 of 6 polls.
- Opened the session's **Browser tab** in the real UI: the `Remote browser display` canvas (780x441) had 6880/6880 sampled pixels non-white and renders the orange "CDP LIVE PROOF 3" page.

Main's manager no longer logs the attach / first-frame lines, so the Streaming state itself (published only after a frame is stored) is the evidence.

## Gaps

1. macOS was not run, since this host is Linux; owner-side.
2. The cdp-bins PATH wrapper is still bypassed by shell rc files: `which google-chrome-stable` resolves to `/usr/bin`.
