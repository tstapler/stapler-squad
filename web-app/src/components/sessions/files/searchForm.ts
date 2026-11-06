import type { SearchQuery } from "@/lib/files/searchClient";

export interface FormState {
  pattern: string;
  regex: boolean;
  caseSensitive: boolean;
  wholeWord: boolean;
  include: string;
  exclude: string;
  includeIgnored: boolean;
}

export const EMPTY: FormState = { pattern: "", regex: false, caseSensitive: false, wholeWord: false, include: "", exclude: "", includeIgnored: false };

const globs = (raw: string) => raw.split(",").map((g) => g.trim()).filter(Boolean);

/** `largeFiles` is a per-device preference, so it rides beside the form and survives "Clear filters". */
export const toQuery = (f: FormState, largeFiles: boolean): SearchQuery => ({
  pattern: f.pattern, regex: f.regex, caseSensitive: f.caseSensitive, wholeWord: f.wholeWord,
  include: globs(f.include), exclude: globs(f.exclude), includeIgnored: f.includeIgnored, largeFiles,
});

/** "(case-sensitive, regex)" style list of the toggles that are on. */
export function activeToggles(q: SearchQuery): string {
  const on = [
    q.caseSensitive && "case-sensitive", q.wholeWord && "whole word", q.regex && "regex",
    q.includeIgnored && "ignored files included", q.largeFiles && "large files included",
  ].filter(Boolean);
  return on.length ? ` (${on.join(", ")})` : "";
}
