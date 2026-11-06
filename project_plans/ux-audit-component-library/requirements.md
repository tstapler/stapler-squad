# UX Audit & Component Library

## Problem
Pages and forms drift: `RepoPathInput` (rich path field with completions, history, GitHub-URL detection) exists in `web-app/src/components/ui/`, but path-accepting fields in `TriggerFormModal`, `ForkModal`, `AliasesManager` (and likely others) use bare `<input>`. Only 3 `*.stories.tsx` exist, so there is no catalog of which UI components exist or where they are (not) used.

## Goals
1. Audit all 18 routes in `web-app/src/app/*/page.tsx` for UX defects; record findings with severity and evidence.
2. Fix inconsistencies, starting with every path/directory field -> `RepoPathInput`.
3. Storybook component library cataloging `components/ui/*` (and shared primitives) with stories covering states.
4. Local, automated verification: jest tests, a lint/consistency guard, Playwright a11y/spec runs, `build-storybook`.

## Acceptance criteria
0. `project_plans/ux-audit-component-library/audit-findings.md` lists every route with findings (severity, file, status) and evidence of re-check.
1. All path/directory text inputs use `RepoPathInput` (or are listed with a justified exemption); a guard test/lint fails on new bare path inputs.
2. Each fixed path field has a jest test asserting the completion dropdown/RepoPathInput renders and value change propagates.
3. Every component exported from `components/ui` has a `.stories.tsx` covering default, disabled, error and edge states.
4. `pnpm build-storybook` succeeds locally; `pnpm storybook` serves the catalog.
5. A generated component-usage report (component -> files using it, plus unused/duplicated components) is committed or reproducible via script.
6. Non-path inconsistencies found (raw `<button>`/`<input>` vs `Button`/`Input`, ad hoc modals, inline styles) are fixed or ticketed with rationale.
7. `pnpm lint`, `pnpm test` (jest), and affected Playwright specs (a11y) pass locally; `lint:duplicates` stays under threshold.
8. Before/after evidence (tests, screenshots from e2e/Storybook) referenced for each fixed finding.

## Non-goals
Redesign of visual identity; backend changes; replacing Storybook builder.
## Constraints
pnpm only in web-app; vanilla-extract CSS; e2e conventions (data-testid/ARIA, no waitForTimeout, `// @feature` header); don't run `make install-service`.
