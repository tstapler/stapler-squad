# ADR-001: `GetCurrentStatus` must write `subagentCount` even though nothing currently reads it there

## Status

Accepted

## Context

`session/claude_controller.go` has two methods that both read/write the same tail-hash cache,
`cc.statusCache` (`atomic.Pointer[statusCacheEntry]`):

- `GetCurrentStatus()` (line 631) — reads the cache, and on a miss, detects and
  `Store()`s a fresh `statusCacheEntry`.
- `GetStatusAndIdleInfo()` (line 955) — reads the *same* cache pointer, and on a miss,
  detects and `Store()`s a fresh `statusCacheEntry`.

Both are called independently from different places: `GetCurrentStatus` from
`claude_controller.go:1151` (a status-change listener that only cares about the status enum,
already discarding the `desc` return value), and `GetStatusAndIdleInfo` from
`session/instance_status.go:76` (`InstanceStatusManager.GetStatus`, the sole feed into
`InstanceStatusInfo` and, transitively, into the `Session.subagent_count` proto field this
feature adds).

The `subagent-spawn-tracking` feature adds a `subagentCount int` field to `statusCacheEntry`.
The naive implementation only updates `GetStatusAndIdleInfo` (the method that actually needs
the count downstream) and leaves `GetCurrentStatus` writing the struct's zero value for that
field.

## Decision

Both `GetCurrentStatus` and `GetStatusAndIdleInfo` must populate `subagentCount` on every
`statusCacheEntry{...}` they `Store()`, even though `GetCurrentStatus`'s own return signature
never exposes it.

## Rationale

Because both methods share one `atomic.Pointer[statusCacheEntry]` keyed by the same FNV tail
hash, whichever method runs first on a given tail's content "wins" the cache write for that
hash. If `GetCurrentStatus` (which doesn't itself need the count) writes a zero-value
`subagentCount` for a tail that genuinely has, say, 2 active background agents, then a
subsequent call to `GetStatusAndIdleInfo()` on that *same unchanged tail* would hit the cache
(`sc.tailHash == h`) and silently return `subagentCount == 0` — even though the correct value
(`2`) was computable and even though `GetStatusAndIdleInfo` itself never got a chance to
detect it, because the cache told it "nothing changed, use the cached entry."

This would produce a real, user-visible bug: the badge would intermittently show no count (or
a stale lower count) depending purely on which of the two methods happened to run first after
a tail change — a race condition that would be very difficult to reproduce and debug later,
since it depends on call ordering between two otherwise-independent call sites, not on any
single method's logic being wrong in isolation.

## Consequences

- `GetCurrentStatus` must call the new `DetectWithContextAndCountFromLines` method (instead of
  the existing `DetectWithContextFromLines`) purely to obtain the count for the cache write,
  discarding it from its own 2-value return signature. This is a small amount of "unused by
  this method's own caller" plumbing that exists solely for cache coherence.
- A dedicated test (`TestGetCurrentStatus_ThenGetStatusAndIdleInfo_should_shareSubagentCount_When_sameTailHash`)
  is required specifically to guard this invariant, since normal per-method unit tests
  (testing each method in isolation) would not catch a coherence bug between the two.
- If a third method is ever added that also writes to `cc.statusCache`, it must follow the
  same rule: any field added to `statusCacheEntry` must be populated by *every* writer, not
  just the writer whose caller currently needs it.
