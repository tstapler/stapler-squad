# Requirements: stapler-squad-website

**Date**: 2026-10-14
**Type**: new service (marketing/docs site, separate from the product itself)
**Complexity**: 3 — system design

## Problem Statement
Stapler Squad's only public face today is its GitHub README and repo. There is no dedicated landing experience that explains the product, no polished demo media beyond a single hero GIF/webm, and no searchable help/documentation site — anyone evaluating or getting stuck with the tool has to read source, the README, or open a GitHub issue. The project needs a GitHub Pages site that showcases the app (landing page, demo videos/GIFs) and gives users self-serve help documentation.

## Baseline
Today, a prospective user's path is: land on github.com/tstapler/stapler-squad → read README.md (feature bullet list, one demo.gif/demo.webm embed, install instructions) → optionally open docs/features.md for a screenshot tour → otherwise dig through docs/how-to, docs/reference, docs/explanation directly in the repo tree, or file a GitHub issue if stuck. GitHub Pages is nominally "enabled" for this repo (`gh api repos/tstapler/stapler-squad/pages` reports `build_type: workflow`, pointing at https://tstapler.github.io/stapler-squad/) but no deploy workflow exists and the URL 404s — confirmed live via `curl -s -o /dev/null -w "%{http_code}" https://tstapler.github.io/stapler-squad/` → `404`.

## Users / Consumers
- Prospective adopters evaluating Stapler Squad for the first time (landing page + demo media is the conversion path)
- Existing users who hit a problem and want self-serve help (docs/how-to content) instead of filing an issue
- The repo's own README, which already links to the (currently broken) Pages URL and can link more deeply once the site exists

## Success Metrics
- `https://tstapler.github.io/stapler-squad/` returns 200 and renders a landing page, demo media, and a help-docs section (today: 404)
- Deploys automatically on push to `main` via a GitHub Actions workflow (today: no such workflow exists)
- A new visitor can go from the landing page to a working local install without leaving the site or reading source (today: requires reading README + docs/ in the repo)
- A user hitting a common issue (e.g. install failure, tmux session issue) can find a relevant how-to page from the site's nav/search (today: must know to look in docs/how-to/ in the repo tree)

## Size
Large (3–15M CU) — new site scaffold + content migration/curation across a dozen+ docs + new automated demo-recording pipeline (higher production value than demo-publish.yml) + CI deploy workflow + landing page design, all net-new. INFERRED per ESTIMATION.md bands; full scope is the default, not a question for the owner.

**Wall-clock blockers**: none identified. Demo/feature media will be produced via scripted, automated Playwright captures (no owner recording/narration time), per the owner's explicit choice during ideation. Content curation, site build, and deploy are all agent-executable; no soak window, device test, or external dependency branch blocks this.

## Constraints
- Must be served via GitHub Pages (per the user's request and the repo's existing, currently-broken Pages configuration) — no separate hosting provider
- No custom domain requested; keep the default `https://tstapler.github.io/stapler-squad/` URL
- Public repo, public site — no content requiring access control
- Should not reintroduce the git-history bloat problem already solved once: `demo-publish.yml`'s own header documents that committing multi-MB GIFs directly into `docs/demos/` left ~15.6GB of dead blob history; new media must follow the same pattern (GitHub Release assets or build-time fetch, not committed binaries) unless a specific asset is small enough that committing it is clearly fine (e.g. the existing sub-2MB `assets/demo.gif`/`demo.webm`/`screenshot.png`)
- Reuse the existing brand assets already in the repo (`web-app/public/logo.svg`, `logos/concepts/*.svg`, `logos/iterations/*.svg`) rather than commissioning new branding
- The new demo-recording workflow must be manually/occasionally triggered (`workflow_dispatch`, mirroring `demo-publish.yml`'s existing pattern), never run automatically on every push or as part of the site's own deploy — the user confirmed mid-ideation that the *previous* demo recordings were disabled specifically because they consumed too much repo/CI space when generated too often. The site's build/deploy must consume already-published media (Release assets), not regenerate it per deploy.

## Non-functional Requirements
- **Performance SLO**: not formally specified; target Lighthouse performance/accessibility scores ≥ 90 for a static marketing site (web-app already runs Lighthouse CI per `web-app/lighthouserc.json` — same bar is reasonable here)
- **Scalability**: not applicable — static site on GitHub Pages' CDN
- **Security classification**: public
- **Data residency**: no special requirements (public static content)

## Scope
### In Scope
- Landing page: hero section, feature highlights (derived from README's existing Highlights section), install CTA, demo media
- Demo videos/GIFs: new, scripted/automated Playwright-driven recordings with better framing/pacing/branding overlays than the current `demo-publish.yml` output (owner chose "automated, higher production value" over manual recording or a manual/automated mix). Triggered manually/occasionally (`workflow_dispatch`), not on every push — the owner previously disabled demo recordings generated too frequently because of the repo/CI space cost; the site consumes the resulting Release assets rather than regenerating them on each deploy
- Help documentation section: curated, user-facing subset only — install/getting-started, feature overview (building on `docs/features.md`), and relevant `docs/how-to/*` troubleshooting content, adapted for a public audience (owner chose "curated user docs" over mirroring the full Diataxis tree or the entire `docs/` tree)
- A GitHub Actions workflow that builds and deploys the site to GitHub Pages on push to `main` (filling the gap left by the currently-broken Pages config)
- Fixing/replacing the existing broken Pages configuration (`build_type: workflow` with no actual workflow) so the long-standing README link resolves

### Out of Scope
- Mirroring internal docs (ADRs, bug tracking, `project_plans/`, architecture audits) to the public site — these remain GitHub-repo-only
- A blog, CMS, or any dynamic/user-generated content
- Custom domain / DNS setup
- Analytics/tracking infrastructure beyond what GitHub's own repo Insights already provide
- Rewriting or restructuring the underlying `docs/` Diataxis hierarchy itself — the site curates/adapts from it, it does not reorganize the source (a single pre-existing broken-reference fix in `docs/features.md`, tracked in plan.md's Tech Debt Disposition, is not considered restructuring)

## Rabbit Holes
- **SSG/tooling choice**: the product's own `web-app/` is Next.js 15 + vanilla-extract, built to a bundled Go-binary-embedded SPA — not necessarily the right fit for a public static marketing+docs site. Research phase should evaluate options (Next.js static export reusing existing theme tokens, vs. a dedicated docs-oriented SSG like Docusaurus/VitePress/Astro, vs. plain static HTML) rather than defaulting to "whatever the app uses."
- **Automated media pipeline**: "higher production value than `demo-publish.yml`" (branding overlays, better pacing/framing) is open-ended — could expand indefinitely. Phase 3 planning should bound exactly which clips get produced and to what spec, reusing `demo-publish.yml`'s existing `FEATURE_SPECS` set as the starting scope rather than inventing new flows. Trigger cadence is resolved (manual `workflow_dispatch`, not per-push — see Constraints); the open part is only the clip spec/scope itself.
- **Docs curation boundary**: "curated user docs" still requires judgment calls per existing `docs/how-to/*` file on whether it's user-facing (e.g. `debug-with-logs.md`) or internal/dev-facing (e.g. `run-nil-safety-analysis.md`) — Phase 3 should produce an explicit file-by-file include/exclude list rather than leaving this to implementation-time judgment.
- **Fixing the broken Pages config**: the existing `build_type: workflow` setting with no workflow may need a specific GitHub API/UI change (not just adding a workflow file) — confirm during research whether `actions/deploy-pages` will pick it up as-is or whether the Pages source setting itself needs to change first.

## Alternatives Considered
- Static HTML/CSS hand-rolled site — simplest, but loses easy Markdown-driven docs authoring
- Docusaurus / VitePress / Astro (dedicated docs+marketing SSGs) — purpose-built for this exact "landing page + docs" shape
- Reusing the existing Next.js 15 + vanilla-extract `web-app/` toolchain via static export, for visual/brand consistency with the actual product UI
- Jekyll (GitHub Pages' native built-in support, zero extra build step) — simplest CI, but weaker component/theming model than the above

## Feasibility Risks
- GitHub Pages' existing `build_type: workflow` setting with no corresponding workflow is an unusual state; may need a Pages-settings change (via API/UI) in addition to adding the deploy workflow
- Automated "higher production value" demo recording depends on the app's Playwright e2e fixtures (`tests/e2e/`) staying stable; flaky or changed UI could break the recording pipeline the same way `demo-publish.yml` already guards against (see its own curated `FEATURE_SPECS` list, deliberately excluding flaky specs)
- No existing precedent in this repo for a second, publicly-hosted static site — build/deploy conventions must be established from scratch

## Observability Requirements
*(complexity 3)* No new runtime observability needed — it's a static site. Rely on: the GitHub Actions deploy workflow's own pass/fail status (visible as a repo badge, consistent with existing `build.yml`/CI badges in README), and GitHub's built-in repo Insights → Traffic for visit counts. No new metrics/alerting infra.

## Risk Control
*(complexity 3)* Low risk — static content only, no user data, no runtime behavior change to the actual product. Rollback = re-run the Pages deploy workflow against the previous `main` commit SHA, or revert the merge commit that introduced a bad change. No feature flag or staged rollout needed.

## Open Questions
- Which SSG/tooling (see Rabbit Holes) — resolve in Phase 2 research
- Exact per-file include/exclude list for "curated user docs" — resolve in Phase 3 planning
- Exact spec/scope for the new automated demo recordings (which flows, what branding overlay treatment) — resolve in Phase 3 planning
- Whether the existing Pages "enabled but broken" setting needs a Pages-settings API change vs. just adding a workflow file — resolve in Phase 2 research
