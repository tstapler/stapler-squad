import Fuse from "fuse.js";

export interface DocEntry {
  slug: string;
  title: string;
  content: string;
}

const DOC_FILES = [
  "what-is-stapler-squad",
  "session-types",
  "omnibar",
  "keyboard-shortcuts",
  "configuration",
  "tmux-integration",
];

export async function loadDocs(): Promise<DocEntry[]> {
  const results = await Promise.allSettled(
    DOC_FILES.map(async (slug) => {
      const res = await fetch(`/docs/${slug}.md`);
      if (!res.ok) throw new Error(`Failed to load ${slug}: ${res.status}`);
      const content = await res.text();
      const titleMatch = content.match(/^#\s+(.+)$/m);
      return {
        slug,
        title: titleMatch?.[1] ?? slug,
        content,
      };
    })
  );
  return results
    .filter(
      (r): r is PromiseFulfilledResult<DocEntry> => r.status === "fulfilled"
    )
    .map((r) => r.value);
}

export function buildFuseIndex(docs: DocEntry[]): Fuse<DocEntry> {
  return new Fuse(docs, {
    keys: ["title", "content"],
    threshold: 0.4,
    includeScore: true,
  });
}
