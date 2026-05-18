"use client";
// +feature: backlog:item-card

import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import * as styles from "./BacklogItemCard.css";

interface BacklogItemCardProps {
  item: BacklogItem;
  onAction: (action: string, itemId: string) => void;
  onClick: (itemId: string) => void;
}

interface ActionSpec {
  label: string;
  action: string;
  disabled?: boolean;
  isDone?: boolean;
}

function getActionSpec(item: BacklogItem): ActionSpec {
  switch (item.status) {
    case "idea":
      return {
        label: "Mark Ready",
        action: "mark_ready",
        disabled: item.acCriteria.length === 0,
      };
    case "ready":
      return { label: "Trigger Triage", action: "trigger_triage" };
    case "in_progress":
      return {
        label: "View Session",
        action: "view_session",
        disabled: item.linkedSessions.length === 0,
      };
    case "review":
      return { label: "View Review", action: "view_review" };
    case "done":
      return { label: "Done ✓", action: "done", isDone: true };
    case "archived":
      return { label: "Archived", action: "archived", isDone: true };
  }
}

function AcSummary({ item }: { item: BacklogItem }) {
  if (item.acCriteria.length === 0) return null;
  const done = item.acCriteria.filter((c) => c.status === "done").length;
  return (
    <span className={styles.acSummary} aria-label={`${done} of ${item.acCriteria.length} criteria done`}>
      {done}/{item.acCriteria.length} done
    </span>
  );
}

const PRIORITY_LABELS: Record<number, string> = {
  1: "P1",
  2: "P2",
  3: "P3",
  4: "P4",
  5: "P5",
};

export function BacklogItemCard({ item, onAction, onClick }: BacklogItemCardProps) {
  const actionSpec = getActionSpec(item);

  const handleCardClick = (e: React.MouseEvent) => {
    // Don't open detail if the action button was clicked
    if ((e.target as HTMLElement).closest("[data-action-button]")) return;
    onClick(item.id);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick(item.id);
    }
  };

  return (
    <div
      className={styles.card}
      role="article"
      tabIndex={0}
      data-testid="backlog-item-card"
      data-item-id={item.id}
      aria-label={`Backlog item: ${item.title}`}
      onClick={handleCardClick}
      onKeyDown={handleKeyDown}
    >
      <div className={styles.cardHeader}>
        <span className={styles.title}>{item.title}</span>
        <span
          className={styles.priorityBadge}
          aria-label={`Priority: ${PRIORITY_LABELS[item.priority] ?? "P?"}`}
        >
          {PRIORITY_LABELS[item.priority] ?? "P?"}
        </span>
      </div>

      <div className={styles.cardFooter}>
        <AcSummary item={item} />
        <button
          className={`${styles.actionButton} ${actionSpec.isDone ? styles.actionButtonDone : ""}`}
          disabled={actionSpec.disabled}
          aria-label={actionSpec.label}
          data-action-button="true"
          data-testid={`backlog-action-${actionSpec.action}`}
          onClick={(e) => {
            e.stopPropagation();
            if (!actionSpec.disabled && !actionSpec.isDone) {
              onAction(actionSpec.action, item.id);
            }
          }}
        >
          {actionSpec.label}
        </button>
      </div>
    </div>
  );
}
