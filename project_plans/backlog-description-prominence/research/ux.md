# UX Research — Backlog Description Prominence

Scope: one-line `defaultExpanded` flip for `DescriptionSection` on an existing
collapsible pattern. No new UI surface.

## Critical finding: the flip point named in the requirements doc is not where the behavior actually lives

The requirements doc says "DescriptionSection's `defaultExpanded` flips from
false to true," but `DescriptionSection.tsx:20` renders inside
`BacklogItemDetail.tsx`'s shared `CollapsibleGroup` (`web-app/src/components/backlog/BacklogItemDetail.tsx:1185-1220`).
Per `CollapsibleSection`'s own doc comment (`web-app/src/components/ui/Collapsible.tsx:83`,
`:139-141`): *"inside a CollapsibleGroup, initial open state is controlled by
the group's defaultValue/value, and this prop has no effect."* The prop on
`DescriptionSection.tsx:20` is architecturally dead code for initial state.

The value that actually drives initial expand/collapse is
`BacklogItemDetail.tsx:323`:
```ts
const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", false);
```
— this `false` third argument is the real default, fed into `openSectionKeys`
(line 374) and the `CollapsibleGroup value=` prop. **The plan phase must flip
this `false` to `true`, not (only) the prop in `DescriptionSection.tsx`.**
For consistency with sibling sections (dev-mode warning is asymmetric — it
only fires when the local prop says `true` but the group says closed, not the
reverse — so leaving `DescriptionSection.tsx:20` as `false` wouldn't warn but
would be a misleading dangling value) the local prop should be flipped to
`true` too, matching how every other grouped section keeps its local
`defaultExpanded` in sync with its driving state var (e.g. `PullRequestSection.tsx:31`
hardcodes `true` alongside `pullRequestExpanded` initialized to `true` at
`BacklogItemDetail.tsx:316`).

## 1. Does expanding Description risk burying AC/Actions further down the page?

No — verdict: **safe, no reordering risk.** DOM order in
`BacklogItemDetail.tsx` is: Acceptance Criteria (~line 1146-1152, always
rendered, not collapsible) → Actions (~1154-1168, always rendered) →
`CollapsibleGroup` start (1185), inside which Description (1215) sits *after*
Reviewing/LastReviewResult/PullRequest and *before* PlanArtifacts/VersionControl/
Sessions/WorkflowHistory/ProgressHistory/Notes. AC and Actions already render
above Description unconditionally — expanding Description cannot push them
down; it only pushes down already-collapsed, lower-priority historical
sections beneath it. No conflict between "surface the description" and
"keep AC actionable" — they don't compete for the same screen position.

## 2. Conditional (Notes-style) vs unconditional (PullRequest-style) default-expand

This codebase already has both patterns:
- **Conditional** — `NotesSection` renders with `defaultExpanded={notesExpanded}`,
  where `notesExpanded` starts `false` (`BacklogItemDetail.tsx:322`) and is
  corrected to `true` post-load only `if (item.notes && !hasStoredPreference("notes"))`
  (`BacklogItemDetail.tsx:361-363`, Story 3.1.5's one-time-apply effect). Content-free
  Notes stays collapsed by default.
- **Unconditional** — `PullRequestSection` (`pullRequestExpanded` seeded `true`
  at line 316, no conditional logic) and `SessionsSection` (`sessionsExpanded`
  seeded `true` at line 319) always default open regardless of content.

**AC1 as literally written requires the unconditional pattern.** Re-reading
the exact text: *"shows Description expanded with content visible, zero
clicks"* — no qualifier like "when description is non-empty." This matches
`PullRequestSection`'s static-seed approach (`useSectionExpandState(itemId, "description", true)`),
not Notes' extra async correction effect. `DescriptionSection.tsx` already
renders a graceful `"No description."` fallback (lines 26-28) when
`item.description` is empty, so an unconditionally-expanded empty state is
harmless, not broken.

**No conflict to flag** — but note the two viable implementations differ in
mechanical weight: the unconditional route is the true one-line change the
requirements doc promises (flip the `useSectionExpandState` seed, done); the
conditional route would require replicating Notes' extra one-time-apply
`useEffect` branch (`BacklogItemDetail.tsx:337-368`) for no requirement
benefit. Recommend unconditional — it's both what AC1 asks for and the
smaller diff.

## 3. Accessibility — ARIA/keyboard-nav contract unaffected

Confirmed via `web-app/src/components/ui/Collapsible.tsx`. `aria-expanded` is
owned entirely by Radix's `Accordion.Trigger` (`CollapsibleItem`,
lines 91-115) and is derived from the *current* open/closed accordion state,
not from `defaultExpanded`. Roving-tabindex (Home/End/Arrow nav, ADR-027) is
implemented by Radix scoped to the shared `Accordion.Root` in
`CollapsibleGroup` (lines 41-77) — also independent of any section's initial
open value. A `defaultExpanded`/seed-value change only shifts which section
starts open; it exercises no different code path in either mechanism. No ARIA
or keyboard-nav regression risk from this change.

## 4. Zero-click JTBD matches problem statement

Problem statement: description is "typically the thing that is filled in by
default" but hidden, while AC is "shown by default but is not filled in by
default." AC1's "zero clicks" requirement directly targets this — surfacing
the field most likely to hold real content without requiring interaction,
while leaving AC's already-adequate always-visible treatment alone (AC3).
Confirmed aligned; no gap between problem statement and acceptance criteria.

## Summary for plan phase

- Real flip site: `BacklogItemDetail.tsx:323` — `useSectionExpandState(itemId, "description", false)` → `true`. Also flip the now-dead-but-should-stay-consistent prop at `DescriptionSection.tsx:20` (`defaultExpanded={false}` → `true`) to avoid a misleading dangling local default, even though the group value is what actually governs it.
- Use the unconditional (PullRequestSection-style) pattern — no content-conditional logic, no new `useEffect` branch. This is what AC1 literally asks for and keeps the diff minimal.
- No layout risk (AC/Actions already render above Description), no ARIA/keyboard-nav impact, no conflict between problem statement and acceptance criteria.
