/**
 * Client-side state for paging server-side scrollback into a terminal, newest page first.
 *
 * Session load sends only the visible pane; older history is requested a page at a time as the
 * user nears the top of what is already loaded. The server addresses history by distance from
 * the live bottom (`oldestSequence`), so the next request starts where the last page ended.
 * Renderer-agnostic: callers decide *when* to ask (DOM scroll for xterm, `onScroll` for ghostty).
 */

export const SCROLLBACK_PAGE_LINES = 500;

/** Ask for more once fewer than this many loaded lines sit above the viewport. */
export const SCROLLBACK_NEAR_TOP_LINES = 200;

export interface ScrollbackPageMeta {
  hasMore: boolean;
  oldestSequence: number;
}

export interface ScrollbackPage {
  fromSequence: number;
  limit: number;
}

export class ScrollbackPager {
  private inFlight = false;
  private hasMore = false;
  private oldestSequence = 0;

  /** A fresh pane snapshot replaced everything loaded, so history starts over from the bottom. */
  reset(): void {
    this.inFlight = false;
    this.hasMore = true;
    this.oldestSequence = 0;
  }

  get isInFlight(): boolean {
    return this.inFlight;
  }

  /** Whether `viewportY` (lines above the viewport, xterm-style) is close enough to the top to want more. */
  isNearTop(viewportY: number): boolean {
    return viewportY < SCROLLBACK_NEAR_TOP_LINES;
  }

  /** Claims the next page and returns what to request, or null if one is already in flight or nothing older exists. */
  tryBegin(): ScrollbackPage | null {
    return this.hasMore ? this.tryBeginIgnoringHasMore() : null;
  }

  /**
   * Like `tryBegin`, for a path that tracks its own exhaustion (app-forwarded history) and has
   * already checked it: shares the in-flight guard and position but not `hasMore`.
   */
  tryBeginIgnoringHasMore(): ScrollbackPage | null {
    if (this.inFlight) return null;
    this.inFlight = true;
    return { fromSequence: this.oldestSequence, limit: SCROLLBACK_PAGE_LINES };
  }

  /** Records a delivered page and frees the in-flight claim. */
  complete(meta?: ScrollbackPageMeta): void {
    if (meta) {
      this.hasMore = meta.hasMore;
      this.oldestSequence = meta.oldestSequence;
    }
    this.inFlight = false;
  }

  /** Frees the in-flight claim without changing position (cancelled, failed, or answered by another path). */
  release(): void {
    this.inFlight = false;
  }
}
