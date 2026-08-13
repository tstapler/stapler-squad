# Cross-Artifact Consistency Check: nav-redesign

**Date**: 2026-06-24
**Verdict**: CONSISTENT (minor gaps noted, none blocking)

---

## Coverage Matrix

| Requirement | Story/Task in Plan | Research Backing | Status |
|-------------|-------------------|-----------------|--------|
| Redesign `nav-pages.ts` to add group/category metadata to all nav items | Story 1.1.1 (Tasks 1.1.1a–c) | stack.md §2, architecture.md Option A | COVERED |
| Reorganize DrawerNav to render grouped sections with headers | Story 2.1.1 + 2.1.2 | features.md §6, architecture.md §minimal-change-surface | COVERED |
| Reorganize BottomNav More sheet to render grouped sections instead of flat list | Story 3.1.1 + 3.1.2 | features.md §5, architecture.md §minimal-change-surface | COVERED |
| Ensure all currently-hidden mobile routes (`mobileNav: false`) are reachable on mobile | Story 1.1.1 Task 1.1.1b (remove `mobileNav: false`); Story 3.1.2 (More sheet includes them) | features.md §3 | COVERED |
| Consolidate Settings / Config Files / Features into a single nav entry | Story 1.1.2 (remove Config Files + Features entries) | features.md §2 consolidation detail | COVERED |
| All existing routes continue to work (no 404s) | Confirmed in Story 1.1.2 AC: "routes are untouched; only the nav entries are removed" | architecture.md (no route changes) | COVERED |
| All existing tests pass; new tests cover grouping logic | Phase 4 (Stories 4.1.1–4.1.3) | pitfalls.md §2 | COVERED |
| Items grouped into ≤3–4 logical sections covering all 16+ items | 4-group taxonomy: Work / Automation / Insights / Settings & Tools | features.md §2, architecture.md §proposed-group-structure | COVERED |

---

## Gaps Found

### GAP-1 (Minor): `NavGroup` type discrepancy between research docs

- **architecture.md** defines `NavGroup = "primary" | "automation" | "insights" | "settings" | "system"` (5 values, uses "primary" not "work", includes a "system" bucket)
- **features.md** defines `NavGroup = "work" | "automation" | "insights" | "settings"` (4 values, no "system")
- **stack.md** defines `NavGroup = "core" | "work" | "analytics" | "settings" | "system"` (5 values, uses "core" and "analytics")
- **plan.md** uses `"work" | "automation" | "insights" | "settings"` (4 values, matching features.md)

The plan resolves the ambiguity by picking the features.md version. The architecture.md "system" bucket (Logs, Errors, Help) is folded into "settings" in the plan's group assignments (Story 1.1.1b: "Settings, Logs, Errors, Help, Files → `group: "settings"`"). This is a valid decision, but it is implicit — no ADR or plan note explains why "system" was collapsed into "settings" rather than kept separate.

**Risk**: Low. The plan is internally consistent; the inconsistency is across research docs, not between research and plan. No action required before implementation, but the ADR (ADR-001-nav-grouping-approach.md) should document this decision.

### GAP-2 (Minor): `Header.tsx` grouped rendering deferred without explicit requirement tracing

- The requirements state "Reorganize the BottomNav 'More' sheet" and "Reorganize the DrawerNav" but do not mention `Header.tsx`.
- The plan correctly defers Header.tsx to Out of Scope.
- However, the `Header.tsx` hamburger menu still renders a flat list after this plan ships. The requirements say "A new user can identify what features exist without trial-and-error" — a flat hamburger menu on mid-size screens still partially violates this.
- **Risk**: Low. The requirements' explicit in-scope list does not include Header.tsx, and the plan's Out of Scope section calls this out. The gap is accepted, not overlooked.

### GAP-3 (Minor): Keyboard focus trap in More sheet

- pitfalls.md is not the source — features.md §5 UX improvements item 5 recommends Tab/Shift+Tab focus trap when the More sheet is open.
- The plan's Out of Scope section defers this as "Keyboard focus trap in More sheet — noted in features research as improvement; deferred."
- The requirements do not list keyboard accessibility as a success metric, so deferral is within scope of the requirements.
- **Risk**: Low. Correctly deferred and explicitly acknowledged.

### GAP-4 (Moderate): `BOTTOM_NAV_MORE` filter behavior after removing `mobileNav: false`

- Story 1.1.1 AC states: "After this story, `BOTTOM_NAV_MORE` includes Settings, Logs, Errors, Help, Escape Analytics, Files (previously `mobileNav: false`) because the `mobileNav: false` exclusion is removed from those entries."
- The current `BOTTOM_NAV_MORE` derivation in `nav-pages.ts` is `mobileNav !== false && !bottomNavPrimary`. After removing `mobileNav: false` from those 8 items, they will automatically appear in `BOTTOM_NAV_MORE`.
- However, the plan does not include a task to verify or update the `MOBILE_NAV_PAGES` and `BOTTOM_NAV_MORE` filter predicates themselves. If those predicates use `mobileNav` in a way that changes semantics (e.g., `mobileNav !== false` becomes a no-op when all items have `mobileNav` truthy or absent), the derived arrays will include everything — including `bottomNavPrimary` items in `MOBILE_NAV_PAGES`.
- **Risk**: Moderate. The filter logic change is implied but not explicitly tasked. An implementer may not realize that `BOTTOM_NAV_PRIMARY` items must remain excluded from `BOTTOM_NAV_MORE` (they are excluded only by `!bottomNavPrimary`, which is unchanged, so this works — but a task to verify the predicate is missing from Phase 1).

---

## Orphaned Tasks

None found. Every task in the plan traces to a clear requirement:

| Task | Requirement Source |
|------|--------------------|
| Story 1.1.1 (NavGroup + groupNavPages) | Requirement: group metadata; success metric: items in ≤4 groups |
| Story 1.1.2 (Settings consolidation) | Requirement: consolidate Settings/Config Files/Features |
| Story 1.1.3 (active-state bug fix) | pitfalls.md §5a — explicitly required to avoid silent breakage from consolidation |
| Story 2.1.1 + 2.1.2 (DrawerNav) | Requirement: reorganize DrawerNav |
| Story 3.1.1 + 3.1.2 (BottomNav More sheet) | Requirement: reorganize BottomNav More sheet; all hidden routes reachable |
| Story 4.1.1 (BottomNav test fix) | pitfalls.md §2a + success metric: all existing tests pass |
| Story 4.1.2 (DrawerNav.test.tsx) | pitfalls.md §2d (zero DrawerNav test coverage); success metric: new tests cover grouping logic |
| Story 4.1.3 (nav-pages.test.ts) | success metric: new tests cover grouping logic |

---

## Out-of-Scope Violations

None found. The plan explicitly lists its Out of Scope section and it correctly excludes everything that requirements mark out of scope:

| Out-of-scope item (requirements) | Plan treatment |
|----------------------------------|---------------|
| Changing routes (no URL changes) | Confirmed: "URL changes — no routes modified" |
| Adding or removing pages | Confirmed: no new page components created |
| Changing BottomNav primary bar item count | Confirmed: `bottomNavPrimary` field is unchanged; primary items are not added/removed |
| Redesigning the settings page itself | Confirmed: "Settings page internal tab navigation — settings page sub-nav is out of scope" |
| Internationalization / translations | Not mentioned in plan (correctly absent) |

One potential concern: Story 2.1.2 adds `useFeatureFlags()` to `DrawerNav.tsx` to fix a pre-existing bug (DrawerNav shows flagged items regardless of flag state). This is not a requirement for the nav redesign but is a bug fix enabled by the refactor. It is not out-of-scope per se, but it is an unbounded behavior change — if a flag is off, the DrawerNav will now hide the item whereas it previously showed it. This should be called out in the PR as a behavior change, not just a refactor.

---

## Research → Plan Alignment

| Research Finding | Reflected in Plan? |
|-----------------|-------------------|
| 4-group taxonomy (Work / Automation / Insights / Settings & Tools) from features.md §2 | Yes — Story 1.1.1b group assignments match exactly |
| Option A architecture (add `group` field, `groupNavPages()` helper) from architecture.md | Yes — Story 1.1.1 implements Option A verbatim |
| iOS safe-area / `--bottom-nav-height` risk from pitfalls.md §7 | Partially — plan notes "keep the bottom nav bar a fixed height" in Story 3.1.2 implicitly (More sheet max-height is capped at 70vh). No explicit task guards against accordion-in-primary-bar. Since the plan does not introduce accordions in the primary bar, the risk is avoided by omission, not explicit mitigation. |
| BottomNav.test.tsx `PRIMARY_ITEMS` hardcode risk from pitfalls.md §2a | Yes — Story 4.1.1 directly addresses this |
| Badge special-casing must be preserved (pitfalls.md §3) | Yes — Story 2.1.2 AC explicitly states "The existing badge special-cases are preserved exactly — no badge is dropped" |
| Empty group header risk when all items are flag-filtered (pitfalls.md §4) | Yes — Story 2.1.2 AC: "Empty groups (all items flag-filtered) do not render a section header"; Story 3.1.2 AC: "Empty groups render no header" |
| Config Files query-param active-state bug (pitfalls.md §5a) | Yes — Story 1.1.3 resolves by removal |
| `/settings` prefix collision (pitfalls.md §5b) | Yes — Story 1.1.3 notes this is resolved by consolidation |
| DrawerNav zero test coverage (pitfalls.md §2d) | Yes — Story 4.1.2 creates DrawerNav.test.tsx |
| vanilla-extract inline style prohibition (pitfalls.md §6) | Partially — Story 3.1.2b draft code uses `style={{ borderTop: ... }}` inline style for the utility section divider, then Story 3.1.2c explicitly corrects this with a `moreSheetUtilitySection` css style. The self-correction is present but the initial draft in 3.1.2b is a known violation. Implementer must not stop at 3.1.2b. |
| More sheet `max-height` needed for small screens (pitfalls.md §7, implied) | Yes — `moreSheetScrollable` with `maxHeight: "70vh"` in Story 3.1.1 |

---

## Summary

The plan is well-aligned with requirements and research. All 5 in-scope requirements have corresponding stories. No plan tasks are orphaned. No out-of-scope items are included.

**Most important gap**: GAP-4 — the plan does not include an explicit verification task for how `BOTTOM_NAV_MORE` and `MOBILE_NAV_PAGES` predicates behave after `mobileNav: false` is removed from 8 items. The logic is almost certainly correct (the `!bottomNavPrimary` guard still excludes primary items from the More sheet), but a missing explicit verification step creates a risk of silent regression if the predicate is misread during implementation.

**Secondary concern**: Story 3.1.2b includes a draft with an inline style for the utility section divider, then 3.1.2c self-corrects it. An implementer who only reads the task description and not the follow-up note could ship the inline style. A pre-commit lint check (`make lint`) would catch this, but it is better to not introduce the anti-pattern in step one.

**Recommendation**: Add a half-line verification note to Story 1.1.1b: "After removing `mobileNav: false` from 8 items, verify that `BOTTOM_NAV_MORE` still excludes `bottomNavPrimary: true` items (it does via the unchanged `!bottomNavPrimary` predicate)." No structural changes to the plan are needed.
