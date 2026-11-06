"use client";

import type { ReactNode } from "react";
import type { FlowSectionStatus } from "@/gen/session/v1/backlog_flow_pb";
import { Skeleton } from "@/components/ui/Skeleton";
import { srOnly } from "@/components/ui/LiveRegion.css";
import type { FlowSectionDef } from "./flowSections";
import * as styles from "./flow.css";

interface FlowSectionProps {
  def: FlowSectionDef;
  /** Undefined until the first response. */
  status: FlowSectionStatus | undefined;
  loading: boolean;
  open: boolean;
  onToggle: () => void;
  summaryLine: string | null;
  onReload: () => void;
  children?: ReactNode;
}

/**
 * One independent section: its own skeleton, error block or content, driven by its own status.
 * A failed section shows only its error block; the page keeps rendering the others.
 */
export function FlowSection({ def, status, loading, open, onToggle, summaryLine, onReload, children }: FlowSectionProps) {
  const failed = status?.status === "failed";
  const bodyId = `${def.id}-body`;

  let body: ReactNode = null;
  if (loading) {
    body = (
      <div className={styles.skeletonRows} aria-busy="true" data-testid={`flow-skeleton-${def.id}`}>
        <Skeleton height={16} width="60%" />
        <Skeleton height={16} width="40%" />
      </div>
    );
  } else if (failed) {
    body = (
      <div className={styles.sectionError} data-testid={`flow-section-error-${def.id}`}>
        <span>
          {def.shortName} section failed to load: {status?.error || "unknown error"}
        </span>
        <button type="button" className={styles.textButton} onClick={onReload}>
          Reload
        </button>
      </div>
    );
  } else {
    body = (
      <>
        {status?.status === "partial" && status.error && <p className={styles.notice}>Partial data: {status.error}</p>}
        {children}
      </>
    );
  }

  return (
    <section className={styles.section} data-testid={`flow-section-${def.id}`}>
      <h2 id={def.id} tabIndex={-1} className={def.headingHidden ? srOnly : styles.sectionHeading}>
        {def.title}
      </h2>
      {def.collapsible && !loading && (
        <div className={styles.sectionSummary}>
          <button
            type="button"
            className={styles.textButton}
            aria-expanded={open}
            aria-controls={bodyId}
            onClick={onToggle}
            data-testid={`flow-toggle-${def.id}`}
          >
            {open ? "Hide" : "Show"}
            <span className={srOnly}> {def.title}</span>
          </button>
          {summaryLine && <span>{summaryLine}</span>}
        </div>
      )}
      {(!def.collapsible || open || loading) && <div id={bodyId}>{body}</div>}
    </section>
  );
}
