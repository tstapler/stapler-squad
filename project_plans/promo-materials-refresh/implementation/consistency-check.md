# Cross-artifact consistency check: promo-materials-refresh

Date: 2026-11-10. Artifacts read: `requirements.md`, `implementation/plan.md` (993 lines), `design/ux.md`, `decisions/ADR-001..005`. `sdd/cross-artifact-consistency-prompt.md` does not exist; the standard checklist was used (terminology drift, requirement without story, story without requirement, plan/ADR/UX contradictions, stale counts, dangling references, UX criteria without a task).

Verified by command: `grep -c '^##### Task'` = 77 tasks, 33 stories, 11 epics, 6 phases; `git grep -il wh40k -- . ':!CHANGELOG.md' ':!project_plans'` = 34 files and the plan's task partition (1.1.3a 2, a2 5, a3 8, b 4, c 6, c2 5, e 1, plus 3 in Story 1.1.2) sums to exactly 34; `DOC_FILES` = 11 slugs; `waitForTimeout` in `tests/e2e/website-demos` = 9; `session-board-view` exists in `docs/registry/features/frontend/`; `.github/workflows/backlog-scaffolding-guard.yml` triggers on `pull_request` to `main` only.

Counts: BLOCKER 1, CONCERN 11, NITPICK 9.

## BLOCKER

**B1. `helpDoc` points at the wrong doc universe.** Story 3.2.1 AC: `approval-review.mdx` `helpDoc: configuration` "links to the existing help slug", and UX s4 item 6 makes it a website "Read the help page" link (`/help/<slug>/`). `configuration` is a `DOC_FILES` slug (in-app `web-app/public/docs/configuration.md`). The website `help/` collection (`website/src/content/docs/help/`) holds only synced `docs/how-to` pages (add-user-themes, getting-started, manage-notifications, ...) and has no `configuration` page, so the link is a 404 and fails the 3.1.3 link check. The plan never says whether `helpDoc` is a `DocSlug` or a `WebsiteDocID`, and no task validates `helpDoc` (3.1.1c checks alt, order, features, clip only; UX s4 "`helpDoc` target missing -> build fails" has no implementing task). Fix: define the type (newtype `WebsiteDocID`, since ADR-005 has both), validate it in the Zod schema or 3.1.3, and pick a real page for `approval-review`.

## CONCERN

**C1. Allow-list for the legacy id is incomplete.** ADR-004 s4 allow-lists the alias module, the server compat constant, their tests and CHANGELOG. Plan 1.1.4a seeds the allow-list with the alias module only, plus "any file 1.1.3 intentionally leaves". Files the plan itself requires to contain the legacy string: `server/user_themes.go` (+ test), `legacy-theme-ids.test.ts`, `ThemeContext.test.tsx`, `theme-boot.test.ts`, `theme-fouc.spec.ts` legacy case (1.1.1c), the `>theme <legacy>` case in `CommandDetector` tests (Story 1.1.3 AC), and `docs/how-to/add-user-themes.md` + its website copy (1.1.2b says to mention the legacy id). Story 1.1.3's "grep returns zero lines" AC and the blocking wiring in 1.1.4b would go red on these. Enumerate the allow-list in 1.1.4a.

**C2. Single-parent HEAD vs `docscoverage` on push.** Cross-cutting 5b / Story 2.1.1 make `gitbase` exit 2 when HEAD has one parent and `-base` is the default `HEAD^1`. Task 4.1.4a adds the docs-coverage step to `build.yml`, which also triggers on `push` to `main` (verified `on.push.branches: [main]`). A fast-forward or squash push has one parent, so the step would exit 2 on main. State which events run it (`pull_request` only) or give `push` a different base.

**C3. Phase 3 ACs depend on Phase 4 artifacts.** Story 3.2.2 ("the sidecar for that id lists `tour: organise-at-scale`") and 3.2.4 ("each resolves to a sidecar") need `docs/registry/docs-coverage/*.json`, which does not exist until 4.1.3 and 4.1.7a (and is hand-authored there, not generated). Re-scope those ACs to frontmatter validity, or move them to 4.1.7.

**C4. Tour frontmatter shape differs between ADR-001 and the plan.** ADR-001 nests everything under a required `tour` object (`title, outcome, clip, features[], helpDoc, order, category, steps[]`). Plan ACs use top-level keys (`clip:`, `helpDoc:`, `features:`). Pick one.

**C5. UX content has no schema field.** UX needs audience tag, "N min read", prerequisite/flag note ("enabled in this recording"), per-step "Why it matters", figcaption text, and a phone-framed figure reference per tour (UX s11 item 2). The ADR-001 schema and 3.1.1a list none of these. Add them or cut them from the UX doc.

**C6. PR-mode missing clip: UX and plan contradict.** UX s4/s6 and UX-4: in a PR smoke build the page renders without a clip block, no placeholder. Plan Story 3.1.3: in PR mode the link checker passes a `<video poster=missing>` through a "produced-later" allow-list, i.e. the video element stays. UX also says "poster missing -> build/link check fails". No task implements the clip-omitted rendering in `TourClip.astro` or a manifest-driven `TourGrid` that excludes unrecorded tours (UX s3 "Tour excluded").

**C7. UX acceptance criteria with no task or test file.** UX-1 (index structure), UX-2 (DOM order), UX-3 (media-blocked route test), UX-6 (44x44 targets, 320 px overflow on `/tours/`), UX-7 (focus order) name Playwright tests, but no Files list contains a tours Playwright spec. 3.1.3a covers only Lighthouse/axe, link check and a mobile-overflow check on one tour page. UX-5 relies on `reduced-motion.spec.ts`, which 3.1.1 never extends to tour pages. Add a task (e.g. `website/tests/e2e/tours.spec.ts`).

**C8. README / landing UX items missing from plan.** (a) UX s5 landing: hero secondary link "See the tours"; Story 3.3.1 and Task 3.3.1a cover the hero source, card links and count only (UX-12 requires the landing link). (b) UX s5 README: three Start-here links plus `/tours/`, and a link check covering README URLs; Story 3.3.2 only greps for removed assets and compares the URL to site defaults, and `check-internal-links.mjs` scans `dist`, not README. (c) UX s3 index build-stamp footer: only tour pages have an AC (3.1.1).

**C9. "Start here" third card is undefined.** UX s3 says "dashboard-lifecycle, approval-review, then the review step"; plan 3.1.1b says "dashboard, approvals, review". The 8 tours contain no separate review tour, and the names drift (approvals / approval-review / review). Name the three TourIDs.

**C10. Two-file rule vs actual edits.** Requirements NFR, ADR-002 and Story 5.1.3 ("no other edit") say a tour is one `.ts` + one `.mdx`. But `clips.ts` is an "index" (ADR-002, 2.3.1a), the Start-here order is hard-coded in `index.mdx` (3.1.1b), and tour-covered features need a hand-authored sidecar (4.1.7a). Require glob discovery for the scene index, or amend the rule.

**C11. Stale effort table.** Row 1.1.4 says "3 tasks, M + 2 S" but the story has four (1.1.4a M, 1.1.4b0 S, 1.1.4b S, 1.1.4c S); the table total says 76 tasks while the repair log and `grep` say 77. Raw tokens should rise by 55k (about 6,015k) and the CU and x1.75 figures follow. The "Counts after repair" line (77) is right; the table and the line-931 sentence ("70 -> 76") are not.

## NITPICK

- N1. Story 4.1.1 AC uses `tour: backlog-flow` / `tours/backlog-flow.mdx` as an example; no `backlog-flow` registry id exists and the tour is `backlog-pipeline`. Use `session-board-view` / `organise-at-scale`.
- N2. Glossary says `ThemeID` unknown ids are "an error, not `clean` silently"; Task 1.1.1a says unknown -> `clean` "only at the app boundary". Reword.
- N3. Glossary lists one `Scenario` interface; Pattern Decisions and 2.2.1a split it into `FileSeeder` / `RPCSeeder`. Glossary `FreshnessCheck` says Go `mediafresh`; the binary is `mediatool fresh` (the plan notes this once).
- N4. Dangling labels: Observability Plan says "Story 2.4.1b" (it is a Task; the flag-age AC lives in Story 2.4.1); Unresolved Questions say "Story 3.1.1a" (Task). Task order in Story 4.1.1 is a, b, d, c; 4.1.1c has no Files line.
- N5. Plan header Status says "ready for re-review" though Round 2 repair is logged; update after this check.
- N6. Wave plan: W1 lists 1.1.1 as parallel fan-out, but 1.1.1b/c must follow 1.1.3a in the same PR (W2). W3 omits 4.1.1d, which 4.1.1b needs.
- N7. UX clip cap "<= 60 s" and "Print", `<noscript>` poster states have no plan task or AC; the plan only has size budgets (clip <= 3 MB).
- N8. Requirements Risk Control says the stale fallback "can be re-enabled by reverting one workflow step"; after Phase 5 deletes the PNGs that is false (plan Risk Control already says so). Note the supersession in requirements or ADR-003.
- N9. ADR-003 allow-list description (logos, favicons, icons, snapshots) is shorter than plan 2.1.1b's list (adds `website/src/assets/**`, `project_plans/**/design/*.png`, test fixture icons). `backlog-scaffolding-guard.yml` runs only on PRs targeting `main`, so stacked-PR bases skip the guard; mention in ADR-003.

## Traceability summary

- Requirements to stories: all of R1-R20 are cited by at least one story. Requirement-level gaps are only the UX-derived ones in C5-C8.
- Stories without a requirement: none (every story carries R-ids).
- UX criteria without a task: UX-1, 2, 3, 6 (partly), 7 (C7); UX-12 (C8); UX-4 (C6). UX-5, 8, 9, 11, 13 are covered (3.1.1, 3.1.3, 3.1.1c/AC, 3.1.1 AC, 1.1.4c/2.4.1b).
- ADR vs plan: ADR-002/003/004/005 match the plan (go-git, `promolib`, flags in `.github/promo-flags.json`, `guard` job, sidecar naming, semantic fields). The one real divergence is ADR-001's schema (C4, C5).
