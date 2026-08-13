# ADR-001: Navigation Grouping Approach

**Date**: 2026-06-24
**Status**: Accepted
**Deciders**: nav-redesign planning

---

## Context

The navigation has 16+ flat items with no logical grouping. Three architectural options were evaluated for adding group metadata to nav items. The choice affects how many files change, how composable filtering and grouping are, and how much risk is introduced.

## Decision

**Option A: Add `group: NavGroup` field to the `NavPage` interface.**

`groupNavPages(pages: NavPage[]): Map<NavGroup, NavPage[]>` is the sole grouping utility. Components that want grouped rendering call it on a pre-filtered array. Components that do not care about groups continue to iterate the flat derived arrays unchanged.

## Alternatives Considered

**Option B: `NAV_GROUPS` nested structure** — replace the flat `NAV_PAGES` array with a `NavGroupDef[]` where each group contains its items. Rejected because: (1) breaks all three existing import sites (`DrawerNav`, `BottomNav`, `Header`) simultaneously; (2) creates a two-dimensional slicing problem — `bottomNavPrimary` vs `group` are orthogonal concerns that become awkward to express in a nested structure; (3) feature-flag filtering currently operates on the flat derived arrays and would need restructuring.

**Option C: Separate group registry** (`NAV_GROUP_REGISTRY: Record<NavGroup, string[]>`) — keep `NAV_PAGES` flat and add a parallel href-keyed registry. Rejected because: (1) two sources of truth — an item can be in `NAV_PAGES` but missing from the registry silently; (2) no TypeScript enforcement that all items are registered; (3) href strings in the registry diverge from `routes.*` constants if routes change.

## Consequences

**Positive**:
- Zero change to `BOTTOM_NAV_PRIMARY`, `BOTTOM_NAV_MORE`, `MOBILE_NAV_PAGES`, `HEADER_NAV_PAGES` derived arrays — all existing consumers continue to work
- `featureFlag` filtering composes naturally: filter first, then call `groupNavPages()` — empty groups produce no header
- `group` field is required (not optional) in the final schema, so TypeScript enforces assignment on every nav entry — no silent ungrouped items
- Backwards compatible — existing tests do not break from the data layer change alone

**Negative**:
- Group display labels (`NAV_GROUP_LABELS`) are a separate export from the items themselves — co-location is slightly weaker than Option B. Mitigated by keeping both in the same file (`nav-pages.ts`).
- Adding a group requires two edits: add the `NavGroup` literal to the type union, and add the display string to `NAV_GROUP_LABELS`.

## NavGroup Taxonomy

| Group | Display | Items |
|-------|---------|-------|
| `"work"` | Work | Sessions, Backlog, Unfinished, Review Queue, Notifications |
| `"automation"` | Automation | Workflows, Rules |
| `"insights"` | Insights | History, Insights, Escape Analytics |
| `"settings"` | Settings & Tools | Settings, Logs, Errors, Help, Files |

Config Files and Features are removed from `NAV_PAGES` as top-level entries (consolidated into the single Settings entry). The Settings page's internal tabs surface Config Files and Features sub-navigation.
