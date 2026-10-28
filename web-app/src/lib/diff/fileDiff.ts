import { parseDiff } from "@/lib/utils/parseDiff";

/** The file path a diff section belongs to: new path from `+++ b/`, else old path from `--- a/` (deleted files). */
function sectionPath(section: string): string | null {
  const added = section.match(/^\+\+\+ b\/(.*)$/m);
  if (added) return added[1];
  const removed = section.match(/^--- a\/(.*)$/m);
  return removed ? removed[1] : null;
}

/** One file's section of a multi-file unified diff, or null when the diff doesn't touch `path`. */
export function extractFileDiff(content: string, path: string): { content: string; added: number; removed: number } | null {
  if (!content) return null;
  for (const section of content.split(/^(?=diff --git )/m)) {
    if (sectionPath(section) !== path) continue;
    const file = parseDiff(section)[0];
    return { content: section, added: file?.additions ?? 0, removed: file?.deletions ?? 0 };
  }
  return null;
}
