"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { ListTaggingRulesRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

/**
 * Module-level shared cache: SessionCard renders one instance of
 * useTaggingRuleNames per card, potentially dozens at once in a session
 * list. Sharing a single in-flight/resolved fetch (instead of one RPC per
 * card) is a cheap dedup that costs nothing in correctness — rule names
 * change rarely, and this hook is a tooltip nicety, not a source of truth
 * any save path depends on.
 */
let cachedNamesPromise: Promise<Record<string, string>> | null = null;

/**
 * Mounted `useTaggingRuleNames` instances register a refetch callback here so
 * `invalidateTaggingRuleNamesCache` can push fresh data to them directly —
 * without this, invalidation would only affect the *next* mount (empty-deps
 * `useEffect`), leaving already-mounted SessionCards stuck on stale names.
 */
const cacheListeners = new Set<() => void>();

async function fetchRuleNames(): Promise<Record<string, string>> {
  try {
    const client = createClient(SessionService, getConnectTransport());
    // This fetch is a module-level cache shared across every mounted SessionCard
    // (see cachedNamesPromise above) — aborting it on any one card's unmount
    // would break the fetch for every other still-mounted consumer.
    // abort-signal-exempt
    const resp = await client.listTaggingRules(create(ListTaggingRulesRequestSchema, {}));
    const names: Record<string, string> = {};
    for (const rule of resp.rules ?? []) {
      names[rule.id] = rule.name;
    }
    return names;
  } catch (err) {
    console.error("Failed to fetch tagging rule names:", err);
    // Don't cache a failed fetch: a transient RPC error would otherwise leave
    // every tooltip blank for the rest of the page's life with no retry.
    cachedNamesPromise = null;
    return {};
  }
}

/** Resets the module-level cache — for tests only. */
export function _resetTaggingRuleNamesCacheForTesting(): void {
  cachedNamesPromise = null;
}

/**
 * Invalidates the module-level rule-names cache so the next render refetches
 * fresh data. Call after a tagging rule mutation (upsert/delete) so already-
 * mounted SessionCards' provenance tooltips pick up the new name/removal
 * instead of showing stale data until a full page reload.
 */
export function invalidateTaggingRuleNamesCache(): void {
  cachedNamesPromise = null;
  for (const listener of cacheListeners) listener();
}

/**
 * Returns a map of TaggingRule.ID -> rule name, for resolving a tag's
 * provenance (`RuleTagProvenance[tag]`, a rule ID or the "llm" sentinel) into
 * human-readable tooltip text (ux.md Surface 1). Falls back to an empty map
 * on any fetch failure — callers should treat an unresolved ID the same as a
 * deleted rule (ux.md's "originating rule no longer exists" edge case).
 */
export function useTaggingRuleNames(): Record<string, string> {
  const [names, setNames] = useState<Record<string, string>>({});

  useEffect(() => {
    let cancelled = false;

    const applyResolved = (resolved: Record<string, string>) => {
      if (cancelled) return;
      setNames(resolved);
    };

    const load = () => {
      if (!cachedNamesPromise) {
        cachedNamesPromise = fetchRuleNames();
      }
      cachedNamesPromise.then(applyResolved);
    };

    load();
    cacheListeners.add(load);
    return () => {
      cancelled = true;
      cacheListeners.delete(load);
    };
  }, []);

  return names;
}
