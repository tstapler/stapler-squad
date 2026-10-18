import { runBulk, formatBulkFeedback } from "./bulkActions";

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
