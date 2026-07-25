"use client";
// +feature: backlog:item-panel

import { useState, useEffect, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AppLink } from "@/components/ui/AppLink";
import { useBacklogService, mapBacklogItem, type BacklogItem } from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";
import { useAppSelector } from "@/lib/store";
import { selectBacklogItemById } from "@/lib/store/backlogItemsSlice";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import { InlineNotice } from "@/components/common/InlineNotice";
import { ConnectionIndicator } from "./ConnectionIndicator";
import * as styles from "./BacklogItemPanel.css";

interface BacklogItemPanelProps {
  backlogItemId: string;
  sessionId: string;
}

// Sweep fix (backlog-event-driven-updates Phase 5 compliance sweep,
// 2026-07-22): Story 5.4.1's own acceptance criterion ("a verdict recorded
// shortly after ... BacklogItemPanel reflects the new status and verdict")
// and ux.md UX AC #14 both require the verdict to surface here — the
// original implementation only ever rendered status/priority/AC criteria,
// never gateVerdict. Mirrors BacklogItemCard.tsx's VERDICT_BADGE_CONFIG;
// PENDING is intentionally left unbadged there too (it just means a review
// is running, not a signal worth a badge).
const VERDICT_BADGE_CONFIG: Partial<Record<NonNullable<BacklogItem["gateVerdict"]>, { label: string; className: string }>> = {
  PASS: { label: "✓ PASS", className: styles.verdictBadgePass },
  PARTIAL: { label: "◑ PARTIAL", className: styles.verdictBadgePartial },
  FAIL: { label: "✗ FAIL", className: styles.verdictBadgeFail },
  UNVERIFIABLE: { label: "? UNVERIFIABLE", className: styles.verdictBadgeUnverifiable },
};

export function BacklogItemPanel({
  backlogItemId,
  sessionId,
}: BacklogItemPanelProps) {
  const { getBacklogItem } = useBacklogService();
  const [open, setOpen] = useState(() => {
    if (typeof window === "undefined") return false;
    return localStorage.getItem(`backlog-panel-${sessionId}`) === "open";
  });
  const [item, setItem] = useState<BacklogItem | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);

  const loadItem = useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    try {
      const result = await getBacklogItem(backlogItemId);
      setItem(result);
    } catch (err) {
      console.error("Failed to load backlog item:", err);
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [getBacklogItem, backlogItemId]);

  // Initial load only — Epic 5.4 (Story 5.4.1) replaces the old
  // exponential-backoff poll below with the live subscription instead. A
  // direct fetch is still needed here for the first paint: the shared
  // store's own initial snapshot (useWatchBacklogItems's REST refresh)
  // excludes terminal/archived items, so a linked item that's already
  // "done" or archived when this panel mounts would otherwise never appear.
  useEffect(() => {
    void loadItem();
  }, [loadItem]);

  // Epic 5.4 (Story 5.4.1 / Task 5.4.1b): live updates replace the old poll
  // entirely. Subscribed unfiltered (no status/category filter) so this
  // panel keeps reflecting the linked item's current state the same way
  // BacklogItemDetail does (Task 5.3.1b) — the hook's own return value is
  // unused here; it exists only to keep the shared store hydrated/connected.
  // This panel reads the single item it cares about straight off the store
  // below via selectBacklogItemById, so unrelated item updates elsewhere
  // never cause this panel (or the surrounding SessionDetail it's embedded
  // in) to re-render.
  // Task 6.2.1c: connectionState is captured here (previously discarded) to
  // mount this panel's own ConnectionIndicator. Task 5.4.1a's discovery pass
  // (plan.md) confirmed SessionDetail/SessionDetailView/SessionDetailBar have
  // no existing session-level "Live" indicator to reuse instead, so this
  // panel needs its own (ux.md §4 UX AC #15/#20).
  const { connectionState } = useWatchBacklogItems();
  const liveRawItem = useAppSelector((state) => selectBacklogItemById(state, backlogItemId));

  useEffect(() => {
    if (!liveRawItem) return;
    setItem(mapBacklogItem(liveRawItem));
  }, [liveRawItem]);

  // Terminal-state (Task 5.4.1c): set when an ArchivedEvent/RemovedEvent
  // arrives for this item from the separate raw watch below. Mirrors
  // BacklogItemDetail's Task 5.3.1c mechanism exactly — see that file for
  // the full rationale (useWatchBacklogItems.ts intentionally never
  // dispatches itemArchived/removal into the normalized store, and there is
  // no server-side item-id filter on WatchBacklogItemsRequest, so this
  // watches unfiltered and matches events against `backlogItemId`
  // client-side).
  const [terminalState, setTerminalState] = useState<"archived" | "removed" | null>(null);

  useEffect(() => {
    setTerminalState(null);
    const abortController = new AbortController();

    const watchTerminal = async () => {
      try {
        const transport = createConnectTransport({
          baseUrl: getApiBaseUrl(),
          interceptors: [createAuthInterceptor()],
        });
        const client = createClient(BacklogService, transport);
        const stream = client.watchBacklogItems(
          { statusFilter: [], categoryFilter: [], afterSeq: 0n },
          { signal: abortController.signal }
        );
        for await (const event of stream) {
          if (event.event.case === "itemArchived" && event.event.value.itemId === backlogItemId) {
            setTerminalState("archived");
          } else if (event.event.case === "itemRemoved" && event.event.value.itemId === backlogItemId) {
            setTerminalState("removed");
          }
        }
      } catch (err) {
        if (err instanceof Error && err.name === "AbortError") return;
        if (abortController.signal.aborted) return;
        console.error("[BacklogItemPanel] terminal-state watch stream error:", err);
      }
    };

    void watchTerminal();
    return () => {
      abortController.abort();
    };
  }, [backlogItemId]);

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
                <ConnectionIndicator connectionState={connectionState} />
              </div>
              <AppLink
                href={`/backlog?item=${item.id}`}
                className={styles.title}
                data-testid="backlog-panel-title"
              >
                {item.title}
              </AppLink>

              {item.gateVerdict && VERDICT_BADGE_CONFIG[item.gateVerdict] && (
                <div className={styles.header} data-testid="backlog-panel-verdict">
                  <span className={VERDICT_BADGE_CONFIG[item.gateVerdict]!.className} title="Last review result">
                    {VERDICT_BADGE_CONFIG[item.gateVerdict]!.label}
                  </span>
                  {item.gateVerdictSummary && (
                    <span className={styles.verdictSummary}>{item.gateVerdictSummary}</span>
                  )}
                </div>
              )}

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
                {terminalState ? (
                  <InlineNotice
                    message={
                      terminalState === "archived"
                        ? "This item was archived elsewhere."
                        : "This item was removed elsewhere."
                    }
                    data-testid="backlog-panel-terminal-notice"
                  />
                ) : (
                  <AppLink
                    href={`/backlog?item=${item.id}`}
                    className={styles.actionLink}
                    data-testid="backlog-panel-view-full"
                  >
                    View full item →
                  </AppLink>
                )}
              </div>
            </>
          ) : loadError ? (
            <div className={styles.error}>Failed to load item</div>
          ) : (
            <div className={styles.error}>Item not found</div>
          )}
        </div>
      )}
    </div>
  );
}
