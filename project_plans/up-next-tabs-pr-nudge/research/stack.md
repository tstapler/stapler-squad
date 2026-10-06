# Stack Research: up-next-tabs-pr-nudge

All paths relative to `web-app/` unless noted. Confidence: VERIFIED = file opened/grepped in this session.

## Versions (VERIFIED, `package.json`)
- next 15.3.2, react/react-dom ^19, typescript ^5.9.3
- @vanilla-extract/css ^1.20.1, @vanilla-extract/recipes ^0.5.7, @vanilla-extract/next-plugin ^2.5.1
- jest ^30.2.0, @testing-library/react ^16.3.0, user-event ^14.5.2, jest-dom ^6.9.1
- **No new dependencies needed.** No tabs library (Radix etc.) is installed; the repo hand-rolls tabs.

## Next.js constraint that shapes the design
`next.config.*` sets `output: "export"` and `trailingSlash: true` (VERIFIED). The app is a static export served by the Go binary: no server components reading `searchParams`, no middleware, no route handlers.
- `src/app/unfinished/page.tsx` wraps `<UnfinishedTab/>` in `<Suspense>` (required for `useSearchParams` under static export). Keep that wrapper.
- Tab state must be client-side. Use `useSearchParams()` (already used in `UnfinishedTab.tsx:34` for `?item=`) to read `?tab=`.
- URL writes: precedent is `router.replace(\`?${params}\`)` in `src/app/insights/InsightsDashboard.tsx:152`. Use `replace` (not `push`) so tab clicks do not pollute history; build from `new URLSearchParams(searchParams)` to preserve `?item=`.
- Routes live in `src/lib/routes.ts` (`unfinished`, `unfinishedItem(id)` -> `/unfinished?item=...`). Add a `unfinishedTab(tab)` helper there if linking to a tab (e.g. nav badge).

## Tabs / ARIA: existing patterns to copy
Hand-rolled `role="tablist"` / `role="tab"` / `aria-selected` exist in:
- `src/app/logs/page.tsx:122` (simple 2-tab filterGroup, no keyboard handling)
- `src/components/backlog/BacklogItemForm.tsx:557`
- `src/components/window/WindowTabStrip.tsx:244,392-415` (best reference: roving tabindex, ArrowLeft/ArrowRight wraparound in `handleTabListKeyDown`; comment notes `role="tablist"` must contain ONLY `role="tab"` children for axe `aria-required-children`)
- `src/components/pane/MobilePaneTabStrip.tsx:27-39`
Axe Core blocks CI on WCAG AA, so the new strip should follow WAI-ARIA APG tabs: `tablist` > `tab` (`id`, `aria-selected`, `aria-controls`, `tabIndex` roving 0/-1), panels `role="tabpanel"` with `aria-labelledby`, Arrow/Home/End keys. Badge counts inside a tab should be part of the accessible name (e.g. `aria-label="PRs, 3 need attention"`) and tests/e2e should use `getByRole("tab", {name})` / `data-testid` (e2e convention: no CSS-class locators).
Recommend extracting a small reusable `<TabStrip>` (generic over a tab-id union) rather than a fifth copy; keeps jscpd (0.12% threshold) happy.

## State persistence (localStorage) precedent
No shared `useLocalStorage`/`usePersistedState` hook exists (grep VERIFIED: none). Existing idioms are ad hoc raw calls:
- `src/app/review-queue/page.tsx:48-51,351` (lazy `useState` initializer reading `localStorage.getItem`, write in handler)
- `src/components/backlog/BacklogItemPanel.tsx:45,145` (namespaced key `backlog-panel-${id}`)
- `src/components/sessions/SessionList.tsx:99,289` (key-prefix prop to avoid collisions)
- `src/lib/pane/usePaneLayout.ts`, `src/lib/contexts/ThemeContext.tsx`, `NavigationContext.tsx`
SSR/static-export hazard: reading `localStorage` in a `useState` initializer on a statically prerendered page causes hydration mismatch. Safer: initial state from URL/default, then read localStorage in `useEffect` (or guard `typeof window`). Wrap in try/catch (private mode / quota).
Suggested key: `up-next-tab` (value in `"prs"|"stuck"|"worktrees"|"queue"`; validate against the union, fall back to `"prs"`).

### URL vs localStorage precedence (rabbit hole in requirements)
Resolve once on mount with a pure function, no competing effects:
1. `?item=` present -> `stuck` (deep link wins; StuckItemsSection's `focusItemId` needs that tab mounted).
2. else valid `?tab=` -> that tab.
3. else valid localStorage value.
4. else `prs`.
After mount, tab click writes both (`router.replace` + `localStorage.setItem`). Do NOT write localStorage when the tab came from a deep link `?item=` (would clobber the user's remembered tab), or decide explicitly. Keep it as a pure `resolveInitialTab(params, stored)` for easy jest testing.
Panels: keep inactive panels unmounted (or `hidden`) deliberately. `GitHubPRsSection` uses `useGitHubPRs` which opens a `WatchUserPRs` server stream; unmounting closes it. Badge counts need PR data while on other tabs, so lift `useGitHubPRs()` to `UnfinishedTab` and pass `prs` down (change `GitHubPRsSection` props accordingly; its test `GitHubPRsSection.test.tsx` mocks the hook).

## Existing code to reuse (VERIFIED)
- `src/app/unfinished/UnfinishedTab.tsx`: owns `useUnfinishedWork()`, filter chips (`role="group"`, `aria-pressed`), dismiss/snooze, `useSearchParams`, builds a `createConnectTransport`/`createClient(UnfinishedWorkService)`. Sections to split into panels: `StuckItemsSection` (`@/components/backlog-stuck`), `GitHubPRsSection` + worktree `UnfinishedRepoGroup`s (`In Progress`), `BacklogQueueSection`.
- `src/lib/hooks/useGitHubPRs.ts`: returns `{ prs: UserPR[], authState, refresh }`; subscribes to `WatchUserPRs` server stream, replaces list per snapshot, auto-reconnects.
- `src/components/unfinished/UnfinishedNavBadge.tsx(+.css.ts)`: nav badge; keep semantics unless requirements' open question decides otherwise.
- Hooks dir `src/lib/hooks/` (very populated; conventions: `useX.ts` with sibling `useX.test.ts`).

## Styling: vanilla-extract
- Co-located `*.css.ts`; import tokens `import { vars } from "@/styles/theme.css"` (e.g. `vars.space["2"]`, `vars.color.cardBackground`) and shared bases like `badgeBase` from `@/styles/collapsibleSection.css` (VERIFIED in `GitHubPRsSection.css.ts:1-3,13`).
- `@vanilla-extract/recipes` `recipe()` is in use (`src/app/insights/TimeRangeFilter.css.ts`, `ProjectedCostCard.css.ts`) -> use a recipe for active/inactive tab variants (or `selectors: {'&[aria-selected="true"]': ...}`, which keeps ARIA as the single source of truth). Existing tab CSS to crib from: `src/styles/window/windowTabStrip.css.ts`, `src/app/settings/settings.css.ts`.
- Reference `docs/reference/css-architecture.md` before adding styles.

## Testing stack
- Unit: jest + RTL + user-event. Existing: `src/app/unfinished/UnfinishedTab.test.tsx`, `src/components/unfinished/GitHubPRsSection.test.tsx`. Mock `next/navigation` (`useSearchParams`, `useRouter`) and `localStorage` (jsdom provides a real one; `localStorage.clear()` in `beforeEach`; per `deterministic-fast-tests` skill, avoid real timers/sleeps).
- E2E: `tests/e2e/` Playwright; first line `// @feature ...`, ARIA/`data-testid` locators only, no `waitForTimeout`; page helpers in `tests/e2e/pages/`. Axe blocks on WCAG AA for PRs touching `web-app/src/`.
- Feature registry: add `// +feature: ...` marker in new React files (first 10 lines) and run `make registry-generate`.
- Package manager: pnpm only (`docs/how-to/use-pnpm-in-web-app.md`).

## Proto / RPC side (outside web-app, for completeness)
- `UserPR` in `proto/session/v1/types.proto` needs additive fields (unresolved comment count, failing check names/URLs); `make proto-gen` regenerates TS (generated `gen/` output is gitignored).
- Nudge: a new RPC (or reuse of existing steering) in `server/services/` registered in `server/server.go`; client calls via `createClient(...)` + `createConnectTransport`/`createAuthInterceptor` as in `UnfinishedTab.tsx`. GitHub fetches must use `NewConditionalRequest*` (`.claude/rules/norawghrequest.md`).

## Dependencies needed
None. No new npm packages; additive proto fields only.

## Gaps / UNVERIFIED
- Did not open `useUnfinishedWork`, `StuckItemsSection` internals, or confirm whether `UnfinishedNavBadge` reads PR data.
- Did not verify whether `router.replace("?...")` scrolls/re-renders cleanly under `output: "export"` on this Next version (precedent in InsightsDashboard suggests it works; add `{ scroll: false }`).
- Did not check `next.config` for other relevant flags beyond `output`/`trailingSlash`.
