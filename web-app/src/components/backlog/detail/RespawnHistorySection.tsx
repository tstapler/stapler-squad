// +feature: backlog-respawn-history
"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { useShowMore } from "@/lib/hooks/useShowMore";
import { formatDate } from "@/lib/backlog/formatDate";
import { formatRespawnReason } from "@/lib/backlog/formatRespawnReason";
import * as styles from "../BacklogItemDetail.css";
import * as sectionStyles from "./RespawnHistorySection.css";

export interface RespawnHistorySectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

const SHOW_MORE_CAP = 8;

/**
 * All-time, append-only audit trail of automated respawn/remediation
 * attempts (AutoRespawnReview/AutoRespawnAutonomousWork/AutoRespawnTriage/
 * RemediateStaleWorkSession) — collapsed by default, one click from the item
 * detail panel (itself reachable in one click from the board), so 2 clicks
 * total from the board per this item's acceptance criteria.
 *
 * Always renders, even with zero events — an explicit empty state, matching
 * WorkflowHistorySection's precedent, not omit-when-empty: hiding the
 * section entirely would make the respawn-tracking feature look like it
 * isn't there, for the common case (an item that has never needed
 * remediation) that this feature otherwise has nothing else to show for.
 *
 * The reconciliation caption below explains a discrepancy users will
 * otherwise read as a bug: this section's count is all-time and never
 * resets, while BlockerChip's ×N (session/backlog_remediation.go's
 * remediation_attempts) is scoped to the current stuck episode and resets
 * once that episode resolves. An item that has cycled stuck→resolved→stuck
 * more than once will show a larger number here than on the board card —
 * expected, not a bug.
 */
export function RespawnHistorySection({ item, defaultExpanded }: RespawnHistorySectionProps) {
  const respawnEvents = item.respawnEvents ?? [];
  const { visible, hasMore, remaining, showAll } = useShowMore(
    item.id,
    "respawn-history",
    respawnEvents,
    SHOW_MORE_CAP
  );

  return (
    <CollapsibleSection sectionKey="respawn-history" title="Respawn History" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        {respawnEvents.length === 0 ? (
          <p className={styles.emptyText} data-testid="respawn-history-empty">
            No automated respawns recorded for this item.
          </p>
        ) : (
          <>
            <p className={sectionStyles.reconciliationCaption}>
              All-time respawn history — may exceed the current retry count shown above if this item has been
              resolved and re-stuck before.
            </p>
            <div className={styles.workflowTimeline} role="list" aria-label="Respawn history">
              {visible.map((ev) => (
                <div key={ev.id} className={styles.workflowEvent} role="listitem">
                  <div className={styles.workflowEventRow}>
                    <span>{formatRespawnReason(ev.reason)}</span>
                    <span className={styles.workflowEventMeta}>{ev.createdAt ? formatDate(ev.createdAt) : ""}</span>
                  </div>
                  <span className={styles.workflowEventMeta}>
                    {ev.triggeringSessionUuid && <>Triggered by session {ev.triggeringSessionUuid}. </>}
                    {ev.resultingSessionUuid ? (
                      <>Spawned session {ev.resultingSessionUuid}.</>
                    ) : ev.queued ? (
                      "Queued — waiting for a concurrency slot."
                    ) : (
                      "Spawn attempt failed."
                    )}
                  </span>
                </div>
              ))}
            </div>
            {hasMore && (
              <button
                type="button"
                className={sectionStyles.showMoreButton}
                onClick={showAll}
                data-testid="respawn-history-show-more"
              >
                Show {remaining} more
              </button>
            )}
          </>
        )}
      </div>
    </CollapsibleSection>
  );
}
