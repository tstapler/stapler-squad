import { configureStore } from "@reduxjs/toolkit";
import bulkSelectionReducer from "../bulkSelectionSlice";
import reviewQueueReducer from "../reviewQueueSlice";
import { connectApi } from "@/lib/api/connectApi";
import sessionsReducer, {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  selectAllSessions,
  selectSessionById,
  selectSessionIds,
  selectSessionsTotal,
  selectSessionsLoading,
  selectSessionsError,
  selectDetectedStatusMap,
  removeDetectedStatus,
} from "../sessionsSlice";
import { Session, SessionSchema, SessionStatus, SubStatus, DetectedStatus } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

function makeStore() {
  return configureStore({
    reducer: { bulkSelection: bulkSelectionReducer, reviewQueue: reviewQueueReducer, sessions: sessionsReducer, [connectApi.reducerPath]: connectApi.reducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }).concat(connectApi.middleware),
  });
}

function makeSession(id: string, title = `Session ${id}`): Session {
  return create(SessionSchema, { id, title });
}

describe("sessionsSlice", () => {
  describe("initial state", () => {
    it("starts with no sessions, not loading, no error", () => {
      const store = makeStore();
      const state = store.getState() as any;
      expect(selectAllSessions(state)).toEqual([]);
      expect(selectSessionsTotal(state)).toBe(0);
      expect(selectSessionsLoading(state)).toBe(false);
      expect(selectSessionsError(state)).toBeNull();
    });
  });

  describe("setSessions", () => {
    it("replaces all sessions (setAll semantics)", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      expect(selectSessionsTotal(store.getState())).toBe(2);
    });

    it("replaces existing sessions on a subsequent call (no accumulation)", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      store.dispatch(setSessions([makeSession("s3")]));
      const state = store.getState() as any;
      expect(selectSessionsTotal(state)).toBe(1);
      expect(selectAllSessions(state)[0].id).toBe("s3");
    });

    it("accepts an empty array to clear all sessions", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1")]));
      store.dispatch(setSessions([]));
      expect(selectSessionsTotal(store.getState())).toBe(0);
    });
  });

  describe("upsertSession", () => {
    it("inserts a new session when id is not present", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeSession("new")));
      expect(selectSessionsTotal(store.getState())).toBe(1);
      expect(selectSessionById(store.getState() as any, "new")).toBeDefined();
    });

    it("updates an existing session in place (preserves other sessions)", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1", "Original"), makeSession("s2")]));
      store.dispatch(upsertSession(makeSession("s1", "Updated")));
      const state = store.getState() as any;
      expect(selectSessionsTotal(state)).toBe(2);
      expect(selectSessionById(state, "s1")!.title).toBe("Updated");
    });

    it("handles rapid successive upserts to the same id", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeSession("s1", "v1")));
      store.dispatch(upsertSession(makeSession("s1", "v2")));
      store.dispatch(upsertSession(makeSession("s1", "v3")));
      const state = store.getState() as any;
      expect(selectSessionsTotal(state)).toBe(1);
      expect(selectSessionById(state, "s1")!.title).toBe("v3");
    });
  });

  describe("removeSession", () => {
    it("removes the session with the matching id", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2"), makeSession("s3")]));
      store.dispatch(removeSession("s2"));
      const state = store.getState() as any;
      expect(selectSessionsTotal(state)).toBe(2);
      expect(selectSessionById(state, "s2")).toBeUndefined();
    });

    it("is a no-op for a non-existent id", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1")]));
      store.dispatch(removeSession("ghost"));
      expect(selectSessionsTotal(store.getState())).toBe(1);
    });

    it("can remove the last remaining session", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("only")]));
      store.dispatch(removeSession("only"));
      expect(selectSessionsTotal(store.getState())).toBe(0);
    });
  });

  describe("selectSessionById", () => {
    it("returns undefined for an id not in the store", () => {
      const store = makeStore();
      expect(selectSessionById(store.getState() as any, "missing")).toBeUndefined();
    });

    it("returns the correct session by id", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1", "Alpha"), makeSession("s2", "Beta")]));
      const session = selectSessionById(store.getState() as any, "s2");
      expect(session?.title).toBe("Beta");
    });
  });

  describe("selectSessionIds", () => {
    it("returns the list of ids in insertion order", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("a"), makeSession("b"), makeSession("c")]));
      expect(selectSessionIds(store.getState())).toEqual(["a", "b", "c"]);
    });
  });

  describe("setLoading / setError", () => {
    it("toggles loading", () => {
      const store = makeStore();
      store.dispatch(setLoading(true));
      expect(selectSessionsLoading(store.getState())).toBe(true);
      store.dispatch(setLoading(false));
      expect(selectSessionsLoading(store.getState())).toBe(false);
    });

    it("sets and clears error", () => {
      const store = makeStore();
      store.dispatch(setError("stream disconnected"));
      expect(selectSessionsError(store.getState())).toBe("stream disconnected");
      store.dispatch(setError(null));
      expect(selectSessionsError(store.getState())).toBeNull();
    });
  });

  describe("real-time event simulation (upsert + remove sequence)", () => {
    it("applies a create → update → delete event sequence correctly", () => {
      const store = makeStore();

      // Initial load
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));

      // New session arrives via stream
      store.dispatch(upsertSession(makeSession("s3", "New")));
      expect(selectSessionsTotal(store.getState())).toBe(3);

      // s2 gets a status update
      store.dispatch(upsertSession(makeSession("s2", "Updated")));
      expect(selectSessionById(store.getState() as any, "s2")!.title).toBe("Updated");

      // s1 is deleted
      store.dispatch(removeSession("s1"));
      expect(selectSessionsTotal(store.getState())).toBe(2);
      expect(selectSessionById(store.getState() as any, "s1")).toBeUndefined();
    });
  });

  describe("delete tombstone — prevents ghost resurrection on stream reconnect", () => {
    // Regression: when the WatchSessions stream reconnects it calls listSessions()
    // and dispatches setSessions() (full replace). If the server delete hadn't fully
    // propagated yet, the session was included in the response and reappeared in the UI.

    it("setSessions does not restore a session that was removed", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      store.dispatch(removeSession("s1"));

      // Simulate reconnect snapshot that still includes s1 (server lag)
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));

      const state = store.getState() as any;
      expect(selectSessionById(state, "s1")).toBeUndefined();
      expect(selectSessionsTotal(state)).toBe(1);
      expect(selectSessionById(state, "s2")).toBeDefined();
    });

    it("setSessions does not restore any of multiple removed sessions", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("a"), makeSession("b"), makeSession("c")]));
      store.dispatch(removeSession("a"));
      store.dispatch(removeSession("b"));

      // Reconnect snapshot includes all three (slow server)
      store.dispatch(setSessions([makeSession("a"), makeSession("b"), makeSession("c")]));

      const state = store.getState() as any;
      expect(selectSessionById(state, "a")).toBeUndefined();
      expect(selectSessionById(state, "b")).toBeUndefined();
      expect(selectSessionById(state, "c")).toBeDefined();
      expect(selectSessionsTotal(state)).toBe(1);
    });

    it("setSessions still loads non-deleted sessions normally", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("old")]));
      store.dispatch(removeSession("old"));

      // Reconnect brings in brand-new sessions
      store.dispatch(setSessions([makeSession("new1"), makeSession("new2")]));

      const state = store.getState() as any;
      expect(selectSessionById(state, "old")).toBeUndefined();
      expect(selectSessionsTotal(state)).toBe(2);
    });

    it("setSessions from a clean snapshot (no stale sessions) is unaffected", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      store.dispatch(removeSession("s1"));

      // Reconnect snapshot has already dropped s1 server-side
      store.dispatch(setSessions([makeSession("s2"), makeSession("s3")]));

      const state = store.getState() as any;
      expect(selectSessionsTotal(state)).toBe(2);
      expect(selectSessionById(state, "s1")).toBeUndefined();
      expect(selectSessionById(state, "s2")).toBeDefined();
      expect(selectSessionById(state, "s3")).toBeDefined();
    });

    it("upsertSession does not restore a removed session (in-flight update event)", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1")]));
      store.dispatch(removeSession("s1"));

      // A lagging sessionUpdated stream event arrives after the delete
      store.dispatch(upsertSession(makeSession("s1", "ghost update")));

      const state = store.getState() as any;
      expect(selectSessionById(state, "s1")).toBeUndefined();
      expect(selectSessionsTotal(state)).toBe(0);
    });

    it("tombstone survives repeated setSessions calls (multiple reconnects)", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      store.dispatch(removeSession("s1"));

      // Three consecutive reconnect snapshots still including s1
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));
      store.dispatch(setSessions([makeSession("s1"), makeSession("s2")]));

      const state = store.getState() as any;
      expect(selectSessionById(state, "s1")).toBeUndefined();
      expect(selectSessionById(state, "s2")).toBeDefined();
    });

    it("removeSession on an id not in the store still tombstones it", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("s2")]));

      // Remove a session that was never loaded (e.g. deleted before initial fetch)
      store.dispatch(removeSession("ghost"));

      store.dispatch(setSessions([makeSession("ghost"), makeSession("s2")]));

      const state = store.getState() as any;
      expect(selectSessionById(state, "ghost")).toBeUndefined();
      expect(selectSessionById(state, "s2")).toBeDefined();
    });

    it("tombstone does not affect sessions with different ids", () => {
      const store = makeStore();
      store.dispatch(setSessions([makeSession("target"), makeSession("bystander")]));
      store.dispatch(removeSession("target"));

      store.dispatch(setSessions([makeSession("bystander"), makeSession("newcomer")]));

      const state = store.getState() as any;
      expect(selectSessionById(state, "target")).toBeUndefined();
      expect(selectSessionById(state, "bystander")).toBeDefined();
      expect(selectSessionById(state, "newcomer")).toBeDefined();
      expect(selectSessionsTotal(state)).toBe(2);
    });
  });

  // Tests for upsertSession syncing detectedStatusMap from proto fields.
  // Replaces the old updateSessionStatus tests (that action was removed in Epic 5).
  describe("upsertSession — detectedStatusMap sync from proto fields", () => {
    function makeActiveSessionWithDetection(id: string, detectedStatus: DetectedStatus, detectedContext = ""): Session {
      return create(SessionSchema, {
        id,
        title: `Session ${id}`,
        status: SessionStatus.ACTIVE,
        detectedStatus,
        detectedContext,
      });
    }

    it("clears detectedStatusMap when session status is not ACTIVE", () => {
      const store = makeStore();
      // First seed a detected status entry by upserting an ACTIVE session
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.EXECUTING, "running")));
      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeDefined();

      // Now upsert the same session as STOPPED
      store.dispatch(upsertSession(create(SessionSchema, { id: "s1", status: SessionStatus.STOPPED })));

      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeUndefined();
    });

    it("sets detectedStatusMap when session is ACTIVE with typed DetectedStatus.EXECUTING", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.EXECUTING, "tool running")));

      const detected = selectDetectedStatusMap(store.getState() as any)["s1"];
      expect(detected?.detectedStatus).toBe(DetectedStatus.EXECUTING);
      expect(detected?.detectedContext).toBe("tool running");
    });

    it("clears detectedStatusMap when session is ACTIVE with DetectedStatus.UNSPECIFIED", () => {
      const store = makeStore();
      // Seed a detected entry
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.PROCESSING)));
      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeDefined();

      // Upsert with UNSPECIFIED detection
      store.dispatch(upsertSession(create(SessionSchema, {
        id: "s1",
        status: SessionStatus.ACTIVE,
        detectedStatus: DetectedStatus.UNSPECIFIED,
      })));

      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeUndefined();
    });

    it("clears detectedStatusMap when session is PAUSED", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.NEEDS_APPROVAL)));
      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeDefined();

      store.dispatch(upsertSession(create(SessionSchema, { id: "s1", status: SessionStatus.PAUSED })));

      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeUndefined();
    });

    it("does not affect detectedStatusMap entries for other sessions", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.EXECUTING)));
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s2", DetectedStatus.PROCESSING)));

      // Stop s1 only
      store.dispatch(upsertSession(create(SessionSchema, { id: "s1", status: SessionStatus.STOPPED })));

      expect(selectDetectedStatusMap(store.getState() as any)["s1"]).toBeUndefined();
      expect(selectDetectedStatusMap(store.getState() as any)["s2"]?.detectedStatus).toBe(DetectedStatus.PROCESSING);
    });
  });

  // UT-TS-06: removeDetectedStatus reducer
  describe("removeDetectedStatus", () => {
    function makeActiveSessionWithDetection(id: string, detectedStatus: DetectedStatus, detectedContext = ""): Session {
      return create(SessionSchema, {
        id,
        title: `Session ${id}`,
        status: SessionStatus.ACTIVE,
        detectedStatus,
        detectedContext,
      });
    }

    it("removes only the targeted session from detectedStatusMap, leaving others intact", () => {
      const store = makeStore();
      // Seed two detected status entries
      store.dispatch(upsertSession(makeActiveSessionWithDetection("abc", DetectedStatus.EXECUTING, "running")));
      store.dispatch(upsertSession(makeActiveSessionWithDetection("xyz", DetectedStatus.PROCESSING, "thinking")));

      // Remove only "abc"
      store.dispatch(removeDetectedStatus("abc"));

      const map = selectDetectedStatusMap(store.getState() as any);
      expect(map["abc"]).toBeUndefined();
      expect(map["xyz"]?.detectedStatus).toBe(DetectedStatus.PROCESSING);
    });

    it("is a no-op for an id not present in detectedStatusMap", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.EXECUTING)));

      // Remove a non-existent entry
      store.dispatch(removeDetectedStatus("ghost"));

      const map = selectDetectedStatusMap(store.getState() as any);
      expect(map["s1"]?.detectedStatus).toBe(DetectedStatus.EXECUTING);
    });

    it("does NOT modify any session entity in the store", () => {
      const store = makeStore();
      store.dispatch(upsertSession(makeActiveSessionWithDetection("s1", DetectedStatus.NEEDS_APPROVAL)));

      const sessionBefore = selectSessionById(store.getState() as any, "s1");
      expect(sessionBefore).toBeDefined();

      store.dispatch(removeDetectedStatus("s1"));

      // Session entity must still exist and be unmodified
      const sessionAfter = selectSessionById(store.getState() as any, "s1");
      expect(sessionAfter).toBeDefined();
      expect(sessionAfter?.id).toBe("s1");
      expect(selectSessionsTotal(store.getState())).toBe(1);
    });
  });
});
