# Pre-mortem: promo-materials-refresh

**Date**: 2026-11-10. **Method**: `~/.claude/commands/sdd/pre-mortem-prompt.md` is absent, so the fallback applies. It is three months after shipping and the project has failed. Failure causes are ranked P1 (likely and severe), P2 and P3. Each has likelihood, impact, early signal and a prevention to patch into `plan.md`.

Claims marked VERIFIED were checked against the repo on 2026-11-10. Everything else is INFERRED.

## Verified facts used below

- VERIFIED: the boot script falls back to `clean` for any stored theme id not in its map (`web-app/src/styles/theme-boot-script.ts:12`: `cls = t && m[t] ? m[t] : m['clean']`). `THEME_NAMES` still lists `wh40k` (`:3`), and `ThemeName` is a closed union (`ThemeContext.tsx:24`).
- VERIFIED: `server/user_themes.go:37` hard-codes the built-in theme bases, including `wh40k`.
- VERIFIED: `git grep -il wh40k` (excluding `CHANGELOG.md` and `project_plans`) returns 34 files. This matches the plan.
- VERIFIED: `deploy-website.yml` runs on `ubuntu-22.04` with `timeout-minutes: 45` and a weekly cron (`17 6 * * 1`). It uses one global `concurrency: group: "pages"` with `cancel-in-progress: false`.
- VERIFIED: the guard job `guard` in `backlog-scaffolding-guard.yml` has `timeout-minutes: 8` and is deliberately checkout-free because `.git` is ~16 GB. Its own comment gives a baseline of avg 4m and max 10.1m.
- VERIFIED: the job already runs close to its timeout (max 10.1m against a limit of 8). Adding checkout, setup-go and a Go build to it raises the risk of timeouts on every PR.
- VERIFIED: `website/public` is 2.1 MB today, so the Pages artifact headroom is currently huge. Size is a growth risk, not a present one.

---

## P1 (act before implementation)

### P1-1. The always-run guard job becomes a flaky, slow gate for every PR

- **Failure story**: Task 1.1.4b and Task 2.1.1b add a checkout (`fetch-depth: 2`), `setup-go`, a build that pulls ~593 transitive packages via `session/git`, and two tools to the one job every PR gets. The job already peaks above its 8-minute timeout. A cold Go cache or a slow checkout of the 16 GB-pack repo makes the check red or timed out. That red check is a required check, so unrelated PRs (docs, Dependabot, mobile) are blocked and people start re-running or admin-merging.
- **Likelihood**: High. **Impact**: High (repo-wide merge friction, erodes trust in the guard).
- **Early signal**: the Task 1.1.4b0 measurement shows p95 above 5 minutes warm. After merge, `guard` rerun rate rises or its max duration approaches the timeout.
- **Prevention (patch into plan)**:
  1. Make 1.1.4b0 a hard gate: if warm p95 is above 5 minutes, split the work into a separate job `promo-guards`, and do not extend `guard`. Run it with `if:` path-sensitive logic inside the job (so the check always reports, but exits early when no relevant paths changed) rather than via `paths:` filters.
  2. Replace the go-git dependency for the two PR-time tools with a checkout-free input. The existing job already uses `gh api .../pulls/N/files`, which lists changed paths with no clone. `mediaguard` needs only the path list plus blob sizes (`files[].additions` is not enough, but the contents API or `git cat-file` on a sparse fetch will do). Fall back to go-git only for `docscoverage`, which already runs in `build.yml` where a checkout exists. This removes the 593-package dependency, `fetch-depth` work and the `HEAD^1` pitfall from the always-run path.
  3. Add a `continue-on-error` kill switch via `.github/promo-flags.json` (`guardsEnforce`) so a broken guard can be turned off by a one-line PR.

### P1-2. Theme rename strands users or breaks the recorder: three places must agree and the plan has no atomic test across all three

- **Failure story**: the plan's ordering constraint (alias, id switch and snapshot move in one PR, three commits) is correct, but it is enforced only by prose. If the PR is split or reviewed commit by commit, the boot script (a template-string-generated inline script in `theme-boot-script.ts`) and `ThemeContext` get out of step: the stored value `grimdark` falls to `clean` on a build where `THEME_NAMES` lacks it, and the stored `wh40k` falls to `clean` on a build where the alias isn't wired. Users also have a third persisted source: the server's user theme files with `"base": "wh40k"` (`user_themes.go:37`), which would be rejected as an unknown base and silently drop the user's custom theme. The recorder (`tests/e2e/website-demos/recorder.ts`) also sets the stored id, so a mismatch records the whole media set in the wrong theme.
- **Likelihood**: Medium. **Impact**: High (visible user regression, silent, and the weekly deploy publishes wrong-theme media).
- **Early signal**: the theme-id parity test is red or missing. A weekly deploy's `themeId` stamp is `clean`. Users report a reset theme after upgrade.
- **Prevention**:
  1. Add an e2e/jest test that, in the same PR, loads the real built boot script with `localStorage['stapler-theme']` set to each of `wh40k`, `grimdark`, `custom:x` and an unknown id, and asserts the exact class. The plan covers only the `wh40k` case in 1.1.1c, so add the other three.
  2. Server side: Task 1.1.2a must also cover any API that persists or echoes a base (for example the themes list consumed by the picker), and a test that a stored `wh40k` base still resolves to `grimdark` end to end.
  3. Keep the alias for at least two releases, not one, and put the removal date in the alias module's header and a tracking issue. Removing it is the real hazard: dormant users who return after a long gap lose their theme.
  4. The recorder asserts theme by computed body background and by the exported `ThemeID` constant (Task 2.3.1c). Make the check fail the shard, not warn, from day one regardless of `freshnessEnforce`.

### P1-3. Recording flakiness blocks the weekly deploy and the freshness flip turns it into a permanent outage of the site pipeline

- **Failure story**: after Story 5.1.2 the deploy fails if any expected clip or shot is missing. There are 8 tours, 19+ card shots, mobile shots, and a seeded-state matrix on shared GitHub runners with Chromium, ffmpeg, tmux and a live agent-free server. A single flaky scene (animation timing, a poll that hasn't rendered, a server-side window that the frozen clock doesn't cover) fails the whole deploy. `concurrency: pages` with `cancel-in-progress: false` queues retries behind each other. Nobody notices until the weekly cron issue is ignored a few times, and the site goes stale for weeks anyway. This is the staleness the project was meant to eliminate.
- **Likelihood**: High. **Impact**: High (site un-deployable, and ordinary website fixes are blocked too because they use the same workflow).
- **Early signal**: the per-clip summary shows any clip with a non-zero retry or failure rate in the soak period; `mediatool fresh` report-only output is not clean for several runs in a row before 5.1.2.
- **Prevention**:
  1. Add a measured soak criterion to Story 5.1.2: flip `freshnessEnforce` only after N consecutive (say 10) green full runs, including at least two weekly cron runs, not after one green run. The plan's current gate is "first green full deploy".
  2. Per-scene retry: wrap each Scene in a bounded retry (2 attempts) with a fresh page and a fresh server state check, and record attempts in the manifest. Retrying at the Scene level is cheap; retrying the whole workflow is not.
  3. Separate the failure domains. A deploy of content-only website changes must not require re-recording. Let `deploy-website.yml` reuse the previous successful run's media artifact (cache by recorded-commit key) when only `website/**` changed, and require fresh recording only for the weekly cron and for app changes. Without this, one flaky scene blocks every docs typo fix.
  4. Define "stale but acceptable" explicitly: if `mediatool fresh` fails on a scheduled run, deploy last-known-good media with a visible banner/issue instead of not deploying, up to a maximum age (for example 28 days), after which it hard-fails. This keeps R5's intent (no silent staleness) without making the pipeline brittle.

### P1-4. The docs-coverage gate blocks legitimate PRs because "changed feature" and "user-facing" are fuzzy and the baseline is a merge-conflict magnet

- **Failure story**: the gate fires on registry semantic fields (`name`, `description`, `component`, `path`, `userFacing`). The registry is scanner-generated; `make registry-generate` is required on many PRs, so a PR that moves a component file or reorders a path changes `path` and trips the gate even though no feature changed. 163 default-userFacing ids, 20 containing `:` (percent-encoded filenames), a duplicate id (`launcher_presets:get`) and two doc universes (in-app `/help` and the Starlight site) mean the resolver will produce false negatives and positives for months. The gate also depends on `HEAD^1` of the merge ref, so a re-run on a head SHA, a `workflow_dispatch`, or a merge queue run compares against the wrong base. Contributors and agents (including autonomous backlog sessions) then add junk `exempt` reasons or stub help pages to get green, which defeats the purpose and pollutes docs.
- **Likelihood**: High. **Impact**: Medium-high (agent and contributor friction, doc quality decays).
- **Early signal**: during the report-only window, the share of PRs that "would have failed" is above ~5%, or the exempt ratio climbs toward the ceiling.
- **Prevention**:
  1. Turn the observation window into a numeric exit criterion in 4.1.5: at least 30 merged PRs or 4 weeks, and a "would have failed" rate under 3% with every false positive triaged. Record the actual count in the flip PR.
  2. Treat a `path`/`filePath`-only change as non-semantic (a move or rename that changes nothing else). Only `id`, `name`, `description`, `userFacing` flips and brand-new ids should count. Keep `component` out as well unless the id is new. This sharply cuts false positives while still catching new features.
  3. Handle merge queue and `workflow_dispatch` explicitly: when the base cannot be resolved, report-only with a notice, never a failure (the plan fails loudly with exit 2 for the media guard; do not apply that to the docs gate once it is blocking).
  4. Give `exempt` a reviewable cost: require `exempt` entries to name an approver in CODEOWNERS-reviewed sidecars, and fail when the exempt count rises by more than 5 in one PR.

### P1-5. Trademark-term check false positives and leaks through channels the scanner does not read

- **Failure story**: two opposite failures. (a) The `\b40k\b` rule and the substring rules (`adeptus`, `astartes`, `space marine`) match legitimate content: token budgets ("a 40k context"), test data, docs, third-party text in vendored files and generated registry JSON, so the always-run job blocks unrelated PRs and people add allow-list entries until the allow-list is the real policy. (b) The scanner covers source, dist HTML text, manifest and DOM text, but not pixels. The theme's own copy in the app UI (fonts, decorative text, theme descriptions, icons, SVGs, CSS `content:` strings) can render trademarked imagery or words that never appear as text in a scanned file, and screenshots are binaries so nothing scans them. The plan accepts this as an owner check, but then it runs weekly and unattended: any later UI change (a new theme string) ships to public media with no human in the loop.
- **Likelihood**: Medium. **Impact**: High for (b) (legal/IP exposure on a public site), Medium for (a).
- **Early signal**: allow-list grows in the first month; a media-text dump contains a banned fragment; reviewer flags imagery in a tour.
- **Prevention**:
  1. Run the term check in `-mode=media-text` on the recorded DOM text *before* the Pages upload and make it block the upload step unconditionally (not behind `freshnessEnforce`).
  2. Add a cheap pixel-side check: OCR (tesseract on the shots, offline, a few seconds for ~40 images) in the record job, scanning for the banned set, as a non-optional step. The plan defers this ("not machine-gateable"), but it is gateable with OCR for text; reserve the manual check for logos and imagery only.
  3. Add a content-change trigger: any PR touching theme CSS/TS, `grimdark.css`, fonts or `web-app/src/styles/theme*` requires the owner's explicit sign-off label (CODEOWNERS entry), since those files decide what the public sees.
  4. For (a): keep the `\b40k\b` rule but scope it to user-visible/published paths only (`website/`, `web-app/src/`, `README.md`, `assets`, `docs/`), not the whole tree, and ship the term list with a golden test of known-benign strings ("40k tokens", "x240kb").

---

## P2

### P2-1. Pages artifact and runtime growth outpaces the budgets

- **Story**: 2.1 MB today (VERIFIED) grows to tens of MB with 8 clips, ~30 shots and mobile variants. The Pages limit (1 GB artifact, ~10 min deploy timeout) is far away, but the per-asset budgets in `budgets.json` (clip 3 MB, image 400 KB) are not tied to a total ceiling, Lighthouse mobile scores fall on the tour pages, and the record job moves from a single-server run towards 45 minutes with 2 shards.
- **Likelihood**: Medium. **Impact**: Medium.
- **Early signal**: `dist` size in the job summary trends upward week on week; record job above 30 minutes; Lighthouse performance on `/tours/*` under 0.90.
- **Prevention**: add a total `dist/` ceiling (for example 40 MB) and a record-job wall-clock budget (for example 30 min) to `budgets.json`/`mediatool fresh`, and print the week-on-week trend in the summary. Serve WebP/AVIF posters only, `preload="none"` (already planned), and keep the clip count within the stated 8.

### P2-2. Weekly cron cost and the single-writer `pages` concurrency group

- **Story**: the sharded matrix runs the full recording (two shards, each with a built binary, Chromium, ffmpeg) weekly plus on every push to main that touches website paths. `concurrency: pages, cancel-in-progress: false` means pushes queue behind a 20-30 minute record run. Runner minutes rise noticeably and contributors wait on website changes.
- **Likelihood**: Medium. **Impact**: Medium.
- **Early signal**: queued runs in the `pages` group, monthly Actions minutes up.
- **Prevention**: record only on cron, `workflow_dispatch` and app-affecting pushes; reuse the last media artifact for content-only pushes (same mechanism as P1-3 step 3); record the `count-pr-checks.py` before/after and an Actions-minutes estimate in the PR as the plan already requires.

### P2-3. Stale `docs/features.md` and a moving product make tours go stale in meaning, not pixels

- **Story**: media is regenerated weekly, but tour copy, step text and `features[]` ids are hand-written. A UI rename or removed feature leaves steps that describe controls that no longer exist while the clip looks fine. The freshness gate only checks that media was produced from the deployed commit.
- **Likelihood**: Medium-high. **Impact**: Medium.
- **Early signal**: a scene's locator was renamed but the tour text still describes the old label.
- **Prevention**: scenes assert on the same strings that the tour steps quote (a test fails if a step's `quote:` text is absent from the DOM dump); registry `features[]` ids that disappear fail the bidirectional check (already planned); add the owner-review reminder to the weekly notify.

### P2-4. Seeding feasibility (spike 0.1.1d) fails for several scenarios and the fallback quietly becomes the product

- **Story**: approvals, backlog, insights and canned terminal output may not be seedable without a real agent. The plan's fallback is `page.route` fixtures per RPC. If most tours end up route-faked, tours show states the product cannot actually produce, and a later server API change silently breaks the fixtures (ConnectRPC JSON drift) while the clip still records successfully.
- **Likelihood**: Medium. **Impact**: Medium-high.
- **Early signal**: spike 0.1.1d reports "no" for three or more items.
- **Prevention**: validate route fixtures against the generated proto types at test time (a Go or TS test that decodes each fixture with the real message schema); cap fixtures to at most 2 per tour; drop a tour rather than fake more than that (the owner decision is already planned, so make the threshold explicit).

### P2-5. Legacy demo tooling deletion and the `allow-media` label override are an escape hatch that outlives its purpose

- **Story**: the label override lets anyone with triage rights bypass the media guard, and it works for any binary, not only the Phase 5 revert. Over time the label becomes the normal way to add a screenshot, and the 16 GB history bloat returns.
- **Likelihood**: Low-medium. **Impact**: Medium.
- **Early signal**: more than one PR uses the label in a quarter.
- **Prevention**: restrict the override to a label applied by a CODEOWNERS member (verify with the events API `labeled` actor), and have the guard print the added byte total. Alert via `notify` when the label is used.

---

## P3

- **P3-1. Safari/older iOS WebM playback** (open question in the plan): the hero shows only a poster on those browsers. Likelihood medium, impact low. Prevention: decide the `.mp4` rendition before Story 3.1.1a final markup, or accept poster-only with `controls` and a visible download link.
- **P3-2. `ubuntu-22.04` runner retirement** shifts ffmpeg and font versions, making shots drift and visual baselines change. Likelihood low in the 3-month window. Prevention: record `ffmpegVersion` and the font hash in the manifest (partly planned) and pin fonts from the repo, not the runner.
- **P3-3. README link rot**: README links to `/tours` on the Pages site; a base-path or domain change breaks it. The plan derives it from `SITE_ORIGIN`/`SITE_BASE` with a unit test, which is enough. Prevention: add the README link to the link checker's scope.
- **P3-4. Sidecar filename portability**: percent-encoded `:` filenames work on Linux and macOS but are awkward on Windows checkouts of the repo (the repo ships a Windows build). Prevention: ensure `%` in filenames is handled in `.gitattributes`/tools and test a Windows checkout if Windows contributors exist.
- **P3-5. Doc coverage report noise**: the coverage count in the job summary is mistaken for a quality metric, and teams game it with thin help pages. Prevention: show "exempt" and "grandfathered" counts next to "covered" and never show a single percentage.

---

## Patch list for plan.md (concise)

1. Task 1.1.4b0: add the hard gate and split-job fallback; prefer checkout-free path listing for the always-run guard (P1-1).
2. Story 1.1.1 / 1.1.2: add boot-script cases for `grimdark`, `custom:*`, unknown ids; server-side end-to-end test; alias retained two releases with a dated removal issue (P1-2).
3. Story 2.4.1 and 5.1.2: Scene-level retry, content-only reuse of last good media, bounded "last-known-good" deploy with a max age, and a 10-run soak criterion before flipping `freshnessEnforce` (P1-3).
4. Story 4.1.2 / 4.1.5: path-only changes are non-semantic; numeric observation window and false-positive triage; unresolved base is report-only; exempt approvals and per-PR exempt cap (P1-4).
5. Story 1.1.4 / 2.4.1: scope `\b40k\b` to published paths with a benign-string golden test; `media-text` mode blocks upload unconditionally; OCR step on shots; CODEOWNERS sign-off on theme files (P1-5).
6. `budgets.json`: total `dist/` and record-job wall-clock ceilings (P2-1, P2-2).
7. Story 3.2.x: scenes assert the strings that tour steps quote; fixtures validated against proto schemas, at most 2 route-fakes per tour (P2-3, P2-4).
8. Story 2.1.1: `allow-media` limited to CODEOWNERS-applied labels with a notify on use (P2-5).

**P1 count: 5**
