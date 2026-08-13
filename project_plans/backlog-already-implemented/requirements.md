# Requirements: backlog-already-implemented

**Date**: 2026-07-14
**Type**: feature addition
**Complexity**: 2 — focused feature

## Problem Statement
When a backlog item's acceptance criteria are already satisfied by existing code — the implementation agent finds nothing to change — it has no reliable way to tell the reviewer that. The implementation agent's only free-text channels are `report_progress`'s `note` param (persisted to `AcCriterion.Note` but never read by `BuildReviewPrompt`, per `session/backlog_review.go:79-87`) and `request_review`'s `verification_notes` (which does reach the reviewer, but the reviewer prompt requires "specific, checkable claims" and there's nothing prompting the agent to use this field for a no-diff scenario). With an empty diff, `session/backlog_review.go:107-108,170-171` renders `"(no diff available)"` to the reviewer, which — per the reviewer system prompt in `session/headless/features.go:79-87` — causes every criterion to be marked UNVERIFIABLE. `session/review_gate.go:307` treats UNVERIFIABLE the same as FAIL/PARTIAL and calls `AutoReopenAfterFailedReview`, respawning a new work session. Because the underlying condition (already-implemented, no diff) doesn't change on rework, this repeats until `maxAutoReworkIterations = 3` (`server/services/backlog_service_triage.go:55`) is hit, at which point the item is silently parked with only an ephemeral WARNING notification (`notifyReworkCapHit`, same file, line 29).

## Baseline
Today: an already-satisfied backlog item burns up to 3 full rework cycles (each a fresh agent session) before getting stuck in `review`/parked state, discovered only if someone happens to check `/unfinished` or notices the WARNING notification before the process restarts (in-memory notify-once state, per prior investigation in `project_plans/backlog-stuck-item-visibility/`). The agent has no path to say "this exists already, here's where" and have that taken seriously.

## Users / Consumers
- The implementation agent (Claude Code session in a worktree) — needs a way to report "no diff needed."
- The reviewer (headless review LLM call in `session/review_gate.go`) — needs enough signal + tooling to confirm or refute that claim against the live codebase, not just a diff.
- Tyler (solo operator) — consumes the outcome via the backlog UI/notifications; currently absorbs the cost of stuck items and wasted rework cycles.

## Success Metrics
*(Quantified 2026-07-14 per triad review Product-lens finding — the original three bullets below were behavioral/binary; each now has an explicit, falsifiable target so "did this work" isn't a judgment call after ship.)*
- **Rework-cycle elimination**: an item whose acceptance criteria are genuinely already satisfied is verified (PASS or an honest FAIL naming what's actually missing) without cycling through the 3-iteration auto-rework loop for that reason. Target: 0 additional rework cycles attributable to this failure mode, measured over the first month post-ship by grepping `AutoReopenAfterFailedReview` invocations against items whose eventual outcome was `codebase-read-verified` PASS or a diff-grounded FAIL/PARTIAL (i.e., cases where the empty-diff path *should* have resolved it on attempt 1). Baseline: today, 100% of such items burn all 3 cycles before parking.
- **Grounded verdicts, bounded degradation**: the reviewer's verdict in an empty/near-empty-diff case is grounded in an actual look at the current codebase, not a blind UNVERIFIABLE stamp — a false "already implemented" claim from the implementation agent should still be caught and rejected. Target: `path=codebase-read-degraded` (Story 2.2.4) represents <10% of all empty-diff reviews after the first month, per Story 2.5.2's logged `duration_ms=`/`path=` data — a higher rate signals `CodebaseReadCallTimeout` or the `tool_reads` heuristic needs tuning, not that the feature is working as intended.
- **Note surfacing**: `report_progress`'s per-criterion `note` field (already collected today, currently dropped) is included in what the reviewer sees. Target: 100% of reviews with a non-empty criterion `Note` render it in the prompt (verified directly by Story 1.2.1's unit tests — this one is binary/structural, not a rate to observe post-ship).

## Appetite
Medium (1–2 weeks) — **as originally scoped in Phase 1 ideation.**

**Scope growth note (added 2026-07-14, post-planning):** the implementation plan that resulted from Phases 3–4 grew substantially past this original estimate — two adversarial-review repair passes added a hard-blocking pre-implementation smoke-test gate, a runtime self-check subsystem, a `tool_reads`/`os.Stat` anti-gaming corroboration layer, 4 additional adversarial integration fixtures, and a `Pool.CallBlocking` API consolidation now split into its own separate PR. This was accepted rather than scoped back down, because each addition closed a specific, concrete failure mode a real adversarial reviewer identified against the anti-gaming success metric above — which requirements.md itself calls "the load-bearing guardrail" — not speculative gold-plating. The honest read: this shipped as a Medium-to-Large appetite feature, not strictly Medium, and that growth should inform future SDD complexity-scoring for security/trust-boundary features specifically (anti-gaming/verification-integrity work tends to reveal more required scope during adversarial review than a first-pass Complexity-2 estimate anticipates) rather than being treated as scope creep to walk back now that the plan is validated.

## Constraints
None hard — no deadline, no compliance surface, solo-maintainer project.

## Non-functional Requirements
- **Performance SLO**: not specified — a codebase-verification review call may run longer/cost more tokens than a diff-only review; acceptable given the current failure mode is 3x full rework sessions instead.
- **Scalability**: not applicable (single-operator backlog).
- **Security classification**: internal.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Surfacing `report_progress`'s existing `note` field to the reviewer (currently persisted but never read).
- Giving the reviewer a way to check acceptance criteria against the *current* codebase state when the diff is empty or when the agent has flagged "no diff needed" — not just the raw diff text.
- Reviewer-prompt changes so an empty diff is not auto-treated as UNVERIFIABLE-for-everything, while preserving the reviewer's ability to reject an unsubstantiated or false "already implemented" claim.
- Any Go/prompt-level plumbing needed to get the implementation agent's evidence (existing file paths, function names, prior commit) in front of the reviewer for a no-diff item.

### Out of Scope
- Rework-cap value/behavior changes, or how items are surfaced once *stuck* (that's `project_plans/backlog-stuck-item-visibility/`, already planned separately).
- Auto-merge / PR-ready visibility issues (separate, deferred per existing project memory).
- Changing verification rigor or PASS/FAIL bar for the normal (non-empty-diff) review path.
- New session-creation modes, proto/session type registry changes — not applicable here.

## Rabbit Holes
- **Anti-gaming**: an implementation agent facing a hard criterion could be tempted to claim "already implemented" to dodge real work. The reviewer must independently verify against the live codebase before crediting the claim (per user decision below) — this is the load-bearing guardrail and must not be weakened for convenience.
- Reviewer needing broader codebase read access (beyond the diff) is a capability change to the review-gate call, not just a prompt tweak — could touch how much context/tooling the headless reviewer call gets. Needs explicit design in Phase 3, not assumed to be "just a prompt edit."
- Distinguishing "genuinely empty diff, already done" from "genuinely empty diff, agent did nothing" — both look identical at the diff level; the reviewer's codebase check is what has to carry that distinction.

## Alternatives Considered
- **Trust specific claims without re-verification** (lighter-weight, described in the trust-model interview but not chosen) — rejected because it re-opens the same anti-gaming risk this project exists to close, in exchange for saving reviewer tokens.
- **Just fix the note plumbing, leave UNVERIFIABLE-on-empty-diff as-is** — rejected as insufficient; it still leaves the reviewer unable to confirm the claim, so the same rework loop would fire whenever a criterion isn't "checkable" from note text alone.

## Feasibility Risks
- Reviewer codebase verification is more expensive (time/tokens) per empty-diff review than a diff read — acceptable trade against 3x rework sessions, but should be measured, not assumed.
- Risk of prompt regressions on the *normal* (non-empty-diff) review path while touching shared reviewer-prompt code (`session/headless/features.go`, `session/backlog_review.go`) — needs test coverage for both paths, not just the new one.

## Observability Requirements
Not required (complexity 2). Nice-to-have if cheap: log when a review is graded via the no-diff/already-implemented path vs. the normal diff path, to make future tuning possible.

## Risk Control
Not required (complexity 2) — this is a review-logic change with existing test coverage patterns (`session/review_gate.go`, `session/backlog_review.go` already have tests per repo conventions); no feature flag needed, rollback is a normal revert.

## Open Questions
- **Resolved by Phase 2 research, with a disagreement to settle in Phase 3 planning**: codebase access for the empty-diff case. Two viable approaches surfaced: (a) extend `headless.CallOptions` with `AllowedTools`/`PermissionMode` fields (mirroring `session/instance_tmux.go:buildClaudeCommand`) so the headless `claude -p` reviewer call gets bounded `Read,Grep,Glob` access scoped to `WorkDir: item.RepoPath` — recommended by `research/architecture.md` and `research/build-vs-buy.md`; or (b) do the codebase check deterministically in Go (targeted grep/read, injected as a labeled prompt section) rather than giving the reviewer an agentic tool loop at all — recommended by `research/pitfalls.md` as more consistent with this codebase's existing preference for deterministic Go-side plumbing (e.g. `RecoverBaseCommitSHA`) and avoiding new latency/cost/injection-surface risk. `architecture.md` also flags an unresolved factual discrepancy: it found `backlog_service_triage.go` already runs a headless call with `WorkDir` set and *no* `--allowedTools` flag, yet the model successfully writes files there — implying headless calls may already have more tool access than the "headless claude -p subprocesses do not have tool access" comment claims. Needs an empirical smoke test before Phase 3 locks in an approach.
- **Resolved — deferred to Phase 3 planning per user decision (2026-07-14)**: whether `verification_notes`/`note` should be required (blocking `request_review`) when the diff is empty, or optional-but-strongly-prompted. *(unresolved after Phase 2 research — user explicitly chose to let the architecture/adversarial review agents decide during planning rather than pre-committing.)*
- **Resolved by Phase 2 research**: the existing headless reviewer call has no file-read tool wired in today (confirmed via repo-wide grep — no `--allowedTools`/`--permission-mode` anywhere in `session/headless/`), but nothing prevents adding it — see codebase-access question above.
