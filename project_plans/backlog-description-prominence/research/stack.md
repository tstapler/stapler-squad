# Research: Stack & Patterns — Backlog Description Prominence

## Versions (web-app/package.json)

| Package | Version |
|---|---|
| `next` | 15.3.2 |
| `react` | ^19.0.0 |
| `typescript` | ^5.9.3 |
| `jest` | ^30.2.0 |
| `@testing-library/react` | ^16.3.0 |
| `@testing-library/jest-dom` | ^6.9.1 |
| `@testing-library/user-event` | ^14.5.2 |
| `@playwright/test` | ^1.57.0 |

**No new dependency needed.** This is a pure default-value + prop-threading change using primitives (`useState`-backed hook, Radix `Accordion` via the existing `CollapsibleSection` wrapper) already in place. Confirmed no gap.

## Jest config (web-app/jest.config.js)

- `preset: "ts-jest"`, `testEnvironment: "jest-environment-jsdom"`.
- `roots: ["<rootDir>/src"]`, `testMatch: ["**/__tests__/**/*.ts?(x)", "**/?(*.)+(spec|test).ts?(x)"]` — confirms `DescriptionSection.test.tsx` and `BacklogItemDetail.markdown.test.tsx` are picked up automatically (colocated `.test.tsx` files under `src/`).
- `.css.ts` / `.module.css` files are mocked (`styleMock.js` / `identity-obj-proxy`) — no real style output to assert on, consistent with constraint "no CSS changes."

## The `defaultExpanded` prop-threading pattern (already established for 8 sibling sections)

Two concrete examples, file:line:

1. **`web-app/src/components/backlog/detail/NotesSection.tsx:8-38`** — `NotesSectionProps` declares `defaultExpanded: boolean;` (required, no default value in the destructure), and the component passes it straight through: `<CollapsibleSection sectionKey="notes" title="Notes" defaultExpanded={defaultExpanded}>`. The section's own docstring (lines 20-25) explains the *policy* for what value the parent should pass ("expanded only when `item.notes` is already non-empty") — the section itself has no opinion, it just renders whatever it's given.

2. **`web-app/src/components/backlog/detail/SessionsSection.tsx:23-30,49,86-90`** — identical shape: `SessionsSectionProps.defaultExpanded: boolean` required prop, forwarded verbatim into `CollapsibleSection`. Docstring again states policy ("Default-expanded... per requirements' emphasis on session inspectability") but the value itself is caller-supplied.

**Parent-side wiring** (`web-app/src/components/backlog/BacklogItemDetail.tsx:314-323`): one `useSectionExpandState(itemId, "<key>", <staticDefault>)` call per section, each producing `[expanded, setExpanded]`, then the `expanded` value is passed as the `defaultExpanded` prop at the JSX call site (e.g. line 319 `sessionsExpanded` → line ~1236 `<SessionsSection ... defaultExpanded={sessionsExpanded} />`). Existing static defaults per section: `reviewing`→false, `last-review-result`→false, `pull-request`→**true**, `plan-artifacts`→false, `version-control`→false, `sessions`→**true**, `workflow`→false, `progress-history`→false, `notes`→false, **`description`→false (line 323 — this is the value to flip to `true`)**.

`DescriptionSection` is the one holdout: its own prop interface (`DescriptionSection.tsx:10-12`) has no `defaultExpanded` field at all, and it hardcodes `defaultExpanded={false}` directly in JSX (`DescriptionSection.tsx:20`), bypassing the `descriptionExpanded` state value computed at `BacklogItemDetail.tsx:323` entirely — the call site at `BacklogItemDetail.tsx:1215` is just `<DescriptionSection item={item} />`, never passing the computed value through. This is exactly the gap the requirements doc identifies and the pattern above is the fix: add `defaultExpanded: boolean` to `DescriptionSectionProps`, forward it into `CollapsibleSection`, and change the parent's static default at line 323 from `false` to `true`, then pass `descriptionExpanded` at the call site (mirroring `NotesSection`/`SessionsSection` exactly).

## `useSectionExpandState` hook (`web-app/src/lib/hooks/useSectionExpandState.ts`)

Signature: `useSectionExpandState(itemId: string, sectionKey: string, defaultExpanded: boolean): [boolean, (expanded: boolean) => void]`.

- localStorage key format (line 3-5): `` `backlog-detail-section-${itemId}-${sectionKey}` `` — for `sectionKey="description"` this is exactly `backlog-detail-section-${itemId}-description`, matching requirement #2 verbatim already (no key-format change needed — only the third-argument default at the call site, line 323, needs to flip `false` → `true`).
- Read: `useState` lazy initializer checks `localStorage.getItem(key)`; if `null` (never-stored), falls back to the passed `defaultExpanded`; otherwise returns `stored === "true"`. This means requirement #1 (expanded by default when nothing stored) and #2 (stored preference wins) are both satisfied purely by changing the literal default argument — the hook's precedence logic already does the right thing.
- Try/catch wraps both read and write paths (private-browsing/quota-disabled fallback), matching `RecentFilesSection.tsx`'s defensive pattern per the hook's own docstring.

## `CollapsibleSection` / `CollapsibleGroup` (`web-app/src/components/ui/Collapsible.tsx`)

- `defaultExpanded?: boolean` prop (line 84) is documented as "only used for standalone (non-grouped) usage" — inside a `CollapsibleGroup`, initial state is controlled by the group's own `defaultValue`/`value`, and a passed `defaultExpanded` that *diverges* from the group's state triggers a dev-time console warning (lines 139-157, "defaultExpanded/onExpandedChange are ignored inside a CollapsibleGroup").
- **Relevant for implementation**: confirm whether `DescriptionSection`'s `CollapsibleSection` is standalone or wrapped in the page-level `CollapsibleGroup` in `BacklogItemDetail.tsx` — if grouped, the `descriptionExpanded` state must also be reflected into that group's `value`/`defaultValue` array (consistent with how `sessionsExpanded`/`notesExpanded` etc. already flow into the group), otherwise the same divergence warning NotesSection/SessionsSection avoid would fire for Description. This is implementation-detail wiring, not a new pattern — follow whatever the other 8 sections already do at their `BacklogItemDetail.tsx` call sites.

## Acceptance Criteria section (must remain provably unaffected — requirement #3)

`web-app/src/components/backlog/BacklogItemDetail.tsx:1146-1149` — Acceptance Criteria is rendered as a plain block (comment `{/* Acceptance Criteria */}`), **not** wrapped in any `CollapsibleSection`, with no `defaultExpanded`/expand-state hook involved at all. It is structurally isolated from the `DescriptionSection`/`useSectionExpandState` change — confirming the requirements doc's claim that this item's changes cannot touch AC's rendering. A regression test should assert AC's container is present and unconditionally visible (no `aria-expanded`/collapsed state) after the Description default flips, e.g. in `BacklogItemDetail.markdown.test.tsx` or `BacklogItemDetail.test.tsx`.

## Existing test files to update (per constraints)

- **`web-app/src/components/backlog/detail/DescriptionSection.test.tsx`** (current, read in full) — the first test, `DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders` (lines 29-35), currently renders `<DescriptionSection item={makeItem()} />` with **no `defaultExpanded` prop** and asserts `aria-expanded="false"`. Once the prop is added and the parent always passes a value, this test needs to either (a) explicitly pass `defaultExpanded={false}` to keep testing the collapsed case, and add a new `..._should_DefaultExpanded_When_defaultExpandedTrue` (or similar) test passing `defaultExpanded={true}` and asserting `aria-expanded="true"` — matching the sibling test pattern implied by `NotesSection`/`SessionsSection` (no visible `.test.tsx` for those was read, but the rename convention `<Component>_should_<effect>_When_<condition>` is visible in this file already, e.g. line 29).
- **`tests/e2e/backlog-item-detail-redesign.spec.ts:107-110`** — currently asserts the OLD behavior directly: comment "DescriptionSection defaults collapsed for every item (Story 3.1.3)" then `expect(...).toHaveAttribute("aria-expanded", "false")` before calling `detailPage.expandSection("description")`. Per requirement #5 this block must invert: assert `aria-expanded="true"` on first load (no prior localStorage), then exercise collapse via a `collapseSection`-style helper (check `tests/e2e/pages/` for an existing helper — `expandSection` already exists on the page object, a symmetric `collapseSection` may need adding or the same click handler reused) and re-expand.
- **`web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`** and **`BacklogItemDetail.test.tsx`** — named explicitly in requirement #6 as targeted suites that must keep passing; likely contain assertions about Description's default state or AC's static rendering that should be spot-checked/extended for the new default and the AC-unaffected proof.

## Summary of the concrete diff shape (confirmed via reading, not guessed)

1. `DescriptionSection.tsx`: add `defaultExpanded: boolean` to `DescriptionSectionProps`, destructure it, pass to `CollapsibleSection` instead of hardcoded `false`; update stale docstring (currently says "collapsed by default").
2. `BacklogItemDetail.tsx:323`: change `useSectionExpandState(itemId, "description", false)` → `useSectionExpandState(itemId, "description", true)`.
3. `BacklogItemDetail.tsx:1215`: change `<DescriptionSection item={item} />` → `<DescriptionSection item={item} defaultExpanded={descriptionExpanded} />` (mirroring `NotesSection`/`SessionsSection`/`PlanArtifactsSection` call sites already visible at lines 1218/1236/1249).
4. If Description's `CollapsibleSection` is inside the page-level `CollapsibleGroup`, thread `descriptionExpanded` into that group's controlled `value`/`defaultValue` set the same way the other grouped sections do (needs a quick check of the group's `value` array construction near where `sectionExpandEntries`/similar aggregation happens, per the comment block in `SessionsSection.tsx:92-106` referencing "sectionExpandEntries").
