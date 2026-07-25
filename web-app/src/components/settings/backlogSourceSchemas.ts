export interface SourceFieldSchema {
  key: string;
  label: string;
  placeholder: string;
}

export interface PluginSchema {
  id: string;
  label: string;
  fields: SourceFieldSchema[];
  requiresToken: boolean;
  tokenLabel: string;
}

// Adding a source type for a new plugin (e.g. Jira, Linear) means adding one entry
// here — the form in BacklogSourcesSettings.tsx renders fields from this schema
// instead of hardcoding owner/repo, so the UI stays in sync with the backend's
// pluggable ItemSourcePlugin registry (session/backlog_plugin.go) without
// per-plugin component changes. Kept in its own module (rather than inline in the
// component) so tests can mock it with a synthetic schema to prove the component
// is genuinely schema-driven rather than hardcoded.
export const PLUGIN_SCHEMAS: PluginSchema[] = [
  {
    id: "github_issues",
    label: "GitHub Issues",
    fields: [
      { key: "owner", label: "Owner", placeholder: "Owner (e.g. acme)" },
      { key: "repo", label: "Repo", placeholder: "Repo (e.g. widgets)" },
    ],
    requiresToken: true,
    tokenLabel: "GitHub personal access token",
  },
  {
    id: "github_prs",
    label: "GitHub Pull Requests",
    fields: [
      { key: "owner", label: "Owner", placeholder: "Owner (e.g. acme)" },
      { key: "repo", label: "Repo", placeholder: "Repo (e.g. widgets)" },
    ],
    requiresToken: true,
    tokenLabel: "GitHub personal access token",
  },
];
