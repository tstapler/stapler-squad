# Requirements: nav-redesign

**Date**: 2026-06-24
**Type**: refactor / improvement to existing code

## Problem Statement

The navigation has grown to 16+ flat items with no logical grouping or hierarchy. The current structure creates three concrete problems:

1. **Discoverability** — features like Insights, Workflows, Escape Analytics are buried in an unstructured "More" drawer/sheet with no indication of what category they belong to.
2. **Mobile nav is incomplete** — several routes (Settings, Logs, Errors, Help, Files, Insights, Escape Analytics) are hidden on mobile (`mobileNav: false`) rather than reorganized. Mobile users lose access to those capabilities entirely.
3. **Settings fragmentation** — "Settings", "Config Files", and "Features" are three separate nav items despite all being configuration-related. Users must know to look in three places.

The affected users are all end users of the stapler-squad web UI, both on desktop (DrawerNav) and mobile (BottomNav + More sheet).

## Users / Consumers

End users of the stapler-squad web UI — both desktop and mobile/tablet form factors.

## Success Metrics

- Navigation works correctly on mobile (all routes are reachable on mobile)
- Items are grouped into logical sections (≤3–4 groups covering all 16+ items)
- A new user can identify what features exist without trial-and-error
- No nav items are silently hidden on mobile without a mobile-accessible alternative
- Existing routes all continue to work (no 404s)
- All existing tests pass; new tests cover grouping logic

## Constraints

No deadline. No hard performance or compliance constraints.

## Scope

### In Scope

- Redesign `nav-pages.ts` to add group/category metadata to all nav items
- Reorganize the DrawerNav (desktop sidebar) to render grouped sections with headers
- Reorganize the BottomNav "More" sheet to render grouped sections instead of a flat list
- Ensure all currently-hidden mobile routes (`mobileNav: false`) are reachable on mobile (either promoted, or put in a visible group in the More sheet)
- Consolidate Settings / Config Files / Features into a single "Settings" nav entry with sub-navigation handled at the settings page level, or a grouped settings section

### Out of Scope

- Changing the routes themselves (no URL changes)
- Adding or removing pages (no new features, no page deletions)
- Changing the BottomNav primary bar item count (still 4–5 items + New + More)
- Redesigning the settings page itself (only the nav entry)
- Internationalization / translations

## Open Questions

1. **Grouping taxonomy** — what are the right group names? Candidates:
   - Work (Sessions, Backlog, Unfinished, Review Queue)
   - Automation (Workflows, Rules)
   - Insights (Insights, History, Escape Analytics)
   - Settings & Config (Settings, Config Files, Features, Logs, Errors, Files, Help)
   - Or a different split?

2. **Desktop DrawerNav** — should groups have collapsible sections, or always-expanded with a visual divider + header?

3. **Mobile Settings access** — should Settings be promoted to a BottomNav primary item (replacing one current item), or is a visible group in the More sheet sufficient?

4. **Feature flags** — the Backlog item is feature-flagged. Should the grouping system support feature-flagged groups (hide the whole group if all items are flagged off)?

5. **Badge routing** — ReviewQueue and Unfinished have count badges; Notifications is custom-rendered. Do badges remain item-level or move to group headers?
