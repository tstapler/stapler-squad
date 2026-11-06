// URL grammar for opening a file: `?file=<path>&line=<n>` (shareable) and `#file=<path>` (the
// selection mirror). Pure; useFilesDeepLink is the only code that writes it to history.

export interface DeepLink { file: string; line?: number }

export function parseDeepLink(search: string, hash: string): DeepLink | null {
  const q = new URLSearchParams(search);
  const file = q.get("file") || new URLSearchParams(hash.replace(/^#/, "")).get("file");
  if (!file) return null;
  const line = Number(q.get("line"));
  return Number.isInteger(line) && line >= 1 ? { file, line } : { file };
}

export const fileHash = (path: string | null): string => (path ? `#file=${encodeURIComponent(path)}` : "");

/** Clamps a requested line into 1..lineCount; `clamped` feeds `files.nav.jump_line`. */
export function clampLine(line: number, lineCount: number): { line: number; clamped: boolean } {
  const target = Math.min(Math.max(1, Math.trunc(line)), Math.max(1, lineCount));
  return { line: target, clamped: target !== line };
}
