const PREFIX = "ssq:draft:";
const memory = new Map<string, string>();

/**
 * Unsent text (an answer, a Deny reason) kept per item id for the life of the tab.
 * `sessionStorage` first; a memory map backs it when storage throws (private mode, quota).
 */
export function readDraft(id: string): string {
  try {
    const stored = window.sessionStorage.getItem(PREFIX + id);
    if (stored !== null) return stored;
  } catch {
    // fall through to memory
  }
  return memory.get(id) ?? "";
}

export function writeDraft(id: string, text: string): void {
  if (text === "") {
    clearDraft(id);
    return;
  }
  memory.set(id, text);
  try {
    window.sessionStorage.setItem(PREFIX + id, text);
  } catch {
    // memory copy above is the fallback
  }
}

/** Call on send or Dismiss. */
export function clearDraft(id: string): void {
  memory.delete(id);
  try {
    window.sessionStorage.removeItem(PREFIX + id);
  } catch {
    // nothing stored to remove
  }
}
