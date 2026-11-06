import type { Terminal } from "@xterm/xterm";
import type { MouseTrackingMode, ScrollMode } from "./scrollRouting";

// xterm's public typings omit `modes.mouseTrackingMode`; type the access once here.
type TerminalModesWithMouse = { mouseTrackingMode?: string };

function readMouseTrackingMode(terminal: Terminal): string | undefined {
  return (terminal.modes as TerminalModesWithMouse | undefined)?.mouseTrackingMode;
}

export function isMouseTracking(terminal: Terminal): boolean {
  const mode = readMouseTrackingMode(terminal);
  return mode !== "none" && mode !== undefined;
}

const KNOWN_TRACKING_MODES: readonly string[] = ["none", "x10", "vt200", "drag", "any"];

/** Snapshot of the terminal's routing signals; re-read every frame. */
export function readScrollMode(terminal: Terminal): ScrollMode {
  const raw = readMouseTrackingMode(terminal);
  const mouseTrackingMode: MouseTrackingMode =
    raw !== undefined && KNOWN_TRACKING_MODES.includes(raw)
      ? (raw as MouseTrackingMode)
      : // unknown non-none values (e.g. a future xterm mode) still mean "tracking on"
        raw === undefined
        ? "none"
        : "any";
  const bufferType = terminal.buffer?.active?.type === "alternate" ? "alternate" : "normal";
  return { bufferType, mouseTrackingMode };
}
