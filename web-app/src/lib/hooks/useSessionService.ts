"use client";

import { useEffect, useCallback, useRef, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { Session, SessionStatus, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  CreateSessionRequest,
  UpdateSessionRequest,
} from "@/gen/session/v1/session_pb";
import { SessionEvent, NotificationEvent } from "@/gen/session/v1/events_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { useAppDispatch, useAppSelector } from "@/lib/store";
import {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  updateSessionStatus,
  selectAllSessions,
  selectSessionsLoading,
  selectSessionsError,
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

  // Methods
  listSessions: (options?: { category?: string; status?: SessionStatus }) => Promise<void>;
  getSession: (id: string) => Promise<Session | null>;
  createSession: (request: Partial<CreateSessionRequest>) => Promise<Session | null>;
  updateSession: (id: string, updates: Partial<UpdateSessionRequest>) => Promise<Session | null>;
  deleteSession: (id: string, force?: boolean) => Promise<boolean>;
  pauseSession: (id: string) => Promise<Session | null>;
  resumeSession: (id: string) => Promise<Session | null>;
  renameSession: (id: string, newTitle: string) => Promise<boolean>;
  restartSession: (id: string) => Promise<boolean>;
  acknowledgeSession: (id: string) => Promise<boolean>;

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

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl,
      interceptors: [createAuthInterceptor()],
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

  // Resume session
  const resumeSession = useCallback(
    async (id: string): Promise<Session | null> => {
      return updateSession(id, {
        status: SessionStatus.RUNNING,
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
        const { sessionId, newStatus } = event.event.value;
        // Dispatch into the reducer where state is always current.
        // This avoids capturing `sessions` in the closure, which would force
        // handleSessionEvent (and watchSessions) to reconnect on every change.
        dispatch(updateSessionStatus({ sessionId, newStatus }));
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

  // Watch sessions for real-time updates
  const watchSessions = useCallback(
    (watchOptions?: { categoryFilter?: string; statusFilter?: SessionStatus }) => {
      if (!clientRef.current) return;

      // Stop any existing watch
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      abortControllerRef.current = new AbortController();

      (async () => {
        try {
          const stream = clientRef.current!.watchSessions(
            {
              categoryFilter: watchOptions?.categoryFilter,
              statusFilter: watchOptions?.statusFilter,
            },
            { signal: abortControllerRef.current!.signal }
          );

          for await (const event of stream) {
            handleSessionEvent(event);
          }
        } catch (err) {
          // Ignore abort errors
          if (err instanceof Error && err.name !== "AbortError") {
            dispatch(setError(err instanceof Error ? err.message : "Watch stream error"));
          }
        }
      })();
    },
    [handleSessionEvent, dispatch]
  );

  // Stop watching sessions
  const stopWatching = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
  }, []);

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

  return {
    sessions,
    loading,
    error,
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
    watchSessions,
    stopWatching,
  };
}
