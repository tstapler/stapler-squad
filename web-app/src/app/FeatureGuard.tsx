"use client";

import type { ReactNode } from "react";
import { RequireFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import type { GuardFlag } from "./route-meta";

/**
 * Renders nothing while flags load, and `router.replace("/")` once if `flag` is off.
 * The single guard for the Next layouts today and the router table later; `RouteMeta.guard`
 * names the flag for each guarded path.
 */
export function FeatureGuard({ flag, children }: { flag: GuardFlag; children: ReactNode }) {
  return <RequireFeatureFlag flag={flag}>{children}</RequireFeatureFlag>;
}
