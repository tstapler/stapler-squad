"use client";

import { useRef, useEffect, useCallback } from "react";
import { COLUMN_DEFS, ColumnKey } from "./session-columns";
import {
  wrapper,
  triggerButton,
  triggerButtonActive,
  dropdown,
  dropdownTitle,
  checkboxRow,
  checkbox,
} from "./ColumnPicker.css";

interface ColumnPickerProps {
  visibleColumns: ColumnKey[];
  onChange: (cols: ColumnKey[]) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ColumnPicker({
  visibleColumns,
  onChange,
  open,
  onOpenChange,
}: ColumnPickerProps) {
  const wrapperRef = useRef<HTMLDivElement>(null);

  // Close on click outside
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        onOpenChange(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open, onOpenChange]);

  const toggle = useCallback(
    (key: ColumnKey) => {
      const next = visibleColumns.includes(key)
        ? visibleColumns.filter((k) => k !== key)
        : [...visibleColumns, key];
      onChange(next);
    },
    [visibleColumns, onChange]
  );

  return (
    <div ref={wrapperRef} className={wrapper}>
      <button
        className={`${triggerButton} ${open ? triggerButtonActive : ""}`}
        onClick={() => onOpenChange(!open)}
        aria-haspopup="listbox"
        aria-expanded={open}
        title="Customize visible columns"
      >
        ⊞ Columns
      </button>

      {open && (
        <div className={dropdown} role="listbox" aria-label="Visible columns">
          <div className={dropdownTitle}>Show columns</div>
          {COLUMN_DEFS.map((col) => (
            <label key={col.key} className={checkboxRow}>
              <input
                type="checkbox"
                className={checkbox}
                checked={visibleColumns.includes(col.key)}
                onChange={() => toggle(col.key)}
              />
              {col.label}
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
