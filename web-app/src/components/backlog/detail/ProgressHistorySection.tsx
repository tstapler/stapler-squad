"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { useShowMore } from "@/lib/hooks/useShowMore";
import * as styles from "../BacklogItemDetail.css";
import * as sectionStyles from "./ProgressHistorySection.css";

function formatDate(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export interface ProgressHistorySectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

const SHOW_MORE_CAP = 8;

/**
 * The implementer's report_progress audit trail — extracted verbatim from
 * BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4e), collapsed by default.
 * Caps its default rendering to the 8 most recent notes via `useShowMore`
 * (Task 3.1.4e2, Blocker C fix).
 */
export function ProgressHistorySection({ item, defaultExpanded }: ProgressHistorySectionProps) {
  const { visible, hasMore, remaining, showAll } = useShowMore(
    item.id,
    "progress-history",
    item.progressNotes,
    SHOW_MORE_CAP
  );

  if (item.progressNotes.length === 0) return null;

  return (
    <CollapsibleSection sectionKey="progress-history" title="Progress History" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <div className={styles.progressNoteList} role="list" aria-label="Implementer progress history">
          {visible.map((n) => (
            <div key={n.id} className={styles.progressNoteItem} role="listitem">
              <div className={styles.progressNoteMeta}>
                <span>Criterion #{n.criterionIndex}</span>
                <span>·</span>
                <span>{n.status}</span>
                {n.createdAt && (
                  <>
                    <span>·</span>
                    <span>{formatDate(n.createdAt)}</span>
                  </>
                )}
              </div>
              {n.note && <span>{n.note}</span>}
            </div>
          ))}
        </div>
        {hasMore && (
          <button
            type="button"
            className={sectionStyles.showMoreButton}
            onClick={showAll}
            data-testid="progress-history-show-more"
          >
            Show {remaining} more
          </button>
        )}
      </div>
    </CollapsibleSection>
  );
}
