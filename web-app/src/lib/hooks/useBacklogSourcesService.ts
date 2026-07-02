"use client";

import { useCallback, useRef, useEffect, useMemo, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BacklogService, ItemSource as ItemSourceProto, SourceSyncEvent as SourceSyncEventProto } from "@/gen/session/v1/backlog_pb";

export interface ItemSource {
  id: string;
  pluginId: string;
  displayName: string;
  enabled: boolean;
  tokenConfigured: boolean;
  lastSyncedAt?: string;
}

export interface SourceSyncEvent {
  id: string;
  startedAt?: string;
  finishedAt?: string;
  itemsCreated: number;
  itemsUpdated: number;
  itemsSkipped: number;
  itemsErrored: number;
  errorMessage?: string;
}

function tsToIso(ts?: { seconds: bigint }): string | undefined {
  return ts ? new Date(Number(ts.seconds) * 1000).toISOString() : undefined;
}

function mapItemSource(p: ItemSourceProto): ItemSource {
  return {
    id: p.id,
    pluginId: p.pluginId,
    displayName: p.displayName,
    enabled: p.enabled,
    tokenConfigured: p.tokenConfigured,
    lastSyncedAt: tsToIso(p.lastSyncedAt),
  };
}

function mapSyncEvent(p: SourceSyncEventProto): SourceSyncEvent {
  return {
    id: p.id,
    startedAt: tsToIso(p.startedAt),
    finishedAt: tsToIso(p.finishedAt),
    itemsCreated: p.itemsCreated,
    itemsUpdated: p.itemsUpdated,
    itemsSkipped: p.itemsSkipped,
    itemsErrored: p.itemsErrored,
    errorMessage: p.errorMessage || undefined,
  };
}

export interface CreateItemSourceInput {
  pluginId: string;
  displayName: string;
  configJson: string;
  token: string;
}

interface UseBacklogSourcesServiceReturn {
  listItemSources: () => Promise<ItemSource[]>;
  createItemSource: (data: CreateItemSourceInput) => Promise<ItemSource | null>;
  setItemSourceEnabled: (id: string, displayName: string, enabled: boolean) => Promise<ItemSource | null>;
  deleteItemSource: (id: string) => Promise<boolean>;
  triggerSync: (id: string) => Promise<boolean>;
  getSyncHistory: (id: string) => Promise<SourceSyncEvent[]>;
  lastError: Error | null;
  clearError: () => void;
}

export function useBacklogSourcesService(): UseBacklogSourcesServiceReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);
  const [lastError, setLastError] = useState<Error | null>(null);

  const clearError = useCallback(() => setLastError(null), []);

  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    clientRef.current = createClient(BacklogService, transport);
  }, []);

  const listItemSources = useCallback(async (): Promise<ItemSource[]> => {
    if (!clientRef.current) return [];
    try {
      const resp = await clientRef.current.listItemSources({});
      return (resp.sources ?? []).map(mapItemSource);
    } catch (err) {
      console.error("[useBacklogSourcesService] listItemSources:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      return [];
    }
  }, []);

  const createItemSource = useCallback(async (data: CreateItemSourceInput): Promise<ItemSource | null> => {
    if (!clientRef.current) return null;
    try {
      setLastError(null);
      const resp = await clientRef.current.createItemSource({
        pluginId: data.pluginId,
        displayName: data.displayName,
        configJson: data.configJson,
        token: data.token,
      });
      return resp.source ? mapItemSource(resp.source) : null;
    } catch (err) {
      console.error("[useBacklogSourcesService] createItemSource:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      return null;
    }
  }, []);

  const setItemSourceEnabled = useCallback(
    async (id: string, displayName: string, enabled: boolean): Promise<ItemSource | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.updateItemSource({
          sourceId: id,
          displayName,
          enabled,
          token: "",
        });
        return resp.source ? mapItemSource(resp.source) : null;
      } catch (err) {
        console.error("[useBacklogSourcesService] updateItemSource:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const deleteItemSource = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      setLastError(null);
      await clientRef.current.deleteItemSource({ sourceId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogSourcesService] deleteItemSource:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      return false;
    }
  }, []);

  const triggerSync = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      setLastError(null);
      await clientRef.current.triggerSync({ sourceId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogSourcesService] triggerSync:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      return false;
    }
  }, []);

  const getSyncHistory = useCallback(async (id: string): Promise<SourceSyncEvent[]> => {
    if (!clientRef.current) return [];
    try {
      const resp = await clientRef.current.getSyncHistory({ sourceId: id });
      return (resp.events ?? []).map(mapSyncEvent);
    } catch (err) {
      console.error("[useBacklogSourcesService] getSyncHistory:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      return [];
    }
  }, []);

  return useMemo(
    () => ({
      listItemSources,
      createItemSource,
      setItemSourceEnabled,
      deleteItemSource,
      triggerSync,
      getSyncHistory,
      lastError,
      clearError,
    }),
    [listItemSources, createItemSource, setItemSourceEnabled, deleteItemSource, triggerSync, getSyncHistory, lastError, clearError]
  );
}
