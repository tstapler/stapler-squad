"use client";
// +feature: backlog-plan-content-viewer

import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { InlineNotice } from "@/components/common/InlineNotice";
import { InlineError } from "../InlineError";
import * as styles from "../BacklogItemDetail.css";
import * as markdownStyles from "../markdownBody.css";

export interface PlanArtifactsSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
  /** Called with the fetched plan.md mtime whenever content loads successfully — the parent threads this into ApprovePlan/RejectPlan's expected_modified_at_unix_ms. */
  onMtimeChange?: (mtimeUnixMs: number) => void;
}

/** Collapsed by default (secondary info) — Story 3.1.4, Task 3.1.4a. */
export function PlanArtifactsSection({ item, defaultExpanded, onMtimeChange }: PlanArtifactsSectionProps) {
  const { getPlanArtifactContent } = useBacklogService();
  const [content, setContent] = useState<string | null>(null);
  const [displayedMtime, setDisplayedMtime] = useState<number | null>(null);
  const [newerAvailable, setNewerAvailable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function fetchContent(force = false) {
    if (!item.planArtifactsPath) return;
    try {
      const res = await getPlanArtifactContent(item.id, "plan.md");
      if (!res) return;
      if (!force && displayedMtime !== null && Number(res.modifiedAtUnixMs) !== displayedMtime) {
        setNewerAvailable(true);
        return;
      }
      setContent(res.content);
      setDisplayedMtime(Number(res.modifiedAtUnixMs));
      onMtimeChange?.(Number(res.modifiedAtUnixMs));
      setNewerAvailable(false);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load plan content.");
    }
  }

  useEffect(() => {
    void fetchContent();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id, item.planArtifactsPath, item.updatedAt]);

  if (!item.planArtifactsPath) return null;

  return (
    <CollapsibleSection sectionKey="plan-artifacts" title="Plan Artifacts" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <code className={styles.artifactsPath}>{item.planArtifactsPath}</code>
        {newerAvailable && (
          <InlineNotice
            message="A newer plan is available."
            actions={[
              {
                label: "Reload",
                onClick: () => void fetchContent(true),
                variant: "primary",
              },
            ]}
            data-testid="plan-content-stale-notice"
          />
        )}
        {error && (
          <InlineError type="transient" onRetry={() => void fetchContent()} onDismiss={() => setError(null)} customMessage={error} />
        )}
        {content !== null && (
          <div className={markdownStyles.markdownBody} data-testid="backlog-plan-content-rendered">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
