import type { SearchEvent, SearchHit, SearchQuery, SearchStream } from "@/lib/files/searchClient";

export const searchQuery = (pattern: string, extra: Partial<SearchQuery> = {}): SearchQuery => ({
  pattern, regex: false, caseSensitive: false, wholeWord: false, include: [], exclude: [], includeIgnored: false, largeFiles: false, ...extra,
});

export const searchHit = (path: string, line: number, text = `line ${line} foo`): SearchHit => ({
  path, line, text, start: Math.max(0, text.indexOf("foo")), end: Math.max(0, text.indexOf("foo")) + 3, byteOffset: 0,
});

export const searchStats = (over: Partial<Extract<SearchEvent, { type: "stats" }>> = {}): SearchEvent => ({
  type: "stats", filesScanned: 12, binarySkipped: 0, truncated: false, durationMs: 5, largeFilesSkipped: 0, ...over,
});

export interface ControlledRequest {
  query: SearchQuery;
  pattern: string;
  signal: AbortSignal;
  push: (e: SearchEvent) => void;
  end: () => void;
  fail: (e: unknown) => void;
}

/** A SearchStream the test drives by hand; every request is recorded in `requests`. */
export function controllableSearchStream() {
  const requests: ControlledRequest[] = [];
  const stream: SearchStream = (query, signal) => ({
    [Symbol.asyncIterator]() {
      const queue: SearchEvent[] = [];
      let wake: (() => void) | null = null;
      let done = false;
      let error: unknown = null;
      const poke = () => { wake?.(); wake = null; };
      requests.push({
        query, pattern: query.pattern, signal,
        push: (e) => { queue.push(e); poke(); },
        end: () => { done = true; poke(); },
        fail: (e) => { error = e; done = true; poke(); },
      });
      signal.addEventListener("abort", () => { done = true; poke(); });
      return {
        async next() {
          for (;;) {
            if (queue.length) return { value: queue.shift()!, done: false };
            if (error) throw error;
            if (done) return { value: undefined, done: true };
            await new Promise<void>((r) => { wake = r; });
          }
        },
      };
    },
  });
  return { stream, requests };
}
