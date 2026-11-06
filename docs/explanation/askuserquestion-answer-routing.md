# Why AskUserQuestion answers route the way they do

When a Claude Code session calls `AskUserQuestion`, the operator can answer from the
notification pane. The answer reaches the agent by one of four routes, chosen from the
state of the session. This page explains why the routes exist, in what order, and what
guards each one. Decision records: ADR-005 (the ladder), ADR-003 (the question store) and
ADR-004 (risk tiers), in `project_plans/notification-actions-and-agent-messaging/decisions/`.
Spike evidence: `project_plans/notification-actions-and-agent-messaging/research/spike-S1.md`.
For how to use the result, see `docs/how-to/use-inline-notification-actions.md`.

## The problem

Claude Code draws `AskUserQuestion` as a terminal dialog. Answering it from outside the
terminal means either typing into the pane or persuading Claude Code to skip the dialog.
Both have costs:

- A keystroke into the wrong dialog selects the wrong option. A keystroke into a pane
  where a person is typing corrupts their input.
- A hook that blocks while it waits for an answer removes the terminal as an answer path.

So the design tries the safest route that can work and falls through on a typed failure
(`server/services/question_answer_router.go`).

## The ladder

| Rung | Route | When | What it does |
|---|---|---|---|
| 1 | `hook_reply` | A durable answer already exists when the `PreToolUse` hook fires | The hook returns `allow` plus `updatedInput.answers`, so the dialog never draws. |
| 2 | `live_keystroke` | The session is running | Types keys into the on-screen dialog. |
| 3 | `resume` | The session is stopped and not archived | Stores the answer. Nothing is sent. |
| 4 | `read_only` | The session is archived, or the harness or dialog shape is unmeasured | Shows the question; stores an answer with state `saved_for_later` and revives nothing. |

The router maps session state to route in one place (`deliveryRouteFor`). Rungs 3 and 4
only write the durable row. The operator resumes the session through Open session, and an
agent reads the stored answer through `get_guidance_request`. No answer ever calls `Start`,
`RecoverFromStopped`, `restartForRetry` or `transitionToLocked(..., Active)`, so the
`noarchivedrevival` rule (`.claude/rules/noarchivedrevival.md`) is never bypassed. An agent
that registered a `guidance_answered` wake is woken when the row is answered
(`server/wakes/sources_guidance.go`).

### Why `hook_reply` is closed

Spike S1 passed on Claude Code 2.1.303: an `allow` plus `updatedInput{questions,answers}`
hook output suppressed the interactive dialog (0 of 100 polls showed `Enter to select`),
and the agent continued with the supplied answer. Spike S2 measured that a hook can block
300 s without Claude Code killing it, and that a timeout, exit 1, bad JSON, HTTP 500 or an
empty 200 fail open to the native dialog. Exit code 2 denies the tool call, so the hook must
never exit 2 for `AskUserQuestion` (ADR-005, "Spike results").

The route is still closed in code. `spikeS1HookReplyPassed` is a compile-time constant set
to `false`, and `stage:hook_reply` resolves on only when it is true
(`server/services/feature_flags_notification_actions.go`). The spike record says its result
awaits the owner's review, and ADR-008 gates the stage on that compiled-in spike result. Setting the flag does not open the route. Flipping the constant is a code change
the owner decides.

The hook also never blocks by default. `hook_block_ms` is 0, so the terminal stays an
answer path.

## The keystroke route

### Single question

One digit selects the option. No Enter. The router looks up the pending question, maps
the chosen label to its digit (`optionDigit`), then writes through the shared pane writer
(`PaneReplyCore`, `server/services/pane_reply_core.go`) under the terminal write lease. A
`DialogMatch` check against a fresh capture runs under that lease, so a dialog that
changed between the click and the write yields `stale` with zero bytes written, not a
wrong answer.

### Two questions, single then multi

A tabbed dialog needs several keys. The plan is `session.PlanTabbedAnswer`
(`session/question_tabbed_plan.go`). It supports one measured shape only
(`MeasuredTabbedShape`): exactly two questions, the first single-select and the second
multi-select. Anything else falls to the single-digit path or stays read-only.

The raw bytes are `Enter` = `\r` and `Down` = `\x1b[B`
(`session/pane_reply_sequence.go:27-28`). The recorded sequence for the measured shape,
from `session/testdata/reply_dialogs/claude_auq_twoq_keys.txt`:

1. `Enter` selects the focused option of question 1 and advances to question 2.
2. For each multi-select row: `Down` to move, `Enter` to toggle.
3. `Down` to the unnumbered `Submit` row, then `Enter` to open "Review your answers".
4. `Enter` on `Submit answers`, which is focused by default.

Each step carries the frame the pane must show right before its key, computed from the
answers and never read from the pane (`TabbedExpect`). One write lease covers the whole
sequence. If a frame check fails, the sequence stops and writes no further keys
(`SubmitReplySequence`, `session/pane_reply_sequence.go`). The tab-bar header does not
satisfy the single-question matcher, which is why the tabbed layout has its own matcher
(spike S1, "Check against ADR-010").

When some keys landed and the sequence then stopped, the outcome is `stale` with reason
`answer_aborted_partial` and detail "N of M keys sent"
(`server/services/question_answer_sequence.go`). Neither Retry nor a second Submit is safe,
because the dialog is in a half-answered state. The card offers Open session. The delivery
state is `aborted_partial`.

### Guards

- **`human_active`.** A visible-session answer is refused with `not_sent` and detail
  `human_active` when browser input reached that pane in the last 5 s. This matters because
  an answer to a visible session can collide with a person typing; hidden sessions have no
  person. The signal is the last browser-originated pane input recorded in
  `paneWriter.SendInput` (`server/services/human_activity.go`). A person who attaches with
  their own `tmux attach` or types through ssq-mux is not covered; ADR-004 lists that as a
  residual risk. Each refusal increments `human_active_refusal_total`.
- **Tier B local only.** A keystroke is Tier B and is refused on the remote listener
  (`desktop_only`). ADR-004 explains why: the local listener has no authentication, and the
  guard that actually protects a keystroke is `DialogMatch`, not caller identity.
- **Flags.** An answer on a visible session needs `stage:answer`, which needs
  `stage:render`. A hidden session needs the `hidden_session_reply` flag
  (`resolveAnswerQuestion`).

## Delivery states

A keystroke answer moves `pending -> sent -> acknowledged`, or ends in `delivery_failed`,
`stale` or `aborted_partial`. A stored answer ends in `saved_for_later`
(`session/domain/guidance_request_source.go`). The state is persisted on the row with a
conditional update, so a racing writer loses instead of overwriting.

`acknowledged` comes from the first of two signals
(`server/services/guidance_ack_detector.go`):

1. A `PostToolUse` receipt for `AskUserQuestion` from that session.
2. Two consecutive fresh captures 3 s apart that show neither the row's dialog nor any
   dialog.

At 60 s with no acknowledgement, the detector takes one more capture under the write lease:

- The row's own dialog still showing means the key was probably lost, so the state is
  `delivery_failed` (`still_showing`).
- No dialog at all counts as acknowledged.
- Anything else, or a capture error, leaves the row `sent` and **Unconfirmed**.

An indeterminate write is Unconfirmed at once. An Unconfirmed row stays in Needs you and in
the bell badge until the agent confirms, the operator marks it handled (browser-local) or
the session ends. A timer never demotes it, because that would hide an agent that may be
stalled. `guidance_answer_unconfirmed_total` counts how often this happens.

The first signal depends on the receipt hook. Only sessions launched with the
`post_tool_logging` hook send it, so other sessions rely on the capture signal alone. See
"Receipt coverage" in `docs/reference/notification-actions-observability.md`.

## Layout canary

The single-question matcher is pinned to fixtures from real Claude Code `v2.1.296`
dialogs (`session/pane_reply_test.go:18`); the tabbed fixtures
(`session/testdata/reply_dialogs/claude_auq_*.txt`) were captured on 2.1.303. A Claude Code update can change the layout. Every
match attempt counts into `question_dialog_match_total{result}`. When more than half of at
least 5 attempts in 1 hour mismatch, the canary raises one notification, "Question answers
paused", and hook-mirrored question rows degrade to `read_only` (question, options, Open
session, no answer control) until the share recovers
(`server/services/guidance_dialog_canary.go`). Agent-created rows use a durable route and
are unaffected. After a Claude Code update, run the layout re-check in
`docs/how-to/verify-notification-actions-rollout.md`, section 5.

## Measured live check

On 2026-10-19, Claude Code 2.1.303 ran in a tmux session on the private socket
`ssq-manual-3`. The keys were written by hand with `tmux send-keys -H 0d` (Enter) and
`tmux send-keys -H 1b 5b 42` (Down). The dialog was a Cache single-select plus a Letters
multi-select. Memcached was chosen for Cache, B and C for Letters, then submitted through
Review > Submit answers. The model reported `Cache: Memcached` and `Letters: B, C`. Result:
PASS.

What this proves: Claude Code 2.1.303 accepts those exact bytes in this dialog. What it
does not prove: it did not run ssq's own writer (`paneWriter.SendInput` and
`SubmitReplySequence`), so it is evidence for the key encoding only. The ssq path is
covered by `server/services/question_tabbed_integration_test.go` and the rollout checklist.
The repeatable procedure and a results table are in
`docs/how-to/verify-notification-actions-rollout.md`, section 4.
