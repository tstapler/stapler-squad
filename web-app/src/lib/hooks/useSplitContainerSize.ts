"use client";

import { useState, useEffect, RefObject } from "react";

interface ContainerSize {
  width: number;
  height: number;
}

/**
 * useSplitContainerSize — tracks the rendered size of a container element
 * via ResizeObserver. Used by the pane keyboard-nudge resize to compute
 * `containerSizePx` for the NUDGE_RESIZE action.
 */
export function useSplitContainerSize(
  ref: RefObject<HTMLElement | null>
): ContainerSize {
  const [size, setSize] = useState<ContainerSize>({ width: 0, height: 0 });

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const ro = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      setSize({ width, height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);

  return size;
}
