# Requirements: promo-materials-refresh

**Date**: 2026-11-10
**Type**: feature addition (website/CI media pipeline + feature-registry extension)
**Complexity**: 3 — system design (multiple epics: media pipeline, tours section, registry doc-coverage gate)

## Problem Statement
The website, README and docs show screenshots and clips from a very old build of stapler-squad. The committed binaries (`website/public/features/01..08-*.png`, `website/public/demo.webm`, `assets/demo.gif`, `assets/screenshot.png`, `assets/demo-recording/*.png`) date from the initial site PR (#1051, 2026-10-15) and show only a handful of features. There is no place to present guided tours of features, and nothing in the feature registry checks that a feature is explained in the in-app `/help` docs or on the website, so docs and promo material drift from the product.

**Why the three problems are bundled, and the harm.** They share one cause: promo and docs are hand-maintained and nothing fails when they drift. (1) Stale media: the site shows a build 26 days older than `main` on 2026-11-10 (recorded 2026-10-15) and 8 of 19 cards carry a screenshot, so an evaluator sees a product that no longer exists. (2) No tours: the only walkthrough is 2 clips, so an evaluator cannot see the core value (approvals, review, backlog) without installing. (3) Docs drift: nothing checks that a feature has a help or website page, so existing users cannot find help for newer features. **The user harm is a hypothesis, not a measured fact**: evaluators leaving because the site shows an old product, and existing users failing to find help for newer features, are assumed from the evidence of staleness (the 26-day gap and the 8-of-19 coverage), not from observed behaviour. Validation signal: tour page views, README-to-`/tours/` click-through and release downloads after launch (see Outcome metrics; all "to be measured"). If those stay flat while the media is current, the harm hypothesis is weak and the tours need revisiting. What is falsifiable now is the drift: if nothing changes, every release widens all three gaps. The gap count G in problem 3 is **pending** plan Task 0.1.1f and no number is quoted until it runs. One pipeline fixes them together because the tours need fresh media, the freshness gate needs the media pipeline, and the docs gate links tours to registry features; separate efforts would each rebuild the same seeding and CI plumbing. Why now: the site and Pages deploy (PR #1051) exist, the registry is stable enough to gate, and every new feature merged before the gate lands adds to the grandfathered baseline. Priority (RICE-style, qualitative): reach is every evaluator and user, impact is high on evaluator trust, confidence is medium (outcome numbers are unmeasured, see below), effort is large and pursued in full. **Displaces**: no other roadmap item is known to compete for this capacity; Tyler confirms that at plan approval (UNVERIFIED, no roadmap source was consulted).

## Baseline
- `deploy-website.yml` already records clips in CI (`scripts/record-website-demos.sh`, `tests/e2e/website-demos/clips.ts`) and ships them in the Pages artifact, but the set is small and falls back to committed stale screenshots.
- `demo-publish.yml` (manual) publishes GIFs to a rolling `demo-gifs-latest` release.
- Screenshots and `demo.webm` are still committed to git (history bloat precedent: ~15.6 GB from earlier GIF commits).
- `docs/registry/features/**` tracks RPCs and UI features against test coverage only; no docs or website coverage fields.
- In-app help (`web-app/src/app/help`, `lib/docs/docLoader`) and website help pages (`website/src/content/docs/help/*`, synced by `sync-content.mjs`) are curated by hand.
- `website/src/content/features.json` has 19 feature cards (from `docs/features.md`, missing most newer areas); 8 have screenshots (research/features.md, VERIFIED against features.json), and only 2 CI clips exist.

## Primary persona and jobs
Primary persona: an **evaluator** (an engineer deciding whether stapler-squad helps them run several coding agents without babysitting). **The persona and the "decides in under 2 minutes" threshold are an owner (Tyler) assumption**, not research: no interviews or analytics back them. Proxy where one exists: tour page engagement (views of `/tours/` and the Start-here tours, if analytics exist); otherwise the threshold is **unmeasured**. The tours are capped at about a minute each (60 s clip, 3-6 steps) to fit the assumption. Secondary: an **existing user** finding help for a feature. Contributors and CI are consumers, not personas.
Jobs to be done: J1 "see it working on realistic data before I install" (evaluator); J2 "find the page that explains this feature" (existing user); J3 "add a feature without leaving docs behind" (contributor, enforced by the gate).
The 8 tours map to jobs: `dashboard-lifecycle`, `approval-review`, `organise-at-scale` (the Start-here set) serve J1 and carry the core value; `backlog-pipeline`, `fanout-compare`, `insights-cost`, `attention-notifications`, `automation-rules` serve J1 for power evaluators and J2 for existing users (each links its help page). Single screenshots per feature card serve J2.

## Users / Consumers
- Prospective users and evaluators landing on the website / README.
- Existing users reading in-app `/help` and website docs.
- Contributors/agents adding features (registry + docs gate).
- CI (GitHub Actions: deploy-website, website-ci, PR checks).

## Success Metrics
- Every media asset on the website, README and docs is produced by a CI run against the current `main` build; zero binary media files committed (excluding logos/favicons/icons and visual-regression snapshots) — verified by a CI check that fails on newly committed media binaries.
- Media coverage grows from 8 screenshot-backed cards (of 19) and 2 clips to every user-facing feature card having a current screenshot or clip, plus **N = 8** feature tours (set in planning, plan R2, from `research/features.md` s5).
- A `/tours` section exists on the website with one page per tour, built in CI and linked from the landing page and README.
- Registry entries carry docs-coverage fields (help page, website explanation); CI blocks a PR that adds or changes a registry feature without both, and a ratchet baseline of existing gaps may not grow.
- Staleness: weekly scheduled redeploy regenerates all media (existing cron), and a recorded-media freshness check fails if the media was not produced from the deployed commit.

## Outcome metrics (baselines and targets)
Success Metrics above are output metrics (things exist). Outcome metrics, with zero-tracking sources (plan Task 0.1.1g verifies whether website analytics exist; if none, **no tracking is added by this plan**, the metrics are marked "to be measured", named owner Tyler). **Sampling schedule**: pre-launch (before the landing/README change merges), launch (first deploy that serves a tour), week 2 and week 4; GitHub's traffic API keeps only 14 days, so the week-2 sample is what makes the 4-week reading complete. Every sample (date, command, value) is recorded in the "Outcome samples" table of `project_plans/promo-materials-refresh/implementation/spike-notes.md` (plan Task 0.1.1g). **Attribution limit**: releases, CI changes, bot traffic and outside promotion move the same numbers, so a movement is consistent with the tours working but does not prove it; targets marked provisional below stay provisional for that reason:
| Metric | Source | Baseline | Target |
|---|---|---|---|
| Tour page views | Site analytics if present, else none | to be measured | non-zero and growing 4 weeks after launch (numeric target set by Tyler after the baseline) |
| Click-through from README and landing to `/tours/` | `gh api repos/{owner}/{repo}/traffic/popular/paths` and referrers (14-day windows, needs push access) | to be measured | tours among the top 5 popular paths within 4 weeks (provisional hypothesis) |
| Release downloads per 4 weeks | `gh api repos/{owner}/{repo}/releases` asset `download_count` delta | to be measured at launch | not below the pre-launch 4-week delta (provisional) |
| Docs-coverage gaps (grandfathered + gaps) | `docscoverage` trend (plan Task 4.1.4b) | G from Task 0.1.1f | never grows; shrinks as tours link features |
| Media age on the live site | `media-manifest.json` commit date | 26 days at planning | 7 days or less (weekly cron) |

## Risky assumptions and validation signals
- Tours drive conversion: an evaluator who sees a tour is more likely to install. Signal: tours in top popular paths and a release-download delta after launch; if tour views are high but downloads flat, revisit tour content before adding more tours.
- Synthetic seed data looks credible: evaluators read it as a real team. Signal: owner review of seed data (wall-clock blocker) and the sample-data line on pages; qualitative feedback after launch.
- Contributors will not route around the docs gate with `exempt` or by growing the baseline. Signal: the exempt and grandfathered trend (plan Task 4.1.4b); an `exempt` rise in 3 of 4 weeks triggers a review comment.
- Docs-gate friction causes bypass or disablement: contributors hit the gate on unrelated PRs and respond by setting `docsCoverageEnforce` back to `false`, growing `exempt`, or asking for admin merges. Signal: PRs that flip a flag in `.github/promo-flags.json` to `false`, the `exempt` trend, and the would-have-failed rate before the flip (under 3%); two flag-off PRs within a month after the flip means the gate's scope is wrong and is revisited, not forced.
- Weekly recording is stable enough: the scheduled run records every clip without falling back to last-known-good. Signal: before the flip, the 10-consecutive-green soak (plan Story 5.1.2); after it, a success-rate threshold of at least 9 of the last 10 scheduled full runs recording every clip on the first or second attempt (per-clip `attempts`/`flaky` rows); below that, fix the flaking scenes before flipping `freshnessEnforce`.
- The 8 tours match what evaluators want: the set came from the registry and `docs/features.md`, not from evaluator input. Signal: if the three Start-here tours draw under half of tour views, or the user's own sense after launch is that a different workflow should lead, reorder via `tour.order` and revisit the set; with no analytics this is **unmeasured** and rests on owner review of the storylines.

## Size
Large (3–15M CU) — informational; full scope pursued, not a gate.

**Wall-clock blockers**: owner review of tour storylines/seed data and branding; first CI run of the expanded recording matrix (Playwright + ffmpeg runtime on GitHub runners); GitHub Pages deploy after merge; `demo-publish.yml`/deploy workflow dispatch needed once post-merge (per PR #1051 follow-up note).

## Constraints
- No binary media committed to git; media generated in GitHub Actions and shipped via Pages artifact (and/or release assets where embedding elsewhere is needed).
- Recording must be hermetic: isolated server with seeded data (existing `--test-mode`/private tmux socket pattern), no dependency on the live `:8543` instance.
- Pages artifact size and CI runtime must stay within sensible bounds (existing 45 min record-clips timeout; `docs/reference/ci-job-budget.md` governs job growth).
- Must respect existing website gates: asset-health manifest, internal-link check, a11y/Lighthouse, reduced-motion video handling.
- All demo clips and screenshots are recorded in the app's Grimdark theme (the theme the website uses), applied deterministically in the recording harness (not left to the default theme).
- Desktop and mobile form factors both considered for screenshots/tour pages.
- Repo conventions: Go for new scripting where practical, Bun for JS, e2e conventions (feature header, no `waitForTimeout`, `data-testid`/ARIA locators).

## Non-functional Requirements
- **Performance SLO**: tours/landing pages meet existing Lighthouse thresholds; media lazy-loaded, `prefers-reduced-motion` respected.
- **Scalability**: adding a tour = adding one spec/clip definition + one content entry, no pipeline changes.
- **Security classification**: public (seed data must contain no real paths, tokens, hostnames or user data).
- **Data residency**: no special requirements.

## Scope
### In Scope
- Audit and replace all stale promo media (website, README, docs) with CI-generated media; remove committed binaries from the tree.
- Record everything under the Grimdark theme (theme selection set via the recording harness, with a check that the captured media really is the Grimdark theme).
- Expand the screenshot/clip set to cover all user-facing features; richer, realistic seeded demo data reflecting the current UI.
- New `/tours` section on the Astro/Starlight site: tour pages (clip + annotated screenshots + steps), index, links from hero/README/features grid.
- CI: record → post-process → publish in Pages artifact; guard against committing media; freshness/health checks.
- Registry extension: docs-coverage fields (in-app help doc, website explanation, optional tour), generator/validator updates, blocking CI gate for new/changed features with ratchet baseline for existing gaps, coverage report.
- Backfill is limited to what the ratchet needs (baseline file); closing existing doc gaps is reported, not required. **Non-goal**: the docs gate does not backfill or fix existing drift; it only stops new drift. Bypass (exempt or baseline growth) is monitored by the exempt/baseline trend rather than assumed away.

### Out of Scope
- Rewriting product copy/brand strategy or the Grimdark theme.
- Mobile app store assets.
- Non-English localisation.
- Purging historical blobs from git history.
- Changing the in-app help renderer beyond what coverage checks need.

## Rabbit Holes
- Deterministic recordings: animations, timestamps, live data and agent output make screenshots/GIFs flaky or non-reproducible; need frozen clock/seed fixtures.
- Seeding realistic multi-session state (approvals, PRs, backlog, escape analytics) in an isolated server without real agents.
- GitHub runner cost/time: matrix sharding, caching, Pages artifact size caps.
- Defining "user-facing feature" and mapping registry IDs → help doc → website page robustly (naming drift, one page covering many features).
- Ratchet baseline merge conflicts across concurrent PRs (per-feature files mitigate).
- Grimdark theme must be applied reliably at record time (theme storage/selection mechanism, fonts loading before capture) and stay consistent with the website's Grimdark styling.
- Some features are flag-gated (`FeatureGuard`) and need flags enabled for recording.
- Keeping `sync-content.mjs`-generated help in step with in-app `/help` docs.

## Alternatives Considered
- Dedicated `/tours` section on the site (chosen).
- Gallery page + rolling release assets; per-feature embeds only (rejected by owner).
- Committing media with Git LFS (rejected: goal is no binaries in repo).
- Report-only or fully blocking docs gate (rejected in favour of blocking-for-new plus ratchet).

## Feasibility Risks
- CI recording flakiness under load (existing script tolerates partial failure; fallback behaviour must not reintroduce committed screenshots).
- Pages artifact size/deploy time as media grows.
- Features needing real agent output or external integrations (GitHub, Slack, Jira) can't be shown live; require fixtures/mocks.

## Observability Requirements
- Recording job emits a per-clip result summary (recorded/failed/skipped, size, sha256) to the job summary; failed clips fail a freshness check rather than silently falling back to stale media.
- Weekly scheduled run failure already notifies via existing `notify` job; extend to cover new checks.
- Docs-coverage gate prints actionable per-feature output (missing help/website link) and a coverage count.

## Risk Control
- Roll out in stages: (1) pipeline + media-commit guard, (2) tours section behind existing deploy, (3) registry fields + report-only mode, (4) flip gate to blocking for new/changed features.
- Rollback: gate is a single CI step/flag; tours section is additive; stale-screenshot fallback can be re-enabled by reverting one workflow step.
- Pages deploys are revertible by redeploying the prior commit.

## Open Questions
Resolved questions were pruned: README embeds vs links and `demo-publish.yml` (decided below under "Decisions after Phase 2 research": README links to `/tours`, `demo-publish.yml` retired); which features get tours (N = 8, plan R2); where tour definitions live (ADR-001: Starlight frontmatter plus one scene file); registry docs-field schema and the ratchet baseline (ADR-005: per-feature sidecars, `make docs-coverage-baseline`).
- How the mapping to in-app `/help` docs (`docLoader`) is verified without a Next build (plan Task 0.1.1a; leading answer: parse the `DOC_FILES` literal in Go).
- Tour storylines, seeded narrative and imagery review for Grimdark naming (owner: Tyler, blocks merge of the tour stories).

## Naming / IP Constraint
The theme is called only the "Grimdark" theme everywhere: public-facing copy (website, tours, alt text, captions, README, media filenames, titles, theme labels in recorded UI) **and the code itself**. No "Warhammer", "40K", "WH40K" or other trademarked names or imagery anywhere user-visible or in source.

The rename is in scope: theme id and label in web-app, `website/src/styles/wh40k.css` and its imports, e2e visual-regression snapshot directories (`visual-wh40k`) and specs, test names, docs, feature registry entries, and any persisted setting. Persisted user preference migration is required: users who stored the old theme id must land on Grimdark, not fall back to the default (planning decides the mapping/migration and whether to keep a legacy-id alias). Add a CI check that fails on trademarked terms in source and published content (allow-list only for the migration alias, if one is kept).

## Decisions after Phase 2 research
- README links to `/tours` pages on the site (no release-asset embeds); `demo-publish.yml` is retired.
- Unresolved, carried into plan.md: whether `docLoader` can export a slug manifest without a Next build; licence and Astro 7 compatibility of any third-party tool adopted *(unresolved after Phase 2 research)*.
- Tracked build output (`web-app/.next`, `web-app/playwright-report`, `build/*.exe`, `uploads/`) was untracked from git and ignored (done in this branch, uncommitted).
