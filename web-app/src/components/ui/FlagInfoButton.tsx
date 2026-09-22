// +feature: settings-programs
"use client";

import { useId, useState } from "react";
import { Icon } from "./ProbeStatusBadge";
import * as styles from "./FlagInfoButton.css";

interface FlagInfoButtonProps {
  flag: string;
  description: string;
}

/**
 * Tap-to-toggle inline description. No hover handler: every device reads it the
 * same way. Renders nothing for a flag without a description. Must stay outside
 * role=option (nested interactive content).
 */
export function FlagInfoButton({ flag, description }: FlagInfoButtonProps) {
  const [open, setOpen] = useState(false);
  const descriptionId = useId();
  if (!description) return null;

  const toggle = (
    // analytics-exempt
    <button
      type="button"
      className={styles.button}
      aria-expanded={open}
      aria-controls={descriptionId}
      aria-label={`${open ? "Hide" : "Show"} description for ${flag}`}
      onClick={() => setOpen((v) => !v)}
      data-testid="prog-flag-info-button"
    >
      <Icon name="info" />
    </button>
  );

  return (
    <>
      {toggle}
      {open && (
        <div id={descriptionId} className={styles.description} data-testid="prog-flag-info-description">
          {description}
        </div>
      )}
    </>
  );
}
