import {
  emitTabEvent,
  emptyTabStats,
  readStats,
  recordTabEvent,
  TAB_STATS_STORAGE_KEY,
  type TabEvent,
} from "./tabStats";

describe("recordTabEvent", () => {
  beforeEach(() => window.localStorage.clear());

  it("recordTabEvent_should_CountLandedLeftPrsWithin5sAndFirstPrCardMs_When_EventsReplayed", () => {
    const events: TabEvent[] = [
      { type: "visit", landedOn: "prs" },
      { type: "firstPrCard", ms: 1234 },
      { type: "click", from: "prs", to: "stuck", msSinceVisit: 4000, firstClickOfVisit: true },
      { type: "click", from: "stuck", to: "queue", msSinceVisit: 4500, firstClickOfVisit: false },
      { type: "visit", landedOn: "queue" },
      { type: "click", from: "queue", to: "prs", msSinceVisit: 100, firstClickOfVisit: true },
      { type: "visit", landedOn: "prs" },
      { type: "click", from: "prs", to: "worktrees", msSinceVisit: 5001, firstClickOfVisit: true },
    ];
    const stats = events.reduce(recordTabEvent, emptyTabStats());
    expect(stats).toEqual({
      visits: 3,
      landed: { prs: 2, stuck: 0, worktrees: 0, queue: 1 },
      leftPrsWithin5s: 1,
      firstPrCardMs: 1234,
    });
  });

  it("swallows a throwing localStorage and makes no network call", () => {
    const fetchSpy = jest.fn();
    (global as unknown as { fetch: unknown }).fetch = fetchSpy;
    const get = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    const set = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    try {
      expect(readStats()).toEqual(emptyTabStats());
      expect(() => emitTabEvent({ type: "visit", landedOn: "prs" })).not.toThrow();
      expect(fetchSpy).not.toHaveBeenCalled();
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });

  it("round-trips through localStorage and tolerates malformed JSON", () => {
    emitTabEvent({ type: "visit", landedOn: "stuck" });
    expect(readStats().landed.stuck).toBe(1);
    window.localStorage.setItem(TAB_STATS_STORAGE_KEY, "{not json");
    expect(readStats()).toEqual(emptyTabStats());
  });
});
