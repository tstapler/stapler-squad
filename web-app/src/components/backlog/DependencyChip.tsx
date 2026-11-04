"use client";
// +feature: backlog:dependency-chip

import type { KeyboardEvent, MouseEvent } from "react";
import { Ban } from "lucide-react";
import type { ItemDependency } from "@/lib/hooks/useBacklogService";
import { dependencyLabel, unresolvedBlockers } from "@/lib/backlog/dependencies";
import * as styles from "./DependencyChip.css";

interface DependencyChipProps {
  /** The item's blockedBy edges; resolved ones are filtered out. Renders nothing when none remain. */
  deps: ItemDependency[];
  /** Called with the first unresolved blocker's itemId on click / Enter / Space. */
  onItemClick?: (itemId: string) => void;
}

const describeDep = (d: ItemDependency) => `${dependencyLabel(d)} (${d.status})`;

export function DependencyChip({ deps, onItemClick }: DependencyChipProps) {
  const open = unresolvedBlockers(deps);
  if (open.length === 0) return null;

  const [primary, ...rest] = open;
  const activate = () => onItemClick?.(primary.itemId);

  const handleClick = (e: MouseEvent) => {
    e.stopPropagation();
    activate();
  };
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Enter" && e.key !== " ") return;
    e.preventDefault();
    e.stopPropagation();
    activate();
  };

  const ariaLabel = `Blocked by ${open.map(describeDep).join(", ")}`;
  const interactive = onItemClick !== undefined;

  return (
    // data-action-button makes the card's handleCardClick ignore clicks anywhere in this subtree.
    <span
      className={interactive ? `${styles.chip} ${styles.interactive}` : styles.chip}
      {...(interactive ? { role: "button", tabIndex: 0, onKeyDown: handleKeyDown } : {})}
      data-action-button="true"
      data-testid="dependency-chip"
      aria-label={ariaLabel}
      title={`Blocked by: ${describeDep(primary)}`}
      onClick={handleClick}
    >
      <Ban size={12} aria-hidden="true" />
      <span className={styles.label}>{dependencyLabel(primary)}</span>
      {rest.length > 0 && (
        <span className={styles.more} data-testid="dependency-chip-more" title={rest.map(describeDep).join("\n")}>
          +{rest.length} more
        </span>
      )}
    </span>
  );
}
