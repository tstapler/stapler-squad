import { configureStore } from "@reduxjs/toolkit";
import approvalsReducer from "../approvalsSlice";
import reviewQueueReducer from "../reviewQueueSlice";
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
} from "../sessionsSlice";
import { Session, SessionSchema } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

function makeStore() {
  return configureStore({
    reducer: { approvals: approvalsReducer, reviewQueue: reviewQueueReducer, sessions: sessionsReducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
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
});
