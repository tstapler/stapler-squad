# Research: feature landscape for promo-materials-refresh

Naming: the theme is called only "Grimdark" in all public copy and media (requirements.md:107-108). Internal ids (`wh40k`, `wh40k.css`) are cited only as code identifiers.

## 1. Inventory (VERIFIED by command output)

### Sources and their sizes
- `docs/features.md`: 19 `## ` sections (lines 3-136): Dashboard, Live Terminal Streaming, Browser Element Inspector, Diff Viewer, Notifications, Instant Search, Status Filtering, Tag-Based Grouping, Workspace Switcher, Bulk Actions, Review Queue, Auto-Approval Rules, Custom Rules, Approval Analytics, History Search, Logs Viewer, Config Viewer, Parallel Fan-Out, Isolated Git Workspaces (19 headings listed; `website/src/content/features.json` has 19 cards, headings at lines 3-107).
- `README.md:16-43` "Highlights": 5 categories (Visibility, Organisation, Review & Approval, History & Debugging, Infrastructure). Demo embeds at `README.md:5` (`assets/demo.gif`) and `README.md:9` (`assets/demo.webm`).
- Routes: `web-app/src/app/route-meta.ts:24-71`, 47 rows, of which ~6 are test/spike/debug (`/test/*`, `/spike-*`, `/debug/escape-codes`). Flag guards only on `backlog` (4 rows: /backlog, /backlog/board, /backlog/flow, /settings/backlog-sources) and `changelog` (route-meta.ts:11, 27-31, 44).
- Registry: `docs/registry/features/` has 15 top-level JSON files, `backend/` 102 entries (RPC-oriented, mostly not user-facing as such), `frontend/` 87 entries (mostly UI sub-features: backlog-*, insights-*, notification-*, etc.). Schema example `docs/registry/features/analytics.json:1-19`: fields id, type, description, backend, frontend, tested, testIds, lastModified; no docs/website/tour fields.
- TS catalog: `web-app/src/lib/features/features/*.ts` (RPC-level `id:`s like `approval-resolve`), separate from the JSON registry.

### Important correction to requirements.md
requirements.md says only 1 of ~19 feature cards has a screenshot. Actually 8 cards have a `screenshot` (`features.json:6,33,40,47,64,71,78,110`, files `website/public/features/01..08-*.png`, all from commit of 2026-10-15, PR #1051). Cards with none: Live Terminal Streaming, Browser Element Inspector, Diff Viewer, Notifications, Status Filtering, Workspace Switcher, Bulk Actions, Approval Analytics, History Search, Logs Viewer, Config Viewer, Parallel Fan-Out, Isolated Git Workspaces (11). Two cards carry demo clips (see 3).

### Big gap: the feature cards cover only the old surface
The 19 cards (derived from `docs/features.md` by `website/scripts/build-features.mjs:37-110`, which hard-fails when a section lacks a category or alt text, lines 261,281) omit nearly all newer product areas visible in routes: Backlog (board, flow, sources, stages, quota, pipeline modes), Insights (cost/spend), Owner Inbox, Up Next (`/unfinished`), Triggers, Workflows, Artifacts, Files viewer, Import External Sessions, Escape Analytics, Omnibar/session creation, Settings (LLM backends, background models, Jules, remotes, features/flags), Account/login, Changelog, Slack/inline notification actions, multi-PR tracking, remote hosts.

## 2. In-app /help docs
- `web-app/src/lib/docs/docLoader.ts:10-22`: hard-coded `DOC_FILES` list of 11 slugs, fetched from `/docs/<slug>.md` (docLoader.ts:26) sourced from `web-app/public/docs/*.md` (11 files).
- Slugs: what-is-stapler-squad (41 lines), session-types (54), omnibar (65), keyboard-shortcuts (43), configuration (70), tmux-integration (73), backlog (11), insights (7), notifications (7), tagging-rules (7), workflows (7).
- Stubs: backlog, insights, notifications, tagging-rules, workflows are 7-11 lines each (wc -l), i.e. effectively placeholder help.
- No in-app doc for: review queue, approval rules, diff viewer, history, logs, fan-out, triggers, files, artifacts, owner inbox, up next, import, escape analytics, remotes, themes.
- Mapping to registry is by naming only: route `/help` has no per-feature deep link besides `?doc=<slug>` (`web-app/src/app/help/page.tsx:45,66`).

## 3. Website pages
- Help pages `website/src/content/docs/help/`: 13 files: add-user-themes, debug-with-logs, dispatch-work-to-google-jules, enable-pi-support, expose-github-webhook-endpoint, expose-slack-interactive-endpoint, features.mdx, getting-started, macos-codesigning, manage-notifications, manage-systemd-service, monitor-external-terminal-sessions, use-up-next-pr-tabs-and-ask-to-fix. Synced from `docs/how-to/*.md` via `website/scripts/content-manifest.json` / `sync-content.mjs:40,109`. These are how-to docs, NOT the in-app /help docs; the two doc sets do not overlap (different sources), so "website explanation" and "in-app help" must be separate registry fields.
- `website/src/content/docs/guides/example.md` and `reference/example.md` are placeholders. No `/tours` exists. Pages: `website/src/pages/404.astro` plus Starlight `index.mdx`.
- `features.mdx` says "All 17 features" (stale: 19 cards).

## 4. Media today
- Committed binaries: `website/public/features/01..08-*.png`, `website/public/{demo.webm,screenshot.png}`, `assets/{demo.gif,demo.webm,screenshot.png}`, `assets/demo-recording/01..08-*.png` (identical set duplicated 3x). Last touched 2026-10-15 (git log).
- CI clips: `tests/e2e/website-demos/clips.ts` defines only 2 clips (`session-dashboard` line 13, `auto-approval-rules` line 39), mapped by `DEMO_CLIP_BY_HEADING` (build-features.mjs:102). Recording by `scripts/record-website-demos.sh`, seeding via `go run ./tests/demo/seed <dir> 6` (script line 50; `tests/demo/seed/main.go:15-40` calls `demo.SeedDirectory`), server `--test-mode --test-dir` (script line 54). Seed is sessions only; no approvals/backlog/PRs/analytics data was found in this survey (UNVERIFIED: depth of `demo.SeedDirectory`, not opened).
- Theme: names in `web-app/src/styles/theme-boot-script.ts:3` (`matrix, cyberpunk77, wh40k, clean, light, dark`); stored in localStorage key `stapler-theme` (`ThemeContext.tsx:22`). Recording harness can set that key before load (needs verification vs boot script).
- NAMING HAZARD: the in-app ThemePicker label is literally "WH40K" with description "Grimdark parchment + gold" (`web-app/src/components/settings/ThemePicker.tsx:28-30`). Any screenshot of Settings > Theme would publish the trademarked label. Also trademarked terms appear in `website/src/styles/wh40k.css`, `website/src/components/NoThemeSelect.astro`, and `website/src/content/docs/help/add-user-themes.md` (grep -il for warhammer/40k). Note `add-user-themes.md:23` publishes the raw theme id `wh40k` in a help page, so the id either gets renamed or that page needs an allowlisted exception; `wh40k.css` is bundled into the site CSS (internal filename, user-invisible unless the trademark check scans CSS). Recommendation: relabel the picker to "Grimdark" (keep id `wh40k`), avoid shooting the picker until then, and add the CI trademark-term check over `website/dist` plus alt text/filenames.

## 5. Proposed tour list
Rule: a multi-step tour when the feature is a workflow (>=3 meaningful states, state change over time, cross-page). One screenshot when it is a single view. Every user-facing card gets at least a screenshot.

### Multi-step tours (proposed 8)
1. Dashboard and session lifecycle (create via Omnibar -> live terminal -> diff -> pause/resume). Needs `/sessions/new`, `/`, terminal; README flagship.
2. Review and approval: needs-approval -> Review Queue -> Auto-Approval Rules -> add custom rule (existing clip `auto-approval-rules` extends this) -> Approval Analytics.
3. Organise at scale: search, status filter, tag grouping (8 strategies), bulk actions, workspace switcher.
4. Backlog pipeline: board -> item detail -> spawn session -> plan approval -> flow view (`/backlog`, `/backlog/board`, `/backlog/flow`). Flag-gated: `backlog`.
5. Parallel Fan-Out / Compare variants (`/fanout`) with isolated git workspaces.
6. Insights and cost: `/insights`, session detail drawer, daily spend, findings. Seeded data required.
7. Notifications and attention: tray, toasts, inline actions, Owner Inbox, Up Next PR tabs.
8. Automation: Triggers, Workflows, tagging rules.

### Single-screenshot features
History Search (`/history`), Logs Viewer (`/logs`), Config Viewer (`/config`), Escape Analytics (`/analytics/escape`), Errors, Files viewer, Artifacts, Import External Sessions, Settings (LLM backends, background models, remotes, features), Browser Element Inspector (needs a live browser session), Diff Viewer (can fold into tour 1), Changelog (flag `changelog`), Help/keyboard shortcuts, mobile dashboard (separate mobile shot set).

Excluded: `/test/*`, `/spike-*`, `/debug/*`, `/login`, `/account`, `/resolve`.

## 6. Industry comparison (UNVERIFIED: from general knowledge, no web fetch performed; verify before citing)
Common patterns on comparable dev-tool tour pages (Linear, Vercel, Cursor, Warp, Raycast, Zed): one short looping muted clip per feature (5-20 s), a headline of the outcome not the feature, 3-5 numbered steps with callout crops, a poster frame for reduced-motion/no-JS, a "try it" or docs link at the end, and a sticky index. Interactive "Arcade/Storylane"-style click-through demos are an alternative but need JS and a hosted tool. Recommendation: clip + numbered annotated stills + docs link per tour, matching the requirements.

## 7. Edge cases
- Flag-gated: `backlog` and `changelog` guards (`route-meta.ts:9`, `FeatureGuard.tsx:5-22` via `RequireFeatureFlag` from `FeatureFlagsContext.tsx`). Recording must enable flags via the feature-flag RPC (or seed config) and assert the guard rendered content (a disabled guard renders the fallback/blank, giving a silently empty screenshot). Settings > Features page exists (`/settings/features`).
- Needs real agent output: live terminal, plan approval, Needs-Approval states, diff content, escape analytics, insights spend. Options: seed fixtures into the `--test-mode` dir (sessions via `demo.SeedDirectory`), a scripted fake agent program in tmux that prints canned Claude-Code-like output, or mocked RPC responses via Playwright `route`. Real agents are non-deterministic and cost money, so rule out.
- Integrations (GitHub PRs, Slack, Jira/Linear, Jules): `/unfinished`, CI-status badge, create-PR, Slack settings. Need mocked fixtures; e2e helpers like `tests/e2e/helpers/multi-pr.ts`, `notification-action-seed.ts`, `backlog-settings-mocks.ts` already exist and are reusable starting points (note: CLAUDE.md says e2e helpers cannot import web-app).
- Determinism: fixed viewport, frozen clock/relative timestamps ("5m ago"), hide cursor blink and animations, wait for fonts (Grimdark theme fonts), no real hostnames/paths in seeded sessions (public data classification, requirements.md:48).
- Mobile: tours need a 390 px variant for at least dashboard, review queue and terminal; existing `*-mobile.spec.ts` specs show the pattern.
- Auth: `/login` and remote access should not appear; record on localhost test-mode.

## 8. Unstated user needs
- Evaluators want a 30-60 s "what is this" hero clip, an install-to-first-session path, and answers to trust/safety questions (what approvals block, isolation of worktrees) more than a feature checklist.
- Honest labelling: tours using fixtures should say "demo data"; avoid implying real agent output.
- Docs-gate usability: a registry author needs a one-line way to mark a feature "internal / no docs needed" (most of the 102 backend and many of the 87 frontend registry entries are not user-facing), otherwise the blocking gate drowns contributors. Needs a `userFacing` flag plus ratchet baseline.
- Mapping registry to docs needs a stable key: registry ids (e.g. `session-board-view`) do not match help slugs (`session-types`) or card headings (`Dashboard`); one help page covers many features. Prefer explicit `docs: {help: "<slug>", website: "<path>", tour: "<id>"}` fields, validated by file existence, over name matching.
- Stale facts to fix alongside: `features.mdx` "17 features", placeholder `guides/example.md`, 5 stub help docs, duplicated media trio.
- Accessibility: alt text per image and captions/transcripts for clips; reduced-motion poster.
- Keep tour count extensible: one definition per tour (spec + content entry), per requirements NFR.

## 9. Open items for planning
- Decide tour definition location (clips.ts today is name/title/script; extend with steps + stills metadata).
- Confirm depth of `demo.SeedDirectory` before choosing fixtures vs mocks.
- Confirm whether theme can be set via localStorage `stapler-theme` before boot, and rename the picker label.
