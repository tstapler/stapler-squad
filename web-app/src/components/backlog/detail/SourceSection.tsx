"use client";

// +feature: backlog:item-detail-source-section

// lucide-react (this repo's pinned v1.14) ships no brand "Github" glyph —
// CircleDot is the closest available icon to GitHub's own issue-tracker
// mark and is used here instead of the `Github` icon plan.md's snippet
// assumed exists (see BacklogItemCard.tsx for the same substitution).
import { CircleDot } from "lucide-react";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "./SourceSection.css";

export interface SourceSectionProps {
  externalUrl: string;
  externalId?: string;
  labels: string[];
  defaultExpanded: boolean;
}

/**
 * "Source" section — provenance for a backlog item imported from an
 * external tracker (currently GitHub issues). Guard-at-call-site, matching
 * PullRequestSection's pattern: the parent (BacklogItemDetail.tsx) decides
 * whether to render this at all (`item.externalUrl` truthy), this component
 * never re-derives that guard itself.
 *
 * externalId is still optional in the type — nothing enforces that it always
 * accompanies a real externalUrl — so the issue number portion is rendered
 * defensively: only shown when externalId is actually present, to avoid a
 * literal "Issue #undefined".
 */
export function SourceSection({ externalUrl, externalId, labels, defaultExpanded }: SourceSectionProps) {
  return (
    <CollapsibleSection sectionKey="source" title="Source" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <a
          href={externalUrl}
          target="_blank"
          rel="noopener noreferrer"
          className={styles.link}
          title="Open on GitHub"
        >
          <CircleDot aria-hidden="true" size={14} />
          {externalId ? `Issue #${externalId}` : "Issue"}
        </a>
        {labels.length > 0 && (
          <div className={styles.labels}>
            {labels.map((label) => (
              <span key={label} className={styles.labelBadge} title={label}>
                {label}
              </span>
            ))}
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
