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
    return {};
  }
}

/** Resets the module-level cache — for tests only. */
export function _resetTaggingRuleNamesCacheForTesting(): void {
  cachedNamesPromise = null;
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
    if (!cachedNamesPromise) {
      cachedNamesPromise = fetchRuleNames();
    }
    cachedNamesPromise.then((resolved) => {
      if (!cancelled) setNames(resolved);
    });
    return () => { cancelled = true; };
  }, []);

  return names;
}
