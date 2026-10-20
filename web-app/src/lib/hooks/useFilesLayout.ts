"use client";

import { useLayoutEffect, useState, type RefObject } from "react";
import { chooseFilesLayout, type FilesLayout } from "@/lib/files/chooseFilesLayout";
import { useCoarsePointer } from "./useCoarsePointer";

const screenShortSide = () => (typeof window === "undefined" ? 0 : Math.min(window.screen?.width ?? 0, window.screen?.height ?? 0));
const initialWidth = () => (typeof window === "undefined" ? 1024 : window.innerWidth);

/** The Files tab's layout and the container width it was chosen from (`screen` is read once; a phone's never changes). */
export function useFilesLayout(containerRef: RefObject<HTMLElement | null>): { layout: FilesLayout; containerWidth: number } {
  const pointerCoarse = useCoarsePointer();
  const [containerWidth, setContainerWidth] = useState(initialWidth);
  const [short] = useState(screenShortSide);

  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    // A zero width means "not laid out" (jsdom, display:none): keep the last answer.
    const measure = (w: number) => { if (w > 0) setContainerWidth(w); };
    measure(el.getBoundingClientRect().width);
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver((entries) => measure(entries[entries.length - 1].contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, [containerRef]);

  return { layout: chooseFilesLayout({ containerWidth, pointerCoarse, screenShortSide: short }), containerWidth };
}
