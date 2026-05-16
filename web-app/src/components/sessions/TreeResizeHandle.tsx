"use client";

import { hitTarget } from "./TreeResizeHandle.css";

interface TreeResizeHandleProps {
  onPointerDown: (e: React.PointerEvent) => void;
  onPointerMove: (e: React.PointerEvent) => void;
  onPointerUp: (e: React.PointerEvent) => void;
  onPointerCancel: (e: React.PointerEvent) => void;
}

export function TreeResizeHandle({
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
}: TreeResizeHandleProps) {
  return (
    <div
      className={hitTarget}
      style={{ touchAction: "none" }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerCancel}
    />
  );
}
