# Build vs Buy: promo-materials-refresh

**Date**: 2026-11-10
**Confidence labels**: VERIFIED = opened in this repo this session. UNVERIFIED = from model knowledge, no web check run (no WebSearch used; confirm versions and licenses before adopting anything).

## Repo facts this analysis rests on (VERIFIED)

- `tests/e2e/website-demos/recorder.ts` (103 lines) already does: fresh Playwright context with `recordVideo` at 1280x720, a `ClipSpec { name, title, theme, scenes }` contract, WH40K theme injected via `localStorage['stapler-theme']` in an init script, onboarding/toast suppression, `caption()` overlay, `typeSlowly()`, measured `trimStartMs` written to `<name>.meta.json`.
- `tests/e2e/website-demos/clips.ts` (71 lines) defines 2 clips (`session-dashboard`, `auto-approval-rules`). Adding a clip = one entry; the deploy ships it.
- `scripts/record-website-demos.sh` (120 lines): seeds with `go run ./tests/demo/seed <dir> 6`, starts an isolated `--test-mode` server with a private tmux socket, runs `playwright.website-demos.config.ts`, post-processes with `scripts/demo-post-process.sh` (ffmpeg trim, title card via rsvg-convert), drops clips under 2 s, prints sha256 per clip, exit 3 on partial failure.
- Website stack (`website/package.json`): Astro ^7.2.10, `@astrojs/starlight` ^0.42.6, `@astrojs/mdx` 8.1.0, `@playwright/test` ^1.64.1, vitest, lhci, axe. Website scripts are `.mjs` (`build-features.mjs`, `check-asset-health.mjs`, `check-internal-links.mjs`, `sync-content.mjs`).
- Registry: `docs/registry/features/*.json` (per-feature files), `docs/registry/schema.json`, generator in `tools/docs-gen/*.ts` (Bun/TS), CI gate scripts in `tools/ci-gates/*.js`. Custom Go analyzers live in `tools/lint/*` (separate `go.mod`).
- Gaps vs requirements: no screenshot capture path in the recorder; no frozen clock; no per-clip result summary/freshness manifest; theme is applied but not verified; only 2 clips.

## (1) Automated product-tour / demo generation

### 1a. Playwright built-in video + `page.screenshot` (extend existing)
- **Pros**: Already in use and CI-proven; same toolchain as e2e (Bun, `data-testid` locators, `--test-mode` isolated server); zero new dependencies or licences; screenshots are one call (`page.screenshot`, optionally `locator.screenshot`, `animations: 'disabled'`, `clock` API for a frozen time); scales as "one spec + one content entry".
- **Cons**: Playwright video is low-bitrate VP8 webm at fixed size (needs ffmpeg post-processing, already present); `waitForTimeout` is banned by e2e conventions yet used in `clips.ts` today (recorder dir is outside the lint scope? UNVERIFIED), so new clips should use web-first waits; cursor is not rendered in the video (the existing clip parks the mouse; a synthetic cursor overlay would be needed for polished tours).
- **Verdict**: **Recommended** (core engine for both clips and screenshots).

### 1b. Dedicated screenshot tools (shot-scraper, Percy/Chromatic, Puppeteer)
- **Pros**: shot-scraper (Python, Playwright-based, YAML-driven) is quick for static shots.
- **Cons**: Adds a second browser-driving stack that duplicates Playwright's seeded-server and theme logic; visual-regression SaaS (Percy/Chromatic) is paid and aimed at diffing, not marketing assets; cannot share the `ClipSpec` scenes.
- **Verdict**: **Not recommended**.

### 1c. Remotion (React-to-video)
- **Pros**: Fully deterministic, frame-exact, can composite captions, zooms, callouts, music, and encode MP4/GIF; good for a polished hero reel.
- **Cons**: Licence is source-available with a paid company licence above a small team size (UNVERIFIED; verify at remotion.dev/license before use); heavy dependency (Chromium + ffmpeg + React build) on top of what Playwright already provides; it needs source footage anyway, so it would sit after Playwright, not replace it; slower CI render; overkill when ffmpeg + the existing title card already work.
- **Verdict**: **Not recommended** for the pipeline now; revisit only for a single polished hero video.

### 1d. VHS (charmbracelet) / asciinema (+ agg) for terminals
- **Pros**: VHS (MIT, UNVERIFIED) renders scripted `.tape` files to GIF/MP4/WebM deterministically; asciinema + `agg` yields small, crisp, text-selectable recordings. Good for the CLI/tmux side of the product.
- **Cons**: The product is a web UI whose terminal is embedded in the page, so most tour value is captured by Playwright already; VHS needs Go + ttyd + ffmpeg in CI (extra install time); a terminal clip of stapler-squad CLI would show little beyond the web UI.
- **Verdict**: **Viable**, as an optional add-on only if a tour needs a raw CLI/tmux segment (for example `ssq-mux`). Not in the core path.

### 1e. SaaS tour builders (Arcade, Supademo, Storylane, Navattic)
- **Pros**: Click-through interactive demos with hotspots and no code; non-engineers can edit copy.
- **Cons**: Violates the constraints directly: assets live in the vendor, not generated from `main` in CI; no freshness gate; capture is manual (browser extension) so they go stale exactly like today; paid tiers, branding/watermarks on free plans, third-party scripts hurt the Lighthouse thresholds and the CSP/a11y gates; seed data would be captured by hand; no WH40K theme guarantee.
- **Verdict**: **Not recommended**.

### Recommendation for (1)
Extend the Playwright recorder: add `screenshot()` helper (frozen clock, animations disabled, WH40K assertion), keep ffmpeg post-processing, emit a JSON manifest. No new runtime dependencies.

## (2) Astro / Starlight components for tours, galleries, video

Starlight 0.42 supports custom components, MDX and component overrides natively (VERIFIED dependency; features UNVERIFIED against 0.42 docs).

### 2a. Hand-written Astro components (`<TourStep>`, `<Clip>`, `<Shot>`) + a `tours` content collection
- **Pros**: Small (a few `.astro` files); full control over lazy-loading, `prefers-reduced-motion` (already handled for existing clips per requirements), a11y, and the asset-health manifest; content collection with Zod schema gives typed frontmatter (steps, clip name, screenshots, feature ids) which the registry gate can read; no third-party JS on the page.
- **Cons**: We own the markup and a11y; need unit tests (vitest present).
- **Verdict**: **Recommended**.

### 2b. Starlight built-in primitives (`<Steps>`, `<Tabs>`, `<Card>`, `<FileTree>`)
- **Pros**: Shipped with Starlight, accessible, themed; ideal for the step list inside a tour page.
- **Cons**: No video/gallery primitive.
- **Verdict**: **Recommended** (use alongside 2a, not instead).

### 2c. Third-party Astro integrations (astro-video, lightbox/gallery plugins such as `astro-lightbox`-style packages, `starlight-image-zoom`)
- **Pros**: `starlight-image-zoom` (UNVERIFIED) gives click-to-zoom for annotated screenshots cheaply.
- **Cons**: Small, single-maintainer packages that lag Astro majors (Astro 7 here is very new; compatibility UNVERIFIED); each adds client JS that hits Lighthouse budgets and may break the asset-health check which parses built HTML; video plugins add nothing over a `<video>` tag.
- **Verdict**: **Viable** for image zoom only after checking Astro 7 / Starlight 0.42 compatibility; **Not recommended** for video/gallery plugins.

### 2d. Embedded players (Video.js, Plyr, YouTube/Vimeo embeds)
- **Verdict**: **Not recommended**: native `<video muted loop playsinline>` with reduced-motion fallback is sufficient, and third-party embeds break the "generated in CI, shipped in the Pages artifact" rule.

## (3) Docs-coverage and link checking: tools vs custom registry gate

Two distinct jobs:

- **A. Is the link/page valid?** (broken links, orphan assets). Generic tools fit.
- **B. Does feature X have a help page, website explanation and (optionally) tour?** This is a registry-semantic check; no off-the-shelf tool knows `docs/registry/features/*.json`.

### 3a. Off-the-shelf link checkers (lychee, linkinator, `starlight-links-validator`, markdown-link-check)
- **Pros**: Mature crawling, anchors, external-URL handling. `starlight-links-validator` (UNVERIFIED version/licence) validates internal links and anchors at Astro build time, which would complement the existing `website/scripts/check-internal-links.mjs` and `check-asset-health.mjs`. lychee (Rust, MIT/Apache, UNVERIFIED) is fast for external links.
- **Cons**: The repo already has `check-internal-links.mjs` and `check-asset-health.mjs`; a second internal-link checker overlaps. External-link checking in a blocking gate is flaky by nature.
- **Verdict**: **Viable** for job A only. Keep the existing scripts; optionally add lychee as a non-blocking scheduled job. Do not use for job B.

### 3b. Doc-coverage tools (Vale, markdownlint, doc-detective, docs-coverage plugins)
- **Pros**: Vale/markdownlint enforce prose style.
- **Cons**: None map feature IDs to docs; doc-detective executes docs as tests, which is a different goal.
- **Verdict**: **Not recommended** for job B.

### 3c. Custom registry gate (chosen for job B)
Extend the registry schema with optional-then-required docs fields (for example `docs: { help: "<doc slug>", website: "<page path>", tour?: "<tour slug>" }`), a per-feature ratchet baseline, and a checker that verifies each referenced slug resolves to a real file (`web-app` help docs via `docLoader` manifest; website `content/docs/**` and `content/tours/**`).
- **Pros**: Exactly the semantics required; per-feature JSON files already avoid merge conflicts; actionable output ("feature X missing help page") satisfies the observability requirement; the ratchet is a baseline file diff.
- **Cons**: We own it; mapping robustness (naming drift, one page covering many features) is the real complexity, not the code.
- **Verdict**: **Recommended**.

### Go vs JS for the checker
| Factor | Go | JS/TS (Bun) |
|---|---|---|
| Repo guidance | Memory note `feedback_use_go_for_scripting` and requirements: new scripts in Go where practical (hermetic, testable, Bazel-cached) | Registry generator `tools/docs-gen/*.ts` and website scripts are already JS/TS |
| Data access | Reads JSON registry and walks files easily with the stdlib | Same |
| Needs the Astro content collection / help doc loader? | Only reads files and frontmatter; fine if the contract is "slug maps to a file path" | Easier to reuse `docLoader`/Zod schemas, but importing web-app code into a gate is fragile |
| Test and CI fit | `go test`, can live beside `tools/lint` or `tools/ci-gates` | `bun test`/vitest like other `tools/ci-gates/*.js` |
| Cost of keeping two registry readers | Duplicates the schema in Go (the TS generator remains the writer) | Single source with the generator |

Recommendation: **Go** for the gate and ratchet checker (reads registry JSON, resolves file paths, parses YAML/MDX frontmatter with a small library). Keep the field definitions in `docs/registry/schema.json` as the single contract, and have the TS generator preserve (not generate) the docs fields. If the coverage rule needs the in-app `docLoader` manifest, export that manifest as JSON from the web-app build and let the Go checker read the JSON rather than import JS. UNVERIFIED: whether `docLoader` can emit a manifest without a Next build; the planner must check `web-app/src/lib/docs/docLoader`.

## (4) Fork / adapt existing `tests/e2e/website-demos` + `record-website-demos.sh`

### 4a. Adapt in place (recommended)
- **Pros**: Most requirements are met by the existing design: hermetic isolated server, private tmux socket, WH40K theme injection, trim from measured load time, partial-failure tolerance, CI-wired. Adding clips and tours is one `ClipSpec` each. Work needed is incremental: screenshot support, frozen clock, richer `tests/demo/seed` data, theme assertion, JSON manifest (name, kind, size, sha256, commit SHA), freshness check, drop the stale-screenshot fallback, shard by tour for CI time.
- **Cons**: `record-website-demos.sh` is bash while repo guidance prefers Go for new scripting: the orchestration (seed, start server, wait, record, post-process, manifest) is the part to port to Go if it grows (manifest and freshness logic especially); the existing bash file is the proven path and rewriting it carries risk. `clips.ts` uses `waitForTimeout` and `page.waitForTimeout` in scenes, which conflicts with the e2e convention; tours should use web-first waits or a shared `pause()` helper that documents the exemption. Per-clip fixed viewport only (mobile screenshots need a second viewport profile).
- **Verdict**: **Recommended**.

### 4b. Fork into a new standalone tool or new package
- **Pros**: Clean slate for tour specs and a typed manifest.
- **Cons**: Duplicates recorder, server bootstrap and post-processing; two things to keep in step; no requirement forces it.
- **Verdict**: **Not recommended**.

### 4c. Port orchestration to Go, keep Playwright specs in TS
- **Pros**: Matches repo preference; manifest/freshness/size-budget logic becomes unit-testable.
- **Cons**: Slightly more work than adding bash lines; two languages in one pipeline.
- **Verdict**: **Viable**, best done only for the new manifest/freshness/media-guard steps (new code), not as a rewrite of the working record script.

## LLM-written bespoke code vs battle-tested libraries

| Concern | Lean toward | Reason |
|---|---|---|
| Browser automation, video capture, screenshots | Library (Playwright) | Already adopted; bespoke capture would be strictly worse |
| Video trimming, title card, encode | Library/tool (ffmpeg, rsvg-convert) | Already in use; never hand-roll codecs |
| Link parsing, anchor validation, external URL crawling | Library (starlight-links-validator / lychee) or the two existing repo scripts | Edge cases (anchors, redirects, encoding) are where LLM code silently misses cases |
| Image zoom / lightbox | Small library, if Astro 7 compatible; else 20 lines of bespoke JS | Low risk either way |
| Tour/steps/clip/screenshot Astro components | Bespoke (thin) over Starlight primitives | Small, specific to the site's WH40K design and asset-health contract; LLMs write this reliably; tests catch regressions |
| Registry docs-coverage gate, ratchet, mapping | Bespoke | No library knows the domain; logic is small, deterministic, and fully unit-testable; the risk (mapping robustness) is a design problem, not a code-volume one |
| Media-committed-binary guard | Bespoke (git ls-files plus extension allowlist) | Trivial; a library adds nothing |
| Determinism (frozen clock, seed data) | Library features (`page.clock`, `animations: 'disabled'`) plus bespoke seed fixtures | Use platform features before custom timers |

Rule of thumb applied: buy (or reuse) anything with a large input space or protocol edge cases (video, browsers, crawling); build anything that encodes this repo's own domain rules (registry, tour schema, ratchet, freshness) and keep it small with tests. LLM-written code is acceptable for the latter because it is covered by unit tests and bounded in surface; it is risky for the former because failures are silent and environment-dependent.

## Overall recommendation

1. Engine: extend existing Playwright recorder (screenshots, clock freeze, theme assertion, manifest); ffmpeg stays; no SaaS, no Remotion.
2. Site: bespoke thin Astro components plus a `tours` content collection, using Starlight `<Steps>`/`<Tabs>`; optional image-zoom plugin only after a compatibility check.
3. Gate: custom Go checker over extended registry schema with per-feature ratchet; keep existing website link/asset scripts, optionally add lychee as a scheduled non-blocking job.

## Open items for the planner
- Verify (WebSearch) licences and Astro 7 / Starlight 0.42 compatibility for `starlight-links-validator`, `starlight-image-zoom`, lychee before adding any.
- Check whether `docLoader` can export a slug manifest without a Next build.
- Decide whether `clips.ts` waits should move to a documented `pause()` helper to satisfy the `no waitForTimeout` convention for new tour specs.

## (5) Scope update: rename WH40K to "Grimdark" and the trademark-term CI check

Note: the Playwright and recorder sections above say "WH40K"; read those as "Grimdark" after the rename (`recorder.ts` default theme and the `stapler-theme` localStorage value change with it).

### Measured size of the rename (VERIFIED, `git grep -iE 'wh40k|warhammer|40k|adeptus|space marine'`)
- 217 tracked files match overall; 40 after excluding `project_plans/`, `CHANGELOG.md`, `web-app/.next/` and `build/`. Of those 40, `go.sum` and `web-app/bun.lock` are false positives: a bare `40k` matches inside hash strings, so the check needs word boundaries (`\b40k\b`) or must skip lockfiles.
- Identifier forms (non-plan files): `wh40k` x198, `wh40kTheme` x43, `WH40K` x15, `visual-wh40k` x4, `wh40kStart` x3, `wh40k-theme` x2, `wh40kBlock` x2. Mixed case and compound identifiers matter: a case-sensitive `s/wh40k/grimdark/` misses `WH40K`, and an unqualified rename of `wh40kTheme` needs a deliberate new name.
- Tracked paths to rename: `website/src/styles/wh40k.css`, `website/tests/unit/wh40k-theme.test.ts`, and two PNGs under `tests/e2e/tests/snapshots/visual-wh40k/visual-regression.spec.ts/`.
- Key code sites: `web-app/src/lib/contexts/ThemeContext.tsx`, `web-app/src/styles/theme.css.ts`, `theme-contract.css.ts`, `theme-boot-script.ts`, `components/settings/ThemePicker.tsx` and `.css.ts`, `lib/omnibar/detectors/CommandDetector.ts`, `server/user_themes.go`, `website/astro.config.mjs`, `website/src/components/NoThemeSelect.astro`, e2e specs (`visual-regression`, `theme-fouc`, `theme-background`, `notification-contrast-over-xterm`) and `tests/e2e/helpers/theme-fixtures.ts`, `tests/e2e/baseline/next-e2e-baseline.json`.
- Surprise worth a separate ticket: `git ls-files` shows `web-app/.next/*`, `web-app/playwright-report/index.html` and `build/windows_amd64/stapler-squad.exe` are tracked. They contain the term but are build output; rename must regenerate or untrack them, not edit them, and the "no committed binaries" goal overlaps.

### Rename approach
| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Plain `sed`/`git grep -l | xargs sed` | Fast, no install | Blind to case variants and compound identifiers; rewrites inside strings, comments and lockfiles without review; no file renames; repo CLAUDE.md steers away from sed for edits | Not recommended as the only tool |
| ast-grep (`sg`, installed at `~/.cargo/bin/sg`) | Structural: rewrites identifiers and string literals in TS/TSX/Go without touching comments or hash strings; per-language rules; dry-run diff | Mixed identifier forms need several rules; CSS/Astro/Markdown/JSON coverage is weaker | Viable for TS/Go identifiers |
| Small scripted codemod (Go or Bun) with an explicit mapping table (`wh40k`->`grimdark`, `WH40K`->`Grimdark`, `wh40kTheme`->`grimdarkTheme`, `visual-wh40k`->`visual-grimdark`, ...), word-aware regex, file/dir renames via `git mv`, skip list (lockfiles, plans, CHANGELOG, build output) | One reviewed pass, idempotent, re-runnable until the CI check is green, handles filenames and non-code files, mapping is the documented spec | Written once, throwaway; needs a dry-run mode | **Recommended** (build): codemod for the mechanical bulk, then review the diff |
| Manual edits | Highest care on the semantic bits | About 40 files, 300+ occurrences; error-prone and slow | Use only for the semantic parts below |

Must be done by hand (not mechanical): the persisted-preference migration. `ThemeContext.tsx` and `theme-boot-script.ts` read the stored `stapler-theme` value; users holding `wh40k` must map to `grimdark` (read-old, write-new, or keep a legacy-id alias) before first paint, with `theme-fouc.spec.ts` and `theme-boot.test.ts` covering it. The server's `user_themes.go` (user theme JSON ids) needs the same alias check. Visual-regression snapshot directories must be renamed with `git mv` so baselines survive (and the Playwright `snapshotPathTemplate` in `playwright.config.ts` updated); regenerating them instead would hide a real visual change.

### Trademark-term CI check
| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Inline `git grep -iEw` step in `lint.yml` | 5 lines, no code to maintain, uses git's tracked-file list (so ignores node_modules), fast | Allow-list and exemptions end up as ad hoc `:!pathspec` args; message quality is minimal | **Recommended for v1** |
| Dedicated Go checker (sibling of `tools/ci-gates`, or run via `go run`) reading a terms file and an allow-list file with per-entry reasons | Actionable per-file:line output, unit-tested, allow-list scoped to the migration alias only, can also scan the built website HTML and recorded-clip metadata | More code for what is a grep; the repo prefers Go for new scripts, so this is the right shape if we want published-content scanning | **Recommended** if the check must scan the built Pages output as well as source |
| Existing `.golangci.yml`/`tools/lint/*` analyzers | Already wired into `make lint-custom` | Go-AST analyzers only see Go; useless for CSS/TS/MD | Not recommended |
| Third-party (Vale, cspell with a forbidden-words list, `codespell`-style) | Mature word lists | New tool for one regex; cspell forbidden words are file-type aware but add install time | Viable, not worth it |

Design notes for either: terms `warhammer`, `games workshop`, `adeptus`, `space marine`, `wh40k`, `\b40k\b`, `40,000`-style variants (decide the list in planning); case-insensitive; exclude `go.sum`, `*.lock`, `bun.lock`, `.git`, `project_plans/**` and `CHANGELOG.md` history (or the check fails on historical text that cannot change), and anything under generated output; allow-list entries only for the legacy-id alias, each with a reason; also scan filenames (`git ls-files`), not just contents; run it on the built `website/dist` and the recording manifest to catch anything the sources do not contain. Wire it as a single CI step so rollback is one revert, consistent with the staged rollout in the requirements.

### Recommendation for (5)
Build a one-off mapping-table codemod (Go or Bun, dry-run first, `git mv` for paths), use ast-grep only if a TS/Go pass needs structural precision, hand-write the preference migration and its tests, and ship the CI check as a `git grep` step first, upgrading to the Go checker only if built-output scanning is required (recommended, since the requirement covers published content).
