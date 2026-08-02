# Architecture Review: backlog-description-prominence
**Date**: 2026-08-01
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (checked
`docs/adr/` — ADRs present start at 003, no ADR-000). No constitution to check against; no
constitution violations possible.

## Blockers

None.

## Concerns

None that rise to the level of a real risk given the scope. The plan is factually grounded
against the actual source (`Collapsible.tsx`, `BacklogItemDetail.tsx:314-323,1215`,
`useSectionExpandState.ts`, all eight sibling `detail/*Section.tsx` files, and the sole
production call site) and every claim checked out, with one factual overstatement noted
below as a nitpick rather than a concern — it doesn't change what code gets written, only a
sentence of justification.

## Nitpicks

- **Epic 1.2 / Pattern Decisions table / build-vs-buy.md "Fork or adapt" section** — the
  plan repeatedly frames `DescriptionSection` as "the one call site in the whole `detail/`
  family that doesn't thread `defaultExpanded` as a prop" / "the sole outlier among ~8
  sibling sections." This is not quite accurate: `PullRequestSection.tsx:31` also hardcodes
  `defaultExpanded={true}` directly on its `CollapsibleSection` rather than accepting it as
  a prop (verified — `PullRequestSectionProps` has no `defaultExpanded` field, unlike
  `NotesSection`, `PlanArtifactsSection`, `ProgressHistorySection`, `VersionControlSection`,
  `WorkflowHistorySection`, `LastReviewResultSection`, and `SessionsSection`, which all do).
  So there are two hardcoded outliers today, not one, and after this plan ships
  `PullRequestSection` becomes the sole remaining one. This doesn't affect any task's
  correctness — Approach B is still the right call, and the new `DescriptionSection` prop
  API is still consistent with 7 of 8 siblings — but the "sole outlier" / "~8 siblings"
  phrasing in the plan and in `research/build-vs-buy.md`'s "Fork or adapt" section overstates
  uniqueness slightly. Cosmetic only: worth a one-line correction if the plan doc is revised
  for other reasons, not worth blocking or re-planning for.
- **`DescriptionSectionProps.defaultExpanded` required, not optional** — Task 1.2.1a makes
  it a required `boolean` (no `?`), matching the convention in all 7 prop-threading siblings
  (`NotesSection`, `PlanArtifactsSection`, etc., all declare it as required). This is
  correct and intentional per the plan; noting it only to confirm it was checked, not to
  flag a problem — a required prop here is the right choice since the single production
  call site (`BacklogItemDetail.tsx:1215`) will always pass it, and making it optional would
  reintroduce the exact "silently disagrees with reality" hazard Epic 1.2 exists to remove.

## Verification Performed

Cross-checked every load-bearing factual claim in the plan against the actual repository
state (not just the plan's own prose):
- `Collapsible.tsx:127-177` — confirmed grouped-mode `defaultExpanded` is architecturally
  inert (drives nothing when `insideGroup`) and the dev-only divergence warning only fires
  when `defaultExpanded === true && !groupSaysOpen`; since the current unthreaded call site
  never passes `defaultExpanded` (defaults to `false`), the warning cannot currently fire
  either way — the plan's own framing of this as "purely internal-consistency, not
  user-visible" is accurate, not a functional necessity being mis-sold as one.
- `BacklogItemDetail.tsx:314-323` — `useSectionExpandState` call sites for all 9 sections,
  confirming the plan's line numbers and confirming `pull-request`/`sessions` are indeed the
  two existing `true`-default precedents cited.
- `BacklogItemDetail.tsx:1215` — sole call site of `<DescriptionSection item={item} />`;
  grepped the whole `web-app/src` tree and found no other production or storybook usage, so
  making `defaultExpanded` a required prop is safe.
- `useSectionExpandState.ts:8-13` — confirmed the localStorage-key/try-catch-fallback
  contract cited in requirements.md and the Pattern Decisions table.
- `DescriptionSection.test.tsx` (current) and `BacklogItemDetail.markdown.test.tsx`
  (current, including its `beforeEach`/`localStorage.clear()`/`getBacklogItem`/`baseItem`
  harness) — confirmed both match what the plan describes as their "before" state, so the
  diffs described in Tasks 1.3.1a/1.3.2a/1.4.1a/1.4.2a are accurate deltas.
- All 8 sibling `detail/*Section.tsx` files — confirmed which do/don't thread
  `defaultExpanded` as a prop (see Nitpicks above for the one correction).

## Scope Assessment

Confirmed this is exactly the XS scope claimed: two boolean-literal flips plus one prop
addition across two files, no new types, no new abstractions, no persistence/service-layer
work, no illegal-state surface added. Applied the three lenses and found nothing
disproportionate to invent here — Approach B (chosen) is the correctly-sized solution:
smaller than a redesign, and only marginally larger than the minimal Approach A in exchange
for removing genuinely stale/misleading code (the hardcoded `false` + stale docstring).
Approach C (content-conditional default) was correctly rejected as unnecessary complexity
for a requirement that explicitly forbids it. No PoEAA or GoF pattern is warranted or
proposed; none should be. Build-vs-buy verdict ("boolean-literal flip, no library/service
change") matches the plan's actual task list line for line.
