import { useEffect, type RefObject } from "react";
import { FOCUS_ON_NAVIGATE } from "./policy";
import { installInputStartCapture, markRouteChangeEnd } from "@/lib/perf/perfMarks";
import { usePathnameChange } from "./usePathnameChange";

const OWN_FOCUS = 'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [autofocus]';

/** Focus the layout `main` (not a page's nested one) after a route commit, unless the page already claimed focus inside it. */
export function useFocusOnNavigate(mainRef: RefObject<HTMLElement | null>): void {
  useEffect(installInputStartCapture, []);
  usePathnameChange(() => {
    markRouteChangeEnd();
    const main = mainRef.current;
    if (!FOCUS_ON_NAVIGATE || !main) return;
    const active = document.activeElement;
    if (active && active !== main && main.contains(active) && active.matches(OWN_FOCUS)) return;
    main.focus({ preventScroll: true });
  });
}
