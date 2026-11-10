# Spike notes: promo-materials-refresh (Phase 0, Story 0.1.1)

Run 2026-11-10 from the worktree on branch `stapler-squad-promi-mayerials`. Labels: **VERIFIED** = command run, output seen; **INFERRED** = read from code, not executed.

## Unresolved Questions answered here

### Q1. `docLoader` slug manifest without a Next build (Task 0.1.1a) - VERIFIED

`DOC_FILES` is a plain array literal (`web-app/src/lib/docs/docLoader.ts:10-22`). A Go program (regexp for `export const DOC_FILES = [...]`, then `go/parser.ParseExpr("[]string{...}")`) printed 11 slugs and found no missing or extra files in `web-app/public/docs/*.md`:

```
11 [what-is-stapler-squad session-types omnibar keyboard-shortcuts configuration tmux-integration backlog insights notifications tagging-rules workflows]
compare done
```

Command: `go run . web-app/src/lib/docs/docLoader.ts web-app/public/docs` (scratch program, not committed). The checker in Story 4.1.1b can take the same approach; it must fail with a clear message when the regexp stops matching (the "stops being a literal" test).

### Q2. Third-party Astro/Starlight plugin licence/compat (Task 0.1.1e) - VERIFIED (decision record)

No third-party Astro or Starlight plugin is adopted (ADR-002). Nothing to check now. If a story later proposes `starlight-links-validator`, `starlight-image-zoom` or lychee, WebSearch its licence and Astro 7 / Starlight 0.42 compatibility first and record it here.

### Q3. How `backlog` / `changelog` flags are enabled before recording (Task 0.1.1c) - VERIFIED

Use the `UpdateFeatureFlag` RPC after boot (`SessionService`, `proto/session/v1/session.proto:4123`; handler `server/services/feature_flag_service.go`). Flags live in `config.json` `feature_flags` (`config/config.go:408`), so a pre-boot config seed would also work, but the RPC needs no file layout knowledge. `backlog` defaults off (`feature_flag_service.go:241`), `changelog` defaults on (`:233`).

```
POST /api/session.v1.SessionService/UpdateFeatureFlag {"name":"backlog","enabled":true}
-> {"flag":{"name":"backlog","enabled":true,...}}
POST ... {"name":"changelog","enabled":true}
-> {"flag":{"name":"changelog","enabled":true,...}}
```

Connect handlers are mounted under `/api` (bare `/session.v1...` returns the SPA). Route guards: `FeatureGuard` in `web-app/src/app/FeatureGuard.tsx`, `GuardFlag = "backlog" | "changelog"` in `web-app/src/app/route-meta.ts:9`. Decision: scenarios enable flags through an `RPCSeeder` step.

### Q4. Is the selected theme stored server-side? (Task 0.1.1c) - VERIFIED

No. `git`-tracked Go has zero matches for `stapler-theme` (`grep -rn "stapler-theme" --include='*.go' .` printed nothing). The selection lives in `localStorage` key `stapler-theme` (`web-app/src/lib/contexts/ThemeContext.tsx:23`, read at boot by `web-app/src/styles/theme-boot-script.ts:14`). The server only serves user custom theme files via `GET /api/themes` (`server/user_themes.go`, registered at `server/server.go:1579`). Consequence for Story 1.1.1b: the alias migration is client-only; no server migration needed.

### Q5. Seeding feasibility in `--test-mode` (Task 0.1.1d) - VERIFIED for approvals, backlog, flags, terminal pane; PARTIAL for notifications; NOT TESTED for insights

Setup. Binary: `~/.stapler-squad/manual-builds/manual-3/stapler-squad` (existing build from 2026-11-01), **not** a fresh build: `make build` needs proto/ent/web generation (no `gen/`, `server/web/dist`, `session/ent/*.go` in this worktree) and the disk had 8.8 GB free with a 203 GB Go build cache, so a build was judged disk-risky. Consequence: behaviour reflects main as of 2026-11-01, not HEAD; re-confirm in Story 2.2.1 against the freshly built binary (the RPC names used here are in the current proto, checked by grep). Port: **62876** (spare block), not 62871, because another worker's live `manual-1` instance held 62871 and 62872. Isolation: `STAPLER_SQUAD_INSTANCE=promo-spike STAPLER_SQUAD_TMUX_SOCKET=ssq-spike STAPLER_SQUAD_USE_CONTROL_MODE=false ... --test-mode --test-dir <scratch> --tmux-keep-server`. The live `:8543` service was never touched. After the run the process was killed, `tmux -L ssq-spike kill-server` run (`no server running` afterwards) and the scratch dir removed.

Per item:

| Item | Mechanism | Result |
|---|---|---|
| Approval rule `git-push-ask` | `SessionService/UpsertApprovalRule` with `{"rule":{"id":"git-push-ask","name":"git-push-ask","toolName":"Bash","commandPattern":"^git push","decision":"AUTO_DECISION_ESCALATE","riskLevel":"medium","priority":50,"enabled":true,"source":"user"}}` | VERIFIED `created:true`; `ListApprovalRules` then returned it first alongside the `seed-*` rules; `GET /rules` returned 200. The UI-lists-it-after-load half is INFERRED (the page is the SPA over `ListApprovalRules`); no browser run. Implement as `RPCSeeder`. |
| Backlog item | `BacklogService/CreateBacklogItem` `{"title","description","repoPath"}` | VERIFIED: item created (status `idea`, `publicId` assigned), `ListBacklogItems` returned it. Needs the `backlog` flag on for UI. Status other than `idea` needs `TransitionBacklogItemStatus` (not tried). `RPCSeeder`. |
| Notification | `SessionService/SendNotification` needs a `sessionId` (else `invalid_argument: session_id is required`); accepted with an unknown id (`success:true`) | PARTIAL: send succeeds but `GetNotificationHistory` returned `{}` afterwards, so it behaves as a live push to connected clients, not a stored row (INFERRED reason, not traced). A scene must send it after the page is open, or use a real seeded session id. |
| Terminal pane with canned output | `SessionService/CreateSession` `{"title","path","sessionType":"SESSION_TYPE_DIRECTORY","program":"<path to show.sh>"}` where `show.sh` does `cat canned.txt; sleep 300` | VERIFIED: session reached `SESSION_STATUS_ACTIVE`; `tmux -L ssq-spike capture-pane -p` showed the three canned lines (one with an ANSI colour). `program` must be a single path: an inline `sh -c '...'` string is split and breaks (first attempt printed `unexpected EOF`, session went `STOPPED`). Implement as a script-file program; no `FileSeeder` needed. |
| Mock sessions without tmux | `tests/demo/helpers.go:152-155` (read) | INFERRED/read: seeded sessions are `Paused` and `Active` only, so the review-queue poller does not reclassify them when no tmux process exists. This is a file seed (`go run ./tests/demo/seed <dir> <n>`, as `scripts/record-website-demos.sh:50` does before boot). |
| Insights | not tried | UNVERIFIED. Question stays open for Story 2.2.2; fallback per plan (route fixture, `seeded: route`) applies if no RPC seed exists. |

Seeder split decided: `RPCSeeder` for flags, approval rules, backlog items, notifications and the canned-terminal session; `FileSeeder` only for the mock sessions (existing `tests/demo/seed`). Fallback (Task 0.1.1f): none needed for approvals/backlog/terminal; insights and notification-history rows fall back to route fixtures if their RPC path fails in 2.2.1.

### Q6. Repo facts (Task 0.1.1b) - VERIFIED unless marked

- `website/src/content.config.ts`: two collections, `docs: defineCollection({ loader: docsLoader(), schema: docsSchema() })` and `i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema() })`. A `tours/` directory under the docs collection needs `docsSchema({ extend: ... })`, matching the plan's reading of ADR-001. Story 3.1.1 owns the file.
- `website/lighthouserc.json` (desktop) and `website/lighthouserc.mobile.json` (mobile) each list two URLs: `http://localhost:4321/stapler-squad/` and `http://localhost:4321/stapler-squad/help/getting-started/`; 3 runs each; assertions error under 0.9 for performance, accessibility, best-practices. Tour pages are not covered until Story 3.1.3 adds a URL.
- `docs/reference/ci-job-budget.md`: every job of a triggered workflow is a check; concurrent-job cap Free 20 / Pro 40 / Team 60, tier unconfirmed (observed ceiling 40, design for 20); no required status checks. Branch `main` unprotected, `gh api repos/tstapler/stapler-squad/rulesets` returns `[]` (per the doc; re-verified below for protection).
- `uv run --with pyyaml python tools/ci/count-pr-checks.py` baseline: **20 checks on a code PR touching every filtered path (upper bound), 1 on a docs-only PR**. Per workflow: `backlog-scaffolding-guard` 1 (always), `build` 9 (paths-ignore), `bazel-cache-proxy-trial` 3, `benchmark` 7 (label-only), `e2e-video` 6 (label-only), one each for `css-differential`, `ghostty-wasm`, `goreleaser-check`, `lint`, `mcp-integration`, `ux-analysis`, `website-ci`.
- `tests/e2e/playwright.website-demos.config.ts` exists: `testMatch: 'website-demos.spec.ts'`, 1 worker, 0 retries, 240 s timeout, Desktop Chrome 1280x720; run only via `scripts/record-website-demos.sh`, which seeds and starts an isolated server on a free port with a private tmux socket. Convention lint scope: there is no single lint script. The conventions are checked by `tools/coverage/feature-coverage.ts` (scans every `*.spec.ts` under `tests/e2e/` for `// @feature`) and by a jest test limited to notification specs (`web-app/src/lib/__tests__/e2e_notification_conventions.test.ts`). The config file is not a spec so no scanner reads it; `website-demos.spec.ts` is scanned by `feature-coverage.ts` (INFERRED from the scanner's glob, not run).
- Run durations (`gh run list --repo tstapler/stapler-squad --workflow <wf> --limit 12 --json startedAt,updatedAt`; seconds, `updatedAt - startedAt`): `website-ci.yml` 200-321 s (12 PR runs, all success; median about 260 s); `deploy-website.yml` 537-1700 s (11 push + 1 schedule run, all success; median about 700 s); `build.yml` PR runs 2590-3800 s. Smoke caps: set website-ci smoke caps above 321 s plus headroom, record-job cap above 1700 s plus headroom; Task 2.4.1 derives exact numbers.
- Binary artifact: `build.yml` uploads `e2e-binary-bun` (contains `stapler-squad`, `seed-demo`, `seed-hidden`, `e2e-sshd`) with `retention-days: 1` (`build.yml:419-428`), built on `build.yml`'s path-filtered triggers. `main-ci.yml` uploads `stapler-squad-<goos>-<goarch>` on main pushes (`main-ci.yml:365-369`). So `website-ci` **cannot rely on a downloadable main binary** (1-day retention, path-filtered); Task 2.4.1c should build the binary itself with caches (or download the `main-ci` artifact with a fallback) and re-derive the 6-minute target. INFERRED from workflow YAML, not tried via `gh run download`.

### Q7. Registry and rename inventories (Task 0.1.1f) - VERIFIED

- `git grep -il wh40k -- . ':!CHANGELOG.md' ':!project_plans' | wc -l` = **9** files today (34 at planning time), by directory: `docs/how-to` 1, `server` 2, `tests/e2e` 1, `web-app/build/__tests__` 1, `web-app/src/lib/contexts` 1, `web-app/src/lib/omnibar/detectors` 1, `web-app/src/lib/theme` 2, `website/src/content/docs/help` 1. Task 1.1.3a can split per these directories (the 34 figure is stale).
- Registry (`find ... -name '*.json' | wc -l`): backend **349**, frontend **148**, root (`-maxdepth 1`) **13** files holding **15** entries, total **510** files and 512 entries (a Python scan; includes the one duplicate id `launcher_presets:get`). Frontend plus root entries: **163** ids, all unique, **20** containing `:` (matches the plan).
- Ids lacking docs: **not computable now**: the registry entries carry no docs field (keys: `alsoIn, backend, component, description, filePath, frontend, handlerFile, httpMethod, httpPath, id, lastModified, markerFound, markerLine, method, name, path, protoFile, rpc, service, testIds, tested, type`). The count arrives with the Story 4.1.1 schema and baseline (4.1.3); the unverified "87 / 80" figures must not be quoted.

### Q8. Analytics and outcome-metric baseline (Task 0.1.1g) - VERIFIED

- Analytics: `grep -rn "analytics|gtag|plausible|googletagmanager|umami|posthog|goatcounter|cloudflareinsights"` over `website/astro.config.mjs website/src website/public README.md .github/workflows/deploy-website.yml` found only README prose about the app's approval analytics (`README.md:33,59`). **The site has no analytics.** Pages build type is `workflow`, site `https://tstapler.github.io/stapler-squad/`. No tracking is added by this plan; outcome metrics are "to be measured", owner Tyler, from the GitHub numbers below.
- Branch protection: `gh api repos/tstapler/stapler-squad/branches/main/protection --jq .required_pull_request_reviews.require_code_owner_reviews` returned HTTP 404 `Branch not protected`, and `gh api repos/tstapler/stapler-squad/rules/branches/main --jq '[.[].type]'` returned `[]`. So `require_code_owner_reviews` is **not enabled**: a CODEOWNERS file alone enforces nothing today; Task 2.1.1c must either record that gap as an owner-gated settings change (Tyler enables protection) or rely on the Cross-cutting 8d label authority rule only. Owner-gated; not changed here.

## Guard wiring (Tasks 1.1.4b0, 1.1.4b, 2.1.1b, 2.1.1c) - 2026-11-10, after merging origin/main

**Guard job id decision: `promo-guards` (separate always-reporting job in `backlog-scaffolding-guard.yml`; `guard` is not extended).** Tests hold the id in one constant, `PROMO_GUARD_JOB_ID` in `website/tests/unit/workflow-yaml.test.ts`. Basis: INFERRED, not a CI measurement. `guard`'s own comment records a baseline of avg 4m / max 10.1m for a job that does two API calls and no checkout, so adding a checkout, Go setup and a build cannot be shown to stay under the 5-minute warm target; the plan's hard gate says not to extend `guard` in that case. Cost of the choice: one more check on every PR (see counts below). If the first real CI runs show `promo-guards` well under the budget, folding it back into `guard` is a one-line job merge the owner can decide.

Local measurements (VERIFIED, run on this Linux host at load average 35-58 because other sessions were running, so absolute seconds are inflated and noisy; fresh private `GOCACHE`, warm module cache, so module download is not included):

| Measure | Result |
|---|---|
| `go build -o` of `tools/promo/cmd/promoguard`, cold build cache | 26.5 s (cache grew to 313 MB) |
| same, warm build cache | 1.7 s |
| `go build -o` of `tools/docscoverage/cmd/docscoverage`, warm cache | 1.0 s |
| `go list -deps` package count, promoguard / docscoverage | 602 / 602 (396 non-stdlib) |
| promoguard binary size | 26.6 MB |
| `promoguard flags -base HEAD^1` | under 0.1 s |
| `promoguard termcheck -mode=source -base HEAD^1` over 14100 tracked files (depth-2 clone) | 13.2 s warm-disk, 35.4 s first run |
| `promoguard mediaguard -base HEAD^1` | 1.2 s (depth-2 clone), 2.0 s (full worktree) |
| `git clone --depth 2` of this branch (local `file://`), `.git` size | 94 MB, 3 commits |

Depth-2 behaviour (answers the 1.1.4b0 and spike 0.1.1d question, VERIFIED): in that clone `git rev-parse HEAD^1` resolves to the pre-merge tip, `gitbase.Open` succeeds (via `session/git.OpenRepo`), `HEAD^1` resolves, and the tree diff, base-blob read and `flags` output all work (`{"freshnessEnforce":false,"docsCoverageEnforce":false,"guardsEnforce":true}`).

UNMEASURED (CI-only, no numbers invented): `actions/checkout` at depth 2 over the network, `setup-go-cached` cache restore, runner speed, warm and cold p95 of the whole job, and the timeout headroom. `timeout-minutes: 12` on `promo-guards` is a placeholder bounding a cold build plus `guard`'s observed 10.1 m max; replace it with 3x the first measured warm run. Follow-up when the first PR runs: record the job's checkout, setup-go and build seconds here. The checkout-free alternative (API-only `mediaguard`) was not taken: `mediaguard` already reads the base blob through `gitbase`, and the saved clone is the minor cost.

Deviation from the plan text: the Go step uses the repo's `.github/actions/setup-go-cached` (restore-only cache, wraps `actions/setup-go` with its own cache off) instead of `actions/setup-go` with `cache: true`, matching every other Go job in the repo.

PR check counts (`uv run --with pyyaml python tools/ci/count-pr-checks.py`, merged tree): before 30 checks on a code PR touching every filtered path and 4 on a docs-only PR; after adding `promo-guards`, 31 and 5. The 20/1 figures in Q6 predate the `origin/main` merge (which added the Bazel workflows).

Bootstrap consequence (VERIFIED by running the tools against `origin/main` as base): `main` has none of the gate inputs yet (`promo-flags.json`, both allow-lists, `.github/CODEOWNERS` entries for the owner), so on THIS PR the guard reads the Cross-cutting 8a defaults. `promoguard termcheck -mode=source -base origin/main` reports the whole legacy-id file set and `promoguard mediaguard -base origin/main` reports every added binary, so `promo-guards` is expected to be red on this PR. Nobody holds `allow-media` authority in a base without CODEOWNERS, so the label cannot clear it either. Merging this PR is an owner decision (merge with the red check, since `main` is unprotected); every later PR sees the inputs in its base blob. The plan's PR A / PR B split would have avoided this; the owner chose to wire the guards in this one PR.

Fallout from the `origin/main` merge, fixed in the allow-lists: `tools/bzl/baseline/pr-sample.json` (recorded historical PR file paths, includes `website/src/styles/wh40k.css`) was added to the termcheck allow-list, and `tools/bzl/otel_overlay/otelc-build.tar.bin` (1,075,200 bytes, over the 512,000-byte ceiling) to the mediaguard allow-list.

### Branch protection (Task 2.1.1c precondition) - NOT MET, OWNER ACTION PENDING

```
$ gh api repos/tstapler/stapler-squad/branches/main/protection --jq .required_pull_request_reviews.require_code_owner_reviews
{"message":"Branch not protected", ... "status":"404"}
$ gh api repos/tstapler/stapler-squad/rules/branches/main --jq '[.[].type]'
[]
$ gh api repos/tstapler/stapler-squad/rulesets --jq length
0
```

The owner decided to wire the blocking guards in this PR anyway and this change does not touch repo settings. Until the owner (1) enables "require review from code owners" on `main` and (2) makes the `promo-guards` job a required status check, the guard checks are red-but-mergeable (advisory), and the CODEOWNERS entries protecting the guard code and its gate inputs are not enforced. PENDING OWNER ACTION; on a solo-owner repo the author cannot approve their own PR, so see ADR-003 "Solo-owner review and admin override".

## Outcome samples

The traffic API keeps 14 days, so readings must overlap. Attribution limit: releases, CI changes, bot traffic and unrelated promotion move the same numbers; a delta is evidence consistent with the tours working, not proof, and stays beside the "provisional" label. Owner of every sample: Tyler.

| Point | Date | Source command | Value | Who |
|---|---|---|---|---|
| pre-launch | 2026-11-10 (window 2026-10-27..2026-11-09) | `gh api repos/tstapler/stapler-squad/traffic/views` | views 588, uniques 149 (14 daily rows) | worker (0.1.1g) |
| pre-launch | 2026-11-10 | `gh api repos/tstapler/stapler-squad/traffic/popular/paths` | top: `/tstapler/stapler-squad/pulls` 166 views/1 unique (owner), repo overview 47 views/16 uniques | worker (0.1.1g) |
| pre-launch | 2026-11-10 | `gh api repos/tstapler/stapler-squad/traffic/popular/referrers` | `aocr.org` 118/118, `github.com` 15/5, `Google` 5/5, `pkg.go.dev` 3/3, `tstapler.github.io` 1/1 | worker (0.1.1g) |
| pre-launch | 2026-11-10 | `gh api repos/tstapler/stapler-squad/traffic/clones --jq '{count,uniques}'` | clones 40792, uniques 2259 (CI/bot dominated, weak signal) | worker (0.1.1g) |
| pre-launch | 2026-11-10 | `gh api repos/tstapler/stapler-squad/releases --jq '[.[].assets[].download_count]\|{assets:length,total:add}'` | 81 assets, 306 downloads total (192 of them `ghostty-vt.wasm`, a build input) | worker (0.1.1g) |
| pre-launch | 2026-11-10 | `gh api repos/tstapler/stapler-squad/releases --jq '[.[]\|select(.tag_name\|startswith("v"))\|.assets[].download_count]\|add'` | 88 downloads across 28 `v*` releases (v2.9.0: 1-2 per archive) | worker (0.1.1g) |
| launch | _fill when the first deploy serving a tour lands_ | same four views/paths/referrers/releases commands | | Tyler |
| week 2 | _launch + 14 days_ | same | | Tyler |
| week 4 | _launch + 28 days_ | same | | Tyler |

The pre-launch sample is taken before Story 3.3.1 merges. `traffic` requires push access (the account has it).
