// Re-export minimum dimension constants from TerminalOutput.tsx to avoid duplication
import { MIN_COLS, MIN_ROWS, XTERM_DEFAULT_COLS, XTERM_DEFAULT_ROWS } from '../components/sessions/TerminalOutput';

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
  }
  return null;
}

/**
 * Validate that dimensions are valid for caching.
 * Rejects xterm.js defaults (80x24) and dimensions below minimum.
 *
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @returns true if dimensions are valid for caching, false otherwise
 */
function isValidToCache(cols: number, rows: number): boolean {
  // Don't cache xterm.js default dimensions to prevent cache corruption
  if (cols === XTERM_DEFAULT_COLS && rows === XTERM_DEFAULT_ROWS) {
    console.log(
      `[TerminalDimensionCache] Rejecting xterm.js default dimensions (${cols}x${rows}) for caching`
    );
    return false;
  }

  // Don't cache dimensions below minimum threshold
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
function shouldOverwriteCache(
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
  }

  return true;
}
