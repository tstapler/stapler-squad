# Nav Redesign — Feature Research

**Date:** 2026-06-24
**Scope:** Navigation grouping, mobile parity, industry patterns, UX improvements

---

## 1. Full Nav Item Inventory

### Current item count: 16 (+ Account + Handedness toggle in More sheet)

| Item | Route | bottomNavPrimary | mobileNav | headerNav | featureFlag |
|---|---|---|---|---|---|
| Sessions | `/` | ✅ | ✅ | ✅ | — |
| Backlog | `/backlog` | ✅ | ✅ | ✅ | `backlog` |
| Unfinished | `/unfinished` | ✅ | ✅ | ✅ | — |
| Review Queue | `/review-queue` | ✅ | ✅ | ✅ | — |
| Notifications | `/notifications` | ✅ (custom) | ✅ | ✅ | — |
| Settings | `/settings` | — | **false** | ✅ | — |
| Insights | `/insights` | — | **false** | **false** | — |
| Workflows | `/workflows` | — | ✅ | **false** | — |
| Rules | `/rules` | — | ✅ | **false** | — |
| History | `/history` | — | ✅ | **false** | — |
| Config Files | `/settings?tab=config-files` | — | ✅ | **false** | — |
| Features | `/settings/features` | — | ✅ | **false** | — |
| Logs | `/logs` | — | **false** | **false** | — |
| Errors | `/errors` | — | **false** | **false** | — |
| Help | `/help` | — | **false** | **false** | — |
| Escape Analytics | `/analytics/escape` | — | **false** | **false** | — |
| Files | `/files` | — | **false** | **false** | — |

**8 items hidden on mobile** (`mobileNav: false`): Settings, Insights, Logs, Errors, Help, Escape Analytics, Files. Note: `headerNav: false` items are still in hamburger on desktop but entirely absent on mobile.

**Currently in BottomNav More sheet (mobile):** Workflows, Rules, History, Config Files, Features. Plus dynamically-appended Handedness toggle and Account link.

---

## 2. Proposed Grouping Taxonomy

### Recommended: 4 groups

After analyzing the item semantics, **4 groups** strikes the right balance — enough to create clear mental models without fragmenting navigation into too many headings.

| Group | Label | Items | Rationale |
|---|---|---|---|
| **Work** | Work | Sessions, Backlog, Unfinished, Review Queue, Notifications | Core task-management loop; what users navigate to every session |
| **Automation** | Automation | Workflows, Rules | Agent-behavior configuration; change infrequently but are related |
| **Insights** | Insights | History, Insights, Escape Analytics | Retrospective/analytical views; all read-only, data exploration |
| **Settings & Tools** | Settings & Tools | Settings (with Config Files + Features sub-tabs), Help, Logs, Errors, Files | Power-user and system tools; Settings consolidation per requirements |

### Consolidation detail: Settings + Config Files + Features → single "Settings" entry

- `routes.settings` becomes the canonical Settings entry in the nav
- Config Files tab already uses `routes.settings + "?tab=config-files"` — this is a sub-tab of Settings, not a top-level route
- `routes.settingsFeatures` (`/settings/features`) is also a sub-page of Settings
- Nav entry: single "Settings" item pointing to `/settings`; tabs within Settings surface Config Files and Features
- Removes 2 redundant top-level entries; drawer nav shows one "Settings" row

---

## 3. Mobile Accessibility Decision for Each Currently-Hidden Item

### Currently `mobileNav: false` — how each should be handled:

| Item | Current | Recommended | Rationale |
|---|---|---|---|
| **Settings** | BottomNav hidden | More sheet (Settings & Tools group) | Settings is medium-frequency; every user needs it eventually; hiding it entirely is a usability gap |
| **Insights** | BottomNav hidden | More sheet (Insights group) | Medium-frequency; belongs alongside History and Escape Analytics in Insights group |
| **Logs** | BottomNav hidden | More sheet (Settings & Tools group) | Low-frequency but legitimately needed on mobile when debugging; should not require desktop |
| **Errors** | BottomNav hidden | More sheet (Settings & Tools group) | Same rationale as Logs; error investigation on-the-go is a real use case |
| **Help** | BottomNav hidden | More sheet (Settings & Tools group) | Especially needed on mobile where discoverability is weakest |
| **Escape Analytics** | BottomNav hidden | More sheet (Insights group) | Niche/power-user analytics; grouped correctly in Insights |
| **Files** | BottomNav hidden | More sheet (Settings & Tools group) | Context-dependent; medium-frequency enough to expose |

**None of the currently-hidden items qualify for BottomNav primary.** All 8 are lower-frequency than the existing 5 primary items. Promoting any of them would crowd the primary bar beyond usability (max 5–6 items in a touch-optimized bar).

**Settings is the only item that could arguably be primary**, but given 5 spots are already taken, it belongs at the top of the More sheet instead — where it's the first item users find when tapping "More."

---

## 4. Industry Navigation Patterns for Tools with Navigation Overflow

### Linear

- **Desktop**: Collapsible left sidebar with **sections** (My Issues, Teams, Projects, Views). Clear section headers with disclosure triangles.
- **Mobile**: Bottom bar with 4 primary items (Inbox, My Issues, Teams, +). No visible "More" — overflow is handled by deep sidebar navigation only accessible after scrolling within a section.
- **Lesson**: Sections in the sidebar greatly reduce cognitive load even before collapsing. Users scan section headers as landmarks, not individual items.

### Vercel Dashboard

- **Desktop**: Left sidebar with icon + label. Groups: Overview, Storage, Integrations, Settings. No explicit section headers — visual whitespace separates groups.
- **Mobile**: Full-page nav drawer (not bottom nav). Sections visible as discrete regions.
- **Lesson**: Whitespace between groups can replace explicit headers when item count per group is small (3–5 items).

### GitHub

- **Desktop**: Repository sidebar has **labeled sections** (Code, Issues, Pull requests, Actions, Projects, Wiki, Security, Insights, Settings). 9 sections for a page with 15+ items.
- **Mobile**: The repository view collapses to a horizontal scroll tab bar (Code, Issues, PRs) for primary items; secondary items accessible via "..." overflow that presents a **bottom sheet grouped by section**.
- **Lesson**: The "... grouped bottom sheet" pattern is exactly what this redesign is proposing for BottomNav. GitHub validates this pattern at scale.

### Retool

- **Desktop**: Left sidebar with icon-only collapsed mode (tooltip on hover) and label-expanded mode. Clear sections: Apps, Workflows, Resources, Settings.
- **Mobile**: Limited mobile support; treated as power-user desktop tool.
- **Lesson**: Icon-only collapsed sidebar is the established pattern for desktop DrawerNav; Retool's 4-section grouping for ~20 items is a strong analog.

### Common patterns across all 4 tools

1. **2–4 section groups** is the universal sweet spot for 10–20 items. More than 4 groups creates its own discoverability problem.
2. **Primary bottom bar capped at 4–5 items**. All tools that support mobile enforce this constraint strictly.
3. **Grouped bottom sheet / overflow drawer**: when the primary bar is full, overflow items go into a contextual sheet that is **not a flat list** — it's organized by section. GitHub, iOS Settings app, and Material Design's Navigation Drawer all use this.
4. **Settings always lives in a dedicated group** at the end of the nav. No tool buries Settings in the primary bar.
5. **Desktop sidebar sections use headers + optional disclosure**: section label in muted uppercase or small caps above items in that section. Standard pattern in Linear, Notion, VS Code.

---

## 5. Current More Sheet UX Problems and Improvements

### Problems identified in `BottomNav.tsx`

1. **Flat list rendering** (lines 119–133): `morePages.map(...)` renders all items as an undifferentiated vertical list. No section headers, no visual grouping. With 5 items today and 12+ items after mobile parity fix, this will be entirely unusable.

2. **Statically filtered by `mobileNav`** (`nav-pages.ts` lines 61, 69–71): Items with `mobileNav: false` are categorically excluded from `MOBILE_NAV_PAGES` and `BOTTOM_NAV_MORE`. After the redesign, all items should be included; the `mobileNav` field should be replaced by the `group` field, and primary/secondary should be the only distinction that matters.

3. **Mixed concerns at bottom of sheet** (lines 134–152): Handedness toggle and Account link are appended as unrelated items below the nav links. These should be in a clearly separated "Account & Preferences" section or moved to Settings.

4. **No section headers**: Screen readers and visual scanners both suffer. Adding `<h3>` or `role="group"` with `aria-label` per section dramatically improves accessibility.

5. **No active-group highlighting on "More" button**: Currently `isMoreActive` checks if any More-sheet item is active (line 70), which lights up the More button. With groups, the button label or badge could reflect the active group name (e.g. "Insights" instead of "More") — though this is a nice-to-have.

6. **Animation**: The sheet slides up (`moreSheetOpen` class), which works. No issues here, but with more items the sheet may need a `max-height: 85vh` + internal scroll to avoid overflowing on small screens.

### Recommended UX improvements

1. **Render sections with headers in the More sheet**: Each group gets a labeled `<section>` or `role="group"` container with a `<h3>` header. Items within each section are listed below the header.

2. **Add `group` field to `NavPage` interface**: Replace the overloaded `mobileNav`/`headerNav` booleans with an explicit `group: "work" | "automation" | "insights" | "settings"` field. Derive visibility rules from group + priority metadata.

3. **Surface Settings at the top of the More sheet**: Since Settings is the most frequently-needed hidden item, it should render first (or in its own top section) in the More sheet.

4. **Separate account/utility actions**: Move Handedness toggle and Account link into a distinct bottom section of the sheet, visually separated (divider line), labeled "Account" or similar. These are not navigation items and mixing them with nav links is confusing.

5. **Keyboard navigation**: The More sheet currently handles `Escape` to close (lines 44–51). Add `Tab`/`Shift+Tab` trap within the sheet when open, and `Enter`/`Space` on items.

---

## 6. DrawerNav Desktop Grouping

The `DrawerNav.tsx` currently iterates `NAV_PAGES` as a flat list (line 34). It already has a `drawerDivider` CSS class imported (line 21) but never renders section headers.

### Recommended desktop sidebar structure

```
[ WORK ]
  Sessions
  Backlog (flag-gated)
  Unfinished
  Review Queue
  Notifications

[ AUTOMATION ]
  Workflows
  Rules

[ INSIGHTS ]
  History
  Insights
  Escape Analytics

[ SETTINGS & TOOLS ]
  Settings
  Help
  Logs
  Errors
  Files
```

When the drawer is **collapsed** (icon-only mode), section headers are hidden and items render as a continuous icon column — same as current behavior. When **expanded**, section headers appear as small-caps muted labels above each group, with subtle spacing between groups.

---

## 7. Proposed `NavPage` Schema Changes

To support grouping, `nav-pages.ts` needs these additions to the `NavPage` interface:

```typescript
export type NavGroup = "work" | "automation" | "insights" | "settings";

export interface NavPage {
  // ... existing fields ...
  group: NavGroup;
  // mobileNav field becomes optional/deprecated — derived from group + bottomNavPrimary
}
```

The derived list helpers (`MOBILE_NAV_PAGES`, `BOTTOM_NAV_MORE`, etc.) would be updated to:
- Include all items regardless of group (removing the `mobileNav: false` exclusion pattern)
- Group items by `group` field for section rendering
- Let `bottomNavPrimary` continue to control primary bar placement
- A new `DRAWER_NAV_GROUPS` export that returns items organized by group

---

## 8. Key Decisions for Planning Phase

1. **Settings consolidation strategy**: Should `/settings`, `/settings?tab=config-files`, and `/settings/features` be a single nav entry pointing to `/settings` (simplest), or should Config Files and Features be removed from nav entirely in favor of tabs within the Settings page? **Recommendation**: single Settings nav entry; verify the Settings page already has or will gain tab navigation for Config Files and Features.

2. **Backlog feature flag**: The `backlog` feature-flagged item stays in the Work group with its existing `featureFlag: "backlog"` annotation. No change needed.

3. **Schema migration**: The `mobileNav: false` pattern can be preserved during transition (it remains valid to prevent an item from appearing on mobile), but the redesign should not use it as the primary access-control mechanism — `group` should determine section placement and all groups are accessible on both platforms.

4. **DrawerNav expanded vs. collapsed rendering**: Section headers only appear in expanded mode. The `navLabel({ visible: isDrawerOpen })` recipe already handles show/hide for labels; section headers need the same treatment.
