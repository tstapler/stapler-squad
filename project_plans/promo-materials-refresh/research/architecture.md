# Architecture Research: promo-materials-refresh

Builds on `project_plans/stapler-squad-website/research/architecture.md` (call it **PRIOR**). Confidence labels: VERIFIED = file opened or command run this session; INFERRED = reasoned, not run.

## 0. What PRIOR got right, and what the code has since overtaken

| PRIOR claim | Status now |
|---|---|
| Site lives in standalone top-level `website/` (PRIOR:12-40) | VERIFIED holds: `website/` has its own `package.json`, `bun.lock`, `astro.config.mjs`. |
| Copy-and-transform docs via `sync-content`, never live-read (PRIOR:54-97) | VERIFIED holds: `website/scripts/sync-content.mjs` + `content-manifest.json` (117 lines, `docs/how-to/*.md` -> `help/<slug>`). Deploy runs `sync-content -- --check` (`deploy-website.yml:127-129`). |
| Demo media = hardcoded `releases/download/demo-gifs-latest/...` URLs, no manifest (PRIOR:119-157) | **Superseded.** `asset-health-manifest.json` (26 sha256 entries) and `check-asset-health.mjs:1-100` exist and the deploy job runs it (`deploy-website.yml:166-168`). The 26 manifest entries are test-recording GIFs (`review-queue-*`, `session-lifecycle-*`), not the clips the site now plays. Treat the manifest as legacy for release-asset refs. |
| Media goes in a *separate* `workflow_dispatch` workflow + new release tag (PRIOR:159-181) | **Superseded.** Clips are recorded inside `deploy-website.yml` (`record-clips` job, `:45`) and shipped in the Pages artifact, no release tag. |
| Deploy is single-job (PRIOR:183-252) | **Superseded.** Now 4 jobs: `record-clips` -> `build` -> `deploy` -> `notify` (`deploy-website.yml:45,97,226,240`), weekly cron `17 6 * * 1` (`:27-28`). |
| "Pages source needs fixing" (PRIOR:254-300) | Moot; site deploys. |

Implication: the media pipeline to extend is the **in-workflow, Pages-artifact** one. PRIOR's release-asset design is only relevant for the open README-embed question (section 7).

## 1. Current media data flow (VERIFIED)

```
scripts/record-website-demos.sh                      (120 lines)
  go run ./tests/demo/seed $DEMO_DIR 6               seed N mock sessions into a temp dir      (:55)
  PORT=<free> STAPLER_SQUAD_TMUX_SOCKET=ssq-website-demos-$$ STAPLER_SQUAD_USE_CONTROL_MODE=false
    ./stapler-squad --test-mode --test-dir $DEMO_DIR --tmux-keep-server &                      (:58-61)
  wait for $TEST_SERVER_URL/  (300s)                                                           (:63-72)
  cd tests/e2e && WEBSITE_DEMO_OUT=... npx playwright test -c playwright.website-demos.config.ts (:77-79)
      tests/e2e/website-demos.spec.ts  loops CLIPS from website-demos/clips.ts -> recordClip()
      recorder.ts: new context, video on, 1280x720, localStorage seed (onboarded, quietMode, theme),
                   saveAs <out>/<name>.webm + <name>.meta.json {title, trimStartMs}
  per clip: demo-post-process.sh --no-gif --title-card <title> --trim-start-frames N            (:83-100)
  drop clips <= 2s; print sha256 per clip; exit 3 on partial failure                            (:103-119)
deploy-website.yml
  record-clips (45 min): checkout, Go, Bun, prepare, go build, playwright cache, ffmpeg+librsvg,
      run script, upload-artifact website-demo-clips (*.webm)                                  (:45-95)
  build: download-artifact -> website/public/clips; warn (not fail) if no .webm (:138-152);
      node scripts/build-features.mjs (injects demoClip into features.json) (:154-156);
      bun run build; check-internal-links; check-asset-health; Lighthouse x2; axe; UX e2e;
      upload-pages-artifact (:158-224)
  deploy -> notify (opens tracking issue if record-clips failed, :256-272)
```

Key facts and gaps:
- `clips.ts:1-71` defines **2 clips** (`session-dashboard`, `auto-approval-rules`); `build-features.mjs:102-106` maps clip names to card headings via `DEMO_CLIP_BY_HEADING`, and attaches a clip only if the file exists (`:284-288`). A missing clip silently keeps the screenshot: this is the "silent fallback to stale media" the requirements forbid (requirements.md "Observability").
- Screenshots are **committed**: `git ls-files` shows `assets/demo-recording/01..08-*.png`, `assets/demo.gif`, `assets/demo.webm`, `assets/screenshot.png`, `website/public/demo.webm`, `website/public/screenshot.png`, `website/public/features/01..08-*.png` (74 tracked png/gif/webm/jpg repo-wide, including logos/icons/visual-regression snapshots). `build-features.mjs:190-211` resolves screenshots from `docs/features.md` image refs into `website/public/features/` and throws if the file is missing, so the committed PNGs are load-bearing today. Landing hero uses `/stapler-squad/demo.webm` + `screenshot.png` (`website/src/content/docs/index.mdx:25`); README embeds `assets/demo.gif` and `assets/demo.webm` (`README.md:5,9`).
- `features.json` is committed and verified current by CI (`node build-features.mjs && git diff --exit-code`, `deploy-website.yml:131-133`) but the deploy then regenerates it with clips (`:154-156`) - two states of one file, only the first checked into git.
- Screenshots (still PNGs) have no recorder path at all; only videos do. `recorder.ts` has no `page.screenshot` helper.
- No freshness check: nothing ties the shipped clip to the deployed commit (requirements.md success metric 5).
- `check-asset-health.mjs` only validates `ReleaseAssetRef` URLs in `dist` (`:60-65`, `extractReleaseAssetUrls`); it does not look at `/clips/*.webm`.

## 2. Seeded isolated server (VERIFIED)

- Seeder: `tests/demo/seed/main.go` (39 lines) -> `demo.SeedDirectory(dir, count)` (`tests/demo/helpers.go:145`) -> `mockSessions()` (`:157`): 6 hard-coded `session.InstanceData` records (paths like `/Users/dev/services/payment-service`, statuses Paused/Active only, comment at `:152-155` explains why, so the review-queue poller does not reclassify them).
- Isolation: `--test-mode --test-dir`, private tmux socket `STAPLER_SQUAD_TMUX_SOCKET`, free port. This is the same pattern CLAUDE.md prescribes for manual instances. Control mode off (`STAPLER_SQUAD_USE_CONTROL_MODE=false`).
- Limits relevant to requirements: seeds **sessions only** - no approval rules beyond built-ins, backlog items, PR state, escape analytics, insights, notifications, tags beyond the six. Realistic seed paths (`/Users/dev/...`) are fictional, which satisfies the "no real paths" security note; keep that invariant.
- Date skew: timestamps are `time.Now()`-relative (`helpers.go:158`), so "20 seconds ago" is stable relative to the run but not frozen. Determinism for screenshots needs either Playwright `page.clock` or masking time elements.
- Feature flags: the live app gates routes with `FeatureGuard` (`web-app/src/app/FeatureGuard.tsx`; `route-meta.ts` `GuardFlag`). The harness has no flag-enable step. INFERRED: flags are settable via the feature-flag RPC (`docs/registry/features/backend/feature-flag/`); the recorder should call it (or seed config) before capture. Needs a spike.

**Recommended seam:** extend the seeder rather than the script. Make `go run ./tests/demo/seed <dir> <scenario>` select a named fixture set (`dashboard`, `backlog`, `insights`, ...), and keep `mockSessions()` as the `default` scenario so the existing 2 clips do not change. Go matches the "new scripting in Go" constraint.

## 3. Where tour definitions should live

Options:
1. Extend `clips.ts` (TS, Playwright scenes) with tour metadata.
2. Content-collection frontmatter drives recording.
3. **Split by concern (recommended):** one **tour manifest** as the single source of truth for *identity and prose*, and recorder scenes as code keyed by tour id.

Rationale: scenes are imperative Playwright code and must live under `tests/e2e/` (Playwright config, conventions: `// @feature` header, no `waitForTimeout`, `data-testid`/ARIA locators - CLAUDE.md "E2E Tests"; note `clips.ts:15,24-27` currently uses `waitForTimeout`, which that convention forbids, though the spec dir is excluded from the regular projects so CI's lint may not scan it - INFERRED, verify). Prose, step titles, alt text and feature links are content and belong where Astro can read them at build time without importing Playwright code.

Concrete shape (proposal):
- `website/src/content/tours/<id>.mdx` (frontmatter: `title`, `summary`, `features: [registry ids]`, `clip: <name>`, `screenshots: [{name, alt, step}]`, `helpDoc`, `order`; body: step prose). Validated by a Zod schema in the content collection config (which also gives the registry link check, section 5).
- `tests/e2e/website-demos/tours/<id>.ts` exports `{ name, scenes, shots }` and `clips.ts` becomes an index (`CLIPS = [...]`). Adding a tour = one scene file + one mdx file (meets the "one spec + one content entry" scalability NFR).
- A drift check (Bun/Node script in `website/scripts/`) asserts the set of `clip:`/`screenshots` names in mdx equals the set the recorder emitted (a `media-manifest.json` produced by the recorder: name, kind, bytes, sha256, commit SHA, theme). That manifest also feeds the freshness check and the job-summary table (requirements "Observability").

`build-features.mjs`'s `DEMO_CLIP_BY_HEADING` (`:102-106`) is a third place with clip names; fold it into the tour/feature frontmatter (`feature: <heading>` -> clip) so there is one mapping, not two.

## 4. `/tours` in Astro Starlight

- Existing structure (VERIFIED): `astro.config.mjs` Starlight with hand-listed sidebar (`help/*` slugs, `:36-60`); content under `website/src/content/docs/{help,guides,reference}` plus `index.mdx`; custom `src/pages/404.astro` on `<StarlightPage>`; `src/components/FeatureGrid.astro` rendered by `help/features.mdx`.
- Simplest fit: add `website/src/content/docs/tours/<id>.mdx` as ordinary Starlight docs (autogen sidebar group `{ label: 'Tours', autogenerate: { directory: 'tours' } }`), plus `tours/index.mdx` rendering a `TourGrid.astro` that reads `getCollection('docs')` filtered on `id.startsWith('tours/')`. This reuses Starlight's layout, search, a11y and the existing internal-link check (`check-internal-links.mjs`) at zero new routing code. A separate `src/content/tours` collection plus custom pages (using `<StarlightPage>` as 404 does) only pays off if tours need a layout Starlight docs cannot express; defer.
- Frontmatter schema extension: `docs` collection `extend` in `src/content.config.ts` (path to be confirmed; INFERRED not yet inspected) adding optional `tour: {clip, features, helpDoc, steps}`.
- Media components: reuse the existing `HeroVideo` (`index.mdx:25`) pattern for clips; add `TourClip.astro` (lazy-loaded `<video>`, `preload="none"`, poster = first screenshot, honors `prefers-reduced-motion` per requirements NFR) and `TourShot.astro` with required `alt`. Media is served from `/clips/` and `/shots/` copied in at build, not committed.
- Mobile: tour pages need a responsive layout and mobile-viewport screenshots; add a second recorder project (390x844) only for shots, not video, to bound runtime.
- Gating: tours pages are in the Pages artifact, so Lighthouse CI/axe (`deploy-website.yml:188-205`) already cover them if `lighthouserc*.json` URL lists are extended (INFERRED; check those files).
- `website-ci.yml` (107 lines) builds PRs without recording (`deploy-website.yml:135` comment). Tour pages referencing media that only exists after recording will 404 in PR CI, so the internal-link/media check must treat tour media as "produced later" with an allow-list derived from the manifest of declared names, then assert existence only in the deploy workflow.

## 5. Feature registry and a docs-coverage gate

### Current state (VERIFIED)
- Per-feature files: `docs/registry/features/backend/**` (349 json files, one per RPC) and `docs/registry/features/frontend/**` (148), plus 13 older top-level files (`alias.json` ...). Frontend entry shape (`frontend/backlog-flow.json`): `id`, `type`, `description`, `component`, `path`, `markerLine`, `tested`, `testIds[]`, `lastModified`. The older top-level files use `filePath` instead of `path` (`features/alias.json`): inconsistent, the gate must tolerate both.
- Generation: `make registry-generate` = `registry-generate-backend` (Go scanner `tools/scanner/backend`, additive + `prune-stale-backend.sh`) + `registry-generate-frontend` (ts-node `tools/scanner/frontend/src/main.ts` over `// +feature:` markers in `web-app/src`) + `registry-aggregate` (`tools/scanner/aggregate.py` -> monolithic `*-features.json`, gitignored per `build.yml:970-972` comment) (`Makefile:146-176`).
- CI: the `build.yml` step "Check new RPCs have tests" (`:938-958`) runs `make registry-generate`, `git diff --exit-code docs/registry/features/`, then fails for **newly added** backend files (`--diff-filter=A` against `origin/main`) with `tested == false`, then `bun run validate-catalog`. `lint.yml:337-343` also runs `validate-catalog` (typed feature catalog in `web-app`). `docs/registry/README.md:90` says "PR that increases the gap count must justify" - prose only, not enforced for frontend entries.
- `scripts/find-orphaned-features.py` (72 lines) is **not** a registry tool: it greps `web-app/src` for unreferenced component files. It does not read `docs/registry`. Do not reuse it for the gate; at most borrow its "ignore gen/__tests__" file-walk list.
- Two registries exist: the per-feature JSON (scanner) and the typed catalog (`web-app` `validate-catalog`, `tools/docs-gen/generate.ts` via `make docs-features`). Which is authoritative for "user-facing feature" is an open question (requirements Rabbit Holes).

### Proposed docs-coverage fields (additive, optional in schema)
```json
"docs": {
  "userFacing": true,                    // explicit; default derived (frontend + has route/component marker)
  "help": "web-app/public/docs/backlog.md#flow",   // in-app /help doc (DOC_FILES slug) [+ anchor]
  "website": "help/features | tours/backlog-flow",  // Starlight slug (docs collection id)
  "tour": "backlog-flow",                // optional
  "exempt": "internal: reason"           // mutually exclusive with the above; requires a reason string
}
```
Rules the validator enforces (new script, Go per repo convention, under `tools/scanner/` or `tools/docs-coverage/`):
1. `help` must resolve to a slug in `DOC_FILES` (`web-app/src/lib/docs/docLoader.ts:9-21`, 11 docs) **and** a file in `web-app/public/docs/` (note `workflows` is in `DOC_FILES` and the directory listing shows `workflows.md`, both present; the check must compare these two lists, since drift here silently makes `/help` omit a doc: `loadDocs` uses `Promise.allSettled` and drops failures, `docLoader.ts:24-39`). Anchor, when given, must exist as a heading.
2. `website` must resolve to an `id` in `website/src/content/docs/**` (read frontmatter/filenames; no Astro build required).
3. `tour`, if set, must resolve to `website/src/content/docs/tours/<id>.mdx` and that file's `features:` must include this registry id (bidirectional).
4. Generator behavior: `registry-generate` must **preserve** hand-written `docs` blocks when regenerating (the scanner currently rewrites files; it must merge). This is the main generator risk: `proto_scanner.go` has 50 touches since 2026-08-01 (git log, VERIFIED) so it is churn-heavy.
5. Backend RPC entries: default `docs.userFacing=false` unless a `// +feature:` frontend entry references them; gate applies to frontend entries and to backend entries explicitly marked user-facing. Avoids requiring a help page per RPC (349 of them).

### Gate with ratchet baseline
- `docs/registry/docs-coverage-baseline.json`: **per-feature-ID list** of currently uncovered ids (not a count), so concurrent PRs rarely conflict and removals are visible. Alternative (single count) conflicts on every PR that touches it; rejected, matching the requirements' "per-feature files mitigate" note.
- Gate logic in CI (new step in the same `build.yml` job that already has `origin/main` and registry tooling at `:938-958`):
  - **New/changed feature**: compute `git diff --name-only origin/main -- docs/registry/features/` (added `A` **and modified `M`**, unlike the existing test gate, which is deliberately `A`-only because `lastModified`/`handlerFile` churn on moves, `build.yml:946-949`). For "changed", do not use mere file modification: compare the JSON minus `lastModified`/`handlerFile`/`markerLine`/`testIds` between base and head, so refactors and test additions do not trigger it. Fail if the feature is `userFacing` and lacks `help` or `website` (without `exempt`).
  - **Ratchet**: fail if any id is in the uncovered set but absent from the baseline (new gap); fail if the baseline contains an id now covered or removed (forces baseline shrink, so it only ratchets down). Print the actionable per-feature line (`id`, missing field, where to add) and the coverage count (requirements "Observability").
  - Rollout per requirements Risk Control: stage 3 report-only (`continue-on-error`/`GITHUB_STEP_SUMMARY` table), stage 4 blocking via one flag/step.
- `make registry-generate` stays the generator; add `make docs-coverage` (report) and `make docs-coverage-baseline` (regenerates the baseline; documented update workflow). Add to `make ci` only when blocking (`Makefile:1094`).
- Path-filter caution: `build.yml:41-42` notes `docs/registry/**` PRs already skip some checks; docs-coverage also depends on `web-app/public/docs/**` and `website/src/content/**`, so confirm the job's `detect-changes` filter includes them (INFERRED, verify at plan time).

## 6. Theme rename to "Grimdark" (Naming / IP constraint)

Public copy and published media use only "Grimdark" (requirements.md:107-110). Inventory (counts via `git grep -I -i "wh40k|warhammer|40k"` excluding `.next`, `go.sum`, `bun.lock`, `project_plans`: ~101 files incl. noise; the **only** literal "Warhammer"/"Adeptus"-style matches found in tracked non-plan files were none - the exposure is the identifier `wh40k`/"WH40K"; `40,000` matches in `docs/project_plans/headless-llm` are unrelated byte limits). VERIFIED touchpoints:

| Area | File(s) | What |
|---|---|---|
| Theme type + registry | `web-app/src/lib/contexts/ThemeContext.tsx:15,24,30` | `ThemeName` union, `wh40kTheme` import, class map; `STORAGE_KEY="stapler-theme"` (`:22`), custom cache key `stapler-theme-custom` (`:40`) |
| Theme definition | `web-app/src/styles/theme.css.ts:523-525` (`wh40kTheme`, "grimdark parchment + gold" comment - the product already uses the word) ; `theme-contract.css.ts` (1 hit) |
| FOUC boot script | `web-app/src/styles/theme-boot-script.ts:3` (`THEME_NAMES`), line 12 reads `localStorage['stapler-theme']`, looks it up in a map, **falls back to `clean`** for unknown ids |
| Picker UI label | `web-app/src/components/settings/ThemePicker.tsx:28-29` (`label: "WH40K"`), `ThemePicker.css.ts` |
| Omnibar command | `web-app/src/lib/omnibar/detectors/CommandDetector.ts:9,42` (`>theme wh40k`, "Switch to WH40K theme") + its tests |
| Server | `server/user_themes.go:37` `builtinThemeBases` allow-list contains `"wh40k"`; custom user themes reference a base by id and default to `clean` (`:22`). Existing `~/.stapler-squad/themes/*.json` files with `"base":"wh40k"` would be rejected or defaulted - a persisted-data migration surface. `server/services/content_search_fixture_test.go` (1 hit) |
| Contrast/theme tests | `web-app/src/styles/__tests__/overlayContrastTokens.test.ts`, `components/command-bar/__tests__/commandBarContrast.test.ts`, `lib/unfinished/chipContrast.test.ts`, `web-app/scripts/check-theme-contrast.ts`, `web-app/build/__tests__/{theme-boot,html-shell}.test.ts`, `build/plugins/__tests__/vanilla-extract.test.ts`, `build/__tests__/fixtures/app/src/styles/theme.css.ts`, `lib/contexts/ThemeContext.test.tsx` (storage tests) |
| e2e | `tests/e2e/playwright.config.ts:200-203` (project `visual-wh40k`, `themeFixturePath('wh40k', ...)`), `playwright.dev-mode.config.ts`, `visual-regression.spec.ts`, `theme-fouc.spec.ts`, `theme-background.spec.ts`, `notification-contrast-over-xterm.spec.ts`, `helpers/theme-fixtures.ts`, `global-setup.dev-mode.ts`, `tools/parity-collect.ts`, `baseline/next-e2e-baseline.json` (6 hits) |
| Snapshot images | 2 tracked PNGs under `tests/e2e/tests/snapshots/visual-wh40k/visual-regression.spec.ts/` - the dir name comes from `snapshotPathTemplate` `{projectName}` (`playwright.config.ts:176`), so renaming the project **moves** the directory; `git mv` and keep byte-identical (visual-regression snapshots are an allowed binary class per requirements) |
| Website | `website/src/styles/wh40k.css`, `astro.config.mjs:13` customCss import (2 hits), `src/components/NoThemeSelect.astro:2-3`, `website/tests/unit/wh40k-theme.test.ts` (8 hits), `website/src/content/docs/help/add-user-themes.md` |
| Recorder | `tests/e2e/website-demos/recorder.ts:21,28,38-41` default `theme = 'wh40k'`, writes `localStorage['stapler-theme']` |
| Docs | `docs/how-to/add-user-themes.md`, `docs/architecture/frontend-audit-2026.md`, `CHANGELOG.md:23` (historical release-please text; add to the checker's allow-list rather than rewriting history) |

Not found in `git grep` of `session/`, `config/`, `proto/` (Go side only references the id in `server/user_themes.go`). No server-persisted selected theme: selection is client `localStorage` only (INFERRED from `ThemeContext.tsx:100-160`; confirm no server-side preference field).

**Persisted-id migration design**
1. New id `grimdark`. Keep a one-way alias map `LEGACY_THEME_IDS = { wh40k: 'grimdark' }` in a single module imported by both `ThemeContext.tsx` (on mount: read stored value, map, **write back** the new id) and `theme-boot-script.ts` (must apply the alias *before* the class lookup, otherwise first paint falls to `clean` then flips: the FOUC test `theme-fouc.spec.ts` must gain a case with `stapler-theme=wh40k` seeded).
2. `server/user_themes.go`: accept `"wh40k"` as a legacy `base` and normalize to `grimdark` on load (user files on disk are not rewritten without consent).
3. Omnibar `>theme wh40k` kept as a hidden alias or dropped (planning decides; a hidden alias must be in the checker's allow-list).
4. The alias map file is the single allow-listed location for the legacy string; the CI term check allows exactly that path (and `CHANGELOG.md`), nothing else.

**CI trademark check** (new, small, Go or Bun script): scan `git ls-files` text files (excluding `go.sum`, lockfiles, `CHANGELOG.md`, the alias module, `project_plans/`) for `warhammer|40k|wh40k|adeptus|...` (case-insensitive; list lives in a checked-in file so terms are reviewable) and **published content**: the built `website/dist` (HTML, alt text, JSON) plus clip/shot filenames and `ffprobe` title metadata, plus `<out>/media-manifest.json`. Run source scan in `lint.yml` (cheap) and dist scan in `deploy-website.yml` build job after `bun run build`. It cannot read pixels; add a recorder-side assertion that the active theme class is `grimdark` and the picker label reads "Grimdark" (requirements "check that captured media really is the theme"). Imagery cannot be machine-checked: note as a residual manual review item at the owner-review blocker.

## 7. Cross-cutting recommendations

- **No committed media (guard):** add CI step failing on newly added `*.png|gif|webm|mp4|jpg` outside an allow-list (logos/favicons/icons, `tests/e2e/tests/snapshots/**`). Use `git diff --diff-filter=A origin/main` like the registry gate (`build.yml:950`); put it in `website-ci.yml`/lint so it runs on every PR. Then delete `assets/demo-recording/*`, `assets/demo.*`, `assets/screenshot.png`, `website/public/{demo.webm,screenshot.png,features/*}` once the recorder emits replacements and `build-features.mjs:190-211`'s source-image requirement is replaced by recorder-manifest names.
- **Freshness check:** recorder writes `media-manifest.json` with `commit = $GITHUB_SHA`; the build job asserts every manifest entry referenced by `features.json`/tours exists and `commit == GITHUB_SHA`; any missing referenced clip **fails** the build rather than warning (`deploy-website.yml:148-152` currently warns only). Keep the notify issue (`:272`) for record failures.
- **Recording scale:** one job at 45 min (`:47`) for 2 clips; N tours need a matrix (shard by tour id, upload per-shard artifacts, `download-artifact` with `merge-multiple`) modelled on `demo-publish.yml`'s record/publish split (PRIOR:101-117). Check `docs/reference/ci-job-budget.md` before adding jobs (not opened this session: UNVERIFIED).
- **README embeds (open question):** README cannot embed Pages-only clips as committed paths once binaries are removed. Options: absolute Pages URLs (stale if site down, simplest, no release asset) vs the rolling release asset from PRIOR section 2. Recommend absolute Pages URL to a stable `/clips/hero.webm` plus poster via the same; keep `demo-publish.yml` as-is or retire after, since its 26-entry hash manifest is legacy (decision for planning).
- **Observability:** emit the per-clip table (recorded/failed/skipped, bytes, sha256) to `$GITHUB_STEP_SUMMARY` from the recorder script; the existing `shasum -a 256` output (`record-website-demos.sh:117`) is the seed.

## 8. Hotspot disposition (INFERRED from `git log --since=2026-08-01 --name-only` touch counts; no complexity metric run this session)

| Touched area | Churn signal | Disposition | Why |
|---|---|---|---|
| `Makefile` (80 touches) | highest | **Isolate via seam** | Add new targets as a small included `make/docs-coverage.mk` (or only call scripts) instead of growing the monolith; one `include` line + one `ci` prerequisite. |
| `tools/scanner/backend/proto_scanner.go` (50) | high | **Isolate via seam** | Do not modify the scanner for `docs`; put docs-coverage in a separate validator that reads the JSON files and a merge step, avoiding regeneration clobbering hand-written `docs` blocks. If the frontend scanner must preserve `docs`, wrap with a post-step merge. |
| `.github/workflows/deploy-website.yml` (5) | moderate | **Extend as-is** (+ matrix) | Already structured as record -> build -> deploy; add shards and checks as steps; extract shared setup into the existing composite actions (`./.github/actions/*`). |
| `scripts/record-website-demos.sh` (120 lines, bash) | n/a | **Refactor-first (small)** | Requirements prefer Go scripting; it needs a per-tour loop, media manifest, shard arg. Extract manifest/summary writing into a Go tool before adding features to avoid growing bash; keep server start/stop in bash. |
| `website/scripts/build-features.mjs` (3) | low | **Extend as-is** | Replace `DEMO_CLIP_BY_HEADING` with frontmatter-driven mapping; unit tests exist in `website/tests/unit`. |
| Theme rename files (`ThemeContext.tsx`, boot script, `theme.css.ts`, e2e configs) | rename-only | **Extend as-is** | Mechanical; risk is the migration alias and FOUC, covered by new tests, not structure. |
| `build.yml` registry step (`:938-958`) | moderate | **Isolate via seam** | Add a separate step/script for docs-coverage; do not edit the existing `--diff-filter=A` test gate logic. |

Residual unknowns to close at plan time: `website/src/content.config.ts` and `lighthouserc*.json` contents; `ci-job-budget.md` limits; whether `playwright.website-demos.config.ts` is scanned by the e2e convention lint (it uses `waitForTimeout`); the feature-flag enabling mechanism for recorded flag-gated pages; which registry (scanner JSON vs typed catalog) defines "user-facing".
