# Research: Tool-Call Ergonomics (UX for an LLM caller)

There is no human-facing surface in this project — the web UI side is already shipped by
`backlog-event-driven-updates`. What follows is "UX" in the narrower sense that still applies:
does the tool's name, schema, and response text lead an LLM agent to call it correctly on the
first try, and to a *stopping* behavior rather than a new retry loop.

## 1. Established convention for MCP tool descriptions in this codebase

Read `server/mcp/tools_terminal.go:117-135` (`wait_for_output`) and every tool registration in
`server/mcp/tools_backlog.go:1716-1978` (`registerBacklogTools`). The convention is consistent
enough to treat as a template:

- **Opening sentence = one-line contract.** What the tool does, stated as an action + return
  shape, e.g. `wait_for_output`: "Wait until a pattern appears in the session's terminal output,
  or until timeout. Returns the matched line and recent output."
- **Second sentence = the failure/edge case, framed as expected, not exceptional.**
  `wait_for_output`: "On timeout, returns matched=false with the last-seen output — this is an
  expected outcome, not an error." This is the single most load-bearing sentence in the codebase
  for exactly the problem this project has to solve (see §3).
- **Role gating stated immediately when it applies** ("Role: work only — do not call from triage
  or review sessions") — `report_progress`, `request_review`, `submit_review_verdict`,
  `report_pr_created`, `report_duplicate`, `submit_triage_result` all open with this.
- **"Use X instead of Y" disambiguation** when two tools could plausibly be reached for the same
  moment — `wait_for_output`'s description ends with "Use run_command for most command
  execution," `report_duplicate`'s ends with "use request_review instead" for the SkipReviewGate
  case. This is exactly the guidance §4/§5 need.
- **Parameter descriptions carry the tool's business rules**, not just types — e.g.
  `report_pr_created`'s `pr_number` says "must match the number in pr_url,"
  `report_progress`'s `criteria_index` says "0 = first criterion." Bare type descriptions
  (`"UUID of the backlog item"`) are reserved for parameters with no surprising behavior.
- **Idempotency and retry-safety are stated explicitly** when they hold — `report_pr_created`:
  "Calling this again with the same PR after it already succeeded is safe (no-op)."
  `report_duplicate` has the identical sentence. This matters directly for the timed-out-retry
  case in §3.
- **Numeric bounds are enforced in schema, not prose** — `wait_for_output`'s `timeout_seconds`
  uses `mcpgo.DefaultNumber(30)` + `mcpgo.Min(1)` + `mcpgo.Max(60)`, not just a description
  saying "1-60". The MCP schema validates it structurally.

## 2. Naming: `wait_for_backlog_event` vs `subscribe_to_backlog_item` / `watch_backlog_item`

`wait_for_output`'s name already establishes the convention this project should follow:
`wait_for_*` in this codebase means "block synchronously inside this one tool call, return once,
with a bounded timeout." `subscribe_to_*` / `watch_*` are the names the *already-shipped*
ConnectRPC layer uses for its actual multi-event streaming primitive
(`BacklogService.WatchBacklogItems`, `useWatchBacklogItems`) — a long-lived connection that keeps
emitting events until explicitly torn down.

Reusing `subscribe`/`watch` for the new MCP tool would be a naming collision with a different
contract one layer down: an agent that has seen `WatchBacklogItems` mentioned anywhere (this
requirements doc, or backend code it reads while triaging) could reasonably expect a
`watch_backlog_item` MCP tool to behave like that stream — keep delivering events across multiple
turns, or require an explicit unsubscribe — when the actual implementation is a single
bounded-blocking call that returns once and is done, exactly like `wait_for_output`.
`wait_for_backlog_event` correctly sets the expectation: one call in, one event-or-timeout out,
same shape as the pattern-matching tool it's modeled on. **Recommendation: keep
`wait_for_backlog_event`** (or `wait_for_backlog_verdict` if planning narrows scope to verdicts
specifically) — reject `subscribe_to_backlog_item`/`watch_backlog_item` as misleading given the
existing streaming RPC those verbs already name in this codebase.

## 3. Timeout/error ergonomics — the actual design problem

This is the crux of the UX question, and the codebase already has the fix pattern: `wait_for_output`
returns timeout as `Success: true` with `Error: {Code: "WAIT_TIMEOUT", Message: "pattern %q not
found within %d seconds"}` — a normal, parseable "no match yet" result, not an exception. Two
things follow for `wait_for_backlog_event`:

- **Don't make the tool description alone carry the retry-discipline burden.** A description
  sentence like "on timeout, don't just call this again — call ScheduleWakeup instead" is easy
  for an agent to skim past mid-task, exactly the way the transcript in the Problem Statement
  shows an agent skimming past its own polling-budget reasoning. The stronger lever is the
  **schema-level `timeout_seconds` cap**, mirroring `wait_for_output`'s `Max(60)`. A hard cap
  means an agent literally cannot "just call this again with a longer timeout" as a way to avoid
  looping — the tool itself won't accept a timeout long enough to make that a viable substitute
  for a real polling strategy. This is a structural fix, not a wording fix, and it's the one that
  actually holds under an agent skimming its own tool descriptions.
- **The returned timeout message should still say what to do next**, following
  `wait_for_output`'s "expected outcome, not an error" framing, but pointed at the harness-level
  fallback rather than inviting a tight retry: e.g. `"no new event on item %s within %d seconds
  — call ScheduleWakeup for a longer interval before checking again, or call
  wait_for_backlog_event again only if you intend to keep this session blocked."` The key
  phrase to avoid is any wording that reads as "call this again" with no other qualifier — that
  is indistinguishable from the tight-loop bug this project exists to fix. State the two valid
  next moves (reschedule-and-return, or one more bounded wait) rather than a bare imperative.
- **Do not word this as "an error occurred."** Consistent with `wait_for_output`'s convention,
  timeout must stay `Success: true` — an agent that sees `Success: false` on a plain timeout is
  more likely to treat it as a failure worth immediately retrying (the classic exception-handler
  reflex) rather than a normal "nothing new yet" outcome that should feed back into whatever
  budget/backoff logic the calling session is running.

## 4. Before/after tool-call sequence

**Before** (from the Problem Statement's observed transcript pattern):

```
1. request_review(item_id, message)                 → item moves to review
2. ScheduleWakeup(interval="5m")                     → session sleeps
3. [wakes] get_backlog_item(item_id)                 → no verdict field present
4. "Still no verdict..." → ScheduleWakeup(interval="5m") again
5. [wakes] get_backlog_item(item_id)                 → still nothing
6. "Still waiting... checked twice now with no result..."
7. attempted: sleep 120                              → blocked by harness (long sleeps disallowed)
8. ScheduleWakeup(interval="5m") again → repeat 3-6 for N more cycles
```

Every one of steps 3/5's `get_backlog_item` calls is a wasted round trip when no verdict has
landed, and the harness-imposed floor on `ScheduleWakeup`'s interval is what bounds the actual
latency from "verdict recorded" to "session reacts" — not the event itself.

**After**:

```
1. request_review(item_id, message)                              → item moves to review
2. wait_for_backlog_event(item_id, timeout_seconds=60)
     → either: returns the VerdictRecorded event directly (verdict + summary), proceed immediately
     → or: times out, Success:true, message nudges toward ScheduleWakeup for a longer horizon
3. [only on timeout] ScheduleWakeup(interval="10m") — now genuinely interval-based waiting for a
   slow review, not a masked polling loop
4. [wakes] wait_for_backlog_event(item_id, timeout_seconds=60) again — one bounded check, not a
   get_backlog_item guess
```

The qualitative change: `get_backlog_item` is no longer in the loop at all for verdict-detection
— it stays a point-in-time read used to *interpret* an event once one arrives (or for a normal
"where do things stand" check), which matches this project's Non-Functional Requirement that
`get_backlog_item` "continue to work unchanged." The new tool absorbs the entire "wait for the
event" concern into one call plus, at most, one harness-level reschedule for the (rare) very-slow
review case.

## 5. Graceful degradation / backward compatibility

If `wait_for_backlog_event` is unavailable — an older client, a session spawned before the tool
was registered, or a `stapler-squad` binary predating this feature — the fallback is exactly the
pre-existing `ScheduleWakeup` + `get_backlog_item` polling loop described in the Problem
Statement. That path is unaffected by this project (Success Metrics: "Existing `get_backlog_item`
point-in-time reads... continue to work unchanged"), so there is a safe, already-battle-tested
degradation path with no new code required to support it.

The one real gap is **discoverability**, not correctness: nothing currently tells a session to
*prefer* the new tool once it exists, so an agent whose training/context predates this tool (or
that simply hasn't noticed it) will keep defaulting to the polling loop even after it's available
— silently forgoing the latency win with no error or signal that a better path exists. Two
low-cost places to close that gap, both already-existing conventions rather than new surface
area:

- `get_backlog_item`'s own response text (`server/mcp/tools_backlog.go:284-293`, the `case
  "work":` workflow-guidance block, step 4 at line 292) already tells a work-role session exactly what to do after
  `request_review` — step 4 currently reads "Do NOT end your session after request_review. Wait a
  bit, then call get_backlog_item again..." This is the single highest-leverage line to update:
  it is the exact place in the codebase that currently *instructs* the polling behavior this
  project exists to replace. Planning should treat updating this string (to instruct
  `wait_for_backlog_event` instead of "wait a bit, then call get_backlog_item again") as
  in-scope — without it, the new tool ships but the one piece of guidance steering sessions
  toward the old loop is left unchanged, and most sessions will keep polling out of habit/inertia
  even with a better tool sitting right next to it.
- No skill or CLAUDE.md documentation update is warranted beyond that — this is an internal
  dev-tool feature (per the requirements doc's Observability Requirements: "No new oncall alert
  — internal dev-tool feature"), and the tool's own MCP description plus the `get_backlog_item`
  guidance text are the two places a session actually reads at the moment this decision matters.
  A rules file or skill would be read at the wrong time (session startup, not
  "just-requested-review, what next") to actually change this behavior.

## Conclusion

There is a real, non-padding UX angle here, but it is narrow and concrete: (1) keep the
`wait_for_*` naming to match its true bounded-single-call contract rather than the `subscribe`/
`watch` verbs already claimed by the underlying streaming RPC; (2) enforce the anti-tight-loop
behavior structurally via a `timeout_seconds` cap, the same way `wait_for_output` already does,
rather than relying on prose alone; (3) keep timeout as a `Success: true` "expected outcome," and
have its message name the two legitimate next moves; (4) update
`get_backlog_item`'s existing work-role guidance text (`tools_backlog.go:292`) to point at the new
tool instead of instructing the old poll — this is the one place in the codebase actively teaching
sessions the pattern this project sets out to retire.
