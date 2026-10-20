"use client";

import type { ResizablePanelResult } from "@/lib/hooks/useResizablePanel";
import { hitTarget } from "./TreeResizeHandle.css";

/** Everything `useResizablePanel().handleProps` provides: pointer drag plus the keyboard separator contract. */
type TreeResizeHandleProps = ResizablePanelResult["handleProps"];

export function TreeResizeHandle(props: TreeResizeHandleProps) {
  return <div className={hitTarget} style={{ touchAction: "none" }} {...props} />;
}
