# Architecture Review: multi-window
**Date**: 2026-09-23
**Verdict**: CLEAN

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository
(`find docs/adr -iname "*constitution*"` returned nothing). No constitution constraints apply —
skipping this section per the task instructions.

## Grounding notes

- Verified plan.md's cited line counts against the actual files: `page.tsx` 561, `paneReducer.ts`
  266, `paneUtils.ts` 212, `usePaneReducer.ts` 135, `PaneTilingContainer.tsx` 366,
  `PaneSplitRenderer.tsx` 502, `usePaneLayout.ts` 73, `shortcutRegistry.ts` 141,
  `MobilePaneTabStrip.tsx` 61 — all match.
- Read `usePaneReducer.ts`, `shortcutRegistry.ts`, and `usePaneShortcuts.ts` in full to check the
  plan's specific technical claims (restore/revalidate/save timing, dispatch-loop semantics,
  existing shortcut key/modifier set) against real code, not just plan prose.
- `kibitzer` is on `PATH`; `.claude/inspect.json` only configures one Go-scoped check
  (`go-primitive-obsession`, `scope: ["**/*.go"]`), so no `component-deps`/`content-rules`/
  `naming-rules` checks are configured for this TypeScript codebase. `kibitzer run web-app/src/lib/pane --trigger batch`
  still runs generic built-in `syntax-rules-typescript` checks (not project-configured) and returned
  a concrete, useful data point: `usePaneReducer.ts` — the file `useWindowManager.ts` explicitly
  models itself on and extends — is *already* flagged `[long-function] body spans 110 lines (over 40)`,
  and `paneReducer.ts` at 217 lines. This is cited in Concern 3 below.
- `research/build-vs-buy.md` was not in the assigned reading list and not opened; the Radix
  Tabs/swipe-library rejections in plan.md's Pattern Decisions table are internally consistent
  with each other and not independently re-verified against that file.

## Resolved

- [x] **Story 1.2.1 / Task 1.2.1b (`windowReducer.ts`'s `CLOSE_WINDOW` case)** — previously an
  illegal, untested state (empty `windows` array reachable if `CLOSE_WINDOW` on the last window
  omitted `replacement`). Re-verified against the current plan.md:
  - Task 1.1.1a (line 134): `replacement` is now a **required** field on `CLOSE_WINDOW`
    (`replacement: { id: WindowId; name: string }`, not `replacement?: ...`), with an explicit note
    that this makes an empty `windows` array unrepresentable by construction.
  - Task 1.2.1b (line 170): the reducer implementation no longer branches on
    `action.replacement`'s presence — only on `remaining.length === 0` — and states plainly that
    "`windows.length === 0` is unreachable by construction."
  - Task 1.2.1e (lines 183–185): the test list now includes both a non-last-window close and a
    **last-window close with its required `replacement` payload**, asserting the result is never an
    empty `windows` array.
  - Task 1.4.1e (line 298): `closeWindow()`'s description now generates and attaches `replacement`
    **unconditionally**, on every `CLOSE_WINDOW` action regardless of `windows.length`, and its
    return type is `WindowId` (not `WindowId | undefined`) — the "should never happen" undefined
    case is explicitly removed, since at least one window always remains.

  All four remediation points from the original blocker are addressed. Blocker resolved.

## Concerns

- [ ] **Task 1.4.1c (`useWindowManager`'s `storage` event listener)** — parse-at-boundary gap. The
  initial-load path (`loadWindowLayout()`, Task 1.3.1a) validates the parsed JSON through an
  exhaustive `switch (parsed.version)` before trusting it. The `storage`-event listener is a
  *second* entry point into the same `PersistedWindowLayoutV2` domain type (data written by another
  tab, another app version, or corrupted), but its task description ("parses `event.newValue`,
  compares `revision`... dispatches `RESTORE_WINDOWS`") doesn't call for routing through that same
  validation — as written it would trust `event.newValue`'s shape and hand `.windows` straight to
  `RESTORE_WINDOWS`. **Recommendation**: extract `loadWindowLayout()`'s parse+version-switch logic
  into a shared `parseWindowLayoutJson(raw: string): PersistedWindowLayoutV2 | null` used by both
  the initial load and the `storage` listener, so a malformed/future-version write from another tab
  fails closed the same way a malformed initial load does, rather than being blindly adopted.

- [ ] **Epic 1.4 / `useWindowManager.ts` as a whole** — disposition ("Isolate via seam," per the
  Tech Debt Disposition table) is directionally right but not sized against the smell it's meant
  to avoid. The plan's own Domain Glossary lists five concerns for this one hook: reducer wiring +
  restore-once, debounced save, revision-guard conflict handling, `storage`-event cross-tab
  reconciliation, and per-window session revalidation — plus the CRUD command API
  (`createWindow`/`closeWindow`/`renameWindow`/`dispatchPane`). That's two more concerns than its
  named precedent, `usePaneReducer.ts` (restore, revalidate, debounced-save — 3 effects), which
  `kibitzer`'s generic `long-function` check already flags at 110 lines against a 40-line
  threshold (`kibitzer run web-app/src/lib/pane --trigger batch`, `usePaneReducer.ts:19`). Nothing
  in Epic 1.4's tasks (1.4.1a–e) splits the hook by concern, so the new "seam" file is likely to
  reproduce the same long-function shape in a brand-new location rather than avoid it.
  **Recommendation**: split the cross-tab-sync concern (revision-guarded save + `storage` listener,
  Tasks 1.4.1b/1.4.1c) into its own hook (e.g. `useWindowCrossTabSync.ts`) that `useWindowManager`
  composes, leaving `useWindowManager.ts` itself scoped to reducer wiring, restore, revalidation,
  and the CRUD API — closer to `usePaneReducer.ts`'s original scope.

- [ ] **Story 3.3.1 (`useWindowShortcuts` leader-key State pattern)** — untested interaction with
  `shortcutRegistry.ts`'s existing input-suppression guard. `ShortcutRegistry.dispatch()`
  (`shortcutRegistry.ts:94-95`) returns early — no shortcut fires, matched or not — for any keydown
  with no `ctrl`/`meta`/`alt` modifier held while `document.activeElement` is an
  input/textarea/select/contenteditable. The leader-key follow-ups (`1`-`9`, `n`, `p`, `,`,
  `Escape`) are all no-modifier shortcuts. If focus happens to be on an input when the follow-up
  key is pressed (e.g. the rename `<input>` Story 3.1.2 introduces, or any other text field), the
  follow-up is silently swallowed by the registry before `useWindowShortcuts` ever sees it — the
  AC "Escape while armed disarms without any action" and "unrecognized follow-up key also disarms"
  both implicitly assume the keydown reaches the registry's dispatch loop, which it won't in this
  case. This self-heals via the existing 3000ms auto-disarm timeout, so it's not a correctness
  blocker, but it's an unstated interaction between two "State pattern" transition sources
  (explicit follow-up key vs. implicit timeout) that Story 3.3.1's AC list doesn't name.
  **Recommendation**: add this as an explicit test case (armed + focus on an input + follow-up key
  → no action, then timeout auto-disarms) and fold a one-line note into Task 3.3.1c's platform-caveat
  comment, alongside the existing `Ctrl+-`/`Ctrl+W` convention.

## Nitpicks

- Tech Debt Disposition table has no row for `PaneTilingContainer.tsx`, even though
  `research/architecture.md`'s Hotspot disposition section explicitly names its disposition
  ("Isolate via seam" via the prop-threading swap) and plan.md's Epic 2.1 correctly implements
  exactly that. The behavior is right; add the row for traceability with the other three entries.
- `WindowId = string`/`PaneId = string` are both unbranded aliases generated via the identical
  `generateSecureId().slice(0, 8)` scheme (verified: `windowUtils.ts`'s planned `generateWindowId()`
  mirrors `paneUtils.ts:22-23`'s `generatePaneId()` byte-for-byte), so the two ID spaces are
  structurally indistinguishable strings, not just conceptually distinct ones. The Pattern
  Decisions table's rationale for skipping branding ("disjoint code paths, never compared") is a
  behavioral claim about today's call sites, not a structural guarantee — `dispatchPane(windowId,
  action)` does receive both a `WindowId` and a `PaneAction` carrying a `PaneId` in the same call
  context (`page.tsx`'s wiring). Low practical risk given the existing, already-unbranded `PaneId`
  precedent this decision is deliberately staying consistent with — not worth blocking on for this
  feature, but worth a one-line comment at the `WindowId` type definition noting the two ID spaces
  are same-shaped by construction.
