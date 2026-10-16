"use client";
// +feature: backlog:item-detail-dependencies

import type { BacklogItem, ItemDependency } from "@/lib/hooks/useBacklogService";
import { isBlockerUnresolved } from "@/lib/backlog/dependencies";
import * as styles from "./DependenciesSection.css";

function DependencyList({ label, deps, testId }: { label: string; deps: ItemDependency[]; testId: string }) {
  if (deps.length === 0) return null;
  return (
    <div data-testid={testId}>
      <h4 className={styles.heading}>{label}</h4>
      <ul className={styles.list}>
        {deps.map((d) => (
          <li key={d.itemId}>
            {d.title || d.publicId || d.itemId}
            <span className={styles.status}>
              ({d.status}
              {isBlockerUnresolved(d) ? "" : ", resolved"})
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Item-detail view of the dependency edges: what this item is blocked by and
 * what is waiting on it. Renders nothing for an item with no edges.
 */
export function DependenciesSection({ item }: { item: Pick<BacklogItem, "blockedBy" | "blocking"> }) {
  const blockedBy = item.blockedBy ?? [];
  const blocking = item.blocking ?? [];
  if (blockedBy.length === 0 && blocking.length === 0) return null;
  return (
    <section className={styles.section} aria-label="Dependencies" data-testid="backlog-dependencies">
      <DependencyList label="Blocked by" deps={blockedBy} testId="backlog-blocked-by" />
      <DependencyList label="Blocking" deps={blocking} testId="backlog-blocking" />
    </section>
  );
}
