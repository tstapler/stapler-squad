# Validation: Session Peek Mobile

## AC Coverage Check

| AC | Covered by plan? | Notes |
|----|-----------------|-------|
| 1. Mobile sheet has "Temporary modal" button between "+ New full-screen pane" and panes list | ✅ | Exact insertion point specified (after line 371) |
| 2. Tapping opens SessionPeekModal and dismisses sheet | ✅ | `setPeekSession(mobilePickerSession); cancelPicker()` |
| 3. Modal behavior identical to desktop (Escape, backdrop, full detail, Peek badge) | ✅ | Same `SessionPeekModal` component, no changes to it |
| 4. `data-testid="mobile-picker-peek-modal"` | ✅ | Specified in JSX |
| 5. Test in mobilePicker.test.tsx | ✅ | Task 2 |
| 6. Existing tests pass | ✅ | No existing code modified |

## Gaps / Open Questions

1. **Copy**: "Temporary modal" vs "Open as overlay" vs "Open in modal" — implementer's choice. The desktop label is "Open in Modal" so consistency would favor "Open in modal" on mobile too.

2. **Icon glyph**: ◻ (U+25FB) is not universally available in all font fallback chains. A simple emoji like "⬡" or just using no icon (matching the desktop button which uses no icon prefix) is safer.

3. **Keyboard accessibility on mobile sheet**: The existing mobile picker buttons do not have explicit keyboard handlers (the sheet itself handles Escape). No change needed.

4. **Analytics**: All buttons in the mobile sheet have `{/* analytics-exempt */}` comments per the existing pattern. The new button should follow suit (already in the plan).

5. **Focus management**: `SessionPeekModal` calls `modalRef.current?.focus()` on mount, which is correct.

## Verdict
Plan is complete, well-scoped, and low-risk. The one ambiguity (copy/icon) does not block implementation — the implementer can decide. No backend work, no new components, no routing changes. Estimate of ~1h is realistic.
