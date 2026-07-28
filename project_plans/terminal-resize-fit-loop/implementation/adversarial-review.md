# Adversarial Review: terminal-resize-fit-loop

**Date**: 2026-07-27
**Verdict**: CONCERNS

## Blockers
(none)

## Concerns

**All 3 resolved during Phase 4 repair passes** (pre-mortem P1 #1 and triad engineering-lens
repair) — checked off below; see `plan.md` Epic 2.4, Task 2.3.1b, and ADR-018 Consequences.

- [x] **RESOLVED (Epic 2.4, added per pre-mortem.md P1 #1)** — **Second `fit()` entry point (`XtermTerminal.tsx:615`, the imperative `fit()` exposed via
  `useImperativeHandle`, called from `TerminalOutput.tsx`'s `isVisible` handler (~801) and its
  `visualViewport.resize` handler (~819)) is never gated by `shouldFit`.** The plan's own Pattern
  Decisions table correctly reasons that hooking the oscillation detector at `terminal.onResize`
  (rather than the ResizeObserver callback) means the AC4 burst counter still sees oscillations
  from this path *when the proposed dims actually change value* — satisfying both explicit
  recommendations in `research/features.md` (font-size effect and visualViewport should feed the
  same counter). But it does not address the sub-case where this path calls `fit()` repeatedly
  *without* the proposed value ever changing (e.g. viewport-height jitter that never crosses a
  cell boundary): `terminal.resize()` with an unchanged value doesn't fire `onResize`, so those
  calls never reach the burst counter, yet each one still does real layout-measurement work. The
  only remaining protection is the pre-existing `isFittingRef` 300-400ms reentrancy guard — and
  that guard's own comment ("iOS where fit() triggers another resize event") is direct evidence
  this exact bug class has already manifested via this path. Not a violation of the 7 literal ACs
  (which describe the ResizeObserver-driven chain specifically), but it's an unaddressed,
  undocumented residual risk in a path the codebase itself flags as history-prone to this bug
  class. **Recommendation**: either explicitly scope this out in ADR-018 with rationale, or apply
  the same `shouldFit` gate inside the line-615 imperative handler (cheap, ~3 lines, same pattern
  as Task 2.2.1a) for full defense-in-depth.

- [x] **RESOLVED (Task 2.3.1b's `terminalRef.current` guard)** — **WebGL double-dispose/teardown-race guard from `research/pitfalls.md` §4
  (xterm.js#5181) is not implemented.** Pitfalls recommended checking `terminalRef.current` is
  still non-null / not-already-disposed before calling `webglAddon.dispose()` from the new
  oscillation-triggered fallback path. Task 2.3.1b's dispose branch (`XtermTerminal.tsx` inside
  `resizeDisposable`) only checks `webglAddonRef.current` truthiness, not terminal-disposed state.
  Likely low-probability in practice given JS's single-threaded execution and the existing
  cleanup ordering (the `onResize` subscription is itself disposed during unmount, so the handler
  shouldn't be reachable post-teardown) — but the plan doesn't add the guard pitfalls.md
  specifically called out, nor does it explain why it's provably unreachable. **Recommendation**:
  add a `terminalRef.current` non-null check before `webglAddonRef.current.dispose()` in Task
  2.3.1b, or add a one-sentence note in ADR-018 explaining why the race is structurally impossible
  given the disposal ordering.

- [x] **RESOLVED (ADR-018 Consequences, Negative section)** — **Residual false-positive risk from `research/pitfalls.md` §3 (legitimate drag jitter near a
  cell boundary producing an A/B/A/B/A `onResize` sequence indistinguishable from the real WebGL
  oscillation bug) is not documented anywhere the plan says the ADR will live.** Pitfalls.md
  explicitly recommends the ADR "state this explicitly... since it's easy to get [the design]
  backwards." The plan's ADR content outline (Task 5.1.1a: Context / terminology correction /
  Decision / Consequences) doesn't list this discriminator or the residual risk as a stated
  tradeoff. Low severity in practice — a false trip just disables WebGL for that tab's session
  (benign degradation, not a correctness break) — but it's a known, named risk from research that
  the plan silently drops rather than accepts-with-reasoning. **Recommendation**: add one
  Consequences bullet to ADR-018 acknowledging the discriminator (count only post-AC2/AC3-dedup
  `onResize` applications, not raw events) and that a sufficiently pathological drag can still
  false-trip the fallback as an accepted, low-cost tradeoff.

## Minors

- The pre-existing `console.log("[XtermTerminal] WebGL2 unavailable (Android?), using canvas
  renderer")` (current line 279, the "WebGL2 unavailable" branch) is not corrected to "default
  renderer" alongside the `onContextLoss` message the plan does fix (Task 2.1.1b). Leaves
  inconsistent "canvas renderer" wording in the codebase after a fix whose whole point is
  correcting that terminology.
- Task 4.1.1c's third assertion (a forced call immediately followed by a non-forced call with the
  same value, asserted to be deduped) doesn't advance fake timers between the two calls, so the
  observed skip could be caused by the pre-existing 200ms time-throttle rather than the AC3
  value-dedup the test claims to prove ("the forced call correctly re-set `lastSentSizeRef`") —
  the two mechanisms aren't isolated in this specific assertion.
- If oscillation continues after a successful WebGL→DOM-renderer fallback (not guaranteed to be
  fully resolved, per the requirements' own out-of-scope note that the underlying pixel/glyph math
  isn't being root-caused), `shouldAbandonWebgl` will keep tripping on every subsequent burst and
  the `console.error` "no WebGL addon to dispose" backstop will log repeatedly with no
  backoff/one-shot latch. Functionally harmless (doesn't peg CPU by itself) but could spam the
  console indefinitely in a still-oscillating session post-fallback.
- AC1 has zero automated regression coverage — appropriately so, since `research/features.md`
  confirms real multi-`XtermTerminal`-instances-in-one-page doesn't exist in this architecture
  (one terminal per browser tab), so Epic 5.2's manual-only checklist is the correct scope. Noting
  only that a future architecture change (e.g. real in-page tiling) would silently lose this
  guarantee with no automated tripwire to catch it.

## What the plan gets right (for context, not part of the verdict)

- AC2's comparison baseline correctly uses live `terminal.cols`/`terminal.rows` (not a
  fit()-only ref), matching pitfalls.md §1's explicit warning about `StateApplicator` mutating
  `terminal.cols/rows` independent of `fit()` — a fit()-only ref would have silently swallowed
  legitimate resizes (AC6 violation). Correctly implemented in Task 2.2.1a.
- Oscillation detector is correctly hooked at `terminal.onResize` (the common funnel across all
  5+ `fit()` call sites), not the ResizeObserver callback — matches pitfalls.md §2's explicit
  finding and satisfies both of `research/features.md`'s explicit recommendations (font-size
  effect and visualViewport path both feed the same burst counter when they produce a changing
  value).
- Oscillation history is correctly pruned on every push (Task 2.3.1b) and correctly scoped as an
  effect-local variable rather than a component-level `useRef`, matching pitfalls.md §6's
  StrictMode/scrollback-remount leak warning.
- `shouldFit` correctly returns `false` (not `true`) when `proposeDimensions()` returns
  `undefined` for either axis — matches AC2's explicit requirement and avoids a "silently spin
  forever" or crash failure mode.
- `force` bypass parameter on `resize()` correctly identified and threaded through both of the 2
  real non-standard callers (`TerminalOutput.tsx:664` reconnect-resync, `:1160` manual
  force-resize) — verified against actual file contents, not just the plan's claims.
- AC4's "canvas" → "default DOM renderer" reinterpretation is well-grounded: verified
  `@xterm/xterm ^6.0.0` is genuinely pinned and `@xterm/addon-canvas` is genuinely absent from
  `package.json` — this is a correction, not an excuse to under-deliver; the functional intent
  (eliminate WebGL as the loop's amplifier via `dispose()`) is still fully met.
- No new npm dependency is introduced anywhere in the plan; RxJS and `@xterm/addon-canvas` are
  both explicitly considered and rejected with sound reasoning.
- No scope drift into `shortcutRegistry.ts`, in-page tiling, or re-tuning the existing 150ms
  debounce / double-rAF / 200ms throttle values.
- AC6 and AC7 both have concrete, findable story+task+test entries (Stories 4.1.1b, 4.2.3, 5.2.1),
  not hand-waved.
- Deferring ADR-018's actual file-write to Phase 5 (after the pure functions and gates are
  implemented and tested) is reasonable, not scope drift — it's tracked as a concrete task, and
  its content is effectively already drafted via the plan's Domain Glossary and Pattern Decisions
  sections.
