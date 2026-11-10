# Implementation Plan: promo-materials-refresh

**Feature**: CI-generated, Grimdark-themed promo media (screenshots, clips, mobile set), a `/tours` website section, README links to tours, a theme rename to "Grimdark" with legacy migration, and a docs-coverage gate on the feature registry.
**Date**: 2026-11-10
**Status**: Repaired in four rounds (architecture/adversarial review, then pre-mortem + consistency check + validation gaps in "Plan repair log - Phase 4", then "Plan repair log - Triad round 1" and "Plan repair log - Triad round 2" at the end); ready for implementation planning
**ADRs**: ADR-001 (tours section), ADR-002 (extend recorder, Go tooling), ADR-003 (CI media, fail loud), ADR-004 (Grimdark rename + alias + term check), ADR-005 (docs-coverage sidecar ratchet) in `project_plans/promo-materials-refresh/decisions/`

Already done on this branch (uncommitted, not replanned): untracking of `web-app/.next`, `web-app/playwright-report`, `build/*.exe`, `uploads/` and their `.gitignore` entries.

## Cross-cutting decisions (repair round 1)

These apply to every task below and override any older wording.

1. **Shared Go code placement.** Go's `internal/` rule lets only code rooted at the parent of `internal/` import it. `tests/demo/scenario` is rooted at `tests/` (VERIFIED: `tests/demo/` has only `demo_test.go`, `helpers.go`, `seed/`; no `tools/` ancestor), so a `tools/**/internal/` package is unreachable from it. Code shared by `tools/promo`, `tools/docscoverage` and `tests/demo/scenario` therefore lives in **non-internal packages `tools/promolib/gitbase` and `tools/promolib/termscan`** (root module, so `make test`, `make lint` and `dupl` cover them). Code used by one tool only stays in that tool's `internal/` (`tools/promo/internal/manifest`, `tools/docscoverage/internal/{resolve,changed,model}`). `tests/demo/scenario/leak_test.go` imports `termscan.LeakPatterns` directly (no CLI exec).
2. **Git access uses go-git, never the CLI.** `make lint-custom` (Makefile:946-950) runs the `norawgitcli` analyzer over the root module, and `session/git/AGENTS.md` plus the `prefer-go-git-over-subshells` skill say never shell out to git. `tools/promolib/gitbase` opens repos with `session/git.OpenRepo` (`session/git/util.go:34`; the `norawgitopen` analyzer forbids raw `PlainOpen`) against the existing `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5` fork. Tools stay in the **root module** (not separate modules like `tools/lint`, `tools/scanner`), so they are linted, tested and `dupl`-checked. Operations: `ls-files` = read the index; changed files = tree diff of `-base <sha>` against HEAD (adds and modifies by path; a rename shows as delete + add, which is correct for the media guard); base blob = tree lookup at `-base`. In CI the `pull_request` checkout is the merge commit, whose first parent is the base, so a depth-2 fetch plus `-base HEAD^1` yields exactly the PR's changes with no deep clone (the repo `.git` is ~16 GB; see the `detect-changes` comments in `build.yml`). Local default for `-base`: merge-base of `origin/main` and HEAD via go-git `Commit.MergeBase`. Fetching in workflow YAML (`git fetch --depth=2`) is workflow plumbing, not tool code.
3. **Always-run PR checks.** `lint.yml` has a `paths:` allow-list (`**.go`, `web-app/**`, `tests/e2e/**`, ...), so a PR touching only `website/**`, `assets/**`, `README.md` or `docs/**` creates no Lint check. `mediaguard` and `termcheck -mode=source` therefore run as steps in the existing path-filter-free **`pr-diff-guards` job of `.github/workflows/backlog-scaffolding-guard.yml`** (the "one check a docs-only PR gets", `docs/reference/ci-job-budget.md`), preferring an extra step over a new job per that doc (this placement, and every `guard` / `pr-diff-guards` reference in this plan, is conditional on the Task 1.1.4b0 outcome: Cross-cutting 8b). A `website/tests/unit/workflow-yaml.test.ts` case asserts that workflow has no `paths:` or `paths-ignore:` under `on.pull_request`. A matching step in `lint.yml` is not added (it would run twice on code PRs).
4. **Rollout flags live in git.** `.github/promo-flags.json` (`{"freshnessEnforce": false, "docsCoverageEnforce": false, "guardsEnforce": true}`; `guardsEnforce` is the kill switch for the always-run guards: `false` turns `mediaguard` and `termcheck -mode=source` report-only via `continue-on-error`, so a broken guard is switched off by a one-line PR) replaces repo variables `PROMO_FRESHNESS_ENFORCE` / `DOCS_COVERAGE_ENFORCE`. Workflows read it with `jq` after checkout; a flip is a reviewable one-line PR that also works for fork PRs, and rollback is a revert. A unit test asserts all three keys exist and are booleans. Each flip story opens a tracking issue; the notify job comments if a flag is still `false` 14 days after its prerequisite story merged (Task 2.4.1b). Wherever this plan says `PROMO_FRESHNESS_ENFORCE=true` or `DOCS_COVERAGE_ENFORCE=true`, read "the matching key in `.github/promo-flags.json` is `true`".

5. **Round-2 repairs (apply everywhere).** (a) `gochecknoglobals` is enabled in `.golangci.yml` and does not exempt `tools/promolib/**` or `tests/demo/scenario`, so the leak patterns and banned terms are **functions** (`termscan.LeakPatterns() []Pattern`, `termscan.BannedTermSet() []Term`) that build compiled regexps inside the function (memoize with a function-local `sync.OnceValue` held in a closure, not a package var); wherever this plan says `termscan.LeakPatterns` or `BannedTermSet` as a value, read the function call. (b) `gitbase` `-base HEAD^1` is valid only on the `pull_request` merge ref; `gitbase` fails loudly (exit 2, message naming the event and the `-base` flag) if HEAD has fewer than two parents while `-base` is the default `HEAD^1`. (c) The always-run guard job has id `guard` (display name `pr-diff-guards (...)`) **if the Task 1.1.4b0 measurement passes, else `promo-guards`** (Cross-cutting 8b); tests assert by the job id recorded by 1.1.4b0; prose below says "`pr-diff-guards`" for the display name. (d) The guard job was checkout-free on purpose (the repo `.git` is ~16 GB; comment at `backlog-scaffolding-guard.yml:41-45`); the checkout, Go setup and runtime budget are set by Task 1.1.4b0. (e) `tools/promolib/gitbase` importing `session/git` pulls ~593 transitive packages (`go list -deps ./session/git`) into three small tools; this is a build-time cost only (no runtime or binary-shipping impact), accepted; `session/git/native.OpenRepo` is the lighter alias if build time matters. (f) Registry file counts, verified by `find docs/registry/features/<dir> -name '*.json' | wc -l`: **349 backend, 148 frontend, 13 root files (510 total)**; no other count is used in this plan.

6. **Round-3 repairs (Phase 4: pre-mortem, consistency check, validation gaps).** (a) **`helpDoc` is a `WebsiteDocID`** (a Starlight docs collection id such as `help/getting-started`), not a `DocSlug`. In-app slugs in `DOC_FILES` (`configuration`, `backlog`, ...) live in `web-app/public/docs/` and have no page on the website, so they cannot be a website link (VERIFIED: `website/src/content/docs/help/` holds only synced `docs/how-to` pages). Task 3.1.1e validates every `tour.helpDoc` against the docs collection ids at build time. (b) **Tour frontmatter shape is ADR-001's nested `tour:` object** (all plan references to `clip:`, `helpDoc:`, `features:` mean `tour.clip`, `tour.helpDoc`, `tour.features`); ADR-001 is extended with the UX fields (Task 3.1.1a). (c) **Media rendering has two modes**, selected by `TOURS_MEDIA_MODE` (`deploy` default, `smoke` set only by `website-ci.yml`): in `deploy` mode any `tour.clip` or step shot absent from `media-manifest.json` fails the build; in `smoke` mode a tour whose clip is absent from the manifest is **not built at all** (the docs collection's loader glob in `content.config.ts` excludes it, computed from the manifest), so it has no page, sidebar entry, search hit, grid card, prev/next link, landing card link or README link; for a built tour, `TourShot` renders nothing for a step shot absent from the manifest (text, narration, transcript and CTAs remain, no placeholder). The smoke subset is the `dashboard-lifecycle` clip (the hero clip) and its shots. This replaces the earlier "produced-later link allow-list". (d) **Flag rollout order**: no gate flips to blocking without its numeric soak/observation criterion recorded in the flip PR. (e) **Start-here tours are fixed**: 1 `dashboard-lifecycle`, 2 `approval-review`, 3 `organise-at-scale`; `index.mdx` derives the strip from `tour.order` 1-3, not hard-coded. (f) **Scene discovery is by glob** (`tests/e2e/website-demos/tours/*.ts`), so the two-file rule for a tour holds with no index edit.

7. **Triad round 1 repairs (apply everywhere; they override older wording).**
   (a) **Gate inputs come from the base blob, never the PR tree.** The guard reads `.github/promo-flags.json` (`guardsEnforce`), `tools/promo/mediaguard/allowlist.txt`, `tools/promo/termcheck/allowlist.txt` and `.github/CODEOWNERS` (authority for `allow-media`) through `gitbase.ReadBlob(base, path)` with `base = HEAD^1` of the `pull_request` merge ref (the base-branch tip; a depth-2 fetch includes its tree). A PR that edits any of these files is judged by the *old* content; the new content takes effect after merge (so a one-line `guardsEnforce: false` PR does not unblock itself; the owner merges it with the ordinary admin override). The guard *binary* is built from the PR tree, so the code that decides is protected by review instead: `.github/CODEOWNERS` gains entries for `.github/promo-flags.json`, `tools/promo/**`, `tools/promolib/**`, `tools/docscoverage/**`, both allow-list files, `.github/workflows/backlog-scaffolding-guard.yml` and `.github/workflows/deploy-website.yml` (Task 2.1.1c), and Task 2.1.1c verifies by `gh api repos/{owner}/{repo}/branches/main/protection` that "require review from code owners" is on (if not, it is an owner action recorded in `spike-notes.md` and the blocking wiring waits: Cross-cutting 8c). Workflow `permissions:` for the guard job are declared explicitly: `contents: read`, `pull-requests: read`, `issues: read` (events API actor lookup), nothing else. `GITHUB_TOKEN` cannot resolve CODEOWNERS **teams** (needs org `read:org`), so `mediaguard` honours `allow-media` only when the labeller's login is listed as an individual user in the base CODEOWNERS entry covering `.github/promo-flags.json`; team entries are ignored with a notice and the override fails closed (`labeller <login> not listed as a user; teams cannot be resolved with GITHUB_TOKEN`). The fallback for a team-only CODEOWNERS is to list the owner's login beside the team.
   (b) **Freshness has a reuse mode.** `FreshnessCheck` stays `commit == GITHUB_SHA` for every full record. A content-only deploy runs `mediatool fresh -reuse-from <recordedCommit>` instead, which replaces the commit equality with: the recorded commit is an ancestor of `GITHUB_SHA` **and** none of the **recording-input path set** changed between them. The path set (one list in `tools/promo/budgets.json` under `recordInputPaths`, shared by the workflow's decision step and `mediatool fresh`): `web-app/**`, `server/**`, `session/**`, `proto/**`, `config/**`, `app/**`, `go.mod`, `go.sum`, `tests/e2e/website-demos/**`, `tests/demo/**`, `docs/features.md`, `website/src/content/docs/tours/**`, `website/src/content/feature-media.json`, `tools/promo/**`, `tools/promolib/**`. Ancestry and the changed-file list come from the compare API (`gh api repos/{owner}/{repo}/compare/<recorded>...<sha> --paginate`, status `ahead` or `identical`; a truncated file list or a non-`ahead` status is not reusable), so no deep clone is needed. Theme and the full `ExpectedMedia` presence checks still apply in reuse mode. A reuse run is **not** a "full run" for any soak count (Task 5.1.2a).
   (c) **Last-known-good (LKG) is a named artifact**, not an `actions/cache` entry (cache entries are evicted after 7 days without access, exactly the weekly cron's period, and the repo now has main-only/bounded cache policy). Task 2.4.1g defines it: artifact name `promo-media-lkg`, `retention-days: 35` set explicitly, uploaded only by a successful full run on `main` that passed `mediatool fresh` in full mode; lookup is the newest successful `deploy-website.yml` run on `main` via `gh run list --workflow deploy-website.yml --branch main --status success`, then `gh run download <id> -n promo-media-lkg`, so the deploy job declares `actions: read`. A missing or expired artifact when LKG or reuse is needed is a **hard failure**, never a silent skip (test). The 28-day age bound stays; with 35-day retention it is satisfiable.
   (d) **Dependency order fixed.** `tools/promolib/gitbase` (Task 1.1.4a0), `.github/promo-flags.json` with its unit test and the combined `tools/promo/cmd/promoguard` binary (Task 1.1.4a1) are created in Phase 1 before `termscan` (1.1.4a), the runtime measurement (1.1.4b0) and the wiring (1.1.4b). `mediaguard` (2.1.1a) becomes a subcommand of `promoguard` and consumes `gitbase`; it no longer creates it. Wherever older text says "created in 2.1.1a" or "created 2.1.2a" for these files, read the Phase 1 task.
   (e) **Video encoding is decided:** every clip is encoded as WebM (VP9) and an H.264 MP4 rendition (`yuv420p`, `+faststart`), emitted as `<source>` elements in the order `webm`, `mp4` with a `poster`; the MP4 counts against the same 3 MB per-clip budget (ffmpeg settings tuned to fit), both files are manifest rows and `ExpectedMedia` entries, and the freshness check requires both. The Safari/iOS Unresolved Question is closed.
   (f) **Tour order and prev/next:** `tour.order` (unique integers 1..N) is the single ordering; Start-here is `order` 1-3; prev/next follow ascending `order`; in smoke mode unrecorded tours are not built at all (6c), so they are skipped by construction.

8. **Triad round 2 repairs (apply everywhere; they override older wording).**
   (a) **Absent gate inputs (bootstrap).** `gitbase.ReadBlob` returns a typed not-found for a path absent from the base tree, and the guard maps it to fixed defaults instead of failing: `.github/promo-flags.json` absent -> `guardsEnforce=true`, `freshnessEnforce=false`, `docsCoverageEnforce=false`; an allow-list file absent -> empty allow-list (every added binary and every banned-term file is a finding); `.github/CODEOWNERS` absent -> nobody holds `allow-media` authority. A file that is present but malformed is exit 2, never the defaults. **PR order so inputs land first:** PR A creates `promo-flags.json`, both allow-lists and `.github/CODEOWNERS` with the Go tools (Tasks 1.1.4a0, 1.1.4a1, 1.1.4a, 2.1.1a, 2.1.1c) and wires nothing; PR B wires the guard steps (Tasks 1.1.4b, 2.1.1b) after A has merged, so B's base blob already holds every input. The defaults only cover the window before A, and tests pin them (Tasks 1.1.4a1, 2.1.1c).
   (b) **Guard job identity.** "The guard job" is the job chosen by Task 1.1.4b0: `guard` (display name `pr-diff-guards`) if the measurement passes, else `promo-guards`. Every plan reference to job id `guard` or `pr-diff-guards` is conditional on that outcome; `workflow-yaml.test.ts` holds the chosen id in one constant, recorded in `spike-notes.md` and ADR-003. **Outcome (2026-11-10): `promo-guards`**, from `guard`'s documented 4m avg / 10.1m max baseline; CI timings unmeasured (`spike-notes.md` "Guard wiring"). The constant is `PROMO_GUARD_JOB_ID`.
   (c) **Blocking precondition (chosen over building the guard from the base ref, which needs a second checkout and build per run and has its own bootstrap problem).** The guard steps are wired as blocking (Tasks 1.1.4b, 2.1.1b) only after Task 2.1.1c has created `.github/CODEOWNERS` (absent today, verified by `ls .github/CODEOWNERS`) and recorded in `spike-notes.md` that `gh api repos/{owner}/{repo}/branches/main/protection --jq .required_pull_request_reviews.require_code_owner_reviews` returns `true`; if not, the wiring waits on that owner action (wall-clock blocker 8). Residual risk (an admin override, a code owner approving a bad guard change, protection disabled later) is recorded in ADR-003.
   (d) **`allow-media` authority.** Honoured only when the **latest** `labeled` event for `allow-media` (the label still on the PR) was made by an actor of type `User` whose login is listed as an individual user in the base-blob CODEOWNERS entry covering `.github/promo-flags.json`. Bots (`type: Bot` or a login ending `[bot]`) never count, and team entries are not resolved. A label removed and re-added is judged by its newest event.
   (e) **No `pull_request_target`.** `backlog-scaffolding-guard.yml` must not use it (the guard builds PR code; that trigger would run it with write-scoped secrets); `workflow-yaml.test.ts` fails if `pull_request_target` appears in the file.
   (f) **Accessible name equals visible text.** Prev/next and "See the tour" links show their full label as visible text ("Previous tour: <title>", "Next tour: <title>", "See the tour: <title>") and carry no `aria-label` (WCAG 2.5.3 holds by construction, and same-target duplicates stay distinguishable because the text names the tour). Only the image-only phone-figure link uses `aria-label`.
   (g) **One build-stamp format:** "Recorded 2026-11-10 (build <small>a1b2c3d</small>)" (date first, short sha de-emphasised). The phrase "Recorded from build" is retired everywhere.
   (h) **Length caps are enforced:** every clip is at most 60 s (`clipMaxDurationMs: 60000` in `tools/promo/budgets.json`, checked by `mediatool fresh`) and every tour has 3-6 steps (Zod `min(3).max(6)`). Tour and index copy says "about a minute" and examples read "Watch 0:45".

---

## Requirement IDs (for traceability)

Derived from `requirements.md`; every task below cites these.

| ID | Requirement (source section) |
|---|---|
| R1 | No committed media binaries; CI guard fails new ones (Success 1, Constraints) |
| R2 | Every user-facing feature card has a current screenshot or clip; N tours (Success 2). **N = 8**, derived from `research/features.md` s5 |
| R3 | `/tours` section, one page per tour, built in CI, linked from landing page and README (Success 3, Scope) |
| R4 | Registry docs-coverage fields; blocking gate for new/changed features; non-growing ratchet baseline; coverage report (Success 4, Scope) |
| R5 | Freshness: weekly redeploy regenerates media; check fails if media not from deployed commit; no stale fallback (Success 5, Observability) |
| R6 | Hermetic recording: isolated seeded server, no live `:8543` (Constraints) |
| R7 | Pages artifact size and CI runtime bounded; `ci-job-budget.md` respected (Constraints) |
| R8 | Existing website gates keep passing: asset health, internal links, a11y/Lighthouse, reduced-motion (Constraints) |
| R9 | All media recorded in Grimdark, applied deterministically and verified (Constraints, Scope) |
| R10 | Desktop and mobile form factors; mobile-viewport screenshot set (Constraints) |
| R11 | Repo conventions: Go for new tooling, Bun for JS, e2e conventions incl. no `waitForTimeout` (Constraints) |
| R12 | Lighthouse thresholds, lazy media, `prefers-reduced-motion` (NFR) |
| R13 | Adding a tour = one spec + one content entry (NFR) |
| R14 | Public data: seed data has no real paths, tokens, hostnames, user data (NFR) |
| R15 | Theme called only "Grimdark" in code and copy; persisted-setting migration; CI trademark check (Naming/IP) |
| R16 | Per-clip result summary, notify coverage, actionable gate output (Observability) |
| R17 | Staged rollout and single-switch rollback (Risk Control) |
| R18 | README links to `/tours`; `demo-publish.yml` retired (Decisions after Phase 2) |
| R19 | Transcript per tour (WCAG 1.2.1) (Decisions, `research/ux.md` s4) |
| R20 | Delete stale committed media once pipeline works; scenario-based seeding; `waitForTimeout` cleanup in `clips.ts` (owner-added stories) |

---

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `Tour` | A guided multi-step presentation of one workflow: one clip, 3-6 steps, transcript, links | Go/TS type `TourFrontmatter`; id is kebab-case `TourID` |
| `TourID` | Stable kebab-case id shared by `tours/<id>.mdx`, `tours/<id>.ts` scene file and media names | Newtype `string` in TS (`type TourID = string & {__b:'TourID'}`) and Go `type TourID string` |
| `Scene` | Imperative Playwright steps that drive the app for one Tour | Lives in `tests/e2e/website-demos/tours/<id>.ts` |
| `Clip` | Recorded `.webm` of a Scene, post-processed with ffmpeg | `MediaKind = clip` |
| `Shot` | A still image captured by `page.screenshot`, WebP or PNG by build | `MediaKind = shot`; `poster` is a Shot extracted from a Clip |
| `MediaKind` | Sum type: `clip` / `shot` / `poster` | Exhaustive switch in Go and TS |
| `FormFactor` | Sum type: `desktop` (1280x720) / `mobile` (390x844) | Mobile is Shots only |
| `MediaName` | `<tourOrCard>-<step>[-mobile]`, lowercase, no theme or trademark terms | Newtype; used as filename stem |
| `MediaEntry` | One manifest row: name, kind, formFactor, bytes, sha256, width, height, durationMs, commit, themeId, ffmpegVersion | Go struct + JSON schema |
| `MediaManifest` | `media-manifest.json` emitted per shard and merged; the record-time truth | Never committed |
| `ExpectedMedia` | `expected-media.json` generated from tour frontmatter and `feature-media.json`; the list the pipeline must produce | Generated, gitignored |
| `FreshnessCheck` | Build gate: every `ExpectedMedia` entry present in `MediaManifest`, `commit == GITHUB_SHA`, `themeId == grimdark`, referenced files exist in `dist` | Go `mediatool fresh` |
| `MediaGuard` | PR gate failing added/modified binary media outside `MediaAllowlist` | Go `mediaguard` |
| `MediaAllowlist` | Checked-in glob list (logos, favicons, icons, e2e snapshot dirs) | One file, reviewed |
| `Scenario` | Named seed fixture set (`default`, `dashboard`, `approvals`, `backlog`, `insights`, `notifications`, `automation`, `fanout`, `files`, `history`) | Implements `FileSeeder` and/or `RPCSeeder` (two small interfaces) in `tests/demo/scenario` |
| `ThemeID` | Sum type of built-in theme ids including `grimdark`; `parseThemeId` returns an error for unknown ids, and only the app boundary (ThemeContext, boot script) maps that error to the `clean` default | TS union; Go constant set |
| `LegacyThemeAlias` | One-way map `legacy id -> ThemeID` | Single module; only allow-listed location of the legacy string |
| `BannedTermSet()` | Trademark terms built from fragments: substring (case-insensitive) rules for distinctive fragments, `\b` rules only for short ambiguous ones | `tools/promolib/termscan`, CLI `termcheck` |
| `TermAllowlistEntry` | `path:reason` line; stale entries fail | Checked in |
| `LeakPattern` | Regex set for environment leakage in captured text (`/home/`, `/Users/`, `/tmp/tmp.`, `ghp_`, `.internal`, runner user, emails) | `termscan.LeakPatterns()` (a function, gochecknoglobals), also CLI `-patterns=leak` |
| `DocsCoverage` | Sidecar record per feature id: `id`, `help`, `website`, `tour`, `exempt`, `userFacing`, `status` | `docs/registry/docs-coverage/<encoded-id>.json` (`:`, `/`, `%` percent-encoded; true id inside) |
| `CoverageStatus` | Sum type: `covered` / `exempt` / `grandfathered` / `gap` | Computed, not stored except `grandfathered` |
| `FeatureID` | Registry feature id (e.g. `backlog-flow`) | Newtype in Go |
| `DocSlug` | A member of `DOC_FILES` in `docLoader.ts` | Newtype in Go |
| `WebsiteDocID` | Starlight docs collection id (e.g. `help/getting-started`, `tours/backlog-pipeline`); the type of `tour.helpDoc` and of sidecar `website` | Newtype in Go; validated against the collection at build |
| `RecordShard` | One matrix job recording a subset of Tours against its own server | Round-robin by `TourID` |

26 terms.

---

## Pattern Decisions

**Step 0.5 alternatives considered (strongest chosen: C).**
- A. Standalone Go "promo" service owning recording, manifest and site generation. Strength: one coherent tool. Weakness: rewrites a working Playwright harness and duplicates Astro build-time reads.
- B. Fold everything into `build-features.mjs` and bash. Strength: fewest new files. Weakness: grows bash and a 3-touch JS script into a monolith; violates Go-for-new-tooling.
- **C. Keep the TS recorder and Astro content as producers; add small Go checkers (guard, freshness, term check, docs coverage) joined by two JSON contracts (`ExpectedMedia`, `MediaManifest`).** Strength: each seam testable, reuses proven pieces. Weakness: two languages joined by JSON, mitigated by schema files and round-trip tests.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Recorder scenes | Strategy: one `Scene` module per `TourID`, index in `clips.ts` | GoF | Switch in one big file | Meets "one spec + one entry" (R13) |
| Media producers vs checkers | Pipes-and-filters over JSON files | PoEAA-adjacent (Hohpe) | Shared in-process library across TS and Go | Language split; files are inspectable CI artifacts |
| `MediaManifest`/`ExpectedMedia` | Typed DTO + schema, validated at both ends | PoEAA Data Transfer Object | Ad hoc JSON | Drift caught at the boundary |
| `MediaName`, `TourID`, `FeatureID`, `DocSlug`, `WebsiteDocID` | Newtypes | type-driven-design | raw strings | Prevent id mix-ups (registry id vs help slug vs site id is the central research hazard) |
| `MediaKind`, `FormFactor`, `CoverageStatus` | Sum types with exhaustive handling | type-driven-design | string enums | Compiler/lint-enforced coverage |
| `ThemeID` alias boundary | Parse, don't validate: `parseThemeId(raw) -> ThemeID` resolves aliases once | type-driven-design | scattered `if (id === 'wh40k')` | Single allow-listed location |
| Scenario seeding | Strategy + registry of `Scenario`s, composed per shard | GoF | Per-spec ad hoc seeding | Deterministic, reusable, testable |
| Server start/stop in bash | Facade kept (existing `record-website-demos.sh`) | GoF | Rewrite in Go | Working code; only new steps are Go (ADR-002) |
| Docs-coverage sidecar | Anti-Corruption Layer / seam around scanner-owned JSON | Evans (DDD) | Hand fields in scanner files | Scanners rewrite those files |
| Changed-feature detection | Specification (semantic-fields predicate: a new `id`, or an existing id whose `name`, `description` or `userFacing` differs; `component` and `path`/`filePath` are non-semantic unless the id is new, so moves and renames never fire; ignores `tested`, `testIds`, `lastModified`, `handlerFile`, `markerLine`) | Evans/Fowler | Mere file modification | Refactors and test additions must not fire the gate |
| Registry authority | Repository over the JSON feature files, read-only | PoEAA Repository | Typed catalog as source | Scanner JSON is what CI already diffs |
| Term/leak scanning | Single scanner, three input adapters (source, dist, DOM dump) | GoF Adapter | Three scripts | One term list, one allow-list |
| Gate enablement | Feature flags in checked-in `.github/promo-flags.json` (report-only to blocking) | Fowler Feature Toggle | Repo `vars.*` (not in git, unreadable by fork PRs, silently stay off) | Reviewable one-line flip, single-switch rollback by revert (R17) |
| Seeding lifecycle | Two small interfaces `FileSeeder` (pre-boot) and `RPCSeeder` (post-boot) | ISP | One `Scenario` with both methods | CLI seeds files without a server; RPC step runs after readiness |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `Makefile` (80 touches since 2026-08-01) | Monolith, highest churn | Isolate via seam | New targets in a new `make/promo.mk` included once (the `make/` directory does not exist today and `Makefile` has no `include` line, both verified; Task 1.1.4b creates the directory, the file and the first `include`); `ci` prerequisite added only when a gate turns blocking. |
| `tools/scanner/backend/proto_scanner.go` (50 touches) | Churn-heavy scanner that rewrites JSON | Isolate via seam | Docs fields live in sidecars (ADR-005); scanner untouched. |
| `.github/workflows/deploy-website.yml` | Single serial record job, warn-and-fallback | Extend as-is (+ matrix) | Structure (record, build, deploy, notify) is sound; shards and checks added as steps (Story 2.4.1); setup reuse via existing composite actions. |
| `scripts/record-website-demos.sh` (120 lines bash) | Would grow with shard/manifest logic | Refactor-first (small) | Manifest/summary/shard-plan logic goes to Go in Epic 2.1 before the script gains flags in 2.4.1; bash keeps server lifecycle only. |
| `website/scripts/build-features.mjs` | `DEMO_CLIP_BY_HEADING`, committed-PNG requirement, two states of `features.json` | Extend as-is (refactor of mapping) | Low churn, unit-tested; Story 3.1.2 replaces the mapping with `feature-media.json`. |
| `tests/e2e/website-demos/clips.ts` | Uses `waitForTimeout` against e2e convention | Refactor-first | Story 2.3.1a introduces `hold()` and removes `waitForTimeout` before new scenes build on it. |
| `.github/workflows/build.yml` registry step (`:938-958`) | Existing `--diff-filter=A` test gate | Isolate via seam | Docs gate is a separate step; test-gate logic not edited (Story 4.1.4). |
| Theme rename files | Rename-only, widespread (34 files with content hits plus 2 snapshot PNGs by path; `git grep -il wh40k` excluding `CHANGELOG.md`, `project_plans`) | Extend as-is | Mechanical; risk covered by migration tests, not structure. |
| `Makefile:1293-1308` demo targets, `scripts/demo-post-process.sh`, `tests/demo/demo_test.go`, `tests/e2e/demo.spec.ts` | Regenerate `assets/demo.*` binaries the guard rejects | Delete in Phase 5 (Task 5.1.1b) | Orphaned once README and site use CI media. |
| `tests/demo/seed` | Positional-arg seeder | Extend as-is | Scenario flag added, legacy args kept (Story 2.2.1). |

---

## Migration Plan

- **Ordering constraint**: the alias module, the id switch (`grimdark` added to `THEME_NAMES` in `theme-boot-script.ts:3` and `ThemeName` in `ThemeContext.tsx:24`) and the alias wiring ship in **one PR, as three commits in this order**: (1) alias module + unit tests (dead code), (2) id switch + alias wiring (Task 1.1.3a then 1.1.1b/c), (3) snapshot `git mv`. No release may be cut between commits; otherwise the alias writes `grimdark`, the boot script maps it to `clean`, and users lose their theme.
- **Persisted theme id (localStorage `stapler-theme`)**: one-way read-time migration `wh40k -> grimdark` with write-back (Tasks 1.1.1a-c). Reversibility: alias retained for at least **two releases** (removal date and a tracking issue recorded in the alias module header, Task 1.1.1d; dormant users returning after a gap are the real hazard); irreversible for users once written back (acceptable, new id is canonical). Zero-downtime: boot script maps before paint, so no `clean` flash. Rollback: revert PR; stored `grimdark` would fall to `clean` on the old build, so rollback is only safe before first release (note in PR).
- **`stapler-theme-custom` cache**: key version bump so stale class names are discarded (Task 1.1.1b).
- **User theme files `~/.stapler-squad/themes/*.json`**: legacy `base` accepted and normalized on load, files untouched (Task 1.1.2a). Rollback: revert.
- **Registry sidecars**: new directory `docs/registry/docs-coverage/` generated by `make docs-coverage-baseline`; purely additive; rollback = delete directory and set `docsCoverageEnforce` to `false` in `.github/promo-flags.json`.
- **Theme-id parity**: one test (`web-app` jest) reads `server/user_themes.go`'s built-in id list textually and the TS `ThemeName` union and fails if `grimdark` or any built-in id differs between them (closes the TS/Go/recorder duplication).
- **Snapshots**: `git mv tests/e2e/tests/snapshots/visual-wh40k` to `visual-grimdark`, byte-identical (Task 1.1.3d).

## Observability Plan

- **Logs**: each Go tool prints one actionable line per finding (`path:line term`, `feature-id missing help`, `media name reason`) and exits non-zero on failure; recorder prints a per-clip line (`recorded|failed|skipped`, bytes, sha256).
- **Metrics**: none new (CI only). Job-summary tables (`$GITHUB_STEP_SUMMARY`): per-media result table (R16), docs-coverage count and per-feature gaps, term-check counts.
- **Alerts**: no new pager alerts. Extend the existing `notify` job in `deploy-website.yml` to open the tracking issue for any of `record`, `mediafresh`, `termcheck-dist` failures, and to comment on the flag tracking issue when a flag in `.github/promo-flags.json` is still `false` 14 days after its prerequisite merged (Task 2.4.1b; AC in Story 2.4.1).

## Risk Control

- **Feature flags** (checked in at `.github/promo-flags.json`, see Cross-cutting decision 4): `freshnessEnforce` default `false` until Story 5.1.2; `docsCoverageEnforce` default `false` until Story 4.1.5; `guardsEnforce` default `true` (kill switch, see Cross-cutting 4). Media guard and trademark check are blocking from introduction (new/changed files only; no legacy blast radius).
- **Rollback**: gate off = set the flag `false` via PR; tours = additive, revert PR; **stale fallback**: the old warn-and-fallback step stays in `deploy-website.yml`, marked `# REMOVE IN 5.1.2`, with `mediafresh` running report-only beside it (Task 2.4.1b); Task 5.1.2a removes the old step. After Task 5.1.1 deletes the committed PNGs the fallback has nothing to fall back to, so from then on rollback means redeploying the previous Pages artifact (dispatch the prior commit); this is stated in the Phase 5 PR descriptions. **Guard vs revert**: reverting the Phase 5 deletion would re-add binaries and `mediaguard` would reject it, so `mediaguard` passes a PR carrying the label `allow-media` (read from the PR event labels; honoured only under the Cross-cutting 8d authority rule) and prints a notice; the label is the documented rollback path (ADR-003).
- **Rename rollback after first release**: unsafe (stored `grimdark` falls to `clean` on an old build); the forward fix is keeping the alias and re-adding `grimdark` to the boot-script list; noted in the PR template line for Story 1.1.3.
- **Staged rollout** (requirements Risk Control): (1) pipeline + media guard + rename, (2) tours section, (3) registry fields report-only, (4) flip docs gate to blocking, (5) delete stale media and flip freshness to blocking.

## Unresolved Questions

- [x] ~~`docLoader` slug manifest without a Next build — leading answer: `DOC_FILES` is a plain array literal (`web-app/src/lib/docs/docLoader.ts:10-22`), so the Go checker parses it textually and compares with `web-app/public/docs/*.md`; add a test that fails if the array stops being a literal. Confirm in Task 0.1.1a — blocks Story 4.1.1b — owner: implementer of 0.1.1.~~ — RESOLVED: `DOC_FILES` is a literal; a Go snippet printed the 11 slugs, no missing/extra files (Q1) (spike-notes.md).
- [x] ~~Licence and Astro 7 / Starlight 0.42 compatibility of any third-party tool (`starlight-links-validator`, `starlight-image-zoom`, lychee) — default is to adopt none (ADR-002); only checked if a story proposes one — blocks nothing now (Task 0.1.1e records the check) — owner: implementer of 0.1.1.~~ — RESOLVED: no third-party plugin adopted, check recorded (Q2) (spike-notes.md).
- [x] ~~Mechanism to enable `backlog` and `changelog` feature flags before recording (feature-flag RPC vs seeded config) — blocks Story 2.3.1d, 3.2.2 — owner: 0.1.1c.~~ — RESOLVED: `UpdateFeatureFlag` RPC after boot (Q3) (spike-notes.md).
- [x] ~~Is any selected theme stored server-side (grep `stapler-theme` in Go)? Inferred client-only — blocks Story 1.1.1b — owner: 0.1.1c.~~ — RESOLVED: client-only, `localStorage` key `stapler-theme`, no Go match (Q4) (spike-notes.md).
- [ ] Can approvals, backlog, insights and notification state be seeded through RPC or direct DB writes in `--test-mode`, and can a live terminal pane show canned output via the private tmux socket? — blocks Stories 2.2.1b, 2.2.2 — owner: 0.1.1d. **Fallback if the spike says no for an item** (Task 0.1.1f): record that item from route-level fixtures (`page.route` returning canned ConnectRPC JSON for that RPC only, in the Scene), flagged `seeded: route` in the manifest and noted on the tour; if a whole tour cannot be produced either way, Tyler decides whether to drop it (N becomes 7) before Story 2.2.2 starts.
- [x] ~~`website/src/content.config.ts` shape, `lighthouserc*.json` URL lists, `docs/reference/ci-job-budget.md` limits, whether `playwright.website-demos.config.ts` is scanned by the e2e-conventions lint (Playwright 1.56.1 per `tests/e2e/bun.lock` supports `page.clock.setFixedTime`; that sub-question is closed) — blocks Stories 2.4.1, 3.1.1, 3.1.3, 2.3.1 — owner: 0.1.1b.~~ — RESOLVED: facts recorded (Q6) (spike-notes.md).
- [ ] ADR-001 interpretation: "tours content collection" built as an extended docs schema under `tours/`, not a separate collection — blocks Story 3.1.1 — owner: Tyler (confirm at plan approval; change is local to 3.1.1/3.1.3).
- [ ] Tour storylines, seeded narrative (repo names, PR titles), annotation style and imagery review for Grimdark naming — blocks merge of Epic 3.2 — owner: Tyler.
- [x] ~~Safari/older iOS WebM playback~~ — RESOLVED (Cross-cutting 7e): WebM plus H.264 MP4 `<source>` fallback with poster; Task 3.1.1h implements and tests it.
- [x] ~~Do the website or README have any analytics (page views, referrers) today? — decides how outcome metrics are measured — blocks nothing; Task 0.1.1g records the answer — owner: Tyler for any decision to add measurement.~~ — RESOLVED: no analytics on website or README; baselines in spike-notes (Q8) (spike-notes.md).
- [x] ~~Shelling out to `git` versus go-git~~ — RESOLVED: go-git via `session/git.OpenRepo`, root module, shared package `tools/promolib/gitbase` (Cross-cutting decisions 1-2). Nothing blocks on it.
- [ ] `ubuntu-22.04` runner retirement shifts ffmpeg and fonts; keep pinned for now, migrate as a separate change — blocks nothing — owner: Tyler.
- [ ] Which new feature cards (beyond the 19) are added to `docs/features.md` (product copy is out of scope) — blocks Story 3.3.3 **and the freezing of the card count in Story 3.2.5's AC** (the AC says "every card in `feature-media.json`", not a literal 19) — owner: Tyler.
- [ ] Banned-term list review (Task 1.1.4a proposes the set) — blocks merge of 1.1.4a — owner: Tyler.
- [ ] Imagery (logos, icons, decorative art) cannot be OCR'd: text in shots is machine-gated by the OCR step (Task 2.4.1f); logos and imagery stay an owner review item with a recorded sign-off (CODEOWNERS label on theme files, Task 2.4.1f) — blocks nothing — owner: Tyler.
- [x] ~~`helpDoc` is an in-app slug or a website id~~ — RESOLVED: `WebsiteDocID` (Cross-cutting 6a); Task 3.1.1e validates it. Which real help page `approval-review` points to is chosen in Task 3.1.1e (default `help/getting-started`).

## Dependency Visualization

```
Phase 0   0.1.1 spikes ───────────────────────────────┐ (unblocks 1.1.1b, 2.2.x, 2.3.1b-d, 2.4.1, 3.1.x, 4.1.1b)
Phase 1   1.1.1a alias module ─> 1.1.3a id switch ─> 1.1.1b/c wiring (same PR, 3 commits) ─> 1.1.3d snapshot mv
              1.1.2 server        1.1.1d theme matrix (after 1.1.1b/c)   1.1.3b..c,e rename (parallel after 1.1.3a)
              1.1.4a0 gitbase ─> 1.1.4a1 promo-flags.json + promoguard binary ─> 1.1.4a termcheck(promolib) ─> 1.1.4b0 measure (promoguard)
              2.1.1a mediaguard fill-in (needs 1.1.4a0/a1) + 1.1.4b0 (guard job id) ─> 2.1.1c CODEOWNERS created + protection check + base-blob inputs  [PR A: inputs and tools land, nothing wired]
              ─> 1.1.4b wire termcheck (needs 1.1.4b0, 2.1.1c, 1.1.3a-e; guard job, inputs read from base blob; owns the allow-list-equals-legacy-id-set test) ─> 2.1.1b wire mediaguard  [PR B] ─> 1.1.4c dist/dom modes
              (1.1.4a lands first with fixture-only tests; 1.1.4b blocking wiring comes AFTER rename tasks 1.1.3a-e (or ships in the same PR) because only then does the real tree match the allow-list enumerated in Task 1.1.4a (alias module, server compat file, every planned legacy-id test and doc); otherwise files still hold the term and every PR goes red) ───────┐
Phase 2   2.1.1 guard (needs 1.1.4a0/a1; 2.1.1b wiring needs 2.1.1c) ──┤
          2.1.2 manifest+fresh+expected (2.1.2d reuse mode) ──┐       │
          2.2.1 scenario fw ─> 2.2.2 scenarios ──┐                    │
          2.3.1 recorder (needs 1.1.3, 2.1.2) ───┼─> 2.4.1 workflow ──┤   (inside 2.4.1: 2.4.1g -> 2.4.1e, 2.4.1h -> 2.4.1f)
Phase 3   3.1.1 components/schema (3.1.1e helpDoc check, 3.1.1f smoke rendering, 3.1.1d tours.spec.ts) ─> 3.1.2 features-media ─> 3.1.3 gates
          3.2.1..3.2.6 tours/shots (need 2.2.2, 2.3.1, 3.1.1; parallel per tour; ACs are frontmatter/build ACs only, no sidecars) ─> 3.3.x entry points (3.3.2c README link check)
Phase 4   4.1.1 schema+checker (4.1.1d before 4.1.1b) ─> 4.1.2 changed-detect ─> 4.1.3 baseline ─> 4.1.4 CI (report-only) ─> 4.1.7 link tours (tour/sidecar ACs live here, after Phase 3 tours exist) ─> [observe: >=30 merged PRs or 4 weeks] ─> 4.1.5 blocking ; 4.1.6 docs
          Sidecars for tour-covered ids are authored only in 4.1.7a; Phase 3 never depends on them.
Phase 5   (after first green full deploy) 5.1.1 delete stale media ─> [soak: 10 consecutive green full runs incl. >=2 weekly cron] ─> 5.1.2 flip freshness ─> 5.1.3 docs
```

Shared-file serialization: `.github/workflows/deploy-website.yml` (2.4.1 then 5.1.2), `.github/workflows/backlog-scaffolding-guard.yml` and `website/tests/unit/workflow-yaml.test.ts` (1.1.4b, then 2.1.1b, then 2.1.1c's permissions case), `.github/promo-flags.json` (created 1.1.4a1, flipped in 4.1.5 / 5.1.2), `.github/CODEOWNERS` (created in 2.1.1c, then 2.4.1f), `website/src/content.config.ts` (3.1.1 only), `make/promo.mk` (created 1.1.4b, appended later; `.PHONY` entries live inside it and `make help` is checked), `website/astro.config.mjs` (1.1.3b then 3.1.1b), `docs/reference/ci-job-budget.md` (2.4.1 before/after, 5.1.3).

---

# Phase 0: Spikes

## Epic 0.1: Close open technical unknowns

**Goal**: Answer the blocking Unresolved Questions before dependent stories start.

#### Story 0.1.1: Technical spikes
**As an** implementer, **I want** the open unknowns resolved and recorded, **so that** later stories build on facts, not guesses. (R6, R11, R14)
**Acceptance Criteria**:
- Each spike writes its finding to `project_plans/promo-materials-refresh/implementation/spike-notes.md` and strikes the matching Unresolved Question.
  - *Given* the `DocSlug` list in `docLoader.ts`, *When* a Go snippet parses the array literal, *Then* it prints the 11 slugs (`what-is-stapler-squad` ... `workflows`) with no Next build.
- Seeding feasibility is proven for at least one non-session `Scenario` item.
  - *Given* a `--test-mode` server on a free port with `STAPLER_SQUAD_TMUX_SOCKET=ssq-spike`, *When* an approval rule `git-push-ask` is seeded, *Then* `/rules` lists it after page load.
**Files**: `project_plans/promo-materials-refresh/implementation/spike-notes.md`

##### Task 0.1.1a: docLoader manifest spike (XS)
- Parse `DOC_FILES` from `web-app/src/lib/docs/docLoader.ts` with a 20-line Go test program; compare to `web-app/public/docs/*.md` listing. Trace: R4.
- Files: `web-app/src/lib/docs/docLoader.ts` (read), spike-notes.

##### Task 0.1.1b: Repo-fact inventory (S)
- Read `website/src/content.config.ts`, `website/lighthouserc*.json`, `docs/reference/ci-job-budget.md`, `tests/e2e/playwright.website-demos.config.ts`, e2e convention lint scope. Record limits and URL lists. Also: run `tools/ci/count-pr-checks.py` for the baseline; measure the current `website-ci.yml` and `deploy-website.yml` record-job durations (`gh run list`) to set the smoke caps; and confirm whether `build.yml` publishes a `stapler-squad` binary artifact on main that `website-ci` can download (if not, Task 2.4.1c builds it with caches and the 6-minute target is re-derived). Trace: R7, R8, R11, R12.
- Files: those paths (read), spike-notes.

##### Task 0.1.1c: Flags and theme storage (S)
- Find how `backlog`/`changelog` flags are set (feature-flag RPC, `docs/registry/features/backend/feature-flag/`); grep Go for any server-side selected-theme field (`stapler-theme`, preferences). Trace: R9, R6.
- Files: `web-app/src/app/FeatureGuard.tsx`, `server/` (read), spike-notes.

##### Task 0.1.1d: Seeding feasibility (M)
- Prototype seeding approvals, a backlog item, a notification and canned terminal scrollback (tmux pane running `cat` of a fixture on the private socket) against a `--test-mode` server; decide RPC-after-boot vs file/DB seed per item (this fixes which scenarios implement `FileSeeder` vs `RPCSeeder`); record sessions' status constraint from `tests/demo/helpers.go:152-155`. Trace: R6, R14.
- Files: `tests/demo/helpers.go`, `tests/demo/seed/main.go` (read), spike-notes.

##### Task 0.1.1e: Third-party licence check record (XS)
- Record that no third-party Astro/Starlight plugin is adopted (ADR-002); if one is proposed later, WebSearch licence and Astro 7 / Starlight 0.42 compatibility first. Trace: R2.
- Files: spike-notes.

##### Task 0.1.1f: Seeding fallback decision and registry/rename inventories (S)
- If 0.1.1d fails for any item, define the route-fixture fallback per item (see Unresolved Questions) and add it to the affected story's notes. Also record by command: `git grep -il wh40k -- . ':!CHANGELOG.md' ':!project_plans'` (34 files at planning time, `| wc -l`) to split Task 1.1.3a per directory; registry counts (`find docs/registry/features/backend -name '*.json' | wc -l` = 349 files; `frontend` = 148 files; root (`-maxdepth 1`) = 13 files holding 15 entries; total 510 files; one duplicated id `launcher_presets:get` across `backend/launcher-presets/get.json` and `backend/launcher_presets/get.json`; 163 default-userFacing ids = 148 + 15; 20 of those contain `:`); and the count of those 163 ids that lack docs (replaces the unverified "87 / 80" figures). Trace: R2, R4, R15.
- Files: spike-notes.

##### Task 0.1.1g: Analytics and outcome-metric baseline inventory (S)
- Record by command what measurement exists today: grep `website/` for analytics snippets and Pages/GitHub settings; `gh api repos/{owner}/{repo}/traffic/views` and `/traffic/popular/paths` (14-day window, needs push access) and release download counts (`gh api repos/{owner}/{repo}/releases --jq '.[].assets[] | {name, download_count}'`) as the zero-tracking baselines for the outcome metrics in `requirements.md`. Write the baselines (or "no data") into `spike-notes.md`. **Sampling schedule** (the traffic API keeps only 14 days, so a 4-week reading needs overlapping windows): record the readings in an "Outcome samples" table in `spike-notes.md` (date, source command, value, who) at four points: **pre-launch** (before Story 3.3.1 merges), **launch** (the first deploy that serves a tour), **week 2** and **week 4** after launch; the launch date is written into the table when it happens and Tyler owns each sample. Attribution limit: releases, CI changes, bot traffic and unrelated promotion move the same numbers, so a delta is evidence consistent with the tours working, not proof, and is reported beside the "provisional" label. If the site has no analytics, **no tracking is added by this plan**: outcome metrics are marked "to be measured" with Tyler as named owner, using the GitHub traffic and release-download numbers sampled at launch and at 4 weeks; adding any analytics is a separate owner decision. Trace: R2, R3.
- Files: spike-notes.

---

# Phase 1: Grimdark rename and checks

## Epic 1.1: Grimdark rename, alias migration, trademark check

**Goal**: Code, copy and storage say only "Grimdark"; existing users keep their theme; CI blocks regressions. (R15, R9)

#### Story 1.1.1: Legacy theme alias and persisted-id migration
**As an** existing user, **I want** my stored theme to carry over, **so that** I land on Grimdark instead of Clean. (R15)
**Acceptance Criteria**:
- `parseThemeId` maps the legacy id to `ThemeID` `grimdark` and ThemeContext writes the new id back.
  - *Given* `localStorage['stapler-theme'] = 'wh40k'`, *When* the app mounts, *Then* `document.documentElement` has the Grimdark class and `localStorage['stapler-theme'] === 'grimdark'`.
- The boot script resolves the alias before first paint.
  - *Given* `localStorage['stapler-theme'] = 'wh40k'` and a cold load, *When* the HTML shell runs `theme-boot-script`, *Then* the Grimdark class is on `<html>` before React hydrates (no `clean` flash).
- A stale custom-theme cache is discarded.
  - *Given* `stapler-theme-custom = {"id":"mine","cls":"oldHashedClass",...}` from the previous key version, *When* the app mounts, *Then* the cache is rebuilt and the page is styled.
**Ordering**: Task 1.1.1a first (dead code), then Task 1.1.3a (id switch), then 1.1.1b and 1.1.1c; all in one PR (see Migration Plan ordering constraint). These ACs are verified after 1.1.3a, not before.
- Theme-id parity test passes.
  - *Given* `ThemeName` in `ThemeContext.tsx`, `THEME_NAMES` in `theme-boot-script.ts` and the built-in list in `server/user_themes.go`, *When* the parity jest test runs, *Then* all three contain `grimdark` and the same built-in ids.
- Boot script handles every stored-value class (pre-mortem P1-2).
  - *Given* the real built boot script and `localStorage['stapler-theme']` set in turn to `wh40k`, `grimdark`, `custom:x` and an unknown id `nope`, *When* it runs, *Then* `<html>` carries the Grimdark class (for `wh40k`), the Grimdark class (for `grimdark`), the custom-theme path (for `custom:x`) and the `clean` class (for `nope`), respectively.
- Alias lifetime is bounded.
  - *Given* `legacy-theme-ids.ts`, *When* read, *Then* its header names a removal date at least two releases after the first release containing it, and a tracking issue exists for the removal (Task 1.1.1d).
- Recorder theme check cannot be softened.
  - *Given* any `freshnessEnforce` value, *When* a shard's `assertGrimdark` fails, *Then* the shard fails (Task 2.3.1c).
**Files**: `web-app/src/lib/theme/legacy-theme-ids.ts`, `web-app/src/lib/theme/theme-id-parity.test.ts`, `web-app/src/lib/contexts/ThemeContext.tsx`, `web-app/src/styles/theme-boot-script.ts`, `web-app/src/lib/contexts/ThemeContext.test.tsx`, `web-app/build/__tests__/theme-boot.test.ts`, `tests/e2e/theme-fouc.spec.ts`

##### Task 1.1.1a: Alias module (S)
- Create `legacy-theme-ids.ts` exporting `LEGACY_THEME_IDS`, `parseThemeId`; unit tests incl. unknown id -> `clean` default only at the app boundary. Trace: R15.
- Files: `web-app/src/lib/theme/legacy-theme-ids.ts`, `web-app/src/lib/theme/legacy-theme-ids.test.ts`.

##### Task 1.1.1b: ThemeContext migration + cache key bump (S)
- Runs after 1.1.3a. Read stored value, map via `parseThemeId`, write back new id; version `stapler-theme-custom`; add the parity test. Needs 0.1.1c answer on server-side preference. Trace: R15.
- Files: `ThemeContext.tsx`, `ThemeContext.test.tsx`.

##### Task 1.1.1c: Boot script alias + FOUC e2e case (S)
- Runs after 1.1.3a. Import alias map into `theme-boot-script.ts` build output; add `theme-fouc.spec.ts` case seeding the legacy id (keep `// @feature` header, no `waitForTimeout`). Trace: R15.
- Files: `theme-boot-script.ts`, `web-app/build/__tests__/theme-boot.test.ts`, `tests/e2e/theme-fouc.spec.ts`.

##### Task 1.1.1d: Stored-value matrix, server end-to-end, alias lifetime (S)
- Runs after 1.1.1b/c in the same PR. Extend `web-app/build/__tests__/theme-boot.test.ts` to load the real built boot script (not a copy) and assert the exact class for stored `wh40k`, `grimdark`, `custom:x` and an unknown id. Add a Go test (with Task 1.1.2a) that a user theme file with `"base":"wh40k"` is returned by `GET /api/themes` with base `grimdark` and that any endpoint echoing a base (the themes list the picker consumes) returns the normalized value. Put the removal date (at least two releases out) in the alias module header and open the tracking issue (`gh issue create`, URL recorded in the PR). Trace: R15, R17.
- Files: `web-app/build/__tests__/theme-boot.test.ts`, `web-app/src/lib/theme/legacy-theme-ids.ts` (header), `server/user_themes_test.go`.

#### Story 1.1.2: Server accepts legacy user-theme base
**As a** user with a custom theme file, **I want** `"base": "wh40k"` to keep working, **so that** my theme still loads. (R15)
**Acceptance Criteria**:
- Legacy base normalizes on load; file untouched.
  - *Given* `~/.stapler-squad/themes/ember.json` with `"base":"wh40k"`, *When* `GET /api/themes` runs, *Then* the theme `ember` is returned with base `grimdark` and a deprecation log line, and the file bytes are unchanged.
- Docs updated together.
  - *Given* `docs/how-to/add-user-themes.md` changed, *When* `sync-content -- --check` runs, *Then* it passes because the website copy changed in the same PR.
**Files**: `server/user_themes.go`, `server/user_themes_test.go`, `docs/how-to/add-user-themes.md`, `website/src/content/docs/help/add-user-themes.md`

##### Task 1.1.2a: Normalize legacy base in Go (S)
- Replace `"wh40k"` in `builtinThemeBases` with `"grimdark"` plus a `legacyThemeBases` map normalized on load; table test; log deprecation via slog; cover every API path that persists or echoes a base (the themes list consumed by the picker) with a test that a stored legacy base resolves to `grimdark` end to end (Task 1.1.1d). Trace: R15.
- Files: `server/user_themes.go`, `server/user_themes_test.go`, `server/services/content_search_fixture_test.go` (verify the hit is noise).

##### Task 1.1.2b: Docs pair update (XS)
- Edit the source how-to to say Grimdark and mention the legacy id as deprecated; run `node website/scripts/sync-content.mjs` to regenerate the website copy. Trace: R15.
- Files: `docs/how-to/add-user-themes.md`, `website/src/content/docs/help/add-user-themes.md`.

#### Story 1.1.3: Rename id, label, css, tests, snapshots
**As a** maintainer, **I want** every identifier, label and test name to say Grimdark, **so that** no trademarked term remains in source. (R15, R9)
**Acceptance Criteria**:
- Picker shows the new label.
  - *Given* ThemePicker is rendered, *When* the theme list is read, *Then* the entry label is "Grimdark" and the description is unchanged.
- Snapshot move is byte-identical.
  - *Given* `tests/e2e/tests/snapshots/visual-wh40k/visual-regression.spec.ts/*.png`, *When* `git mv` to `visual-grimdark/...` is committed alone, *Then* `git diff -M --stat` shows 100% renames.
- Legacy command still works without a visible legacy name.
  - *Given* Omnibar input `>theme wh40k`, *When* the detector runs, *Then* it resolves to the `grimdark` theme through `parseThemeId`.
**Files**: the exact list produced by `git grep -il wh40k -- . ':!CHANGELOG.md' ':!project_plans'` (34 files; partitioned across the tasks below, with `server/user_themes.go` and `docs/how-to/add-user-themes.md` + its website copy in Story 1.1.2) plus the two snapshot PNGs. Acceptance for the whole story: that grep (with the allow-list entries excluded) returns zero lines.

##### Task 1.1.3a: web-app id switch (S) — first task of the PR
- Add `grimdark` to `ThemeName` (`ThemeContext.tsx:24`) and `THEME_NAMES` (`theme-boot-script.ts:3`) and rename the theme constant; the alias wiring follows in 1.1.1b/c. Codemod with explicit mapping (`wh40k->grimdark`, `WH40K->Grimdark`, `wh40kTheme->grimdarkTheme`), dry-run first, not committed. Skip the alias module. Trace: R15.
- Files: `web-app/src/lib/contexts/ThemeContext.tsx`, `web-app/src/styles/theme-boot-script.ts`.

##### Task 1.1.3a2: web-app styles, picker, omnibar (S)
- Files: `web-app/src/styles/theme.css.ts`, `web-app/src/styles/theme-contract.css.ts`, `web-app/src/components/settings/ThemePicker.tsx`, `web-app/src/components/settings/ThemePicker.css.ts`, `web-app/src/lib/omnibar/detectors/CommandDetector.ts` (+ its test if it names the id). Trace: R15.

##### Task 1.1.3a3: web-app tests, build fixtures, scripts (S)
- Files: `web-app/build/plugins/__tests__/vanilla-extract.test.ts`, `web-app/build/__tests__/fixtures/app/src/styles/theme.css.ts`, `web-app/build/__tests__/html-shell.test.ts`, `web-app/build/__tests__/theme-boot.test.ts`, `web-app/scripts/check-theme-contrast.ts`, `web-app/src/components/command-bar/__tests__/commandBarContrast.test.ts`, `web-app/src/lib/unfinished/chipContrast.test.ts`, `web-app/src/styles/__tests__/overlayContrastTokens.test.ts`. Trace: R15.

##### Task 1.1.3b: website rename (S)
- `git mv website/src/styles/wh40k.css grimdark.css`; update `astro.config.mjs` customCss, `NoThemeSelect.astro`, `website/tests/unit/wh40k-theme.test.ts` (rename + content). Trace: R15.
- Files: `website/src/styles/grimdark.css`, `website/astro.config.mjs`, `website/src/components/NoThemeSelect.astro`, `website/tests/unit/grimdark-theme.test.ts`.

##### Task 1.1.3c: e2e config, helpers, baseline (S)
- Rename Playwright project `visual-wh40k` -> `visual-grimdark` in `tests/e2e/playwright.config.ts`, `playwright.dev-mode.config.ts`; update `helpers/theme-fixtures.ts`, `global-setup.dev-mode.ts`, `tools/parity-collect.ts`. `baseline/next-e2e-baseline.json` is a generated artifact: regenerate it with `parity-collect` instead of codemodding, and check the diff is rename-only. Trace: R15, R11.
- Files: those five plus the baseline JSON.

##### Task 1.1.3c2: e2e specs and recorder (S)
- Files: `tests/e2e/visual-regression.spec.ts`, `theme-fouc.spec.ts` (case added in 1.1.1c; coordinate order), `theme-background.spec.ts`, `notification-contrast-over-xterm.spec.ts`, `tests/e2e/website-demos/recorder.ts`. Keep `// @feature` headers; no `waitForTimeout`. Trace: R15, R11.

##### Task 1.1.3d: Snapshot directory move (XS)
- Own commit, last in the PR: `git mv tests/e2e/tests/snapshots/visual-wh40k tests/e2e/tests/snapshots/visual-grimdark` (only this one of the five variant snapshot directories is renamed). Trace: R15, R1.
- Files: 2 PNGs.

##### Task 1.1.3e: Docs mentions (XS)
- Update `docs/architecture/frontend-audit-2026.md`. (`docs/how-to/add-user-themes.md` is Task 1.1.2b; `docs/reference/css-architecture.md` has no hit and is dropped from the list; verify with the grep.) `CHANGELOG.md` stays. Trace: R15.
- Files: `docs/architecture/frontend-audit-2026.md`.

#### Story 1.1.4: Trademark-term check
**As a** maintainer, **I want** CI to fail on trademarked terms in source and published content, **so that** the naming rule holds. (R15)
**Acceptance Criteria**:
- Distinctive fragments match as case-insensitive substrings, so glued identifiers are caught.
  - *Given* tracked files containing `wh40kTheme`, `wh40kStart`, `wh40kBlock`, `isWH40K`, `Warhammer40K`, `WH-40K`, `wh_40k`, `wh 40k` and `WH40K theme` (table test, one case each), *When* `termcheck -mode=source` runs, *Then* every case exits 1 printing `path:line` and the matched term; a file in `website/src/content/docs/help/x.md` containing `WH40K theme` is reported as `website/src/content/docs/help/x.md:1`.
- The short ambiguous term is `\b`-bounded; noise is skipped.
  - *Given* a file with `40k` as a word (`a 40k budget`) and another with `x240kb` and base64 `...40k...` in `go.sum` / `bun.lock`, *When* the scan runs, *Then* `a 40k budget` is reported, `x240kb` is not, and lockfiles and binaries are skipped.
- Skip rules use path components.
  - *Given* `docs/project_plans/x.md` and `project_plans/y/z.md` containing a banned term, *When* the scan runs, *Then* both are skipped (any path component named `project_plans`), while `docs/other/x.md` is reported.
- The Go tool is shared, not private.
  - *Given* `tests/demo/scenario/leak_test.go` and `tools/docscoverage`, *When* `go build ./...` runs, *Then* they compile against `tools/promolib/...` (an `internal/` placement would fail with "use of internal package not allowed").
- Stale allow-list entries fail.
  - *Given* allow-list line `web-app/src/lib/theme/legacy-theme-ids.ts:legacy alias` and the file no longer contains the term, *When* the scan runs, *Then* it exits 1 naming the stale entry.
- The allow-list equals the real legacy-id file set (checked after the rename, Task 1.1.4b, not before).
  - *Given* the tree after Tasks 1.1.3a-e, *When* `TestAllowlistEqualsLegacyIdFileSet` (`tools/promo/internal/termcheck/termcheck_test.go` (`TestAllowlistEqualsLegacyIdFileSet`, lives beside the scanner it exercises rather than under `cmd/`)) lists the files containing the legacy id (the index listing minus the skip rules) and compares them with `tools/promo/termcheck/allowlist.txt`, *Then* the two sets are equal (no missing entry, no stale entry). This test runs in Task 1.1.4b, never in Task 1.1.4a, because before the rename lands the tree still holds the term in many more files.
- Absent allow-list means empty (Cross-cutting 8a).
  - *Given* a base tree with no `tools/promo/termcheck/allowlist.txt`, *When* `termcheck -mode=source` runs, *Then* it treats the allow-list as empty and reports every banned-term file; a present but malformed allow-list exits 2.
- Dist and DOM-text modes catch published leakage.
  - *Given* a `website/dist/tours/x/index.html` whose `alt` contains a banned term, *When* `termcheck -mode=dist` runs, *Then* it exits 1 with the file and term.
- Runs on PRs that touch only website, assets, README or docs.
  - *Given* a PR changing only `website/src/content/docs/x.md` to include a banned term, *When* CI runs, *Then* the guard job (Cross-cutting 8b: `guard`, display name `pr-diff-guards`, or `promo-guards` per Task 1.1.4b0) of `backlog-scaffolding-guard.yml` runs `termcheck -mode=source` and fails; and `website/tests/unit/workflow-yaml.test.ts` asserts that workflow's `on.pull_request` has no `paths`/`paths-ignore`, that `pull_request_target` appears nowhere in the file (Cross-cutting 8e), and that the guard job has a checkout with `fetch-depth: 2`, a `setup-go` step with caching, and a `timeout-minutes` matching Task 1.1.4b0.
- `40k` is scoped and has a benign golden test (pre-mortem P1-5).
  - *Given* a golden list of benign strings (`40k tokens`, `a 40k context`, `x240kb`, a base64 blob containing `40k`), *When* the `\b40k` rule runs, *Then* it applies only under the published paths `website/`, `web-app/src/`, `README.md`, `assets/`, `docs/` and reports the `40k` word only there; the distinctive-fragment substring rules apply to the whole tracked tree. Golden file: `tools/promolib/termscan/testdata/benign.txt`.
- Media-text mode blocks the upload unconditionally.
  - *Given* a DOM text dump containing a banned fragment, *When* `termcheck -mode=media-text` runs in the record job, *Then* the job fails before the Pages artifact upload, independent of `freshnessEnforce` (Task 2.4.1f).
- Banned terms are functions, not globals.
  - *Given* `termscan.BannedTermSet()` and `termscan.LeakPatterns()`, *When* `make lint` runs, *Then* `gochecknoglobals` reports nothing for `tools/promolib/**`.
**Files**: `tools/promo/cmd/promoguard/main.go`, `tools/promo/cmd/termcheck/main.go`, `tools/promolib/gitbase/*.go`, `tools/promolib/termscan/*.go`, `tools/promo/termcheck/allowlist.txt`, `.github/promo-flags.json`, `make/promo.mk`, `.github/workflows/backlog-scaffolding-guard.yml`, `website/tests/unit/workflow-yaml.test.ts`

##### Task 1.1.4a0: `gitbase` shared go-git package (M)
- Created here (moved from Task 2.1.1a, which now only consumes it; Cross-cutting 7d): `tools/promolib/gitbase/{index.go,diff.go,blob.go,gitbase_test.go}` over `session/git.OpenRepo` (Cross-cutting 2): index listing (`ls-files`), tree diff of `-base` vs HEAD (adds and modifies by path; deletions reported separately), `ReadBlob(base, path)` (base-tree lookup, used for gate inputs per Cross-cutting 7a), and the loud failure (exit 2, message naming the event and `-base`) when `-base` is the default `HEAD^1` and HEAD has fewer than two parents. Tests against fixture repos built with go-git (no CLI): single-parent HEAD must fail loudly, two-parent merge passes, `ReadBlob` returns the base content when the PR tree changed the file, missing path returns a typed not-found error. Trace: R1, R11.
- Files: `tools/promolib/gitbase/{index.go,diff.go,blob.go,gitbase_test.go}`.

##### Task 1.1.4a1: `promo-flags.json` and the `promoguard` binary (S)
- Created here (moved from Task 2.1.2a). `.github/promo-flags.json` = `{"freshnessEnforce": false, "docsCoverageEnforce": false, "guardsEnforce": true}` and a Go unit test (`tools/promo/internal/flags`) that asserts all three keys exist, are booleans, and the defaults at introduction, plus bootstrap cases (Cross-cutting 8a): a base tree with no `promo-flags.json` yields `guardsEnforce=true`, `freshnessEnforce=false`, `docsCoverageEnforce=false`, and a present but malformed file exits 2; the loader reads the file from a base blob via `gitbase.ReadBlob` (never the working tree, Cross-cutting 7a) and from the working tree only on `push`, `schedule` and `workflow_dispatch` runs (`mediatool fresh`), where the tree is already merged; `docscoverage` on `pull_request` reads `docsCoverageEnforce` from the base blob too, so a PR cannot flip its own gate. `tools/promo/cmd/promoguard/main.go` is the one combined binary for the always-run guards, with subcommands `termcheck` and `mediaguard`; at this point `mediaguard` walks the diff and extension list through `gitbase` and returns success, and Task 2.1.1a fills in allow-list, size ceiling and label logic. This is the binary Task 1.1.4b0 measures and 1.1.4b/2.1.1b wire; the standalone `cmd/termcheck` and `cmd/mediaguard` mains become thin wrappers over the same packages. Trace: R17, R15, R1.
- Files: `.github/promo-flags.json`, `tools/promo/internal/flags/{flags.go,flags_test.go}`, `tools/promo/cmd/promoguard/main.go`.

##### Task 1.1.4a: Go scanner (source mode) in the shared package (M)
- Depends on 1.1.4a0. `tools/promolib/termscan` (non-internal, see Cross-cutting 1): file walk via `tools/promolib/gitbase` index listing (Task 1.1.4a0; go-git, no CLI). Skip: lockfiles (`go.sum`, `bun.lock`, `package-lock.json`, `*.lock`), binary files (NUL-byte sniff), path components `.next`, `build`, `project_plans`, `node_modules`, and `CHANGELOG.md`; the tool's own dir is excluded by path.
- **Proposed banned set** (owner review, Unresolved Questions), built from string fragments so the source never contains the literals: substring, case-insensitive: `wh40k`, `warhammer`, `wh-40k`, `wh_40k`, `wh 40k`, `games workshop`, `space marine`, `adeptus`, `astartes`, `black library`; `\b`-bounded case-insensitive: `40k`. Not banned: generic fantasy words (`emperor`, `heresy`, `citadel`, `imperium` alone). Each term carries a one-line rationale in the code.
- `BannedTermSet()` is a function returning the compiled terms (no package-level var; `gochecknoglobals` is enabled and `tools/promolib/**` is not exempt). AC: `make lint` passes with no `//nolint:gochecknoglobals`.
- Allow-list parse (`path:reason`) with stale detection; table tests for every case in the Story ACs. The initial allow-list is **enumerated**, one `path:reason` line each: `web-app/src/lib/theme/legacy-theme-ids.ts`, `web-app/src/lib/theme/legacy-theme-ids.test.ts`, `web-app/src/lib/contexts/ThemeContext.test.tsx` (stored legacy id case), `web-app/build/__tests__/theme-boot.test.ts` (stored matrix), `tests/e2e/theme-fouc.spec.ts` (legacy case, Task 1.1.1c), `web-app/src/lib/omnibar/detectors/CommandDetector.test.ts` (`>theme <legacy>` case, Story 1.1.3 AC), `server/user_themes.go` and `server/user_themes_test.go` (compat constant and test), `docs/how-to/add-user-themes.md` and `website/src/content/docs/help/add-user-themes.md` (Task 1.1.2b mentions the legacy id as deprecated), `.github/CODEOWNERS` only if it names the term, and `CHANGELOG.md` (also skipped by path). The tests in this task use fixtures only (parse, stale detection, the Story ACs); the test that the allow-list equals the real set of files containing the legacy id is Task 1.1.4b, which runs after the rename (Story 1.1.4 AC). `\b40k` is applied only under the published paths (see the Story AC) and ships with the benign-string golden test. Trace: R15.
- Files: `tools/promo/cmd/termcheck/main.go`, `tools/promolib/termscan/{scan.go,terms.go,scan_test.go}`, `tools/promo/termcheck/allowlist.txt`.

##### Task 1.1.4b0: Guard-job runtime measurement and budget (S)
- Before wiring, measure on a throwaway PR branch the guard job with: `actions/checkout` with `fetch-depth: 2` from the start (not `1` then deepen), `actions/setup-go` with `cache: true` (`cache-dependency-path: go.sum`), and one prebuilt `go build -o` of the `promoguard` binary created by Task 1.1.4a1 (subcommands `termcheck -mode=source` and `mediaguard`; `mediaguard` is the real `gitbase` diff walk with the allow-list and label logic still to come, so the measured work is representative; the task depends on 1.1.4a0, 1.1.4a1 and 1.1.4a). Measure both a **warm** run and a **cold-cache** run (empty Go build and module cache, as after a cache eviction) and record checkout, setup-go and build seconds for each in `spike-notes.md`. Set the target: job p95 <= 5 minutes warm; if the measurement exceeds the existing `timeout-minutes: 8` (the job's own comment: baseline avg 4m / max 10.1m), raise `timeout-minutes` in the same PR and record the new value in `docs/reference/ci-job-budget.md`. Also confirm go-git `OpenRepo`, `HEAD^1` resolution and the tree diff work on a depth-2 checkout (add to spike 0.1.1d notes).
- **Hard gate (pre-mortem P1-1)**: if the warm p95 of the extended `guard` job is above 5 minutes, or the cold run exceeds its `timeout-minutes`, **do not extend `guard`**. Instead add a separate always-reporting job `promo-guards` in the same workflow (no `paths:` filter; the job decides inside via a first step that lists changed paths with `gh api repos/{owner}/{repo}/pulls/{n}/files` and exits early with success when no media, website, docs, README or source-term-relevant path changed, so the check always reports a result). Tasks 1.1.4b and 2.1.1b then target job id `promo-guards` and the workflow-yaml tests assert that id instead of `guard`; record the decision and numbers in `spike-notes.md` and ADR-003.
- **Checkout-free alternative to evaluate in the same measurement**: `mediaguard` needs only the changed-path list and blob sizes, both available from `gh api .../pulls/N/files` plus the contents API, with no clone and no go-git edge; if it keeps `guard` under 5 minutes, use it for `mediaguard` on the PR path and keep go-git (`gitbase`) only for `docscoverage` in `build.yml`, where a checkout exists. `termcheck -mode=source` scans changed files fetched the same way on PRs; the full-tree scan runs in `build.yml`. Record the chosen variant.
- **Kill switch**: both guard steps run `continue-on-error: ${{ !fromJSON(guardsEnforce) }}` where `guardsEnforce` is read from the **base blob** of `.github/promo-flags.json` (`promoguard flags -base HEAD^1`, file created in Task 1.1.4a1; Cross-cutting 7a), never from the PR tree; setting it `false` by PR turns the guards report-only for PRs opened after that PR merges (the flipping PR is judged by the old value). Trace: R15, R17.
- Files: `project_plans/promo-materials-refresh/implementation/spike-notes.md` (record), `docs/reference/ci-job-budget.md`.

##### Task 1.1.4b: Wire into the always-run guard job and Makefile seam (S)
- Depends on 1.1.4b0, on rename tasks 1.1.3a-e (blocking wiring lands after them or in the same PR; see dependency graph) and on Task 2.1.1c (`.github/CODEOWNERS` exists and branch protection requires code-owner review; Cross-cutting 8c), so it ships as PR B after the inputs of PR A are merged (Cross-cutting 8a). **Creates the `make/` directory and `make/promo.mk`** (neither exists today: verified, `ls make` fails and `Makefile` has no `include` line; own `.PHONY` list; `make help` shows its targets) with a `termcheck` target, and adds the first `include make/promo.mk` to `Makefile`. Add steps to the guard job (Cross-cutting 8b: `guard`/`pr-diff-guards`, or `promo-guards` if 1.1.4b0's hard gate fired) of `.github/workflows/backlog-scaffolding-guard.yml`: `actions/checkout` with `fetch-depth: 2`, `setup-go` with cache, then the prebuilt `promoguard termcheck -mode=source` run; apply the timeout from 1.1.4b0. Wire the `guardsEnforce` kill switch (Cross-cutting 4). Add the workflow-yaml test case (job selected by the one id constant recorded by 1.1.4b0; also asserts `pull_request_target` appears nowhere) and `TestAllowlistEqualsLegacyIdFileSet` (Story 1.1.4 AC; first point at which the real tree is compared with the allow-list). Add a note to ADR-004 consequences. Trace: R15, R17.
- Files: `make/promo.mk`, `Makefile`, `.github/workflows/backlog-scaffolding-guard.yml`, `website/tests/unit/workflow-yaml.test.ts`, `tools/promo/internal/termcheck/termcheck_test.go` (`TestAllowlistEqualsLegacyIdFileSet`, lives beside the scanner it exercises rather than under `cmd/`).

##### Task 1.1.4c: dist, media-text and leak modes (S)
- Add `-mode=dist` (text under `website/dist`, search index, file/media names, manifest JSON, `<title>`/OG meta, sitemap) and `-mode=media-text` (dir of DOM text dumps), and export `termscan.LeakPatterns()` as a function (no package var: `gochecknoglobals`) (`/home/`, `/Users/`, `/tmp/tmp.`, `ghp_`, `.internal`, runner user, emails; R14) used by CLI `-patterns=leak` and by `tests/demo/scenario/leak_test.go`. Scenario fixture strings are covered by that leak test and by `-mode=source` (tracked). Wired into workflows in Story 2.4.1. Trace: R15, R14.
- Files: `tools/promolib/termscan/dist.go`, `leak.go`, tests.

---

# Phase 2: Media pipeline

## Epic 2.1: Go media tooling (seams)

**Goal**: Guard and freshness checks exist before any media is produced at scale. (R1, R5, R16)

#### Story 2.1.1: Media-commit guard
**As a** maintainer, **I want** PRs that add binary media to fail, **so that** history stops growing. (R1)
**Acceptance Criteria**:
- Added media outside the allow-list fails.
  - *Given* a PR diff vs `origin/main` adding `website/public/clips/new.webm`, *When* `mediaguard` runs, *Then* it exits 1 listing the path and the allow-list file.
- Allowed, deleted and unrelated files pass.
  - *Given* a PR adding `web-app/public/icon-192.png` (allow-listed), deleting `assets/demo.gif`, and touching `README.md`, *When* `mediaguard` runs, *Then* exit 0.
- Size backstop.
  - *Given* an added `data.json` of 900 KB (ceiling 500 KB), *When* `mediaguard` runs, *Then* it exits 1.
- Runs on media-only PRs.
  - *Given* a PR touching only `website/public/clips/new.webm`, *When* CI runs, *Then* the guard check (Cross-cutting 8b: `pr-diff-guards` with job id `guard`, or `promo-guards`) exists, runs `mediaguard`, and fails (it is not skipped by a path filter; see Cross-cutting 3).
- Revert-friendly override.
  - *Given* a PR re-adding `website/public/demo.webm` and labelled `allow-media`, *When* `mediaguard` runs, *Then* it exits 0 and prints a notice naming the label; without the label it exits 1.
- The override is restricted and visible (pre-mortem P2-5).
  - *Given* the label `allow-media` applied by an actor who is not an individual user in the base `.github/CODEOWNERS`, *When* `mediaguard` reads the **latest** `labeled` event for `allow-media` from the PR events API (Cross-cutting 8d), *Then* the override is ignored and it exits 1 naming the actor; applied by a listed user it exits 0, prints the added byte total, and the use is recorded in the guard job's **job summary** (`$GITHUB_STEP_SUMMARY`; or by the deploy-side `notify` job when the override is reverting the Phase 5 deletion), never as a PR comment: the guard job is read-only (`pull-requests: read`) and cannot post one. Table cases: label removed and re-added by a different actor (the newest event decides); actor `type: Bot` or login `dependabot[bot]`, even if the bot's login is listed in CODEOWNERS (never counts); label removed (no override).
- Adding the label re-runs the check.
  - *Given* a red PR with no label, *When* `allow-media` is added, *Then* the workflow re-triggers (`on.pull_request.types` is `[opened, synchronize, reopened, labeled, unlabeled]`, the concurrency group cancels superseded runs) and the check turns green; `workflow-yaml.test.ts` asserts exactly those `types` and that labels are read from the live event payload of the new run.
- Gate inputs come from the base, not the PR (Cross-cutting 7a).
  - *Given* a PR that flips `guardsEnforce` to `false`, adds `website/public/clips/x.webm` to `tools/promo/mediaguard/allowlist.txt`, and adds its author to `.github/CODEOWNERS`, *When* `mediaguard` runs, *Then* it reads all three from the base blob (`gitbase.ReadBlob`), the added clip still fails, the label override is still refused and the guard stays blocking (table test with a fixture repo whose PR tree differs from its base).
- Absent gate inputs fall back to safe defaults (Cross-cutting 8a).
  - *Given* a base tree with none of `.github/promo-flags.json`, `tools/promo/mediaguard/allowlist.txt` and `.github/CODEOWNERS`, *When* `mediaguard` runs on a PR adding `website/public/clips/x.webm` and labelled `allow-media`, *Then* `guardsEnforce` is `true`, the allow-list is empty (the clip fails; so would an added icon), nobody holds override authority (the label is refused) and it exits 1; and a present but malformed allow-list or flags file exits 2.
- The workflow cannot run PR code with secrets (Cross-cutting 8e).
  - *Given* `backlog-scaffolding-guard.yml`, *When* `workflow-yaml.test.ts` parses it, *Then* `pull_request_target` appears nowhere.
- Override authority and its limits.
  - *Given* a base CODEOWNERS entry that lists only a team (`@org/maintainers`) for `.github/promo-flags.json`, *When* `allow-media` is applied by a member of that team, *Then* the override is refused with the message that teams cannot be resolved with `GITHUB_TOKEN`; and *Given* the entry lists the labeller's user login, *Then* it is honoured.
- Labels do not re-run unrelated checks (G5).
  - *Given* a `labeled` or `unlabeled` event for any label other than `allow-media`, *When* the workflow runs, *Then* the `promoguard` steps and the pre-existing backlog-scaffolding checks are skipped (`if: github.event.action != 'labeled' && github.event.action != 'unlabeled' || github.event.label.name == 'allow-media'`), and `workflow-yaml.test.ts` asserts that expression on every step of the guard job (Cross-cutting 8b); the effect on re-run volume is recorded in `docs/reference/ci-job-budget.md`.
- Wrong base fails loudly.
  - *Given* a repo whose HEAD has one parent and the default `-base HEAD^1` (a `push` event or re-run on a head SHA), *When* `mediaguard` or `docscoverage` runs, *Then* it exits 2 with a message naming the `-base` flag instead of diffing against the wrong commit (`gitbase` test with a single-parent fixture).
**Files**: `tools/promo/cmd/promoguard/main.go`, `tools/promo/cmd/mediaguard/main.go`, `tools/promolib/gitbase/*.go` (created in Task 1.1.4a0), `tools/promo/mediaguard/allowlist.txt`, `make/promo.mk`, `.github/workflows/backlog-scaffolding-guard.yml`, `.github/CODEOWNERS`, `website/tests/unit/workflow-yaml.test.ts`

##### Task 2.1.1a: mediaguard (S)
- Depends on 1.1.4a0 and 1.1.4a1 (`gitbase` and the `promoguard` binary already exist; this task no longer creates them). Fill in the `mediaguard` subcommand: extension-based (`png jpg jpeg gif webp avif webm mp4 mov`); changed files via `gitbase` (adds and modifies; deletions ignored); allow-list globs read from the base blob (Cross-cutting 7a); size ceiling from the blob size; `-allow-media` flag fed from the PR labels (honoured only under the Cross-cutting 8d rule: the latest `labeled` event for `allow-media`, actor type `User` and listed as an individual user in base CODEOWNERS, looked up through the events API; bots and teams are refused; prints the added byte total). Absent inputs fall back to the Cross-cutting 8a defaults, with a test per input. The same `gitbase` package serves `docscoverage` (avoids `dupl` hits); optional `-base` may be `github.event.pull_request.base.sha` after the depth-2 fetch (base.sha can be older than the merge parent if main moved, so `HEAD^1` stays the default). Tests: every Story 2.1.1 AC as a table test over go-git fixture repos. Trace: R1, R11.
- Files: `tools/promo/cmd/mediaguard/main.go`, `tools/promo/cmd/promoguard/main.go`, tests.

##### Task 2.1.1b: Allow-list and wiring (S)
- Depends on 2.1.1c (blocking precondition, Cross-cutting 8c); ships as part of PR B, after the allow-list file itself has merged in PR A (Cross-cutting 8a). Enumerate legit binaries via the index listing (logos, favicons, `web-app/public/*icon*`, `tests/e2e/tests/snapshots/**`, `website/public/favicon*`, `logos/**`, `website/src/assets/**`, `project_plans/**/design/*.png`, `web-app/build/__tests__/fixtures/app/public/icons/**`; verify with `git ls-files` by extension). Wire as a step in the guard job (Cross-cutting 8b; `.github/workflows/backlog-scaffolding-guard.yml`), not `lint.yml`, sharing the checkout/setup-go/prebuilt binary from Task 1.1.4b; add `types: [opened, synchronize, reopened, labeled, unlabeled]` to `on.pull_request` and pass the labels from the event payload; add `make promo-guards` in `make/promo.mk`. Gate the new steps with the label expression from the Story 2.1.1 AC so unrelated `labeled`/`unlabeled` events do not re-run the guard job's pre-existing backlog-scaffolding checks, and record the new re-run behaviour and the count-pr-checks baseline in `docs/reference/ci-job-budget.md`. Record the label override and the guard placement in ADR-003. Trace: R1, R7.
- Files: `tools/promo/mediaguard/allowlist.txt`, `.github/workflows/backlog-scaffolding-guard.yml`, `make/promo.mk`, `docs/reference/ci-job-budget.md`, `website/tests/unit/workflow-yaml.test.ts`.

##### Task 2.1.1c: Base-blob gate inputs, CODEOWNERS file, workflow permissions (S)
- Depends on 1.1.4a1, 2.1.1a and **1.1.4b0** (the permissions block and its test name the guard job id, `guard` or `promo-guards`, which only that measurement decides; 1.1.4b0 does not depend on this task, so the graph has no cycle); it is the blocking precondition for Tasks 1.1.4b and 2.1.1b (Cross-cutting 8c) and ships in PR A (8a). Make `promoguard` read `promo-flags.json` (`guardsEnforce`), both allow-lists and `.github/CODEOWNERS` only through `gitbase.ReadBlob(base, path)` (Cross-cutting 7a) with a test that a PR-modified copy is ignored and tests that each input absent from the base takes its Cross-cutting 8a default. **Create `.github/CODEOWNERS`** (it does not exist today: verified, and there is no root or `docs/` CODEOWNERS either) with entries (user logins, not only teams) for `.github/promo-flags.json`, `tools/promo/**`, `tools/promolib/**`, `tools/docscoverage/**`, `tools/promo/mediaguard/allowlist.txt`, `tools/promo/termcheck/allowlist.txt`, `.github/workflows/backlog-scaffolding-guard.yml`, `.github/workflows/deploy-website.yml` (the theme-file entries stay in Task 2.4.1f; `docs/registry/docs-coverage/` in Story 4.1.1). Declare job-level `permissions: {contents: read, pull-requests: read, issues: read}` on the guard job whose id 1.1.4b0 chose (Cross-cutting 8b; this task adds only the block, the job's steps arrive in 1.1.4b and 2.1.1b) and assert it in `workflow-yaml.test.ts`, together with the no-`pull_request_target` case (8e). Check branch protection with `gh api repos/{owner}/{repo}/branches/main/protection --jq .required_pull_request_reviews.require_code_owner_reviews`; record the result (with the command output) in `spike-notes.md`; if `false` or unreadable it is an owner action and Tasks 1.1.4b and 2.1.1b do not merge until it is `true` (the plan's protection of the guard code depends on it; Cross-cutting 8c). **The owner action also makes the guard job id a required status check on `main`**: `docs/reference/ci-job-budget.md:29` shows `main` as `protected: false` with no required checks and no rulesets, so without it a red guard is advisory and "blocking" is untrue. Record the required-check list (command output) in `spike-notes.md`; 1.1.4b and 2.1.1b do not merge until both the code-owner-review setting and the required check are in place. Note a PR can edit the workflow file itself (including the job's `if:` or steps), so the only control over that file is CODEOWNERS review, not the check. State in ADR-003 the residual risk and that CODEOWNERS teams cannot be resolved with `GITHUB_TOKEN` and the fail-closed fallback. Trace: R1, R15, R17.
- Files: `tools/promo/cmd/promoguard/main.go`, `.github/CODEOWNERS`, `.github/workflows/backlog-scaffolding-guard.yml`, `website/tests/unit/workflow-yaml.test.ts`, spike-notes.

#### Story 2.1.2: Media manifest, expected list and freshness check
**As a** maintainer, **I want** a single expected-media list and a record-time manifest, **so that** a missing or stale asset fails the build. (R5, R16)
**Acceptance Criteria**:
- Missing expected media fails.
  - *Given* `ExpectedMedia` containing `approvals-queue` and a merged `MediaManifest` without it, *When* `mediafresh` runs, *Then* it exits 1 printing `approvals-queue: missing`.
- Commit and theme stamps enforced.
  - *Given* a `MediaEntry` with `commit=abc123` and `GITHUB_SHA=def456`, *When* `mediafresh` runs, *Then* it exits 1; and a `themeId=clean` entry also fails.
- Summary table.
  - *Given* 3 entries (recorded 2, failed 1), *When* `mediafresh -summary` runs, *Then* it appends a Markdown table with status, bytes and sha256 to `$GITHUB_STEP_SUMMARY`.
- Flags are reviewable.
  - *Given* `.github/promo-flags.json`, *When* the unit test runs, *Then* `freshnessEnforce`, `docsCoverageEnforce` and `guardsEnforce` exist as booleans, the first two `false` and `guardsEnforce` `true` at introduction, and `mediatool fresh` reads `freshnessEnforce` (report-only prints and exits 0 when `false`).
- Budgets come from the single `budgets.json` (R7).
  - *Given* `tools/promo/budgets.json` holding `clipMaxBytes` (3 MB), `imageMaxBytes` (400 KB), `distMaxBytes` (40 MB total `dist/`), `recordWallClockMaxMinutes` (30) and `clipMaxDurationMs` (60000), *When* `mediatool fresh` runs on fixtures that exceed each in turn, *Then* it exits 1 naming the budget (`TestFresh_PerAssetBudgets`, `TestFresh_DistCeiling`, `TestFresh_RecordWallClockCeiling` read from the manifest start/end stamps, `TestFresh_ClipOver60sFails`); each value lives only in `budgets.json`.
- Reuse mode keeps freshness meaningful for content-only deploys (G1, Cross-cutting 7b).
  - *Given* a manifest recorded at commit `C`, `GITHUB_SHA=H`, and `-reuse-from C`, *When* the compare-API file list for `C...H` contains only `website/src/content/docs/help/x.md`, *Then* `mediatool fresh` exits 0 and prints `reused media from C`; *When* the list contains any path under `recordInputPaths` (`web-app/**`, `server/**`, `session/**`, `proto/**`, `config/**`, `app/**`, `go.mod`, `go.sum`, `tests/e2e/website-demos/**`, `tests/demo/**`, `docs/features.md`, `website/src/content/docs/tours/**`, `website/src/content/feature-media.json`, `tools/promo/**`, `tools/promolib/**`), *Then* it exits 1 naming the first path; *When* the compare status is `diverged` or `behind` (C not an ancestor), or the file list is truncated, *Then* it exits 1; *When* `-reuse-from` is absent, *Then* `commit == GITHUB_SHA` is still required. Theme and expected-name presence checks apply in every mode. Table tests in `fresh_test.go`.
**Files**: `tools/promo/cmd/mediatool/{main.go,fresh.go,shardplan.go,summary.go}`, `tools/promo/internal/manifest/*.go`, `tools/promo/schema/media-manifest.schema.json`, `website/scripts/expected-media.mjs`, `tools/promo/budgets.json`, `.github/promo-flags.json` (created in Task 1.1.4a1; read here)

The three responsibilities share one binary `tools/promo/cmd/mediatool` with subcommands `fresh`, `shard-plan`, `summary` (one name used everywhere; there is no separate `mediafresh` binary, and any plan text saying `mediafresh` means `mediatool fresh`).

##### Task 2.1.2a: Manifest types and `mediatool fresh` (M)
- Go `MediaManifest`/`MediaEntry` (+JSON schema), merge of shard manifests, checks: presence, commit, theme, `dist` file existence for referenced `src/poster/source`, per-asset budgets (clip <= 3 MB, image <= 400 KB), a total `dist/` ceiling (40 MB), a record-job wall-clock ceiling (30 min, read from the manifest's start/end stamps) and a clip duration cap (60 s, Cross-cutting 8h) from the single `budgets.json`; the summary prints the dist-size trend against the previous run's value (P2-1, P2-2). Manifest rows also carry `attempts` (Task 2.3.1f), `seeded` (`db|rpc|route`) and a `format` (`webm|mp4|webp|png`, one row per rendition, Cross-cutting 7e). Reads `.github/promo-flags.json` (created in Task 1.1.4a1, not here). The 30-minute record ceiling is checked against a measured run that includes the cold cache and the `apt-get install tesseract-ocr` step (see Task 2.4.1a), not only the Playwright time. `budgets.json` also holds `recordInputPaths` (Cross-cutting 7b). Trace: R5, R7, R16.
- Files: `tools/promo/cmd/mediatool/{main.go,fresh.go}`, `tools/promo/internal/manifest/manifest.go`, tests, `tools/promo/budgets.json`.

##### Task 2.1.2b: Summary renderer (S)
- Markdown table writer to `$GITHUB_STEP_SUMMARY` (recorded/failed/skipped, bytes, sha256, form factor), exposed as `mediatool summary`. Trace: R16.
- Files: `tools/promo/cmd/mediatool/summary.go`, `tools/promo/internal/manifest/summary.go`, tests.

##### Task 2.1.2c: `expected-media.mjs` (S)
- Generate `website/expected-media.json` from `tours/*.mdx` frontmatter and `website/src/content/feature-media.json` (gitignore the output); unit tests run against **fixture inputs** (`website/tests/unit/fixtures/expected-media/{tours/*.mdx,feature-media.json}`) because the real `tours/*.mdx` and `feature-media.json` do not exist until Stories 3.1.2 and 3.2.x; the test over the real tree is Task 3.2.5b. Stays `.mjs` (not Go) because it reads Astro content frontmatter at build time with the site's own Zod/YAML tooling; recorded in ADR-002. Only media with a consumer is listed (Story 3.2.5 equality test). Add `.gitignore` entries for every recorded or generated output so a local run cannot stage binaries: `website/expected-media.json`, `website/public/clips/`, `website/public/shots/`, `website/public/media-manifest.json` (and wherever the recorder writes it), `dom-text/` and `ocr-text/`; a test (`website/tests/unit/gitignore-media.test.ts`) runs `git check-ignore` over a sample path for each. Trace: R5, R13, R1.
- Files: `website/scripts/expected-media.mjs`, `website/tests/unit/expected-media.test.ts`, `website/tests/unit/fixtures/expected-media/*`, `website/tests/unit/gitignore-media.test.ts`, `.gitignore`.

##### Task 2.1.2d: `mediatool fresh` reuse mode (S)
- Implement `-reuse-from <commit>` per the Story 2.1.2 reuse AC and Cross-cutting 7b: inputs are the recorded commit (from the manifest), `GITHUB_SHA`, and a file list plus status from `gh api repos/{owner}/{repo}/compare/<recorded>...<sha> --paginate` (the workflow passes them as `-compare-json`, so the Go code needs no network and tests use fixtures); `recordInputPaths` come from `budgets.json`, the same list the workflow's decision step uses (one list, no copy in YAML: the step calls `mediatool fresh -decide-reuse` which prints `record` or `reuse`). Tests: ancestor and no input path (pass), ancestor with `web-app/` change (fail), `diverged` (fail), truncated list (fail), no flag (strict equality), tours `.mdx` change (fail: it changes expected media). Trace: R5, R7.
- Files: `tools/promo/cmd/mediatool/fresh.go`, `tools/promo/cmd/mediatool/fresh_test.go`, `tools/promo/budgets.json`.

## Epic 2.2: Scenario-based Go seeding

**Goal**: Realistic, deterministic, public-safe seed data for every tour. (R6, R14, R20)

#### Story 2.2.1: Scenario framework
**As a** recorder author, **I want** named seed scenarios, **so that** each tour gets exactly the data it needs. (R6, R20)
**Acceptance Criteria**:
- `default` is unchanged.
  - *Given* `go run ./tests/demo/seed --scenario default <dir> 6`, *When* seeding completes, *Then* the six `mockSessions()` records exist byte-for-byte as today.
- Scenarios compose.
  - *Given* `--scenario dashboard,approvals`, *When* seeding runs, *Then* both scenarios' data exist and a duplicate id returns an error naming both scenarios.
**Files**: `tests/demo/scenario/scenario.go`, `tests/demo/seed/main.go`, `tests/demo/helpers.go`

##### Task 2.2.1a: Scenario interface and CLI (S)
- Two small interfaces: `FileSeeder{Name(); SeedFiles(dir string, ref time.Time) error}` (pre-boot; what the CLI `go run ./tests/demo/seed --scenario ... <dir>` calls, no server needed) and `RPCSeeder{Name(); SeedRPC(ctx, client, ref time.Time) error}` (post-boot). A scenario implements either or both; the registry holds both. A new CLI subcommand `go run ./tests/demo/seed rpc --scenario ... --url <server>` runs the RPC step, and `record-website-demos.sh` calls it after server readiness (which items need which is fixed by 0.1.1d). `ref` is the single reference instant: seeded timestamps are `ref` minus fixed offsets, and the recorder passes the same `ref` to `page.clock.setFixedTime` (Task 2.3.1d). Keep legacy positional args working. Trace: R6, R20.
- Files: `tests/demo/scenario/scenario.go`, `tests/demo/seed/main.go`, `tests/demo/scenario/scenario_test.go`, `scripts/record-website-demos.sh`.

##### Task 2.2.1b: Fixture repo, synthetic history, canned terminal (M)
- Dedicated fixture git repo under the temp dir (fake names), synthetic `~/.claude`-style history dir via the seam found in 0.1.1d, canned scrollback fixtures; no runner paths visible. Trace: R14, R6.
- Files: `tests/demo/scenario/fixtures/*.go`, `tests/demo/scenario/testdata/*`.

#### Story 2.2.2: Scenarios for all tours
**As a** tour author, **I want** fixtures for dashboard, approvals, backlog, insights, notifications, automation, fan-out, files and history, **so that** every tour looks like a believable team. (R2, R14)
**Acceptance Criteria**:
- Seed data is public-safe.
  - *Given* all scenarios seeded to a temp dir, *When* the leak test runs `termcheck -patterns=leak` over every string field, *Then* it reports zero hits for `/home/`, `/tmp/tmp.`, `ghp_`, `.internal`, emails.
- Timestamps are fixed offsets from the shared reference instant.
  - *Given* scenario `dashboard` and reference instant `ref`, *When* seeded with two different `ref` values, *Then* every timestamp is the same offset from `ref` (e.g. `-3m`, `-47m`); and the recorder's browser clock is frozen at that same `ref` (Task 2.3.1d), so the rendered label is the same string on every run.
- Default-scenario paths accepted as fake.
  - *Given* `mockSessions()` paths such as `/Users/dev/services/...`, *When* the leak test runs, *Then* `/Users/` is in `LeakPatterns` and the default scenario's paths are either rewritten to `/demo/...` for recorded runs or listed in an explicit allow-list of obviously fake paths in `leak_test.go`.
**Files**: `tests/demo/scenario/{dashboard,approvals,backlog,insights,notifications,automation,fanout,files,history}.go`

##### Task 2.2.2a: dashboard + approvals scenarios (M)
- Sessions in Active, Paused, Needs-Approval-looking states within the poller constraint (0.1.1d), tags, approval rules, queue items. Trace: R2, R14.
- Files: `tests/demo/scenario/dashboard.go`, `approvals.go`, tests.

##### Task 2.2.2b: backlog + insights scenarios (M)
- Backlog items across stages with flag prerequisite, spend and findings data. Trace: R2, R14.
- Files: `tests/demo/scenario/backlog.go`, `insights.go`, tests.

##### Task 2.2.2c: notifications, automation, fan-out, files, history scenarios (M)
- Notification tray, triggers/workflows/tagging rules, fan-out variants, files-viewer fixture repo wiring, history entries. Trace: R2, R14.
- Files: `tests/demo/scenario/notifications.go`, `automation.go`, `fanout.go`, `files.go`, `history.go`.

##### Task 2.2.2d: Seed leak test (S)
- Go test seeding every scenario and scanning serialized data with `termscan.LeakPatterns()` (function, not a package var: `gochecknoglobals`) imported from `tools/promolib/termscan` (non-internal, so the import compiles from `tests/demo/scenario`); fails on hits. Trace: R14.
- Files: `tests/demo/scenario/leak_test.go`.

## Epic 2.3: Recorder

**Goal**: Deterministic, verified capture of clips, shots, mobile shots and transcripts. (R9, R10, R11, R19, R20)

#### Story 2.3.1: Recorder core
**As a** tour author, **I want** a recorder that asserts Grimdark, freezes time, captures shots and writes the manifest, **so that** output is reproducible and verifiable. (R9, R10, R11, R14, R19, R20)
**Acceptance Criteria**:
- No `waitForTimeout` remains.
  - *Given* `tests/e2e/website-demos/**` (9 `waitForTimeout` occurrences at planning time), *When* `grep -r waitForTimeout` runs, *Then* zero matches; pacing uses `hold(page, ms)` and state changes use locator waits.
- Wrong theme fails the clip.
  - *Given* a binary without the alias and `localStorage['stapler-theme']='grimdark'`, *When* `recordClip` reaches `ready()`, *Then* it throws `expected theme class grimdark, got clean` and no media entry is written as recorded.
- Flag-gated page guards.
  - *Given* the `backlog` flag disabled, *When* the `backlog-board` Scene navigates to `/backlog/board`, *Then* the final-URL/landmark assertion fails the clip rather than saving the redirect target.
- Transcript emitted.
  - *Given* a Scene calling `caption(page, 'Approve the push')` at 4.2 s, *When* the clip finishes, *Then* `<name>.transcript.json` contains `{tMs:4200, text:'Approve the push'}`.
- A flaky scene is retried and recorded, a wrong theme is not.
  - *Given* a Scene that fails its first attempt on a locator timeout and passes the second, *When* it records, *Then* the manifest row has `attempts: 2`; and *Given* `assertGrimdark` fails, *Then* there is no retry and the shard fails regardless of `freshnessEnforce`.
- Recording is hermetic.
  - *Given* the recorder script and Playwright config, *When* `hermetic_test.go` runs, *Then* it passes (no `8543`, free `PORT`, `ssq-*` socket, `--test-mode --test-dir`).
- Mobile shots.
  - *Given* shot spec `dashboard-mobile` with `FormFactor=mobile`, *When* captured, *Then* the PNG/WebP is 390x844 (x DPR) and its `MediaEntry.formFactor` is `mobile`.
**Files**: `tests/e2e/website-demos/{recorder.ts,retry.ts,clips.ts,shots.ts,assertions.ts,hold.ts}`, `tools/promo/cmd/mediatool/hermetic_test.go`, `tests/e2e/playwright.website-demos.config.ts`, `tests/e2e/website-demos.spec.ts`

##### Task 2.3.1a: `hold()` helper, scene index, waitForTimeout removal (M)
- Scenes are **discovered by glob** over `tests/e2e/website-demos/tours/*.ts` (each exports a `Scene`), so `clips.ts` holds only the legacy non-tour clips and no per-tour index entry exists; this keeps adding a tour at one `.ts` plus one `.mdx` (Cross-cutting 6f). Replace every `waitForTimeout` with `hold` or locator waits; add `// @feature` header; ensure e2e convention lint passes. Trace: R11, R20.
- Files: `clips.ts`, `hold.ts`, `recorder.ts`, `website-demos.spec.ts`.

##### Task 2.3.1b: Shot helper, manifest emit, form factors (M)
- `captureShot()`: `reducedMotion: 'reduce'`, `animations: 'disabled'`, `document.fonts.ready` + `fonts.check`, masks for version/SHA; desktop and mobile projects (mobile shots only); emit `media-manifest.json` rows; annotation badge overlay via theme tokens. Trace: R9, R10, R5.
- Files: `shots.ts`, `recorder.ts`, `playwright.website-demos.config.ts`.

##### Task 2.3.1c: Capture-time assertions and DOM dump (M)
- `assertGrimdark()` using the exported `ThemeID` constant plus computed body background; final URL and landmark; no `[role=dialog]`; dump `body.innerText` to `<out>/dom-text/<name>.txt` for `termcheck -mode=media-text` and leak scan. Trace: R9, R14, R15.
- Files: `assertions.ts`, `recorder.ts`.

##### Task 2.3.1d: Clock freeze and flag enablement (S)
- `page.clock.setFixedTime(ref)` (not `install`, per `research/pitfalls.md` s2; Playwright 1.56.1 supports it) before navigation, where `ref` is the same reference instant passed to the seeder (Task 2.2.1a; harness exports it as `DEMO_REF_TIME`). AC: a captured shot's DOM dump shows a relative-time string such as "3m ago" for a session seeded at `ref - 3m`. Server-computed windows (insights spend, analytics rollups, poller timers) keep real time and are not frozen; seeds for those scenarios use absolute day offsets and the Scene asserts non-empty data, not exact values. Enable `backlog`/`changelog` via the mechanism found in 0.1.1c. Trace: R6, R9.
- Files: `recorder.ts`.

##### Task 2.3.1e: Transcript emission (S)
- `caption()` records `{tMs,text}`; write `<name>.transcript.json` beside the clip; manifest links it. An empty or missing transcript for a clip fails the shard. Trace: R19.
- Files: `recorder.ts`, `tours/*.ts`.

##### Task 2.3.1f: Scene-level retry (S)
- Wrap each Scene in a bounded retry (2 attempts, fresh page and context, server-state check between attempts); write `attempts` into the manifest row; a Scene that needs the second attempt is listed in the job summary as `flaky`. An `assertGrimdark` or leak failure is **not** retried. Scenes assert on the strings their tour steps quote: each step's `quote` (when present in frontmatter) must appear in the DOM dump, otherwise the Scene fails (P2-3). Route-fixture fallbacks (0.1.1f) are validated against the generated proto message types in a test, at most 2 per tour, flagged `seeded: route` (P2-4). Trace: R5, R6, R16.
- Files: `tests/e2e/website-demos/recorder.ts`, `tests/e2e/website-demos/retry.ts`, tests.

##### Task 2.3.1g: Hermeticity check (S)
- A Go test `tools/promo/cmd/mediatool/hermetic_test.go` (or a script lint test in `website/tests/unit/`) parses `scripts/record-website-demos.sh` and `tests/e2e/playwright.website-demos.config.ts` and asserts: `PORT` is a dynamically allocated free port, `STAPLER_SQUAD_TMUX_SOCKET` is set to an `ssq-*` name, `--test-mode --test-dir` are passed, and neither file contains `8543` or `localhost:8543`; and (BUG-116 residual: a second instance's startup cleanup can delete the shared `~/.stapler-squad/cdp-bins/<id>` wrapper dirs of a live instance) that `record-website-demos.sh` sets `HOME` to a per-run temp directory before starting the server, or, if the spike shows the server resolves its state elsewhere, that the script header says "CI only until BUG-116 is fixed" and refuses to run when `CI` is unset and a process listens on `:8543` (the test asserts whichever mechanism is chosen, and the script header links `docs/bugs/open/BUG-116*`). Integration note in the PR: during one shard run, `ss -ltnp` shows no connection to `:8543` and the tmux socket is `ssq-*`. Trace: R6.
- Files: `tools/promo/cmd/mediatool/hermetic_test.go`, `scripts/record-website-demos.sh` (read).

## Epic 2.4: Workflow

**Goal**: Sharded recording, fail-loud freshness, PR smoke run, legacy publishing retired. (R5, R7, R8, R16, R17, R18)

#### Story 2.4.1: Restructure `deploy-website.yml` and retire legacy publishing
**As a** maintainer, **I want** a sharded record pipeline with fail-loud freshness, **so that** the site only ships current media. (R5, R7, R16, R17, R18)
**Acceptance Criteria**:
- Shards merge cleanly.
  - *Given* 4 `RecordShard`s uploading `media-shard-0..3` artifacts, *When* the build job downloads with `pattern: media-shard-*` and `merge-multiple: true`, *Then* `website/public/clips` and `shots` contain every expected file and one merged `media-manifest.json`.
- Missing clip fails the deploy (once enforcing).
  - *Given* shard 2 fails to record `approvals-queue` and `freshnessEnforce` is `true` in `.github/promo-flags.json`, *When* the build job runs `mediatool fresh`, *Then* the job fails and `deploy` is skipped; the notify job opens the tracking issue; the previous Pages deployment stays live. With the flag `false`, `mediatool fresh` reports the same finding, exits 0, and the old fallback step still runs.
- Content-only pushes do not re-record (pre-mortem P1-3 step 3, P2-2).
  - *Given* a push to main where `mediatool fresh -decide-reuse` (Task 2.1.2d) finds the recorded commit of the LKG artifact is an ancestor of `GITHUB_SHA` and no path in `recordInputPaths` changed (the list in Cross-cutting 7b, which includes `tours/**` and `feature-media.json`, so editing a tour's alt text or steps re-records), *When* `deploy-website.yml` runs, *Then* the record job is skipped, the build downloads `promo-media-lkg`, and `mediatool fresh -reuse-from <recordedCommit>` (not strict `commit == GITHUB_SHA`) passes; the weekly cron, `workflow_dispatch` and any push touching a recording-input path always re-record in full mode. A reuse run uploads no new LKG artifact and is not a "full run" (Task 5.1.2a).
- LKG is a named, retained, looked-up artifact (G4).
  - *Given* a successful full run on main, *When* it finishes, *Then* it has uploaded artifact `promo-media-lkg` with `retention-days: 35`; *Given* a reuse or LKG deploy, *When* it looks up the artifact, *Then* it uses the newest successful `deploy-website.yml` run on main (`gh run list`, `gh run download -n promo-media-lkg`) with job permission `actions: read`; *Given* the artifact is missing or expired, *Then* the job hard-fails with a message naming the artifact (never a silent skip), asserted by `workflow-yaml.test.ts` (retention, name, `actions: read`, no `continue-on-error` on the download step) and a fixture test of the lookup script.
- Last-known-good deploy is bounded (pre-mortem P1-3 step 4).
  - *Given* a scheduled run where `mediatool fresh` fails (enforcing or not), *When* the last-known-good media artifact is younger than 28 days, *Then* the deploy publishes it with a visible banner line in the job summary and the notify job opens the tracking issue; *When* it is 28 days or older, *Then* the run hard-fails. Last-known-good applies only to `schedule`, never to `push` or PR runs.
- Per-clip retry and flakiness are visible.
  - *Given* a Scene that needed 2 attempts, *When* the summary renders, *Then* it appears as `flaky` with attempts; the soak criterion for Story 5.1.2 reads these rows.
- Media-text and OCR block the upload (pre-mortem P1-5).
  - *Given* a DOM dump or a shot whose OCR text contains a banned fragment, *When* the record job runs `termcheck -mode=media-text` and the OCR step, *Then* the job fails before the Pages artifact upload and independent of `freshnessEnforce`.
- Notify covers every failure source (validation R16).
  - *Given* `website/tests/unit/workflow-yaml.test.ts`, *When* it parses `deploy-website.yml`, *Then* the `notify` job `needs` includes `record`, `mediafresh` (or the job holding `mediatool fresh`) and `termcheck-dist`, and a flag-age step exists; an induced shard failure on a test branch shows a `failed` row and opens the tracking issue.
- Flag-age reminder.
  - *Given* a flag still `false` 14 days after its prerequisite story merged, *When* `notify` runs on the weekly cron, *Then* it comments on the flag's tracking issue.
- PR smoke run is bounded.
  - *Given* a PR touching `tests/e2e/website-demos/**`, `tests/demo/**` or any `recordInputPaths` UI path together with `website/**`, *When* `website-ci.yml` runs, *Then* the smoke step does not run against the main-built binary (scenes and seeds that need UI not yet on main would fail for reasons unrelated to the PR): it either builds the binary from the PR (cached, only for those paths) or is skipped with a visible job-summary notice naming the paths that triggered the skip (G6); the trigger path list is asserted in `workflow-yaml.test.ts`, and the smoke trigger paths are `website/**`, `tests/e2e/website-demos/**` and `tests/demo/**`.
  - *Given* a PR touching `website/**`, *When* `website-ci.yml` runs, *Then* it records 1 clip + 1 shot set (the `dashboard-lifecycle` clip, which is also the hero clip, and its shots) against a binary downloaded as an artifact from the latest successful `build.yml` run on main (no proto-gen/web build/go build in website-ci), and does not run the full matrix. Targets (capped by `timeout-minutes`, which website-ci sets to 30): smoke record <= 6 min, whole job <= 20 min; Task 0.1.1b records the measured baseline and the cap is set to that plus 50%. If no artifact is available the smoke step is skipped with a visible notice, not failed.
- PR Lighthouse does not penalize missing media.
  - *Given* a PR build in `TOURS_MEDIA_MODE=smoke` where only the smoke subset of tour media exists, *When* PR Lighthouse and axe run, *Then* Lighthouse covers non-tour URLs plus the `/tours/` index (which lists only the recorded tour, Cross-cutting 6c), and **axe additionally covers one smoke tour page, `/tours/dashboard-lifecycle/`** (the one recorded tour, so tour-page markup regressions fail on the PR); tour-page Lighthouse and axe over every tour run in `deploy-website.yml` (full media) only. The PR link check needs no allow-list because unrecorded tours are not built and absent step media is omitted from the HTML, not linked.
- Concurrency/budget measured.
  - *Given* `tools/ci/count-pr-checks.py` run before and after the change (per `docs/reference/ci-job-budget.md` checklist item 7), *When* compared, *Then* the PR check count is unchanged or the delta is recorded in the PR; the matrix is 2 shards on `push` to main unless 0.1.1b shows headroom for 4.
- Existing website suites stay green.
  - *Given* the edits, *When* `bun test`/vitest unit suites and `website/tests/e2e/landing.spec.ts` run, *Then* they pass with updated expectations (files below).
- Retirement.
  - *Given* the merged change, *When* `.github/workflows/demo-publish.yml` is looked up, *Then* it no longer exists and no workflow references `demo-gifs-latest`.
**Files**: `.github/workflows/deploy-website.yml`, `.github/workflows/website-ci.yml`, `.github/workflows/demo-publish.yml` (delete), `scripts/record-website-demos.sh`, `website/scripts/check-asset-health.mjs`, `website/scripts/asset-health-manifest.json`, `website/src/lib/release-asset.ts`, `website/tests/unit/workflow-yaml.test.ts`, `website/tests/unit/check-asset-health.test.ts`, `website/tests/integration/check-asset-health.integration.test.ts`, `website/tests/unit/release-asset.test.ts`, `docs/reference/ci-job-budget.md`, `.github/CODEOWNERS`, `tools/promo/cmd/mediatool/fresh.go`

##### Task 2.4.1a: Build-once binary, shard matrix, `--only/--smoke` (M)
- Binary built once and passed as artifact; matrix from `mediatool shard-plan` (round-robin by `TourID`; 2 shards unless 0.1.1b allows 4); `record-website-demos.sh --only <names> --smoke`; `workers: 1` per server; per-shard artifact upload; drop `lfs: true` from the record checkout (masks the guard); before/after `count-pr-checks.py` recorded in the PR and in `docs/reference/ci-job-budget.md`. The 30-minute record ceiling is measured on a **cold-cache** shard that includes the `apt-get install` of ffmpeg and `tesseract-ocr` and the Chromium/dependency cache restore (record these seconds separately in the job summary); the ceiling is set from that measurement, not from Playwright time alone. Trace: R7, R6.
- Files: `.github/workflows/deploy-website.yml`, `scripts/record-website-demos.sh`, `tools/promo/cmd/mediatool/shardplan.go`, `docs/reference/ci-job-budget.md`.

##### Task 2.4.1b: Report-only freshness beside the old fallback, summary, notify, dist checks (M)
- **Keep** the existing warn-and-fallback step (`deploy-website.yml:134-152`), mark it `# REMOVE IN 5.1.2`, and add `mediatool fresh` as a separate step beside it that obeys `freshnessEnforce` (report-only while `false`, fail-loud when `true`). Task 5.1.2a is the only place the old step is removed. Add `termcheck -mode=dist` and `-mode=media-text` after `bun run build`; extend `notify` to the new failure sources and the flag-age reminder; dist budget check. Trace: R5, R15, R16.
- Files: `.github/workflows/deploy-website.yml`, `website/tests/unit/workflow-yaml.test.ts` (step order, `notify` `needs`, flag-age step, `REMOVE IN 5.1.2` marker), `.github/promo-flags.json` (read by the flag-age step), `tools/promo/budgets.json` (read for the dist budget step), `tools/promo/cmd/mediatool/fresh.go` (invoked; not edited here).

##### Task 2.4.1c: PR smoke in website-ci (S)
- Smoke steps in the existing `checks` job that build the server binary from the PR themselves (Phase 6 correction: `e2e-binary-bun` uploads its binary with `retention-days: 1`, so a main-built artifact is not reliably downloadable, and `website-ci` does not download one), setting `TOURS_MEDIA_MODE=smoke` for the site build (the only place it is set); exclude tour pages from PR Lighthouse but run axe on `/tours/dashboard-lifecycle/` (Story 2.4.1 AC); no produced-later allow-list (Cross-cutting 6c); keep `workflow-yaml.test.ts` assertion that website-ci runs the same checks as deploy, updated to name the intentional differences. Trace: R7, R8, R12.
- Files: `.github/workflows/website-ci.yml`, `website/scripts/check-internal-links.mjs`, `website/tests/unit/workflow-yaml.test.ts`.

##### Task 2.4.1d: Retire demo-publish and legacy manifest (S)
- Delete `demo-publish.yml`; replace `check-asset-health.mjs` release-URL logic with the local-media existence check (already in `mediatool fresh`); delete `asset-health-manifest.json` and its deploy step; `website/src/lib/release-asset.ts` keeps only `findHtmlFiles` (still imported by `check-internal-links.mjs`) and its release-URL exports and tests are removed; update `check-asset-health.test.ts`, the integration test and `release-asset.test.ts` accordingly, and the `workflow-yaml.test.ts` step-order assertions (asset-health before Lighthouse, "without committing anything"). Grep for `demo-gifs-latest` references. Trace: R18, R20.
- Files: `.github/workflows/demo-publish.yml`, `website/scripts/check-asset-health.mjs`, `website/scripts/asset-health-manifest.json`, `website/src/lib/release-asset.ts`, `website/tests/unit/{check-asset-health,release-asset,workflow-yaml}.test.ts`, `website/tests/integration/check-asset-health.integration.test.ts`, `deploy-website.yml`.

##### Task 2.4.1e: Content-only reuse and bounded last-known-good (M)
- **Depends on 2.1.2d and 2.4.1g** (the `promo-media-lkg` artifact and its lookup are defined in 2.4.1g). Add a decision step to `deploy-website.yml` that runs `mediatool fresh -decide-reuse` (Task 2.1.2d; `record` vs `reuse` from the compare API and `recordInputPaths`); on `reuse` skip the record job and build from the LKG artifact (Task 2.4.1g) checked in reuse mode (Cross-cutting 7b); on scheduled runs where `mediatool fresh` fails in full mode, deploy the previous LKG artifact if younger than 28 days (age from its `media-manifest.json` timestamp) with a summary banner, else fail; `mediatool fresh` gains `-max-age-days 28` and `-allow-last-known-good` (only on `schedule`). Unit tests for the age rule, the reuse decision and the event gating (`push`/PR never use last-known-good). Storage and lookup of the artifact are Task 2.4.1g. Trace: R5, R7, R17.
- Files: `.github/workflows/deploy-website.yml`, `tools/promo/cmd/mediatool/fresh.go`, `tools/promo/cmd/mediatool/fresh_test.go`, `website/tests/unit/workflow-yaml.test.ts`.

##### Task 2.4.1g: Last-known-good artifact: upload, lookup, hard-fail on missing (S)
- Upload step (full-mode success on main only): `actions/upload-artifact` name `promo-media-lkg`, `retention-days: 35`, containing the merged `website/public/{clips,shots}` and `media-manifest.json`. Lookup script (bash in the workflow or a `mediatool lkg` subcommand, chosen by the implementer; Go preferred per repo rule): newest successful `deploy-website.yml` run on `main` through `gh run list --workflow deploy-website.yml --branch main --status success --json databaseId`, then `gh run download <id> -n promo-media-lkg`; job `permissions: actions: read, contents: read`. Missing, expired or empty artifact exits non-zero with a message naming it. Tests: `workflow-yaml.test.ts` asserts name, retention (>= 35), permission and no `continue-on-error`; a fixture test runs the lookup with a stubbed `gh` returning no runs and asserts a hard failure. `actions/cache` is not used for LKG (7-day eviction). Trace: R5, R17.
- Files: `.github/workflows/deploy-website.yml`, `tools/promo/cmd/mediatool/lkg.go`, `tools/promo/cmd/mediatool/lkg_test.go`, `website/tests/unit/workflow-yaml.test.ts`.

##### Task 2.4.1f: Media-text blocking, OCR step, theme-file sign-off (S)
- **Depends on 2.4.1h** (OCR fixture and confidence rule) **and 1.1.4c** (media-text mode); the blocking OCR step is not wired before the fixture exists. In the record job, after capture and before artifact upload: run `termcheck -mode=media-text` over `dom-text/` and an OCR step (`tesseract` installed with `apt-get` in the job, run over every shot, output concatenated to `ocr-text/<name>.txt`, scanned by `termcheck -mode=media-text`); both fail the job unconditionally (the OCR tuning, known-text fixture and confidence rule are Task 2.4.1h). Add `.github/CODEOWNERS` entries for `web-app/src/styles/theme*`, `website/src/styles/grimdark.css`, fonts and `web-app/src/lib/theme/**` so a theme change needs the owner's review (the recorded sign-off for imagery, logos and decorative art that OCR cannot read); weekly `notify` adds an owner-review reminder line. Trace: R15, R14, R16.
- Files: `.github/workflows/deploy-website.yml`, `.github/CODEOWNERS`, `tools/promolib/termscan/dist.go` (media-text mode already in 1.1.4c).

##### Task 2.4.1h: OCR known-text fixture and confidence rule (S)
- Stylised Grimdark fonts will give OCR false negatives and false positives, and a flaky blocking gate would block the weekly deploy even with `freshnessEnforce` off. Add: a fixture test (`tools/promolib/termscan/ocr_test.go` with `testdata/ocr/known-text.png`, a shot of known Grimdark-font text rendered in CI by the recorder's own pipeline or committed under the mediaguard allow-list `tools/promolib/termscan/testdata/**`) asserting the OCR step recovers a fixed word list at or above a threshold (so a font/Tesseract change that breaks recognition fails the *fixture*, in PR CI, not the deploy); a confidence rule in `termscan` media-text mode that only counts a banned-fragment hit when Tesseract word confidence is >= the configured value (default 60, in `tools/promo/budgets.json`) and the fragment matches a whole OCR token after normalising `0/O`, `1/l`, `5/S`; a `tools/promo/ocr-allowlist.txt` (`shot-name:reason`, stale entries fail) for confirmed false positives. Low-confidence hits are reported as warnings in the job summary and never block; DOM-text hits always block. Trace: R15, R14, R7.
- Files: `tools/promolib/termscan/{media.go,ocr_test.go}`, `tools/promolib/termscan/testdata/ocr/*`, `tools/promo/ocr-allowlist.txt`, `tools/promo/budgets.json`.

---

# Phase 3: Tours section

## Epic 3.1: Site foundation

**Goal**: Typed, accessible, performant tour pages. (R3, R8, R12, R13, R19)

#### Story 3.1.1: Tour schema, components, index
**As a** visitor, **I want** a tours index and tour pages with clip, steps and transcript, **so that** I can evaluate the product quickly. (R3, R12, R19)
**Acceptance Criteria**:
- Missing alt fails the build.
  - *Given* `tours/x.mdx` with a `steps[1].shot` lacking `alt`, *When* `bun run build` runs, *Then* it fails with a Zod error naming the field.
- Clip honors reduced motion and is lazy.
  - *Given* `prefers-reduced-motion: reduce`, *When* `/tours/dashboard-lifecycle/` loads, *Then* the `<video>` has `preload="none"`, a poster, `controls`, and no autoplay.
- Transcript present.
  - *Given* tour `approval-review` with clip `approval-review`, *When* the page renders, *Then* a `<details><summary>Transcript</summary>` lists each step's `narration` and the caption timeline.
- Build stamp (one format, Cross-cutting 8g).
  - *Given* `media-manifest.json` commit `a1b2c3d` recorded on 2026-11-10, *When* a tour page renders, *Then* the stamp reads "Recorded 2026-11-10 (build a1b2c3d)" with the sha inside `<small>`; "Recorded from build" appears nowhere.
- Length caps are enforced (Cross-cutting 8h, G8).
  - *Given* `tour-schema.test.ts` with a tour of 2 steps, 7 steps, and 3-6 steps, *When* the Zod schema parses each, *Then* only 3-6 passes; and *Given* a manifest clip row with `durationMs` 61000, *When* `mediatool fresh` runs, *Then* it fails naming `clipMaxDurationMs` (Task 2.1.2a).
- Tours index and tour page structure (UX-1, UX-2).
  - *Given* the built site, *When* `website/tests/e2e/tours.spec.ts` loads `/tours/`, *Then* it finds one h1, a 3-card "Start here" strip (tours with `tour.order` 1-3: `dashboard-lifecycle`, `approval-review`, `organise-at-scale`), one h2 per category and every card linking to a tour; and on a tour page the DOM order is breadcrumb, h1, meta row, video, **Install link (after the clip)**, `details` transcript, `ol` steps, "Try it" block (Install link, help link), prev/next, "All tours".
- Media blocked or failing at runtime (UX-3, G7).
  - *Given* `page.route` aborting `*.webm`, `*.webp` and `*.png`, *When* a tour page loads, *Then* step text, transcript and CTAs are present; every `figure` (step and phone) still occupies its reserved width and height (explicit `width`/`height` plus `aspect-ratio`; the test compares the figure's bounding box with and without blocking), its `alt` text is shown in place of the image, and no empty box or collapsed figure appears. The `<video>` keeps its poster-sized box and its inner fallback text is present in the DOM (it renders only when no `<source>` type is supported; a network or decode failure leaves the native player's error state with steps intact: ux.md s4, no error-event handler is added).
- Mobile (UX-6).
  - *Given* viewports 390x844 and 320x640, *When* `/tours/` and a tour load, *Then* `scrollWidth <= clientWidth`, card links, prev/next, transcript summary are at least 44x44 px, and phone-framed figures have alt.
- Keyboard (UX-7).
  - *Given* `keyboard-nav.spec.ts` extended, *When* tabbing a tour page, *Then* focus order is breadcrumb, video, **Install link after the clip**, transcript summary, steps links, "Try it" Install link, help link, prev/next, "All tours", and each stop has a visible focus ring.
- Reduced motion on tours and landing (UX-5, G11).
  - *Given* `reduced-motion.spec.ts` extended to loop over `/` and every tour page, *When* `prefers-reduced-motion: reduce`, *Then* no `<video>` autoplays and each has a poster and `controls`; and the computed `transition-duration` and `animation-duration` of the tour page's cards, links and figures are `0s` (the tour CSS wraps its transitions in `@media (prefers-reduced-motion: no-preference)`, so reduce is the default-off case).
- WCAG 2.2 AA additions (G3).
  - *Given* a tour page with the Starlight header sticky, *When* the keyboard focuses the last transcript, step and nav links (and the page is scrolled with `scrollIntoView` default behaviour), *Then* the focused element's bounding box is below the header's bottom edge (2.4.11 Focus Not Obscured; the tour layout sets `scroll-padding-top` from the header height token); *Given* the text-spacing override (line-height 1.5, paragraph spacing 2x, letter spacing 0.12em, word spacing 0.16em) injected as CSS, *When* `/tours/` and a tour page render at 390 px, *Then* no text is clipped or overlaps and `scrollWidth <= clientWidth` (1.4.12); *Given* the prev/next and "See the tour" links, *When* their accessible names are read, *Then* each equals its visible text (2.5.3; Cross-cutting 8f); 1.2.2 (captions for prerecorded audio) is **not applicable**: clips have no audio track (Playwright recordings are silent and the ffmpeg post-process passes `-an`, Task 3.1.1h) and the transcript covers the visual content (1.2.1, 1.2.3).
- Lazy media attributes (R12).
  - *Given* built `dist/tours/*/index.html`, *When* `tour-media-attrs.test.ts` runs, *Then* every `<video>` has `preload="none"`, `poster`, `controls`, no `autoplay`, and every `<img>` has `loading="lazy"` (except an above-fold hero) with explicit `width` and `height`.
- Transcript equals steps plus captions (R19).
  - *Given* a built tour page and its `<name>.transcript.json`, *When* `tour-transcript.test.ts` runs, *Then* the `<details>` text contains, in order, every step's `narration` as authored in the tour's `.mdx` frontmatter and every caption from the transcript JSON; a tour whose transcript JSON is missing or empty fails the build.
- helpDoc is a real website page.
  - *Given* a tour with `tour.helpDoc: help/does-not-exist`, *When* `bun run build` runs, *Then* it fails naming the tour and the field; every shipped `helpDoc` is a member of the docs collection ids (Task 3.1.1e).
- PR smoke rendering (UX-4, G6) and build stamp (UX-11).
  - *Given* `TOURS_MEDIA_MODE=deploy` and a tour whose clip is absent from `media-manifest.json`, *When* the site builds, *Then* the build fails naming the clip; *Given* `smoke`, *Then* that tour is **not built** (no page, no sidebar entry from the `tours` autogenerate group, no search hit, no grid card, no prev/next link, no landing card link, no README-link check), a built tour's unrecorded step shots render no figure, and there is no "coming soon" text anywhere. The meta row's "Watch M:SS" label renders only when the manifest has the clip's `durationMs` and is omitted otherwise (unit test on the meta-row component with no duration). *Given* no `media-manifest.json` in deploy mode, *Then* the build fails with a message naming the file; the stamp (format of the Build stamp AC) appears on both the tour pages and the `/tours/` index footer.
- Smoke-mode-parametrized structure (triad UX gaps).
  - *Given* `tours.spec.ts` runs in both modes, *When* it asserts UX-1, UX-2 and the Start-here strip, *Then* the expectations are parametrized by the manifest: no category `h2` is rendered for a category with zero recorded tours; the Start-here strip renders the recorded subset of `order` 1-3 and, with fewer than 3 recorded, shows only those (no empty card, no gap) and is omitted entirely at 0; prev/next skip unrecorded tours by construction and the last built tour's "next" becomes "Back to all tours" (Task 3.1.1i). In the PR smoke build only `dashboard-lifecycle` is built, so its prev is omitted and its next is "Back to all tours".
- Encoding fallback (Cross-cutting 7e).
  - *Given* a built tour page and the hero, *When* `tour-media-attrs.test.ts` runs, *Then* each `<video>` has a `<source type="video/webm">` first and a `<source type="video/mp4">` second, a `poster`, and both files exist in `dist` with sizes under the clip budget (Task 3.1.1h).
- Hero pause control (WCAG 2.2.2).
  - *Given* the landing hero clip auto-plays (muted, no reduced-motion preference), *When* the page loads, *Then* a visible, keyboard-focusable "Pause" button (toggling to "Play", `aria-pressed`) precedes the video in tab order and pauses it; Playwright asserts the button, its toggle and that the clip pauses (Task 3.1.1h).
- Hero edge cases (G4).
  - *Given* a browser context with JavaScript disabled, *When* the landing loads, *Then* the hero shows its poster with no autoplay and the native `controls` work (`<noscript>` poster; no Pause button is needed because nothing plays). *Given* `HTMLMediaElement.play()` rejecting (autoplay blocked, stubbed in Playwright), *When* the landing loads, *Then* the hero stays paused, the button reads "Play" with `aria-pressed="false"`, and activating it plays the clip. *Given* `prefers-reduced-motion: reduce`, *Then* the hero shows native `controls` only: no autoplay and no custom Pause/Play button is rendered (the button exists only when the clip can autoplay).
- Transcript describes the action (WCAG 1.2.1, 1.2.3).
  - *Given* a tour `.mdx`, *When* `tour-schema.test.ts` runs, *Then* each step has a `narration` field (what happens on screen, e.g. "The approval prompt for `git push` appears; focus moves to Allow; the session resumes") distinct from the imperative `text`, the `<details>` transcript renders `narration` for every step in time order alongside the captions, and a step whose `narration` equals its `text` fails the build.
- Install CTA also follows the clip; honest help label; sample-data and date disclosure.
  - *Given* a built tour page, *When* the DOM is read, *Then* an Install link appears directly after the clip block (inside the clip component, before the transcript) and again in the "Try it" block; the help link label is "Read the help page" only when `helpDoc` is not the generic `help/getting-started`, else "Getting started guide" (the label never promises a topic page that does not exist); the build stamp follows the one format of the Build stamp AC (date first, sha in `<small>`); and each tour page and `/tours/` carry the line "Recorded with sample data, not a real team's work." (Task 3.1.1i).
- Duration labelling is consistent.
  - *Given* a tour, *When* the meta row renders, *Then* it shows "Watch M:SS" (from the manifest `durationMs`, at most 1:00) and "Read N min" (from `tour.durationMin`, a read-time estimate of the steps), never one ambiguous "min"; the index card shows "Watch M:SS"; the index promise says "about a minute".
- Adding a tour is one spec plus one entry (R13).
  - *Given* a temporary tour pair (`tours/zz-fixture.ts` + `tours/zz-fixture.mdx`) in a fixture directory, *When* `add-tour-fixture.test.ts` runs `expected-media.mjs` and `mediatool shard-plan`, *Then* the new id is in the expected media and in exactly one shard, with no other file edited.
- Existing hero/reduced-motion suites stay green.
  - *Given* the new components, *When* `website/tests/unit/hero-video.test.ts`, `hero.test.ts`, `feature-card.test.ts` and `website/tests/e2e/reduced-motion.spec.ts` run, *Then* they pass (expectations updated where the hero source changes in Story 3.3.1).
**Component ownership (G2)**: `TourHeader.astro` = breadcrumb and meta row (audience, Watch/Read, flag note); `TourClip.astro` = the video block and the Install link that follows it; `TourSteps.astro` = transcript `<details>` and the `ol` steps; `TourShot.astro` = step and phone figures; `TourFooter.astro` = "Try it" block (Install, help link), prev/next and "All tours"; `TourGrid.astro` = index strip and category grids; `BuildStamp.astro` = stamp and sample-data line (Task 3.1.1f). All created in Task 3.1.1a except `BuildStamp` (3.1.1f).
**Files**: `website/src/content.config.ts`, `website/src/components/{TourClip,TourShot,TourSteps,TourGrid,TourHeader,TourFooter,BuildStamp}.astro`, `website/src/content/docs/tours/index.mdx`, `website/astro.config.mjs`, `website/tests/unit/hero-video.test.ts`, `website/tests/e2e/tours.spec.ts`, `website/tests/e2e/keyboard-nav.spec.ts`, `website/tests/e2e/reduced-motion.spec.ts`, `website/tests/unit/{tour-schema,tour-media-attrs,tour-transcript,tour-build-missing-clip,tour-build-stamp,add-tour-fixture,tour-contrast,tour-link-names}.test.ts`, `website/src/components/HeroVideo.astro`, `scripts/record-website-demos.sh`

##### Task 3.1.1a: Schema + components (M)
- Extend docs schema with the nested `tour` object per ADR-001 as amended in Phase 4: `title`, `outcome`, `clip`, `features[]` (registry ids), `helpDoc` (`WebsiteDocID`), `order`, `category`, `audience`, `durationMin`, optional `flagNote`, optional `phoneFigure` (a mobile shot name), `steps[]` (`text`, `shot`, `alt`, `caption` (figcaption), `narration`, optional `why`, optional `quote`, optional `annotated` default true); six components (ownership in the Story's component-ownership note: `TourHeader`, `TourClip`, `TourSteps`, `TourShot`, `TourFooter`, `TourGrid`) wrapping `reduced-motion-video.ts`, rendering the audience tag, "Read N min", flag note, figcaption and per-step why-it-matters line, and the two media modes of Cross-cutting 6c; `<track kind="captions">` deferred (transcript is the minimum); mp4 fallback implemented in Task 3.1.1h. Steps are `min(3).max(6)` (Cross-cutting 8h). The tour layout CSS sets `scroll-padding-top` from the Starlight header height token (2.4.11) and wraps transitions in `@media (prefers-reduced-motion: no-preference)`. Figures reserve their space with explicit `width`/`height` and `aspect-ratio` so an image that fails at runtime keeps its box and shows its alt text (G7). Trace: R3, R12, R19.
- Files: `content.config.ts`, 6 components (`TourClip`, `TourShot`, `TourSteps`, `TourGrid`, `TourHeader`, `TourFooter`), tour layout CSS.

- Triad round 1 additions to the schema and components: step `narration` (required, see the transcript AC), `tour.order` unique 1..N as the only ordering for Start-here and prev/next (Cross-cutting 7f), a "Watch M:SS / Read N min" meta row (`TourHeader`), the Install link after the clip (`TourClip`), the generic-help label rule (`TourFooter`), the build stamp in the one format of Cross-cutting 8g and the "sample data" line (`BuildStamp`, Task 3.1.1f), and distinguishable duplicate links: every prev/next and "See the tour" link shows its full label as visible text ("Previous tour: <title>", "Next tour: <title>", "See the tour: <title>") and has no `aria-label`, so two links to the same target are distinguishable and the accessible name equals the visible text (Cross-cutting 8f). Mobile phone-framed figures wrap the image in a link to the full-size shot (`target` same tab, `aria-label="Open full-size screenshot: <alt>"`, 44x44 px minimum) so small text is readable by tapping.

##### Task 3.1.1b: Index page and sidebar (S)
- `tours/index.mdx` with a "Start here" strip derived from `tour.order` 1-3 (`dashboard-lifecycle`, `approval-review`, `organise-at-scale`; not hard-coded), grouped by the 5 README categories; promise line "about a minute" (Cross-cutting 8h); build-stamp footer (`BuildStamp`); autogenerate sidebar entry (unbuilt smoke tours are excluded by the loader glob, Task 3.1.1f). Trace: R3.
- Files: `tours/index.mdx`, `astro.config.mjs`.

##### Task 3.1.1c: Schema unit tests (S)
- Vitest/bun tests: required alt, steps `min(3).max(6)`, unique `order`, `features[]` present, the content-quality checks of Epic 3.2 (G12), `clip` name matches a scene file in `tests/e2e/website-demos/tours/`, every `.mdx` has a scene file and vice versa, nothing else registered (Cross-cutting 6f); `tour-media-attrs.test.ts` and `tour-transcript.test.ts` per the Story ACs. Trace: R13, R12, R19.
- Files: `website/tests/unit/{tour-schema,tour-media-attrs,tour-transcript}.test.ts`.

##### Task 3.1.1d: `tours.spec.ts` and extended keyboard/reduced-motion specs (M)
- New `website/tests/e2e/tours.spec.ts` covering UX-1 (index structure), UX-2 (DOM order), UX-3 (all media blocked: the figure bounding box is unchanged and the alt text is present and non-empty; no `naturalWidth` assertion), UX-6 (390x844 and 320x640: no overflow, 44x44 targets, phone-figure alt); extend `keyboard-nav.spec.ts` for UX-7 (tab order including the Install link after the clip, visible focus ring, and the 2.4.11 not-obscured-by-sticky-header check) and `reduced-motion.spec.ts` for UX-5 (loop over `/` and every tour; the `0s` transition/animation durations check). The UX-3 case asserts the figure bounding box is unchanged with media blocked and the alt text is present. The smoke build always includes `dashboard-lifecycle`, so `tours.spec.ts` also asserts in smoke mode that at least one tour page is built and `/tours/` lists at least one tour (zero built tours fails the spec, matching the empty-index build failure). The text-spacing (1.4.12) case lives here too. These run in `deploy-website.yml` against full media and in `website-ci.yml` against `/tours/` plus the smoke subset (the grid lists only recorded tours). No `waitForTimeout`, ARIA/`data-testid` locators. Trace: R3, R10, R12, R19.
- Files: `website/tests/e2e/tours.spec.ts`, `website/tests/e2e/keyboard-nav.spec.ts`, `website/tests/e2e/reduced-motion.spec.ts`.

##### Task 3.1.1e: `helpDoc` validation and target choice (S)
- Validate every `tour.helpDoc` against the Starlight docs collection ids (including synced `help/*` pages) in the Zod schema's refinement or a build-time check in `content.config.ts`; the error names the tour and field. Pick the real page for each tour (`approval-review` defaults to `help/getting-started` because no website approvals page exists; if the spike finds a better synced how-to, use it, and record any new help page as a follow-up, not scope). Update Story 3.2.1's AC. Trace: R3, R4.
- Files: `website/src/content.config.ts`, `website/tests/unit/tour-schema.test.ts`.

##### Task 3.1.1f: Media modes, manifest-driven grid, missing-manifest error (S)
- Implement `TOURS_MEDIA_MODE` (`deploy` default, `smoke`) in `TourClip`, `TourShot` and `TourGrid` per Cross-cutting 6c, reading `media-manifest.json`; deploy mode fails the build for a missing clip, shot or manifest; smoke mode does not build unrecorded tours and omits unrecorded step media of built tours; build-stamp component for tour pages and the index footer. `content.config.ts` also computes the smoke-mode glob exclusion for tours absent from the manifest (Cross-cutting 6c), which removes them from the sidebar autogenerate group, grid, prev/next and search. Tests: `tour-build-missing-clip.test.ts` (deploy fails; smoke does not build the unrecorded tour, no "coming soon" text, `TourGrid` and the sidebar exclude it) and `tour-build-stamp.test.ts` (stamp "Recorded 2026-11-10 (build <small>a1b2c3d</small>)" equals the manifest date and commit; "Recorded from build" is absent; missing manifest fails). Trace: R5, R3, R12.
- Files: `website/src/components/{TourClip,TourShot,TourGrid,BuildStamp}.astro`, `website/tests/unit/{tour-build-missing-clip,tour-build-stamp}.test.ts`.

##### Task 3.1.1g: Add-a-tour proof (S)
- `website/tests/unit/add-tour-fixture.test.ts` per the Story AC: a temp tour pair in a fixture dir run through `expected-media.mjs` and `mediatool shard-plan` yields the new id in expected media and in exactly one shard, with no other file edited. Trace: R13.
- Files: `website/tests/unit/add-tour-fixture.test.ts`.

##### Task 3.1.1h: Video encoding fallback and hero pause control (M)
- Post-process each clip into WebM (VP9) and H.264 MP4 (`-pix_fmt yuv420p -movflags +faststart`, constrained to the 3 MB budget; both are manifest rows and `ExpectedMedia` names, Cross-cutting 7e) in `scripts/record-website-demos.sh`/the Go post-process step; `TourClip` and `HeroVideo` emit `<source>` webm then mp4 plus `poster`; add a hero Pause/Play button (visible, focusable, before the video in tab order, `aria-pressed`, label toggles) wired through `reduced-motion-video.ts` so reduced-motion users get `controls` and no autoplay as today (and no Pause/Play button) and everyone else can stop the motion (WCAG 2.2.2); the post-process passes `-an` (no audio track, WCAG 1.2.2 not applicable). Edge cases (Story 3.1.1 "Hero edge cases" AC): JavaScript disabled shows the poster with native controls and no autoplay; a rejected `play()` leaves the hero paused with the button reading "Play" (`aria-pressed="false"`). Tests: `tour-media-attrs.test.ts` (source order, both files exist, sizes), `hero-video.test.ts` updated, Playwright `landing.spec.ts` asserts the pause button toggles playback and covers the JS-disabled and rejected-`play()` cases; a Go test that `mediatool fresh` fails when only one rendition is present. Trace: R12, R5, R3.
- Files: `scripts/record-website-demos.sh`, `website/src/components/{TourClip,HeroVideo}.astro`, `website/src/lib/reduced-motion-video.ts`, `website/tests/unit/{tour-media-attrs,hero-video}.test.ts`, `website/tests/e2e/landing.spec.ts`, `tools/promo/cmd/mediatool/fresh_test.go`.

##### Task 3.1.1i: Smoke-parametrized UX assertions, accessibility checks and deferrals (S)
- Make `tours.spec.ts` (UX-1, UX-2, 3.1.1d) read the manifest and assert per the "Smoke-mode-parametrized structure" AC; add an accessible-name test (no two links on a tour page share the same accessible name and different targets; same-target links such as prev/next and "See the tour" are told apart by their visible text, and each accessible name equals its visible text: WCAG 2.5.3, Cross-cutting 8f); a **contrast check of the annotation badge tokens** computed from the theme tokens (text >= 4.5:1, shape >= 3:1, unit test over the token pairs; pixel-level contrast of the rendered badge over screenshots is an **owner manual check** listed in the 6-verify checklist). A **manual screen-reader pass** (VoiceOver or NVDA over `/tours/` and one tour page: landmarks, `ol` steps, `figcaption`, transcript `<details>`) is an owner check recorded in the 6-verify checklist and in `docs/how-to/add-a-website-tour.md`. **Forced-colors and print** tests (`forced-colors: active` emulation asserting focus ring and badge visibility; print emulation hiding the clip and keeping steps) are **explicitly deferred to 6-verify** as owner-run checks, not automated here. Trace: R12, R19.
- Files: `website/tests/e2e/tours.spec.ts`, `website/tests/unit/{tour-contrast,tour-link-names}.test.ts`.

#### Story 3.1.2: Feature-card media by name (kill the two-state `features.json`)
**As a** maintainer, **I want** card media declared by name, **so that** `features.json` is identical in git and in the deployed build. (R5, R20)
**Acceptance Criteria**:
- Deterministic output.
  - *Given* no recorded files present, *When* `node scripts/build-features.mjs` runs, *Then* `features.json` equals the committed file and `git diff --exit-code` passes.
- Old mapping removed.
  - *Given* the repo, *When* `grep DEMO_CLIP_BY_HEADING website/scripts` runs, *Then* zero matches.
- Existing feature suites green.
  - *Given* the refactor, *When* `website/tests/unit/{features-json,features-md,features-mdx,feature-card}.test.ts` and `website/tests/integration/features-json.integration.test.ts` run, *Then* they pass with expectations updated for `feature-media.json`.
**Files**: `website/scripts/build-features.mjs`, `website/src/content/feature-media.json`, `website/src/content/features.json`, `website/tests/unit/build-features.test.ts`, `website/tests/unit/{features-json,features-md,features-mdx}.test.ts`, `website/tests/integration/features-json.integration.test.ts`

##### Task 3.1.2a: `feature-media.json` and refactor (M)
- Replace `DEMO_CLIP_BY_HEADING` and the `docs/features.md` image requirement (`:190-211`) with `feature-media.json` (`heading -> {clip?, shot, alt}`); no existence checks at this stage (freshness gate does that). Trace: R5, R2.
- Files: as above.

#### Story 3.1.3: Extend site gates to tours
**As a** maintainer, **I want** Lighthouse, axe and link checks to cover tours, **so that** new pages cannot regress quality. (R8, R12)
**Acceptance Criteria**:
- Lighthouse covers tours.
  - *Given* `lighthouserc*.json`, *When* inspected, *Then* its URL list includes `/tours/` and `/tours/dashboard-lifecycle/` at the existing >=0.90 thresholds, desktop and mobile.
- Link checker covers media.
  - *Given* a tour page with `<video poster="/stapler-squad/shots/missing.webp">` in deploy mode, *When* `check-internal-links.mjs` runs, *Then* it exits 1 naming the poster. In smoke mode the HTML has no `<video>` for unrecorded media (Cross-cutting 6c), so no allow-list exists.
- Tour Lighthouse and the all-tours axe run in the deploy job only (see Story 2.4.1 PR Lighthouse AC); PR runs cover the `/tours/` index with Lighthouse and axe, plus axe on the one smoke tour page `/tours/dashboard-lifecycle/` (G5).
**Files**: `website/lighthouserc*.json`, `website/tests/unit/lighthouserc.test.ts`, `website/scripts/check-internal-links.mjs`, `website/tests/unit/check-internal-links.test.ts`, deploy workflow axe step list

##### Task 3.1.3a: Gates and link-check coverage (M)
- Add tour URLs to Lighthouse/axe lists; extend link checker to `src/poster/<source>`; add a tour page and the `/tours/` index to the mobile-overflow check (the 44 px and 320 px assertions live in Task 3.1.1d). Trace: R8, R12.
- Files: as above.

## Epic 3.2: Tour content and shots

**Goal**: 8 tours and a screenshot for every user-facing card, plus a mobile set. (R2, R10, R13, R19)

Each tour = `tests/e2e/website-demos/tours/<id>.ts` + `website/src/content/docs/tours/<id>.mdx` (nested `tour:` frontmatter: `features[]` registry ids, `helpDoc` as a `WebsiteDocID`, 3-6 steps with alt, transcript derived). Scenes assert on the strings their steps quote (Task 2.3.1f). Tour-story ACs below are frontmatter and build ACs only: sidecar links for tour-covered ids are authored and checked in Story 4.1.7, never required here. All copy uses "Grimdark" only (R15).

**Content-quality ACs for every tour story 3.2.1-3.2.4 (G12).** Each tour passes these before its story is done:
- *Narration.* Every step has a `narration` that differs from `text` (schema, Story 3.1.1) and is rendered in the transcript.
- *Alt text.* Machine check: `alt` is 20-300 characters and differs from the step `caption` and the shot filename (`tour-schema.test.ts`). Quality (describes the point in 1-2 sentences, not pixels) is an **owner check**, recorded as a checked list in the story's PR description.
- *Figcaption marker text.* Every annotated step's `caption` matches `/\bmarkers?\b/i` and says what each numbered gold marker shows (`tour-schema.test.ts`); steps whose shot carries no badge are marked `annotated: false` and exempt.
- *Flag note.* A scene that enables a feature flag exports `flagsEnabled: string[]`; `tour-schema.test.ts` reads it textually and requires a non-empty `tour.flagNote` for every tour whose scene lists any flag (backlog-pipeline: "Backlog is enabled in this recording"), and none otherwise.
- *Length.* 3-6 steps and a clip of at most 60 s (Story 3.1.1 length-cap AC).

#### Story 3.2.1: Tours 1-2 (hero)
**As an** evaluator, **I want** dashboard-lifecycle and approval-review tours, **so that** I see the core value in under a minute. (R2, R3)
**Acceptance Criteria**:
- Tour recorded and mapped.
  - *Given* `dashboard-lifecycle.ts` and `.mdx` with `tour.clip: dashboard-lifecycle`, *When* the shard records, *Then* `media-manifest.json` has `dashboard-lifecycle` (clip, desktop, `themeId: grimdark`) and 4+ shot entries.
- Docs links.
  - *Given* `approval-review.mdx` with `tour.helpDoc: help/getting-started` (a `WebsiteDocID`; `configuration` is an in-app slug with no website page and is not used), *When* the page builds, *Then* the "Read the help page" CTA links to `/stapler-squad/help/getting-started/` and `check-internal-links.mjs` resolves it (Task 3.1.1e).
**Files**: `tests/e2e/website-demos/tours/dashboard-lifecycle.ts`, `approval-review.ts`, `website/src/content/docs/tours/dashboard-lifecycle.mdx`, `approval-review.mdx`

##### Task 3.2.1a: dashboard-lifecycle (M)
- Omnibar create -> live terminal -> diff -> pause/resume; reuses existing `session-dashboard` scene. Trace: R2, R19.
##### Task 3.2.1b: approval-review (M)
- Needs-approval -> Review Queue -> Auto-Approval Rules -> custom rule -> Approval Analytics; extends `auto-approval-rules` clip. Trace: R2, R19.

#### Story 3.2.2: Tours 3-4
**As a** power user, **I want** organise-at-scale and backlog-pipeline tours. (R2, R3)
**Acceptance Criteria**:
- Flag-gated tour is verified.
  - *Given* `backlog` flag enabled by the recorder, *When* `backlog-pipeline` records, *Then* the final URL is `/backlog/flow` and the landmark `data-testid="backlog-flow"` (or ARIA role per scene) is visible before capture.
- Feature ids are real.
  - *Given* `organise-at-scale.mdx` with `tour.features: [session-board-view]`, *When* the site builds, *Then* every id exists in `docs/registry/features/` (build-time check against the registry file listing); the sidecar link is verified in Story 4.1.7.
**Files**: `tours/organise-at-scale.ts`, `tours/backlog-pipeline.ts`, matching `.mdx`

##### Task 3.2.2a: organise-at-scale (M)
##### Task 3.2.2b: backlog-pipeline (M)

#### Story 3.2.3: Tours 5-6
**As a** power user, **I want** fan-out and insights tours. (R2, R3)
**Acceptance Criteria**:
- Insights show seeded data.
  - *Given* scenario `insights` seeded, *When* `insights-cost` captures `/insights`, *Then* the shot shows non-empty daily spend and the DOM dump contains no leak patterns.
- Fan-out uses isolated workspaces fixture.
  - *Given* scenario `fanout`, *When* `/fanout` is captured, *Then* 3 variant cards are visible with fixture repo paths under the temp dir name `demo-fixtures`, not a runner path.
**Files**: `tours/fanout-compare.ts`, `tours/insights-cost.ts`, matching `.mdx`

##### Task 3.2.3a: fanout-compare (M)
##### Task 3.2.3b: insights-cost (M)

#### Story 3.2.4: Tours 7-8
**As a** power user, **I want** attention/notifications and automation tours. (R2, R3)
**Acceptance Criteria**:
- Notifications tour has no real integration data.
  - *Given* scenario `notifications`, *When* captured, *Then* the DOM dump contains no `.internal`, email or token pattern.
- Automation tour covers triggers, workflows, tagging rules.
  - *Given* `automation-rules.mdx`, *When* built, *Then* its `tour.features` lists at least 3 registry ids and each exists in `docs/registry/features/` (sidecars: Story 4.1.7).
**Files**: `tours/attention-notifications.ts`, `tours/automation-rules.ts`, matching `.mdx`

##### Task 3.2.4a: attention-notifications (M)
##### Task 3.2.4b: automation-rules (M)

#### Story 3.2.5: Single-shot feature coverage
**As a** visitor, **I want** a current screenshot for every feature card (and any view a tour references). (R2)
**Acceptance Criteria**:
- Every card has a shot.
  - *Given* `feature-media.json` with one entry per card heading (19 today; the count is whatever `docs/features.md` holds once Story 3.3.3 is settled), *When* the recorder finishes, *Then* `mediatool fresh` finds a `MediaEntry` for each shot name and none is missing.
- No orphan media.
  - *Given* `expected-media.json`, *When* a test compares it with the names referenced by `feature-media.json` and tour frontmatter, *Then* the two sets are equal (every expected shot has a consumer and vice versa). The 11 extra "remaining views" shots (history, logs, config, escape analytics, files, artifacts, import, settings, inspector, changelog, help) are **dropped**: no card, tour or README section consumes them. A view gets a shot only when a tour step or card references it.
**Files**: `tests/e2e/website-demos/shots.ts` (index), `tests/e2e/website-demos/shotlists/cards.ts`, `website/src/content/feature-media.json`, `website/tests/unit/expected-media.test.ts`

##### Task 3.2.5a: Card shots (M)
- One shot per card, alt text per card, Grimdark-asserted. Trace: R2.
##### Task 3.2.5b: Consumer-equality test (S)
- Add the expected-vs-referenced equality test; no extra view shots. Trace: R2, R13.

#### Story 3.2.6: Mobile-viewport shot set
**As a** mobile visitor, **I want** readable phone-layout screenshots. (R10)
**Acceptance Criteria**:
- Mobile set exists and is shown.
  - *Given* mobile shots `dashboard-mobile`, `review-queue-mobile`, `terminal-mobile`, `backlog-mobile`, *When* tour pages render at 390 px, *Then* each appears as a phone-framed figure with required alt and no horizontal scroll.
- Which tours carry a phone figure (G10, `tour.phoneFigure`; one per tour).
  - *Given* the eight tours, *Then* exactly four set `phoneFigure`: `dashboard-lifecycle` -> `terminal-mobile`, `organise-at-scale` -> `dashboard-mobile`, `approval-review` -> `review-queue-mobile`, `backlog-pipeline` -> `backlog-mobile`; the other four (`fanout-compare`, `insights-cost`, `attention-notifications`, `automation-rules`) have none, and the consumer-equality test (Task 3.2.5b) finds each mobile shot referenced by exactly one tour.
**Files**: `tests/e2e/website-demos/shotlists/mobile.ts`, `TourShot.astro`

##### Task 3.2.6a: Mobile shots and figure variant (S)
- Capture the four mobile shots (390x844, touch); `TourShot` `variant="phone"` renders the figure inside a link to the full-size shot (tap-to-open, 44x44 px, `aria-label`), so small text stays readable on a phone. Trace: R10.

## Epic 3.3: Entry points

**Goal**: Landing, features grid and README point at tours. (R3, R18)

#### Story 3.3.1: Landing hero and features grid
**As a** visitor, **I want** the hero and grid to use current media and link to tours. (R3, R5)
**Acceptance Criteria**:
- Hero uses a CI clip.
  - *Given* `index.mdx`, *When* built, *Then* the hero `<video>` source is `/stapler-squad/clips/dashboard-lifecycle.webm` and `website/public/demo.webm` is not referenced.
- Landing links to the tours index (UX-12).
  - *Given* `index.mdx`, *When* built, *Then* directly under the hero CTAs there is a secondary link "See the tours" to `/stapler-squad/tours/`, `landing.spec.ts` asserts it, and `check-internal-links.mjs` resolves it; cards that have a tour show a second link "See the tour: <title>" next to the media (visible text is the accessible name, Cross-cutting 8f; in a smoke build only for tours that are built, so no card links to an unbuilt page).
- Stale count fixed.
  - *Given* `help/features.mdx`, *When* built, *Then* the card count text equals the number of cards in `features.json`.
- Existing landing suites stay green.
  - *Given* the hero source change, *When* `website/tests/e2e/landing.spec.ts` (which intercepts `**/demo.webm`, lines ~74-140) and `website/tests/unit/hero-video.test.ts` run, *Then* they pass with the intercept and assertions retargeted to `/clips/dashboard-lifecycle.webm`.
**Files**: `website/src/content/docs/index.mdx`, `website/src/content/docs/help/features.mdx`, `website/src/components/FeatureCard.astro`, `website/tests/e2e/landing.spec.ts`, `website/tests/unit/hero-video.test.ts`

##### Task 3.3.1a: Hero, card links, count (S)
- Switch hero source, add the hero secondary link "See the tours", add "See the tour" link on cards that have a tour, derive count. Trace: R3.

#### Story 3.3.2: README links to tours
**As a** GitHub visitor, **I want** README links to tour pages, **so that** nothing binary lives in the repo. (R18, R1)
**Acceptance Criteria**:
- No embeds.
  - *Given* `README.md`, *When* grepped for `assets/demo` and `assets/screenshot`, *Then* zero matches, and it links the tours URL derived from the same `SITE_ORIGIN`/`SITE_BASE` defaults as `website/astro.config.mjs:9-10` (a unit test compares the README link with those defaults rather than hard-coding the host).
- Start-here links and link check (UX-12).
  - *Given* `README.md`, *When* read, *Then* a "Tours" section has three text links (the Start-here tours `dashboard-lifecycle`, `approval-review`, `organise-at-scale`, link text describing the destination, each with a one-line outcome) plus one link to `/tours/`; and *When* `readme-tours-link.test.ts` plus the README link check (Task 3.3.2c) run, *Then* every README URL under the site origin resolves to a path in `dist` (or to the sitemap in a no-build unit run; in a smoke build links to unbuilt tours are skipped with a notice, Task 3.3.2c) and a wrong `SITE_BASE` fails.
**Files**: `README.md`, `docs/**` references to `assets/`, `website/tests/unit/readme-tours-link.test.ts`, `website/scripts/check-readme-links.mjs`

##### Task 3.3.2a: README rewrite of media section (S)
##### Task 3.3.2b: Fix other references to removed assets (XS)
##### Task 3.3.2c: README link check (S)
- `website/scripts/check-readme-links.mjs` extracts every URL under the site origin from `README.md` and checks it against `dist` (run in `deploy-website.yml` and `website-ci.yml` after `bun run build`); `check-internal-links.mjs` itself scans only `dist`, so this is a separate small script with a unit test. In a smoke build (`TOURS_MEDIA_MODE=smoke`) README links to tour pages that were not built are skipped with a notice naming them, while `/tours/` and the recorded tour must resolve; in deploy mode every README tour link must resolve (G6). Trace: R18, R3.
- Files: `website/scripts/check-readme-links.mjs`, `website/tests/unit/check-readme-links.test.ts`, workflow steps.

#### Story 3.3.3: Cards for tour-covered areas (owner review)
**As an** evaluator, **I want** cards for areas newer than the original 19 (backlog, insights, owner inbox / Up Next, automation, files, import), **so that** the grid reflects the product. (R2)
**Acceptance Criteria**:
- Each new card links to a tour or shot.
  - *Given* new sections in `docs/features.md`, *When* `build-features.mjs` runs, *Then* each new card has a category, alt and a `feature-media.json` entry, or the build fails with the heading named.
**Files**: `docs/features.md`, `website/src/content/feature-media.json`, `website/src/content/features.json`

##### Task 3.3.3a: Add minimal one-sentence cards (S)
- Limited, factual entries only (product copy rewrite is out of scope); owner reviews wording. Trace: R2.

---

# Phase 4: Registry docs coverage

## Epic 4.1: Docs-coverage gate (ADR-005)

**Goal**: Registry features carry verified docs links; new/changed features cannot ship without them; the baseline only shrinks. (R4, R16, R17)

#### Story 4.1.1: Sidecar schema and coverage checker
**As a** contributor, **I want** a validator that resolves `help`, `website` and `tour` links, **so that** coverage claims are real. (R4)
**Acceptance Criteria**:
- Help slug resolves against the code, not names.
  - *Given* sidecar `help: "backlog"` and `DOC_FILES` containing `backlog` with `web-app/public/docs/backlog.md` present, *When* `docscoverage` runs, *Then* status `covered`; with `help: "reviews"` it reports `reviews: not in DOC_FILES` with the fix location.
- `DOC_FILES` vs directory drift caught.
  - *Given* `DOC_FILES` lists `workflows` but `web-app/public/docs/workflows.md` is deleted, *When* it runs, *Then* it fails naming the slug.
- Tour link is bidirectional.
  - *Given* sidecar `tour: organise-at-scale` for id `session-board-view` and `tours/organise-at-scale.mdx` whose `tour.features` lacks `session-board-view`, *When* it runs, *Then* it reports the mismatch.
- Anchors validated.
  - *Given* `website: "help/getting-started#install"` and no `## Install` heading, *When* it runs, *Then* it reports `anchor install not found`.
- Registry loader handles the real shapes.
  - *Given* `docs/registry/features/escape-analytics.json` (a JSON array) and the two files that both declare `launcher_presets:get` (`backend/launcher-presets/get.json`, `backend/launcher_presets/get.json`), *When* the loader runs, *Then* it returns one record per entry keyed by `id` (not filename), collapses the duplicate id to one record, and reports the duplicate as a warning naming both files (not a gate failure).
- Sidecar filenames are filesystem-safe and injective.
  - *Given* id `alias:invoke`, *When* its sidecar path is computed, *Then* it is `docs/registry/docs-coverage/alias%3Ainvoke.json` (only `%`, `:` and `/` are percent-encoded), the file's inner `id` is `alias:invoke`, and a test asserts `decode(encode(id)) == id` for every id in the registry (20 of the 163 default-userFacing ids contain `:`).
- Orphan sidecars fail.
  - *Given* a sidecar whose `id` is in no registry file, *When* `docscoverage` runs, *Then* it fails naming the file (so the grandfather baseline can only shrink).
- Many-to-one is allowed; exempt is policed.
  - *Given* ten features whose `website` is `help/getting-started#install`, *When* it runs, *Then* all ten are covered (one page may cover many features); `exempt` requires a non-empty reason and the report prints the exempt count so overuse is visible; an `exempt` ratio above a ceiling in `tools/docscoverage/budgets.json` (set at baseline) fails; a PR that raises the total exempt count by more than 5 fails; every `exempt` sidecar names an `approver` (a GitHub login) and `docs/registry/docs-coverage/` is covered by a `.github/CODEOWNERS` entry so exemptions need an owner review (pre-mortem P1-4 step 4).
- Sync-generated pages are attributed to their source.
  - *Given* `website: "help/add-user-themes"` whose page is generated from `docs/how-to/add-user-themes.md` via `content-manifest.json`, *When* it runs, *Then* it resolves against the generated page's id and the report names the source file to edit (worked example in the how-to, Task 4.1.6a).
**Files**: `docs/registry/docs-coverage/schema.json`, `tools/docscoverage/{cmd/docscoverage/main.go,internal/resolve/*.go,internal/model/*.go,internal/registry/*.go,budgets.json}`, `tools/promolib/gitbase/*.go` (reused), `docs/registry/README.md`, `web-app/src/lib/docs/docLoader.shape.test.ts`

##### Task 4.1.1a: Schema, types, README fix (S)
- `schema.json` for sidecars, Go newtypes (`FeatureID`, `DocSlug`, `WebsiteDocID`), update `docs/registry/README.md` (monolith language is stale) and reconcile its `coverage-gaps.json` "gap count must not grow" prose with the new per-feature ratchet so there is one baseline, not two. Trace: R4.
##### Task 4.1.1d: Registry loader and sidecar identity (S)
- `internal/registry`: read `docs/registry/features/**.json` incl. array-shaped files and nested `backend/<group>/` dirs, key by `id`, duplicate-id rule above, userFacing default (frontend + root true, backend false unless sidecar says true), filename encode/decode, orphan check, uniqueness test. Trace: R4, R11.
- Files: `tools/docscoverage/internal/registry/*.go`, tests.
##### Task 4.1.1b: Resolvers (M)
- `DOC_FILES` literal parser, public/docs listing, website docs ids from filesystem frontmatter, tour bidirectional check, anchors validated with the same slug algorithm Starlight uses (`github-slugger`; the Go checker ports its rules and a test compares both on the real headings), `exempt` rules. The shape-guard that fails if `DOC_FILES` stops being a literal runs both in the Go checker and as a web-app jest test (`docLoader.shape.test.ts`, which shells the Go parser's fixture or re-parses in TS) so web-app-only PRs catch it; `detect-changes` also selects the gate for `web-app/src/lib/docs/**` (Task 4.1.4a). Needs 0.1.1a and Task 4.1.1d (registry loader first). Trace: R4, R11.
- Files: `tools/docscoverage/internal/resolve/*.go`, `web-app/src/lib/docs/docLoader.shape.test.ts`, tests.
##### Task 4.1.1c: Report output (S)
- Per-feature actionable lines plus coverage count to stdout and `$GITHUB_STEP_SUMMARY`. Trace: R4, R16.
- Files: `tools/docscoverage/internal/report/*.go`, `tools/docscoverage/internal/report/report_test.go` (`TestReportPrintsPerFeatureMissingFieldsAndCount`: per-feature missing-field lines, count, exempt and grandfathered counts shown next to covered, never a single percentage).

#### Story 4.1.2: Changed-feature detection and ratchet rules
**As a** maintainer, **I want** the gate to fire only on new or meaningfully changed features, **so that** refactors do not trip it. (R4)
**Acceptance Criteria**:
- Non-semantic fields ignored.
  - *Given* a frontend entry whose only diff vs the base is any of `lastModified`, `markerLine`, `handlerFile`, `tested`, `testIds` (e.g. `tested: false -> true` with `testIds` appended, the documented "feature now has tests" edit in `docs/registry/README.md`), *When* `docscoverage -changed` runs, *Then* it is not treated as changed.
  - *Given* an entry whose `name`, `description` or `userFacing` differs from base, *When* it runs, *Then* it is changed.
  - *Given* an existing id whose only differences are `path`/`filePath` or `component` (a component move or rename), *When* it runs, *Then* it is **not** changed; the same two fields on a brand-new id are irrelevant because the id is new (pre-mortem P1-4 step 2).
- Unresolvable base never blocks.
  - *Given* a `push` event whose HEAD has one parent (squash or fast-forward) with `-base` set from `github.event.before`, *When* the base resolves, *Then* it is used; *Given* `workflow_dispatch` or an all-zero `before`, *When* the base cannot be resolved, *Then* the checker prints a notice, runs report-only and exits 0 even with `docsCoverageEnforce` `true`.
- Grandfathered-on-change passes.
  - *Given* a grandfathered id whose semantic fields changed, *When* it runs, *Then* it is **not** a failure (the ratchet only forbids new ids and forbids keeping `grandfathered` once docs exist); the report lists it under "changed, still grandfathered" so the debt is visible.
- New feature without docs fails (when enforcing).
  - *Given* new `docs/registry/features/frontend/foo-panel.json` (`userFacing` by default) with no sidecar, *When* `docsCoverageEnforce` is `true` in `.github/promo-flags.json`, *Then* exit 1 with `foo-panel: missing help, website`.
- Identity is the id, not the filename.
  - *Given* a file renamed with the same `id` and unchanged semantic fields, or an entry moved between files, *When* it runs, *Then* it is not new; an id new to the base is new even if its filename existed.
- Grandfathering is base-aware.
  - *Given* sidecar `status: grandfathered` for id `new-thing` that is absent from `origin/main`, *When* it runs, *Then* it fails with `grandfathered only valid for ids existing on base`.
  - *Given* a grandfathered id that now has `help` and `website`, *When* it runs, *Then* it fails asking to drop `status` (baseline shrink).
**Files**: `tools/docscoverage/internal/changed/*.go`, `tools/promolib/gitbase/*.go` (reused: base blob read by tree lookup, no CLI), tests

##### Task 4.1.2a: Semantic-diff predicate and ratchet (M)
- Compare base vs HEAD records by `id` over semantic fields only (new `id`; `name`, `description`, `userFacing` on an existing id); `component` and `path`/`filePath` never count for an existing id; ignore `tested`, `testIds`, `lastModified`, `handlerFile`, `markerLine` and any other field not in that list (default-ignore, so new scanner fields cannot fire the gate). Base is `-base <sha>` (CI: `HEAD^1` of the PR merge commit on `pull_request`; for a `push` to main, `-base ${{ github.event.before }}`), blobs read through go-git. **Unresolved base is report-only, never a failure, for `docscoverage`** (unlike `mediaguard`, which exits 2): `workflow_dispatch`, merge-queue runs, a force-push `before` of all zeros, a one-parent HEAD with default `-base`, or a base commit absent from the checkout all print a notice and exit 0 with the report only (pre-mortem P1-4 step 3). Ratchet logic per ADR-005 incl. grandfathered-on-change. Trace: R4.

#### Story 4.1.3: Baseline generation
**As a** maintainer, **I want** one command to create the grandfather sidecars, **so that** existing gaps are recorded per feature. (R4)
**Acceptance Criteria**:
- Baseline is per feature and idempotent.
  - *Given* the 163 default-userFacing registry ids (148 frontend files + 15 entries in the 13 root files; whole registry 349 backend + 148 frontend + 13 root = 510 files; counted with `find docs/registry/features/frontend -name '*.json' | wc -l`, etc.) of which G lack docs (G is measured by Task 0.1.1f and `docscoverage -report`, not assumed), *When* `make docs-coverage-baseline` runs twice, *Then* it creates exactly G sidecars with `status: grandfathered`, named by the percent-encoded id, and the second run changes nothing. Backend ids (349 files) get no sidecar unless `userFacing: true`.
**Files**: `tools/docscoverage/cmd/docscoverage/baseline.go`, `make/promo.mk`, `docs/registry/docs-coverage/*.json`

##### Task 4.1.3a: `baseline` subcommand + Makefile target (S)

#### Story 4.1.4: CI wiring (report-only)
**As a** maintainer, **I want** the checker in `build.yml` beside the registry step, **so that** gaps are visible without blocking. (R4, R17, R16)
**Acceptance Criteria**:
- Report-only default.
  - *Given* `docsCoverageEnforce` `false` in `.github/promo-flags.json`, *When* a PR adds a feature without docs, *Then* the step passes, the job summary lists the missing fields and count.
- Path filter covers inputs.
  - *Given* a PR changing only `web-app/public/docs/backlog.md`, *When* CI runs, *Then* the docs-coverage step runs in the always-run `promo-guards` job of `backlog-scaffolding-guard.yml` (Phase 6 correction: putting it in `build.yml`'s `test-affected` made every content-only PR run ~20 checks, so `detect-changes` stays as on `main` and `build.yml` keeps the step for `push` only).
**Files**: `.github/workflows/build.yml`, `make/promo.mk`

##### Task 4.1.4a: Workflow step, filters (S)
- Add separate step near `:938-958` (do not edit the test gate). Task 4.1.4b adds bypass monitoring beside it. **Events**: on `pull_request` the job fetches depth 2 (workflow plumbing) and runs `docscoverage -base HEAD^1`; on `push` to main (the workflow also triggers there, VERIFIED `on.push.branches: [main]`) it runs `-base ${{ github.event.before }}` after fetching that commit, and falls back to report-only when unresolvable (Task 4.1.2a); `workflow_dispatch` and merge-queue runs are report-only. A workflow-yaml test asserts the step passes an explicit `-base` per event; **superseded (Phase 6 fix):** the `pull_request` run moved to the `promo-guards` job (`docscoverage -base HEAD^1` on a depth-2 checkout, no `detect-changes` involvement) and `detect-changes` is unchanged from `main`; `build.yml` runs the step on `push` only, for the trend artifact. Trace: R4, R17.

##### Task 4.1.4b: Bypass monitoring (exempt and baseline trend) (S)
- The gate does not backfill existing drift (non-goal); the risk is contributors routing around it with `exempt` or by growing the baseline. `docscoverage` writes one line per run to `$GITHUB_STEP_SUMMARY` and a `docs-coverage-trend.json` artifact: counts of `covered`, `exempt`, `grandfathered`, `gap`, and the delta of `exempt` and `grandfathered` against the base (read via `gitbase.ReadBlob`); a PR that raises `grandfathered` at all fails (ratchet), one that raises `exempt` by more than 5 fails (Story 4.1.1). The weekly `notify` run reads the last 8 weekly trend artifacts and comments on the flag tracking issue when `exempt` grew in 3 of the last 4 weeks or its ratio passed 80% of the ceiling. Test: fixture trend series triggers the comment, a flat series does not. Trace: R4, R16.
- Files: `tools/docscoverage/internal/report/trend.go`, `tools/docscoverage/internal/report/trend_test.go`, `.github/workflows/deploy-website.yml` (notify step).

#### Story 4.1.5: Flip gate to blocking
**As a** maintainer, **I want** the gate to block new/changed features, **so that** docs cannot drift. (R4, R17)
**Acceptance Criteria**:
- The flip has a numeric exit criterion.
  - *Given* the flip PR description, *When* read, *Then* it records at least 30 merged PRs or 4 weeks of report-only runs (whichever is later), a "would have failed" rate below 3% of those PRs with every false positive triaged, and the exempt count against its ceiling.
- Blocking via one switch.
  - *Given* `docsCoverageEnforce: true` in `.github/promo-flags.json`, *When* a PR adds `foo-panel` with no docs, *Then* the `build` job fails with the per-feature message; a one-line PR setting it back to `false` makes the same PR pass.
**Files**: `.github/promo-flags.json`, `make/promo.mk` (`ci` prerequisite), `CLAUDE.md` pointer

##### Task 4.1.5a: Enable and add to `make ci` (S)
- **Wall-clock observation window, not a wave**: the task starts only when the window below has elapsed. After the observation window (at least 30 merged PRs or 4 weeks, would-have-failed rate under 3% with each false positive triaged; record the actual numbers in the flip PR); flip the flag in `.github/promo-flags.json` by PR and add `docs-coverage` to `ci`; open the tracking issue the notify job references. Trace: R4, R17.

#### Story 4.1.6: Docs for contributors
**As a** contributor, **I want** a how-to for sidecars and exemptions. (R4)
**Acceptance Criteria**:
- Documented workflow.
  - *Given* `docs/how-to/add-docs-coverage-for-a-feature.md`, *When* followed, *Then* a new feature gets `help`+`website` or `exempt: <reason>` and the checker passes.
**Files**: `docs/how-to/add-docs-coverage-for-a-feature.md`, `docs/README.md` index, `CLAUDE.md` Reference Documents row

##### Task 4.1.6a: How-to and index rows (XS)
- Include: the many-to-one rule, the `exempt` policy, the percent-encoded sidecar filename rule, and a worked example for a sync-generated website page (edit `docs/how-to/x.md`, run `node website/scripts/sync-content.mjs`, point `website` at the generated id).

#### Story 4.1.7: Link tour-covered features
**As a** reader, **I want** registry features covered by tours/help linked, **so that** coverage numbers reflect reality. (R4, R3)
**Acceptance Criteria**:
- Tour features resolve.
  - *Given* the 8 tours' `tour.features`, *When* `docscoverage` runs, *Then* every listed id has a sidecar with `tour: <id>` and `website: tours/<id>` and no mismatch is reported.
**Files**: `docs/registry/docs-coverage/<percent-encoded-id>.json` (those ids)

##### Task 4.1.7a: Author sidecars for tour-covered ids (S)
- Remove `grandfathered` from those ids; hand-authored (not generated), after the Phase 3 tours exist; backfill beyond this is reported, not required. Add the bidirectional-check test over the real 8 tours (`TestAllTourFeaturesHaveSidecars`). Phase 3 tour stories do not depend on this task. Trace: R4, R3.

---

# Phase 5: Cleanup and enforcement

## Epic 5.1: Remove stale media, flip freshness

**Goal**: The repo holds no promo binaries and staleness is a hard failure. (R1, R5, R20)

#### Story 5.1.1: Delete stale committed media
**As a** maintainer, **I want** the old binaries removed once the pipeline is green, **so that** nothing stale can be served. (R1, R20)
**Acceptance Criteria**:
- Files gone, nothing references them.
  - *Given* a green full `deploy-website.yml` run on main, *When* the PR deletes `website/public/features/*.png`, `website/public/demo.webm`, `website/public/screenshot.png`, `assets/demo.gif`, `assets/demo.webm`, `assets/screenshot.png`, `assets/demo-recording/*`, *Then* `git grep` finds no references, `mediaguard` passes (deletions allowed), and `bun run build` succeeds.
- Legacy demo tooling removed so nothing regenerates the binaries.
  - *Given* the repo, *When* `grep -n 'demo-video\|demo-post-process\|demo-gif\|assets/demo' Makefile` runs, *Then* zero matches (targets at `Makefile:1293-1308` and their `.PHONY` entries removed), and `scripts/demo-post-process.sh`, `tests/demo/demo_test.go` (`TestRecordDemo`), `tests/e2e/demo.spec.ts` and `.gitignore` lines for `assets/demo.*` are deleted or repointed; `tests/demo/helpers.go` and `tests/demo/seed` stay (still used by seeding); `docs/ci/baseline-time-to-green.md` mentions are updated; `make lint` and `go build ./tests/demo/...` pass.
- Rollback is documented.
  - *Given* the Phase 5 PR description, *When* read, *Then* it says a revert needs the `allow-media` label, and that after this PR the stale fallback no longer exists so Pages rollback = redeploy the previous commit.
**Files**: the deleted paths, `website/scripts/build-features.mjs`, `docs/features.md`, `Makefile`, `scripts/demo-post-process.sh`, `tests/demo/demo_test.go`, `tests/e2e/demo.spec.ts`, `.gitignore`, `docs/ci/baseline-time-to-green.md`

##### Task 5.1.1a: Delete and de-reference (S)
- Depends on 2.4.1, 3.3.1, 3.3.2 merged and one deploy verified. Trace: R1, R20.
##### Task 5.1.1b: Remove legacy demo-video tooling (S)
- Delete the Makefile `assets/demo.gif`, `assets/demo.webm`, `demo-post-process`, `demo-gif`, `demo-video` targets and their `.PHONY` names, the post-process script, `tests/demo/demo_test.go` and `tests/e2e/demo.spec.ts`; keep the shared seeding helpers. Trace: R1, R20.

#### Story 5.1.2: Flip freshness to blocking and remove fallback
**As a** maintainer, **I want** a missing/stale clip to fail the deploy. (R5, R17)
**Acceptance Criteria**:
- The flip has a soak criterion (pre-mortem P1-3 step 1).
  - *Given* the flip PR description, *When* read, *Then* it records 10 consecutive green **full** `deploy-website.yml` runs (a *full run* = a record in full mode on `schedule`, `workflow_dispatch` or an app-affecting push, `mediatool fresh` strict `commit == GITHUB_SHA`; content-only reuse runs and LKG deploys neither count nor reset the streak; a red full run resets it), including at least 2 weekly cron runs, with `mediatool fresh` clean in report-only mode, the per-clip `attempts`/`flaky` rows from those runs, and a link to each run; a single green run is not sufficient.
- Fail-loud confirmed.
  - *Given* `freshnessEnforce: true` in `.github/promo-flags.json` and a Scene deliberately broken on a test branch via dispatch, *When* `deploy-website.yml` runs, *Then* it fails at `mediatool fresh`, `deploy` is skipped and the previous Pages deployment still serves.
- Old fallback step is gone.
  - *Given* `deploy-website.yml`, *When* grepped for `REMOVE IN 5.1.2`, *Then* zero matches (the step marked in Task 2.4.1b is deleted here and only here), and `workflow-yaml.test.ts` passes.
**Files**: `.github/workflows/deploy-website.yml`, `.github/promo-flags.json`, `website/tests/unit/workflow-yaml.test.ts`

##### Task 5.1.2a: Enable, delete the fallback step (S)
- **Wall-clock observation window, not a wave**: starts only after the soak below has elapsed. Flip the flag by PR after the 10-run soak is recorded and delete the `# REMOVE IN 5.1.2` step. Last-known-good (Task 2.4.1e) stays in force for scheduled runs. Trace: R5, R17.

#### Story 5.1.3: Contributor docs for tours and pipeline
**As a** contributor, **I want** a how-to for adding a tour. (R13)
**Acceptance Criteria**:
- Two-file rule documented.
  - *Given* `docs/how-to/add-a-website-tour.md`, *When* followed, *Then* adding `tours/<id>.ts` and `tours/<id>.mdx` yields a new tour page and media in CI with no other edit to scene indexes or listings (proved by `add-tour-fixture.test.ts`, Task 3.1.1g); registry sidecars for its features are a separate Story 4.1.7 change.
**Files**: `docs/how-to/add-a-website-tour.md`, `docs/reference/ci-job-budget.md` (row for new jobs), `CLAUDE.md` index

##### Task 5.1.3a: How-to, budget row, index (S)
- The how-to states the two-file rule (scene by glob, `.mdx` with Start-here order from `tour.order`) and that linking the tour's registry ids to docs coverage is a separate sidecar change under Story 4.1.7, not part of the tour PR. Trace: R13.

---

## Effort estimate

Basis: bands and weights INFERRED (`ESTIMATION.md`); recalibrate after the first implementer run. Token classes: XS 20k, S 55k, M 130k per task. Assumed mix per run: ~12% output (spikes ~8%), ~60% cache reads, rest fresh/cache-write; so 1 raw token is about 0.97 CU.

| Story | Tasks | Class mix | Raw tokens | Output share | CU | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.1.1 spikes | 7 | 2 XS, 4 S, 1 M | 390k | 8% | 378k | outcome sampling at pre-launch, launch, week 2, week 4 (Tyler) |
| 1.1.1 alias + migration | 4 | 4 S | 220k | 12% | 213k | none |
| 1.1.2 server legacy base | 2 | S, XS | 75k | 12% | 72k | none |
| 1.1.3 rename | 8 | 6 S, 2 XS | 370k | 12% | 358k | none |
| 1.1.4 trademark check | 6 | 2 M, 4 S | 480k | 12% | 465k | allow-list and banned-set review (Tyler) |
| 2.1.1 media guard | 3 | 3 S | 165k | 12% | 159k | none |
| 2.1.2 manifest + freshness | 4 | M, 3 S | 295k | 12% | 286k | none |
| 2.2.1 scenario framework | 2 | S, M | 185k | 12% | 179k | none |
| 2.2.2 scenarios | 4 | 3 M, S | 445k | 12% | 431k | none |
| 2.3.1 recorder | 7 | 3 M, 4 S | 610k | 12% | 591k | none |
| 2.4.1 workflow | 8 | 3 M, 5 S | 665k | 12% | 644k | first CI run of matrix (Tyler/CI) |
| 3.1.1 schema + components | 9 | 3 M, 6 S | 720k | 12% | 698k | confirm ADR-001 reading (Tyler) |
| 3.1.2 feature-media refactor | 1 | M | 130k | 12% | 126k | none |
| 3.1.3 site gates | 1 | M | 130k | 12% | 126k | none |
| 3.2.1-3.2.4 eight tours | 8 | 8 M | 1,040k | 12% | 1,009k | owner review of storylines, seed data, imagery (Tyler) |
| 3.2.5 single shots | 2 | M, S | 185k | 12% | 179k | none |
| 3.2.6 mobile set | 1 | S | 55k | 12% | 53k | none |
| 3.3.1 hero/grid | 1 | S | 55k | 12% | 53k | none |
| 3.3.2 README | 3 | 2 S, XS | 130k | 12% | 126k | none |
| 3.3.3 new cards | 1 | S | 55k | 12% | 53k | wording review (Tyler) |
| 4.1.1 schema + checker | 4 | 3 S, M | 295k | 12% | 286k | none |
| 4.1.2 changed detection | 1 | M | 130k | 12% | 126k | none |
| 4.1.3 baseline | 1 | S | 55k | 12% | 53k | none |
| 4.1.4 CI report-only + bypass trend | 2 | 2 S | 110k | 12% | 106k | none |
| 4.1.5 flip blocking | 1 | S | 55k | 12% | 53k | observation window of merged PRs (Tyler decides N) |
| 4.1.6 contributor docs | 1 | XS | 20k | 12% | 19k | none |
| 4.1.7 link tour features | 1 | S | 55k | 12% | 53k | none |
| 5.1.1 delete stale media | 2 | 2 S | 110k | 8% | 107k | first green main deploy + Pages dispatch (Tyler) |
| 5.1.2 flip freshness | 1 | S | 55k | 8% | 53k | dispatch run (Tyler) |
| 5.1.3 how-to docs | 1 | S | 55k | 12% | 53k | none |
| **Total (implementation)** | **97** | | **7,340k** | | **~7.11M CU** (rows sum to 7,108k) | |

- Verification multiplier x1.75 (spec-compliance and quality review per story, `6-verify` layers, CI reruns): **~12.4M CU** (7,108k x 1.75 = 12.44M; repair round 1 raised the task count 70 -> 77; Phase 4 repair raised it 77 -> 87 and raw 5,960k -> 6,715k; Triad round 1 raised it 87 -> 97 and raw 6,715k -> 7,340k; Triad round 2 added no tasks (97, 7,340k raw) and corrected the CU rounding; recounted with `grep -c '^##### Task'` and an `awk` sum of the table rows. Triad round 2 widened the scope of existing tasks (3.1.1a/d/f/h/i, 2.1.1a-c, 1.1.4b), which the x1.75 verification multiplier and the overrun checkpoint absorb; no row class changed). Size-weighted basis: XS 20k, S 55k, M 130k, x0.9695 for CU. Size band: Large (3-15M), consistent with the request (informational, not a gate).
- Planning overhead already spent is excluded.
- **Critical path in agent waves**: W0 spikes (1) -> W1 fan-out of 1.1.1a, 1.1.4a0 then 1.1.4a1 then 1.1.4a, 2.1.2, 4.1.1a (4 parallel; 2.1.1 starts after 1.1.4a1; 1.1.1b-d wait for 1.1.3a in W2) -> W2 1.1.2 + 1.1.3 + 2.2.1 + 3.1.1 (parallel) -> W3 2.3.1 (needs 1.1.3, 2.1.2) + 2.2.2 (3 parallel runs by scenario) + 3.1.2 + 4.1.1d then 4.1.1b-c, 4.1.2 -> W4 2.4.1 + 3.1.3 + 4.1.3-4.1.4 -> W5 eight tours in parallel (one worker per tour; file-disjoint) + 3.2.5 + 3.2.6 -> W6 3.3.x + 4.1.7 -> (first green full deploy) -> W7 5.1.1 and 5.1.3 (5.1.2a and 4.1.5a are **not** wave work, see below). About 8 serial waves. **Wall-clock observation windows, not waves**: Story 5.1.2 (10 consecutive green full runs including 2 weekly cron runs, at least about 2 weeks after the first green full deploy) and Story 4.1.5 (at least 30 merged PRs or 4 weeks of report-only runs) each start only when their observation criterion is met; the agent task in each is a one-line flag-flip PR plus deleting the fallback step (5.1.2a), and the elapsed time is waiting, not agent work, so neither counts toward the wave estimate. Serialization points: `deploy-website.yml`, `astro.config.mjs`, `make/promo.mk`.
- **Overrun checkpoint**: if spend exceeds the table by more than 25% for completed stories (for example more than ~1.25 x 1.75 x the summed CU of finished stories), pause and report the delta and its cause. Informational only.
- **Wall-clock blockers**: (1) owner review of tour storylines, seeded narrative, imagery and card wording — Tyler; (2) first CI run of the expanded recording matrix on GitHub runners (queue time, ffmpeg/Chromium behaviour) — CI; (3) Pages deploy after merge and one `deploy-website.yml` dispatch (per PR #1051 follow-up note) before Stories 5.1.1-5.1.2 can be verified — Tyler; (4) docs-gate observation window before flipping to blocking (Story 4.1.5) — Tyler; (5) confirm ADR-001 reading before 3.1.1 — Tyler; (6) the Story 5.1.2 freshness soak: 10 consecutive green *full* runs including at least 2 weekly cron runs, so at least about 2 weeks of wall clock after the first green full deploy (reuse runs and LKG deploys do not count) — CI/Tyler; (7) outcome-metric sampling at pre-launch, launch, week 2 and week 4 (the 14-day traffic window needs the week-2 sample), recorded by Task 0.1.1g — Tyler; (8) branch-protection check for required CODEOWNERS review **and the guard job id as a required status check on `main`** (Task 2.1.1c; main is currently unprotected with no required checks, `docs/reference/ci-job-budget.md:29`), an owner action if either is off — Tyler; plus the ADR-003 owner decision on a second reviewer vs admin override.

---

## Plan repair log

Round 1 repair of `architecture-review.md` (AR) and `adversarial-review.md` (AD). Findings are numbered as in those files (AR-B = blocker, AR-C = concern, AD-B / AD-C / AD-M). Verified by command during repair: `tests/demo/` has no `tools/` ancestor; root `go.mod` already replaces go-git with the `tstapler` fork; `Makefile:946-950` runs `norawgitcli`; `session/git/util.go:34` defines `OpenRepo`; registry counts are 349 backend / 148 frontend / 13 root files (15 root entries, 163 default-userFacing ids, 1 duplicate id `launcher_presets:get`, 322 colon ids), which confirms ADR-005's original file counts and refutes the review's "102/87" figures (those counted top-level files only); `git grep -il wh40k` (excluding `CHANGELOG.md`, `project_plans`) = 34 files.

| Finding | Change |
|---|---|
| AR-B1 (internal/ visibility) | Cross-cutting 1: shared code moved to non-internal `tools/promolib/{gitbase,termscan}`; Files lists of 1.1.4a/c, 2.1.1a, 2.2.2d, 4.1.1, 4.1.2 updated; ADR-002/004/005 updated |
| AR-B2, AD-B2 (git CLI vs norawgitcli) | Cross-cutting 2: go-git via `session/git.OpenRepo`, root module, `-base` semantics for CI (merge commit `HEAD^1`) and local (merge-base); Unresolved Question closed; no new task needed because gitbase is part of 2.1.1a |
| AD-B1 (guard never runs on website/docs/README PRs) | Cross-cutting 3: `mediaguard` and `termcheck -mode=source` run in `pr-diff-guards` (`backlog-scaffolding-guard.yml`), AC + workflow-yaml test added in 1.1.4, 2.1.1; ADR-003/004 updated |
| AD-B3 (regex misses glued identifiers) | Task 1.1.4a and Story 1.1.4 ACs: substring rules for distinctive fragments, `\b` only for `40k`, cases `wh40kTheme`, `wh40kStart`, `wh40kBlock`-style, `isWH40K`, `Warhammer40K`, `WH-40K`, `wh_40k`; ADR-004 updated |
| AD-C6 (term list, path components) | Proposed banned set in 1.1.4a (owner review item added), skip by path component, dist mode adds title/OG/sitemap, scenario fixtures covered by leak test + source mode; OCR of imagery left as the documented owner check (deferred) |
| AR-C Specification, AD-C5 | Semantic-field predicate (ignores `tested`, `testIds`, `lastModified`, others by default), grandfathered-on-change passes, id-keyed identity: Story 4.1.2, Task 4.1.2a, ADR-005 |
| AR-C registry shape, AD-C7 | Task 4.1.1d + Story 4.1.1 ACs: array files, duplicate ids, percent-encoded sidecar filenames with inner `id`, orphan sidecars fail; counts corrected and G (gap count) measured not assumed (0.1.1f, 4.1.3) |
| AD-C8 (two doc universes) | Story 4.1.1 ACs: many-to-one allowed, exempt policed with ceiling, Starlight-compatible slugs, sync-generated page attribution; worked example in 4.1.6a |
| AR-C orphan sidecars | Story 4.1.1 AC |
| AR-C fallback consistency, AD-C11d | 2.4.1b keeps old step marked `# REMOVE IN 5.1.2` and adds `mediatool fresh` report-only beside it; 5.1.2a removes it; Risk Control and ADR-003 aligned; AC added in 5.1.2 |
| AD-C1 (1.1.1 vs 1.1.3 ordering) | Migration Plan ordering constraint; 1.1.1b/c run after 1.1.3a in one PR, three commits; dependency graph and ADR-004 updated |
| AD-C2 (existing website tests) | `workflow-yaml.test.ts`, `check-asset-health` unit/integration tests, `release-asset(.ts/.test.ts)`, `landing.spec.ts`, `hero-video.test.ts`, `features-*` tests, `lighthouserc.test.ts` added to Files and ACs of 2.4.1, 3.1.1, 3.1.2, 3.1.3, 3.3.1 |
| AD-C3 (clock vs seed timestamps) | Shared `ref` instant passed to seeder and `setFixedTime`; AC on rendered relative time; server-side windows noted as unfrozen (2.2.1a, 2.2.2, 2.3.1d) |
| AD-C4 (seeding fallback, `/Users/`) | Task 0.1.1f and Unresolved Question: route-fixture fallback per item, drop-tour decision by owner; `/Users/` added to `LeakPatterns`, default-scenario paths handled |
| AR-C Scenario lifecycle | `FileSeeder` / `RPCSeeder` split, `seed rpc` step after server readiness (2.2.1a, Pattern Decisions) |
| AR-C mediafresh cohesion | One `mediatool` binary with `fresh`, `shard-plan`, `summary` (2.1.2) |
| AR-C theme id duplication | Parity test (Story 1.1.1, Migration Plan) |
| AR-C rename blast radius, AD-M2 | Exact 34-file list partitioned into 1.1.3a, a2, a3, b, c, c2, d, e (+1.1.2); `next-e2e-baseline.json` regenerated not codemodded; only `visual-wh40k` snapshot dir renamed |
| AR-C Makefile seam | `make/promo.mk` owns its `.PHONY`; `make help` checked (1.1.4b, serialization list) |
| AR-C DOC_FILES parse guard | Shape test also as a web-app jest test; `detect-changes` selects the gate for `web-app/src/lib/docs/**` (4.1.1b, 4.1.4a) |
| AD-C9 (PR smoke cost, Lighthouse) | Smoke builds its own binary from the PR (a main-built artifact expires after 1 day, found in Phase 6), runtime targets and cap, tour pages excluded from PR Lighthouse/axe, produced-later link allow-list (2.4.1 ACs, 2.4.1c, 3.1.3) |
| AD-C10 (legacy tooling, concurrency) | Task 5.1.1b removes Makefile demo targets, post-process script, `demo_test.go`, `demo.spec.ts`; `count-pr-checks.py` before/after and 2 shards default in 2.4.1; budget doc row |
| AD-C11 (rollback) | `allow-media` label override for revert, flags in checked-in `.github/promo-flags.json` with 14-day reminder, post-5.1.1 rollback = Pages redeploy, rename rollback note (Risk Control, ADR-003) |
| AD-C12 (orphan media, 3.3.3 gating) | 3.2.5b's 11 extra view shots dropped; expected-vs-referenced equality test; card count no longer frozen at 19 pending 3.3.3 |
| AD-M1 | Playwright 1.56.1 sub-question struck |
| AD-M3 | `.mjs` justification in 2.1.2c and ADR-002 |
| AD-M4 | `coverage-gaps.json` reconciliation in 4.1.1a |
| AD-M6 | `lfs: true` dropped in 2.4.1a |
| AD-M7 | README link derived from `SITE_ORIGIN`/`SITE_BASE`, unit test (3.3.2) |
| AR nitpick (`waitForTimeout` count) | 9 stated in 2.3.1 AC |

**Counts after repair** (recounted with `grep -c`): 6 phases, 11 epics, 33 stories, 77 tasks (was 70: added 0.1.1f, 1.1.3a2, 1.1.3a3, 1.1.3c2, 4.1.1d, 5.1.1b); effort ~5.96M raw / ~5.78M CU, ~10.1M with the verification multiplier.

**Round 2 repair** (architecture-review and adversarial-review "Re-review" sections):

| Finding | Change |
|---|---|
| AR gochecknoglobals (A1) | Cross-cutting 5a; `BannedTermSet()` / `LeakPatterns()` are functions; ACs in 1.1.4a, Story 1.1.4, 1.1.4c, 2.2.2d, glossary |
| AR merge-commit assumption (A2) | Cross-cutting 5b; `gitbase` fails loudly with <2 parents on default `-base HEAD^1`; test and AC in 2.1.1a / Story 2.1.1 |
| AR workflow cost, AD-N3 (A3) | New Task 1.1.4b0 (measurement: `fetch-depth: 2`, `setup-go` cache, prebuilt single binary, runtime target, `timeout-minutes`); 1.1.4b and 2.1.1b reference it; workflow-yaml test asserts it |
| AR heavy dependency edge (A4) | Cross-cutting 5e: `session/git` import is build-time only |
| AD-N1 (ordering) | Dependency graph: 1.1.4b after 1.1.3a-e (or same PR); alias module allow-listed in 1.1.4a |
| AD-N2 (label re-trigger) | `types: [opened, synchronize, reopened, labeled, unlabeled]` in 2.1.1b, AC and workflow-yaml test in Story 2.1.1 |
| AD-N4 (job id) | Cross-cutting 5c: job id `guard`, display name `pr-diff-guards`; ACs/tests use `guard` |
| Registry counts | Cross-cutting 5f, 0.1.1f, 4.1.3: 349 backend / 148 frontend / 13 root = 510 files (re-derived with `find`) |

Task count now 77 (added 1.1.4b0).

**Deliberately deferred**: AD-C6(c) OCR or keyframe check of imagery (superseded in Phase 4: OCR now gates text in shots, Task 2.4.1f; imagery stays an owner review item per ADR-004); AD-C11b automated tooling for post-release rename rollback (documented forward-fix only); AD-M5 separate CI-rerun allowance in the estimate (no calibration data, the overrun checkpoint covers it); AD-M "weak traceability" for R12 LCP, R16 notify AC beyond the flag-age AC, and R19 caption/transcript equality (low risk, revisit at validation phase 4); AR nitpicks on `helpDoc: configuration` slug (resolved in Phase 4, Cross-cutting 6a) and ADR wording of `clips.ts` as index (resolved: glob discovery, Cross-cutting 6f).

## Plan repair log - Phase 4

Inputs: `implementation/pre-mortem.md` (PM), `implementation/consistency-check.md` (CC), `implementation/validation.md` (V). Counts below were recomputed by command after the edits: `grep -c '^##### Task'` = 87, `grep -c '^#### Story'` = 33, `grep -c '^## Epic'` = 11, `grep -c '^# Phase'` = 6; effort 6,715k raw, ~6.51M CU, ~11.4M with the x1.75 multiplier (size-weighted sum per story; the table was re-derived from the same script).

| Finding | Change |
|---|---|
| PM P1-1 (guard job slow/flaky) | Task 1.1.4b0 is a hard gate: warm p95 above 5 min or a cold run over the timeout means a separate always-reporting `promo-guards` job with in-job early exit, not an extended `guard`; checkout-free `gh api` variant for `mediaguard` evaluated in the same measurement; `guardsEnforce` kill switch in `.github/promo-flags.json` (Cross-cutting 4, 1.1.4b, 2.1.2) |
| PM P1-2 (theme rename) | New Task 1.1.1d and Story 1.1.1 ACs: boot script cases `wh40k`/`grimdark`/`custom:x`/unknown on the real built script; server end-to-end base normalization incl. echoing endpoints (1.1.2a); alias kept two releases with removal date and tracking issue (Migration Plan); recorder theme assertion fails the shard regardless of flags (2.3.1 AC) |
| PM P1-3 (recording flakiness) | New Task 2.3.1f (Scene retry, `attempts` in manifest); new Task 2.4.1e (content-only reuse of last good media; last-known-good deploy on scheduled runs up to 28 days, then hard fail); Story 5.1.2 requires 10 consecutive green full runs incl. 2 weekly cron runs before the freshness flip |
| PM P1-4 (docs gate false positives) | Story 4.1.2: `path`/`component`-only changes non-semantic, only new ids and `name`/`description`/`userFacing` changes count; unresolved base (dispatch, merge queue, zero `before`, single parent) is report-only and exits 0 (Task 4.1.2a); Story 4.1.5 numeric window (30 merged PRs or 4 weeks, under 3% would-have-failed, triaged); exempt needs an `approver`, CODEOWNERS entry and a +5 per-PR cap (Story 4.1.1) |
| PM P1-5 (trademark check) | `\b40k` scoped to published paths with a benign-string golden test (Story 1.1.4, 1.1.4a); `media-text` blocks upload unconditionally; OCR step over shots and CODEOWNERS sign-off on theme files (new Task 2.4.1f); imagery (logos/art) remains an owner review item |
| PM P2-1, P2-2 | Total `dist/` ceiling (40 MB) and record wall-clock ceiling (30 min) in `budgets.json` (2.1.2a); content-only reuse (2.4.1e) |
| PM P2-3, P2-4 | Scenes assert quoted step strings; route fixtures validated against proto types, at most 2 per tour (2.3.1f) |
| PM P2-5 | `allow-media` honoured only when applied by a CODEOWNERS member; byte total printed; use recorded (2.1.1a, Story 2.1.1) |
| PM P3 items | Not planned beyond existing notes: Safari WebM stays an open question; runner pin and `ffmpegVersion` already in manifest; README link added to a link check (Task 3.3.2c); Windows checkout of `%`-named sidecars and a single coverage percentage are **deferred** (report shows covered/exempt/grandfathered counts, no percentage: Task 4.1.1c) |
| CC B1 (helpDoc 404) | `helpDoc` is a `WebsiteDocID` (Cross-cutting 6a, Glossary); new Task 3.1.1e validates it against the docs collection at build; Story 3.2.1 AC now uses `help/getting-started` for `approval-review` |
| CC C1 (allow-list) | Task 1.1.4a enumerates every file that legitimately contains the legacy id plus a test that the list equals the actual set |
| CC C2 (push base) | Task 4.1.4a passes an explicit `-base` per event (`HEAD^1` on PR, `github.event.before` on push, report-only otherwise) |
| CC C3 (Phase 3 vs 4) | Stories 3.2.2 and 3.2.4 ACs re-scoped to frontmatter/build checks; sidecar ACs live only in Story 4.1.7; dependency graph states it |
| CC C4, C5 (schema) | Nested `tour:` object adopted (Cross-cutting 6b) and extended with `audience`, `durationMin`, `flagNote`, `phoneFigure`, step `caption`/`why`/`quote` in Task 3.1.1a; ADR-001 updated |
| CC C6 (PR rendering) | Cross-cutting 6c media modes; Tasks 3.1.1f, 2.4.1c, Story 3.1.3 changed; the produced-later allow-list is removed; `design/ux.md` s3-s6 aligned |
| CC C7 | Tasks 3.1.1d, 3.1.1c, Story 3.1.1 ACs (UX-1/2/3/5/6/7, R12, R19) |
| CC C8 | Story 3.3.1 AC "See the tours" link, Story 3.3.2 Start-here links AC, Task 3.3.2c README link check, build stamp on the index footer (3.1.1f) |
| CC C9 | Start-here tours fixed: `dashboard-lifecycle`, `approval-review`, `organise-at-scale` (Cross-cutting 6e, 3.1.1b); `design/ux.md` updated |
| CC C10 | Scenes discovered by glob (Cross-cutting 6f, 2.3.1a); Start-here derived from `tour.order`; sidecars (Story 4.1.7) are a Phase 4 step, not part of the two-file rule (documented in Task 5.1.3a how-to scope below) |
| CC C11 | Effort table recomputed (87 tasks, 4 in Story 1.1.4) |
| CC nitpicks N1-N6, N8, N9 | N1 example fixed; N2/N3 glossary reworded; N4 labels and task order fixed, 4.1.1c Files added; N5 status updated; N6 wave plan updated; N8 requirements' re-enable claim superseded in ADR-003; N9 allow-list scope and the PR-to-`main`-only trigger noted in ADR-003. N7 (clip <= 60 s, Print, `<noscript>`) **deferred**: covered only by size budgets plus `tour-media-attrs.test.ts`; revisit in `6-verify` |
| V gap 1-3 (R12, R16, R19) | `tour-media-attrs.test.ts`, `tour-transcript.test.ts` (Story 3.1.1 ACs, Task 3.1.1c); notify `needs` test and induced-failure run (Story 2.4.1) |
| V gap 4-5 (UX-1/2/3/6/7, UX-4, UX-11) | Task 3.1.1d (`tours.spec.ts`, keyboard-nav, reduced-motion); Task 3.1.1f (smoke render, missing-manifest error) |
| V gap 6 (R6) | New Task 2.3.1g hermeticity check |
| V gap 7 (R13) | New Task 3.1.1g and Story 3.1.1 AC `add-tour-fixture.test.ts` |

New tasks (10): 1.1.1d, 2.3.1f, 2.3.1g, 2.4.1e, 2.4.1f, 3.1.1d, 3.1.1e, 3.1.1f, 3.1.1g, 3.3.2c. Tour-covered sidecars (Story 4.1.7) are the one edit beyond the two files per tour and are documented in Task 5.1.3a: add one line to its how-to scope ("covering registry ids with a tour is a separate sidecar PR under Story 4.1.7").

**Deferred**: Windows checkout handling of `%` filenames (P3-4); N7 UX details (60 s cap, print, `<noscript>`); VTT tracks; Safari WebM decision (owner); imagery review of logos/art (owner); actual p95 numbers and the `promo-guards` vs `guard` choice (decided by the 1.1.4b0 measurement at implementation time).

## Plan repair log - Triad round 1

Inputs: the Engineering, UX and Product lenses of `pm:triad-review` round 1 (engineering analysis: G1-G9c). Counts recomputed by command after the edits: `grep -c '^##### Task'` = 97, `grep -c '^#### Story'` = 33, `grep -c '^## Epic'` = 11, `grep -c '^# Phase'` = 6; effort 7,340k raw, ~7.12M CU, ~12.5M with the x1.75 multiplier (size-weighted sum per story, table rows regenerated by script). No downscaling or cut lines were introduced.

| Finding | Change |
|---|---|
| G1 reuse contradicts `commit == GITHUB_SHA` | Cross-cutting 7b: `mediatool fresh -reuse-from <commit>` (ancestor via compare API, no `recordInputPaths` change; path set listed and kept in `budgets.json`); new Task 2.1.2d with table tests; Story 2.1.2 reuse AC; Story 2.4.1 content-only AC rewritten; reuse runs are not "full runs" |
| G2 gate inputs from PR tree | Cross-cutting 7a: flags, allow-lists and CODEOWNERS read from the base blob via `gitbase.ReadBlob`; CODEOWNERS entries for `promo-flags.json`, `tools/promo/**`, `tools/promolib/**`, `tools/docscoverage/**`, allow-lists and the two workflows; job `permissions` (`contents`, `pull-requests`, `issues` read); CODEOWNERS teams cannot be resolved with `GITHUB_TOKEN` so team entries fail closed and user logins are required; branch-protection check; new Task 2.1.1c; Story 2.1.1 ACs; docscoverage reads its flag from base too |
| G3a/b/c dependency inversions | New Tasks 1.1.4a0 (`gitbase`, moved out of 2.1.1a) and 1.1.4a1 (`promo-flags.json` + unit test + `promoguard` binary, moved out of 2.1.2a); 1.1.4a, 1.1.4b0, 1.1.4b and 2.1.1a rewritten to depend on them; dependency graph and shared-file list fixed; `mediaguard` is a `promoguard` subcommand with a representative stub for the measurement |
| G4 LKG mechanism | Cross-cutting 7c and new Task 2.4.1g: artifact `promo-media-lkg`, `retention-days: 35`, newest successful deploy run lookup, `actions: read`, missing artifact hard-fails (test); `actions/cache` explicitly not used |
| G5 label re-runs | `if:` expression on every new step, test and ci-job-budget.md record (Story 2.1.1 AC, Task 2.1.1b) |
| G6 smoke vs PR scenes | Story 2.4.1 AC: smoke trigger paths named; app/scene-touching PRs build the binary or skip with a notice; asserted in `workflow-yaml.test.ts` |
| G7 BUG-116 residual | Task 2.3.1g: per-run `HOME` or "CI only" guard, header links BUG-116, test asserts it |
| G8 soak | Wall-clock blockers (6)-(8); "full run" defined in Story 5.1.2 and Cross-cutting 7b |
| G9a/b/c minor | `.gitignore` entries and test (Task 2.1.2c); OCR fixture, confidence rule and allow-list (new Task 2.4.1h); cold cache plus `apt-get` in the 30-minute ceiling measurement (Tasks 2.1.2a, 2.4.1a, 1.1.4b0) |
| UX: smoke-mode assertions, strip with <3 tours, prev/next skipping | Story 3.1.1 AC and new Task 3.1.1i; `tour.order` as the single order (Cross-cutting 7f) |
| UX: video encoding | Decided: WebM + H.264 MP4 `<source>` order, poster, both files required by freshness; new Task 3.1.1h with tests; Unresolved Question closed |
| UX: hero pause, transcript, CTAs, mobile, labels, disclosure | Task 3.1.1h (Pause control, WCAG 2.2.2); `narration` field and transcript AC (1.2.1/1.2.3); Install CTA after the clip; tap-to-open phone figures (Tasks 3.1.1a, 3.2.6a); "Watch M:SS / Read N min"; date first and sha de-emphasised; sample-data line; honest help label; accessible names for duplicate links |
| UX: contrast, screen reader, forced-colors/print | Token-pair contrast test (Task 3.1.1i); pixel contrast and manual screen-reader pass are owner checks in the 6-verify checklist; forced-colors and print tests explicitly **deferred to 6-verify** |
| Product gaps | `requirements.md`: problem/harm, why now, personas and jobs, tour-to-job map, outcome metrics with baselines, assumptions with validation signals, docs-gate non-goal; new Task 0.1.1g (analytics inventory, GitHub traffic/download baselines, named owner if unmeasured); new Task 4.1.4b (exempt/baseline trend and bypass monitoring) |

New tasks (10): 0.1.1g, 1.1.4a0, 1.1.4a1, 2.1.1c, 2.1.2d, 2.4.1g, 2.4.1h, 3.1.1h, 3.1.1i, 4.1.4b.

**Deferred**: forced-colors and print automated tests (to 6-verify, owner-run); pixel-level badge contrast and screen-reader pass (owner checks); adding website analytics (separate owner decision; outcome metrics are marked to-be-measured with GitHub traffic and release downloads as the zero-tracking proxy); `.vtt` tracks (unchanged); team-based CODEOWNERS resolution (needs an org-scoped token, out of scope).

## Plan repair log — Triad round 2

Inputs: the Engineering, UX and Product lenses of `pm:triad-review` round 2. Counts recomputed by command after the edits: `grep -c '^##### Task'` = 97, `grep -c '^#### Story'` = 33, `grep -c '^## Epic'` = 11, `grep -c '^# Phase'` = 6 (no task added: every gap was closed by editing, merging or stating a default); effort 7,340k raw (awk sum of the table rows), 7,108k CU (~7.11M, the earlier ~7.12M was a rounding slip), ~12.4M with the x1.75 multiplier. No downscaling or cut lines were introduced; simplifications chosen over new machinery are marked (S).

| Finding | Change |
|---|---|
| D1 allow-list test vs rename ordering | The "allow-list equals the real legacy-id file set" test moved from Task 1.1.4a to Task 1.1.4b (which already follows 1.1.3a-e); 1.1.4a keeps fixture-only tests; new Story 1.1.4 AC `TestAllowlistEqualsLegacyIdFileSet`; dependency graph and ADR-004 fixed |
| S1 bootstrap when a gate input is absent | Cross-cutting 8a: typed not-found maps to defaults (`guardsEnforce=true`, empty allow-lists, no `allow-media` authority), malformed = exit 2; PR A lands inputs and tools, PR B wires; tests in Tasks 1.1.4a1, 2.1.1a, 2.1.1c and Story ACs in 1.1.4 and 2.1.1 |
| D2 missing `Depends on` | Task 2.4.1e depends on 2.1.2d and 2.4.1g; Task 2.4.1f depends on 2.4.1h and 1.1.4c; dependency graph annotated |
| D3 fixtures for 2.1.2c | `expected-media.mjs` tests use `website/tests/unit/fixtures/expected-media/*`; the real-tree test is Task 3.2.5b |
| D6 4.1.5 is not a wave | Tasks 4.1.5a and 5.1.2a labelled wall-clock observation windows; critical-path text and blockers updated (S: relabel only) |
| S2 CODEOWNERS and branch protection | Chose the simpler option (S): a blocking precondition (Cross-cutting 8c, Task 2.1.1c, 1.1.4b and 2.1.1b depend on it) instead of building the guard from the base ref; residual risk in ADR-003 |
| S3 `allow-media` rule | Cross-cutting 8d: latest `labeled` event, actor type `User` listed as an individual user in base CODEOWNERS, bots never count; Story 2.1.1 ACs, Task 2.1.1a, ADR-003 |
| S4 `pull_request_target` | Cross-cutting 8e and Story 2.1.1 AC; asserted in `workflow-yaml.test.ts` (Tasks 1.1.4b, 2.1.1c) |
| `.github/CODEOWNERS` absent | Task 2.1.1c creates it (verified absent); shared-file list says "created in 2.1.1c" |
| `make/` absent | Task 1.1.4b and the Tech Debt row state that the directory, the file and the first `include` are created (verified: no `make/`, no `include` in `Makefile`) |
| Guard id conditional on 1.1.4b0 | Cross-cutting 8b plus edits to Cross-cutting 3 and 5c, Story 1.1.4 and 2.1.1 ACs, Tasks 1.1.4b, 2.1.1b, 2.1.1c, ADR-003, ADR-004 |
| `budgets.json` ceilings and R7 | New Story 2.1.2 AC for per-asset, total `dist/` (40 MB), record wall-clock (30 min) and clip-duration (60 s) budgets; `validation.md` R7 row names the four tests |
| 2.4.1b file list | Named: `deploy-website.yml`, `workflow-yaml.test.ts`, `.github/promo-flags.json` (read), `tools/promo/budgets.json` (read), `tools/promo/cmd/mediatool/fresh.go` (invoked) |
| B1 build stamp | One format everywhere (Cross-cutting 8g): "Recorded 2026-11-10 (build <small>a1b2c3d</small>)"; Story 3.1.1 AC, UX-11, UX-16, ux.md s4.8, `tour-build-stamp.test.ts`, `validation.md` UX-11 |
| B2 Install link in DOM and tab order | UX-2, UX-7, ux.md s8 and s9, plan DOM-order and keyboard ACs, `validation.md` UX-7: Install link directly after the clip |
| G1 video fallback claim | Narrowed: inner fallback appears only when no `<source>` type is supported; runtime failure leaves the native error state; no error handler (S); ux.md s4, plan UX-3 AC |
| G2 owning components | Six components plus `BuildStamp` with a Story 3.1.1 ownership note (`TourHeader`, `TourClip`, `TourSteps`, `TourShot`, `TourFooter`, `TourGrid`); ux.md s4; ADR-001 |
| G3 WCAG 2.2 | `scroll-padding-top` and a not-obscured test (2.4.11), visible text equals accessible name (2.5.3, Cross-cutting 8f, prev/next/See the tour lose their `aria-label`: S), text-spacing test (1.4.12), 1.2.2 N/A (clips silent, `-an`); UX-19 |
| G4 hero edge cases | JS disabled, autoplay blocked (button reads Play), reduced motion (native controls only, no button): Story 3.1.1 AC, Task 3.1.1h, ux.md s5 and s6, UX-15 |
| G5 PR axe on a tour page | axe on `/tours/dashboard-lifecycle/` in PR runs; Lighthouse stays non-tour plus index (Stories 2.4.1 and 3.1.3, Task 2.4.1c) |
| G6 smoke-mode links | Unrecorded tours are not built in smoke (loader glob), so sidebar, grid, prev/next, search and landing card links exclude them by construction (S: deletes the "page without clip" state); README link check skips unbuilt tours with a notice; Watch label omitted without a manifest duration; smoke clip is `dashboard-lifecycle` |
| G7 runtime image failure | Figures keep reserved dimensions and show alt (Story 3.1.1 UX-3 AC, ux.md s4 states, Task 3.1.1a) |
| G8 copy vs 60 s cap | "about a minute", "Watch 0:45"; 3-6 step Zod test and `TestFresh_ClipOver60sFails` (Cross-cutting 8h) |
| G9 transcript test | Asserts `.mdx` narration plus transcript-JSON captions (Story 3.1.1 AC) |
| G10 phone figures | Four tours named (`dashboard-lifecycle`, `organise-at-scale`, `approval-review`, `backlog-pipeline`), four have none; Story 3.2.6 AC, ux.md s7 and s11 |
| G11 reduced-motion CSS | `0s` transition/animation durations asserted under `reduce` (Story 3.1.1 AC, Task 3.1.1d, UX-20) |
| G12 tour content quality | Epic 3.2 ACs: narration, alt length check plus owner quality check, figcaption marker text, `flagNote` iff the scene lists `flagsEnabled`; schema gains optional `annotated` (ADR-001) |
| Product gaps (requirements.md) | Harm labelled a hypothesis with a validation signal and G pending 0.1.1f; persona and 2-minute threshold labelled an owner assumption (proxy or unmeasured); sampling schedule pre-launch/launch/week 2/week 4 recorded in `spike-notes.md` (Task 0.1.1g) with the 14-day API limit and the attribution limit; N = 8; resolved Open Questions pruned; three risky assumptions added (docs-gate friction, weekly recording stability with a 9-of-10 threshold after the soak, tour set vs evaluator wants); displacement line (UNVERIFIED: Tyler to confirm no competing roadmap item) |

**Deferred**: whether `validation.md` needs a per-test row for UX-19/UX-20 beyond the updated UX-7/UX-11/R7 rows (the plan ACs name the tests; the matrix is refreshed at the next validate phase); forced-colors and print automated tests (unchanged: 6-verify, owner-run); `.vtt` tracks; the actual `guard` vs `promo-guards` choice (decided by the 1.1.4b0 measurement at implementation time); confirmation of the displacement line.

## Plan repair log — Triad round 3

| Finding | Repair |
|---|---|
| E1 2.1.1c permissions block depends on a later-decided job id | Task 2.1.1c now depends on 1.1.4b0 (which does not depend on it, no cycle); dependency graph updated; the task adds only the permissions block and its test |
| E2 "blocking" is advisory on an unprotected `main` | Task 2.1.1c owner action and wall-clock blocker (8) include the guard job id as a required status check (`docs/reference/ci-job-budget.md:29`: `main` unprotected, no required checks, no rulesets); a PR can edit the workflow itself, so only CODEOWNERS review protects it |
| E3 read-only guard job cannot comment | Story 2.1.1 `allow-media` AC: use recorded in the job summary (or deploy-side `notify`), not a PR comment |
| E4 solo-owner approval | ADR-003 documents admin override frequency and procedure; "second reviewer or accept admin override" listed as an OWNER DECISION (not decided) and added to blocker (8) |
| U1 smoke always has a tour | ux.md s3 states it; Task 3.1.1d asserts at least one built tour in smoke |
| U2 naturalWidth | Task 3.1.1d wording aligned to bounding-box plus alt-text |
| U3 link label | ux.md s2 and s5 use "See the tour: <title>" |
| U4 missing step screenshot | ux.md s4: build failure applies to deploy mode only; smoke omits the figure |
