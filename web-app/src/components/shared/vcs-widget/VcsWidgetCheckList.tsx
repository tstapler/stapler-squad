"use client";

import { CheckCircle2, Clock, MinusCircle, XCircle } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import type { CheckItemSummary } from "@/lib/vcs/types";
import { visuallyHidden } from "@/styles/a11y.css";
import * as styles from "./VcsWidgetCheckList.css";

interface VcsWidgetCheckListProps {
  checks: CheckItemSummary[];
}

type CheckBucket = "success" | "failure" | "neutral" | "pending";

// `conclusion` is Checks-API-only and empty for a legacy Commit-Status-API
// item — falls back to `state` in that case, mirroring the same fallback
// github.getCheckConclusion already applies server-side (github/client.go)
// so a legacy-API check isn't mis-shown as still-pending regardless of its
// real outcome.
function effectiveConclusion(check: CheckItemSummary): string {
  const conclusion = check.conclusion.toLowerCase();
  return conclusion || check.state.toLowerCase();
}

// Buckets mirror getCheckConclusion's grouping (github/client.go) so a
// single check's per-item display agrees with how the collapsed rollup
// classifies the same conclusion values.
function checkBucket(check: CheckItemSummary): CheckBucket {
  switch (effectiveConclusion(check)) {
    case "success":
      return "success";
    case "failure":
    case "error":
    case "action_required":
    case "timed_out":
      return "failure";
    case "neutral":
    case "skipped":
    case "cancelled":
      return "neutral";
    default:
      return "pending";
  }
}

const CHECK_BUCKET_META: Record<CheckBucket, { className: string; icon: LucideIcon; label: string }> = {
  success: { className: styles.checkSuccess, icon: CheckCircle2, label: "Passed" },
  failure: { className: styles.checkFailure, icon: XCircle, label: "Failed" },
  neutral: { className: styles.checkNeutral, icon: MinusCircle, label: "Skipped" },
  pending: { className: styles.checkPending, icon: Clock, label: "Pending" },
};

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
          const meta = CHECK_BUCKET_META[checkBucket(check)];
          const Icon = meta.icon;
          return (
            <li key={check.context || check.name} className={styles.row}>
              <Icon aria-hidden="true" size={14} className={meta.className} />
              <span className={visuallyHidden}>{meta.label}: </span>
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
