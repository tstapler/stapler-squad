/**
 * With text selected, Ctrl/Cmd+C (and Ctrl+Shift+C) must copy rather than send ^C to the app,
 * which would interrupt a running agent. XtermTerminal does this in its key handler; ghostty-web
 * does not (it only lets Meta+C through, and its mouse-up copy leaves Ctrl+C untouched).
 * Without a selection the key falls through, so Ctrl+C still interrupts.
 */

export interface CopyKeyDeps {
  getSelection(): string;
  clearSelection(): void;
  copy(text: string): Promise<boolean>;
  onCopied?(ok: boolean): void;
}

type KeyLike = Pick<KeyboardEvent, "type" | "key" | "ctrlKey" | "metaKey" | "altKey" | "preventDefault">;

/** Handler for ghostty-web's `attachCustomKeyEventHandler`: returns true when the key was consumed. */
export function createCopyKeyHandler(deps: CopyKeyDeps): (event: KeyLike) => boolean {
  return (event) => {
    const isCopy = event.type === "keydown" && (event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === "c";
    if (!isCopy) return false;
    const text = deps.getSelection();
    if (!text) return false;
    event.preventDefault();
    deps.clearSelection();
    void deps.copy(text).then((ok) => deps.onCopied?.(ok));
    return true;
  };
}
