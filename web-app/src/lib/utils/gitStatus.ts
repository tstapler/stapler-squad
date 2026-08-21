import { FileStatus, type FileChange } from "@/gen/session/v1/types_pb";

/**
 * Maps a FileStatus to the single-character badge shown in compact file-tree
 * rows. The conflict glyph here ("U", for "Unmerged" — matching git's own
 * porcelain status char) intentionally differs from VcsPanel.tsx's
 * FILE_STATUS_META, which uses "!!" for the same status in its full-row
 * panel context where a louder glyph is appropriate.
 */
export function fileChangeToStatusLetter(status: FileStatus): string {
  switch (status) {
    case FileStatus.MODIFIED:    return "M";
    case FileStatus.ADDED:       return "A";
    case FileStatus.DELETED:     return "D";
    case FileStatus.RENAMED:     return "R";
    case FileStatus.COPIED:      return "C";
    case FileStatus.UNTRACKED:   return "?";
    case FileStatus.IGNORED:     return "!";
    case FileStatus.CONFLICT:    return "U";
    default:                     return "";
  }
}

export function buildGitStatusMap(files: FileChange[]): Map<string, string> {
  const map = new Map<string, string>();
  for (const f of files) {
    const letter = fileChangeToStatusLetter(f.status);
    if (letter && f.path) {
      map.set(f.path, letter);
    }
  }
  return map;
}

export interface LineStats {
  add: number;
  del: number;
}

/** Maps a file path to its added/removed line counts, from FileChange.additions/deletions. */
export function buildLineStatsMap(files: FileChange[]): Map<string, LineStats> {
  const map = new Map<string, LineStats>();
  for (const f of files) {
    if (f.path && (f.additions > 0 || f.deletions > 0)) {
      map.set(f.path, { add: f.additions, del: f.deletions });
    }
  }
  return map;
}
