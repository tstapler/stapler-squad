import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { GetInsightsSummaryResponseSchema } from "@/gen/session/v1/insights_pb";
import { SummaryCards } from "./SummaryCards";

describe("SummaryCards", () => {
  it("SummaryCards_should_showUnpricedFootnote_When_unpricedModelsPresent", () => {
    const summary = create(GetInsightsSummaryResponseSchema, {
      totalCostUsd: 0.045,
      unpricedModels: ["claude-opus-6"],
    });

    render(<SummaryCards summary={summary} />);

    expect(screen.getByText(/excludes 1 unpriced model/i)).toBeInTheDocument();
  });

  it("SummaryCards_should_showPluralFootnote_When_multipleUnpricedModels", () => {
    const summary = create(GetInsightsSummaryResponseSchema, {
      totalCostUsd: 0.045,
      unpricedModels: ["claude-opus-6", "claude-haiku-6"],
    });

    render(<SummaryCards summary={summary} />);

    expect(screen.getByText(/excludes 2 unpriced models/i)).toBeInTheDocument();
  });

  it("SummaryCards_should_omitFootnote_When_noUnpricedModels", () => {
    const summary = create(GetInsightsSummaryResponseSchema, {
      totalCostUsd: 0.045,
      unpricedModels: [],
    });

    render(<SummaryCards summary={summary} />);

    expect(screen.queryByText(/unpriced/i)).not.toBeInTheDocument();
  });

  it("SummaryCards_should_notRenderAlertRole_When_unpricedFootnoteShown", () => {
    const summary = create(GetInsightsSummaryResponseSchema, {
      totalCostUsd: 0.045,
      unpricedModels: ["claude-opus-6"],
    });

    render(<SummaryCards summary={summary} />);

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
