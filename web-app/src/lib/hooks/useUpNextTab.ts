"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  readStoredTab,
  tabFromUrl,
  writeStoredTab,
  type UpNextTab,
} from "@/lib/unfinished/upNextTab";
import { emitTabEvent } from "@/lib/unfinished/tabStats";

// useLayoutEffect warns during SSR/static export; fall back to useEffect there.
const useIsomorphicLayoutEffect = typeof window !== "undefined" ? useLayoutEffect : useEffect;

export interface UseUpNextTab {
  tab: UpNextTab;
  setTab: (tab: UpNextTab) => void;
}

/**
 * Tab state: URL (`?item=`, `?tab=`) wins and is read synchronously on first render; only the
 * localStorage fallback is read post-mount (layout effect, before paint) so SSR/hydration agree.
 * Only user clicks write storage; deep links never do.
 */
export function useUpNextTab(now: () => number = () => performance.now()): UseUpNextTab {
  const router = useRouter();
  const searchParams = useSearchParams();
  const urlTab = tabFromUrl({ item: searchParams.get("item"), tab: searchParams.get("tab") });
  const [storedTab, setStoredTab] = useState<UpNextTab | null>(null);
  const tab = urlTab ?? storedTab ?? "prs";

  const tabRef = useRef(tab);
  tabRef.current = tab;
  const visitStartRef = useRef<number | null>(null);
  const clickedRef = useRef(false);

  useIsomorphicLayoutEffect(() => {
    const stored = urlTab === null ? readStoredTab() : null;
    if (stored) setStoredTab(stored);
    visitStartRef.current = now();
    emitTabEvent({ type: "visit", landedOn: urlTab ?? stored ?? "prs" });
    // Mount-only: later URL changes are handled by derivation, not re-reading storage.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const setTab = useCallback(
    (next: UpNextTab) => {
      emitTabEvent({
        type: "click",
        from: tabRef.current,
        to: next,
        msSinceVisit: now() - (visitStartRef.current ?? now()),
        firstClickOfVisit: !clickedRef.current,
      });
      clickedRef.current = true;
      writeStoredTab(next);
      setStoredTab(next);
      const params = new URLSearchParams(searchParams.toString());
      params.delete("item");
      params.set("tab", next);
      router.replace(`?${params.toString()}`, { scroll: false });
    },
    [router, searchParams, now],
  );

  return { tab, setTab };
}
