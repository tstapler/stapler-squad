# Plan: Fix Mobile Bell Dead Spot

## Chosen Approach: CSS Custom Property `--tray-handle-width`

Mirrors the established `--bottom-nav-height` pattern. TrayHandle publishes its own width as a CSS variable; scroll containers consume it. No hardcoded pixel values in layout code.

## Implementation Steps

### Step 1: Publish `--tray-handle-width` from TrayHandle

In `TrayHandle.tsx`, add a `useEffect` (similar to BottomNav's height publisher) that sets `--tray-handle-width` on `document.documentElement` when the component mounts and clears it on unmount:

```tsx
useEffect(() => {
  document.documentElement.style.setProperty("--tray-handle-width", "44px");
  return () => {
    document.documentElement.style.setProperty("--tray-handle-width", "0px");
  };
}, []);
```

This runs only when TrayHandle actually renders (i.e., when the bell is visible), so the variable is only non-zero when the bell is active.

### Step 2: Apply `paddingRight` to `sessionListScroll` (`paneSplit.css.ts`)

```ts
export const sessionListScroll = style({
  flex: 1,
  overflowY: "auto",
  minHeight: 0,
  paddingRight: "var(--tray-handle-width, 0px)",
});
```

This ensures the session list content doesn't underlap the bell at any viewport width where TrayHandle is visible.

### Step 3: Apply `paddingRight` to `listPane` in backlog (`backlog.css.ts`)

```ts
export const listPane = style({
  flex: 1,
  overflow: "auto",
  padding: vars.space["4"],
  paddingRight: "calc(var(--tray-handle-width, 0px) + " + vars.space["4"] + ")",
});
```

Or equivalently split the padding shorthand so the right value accounts for the tray:
```ts
paddingRight: `calc(var(--tray-handle-width, 0px) + ${vars.space["4"]})`,
```

### Step 4: Verify no other full-width scroll containers are affected

Check additional list pages (notifications page, unfinished page, review queue, etc.) for similar full-width scrollable containers that sit at the right viewport edge.

### Step 5: Add a regression test

Extend `SessionListScrollWrapper.test.tsx` with a test that confirms:
- When TrayHandle is visible, the scroll container has right padding set via the custom property

Alternatively add a snapshot or CSS-variable-aware test that documents the layout contract.

### Step 6: Manual verification

At 900–1099px viewport width:
1. Open session list
2. Confirm all list item rows are clickable including the right portion
3. Confirm the bell is still clickable
4. Confirm tray open/close works

## Files Changed

| File | Change |
|---|---|
| `web-app/src/components/ui/TrayHandle.tsx` | Publish `--tray-handle-width: 44px` on mount, `0px` on unmount |
| `web-app/src/styles/pane/paneSplit.css.ts` | Add `paddingRight: "var(--tray-handle-width, 0px)"` to `sessionListScroll` |
| `web-app/src/app/backlog/backlog.css.ts` | Add tray-width offset to `listPane` right padding |
| `web-app/src/components/pane/__tests__/SessionListScrollWrapper.test.tsx` | Add test asserting right padding via custom property |

## Risk Assessment

**Low risk.** The change is purely additive CSS and a new `useEffect` that sets a CSS variable. No business logic changes. The CSS variable falls back to `0px` everywhere except when TrayHandle is mounted, so existing tests and layouts are unaffected.

**Edge case: tray open state.** When the tray is open, the handle moves to `right: min(400px, 40vw)`. The 44px padding remains, which is conservative but correct — the handle still occupies some right-edge width in the open state. If the tray moves fully out of list territory on open, the padding could be relaxed, but this is acceptable for now.

**Edge case: phone floating bell.** When the phone floating bell is opt-in (`floatingBellEnabled`), TrayHandle renders on mobile too. The CSS variable will be set, applying padding to the session list on phone. This is correct — the bell still overlaps content on phone when it's enabled.
