"use client";

import { CheckCircle2, Clock, XCircle } from "lucide-react";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import type { CheckItemSummary } from "@/lib/vcs/types";
import { visuallyHidden } from "@/styles/a11y.css";
import * as styles from "./VcsWidgetCheckList.css";

interface VcsWidgetCheckListProps {
  checks: CheckItemSummary[];
}

// Local equivalent of VcsWidgetGithubRow's `ciClassName` (not exported from
// that module) — this is the second usage of this conclusion→style mapping,
// not yet a third-usage abstraction-extraction trigger.
function checkClassName(conclusion: string): string {
  switch (conclusion) {
    case "success":
      return styles.checkSuccess;
    case "failure":
      return styles.checkFailure;
    default:
      return styles.checkPending;
  }
}

function checkIcon(conclusion: string) {
  switch (conclusion) {
    case "success":
      return CheckCircle2;
    case "failure":
      return XCircle;
    default:
      return Clock;
  }
}

// Human-readable text alternative for `conclusion` — the icon+color pair
// above is aria-hidden, so without this a check row announces its name but
// never whether it passed (screen-reader-only, per design-review finding).
function checkConclusionLabel(conclusion: string): string {
  switch (conclusion) {
    case "success":
      return "Passed";
    case "failure":
      return "Failed";
    default:
      return "Pending";
  }
}

export function VcsWidgetCheckList({ checks }: VcsWidgetCheckListProps) {
  if (checks.length === 0) return null;

  return (
    <CollapsibleSection
      sectionKey="ci-checks"
      title={`Checks (${checks.length})`}
      defaultExpanded={false}
    >
      <ul className={styles.list}>
        {checks.map((check) => {
          const Icon = checkIcon(check.conclusion);
          return (
            <li key={check.context || check.name} className={styles.row}>
              <Icon aria-hidden="true" size={14} className={checkClassName(check.conclusion)} />
              <span className={visuallyHidden}>{checkConclusionLabel(check.conclusion)}: </span>
              <span className={styles.name}>{check.name}</span>
              {check.context && check.context !== check.name && (
                <span className={styles.context}>{check.context}</span>
              )}
            </li>
          );
        })}
      </ul>
    </CollapsibleSection>
  );
}
