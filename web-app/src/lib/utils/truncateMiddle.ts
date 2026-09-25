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

  // Find the extension. Only treat a trailing dot as a real filename extension
  // when what follows it is short — a genuine extension like ".tsx" is a handful
  // of chars, but a dotted path segment (e.g. "github.com/some-org/repo-name")
  // can have dozens of chars after its last "." and isn't an extension at all.
  // Treating that whole tail as a preserved "suffix" starves `keep`'s head/tail
  // budget and forces the right-truncation fallback below, which drops the most
  // identifying trailing path segment instead of truncating the middle.
  const MAX_EXTENSION_LENGTH = 10;
  const dotIdx = name.lastIndexOf(".");
  let suffix: string;
  let base: string;
  if (dotIdx > 0 && name.length - dotIdx <= MAX_EXTENSION_LENGTH) {
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
