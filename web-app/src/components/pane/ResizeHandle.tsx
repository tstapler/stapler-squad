"use client";

import { useRef } from "react";
import type { PaneId, SplitDirection } from "@/lib/pane/paneTypes";
import { resizeHandle } from "@/styles/pane/resizeHandle.css";

const MIN_PX = 200;

function clampRatio(rawRatio: number, minPx: number, totalPx: number): number {
  if (totalPx <= 0) return rawRatio;
  const minRatio = minPx / totalPx;
  return Math.max(minRatio, Math.min(1 - minRatio, rawRatio));
}

interface ResizeHandleProps {
  splitId: PaneId;
  direction: SplitDirection;
  onResize: (splitId: PaneId, ratio: number) => void;
}

export function ResizeHandle({ splitId, direction, onResize }: ResizeHandleProps) {
  const handleRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);
  const rafRef = useRef<number | null>(null);
  const pendingRatioRef = useRef<number | null>(null);

  function handlePointerDown(e: React.PointerEvent<HTMLDivElement>) {
    e.preventDefault();
    (e.currentTarget as HTMLDivElement).setPointerCapture(e.pointerId);
    draggingRef.current = true;
  }

  function handlePointerMove(e: React.PointerEvent<HTMLDivElement>) {
    if (!draggingRef.current) return;
    const container = handleRef.current?.parentElement;
    if (!container) return;

    const rect = container.getBoundingClientRect();
    const rawRatio = direction === "vertical"
      ? (e.clientX - rect.left) / rect.width
      : (e.clientY - rect.top) / rect.height;

    const totalPx = direction === "vertical" ? rect.width : rect.height;
    pendingRatioRef.current = clampRatio(rawRatio, MIN_PX, totalPx);

    if (rafRef.current === null) {
      rafRef.current = requestAnimationFrame(() => {
        if (pendingRatioRef.current !== null) {
          onResize(splitId, pendingRatioRef.current);
          pendingRatioRef.current = null;
        }
        rafRef.current = null;
      });
    }
  }

  function handlePointerUp(e: React.PointerEvent<HTMLDivElement>) {
    draggingRef.current = false;
    (e.currentTarget as HTMLDivElement).releasePointerCapture(e.pointerId);
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
  }

  return (
    <div
      ref={handleRef}
      data-testid="resize-handle"
      className={resizeHandle({ direction })}
      style={{ touchAction: "none" }}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
    />
  );
}
