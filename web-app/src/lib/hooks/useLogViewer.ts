"use client";

import {
  useState,
  useRef,
  useCallback,
  useEffect,
  useMemo,
  startTransition,
} from "react";
import type { VirtuosoHandle } from "react-virtuoso";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
import { useLiveTail } from "./useLiveTail";
import { detectLevel } from "@/lib/logs/logParser";
import type { LogLevel } from "@/lib/logs/logParser";

export type { LogLevel };

export interface LogEntry {
  id: string;
  timestamp: string;
  level: LogLevel;
  message: string;
  raw: string;
}

export interface LogViewerState {
  logs: LogEntry[];
  isFollowing: boolean;
  searchQuery: string;
  setSearchQuery: (q: string) => void;
  levelFilters: string[];
  setLevelFilters: (l: string[]) => void;
  matchCount: number;
  totalCount: number;
  toggleRow: (i: number) => void;
  expandedRowIndex: number | null;
  selectedRowIndex: number | null;
  setSelectedRowIndex: (i: number | null) => void;
  jumpToLatest: () => void;
  queuedNewLineCount: number;
  onAtBottomStateChange: (atBottom: boolean) => void;
  virtuosoRef: React.RefObject<VirtuosoHandle | null>;
  liveTailEnabled: boolean;
  setLiveTailEnabled: (enabled: boolean) => void;
}

function mapLevel(raw: string): LogLevel {
  const upper = raw.toUpperCase();
  if (upper === "ERROR" || upper === "ERR") return "ERROR";
  if (upper === "WARN" || upper === "WARNING") return "WARN";
  if (upper === "INFO") return "INFO";
  if (upper === "DEBUG") return "DEBUG";
  if (upper === "TRACE") return "TRACE";
  return detectLevel(raw);
}

let entryCounter = 0;
function makeId() {
  return `log-${++entryCounter}`;
}

export function useLogViewer(
  source: "app" | "session",
  sessionId?: string,
): LogViewerState {
  // --- Core log storage: mutable ref + version counter for O(1) appending ---
  const logsRef = useRef<LogEntry[]>([]);
  const [version, setVersion] = useState(0);

  // --- Search and filter ---
  const [searchQuery, setSearchQuery] = useState("");
  const [levelFilters, setLevelFilters] = useState<string[]>([]);

  // --- Expansion ---
  const [expandedRowIndex, setExpandedRowIndex] = useState<number | null>(null);

  // --- Selection (keyboard row navigation) ---
  const [selectedRowIndex, setSelectedRowIndex] = useState<number | null>(null);

  // --- Live-tail pause state machine ---
  const [isFollowing, setIsFollowing] = useState(true);
  const [queuedNewLineCount, setQueuedNewLineCount] = useState(0);

  // --- Live-tail toggle ---
  const [liveTailEnabled, setLiveTailEnabled] = useState(true);

  // --- Virtuoso ref for programmatic scroll ---
  const virtuosoRef = useRef<VirtuosoHandle | null>(null);

  // --- ConnectRPC client ---
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  // --- Live-tail cursor: tracks server-side total_count so polling can detect new entries ---
  // GetLogs returns entries newest-first (DESC). Using offset:0 always yields the most-recent
  // entries. We compare total_count to knownTotalRef to find how many new entries arrived.
  const knownTotalRef = useRef(0);

  // --- Initial fetch ---
  const fetchInitialLogs = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      const response = await clientRef.current.getLogs({
        limit: 200,
        offset: 0,
        sessionId: source === "session" ? sessionId : undefined,
        // levels will be applied once filters are set (initial fetch uses no level filter)
      });
      const entries: LogEntry[] = (response.entries ?? []).map((e, i) => ({
        id: makeId(),
        timestamp: e.timestamp
          ? new Date(Number(e.timestamp.seconds) * 1000).toISOString()
          : new Date().toISOString(),
        level: mapLevel(e.level),
        message: e.message,
        raw: e.message,
      }));
      // Seed knownTotalRef from the initial fetch so live-tail polling knows
      // how many entries the server already had.
      knownTotalRef.current = response.totalCount ?? entries.length;
      logsRef.current = entries;
      setVersion((v) => v + 1);
    } catch {
      // Non-fatal: show empty list; errors surfaced elsewhere
    }
  }, [source, sessionId]);

  useEffect(() => {
    void fetchInitialLogs();
  }, [fetchInitialLogs]);

  // --- Live-tail polling: fetch incremental logs ---
  const isFollowingRef = useRef(isFollowing);
  useEffect(() => {
    isFollowingRef.current = isFollowing;
  }, [isFollowing]);

  const fetchNewLogs = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      const knownTotal = knownTotalRef.current;
      // Pass multi-level filter via repeated levels field; fall back to no filter for "ALL"
      const activeLevels = levelFilters.filter((l) => l !== "ALL");
      // Request from offset:0 (newest-first) with a limit that covers any newly
      // arrived entries since the last poll. Cap at 200 to bound payload size.
      const limit = Math.min(Math.max(100, 200), 200);
      const response = await clientRef.current.getLogs({
        limit,
        offset: 0,
        sessionId: source === "session" ? sessionId : undefined,
        levels: activeLevels.length > 0 ? activeLevels : undefined,
      });
      const serverTotal = response.totalCount ?? 0;
      const newCount = serverTotal - knownTotal;
      if (newCount <= 0) return;

      // The first `newCount` entries in the newest-first response are the ones
      // we haven't seen yet. Slice them off and reverse so they're oldest-first
      // for appending at the end of logsRef.current.
      const freshSlice = (response.entries ?? []).slice(0, newCount);
      const newEntries: LogEntry[] = freshSlice.reverse().map((e) => ({
        id: makeId(),
        timestamp: e.timestamp
          ? new Date(Number(e.timestamp.seconds) * 1000).toISOString()
          : new Date().toISOString(),
        level: mapLevel(e.level),
        message: e.message,
        raw: e.message,
      }));
      if (newEntries.length === 0) return;

      knownTotalRef.current = serverTotal;
      logsRef.current = [...logsRef.current, ...newEntries];

      startTransition(() => {
        setVersion((v) => v + 1);
        if (!isFollowingRef.current) {
          setQueuedNewLineCount((prev) => prev + newEntries.length);
        }
      });
    } catch {
      // Non-fatal polling error
    }
  }, [source, sessionId, levelFilters]);

  const [_liveTailState, _liveTailControls] = useLiveTail(fetchNewLogs, {
    enabled: liveTailEnabled,
    interval: 2000,
  });

  // --- At-bottom state change: drive follow/pause state machine ---
  const onAtBottomStateChange = useCallback((atBottom: boolean) => {
    if (atBottom) {
      setIsFollowing(true);
      setQueuedNewLineCount(0);
    } else {
      setIsFollowing(false);
    }
  }, []);

  // --- Jump to latest ---
  const jumpToLatest = useCallback(() => {
    setIsFollowing(true);
    setQueuedNewLineCount(0);
    virtuosoRef.current?.scrollToIndex({ index: "LAST", behavior: "smooth" });
  }, []);

  // --- Row toggle (accordion) ---
  const toggleRow = useCallback((i: number) => {
    setExpandedRowIndex((prev) => (prev === i ? null : i));
  }, []);

  // --- Derived: filtered logs ---
  const filteredLogs = useMemo(() => {
    // version is the cache key — reading logsRef.current here is intentional
    // eslint-disable-next-line @typescript-eslint/no-unused-expressions
    version;
    let result = logsRef.current;
    if (searchQuery) {
      const lower = searchQuery.toLowerCase();
      result = result.filter((e) => e.message.toLowerCase().includes(lower));
    }
    if (levelFilters.length > 0 && !levelFilters.includes("ALL")) {
      result = result.filter((e) => levelFilters.includes(e.level));
    }
    return result;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, searchQuery, levelFilters]);

  return {
    logs: filteredLogs,
    isFollowing,
    searchQuery,
    setSearchQuery,
    levelFilters,
    setLevelFilters,
    matchCount: searchQuery ? filteredLogs.length : 0,
    totalCount: logsRef.current.length,
    toggleRow,
    expandedRowIndex,
    selectedRowIndex,
    setSelectedRowIndex,
    jumpToLatest,
    queuedNewLineCount,
    onAtBottomStateChange,
    virtuosoRef,
    liveTailEnabled,
    setLiveTailEnabled,
  };
}
