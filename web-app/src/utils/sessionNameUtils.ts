/**
 * Generate a unique session name by appending a numeric suffix if needed.
 * Strips any existing "(N)" suffix before checking for conflicts.
 */
export function generateUniqueName(baseName: string, existingNames: string[]): string {
  if (!baseName || !baseName.trim()) return baseName;
  const existingSet = new Set(existingNames);

  // Strip trailing " (N)" suffix to get the root name
  const stripped = baseName.replace(/\s*\(\d+\)$/, "").trim();

  if (!existingSet.has(stripped)) return stripped;

  // If the original name (with suffix) doesn't conflict, keep it
  if (baseName !== stripped && !existingSet.has(baseName)) return baseName;

  let counter = 2;
  while (existingSet.has(`${stripped} (${counter})`)) counter++;
  return `${stripped} (${counter})`;
}
