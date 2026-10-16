"use client";
// +feature: backlog:claim-chip

import { AlertTriangle, Link2 } from "lucide-react";
import type { ClaimedElsewhere } from "@/lib/hooks/useBacklogService";
import { claimingHostLabel } from "./ClaimedElsewhereNotice";
import * as styles from "./BacklogItemCard.css";

/**
 * Compact board-card indicator that another host claimed this item's external
 * URL (design/ux.md Surface 2). A heads-up, not a control: clicking the card
 * opens the detail view where ClaimConflictBanner offers the actions.
 */
export function ClaimChip({ claim }: { claim: ClaimedElsewhere }) {
  const host = claimingHostLabel(claim);
  const Icon = claim.disputed ? AlertTriangle : Link2;
  const label = claim.disputed ? "Disputed" : `Claimed: ${host}`;
  return (
    <span
      className={styles.claimChip}
      title={claim.disputed ? `Claim disputed between hosts, currently ${host}` : `Claimed by ${host}`}
      data-testid="claim-chip"
    >
      <Icon aria-hidden="true" size={12} />
      <span className={styles.claimChipLabel}>{label}</span>
    </span>
  );
}
