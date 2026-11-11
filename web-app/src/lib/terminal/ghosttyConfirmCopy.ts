/**
 * ghostty-web copies every selection to the clipboard the moment the mouse is released (and on
 * double-click), which silently overwrote what the user had copied. xterm never did: selecting
 * only highlights, and copying takes a Copy button, the menu, or Ctrl/Cmd+C.
 *
 * `selectionManager` and its `copyToClipboard` are private in ghostty-web's typings; the narrow
 * cast lives here only, and a version bump that renames them degrades to "still auto-copies".
 */

interface AutoCopyingTerminal {
  selectionManager?: {
    copyToClipboard?: (text: string) => Promise<void> | void;
    hasSelection?: () => boolean;
    clearSelection?: () => void;
    selectionChangedEmitter?: { fire?: () => void };
  };
}

/** Stops ghostty-web's copy-on-select. The selection (and its change event) are unaffected. */
export function disableGhosttyAutoCopy(term: object): boolean {
  const manager = (term as AutoCopyingTerminal).selectionManager;
  if (!manager || typeof manager.copyToClipboard !== "function") return false;
  manager.copyToClipboard = async () => {};
  return true;
}

/**
 * ghostty-web's `clearSelection()` (mouse-down, click outside, resize, `term.clearSelection()`)
 * never fires `onSelectionChange`; only `deselect()` does. Anything keyed off that event, like
 * the floating Copy button, therefore stays up after the selection is gone. Fire it when a
 * clear actually removed a selection. Idempotent; false when the private API is gone.
 */
export function notifyGhosttySelectionCleared(term: object): boolean {
  const manager = (term as AutoCopyingTerminal).selectionManager;
  const clear = manager?.clearSelection;
  if (!manager || typeof clear !== "function" || typeof manager.hasSelection !== "function") return false;
  if ((clear as { notifies?: boolean }).notifies) return true;
  const patched = () => {
    const had = manager.hasSelection?.() ?? false;
    clear.call(manager);
    if (had) manager.selectionChangedEmitter?.fire?.();
  };
  (patched as { notifies?: boolean }).notifies = true;
  manager.clearSelection = patched;
  return true;
}
