"use client";

import React, { createContext, useContext, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import {
  CreateSessionRequest,
  UpdateSessionRequest,
  RunOneShotResponse,
  PromptHistoryEntry,
} from "@/gen/session/v1/session_pb";
import { CheckpointProto } from "@/gen/session/v1/types_pb";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useSessionNotifications } from "@/lib/hooks/useSessionNotifications";
import { useAuth } from "@/lib/contexts/AuthContext";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { getApiBaseUrl } from "@/lib/config";
import type { ConnectionState } from "@/lib/store/sessionsSlice";

interface SessionServiceContextValue {
  sessions: Session[];
  loading: boolean;
  error: Error | null;
  connectionState: ConnectionState;
  listSessions: (options?: { category?: string; status?: SessionStatus }) => Promise<void>;
  getSession: (id: string) => Promise<Session | null>;
  createSession: (request: Partial<CreateSessionRequest>) => Promise<Session | null>;
  updateSession: (id: string, updates: Partial<UpdateSessionRequest>) => Promise<Session | null>;
  deleteSession: (id: string, force?: boolean) => Promise<boolean>;
  pauseSession: (id: string) => Promise<Session | null>;
  resumeSession: (id: string, updates?: { title?: string; tags?: string[] }) => Promise<Session | null>;
  renameSession: (id: string, newTitle: string) => Promise<boolean>;
  restartSession: (id: string) => Promise<boolean>;
  clearConversationState: (id: string) => Promise<boolean>;
  acknowledgeSession: (id: string) => Promise<boolean>;
  createCheckpoint: (sessionId: string, label: string) => Promise<boolean>;
  listCheckpoints: (sessionId: string) => Promise<CheckpointProto[]>;
  forkSession: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;
  runOneShot: (sessionId: string, prompt: string, timeoutSeconds?: number) => Promise<RunOneShotResponse | null>;
  listPromptHistory: (limit?: number) => Promise<PromptHistoryEntry[]>;
  watchSessions: (options?: { categoryFilter?: string; statusFilter?: SessionStatus }) => void;
  stopWatching: () => void;
}

const SessionServiceContext = createContext<SessionServiceContextValue | null>(null);

/**
 * GlobalSessionServiceProvider mounts a single persistent watchSessions connection
 * that lives at the layout level. This ensures:
 *  - Notification toasts appear on every page, not just the main session list.
 *  - The streaming connection is not torn down on page navigation.
 *
 * page.tsx reads session state from Redux (which this provider keeps up-to-date)
 * and calls CRUD methods via useSessionServiceContext().
 */
export function GlobalSessionServiceProvider({ children }: { children: React.ReactNode }) {
  const { authEnabled, authenticated, loading: authLoading } = useAuth();
  const { refreshHistory } = useNotifications();
  const router = useRouter();

  // Navigate to the session detail when user clicks "View" on a toast.
  // Works from any page — redirects to /?session=<id> if not already on home.
  const onViewSession = useCallback((sessionId: string) => {
    router.push(`/?session=${encodeURIComponent(sessionId)}&tab=terminal`);
  }, [router]);

  const handleNotification = useSessionNotifications({
    enableAudio: true,
    onViewSession,
  });

  const service = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true,
    enabled: !authLoading && (!authEnabled || authenticated),
    onNotification: handleNotification,
    onReconnect: refreshHistory,
    onApprovalResponse: refreshHistory,
  });

  return (
    <SessionServiceContext.Provider value={service}>
      {children}
    </SessionServiceContext.Provider>
  );
}

export function useSessionServiceContext(): SessionServiceContextValue {
  const ctx = useContext(SessionServiceContext);
  if (!ctx) {
    throw new Error("useSessionServiceContext must be used within GlobalSessionServiceProvider");
  }
  return ctx;
}
