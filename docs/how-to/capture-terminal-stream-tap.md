# Capture the terminal stream to diagnose garbled output

The capture tap records what the server sees on the way to the browser terminal:
raw output, dropped frames (with the reason), resizes and snapshots. Use it on a
manual instance to find out where a garbled terminal lost bytes.

**It records raw terminal output, which can include passwords, tokens and API
keys.** Turn it on only for a session you are debugging, and delete the files
afterwards.

## Turn it on at runtime

Call the `SetCaptureTap` RPC. No restart is needed: it takes effect on terminal
streams that are already running. Use a manual instance, never the live service
(see "Manual/interactive testing" in `CLAUDE.md`).

```bash
# One session (session_ids are the session names shown in the UI)
curl -sS -X POST http://localhost:62871/api/session.v1.SessionService/SetCaptureTap \
  -H 'Content-Type: application/json' \
  -d '{"enabled": true, "sessionIds": ["my-session"], "ttlSeconds": 600}'

# Every session (omit sessionIds)
curl -sS -X POST http://localhost:62871/api/session.v1.SessionService/SetCaptureTap \
  -H 'Content-Type: application/json' -d '{"enabled": true}'

# Check state, including per-session file paths and sizes
curl -sS -X POST http://localhost:62871/api/session.v1.SessionService/GetCaptureTap \
  -H 'Content-Type: application/json' -d '{}'
```

Rules:

- **Loopback only.** Both RPCs are rejected with `PermissionDenied` unless the
  TCP peer is `127.0.0.1` or `::1`, even with `--remote-access`. A request
  carrying a proxy header (`X-Forwarded-For`, `Forwarded`, `X-Real-Ip`, `Via`,
  `X-Forwarded-Host`, `X-Forwarded-Proto`) or a non-loopback `Origin` is
  rejected too, because a local reverse proxy or tunnel connects from loopback.
- **Every enable expires.** `ttlSeconds` defaults to 1800 (30 minutes) and is
  clamped to 14400 (4 hours). When it ends the tap turns itself off and logs a
  warning. Enable again to extend it.
- **The directory is fixed by the server**, never by the caller: `<state
  dir>/tap` (so `STAPLER_SQUAD_INSTANCE` and workspace isolation apply), or
  `STAPLER_SQUAD_CAPTURE_TAP_DIR` if set.
- Every change is logged at Warn with the peer address and scope.

## Turn it off

```bash
curl -sS -X POST http://localhost:62871/api/session.v1.SessionService/SetCaptureTap \
  -H 'Content-Type: application/json' -d '{"enabled": false}'
```

With no `sessionIds` this stops every session and closes the files. To stop only
named sessions, send their `sessionIds`; that is rejected while the tap is on
for all sessions. Then delete the files. Nothing rotates them, and turning the
tap back on appends to the same files.

## Enable at startup (operator override)

Setting `STAPLER_SQUAD_CAPTURE_TAP_DIR` starts the tap on for all sessions with
no TTL and writes to that directory. Give each instance its own directory: two
instances sharing one append to the same files. The server logs a `capture tap
ACTIVE` warning when it first builds the registry. A runtime `enabled: false`
still turns it off.

## What it writes

- One file per session, opened on the first write after the tap is enabled:
  `<dir>/<session>.jsonl`. A session name that needed
  characters replaced gets a short hash suffix so two names never share a file.
- The directory is created `0700` and files `0600`. An existing directory is
  tightened to `0700`. A symlink at the file path is refused.
- Each file stops at 64 MiB (logged once) and is closed; `GetCaptureTap` reports
  it as `capped`, and enabling again clears the flag (the cap counts the whole
  file, so a full file stops again at once). Writes are synchronous
  on the output path, so use a local disk.
- Each line is `{"t_ns","kind","src","cause","cols","rows","b64"}`:
  - `kind`: `output`, `drop`, `resize` or `snapshot`.
  - `src`: `hub` or `legacy`. Filter on it, because a session served by several
    connections on the legacy path is recorded once per connection.
  - `cause` (drops only): `resize_settling`, `forwarding_not_ready`, or
    `subscriber_undelivered` (a subscriber's queue was full or closed; one record
    per subscriber).
  - `b64`: the bytes, base64-encoded.

## What it does not cover

- `output` means bytes entered the hub or forwarder, not that a client received
  them. Delivery failures show up only as `subscriber_undelivered` drops.
- `resize` and `snapshot` records come from the hub path only, so settle latency
  (`streamhub.PairSettleLatency`) is measurable only there. The hub is the
  default path.
- The shell-stream forwarder is not tapped.
- Legacy `output` records are post-coalesce buffers; hub records are pre-batch
  frames, so the two are not byte-for-byte comparable.
