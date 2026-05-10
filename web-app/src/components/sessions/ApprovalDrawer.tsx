"use client";

import { useEffect, useRef, useState } from "react";
import { useApprovals } from "@/lib/hooks/useApprovals";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { ApprovalCard } from "./ApprovalCard";
import * as styles from "./ApprovalDrawer.css";

interface ApprovalDrawerProps {
  isOpen: boolean;
  onClose: () => void;
}

/**
 * Non-modal right-side drawer listing all pending approvals across all sessions.
 * Sorted by time-to-expire (most urgent first) per MP3 requirement.
 * Does not block the rest of the UI — no backdrop overlay.
 */
export function ApprovalDrawer({ isOpen, onClose }: ApprovalDrawerProps) {
  const { approvals, approve, deny, refresh, error, loading } = useApprovals({});
  const sessions = useAppSelector(selectAllSessions);
  const [announcement, setAnnouncement] = useState("");
  const prevCountRef = useRef(approvals.length);
  const closeButtonRef = useRef<HTMLButtonElement>(null);

  // Build session ID → title lookup map
  const sessionTitleById = Object.fromEntries(
    sessions.map((s) => [s.id, s.title])
  );

  // Detect expiry to produce aria-live announcements
  useEffect(() => {
    const prev = prevCountRef.current;
    prevCountRef.current = approvals.length;
    if (prev > approvals.length) {
      const expired = prev - approvals.length;
      setAnnouncement(
        `${expired} approval${expired !== 1 ? "s" : ""} expired or resolved.`
      );
    }
  }, [approvals.length]);

  // Focus close button on open
  useEffect(() => {
    if (isOpen) {
      closeButtonRef.current?.focus();
    }
  }, [isOpen]);

  // Close on Escape
  useEffect(() => {
    if (!isOpen) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  // Sort by secondsRemaining ascending (most urgent first)
  const sorted = [...approvals].sort(
    (a, b) => a.secondsRemaining - b.secondsRemaining
  );

  return (
    <aside
      className={styles.drawer}
      role="complementary"
      aria-label="Pending approvals"
    >
      {/* Visually-hidden live region for expiry announcements */}
      <div
        className={styles.announcer}
        aria-live="polite"
        aria-atomic="true"
      >
        {announcement}
      </div>

      <div className={styles.header}>
        <h2 className={styles.title}>
          Pending Approvals{approvals.length > 0 && ` (${approvals.length})`}
        </h2>
        <button
          ref={closeButtonRef}
          className={styles.closeButton}
          onClick={onClose}
          aria-label="Close approvals drawer"
          title="Close"
        >
          ✕
        </button>
      </div>

      <div className={styles.list}>
        {error ? (
          <p className={styles.empty}>Failed to load approvals: {error.message}</p>
        ) : loading && sorted.length === 0 ? (
          <p className={styles.empty}>Loading…</p>
        ) : sorted.length === 0 ? (
          <p className={styles.empty}>No pending approvals</p>
        ) : (
          sorted.map((approval) => (
            <ApprovalCard
              key={approval.id}
              approval={approval}
              onApprove={() => { approve(approval.id); refresh(); }}
              onDeny={() => { deny(approval.id); refresh(); }}
              sessionTitle={sessionTitleById[approval.sessionId]}
            />
          ))
        )}
      </div>
    </aside>
  );
}
