# ADR-002: Storybook stays on `@storybook/react-webpack5`; `@storybook/nextjs` is deleted

Status: Accepted — owner chose to drop `@storybook/nextjs` on 2026-10-19 (flagged choice F2). Date: 2026-10-19
Evidence: `research/stack.md` §0 and §6, `research/build-vs-buy.md` §4. `web-app/.storybook/main.ts:1,20`.

## Context

`.storybook/main.ts` already uses `@storybook/react-webpack5` (with `useSWC`, a babel-loader rule and `.storybook/css-ts-mock-loader.js` that stubs `.css.ts`). `@storybook/nextjs` survives only as a devDependency at `package.json:136`. There are 50 `*.stories.tsx`; no workflow or Makefile target runs Storybook or Chromatic. Only `AppLink.stories.tsx` and `Navigation.stories.tsx` import `next/*`.

## Decision (default: keep)

1. Delete `@storybook/nextjs`. Keep `react-webpack5` 8.6.18; no Storybook major upgrade in this project.
2. Stories get the router through a `@/lib/nav/impl` webpack alias to `impl.memory.tsx` (an in-memory fake that satisfies `impl.types.ts`), so Storybook depends on neither Next nor React Router.
3. Add a non-blocking CI job that runs `storybook build`, so Storybook cannot rot silently once Next is gone.
4. The Bun VE plugin is not exercised by Storybook (it uses the mock loader); this is accepted.

## Alternatives

- **Delete Storybook**: drops about 10 devDependencies and a second bundler. Choose this if the owner does not use it; Story 6.6 then deletes `.storybook/`, 50 stories, scripts and the CI job instead.
- **`@storybook/react-vite`** with the official VE plugin: real styles in stories, but adds Vite and a second CSS pipeline and needs a Storybook 8 to 10 upgrade. Deferred as a separate project.

## Consequences

One devDependency removed, no framework migration. Remaining cost: webpack stays in devDependencies. Revisit if Storybook starts costing more than it returns.
