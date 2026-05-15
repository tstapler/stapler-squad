// logParser.ts — Epic 3 implementation: level detection, search segmentation,
// ANSI pipeline, and JSON detection.

import AnsiToHtml from "ansi-to-html";
import DOMPurify from "dompurify";

export type LogLevel = "ERROR" | "WARN" | "INFO" | "DEBUG" | "TRACE" | "UNKNOWN";

// ---------------------------------------------------------------------------
// T1: Level detection
// ---------------------------------------------------------------------------

// Precompile at module level for performance (not inside the function)
const LEVEL_PATTERNS: Array<[LogLevel, RegExp]> = [
  ["ERROR", /\b(ERROR|ERR)\b/i],
  ["WARN", /\bWARN(?:ING)?\b/i],
  ["INFO", /\bINFO\b/i],
  ["DEBUG", /\bDEBUG\b/i],
  ["TRACE", /\bTRACE\b/i],
];

export function detectLevel(line: string): LogLevel {
  for (const [level, pattern] of LEVEL_PATTERNS) {
    if (pattern.test(line)) return level;
  }
  return "UNKNOWN";
}

// ---------------------------------------------------------------------------
// T2: Search highlight segmentation
// ---------------------------------------------------------------------------

export function segmentText(
  text: string,
  query: string,
): Array<{ text: string; highlight: boolean }> {
  if (!query) return [{ text, highlight: false }];
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase();
  const segments: Array<{ text: string; highlight: boolean }> = [];
  let pos = 0;
  let idx: number;
  while ((idx = lowerText.indexOf(lowerQuery, pos)) !== -1) {
    if (idx > pos) segments.push({ text: text.slice(pos, idx), highlight: false });
    segments.push({ text: text.slice(idx, idx + query.length), highlight: true });
    pos = idx + query.length;
  }
  if (pos < text.length) segments.push({ text: text.slice(pos), highlight: false });
  return segments.length > 0 ? segments : [{ text, highlight: false }];
}

// ---------------------------------------------------------------------------
// T3: ANSI pipeline
// ---------------------------------------------------------------------------

const converter = new AnsiToHtml({ escapeXML: true });

// Strip OSC sequences (OSC 8 hyperlinks etc.) before processing
const OSC_REGEX = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;

export function renderAnsi(raw: string): string {
  const stripped = raw.replace(OSC_REGEX, "");
  const html = converter.toHtml(stripped);
  // DOMPurify only runs in browser; in SSR/test context, return raw html
  if (typeof window === "undefined") return html;
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: ["span"],
    ALLOWED_ATTR: ["style"],
  });
}

// ---------------------------------------------------------------------------
// T4: JSON detection with bounded LRU-style cache
// ---------------------------------------------------------------------------

const jsonCache = new Map<string, object | null>();

export function tryParseJson(text: string): object | null {
  const trimmed = text.trim();
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return null;
  if (jsonCache.has(trimmed)) return jsonCache.get(trimmed)!;
  try {
    const parsed = JSON.parse(trimmed);
    if (jsonCache.size >= 500) {
      const firstKey = jsonCache.keys().next().value;
      if (firstKey !== undefined) jsonCache.delete(firstKey);
    }
    jsonCache.set(trimmed, typeof parsed === "object" ? parsed : null);
    return typeof parsed === "object" ? parsed : null;
  } catch {
    jsonCache.set(trimmed, null);
    return null;
  }
}
