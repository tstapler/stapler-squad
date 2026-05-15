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
  const entries = await Promise.all(
    DOC_FILES.map(async (slug) => {
      const res = await fetch(`/docs/${slug}.md`);
      const content = await res.text();
      const titleMatch = content.match(/^#\s+(.+)$/m);
      return {
        slug,
        title: titleMatch?.[1] ?? slug,
        content,
      };
    })
  );
  return entries;
}

export function buildFuseIndex(docs: DocEntry[]): Fuse<DocEntry> {
  return new Fuse(docs, {
    keys: ["title", "content"],
    threshold: 0.4,
    includeScore: true,
  });
}
