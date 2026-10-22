import type { Buffer } from "@/lib/files/bufferReducer";

export const isDirty = (b: Buffer) => b.status === "loaded" && b.dirty;
export const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

/** Basename, with the parent directory added only when two open tabs would read the same. */
export function labelOf(path: string, all: readonly Buffer[]): string {
  const base = baseName(path);
  if (all.filter((b) => baseName(b.path) === base).length < 2) return base;
  const parts = path.split("/");
  return parts.length > 1 ? `${parts[parts.length - 2]}/${base}` : base;
}

/** State is in the name as words, never colour or italic alone (ux.md 5, 6). */
export function accessibleName(b: Buffer, all: readonly Buffer[]): string {
  const label = labelOf(b.path, all);
  if (isDirty(b)) return `${label}, unsaved`;
  return `${label}, ${b.pinned ? "pinned" : "preview"}`;
}
