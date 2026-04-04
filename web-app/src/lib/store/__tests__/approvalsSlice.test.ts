import { configureStore } from "@reduxjs/toolkit";
import approvalsReducer, {
  setApprovals,
  setLoading,
  setError,
  removeApproval,
  selectApprovals,
  selectApprovalsLoading,
  selectApprovalsError,
} from "../approvalsSlice";
import reviewQueueReducer from "../reviewQueueSlice";
import sessionsReducer from "../sessionsSlice";
import { PendingApprovalProto, PendingApprovalProtoSchema } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

// Mirror the real store shape so selectors receive the correct RootState type
// without needing `as any` casts. A fresh instance per-test prevents state leakage.
function makeStore() {
  return configureStore({
    reducer: { approvals: approvalsReducer, reviewQueue: reviewQueueReducer, sessions: sessionsReducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

function makeApproval(id: string): PendingApprovalProto {
  return create(PendingApprovalProtoSchema, { id, sessionId: `session-${id}` });
}

describe("approvalsSlice", () => {
  describe("initial state", () => {
    it("starts with empty approvals, not loading, no error", () => {
      const store = makeStore();
      const state = store.getState();
      expect(selectApprovals(state)).toEqual([]);
      expect(selectApprovalsLoading(state)).toBe(false);
      expect(selectApprovalsError(state)).toBeNull();
    });
  });

  describe("setApprovals", () => {
    it("replaces the approvals list", () => {
      const store = makeStore();
      const approvals = [makeApproval("a1"), makeApproval("a2")];
      store.dispatch(setApprovals(approvals));
      expect(selectApprovals(store.getState())).toHaveLength(2);
      expect(selectApprovals(store.getState())[0].id).toBe("a1");
    });

    it("replaces existing approvals on subsequent calls", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("old")]));
      store.dispatch(setApprovals([makeApproval("new1"), makeApproval("new2")]));
      const approvals = selectApprovals(store.getState());
      expect(approvals).toHaveLength(2);
      expect(approvals[0].id).toBe("new1");
    });

    it("accepts an empty array to clear approvals", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("a1")]));
      store.dispatch(setApprovals([]));
      expect(selectApprovals(store.getState())).toHaveLength(0);
    });
  });

  describe("removeApproval (optimistic update)", () => {
    it("removes the approval with the matching id", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("a1"), makeApproval("a2"), makeApproval("a3")]));
      store.dispatch(removeApproval("a2"));
      const approvals = selectApprovals(store.getState());
      expect(approvals).toHaveLength(2);
      expect(approvals.map((a) => a.id)).toEqual(["a1", "a3"]);
    });

    it("is a no-op when the id does not exist", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("a1")]));
      store.dispatch(removeApproval("nonexistent"));
      expect(selectApprovals(store.getState())).toHaveLength(1);
    });

    it("correctly removes the last item", () => {
      const store = makeStore();
      store.dispatch(setApprovals([makeApproval("only")]));
      store.dispatch(removeApproval("only"));
      expect(selectApprovals(store.getState())).toHaveLength(0);
    });
  });

  describe("setLoading", () => {
    it("sets loading to true", () => {
      const store = makeStore();
      store.dispatch(setLoading(true));
      expect(selectApprovalsLoading(store.getState())).toBe(true);
    });

    it("sets loading back to false", () => {
      const store = makeStore();
      store.dispatch(setLoading(true));
      store.dispatch(setLoading(false));
      expect(selectApprovalsLoading(store.getState())).toBe(false);
    });
  });

  describe("setError", () => {
    it("stores an error message", () => {
      const store = makeStore();
      store.dispatch(setError("fetch failed"));
      expect(selectApprovalsError(store.getState())).toBe("fetch failed");
    });

    it("clears the error with null", () => {
      const store = makeStore();
      store.dispatch(setError("some error"));
      store.dispatch(setError(null));
      expect(selectApprovalsError(store.getState())).toBeNull();
    });
  });

  describe("optimistic update + rollback pattern", () => {
    it("restores state after rollback via setApprovals", () => {
      const store = makeStore();
      const initial = [makeApproval("a1"), makeApproval("a2")];
      store.dispatch(setApprovals(initial));

      // Simulate optimistic remove
      store.dispatch(removeApproval("a1"));
      expect(selectApprovals(store.getState())).toHaveLength(1);

      // Simulate rollback (API failed, re-fetch restored original list)
      store.dispatch(setApprovals(initial));
      expect(selectApprovals(store.getState())).toHaveLength(2);
    });
  });
});
