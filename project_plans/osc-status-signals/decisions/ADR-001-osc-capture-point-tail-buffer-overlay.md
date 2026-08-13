# ADR-001: OSC Title Capture Reads the Existing Status-Detection Tail Buffer, Not a New Per-Chunk Read-Loop Hook

**Status**: Accepted
**Date**: 2026-08-06
**Project**: osc-status-signals

## Context

`research/architecture.md`'s Option A (the recommended integration point) proposes extracting
the OSC title inside `session/response_stream.go`'s PTY read loop (`response_stream.go:278-305`,
the same call site as `rs.escapeParser.Parse(data, sessionSeq)`), storing the latest title into a
new `atomic.Pointer[string]` on `ResponseStream`/`PTYAccess`, populated once per `pty.Read()` call.

`research/pitfalls.md` §2 live-tested the same tmux control-mode path this read loop consumes and
demonstrated — not hypothesized — that a single OSC escape sequence can arrive fragmented across
dozens of individual `%output` control-mode messages (captured verbatim: `printf`'s `\x1b]0;...`
arrived intact in one message when written by a single `write()`, but the *keystroke-echo* path for
the same session delivered the identical byte sequence one character at a time). `response_stream.go:450`'s
own `buf := make([]byte, 4096)` read granularity is a second, independent way a chunk boundary can
split a sequence. A per-chunk regex/scanner over `data := readBuf[:n]` in isolation would therefore
miss any OSC sequence that happens to straddle a chunk boundary — not a rare edge case, but the
*demonstrated common case* for anything typed/echoed rather than written in one syscall.

Separately, `research/architecture.md` §4 flags its own tension: the existing `statusCache`/`idleCache`
in `session/claude_controller.go` key on a content hash of the *tail buffer* (`GetRecentHash`), and
notes the OSC signal "falls out naturally" as compatible with that cache only *if* it is parsed from
the same tail slice — i.e., a separately-tracked `atomic.Pointer[string]` populated out-of-band by the
read loop would NOT participate in that hash-based cache, becoming a second, uncoordinated
invalidation input the cache doesn't know about.

## Decision

OSC title extraction is a **read-only overlay scan over the same raw tail buffer** already read by
`ClaudeController.GetCurrentStatus()` / `GetStatusAndIdleInfo()` / `GetIdleState()` (the existing
`tail := string((*bufp)[:n])`, sourced from `pa.GetRecentOutputInto(..., statusDetectionTailBytes)`,
the same 4096-byte window text-pattern detection already operates on). No new capture point is added
to `session/response_stream.go`'s read loop, and no new field is added to `ResponseStream`/`PTYAccess`.

`pkg/ansi.ExtractLastOSC` (new, see `plan.md` Phase 1) is called directly against this tail string on
every cache-miss poll, exactly where `filterTmuxMetadata`/`hasScreenOverwrite` already inspect the
same bytes.

## Consequences

- **Split-sequence risk is structurally eliminated, not mitigated.** By the time `tail` is read, the
  circular buffer has already reassembled every PTY chunk/every `%output` fragment in byte order —
  chunk boundaries from the read loop no longer exist at this point. The only remaining truncation
  risk is the tail *window* boundary itself (a title whose opening `\x1b]0;` falls just before the
  4096-byte cutoff) — `ExtractLastOSC`'s "prefix not found → no match for that occurrence" behavior
  (see Phase 1) already treats this correctly as "no title" rather than misparsing a fragment.
- **No new concurrency surface.** No new atomic field, no new write path from the read-loop
  goroutine into a value read by a different goroutine — OSC classification is computed synchronously,
  on-demand, by whichever goroutine calls `GetCurrentStatus`/`GetStatusAndIdleInfo`/`GetIdleState`,
  identically to how text-pattern detection already works.
- **Participates in the existing tailHash cache for free.** Because the OSC signal is derived from
  the exact bytes the cache already hashes, a cache hit is provably still correct (the OSC-derived
  contribution to a cached `status`/`idleState` value cannot go stale independently of the bytes it
  was computed from).
- **Trade-off accepted**: OSC-derived status latency is bounded by the polling cadence (same as
  text-pattern status today), not by "the instant the byte is written." Given `research/ux.md`'s
  finding that `DetectedStatus` already has no delivery-side smoothing (WebSocket push, immediate
  render), this only matters up to however often `GetCurrentStatus`-family functions are actually
  polled — a push-based per-chunk design would only meaningfully improve on this if that polling
  interval were the bottleneck, which was not established as true.
- Rejects `research/architecture.md`'s literal per-chunk Option A sub-step and `research/features.md`
  §4's ratelimit-style parallel-detector alternative — see `plan.md`'s Pattern Decisions table for
  both, with reasoning.
