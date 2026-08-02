# Research: Technology Stack — backlog-description-prominence

## Confirmed versions (`web-app/package.json`)

| Package | Version |
|---|---|
| `@radix-ui/react-accordion` | `^1.2.17` |
| `react` | `^19.0.0` |
| `next` | `15.3.2` |
| `jest` | `^30.2.0` |
| `@testing-library/react` | `^16.3.0` |
| `@testing-library/jest-dom` | `^6.9.1` |
| `@testing-library/user-event` | `^14.5.2` |

No new dependencies are needed — this is a pure default-value flip using
libraries already in the tree. No `package.json` change required.

## Architecture: two independent "defaults" for the same section

This is the single most important finding — there are **two separate
`defaultExpanded` values** for the Description section, and only one of
them controls the real production UI:

1. **`BacklogItemDetail.tsx:323`** —
   `useSectionExpandState(itemId, "description", false)`. This is the value
   that actually matters. `DescriptionSection` is rendered as a child of a
   `CollapsibleGroup` (`web-app/src/components/ui/Collapsible.tsx`), and the
   group's initial-open set comes from `sectionExpandEntries` /
   `openSectionKeys` (`BacklogItemDetail.tsx:370-380`), which is built from
   this hook's returned boolean, not from any prop on `DescriptionSection`
   itself. **This is the line the fix must change** (`false` → `true`).

2. **`DescriptionSection.tsx`**'s own
   `<CollapsibleSection sectionKey="description" ... defaultExpanded={false}>`.
   Per `Collapsible.tsx:139-157`, `defaultExpanded` is **"architecturally
   dead" whenever the section renders inside a `CollapsibleGroup`** — Radix's
   single shared `Accordion.Root` (`type="multiple"`) is driven by the
   group's own `value`/`defaultValue`, and a child's own `defaultExpanded` is
   ignored (only used for the *standalone* fallback `Accordion.Root` that
   `CollapsibleSection` mounts when there is no enclosing group, e.g. in the
   sibling unit test below). There is a dev-only `console.warn` guard
   (`defaultExpandedDiverges = defaultExpanded && !groupSaysOpen`) but it
   only fires when `defaultExpanded` is `true` while the group says closed
   — leaving this prop at `false` after flipping (1) to `true` will **not**
   trigger the warning (the asymmetric condition doesn't check the reverse
   case), but leaving it stale is misleading given its own doc comment
   ("secondary info, collapsed by default (Story 3.1.3, Task 3.1.3a)").
   Should be flipped to `true` too, for consistency and because the comment
   text needs updating regardless.

## Radix `Accordion` pattern in use

`web-app/src/components/ui/Collapsible.tsx` wraps
`@radix-ui/react-accordion`'s `Accordion.Root` (`type="multiple"`),
`Accordion.Item`, `Accordion.Header`, `Accordion.Trigger`,
`Accordion.Content`. Two render modes:
- **Grouped** (`CollapsibleGroup` ancestor present, detected via React
  context `CollapsibleGroupContext`): renders only `Accordion.Item` /
  `Trigger` / `Content`, sharing one `Accordion.Root` per group for Radix's
  roving-tabindex keyboard nav across sibling headers (ADR-027).
- **Standalone** (no group ancestor): `CollapsibleSection` mounts its own
  single-item `Accordion.Root`, seeded from its own `defaultExpanded` prop.
  This is the mode `DescriptionSection.test.tsx` currently exercises (it
  renders `<DescriptionSection>` directly, with no `CollapsibleGroup`
  wrapper), so that test's assertions reflect mode (2) above, not the real
  production (grouped) behavior driven by mode (1).

No accordion API/version concerns — 1.2.17 is a recent 1.x release, already
in use elsewhere in the same file for every other collapsible section
(`reviewing`, `pull-request`, `sessions`, etc. all follow the identical
`useSectionExpandState(...)` + `sectionExpandEntries` + `CollapsibleGroup`
pattern), so this change follows an established, already-proven pattern —
no new architecture.

## Testing conventions (Jest + RTL)

Sibling test `web-app/src/components/backlog/detail/DescriptionSection.test.tsx`
uses:
- `render` / `screen` / `fireEvent` from `@testing-library/react`
- Test naming convention: `ComponentName_should_<effect>_When_<condition>`
  (e.g. `DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders`) —
  matches the convention documented in
  `.claude/rules/feature-testing-registry.md` for other frontend registries.
- `data-testid="collapsible-header-description"` /
  `data-testid="backlog-description-rendered"` — matches
  `.claude/rules/e2e-test-conventions.md`'s "locators: data-testid or ARIA
  roles only" convention (already followed here even though this is a unit,
  not e2e, test).
- Asserts `aria-expanded` on the header and DOM presence/absence (not just
  visibility) of the content `data-testid`, matching `Collapsible.tsx`'s
  documented contract that collapsed content is removed from the DOM, not
  hidden via CSS.

**Existing tests that will need updating alongside the fix** (found during
this research, in scope for the implementation phase, not this research
doc):
- `DescriptionSection.test.tsx`: `DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders`
  currently asserts `aria-expanded="false"` on initial (standalone) render —
  will need to flip to expect `"true"` (and be renamed) once
  `DescriptionSection.tsx`'s own `defaultExpanded` prop is flipped for
  consistency.
- `BacklogItemDetail.markdown.test.tsx`: has a helper `expandDescription()`
  (lines 12-21) that `fireEvent.click`s the header, with a doc comment
  stating "DescriptionSection ... is collapsed by default ... Expand it
  before asserting." Three tests call this helper
  (`bold/link markdown`, `image markdown`, `script-injection safety`).
  Once the group default is `true`, clicking an already-expanded header
  **collapses** it, which would break these three tests — the helper
  either needs to become a no-op/removed, or guarded to only click when
  currently collapsed (check `aria-expanded` first).
- No other `*.test.tsx` files reference `description` expand state
  (`BacklogItemDetail.test.tsx`'s only `CollapsibleGroup`-related tests are
  about generic keyboard nav across all sections and a "truthy" group value,
  not specifically about description's default — worth a quick recheck
  during implementation but nothing else matched `descriptionExpanded` /
  `DescriptionSection` in that file besides the keyboard-nav describe
  block).

## Summary of the actual code change (for the plan phase)

1. `web-app/src/components/backlog/BacklogItemDetail.tsx:323` — flip
   `useSectionExpandState(itemId, "description", false)` to `..., true)`.
2. `web-app/src/components/backlog/detail/DescriptionSection.tsx` — flip its
   own `defaultExpanded={false}` to `true` (dead in grouped mode, but keeps
   the component internally consistent/self-documenting) and update its
   stale doc comment ("collapsed by default").
3. Update the two test files identified above to match the new default.

No proto changes, no registry changes (this doesn't touch a `// +feature:`
marker or RPC), no CSS changes (per `.claude/rules/css-architecture.md`,
confirmed not needed — pure boolean default).
