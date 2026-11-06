"use client";

import { useEffect, type RefObject } from "react";
import { usePanelHistory } from "./usePanelHistory";

interface Options {
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
export function useTrayDismissal({
  isOpen,
  isSheet,
  modal,
  trayRef,
  close,
}: Options): { releaseHistoryEntry: () => void } {
  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      const target = e.target as Node | null;
      const inTray = !!target && !!trayRef.current?.contains(target);
      const onBody = target === document.body || target === document.documentElement || target === null;
      if (inTray || onBody) close();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isOpen, trayRef, close]);

  const { releaseHistoryEntry } = usePanelHistory({ active: isSheet && isOpen, onBack: close });

  useEffect(() => {
    if (!modal) return;
    const main = document.getElementById("main-content");
    main?.setAttribute("inert", "");
    return () => main?.removeAttribute("inert");
  }, [modal]);

  return { releaseHistoryEntry };
}
