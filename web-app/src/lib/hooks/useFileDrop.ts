import { useCallback, useRef, useState } from "react";
import type React from "react";

const isFileDrag = (enabled: boolean, e: React.DragEvent) =>
  enabled && e.dataTransfer.types.includes("Files");

/**
 * Drag-and-drop of OS files onto an element. Ignores non-file drags (e.g. pane reordering)
 * and does nothing when `enabled` is false. dragCounterRef balances enter/leave events
 * fired by child elements so the overlay doesn't flicker.
 */
// kibitzer:ignore flag-argument -- `enabled` gates the whole hook (read-only/touch), not a mode switch
export function useFileDrop(enabled: boolean, onFiles: (files: File[]) => void) {
  const [isDragOver, setIsDragOver] = useState(false);
  const dragCounterRef = useRef(0);

  const onDragEnter = useCallback((e: React.DragEvent) => {
    if (!isFileDrag(enabled, e)) return;
    e.preventDefault();
    dragCounterRef.current++;
    if (dragCounterRef.current === 1) setIsDragOver(true);
  }, [enabled]);

  const onDragOver = useCallback((e: React.DragEvent) => {
    if (!isFileDrag(enabled, e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  }, [enabled]);

  const onDragLeave = useCallback((e: React.DragEvent) => {
    if (!isFileDrag(enabled, e)) return;
    e.preventDefault();
    dragCounterRef.current = Math.max(0, dragCounterRef.current - 1);
    if (dragCounterRef.current === 0) setIsDragOver(false);
  }, [enabled]);

  const onDrop = useCallback((e: React.DragEvent) => {
    dragCounterRef.current = 0;
    setIsDragOver(false);
    if (!isFileDrag(enabled, e)) return;
    e.preventDefault();
    const files = Array.from(e.dataTransfer.files);
    if (files.length > 0) onFiles(files);
  }, [enabled, onFiles]);

  return { isDragOver: isDragOver && enabled, dropHandlers: { onDragEnter, onDragOver, onDragLeave, onDrop } };
}
