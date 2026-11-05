"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import {
  useWatchBacklogItems,
  type UseWatchBacklogItemsReturn,
} from "@/lib/hooks/useWatchBacklogItems";

const BacklogStreamContext = createContext<UseWatchBacklogItemsReturn | null>(null);

/**
 * Owns the single backlog WebSocket stream for everything under
 * `app/backlog/`, so list<->board navigation never tears it down and
 * re-establishes it.
 */
export function BacklogStreamProvider({ children }: { children: ReactNode }) {
  const { items, connectionState } = useWatchBacklogItems();
  const value = useMemo(() => ({ items, connectionState }), [items, connectionState]);
  return <BacklogStreamContext.Provider value={value}>{children}</BacklogStreamContext.Provider>;
}

/**
 * Reads the shared stream when rendered under {@link BacklogStreamProvider};
 * otherwise (session pages, isolated tests) opens its own. The provider's
 * presence is fixed for a component's lifetime, so the conditional hook call
 * never changes order between renders.
 */
export function useBacklogStream(): UseWatchBacklogItemsReturn {
  const shared = useContext(BacklogStreamContext);
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return shared ?? useWatchBacklogItems();
}
