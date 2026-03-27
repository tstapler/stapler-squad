/**
 * Centralized route definitions for type-safe navigation
 */

export const routes = {
  home: "/",
  sessionCreate: "/sessions/new",
  reviewQueue: "/review-queue",
  rules: "/rules",
  history: "/history",
  logs: "/logs",
  config: "/config",
  login: "/login",
  sessionDetail: (id: string) => `/sessions/${id}`,
} as const;

export type Route = typeof routes;
