/**
 * Centralized route definitions for type-safe navigation
 */

export const routes = {
  home: "/",
  sessionDetail: (id: string) => `/sessions/${id}`,
  sessionCreate: "/sessions/new",
  dashboard: "/dashboard",
  settings: "/settings",
} as const;

export type Route = typeof routes;
