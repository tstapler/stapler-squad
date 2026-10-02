"use client";
// +feature: terminal-jump-to-latest

import { useCallback, useEffect, useState } from "react";
import type { NetPagesUpTracker } from "@/lib/terminal/scrollPosition";
import type { ScrollTarget } from "@/lib/terminal/scrollRouting";
import { JumpToLatestButton, type JumpTerminal, type ViewRect } from "./JumpToLatestButton";

export interface JumpToLatestMountProps {
  terminal: JumpTerminal | null;
  route: ScrollTarget;
  netPagesUp: NetPagesUpTracker;
  connectionEpoch: number;
  sendData: (data: string) => void;
  gestureActive: boolean;
  /** The element the button is positioned inside; its rect locates the cursor row. */
  getContainer: () => HTMLElement | null;
}

/**
 * Owns the per-frame output tick so streaming output re-renders only this subtree,
 * not the whole TerminalOutput.
 */
export function JumpToLatestMount({ terminal, getContainer, ...rest }: JumpToLatestMountProps) {
  const [outputTick, setOutputTick] = useState(0);

  useEffect(() => {
    if (!terminal?.onWriteParsed) return;
    let frame = 0;
    const subscription = terminal.onWriteParsed(() => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        setOutputTick((t) => t + 1);
      });
    });
    return () => {
      subscription.dispose();
      if (frame) cancelAnimationFrame(frame);
    };
  }, [terminal]);

  const getCursorRowRect = useCallback((): ViewRect | null => {
    const container = getContainer();
    const cursorY = (terminal?.buffer.active as { cursorY?: number } | undefined)?.cursorY;
    if (!container || !terminal || cursorY === undefined || terminal.rows <= 0) return null;
    const rect = container.getBoundingClientRect();
    const cellHeight = rect.height / terminal.rows;
    const top = rect.top + cursorY * cellHeight;
    return { left: rect.left, right: rect.right, top, bottom: top + cellHeight };
  }, [terminal, getContainer]);

  return <JumpToLatestButton terminal={terminal} outputTick={outputTick} getCursorRowRect={getCursorRowRect} {...rest} />;
}
