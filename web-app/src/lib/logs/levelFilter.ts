/** Badge tap: show only `level`; tapping it again while it is the sole active filter resets to "ALL" (what the toolbar chips treat as no filter). */
export function nextLevelFilters(current: string[], level: string): string[] {
  return current.length === 1 && current[0] === level ? ["ALL"] : [level];
}
