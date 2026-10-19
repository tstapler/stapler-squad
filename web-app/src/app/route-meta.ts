/**
 * One row per `page.tsx` under src/app: the effective document title and the
 * feature-flag guard (if any). `route-meta.test.ts` fails when this table
 * drifts from the Next `metadata` exports and layouts. When it does, add or fix
 * the row here.
 */

export type GuardFlag = "backlog" | "changelog";

export interface RouteMetaEntry {
  /** Effective title: the page's own `metadata.title`, else the nearest layout's, else the root's. */
  title: string;
  /** Feature flag that gates this path (and, via its layout, everything below it). */
  guard?: GuardFlag;
}

const ROOT_TITLE = "Stapler Squad Sessions";
const SETTINGS_TITLE = "Settings - Stapler Squad";

export const NOT_FOUND_META = {
  title: "Page not found - Stapler Squad",
} as const;

export const ROUTE_META: Record<string, RouteMetaEntry> = {
  "/": { title: ROOT_TITLE },
  "/account": { title: "Account - Stapler Squad" },
  "/analytics/escape": { title: "Escape Analytics - Stapler Squad" },
  "/backlog": { title: ROOT_TITLE, guard: "backlog" },
  "/backlog/board": { title: ROOT_TITLE, guard: "backlog" },
  "/changelog": { title: ROOT_TITLE, guard: "changelog" },
  "/config": { title: "Configuration - Stapler Squad" },
  "/debug/escape-codes": { title: ROOT_TITLE },
  "/errors": { title: "Errors - Stapler Squad" },
  "/fanout": { title: "Compare Variants - Stapler Squad" },
  "/files": { title: ROOT_TITLE },
  "/help": { title: ROOT_TITLE },
  "/history": { title: "History - Stapler Squad" },
  "/insights": { title: "Insights - Stapler Squad" },
  "/insights/session-detail": { title: ROOT_TITLE },
  "/login": { title: "Sign In - Stapler Squad" },
  "/logs": { title: "Logs - Stapler Squad" },
  "/notifications": { title: "Notifications - Stapler Squad" },
  "/resolve": { title: ROOT_TITLE },
  "/review-queue": { title: "Review Queue - Stapler Squad" },
  "/rules": { title: "Approval Rules - Stapler Squad" },
  "/sessions/import": { title: "Import External Sessions - Stapler Squad" },
  "/sessions/new": { title: ROOT_TITLE },
  "/sessions/summary": { title: ROOT_TITLE },
  "/settings": { title: SETTINGS_TITLE },
  "/settings/background-models": { title: "Background Models - Settings - Stapler Squad" },
  "/settings/backlog-sources": { title: "Backlog Sources - Settings - Stapler Squad", guard: "backlog" },
  "/settings/backlog-stages": { title: SETTINGS_TITLE },
  "/settings/defaults": { title: SETTINGS_TITLE },
  "/settings/features": { title: SETTINGS_TITLE },
  "/settings/jules": { title: "Jules - Settings - Stapler Squad" },
  "/settings/llm-backends": { title: "LLM Backends - Settings - Stapler Squad" },
  "/settings/pipeline-modes": { title: SETTINGS_TITLE },
  "/settings/remotes": { title: SETTINGS_TITLE },
  "/settings/tagging-classifier": { title: "Tag Classification - Settings - Stapler Squad" },
  "/settings/unfinished": { title: "Unfinished Sources - Settings - Stapler Squad" },
  "/test/escape-codes": { title: ROOT_TITLE },
  "/test/layout-overlap": { title: ROOT_TITLE },
  "/test/terminal-stress": { title: ROOT_TITLE },
  "/triggers": { title: "Triggers - Stapler Squad" },
  "/unfinished": { title: "Up Next - Stapler Squad" },
  "/workflows": { title: "Workflows - Stapler Squad" },
};
