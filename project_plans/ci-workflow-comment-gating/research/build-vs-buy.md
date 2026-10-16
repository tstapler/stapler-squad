# Research: Build vs. Buy — Gating PR-Comment Steps

Agent: Research Agent 6 (Build vs. Buy), SDD Phase 2.

## Grounding: what's actually in these workflows today

```
$ grep -n "uses:" .github/workflows/{ux-analysis,benchmark,build,e2e-video,registry-validation}.yml
```

All five comment-posting steps use `actions/github-script` (pinned `@v7` in
`ux-analysis.yml:151`, `build.yml:218`, `e2e-video.yml:246`, `registry-validation.yml:75`; pinned
to a commit SHA `f28e40c...# v7` in `benchmark.yml:108,334,548`) with a hand-rolled
find-by-HTML-marker → `updateComment`/`createComment` block, ~20 identical lines each. No
third-party comment or check-run action is used anywhere in `.github/workflows/` today —
confirmed: `grep -rn "marocchino\|peter-evans" .github/workflows/` → no hits, and
`grep -rn "checks: write\|checks.create\|checks.update"` → no hits.

`.github/actions/` contains exactly one composite action, `prepare/action.yml` (protobuf/ent/web-UI
build steps), used by `ux-analysis.yml:48`, `e2e-video.yml:84`, `build.yml:56`, and
`registry-validation.yml:35`. It composes `actions/setup-go`, `pnpm/action-setup`,
`actions/setup-node`, `bufbuild/buf-setup-action`, and shell steps — no JS scripting, no
`github-script` usage inside it. That's the only local precedent for "shared composite action" in
this repo, and it's mechanically a different shape (environment setup, not GitHub API scripting).

`permissions:` blocks already scope `pull-requests: write` narrowly per job (`ux-analysis.yml:16`,
`e2e-video.yml:15`, `build.yml:40`, `benchmark.yml:59,253,445` — each of the three comment-posting
benchmark jobs declares it separately; the non-commenting `frontend-lighthouse-comment`... wait,
the two non-commenting benchmark jobs at lines 204 and 387 correctly omit `pull-requests: write`).
No workflow currently declares `checks: write` anywhere.

## Option 1 — Build in-house: copy `registry-validation.yml`'s gate pattern

**What it is:** Add an `if (!actionable) { return; }` early-exit at the top of each comment
step's script body, mirroring `registry-validation.yml:90-95`, using the specific data source
each workflow already extracts into a step output or env var (benchstat text, Lighthouse score,
coverage pct, video count).

**Pros**
- Zero new dependencies, zero new permissions, zero new action pins to track/Dependabot-update.
- Exactly matches the pattern already proven working in this repo for 100% of its runtime (
  `registry-validation.yml` has shipped this since before this item existed).
- Every one of the 4 target workflows already computes the input the gate needs as a step
  output (`steps.lighthouse.outputs.score`, `steps.axe.outcome`, `steps.coverage.outputs.pct`,
  the benchstat text file, `needs.detect-feature-changes.outputs.record_features`) — the gate
  condition is a few lines of arithmetic/string-matching on data that's already there, not new
  data collection.
- Change is entirely additive and localized: no step reordering, no new jobs, no new triggers.
  Each of the 6 gate sites is independently reviewable and revertable.
- Stale-comment cleanup (AC #2, benchmark) is the same shape as create/update — a
  `deleteComment` call on the same `existing` lookup already present, not new machinery.

**Cons**
- 4 near-duplicate implementations of "compute actionability" (each workflow's gate logic is
  necessarily bespoke to its own data — benchstat regression parsing differs from a numeric
  Lighthouse threshold differs from a coverage delta), so this is not a single shared change;
  it's 4 separate, hand-written conditions to get right (this is inherent to the problem, not
  specific to choosing Option 1 — see Option 4 below on how much of *that* is actually shareable).

**Verdict: Recommended.** This is the baseline fix the requirements document assumes, and
nothing found in this survey undercuts that assumption — no better-fit tool exists in the
ecosystem for "conditionally skip a step already using github-script," and the in-repo precedent
is exact.

## Option 2 — Source from an existing GitHub Action

Checked the three named candidates plus whether Lighthouse CI's own action already does this:

**`marocchino/sticky-pull-request-comment`** — handles the sticky-find-and-update part (header-based
matching, `hide_and_recreate`/delete modes) but has **no built-in actionability/threshold logic**
— the caller still has to compute "should I post" and either skip the step (`if:`) or pass empty
`message:` (which the action treats as "delete the comment" in `hide_and_recreate` mode, or a
no-op depending on config). So adopting it would still require writing the same gate condition
this item is about — it would only replace the ~10-line CRUD block (list/find/update/create),
not eliminate the actual work item. Would add a new external action dependency (SHA-pin +
Dependabot tracking) to save writing code that's already correct and running (
`registry-validation.yml`).

**`peter-evans/create-or-update-comment`** — same shape and same limitation: does the
find-by-marker/update-or-create mechanics, no gating/threshold logic. Same verdict as above.

**`actions/github-script` (already in use)** — no reason to move off it. It's the tool that lets
each workflow express its bespoke gate condition in plain JS against `github.rest.issues.*`
directly, which is exactly what's needed; a dedicated sticky-comment action would only replace a
part of the script that isn't the part causing the problem.

**`treosh/lighthouse-ci-action`** — not currently used. `ux-analysis.yml` invokes `lhci autorun`
directly via the LHCI CLI (`ux-analysis.yml:97-99`) inside a `run:` step, extracting the score
with a small inline Node snippet, not via this action. `treosh/lighthouse-ci-action` does have
`uploadArtifacts`, budget-based `github-status` checks (writes a commit status, not a full Check
Run), and (per its README) can post a PR comment itself when scores regress — but adopting it
would mean **replacing the existing `lhci autorun` CLI invocation and its Axe-adjacent
integration** with a new action's config format (`.lighthouserc` vs the current
`lighthouse.config.js` used with the CLI), a materially larger and riskier change than adding an
`if` gate to an existing script step, for a workflow where Axe (not Lighthouse) is the actual
blocking check and Lighthouse is explicitly advisory-only already.

**Pros of buying in general:** less code to hand-verify for the CRUD mechanics.

**Cons:** none of the three eliminate the actual work (the gate condition); two add an external
action dependency for no functional gain over the in-repo pattern; the fourth is a
disproportionately large rewrite for a comment-gating item.

**Verdict: Not recommended.** No candidate action solves the actual problem (deciding
actionability); at best they solve a sub-problem (sticky-comment CRUD) that isn't broken.

## Option 3 — GitHub-native Check Runs API (`checks: write`)

**What it is:** Replace some or all of these 4 advisory comments with `github.rest.checks.create`
calls, showing status inline in the PR's Checks tab instead of (or alongside) a comment thread.

**Effort assessment:**
- New permission scope needed on every job that would create a check run: `checks: write`
  (currently absent from all 5 workflows — confirmed via grep above). This is a broader
  permission grant than `pull-requests: write` and needs its own review.
- `checks.create` requires an app/token with check-run write access — the default
  `GITHUB_TOKEN` covers same-repo PRs but check runs created via `GITHUB_TOKEN` are
  attributed to `github-actions[bot]` and don't have some of the richer features (custom
  check-run apps, annotations tied to a GitHub App identity) — usable here, but worth flagging
  since annotations (`output.annotations`) have a 50-per-call/API-paginated limit that none of
  the current comment bodies need to worry about.
- Structurally different UX: a Check Run is inherently pass/neutral/fail against a specific SHA,
  re-created per run (no sticky-comment "find and update" concept needed — GitHub already
  dedupes check runs by name+SHA) — this actually *removes* the CRUD complexity Option 2 was
  trying to buy, for free, as a side effect of the API shape.
- Would require rewriting each script block's output shape (Markdown comment body →
  `output: { title, summary, text }` check-run fields) — a real rewrite, not a gate condition
  add-on, for each of the 4 workflows.

**Pros**
- Structurally correct home for scored-but-non-blocking signals (Lighthouse score, benchmark
  deltas, coverage %) — exactly what requirements.md's Open Questions names it for. Shows in the
  PR's native Checks list without contributing to comment-thread noise regardless of gating,
  which is a stronger fix than gating a comment (a gated comment can still reappear/update
  and clutter the thread on every regression-to-fixed-to-regression cycle; a check run just
  reflects current state per-SHA with no accumulation).
- Removes the need for marker-based sticky-comment CRUD entirely for whichever signals migrate.

**Cons**
- New permission surface (`checks: write`) across up to 4 workflows — needs its own security
  review, out of proportion to a comment-noise fix.
- Real rewrite effort per workflow (4x), not a copy-paste gate — contradicts the Low-Medium
  effort estimate in requirements.md for the *baseline* fix, which is explicitly why
  requirements.md scoped this out (see "Out of Scope").
- Requirements.md explicitly places this behind Open Questions, not the acceptance criteria —
  confirmed at requirements.md:109-111 and :123-125.

**Verdict: Viable as a fast-follow, not recommended for this item.** The research supports
requirements.md's own scoping: Check Runs is a strictly larger, structurally better long-term
answer for the *scored* signals (Lighthouse, benchmark deltas, coverage%) but is 4x the rewrite
effort of Option 1 and adds a new permission scope that deserves its own review cycle. Flag it
explicitly in the PLAN phase as a named fast-follow item, not a redesign folded into this fix.

## Option 4 — Fork/adapt `registry-validation.yml`'s script into a shared composite action

**Precedent check:** `.github/actions/` exists today with exactly one composite action
(`prepare/action.yml`), which composes `uses:` steps and shell `run:` steps — it does not wrap a
`github-script` JS body. There is no existing precedent in this repo for a composite action that
wraps `actions/github-script`'s scripting mechanism itself.

**What's actually shared vs. bespoke across the 5 workflows' comment steps**, from reading all
five script bodies (`registry-validation.yml:83-109`, `benchmark.yml:110-137` ×3,
`ux-analysis.yml:155-181`, and `build.yml`/`e2e-video.yml`'s equivalent blocks):

| Part | Shared? |
|---|---|
| Marker constant, `listComments` → `find(c => c.body.includes(marker))` → `updateComment`/`createComment` | Identical shape in all 5 (differs only in marker string) |
| Actionability gate condition (`hasDivergence`, Lighthouse `score < 70`, benchstat regression, coverage delta, video count) | **Bespoke per workflow** — different data sources, different thresholds, no shared logic possible without a generic "is this number outside this range" abstraction that would be thinner than the code it replaces |
| Comment body Markdown template | Bespoke per workflow (different content structure) |

**Pros of extracting the shared CRUD part**
- Would deduplicate ~15 lines × 5 sites → 1 composite action + 5 short call sites, real
  maintenance win if this pattern grows to more workflows.
- `actions/github-script` *does* support composite actions calling it internally (a composite
  action step can itself be `uses: actions/github-script@v7` with `script: |` sourced from a
  `.js` file via `script-path:`), so this is technically feasible without inventing new
  infrastructure.

**Cons**
- `actions/github-script`'s `with: script:` is inline YAML by convention in this repo (no
  workflow currently uses `script-path:` to source from a `.js` file) — introducing a composite
  action here is a bigger structural departure from repo convention than the `prepare` action's
  pattern (which wraps whole third-party actions, not inline scripting).
- The part that's actually the *subject of this item* (the actionability gate) is exactly the
  part that **can't** be shared — it's bespoke per data source. Extracting only the CRUD
  mechanics doesn't touch the acceptance criteria at all; it's a separate refactor with its own
  risk (getting the composite action's input/output contract right across 5 call sites) for a
  problem (~75 duplicated lines total) that isn't the one requirements.md describes.
- Would expand this item's blast radius beyond "add a gate condition" into "introduce a new
  composite-action shape + migrate 5 call sites," raising review surface for a Low-Medium-effort
  item requirements.md explicitly scoped as mechanical.

**Verdict: Viable, but not recommended for this item.** The CRUD deduplication is real but
orthogonal to the acceptance criteria — it's a legitimate follow-up refactor (worth a backlog
note), not something the PLAN phase should fold into the comment-gating fix. Doing it now would
inflate a Low-Medium item into a structural refactor touching all 5 workflows' comment mechanics,
for a benefit (curbing ~75 duplicated lines) unrelated to the actual complaint (comment noise).

## Summary Table

| Option | Verdict |
|---|---|
| 1. Copy `registry-validation.yml`'s gate pattern in-house | **Recommended** |
| 2. Adopt `marocchino/sticky-pull-request-comment` / `peter-evans/create-or-update-comment` / `treosh/lighthouse-ci-action` | Not recommended |
| 3. GitHub-native Check Runs API for scored signals | Viable — fast-follow, not for this item |
| 4. Extract shared composite action for sticky-comment CRUD | Viable — separate refactor, not for this item |

## Recommendation for PLAN phase

**Confirms** requirements.md's own effort estimate and Suggested Entry Point: copy
`registry-validation.yml:90-95`'s gate pattern into the 6 sites (3 in `benchmark.yml` + 1 each in
`ux-analysis.yml`, `build.yml`, `e2e-video.yml`), using the step outputs each workflow already
computes. No new action, no new permission, no new composite-action infrastructure is justified
by this survey. Nothing evaluated here beats "add an `if` gate mirroring the pattern already
proven in this repo."

Two items surfaced during this survey are worth carrying into requirements.md's Open Questions
(they're already there) with this research's evidence attached, but should NOT be pulled into
this item's scope:
- Check Runs API (Option 3) as a fast-follow for the scored signals specifically — real
  permission and rewrite cost, correctly deferred.
- Shared composite action for sticky-comment CRUD (Option 4) as a separate housekeeping refactor
  once/if a 6th such workflow appears — not blocking, not urgent, ~75 duplicated lines is not
  large enough on its own to justify introducing a new composite-action shape not otherwise used
  in this repo (`.github/actions/prepare` wraps external actions/shell, not inline
  `github-script` bodies).
