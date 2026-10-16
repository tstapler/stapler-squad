# ADR-001: Adopt `tailscale.com/net/netmon` for OS-level network-change detection

**Status**: Accepted
**Date**: 2026-09-18
**Project**: network-hostname-redetect

## Context

`stapler-squad` needs to know, on both macOS and Linux, when the machine's network configuration changes (new Wi-Fi SSID, VPN connect/disconnect, DHCP renewal onto a new subnet) so it can re-run its existing `detectLANIPs`/`resolveLANHostnames` logic promptly, in addition to a periodic timer. There is no shared Go stdlib API for this across macOS and Linux, and no existing dependency in `go.mod` covers it.

Three options were evaluated (full detail in `project_plans/network-hostname-redetect/research/build-vs-buy.md`):

1. **Hand-roll per-OS shims** — raw `AF_ROUTE` socket parsing on macOS (via `golang.org/x/net/route`, already an indirect subpackage of an existing dependency) and raw `NETLINK_ROUTE` socket parsing on Linux (via `golang.org/x/sys/unix`, already a direct dependency).
2. **`vishvananda/netlink` for Linux only** — well-regarded, but solves nothing for macOS.
3. **`tailscale.com/net/netmon`** — a subpackage of the `tailscale.com` Go module implementing cross-platform (macOS + Linux + others) network-change monitoring with built-in event coalescing, used as a library dependency inside Tailscale's own codebase (not merely an internal implementation detail — `pkg.go.dev` shows 73 importers).

## Decision

Adopt `tailscale.com/net/netmon`, pinned to a specific tagged release, wrapped behind a small project-owned `NetworkChangeSource` interface (`RegisterChangeCallback`/`Close`) so the rest of the codebase never imports `tailscale.com` directly.

## Rationale

- **The "obvious" hand-rolled macOS path has a known, unresolved correctness bug.** `golang.org/x/net/route` — Go's own official extended-networking package, written with direct kernel-ABI knowledge — has an open issue (golang/go#44740) where live route-change notifications read off an `AF_ROUTE` socket fail to parse (`errMessageTooShort`), because the on-the-wire format for live notifications differs from the `FetchRIB`/`ParseRIB` format the package was designed around. This package reliably fetches a routing-table *snapshot*; it is not a solid foundation for streaming live change *events* on Darwin. Reproducing this parsing layer by hand (or via an LLM) would plausibly hit the same class of silent-but-wrong bug (a missed or malformed event), with no test able to catch it short of a real interface flap on real hardware.
- **`netmon` already solves the exact problem this project has**, per its own doc comment: "monitoring network interface and route changes... primarily exists to know when portable devices move between different networks." It implements both the macOS side (route-socket/`sysctl`-based, no cgo) and the Linux side (netlink-based) under one interface, plus a generic polling fallback.
- **Built-in event coalescing removes an entire Rabbit Hole from this project's scope.** The requirements explicitly flag "a network transition can fire multiple raw OS events in quick succession" as needing a debounce window sized during planning; `netmon`'s callbacks are already coalesced within "a fraction of a second," so this project does not need to hand-tune a debounce timer for the OS-event arm (see `plan.md`'s Pattern Decisions: "rely on netmon's built-in coalescing... don't leave ambiguous").
- **`netmon.Monitor.Poll()` gives one code path for both the timer and event triggers** if desired in a future iteration, simplifying "trigger source" logging — not required for this plan's initial cut, but a reason the API shape is a good fit, not just a checkbox.
- **License and maintenance posture are acceptable**: BSD-3-Clause, actively maintained as load-bearing production code for Tailscale's own client on every platform it ships, including macOS and Linux continuously.
- **No root/elevated privileges required** for the read-only interface/route *observation* this project needs (unlike `vishvananda/netlink`'s write paths).

## Consequences

- **New dependency weight**: importing `tailscale.com/net/netmon` pulls in that subpackage's own transitive dependency graph as a `go.mod` entry for the whole `tailscale.com` module (Go's linker still dead-code-strips the compiled binary, but the *declared* dependency surface grows). This must be checked concretely — not assumed — via `go mod tidy` and inspecting the actual `go.sum` diff before merging (Story 3.1.1, Task 3.1.1a). If the diff turns out to pull in something unacceptable (e.g. a license conflict, an unexpectedly large closure), this ADR's decision should be revisited before merge, not after.
- **No contractual API stability guarantee** beyond Tailscale's own internal usage discipline (no semver promise). Mitigated by pinning to a specific tagged release rather than `@latest`/a branch; future upgrades need the same scrutiny as any other dependency bump.
- **A thin adapter layer is required regardless** (`NetworkChangeSource`), since `netmon`'s API surface (`Monitor`, `ChangeDelta`) is shaped for Tailscale's own use, not this project's `HostnameDetector.events <-chan struct{}` model. This is treated as a feature, not a cost: it's also exactly what makes the OS event source fake-able in tests (per `deterministic-fast-tests`), independent of whether `netmon` itself ships good test hooks.
- **CI still cannot exercise the real macOS event path** (no macOS runner, confirmed in `research/pitfalls.md` §5) — this is unchanged by adopting `netmon` versus hand-rolling; either way, the macOS-specific behavior is manually verified only. Adopting `netmon` does not make this worse, and its cross-platform design at least means the *same* code path is exercised on Linux in CI (via `netmon`'s Linux backend), rather than two independently-hand-rolled, independently-untested implementations.

## Alternatives Rejected

- **Hand-rolled `AF_ROUTE`/netlink parsing per OS** — rejected per the correctness-risk argument above; would also consume a disproportionate share of the Medium (1-2 week) appetite on protocol plumbing instead of the actual feature (debounce, `Server.hostnames` threading, RPID/SAN wiring).
- **`vishvananda/netlink` for Linux + a separate macOS-only mechanism** — rejected because it only solves half the problem; would still need a macOS answer (hand-rolled or another library), reintroducing the exact `golang.org/x/net/route` risk above for that half.
