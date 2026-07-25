# ADR-022: Use Headless Pool for Triage Instead of AutonomousDriver

**Date**: 2026-06-22
**Status**: Accepted
**Deciders**: Tyler Stapler

---

## Context

`TriggerTriage` was designed to spawn a tmux session and hand it to `AutonomousDriver`
for LLM-driven orchestration. In practice, this flow never works:

1. **`Prompt` vs `InitialPrompt` mismatch**: `CreateDirectorySession` sets `inst.Prompt`
   but `session_driver.go` injects `inst.InitialPrompt`. For autonomous sessions
   `InitialPrompt` is empty, so Claude sees the generic fallback "Please proceed." instead
   of the triage prompt. Claude never receives the item ID, artifact paths, or the JSON
   output schema.

2. **Silent `headlessPool` nil gate**: `StartAutonomousDriverWithTimeout` returns early
   (log warn only) when `s.headlessPool == nil`, which happens whenever the `claude` binary
   is absent at startup. The tmux session is spawned and sits idle forever.

3. **5-minute per-turn idle timeout fires during normal execution**: Triage instructs
   Claude to run 4 parallel subagents taking 8–15 minutes. The driver's 5-minute per-turn
   cap fires mid-execution, injecting a spurious NEXT_MESSAGE turn that disrupts the
   subagent run and causes early DONE signals.

4. **Status transition gap**: `onAutonomousDriverComplete` transitions `idea → ready`
   only on `outcome.Done == true`. `submit_triage_result` has no equivalent
   `TriageCompletionSignaler` to signal the driver, so the LLM orchestrator must infer
   completion from the terminal tail — and often gets it wrong.

The headless review gate (`spawnReviewGate`) avoids all of these problems: it calls
`pool.CallBlocking` directly, has a single failure mode (LLM error or timeout), and
transitions status synchronously inside the goroutine. Triage has the same shape.

---

## Decision

Replace the `AutonomousDriver` + tmux session path in `TriggerTriage` with a direct
headless pool call, mirroring the `spawnReviewGate` pattern:

- `TriggerTriage` creates a synthetic `ItemSession` (UUID: `"headless-triage-<uuid>"`)
  synchronously and returns.
- A bounded goroutine (`triageSem`, cap 8) calls
  `pool.CallBlockingWithOptions(ctx, FeatureKeyTriage, headlessTriageSystemPrompt, prompt, CallOptions{WorkDir: item.RepoPath})`.
- The prompt instructs Claude to write artifact files and output a JSON result object.
- `ParseHeadlessTriageResult` parses the JSON and extracts summary, suggestions, tasks.
- The goroutine persists the result and calls `TransitionBacklogItemStatus(idea → ready)`.
- On failure the goroutine records `ended_at` on the `ItemSession` and publishes a failure
  notification. The item stays at `idea`; the operator can retry.

`submit_triage_result` MCP tool is NOT called by the headless path (it requires
`STAPLER_SESSION_UUID` which headless calls do not inject). JSON output replaces it.

---

## Alternatives Rejected

### Fix AutonomousDriver In-Place
Set `InitialPrompt = triagePrompt`, extend startup timeout to 15 minutes, add
`TriageCompletionSignaler`. This patches symptom 1 but leaves symptoms 2, 3, and 4
in place. Raising the per-turn timeout globally risks regressions in work sessions.
Rejected: too many moving parts, incomplete fix.

### Hybrid: Headless Research + Tmux Synthesis
Run 4 parallel headless calls for research, then spawn a tmux session for synthesis.
Rejected: introduces a handoff race, significant complexity, still requires a working
AutonomousDriver for the second phase. The operator visibility benefit does not justify
the complexity at this stage.

---

## Consequences

**Positive**:
- Eliminates all four identified failure modes simultaneously.
- Reuses a pattern already proven in production.
- Single failure mode (LLM timeout/error) with deterministic error handling.
- No change to `AutonomousDriver` code; work sessions and review sessions are unaffected.
- Concurrency is bounded by the `triageSem` (cap 8, matching `maxConcurrentReviewGates`).

**Negative**:
- No tmux pane to inspect mid-triage. Operators cannot watch research happen in real time.
  (Mitigated by: artifact files are written to disk during the call; operator can check
  `docs/tasks/<slug>/research/*.md` after completion.)
- `submit_triage_result` is not called in headless mode, so the MCP tool's `STAPLER_SESSION_UUID`
  authorization path is bypassed. Headless triage writes results directly; this is consistent
  with the headless review gate design.

**Neutral**:
- The `headlessTriageSystemPrompt` constant must instruct Claude to write artifact files
  and output JSON — two responsibilities in one prompt. This is unavoidable without a
  two-stage approach.
- Future enhancement: add a streaming progress UI using SSE or EventBus artifacts-written events.
