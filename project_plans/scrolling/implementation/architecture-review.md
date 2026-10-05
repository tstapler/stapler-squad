# Architecture Review: scrolling
**Date**: 2026-10-01
**Verdict**: CONCERNS (initial: BLOCKED; see Re-review)

Constitution check: `docs/adr/ADR-000-architecture-constitution.md` does not exist; no constitution violations.
Kibitzer: on `PATH`, but `.claude/inspect.json` only configures a Go `primitive-obsession` check, so it adds nothing for the TS files touched. Review is from plan text plus direct reads of the code.

Path/line verification (all VERIFIED by reading the files): `useTerminalGestures.ts` rAF callback L240-247 and `Math.round`/`scrollLines` L245-246; 15 px slop L228; `XtermTerminal.tsx` sampler confirmed-fit L1051-1070, zero-size skip L1144-1146, imperative handle `fit` L1269-1271; `TerminalOutput.tsx` visibility fit L1160-1167 and `onVpResize` L1172-1188; toolbar PgUp/PgDn L1984/L2018; `XtermTerminal.css.ts` `touchAction: "none"` L60; `debug-terminal` flag at `TerminalStreamManager.ts:208`; `mouseTracking.ts`, `lib/terminal/__tests__/`, `useTerminalGestures.test.ts`, `XtermTerminal*.test.tsx`, `tests/e2e/terminal-resize.spec.ts` all exist. Not verified: `ADR-002` / touch `ADR-012` have no standalone file in `docs/adr/` (`012-*` there is react-virtuoso); they are only cited from code comments and `docs/tasks/*`.

## Blockers
- [ ] Story 2.1.2 / Task 2.1.2b-c (`refit()` + `postFitRepaint`) — The sampler (`XtermTerminal.tsx:1038-1093`) and `startSamplerIfNeeded` are local to the mount `useEffect` closure, while `useImperativeHandle` (L1250) sits outside it, so `refit()` has no way to reach the sampler as written. Worse, `postFitRepaint` is only called in the `result.schedule` branch (L1051); when proposed dims equal applied dims (the common keyboard open/close case where rows/cols are unchanged, or a missed repaint with a correct size) the sampler hits the at-rest branch (L1074-1078) and stops with no fit and no repaint. The primary "missed repaint" fix therefore never fires in exactly the scenario it targets. — Add a named task: store a `requestFitRef` (set inside the effect, invoked by `refit()`); have the sampler's at-rest and give-up branches call `postFitRepaint` when the request came from `refit()` (e.g. `startSamplerIfNeeded({forceRepaint:true})`); add an acceptance criterion "refit with unchanged dims still calls `refresh(0, rows-1)`".

## Concerns
- [ ] Story 2.1.3 / "single owner of fit" claim — Plan says exactly one pipeline calls `fit()`, but other bare calls remain: `XtermTerminal.tsx:502`, `:710`, `:1227`, `:1236`, and `TerminalOutput.tsx:1511` (`xtermRef.current.fit()`). Only L1163/L1181 are removed. — Enumerate each call site with a disposition (initial-mount fits may stay; document as exceptions) and soften the Goal wording, or route all through `refit()`.
- [ ] Story 1.2.2 / state machine — Touch-to-stop-without-focus needs a state the machine lacks (`GestureState` has no MOMENTUM). `onTouchStart` sets PENDING and `onTouchEnd` (L275-) would run the tap path and `focus()`. — Add an explicit `COASTING` state or a `momentumWasActive` flag consumed at touchend, in the Story 1.2.2 task list.
- [ ] Story 1.2.2 / Task 2.1.4b — Momentum cancel on viewport resize is wired via a callback from `createViewportSettle`, coupling the TerminalOutput settle helper to the gesture hook. — Have the hook subscribe to `visualViewport` resize itself (it already owns its cleanup), keeping the settle helper single-purpose.
- [ ] Story 1.2.1b — The SGR report needs col/row; plan doesn't say where they come from. — Specify using the touch-start cell (`startCol`/`startRow`, L74-75, already computed) clamped to `cols`/`rows`.
- [ ] Story 1.2.1c — Currently the first move past slop in PENDING returns without `preventDefault` (L249-250), so the first scroll event is cancelable-but-uncancelled; plan covers this but the test list has no assertion for it. — Add an AC "first move past slop calls `preventDefault` when `e.cancelable`".
- [ ] Story 2.1.2 / `refit()` timing — Settle (N stable rAF frames) feeds a sampler that already has a 150 ms RO debounce plus 50 ms ticks; `refit()` should bypass the 150 ms RO debounce or the blank window is ~200+ ms beyond settle, against the "within one frame after settle" metric. — State explicitly that `refit()` starts the sampler directly (skipping RO debounce).
- [ ] Tech Debt Disposition — `TerminalOutput.tsx` (2033 lines) is touched in three places (L1160-1188, `isFittingRef` L124, possibly L1511) but has no row in the table other than the fit pipeline; "Refactor-first" label for 2.1.3 is sequenced after 2.1.1, which reads as Isolate-via-seam. — Rename the disposition or add a row for the file.
- [ ] ADR references — ADR-002 / ADR-012 have no file under `docs/adr/` (verified). Plan header claims it "narrows ADR-002". — Cite the code comment or `docs/tasks/terminal-jank.md` location, or record the single-owner decision as a new ADR.

## Nitpicks
- `LineDelta` "branded number type" plus string-union `ScrollTarget` is good; also brand pixel deltas (`Px`) since px/line mix-up is the stated bug class.
- `readScrollMode` casts `terminal.modes as any` like existing `mouseTracking.ts`; prefer typing it once there.
- `TUI_SCROLL_POLICY` as a module constant means the spike result is a code edit; fine, but consider passing policy as a hook option for test injection (tests already need it).
- Story 1.1.1 AC second bullet ("22 px minus 0") is confusingly worded.

## Re-review (after plan.md revision)

Scope: only the previously BLOCKED item (Story 2.1.2). Verified against `web-app/src/components/sessions/XtermTerminal.tsx`.

**Blocker: RESOLVED.**
- Reachability: `sampleTick`/`startSamplerIfNeeded` are still closure-local (L1038-1101, inside the mount effect that closes at L1193) and `useImperativeHandle` (L1250) is outside it. The plan's fix (a `requestFitRef` assigned inside the effect, invoked by `refit()`) is the correct mechanism and the plan states this root cause accurately.
- Missed repaint at unchanged dims: the at-rest branch (L1074-1078) and give-up branch (L1083-1090) both `stopSampler()` with no fit. The plan now adds `repaintRequested`, consumed in schedule, at-rest and give-up branches, and a `refit()` while the sampler is already active sets the flag instead of being dropped (`startSamplerIfNeeded` is a no-op when `samplerActive`, L1096, so this case is genuinely needed and covered).
- Timing: `startSamplerIfNeeded` calls `sampleTick()` synchronously (L1100), so the at-rest repaint lands on the first tick, matching the plan's "within ~1 frame" claim. Bypassing the 150 ms RO debounce (L1126) is explicit.
- ACs now cover unchanged dims, already-active sampler, non-`refit()` RO runs (no regression), debounce bypass, bounded zero-size retry, restore after collapse, and context loss.

Other previously raised concerns (fit-call-site dispositions, `refit()` timing, tech-debt row) are addressed in the plan text (Story 2.1.3 table, Tech Debt table).

**Remaining minor concerns (non-blocking):**
- Clear `requestFitRef.current = null` in the mount effect cleanup so a late `refit()` after unmount cannot start a sampler on a disposed terminal (`sampleTick` guards on null refs, but the intent should be explicit in Task 2.1.2b).
- Concerns on Stories 1.2.1b/1.2.1c/1.2.2 (COASTING state, SGR cell source, first-move `preventDefault` AC, settle/gesture coupling) and the missing ADR-002/ADR-012 files were outside this re-review scope and were not re-checked.

**Final verdict: CONCERNS** (no blockers).
