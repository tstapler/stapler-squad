"use client";

import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { DiffRenderer } from "@/components/shared/DiffRenderer";

interface DiffViewerProps {
  // Props kept for backward compatibility but data now comes from SessionVcsContext.
}

/** Session-aware diff viewer — reads from SessionVcsContext. */
export function DiffViewer(_props: DiffViewerProps) {
  const { diff: rawDiff, diffLoading: loading, refreshDiff } = useSessionVcsContext();

  return (
    <DiffRenderer
      content={rawDiff?.content ?? ""}
      added={rawDiff?.added ?? 0}
      removed={rawDiff?.removed ?? 0}
      loading={loading}
      onRefresh={refreshDiff}
    />
  );
}
