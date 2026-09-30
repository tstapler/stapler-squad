"use client";

import { useRef, useEffect, useCallback, useState } from "react";
import { createPortal } from "react-dom";
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
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownPos, setDropdownPos] = useState({ top: 0, right: 0 });

  // Close on click outside — checks both the trigger wrapper and the portaled dropdown,
  // since the dropdown no longer lives inside wrapperRef's DOM subtree.
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (wrapperRef.current?.contains(target) || dropdownRef.current?.contains(target)) {
        return;
      }
      onOpenChange(false);
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
        onClick={() => {
          // Compute the dropdown's position synchronously, before it opens, so the
          // portaled dropdown never paints at the stale {top: 0, right: 0} default
          // for a frame (matches MoveToMenu.tsx's openMenu pattern).
          if (!open) {
            const rect = wrapperRef.current?.getBoundingClientRect();
            if (rect) {
              setDropdownPos({ top: rect.bottom + 4, right: window.innerWidth - rect.right });
            }
          }
          onOpenChange(!open);
        }}
        aria-haspopup="listbox"
        aria-expanded={open}
        title="Customize visible columns"
      >
        ⊞ Columns
      </button>

      {open &&
        createPortal(
          <div
            ref={dropdownRef}
            className={dropdown}
            role="listbox"
            aria-label="Visible columns"
            style={{ top: dropdownPos.top, right: dropdownPos.right }}
          >
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
          </div>,
          document.body
        )}
    </div>
  );
}
