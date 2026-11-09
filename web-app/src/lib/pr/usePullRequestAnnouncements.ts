"use client";

import { useEffect, useRef, useState } from "react";
import type { PrHold, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { holdsFor, prKey, prRowState } from "./pullRequests";

/** Coalescing window for the polite status region (ux.md S10). */
export const ANNOUNCE_DEBOUNCE_MS = 2000;

const STATE_WORDS = { open: "open", draft: "a draft", merged: "merged", closed: "closed", unknown: "being checked", unverified: "closed or merged (unverified)" } as const;

/**
 * Text for a `role="status"` region: "PR #12 is now merged" when a row's state changes and
 * "Hold: <kind> #23" lines the first time a distinct hold appears. Never speaks on the initial
 * render, coalesces changes inside ANNOUNCE_DEBOUNCE_MS into one message, and does not repeat a hold that
 * persists (a hold that disappears and returns is announced again).
 */
export function usePullRequestAnnouncements(
  prs: readonly SessionPullRequest[],
  holds: readonly PrHold[],
  holdText: (hold: PrHold, pr: SessionPullRequest | undefined) => string,
): string {
  const [message, setMessage] = useState("");
  const seen = useRef<{ states: Map<string, string>; holds: Set<string> } | null>(null);
  const pending = useRef<string[]>([]);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const holdTextRef = useRef(holdText);
  holdTextRef.current = holdText;

  useEffect(() => {
    const states = new Map(prs.map((p) => [prKey(p), prRowState(p)]));
    const activeHolds = new Map<string, string>();
    for (const h of holds) {
      const pr = prs.find((p) => holdsFor([h], p).length > 0);
      activeHolds.set(`${h.kind}:${h.pr ? prKey(h.pr) : ""}`, holdTextRef.current(h, pr));
    }
    const prev = seen.current;
    seen.current = { states, holds: new Set(activeHolds.keys()) };
    if (!prev) return;

    for (const p of prs) {
      const before = prev.states.get(prKey(p));
      const now = states.get(prKey(p));
      if (before && now && before !== now) pending.current.push(`PR #${p.number} is now ${STATE_WORDS[now]}`);
    }
    for (const [key, text] of activeHolds) if (!prev.holds.has(key) && text) pending.current.push(text);

    if (pending.current.length > 0 && !timer.current) {
      timer.current = setTimeout(() => {
        setMessage(pending.current.join(". "));
        pending.current = [];
        timer.current = null;
      }, ANNOUNCE_DEBOUNCE_MS);
    }
  }, [prs, holds]);

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  return message;
}
