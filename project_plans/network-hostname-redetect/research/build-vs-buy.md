# Build vs. Buy: OS-Level Network-Change Detection

Research for Phase 2, Agent 6. Question: should the OS-level network-change
trigger (macOS + Linux) be built from scratch, or sourced from an existing
library?

## Repo context

`go.mod` already depends on `golang.org/x/net v0.58.0` (no `route` subpackage
imported yet) and `golang.org/x/sys v0.47.0`. No existing dependency for
network-change notification, netlink, or route-socket handling. `main.go`
already shells out to `scutil`/`avahi-resolve`/`hostname` via
`safeexec.CommandContext` for `detectLANIPs`/`resolveLANHostnames`
(`main.go:965-1267`) — the requirements explicitly say to reuse those, not
replace them; this research is only about the *trigger* that decides when to
re-run them.

## Option 1: Hand-roll per-OS shims (raw `AF_ROUTE` socket on macOS, raw netlink socket on Linux)

**Pros:**
- Zero new dependencies; full control over event shape and coalescing.
- Matches the repo's existing pattern of shelling out to/parsing OS-native
  interfaces directly (`scutil --dns`) rather than adding abstraction layers.

**Cons:**
- Raw route-socket/netlink message parsing is exactly the kind of
  binary-protocol-parsing code where hand-rolled correctness risk is highest
  (see Option 3 below) — sequence numbers, PID filtering, multi-part messages,
  message-type enums differ across BSD variants and kernel versions.
- Two entirely separate, non-portable implementations to write, test, and
  maintain (`//go:build darwin` and `//go:build linux`), each requiring
  root-cause-level understanding of a kernel ABI most Go engineers never
  touch.
- Medium appetite (1–2 weeks) budget mostly consumed by protocol plumbing
  instead of the actual feature (debounce, `Server.hostnames` threading,
  RPID/SAN wiring).

**Verdict: Not recommended** as a from-scratch effort, given a maintained,
narrowly-scoped library exists that already solves this (Option 4).

## Option 2: SaaS / managed API

Not applicable. This is a fully local, on-device, single-host detection
problem — there is no network-accessible service to call for "did this
machine's interfaces change." Noted per the research brief and moving on.

## Option 3: LLM-generated raw socket parsing vs. a focused library

For the specific sub-problem of parsing `AF_ROUTE`/netlink wire messages:

- **Correctness risk of hand-rolling is high and not self-evident from
  review.** The `golang.org/x/net/route` package's own experience is the
  cautionary tale here: it's the *official* Go extended-networking package,
  written by people with direct kernel-ABI knowledge, and it **still** has an
  open, unresolved bug (golang/go#44740) where live route-change
  notifications read off an `AF_ROUTE` socket fail to parse
  (`errMessageTooShort`) because the on-the-wire format for live
  notifications differs from the `FetchRIB`/`ParseRIB` format the package was
  actually designed around. `x/net/route` reliably fetches a routing table
  snapshot; it is not a solid foundation for streaming live change events on
  Darwin. An LLM-generated (or from-scratch human) implementation attempting
  the same live-socket read+parse loop would very plausibly reproduce this
  exact class of bug, with no test able to catch it without a real interface
  flap on real hardware — the failure mode is silent-but-wrong (a missed or
  malformed event), not a crash.
- **Netlink on Linux is comparatively lower risk** — `vishvananda/netlink`
  (BSD-3-Clause, MPL-2.0-compatible-style license — see repo `LICENSE`;
  actively used and referenced by major projects like Docker/libcontainer
  history, Cilium, etc.) already wraps `AddrSubscribeWithOptions`/
  `LinkSubscribeWithOptions`/`RouteSubscribe` with sequence/PID handling
  done correctly. Hand-rolling raw `NETLINK_ROUTE` socket parsing here has
  lower correctness risk than the macOS case, but is still solving a
  already-solved problem for no benefit.
- **Conclusion:** for the parsing layer specifically, adopting a maintained
  library is the correct call on both platforms, but for different reasons —
  macOS because the "obvious" building block (`x/net/route`) is documented
  to be unreliable for exactly this use case, and Linux because
  `vishvananda/netlink` is a well-exercised, correct implementation that
  would only be reinvented at cost with no upside.

## Option 4: Adopt `tailscale.com/net/netmon` directly (fork-or-adapt question)

`netmon` (`tailscale.com/net/netmon`, BSD-3-Clause) is a subpackage of the
`tailscale.com` Go module, not a separate module — but Go's module system
resolves subpackage imports independently, so importing
`tailscale.com/net/netmon` pulls in only its own transitive dependency graph
(currently on the order of ~35 imports per pkg.go.dev), not the whole
tailscaled binary. It is designed to be imported standalone: pkg.go.dev shows
73 importers, and Tailscale's own `net/netns` and `net/netcheck` packages
depend on `*netmon.Monitor` as a plain constructor parameter, i.e. it's
already used as a library dependency inside Tailscale's own codebase, not
just an internal implementation detail.

What it does, mapped to this project's needs:
- `Monitor.RegisterChangeCallback` — the OS-level event hook the requirements
  ask for (§ "OS-level network-change trigger").
- Built-in **event coalescing** ("callbacks are called within the event
  coalescing period, under a fraction of a second") — this is exactly the
  debounce/coalescing behavior flagged as a Rabbit Hole in the requirements
  (multiple raw events firing in quick succession during a transition). Using
  `netmon` removes that concern almost entirely instead of requiring
  bespoke debounce-window tuning during Phase 3 planning.
- `Monitor.Poll` — a manual "pretend something changed" hook, useful for
  driving the periodic-timer fallback path through the same code path as the
  event-driven one (one trigger surface for both timer tick and OS event,
  simplifying the observability/logging requirement to log "trigger source").
- Cross-platform: implements both the macOS (`SCDynamicStore` via cgo-free
  polling/route-socket handling internally) and Linux (netlink-based) sides
  already, i.e. it already solves the exact "no shared Go stdlib API between
  macOS and Linux" problem called out in the requirements' Rabbit Holes
  section, under one interface.
- No root/elevated privileges required for interface/route *observation*
  (unlike `vishvananda/netlink`'s write paths, which do need root — this
  project only needs to *observe*, not mutate routes).

**Pros:**
- Solves precisely the stated Feasibility Risk ("No existing Go dependency in
  this repo for OS network-change notifications... verifying there's a
  maintained one that fits this repo's dependency bar is Phase 2 research")
  — this is that library.
- Actively maintained: it's load-bearing production code for Tailscale's own
  client, used on every platform Tailscale ships in production continuously,
  including macOS and Linux — this is about as battle-tested as this specific
  problem gets in the Go ecosystem.
- Coalescing/debounce built in, removing an entire Rabbit Hole from Phase 3
  scope.
- Permissive BSD-3-Clause license, no root privilege requirement for the
  read-only monitoring this project needs.
- Small, focused API surface (`Monitor`, `RegisterChangeCallback`, `Poll`,
  `Close`) — easy to wrap behind this project's own interface for the fake
  clock/fake-event-source test injection the requirements call for
  (`deterministic-fast-tests` skill guidance), without leaking Tailscale
  internals into the rest of the codebase.

**Cons:**
- Adds a dependency on a subpackage of a large, fast-moving module
  (`tailscale.com`) that is not designed as a small standalone library first
  — go.sum will pull in `netmon`'s own transitive deps (still much smaller
  than the full module, but non-zero: likely `golang.org/x/sys`, possibly a
  couple of Tailscale-internal utility packages under `tailscale.com/...`
  that `netmon` itself imports). Needs a `go mod tidy` + `go mod graph` check
  during Phase 3/5 to confirm the actual pulled-in dependency set before
  committing to it, since exact transitive imports weren't independently
  verified by walking the source tree in this research pass (a real gap,
  flagged per the evidence-and-claims discipline — treat the "~35 imports"
  figure from pkg.go.dev as informational, not verified against this
  project's exact `go.sum` outcome).
- API stability is not contractually guaranteed (no semver promise beyond
  "Tailscale's own internal usage discipline") — Tailscale does version-tag
  releases (`tailscale.com@vX.Y.Z`), so pinning a specific tag mitigates
  breakage risk, but upgrades need the same scrutiny as any other dependency
  bump.
- Slightly more indirection than a hand-rolled shim: consumers see a
  Tailscale-shaped API (`Monitor`, `InterfaceState`) rather than something
  purpose-built for this project's `Server.hostnames` model — needs a thin
  adapter layer regardless.

**Verdict: Recommended.** This is the "small enough existing implementation
worth adopting directly" the requirements ask about in Alternatives/Rabbit
Holes — no fork or source-extraction needed, since it's already designed to
be imported as a library dependency rather than copied.

## Final Recommendation

**Adopt `tailscale.com/net/netmon` as the OS-level network-change trigger on
both macOS and Linux**, wrapped behind a small project-owned interface (e.g.
a `NetworkChangeSource` with `RegisterChangeCallback`/`Close`) so:

1. Phase 5 implementation can inject a fake source for deterministic tests
   (per `deterministic-fast-tests`) without touching the real OS APIs.
2. The periodic timer and the OS-event path can both funnel into the same
   re-detection function, with `netmon.Monitor.Poll()` usable to drive the
   timer-tick case through the identical code path as a real network event —
   simplifying the "trigger source" logging requirement to a single enum
   passed at the call site.
3. `detectLANIPs`/`resolveLANHostnames` remain untouched, satisfying the
   constraint to reuse existing detection primitives — `netmon` only decides
   *when* to call them, never *how*.

Do not hand-roll `AF_ROUTE`/netlink parsing: the macOS route-socket path in
particular (`golang.org/x/net/route`) has a known, unresolved live-event
parsing bug (golang/go#44740) even in Go's own extended-networking package,
which is strong evidence this is a genuinely hard-to-get-right protocol
surface, not one worth re-deriving for a single-host diagnostic feature.
Before merging the dependency, run `go mod tidy` and inspect the actual
transitive import diff to confirm the dependency-weight cost is acceptable
(this research did not independently verify netmon's exact transitive graph
against this repo's existing dependency set — flagged as a to-verify item for
Phase 3 planning, not assumed).

### Sources
- [netmon package - tailscale.com/net/netmon - Go Packages](https://pkg.go.dev/tailscale.com/net/netmon)
- [route package - golang.org/x/net/route - Go Packages](https://pkg.go.dev/golang.org/x/net/route)
- [x/net/route: ParseRIB fails with errMessageTooShort on an AF_ROUTE message from Darwin · Issue #44740 · golang/go](https://github.com/golang/go/issues/44740)
- [GitHub - vishvananda/netlink: Simple netlink library for go.](https://github.com/vishvananda/netlink)
- [netlink package - github.com/vishvananda/netlink - Go Packages](https://pkg.go.dev/github.com/vishvananda/netlink)
