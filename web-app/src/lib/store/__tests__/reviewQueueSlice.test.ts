import { configureStore } from "@reduxjs/toolkit";
import bulkSelectionReducer from "../bulkSelectionSlice";
import sessionsReducer from "../sessionsSlice";
import { connectApi } from "@/lib/api/connectApi";
import reviewQueueReducer, {
  setReviewQueue,
  setReviewQueueStats,
  setLoading,
  setError,
  removeItem,
  addItem,
  updateItem,
  selectReviewQueue,
  selectReviewQueueItems,
  selectReviewQueueStats,
  selectReviewQueueLoading,
  selectReviewQueueError,
  selectReviewQueueItemsWithLiveStatus,
} from "../reviewQueueSlice";
import { ReviewQueue, ReviewItem, ReviewItemSchema, ReviewQueueSchema, DetectedStatus, SubStatus, WorkingState } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";
import { upsertSession } from "../sessionsSlice";
import { Session, SessionSchema, SessionStatus } from "@/gen/session/v1/types_pb";

function makeStore() {
  return configureStore({
    reducer: { bulkSelection: bulkSelectionReducer, reviewQueue: reviewQueueReducer, sessions: sessionsReducer, [connectApi.reducerPath]: connectApi.reducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }).concat(connectApi.middleware),
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

  // UT-TS-04 — addItem and updateItem reducers
  describe("addItem (UT-TS-04)", () => {
    it("Test A — inserts new item and updates totalItems", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([])));
      store.dispatch(addItem(makeReviewItem("sess-new")));
      const state = store.getState() as any;
      expect(selectReviewQueueItems(state)).toHaveLength(1);
      expect(selectReviewQueue(state)!.totalItems).toBe(1);
    });

    it("Test B — is idempotent: duplicate sessionId is not inserted", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("sess-1")])));
      store.dispatch(addItem(makeReviewItem("sess-1")));
      const state = store.getState() as any;
      expect(selectReviewQueueItems(state)).toHaveLength(1);
      expect(selectReviewQueue(state)!.totalItems).toBe(1);
    });

    it("is a no-op when queue is null", () => {
      const store = makeStore();
      expect(() => store.dispatch(addItem(makeReviewItem("sess-1")))).not.toThrow();
      expect(selectReviewQueue(store.getState())).toBeNull();
    });
  });

  describe("updateItem (UT-TS-04)", () => {
    it("Test C — modifies existing item in-place", () => {
      const store = makeStore();
      store.dispatch(
        setReviewQueue(
          makeQueue([
            create(ReviewItemSchema, { sessionId: "sess-1", subStatus: SubStatus.NEEDS_APPROVAL }),
          ])
        )
      );
      store.dispatch(
        updateItem({ sessionId: "sess-1", updates: { subStatus: SubStatus.PROCESSING } })
      );
      const items = selectReviewQueueItems(store.getState() as any);
      expect(items).toHaveLength(1);
      expect(items[0].subStatus).toBe(SubStatus.PROCESSING);
    });

    it("Test D — is a no-op for nonexistent sessionId", () => {
      const store = makeStore();
      store.dispatch(setReviewQueue(makeQueue([makeReviewItem("sess-1")])));
      expect(() =>
        store.dispatch(
          updateItem({ sessionId: "sess-nonexistent", updates: { subStatus: SubStatus.PROCESSING } })
        )
      ).not.toThrow();
      expect(selectReviewQueueItems(store.getState() as any)).toHaveLength(1);
    });

    it("is a no-op when queue is null", () => {
      const store = makeStore();
      expect(() =>
        store.dispatch(updateItem({ sessionId: "sess-1", updates: { subStatus: SubStatus.PROCESSING } }))
      ).not.toThrow();
      expect(selectReviewQueue(store.getState())).toBeNull();
    });
  });

  // UT-TS-08 — selectReviewQueueItemsWithLiveStatus joins live session state
  describe("selectReviewQueueItemsWithLiveStatus (UT-TS-08)", () => {
    it("overrides workingState using live detectedStatus when session is in detectedStatusMap", () => {
      const store = makeStore();
      // Queue item has UNSPECIFIED subStatus and UNSPECIFIED workingState (stale)
      store.dispatch(
        setReviewQueue(
          makeQueue([
            create(ReviewItemSchema, {
              sessionId: "sess-1",
              subStatus: SubStatus.UNSPECIFIED,
              workingState: WorkingState.UNSPECIFIED,
            }),
          ])
        )
      );
      // Live session with EXECUTING detected status → should override to WorkingState.ACTIVE
      store.dispatch(
        upsertSession(
          create(SessionSchema, {
            id: "sess-1",
            status: SessionStatus.ACTIVE,
            detectedStatus: DetectedStatus.EXECUTING,
          })
        )
      );
      const state = store.getState() as any;
      const result = selectReviewQueueItemsWithLiveStatus(state);
      expect(result).toHaveLength(1);
      expect(result[0].sessionId).toBe("sess-1");
      // Live detectedStatus EXECUTING → WorkingState.ACTIVE
      expect(result[0].workingState).toBe(WorkingState.ACTIVE);
    });

    it("returns item unchanged (identity) when session has no live detectedStatusMap entry", () => {
      const store = makeStore();
      const originalItem = create(ReviewItemSchema, {
        sessionId: "sess-2",
        subStatus: SubStatus.UNSPECIFIED,
        workingState: WorkingState.UNSPECIFIED,
      });
      store.dispatch(setReviewQueue(makeQueue([originalItem])));
      const state = store.getState() as any;
      const result = selectReviewQueueItemsWithLiveStatus(state);
      expect(result).toHaveLength(1);
      // No live session entry — item is returned as-is
      expect(result[0].sessionId).toBe("sess-2");
      expect(result[0].workingState).toBe(WorkingState.UNSPECIFIED);
    });

    it("overrides workingState to ACTIVE when live detectedStatus is EXECUTING", () => {
      const store = makeStore();
      store.dispatch(
        setReviewQueue(
          makeQueue([
            create(ReviewItemSchema, {
              sessionId: "sess-3",
              subStatus: SubStatus.UNSPECIFIED,
              workingState: WorkingState.UNSPECIFIED,
            }),
          ])
        )
      );
      // Inject a live detectedStatus via upsertSession (EXECUTING → WorkingState.ACTIVE)
      store.dispatch(
        upsertSession(
          create(SessionSchema, {
            id: "sess-3",
            status: SessionStatus.ACTIVE,
            detectedStatus: DetectedStatus.EXECUTING,
          })
        )
      );
      const state = store.getState() as any;
      const result = selectReviewQueueItemsWithLiveStatus(state);
      expect(result).toHaveLength(1);
      expect(result[0].workingState).toBe(WorkingState.ACTIVE);
    });

    it("subStatus takes priority over live detectedStatus per deriveWorkingState logic", () => {
      const store = makeStore();
      // Item has NEEDS_APPROVAL subStatus (should map to PROCESSING regardless of detectedStatus)
      store.dispatch(
        setReviewQueue(
          makeQueue([
            create(ReviewItemSchema, {
              sessionId: "sess-4",
              subStatus: SubStatus.NEEDS_APPROVAL,
              workingState: WorkingState.UNSPECIFIED,
            }),
          ])
        )
      );
      // Even with EXECUTING detectedStatus, subStatus takes priority
      store.dispatch(
        upsertSession(
          create(SessionSchema, {
            id: "sess-4",
            status: SessionStatus.ACTIVE,
            detectedStatus: DetectedStatus.EXECUTING,
          })
        )
      );
      const state = store.getState() as any;
      const result = selectReviewQueueItemsWithLiveStatus(state);
      expect(result).toHaveLength(1);
      // NEEDS_APPROVAL subStatus → PROCESSING (not ACTIVE from detectedStatus)
      expect(result[0].workingState).toBe(WorkingState.PROCESSING);
    });

    it("returns empty array when queue is null", () => {
      const store = makeStore();
      const state = store.getState() as any;
      expect(selectReviewQueueItemsWithLiveStatus(state)).toEqual([]);
    });
  });
});
