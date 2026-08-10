"use client";
// +feature: backlog:item-card

import { memo, useCallback, useEffect, useRef, useState } from "react";
// lucide-react (this repo's pinned v1.14) ships no brand "Github" glyph —
// CircleDot is the closest available icon to GitHub's own issue-tracker
// mark and is used here instead (plan.md's snippet assumed `Github` exists;
// it doesn't in the installed version).
import { CircleDot } from "lucide-react";
import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import type { StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { getStatusLabel } from "@/lib/backlog/status";
import { BlockerChip } from "./BlockerChip";
import { TriageLoadingIndicator } from "./TriageLoadingIndicator";
import * as styles from "./BacklogItemCard.css";

interface BacklogItemCardProps {
  item: BacklogItem;
  onAction: (action: string, itemId: string) => void;
  onClick: (itemId: string) => void;
  /** The action key currently in flight for this card, or null when idle. */
  pendingAction?: string | null;
  /**
   * Epic 6.4 (backlog-event-driven-updates): force the "just changed" flash
   * even though this exact card instance just (re)mounted. Used by
   * BacklogBoard.tsx when a live status change moves an item into a new
   * column — the card is a *fresh* mount there (different column/parent),
   * so this component's own `item.liveVersion` before/after comparison
   * below can never detect the change on its own (there is no "before").
   * BacklogBoard tracks the column transition itself and flips this on for
   * a bounded window so the destination-column entry still reads as part
   * of the same "just changed" event (ux.md §7 AC #8).
   */
  forceJustChanged?: boolean;
  /**
   * This item's entry from useStuckBacklogItems()'s open list, or undefined
   * when the item isn't currently flagged stuck. Resolved once at the board
   * page level (not per-card) and passed down — see board/page.tsx.
   */
  stuckItem?: StuckBacklogItem;
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
    case "refining":
      return { label: "Refining…", action: "refining", isDone: true };
    case "ready":
      return { label: "Trigger Triage", action: "trigger_triage", disabled: !item.repoPath };
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
    default:
      return { label: item.status, action: item.status, isDone: true };
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

const VERDICT_BADGE_CONFIG: Partial<Record<NonNullable<BacklogItem["gateVerdict"]>, { label: string; className: string }>> = {
  PASS: { label: "✓ PASS", className: styles.verdictBadgePass },
  PARTIAL: { label: "◑ PARTIAL", className: styles.verdictBadgePartial },
  FAIL: { label: "✗ FAIL", className: styles.verdictBadgeFail },
  UNVERIFIABLE: { label: "? UNVERIFIABLE", className: styles.verdictBadgeUnverifiable },
};

// Last review result, at a glance — most useful on in_progress cards, where a
// FAIL/PARTIAL verdict that triggered rework is otherwise invisible until the
// item is opened (PENDING is left unbadged: it just means a review is running,
// not a signal worth a card badge).
function VerdictBadge({ item }: { item: BacklogItem }) {
  if (!item.gateVerdict) return null;
  const config = VERDICT_BADGE_CONFIG[item.gateVerdict];
  if (!config) return null;
  return (
    <span className={config.className} title="Last review result">
      {config.label}
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

// Epic 6.1 (backlog-event-driven-updates): how long the `.justChanged` flash
// class stays applied. ux.md §1 calls for "~250ms, fading" (Linear/Jira-
// style); Story 6.1.1's Given/When/Then uses the same figure.
const FLASH_DURATION_MS = 250;

export const BacklogItemCard = memo(function BacklogItemCard({
  item,
  onAction,
  onClick,
  pendingAction = null,
  forceJustChanged = false,
  stuckItem,
}: BacklogItemCardProps) {
  const actionSpec = getActionSpec(item);
  const isTriageRunning = item.triageStatus === "running";
  const isActionPending = pendingAction === actionSpec.action;

  // `item.liveVersion` only advances for a genuine live (non-snapshot)
  // BacklogItemEvent (see useWatchBacklogItems.ts / backlogItemsSlice.ts) —
  // never for the initial snapshot, a reconnect resync, or a forced-
  // is_snapshot replay copy (pre-mortem #4). Comparing against the
  // previously-seen value (not just "did the item prop change") is what
  // keeps a resnapshot from flashing even though the item's fields did
  // change while disconnected — and comparing to a ref instead of keying off
  // `item` reference identity means this never fires on first mount.
  const prevLiveVersionRef = useRef(item.liveVersion);
  const [justChanged, setJustChanged] = useState(false);

  useEffect(() => {
    const prev = prevLiveVersionRef.current;
    const next = item.liveVersion;
    prevLiveVersionRef.current = next;
    if (prev === undefined || next === undefined || next === prev) return;

    setJustChanged(true);
    const timer = setTimeout(() => setJustChanged(false), FLASH_DURATION_MS);
    return () => clearTimeout(timer);
  }, [item.liveVersion]);

  const handleCardClick = (e: React.MouseEvent) => {
    // Don't open detail if the action button was clicked
    if ((e.target as HTMLElement).closest("[data-action-button]")) return;
    onClick(item.id);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Ignore keydowns that bubbled up from a nested focusable child (the
    // provenance badge link, the action button) — otherwise Enter on those
    // elements both triggers their own behavior AND this card's onClick,
    // and for the badge `<a>` specifically, this handler's preventDefault()
    // stops the anchor from navigating at all.
    if (e.target !== e.currentTarget) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick(item.id);
    }
  };

  const handleCancelTriage = useCallback(
    () => onAction("cancel_triage", item.id),
    [onAction, item.id],
  );

  const showFlash = justChanged || forceJustChanged;

  return (
    <div
      className={showFlash ? `${styles.card} ${styles.justChanged}` : styles.card}
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
        <span className={styles.statusLabel} data-testid="backlog-item-card-status">
          {getStatusLabel(item.status)}
        </span>
      </div>

      {isTriageRunning && (
        <TriageLoadingIndicator
          elapsedSeconds={0}
          context="list"
          onCancel={pendingAction !== null ? () => {} : handleCancelTriage}
          compact
        />
      )}

      <div className={styles.cardFooter}>
        <span className={styles.footerLeft}>
          <AcSummary item={item} />
          <VerdictBadge item={item} />
        </span>
        {item.externalUrl && item.externalId && (
          <a
            href={item.externalUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={styles.provenanceBadge}
            aria-label={`Imported from GitHub issue #${item.externalId}`}
            data-action-button="true"
            onClick={(e) => e.stopPropagation()}
          >
            <CircleDot aria-hidden="true" size={12} />
            #{item.externalId}
          </a>
        )}
        <button
          className={`${styles.actionButton} ${actionSpec.isDone ? styles.actionButtonDone : ""}`}
          disabled={actionSpec.disabled || isTriageRunning || pendingAction !== null}
          aria-label={isActionPending ? "Running…" : isTriageRunning ? "Triage in progress" : actionSpec.label}
          data-action-button="true"
          data-testid={`backlog-action-${actionSpec.action}`}
          onClick={(e) => {
            e.stopPropagation();
            if (!actionSpec.disabled && !actionSpec.isDone && !isTriageRunning && pendingAction === null) {
              onAction(actionSpec.action, item.id);
            }
          }}
        >
          {isActionPending ? (
            <>
              <span className={styles.buttonSpinner} aria-hidden="true" />
              Running…
            </>
          ) : (
            actionSpec.label
          )}
        </button>
        {stuckItem && <BlockerChip variant="compact" item={stuckItem} />}
      </div>
    </div>
  );
});
