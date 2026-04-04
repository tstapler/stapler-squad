import { configureStore } from "@reduxjs/toolkit";
import approvalsReducer from "../approvalsSlice";
import sessionsReducer from "../sessionsSlice";
import reviewQueueReducer, {
  setReviewQueue,
  setReviewQueueStats,
  setLoading,
  setError,
  removeItem,
  selectReviewQueue,
  selectReviewQueueItems,
  selectReviewQueueStats,
  selectReviewQueueLoading,
  selectReviewQueueError,
} from "../reviewQueueSlice";
import { ReviewQueue, ReviewItem, ReviewItemSchema, ReviewQueueSchema } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

function makeStore() {
  return configureStore({
    reducer: { approvals: approvalsReducer, reviewQueue: reviewQueueReducer, sessions: sessionsReducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

function makeReviewItem(sessionId: string): ReviewItem {
  return create(ReviewItemSchema, { sessionId });
}

function makeQueue(items: ReviewItem[]): ReviewQueue {
  return create(ReviewQueueSchema, { items, totalItems: items.length });
}

describe("reviewQueueSlice", () => {
  describe("initial state", () => {
    it("starts with null queue, not loading, no error, zero stats", () => {
      const store = makeStore();
      const state = store.getState() as any;
      expect(selectReviewQueue(state)).toBeNull();
      expect(selectReviewQueueItems(state)).toEqual([]);
      expect(selectReviewQueueLoading(state)).toBe(false);
      expect(selectReviewQueueError(state)).toBeNull();
      expect(selectReviewQueueStats(state).totalItems).toBe(0);
    });
  });

  describe("setReviewQueue", () => {
    it("stores the queue and syncs totalItems into stats", () => {
      const store = makeStore();
      const queue = makeQueue([makeReviewItem("s1"), makeReviewItem("s2")]);
      store.dispatch(setReviewQueue(queue));
      const state = store.getState() as any;
      expect(selectReviewQueue(state)).toBe(queue);
      expect(selectReviewQueueItems(state)).toHaveLength(2);
      expect(selectReviewQueueStats(state).totalItems).toBe(2);
    });

    it("accepts null to clear the queue", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("s1")])));
      store.dispatch(setReviewQueue(null));
      const state = store.getState() as any;
      expect(selectReviewQueue(state)).toBeNull();
      expect(selectReviewQueueItems(state)).toEqual([]);
    });

    it("replaces an existing queue on refresh", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("old")])));
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("new1"), makeReviewItem("new2")])));
      expect(selectReviewQueueItems(store.getState())).toHaveLength(2);
    });
  });

  describe("removeItem (optimistic update)", () => {
    it("removes the item with the matching sessionId", () => {
      const store = makeStore();
      store.dispatch(
        setReviewQueue(makeQueue([makeReviewItem("s1"), makeReviewItem("s2"), makeReviewItem("s3")]))
      );
      store.dispatch(removeItem("s2"));
      const items = selectReviewQueueItems(store.getState());
      expect(items).toHaveLength(2);
      expect(items.map((i) => i.sessionId)).toEqual(["s1", "s3"]);
    });

    it("decrements totalItems in the queue and in stats", () => {
      const store = makeStore();
      store.dispatch(
        setReviewQueue(makeQueue([makeReviewItem("s1"), makeReviewItem("s2")]))
      );
      store.dispatch(removeItem("s1"));
      const state = store.getState() as any;
      expect(selectReviewQueue(state)!.totalItems).toBe(1);
      expect(selectReviewQueueStats(state).totalItems).toBe(1);
    });

    it("does not go below 0 for totalItems (boundary value)", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("s1")])));
      store.dispatch(removeItem("s1"));
      // Try to remove again — queue is now empty
      store.dispatch(removeItem("s1"));
      const state = store.getState() as any;
      expect(selectReviewQueueStats(state).totalItems).toBe(0);
    });

    it("is a no-op when queue is null", () => {
      const store = makeStore();
      // queue is null, should not throw
      expect(() => store.dispatch(removeItem("s1"))).not.toThrow();
      expect(selectReviewQueue(store.getState())).toBeNull();
    });

    it("is a no-op when sessionId does not match any item", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("s1")])));
      store.dispatch(removeItem("nonexistent"));
      expect(selectReviewQueueItems(store.getState())).toHaveLength(1);
    });
  });

  describe("setReviewQueueStats", () => {
    it("replaces the stats object", () => {
      const store = makeStore();
      const newStats = {
        totalItems: 5,
        byPriority: { 1: 3, 2: 2 },
        byReason: { 0: 5 },
        averageAgeSeconds: "120",
        oldestItemId: "s1",
        oldestAgeSeconds: "300",
      };
      store.dispatch(setReviewQueueStats(newStats));
      expect(selectReviewQueueStats(store.getState())).toEqual(newStats);
    });
  });

  describe("setLoading / setError", () => {
    it("toggles loading", () => {
      const store = makeStore();
      store.dispatch(setLoading(true));
      expect(selectReviewQueueLoading(store.getState())).toBe(true);
      store.dispatch(setLoading(false));
      expect(selectReviewQueueLoading(store.getState())).toBe(false);
    });

    it("sets and clears error", () => {
      const store = makeStore();
      store.dispatch(setError("network error"));
      expect(selectReviewQueueError(store.getState())).toBe("network error");
      store.dispatch(setError(null));
      expect(selectReviewQueueError(store.getState())).toBeNull();
    });
  });

  describe("optimistic update + rollback pattern", () => {
    it("restores items after rollback via setReviewQueue", () => {
      const store = makeStore();
      const original = makeQueue([makeReviewItem("s1"), makeReviewItem("s2")]);
      store.dispatch(setReviewQueue(original));

      store.dispatch(removeItem("s1"));
      expect(selectReviewQueueItems(store.getState())).toHaveLength(1);

      // Rollback: re-fetch restored the original
      store.dispatch(setReviewQueue(original));
      expect(selectReviewQueueItems(store.getState())).toHaveLength(2);
      expect(selectReviewQueueStats(store.getState()).totalItems).toBe(2);
    });
  });
});
