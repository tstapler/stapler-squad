import {
  CheckCircle,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  AlertCircle,
  MinusCircle,
  GitPullRequestDraft,
  GitMerge,
  Clock,
  Ban,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { MergeabilityState } from "@/lib/vcs/mergeability";
import { pill } from "./MergeabilityPill.css";

interface MergeabilityPillProps {
  state: MergeabilityState;
}

interface PillContent {
  icon: LucideIcon;
  label: string;
}

// Exhaustive switch, no default case — a new MergeabilityState member that
// isn't handled here fails `tsc --noEmit` with "not all code paths return a
// value" instead of silently falling through to an unlabeled pill.
function pillContent(state: MergeabilityState): PillContent {
  switch (state) {
    case "shipped":
      return { icon: CheckCircle, label: "Shipped" };
    case "ready_to_merge":
      return { icon: CheckCircle2, label: "Ready to merge" };
    case "draft":
      return { icon: GitPullRequestDraft, label: "Draft" };
    case "conflicted":
      return { icon: GitMerge, label: "Conflicts" };
    case "diverged":
      return { icon: GitMerge, label: "Diverged from base" };
    case "changes_requested":
      return { icon: AlertCircle, label: "Changes requested" };
    case "ci_failing":
      return { icon: XCircle, label: "CI failing" };
    case "ci_pending":
      return { icon: Clock, label: "CI running" };
    case "closed_unshipped":
      return { icon: Ban, label: "Closed — not merged" };
    case "snapshot_unavailable":
      return { icon: AlertTriangle, label: "Status unavailable" };
    case "no_pr":
      return { icon: MinusCircle, label: "No PR" };
  }
}

export function MergeabilityPill({ state }: MergeabilityPillProps) {
  const { icon: Icon, label } = pillContent(state);
  return (
    <span className={pill({ state })}>
      <Icon aria-hidden="true" size={14} />
      {label}
    </span>
  );
}
