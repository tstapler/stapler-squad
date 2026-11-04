"use client";

import { useEffect, useState } from "react";
import { loadFlags, peekFlags, resetFlagStoreForTest, subscribeFlags } from "@/lib/features/flagStore";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";

/** Operator copy for the FLAG_OFF action outcome (UX-AC55, UX-AC57); Story 2.5's ActionRow maps the outcome to it. */
export const flagOffMessage = "This action is turned off. Open session.";

/** Clears the module cache (tests, and after a flag flip a caller knows about). */
export const resetFeatureFlagValueCache = resetFlagStoreForTest;

/**
 * Advisory read of one decision point: the scope's explicit value, else the
 * global value. Dependencies (render before answer, ...) are applied by the
 * server on every write; this only chooses which row to draw.
 */
export function resolveFlagValue(flags: FeatureFlag[], name: string, scope?: string): boolean | undefined {
  const flag = flags.find((f) => f.name === name);
  if (!flag) return undefined;
  const override = scope ? flag.scopes.find((s) => s.scope === scope) : undefined;
  return override ? override.enabled : flag.enabled;
}

/**
 * Reads a feature flag (optionally at a scope) through GetFeatureFlags, with a
 * short shared cache. Until loaded, and when the first read fails, it returns
 * `fallback` (default OFF), so a failed read never enables a new path. It re-reads
 * every cache TTL (one shared store timer) so a kill switch reaches an open page; a failed re-read keeps the
 * last known value rather than reverting to `fallback`.
 * `enabled=false` skips the read (the row has nothing the flag could change).
 */
export function useFeatureFlagValue(name: string, scope?: string, fallback = false, enabled = true): boolean {
  const [value, setValue] = useState<boolean>(fallback);

  useEffect(() => {
    if (!enabled) return;
    let live = true;
    const apply = (flags: FeatureFlag[]) => {
      if (live) setValue(resolveFlagValue(flags, name, scope) ?? fallback);
    };
    // Cheap when the cache is fresh; after a recorded write the cache is stale, so this refetches once for all consumers.
    const read = () => void loadFlags(Date.now).then(apply).catch(() => {});
    const unsubscribe = subscribeFlags(() => {
      const last = peekFlags();
      if (last) apply(last);
      read();
    });
    read();
    return () => {
      live = false;
      unsubscribe();
    };
  }, [name, scope, fallback, enabled]);

  return value;
}
