import { extractFileDiff } from "./fileDiff";

const diff = [
  "diff --git a/a.ts b/a.ts", "--- a/a.ts", "+++ b/a.ts", "@@ -1,1 +1,2 @@", " keep", "+new",
  "diff --git a/dir/b.go b/dir/b.go", "--- a/dir/b.go", "+++ b/dir/b.go", "@@ -1,1 +1,1 @@", "-old", "+fresh",
  "",
].join("\n");

describe("extractFileDiff", () => {
  it("returns only the requested file's section with its own counts", () => {
    const r = extractFileDiff(diff, "dir/b.go");
    expect(r?.content).toContain("+fresh");
    expect(r?.content).not.toContain("+new");
    expect(r).toMatchObject({ added: 1, removed: 1 });
  });
  it("returns null for a path not in the diff or an empty diff", () => {
    expect(extractFileDiff(diff, "missing.ts")).toBeNull();
    expect(extractFileDiff("", "a.ts")).toBeNull();
  });
});
