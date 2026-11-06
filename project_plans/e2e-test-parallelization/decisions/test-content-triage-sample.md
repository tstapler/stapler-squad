# Bounded E2E Test-Content Triage (Story 4.1.1)

**Summary**: Of the 20 sampled files, 19 were classified "full e2e justified" (real backend
RPCs, real tmux/PTY/xterm rendering, real Axe/pixel-contrast scans, or real browser layout
measurements that a lighter tier cannot reproduce) and 1 — `tests/e2e/triage-question-answer.spec.ts`
— was classified "lighter-tier candidate" (every RPC it exercises is mocked or intercepted; the
backend contributes only a throwaway `CreateBacklogItem` call to mint a valid item ID). A second
file, `vcs-widget.spec.ts`, is a mixed case: its "Backlog item detail" sub-suite (2 of 3 tests) is
fully mocked-RPC-driven and plausibly convertible, but the file's overall verdict stays "full e2e
justified" because its third sub-suite genuinely needs real git-worktree scanning — see its row
below for the split. Recommendation: given this sample found only 1 clean candidate (plus one
partial case) out of 20 — and the methodology caveat below means this sample is structurally
biased toward files that are *less* likely to be lighter-tier candidates — a follow-up conversion
story is worth opening to track the finding, but the ROI case for actually converting
`triage-question-answer.spec.ts` is thin on its own; it's more useful as a seed for a differently
sampled follow-up pass (e.g. smaller files, or files outside the 54-file pool) that might surface
more candidates. A tracked backlog item is opened per the plan's delivery-mechanism requirement:
**backlog item `c52a2b3a-5393-4d1b-82d4-a274b4de3aad`** — "Review e2e test-content triage
candidates (e2e-test-parallelization Phase 4 follow-up)".

---

## Methodology (written before any sampled file's content was read)

Per the plan's Story 4.1.1 (`project_plans/e2e-test-parallelization/implementation/plan.md`,
Phase 4), this is a **stratified, bounded sample** of the 193(-ish)-spec e2e suite — not a
rubber-stamp of all of it, and not an exhaustive re-triage of every file.

### Step 1 — re-derive the 54-file "file-scoped `beforeAll`/`afterAll`" pool

`architecture.md` §5 named 54 files (28% of the then-193) in this category but did not record
its exact grep invocation, so this task re-derives it against the current tree rather than
trusting the cached number. The reconstruction:

1. Find every `tests/e2e/**/*.spec.ts` file containing a `beforeAll(` or `afterAll(` call
   (file-scope Playwright hooks, as opposed to `beforeEach`/`afterEach` which run per-test):
   ```
   grep -rlE "\b(test\.)?(beforeAll|afterAll)\s*\(" tests/e2e --include="*.spec.ts"
   ```
   This found **56** files against the current tree (current total spec count: 198, vs.
   architecture.md's 193 — a small drift, as the task anticipated).
2. Exclude any file that is *also* a member of one of architecture.md §5's other three risk
   categories (direct `TEST_SERVER_TESTDIR`/`TEST_SERVER_DIR` reads, the global seeded-session
   naming scheme, or an own-managed shared/network resource) — those files' `beforeAll`/`afterAll`,
   if present, sets up state that reaches *beyond* the one file, so they don't belong in the
   "safe because state is scoped to this file alone" category even though the hook syntax matches.
   Checking the 56 against those three other categories' current membership removed exactly 2
   files (`tests/e2e/remote-workspaces.spec.ts` — the confirmed real sshd external resource —
   and `tests/e2e/login-static-assets.spec.ts` — seeded-session-adjacent `seedSessions`/
   `liveSeedSessions` config).
3. Result: **54 files** — matching architecture.md §5's count exactly, despite the 5-file drift
   in the total suite size. (Coincidence, not proof the file *set* is identical — some churn in
   the 198-file tree could have added and removed files from this pool in equal numbers. Not
   independently verified file-by-file against the original list, since architecture.md didn't
   record the list itself, only the count.)

### Step 2 — sample the top 20 by line count, descending

Per the plan's explicit instruction, the 54-file pool is sorted by `wc -l` descending and the
top 20 are taken as the sample — a proxy for "most assertion surface to re-examine," using size
as the triage signal per this repo's own hotspot-analysis convention. This is a deliberate choice
of "most to gain/lose" files, not a uniform random sample across the pool.

Two files tied at 479 lines (`terminal-stress/tmux-roundtrip.spec.ts` and `backlog-quota.spec.ts`);
both fall inside the top 20 by rank regardless of tie-break order, so no tie-break rule was needed.

### Methodology caveat (stated explicitly, not left implicit)

This sample draws **exclusively** from the 54-file "file-scoped `beforeAll`/`afterAll`" pool, and
within that pool it is biased toward the **largest** files. That combination carries a real,
unresolved tension: a large file in this pool plausibly correlates with *heavier* backend/session
setup — more reason the file bothered to write a file-scoped `beforeAll`/`afterAll` at all — which
may **anti-correlate** with "doesn't need a real backend." That's the opposite of this sampling
rule's own "more to gain, more to lose" rationale, which assumed size correlates with conversion
upside. The sample intentionally includes **no file from outside the 54-file pool** as a
counterexample or sanity check — so if the true population of lighter-tier candidates is
concentrated in *small*, file-scope-hook-free specs, or in specs outside this pool entirely, this
sample is structurally unable to find them. A reviewer should read the candidate list below with
that selection bias in mind, not infer that "largest first" was the only consideration in scoping
this triage.

### Sample (top 20 of the 54-file pool, by line count descending)

| # | File | Lines |
|---|------|-------|
| 1 | `tests/e2e/accessibility.spec.ts` | 1355 |
| 2 | `tests/e2e/ux-error-exits.spec.ts` | 876 |
| 3 | `tests/e2e/backlog-send-back-feedback.spec.ts` | 816 |
| 4 | `tests/e2e/backlog.spec.ts` | 647 |
| 5 | `tests/e2e/backlog-engine-mode.spec.ts` | 520 |
| 6 | `tests/e2e/backlog-settings-a11y.spec.ts` | 495 |
| 7 | `tests/e2e/terminal-stress/tmux-roundtrip.spec.ts` | 479 |
| 8 | `tests/e2e/backlog-quota.spec.ts` | 479 |
| 9 | `tests/e2e/jules-dispatch.spec.ts` | 437 |
| 10 | `tests/e2e/notification-contrast-over-xterm.spec.ts` | 373 |
| 11 | `tests/e2e/backlog-session-steer.spec.ts` | 361 |
| 12 | `tests/e2e/unfinished-work.spec.ts` | 349 |
| 13 | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | 348 |
| 14 | `tests/e2e/vcs-widget.spec.ts` | 330 |
| 15 | `tests/e2e/repo-path-picker-parity.spec.ts` | 320 |
| 16 | `tests/e2e/backlog-stuck-items.spec.ts` | 315 |
| 17 | `tests/e2e/jules-status-badge.spec.ts` | 311 |
| 18 | `tests/e2e/triage-question-answer.spec.ts` | 300 |
| 19 | `tests/e2e/notification-inline-actions.spec.ts` | 282 |
| 20 | `tests/e2e/files-large.spec.ts` | 273 |

---

## Verdict table (Task 4.1.1b)

Each of the 20 sampled files was read in full before this table was written.

| # | File | Lines | Verdict | Justification |
|---|------|-------|---------|----------------|
| 1 | `tests/e2e/accessibility.spec.ts` | 1355 | full e2e justified | Real `@axe-core/playwright` scans against live-rendered DOM/CSS (color-contrast, focus-trap, tab order), real backend-seeded backlog items/notifications, real keyboard navigation and `getComputedStyle` reads — none of this exists without a real browser + real server. |
| 2 | `tests/e2e/ux-error-exits.spec.ts` | 876 | full e2e justified | Drives real session lifecycle transitions (pause/archive/delete via `SessionClient`) and real notification-tray DOM state; only the final RPC answer is stubbed per row, the session/approval/guidance state and the tray rendering are real. |
| 3 | `tests/e2e/backlog-send-back-feedback.spec.ts` | 816 | full e2e justified | Several tests call the real `TransitionBacklogItemStatus`/`RejectPlan` RPCs end to end (only `TriggerTriage` is ever mocked, to avoid a real LLM call) and assert on real ARIA attributes, focus order, and busy/disabled sync — a real backend + real browser round trip. |
| 4 | `tests/e2e/backlog.spec.ts` | 647 | full e2e justified | Real `CreateBacklogItem`/`ArchiveBacklogItem` CRUD, real flag-gated redirect behavior, real server-driven sort/group-by ordering, and real `localStorage`-backed tour persistence across a real page reload. |
| 5 | `tests/e2e/backlog-engine-mode.spec.ts` | 520 | full e2e justified | Real cross-viewport `getBoundingClientRect`/CSS-token layout checks (scroll margins, sticky-chrome overlap) and a real Features-page flag round trip against the backend (`setFlagViaApi`) that the banner polls — needs genuine browser layout + live flag state. |
| 6 | `tests/e2e/backlog-settings-a11y.spec.ts` | 495 | full e2e justified | Real Axe WCAG 2.2 AA scans across both themes/viewports plus real keyboard-only Tab-walks checking unobscured focus via `elementFromPoint` — fundamentally pixel/DOM-geometry assertions that require a real rendered page. |
| 7 | `tests/e2e/terminal-stress/tmux-roundtrip.spec.ts` | 479 | full e2e justified | The file's entire purpose is proving the real browser→WebSocket→tmux→PTY→browser round trip (keystrokes, `top` TUI rendering, 2000-line scrollback, viewport resize vs. real `tput cols`) — the canonical case a lighter tier cannot touch. |
| 8 | `tests/e2e/backlog-quota.spec.ts` | 479 | full e2e justified | One test runs end to end against the real `QuotaService` RPCs and asserts persistence after a real page reload; the rest drive real `beforeunload`/back-navigation browser guards and real cross-viewport layout — all require a live server + real browser. |
| 9 | `tests/e2e/jules-dispatch.spec.ts` | 437 | full e2e justified | Real `UpdateJulesConfig` writes to the test instance's OS keychain, real `WatchBacklogItems` live-update stream observation, and a real keyboard-driven focus trap through the dispatch dialog — the dispatch RPC itself is the only intercepted call (for good reason: a live Jules account would be required otherwise). |
| 10 | `tests/e2e/notification-contrast-over-xterm.spec.ts` | 373 | full e2e justified | Samples real rendered pixels from a real screenshot (ink made transparent, worst-pixel contrast scored against a real xterm canvas) across 6 themes and viewports — this is literally pixel-level verification of real rendering; no lighter tier can see canvas pixels at all. |
| 11 | `tests/e2e/backlog-session-steer.spec.ts` | 361 | full e2e justified | The steering tests create a real tmux-backed session and the real `UpdateSession` RPC genuinely reaches a live `Instance` and calls `SendKeys` — explicitly chosen over mocking so the widened-`UpdateSession` backend path is exercised for real. |
| 12 | `tests/e2e/unfinished-work.spec.ts` | 349 | full e2e justified | Seeds real bare git repos + `git worktree add` fixtures and drives the real `UnfinishedWorkService` scanner (`git worktree list` against disk) — the scanner's correctness is the thing under test, which only exists against a real filesystem/git state. |
| 13 | `tests/e2e/pipeline-mode-stage-executors.spec.ts` | 348 | full e2e justified | Calls the real `CreatePipelineMode`/`UpdatePipelineMode` RPCs and asserts on the *exact* server-side validation error strings from `session/pipeline_mode_validation.go` — the point of the test is that a wording change in that Go file fails the spec, which requires hitting the real backend. |
| 14 | `tests/e2e/vcs-widget.spec.ts` | 330 | full e2e justified (mixed — see note) | The "Unfinished item detail (compact mode)" sub-suite seeds a real git repo/worktree (ahead commit + uncommitted change) and reads it through the real scanner, so the file as a whole needs real backend+browser. However its "Backlog item detail (durable snapshot)" sub-suite (2 of its 3 tests) only ever intercepts `GetBacklogItemShipStatus` with a fully fabricated response against a trivial real-item fixture — that half is a plausible lighter-tier (RTL) candidate on its own if ever split into a separate file; not counted as a whole-file candidate here since the file can't be converted as a unit. |
| 15 | `tests/e2e/repo-path-picker-parity.spec.ts` | 320 | full e2e justified | Relies on real session-history ordering from the backend (`selectActiveSessionsSortedByUpdatedAt`) to populate the path-suggestion dropdown, plus real `overflow:hidden`/viewport-clipping geometry checks at 390px width — both need a real server and real rendered layout. |
| 16 | `tests/e2e/backlog-stuck-items.spec.ts` | 315 | full e2e justified | Seeds real `BacklogStuckState` rows via the e2e-only debug endpoint and drives real navigation/deep-linking, real keyboard operability, and a real (route-intercepted only for its pending-state timing) remediation retry — the surrounding stuck-item list/detail wiring is real. |
| 17 | `tests/e2e/jules-status-badge.spec.ts` | 311 | full e2e justified | Real Axe color-contrast scans per theme, real `routeWebSocket` interception of the live `WatchBacklogItems` socket to prove no optimistic-flash badge renders before real state arrives, and real backend-seeded sessions — needs a real browser accessibility tree and a real socket. |
| 18 | `tests/e2e/triage-question-answer.spec.ts` | 300 | **lighter-tier candidate** | Every RPC except a throwaway `CreateBacklogItem` (used solely to mint a valid item ID) is mocked/intercepted: `GetBacklogItem` is rewritten client-side to inject a fabricated completed-triage result, and `TriggerTriage` is always stubbed. All four tests assert pure frontend behavior — the composed `"Q:.../A:..."` feedback string, inline-error-with-preserved-draft on failure, focus return to the toggle on Cancel, and Submit staying disabled until text is typed. This is a strong candidate for a Jest/RTL test of `TriageDiffSection`/`TriageReviewPanel` with a mocked ConnectRPC client instead of a real backend+browser. Coverage to preserve on conversion: the exact feedback-string composition sent on submit, the `role="alert"` error path that keeps the draft text intact, focus-return-to-toggle on Cancel, and the submit-button disabled-until-non-empty gating. |
| 19 | `tests/e2e/notification-inline-actions.spec.ts` | 282 | full e2e justified | File header states explicitly: "Pending state is real, not mocked" — real `PermissionRequest` hook approvals, real `CreateGuidanceRequest` guidance, real backlog decisions, plus a real 20-approval p95-latency measurement over real round trips. |
| 20 | `tests/e2e/files-large.spec.ts` | 273 | full e2e justified | Generates real multi-hundred-MB file fixtures on disk, exercises the real `ReadFileRange`/`GetFileContent` RPCs and range-read feature flag, and measures real JS heap usage (`performance.memory`) in a real browser against a phone-class memory budget — none of this is reproducible without a real backend and a real browser. |
