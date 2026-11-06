import React from "react";
import { render, screen } from "@testing-library/react";
import { ItemBudgetWarning } from "./ItemBudgetWarning";

describe("ItemBudgetWarning", () => {
  it("ItemBudgetWarning_should_RenderNull_When_ThresholdUnset", () => {
    const { container } = render(
      <ItemBudgetWarning thresholdUsd={undefined} totalCostUsd={5.05} />
    );

    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId("item-budget-warning")).toBeNull();
  });

  it("ItemBudgetWarning_should_RenderNull_When_TotalCostBelowThreshold", () => {
    render(<ItemBudgetWarning thresholdUsd={5.0} totalCostUsd={3.0} />);

    expect(screen.queryByTestId("item-budget-warning")).toBeNull();
  });

  it("ItemBudgetWarning_should_ShowWarning_When_TotalCostMeetsOrExceedsThreshold", () => {
    render(<ItemBudgetWarning thresholdUsd={5.0} totalCostUsd={5.05} />);

    const warning = screen.getByTestId("item-budget-warning");
    expect(warning).toBeInTheDocument();
    expect(warning).toHaveTextContent(/over budget/i);
    expect(warning).toHaveTextContent("$5.05");
    expect(warning).toHaveTextContent("$5.00");
  });
});
