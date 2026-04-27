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
  config: "/config",
  settings: "/settings",
  settingsDefaults: "/settings/defaults",
  settingsUnfinished: "/settings/unfinished",
  login: "/login",
  account: "/account",
  sessionDetail: (id: string) => `/sessions/${id}`,
} as const;

export type Route = typeof routes;
