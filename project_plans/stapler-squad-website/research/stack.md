# Research: stack (Agent 1 — SSG/tooling + GitHub Pages deploy mechanics)

Date: 2026-10-14. Scope: `project_plans/stapler-squad-website/requirements.md` Rabbit Holes
("SSG/tooling choice") and Open Questions ("Which SSG/tooling" + "Pages-settings change
needed?").

## 1. Live repo state (verified)

```
$ gh api repos/tstapler/stapler-squad/pages
{"url":"https://api.github.com/repos/tstapler/stapler-squad/pages",
 "status":null,"cname":null,"build_type":"workflow",
 "source":{"branch":"main","path":"/"},
 "public":true,"https_enforced":true,
 "html_url":"https://tstapler.github.io/stapler-squad/"}
```

- `status: null` — Pages has never had a successful deployment. No `latest_build`/build error.
- `build_type: "workflow"` — the repo's Pages **Source** setting is *already* "GitHub
  Actions" (the alternative is `build_type: "legacy"`, meaning "deploy from a branch").
  `workflow` vs `legacy` is exactly the Source toggle in Settings → Pages → "Build and
  deployment"; there is no third state for "Actions selected but not yet configured."
- Existing workflows: `.github/workflows/demo-publish.yml` (manual GIF→Release-asset
  recorder, `workflow_dispatch` only) and 12 others (CI/lint/build/e2e/release) — **none**
  deploy to Pages today. That's the entire cause of the 404: Pages is pointed at "a workflow
  will deploy this" and no workflow has ever run `actions/deploy-pages`.

## 2. Answer to Open Question: does `build_type: workflow` need a Pages-settings change?

**No separate settings/API change is needed.** `build_type: "workflow"` is already the
correct state for an Actions-based deploy — it's set the moment someone (previously,
presumably, via the repo UI) chose "GitHub Actions" as the Pages source. GitHub's own Pages
REST docs describe `build_type` as an optional field on the *same* `POST/PUT
/repos/{owner}/{repo}/pages` endpoint used to pick the source, with `workflow` meaning
"this site is built by a custom GitHub Actions workflow" (vs. `legacy`, branch-based). There
is no documented precondition or extra PUT required before a workflow using
`actions/deploy-pages` can publish — the deploy action's own `POST
/repos/{owner}/{repo}/pages/deployments` call (which `deploy-pages` makes internally) is what
actually flips `status` away from `null`. Practical implication: **just add the workflow.**
The first successful run of a workflow ending in `actions/deploy-pages` will populate
`status`/`latest_build` and make `https://tstapler.github.io/stapler-squad/` start returning
200. No `gh api repos/tstapler/stapler-squad/pages --method PUT` call is needed (confirmed by
reasoning from the API docs — not executed, per the research-only instruction).

One caveat worth flagging for Phase 3: if the UI toggle was in fact never actually saved (some
repos show `build_type: workflow` as a *default* even when nothing was explicitly chosen —
this wasn't fully disambiguated in GitHub's docs), the fallback is a one-line settings check in
the repo UI (Settings → Pages → Source → "GitHub Actions") before merging the new workflow —
cheap, and not worth gating planning on.

## 3. Current GitHub Actions deploy-pages versions (verified via `gh api .../releases` and tags)

| Action | Latest full tag | Latest major (moving) tag |
|---|---|---|
| `actions/configure-pages` | v6.0.0 | `@v6` |
| `actions/upload-pages-artifact` | v5.0.0 | `@v5` |
| `actions/deploy-pages` | v5.0.1 | `@v5` |
| `actions/checkout` (used in build job) | — | `@v6` |

Note: GitHub's own docs page ("Using custom workflows with GitHub Pages") still shows
`upload-pages-artifact@v4` / `deploy-pages@v4` / `configure-pages@v5` in its example — that
page is stale relative to the actual latest releases above (confirmed by checking the
real release/tag lists, not the docs prose). Use the versions in the table, and pin to a
specific `vX.Y.Z` per this repo's `gha-pin-actions` skill convention at implementation time
rather than the moving major tag.

Canonical workflow shape (from GitHub's docs, adjusted to real versions):

```yaml
permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: "pages"
  cancel-in-progress: false

jobs:
  build:
    runs-on: ubuntu-22.04
    steps:
      - uses: actions/checkout@v6
      - uses: pnpm/action-setup@v4        # repo already uses pnpm in web-app/
      - uses: actions/setup-node@v5
        with: { node-version: '22', cache: 'pnpm' }
      - run: pnpm install --frozen-lockfile
      - run: pnpm run build                # SSG build → static output dir
      - uses: actions/configure-pages@v6
      - uses: actions/upload-pages-artifact@v5
        with: { path: ./<site>/dist }       # or /out for Next export

  deploy:
    needs: build
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    runs-on: ubuntu-22.04
    steps:
      - id: deployment
        uses: actions/deploy-pages@v5
```

This matches the repo's existing CI conventions (pnpm, pinned actions) and is identical in
shape regardless of which SSG is chosen below — only the build command/output dir changes.

## 4. SSG/tooling candidates

### Starlight (Astro) — recommended

- **Current version**: `@astrojs/starlight@0.42.6` (verified via
  `gh api repos/withastro/starlight/releases/latest`), built on Astro.
- **Fit**: Starlight is purpose-built for exactly this project's shape — Markdown/MDX-sourced
  docs with automatic sidebar nav, built-in full-text search (Pagefind, static-index based, no
  external service/API key needed, unlike Docusaurus's Algolia default), i18n not needed here,
  dark/light theming, and a dedicated **"splash" page template** for marketing/landing pages
  distinct from the docs layout — so one Astro+Starlight project covers both "landing page" and
  "help docs" in requirements.md's scope without bolting a separate marketing framework onto a
  docs tool (the opposite direction, Docusaurus, treats the landing page as a secondary citizen
  of a docs-first tool).
- **GitHub Pages deploy**: Astro's official docs (`docs.astro.build/en/guides/deploy/github`)
  document GitHub Pages deployment directly: either Astro's own first-party GitHub Action, or
  the generic `configure-pages`/`upload-pages-artifact`/`deploy-pages` pattern above with
  `pnpm run build` → `dist/`. Requires setting `site: 'https://tstapler.github.io'` and
  `base: '/stapler-squad'` in `astro.config.mjs` (standard project-pages subpath pattern — every
  internal link must respect `base`, which Starlight's routing handles automatically once set).
- **Markdown docs nav/search out of the box**: yes, natively — no plugin wiring needed, unlike
  Docusaurus (Algolia) or a hand-rolled Next.js docs section.
  **Jekyll**: effectively no support — Pages' native Jekyll build is for simple
  blog-shaped sites with no component model.
- **Agent-maintainability fit**: content is plain `.md`/`.mdx` under `src/content/docs/`; no
  custom theming system to learn beyond Starlight's own CSS custom properties (which can
  reference the repo's existing brand colors directly — no vanilla-extract required or
  expected, it's a from-scratch static site, not an extension of `web-app/`).
- **Media embeds**: trivial — `<video>`/`<img>` tags or MDX components pointing at external
  Release-asset URLs (`https://github.com/<repo>/releases/download/demo-gifs-latest/<name>.gif`),
  satisfying the "no committed binaries" constraint identically to any other SSG choice.

### VitePress

- **Current version**: `v2.0.0-alpha.20` per npm/GitHub tags — the "2.0" line is still alpha;
  the stable line is 1.x. Using an alpha major version for a new production site is a real risk
  (breaking changes before 2.0 stable) — if VitePress were chosen, pin to latest 1.x, not the
  alpha tag returned by `releases/latest`.
- **Fit**: lightweight, Vue-based, built-in Markdown nav/search (also Pagefind-free, its own
  MiniSearch-based local search) — very close to Starlight in capability. Where it's a weaker
  fit here: no first-class "marketing landing page" template distinct from docs (VitePress's
  home layout exists but is less fleshed out for hero/feature-grid marketing content than
  Starlight's splash template), and Vue is a second frontend framework in a repo that otherwise
  uses React (`web-app/`) — Starlight/Astro's islands architecture can embed React components if
  ever needed, Vue cannot share components with `web-app/` at all.

### Docusaurus

- **Current version**: v3.10.2.
- **Fit**: most feature-rich (versioned docs, i18n, blog) — all scope requirements.md
  explicitly excludes (no blog, no versioning need, single-locale). Its heavier default
  (React-based, larger bundle, Algolia-oriented search needing either a hosted DocSearch
  application or self-hosted Algolia-compatible index) adds setup surface with no matching
  requirement. React-based, so nominally closer to `web-app/`'s stack than Astro/Vue, but the
  two apps don't and shouldn't share a build — Starlight's Astro islands can still mount React
  components if that benefit is ever wanted.

### Next.js 15 static export reusing `web-app/`'s vanilla-extract tokens

- **Fit**: technically possible (`output: 'export'` in `next.config.ts`, `images: {
  unoptimized: true }` since the static `next/image` loader doesn't work under Pages' CDN,
  `basePath`/`assetPrefix` set to `/stapler-squad` for the project-pages subpath — confirmed
  pattern from multiple current guides, with the caveat that `assetPrefix` does not
  automatically rewrite paths to files under `public/`, a real footgun found repeatedly in
  the research) — but there's no Markdown-docs-with-nav/search primitive in Next.js itself;
  that would mean hand-building a docs renderer (MDX pipeline, sidebar, search index) from
  scratch, which is exactly the hand-rolled work Starlight/VitePress/Docusaurus give for free.
  Reusing vanilla-extract tokens buys brand consistency but costs the entire docs-framework
  layer requirements.md needs (install docs, troubleshooting nav, search). Given this is a
  **separate, standalone static site** per requirements.md ("new service... separate from the
  product itself"), there's no actual code-sharing need with `web-app/`'s Next.js app beyond
  copying the same SVG logo files — which any SSG can do by just referencing the static asset.

### Plain static HTML / Jekyll

- Jekyll is GitHub Pages' zero-config native option (via `actions/jekyll-build-pages`), but
  gives no component model for a landing page with feature grids/demo embeds beyond raw
  Liquid templating, and no Markdown-docs nav/search without manually wiring a plugin — more
  hand-building than Starlight for less capability. Ruled out.
- Hand-rolled static HTML: no framework risk at all, but every piece of "nav/search/feature
  grid/responsive layout" requirements.md implicitly wants (Lighthouse ≥90, mobile-friendly,
  searchable docs) would be built from scratch instead of inherited from a maintained project.
  Worse agent-maintainability than Starlight: no component reuse, raw HTML duplication across
  pages literally recreates the `jscpd` duplication problem this repo already fights elsewhere.

## 5. Recommendation

**Use Astro + Starlight.** Rationale, directly against this project's stated constraints:

1. **Agent-maintained, not human-maintained long-term** (requirements.md Baseline/Size) —
   Starlight's content model is plain Markdown/MDX files with frontmatter; an agent adding a
   new how-to page edits/creates one file and the sidebar nav updates automatically from the
   file tree config, with no custom routing code to write or review.
2. **Markdown docs + landing page in one project, no bolted-together two-framework setup** —
   Starlight's splash-page template handles the marketing landing page; its default docs layout
   handles curated how-to content; both come from the same `astro build`, same CI job, same
   deploy workflow.
3. **Search and nav out of the box** — Pagefind-based static search needs no Algolia account,
   no API keys, nothing that could silently stop working after this project ships (an agent six
   months from now re-running `astro build` gets working search for free; a Docusaurus choice
   risks an expired/misconfigured Algolia index going unnoticed, since it fails soft).
4. **Video/GIF embeds and "no committed binaries" constraint** are satisfied identically
   regardless of SSG choice — reference Release-asset URLs from `demo-publish.yml`'s existing
   pattern (or a new, higher-production-value recorder per requirements.md scope) directly in
   Markdown/MDX `<video>`/`<img>` tags. This is not a differentiator between candidates.
5. **No real code-sharing benefit from Next.js/vanilla-extract reuse** — this is explicitly a
   separate service from `web-app/` (requirements.md Type), so the "reuse the toolchain"
   rabbit hole resolves to "don't": pulling in vanilla-extract would mean recreating Starlight's
   entire docs/search/nav layer by hand for a brand-consistency benefit that's achievable anyway
   by just copying the existing SVG files into Starlight's `public/`.
6. **Deploy mechanics are a non-issue either way** — `actions/configure-pages@v6` +
   `actions/upload-pages-artifact@v5` + `actions/deploy-pages@v5` against `astro build`'s
   `dist/` output is a standard, documented pattern (§3 above), no different in shape from any
   other SSG's equivalent build→upload→deploy job.

**Runner-up**: VitePress, if an Astro/Starlight-specific blocker emerges in Phase 3 planning
(e.g. a specific layout need Starlight's splash template can't express) — same capability
tier, same deploy mechanics, smaller ecosystem/plugin surface. Pin to the latest stable 1.x line
if chosen, not the `v2.0.0-alpha.20` tag `releases/latest` currently resolves to.

## Sources

- [GitHub Pages REST API — pages](https://docs.github.com/en/rest/pages/pages) (`build_type` field semantics)
- [GitHub Docs — Using custom workflows with GitHub Pages](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages) (workflow shape; versions there are stale — see §3)
- [Astro — Deploy your Astro Site to GitHub Pages](https://docs.astro.build/en/guides/deploy/github/) (`site`/`base` config, official Action)
- `gh api repos/actions/{deploy-pages,upload-pages-artifact,configure-pages}/releases/latest` and `.../tags` (run live, output in §3)
- `gh api repos/tstapler/stapler-squad/pages` (run live, output in §1)
- `gh api repos/withastro/starlight/releases/latest`, `repos/vuejs/vitepress/releases/latest`, `repos/facebook/docusaurus/releases/latest` (run live, output in §4)
- `.github/workflows/demo-publish.yml` header comment (Release-asset pattern precedent, `/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-website_18de8fdccb9d2a48/.github/workflows/demo-publish.yml`)
