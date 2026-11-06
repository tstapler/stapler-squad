# Requirements: backlog-noop-loop-detection

## Problem
Item e2430349 (PASS verdict, fix already shipped in PR #801, 0 commits ahead of main) was re-dispatched to /backlog/ship 70+ times in ~4h. Each session re-derived "nothing to ship" and failed `report_duplicate` (PERMISSION_DENIED, separate bug). When `report_duplicate` later succeeded, the item stayed in `review`, indistinguishable from a real in-review PR.

## Goals
1. Detect repeated no-op dispatches of one item and stop/flag them early (3rd-5th, not 70th).
2. Make "duplicate claimed, awaiting confirmation" visibly distinct from "PR in review".
3. Surface both on `/unfinished` and via notification, like existing StuckReasons.

## Non-goals
- The STAPLER_SESSION_UUID propagation fix (separate item).
- Cross-host claim dedup (#861); a general status-change tool (#283).

## Acceptance criteria
- New StuckReason (e.g. `repeated_noop_dispatch`): item with PASS verdict, in review/in_progress, with N (default 3, configurable) consecutive dispatches with no new commits/diff, gets an open stuck row, notification, and appears on /unfinished.
- Dispatcher refuses to spawn another ship/work session for an item with an open such row (until operator clears it); a loud WARN is logged at the 3rd identical retry.
- Item with a recorded `duplicate_ref=` claim exposes a `duplicate_pending` flag/derived state via RPC and shows a distinct badge in the backlog UI and /unfinished.
- `report_duplicate` idempotent no-op response states that confirmation is pending, and includes how the operator resolves it.
- Stuck row auto-resolves when item leaves review/in_progress, a new commit lands, or operator archives/confirms the duplicate.
- Unit tests for the detection decision function, dispatcher gate, and MCP response; frontend test for the badge; registry regenerated if RPCs change.
