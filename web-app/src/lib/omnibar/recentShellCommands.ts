const STORAGE_KEY = "ssq.recentShellCommands";
const MAX_ENTRIES = 8;

export function getRecentShellCommands(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === "string") : [];
  } catch {
    return [];
  }
}

export function addRecentShellCommand(command: string): void {
  if (typeof window === "undefined") return;
  const trimmed = command.trim();
  if (!trimmed) return;
  try {
    const existing = getRecentShellCommands().filter((c) => c !== trimmed);
    const updated = [trimmed, ...existing].slice(0, MAX_ENTRIES);
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
  } catch {
    // localStorage unavailable (e.g. private browsing quota) — history is best-effort
  }
}
