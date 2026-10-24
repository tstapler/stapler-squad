"use client";

import type { ReactNode } from "react";
import { usePathname } from "@/lib/nav";
import { RequireFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { ROUTE_META, type GuardFlag, type RouteMetaEntry } from "./route-meta";

/**
 * Renders nothing gated while flags load, and `router.replace("/")` once if `flag` is off.
 * The single guard for the Next layouts today and the router table later; `RouteMeta.guard`
 * names the flag for each guarded path.
 *
 * While nothing is rendered Next's metadata (inside `children`) is dropped from the prerendered
 * head, which fails axe `document-title`; the fallback re-emits the route's `<title>` from
 * `ROUTE_META`, the table `route-meta.test.ts` keeps equal to the effective Next title.
 */
export function FeatureGuard({ flag, children }: { flag: GuardFlag; children: ReactNode }) {
  const pathname = usePathname();
  const title = (ROUTE_META as Record<string, RouteMetaEntry | undefined>)[pathname.replace(/(.)\/$/, "$1")]?.title;
  return (
    <RequireFeatureFlag flag={flag} fallback={title ? <title>{title}</title> : null}>
      {children}
    </RequireFeatureFlag>
  );
}
