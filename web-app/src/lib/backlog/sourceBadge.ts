// Per-tracker presentation for a backlog item's source badge. Unknown or empty
// plugin ids fall back to GitHub, which is what every pre-plugin_id item was.
export interface SourceBadgeConfig {
  label: string;
  /** Prefix for the external id in the badge text ("#" for GitHub issue numbers, none for ENG-123). */
  idPrefix: string;
  openLabel: string;
}

export const SOURCE_BADGE_CONFIG: Record<string, SourceBadgeConfig> = {
  github_issues: { label: "GitHub", idPrefix: "#", openLabel: "Open on GitHub" },
  linear: { label: "Linear", idPrefix: "", openLabel: "Open in Linear" },
  jira: { label: "JIRA", idPrefix: "", openLabel: "Open in JIRA" },
};

export function getSourceBadgeConfig(pluginId: string | undefined): SourceBadgeConfig {
  return SOURCE_BADGE_CONFIG[pluginId ?? ""] ?? SOURCE_BADGE_CONFIG.github_issues;
}

/** Plugin ids offered in the source filter (issue trackers only). */
export const FILTERABLE_SOURCE_IDS = ["github_issues", "linear", "jira"] as const;
