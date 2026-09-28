# Stack Research: backlog-diagnose-and-nudge

Scope: identify which libraries/frameworks/patterns this feature needs, and confirm
what's already reusable from the shipped `context-compression` feature
(`session/handoff_summary_service.go`) and `session/autonomous_driver.go`'s nudge
mechanism, per the task brief's prior-art pointers.

## Bottom line

**No new Go module or npm package is required.** This is a composition of four
already-shipped subsystems (handoff summarization, autonomous-driver nudging, headless
LLM dispatch, MCP session-control tools) behind one new RPC and one new UI action. The
only genuinely new code is: the diagnostic-bundle assembler, the dispatch/safety-gate
orchestration, a new config block (cap/cooldown + feature flag), and the UI wiring.

## 1. Token budgeting for the ~250k-token bundle cap

**`session/tokens` does not provide a "count/estimate tokens in this text" function.**
It is a JSONL-transcript *parser* (`session/tokens/{parser,types,jsonl_types,store}.go`)
that extracts the exact `usage` object Claude Code's CLI already persisted for
completed turns — i.e., it reports tokens *already billed*, not a way to size
arbitrary new text (item description, log excerpts, git diff) before sending it. This
was independently confirmed in the sibling `context-compression` research
(`project_plans/context-compression/research/stack.md`, §2): no tokenizer library
(`tiktoken-go` etc.) exists in `go.mod`, and the codebase's answer to "what does a
piece of text cost" is a **bytes/4-per-token heuristic**, not a real tokenizer.

The already-shipped, directly reusable pattern for this is
`session/handoff_summary_excerpt.go`'s `SummaryBudget`/`newSummaryBudget`:

```go
// SummaryBudget.MiddleExcerptMaxBytes: "a rough bytes-per-token approximation
// (4 bytes/token) rather than an actual tokenizer count, since this repo has
// no tokenizer library and doesn't need one for a soft excerpt cap like this."
ceiling := resolved.MaxMiddleExcerptTokens * 4
proportional := totalTranscriptBytes / 2  // capped at ceiling
```

**Recommendation:** budget the diagnostic bundle the same way — a `DiagnosticBundleConfig`
struct (mirroring `HandoffSummaryConfig`) with a token ceiling, converted to a byte
ceiling via the same `* 4` approximation, and the same "prune each section, then drop
oldest-first until under budget" shape as `applySummaryBudget`. Don't add a tokenizer
dependency for this — the existing codebase has explicitly decided a byte heuristic is
sufficient for soft caps, and introducing exact tokenization here would be
inconsistent with that precedent for no compaction-quality benefit (the cap is a
safety rail, not a billing-accuracy requirement).

## 2. Cross-session summarization: `HandoffSummaryGenerator` (already shipped, PR #612)

`session/handoff_summary_service.go` (434 lines) is a complete, tested pipeline:
`NewHandoffSummaryGenerator(entClient, pool)` → `BeginGeneration` (sync dedup-guard +
interim `GENERATING` row) → `GenerateAndPersist` (async: load transcript, window via
`buildTranscriptWindow`/`applySummaryBudget`, call
`headless.GenerateHandoffSummary(ctx, pool, title, head, middle, tail)`, persist
`READY`/`ERROR`). Key facts the requirements.md explicitly depends on, verified against
this file:

- **Hard 60s timeout**: `handoffSummaryTimeout = 60 * time.Second` (line 40), enforced
  via `context.WithTimeout` around the `pool.CallBlocking` call (line 384). On timeout
  the row is written to `HandoffSummaryStatusError` via `failStage`, never left in
  `GENERATING` forever — confirms the requirement's "handoff-then-cleanup must handle
  the ERROR-on-timeout path; don't kill the old session if handoff failed" is checkable
  by reading `HandoffSummary.Status` after awaiting the goroutine (or polling
  `FindRowBySessionID`/`ReconcileStaleness`), not something this feature needs to
  reimplement.
- **Panic-safe, dedup-guarded, in-flight-tracked** (`sync.Map` keyed by session ID) —
  reuse `BeginGeneration`/`GenerateAndPersist` as-is for the "old session generates a
  handoff summary" step; do not build a second summarization path.
- Depends on `session/headless.PoolClient` — a one-method interface
  (`CallBlocking(ctx, FeatureKey, systemPrompt, userPrompt, CallOptions, CostSink) (string, error)`),
  already the seam this feature's own diagnostic-agent LLM calls (if any non-dispatch
  LLM calls are needed, e.g. summarizing logs before bundling) should go through.

**This mechanism is for the "cross-session handoff" case only** (requirements.md's own
Rabbit Hole warning) — it is not `/compact` and must not be conflated with it. `/compact`
is a harness-native command the *newly dispatched diagnostic agent* can use for its own
context growth; `HandoffSummaryGenerator` is what compresses the *stuck target session's*
history into the bundle. Both may be relevant to different parts of this feature but
solve different problems — keep them in separate code paths as the requirements insist.

## 3. Nudge mechanism: `AutonomousDriver` (`session/autonomous_driver.go`) — partially reusable, not directly

This file already has a mature "nudge a possibly-idle session" concept (commit
`bdfd9479b`, "suppress duplicate nudges on fixed idle-settle cadence #511") with real
prior art worth copying the *pattern* from, but it's built for a different loop shape
than this feature needs:

- `lastNudge` struct + `isDuplicateNudge`/`nextLastNudge` (pure functions, unit-tested
  in `nudge_dedup_test.go`) — dedup guard against re-sending the same nudge text inside
  a cooldown window (`nudgeCooldown`, 3 min production default per the code comment at
  line 369).
- `buildOrchestrationPrompt` — escapes prior nudge text via `lastNudgeTagEscaper`
  (`<`/`>` → entities) before round-tripping it into a new prompt, specifically to stop
  a prior LLM output containing a spoofed closing tag from corrupting prompt structure.
  This escaping discipline is directly relevant: this feature's diagnostic bundle also
  round-trips untrusted prior content (session output, log lines) into a new agent's
  prompt, so the same anti-injection escaping approach should be copied for any XML/tag-delimited
  bundle sections.
- **Why not reuse `AutonomousDriver` wholesale**: it drives a single session's own
  internal orchestration loop (turn-capped, `AutonomousMaxTurns`-gated, injecting nudges
  into *its own* managed session via `SendKeys`). This feature's nudge is a one-shot,
  externally-triggered write into *someone else's* (the stuck item's) session from a
  freshly dispatched diagnostic agent — a different call shape (`resume_session` /
  `steer_session` / `write_to_session`, see §5) invoked once, not a polling loop. Reuse the
  dedup/escaping *patterns*, not the `AutonomousDriver` type itself.

**Nudge-cap config precedent** — mirror `config.Config.AutonomousMaxTurns` /
`AutonomousMaxTurnsOrDefault()` (`config/config.go:302-945`): a `json:"...,omitempty"`
int field, a private `xxxDefault`/`xxxHardCeiling` const pair, and an
`OrDefault()` accessor that clamps `[1, hardCeiling]` and falls back to the default
when unset/non-positive. Add e.g. `DiagnoseNudge.MaxNudgesPerItem` /
`DiagnoseNudge.CooldownSeconds` following this exact shape rather than inventing a new
config idiom.

## 4. Feature flag for the nudge-execution gate

`config.Config.FeatureFlags map[string]bool` (`config/config.go:331`) plus the
`GetFeatureFlagWithDefault`/`SetFeatureFlag`/`GetFeatureFlagOverride` accessor family
(lines 451-615, see `TriageGuidanceHaltFeatureFlag`/`TymuxFeatureFlag` for the exact
const+accessor pattern) is the existing live-settable, global+per-scope-override
mechanism — this satisfies requirements.md's Risk Control section ("live-settable
feature flag... never an env var") and project memory's
`feedback_rollout_flags_live_settable_no_env_vars`. Add a new
`DiagnoseNudgeExecutionFeatureFlag` const following the `TriageGuidanceHaltFeatureFlag`
precedent (default OFF, per requirements), gating only the nudge-write path — diagnose/
bundle-assembly/bug-filing stay ungated.

## 5. Nudge execution surface: MCP tools are backed by plain Go methods

`resume_session` (`server/mcp/tools_lifecycle.go:77,399` — `lifecycleHandlers.resumeSession`),
`write_to_session` (`server/mcp/tools_terminal.go:83,277` — `terminalHandlers.writeToSession`),
and `steer_session` (`server/mcp/tools_terminal.go:139,664` — `terminalHandlers.steerSession`)
are MCP-protocol wrappers around ordinary Go handler methods that call into
`server/services` / `session.Instance`. Two implications for this feature's dispatched
diagnostic agent:

- The dispatched agent is a real Claude Code session (headless or interactive) with the
  stapler-squad MCP server attached, and it calls these tools exactly like any other MCP
  client (per requirements.md: "the agent calls resume_session/steer_session/
  write_to_session directly"). It is **not** expected to call the underlying Go methods
  in-process — the whole point is autonomous-agent-driven dispatch.
- This makes the feasibility risk in requirements.md concrete and worth taking
  seriously: this session's own transcript already observed the stapler-squad MCP
  server disconnect mid-conversation (`ECONNREFUSED`) — see the tool-call error banner
  encountered live in this research pass. A dispatched diagnostic agent whose nudge
  action depends on that same local MCP connection needs an explicit
  documented-and-logged failure path (e.g. "MCP unreachable → post a diagnostic note
  instead of silently doing nothing"), not an assume-success design, exactly as
  requirements.md's Feasibility Risks section says.

## 6. Nudge safety gate: `detection.StatusIdle`

`session/detection/detector.go`, `idle.go`, `pattern_set.go`, `osc_priority.go`, and
`proto_mapping.go` all actively produce/consume `StatusIdle` (matched via
`ps.idleRegexes`/`ps.patterns.Idle` in `pattern_set.go:178`, promoted via OSC sequences
in `idle.go:204`, and prioritized alongside other statuses in `osc_priority.go`) — this
is a live, actively-matched status, unlike `detection.StatusReady`, which project
memory (`instinct_detection_status_ready_dead_code.md`) already confirmed is dead code
never produced by `MatchLines`. **`StatusIdle` is a safe gate to build on**, but note
`idle.go:234`'s `case StatusIdle, StatusReady:` — some code paths still treat the two as
equivalent, so verify (in planning/implementation) that the specific idle-check this
feature adds reads the live `StatusIdle` signal and doesn't accidentally depend on a
branch that's only reachable via the dead `StatusReady` path.

## 7. Frontend: no new npm dependencies

`web-app/package.json` already has everything needed — React 19, `@connectrpc/connect`
+ `@connectrpc/connect-web` v2.1.1, Redux Toolkit, Radix UI primitives. The existing
`StuckItemDetail.tsx` (`web-app/src/components/backlog-stuck/StuckItemDetail.tsx`)
establishes the exact prop-callback pattern to follow for the new "Diagnose" action:

```tsx
/**
 * Approves the item's plan (ApprovePlan RPC) — omitted disables the
 * approve control entirely. Rejects (throws) on failure so this component
 * can surface the actual backend message...
 */
onApprovePlan?: (itemId: string) => Promise<void>;
```

Add `onDiagnose?: (itemId: string) => Promise<void>` (or a richer result type if the UI
needs to show dispatch status inline) to both `StuckItemDetail.tsx` and
`BacklogItemDetail.tsx`, with the parent components (`StuckItemsSection.tsx` and
whatever hosts `BacklogItemDetail`) owning the actual ConnectRPC client call — same
ownership split ApprovePlan already uses. Per `docs/reference/feature-testing-registry.md`
and `docs/reference/session-creation-registry.md`, this also needs a
`docs/registry/features/` entry and (if it's framed as a new session-creation mode,
since dispatch spins up a new agent session) a pass through the 7-touchpoint session-
creation registry checklist.

## Summary of concrete file touchpoints

| Need | Reuse | File |
|---|---|---|
| Bundle token/byte budgeting | Pattern only (bytes/4 heuristic) | `session/handoff_summary_excerpt.go` |
| Cross-session handoff summary | Direct reuse | `session/handoff_summary_service.go` |
| LLM call seam | Direct reuse | `session/headless.PoolClient`, `CallBlocking` |
| Nudge dedup/anti-spoofing pattern | Pattern only | `session/autonomous_driver.go` (`lastNudge`, `lastNudgeTagEscaper`) |
| Nudge cap config shape | Pattern only | `config/config.go` (`AutonomousMaxTurns*`) |
| Nudge-execution feature flag | Direct reuse of mechanism | `config/config.go` (`FeatureFlags`, `GetFeatureFlagWithDefault`) |
| Nudge execution (agent-side) | Direct reuse (MCP tools) | `server/mcp/tools_lifecycle.go`, `tools_terminal.go` |
| Idle-safety gate | Direct reuse, verify branch | `session/detection/{detector,idle,pattern_set}.go` |
| UI action wiring pattern | Direct reuse of pattern | `web-app/src/components/backlog-stuck/StuckItemDetail.tsx` (`onApprovePlan`) |
