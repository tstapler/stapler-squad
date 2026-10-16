// logPatterns.ts — Datadog-style "log pattern" clustering: groups log
// entries by message with dynamic content (UUIDs, paths, embedded objects,
// numbers) normalized into placeholders, so structurally identical messages
// collapse into one row even when a value is interpolated into the message
// text.

import type { LogEntry, LogLevel } from "@/lib/hooks/useLogViewer";

const UUID_RE = /[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/g;
const PATH_RE = /(\/[A-Za-z0-9_.-]+){2,}/g;
const OBJ_RE = /\{[^{}]*\}/g;
const NUMBER_RE = /[0-9]+/g;

/** Normalizes one log message into a pattern with placeholders substituted for dynamic content. */
export function normalizeMessage(message: string): string {
  return message
    .replace(UUID_RE, "<uuid>")
    .replace(PATH_RE, "<path>")
    .replace(OBJ_RE, "<obj>")
    .replace(NUMBER_RE, "<n>");
}

export interface LogPatternGroup {
  /** Stable key: `${level} ${pattern}`. */
  key: string;
  level: LogLevel;
  /** The normalized message with placeholders, e.g. "queue full, dropping path=<path>". */
  pattern: string;
  count: number;
  /** All raw entries that normalized to this pattern, most recent first. */
  entries: LogEntry[];
}

/**
 * Groups log entries into patterns by (level, normalized message), sorted by
 * count descending, so a page full of near-identical lines collapses into
 * one row with an expandable list of the actual matching entries (which
 * still show whatever varied — the path, the UUID, the count — since
 * normalization only affects the grouping key, not what's displayed on
 * expand).
 */
export function groupLogsByPattern(entries: LogEntry[]): LogPatternGroup[] {
  const groups = new Map<string, LogPatternGroup>();

  for (const entry of entries) {
    const pattern = normalizeMessage(entry.message);
    const key = `${entry.level} ${pattern}`;
    const existing = groups.get(key);
    if (existing) {
      existing.count += 1;
      existing.entries.push(entry);
    } else {
      groups.set(key, { key, level: entry.level, pattern, count: 1, entries: [entry] });
    }
  }

  return Array.from(groups.values()).sort((a, b) => b.count - a.count);
}
