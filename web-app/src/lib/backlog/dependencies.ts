import type { ItemDependency } from "@/lib/hooks/useBacklogService";

// Mirrors the server's resolved definition (session.TerminalStatusStrings):
// a blocker is resolved once done or archived; deletion removes the edge.
const RESOLVED_STATUSES = new Set(["done", "archived"]);

export function isBlockerUnresolved(dep: Pick<ItemDependency, "status">): boolean {
  return !RESOLVED_STATUSES.has(dep.status);
}

export function unresolvedBlockers(deps: ItemDependency[] | undefined): ItemDependency[] {
  return (deps ?? []).filter(isBlockerUnresolved);
}

/** "blocked by <title>" / "blocked by <title> +N", or undefined when nothing blocks the item. */
export function blockedByLabel(deps: ItemDependency[] | undefined): string | undefined {
  const open = unresolvedBlockers(deps);
  if (open.length === 0) return undefined;
  const first = open[0].title || open[0].publicId || open[0].itemId;
  return open.length === 1 ? `blocked by ${first}` : `blocked by ${first} +${open.length - 1}`;
}
