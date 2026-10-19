import {
  runBulk, formatBulkFeedback, isBulkTriageEligible, isBulkAutoImplementEligible, runBulkChunked,
  runBulkSpawnChunked, formatSpawnBulkFeedback, formatTriageFeedback, runGuarded,
} from "./bulkActions";

describe("bulkActions", () => {
  it("collects successes and failures without aborting", async () => {
    const calls: string[] = [];
    const result = await runBulk(["a", "b", "c"], async (id) => {
      calls.push(id);
      if (id === "b") throw new Error("boom");
    });
    expect(calls).toEqual(["a", "b", "c"]);
    expect(result).toEqual({ succeeded: ["a", "c"], failed: ["b"] });
  });

  it("formats full success", () => {
    expect(formatBulkFeedback("Archived", { succeeded: ["a", "b"], failed: [] })).toBe("Archived 2 items");
    expect(formatBulkFeedback("Archived", { succeeded: ["a"], failed: [] })).toBe("Archived 1 item");
  });

  it("formats partial failure", () => {
    expect(formatBulkFeedback("Archived", { succeeded: ["a", "b"], failed: ["c"] })).toBe("Archived 2 of 3 (1 failed)");
  });
});

describe("bulk eligibility", () => {
  it.each([
    ["idea", undefined, true],
    ["ready", "", true],
    ["ready", "running", false],
    ["in_progress", undefined, false],
    ["archived", undefined, false],
  ])("triage: %s / %s -> %s", (status, triageStatus, want) => {
    expect(isBulkTriageEligible({ status, triageStatus })).toBe(want);
  });

  it.each([
    [{ status: "ready", skipPlanning: true }, true],
    [{ status: "ready", planApproved: true }, true],
    [{ status: "ready" }, false],
    [{ status: "in_progress", planApproved: true }, false],
    [{ status: "idea", skipPlanning: true }, false],
  ])("auto-implement: %j -> %s", (item, want) => {
    expect(isBulkAutoImplementEligible(item)).toBe(want);
  });
});

describe("runBulkChunked", () => {
  it("never exceeds the chunk size and reports partial failure across chunks", async () => {
    let inFlight = 0;
    let maxInFlight = 0;
    const ids = ["1", "2", "3", "4", "5", "6", "7"];
    const result = await runBulkChunked(ids, async (id) => {
      inFlight++;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await Promise.resolve();
      inFlight--;
      if (id === "3" || id === "6") throw new Error("x");
    }, 5);
    expect(maxInFlight).toBe(5);
    expect(result.failed).toEqual(["3", "6"]);
    expect(result.succeeded).toEqual(["1", "2", "4", "5", "7"]);
  });

  it("handles an empty list", async () => {
    expect(await runBulkChunked([], async () => {})).toEqual({ succeeded: [], failed: [] });
  });
});

describe("runBulkSpawnChunked", () => {
  it("buckets started, queued, failed (rejection and null)", async () => {
    const result = await runBulkSpawnChunked(["a", "b", "c", "d"], async (id) => {
      if (id === "b") return { queued: true };
      if (id === "c") throw new Error("x");
      if (id === "d") return null;
      return { queued: false };
    });
    expect(result).toEqual({ started: ["a"], queued: ["b"], failed: ["c", "d"] });
  });

  it("limits concurrency to the chunk size", async () => {
    let inFlight = 0;
    let max = 0;
    await runBulkSpawnChunked(["1", "2", "3", "4", "5", "6"], async () => {
      inFlight++;
      max = Math.max(max, inFlight);
      await Promise.resolve();
      inFlight--;
      return { queued: false };
    }, 5);
    expect(max).toBe(5);
  });
});

describe("feedback strings", () => {
  it("spawn feedback", () => {
    expect(formatSpawnBulkFeedback({ started: ["a"], queued: [], failed: [] })).toBe("Started 1 autonomous session");
    expect(formatSpawnBulkFeedback({ started: ["a", "b"], queued: [], failed: [] })).toBe("Started 2 autonomous sessions");
    expect(formatSpawnBulkFeedback({ started: ["a"], queued: ["b"], failed: [] })).toBe("Started 1, queued 1");
    expect(formatSpawnBulkFeedback({ started: ["a"], queued: ["b"], failed: ["c"] })).toBe("Started 1, queued 1 (1 failed)");
    expect(formatSpawnBulkFeedback({ started: [], queued: ["b"], failed: [] })).toBe("Queued 1");
  });

  it("triage feedback", () => {
    expect(formatTriageFeedback({ succeeded: ["a", "b"], failed: [] }, 0)).toBe("Triggered triage for 2 items");
    expect(formatTriageFeedback({ succeeded: ["a"], failed: ["b"] }, 0)).toBe("Triggered 1 of 2 (1 failed)");
    expect(formatTriageFeedback({ succeeded: [], failed: [] }, 3)).toBe("Nothing to triage (3 ineligible)");
  });
});

describe("runGuarded", () => {
  it("drops a second call while the first is in flight and releases afterwards", async () => {
    const busyRef = { current: false };
    const setBusy = jest.fn();
    let release!: () => void;
    const fn = jest.fn(() => new Promise<void>((r) => { release = r; }));
    const first = runGuarded(busyRef, setBusy, fn);
    const second = await runGuarded(busyRef, setBusy, fn);
    expect(second).toBeUndefined();
    expect(fn).toHaveBeenCalledTimes(1);
    release();
    await first;
    expect(busyRef.current).toBe(false);
    expect(setBusy.mock.calls).toEqual([[true], [false]]);
  });
});
