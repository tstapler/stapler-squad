# Agent 3: Drag-Resize Implementation

## Pointer Events API Approach

Use the Pointer Events API (`onPointerDown` / `setPointerCapture` / `onPointerMove` / `onPointerUp`) rather than separate mouse and touch event listeners. This is the correct modern approach for resize handles:

```typescript
function ResizeHandle({ splitId, direction, onResize }: ResizeHandleProps) {
  const handleRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);
  const rafRef = useRef<number | null>(null);
  const pendingRatioRef = useRef<number | null>(null);

  const handlePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    // Capture the pointer to this element — subsequent events are delivered
    // to this element even if the pointer moves outside the window.
    // Cleanup is automatic when the pointer is released or cancelled.
    (e.currentTarget as HTMLDivElement).setPointerCapture(e.pointerId);
    draggingRef.current = true;
  };

  const handlePointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!draggingRef.current) return;
    const container = handleRef.current?.parentElement;
    if (!container) return;
    
    const rect = container.getBoundingClientRect();
    const newRatio = direction === "vertical"
      ? (e.clientX - rect.left) / rect.width
      : (e.clientY - rect.top) / rect.height;
    
    // Store the new ratio but don't dispatch yet — defer to rAF
    pendingRatioRef.current = clampRatio(newRatio, MIN_PX, 
      direction === "vertical" ? rect.width : rect.height);
    
    if (rafRef.current === null) {
      rafRef.current = requestAnimationFrame(() => {
        if (pendingRatioRef.current !== null) {
          onResize(splitId, pendingRatioRef.current);
          pendingRatioRef.current = null;
        }
        rafRef.current = null;
      });
    }
  };

  const handlePointerUp = (e: React.PointerEvent<HTMLDivElement>) => {
    draggingRef.current = false;
    (e.currentTarget as HTMLDivElement).releasePointerCapture(e.pointerId);
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
  };

  return (
    <div
      ref={handleRef}
      className={resizeHandle({ direction })}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}  // covers window blur / touch cancel
    />
  );
}
```

**Why `setPointerCapture` on the handle div (not document):**
- Pointer capture ensures all subsequent pointer events are delivered to the handle element even when the pointer moves outside it or the window.
- When the pointer is released or cancelled, capture is automatically released.
- No need to attach/detach global `mousemove`/`touchmove` listeners — the element handles it all.
- Works identically for mouse, stylus, and touch — no `TouchEvent` duplication.

## rAF-Based Resize Loop (No Jank)

The pattern above uses a "pending + rAF" approach:
1. `onPointerMove` stores the latest ratio in `pendingRatioRef.current` (synchronous, <1ms)
2. If no rAF is pending, schedule one
3. The rAF callback reads `pendingRatioRef.current`, dispatches the reducer action, clears itself
4. If multiple `pointermove` events arrive before the next frame, only the most recent ratio is applied — this is correct behavior (no lag buildup)

Do NOT dispatch the reducer action directly in `onPointerMove` — React state updates inside `pointermove` cause synchronous re-renders that block the compositor thread.

## Resize Handle Component Design

```typescript
// resizeHandle.css.ts
export const resizeHandle = recipe({
  base: {
    // Visual: 6px with subtle indicator
    position: "relative",
    flexShrink: 0,
    zIndex: vars.zIndex.raised,  // use zIndex token from theme-contract
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    transition: "background 120ms ease",
    userSelect: "none",
    touchAction: "none",  // prevent browser pan/zoom hijacking during drag
    
    // Indicator dots or chevrons (using pseudo-element or inner span)
    "::after": {
      content: "''",
      display: "block",
      borderRadius: vars.radii.full,
      background: vars.color.borderColor,
      opacity: 0.6,
      transition: "opacity 120ms ease, background 120ms ease",
    },
    
    ":hover::after": {
      opacity: 1,
      background: vars.color.primary,
    },
  },
  variants: {
    direction: {
      vertical: {
        // Sits in the middle column of the CSS grid
        width: "6px",         // visual size
        cursor: "col-resize",
        // Expand hit target to 20px with negative horizontal margin:
        marginLeft: "-7px",
        marginRight: "-7px",
        paddingLeft: "7px",
        paddingRight: "7px",
        "::after": {
          width: "2px",
          height: "24px",
        },
      },
      horizontal: {
        height: "6px",
        cursor: "row-resize",
        marginTop: "-7px",
        marginBottom: "-7px",
        paddingTop: "7px",
        paddingBottom: "7px",
        "::after": {
          width: "24px",
          height: "2px",
        },
      },
    },
  },
});
```

**Hit target expansion technique:** The handle renders at 6px visual size (matching the grid column/row allocation), but the negative margin + matching padding expands the actual clickable/touchable area to 20px (6px visual + 7px on each side). This keeps the visual clean while meeting the 20px mobile touch target requirement from US-5.

Note: `touchAction: "none"` is critical — without it, the browser may intercept the pointer events for pan/zoom on touch devices before they reach `onPointerMove`.

## Min-Size Clamping

```typescript
const MIN_PX = 200; // minimum pane size in pixels (matches US-4: 200px × 150px)

function clampRatio(rawRatio: number, minPx: number, totalPx: number): number {
  const minRatio = minPx / totalPx;
  return Math.max(minRatio, Math.min(1 - minRatio, rawRatio));
}
```

For `NUDGE_RESIZE` (keyboard arrow resize, 20px step from requirements):
```typescript
const nudgeRatio = (current: number, deltaPx: number, totalPx: number): number => {
  return clampRatio(current + deltaPx / totalPx, MIN_PX, totalPx);
};
```

The `containerSizePx` needed for clamping comes from a `ResizeObserver` on the split container — see next section.

## Container Size via ResizeObserver

Each `SplitPane` renderer needs to know the current container size to:
1. Clamp min ratios correctly
2. Pass `containerSizePx` to the `NUDGE_RESIZE` action

```typescript
function useSplitContainerSize(ref: RefObject<HTMLDivElement | null>) {
  const [size, setSize] = useState({ width: 0, height: 0 });
  
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      setSize({ width, height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  
  return size;
}
```

Alternatively, pass the container rect from the drag event directly (avoiding a separate observer per split node). Since `e.currentTarget.parentElement.getBoundingClientRect()` is available in `handlePointerMove`, the observer is only needed for keyboard nudge.

## vanilla-extract CSS Bridge for Runtime Ratio

The `--split-ratio` custom property approach:

```typescript
// In PaneNode renderer:
<div
  className={splitContainer({ direction: node.direction })}
  style={{ "--split-ratio": String(node.ratio) } as React.CSSProperties}
>
```

```typescript
// paneSplit.css.ts:
export const splitContainer = recipe({
  variants: {
    direction: {
      vertical: {
        gridTemplateColumns: "calc(var(--split-ratio, 0.5) * 100%) 6px 1fr",
      },
      horizontal: {
        gridTemplateRows: "calc(var(--split-ratio, 0.5) * 100%) 6px 1fr",
      },
    },
  },
});
```

The fallback `0.5` in `var(--split-ratio, 0.5)` ensures a 50/50 split renders correctly on the first paint before JavaScript sets the property.

**Important:** vanilla-extract files are build-time only. The `var(--split-ratio)` string inside a `.css.ts` file is fine — it's a CSS custom property reference, not a JavaScript variable. The actual value is injected at runtime via `style={{ "--split-ratio": ... }}`. Do NOT attempt to use `node.ratio` directly inside the `.css.ts` — it's not available at build time.

## Pointer Capture vs. Document Listener Pattern

The existing mobile gesture hooks (`useTouchScroll.ts`, `useMobileTerminalGestures.ts`) use `addEventListener` on the container with `{ passive: false }` to intercept touch events. The resize handle should NOT follow this pattern. Instead:

- **Pointer capture on the handle element** (not document): `e.currentTarget.setPointerCapture(e.pointerId)` routes all events to the handle until `pointerup` or `pointercancel`
- No global listener to add/remove
- No `useEffect` cleanup required for the drag itself (pointer capture is automatically released on pointer up)
- The only cleanup needed is cancelling any pending rAF on `pointerup`/`pointercancel`

This is strictly cleaner than the global document listener approach and avoids the race condition where a fast pointer-up can leave a stale global listener attached.
