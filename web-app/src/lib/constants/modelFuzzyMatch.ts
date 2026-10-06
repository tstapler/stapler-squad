import Fuse, { type IFuseOptions } from "fuse.js";
import { MODEL_AUTOCOMPLETE_OPTIONS, type ModelOption } from "./programs";

// Search both the human label ("Sonnet (latest)") and the raw value
// ("claude-sonnet-4-6") so typos against either surface a result.
const FUSE_OPTIONS: IFuseOptions<ModelOption> = {
  keys: [
    { name: "label", weight: 0.6 },
    { name: "value", weight: 0.4 },
  ],
  includeScore: true,
  threshold: 0.4,
  minMatchCharLength: 1,
  ignoreLocation: true,
};

const modelFuse = new Fuse(MODEL_AUTOCOMPLETE_OPTIONS, FUSE_OPTIONS);

/**
 * Fuzzy/typo-tolerant filter for the Workflow model field's AutocompleteInput.
 * `suggestions` is the value list passed to AutocompleteInput (kept as the
 * signature contract so it can be used directly as a `filterFn`); results are
 * ranked best-match-first by Fuse.js and restricted to that value list.
 */
export function fuzzyMatchModels(query: string, suggestions: string[]): string[] {
  const trimmed = query.trim();
  if (!trimmed) return suggestions;

  const allowed = new Set(suggestions);
  return modelFuse
    .search(trimmed)
    .map((r) => r.item.value)
    .filter((value) => allowed.has(value));
}

/** Display label for a model suggestion value; falls back to the raw value. */
export function getModelLabel(value: string): string {
  return MODEL_AUTOCOMPLETE_OPTIONS.find((o) => o.value === value)?.label ?? value;
}
