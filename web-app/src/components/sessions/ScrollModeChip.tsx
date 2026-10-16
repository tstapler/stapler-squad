"use client";
// +feature: terminal-scroll-settings

import { useCallback, useEffect, useRef, useState } from "react";
import { srOnly } from "@/components/ui/LiveRegion.css";
import { MIN_ROWS_FOR_OVERLAYS } from "@/lib/terminal/scrollPosition";
import type { ScrollTarget } from "@/lib/terminal/scrollRouting";
import * as styles from "./ScrollingPanel.css";

export const MISROUTE_HIGHLIGHT_MS = 5000;
export const MISROUTE_ANNOUNCE_INTERVAL_MS = 30000;
export const MISROUTE_ANNOUNCEMENT =
  "The drag did not scroll. Scroll mode is Terminal history. Tap the chip to change.";

export function routeLabel(target: ScrollTarget): string {
  return target === "xterm-local" ? "Terminal history" : "Page keys";
}

/**
 * True when the device has a coarse pointer or a touchstart has been seen on the surface.
 * `getSurface` is read on mount; without a surface the document is watched instead.
 */
export function useTouchCapable(getSurface?: () => EventTarget | null): boolean {
  const [touchSeen, setTouchSeen] = useState(false);
  const coarse =
    typeof window !== "undefined" && typeof window.matchMedia === "function"
      ? window.matchMedia("(any-pointer: coarse)").matches
      : false;

  useEffect(() => {
    const target = getSurface?.() ?? document;
    const onTouch = () => setTouchSeen(true);
    target.addEventListener("touchstart", onTouch, { passive: true, once: true });
    return () => target.removeEventListener("touchstart", onTouch);
  }, [getSurface]);

  return coarse || touchSeen;
}

export interface MisrouteInfo {
  route: ScrollTarget;
  postSlopLines: number;
  viewportYChanged: boolean;
}

export interface MisrouteCueOptions {
  now?: () => number;
  highlightMs?: number;
  announceIntervalMs?: number;
}

/** Misroute cue (ux.md S6): a local drag that moved nothing highlights the chip and announces at most once per interval. */
export function useMisrouteCue(options: MisrouteCueOptions = {}) {
  const { now = Date.now, highlightMs = MISROUTE_HIGHLIGHT_MS, announceIntervalMs = MISROUTE_ANNOUNCE_INTERVAL_MS } = options;
  const [highlighted, setHighlighted] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastAnnounceAt = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const report = useCallback(
    ({ route, postSlopLines, viewportYChanged }: MisrouteInfo) => {
      if (route !== "xterm-local" || postSlopLines < 1 || viewportYChanged) return;
      setHighlighted(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        setHighlighted(false);
        setAnnouncement("");
        timer.current = null;
      }, highlightMs);
      const t = now();
      if (lastAnnounceAt.current === null || t - lastAnnounceAt.current >= announceIntervalMs) {
        lastAnnounceAt.current = t;
        setAnnouncement(MISROUTE_ANNOUNCEMENT);
      }
    },
    [now, highlightMs, announceIntervalMs],
  );

  return { highlighted, announcement, report };
}

export interface ScrollModeChipProps {
  effectiveTarget: ScrollTarget;
  gestureScrollEnabled: boolean;
  onClick: () => void;
  visibleRows: number;
  highlighted?: boolean;
  /** Misroute announcement text from useMisrouteCue. */
  announcement?: string;
  /** True while the panel/picker is open: it owns the live region, so the chip stays silent. */
  suppressAnnouncements?: boolean;
  /** Returns the terminal container; touchstart on it reveals the chip. */
  getTouchSurface?: () => EventTarget | null;
  buttonRef?: React.Ref<HTMLButtonElement>;
}

export function ScrollModeChip(props: ScrollModeChipProps) {
  const touchCapable = useTouchCapable(props.getTouchSurface);
  if (!touchCapable || props.visibleRows < MIN_ROWS_FOR_OVERLAYS) return null;

  const label = routeLabel(props.effectiveTarget);
  const gestures = props.gestureScrollEnabled ? "on" : "off";
  const text = `${props.highlighted ? "! " : ""}${label}${props.gestureScrollEnabled ? "" : ", gestures off"}`;
  return (
    <>
      <button
        type="button"
        ref={props.buttonRef}
        className={styles.chip}
        data-highlighted={props.highlighted ? "true" : "false"}
        aria-label={`Scroll mode: ${label}. Touch gestures ${gestures}. Opens scroll settings`}
        onClick={props.onClick}
      >
        {text}
      </button>
      <div role="status" aria-live="polite" className={srOnly} data-testid="scroll-chip-announcer">
        {props.suppressAnnouncements ? "" : (props.announcement ?? "")}
      </div>
    </>
  );
}
