# Research: Help Tooltip Coverage

## Codebase Audit Results

### Help page architecture

- Route: `/help` (defined in `web-app/src/lib/routes.ts:18`)
- Page component: `web-app/src/app/help/page.tsx` — sidebar + article pane, search via Fuse.js
- Doc loader: `web-app/src/lib/docs/docLoader.ts` — hardcoded `DOC_FILES` array, fetches from `/docs/<slug>.md` at runtime
- Public docs directory: `web-app/public/docs/` — currently contains exactly 6 files:
  - `what-is-stapler-squad.md`
  - `session-types.md`
  - `omnibar.md`
  - `keyboard-shortcuts.md`
  - `configuration.md`
  - `tmux-integration.md`

The help page currently has no deep-link mechanism (`?doc=` URL param is not read on mount).

### Tooltip component

`web-app/src/components/ui/Tooltip.tsx`:
- Wraps Radix UI `@radix-ui/react-tooltip`
- Props: `{ children, label: string, side? }` — no help-link support
- The `label` prop is `string` only; no React node or link content is possible today

### Tooltip usage vs native `title=`

| Pattern | Production `.tsx` files | Notes |
|---|---|---|
| `<Tooltip` (custom component) | 6 production files | 4 insights charts, `SessionCard.tsx`, `SessionRow.tsx` |
| native `title=` attribute | 135 production files | Inaccessible (no keyboard, screen reader inconsistent) |

The 6 `<Tooltip>` users:
1. `app/insights/DailySpendChart.tsx`
2. `app/insights/ModelBreakdownChart.tsx`
3. `app/insights/ModelOverTimeChart.tsx`
4. `app/insights/StageCostChart.tsx`
5. `components/sessions/SessionCard.tsx`
6. `components/sessions/SessionRow.tsx`

### Key components with native `title=` (high-traffic)

From `grep` over `components/layout/`:
- **`Header.tsx`**: create-session button, notifications button, debug menu button, keyboard-shortcuts button — all use `title=` + `aria-label=`
- **`DrawerNav.tsx`**: collapsed-state icon labels use `title=` (line 69)
- **`CockpitShell.tsx`**: keyboard shortcuts button uses `title=` (lines 88–89)
- **`ConnectionIndicator.tsx`**: uses `title=` (line 73) — also already has `aria-label=`
- **`WorkspaceSwitcher.tsx`**: workspace path/merge buttons use `title=`

Other high-value areas (outside layout/):
- `components/backlog/BlockerChip.tsx`, `ClaimChip.tsx`, `DuplicatePendingBadge.tsx` — status chips
- `components/sessions/StatusBadge.tsx` — status chips (confirmed via `<Tooltip>` import in SessionCard)
- `components/sessions/CIStatusBadge.tsx`, `ConnectionCountIndicator.tsx`

### Hint/inline-help pattern

Some form-level components use a `hint` prop for sub-label help text:
- `web-app/src/components/ui/RadioGroup.tsx`: accepts `hintForValue?: (v) => string` prop
- `web-app/src/components/backlog/BacklogItemForm.tsx`: `hint="..."` on input fields
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx`: `hint="..."` on path input
- `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`: `hintText` style class for inline instructions

These are inline text, not tooltip-based. They serve a different purpose (persistent form guidance vs. hover/focus contextual label).

### Existing coverage checks (analogues)

`web-app/src/lib/__tests__/notification_docs_exist.test.ts` — CI test that:
1. Verifies specific markdown files exist at expected paths under `docs/`
2. Verifies every relative link in those files resolves
3. Verifies a specific required phrase is present

This is the exact pattern to adapt for tooltip/help coverage.

### Settings page help links

`web-app/src/app/settings/page.tsx` uses a `helpLink` CSS class on `<Link>` elements to settings sub-pages. One link goes to `routes.help`. These are settings-level nav links, not tooltip help links.

### `/help` deep-link support (not yet implemented)

`web-app/src/app/help/page.tsx` mounts with `useEffect` to load docs, then sets `selectedSlug` to `entries[0].slug`. There is no URL parameter read. Adding `useSearchParams()` to read `?doc=<slug>` and pre-selecting that entry would be straightforward.

### Feature flags / behind-gate check

No feature flag gates the help page. It is always accessible.

## Key Findings

1. **The `Tooltip` component needs a `helpSlug?: string` prop** that, when set, appends a "Learn more →" link navigating to `/help?doc=<slug>`. This requires the `label` render branch to produce a React node containing text + link. The existing `Tooltip.css.ts` will need a link style.

2. **The `/help` page needs `?doc=<slug>` deep-link support** — read `useSearchParams()` on mount, find the matching slug in the loaded docs, and set `selectedSlug` to it.

3. **`docLoader.ts` `DOC_FILES` must stay in sync with `public/docs/`** — a Jest test in `web-app/src/lib/__tests__/` that reads the filesystem (like `notification_docs_exist.test.ts`) can enforce this. The test runs in Node context so `fs.readdirSync` is available.

4. **New doc files needed** for at least: `backlog.md`, `insights.md`, `notifications.md`, `tagging-rules.md`, `workflows.md`. These map to the major nav sections lacking docs.

5. **High-traffic `title=` → `<Tooltip>` migration** is valuable for accessibility but is a non-trivial sweep. Scope it to layout-level controls (Header, DrawerNav, CockpitShell, ConnectionIndicator) and session-status chips (StatusBadge, CIStatusBadge).

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `Tooltip` `helpSlug` breaks existing callers | Low | Low | `helpSlug` is optional; existing callers unchanged |
| Deep-link support creates flickering on load | Low | Medium | Set `selectedSlug` synchronously from URL before first render via `useSearchParams` |
| New doc files are thin/low-quality | Medium | Medium | Draft minimal content; iterate after ship |
| `DOC_FILES` diverges from `public/docs/` again | Low after gate | Low after gate | The CI test is the permanent fix |
| Wide `title=` sweep causes merge conflicts | Medium | Low | Scope to specific files, not blanket sweep |
