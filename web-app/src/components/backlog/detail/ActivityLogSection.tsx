"use client";

// +feature: backlog:activity-log

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { useShowMore } from "@/lib/hooks/useShowMore";
import { formatDate } from "@/lib/backlog/formatDate";
import * as styles from "../BacklogItemDetail.css";
import * as sectionStyles from "./ProgressHistorySection.css";

export interface ActivityLogSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

const SHOW_MORE_CAP = 8;

/**
 * The ungated, free-form activity log posted via post_backlog_update
 * (backlog-item-activity-log, ADR-001's sibling table to
 * BacklogProgressNote) — structurally cloned from ProgressHistorySection.tsx
 * but deliberately renders a visually distinct meta-line ("<author> ·
 * <date>", never "Criterion #N · status") so an informal note is never
 * confusable with an official progress mark. Collapsed by default; caps its
 * default rendering to the 8 most recent notes via useShowMore.
 *
 * The list carries a stable `backlog-activity-log` DOM id, and each note an
 * `activity-note-<id>` one, so other surfaces (DiagnoseHistoryList's "View
 * note" link for an InconclusiveNoteFiled dispatch, plan.md Task 8.2.2b) can
 * link into this feed instead of duplicating note text of their own.
 * DiagnoseDispatchProto currently carries only the note's text, not the
 * resulting activity-note's id, so that link targets the list as a whole
 * rather than the specific note -- a precise per-note anchor needs a
 * backend-side id correlation this feature doesn't yet expose.
 */
export function ActivityLogSection({ item, defaultExpanded }: ActivityLogSectionProps) {
  const { visible, hasMore, remaining, showAll } = useShowMore(
    item.id,
    "activity-log",
    item.activityNotes,
    SHOW_MORE_CAP
  );

  if ((item.activityNotes ?? []).length === 0) return null;

  return (
    <CollapsibleSection sectionKey="activity-log" title="Activity Log" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <div
          id="backlog-activity-log"
          className={styles.progressNoteList}
          role="list"
          aria-label="Backlog item activity log"
        >
          {visible.map((n) => {
            const author = n.authorSessionTitle || (n.authorSessionUuid ? n.authorSessionUuid.slice(0, 8) : "") || "manual";
            return (
              <div key={n.id} id={`activity-note-${n.id}`} className={styles.progressNoteItem} role="listitem">
                <div className={styles.progressNoteMeta}>
                  <span>{author}</span>
                  {n.createdAt && (
                    <>
                      <span>·</span>
                      <span>{formatDate(n.createdAt)}</span>
                    </>
                  )}
                </div>
                {n.message && <span>{n.message}</span>}
              </div>
            );
          })}
        </div>
        {hasMore &&
          // Expands the already-fetched, already-rendered rest of this list
          // client-side -- no separate analytics event to add here.
          (
            // analytics-exempt
            <button
              type="button"
              className={sectionStyles.showMoreButton}
              onClick={showAll}
              data-testid="activity-log-show-more"
            >
              Show {remaining} more
            </button>
          )}
      </div>
    </CollapsibleSection>
  );
}
