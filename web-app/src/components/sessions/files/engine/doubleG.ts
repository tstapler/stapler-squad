// Vim-style "gg" needs two "g" presses in a short window; a lone "g" is swallowed.
// Kept CodeMirror-free so FileContentViewer can re-export it without loading the editor chunk.
export const DOUBLE_G_WINDOW_MS = 400;

export function isDoubleGPress(lastGPressAt: number, now: number, windowMs: number = DOUBLE_G_WINDOW_MS): boolean {
  return now - lastGPressAt < windowMs;
}
