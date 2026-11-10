"use client";

import { useRef, type KeyboardEvent } from "react";
import { useMediaQuery } from "@/lib/hooks/useMediaQuery";
import * as styles from "./VcsWidgetPrSwitcher.css";

/** Open PRs up to this many get a segmented control; more get a select (ux.md S4). */
export const MAX_SEGMENTS = 4;

export interface PrSwitcherOption {
  /** Index into the widget's PR list. */
  index: number;
  number: number;
  label: string;
  isActive: boolean;
}

export interface VcsWidgetPrSwitcherProps {
  options: PrSwitcherOption[];
  selected: number;
  onSelect: (index: number) => void;
  /** "#12 acme/app": the PR the actions below apply to. */
  target: string;
  /** Why the selection moved ("#14 is now merged; showing #12"); announced with the target line. */
  notice?: string;
}

const optionText = (o: PrSwitcherOption) => `#${o.number}${o.isActive ? " Active" : ""}`;

/** Chooses which open PR the widget shows. Selection is local; it never changes the pin. */
export function VcsWidgetPrSwitcher({ options, selected, onSelect, target, notice }: VcsWidgetPrSwitcherProps) {
  const phone = useMediaQuery("(max-width: 480px)");
  const refs = useRef(new Map<number, HTMLButtonElement>());
  const useSelect = phone || options.length > MAX_SEGMENTS;

  const onKeyDown = (e: KeyboardEvent, at: number) => {
    const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const next = options[(at + step + options.length) % options.length];
    onSelect(next.index);
    refs.current.get(next.index)?.focus();
  };

  return (
    <div className={styles.container} data-testid="pr-switcher">
      <div className={styles.row}>
        <span id="pr-switcher-label">Pull request:</span>
        {useSelect ? (
          <select
            className={styles.select}
            aria-label="Pull request"
            value={selected}
            onChange={(e) => onSelect(Number(e.target.value))}
          >
            {options.map((o) => (
              <option key={o.index} value={o.index}>{`#${o.number} ${o.label}${o.isActive ? " (active)" : ""}`}</option>
            ))}
          </select>
        ) : (
          <div role="radiogroup" aria-labelledby="pr-switcher-label" className={styles.segments}>
            {options.map((o, at) => (
              <button
                key={o.index}
                ref={(el) => { if (el) refs.current.set(o.index, el); else refs.current.delete(o.index); }}
                type="button"
                role="radio"
                aria-checked={o.index === selected}
                tabIndex={o.index === selected ? 0 : -1}
                className={`${styles.segment} ${o.index === selected ? styles.segmentSelected : ""}`}
                onClick={() => onSelect(o.index)}
                onKeyDown={(e) => onKeyDown(e, at)}
              >{/* analytics-exempt */}
                {optionText(o)}
              </button>
            ))}
          </div>
        )}
      </div>
      <div aria-live="polite" data-testid="pr-switcher-target">
        {notice && <p className={styles.target}>{notice}</p>}
        <p className={styles.target}>Actions apply to {target}.</p>
      </div>
    </div>
  );
}
