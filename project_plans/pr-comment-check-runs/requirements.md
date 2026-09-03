# Requirements: Reduce PR/Issue Comment Noise, Prefer Check Runs

Source: backlog item `2375ca2e-2155-4165-a38a-214f1fd80e39`. This document was generated
directly from the backlog item's title, description, and acceptance criteria — no
interactive ideation interview was run (no user present in this session).

## Problem Statement

Automated PR/issue commenting today is driven ad hoc by whichever session/skill is
shepherding a PR (`github:pr-ship`, `github:pr-refine`, `code:review`, backlog-driven
autonomous shepherding) — there is no single fixed backend catalog of comment types.
Comment threads mix genuine action items (something the user needs to decide or respond
to) with routine status/progress updates (CI passed, retrying, still working) that have
no distinguishing visual or semantic marker. Users must read every comment to find the
ones that matter, eroding trust in the automation and risking missed real action items.

**Desired outcome:** automation posts a PR/issue comment only when there is something
the user needs to react to (a decision, a blocker, a question). Status information with
a clean pass/fail or state shape should be surfaced through GitHub's native Check Run
API (or PR status checks) instead of a comment.

## Context Already Established (do not re-derive)

- The Go backend itself posts comments sparingly. The one deliberate, backend-owned
  example is `forwardSyncCloseComment` (`server/services/backlog_github_forward_sync.go:32`),
  gated by an explicit "no silent automated action" convention — this is signal, not
  noise, and is **out of scope**.
- `PostPRComment` (`server/services/github_service.go`) and `Instance.PostComment`
  (`session/pr_tracking.go`) are generic posting primitives. Callers — agent sessions
  running skills — decide what and when to post. There is no fixed enum of "comment
  types" to audit in the backend; the noise originates from skill/session behavior over
  the life of a PR, not from one code path.
- The app already reads GitHub Check Runs when polling CI (`githubCheckRun` struct,
  `session/backlog_plugin_github_prs.go`), but no code path creates/sets its own check
  runs (`Checks.Create` or equivalent) today.
- This is as much a behavioral/policy problem (when a session should comment vs. stay
  silent vs. set a check) as a new-capability problem (check-run creation) — research
  must inventory actual comment sources across skills before a plan can lock in an
  approach.
- No duplicate found in the existing stapler-squad backlog or in a prior codebase
  keyword scan for "post comment" catalogs.

## Kano Classification

**Basic expectation (must-be), not a delighter.** Users don't rate "the bot didn't spam
me" as a bonus feature — they only notice a violation, and the violation actively
damages trust (a wall of comments trains the user to stop reading, risking a missed
genuine blocker). Treat this as a defect-reduction ticket even though the mechanism
(check runs) is net-new capability.

## RICE-style Signal (qualitative)

| Dimension | Rating | Justification |
|---|---|---|
| Reach | High | Affects every PR touched by an automated session (backlog shepherding, `pr-ship`, `pr-refine`, `code:review`) — most of this tool's PR volume. |
| Impact | High | Directly targets the owner's top PR-UX complaint (comment-thread trust erosion). |
| Confidence | Medium | The problem is well-evidenced; the exact noisy-vs-signal comment mix is unaudited, so confidence in the precise fix shape is lower than confidence the problem is real. |
| Effort | Medium-High | Spans a behavioral/prompt-convention change across multiple skills *and* new backend capability (check-run creation, possible auth/scope gap). |

## Acceptance Criteria

1. A clear, written convention exists (and is followed by the relevant skills)
   distinguishing "post a comment" from "set a check run" — discoverable by future skill
   work, not just this ticket's authors.
2. Comment volume from automated sessions on a typical shepherded PR drops measurably,
   with remaining comments being ones a user would agree required their attention
   (qualitative test: "would I have needed to read this to know the PR was fine?").
3. Routine pass/fail state (e.g., "backlog auto-review passed", "CI retriggered and
   green") is visible via check run / status check UI in the PR header, not as a
   discrete comment.
4. A user can glance at a PR's checks and comment count and know, without opening every
   comment, whether the PR needs anything from them.
5. The existing `forwardSyncCloseComment` behavior is unaffected (still fires, still the
   only backend-initiated comment of its kind).

## Out of Scope

- Redesigning backlog automation end-to-end, or any part of the autonomous
  pickup/implement/ship pipeline unrelated to comment/check output.
- Mandating a specific check-run taxonomy (names, granularity, which checks exist) up
  front — that's a planning-phase decision informed by the research audit.
- Migrating or touching `forwardSyncCloseComment` — it already follows the "no silent
  automated action" convention and is not an instance of the noise problem.
- Any change to how human (non-automated) comments are handled — scoped to
  automation-originated comments only.

## Open Questions (preserve for planning phase, do not resolve here)

- Which specific comment types posted today are noise vs. signal? Needs an actual audit
  across `github:pr-ship`, `github:pr-refine`, `code:review`, and backlog-driven
  shepherding session transcripts/skill definitions — not a guess from this ticket.
- Does GitHub's Check Runs API require a GitHub App installation / token scope beyond
  what the repo's current PAT or `gh` CLI auth provides? Needs verification before
  assuming check-run creation is a drop-in replacement for a comment.
- For "informational but occasionally useful" comments (e.g., CI failure root-cause
  analysis) — should these become a single PR bot comment that's edited in place (à la
  Danger/Renovate) rather than posting a new comment every time, as a lower-effort
  middle ground where a check run doesn't fit (check runs can't carry rich prose the way
  a comment can)?
- Should the "does this need a check or a comment" decision be encoded as an explicit
  convention in each skill's instructions, or centralized behind a shared
  helper/primitive that skills call instead of `PostPRComment` directly?

## Suggested Entry Point

`/sdd:full` — spans both a behavioral/policy convention change (when to comment vs.
check, applied across multiple skills) and new backend architecture (check-run
creation, possible auth verification). The research phase must produce the comment-
source audit before planning locks in an approach.
