/**
 * Truncates a filename using middle truncation, preserving both the start of
 * the name and the file extension. E.g.:
 *   truncateMiddle("very-long-filename.tsx", 18) → "very-lon…name.tsx"
 */
export function truncateMiddle(name: string, maxLen: number): string {
  if (!name) return name;
  if (name.length <= maxLen) return name;

  // For very small maxLen, just truncate from the right with an ellipsis.
  if (maxLen < 5) {
    return name.slice(0, maxLen - 1) + "…";
  }

  // Find the extension.
  const dotIdx = name.lastIndexOf(".");
  let suffix: string;
  let base: string;
  if (dotIdx > 0) {
    suffix = name.slice(dotIdx); // e.g. ".tsx"
    base = name.slice(0, dotIdx);
  } else {
    suffix = "";
    base = name;
  }

  // How many chars of the base we can show (1 char budget for the ellipsis "…").
  let keep = maxLen - suffix.length - 1;

  // Ensure meaningful truncation: we need at least 1 char head + 1 char tail.
  if (keep < 2) {
    // Fall back to right-truncation with ellipsis.
    return name.slice(0, maxLen - 1) + "…";
  }

  let head = Math.ceil(keep * 0.6);
  let tail = keep - head;

  // Guarantee at least 1 char on each side.
  if (head < 1) head = 1;
  if (tail < 1) {
    tail = 1;
    head = keep - 1;
  }

  return base.slice(0, head) + "…" + base.slice(base.length - tail) + suffix;
}
