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
# One session (sessionIds are session titles, as shown in the UI; not tmux names)
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
  TCP peer is `127.0.0.1` or `::1`, even with `--remote-access`, and the request
  `Host` is a loopback name (`localhost`, `127.0.0.1`, `[::1]`). A local reverse
  proxy or tunnel connects from loopback but forwards its public `Host`, so it
  is rejected. So is a request carrying a proxy header (`Forwarded`,
  `X-Forwarded-*`, `X-Original-Forwarded-For`, `X-Real-Ip`, `X-Client-Ip`,
  `True-Client-Ip`, `CF-Connecting-IP`, `Cdn-Loop`, `Via`, any `Tailscale-*`).
  A browser `Origin`, when present, must match the `Host` exactly; requests with
  no `Origin` (curl, scripts) are allowed.
- **Session ids** are session titles, at most 200 bytes, with no control
  characters. The hub and legacy stream paths share one file per session because
  both key on the title.
- **Every enable expires.** `ttlSeconds` defaults to 1800 (30 minutes) and is
  clamped to 14400 (4 hours). When it ends the tap turns itself off and logs a
  warning. Enable again to extend it. One timer tracks the earliest deadline.
  Enabling a named session while all sessions are on keeps the later deadline.
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
for all sessions. Disabling never waits on a slow disk: an in-flight write
finishes, then the file closes. Then delete the files. Turning the tap back on
appends to the same file, unless it was full (see below).

## Enable at startup (operator override)

Setting `STAPLER_SQUAD_CAPTURE_TAP_DIR` starts the tap on for all sessions with
no TTL and writes to that directory. Give each instance its own directory: two
instances sharing one append to the same files. The server logs a `capture tap
ACTIVE` warning at startup. At runtime, `enabled: true` for all sessions
replaces the no-expiry with the TTL, and `enabled: false` for all sessions
turns the tap off until restart. If the directory already exists it must be
owned by the server's user and not group- or world-writable; it is never
chmodded. If it does not exist it is created `0700`.

## What it writes

- One file per session, opened on the first write after the tap is enabled:
  `<dir>/<session>.jsonl`. A session name that needed
  characters replaced gets a short hash suffix so two names never share a file.
- A missing directory is created `0700` and files are `0600`. An existing
  directory is validated (owned by the server's user, not group- or
  world-writable), not chmodded; a directory that fails is refused and the tap
  reports `failed`. If the state directory cannot be resolved the tap is
  unavailable (there is no temp-dir fallback). A symlink at the file path is
  refused. On open, a file whose last line was torn (no trailing newline) gets
  one so later records stay readable.
- Each file stops at 64 MiB (logged once) and is closed; `GetCaptureTap` reports
  it as `capped`. Enabling again rotates the full file to `<name>.jsonl.old`
  (replacing any earlier `.old`) and records into a fresh file, reported as
  `rotated`, so a session has at most two files. Writes are synchronous
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

## Limits

- The registry keeps one small handle per session title it has seen, until the
  server restarts. Nothing prunes idle handles, so memory grows with the number
  of distinct sessions streamed.
- Only 64 MiB x 2 per session is kept, but there is no total cap across sessions:
  enabling all sessions on a busy server can fill the disk within the TTL.
- Turning the tap off is not instantaneous for a write already in flight: at
  most one more record per in-flight writer can land after `enabled: false`
  returns, then the file closes.
- Closing a file happens while the registry lock is held, so on a hung network
  or FUSE mount a slow `Close` can stall the RPCs and the first stream of a new
  session (never the output path of a stream already running). Use a local disk.
- On a case-insensitive filesystem (the macOS default) titles that differ only
  in case, such as `Foo` and `foo`, share one file.
