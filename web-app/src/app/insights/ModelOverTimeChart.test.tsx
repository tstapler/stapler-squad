import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { ModelOverTimeChart } from "./ModelOverTimeChart";
import { DailyTokenBucketSchema, type DailyTokenBucket } from "@/gen/session/v1/insights_pb";

function makeBucket(fields: {
  day: number;
  costByModel?: Record<string, number>;
  tokensByModel?: Record<string, bigint>;
  unpricedModels?: string[];
}): DailyTokenBucket {
  return create(DailyTokenBucketSchema, {
    date: timestampFromDate(new Date(Date.UTC(2026, 6, fields.day))),
    estimatedCostUsd: 0,
    totalInputTokens: 0n,
    totalOutputTokens: 0n,
    cacheReadTokens: 0n,
    sessionCount: 1,
    costByModel: fields.costByModel ?? {},
    tokensByModel: fields.tokensByModel ?? {},
    unpricedModels: fields.unpricedModels ?? [],
  });
}

describe("ModelOverTimeChart", () => {
  it("ModelOverTimeChart_should_showUnpricedSuffix_When_familyInUnpricedModels", () => {
    render(
      <ModelOverTimeChart
        daily={[
          makeBucket({
            day: 1,
            costByModel: { "claude-opus-6": 0 },
            tokensByModel: { "claude-opus-6": 500000n },
            unpricedModels: ["claude-opus-6"],
          }),
        ]}
      />
    );

    expect(screen.getByText(/pricing unavailable/i)).toBeInTheDocument();
  });

  it("ModelOverTimeChart_should_renderUnchanged_When_familyNotInUnpricedModels", () => {
    render(
      <ModelOverTimeChart
        daily={[
          makeBucket({
            day: 1,
            costByModel: { "claude-sonnet-4": 0.02 },
            tokensByModel: { "claude-sonnet-4": 500000n },
          }),
        ]}
      />
    );

    expect(screen.getByText("claude-sonnet-4")).toBeInTheDocument();
    expect(screen.queryByText(/pricing unavailable/i)).toBeNull();
  });
});
