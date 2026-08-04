"use client";

import { useState } from "react";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { InlineNotice } from "@/components/common/InlineNotice";
import { getAvailableActions } from "@/lib/backlog/itemActions";
import * as styles from "../BacklogItemDetail.css";
import * as gateStyles from "../GateVerdictBox.css";
import { ActionButtonLabel } from "./ActionButtonLabel";

export interface PullRequestSectionProps {
  item: BacklogItem;
  actionLoading: string | null;
  onMarkDone: () => void;
  /**
   * Manual escape-hatch "Link existing PR" (project 7a383b3b) — associates a
   * PR that already exists on GitHub with this item, via UpdateBacklogItem's
   * pr_url/pr_number handling. Only offered while `actions.has("link_existing_pr")`
   * (item.status === "review" with no PR yet, itemActions.ts) — the same v1
   * scope SetBacklogItemPRAndTransition's hardcoded ExpectedStatus=review
   * enforces server-side.
   */
  onLinkPr?: (prUrl: string, prNumber: number) => Promise<void>;
  /**
   * Task 5.3.1c (backlog-event-driven-updates): true once an
   * ArchivedEvent/RemovedEvent has arrived for this item — disables "Mark
   * Done" the same way the Actions panel's own mutating buttons are
   * replaced entirely once the item is no longer live.
   */
  readOnly?: boolean;
}

/**
 * "Pull Request" block. Historically only rendered while
 * `item.status === "pr_pending"` (Story 3.1.2 Task 3.1.2c); now also
 * rendered while `item.status === "review"` so the "Link existing PR" form
 * below has somewhere to live — see the render guard this component's own
 * caller (BacklogItemDetail.tsx) applies. Only rendered when relevant, so
 * always-expanded by default is correct here. This is the sole data source
 * for the PR URL/number text (D4) — VersionControlSection (Story 3.1.4) opts
 * its own VcsWidget out of the duplicate PR link via `showPrLink={false}`
 * when this section is also rendering.
 */
export function PullRequestSection({ item, actionLoading, onMarkDone, onLinkPr, readOnly = false }: PullRequestSectionProps) {
  const [showLinkForm, setShowLinkForm] = useState(false);
  const [prUrlInput, setPrUrlInput] = useState("");
  const [prNumberInput, setPrNumberInput] = useState("");
  const [linkError, setLinkError] = useState<string | null>(null);

  const { actions } = getAvailableActions(item);
  const canLinkPr = actions.has("link_existing_pr") && !!onLinkPr;

  const resetLinkForm = () => {
    setShowLinkForm(false);
    setPrUrlInput("");
    setPrNumberInput("");
    setLinkError(null);
  };

  const handleLinkSubmit = async () => {
    const prNumber = Number(prNumberInput);
    if (!prUrlInput.trim() || !Number.isInteger(prNumber) || prNumber <= 0 || !onLinkPr) return;
    setLinkError(null);
    try {
      await onLinkPr(prUrlInput.trim(), prNumber);
      resetLinkForm();
    } catch (err) {
      setLinkError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <CollapsibleSection sectionKey="pull-request" title="Pull Request" defaultExpanded={true}>
      <div className={styles.section}>
        {item.status === "pr_pending" && (
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
        )}

        {canLinkPr && (
          <div className={styles.section}>
            <button
              className={gateStyles.overrideToggle}
              aria-expanded={showLinkForm}
              disabled={readOnly}
              onClick={() => setShowLinkForm((prev) => !prev)}
              data-testid="backlog-link-existing-pr-toggle"
            >
              Link existing PR {showLinkForm ? "▾" : "▸"}
            </button>

            {showLinkForm && (
              <div
                role="form"
                aria-label="Link existing pull request"
                className={gateStyles.overrideForm}
                data-testid="backlog-link-existing-pr-form"
              >
                <label htmlFor="link-pr-url" className={gateStyles.formLabel}>
                  PR URL
                </label>
                <input
                  id="link-pr-url"
                  type="text"
                  placeholder="https://github.com/owner/repo/pull/123"
                  value={prUrlInput}
                  onChange={(e) => setPrUrlInput(e.target.value)}
                  className={styles.manualReviewSelect}
                  data-testid="backlog-link-existing-pr-url"
                />

                <label htmlFor="link-pr-number" className={gateStyles.formLabel}>
                  PR number
                </label>
                <input
                  id="link-pr-number"
                  type="number"
                  min={1}
                  placeholder="123"
                  value={prNumberInput}
                  onChange={(e) => setPrNumberInput(e.target.value)}
                  className={styles.manualReviewSelect}
                  data-testid="backlog-link-existing-pr-number"
                />

                {linkError && <InlineNotice message={linkError} data-testid="backlog-link-existing-pr-error" />}

                <div className={gateStyles.formActions}>
                  <button
                    className={gateStyles.secondaryButton}
                    onClick={resetLinkForm}
                    data-testid="backlog-link-existing-pr-cancel"
                  >
                    Cancel
                  </button>
                  <button
                    className={gateStyles.dangerButton}
                    disabled={
                      actionLoading !== null ||
                      !prUrlInput.trim() ||
                      !Number.isInteger(Number(prNumberInput)) ||
                      Number(prNumberInput) <= 0
                    }
                    aria-busy={actionLoading === "link_existing_pr"}
                    onClick={() => void handleLinkSubmit()}
                    data-testid="backlog-link-existing-pr-submit"
                  >
                    <ActionButtonLabel pending={actionLoading === "link_existing_pr"} label="Link PR" />
                  </button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
