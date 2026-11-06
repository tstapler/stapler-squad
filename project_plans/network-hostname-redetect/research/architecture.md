# Architecture Research: network-hostname-redetect

## Current data flow (file:line)

1. **Boot-time detection** — `main.go:329-339` (main server start) and `main.go:735-747`
   (setup-token/QR CLI subcommand, a *separate one-shot invocation of the binary*, out of
   scope for a runtime loop): `detectLANIPs()` → for each IP, `resolveLANHostnames(ip)` →
   dedup into `[]string hostnames`.
2. **Publish** — `main.go:434` calls `srv.SetHostnames(hostnames)` once, during the
   `"runtime"` `warren.App.Phase` (`main.go:362`). `Server.SetHostnames`/`GetHostnames`
   (`server/server.go:1510-1518`) are a bare `[]string` field with no mutex, no atomic — a
   plain unsynchronized read/write pair. This is the field the requirements call out as
   needing thread-safety.
3. **TLS SANs** — `startRemoteAccess()` (`main.go:1271`) reads `srv.GetHostnames()` once
   (`main.go:1280`), builds a `networks map[string][]string` (one SAN list per LAN IP,
   `main.go:1288-1293`), and calls `server.EnsureNetworkTLSCerts(networks)`
   (`server/tls.go:58`) to get back `certs map[string]*NetworkCert`. That map is closed over
   by `server.GetCertificateByLocalAddr(certs)` (`server/tls.go:128-142`), which becomes
   `tls.Config.GetCertificate` (`main.go:1300-1303`).
4. **WebAuthn RPIDs** — still inside `startRemoteAccess`, boot-time `hostnames` are
   re-verified via a synchronous forward-DNS `hostnameValidator` closure (`main.go:1312-1329`,
   1337-1346) before becoming `allRPIDs`/`origins`, passed into
   `serverauth.NewHandler(allRPIDs, origins, store, sessions, hostnameValidator)`
   (`main.go:1401`). `Handler` (`server/auth/webauthn.go:18-41`) stores `rpIDs`/`webauthn map`
   under `sync.RWMutex mu`; `webauthnForHost` (`webauthn.go:93-155`) already does exactly the
   add-only, lazy, per-request registration the requirements describe as today's only dynamic
   behavior — locked, race-free, but reactive (fires on request, not on network change) and
   guarded by `negativeHostnameCache`/`sourceIPLimiter` (`server/auth/hostname_guard.go`).

## Answering the Phase-2 open questions directly

**"Is the TLS cert a static `tls.Config.Certificates` or a `GetCertificate` callback?"**
It is **already a `GetCertificate` callback** (`main.go:1301`, `server/tls.go:128`), keyed by
the local IP the connection was accepted on. `EnsureNetworkTLSCerts`'s doc comment
(`server/tls.go:48-51`) explicitly designs for this: "adding, removing, or renaming one
network never forces regeneration of another network's cert." So dynamic SAN growth is
architecturally supported today — the Rabbit Hole's pessimistic branch (b) does not apply.
The actual gap: the `certs map[string]*NetworkCert` captured by the `GetCertificateByLocalAddr`
closure is a **plain, unsynchronized map** — reading it from the TLS handshake goroutine while
a hostname-detector goroutine inserts a new network's cert is the same unsynchronized-mutation
hazard as `Server.hostnames`, just in a different location. Adding a network at runtime is a
call to `EnsureNetworkTLSCerts` for the new IP/hostname set plus a synchronized swap into
whatever `GetCertificateByLocalAddr` closes over — not a redesign.

**"Is there a maintained low-weight Go library for cross-platform network-change
notification?"** Not investigated in this pass (no existing repo dependency found via
`go.mod` grep for netlink/route-monitor libraries); flagged for Phase 3 planning per the
requirements' own framing — this research focused on the seam/observer question, which is
answerable from the code alone.

## Recommended architecture: `HostnameDetector` mirroring `HostAdvertiser`, publish via `atomic.Pointer`

**Pattern: a small owning type with a `Run(ctx)` background loop (ticker + OS-event channel),
publishing an immutable snapshot via `atomic.Pointer`, not a channel/observer/pub-sub
pattern.** Two independent seams point the same way:

1. **This repo already has the exact lifecycle shape needed.**
   `session/host_advertiser.go`'s `HostAdvertiser.Run(ctx)` (`host_advertiser.go:75-91`) is a
   `time.NewTicker`-driven `select { case <-ctx.Done(): ...; case <-ticker.C: ... }` loop,
   started as a background goroutine from `main.go`'s `warren.App` runtime phase and
   cancelled via the app's shared `ctx`. A `HostnameDetector.Run(ctx)` should be the same
   shape, with one more `case` for the OS network-change event channel (whatever the Phase-3
   platform shim produces) alongside the ticker case — both branches call the same internal
   "redetect and publish" method, satisfying the debounce requirement by construction (a
   single `select` loop naturally serializes concurrent ticker/event firings; an explicit
   debounce timer can sit inside that method if bursts of OS events prove to be a problem
   in practice).
2. **`atomic.Pointer` copy-on-write is this repo's established convention for exactly this
   read-heavy/write-rare shape**, not an isolated choice: `session/instance.go:557`
   (`Instance.snapshot`), `github/rate_limit.go:61`, `log/log.go:156,168`,
   `session/claude_controller.go:145-150`, and `server/server.go:54` (`addr
   atomic.Pointer[string]`) all use it, and the project has a standing rule
   (`.claude/rules/instance-lock-free-reads.md`) codifying "publish via atomic snapshot, read
   without a lock" as the house style for mutable state read from multiple goroutines. A
   `HostnameSet` (or similar) held as `atomic.Pointer[HostnameSnapshot]` on `Server` — replacing
   the bare `[]string hostnames` field at `server/server.go:1510-1518` — is the smallest change
   consistent with that convention: `SetHostnames` becomes a `Store()` of a new slice (built by
   copy-on-write union with the previous snapshot, to satisfy add-only semantics), `GetHostnames`
   becomes a lock-free `Load()`. No new mutex, no new dependency, and it slots into the same
   review mental model `Snapshot()`-reading contributors already have.

   An explicit observer/callback/pub-sub layer (each consumer registers a callback invoked on
   change) was considered and rejected: there is no existing pub/sub primitive in this codebase
   for in-process state fan-out (the one gossip mechanism, `HostAdvertiser`, is a *network*
   protocol for advertising this host to *other* hosts — unrelated, and explicitly out of scope
   per the requirements' own text). Introducing one now for three known consumers (`Server`,
   WebAuthn `Handler`, TLS cert map) adds an abstraction layer repo conventions don't otherwise
   use, for no benefit over each consumer independently `Load()`-ing the latest snapshot when it
   needs it — WebAuthn's `webauthnForHost` and the TLS `GetCertificate` callback are already
   *pull*-based (invoked per-request/per-handshake), not push-subscribers, so there is nothing
   for a push notification to trigger.

### Concrete wiring for the three consumers

- **(a) `Server.hostnames`** — direct: `HostnameDetector.Run` calls `srv.SetHostnames(...)`
  (or a new `srv.MergeHostnames(...)` that does the add-only union) each cycle; `Server`
  itself does the atomic-swap internally, as above.
- **(b) WebAuthn RPID set** — `webauthnForHost`'s existing add-only, mutex-guarded
  `h.webauthn[hostname] = wa; h.rpIDs = append(...)` path (`webauthn.go:126-154`) already does
  the right thing *reactively*. The detector's job is only to make sure a newly-detected
  hostname is available to be validated by `hostnameValidator` (already a live forward-DNS
  check, `main.go:1312-1329`, not dependent on the boot-time list) — no new push path into
  `Handler` is required. Optionally, `HostnameDetector` could also proactively call something
  like `Handler.RegisterHostname(hostname)` for a newly-detected name so it doesn't wait for a
  real request to arrive on it first; this is a small addition to `Handler`, not an
  architecture change, and can be decided in Phase 3 planning.
- **(c) TLS SANs** — the detector (or `startRemoteAccess`, listening to the detector) calls
  `EnsureNetworkTLSCerts` for the new network's IP/hostname set, producing an updated
  `certs map[string]*NetworkCert`, and swaps it into whatever `GetCertificateByLocalAddr`
  reads from — this needs `certs` itself moved from a bare closed-over map to an
  `atomic.Pointer[map[string]*NetworkCert]` (or a small wrapper struct), fixing the
  unsynchronized-map hazard identified above as a side effect.

### Interaction with `negativeHostnameCache`

`negativeHostnameCache.IsNegative` (`hostname_guard.go:40-52`) already self-expires entries
after `negativeCacheTTL` (5 minutes, `hostname_guard.go:19`) — a hostname that failed
validation while on the old network stops being negatively cached on its own within 5 minutes
regardless of this feature, so periodic redetection (interval ≥ that TTL) will not be
permanently blocked by a stale negative entry. If Phase 3 picks a redetection interval shorter
than 5 minutes, either shorten `negativeCacheTTL` to match or have `HostnameDetector` call a new
`Handler` method to proactively clear a specific hostname's negative entry when redetection
confirms it now resolves.

## Architecture smell disposition: **Extend as-is**

`main.go` at ~1400+ lines mixing CLI parsing, boot-time detection, TLS setup, and WebAuthn
wiring is a God-function pattern (`startRemoteAccess` alone spans `main.go:1271` to well past
1400), but it is not this project's problem to fix. The existing seams (`Server.SetHostnames`/
`GetHostnames`, `Handler`'s add-only registration, `GetCertificateByLocalAddr`'s map-keyed
callback) are each already well-isolated single-purpose units that only need their storage
swapped from unsynchronized-plain-field to atomic-published-snapshot — a `HostnameDetector`
type can be introduced as a new, separate file (mirroring `host_advertiser.go`) with its `Run`
loop started from the same `warren.App` runtime phase that currently does one-shot detection,
without restructuring `main.go`'s existing control flow. Isolating this one concern via a new
type is enough; a broader `main.go` decomposition is a separate, larger effort out of this
project's scope.

## Summary of files a Phase-3 plan will touch

| File | Change |
|---|---|
| `main.go` (new file preferred, e.g. `hostname_detector.go`) | New `HostnameDetector` type: `Run(ctx)` ticker+event loop, calls `detectLANIPs`/`resolveLANHostnames`, publishes via `Server` |
| `server/server.go:1510-1518` | `hostnames []string` → `atomic.Pointer[[]string]` (or small snapshot struct); `SetHostnames` becomes add-only union + `Store`; `GetHostnames` becomes `Load` |
| `server/tls.go:128-142` | `certs map[string]*NetworkCert` closure input needs to become swappable (atomic-published) so a runtime-added network doesn't race the TLS handshake path |
| `server/auth/webauthn.go` | Optional: new `RegisterHostname` proactive-registration method reusing `webauthnForHost`'s existing add-only logic |
| `server/auth/hostname_guard.go` | Possibly: method to clear a specific negative-cache entry, or just rely on existing 5-minute TTL |
| Platform network-change shim (new, Phase 3) | Feeds the OS-event channel `HostnameDetector.Run` selects on |
