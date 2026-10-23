"use client";
// +feature: backlog:dependency-chip

import type { KeyboardEvent, MouseEvent } from "react";
import { Ban } from "lucide-react";
import type { ItemDependency } from "@/lib/hooks/useBacklogService";
import { unresolvedBlockers } from "@/lib/backlog/dependencies";
import * as styles from "./DependencyChip.css";

interface DependencyChipProps {
  /** The item's blockedBy edges; resolved ones are filtered out. Renders nothing when none remain. */
  deps: ItemDependency[];
  /** Called with the first unresolved blocker's itemId on click / Enter / Space. */
  onItemClick?: (itemId: string) => void;
}

const depTitle = (d: ItemDependency) => d.title || d.publicId || d.itemId;

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

  return (
    // data-action-button makes the card's handleCardClick ignore clicks anywhere in this subtree.
    <span
      className={styles.chip}
      role="button"
      tabIndex={0}
      data-action-button="true"
      data-testid="dependency-chip"
      aria-label={`Blocked by ${depTitle(primary)}${rest.length > 0 ? ` and ${rest.length} more` : ""}`}
      title={`Blocked by: ${depTitle(primary)} (${primary.status})`}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      <Ban size={12} aria-hidden="true" />
      <span className={styles.label}>{depTitle(primary)}</span>
      {rest.length > 0 && (
        <span
          className={styles.more}
          data-testid="dependency-chip-more"
          title={rest.map((d) => `${depTitle(d)} (${d.status})`).join("\n")}
        >
          +{rest.length} more
        </span>
      )}
    </span>
  );
}
