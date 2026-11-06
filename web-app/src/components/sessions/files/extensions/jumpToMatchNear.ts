export interface MatchPosition { line: number; col: number }

/**
 * Nearest line (1-based) to `hitLine` that contains `needle`, searching outward up to `radius` lines;
 * on a tie the earlier line wins. Search hits come from disk, so a dirty buffer may have moved the text.
 */
export function jumpToMatchNear(lines: readonly string[], hitLine: number, needle: string, radius: number): MatchPosition | null {
  if (!needle || lines.length === 0) return null;
  const center = Math.min(Math.max(1, Math.floor(hitLine)), lines.length);
  for (let d = 0; d <= radius; d++) {
    for (const line of d === 0 ? [center] : [center - d, center + d]) {
      if (line < 1 || line > lines.length) continue;
      const col = lines[line - 1].indexOf(needle);
      if (col >= 0) return { line, col };
    }
  }
  return null;
}
