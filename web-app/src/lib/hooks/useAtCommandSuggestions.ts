import { useMemo } from "react";
import type { WorkflowEntry } from "@/lib/omnibar/detectors/WorkflowDetector";

// Matches "@slug" with no space — the slug is still being completed.
const AT_PREFIX = /^@([a-zA-Z0-9_-]*)$/;

export interface UseAtCommandSuggestionsResult {
  isAtCommand: boolean;
  query: string;
  suggestions: WorkflowEntry[];
  complete: (wf: WorkflowEntry) => string;
}

/**
 * Given the current input value and a list of available workflows, returns
 * filtered @slug suggestions and a helper to produce the completed input string.
 *
 * Active when the input matches /^@slug-chars-only$/ (no space yet).
 * Calling complete(wf) returns "@slug " (with trailing space) ready to set as input.
 */
export function useAtCommandSuggestions(
  value: string,
  workflows: WorkflowEntry[]
): UseAtCommandSuggestionsResult {
  const match = value.match(AT_PREFIX);
  const isAtCommand = match !== null;
  const query = isAtCommand ? match[1].toLowerCase() : "";

  const suggestions = useMemo<WorkflowEntry[]>(() => {
    if (!isAtCommand) return [];
    if (!query) return workflows;
    return workflows.filter((w) => w.slug.toLowerCase().startsWith(query));
  }, [isAtCommand, query, workflows]);

  function complete(wf: WorkflowEntry): string {
    return `@${wf.slug} `;
  }

  return { isAtCommand, query, suggestions, complete };
}
