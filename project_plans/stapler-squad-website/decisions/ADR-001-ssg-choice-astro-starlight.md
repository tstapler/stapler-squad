# ADR-001: Use Astro + Starlight as the site's SSG/framework

**Status**: Accepted
**Date**: 2026-10-14
**Context project**: stapler-squad-website

## Context

Two Phase 2 research tracks disagreed:

- `research/stack.md` (Agent 1, SSG/deploy-mechanics focus) recommends **Astro + Starlight**.
- `research/build-vs-buy.md` (Agent 6, build-vs-buy focus) recommends **Nextra on the
  existing Next.js 15 toolchain** (with VitePress as a fully-decoupled second choice).
- `research/architecture.md` (Agent 3, integration-points focus) independently concluded the
  site should **not** reuse `web-app/`'s Next.js toolchain, for different reasons (deploy
  target, config double-duty, no real code-sharing benefit since this is an explicitly
  separate service).

This ADR resolves the conflict rather than silently picking a side.

## Options considered

| Option | Key strength | Key weakness |
|---|---|---|
| **Astro + Starlight** | Islands architecture ships zero JS by default — best structural fit for the ≥90 Lighthouse performance target; Starlight's "splash" template covers the marketing landing page and its default docs layout covers help docs from one project/build; Pagefind search and sidebar nav ship with no plugin wiring, no API keys, no third-party service that can silently rot | New framework + toolchain with zero code-sharing with `web-app/` (Next.js/React/vanilla-extract) — a second Node-based build to maintain, mitigated by this being an explicitly separate service already |
| **Nextra on existing Next.js 15 toolchain** (build-vs-buy.md's pick) | Reuses the pnpm/Node CI steps and `output: "export"` pattern already proven in `web-app/next.config.ts`; zero new framework/license to adopt | React hydration is the default cost model even for static content, working against the ≥90 Lighthouse target this project explicitly sets higher than `web-app/lighthouserc.json`'s own ≥0.70-0.80 (warn-only) bar; Nextra's current release maturity/version was not live-verified the way Starlight/VitePress/Docusaurus were in `stack.md` (`gh api repos/withastro/starlight/releases/latest` etc. — no equivalent check was run against Nextra); bolts a docs theme onto a single-SPA-route config (`web-app/next.config.ts`'s `basePath`/`trailingSlash` are tuned for Go-binary embedding, not a second deploy target) |
| **VitePress** | Most turnkey GitHub Pages subpath story of the three dedicated SSGs (`base` config maps directly onto `/stapler-squad/`); lightweight, fast builds | Introduces Vue as a third UI framework with zero reuse anywhere else in the repo (Go + React); weaker "splash"/marketing-page template than Starlight for the hero+feature-grid landing page; stable release line is still 1.x (the `v2.0.0-alpha.20` tag `releases/latest` resolves to is pre-release and would need explicit pinning away from) |

## Decision

**Astro + Starlight.** Rationale, in order of weight:

1. **Two of the three research tracks converge on it independently**, from different angles
   (SSG-capability comparison in `stack.md`; "don't reuse web-app's toolchain, it's a separate
   service" in `architecture.md`). `build-vs-buy.md`'s Nextra recommendation is the outlier, and
   its own writeup hedges it twice ("if Nextra itself turns out to be too opinionated for the
   landing-page half... the fallback is...") rather than presenting it as a clean fit.
2. **The ≥90 Lighthouse NFR is a structural, not incidental, requirement** (`requirements.md`
   Non-functional Requirements), and zero-JS-by-default is a direct, mechanical answer to it in
   a way "ship a React app and hope hydration cost stays low" is not.
3. **Nextra's version/maintenance health was never independently verified** the way the three
   dedicated SSGs were (`stack.md` §4 ran a live `gh api .../releases/latest` check against
   Starlight, VitePress, and Docusaurus; no equivalent check exists for Nextra anywhere in the
   research set). Recommending an unverified option over three verified ones is a gap this ADR
   closes by picking a verified one.
4. **One Astro+Starlight project satisfies both halves of scope** (marketing landing page via
   the splash template, curated help docs via the default docs layout) without a second
   route-tree or a second framework bolted onto a single-SPA Next.js config that was never
   designed to serve two deploy targets.

## Consequences

- New standalone package `website/` (own `package.json`, pnpm lockfile), per
  `research/architecture.md`'s placement recommendation — not nested under `docs/` or `web-app/`.
- `astro.config.mjs` must set `site: 'https://tstapler.github.io'` and
  `base: '/stapler-squad'` — every internal link/asset must go through Starlight's
  base-aware routing (see `research/pitfalls.md` §2c) or it breaks only once deployed, not in
  local dev.
- No code or component sharing with `web-app/`'s React/vanilla-extract stack; brand consistency
  is achieved by copying `web-app/public/logo.svg` and `logos/*/*.svg` into `website/public/`,
  not by sharing a build.
- Runner-up for future reconsideration if a Starlight-specific limitation emerges during
  implementation: **VitePress** (same deploy-mechanics tier, smaller ecosystem) — not Nextra,
  per point 3 above.

## Rejected

- **Nextra on Next.js 15** — optimizes for toolchain reuse at the cost of the zero-JS
  performance edge the Lighthouse NFR needs, and was recommended without the same
  release-health verification applied to the other three candidates.
- **VitePress** — viable, same deploy-mechanics tier as Astro+Starlight, but introduces Vue as
  a third framework with zero reuse elsewhere and a weaker marketing/splash template; kept as
  the documented fallback, not the primary choice.
- **Docusaurus** — most feature-complete (versioning, i18n, blog) but all of that is explicitly
  out of scope (`requirements.md` Out of Scope); heavier React+webpack toolchain and an
  Algolia-DocSearch-oriented search default (hosted-service dependency, fails soft if
  misconfigured) for no matching requirement.
- **Plain Jekyll / hand-rolled static HTML** — ruled out in `research/stack.md` §4: no
  component model for a landing page with feature grids/demo embeds, and no Markdown-docs
  nav/search without hand-building exactly what Starlight gives for free.
