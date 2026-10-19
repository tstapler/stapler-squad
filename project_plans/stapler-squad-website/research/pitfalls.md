# Research: Pitfalls — stapler-squad-website

Agent 4 of Phase 2 (SDD). Scope: known risks for building/deploying a GitHub
Pages marketing+docs site, with specific attention to this repo's own prior
incidents.

## 1. Repo-specific precedent: the demo-GIF git-bloat incident

[`.github/workflows/demo-publish.yml`](../../../.github/workflows/demo-publish.yml)'s
header comment (lines 1-17) documents the incident directly:

> Previously committed GIFs straight into docs/demos/ on main. Multi-MB
> binary GIFs don't delta-compress between re-recordings, so 16 runs of this
> workflow left ~15.6GB of dead blob history behind... Release assets are
> durable and get a stable download URL
> (`https://github.com/<repo>/releases/download/demo-gifs-latest/<name>.gif`)
> without touching git history at all.

**Exact lesson**: any binary asset that gets *regenerated* (not authored
once) must never be `git commit`-ed, even if an individual file looks small —
the damage compounds across every re-run because GIF/video binaries don't
delta-compress, so N regenerations cost N × file-size in permanent,
unrecoverable git-blob history (`git gc` doesn't reclaim it once pushed;
every future clone/`fetch-depth: 0` pays the cost forever). The fix pattern
already proven in this repo: publish to a rolling GitHub Release
(`demo-gifs-latest`) with `gh release upload ... --clobber`, which keeps a
stable download URL while overwriting the asset in place — no history
growth per run.

**Does the new site's design (per `requirements.md`) avoid repeating this?**
Yes, by explicit constraint — confirmed in `requirements.md`'s Constraints
section: "new media must follow the same pattern (GitHub Release assets or
build-time fetch, not committed binaries) unless a specific asset is small
enough that committing it is clearly fine (e.g. the existing sub-2MB
`assets/demo.gif`/`demo.webm`/`screenshot.png`)." One sharp edge for Phase 3
planning: the site's **build/deploy step** must fetch the Release assets at
build time (e.g. `curl`/`gh release download` against the `demo-gifs-latest`
tag) rather than committing them into the new site's own source tree — doing
the latter would just relocate the same bloat problem from `docs/demos/` to
e.g. `website/public/demos/`. The requirements doc already states this
correctly ("the site's build/deploy must consume already-published media
...not regenerate it per deploy") but doesn't yet say *fetch into the build*
vs. *commit into the repo* — Phase 3 should make that explicit in the task
breakdown, since both satisfy "don't regenerate per deploy" but only one
avoids the git-history-growth failure mode.

## 2. GitHub Pages + GitHub Actions deploy pitfalls

### 2a. Verified current repo state (not generic risk — checked live)

```
$ gh api repos/tstapler/stapler-squad/pages
build_type: "workflow", source: {branch: "main", path: "/"}, html_url: "https://tstapler.github.io/stapler-squad/"
```

Pages is **already** configured with `build_type: workflow` — this answers
one of `requirements.md`'s Open Questions ("whether the existing Pages
'enabled but broken' setting needs a Pages-settings API change vs. just
adding a workflow file"). **No Pages-settings change is needed**; a
conforming `actions/upload-pages-artifact` + `actions/deploy-pages` workflow
should be picked up as-is. The 404 today is explained fully by "no workflow
exists yet," not by any settings mismatch.

```
$ gh api repos/tstapler/stapler-squad/environments
"github-pages": protection_rules: [{type: "branch_policy"}], deployment_branch_policy: {custom_branch_policies: true}
$ gh api repos/tstapler/stapler-squad/environments/github-pages/deployment-branch-policies
branch_policies: [{name: "main", type: "branch"}]
```

The `github-pages` environment already exists, already restricts deploys to
the `main` branch, and — checked explicitly — carries **no required-reviewers
rule** (only the branch policy). This rules out the generic
"environment-protection-rules silently block the first deploy pending manual
approval" pitfall for this repo specifically: there is no reviewer gate to
get stuck behind. The one real constraint this confirms: **the deploy job
must run on push to `main`** (or trigger with `main` as the ref) — a
workflow that deploys from a PR branch or any other ref will be rejected by
this existing branch policy with a "branch not allowed to deploy to this
environment" error. This matches the requirement ("deploys automatically on
push to main") so no design change is needed — just confirms the workflow's
`on:` block and environment ref must target `main`.

### 2b. Generic pitfalls to design against (not yet verified against this repo's exact workflow, since it doesn't exist yet)

- **Missing `permissions:` block.** The deploy job needs `pages: write` and
  `id-token: write` on the job (or workflow) — without both, `deploy-pages`
  fails with a permissions/OIDC error. `demo-publish.yml`'s `publish-gifs`
  job already demonstrates the pattern of declaring job-scoped `permissions:`
  explicitly (`contents: write` there) — the new deploy workflow should do
  the same with `pages: write`/`id-token: write`.
- **Artifact must come from `actions/upload-pages-artifact`.** `deploy-pages`
  expects the specific artifact format/name that action produces; a
  hand-rolled `actions/upload-artifact` won't work as the deploy source.
- **Environment name.** `actions/deploy-pages`'s own README recommends (but
  doesn't strictly require) naming the environment `github-pages` — this
  repo's environment is already named exactly that, so no mismatch risk.
- **Concurrency.** Multiple pushes to `main` in quick succession can race
  two Pages deployments; `e2e-video.yml`'s existing
  `concurrency: {group: ..., cancel-in-progress: true}` pattern is the
  established convention in this repo for exactly this kind of race —
  the new deploy workflow should adopt the same `concurrency:` group
  (standard practice is `group: "pages"`, `cancel-in-progress: false`,
  since cancelling a half-published Pages deploy can leave the site in an
  inconsistent state — differs from the video workflow's `cancel-in-progress: true`
  precedent, which is safe there only because stale video artifacts have no
  live-user-facing cost).

### 2c. Base-path / subpath pitfall (real and must be designed around)

The site has no custom domain, so it serves from
`https://tstapler.github.io/stapler-squad/` — a **subpath**, not a root
domain. Every candidate SSG in `requirements.md`'s Alternatives Considered
list has a distinct knob for this that must be set correctly or assets
404/links break when deployed (works fine in local dev at `/`, breaks only
once deployed under `/stapler-squad/` — an easy miss since local dev never
exercises it):

| SSG | Knob | Common failure mode if unset/wrong |
|---|---|---|
| Next.js (static export) | `basePath` + `assetPrefix` in `next.config.js` | `basePath` fixes page links; `assetPrefix` fixes CSS/JS/`_next/` asset URLs — **both** are needed, and files placed in `public/` are not automatically prefixed by either, so hand-written `<img src="/logo.svg">` references need a repo-name-aware helper/env var, not literal root-relative paths. Also pair with `images: { unoptimized: true }` for static export since the default Image Optimization API needs a server GitHub Pages doesn't provide. |
| Docusaurus | `baseUrl: '/stapler-squad/'` in `docusaurus.config.js` | Build-time broken-link checking (`onBrokenLinks: 'throw'` by default) catches most link mistakes at build time, which is a net positive — but it has a known false-positive mode on links into content Docusaurus didn't generate itself (e.g. a path into raw static files), worth knowing before treating every broken-link failure as a real bug. |
| Astro | `base: '/stapler-squad/'` in `astro.config.mjs` | Internal links/assets not routed through Astro's `base`-aware helpers (`Astro.url`, the `base` relative import attribute) silently resolve against `/` instead of `/stapler-squad/`. |
| Jekyll | `baseurl: /stapler-squad` in `_config.yml` | Native GitHub Pages support normally injects this correctly for a project-site build, but any Liquid template using a literal `/assets/...` instead of `{{ "/assets/..." | relative_url }}` bypasses it. |

**Design implication for Phase 3**: whichever SSG Agent 1 recommends, the
plan's task breakdown must include an explicit task/AC for setting the
subpath knob **and a verification step that loads the deployed URL (not just
`localhost`)** before calling any page/asset-loading work done — subpath
bugs are invisible in local dev by construction.

## 3. Content-curation pitfalls

- **Stale/broken links after curation.** The curated site will copy or
  adapt content from `docs/how-to/*.md` et al. There is **no automated
  staleness check today** — confirmed by reading
  [`docs/how-to/debug-with-logs.md`](../../../docs/how-to/debug-with-logs.md)
  and [`docs/how-to/enable-otel-auto-instrumentation.md`](../../../docs/how-to/enable-otel-auto-instrumentation.md):
  both use plain relative Markdown links with explicit `../../` traversal and
  `#anchor` fragments, e.g. `[spike-verdicts.md](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md)`
  and `../../CLAUDE.md#manual-dev-port-block`. These links resolve correctly
  *inside the repo's own GitHub-rendered Markdown*, but:
  - If curated content is copy-pasted into the new site rather than
    build-time-included, every such relative link needs rewriting (the new
    site's file tree doesn't mirror `docs/`'s tree), and nothing currently
    re-checks that rewrite when the *source* `docs/*.md` file is later
    renamed/restructured — this is exactly the "stale/broken links" risk
    `requirements.md`'s Rabbit Holes section flags.
  - Several of these cross-links point at files explicitly **out of
    scope** for the public site (`project_plans/`, `CLAUDE.md` — both listed
    under `requirements.md`'s Out of Scope). A naive copy of a how-to file
    that keeps such a link produces either a dead link (404 on the public
    site, since those files aren't published) or, worse, an accidental leak
    of internal-only content if the link target also gets copied by mistake.
  - **Recommended mitigation for Phase 3**: treat curated docs as
    build-time-included (e.g. read the real `docs/*.md` file at build time
    and transform it, rather than hand-copying prose) so a future rename of
    the source file is a build break, not a silent stale link — and add an
    explicit link-checker step (most SSGs ship one, e.g. Docusaurus's
    built-in `onBrokenLinks`) to the site's CI so this is caught before
    deploy rather than discovered by a user 404.
- **Front-matter/convention mismatch.** The files sampled use no YAML
  front-matter at all — just an H1 and prose — so there's no
  title/description metadata to extract automatically; whichever SSG is
  chosen will need either (a) a per-page front-matter block added during
  curation (extra curation work, not automatic), or (b) a convention of
  deriving the title from the first H1, which works for the two files
  sampled but should be spot-checked across the full curated-file list in
  Phase 3, not assumed universal.
- **Judgment-call files are genuinely ambiguous**, matching
  `requirements.md`'s own flagged example: `debug-with-logs.md` is
  reasonably user-facing (a real user with a local install could use `jq`
  on their own log file), while a neighboring file like
  `run-nil-safety-analysis.md` is unambiguously dev-only (NilAway, `go vet
  -nilness` — nothing a non-contributor would ever run). Confirms Phase 3's
  planned "explicit file-by-file include/exclude list" is the right
  mechanism; a blanket rule (e.g. "include everything in `docs/how-to/`")
  would wrongly pull in developer-only content.

## 4. Automated-recording pipeline pitfalls

- **Trigger discipline is the load-bearing control, and it's already a
  literal, structural pattern in this repo** — not just a convention to
  remember. Confirmed by reading both workflows' `on:` blocks:
  - `demo-publish.yml`: `on: workflow_dispatch:` only — no `push`, no
    `pull_request`.
  - `e2e-video.yml` (the other Playwright-video-producing workflow in this
    repo, used for PR review, not publishing): `on: pull_request: branches:
    [main]` — deliberately different, and its own header comment explains
    why PR-triggered video capture is fine for that workflow specifically
    (ephemeral `upload-artifact` with `retention-days`, never published
    anywhere durable) while `demo-publish.yml` is manual-only because its
    output is durable (a Release asset).
  - **Direct implication for the new "higher production value" recording
    workflow**: it must copy `demo-publish.yml`'s `on: workflow_dispatch:`
    pattern exactly (no `push`/`pull_request` trigger at all), because its
    output is also durable (new Release assets or equivalent). This is
    exactly what `requirements.md`'s Constraints section already specifies,
    and it's enforceable as a near-mechanical review check in Phase 6: diff
    the new workflow's `on:` block against this one.
- **Flaky-spec risk is pre-acknowledged and pre-mitigated** by the existing
  curated `FEATURE_SPECS` list (both `demo-publish.yml` and `e2e-video.yml`
  explicitly exclude `terminal-flickering.spec.ts`,
  `accessibility.spec.ts`, and `smoke.spec.ts` — the first for being a
  stress/benchmark spec prone to timing flakiness, not a demo-worthy flow).
  `requirements.md`'s Rabbit Holes section already directs Phase 3 to reuse
  this exact `FEATURE_SPECS` set as the starting scope — right call; a new
  pipeline that records specs outside this curated set reintroduces the
  flakiness risk this list exists to avoid.
- **Timing-dependent framing/pacing risk** (new, not yet mitigated by
  existing workflows): "higher production value... better framing/pacing"
  implies new camera/zoom/crop logic, which is inherently timing-sensitive —
  a slow CI runner producing a different frame count/pacing than a fast one
  is a classic source of visually-inconsistent output between runs (not a
  test *failure*, just an inconsistent-looking artifact, which is easy to
  ship unnoticed since nothing fails red). Phase 3 should scope "higher
  production value" to overlays/branding that are *not* wall-clock-timing
  dependent (e.g. a static branded frame/border, caption text keyed to
  Playwright actions rather than elapsed time) rather than anything requiring
  precise sub-second choreography, to avoid inheriting this risk.
- **Growth-back-into-the-original-problem risk**, named explicitly in the
  task brief: the structural control that prevents this is the
  `workflow_dispatch`-only trigger (point 1 above) *combined with* publishing
  to a release-asset pattern rather than committing — both of which are
  already proven, existing patterns in `demo-publish.yml` to copy forward,
  not new designs to invent. The risk is real only if the new workflow
  diverges from either pattern (e.g. adds a scheduled `cron:` trigger "to
  keep demos fresh," or a well-meaning contributor later adds a `push`
  trigger) — worth a one-line comment in the new workflow file itself (as
  `demo-publish.yml` already does) explaining why it's dispatch-only, so a
  future editor doesn't reintroduce the original incident.

## 5. Lighthouse / accessibility pitfalls

- **This repo's own existing Lighthouse CI convention is lower than the new
  target.** Checked [`web-app/lighthouserc.json`](../../../web-app/lighthouserc.json):
  the product's own web UI gates at `"warn"` severity (not a hard failure)
  with `performance ≥ 0.70`, `accessibility ≥ 0.80`, `best-practices ≥ 0.80`
  — all below the ≥90 target `requirements.md` sets for the new marketing
  site. `requirements.md`'s own framing ("same bar is reasonable here") is
  describing *using Lighthouse CI as a mechanism*, not matching its
  *thresholds* — worth flagging explicitly in Phase 3 so the plan doesn't
  accidentally inherit the lower numeric bar by copying `lighthouserc.json`
  wholesale. A static marketing/docs site with no app shell/JS-heavy runtime
  has a materially easier path to ≥90 than the actual Next.js SPA product
  does, so the higher target is realistic, but the config needs its own
  thresholds, not a copy of the web-app's.
- **Animated GIFs are a known, well-documented Lighthouse performance
  risk** (general web-performance knowledge, not specific to this repo):
  an `<img>`-embedded GIF is dramatically less efficient than an equivalent
  muted/autoplay/loop `<video>` (webm/mp4) of the same content — GIF has no
  modern inter-frame compression, so file sizes are routinely 5-10x larger
  for the same visual content, which directly costs LCP/page-weight points.
  Given `requirements.md` already specifies demo media should be "higher
  production value" and sourced from Release assets, Phase 3 should specify
  **video embeds (webm with GIF/poster-image fallback), not raw animated
  GIFs**, for anything beyond the already-committed small `assets/demo.gif`.
  This also naturally supports `loading="lazy"` / deferred autoplay for
  any demo below the fold, without the common mis-application pitfall of
  lazy-loading an above-the-fold hero asset (which delays LCP instead of
  helping it — hero/first-viewport media should never carry `loading="lazy"`).
- **Alt text and color contrast** are the two other most common
  Lighthouse/axe-core accessibility failures for a content-heavy static
  site: every demo GIF/video and every brand-asset `<img>` (the
  `logos/concepts/*.svg`, `web-app/public/logo.svg` assets
  `requirements.md` calls out for reuse) needs real alt text, and any
  reused brand color pairing (text-on-background from the existing brand
  palette) should be contrast-checked before reuse on a bare marketing page
  — the existing `web-app` theme was designed for an app UI with its own
  surrounding chrome/contrast context, which doesn't automatically carry
  over to a plain white/light marketing background.
- **Decorative vs. meaningful images**: per general Lighthouse/axe guidance,
  a purely decorative image (background pattern, divider) should carry
  `alt=""` explicitly rather than omitting the `alt` attribute — an image
  with no `alt` attribute at all is flagged by axe-core regardless of
  whether it's decorative, while `alt=""` is accepted as an explicit,
  correct signal.

## Summary of top risks (for the 3-bullet handback)

1. Subpath (`basePath`/`baseUrl`/`base`/`baseurl`) misconfiguration is the
   single most likely "works locally, breaks live" bug class for this
   project, and it's invisible until you load the deployed URL — Phase 3
   needs an explicit AC that verifies against the live
   `https://tstapler.github.io/stapler-squad/` URL, not just local dev.
2. Content curation has no structural staleness protection today (plain
   relative-link Markdown, no front-matter, no link checker) — recommend
   build-time inclusion of source `docs/*.md` content plus an automated
   broken-link check wired into the new site's CI, rather than hand-copied
   prose that silently rots when source files move.
3. Both "don't regenerate media per deploy" and "demo-recording workflow is
   dispatch-only, never push/PR-triggered" are already proven, working
   patterns in this exact repo (`demo-publish.yml`) — the risk is a future
   editor diverging from them (e.g. adding a `push`/`cron` trigger), not a
   gap in current understanding; the new workflow should copy the pattern
   verbatim plus the explanatory header comment, and Phase 6 review should
   diff the new workflow's `on:` block against `demo-publish.yml`'s as a
   near-mechanical check.
