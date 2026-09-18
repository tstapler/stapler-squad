# Requirements: network-hostname-redetect

**Date**: 2026-09-18
**Type**: bug fix / feature addition (runtime re-detection of a currently boot-only computation)
**Complexity**: 3 — system design

## Problem Statement
`stapler-squad` resolves which LAN hostnames it's reachable at (`detectLANIPs()`/`resolveLANHostnames()` in `main.go:965-1267`) exactly once, at process startup, and stores the result unconditionally in `Server.hostnames` (`server/server.go:1510-1518` — a plain `[]string` field, no mutex, no TTL). When the machine changes networks (e.g. switching Wi-Fi networks, VPN connect/disconnect, DHCP renewal onto a different subnet), nothing re-runs detection. A hostname the server served fine before the switch (e.g. `netflix1.staplerhome.com`) silently stops being resolvable by the server's internal book-keeping, and the only fix today is a full process restart.

The only existing dynamic behavior is in `server/auth/webauthn.go`'s `webauthnForHost` (line 93+): a lazy, per-incoming-request, WebAuthn-specific, **add-only** check that registers a newly-seen `Host` header as an additional trusted RPID if it resolves — it never triggers proactively and never removes anything.

## Baseline
Today, a network change requires the user to notice the symptom (a previously working hostname/URL no longer loads, or a WebAuthn/passkey ceremony fails with an RPID mismatch) and manually restart the `stapler-squad` service (`make install-service`, which also kills every live tmux session — a costly workaround) to force re-detection.

## Users / Consumers
- The `stapler-squad` Go service itself (`main.go`, `server/server.go`) — internal consumer of `Server.hostnames`.
- `server/auth/webauthn.go`'s `Handler` — reads the hostname/RPID set to validate WebAuthn ceremonies.
- The `--remote-access` HTTPS server (`main.go:1269+`, `startRemoteAccess()`) — reads `srv.GetHostnames()` to build TLS certificate SANs.
- End user (Tyler) accessing the web UI or remote-access endpoint from a LAN hostname (e.g. `netflix1.staplerhome.com`) that depends on this detection being current for the network actually in use.

## Success Metrics
- After switching networks (new Wi-Fi SSID, VPN toggle, or DHCP renewal to a new subnet) **without restarting the process**, a LAN hostname newly resolvable on the new network becomes usable (served, and — per the scope decision below — trusted as a WebAuthn RPID / present in the remote-access TLS cert SAN list) within one re-detection cycle, not "after next restart."
- A hostname that resolved at boot but no longer resolves on the new network is *not* actively purged (see Scope) — success here is measured by "no new hostnames are missed," not by pruning.
- No regression: hostnames valid at boot remain valid after a re-detection cycle finds no change (idempotent, no flapping).

## Appetite
Medium (1–2 weeks) — includes both a periodic timer *and* an OS-level network-change event hook (not timer-only), plus test coverage across the platforms this repo targets (macOS primary, Linux).

## Constraints
- Single developer, no fixed external deadline.
- Must not require restarting the live systemd/launchd-managed service to pick up a network change (that's the whole point) and must not introduce a new *required* restart path for normal operation.
- Must reuse the existing detection primitives (`detectLANIPs`, `resolveLANHostnames`, `getDNSSearchDomains`, `scutilNameservers`) rather than rewriting hostname-resolution logic — this is about making that logic re-run and its result observable at runtime, not replacing it.

## Non-functional Requirements
- **Performance SLO**: re-detection must be cheap enough to run periodically (dozens of DNS/`scutil`/`avahi-resolve` subprocess calls) without perceptibly affecting request latency — must run off the request path, in a background goroutine.
- **Scalability**: not applicable (single-host, single-process detection).
- **Security classification**: internal/confidential — this affects WebAuthn RPID trust and TLS SAN issuance, both auth-adjacent surfaces already covered by existing tests (`webauthn_test.go`) and a negative-hostname cache/rate limiter (`server/auth/hostname_guard.go`).
- **Data residency**: not applicable.

## Scope

### In Scope
- A background mechanism that periodically re-runs LAN IP/hostname detection at runtime (not just at boot) and updates `Server.hostnames` thread-safely (replacing the current unsynchronized plain-field access in `server/server.go:1510-1518`).
- An OS-level network-change trigger (in addition to the periodic timer) so re-detection can also fire promptly on an actual interface/network change rather than only on a fixed interval, on at least macOS (the primary dev platform) and Linux.
- **Add-only** semantics for the resulting hostname set feeding WebAuthn RPID registration and `--remote-access` TLS cert SANs: newly-resolvable hostnames get added; hostnames that stop resolving are *not* removed from the trusted/served set during the process's lifetime (per the scope decision made in ideation — matches the existing `webauthnForHost` add-only precedent, avoids live passkey/session breakage from transient DNS flakiness).
- Enough logging/observability to see when a re-detection cycle ran, what (if anything) changed, and why (timer tick vs. network-change event).

### Out of Scope
- Actively removing/pruning hostnames, RPIDs, or TLS SANs that no longer resolve. (Explicitly deferred — see Alternatives Considered.)
- Regenerating/rotating the `--remote-access` TLS certificate's private key or re-serving a brand-new cert file on disk if that's how SANs are currently baked in — only in scope if this is a low-cost addition to the SAN set for the already-supported live-registration path; if the existing cert mechanism requires a full regenerate-and-reload, that's flagged as a Feasibility Risk / Rabbit Hole for Phase 3 planning to resolve, not assumed solved here.
- Cross-cutting rework of the DNS/mDNS resolution strategy itself (`resolveLANHostnames`'s internal fallback chain) — reused as-is.
- Windows support (not a target platform per the repo's stated environments — Manjaro/Ubuntu Linux primary, macOS at work, some WSL2).
- The separate gossip-based cross-host advertisement feature (`session/host_advertiser.go`, `server/auth/host_advertisement.go`, ADR-002) — unrelated to LAN hostname detection, not touched here.

## Rabbit Holes
- **TLS cert SAN mutation after issuance**: if `--remote-access`'s TLS certificate is generated once at startup with SANs baked in (rather than served via a dynamic `GetCertificate` callback), adding a hostname at runtime may require either (a) a `tls.Config.GetCertificate` callback that can serve a freshly-minted cert per SNI/host, mirroring the existing add-only RPID pattern, or (b) accepting that TLS SAN coverage stays boot-time-only while WebAuthn RPID coverage becomes dynamic — an inconsistency Phase 3 must explicitly resolve, not paper over.
- **OS-level network-change detection portability**: macOS (`SCDynamicStore`/`scutil` reachability callbacks) and Linux (netlink route/address change sockets, or polling `/sys/class/net/*/operstate`) have no shared Go stdlib API — likely needs either a small platform-specific shim per OS or a well-chosen third-party library; verifying there's a maintained one that fits this repo's dependency bar is Phase 2 research, not assumed here.
- **Debounce/coalescing**: a network transition can fire multiple raw OS events in quick succession (e.g. interface down then up then DHCP-assigned) — naive re-detection on every event could hammer `scutil`/`avahi-resolve` subprocesses; needs a debounce window, sized during planning.
- **Interaction with `negativeHostnameCache`** (`server/auth/hostname_guard.go`): confirm a hostname that failed lazy runtime validation while on the old network doesn't stay negatively cached and block it from being picked up by the new periodic/event-driven detection once it becomes valid on a subsequent network.

## Alternatives Considered
- **Prune stale hostnames too** (rejected for this project, per ideation answer): would fully "fix" staleness but risks breaking a live passkey ceremony or an in-flight remote-access session if DNS is briefly flaky during a network transition; add-only avoids that failure mode at the cost of unbounded (but process-lifetime-bounded, i.e. cleared on restart) growth of the trusted set.
- **Detection-only, no RPID/TLS wiring** (rejected): would fix the internal `Server.hostnames` list but leave the reported symptom (a hostname not being served/trusted after a network switch) unresolved for the auth-relevant paths that actually matter to the user.
- **Timer-only, no OS event hook** (rejected — Small appetite option not chosen): simpler, but leaves a window (up to the poll interval) after a network change before the new hostname works, and was traded for the OS-event-hook approach at Medium appetite.

## Feasibility Risks
- No existing Go dependency in this repo for OS network-change notifications — may need a new dependency or platform-specific `cgo`/exec-based shim (`scutil` is already shelled out to for DNS lookups, so a similar approach may extend to reachability callbacks on macOS).
- `detectLANIPs`/`resolveLANHostnames` shell out to several external binaries per call (`scutil`, `avahi-resolve`, `hostname`) — periodic re-invocation must not leak subprocesses or accumulate goroutines if the timer/event loop isn't properly bounded and cancellable via `context.Context` (this repo already has conventions here — see `docs/explanation/concurrency-patterns.md`).
- Test coverage: `server/auth/webauthn_test.go` already documents the current one-shot limitation in a comment; the new periodic/event-driven path needs deterministic tests per this repo's `deterministic-fast-tests` skill guidance (no real sleeps/timeouts) — likely means injecting a fake clock/ticker and a fake OS-event source rather than testing against real network changes.

## Observability Requirements
Log (structured `slog`, matching `logs/staplersquad.log` conventions) each re-detection cycle: trigger source (timer vs. OS event), duration, previous hostname count, new hostname count, and any hostnames added (never previously seen). No new metric/alert is required — this is single-host diagnostic logging, not a paged condition.

## Risk Control
Feature is purely additive to hostname *discovery* (add-only, as scoped) — no removal behavior means no risk of a working hostname/session breaking as a side effect of this change. Rollback is a plain revert (no data migration, no persisted state format change). No feature flag needed given the additive-only blast radius, but the periodic/event loop should be trivially disableable (e.g. via existing `--remote-access`-style flag or env var) in case the OS-event integration proves noisy or resource-heavy on some platform, discovered post-ship.

## Open Questions
- Does the `--remote-access` TLS certificate currently support serving a dynamically-updated SAN set (`tls.Config.GetCertificate`), or is it a single static cert generated once at `startRemoteAccess()` time? (Phase 2 research — determines whether TLS SAN coverage can be made dynamic in this project or must be flagged as a known residual gap.)
- What's the right re-detection interval, and does it need to be configurable? (Phase 3 planning.)
- Is there a maintained, low-dependency-weight Go library for cross-platform (macOS + Linux) network-change notification, or does this need a hand-rolled per-OS shim? (Phase 2 research.)
