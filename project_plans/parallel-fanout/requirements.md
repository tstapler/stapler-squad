# Parallel Fan-Out: Run One Prompt Across N Isolated Worktree Sessions

## Problem Statement

Stapler Squad can fork exactly one new session from an existing session's checkpoint (`ForkSession` RPC), but there is no "run the same starting prompt through N agents simultaneously in isolated worktrees, then compare results and pick a winner" workflow. This leaves a meaningful capability gap relative to Orca's Parallel Worktrees feature, which fans a prompt across up to five agents simultaneously.

The distinction matters: forking branches off *one agent's existing progress* (branching mid-run), whereas parallel fan-out runs the *same starting prompt* through several agents/configs at once so you can review the diffs side-by-side and pick the winner before any code lands. Stapler Squad already has all the infrastructure this needs (worktree isolation, multi-session dashboard view, per-session diff viewer) except two affordances: "launch N at once from one input" and "compare side-by-side and pick the winner."

## Goals

1. Let users run one prompt across N simultaneously created, isolated worktree sessions (fan-out).
2. Provide a side-by-side comparison view of the N sessions' diffs.
3. Allow a "pick the winner" action that merges the chosen variant's branch and archives the others.
4. Enforce safety guardrails (cap N ≤ 5; show a cost/quota warning before launch) consistent with the existing `CapacityMonitor` / `QuotaGate` infrastructure.

## Non-Goals

- Automatic merge-conflict resolution across variants.
- Mid-run forking (that is the existing `ForkSession` workflow).
- More than five variants per fan-out (mirrors Orca's cap and keeps cost predictable).

## Acceptance Criteria

### AC-1 — Fan-out session creation
A "Run as N variants" action is available on session creation (and on any existing session) that creates N sessions with the same prompt/goal, each in its own git worktree, optionally varying agent/model per variant.

### AC-2 — Comparison view
A comparison view lets the user see the N sessions' diffs side by side, built on the existing per-session `DiffViewer.tsx` component rather than a from-scratch implementation.

### AC-3 — Pick the winner
A "pick the winner" action merges the chosen variant's branch into the base branch and archives (or discards) the others' worktrees.

### AC-4 — Guardrails
- Maximum of 5 variants per fan-out (enforced server-side; client shows an error for N > 5).
- A cost/quota warning is displayed before launch, consistent with the existing `QuotaGate` / `CapacityMonitor` integration.
- The `CapacityMonitor`'s guardrail system can pause or terminate fan-out sessions under cost pressure the same as it does for ordinary sessions.

### AC-5 — Documentation
The feature is documented in `docs/features.md` with a how-to covering the fan-out action, comparison view, and pick-the-winner flow.

## Source References

- Existing fork RPC: `proto/session/v1/session.proto` lines 284–287, 2266–2279
- Conversation-fork helper: `session/history_fork.go`
- Quota gate: `server/services/quota_gate.go`
- Capacity monitor: `server/services/capacity_monitor.go`
- DiffViewer component: `web-app/src/components/sessions/DiffViewer.tsx`
- Orca parallel worktrees reference: https://www.onorca.dev/docs/model/worktrees
