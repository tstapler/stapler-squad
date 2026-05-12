"use client";
// +feature: backlog:item-panel

import { useState, useEffect, useCallback } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { useBacklogService, type BacklogItem } from "@/lib/hooks/useBacklogService";
import * as styles from "./BacklogItemPanel.css";

interface BacklogItemPanelProps {
  backlogItemId: string;
  sessionId: string;
  isSessionActive: boolean;
}

export function BacklogItemPanel({
  backlogItemId,
  sessionId,
  isSessionActive,
}: BacklogItemPanelProps) {
  const service = useBacklogService();
  const [open, setOpen] = useState(() => {
    if (typeof window === "undefined") return false;
    return localStorage.getItem(`backlog-panel-${sessionId}`) === "open";
  });
  const [item, setItem] = useState<BacklogItem | null>(null);
  const [loading, setLoading] = useState(false);

  const loadItem = useCallback(async () => {
    setLoading(true);
    try {
      const result = await service.getBacklogItem(backlogItemId);
      setItem(result);
    } catch (err) {
      console.error("Failed to load backlog item:", err);
    } finally {
      setLoading(false);
    }
  }, [service, backlogItemId]);

  useEffect(() => {
    void loadItem();
  }, [loadItem]);

  // Poll while session is active and panel is open
  useEffect(() => {
    if (!open || !isSessionActive) return;
    let delay = 3000;
    const maxDelay = 30000;
    let timer: ReturnType<typeof setTimeout>;
    const poll = () => {
      void loadItem();
      delay = Math.min(delay * 1.5, maxDelay);
      timer = setTimeout(poll, delay);
    };
    timer = setTimeout(poll, delay);
    return () => clearTimeout(timer);
  }, [open, isSessionActive, loadItem]);

  const toggleOpen = () => {
    const next = !open;
    setOpen(next);
    localStorage.setItem(`backlog-panel-${sessionId}`, next ? "open" : "closed");
  };

  const statusIcon = (status: string) => {
    if (status === "done") return "✓";
    if (status === "in_progress") return "●";
    return "○";
  };

  return (
    <div className={styles.panel} data-testid="backlog-panel">
      <button
        className={styles.toggle}
        onClick={toggleOpen}
        data-testid="backlog-panel-toggle"
        aria-expanded={open}
        aria-label={open ? "Collapse backlog panel" : "Expand backlog panel"}
      >
        <span className={styles.toggleIcon}>{open ? "▼" : "▶"}</span>
        {!open && <span className={styles.toggleLabel}>Task</span>}
      </button>

      {open && (
        <div className={styles.content}>
          {loading && !item ? (
            <div className={styles.loading}>Loading...</div>
          ) : item ? (
            <>
              <div className={styles.header}>
                <span className={styles.priorityBadge}>P{item.priority}</span>
                <span className={styles.statusChip}>
                  {item.status.replace(/_/g, " ")}
                </span>
              </div>
              <AppLink
                href={`/backlog?item=${item.id}`}
                className={styles.title}
                data-testid="backlog-panel-title"
              >
                {item.title}
              </AppLink>

              {item.acCriteria.length > 0 && (
                <div className={styles.criteriaSection}>
                  <div className={styles.criteriaHeader}>
                    Acceptance Criteria
                  </div>
                  <ul className={styles.criteriaList}>
                    {item.acCriteria.map((c) => (
                      <li
                        key={c.index}
                        className={styles.criterionRow}
                        data-testid={`backlog-panel-criterion-${c.index}`}
                      >
                        <span
                          className={
                            c.status === "done"
                              ? styles.criterionDone
                              : styles.criterionPending
                          }
                        >
                          {statusIcon(c.status)}
                        </span>
                        <span className={styles.criterionText}>{c.text}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              <div className={styles.actions}>
                <AppLink
                  href={`/backlog?item=${item.id}`}
                  className={styles.actionLink}
                  data-testid="backlog-panel-view-full"
                >
                  View full item →
                </AppLink>
              </div>
            </>
          ) : (
            <div className={styles.error}>Failed to load item</div>
          )}
        </div>
      )}
    </div>
  );
}
