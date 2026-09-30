"use client";
// +feature: backlog:claim-conflict-banner

import { useCallback, useEffect, useState } from "react";
import { useBacklogService, type CrossHostClaimStatus } from "@/lib/hooks/useBacklogService";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { ClaimConflictBanner } from "./ClaimConflictBanner";
import { claimingHostLabel } from "./ClaimedElsewhereNotice";

/**
 * Runs the cross-host claim check for an item's external URL in the
 * background and renders ClaimConflictBanner only when there is something to
 * say. Renders nothing while loading, when the feature is off, when the URL is
 * unclaimed, or when the RPC fails (design/ux.md Surface 1 error table).
 */
export function ItemClaimBanner({ externalUrl }: { externalUrl: string }) {
  const { checkCrossHostClaim, resolveClaimDispute } = useBacklogService();
  const [status, setStatus] = useState<CrossHostClaimStatus | null>(null);
  const [dismissed, setDismissed] = useState(false);

  const runCheck = useCallback(async () => {
    // Optional call: tests that stub useBacklogService without this method
    // must keep rendering the detail view.
    const result = await checkCrossHostClaim?.(externalUrl);
    setStatus(result ?? null);
  }, [checkCrossHostClaim, externalUrl]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const result = await checkCrossHostClaim?.(externalUrl);
      if (!cancelled) setStatus(result ?? null);
    })();
    return () => {
      cancelled = true;
    };
  }, [checkCrossHostClaim, externalUrl]);

  if (!status || !status.enabled || dismissed) return null;

  if (!status.claim) {
    if (status.checked) return null;
    return <ClaimConflictBanner hostname="" itemDeepLink="" indeterminate onRetry={() => void runCheck()} />;
  }

  const claim = status.claim;
  const hostname = claimingHostLabel(claim);
  return (
    <ClaimConflictBanner
      hostname={hostname}
      itemDeepLink={claim.itemDeepLink}
      lastSeenAt={status.claimedAtUnix > 0 ? formatRelativeTime(status.claimedAtUnix * 1000) : undefined}
      disputed={claim.disputed}
      onOverride={(reason) => {
        // No server-side block exists for an item that has not been dequeued;
        // the override is recorded client-side and hides the banner for this view.
        console.info("[claim] override acknowledged", { externalUrl, claimingHost: claim.claimingHostId, reason });
        setDismissed(true);
      }}
      onResolveDispute={async (reason) => {
        await resolveClaimDispute(externalUrl, reason);
        await runCheck();
      }}
    />
  );
}
