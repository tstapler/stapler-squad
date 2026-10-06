"use client";
// +feature: terminal-scroll-settings

import { useCallback, useId, useState, type RefObject } from "react";
import { srOnly } from "@/components/ui/LiveRegion.css";
import { MIN_ROWS_FOR_OVERLAYS } from "@/lib/terminal/scrollPosition";
import type { ScrollOverride, ScrollTarget } from "@/lib/terminal/scrollRouting";
import * as styles from "./ScrollingPanel.css";

export { ScrollModeChip, useTouchCapable, useMisrouteCue, routeLabel } from "./ScrollModeChip";
export type { ScrollModeChipProps, MisrouteInfo } from "./ScrollModeChip";

/** True when opening the panel inline would leave too few terminal rows, so it must overlay instead. */
export function shouldRenderPanelAsOverlay(rowsLeftIfInline: number): boolean {
  return rowsLeftIfInline < MIN_ROWS_FOR_OVERLAYS;
}

interface OptionDef {
  value: ScrollOverride;
  label: string;
  announce: string;
  description: string;
}

export const SCROLL_OPTIONS: readonly OptionDef[] = [
  { value: "auto", label: "Auto (recommended)", announce: "Auto", description: "Picks for you based on what is running." },
  {
    value: "local",
    label: "Terminal history",
    announce: "Terminal history",
    description: "Dragging moves the terminal's own scrollback, following your finger. Use in a plain shell.",
  },
  {
    value: "tui",
    label: "Page keys",
    announce: "Page keys",
    description: "Dragging sends Page Up/Down to the app, one page per half-screen. Use for Claude Code or tmux history.",
  },
];

export const GESTURE_DESCRIPTION =
  "Turn off if you use a screen reader (TalkBack). Off disables all touch gestures on the terminal: drag scrolling, tap to focus, double-tap and long-press selection. Use the PgUp/PgDn keys to scroll.";
export const WIDE_OUTPUT_NOTE = "Long lines wrap; wide output cannot be panned sideways.";
export const TMUX_NOTE =
  "In tmux, Page keys scroll tmux history. If typing seems ignored, tap PgDn until you reach the bottom. Esc is not offered because it interrupts a running Claude Code turn.";

export interface ScrollingPanelProps {
  variant: "picker" | "full";
  override: ScrollOverride;
  onOverrideChange: (value: ScrollOverride) => void;
  /** Effective route, shown as "now: ..." under Auto. */
  effectiveTarget: ScrollTarget;
  gestureScrollEnabled: boolean;
  onGestureScrollChange: (on: boolean) => void;
  onClose: () => void;
  /** Picker only: opens the full panel. */
  onOpenFull: () => void;
  /** Focus returns here on close/select. */
  openerRef?: RefObject<HTMLElement | null>;
  /** Host computes via shouldRenderPanelAsOverlay. */
  renderAsOverlay: boolean;
  /** Overlay only: viewport minus toolbar minus MIN_ROWS_FOR_OVERLAYS rows. */
  maxHeight?: number | string;
}

export function ScrollingPanel(props: ScrollingPanelProps) {
  const { variant, override, effectiveTarget, gestureScrollEnabled, openerRef, renderAsOverlay, maxHeight } = props;
  const uid = useId();
  const [announcement, setAnnouncement] = useState("");

  const returnFocus = useCallback(() => openerRef?.current?.focus(), [openerRef]);
  const close = () => {
    props.onClose();
    returnFocus();
  };

  const select = (opt: OptionDef) => {
    props.onOverrideChange(opt.value);
    setAnnouncement(`Scroll mode: ${opt.announce}`);
    if (variant === "picker") close();
  };

  const nowText = effectiveTarget === "xterm-local" ? "Terminal history" : "Page keys";
  const isFull = variant === "full";

  return (
    <div
      className={styles.panel}
      data-testid={`scrolling-${variant}`}
      data-overlay={renderAsOverlay}
      style={renderAsOverlay && maxHeight !== undefined ? { maxHeight } : undefined}
    >
      <fieldset className={styles.fieldset}>
        <legend className={styles.legend}>How dragging scrolls</legend>
        {SCROLL_OPTIONS.map((opt) => {
          const id = `${uid}-${opt.value}`;
          const descId = `${id}-desc`;
          return (
            <label key={opt.value} className={styles.option} htmlFor={id}>
              <input
                id={id}
                className={styles.optionInput}
                type="radio"
                name={`${uid}-scroll-mode`}
                value={opt.value}
                checked={override === opt.value}
                onChange={() => select(opt)}
                aria-describedby={descId}
              />
              <span className={styles.optionText}>
                <span className={styles.optionLabel}>{opt.label}</span>
                {opt.value === "auto" && <span>now: {nowText}</span>}
                <span id={descId} className={styles.description}>
                  {opt.description}
                </span>
              </span>
            </label>
          );
        })}
      </fieldset>

      {isFull && (
        <>
          <label className={styles.switchRow}>
            <input
              type="checkbox"
              role="switch"
              checked={gestureScrollEnabled}
              onChange={(e) => {
                props.onGestureScrollChange(e.target.checked);
                setAnnouncement(`Touch gestures: ${e.target.checked ? "on" : "off"}`);
              }}
              aria-describedby={`${uid}-gesture-desc`}
              className={styles.optionInput}
            />
            <span className={styles.optionText}>
              <span className={styles.optionLabel}>Touch gestures (scroll, select)</span>
              <span id={`${uid}-gesture-desc`} className={styles.description}>
                {GESTURE_DESCRIPTION}
              </span>
            </span>
          </label>
          <p className={styles.note}>{WIDE_OUTPUT_NOTE}</p>
          <p className={styles.note}>{TMUX_NOTE}</p>
        </>
      )}

      {!isFull && (
        <button type="button" className={styles.button} onClick={props.onOpenFull}>
          More scrolling settings
        </button>
      )}
      <button type="button" className={styles.button} onClick={close}>
        Close
      </button>

      <div role="status" aria-live="polite" className={srOnly} data-testid="scrolling-announcer">
        {announcement}
      </div>
    </div>
  );
}
