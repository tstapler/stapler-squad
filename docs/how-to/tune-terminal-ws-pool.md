# Turn on and tune the terminal WebSocket pool

1. Enable the flag: Settings -> Features -> `terminal_ws_pool`, then reload the page. (`curl -s -X POST localhost:8543/api/session.v1.SessionService/UpdateFeatureFlag -H 'Content-Type: application/json' -d '{"name":"terminal_ws_pool","enabled":true}'` does the same.)
2. Confirm it is active: DevTools -> Network -> WS shows one or two `/api/ws/terminal-mux` sockets and no per-terminal `StreamTerminal` sockets.
3. Pick a size. Start with the default 2. Use 1 for the fewest sockets (all terminals share one TCP stream, so one stalled network path delays all of them); raise it if the Grafana panel "Terminal mux: head-of-line wait p95" is high while "credit stalls" stays low.
   - Server-wide, at startup: `STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE=3`.
   - Live, no restart: `curl -X POST 'localhost:8543/actuator/terminal-mux?poolSize=3'` (loopback only; check with `curl localhost:8543/actuator/terminal-mux`). Browsers that did not pin a size follow it immediately.
   - One browser: `localStorage.setItem("ssq:terminal-pool-size", "1")` in the console, then reload.
4. Roll back: turn the flag off; new streams use per-terminal sockets within about 30 s, and open terminals switch when they reconnect or the page reloads (no restart). To also remove the endpoint, start the server with `STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE=0`.

Reference: [terminal-ws-pool](../reference/terminal-ws-pool.md).
