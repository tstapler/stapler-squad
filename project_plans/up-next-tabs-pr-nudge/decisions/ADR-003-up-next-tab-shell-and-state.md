# ADR-003: Tab shell (controlled Radix), state precedence, data lifting

**Status**: Accepted | **Date**: 2026-10-05

## Context
`research/stack.md` claims no tabs library; `@radix-ui/react-tabs` is installed (`web-app/package.json:77`) and used in `web-app/src/app/settings/page.tsx:6,38-40` (uncontrolled). The app is a static export (`output: "export"`) so tab state is client-side; `page.tsx` keeps `<Suspense>` for `useSearchParams`.

## Decision
- Controlled Radix Tabs in a thin `UpNextTabs` wrapper with badge slot; no hand-rolled strip.
- `resolveInitialTab`: `?item=` (Stuck) > valid `?tab=` > localStorage `up-next-tab` > `prs`. First render pass uses `prs` (hydration-safe); storage is read in a `useLayoutEffect` before first paint, so the PRs panel is never painted for a user whose stored tab differs (no skeleton flash). Tabs use `activationMode="manual"` (arrow moves focus, Enter/Space selects) so arrowing past the heavy Worktrees panel does not mount it or write storage/URL per keypress. Panels are `tabindex=0` with an `h2`. Storage access in try/catch.
- Writes: user click only -> localStorage + `router.replace(?tab=..., {scroll:false})`, which deletes `item`. Deep-link resolution never writes storage.
- Inactive panels unmount (Radix default). To keep badges and PR filters stable, `useGitHubPRs` (single `WatchUserPRs` stream) and PR filter state are lifted into `UnfinishedTab`. Stuck `focusItemId` runs on mount of the Stuck panel.
- `UnfinishedNavBadge` unchanged; per-tab badges are separate (PRs = count of non-draft PRs with failing CI, changes requested, unresolved threads or conflicts; Stuck = stuck count; zero hides badge).

## Alternatives rejected
`forceMount` all panels (runs hidden effects, complicates focus/scroll); localStorage-in-initializer (hydration mismatch); shared hand-rolled TabStrip from `EscapeAnalyticsPage` (re-implements ARIA).
