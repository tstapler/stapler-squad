# Pitfalls: flipping DescriptionSection's default-expanded state

## 0. CRITICAL — the requirements doc's premise about *where* the default lives is wrong

The requirements doc says: "DescriptionSection is a CollapsibleSection with
`defaultExpanded={false}` hardcoded internally. This item flips the default to
true and threads it as a prop like sibling sections."

That hardcoded `defaultExpanded={false}` in
`web-app/src/components/backlog/detail/DescriptionSection.tsx:20` is **dead
code today** and would remain dead code if simply flipped to `true`. Here's why:

- `DescriptionSection` is always rendered as a child of `CollapsibleGroup`
  (`BacklogItemDetail.tsx:1185` / `:1215`, `<CollapsibleGroup value={openSectionKeys}
  onValueChange={handleGroupValueChange}>`), i.e. the group is **controlled**
  (`value` is set, not `defaultValue`).
- In `Collapsible.tsx`'s `CollapsibleSection` (`web-app/src/components/ui/Collapsible.tsx:127-177`),
  when `insideGroup` is true, the function returns early via `CollapsibleItem`
  and **never reads `defaultExpanded` at all** — it's not passed to Radix's
  `Accordion.Item`/`Accordion.Root` in grouped mode. `defaultExpanded` is only
  wired to Radix's own `defaultValue` in the standalone (non-grouped) branch
  (lines 165-176), which DescriptionSection never hits.
- The actual value driving "description" section's initial open/closed state
  is `descriptionExpanded` from
  `useSectionExpandState(itemId, "description", false)` at
  **`BacklogItemDetail.tsx:323`**, which feeds into `sectionExpandEntries`
  (line 374) → `openSectionKeys` (line 382) → the group's controlled `value`.

**Implication for the plan**: the real fix is changing the third argument at
`BacklogItemDetail.tsx:323` from `false` to `true` — *not* (only) editing
`DescriptionSection.tsx`'s internal `CollapsibleSection` call. Requirement 4
("DescriptionSection accepts defaultExpanded prop instead of hardcoding it")
should still be done for API parity with siblings (`NotesSection` takes a
required `defaultExpanded: boolean` and forwards it — see below), and to stop
the prop being misleadingly dead, but doing *only* that without touching line
323 will not change any observable behavior at all — `make quick-check` would
pass but the feature would be a no-op in the running app. Concretely, both of
these need to happen together:

1. `DescriptionSection.tsx`: add `defaultExpanded: boolean` to
   `DescriptionSectionProps` (required, matching `NotesSectionProps` — see
   §3), forward it to `CollapsibleSection`'s `defaultExpanded` prop, and
   delete the stale "collapsed by default (Story 3.1.3, Task 3.1.3a)"
   docstring on lines 14-17.
2. `BacklogItemDetail.tsx:323`: change
   `useSectionExpandState(itemId, "description", false)` →
   `useSectionExpandState(itemId, "description", true)`.
3. `BacklogItemDetail.tsx:1215`: change `<DescriptionSection item={item} />`
   to `<DescriptionSection item={item} defaultExpanded={descriptionExpanded} />`
   (same pattern as `NotesSection` at line ~1249:
   `defaultExpanded={notesExpanded}`).

Without step 2, step 1/3 alone changes nothing user-visible (dead-code prop
threading only). Without steps 1/3, step 2 alone works behaviorally (since
the group's controlled `value` is the actual source of truth) but leaves
`DescriptionSectionProps` without the prop the requirements doc explicitly
asks for, and leaves the stale docstring in place.

## 1. localStorage "no preference" vs "explicit false" — NOT a risk here

Read `useSectionExpandState.ts` fully (44 lines). `localStorage.getItem`
returning `null` is correctly distinguished from a stored `"false"`:
```ts
const stored = localStorage.getItem(storageKey(itemId, sectionKey));
if (stored === null) return defaultExpanded;
return stored === "true";
```
`setExpanded` (which writes to localStorage) is only invoked in response to
explicit user interaction — clicking a header (goes through
`CollapsibleGroup`'s `onValueChange` → `handleGroupValueChange` →
`setExpanded` only when the new boolean differs from current) — or by the
one-time "Story 3.1.5" auto-expand effect at `BacklogItemDetail.tsx:337-368`,
which only ever calls `setXExpanded(true)` (never `false`), and only when
`hasStoredPreference(...)` is false. That effect does **not** cover
`"description"` at all (it only auto-opens `reviewing`,
`last-review-result`, `version-control`, `notes`) — so there is no code path
in this component that writes anything to the `description` localStorage key
except a real user click. **No component behavior writes `"false"` on mount
before user interaction.** Flipping the default is safe from this angle: any
item a user has never expanded/collapsed description on will have no stored
key and will pick up the new default (`true`) correctly; any item where a
user previously explicitly collapsed it (stored `"false"`) will keep it
collapsed, satisfying requirement 2.

## 2. `expandDescription()` helper will invert behavior — CONFIRMED, must fix

`web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx:18-21`:
```ts
async function expandDescription() {
  const header = await screen.findByTestId("collapsible-header-description");
  fireEvent.click(header);
}
```
This unconditionally clicks the header — a **toggle**, not an "ensure
expanded" action. It currently works because the section starts collapsed
(click → expand). Once the default flips to `true`, this same click will
**collapse** the section, and the subsequent
`await screen.findByTestId("backlog-description-rendered")` in each caller
will fail/timeout because the markdown body is removed from the DOM while
collapsed (Radix `Accordion.Content` unmounts, not just hides).

**All 3 call sites found** (only usages of `expandDescription` in the repo —
confirmed via grep, no other file references it):
- `BacklogItemDetail.markdown.test.tsx:128` — "renders bold text and links…"
- `BacklogItemDetail.markdown.test.tsx:144` — "renders an embedded image"
- `BacklogItemDetail.markdown.test.tsx:158` — "never executes an injected
  `<script>` tag…"

Required fix: either delete the `await expandDescription()` calls entirely
(description is now expanded by default, so `findByTestId("backlog-description-rendered")`
will resolve without any click), or rewrite the helper to be idempotent
("ensure expanded" — check `aria-expanded` before clicking) if the plan wants
to keep it defensive against future default changes. Given requirement 1 (no
stored localStorage preference in these tests — confirmed by
`beforeEach(() => localStorage.clear())` at line 118), deleting the calls is
simplest and matches "expanded by default on first view."

## 3. `DescriptionSection.test.tsx` — will fail to compile once prop is required

Read fully (52 lines). All 3 tests instantiate `<DescriptionSection item={makeItem()} />`
with **no `defaultExpanded` prop**:
- Line 30: `DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders` —
  this test's *name and assertions* are actively about collapsed-by-default
  behavior (asserts `aria-expanded="false"` and that the rendered markdown
  testid is absent) and must be rewritten/renamed for the new default, not
  just have a prop added.
- Line 37: "reveals the markdown description once expanded" — clicks the
  header first; if instantiated with `defaultExpanded={true}` this click
  would collapse it (same toggle problem as §2). If instantiated with
  `defaultExpanded={false}` explicitly, this test remains valid as an
  "explicit collapsed-preference / manual expand" case and needs no logic
  change, only the new required prop added.
- Line 45: "shows an empty-state message when there is no description" — same
  click-then-assert pattern as line 37; same fix.

**TypeScript will fail to compile** once `defaultExpanded` is added as a
required (non-optional) field to `DescriptionSectionProps`, confirmed by
checking the sibling pattern: `NotesSectionProps` at
`web-app/src/components/backlog/detail/NotesSection.tsx:13` declares
`defaultExpanded: boolean;` **without** a `?`, i.e. required, and
`NotesSection.test.tsx` presumably passes it explicitly at every call site
(not verified line-by-line here, but the type signature is unambiguous: no
default value in the destructured param list either — `NotesSection.tsx:31`
takes `defaultExpanded` straight from props, no `= false` fallback). So
`DescriptionSectionProps` should also make `defaultExpanded: boolean`
required (not `defaultExpanded?: boolean`) for consistency, and this is a
**required, not optional**, test-file update — `make quick-check` /
`tsc` will hard-fail otherwise on all 3 call sites in
`DescriptionSection.test.tsx`, in addition to the runtime failures described
in §2.

## 4. CollapsibleGroup / Radix double-sourcing — resolved above (§0), no NEW risk beyond what's already documented

This is the same fact as §0, restated for the specific question asked:
`defaultExpanded` is **not** double-sourced — inside a group it is not passed
to Radix at all (dead), so there's no risk of Radix's internal open state
disagreeing with a `defaultExpanded` boolean. The *actual* risk this item
must guard against is the opposite: forgetting to also update
`BacklogItemDetail.tsx:323`'s `useSectionExpandState` call, since that (not
`DescriptionSection`'s own prop) is what Radix's controlled `value` ultimately
reflects. `CollapsibleSection`'s own dev-mode `console.warn` (Collapsible.tsx
:150-156) only fires when `defaultExpanded` is `true` while the group says
the section is closed (`defaultExpandedDiverges = defaultExpanded &&
!groupSaysOpen`) — so *if* steps in §0 are done together (both the
`useSectionExpandState` default and the prop threaded through with the same
resolved value), no warning fires and everything stays consistent. If only
the component-level prop is flipped without touching line 323, no warning
fires either (since a `false`-vs-open mismatch is not the case the warning
checks for) — this is exactly why the bug in §0 is easy to miss in review:
nothing crashes or warns, it just silently doesn't do anything.

## 5. Acceptance Criteria always-visible layout — unaffected, low risk

`Acceptance Criteria` (`BacklogItemDetail.tsx:1146-1152`) and `ActionsSection`
are rendered **before** and **outside** the `CollapsibleGroup`
(`BacklogItemDetail.tsx:1170-1185`'s comment explicitly documents this:
"PlanningSection/ActionsSection are primary, always-visible content and
intentionally sit outside the group"). They share no state, hook call, or
DOM ancestry with `DescriptionSection`/`descriptionExpanded`, so flipping the
description default cannot affect AC's rendering or layout by construction.
Requirement 3 ("Acceptance Criteria's always-visible rendering/layout must be
provably unaffected — directly asserted test") is satisfiable with a
straightforward `BacklogItemDetail.test.tsx` assertion that the AC heading /
`AcCriteriaList` content is present and not gated behind any
`collapsible-header-*` testid — no new coupling was found that would make
this fail.

One adjacent (not-broken, but worth noting) test:
`BacklogItemDetail.test.tsx:995-1023`, the Task 3.1.4i keyboard-roving-tabindex
test, focuses `collapsible-header-description` and asserts `ArrowDown` moves
focus to `collapsible-header-plan-artifacts`. This is about Radix's
roving-tabindex among headers (works identically regardless of open/closed
state) and does not depend on description's expanded/collapsed state — no
change needed, confirmed by reading Radix's Accordion Trigger behavior
implied by the group wiring (headers stay focusable/tabbable regardless of
content open state).

## 6. e2e spec — confirmed exact edit needed

`tests/e2e/backlog-item-detail-redesign.spec.ts:107-110`:
```ts
// DescriptionSection defaults collapsed for every item (Story 3.1.3).
await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "false");
await detailPage.expandSection("description");
await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
```
Matches requirement 5 exactly: remove the `"false"` assertion and stale
comment, assert `"true"` immediately (expanded by default), then exercise
collapse (click) → assert `"false"`, then re-expand (click) → assert `"true"`
to cover both directions per the requirement's "exercise collapse/re-expand."
`detailPage.expandSection()` (`tests/e2e/pages/BacklogItemDetailPage.ts:44-`)
should be checked for whether it's also a raw toggle-click (like
`expandDescription()` in the Jest test) or whether it's guarded/idempotent —
if it unconditionally clicks, a `collapseSection()`-equivalent (or reuse of
the same method, since it's a toggle either way) is needed for the
collapse step; grep found no dedicated `collapseSection` helper in
`BacklogItemDetailPage.ts`, so the same `expandSection` method (a raw click)
can likely serve as `toggleSection` for both directions, but confirm its
current implementation logic (not just its name) before reusing it for
"collapse."

## 7. Lint / registry / CI pitfalls — low risk, none blocking found

- No `docs/registry/features/*` entry exists for `DescriptionSection` as a
  distinct "feature" separate from the existing backlog-item-detail feature
  set (this is a prop/behavior change to an existing component, not a new
  RPC or new page/component per `.claude/rules/feature-registry.md`'s
  "New UI feature" trigger) — `make registry-generate` is unlikely to flag
  anything, but run it anyway per the standing rule and check
  `git diff docs/registry/` is empty (expected) before considering the PR
  complete.
- No `eslint-plugin-react` `require-default-props`/`prop-types` rule found in
  `web-app/.eslintrc.json` (grep found nothing) — making `defaultExpanded`
  required-with-no-default on `DescriptionSectionProps` (matching
  `NotesSectionProps`'s pattern) should not trip any lint rule.
- Docstring update (`DescriptionSection.tsx:14-17`) is prose only, no format
  CI check found for JSDoc content — `gofmt`/Go tooling doesn't apply here
  (frontend-only change), so `make lint`'s Go-side checks are unaffected;
  only `web-app`'s own `eslint`/`tsc` (via jest's ts-jest or a separate
  typecheck step) matter, and those are covered by §3's required-prop
  compile-break analysis.
