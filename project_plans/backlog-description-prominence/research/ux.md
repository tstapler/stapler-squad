# UX Research: Description Prominence in Backlog Item Detail

**Item**: `5b0e1d57-5244-4847-9b61-029b147f6aab` — "Backlog description should be more prominent."
**Scope**: single default-value flip (`defaultExpanded` for the Description
section), not a redesign. This doc grounds that flip in UX principle so the
decision isn't just "it felt right."

## Code baseline (for context, not the point of this doc)

- `DescriptionSection.tsx` renders inside `CollapsibleGroup` (Radix
  `Accordion.Root type="multiple"`). Per `Collapsible.tsx:137-163`, a
  `CollapsibleSection`'s own `defaultExpanded` prop is **architecturally
  dead when rendered inside a group** — the group's own `defaultValue`/`value`
  array is what actually determines initial open state. That means the real
  fix point is wherever `BacklogItemDetail.tsx` builds the `CollapsibleGroup`'s
  `defaultValue` array from `useSectionExpandState(itemId, "description", false)`
  (line ~323) — not `DescriptionSection.tsx`'s own prop. Worth flagging for
  the planning phase since the requirements doc's file/line pointer describes
  the symptom correctly but the literal prop to flip lives one level up.
- `AcCriteriaList` is rendered outside the group entirely, unconditionally
  visible, no collapse affordance at all.
- Empty-description state already renders `<p>No description.</p>`
  (`DescriptionSection.tsx:26-28`) — this doc's edge-state question (4) is
  about whether that state should also auto-expand, not about building new
  empty-state UI.

## 1. Comparable UX patterns — is body/description ever collapsed by default?

Across mainstream issue trackers, the primary description/body field is
**never** collapsed by default — it renders immediately below the title,
full width, no click required:

- **Linear**: issue description is the first thing rendered in the detail
  panel, directly under the title, always expanded. Metadata (labels,
  project, cycle, sub-issues, activity feed) lives in a secondary sidebar or
  below-the-fold activity log — that's what gets progressively disclosed,
  not the description.
- **GitHub Issues**: the issue body (rendered Markdown) is the first content
  block, always visible. Comments below it are sequential, not
  collapsed. The only things collapsed by default are secondary metadata
  (e.g. "N participants", collapsed long comment threads after they exceed a
  length threshold, resolved conversation threads on a PR diff).
  Never the issue body itself.
- **Jira**: same shape — Description field is a top-level, always-expanded
  panel in the issue view; the collapsible/tab-based real estate goes to
  secondary panels ("Activity", "Development", "Linked issues").

The pattern is consistent: **progressive disclosure is reserved for content
that is secondary to the task at hand or genuinely voluminous** (long
activity logs, linked-issue graphs, git branch/PR integration panels) —
never for the field that answers "what is this ticket actually about."
This matches general UX guidance: NN/g's accordion guidance says accordions
work "when content is conceptually secondary but still relevant" (e.g. a
checkout flow's billing address vs. the always-visible shipping address) —
the primary content stays visible, secondary content collapses ([NN/g,
Accordions on Desktop](https://www.nngroup.com/articles/accordions-on-desktop/)).

**Signals for show-by-default vs. collapse-by-default**, derived from this
comparison:

| Signal | Show by default | Collapse by default |
|---|---|---|
| Primacy to the task ("what is this item about") | Yes | No |
| Populated at creation time, near-100% non-empty in practice | Yes | — |
| Read on nearly every visit (not just occasionally) | Yes | — |
| Length/scannability (short paragraph vs. long log) | Short-to-medium: show | Long/rarely-needed: collapse |
| Frequently empty / filled in later by a separate workflow step | — | Yes |

Applied to this item: Description scores "show" on every signal (primary,
populated up front, read every time, typically short-to-medium markdown).
Acceptance Criteria currently scores "collapse" on the "frequently empty at
creation" signal but is out of scope here (see Out of Scope in
requirements.md) — flagging only because it's the same signal, applied
consistently, that would justify a symmetric follow-up.

## 2. User mental model — what does the user expect to see first?

When a user (or Tyler specifically, as author/triage) opens an item, the
implicit question is "what is this asking me to do?" — answered by the
description, not by acceptance criteria (which formalize *how to know it's
done*, a secondary/later-stage concern) or by any other collapsible
metadata. Requiring a click to see the one field that's actually populated
inverts the expected information scent: the always-visible section
(Acceptance Criteria) is disproportionately likely to say "No acceptance
criteria defined," while the actually-useful content sits behind a toggle.
This is a textbook violation of Nielsen's "recognition rather than recall"
and "match between system and the real world" heuristics — the UI's default
visibility doesn't match the data's actual population pattern.

## 3. Accessibility (WCAG / ARIA) implications

Radix `Accordion` (used here in `type="multiple"` mode via `CollapsibleGroup`)
manages `aria-expanded`, `aria-controls`, and content `region`
labelling automatically regardless of initial open state — defaulting a
section open vs. closed is purely a `defaultValue`/`value` array change and
carries **no extra ARIA wiring burden**; Radix's accessibility contract is
identical either way (confirmed via Radix's accessibility docs: triggers
get `aria-expanded="true|false"` managed automatically, content is a
labelled `region` announced on open).

Practical accessibility effects of defaulting Description open:

- **Screen reader users**: encounter the description content immediately
  when navigating into the detail panel's content region, instead of
  needing to locate and activate the Description trigger first. This is a
  net accessibility *improvement* for the common case (non-empty
  description) — one fewer required interaction to reach primary content,
  consistent with WCAG's general preference for minimizing operations
  needed to complete a task (2.4.x — no single explicit success criterion
  mandates this, but it aligns with the intent of "Consistent Help" /
  minimizing unnecessary steps).
- **Tab order**: unaffected. Radix's roving-tabindex accordion nav
  (per the `ADR-027` comment in `Collapsible.tsx:6-10`) spans all sibling
  headers regardless of which are open; opening Description by default
  doesn't reorder or add tab stops — it only changes whether its content
  node is mounted (Radix removes closed content from the DOM, not just
  visually hides it, per the `CollapsibleSection` docstring at
  `Collapsible.tsx:117-120`), so an open-by-default Description means one
  more content region is present in the accessibility tree from first
  render, same as it would be after a sighted user's first click today.
- **No new violations introduced**: this is a `defaultValue` array change
  only, no markup/ARIA-attribute changes, so it carries no new WCAG risk in
  either direction — the only accessibility delta is the reduced-clicks
  benefit above.

## 4. Error/edge state — empty description

`DescriptionSection.tsx` already renders `<p className={styles.emptyText}>No
description.</p>` when `item.description` is falsy. The open question is
whether that empty state should *also* auto-expand, or whether expanding to
show one line of "No description." is wasted vertical space.

Recommendation: **expand unconditionally, regardless of content presence** —
i.e. don't make `defaultExpanded` conditional on `item.description` being
non-empty. Reasons:

- Consistency with Acceptance Criteria's current (in-scope-preserved)
  behavior: AC is always visible today even when it renders "No acceptance
  criteria defined." — a single-line empty state in an always-visible
  section is already the established pattern in this exact UI; making
  Description's empty state behave differently (collapsed) would be an
  inconsistency, not a saving.
- Implementation simplicity: an unconditional `defaultExpanded`/default
  `openKeys` value — a single static default passed once — is simpler, has
  no extra render-time branching, and avoids a flicker/layout-shift edge
  case where the section would need to change its expanded state reactively
  if `item.description` becomes populated later without a full remount.
- The wasted-space cost of one line ("No description.") is low compared to
  the cost of an inconsistent, content-dependent collapse rule that a future
  reader of the code has to reverse-engineer.
- Empty-state visibility is itself informative: seeing "No description."
  immediately (rather than needing to expand to discover the item has no
  description) is exactly the same "reduce required clicks to see the
  state of the primary field" argument driving the whole fix — it applies
  equally to the empty case.

## 5. Jobs-to-be-done — what job does this serve for Tyler?

Tyler's JTBD when opening a backlog item from the list (whether authoring,
triaging, or resuming work) is: **"Let me quickly confirm what this item is
about before I decide what to do with it"** — a fast recognition/context-load
step, typically followed by triage/routing/dispatch decisions, not a
close-read of every possible field. The functional job is speed and
low-friction context recovery, particularly for a **solo user** operating
across desktop and mobile (per the mobile+desktop UX memory) where every
extra tap has proportionally higher cost on a touch interface than a desktop
click. An extra click to reveal the one field that's populated ~100% of the
time is friction with no corresponding benefit (nothing is being protected
from information overload, since AC is already always-visible and typically
near-empty) — it's collapsed-by-default machinery applied to content that
doesn't need protecting, which is the definition of unwarranted progressive
disclosure per IxDF/NN·g guidance above.

## Summary recommendation

Flip Description's effective default-open state to `true` for
users/items with no stored preference, leaving `useSectionExpandState`'s
persistence contract (and Acceptance Criteria's always-visible behavior)
untouched, per requirements.md's constraints. This is supported by: (1)
comparable-product convention (description always visible, metadata
collapsed), (2) the user's actual mental model on opening an item, (3) a
neutral-to-positive accessibility read (fewer clicks to reach primary
content, no new ARIA burden), (4) applying the same open-by-default rule
unconditionally including the empty state, for consistency with the
existing Acceptance Criteria empty-state pattern, and (5) directly serving
the fast-context-recovery job Tyler is doing when triaging.

## Sources

- [NN/g — Accordions on Desktop: When and How to Use](https://www.nngroup.com/articles/accordions-on-desktop/)
- [IxDF — What is Progressive Disclosure?](https://ixdf.org/literature/topics/progressive-disclosure)
- [Radix UI — Accessibility](https://www.radix-ui.com/primitives/docs/overview/accessibility)
