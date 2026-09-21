# Adversarial Review: session-lifecycle-state-machine
**Date**: 2026-09-06
**Verdict**: CONCERNS
**Note**: Repair-loop iteration 1 — scoped re-review of the two prior blockers (Risk Control gate, abandoned-goroutine gauge). See git history for the original full review if needed.

## Blockers

Both prior blockers are resolved. No new blocker was introduced.

**Blocker 1 (Risk Control) — RESOLVED.** Story 3.1.2 (plan.md:871-923) adds
`config.TmuxLifecycleV2Enabled()`, an env-var flag (`STAPLER_SQUAD_TMUX_LIFECYCLE_V2`,
default `false`) that gates all of Epic 3.1's consolidated-classification code and Epic
3.2's observability wiring:
- Default-false path is specified as a byte-for-byte copy of today's shipped code ("the
  `else` branch is a verbatim copy of what ships today — no `lifecycle` import, no
  `classifyControlModeExit` call," Task 3.1.2b) — a real safe default, not a doc note.
- Both flag states are tested: Task 3.1.2c parametrizes Epic 3.3's 3 new tests across
  `""`/`"true"` asserting identical `onExit` behavior, and Task 3.4.1b re-runs the full
  `session/tmux` suite with the flag on, both attached to the PR.
- A removal plan exists: Task 5.3.2a files a tracked follow-on (GitHub issue or
  `decisions/` entry) recording the flag's purpose, a bake-time criterion for flipping the
  default and deleting the `false` branch, and a link back to Risk Control — explicitly
  framed as required, not optional cleanup ("not removing it is itself a form of the exact
  'ad hoc flag litter' this project exists to close out").
- This satisfies requirements.md's mandate (`requirements.md:303-306`) for "a build tag,
  gradual rollout, or extra bake time in review" distinct from tymux's opt-in safety net —
  it's a real functioning flag (not a comment), defaults to the old path, is tested both
  ways, and has an explicit sunset plan. Not a fig leaf.

**Blocker 2 (Abandoned-goroutine blind spot) — RESOLVED.** Story 1.3.3 (plan.md:399-452)
adds `session_lifecycle_active_generations{subsystem}`, an `Int64UpDownCounter` incremented
in `StartGeneration` and decremented in `EndGeneration` only (Task 1.3.3b) — `RecordEnd`
deliberately never touches it, so call sites with no matching `StartGeneration` (e.g.
`ReconnectLoop`'s give-up branches) can't drive it negative.
- Diagnosability: an abandoned generation now shows up as "active count not decreasing"
  (exactly the Success Metrics framing) rather than nothing — the isolated unit test
  (Task 1.3.3c) proves the mechanism, and Story 2.2.2/Task 2.2.2a extends the *existing*
  `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged` test with a
  `metric.ManualReader` assertion that the gauge stays `>= 1` after the real wedge scenario
  runs — this is a genuine end-to-end regression test against the actual mechanism that
  reproduces incident #3's shape, not just a synthetic gauge-only test.
- Leak risk on normal (non-abandoned) exits was checked for both migrated call sites:
  - `session/tymux`: Task 2.2.1a's wrapper is a plain sequential `StartGeneration` →
    `s.readAttachLoop(...)` → `EndGeneration` with no branching — every return value of
    `readAttachLoop` reaches the same unconditional `EndGeneration` call, so there is no
    normal-exit path that skips it.
  - `session/tmux/control_mode.go`: `EndGeneration` is called from inside the single
    `t.onExitOnce.Do(...)` closure shared by all 3 exit call sites (scanner-EOF fallback,
    `%exit`, `%session-closed` — Tasks 3.1.1b/c/d), and the scanner-EOF fallback is the
    pre-existing catch-all that already fires whenever the scan loop ends for any reason
    (that invariant predates this migration; the plan does not change it), so a normal
    (non-wedged) exit is always guaranteed to hit `onExitOnce.Do` exactly once. `sync.Once`
    also rules out the double-decrement direction. Only a goroutine that never reaches any of
    the 3 sites (the wedged/abandoned shape) leaves the gauge elevated — which is the intended
    signal, called out explicitly in Story 3.2.1's acceptance criteria ("this is Story
    1.3.3's intended signal, not a gap to patch here").

## Concerns
Not re-checked this iteration — carried forward verbatim from the previous review, see
prior version in git history:
- [ ] Epic 2.2, Task 2.2.1a — `genCtx`/cancellation-propagation-through-`trace.WithNewRoot()` assumption is asserted but only indirectly verified via a `-race` run, not a direct unit test.
- [ ] Phase 4 (`session/external_streamer.go`) — bundling `readLoop`'s `IsBenignTimeout` swap into the "3 subsystems migrated" count/Success Metrics framing when it doesn't fit requirements.md's Scope criterion for a `Reason`-mechanism migration.
- [ ] Epic 1.2, Task 1.2.1a — the shared `session/[^d]` → `session/[^dl]` regex edit isn't re-verified against the pre-existing `session/detection` exhaustive-linter carve-out.
- [ ] Epic 1.1 (`Reason.ShouldContinue()`/`ShouldFireExitCallback()`) — hardcodes one global answer per `Reason` for all current and future consumers; no discussion of what happens if a future third subsystem needs different semantics for the same `Reason` value.
- [ ] Epic 2.2 + Epic 3.2 (span attributes) — no session/pane identifier attached to either `StartGeneration` call site, and `control_mode.go`'s span is started from `context.Background()`, making its `trace.WithLinks` a no-op; undercuts "diagnosable from a trace" for exactly the backend Risk Control most cares about.
- [ ] Epic 3.1, Task 3.1.1c — accepted metric double-counting risk across `control_mode.go`'s racing call sites vs. `RecordEnd`, if a future dashboard panel is added without preserving the double-counting caveat. (Note: as re-read this iteration, Tasks 3.1.1b/c/d's current design routes classification/`EndGeneration`/callback *inside* the shared `onExitOnce.Do` closure specifically to eliminate this double-count — worth confirming on the next full review pass whether this concern is now stale.)

## Minors
Not re-checked this iteration — carried forward verbatim from the previous review, see
prior version in git history:
- `classifyStreamEnd`'s read of `s.exited` (session-scoped, not generation-scoped) inherits pre-existing cross-generation staleness exposure.
- Placing `IsBenignTimeout` inside `session/lifecycle` will read confusingly re: what "lifecycle" means in that package.
- Task 1.2.1b's carve-out verification is explicitly not a permanent automated test; relies on human review noticing future `.golangci.yml` diffs.

Additional minor noted this iteration:
- Task 3.1.2a's instruction to "mirror `STAPLER_SQUAD_USE_CONTROL_MODE`'s existing
  read-and-parse shape exactly" is imprecise: that existing flag defaults to *enabled*
  on unset/`"true"` (`server/services/connectrpc_websocket.go:938-939`), the opposite
  polarity from `STAPLER_SQUAD_TMUX_LIFECYCLE_V2`'s required default-`false`. The plan's
  own acceptance criteria (Story 3.1.2) are unambiguous about the correct default, so this
  doesn't change the design, but "mirror... exactly" should read "mirror the parsing idiom
  (env var + truthy-string check), not the default polarity" to avoid an implementer
  copy-pasting the wrong default.
