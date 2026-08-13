"use client";

import * as styles from "./OmnibarModeBadge.css";

interface OmnibarModeBadgeProps {
  mode: "discovery" | "creation";
  onToggle: () => void;
}

export function OmnibarModeBadge({ mode, onToggle }: OmnibarModeBadgeProps) {
  return (
    <div className={styles.badgeContainer} role="group" aria-label="Omnibar mode">
      <button
        className={[styles.badgeButton, mode === "discovery" ? styles.badgeActive : ""].filter(Boolean).join(" ")}
        onClick={mode !== "discovery" ? onToggle : undefined}
        aria-pressed={mode === "discovery"}
        title="Jump to existing session (Cmd+K)"
        type="button"
      >
        Jump
      </button>
      <button
        className={[styles.badgeButton, mode === "creation" ? styles.badgeActive : ""].filter(Boolean).join(" ")}
        onClick={mode !== "creation" ? onToggle : undefined}
        aria-pressed={mode === "creation"}
        title="Create new session (Cmd+Shift+K)"
        type="button"
      >
        Create
      </button>
    </div>
  );
}
