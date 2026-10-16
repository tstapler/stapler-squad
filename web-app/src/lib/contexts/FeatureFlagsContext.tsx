"use client";

import { createContext, useContext, useState, useEffect, useCallback, useMemo, ReactNode } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { FlagMutation, SessionService } from "@/gen/session/v1/session_pb";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import { GLOBAL_SCOPE } from "./featureFlagScopes";

export interface FeatureFlagMeta {
  name: string;
  enabled: boolean;
  description: string;
  statusDetail: string;
  /** Explicit per-scope values read back from the server ("kind:review" -> true). */
  scopes?: Record<string, boolean>;
}

/**
 * A scoped or mutation-style flag change. `scope` is "global" for the global
 * value or "kind:<name>"; a bare boolean passed to setFlag is the legacy
 * global set.
 */
export type FlagChange =
  | { mutation: "set"; scope: string; enabled: boolean }
  | { mutation: "clear"; scope: string }
  | { mutation: "reset" };


function scopesOf(flag: FeatureFlag): Record<string, boolean> {
  return Object.fromEntries(flag.scopes.map((s) => [s.scope, s.enabled]));
}

function metaOf(f: FeatureFlag): FeatureFlagMeta {
  return {
    name: f.name,
    enabled: f.enabled,
    description: f.description,
    statusDetail: f.statusDetail,
    scopes: scopesOf(f),
  };
}

/** What the server must have persisted for `change`, or null when the read-back agrees. */
export function readbackMismatch(change: FlagChange, flag: FeatureFlag): string | null {
  if (change.mutation === "reset") return null;
  if (change.scope === GLOBAL_SCOPE) {
    return change.mutation === "set" && flag.enabled !== change.enabled ? "global value" : null;
  }
  const entry = flag.scopes.find((s) => s.scope === change.scope);
  if (change.mutation === "clear") return entry ? change.scope : null;
  return entry?.enabled === change.enabled ? null : change.scope;
}

interface FeatureFlagsContextValue {
  flags: Record<string, boolean>;
  flagList: FeatureFlagMeta[];
  isLoading: boolean;
  error: string | null;
  setFlag: (name: string, change: boolean | FlagChange) => Promise<void>;
}

const FeatureFlagsContext = createContext<FeatureFlagsContextValue>({
  flags: {},
  flagList: [],
  isLoading: true,
  error: null,
  setFlag: async () => {},
});

export function FeatureFlagsProvider({ children }: { children: ReactNode }) {
  const [flags, setFlags] = useState<Record<string, boolean>>({});
  const [flagList, setFlagList] = useState<FeatureFlagMeta[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const client = useMemo(
    () => createClient(SessionService, getConnectTransport()),
    []
  );

  const fetchFlags = useCallback(async () => {
    try {
      const res = await client.getFeatureFlags({});
      const map: Record<string, boolean> = {};
      const list: FeatureFlagMeta[] = [];
      for (const f of res.flags) {
        map[f.name] = f.enabled;
        list.push(metaOf(f));
      }
      setFlags(map);
      setFlagList(list);
    } catch {
      setError("Failed to load feature flags");
    } finally {
      setIsLoading(false);
    }
  }, [client]);

  useEffect(() => { fetchFlags(); }, [fetchFlags]);

  // The displayed state is always the server's read-back, never an optimistic
  // value: a failed or ignored change leaves the previous state and re-reads it.
  const setFlag = useCallback(async (name: string, change: boolean | FlagChange) => {
    const req =
      typeof change === "boolean"
        ? { name, enabled: change }
        : change.mutation === "set"
          ? {
              name,
              scope: change.scope,
              mutation: change.enabled ? FlagMutation.SET_ENABLED : FlagMutation.SET_DISABLED,
            }
          : change.mutation === "clear"
            ? { name, scope: change.scope, mutation: FlagMutation.CLEAR_SCOPE }
            : { name, mutation: FlagMutation.RESET_GLOBAL };
    try {
      const res = await client.updateFeatureFlag(req);
      const flag = res.flag;
      if (!flag) throw new Error("response carried no flag");
      if (typeof change !== "boolean") {
        const bad = readbackMismatch(change, flag);
        if (bad) {
          // An older server that ignores `scope` answers with success and no entry.
          throw new Error(`server read-back does not show the change for ${bad}`);
        }
      }
      setFlags((prev) => ({ ...prev, [flag.name]: flag.enabled }));
      setFlagList((prev) => prev.map((f) => (f.name === flag.name ? { ...f, ...metaOf(flag) } : f)));
    } catch (err) {
      console.error("Failed to update feature flag", name, err);
      setError("Failed to update feature flag");
      void fetchFlags();
    }
  }, [client, fetchFlags]);

  const value = useMemo(
    () => ({ flags, flagList, isLoading, error, setFlag }),
    [flags, flagList, isLoading, error, setFlag]
  );

  return (
    <FeatureFlagsContext.Provider value={value}>
      {children}
    </FeatureFlagsContext.Provider>
  );
}

export function useFeatureFlags() {
  return useContext(FeatureFlagsContext);
}

export function useFeatureFlag(name: string): boolean {
  const { flags } = useContext(FeatureFlagsContext);
  return flags[name] ?? false;
}

/**
 * Gates `children` behind a feature flag: renders nothing (and redirects to
 * `redirectTo`) while the flag is disabled or still loading; renders
 * `children` once the flag is confirmed enabled.
 *
 * Intentionally usable from more than one place in a route's render tree
 * (e.g. a segment's `layout.tsx` AND its `page.tsx`) — that's not
 * duplication, it's defense in depth: the layout keeps a disabled flag from
 * ever mounting the page's client bundle, and the same check inside the
 * page guarantees the page's own render output can never show gated content
 * even if it's ever rendered outside that layout (unit tests, future route
 * restructuring). Using a Server Component `page.tsx` to render this Client
 * Component also lets the page keep a static `metadata` export, which a
 * fully-client page cannot do.
 */
export function RequireFeatureFlag({
  flag,
  redirectTo = "/",
  children,
}: {
  flag: string;
  redirectTo?: string;
  children: ReactNode;
}) {
  const { flags, isLoading } = useFeatureFlags();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && !flags[flag]) {
      router.replace(redirectTo);
    }
  }, [isLoading, flags, flag, redirectTo, router]);

  if (isLoading) return null;
  if (!flags[flag]) return null;
  return <>{children}</>;
}
