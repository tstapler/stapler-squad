// Keeps docLoader's DOC_FILES, the files in public/docs/, and every helpSlug used in src/ in sync.
import fs from "fs";
import path from "path";
import { DOC_FILES } from "@/lib/docs/docLoader";

const DOCS_DIR = path.join(process.cwd(), "public/docs");
const SRC_DIR = path.join(process.cwd(), "src");

const docFileSlugs = fs
  .readdirSync(DOCS_DIR)
  .filter((f) => f.endsWith(".md"))
  .map((f) => f.slice(0, -".md".length));

function sourceFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) return e.name === "__tests__" ? [] : sourceFiles(full);
    return /\.tsx?$/.test(e.name) && !/\.(test|stories)\.tsx?$/.test(e.name) ? [full] : [];
  });
}

describe("help docs coverage", () => {
  it("has a file in public/docs/ for every DOC_FILES entry", () => {
    expect(DOC_FILES.filter((slug) => !docFileSlugs.includes(slug))).toEqual([]);
  });

  it("lists every file in public/docs/ in DOC_FILES", () => {
    expect(docFileSlugs.filter((slug) => !DOC_FILES.includes(slug))).toEqual([]);
  });

  it.each(DOC_FILES)("%s.md is non-trivial and starts with an H1", (slug) => {
    const content = fs.readFileSync(path.join(DOCS_DIR, `${slug}.md`), "utf8");
    expect(Buffer.byteLength(content)).toBeGreaterThan(50);
    expect(content).toMatch(/^#\s+\S/m);
  });

  it("only references helpSlug values that exist in DOC_FILES", () => {
    const unknown: string[] = [];
    for (const file of sourceFiles(SRC_DIR)) {
      for (const m of fs.readFileSync(file, "utf8").matchAll(/helpSlug=(?:"([^"]+)"|\{"([^"]+)"\})/g)) {
        const slug = m[1] ?? m[2];
        if (!DOC_FILES.includes(slug)) unknown.push(`${path.relative(SRC_DIR, file)}: ${slug}`);
      }
    }
    expect(unknown).toEqual([]);
  });
});
