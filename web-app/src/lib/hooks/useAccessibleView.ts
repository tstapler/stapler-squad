"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import type { LabelsOf } from "@/lib/files/analytics/events";

const STORAGE_KEY = "files.accessibleView";

type Via = LabelsOf<"files.a11y.accessible_view">["via"];
type Size = LabelsOf<"files.a11y.accessible_view">["size"];

const sizeBucket = (chars: number): Size => (chars < 10_000 ? "lt10k" : chars < 100_000 ? "10k-100k" : chars < 1_000_000 ? "100k-1m" : "gt1m");

function read(): boolean {
  try { return localStorage.getItem(STORAGE_KEY) === "1"; } catch { return false; }
}
function write(on: boolean) {
  try { localStorage.setItem(STORAGE_KEY, on ? "1" : "0"); } catch { /* private mode: the choice lasts for this page only */ }
}

export interface AccessibleViewState {
  on: boolean;
  toggle: (via: Via) => void;
}

/** The per-device choice between CodeMirror and the Accessible view (ux.md 8.10). */
export function useAccessibleView({ textLength, swapView }: { textLength: () => number; swapView: () => void }): AccessibleViewState {
  const analytics = useFilesAnalytics({ component: "FileContentViewer" });
  const [on, setOn] = useState(false);
  // Read after mount so server and client render the same first frame.
  useEffect(() => setOn(read()), []);

  const toggle = useCallback((via: Via) => {
    const to = !on;
    swapView();
    analytics.emit("files.a11y.accessible_view", { to: to ? "on" : "off", via, size: sizeBucket(textLength()) });
    write(to);
    setOn(to);
  }, [on, swapView, analytics, textLength]);

  return useMemo(() => ({ on, toggle }), [on, toggle]);
}
