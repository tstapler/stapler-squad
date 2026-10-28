/**
 * Smallest terminal size the client sends to the server. Mirrors the server's
 * nudge guard (`cols < 2 || rows < 1`, Task 3.1.2c): a hidden (`display:none`)
 * container fits to 0x0 or 1x1, and that must never reach tmux.
 */
export const MIN_HANDSHAKE_COLS = 2;
export const MIN_HANDSHAKE_ROWS = 1;

export function isUsableTerminalSize(cols: number | undefined, rows: number | undefined): boolean {
  return (
    cols !== undefined &&
    rows !== undefined &&
    cols >= MIN_HANDSHAKE_COLS &&
    rows >= MIN_HANDSHAKE_ROWS
  );
}
