# verify-web: TS/React + CSS review of cli-flag-discovery

Method: read every key file in full (VERIFIED by reading); no runtime execution. "INFERRED" = derived from code + browser semantics, not run.
Scope note: skill checklists not loaded; standards applied directly.

## MUST FIX

1. web-app/src/lib/hooks/useProbeProgram.ts:131 - `if (inFlightRef.current === cmd) return;` runs before the `explicit` check, so an explicit Check is dropped when a blur/implicit probe of the same command is in flight. Path: user types a command, clicks Check with the mouse; mousedown blurs the input -> `check()` (ProgramsManager.tsx:360) -> state "checking" -> Check button `disabled` (ProgramsManager.tsx:~384) -> the click never fires / is ignored; user must click a second time. The e2e helper masks this by focusing another field first (ProgramsSettingsPage.ts `enterCommandAndBlur`). Fix: `if (inFlightRef.current === cmd && !explicit) return;` (explicit already cancel()s the prior request), and add an e2e/hook test "explicit Check while blur probe in flight". INFERRED.

## SUGGEST

2. web-app/src/components/sessions/OmnibarCreationPanel.tsx:951 + lib/hooks/useAvailablePrograms.ts:24-27,44 - Regression vs removed `isProgramRecognized`: a `program` value with no matching option, and every static/fallback option (PROGRAMS constants, /api/server-info fallback) has no `command`, so `ProgramProbeSection` gets command "" -> idle -> no not-found warning at all. Fix: `const command = option?.command ?? option?.value ?? program` (pass the selected `program` string through), or fill `command: fullPath/value` in the fallback options. INFERRED from code.
3. web-app/src/components/sessions/OmnibarPresetList.css.ts:71 - `programWarning` export is now unreferenced (grep: only its definition). Delete it.
4. web-app/src/lib/hooks/useProbeProgram.ts:77-82,165 - "notFound" is memoized for the hook lifetime and Omnibar has no Retry/Check for it, so installing the binary needs a page reload (Settings has Check; Omnibar does not). Fix: drop "notFound" from MEMOIZABLE, or show Retry for notFound.
5. web-app/src/lib/hooks/useProbeProgram.ts:118 - `setState({ kind: "idle" })` allocates a new object every command change/mount, forcing a render even when already idle. Fix: module-level `const IDLE: ProbeUiState = { kind: "idle" }`.
6. web-app/src/components/ui/FlagCombobox.tsx:43,46 - `nav.active` is not reset when the token/matches change via caret movement (onSelect -> setCaret) while the list is open, so the active index can point at a different option (or past the end -> aria-activedescendant silently dropped). Fix: in `syncCaret`, if token text changed call `nav.openList()`/reset active, or clamp `active < matches.length`.
7. web-app/src/components/ui/ProbeStatusBadge.tsx:109-152,158 - `window.matchMedia` read during render can mismatch SSR/hydration (`data-static`, class) and is not reactive. Fix: drop `spinnerStatic`/JS check and add `@media (prefers-reduced-motion: reduce) { animation: none }` to `css.spinner` in ProbeStatusBadge.css.ts.
8. web-app/src/components/settings/ProgramsManager.tsx:369 and sessions/ProgramProbeSection.tsx:374 (PROGRAM_PROBE_STATUS_ID) - `aria-describedby` targets an id that is not in the DOM while state is idle/disabled (badge returns null). Harmless to AT but a dangling ref; set the attribute only when `probe.state.kind` is not idle/disabled.
9. web-app/src/components/ui/ProbeStatusBadge.tsx:156-199 - the "Show full path" toggle sits inside the `flex-wrap` root beside the status; on 375px it uses `flexBasis:auto` (toggle style) with 44px min height - OK, but `describe()` `default: return null` hides new `ProbeUiState` kinds silently. Add an exhaustive `never` check for the idle/disabled cases so a new variant fails the type-check.

## NITPICK

10. FlagCombobox.css.ts:13,29 / ProgramsManager.css.ts (checkButton borderRadius "4px"), fontSize "0.875rem"/"0.8125rem" - hardcoded where `vars.radii.*` / `vars.fontSize.*` exist. No hex, no zIndex, no inline layout styles found (VERIFIED: grep of all new .css.ts files). Use vars.
11. ProbeStatusBadge.tsx:94,157,171 etc. - pointless template literals (`` `${css.icon}` ``); pass the class directly.
12. AvailableFlags.tsx:274 - the "▴/▾" glyph is read by screen readers; wrap in `<span aria-hidden="true">`.
13. FlagInfoButton.tsx:225-226 - `aria-label` flips Show/Hide while `aria-expanded` already conveys state (double-announces); use a constant label. `aria-controls` also references an id absent while collapsed.
14. FlagCombobox.tsx:135 - `activeDescId` element exists only for the active option; correct, but descriptions are unreachable on touch (no active option). Acceptable because AvailableFlags exposes them; note in the component doc.

## Checked and clean (VERIFIED by reading)
- Race/abort: `cancel()` bumps token, aborts, clears in-flight; result handler checks token and `signal.aborted`; unmount cleanup via effect return (useProbeProgram.ts:116-120); no setState after unmount.
- Timers: ProbeStatusBadge announce timeout cleared in effect cleanup (:131-138).
- Deps arrays: correct; the single eslint-disable in FlagCombobox.tsx:60 is justified (optionId derives from `id`).
- ARIA combobox: role/aria-expanded/aria-controls/aria-activedescendant only when flags exist; option ids match `${id}-option-i`; listbox in normal flow; FlagInfoButton kept outside role=option (no nested-interactive); warning is plain text (no role=alert/aria-invalid); `prog-flags-hint`/`prog-flags-warning` ids exist when referenced.
- Touch/no-hover: all buttons and options min 44px (FlagCombobox.css.ts:17, FlagInfoButton.css.ts:5-6, FlagNotes.css.ts:18, ProbeStatusBadge.css.ts:70-71, ProgramsManager.css.ts checkButton); no :hover/mouseenter dependence; 640px breakpoint switches full-width to inline.
- Shared probe mock used by all consumers (ProgramsManager.test.tsx:13, ProgramProbeSection.test.tsx:10, OmnibarCreationPanel.test.tsx:8, useProbeProgram.test.ts:12-13, ProbeStatusBadge.test.tsx:4); the extra local `jest.mock("@connectrpc/connect")` in ProgramsManager.test.tsx:19 is needed for the list client, not a duplicate probe mock.
- e2e conventions: `// @feature` header on line 1; no `waitForTimeout`; locators via getByTestId/getByRole only; page helper in tests/e2e/pages/ProgramsSettingsPage.ts. (`expect.poll(ran)` is fs-based, allowed.)
