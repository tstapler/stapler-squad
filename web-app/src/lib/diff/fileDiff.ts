import { parseDiff } from "@/lib/utils/parseDiff";

/** One file's section of a multi-file unified diff, or null when the diff doesn't touch `path`. */
export function extractFileDiff(content: string, path: string): { content: string; added: number; removed: number } | null {
  if (!content) return null;
  const sections = content.split(/^(?=diff --git )/m);
  for (const section of sections) {
    const header = section.match(/^diff --git a\/(.*?) b\/(.*)$/m);
    if (!header || header[2] !== path) continue;
    const file = parseDiff(section)[0];
    return { content: section, added: file?.additions ?? 0, removed: file?.deletions ?? 0 };
  }
  return null;
}
