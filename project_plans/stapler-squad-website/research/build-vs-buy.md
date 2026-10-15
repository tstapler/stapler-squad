# Build vs. Buy — stapler-squad-website (Phase 2, Agent 6)

Scope: does the GitHub Pages marketing+docs site get hand-rolled or built on an
existing SSG/framework, does the "higher production value" demo-recording
pipeline reuse an existing tool, and does any supporting script (docs
curation sync) need a library instead of a bespoke script. See
`project_plans/stapler-squad-website/requirements.md` for full constraints;
this doc evaluates the "Alternatives Considered" and "Rabbit Holes: SSG/tooling
choice" / "Automated media pipeline" sections through a build-vs-buy lens
(maturity, maintenance burden, license, fit) — not a stack feature comparison,
which Agent 1's parallel research covers.

## 1. SSG / framework options

Constraints that matter most here, repeated from requirements.md: the site is
**agent-maintained** (no human doing ongoing manual upkeep — favor low
operational surface over editor ergonomics), needs **Markdown-driven docs with
nav**, needs to **embed external video/GIF URLs** (Release-asset links, not
local files), needs to **reuse existing brand SVGs** (`web-app/public/logo.svg`,
`logos/concepts/*.svg`, `logos/iterations/*.svg`), and **deploys via GitHub
Actions to GitHub Pages at a repo subpath** (`https://tstapler.github.io/stapler-squad/`,
no custom domain).

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **Docusaurus** (Meta-maintained, React, MIT) | Most feature-complete docs framework (built-in sidebar nav, search via local plugin or Algolia, versioning, MDX for embedding video); huge ecosystem; Meta-backed so unlikely to be abandoned | React + webpack toolchain is heavier than this project needs for ~15 content pages; its opinionated project structure (`docusaurus.config.js`, sidebars, versioned-docs folders) is more scaffolding to maintain than the content volume justifies; another JS framework/build system alongside the existing Next.js `web-app/` and Playwright `tests/e2e/` toolchains, for no code-sharing benefit (it can't reuse `web-app`'s vanilla-extract theme) | **Viable**, but oversized for this project's scope |
| **VitePress** (Vue, Vite, MIT) | Lightweight, fast builds, the de facto choice for small-to-medium dev-tool docs (Vite, Vue, Vitest itself use it); Markdown-first with minimal config; built-in local search (minisearch), sidebar/nav from a single config object; easy GitHub Pages deploy (official guide ships a `base` option for subpath deploys, matching the repo-subpath constraint exactly) | Introduces Vue into a repo that is otherwise Go + React/Next.js — a third UI framework with zero reuse elsewhere; still a Node toolchain to keep patched | **Viable** — best-fit dedicated SSG if a dedicated SSG is wanted, mainly because its subpath-deploy story is the most turnkey of the three JS-framework options |
| **Astro** (framework-agnostic, content-focused, MIT) | "Islands" architecture ships zero JS by default — best Lighthouse scores of the three framework options, which matters given the ≥90 Lighthouse target; content collections give typed Markdown/MDX with built-in validation; can embed a vanilla-extract/React landing-page island if later desired, so it isn't a dead end | Yet another framework + build toolchain for a project whose owner is explicit that no human does ongoing manual upkeep; Starlight (Astro's docs theme) is the fastest-growing option per community sources but has the least install-base track record of the three | **Viable** — strongest technical fit (performance, flexibility) if the team were optimizing for the best possible site; weakest "boring tool, low maintenance surface" fit |
| **Plain Jekyll** (GitHub Pages native, Ruby, MIT) | Zero custom build step — GitHub Pages can build it server-side with no Actions workflow at all; GitHub's own longest-supported Pages path; minimal config (`_config.yml`, `_layouts/`) | Requirements.md explicitly calls for a GitHub Actions deploy workflow (to fix the broken `build_type: workflow` Pages setting) — Jekyll's main advantage (no workflow needed) is moot here since a workflow is being built anyway; Ruby is a fourth language/toolchain in a Go+TS repo with no other Ruby anywhere; weaker component/theming model (plain Liquid templates) makes "feature highlights derived from README," demo-media embeds, and nav nontrivial without plugins; GitHub Pages' native Jekyll build only supports a safelisted plugin set, which rules out custom-logic plugins (e.g. auto-pulling Release-asset URLs) unless that logic moves into a pre-build script anyway, eroding the "zero build step" advantage | **Not recommended** — its one differentiator (no Actions workflow) doesn't apply to this project's confirmed design, and it adds a language with zero other footprint in the repo |
| **Reuse `web-app/`'s Next.js 15 + vanilla-extract via static export** | Already proven to produce a static export in this repo — `web-app/next.config.ts` sets `output: "export"` today (used for the product's own embedded-SPA build), so the toolchain, CI caching (`pnpm`, Node setup in `.github/actions/prepare`), and vanilla-extract theme tokens are all already present and working; zero new language/framework/license to adopt; visual consistency with the actual product UI for free | Next.js has no built-in Markdown-docs-with-nav story — "Markdown-driven docs with nav" would need a hand-built MDX/remark pipeline (content loader, sidebar generator, search) that a dedicated docs SSG gives for free; `web-app/`'s existing `output: "export"` config is tuned for a single embedded SPA route tree, not a multi-page marketing+docs site, so reusing it means a second, separately-configured Next app (or a second export target) rather than literally the same build | **Recommended** — but see caveat below: only if MDX/nav tooling is added via a small, well-trodden plugin (e.g. `nextra` — a Next.js-native docs theme — or a hand-rolled remark/rehype pipeline), not built from raw `fs.readdir` + custom parsing |
| **Hand-rolled static HTML/CSS + simple Markdown→HTML step** | Simplest possible mental model; no framework version-upgrade burden ever; total control over output; trivially debuggable by an agent with no framework-specific knowledge required | Loses *all* docs-authoring ergonomics the requirement explicitly wants ("Markdown-driven docs with nav") — nav, breadcrumbs, search, and cross-linking all become bespoke code the project must build and maintain itself, which is more total surface area than adopting a maintained SSG, not less, once the doc count is "a dozen+" (per Success Metrics) and growing | **Not recommended** — the "simplicity" is illusory once nav/search/Markdown-rendering is counted; this is the one place "build" is clearly worse than "buy" |

### Overall SSG verdict

**Recommended: Nextra on top of the existing Next.js 15 toolchain**, as a
refinement of "reuse `web-app/`'s toolchain." Nextra (MIT, actively maintained,
built specifically as a Next.js docs theme) gives Markdown-driven docs with
auto-generated nav/sidebar/search on top of the exact static-export pipeline
this repo already runs (`output: "export"`, same pnpm/Node CI steps as
`web-app/`), closing the one real gap ("reuse Next.js" lacks a docs-nav story)
without adopting a second UI framework (Docusaurus/VitePress/Astro would each
be the *only* user of React-via-Docusaurus, Vue, or Astro's component model in
this otherwise Go+Next.js+Go-CLI repo). If Nextra itself turns out to be too
opinionated for the landing-page half of the site (hero + feature highlights +
demo embeds, which isn't "docs" in Nextra's sense), the fallback is a plain
Next.js page for the landing page and Nextra (or hand-rolled MDX via
`@next/mdx`) for the `/docs` subtree only — still one toolchain, two route
trees, zero new frameworks.

If the owner prefers a toolchain with zero shared code with `web-app/` (e.g.
to keep the marketing site's build fully decoupled from the product's), the
second choice is **VitePress** — not Docusaurus or Astro — specifically
because its subpath-deploy story (`base` config option) is the most direct
match for this project's fixed `/stapler-squad/` GitHub Pages subpath, and its
footprint (single Vite build, minimal config surface) best matches the
"agent-maintained, no ongoing human upkeep" constraint among the three
dedicated SSGs.

Plain Jekyll and hand-rolled HTML are both **not recommended** for this
project's specific scope, for the distinct reasons in the table above.

## 2. Demo-recording tooling

**Finding: no mature, widely-adopted OSS library exists for "Playwright video
capture + branded overlay/pacing post-processing."** A web search (2026-10-14)
for this exact capability turned up only: Playwright's own built-in video/trace
APIs (capture only, no branding), a handful of small/recent community
"skills"/plugins (e.g. an "Automated Demo Video Recorder" skill pairing
Playwright with ffmpeg, a "walkthrough-record" skill with a cursor/ripple
overlay engine) with no established maintenance or adoption track record, and
otherwise plain ffmpeg pipelines assembled by individual blog authors. The
`playwright-video` npm package exists and isn't marked deprecated, but it only
wraps Chromium's CDP screencast API to produce a raw video — it adds nothing
over Playwright's own native `recordVideo` option (which this repo's
`tests/e2e/` already uses) and has no overlay/branding capability at all.

This repo already has the right primitive, further along than any external
option found: **`scripts/demo-post-process.sh`** (read in full for this
research) already does real post-production work — it re-encodes a captured
WebM with a drawn browser-chrome frame (traffic-light buttons, URL bar, via
ffmpeg `drawbox` filters) and exports a palette-optimized GIF preview
(two-pass `palettegen`/`paletteuse`, matching the same technique
`demo-publish.yml` uses independently for its own GIF conversion step). It is
not currently wired into `demo-publish.yml`'s `record-demos` job — that
workflow does its own separate, simpler webm→GIF conversion inline and never
calls this script — so there's existing, unused post-processing logic to
build on rather than two competing implementations to reconcile.

**Verdict: extend `demo-post-process.sh`'s ffmpeg approach, do not adopt an
external tool.** Concretely, "higher production value than `demo-publish.yml`"
should mean: (a) wire `demo-post-process.sh` (or an evolved version of it)
into the new demo workflow so every clip gets the browser-chrome frame
treatment already built, (b) add incremental ffmpeg-filter-graph work for
whatever specific "better pacing/framing/branding" elements Phase 3 planning
scopes (e.g. a title-card intro via `drawtext`/`overlay` with the repo's logo
SVG rasterized once at build time, slight speed-ramping via `setpts` for dead
air, consistent crop/letterbox framing) — all achievable as additional ffmpeg
filter-graph stages layered onto the existing script's pattern, not a new
dependency. This keeps the entire pipeline inside a single well-understood
tool (ffmpeg) already present in CI (`demo-publish.yml` already does
`apt-get install -y ffmpeg`), with no new license, registry dependency, or
maintenance surface, and reuses code instead of writing a parallel
implementation. Rationale for rejecting the external "skills"/plugins found:
they're recent (first indexed ~2026-07), unverified for maintenance/license,
and solve a problem this repo has already solved more specifically for its
own use case (the existing script already matches its own brand's chrome
styling via hardcoded hex colors) — adopting one would mean *more* integration
work (reconciling a generic tool's overlay conventions with the existing
look) than extending the script already written for this exact purpose.

## 3. Supporting scripts: docs-curation sync, Release-asset manifest

**Finding: no standard "docs subset sync" tool exists.** A targeted search
found only tangential tools — a generic docs-directory-mirroring plugin for
an unrelated static-site generator ("Steno"), a single-file push tool for the
ReadMe.io SaaS product, and an internal CLI (`go-devops`/`core.help`) that
copies whole repos' docs trees wholesale rather than a hand-picked subset.
None matches the requirement's actual shape: a small, fixed, explicitly
curated list of source files (requirements.md's Rabbit Holes section calls
for "an explicit file-by-file include/exclude list," not a pattern-based
sync) copied/adapted into the site's content directory.

**Verdict: a bespoke script is clearly correct here, and reaching for a
library would be over-engineering.** The actual mechanism needed is: read a
fixed, version-controlled list of `docs/*.md` paths (produced by Phase 3
planning's file-by-file curation pass), copy or lightly transform each
(e.g. strip internal-only sections, rewrite relative links to the public
site's structure, inject frontmatter for the chosen SSG's nav), and fail
loudly if a listed source file goes missing (the registry-pattern precedent
already used elsewhere in this repo, e.g. `make registry-diff`'s dry-run
check, is the right template to copy, not a new tool to adopt). This is a
few dozen lines of Node/Go/shell with no algorithmic complexity (no
diffing, no two-way sync, no conflict resolution — it's one-directional,
small, and bounded), and the project already has conventions for exactly
this shape of "generate derived files from a manifest, verify in CI" problem
(the Feature Registry's `make registry-generate`/`make registry-diff`
pattern in the top-level `CLAUDE.md`). Likewise, a Release-asset-URL
manifest generator (mapping demo names to their stable
`.../releases/download/demo-gifs-latest/<name>.gif` URLs for the site to
embed) is a direct, bounded lookup with a well-known URL shape
(`demo-publish.yml`'s own job already documents the stable-URL convention,
including `--clobber` to keep names stable across reruns) — not a case for
pulling in a GitHub-Releases SDK or client library.

## 4. Overall build-vs-buy verdict

| Component | Build or buy | Rationale |
|---|---|---|
| Site framework/SSG | **Buy** (adopt Nextra on the existing Next.js toolchain, or VitePress as the fully-decoupled alternative) | Markdown nav/search/theming is a solved problem with mature, actively-maintained, permissively-licensed options; hand-rolling it duplicates work for no benefit once content exceeds a handful of pages |
| Landing page itself | **Build** (thin layer on top of whichever framework is chosen) | It's a handful of bespoke sections (hero, feature highlights, demo embeds, install CTA) driven by this project's own content and brand assets — no existing tool "is" a landing page; this is normal site-building work, not a build-vs-buy question |
| Demo-recording capture | **Keep** existing Playwright + `tests/e2e/` fixtures (already built, already proven via `demo-publish.yml`) | No external capture tool found that improves on Playwright's native video recording this repo already uses |
| Demo-recording post-processing ("higher production value") | **Build** (extend `scripts/demo-post-process.sh`'s ffmpeg filter-graph approach) | No mature external tool exists for this specific need; this repo already has a more specific, already-working implementation than anything found externally; extending it is less work and less risk than integrating an unproven community skill |
| Docs-curation sync script | **Build** (small bespoke script, following the Feature Registry's generate/diff convention) | Scope is small and bounded by design (explicit file-by-file list, one-directional copy); no tool exists for this narrow a problem, and reaching for one would add a dependency to replace ~50 lines of straightforward code |
| Release-asset URL manifest | **Build** (small bespoke script/lookup) | Fixed, well-known URL shape already established by `demo-publish.yml`; no SDK or client library is justified for constructing a URL string |

**Net assessment:** this project should buy exactly one thing (the docs
framework/SSG) and build everything else. The SSG choice is the only place a
mature, low-maintenance, permissively-licensed option clearly beats a custom
build at this project's content volume (a dozen-plus Markdown pages with
nav/search). Every other piece in scope — the landing page content, the demo
post-processing branding, and the two small sync/manifest scripts — is either
inherently bespoke (no tool "is" a landing page) or narrow enough in scope
that no external tool exists to replace a straightforward custom script, and
this repo already has a more specific, working starting point
(`demo-post-process.sh`, the Feature Registry generate/diff pattern) than
anything an external search turned up.

---

### Sources consulted

- This repo: `web-app/next.config.ts` (confirms `output: "export"` already
  works in this codebase), `scripts/demo-post-process.sh` (read in full),
  `.github/workflows/demo-publish.yml` (existing GIF-publish pattern,
  Release-asset URL convention), `tests/e2e/` directory listing (existing
  Playwright spec inventory), `logos/concepts/`, `logos/iterations/`,
  `web-app/public/` (existing brand assets).
- Web search, 2026-10-14: Docusaurus/VitePress/Astro/Starlight comparison
  blogs (pkgpulse.com, wmtips.com, docsio.co) — secondary sources with no
  verifiable maintenance-activity data; treated as directional only, not as
  verified fact, per the caveats noted inline above.
- Web search, 2026-10-14: Playwright video/overlay tooling search (surfaced
  the "Automated Demo Video Recorder" and "walkthrough-record" community
  skills, the `playwright-video` npm package, and Flavio Copes' ffmpeg
  walkthrough) — none found to be a mature, widely-adopted standard.
- Web search, 2026-10-14: "docs subset sync" tooling search — no matching
  standard tool found; closest analogs (Steno docs-mirror plugin, ReadMe
  sync CLI, internal go-devops `core.help` sync) are directory-level mirrors
  or SaaS-push tools, not file-subset selectors.
