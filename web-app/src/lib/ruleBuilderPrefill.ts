export interface RuleBuilderPrefill {
  programs?: string[];
  subcommands?: string[];
  toolName?: string;
  toolCategory?: string;
  suggestedDecision?: number;
}

export function encodePrefill(payload: RuleBuilderPrefill): string {
  return btoa(JSON.stringify(payload));
}

const VALID_DECISIONS = new Set([0, 1, 2, 3]); // AutoDecision enum values

export function decodePrefill(encoded: string): RuleBuilderPrefill | null {
  try {
    const obj = JSON.parse(atob(encoded));
    if (typeof obj !== "object" || obj === null) return null;
    const result: RuleBuilderPrefill = {};
    if (Array.isArray(obj.programs)) result.programs = obj.programs.filter((v: unknown) => typeof v === "string");
    if (Array.isArray(obj.subcommands)) result.subcommands = obj.subcommands.filter((v: unknown) => typeof v === "string");
    if (typeof obj.toolName === "string") result.toolName = obj.toolName;
    if (typeof obj.toolCategory === "string") result.toolCategory = obj.toolCategory;
    if (typeof obj.suggestedDecision === "number" && VALID_DECISIONS.has(obj.suggestedDecision)) {
      result.suggestedDecision = obj.suggestedDecision;
    }
    return result;
  } catch {
    return null;
  }
}

export function buildPrefillHref(payload: RuleBuilderPrefill): string {
  return `/rules?prefill=${encodePrefill(payload)}`;
}
