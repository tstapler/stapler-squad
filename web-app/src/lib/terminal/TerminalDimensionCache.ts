/**
 * TerminalDimensionCache - localStorage persistence for terminal dimensions.
 *
 * Caches terminal dimensions per session to enable instant reconnection
 * without waiting for size stability detection. Pure utility functions
 * with no React dependencies.
 */

export interface CachedDimensions {
  cols: number;
  rows: number;
  /**
   * Pixels per column at the time of the last fit. When present alongside
   * cellHeight, TerminalOutput can pre-calculate cols/rows from the container's
   * pixel size on mount — enabling an immediate connection before xterm fires
   * its first onResize event.
   */
  cellWidth?: number;
  /**
   * Pixels per row at the time of the last fit.
   */
  cellHeight?: number;
  /**
   * Font size (px) at the time this cache entry was written.
   * Used to invalidate stale cell dimensions when font config changes (R1.6).
   */
  fontSize?: number;
  /**
   * Font family at the time this cache entry was written.
   * Used to invalidate stale cell dimensions when font config changes (R1.6).
   */
  fontFamily?: string;
}

/**
 * Retrieve cached terminal dimensions for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @returns The cached dimensions, or null if not found or on error
 */
export function getCachedDimensions(sessionId: string): CachedDimensions | null {
  if (typeof window === 'undefined') return null;
  try {
    const key = `terminal-dimensions-${sessionId}`;
    const cached = localStorage.getItem(key);
    if (cached) {
      const dims = JSON.parse(cached) as CachedDimensions;
      console.log(`[TerminalDimensionCache] Loaded cached dimensions for ${sessionId}: ${dims.cols}x${dims.rows} (cell: ${dims.cellWidth?.toFixed(2)}x${dims.cellHeight?.toFixed(2)})`);
      return dims;
    }
  } catch (err) {
    console.warn('[TerminalDimensionCache] Failed to load cached dimensions:', err);
  }
  return null;
}

/**
 * Save terminal dimensions to localStorage for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param cellWidth - Optional pixel width per column (from xterm's render service)
 * @param cellHeight - Optional pixel height per row (from xterm's render service)
 * @param fontSize - Optional current font size in px (used to detect stale cache on next load)
 * @param fontFamily - Optional current font family (used to detect stale cache on next load)
 */
export function saveDimensions(
  sessionId: string,
  cols: number,
  rows: number,
  cellWidth?: number,
  cellHeight?: number,
  fontSize?: number,
  fontFamily?: string,
): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `terminal-dimensions-${sessionId}`;
    const payload: CachedDimensions = { cols, rows };
    if (cellWidth != null && cellHeight != null) {
      payload.cellWidth = cellWidth;
      payload.cellHeight = cellHeight;
    }
    if (fontSize != null) payload.fontSize = fontSize;
    if (fontFamily != null) payload.fontFamily = fontFamily;
    localStorage.setItem(key, JSON.stringify(payload));
    console.log(`[TerminalDimensionCache] Saved dimensions for ${sessionId}: ${cols}x${rows}${cellWidth != null ? ` (cell: ${cellWidth.toFixed(2)}x${cellHeight!.toFixed(2)})` : ''}`);
  } catch (err) {
    console.warn('[TerminalDimensionCache] Failed to save dimensions:', err);
  }
}

/**
 * Validate cached cell dimensions against the current font config (R1.6).
 * Stale cell dimensions from a previous font configuration produce an incorrect
 * initial fit() measurement, causing the first resize to report wrong cols/rows.
 *
 * Returns a copy of `cached` with cellWidth/cellHeight cleared if the font
 * configuration has changed. Callers should treat absent cell dims as "no pre-sizing".
 */
export function validateCellDimensions(
  cached: CachedDimensions,
  currentFontSize: number,
  currentFontFamily: string,
): CachedDimensions {
  if (cached.cellWidth == null || cached.cellHeight == null) {
    return cached; // No cell dims to validate — safe
  }

  // If we have cell dims but no font metadata, the entry predates R1.6 and is stale.
  // Using pre-R1.6 cell dims with a different (or unknown) font config causes wrong initial fit (Bug 3 fix).
  if (cached.fontSize == null || cached.fontFamily == null) {
    console.log(
      `[TerminalDimensionCache] Discarding stale cell dimensions (pre-R1.6 entry: no font metadata)`
    );
    const { cellWidth: _cw, cellHeight: _ch, ...rest } = cached;
    return rest;
  }

  const fontSizeChanged = cached.fontSize !== currentFontSize;
  const fontFamilyChanged = cached.fontFamily !== currentFontFamily;

  if (fontSizeChanged || fontFamilyChanged) {
    // Stale cache from different font config causes wrong initial fit (R1.6)
    console.log(
      `[TerminalDimensionCache] Discarding stale cell dimensions (font changed: ` +
      `size ${cached.fontSize}→${currentFontSize}, family "${cached.fontFamily}"→"${currentFontFamily}")`
    );
    const { cellWidth: _cw, cellHeight: _ch, ...rest } = cached;
    return rest;
  }

  return cached;
}
