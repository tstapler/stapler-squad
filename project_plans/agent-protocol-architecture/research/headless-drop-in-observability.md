# Headless-by-Default with Drop-In Observability — Research

## The question

The team wants Claude Code (and potentially other CLI agents) to run headlessly by default
for background/autonomous work, but wants a human to be able to attach mid-flight and see
live progress — and ideally intervene — without the session having been interactive from
the start. This is Phase 2 research in the same thread as `research/mcp.md`, `research/a2a.md`,
`research/acp.md` and responds to a gap explicitly flagged in ADR-022 (see below).

## Part 1: What stapler-squad actually has today (three mechanisms, not one)

### 1. True headless — `session/headless/{pool,runner,caller}.go`

`Pool.call()` (`session/headless/caller.go:184`) launches `claude -p` via
`ProcessRunner.Run()` (`session/headless/runner.go:87`), which calls
`executor.StartProcess(ctx, bin, args, executor.WithNewSession(), ...)`. `WithNewSession()`
sets `Setsid: true` in `executor/managed_process_linux.go`'s `buildSysProcAttr` — a bare
detached subprocess with **no PTY at all**. Stdin feeds the prompt (kept off
`/proc/<pid>/cmdline` deliberately, per the comment in `runner.go`); stdout is scanned
line-by-line (resumed calls) or read whole and JSON-parsed (first call) into `StreamChunk`
values pushed onto a channel. Session continuity across calls comes from `claude -p --resume
<session_id>`, not from keeping the process alive — each `Pool.Call` is a fresh subprocess.

**There is nothing to attach to.** Once the subprocess starts, the only observability surface
is the stdout pipe already being consumed by the pool's own goroutine. A second observer would
have to either duplicate the pipe (impossible after the fact — pipes aren't broadcast) or wait
for `Pool.CallBlocking` / `drainChannel` to finish and read the persisted result.

Used by `session/backlog_triage.go` and `server/services/backlog_service.go`'s
`TriggerTriage` (line 1101) and the review-gate path (`spawnReviewGate`, referenced from
`TriggerReReview`'s comments). Concurrency is bounded by semaphores (`triageSem`, cap 8) and
governed entirely by context timeouts (30 min for triage).

**Git history confirms the ENOTTY failure mode the task warned about**: three separate fix
commits (`b35276ce`, `c4e2e84b`, `095e09e3` — "fix(headless): use Setsid instead of Noctty for
headless runner subprocess") had to move from `Noctty` to `Setsid` because `Noctty` calls
`ioctl(0, TIOCNOTTY)`, which requires the *parent* to already have a controlling terminal —
and when stapler-squad runs as a systemd user service (no TTY), that ioctl fails with `ENOTTY`
and the subprocess never starts. This is documented in-code at
`executor/managed_process_linux.go:11-29`. **Any redesign that puts a PTY under every headless
call must not reintroduce this exact bug class** — PTY allocation (`posix_openpt`/`openpty`)
has its own failure modes under a headless systemd context (no `/dev/pts` access issues are
less common than TIOCNOTTY, but resource/fd-limit exhaustion under high concurrency is a real
new risk that pure pipes don't have).

### 2. "Hidden" real tmux sessions — `TriggerReReview` and `hidden: true`

`TriggerReReview` (`server/services/backlog_service.go:1422`) spawns a session through the
exact same `CreateDirectorySession` path (`server/services/session_service.go:647`) as any
interactive session — full tmux + PTY, `session.NewInstance` + `instance.Start(true)` +
`session.StartSessionDriver(instance, path)` — but passes `hidden: true`.

Tracing `Hidden` (`session/instance.go:206`, `:451`, `:565`; `session/storage.go:112`):
it is **only** a list-filtering flag. `ListSessions` in `server/services/session_service.go`
skips instances where `inst.Hidden && !req.Msg.IncludeHidden` (lines 910, 950, 1859, 1891,
1925) — but `GetSession` (line 971) has **no such gate**. It looks the instance up directly
by UUID via the live poller (`reviewQueuePoller.FindInstance`) or external discovery, with zero
reference to `Hidden`. That means a hidden session is **fully attachable at any time** by
anyone who knows (or can look up) its UUID — the terminal stream, keystroke injection,
scrollback, everything works identically to a visible session. "Hidden" only means "absent
from the default list," not "inaccessible."

This is precisely the "drop-in" capability the team wants, already built, already shipping —
just not applied to true headless calls, and not surfaced as an intentional pattern (it reads
as an implementation detail of the backlog review-gate feature, not a general primitive).

### 3. Fully visible tmux sessions — the interactive default

Driven by `session/session_driver.go`'s terminal-scraping detection layer (regex-based
readline-prompt detection, keystroke injection with read-back verification, 2s poll ticker).
Full attach/observe/type at any time — this is the baseline UX stapler-squad is built around.

### The orchestration layer that sits on top: `session/autonomous_driver.go`

`AutonomousDriver` (confirmed via `session/autonomous_driver.go`: `NewAutonomousDriver`,
`Start`, `run`, `waitForIdle`, `buildOrchestrationPrompt`, `parseOrchestrationResponse`,
`ExtractPRURL`) is turn-continuation *policy* — decide whether to inject `NEXT_MESSAGE` or
declare `DONE`, with max-turns, stuck detection, rate-limit backoff — built on top of mechanism
3's scraping/injection primitives (`waitForIdle` takes a `detection.DetectedStatus` channel and
a `*ClaudeController`). It is orthogonal to the transport question: it needs *some* channel to
observe idle/active state and inject the next prompt, whether that's terminal-scraping, a
structured event stream, or an SDK callback.

### Why triage moved off mechanism 2/3 onto mechanism 1 — ADR-022

`docs/adr/ADR-022-headless-triage-over-autonomous-driver.md` is directly on point. The old
`TriggerTriage` spawned a tmux session and handed it to `AutonomousDriver`, and it had four
concrete failure modes documented in the ADR:

1. `Prompt` vs `InitialPrompt` field mismatch — the triage prompt silently never reached Claude.
2. **Silent nil-pool gate** — `StartAutonomousDriverWithTimeout` returns early (log warn only)
   when `headlessPool == nil`; the tmux session spins up and sits idle forever with no user-
   visible error.
3. **5-minute per-turn idle timeout fired mid-execution** — triage runs 4 parallel subagents
   for 8–15 minutes; the driver's fixed per-turn cap injected a spurious `NEXT_MESSAGE` turn
   mid-run, corrupting the subagent execution and causing premature `DONE` signals.
4. **No completion signaler** — `submit_triage_result` (an MCP tool) has no equivalent to the
   review gate's synchronous status transition, so the LLM orchestrator had to *infer*
   completion from scraped terminal tail text, and often got it wrong.

The ADR's own "Negative" consequences section is explicit about the tradeoff it accepted:
*"No tmux pane to inspect mid-triage. Operators cannot watch research happen in real time.
(Mitigated by: artifact files are written to disk during the call...)"* and flags as a
**Neutral / Future enhancement**: *"add a streaming progress UI using SSE or EventBus
artifacts-written events."* This research task is effectively answering that exact
future-enhancement note — with the added constraint that any answer must not reintroduce
failure modes 1–4.

**Constraint carried forward**: a "drop-in" redesign must not require reviving
`AutonomousDriver`'s turn-taking loop as the *only* way to get observability, since that loop
is precisely what produced failure mode 3 (fixed timeout racing against variable-length work)
and failure mode 4 (no reliable completion signal). Observability and turn-continuation policy
must be decoupled.

---

## Part 2: External research

### 2.1 ACP's `session/update` model — does it solve exactly this?

Confirmed against the spec (`agentclientprotocol.com`) and cross-checked with `research/acp.md`:

- **`session/update`** is a server→client (agent→editor) JSON-RPC *notification* (not a
  request/response) streamed during a `session/prompt` turn. Variants include
  `agent_message_chunk` (incremental LLM text), `agent_thought_chunk` (reasoning), `tool_call`
  / `tool_call_update` (tool execution lifecycle), `plan` (multi-step execution plan), and
  `available_commands`. This is structurally exactly "live observability without a full PTY" —
  the client renders these into a UI without ever touching raw terminal bytes.
- **Yes, attaching mid-session to a pre-existing session is a first-class capability, not a
  side effect**: ACP defines three separate mechanisms, gated by capability flags the agent
  advertises in `initialize`:
  - **`session/load`** (`sessionCapabilities.loadSession`) — full history replay. The agent
    "restores session context and conversation history, connects to specified MCP servers,
    and streams the entire conversation history back to the client via `session/update`
    notifications" before responding to the original request. This is precisely
    "human drops in and sees everything that already happened."
  - **`session/resume`** (`sessionCapabilities.resume`) — lightweight reconnect *without*
    replaying history; restores context and MCP connections and returns once ready to
    continue. Cheaper than `load` when the client doesn't need backfill.
  - **`session/list`** (optional) — discover existing sessions, optionally filtered by working
    directory — i.e., a client can enumerate what's running before deciding what to attach to.
  - The protocol explicitly frames this as enabling "persistence across restarts and **sharing
    sessions between different Client instances**" — meaning the client that started a session
    headlessly (e.g. a script, or no client at all beyond the initial `session/new` call) does
    not have to be the same client that later attaches to observe/drive it.

- **The open question is not the protocol, it's the implementation**: whether
  `agentclientprotocol/claude-agent-acp` (the healthiest available Claude bridge, per
  `research/acp.md`) actually implements `loadSession`/`resume` server-side was **not verified
  in this pass** — this is the same "feature-parity unverified" caveat already flagged in
  `research/acp.md`, now sharpened to a specific, testable claim: *does the bridge advertise
  `sessionCapabilities.loadSession` or `.resume`, and does it work against a session it did not
  itself just create in-process?* This is a five-minute check against the bridge's source
  before committing to it as the drop-in mechanism.

**Verdict on this sub-question**: ACP's protocol design is exactly the right shape for
"structured live observability + attach-after-the-fact," and it explicitly anticipates the
multi-client/reconnect scenario the team wants. It is not yet confirmed that the specific
Claude bridge implements the attach half of that shape.

### 2.2 Comparable prior art for "background process + optional live attach"

| System | Mechanism | Relevance |
|---|---|---|
| **tmux/screen** | Server process owns the PTY; detach just drops the client socket, the PTY and shell keep running; multiple clients can attach simultaneously and see mirrored output. | Exactly stapler-squad's own mechanism 2/3 already. No reason it can't back mechanism 1 too — the only obstacle is that headless calls are currently architected as short-lived, stateless subprocesses (session continuity via `--resume`, not via a kept-alive process), whereas tmux attach requires a long-lived process to attach *to*. |
| **`docker exec -it` / `docker attach`** | `attach` connects to the container's PID-1 stdio streams (a single logical stream, mirrored across attaches); `exec` instead spawns a *new* process inside the container's namespaces — it does not see PID-1's original output. | Important distinction for the recommendation: "attach" (shared view of the one running thing) is the right analogy, not "exec" (spawn a parallel process). A pipe-based headless call is more like neither — there's no PID-1 stdio to reattach to once the pipe's original reader has consumed it. |
| **`kubectl logs -f` / `kubectl attach`** | `logs -f` tails the structured/plain stdout stream — read-only, no intervention channel. `attach` connects to the container's stdio and can accept stdin if the container was started with `-i`, but is best-effort/fragile in practice. | Reinforces that "read-only tail" and "read-write attach" are different tiers of capability with different reliability profiles — worth offering both, not conflating them. |
| **systemd + `journalctl -f`** | The service is managed by systemd (itself a supervisor decoupled from any terminal); `journalctl -f` tails the structured journal. No built-in intervention channel — that requires a separate control mechanism (a D-Bus method, a signal, a socket). | Directly analogous to stapler-squad's own systemd-user-service model (`.claude/rules/systemd-user-service.md`). Confirms "supervisor + structured log tail" is a well-trodden pattern for exactly "background by default, observe on demand" — but intervention is a separate concern layered on top, not free. |
| **mosh** | Persists interactive session state independent of the underlying transport/network connection (SSP protocol resynchronizes screen state on reconnect). | Less directly applicable — mosh is about network resilience for an *already-interactive* session, not about starting headless and attaching later. Useful mainly as a reminder that "reattach" and "state resync" are a solved problem in general, just usually scoped to the terminal-emulation layer, not a structured event layer. |
| **Jupyter kernel/frontend split** | The kernel (compute) is a fully independent long-lived process; frontends (notebook, console, `jupyter console --existing`) connect over ZeroMQ pub/sub (`iopub` broadcast channel) + req/rep (`shell`/`stdin` channels). **Multiple frontends can attach to one running kernel simultaneously**, each receiving the same broadcast state, with zero coupling between "who started the kernel" and "who's watching/driving it now." | **This is the closest architectural analogy to what the team is asking for.** It decouples "the thing doing the work" from "the channel through which it's observed," exactly the property a PTY-based model doesn't have (a PTY has exactly one reader position in scrollback terms, even if multiple tmux clients can view it) and exactly the property a one-shot pipe fundamentally cannot have once the original reader has drained it. |

### 2.3 How other AI coding-agent platforms actually solve this

- **Anthropic's own Claude Code — Agent View, shipped May 2026** (`claude --bg "<task>"`, `/bg`,
  and the `claude agents` dashboard). This is the single most relevant data point found,
  because it's Anthropic solving *this exact problem* for their own CLI, after this
  repository's ADR-022 was written:
  - **Supervisor-based architecture**: the first `--bg` dispatch starts a per-user supervisor
    process, detached from any terminal. Each background session is an **independent Claude
    Code process, a child of the supervisor, not of the terminal that dispatched it** — so
    closing the shell, closing agent view, or starting a new interactive session does not
    affect the background work.
  - **Drop-in mechanism**: `claude agents` lists every session grouped by `Needs input` /
    `Working` / `Completed`. Selecting a row and pressing Space "peeks" (shows last output or
    the pending question) without leaving the dashboard; typing a reply and pressing Enter
    sends it inline — i.e., read (peek) and write (reply) are both available from the list
    view, no separate "attach to full terminal" step required for the common case. Pressing
    Enter opens the full transcript for deeper inspection.
  - **Worktree isolation**: background sessions move themselves into an isolated git worktree
    under `.claude/worktrees/` before editing files — the same posture stapler-squad already
    has via `session/` worktree management, for the same reason (parallel background sessions
    must not collide on a shared working tree).
  - Notably, **Anthropic did not build this on top of a PTY grid**. The publicly documented
    behavior (peek = "last output or pending question", reply = inline text) reads as
    structured session-state (a transcript/JSONL-like record plus a pending-question marker),
    not raw terminal bytes — consistent with the next point.
- **Anthropic's Managed Agents API** (`platform.claude.com/docs/en/managed-agents/*`,
  beta header `managed-agents-2026-04-01`) — a *fully separate*, cloud-hosted product from
  Claude Code, but directly answers "does Anthropic's own agent SDK/platform have a built-in
  session handle you can both drive programmatically and observe live":
  - Sessions emit a **persisted, typed event log**: `user.*` (message, interrupt,
    tool_confirmation, custom_tool_result, define_outcome), `agent.*` (message, thinking,
    tool_use/tool_result, mcp_tool_use/mcp_tool_result, thread_context_compacted), `session.*`
    (status_running/idle/rescheduled/terminated, error, updated, thread_*), and `span.*`
    (model_request_start/end, outcome_evaluation_*) — all following a `{domain}.{action}`
    naming convention, each with a server-recorded `processed_at` timestamp.
  - Critically, **`event_start`/`event_delta` are explicitly stream-only, never persisted** —
    a clean separation between the durable structured record (what a late-attaching observer
    replays) and the live incremental preview (what a currently-attached observer additionally
    gets, opt-in via `event_deltas[]`). A client that attaches after the fact gets the full
    persisted history; a client attached from the start additionally gets token-level deltas.
    This is essentially ACP's `session/load` (full replay) vs. streaming deltas, independently
    arrived at.
  - `user.interrupt` and `user.tool_confirmation` are the intervention primitives — structured
    RPC calls, not keystroke injection into a terminal.
  - This confirms the pattern at the vendor level: **Anthropic's own answer to "background
    agent + drop-in observability + intervention" is a structured, persisted, replayable event
    stream — not PTY exposure.** It is a different product surface (hosted Managed Agents, not
    the local CLI or the open Agent SDK used for local subprocess control), so it isn't
    something stapler-squad can adopt directly, but it is strong evidence for which
    *architecture family* to build toward.
- **OpenAI Codex** (cloud/background agent mode) — runs agents in a sandboxed cloud workspace
  in parallel, with plans to move toward triggered cloud-native jobs ("Codex Jobs"). Public
  documentation found in this pass did not detail a specific structured-event intervention
  mechanism at the level of ACP or Anthropic's Managed Agents API; the emphasis in available
  sources is sandboxing and network-egress control rather than the observability transport.
  Not a strong data point either way.
- **Devin (Cognition)** — real-time display of the agent's terminal, code editor, and browser
  with sub-50ms latency, plus bidirectional filesystem sync between local and cloud sandbox.
  This is closer to "stream a rendered terminal/IDE view" than "structured event log" — i.e.,
  Devin's public description leans toward the PTY/screen-share end of the spectrum rather than
  the structured-event end. Session management (init/pause/resume/terminate) is documented but
  the transport underneath "real-time display" was not detailed in available sources.
- **GitHub Copilot Workspace** — not independently re-verified in this pass; prior general
  knowledge is that it is closer to a plan/PR-review UI than a live-attach session model, so it
  wasn't pursued further as it doesn't bear on the "drop into a running process" question.

### 2.4 Claude Agent SDK — built-in support for this pattern?

The **Claude Agent SDK** (what `claude-agent-acp` wraps, per `research/acp.md`) supports:
- `resume` in `ClaudeAgentOptions` to reattach to a prior session's accumulated history
  (prompt, every tool call/result, every response) — written to disk automatically.
- `include_partial_messages` / `includePartialMessages` for character-level streaming.
- Persistent sessions that accept multiple messages without restarting the subprocess.

This is a **programmatic session handle**, not itself a UI-attach mechanism — it's the layer
`claude-agent-acp` sits on top of to produce ACP's `session/update` stream. It confirms the SDK
has the right primitives underneath (durable session state + streaming), but "attach a terminal
UI to it" is exactly the part ACP (or a bespoke equivalent) has to add on top; the SDK alone
does not ship a multi-observer attach protocol — that's what ACP's `session/load`/`resume` or
Anthropic's Managed Agents event log provide as separate layers over the same underlying
SDK-session concept.

---

## Part 3: Synthesis

### The three options on the table

1. **Formalize mechanism 2 (hidden tmux) as the default for everything** — every `claude` CLI
   invocation gets a real (possibly hidden) PTY/tmux session, cheap enough to always create,
   attachable on demand via the existing `GetSession`/attach path (already unauthenticated
   against `Hidden`).
2. **Keep headless (mechanism 1) for stateless calls; add a structured event-stream layer
   (ACP-shaped or Anthropic-Managed-Agents-shaped) as the observability mechanism, independent
   of whether there's a PTY underneath.**
3. **Hybrid**: headless by default, with an explicit "promote to attachable" escape hatch that
   spins up a tmux session and replays/tails when a human explicitly asks to look.

### Tradeoffs, concretely

**Option 1 (always-tmux) costs:**
- Every headless call currently launches a bare `Setsid` subprocess with no PTY allocation, no
  tmux server round-trip, no session-file bookkeeping. Converting every one of these (including
  the 8-concurrent-triage-call semaphore path and the review-gate path) to a real tmux session
  multiplies process/fd overhead by whatever tmux + PTY allocation costs per session, at
  whatever concurrency triage/review actually run at. This is a real, measurable resource cost,
  not hypothetical — it's the exact overhead ADR-022 was written to eliminate by moving *off*
  tmux+`AutonomousDriver` for triage.
- Reintroduces cleanup complexity: session lifecycle (start, monitor, tear down, orphan
  detection) that `session/headless` was specifically built to avoid. The "orphan-aware guard"
  logic already in `TriggerTriage` (lines 1131-1153) exists *because* tmux-backed sessions can
  go stale in ways bare subprocesses structurally cannot (a `Setsid` subprocess either is
  running or its stdout pipe is closed — there's no "is this tmux session actually alive"
  ambiguity to resolve).
- Still requires `session_driver.go`'s terminal-scraping detection layer to answer "is it done,
  did it hit a permission prompt" for the attached view — i.e., this option does not remove any
  of the fragility this whole research thread (and the sibling ACP research) is trying to get
  away from; it just applies the fragile layer to *more* sessions, including the currently-clean
  headless ones.
- Directly risks reintroducing ADR-022's failure modes if any orchestration is layered back on
  top (fixed timeouts racing variable-length subagent work, silent nil-pool-style gates,
  inferred-not-signaled completion) — not because tmux itself causes these, but because "now
  there's a tmux session, might as well drive it with `AutonomousDriver`" is exactly the
  regression path ADR-022 was written to close off.

**Option 2 (structured event layer, decoupled from PTY) costs:**
- Genuine new-build cost: there is no existing structured event bus in stapler-squad today.
  `session/headless`'s `StreamChunk` is close (`Text`/`Err`/`Done`/`CostUSD`) but is a
  single-consumer channel, not a persisted, multi-subscriber, replayable log. Building the
  "persist events + let a late subscriber replay + fan out live deltas to attached observers"
  layer (the thing both ACP and Anthropic's Managed Agents API converged on independently) is
  real engineering, not a config flag.
- Benefit: this is the only option that gives *both* headless-by-default (zero extra process
  overhead, no PTY/tmux ENOTTY risk class, no orphan-detection ambiguity) *and* genuine
  multi-observer drop-in — matching the architecture Anthropic itself shipped for both Claude
  Code's local Agent View and the cloud Managed Agents API, and matching what ACP already
  standardizes for editor integration. It also composes cleanly with `AutonomousDriver`'s
  policy layer: turn-continuation logic can subscribe to the same event stream a human observer
  uses, instead of needing bespoke `waitForIdle`/tail-scraping.
- Intervention (not just observation) requires the event layer to also carry a write path
  (structured "user message" / "interrupt" event types, mirroring `user.message`/
  `user.interrupt` in Anthropic's Managed Agents API or `session/prompt`/`session/cancel` in
  ACP) — read-only tailing alone (the `journalctl -f` tier) is not sufficient for "intervene."

**Option 3 (hybrid: promote-on-demand) costs/benefits:**
- Cheapest to build incrementally: headless stays exactly as-is (no risk to the ADR-022 fix);
  a human clicking "watch this" is a rare-enough action that spinning up a tmux session
  *at that moment* (not preemptively for every call) avoids Option 1's blanket overhead.
- The hard part is state handoff: a headless `claude -p --resume <session_id>` subprocess has
  no live process to "promote" once its current call has exited (each `Pool.Call` is a fresh,
  short-lived subprocess) — so "promote" really means "start a *new*, now-visible, `claude
  --resume <session_id>` invocation in a tmux pane, picking up the same conversation state,"
  not literally attaching to the in-flight process. This is achievable (the session ID is
  already tracked in `sessionState.sessionID`) but means the human is watching a *new* process
  continue the conversation, with a small gap between "click watch" and "pane is live" — good
  enough for "drop in and see what's happening," not true zero-latency attach to an
  in-flight tool call.
- Does not, by itself, solve sub-call-granularity observability (watching *inside* one 8-15
  minute triage call as it happens) — only inter-call granularity (see the result of the call
  that just finished, then watch the next one live). This is the same limitation ADR-022
  already accepted for triage's headless design.

### Recommendation

**Option 2, but scoped and sequenced realistically — not Option 1.**

Option 1 (always-real-tmux) is a regression relative to ADR-022 and should be rejected outright:
it reintroduces exactly the overhead and fragility-surface-area ADR-022 was written to remove,
for a benefit (attach) that a structured event layer gets more cheaply and without the tmux
lifecycle/orphan-detection tax.

The right target architecture is a structured, persisted, replayable session-event stream —
independent of whether a PTY exists underneath — matching the pattern both ACP
(`session/update` + `session/load`/`resume`) and Anthropic's own Managed Agents API
(`{domain}.{action}` event log + `event_start`/`event_delta` stream-only deltas) converged on
independently, and matching the architecture Anthropic shipped for Claude Code's own local
background-session feature (Agent View: supervisor process + structured peek/reply, explicitly
not a PTY-per-session model). Given three independent parties arrived at the same shape, this
is not a speculative bet — it's the validated direction.

But **do not build this from scratch as a big-bang layer**. Sequence it as a small, concrete
first step that extends what already half-exists:

**Concrete first step**: extend `session/headless`'s `StreamChunk` type into a persisted,
replayable event log, starting with triage (the lowest-risk, already-isolated call site named
in ADR-022's own "Future enhancement" note).

1. Add a `session/headless/events.go` (or extend `caller.go`) that, alongside pushing each
   `StreamChunk` onto the existing per-caller channel, also appends a structured record — feature
   key, `ItemSession` ID, event type (`text_chunk`, `error`, `done`), timestamp, payload — to a
   small append-only store keyed by the `ItemSession.ID` already created synchronously in
   `TriggerTriage` (line 1184) before the goroutine starts. SQLite (already the storage engine
   per `session/storage.go`/ent) is sufficient; no new infra.
2. Add one read endpoint — `GetItemSessionEvents(itemSessionID, afterSeq)` — that returns
   persisted events after a sequence number. This is the "late attach = replay" primitive
   (ACP's `session/load`, Anthropic's persisted event history) in its simplest possible form:
   polling, not even push-based yet.
3. Wire a minimal UI affordance: on a running/recent `ItemSession` row, a "view progress" action
   that polls this endpoint — directly closing the gap ADR-022 flagged ("no tmux pane to inspect
   mid-triage... future enhancement: SSE or EventBus artifacts-written events") without
   resurrecting `AutonomousDriver` or tmux for triage.
4. Only after that lands and proves useful, generalize: (a) add push (SSE, reusing the reconnect
   patterns already built in `837ba8cc`'s jittered-backoff session-watch/terminal-stream work)
   instead of polling, (b) add a write/intervention event type (structured "inject follow-up
   message" — meaningful once a headless call type exists that's long-running enough and
   resumable enough to accept one), and (c) evaluate whether this event layer should *become*
   the backing store the ACP spike (already recommended in `recommendation.md` as
   prototype-behind-a-flag) reads from — i.e., one structured session-event model serving both
   the ACP bridge and stapler-squad's own UI, rather than two parallel structured layers.

This step is deliberately small enough to not compete with the ACP spike for priority, reuses
existing storage/session-ID plumbing instead of inventing new infrastructure, and directly
targets the exact gap (triage has no live progress view) that motivated this whole research
question — without touching `AutonomousDriver`, `session_driver.go`, or the tmux-based
mechanisms 2/3 at all. Mechanism 2 (hidden-but-attachable tmux, used by `TriggerReReview`)
should be left as-is for the program types that need it (`plainProgram`/non-Claude flows where
no structured event API will ever exist) — it is not being replaced, just not generalized to
everything.
