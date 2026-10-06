# Validation: AC -> check
0 audit-findings.md: script output + manual sweep; reviewed.
1 guard test (task 6) + migrated-field tests (task 3).
2 jest per field (task 3): `pnpm jest --testPathPatterns=<field>`.
3 guard test: every ui export has stories.
4 `pnpm build-storybook` exit 0.
5 `node scripts/audit-ui.mjs` regenerates component-usage.md deterministically.
6 audit script counts of raw elements decrease; remainder ticketed.
7 pnpm lint, pnpm test, lint:duplicates.
8 findings table references test names / e2e specs.
## Pre-mortem
Storybook build breaks on css.ts imports (mitigate: mock loader, build early in task 4). RepoPathInput stories flaky due to RPC (mock hooks). E2E selectors break when swapping inputs (preserve ids/testids; run affected specs). Scope blowup (timebox task 8).
