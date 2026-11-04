import { forwardRef } from "react";
import type { ReactNode } from "react";
import { windowPanelDomId, windowTabDomId } from "@/lib/window/windowDomIds";
import type { WindowId } from "@/lib/window/windowTypes";
import { windowPanel } from "./WindowPanel.css";

interface WindowPanelProps {
  windowId: WindowId;
  children: ReactNode;
}

/**
 * The `tabpanel` a window's tab controls. Rendered regardless of `window_keep_alive`; focusable
 * (`tabIndex=-1`) so an empty window still has somewhere for focus to land. No transitions by design.
 */
export const WindowPanel = forwardRef<HTMLDivElement, WindowPanelProps>(function WindowPanel(
  { windowId, children },
  ref,
) {
  return (
    <div
      ref={ref}
      role="tabpanel"
      id={windowPanelDomId(windowId)}
      aria-labelledby={windowTabDomId(windowId)}
      tabIndex={-1}
      data-testid="window-panel"
      className={windowPanel}
    >
      {children}
    </div>
  );
});
