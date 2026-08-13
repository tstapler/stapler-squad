# ADR-017: CDP Port Allocation — OS-Assigned Port with TOCTOU Trade-off

## Status
Accepted

## Context

Chrome's `--remote-debugging-port=<N>` flag requires the port number to be known at Chrome launch time — it is passed as a command-line argument, not negotiated after startup. This means the port must be chosen before the Chrome process is started.

### Requirement

The port must be:
1. Free at the time Chrome starts (Chrome will fail to bind an already-occupied port and exit with an error).
2. Bound to `127.0.0.1` only (security requirement from ADR-014's proxy-only model).
3. OS-assigned (not a fixed port like `9222`) to avoid collisions when multiple sessions are created concurrently or when Chrome is restarted after a crash.

### Options Considered

**Option A: `net.Listen("tcp", "127.0.0.1:0")`, record port, close listener, pass to Chrome (Chosen)**

The Go standard library's `net.Listen` with port `0` asks the OS kernel to assign a free ephemeral port. The assigned port is read from `listener.Addr()`, the listener is closed, and the port number is written into the Chrome wrapper script.

```go
ln, err := net.Listen("tcp", "127.0.0.1:0")
// ...
port := ln.Addr().(*net.TCPAddr).Port
ln.Close()
// pass port to Chrome via --remote-debugging-port=<port>
```

This has a time-of-check/time-of-use (TOCTOU) window: between `ln.Close()` and Chrome's `bind(2)` call, another process could claim the port.

**Why the TOCTOU window is acceptable:**

- The port is on `127.0.0.1` (loopback) only. External processes on other hosts cannot race for it.
- On a typical developer workstation or small-team server, the volume of concurrent port-bind activity on loopback is negligible. The window is on the order of microseconds to low milliseconds.
- The stapler-squad process itself is the only entity creating CDP sessions; no other process in the system is competing for the same OS-assigned ephemeral ports.
- On failure (Chrome fails to bind), `CDPStreamManager.Start()` retries up to 3 times, each time calling `Allocate()` to obtain a fresh port. This retry loop handles the rare collision without surfacing it to the user.

**Option B: Pass the open file descriptor to Chrome**

Keep `ln` open and pass the file descriptor to Chrome via `--remote-debugging-fd=<fd>` (if such a flag existed). Chrome would inherit the fd and bind to it without a TOCTOU window.

Chrome does not expose a `--remote-debugging-fd` flag. The `--remote-debugging-port` flag is the only supported mechanism. This option is not available.

**Option C: Fixed per-session port arithmetic (e.g., `9222 + slot`)**

Assign port `9222 + display_slot` to each session. Display slots are in the range `[100, 200)` (ADR-013), giving ports `9322`–`9422`.

This is rejected for the same reason fixed VNC port arithmetic was rejected in ADR-013 (§Display Number and Port Allocation): rapid session create/destroy cycles can leave ports in `TIME_WAIT` state, causing bind failures on slot reuse. The OS-assigned approach avoids this entirely.

**Option D: Reserve port via `SO_REUSEPORT` and coordinate with Chrome**

Set `SO_REUSEPORT` on the listener and keep it open while Chrome starts, relying on both the Go listener and Chrome to bind the same port. Chrome does not support `SO_REUSEPORT` coordination and will fail to start if the port is already bound by another socket (regardless of `SO_REUSEPORT` on that socket).

Not viable.

### Comparison

| Option | TOCTOU window | Requires Chrome flag support | Port collision risk | Retry needed |
|---|---|---|---|---|
| A: Listen+close (chosen) | Yes (microseconds) | No (`--remote-debugging-port`) | Very low | On collision only |
| B: fd passthrough | No | Yes (not available) | None | No |
| C: Fixed arithmetic | No | No | High (TIME_WAIT) | On TIME_WAIT only |
| D: SO_REUSEPORT | No | Yes (not available) | None | No |

## Decision

Use `net.Listen("tcp", "127.0.0.1:0")` to find a free port, close the listener, and pass the port to Chrome via `--remote-debugging-port=<N>`. Retry up to 3 times on Chrome startup failure to handle the rare TOCTOU collision.

The same OS-assigned port approach is used for VNC ports (ADR-013, §Display Number and Port Allocation). This ADR documents the same trade-off applied to CDP.

## Consequences

### Positive

- No fixed port assignment; no risk of `TIME_WAIT` collisions on rapid session cycling.
- Simple implementation using only the Go standard library.
- Consistent with the VNC port allocation strategy already in place.

### Negative / Constraints

- A TOCTOU race exists between `ln.Close()` and Chrome's `bind(2)`. This is accepted given the loopback-only binding and the retry loop.
- The retry loop (up to 3 attempts) adds a small amount of latency on the rare collision path. Each retry re-runs `Allocate()` which calls `net.Listen` + `Close` again — this is fast (sub-millisecond) and the retry adds no perceptible delay in practice.
- Port is recorded in `CDPStreamManager.port` (atomic int32). If `Allocate()` is called concurrently (which it is not — `sync.Once` guards it), only one port is recorded. This is correct behaviour.

## References

- ADR-013 §Display Number and Port Allocation (same pattern for VNC ports)
- ADR-014 (proxy-only auth; CDP port is localhost-only for the same reason)
- ADR-016 (CDP screencasting; this ADR covers the port allocation sub-decision)
- Implementation: `session/cdp/manager.go` (`Allocate()` method)
- Chrome DevTools Protocol launch flags: https://chromedevtools.github.io/devtools-protocol/#how-do-i-access-the-browser-target
