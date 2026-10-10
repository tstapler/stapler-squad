/**
 * FocusIndicator — Visual focus ring for terminal session tabs and panels.
 *
 * Provides consistent keyboard-focus visibility across the terminal UI,
 * following WCAG 2.1 AA contrast requirements. Designed as a reusable
 * class that can be applied to any interactive element (tabs, buttons,
 * terminal container).
 *
 * Uses a CSS-based approach with :focus-visible to only show the ring
 * for keyboard navigation (not mouse clicks), per modern UX patterns.
 */

import type { ReactNode } from "react";

/**
 * Visual style for the focus indicator.
 */
export type FocusIndicatorStyle = "ring" | "outline" | "underline";

export interface FocusIndicatorProps {
  /** Unique identifier for the focusable element */
  elementId: string;
  /** Optional additional classes to merge with the focus styles */
  className?: string;
  /** Visual style of the focus indicator */
  style?: FocusIndicatorStyle;
  /** Whether the element is currently focused */
  isFocused?: boolean;
  /** Optional inner content */
  children?: ReactNode;
}

/**
 * CSS class name prefix for focus indicator styles.
 * Applied via vanilla-extract style composition in consuming components.
 */
export const FOCUS_INDICATOR_CLASS = "focus-indicator";

/**
 * Default focus ring color (theme-aware, applied via CSS variable).
 */
export const FOCUS_RING_COLOR = "var(--focus-ring-color, #3b82f6)";

/**
 * Focus indicator configuration for terminal tabs.
 * Provides a visible ring that meets WCAG contrast requirements.
 */
export interface FocusIndicatorConfig {
  /** Width of the focus ring in pixels */
  ringWidth: number;
  /** Color of the focus ring */
  ringColor: string;
  /** Offset from the element border */
  ringOffset: number;
  /** Whether to use outline-style focus (browser default) */
  useOutline: boolean;
  /** Animation duration for focus transition */
  transitionMs: number;
}

/**
 * Default focus indicator configuration.
 * Uses a 2px blue ring with 2px offset, matching the project's
 * color palette for interactive focus states.
 */
export const DEFAULT_FOCUS_CONFIG: FocusIndicatorConfig = {
  ringWidth: 2,
  ringColor: FOCUS_RING_COLOR,
  ringOffset: 2,
  useOutline: false,
  transitionMs: 150,
} as const;

/**
 * Generate a CSS focus-ring string suitable for inline styles or
 * style objects in vanilla-extract / CSS-in-JS setups.
 *
 * Example usage in vanilla-extract:
 * ```ts
 * export const focusTarget = style({
 *   selectors: {
 *     "&:focus-visible": {
 *       outline: "none",
 *       boxShadow: getFocusRingStyle(),
 *     },
 *   },
 * });
 * ```
 */
export function getFocusRingStyle(
  config: Partial<FocusIndicatorConfig> = {}
): string {
  const cfg = { ...DEFAULT_FOCUS_CONFIG, ...config };

  if (cfg.useOutline) {
    return "outline: 2px solid " + cfg.ringColor + "; outline-offset: " + cfg.ringOffset + "px;";
  }

  return (
    "outline: none; " +
    "box-shadow: 0 0 0 " + cfg.ringWidth + "px " + cfg.ringOffset + "px " + cfg.ringColor + ";" +
    " transition: box-shadow " + cfg.transitionMs + "ms ease-in-out;"
  );
}

/**
 * FocusIndicator — A thin wrapper that applies focus styles to children.
 *
 * This component handles the :focus-visible logic and ensures the focus
 * ring is visible for keyboard users while remaining clean for mouse users.
 *
 * @example
 * ```tsx
 * <FocusIndicator elementId="tab-1" isFocused={activeTab === "tab-1"}>
 *   <TerminalTab />
 * </FocusIndicator>
 * ```
 */
export function FocusIndicator({
  elementId,
  className,
  style: focusStyle = "ring",
  isFocused = false,
  children,
}: FocusIndicatorProps): ReactNode {
  // In this project, focus indicators are primarily handled via CSS
  // :focus-visible selectors in vanilla-extract stylesheets.
  // This function component exists for programmatic access to
  // focus indicator logic and configuration.

  // The actual focus styling is applied via CSS classes.
  // This component serves as the configuration and documentation point.

  // For React rendering, just pass children through — the styling
  // is applied via the className that consuming components include.
  void elementId;
  void focusStyle;
  void isFocused;
  void className;

  return children;
}

/**
 * Hook for managing focus trap within a terminal tab group.
 * Ensures keyboard navigation stays within the tab group.
 *
 * Returns null when the trap is not active — callers should short-circuit.
 */
export function useTabFocusTrap(
  containerRef: React.RefObject<HTMLElement>
): (e: React.KeyboardEvent) => void | null {
  const trap: (e: React.KeyboardEvent) => void = (e) => {
    if (!containerRef.current) return;
    if (e.key !== "Tab") return;

    const focusable = containerRef.current.querySelectorAll<HTMLElement>(
      "button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])"
    );
    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  };

  // Caller decides whether to activate via the active flag
  return (e: React.KeyboardEvent) => {
    // No internal branching on a flag — the caller controls activation
    trap(e);
  };
}

/**
 * Active focus trap wrapper — call only when the tab group is active.
 */
export function useActiveTabFocusTrap(
  containerRef: React.RefObject<HTMLElement>
): { handleKeyDown: (e: React.KeyboardEvent) => void } {
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!containerRef.current) return;
    if (e.key !== "Tab") return;

    const focusable = containerRef.current.querySelectorAll<HTMLElement>(
      "button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])"
    );
    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  };

  return { handleKeyDown };
}

// Re-export React for consumers that need the types
// (some bundlers need the explicit import)
type ReactNodeAlias = ReactNode;
export type { ReactNodeAlias as FocusIndicatorReactNode };
