// +feature: unfinished-github-prs
"use client";

import { useId, useState } from "react";
import type { UserPR } from "@/gen/session/v1/types_pb";
import * as styles from "./PRCard.css";

const BODY_PREVIEW_CHARS = 300;

export function PRExpandedInfo({ pr }: { pr: UserPR }) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const chars = Array.from(pr.body);
  const body = chars.length > BODY_PREVIEW_CHARS ? `${chars.slice(0, BODY_PREVIEW_CHARS).join("")}…` : pr.body;
  return (
    <div className={styles.expandedInfo}>
      <button
        type="button"
        className={styles.expandToggle}
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((v) => !v)}
        data-testid="pr-details-toggle"
      >
        {open ? "Hide details" : "Show details"}
      </button>
      {open && (
        <div id={panelId} data-testid="pr-details">
          {body && <p className={styles.bodyPreview}>{body}</p>}
          {pr.labels.length > 0 && (
            <ul className={styles.labelList} aria-label="Labels">
              {pr.labels.map((l) => (
                <li key={l} className={styles.chipNeutral}>
                  {l}
                </li>
              ))}
            </ul>
          )}
          <p className={styles.statLine} data-testid="pr-stats">
            +{pr.additions} / -{pr.deletions} / {pr.changedFiles} files
          </p>
          {pr.requestedReviewerLogins.length > 0 && (
            <p className={styles.statLine} data-testid="pr-reviewers">
              Awaiting review: {pr.requestedReviewerLogins.join(", ")}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
