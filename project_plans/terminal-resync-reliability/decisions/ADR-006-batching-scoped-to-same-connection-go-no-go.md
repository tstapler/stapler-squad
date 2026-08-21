# ADR-006: Batching Scoped to Same-Session/Same-Connection Only, With an Explicit Go/No-Go

**Date**: 2026-08-13
**Status**: Accepted (go/no-go itself deferred — see Consequences)
**Project**: terminal-resync-reliability

## Context

`requirements.md` Scope item 5 covers both batching and compression, but its Rabbit Holes
section explicitly warns against letting this balloon into a full streaming-protocol
rewrite (e.g. MOSH-style delta state sync) and instructs that batching be scoped as its own
epic with an explicit go/no-go decision, not silently bundled in as always-on.

Phase 6's stagger coordinator (`SessionDetailView.tsx`) already delays and jitters
multiple sibling terminals' resync triggers within the same browser tab/connection —
creating a natural coalescing point that did not exist before this project.

## Decision

Scope batching to exactly one shape: a new `BatchedCurrentPaneRequest` proto message
coalescing multiple `CurrentPaneRequest`s that are (a) bound for the same WebSocket
connection and (b) already delayed into the same tick by Phase 6's stagger coordinator.
Batching does not coalesce across connections, does not change the streaming transport
itself, and does not touch the resize/scrollback/input message types. Ship it behind its
own `terminal:resync-batching` flag (default off), with the decision to ever recommend
default-on **explicitly deferred** until Epic 5.1's compression benchmark (Task 5.1.1.2)
provides real wire-size numbers to compare against (Unresolved Question #1 in
`implementation/plan.md`).

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| General streaming-protocol rewrite (delta state sync) | Explicitly named as Out of Scope / a Rabbit Hole in `requirements.md`; far exceeds this project's appetite (Large, 3-6 weeks) and touches every message type, not just resync |
| Coalesce at the WebSocket framing layer instead of the proto layer | Would affect every message type sharing that connection (input, resize, scrollback), not just resync — expands blast radius past what Scope item 5 asks for |
| Ship batching default-on immediately, since the stagger coordinator already delays requests anyway | Skips the explicit go/no-go decision point `requirements.md`'s Rabbit Holes section requires; also has no comparative data yet against compression alone |

## Consequences

- `BatchedCurrentPaneRequest` adds one new message and one new `TerminalData` oneof field
  (Task 5.2.1.1) — a genuinely new wire shape, which is why this decision is recorded as an
  ADR rather than left as an implicit implementation detail.
- Per-request `resync_id` correlation must survive batching (Task 5.2.1.2) — each request
  inside a batch gets its own individually-tagged `TerminalOutput` reply, not one combined
  reply.
- The go/no-go decision itself is **not resolved by this plan**; Story 5.2.1 documents the
  decision point explicitly and a follow-up ADR (or an amendment to this one) should record
  the eventual outcome once compression's numbers land.
