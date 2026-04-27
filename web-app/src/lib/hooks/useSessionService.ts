"use client";

import { useEffect, useCallback, useRef, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { Session, SessionStatus, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  CreateSessionRequest,
  UpdateSessionRequest,
  PromptHistoryEntry,
  RunOneShotResponse,
} from "@/gen/session/v1/session_pb";
import { SessionEvent, NotificationEvent } from "@/gen/session/v1/events_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { createRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { useAppDispatch, useAppSelector } from "@/lib/store";
import {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  setConnectionState,
  updateSessionStatus,
  selectAllSessions,
  selectSessionsLoading,
  selectSessionsError,
  selectConnectionState,
} from "@/lib/store/sessionsSlice";

interface UseSessionServiceOptions {
  baseUrl?: string;
  autoWatch?: boolean;
  /** When false, suppresses all API calls (e.g. while auth is loading). Defaults to true. */
  enabled?: boolean;
  onNotification?: (notification: NotificationEvent) => void;
}

interface UseSessionServiceReturn {
  // State
  sessions: Session[];
  loading: boolean;
  error: Error | null;
  connectionState: import("@/lib/store/sessionsSlice").ConnectionState;

  // Methods
  listSessions: (options?: { category?: string; status?: SessionStatus }) => Promise<void>;
  getSession: (id: string) => Promise<Session | null>;
  createSession: (request: Partial<CreateSessionRequest>) => Promise<Session | null>;
  updateSession: (id: string, updates: Partial<UpdateSessionRequest>) => Promise<Session | null>;
  deleteSession: (id: string, force?: boolean) => Promise<boolean>;
  runOneShot: (sessionId: string, prompt: string, timeoutSeconds?: number) => Promise<RunOneShotResponse | null>;
  listPromptHistory: (limit?: number) => Promise<PromptHistoryEntry[]>;
  pauseSession: (id: string) => Promise<Session | null>;
  resumeSession: (id: string, updates?: { title?: string; tags?: string[] }) => Promise<Session | null>;
  renameSession: (id: string, newTitle: string) => Promise<boolean>;
  restartSession: (id: string) => Promise<boolean>;
  acknowledgeSession: (id: string) => Promise<boolean>;
  createCheckpoint: (sessionId: string, label: string) => Promise<boolean>;
  listCheckpoints: (sessionId: string) => Promise<import("@/gen/session/v1/types_pb").CheckpointProto[]>;
  forkSession: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;

  // Real-time updates
  watchSessions: (options?: { categoryFilter?: string; statusFilter?: SessionStatus }) => void;
  stopWatching: () => void;
}

export function useSessionService(
  options: UseSessionServiceOptions = {}
): UseSessionServiceReturn {
  const { baseUrl = getApiBaseUrl(), autoWatch = false, enabled = true, onNotification } = options;
  const onNotificationRef = useRef(onNotification);

  // Keep ref updated for callback in streaming loop
  useEffect(() => {
    onNotificationRef.current = onNotification;
  }, [onNotification]);

  const dispatch = useAppDispatch();
  const sessions = useAppSelector(selectAllSessions);
  const loading = useAppSelector(selectSessionsLoading);
  const errorStr = useAppSelector(selectSessionsError);

  const abortControllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  // Reconnect control: true while watchSessions is active (user did not explicitly stop)
  const shouldReconnectRef = useRef(false);
  // Backoff delay in ms, doubles on each failure up to 30s
  const reconnectDelayRef = useRef(1000);
  // Timestamp of last received stream event, used to detect staleness
  const lastEventTimeRef = useRef<number | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl,
      interceptors: [createAuthInterceptor(), createRpcTimingInterceptor()],
    });

    clientRef.current = createClient(SessionService, transport);
  }, [baseUrl]);

  // List sessions with retry logic
  const listSessions = useCallback(
    async (listOptions?: { category?: string; status?: SessionStatus }) => {
      if (!clientRef.current) return;

      dispatch(setLoading(true));
      dispatch(setError(null));

      try {
        const response = await clientRef.current.listSessions({
          category: listOptions?.category,
          status: listOptions?.status,
        });

        dispatch(setSessions(response.sessions));
        dispatch(setError(null)); // Clear any previous errors
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
          oneOff: request.oneOff ?? false,
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
        });

        // Update in store
        if (response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.session ?? null;
      } catch (err) {
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

  // Handle session events from watch stream
  const handleSessionEvent = useCallback((event: SessionEvent) => {
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
        break;
      }
      case "statusChanged": {
        const { sessionId, newStatus, detectedStatus, detectedContext } = event.event.value;
        // Dispatch into the reducer where state is always current.
        // This avoids capturing `sessions` in the closure, which would force
        // handleSessionEvent (and watchSessions) to reconnect on every change.
        dispatch(updateSessionStatus({
          sessionId,
          newStatus,
          detectedStatus: detectedStatus ?? undefined,
          detectedContext: detectedContext ?? undefined,
        }));
        break;
      }
      case "notification": {
        // Route notification events to the callback
        if (onNotificationRef.current) {
          onNotificationRef.current(event.event.value);
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

      // Stop any existing watch
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      shouldReconnectRef.current = true;
      reconnectDelayRef.current = 1000; // Reset backoff when explicitly (re)started

      const startStream = async () => {
        if (!shouldReconnectRef.current || !clientRef.current) return;

        abortControllerRef.current = new AbortController();
        lastEventTimeRef.current = Date.now(); // Treat stream start as an activity timestamp
        dispatch(setConnectionState("connected"));

        try {
          const stream = clientRef.current.watchSessions(
            {
              categoryFilter: watchOptions?.categoryFilter,
              statusFilter: watchOptions?.statusFilter,
            },
            { signal: abortControllerRef.current.signal }
          );

          for await (const event of stream) {
            lastEventTimeRef.current = Date.now();
            handleSessionEvent(event);
          }

          // Stream ended normally (server-side close). Reconnect if still desired.
          if (shouldReconnectRef.current) {
            dispatch(setConnectionState("disconnected"));
            // Refresh state before reconnecting — flushes changes missed while disconnected
            if (clientRef.current) {
              try {
                const response = await clientRef.current.listSessions({});
                dispatch(setSessions(response.sessions));
              } catch { /* best-effort */ }
            }
            await new Promise(r => setTimeout(r, reconnectDelayRef.current));
            reconnectDelayRef.current = Math.min(reconnectDelayRef.current * 2, 30_000);
            startStream();
          }
        } catch (err) {
          if (err instanceof Error && err.name === "AbortError") {
            return; // Intentional stop via stopWatching()
          }
          // Unexpected network error — log, refresh state, then reconnect
          dispatch(setError(err instanceof Error ? err.message : "Watch stream error"));
          if (shouldReconnectRef.current) {
            dispatch(setConnectionState("disconnected"));
            if (clientRef.current) {
              try {
                const response = await clientRef.current.listSessions({});
                dispatch(setSessions(response.sessions));
              } catch { /* best-effort */ }
            }
            await new Promise(r => setTimeout(r, reconnectDelayRef.current));
            reconnectDelayRef.current = Math.min(reconnectDelayRef.current * 2, 30_000);
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
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    dispatch(setConnectionState("disconnected"));
  }, [dispatch]);

  // Staleness detector: if no events for >15s, mark state as stale
  useEffect(() => {
    if (!enabled) return;
    const interval = setInterval(() => {
      if (
        lastEventTimeRef.current !== null &&
        shouldReconnectRef.current &&
        Date.now() - lastEventTimeRef.current > 15_000
      ) {
        dispatch(setConnectionState("stale"));
      }
    }, 5_000);
    return () => clearInterval(interval);
  }, [enabled, dispatch]);

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

  // Initial load (gated on auth being ready)
  useEffect(() => {
    if (!enabled) return;
    listSessions();
  }, [enabled, listSessions]);

  // Convert error string back to Error object for backward compatibility
  const error = useMemo(() => (errorStr ? new Error(errorStr) : null), [errorStr]);

  const connectionState = useAppSelector(selectConnectionState);

  return {
    sessions,
    loading,
    error,
    connectionState,
    listSessions,
    getSession,
    createSession,
    updateSession,
    deleteSession,
    pauseSession,
    resumeSession,
    renameSession,
    restartSession,
    acknowledgeSession,
    createCheckpoint,
    listCheckpoints,
    forkSession,
    runOneShot,
    listPromptHistory,
    watchSessions,
    stopWatching,
  };
}
