import { RefObject, MutableRefObject, useEffect } from "react";

const FOCUSABLE_SELECTORS =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Traps keyboard focus within a container element when active.
 * Moves focus to the first focusable element on activation and
 * returns focus to the trigger element on deactivation.
 *
 * @param ref - Ref to the container element that should trap focus
 * @param isActive - Whether the trap is currently active
 * @param triggerRef - Optional ref to the element that opened the trap (focus returns here on close)
 */
type AnyElementRef = RefObject<HTMLElement | null> | MutableRefObject<HTMLElement | null>;

export function useFocusTrap(
  ref: AnyElementRef,
  isActive: boolean,
  triggerRef?: AnyElementRef
) {
  useEffect(() => {
    if (!isActive || !ref.current) return;

    const container = ref.current as HTMLElement;
    const focusable = Array.from(
      container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)
    ).filter((el) => !el.closest("[aria-hidden='true']"));

    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    // Move focus into the container
    first?.focus();

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Tab") return;
      if (focusable.length === 0) {
        e.preventDefault();
        return;
      }
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        }
      } else {
        if (document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };

    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      // Return focus to the trigger element when the trap is deactivated
      triggerRef?.current?.focus();
    };
  }, [isActive, ref, triggerRef]);
}
