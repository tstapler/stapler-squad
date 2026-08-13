export interface DiffFile {
  filename: string;
  additions: number;
  deletions: number;
  changes: DiffHunk[];
}

export interface DiffHunk {
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
  lines: DiffLine[];
}

export interface DiffLine {
  type: "add" | "delete" | "context";
  content: string;
  oldLineNumber?: number;
  newLineNumber?: number;
}

/** Parse unified diff format from git into structured DiffFile objects. */
export function parseDiff(diffContent: string): DiffFile[] {
  if (!diffContent?.trim()) return [];

  const files: DiffFile[] = [];
  const lineList = diffContent.split("\n");
  let currentFile: DiffFile | null = null;
  let currentHunk: DiffHunk | null = null;
  let oldLineNum = 0;
  let newLineNum = 0;

  for (const l of lineList) {
    if (l.startsWith("diff --git")) {
      if (currentFile && currentHunk) currentFile.changes.push(currentHunk);
      if (currentFile) files.push(currentFile);
      currentFile = { filename: "", additions: 0, deletions: 0, changes: [] };
      currentHunk = null;
    } else if (l.startsWith("+++")) {
      const match = l.match(/\+\+\+ b\/(.*)/);
      if (match && currentFile) currentFile.filename = match[1];
    } else if (l.startsWith("@@")) {
      if (currentFile && currentHunk) currentFile.changes.push(currentHunk);
      const match = l.match(/@@ -(\d+),(\d+) \+(\d+),(\d+) @@/);
      if (match) {
        currentHunk = {
          oldStart: parseInt(match[1]),
          oldLines: parseInt(match[2]),
          newStart: parseInt(match[3]),
          newLines: parseInt(match[4]),
          lines: [],
        };
        oldLineNum = parseInt(match[1]);
        newLineNum = parseInt(match[3]);
      }
    } else if (currentHunk && (l.startsWith("+") || l.startsWith("-") || l.startsWith(" "))) {
      if (l.startsWith("+")) {
        currentHunk.lines.push({ type: "add", content: l, newLineNumber: newLineNum++ });
        if (currentFile) currentFile.additions++;
      } else if (l.startsWith("-")) {
        currentHunk.lines.push({ type: "delete", content: l, oldLineNumber: oldLineNum++ });
        if (currentFile) currentFile.deletions++;
      } else {
        currentHunk.lines.push({
          type: "context",
          content: l,
          oldLineNumber: oldLineNum++,
          newLineNumber: newLineNum++,
        });
      }
    }
  }

  if (currentFile && currentHunk) currentFile.changes.push(currentHunk);
  if (currentFile) files.push(currentFile);
  return files;
}

export type GutterMarkType = "add" | "delete" | "modify";

/**
 * Build a map of new-file line number → change type, for gutter decorations
 * on an open file. Pure deletions (no matching added line in the hunk) are
 * marked on the line immediately following the deletion point (hunk.newStart),
 * matching common diff-gutter UX (e.g. VS Code's "removed above" indicator).
 */
export function buildGutterMarks(diffContent: string, filePath: string): Map<number, GutterMarkType> {
  const marks = new Map<number, GutterMarkType>();
  const file = parseDiff(diffContent).find((f) => f.filename === filePath);
  if (!file) return marks;

  for (const hunk of file.changes) {
    const hasAdd = hunk.lines.some((l) => l.type === "add");
    const hasDelete = hunk.lines.some((l) => l.type === "delete");
    for (const l of hunk.lines) {
      if (l.type === "add" && l.newLineNumber !== undefined) {
        marks.set(l.newLineNumber, hasDelete ? "modify" : "add");
      }
    }
    if (hasDelete && !hasAdd) {
      marks.set(Math.max(1, hunk.newStart), "delete");
    }
  }
  return marks;
}
