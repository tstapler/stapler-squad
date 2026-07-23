"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import {
  getStuckReasonClass,
  getStuckReasonIcon,
  getStuckReasonLabel,
  isPrStatusUnknown,
  formatStuckDuration,
  formatAgo,
  PR_STATUS_UNKNOWN_CLASS,
  PR_STATUS_UNKNOWN_ICON,
  PR_STATUS_UNKNOWN_LABEL,
} from "./stuckReason";
import { StuckItemDetail } from "./StuckItemDetail";
import * as styles from "./StuckItem.css";

interface StuckItemProps {
  item: StuckBacklogItem;
  isExpanded: boolean;
  onToggleExpand: () => void;
  /** Count of *other* currently-visible reason groups this same item_id also appears in (design/ux.md Surface 2 cross-reference badge). 0 or omitted suppresses the badge. */
  otherReasonsCount?: number;
  otherReasonLabels?: string[];
  /** True once the underlying condition has resolved while this card was expanded (design/ux.md Surface 12). */
  justResolved?: boolean;
  resolvedMessage?: string;
  /** Imperative snooze action from useStuckBacklogItems — omitted disables the snooze control entirely. */
  onSnooze?: (itemId: string, reason: StuckReason, until: Date) => Promise<boolean>;
  /** Sets a per-item rework-cap override and immediately reopens the item — omitted disables the rework_cap override control. */
  onReworkCapOverride?: (itemId: string, override: number) => Promise<boolean>;
  /**
   * Operator "Retry now" escape hatch (TriggerRemediationNow RPC) — omitted
   * disables the retry control entirely. Rejects (throws) when the row is
   * already parked or has no wired remediation action; the caller (this
   * component) surfaces that as inline error text rather than the parent
   * needing to know the specific failure shape.
   */
  onTriggerRemediationNow?: (itemId: string, reason: StuckReason) => Promise<void>;
}

/** Extracts "owner/repo" from a GitHub PR URL, for the glance-level identity line. */
function repoFromPrUrl(prUrl: string): string | null {
  const match = prUrl.match(/github\.com\/([^/]+\/[^/]+)\/pull\//);
  return match ? match[1] : null;
}

type SnoozeDuration = "1h" | "1d" | "3d";

const SNOOZE_DURATION_MS: Record<SnoozeDuration, number> = {
  "1h": 60 * 60 * 1000,
  "1d": 24 * 60 * 60 * 1000,
  "3d": 3 * 24 * 60 * 60 * 1000,
};

const SNOOZE_DURATION_LABELS: Record<SnoozeDuration, string> = {
  "1h": "1 hour",
  "1d": "1 day",
  "3d": "3 days",
};

/**
 * Mirrors session.MaxRemediationAttempts (session/backlog_remediation.go) —
 * the backoff schedule has 5 entries (30m/2h/8h/24h/72h), so a row with
 * remediation_attempts >= 5 is "parked". Not sourced from the proto response
 * itself since the cap is a backend policy constant, not per-item data.
 */
const MAX_REMEDIATION_ATTEMPTS = 5;

/**
 * Detects a `(hover: none), (pointer: coarse)` pointer — the media query the
 * rest of the app already uses to gate hover-only chrome (design/ux.md
 * Surface 7). Returns false during SSR / before the effect runs, matching a
 * hover-capable desktop default.
 */
function useHoverUnavailable(): boolean {
  const [hoverUnavailable, setHoverUnavailable] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    const mql = window.matchMedia("(hover: none), (pointer: coarse)");
    setHoverUnavailable(mql.matches);

    const listener = (e: MediaQueryListEvent) => setHoverUnavailable(e.matches);
    if (mql.addEventListener) {
      mql.addEventListener("change", listener);
      return () => mql.removeEventListener("change", listener);
    }
    // Safari <14 fallback.
    mql.addListener(listener);
    return () => mql.removeListener(listener);
  }, []);

  return hoverUnavailable;
}

/**
 * Card representing a single stuck backlog item (one BacklogStuckState row).
 * Verbatim copy of UnfinishedItem.tsx's expand/keyboard/aria-expanded shape.
 */
export function StuckItem({
  item,
  isExpanded,
  onToggleExpand,
  otherReasonsCount = 0,
  otherReasonLabels = [],
  justResolved = false,
  resolvedMessage,
  onSnooze,
  onReworkCapOverride,
  onTriggerRemediationNow,
}: StuckItemProps) {
  const cardRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const wasExpandedRef = useRef(isExpanded);
  const hoverUnavailable = useHoverUnavailable();

  const [snoozeOpen, setSnoozeOpen] = useState(false);
  const [snoozeDuration, setSnoozeDuration] = useState<SnoozeDuration>("1d");
  const [snoozeState, setSnoozeState] = useState<"idle" | "pending" | "error">("idle");

  const [retryState, setRetryState] = useState<"idle" | "pending" | "error">("idle");
  const [retryErrorMessage, setRetryErrorMessage] = useState<string | null>(null);

  // AC 29: when this card collapses (Escape, re-click, or a parent-driven
  // toggle), keyboard focus returns to the card's own toggle control — it
  // must never be dropped to <body>.
  useEffect(() => {
    if (wasExpandedRef.current && !isExpanded) {
      cardRef.current?.focus();
    }
    wasExpandedRef.current = isExpanded;
  }, [isExpanded]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onToggleExpand();
      }
      if (e.key === "Escape" && isExpanded) {
        onToggleExpand();
      }
    },
    [isExpanded, onToggleExpand]
  );

  // Surface 10: clicking outside the open picker closes it with no request sent.
  useEffect(() => {
    if (!snoozeOpen) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setSnoozeOpen(false);
        setSnoozeState("idle");
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [snoozeOpen]);

  const handleSnoozeTriggerClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setSnoozeOpen((open) => !open);
    setSnoozeState("idle");
  }, []);

  const handleSnoozeCancel = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setSnoozeOpen(false);
    setSnoozeState("idle");
  }, []);

  const handleSnoozeConfirm = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();
      if (!onSnooze) return;
      setSnoozeState("pending");
      const until = new Date(Date.now() + SNOOZE_DURATION_MS[snoozeDuration]);
      const applied = await onSnooze(item.itemId, item.reason, until);
      if (applied) {
        // Success: the hook refetches and this card is removed from the
        // parent's list on the next render — nothing further to do here.
        setSnoozeOpen(false);
        setSnoozeState("idle");
      } else {
        setSnoozeState("error");
      }
    },
    [onSnooze, item.itemId, item.reason, snoozeDuration]
  );

  const handleSnoozePickerKeyDown = useCallback((e: React.KeyboardEvent) => {
    e.stopPropagation();
    if (e.key === "Escape") {
      setSnoozeOpen(false);
      setSnoozeState("idle");
    }
  }, []);

  const isParked = item.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS;

  const handleRetryNow = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();
      if (!onTriggerRemediationNow) return;
      setRetryState("pending");
      setRetryErrorMessage(null);
      try {
        await onTriggerRemediationNow(item.itemId, item.reason);
        // Success: the hook refetches; this item's remediation_attempts will
        // reflect the new attempt on the next render. No local "success"
        // state needed beyond clearing pending.
        setRetryState("idle");
      } catch (err) {
        setRetryState("error");
        setRetryErrorMessage(err instanceof Error ? err.message : "Retry failed");
      }
    },
    [onTriggerRemediationNow, item.itemId, item.reason]
  );

  const unknown = isPrStatusUnknown(item);
  const chipLabel = unknown ? PR_STATUS_UNKNOWN_LABEL : getStuckReasonLabel(item.reason);
  const chipIcon = unknown ? PR_STATUS_UNKNOWN_ICON : getStuckReasonIcon(item.reason);
  const chipClass = unknown ? PR_STATUS_UNKNOWN_CLASS : getStuckReasonClass(item.reason);

  const repo = item.prNumber > 0 ? repoFromPrUrl(item.prUrl) : null;
  const identity =
    item.prNumber > 0
      ? `PR #${item.prNumber}${repo ? ` · ${repo}` : ""}`
      : `item ${item.itemId.slice(0, 8)}`;

  return (
    <div ref={containerRef}>
      <div
        ref={cardRef}
        role="button"
        tabIndex={0}
        aria-expanded={isExpanded}
        className={`${styles.card} ${isExpanded ? styles.cardExpanded : ""} ${
          justResolved ? styles.cardResolved : ""
        }`}
        onClick={onToggleExpand}
        onKeyDown={handleKeyDown}
        data-testid="stuck-item"
        data-reason={item.reason}
      >
        <div className={styles.header}>
          <span
            className={`${chipClass}`}
            aria-label={chipLabel}
            data-testid="stuck-item-chip"
          >
            <span aria-hidden="true">{chipIcon}</span>
            {chipLabel}
          </span>
          <span className={styles.title} title={item.title}>
            {item.title}
          </span>
          <span className={styles.duration} data-testid="stuck-item-duration">
            stuck {formatStuckDuration(item.firstDetectedAt)}
          </span>
          {onTriggerRemediationNow && (
            <button
              type="button"
              className={`${styles.retryBtn} ${hoverUnavailable ? styles.retryBtnAlwaysOn : ""}`}
              onClick={handleRetryNow}
              disabled={isParked || retryState === "pending"}
              aria-label={
                isParked
                  ? "Retry now (disabled — remediation attempts exhausted; use Reset to try again)"
                  : "Retry remediation now"
              }
              title={
                isParked
                  ? "Automated retries have been exhausted for this item — use Reset to try again"
                  : "Immediately retry the automated fix, skipping the wait"
              }
              data-testid="stuck-item-retry-now"
            >
              {retryState === "pending" ? "Retrying…" : "Retry now"}
            </button>
          )}
          {onSnooze && (
            <button
              type="button"
              className={`${styles.snoozeBtn} ${hoverUnavailable ? styles.snoozeBtnAlwaysOn : ""}`}
              onClick={handleSnoozeTriggerClick}
              aria-haspopup="true"
              aria-expanded={snoozeOpen}
              aria-label={hoverUnavailable ? "Snooze options" : "Snooze this item"}
              title="Snooze"
              data-testid="stuck-item-snooze-trigger"
            >
              {hoverUnavailable ? "⋮" : "Snooze"}
            </button>
          )}
        </div>
        <div className={styles.metaRow}>
          <span>{identity}</span>
          {unknown && (
            <span data-testid="stuck-item-last-checked">
              · last checked {formatAgo(item.lastCheckedAt)} (check failing)
            </span>
          )}
          {otherReasonsCount > 0 && (
            <span
              className={styles.otherReasonsBadge}
              title={otherReasonLabels.length > 0 ? otherReasonLabels.join(", ") : undefined}
              data-testid="stuck-item-other-reasons-badge"
            >
              · also stuck for {otherReasonsCount} other reason{otherReasonsCount !== 1 ? "s" : ""} ⓘ
            </span>
          )}
        </div>
        {retryState === "error" && retryErrorMessage && (
          <div className={styles.retryError} data-testid="stuck-item-retry-error">
            Retry failed: {retryErrorMessage}
          </div>
        )}
      </div>

      {snoozeOpen && onSnooze && (
        <div
          className={styles.snoozePicker}
          role="group"
          aria-label="Snooze duration"
          data-testid="stuck-item-snooze-picker"
          onClick={(e) => e.stopPropagation()}
          onKeyDown={handleSnoozePickerKeyDown}
        >
          <div className={styles.snoozeOptions}>
            {(Object.keys(SNOOZE_DURATION_LABELS) as SnoozeDuration[]).map((d) => (
              <label key={d} className={styles.snoozeOptionLabel}>
                <input
                  type="radio"
                  name={`snooze-duration-${item.itemId}-${item.reason}`}
                  value={d}
                  checked={snoozeDuration === d}
                  onChange={() => setSnoozeDuration(d)}
                  data-testid={`stuck-item-snooze-option-${d}`}
                />
                {SNOOZE_DURATION_LABELS[d]}
              </label>
            ))}
          </div>

          {snoozeState === "error" && (
            <div className={styles.snoozeErrorRow} data-testid="stuck-item-snooze-error">
              Couldn&apos;t snooze — try again
            </div>
          )}

          <div className={styles.snoozeActions}>
            <button
              type="button"
              className={styles.snoozeCancelBtn}
              onClick={handleSnoozeCancel}
              data-testid="stuck-item-snooze-cancel"
            >
              Cancel
            </button>
            <button
              type="button"
              className={styles.snoozeConfirmBtn}
              onClick={handleSnoozeConfirm}
              disabled={snoozeState === "pending"}
              data-testid="stuck-item-snooze-confirm"
            >
              {snoozeState === "pending" ? "Snoozing…" : snoozeState === "error" ? "Retry" : "Confirm"}
            </button>
          </div>
        </div>
      )}

      {justResolved && (
        <div className={styles.resolvedBanner} data-testid="stuck-item-resolved-banner">
          ✓ {resolvedMessage || "This item was just resolved."} It will be removed from this
          list shortly.
        </div>
      )}

      {isExpanded && !justResolved && (
        <StuckItemDetail item={item} onReworkCapOverride={onReworkCapOverride} />
      )}
    </div>
  );
}
