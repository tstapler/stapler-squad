# Research: Backlog Description Prominence — Existing Patterns & Edge Cases

## 1. Existing `defaultExpanded` call sites in `BacklogItemDetail.tsx`

All ~10 secondary sections share one `CollapsibleGroup` (lines 1185–1258, comment at
1170–1184, "Task 3.1.4i"). Each section's real, effective default lives in a
`useSectionExpandState(itemId, sectionKey, defaultBool)` call at lines 314–323 — **not**
in the boolean literal each section component receives as `defaultExpanded` (that prop is
architecturally dead once the section is rendered inside a `CollapsibleGroup`; see
`Collapsible.tsx` lines 137–163 — the group's own controlled `value`/`openSectionKeys`
is what actually drives open state, and a diverging `defaultExpanded` only triggers a
dev-mode `console.warn`).

Current static defaults (`web-app/src/components/backlog/BacklogItemDetail.tsx:314-323`):

| Section key | Static default | Rationale (from surrounding comments) |
|---|---|---|
| `reviewing` | `false` | status-dependent (see below), static value here is just the pre-load fallback |
| `last-review-result` | `false` | status-dependent |
| `pull-request` | `true` | always-relevant once a PR exists — the actionable content of that state |
| `plan-artifacts` | `false` | secondary reference material |
| `version-control` | `false` | status-dependent |
| `sessions` | `true` | primary, frequently-checked content once sessions exist |
| `workflow` | `false` | historical/audit trail, rarely needed |
| `progress-history` | `false` | historical/audit trail |
| `notes` | `false` | status-dependent (opens if item has notes) |
| `description` | `false` **(the item this task changes)** | comment at `DescriptionSection.tsx:14-16`: "secondary info, collapsed by default (Story 3.1.3, Task 3.1.3a)" — this is exactly the framing the requirements doc says is wrong |

A second layer (lines 325–368, "Story 3.1.5") applies **status-dependent** one-time
overrides after `item` first loads, but only if `hasStoredPreference(key)` is false (a
direct `localStorage.getItem` check, bypassing `useSectionExpandState`'s own fallback):
`reviewing`→true when `status==="review"`, `last-review-result`→true when there's a
gate verdict, `version-control`→true for in-progress/review/pr_pending, `notes`→true
when `item.notes` is non-empty. **`description` has no such override today** — it is a
plain static default, so the fix here is simply changing the literal at line 323 from
`false` to `true` (FR1). No status-dependent logic is needed or requested.

`PullRequestSection` (`true`) and `SessionsSection` (`true`) are the two existing
precedents for "defaults open because it's the load-bearing content for that view" —
same rationale this task applies to Description.

## 2. `DescriptionSection.tsx` — the actual bug (FR4)

`web-app/src/components/backlog/detail/DescriptionSection.tsx:20` hardcodes
`defaultExpanded={false}` directly on its internal `<CollapsibleSection>`, and its props
interface (`DescriptionSectionProps`, lines 10-12) has no `defaultExpanded` field at all —
unlike every sibling section (`ReviewingSection`, `PullRequestSection`,
`PlanArtifactsSection`, `VersionControlSection`, `SessionsSection`,
`WorkflowHistorySection`, `ProgressHistorySection`, `NotesSection` — 8 siblings, matching
the requirement's "~8 sibling detail sections" count), which all accept `defaultExpanded`
as a prop threaded from `BacklogItemDetail.tsx`. Because Description is rendered inside
the shared `CollapsibleGroup` (line 1215: `<DescriptionSection item={item} />`, no
`defaultExpanded` passed), this hardcoded `false` is currently dead code in practice — the
group's controlled `value` (`openSectionKeys`, built from `descriptionExpanded` at line
374) is what actually governs the rendered state. But it's still wrong/misleading and
diverges from every sibling's shape, and it **is** live in `DescriptionSection.test.tsx`,
which renders `<DescriptionSection item={...} />` standalone (no `CollapsibleGroup`
wrapper) — there the hardcoded `false` is the only thing determining behavior. That
existing unit test (`DescriptionSection.test.tsx:29-35`,
`DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders`) directly asserts
`aria-expanded="false"` and must be updated to pass `defaultExpanded` and assert per the
new default — this is a required companion change to satisfy FR6 ("targeted Jest suites
must pass"), even though the requirements doc's numbered FRs don't name this file
explicitly.

The docstring at lines 14-16 ("secondary info, collapsed by default (Story 3.1.3, Task
3.1.3a)") is the "stale docstring" FR4 calls out — needs updating to reflect
expanded-by-default and to accept `defaultExpanded` as a prop like siblings.

## 3. `useSectionExpandState.ts` — localStorage semantics (FR2, migration concern)

Read in full (`web-app/src/lib/hooks/useSectionExpandState.ts:15-43`). Key finding for
the "stale stored `false`" migration concern:

- Storage key: `` `backlog-detail-section-${itemId}-${sectionKey}` ``, value stored as
  the literal string `"true"` or `"false"` via `String(next)`.
- Read path (lines 20-28): `localStorage.getItem(...)` — if the key is **absent**
  (`stored === null`), returns `defaultExpanded` (the new `true`). If the key is
  **present**, returns `stored === "true"` regardless of what `defaultExpanded` is.
- **Critically: "no stored key" and "explicitly stored `false`" are already fully
  distinguishable** — `stored === null` is the only path that falls through to
  `defaultExpanded`. A user who has never touched the Description section's
  collapse/expand toggle has **no key at all** in localStorage for `description` (the
  component only calls `setExpanded`/`localStorage.setItem` in response to a user
  interaction — Radix's `onValueChange` on the `CollapsibleGroup`, wired via
  `handleGroupValueChange` at lines 383-389 — never on mount or on read).
- **Conclusion: there is no migration hazard.** Because collapsing/expanding is the only
  way a `description` key is ever written, every existing localStorage entry for
  `description` (if any exist from real usage) reflects a real, deliberate user action to
  collapse or expand it — not "the old hardcoded default's absence of an entry"
  (there never was a hardcoded-default write; the default was only ever read, never
  persisted). So FR2 ("stored per-item preference still wins over the new default") is
  already correctly implemented by existing code with **zero changes needed to
  `useSectionExpandState.ts` itself** — changing the literal default at
  `BacklogItemDetail.tsx:323` from `false` to `true` is sufficient, and any user who
  previously clicked to collapse Description on a given item keeps seeing it collapsed
  (their stored `"false"` wins), exactly as FR2 requires.

## 4. Acceptance Criteria rendering (FR3 — must stay provably unaffected)

`BacklogItemDetail.tsx:1146-1152`: rendered unconditionally as a plain `<div
className={styles.section}>` with an `<h3>` and `<AcCriteriaList criteria={item.acCriteria}
/>` — **not** a `CollapsibleSection`, not inside the `CollapsibleGroup`, no
localStorage/expand-state involvement at all, and physically positioned after the
`CollapsibleGroup` closes (group ends at line 1258, Acceptance Criteria block starts at
1146 — actually *before* the group in source order; Acceptance Criteria renders first,
then Actions, then modals, then the Description-containing `CollapsibleGroup`). No shared
state, hook, or component touches both Description and Acceptance Criteria, so a change
scoped to the `description` `useSectionExpandState` call and `DescriptionSection.tsx`'s
prop signature has zero code path that could affect Acceptance Criteria's rendering.
FR3's "must be provably unaffected" test should assert `AcCriteriaList`/the AC heading
renders with content regardless of Description's expand state (e.g. render with
Description explicitly collapsed via a stored `"false"` and confirm AC content is still
present, or simply confirm AC's DOM structure/testid is untouched by a snapshot/count
assertion) — there's no shared code to accidentally regress, so this is mostly a
regression-guard rather than something the implementation must actively preserve.

## 5. E2E test needing updates (FR5)

`tests/e2e/backlog-item-detail-redesign.spec.ts:107-110`:
```ts
// DescriptionSection defaults collapsed for every item (Story 3.1.3).
await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "false");
await detailPage.expandSection("description");
await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
```
Must become: assert `aria-expanded="true"` on first view (no stored preference), then
exercise collapse (a `collapseSection` helper likely already exists alongside
`expandSection` in `tests/e2e/pages/` — check the page-object class backing
`detailPage`) and re-expand, per FR5's explicit "exercise collapse/re-expand" wording.

## 6. Edge cases / unstated needs

- **Empty description**: `DescriptionSection.tsx:22-28` renders `<p>No description.</p>`
  as the empty state, inside the same `CollapsibleSection` body regardless of whether
  `item.description` is truthy. The expand/collapse default is orthogonal to content —
  the section should expand by default per FR1 even when the body will just show "No
  description." This matches how `PullRequestSection`/`SessionsSection` already default
  open regardless of whether they currently have content, so no special-casing is needed
  or consistent with existing patterns.
- **Very long descriptions**: no truncation/clamping exists in `DescriptionSection.tsx` —
  it renders the full `ReactMarkdown` output unconditionally once expanded. Expanding by
  default makes a very long description immediately visible/pushes other content down;
  this is an accepted UX tradeoff already implicit in the other `true`-default sections
  (e.g. `SessionsSection`, `PullRequestSection` can also be long) and out of scope per the
  requirements doc's explicit "does not touch Acceptance Criteria's rendering" scoping
  language — no truncation behavior is requested here either.
- **Pre-existing stored `false` preference**: covered in section 3 above — no migration
  risk exists; a stored `"false"` is unambiguously a deliberate past collapse action and
  correctly continues to win (FR2), which is exactly the desired behavior, not a bug to
  route around.
- **"First view" semantics**: `useSectionExpandState`'s `useState` initializer runs once
  per mount, and `BacklogItemDetail`'s call site is remounted `key={itemId}`-scoped (per
  the Story 3.1.5 comment at line 330, "Combined with Story 3.1.1's `key={itemId}` remount
  at the call site"). So "first view" effectively means "first time this item's detail
  panel mounts with no stored localStorage key for `description`" — every re-render while
  mounted keeps whatever the user has toggled; a fresh mount (navigating away and back)
  re-reads localStorage, and if still no key exists (user never toggled it) it opens
  expanded again. This satisfies "every time initially rendered, until the user makes a
  choice" — not strictly "once ever" — which matches FR1/FR2's literal wording ("no stored
  localStorage preference" as the trigger condition, not a one-time flag) and requires no
  extra "have I shown this before" tracking beyond what `useSectionExpandState` already
  does.
