# ADR-014: VNC Authentication — Proxy-Only, No x11vnc Password

## Status
Accepted

## Context

FR-2 and NFR-2 require that VNC access be authenticated and that VNC ports not be exposed outside `127.0.0.1`. Three authentication models were evaluated.

### The RFB Password Problem

The RFB 3.x protocol (used by x11vnc and all standard VNC servers) authenticates via a DES-based challenge-response scheme (VNC Authentication type 2). DES keys are 56 bits, derived from an 8-byte input. **Any password longer than 8 characters is silently truncated** — extra characters are simply ignored by the DES key schedule.

This means a 32-character random token (which would provide ~190 bits of entropy) is reduced to 8 characters (~48 bits of DES entropy, with known key schedule weaknesses). RFB 3.x VNC auth cannot be made strong regardless of password length.

### Options Considered

**Option A: x11vnc `-nopw -localhost`, Go proxy as sole auth gate (Recommended)**

x11vnc is launched with:
- `-nopw` — no VNC-level password required
- `-localhost` — refuses connections from any non-loopback address

All access to the VNC port goes through the Go WebSocket proxy at `/api/sessions/{id}/vnc`. The proxy sits behind the existing `server/middleware/auth.go` middleware chain, which validates the stapler-squad session cookie or Bearer token before the WebSocket upgrade completes. An unauthenticated request receives HTTP 401 before any VNC bytes are exchanged.

Per-session isolation is enforced at the proxy layer: the session ID in the URL path is used to look up the VNC port in `VNCProcessManager`; a valid token for session A cannot access session B's display.

This design makes VNC-level auth entirely redundant — the `127.0.0.1`-only binding ensures the VNC port is physically unreachable without going through the proxy, and the proxy enforces the same session auth used for terminal access and all other API calls.

**Option B: x11vnc `-rfbauth <passfile>` with per-session random 8-char token**

x11vnc's `-rfbauth` flag accepts an htpasswd-format file. A random 8-character ASCII token could be generated per session and written to a temp file (mode 0600) owned by the stapler-squad process.

The noVNC client would receive the token from the `VNCState.vnc_password` field in `GetSession` and pass it to `new RFB(canvas, url, { credentials: { password: token } })`.

This adds a second authentication layer. However, the 8-character ceiling is a hard protocol constraint — 8 printable ASCII characters is ~52 bits of entropy at best, equivalent to a strong PIN, not a cryptographic secret. If the Go proxy is already enforcing session auth, the marginal security value of this second layer is low relative to the added complexity (token generation, temp file lifecycle, cleanup, rotation on session restart).

The Go proxy handles auth correctly and completely. Adding a weak VNC password on top does not meaningfully improve the security posture and introduces an additional failure mode (x11vnc fails to start if the passfile is unreadable or malformed).

**Option C: SSH tunnel through the stapler-squad host**

The browser noVNC client could connect via an SSH tunnel rather than the Go WebSocket proxy. This would provide strong cryptographic auth (SSH keys or certificates) without the 8-char RFB limitation.

This option requires:
- The browser client to establish an SSH tunnel, which is not natively possible in JavaScript without a helper process or WASM SSH implementation.
- A separate SSH server process per session or a multiplexed SSH tunnel.

The complexity is wholly disproportionate to the security gain over the proxy-auth model. Eliminated.

### Security Posture Summary

With the proxy-only model:
- VNC port bound to `127.0.0.1` only — not reachable by any process on another host or in a container without explicit port-forwarding.
- stapler-squad session token required to upgrade to WebSocket — same strength as all other API auth.
- Per-session proxy lookup ensures display isolation even if two sessions' cookies are both valid.
- No VNC-level credential to rotate, leak, or manage.

The threat model for which RFB auth would add value — an attacker who can reach `127.0.0.1:<port>` on the host but cannot intercept the stapler-squad session cookie — is not a realistic attack vector for the target deployment (single-user workstations and small team servers on a LAN or Tailscale VPN).

## Decision

Launch x11vnc with **`-nopw -localhost`**. All VNC access is brokered exclusively through the authenticated Go WebSocket proxy at `/api/sessions/{id}/vnc`.

- No VNC-level password is generated, stored, or transmitted.
- The proxy endpoint is covered by `server/middleware/auth.go` automatically (all `/api/…` paths go through it).
- The proxy looks up the per-session VNC port via `VNCProcessManager.GetPort(sessionID)` — a valid token for one session cannot reach another session's port.
- The noVNC `RFB` constructor is called without a `credentials` option.

## Consequences

### Positive

- Eliminates the complexity of per-session VNC token generation, passfile lifecycle, temp-file cleanup, and noVNC credential passing.
- Auth strength is determined by the stapler-squad session token, not by the 8-character DES ceiling.
- The proxy's session-to-port lookup provides display isolation as a side effect of the existing session model.
- No VNC-level credential to rotate on session restart.

### Negative / Constraints

- Defense-in-depth is reduced: if the Go proxy has a vulnerability that allows auth bypass, the VNC port has no second lock. This is accepted — the proxy is the same auth layer protecting the terminal and all other session data, so VNC is no weaker than the rest of the system.
- The `VNCState.vnc_password` field defined in the proto schema (`architecture.md §5`) is not used in this design. It should be omitted from `GetSession` responses or documented as reserved for future use.
- x11vnc's `-nopw` flag may log a warning on startup ("WARNING: you are using -nopw"). This is expected and should be suppressed in stapler-squad's log output by filtering x11vnc stderr.

## References

- Requirements: `project_plans/browser-passthrough/requirements.md` (FR-2, NFR-2)
- Pitfalls: `project_plans/browser-passthrough/research/pitfalls.md` §1.1
- Stack research: `project_plans/browser-passthrough/research/stack.md` §2 (VNC auth row), §4
- Architecture: `project_plans/browser-passthrough/research/architecture.md` §2 (auth middleware), §4
- RFB specification (password truncation): RFC 6143 §7.2.2
