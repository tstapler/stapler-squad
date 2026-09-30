# Capture the terminal stream to diagnose garbled output

The capture tap records what the server sees on the way to the browser terminal:
raw output, dropped frames (with the reason), resizes and snapshots. Use it on a
manual instance to find out where a garbled terminal lost bytes.

**It records raw terminal output, which can include passwords, tokens and API
keys.** Turn it on only for a session you are debugging, and delete the files
afterwards.

## Turn it on

Use a manual instance, never the live service (see "Manual/interactive testing"
in `CLAUDE.md`). Give each instance its own directory: two instances sharing one
append to the same files.

```bash
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test \
  STAPLER_SQUAD_CAPTURE_TAP_DIR=$HOME/.stapler-squad/tap-manual-1 \
  ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server
```

At startup the server logs a `capture tap ACTIVE` warning with the directory. It
does not log anything if the variable is unset.

## Turn it off

Unset `STAPLER_SQUAD_CAPTURE_TAP_DIR` and restart, then delete the directory.
Nothing rotates or expires the files.

## What it writes

- One file per session: `<dir>/<session>.jsonl`. A session name that needed
  characters replaced gets a short hash suffix so two names never share a file.
- The directory is created `0700` and files `0600`. An existing directory is
  tightened to `0700`. A symlink at the file path is refused.
- Each file stops at 64 MiB (logged once) and is closed. Writes are synchronous
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
