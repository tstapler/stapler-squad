"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";

/** Operator copy for the FLAG_OFF action outcome (UX-AC55, UX-AC57); Story 2.5's ActionRow maps the outcome to it. */
export const flagOffMessage = "This action is turned off. Open session.";

const CACHE_TTL_MS = 30_000;

let cached: { at: number; flags: FeatureFlag[] } | null = null;
let inFlight: Promise<FeatureFlag[]> | null = null;

/** Clears the module cache (tests, and after a flag flip a caller knows about). */
export function resetFeatureFlagValueCache(): void {
  cached = null;
  inFlight = null;
}

async function loadFlags(now: () => number): Promise<FeatureFlag[]> {
  if (cached && now() - cached.at < CACHE_TTL_MS) return cached.flags;
  if (!inFlight) {
    const client = createClient(SessionService, getConnectTransport());
    inFlight = client
      .getFeatureFlags({})
      .then((res) => {
        cached = { at: now(), flags: res.flags };
        return res.flags;
      })
      .finally(() => {
        inFlight = null;
      });
  }
  return inFlight;
}

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
 * short shared cache. Until loaded, and when the read fails, it returns
 * `fallback` (default OFF), so a failed read never enables a new path.
 */
export function useFeatureFlagValue(name: string, scope?: string, fallback = false): boolean {
  const [value, setValue] = useState<boolean>(fallback);

  useEffect(() => {
    let live = true;
    loadFlags(Date.now)
      .then((flags) => {
        if (live) setValue(resolveFlagValue(flags, name, scope) ?? fallback);
      })
      .catch(() => {
        if (live) setValue(fallback);
      });
    return () => {
      live = false;
    };
  }, [name, scope, fallback]);

  return value;
}
