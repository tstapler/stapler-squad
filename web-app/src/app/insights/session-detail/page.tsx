// +feature: insights-session-detail-route
"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { SessionDetailPageClient } from "./SessionDetailPageClient";

/**
 * Durable standalone route for a session's detail view (Epic 1.4, Story
 * 1.4.3), replacing the original `/insights/session/[sessionId]/` dynamic
 * route. This app builds with `output: "export"` (static export embedded in
 * the Go binary — see next.config.ts), and a dynamic path segment has no
 * pre-renderable params at build time since session IDs aren't known until
 * runtime: a cold navigation straight to `/insights/session/<id>` (bookmark,
 * refresh, shared link — bypassing the already-loaded SPA router) rendered
 * the root dashboard instead of the session page, defeating the point of a
 * "bookmarkable route".
 *
 * Uses a `?sessionId=` query param instead, mirroring the identical fix
 * already established at `/sessions/summary?sessionId=`
 * (src/app/sessions/summary/page.tsx, Epic 3.3): a query param resolves
 * entirely client-side via `useSearchParams()` against the same static
 * HTML/JS bundle on cold load, with no dynamic path segment involved.
 */
function SessionDetailRouteInner() {
  const searchParams = useSearchParams();
  const sessionId = searchParams.get("sessionId") ?? "";

  return <SessionDetailPageClient sessionId={sessionId} />;
}

export default function SessionDetailRoute() {
  return (
    <Suspense>
      <SessionDetailRouteInner />
    </Suspense>
  );
}
