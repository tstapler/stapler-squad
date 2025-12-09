"use client";

import styles from './FilterPill.module.css';

interface FilterPillProps {
  label: string;
  value: string;
  onRemove: () => void;
  color?: string;
  className?: string;
}

export function FilterPill({ label, value, onRemove, color, className }: FilterPillProps) {
  return (
    <div
      className={`${styles.pill} ${className || ''}`}
      style={color ? { borderColor: color } : undefined}
    >
      <span className={styles.label}>{label}:</span>
      <span className={styles.value} style={color ? { color } : undefined}>
        {value}
      </span>
      <button
        className={styles.removeButton}
        onClick={onRemove}
        aria-label={`Remove ${label}: ${value} filter`}
        type="button"
      >
        ×
      </button>
    </div>
  );
}

interface FilterPillsProps {
  children: React.ReactNode;
  onClearAll?: () => void;
  className?: string;
}

export function FilterPills({ children, onClearAll, className }: FilterPillsProps) {
  const hasChildren = React.Children.count(children) > 0;

  if (!hasChildren) {
    return null;
  }

  return (
    <div className={`${styles.container} ${className || ''}`}>
      {children}
      {onClearAll && (
        <button
          className={styles.clearAllButton}
          onClick={onClearAll}
          type="button"
          aria-label="Clear all filters"
        >
          Clear all
        </button>
      )}
    </div>
  );
}

// Re-export for convenience
import React from 'react';
export default FilterPill;
