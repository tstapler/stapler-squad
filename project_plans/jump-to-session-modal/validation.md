# Validation: Jump to Session — Modal Mode on Mobile

## Acceptance Criteria Checklist

### AC-1: Mobile opens SessionPeekModal
**Test:** `JumpToSessionPanel_should_OpenModal_When_MobileAndButtonClicked`
- Mock `useViewport()` → `{ isMobile: true }`
- Click "Jump to Work session" button
- Assert `SessionPeekModal` renders with the fetched session
- Assert no `jump-to-session-panel` inline div is present

### AC-2: Desktop keeps inline expand
**Test:** `JumpToSessionPanel_should_KeepInlineExpand_When_Desktop` (existing tests cover this with `isMobile: false`)
- Existing test `JumpToSessionPanel_should_FetchAndRenderTerminal_When_ButtonClicked` must continue passing
- Assert `data-testid="jump-to-session-panel"` is present (inline)
- Assert no `SessionPeekModal` is rendered

### AC-3: Modal closes correctly on mobile
**Test:** `JumpToSessionPanel_should_CloseModal_When_MobileAndModalClosed`
- Mock mobile, open modal
- Call the modal's `onClose` callback
- Assert modal is gone, no `SessionPeekModal` in DOM

### AC-4: Role menu still works on mobile
**Test:** `JumpToSessionPanel_should_SwitchTargetSession_When_DifferentRoleChosenFromMenu` (extend to run with `isMobile: true`)
- Mock mobile with two roles (work + review)
- Click caret, pick review
- Assert `getSession` called with review session id
- Assert `SessionPeekModal` renders (not inline panel)

### AC-5: Analytics fires on mobile
- No new test needed — existing analytics mock in the test suite captures the `track()` call
- Verify analytically that both code paths still call `track({ name: "backlog_jump_to_session", ... })`

### AC-6: SessionPeekModal full-screen on mobile
**Manual check only** (CSS-only change, no unit test needed):
- Open session peek modal on a 390px-wide viewport
- Assert modal has `inset: 0`, `border-radius: 0`
- Assert no visible gap between modal edge and viewport edge

### AC-7: No double-fetch regression
**Test:** `JumpToSessionPanel_should_NotDoubleFetch_When_MobileRoleChangedThenOpened`
- Mock mobile, open panel for role A
- Switch role (caret) to role B
- Assert `getSession` called exactly once per role selection, not twice

## CI Gates
- `make build && make test` — all existing + new unit tests green
- `make lint` — no warnings in `JumpToSessionPanel.tsx` or `SessionPeekModal.css.ts`
- `make registry-generate && git diff --exit-code docs/registry/` — no uncommitted registry drift
- `cd web-app && bun run lint:duplicates` — jscpd duplication threshold still passes

## Manual QA
1. On mobile browser (`onyx.staplerhome.internal:8444`), open a backlog item with a linked work session.
2. Tap "Jump to Work session" — confirm a full-screen modal opens with the terminal.
3. Open virtual keyboard in the terminal — confirm modal stays visible and does not get pushed off-screen (document any iOS Safari visual viewport clamping as known behavior).
4. Tap × close button — confirm modal closes and you return to the backlog item detail.
5. On desktop (> 600px), confirm existing inline-expand behavior is unchanged.
