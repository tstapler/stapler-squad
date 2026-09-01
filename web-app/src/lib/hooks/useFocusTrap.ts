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

    // Computed fresh on every Tab press (not cached once at activation) so a
    // control that becomes disabled/enabled after the trap activates (e.g. a
    // dialog's input+Send disabled while its own async action is in flight)
    // doesn't leave a stale "first"/"last" pointing at an element that's no
    // longer in the tab order — chasing a stale pointer would fail to
    // preventDefault, letting Tab/Shift+Tab escape the container to whatever
    // real browser tab order finds next (confirmed via a real-browser
    // regression: SessionActionsOverflow's "Give Direction" dialog disables
    // its input/Send while steering, and a stale snapshot let Shift+Tab from
    // the still-enabled Cancel button escape the dialog entirely).
    const getFocusable = () =>
      Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)).filter(
        (el) => !el.closest("[aria-hidden='true']")
      );

    // Move focus into the container
    getFocusable()[0]?.focus();

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Tab") return;
      const focusable = getFocusable();
      if (focusable.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
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

    // Safety net for widgets that rewrite their own tabindex on focus (e.g.
    // react-arborist's FileTree), which can let native Tab slip past
    // handleKeyDown's first/last check entirely — backlog item
    // 4a1f73c4-5558-41f8-9860-8508fb874fcc.
    const handleFocusIn = (e: FocusEvent) => {
      const target = e.target as Node | null;
      if (!target || container.contains(target)) return;
      // A deliberate close-restore (e.g. a sibling trap sharing this trigger,
      // like SessionActionsOverflow's Program Picker over its overflow menu),
      // not an escape.
      if (target === triggerRef?.current) return;
      // Focus moved into another legitimate dialog — e.g. React's autoFocus
      // lands here before that dialog's own trap effect has registered.
      const targetEl = target as HTMLElement;
      if (targetEl.closest?.('[role="dialog"], [role="alertdialog"]')) return;
      (getFocusable()[0] ?? container).focus();
    };

    document.addEventListener("keydown", handleKeyDown);
    document.addEventListener("focusin", handleFocusIn);

    const triggerEl = triggerRef?.current;
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.removeEventListener("focusin", handleFocusIn);
      // Return focus to the trigger element when the trap is deactivated
      triggerEl?.focus();
    };
  }, [isActive, ref, triggerRef]);
}
