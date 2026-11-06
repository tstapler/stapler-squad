// +feature: settings-programs
"use client";

import { useId, useState } from "react";
import type { FlagOption } from "@/lib/flags/flagTokens";
import { FlagInfoButton } from "./FlagInfoButton";
import * as styles from "./FlagNotes.css";

interface AvailableFlagsProps {
  flags: readonly FlagOption[];
  testId: string;
}

/** "Available flags (N)" disclosure; descriptions open via FlagInfoButton, outside any listbox. */
export function AvailableFlags({ flags, testId }: AvailableFlagsProps) {
  const [open, setOpen] = useState(false);
  const listId = useId();
  if (flags.length === 0) return null;

  const toggle = (
    // analytics-exempt
    <button
      type="button"
      className={styles.disclosureToggle}
      aria-expanded={open}
      aria-controls={listId}
      onClick={() => setOpen((v) => !v)}
      data-testid={`${testId}-toggle`}
    >
      Available flags ({flags.length}) {open ? "▴" : "▾"}
    </button>
  );

  return (
    <div data-testid={testId}>
      {toggle}
      {open && (
        <ul id={listId} className={styles.flagList}>
          {flags.map((f) => (
            <li key={f.name} className={styles.flagItem}>
              <span className={styles.flagName}>
                {f.name}
                {f.takesValue ? " <value>" : ""}
              </span>
              <FlagInfoButton flag={f.name} description={f.description} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
