"use client";

import type { ReactNode } from "react";
import * as styles from "./flow.css";

interface DrillButtonProps {
  /** The `drill_key` that travels with the number; empty means no item set exists behind it. */
  drillKey: string;
  onDrill: (key: string) => void;
  children: ReactNode;
  /** Accessible name when the visible text is only a number. */
  label?: string;
  testId?: string;
}

/** Every number on the page is one of these, so it opens the items behind it. */
export function DrillButton({ drillKey, onDrill, children, label, testId }: DrillButtonProps) {
  if (drillKey === "") return <span data-testid={testId}>{children}</span>;
  return (
    <button type="button" className={styles.drillButton} aria-label={label} data-testid={testId} onClick={() => onDrill(drillKey)}>{/* analytics-exempt */}
      {children}
    </button>
  );
}
