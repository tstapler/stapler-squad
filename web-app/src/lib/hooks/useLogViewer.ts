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
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { LogEntry as ProtoLogEntry } from "@/gen/session/v1/session_pb";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
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
  /** Message from the most recent failed fetch, or null if the last fetch succeeded. */
  error: string | null;
  /** Re-runs the initial fetch — for a manual "Refresh" action. */
  refresh: () => void;
}

interface DisplayLogEntry {
  display: LogEntry;
  raw: ProtoLogEntry;
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
  // Each display entry is paired with its raw proto counterpart in one
  // record, so filtering can never desync the two the way two independently
  // maintained parallel arrays could.
  const entriesRef = useRef<DisplayLogEntry[]>([]);
  const [version, setVersion] = useState(0);
  const [serverTotalCount, setServerTotalCount] = useState(0);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);

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
    clientRef.current = createClient(SessionService, getConnectTransport());
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
      const entries: DisplayLogEntry[] = protoEntries.map((e) => ({
        display: {
          id: makeId(),
          timestamp: e.timestamp
            ? new Date(Number(e.timestamp.seconds) * 1000).toISOString()
            : new Date().toISOString(),
          level: mapLevel(e.level),
          message: e.message,
          raw: e.message,
        },
        raw: e,
      }));
      // Seed knownTotalRef from the initial fetch so live-tail polling knows
      // how many entries the server already had.
      knownTotalRef.current = response.totalCount ?? entries.length;
      entriesRef.current = entries;
      setServerTotalCount(response.totalCount ?? entries.length);
      setLastRefresh(new Date());
      setError(null);
      setVersion((v) => v + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load logs");
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
    if (timeRangeStartMs !== undefined || timeRangeEndMs !== undefined) return;
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
      setError(null);
      if (newCount <= 0) return;

      // The first `newCount` entries in the newest-first response are the ones
      // we haven't seen yet. Slice them off and reverse so they're oldest-first
      // for appending at the end of entriesRef.current.
      const freshProtoSlice = (response.entries ?? []).slice(0, newCount).reverse();
      const newEntries: DisplayLogEntry[] = freshProtoSlice.map((e) => ({
        display: {
          id: makeId(),
          timestamp: e.timestamp
            ? new Date(Number(e.timestamp.seconds) * 1000).toISOString()
            : new Date().toISOString(),
          level: mapLevel(e.level),
          message: e.message,
          raw: e.message,
        },
        raw: e,
      }));
      if (newEntries.length === 0) return;

      knownTotalRef.current = serverTotal;
      entriesRef.current = [...entriesRef.current, ...newEntries];

      startTransition(() => {
        setVersion((v) => v + 1);
        if (!isFollowingRef.current) {
          setQueuedNewLineCount((prev) => prev + newEntries.length);
        }
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to refresh logs");
    }
  }, [source, sessionId, levelFilters, timeRangeStartMs, timeRangeEndMs]);

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

  // --- Derived: filtered entries (display + raw stay paired since they're
  // filtered together as one record, not as two independently-indexed arrays) ---
  const matchesFilter = useCallback(
    (e: LogEntry) => {
      if (searchQuery && !e.message.toLowerCase().includes(searchQuery.toLowerCase())) return false;
      if (levelFilters.length > 0 && !levelFilters.includes("ALL") && !levelFilters.includes(e.level)) return false;
      return true;
    },
    [searchQuery, levelFilters],
  );

  const filteredEntries = useMemo(() => {
    // version is the cache key — reading entriesRef.current here is intentional
    // eslint-disable-next-line @typescript-eslint/no-unused-expressions
    version;
    return entriesRef.current.filter((e) => matchesFilter(e.display));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, matchesFilter]);

  const filteredLogs = useMemo(() => filteredEntries.map((e) => e.display), [filteredEntries]);
  const filteredRawEntries = useMemo(() => filteredEntries.map((e) => e.raw), [filteredEntries]);

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
    totalCount: entriesRef.current.length,
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
    error,
    refresh,
  };
}
