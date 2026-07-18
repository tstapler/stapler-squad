"use client";
// +feature: backlog:empty-state

import * as styles from "./BacklogEmptyState.css";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BacklogEmptyStateProps {
  onCreateItem: () => void;
}

interface FilterZeroStateProps {
  onClearFilters: () => void;
}

interface FooterNudgeProps {}

// ---------------------------------------------------------------------------
// Lifecycle diagram data
// ---------------------------------------------------------------------------

const LIFECYCLE_NODES = [
  { label: "idea", active: true },
  { label: "ready", active: false },
  { label: "in progress", active: false },
  { label: "review", active: false },
  { label: "done", active: false },
];

export function LifecycleDiagram() {
  return (
    <div className={styles.lifecycleDiagram} aria-hidden="true" data-testid="backlog-lifecycle-diagram">
      {LIFECYCLE_NODES.map((node, i) => (
        <div key={node.label} style={{ display: "contents" }}>
          <div
            className={`${styles.lifecycleNode} ${node.active ? styles.lifecycleNodeActive : styles.lifecycleNodeInactive}`}
            data-testid={`backlog-lifecycle-node-${node.label}`}
          >
            <span>{node.active ? "◉" : "○"}</span>
            <span>{node.label}</span>
            {node.active && (
              <span style={{ fontSize: "10px", opacity: 0.75 }}>(you start here)</span>
            )}
          </div>
          {i < LIFECYCLE_NODES.length - 1 && (
            <span className={styles.lifecycleArrow}>──►</span>
          )}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// BacklogEmptyState
// ---------------------------------------------------------------------------

export function BacklogEmptyState({ onCreateItem }: BacklogEmptyStateProps) {
  return (
    <section role="region" aria-label="Backlog — empty" className={styles.wrapper} data-testid="backlog-empty-state">
      <h2 className={styles.headline} data-testid="backlog-empty-headline">Your backlog is empty.</h2>
      <p className={styles.subline}>
        Create a work item, define what &ldquo;done&rdquo; looks like, spawn an agent — the system reviews output automatically.
      </p>
      <LifecycleDiagram />
      <button
        className={styles.ctaButton}
        autoFocus
        onClick={onCreateItem}
        data-testid="backlog-empty-cta-button"
      >
        + Create First Item
      </button>
    </section>
  );
}

// ---------------------------------------------------------------------------
// FilterZeroState
// ---------------------------------------------------------------------------

export function FilterZeroState({ onClearFilters }: FilterZeroStateProps) {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label="No results"
      className={styles.filterZeroWrapper}
      data-testid="backlog-filter-zero-state"
    >
      <p className={styles.filterZeroText}>No items match your filters.</p>
      <button className={styles.clearFiltersButton} onClick={onClearFilters} data-testid="backlog-clear-filters-button">
        Clear filters
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// FooterNudge
// ---------------------------------------------------------------------------

export function FooterNudge(_: FooterNudgeProps) {
  return (
    <div role="status" aria-live="polite" className={styles.footerNudge}>
      No items are currently in progress. Mark an item ready and spawn a session to start working.
    </div>
  );
}
