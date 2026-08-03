import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { ModelBreakdownChart } from "./ModelBreakdownChart";
import { ModelBreakdownSchema, type ModelBreakdown } from "@/gen/session/v1/insights_pb";

function makeModel(fields: {
  modelFamily: string;
  estimatedCostUsd?: number;
  pricingUnavailable?: boolean;
  totalInputTokens?: bigint;
  totalOutputTokens?: bigint;
  cacheReadTokens?: bigint;
}): ModelBreakdown {
  return create(ModelBreakdownSchema, {
    modelFamily: fields.modelFamily,
    estimatedCostUsd: fields.estimatedCostUsd ?? 0,
    pricingUnavailable: fields.pricingUnavailable ?? false,
    totalInputTokens: fields.totalInputTokens ?? 0n,
    totalOutputTokens: fields.totalOutputTokens ?? 0n,
    cacheReadTokens: fields.cacheReadTokens ?? 0n,
    sessionCount: 1,
  });
}

describe("ModelBreakdownChart", () => {
  it("ModelBreakdownChart_should_showUnpricedSuffix_When_pricingUnavailable", () => {
    render(
      <ModelBreakdownChart
        models={[
          makeModel({
            modelFamily: "claude-opus-6",
            estimatedCostUsd: 0,
            pricingUnavailable: true,
            totalInputTokens: 500000n,
          }),
        ]}
      />
    );

    expect(screen.getByText(/pricing unavailable/i)).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("ModelBreakdownChart_should_renderUnchanged_When_pricingUnavailableFalseOrAbsent", () => {
    const { unmount } = render(
      <ModelBreakdownChart
        models={[
          makeModel({
            modelFamily: "claude-sonnet-4",
            estimatedCostUsd: 0.02,
            pricingUnavailable: false,
          }),
        ]}
      />
    );

    expect(screen.getByText("claude-sonnet-4")).toBeInTheDocument();
    expect(screen.queryByText(/pricing unavailable/i)).toBeNull();
    unmount();

    // field omitted entirely (defaults to false on the proto message)
    render(
      <ModelBreakdownChart
        models={[create(ModelBreakdownSchema, { modelFamily: "claude-haiku-4", estimatedCostUsd: 0.01 })]}
      />
    );

    expect(screen.getByText("claude-haiku-4")).toBeInTheDocument();
    expect(screen.queryByText(/pricing unavailable/i)).toBeNull();
  });

  it("ModelBreakdownChart_should_renderFullChartNotEmptyState_When_allModelsUnpriced", () => {
    render(
      <ModelBreakdownChart
        models={[
          makeModel({ modelFamily: "claude-sonnet-5", pricingUnavailable: true }),
          makeModel({ modelFamily: "claude-opus-6", pricingUnavailable: true }),
        ]}
      />
    );

    expect(screen.queryByText("No data")).toBeNull();
    expect(screen.getByText(/claude-sonnet-5/)).toBeInTheDocument();
    expect(screen.getByText(/claude-opus-6/)).toBeInTheDocument();
    expect(screen.getAllByText(/pricing unavailable/i)).toHaveLength(2);
  });

  it("ModelBreakdownChart_should_showCacheHitRate_When_modelHasCacheReads", () => {
    render(
      <ModelBreakdownChart
        models={[
          makeModel({
            modelFamily: "claude-opus-6",
            estimatedCostUsd: 1.0,
            totalInputTokens: 300n,
            cacheReadTokens: 700n,
          }),
        ]}
      />
    );

    // 700 / (300 + 700) = 70.0%
    expect(screen.getByText(/70\.0% cache hit/)).toBeInTheDocument();
  });

  it("ModelBreakdownChart_should_showZeroPercentCacheHit_When_noCacheEligibleTokens", () => {
    render(
      <ModelBreakdownChart
        models={[
          makeModel({
            modelFamily: "claude-sonnet-4",
            estimatedCostUsd: 0.02,
            totalInputTokens: 0n,
            cacheReadTokens: 0n,
          }),
        ]}
      />
    );

    expect(screen.getByText(/0\.0% cache hit/)).toBeInTheDocument();
  });
});
