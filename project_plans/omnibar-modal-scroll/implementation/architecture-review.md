# Architecture Review: omnibar-modal-scroll
**Date**: 2026-07-28
**Verdict**: CLEAN

## Constitution Violations
- `docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (checked `docs/adr/` listing). No constitution to check against — skipped.

## Scope Calibration

This is a Complexity-1, 5-line CSS-only change: add `maxHeight: "80vh"`, `display: "flex"`, `flexDirection: "column"` to `.modal`, and `overflowY: "auto"`, `flex: 1`, `minHeight: 0` to `.body`, in `web-app/src/components/sessions/Omnibar.css.ts`. It copies a pattern already shipped in two sibling files. Verified directly against the source:

- `ResumeSessionModal.css.ts` `.modal` (lines 28-39) already has `maxHeight: "80vh"`, `display: "flex"`, `flexDirection: "column"`.
- `WorkspaceSwitchModal.css.ts` `.modal` (lines 25-36) has the identical shape plus `overflow: "hidden"`.
- Current `Omnibar.css.ts` `.modal` (lines 42-58) has none of the three properties; `.body` (lines 116-121) has none of `overflowY`/`flex`/`minHeight`. The plan's line-number claims match the actual file.
- `OmnibarCreationPanel.tsx`: `body` div closes at line 706, an `errorClass` div sits as a sibling at line 709, `footer` div is a sibling at line 712 — confirms the plan's FR2 claim that `.body` and `.footer` are siblings within `.modal`, so a flex-column `.modal` with `flex:1`/`minHeight:0` on `.body` correctly leaves `.footer` pinned outside the scroll region.

Given this, most of the eleven-item checklist (aggregate boundaries, persistence patterns, API contracts, GoF creational/structural patterns, repository/service-layer concerns, DDD entities/value objects) is legitimately N/A — there is no domain logic, no new type, no new component, no state machine, and no cross-layer boundary involved. Applying those lenses here would manufacture findings against a stylesheet property addition. The two questions worth answering are the ones the assignment calls out explicitly:

### 1. Does the plan correctly keep `.tsx` unchanged (NFR1)?
Yes. Task 1.1.1a/b touch only `Omnibar.css.ts`, and only the `modal` and `body` exports. The plan's own AC5 acceptance criterion requires a diff review confirming no `.tsx` file and no other export changed. This is correctly scoped — no markup restructuring is needed because `.body`/`.footer` are already siblings (verified above), so the flex-column parent + `flex:1`/`minHeight:0` child is sufficient without touching JSX.

### 2. Is the plan correctly scoped to only `.modal`/`.body`, leaving `.overlay`/`.footer` untouched?
Yes, and this is the right analog to "layer coupling" for a CSS tree: `.overlay` owns positioning/backdrop concerns (`position: fixed`, centering, `paddingTop: 10vh`) and `.footer` owns its own layout — neither needs to change for a height-cap-and-inner-scroll fix, and touching them would risk the exact kind of untargeted blast radius NFR1/AC4/AC5 are designed to prevent. The plan explicitly calls out leaving `overflow: "hidden"` on `.modal` alone (it still clips the rounded-corner container) rather than removing it, which is correct — removing it would be unnecessary and could let the flex-column overflow before the inner `.body` scrollbar engages depending on paint order.

### 3. Is the "Alternative Rejected" reasoning (deferring the `ui/Modal.tsx` primitive migration) architecturally sound?
Yes, with one thing worth tracking (see Concerns). Verified `web-app/src/components/ui/Modal.css.ts`: it uses a *different* shape from the Resume/WorkspaceSwitch/Omnibar pattern — a single `content` region with `maxHeight: "85vh"` + `overflowY: "auto"` directly on itself (no separate flex-column `.modal`/scrollable-`.body`/pinned-`.footer` split), because `ui/Modal.tsx` doesn't need to keep a footer pinned outside scroll for its current call sites. So migrating `Omnibar.tsx` to `ui/Modal` would not be a drop-in swap of an identical existing pattern — it would require either extending `ui/Modal`'s API to support a pinned-footer variant, or accepting a UX shift. That's real design work, correctly out of scope for a bug fix whose NFR1 is "CSS-only, no `.tsx` changes." Bundling a primitive-consolidation refactor into this fix would violate the Complexity-1 sizing and introduce regression risk across `Omnibar.tsx`'s hand-rolled `role="dialog"` markup for no bug-fix benefit. Symptom-matching the proven flex-column shell (already independently reimplemented twice) is the right call here — the type/pattern consolidation is a separate, larger decision that deserves its own review, not a rider on a scroll-clamp fix.

## Blockers
- None.

## Concerns
- None. (The `ui/Modal` consolidation question is noted above and in Nitpicks as a forward-looking observation, not a defect in this plan.)

## Nitpicks
- This fix will bring the "capped-height flex-column modal + `flex:1`/`minHeight:0` scrollable body + pinned footer" shell to its **third** independent implementation (`ResumeSessionModal.css.ts`, `WorkspaceSwitchModal.css.ts`, now `Omnibar.css.ts`), on top of `ui/Modal.css.ts`'s related-but-distinct single-region scroll shape. Rule-of-three is already past. Worth a follow-up backlog item to extract a shared `modalShell`/`scrollableModalBody` recipe (or extend `ui/Modal` with a pinned-footer variant) once there's a reason to touch modal styling again — not blocking, and correctly deferred per the plan's own reasoning.
- The `errorClass` div in `OmnibarCreationPanel.tsx` (line 709) sits as a sibling between `.body` and `.footer`, outside the scrollable region. Pre-existing structure, unaffected by and out of scope for this fix — noted only in case a future error-message-visibility bug gets attributed to this change.
