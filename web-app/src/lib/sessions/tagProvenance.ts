/**
 * Shared tag-provenance helpers (ux.md Surfaces 1–3, Epic 6.2). A tag's entry
 * in `Session.ruleTagProvenance` is either a `TaggingRule.ID`, the sentinel
 * "llm" (LLM-classifier-derived), or absent (a plain user-added tag).
 */

/** Sentinel value the LLM-fallback poller uses when classification fails/times out. */
export const UNCLASSIFIED_TAG = "Unclassified";

/** Sentinel provenance value for LLM-classifier-derived tags (session.Instance's "llm" key). */
export const LLM_PROVENANCE_SENTINEL = "llm";

/** Tooltip text for the transient Unclassified sentinel pill (ux.md AC8). */
export const UNCLASSIFIED_TOOLTIP = "LLM classification failed or timed out — will retry next poll cycle";

/**
 * Returns the rule-provenance display name for `tag`, or null if it has no
 * provenance entry (a plain user tag). Degrades to a generic name when the
 * provenance rule ID doesn't resolve (deleted rule) — never returns
 * undefined/blank (ux.md's explicit edge case for both Surface 1 and 3).
 */
export function resolveProvenanceRuleName(
  tag: string,
  provenance: Record<string, string> | undefined,
  ruleNames: Record<string, string>
): string | null {
  const entry = provenance?.[tag];
  if (!entry) return null;
  if (entry === LLM_PROVENANCE_SENTINEL) return null; // handled separately — not a rule name
  return ruleNames[entry] ?? null;
}

/** True if `tag` has any provenance entry (rule- or LLM-derived) — i.e. not a plain user tag. */
export function hasProvenance(tag: string, provenance: Record<string, string> | undefined): boolean {
  return !!provenance?.[tag];
}

/** True if `tag`'s provenance entry is the LLM sentinel. */
export function isLlmProvenance(tag: string, provenance: Record<string, string> | undefined): boolean {
  return provenance?.[tag] === LLM_PROVENANCE_SENTINEL;
}

/** `title` tooltip text for a tag pill (ux.md Surface 1). Undefined for plain user tags. */
export function tagProvenanceTitle(
  tag: string,
  provenance: Record<string, string> | undefined,
  ruleNames: Record<string, string>
): string | undefined {
  if (tag === UNCLASSIFIED_TAG) return UNCLASSIFIED_TOOLTIP;
  const entry = provenance?.[tag];
  if (!entry) return undefined;
  if (entry === LLM_PROVENANCE_SENTINEL) return "Applied by AI classification";
  const ruleName = ruleNames[entry];
  return ruleName
    ? `Applied by rule: ${ruleName}`
    : "Applied automatically — originating rule no longer exists";
}

/** Accessible name for a tag pill (ux.md AC2/AC4). Manual tags get the plain `Tag: {name}` form. */
export function tagProvenanceAriaLabel(
  tag: string,
  provenance: Record<string, string> | undefined
): string {
  if (tag === UNCLASSIFIED_TAG) return `Tag: ${tag} (auto-applied, will retry)`;
  const entry = provenance?.[tag];
  if (!entry) return `Tag: ${tag}`;
  return entry === LLM_PROVENANCE_SENTINEL
    ? `Tag: ${tag} (auto-applied by AI)`
    : `Tag: ${tag} (auto-applied by rule)`;
}

/**
 * The "may reappear" removal-confirmation text for a rule-provenance tag
 * (ux.md Surface 3, AC11) — falls back to generic wording when the rule name
 * can't be resolved, never blank/undefined.
 */
export function removalConfirmationText(
  tag: string,
  provenance: Record<string, string> | undefined,
  ruleNames: Record<string, string>
): string {
  const entry = provenance?.[tag];
  if (entry && entry !== LLM_PROVENANCE_SENTINEL) {
    const ruleName = ruleNames[entry];
    if (ruleName) return `Applied by rule "${ruleName}" — may reappear.`;
  }
  return "This tag was applied automatically and may reappear — remove anyway?";
}
