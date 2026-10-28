type Listener = (text: string) => void;

const listeners = new Set<Listener>();

/** Speaks `text` through the single polite `RouteAnnouncer` live region; a no-op when none is mounted. */
export function announce(text: string): void {
  for (const listener of listeners) listener(text);
}

/** Used by `RouteAnnouncer` only. */
export function subscribeAnnouncements(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** `Window 2 of 3: api`, or `Window 2 of 3` when the window is unnamed. */
export function formatWindowAnnouncement(position: number, total: number, name: string): string {
  const base = `Window ${position} of ${total}`;
  const trimmed = name.trim();
  return trimmed ? `${base}: ${trimmed}` : base;
}
