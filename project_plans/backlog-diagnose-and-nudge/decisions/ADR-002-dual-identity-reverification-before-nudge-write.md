# ADR-002: Require BOTH `Snapshot()`- and Tmux-Pane-Marker-Based Identity Reverification Immediately Before Any Nudge Write

**Status**: Accepted
**Date**: 2026-09-27

## Context

`ce71ad1a-a6a5-485f-8245-c5a502754a8b` was a real incident: a stale tmux session left
behind under a name a new, unrelated `Instance` also resolved to was silently
reattached to on a name match alone, delivering one session's message into an
unrelated, attended session. `research/architecture.md` §1.4-1.5 confirms:

- A fix exists (`session/tmux/tmux_ownership.go`'s `ReadSessionOwnerUUID`,
  commit `6c9026e81`) but lives on an **unmerged** branch
  (`backlog/stapler-squad-verify-tmux-session-ownership`) and operates at the
  **tmux-session-reuse layer** (`start()`, `AttachToExisting`,
  `KillTmuxSessionByTitle`) — not the MCP write layer.
- Reading `server/mcp/tools_terminal.go:277,664` (`writeToSession`, `steerSession`) and
  `tools_lifecycle.go:399` (`resumeSession`) directly confirms **zero** ownership
  verification exists at the actual write call site today. Both layers ask different
  questions: "is this the pane I created?" (tmux-reuse layer, unmerged fix) vs. "is
  this still the pane I'm about to write into, right now?" (write layer, this ADR).
- `.claude/rules/instance-lock-free-reads.md` already mandates `Snapshot()` over raw
  field reads for `Instance` state — necessary but not sufficient, since it only
  answers "does the Go heap object still believe it's session X," not "does the actual
  OS-level tmux pane still carry session X's marker."
- This feature's nudge is a genuinely autonomous write with no human approval step —
  identical blast radius to `ce71ad1a` but worse, because no operator is present to
  notice a misdirected message immediately.

## Decision

The nudge-execution gate requires **both** checks, independently, as the two
requirements are answering different questions and neither subsumes the other:

| Layer | Question | Mechanism |
|---|---|---|
| Instance object (Go heap) | Does this `*Instance` still believe it is session X? | `inst.Snapshot().SessionUUID` (or equivalent field), never a raw field read |
| tmux pane (OS process) | Does the actual pane still carry session X's marker? | A new, purpose-built marker read-back at the write call site |

This feature adds its **own** equivalent of `ReadSessionOwnerUUID`, in a new file
(`session/tmux/write_gate_ownership.go`, deliberately **not** reusing or colliding with
the unmerged branch's `session/tmux/tmux_ownership.go` filename) rather than depending
on that branch merging — the requirements explicitly decline that dependency (Out of
Scope: "Merging or depending on the unmerged `ce71ad1a` tmux-ownership-verification
branch"). Both checks are composed into a single facade function,
`verifyIdentityImmediatelyBeforeWrite(ctx, inst, expectedSessionUUID)
(SessionIdentity, error)`, which:

1. Reads `inst.Snapshot()` and compares its session UUID to `expectedSessionUUID`.
2. Reads the pane's marker via the new tmux helper and compares it to the same UUID.
3. Performs these two reads back-to-back with **no I/O between them** — enforced by
   the facade's own structure (a single function body, not two call sites a future
   editor could separate) rather than by convention alone.
4. Returns a non-nil error on **any** ambiguity (mismatch, read failure, marker
   absent) — fail closed, never "verification skipped, proceed anyway."

The nudge write call site calls this facade as the literal last step before
`SendKeysWithTimeout`/`SubmitContentWithEnter`.

## Consequences

- Some logic (`STAPLER_SESSION_UUID` marker read-back) is duplicated between this
  feature's new helper and the unmerged branch's `tmux_ownership.go`. This is an
  accepted, documented duplication — flagged here as a follow-up consolidation item
  for whoever merges the `ce71ad1a` branch later: fold the write-layer check into
  the same package once both exist, but do not block this feature on that merge.
- The check is Instance-agnostic of *why* a mismatch happened (superseded, killed,
  reused) — any mismatch is treated identically: abort, log, route to
  `SafetyGateReasonIdentityMismatch`, never retried against the same target
  automatically (retrying risks a double-nudge per `research/pitfalls.md` §4).

## Alternatives Considered

- **Depend on / merge the unmerged `ce71ad1a` branch first** — rejected: explicitly
  out of scope per requirements.md, and blocks this feature's timeline on a decision
  (merging someone else's in-flight branch) outside this plan's control.
- **`Snapshot()` check alone** — rejected: does not close the actual `ce71ad1a` root
  cause (pane reuse under a stale name), which happens entirely outside the Go
  `Instance` object's own state.
- **Tmux marker check alone** — rejected: does not close the case where the `Instance`
  object itself has been mutated to point at a different session (e.g., by a
  concurrent `setGitHubResolutionLocked`-style background write) even if the pane's
  marker independently matches.
