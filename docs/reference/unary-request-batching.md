# Unary request batching (`batch_unary`)

Feature flag `batch_unary`, default **off**, read by the web client from `GetFeatureFlags` (absent means off; applies on next page load).

When on, cheap idempotent reads issued within a 4 ms window are sent as one `POST /api/batch/unary` instead of one HTTP request each, which frees connection slots under HTTP/1.1's 6-connection cap. It does not change latency on localhost. Calls that already travel over the unary WebSocket (`unary_over_ws`) are never batched.

| Piece | File |
|---|---|
| Client decorator (window, caps, abort isolation, fallback) | `web-app/src/lib/api/batchTransport.ts` |
| Client eligibility list (`batchEligible`) | `web-app/src/lib/api/idempotentMethods.ts` |
| Server endpoint and deny-by-default `batchAllowed` set | `server/services/ws_unary_batch.go` |
| Shared allow-list fixture (client and server tests both check it) | `web-app/src/lib/transport/__fixtures__/ws-unary-allowlist.json` |

## Wire format

Request (JSON, at most 256 KiB, 1 to 16 calls, unique ids): `{"calls":[{"id":1,"method":"/session.v1.SessionService/ListShells","body":"<base64 proto>","timeoutMs":30000}]}`.

Response (always 200 for a well-formed envelope; malformed is 400, oversize is 413): `{"results":[{"id":1,"body":"<base64 proto>"} | {"id":2,"error":{"code":"not_found","message":"..."}} | {"id":3,"fallback":true}]}`. Results carry no ordering guarantee.

## Safety properties

- Every call re-enters the listener's full dispatcher (HostGuard, auth or LocalWriteGuard, interceptors), the same path the unary WebSocket bridge uses, so a batch never widens what a direct call may do.
- `batchAllowed` is a subset of `wsUnaryAllowed` and excludes `GetVCSStatus`, `GetSessionDiff` and `GetFileContent`: a batch answers only when every item is done, so slow or bulk calls travel alone.
- Server work is bounded: 6 concurrent calls per batch, 2 MiB per item, 8 MiB per response (items past either cap come back as `fallback`).
- The client falls back to individual calls on any batch-level failure (network error, non-200, malformed body) and for any item the server declines, so behaviour never regresses.
- Aborting one caller rejects only that caller. The in-flight request is cancelled only when every caller in it has aborted.

## Extending

Add the method to `batchAllowed` (Go), `batchEligible` and the fixture's `batchAllowed` (client). Both tests fail until the three agree. Only add reads that are cheap and not in `guardedProcedures`.
