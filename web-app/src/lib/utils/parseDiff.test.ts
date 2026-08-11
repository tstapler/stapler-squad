import { parseDiff, buildGutterMarks } from "./parseDiff";

describe("parseDiff", () => {
  it("parses a single-file diff with one hunk", () => {
    const diff = `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,2 @@
-old
+new
 context
`;
    const files = parseDiff(diff);
    expect(files).toHaveLength(1);
    expect(files[0].filename).toBe("a.txt");
    expect(files[0].additions).toBe(1);
    expect(files[0].deletions).toBe(1);
  });

  it("returns an empty array for blank content", () => {
    expect(parseDiff("")).toEqual([]);
    expect(parseDiff("   \n")).toEqual([]);
  });
});

describe("buildGutterMarks", () => {
  it("returns an empty map when the file has no diff", () => {
    const diff = `diff --git a/other.txt b/other.txt
--- a/other.txt
+++ b/other.txt
@@ -1,1 +1,1 @@
-old
+new
`;
    expect(buildGutterMarks(diff, "a.txt")).toEqual(new Map());
  });

  it("returns an empty map for blank diff content (untouched file)", () => {
    expect(buildGutterMarks("", "a.txt")).toEqual(new Map());
  });

  it("marks an added line as 'modify' when its hunk also deletes a line", () => {
    const diff = `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,2 @@
-old line
+new line
 context
`;
    const marks = buildGutterMarks(diff, "a.txt");
    expect(marks.get(1)).toBe("modify");
    expect(marks.size).toBe(1);
  });

  it("marks an added line as 'add' when its hunk has no deletions", () => {
    const diff = `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,1 +1,2 @@
 context
+added line
`;
    const marks = buildGutterMarks(diff, "a.txt");
    expect(marks.get(2)).toBe("add");
    expect(marks.size).toBe(1);
  });

  it("returns an empty map for a binary-file diff (no @@ hunks, just a 'Binary files differ' marker)", () => {
    const diff = `diff --git a/image.png b/image.png
index 111..222 100644
Binary files a/image.png and b/image.png differ
`;
    expect(() => buildGutterMarks(diff, "image.png")).not.toThrow();
    expect(buildGutterMarks(diff, "image.png")).toEqual(new Map());
  });

  it("marks the line after a pure-deletion hunk as 'delete'", () => {
    const diff = `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,0 @@
-removed one
-removed two
`;
    const marks = buildGutterMarks(diff, "a.txt");
    // Pure deletion at the start of the file — clamped to line 1.
    expect(marks.get(1)).toBe("delete");
    expect(marks.size).toBe(1);
  });
});
