# Research Findings: fix-e2e-stale-baseline-specs

Refreshed 2026-11-25 at merge commit `1a7226027` by **running** the eight specs
(`chromium` + `chromium-dom`, `--retries=0`, one worker, load average ~23-33) and probing the DOM.
This replaces the earlier by-reading findings, most of which the runs disproved.

## Observed failures (before any change): 51 failed

| Spec | Failed rows | Observed error | Verified cause |
|---|---|---|---|
| `terminal-flickering` | 14 | `terminal-container` never visible | `/test-terminal` route no longer exists |
| `input-drop-badge` | 10 | `waitForFunction` timeout | `TerminalOutput.tsx` gates the trigger on `NODE_ENV === "production"`; `build.ts` always defines production, so the built chunk reads `if (... \|\| true) return;` |
| `terminal-mobile-overflow` | 8 | `toolbar-more-button` not found | `/` shows an empty pane, not session cards; `isVisible()` does not wait, so the helper silently does nothing. The toolbar starts collapsed. "Show keys" is now "Show mobile keyboard"; Paste and uploads moved into the overflow row on mobile |
| `terminal-resync-feature-flags` | 6 | "Enable …" button not visible | Toggle is now `role="switch"` with `aria-checked`; a failed save shows `feature-flag-row-error` with Retry, not a "refresh" alert |
| `android-url-bar` | 2 | 2 fixed elements below viewport | Closed bottom drawers (`notification-tray`, "More navigation") parked with `translateY(100%)`. BottomNav tests all pass |
| `terminal-resize` (dom) | 1 | "Resize terminal" button not found | Renamed to "Redraw terminal (fixes a blank screen)" |
| `terminal-stress/ascii-video` | 6 | frames 50/215, 150/180, 10/40 below floors | Real throughput floors, CPU dependent |
| `terminal-stress/large-payload` | 4 | `> 102400` got 102400; `> 0` got 0 | **Not perf floors.** The page flushes `bytesGenerated` only every 5th frame and never on completion; `framesRendered` updates only every 10th frame, stop, or completion, and the preset runs at 1 fps. `avgFrameTime < 500` cannot hold at 1 fps (frame time is the inter-frame interval, observed 1015 ms) |

## Baseline rows

`next-e2e-baseline.json` holds 55 `fail` rows for these files; the item says 47. The extra rows
are 4 resync (also failing today, same cause) and 4 stress rows
(`should not show visible flickering` ×2, `should not corrupt terminal state` ×2) that pass locally.

The baseline was recorded from the Next build, which has been removed, so it cannot be re-recorded.
Precedent (#1459, `c6f8c1464`): flip the `class` of rows verified by a run, and drop rows of deleted tests.

## Notes

- PR shards skip baseline-`fail` tests except in spec files the PR changes; the comparator runs on main only.
- `waitForSessionsActive` matches **titles**, not IDs.
- `terminal-resize` tests 1-4 skip for the same session-card reason. They are `skipped` rows, outside this item.
- The closed "More navigation" sheet is not `visibility: hidden`, so it stays in the accessibility tree. Out of scope; report it.

## 2026-11-26: the baseline can be re-recorded (corrects "cannot be re-recorded" above)

Done inline (no research agents): the question is narrow and the sources are in the tree.

- Only the *Next* build is gone. The gate scores the Bun build against the file, so a capture from
  Bun-build CI runs is the current ground truth for the repaired specs.
- The original capture rules are in `project_plans/web-app-bun-build/spikes/s0/classify-e2e.mjs`:
  three reports; `pass` = attempt-0 pass in every run, `fail` = final failure in every run,
  `skipped` = skipped in every run, else `flaky`.
- `.github/workflows/build.yml` has `workflow_dispatch`. A dispatch runs the full suite (no
  baseline-fail skipping, which is PR-only) and uploads `e2e-bun-report` (7-day retention).
- Concurrency group is `build-<ref>` with cancel-in-progress off `main`, so the three dispatches
  on one ref must run one after another.
- `routesVisited` is not in the CI report. The rows keep their previous value, which
  `scripts/route-coverage-manifest.ts` may read.
