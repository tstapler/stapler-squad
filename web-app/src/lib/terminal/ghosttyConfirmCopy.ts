/**
 * ghostty-web copies every selection to the clipboard the moment the mouse is released (and on
 * double-click), which silently overwrote what the user had copied. xterm never did: selecting
 * only highlights, and copying takes a Copy button, the menu, or Ctrl/Cmd+C.
 *
 * `selectionManager` and its `copyToClipboard` are private in ghostty-web's typings; the narrow
 * cast lives here only, and a version bump that renames them degrades to "still auto-copies".
 */

interface AutoCopyingTerminal {
  selectionManager?: { copyToClipboard?: (text: string) => Promise<void> | void };
}

/** Stops ghostty-web's copy-on-select. The selection (and its change event) are unaffected. */
export function disableGhosttyAutoCopy(term: object): boolean {
  const manager = (term as AutoCopyingTerminal).selectionManager;
  if (!manager || typeof manager.copyToClipboard !== "function") return false;
  manager.copyToClipboard = async () => {};
  return true;
}
