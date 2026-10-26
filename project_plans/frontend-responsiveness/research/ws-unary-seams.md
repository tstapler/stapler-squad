# WS unary seams (Task 5.1.1a spike)

Proof: `server/ws_unary_spike_test.go` (`package server`, throwaway; Task 5.2.1d promotes or deletes it).
Run: `go test ./server -run 'TestSpike_' -v -count=1` -> all 4 tests PASS (2026-10-26).

## Dispatch seam (VERIFIED by test)

| Item | Finding |
|---|---|
| Local composition | `localChainWith` (`server/guarded_procedures.go:95-108`): `stampRequest(local)` -> `HostGuard` -> (`authMiddleware` or `LocalWriteGuard(guardedProcedures)`) -> `Server` (mux), wrapped outermost by `Logging(CORS(Compress(...)))`. |
| Remote composition | `remoteChain` (`server/server.go:1941-1948`): `stampRequest(remote)` -> `authMW` -> `Server`, same outer wrappers. `StartRemote` adds `otelhttp` outside (`server.go:1972-1974`); `Start` does the same (`server.go:1603-1607`). |
| Split needed (Task 5.2.1f) | Expose the stamp -> guards -> mux composition without Logging/CORS/Compress. The spike builds it in-test (`spikeLocalDispatcher`, `spikeRemoteDispatcher`) with the same unexported pieces (`stampRequest`, `hostGuardConfig`, `probeGuardConfig`, `LocalWriteVerdictConfig`); both the split form and the full `localChain()`/`remoteChain()` give identical results. |
| Listener selection | `services.RequestRecordFrom(ctx)` returns `(RequestRecord, bool)`; `.Listener` was `local` / `remote` inside the real handler for the matching dispatcher. The bridge selects its dispatcher from that. |
| Synthesized request | `POST /api/<pkg.Service>/<Method>`, body = raw proto, `Content-Type: application/proto`, `Accept-Encoding: identity`, `Connect-Protocol-Version: 1`, and `Host`, `RemoteAddr`, `Origin`, `Cookie` copied from the upgrade request. Reaches `GetVCSStatus` and decodes back to the expected proto (`id=s1`). |
| Content type | Unary: `application/proto`, raw body, HTTP status + JSON error. The streaming bridge uses `application/connect+proto` with envelopes (`server/services/ws_stream_bridge.go:94-98`) and copies only `Authorization`, `Cookie`, `Connect-Protocol-Version` (lines 85-92), so it is not a precedent for guards. |
| Guard parity | `ProbeProgram` (in `guardedProcedures`) through the synthesized path vs a real HTTP request to `localChain()`: foreign Origin 403/403, rebinding Host 403/403, same-origin 501/501 (guard passed, stub unimplemented). The guard log shows the carried `Host`/`Origin`/`RemoteAddr`. |

## Remote listener (a)-(d)

- (a) VERIFIED: via `remoteChain(middleware.Auth(v), true)` and the split form, a request with the upgrade's `Cookie` (`cs_auth`) returns 200 and the handler sees `Listener == remote`.
- (b) PARTIAL: `GetOrigins()` is empty on a fresh server; the remote origin list is only appended at `main.go:1626` (`https://<rpID>:<port>`, `https://<hostname>:<port>`, `https://localhost:<port>`, read at HEAD). `GetHostnames()` after `SetHostnames` returns the merged hostnames (tested). The real values on the owner's remote-access run are UNVERIFIED (need a live remote-access instance); Task 5.2.1g must write its allow-list against the live `/api/server-info` or startup log on such a run.
- (c) VERIFIED: an unauthenticated WebSocket upgrade through `otelhttp -> remoteChain(Auth)` is answered 401 and the bridge handler never runs (counter 0); with the cookie the upgrade succeeds.
- (d) VERIFIED (plain HTTP/1.1 listener in the test): `otelhttp -> Logging -> CORS -> Compress -> Auth` preserve `http.Hijacker` (the handler's type assertion succeeded and a real gorilla upgrade worked). UNVERIFIED: the same over the real TLS/h2 `StartRemote` listener (browsers open WS on an HTTP/1.1 connection, so h2 hijack is not expected to matter; not run).
- Not covered by the spike: the `AuthMode == AuthModeRequired` refusal (ADR-002 item 7) is a new bridge check, built and tested in Task 5.2.1g.

## Fixed-TTL auth (VERIFIED)

`middleware.Auth` never calls `SetCookie` (no `Set-Cookie` header on 200 responses in either chain; grep of `SetCookie` shows only the login handlers). A request after the validator's token expires gets 401 on the same chain. The test uses a switchable validator as the clock stand-in; `SessionManager.ValidateAuthSession` deletes expired tokens on each call (`server/auth/session.go:195-208`), so expiry is per request.

## ADR outcome

ADR-001 and ADR-002 are Accepted (no amendment needed). ADR-003 stays Proposed: it is confirmed by measurement in Stories 5.1.2, 5.5.1, 5.5.2.
