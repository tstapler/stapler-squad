"use client";

import { useEffect, useCallback, useRef, useMemo, useState } from "react";
import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { createWatchTransport } from "@/lib/transport/watch-ws-transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import { Session, SessionStatus, Shell, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  CreateSessionRequest,
  UpdateSessionRequest,
  PromptHistoryEntry,
  RunOneShotResponse,
  SpawnShellRequest,
  RunWorkflowRequestSchema,
  ArchiveSessionRequestSchema,
  UnarchiveSessionRequestSchema,
  ListSessionsRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { SessionEvent, NotificationEvent } from "@/gen/session/v1/events_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BackoffState, getWsCloseCode, isRetriableCloseCode } from "@/lib/utils/backoff";
import { createRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { useAppDispatch, useAppSelector } from "@/lib/store";
import {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  setConnectionState,
  selectAllSessions,
  selectSessionsLoading,
  selectSessionsError,
  selectConnectionState,
  removeDetectedStatus,
} from "@/lib/store/sessionsSlice";

// ponytail: stable empty array so non-watching callers (e.g. useSessionActions
// in each SessionCard) don't subscribe to the full sessions list. Without this,
// 29 cards × N events/second = N*29 card re-renders/second saturating the JS thread.
const EMPTY_SESSIONS: Session[] = [];
const selectNoSessions = () => EMPTY_SESSIONS;
import { removeItem as removeReviewQueueItem } from "@/lib/store/reviewQueueSlice";

interface UseSessionServiceOptions {
  baseUrl?: string;
  autoWatch?: boolean;
  /** When false, suppresses all API calls (e.g. while auth is loading). Defaults to true. */
  enabled?: boolean;
  onNotification?: (notification: NotificationEvent) => void;
  /**
   * Called after a stream disconnect-and-reconnect (after the reconciling
   * listSessions call). Use this to re-fetch data that may have been missed
   * during the gap (e.g. notification history).
   */
  onReconnect?: () => void;
  /**
   * Called when an approval_response event arrives on the stream. Use this to
   * refresh notification history so all connected clients stay in sync when any
   * device resolves an approval. Receives the approvalId and sessionId from the event.
   */
  onApprovalResponse?: (approvalId: string, sessionId: string) => void;
  /**
   * Called when a session is deleted. Use this to clear related state such as
   * notifications keyed to the deleted session.
   */
  onSessionDeleted?: (sessionId: string) => void;
}

interface UseSessionServiceReturn {
  // State
  sessions: Session[];
  loading: boolean;
  error: Error | null;
  connectionState: import("@/lib/store/sessionsSlice").ConnectionState;
  /** System-wide memory usage percentage (0–100). Zero when unavailable. */
  systemMemoryPct: number;

  // Methods
  listSessions: (options?: { category?: string; status?: SessionStatus; includeArchived?: boolean }) => Promise<void>;
  getSession: (id: string) => Promise<Session | null>;
  createSession: (request: Partial<CreateSessionRequest>) => Promise<Session | null>;
  updateSession: (id: string, updates: Partial<UpdateSessionRequest>) => Promise<Session | null>;
  deleteSession: (id: string, force?: boolean) => Promise<boolean>;
  runOneShot: (sessionId: string, prompt: string, timeoutSeconds?: number) => Promise<RunOneShotResponse | null>;
  listPromptHistory: (limit?: number) => Promise<PromptHistoryEntry[]>;
  pauseSession: (id: string) => Promise<Session | null>;
  resumeSession: (id: string, updates?: { title?: string; tags?: string[] }) => Promise<Session | null>;
  hibernateSession: (id: string) => Promise<Session | null>;
  resumeHibernatedSession: (id: string) => Promise<Session | null>;
  renameSession: (id: string, newTitle: string) => Promise<boolean>;
  restartSession: (id: string) => Promise<boolean>;
  clearConversationState: (id: string) => Promise<boolean>;
  acknowledgeSession: (id: string) => Promise<boolean>;
  createCheckpoint: (sessionId: string, label: string) => Promise<boolean>;
  listCheckpoints: (sessionId: string) => Promise<import("@/gen/session/v1/types_pb").CheckpointProto[]>;
  forkSession: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;

  // Archive methods
  archiveSession: (id: string) => Promise<boolean>;
  unarchiveSession: (id: string) => Promise<boolean>;
  listSessionsByWorkflow: (workflowId: string, includeArchived?: boolean) => Promise<Session[]>;

  // Workflow methods
  runWorkflow: (request: { id: string; arg?: string }) => Promise<string | null>;

  // Shell methods
  spawnShell: (request: Partial<SpawnShellRequest>) => Promise<Shell | null>;
  stopShell: (sessionId: string, shellId: string) => Promise<boolean>;
  restartShell: (sessionId: string, shellId: string) => Promise<boolean>;
  listShells: (sessionId: string) => Promise<Shell[]>;
  deleteShell: (sessionId: string, shellId: string) => Promise<boolean>;

  reconnectAttemptCount: number;

  // Real-time updates
  watchSessions: (options?: { categoryFilter?: string; statusFilter?: SessionStatus }) => void;
  stopWatching: () => void;

  // Session monitor
  getTerminalSnapshot: (sessionId: string, lastNLines?: number) => Promise<string>;
  writeToSession: (sessionId: string, input: string, pressEnter?: boolean) => Promise<boolean>;
  getConversationMessages: (sessionId: string, limit?: number) => Promise<Array<{ role: string; content: string; timestamp?: string; model?: string }>>;
}

export function useSessionService(
  options: UseSessionServiceOptions = {}
): UseSessionServiceReturn {
  const { baseUrl = getApiBaseUrl(), autoWatch = false, enabled = true, onNotification, onReconnect, onApprovalResponse, onSessionDeleted } = options;
  const analytics = useAnalytics();
  const onReconnectRef = useRef(onReconnect);
  useEffect(() => { onReconnectRef.current = onReconnect; }, [onReconnect]);
  const onNotificationRef = useRef(onNotification);
  const onApprovalResponseRef = useRef(onApprovalResponse);
  const onSessionDeletedRef = useRef(onSessionDeleted);

  // Keep ref updated for callback in streaming loop
  useEffect(() => {
    onNotificationRef.current = onNotification;
  }, [onNotification]);

  useEffect(() => {
    onApprovalResponseRef.current = onApprovalResponse;
  }, [onApprovalResponse]);

  useEffect(() => {
    onSessionDeletedRef.current = onSessionDeleted;
  }, [onSessionDeleted]);

  const dispatch = useAppDispatch();
  const [systemMemoryPct, setSystemMemoryPct] = useState<number>(0);
  const [reconnectAttemptCount, setReconnectAttemptCount] = useState(0);
  const sessions = useAppSelector(autoWatch ? selectAllSessions : selectNoSessions);
  const loading = useAppSelector(selectSessionsLoading);
  const errorStr = useAppSelector(selectSessionsError);

  const abortControllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  // Reconnect control: true while watchSessions is active (user did not explicitly stop)
  const shouldReconnectRef = useRef(false);
  // Jittered exponential backoff state
  const backoffRef = useRef(new BackoffState(1000, 30_000));
  // Timestamp of last received stream event, used to detect staleness
  const lastEventTimeRef = useRef<number | null>(null);
  // Last seen event sequence number — passed as after_seq on reconnect so the
  // server replays any events missed during the disconnect window (up to 1 hour).
  const lastSeqRef = useRef<bigint>(0n);
  // Stores current watch options so reconnects use the latest options without stale closure
  const watchOptionsRef = useRef<{ categoryFilter?: string; statusFilter?: SessionStatus } | undefined>(undefined);
  // Monotonically-increasing stream generation counter; checked at every await checkpoint
  const streamGenerationRef = useRef(0);
  // Whether isConnected — synced directly (not via useEffect) to avoid render-cycle lag
  const isConnectedRef = useRef(false);
  // Ref to current watchSessions function — updated every render for stable event handler indirection
  const watchSessionsRef = useRef<((opts?: { categoryFilter?: string; statusFilter?: SessionStatus }) => void) | undefined>(undefined);
  // Debounce timer for visibilitychange/online handlers
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Tracks whether seq backwards-jump was detected, triggering full resync
  const needsFullResyncRef = useRef(false);
  // Tracks whether the backstop interval has already triggered a reconnect
  const backstopTriggeredRef = useRef(false);
  // Stable ref to dispatch so visibility/online handler can use [] deps
  const dispatchRef = useRef(dispatch);

  // Initialize ConnectRPC client — uses HTTP for unary, WebSocket for streaming Watch* RPCs
  useEffect(() => {
    const transport = createWatchTransport({
      baseUrl,
      interceptors: [createAuthInterceptor(), createRpcTimingInterceptor(analytics)],
    });

    clientRef.current = createClient(SessionService, transport);
  // analytics identity is stable (memoized in AnalyticsContextProvider); baseUrl is the only real dep
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseUrl]);

  // List sessions with retry logic
  const listSessions = useCallback(
    async (listOptions?: { category?: string; status?: SessionStatus; includeArchived?: boolean }) => {
      if (!clientRef.current) return;

      dispatch(setLoading(true));
      dispatch(setError(null));

      try {
        const response = await clientRef.current.listSessions({
          category: listOptions?.category,
          status: listOptions?.status,
          includeArchived: listOptions?.includeArchived,
        });

        dispatch(setSessions(response.sessions));
        dispatch(setError(null)); // Clear any previous errors
        if (response.systemMemoryPct > 0) {
          setSystemMemoryPct(response.systemMemoryPct);
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error("Failed to list sessions");
        dispatch(setError(error.message));
        console.error("Failed to list sessions:", error);
      } finally {
        dispatch(setLoading(false));
      }
    },
    [dispatch]
  );

  // Get single session
  const getSession = useCallback(async (id: string): Promise<Session | null> => {
    if (!clientRef.current) return null;

    try {
      const response = await clientRef.current.getSession({ id });
      return response.session ?? null;
    } catch (err) {
      dispatch(setError(err instanceof Error ? err.message : "Failed to get session"));
      return null;
    }
  }, [dispatch]);

  // Create session
  const createSession = useCallback(
    async (request: Partial<CreateSessionRequest>): Promise<Session | null> => {
      if (!clientRef.current) return null;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.createSession({
          title: request.title ?? "",
          path: request.path ?? "",
          workingDir: request.workingDir,
          branch: request.branch,
          program: request.program,
          category: request.category,
          prompt: request.prompt,
          autoYes: request.autoYes,
          existingWorktree: request.existingWorktree,
          sessionType: request.sessionType,
          createIfMissing: request.createIfMissing ?? false,
          initialPrompt: request.initialPrompt,
          autonomousMode: request.autonomousMode ?? false,
          permissionMode: request.permissionMode ?? "",
          aliasName: request.aliasName ?? "",
          cliFlags: request.cliFlags ?? "",
        });

        // Add to store (with duplicate check handled by entity adapter upsertOne)
        if (response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.session ?? null;
      } catch (err) {
        const wrappedErr = err instanceof Error ? err : new Error("Failed to create session");
        dispatch(setError(wrappedErr.message));
        throw wrappedErr;
      }
    },
    [dispatch]
  );

  // Update session
  const updateSession = useCallback(
    async (
      id: string,
      updates: Partial<UpdateSessionRequest>
    ): Promise<Session | null> => {
      if (!clientRef.current) return null;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.updateSession({
          id,
          status: updates.status,
          category: updates.category,
          title: updates.title,
          program: updates.program,
          tags: updates.tags ?? [],
          workingDir: updates.workingDir,
          rateLimitEnabled: updates.rateLimitEnabled,
          autonomousMode: updates.autonomousMode,
          steerMessage: updates.steerMessage,
        });

        // Update in store
        if (response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.session ?? null;
      } catch (err) {
        console.error("[useSessionService] updateSession failed:", err);
        dispatch(setError(err instanceof Error ? err.message : "Failed to update session"));
        return null;
      }
    },
    [dispatch]
  );

  // Delete session
  const deleteSession = useCallback(
    async (id: string, force: boolean = false): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.deleteSession({ id, force });

        // Remove from store
        if (response.success) {
          dispatch(removeSession(id));
        }

        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to delete session"));
        return false;
      }
    },
    [dispatch]
  );

  // Pause session
  const pauseSession = useCallback(
    async (id: string): Promise<Session | null> => {
      return updateSession(id, {
        status: SessionStatus.PAUSED,
      });
    },
    [updateSession]
  );

  // Resume session with optional metadata updates (title, tags)
  const resumeSession = useCallback(
    async (id: string, updates?: { title?: string; tags?: string[] }): Promise<Session | null> => {
      return updateSession(id, {
        status: SessionStatus.RUNNING,
        ...(updates?.title ? { title: updates.title } : {}),
        ...(updates?.tags && updates.tags.length > 0 ? { tags: updates.tags } : {}),
      });
    },
    [updateSession]
  );

  // Hibernate session (Active → Hibernated)
  const hibernateSession = useCallback(
    async (id: string): Promise<Session | null> => {
      if (!clientRef.current) return null;
      dispatch(setError(null));
      try {
        const response = await clientRef.current.hibernateSession({ id, reason: "manual" });
        if (response.session) dispatch(upsertSession(response.session));
        return response.session ?? null;
      } catch (err) {
        console.error("[useSessionService] hibernateSession failed:", err);
        dispatch(setError(err instanceof Error ? err.message : "Failed to hibernate session"));
        return null;
      }
    },
    [dispatch]
  );

  // Resume a hibernated session (Hibernated → Active)
  const resumeHibernatedSession = useCallback(
    async (id: string): Promise<Session | null> => {
      if (!clientRef.current) return null;
      dispatch(setError(null));
      try {
        const response = await clientRef.current.resumeHibernatedSession({ id });
        if (response.session) dispatch(upsertSession(response.session));
        return response.session ?? null;
      } catch (err) {
        console.error("[useSessionService] resumeHibernatedSession failed:", err);
        dispatch(setError(err instanceof Error ? err.message : "Failed to resume hibernated session"));
        return null;
      }
    },
    [dispatch]
  );


  // Rename session
  const renameSession = useCallback(
    async (id: string, newTitle: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.renameSession({
          id,
          newTitle
        });

        // Update in store
        if (response.session) {
          dispatch(upsertSession(response.session));
        }

        return !!response.session;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to rename session"));
        return false;
      }
    },
    [dispatch]
  );

  // Restart session
  const restartSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.restartSession({ id });

        // Update in store
        if (response.success && response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to restart session"));
        return false;
      }
    },
    [dispatch]
  );

  // Clear the stored Claude conversation UUID so next resume starts fresh
  const clearConversationState = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.clearConversationState({ id });
        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to clear conversation state"));
        return false;
      }
    },
    [dispatch]
  );

  // Create checkpoint for a session
  const createCheckpoint = useCallback(
    async (sessionId: string, label: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        await clientRef.current.createCheckpoint({ sessionId, label });
        return true;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to create checkpoint"));
        return false;
      }
    },
    [dispatch]
  );

  // List checkpoints for a session
  const listCheckpoints = useCallback(
    async (sessionId: string) => {
      if (!clientRef.current) return [];
      try {
        const response = await clientRef.current.listCheckpoints({ sessionId });
        return response.checkpoints;
      } catch {
        return [];
      }
    },
    []
  );

  // Fork a session from a checkpoint
  const forkSession = useCallback(
    async (sessionId: string, checkpointId: string, newTitle: string): Promise<Session | null> => {
      if (!clientRef.current) return null;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.forkSession({ sessionId, checkpointId, newTitle });
        if (response.session) {
          dispatch(upsertSession(response.session));
        }
        return response.session ?? null;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to fork session"));
        return null;
      }
    },
    [dispatch]
  );

  // Acknowledge session (skip from review queue)
  const acknowledgeSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.acknowledgeSession({ id });
        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to acknowledge session"));
        return false;
      }
    },
    [dispatch]
  );

  // Run one-shot claude command for a session (S3)
  const runOneShot = useCallback(
    async (sessionId: string, prompt: string, timeoutSeconds?: number): Promise<RunOneShotResponse | null> => {
      if (!clientRef.current) return null;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.runOneShot({
          sessionId,
          prompt,
          timeoutSeconds: timeoutSeconds ?? 0,
        });
        return response;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to run one-shot"));
        return null;
      }
    },
    [dispatch]
  );

  // List prompt history entries (S1)
  const listPromptHistory = useCallback(
    async (limit?: number): Promise<PromptHistoryEntry[]> => {
      if (!clientRef.current) return [];

      try {
        const response = await clientRef.current.listPromptHistory({ limit: limit ?? 20 });
        return response.entries;
      } catch {
        return [];
      }
    },
    []
  );

  // Fire a workflow immediately (outside of cron schedule).
  const archiveSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        await clientRef.current.archiveSession(create(ArchiveSessionRequestSchema, { sessionId: id }));
        return true;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to archive session"));
        return false;
      }
    },
    [dispatch]
  );

  const unarchiveSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        await clientRef.current.unarchiveSession(create(UnarchiveSessionRequestSchema, { sessionId: id }));
        return true;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to unarchive session"));
        return false;
      }
    },
    [dispatch]
  );

  const listSessionsByWorkflow = useCallback(
    async (workflowId: string, includeArchived = true): Promise<Session[]> => {
      if (!clientRef.current) return [];
      try {
        const req = create(ListSessionsRequestSchema, {
          workflowId,
          includeArchived,
        });
        const response = await clientRef.current.listSessions(req);
        return response.sessions ?? [];
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to list workflow runs"));
        return [];
      }
    },
    [dispatch]
  );

  const runWorkflow = useCallback(
    async (request: { id: string; arg?: string }): Promise<string | null> => {
      if (!clientRef.current) return null;
      try {
        const req = create(RunWorkflowRequestSchema, {
          id: request.id,
          arg: request.arg ?? "",
        });
        const response = await clientRef.current.runWorkflow(req);
        return response.sessionId ?? null;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to run workflow"));
        return null;
      }
    },
    [dispatch]
  );

  // Spawn a new shell attached to a session
  const spawnShell = useCallback(
    async (request: Partial<SpawnShellRequest>): Promise<Shell | null> => {
      if (!clientRef.current) return null;
      try {
        const response = await clientRef.current.spawnShell({
          sessionId: request.sessionId ?? "",
          name: request.name ?? "",
          command: request.command ?? "",
          workingDir: request.workingDir ?? "",
        });
        return response.shell ?? null;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to spawn shell"));
        return null;
      }
    },
    [dispatch]
  );

  // Stop a running shell
  const stopShell = useCallback(
    async (sessionId: string, shellId: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        const response = await clientRef.current.stopShell({ sessionId, shellId });
        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to stop shell"));
        return false;
      }
    },
    [dispatch]
  );

  // Restart a shell
  const restartShell = useCallback(
    async (sessionId: string, shellId: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        const response = await clientRef.current.restartShell({ sessionId, shellId });
        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to restart shell"));
        return false;
      }
    },
    [dispatch]
  );

  // List all shells for a session
  const listShells = useCallback(
    async (sessionId: string): Promise<Shell[]> => {
      if (!clientRef.current) return [];
      try {
        const response = await clientRef.current.listShells({ sessionId });
        return response.shells;
      } catch {
        return [];
      }
    },
    []
  );

  // Delete a shell (stop + remove from storage)
  const deleteShell = useCallback(
    async (sessionId: string, shellId: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        const response = await clientRef.current.deleteShell({ sessionId, shellId });
        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to delete shell"));
        return false;
      }
    },
    [dispatch]
  );

  // Handle session events from watch stream
  const handleSessionEvent = useCallback((event: SessionEvent) => {
    // Advance the sequence cursor so reconnects can request a targeted replay.
    const prevSeq = lastSeqRef.current;
    if (event.seq > prevSeq) {
      lastSeqRef.current = event.seq;
    }

    // Seq backwards-jump detection: indicates server restart → request full snapshot
    if (event.seq > 0n && event.seq < prevSeq) {
      console.warn("[reconnect] seq backwards-jump detected — resetting afterSeq to 0");
      lastSeqRef.current = 0n;
      needsFullResyncRef.current = true;
    }

    // Handle different event types based on oneof case
    switch (event.event.case) {
      case "sessionCreated": {
        const session = event.event.value.session;
        if (!session) return;
        // Entity adapter handles deduplication via upsertOne
        dispatch(upsertSession(session));
        break;
      }
      case "sessionUpdated": {
        const session = event.event.value.session;
        if (!session) return;
        dispatch(upsertSession(session));
        break;
      }
      case "sessionDeleted": {
        const sessionId = event.event.value.sessionId;
        dispatch(removeSession(sessionId));
        dispatch(removeReviewQueueItem(sessionId));
        dispatch(removeDetectedStatus(sessionId));
        onSessionDeletedRef.current?.(sessionId);
        break;
      }
      case "notification": {
        // Route notification events to the callback
        if (onNotificationRef.current) {
          onNotificationRef.current(event.event.value);
        }
        break;
      }
      case "approvalResponse": {
        // An approval was resolved on this device or another — remove the toast
        // preemptively and refresh history to show the resolved badge.
        const approvalId = event.event.value.context ?? "";
        const sessionId = event.event.value.sessionId ?? "";
        if (event.event.value.approved && sessionId) {
          dispatch(removeDetectedStatus(sessionId));
        }
        if (approvalId) {
          onApprovalResponseRef.current?.(approvalId, sessionId);
        }
        break;
      }
      case "sessionAcknowledged": {
        const sessionId = event.event.value.sessionId ?? "";
        if (sessionId) {
          dispatch(removeDetectedStatus(sessionId));
          dispatch(removeReviewQueueItem(sessionId));
        }
        break;
      }
    }
  }, [dispatch]);

  // Watch sessions for real-time updates with automatic reconnect on failure.
  // On reconnect, ListSessions is called first to flush any state missed while disconnected.
  const watchSessions = useCallback(
    (watchOptions?: { categoryFilter?: string; statusFilter?: SessionStatus }) => {
      if (!clientRef.current) return;

      // Store options in ref so reconnects use them without stale closure
      watchOptionsRef.current = watchOptions;

      // Stop any existing watch
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      shouldReconnectRef.current = true;
      backoffRef.current.reset(); // Reset backoff when explicitly (re)started
      setReconnectAttemptCount(0);
      ++streamGenerationRef.current; // Invalidate any in-flight startStream from prior call

      const startStream = async () => {
        if (!shouldReconnectRef.current || !clientRef.current) return;
        const myGeneration = ++streamGenerationRef.current;

        abortControllerRef.current = new AbortController();
        lastEventTimeRef.current = Date.now(); // Treat stream start as an activity timestamp

        try {
          // Initial snapshot before stream starts (pass active filters so the snapshot matches the stream)
          const initialResponse = await clientRef.current.listSessions({
            category: watchOptionsRef.current?.categoryFilter,
            status: watchOptionsRef.current?.statusFilter,
          });
          if (!shouldReconnectRef.current || streamGenerationRef.current !== myGeneration) return;
          dispatch(setSessions(initialResponse.sessions));

          const stream = clientRef.current.watchSessions(
            {
              categoryFilter: watchOptionsRef.current?.categoryFilter,
              statusFilter: watchOptionsRef.current?.statusFilter,
              afterSeq: lastSeqRef.current,
            },
            { signal: abortControllerRef.current.signal }
          );

          let firstEvent = true;
          for await (const event of stream) {
            if (firstEvent) {
              firstEvent = false;
              isConnectedRef.current = true;
              backstopTriggeredRef.current = false; // Reset backstop flag on successful stream
              dispatch(setConnectionState("connected"));
            }
            lastEventTimeRef.current = Date.now();
            handleSessionEvent(event);
          }

          // Stream ended normally (server-side close). Reconnect if still desired.
          if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
            dispatch(setConnectionState("disconnected"));
            isConnectedRef.current = false;

            // Handle backwards-jump: do a full resync
            if (needsFullResyncRef.current) {
              needsFullResyncRef.current = false;
              void clientRef.current?.listSessions({
                category: watchOptionsRef.current?.categoryFilter,
                status: watchOptionsRef.current?.statusFilter,
              }).then(r => {
                if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
                  dispatch(setSessions(r.sessions));
                }
              });
            }

            onReconnectRef.current?.();
            const delay = backoffRef.current.next();
            setReconnectAttemptCount(backoffRef.current.attempt);
            console.info(`[reconnect] stream=watch trigger=close attempt=${backoffRef.current.attempt} delay=${delay}ms`);
            await new Promise(r => setTimeout(r, delay));
            if (streamGenerationRef.current !== myGeneration || !shouldReconnectRef.current) return;
            startStream();
          }
        } catch (err) {
          if (err instanceof Error && err.name === "AbortError") {
            return; // Intentional stop via stopWatching()
          }
          if (err instanceof ConnectError && err.code === Code.Canceled) {
            return; // ConnectRPC abort (e.g. AbortController signal)
          }

          // Check for non-retriable WS close codes
          const wsCode = getWsCloseCode(err);
          if (wsCode !== null && !isRetriableCloseCode(wsCode)) {
            console.warn(`[reconnect] stream=watch non-retriable close code=${wsCode}, stopping reconnect`);
            shouldReconnectRef.current = false;
            isConnectedRef.current = false;
            dispatch(setConnectionState("disconnected"));
            return;
          }

          // Unexpected network error — log, then reconnect
          dispatch(setError(err instanceof Error ? err.message : "Watch stream error"));
          if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
            dispatch(setConnectionState("disconnected"));
            isConnectedRef.current = false;

            // Handle backwards-jump: do a full resync
            if (needsFullResyncRef.current) {
              needsFullResyncRef.current = false;
              void clientRef.current?.listSessions({
                category: watchOptionsRef.current?.categoryFilter,
                status: watchOptionsRef.current?.statusFilter,
              }).then(r => {
                if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
                  dispatch(setSessions(r.sessions));
                }
              });
            }

            onReconnectRef.current?.();
            const delay = backoffRef.current.next();
            setReconnectAttemptCount(backoffRef.current.attempt);
            console.info(`[reconnect] stream=watch trigger=error attempt=${backoffRef.current.attempt} delay=${delay}ms`);
            await new Promise(r => setTimeout(r, delay));
            if (streamGenerationRef.current !== myGeneration || !shouldReconnectRef.current) return;
            startStream();
          }
        }
      };

      startStream();
    },
    [handleSessionEvent, dispatch]
  );

  // Stop watching sessions
  const stopWatching = useCallback(() => {
    shouldReconnectRef.current = false;
    isConnectedRef.current = false;
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    dispatch(setConnectionState("disconnected"));
  }, [dispatch]);

  // Backstop staleness detector: 30s interval for always-visible tabs
  useEffect(() => {
    if (!enabled) return;
    const interval = setInterval(() => {
      if (
        shouldReconnectRef.current &&
        !isConnectedRef.current &&
        lastEventTimeRef.current !== null &&
        Date.now() - lastEventTimeRef.current > 30_000
      ) {
        dispatch(setConnectionState("stale"));
        if (!backstopTriggeredRef.current) {
          backstopTriggeredRef.current = true;
          watchSessionsRef.current?.(watchOptionsRef.current);
        }
      }
    }, 30_000);
    return () => clearInterval(interval);
  }, [enabled, dispatch]);

  // Keep refs current on every render (for stable event handler indirection)
  watchSessionsRef.current = watchSessions;
  dispatchRef.current = dispatch;

  // Browser lifecycle listeners: reconnect on tab visibility restore or network online.
  // Empty deps + dispatchRef indirection keeps the function reference stable across renders
  // so removeEventListener correctly deregisters the exact same handler instance.
  const handleVisibilityOrOnline = useCallback((ev: Event) => {
    if (document.visibilityState !== "visible" && ev.type !== "online") return;
    if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    debounceTimerRef.current = setTimeout(() => {
      debounceTimerRef.current = null;
      if (!shouldReconnectRef.current) return;
      const isStale = lastEventTimeRef.current !== null && lastEventTimeRef.current < Date.now() - 15_000;
      if (!isConnectedRef.current || isStale) {
        if (isStale) {
          dispatchRef.current(setConnectionState("stale"));
        }
        backoffRef.current.reset();
        watchSessionsRef.current?.(watchOptionsRef.current);
      }
    }, 200);
  }, []);

  useEffect(() => {
    if (!enabled || process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return;
    document.addEventListener("visibilitychange", handleVisibilityOrOnline);
    window.addEventListener("online", handleVisibilityOrOnline);
    return () => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
      document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
      window.removeEventListener("online", handleVisibilityOrOnline);
    };
  }, [enabled, handleVisibilityOrOnline]);

  // Auto-watch on mount if enabled and authenticated
  useEffect(() => {
    if (!enabled) return;
    if (autoWatch) {
      watchSessions();
    }

    return () => {
      stopWatching();
    };
  }, [enabled, autoWatch, watchSessions, stopWatching]);

  // Initial load — only for the watching instance (autoWatch: true).
  // Non-watching callers (useSessionActions, OmnibarContext, etc.) should read
  // from Redux directly; firing listSessions() from every caller causes N × 5
  // synchronous dispatches on mount that saturate the React render queue.
  useEffect(() => {
    if (!enabled || !autoWatch) return;
    listSessions();
  }, [enabled, autoWatch, listSessions]);

  // Convert error string back to Error object for backward compatibility
  const error = useMemo(() => (errorStr ? new Error(errorStr) : null), [errorStr]);

  const connectionState = useAppSelector(selectConnectionState);

  const getTerminalSnapshot = useCallback(
    async (sessionId: string, lastNLines = 50): Promise<string> => {
      if (!clientRef.current) return "";
      try {
        const resp = await clientRef.current.getTerminalSnapshot({ sessionId, lastNLines });
        return resp.content ?? "";
      } catch {
        return "";
      }
    },
    []
  );

  const writeToSession = useCallback(
    async (sessionId: string, input: string, pressEnter = true): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        const resp = await clientRef.current.writeToSession({ sessionId, input, pressEnter });
        return resp.success ?? false;
      } catch {
        return false;
      }
    },
    []
  );

  const getConversationMessages = useCallback(
    async (sessionId: string, limit = 30): Promise<Array<{ role: string; content: string; timestamp?: string; model?: string }>> => {
      if (!clientRef.current) return [];
      try {
        const resp = await clientRef.current.getClaudeHistoryMessages({ id: sessionId, limit, tail: true });
        return (resp.messages ?? []).map((m) => ({
          role: m.role,
          content: m.content,
          timestamp: m.timestamp ? new Date(Number(m.timestamp.seconds) * 1000).toISOString() : undefined,
          model: m.model,
        }));
      } catch {
        return [];
      }
    },
    []
  );

  return {
    sessions,
    loading,
    error,
    connectionState,
    systemMemoryPct,
    reconnectAttemptCount,
    listSessions,
    getSession,
    createSession,
    updateSession,
    deleteSession,
    pauseSession,
    resumeSession,
    hibernateSession,
    resumeHibernatedSession,
    renameSession,
    restartSession,
    clearConversationState,
    acknowledgeSession,
    createCheckpoint,
    listCheckpoints,
    forkSession,
    runOneShot,
    listPromptHistory,
    watchSessions,
    stopWatching,
    archiveSession,
    unarchiveSession,
    listSessionsByWorkflow,
    runWorkflow,
    spawnShell,
    stopShell,
    restartShell,
    listShells,
    deleteShell,
    getTerminalSnapshot,
    writeToSession,
    getConversationMessages,
  };
}
