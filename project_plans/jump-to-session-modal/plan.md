# Plan: Jump to Session — Modal Mode on Mobile

## Summary

Replace the inline-expand behavior of `JumpToSessionPanel` on mobile with a `SessionPeekModal` overlay. Desktop behavior is unchanged. Also make `SessionPeekModal` full-screen on mobile so the terminal has maximum space.

## Stories

### Story 1 — Mobile viewport detection in JumpToSessionPanel

**File:** `web-app/src/components/backlog/detail/JumpToSessionPanel.tsx`

Add `const { isMobile } = useViewport();` from `@/components/providers/ViewportProvider`.

Change the click handler for the main jump button:
- **Desktop (default):** existing toggle `expanded` state — no change.
- **Mobile:** instead of toggling `expanded`, set a new `modalOpen` boolean state to `true` and trigger the session fetch.

Add a new `modalOpen: boolean` state alongside `expanded`. Fetch logic already lives in a `useEffect` keyed on `[expanded, target, getSession]` — extend the key to include `modalOpen` (or refactor to a shared `isOpen` flag).

When `modalOpen && session`:
- Render `<SessionPeekModal session={session} onClose={() => setModalOpen(false)} />` (portal, already from `SessionPeekModal`).

Collapse / close semantics:
- Mobile: modal close button sets `modalOpen = false`, session cleared.
- Caret role menu: choosing a role on mobile should set `selectedRole` then immediately open the modal.

Analytics: both code paths (main button, role menu) already call `track()`; no change needed.

### Story 2 — Full-screen SessionPeekModal on mobile

**File:** `web-app/src/components/sessions/SessionPeekModal.css.ts`

Add a `@media (max-width: 599px)` block to the `modal` style:

```ts
"@media": {
  "screen and (max-width: 599px)": {
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    borderRadius: 0,
  },
},
```

This improves the peek modal for ALL callers on mobile, including the session-list row peek from `PaneTilingContainer`.

### Story 3 — Update JumpToSessionPanel tests

**File:** `web-app/src/components/backlog/detail/JumpToSessionPanel.test.tsx`

Add test cases:
- `JumpToSessionPanel_should_OpenModal_When_MobileAndButtonClicked` — mock `useViewport()` to return `{ isMobile: true }`, click button, assert `SessionPeekModal` renders (mock it and assert mock called with session).
- `JumpToSessionPanel_should_CloseModal_When_MobileAndModalClosed` — verify `modalOpen` resets when `onClose` fires.
- `JumpToSessionPanel_should_KeepInlineExpand_When_Desktop` — mock `useViewport()` to return `{ isMobile: false }`, existing expand tests pass unchanged.

Add jest.mock for `@/components/providers/ViewportProvider` so `useViewport` can be controlled per-test.

### Story 4 — Feature registry update

**File:** `docs/registry/features/frontend/backlog-jump-to-session.json`

Update the description to reflect that mobile opens a modal and desktop expands inline.

Run `make registry-generate` and commit changed per-feature files.

## Adversarial Review

**Risk: Double-fetch on mobile** — if `selectedRole` change and `modalOpen` both trigger the `useEffect`, the session might be fetched twice. Mitigation: keep a single `useEffect` keyed on `[isOpen, target, getSession]` where `isOpen = expanded || modalOpen`.

**Risk: SessionPeekModal full-screen CSS affects PaneTilingContainer caller** — this is intentional and beneficial; the existing caller's modal is also too small on mobile. Confirmed beneficial side-effect.

**Risk: Virtual keyboard pushes modal out of view** — `SessionPeekModal` uses `position: fixed` which is anchored to the layout viewport, not the visual viewport. On iOS Safari, `position: fixed` is clipped to the visual viewport when a virtual keyboard is open. Setting `inset: 0` on mobile may still have this issue. Mitigation: document as known limitation; a future fix can use `env(safe-area-inset-*)` or the `virtualKeyboard` API — out of scope for this item.

**Risk: `useFocusTrap` not needed in JumpToSessionPanel for modal** — `SessionPeekModal` has its own Escape key handler and `tabIndex=-1` focus management; no additional focus trap is required.

## Task Breakdown

| # | Task | File(s) | Estimate | Category |
|---|------|---------|----------|----------|
| 1 | Add `isMobile` detection + `modalOpen` state in `JumpToSessionPanel` | `JumpToSessionPanel.tsx` | 2h | frontend |
| 2 | Refactor fetch effect to key on unified `isOpen` flag | `JumpToSessionPanel.tsx` | 1h | frontend |
| 3 | Render `SessionPeekModal` on mobile when session loaded | `JumpToSessionPanel.tsx` | 1h | frontend |
| 4 | Make `SessionPeekModal` full-screen on mobile | `SessionPeekModal.css.ts` | 30m | frontend |
| 5 | Mock `useViewport` in tests + add mobile modal test cases | `JumpToSessionPanel.test.tsx` | 2h | test |
| 6 | Update feature registry JSON + run `make registry-generate` | `docs/registry/features/frontend/backlog-jump-to-session.json` | 30m | docs |
| 7 | Manual QA on mobile browser at `onyx.staplerhome.internal:8444` | — | 1h | frontend |
