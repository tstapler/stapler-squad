/** Where the Accessible view gets its lines; Task 1.6.5c swaps in a transport-backed source for windowed buffers. */
export interface PageSource {
  readonly lineCount: number;
  /** Lines [start, end), 0-based, clamped to the document. */
  lines(start: number, end: number): string[];
  text(): string;
}

export const ACCESSIBLE_PAGE_SIZE = 500;

/** A source over an in-memory string: line starts are indexed once, lines are sliced per page (no per-line copy). */
export function textPageSource(text: string): PageSource {
  const starts = [0];
  for (let i = text.indexOf("\n"); i !== -1; i = text.indexOf("\n", i + 1)) starts.push(i + 1);
  const lineEnd = (i: number) => (i + 1 < starts.length ? starts[i + 1] - 1 : text.length);
  return {
    lineCount: starts.length,
    text: () => text,
    lines(start, end) {
      const out: string[] = [];
      for (let i = Math.max(0, start); i < Math.min(end, starts.length); i++) {
        const e = lineEnd(i);
        out.push(text.slice(starts[i], e > starts[i] && text.charCodeAt(e - 1) === 13 ? e - 1 : e));
      }
      return out;
    },
  };
}

export interface Page {
  /** 1-based number of the first line on the page. */
  first: number;
  last: number;
  total: number;
  pages: number;
  lines: string[];
  label: string;
}

export const pageCount = (total: number, size = ACCESSIBLE_PAGE_SIZE) => Math.max(1, Math.ceil(total / size));
export const pageOfLine = (line: number, size = ACCESSIBLE_PAGE_SIZE) => Math.floor((line - 1) / size);

/** The `page`-th (0-based) page of at most `size` lines; an out-of-range page clamps to the last. */
export function paginateLines(source: PageSource, page: number, size = ACCESSIBLE_PAGE_SIZE): Page {
  const total = source.lineCount;
  const pages = pageCount(total, size);
  const p = Math.min(Math.max(0, page), pages - 1);
  const start = p * size;
  const lines = source.lines(start, start + size);
  const first = start + 1;
  const last = start + lines.length;
  return { first, last, total, pages, lines, label: `Lines ${first}-${last} of ${total}` };
}
