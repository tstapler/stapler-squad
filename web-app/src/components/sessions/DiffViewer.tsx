"use client";

import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { DiffRenderer } from "@/components/shared/DiffRenderer";
import { CIStatusBadge } from "./CIStatusBadge";
import { Session } from "@/gen/session/v1/types_pb";
import { diffHeader } from "./DiffViewer.css";

interface DiffViewerProps {
  session?: Session;
}

/** Session-aware diff viewer — reads from SessionVcsContext. */
export function DiffViewer({ session }: DiffViewerProps) {
  const { diff: rawDiff, diffLoading: loading, refreshDiff } = useSessionVcsContext();

  return (
    <>
      {session?.githubPrNumber ? (
        <div className={diffHeader}>
          <CIStatusBadge
            checkConclusion={session.githubCheckConclusion}
            prUrl={session.githubPrUrl}
            prNumber={session.githubPrNumber}
            lastChecked={session.lastPrStatusCheck}
          />
        </div>
      ) : null}
      <DiffRenderer
        content={rawDiff?.content ?? ""}
        added={rawDiff?.added ?? 0}
        removed={rawDiff?.removed ?? 0}
        loading={loading}
        onRefresh={refreshDiff}
      />
    </>
  );
}
