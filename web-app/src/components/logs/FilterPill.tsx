"use client";

import React from 'react';
import { container as containerClass, pill as pillClass, label as labelClass, value as valueClass, removeButton as removeButtonClass, clearAllButton as clearAllButtonClass } from './FilterPill.css';

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
      className={`${pillClass} ${className || ''}`}
      style={color ? { borderColor: color } : undefined}
    >
      <span className={labelClass}>{label}:</span>
      <span className={valueClass} style={color ? { color } : undefined}>
        {value}
      </span>
      <button
        className={removeButtonClass}
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
    <div className={`${containerClass} ${className || ''}`}>
      {children}
      {onClearAll && (
        <button
          className={clearAllButtonClass}
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

export default FilterPill;
