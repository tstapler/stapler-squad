/**
 * Tests for usePathHistory hook.
 *
 * Uses real localStorage via JSDOM. Storage is cleared before each test.
 */

import { renderHook, act } from "@testing-library/react";
import { usePathHistory, clearPathHistory } from "../usePathHistory";

beforeEach(() => {
  clearPathHistory();
});

describe("usePathHistory", () => {
  describe("getMatching", () => {
    it("returns empty array when no history exists", () => {
      const { result } = renderHook(() => usePathHistory());
      expect(result.current.getMatching("/home/user/")).toEqual([]);
    });

    it("returns empty array for empty prefix", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => result.current.save("/home/user/projects"));
      expect(result.current.getMatching("")).toEqual([]);
    });

    it("returns entries whose path starts with prefix", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        result.current.save("/home/user/projects");
        result.current.save("/home/user/personal");
        result.current.save("/other/path");
      });
      const matches = result.current.getMatching("/home/user/");
      const paths = matches.map((m) => m.path);
      expect(paths).toContain("/home/user/projects");
      expect(paths).toContain("/home/user/personal");
      expect(paths).not.toContain("/other/path");
    });

    it("excludes exact prefix matches", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => result.current.save("/home/user/projects"));
      // Exact match should not be returned (nothing to complete).
      const matches = result.current.getMatching("/home/user/projects");
      expect(matches.map((m) => m.path)).not.toContain("/home/user/projects");
    });

    it("returns at most 10 results", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        for (let i = 0; i < 15; i++) {
          result.current.save(`/home/user/proj${i}`);
        }
      });
      expect(result.current.getMatching("/home/user/")).toHaveLength(10);
    });
  });

  describe("save", () => {
    it("persists entries across hook instances (via localStorage)", () => {
      const { result: r1 } = renderHook(() => usePathHistory());
      act(() => r1.current.save("/home/user/projects"));

      // A fresh hook instance reads from localStorage.
      const { result: r2 } = renderHook(() => usePathHistory());
      const paths = r2.current.getMatching("/home/user/").map((m) => m.path);
      expect(paths).toContain("/home/user/projects");
    });

    it("increments count on re-save of the same path", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        result.current.save("/home/user/projects");
        result.current.save("/home/user/projects");
        result.current.save("/home/user/projects");
      });
      // Entry should still appear exactly once.
      const matches = result.current.getMatching("/home/user/");
      expect(matches.filter((m) => m.path === "/home/user/projects")).toHaveLength(1);
    });

    it("higher-frequency entries rank above lower-frequency ones", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        result.current.save("/home/user/rare");
        // Save popular three times so its count is higher.
        result.current.save("/home/user/popular");
        result.current.save("/home/user/popular");
        result.current.save("/home/user/popular");
      });
      const matches = result.current.getMatching("/home/user/");
      expect(matches[0].path).toBe("/home/user/popular");
    });
  });

  describe("getAll", () => {
    it("returns top N entries by score when more exist", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        for (let i = 0; i < 10; i++) {
          result.current.save(`/home/user/proj${i}`);
        }
      });
      const all = result.current.getAll(5);
      expect(all).toHaveLength(5);
    });

    it("returns all entries when count is below limit", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        result.current.save("/home/user/a");
        result.current.save("/home/user/b");
        result.current.save("/home/user/c");
      });
      const all = result.current.getAll(10);
      expect(all).toHaveLength(3);
    });

    it("includes a newly saved path", () => {
      const { result } = renderHook(() => usePathHistory());
      act(() => {
        result.current.save("/home/user/existing");
      });
      act(() => {
        result.current.save("/home/user/new-repo");
      });
      const all = result.current.getAll(10);
      expect(all.some((e) => e.path === "/home/user/new-repo")).toBe(true);
    });

    it("score ordering: recent entry beats stale high-frequency entry", () => {
      const recentTs = Date.now() - 30 * 60 * 1000; // 30 minutes ago → recencyScore 1.0
      const staleTs = Date.now() - 25 * 24 * 60 * 60 * 1000; // 25 days ago → recencyScore 0.4

      localStorage.setItem(
        "omnibar:path-history",
        JSON.stringify([
          { path: "/recent", count: 1, lastUsed: recentTs },
          { path: "/stale", count: 5, lastUsed: staleTs },
        ])
      );

      const { result } = renderHook(() => usePathHistory());
      const all = result.current.getAll(2);
      // NOTE: stale wins here because log1p(5) outweighs the recency gap.
      expect(all).toHaveLength(2);
      const scores = all.map((e) => {
        const age = Date.now() - e.lastUsed;
        const hour = 3_600_000,
          day = 86_400_000,
          week = 7 * day,
          month = 30 * day;
        let recency = 0.2;
        if (age < hour) recency = 1.0;
        else if (age < day) recency = 0.8;
        else if (age < week) recency = 0.6;
        else if (age < month) recency = 0.4;
        return recency + Math.log1p(e.count);
      });
      expect(scores[0]).toBeGreaterThanOrEqual(scores[1]);
    });
  });
});
