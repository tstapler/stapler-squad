import { compareByRepoPath, groupByRepoPath, NO_REPOSITORY } from "./sortGroup";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> & { id: string }): BacklogItem {
  return {
    title: overrides.id,
    status: "ready",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("compareByRepoPath", () => {
  it("sorts ascending by repo path", () => {
    const a = makeItem({ id: "a", repoPath: "org/beta" });
    const b = makeItem({ id: "b", repoPath: "org/alpha" });
    expect(compareByRepoPath(a, b)).toBeGreaterThan(0);
    expect(compareByRepoPath(b, a)).toBeLessThan(0);
  });

  it("treats missing repo_path as empty string without throwing", () => {
    const withPath = makeItem({ id: "a", repoPath: "org/alpha" });
    const withoutPath = makeItem({ id: "b" });
    expect(() => compareByRepoPath(withoutPath, withPath)).not.toThrow();
    expect(compareByRepoPath(withoutPath, withPath)).toBeLessThan(0);
    expect(compareByRepoPath(withPath, withoutPath)).toBeGreaterThan(0);
  });

  it("is stable (zero) for equal repo paths", () => {
    const a = makeItem({ id: "a", repoPath: "org/repo" });
    const b = makeItem({ id: "b", repoPath: "org/repo" });
    expect(compareByRepoPath(a, b)).toBe(0);
  });
});

describe("groupByRepoPath", () => {
  it("buckets items by repo path", () => {
    const items = [
      makeItem({ id: "1", repoPath: "org/beta" }),
      makeItem({ id: "2", repoPath: "org/alpha" }),
      makeItem({ id: "3", repoPath: "org/beta" }),
    ];
    const groups = groupByRepoPath(items);
    const beta = groups.find((g) => g.groupKey === "org/beta");
    expect(beta?.items.map((i) => i.title)).toEqual(["1", "3"]);
  });

  it("sorts groups alphabetically (case-insensitive)", () => {
    const items = [
      makeItem({ id: "1", repoPath: "Zeta" }),
      makeItem({ id: "2", repoPath: "alpha" }),
      makeItem({ id: "3", repoPath: "Beta" }),
    ];
    const groups = groupByRepoPath(items);
    expect(groups.map((g) => g.groupKey)).toEqual(["alpha", "Beta", "Zeta"]);
  });

  it("buckets items with no repo_path into a 'No Repository' group sorted last", () => {
    const items = [
      makeItem({ id: "1", repoPath: "org/alpha" }),
      makeItem({ id: "2" }), // no repoPath
      makeItem({ id: "3", repoPath: "" }), // empty repoPath
    ];
    const groups = groupByRepoPath(items);
    expect(groups[groups.length - 1].groupKey).toBe(NO_REPOSITORY);
    expect(groups[groups.length - 1].items.map((i) => i.title)).toEqual(["2", "3"]);
  });

  it("does not drop or duplicate any items", () => {
    const items = [
      makeItem({ id: "1", repoPath: "org/alpha" }),
      makeItem({ id: "2" }),
      makeItem({ id: "3", repoPath: "org/beta" }),
    ];
    const groups = groupByRepoPath(items);
    const total = groups.reduce((sum, g) => sum + g.items.length, 0);
    expect(total).toBe(items.length);
  });
});
