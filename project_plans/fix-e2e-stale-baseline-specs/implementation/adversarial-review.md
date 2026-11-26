**Verdict: BLOCKED**

# Adversarial review: plan revision 2

Method: read the code only; no builds, tests or browsers. Runtime consequences are inferred.

## Blocker

### B1. Stop does not stop the stress page's loop, so change 3's reset test can pass on the old run

- `web-app/src/app/test/terminal-stress/TerminalStressTest.tsx:296-299`: `stopTest` aborts, then sets `abortControllerRef.current = null`.
- Line 254 checks `abortControllerRef.current?.signal.aborted`, which is then `undefined`, so the loop keeps rendering. Once a second run starts, the old loop reads the new run's signal. Nothing calls `cancelAnimationFrame`.
- In `tests/e2e/terminal-stress/large-payload.spec.ts:116`, run A (5 s) is stopped at about 2 s, then run B starts. About 3 s later A reaches line 261, sets `completed` and publishes metrics from the shared `frameTimesRef`.
- "Wait for completion, then `framesRendered > 0`" is therefore satisfied even if B renders nothing. "Publish final counters" would also publish A's bytes over B's.

Recommendation: capture the controller's `signal` inside `startTest` and exit the loop on it. On abort, only return: `setStatus('completed')` there would overwrite reset's `idle`. Publish final counters from a ref in `stopTest` and on natural completion. Add a check that no frames arrive after Stop.

## Concerns

### C1. The resize test can pass without observing a redraw
`tests/e2e/terminal-resize.spec.ts:125-129`: `not.toBeAttached` succeeds immediately if the overlay has not appeared, and empty `rowsAfter` yields no duplicates. Renaming the button flips the row to `pass` with this hole.
Recommendation: assert `nonBlankBefore` is non-empty, and poll for the redrawn content before counting duplicates.

### C2. Local runs are the only evidence for flipped rows, and PR CI will not gate them
The comparator is main-only (`.github/workflows/build.yml:687-752`). `should not show visible flickering` and `should not corrupt terminal state` failed in the recorded baseline, get no fix, and pass only locally. Flipped to `pass`, a CI failure reddens main.
Recommendation: before merge, read the PR's `e2e-bun-report` for every flipped (spec, project, title).

### C3. 100KB test loses freeze detection on CI
`large-payload.spec.ts:33-34`: with both ceilings off in CI, a run with very slow frames still passes if it completes in time with some frames and enough bytes.
Recommendation: keep a loose `maxFrameTime` ceiling on CI (for example 5000 ms, as the 1MB test uses).

### C4. Departures from acceptance criteria are not recorded
- AC-1 requires a commit message saying what replaces the flickering spec; the plan omits it. Name only coverage that exists.
- AC-9 requires the capture script; the plan hand-edits (justified, state it in the PR).
- AC-5 expects a BottomNav label fix; the cause differs.
- AC-2 and AC-7 row counts (10, 2) differ from the baseline (14, 6).

## Minor

- m1. `android-url-bar.spec.ts:68-81`: "translated down by its own height" would also hide a wrongly translated visible-state element. Also require it to start at or below the viewport bottom, or limit the exclusion to the two known drawers.
- m2. `TerminalOutput.tsx:855-861`: the comment saying the hook is never attached in production becomes false; update it. No Jest case for the new gate is planned.
- m3. Mobile overflow: the title says "primary buttons are visible without opening overflow"; also assert Paste and uploads are hidden before opening More. Use a unique seeded title, since `beforeAll` reruns after a worker restart.
- m4. Resync failure test: also assert the Retry button. `tests/e2e/baseline/build-coupled-specs.json:34-37` (and the baseline's `buildCoupledSpecs`) still describes the `refresh` alert filter.
- m5. `next-e2e-baseline.json` `meta.counts` is already wrong (says fail 319, pass 2142, 2693; rows give 297, 2162, 2691). Recompute from rows.
- m6. `validation.md` still describes revision 1 (all stress tests skipped, android seeding, 47 rows).

## Checked, no problem found

- Deleting and reclassifying rows does not break `scripts/e2e-baseline-compare.ts` (`skipped` has no evaluator) or `tests/e2e/scripts/baseline-skip.ts` and its tests.
- `tests/e2e/baseline/select-specs.test.ts:7` names the deleted spec only in a synthetic list. Other references are `shard-timings.json:200`, `e2e-video.yml:29` (both in the plan), the triage JSON and historical docs.

## Re-review (B1 only)

**Verdict: CONCERNS** (B1 is no longer blocking; the other findings were not re-reviewed.)

- The page fix is sufficient: a per-run `signal`, with a silent return on abort, ends the old loop (`TerminalStressTest.tsx:254-258`, `296-299`).
- The proposed guard is not. "Counter still zero right after Reset" also passes with the bug: at 1 fps the old loop does not publish until its 10th frame or completion (lines 204, 261-263). The old run's completion can then still satisfy the restart assertion.
- Recommendation: keep the first Stop mid-run, then assert that the restarted run reaches `completed` no sooner than about 4.5 s after the second Start (preset duration 5 s). The old run would finish at about 3 s. A lower bound does not flake under load.
- Wording: change 3 still says counters are published when a run "stops". The abort path now publishes nothing, so either publish from `stopTest` or drop "or stops".
