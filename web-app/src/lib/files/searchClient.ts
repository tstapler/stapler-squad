// Streaming content search over plain Connect fetch streaming (ADR-030 decision 4, path a).
// Hooks depend on the SearchStream port so tests drive a hand-written stream.
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import {
  SearchCaseMode, SearchMatchMode, SearchWordMode, SessionService,
} from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

export interface SearchQuery {
  pattern: string;
  regex: boolean;
  caseSensitive: boolean;
  wholeWord: boolean;
  include: string[];
  exclude: string[];
  includeIgnored: boolean;
  largeFiles: boolean;
}

export interface SearchHit {
  path: string;
  /** 1-based. */
  line: number;
  text: string;
  /** Match range inside `text`, UTF-16 code units, end exclusive. */
  start: number;
  end: number;
  byteOffset: number;
}

export type SearchEvent =
  | { type: "queued"; ahead: number }
  | { type: "batch"; matches: SearchHit[] }
  | { type: "stats"; filesScanned: number; binarySkipped: number; truncated: boolean; durationMs: number; largeFilesSkipped: number };

export type SearchStream = (query: SearchQuery, signal: AbortSignal) => AsyncIterable<SearchEvent>;

/** The default per-file scan cap (ADR-030); a hit at or past this offset can only come from a large-file search. */
export const SEARCH_FILE_CAP_BYTES = 2 * 1024 * 1024;
/** `byteOffset` is the only size signal a hit carries, so files over the cap with an early match show no badge. */
export const isLargeFileHit = (hit: Pick<SearchHit, "byteOffset">): boolean => hit.byteOffset >= SEARCH_FILE_CAP_BYTES;

/** Longer patterns are refused by the server; a shorter one is never sent. */
export const MIN_QUERY_LENGTH = 2;

/** Per-browser-tab id: it only keys newest-wins on the server, never identity. */
export function newClientId(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `c${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`;
}

export function connectSearchStream(sessionId: string, clientId: string): SearchStream {
  return async function* (q, signal) {
    const client = createClient(SessionService, getConnectTransport());
    const request = {
      sessionId,
      clientId,
      options: {
        pattern: q.pattern,
        matchMode: q.regex ? SearchMatchMode.REGEX : SearchMatchMode.LITERAL,
        caseMode: q.caseSensitive ? SearchCaseMode.SENSITIVE : SearchCaseMode.INSENSITIVE,
        wordMode: q.wholeWord ? SearchWordMode.WHOLE : SearchWordMode.ANY,
        scope: { includeGlobs: q.include, excludeGlobs: q.exclude, includeIgnored: q.includeIgnored },
        largeFiles: q.largeFiles,
      },
    };
    for await (const msg of client.searchContent(request, { signal })) {
      const m = msg.message;
      if (m.case === "queued") yield { type: "queued", ahead: m.value.ahead };
      else if (m.case === "batch") {
        yield {
          type: "batch",
          matches: m.value.matches.map((x) => ({
            path: x.path, line: x.line, text: x.text, start: x.startUtf16, end: x.endUtf16, byteOffset: Number(x.byteOffset),
          })),
        };
      } else if (m.case === "stats") {
        const s = m.value;
        yield {
          type: "stats", filesScanned: s.filesScanned, binarySkipped: s.binarySkipped, truncated: s.truncated,
          durationMs: Number(s.durationMs), largeFilesSkipped: s.largeFilesSkipped,
        };
      }
    }
  };
}

export type SearchErrorKind = "busy" | "invalid" | "worktree" | "cancelled" | "error";

/** Maps a stream failure to a UI state; `message` is only set where the server text is meant for the user. */
export function classifySearchError(err: unknown): { kind: SearchErrorKind; message?: string } {
  if (!(err instanceof ConnectError)) return { kind: "error" };
  switch (err.code) {
    case Code.ResourceExhausted: return { kind: "busy" };
    case Code.InvalidArgument: return { kind: "invalid", message: err.rawMessage };
    case Code.FailedPrecondition: return err.rawMessage.includes("WorktreeMissing") ? { kind: "worktree" } : { kind: "error" };
    case Code.Canceled: return { kind: "cancelled" };
    default: return { kind: "error" };
  }
}
