
/**
 * Constants for minimum and default terminal dimensions.
 *
 * Dimensions smaller than MIN_COLS/MIN_ROWS are transient (e.g., from xterm.js before
 * the CSS container has finished laying out). Caching or connecting at those dimensions
 * produces a garbled terminal on the next view. The first resize event often fires at
 * e.g., 10x6 before layout is complete.
 *
 * We also reject xterm.js default dimensions (80x24) to prevent cache corruption — the
 * logic in saveDimensions will not overwrite a larger existing cache with smaller
 * dimensions from the xterm.js default mount behavior.
 */
export const MIN_COLS = 30;
export const MIN_ROWS = 10;

/** xterm.js's default terminal dimensions — rejected to prevent cache corruption. */
export const XTERM_DEFAULT_COLS = 80;
export const XTERM_DEFAULT_ROWS = 24;

/**
 * Cached terminal dimensions structure with optional cell/pixel dimensions for pre-sizing
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
      console.log(
        `[TerminalDimensionCache] Loaded cached dimensions for ${sessionId}: ${dims.cols}x${dims.rows} (cell: ${dims.cellWidth?.toFixed(2)}x${dims.cellHeight?.toFixed(2)})`
      );
      return dims;
    }
  } catch (err) {
    console.warn('[TerminalDimensionCache] Failed to load cached dimensions:', err);
    return null;
  }
  return null;
}

/**
 * Get the current window dimensions with validation against minimum values.
 *
 * @returns Object with cols, rows, cellWidth, cellHeight, or null if dimensions invalid
 */
export function getCurrentWindowDimensions(): CachedDimensions | null {
  if (typeof window === 'undefined') return null;

  // Get window inner dimensions
  const cols = Math.max(MIN_COLS, Math.min(window.innerWidth / 8, XTERM_DEFAULT_COLS));
  const rows = Math.max(MIN_ROWS, Math.min(window.innerHeight / 18, XTERM_DEFAULT_ROWS));

  console.log(`[TerminalDimensionCache] Calculated window dimensions: ${cols}x${rows}`);

  return { cols, rows };
}

/**
 * Validate cached terminal dimensions against current font configuration.
 *
 * @param cached - Cached dimensions from localStorage (must include cols, rows, optionally cellWidth/cellHeight, fontSize/fontFamily)
 * @param fontSize - Current font size in px (from DEFAULT_TERMINAL_CONFIG)
 * @param fontFamily - Current font family (from DEFAULT_TERMINAL_CONFIG)
 * @returns Validated dimensions, or null if cache is stale/corrupt
 */
export function validateCellDimensions(
  cached: CachedDimensions,
  fontSize: number,
  fontFamily: string,
): CachedDimensions | null {
  // Create a copy to avoid mutating the cached value
  const validated: CachedDimensions = { cols: cached.cols, rows: cached.rows };

  // Font configuration changed — invalidate cell dimensions
  if (cached.fontSize != null && cached.fontFamily != null &&
      (cached.fontSize !== fontSize || cached.fontFamily !== fontFamily)) {
    console.log(
      `[TerminalDimensionCache] Invalidating stale cache due to font config change: ` +
      `${cached.fontSize}px/${cached.fontFamily} → ${fontSize}px/${fontFamily}`
    );
    return null;
  }

  // Preserve cell dimensions when font unchanged, but reset font-related fields
  if (cached.cellWidth != null && cached.cellHeight != null) {
    validated.cellWidth = cached.cellWidth;
    validated.cellHeight = cached.cellHeight;
    validated.fontSize = fontSize;
    validated.fontFamily = fontFamily;
  }

  console.log(
    `[TerminalDimensionCache] Validated cached dimensions: ${validated.cols}x${validated.rows}`
  );
  return validated;
}


/** Rejects xterm.js defaults (80x24) and sub-minimum dimensions so they never corrupt the cache. */
function isValidToCache(cols: number, rows: number): boolean {
  if (cols === XTERM_DEFAULT_COLS && rows === XTERM_DEFAULT_ROWS) {
    console.log(
      `[TerminalDimensionCache] Rejecting xterm.js default dimensions (${cols}x${rows}) for caching`
    );
    return false;
  }

  if (cols < MIN_COLS || rows < MIN_ROWS) {
    console.log(
      `[TerminalDimensionCache] Rejecting sub-minimum dimensions (${cols}x${rows}) for caching`
    );
    return false;
  }

  return true;
}

/**
 * Configuration options for saveDimensions
 */
export interface SaveDimensionsOptions {
  /** Pixel width per column (from xterm's render service) */
  cellWidth?: number;
  /** Pixel height per row (from xterm's render service) */
  cellHeight?: number;
  /** Current font size in px (used to detect stale cache on next load) */
  fontSize?: number;
  /** Current font family (used to detect stale cache on next load) */
  fontFamily?: string;
}

/**
 * Save terminal dimensions to localStorage for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param options - Optional configuration for cell dimensions and font metadata
 */
export function saveDimensions(
  sessionId: string,
  cols: number,
  rows: number,
  options?: SaveDimensionsOptions,
): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `terminal-dimensions-${sessionId}`;
    const payload: CachedDimensions = { cols, rows };

    // Validate dimensions before caching
    if (!isValidToCache(cols, rows)) {
      return;
    }

    // Check if these dimensions are better than existing cache
    if (!shouldOverwriteCache(key, cols, rows, options)) {
      return;
    }

    if (options?.cellWidth != null && options?.cellHeight != null) {
      payload.cellWidth = options.cellWidth;
      payload.cellHeight = options.cellHeight;
      payload.fontSize = options.fontSize;
      payload.fontFamily = options.fontFamily;
    }

    localStorage.setItem(key, JSON.stringify(payload));
    console.log(`[TerminalDimensionCache] Saved dimensions for ${sessionId}: ${cols}x${rows}${options?.cellWidth != null ? ` (cell: ${options.cellWidth.toFixed(2)}x${options.cellHeight!.toFixed(2)})` : ''}`);
  } catch (err) {
    console.warn('[TerminalDimensionCache] Failed to save dimensions:', err);
  }
}

/**
 * Check if we should overwrite the existing cache entry.
 *
 * @param key - The localStorage key to check
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param options - Optional configuration for cell dimensions
 * @returns true if we should overwrite the cache, false otherwise
 */
export function shouldOverwriteCache(
  key: string,
  cols: number,
  rows: number,
  options?: SaveDimensionsOptions,
): boolean {
  const existing = localStorage.getItem(key);
  if (!existing) return true;

  try {
    const existingDims = JSON.parse(existing) as CachedDimensions;
    // Only overwrite if new dimensions are larger (better)
    if (existingDims.cols >= cols && existingDims.rows >= rows) {
      console.log(`[TerminalDimensionCache] Skipping smaller dimensions ${cols}x${rows} (existing: ${existingDims.cols}x${existingDims.rows})`);
      return false;
    }
    // If existing has cell dimensions and new doesn't, preserve existing
    if (existingDims.cellWidth != null && existingDims.cellHeight != null &&
        (options?.cellWidth == null || options?.cellHeight == null)) {
      console.log(`[TerminalDimensionCache] Skipping dimensions without cell data (existing has cell: ${existingDims.cellWidth}x${existingDims.cellHeight})`);
      return false;
    }
  } catch (parseErr) {
    console.warn('[TerminalDimensionCache] Failed to parse existing dimensions:', parseErr);
    return false;
  }

  return true;
}
