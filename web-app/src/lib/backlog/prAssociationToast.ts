import type { PrAssociation } from "@/lib/hooks/useBacklogService";

/** Toast text for a manual PR link, worded from what the server reported rather than assumed. */
export function prAssociationToast(association: PrAssociation, prNumber: number): string {
  switch (association) {
    case "linked_and_moved": return `PR #${prNumber} linked — item moved to pr_pending.`;
    case "already_tracked": return `Already tracking PR #${prNumber} — nothing changed.`;
    default: return `PR #${prNumber} linked.`;
  }
}
