"use client";

import { useEffect, useRef } from "react";
import { useApprovals } from "@/lib/hooks/useApprovals";
import { ApprovalCard } from "./ApprovalCard";
import styles from "./ApprovalPanel.module.css";

interface ApprovalPanelProps {
  sessionId?: string; // if provided, filter to this session
  onResolved?: () => void; // fires when all approvals for this session are resolved
}

/**
 * Panel showing all pending tool-use approval requests.
 *
 * Displays a header with count badge, an empty state when no approvals are pending,
 * and a list of ApprovalCard components for each pending request.
 *
 * @example
 * ```tsx
 * // Show all pending approvals
 * <ApprovalPanel />
 *
 * // Show approvals for a specific session
 * <ApprovalPanel sessionId="session-123" />
 * ```
 */
export function ApprovalPanel({ sessionId, onResolved }: ApprovalPanelProps) {
  const { approvals, loading, error, approve, deny, refresh } = useApprovals({
    sessionId,
  });

  // Fire onResolved when approvals drain from >0 to 0 (last approval was resolved)
  const prevCountRef = useRef<number | null>(null);
  useEffect(() => {
    const prevCount = prevCountRef.current;
    prevCountRef.current = approvals.length;
    if (prevCount !== null && prevCount > 0 && approvals.length === 0) {
      onResolved?.();
    }
  }, [approvals, onResolved]);

  // Keyboard shortcuts: Enter = Approve, Shift+Enter = Deny
  // Only active when exactly one approval is pending and terminal is not focused.
  useEffect(() => {
    if (approvals.length !== 1) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Enter") return;
      if (e.ctrlKey || e.altKey || e.metaKey) return;
      // Guard: do not fire when an input field or the terminal (textarea) has focus
      const tag = (document.activeElement as HTMLElement)?.tagName?.toUpperCase();
      if (tag === "INPUT" || tag === "TEXTAREA" || (document.activeElement as HTMLElement)?.isContentEditable) return;
      e.preventDefault();
      if (e.shiftKey) {
        deny(approvals[0].id);
      } else {
        approve(approvals[0].id);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [approvals, approve, deny]);

  if (error) {
    return (
      <div className={styles.panel}>
        <div className={styles.error}>
          Failed to load approvals: {error.message}
          <br />
          <button onClick={refresh} className={styles.retryButton}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Don't render at all when there are no approvals and not loading
  if (!loading && approvals.length === 0) {
    return null;
  }

  return (
    <div className={styles.panel} data-testid="approval-panel">
      <div className={styles.header}>
        <h3 className={styles.title}>
          Pending Approvals
          {approvals.length > 0 && (
            <span className={styles.countBadge}>{approvals.length}</span>
          )}
        </h3>
        <button
          onClick={refresh}
          className={styles.refreshButton}
          disabled={loading}
          aria-label="Refresh approvals"
        >
          {loading ? "\u27F3" : "\u21BB"}
        </button>
      </div>

      <div className={styles.list}>
        {loading && approvals.length === 0 ? (
          <div className={styles.empty}>Loading approvals...</div>
        ) : (
          approvals.map((approval) => (
            <ApprovalCard
              key={approval.id}
              approval={approval}
              onApprove={() => approve(approval.id)}
              onDeny={() => deny(approval.id)}
            />
          ))
        )}
      </div>
    </div>
  );
}
