import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { DailySpendChart } from "./DailySpendChart";
import { DailyTokenBucketSchema, type DailyTokenBucket } from "@/gen/session/v1/insights_pb";

function makeBucket(fields: {
  day: number;
  cost?: number;
  unpricedModels?: string[];
}): DailyTokenBucket {
  return create(DailyTokenBucketSchema, {
    date: timestampFromDate(new Date(Date.UTC(2026, 6, fields.day))),
    estimatedCostUsd: fields.cost ?? 0,
    totalInputTokens: 0n,
    totalOutputTokens: 0n,
    cacheReadTokens: 0n,
    sessionCount: 1,
    costByModel: {},
    tokensByModel: {},
    unpricedModels: fields.unpricedModels ?? [],
  });
}

describe("DailySpendChart", () => {
  it("DailySpendChart_should_showUnpricedFootnote_When_anyDayHasUnpricedModels", () => {
    render(
      <DailySpendChart
        daily={[
          makeBucket({ day: 1, cost: 0.01 }),
          makeBucket({ day: 2, cost: 0, unpricedModels: ["claude-opus-6"] }),
          makeBucket({ day: 3, cost: 0.02 }),
        ]}
      />
    );

    expect(screen.getByText(/1 day includes unpriced model usage/i)).toBeInTheDocument();
  });

  it("DailySpendChart_should_omitFootnote_When_noDayHasUnpricedModels", () => {
    render(
      <DailySpendChart
        daily={[
          makeBucket({ day: 1, cost: 0.01 }),
          makeBucket({ day: 2, cost: 0.02 }),
        ]}
      />
    );

    expect(screen.queryByText(/unpriced model usage/i)).toBeNull();
  });

  it("DailySpendChart_should_pluralizeFootnote_When_multipleDaysHaveUnpricedModels", () => {
    render(
      <DailySpendChart
        daily={[
          makeBucket({ day: 1, cost: 0, unpricedModels: ["claude-opus-6"] }),
          makeBucket({ day: 2, cost: 0, unpricedModels: ["claude-opus-6"] }),
        ]}
      />
    );

    expect(screen.getByText(/2 days include unpriced model usage/i)).toBeInTheDocument();
  });
});
