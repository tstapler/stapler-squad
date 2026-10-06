# ADR-001: Per-Tool Cost Attribution at Tool-Type Granularity (Double-Counts Within a Turn)

**Date**: 2026-09-03
**Status**: Accepted

## Context

Claude Code transcripts record token usage per assistant turn
(`session/tokens/types.go:29-37`'s `TurnStats.{Input,Output,CacheCreation,CacheRead}`),
not per tool call. A turn with `N` tool calls (Claude Code routinely fires several in
parallel, e.g. multiple independent `Read`s) has one shared cost figure and no
data source that splits it across the `N` tools — confirmed by direct read of
`parser.go:172-187`: every `tool_use` block in a message just appends to that
message's shared `ToolNames []string`, no per-block token accounting exists in the
JSONL format itself. `requirements.md`'s Rabbit Holes section names this exact
ambiguity and asks Phase 2/3 to pick one of three options rather than let it stall
implementation.

## Options considered

1. **Full-turn attribution to every tool in the turn** — add the whole turn's cost
   to each of its `N` tools' running totals.
   - *Strength*: simplest to implement, and correct for the common single-tool-turn
     case.
   - *Weakness*: for a multi-tool turn, summing `TopToolEntry.cost_usd` across a
     session's distinct tools provably exceeds that session's real
     `estimated_cost_usd` — e.g. a $10 session with 3 co-occurring tools every turn
     would show ~$30 summed — which is wrong in the one direction a cost dashboard
     can least afford (`research/features.md` §3).

2. **Even split across a turn's tools** — divide the turn's cost by `N` and add the
   share to each tool.
   - *Strength*: the per-tool sum never exceeds the session total.
   - *Weakness*: produces a precise-looking number with no operational meaning — a
     turn pairing a cheap `Read` with an expensive multi-file `Grep` attributes both
     the same share. Not more true than option 3, only costlier to compute and
     harder to caveat honestly.

3. **Tool-type-level session sum** — for each turn, compute that turn's cost once
   (`EstimateTurnCost`), then add it once to the running total of every *distinct*
   tool name that appeared in that turn (a turn with `Read` + `Read` + `Grep` adds
   the turn's cost to `Read`'s total once and to `Grep`'s total once — not
   proportional to call count).
   - *Strength*: double-counts only *within* a multi-tool turn, never *across*
     turns; the caveat is a single honest sentence ("a turn with multiple tools
     counts its full cost toward each tool it used") rather than a
     methodology footnote nobody reads.
   - *Weakness*: still lets a session's per-tool costs sum to more than its total
     when multiple tool types co-occur in the same turn — must be caveated in the
     UI, not hidden.

## Decision

**Option 3.** Implemented as `tokens.AttributeToolCosts(r *ParseResult, pt
*PricingTable) (costs map[string]float64, doubleCounted map[string]bool)`
(`session/tokens/pricing.go`), built on a new `EstimateTurnCost` helper that mirrors
`EstimateCost`'s arithmetic at turn granularity (`pricing.go:205-257` has no
per-turn equivalent today — confirmed by grep). `doubleCounted[toolName]` is `true`
iff that tool ever co-occurred with another tool in the same turn anywhere in the
session, so the frontend renders the caveat marker only on rows where it actually
applies, not as a blanket disclaimer.

This is architecturally the cleanest of the three: no schema change to
`ParseResult`/`TurnStats`, no new per-tool-call data source (Claude Code transcripts
don't record output tokens per `tool_use` block — a transcript-format ceiling, not a
stapler-squad parsing gap), and it satisfies the "abstain rather than guess"
precedent `requirements.md` cites from `cacheeconomics` by stating the caveat
plainly rather than either hiding it or inflating the dashboard's headline numbers.

## Consequences

- `proto/session/v1/insights.proto`'s `TopToolEntry` gains `double cost_usd = 4;`
  and `bool cost_may_double_count = 5;`.
- The web UI must render a visible, non-color-only caveat (a `~` prefix plus a
  tooltip, per `research/ux.md` §4 and `research/pitfalls.md` §2's
  measured-vs-modeled distinction) on any `TopToolEntry` row where
  `cost_may_double_count` is true — this is not optional polish, it is the
  mechanism that keeps the option-3 tradeoff honest.
- Per-tool costs must never be summed into a "total tool spend" figure presented
  alongside the session's real `estimated_cost_usd` without the same caveat
  attached — the two numbers can legitimately disagree by design.
- If Anthropic's transcript format ever starts recording per-tool-call token
  usage, this ADR should be revisited — the ceiling that forced option 3 would be
  gone.
