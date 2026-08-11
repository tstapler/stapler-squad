# Research: Feature Landscape for Mid-Session Context Compression

Scope: only the "compress/summarize a long session's context automatically"
part of `context-compression/requirements.md`. Detection of Claude Code's own
auto-compaction is covered by `project_plans/context-compaction-detection/`
(not re-derived here). Broader context-degradation detection + "restart with
handoff summary" UX is covered by `project_plans/context-health-monitoring/`
(also not re-derived — summarized below only where it bears on compression).

## 1. Can stapler-squad inject a synthetic message mid-stream into a
   `claude --resume` conversation? — No, not in the Hermes sense.

Traced the actual write path: `session/claude_controller.go`'s
`SendCommand`/`SendCommandImmediate` (lines 452-512) enqueue a `Command` that
`session/command_executor.go`'s `ExecuteImmediate` (line 433) eventually
executes via `session/instance_tmux.go:448` `WriteToPTY` →
`session/tmux_process_manager.go:247` `SendKeys`. This is **literal keyboard
input into the interactive terminal** — the same mechanism a human typing at
the Claude Code prompt would use (`inst.SendKeys(msg + "\r")`, confirmed at
`session/autonomous_driver.go:240-258` and `session/session_driver.go:327`).

Consequences for the Hermes-style pattern the requirements doc proposes
(`compress()` splicing a summary into turns `[head+1 .. tail-N]`, protecting
head and tail):

- **There is no message array to splice into.** stapler-squad never talks to
  a raw chat-completions API from the controller — it drives a CLI
  subprocess over a PTY. The only "conversation" object it can influence is
  whatever the next literal keystrokes are, appended at the *current* tail.
  A synthetic message can only ever be a **new tail turn**, never inserted
  mid-history.
- **`--output-format=stream-json` (structured message-level control) is
  `--print`-only** — confirmed in `context-compaction-detection/research/build-vs-buy.md`
  via `claude --help`: it does not work in the interactive/resumed-session
  mode stapler-squad uses. So there's no alternate structured channel to
  reach into the conversation state either.
- **Timing edge case**: `SendKeys` racing a live turn. If injected while
  Claude is actively "Thinking…"/executing a tool, the keystrokes land in
  whatever the terminal is currently accepting (could be swallowed, could
  interrupt, could queue after `esc to interrupt` — behavior is
  state-dependent, not deterministic). `session/session_driver.go:476-489`'s
  own comment documents exactly this class of hazard for a different nudge
  path ("a SendKeys failure here... is very [consequential]"). Any
  compression-turn injection must gate on `GetCurrentStatus()`/idle state
  before writing, same as existing command queueing already does via
  `CommandExecutor`.
- **No way to "protect the head."** Because injection is append-only, the
  proposed technique of preserving early instructions verbatim while
  compressing the middle is not something stapler-squad's controller can
  enforce directly — that structural protection can only come from *what
  Claude Code itself already does* internally (see §2), not from
  post-hoc message-array surgery from the Go side.

**Bottom line**: the feasibility question the requirements doc's "Open
architectural question" section poses is answered — "inject a synthetic user
message mid-session" is possible only as *append a new turn now*, never as
history splicing. Any plan built on this item must drop the head/tail-splice
framing and either (a) reframe compression as "send Claude a compress-now
instruction as a new turn" (functionally: send `/compact`, or a
custom-instructions variant of it — see §2), or (b) not attempt in-band
injection at all and instead act only at session-boundary (new session
seeded with a summary — this is `context-health-monitoring`'s deferred
"restart with summary", not a mid-session mechanism).

## 2. Token usage tracking without API response metadata — already exists

The requirements doc's proposed step 1 ("Add token usage tracking to
`session/claude_controller.go` from Claude's API response metadata") is
unnecessary as scoped — the controller never sees API response metadata
(it's not calling the API directly, see §1), but stapler-squad already
computes equivalent data from a different, already-integrated source:

- `session/tokens/` parses the Claude Code CLI's own JSONL transcript files
  (`parser.go`, `store.go`) and produces `ParseResult.TurnTimeline
  []TurnStats`, where each `TurnStats` (types.go:29-37) carries
  `Input`/`Output`/`CacheCreation`/`CacheRead` token counts per
  assistant turn. `CacheReadInputTokens` on the most recent turn is a
  reasonable proxy for "context currently loaded," since Anthropic's
  prompt-caching mechanics mean a resumed conversation's cache-read count
  approximates prior-turn context re-sent.
  `context-health-monitoring/requirements.md`'s own Constraints section
  independently arrived at same pipeline (its `session/detection` "Superseded
  by Phase 2/3 findings" note explicitly names `session/tokens` as the
  correct source) and flags "token-percentage signal" as explicit
  **out of scope for that project**, deferred to a project that has landed
  token/context-window accounting — i.e. this item, or `token-monitoring`/
  `token-cost-tracking` (both already exist as separate `project_plans/`
  entries, unread in depth here — check for overlap before planning).
- **What's still missing**: a *denominator*. `TurnStats` has no
  `context_length`/model max-context field — pricing/model-family data lives
  in `session/tokens/pricing.go`'s `PricingTable` (keyed by normalized model
  family) but that's USD pricing, not context-window size. A "used/limit"
  percentage needs a small static table (model family → max context tokens,
  e.g. Sonnet 4.5's window) added alongside `PricingTable`, not per-turn API
  metadata.

## 3. Does `context-health-monitoring` already propose a compress/handoff
   mechanism? — No; it explicitly scoped it out, but recorded design intent.

Read `context-health-monitoring/requirements.md`, `implementation/plan.md`,
`design/ux.md`, `decisions/ADR-00{1,2}`. Findings:

- **Out of Scope, explicitly** (`requirements.md` line 55 and
  `implementation/plan.md` line 104): *"'Restart with summary'
  handoff-packet generation and the new-session creation flow it triggers —
  this touches the 7-touchpoint session-creation registry
  (`.claude/rules/session-creation-registry.md`) and deserves its own
  follow-on requirements/plan"*. The **Rabbit Holes** section
  (`requirements.md:62`) is explicit that this isn't a small button: *"is
  actually: summarization (calls back into an LLM), new session creation (7
  touchpoints), and state transfer (what 'counts' as context to carry
  over) — each a separate research/plan cycle."*
- The shipped slice of `context-health-monitoring` is **detection only**:
  loop-detection + apology-language heuristics over the `session/tokens`
  transcript pipeline, surfaced as an amber/red badge
  (`ContextHealth` + reason string) with **no on-click action** — confirmed
  in `implementation/architecture-review.md:88-92`: *"The badge itself does
  nothing on click — it has no destructive or navigational action... This
  matches... the requirements doc's explicit exclusion of the
  handoff/restart flow from this badge."*
- **No compression/summarization mechanism was designed**, not even as a
  stub — the plan's data model (`ContextHealth` status + reason) has no
  field for a generated summary, and `plan.md:78` confirms the value is
  "deliberately not persisted," recomputed from the transcript on every
  parse — i.e. nothing exists yet to hang a summary payload off of.
- **This item (`context-compression`) is squarely the "separate,
  follow-on requirements/plan" that `context-health-monitoring` deferred to.**
  It is not redundant with that project; it is the next slice in the
  explicitly-named sequence (detect → [this item: compress] → restart/handoff
  UX). If this item's plan produces a summarization primitive, it should be
  built so `context-health-monitoring`'s deferred "Restart with summary"
  button can consume it later, rather than each project inventing its own
  summary format.

## 4. What is Claude Code's own built-in auto-compaction actually doing?
   (bears directly on "should we reimplement this")

From `context-compaction-detection/research/features.md` and
`research/build-vs-buy.md` (both already-verified, not re-derived here):

- Auto-compaction **already triggers automatically near ~95% context
  capacity** and **already replaces history with an LLM-generated summary**
  — confirmed via multiple `anthropics/claude-code` GitHub issues (#5243,
  #17808, #26518) and a community comparison gist. This is functionally the
  same operation the requirements doc's `ContextEngine.compress()` proposes
  to reimplement — Claude Code is already doing head-preserving,
  auto-triggered, LLM-summarized compaction *inside the subprocess*, without
  stapler-squad's involvement.
- Claude Code exposes **`PreCompact`/`PostCompact` hooks**
  (`https://code.claude.com/docs/en/hooks`, fetched 2026-08-06 per that
  research doc) that fire for interactive terminal sessions (unlike
  `stream-json`). `PostCompact` receives a `compact_summary` field on stdin
  — **Claude Code's own compaction already produces a summary payload that a
  hook can capture**, and `matcher: auto` vs `manual` distinguishes
  auto-triggered compaction from a user-invoked `/compact`.
  `custom_instructions` is accepted on `PreCompact`, meaning stapler-squad
  could *influence* (not replace) the CLI's own compaction — e.g. steering
  what it preserves — without reimplementing summarization at all.
- stapler-squad already has the exact infrastructure pattern needed to wire
  these hooks in: `server/services/hook_injector.go` +
  `hook_receivers.go` inject per-session HTTP hooks
  (`curl → POST {base}/api/hooks/<event>`), the same mechanism used for
  existing hook types. Adding `HookPreCompact`/`HookPostCompact` following
  that template was explicitly flagged by the compaction-detection research
  as a **viable alternative not adopted in that pass** (that item stayed
  scoped to PTY-regex detection) and recommended as a fast-follow.
- The `/compact` slash command itself can also be sent as a literal turn via
  the same `SendKeys` mechanism from §1 (it's just text + `\r`) — i.e.
  stapler-squad can already *trigger* Claude Code's own compaction
  on-demand today, at a threshold stapler-squad computes from `session/tokens`
  (§2), without writing any new summarization logic.

## 5. What unstated need is the requester actually chasing?

The requirements doc cites an external reference implementation (Hermes
Agent) that talks to a raw chat-completions API and owns its own message
history — a fundamentally different architecture from stapler-squad's
CLI-over-PTY model. Reading the acceptance criteria and problem statement
against what's now known:

- **Not "don't let quality degrade before the CLI reacts"** — that's
  `context-health-monitoring`'s territory (loop/confusion detection), and
  it's a *different* failure mode: quality degradation can occur well before
  95% context capacity, from causes compaction wouldn't fix (repetition,
  confusion). Nothing in this item's problem statement mentions loops or
  confusion language.
- **Not really "don't let the session hard-fail at the context ceiling"**
  either, in the narrow sense — Claude Code's own auto-compaction already
  exists precisely to prevent a hard context-limit failure (§4), and does so
  today with no code change. If avoiding a hard failure were the whole need,
  the item would already be satisfied by shipping
  `context-compaction-detection` (visibility) alone.
- **The actual gap, reading the problem statement's own words** ("silently
  fail or produce degraded output... no compression, no warning to the
  operator, no recovery path... has to manually restart and re-orient") is
  **operator visibility and trust in an opaque, automatic process**, not a
  missing compression capability. Claude Code compacts on its own with no
  operator-visible signal in stapler-squad today (that's exactly what
  `context-compaction-detection` is closing), and produces a summary the
  operator never sees or can influence (that's what `PostCompact`'s
  `compact_summary` payload could expose, per §4, but nothing surfaces it
  yet). The requester's Hermes reference — with its explicit
  REFERENCE-ONLY summary prefix, "## Active Task" section, and visible
  compression badge — reads as wanting **transparency and steerability of
  compaction**, not a competing compression engine: know when it happened,
  see/audit what got kept vs. summarized, and optionally shape what's
  preserved (`custom_instructions` on `PreCompact`).
- **Concretely, this suggests reframing the "compression" slice of this item
  around Claude Code's own compaction rather than reimplementing it**: wire
  `PreCompact`/`PostCompact` hooks (§4) to (a) capture `compact_summary` for
  display, satisfying the "surface compression events in the UI" acceptance
  criterion without inventing new summarization logic, and (b) optionally
  pass `custom_instructions` derived from stapler-squad's own state (e.g. an
  "Active Task" pointer, workspace context) to steer what Claude Code's
  built-in compaction protects — much cheaper and more consistent with the
  CLI's own model than a parallel `session/context_compressor.go` calling
  out to "a cheap auxiliary model" the controller has no existing
  integration point for. The one part of the original proposal that
  *doesn't* map to this reframing is the exact 85%-threshold, operator-timed
  trigger — Claude Code's auto-compact threshold (~95%, per §4) isn't
  configurable from outside the subprocess today, so "configurable threshold,
  default 85%" as literally specified would still require either (a) an
  operator-triggered proactive `/compact` send (deterministic, using
  `session/tokens` data from §2, no new model call) rather than an automatic
  85% trigger, or (b) confirming whether Claude Code exposes its own
  auto-compact threshold as a CLI flag/env var (not checked in this pass —
  open question for Phase 2 proper).

## Summary of what a plan should NOT do, based on this research

- Do not build `session/context_compressor.go` as a Go-side summarizer that
  calls "a cheap auxiliary model" directly — no such model-API integration
  point exists in the controller today (it drives a CLI subprocess, not a
  chat API), and Claude Code already performs equivalent summarization
  in-process.
- Do not design around mid-history message-array splicing (head/tail
  protection via array indices) — the PTY/SendKeys write path only supports
  appending a new tail turn.
- Do not re-plan "token usage tracking from API response metadata" — reuse
  `session/tokens`' existing JSONL-derived `TurnStats`, adding only a
  model-family → max-context-tokens lookup table for the denominator.
