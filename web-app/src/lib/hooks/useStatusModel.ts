"use client";

import { useEffect, useState } from "react";
import type { useEditorBridge } from "./useEditorBridge";

type Bridge = ReturnType<typeof useEditorBridge>;
const CURSOR_POLL_MS = 250;

export interface StatusModel { line: number; col: number; eol: "LF" | "CRLF" | "mixed" | null }

/** Cursor position and line endings for the status bar; the engine port has no change event, so it polls cheaply. */
export function useStatusModel({ inspect, eol }: Pick<Bridge, "inspect" | "eol">): StatusModel {
  const [model, setModel] = useState<StatusModel>({ line: 1, col: 1, eol: null });
  useEffect(() => {
    const read = () => {
      const at = inspect();
      if (!at) return;
      setModel((m) => (m.line === at.line && m.col === at.col + 1 ? m : { ...m, line: at.line, col: at.col + 1 }));
    };
    setModel((m) => ({ ...m, eol: eol() }));
    read();
    const id = setInterval(read, CURSOR_POLL_MS);
    return () => clearInterval(id);
  }, [inspect, eol]);
  return model;
}
