import { configureStore } from "@reduxjs/toolkit";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import bulkSelectionReducer from "../bulkSelectionSlice";
import reviewQueueReducer from "../reviewQueueSlice";
import sessionsReducer from "../sessionsSlice";
import { connectApi } from "@/lib/api/connectApi";
import backlogItemsReducer, {
  upsertItem,
  removeItem,
  selectAllBacklogItems,
  selectBacklogItemById,
  selectBacklogItemsMap,
  selectBacklogItemsTotal,
  selectBacklogItemsLiveVersionMap,
  makeSelectBacklogItemsByStatus,
} from "../backlogItemsSlice";
import { BacklogItem, BacklogItemSchema, ItemSessionSchema } from "@/gen/session/v1/backlog_pb";

function makeStore() {
  return configureStore({
    reducer: {
      backlogItems: backlogItemsReducer,
      bulkSelection: bulkSelectionReducer,
      reviewQueue: reviewQueueReducer,
      sessions: sessionsReducer,
      [connectApi.reducerPath]: connectApi.reducer,
    },
    middleware: (getDefault) => getDefault({ serializableCheck: false }).concat(connectApi.middleware),
  });
}

function makeItem(
  id: string,
  status: string,
  updatedAtIso: string,
  sessionIds: string[] = []
): BacklogItem {
  return create(BacklogItemSchema, {
    id,
    status,
    updatedAt: timestampFromDate(new Date(updatedAtIso)),
    itemSessions: sessionIds.map((sessionUuid) => create(ItemSessionSchema, { sessionUuid })),
  });
}

describe("backlogItemsSlice", () => {
  describe("initial state", () => {
    it("starts with no items", () => {
      const store = makeStore();
      const state = store.getState() as any;
      expect(selectAllBacklogItems(state)).toEqual([]);
      expect(selectBacklogItemsTotal(state)).toBe(0);
    });
  });

  describe("upsertItem", () => {
    it("inserts a new item when id is not present", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.status).toBe("in_progress");
      expect(selectBacklogItemsTotal(state)).toBe(1);
    });

    it("drops an incoming item older than the stored one", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z")));
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:02Z")));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.status).toBe("review");
    });

    it("applies an incoming item newer than the stored one", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z")));
      store.dispatch(upsertItem(makeItem("item-1", "done", "2026-07-21T10:00:10Z")));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.status).toBe("done");
    });

    it("applies an incoming item with an equal timestamp (idempotent overwrite)", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z")));
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z")));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.status).toBe("review");
    });

    it("applies an item with no prior stored entry regardless of missing updatedAt", () => {
      const store = makeStore();
      const item = create(BacklogItemSchema, { id: "item-1", status: "in_progress" });
      store.dispatch(upsertItem(item));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.status).toBe("in_progress");
    });

    it("preserves other items when upserting one", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      store.dispatch(upsertItem(makeItem("item-2", "review", "2026-07-21T10:00:00Z")));
      store.dispatch(upsertItem(makeItem("item-1", "done", "2026-07-21T10:05:00Z")));
      const state = store.getState() as any;
      expect(selectBacklogItemsTotal(state)).toBe(2);
      expect(selectBacklogItemById(state, "item-2")?.status).toBe("review");
    });

    // pitfalls.md #2 / Epic 7.3: out-of-order concurrent-publish scenario —
    // final state must reflect the newer updatedAt regardless of dispatch order.
    it("final state reflects the newer update regardless of dispatch order", () => {
      const store = makeStore();
      const t1 = "2026-07-21T10:00:00.100Z";
      const t2 = "2026-07-21T10:00:00.200Z";
      // Dispatch the newer event first, then the older one arrives late.
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", t2)));
      store.dispatch(upsertItem(makeItem("item-1", "review", t1)));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.status).toBe("in_progress");
    });
  });

  // Backstop against a partially-loaded event push (item_sessions dropped by a
  // non-eager-loaded publish/snapshot path) clobbering sessions already in the
  // store — see backlogItemsSlice.ts's upsertItem doc comment.
  describe("upsertItem — itemSessions backstop", () => {
    it("preserves existing sessions when a newer update arrives with an empty itemSessions", () => {
      const store = makeStore();
      store.dispatch(
        upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z", ["session-a"]))
      );
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z", [])));
      const state = store.getState() as any;
      const item = selectBacklogItemById(state, "item-1");
      expect(item?.status).toBe("review");
      expect(item?.itemSessions.map((s: any) => s.sessionUuid)).toEqual(["session-a"]);
    });

    it("replaces sessions when the incoming update carries its own non-empty itemSessions", () => {
      const store = makeStore();
      store.dispatch(
        upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z", ["session-a"]))
      );
      store.dispatch(
        upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z", ["session-b"]))
      );
      const state = store.getState() as any;
      expect(
        selectBacklogItemById(state, "item-1")?.itemSessions.map((s: any) => s.sessionUuid)
      ).toEqual(["session-b"]);
    });

    it("does not backstop when the store had no prior sessions to preserve", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z", [])));
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z", [])));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")?.itemSessions).toEqual([]);
    });
  });

  // Epic 6.1 (backlog-event-driven-updates): liveVersion is the signal
  // BacklogItemCard's flash treatment watches — it must advance only for a
  // genuine live (isSnapshot: false) event, never for the default/snapshot
  // path, and never for an unrelated item.
  describe("upsertItem — liveVersion (Epic 6.1)", () => {
    it("does not bump liveVersion when isSnapshot is omitted (defaults to true)", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      const state = store.getState() as any;
      expect(selectBacklogItemsLiveVersionMap(state)["item-1"]).toBeUndefined();
    });

    it("does not bump liveVersion when isSnapshot is explicitly true", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z"), true));
      const state = store.getState() as any;
      expect(selectBacklogItemsLiveVersionMap(state)["item-1"]).toBeUndefined();
    });

    it("bumps liveVersion once per genuine live (isSnapshot: false) upsert", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z"), false));
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z"), false));
      const state = store.getState() as any;
      expect(selectBacklogItemsLiveVersionMap(state)["item-1"]).toBe(2);
    });

    it("leaves an unrelated item's liveVersion untouched", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z"), false));
      store.dispatch(upsertItem(makeItem("item-2", "review", "2026-07-21T10:00:00Z"), false));
      store.dispatch(upsertItem(makeItem("item-1", "done", "2026-07-21T10:05:00Z"), false));
      const state = store.getState() as any;
      expect(selectBacklogItemsLiveVersionMap(state)["item-1"]).toBe(2);
      expect(selectBacklogItemsLiveVersionMap(state)["item-2"]).toBe(1);
    });

    it("does not bump liveVersion when a stale (out-of-order) live event is dropped", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "review", "2026-07-21T10:00:05Z"), false));
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:02Z"), false));
      const state = store.getState() as any;
      expect(selectBacklogItemsLiveVersionMap(state)["item-1"]).toBe(1);
    });
  });

  describe("removeItem", () => {
    it("deletes the id from the map entirely", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      store.dispatch(removeItem("item-1"));
      const state = store.getState() as any;
      expect(selectBacklogItemById(state, "item-1")).toBeUndefined();
      expect(selectBacklogItemsTotal(state)).toBe(0);
    });

    it("is a no-op for an id that was never present", () => {
      const store = makeStore();
      store.dispatch(removeItem("does-not-exist"));
      const state = store.getState() as any;
      expect(selectBacklogItemsTotal(state)).toBe(0);
    });

    it("does not resurrect via a later upsert being required — removal is permanent for that dispatch", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      store.dispatch(removeItem("item-1"));
      const state = store.getState() as any;
      expect(selectBacklogItemsMap(state)).toEqual({});
    });
  });

  describe("memoized selectors", () => {
    it("selectAllBacklogItems returns a referentially stable array across unrelated re-reads", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      store.dispatch(upsertItem(makeItem("item-2", "review", "2026-07-21T10:00:00Z")));
      const state = store.getState() as any;
      const first = selectAllBacklogItems(state);
      const second = selectAllBacklogItems(state);
      expect(second).toBe(first);
    });

    it("selectAllBacklogItems recomputes only when the underlying map changes", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      const before = selectAllBacklogItems(store.getState() as any);
      store.dispatch(upsertItem(makeItem("item-2", "review", "2026-07-21T10:00:00Z")));
      const after = selectAllBacklogItems(store.getState() as any);
      expect(after).not.toBe(before);
      expect(after).toHaveLength(2);
    });

    it("makeSelectBacklogItemsByStatus filters correctly and stays stable when an unrelated item changes", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      store.dispatch(upsertItem(makeItem("item-2", "review", "2026-07-21T10:00:00Z")));

      const selectInProgress = makeSelectBacklogItemsByStatus();
      const firstResult = selectInProgress(store.getState() as any, "in_progress");
      expect(firstResult.map((i) => i.id)).toEqual(["item-1"]);

      // Update item-2 (a different status bucket); item-1's stored reference is unchanged,
      // so the "in_progress" filtered result should remain referentially stable.
      store.dispatch(upsertItem(makeItem("item-2", "done", "2026-07-21T10:05:00Z")));
      const secondResult = selectInProgress(store.getState() as any, "in_progress");
      expect(secondResult).toBe(firstResult);
    });

    it("makeSelectBacklogItemsByStatus produces independent instances for independent components", () => {
      const store = makeStore();
      store.dispatch(upsertItem(makeItem("item-1", "in_progress", "2026-07-21T10:00:00Z")));
      store.dispatch(upsertItem(makeItem("item-2", "review", "2026-07-21T10:00:00Z")));

      const selectA = makeSelectBacklogItemsByStatus();
      const selectB = makeSelectBacklogItemsByStatus();
      const state = store.getState() as any;
      expect(selectA(state, "in_progress").map((i) => i.id)).toEqual(["item-1"]);
      expect(selectB(state, "review").map((i) => i.id)).toEqual(["item-2"]);
    });
  });
});
