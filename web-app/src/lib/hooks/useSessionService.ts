"use client";

import { useEffect, useCallback, useRef, useMemo, useState } from "react";
import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { createSessionWatchTransport } from "@/lib/transport/watch-ws-transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import { Session, SessionStatus, Shell, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  CreateSessionRequest,
  UpdateSessionRequest,
  PromptHistoryEntry,
  RunOneShotResponse,
  DraftPullRequestResponse,
  CreatePullRequestResponse,
  SpawnShellRequest,
  RunWorkflowRequestSchema,
  ArchiveSessionRequestSchema,
  UnarchiveSessionRequestSchema,
  ListSessionsRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { SessionEvent, NotificationEvent } from "@/gen/session/v1/events_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BackoffState, getWsCloseCode, isNonRetriableConnectError } from "@/lib/utils/backoff";
import { RefreshCoordinator } from "@/lib/utils/refreshCoordinator";
import type { ListSessionsResponse } from "@/gen/session/v1/session_pb";
import { createRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { getErrorMessage } from "@/lib/utils/connectError";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { useNotifications, getFailureReasonToastMessage } from "@/lib/contexts/NotificationContext";
import { useAppDispatch, useAppSelector } from "@/lib/store";
// NOTE (backlog #488, 2026-08-17): this file's other `dispatch(setError(...))`
// sites are deliberately NOT converted to the getErrorMessage() helper
// (web-app/src/lib/utils/connectError.ts) used everywhere in components/backlog/**.
// `setError` here writes to the shared sessionsSlice Redux store, consumed by
// non-backlog surfaces (the general session list/terminal views) — stripping the
// ConnectRPC `[code]` prefix there is a behavior change with a materially larger
// blast radius than the local useState catch-block conversions this PR made, and
// is out of scope for a backlog-toast presentation fix. Tracked as a follow-up,
// not fixed here.
//
// updateSession()'s setError call below is the one exception: BacklogItemDetail.tsx's
// handleSteerSession reads this exact value back via selectSessionsError() and
// re-throws it into a backlog action toast, so it IS a backlog-facing surface
// this PR touches — stripped here to match.
import {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  setErrorCode,
  setConnectionState,
  selectAllSessions,
  selectSessionsLoading,
  selectSessionsError,
  selectConnectionState,
  removeDetectedStatus,
} from "@/lib/store/sessionsSlice";
import { remoteHealthChanged } from "@/lib/store/remotesSlice";

// ponytail: stable empty array so non-watching callers (e.g. useSessionActions
// in each SessionCard) don't subscribe to the full sessions list. Without this,
// 29 cards × N events/second = N*29 card re-renders/second saturating the JS thread.
const EMPTY_SESSIONS: Session[] = [];
const selectNoSessions = () => EMPTY_SESSIONS;
import { removeItem as removeReviewQueueItem } from "@/lib/store/reviewQueueSlice";

// Bounds the createSession RPC so a stalled backend (e.g. a stuck tmux start
// or hung GitHub clone) can't leave the omnibar's Create button grayed out
// forever — the promise always settles, letting the caller's catch handler
// reset isSubmitting and surface an error. Kept comfortably above the
// server's own createSessionTimeout (150s in session_service.go); if that
// value changes, update this one too.
const CREATE_SESSION_TIMEOUT_MS = 160_000;

// Bounds every ListSessions RPC (all 4 call sites share refreshCoordinatorRef,
// see below) so a hung/slow backend can't wedge the coordinator's single
// in-flight slot forever — adversarial-review Blocker 1. A read-only list
// call is expected to be fast; well above typical latency but far below
// CREATE_SESSION_TIMEOUT_MS since no backend work (tmux/clone) is involved.
export const LIST_SESSIONS_TIMEOUT_MS = 15_000;

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
  draftPullRequest: (sessionId: string) => Promise<DraftPullRequestResponse | null>;
  createPullRequest: (req: {
    sessionId: string;
    title: string;
    body: string;
    baseBranch: string;
  }) => Promise<CreatePullRequestResponse | null>;
  listPromptHistory: (limit?: number) => Promise<PromptHistoryEntry[]>;
  pauseSession: (id: string) => Promise<Session | null>;
  resumeSession: (id: string, updates?: { title?: string; tags?: string[] }) => Promise<Session | null>;
  hibernateSession: (id: string) => Promise<Session | null>;
  resumeHibernatedSession: (id: string) => Promise<Session | null>;
  resumeCrashedSession: (id: string) => Promise<Session | null>;
  renameSession: (id: string, newTitle: string) => Promise<boolean>;
  restartSession: (id: string) => Promise<boolean>;
  retrySession: (id: string) => Promise<boolean>;
  cancelSessionCreation: (id: string) => Promise<{ success: boolean; lostRace: boolean }>;
  retrySessionCreation: (id: string) => Promise<boolean>;
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
  // Both reject on a genuine fetch failure (network/RPC error) rather than
  // resolving to an empty result — callers must distinguish "fetch failed" from
  // "genuinely no output yet" and surface the former as a retryable error state.
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
  const { addNotification } = useNotifications();
  const [systemMemoryPct, setSystemMemoryPct] = useState<number>(0);
  const [reconnectAttemptCount, setReconnectAttemptCount] = useState(0);
  const sessions = useAppSelector(autoWatch ? selectAllSessions : selectNoSessions);
  const loading = useAppSelector(selectSessionsLoading);
  const errorStr = useAppSelector(selectSessionsError);

  // Async-session-creation Epic 5.3 (Surface 4): fire a global failure toast
  // exactly once per Creating -> Failed transition, regardless of which
  // page/session the user is currently viewing. This reads Redux `sessions`
  // state (populated by both the WatchSessions stream and ListSessions
  // snapshots via the same upsertSession/setSessions actions) rather than
  // hooking the raw stream event directly, so it fires identically whichever
  // path delivered the transition.
  //
  // `lastKnownStatusRef` is the dedup guard: a session's id is only ever
  // recorded here on a render where it was already present with a
  // *different* status, so (a) a session's first-ever appearance (including
  // one that is already Failed on initial load, e.g. after a page refresh)
  // never fires a toast, and (b) once fired, subsequent renders with the
  // same Failed status never re-fire — a status transition notifies exactly
  // once, not once per re-render or redundant stream event.
  const lastKnownStatusRef = useRef<Map<string, SessionStatus>>(new Map());
  useEffect(() => {
    if (!enabled || !autoWatch) return;
    const seen = lastKnownStatusRef.current;
    for (const s of sessions) {
      const prevStatus = seen.get(s.id);
      if (
        prevStatus !== undefined &&
        prevStatus !== SessionStatus.FAILED &&
        s.status === SessionStatus.FAILED
      ) {
        addNotification({
          sessionId: s.id,
          sessionName: s.title || s.id,
          title: "Session creation failed",
          message: getFailureReasonToastMessage(s.failureReason),
          notificationType: "task_failed",
          priority: "high",
          metadata: { failure_reason: s.failureReason },
        });
      }
      seen.set(s.id, s.status);
    }
    // Prune ids no longer present so a deleted-then-recreated session (same
    // title reused) is treated as a fresh first-appearance, not a transition.
    const currentIds = new Set(sessions.map((s) => s.id));
    for (const id of seen.keys()) {
      if (!currentIds.has(id)) seen.delete(id);
    }
  }, [sessions, enabled, autoWatch, addNotification]);

  const abortControllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  // Reconnect control: true while watchSessions is active (user did not explicitly stop)
  const shouldReconnectRef = useRef(false);
  // Jittered exponential backoff state
  const backoffRef = useRef(new BackoffState(1000, 30_000));
  // Coalesces concurrent ListSessions fetches across all 4 call sites in
  // this hook (listSessions, watch-stream initial snapshot, backwards-jump
  // resync ×2, staleness-backstop reconnect) — see refreshCoordinator.ts.
  const refreshCoordinatorRef = useRef(new RefreshCoordinator<ListSessionsResponse>());
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
    const transport = createSessionWatchTransport({
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
        await refreshCoordinatorRef.current.request(
          () =>
            clientRef.current!.listSessions(
              {
                category: listOptions?.category,
                status: listOptions?.status,
                includeArchived: listOptions?.includeArchived,
              },
              { timeoutMs: LIST_SESSIONS_TIMEOUT_MS }
            ),
          (response) => {
            dispatch(setSessions(response.sessions));
            dispatch(setError(null)); // Clear any previous errors
            if (response.systemMemoryPct > 0) {
              setSystemMemoryPct(response.systemMemoryPct);
            }
          }
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error("Failed to list sessions");
        dispatch(setError(error.message));
        console.error("Failed to list sessions:", error);
      } finally {
        // Known limitation (sdd:6-verify follow-up, not fixed here): this
        // clears as soon as THIS call's own request() settles, even if it
        // got coalesced behind a still-in-flight rerun whose data hasn't
        // landed yet — `loading` can briefly read false mid-refresh. A
        // correct fix needs a site-scoped "is my own work done" signal, not
        // the coordinator's shared busy state (which also reflects unrelated
        // background stream reconnects from sites #2/#3/#3b).
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
        const response = await clientRef.current.createSession(
          {
            title: request.title ?? "",
            path: request.path ?? "",
            workingDir: request.workingDir,
            branch: request.branch,
            program: request.program,
            category: request.category,
            prompt: request.prompt,
            autoYes: request.autoYes,
            autoApprove: request.autoApprove ?? false,
            existingWorktree: request.existingWorktree,
            sessionType: request.sessionType,
            createIfMissing: request.createIfMissing ?? false,
            initialPrompt: request.initialPrompt,
            autonomousMode: request.autonomousMode ?? false,
            // No default (unlike autonomousMode) -- an omitted remote must stay omitted on
            // the wire, not coerced to a zero-value RemoteTarget, so local session creation
            // is byte-identical to pre-change behavior (ADR-001: remote-as-orthogonal-flag).
            remote: request.remote,
            permissionMode: request.permissionMode ?? "",
            aliasName: request.aliasName ?? "",
            cliFlags: request.cliFlags ?? "",
            extraArgs: request.extraArgs ?? [],
            restartFromSessionId: request.restartFromSessionId,
            confirmRestartWithLiveSource: request.confirmRestartWithLiveSource,
          },
          { timeoutMs: CREATE_SESSION_TIMEOUT_MS }
        );

        // Add to store (with duplicate check handled by entity adapter upsertOne)
        if (response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.session ?? null;
      } catch (err) {
        const wrappedErr = err instanceof Error ? err : new Error("Failed to create session");
        // Deliberately NOT dispatch(setError(...)) here: `sessions.error` is what
        // PaneSplitRenderer's SessionListPaneBody checks to decide whether to render
        // the whole session list or replace it with a full-screen "Failed to Load
        // Sessions / Unable to connect to the server" state (see PaneSplitRenderer.tsx).
        // That's meant for list-load/watch-stream connectivity failures (listSessions'
        // and watchSessions' own catch blocks set it correctly). A createSession
        // rejection -- including an expected, synchronous validation failure like a
        // duplicate title -- is neither: the list loaded fine and the server is
        // reachable, it just refused this one request. Setting the same flag here
        // blanked the entire (already-populated) session list behind a misleading
        // connectivity error for as long as nothing else happened to clear it, which
        // is exactly what session-creation-async.spec.ts's "duplicate title keeps the
        // omnibar open with inline error" test was hitting: the omnibar's own
        // `omnibar-create-error` correctly showed the rejection, but the session card
        // it was asserting on had vanished behind this unrelated global error state.
        // The thrown error already reaches the caller (OmnibarContext/Omnibar.tsx),
        // which surfaces it via its own local, create-scoped error state -- no need
        // for a second, differently-scoped copy here.
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
          note: updates.note,
          title: updates.title,
          program: updates.program,
          tags: updates.tags ?? [],
          workingDir: updates.workingDir,
          rateLimitEnabled: updates.rateLimitEnabled,
          autonomousMode: updates.autonomousMode,
          autoApprove: updates.autoApprove,
          steerMessage: updates.steerMessage,
        });

        // Update in store
        if (response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.session ?? null;
      } catch (err) {
        console.error("[useSessionService] updateSession failed:", err);
        dispatch(setError(getErrorMessage(err, "Failed to update session")));
        // Board drag-rejection reconciliation (SessionBoard's attemptColumnMove) needs the
        // ConnectRPC code to distinguish a transport failure from a business-rule rejection —
        // see sessionsSlice.ts's errorCode doc comment.
        if (err instanceof ConnectError) {
          dispatch(setErrorCode(err.code));
        }
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
        if (err instanceof ConnectError) {
          dispatch(setErrorCode(err.code));
        }
        return null;
      }
    },
    [dispatch]
  );

  // Resume a crashed session (Crashed → Active). Server-side resume of a dead
  // tmux pane detected by SessionHealthChecker — threads --resume automatically
  // when a conversation UUID is known, no manual copy/paste required.
  const resumeCrashedSession = useCallback(
    async (id: string): Promise<Session | null> => {
      if (!clientRef.current) return null;
      dispatch(setError(null));
      try {
        const response = await clientRef.current.resumeCrashedSession({ id });
        if (response.session) dispatch(upsertSession(response.session));
        return response.session ?? null;
      } catch (err) {
        console.error("[useSessionService] resumeCrashedSession failed:", err);
        dispatch(setError(err instanceof Error ? err.message : "Failed to resume crashed session"));
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

  // Retry a session immediately, bypassing any pending backoff delay —
  // including from PERMANENTLY_FAILED (session-retry-backoff, AC6).
  const retrySession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.retrySession({ id });

        if (response.success && response.session) {
          dispatch(upsertSession(response.session));
        }

        return response.success;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to retry session"));
        return false;
      }
    },
    [dispatch]
  );

  // Cancel an in-progress (Creating) session's creation pipeline. Epic 5.4
  // (async-session-creation). Success means the instance was removed
  // server-side (session_service.go's CancelSessionCreation deletes it) --
  // the caller should remove the card from the list. A FailedPrecondition
  // means cancel lost the race with the pipeline's own terminal write (the
  // session is already Active/Failed); the normal watchSessions stream
  // event already carries that real status, so this just reports the loss
  // without touching the store -- no stale optimistic removal.
  const cancelSessionCreation = useCallback(
    async (id: string): Promise<{ success: boolean; lostRace: boolean }> => {
      if (!clientRef.current) return { success: false, lostRace: false };

      dispatch(setError(null));

      try {
        const response = await clientRef.current.cancelSessionCreation({ id });
        if (response.success) {
          dispatch(removeSession(id));
        }
        return { success: response.success, lostRace: false };
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
          // Pipeline won the race -- leave the store alone, the stream's
          // own status update (Active/Failed) is already on its way.
          return { success: false, lostRace: true };
        }
        dispatch(setError(err instanceof Error ? err.message : "Failed to cancel session creation"));
        return { success: false, lostRace: false };
      }
    },
    [dispatch]
  );

  // Retry a Failed session creation in place. Epic 5.4. The RPC itself only
  // returns a bool -- the same instance's status flips Failed -> Creating
  // server-side and the existing watchSessions stream delivers that update,
  // so this wrapper doesn't need to (and must not) synthesize a session
  // update itself. A FailedPrecondition means the instance was no longer
  // Failed by the time the retry command ran (e.g. a concurrent retry already
  // won); treated as a no-op failure, not an error toast.
  const retrySessionCreation = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.retrySessionCreation({ id });
        return response.success;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
          return false;
        }
        dispatch(setError(err instanceof Error ? err.message : "Failed to retry session creation"));
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

  // Fetch the pre-filled draft (title/body/base branch) for a session's PR (AC4)
  const draftPullRequest = useCallback(
    async (sessionId: string): Promise<DraftPullRequestResponse | null> => {
      if (!clientRef.current) return null;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.draftPullRequest({ sessionId });
        return response;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to draft pull request"));
        return null;
      }
    },
    [dispatch]
  );

  // Create the pull request for a session (AC3/AC7)
  const createPullRequest = useCallback(
    async (req: { sessionId: string; title: string; body: string; baseBranch: string }): Promise<CreatePullRequestResponse | null> => {
      if (!clientRef.current) return null;

      dispatch(setError(null));

      try {
        const response = await clientRef.current.createPullRequest({
          sessionId: req.sessionId,
          title: req.title,
          body: req.body,
          baseBranch: req.baseBranch,
        });
        return response;
      } catch (err) {
        dispatch(setError(err instanceof Error ? err.message : "Failed to create pull request"));
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
      case "remoteHealthChanged": {
        // ssh-remote-workspaces Epic 6.2: a configured remote's SSH connection
        // health transitioned (session/sshremote.RemoteHealthProber, pushed
        // over this same WatchSessions stream -- no separate subscription or
        // polling). Routed into remotesSlice so RemoteConnectionIndicator can
        // read it via selectRemoteConnectionState.
        const remoteHealth = event.event.value;
        if (remoteHealth.remoteName) {
          dispatch(remoteHealthChanged({
            remoteName: remoteHealth.remoteName,
            state: remoteHealth.state,
            previousState: remoteHealth.previousState,
          }));
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

      // Backwards-jump full resync, shared by both the stream-close and
      // stream-error paths below. Guarded — must not be silently dropped by
      // a later unguarded caller (adversarial-review Blocker 2).
      const runFullResync = (myGeneration: number) =>
        refreshCoordinatorRef.current.request(
          () =>
            clientRef.current!.listSessions(
              {
                category: watchOptionsRef.current?.categoryFilter,
                status: watchOptionsRef.current?.statusFilter,
              },
              { timeoutMs: LIST_SESSIONS_TIMEOUT_MS }
            ),
          (response) => {
            if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
              dispatch(setSessions(response.sessions));
            }
          },
          { guarded: true }
        );

      const startStream = async () => {
        if (!shouldReconnectRef.current || !clientRef.current) return;
        const myGeneration = ++streamGenerationRef.current;

        abortControllerRef.current = new AbortController();
        lastEventTimeRef.current = Date.now(); // Treat stream start as an activity timestamp

        try {
          // Initial snapshot before stream starts (pass active filters so the snapshot matches the stream).
          // Guarded: this reconnect-flush RPC must always fire, even if a
          // later, unguarded listSessions() call coalesces behind it
          // (adversarial-review Blocker 2).
          await refreshCoordinatorRef.current.request(
            () =>
              clientRef.current!.listSessions(
                {
                  category: watchOptionsRef.current?.categoryFilter,
                  status: watchOptionsRef.current?.statusFilter,
                },
                { timeoutMs: LIST_SESSIONS_TIMEOUT_MS }
              ),
            (response) => {
              if (!shouldReconnectRef.current || streamGenerationRef.current !== myGeneration) return;
              dispatch(setSessions(response.sessions));
            },
            { guarded: true }
          );

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

            // Handle backwards-jump: do a full resync.
            if (needsFullResyncRef.current) {
              needsFullResyncRef.current = false;
              runFullResync(myGeneration).catch((err) => {
                console.error("[reconnect] full resync failed:", err);
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

          // Check for non-retriable failures — a WS-bridge close code, or the
          // equivalent ConnectError code on the native transport (no
          // ws-close-code header exists there; see isNonRetriableConnectError).
          if (isNonRetriableConnectError(err)) {
            const wsCode = getWsCloseCode(err);
            const reason = wsCode !== null ? `close code=${wsCode}` : `connect code=${err.code}`;
            console.warn(`[reconnect] stream=watch non-retriable ${reason}, stopping reconnect`);
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

            // Handle backwards-jump: do a full resync.
            if (needsFullResyncRef.current) {
              needsFullResyncRef.current = false;
              runFullResync(myGeneration).catch((err) => {
                console.error("[reconnect] full resync failed:", err);
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
      // Deliberately not caught here: a fetch failure must propagate to the
      // caller (SessionMonitor) so it can render a distinct "failed to load"
      // state instead of silently rendering identically to real emptiness.
      const resp = await clientRef.current.getTerminalSnapshot({ sessionId, lastNLines });
      return resp.content ?? "";
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
      // See getTerminalSnapshot above: fetch failures propagate to the caller
      // instead of resolving to an empty array indistinguishable from "no
      // conversation history yet".
      const resp = await clientRef.current.getClaudeHistoryMessages({ id: sessionId, limit, tail: true });
      return (resp.messages ?? []).map((m) => ({
        role: m.role,
        content: m.content,
        timestamp: m.timestamp ? new Date(Number(m.timestamp.seconds) * 1000).toISOString() : undefined,
        model: m.model,
      }));
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
    resumeCrashedSession,
    renameSession,
    restartSession,
    retrySession,
    cancelSessionCreation,
    retrySessionCreation,
    clearConversationState,
    acknowledgeSession,
    createCheckpoint,
    listCheckpoints,
    forkSession,
    runOneShot,
    draftPullRequest,
    createPullRequest,
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
