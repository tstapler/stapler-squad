import { parseExternalRef } from "@/components/backlog/externalRef";

// Per-tracker presentation for a backlog item's source badge. An empty plugin id
// falls back to GitHub (every pre-plugin_id item was a GitHub import); a
// non-empty unknown id gets a neutral "External" badge.
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

const EXTERNAL_BADGE_CONFIG: SourceBadgeConfig = { label: "External", idPrefix: "", openLabel: "Open externally" };

export function getSourceBadgeConfig(pluginId: string | undefined): SourceBadgeConfig {
  if (!pluginId) return SOURCE_BADGE_CONFIG.github_issues;
  return Object.hasOwn(SOURCE_BADGE_CONFIG, pluginId) ? SOURCE_BADGE_CONFIG[pluginId] : EXTERNAL_BADGE_CONFIG;
}

/**
 * The source id an item is badged and filtered under: its own plugin id when
 * known, github_issues for legacy imports (no plugin id, has an externalUrl),
 * otherwise undefined.
 */
export function resolveSourceId(item: { pluginId?: string; externalUrl?: string }): string | undefined {
  if (item.pluginId && Object.hasOwn(SOURCE_BADGE_CONFIG, item.pluginId)) return item.pluginId;
  return !item.pluginId && item.externalUrl ? "github_issues" : undefined;
}

/**
 * Display refs for an imported item: repo-qualified for GitHub URLs
 * ("acme/widgets#22"; short form drops the owner), bare id otherwise since
 * Linear/JIRA ids (ENG-123) are already unambiguous.
 */
export function buildExternalRefLabels(
  pluginId: string | undefined,
  externalUrl: string,
  externalId: string,
): { fullRef: string; shortRef: string } {
  const ref = pluginId && pluginId !== "github_issues" ? undefined : parseExternalRef(externalUrl);
  const fullRef = ref ? `${ref.repo}#${ref.number}` : `${getSourceBadgeConfig(pluginId).idPrefix}${externalId}`;
  const shortRef = ref ? `${ref.repo.split("/")[1]}#${ref.number}` : fullRef;
  return { fullRef, shortRef };
}

/** Plugin ids offered in the source filter (issue trackers only). */
export const FILTERABLE_SOURCE_IDS = ["github_issues", "linear", "jira"] as const;

export function isFilterableSourceId(id: unknown): boolean {
  return typeof id === "string" && (FILTERABLE_SOURCE_IDS as readonly string[]).includes(id);
}
