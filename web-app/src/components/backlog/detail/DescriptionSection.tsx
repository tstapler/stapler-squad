"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "../BacklogItemDetail.css";
import * as markdownStyles from "../markdownBody.css";

export interface DescriptionSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

/**
 * The item's markdown description — the field a user actually fills in, so
 * it seeds expanded by default. `defaultExpanded` seeds the initial
 * `useSectionExpandState`-backed value only; a stored per-item preference
 * can still collapse it.
 */
export function DescriptionSection({ item, defaultExpanded }: DescriptionSectionProps) {
  return (
    <CollapsibleSection sectionKey="description" title="Description" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        {item.description ? (
          <div className={markdownStyles.markdownBody} data-testid="backlog-description-rendered">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.description}</ReactMarkdown>
          </div>
        ) : (
          <p className={styles.emptyText}>No description.</p>
        )}
      </div>
    </CollapsibleSection>
  );
}
