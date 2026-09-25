# ADR-003: v2 window layout is written to a new `cockpit.windowLayout` key, leaving `cockpit.paneLayout` untouched

**Date**: 2026-09-23
**Status**: Accepted

## Context

`research/architecture.md` flags two options for the v1→v2 migration target key:
reuse `cockpit.paneLayout` with the new v2 shape, or write to a new key and leave the
old key alone as an untouched backup. `requirements.md`'s Risk Control explicitly
requires "a fallback that preserves the raw old data untouched if migration parsing
fails."

## Decision

Write the migrated/created v2 layout to a **new** localStorage key,
`cockpit.windowLayout`, and never write to `cockpit.paneLayout` again post-migration.
`web-app/src/lib/pane/usePaneLayout.ts`'s existing `loadPaneLayout()`/
`savePaneLayout()`/`clearPaneLayout()` functions are read-only inputs to the one-time
migration path in `web-app/src/lib/window/windowPersistence.ts` and are otherwise
untouched (per the "reuse the existing pane engine unmodified" constraint).

## Consequences

- `cockpit.paneLayout` remains byte-for-byte whatever it was before this feature
  shipped, for the lifetime of the browser profile — a literal, inspectable backup a
  user (or support engineer) can recover from via devtools if the v2 path ever
  misbehaves, satisfying the Risk Control requirement more directly than an in-place
  version bump would.
- Costs one extra `localStorage.getItem` call on the migration path only (checked
  once per browser profile, until `cockpit.windowLayout` exists) — negligible.
- `cockpit.paneLayout` becomes permanently orphaned storage (never read again once
  `cockpit.windowLayout` exists) — acceptable; it's a small JSON blob, not an
  unbounded leak. Not proposing a cleanup/removal task, since removing it would
  defeat its purpose as a fallback.
