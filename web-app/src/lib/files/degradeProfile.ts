/** What the editor may switch off so a large file stays responsive on a phone (ux.md U-MOB-5). */
export interface DegradeProfile {
  degraded: boolean;
  syntaxHighlight: boolean;
  bracketMatching: boolean;
  activeLine: boolean;
  foldGutter: boolean;
  /** False when a single line is so long that wrapping it would stall layout; the line scrolls sideways instead. */
  wrap: boolean;
  /** One line shown above the editor when features were dropped. */
  notice: string | null;
}

/** Thresholds are constants so the Task 0.1.5b phone sitting can retune them. UTF-16 units, close enough to bytes for source. */
export const DEGRADE_MIN_CHARS = 512 * 1024;
export const LONG_LINE_CHARS = 4000;

export const DEGRADE_NOTICE = "Large file: syntax highlighting, bracket matching and folding are off to keep scrolling smooth.";

export const FULL_PROFILE: DegradeProfile = {
  degraded: false, syntaxHighlight: true, bracketMatching: true, activeLine: true, foldGutter: true, wrap: true, notice: null,
};

export function longestLineLength(doc: string): number {
  let longest = 0;
  let start = 0;
  for (let i = 0; i <= doc.length; i++) {
    if (i === doc.length || doc.charCodeAt(i) === 10) {
      const end = i > start && doc.charCodeAt(i - 1) === 13 ? i - 1 : i;
      if (end - start > longest) longest = end - start;
      start = i + 1;
    }
  }
  return longest;
}

/**
 * Desktop and small files keep everything; only a coarse pointer degrades. A windowed file (over 1 MiB, served by
 * range reads) is never degraded whole: `doc` is only the loaded text, which still decides wrapping.
 */
export function degradeProfile({ doc, pointerCoarse, windowed = false }: { doc: string; pointerCoarse: boolean; windowed?: boolean }): DegradeProfile {
  if (!pointerCoarse) return FULL_PROFILE;
  const wrap = longestLineLength(doc) < LONG_LINE_CHARS;
  if (windowed || doc.length < DEGRADE_MIN_CHARS) return wrap ? FULL_PROFILE : { ...FULL_PROFILE, wrap };
  return { degraded: true, syntaxHighlight: false, bracketMatching: false, activeLine: false, foldGutter: false, wrap, notice: DEGRADE_NOTICE };
}
