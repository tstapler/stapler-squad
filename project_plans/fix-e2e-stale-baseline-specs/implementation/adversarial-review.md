# Adversarial Review: fix-e2e-stale-baseline-specs

## Challenge 1: Deleting terminal-flickering loses coverage

**Claim:** Deleting the spec eliminates 14 baseline-fail rows.

**Concern:** The spec was testing incremental-rendering correctness of `StateApplicator` — a
real correctness property, not just a perf floor.  Deleting it without a replacement means
no e2e test ever verifies that only changed lines update during rapid ANSI sequences.

**Response:** Accepted risk.  The `/test-terminal` page no longer exists in the production
router (it was removed before PR #1166), so the test can never run without first restoring the
page.  The incremental-rendering logic is better verified by Jest unit tests for
`StateApplicator`.  If a future Sprint wants to re-verify at the e2e level, the stress page
already supports frame-time metrics; a new spec can target `/test/terminal-stress`.

---

## Challenge 2: `test.skip(!!process.env.CI)` for stress specs is too broad

**Claim:** Skipping the entire stress spec under CI fixes the baseline.

**Concern:** This approach means the stress specs are only ever run locally by developers who
remember to run them.  Any regression in throughput goes undetected on CI.

**Response:** Valid long-term concern; accepted as a temporary measure.  A follow-on item
(already called out in the plan as "tracked separately") creates a dedicated benchmark GHA
workflow on a reserved runner.  The CI flag skip is not permanent infrastructure — it is a
ratchet that stops the correctness gate from being polluted by perf noise.  The spec files
remain in the repo; they are not deleted.

---

## Challenge 3: `beforeAll` for session seeding races with test parallelism

**Claim:** `beforeAll` in `terminal-mobile-overflow.spec.ts` and `android-url-bar.spec.ts`
seeds a session before tests run.

**Concern:** `beforeAll` creates a single session shared across all tests in the file.  If
tests run in parallel (sub-describe parallel), the session might be in an unexpected state by
the time later tests run.

**Response:** The existing e2e test infrastructure runs each spec file in a separate worker
(Playwright worker isolation, `E2E_WORKERS` for slot isolation).  Within a single spec file,
Playwright runs tests sequentially by default unless `test.describe.configure({ mode: 'parallel' })`
is set — these files don't set that.  `beforeAll` is the established pattern for session
seeding in this codebase (see `session-lifecycle.spec.ts`, `backlog.spec.ts`).  Risk is low.

---

## Challenge 4: `waitForSessionsActive` might add significant latency

**Claim:** Adding `waitForSessionsActive` before `waitForTestTrigger` / page navigation fixes
timeout-related failures.

**Concern:** `waitForSessionsActive` polls until the session is `ACTIVE`.  For a bash session
in test mode, this might take several seconds, adding latency to every test run.

**Response:** `waitForSessionsActive` polls a lightweight API endpoint and returns quickly once
the session is active.  Existing specs that use it (e.g., `session-lifecycle.spec.ts`) do not
show notably longer runtimes.  More importantly, the alternative is a 10 s timeout that always
fires when the session is slow — the fix reduces average runtime by ensuring the page loads only
after the session is ready.

---

## Challenge 5: terminal-resync-feature-flags failure cause may be different

**Claim:** Adding `waitForFlagRow` after `gotoFeaturesPage` fixes the 2 failing rows.

**Concern:** If the labels match and the page already waits for `h1`, the 2 failing rows may
be caused by something unrelated (e.g., the `resetFlag` POST itself returning 500 in test mode
because a feature-flag endpoint is not registered).

**Response:** The plan is a best-effort fix based on available evidence.  If `waitForFlagRow`
does not resolve the failures, the implementer should check the test-mode server's feature-flag
endpoint availability and whether `UpdateFeatureFlag` RPC is wired in the test-mode binary.  A
fallback is `test.skip(!flagEndpointAvailable, ...)` if the endpoint is not available in test
mode.

---

## Summary Verdict

No blocking objections.  Challenges 1 and 2 are accepted trade-offs with explicit rationale.
Challenges 3–5 are managed by established patterns.  The plan proceeds as written.
