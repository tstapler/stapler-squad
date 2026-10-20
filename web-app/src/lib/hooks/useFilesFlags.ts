"use client";

import { useCallback, useEffect, useRef } from "react";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";

export type FilesFlagsStatus = "loading" | "ready" | "error";

export interface FilesFlags {
  status: FilesFlagsStatus;
  editing: boolean;
  /** Latched at first ready so a mid-session flip never remounts the layout. */
  viewerV2: boolean;
  mergePhone: boolean;
  rangeRead: boolean;
  deltaWrite: boolean;
  editLarge: boolean;
  /** Call on a typed FlagOff refusal (WriteFile/SearchContent): re-reads the flags now. */
  onFlagOff: () => Promise<void>;
}

// Server defaults (server/services/feature_flag_service.go); editing and the ADR-036 flags fail closed.
const DEFAULTS = {
  editing: false, viewerV2: true, mergePhone: true, rangeRead: false, deltaWrite: false, editLarge: false,
} as const;

const REFRESH_THROTTLE_MS = 5000;

export function useFilesFlags(): FilesFlags {
  const { flags, isLoading, error, flagList, refresh } = useFeatureFlags();
  const status: FilesFlagsStatus = isLoading ? "loading" : error && flagList.length === 0 ? "error" : "ready";
  const lastRefreshAt = useRef(0);
  const latchedViewerV2 = useRef<boolean | null>(null);

  const refreshNow = useCallback(async () => {
    lastRefreshAt.current = Date.now();
    await refresh?.();
  }, [refresh]);

  useEffect(() => {
    const throttled = () => {
      if (document.visibilityState === "hidden") return;
      if (Date.now() - lastRefreshAt.current >= REFRESH_THROTTLE_MS) void refreshNow();
    };
    window.addEventListener("focus", throttled);
    document.addEventListener("visibilitychange", throttled);
    return () => {
      window.removeEventListener("focus", throttled);
      document.removeEventListener("visibilitychange", throttled);
    };
  }, [refreshNow]);

  const read = (name: string, fallback: boolean) => (status === "error" ? fallback : flags[name] ?? fallback);
  if (status === "ready" && latchedViewerV2.current === null) {
    latchedViewerV2.current = read("files:viewer-v2", DEFAULTS.viewerV2);
  }
  return {
    status,
    editing: read("files:editing", DEFAULTS.editing),
    viewerV2: latchedViewerV2.current ?? DEFAULTS.viewerV2,
    mergePhone: read("files:merge-phone", DEFAULTS.mergePhone),
    rangeRead: read("files:range-read", DEFAULTS.rangeRead),
    deltaWrite: read("files:delta-write", DEFAULTS.deltaWrite),
    editLarge: read("files:edit-large", DEFAULTS.editLarge),
    onFlagOff: refreshNow,
  };
}
