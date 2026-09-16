"use client";
// +feature: ui:system-banner

import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
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

export interface SystemBannerPager {
  /** 0-based position of the currently shown item. */
  index: number;
  total: number;
  onPrev: () => void;
  onNext: () => void;
}

export interface SystemBannerProps {
  severity: SystemBannerSeverity;
  /** Stable identifier for this banner's kind, tracked as `labels.banner`. */
  id: string;
  icon?: string;
  message: ReactNode;
  actions?: SystemBannerAction[];
  onDismiss?: () => void;
  testId?: string;
  /** Renders "‹ N / total ›" paging controls when set by SystemBannerStack. */
  pager?: SystemBannerPager;
}

interface SystemBannerRowActionsProps {
  id: string;
  severity: SystemBannerSeverity;
  overflowing: boolean;
  expanded: boolean;
  onToggleExpanded: () => void;
  actions?: SystemBannerAction[];
  onDismiss?: () => void;
  pager?: SystemBannerPager;
}

function SystemBannerPagerControls({
  id,
  severity,
  pager,
}: {
  id: string;
  severity: SystemBannerSeverity;
  pager: SystemBannerPager;
}) {
  const { track } = useAnalytics();
  if (pager.total <= 1) return null;

  const page = (direction: "prev" | "next", handler: () => void) => {
    track({ name: "system_banner_paged", category: "user_action", labels: { banner: id, direction } });
    handler();
  };

  return (
    <div className={styles.pager}>
      <button
        type="button"
        className={styles.pagerButton[severity]}
        aria-label="Previous notification"
        onClick={() => page("prev", pager.onPrev)}
      >
        ‹
      </button>
      <span className={styles.pagerLabel[severity]}>
        {pager.index + 1} / {pager.total}
      </span>
      <button
        type="button"
        className={styles.pagerButton[severity]}
        aria-label="Next notification"
        onClick={() => page("next", pager.onNext)}
      >
        ›
      </button>
    </div>
  );
}

function SystemBannerRowActions({
  id,
  severity,
  overflowing,
  expanded,
  onToggleExpanded,
  actions,
  onDismiss,
  pager,
}: SystemBannerRowActionsProps) {
  const { track } = useAnalytics();

  return (
    <div className={styles.rowActions}>
      {pager && <SystemBannerPagerControls id={id} severity={severity} pager={pager} />}
      {overflowing && (
        <button type="button" className={styles.toggleButton[severity]} onClick={onToggleExpanded}>
          {expanded ? "Show less" : "Show more"}
        </button>
      )}
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
  );
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
export function SystemBanner({ severity, id, icon, message, actions, onDismiss, testId, pager }: SystemBannerProps) {
  const [expanded, setExpanded] = useState(false);
  const [overflowing, setOverflowing] = useState(false);
  const textRef = useRef<HTMLSpanElement>(null);

  // Only re-measure while clamped -- expanding removes the clamp, so
  // clientHeight would equal scrollHeight and wrongly report "not overflowing".
  useLayoutEffect(() => {
    if (expanded) return;
    const el = textRef.current;
    if (!el) return;
    setOverflowing(el.scrollHeight > el.clientHeight + 1);
  }, [message, expanded]);

  return (
    <div className={styles.banner[severity]} role="alert" aria-live="polite" data-testid={testId}>
      <div className={styles.row}>
        <span
          ref={textRef}
          className={expanded ? styles.text[severity] : `${styles.text[severity]} ${styles.textClamp}`}
        >
          {icon && <span className={styles.icon}>{icon}</span>}
          {message}
        </span>
        <SystemBannerRowActions
          id={id}
          severity={severity}
          overflowing={overflowing}
          expanded={expanded}
          onToggleExpanded={() => setExpanded((prev) => !prev)}
          actions={actions}
          onDismiss={onDismiss}
          pager={pager}
        />
      </div>
    </div>
  );
}

/**
 * Shows one banner at a time from a list, paged with "‹ N / total ›" controls
 * instead of stacking every simultaneously-elevated system signal into a wall
 * of banners that eats the top of the page. A single item renders with no
 * pager at all (see SystemBannerPagerControls). Paging state resets to a
 * valid index automatically as items are dismissed/added, since `index` is
 * clamped to the current list length on every render rather than stored
 * relative to a specific item.
 */
export function SystemBannerStack({ items }: { items: SystemBannerProps[] }) {
  const [index, setIndex] = useState(0);
  if (items.length === 0) return null;

  const clamped = Math.min(index, items.length - 1);
  const current = items[clamped];

  return (
    <SystemBanner
      {...current}
      pager={{
        index: clamped,
        total: items.length,
        onPrev: () => setIndex((clamped - 1 + items.length) % items.length),
        onNext: () => setIndex((clamped + 1) % items.length),
      }}
    />
  );
}
