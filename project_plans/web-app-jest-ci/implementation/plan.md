# Implementation Plan: Wire Jest (web-app) into CI

## Critical Finding — Read First

**The two known-broken test suites this plan was scoped to fix are already fixed on `origin/main`.**

This repo's local `main` branch (current checkout, `HEAD` at `cb82049a5`) is **87 commits behind** `origin/main` (`tstapler/stapler-squad`, fetched during planning on 2026-08-07). Commit `bf5b19f201bda366b9a290406cf5e32f2a60cbcd` ("fix(web-app): un-quarantine BacklogEmptyState and SessionDetail.embedded suites (#354)", merged 2026-08-05, `origin/main` is an ancestor — verified via `git merge-base --is-ancestor bf5b19f20 origin/main`) fixes both suites identified in `research/pitfalls.md`:

- **`BacklogEmptyState.test.tsx` unhandled-rejection crash**: fixed by wrapping the button's `onClick` in `Promise.resolve().then(onCreateItem).catch(...)` in `web-app/src/components/backlog/BacklogEmptyState.tsx`. The component itself was *also* independently redesigned between the research pass and now — a separate, unrelated merge (`5b1fc21c5`, "chore: sync upstream → personal fork (20260709)") removed the inline create-form (title input, priority select) in favor of a page-level form in `web-app/src/app/backlog/page.tsx`, making `onCreateItem` a plain `() => void` button click. `BacklogEmptyState.test.tsx` was rewritten (7 stale form-assertion tests removed, replaced with 4 tests matching the button-only API) in the same PR #354.
- **`SessionDetail.embedded.test.tsx` stale DOM assertions**: root cause was **not** the `ef83d1e9a` tab-strip refactor research originally suspected — it was a *later*, unrelated fork-sync commit (`5b1fc21c5`, same one above) that made `SessionDetail.tsx` lazy-load `SessionDetailView` via `next/dynamic()` (a legitimate ~2MB bundle-size optimization). The test's pre-existing blanket `jest.mock("next/dynamic", ...)` — originally written to stub only the inner `TerminalOutput` dynamic import — started intercepting *this* new outer call too, replacing the entire `SessionDetailView` (and its real `role="tablist"`/`role="tabpanel"`/`aria-labelledby="tab-terminal"` markup, confirmed present and correct at `SessionDetailView.tsx:558,635,736,757,791,796,807,817,822,1240`) with an inert stub div. Fix (PR #354): differentiate the mock by `loader.toString().includes("SessionDetailView")`, resolving that one loader synchronously via `require("../SessionDetailView").SessionDetailView` while still stubbing the `TerminalOutput` loader. `SessionDetailView.tsx` itself needed no change — its ARIA wiring was already correct.

**Implication for this plan**: Tasks 1–2 below (originally scoped as "root-cause-fix these two suites") become **sync + verify**, not **implement**. Re-doing the fix would either be a no-op (if synced correctly) or, worse, produce a *second*, conflicting fix if attempted against the stale local `BacklogEmptyState.tsx`/`.test.tsx` (which still has the old inline-form component researched in `research/pitfalls.md` — that file no longer reflects reality). **The implementer must sync onto `origin/main` before doing anything else.** This does not shrink the plan to "just add a YAML step" — the sync + verification work is still real, evidence-bearing work per AC #6, it's just not new source-code authorship.

`origin/main` does **not** yet have a Jest CI step anywhere (`git grep -l jest origin/main -- .github/workflows/` → no matches) — the actual CI-wiring task (this project's core goal) is still fully unstarted upstream.

---

## Step 0.5 — Creative Pass (Alternatives Considered)

**A. Append a `Jest` step to `lint.yml`'s existing `golangci` job.**
Strength: reuses pnpm 10.27.0 / Node 22 / `pnpm install --frozen-lockfile` already running in that job — zero new toolchain setup, zero new path-filter surface to keep in sync.
Weakness: couples an unrelated concern (JS test execution) to a job named/badged around Go linting; a Jest hang would block the whole `lint` check, including unrelated Go-only checks.

**B. New job within `lint.yml` (`jest:` alongside `golangci:`), running in parallel.**
Strength: failure isolation (Jest failure doesn't block/get-blocked-by golangci-lint) and true parallel wall-clock time.
Weakness: duplicates the entire pnpm/Node/checkout setup block (lines 37–64) a second time, and doubles the path-filter list to keep in sync across two jobs in the same file — the classic drift risk `build-vs-buy.md` already flagged for a *new file*; a second job has the identical problem.

**C. New standalone `jest.yml` workflow file.**
Strength: fully independent triggers, badge, and failure surface — cleanest separation of concerns long-term.
Weakness: `build-vs-buy.md`'s stated verdict — no genuine parallel-matrix need (unlike `build.yml`'s prepare→matrix cross-compile split) to justify a second full workflow; a new file means a *third* copy of the pnpm/Node pin (`lint.yml` already has one) to keep in sync, for a complexity-1 task.

**Chosen: A** — append the step to the existing `golangci` job. The weakness (coupling to an unrelated job name, no failure isolation) is real but minor at this scale (single `npx jest` invocation, ~20s local wall-clock per `research/stack.md` §2) and matches the requirements doc's own proposed approach verbatim. B and C both trade a real, current cost (setup/path-filter duplication) for a parallelism benefit this task doesn't need yet. Rejected alternatives recorded below in Pattern Decisions.

---

## Domain Glossary

This is CI YAML plumbing plus inherited (already-merged) bugfixes — genuinely little domain vocabulary. Honest list:

| Term | Meaning |
|---|---|
| **Jest project** | One of the 4 sub-configs in `web-app/jest.config.js`'s `projects` array (`web-app`, `eslint-plugin-analytics`, `dev-stack`, `e2e-dev-mode`) — Jest's built-in multi-config aggregation, not a repo-specific concept. |
| **`golangci` job** | The existing (only) job in `.github/workflows/lint.yml`, named after its primary step; this plan appends a step to it rather than adding a job. |
| **Path filter** | The `on.push.paths` / `on.pull_request.paths` glob list in `lint.yml` that scopes which file changes trigger the workflow — a GitHub Actions primitive, not repo-specific. |
| **"Un-quarantine"** | Informal language from commit `bf5b19f20`'s message for "fix a suite that was known-broken but not yet gated in CI" — there is no actual `testPathIgnorePatterns` or skip-list in `jest.config.js`; nothing was mechanically quarantined, it was just untested by CI (confirmed via `grep -rn "quarantine\|testPathIgnorePatterns" web-app/jest.config.js` → no matches). |

No new types, services, or abstractions are introduced by this work.

---

## Pattern Decisions

Infra/workflow-structure decisions, not GoF/PoEAA patterns — forcing an OOP pattern label onto YAML edits would be wrong, so this table uses "workflow pattern" language throughout.

| Decision | Chosen Pattern | Alternative Rejected | Reason |
|---|---|---|---|
| Where does the Jest step live? | Append step to existing `golangci` job in `lint.yml` | New parallel job in `lint.yml` | Duplicates the pnpm/Node/checkout setup block a second time in the same file; doubles path-filter surface to keep in sync for a single ~20s test run — not worth it at this scale (see Creative Pass B). |
| Where does the Jest step live? | Append step to existing `golangci` job in `lint.yml` | New standalone `jest.yml` workflow | `build-vs-buy.md`'s verdict: no genuine parallel-matrix need (unlike `build.yml`'s cross-compile matrix); a third copy of the pnpm/Node pin to maintain for a complexity-1 task (see Creative Pass C). |
| How to reach "green" for AC #6? | Sync the branch onto `origin/main` (already has fix `bf5b19f20`), then verify | Re-implement the fix locally against the stale pre-sync `BacklogEmptyState.tsx`/test | Re-implementing would produce a second, conflicting fix and ignore that the real current `BacklogEmptyState.tsx` (post `5b1fc21c5`) has a materially different component shape (no inline form) than what `research/pitfalls.md` describes — that research predates the fix-and-redesign history. |
| Should CI "fix" (skip/exclude) the 2 known suites instead? | No — sync in the real fix, don't skip | `--passWithNoTests` / suite exclusion / `continue-on-error` on the Jest step | `requirements.md` explicitly forbids this ("do not add `--passWithNoTests` or exclude failing suites to force green"); also moot now that both suites are already genuinely fixed upstream. |
| Test reporting / annotations | Raw `npx jest` CLI log output in the Actions log | `dorny/test-reporter` / `mikepenz/action-junit-report` / `jest-junit` | No such tooling exists anywhere in the repo today (`build-vs-buy.md`, verified via grep); adding one is a new dependency + `checks:write` permission for a "just make it run" task — legitimate future follow-up, not in scope. |
| Step placement within the job | Immediately after "Install web-app dependencies" (before proto/ent generation) | After golangci-lint / at the end of the job | Jest needs only `pnpm install`, not `buf generate` or ent codegen (those exist for the Go build/`import cycle check` step) — placing Jest first lets a test regression fail fast without waiting ~1-2 min for unrelated Go codegen steps to run first. |

---

## Migration Plan

Omitted — no schema, data, or persistence changes. This is CI configuration plus already-merged (upstream) UI bugfixes.

---

## Observability Plan

CI-native only — no new telemetry:
- **Pass/fail signal**: GitHub Actions check-run status on the `lint` job (existing check name), visible on the PR "Checks" tab and as a commit status.
- **Failure diagnosis**: raw `npx jest` stdout/stderr in the step's Actions log — per-suite `FAIL <path>` lines and per-assertion diffs, no separate reporter/dashboard.
- **No coverage tracking** — explicitly out of scope per `requirements.md`.
- **No flake-retry telemetry** — none exists in the repo to extend (`research/pitfalls.md` confirms no retry-on-flake convention).

---

## Risk Control

| Risk | Mitigation | Residual |
|---|---|---|
| `ubuntu-latest` runner is 4-core; Jest's default worker autodetect can over-subscribe in containers | Pass `--maxWorkers=4` explicitly (per `research/stack.md` §2) | Low — explicit flag removes the autodetect gotcha entirely. |
| Cold `node_modules`/ts-jest transform cache every run (`cache: 'pnpm'` only caches the content-addressable store, not `node_modules`) | None added now — `isolatedModules: true` in tsconfig keeps ts-jest in cheap per-file transpile mode | Accepted per `research/pitfalls.md`: "don't preemptively add a ts-jest cache step, watch real CI timing first." Re-open if CI runs prove slow. |
| No branch protection / required status checks on `main` on either remote (verified via `gh api`, `research/pitfalls.md`) | None — the check will run and report but can't yet technically block a merge | Accepted, explicitly out of scope for this task; flagged in Unresolved Questions for future-proofing. |
| Trigger paths scoped to `web-app/**` per requirements don't cover `scripts/dev-stack/**` or `tests/e2e/**` source, which 2 of the 4 Jest projects (`dev-stack`, `e2e-dev-mode`) actually test | None — accepted gap, requirements explicitly scope to `web-app/**` minimum | Documented in Unresolved Questions; a `dev-stack`/`e2e-dev-mode`-only change would silently not trigger the Jest gate. Existing `lint.yml` paths already include `tests/e2e/**` for other reasons (ESLint/build), so e2e-dev-mode is likely covered incidentally — `scripts/dev-stack/**` is not covered by anything today. |
| Sync onto `origin/main` (Task 1) surfaces *other*, unrelated changes in the 87-commit gap that could themselves affect `web-app` test outcomes | Task 2's verification step re-runs the full suite post-sync and treats any *new* failure as a blocker to root-cause before proceeding — not assumed away | If a new failure appears, it is out of this plan's scope to fix speculatively; escalate/re-scope rather than force green. |

---

## Unresolved Questions

1. **Path-filter scope gap** (accepted, not solved): `scripts/dev-stack/**` and `tests/e2e/**` source changes can trigger 2 of the 4 Jest projects but aren't in the `web-app/**` trigger-path minimum requirements.md specifies. Do not silently widen scope beyond what's asked — flagged for a future task if it causes a missed-regression incident.
2. **No branch protection today** on either `tstapler/stapler-squad` or `TylerStaplerAtFanatics/stapler-squad` `main` (verified via `gh api` during research) — this Jest step will run and report a check but cannot yet technically block a merge. Not blocking this task; worth a follow-up once this and other checks have proven stable.
3. **Sync mechanics**: this plan assumes the implementer syncs the working branch onto `origin/main` (e.g. `git rebase origin/main` or branching fresh off `origin/main`) before Task 3. The exact mechanics (rebase vs. fresh branch vs. cherry-pick of just `bf5b19f20`) are an implementation-time judgment call depending on what else is in flight on the local branch — not prescribed here.

---

## Dependency Visualization

```
┌─────────────────────────────────────────────────────────────┐
│ Task 1: Sync working branch onto origin/main                │
│  (inherits commit bf5b19f20 / PR #354)                      │
└───────────────────────────┬───────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│ Task 2: Verify BacklogEmptyState + SessionDetail.embedded   │
│  suites pass post-sync (investigate-first if not)            │
└───────────────────────────┬───────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│ Task 3: Add "Jest" step to lint.yml's golangci job           │
└───────────────────────────┬───────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│ Task 4: Widen lint.yml trigger paths                         │
│  (web-app/src/**, package.json, pnpm-lock.yaml,               │
│   jest.config.js)                                             │
└───────────────────────────┬───────────────────────────────┘
                             │
              ┌──────────────┴──────────────┐
              ▼                             ▼
┌───────────────────────────┐   ┌───────────────────────────────┐
│ Task 5: Local/Docker green │   │ Task 6: Live CI red/green demo │
│ run (AC #6 evidence)        │   │ (AC #4 evidence)               │
└───────────────────────────┘   └───────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│ Task 7: AC #7 docs check — verified no-op, no file edit       │
└─────────────────────────────────────────────────────────────┘
```

---

## Phase / Epic / Story / Task Breakdown

### Epic 1: Wire the web-app Jest suite into GitHub Actions CI

#### Story 1.1 — Inherit the already-merged test-suite fixes

**Task 1 — Sync working branch onto `origin/main`**
- Files: none (repo-level git operation)
- Action: Rebase or re-branch the implementation branch onto `origin/main` so it includes commit `bf5b19f201bda366b9a290406cf5e32f2a60cbcd` ("fix(web-app): un-quarantine BacklogEmptyState and SessionDetail.embedded suites (#354)"). Confirm with `git merge-base --is-ancestor bf5b19f20 HEAD && echo yes`.
- Est: 5 min (mechanical; may be longer if local branch has unrelated conflicting work — not expected here since `git status` shows only `project_plans/**` changes, no `web-app/**` changes).

**Task 2 — Verify the inherited fixes actually pass**
- Files: `web-app/src/components/backlog/BacklogEmptyState.tsx`, `web-app/src/components/backlog/BacklogEmptyState.test.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`, `web-app/src/components/sessions/__tests__/SessionDetail.embedded.test.tsx`
- Action: Run `cd web-app && npx jest --testPathPatterns="BacklogEmptyState|SessionDetail.embedded" --no-coverage`. Expect `Test Suites: 2 passed, 2 total`. **If either still fails post-sync** (e.g. sync mechanics went wrong), do NOT re-derive a fix from scratch — the exact correct diff is captured verbatim in commit `bf5b19f20` (`git show bf5b19f20`); re-apply it (`git cherry-pick bf5b19f20` or manual re-apply) rather than reinventing it against the stale component shape described in `research/pitfalls.md`.
- Est: 3 min.

#### Story 1.2 — Add the CI gate

**Task 3 — Add the Jest step to `lint.yml`**
- Files: `.github/workflows/lint.yml`
- Action: Insert a new step immediately after "Install web-app dependencies" (after line 64, before "Generate protobuf code" at line 66):
  ```yaml
      - name: Jest
        working-directory: web-app
        run: npx jest --ci --maxWorkers=4
  ```
- Est: 3 min.

**Task 4 — Widen trigger paths in `lint.yml`**
- Files: `.github/workflows/lint.yml`
- Action: Add three new glob entries to *both* the `on.push.paths` list (lines 6–16) and the `on.pull_request.paths` list (lines 20–30): `web-app/src/**`, `web-app/package.json`, `web-app/pnpm-lock.yaml`, `web-app/jest.config.js`. (`web-app/**.ts`/`web-app/**.tsx`/`web-app/**.css` already exist and remain; the new entries close the gap for non-`.ts`/`.tsx`/`.css` files that affect the Jest suite — lockfile bumps, `package.json` script changes, and `jest.config.js` itself.)
- Est: 4 min.

#### Story 1.3 — Verify the gate works (both directions)

**Task 5 — Local/Docker full-suite green run (AC #6 evidence)**
- Files: none (verification only; run against the synced tree from Task 1–2 plus the `lint.yml` edits from Task 3–4, which don't affect test execution)
- Action: `docker run --rm -v $(pwd):/repo -w /repo/web-app node:22-bookworm bash -c "corepack enable && pnpm install --frozen-lockfile && npx jest --ci --maxWorkers=4"` — matches `research/pitfalls.md`'s reproduction methodology (Node 22, matching CI's pin) but now expected clean post-sync. Capture the summary line (`Test Suites: 4 passed, 4 total`) as evidence.
- Est: 5 min (mostly `docker run` wait time).

**Task 6 — Live CI red/green demonstration (AC #4 evidence)**
- Files: `web-app/src/components/backlog/BacklogEmptyState.test.tsx` (scratch edit, reverted after)
- Action: On a throwaway branch/PR, add `expect(true).toBe(false);` inside an existing `it(...)` block in `BacklogEmptyState.test.tsx`, push, and confirm via `gh pr checks` (or the Actions UI) that the `lint` check goes red with a `FAIL src/components/backlog/BacklogEmptyState.test.tsx` line in the Jest step's log. Record the Actions run URL as evidence, then revert the scratch edit (`git checkout -- web-app/src/components/backlog/BacklogEmptyState.test.tsx` or a follow-up revert commit) before merging the real PR.
- Est: 5 min (plus CI queue/run wait, not counted against the 2–5 min authoring estimate).

#### Story 1.4 — Docs (AC #7)

**Task 7 — Confirm no docs update is needed**
- Files: `README.md` (read-only verification, no edit expected)
- Action: `grep -n -i "ci\b\|workflow\|github actions" README.md` — confirmed during planning to return only the line-1 CI badge (linking to `build.yml`, unaffected by this change) and unrelated "Development workflows" Makefile-comment matches. No section enumerates individual workflow files or checks. Per `requirements.md` AC #7 ("README/docs updated only if they already document CI checks"), **no edit is made**. Record this grep output as the evidence for AC #7 rather than skipping the criterion silently.
- Est: 2 min.

---

## Given-When-Then per Acceptance Criterion

**AC #1** — Jest suite (all 4 projects) runs on PRs targeting main.
Given `.github/workflows/lint.yml`'s `golangci` job has the new `Jest` step (Task 3) running `npx jest --ci --maxWorkers=4` from `web-app/`, When a PR that modifies `web-app/src/components/backlog/BacklogEmptyState.tsx` is opened against `main`, Then the Actions run's Jest step log shows results for all 4 `displayName`s from `web-app/jest.config.js` (`web-app`, `eslint-plugin-analytics`, `dev-stack`, `e2e-dev-mode`).

**AC #2** — Same on push to main.
Given the same `Jest` step, When a commit is pushed directly to `main` (e.g. a squash-merge) that touches `web-app/jest.config.js`, Then the `push` trigger (lines 4–16 of `lint.yml`) fires the `lint` job and its Jest step runs identically to the PR case.

**AC #3** — Trigger paths cover the required set.
Given `lint.yml`'s `pull_request.paths` list includes `web-app/src/**`, `web-app/package.json`, `web-app/pnpm-lock.yaml`, `web-app/jest.config.js`, and `.github/workflows/lint.yml` (Task 4), When a PR touches only `web-app/pnpm-lock.yaml` (a dependency bump with no `.ts`/`.tsx`/`.css` change), Then GitHub still triggers the `lint` workflow run — which it would **not** have under the pre-Task-4 path list.

**AC #4** — A failing test fails the check.
Given the Jest step from Task 3, When `expect(true).toBe(false);` is added to an existing test in `BacklogEmptyState.test.tsx` and pushed on a scratch branch (Task 6), Then the `lint` check run reports failure (red ✗) with `FAIL src/components/backlog/BacklogEmptyState.test.tsx` visible in the Jest step's Actions log, and the job's exit code is non-zero.

**AC #5** — Node/pnpm setup reuses `lint.yml`'s pinned versions.
Given the Jest step is appended inside the existing `golangci` job rather than a new job or workflow (Pattern Decision, Creative Pass choice A), When the job runs, Then it reuses the `pnpm/action-setup@f40ffcd9367d9f12939873eb1018b921a783ffaa` (v4, version `10.27.0`) and `actions/setup-node@v4` (`node-version: '22'`) steps already at `lint.yml` lines 45–55 — no second Node/pnpm setup step is introduced anywhere in the file.

**AC #6** — Full suite passes green in the PR that adds this.
Given the branch has been synced onto `origin/main` per Task 1 (inheriting commit `bf5b19f201bda366b9a290406cf5e32f2a60cbcd` / PR #354, which already fixed both `BacklogEmptyState.test.tsx` and `SessionDetail.embedded.test.tsx`), When `npx jest --ci --maxWorkers=4` is run from `web-app/` inside a `node:22-bookworm` container (Task 5), Then the summary reads `Test Suites: 4 passed, 4 total` with 0 failed tests — a clean rerun of the exact reproduction steps `research/pitfalls.md` used to find the original 2-suite, 7-test failure.

**AC #7** — Docs updated only if they already document CI checks.
Given `README.md` contains only the line-1 CI status badge (pointing at `build.yml`, which this change does not touch) and no dedicated section listing individual workflow files or required checks (confirmed via `grep -n -i "ci\b\|workflow\|github actions" README.md`), When this PR is authored, Then no `README.md` edit is made, and the grep output is recorded as the AC #7 evidence trail (Task 7).

---

## ADR

See `project_plans/web-app-jest-ci/decisions/ADR-001-append-jest-step-to-lint-job.md` for the one decision judged genuinely reversible-cost and non-obvious enough to record: appending the Jest step to `lint.yml`'s existing `golangci` job rather than a new job or a standalone workflow file.
