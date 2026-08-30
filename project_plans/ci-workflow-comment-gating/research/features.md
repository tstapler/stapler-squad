# Research: Feature Landscape for Gating CI PR Comments (Agent 2)

Scope: `.github/workflows/benchmark.yml`, `ux-analysis.yml`, `build.yml`, `e2e-video.yml`,
applying the `registry-validation.yml:90-95` gate pattern. All line numbers verified against
the current worktree checkout (2026-08-12).

## 1. Sticky-comment cleanup gap — CONFIRMED

Read `registry-validation.yml` in full (lines 1-117). The gate is:

```js
// line 90-95
const hasDivergence = output.includes('Added') || output.includes('Removed') || exitCode !== '0';
const hasLowCoverage = parseFloat(pct) < 50;
if (!hasDivergence && !hasLowCoverage) {
  return;
}
```

This `return` is the **first thing the script does** after computing the signal — the
`listComments` / find-existing / `updateComment`/`createComment` block (lines 111-116, and in
general the pattern across all 5 workflows) sits entirely *after* the gate and is never
reached on a clean run. There is no `deleteComment` call anywhere in `registry-validation.yml`,
and `grep -rn "deleteComment" .github/workflows/` across the whole repo returns **zero
matches** — no workflow in this repo has ever deleted a stale sticky comment.

**Consequence, confirmed**: if PR run N shows divergence/low-coverage and posts a comment, and
PR run N+1 (after a fix push) is clean, the gate returns early on run N+1 *before* even
listing comments, so the stale "divergence detected" comment from run N is left on the PR
forever, showing outdated information. Copying this exact pattern into the other 4 workflows
would reproduce this gap verbatim — directly relevant to AC #2, which explicitly requires the
cleared case to either remove or update the stale comment.

**Minimal fix shape** (not yet in `registry-validation.yml`, needs to land in whichever gate
implementation the plan phase designs — including retrofitting `registry-validation.yml`
itself if plan scope allows, since it's the shared reference other workflows will copy):

```js
const { data: comments } = await github.rest.issues.listComments({...});
const existing = comments.find(c => c.body.includes(marker));
if (!hasDivergence && !hasLowCoverage) {
  if (existing) {
    await github.rest.issues.deleteComment({ owner, repo, comment_id: existing.id });
  }
  return;
}
```

i.e. hoist the comment lookup *above* the gate so the "clean" branch can act on a pre-existing
comment, rather than gating before comments are ever fetched.

## 2. Ecosystem convention: delete-on-clean vs. update-in-place

No single universal convention — both are established, documented patterns in the GitHub
Actions ecosystem:

- **`marocchino/sticky-pull-request-comment`** (a widely-used marketplace action) supports
  explicit comment deletion via a `delete: true` mode keyed on the same header/marker used to
  find the comment for updates — i.e. delete-on-clean is a first-class supported mode, not a
  workaround.
- **`mshick/add-pr-comment`** supports a `delete-on-status`/message-minimize option — same
  idea: collapse/remove the sticky comment when the triggering condition clears.
- **Danger.js** (`danger/danger-js`) is designed to edit-or-delete its own comment when there's
  nothing to report, though GitHub-Actions-specific issues have been filed
  (`danger/danger-js#936`) about it sometimes creating a new comment instead of updating,
  suggesting the delete/update behavior is more fragile in practice than in Bitbucket/GitLab.
- **CodeCov's PR comment action** is a *counter-example* worth naming explicitly: it posts
  unconditionally on every run (coverage is treated as a first-class report, not an advisory
  finding) — this is the "always show, it's not noise" school. That's a deliberate contrast to
  this repo's stated Kano framing (Basic Expectation: "CI didn't spam me"), so CodeCov's
  pattern is *not* the one to imitate here.

**Recommendation for planning phase**: given this repo's own AC #6 ("zero or near-zero
advisory comments" on a green PR) and Kano framing, **delete-on-clean** is the better fit than
"update in place to a green state" — an update-in-place still leaves a permanent comment
artifact on every clean PR (contradicting "near-zero"), whereas delete removes it entirely once
resolved. AC #2's wording ("either removed or updated") leaves both technically compliant, so
this is a recommendation, not a hard requirement — flag it as an open decision for `/sdd:3-plan`.

## 3. Per-workflow actionability signal availability

### `benchmark.yml` — 3 near-identical jobs (go-tier1 L106-137, frontend-throughput L332-363,
e2e-latency L546-577)

- **go-tier1** (L95-104 compare step): runs plain `benchstat baseline current` (no
  `-delta-test` flag) into `benchstat-tier1.txt`, then the comment step (L106-137) dumps that
  raw text verbatim into the comment with no threshold logic at all. **No machine-readable
  regression flag exists today** for this job — would need new parsing logic.
  - **In-repo precedent already exists** for exactly this parsing: `build.yml`'s
    `benchmark-gate` job (L451-463, main-branch-only, blocking) already does
    `benchstat -delta-test=utest baseline current | tee bench-diff.txt` followed by
    `grep -E '\+([2-9][0-9]|[1-9][0-9]{2,})\.' bench-diff.txt | grep -v '±'` to detect ≥20%
    regressions. The advisory PR-comment gate for go-tier1 could reuse this same regex
    technique at a different (presumably lower/more sensitive) threshold, and should also add
    `-delta-test=utest` to the PR-facing `benchstat` invocation at L99 (it currently omits the
    flag `benchmark-gate` uses), since without it there's no significance filtering — plan
    phase should confirm whether default benchstat marks non-significant deltas.
- **frontend-throughput** (L310-330 compare step) and **e2e-latency** (L524-544 compare step):
  both already compute a per-metric `pct` delta and directional `arrow` (▲/▼ or "faster/slower")
  in inline Node scripts, but only write formatted *text* to a `.txt` file — the regression
  determination (is this delta bad, and by how much) is never captured as a boolean/output.
  Minimal fix: extend each Node script to also compute `hasRegression = rows.some(r =>
  r.pct exceeds threshold)` and emit it via `require('fs').appendFileSync(process.env.GITHUB_OUTPUT, ...)`
  (or a sibling small script), then gate the comment step on that step output. This is a very
  small addition given the pct math already exists — no new capability required.

### `ux-analysis.yml` (comment step L149-203)

Three inputs combine, with very different signal maturity:

- **Axe** (`steps.axe.outcome`, already captured, used at L158): `continue-on-error: false`
  (L93) means Axe failure already fails the *job* — this is a real, already-blocking signal.
  Green Axe (`outcome === 'success'`) needs no comment contribution.
- **Lighthouse** (`steps.lighthouse.outputs.score`, L108 sets `score=`, threshold check
  already exists at L161-167 for emoji selection): `score < 70` is exactly the signal AC #3
  wants, already computed — gating just needs to reuse this existing `isNaN(score) ||
  score < 70` condition instead of only using it for emoji choice.
- **Claude UX analysis** (`tools/ux-analysis/analyze.ts`, run at L129-136 if
  `ANTHROPIC_API_KEY` is set): **no machine-readable signal is captured today.** Read
  `tools/ux-analysis/analyze.ts` in full — it does compute `result.findings` (an array with a
  `severity` per finding, L33/L208-212) and logs `${findings.length} finding(s)...` to stdout
  (L264), and writes a markdown report to `docs/qa/ux-findings*.md` (L254-257), but:
  - The workflow step has `id: screenshots` (L112) yet never captures a step output for
    finding count — the script's result is only visible in raw stdout/log and the written
    markdown file, neither of which the comment step currently reads.
  - The step itself is conditional on `ANTHROPIC_API_KEY != ''` (L113) — on forks/PRs without
    the secret, the step **doesn't run at all**, so `steps.screenshots.outcome` would be
    `skipped`, not `success`/`failure`. Any new gate must treat "skipped" as "no finding" (safe
    to suppress), not accidentally as "flagged" or crash on an undefined output.
  - **Needed for AC #3's third condition**: add a line in `analyze.ts`'s CLI path (or the
    workflow step, parsing its stdout/file) that writes `findings_count=N` to
    `$GITHUB_OUTPUT`, so the comment-gate step can read it like it already reads
    `LIGHTHOUSE_SCORE`.

### `build.yml` — feature-coverage comment (step at L216-247)

- Only the **current** absolute number is computed: `feature-coverage.ts` (invoked L206-214)
  prints `Feature E2E coverage: X/Y tested (Z%)`, captured into `steps.coverage.outputs.summary`
  — a plain string, not even a parsed numeric percentage.
- **No prior-run value is available anywhere in this job.** Confirmed via
  `grep -rln "actions/cache" .github/workflows/build.yml` — `build.yml` does use
  `actions/cache` twice (tmux binary cache L153-158, Next.js build cache is actually in
  `.github/actions/prepare`), but **never** for the coverage percentage. There is no
  `docs/registry/coverage-gaps.json`-vs-previous-commit diff either — the neighboring "Check
  new RPCs have tests" step (L249-264) diffs `docs/registry/features/` against `origin/main`
  for a *different* purpose (staleness of committed registry files, not coverage trend).
- AC #4 ("only post when coverage changed or dropped relative to the prior state") therefore
  needs a genuinely new mechanism. Two realistic options, both consistent with existing
  in-repo patterns:
  1. **Reuse the sticky-comment body itself as the "previous value" store** — the comment step
     already calls `listComments` and finds `existing` (L226-231); before overwriting, regex
     the *existing* comment's body for its previously-posted percentage, compare to the new
     one, and only post/update if they differ (or if it dropped). Zero new infrastructure,
     smallest diff, but only works if a comment already exists — first-ever run on a PR has no
     "previous" state to compare, which is fine (post unconditionally the first time a
     concerning value appears, matching the qualitative wording of AC #4).
  2. **`actions/cache` baseline file**, mirroring `benchmark.yml`'s exact pattern (cache key
     `coverage-pct-`, `restore-keys: coverage-pct-`, save on push to main). More consistent
     with the benchmark jobs' established idiom in this repo, but is new infrastructure for a
     single percentage number — likely overkill vs. option 1.
  - Recommend option 1 to `/sdd:3-plan` as the lower-effort path matching this item's
    "Low-Medium effort / mechanical" RICE rating.

### `e2e-video.yml` — notify job (L239-320)

- The "anomaly" signal AC #5 wants (**zero videos produced when videos were expected**) is
  **already fully computed inline** in the comment script itself: `videoArtifacts.length === 0`
  is literally the branch condition already used to select the "⚠️ No video artifacts were
  produced" message body (L279-286) vs. the normal "🎬 E2E Feature Demos" body (L287-297).
  **No new signal is needed at all** — this is the cheapest of the four fixes: wrap the
  existing `if (videoArtifacts.length === 0) { ...lines... }` branch's *else* branch (the
  normal-case navigation comment) so it simply doesn't call `createComment`/`updateComment`
  instead of building `lines` for the happy path, and only proceeds to post (and delete any
  stale prior comment, per the AC #2-adjacent cleanup issue) in the anomaly branch.
- Secondary note for planning: the job's own `if:` (L243) already gates the whole `notify` job
  on `needs.detect-feature-changes.outputs.record_features == 'true'` — so on PRs that don't
  touch feature-marked files, this job doesn't run at all today (that gate is already correct
  and out of scope).

## 4. Summary table

| Workflow / job | Actionability signal available today? | New logic needed |
|---|---|---|
| benchmark.yml go-tier1 | No — raw benchstat text only | Add `-delta-test=utest` + reuse `build.yml`'s existing regex-threshold technique (L459-460) at an advisory threshold |
| benchmark.yml frontend-throughput | Partial — pct/arrow computed but not thresholded | Add a `hasRegression` boolean from the already-computed `pct` values |
| benchmark.yml e2e-latency | Partial — same as above | Same as above |
| ux-analysis.yml (Axe) | Yes — `steps.axe.outcome`, already blocking | None (reuse) |
| ux-analysis.yml (Lighthouse) | Yes — `score < 70` already computed (L165) | None (reuse) |
| ux-analysis.yml (Claude UX) | No — findings computed in-script but never surfaced as a step output | Add `findings_count` to `$GITHUB_OUTPUT` in the workflow step or in `analyze.ts` |
| build.yml feature-coverage | No — only current absolute value, no prior-state comparison anywhere | Compare against existing sticky comment's previously-posted value (cheapest) or add a cache baseline (heavier, matches benchmark.yml idiom) |
| e2e-video.yml notify | Yes — `videoArtifacts.length === 0` already computed inline | None (reuse; just restructure control flow to skip the happy-path branch) |

## 5. Cross-cutting finding: gate-before-fetch vs. fetch-before-gate

Every one of the 5 workflows' comment steps follows the same shape: **gate check (if any) →
build `body` → `listComments` → find `existing` → update-or-create.** For AC #2-style cleanup
to work in any of the 4 target workflows (not just benchmark.yml, whose AC #2 explicitly calls
this out — the same "stale comment lingers" risk applies structurally to ux-analysis.yml,
build.yml, and e2e-video.yml too, since they share the identical script shape), the
`listComments`/`existing` lookup must be **hoisted above** the new gate in all of them, not
just appended as an afterthought — otherwise every one of the 4 new gates will reproduce the
same bug `registry-validation.yml` already has.
