"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { Session, SessionStatus, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  CreateSessionRequest,
  UpdateSessionRequest,
} from "@/gen/session/v1/session_pb";
import { SessionEvent, NotificationEvent } from "@/gen/session/v1/events_pb";
import { getApiBaseUrl } from "@/lib/config";

interface UseSessionServiceOptions {
  baseUrl?: string;
  autoWatch?: boolean;
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
  const { baseUrl = getApiBaseUrl(), autoWatch = false, onNotification } = options;
  const onNotificationRef = useRef(onNotification);

  // Keep ref updated for callback in streaming loop
  useEffect(() => {
    onNotificationRef.current = onNotification;
  }, [onNotification]);

  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef<any>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl,
    });

    clientRef.current = createPromiseClient(SessionService, transport);
  }, [baseUrl]);

  // List sessions with retry logic
  const listSessions = useCallback(
    async (listOptions?: { category?: string; status?: SessionStatus }) => {
      if (!clientRef.current) return;

      setLoading(true);
      setError(null);

      try {
        const response = await clientRef.current.listSessions({
          category: listOptions?.category,
          status: listOptions?.status,
        });

        setSessions(response.sessions);
        setError(null); // Clear any previous errors
      } catch (err) {
        const error = err instanceof Error ? err : new Error("Failed to list sessions");
        setError(error);
        console.error("Failed to list sessions:", error);
      } finally {
        setLoading(false);
      }
    },
    []
  );

  // Get single session
  const getSession = useCallback(async (id: string): Promise<Session | null> => {
    if (!clientRef.current) return null;

    try {
      const response = await clientRef.current.getSession({ id });
      return response.session ?? null;
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to get session"));
      return null;
    }
  }, []);

  // Create session
  const createSession = useCallback(
    async (request: Partial<CreateSessionRequest>): Promise<Session | null> => {
      if (!clientRef.current) return null;

      setError(null);

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

        // Add to local state (with duplicate check to handle race with watch stream)
        if (response.session) {
          setSessions((prev) => {
            if (prev.some((s) => s.id === response.session!.id)) {
              return prev;
            }
            return [...prev, response.session!];
          });
        }

        return response.session ?? null;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to create session"));
        return null;
      }
    },
    []
  );

  // Update session
  const updateSession = useCallback(
    async (
      id: string,
      updates: Partial<UpdateSessionRequest>
    ): Promise<Session | null> => {
      if (!clientRef.current) return null;

      setError(null);

      try {
        const response = await clientRef.current.updateSession({
          id,
          status: updates.status,
          category: updates.category,
          title: updates.title,
        });

        // Update local state
        if (response.session) {
          setSessions((prev) =>
            prev.map((s) => (s.id === id ? response.session! : s))
          );
        }

        return response.session ?? null;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to update session"));
        return null;
      }
    },
    []
  );

  // Delete session
  const deleteSession = useCallback(
    async (id: string, force: boolean = false): Promise<boolean> => {
      if (!clientRef.current) return false;

      setError(null);

      try {
        const response = await clientRef.current.deleteSession({ id, force });

        // Remove from local state
        if (response.success) {
          setSessions((prev) => prev.filter((s) => s.id !== id));
        }

        return response.success;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to delete session"));
        return false;
      }
    },
    []
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

      setError(null);

      try {
        const response = await clientRef.current.renameSession({
          id,
          title: newTitle
        });

        // Update local state
        if (response.success && response.session) {
          setSessions((prev) =>
            prev.map((s) => (s.id === id ? response.session! : s))
          );
        }

        return response.success;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to rename session"));
        return false;
      }
    },
    []
  );

  // Restart session
  const restartSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      setError(null);

      try {
        const response = await clientRef.current.restartSession({ id });

        // Update local state
        if (response.success && response.session) {
          setSessions((prev) =>
            prev.map((s) => (s.id === id ? response.session! : s))
          );
        }

        return response.success;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to restart session"));
        return false;
      }
    },
    []
  );

  // Acknowledge session (skip from review queue)
  const acknowledgeSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      setError(null);

      try {
        const response = await clientRef.current.acknowledgeSession({ id });
        return response.success;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to acknowledge session"));
        return false;
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
        setSessions((prev) => {
          // Avoid duplicates
          if (prev.some((s) => s.id === session.id)) {
            return prev;
          }
          return [...prev, session];
        });
        break;
      }
      case "sessionUpdated": {
        const session = event.event.value.session;
        if (!session) return;
        setSessions((prev) =>
          prev.map((s) => (s.id === session.id ? session : s))
        );
        break;
      }
      case "sessionDeleted": {
        const sessionId = event.event.value.sessionId;
        setSessions((prev) => prev.filter((s) => s.id !== sessionId));
        break;
      }
      case "statusChanged": {
        const { sessionId, newStatus } = event.event.value;
        setSessions((prev) =>
          prev.map((s) => {
            if (s.id === sessionId) {
              // Create new Session with updated status
              return new Session({
                ...s,
                status: newStatus,
              });
            }
            return s;
          })
        );
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
  }, []);

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
            setError(err);
          }
        }
      })();
    },
    [handleSessionEvent]
  );

  // Stop watching sessions
  const stopWatching = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
  }, []);

  // Auto-watch on mount if enabled
  useEffect(() => {
    if (autoWatch) {
      watchSessions();
    }

    return () => {
      stopWatching();
    };
  }, [autoWatch, watchSessions, stopWatching]);

  // Initial load
  useEffect(() => {
    listSessions();
  }, [listSessions]);

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
