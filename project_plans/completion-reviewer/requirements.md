# Requirements: Background Post-Completion Reviewer

Source: backlog item `b09c1b49-877f-4d2c-96ee-76a3d7a78442` — "feat: background post-completion
reviewer to build institutional memory" (migrated from
`TylerStaplerAtFanatics/stapler-squad#118`, filed 2026-05-29).

Complexity: 3 (feature / system design — new subsystem, cross-cutting hook into an existing
lifecycle state machine, hard security invariant around tool restriction).

## Problem

When a backlog item transitions to `done`, everything learned while working it — triage
rationale, review verdict, acceptance-criteria outcomes, decisions made mid-session — lives only
in session scrollback. Nothing feeds forward into future triage/work/review sessions, so the
fleet re-learns the same lessons on every similar item.

## Proposed Solution (from the issue)

A `session/completion_reviewer.go` hook that fires when a backlog item reaches
`BacklogStatusDone`:

1. Collect context: item title, description, AC snapshot with final statuses, triage notes,
   final diff size/summary, review verdict, review-session notes.
2. Spawn a short-lived, restricted Claude session with a focused prompt asking what future
   triage/work/review sessions should know, instructing it to write learnings to operator
   memory.
3. The spawned session gets a restricted tool list: memory-write only — no terminal, no
   delegation, no approval requests.
4. Wire the hook into `session/backlog_lifecycle.go` at the `done` transition.

Modeled on Hermes Agent's `agent/background_review.py` / `agent/curator.py` pattern: a forked
agent with only `(memory, skill_manage)` tools, inheriting the parent's prefix cache, writing to
disk without the main session ever seeing the write.

## Hard Dependency

Depends on the **operator memory** backlog item (originally issue #116) to provide the actual
memory store/write primitive. **VERIFIED as not yet implemented**: no `OperatorMemory`,
`MemoryStore`, or equivalent type exists anywhere in `session/`, `server/`, or `docs/registry/`
(repo-wide grep, 2026-08-06). This plan can be researched and designed now, but implementation
is blocked until #116 lands — the plan must produce a clean interface seam so the two efforts
don't have to be co-developed by the same session.

## Acceptance Criteria

- [ ] Completion reviewer fires on `BacklogStatusDone` transition (not on archive, not on other
      terminal states like superseded/failed).
- [ ] Spawned review session has no tools except memory-write, enforced at the session-builder
      level (allowed-tool list / capability gate in code) — not by prompt instruction alone.
- [ ] Reviewer never blocks the main workflow: fires from a background goroutine off the
      lifecycle transition; failures are logged, never surfaced to the user or the backlog item.
- [ ] Memory entries written by the reviewer are tagged with the source backlog item ID for
      traceability.
- [ ] Reviewer only fires when the item has non-trivial content: has a description AND at least
      one acceptance criterion recorded.
- [ ] Background reviewer only appends/updates memory — never deletes an existing entry.
- [ ] Pinned memory entries (if the memory store supports pinning) bypass any auto-transition
      the reviewer might otherwise trigger.
- [ ] Unit tests cover: context assembly from a completed item, tool-restriction enforcement
      (the restricted session literally cannot invoke a non-memory tool), and the no-op path for
      an item with no meaningful content.

## Out of Scope

- Building the operator memory store itself (tracked separately as #116).
- The periodic curator pass (staleness/dedup/pruning of accumulated memory) — Hermes describes
  this as a separate weekly job, not part of this item.
- Any UI surface for browsing what the reviewer wrote (follow-up, not required by the ACs above).

## Open Questions

- What does the operator-memory write primitive actually look like (function signature, storage
  location, format)? Unknown until #116 lands — this plan should define the *shape* this item
  needs from that interface, not assume a specific one.
- Is "restricted Claude session" here a real spawned `stapler-squad` session (tmux + full agent
  harness) or a lighter-weight direct API call with a constrained tool list? The issue's Hermes
  reference implies a forked in-process agent, which doesn't map 1:1 onto this codebase's
  tmux-backed session model.
