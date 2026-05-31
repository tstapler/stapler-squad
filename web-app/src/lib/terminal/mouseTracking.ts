import type { Terminal } from "@xterm/xterm";

export function isMouseTracking(terminal: Terminal): boolean {
  return (
    (terminal.modes as any)?.mouseTrackingMode !== "none" &&
    (terminal.modes as any)?.mouseTrackingMode !== undefined
  );
}
