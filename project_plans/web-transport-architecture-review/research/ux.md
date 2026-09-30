# Research: UX for web-transport-architecture-review

**Phase**: 2 (research) for `web-transport-architecture-review`
**Question being answered**: does a transport change (Apache Iggy / yetty / WebTransport) under
terminal streaming, the RPC bridge, or CDP/VNC proxying have any *user-perceptible* consequence
beyond "did it stay connected and fast"?

**Bottom line: mostly no.** At this product's actual scale (single operator, a handful of
concurrent connections, mostly LAN/localhost/Tailscale), transport choice is invisible to the
job the user is hiring the terminal/RPC/CDP streams to do, with one narrow exception (mobile
network-switch continuity, §2) that is real but small, already 80%-mitigated by application-level
work this repo already shipped (ADR-023), and not itself sufficient to justify WebTransport's
infra cost — that cost/benefit tradeoff is the Phase 2 architecture/stack docs' call, not this
doc's. This finding is the same shape as `exotic-transports.md`'s verdict on IWA Direct
Sockets/WASM terminals: a real technology, evaluated on its own terms, that doesn't solve a
problem this product has at this scale — see that doc
(`project_plans/terminal-multi-connection-streaming/research/exotic-transports.md`) for the
established precedent of reaching "not applicable" on evidence rather than by assumption.

## 1. Perceived responsiveness: does WebTransport's multiplexing win anything visible here?

**Claim being tested**: WebTransport's QUIC-based independent streams eliminate head-of-line
(HOL) blocking that TCP-based WebSocket suffers — could that make terminal redraw or RPC calls
feel snappier?

**What the evidence actually says**: HOL blocking is a *packet-loss* problem, not a raw-latency
problem. A lost TCP segment stalls every stream multiplexed on that connection until
retransmission; QUIC's independent streams don't share that fate. The real-world numbers found:
one benchmark showed a chat stream averaging 62ms on WebTransport vs. 68ms on WebSocket — a
~9% difference attributable to HOL-blocking avoidance — and one blog reported a 35% cut in a
specific high-loss scenario. The much larger gains cited (QUIC vs. TCP under 4% packet loss:
56.7s vs. 6.6s for a resumed download) are all *lossy/mobile-network* scenarios, not clean
connections. [WebSocket vs WebTransport: When to Use Which](https://websocket.org/comparisons/webtransport/),
[Low-Latency Browser Communication with WebTransport](https://blog.openreplay.com/low-latency-browser-communication-webtransport/).
One of those sources says it plainly: *"If head-of-line blocking is a real problem for your
application (measure first, don't assume), WebTransport is the better protocol choice."*

**Applied to stapler-squad's actual traffic**: `session/streamhub/`'s connections are terminal
output frames from a single operator's tmux sessions, typically over `localhost:8543` or a
LAN/Tailscale hop to `onyx.staplerhome.internal` — not a lossy public-internet path. Packet loss
on a home LAN or a Tailscale WireGuard tunnel is near-zero under normal conditions; the scenario
HOL-blocking protects against (a lost packet stalling every other in-flight message on the same
connection) is rare here to begin with, and even when it does happen, a single operator with one
or two concurrent tabs isn't multiplexing many independent streams over one connection the way a
video conferencing app or a high-fan-out chat server does — there's little cross-stream
contention for a lost packet to create in the first place. The theoretical mechanism is real; the
traffic pattern that makes it *matter* (many concurrent multiplexed streams, non-trivial packet
loss) doesn't match this product's actual usage.

**Verdict**: theoretical, not perceptible at this scale. This is a case where the technology's own
advocates' framing ("measure first, don't assume") already tells you the answer for a low-loss,
low-concurrency, single-operator connection: no measurable redraw-latency or RPC-responsiveness
win worth attributing to the transport swap. The RPC bridge's actual complaint
(`connectrpc_websocket.go`'s hand-rolled envelope framing, per `requirements.md`'s Open
Questions) is an engineering-cost problem, not a user-facing latency problem — Phase 2's
architecture/stack docs are the right place to weigh "reinventing ConnectRPC's native transport"
against "adopting WebTransport," not this doc.

## 2. Reliability/reconnection UX: connection drops and the mobile remote-access case

This is the one place transport choice has a real, if narrow, user-facing consequence, and it
maps directly onto the documented mobile-app remote-access use case (connecting to the home
server over LAN or Tailscale — per user memory, `https://onyx.staplerhome.internal:8444`).

**What's already built (ADR-023, shipped)**: `docs/adr/ADR-023-client-reconnect-browser-lifecycle.md`
already solved most of the *application-visible* pain of network-change reconnects, independent
of transport:
- `visibilitychange`/`online` listeners reconnect within ~200ms of a tab returning from
  background or a network coming back, instead of waiting out a full exponential-backoff cycle
  (previously up to 30s).
- Full-jitter backoff avoids thundering-herd reconnect storms.
- The terminal stream (`useTerminalStream.ts`) now auto-reconnects without requiring the user to
  navigate away and back — previously a real gap (`error` never set on clean WS close, so the
  reconnect guard never fired).
- `TerminalOutput.tsx:1807-1811` already has the "Reconnecting…" banner
  (`role="status"`/`aria-live="polite"`) this repo uses for non-blocking reconnect UX, and
  `TerminalOutput.tsx:1815` the `hardFailedBanner`/`role="alert"` for a genuine dead connection.

**What WebTransport's QUIC layer would add on top of that**: real *connection migration* — a
QUIC connection ID is decoupled from the underlying IP/port 5-tuple, so switching Wi-Fi→cellular
(or Wi-Fi→Tailscale-over-cellular) doesn't require a new handshake at all; the same connection
survives the network-path change. [HTTP/3 Connection Migration and mobile networks](https://webhosting.de/en/http3-connection-migration-mobile-networks-roaming-insights/),
[How to Understand QUIC Connection Migration with IPv6](https://oneuptime.com/blog/post/2026-03-20-quic-connection-migration-ipv6/view).
That's a materially different thing than "reconnect fast after the fact" (what ADR-023 does) —
it's "never actually disconnect" for the specific case of a client's network path changing while
the server's address stays fixed. For the mobile-app-connects-to-home-server-over-Tailscale
scenario, that's a real match: a phone moving from home Wi-Fi to cellular mid-session is exactly
the network-path change QUIC migration is built for, and it's the one case ADR-023's fast
reconnect can't fully hide (there's still a WebSocket close + full re-handshake + re-subscribe
in between, even if it now happens in ~200ms instead of ~30s).

**But**: no evidence was found — in this repo's docs or via external search — that this gap is an
observed pain point today. `docs/bugs/fixed/BUG-014-reconnect-backoff-reset-on-unstable-connection.md`
and ADR-023 both address *server-side* or *tab-visibility* reconnect scenarios, not a documented
"my phone dropped to Reconnecting… when I walked out of Wi-Fi range" complaint. Given ADR-023 was
scoped and shipped specifically to close reconnect-latency gaps, and no comparable open bug/ADR
exists for the mobile-network-switch case, this reads as a plausible-but-unverified UX win, not a
demonstrated one at this product's current usage pattern (mostly stationary use — a laptop/desktop
browser tab on a fixed network, occasional phone check-ins over Tailscale). The gap it closes is
real; there's no evidence it's currently *felt*.

**Verdict**: the one legitimate transport-driven UX win in this review, scoped narrowly to the
mobile remote-access path — but it is an incremental improvement on top of an already-decent
baseline (ADR-023's ~200ms visibility-triggered reconnect), not a "reconnecting spinner disappears
entirely" transformation, and it is unverified as an actual pain point rather than a theoretical
one. It should factor into Phase 2/3's cost-benefit for WebTransport specifically (not Iggy or
yetty, neither of which touch this) as a real but minor tailwind, not a driving justification on
its own — the HTTP/3 termination and infra cost named in `requirements.md`'s Rabbit Holes is the
dominant factor, and that's an infra-stack question, not a UX one.

## 3. Accessibility/keyboard nav

**N/A.** This review evaluates a transport layer underneath existing streaming/RPC/proxy
subsystems — it introduces no new UI surface, no new interactive component, no new focus target.
The accessibility patterns already established for connection state
(`role="status"`/`aria-live="polite"` for reconnecting, `role="alert"` for hard failure — both in
`TerminalOutput.tsx`, and the same convention documented in the prior
`terminal-multi-connection-streaming/research/ux.md` §3) are unaffected by what protocol carries
the bytes underneath them. Any Phase 3 plan should explicitly preserve these existing
banners/announcements as-is rather than reinvent them for a new transport.

## 4. Error states for a transport migration rollout

Following this repo's own dark-launch precedent (`session/streamhub`'s flag-gated rollout,
ADR-023's `NEXT_PUBLIC_RECONNECT_V2` flag), any transport swap that ships needs distinguishable,
non-alarming error states during the flag-flip window, not a single generic failure banner:

- **New transport unavailable/unsupported (e.g., browser lacks WebTransport support, or HTTP/3
  fails to negotiate)**: must fail closed to the existing WebSocket path silently, not surface an
  error to the user at all. This is the same shape as any feature-flag fallback in this repo —
  the user should never see "WebTransport failed" as a banner; they should just transparently get
  the WebSocket experience they have today. Surfacing this as a *log line*
  (structured, per-connection, per `requirements.md`'s Observability Requirements) is right; a
  user-visible error is not.
- **Mid-session fallback (started on new transport, had to drop to old one)**: this is the one
  case that could look like today's existing "Reconnecting…" banner if handled well, or like
  scrambled/lost output if handled badly (the same root failure mode
  `terminal-multi-connection-streaming` was built to fix — see its `research/ux.md` §2 and §4 on
  not letting a transport-level transition look like corruption). Reuse the existing
  `role="status"` reconnecting banner rather than inventing new copy; the user doesn't need to
  know *which* transport failed, only that the connection is momentarily reestablishing.
- **Do not add a third, transport-specific error banner.** The precedent this repo has already
  set (`DeepLinkErrorBanner.tsx`'s discipline of distinct copy per *failure reason* but consistent
  `alert`/`status` severity split, reaffirmed in the prior streaming project's `research/ux.md`
  §4) argues for folding any new transport-fallback state into the existing reconnect/hard-fail
  vocabulary, not adding a new one users have to learn.

This is guidance for a Phase 3 plan.md if one is written recommending WebTransport adoption
anywhere; it is not itself a recommendation to adopt.

## 5. Job-to-be-done

The job being hired here is **"let me see and act on my remote agent session (or browser
automation view) right now, from whatever device I'm holding, without thinking about how the
bytes get there."** Two sub-jobs:

- **Functional**: watch tmux/agent output update live, send keystrokes/commands, watch a CDP/VNC
  browser view, and have RPC calls (session list, backlog actions) resolve promptly. All three
  subsystems already do this over WebSocket today; none of Iggy, yetty, or WebTransport change
  *whether* this job gets done, only *how reliably/cheaply* the plumbing underneath does it.
- **Emotional**: the same "don't let me mistake connection noise for my agent breaking" job
  identified in `terminal-multi-connection-streaming/research/ux.md` §5 — a user watching a
  dropped/reconnecting stream needs confidence that their agent's actual state is intact, not
  that the transport swap that happened underneath introduced a new class of ambiguous failure.

Transport choice is invisible to both sub-jobs as long as the connection stays up and reconnects
predictably. The one sub-case where transport is briefly *visible* to the job is the mobile
network-switch scenario in §2 — going from "Reconnecting…" flashing during a Wi-Fi→cellular
handoff to no visible interruption at all is a real, if small, improvement to the "don't make me
think about connectivity" job. Nothing about Iggy or yetty touches this job at all — Iggy is a
server-side message-streaming engine (a durability/throughput concern, not a client-perceived
latency or reconnection concern) and yetty's actual applicability is for the architecture/stack
research docs to establish, not this one.

## Summary for planning

- **No manufactured UX findings.** Outside of the narrow, unverified mobile-network-switch case
  in §2, transport choice has no user-perceptible consequence at this product's current scale —
  say so plainly rather than inventing a responsiveness story HOL-blocking research doesn't
  support for a low-loss, low-concurrency, mostly-LAN/localhost usage pattern.
- **§2's finding is a real but minor tailwind for WebTransport specifically**, not a case for
  Iggy or yetty, and not strong enough on its own to outweigh the HTTP/3 infra cost this repo's
  own Rabbit Holes section already flags as the dominant unresolved question — that tradeoff
  belongs in Phase 2's architecture/stack docs and Phase 3 planning, not here.
- **Accessibility is N/A** — no new UI surface; the existing `role="status"`/`role="alert"`
  reconnect/hard-fail conventions in `TerminalOutput.tsx` should be preserved unchanged by any
  transport swap.
- **If a transport migration ships**, route any new failure mode through the existing
  reconnecting-banner/hard-fail-banner vocabulary rather than inventing new user-visible states;
  a failed-transport-negotiation fallback to WebSocket should be silent to the user and visible
  only in structured logs.

## Sources

- [WebSocket vs WebTransport: When to Use Which | WebSocket.org](https://websocket.org/comparisons/webtransport/)
- [Low-Latency Browser Communication with WebTransport](https://blog.openreplay.com/low-latency-browser-communication-webtransport/)
- [HTTP/3 Connection Migration and mobile networks: How QUIC accelerates the mobile web](https://webhosting.de/en/http3-connection-migration-mobile-networks-roaming-insights/)
- [How to Understand QUIC Connection Migration with IPv6](https://oneuptime.com/blog/post/2026-03-20-quic-connection-migration-ipv6/view)
- `docs/adr/ADR-023-client-reconnect-browser-lifecycle.md` (this repo)
- `docs/bugs/fixed/BUG-014-reconnect-backoff-reset-on-unstable-connection.md` (this repo)
- `project_plans/terminal-multi-connection-streaming/research/exotic-transports.md` (this repo,
  methodological precedent)
- `project_plans/terminal-multi-connection-streaming/research/ux.md` (this repo, accessibility/
  error-state precedent)
- `web-app/src/components/sessions/TerminalOutput.tsx` (reconnecting/hard-fail banner
  implementation, lines ~1807-1815)
