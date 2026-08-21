"use client";

import type { FieldSource } from "@/lib/hooks/useSessionDefaults";
import * as styles from "./SourceBadge.css";

interface SourceBadgeProps {
  source: FieldSource;
  detail?: string;
}

export function SourceBadge({ source, detail }: SourceBadgeProps) {
  if (source === "none") {
    return null;
  }

  let label: string;
  switch (source) {
    case "global":
      label = "from defaults";
      break;
    case "directory":
      label = detail ? `from directory` : "from directory";
      break;
    case "profile":
      label = detail ? `from ${detail}` : "from profile";
      break;
  }

  return <span className={styles.badge}>{label}</span>;
}
