# Validation: fix-e2e-stale-baseline-specs (plan revision 2)

The tests under change are the tests, so each AC is verified by running them. All runs use both
Chromium projects with `--retries=0`.

| AC | Verification | Pass condition |
|---|---|---|
| 1 | `git ls-files tests/e2e/terminal-flickering.spec.ts`; `grep -rn "test-terminal" web-app/src tests/e2e/*.ts` | both empty |
| 2 | stress specs with `CI=1`, then without | CI: 3 ascii tests per project skipped, none failed. Off CI: large-payload passes; ascii floors may fail on a loaded host |
| 3 | `terminal-mobile-overflow.spec.ts` | 12/12 pass |
| 4 | `input-drop-badge.spec.ts` | 10/10 pass |
| 5 | `android-url-bar.spec.ts` | 10/10 pass |
| 6 | `terminal-resize.spec.ts` | dom test 5 passes; no failures |
| 7 | `terminal-resync-feature-flags.spec.ts` | 6/6 pass |
| 8 | `make ci`; `bun run lint:duplicates`; baseline unit tests; PR shard report for changed specs | exit 0; no previously-passing row fails |
| 9 | recompute counts from `next-e2e-baseline.json` rows | `meta` matches rows; every flipped row was observed pass/skip |

## Regression guards (does the test fail without the fix?)

| Change | Evidence it fails without the fix |
|---|---|
| Hook gate | before-run: 10 timeouts against the built bundle |
| Stress progress flush | before-run: `> 102400` received 102400 |
| Stress Stop loop | run the new reset test against the unfixed loop; it must fail on the duration bound |
| Android drawer filter | filter names two drawers only, so any other overflowing fixed element still fails |

## Pre-mortem

| Risk | Priority | Mitigation |
|---|---|---|
| A row flipped to `pass` fails on a slower CI runner and reds main (comparator is main-only) | P2 | Condition-based waits only; check the PR shard report for the changed specs before merge |
| `skipped` baseline rows treated as a gap by the comparator | P2 | Run the comparator and its unit tests against the edited baseline |
| Hook enabled in production by accident | P3 | Requires a script to set `window.__E2E_HOOKS__`; it only shows a badge |

No P1 items.

## Readiness gate: PASS

Every AC has a verification; no TODOs or ADR references in plan.md; the one adversarial BLOCKER is
resolved (re-review verdict CONCERNS, its guard suggestion adopted); no schema change; no new package.

Triad check (done inline, not by separate agents): product — restores PR signal on eight specs;
UX — no user-visible change; engineering — product edits limited to a test hook and a test page.

## Verification results (2026-11-26, local, load average 19-53)

- Before: the eight specs had 51 failures (`--retries=0`, chromium + chromium-dom).
- After, `CI=1`, seven remaining specs: 0 failed; ascii frame floors (6) and 9 resize tests skipped.
- Input-drop, resync, resize, mobile overflow: 87 passes over `--repeat-each=3`.
- New reset test fails against the unfixed stress loop (second run completed after 2993 ms).
- `make ci` exit 0 (`GOTMPDIR` in `/var/tmp`; `/tmp` tmpfs filled during an earlier link).
  Rerun on `bad0c94a9` (after the last merge and review-note commits): `make ci` and
  `lint:duplicates` exit 0.
- **Limit:** with the server resize call removed, the redraw e2e still passed in some runs because
  another in-flight snapshot restored the rows. The forced resize is asserted by
  `TerminalOutput.refit.test.tsx` (13 `TerminalOutput` suites, 209 tests pass).
- Rerun on `44d638b9a`: `make ci` and `lint:duplicates` exit 0; changed specs (`CI=1`,
  `--retries=0`, both Chromium projects) 57 passed, 15 skipped; other `terminal-stress` specs and
  `terminal-hub-path` 26 passed, 12 skipped; `bun test tests/e2e/scripts tests/e2e/baseline` 103 pass.
