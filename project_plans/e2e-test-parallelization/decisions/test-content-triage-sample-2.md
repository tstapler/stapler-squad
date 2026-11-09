# Differently-Sampled E2E Test-Content Triage (Follow-up to Story 4.1.1)

**Summary**: Of 20 sampled files, **2 are clear lighter-tier candidates** (no browser or server needed at all),
**3 are probable candidates** that need an overlap check against existing Jest coverage before converting,
**1 is mixed** (one of two tests is convertible), and **14 are full e2e justified**. The first sample
(`test-content-triage-sample.md`) found 1 of 20, so this sample, drawn from the opposite end of the
suite, finds a higher rate (2–5 of 20 vs 1 of 20), which supports the earlier suspicion that candidates
concentrate in small, hook-free files. Counts are from the per-file table below.

## Methodology

- **Pool**: every `*.spec.ts` file under `tests/e2e/` that has no file-scope `beforeAll`/`afterAll`, i.e. outside
  the 54-file pool of the first sample. Current counts (2026-11-04, after merging `origin/main`):
  `find tests/e2e -type f -name "*.spec.ts" | wc -l` = 203 files, of which
  `grep -rlE "\b(test\.)?(beforeAll|afterAll)\s*\(" tests/e2e --include="*.spec.ts" | wc -l` = 61 have file-scope
  hooks, leaving 142 outside. (The 54-file pool was 60 minus files in the other risk categories; not re-derived
  here, so a file with hooks could in principle belong to neither.)
- **Sample**: the 20 **smallest** files in that pool by `wc -l` (18–51 lines), ascending. Five zero-line
  entries under `tests/snapshots/**/visual-regression.spec.ts` are directories, not specs, and were skipped.
  Every sampled file was read in full before its verdict was written.
- **Bias, stated explicitly**: the first sample took the *largest* files of a hook-heavy pool, which plausibly
  anti-correlates with "no real backend needed". This one takes the *smallest* hook-free files, which plausibly
  *over*-correlates with it: small files are often smoke checks or pure frontend flows. Neither sample is
  random, so neither supports an estimate of the suite-wide rate; the two together bracket it. A middle band
  (files of 52–300 lines) was not sampled.
- **Rubric** (same as the first sample): "lighter-tier candidate" means every behavior asserted can be
  reproduced by Jest/RTL or a node script with mocked RPCs; "full e2e justified" means it needs a real
  server, real browser layout/pixels/accessibility tree, real tmux/PTY, or real persistence across reload.

## Sample and verdicts

| # | File | Lines | Verdict | Justification |
|---|------|-------|---------|---------------|
| 1 | `tests/e2e/website-demos.spec.ts` | 18 | full e2e justified | Not a test: records `.webm` clips via a real browser, skipped unless `WEBSITE_DEMO_OUT` is set. Needs a real browser and server by definition. |
| 2 | `tests/e2e/perf/findingsNotes.spec.ts` | 22 | **lighter-tier candidate** | Uses only `fs` and `path`: checks that markdown notes under `project_plans/frontend-responsiveness/research/` have required headings. No page, no server. A Jest/node test (or a repo script) does the same job. |
| 3 | `tests/e2e/session-artifacts.spec.ts` | 28 | full e2e justified (weak) | Clicks the first real session, opens the Artifacts tab, matches any of several states. Skips itself when no session exists, so it may assert nothing in a fresh environment; flag for a value review rather than conversion. |
| 4 | `tests/e2e/import-external-session.spec.ts` | 29 | full e2e justified | Clicks the real nav "Import" link and asserts the routed page and the real "no external sessions discovered" empty state from the backend discovery RPC. |
| 5 | `tests/e2e/files-search.spec.ts` | 32 | full e2e justified | Search-to-line flow on the real Files tab over a seeded fixture, with `toBeInViewport` and line-focus checks that need real layout. |
| 6 | `tests/e2e/history-search.spec.ts` | 35 | **probable candidate** | Types `payment` into the search box, asserts the input value and the page title, clears it. Near-vacuous; RTL on the search input covers it, or delete as low value. |
| 7 | `tests/e2e/omnibar-backlog-intent-confirm.spec.ts` | 40 | full e2e justified | Presses Enter on `backlog: ...` and waits up to 15 s for the real intent-parse path to open the item form; exercises the real RPC (or its fallback), not a mock. |
| 8 | `tests/e2e/tools/e2e-conventions.spec.ts` | 41 | **lighter-tier candidate** | Pure static scan of the e2e source tree for one-shot `page.title()` reads. Its own header says it "runs without a browser". Belongs in a node test or lint rule. |
| 9 | `tests/e2e/a11y-responsiveness-states.spec.ts` | 45 | full e2e justified | Axe WCAG sweep over three retained windows with a real feature flag set via the API. |
| 10 | `tests/e2e/workspace-management.spec.ts` | 45 | full e2e justified (weak) | Smoke checks against pre-seeded sessions (card count > 0, title regex); the second test passes whether or not the link exists. Value is the seeded-session wiring; flag for review. |
| 11 | `tests/e2e/scroll-forward-outcome-visible.spec.ts` | 46 | full e2e justified | Needs a real alt-screen tmux program and a real scroll gesture through the stream hub. |
| 12 | `tests/e2e/onboarding-hook-install.spec.ts` | 47 | **probable candidate** | The onboarding modal steps and the two toggles are frontend-only; `GetHookStatus` is read-only and could be mocked. `web-app/src/components/onboarding/__tests__/OnboardingModal.hooks.test.tsx` exists; overlap with it is **not yet checked**, so confirm before deleting. |
| 13 | `tests/e2e/alias.spec.ts` | 49 | **probable candidate** | Typing `@` in the omnibar shows the alias palette or its empty state; test 2 accepts either outcome. Frontend-only given a mocked alias list. No Jest file for the alias palette was found (`find web-app/src -iname "*AliasPalette*test*"` returned nothing); coverage elsewhere is not checked. |
| 14 | `tests/e2e/pi-session.spec.ts` | 49 | full e2e justified | Creates a real session through the omnibar and asserts the created session's `program` field comes back from the backend. |
| 15 | `tests/e2e/visual-regression.spec.ts` | 50 | full e2e justified | Pixel baselines (`toHaveScreenshot`) across four theme projects; only a real browser can produce them. |
| 16 | `tests/e2e/session-pin.spec.ts` | 51 | full e2e justified (mixed) | Test 1 needs a real `PinSession` plus a real reload to prove persistence. Test 2 (RPC returns 500, optimistic pin rolls back) is mocked-RPC only and RTL-convertible on its own. |
| 17 | `tests/e2e/smoke.spec.ts` | 51 | full e2e justified | A smoke test is valuable precisely because it boots the real built app and server and loads real routes. |
| 18 | `tests/e2e/viewport-meta.spec.ts` | 52 | full e2e justified | Asserts the viewport meta and `<html lang>` of the real served HTML on five routes and a computed `env(safe-area-inset-bottom)` padding in a real browser. |
| 19 | `tests/e2e/scroll-forward-unsupported-path-honest-copy.spec.ts` | 53 | full e2e justified | Needs a real tmux session pinned to the legacy streaming path via a backend RPC; asserts the real blocked-toast copy. |
| 20 | `tests/e2e/session-program-change.spec.ts` | 53 | full e2e justified | Drives the real `UpdateSession` RPC and asserts the seeded session's list card updates. |

## Counts (derived from the table above)

- lighter-tier candidate (clear): **2** (#2, #8)
- probable candidate, overlap check needed: **3** (#6, #12, #13)
- mixed, partly convertible: **1** (#16)
- full e2e justified: **14** (the remaining rows, including two flagged weak: #3, #10)

## Recommended follow-ups (not done here)

1. Move #2 and #8 out of Playwright. They need no browser, so they cost a Playwright worker slot for nothing.
   (#8 is a convention guard; moving it must keep it running in `make ci`.)
2. For #12 and #13, diff the spec's assertions against existing Jest coverage first; delete the spec only if
   nothing unique remains.
3. Review #3, #6, #10 for value, since all three can pass without proving anything.
4. Sample the unsampled middle band (52–300 lines, hook-free) if the owner wants a suite-wide estimate.

## Decision record (backlog item c52a2b3a)

Tyler chose **Option D** (B + C) on 2026-11-04:

- Convert `tests/e2e/triage-question-answer.spec.ts` to `BacklogItemDetail.triageQuestionAnswer.test.tsx`.
- Drop the durable-snapshot half of `tests/e2e/vcs-widget.spec.ts`; keep its real-worktree compact-mode test.
- Run this differently-sampled triage pass.

Rationale: component-level Jest already covered the scenarios, so the missing piece was a `BacklogItemDetail`
integration test for the prop-threading chain; the first sample's own caveat said its candidate count may undersell.

Accepted coverage gap: the Jest test mocks `useBacklogService` and sets `item.triageResult` directly, so it no longer covers the proto itemSessions→triageResult mapping, the real `TriggerTriage` request body, or the 400 `failed_precondition` error mapping that the deleted e2e spec exercised. Those are accepted as lower-risk than the e2e cost; add a focused adapter test if any of them regresses.
