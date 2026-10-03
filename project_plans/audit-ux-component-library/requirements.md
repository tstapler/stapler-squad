# Audit page UX and build a component library

Backlog item d63d38e0-4a9c-497c-a38a-ccdce66fdcfc.

## Goal
Review every page for UX problems, fix concrete ones with local test evidence, catalog shared
components in a Storybook-style library, and make the app use shared components consistently
(starting with the rich path field, `RepoPathInput`).

## Requirements (from acceptance criteria)
| # | Requirement | Evidence |
|---|---|---|
| AC0 | Every page reviewed; findings + improvements documented per page | `audit.md` |
| AC1 | UX improvements covered by passing local tests | jest tests next to each change |
| AC2 | Storybook runs locally, shows each shared component (incl. `RepoPathInput`) in its main states | `pnpm storybook`, `pnpm build-storybook`, `storyCatalog.test.tsx` |
| AC3 | Path-accepting fields use `RepoPathInput`, or exceptions documented with reason | `audit.md` § Path fields |
| AC4 | Other unused-shared-component sites fixed or recorded as follow-up | `audit.md` § Shared-component adoption |

## Non-goals
Migrating all ~221 raw `<button>` files / ~90 raw `<input>` files; redesigning pages.
Those are recorded as follow-ups, not done here.

## Constraints
- Storybook builder is `@storybook/react-webpack5` (see `.storybook/main.ts`); no Next.js internals in stories.
- Tests run with jest (`pnpm exec jest`), no deployed environment.
