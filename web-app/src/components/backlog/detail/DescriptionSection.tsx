"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "../BacklogItemDetail.css";
import * as markdownStyles from "../markdownBody.css";

export interface DescriptionSectionProps {
  item: BacklogItem;
}

/**
 * The item's markdown description — secondary info, collapsed by default
 * (Story 3.1.3, Task 3.1.3a).
 */
export function DescriptionSection({ item }: DescriptionSectionProps) {
  return (
    <CollapsibleSection sectionKey="description" title="Description" defaultExpanded={false}>
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
