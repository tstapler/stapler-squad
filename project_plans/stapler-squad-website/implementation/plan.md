# Implementation Plan: stapler-squad-website

**Feature**: A GitHub Pages marketing + help-docs site for Stapler Squad (Astro + Starlight), with a new automated higher-production-value demo-recording pipeline and a GitHub Actions deploy workflow that fixes the repo's long-broken Pages config.
**Date**: 2026-10-14
**Status**: Ready for implementation
**ADRs**: [ADR-001 (SSG choice: Astro + Starlight)](../decisions/ADR-001-ssg-choice-astro-starlight.md), [ADR-002 (demo-asset URL convention)](../decisions/ADR-002-demo-asset-url-convention.md), [ADR-003 (docs-curation copy-and-transform)](../decisions/ADR-003-docs-curation-copy-and-transform.md)

---

## Step 0.5 — Creative pass (alternatives explored)

Three distinct high-level approaches were weighed for the site's foundation (full detail in
ADR-001):

1. **Astro + Starlight** — *Strength*: zero-JS-by-default islands architecture is a direct,
   structural answer to the ≥90 Lighthouse NFR, and one project's splash+docs templates cover
   both landing page and help docs. *Weakness*: a new framework/toolchain with zero
   code-sharing with `web-app/`.
2. **Nextra on the existing Next.js 15 toolchain** (`research/build-vs-buy.md`'s pick) —
   *Strength*: reuses the already-proven pnpm/Node CI steps and `output: "export"` pattern.
   *Weakness*: React-hydration-by-default works against the Lighthouse NFR, and Nextra's own
   release maturity was never live-verified the way Starlight/VitePress/Docusaurus were.
3. **VitePress** — *Strength*: the most turnkey GitHub Pages subpath (`base`) story of the
   three dedicated SSGs. *Weakness*: introduces Vue as a third UI framework with zero reuse
   elsewhere in the repo, and a weaker marketing/splash template than Starlight.

**Chosen: Astro + Starlight** (ADR-001). Rejected alternatives and reasons are recorded in the
Pattern Decisions table below and in ADR-001.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SourceDoc` | An original, dev-facing doc file in `docs/how-to/*.md`, `docs/features.md`, or `README.md` — the authoritative version, unchanged by this project. | Never edited by the website project, except for the single, pre-existing-bug-fix edit to `docs/features.md` in Task 2.2.2a (removing a dangling broken-image reference) — see Tech Debt Disposition. Otherwise only read. |
| `CuratedDoc` | The copied-and-transformed version of a `SourceDoc` that ships on the public site, with internal-only links stripped and Starlight frontmatter injected. | Lives at `website/src/content/docs/help/<slug>.md`. Committed, reviewable in a PR diff. |
| `ContentManifest` | The version-controlled list of `{ source, slug, include, reason }` entries recording which `SourceDoc`s become `CuratedDoc`s and why. | `website/scripts/content-manifest.json`. See ADR-003. |
| `IncludeDecision` | The sum type on a `ContentManifest` entry: `Included(slug)` or `Excluded(reason)` — never a bare boolean, so the judgment call is recorded, not just the outcome. | Modeled in TS as `{ include: true, slug: string } \| { include: false, reason: string }`. |
| `sync-content` | The one-directional script that reads the `ContentManifest` and produces `CuratedDoc`s, failing loudly if a referenced `SourceDoc` no longer exists. Two modes: **generate** (default invocation, on-demand only — a human/agent runs it to create/update `CuratedDoc`s) and **verify** (`--check`-style idempotency mode, run automatically on every deploy per Story 6.3.1 — fails the build if the committed `CuratedDoc`s have drifted from what regenerating them would produce, rather than regenerating them). The deploy workflow only ever runs verify mode. | `website/scripts/sync-content.mjs`. |
| `DemoClip` | One named, scripted Playwright-recorded video/GIF asset (e.g. `review-queue`, `session-lifecycle`) produced by the demo-recording pipeline. | Identity = filename stem, shared between the recording workflow and the site's embeds. |
| `ReleaseAssetRef` | The typed reference to a `DemoClip`'s published location: `{ tag, name, ext }`, rendered to the URL `https://github.com/tstapler/stapler-squad/releases/download/<tag>/<name>.<ext>`. | `website/src/lib/release-asset.ts`. See ADR-002. Never raw string interpolation in MDX. |
| `website-demo-gifs-latest` | The rolling GitHub Release tag holding the new, higher-production-value `DemoClip`s — distinct from the pre-existing `demo-gifs-latest` tag `demo-publish.yml` already owns. | Overwritten via `gh release upload ... --clobber`, same pattern as the existing tag. |
| `AssetHealthManifest` | The version-controlled map of each `ReleaseAssetRef`'s `name.ext` to the SHA256 of its expected content, written by `website-demo-publish.yml` immediately after each `gh release upload --clobber` so the recorded hash always matches what was just published. Distinct from `ContentManifest` (which drives *inclusion* decisions for docs) and from the generated-manifest option ADR-002 rejects for *URL discovery* — this manifest never decides which assets exist or what their URL is; it only records expected content identity for an already-known `ReleaseAssetRef`, so it doesn't reopen ADR-002's decision. | `website/scripts/asset-health-manifest.json`. See pre-mortem.md Failure #5. |
| `AssetHealthCheck` | The build-time step that, for every `ReleaseAssetRef` URL referenced in site content, fetches the live asset, computes its SHA256, and compares it against the `AssetHealthManifest`'s recorded hash for that name — failing the build/deploy on a non-2xx/3xx response **or** a hash mismatch (stale/corrupted `--clobber` content at an otherwise-valid URL). | `website/scripts/check-asset-health.mjs`. |
| `BrokenLinkCheck` | The build-time step that validates every internal link between `CuratedDoc`s (and the landing page) resolves to a real route, catching rename/typo drift before deploy. | `website/scripts/check-internal-links.mjs`. |
| `FeatureCard` | One entry in the landing page's feature-highlights grid: `{ heading, description, screenshot?, category }`, sourced from `README.md`'s Highlights section and `docs/features.md`. | `website/src/content/features.json` or equivalent typed data file. |
| `FeatureSpec` | A curated Playwright spec filename reused from `demo-publish.yml`/`e2e-video.yml`'s existing `FEATURE_SPECS` set (`demo.spec.ts`, `review-queue.spec.ts`, `session-lifecycle.spec.ts`, `history-search.spec.ts`, `workspace-management.spec.ts`). | The new recording workflow reuses this exact set as its starting scope — no new specs invented. |
| `SplashPage` | Starlight's marketing-page template (`template: splash` frontmatter), used for the site's landing page (`website/src/content/docs/index.mdx`). | Distinct from Starlight's default docs layout, which renders `HelpSection` pages. |
| `HelpSection` | The Starlight docs collection (`website/src/content/docs/help/**`) holding all `CuratedDoc`s, organized into Getting Started / Features / Troubleshooting sidebar groups. | Sidebar grouping configured in `astro.config.mjs`'s `starlight()` integration options. |
| `BasePath` | The Astro `base: '/stapler-squad'` config value (paired with `site: 'https://tstapler.github.io'`) that makes every Starlight-generated link/asset URL subpath-correct. | Set once in `astro.config.mjs`; the single most likely "works locally, breaks live" bug class per `research/pitfalls.md` §2c. |
| `PagesDeployWorkflow` | The new `.github/workflows/deploy-website.yml` — checkout → `configure-pages` → `astro build` → `upload-pages-artifact` → `deploy-pages`, triggered on push to `main`. | Fixes the long-broken `build_type: workflow`-with-no-workflow Pages config. |
| `WebsiteDemoWorkflow` | The new `.github/workflows/website-demo-publish.yml` — `workflow_dispatch`-only, records `FeatureSpec`s, applies the extended post-processing, publishes `DemoClip`s to `website-demo-gifs-latest`. | Separate from, not a modification of, the existing `demo-publish.yml`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Site framework/SSG | Astro + Starlight | ADR-001 | Nextra on Next.js 15 (build-vs-buy.md's pick); VitePress; Docusaurus; hand-rolled HTML/Jekyll | Nextra costs the zero-JS Lighthouse edge and was never version/maturity-verified like the other candidates; VitePress is a viable but weaker-fit runner-up (third framework, weaker splash template); Docusaurus's extra feature surface (versioning/i18n/blog) is explicitly out of scope; hand-rolled/Jekyll lose all nav/search ergonomics the requirement wants |
| Docs-curation sync mechanism | Transaction Script (Fowler) — a single procedural `sync-content.mjs` reading a manifest and writing files, no persistence layer, no domain model | PoEAA; ADR-003 | Live symlink/build-time read of `docs/`; one-time manual copy-paste; a Repository-pattern abstraction over content sources | A Repository/Domain Model is over-engineering for a one-directional, bounded, infrequent (manifest changes "maybe once a quarter" per `research/build-vs-buy.md`) copy operation — Transaction Script matches the complexity; live-read reintroduces the unreviewed-mirror problem; manual copy-paste has no drift detection |
| `ContentManifest` entry shape | Type-driven design: `IncludeDecision` sum type, not a bare boolean | Type-driven design (Parse-Don't-Validate) | A plain `{ include: boolean }` flag | A bare boolean loses the *reason* for a judgment call (e.g. why `run-nil-safety-analysis.md` is excluded) — the sum type forces every entry to carry its justification, which is exactly what `requirements.md`'s Rabbit Holes section asks for ("explicit file-by-file include/exclude list") |
| Demo-media reference mechanism | Hardcoded `ReleaseAssetRef` URL convention, centralized in one typed helper | ADR-002 | A generated/committed manifest file; a build-time GitHub API call | The API call violates the "never touch the recording pipeline's live state at deploy time" constraint outright; the generated manifest adds a sync-and-commit step for a problem the filename-is-the-identity convention already solves |
| `ReleaseAssetRef` representation | Value Object / newtype (`{ tag, name, ext }` + a single `releaseAssetUrl()` constructor function) | Type-driven design (primitive obsession fix) | Raw string interpolation (`` `https://github.com/.../${tag}/${name}.${ext}` ``) scattered across MDX files | Scattered string interpolation is exactly the primitive-obsession failure mode `research/pitfalls.md` flags as the mechanism behind silent 404s on asset rename — one typed constructor makes a tag/name change a one-place edit and gives `AssetHealthCheck` a single enumerable source of truth |
| Asset content-identity verification | Content-hash manifest (`AssetHealthManifest`, SHA256 keyed by `name.ext`), written by `website-demo-publish.yml` at publish time and diffed by `check-asset-health.mjs` | pre-mortem.md Failure #5 (P1) | Status-code-only HEAD check (original plan); re-querying the GitHub API for asset metadata at build time | A HEAD check can only prove the URL resolves, not that `--clobber` actually delivered the new bytes (stale CDN edge, interrupted upload) — the same "is this the *right* asset, not just *an* asset" gap `ReleaseAssetRef` closed for URLs; a build-time API call repeats ADR-002's already-rejected "touch the recording pipeline's live state at deploy time" mistake, so the hash is recorded once, at publish time, instead |
| Demo post-processing ("higher production value") | Extend `scripts/demo-post-process.sh`'s existing ffmpeg filter-graph in place (additional filter stages: title-card `drawtext`/`overlay`, consistent crop/letterbox) | `research/build-vs-buy.md` §2 | Adopting an external community Playwright-overlay "skill"/plugin | No mature, widely-adopted external tool exists for this need (verified by web search); this repo already has a more specific, working implementation to extend, and extending costs less than reconciling a generic tool's overlay conventions with the existing brand chrome |
| Pages deploy workflow shape | Plain declarative GitHub Actions YAML — no code-level pattern forced | GitHub's own documented `configure-pages`/`upload-pages-artifact`/`deploy-pages` shape | A custom deploy script/action | This is config, not business logic — forcing an abstraction onto a ~20-line YAML job that already has one canonical, documented shape would violate "don't force patterns where a plain script/config is the honest answer" |
| Landing page content assembly (hero, `FeatureCard` grid, install CTA) | Plain typed data file + static Astro components — no GoF pattern forced | Type-driven design (plain value objects only) | Strategy/Factory pattern for "varying feature card rendering" | The underlying problem (render N similarly-shaped cards from a typed array) has no real extensibility axis recurring in this project — forcing Strategy/Factory here would be pattern-for-pattern's-sake; a plain `FeatureCard[]` rendered by one Astro component is the honest, sufficient answer |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| Repo architecture / hotspots | None identified | N/A | `research/architecture.md` confirms this is a brand-new subsystem with no legacy code to assess; no hotspot/complexity×churn signal applies. |
| `docs/features.md` demo imagery | Line 117 embeds `../assets/demo-recording/09-unfinished.png`, which does not exist on disk — confirmed via `ls assets/demo-recording/` (only `01-dashboard.png` through `08-final.png` exist; verified independently in this plan's own research pass). `docs/features.md` has 18 `## ` sections total (`grep -c '^## ' docs/features.md`), not 17 — the entire "Unfinished Work" section (no cheap Playwright capture flow produces `09-unfinished.png`) is deleted rather than patched, which is why every section-count AC below is written against 17 sections (post-deletion), not 18. | Fix now, as Task 2.2.2a, before this file is curated into the site (see Story 2.2.2) — delete the entire "Unfinished Work" section (not just its broken `<img>` tag), since its content predates the current demo-recording fixtures and no replacement screenshot flow exists. | Pre-existing bug, already identified by `research/features.md` §4; per this repo's standing "fix collateral debt found while working" convention, it blocks `docs/features.md` from being cleanly curated into `CuratedDoc` form otherwise — a broken `<img>` would ship straight to the public site. |

---

## Observability Plan
- **Logs**: GitHub Actions run logs for `PagesDeployWorkflow` and `WebsiteDemoWorkflow` — both already have pass/fail status visible as standard Actions run history; no new log pipeline needed since this is a static site with no runtime process.
- **Metrics**: (1) `PagesDeployWorkflow`'s own pass/fail, surfaced as a repo badge in `README.md` alongside the existing `build.yml` CI badge; (2) GitHub's built-in repo Insights → Traffic for visit counts, per `requirements.md`'s Observability Requirements — no new metrics infrastructure.
- **Alerts**: None beyond GitHub Actions' default run-failure notification (email/whatever the account's existing notification settings already do) — consistent with `requirements.md`'s "no new metrics/alerting infra" framing for this complexity level.

## Risk Control
- **Feature flag**: None needed — static content only, no runtime behavior toggle (`requirements.md` Risk Control).
- **Rollback procedure**: Re-run `PagesDeployWorkflow` against the previous `main` commit SHA (`gh workflow run deploy-website.yml --ref <previous-sha>`), or revert the merge commit that introduced a bad change and let the next push redeploy.
- **Staged rollout**: None — direct deploy on push to `main`, consistent with the "low risk, static content, no user data" framing in `requirements.md`.

## Unresolved Questions
- [ ] Exact title-card copy/duration for the demo-recording branding overlay (what text, how long on screen) — blocks Task 5.2.1b (title-card filter-graph stage) — owner: Tyler (names the actual tagline/branding text; a placeholder using the README headline is the fallback if not resolved before that task starts).
- [ ] Whether the install CTA should mention Windows/WSL at all, given `install.sh`'s shell-detection only branches on zsh/bash/fish/ash (no `cmd`/PowerShell path confirmed) — blocks Task 3.1.1b (install CTA copy) — owner: Tyler (confirm whether to omit Windows/WSL entirely or add an explicit "not yet supported" note, since `research/features.md` §3 flags this as currently unconfirmed either way).
- [ ] Whether the first live `PagesDeployWorkflow` run will in fact succeed given `build_type: workflow`'s unusual `status: null` history, or whether a one-time manual Settings → Pages → Source confirmation is still needed despite the API already reading `workflow` — blocks Story 1.1.2's acceptance — owner: whoever runs the first deploy (self-resolving: the spike task itself is the resolution mechanism, with the manual UI check as the documented fallback per `research/architecture.md` §4).

## Dependency Visualization

```
Phase 1: Deploy Spike & Scaffold  (riskiest work — gates everything else)
   [Epic 1.1: Scaffold + live Pages deploy proof]
                |
                |-------------------------------------------.
                v                                            v
Phase 2: Content & Media Mechanisms           Phase 5: Demo Recording Pipeline
   [Epic 2.1: docs-curation sync]                 [Epic 5.1: WebsiteDemoWorkflow]
   [Epic 2.2: demo-asset reference + fix 09-*.png] [Epic 5.2: extend post-process.sh]
                |                                            |
       .--------+--------.                                   |
       v                 v                                   |
Phase 3: Landing Page   Phase 4: Help Docs Site               |
   [Epic 3.1 Hero/CTA]     [Epic 4.1 Getting Started]         |
   [Epic 3.2 Feature grid] [Epic 4.2 Feature overview]        |
   [Epic 3.3 Demo embeds]  [Epic 4.3 Troubleshooting]         |
                            [Epic 4.4 Nav/search/404]         |
       '--------+--------'                                   |
                v                                            |
       Phase 6: Hardening & Ship  <----------------------------'
          [Epic 6.1 Lighthouse CI] [Epic 6.2 a11y gate] [Epic 6.3 deploy hardening]
```

Phase 5 only depends on Phase 1's confirmed deploy mechanics (so new `DemoClip` names land on
a site that can already deploy) — it does not depend on Phase 2's docs-sync mechanism and can
run fully in parallel with Phases 2–4. Phase 6 is the final gate: it needs Phase 3 and Phase 4
content in place (to Lighthouse/axe-scan real pages) and Phase 5's clips published (to verify
`AssetHealthCheck` against real URLs).

---

## Phase 1: Deploy Spike & Scaffold

### Epic 1.1: Astro + Starlight scaffold with a proven, live GitHub Pages deploy

**Goal**: De-risk the single biggest "works locally, breaks live" failure class
(`BasePath`/subpath misconfiguration, and the long-broken Pages config) *before* any content
work is built on top of it — per `research/pitfalls.md`'s top-risk ranking.

#### Story 1.1.1: Scaffold the Astro + Starlight project
**As a** maintainer, **I want** a working `website/` Astro+Starlight project with the correct
subpath config, **so that** every later content task has a real project to add files to.
**Acceptance Criteria**:
- A new standalone package `website/` exists with its own `package.json`, pnpm lockfile, and
  `astro.config.mjs` setting `site: 'https://tstapler.github.io'` and `base: '/stapler-squad'`.
  - *Given* a fresh clone of the repo, *When* `cd website && pnpm install && pnpm run build`
    runs, *Then* it produces `website/dist/index.html` with no error, and
    `website/astro.config.mjs` contains `base: '/stapler-squad'`.
- The Starlight integration is configured with a placeholder `HelpSection` sidebar group
  structure (Getting Started / Features / Troubleshooting) so later content tasks only add
  files, not wire up navigation.
  - *Given* the scaffolded `website/astro.config.mjs`, *When* `pnpm run dev` starts, *Then*
    `http://localhost:4321/stapler-squad/` renders a sidebar with three empty/placeholder
    section headers matching the `HelpSection` grouping.
**Files**: `website/package.json`, `website/astro.config.mjs`, `website/src/content/docs/`, `website/src/content.config.ts`

##### Task 1.1.1a: Scaffold package + Starlight config (S)
- Run `pnpm create astro@latest website -- --template starlight`, adjust `package.json` name/version, set `site`/`base` in `astro.config.mjs`.
- Configure the `starlight()` integration's `sidebar` option with the three placeholder groups.
- Files: `website/package.json`, `website/astro.config.mjs`, `website/src/content.config.ts`

#### Story 1.1.2: Prove the GitHub Pages deploy end-to-end against the live URL
**As a** maintainer, **I want** the new deploy workflow verified against the real deployed
`https://tstapler.github.io/stapler-squad/` URL (not just local dev), **so that** the subpath
and Pages-config risks are retired before any content work depends on them.
**Acceptance Criteria**:
- `.github/workflows/deploy-website.yml` builds `website/` and deploys it via
  `actions/configure-pages@v6`, `actions/upload-pages-artifact@v5`,
  `actions/deploy-pages@v5` (pinned exact versions, per `research/stack.md` §3 and this repo's
  `gha-pin-actions` convention), triggered on push to `main`, with job permissions
  `contents: read`, `pages: write`, `id-token: write`.
  - *Given* a commit merged to `main` that only touches `website/`, *When*
    `deploy-website.yml` runs, *Then* the `deploy` job's `steps.deployment.outputs.page_url`
    output equals `https://tstapler.github.io/stapler-squad/`.
- The deployed site is verified live, not just in CI logs.
  - *Given* a successful `deploy-website.yml` run, *When* `curl -s -o /dev/null -w "%{http_code}" https://tstapler.github.io/stapler-squad/` is run, *Then* it returns `200` (not the `404` recorded in `requirements.md`'s Baseline).
  - *Given* the deployed page, *When* its rendered HTML is inspected for the Starlight-generated stylesheet `<link>` tag, *Then* its `href` starts with `/stapler-squad/` (proving `BasePath` applied to asset URLs, not just page routes — the exact failure mode `research/pitfalls.md` §2c calls out as invisible in local dev).
- The `github-pages` environment's existing branch policy (restricted to `main`, confirmed live in `research/pitfalls.md` §2a) is respected without modification.
  - *Given* the existing `deployment-branch-policies` restricting deploys to `main`, *When* `deploy-website.yml`'s `on:` block is inspected, *Then* it triggers only on `push: branches: [main]` (no PR-branch deploy attempt that would be rejected).
**Files**: `.github/workflows/deploy-website.yml`

##### Task 1.1.2a: Write and merge the deploy workflow (M)
- Author `deploy-website.yml` per the canonical shape in `research/stack.md` §3 / `research/architecture.md` §3, path-filtered to `website/**` (plus whichever `docs/**` paths Phase 2 adds), with `concurrency: {group: "pages", cancel-in-progress: false}`.
- Pin exact action versions (`actions/checkout@v6`, `actions/configure-pages@v6`, `actions/upload-pages-artifact@v5`, `actions/deploy-pages@v5`) per `gha-pin-actions`.
- Files: `.github/workflows/deploy-website.yml`

##### Task 1.1.2b: Live-verify the deployed URL and asset base path (S)
- After the workflow's first successful run on `main`, run the `curl`/HTML-inspection checks above against the real live URL (not `localhost`) and record the result.
- If the deploy fails due to the residual Pages-config risk noted in Unresolved Questions, perform the one-time manual Settings → Pages → Source check as the documented fallback.
- Files: none (verification task — update this plan's Unresolved Questions checkbox once confirmed)

#### Story 1.1.3: Confirm no separate Pages-settings change is required
**As a** maintainer, **I want** written, live confirmation that `build_type: workflow` is
already correct, **so that** the team doesn't waste time hunting for a settings bug that
doesn't exist.
**Acceptance Criteria**:
- The live Pages API state is re-confirmed immediately before merging Story 1.1.2's workflow.
  - *Given* `gh api repos/tstapler/stapler-squad/pages`, *When* run again at implementation
    time, *Then* `build_type` still reads `"workflow"` and `status` is `null` (pre-deploy
    baseline unchanged from `research/stack.md`'s §1 snapshot), confirming no drift since
    research.
**Files**: none (verification task)

##### Task 1.1.3a: Re-run the live Pages API check (XS)
- `gh api repos/tstapler/stapler-squad/pages`, diff against `research/stack.md` §1's recorded snapshot.
- Files: none

---

## Phase 2: Content & Media Mechanisms

### Epic 2.1: Docs-curation sync mechanism

**Goal**: Build the `ContentManifest` + `sync-content` script (ADR-003) once, so every later
content story is "add a manifest entry and run the script," not bespoke copy-paste.

#### Story 2.1.1: Build the ContentManifest schema and sync-content script
**As a** maintainer, **I want** a script that turns a reviewed manifest entry into a
transformed, committed `CuratedDoc`, **so that** curation is auditable and drift-safe (ADR-003).
**Acceptance Criteria**:
- `website/scripts/content-manifest.json` holds an array of `IncludeDecision`-shaped entries.
  - *Given* the entry `{"source": "docs/how-to/debug-with-logs.md", "include": true, "slug": "help/debug-with-logs"}`, *When* `node website/scripts/sync-content.mjs` runs, *Then* `website/src/content/docs/help/debug-with-logs.md` is created with frontmatter `title: Debug with Logs` (derived from the source's first `# ` heading) and `description` populated.
- The script fails loudly on a missing source.
  - *Given* a manifest entry `{"source": "docs/how-to/does-not-exist.md", "include": true, "slug": "help/x"}`, *When* `sync-content.mjs` runs, *Then* it exits non-zero and prints the missing path, rather than silently skipping it.
- Internal-only cross-references are stripped, not carried over.
  - *Given* `docs/how-to/debug-with-logs.md`'s source content contains a link to
    `../../project_plans/...` (an out-of-scope path per `requirements.md` Out of Scope), *When*
    `sync-content.mjs` transforms it, *Then* the resulting `CuratedDoc` contains no
    `project_plans/` or `CLAUDE.md` link — either the link is removed or its surrounding
    sentence is rewritten to not reference it.
**Files**: `website/scripts/content-manifest.json`, `website/scripts/sync-content.mjs`, `website/scripts/transform-markdown.mjs`

##### Task 2.1.1a: Manifest schema + IncludeDecision validation (S)
- Define the `ContentManifest` JSON shape and a small loader that validates each entry (Parse-Don't-Validate: reject malformed entries at load time, not deep in the transform pass).
- Files: `website/scripts/content-manifest.json`, `website/scripts/sync-content.mjs`

##### Task 2.1.1b: Markdown transform pass (frontmatter + link stripping) (M)
- Implement the H1-derived title extraction, frontmatter injection, and a link-rewrite pass that detects and removes/flags links into out-of-scope paths (`project_plans/`, `CLAUDE.md`, `docs/explanation/concurrency-patterns.md` if dev-only, etc.).
- Files: `website/scripts/transform-markdown.mjs`

#### Story 2.1.2: Curate the file-by-file include/exclude list and run the first sync
**As a** maintainer, **I want** every `docs/how-to/*.md` file explicitly classified, **so
that** the "curated subset" judgment call (`requirements.md` Rabbit Holes) is resolved once,
in writing, rather than left to each later task's discretion.
**Acceptance Criteria**:
- All 23 files in `docs/how-to/` have an explicit manifest entry (include or exclude with
  reason). Resolved classification (recorded as the manifest's seed data):
  - **Include** (11, user-facing): `add-user-themes.md`, `debug-with-logs.md`,
    `dispatch-work-to-google-jules.md`, `enable-pi-support.md`,
    `expose-github-webhook-endpoint.md`, `expose-slack-interactive-endpoint.md`,
    `macos-codesigning.md`, `manage-notifications.md`, `manage-systemd-service.md`,
    `monitor-external-terminal-sessions.md`, `use-up-next-pr-tabs-and-ask-to-fix.md`.
  - **Exclude** (12, dev/maintainer-only): `bundle-tmux.md`, `capture-terminal-stream-tap.md`
    (security-sensitive — "records secrets"), `commit-sdd-planning-artifacts.md`,
    `enable-opentelemetry.md`, `enable-otel-auto-instrumentation.md`,
    `fix-playwright-chromium-install-stall.md`, `flip-git-backend-cohort.md`,
    `merge-prs-with-gh-cli.md`, `profile-lockups.md`, `roll-back-go-git-fork.md`,
    `run-nil-safety-analysis.md` (requirements.md's own flagged example), `use-pnpm-in-web-app.md`.
  - *Given* the manifest entry for `run-nil-safety-analysis.md` (`{"include": false, "reason": "NilAway/go vet -nilness — dev-contributor-only tooling, no end-user relevance"}`), *When* `sync-content.mjs` runs, *Then* no `CuratedDoc` is produced for it and the public site's build output contains no reference to it.
  - *Given* the manifest entry for `debug-with-logs.md` (`{"include": true, ...}`), *When* `sync-content.mjs` runs, *Then* `website/src/content/docs/help/debug-with-logs.md` exists in the committed output.
**Files**: `website/scripts/content-manifest.json`

##### Task 2.1.2a: Classify all 23 how-to files + run first sync (S)
- Populate every entry in `website/scripts/content-manifest.json` per the classification above, run `sync-content.mjs`, commit the resulting `CuratedDoc`s.
- Files: `website/scripts/content-manifest.json`, `website/src/content/docs/help/*.md`

### Epic 2.2: Demo-media reference mechanism

**Goal**: Build the `ReleaseAssetRef` helper + `AssetHealthCheck` (ADR-002), and fix the
pre-existing broken `09-unfinished.png` reference before `docs/features.md` content is curated.

#### Story 2.2.1: Build the ReleaseAssetRef helper and AssetHealthCheck
**As a** maintainer, **I want** one typed place that constructs demo-asset URLs and a
build-time check that they resolve *and* that their content matches what was actually
published, **so that** a renamed/missing clip, or a `--clobber` re-publish that silently
served stale/corrupted bytes at an otherwise-valid URL, fails the build loudly instead of
shipping a broken or wrong demo embed (ADR-002; pre-mortem.md Failure #5, P1).
**Acceptance Criteria**:
- `website/src/lib/release-asset.ts` exports `releaseAssetUrl({ tag, name, ext })`.
  - *Given* `releaseAssetUrl({ tag: "website-demo-gifs-latest", name: "review-queue", ext: "webm" })`, *When* called, *Then* it returns exactly `https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/review-queue.webm`.
- `website/scripts/check-asset-health.mjs` HEAD-checks every URL produced by the helper across all site content.
  - *Given* a site page embedding `releaseAssetUrl({ tag: "website-demo-gifs-latest", name: "does-not-exist-yet", ext: "gif" })`, *When* `node website/scripts/check-asset-health.mjs` runs before a clip named `does-not-exist-yet.gif` has ever been published, *Then* it exits non-zero and prints the failing URL, rather than letting the deploy proceed with a silently-broken embed.
- Every `ReleaseAssetRef` the health check HEAD-checks also has its live content hash compared against `website/scripts/asset-health-manifest.json`'s (`AssetHealthManifest`) recorded SHA256 for that `name.ext`, and a mismatch fails the build even though the HTTP status is 2xx/3xx.
  - *Given* `asset-health-manifest.json` records `"review-queue.webm": "a3f29e7c1b6d4598e0f2a7c9b4d61e8f3a0c7b2d5e9f4a1c8b6d3e0f7a2c9b5d"` (the SHA256 recorded when `website-demo-publish.yml` last published that clip), *When* `review-queue.webm` is later re-published by a `--clobber` run with different content and the live asset's computed SHA256 is `f1c8a94e2b7d3056c9e1a4f8b2d6c3e07a9f5b1d4e8c2a6f0b3d9e5c1a7f4b82` (≠ the manifest's recorded hash), *Then* `node website/scripts/check-asset-health.mjs` exits non-zero and prints both the expected and actual hashes for `review-queue.webm`, flagging the drift a status-code-only check would have reported as passing.
**Files**: `website/src/lib/release-asset.ts`, `website/scripts/check-asset-health.mjs`, `website/scripts/asset-health-manifest.json`

##### Task 2.2.1a: ReleaseAssetRef helper (S)
- Implement the typed constructor + a short extraction utility that scans built `website/dist/**/*.html` for `releases/download/` URLs (so the health check doesn't need a second, hand-maintained list of "which clips are referenced").
- Files: `website/src/lib/release-asset.ts`

##### Task 2.2.1b: AssetHealthCheck script + wire into deploy workflow (S)
- Implement the HEAD-request loop; add it as a step in `deploy-website.yml` between build and `upload-pages-artifact` (fail the deploy, don't just warn). Extend the check beyond status code: for each `ReleaseAssetRef` that HEAD-checks clean, GET the live asset, compute its SHA256, and look up the expected hash for that `name.ext` in `website/scripts/asset-health-manifest.json` (`AssetHealthManifest`) — fail the build on a hash mismatch, not just a non-2xx/3xx status, and print both hashes in the failure message. A `name.ext` with no manifest entry yet (not-yet-published clip) is reported by the existing HEAD-check path, not the hash path.
- Files: `website/scripts/check-asset-health.mjs`, `.github/workflows/deploy-website.yml`, `website/scripts/asset-health-manifest.json`

#### Story 2.2.2: Delete the "Unfinished Work" section and its broken `09-unfinished.png` reference
**As a** maintainer, **I want** `docs/features.md`'s dangling image reference fixed, **so
that** curating this file into the site doesn't ship an already-broken image (Tech Debt
Disposition row above).
**Acceptance Criteria**:
- `docs/features.md` no longer references a nonexistent file, and its section count drops from 18 to 17.
  - *Given* `docs/features.md` line 117's current `![Unfinished Work](../assets/demo-recording/09-unfinished.png)`, with no matching Playwright capture flow available to regenerate it, *When* fixed, *Then* the entire "Unfinished Work" `## ` section (heading, prose, and broken image tag) is deleted from `docs/features.md` — *Then* `grep -c '^## ' docs/features.md` reports 17, and `ls assets/demo-recording/*.png | wc -l` (8) matches the count of image references actually used across the remaining sections.
**Files**: `docs/features.md`

##### Task 2.2.2a: Delete the entire "Unfinished Work" section (XS)
- Delete the whole `## Unfinished Work` section — heading, body, and the broken `09-unfinished.png` image tag — not just the `<img>`/Markdown tag on its own; no cheap Playwright capture flow exists to regenerate `09-unfinished.png`, and leaving a text-only stub behind would misstate the section's status. This drops `docs/features.md` from 18 `## ` sections to 17, which every downstream section/screenshot count in this plan (Stories 3.2.1, 3.2.2, 3.3.1, 4.2.1) is written against.
- Files: `docs/features.md`

---

## Phase 3: Landing Page

### Epic 3.1: Hero & install CTA

#### Story 3.1.1: Hero section with headline, value prop, and copyable install command
**As a** prospective adopter, **I want** to understand what Stapler Squad is and how to
install it within 5 seconds of landing, **so that** I don't bounce before reaching the demo
(per `research/ux.md` §2's "answer in <5 seconds" finding).
**Acceptance Criteria**:
- The hero reuses the README's value prop verbatim, not reworded copy.
  - *Given* `README.md:3`'s text ("web-based mission control for running multiple AI coding
    agents... with a real-time dashboard, automatic approval rules, and a structured review
    queue"), *When* the `SplashPage` renders, *Then* its hero tagline contains that same
    sentence (verbatim or near-verbatim per `research/ux.md`'s explicit "don't invent new copy,
    lift it" instruction).
  - *Given* the rendered hero, *When* a user clicks the copy-icon next to the install command block, *Then* the clipboard contains the exact text `brew tap tstapler/stapler-squad https://github.com/tstapler/stapler-squad && brew install tstapler/stapler-squad/stapler-squad` (README.md:50's own install command, reused verbatim per `research/ux.md` §1).
**Files**: `website/src/content/docs/index.mdx`, `website/src/components/InstallCommand.astro`

##### Task 3.1.1a: SplashPage hero markup + copy-to-clipboard install block (S)
- Author the `index.mdx` splash frontmatter + hero content; build a small `InstallCommand.astro` component with a copy button.
- Files: `website/src/content/docs/index.mdx`, `website/src/components/InstallCommand.astro`

##### Task 3.1.1b: Install CTA copy, OS-coverage note (XS — blocked on Unresolved Question re: Windows/WSL)
- Write the secondary CTA ("View on GitHub") and an OS-coverage line (macOS/Linux via Homebrew tap + binary); resolve the Windows/WSL wording per the Unresolved Questions entry before finalizing.
- Files: `website/src/content/docs/index.mdx`

### Epic 3.2: Feature highlights grid

#### Story 3.2.1: Build the FeatureCard data set from README + docs/features.md
**As a** prospective adopter, **I want** a feature grid with real descriptions and screenshots,
**so that** I can tell "is this for me" without reading the full README (per `research/ux.md`
§1's k9s/Posting/Starship comparison).
**Acceptance Criteria**:
- The grid reconciles README's 5-category grouping with `docs/features.md`'s 17-section detail
  into one structure (per `research/features.md` §4's explicit call-out that these are "two
  different taxonomies of the same feature set").
  - *Given* README's "Review & Approval" category (Auto-approval rules engine, Review Queue,
    Approval analytics) and `docs/features.md`'s matching sections, *When* `website/src/content/features.json`
    is built, *Then* it contains three `FeatureCard` entries tagged `category: "Review & Approval"`,
    each with a `description` sourced from `docs/features.md`'s longer-form prose (not just README's one-liner).
  - *Given* the 8 `docs/features.md` sections with an existing screenshot (Dashboard,
    Instant Search, Status Filtering, Tag-Based Grouping, Review Queue, Auto-Approval Rules,
    Custom Rules, Isolated Git Workspaces — "Unfinished Work" was deleted entirely by Task
    2.2.2a, so it contributes no entry), *When* the data set is built,
    *Then* each of those 8 `FeatureCard` entries has a `screenshot` field pointing at the
    corresponding `assets/demo-recording/0N-*.png`, copied into `website/public/features/`.
**Files**: `website/src/content/features.json`, `website/public/features/*.png`

##### Task 3.2.1a: Build features.json reconciling both taxonomies (M)
- Walk README's Highlights + `docs/features.md`'s 17 sections (post-Task-2.2.2a), emit one typed `FeatureCard[]` array; copy the 8 existing screenshots into `website/public/features/`.
- Files: `website/src/content/features.json`, `website/public/features/*.png`

#### Story 3.2.2: Render the feature grid component
**As a** prospective adopter, **I want** a responsive, accessible feature grid, **so that**
it reads well on both mobile and desktop (per the standing mobile+desktop UX instinct).
**Acceptance Criteria**:
- The grid renders without horizontal scroll at a phone viewport width.
  - *Given* `website/src/content/features.json`'s 17 `FeatureCard` entries, *When* the splash
    page renders at a 375px viewport width (iPhone SE class), *Then* the grid lays out as a
    single column with no horizontal scrollbar.
- Every screenshot has real alt text, not a filename.
  - *Given* the Dashboard `FeatureCard`'s `screenshot: "/features/01-dashboard.png"`, *When*
    rendered, *Then* its `<img>` tag's `alt` attribute reads a sentence like "Dashboard showing
    status badges, diff stats, tags, and approvals for all agents at a glance" (reusing
    `docs/features.md`'s own prose per `research/ux.md` §3), not `01-dashboard.png` or empty.
**Files**: `website/src/components/FeatureGrid.astro`, `website/src/components/FeatureCard.astro`

##### Task 3.2.2a: FeatureGrid/FeatureCard components with alt text (S)
- Build the responsive CSS grid and per-card alt-text wiring from `features.json`.
- Files: `website/src/components/FeatureGrid.astro`, `website/src/components/FeatureCard.astro`

### Epic 3.3: Demo media embeds

#### Story 3.3.1: Embed hero demo video and feature-card clips with safe fallbacks
**As a** prospective adopter, **I want** to see the product in motion above the fold, **so
that** I don't have to scroll past a "Features" wall of text to know what it looks like
(per `research/features.md` §3 and `research/ux.md` §5's "social" JTBD).
**Acceptance Criteria**:
- The hero embeds the existing small committed `assets/demo.webm` (not a Release asset — it's
  already <2MB and exempt per `requirements.md` Constraints), with a `poster` fallback.
  - *Given* `website/public/demo.webm` (copied from the repo's `assets/demo.webm`), *When* the
    hero renders, *Then* the `<video>` tag has `autoplay loop muted controls poster="/stapler-squad/screenshot.png"` and respects `prefers-reduced-motion` (no `autoplay` attribute applied when that media query matches, verified via a small client-side check).
- Feature-grid clips (if any beyond the 8 static screenshots) use `ReleaseAssetRef`, not a
  committed file.
  - *Given* a feature card for "Live Terminal Streaming" (one of the 9 `docs/features.md`
    sections with no existing screenshot, per `research/features.md` §4) that embeds a new
    `DemoClip` named `terminal-streaming`, *When* rendered, *Then* its `<video>` `src` equals
    `releaseAssetUrl({ tag: "website-demo-gifs-latest", name: "terminal-streaming", ext: "webm" })`'s output, never a hardcoded literal string duplicated from that helper.
**Files**: `website/public/demo.webm`, `website/public/screenshot.png`, `website/src/components/FeatureCard.astro`

##### Task 3.3.1a: Hero video embed with reduced-motion + poster fallback (S)
- Copy `assets/demo.webm`/`assets/screenshot.png` into `website/public/`; wire the `<video>` tag and reduced-motion check.
- Files: `website/public/demo.webm`, `website/public/screenshot.png`, `website/src/content/docs/index.mdx`

---

## Phase 4: Help Docs Site

### Epic 4.1: Getting started / install docs

#### Story 4.1.1: Getting-started page adapted from README's Installation section
**As an** existing or prospective user, **I want** a getting-started page with more detail
than the landing page's CTA, **so that** install failures have somewhere to go
(per `research/ux.md` §5's functional JTBD).
**Acceptance Criteria**:
- The page covers all three install paths from `README.md`'s Installation section (Homebrew
  tap, manual `install.sh`, build-from-source), adapted for public-audience framing.
  - *Given* `website/src/content/docs/help/getting-started.md`, *When* rendered, *Then* it
    contains three subsections — "Homebrew", "Manual install script", "Build from source" —
    each with a copyable command block, matching README's three paths 1:1 (no install path
    invented that isn't in the source).
**Files**: `website/src/content/docs/help/getting-started.md`

##### Task 4.1.1a: Write getting-started.md (S)
- Adapt README's Installation section; cross-link to the troubleshooting section for build failures.
- Files: `website/src/content/docs/help/getting-started.md`

### Epic 4.2: Feature overview docs

#### Story 4.2.1: Feature-overview docs page built from the curated FeatureCard data
**As an** existing user exploring capabilities, **I want** a docs-site feature overview (not
just the landing-page teaser grid), **so that** I can read full descriptions without
scrolling a marketing page.
**Acceptance Criteria**:
- The page reuses `website/src/content/features.json` (Story 3.2.1's output) rather than
  duplicating feature copy a second time.
  - *Given* `features.json`'s 17 `FeatureCard` entries, *When* `website/src/content/docs/help/features.md` (or an `.astro`/`.mdx` page importing the same data file) renders, *Then* all 17 entries appear, including the 9 that have no screenshot (rendered as text-only cards, per `research/features.md` §4's finding that just over half the entries are text-only).
**Files**: `website/src/content/docs/help/features.mdx`

##### Task 4.2.1a: Build features.mdx importing the shared FeatureCard data (M)
- Import `features.json`; render full-length descriptions (not the grid's shortened version).
- Files: `website/src/content/docs/help/features.mdx`

### Epic 4.3: Troubleshooting docs (curated how-to subset)

#### Story 4.3.1: Publish the 11 included how-to CuratedDocs under Troubleshooting
**As an** existing user stuck on a specific problem, **I want** the 11 curated how-to pages
reachable within 2 clicks from any docs page, **so that** I can self-serve instead of filing
an issue (per `requirements.md` Success Metrics and `research/ux.md` §2).
**Acceptance Criteria**:
- All 11 `include: true` manifest entries from Story 2.1.2 render under a persistent
  "Troubleshooting" sidebar group.
  - *Given* the `HelpSection` sidebar config, *When* any `help/*` page renders, *Then* the
    "Troubleshooting" group (containing `debug-with-logs`, `manage-notifications`,
    `manage-systemd-service`, etc. — all 11) is visible in the sidebar without needing to
    navigate back to the docs home first.
**Files**: `website/astro.config.mjs` (sidebar config), `website/src/content/docs/help/*.md` (already produced by Story 2.1.2)

##### Task 4.3.1a: Wire the 11 CuratedDocs into the sidebar's Troubleshooting group (S)
- Update `astro.config.mjs`'s `starlight({ sidebar: [...] })` to list the 11 slugs under "Troubleshooting".
- Files: `website/astro.config.mjs`

### Epic 4.4: Search, nav, and 404 handling

#### Story 4.4.1: Pagefind search and BrokenLinkCheck
**As an** existing user, **I want** full-text search across all docs pages, and confidence
that no curated link is dead, **so that** the site doesn't "read as neglected"
(per `research/ux.md` §5's emotional JTBD: "a site with broken search reads as neglected").
**Acceptance Criteria**:
- Search is available with no external API key/service (Starlight's built-in Pagefind).
  - *Given* the built site, *When* a user presses `Cmd/Ctrl+K` and types `"tmux session"`,
    *Then* a results list appears client-side with no network request to a third-party search
    API (verified via the browser network panel showing only same-origin Pagefind index requests).
  - *Given* a search query with no matches (e.g. `"zzqqxyz"`), *When* submitted, *Then* a
    "No results for 'zzqqxyz'" message with a link to the docs home renders — never a blank panel (per `research/ux.md` §4).
- `BrokenLinkCheck` catches a stale internal link before deploy.
  - *Given* a `CuratedDoc` containing a link to `help/does-not-exist`, *When*
    `node website/scripts/check-internal-links.mjs` runs, *Then* it exits non-zero and names
    the broken link and its source file.
**Files**: `website/astro.config.mjs` (Pagefind is built-in to Starlight, minimal config), `website/scripts/check-internal-links.mjs`

##### Task 4.4.1a: Confirm Pagefind defaults + no-results UI copy (XS)
- Verify Starlight's default Pagefind integration needs no extra config; add the no-results slot copy if the default is a blank panel.
- Files: `website/astro.config.mjs`

##### Task 4.4.1b: BrokenLinkCheck script, wired into deploy workflow (S)
- Walk all `CuratedDoc` + landing-page internal links, validate each resolves to a real Starlight route; add as a `deploy-website.yml` step (fail, don't warn).
- Files: `website/scripts/check-internal-links.mjs`, `.github/workflows/deploy-website.yml`

#### Story 4.4.2: Branded 404 page
**As a** user following a stale/mistyped link, **I want** a 404 page matching the site's own
chrome, **so that** the dead-end doesn't look broken or abandoned.
**Acceptance Criteria**:
- A custom 404 page replaces GitHub Pages' generic default.
  - *Given* a request to `https://tstapler.github.io/stapler-squad/does-not-exist`, *When*
    loaded, *Then* the response renders the site's own header/nav/branding with a "Page not
    found" message and a link back to the docs home — not GitHub's default 404 HTML.
**Files**: `website/src/pages/404.astro`

##### Task 4.4.2a: Build 404.astro (XS)
- Files: `website/src/pages/404.astro`

---

## Phase 5: Automated Demo Recording Pipeline

### Epic 5.1: WebsiteDemoWorkflow (new, separate recording workflow)

#### Story 5.1.1: New workflow_dispatch-only recording workflow publishing to a new release tag
**As a** maintainer, **I want** a new, separate workflow that records `FeatureSpec`s and
publishes to `website-demo-gifs-latest`, **so that** the site's higher-production clips never
collide with or modify the existing `demo-gifs-latest` release `demo-publish.yml` owns
(per `research/architecture.md` §2).
**Acceptance Criteria**:
- The workflow mirrors `demo-publish.yml`'s trigger discipline exactly.
  - *Given* `.github/workflows/website-demo-publish.yml`'s `on:` block, *When* inspected,
    *Then* it reads `on: workflow_dispatch:` only — no `push`, `pull_request`, or `schedule`/`cron` trigger — matching `demo-publish.yml`'s pattern per `research/pitfalls.md` §4's "near-mechanical review check."
  - *Given* the workflow's `env.FEATURE_SPECS`, *When* inspected, *Then* it contains exactly
    the same five specs as `demo-publish.yml`'s `FEATURE_SPECS` (`demo.spec.ts`,
    `review-queue.spec.ts`, `session-lifecycle.spec.ts`, `history-search.spec.ts`,
    `workspace-management.spec.ts`) — no new, unvetted spec added.
- Published assets land under the new tag, never the existing one.
  - *Given* a manual `gh workflow run website-demo-publish.yml`, *When* it completes, *Then*
    `gh release view website-demo-gifs-latest` shows the newly uploaded clips, and
    `gh release view demo-gifs-latest` is unchanged (its asset list/timestamps are identical
    to before the run).
- Each publish updates the `AssetHealthManifest` to match what was actually uploaded (pre-mortem.md Failure #5).
  - *Given* a successful `gh release upload --clobber` of `review-queue.webm` within the run, *When* the workflow's manifest-update step runs immediately after, *Then* `website/scripts/asset-health-manifest.json`'s `"review-queue.webm"` entry is set to the SHA256 of the exact bytes just uploaded, and that change is committed to `main` (same `git config`/`git commit -m "chore(...): ..." [skip ci]`/`git push` pattern as `.github/workflows/ci-duration-trend.yml`) so the manifest is never stale relative to the release it describes.
**Files**: `.github/workflows/website-demo-publish.yml`, `website/scripts/asset-health-manifest.json`

##### Task 5.1.1a: Author website-demo-publish.yml from demo-publish.yml's shape (M)
- Copy `demo-publish.yml`'s two-job (`record-demos`/`publish-gifs`) structure, retarget the release tag to `website-demo-gifs-latest`, keep the explanatory header comment pattern (why dispatch-only) per `research/pitfalls.md` §4.
- Files: `.github/workflows/website-demo-publish.yml`

##### Task 5.1.1b: Write the published hash into AssetHealthManifest (XS)
- In the `publish-gifs` job, after each `gh release upload --clobber` succeeds, compute the uploaded file's SHA256 (`sha256sum`) and update/insert its `name.ext` → hash entry in `website/scripts/asset-health-manifest.json`; commit and push that file using `ci-duration-trend.yml`'s bot-commit pattern (`git config user.{name,email}`, `git add`, commit with `[skip ci]`, `git pull --rebase --autostash && git push` retry loop) so a concurrent push doesn't lose the update.
- Files: `.github/workflows/website-demo-publish.yml`, `website/scripts/asset-health-manifest.json`

#### Story 5.1.2: Verify the new tag's stable URL convention end-to-end
**As a** maintainer, **I want** confirmation that a published clip's URL matches
`ReleaseAssetRef`'s expected shape *and* that its recorded hash matches what's live, **so
that** Phase 3/4's embeds and `AssetHealthCheck` (Epic 2.2) actually work against real data,
including the content-identity check added for pre-mortem.md Failure #5.
**Acceptance Criteria**:
- A real published clip resolves at the exact URL the site's helper constructs.
  - *Given* a successful `website-demo-publish.yml` run producing `review-queue.webm`, *When*
    `curl -sI https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/review-queue.webm` is run, *Then* it returns a `302` (GitHub's release-asset redirect) rather than `404`, and `website/scripts/check-asset-health.mjs` passes for that URL, including its hash comparison against `asset-health-manifest.json`.
**Files**: none (verification task)

##### Task 5.1.2a: Run and verify against the live release (XS)
- Files: none

### Epic 5.2: Extend demo-post-process.sh with branding/pacing

#### Story 5.2.1: Add a title-card/branding filter-graph stage
**As a** prospective adopter, **I want** the new demo clips to carry consistent branding (not
just the existing browser-chrome frame), **so that** "higher production value than
demo-publish.yml" is a real, visible difference (per `requirements.md` Scope).
**Acceptance Criteria**:
- `scripts/demo-post-process.sh` gains an optional title-card stage, not a parallel duplicate script.
  - *Given* `./scripts/demo-post-process.sh --title-card "Review Queue" tests/e2e/test-results/review-queue-chromium/video.webm`, *When* run, *Then* the output GIF/webm's first ~1.5s shows a static branded title-card frame (logo + the given text) before the captured footage begins, implemented as an additional `drawtext`/`overlay` ffmpeg filter-graph stage layered onto the script's existing `drawbox`-based browser-chrome stage — not a new script.
**Files**: `scripts/demo-post-process.sh`

##### Task 5.2.1a: Add --title-card flag + drawtext/overlay filter stage (M)
- Rasterize `web-app/public/logo.svg` once at build/CI time (ffmpeg can't composite SVG directly) for the overlay image; add the new filter-graph stage ahead of the existing browser-chrome pad.
- Files: `scripts/demo-post-process.sh`

##### Task 5.2.1b: Resolve title-card copy per clip (XS — blocked on Unresolved Question)
- Apply the actual per-clip title text once Tyler confirms branding/tagline wording (Unresolved Questions above); fall back to the clip's `FeatureSpec` name capitalized if unresolved by the time this task runs.
- Files: `.github/workflows/website-demo-publish.yml` (per-clip `--title-card` argument)

#### Story 5.2.2: Non-timing-dependent framing/pacing improvements
**As a** maintainer, **I want** pacing/framing improvements that don't depend on CI-runner
speed, **so that** output doesn't visually vary between a fast and slow runner
(per `research/pitfalls.md` §4's "timing-dependent framing/pacing risk").
**Acceptance Criteria**:
- Any speed/pacing adjustment uses frame-count-relative filters, not wall-clock sleeps.
  - *Given* a captured webm with dead air at the start (e.g. a page-load wait), *When*
    `demo-post-process.sh` applies its pacing stage, *Then* it uses `setpts`/frame-trimming
    keyed to detected scene changes or fixed frame offsets, never a `sleep`-based or
    real-time-elapsed-based trigger — so the same input produces byte-for-byte-equivalent
    pacing regardless of which CI runner captured it.
**Files**: `scripts/demo-post-process.sh`

##### Task 5.2.2a: Add frame-relative pacing/crop filter stage (S)
- Files: `scripts/demo-post-process.sh`

---

## Phase 6: Hardening, Accessibility, and Ship

### Epic 6.1: Lighthouse CI with the correct (higher) thresholds

#### Story 6.1.1: website/lighthouserc.json with ≥90 thresholds, desktop + mobile passes
**As a** maintainer, **I want** the site's own Lighthouse CI config (not a copy of `web-app/`'s
lower, warn-only bar), **so that** the ≥90 NFR is actually enforced
(per `research/pitfalls.md` §5 and `research/ux.md` §3).
**Acceptance Criteria**:
- Thresholds are higher than and independent from `web-app/lighthouserc.json`'s.
  - *Given* `website/lighthouserc.json`, *When* inspected, *Then* `performance`,
    `accessibility`, and `best-practices` are all configured at `["error", {"minScore": 0.9}]`
    (blocking, not `"warn"`) — distinct from `web-app/lighthouserc.json`'s `0.70`/`0.80` warn-only values.
  - *Given* a CI run against the built `website/dist/`, *When* Lighthouse CI runs both a
    desktop and a mobile-emulation pass, *Then* both passes report ≥0.90 on all three
    categories, or the CI job fails.
**Files**: `website/lighthouserc.json`, `.github/workflows/deploy-website.yml` (or a separate `website-lighthouse.yml`)

##### Task 6.1.1a: Author lighthouserc.json with desktop+mobile presets (S)
- Files: `website/lighthouserc.json`

### Epic 6.2: Accessibility gate

#### Story 6.2.1: axe-core Playwright scan on the landing page and one docs page
**As a** maintainer, **I want** the same `@axe-core/playwright` library the main app's
`tests/e2e/accessibility.spec.ts` already uses, **so that** the new site doesn't introduce a
second, inconsistent a11y-tooling choice (per `research/ux.md` §3's explicit "reuse the same
library" instruction).
**Acceptance Criteria**:
- The scan blocks on critical/serious violations, matching the main app's severity gate.
  - *Given* `website/tests/a11y.spec.ts` running `@axe-core/playwright` against
    `http://localhost:4321/stapler-squad/` (landing page) and
    `http://localhost:4321/stapler-squad/help/getting-started/` (one docs page), *When* run in
    CI, *Then* it fails the build on any `critical` or `serious` violation — e.g. a contrast
    failure on the hero's CTA button text against its background — exactly mirroring
    `tests/e2e/accessibility.spec.ts`'s existing severity threshold.
- Brand-color contrast is checked before, not after, shipping (per `research/ux.md` §3's note
  that `web-app`'s theme already needed post-hoc contrast fixes twice).
  - *Given* the site's chosen CTA button color (reusing `web-app`'s `primary: #0070f3` light-mode
    token per `research/ux.md` §3), *When* placed against the splash page's background, *Then*
    the axe-core contrast check passes at ≥4.5:1 for its label text before merge, not fixed
    in a follow-up PR.
**Files**: `website/tests/a11y.spec.ts`, `website/package.json` (dev dependency)

##### Task 6.2.1a: Add @axe-core/playwright scan for landing + one docs page (S)
- Files: `website/tests/a11y.spec.ts`, `website/package.json`

### Epic 6.3: Deploy workflow hardening and final live verification

#### Story 6.3.1: Finalize deploy workflow (path filters, concurrency, asset/link checks wired) and re-verify live
**As a** maintainer, **I want** the production `deploy-website.yml` to include every check
built across Phases 2–5 (AssetHealthCheck, BrokenLinkCheck, Lighthouse, axe-core) as blocking
steps, **so that** the final shipped workflow matches what Phase 1's spike proved, plus every
later safety net.
**Acceptance Criteria**:
- All four checks run as blocking steps in the same workflow, in dependency order.
  - *Given* the finalized `deploy-website.yml`, *When* inspected, *Then* its `build` job runs,
    in order: `pnpm run sync-content` (idempotency check only — fails if running it produces
    a diff, meaning someone forgot to commit a sync), `pnpm run build`,
    `check-internal-links.mjs`, `check-asset-health.mjs`, Lighthouse CI, before
    `upload-pages-artifact`/`deploy-pages`.
- The full pipeline is re-verified against the live URL one final time with real content
  (not Phase 1's placeholder scaffold).
  - *Given* all of Phases 2–5 merged to `main`, *When* `deploy-website.yml`'s next run
    completes, *Then* `https://tstapler.github.io/stapler-squad/` returns `200`, its feature
    grid renders all 17 `FeatureCard`s, its Troubleshooting sidebar lists all 11 curated pages,
    and at least one `website-demo-gifs-latest` clip loads successfully in the browser.
**Files**: `.github/workflows/deploy-website.yml`

##### Task 6.3.1a: Wire remaining checks into the workflow + path filters (S)
- Add `paths: [website/**, docs/how-to/**, docs/features.md]` filter to the `on: push` trigger; order the blocking steps.
- Files: `.github/workflows/deploy-website.yml`

##### Task 6.3.1b: Final live verification pass (S)
- Run the full Given/When/Then checks above against the real deployed site.
- Files: none

---

## Effort estimate

Bands and weights are INFERRED per `ESTIMATION.md`; recalibrate after the first implementer run.
Output share assumed 10–15% for implementation tasks (code/config/content written), 5–10% for
verification-only tasks (mostly re-reading/checking). CU = fresh-input-equivalent tokens
(`in + 1.25·cache_write + 0.1·cache_read + 5·out`); shown here as a single estimated CU per
task using the assumed mix, since no cache-read/write split is knowable at planning time.

| Story | Tasks (class) | Est. raw tokens | Output share | Est. CU | Wall-clock blocker |
|---|---|---|---|---|---|
| 1.1.1 Scaffold | 1a (S) | 55k | 12% | ~85k | none |
| 1.1.2 Deploy spike | 2a (M), 2b (S) | 130k+55k=185k | 12% | ~290k | none |
| 1.1.3 Pages config confirm | 3a (XS) | 20k | 8% | ~27k | none |
| 2.1.1 Sync script | 1a (S), 1b (M) | 55k+130k=185k | 15% | ~310k | none |
| 2.1.2 Curation list + sync | 2a (S) | 55k | 10% | ~82k | none |
| 2.2.1 ReleaseAssetRef + health check | 1a (S), 1b (S) | 55k+55k=110k | 12% | ~175k | none |
| 2.2.2 Fix broken image ref | 2a (XS) | 20k | 10% | ~29k | none |
| 3.1.1 Hero + install CTA | 1a (S), 1b (XS) | 55k+20k=75k | 15% | ~130k | owner decision (Windows/WSL wording) |
| 3.2.1 FeatureCard data | 1a (M) | 130k | 12% | ~205k | none |
| 3.2.2 FeatureGrid component | 2a (S) | 55k | 12% | ~90k | none |
| 3.3.1 Demo media embeds | 1a (S) | 55k | 12% | ~90k | none |
| 4.1.1 Getting started docs | 1a (S) | 55k | 15% | ~96k | none |
| 4.2.1 Feature overview docs | 1a (M) | 130k | 12% | ~205k | none |
| 4.3.1 Troubleshooting sidebar | 1a (S) | 55k | 10% | ~82k | none |
| 4.4.1 Search + BrokenLinkCheck | 1a (XS), 1b (S) | 20k+55k=75k | 12% | ~120k | none |
| 4.4.2 404 page | 2a (XS) | 20k | 12% | ~29k | none |
| 5.1.1 WebsiteDemoWorkflow | 1a (M), 1b (XS) | 130k+20k=150k | 12% | ~225k | none |
| 5.1.2 Verify live tag URL | 2a (XS) | 20k | 8% | ~27k | none |
| 5.2.1 Title-card filter stage | 1a (M), 1b (XS) | 130k+20k=150k | 15% | ~255k | owner decision (title-card copy) |
| 5.2.2 Pacing filter stage | 2a (S) | 55k | 12% | ~90k | none |
| 6.1.1 Lighthouse CI config | 1a (S) | 55k | 10% | ~82k | none |
| 6.2.1 axe-core scan | 1a (S) | 55k | 12% | ~90k | none |
| 6.3.1 Final wiring + live verify | 1a (S), 1b (S) | 55k+55k=110k | 12% | ~175k | none |

**Sum of task-level estimates**: ~2.67M raw tokens, ~2.90M CU before verification multiplier
(bumped from ~2.61M/~2.84M by the pre-mortem Failure #5 fix: Task 2.2.1b's hash-comparison logic (XS→S) and new Task 5.1.1b writing the `AssetHealthManifest`).

**Verification multiplier**: ×1.75 (mid-range per ESTIMATION.md — spec-compliance + code-quality
review per story, plus a Phase 6 re-verify pass) → **~5.08M CU total**.

**Resulting size band**: **Large (3–15M CU)**, consistent with `requirements.md`'s own Size
estimate ("Large (3–15M CU)") — confirms the requirements-phase sizing, no surprise overrun at
planning time.

**Critical path in agent waves**:
1. Wave 1: Story 1.1.1 (scaffold) → Story 1.1.2 (deploy spike, gates everything) → Story 1.1.3 (confirm).
2. Wave 2 (parallel fan-out after Wave 1): Epic 2.1 + Epic 2.2 (content/media mechanisms) run concurrently with Epic 5.1 + Epic 5.2 (demo pipeline) — four independent agent runs.
3. Wave 3 (parallel, depends on Wave 2's Epic 2.1/2.2 output): Phase 3 (landing page, 5 stories) and Phase 4 (help docs, 5 stories) run concurrently.
4. Wave 4: Phase 6 (hardening) — depends on Wave 3's content existing to scan/Lighthouse-test, and Wave 2's Epic 5 clips existing to verify `AssetHealthCheck` against real URLs.

**Overrun checkpoint**: if spend exceeds **~6.35M CU** (25% over the ~5.08M total) for the
stories completed so far, pause and report the delta with cause (wrong estimate vs. real
surprise) — informational, not a scope-cut trigger.

**Wall-clock blockers**: none that block agent execution — `requirements.md` already confirms
"no soak window, device test, or external dependency branch blocks this." The two
owner-decision items above (Windows/WSL install-CTA wording; demo title-card copy) are
**Unresolved Questions**, not wall-clock blockers in the ESTIMATION.md sense — they gate two
specific small tasks (3.1.1b, 5.2.1b), not the critical path, and default to a documented
fallback if not resolved before those tasks run.

**Basis note**: bands and weights INFERRED per `ESTIMATION.md`; recalibrate against the first
real implementer run's `subagent_tokens` and update this table.
