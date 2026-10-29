import { useMemo, useRef } from "react";
import type { PaneId } from "@/lib/pane/paneTypes";
import type { WindowId } from "./windowTypes";

/** How a window switch was started; decides where focus lands afterwards (design/ux.md S3). */
export type WindowSwitchTrigger =
  | "tab-key" // Arrow/Home/End/Enter/Space on a tab: focus stays on the tab
  | "mouse"
  | "touch" // touch or pen tap: never auto-focus a terminal (opens the on-screen keyboard, BUG-110)
  | "swipe"
  | "leader"
  | "overview"
  | "programmatic";

export type FocusKind = "tab" | "pane" | "none";

export function focusKindForTrigger(trigger: WindowSwitchTrigger): FocusKind {
  if (trigger === "tab-key") return "tab";
  if (trigger === "touch" || trigger === "swipe") return "none";
  // The overview dialog cannot tell a tap from a click, so fall back to the primary pointer type.
  if (trigger === "overview" && window.matchMedia?.("(pointer: coarse)").matches) return "none";
  return "pane";
}

export interface WindowFocusSnapshot {
  paneId: PaneId | null;
  scrollTop: number;
}

export interface WindowFocusMemory {
  record: (windowId: WindowId, snapshot: WindowFocusSnapshot) => void;
  recall: (windowId: WindowId) => WindowFocusSnapshot | null;
}

/** Per-window last-focused pane and scroll offset, written on deactivation and read on reactivation. Flag-independent. */
export function useWindowFocusMemory(): WindowFocusMemory {
  const entries = useRef(new Map<WindowId, WindowFocusSnapshot>());
  return useMemo(
    () => ({
      record: (windowId, snapshot) => void entries.current.set(windowId, snapshot),
      recall: (windowId) => entries.current.get(windowId) ?? null,
    }),
    [],
  );
}

const FOCUSABLE = 'button:not([disabled]), input:not([disabled]), select:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])';

/**
 * Moves focus into `paneId` inside `panel` (the terminal first, then any focusable control), else onto the
 * panel itself. Returns the focused element, or null when there is no panel.
 */
export function focusWindowContent(panel: HTMLElement | null, paneId: PaneId | null): HTMLElement | null {
  if (!panel) return null;
  const leafTestId = `pane-leaf-${paneId}`;
  const leaf = paneId
    ? Array.from(panel.querySelectorAll<HTMLElement>('[data-testid^="pane-leaf-"]')).find(
        (el) => el.getAttribute("data-testid") === leafTestId,
      )
    : undefined;
  const target = leaf?.querySelector<HTMLElement>("textarea") ?? leaf?.querySelector<HTMLElement>(FOCUSABLE) ?? panel;
  target.focus();
  return target;
}
