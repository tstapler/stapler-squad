# Research: Jump to Session — Modal Mode on Mobile

## 1. Current Architecture

### Component graph (jump-to-session flow)

```
SessionsSection.tsx
  └─ JumpToSessionPanel (props: linkedSessions, backlogItemId, getSession)
       ├─ [button] "Jump to {role} session"  → setExpanded(true)
       ├─ [caret] role-selector dropdown     → setSelectedRole, setExpanded(true)
       └─ {expanded && div.panel}
              └─ SessionDetail (session, embedded=true, initialTab="terminal")
                   └─ ... xterm.js terminal + tabs
```

`JumpToSessionPanel` is rendered once in `SessionsSection` (line 261), which is called from `BacklogItemDetail`. The session fetch is driven by `useEffect` on `[expanded, target, getSession]`.

### Inline panel size constraint (the problem)

`JumpToSessionPanel.css.ts` panel style:
```ts
vars: {
  "--viewport-height": "min(60vh, 520px)",   // desktop
  "--header-height": "0px",
},
"@media": {
  [`screen and (max-width: ${breakpoints.sm})`]: {  // breakpoints.sm = 640px
    vars: { "--viewport-height": "min(70vh, 420px)" },
  },
},
```

`SessionDetail.css.ts` container derives its height from:
```ts
height: "calc(var(--viewport-height, 100dvh) - var(--header-height))"
```

On mobile with virtual keyboard open (visualViewport shrinks), 70 % of a ~400 px visual viewport is ~280 px — barely enough for 8 terminal rows. The inline panel also sits in page scroll context, so keyboard interactions can cause unwanted page-level scroll.

---

## 2. SessionPeekModal — Existing Callers and Props

**Only one existing caller:** `PaneTilingContainer.tsx` (line 284–287):
```tsx
{peekSession && (
  <SessionPeekModal
    session={peekSession}
    onClose={() => setPeekSession(null)}
  />
)}
```

`SessionPeekModal` props: `{ session: Session; onClose: () => void }`. The modal internally:
- Creates a `createPortal(…, document.body)` with a backdrop + fixed overlay.
- Renders its own header (session title, "Peek" badge, close button).
- Passes `embedded={true}` and `initialTab="terminal"` to `SessionDetail`.
- Handles Escape key via `window.addEventListener("keydown", …, { capture: true })`.
- Calls `modalRef.current?.focus()` on mount (no `useFocusTrap` — simple autofocus).

`JumpToSessionPanel` already has all the data `SessionPeekModal` needs: a fetched `Session` object. Wiring in the modal is a one-line render swap gated on `isMobile`.

---

## 3. `useViewport` Mobile Detection

`ViewportProvider.tsx` exports `useViewport()` returning:
```ts
isMobile: boolean   // window.innerWidth < 600
```

The breakpoint aligns with `breakpoints.fold = "600px"` in `theme-contract.css.ts`.

**Established pattern** (`PaneTilingContainer.tsx`, `PaneSplitRenderer.tsx`):
```ts
const { isMobile, isFoldable } = useViewport();
const isNarrow = isMobile || isFoldable;
```
For `JumpToSessionPanel`, only `isMobile` (< 600 px) is needed — foldable devices (600–900 px) have enough room for the inline panel.

---

## 4. `--viewport-height` CSS Variable Chain

```
ViewportProvider (visualViewport.height) → document.documentElement style --viewport-height
  → SessionDetail.css.ts container: calc(var(--viewport-height, 100dvh) - var(--header-height))
```

`JumpToSessionPanel.css.ts` **overrides** `--viewport-height` on the panel wrapper via vanilla-extract's scoped `vars`, restricting `SessionDetail`'s height to `min(70vh, 420px)` on mobile.

`SessionPeekModal.css.ts` does **not** override `--viewport-height`. The modal body is `flex: 1; overflow: hidden`, so `SessionDetail` would try `calc(100dvh - 0px)`. In practice the modal itself is `position: fixed; top: 3vh; bottom: 3vh`, so the body height is constrained by the flex container (overflow hidden) and `SessionDetail` fills correctly — verified by the existing peek-from-session-list use case in `PaneTilingContainer`.

**However:** on mobile with virtual keyboard open, `--viewport-height` (set to `visualViewport.height`) is already reduced. The modal's `top: 3vh; bottom: 3vh` margins compound the constraint. This argues for making the modal full-screen on mobile (see §6 below).

---

## 5. Feature Flag and Registry

`// +feature: backlog:jump-to-session` (line 2 of `JumpToSessionPanel.tsx`) is a **source marker** for `make registry-generate`. It maps to:

- `docs/registry/features/frontend/backlog-jump-to-session.json` — exists, `tested: true`
- `docs/registry/coverage-gaps.json` — listed (coverage gap, E2E exists but gap tracking hasn't been cleared)
- `docs/registry/frontend-features.json` — aggregated JSON

The registry description ("expanded inline in the detail pane with no page navigation") will need updating after the change. The feature flag does **not** control runtime behavior (no `FeatureGuard`); it's purely for the registry scan.

`route-meta.ts` — **no new route needed**. This is a UI behaviour change within an existing page component.

---

## 6. Risk Factors and Gotchas

### R1 — Modal CSS on mobile is not full-screen
`SessionPeekModal.css.ts` modal: `top: 3vh; left: 3vw; right: 3vw; bottom: 3vh`. On a 390×844 phone with keyboard open (~390×450 visual viewport after keyboard), `3vh ≈ 13px` margins leave ~424 px available — better than the 280 px inline panel, but room is tight. **Fix**: add `@media (max-width: 599px)` rule setting `inset: 0; border-radius: 0; border: none` for a true full-screen experience on phones.

### R2 — `SessionPeekModal` renders its own title header
The modal header includes session title, "Peek" badge, and close button (from `SessionPeekModal.tsx`). For the jump-to-session context the "Peek" label is slightly misleading (it's a live terminal, not a peek). Acceptable for now; can be addressed as a follow-up by adding an optional `label` prop.

### R3 — E2E spec needs a new mobile test case
`tests/e2e/jump-to-session.spec.ts` has a mobile test that only checks button height ≥ 44 px. After this change, that test must also assert the modal opens (not an inline panel) at mobile width. The existing "opens a real terminal" test runs at default (desktop) width and should continue passing unchanged.

### R4 — Analytics double-fire risk
The current `expanded` state is set to `false` on mobile (modal path). The analytics `track()` call for `backlog_jump_to_session` fires inside `onClick` and inside the role-menu item click. Both paths must continue to fire once. Since the modal path doesn't use the `expanded` boolean, care is needed to ensure `track()` fires exactly once in both branches.

### R5 — `useFocusTrap` not needed for modal path
`SessionPeekModal` manages its own focus (autofocus on mount + Escape handler). `JumpToSessionPanel`'s `useFocusTrap` hook is scoped to `menuRef` (the role-selector dropdown) and is unaffected.

### R6 — Registry description drift
`docs/registry/features/frontend/backlog-jump-to-session.json` description says "expanded inline … no page navigation". After the change, mobile opens a modal, not an inline expansion. `make registry-generate` only updates structural fields (RPCs, components, markers) — description is hand-authored and must be updated manually.

---

## 7. Existing Tests

**Unit (Jest)** — `JumpToSessionPanel.test.tsx` — 6 tests:
- `_RenderNothing_When_NoJumpableSessionExists`
- `_HideRoleMenu_When_OnlyOneRoleIsJumpable`
- `_FetchAndRenderTerminal_When_ButtonClicked` (checks inline `session-detail` testid)
- `_ShowError_When_GetSessionReturnsNull`
- `_CollapsePanel_When_ClickedAgain` (checks `jump-to-session-panel` testid appears/disappears)
- `_SwitchTargetSession_When_DifferentRoleChosenFromMenu`

All existing tests run at default (desktop) width. The `_FetchAndRenderTerminal` and `_CollapsePanel` tests rely on `jump-to-session-panel` — which won't exist on mobile. **New tests needed:**

1. `JumpToSessionPanel_should_OpenModal_When_Mobile_And_ButtonClicked` — mock `useViewport` to return `isMobile: true`; click button; expect `SessionPeekModal` rendered (mock it via `jest.mock`).
2. `JumpToSessionPanel_should_CloseModal_When_Mobile_And_ModalClosed` — verify `onClose` from `SessionPeekModal` closes the modal.

**E2E** — `tests/e2e/jump-to-session.spec.ts` — 3 tests. New test needed:
- `"opens a modal terminal at mobile width"` — at 390px viewport, click button, assert `[role="dialog"]` appears (from `SessionPeekModal`), assert `session-detail` testid is inside it.

---

## 8. Implementation Touchpoints

| File | Change |
|---|---|
| `JumpToSessionPanel.tsx` | Add `const { isMobile } = useViewport()`. On click (and role-menu select), if `isMobile`: fetch session → set `modalSession` state → render `<SessionPeekModal>`. Keep `expanded` + inline panel for desktop. |
| `JumpToSessionPanel.css.ts` | No changes required (mobile panel style unused when modal path active). |
| `SessionPeekModal.css.ts` | Add `@media (max-width: 599px)` rule: `inset: 0; border-radius: 0; border: none` for true full-screen on phones. |
| `JumpToSessionPanel.test.tsx` | Add 2 new unit tests for mobile modal path (mock `useViewport`). |
| `tests/e2e/jump-to-session.spec.ts` | Add 1 new e2e test at 390 px that verifies modal opens. |
| `docs/registry/features/frontend/backlog-jump-to-session.json` | Update `description` to reflect both inline (desktop) and modal (mobile) modes. |
