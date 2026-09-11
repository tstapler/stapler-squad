"use client";
// +feature: ui:system-banner

import type { ReactNode } from "react";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "./SystemBanner.css";

export type SystemBannerSeverity = "info" | "warning" | "error";

export interface SystemBannerAction {
  /** Button label, and the value tracked as `labels.action` on click. */
  label: string;
  onClick: () => void;
  disabled?: boolean;
  /** "danger" renders the action in the error palette for destructive actions. */
  variant?: "default" | "danger";
  testId?: string;
}

interface SystemBannerProps {
  severity: SystemBannerSeverity;
  /** Stable identifier for this banner's kind, tracked as `labels.banner`. */
  id: string;
  icon?: string;
  message: ReactNode;
  actions?: SystemBannerAction[];
  onDismiss?: () => void;
  testId?: string;
}

/**
 * Generic, reusable persistent banner for important system-health signals
 * (a degraded subsystem, a resource-pressure warning, ...) that need more
 * than a transient toast: a severity-colored bar with a message, optional
 * action buttons, and an optional dismiss control. Every action and dismiss
 * click is tracked automatically (`system_banner_action` / `system_banner_dismissed`)
 * so a new banner gets telemetry for free instead of each consumer
 * remembering to wire it up.
 *
 * First consumer: TmuxVersionMismatchBanner. Session-scoped banners like
 * MemoryPressureCallout are a different shape (per-item list, bulk action)
 * and aren't forced through this -- reach for this component for a
 * single-message, whole-app system-health signal.
 */
export function SystemBanner({ severity, id, icon, message, actions, onDismiss, testId }: SystemBannerProps) {
  const { track } = useAnalytics();

  return (
    <div className={styles.banner[severity]} role="alert" aria-live="polite" data-testid={testId}>
      <div className={styles.row}>
        <span className={styles.text[severity]}>
          {icon && <span className={styles.icon}>{icon}</span>}
          {message}
        </span>
        <div className={styles.rowActions}>
          {actions?.map((action) => (
            <button
              key={action.label}
              type="button"
              className={action.variant === "danger" ? styles.dangerButton : styles.actionButton[severity]}
              disabled={action.disabled}
              data-testid={action.testId}
              onClick={() => {
                track({ name: "system_banner_action", category: "user_action", labels: { banner: id, action: action.label } });
                action.onClick();
              }}
            >
              {action.label}
            </button>
          ))}
          {onDismiss && (
            <button
              type="button"
              className={styles.dismissButton[severity]}
              aria-label="Dismiss"
              title="Dismiss"
              onClick={() => {
                track({ name: "system_banner_dismissed", category: "user_action", labels: { banner: id } });
                onDismiss();
              }}
            >
              ✕
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
