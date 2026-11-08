"use client";

import { useEffect, useState } from "react";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import { loadFlags, peekFlags, subscribeFlags } from "./flagStore";
import { resolveFlagValue } from "@/lib/hooks/useFeatureFlagValue";

export const SESSION_MULTI_PR_FLAG = "session_multi_pr";

/** Reader scopes in dependency order; a reader is effective only when every earlier one is (config/session_multi_pr_flag.go). */
export const SESSION_MULTI_PR_READER_ORDER = ["reader:poller", "reader:backlog", "reader:actions", "reader:ui"] as const;
export type SessionMultiPrScope = (typeof SESSION_MULTI_PR_READER_ORDER)[number];

/** Advisory client read of `SessionMultiPR(scope)`: the scope and every earlier reader are on. */
export function resolveSessionMultiPr(flags: FeatureFlag[], scope: SessionMultiPrScope): boolean {
  const upTo = SESSION_MULTI_PR_READER_ORDER.indexOf(scope);
  return SESSION_MULTI_PR_READER_ORDER.slice(0, upTo + 1).every((s) => resolveFlagValue(flags, SESSION_MULTI_PR_FLAG, s) === true);
}

/**
 * Whether the multi-PR UI (`reader:ui`) is on. Off until the flags load and when the read fails, so a
 * failed read renders the legacy single badge. Re-reads with the shared flag store, so a flip reaches
 * an open page without a reload (UX-S10-1).
 */
export function useSessionMultiPR(scope: SessionMultiPrScope = "reader:ui"): boolean {
  const [on, setOn] = useState(() => {
    const cached = peekFlags();
    return cached ? resolveSessionMultiPr(cached, scope) : false;
  });

  useEffect(() => {
    let live = true;
    const apply = (flags: FeatureFlag[]) => { if (live) setOn(resolveSessionMultiPr(flags, scope)); };
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
  }, [scope]);

  return on;
}
