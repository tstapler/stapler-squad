# Pre-mortem: session-revive-uuid-loss (reduced scope)

Imagine this ships and causes an incident or complaint six weeks from now. What
happened?

## 1. Duplicate work collision (highest-probability failure, specific to this item)

**Scenario**: This backlog item has already produced 6 overlapping `project_plans/`
directories (`cold-start-uuid-loss`, `cold-restart-uuid-recovery`,
`session-cold-start-uuid-loss`, `session-resume-uuid-fix`, `session-resume-uuid-loss`,
`session-revive-uuid-loss`) and at least one of them (`cold-restart-uuid-recovery`)
already shipped as PR #439 against a *different* backlog item ID for the *same* bug.
A second parallel session could implement the AC3 notification/badge work
concurrently under one of the other five directory names, landing a colliding
`ReviveOutcome`-shaped enum or duplicate `RevivedContextBadge` component.

**Mitigation**: Before starting implementation, grep for `ReviveOutcome`,
`EverHadConversationHistory`, `RevivedContextBadge` one more time (validation.md
already flags this) and check open PRs (`gh pr list --search "revive OR cold-restart OR
uuid-loss"`) for in-flight work under any of the other five names. This is a process
gap (six duplicate planning directories for one bug), not a code gap — worth a
standalone observation, not a blocker for this item.

## 2. Notification fires on every restart-churn cycle, becomes noise

**Scenario**: The bug report's own timeline shows a session restarted 2+ times in
under an hour (inactivity watchdog) before landing in the lost-history state. If
`FRESH_LOST_HISTORY` can be reached repeatedly for the same session (not just once),
`onColdRestoreLostHistory` fires a WARNING notification every cycle, and the user
tunes it out or files a "notification spam" complaint — trading silent data loss for
noisy false urgency.

**Mitigation**: Confirm (during implementation, not deferred) that once
`EverHadConversationHistory` is consumed into a `FRESH_LOST_HISTORY` outcome, the
*next* cold restore's `HasClaudeSession()` will be true from the fresh conversation
that Claude just started — so `EverHadConversationHistory` should get set `true` again
on the *new* conversation, and a second consecutive loss is a distinct, legitimately
repeatable event rather than a stuck flag. If restart churn is fast enough that this
still floods notifications, that's evidence for the requirements doc's explicitly
out-of-scope "reduce restart churn" item to get prioritized as a fast-follow — not a
reason to suppress this notification silently.

## 3. `EverHadConversationHistory` never gets reset, badge shows on every future fresh session

**Scenario**: A user explicitly runs "start over" (clears conversation), which correctly
resets `EverHadConversationHistory = false` per the plan — but if that reset is missed
at even one of the 3 `ClearConversationState()` callers plan.md's own Pattern Decisions
table names (stale-resume rejection, program switch, explicit RPC), the flag stays
stuck `true` forever, and every subsequent legitimate fresh start on that instance
incorrectly shows the "lost history" badge.

**Mitigation**: `ClearConversationState()` is a single function all 3 callers already
route through (confirmed: `instance_claude.go:278`) — resetting the flag there, once,
covers all 3 by construction (this is exactly `plan.md`'s own reasoning for choosing
that centralization). Add a test asserting the flag resets specifically via the RPC
path, not just a direct unit call, since that's the path a real user hits.

## 4. Badge/notification ships but nothing ever triggers it in practice

**Scenario**: The team implements `FRESH_LOST_HISTORY` detection, ships it, and it
never fires because in practice `DetectByPath`'s fallback (already covering AC1) is
good enough that genuine unrecoverable loss is now rare — the fix "works" but is
unverifiable and quietly bit-rots.

**Mitigation**: Not a blocker — AC3's value is defense-in-depth for the residual case
(no JSONL on disk at all: history file deleted, disk cleared, etc.), not the common
case AC1 already fixed. Confirm this expectation explicitly in the PR description so a
reviewer doesn't mistake low trigger frequency for a broken feature.

## Verdict

No pre-mortem finding blocks implementation. Finding #1 (duplicate-work collision) is
the one worth acting on immediately, outside code: dedupe/close the 5 sibling
`project_plans/` directories for this bug once this item's PR merges, so a 7th
planning pass doesn't happen next month.
