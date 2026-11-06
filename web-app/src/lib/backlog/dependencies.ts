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

/** Display name for a dependency: title, else public id, else item id. */
export function dependencyLabel(dep: Pick<ItemDependency, "title" | "publicId" | "itemId">): string {
  return dep.title || dep.publicId || dep.itemId;
}
