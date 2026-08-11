import { useState, useCallback } from "react";

export function useSelection<T extends { id: string }>(items: T[]) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [selectMode, setSelectMode] = useState(false);

  const toggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    setSelected(new Set(items.map((i) => i.id)));
  }, [items]);

  const clearSelection = useCallback(() => {
    setSelected(new Set());
    setSelectMode(false);
  }, []);

  const isSelected = useCallback(
    (id: string) => {
      return selected.has(id);
    },
    [selected]
  );

  const toggleSelectMode = useCallback(() => {
    setSelectMode((prev) => !prev);
    if (selectMode) {
      clearSelection();
    }
  }, [selectMode, clearSelection]);

  return {
    selected,
    selectMode,
    setSelectMode,
    toggle,
    selectAll,
    clearSelection,
    isSelected,
    toggleSelectMode,
    selectedCount: selected.size,
  };
}
