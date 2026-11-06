"use client";

import { useEffect, useLayoutEffect, useRef, type RefObject } from "react";

/**
 * Focus policy for the non-modal tray (ADR-006, ux.md D6/C11). Restoring focus to
 * the xterm textarea re-summons the soft keyboard on a phone and votes a terminal
 * resize, so it is limited to keyboard-initiated closes and never happens on a
 * coarse pointer.
 */

export type InputMethod = "pointer" | "keyboard";

let lastInputMethod: InputMethod = "pointer";
let listenersInstalled = false;

function installInputMethodListeners(): void {
  if (listenersInstalled || typeof document === "undefined") return;
  listenersInstalled = true;
  document.addEventListener("keydown", () => (lastInputMethod = "keyboard"), true);
  document.addEventListener("pointerdown", () => (lastInputMethod = "pointer"), true);
  document.addEventListener("touchstart", () => (lastInputMethod = "pointer"), true);
}

export function currentInputMethod(): InputMethod {
  return lastInputMethod;
}

/** The xterm helper textarea (accessible name "Terminal input"). */
export function isTerminalInput(el: Element | null): boolean {
  if (!el) return false;
  return el.classList.contains("xterm-helper-textarea") || el.getAttribute("aria-label") === "Terminal input";
}

export interface CloseFocusContext {
  coarse: boolean;
  closedBy: InputMethod;
  /** The element focused when the tray opened, still in the document. */
  storedConnected: boolean;
  storedIsTerminal: boolean;
  /** Focus is inside the tray at the moment it closes (it would otherwise be lost to <body>). */
  focusInsideTray: boolean;
}

/** Whether closing the tray may call `.focus()` on the element it opened from. */
export function shouldRestoreFocus(ctx: CloseFocusContext): boolean {
  if (!ctx.storedConnected) return false;
  if (ctx.storedIsTerminal && ctx.coarse) return false;
  if (ctx.closedBy === "keyboard") return true;
  return ctx.focusInsideTray;
}

interface UseTrayFocusOptions {
  isOpen: boolean;
  trayRef: RefObject<HTMLElement | null>;
  headingRef: RefObject<HTMLElement | null>;
  coarse: boolean;
}

export function useTrayFocus({ isOpen, trayRef, headingRef, coarse }: UseTrayFocusOptions): void {
  const storedRef = useRef<HTMLElement | null>(null);
  const wasOpenRef = useRef(false);

  useEffect(installInputMethodListeners, []);

  // Layout effect: runs before the browser's focus fixup for a hidden subtree.
  useLayoutEffect(() => {
    const wasOpen = wasOpenRef.current;
    wasOpenRef.current = isOpen;
    if (isOpen && !wasOpen) {
      const active = document.activeElement;
      storedRef.current = active instanceof HTMLElement && active !== document.body ? active : null;
      if (currentInputMethod() === "keyboard") headingRef.current?.focus({ preventScroll: true });
      return;
    }
    if (!isOpen && wasOpen) {
      const stored = storedRef.current;
      storedRef.current = null;
      if (!stored) return;
      const active = document.activeElement;
      const shouldRestore = shouldRestoreFocus({
        coarse,
        closedBy: currentInputMethod(),
        storedConnected: stored.isConnected,
        storedIsTerminal: isTerminalInput(stored),
        focusInsideTray: !!active && !!trayRef.current?.contains(active),
      });
      if (shouldRestore) stored.focus({ preventScroll: true });
    }
  }, [isOpen, coarse, headingRef, trayRef]);
}
