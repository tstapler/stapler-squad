# Architecture Research: stapler-squad-website

Scope note per dispatch: this is a brand-new subsystem (no legacy code to
assess), so this doc covers the architecture of the thing being built, not a
hotspot/disposition analysis. EventStorming table skipped — single-actor,
content-delivery system, not a business domain.

## 1. Integration points with the existing repo

### Where the site's source lives

**Recommendation: new top-level `website/` directory**, sibling to `web-app/`
and `docs/`, not nested under either.

- Not under `docs/`: `docs/` is reserved for the Diataxis hierarchy
  (`docs/how-to`, `docs/reference`, `docs/explanation`) plus a pile of
  artifact-type folders (ADRs, `project_plans/`, architecture audits) — see
  `docs/README.md`. Mixing a Next.js/Astro/Docusaurus *application* into that
  tree would conflate "documentation content" with "a buildable app," and
  would make `docs/README.md`'s existing map wrong the day it landed.
- Not reusing `web-app/`: `web-app/` is the product's own SPA — a different
  framework-usage shape (Next.js dev server + Go-binary-embedded output,
  ConnectRPC client code, vanilla-extract theming wired to the actual app's
  design tokens) with its own ambitious build pipeline
  (`make install-service` builds it as a step, not standalone). A marketing
  site has a different deploy target (GitHub Pages static export, not
  embedded-in-Go-binary) and a different lifecycle (deploys on every `main`
  push vs. only on `make install-service`). Bolting it on as a second
  `output: "export"` target inside `web-app/next.config.ts` would make that
  config's `output`/`basePath`/`trailingSlash` settings (currently tuned for
  embedding, e.g. `trailingSlash: true` for the Go embed FS) do double duty
  for two different deployment targets — fragile the first time one target's
  needs diverge from the other's.
- A new top-level dir mirrors the existing pattern of `web-app/` as an
  independent, self-contained package with its own `package.json`,
  `node_modules`, and build commands, landing a `website/package.json` with
  its own lockfile. Since the root has no `pnpm-workspace.yaml` (confirmed —
  `web-app/` is a standalone package, not a pnpm workspace member), `website/`
  should follow the same standalone-package convention rather than introducing
  a monorepo workspace file as a side effect of this project.

### What the site reads from the existing repo vs. owns natively

| Content | Source of truth | Site's relationship to it |
|---|---|---|
| Landing page copy/layout, hero section | Net new | Owned natively in `website/` |
| Feature highlights | README's `### Highlights` section (`README.md:14-39`) | Transformed/copied once, not live-read (see below) |
| Feature tour imagery | `docs/features.md` + `assets/demo-recording/*.png` | Curated subset copied in, not the whole file verbatim |
| Install instructions | README's `### Installation` section | Adapted copy, since the site's CTA is public-audience-facing and the README's is dev-audience-facing (mentions `brew tap`, build-from-source, etc. — some of that is fine for both) |
| Help docs | Curated subset of `docs/how-to/*.md` (per Phase 3's file-by-file list) | Copied + transformed (frontmatter added, internal-only cross-references stripped) |
| Brand assets | `web-app/public/logo.svg`, `logos/iterations/*.svg` (or `logos/concepts/*.svg`) | Copied into `website/public/` (or equivalent) at author time, not symlinked |
| Demo media | GitHub Release assets (`demo-gifs-latest`, see §2) | Referenced by URL, never copied into the repo |

### Copy/transform vs. symlink/live-reference — recommendation

**Copy-and-transform at a defined content-sync point, never symlink and never
read `docs/` live during the site's build.** Concretely: a small script
(`website/scripts/sync-content.mjs` or similar, invoked as an explicit
pre-build step, e.g. `pnpm run sync-content && next build`) that:

1. Reads a maintained manifest (the Phase-3-produced file-by-file include
   list) mapping `docs/how-to/<file>.md` → `website/src/content/help/<file>.md`.
2. Copies each file, applying a transform pass: strip/rewrite internal-only
   cross-references (links to `project_plans/`, ADRs, other dev-only docs),
   add SSG-required frontmatter (title, description, slug), and flag (fail
   the build, or `console.warn` loudly) if a referenced source file no longer
   exists — this is the mechanism that keeps "curated subset" honest over
   time instead of silently drifting from `docs/` as that tree evolves.
3. Writes the result into the site's own content directory, which is
   version-controlled (committed), not generated fresh on every build and
   discarded — so the site's content directory is auditable in a PR diff
   independent of the source `docs/` files, and reviewable the same way any
   other content change is.

Why not a live symlink or live read from `docs/` at build time:
- The requirements explicitly frame this as "curated subset, not a mirror"
  (Scope, line 47) — a symlink or live include is definitionally a mirror: any
  edit to the source file propagates untransformed and unreviewed. A
  dev-facing aside added to `docs/how-to/debug-with-logs.md` for an unrelated
  reason would ship to the public site with no review step in between.
- Symlinks also don't survive `actions/checkout` + Node tooling cleanly across
  OSes/CI runners the way a plain committed file does, and most SSGs'
  file-watchers/content-collection loaders (Astro content collections, VitePress,
  Docusaurus) expect real files under their configured content root, not
  symlink targets outside it — using one would fight the chosen SSG's own
  content-loading conventions (Agent 1's SSG pick) rather than working with
  them.
- A one-time manual copy-paste (no script) was considered and rejected: it has
  no mechanism to catch drift (a `docs/how-to/*.md` file renamed or deleted
  upstream silently orphans the copy) and no re-run story when Phase 3's
  curation list changes incrementally over time. A script costs little more
  to write and removes an entire class of "oops, forgot to re-sync" bugs.

This sync step runs manually/on-demand (a maintainer runs it, reviews the
diff, commits it) — **not** as part of the deploy-on-push-to-`main` workflow
in §3. Running it automatically on every push would reintroduce exactly the
untransformed-live-mirror problem above, just on a cron instead of instantly.

## 2. Media pipeline integration

Read `.github/workflows/demo-publish.yml` in full (152 lines). Key structural
facts this design must mirror/extend:

- `workflow_dispatch` only, two-job shape: `record-demos` (matrix-sharded
  Playwright recording → ffmpeg GIF conversion → `upload-artifact`) feeding
  `publish-gifs` (`needs: record-demos`, downloads both shards, `gh release
  upload demo-gifs-latest ... --clobber`).
  - Mutable release: `demo-gifs-latest` is a non-"latest" (`--latest=false`),
    perpetually-overwritten release. `gh release upload --clobber` replaces
    same-named assets in place, so **the download URL per asset name is
    stable across runs**:
    `https://github.com/tstapler/stapler-squad/releases/download/demo-gifs-latest/<name>.gif`.
  - `FEATURE_SPECS` env var is the single curated list (5 specs today) that
    both this workflow and `e2e-video.yml` share, deliberately excluding
    flaky/noisy specs (`terminal-flickering`, `accessibility`, `smoke`).

### How the site knows which Release/asset URLs to embed

**Recommendation: hardcode the stable `demo-gifs-latest` tag + per-clip
filename convention directly in site content, no manifest file and no
build-time GitHub API call.**

Rationale, ruling out the two alternatives named in the task:

- **A generated manifest file checked into the site's content dir** — adds a
  moving part (something has to generate and commit it) for a problem that's
  already solved by the asset-name-is-the-identity convention
  `demo-publish.yml` already established. The filenames derive deterministically
  from the Playwright spec/test names (`$(basename "$(dirname "$webm")")` with
  suffix stripped) — i.e. they're already a de facto stable manifest, just one
  that lives in the recording workflow's own output convention rather than a
  separate file. A hand-maintained manifest duplicates that convention and can
  drift from it (asset renamed in the workflow, manifest not updated → broken
  image on the site with no build-time signal).
- **Reading via GitHub API at build time** — directly contradicts the
  constraint that the site's build must only *consume* already-published
  assets, never touch the recording/publishing pipeline's live state. An API
  call at build time also adds a network dependency and rate-limit exposure
  to every deploy, for data (which clips exist) that changes only when someone
  manually re-runs `workflow_dispatch` — i.e. rarely, and under direct human
  control at exactly the time they'd also update the site content referencing
  it.
- **Hardcoded URL convention** (chosen): the site's content directly embeds
  `https://github.com/tstapler/stapler-squad/releases/download/demo-gifs-latest/<clip-name>.<ext>`
  per clip, the same way `README.md` today hardcodes `assets/demo.gif` as a
  relative path. This is a plain `<img>`/`<video>` src with no build-time
  fetching, no API dependency, no extra workflow step — the simplest thing
  that satisfies "consume already-published URLs, never regenerate them."
  Cost: if a clip is renamed/removed in a future `demo-publish.yml` edit, the
  site's hardcoded reference breaks silently (404 on that one asset, not a
  build failure) until someone notices. That's an acceptable, low-probability
  failure mode for a site with ~5-10 embedded clips, cheaper than carrying a
  manifest-generation step for a name set that changes maybe once a quarter.
  If this surfaces as a running problem, the manifest mitigation is a small,
  additive follow-up — not a pre-mortem blocker — e.g. a cheap CI check that
  does one `curl -sI` per embedded URL and fails the build on a 404, rather
  than a full manifest-generation pipeline.

### New demo-recording workflow for the higher-production-value clips

Extend `demo-publish.yml`'s pattern as a **separate** new workflow (e.g.
`.github/workflows/website-demo-publish.yml`), not a modification of
`demo-publish.yml` itself, because:
- `demo-publish.yml`'s existing release (`demo-gifs-latest`) and consumer
  (README's `assets/demo.gif`/`.webm`, which are committed small files, not
  release assets — note `demo-publish.yml` doesn't currently feed the
  README's embeds at all) should stay decoupled from the new, higher-fidelity
  site-specific clips, which need a new release tag (e.g.
  `website-demo-gifs-latest`) to avoid naming collisions/overwrites against
  the unrelated existing release.
- The "branding overlays, better pacing/framing" production-value work (ffmpeg
  filter changes, possibly a watermark/overlay compositing step) is additive
  scope that's easiest to review and roll back independently of the existing,
  working `demo-publish.yml` if Phase 3 scope-bounds it tightly (per the
  Rabbit Holes note to reuse `FEATURE_SPECS` as the starting scope, not invent
  new flows).
- Both workflows share the `workflow_dispatch`-only, manual-trigger-only
  constraint and the "publish to Release assets, never commit binaries"
  constraint — this is a copy of the existing pattern's job/permissions shape
  (`permissions: contents: write` on the publish job only), not a new
  mechanism.

## 3. Deploy workflow data flow

Confirmed via GitHub's own docs
([using-custom-workflows-with-github-pages](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages),
fetched 2026-10-14) — the canonical single-job shape is:

```yaml
permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: "pages"
  cancel-in-progress: false

jobs:
  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/configure-pages@v5
      - name: Build
        run: |
          cd website
          pnpm install --frozen-lockfile
          pnpm run build   # -> website/out (or /dist, per Agent 1's SSG pick)
      - uses: actions/upload-pages-artifact@v4
        with:
          path: website/out
      - name: Deploy to GitHub Pages
        id: deployment
        uses: actions/deploy-pages@v4
```

Trigger: `on: push: branches: [main]` (paths-filtered to `website/**` plus
whichever `docs/**`/`assets/**` paths feed the content-sync step, per §1, to
avoid a wasted deploy run on unrelated commits — mirrors `build.yml`'s general
pattern of scoping workflows to relevant paths rather than running on every
push repo-wide, though note `build.yml` itself doesn't path-filter and this
is a new judgment call for the website workflow specifically).

Notes on the pieces:
- `pages: write` + `id-token: write` are the two deploy-job permissions the
  official docs specify; `id-token: write` exists so the OIDC token proves the
  deployment's provenance to GitHub's Pages backend (per `actions/deploy-pages`'s
  own README, surfaced in the same search).
- The `github-pages` environment is auto-created by GitHub on first successful
  deployment if it doesn't exist; declaring it explicitly in the job is what
  lets the job's summary surface the live URL (`steps.deployment.outputs.page_url`)
  and is also where any future deployment-protection rules would attach — not
  needed functionally for a public, unprotected site, but it's the documented
  shape and costs nothing to include.
- `actions/configure-pages@v5`'s role (per the same docs fetch) is to confirm
  Pages is enabled for the repo and gather site metadata for the artifact
  upload/deploy steps; it does **not** regenerate or touch content — it's a
  metadata/enablement step, not a build step. Keep it immediately before the
  build step (order doesn't strictly matter since it doesn't depend on build
  output, but this matches the docs' example ordering).
- `actions/upload-pages-artifact@v4` and `actions/deploy-pages@v4` should be
  separate steps in the **same job** here (not separate jobs with a `needs`
  dependency) — the official docs' single-job example does this, and splitting
  across jobs only matters when you want the build and deploy to be
  independently retriable/cancelable, which this simple a site doesn't need.
  (The alternative two-job split is also valid and is what `demo-publish.yml`
  uses for its own record/publish split — either shape is fine; single-job is
  simpler for a site with no fan-out/sharding need.)

## 4. Fixing the broken Pages config

**Verified live state** (this session, `gh api repos/tstapler/stapler-squad/pages`):

```json
{"url":"...","status":null,"cname":null,"custom_404":false,
 "html_url":"https://tstapler.github.io/stapler-squad/",
 "build_type":"workflow","source":{"branch":"main","path":"/"},
 "public":true,"https_enforced":true}
```

`build_type` is already `"workflow"` — i.e. **the Pages publishing source is
already set to "GitHub Actions," not "Deploy from a branch."** `status: null`
means no deployment has ever run/succeeded (there is currently no workflow at
all that calls `actions/deploy-pages`, confirmed by `grep -rln
"actions/deploy-pages\|actions/upload-pages-artifact" .github/` returning
nothing). The `source.branch`/`source.path` fields are a vestige of the
branch-based-deploy schema and are ignored when `build_type` is `"workflow"`
(per GitHub's Pages API docs — the `source` object only takes effect for
`build_type: "legacy"`).

**Disposition: adding the `actions/deploy-pages`-based workflow is sufficient
on its own. No separate Pages-settings API/UI change is needed first**, for
these reasons:
1. The live API response already shows the source is `workflow`-mode — this
   is exactly the state a one-time "switch source to GitHub Actions" UI change
   would produce. Whatever configured it that way (plausibly an earlier,
   abandoned attempt, or simply GitHub's default for a repo that never
   configured branch-based Pages), it's already correct for what this project
   needs — there's nothing left to flip.
2. `actions/configure-pages@v5`'s own documented behavior (its `enablement`
   input, default `true`) is to enable Pages automatically on first run if
   it isn't already enabled — i.e. even in the counterfactual where the
   source were still unset, the first run of the new workflow would fix it
   with no manual settings step. Since it's already in `workflow` mode here,
   this is a no-op safety net, not a required fix.
3. The 404 today is fully explained by "zero deployments have ever
   succeeded because no workflow producing one exists" — `status: null` in
   the API response is the direct signature of that, not of a misconfigured
   source.

**Residual risk** (worth a one-line pre-mortem note, not a blocker): if the
first run of the new workflow still fails to deploy despite this, the
fallback is the manual UI path — Settings → Pages → Build and deployment →
confirm "GitHub Actions" is selected as the source (it already should read
that way given `build_type: workflow`) — a 30-second check, not a structural
redesign, if it's ever needed.

## Summary of key architectural decisions

| Decision | Choice |
|---|---|
| Site source location | New top-level `website/` directory, standalone package (no root pnpm workspace) |
| Docs content sourcing | Copy-and-transform via a committed sync script output, run on-demand — never symlink/live-read `docs/` |
| Demo clip URL resolution | Hardcoded `releases/download/<tag>/<name>.<ext>` URLs in site content — no manifest file, no build-time GitHub API call |
| New demo-recording workflow | Separate `workflow_dispatch`-only workflow + separate release tag, mirroring `demo-publish.yml`'s job shape, not a modification of it |
| Deploy workflow shape | Single job: checkout → `configure-pages` → SSG build → `upload-pages-artifact` → `deploy-pages`, permissions `contents: read, pages: write, id-token: write`, `environment: github-pages` |
| Pages config fix | Add the workflow only — `build_type` is already `"workflow"` per live API check; no Settings/API source change needed |
