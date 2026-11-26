import { useEffect } from "react";

type E2eWindow = Window & { __e2eTriggerInputDropped?: (count: number) => void };

let allowed: Promise<boolean> | undefined;

/** One request per page load; only the e2e server sends `e2e_hooks`, so any failure means "no". */
function serverAllowsE2eHooks(): Promise<boolean> {
  allowed ??= (async () => {
    try {
      const info = (await (await fetch("/api/server-info")).json()) as { e2e_hooks?: boolean };
      return info.e2e_hooks === true;
    } catch {
      return false;
    }
  })();
  return allowed;
}

/** Exposes `window.__e2eTriggerInputDropped` for tests/e2e/input-drop-badge.spec.ts, on the e2e server only. */
export function useE2eInputDropTrigger(report: (count: number) => void): void {
  useEffect(() => {
    let cancelled = false;
    const target = window as E2eWindow;
    void serverAllowsE2eHooks().then((ok) => {
      if (ok && !cancelled) target.__e2eTriggerInputDropped = report;
    });
    return () => {
      cancelled = true;
      if (target.__e2eTriggerInputDropped === report) delete target.__e2eTriggerInputDropped;
    };
  }, [report]);
}
