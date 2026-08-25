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
import type { LogEntry as ProtoLogEntry } from "@/gen/session/v1/session_pb";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
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

export interface LogViewerTimeRange {
  start: Date;
  end: Date;
}

export interface LogViewerOptions {
  /** Restrict fetches to this time range. Omit for "most recent N" (default behavior). */
  timeRange?: LogViewerTimeRange;
  /** Max entries to fetch per request (default: 200). */
  limit?: number;
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
  /** Raw proto entries matching the current search/level filters — for export and pattern analysis. */
  rawEntries: ProtoLogEntry[];
  /** Total entries matching the current filters on the server (not just what's been fetched). */
  serverTotalCount: number;
  /** Timestamp of the last successful fetch (initial or live-tail poll). */
  lastRefresh: Date | null;
  /** Re-runs the initial fetch — for a manual "Refresh" action. */
  refresh: () => void;
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
  options?: LogViewerOptions,
): LogViewerState {
  const fetchLimit = options?.limit ?? 200;
  const timeRange = options?.timeRange;
  // Time range as primitives so it can sit in a dependency array without a
  // new-object-every-render identity problem (callers often build the Date
  // objects inline).
  const timeRangeStartMs = timeRange?.start.getTime();
  const timeRangeEndMs = timeRange?.end.getTime();

  // --- Core log storage: mutable ref + version counter for O(1) appending ---
  const logsRef = useRef<LogEntry[]>([]);
  const rawEntriesRef = useRef<ProtoLogEntry[]>([]);
  const [version, setVersion] = useState(0);
  const [serverTotalCount, setServerTotalCount] = useState(0);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);

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
        limit: fetchLimit,
        offset: 0,
        sessionId: source === "session" ? sessionId : undefined,
        startTime: timeRangeStartMs !== undefined ? timestampFromDate(new Date(timeRangeStartMs)) : undefined,
        endTime: timeRangeEndMs !== undefined ? timestampFromDate(new Date(timeRangeEndMs)) : undefined,
        // levels will be applied once filters are set (initial fetch uses no level filter)
      });
      const protoEntries = response.entries ?? [];
      const entries: LogEntry[] = protoEntries.map((e) => ({
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
      rawEntriesRef.current = protoEntries;
      setServerTotalCount(response.totalCount ?? entries.length);
      setLastRefresh(new Date());
      setVersion((v) => v + 1);
    } catch {
      // Non-fatal: show empty list; errors surfaced elsewhere
    }
  }, [source, sessionId, fetchLimit, timeRangeStartMs, timeRangeEndMs]);

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
    // A caller-supplied time range means "browse this historical window",
    // not "follow the tail" — polling for entries newer than knownTotalRef
    // would pull in current-time entries outside that window. The live-tail
    // toggle stays available but is a no-op until the range is cleared.
    if (timeRange) return;
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
      setServerTotalCount(serverTotal);
      setLastRefresh(new Date());
      if (newCount <= 0) return;

      // The first `newCount` entries in the newest-first response are the ones
      // we haven't seen yet. Slice them off and reverse so they're oldest-first
      // for appending at the end of logsRef.current.
      const freshProtoSlice = (response.entries ?? []).slice(0, newCount).reverse();
      const newEntries: LogEntry[] = freshProtoSlice.map((e) => ({
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
      rawEntriesRef.current = [...rawEntriesRef.current, ...freshProtoSlice];

      startTransition(() => {
        setVersion((v) => v + 1);
        if (!isFollowingRef.current) {
          setQueuedNewLineCount((prev) => prev + newEntries.length);
        }
      });
    } catch {
      // Non-fatal polling error
    }
  }, [source, sessionId, levelFilters, timeRange]);

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

  // --- Derived: filtered logs (and the parallel raw proto entries — the two
  // refs are always extended in lockstep by the same index, so filtering by
  // index on the display copy tells us which raw entries survive too) ---
  const matchesFilter = useCallback(
    (e: LogEntry) => {
      if (searchQuery && !e.message.toLowerCase().includes(searchQuery.toLowerCase())) return false;
      if (levelFilters.length > 0 && !levelFilters.includes("ALL") && !levelFilters.includes(e.level)) return false;
      return true;
    },
    [searchQuery, levelFilters],
  );

  const filteredLogs = useMemo(() => {
    // version is the cache key — reading logsRef.current here is intentional
    // eslint-disable-next-line @typescript-eslint/no-unused-expressions
    version;
    return logsRef.current.filter(matchesFilter);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, matchesFilter]);

  const filteredRawEntries = useMemo(() => {
    // eslint-disable-next-line @typescript-eslint/no-unused-expressions
    version;
    return logsRef.current.reduce<ProtoLogEntry[]>((acc, e, i) => {
      if (matchesFilter(e) && rawEntriesRef.current[i]) {
        acc.push(rawEntriesRef.current[i]);
      }
      return acc;
    }, []);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, matchesFilter]);

  const refresh = useCallback(() => {
    void fetchInitialLogs();
  }, [fetchInitialLogs]);

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
    rawEntries: filteredRawEntries,
    serverTotalCount,
    lastRefresh,
    refresh,
  };
}
