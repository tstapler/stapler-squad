import { createCoalescer, createTokenBucket, createWindowAggregator } from "../sampling";
import { fakeClock } from "../../__tests__/fixtures/fakeClock";

describe("sampling", () => {
  it("sampling_should_CoalesceTabSwitchCapTokenBucketAndAggregateExtraKeys_When_FakeTimers", () => {
    const { clock, advance } = fakeClock();

    const allowed = createCoalescer(clock);
    let passed = 0;
    for (let i = 0; i < 100; i++) {
      if (allowed("files.tab.switch", 250)) passed++;
      advance(1);
    }
    expect(passed).toBe(1);
    advance(250);
    expect(allowed("files.tab.switch", 250)).toBe(true);

    const dropped: [string, number][] = [];
    const take = createTokenBucket((k, n) => dropped.push([k, n]), clock);
    const kept = Array.from({ length: 50 }, () => take("edit_save")).filter(Boolean).length;
    expect(kept).toBe(30);
    expect(dropped).toEqual([]);
    advance(60_000);
    expect(dropped).toEqual([["edit_save", 20]]);
    expect(take("edit_save")).toBe(true);

    const closed: [string, number][] = [];
    const keys = createWindowAggregator((k, n) => closed.push([k, n]), clock);
    for (let i = 0; i < 40; i++) { keys.add("arrow"); advance(500); }
    expect(closed).toEqual([]);
    advance(10_000);
    expect(closed).toEqual([["arrow", 40]]);
  });

  it("windowAggregator_should_FlushOnDemandAndClearTheTimer_When_PagehideFires", () => {
    const { clock, advance } = fakeClock();
    const closed: [string, number][] = [];
    const keys = createWindowAggregator((k, n) => closed.push([k, n]), clock);
    keys.add("esc");
    keys.add("esc");
    keys.flush();
    expect(closed).toEqual([["esc", 2]]);
    advance(60_000);
    expect(closed).toHaveLength(1);
  });
});
