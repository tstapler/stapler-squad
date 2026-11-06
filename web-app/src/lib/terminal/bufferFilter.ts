/**
 * Display-only filtering of terminal output. Works on lines read out of the xterm
 * buffer, never on the PTY byte stream, so filtering can't corrupt rendering.
 */

export const MAX_FILTER_RESULTS = 500;

export interface FilterOptions {
  regex?: boolean;
  caseSensitive?: boolean;
}

export interface FilterMatch {
  /** 1-based line number in the buffer (scrollback + viewport). */
  line: number;
  text: string;
}

export interface FilterResult {
  matches: FilterMatch[];
  /** More than MAX_FILTER_RESULTS lines matched; only the most recent are kept. */
  truncated: boolean;
  /** Set when `regex` is on and the query isn't a valid pattern. */
  error?: string;
}

interface BufferLike {
  buffer: {
    active: {
      length: number;
      getLine(index: number): { translateToString(trimRight?: boolean): string } | undefined;
    };
  };
}

/** Lines of the active xterm buffer (scrollback + viewport) as plain text. */
export function readBufferLines(terminal: BufferLike | null | undefined): string[] {
  if (!terminal) return [];
  const buf = terminal.buffer.active;
  const lines: string[] = [];
  for (let i = 0; i < buf.length; i++) {
    lines.push(buf.getLine(i)?.translateToString(true) ?? "");
  }
  return lines;
}

function buildMatcher(query: string, { regex, caseSensitive }: FilterOptions): ((s: string) => boolean) | { error: string } {
  if (regex) {
    try {
      const re = new RegExp(query, caseSensitive ? "" : "i");
      return (s) => re.test(s);
    } catch (err) {
      return { error: err instanceof Error ? err.message : "Invalid pattern" };
    }
  }
  const needle = caseSensitive ? query : query.toLowerCase();
  return (s) => (caseSensitive ? s : s.toLowerCase()).includes(needle);
}

/** Matching lines for `query`; an empty query matches nothing (the panel shows a hint instead). */
export function filterLines(lines: string[], query: string, options: FilterOptions = {}): FilterResult {
  if (query === "") return { matches: [], truncated: false };

  const matcher = buildMatcher(query, options);
  if ("error" in matcher) return { matches: [], truncated: false, error: matcher.error };

  const all: FilterMatch[] = [];
  lines.forEach((text, i) => {
    if (matcher(text)) all.push({ line: i + 1, text });
  });
  const truncated = all.length > MAX_FILTER_RESULTS;
  return { matches: truncated ? all.slice(-MAX_FILTER_RESULTS) : all, truncated };
}
