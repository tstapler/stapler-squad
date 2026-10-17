/**
 * Direct unit coverage for useBacklogFilters.ts — previously only exercised
 * indirectly through BacklogBoard.filters.test.tsx, which never tested the
 * `search` field or combined filters (status+priority+search together).
 *
 * Also proves the hook's core claim for this PR ("board now shares filter
 * state with list"): two independent consumers reading/writing through
 * useBacklogFilters() round-trip a value via localStorage.
 */

import { act, renderHook, waitFor } from "@testing-library/react";
import { filterBacklogItems, useBacklogFilters, type BacklogFilterState } from "./useBacklogFilters";
import type { BacklogItem } from "./useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Test item",
    description: "",
    status: "idea",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    liveVersion: 1,
    ...overrides,
  } as BacklogItem;
}

const NO_OP_FILTERS: BacklogFilterState = {
  search: "",
  statusFilter: [],
  priorityFilter: [],
  sourceFilter: [],
  showArchived: false,
};

describe("filterBacklogItems", () => {
  const items = [
    makeItem({ id: "1", title: "Fix login bug", description: "Auth flow is broken", status: "idea", priority: 1 }),
    makeItem({ id: "2", title: "Add dashboard widget", description: "Show recent activity", status: "ready", priority: 3 }),
    makeItem({ id: "3", title: "Refactor session store", description: "Cleanup login helpers", status: "ready", priority: 1 }),
    makeItem({ id: "4", title: "Archived thing", description: "", status: "archived", priority: 5 }),
  ];

  it.each([
    {
      name: "search alone matches case-insensitively on title",
      filters: { ...NO_OP_FILTERS, search: "LOGIN" },
      expectedIds: ["1", "3"], // "Fix login bug" (title) + "Refactor session store" (description: "login helpers")
    },
    {
      name: "search alone matches case-insensitively on description",
      filters: { ...NO_OP_FILTERS, search: "recent activity" },
      expectedIds: ["2"],
    },
    {
      name: "status alone",
      filters: { ...NO_OP_FILTERS, statusFilter: ["ready"] },
      expectedIds: ["2", "3"],
    },
    {
      name: "priority alone",
      filters: { ...NO_OP_FILTERS, priorityFilter: [1] },
      expectedIds: ["1", "3"],
    },
    {
      name: "status + priority + search combined use AND semantics",
      filters: { ...NO_OP_FILTERS, statusFilter: ["ready"], priorityFilter: [1], search: "login" },
      expectedIds: ["3"], // only item 3 satisfies all three dimensions at once
    },
    {
      name: "combined filters exclude an item matching only some dimensions",
      filters: { ...NO_OP_FILTERS, statusFilter: ["ready"], priorityFilter: [3], search: "login" },
      expectedIds: [], // item 2 matches status+priority but not search; item 3 matches status+search but not priority
    },
    {
      name: "archived items excluded by default regardless of other filters",
      filters: { ...NO_OP_FILTERS, priorityFilter: [5] },
      expectedIds: [],
    },
    {
      name: "showArchived reveals archived items when explicitly enabled",
      filters: { ...NO_OP_FILTERS, showArchived: true, priorityFilter: [5] },
      expectedIds: ["4"],
    },
  ])("filterBacklogItems_should_ApplyAllActiveFilterDimensions_When_$name", ({ filters, expectedIds }) => {
    const result = filterBacklogItems(items, filters as BacklogFilterState);
    expect(result.map((i) => i.id).sort()).toEqual([...expectedIds].sort());
  });
});

describe("useBacklogFilters persistence (shared state across independent consumers)", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("useBacklogFilters_should_HydrateSetterValueFromLocalStorage_When_ASecondIndependentInstanceMountsFresh", async () => {
    const first = renderHook(() => useBacklogFilters());
    await waitFor(() => expect(first.result.current.isHydrated).toBe(true));

    act(() => {
      first.result.current.setters.statusFilter(["ready", "review"]);
      first.result.current.setters.search("widget");
    });

    await waitFor(() => {
      expect(window.localStorage.getItem("stapler-squad-backlog-status-filter")).toBe(
        JSON.stringify(["ready", "review"])
      );
    });

    first.unmount();

    // A fresh, independent instance (e.g. the board view mounting after the
    // list view set a filter) must hydrate the same values from the shared
    // localStorage keys — this is the literal claim this PR makes.
    const second = renderHook(() => useBacklogFilters());
    await waitFor(() => expect(second.result.current.isHydrated).toBe(true));

    expect(second.result.current.state.statusFilter).toEqual(["ready", "review"]);
    expect(second.result.current.state.search).toBe("widget");
  });
});

describe("filterBacklogItems sourceFilter", () => {
  const items = [
    { ...makeItem({ id: "a" }), pluginId: "linear" },
    { ...makeItem({ id: "b" }), pluginId: "github_issues" },
    { ...makeItem({ id: "c" }) },
  ] as BacklogItem[];

  it("keeps only items from the selected source", () => {
    const out = filterBacklogItems(items, { ...NO_OP_FILTERS, sourceFilter: ["linear"] });
    expect(out.map((i) => i.id)).toEqual(["a"]);
  });

  it("matches an imported item lacking pluginId under the GitHub chip", () => {
    const legacy = [
      makeItem({ id: "legacy", externalUrl: "https://github.com/acme/w/issues/1", externalId: "1" }),
      makeItem({ id: "manual" }),
    ] as BacklogItem[];
    const out = filterBacklogItems(legacy, { ...NO_OP_FILTERS, sourceFilter: ["github_issues"] });
    expect(out.map((i) => i.id)).toEqual(["legacy"]);
  });

  it("shows everything when no source is selected", () => {
    expect(filterBacklogItems(items, NO_OP_FILTERS)).toHaveLength(3);
  });
});
