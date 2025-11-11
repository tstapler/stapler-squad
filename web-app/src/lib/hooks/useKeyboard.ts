import { useEffect, useCallback } from "react";

export type KeyHandler = () => void;
export type KeyboardHandlers = Record<string, KeyHandler>;

interface UseKeyboardOptions {
  /**
   * Whether keyboard shortcuts are enabled
   */
  enabled?: boolean;

  /**
   * Elements to ignore keyboard events from (e.g., inputs, textareas)
   */
  ignoreElements?: string[];

  /**
   * Require modifier keys (Ctrl, Alt, Meta) for shortcuts
   */
  requireModifier?: boolean;
}

/**
 * Hook for managing keyboard shortcuts and navigation
 */
export function useKeyboard(
  handlers: KeyboardHandlers,
  options: UseKeyboardOptions = {}
) {
  const {
    enabled = true,
    ignoreElements = ["INPUT", "TEXTAREA", "SELECT"],
    requireModifier = false,
  } = options;

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if (!enabled) return;

      // Ignore if we're in an input element
      const target = event.target as HTMLElement;
      if (ignoreElements.includes(target.tagName)) {
        return;
      }

      // Build key identifier with modifiers
      const key = event.key;
      const hasModifier =
        event.ctrlKey || event.altKey || event.metaKey || event.shiftKey;

      // If requireModifier is true, skip unless a modifier is pressed
      if (requireModifier && !hasModifier) {
        return;
      }

      // Check if we have a handler for this key
      if (handlers[key]) {
        event.preventDefault();
        handlers[key]();
      }
    },
    [enabled, handlers, ignoreElements, requireModifier]
  );

  useEffect(() => {
    if (!enabled) return;

    window.addEventListener("keydown", handleKeyDown);

    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [enabled, handleKeyDown]);
}

/**
 * Hook for managing arrow key navigation in a list
 */
export function useArrowNavigation(
  itemCount: number,
  onSelect?: (index: number) => void,
  options: UseKeyboardOptions & {
    initialIndex?: number;
    wrap?: boolean;
  } = {}
) {
  const { initialIndex = 0, wrap = false, ...keyboardOptions } = options;

  const [selectedIndex, setSelectedIndex] = useState(initialIndex);

  const handlers: KeyboardHandlers = {
    ArrowDown: () => {
      setSelectedIndex((prev) => {
        const next = prev + 1;
        if (next >= itemCount) {
          return wrap ? 0 : prev;
        }
        return next;
      });
    },
    ArrowUp: () => {
      setSelectedIndex((prev) => {
        const next = prev - 1;
        if (next < 0) {
          return wrap ? itemCount - 1 : prev;
        }
        return next;
      });
    },
    Home: () => setSelectedIndex(0),
    End: () => setSelectedIndex(itemCount - 1),
    Enter: () => {
      if (selectedIndex >= 0 && selectedIndex < itemCount) {
        onSelect?.(selectedIndex);
      }
    },
  };

  useKeyboard(handlers, keyboardOptions);

  return { selectedIndex, setSelectedIndex };
}

// Helper to use useState (need to import it)
import { useState } from "react";
