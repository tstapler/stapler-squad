/**
 * Truncates a goal text to a maximum length, appending "…" if truncated.
 * Returns the original string unchanged when it is already within the limit.
 */
export function truncateGoal(text: string, max: number): string {
  if (!text) return text;
  if (text.length <= max) return text;
  return text.slice(0, max) + "…";
}
