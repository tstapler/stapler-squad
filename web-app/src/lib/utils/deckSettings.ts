/**
 * Per-device deck timing settings. The choices live in localStorage (they are
 * about this device's screen and thumb, not the account), and every read falls
 * back to the default when storage is missing, blocked or holds junk.
 */
export const PINNED_COLLAPSE_MS = 8_000;
export const PINNED_COLLAPSE_CHOICES_MS = [8_000, 15_000, 30_000] as const;
export const PINNED_COLLAPSE_NEVER = "never";

export const UNDO_WINDOW_DEFAULT_MS = 8_000;
export const UNDO_WINDOW_CHOICES_MS = [5_000, 8_000, 15_000, 30_000] as const;

export const PINNED_COLLAPSE_STORAGE_KEY = "ssq.notifications.pinnedCollapse";
export const UNDO_WINDOW_STORAGE_KEY = "ssq.notifications.undoWindow";

function readRaw(key: string): string | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

/** Milliseconds before a pinned card collapses on a phone, or null for "Never". */
export function readPinnedCollapseMs(): number | null {
  const raw = readRaw(PINNED_COLLAPSE_STORAGE_KEY);
  if (raw === PINNED_COLLAPSE_NEVER) return null;
  const value = Number(raw);
  return (PINNED_COLLAPSE_CHOICES_MS as readonly number[]).includes(value) ? value : PINNED_COLLAPSE_MS;
}

export function readUndoWindowMs(): number {
  const value = Number(readRaw(UNDO_WINDOW_STORAGE_KEY));
  return (UNDO_WINDOW_CHOICES_MS as readonly number[]).includes(value) ? value : UNDO_WINDOW_DEFAULT_MS;
}

export const WHAT_CHANGED_STORAGE_KEY = "ssq.notifications.whatChangedSeen";

function writeRaw(key: string, value: string): void {
  try {
    if (typeof window !== "undefined") window.localStorage.setItem(key, value);
  } catch {
    // Storage blocked: the setting simply does not persist; reads keep returning the default.
  }
}

export function writeUndoWindowMs(ms: number): void {
  writeRaw(UNDO_WINDOW_STORAGE_KEY, String(ms));
}

/** Pass null for "Never". */
export function writePinnedCollapseMs(ms: number | null): void {
  writeRaw(PINNED_COLLAPSE_STORAGE_KEY, ms === null ? PINNED_COLLAPSE_NEVER : String(ms));
}

export function readWhatChangedSeen(): boolean {
  return readRaw(WHAT_CHANGED_STORAGE_KEY) === "true";
}

export function writeWhatChangedSeen(): void {
  writeRaw(WHAT_CHANGED_STORAGE_KEY, "true");
}
