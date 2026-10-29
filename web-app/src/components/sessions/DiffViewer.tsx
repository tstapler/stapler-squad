"use client";

import { useEffect } from "react";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { DiffPane } from "./DiffPane";
import { CIStatusBadge } from "./CIStatusBadge";
import type { Session } from "@/gen/session/v1/types_pb";
import { diffHeader } from "./DiffViewer.css";

interface DiffViewerProps {
  session?: Session;
}

/** Session-aware diff viewer — reads from SessionVcsContext. */
export function DiffViewer({ session }: DiffViewerProps) {
  const { diff: rawDiff, diffLoading: loading, refreshDiff, ensureDiff, diffContextLines, setDiffContextLines } =
    useSessionVcsContext();
  // Mounted only while the diff tab is shown, so this is the "tab is visible" trigger.
  useEffect(() => { ensureDiff(); }, [ensureDiff]);

  return (
    <DiffPane
      autoFocus
      header={
        session?.githubPrNumber ? (
          <div className={diffHeader}>
            <CIStatusBadge
              checkConclusion={session.githubCheckConclusion}
              prUrl={session.githubPrUrl}
              prNumber={session.githubPrNumber}
              lastChecked={session.lastPrStatusCheck}
            />
          </div>
        ) : null
      }
      content={rawDiff?.content ?? ""}
      added={rawDiff?.added ?? 0}
      removed={rawDiff?.removed ?? 0}
      loading={loading && !rawDiff}
      onRefresh={refreshDiff}
      contextLines={diffContextLines}
      onContextLinesChange={setDiffContextLines}
    />
  );
}
