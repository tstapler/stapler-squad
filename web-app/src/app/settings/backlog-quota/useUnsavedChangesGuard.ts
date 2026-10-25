"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { pushHistoryMarker, useRouter } from "@/lib/nav";

type PendingLeave = { kind: "back" } | { kind: "link"; href: string };

export interface UnsavedChangesGuard {
  /** True while the "Discard unsaved changes?" dialog should be shown. */
  promptOpen: boolean;
  keepEditing: () => void;
  discard: () => void;
}


function isPlainSameOriginNavigation(a: HTMLAnchorElement, e: MouseEvent): boolean {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return false;
  if ((a.target && a.target !== "_self") || a.hasAttribute("download")) return false;
  const url = new URL(a.href, window.location.href);
  if (url.origin !== window.location.origin) return false;
  return url.pathname + url.search !== window.location.pathname + window.location.search;
}

/**
 * Leave guard for a form with unsaved edits. The Next.js app router has no
 * route-change veto, so a programmatic router.push from other code is not covered.
 * Covers tab close/reload (beforeunload), same-origin link clicks (capture phase),
 * and browser Back (a same-URL history sentinel pushed when the form turns dirty).
 */
export function useUnsavedChangesGuard({
  dirty,
  savePending,
}: {
  dirty: boolean;
  savePending: boolean;
}): UnsavedChangesGuard {
  const router = useRouter();
  const [pending, setPending] = useState<PendingLeave | null>(null);
  const dirtyRef = useRef(dirty);
  const armedRef = useRef(false);
  dirtyRef.current = dirty;

  const arm = useCallback(() => {
    pushHistoryMarker("ssqLeaveGuard", true);
    armedRef.current = true;
  }, []);

  useEffect(() => {
    if (!dirty && !savePending) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [dirty, savePending]);

  useEffect(() => {
    if (dirty && !armedRef.current) arm();
  }, [dirty, arm]);

  useEffect(() => {
    const onPopState = () => {
      if (!armedRef.current) return;
      armedRef.current = false;
      if (dirtyRef.current) setPending({ kind: "back" });
      else window.history.back();
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  useEffect(() => {
    if (!dirty) return;
    const onClick = (e: MouseEvent) => {
      const anchor = (e.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
      if (!anchor || !isPlainSameOriginNavigation(anchor, e)) return;
      e.preventDefault();
      e.stopPropagation();
      setPending({ kind: "link", href: anchor.getAttribute("href") ?? anchor.href });
    };
    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, [dirty]);

  const keepEditing = useCallback(() => {
    if (!armedRef.current) arm();
    setPending(null);
  }, [arm]);

  const discard = useCallback(() => {
    const leave = pending;
    setPending(null);
    if (!leave) return;
    if (leave.kind === "back") {
      window.history.back();
      return;
    }
    if (armedRef.current) {
      armedRef.current = false;
      router.replace(leave.href);
    } else {
      router.push(leave.href);
    }
  }, [pending, router]);

  return { promptOpen: pending !== null, keepEditing, discard };
}
