"use client";

import { useLayoutEffect, useRef, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { WindowPanel } from "@/components/window/WindowPanel";
import { nextRetainedWindows, MAX_RETAINED_WINDOWS } from "@/lib/window/retainedWindows";
import type { WindowFocusMemory } from "@/lib/window/useWindowFocusMemory";
import type { NamedWindow, WindowId } from "@/lib/window/windowTypes";

export interface WindowRenderContext<TAssign> {
  active: boolean;
  /** Delivered to the active window only; hidden windows never consume an external assign. */
  externalAssign: TAssign | null;
}

interface WindowKeepAliveHostProps<TAssign> {
  windows: NamedWindow[];
  currentWindowId: WindowId;
  /** `window_keep_alive`. Off renders exactly the current window's panel. */
  keepAlive: boolean;
  focusMemory: WindowFocusMemory;
  /** Points at the active window's `tabpanel` after every commit. */
  panelRef?: RefObject<HTMLDivElement | null>;
  externalAssign?: TAssign | null;
  maxRetained?: number;
  renderWindow: (window: NamedWindow, ctx: WindowRenderContext<TAssign>) => ReactNode;
}

const PANE_LEAF_SELECTOR = '[data-testid^="pane-leaf-"]';
const PANE_LEAF_PREFIX = "pane-leaf-";

function focusedPaneId(panel: HTMLElement): string | null {
  const leaf = document.activeElement?.closest(PANE_LEAF_SELECTOR);
  const testId = leaf?.getAttribute("data-testid");
  return panel.contains(leaf ?? null) && testId ? testId.slice(PANE_LEAF_PREFIX.length) : null;
}

/**
 * Renders the current window and, with `keepAlive`, the most recent others hidden. Hidden panels use
 * `hidden` + `inert` (CSS hide, not `<Activity>`: research/activity-spike.md) so their effects and
 * terminal streams stay live. `hidden`/`inert` are applied imperatively so focus can leave a
 * deactivating panel first; `inert` on a focused subtree drops focus to `body`.
 */
export function WindowKeepAliveHost<TAssign = unknown>({
  windows,
  currentWindowId,
  keepAlive,
  focusMemory,
  panelRef,
  externalAssign = null,
  maxRetained = MAX_RETAINED_WINDOWS,
  renderWindow,
}: WindowKeepAliveHostProps<TAssign>) {
  const [retainedState, setRetained] = useState<readonly WindowId[]>([currentWindowId]);
  const retained = keepAlive ? nextRetainedWindows(retainedState, currentWindowId, windows, maxRetained) : [currentWindowId];
  if (keepAlive && retained !== retainedState) setRetained(retained);

  const panels = useRef(new Map<WindowId, HTMLDivElement>());
  const previousActive = useRef<WindowId | null>(null);

  useLayoutEffect(() => {
    const active = panels.current.get(currentWindowId) ?? null;
    if (panelRef) panelRef.current = active;
    if (active?.hidden) {
      active.hidden = false;
      active.removeAttribute("inert");
      active.scrollTop = focusMemory.recall(currentWindowId)?.scrollTop ?? 0;
    }
    for (const [id, panel] of panels.current) {
      if (id === currentWindowId || panel.hidden) continue;
      if (id === previousActive.current) {
        focusMemory.record(id, {
          paneId: focusedPaneId(panel) ?? focusMemory.recall(id)?.paneId ?? null,
          scrollTop: panel.scrollTop,
        });
      }
      if (panel.contains(document.activeElement)) active?.focus();
      panel.setAttribute("inert", ""); // attribute, not the `inert` property: jsdom lacks the property
      panel.hidden = true;
    }
    previousActive.current = currentWindowId;
  });

  const shown = windows.filter((w) => retained.includes(w.id));
  return (
    <>
      {shown.map((w) => {
        const active = w.id === currentWindowId;
        return (
          <WindowPanel
            // Flag off keeps one element across switches, as before the host existed.
            key={keepAlive ? w.id : "single"}
            windowId={w.id}
            ref={(el) => {
              if (el) panels.current.set(w.id, el);
              else panels.current.delete(w.id);
            }}
          >
            {renderWindow(w, { active, externalAssign: active ? externalAssign : null })}
          </WindowPanel>
        );
      })}
    </>
  );
}
