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
  // When true, this plugin authenticates via the shared, host-keyed GitHub
  // keychain (Settings > GitHub Accounts) instead of a per-source token field.
  // BacklogSourcesSettings.tsx renders a link to that page instead of a
  // token input when this is set.
  credentialsManagedExternally?: boolean;
  // With credentialsManagedExternally, replaces the GitHub-accounts link for
  // plugins whose credentials come from the server's keychain/environment.
  credentialsHelp?: string;
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
    credentialsManagedExternally: true,
  },
  {
    id: "linear",
    label: "Linear",
    fields: [{ key: "team_key", label: "Team Key", placeholder: "Team key (e.g. ENG)" }],
    requiresToken: true,
    tokenLabel: "Linear API key",
    credentialsManagedExternally: true,
    credentialsHelp: "Credentials are read from the server's keychain or the LINEAR_API_KEY environment variable.",
  },
  {
    id: "jira",
    label: "JIRA",
    fields: [
      { key: "base_url", label: "Base URL", placeholder: "Base URL (e.g. https://acme.atlassian.net)" },
      { key: "project_key", label: "Project Key", placeholder: "Project key (e.g. ENG)" },
    ],
    requiresToken: true,
    tokenLabel: "JIRA API token",
    credentialsManagedExternally: true,
    credentialsHelp:
      "Credentials are read from the server's keychain or the JIRA_BASE_URL, JIRA_EMAIL and JIRA_API_TOKEN environment variables.",
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
    credentialsManagedExternally: true,
  },
];
