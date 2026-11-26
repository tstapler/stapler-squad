"use client";
// +feature: backlog:item-detail-integration-check

import { useEffect, useState } from "react";

import { useBacklogService, type BacklogItem } from "@/lib/hooks/useBacklogService";
import { INTEGRATION_CHECK_LABEL, integrationCheckState } from "@/lib/backlog/integrationCheck";
import * as styles from "./DependenciesSection.css";

/** Epic detail: status of the epic's integration child with a link to it. Renders nothing without one. */
export function IntegrationCheckSection({
  item,
  onItemClick,
}: {
  item: Pick<BacklogItem, "integrationChildId" | "status">;
  onItemClick?: (itemId: string) => void;
}) {
  if (!item.integrationChildId) return null;
  return <IntegrationCheckBody childId={item.integrationChildId} epicDone={item.status === "done"} onItemClick={onItemClick} />;
}

function IntegrationCheckBody({
  childId,
  epicDone,
  onItemClick,
}: {
  childId: string;
  epicDone: boolean;
  onItemClick?: (itemId: string) => void;
}) {
  const { getBacklogItem } = useBacklogService();
  const [child, setChild] = useState<BacklogItem | null>(null);
  useEffect(() => {
    let cancelled = false;
    void getBacklogItem(childId).then((c) => {
      if (!cancelled) setChild(c);
    });
    return () => {
      cancelled = true;
    };
  }, [childId, getBacklogItem]);
  const state = epicDone ? "passed" : integrationCheckState(child ?? undefined);
  return (
    <section className={styles.section} aria-label="Integration check" data-testid="backlog-integration-check" data-state={state}>
      <h4 className={styles.heading}>{INTEGRATION_CHECK_LABEL[state]}</h4>
      {onItemClick && (
        <button type="button" className={styles.depLink} onClick={() => onItemClick(childId)}>
          {/* analytics-exempt */}
          View integration child
        </button>
      )}
    </section>
  );
}
