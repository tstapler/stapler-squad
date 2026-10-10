"use client";

import { useEffect, useRef, type RefObject } from "react";

interface Options {
  /** notification_tray_v2 is on. */
  enabled: boolean;
  isOpen: boolean;
  /** The tray is a bottom sheet (owns a history entry for hardware Back). */
  isSheet: boolean;
  /** The expanded sheet is modal (fine pointer): the page behind it is inert. */
  modal: boolean;
  trayRef: RefObject<HTMLElement | null>;
  close: () => void;
}

/**
 * Every way the open tray goes away that is not a button: Esc (from inside the
 * tray or the page body, never while focus is in the terminal, TY-4), hardware
 * Back on a bottom sheet (TS-8; the entry keeps the URL and Next's own state), and
 * the modal sheet's inert background.
 */
export function useTrayDismissal({ enabled, isOpen, isSheet, modal, trayRef, close }: Options): void {
  useEffect(() => {
    if (!enabled || !isOpen) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      const target = e.target as Node | null;
      const inTray = !!target && !!trayRef.current?.contains(target);
      const onBody = target === document.body || target === document.documentElement || target === null;
      if (inTray || onBody) close();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [enabled, isOpen, trayRef, close]);

  const closeRef = useRef(close);
  closeRef.current = close;
  const ownsHistoryEntry = useRef(false);
  useEffect(() => {
    if (!enabled || !isSheet || !isOpen) return;
    window.history.pushState({ ...(window.history.state ?? {}), ssqTray: true }, "");
    ownsHistoryEntry.current = true;
    const onPopState = () => {
      ownsHistoryEntry.current = false;
      closeRef.current();
    };
    window.addEventListener("popstate", onPopState);
    return () => {
      window.removeEventListener("popstate", onPopState);
      if (ownsHistoryEntry.current) {
        ownsHistoryEntry.current = false;
        window.history.back();
      }
    };
  }, [enabled, isSheet, isOpen]);

  useEffect(() => {
    if (!modal) return;
    const main = document.getElementById("main-content");
    main?.setAttribute("inert", "");
    return () => main?.removeAttribute("inert");
  }, [modal]);
}
