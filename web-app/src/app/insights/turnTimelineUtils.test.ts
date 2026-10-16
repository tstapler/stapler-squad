import { create } from "@bufbuild/protobuf";
import { TurnTokenStatSchema, type TurnTokenStat } from "@/gen/session/v1/insights_pb";
import { sortTurnsByTokensDesc, computeOutlierThreshold, isOutlierTurn } from "./turnTimelineUtils";

function makeTurn(input: bigint, output: bigint, cacheCreation = 0n, cacheRead = 0n): TurnTokenStat {
  return create(TurnTokenStatSchema, {
    model: "claude-sonnet-4",
    inputTokens: input,
    outputTokens: output,
    cacheCreationTokens: cacheCreation,
    cacheReadTokens: cacheRead,
    toolNames: [],
  });
}

describe("sortTurnsByTokensDesc", () => {
  it("sortTurnsByTokensDesc_should_orderTurnsByTotalTokensDescending_When_turnsVarySize", () => {
    const small = makeTurn(10n, 5n);
    const large = makeTurn(1000n, 500n);
    const medium = makeTurn(100n, 50n);

    const sorted = sortTurnsByTokensDesc([small, large, medium]);

    expect(sorted).toEqual([large, medium, small]);
  });

  it("sortTurnsByTokensDesc_should_returnEmptyArray_When_turnsArrayEmpty", () => {
    expect(sortTurnsByTokensDesc([])).toEqual([]);
  });
});

describe("computeOutlierThreshold", () => {
  it("computeOutlierThreshold_should_returnZero_When_turnsArrayEmpty", () => {
    expect(computeOutlierThreshold([])).toBe(0);
  });

  it("computeOutlierThreshold_should_returnTwiceMean_When_turnsVarySize", () => {
    // totals: 100, 200, 300 -> mean 200 -> threshold 400
    const turns = [makeTurn(60n, 40n), makeTurn(120n, 80n), makeTurn(180n, 120n)];
    expect(computeOutlierThreshold(turns)).toBe(400);
  });
});

describe("isOutlierTurn", () => {
  it("isOutlierTurn_should_returnFalseAndNotThrow_When_turnsArrayEmpty", () => {
    const threshold = computeOutlierThreshold([]);
    const zeroTokenTurn = makeTurn(0n, 0n);
    expect(() => isOutlierTurn(zeroTokenTurn, threshold)).not.toThrow();
    expect(isOutlierTurn(zeroTokenTurn, threshold)).toBe(false);
  });

  it("isOutlierTurn_should_returnFalse_When_totalEqualsTwiceMeanThresholdExactly", () => {
    // total = 400, threshold = 400 -> exactly at, not above -> not an outlier
    const turn = makeTurn(300n, 100n);
    expect(isOutlierTurn(turn, 400)).toBe(false);
  });

  it("isOutlierTurn_should_returnTrue_When_totalExceedsThreshold", () => {
    const turn = makeTurn(300n, 101n);
    expect(isOutlierTurn(turn, 400)).toBe(true);
  });

  it("isOutlierTurn_should_countCacheTokens_When_turnIsCacheDominated", () => {
    // input+output alone (10+10=20) is well under the threshold, but the huge
    // cache read (1000) should push the turn's total over it.
    const turn = makeTurn(10n, 10n, 0n, 1000n);
    expect(isOutlierTurn(turn, 400)).toBe(true);
  });
});
