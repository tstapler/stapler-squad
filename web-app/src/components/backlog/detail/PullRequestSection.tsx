"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "../BacklogItemDetail.css";
import { ActionButtonLabel } from "./ActionButtonLabel";

export interface PullRequestSectionProps {
  item: BacklogItem;
  actionLoading: string | null;
  onMarkDone: () => void;
  /**
   * Task 5.3.1c (backlog-event-driven-updates): true once an
   * ArchivedEvent/RemovedEvent has arrived for this item — disables "Mark
   * Done" the same way the Actions panel's own mutating buttons are
   * replaced entirely once the item is no longer live.
   */
  readOnly?: boolean;
}

/**
 * "Pull Request" block — only rendered while `item.status === "pr_pending"`
 * (guard preserved verbatim, Story 3.1.2 Task 3.1.2c). Only rendered when
 * relevant, so always-expanded by default is correct here. This is the sole
 * data source for the PR URL/number text (D4) — VersionControlSection
 * (Story 3.1.4) opts its own VcsWidget out of the duplicate PR link via
 * `showPrLink={false}` when this section is also rendering.
 */
export function PullRequestSection({ item, actionLoading, onMarkDone, readOnly = false }: PullRequestSectionProps) {
  return (
    <CollapsibleSection sectionKey="pull-request" title="Pull Request" defaultExpanded={true}>
      <div className={styles.section}>
        <div className={styles.reviewContextBox}>
          <div className={styles.reviewContextInfo}>
            {item.prUrl ? (
              <>
                <span className={styles.reviewContextLabel}>
                  PR #{item.prNumber} — waiting for merge
                </span>
                <a
                  className={styles.reviewContextSessionId}
                  href={item.prUrl}
                  target="_blank"
                  rel="noreferrer"
                  title="Open pull request on GitHub"
                >
                  {item.prUrl}
                </a>
              </>
            ) : (
              <span className={styles.reviewContextLabel}>
                PR pending — no URL recorded yet
              </span>
            )}
          </div>
          <button
            className={styles.actionButton}
            onClick={onMarkDone}
            disabled={actionLoading !== null || readOnly}
            aria-busy={actionLoading === "mark_done"}
            title="Mark done manually (if PR already merged)"
            data-testid="backlog-action-mark-done"
          >
            <ActionButtonLabel pending={actionLoading === "mark_done"} label="Mark Done" />
          </button>
        </div>
      </div>
    </CollapsibleSection>
  );
}
