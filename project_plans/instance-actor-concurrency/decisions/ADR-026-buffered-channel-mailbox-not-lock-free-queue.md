# ADR-026: Buffered Go Channel as the Mailbox, Not a Third-Party Lock-Free Queue

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/requirements.md` (R2.7)

---

## Context

With one actor goroutine per `Instance` decided (ADR-025), the mailbox implementation itself
needed a choice. The `go-concurrency` skill's primitive ladder includes lock-free
multi-producer queues/ring buffers (e.g. `Workiva/go-datastructures`, `golang-design/lockfree`)
as an option for high-contention or high-throughput hand-off, and this question was raised
explicitly earlier in this project's research: `research/stack.md`'s actor-library survey
evaluated `protoactor-go`, `anthdm/hollywood`, `ergo-services/ergo`, and `Tochemey/goakt` —
all four target distributed or high-throughput actor systems (clustering, remoting, supervision
trees, 2M-21M+ msgs/sec) and were rejected as overkill for "one owning goroutine per Instance,
commands via buffered channel, occasional synchronous request/response" at this project's scale.

## Decision

The mailbox is a plain buffered Go channel (`chan command`), not a third-party lock-free queue or
ring buffer.

This is multi-producer/single-consumer traffic at human/LLM-paced frequency: RPC calls
(pause/resume/rename/program-switch — "at most a few/sec across the whole fleet,"
`research/pitfalls.md` §5) and poller ticks on 2-60s intervals (`PRStatusPoller`,
`ReviewQueuePoller`, `CapacityMonitor`). This is many orders of magnitude below the throughput
regime (millions of messages/sec) that motivates a lock-free ring buffer — `hollywood` is built
for 10M+ msgs/sec single-node, `ergo` for 21M+ msgs/sec local — numbers that exist to serve
actor *swarms* (e.g. game entities) or distributed clustering, not dozens-to-hundreds of
long-lived, low-frequency command consumers. A lock-free queue's throughput advantage is
irrelevant at a workload this far below its design point.

Channels are also stdlib (no new dependency to vet, pin, or update), idiomatic Go ("share memory
by communicating" — Pike, "Go Concurrency Patterns," cited in `research/features.md` §4), and
already the pattern this codebase uses for its other long-lived background goroutines:
`session/pr_status_poller.go` and `session/review_queue_poller.go` both already use
`context.WithCancel` + an explicit `Stop()`, and `server/services/capacity_monitor.go`'s poll
loop already selects on `ctx.Done()`. Adopting a third-party lock-free queue for the mailbox
would introduce a second concurrency idiom alongside the channel-based patterns already in
production use, for no measurable benefit at this traffic level.

## Consequences

### Positive
- Zero new dependency surface to vet, pin, or keep updated.
- Consistent with this codebase's existing poller/worker concurrency idiom — one fewer pattern
  for future contributors to learn.
- A buffered channel gives natural, well-understood backpressure (blocking `send`) as the
  default signal when an actor falls behind, with a `select`-based `sendCtx` escape hatch
  (`research/architecture.md` §5) for pollers sweeping many instances that should not stall on
  one wedged actor.

### Negative / Accepted tradeoffs
- A plain channel cannot be resized at runtime; the buffer capacity is a fixed constant chosen
  at construction (see ADR-027 for the specific value).
- If this workload's shape changes materially (e.g. a future feature drives sustained
  high-frequency per-Instance command rates), this decision should be revisited — but per R2.7,
  that revisit is explicitly deferred until `go tool pprof` shows the channel itself, rather than
  tmux/git I/O inside a command handler, is the bottleneck. No such evidence exists today.
