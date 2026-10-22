import type { ComponentType, ReactNode } from "react";
import type { AppRoute, PageRoute } from "@/lib/nav/app-types";
import type { RoutePath } from "@/lib/nav/paths";
import { FeatureGuard } from "./FeatureGuard";
import { routeMetaFor } from "./route-meta";

type RouteDef =
  | { load: () => Promise<{ default: ComponentType }> }
  | { redirect: string; keepQuery: boolean };

/** One entry per `page.tsx`; a missing key fails `tsc`, and `routes.test.tsx` checks it against the directory. */
export const routeTable = {
  "/": { load: () => import("./page") },
  "/account": { load: () => import("./account/page") },
  "/analytics/escape": { load: () => import("./analytics/escape/page") },
  "/backlog": { load: () => import("./backlog/page") },
  "/backlog/board": { load: () => import("./backlog/board/page") },
  "/changelog": { load: () => import("./changelog/page") },
  "/config": { redirect: "/settings?tab=config-files", keepQuery: false },
  "/debug/escape-codes": { load: () => import("./debug/escape-codes/page") },
  "/errors": { load: () => import("./errors/page") },
  "/fanout": { load: () => import("./fanout/page") },
  "/files": { load: () => import("./files/page") },
  "/help": { load: () => import("./help/page") },
  "/history": { load: () => import("./history/page") },
  "/insights": { load: () => import("./insights/page") },
  "/insights/session-detail": { load: () => import("./insights/session-detail/page") },
  "/login": { load: () => import("./login/page") },
  "/logs": { load: () => import("./logs/page") },
  "/notifications": { load: () => import("./notifications/page") },
  "/resolve": { load: () => import("./resolve/page") },
  "/review-queue": { load: () => import("./review-queue/page") },
  "/rules": { load: () => import("./rules/page") },
  "/sessions/import": { load: () => import("./sessions/import/page") },
  "/sessions/new": { load: () => import("./sessions/new/page") },
  "/sessions/summary": { load: () => import("./sessions/summary/page") },
  "/settings": { load: () => import("./settings/page") },
  "/settings/background-models": { load: () => import("./settings/background-models/page") },
  "/settings/backlog-sources": { load: () => import("./settings/backlog-sources/page") },
  "/settings/backlog-stages": { load: () => import("./settings/backlog-stages/page") },
  "/settings/defaults": { redirect: "/settings", keepQuery: false },
  "/settings/features": { load: () => import("./settings/features/page") },
  "/settings/jules": { load: () => import("./settings/jules/page") },
  "/settings/llm-backends": { load: () => import("./settings/llm-backends/page") },
  "/settings/pipeline-modes": { load: () => import("./settings/pipeline-modes/page") },
  "/settings/remotes": { load: () => import("./settings/remotes/page") },
  "/settings/tagging-classifier": { load: () => import("./settings/tagging-classifier/page") },
  "/settings/unfinished": { load: () => import("./settings/unfinished/page") },
  "/spike-files": { load: () => import("./spike-files/page") },
  "/spike-lazydoc": { load: () => import("./spike-lazydoc/page") },
  "/test/escape-codes": { load: () => import("./test/escape-codes/page") },
  "/test/layout-overlap": { load: () => import("./test/layout-overlap/page") },
  "/test/terminal-stress": { load: () => import("./test/terminal-stress/page") },
  "/triggers": { load: () => import("./triggers/page") },
  "/unfinished": { load: () => import("./unfinished/page") },
  "/workflows": { load: () => import("./workflows/page") },
} satisfies Record<RoutePath, RouteDef>;

function guardFor(path: RoutePath): PageRoute["wrap"] {
  const flag = routeMetaFor(path)?.guard;
  if (!flag) return undefined;
  const Guarded = ({ children }: { children: ReactNode }) => <FeatureGuard flag={flag}>{children}</FeatureGuard>;
  return Guarded;
}

/** Titles and guards come from `RouteMeta`, so they cannot drift from the Next layouts the drift test checks. */
export const appRoutes: AppRoute[] = (Object.entries(routeTable) as [RoutePath, RouteDef][]).map(([path, def]) =>
  "load" in def ? { path, load: def.load, wrap: guardFor(path) } : { path, ...def },
);
