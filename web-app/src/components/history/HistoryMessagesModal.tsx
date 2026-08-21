"use client";

import { useRef, useEffect, useMemo } from "react";
import { ClaudeMessage } from "@/gen/session/v1/session_pb";
import { formatDate } from "@/lib/utils/timestamp";
import * as styles from "./HistoryMessagesModal.css";

interface HistoryMessagesModalProps {
  open: boolean;
  messages: ClaudeMessage[];
  messageSearchQuery: string;
  onSearchChange: (query: string) => void;
  onClose: () => void;
}

export function HistoryMessagesModal({
  open,
  messages,
  messageSearchQuery,
  onSearchChange,
  onClose,
}: HistoryMessagesModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);

  // Filter messages in modal
  const filteredMessages = useMemo(() => {
    if (!messageSearchQuery) return messages;
    const query = messageSearchQuery.toLowerCase();
    return messages.filter(msg =>
      msg.content.toLowerCase().includes(query)
    );
  }, [messages, messageSearchQuery]);

  // Focus trap for modal
  useEffect(() => {
    if (open && modalRef.current) {
      const focusableElements = modalRef.current.querySelectorAll(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      );
      const firstElement = focusableElements[0] as HTMLElement;
      const lastElement = focusableElements[focusableElements.length - 1] as HTMLElement;

      const handleTabKey = (e: KeyboardEvent) => {
        if (e.key !== "Tab") return;

        if (e.shiftKey) {
          if (document.activeElement === firstElement) {
            e.preventDefault();
            lastElement?.focus();
          }
        } else {
          if (document.activeElement === lastElement) {
            e.preventDefault();
            firstElement?.focus();
          }
        }
      };

      firstElement?.focus();
      window.addEventListener("keydown", handleTabKey);
      return () => window.removeEventListener("keydown", handleTabKey);
    }
  }, [open]);

  if (!open) return null;

  return (
    <div
      className={styles.modalOverlay}
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="messages-modal-title"
    >
      <div
        ref={modalRef}
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal Header */}
        <div className={styles.modalHeader}>
          <h2 id="messages-modal-title" className={styles.modalTitle}>
            Conversation Messages ({filteredMessages.length}
            {messageSearchQuery && ` of ${messages.length}`})
          </h2>
          <div className={styles.messageSearchContainer}>
            <input
              type="text"
              placeholder="Search in messages..."
              value={messageSearchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              className={styles.messageSearchInput}
            />
            {messageSearchQuery && (
              <button
                onClick={() => onSearchChange("")}
                className="btn btn-ghost btn-sm"
              >
                Clear
              </button>
            )}
          </div>
          <button
            onClick={onClose}
            className={styles.modalCloseButton}
            aria-label="Close messages dialog"
          >
            ✕
          </button>
        </div>

        {/* Messages List */}
        <div className={styles.modalContent}>
          {filteredMessages.length === 0 && messageSearchQuery ? (
            <div className={styles.emptyStateContainer}>
              <div className={styles.emptyStateIcon}>🔍</div>
              <h3 className={styles.emptyStateTitle}>No messages match &quot;{messageSearchQuery}&quot;</h3>
              <button
                onClick={() => onSearchChange("")}
                className={styles.linkButton}
              >
                Clear search
              </button>
            </div>
          ) : (
            filteredMessages.map((msg, idx) => (
              <div
                key={idx}
                className={msg.role === "user" ? styles.messageUser : styles.messageAssistant}
              >
                <div className={styles.messageHeader}>
                  <div
                    style={{
                      fontWeight: "600",
                      color: msg.role === "user" ? "var(--primary)" : "var(--text-secondary)",
                      textTransform: "capitalize",
                    }}
                  >
                    {msg.role}
                  </div>
                  <div className="text-muted" style={{ fontSize: "12px" }}>
                    {formatDate(msg.timestamp)}
                  </div>
                </div>
                <div className={styles.messageContent}>
                  {msg.content}
                </div>
                {msg.model && (
                  <div
                    className="text-muted"
                    style={{
                      fontSize: "11px",
                      marginTop: "8px",
                      fontFamily: "monospace",
                    }}
                  >
                    Model: {msg.model}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
