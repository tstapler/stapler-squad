/**
 * Centralized route definitions for type-safe navigation
 */

export const routes = {
  home: "/",
  sessionCreate: "/?new=true",
  reviewQueue: "/review-queue",
  unfinished: "/unfinished",
  rules: "/rules",
  history: "/history",
  logs: "/logs",
  errors: "/errors",
  config: "/config",
  notifications: "/notifications",
  settings: "/settings",
  settingsDefaults: "/settings/defaults",
  help: "/help",
  settingsUnfinished: "/settings/unfinished",
  insights: "/insights",
  settingsFeatures: "/settings/features",
  settingsBacklogSources: "/settings/backlog-sources",
  settingsPipelineModes: "/settings/pipeline-modes",
  settingsRemotes: "/settings/remotes",
  backlog: "/backlog",
  backlogBoard: "/backlog/board",
  sessionsImport: "/sessions/import",
  workflows: "/workflows",
  triggers: "/triggers",
  login: "/login",
  account: "/account",
  escapeAnalytics: "/analytics/escape",
  files: "/files",
  sessionDetail: (id: string) => `/?session=${id}`,
  newSessionFromWorktree: (worktreePath: string, branch: string, title?: string) => {
    const params = new URLSearchParams({ worktree: worktreePath, branch });
    if (title) params.set("title", title);
    return `/?${params.toString()}`;
  },
  unfinishedItem: (itemId: string) => `/unfinished?item=${encodeURIComponent(itemId)}`,
} as const;

export type Route = typeof routes;
