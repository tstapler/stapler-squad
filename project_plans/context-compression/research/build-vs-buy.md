# Build vs. Buy: Context Compression at 85% Threshold

## Question

Should stapler-squad build the originally-proposed custom Go compression
pipeline (token tracking → 85% threshold → cheap-model summarization →
synthetic message injection), or is the actual gap already covered —
fully or partially — by adjacent, already-planned work?

## The architecture constraint this analysis turns on

`session/claude_controller.go` drives Claude Code CLI as an interactive
subprocess inside a tmux pane (confirmed by prior parallel research). It
talks to the CLI via PTY bytes in and `--resume <uuid>` for continuity, not
via direct Anthropic Messages API calls. Two consequences, both verified
against this repo's code:

1. **Token usage is observable, but only after the fact, from disk.**
   `session/tokens/parser.go` and `jsonl_types.go` already extract
   `input_tokens`/`output_tokens`/cache tokens from the JSONL transcript
   Claude Code CLI writes to `~/.claude/projects/.../*.jsonl`
   (`session/tokens/jsonl_types.go:22-31`). This is not "API response
   metadata" observed live by the controller — it's a parse of a file the
   CLI subprocess produces as a side effect, on its own schedule (fsnotify
   pipeline, `session/tokens/store.go`). Per-turn token counts (acceptance
   criterion 1) are therefore **already computed**, as a by-product of the
   token-tracking/pricing feature — nothing new needs building for that
   half of AC-1.
2. **The controller cannot edit or replace the conversation history sent to
   the model.** Hermes's `ContextEngine.compress()` works because Hermes
   calls the chat-completions API directly and owns the `messages` array —
   it can literally delete the middle turns and substitute a summary before
   the *next* API call. stapler-squad's controller has no equivalent lever:
   `ClaudeController.SendCommand`/`SendCommandImmediate`
   (`session/claude_controller.go:452-514`) can only type additional text
   into the live PTY, which becomes one more turn appended to a transcript
   the CLI itself still holds in full. There is no mechanism in this
   codebase (and none exposed by the CLI outside of its own internal
   `/compact`) to remove or replace prior turns from what the model
   actually sees on the next request.

This second point is the load-bearing finding: "inject a synthetic message
that reduces context" is not just harder to build here, it does not
compress anything even if built, because injection only appends — it never
subtracts. The technique the requirements doc names (`should_compress` →
`compress()` → synthetic replacement message) is specifically a
messages-array-ownership pattern, and this architecture doesn't own the
messages array.

## Option 1: Build custom (as originally proposed)

Go controller tracks tokens, calls a cheap model (e.g. Haiku) to summarize
turns `[head+1..tail-N]`, injects the summary as a synthetic user message
wrapped in the REFERENCE-ONLY prefix.

- **Token tracking (AC-1)**: Buildable, and largely already built —
  `session/tokens` already parses per-turn usage from the JSONL transcript.
  A threshold check (`used/context_length > 0.85`) is a small addition on
  top of existing infrastructure. This slice alone is low-risk and would
  overlap almost entirely with `session/tokens/context_health.go`'s
  existing signal-extraction shape (see `context-health-monitoring`'s plan)
  — it is not, by itself, a reason to build a whole new pipeline.
- **Cheap-model call (compress step)**: Buildable and has direct precedent
  — `server/services/anthropic_client.go` (`AnthropicAIClient`, model
  `claude-haiku-4-5-20251001`, credential-chain auth) and
  `server/services/cli_ai_client.go` (CLI-fallback one-shot invocation via
  `claude --print`) already exist and are already used for exactly this
  kind of one-shot narrative-generation task by
  `session/session_summary_service.go`'s `SessionSummaryGenerator`. So "call
  a cheap model to summarize" is not new architecture — it's a proven,
  reusable capability.
- **Synthetic message injection that achieves compression**: **Not
  buildable as specified**, per the architecture constraint above. The
  controller can inject a message, but injecting a message does not remove
  the turns it's meant to replace from the CLI's own context — the CLI
  still holds (and sends to the model) everything in its own transcript.
  At best, injection could *add* a short "orientation" message on top of an
  already-full context, which is the opposite of what "compression at 85%"
  is supposed to achieve. It would not measurably reduce the tokens sent
  on the next turn, and could push a session past its ceiling faster, not
  slower.
- **Interaction with the CLI's own auto-compaction**: Claude Code CLI
  already performs its own auto-compaction near its context ceiling
  (confirmed by `context-compaction-detection`'s research, including the
  documented `PreCompact`/`PostCompact` hook events). A Go-side 85%
  threshold firing independently would race the CLI's own compaction logic
  with no coordination mechanism between them — two systems both deciding
  "context is getting full" with neither aware of the other's threshold or
  actions.
- **Verdict: Not recommended.** The token-tracking and cheap-model-call
  legs are buildable (and substantially pre-built), but the core mechanism
  — replacing mid-conversation history with a synthetic summary — requires
  message-array ownership this architecture structurally does not have.
  Building the "compress and inject" half as literally specified would
  ship a feature that cannot do what its name promises.

## Option 2: Rely on Claude Code's built-in auto-compaction, surface it

Adopt `project_plans/context-compaction-detection/`'s already-fully-planned
work (regex/hook-based detection of the CLI's own auto-compaction, surfaced
as a `SubStatus`/badge) as the delivery vehicle for this item, instead of
reimplementing compression logic the CLI already owns.

- **Fit**: Strong for the "don't let context silently degrade unnoticed"
  half of the original problem statement. The CLI already does head/tail
  protection and mid-conversation summarization *itself*, natively, with
  full messages-array ownership stapler-squad's controller doesn't have —
  it's a strictly better implementation of the compression mechanic than
  anything buildable at the controller layer. `context-compaction-detection`
  already has a complete plan (`implementation/plan.md`,
  `pre-mortem.md`, `validation.md`), an ADR
  (`decisions/ADR-001-substatus-not-just-detectedstatus.md`), and a
  finished build-vs-buy pass recommending the regex-detector approach now
  and a `PreCompact`/`PostCompact` hook-based approach as a stronger
  fast-follow.
- **Gap this does NOT close**: Surfacing "compaction is happening" is a
  visibility feature, not a compression feature. It doesn't give a user a
  worse-case recovery path (Hermes's motivating use case) if the CLI's own
  compaction still isn't enough — e.g. if a session degrades badly enough
  that a clean restart is actually what's needed, a badge alone doesn't
  offer that action.
- **Verdict: Adopt, but it only satisfies part of this item's intent.**
  This is the right home for "detect and show that context management is
  happening" (a materially reduced version of AC-1/AC-2 — token-threshold
  *awareness*, not token-threshold *action*), not for the summarization/
  injection mechanic, which Option 1 shows isn't buildable as specified
  anyway.

## Option 3: Extend context-health-monitoring's handoff-summary direction

`project_plans/context-health-monitoring`'s requirements
(`requirements.md:24`) name "restart with a compressed handoff summary" as
the recovery path this whole class of feature is ultimately for — but its
**shipped/ready-for-implementation plan explicitly excludes it**:
`implementation/plan.md`'s "Explicit Follow-Ons (out of scope)" item 3
states verbatim: *`"Restart with summary" handoff-packet generation +
new-session creation (7-touchpoint registry)`*. `requirements.md:55,62`
gives the reason: "Restart with summary" is actually three separate
concerns (LLM summarization call, new-session creation across the
7-touchpoint registry, and defining what "counts" as context to carry
over), deliberately deferred so the health-detection iteration didn't
balloon.

So there is no existing *design* to extend yet for the handoff-summary
mechanic itself — only a named, deferred slot for it, plus (critically) the
signal that would trigger it: `context-health-monitoring`'s `ContextHealth`
verdict (green/amber/red, from loop-detection + confusion-phrase heuristics
over the JSONL transcript, `session/tokens/context_health.go`).

What already exists that a "restart with summary" implementation could
reuse, found by reading the actual code rather than assuming:

- **A proven async narrative-generation pipeline**:
  `session/session_summary_service.go`'s `SessionSummaryGenerator` already
  does — for session-completion narratives — everything a handoff-summary
  generator needs structurally: dispatch a cheap-model call
  (`AnthropicAIClient`/CLI-fallback) asynchronously, dedup in-flight
  generations per session, persist a status-tracked row (GENERATING →
  READY/ERROR) via ent, reconcile staleness on restart, and expose
  get/regenerate over ConnectRPC (`server/services/session_summary_service.go`).
  It is not a handoff summary today — its prompt/content is a
  "what happened" completion report (diff + decisions + cost), not an
  "active task + state to resume" packet — but the plumbing (async
  dispatch, dedup, persistence, polling, graceful degradation) is a direct,
  working template, not something to invent.
- **A trigger signal**: `context-health-monitoring`'s RED verdict is
  exactly the "should this session be handed off" trigger the original
  Hermes `should_compress()` was meant to provide, already computed from
  the same JSONL substrate token-tracking uses.
- **A well-known target for the new-session leg**: the 7-touchpoint session
  creation registry (`.claude/rules/session-creation-registry.md`) is a
  known, mechanical checklist — adding a "resume with handoff" creation
  mode is a bounded, previously-executed pattern (see the one-off-session
  reference implementation it cites), not exploratory work.

Assessed against this item's acceptance criteria:

| AC (as filed) | Satisfied by this direction? |
|---|---|
| Token usage tracked per turn | Already true today, via `session/tokens` (no new work) |
| Compression fires at threshold, thrash-guarded | Reframe as: `ContextHealth` RED verdict triggers a **restart offer**, not in-place compression — thrash-guarded the same way `context-health-monitoring`'s level-transition-only gating already works |
| REFERENCE-ONLY summary prefix | Directly adoptable verbatim into a new handoff-summary prompt/template — no reason not to reuse the exact wording |
| Tool-output pruning before summarization | Directly adoptable technique, same as above — apply when building the transcript excerpt fed to `SessionSummaryGenerator`'s (extended) prompt |
| Compression event shown in UI | Becomes: a "Restart with summary" action button on a RED-flagged session card, per `context-health-monitoring/requirements.md:24` — a clearer, more actionable UI surface than a passive "compressed" badge |
| Unit tests: threshold, head/tail, pruning | Same test shapes apply, scoped to a handoff-summary generator instead of a mid-session compressor |

- **Verdict: Strongest fit, but requires a new (currently-unwritten) SDD
  cycle**, not literal reuse of an existing plan. It reframes this item
  from "compress context live, mid-conversation" (Option 1, structurally
  unbuildable) to "detect degradation, then offer a clean restart seeded by
  an AI-generated handoff summary" — which both satisfies the *intent*
  behind the original acceptance criteria and reuses proven infrastructure
  (`SessionSummaryGenerator`, `ContextHealth`, the AIClient abstraction)
  instead of building `session/context_compressor.go` from scratch. This is
  meaningfully less new code than Option 1 as filed, because the
  hardest-to-build pieces (async AI-call plumbing, dedup, persistence) are
  already solved by `SessionSummaryGenerator`, and the trigger signal is
  already solved by `context-health-monitoring`.

## Option 4: Hermes's Python `ContextEngine` pattern verbatim

Not applicable as a code fork (different language, different architecture
— Hermes owns the messages array via direct API calls; stapler-squad
drives an opaque CLI subprocess). Confirmed out of scope per
`requirements.md`'s own framing.

The *techniques*, however, transfer regardless of which option above is
chosen, and should be carried forward into whichever wins:

- **Head/tail protection** — protect the initial task framing and the most
  recent turns from being summarized away; only compress the middle.
  Directly applicable to a handoff-summary generator's transcript
  windowing (Option 3), same as it would have been to a live compressor.
- **REFERENCE-ONLY prefix** — the exact wording quoted in
  `requirements.md:26-33` is worth reusing verbatim in a handoff-summary
  prompt, regardless of delivery mechanism, so the receiving session
  doesn't treat the summary as active instructions to re-execute.
- **Tool-output pruning pre-pass** — strip image blobs, long file reads,
  and diff dumps from the transcript excerpt before it's ever sent to the
  summarization model, both for token-cost reasons and prompt quality.
  Directly reusable regardless of option.
- **Proportional summary budget with an absolute ceiling** (e.g. 12k
  tokens) — reusable sizing heuristic for whatever generates the handoff
  packet.

## Recommendation

**Do not build the originally-proposed live mid-session compression
pipeline as specified (Option 1) — the core injection mechanism cannot
achieve compression given this architecture's PTY/subprocess design, since
the controller can append to but not edit the CLI's own conversation
history.** The token-tracking and cheap-model-call sub-pieces are
buildable, but they're largely redundant with infrastructure that already
exists (`session/tokens`, `AnthropicAIClient`/`cli_ai_client.go`) or is
already scoped elsewhere.

Recommended path, in priority order:

1. **Ship `context-compaction-detection` as already planned** (Option 2) —
   it's fully planned, unimplemented, and closes the "silent degradation,
   no visibility" half of the original problem cheaply and correctly, by
   surfacing the CLI's own (better) compaction mechanism instead of
   competing with it.
2. **Retarget this item (`context-compression`) as a new, reduced-scope
   SDD cycle for "Restart with a compressed handoff summary"** (Option 3)
   — the deferred follow-on `context-health-monitoring` already named but
   didn't plan. Use `context-health-monitoring`'s RED `ContextHealth`
   verdict as the trigger, extend/parallel `SessionSummaryGenerator`'s
   proven async-AI-call/dedup/persistence pipeline for the summary
   content, route the "restart" leg through the existing 7-touchpoint
   session-creation registry, and adopt the Hermes techniques (Option 4)
   verbatim inside that generator's prompt construction. This is
   substantially less new code than Option 1's `context_compressor.go`
   because the two hardest pieces (trigger signal, AI-call plumbing) are
   already built elsewhere — this cycle mainly needs to wire them together
   and add the new session-creation mode.
3. **Close out the original `context-compression` acceptance criteria as
   superseded**, replacing them with the reframed set in the Option 3
   table above, rather than carrying forward criteria (live compression,
   synthetic message injection) that this research shows are not
   achievable given the confirmed architecture.

## Sources

- Local repo (read directly, this pass):
  `session/claude_controller.go:98-631` (controller shape, `SendCommand`/
  `SendCommandImmediate`, no messages-array access),
  `session/tokens/jsonl_types.go:22-31`, `session/tokens/parser.go`,
  `session/tokens/doc.go` (JSONL-sourced token accounting, already built),
  `server/services/anthropic_client.go:1-40` (`AnthropicAIClient`, Haiku
  model, credential-chain auth), `server/services/cli_ai_client.go:1-60`
  (CLI one-shot fallback, `claude --print`),
  `session/session_summary_service.go:252-330` (`GenerateAndPersist` —
  async dispatch, dedup guard, panic safety, status-tracked persistence),
  `server/services/session_summary_service.go:1-120` (ConnectRPC surface,
  polling contract).
- `project_plans/context-compaction-detection/research/build-vs-buy.md`,
  `research/architecture.md`, `implementation/plan.md` — confirms this is
  a fully-planned, unimplemented item surfacing the CLI's native
  auto-compaction via `PreCompact`/`PostCompact` hooks or regex detection.
- `project_plans/context-health-monitoring/requirements.md:9,24,55,62`,
  `implementation/plan.md` (Explicit Follow-Ons item 3, lines 99-104) —
  confirms "restart with summary" is named as the ultimate goal but
  explicitly deferred/unplanned in the shipped health-monitoring slice.
- `project_plans/context-compression/requirements.md` — this item's own
  requirements doc, including the Hermes reference implementation excerpt
  and the Phase-2 open architectural question this analysis resolves.
- `.claude/rules/session-creation-registry.md` — 7-touchpoint checklist
  cited as the known mechanism for a new "restart with handoff" creation
  mode.
