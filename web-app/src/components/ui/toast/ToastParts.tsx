"use client";

import {
  reveal,
  revealEnd,
  closeButton,
  closeButtonStacked,
  trayButton,
  commandText,
  commandTextExpanded,
  disclosureButton,
  riskLabel,
} from "../NotificationToast.css";
import { RISK_LEVEL_LABELS, type RiskLevel } from "@/lib/sessions/riskLevel";

/**
 * The card's close control. Legacy cards keep the x. In the deck an informational
 * toast shows an x ("Dismiss notification") and a pinned one a tray icon plus the
 * visible word "Tray" ("Move to tray"), so a touch user can see it is not a
 * dismissal. Both are at least 44x44.
 */
export function ToastCloseControl({
  stacked,
  pinned,
  onClose,
}: {
  stacked: boolean;
  pinned: boolean;
  onClose: () => void;
}) {
  if (!stacked) {
    return (
      <button className={closeButton} onClick={onClose} aria-label="Close notification">
        ×
      </button>
    );
  }
  if (pinned) {
    return (
      <button
        type="button"
        className={trayButton}
        aria-label="Move to tray"
        data-testid="toast-move-to-tray"
        onClick={onClose}
      >
        <span aria-hidden="true">📥</span>
        <span>Tray</span>
      </button>
    );
  }
  return (
    <button
      type="button"
      className={closeButtonStacked}
      aria-label="Dismiss notification"
      data-testid="toast-dismiss"
      onClick={onClose}
    >
      ×
    </button>
  );
}

/** The command an approval is for: wrapped, clamped to four lines until "Show full command". */
export function ToastCommand({
  command,
  riskLevel,
  expanded,
  onToggle,
}: {
  command: string;
  riskLevel: string | undefined;
  expanded: boolean;
  onToggle: () => void;
}) {
  const risk =
    riskLevel && riskLevel in RISK_LEVEL_LABELS
      ? `Risk: ${RISK_LEVEL_LABELS[riskLevel as RiskLevel]}`
      : "Risk not recorded";
  return (
    <>
      <span className={riskLabel} data-testid="toast-risk">
        {risk}
      </span>
      <p
        className={`${commandText} ${expanded ? commandTextExpanded : ""}`}
        data-testid="toast-command"
        data-expanded={expanded ? "true" : "false"}
      >
        {command}
      </p>
      <button
        type="button"
        className={disclosureButton}
        data-testid="toast-show-command"
        aria-expanded={expanded}
        onClick={onToggle}
      >
        {expanded ? "Hide full command" : "Show full command"}
      </button>
    </>
  );
}

/** Layer behind a dragged card: what releasing will do. */
export function SwipeReveal({ pinned, toTheLeft }: { pinned: boolean; toTheLeft: boolean }) {
  return (
    <div className={`${reveal} ${toTheLeft ? revealEnd : ""}`} data-testid="toast-swipe-reveal" aria-hidden="true">
      <span>{pinned ? "📥" : "✕"}</span>
      <span>{pinned ? "Move to tray" : "Dismiss"}</span>
    </div>
  );
}
