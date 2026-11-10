"use client";

import { RadioGroup, type RadioGroupOption } from "@/components/ui/RadioGroup";
import type { FlowWindow } from "@/lib/hooks/useBacklogFlow";
import type { FlowCompare } from "./flowUrlState";
import { formatAge, formatClock } from "./flowFormat";
import { FLOW_JUMP_LINKS, type FlowSectionId } from "./flowSections";
import * as styles from "./flow.css";

export const COMPARE_ALL_TIME_HINT = "Compare needs a 7, 30 or 90 day window";

const WINDOW_OPTIONS: readonly RadioGroupOption<string>[] = [
  { value: "7", label: "7d", dataTestId: "flow-window-7" },
  { value: "30", label: "30d", dataTestId: "flow-window-30" },
  { value: "90", label: "90d", dataTestId: "flow-window-90" },
  { value: "all", label: "All time", dataTestId: "flow-window-all" },
];

interface FlowHeaderProps {
  window: FlowWindow;
  compare: FlowCompare;
  onWindowChange: (window: FlowWindow) => void;
  onCompareChange: (compare: FlowCompare) => void;
  /** Server `as_of` of the data on screen; null while nothing has loaded. */
  asOf: Date | null;
  /** Client clock, used only for the relative age. */
  now: number;
  loading: boolean;
  isRefreshing: boolean;
  onRefresh: () => void;
  slowPath: boolean;
  estimatedCount: number;
  unavailableSections: number;
  onJump: (id: FlowSectionId) => void;
}

function compareOptions(allTime: boolean): readonly RadioGroupOption<string>[] {
  return [
    { value: "none", label: "none", dataTestId: "flow-compare-none" },
    { value: "prev", label: "previous period", dataTestId: "flow-compare-prev", disabled: allTime },
  ];
}

/** Window and compare selectors, freshness, refresh and the jump list (UX S2). */
export function FlowHeader(props: FlowHeaderProps) {
  const { window, compare, asOf, now, loading, isRefreshing, slowPath, estimatedCount, unavailableSections } = props;
  const allTime = window === "all";

  return (
    <header className={styles.header}>
      <h1 className={styles.title}>Flow</h1>
      <div className={styles.controls}>
        <RadioGroup
          groupLabel="Window"
          options={WINDOW_OPTIONS}
          value={String(window)}
          onChange={(value) => props.onWindowChange(value === "all" ? "all" : (Number(value) as FlowWindow))}
        />
        <RadioGroup
          groupLabel="Compare"
          options={compareOptions(allTime)}
          value={allTime ? "none" : compare}
          onChange={(value) => props.onCompareChange(value === "prev" ? "prev" : "none")}
          hintForValue={() => (allTime ? COMPARE_ALL_TIME_HINT : undefined)}
        />
      </div>
      {allTime && slowPath && <p className={styles.hint}>All time reads the full event history (slow path), so it can take a few seconds.</p>}
      <div className={styles.facts}>
        <span data-testid="flow-as-of" title={asOf?.toString()}>
          {asOf ? `As of ${formatClock(asOf, true)} (${formatAge(now - asOf.getTime())})` : loading ? "Loading..." : ""}
        </span>
        <button
          type="button"
          className={styles.textButton}
          aria-disabled={isRefreshing}
          onClick={() => !isRefreshing && props.onRefresh()}
          data-testid="flow-refresh"
        >{/* analytics-exempt */}
          {isRefreshing ? "Refreshing..." : "Refresh"}
        </button>
        {estimatedCount > 0 && (
          <button type="button" className={styles.linkButton} onClick={() => props.onJump("notes")}>{/* analytics-exempt */}
            {estimatedCount} {estimatedCount === 1 ? "metric" : "metrics"} estimated
          </button>
        )}
        {unavailableSections > 0 && (
          <span data-testid="flow-unavailable-count">
            {unavailableSections} {unavailableSections === 1 ? "section" : "sections"} unavailable
          </span>
        )}
      </div>
      <nav className={styles.jumpList} aria-label="Jump to section">
        <span>Jump to:</span>
        {FLOW_JUMP_LINKS.map((link) => (
          <a
            key={link.id}
            href={`#${link.id}`}
            className={styles.jumpLink}
            onClick={(event) => {
              event.preventDefault();
              props.onJump(link.id);
            }}
          >{/* analytics-exempt */}
            {link.label}
          </a>
        ))}
      </nav>
    </header>
  );
}
