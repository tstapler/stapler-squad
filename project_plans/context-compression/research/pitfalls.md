# Pitfalls: Context Compression at 85% Threshold

Research for Phase 2, following on from `requirements.md`'s open architectural
question. Findings below assume the codebase-confirmed premise: Stapler Squad
drives Claude Code CLI as a PTY/tmux subprocess (`session/claude_controller.go`,
`session/instance_claude.go`), not via direct Anthropic Messages API calls for
the agent loop itself. All four areas below are pitfalls *specific to that
constraint*, not generic LLM-summarization caveats restated without grounding.

## 1. Redundant/conflicting compression vs. Claude Code's own auto-compact

**Confirmed**: Claude Code CLI already runs its own auto-compaction and emits
a live progress indicator in its status line — verified in existing test
fixtures cited by the sibling item `context-compaction-detection`
(`session/detection/testdata/claude_active.txt:5`): `esc to interrupt   10%
until auto-compact`. That project's requirements.md explicitly frames the gap
as *visibility* (surfacing the CLI's own compaction state in the UI), not
reimplementing compression — and this item's own requirements.md (lines
66–80) names that exact same open question but leaves it unresolved.

**Pitfall**: a Go-side compressor firing at a token-percentage threshold has
no way to know whether Claude Code's own auto-compact is about to fire (or
already firing) in the same window, because:
- The Go controller does not see Claude's internal context-accounting state
  directly — the only token data it can access is the JSONL transcript
  (`session/tokens/parser.go`), which is written *after* a turn completes,
  not a live per-request counter. Any threshold check is retrospective by at
  least one full turn.
- Claude Code's own auto-compact threshold is not configurable or observable
  from outside the subprocess; there is no API to ask "will you compact on
  the next turn." Two independent compaction systems racing on the same
  session is a real failure mode: Stapler Squad injects its synthetic
  "compact now" message at ~85%, Claude Code's own auto-compact fires
  moments later on a context that has *already* been compressed once —
  double-summarization, which compounds the information-loss problem in §3
  rather than avoiding it.
- Building a second compression engine duplicates work two sibling backlog
  items already scoped without overlap: `context-compaction-detection`
  (surfacing Claude's own compaction as a UI state) and
  `context-health-monitoring` (which explicitly deferred a "token-percentage
  signal" out of scope, citing dependency on token accounting landing first —
  see `project_plans/context-health-monitoring/requirements.md`'s Out of
  Scope section). Neither of those items proposes a parallel Go-side
  compression engine; this item is the only one that does, and it is the one
  most exposed to conflicting with the CLI's native behavior.

**Implication for planning**: the threshold-and-inject design as filed
assumes Stapler Squad is the *only* actor managing context size for the
session. It is not. Any plan must either (a) prove Claude Code's own
auto-compact can be disabled/deferred for the duration of Stapler-Squad-
managed sessions (unconfirmed — no CLI flag for this was found in this
research pass), or (b) narrow scope to *supplementing* Claude's own
compaction (e.g., proactive summarization only for the specific "middle
turns" Claude's own compactor discards, timed to run only when Claude's own
indicator is nowhere near firing) rather than racing it.

## 2. Injecting a synthetic user message into a `--resume`'d PTY session

**Confirmed mechanism**: there is no structured "inject assistant-visible
context" API in this codebase's Claude integration. The only channel from Go
into a running Claude Code subprocess is `ClaudeController.SendCommand`
(`session/claude_controller.go:452`), which enqueues literal text onto a
`CommandQueue` (`session/command_queue.go:125`) to be typed into the tmux
pane as keystrokes — indistinguishable, from Claude Code's perspective, from
a real human typing at the prompt. `SendCommandImmediate`
(`session/claude_controller.go:480`) bypasses the queue for immediate
delivery but the delivery mechanism is the same: PTY keystrokes, not an API
message object with a role field.

**Pitfall — queue semantics fight the goal**: `SendCommand`-queued text is
gated on the session reaching an idle/ready state before the queue drains
(standard pattern for this controller, consistent with the double-fire /
"awaiting clear" idle-gating pattern used elsewhere for PTY input —
`session/session_driver.go:661` `shouldSendOnce`). A compression turn that
must fire *proactively before* the context ceiling is hit therefore either:
- queues behind whatever the agent is currently doing and only gets typed in
  once Claude Code goes idle — by which point the context may already be
  higher than 85%, or Claude's own auto-compact may have already fired
  (see §1) — or
- uses `SendCommandImmediate` to force delivery now, which means typing text
  into the pane while Claude may be mid-tool-call or mid-response. Interrupting
  an in-flight tool execution with unrelated keystrokes has no defined
  behavior in Claude Code CLI; at minimum it risks the injected text being
  interpreted as freeform input appended to whatever prompt is currently
  displayed, not a new turn.

**Pitfall — no role separation**: because delivery is raw keystrokes, there
is no mechanism to mark the injected text as anything other than "the user
typed this." The REFERENCE-ONLY prefix (requirements.md's proposed wording)
is the *only* signal Claude Code will ever see distinguishing it from a real
user instruction — it lives entirely in-band, as plain text Claude has to
correctly parse and obey mid-conversation. If Claude's own attention drifts
(exactly the failure mode `context-health-monitoring/requirements.md`
describes as "confusion detection" — repeated apology/self-correction
language), there's a real risk the agent treats the injected summary as a new
active request rather than background reference, i.e. "the agent talking to
itself" concern in the task brief is not hypothetical — it's the default
outcome unless the prefix wording is unusually robust, and this codebase has
no existing test coverage or track record of *any* text reliably steering
Claude Code's turn-taking behavior (the closest precedent, `SendCommand`'s
callers, are used for ordinary user-facing commands, not framing/meta
instructions).

**Pitfall — `--resume` transcript integrity**: `session/instance_claude.go`
already treats `--resume` UUID handling as fragile (see the stale-UUID
recovery path at `session/instance_claude.go:17-95`, which exists precisely
because a bad `--resume` state silently degrades the session and requires
detecting a specific staleness string in output to auto-recover). Injecting
a large synthetic message mid-transcript adds another way the resumed
transcript can diverge from what a human operator would expect to see
scrolling back — there is no rollback path today if an injected compression
turn turns out to be malformed or mistimed, matching the class of problem
`instance_claude.go` was written to detect and recover from, but for a
new failure mode it doesn't cover.

## 3. LLM summarization pitfalls specific to this codebase's shape

**Existing cheap-model integration found**: `server/services/anthropic_client.go`
already wires up `claude-haiku-4-5-20251001` (`anthropicModel` constant,
line 14) via `AnthropicAIClient`, gated on a resolved `Credential`
(`server/services/credentials.go:154`, `ANTHROPIC_API_KEY` or CLI OAuth token
per `credentials.go:216-228`). This is currently used for AI rule generation
(`server/services/rules_service.go:620`, `server/services/session_service.go:310-321`)
— **not** for any conversation-summarization purpose. Reusing this client for
compression is plausible (the integration point exists) but note:
- It fails closed today when no credential is configured
  (`session_service.go:321`: "AI rule generation unavailable: set
  ANTHROPIC_API_KEY or install claude/gemini/opencode CLI"). A compression
  feature built on the same client inherits the same fail-closed gap —
  sessions running under Claude subscription OAuth-only auth (explicitly
  called out at `credentials.go:216`, "Claude Pro/Max subscription...no API
  key in this flow") may not have a usable auxiliary-model credential at all,
  meaning compression silently can't run for exactly the users most likely to
  be running long, context-hungry sessions.
- Model choice is hardcoded to one Haiku snapshot with no override — the
  compression prompt's "cheap auxiliary model" isn't itself configurable per
  the acceptance criteria; that's a scope decision to make explicit in
  planning rather than default to whatever `anthropic_client.go` already
  uses.

**Tool-call chain information loss**: `session/tokens/parser.go` parses the
JSONL transcript that already records tool name + args/results
(`session/tokens/jsonl_types.go`), which is the only structured (non-PTY-text)
view of the conversation Stapler Squad has. But per `session/tokens/doc.go`'s
stated privacy guarantee, "the actual text of user prompts, assistant
responses, file contents, or command outputs is NEVER stored" in the parsed
result — only token counts, tool names, and skill names. This means the
richest structured data source available for building a summarization input
does not retain the content a summarizer needs (file paths edited, line
ranges, diff content) — a compressor would have to re-derive that from raw
PTY scrollback text instead, which is unstructured and where the standard
summarization failure modes apply: losing exact file:line context for
in-progress edits, collapsing multi-step tool chains into vague prose, and
summary drift compounding if compression fires more than once per session
(each compression summarizing an already-compressed transcript rather than
raw turns).

**Cost/latency not accounted for in acceptance criteria**: firing a second
model call synchronously at an 85% threshold during an active work session
adds real latency (an HTTP round trip to `anthropicAPIURL`,
`server/services/anthropic_client.go:13`) on the critical path of whatever
the primary agent is doing, and a real per-call cost on top of the primary
session's own token spend. Neither is mentioned in the acceptance criteria's
"Unit tests: threshold detection, head/tail protection boundaries, tool
output pruning" list — those are pure-function tests; none of them exercise
the live network call, its failure modes (timeout, 4xx/5xx, malformed
response), or degraded behavior when the auxiliary call itself fails mid-task
(does compression silently no-op, or does it block the primary session?).

## 4. Thrash-prevention: what breaks if the "once per N turns" guard is missing or miscalibrated

The acceptance criteria's own phrasing — "not more than once per N turns" —
is a fixed-count cooldown. This codebase has direct, already-learned
precedent that fixed-count/fixed-time cooldowns are the wrong tool for
exactly this class of "did the triggering condition actually clear" problem:
`session/session_driver.go:661-673`'s `shouldSendOnce`/`dialogAwaitingClear`
pattern was built specifically because "a fixed time-based cooldown is not
sufficient here — a slow/loaded terminal can still be redrawing the same
already-answered [state] past any fixed window, and sending another
[action] then queues into whatever [state] appears next once the redraw
finally completes" (comment at `session/session_driver.go:145-148`, restated
at `:666-669` for a second, independent call site — i.e. this is a pattern
the codebase hit twice already, not a one-off).

Applied to compression: if the guard is purely "N turns since last
compression" rather than "confirm token usage actually dropped back below
threshold before allowing another fire":
- A session sitting right at the threshold boundary (e.g. oscillating
  84%→86%→84% as turns complete) could fire compression repeatedly at the
  earliest allowed turn each cycle, burning auxiliary-model calls without the
  head/tail-protected summary ever meaningfully reducing size (each
  compression summarizes a transcript that's already-mostly-summary from the
  last pass — the "summary drift compounding" problem from §3).
- Conversely, an N-turn floor with no ceiling on *how large* the context has
  grown between allowed compressions means a session that blows past 85% to
  95%+ within the cooldown window gets no relief until N turns elapse,
  precisely the silent-failure/degraded-output scenario requirements.md's
  Problem section describes as the status quo this item exists to fix.
- The correct analog to `shouldSendOnce`'s "awaiting clear" latch would be:
  block further compression attempts until the measured token percentage is
  observed back under some hysteresis band below 85% (not just "N turns
  passed") — turn-count alone doesn't confirm the compression actually took
  effect, the same gap the existing dialog-clear guard was built to close for
  PTY-output-detected state.

## Summary of open feasibility blockers for Phase 3 planning

1. No confirmed way to suppress or coordinate with Claude Code's own
   auto-compact — racing it is a real risk, not just a naming overlap with
   sibling items.
2. The only injection channel is PTY keystrokes via `SendCommand`/
   `SendCommandImmediate`, which are queue-gated (fights "proactive" timing)
   and carry no role/type metadata (the REFERENCE-ONLY prefix is doing all
   the work, in-band, with no track record in this codebase of steering text
   reliably surviving Claude's own attention).
3. The richest existing structured data source (`session/tokens`) deliberately
   excludes the content a summarizer needs (privacy guarantee at
   `session/tokens/doc.go`), pushing summarization back onto raw PTY text.
4. An existing cheap-model client (`anthropic_client.go`, Haiku) is reusable
   but fails closed for OAuth-only/subscription users and is not currently
   built for synchronous mid-session calls with defined failure/timeout
   handling.
5. "Once per N turns" as specified is the same shape of guard this codebase
   already learned is insufficient for a state-confirmation problem
   (`session_driver.go`'s dialog-clear precedent); a hysteresis-band design
   is the safer analog.
