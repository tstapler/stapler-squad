"use client";

import { useState, useRef, useCallback, useEffect } from "react";

export interface ResizablePanelOptions {
  storageKey: string;
  defaultWidth: number;
  minWidth: number;
  maxWidthFraction: number;
}

export interface ResizablePanelResult {
  width: number;
  collapsed: boolean;
  containerRef: React.RefObject<HTMLDivElement | null>;
  handleProps: {
    onPointerDown: (e: React.PointerEvent) => void;
    onPointerMove: (e: React.PointerEvent) => void;
    onPointerUp: (e: React.PointerEvent) => void;
    onPointerCancel: (e: React.PointerEvent) => void;
  };
  collapse: () => void;
  expand: () => void;
}

function readStoredNumber(key: string, fallback: number, min: number): number {
  if (typeof localStorage === "undefined") return fallback;
  try {
    const v = localStorage.getItem(key);
    if (v !== null) {
      const n = Number(v);
      if (!isNaN(n)) return Math.max(min, n);
    }
  } catch {
    // ignore
  }
  return fallback;
}

function readStoredBool(key: string, fallback: boolean): boolean {
  if (typeof localStorage === "undefined") return fallback;
  try {
    const v = localStorage.getItem(key);
    if (v !== null) return v === "true";
  } catch {
    // ignore
  }
  return fallback;
}

export function useResizablePanel({
  storageKey,
  defaultWidth,
  minWidth,
  maxWidthFraction,
}: ResizablePanelOptions): ResizablePanelResult {
  const collapsedKey = storageKey + "Collapsed";

  const [width, setWidth] = useState<number>(() =>
    readStoredNumber(storageKey, defaultWidth, minWidth)
  );
  const [collapsed, setCollapsed] = useState<boolean>(() =>
    readStoredBool(collapsedKey, false)
  );

  const containerRef = useRef<HTMLDivElement>(null);
  const isDraggingRef = useRef(false);
  const lastWidthRef = useRef<number>(readStoredNumber(storageKey, defaultWidth, minWidth));
  const rafRef = useRef<number | null>(null);
  const pendingWidthRef = useRef<number | null>(null);

  // Persist width changes to localStorage
  useEffect(() => {
    try {
      localStorage.setItem(storageKey, String(width));
    } catch {
      // ignore
    }
  }, [storageKey, width]);

  // Persist collapsed changes to localStorage
  useEffect(() => {
    try {
      localStorage.setItem(collapsedKey, String(collapsed));
    } catch {
      // ignore
    }
  }, [collapsedKey, collapsed]);

  // Clamp width against the actual container size after mount — guards against
  // a stored value that exceeds maxWidthFraction on a narrower viewport.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const maxWidth = container.clientWidth * maxWidthFraction;
    if (maxWidth > 0 && width > maxWidth) {
      setWidth(maxWidth);
    }
  }, [maxWidthFraction]); // intentionally omits width — runs once on mount per fraction change

  const onPointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    isDraggingRef.current = true;
  }, []);

  const onPointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!isDraggingRef.current) return;
      const container = containerRef.current;
      if (!container) return;

      const rect = container.getBoundingClientRect();
      const rawWidth = e.clientX - rect.left;
      const maxWidth = container.clientWidth * maxWidthFraction;
      const clamped = Math.max(minWidth, Math.min(maxWidth, rawWidth));

      pendingWidthRef.current = clamped;

      if (rafRef.current === null) {
        rafRef.current = requestAnimationFrame(() => {
          if (pendingWidthRef.current !== null) {
            setWidth(pendingWidthRef.current);
            pendingWidthRef.current = null;
          }
          rafRef.current = null;
        });
      }
    },
    [minWidth, maxWidthFraction]
  );

  const onPointerUp = useCallback((e: React.PointerEvent) => {
    isDraggingRef.current = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
  }, []);

  const collapse = useCallback(() => {
    lastWidthRef.current = width;
    setCollapsed(true);
  }, [width]);

  const expand = useCallback(() => {
    setCollapsed(false);
    setWidth(lastWidthRef.current || defaultWidth);
  }, [defaultWidth]);

  return {
    width,
    collapsed,
    containerRef,
    handleProps: {
      onPointerDown,
      onPointerMove,
      onPointerUp,
      onPointerCancel: onPointerUp,
    },
    collapse,
    expand,
  };
}
