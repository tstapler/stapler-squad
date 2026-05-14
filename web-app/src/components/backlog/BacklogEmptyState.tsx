"use client";
// +feature: backlog:empty-state

import { useState, useEffect, useRef } from "react";
import * as styles from "./BacklogEmptyState.css";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BacklogEmptyStateProps {
  onCreateItem: (data: { title: string; priority: number }) => Promise<void>;
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

function LifecycleDiagram() {
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
  const [showForm, setShowForm] = useState(false);
  const [title, setTitle] = useState("");
  const [priority, setPriority] = useState(3);
  const [submitting, setSubmitting] = useState(false);
  const [titleError, setTitleError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const ctaRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!showForm && ctaRef.current) {
      ctaRef.current.focus();
    }
  }, [showForm]);

  const handleCancel = () => {
    setShowForm(false);
    setTitle("");
    setPriority(3);
    setTitleError(null);
  };

  const handleSubmit = async () => {
    if (!title.trim()) {
      setTitleError("Title is required.");
      return;
    }
    setSubmitting(true);
    try {
      await onCreateItem({ title: title.trim(), priority });
    } catch (err) {
      setSubmitError("Failed to create item. Please try again.");
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  if (!showForm) {
    return (
      <section role="region" aria-label="Backlog — empty" className={styles.wrapper} data-testid="backlog-empty-state">
        <h2 className={styles.headline} data-testid="backlog-empty-headline">Your backlog is empty.</h2>
        <p className={styles.subline}>
          Create a work item, define what &ldquo;done&rdquo; looks like, spawn an agent — the system reviews output automatically.
        </p>
        <LifecycleDiagram />
        <button
          ref={ctaRef}
          className={styles.ctaButton}
          autoFocus
          onClick={() => setShowForm(true)}
          data-testid="backlog-empty-cta-button"
        >
          + Create First Item
        </button>
      </section>
    );
  }

  return (
    <section role="region" aria-label="Backlog — empty" className={styles.wrapper} data-testid="backlog-empty-state">
      <h2 className={styles.headline}>Your backlog is empty.</h2>
      <LifecycleDiagram />
      <div role="form" aria-label="Create new backlog item" className={styles.inlineForm} data-testid="backlog-empty-form">
        <div>
          <label htmlFor="item-title" className={styles.formLabel}>
            Title
          </label>
          <input
            id="item-title"
            type="text"
            autoFocus
            required
            placeholder="What do you want to build or fix?"
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              if (titleError) setTitleError(null);
              if (submitError) setSubmitError(null);
            }}
            className={styles.formInput}
            data-testid="backlog-empty-form-title"
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                handleCancel();
              }
            }}
          />
          {titleError && (
            <span role="alert" className={styles.validationError}>
              {titleError}
            </span>
          )}
        </div>
        <div>
          <label htmlFor="item-priority" className={styles.formLabel}>
            Priority
          </label>
          <select
            id="item-priority"
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
            className={styles.formSelect}
            data-testid="backlog-empty-form-priority"
          >
            <option value={1}>P1 — Critical</option>
            <option value={2}>P2 — High</option>
            <option value={3}>P3 — Medium</option>
            <option value={4}>P4 — Low</option>
            <option value={5}>P5 — Minimal</option>
          </select>
        </div>
        <div className={styles.formActions}>
          <button type="button" className={styles.cancelButton} onClick={handleCancel} data-testid="backlog-empty-form-cancel">
            Cancel
          </button>
          <button
            type="submit"
            className={`${styles.submitButton}${!title.trim() ? ` ${styles.submitButtonDisabled}` : ""}`}
            aria-disabled={!title.trim()}
            disabled={submitting}
            onClick={handleSubmit}
            data-testid="backlog-empty-form-submit"
          >
            {submitting ? "Creating…" : "Create Item"}
          </button>
        </div>
        {submitError && (
          <span role="alert" className={styles.validationError}>{submitError}</span>
        )}
      </div>
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
