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
  it("matches a deleted file by its old path and a path containing ' b/'", () => {
    const d = [
      "diff --git a/gone.ts b/gone.ts", "deleted file mode 100644", "--- a/gone.ts", "+++ /dev/null", "@@ -1,1 +0,0 @@", "-bye",
      "diff --git a/x b/y b/x b/y", "--- a/x b/y", "+++ b/x b/y", "@@ -1,1 +1,1 @@", "-1", "+2", "",
    ].join("\n");
    expect(extractFileDiff(d, "gone.ts")?.content).toContain("-bye");
    expect(extractFileDiff(d, "x b/y")?.content).toContain("+2");
  });
  it("matches a renamed file by its new path", () => {
    const renamed = [
      "diff --git a/old.ts b/new.ts", "similarity index 90%", "rename from old.ts", "rename to new.ts",
      "--- a/old.ts", "+++ b/new.ts", "@@ -1,1 +1,1 @@", "-x", "+y", "",
    ].join("\n");
    expect(extractFileDiff(renamed, "new.ts")).toMatchObject({ added: 1, removed: 1 });
    expect(extractFileDiff(renamed, "old.ts")).toBeNull();
  });
  it("extracts a deleted file's section", () => {
    const deleted = [
      "diff --git a/gone.ts b/gone.ts", "deleted file mode 100644", "--- a/gone.ts", "+++ /dev/null",
      "@@ -1,2 +0,0 @@", "-a", "-b", "",
    ].join("\n");
    const r = extractFileDiff(deleted, "gone.ts");
    expect(r?.content).toContain("deleted file mode");
    expect(r).toMatchObject({ added: 0, removed: 2 });
  });
});
