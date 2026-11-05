# ADR-006: Acknowledgment keyed to the turn-end episode; ack bypass by reason

**Status**: Proposed
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Context (VERIFIED)

`checkSession` bypasses the ack snooze with `isActiveHighPriority := shouldAdd && priority <= PriorityHigh` (`session/review_queue_poller.go:901`). Priorities: Urgent=1, High=2, Medium=3, Low=4 (`session/queue/queue.go:55-58`). The comment at `:895-900` says approval, error and input are bypassed, but `ReasonInputRequired` is Medium (3), so a live input prompt is snoozed by `IsAcknowledgedAfterOutput()`. Requirement text and architecture D10 are correct; the research doc's claim holds.

Ack today suppresses "until the next output" (`LastAcknowledged > LastMeaningfulOutput`), plus a 5-minute re-add grace for Low/no-controller items (`:912-921`). For prose questions that means an ack is either resurrected by any spinner output or buries a genuinely new question.

## Decision

1. **Episode id**: each `TurnEnded` creates `EpisodeID = hash(sessionID, turnEndedAt)`. An ack records `AckedEpisodeID`. A new turn end is a new episode and un-acks; output without a turn end does not.
2. **Bypass by reason**, not priority: needs-human reasons `{ApprovalPending, InputRequired, ErrorState, ProseQuestion}` bypass the output-based snooze unless `AckedEpisodeID` equals the current episode. The bypass for the pre-existing reasons (`InputRequired`) is behind `attention_ack_bypass_fix` (live-settable, default off) because it changes ack UX for Medium items. The episode rule for `ProseQuestion`/`AwaitingDirection` is NOT flagged: those reasons only exist behind `attention_prose_question_detection`, and a Medium prose question must never be buried by the output snooze during the interval before the bypass flag is flipped.
2a. **Persistence (amended in repair iteration 1)**: `TurnEnded` (episode, kind, excerpt, source, at) and `AckedEpisodeID` are persisted as nullable ent columns, wired like `LastAcknowledged` (`session/ent_repository.go:340,580`, `session/instance_serialization.go:134,327`), so a restart neither drops an open prose question (the queue is rebuilt from `Determine`) nor re-raises an acked one. Rehydrating from the event log was rejected because it would make `session_event_log` a hard prerequisite.
3. Snooze gets an expiry (default 4h, table-tested) so a snoozed item cannot hide forever.
4. Ack never deletes event-log rows; it appends an `Acknowledged` event.
5. Bulk "Done" is limited to informational rows (class table in plan.md Attention Reason Classification: `AwaitingDirection`, `TaskComplete`, `UncommittedChanges`, `Stale`), consistent with tray ADR-008; the server enforces this (it rejects a needs-human row in a batch). Undo of a Done or "Not a question" is an `UnacknowledgeSession` that clears `AckedEpisodeID` only if it still equals the undone episode; if a newer episode exists the undo is a no-op ("already superseded"). Both are logged as `Acknowledged` events (undo carries `undone=true` in the payload; no new event kind).
6. Opening a session is an implicit "seen" (does not clear a needs-human item); explicit Done removes finished items.

## Consequences

Table-driven tests over (reason, controller-active, ack-before/after-output, episode old/new). Characterization test pins today's behavior with the flag off.
