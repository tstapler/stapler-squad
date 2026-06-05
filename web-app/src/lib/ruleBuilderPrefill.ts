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

export function decodePrefill(encoded: string): RuleBuilderPrefill | null {
  try {
    const obj = JSON.parse(atob(encoded));
    if (typeof obj !== "object" || obj === null) return null;
    return obj as RuleBuilderPrefill;
  } catch {
    return null;
  }
}

export function buildPrefillHref(payload: RuleBuilderPrefill): string {
  return `/rules?prefill=${encodePrefill(payload)}`;
}
